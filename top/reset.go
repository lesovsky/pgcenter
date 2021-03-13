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
func resetStat(db *postgres.DB, pgssSchema string) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		msg := "Reset statistics."

		_, err := db.Exec(query.ExecResetStats)
		if err != nil {
			msg = fmt.Sprintf("Reset statistics failed: %s", err)
		}

		if pgssSchema != "" {
			opts := query.Options{PGSSSchema: pgssSchema}

			q, err := query.Format(query.ExecResetPgStatStatements, opts)
			if err != nil {
				msg = fmt.Sprintf("Reset statistics failed: %s", err)
			}

			_, err = db.Exec(q)
			if err != nil {
				msg = fmt.Sprintf("Reset pg_stat_statements statistics failed: %s", err)
			}
		}

		printCmdline(g, msg)

		return nil
	}
}
