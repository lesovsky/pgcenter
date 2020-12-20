// Define key bindings.

package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/stat"
	"os"
	"strconv"
	"strings"
	"time"
)

// Key represents particular key, a view where it should work and associated function.
type key struct {
	viewname string
	key      interface{}
	handler  func(g *gocui.Gui, v *gocui.View) error
}

// Setup key bindings and handlers.
func keybindings(app *app) error {
	var keys = []key{
		{"", gocui.KeyCtrlC, quit(app)},
		{"", gocui.KeyCtrlQ, quit(app)},
		{"sysstat", 'q', quit(app)},
		{"sysstat", gocui.KeyArrowLeft, orderKeyLeft(app.config)},
		{"sysstat", gocui.KeyArrowRight, orderKeyRight(app.config)},
		{"sysstat", gocui.KeyArrowUp, increaseWidth(app.config)},
		{"sysstat", gocui.KeyArrowDown, decreaseWidth(app.config)},
		{"sysstat", '<', switchSortOrder(app.config)},
		{"sysstat", ',', toggleSysTables(app.config)},
		{"sysstat", 'I', toggleIdleConns(app.config)},
		{"sysstat", 'd', switchViewTo(app, "databases")},
		{"sysstat", 'r', switchViewTo(app, "replication")},
		{"sysstat", 't', switchViewTo(app, "tables")},
		{"sysstat", 'i', switchViewTo(app, "indexes")},
		{"sysstat", 's', switchViewTo(app, "sizes")},
		{"sysstat", 'f', switchViewTo(app, "functions")},
		{"sysstat", 'p', switchViewTo(app, "progress")},
		{"sysstat", 'a', switchViewTo(app, "activity")},
		{"sysstat", 'x', switchViewTo(app, "statements")},
		{"sysstat", 'Q', resetStat(app.db)},
		{"sysstat", 'E', menuOpen(menuConfStyle, false)},
		{"sysstat", 'X', menuOpen(menuPgssStyle, app.postgresProps.ExtPGSSAvail)},
		{"sysstat", 'P', menuOpen(menuProgressStyle, false)},
		{"sysstat", 'l', showPgLog(app.db, app.postgresProps.VersionNum, app.doExit)},
		{"sysstat", 'C', showPgConfig(app.db, app.doExit)},
		{"sysstat", '~', runPsql(app.db, app.doExit)},
		{"sysstat", 'B', showExtra(app, stat.CollectDiskstats)},
		{"sysstat", 'N', showExtra(app, stat.CollectNetdev)},
		{"sysstat", 'L', showExtra(app, stat.CollectLogtail)},
		{"sysstat", 'R', dialogOpen(app, dialogPgReload)},
		{"sysstat", '/', dialogOpen(app, dialogFilter)},
		{"sysstat", '-', dialogOpen(app, dialogCancelQuery)},
		{"sysstat", '_', dialogOpen(app, dialogTerminateBackend)},
		{"sysstat", 'n', dialogOpen(app, dialogSetMask)},
		{"sysstat", 'm', showBackendMask},
		{"sysstat", 'k', dialogOpen(app, dialogCancelGroup)},
		{"sysstat", 'K', dialogOpen(app, dialogTerminateGroup)},
		{"sysstat", 'A', dialogOpen(app, dialogChangeAge)},
		{"sysstat", 'G', dialogOpen(app, dialogQueryReport)},
		{"sysstat", 'z', dialogOpen(app, dialogChangeRefresh)},
		{"dialog", gocui.KeyEsc, dialogCancel(app)},
		{"dialog", gocui.KeyEnter, dialogFinish(app)},
		{"menu", gocui.KeyEsc, menuClose},
		{"menu", gocui.KeyArrowUp, moveCursor(moveUp)},
		{"menu", gocui.KeyArrowDown, moveCursor(moveDown)},
		{"menu", gocui.KeyEnter, menuSelect(app)},
		{"sysstat", 'h', showHelp},
		{"sysstat", gocui.KeyF1, showHelp},
		{"help", gocui.KeyEsc, closeHelp},
		{"help", 'q', closeHelp},
	}

	app.ui.InputEsc = true

	for _, k := range keys {
		if err := app.ui.SetKeybinding(k.viewname, k.key, gocui.ModNone, k.handler); err != nil {
			return fmt.Errorf("ERROR: failed to setup keybindings: %s", err)
		}
	}

	return nil
}

// Change interval of stats refreshing.
func changeRefresh(g *gocui.Gui, v *gocui.View, answer string, config *config) {
	answer = strings.TrimPrefix(v.Buffer(), dialogPrompts[dialogChangeRefresh])
	answer = strings.TrimSuffix(answer, "\n")

	if answer == "" {
		printCmdline(g, "Do nothing. Empty input.")
		return
	}

	interval, _ := strconv.Atoi(answer)

	switch {
	case interval < 1:
		printCmdline(g, "Should not be less than 1 second.")
		return
	case interval > 300:
		printCmdline(g, "Should not be more than 300 seconds.")
		return
	}

	// Set refresh interval, send it to stats channel and reset interval in the view.
	// Refresh interval should not be saved as a per-view setting. It's used as a setting for stats goroutine.
	config.view.Refresh = time.Duration(interval) * time.Second
	config.viewCh <- config.view
	config.view.Refresh = 0
}

// Quit program.
func quit(app *app) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		close(app.doUpdate)
		close(app.doExit)
		g.Close()

		app.db.Close()

		os.Exit(0) // TODO: this is a very dirty hack
		return gocui.ErrQuit
	}
}
