// Stuff related to Postgres management functions.

package top

import (
	"database/sql"
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/stat"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
)

// Performs reload of Postgres config files.
func doReload(g *gocui.Gui, v *gocui.View, db *postgres.DB, answer string) {
	answer = strings.TrimPrefix(string(v.Buffer()), dialogPrompts[dialogPgReload])
	answer = strings.TrimSuffix(answer, "\n")

	switch answer {
	case "y":
		var status sql.NullBool

		db.QueryRow(stat.PgReloadConfQuery).Scan(&status)
		if status.Bool {
			printCmdline(g, "Reload issued.")
		} else {
			printCmdline(g, "Reload failed.")
		}
	case "n":
		printCmdline(g, "Do nothing. Reload canceled.")
	default:
		printCmdline(g, "Do nothing. Unknown answer.")
	}
}

// Reset pg_stat_* statistics counters. Reset statistics that belongs to current database and pg_stat_statements stats.
// Don't reset shared stats, such as bgwriter/archiver.
func resetStat(db *postgres.DB) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		var msg string = "Reset statistics."

		_, err := db.Exec(stat.PgResetStats)
		if err != nil {
			msg = fmt.Sprintf("Reset statistics failed: %s", err)
		}
		_, err = db.Exec(stat.PgResetPgss)
		if err != nil {
			msg = fmt.Sprintf("Reset pg_stat_statements statistics failed: %s", err)
		}

		printCmdline(g, msg)

		return nil
	}
}

// Run psql session to the database using current connection's settings
func runPsql(db *postgres.DB, doExit chan int) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		// Ignore interrupts in pgCenter, because Ctrl+C in psql interrupts pgCenter
		signal.Ignore(os.Interrupt)

		// exit from UI and stats loop... will restore it after psql is closed.
		doExit <- 1
		g.Close()
		cmd := exec.Command("psql",
			"-h", db.Config.Host, "-p", strconv.Itoa(int(db.Config.Port)),
			"-U", db.Config.User, "-d", db.Config.Database)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			// if external program fails, save error and show it to user in next UI iteration
			errSaved = err
			return err
		}

		return nil
	}
}
