// Dialogs are used for asking a user about something or for confirming actions that are going to be executed.

package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"strings"
)

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

// dialogOpen opens view for the dialog.
func dialogOpen(app *app, d dialogType) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		prompt := dialogPrompts(d)

		// some types of actions allowed only in specifics stats contexts.
		if (d > dialogFilter && d <= dialogChangeAge) && app.config.view.Name != "activity" {
			var msg string
			switch d {
			case dialogCancelQuery, dialogTerminateBackend, dialogCancelGroup, dialogTerminateGroup:
				msg = "Terminate backends or cancel queries allowed in pg_stat_activity view only."
			case dialogSetMask:
				msg = "State mask setup allowed in pg_stat_activity view only."
			case dialogChangeAge:
				msg = "Changing queries age threshold allowed in pg_stat_activity view only."
			}
			printCmdline(g, msg)
			return nil
		}

		if d == dialogQueryReport && !strings.Contains(app.config.view.Name, "statements") {
			printCmdline(g, "Query reports allowed in pg_stat_statements views only.")
			return nil
		}

		maxX, _ := g.Size()

		// Create one-line editable view, print a prompt and set cursor after it.
		v, err := g.SetView("dialog", len(prompt)-1, 3, maxX-1, 5)
		if err != nil {
			// gocui.ErrUnknownView is OK it means a new view has been created, continue if it happens.
			if err != gocui.ErrUnknownView {
				return fmt.Errorf("set dialog view on layout failed: %s", err)
			}
		}

		p, err := g.View("cmdline")
		if err != nil {
			return fmt.Errorf("set focus on cmdline view failed: %s", err)
		}

		_, err = fmt.Fprint(p, prompt)
		if err != nil {
			return fmt.Errorf("print to cmdline view failed: %s", err)
		}

		g.Cursor = true
		v.Editable = true
		v.Frame = false

		if _, err := g.SetCurrentView("dialog"); err != nil {
			return fmt.Errorf("set dialog view as current on layout failed: %s", err)
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
			r, message = getQueryReport(answer, app.postgresProps.VersionNum, app.db)
			if message == "" {
				message = printQueryReport(g, r, app.uiExit)
			}
		case dialogChangeRefresh:
			message = changeRefresh(answer, app.config)
		case dialogNone:
			// do nothing
		}

		printCmdline(g, message)

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
		return fmt.Errorf("deleting dialog view failed: %s", err)
	}

	// Switch focus from destroyed 'dialog' view to 'sysstat'.
	_, err = g.SetCurrentView("sysstat")
	if err != nil {
		return fmt.Errorf("set sysstat view as current on layout failed: %s", err)
	}

	return nil
}
