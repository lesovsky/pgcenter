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
	// dialogPrompts defines prompts for user-requested actions.
	// This is read-only and should not be changed during runtime.
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
)

// dialogOpen opens view for the dialog.
func dialogOpen(app *app, d dialogType) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
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
		v, err := g.SetView("dialog", len(dialogPrompts[d])-1, 3, maxX-1, 5)
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

		_, err = fmt.Fprintf(p, dialogPrompts[d])
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
		var answer string

		printCmdline(g, "")

		// TODO: refactor functions to return value and not use gocui object inside
		//   Most of the should return error/success response which should be printed to user.

		switch app.config.dialog {
		case dialogPgReload:
			_ = doReload(g, v.Buffer(), app.db)
		case dialogFilter:
			setFilter(g, v.Buffer(), app.config.view)
		case dialogCancelQuery:
			_ = killSingle(app.db, "cancel", v.Buffer())
		case dialogTerminateBackend:
			_ = killSingle(app.db, "terminate", v.Buffer())
		case dialogSetMask:
			setProcMask(g, v.Buffer(), app.config)
		case dialogCancelGroup:
			_, _ = killGroup(app, "cancel")
		case dialogTerminateGroup:
			_, _ = killGroup(app, "terminate")
		case dialogChangeAge:
			changeQueryAge(g, v.Buffer(), app.config)
		case dialogQueryReport:
			_ = buildQueryReport(g, answer, app.db, app.doExit)
		case dialogChangeRefresh:
			changeRefresh(g, v.Buffer(), app.config)
		case dialogNone:
			/* do nothing */
		}

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
