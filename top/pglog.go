package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/stat"
	"os"
	"os/exec"
)

// showPgLog opens Postgres log in $PAGER program.
func showPgLog(db *postgres.DB, version int, uiExit chan int) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		if !db.Local {
			printCmdline(g, "Show log is not supported for remote hosts")
			return nil
		}

		logfile, err := stat.GetPostgresCurrentLogfile(db, version)
		if err != nil {
			printCmdline(g, "Can't get path to log file")
			return nil
		}

		var pager string
		if pager = os.Getenv("PAGER"); pager == "" {
			pager = "less"
		}

		// Exit from UI and stats loop. Restore it after $PAGER is closed.
		uiExit <- 1
		g.Close()

		cmd := exec.Command(pager, logfile)
		cmd.Stdout = os.Stdout

		if err := cmd.Run(); err != nil {
			// If external program fails, save error and show it to user in next UI iteration.
			errSaved = fmt.Errorf("open %s failed: %s", logfile, err)
			return err
		}

		return nil
	}
}
