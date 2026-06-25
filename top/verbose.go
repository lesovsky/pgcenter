package top

import (
	"github.com/jroimartin/gocui"
)

// toggleVerbose flips the verbose display mode for the top summary panels. It mirrors the
// showExtra write-into-all-views pattern (top/extra.go): the new flag is written onto every
// entry in config.views (so it survives a screen switch), onto the active config.view, and onto
// config.verbose, then the updated view is pushed on viewCh to notify the stats goroutine.
//
// Unlike showExtra there is no separate gocui view to open or close — verbose is rendered into
// the existing sysstat/pgstat panels (handled in later tasks), so no openExtraView/SetView call.
func toggleVerbose(app *app) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		newVal := !app.config.verbose

		// Mirror the flag into every view so it persists across screen switches.
		for k, v := range app.config.views {
			v.Verbose = newVal
			app.config.views[k] = v
		}
		app.config.view.Verbose = newVal
		app.config.verbose = newVal

		app.config.viewCh <- app.config.view

		if newVal {
			printCmdline(g, "Verbose mode: on")
		} else {
			printCmdline(g, "Verbose mode: off")
		}

		return nil
	}
}
