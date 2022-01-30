package top

import (
	"context"
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/stat"
	"sync"
	"time"
)

// mainLoop starts application worker and UI loop.
func mainLoop(ctx context.Context, app *app) error {
	var e errorRate

	// Run in infinite loop - if UI crashes then reinitialize it.
	for {
		// Init UI
		g, err := gocui.NewGui(gocui.OutputNormal)
		if err != nil {
			return fmt.Errorf("create UI failed: %s", err)
		}

		app.ui = g

		// Setup UI layout.
		app.ui.SetManagerFunc(layout(app))

		// Setup keybindings.
		if err := keybindings(app); err != nil {
			return err
		}

		var wg sync.WaitGroup
		ctx, cancel := context.WithCancel(ctx)

		// Run backend workers which collect and print stats.
		wg.Add(1)
		go func() {
			doWork(ctx, app)
			wg.Done()
		}()

		// Run UI management loop.
		err = app.ui.MainLoop()
		if err != nil {
			// quit received.
			if err == gocui.ErrQuit {
				cancel()
				return nil
			}

			// remember error, will show it to user in cmdLine
			app.uiError = err

			// check errors rate - quit if them too much (allow no more than 5 errors within 1 second)
			if err := e.check(1*time.Second, 5); err != nil {
				cancel()
				return fmt.Errorf("too many UI errors occurred: %s (%d errors within %.0f seconds)", err, e.errCnt, e.timeElapsed.Seconds())
			}
		}

		// If there are too few errors, just restart worker and UI.
		cancel()

		// Wait until doWork() finish.
		wg.Wait()
	}
}

func doWork(ctx context.Context, app *app) {
	var wg sync.WaitGroup
	statCh := make(chan stat.Stat)

	wg.Add(1)
	go func() {
		collectStat(ctx, app.db, statCh, app.config.viewCh)
		wg.Done()
	}()

	// Send default view and default refresh interval to stats collector goroutine.
	app.config.view.Refresh = time.Second
	app.config.viewCh <- app.config.view

	// Reset refresh interval, it should not be saved as per-view setting.
	app.config.view.Refresh = 0

	for {
		select {
		case <-app.uiExit:
			// used for exit from UI (not the program) in case when need to open $PAGER or $EDITOR programs.
			return
		case s := <-statCh:
			printStat(app, s, app.postgresProps)
		case <-ctx.Done():
			wg.Wait()
			return
		}
	}
}

// layout defines UI layout - set of screen areas and their locations.
func layout(app *app) func(g *gocui.Gui) error {
	return func(g *gocui.Gui) error {
		maxX, maxY := app.ui.Size()

		// Screen dimensions could be equal to zero after executing external programs like pager/editor/psql.
		// Just return empty error and allow UI manager to redraw screen at next loop iteration.
		if maxX == 0 || maxY == 0 {
			return fmt.Errorf("")
		}

		// Sysstat view.
		v, err := app.ui.SetView("sysstat", -1, -1, (maxX-1)/2, 4)
		if err != nil {
			if err != gocui.ErrUnknownView {
				return fmt.Errorf("set sysstat view on layout failed: %s", err)
			}
			if _, err := app.ui.SetCurrentView("sysstat"); err != nil {
				return fmt.Errorf("set sysstat view as current on layout failed: %s", err)
			}
		}
		if v != nil {
			v.Frame = false
		}

		// Postgres activity view.
		v, err = app.ui.SetView("pgstat", maxX/2, -1, maxX, 4)
		if err != nil {
			if err != gocui.ErrUnknownView {
				return fmt.Errorf("set pgstat view on layout failed: %s", err)
			}
		}
		if v != nil {
			v.Frame = false
		}

		// Command line.
		v, err = app.ui.SetView("cmdline", -1, 3, maxX, 5)
		if err != nil {
			if err != gocui.ErrUnknownView {
				return fmt.Errorf("set cmdline view on layout failed: %s", err)
			}
			// show saved error to user if any
			if app.uiError != nil {
				printCmdline(app.ui, "%s", app.uiError)
				app.uiError = nil
			}
		}
		if v != nil {
			v.Frame = false
		}

		// Postgres main stats view.
		v, err = app.ui.SetView("dbstat", -1, 4, maxX, maxY-1)
		if err != nil {
			if err != gocui.ErrUnknownView {
				return fmt.Errorf("set dbstat view on layout failed: %s", err)
			}
		}
		if v != nil {
			v.Frame = false
		}

		// Extra stats view.
		if app.config.view.ShowExtra > stat.CollectNone {
			v, err := app.ui.SetView("extra", -1, 3*maxY/5-1, maxX, maxY-1)
			if err != nil {
				if err != gocui.ErrUnknownView {
					return fmt.Errorf("set extra view on layout failed: %s", err)
				}
				_, err := fmt.Fprintln(v, "")
				if err != nil {
					return fmt.Errorf("print extra stats failed: %s", err)
				}
			}
			if v != nil {
				v.Frame = false
			}
		}

		return nil
	}
}

// printCmdline prints formatted message on cmdline.
func printCmdline(g *gocui.Gui, format string, s ...interface{}) {
	// Do nothing if Gui is not defined.
	if g == nil {
		return
	}

	g.Update(func(g *gocui.Gui) error {
		v, err := g.View("cmdline")
		if err != nil {
			return fmt.Errorf("set focus on cmdline failed: %s", err)
		}
		v.Clear()
		_, err = fmt.Fprintf(v, format, s...)
		if err != nil {
			return fmt.Errorf("print on cmdline failed: %s", err)
		}

		// Clear the message after 2 seconds.
		if format != "" {
			t := time.NewTimer(2 * time.Second)
			go func() {
				<-t.C
				v.Clear()
			}()
		}

		return nil
	})
}
