// Package top -- auxiliary (aux) stats is not displayed by default and can be enabled if needed.
// Aux stats includes diskstat, nicstat and logtail.
package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/stat"
	"os"
)

type auxType int

const (
	auxNone auxType = iota
	auxDiskstat
	auxNicstat
	auxLogtail
)

// Prepares aux stat - open or close dedicated 'view' in which stats are displayed, create stats containers or open log.
func showAux(app *app, auxtype auxType) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		// Close 'view' if passed type of aux stats are already displayed
		if app.config.aux == auxtype {
			closeAuxView(g, v, app.config)
			return nil
		}

		// If other type of aux stats are already displayed, ignore it and open 'view' for new aux stats. For diskstat/nicstat
		// get number of devices and create appropriate storages. For logtail, a logfile have to be opened. In the end,
		// set passed 'auxtype' in the context and aux stats can be displayed in the statsLoop().
		switch auxtype {
		case auxDiskstat:
			if err := openAuxView(g, v); err != nil {
				return err
			}
			nlines, err := stat.CountDevices(stat.ProcDiskstats, app.db, app.postgresProps.SchemaPgcenterAvail)
			if err != nil {
				printCmdline(g, err.Error())
				closeAuxView(g, nil, app.config)
			}
			app.stats.Iostat.New(nlines)
			app.config.aux = auxtype
			printCmdline(g, "Show diskstats")
			app.doUpdate <- 1
		case auxNicstat:
			if err := openAuxView(g, v); err != nil {
				return err
			}
			nlines, err := stat.CountDevices(stat.ProcNetdevFile, app.db, app.postgresProps.SchemaPgcenterAvail)
			if err != nil {
				printCmdline(g, err.Error())
				closeAuxView(g, nil, app.config)
			}
			app.stats.Nicstat.New(nlines)
			app.config.aux = auxtype
			printCmdline(g, "Show nicstat")
			app.doUpdate <- 1
		case auxLogtail:
			if !app.db.Local {
				printCmdline(g, "Log tail is not supported for remote hosts")
				return nil
			}

			pgLog.Size = 0
			pgLog.Path = readLogPath(app.db)

			// Check the logfile isn't an empty
			if info, err := os.Stat(pgLog.Path); err == nil && info.Size() == 0 {
				printCmdline(g, "Empty logfile")
				return nil
			} else if err != nil {
				printCmdline(g, "Failed to stat logfile: %s", err)
				return nil
			}

			if err := pgLog.Open(); err != nil {
				printCmdline(g, "Failed to open %s", pgLog.Path)
				return nil
			}

			if err := openAuxView(g, v); err != nil {
				return err
			}
			app.config.aux = auxtype
			printCmdline(g, "Show logtail")
			app.doUpdate <- 1
		}

		return nil
	}
}

// Depending on current AUXTYPE read specific stats: Diskstat, Nicstat. Logtail AUXTYPE processed separately.
func getAuxStat(app *app) {
	switch app.config.aux {
	case auxDiskstat:
		ndev, err := stat.CountDevices(stat.ProcDiskstats, app.db, app.postgresProps.SchemaPgcenterAvail)
		if err != nil {
			printCmdline(app.ui, err.Error())
			closeAuxView(app.ui, nil, app.config)
		}
		// If number of devices is changed, re-create stats container
		if ndev != len(app.stats.CurrDiskstats) {
			app.stats.Iostat.New(ndev)
		}
		// Read stats
		if err := app.stats.CurrDiskstats.Read(app.db, app.postgresProps.SchemaPgcenterAvail); err != nil {
			printCmdline(app.ui, err.Error())
			closeAuxView(app.ui, nil, app.config)
		} else {
			app.stats.DiffDiskstats.Diff(app.stats.CurrDiskstats, app.stats.PrevDiskstats)
			copy(app.stats.PrevDiskstats, app.stats.CurrDiskstats)
		}
	case auxNicstat:
		ndev, err := stat.CountDevices(stat.ProcNetdevFile, app.db, app.postgresProps.SchemaPgcenterAvail)
		if err != nil {
			printCmdline(app.ui, err.Error())
			closeAuxView(app.ui, nil, app.config)
		}
		// If number of interfaces is changed, re-create stats container
		if ndev != len(app.stats.CurrNetdevs) {
			app.stats.Nicstat.New(ndev)
		}
		// Read stats
		if err := app.stats.CurrNetdevs.Read(app.db, app.postgresProps.SchemaPgcenterAvail); err != nil {
			printCmdline(app.ui, err.Error())
			closeAuxView(app.ui, nil, app.config)
		} else {
			app.stats.DiffNetdevs.Diff(app.stats.CurrNetdevs, app.stats.PrevNetdevs)
			copy(app.stats.PrevNetdevs, app.stats.CurrNetdevs)
		}
	case auxNone:
		// do nothing
	}
}

// Open 'gocui' object for aux stats
func openAuxView(g *gocui.Gui, _ *gocui.View) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("aux", -1, 3*maxY/5-1, maxX-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return fmt.Errorf("set aux view on layout failed: %s", err)
		}
		v.Frame = false
	}
	return nil
}

// Close 'gocui' object
func closeAuxView(g *gocui.Gui, _ *gocui.View, c *config) error {
	if c.aux > auxNone {
		g.DeleteView("aux")
		c.aux = auxNone
	}
	return nil
}
