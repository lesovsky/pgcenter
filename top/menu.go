// Menus used in case when user should make a choice from the list of similar items.

package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/lib/stat"
)

// Type of the menu
type menuType int

// Direction of user choice
type direction int

// Particular menu types
const (
	menuNone menuType = iota
	menuPgss
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
	menu menuType
)

// Open 'gocui' view object and display menu items depending on passed menu type.
func menuOpen(m menuStyle) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		g.Cursor = true

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

		/* print menu items */
		for _, item := range m.menuItems {
			fmt.Fprintln(v, item)
		}
		menu = m.menuType

		return nil
	}
}

// When user made a choice, depending on menu type, run appropriate handler
func menuSelect(g *gocui.Gui, v *gocui.View) error {
	_, cy := v.Cursor() /* cy point to an index of the entry, use it to switch to a context */

	switch menu {
	case menuPgss:
		switch cy {
		case 0:
			switchContextToPgss(g, stat.StatementsTimingView)
		case 1:
			switchContextToPgss(g, stat.StatementsGeneralView)
		case 2:
			switchContextToPgss(g, stat.StatementsIOView)
		case 3:
			switchContextToPgss(g, stat.StatementsTempView)
		case 4:
			switchContextToPgss(g, stat.StatementsLocalView)
		default:
			switchContextToPgss(g, stat.StatementsTimingView)
		}
	case menuConf:
		switch cy {
		case 0:
			editPgConfig(g, stat.GucMainConfFile)
		case 1:
			editPgConfig(g, stat.GucHbaFile)
		case 2:
			editPgConfig(g, stat.GucIdentFile)
		case 3:
			editPgConfig(g, stat.GucRecoveryFile)
		}
	case menuNone:
		/* do nothing */
	}

	return menuClose(g, v)
}

// Close 'gocui' view object when menu is closed
func menuClose(g *gocui.Gui, v *gocui.View) error {
	g.Cursor = false

	if err := g.DeleteView("menu"); err != nil {
		return err
	}

	if _, err := g.SetCurrentView("sysstat"); err != nil {
		return err
	}
	return nil
}

// Move cursor to one item up or down.
func moveCursor(d direction) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		if v != nil {
			cx, cy := v.Cursor()
			switch d {
			case moveDown:
				v.SetCursor(cx, cy+1) /* errors don't make sense here */
			case moveUp:
				v.SetCursor(cx, cy-1) /* errors don't make sense here */
			}
		}
		return nil
	}
}
