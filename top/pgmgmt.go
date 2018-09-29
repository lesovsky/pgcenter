// Stuff related to Postgres management functions.

package top

import (
	"database/sql"
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/lib/stat"
	"github.com/lesovsky/pgcenter/lib/utils"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
)

// Performs reload of Postgres config files.
func doReload(g *gocui.Gui, v *gocui.View, answer string) {
	answer = strings.TrimPrefix(string(v.Buffer()), dialogPrompts[dialogPgReload])
	answer = strings.TrimSuffix(answer, "\n")

	switch answer {
	case "y":
		var status sql.NullBool

		conn.QueryRow(stat.PgReloadConfQuery).Scan(&status)
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
func resetStat(g *gocui.Gui, _ *gocui.View) error {
	var msg string = "Reset statistics."

	_, err := conn.Exec(stat.PgResetStats)
	if err != nil {
		msg = fmt.Sprintf("Reset statistics failed: %s", err)
	}
	_, err = conn.Exec(stat.PgResetPgss)
	if err != nil {
		msg = fmt.Sprintf("Reset pg_stat_statements statistics failed: %s", err)
	}

	printCmdline(g, msg)

	return nil
}

// Run psql session to the database using current connection's settings
func runPsql(g *gocui.Gui, _ *gocui.View) error {
	// Ignore interrupts in pgCenter, because Ctrl+C in psql interrupts pgCenter
	signal.Ignore(os.Interrupt)

	// exit from UI and stats loop... will restore it after psql is closed.
	do_exit <- 1
	g.Close()
	cmd := exec.Command(utils.DefaultPsql,
		"-h", conninfo.Host, "-p", strconv.Itoa(conninfo.Port),
		"-U", conninfo.User, "-d", conninfo.Dbname)
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
