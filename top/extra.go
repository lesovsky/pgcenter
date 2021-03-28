package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/stat"
	"os"
)

// showExtra manages displaying extra stats - depending on user selection it opens or closes dedicated 'view' for extra stats.
func showExtra(app *app, extra int) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		// Close 'view' if passed type of extra stats are already displayed
		if app.config.view.ShowExtra == extra {
			if extra == stat.CollectLogtail {
				err := app.config.logtail.Close()
				if err != nil {
					return err
				}
			}

			return closeExtraView(g, v, app.config)
		}

		var msg string

		// Depending on requested extra stats, additional steps might to be necessary.
		switch extra {
		case stat.CollectDiskstats:
			msg = "Show block devices statistics"
		case stat.CollectNetdev:
			msg = "Show network interfaces statistics"
		case stat.CollectFsstats:
			msg = "Show mounted filesystems statistics"
		case stat.CollectLogtail:
			if !app.db.Local {
				printCmdline(g, "Log tail is not supported for remote hosts")
				return nil
			}

			logfile, err := stat.GetPostgresCurrentLogfile(app.db, app.postgresProps.VersionNum)
			if err != nil {
				return err
			}
			app.config.logtail.Path = logfile
			app.config.logtail.Size = 0

			// Check the logfile exists, is not empty and available for reading.
			if info, err := os.Stat(app.config.logtail.Path); err == nil && info.Size() == 0 {
				printCmdline(g, "Empty logfile")
				return nil
			} else if err != nil {
				printCmdline(g, "Failed to stat logfile: %s", err)
				return nil
			}
			if err := app.config.logtail.Open(); err != nil {
				printCmdline(g, "Failed to open %s", app.config.logtail.Path)
				return nil
			}

			msg = "Tail Postgres log"
		}

		// If other type of extra stats already displayed, ignore it and reopen 'view' for requested extra stats.
		if err := openExtraView(g, v); err != nil {
			return err
		}

		// Update views configuration and notify stats goroutine - it have to start collecting extra stats.
		for k, v := range app.config.views {
			v.ShowExtra = extra
			app.config.views[k] = v
		}
		app.config.view.ShowExtra = extra
		app.config.viewCh <- app.config.view

		printCmdline(g, msg)

		return nil
	}
}

// openExtraView create new UI view object for displaying extra stats.
func openExtraView(g *gocui.Gui, _ *gocui.View) error {
	maxX, maxY := g.Size()
	v, err := g.SetView("extra", -1, 3*maxY/5-1, maxX-1, maxY-1)
	if err != nil {
		// gocui.ErrUnknownView is OK, it means a new view has been created.
		if err != gocui.ErrUnknownView {
			return fmt.Errorf("set extra view on layout failed: %s", err)
		}
	}
	v.Frame = false
	return nil
}

// closeExtraView updates configuration and closes view with extra stats.
func closeExtraView(g *gocui.Gui, _ *gocui.View, c *config) error {
	for k, v := range c.views {
		v.ShowExtra = stat.CollectNone
		c.views[k] = v
	}
	c.view.ShowExtra = stat.CollectNone
	c.viewCh <- c.view

	return g.DeleteView("extra")
}
