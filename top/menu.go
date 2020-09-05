// Menus used in case when user should make a choice from the list of similar items.

package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/stat"
)

// Type of the menu
type menuType int

// Direction of user choice
type direction int

// Particular menu types
const (
	menuNone menuType = iota
	menuPgss
	menuProgress
	menuConf
)

// Directions - user allowed to move up and down.
const (
	moveUp direction = iota
	moveDown
)

// Describes menu and its details
type menuStyle struct {
	menuType           // Type of a menu
	menuTitle string   // Title
	menuItems []string // List of items
}

var (
	// pg_stat_statements menu
	menuPgssStyle = menuStyle{
		menuType:  menuPgss,
		menuTitle: " Choose pg_stat_statements mode (Enter to choose, Esc to exit): ",
		menuItems: []string{
			" pg_stat_statements timings",
			" pg_stat_statements general",
			" pg_stat_statements input/output",
			" pg_stat_statements temp files input/output",
			" pg_stat_statements temp tables (local) input/output",
		},
	}

	// pg_stat_progress_* menu
	menuProgressStyle = menuStyle{
		menuType:  menuProgress,
		menuTitle: " Choose pg_stat_progress_* view (Enter to choose, Esc to exit): ",
		menuItems: []string{
			" pg_stat_progress_vacuum",
			" pg_stat_progress_cluster",
			" pg_stat_progress_create_index",
		},
	}

	// edit configuration files
	menuConfStyle = menuStyle{
		menuType:  menuConf,
		menuTitle: " Edit configuration file (Enter to edit, Esc to exit): ",
		menuItems: []string{
			" postgresql.conf",
			" pg_hba.conf",
			" pg_ident.conf",
			" recovery.conf",
		},
	}

	// Variable-transporter, function which check user's choice, uses this variable to select appropriate handler. Depending on menu type, select appropriate function.
	menu  menuType
	items []string
)

// Open 'gocui' view object and display menu items depending on passed menu type.
func menuOpen(m menuStyle, pgssAvail bool) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		// in case of opening menu for switching to pg_stat_statements and if pgss isn't available - it's unnecessary to open menu, just notify user and do nothing
		if !pgssAvail && m.menuType == menuPgss {
			printCmdline(g, msgPgStatStatementsUnavailable)
			return nil
		}

		v, err := g.SetView("menu", 0, 5, 72, 6+len(m.menuItems))
		if err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.Title = m.menuTitle
		}
		if _, err := g.SetCurrentView("menu"); err != nil {
			return err
		}

		menu = m.menuType
		items = m.menuItems

		menuDraw(v)

		return nil
	}
}

// When user made a choice, depending on menu type, run appropriate handler
func menuSelect(app *app) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		_, cy := v.Cursor() /* cy point to an index of the entry, use it to switch to a context */

		switch menu {
		case menuPgss:
			switch cy {
			case 0:
				switchContextToPgss(app, "statements_timings")
			case 1:
				switchContextToPgss(app, "statements_general")
			case 2:
				switchContextToPgss(app, "statements_io")
			case 3:
				switchContextToPgss(app, "statements_temp")
			case 4:
				switchContextToPgss(app, "statements_local")
			default:
				switchContextToPgss(app, "statements_timings")
			}
		case menuProgress:
			switch cy {
			case 0:
				switchContextToProgress(app, "progress_vacuum")
			case 1:
				switchContextToProgress(app, "progress_cluster")
			case 2:
				switchContextToProgress(app, "progress_index")
			}
		case menuConf:
			switch cy {
			case 0:
				editPgConfig(g, app.db, stat.GucMainConfFile, app.doExit)
			case 1:
				editPgConfig(g, app.db, stat.GucHbaFile, app.doExit)
			case 2:
				editPgConfig(g, app.db, stat.GucIdentFile, app.doExit)
			case 3:
				editPgConfig(g, app.db, stat.GucRecoveryFile, app.doExit)
			}
		case menuNone:
			/* do nothing */
		}

		return menuClose(g, v)
	}
}

// Close 'gocui' view object when menu is closed
func menuClose(g *gocui.Gui, v *gocui.View) error {
	if err := g.DeleteView("menu"); err != nil {
		return err
	}

	if _, err := g.SetCurrentView("sysstat"); err != nil {
		return err
	}
	return nil
}

func menuDraw(v *gocui.View) {
	_, cy := v.Cursor()
	v.Clear()
	/* print menu items */
	for i, item := range items {
		if i == cy {
			fmt.Fprintln(v, "\033[30;47m"+item+"\033[0m")
		} else {
			fmt.Fprintln(v, item)
		}
	}
}

// Move cursor to one item up or down.
func moveCursor(d direction) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		if v != nil {
			cx, cy := v.Cursor()
			switch d {
			case moveDown:
				v.SetCursor(cx, cy+1) /* errors don't make sense here */
				menuDraw(v)
			case moveUp:
				v.SetCursor(cx, cy-1) /* errors don't make sense here */
				menuDraw(v)
			}
		}
		return nil
	}
}
