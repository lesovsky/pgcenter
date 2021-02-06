package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
)

// resetStat resets Postgres stats counters.
// Reset statistics that belongs to current database and pg_stat_statements stats.
// Don't reset shared stats, such as bgwriter or archiver.
func resetStat(db *postgres.DB, pgssAvail bool) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		msg := "Reset statistics."

		_, err := db.Exec(query.ExecResetStats)
		if err != nil {
			msg = fmt.Sprintf("Reset statistics failed: %s", err)
		}

		if pgssAvail {
			_, err = db.Exec(query.ExecResetPgStatStatements)
			if err != nil {
				msg = fmt.Sprintf("Reset pg_stat_statements statistics failed: %s", err)
			}
		}

		printCmdline(g, msg)

		return nil
	}
}
