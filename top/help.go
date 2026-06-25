package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/version"
)

const (
	helpTemplate = `%s: Help for interactive commands

general actions:
    a,b,f,o     mode: 'a' activity, 'b' bgwriter/checkpointer, 'f' functions, 'o' replication slots,
    r,w               'r' replication, 'w' WAL,
    s,t,i             's' tables sizes, 't' tables, 'i' indexes.
    d,D               'd' pg_stat_database switch, 'D' pg_stat_database menu.
    x,X               'x' pg_stat_statements switch, 'X' pg_stat_statements menu.
    p,P               'p' pg_stat_progress_* switch, 'P' pg_stat_progress_* menu.
    j,J               'j' pg_stat_io switch (operations/timings), 'J' pg_stat_io menu.
    S                 'S' per-process system stats (local mode only; Shift+S).
    Left,Right,<,/    'Left,Right' change column sort, '<' desc/asc sort toggle, '/' set filter.
    Up,Down           'Up' increase column width, 'Down' decrease column width.
    [,]               '[' scroll columns left, ']' scroll columns right.
    C,E,R       config: 'C' show config, 'E' edit configs, 'R' reload config.
    ~                 start psql session.
    l                 open log file with pager.

extra stats actions:
    B,N,F,L       'B' diskstat, 'N' nicstat, 'F' filesystems, L' logtail.
    v             'v' verbose mode for the summary panels on/off.

activity actions:
    -,_         '-' cancel backend by pid, '_' terminate backend by pid.
    n,m         'n' set new mask, 'm' show current mask.
    k,K         'k' cancel group of queries using mask, 'K' terminate group of backends using mask.
    I           show IDLE connections toggle.
    A           change activity age threshold.
    G           get query report.

other actions:
    , Q         ',' show system tables on/off, 'Q' reset postgresql statistics counters
                      ('Q' does not reset shared stats: pg_stat_io, bgwriter, wal).
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
			return fmt.Errorf("set 'help' view on layout failed: %w", err)
		}

		name, tag, commit, branch := version.Version()
		versionStr := fmt.Sprintf("%s %s (%s, %s)", name, tag, commit, branch)

		v.Frame = false
		_, err = fmt.Fprintf(v, helpTemplate, versionStr)
		if err != nil {
			return fmt.Errorf("print on 'help' view failed: %w", err)
		}

		if _, err := g.SetCurrentView("help"); err != nil {
			return fmt.Errorf("set 'help' view as current on layout failed: %w", err)
		}
	}
	return nil
}

// closeHelp closes 'help' view and switches focus to 'sysstat' view.
func closeHelp(g *gocui.Gui, v *gocui.View) error {
	v.Clear()
	err := g.DeleteView("help")
	if err != nil {
		return fmt.Errorf("delete help view failed: %w", err)
	}

	if _, err := g.SetCurrentView("sysstat"); err != nil {
		return fmt.Errorf("set focus on sysstat view failed: %w", err)
	}
	return nil
}
