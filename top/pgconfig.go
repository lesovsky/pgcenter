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

const (
	// gucMainConfFile is the name of GUC which stores Postgres config file location
	gucMainConfFile = "config_file"
	// gucHbaFile is the name of GUC which stores Postgres HBA file location
	gucHbaFile = "hba_file"
	// gucIdentFile is the name of GUC which stores ident file location
	gucIdentFile = "ident_file"
	// gucRecoveryFile is the name of pseudo-GUC which stores recovery settings location
	gucRecoveryFile = "recovery.conf"
	// gucDataDir is the name of GUC which stores data directory location
	gucDataDir = "data_directory"
)

// showPgConfig fetches Postgres configuration settings and opens it in $PAGER program.
func showPgConfig(db *postgres.DB, doExit chan int) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		res, err := stat.NewPGresult(db, query.GetAllSettings)
		if err != nil {
			printCmdline(g, err.Error())
			return nil
		}

		var buf bytes.Buffer
		if _, err := fmt.Fprintf(&buf, "PostgreSQL configuration:\n"); err != nil {
			printCmdline(g, "print string to buffer failed: %s", err)
			return nil
		}

		if err := res.Fprint(&buf); err != nil {
			printCmdline(g, "print string to buffer failed: %s", err)
			return nil
		}

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

// editPgConfig opens specified configuration file in $EDITOR program.
func editPgConfig(g *gocui.Gui, db *postgres.DB, filename string, doExit chan int) error {
	if !db.Local {
		printCmdline(g, "Edit config is not supported for remote hosts")
		return nil
	}

	var configFile string
	if filename != gucRecoveryFile {
		if err := db.QueryRow(query.GetSetting, filename).Scan(&configFile); err != nil {
			printCmdline(g, "scan failed: %s", err)
			return nil
		}
	} else {
		var dataDirectory string
		if err := db.QueryRow(query.GetSetting, gucDataDir).Scan(&dataDirectory); err != nil {
			printCmdline(g, "scan failed: %s", err)
			return nil
		}
		configFile = dataDirectory + "/" + filename
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
