// Stuff that allows to cancel Postgres queries and terminate backends.

package top

import (
	"database/sql"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/lib/stat"
	"strconv"
	"strings"
)

const (
	groupActive int = 1 << iota
	groupIdle
	groupIdleXact
	groupWaiting
	groupOthers
)

var (
	groupMask int
)

// Send signal to a single Postgres backend.
func killSingle(g *gocui.Gui, v *gocui.View, answer string, mode string) {
	if mode != "cancel" && mode != "terminate" {
		printCmdline(g, "Do nothing. Unknown mode (not cancel, nor terminate).") // should never be here
		return
	}

	var query string

	switch mode {
	case "cancel":
		query = stat.PgCancelSingleQuery
		answer = strings.TrimPrefix(string(v.Buffer()), dialogPrompts[dialogCancelQuery])
	case "terminate":
		query = stat.PgTerminateSingleQuery
		answer = strings.TrimPrefix(string(v.Buffer()), dialogPrompts[dialogTerminateBackend])
	}
	answer = strings.TrimSuffix(answer, "\n")

	pid, err := strconv.Atoi(answer)
	if err != nil {
		printCmdline(g, "Do nothing. Unacceptable integer.")
		return
	}

	var killed sql.NullBool

	conn.QueryRow(query, pid).Scan(&killed)

	if killed.Bool == true {
		printCmdline(g, "Successful.")
	} else {
		printCmdline(g, "Failed.")
	}
}

// Send signal to group of Postgres backends.
func killGroup(g *gocui.Gui, _ *gocui.View, mode string) {
	if ctx.current.Name != stat.ActivityView {
		printCmdline(g, "Terminate or cancel backend allowed in pg_stat_activity.")
		return
	}

	if groupMask == 0 {
		printCmdline(g, "Do nothing. The mask is empty.")
		return
	}

	if mode != "cancel" && mode != "terminate" {
		printCmdline(g, "Do nothing. Unknown mode (not cancel, nor terminate).") // should never be here
		return
	}

	var tempMask string
	if (groupMask & groupIdle) != 0 {
		tempMask += "i"
	}
	if (groupMask & groupIdleXact) != 0 {
		tempMask += "x "
	}
	if (groupMask & groupActive) != 0 {
		tempMask += "a"
	}
	if (groupMask & groupWaiting) != 0 {
		tempMask += "w"
	}
	if (groupMask & groupOthers) != 0 {
		tempMask += "o"
	}

	var query string
	var killed sql.NullInt64
	var killedTotal int64

	// Select kill function: pg_cancel_backend or pg_terminate_backend
	switch mode {
	case "cancel":
		query = stat.PgCancelGroupQuery
	case "terminate":
		query = stat.PgTerminateGroupQuery
	}

	for _, ch := range tempMask {
		switch string(ch) {
		case "i":
			ctx.sharedOptions.BackendState = "state = 'idle'"
		case "x":
			ctx.sharedOptions.BackendState = "state IN ('idle in transaction (aborted)', 'idle in transaction')"
		case "a":
			ctx.sharedOptions.BackendState = "state = 'active'"
		case "w":
			if stats.PgVersionNum < 90600 {
				ctx.sharedOptions.BackendState = "wait_event IS NOT NULL OR wait_event_type IS NOT NULL"
			} else {
				ctx.sharedOptions.BackendState = "waiting"
			}
		case "o":
			ctx.sharedOptions.BackendState = "state IN ('fastpath function call', 'disabled')"
		}

		query, _ = stat.PrepareQuery(query, ctx.sharedOptions)
		conn.QueryRow(query).Scan(&killed)

		killedTotal += killed.Int64
	}

	switch mode {
	case "cancel":
		printCmdline(g, "Cancelled "+strconv.FormatInt(killedTotal, 10)+" queries.")
	case "terminate":
		printCmdline(g, "Terminated "+strconv.FormatInt(killedTotal, 10)+" backends.")
	}
}

// Specify types of backends into dedicated group. Using the group it's possible to send them a signal.
func setBackendMask(g *gocui.Gui, v *gocui.View, answer string) {
	answer = strings.TrimPrefix(string(v.Buffer()), dialogPrompts[dialogSetMask])
	answer = strings.TrimSuffix(answer, "\n")

	// clear previous mask
	groupMask = 0

	for _, ch := range answer {
		switch string(ch) {
		case "i":
			groupMask |= groupIdle
		case "x":
			groupMask |= groupIdleXact
		case "a":
			groupMask |= groupActive
		case "w":
			groupMask |= groupWaiting
		case "o":
			groupMask |= groupOthers
		}
	}

	showBackendMask(g, v)
}

// Show which types of backends are in the group.
func showBackendMask(g *gocui.Gui, v *gocui.View) error {
	ct := "Mask: "
	if groupMask == 0 {
		ct += "empty "
	}
	if (groupMask & groupIdle) != 0 {
		ct += "idle "
	}
	if (groupMask & groupIdleXact) != 0 {
		ct += "idle_xact "
	}
	if (groupMask & groupActive) != 0 {
		ct += "active "
	}
	if (groupMask & groupWaiting) != 0 {
		ct += "waiting "
	}
	if (groupMask & groupOthers) != 0 {
		ct += "others "
	}

	printCmdline(g, ct)

	return nil
}
