// Dialogs are used for asking a user about something or for confirming actions that are going to be executed.

package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"strings"
)

type dialogType int

// Dialog types
const (
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

var (
	// There is a prompt for every dialog.
	dialogPrompts = map[dialogType]string{
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

	// Variable-transporter, function which check user's input, uses this variable to select appropriate handler. Depending on dialog type, select appropriate function.
	dialog dialogType
)

// Open 'gocui' view for the dialog.
func dialogOpen(app *app, d dialogType) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		// some types of actions allowed only in specifics stats contexts.
		if (d > dialogFilter && d <= dialogChangeAge) && app.config.view.Name != "activity" {
			var msg string
			switch d {
			case dialogCancelQuery, dialogTerminateBackend, dialogCancelGroup, dialogTerminateGroup:
				msg = "Terminate or cancel backend is allowed in pg_stat_activity tab."
			case dialogSetMask:
				msg = "State mask setup is allowed in pg_stat_activity tab."
			case dialogChangeAge:
				msg = "Changing queries' min age is allowed in pg_stat_activity tab."
			}
			printCmdline(g, msg)
			return nil
		}

		if d == dialogQueryReport && !strings.Contains(app.config.view.Name, "statements") {
			printCmdline(g, "Query report is allowed in pg_stat_statements tabs.")
			return nil
		}

		maxX, _ := g.Size()
		// Open one-line editable view, print a propmt and set cursor after it.
		if v, err := g.SetView("dialog", len(dialogPrompts[d])-1, 3, maxX-1, 5); err != nil {
			if err != gocui.ErrUnknownView {
				return fmt.Errorf("set dialog view on layout failed: %s", err)
			}

			p, err := g.View("cmdline")
			if err != nil {
				return fmt.Errorf("Set focus on cmdline view failed: %s", err)
			}
			fmt.Fprintf(p, dialogPrompts[d])

			g.Cursor = true
			v.Editable = true
			v.Frame = false

			if _, err := g.SetCurrentView("dialog"); err != nil {
				return fmt.Errorf("set dialog view as current on layout failed: %s", err)
			}

			// Remember the type of an opened dialog. It will be required when the dialog will be finished.
			dialog = d
		}
		return nil
	}
}

// When gocui.KeyEnter is pressed in the end of user's input, depending on dialog type an appropriate handler should be started.
func dialogFinish(app *app) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		var answer string

		printCmdline(g, "")

		switch dialog {
		case dialogPgReload:
			doReload(g, v, app.db, answer)
		case dialogFilter:
			setFilter(g, v, answer, app.config.view)
		case dialogCancelQuery:
			killSingle(g, v, answer, app.db, "cancel")
		case dialogTerminateBackend:
			killSingle(g, v, answer, app.db, "terminate")
		case dialogSetMask:
			setBackendMask(g, v, answer)
		case dialogCancelGroup:
			killGroup(g, v, app, "cancel")
		case dialogTerminateGroup:
			killGroup(g, v, app, "terminate")
		case dialogChangeAge:
			changeQueryAge(g, v, answer, app.config)
		case dialogQueryReport:
			buildQueryReport(g, v, answer, app.db, app.doExit)
		case dialogChangeRefresh:
			changeRefresh(g, v, answer, app.config)
		case dialogNone:
			/* do nothing */
		}

		return dialogClose(g, v)
	}
}

// Finish dialog when user presses Esc to cancel.
func dialogCancel(g *gocui.Gui, v *gocui.View) error {
	dialog = dialogNone
	printCmdline(g, "Do nothing. Operation canceled.")

	return dialogClose(g, v)
}

// Close 'gocui' view object related to dialog.
func dialogClose(g *gocui.Gui, v *gocui.View) error {
	g.Cursor = false
	v.Clear()
	g.DeleteView("dialog")
	if _, err := g.SetCurrentView("sysstat"); err != nil {
		return fmt.Errorf("set sysstat view as current on layout failed: %s", err)
	}
	return nil
}
