package top

import (
	"database/sql"
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
)

// doReload performs reload of Postgres service by executing pg_reload_conf().
func doReload(g *gocui.Gui, buf string, db *postgres.DB) error {
	answer := strings.TrimPrefix(buf, dialogPrompts[dialogPgReload])
	answer = strings.TrimSuffix(answer, "\n")

	switch answer {
	case "y":
		var status sql.NullBool

		err := db.QueryRow(query.ExecReloadConf).Scan(&status)
		if err != nil {
			printCmdline(g, "Reload failed: %s", err)
		}
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

	return nil
}

// resetStat resets Postgres stats counters.
// Reset statistics that belongs to current database and pg_stat_statements stats.
// Don't reset shared stats, such as bgwriter/archiver.
func resetStat(db *postgres.DB) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		msg := "Reset statistics."

		_, err := db.Exec(query.ExecResetStats)
		if err != nil {
			msg = fmt.Sprintf("Reset statistics failed: %s", err)
		}

		// TODO: if pg_stat_statements is not installed, this will fail.
		_, err = db.Exec(query.ExecResetPgStatStatements)
		if err != nil {
			msg = fmt.Sprintf("Reset pg_stat_statements statistics failed: %s", err)
		}

		printCmdline(g, msg)

		return nil
	}
}

// runPsql starts psql session to the current connected database.
func runPsql(db *postgres.DB, uiExit chan int) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		// Ignore interrupts in pgCenter, because Ctrl+C in psql interrupts pgCenter.
		signal.Ignore(os.Interrupt)

		// exit from UI and stats loop... will restore it after psql is closed.
		uiExit <- 1
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
