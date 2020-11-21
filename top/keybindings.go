// Define key bindings.

package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
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
		{"sysstat", gocui.KeyArrowUp, changeWidth(app.config, colsWidthIncr)},
		{"sysstat", gocui.KeyArrowDown, changeWidth(app.config, colsWidthDecr)},
		{"sysstat", '<', switchSortOrder(app.config)},
		{"sysstat", ',', toggleSysTables(app.config, app.doUpdate)},
		{"sysstat", 'I', toggleIdleConns(app.config)},
		{"sysstat", 'd', switchContextTo(app, "databases")},
		{"sysstat", 'r', switchContextTo(app, "replication")},
		{"sysstat", 't', switchContextTo(app, "tables")},
		{"sysstat", 'i', switchContextTo(app, "indexes")},
		{"sysstat", 's', switchContextTo(app, "sizes")},
		{"sysstat", 'f', switchContextTo(app, "functions")},
		{"sysstat", 'p', switchContextTo(app, "progress")},
		{"sysstat", 'a', switchContextTo(app, "activity")},
		{"sysstat", 'x', switchContextTo(app, "statements")},
		{"sysstat", 'Q', resetStat(app.db)},
		{"sysstat", 'E', menuOpen(menuConfStyle, false)},
		{"sysstat", 'X', menuOpen(menuPgssStyle, app.postgresProps.ExtPGSSAvail)},
		{"sysstat", 'P', menuOpen(menuProgressStyle, false)},
		{"sysstat", 'l', showPgLog(app.db, app.doExit)},
		{"sysstat", 'C', showPgConfig(app.db, app.doExit)},
		{"sysstat", '~', runPsql(app.db, app.doExit)},
		{"sysstat", 'B', showAux(app, auxDiskstat)},
		{"sysstat", 'N', showAux(app, auxNicstat)},
		{"sysstat", 'L', showAux(app, auxLogtail)},
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
		{"dialog", gocui.KeyEsc, dialogCancel},
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
func changeRefresh(g *gocui.Gui, v *gocui.View, answer string, config *config, doUpdate chan int) {
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

	config.refreshInterval = time.Duration(interval) * config.minRefresh
	doUpdate <- 1
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
