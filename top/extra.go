package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/stat"
)

// Prepares extra stat - open or close dedicated 'view' which shows extra stats depending on user selection.
func showExtra(app *app, extra int) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		// Close 'view' if passed type of aux stats are already displayed
		if app.config.view.ShowExtra == extra {
			return closeExtraView(g, v, app.config)
		}

		// If other type of aux stats are already displayed, ignore it and open 'view' for new aux stats. For diskstat/nicstat
		// get number of devices and create appropriate storages. For logtail, a logfile have to be opened. In the end,
		// set passed 'auxtype' in the context and aux stats can be displayed in the statsLoop().
		switch extra {
		case stat.CollectDiskstats:
			if err := openExtraView(g, v); err != nil {
				return err
			}
			for _, v := range app.config.views {
				v.ShowExtra = stat.CollectDiskstats
			}
			app.config.view.ShowExtra = stat.CollectDiskstats
			app.config.viewCh <- app.config.view

			printCmdline(g, "Show diskstats")
		case stat.CollectNetdev:
			if err := openExtraView(g, v); err != nil {
				return err
			}
			for _, v := range app.config.views {
				v.ShowExtra = stat.CollectNetdev
			}
			app.config.view.ShowExtra = stat.CollectNetdev
			app.config.viewCh <- app.config.view

			printCmdline(g, "Show netdev")
			//case auxLogtail:
			//	if !app.db.Local {
			//		printCmdline(g, "Log tail is not supported for remote hosts")
			//		return nil
			//	}
			//
			//	pgLog.Size = 0
			//	pgLog.Path = readLogPath(app.db)
			//
			//	// Check the logfile isn't an empty
			//	if info, err := os.Stat(pgLog.Path); err == nil && info.Size() == 0 {
			//		printCmdline(g, "Empty logfile")
			//		return nil
			//	} else if err != nil {
			//		printCmdline(g, "Failed to stat logfile: %s", err)
			//		return nil
			//	}
			//
			//	if err := pgLog.Open(); err != nil {
			//		printCmdline(g, "Failed to open %s", pgLog.Path)
			//		return nil
			//	}
			//
			//	if err := openAuxView(g, v); err != nil {
			//		return err
			//	}
			//	app.config.aux = auxtype
			//	printCmdline(g, "Show logtail")
			//	app.doUpdate <- 1
		}

		return nil
	}
}

// openExtraView opens extra 'gocui' view for displaying extra stats.
func openExtraView(g *gocui.Gui, _ *gocui.View) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("aux", -1, 3*maxY/5-1, maxX-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return fmt.Errorf("set aux view on layout failed: %s", err)
		}
		v.Frame = false
	}
	return nil
}

// closeExtraView closes extra 'gocui' view.
func closeExtraView(g *gocui.Gui, _ *gocui.View, c *config) error {
	for _, v := range c.views {
		v.ShowExtra = stat.CollectNone
	}
	c.view.ShowExtra = stat.CollectNone
	c.viewCh <- c.view
	g.DeleteView("aux")

	return nil
}
