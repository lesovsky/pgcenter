package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/version"
)

const (
	helpTemplate = `%s: Help for interactive commands

general actions:
    a,f,r,w     mode: 'a' activity, 'f' functions, 'r' replication, 'w' WAL,
    s,t,i             's' tables sizes, 't' tables, 'i' indexes.
    d,D               'd' pg_stat_database switch, 'X' pg_stat_database menu.
    x,X               'x' pg_stat_statements switch, 'X' pg_stat_statements menu.
    p,P               'p' pg_stat_progress_* switch, 'P' pg_stat_progress_* menu.
    Left,Right,<,/    'Left,Right' change column sort, '<' desc/asc sort toggle, '/' set filter.
    Up,Down           'Up' increase column width, 'Down' decrease column width.
    C,E,R       config: 'C' show config, 'E' edit configs, 'R' reload config.
    ~                 start psql session.
    l                 open log file with pager.

extra stats actions:
    B,N,F,L       'B' diskstat, 'N' nicstat, 'F' filesystems, L' logtail.

activity actions:
    -,_         '-' cancel backend by pid, '_' terminate backend by pid.
    n,m         'n' set new mask, 'm' show current mask.
    k,K         'k' cancel group of queries using mask, 'K' terminate group of backends using mask.
    I           show IDLE connections toggle.
    A           change activity age threshold.
    G           get query report.

other actions:
    , Q         ',' show system tables on/off, 'Q' reset postgresql statistics counters.
    z           'z' set refresh interval.
    h,F1        show this tab.
    q,Ctrl+Q    quit.

Type 'q' or 'Esc' to continue.`
)

// showHelp opens fullscreen view with built-in help.
func showHelp(g *gocui.Gui, _ *gocui.View) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("help", -1, -1, maxX-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return fmt.Errorf("set 'help' view on layout failed: %s", err)
		}

		name, tag, commit, branch := version.Version()
		versionStr := fmt.Sprintf("%s %s (%s, %s)", name, tag, commit, branch)

		v.Frame = false
		_, err = fmt.Fprintf(v, helpTemplate, versionStr)
		if err != nil {
			return fmt.Errorf("print on 'help' view failed: %s", err)
		}

		if _, err := g.SetCurrentView("help"); err != nil {
			return fmt.Errorf("set 'help' view as current on layout failed: %s", err)
		}
	}
	return nil
}

// closeHelp closes 'help' view and switches focus to 'sysstat' view.
func closeHelp(g *gocui.Gui, v *gocui.View) error {
	v.Clear()
	err := g.DeleteView("help")
	if err != nil {
		return fmt.Errorf("delete help view failed: %s", err)
	}

	if _, err := g.SetCurrentView("sysstat"); err != nil {
		return fmt.Errorf("set focus on sysstat view failed: %s", err)
	}
	return nil
}
