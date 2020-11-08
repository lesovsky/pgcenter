// Stuff related to work with Postgres log.

package top

import (
	"fmt"
	"github.com/jehiah/go-strftime"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Open Postgres log in $PAGER program.
func showPgLog(db *postgres.DB, doExit chan int) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		if !db.Local {
			printCmdline(g, "Show log is not supported for remote hosts")
			return nil
		}

		var currentLogfile = readLogPath(db)
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

// Get an absolute path of current Postgres log.
func readLogPath(db *postgres.DB) string {
	var logfileRealpath, pgDatadir string

	// An easiest way to get logfile is using pg_current_logfile() function, but it's available since PG 10.
	db.QueryRow(query.PgGetCurrentLogfileQuery).Scan(&logfileRealpath)

	if logfileRealpath != "" {
		// Even pg_current_logfile() might return relative path
		if !strings.HasPrefix(logfileRealpath, "/") {
			db.QueryRow(query.PgGetSingleSettingQuery, "data_directory").Scan(&pgDatadir)
			logfileRealpath = pgDatadir + "/" + logfileRealpath
		}
		return logfileRealpath
	}

	// if we're here, it means we are connected to old Postgres that has no pg_current_logfile() function (9.6 and older).
	// Anyway, after September 2021 years, all Postgres 9.x will become EOL and code below could be deleted.
	return lookupPostgresLogfile(db)
}

// lookupPostgresLogfiles tries to assemble in a hard way an absolute path to Postgres logfile
func lookupPostgresLogfile(db *postgres.DB) (absLogfilePath string) {
	var pgDatadir, pgLogdir, pgLogfile, pgLogfileFallback string
	db.QueryRow(query.PgGetSingleSettingQuery, "data_directory").Scan(&pgDatadir)
	db.QueryRow(query.PgGetSingleSettingQuery, "log_directory").Scan(&pgLogdir)
	db.QueryRow(query.PgGetSingleSettingQuery, "log_filename").Scan(&pgLogfile)

	if strings.HasPrefix(pgLogdir, "/") {
		absLogfilePath = pgLogdir + "/" + pgLogfile // absolute path
	} else {
		absLogfilePath = pgDatadir + "/" + pgLogdir + "/" + pgLogfile // relative to DATADIR path
	}

	// If log_filename GUC contains %H%M%S part (Ubuntu-style default), it has to be replaced to timestamp of Postgres startup time.
	// Here, you're on thin fuc*ing ice, my pedigree chums (c).
	// General assumption here is that there will be Ubuntu-default '%H%M%S' expression, but potentially there may be
	// different variations of that: %H_%M_%S, %H-%M-%S or similar, and code below will not work.
	// Also things becomes a bit tricky if logfile rotated through pg_rotate_logfile(), hence instead of Postgres startup time
	// the time when rotation occurred will be use. This use case is not covered here.
	if strings.Contains(absLogfilePath, "%H%M%S") {
		var pgStartTime string
		db.QueryRow(query.PgPostmasterStartTimeQuery).Scan(&pgStartTime)
		// rotated logfile, fallback to it in case when above isn't exist
		pgLogfileFallback = strings.Replace(absLogfilePath, "%H%M%S", "000000", 1)
		// logfile created today
		absLogfilePath = strings.Replace(absLogfilePath, "%H%M%S", pgStartTime, 1)
	}

	if strings.Contains(absLogfilePath, "%") {
		var pgLogTz = "timezone"
		db.QueryRow(query.PgGetSingleSettingQuery, pgLogTz).Scan(&pgLogTz)

		t := time.Now()
		tz, _ := time.LoadLocation(pgLogTz)
		t = t.In(tz)
		absLogfilePath = strftime.Format(absLogfilePath, t)

		// check the logfile exists, if not -- use fallback name.
		if _, err := os.Stat(absLogfilePath); err != nil {
			absLogfilePath = strftime.Format(pgLogfileFallback, t)
		}
	}

	return absLogfilePath
}
