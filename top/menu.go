package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
)

// menuType defines a type of the used menu.
type menuType int

// direction defines direction of user choice in the used menu.
type direction int

const (
	// Available menu types.
	menuNone     menuType = iota // no active menu
	menuPgss                     // menu with pg_stat_statements stats
	menuProgress                 // menu with pg_stat_progress_* stats
	menuConf                     // menu with configuration files

	// Directions allowed when working with menu.
	moveUp   direction = iota // move up
	moveDown                  // move down
)

// menuStyle describes menu properties.
type menuStyle struct {
	menuType          // Type of a menu
	title    string   // Title
	items    []string // List of items
}

// selectMenuStyle returns selected menuStyle properties.
func selectMenuStyle(t menuType) menuStyle {
	var s menuStyle

	switch t {
	case menuPgss:
		s = menuStyle{
			menuType: menuPgss,
			title:    " Choose pg_stat_statements mode (Enter to choose, Esc to exit): ",
			items: []string{
				" pg_stat_statements timings",
				" pg_stat_statements general",
				" pg_stat_statements input/output",
				" pg_stat_statements temp files input/output",
				" pg_stat_statements temp tables (local) input/output",
			},
		}
	case menuProgress:
		s = menuStyle{
			menuType: menuProgress,
			title:    " Choose pg_stat_progress_* view (Enter to choose, Esc to exit): ",
			items: []string{
				" pg_stat_progress_vacuum",
				" pg_stat_progress_cluster",
				" pg_stat_progress_create_index",
			},
		}
	case menuConf:
		s = menuStyle{
			menuType: menuConf,
			title:    " Edit configuration file (Enter to edit, Esc to exit): ",
			items: []string{
				" postgresql.conf",
				" pg_hba.conf",
				" pg_ident.conf",
				" recovery.conf",
			},
		}
	default:
		s = menuStyle{
			menuType: menuNone,
		}
	}

	return s
}

// menuOpen opens UI view object for menu.
func menuOpen(m menuType, config *config, pgssAvail bool) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		s := selectMenuStyle(m)

		// in case of opening menu for switching to pg_stat_statements and if it isn't available - it's unnecessary to open menu, just notify user and do nothing
		if !pgssAvail && s.menuType == menuPgss {
			printCmdline(g, msgPgStatStatementsUnavailable)
			return nil
		}

		v, err := g.SetView("menu", 0, 5, 72, 6+len(s.items))
		if err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
			v.Title = s.title
		}
		if _, err := g.SetCurrentView("menu"); err != nil {
			return err
		}

		menuDraw(v, s.items)

		// Save menu properties in config.
		config.menu = s

		return nil
	}
}

// When user made a choice, depending on menu type, run appropriate handler
func menuSelect(app *app) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		_, cy := v.Cursor() /* cy point to an index of the entry, use it to switch to a context */

		switch app.config.menu.menuType {
		case menuPgss:
			switch cy {
			case 0:
				viewSwitchHandler(app.config, "statements_timings")
			case 1:
				viewSwitchHandler(app.config, "statements_general")
			case 2:
				viewSwitchHandler(app.config, "statements_io")
			case 3:
				viewSwitchHandler(app.config, "statements_temp")
			case 4:
				viewSwitchHandler(app.config, "statements_local")
			default:
				viewSwitchHandler(app.config, "statements_timings")
			}
			printCmdline(app.ui, app.config.view.Msg)
		case menuProgress:
			switch cy {
			case 0:
				viewSwitchHandler(app.config, "progress_vacuum")
			case 1:
				viewSwitchHandler(app.config, "progress_cluster")
			case 2:
				viewSwitchHandler(app.config, "progress_index")
			}
			printCmdline(app.ui, app.config.view.Msg)
		case menuConf:
			switch cy {
			case 0:
				editPgConfig(g, app.db, gucMainConfFile, app.doExit)
			case 1:
				editPgConfig(g, app.db, gucHbaFile, app.doExit)
			case 2:
				editPgConfig(g, app.db, gucIdentFile, app.doExit)
			case 3:
				editPgConfig(g, app.db, gucRecoveryFile, app.doExit)
			}
		case menuNone:
			/* do nothing */
		}

		// When menu item has been selected, close menu and reset menu properties from config.
		app.config.menu = selectMenuStyle(menuNone)
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

func menuDraw(v *gocui.View, items []string) {
	_, cy := v.Cursor()
	v.Clear()
	// print menu items
	for i, item := range items {
		if i == cy {
			fmt.Fprintln(v, "\033[30;47m"+item+"\033[0m")
		} else {
			fmt.Fprintln(v, item)
		}
	}
}

// Move cursor to one item up or down.
func moveCursor(d direction, config *config) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		if v != nil {
			cx, cy := v.Cursor()
			switch d {
			case moveDown:
				v.SetCursor(cx, cy+1) /* errors don't make sense here */
				menuDraw(v, config.menu.items)
			case moveUp:
				v.SetCursor(cx, cy-1) /* errors don't make sense here */
				menuDraw(v, config.menu.items)
			}
		}
		return nil
	}
}
