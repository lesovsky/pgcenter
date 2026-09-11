package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"strings"
	"unicode/utf8"
)

// minDialogInputWidth is the minimum number of USABLE columns the dialog input field must keep on
// screen. A gocui view spanning x0..x1 offers x1-x0-1 usable columns, so this is the room reserved
// for the field before the prompt is composed - an overlong prompt is truncated into what is left,
// never overlaid by the field. Ten columns fit a PID, a refresh value, a y/n answer and a short
// regexp; longer input scrolls inside gocui's editor rather than widening the view.
const minDialogInputWidth = 10

// dialogType defines type of dialog between pgcenter and user.
type dialogType int

const (
	// All possible dialog types.
	dialogNone dialogType = iota
	dialogPgReload
	dialogFilter
	dialogCancelQuery
	dialogTerminateBackend
	dialogCancelGroup
	dialogTerminateGroup
	dialogSetMask
	dialogChangeAge
	dialogQueryReport
	dialogChangeRefresh
)

// dialogPrompts returns dialog prompt depending on user-requested actions.
func dialogPrompts(t dialogType) string {
	prompts := map[dialogType]string{
		dialogPgReload:         "Reload configuration files (y/n): ",
		dialogFilter:           "Set filter: ",
		dialogCancelQuery:      "PID to cancel: ",
		dialogTerminateBackend: "PID to terminate: ",
		dialogCancelGroup:      "Cancel group of queries. Confirm [Enter - yes, Esc - no]",
		dialogTerminateGroup:   "Terminate group of backends. Confirm [Enter - yes, Esc - no]",
		dialogSetMask:          "Set state mask for group backends [a: active, i: idle, x: idle_xact, w: waiting, o: others]: ",
		dialogChangeAge:        "Enter new min age, format: HH:MM:SS[.NN]: ",
		dialogQueryReport:      "Enter the queryid: ",
		dialogChangeRefresh:    "Change refresh (min 1, max 300) to ",
	}

	return prompts[t]
}

// dialogPromptFit cuts prompt down to what fits on the command line next to the state prefix
// without eating into the room reserved for the input field. The budget is
// maxX - minWidth - 1: the field's usable columns plus the column x0 itself occupies.
//
// The prefix is rendered at maxX - the width the cmdline writer will use - so the tokens degrade
// here exactly as they will on screen, and the line the writer draws is by construction the one
// this budget was computed against. Returns the prompt to PRINT: the caller must hand this value,
// not the original, to the writer, which composes the prefix itself.
//
// A prompt that did not fit is marked with an ellipsis, the same way the indicator ladder marks its
// own cuts - otherwise the user reads a sentence that simply stops and cannot tell it was cut. The
// marker is spent FROM the budget, not added on top of it: one rune less of text, so the reserved
// room for the input field is untouched and the clamp in dialogInputX0 still never has to bite.
//
// Lengths are rune counts, not bytes (Decision 12): the truncation ladder emits '…' and the header
// emits '‹'/'›'. When the prefix alone exceeds the budget - a terminal narrower than the
// reservation - the prompt disappears entirely rather than pushing the field off the screen.
func dialogPromptFit(tokens []cmdlineToken, prompt string, maxX, minWidth int) string {
	prefix := composeCmdline(tokens, "", maxX)

	room := maxX - minWidth - 1 - utf8.RuneCountInString(prefix)
	if prefix != "" {
		// composeCmdline puts a single space between the prefix and the message.
		room--
	}

	if utf8.RuneCountInString(prompt) <= room {
		return prompt
	}
	if room < 2 {
		// Nothing but the marker would fit, and a lone '…' says less than an empty command line
		// while still costing the column. Drop the prompt, as the no-room case does.
		return ""
	}

	return truncateRunes(prompt, room-1) + "…"
}

// dialogInputX0 computes the x0 of the dialog input view from the PRINTED (rune, not byte) length
// of the composed command line; x1 is maxX-1. The base value reproduces "start right after the
// text", and the bounds keep the result inside [-1, maxX-2-minWidth].
//
// Why the bounds matter: SetView rejects x0 >= x1 with "invalid dimensions", that error leaves the
// key handler through onKey -> handleEvent -> MainLoop, and mainLoop reacts to it by tearing down
// and rebuilding the whole UI - every view, doWork and collectStat included. A mispositioned input
// field is a cosmetic bug; an inverted one restarts the interface.
//
// Domain: maxX >= 1, which is what dialogOpen's degenerate-geometry guard admits. At maxX <= 0 the
// right edge is invalid by itself and no x0 can help, hence the guard rather than a wider clamp.
// With the reserved budget of dialogPromptFit the upper bound should never bite; it stays as a
// backstop so a future change to the budget cannot silently bring the teardown back.
func dialogInputX0(cmdlineLen, maxX, minWidth int) int {
	x0 := cmdlineLen - 1

	if hi := maxX - 2 - minWidth; x0 > hi {
		x0 = hi
	}
	if x0 < -1 {
		x0 = -1
	}

	return x0
}

// dialogOpen opens view for the dialog.
func dialogOpen(app *app, d dialogType) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		prompt := dialogPrompts(d)

		// Cancel/terminate/mask dialogs are allowed in pg_stat_activity view only.
		if (d == dialogCancelQuery || d == dialogTerminateBackend ||
			d == dialogCancelGroup || d == dialogTerminateGroup ||
			d == dialogSetMask) && app.config.view.Name != "activity" {
			var msg string
			switch d {
			case dialogCancelQuery, dialogTerminateBackend, dialogCancelGroup, dialogTerminateGroup:
				msg = "Terminate backends or cancel queries allowed in pg_stat_activity view only."
			case dialogSetMask:
				msg = "State mask setup allowed in pg_stat_activity view only."
			}
			printCmdline(g, "%s", msg)
			return nil
		}

		// Changing queries age threshold is allowed in pg_stat_activity and procpidstat views.
		if d == dialogChangeAge && app.config.view.Name != "activity" && app.config.view.Name != "procpidstat" {
			printCmdline(g, "%s", "Changing queries age threshold allowed in pg_stat_activity view only.")
			return nil
		}

		if d == dialogQueryReport && !strings.Contains(app.config.view.Name, "statements") {
			printCmdline(g, "Query reports allowed in pg_stat_statements views only.")
			return nil
		}

		maxX, maxY := g.Size()

		// Screen dimensions can read as zero after returning from an external program like a
		// pager/editor/psql - the same case layout() guards. The right edge would be maxX-1 = -1,
		// which no x0 can sit left of, so there is no dialog to open: report and leave, as the
		// view guards above do. The next redraw restores the real size.
		if maxX <= 0 || maxY <= 0 {
			printCmdline(g, "%s", "Terminal is too small to open a dialog.")
			return nil
		}

		// The cmdline view spans -1..maxX, so the writer composes against exactly maxX columns.
		// Reserve the input field's room in that budget, so an overlong prompt is truncated
		// instead of being overlaid by the field, and measure the very line the writer will draw.
		tokens := cmdlineTokens(app.config)
		prompt = dialogPromptFit(tokens, prompt, maxX, minDialogInputWidth)
		line := composeCmdline(tokens, prompt, maxX)
		x0 := dialogInputX0(utf8.RuneCountInString(line), maxX, minDialogInputWidth)

		// The dialog shares the command line's row, so it takes the same geometry layout() gives
		// the cmdline view in this frame - in compact mode these are the historical 3 and 5.
		_, _, cmdlineY0, cmdlineY1, _, _ := topBandLayout(app.config.verbose, maxY)

		// Create one-line editable view, print a prompt and set cursor after it.
		v, err := g.SetView("dialog", x0, cmdlineY0, maxX-1, cmdlineY1)
		if err != nil {
			// gocui.ErrUnknownView is OK it means a new view has been created, continue if it happens.
			if err != gocui.ErrUnknownView {
				return fmt.Errorf("set dialog view on layout failed: %w", err)
			}
		}

		// Print the prompt through the cmdline composer, so it lands behind the state prefix
		// instead of overwriting it - and without arming the clear timer, which would erase the
		// prompt from under the user's cursor after two seconds. The already truncated prompt is
		// passed, not the original: the writer composes the prefix itself, and this is the string
		// the field position was computed from.
		printCmdlinePersist(g, "%s", prompt)

		g.Cursor = true
		v.Editable = true
		v.Frame = false

		if _, err := g.SetCurrentView("dialog"); err != nil {
			return fmt.Errorf("set dialog view as current on layout failed: %w", err)
		}

		// Remember the type of an opened dialog. It will be required when the dialog will be finished.
		app.config.dialog = d

		return nil
	}
}

// dialogFinish runs proper handler after user submits its dialog input.
func dialogFinish(app *app) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		printCmdline(g, "")

		// Extract user entered answer from buffer.
		answer := strings.TrimPrefix(v.Buffer(), dialogPrompts(app.config.dialog))
		answer = strings.TrimSuffix(answer, "\n")

		var message string

		switch app.config.dialog {
		case dialogPgReload:
			message = doReload(answer, app.db)
		case dialogFilter:
			message = setFilter(answer, app.config.view)
			app.config.verticalOffset = 0
		case dialogCancelQuery:
			message = killSingle(app.db, "cancel", answer)
		case dialogTerminateBackend:
			message = killSingle(app.db, "terminate", answer)
		case dialogSetMask:
			message = setProcMask(answer, app.config)
		case dialogCancelGroup:
			message = killGroup(app, "cancel")
		case dialogTerminateGroup:
			message = killGroup(app, "terminate")
		case dialogChangeAge:
			message = changeQueryAge(answer, app.config)
		case dialogQueryReport:
			var r report
			r, message = getQueryReport(answer, app.postgresProps.VersionNum, app.postgresProps.ExtPGSSSchema, app.db)
			if message == "" {
				message = printQueryReport(g, r, app.uiExit)
			}
		case dialogChangeRefresh:
			message = changeRefresh(answer, app.config)
		case dialogNone:
			// do nothing
		}

		printCmdline(g, "%s", message)

		return dialogClose(g, v)
	}
}

// dialogCancel reset dialog state when user cancels input.
func dialogCancel(app *app) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		app.config.dialog = dialogNone
		printCmdline(g, "Do nothing. Operation canceled.")
		return dialogClose(g, v)
	}
}

// dialogClose destroys UI view object related to dialog.
func dialogClose(g *gocui.Gui, v *gocui.View) error {
	g.Cursor = false
	v.Clear()

	err := g.DeleteView("dialog")
	if err != nil {
		return fmt.Errorf("deleting dialog view failed: %w", err)
	}

	// Switch focus from destroyed 'dialog' view to 'sysstat'.
	_, err = g.SetCurrentView("sysstat")
	if err != nil {
		return fmt.Errorf("set sysstat view as current on layout failed: %w", err)
	}

	return nil
}
