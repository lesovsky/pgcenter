package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/stat"
)

// Key represents binding between key button and handler should be running when user presses the button.
type key struct {
	viewname string
	key      interface{}
	handler  func(g *gocui.Gui, v *gocui.View) error
}

// keybindings set up key bindings with handlers.
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
		{"sysstat", 'E', menuOpen(menuConf, app.config, false)},
		{"sysstat", 'X', menuOpen(menuPgss, app.config, app.postgresProps.ExtPGSSAvail)},
		{"sysstat", 'P', menuOpen(menuProgress, app.config, false)},
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
		{"sysstat", 'm', showProcMask(app.config.procMask)},
		{"sysstat", 'k', dialogOpen(app, dialogCancelGroup)},
		{"sysstat", 'K', dialogOpen(app, dialogTerminateGroup)},
		{"sysstat", 'A', dialogOpen(app, dialogChangeAge)},
		{"sysstat", 'G', dialogOpen(app, dialogQueryReport)},
		{"sysstat", 'z', dialogOpen(app, dialogChangeRefresh)},
		{"dialog", gocui.KeyEsc, dialogCancel(app)},
		{"dialog", gocui.KeyEnter, dialogFinish(app)},
		{"menu", gocui.KeyEsc, menuClose},
		{"menu", gocui.KeyArrowUp, moveCursor(moveUp, app.config)},
		{"menu", gocui.KeyArrowDown, moveCursor(moveDown, app.config)},
		{"menu", gocui.KeyEnter, menuSelect(app)},
		{"sysstat", 'h', showHelp},
		{"sysstat", gocui.KeyF1, showHelp},
		{"help", gocui.KeyEsc, closeHelp},
		{"help", 'q', closeHelp},
	}

	app.ui.InputEsc = true

	for _, k := range keys {
		if err := app.ui.SetKeybinding(k.viewname, k.key, gocui.ModNone, k.handler); err != nil {
			return fmt.Errorf("setup keybindings failed: %s", err)
		}
	}

	return nil
}
