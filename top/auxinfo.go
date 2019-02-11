// Package top -- auxiliary (aux) stats is not displayed by default and can be enabled if needed.
// Aux stats includes diskstat, nicstat and logtail.
package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/lib/stat"
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
func showAux(auxtype auxType) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		// Close 'view' if passed type of aux stats are already displayed
		if ctx.aux == auxtype {
			closeAuxView(g, v)
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
			nlines, err := stat.CountLines(stat.ProcDiskstats, conn, conninfo.ConnLocal)
			if err != nil {
				printCmdline(g, err.Error())
				closeAuxView(g, nil)
			}
			stats.Iostat.New(nlines)
			ctx.aux = auxtype
			printCmdline(g, "Show diskstats")
			doUpdate <- 1
		case auxNicstat:
			if err := openAuxView(g, v); err != nil {
				return err
			}
			nlines, err := stat.CountLines(stat.ProcNetdevFile, conn, conninfo.ConnLocal)
			if err != nil {
				printCmdline(g, err.Error())
				closeAuxView(g, nil)
			}
			stats.Nicstat.New(nlines)
			ctx.aux = auxtype
			printCmdline(g, "Show nicstat")
			doUpdate <- 1
		case auxLogtail:
			if !conninfo.ConnLocal {
				printCmdline(g, "Log tail is not supported for remote hosts")
				return nil
			}

			pgLog.Size = 0
			pgLog.Path = readLogPath()

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
			ctx.aux = auxtype
			printCmdline(g, "Show logtail")
			doUpdate <- 1
		}

		return nil
	}
}

// Depending on current AUXTYPE read specific stats: Diskstat, Nicstat. Logtail AUXTYPE processed separately.
func getAuxStat(g *gocui.Gui) {
	switch ctx.aux {
	case auxDiskstat:
		ndev, err := stat.CountLines(stat.ProcDiskstats, conn, conninfo.ConnLocal)
		if err != nil {
			printCmdline(g, err.Error())
			closeAuxView(g, nil)
		}
		// If number of devices is changed, re-create stats container
		if ndev != len(stats.CurrDiskstats) {
			stats.Iostat.New(ndev)
		}
		// Read stats
		if err := stats.CurrDiskstats.Read(conn, conninfo.ConnLocal); err != nil {
			printCmdline(g, err.Error())
			closeAuxView(g, nil)
		} else {
			stats.DiffDiskstats.Diff(stats.CurrDiskstats, stats.PrevDiskstats)
			copy(stats.PrevDiskstats, stats.CurrDiskstats)
		}
	case auxNicstat:
		ndev, err := stat.CountLines(stat.ProcNetdevFile, conn, conninfo.ConnLocal)
		if err != nil {
			printCmdline(g, err.Error())
			closeAuxView(g, nil)
		}
		// If number of interfaces is changed, re-create stats container
		if ndev != len(stats.CurrNetdevs) {
			stats.Nicstat.New(ndev)
		}
		// Read stats
		if err := stats.CurrNetdevs.Read(conn, conninfo.ConnLocal); err != nil {
			printCmdline(g, err.Error())
			closeAuxView(g, nil)
		} else {
			stats.DiffNetdevs.Diff(stats.CurrNetdevs, stats.PrevNetdevs)
			copy(stats.PrevNetdevs, stats.CurrNetdevs)
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
func closeAuxView(g *gocui.Gui, _ *gocui.View) error {
	if ctx.aux > auxNone {
		g.DeleteView("aux")
		ctx.aux = auxNone
	}
	return nil
}
