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

			// The panel really is closing now - below the logtail Close() error return, which
			// leaves the panel open and therefore changes nothing. This is the one lifting path in
			// the feature on which NOBODY writes the cmdline: not closeExtraView, not layout, not
			// printStat. Hence the refreshing variant - a silent lift here would strand [PAUSED]
			// over live data.
			liftPauseRefresh(g, app.config)

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

			// Build the log file locally and commit it into the configuration only once it is
			// really open. It is one local VALUE rather than separate path/size variables because
			// Logfile.Open reads Path off its own receiver and writes File back into it
			// (internal/stat/log.go:22-30), so the path, the zeroed size and the descriptor have to
			// be committed together. That is what makes the three early returns below true no-ops:
			// an 'L' that failed leaves no trace in app.config at all. Committing first and rolling
			// back on error would reintroduce exactly the mutation this avoids.
			logtail := stat.Logfile{Path: logfile}

			// Check the logfile exists, is not empty and available for reading.
			if info, err := os.Stat(logtail.Path); err == nil && info.Size() == 0 {
				printCmdline(g, "Empty logfile")
				return nil
			} else if err != nil {
				printCmdline(g, "Failed to stat logfile: %s", err)
				return nil
			}
			if err := logtail.Open(); err != nil {
				printCmdline(g, "Failed to open %s", logtail.Path)
				return nil
			}

			// The descriptor is now owned by app.config.logtail. The local copy must NOT be closed:
			// both values hold the same *os.File.
			app.config.logtail = logtail

			msg = "Tail Postgres log"
		}

		// If other type of extra stats already displayed, ignore it and reopen 'view' for requested extra stats.
		if err := openExtraView(g, v); err != nil {
			return err
		}

		// The panel is open, so the extra statistics behind it are now the collector's job. This is
		// the only spot that lifts on the opening path: it is below every early return of the
		// logtail branch above (a remote host, an empty log, a log that could not be stat'ed or
		// opened - all no-ops that must leave the freeze intact) and below openExtraView's own error
		// return, and it is OUTSIDE the switch, so B/N/F reach it too. Silent variant: the write at
		// the end of this handler repaints the prefix.
		liftPause(app.config)

		// Update views configuration and notify stats goroutine - it have to start collecting extra stats.
		for k, v := range app.config.views {
			v.ShowExtra = extra
			app.config.views[k] = v
		}
		app.config.view.ShowExtra = extra
		app.config.viewCh <- app.config.view

		printCmdline(g, "%s", msg)

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
			return fmt.Errorf("set extra view on layout failed: %w", err)
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
