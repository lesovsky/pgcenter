// Stuff related to Postgres config files such as displaying, editing, etc.

package top

import (
	"bytes"
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/stat"
	"os"
	"os/exec"
	"strings"
)

// showPgConfig gets Postgres settings and show it in $PAGER program.
func showPgConfig(db *postgres.DB, doExit chan int) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		res, err := stat.NewPGresult(db, query.PgGetConfigAllQuery)
		if err != nil {
			printCmdline(g, err.Error())
			return nil
		}

		var buf bytes.Buffer
		fmt.Fprintf(&buf, "PostgreSQL configuration:\n")
		res.Fprint(&buf)

		var pager string
		if pager = os.Getenv("PAGER"); pager == "" {
			pager = "less"
		}

		// Exit from UI and stats loop... will restore it after $PAGER is closed.
		doExit <- 1
		g.Close()

		cmd := exec.Command(pager)
		cmd.Stdin = strings.NewReader(buf.String())
		cmd.Stdout = os.Stdout

		if err := cmd.Run(); err != nil {
			// If external program fails, save error and show it to user in next UI iteration
			errSaved = err
		}

		return err
	}
}

// Open specified configuration file in $EDITOR program
func editPgConfig(g *gocui.Gui, db *postgres.DB, n string, doExit chan int) error {
	if !db.Local {
		printCmdline(g, "Edit config is not supported for remote hosts")
		return nil
	}

	var configFile string

	if n != stat.GucRecoveryFile {
		db.QueryRow(query.PgGetSingleSettingQuery, n).Scan(&configFile)
	} else {
		var dataDirectory string
		db.QueryRow(query.PgGetSingleSettingQuery, stat.GucDataDir).Scan(&dataDirectory)
		configFile = dataDirectory + "/" + n
	}

	var editor string
	if editor = os.Getenv("EDITOR"); editor == "" {
		editor = "vi"
	}

	// Exit from UI and stats loop... will restore it after $EDITOR is closed.
	doExit <- 1
	g.Close()

	cmd := exec.Command(editor, configFile)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	if err := cmd.Run(); err != nil {
		// If external program fails, save error and show it to user in next UI iteration
		errSaved = err
		return err
	}

	return nil
}
