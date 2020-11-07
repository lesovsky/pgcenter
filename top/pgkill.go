// Stuff that allows to cancel Postgres queries and terminate backends.

package top

import (
	"database/sql"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/stat"
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
func killSingle(g *gocui.Gui, v *gocui.View, answer string, db *postgres.DB, mode string) {
	if mode != "cancel" && mode != "terminate" {
		printCmdline(g, "Do nothing. Unknown mode (not cancel, nor terminate).") // should never be here
		return
	}

	var query string

	switch mode {
	case "cancel":
		query = stat.PgCancelSingleQuery
		answer = strings.TrimPrefix(v.Buffer(), dialogPrompts[dialogCancelQuery])
	case "terminate":
		query = stat.PgTerminateSingleQuery
		answer = strings.TrimPrefix(v.Buffer(), dialogPrompts[dialogTerminateBackend])
	}
	answer = strings.TrimSuffix(answer, "\n")

	pid, err := strconv.Atoi(answer)
	if err != nil {
		printCmdline(g, "Do nothing. Unacceptable integer.")
		return
	}

	var killed sql.NullBool

	db.QueryRow(query, pid).Scan(&killed)

	if killed.Bool == true {
		printCmdline(g, "Successful.")
	} else {
		printCmdline(g, "Failed.")
	}
}

// Send signal to group of Postgres backends.
func killGroup(g *gocui.Gui, _ *gocui.View, app *app, mode string) {
	if app.config.view.Name != "activity" {
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

	var template, query string
	var killed sql.NullInt64
	var killedTotal int64

	// Select kill function: pg_cancel_backend or pg_terminate_backend
	switch mode {
	case "cancel":
		template = stat.PgCancelGroupQuery
	case "terminate":
		template = stat.PgTerminateGroupQuery
	}

	/* advanced mode */
	var states = map[int]string{
		groupIdle:     "state = 'idle'",
		groupIdleXact: "state IN ('idle in transaction (aborted)', 'idle in transaction')",
		groupActive:   "state = 'active'",
		groupWaiting:  "wait_event IS NOT NULL OR wait_event_type IS NOT NULL",
		groupOthers:   "state IN ('fastpath function call', 'disabled')",
	}

	for state, part := range states {
		if (groupMask & state) != 0 {
			app.config.sharedOptions.BackendState = part
			if state == groupWaiting && app.postgresProps.VersionNum < 90600 {
				app.config.sharedOptions.BackendState = "waiting"
			}
			query, _ = stat.PrepareQuery(template, app.config.sharedOptions)
			err := app.db.QueryRow(query).Scan(&killed)
			if err != nil {
				printCmdline(g, "failed to send signal to backends: %s", err)
			}

			killedTotal += killed.Int64
		}
	}

	switch mode {
	case "cancel":
		printCmdline(g, "Cancelled "+strconv.FormatInt(killedTotal, 10)+" queries.")
	case "terminate":
		printCmdline(g, "Terminated "+strconv.FormatInt(killedTotal, 10)+" backends.")
	}
}

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
