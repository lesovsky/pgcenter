// Stuff related to work with Postgres log.

package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/stat"
	"os"
	"os/exec"
)

// Open Postgres log in $PAGER program.
func showPgLog(db *postgres.DB, doExit chan int) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		if !db.Local {
			printCmdline(g, "Show log is not supported for remote hosts")
			return nil
		}

		currentLogfile := stat.ReadLogPath(db)
		if currentLogfile == "" {
			printCmdline(g, "Can't assemble absolute path to log file")
			return nil
		}

		var pager string
		if pager = os.Getenv("PAGER"); pager == "" {
			pager = "less"
		}

		// exit from UI and stats loop... will restore it after $PAGER is closed.
		doExit <- 1
		g.Close()

		cmd := exec.Command(pager, currentLogfile)
		cmd.Stdout = os.Stdout

		if err := cmd.Run(); err != nil {
			// if external program fails, save error and show it to user in next UI iteration
			errSaved = fmt.Errorf("failed to open %s: %s", currentLogfile, err)
			return err
		}

		return nil
	}
}
