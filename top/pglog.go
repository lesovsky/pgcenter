// Stuff related to work with Postgres log.

package top

import (
	"fmt"
	"github.com/jehiah/go-strftime"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/lib/stat"
	"github.com/lesovsky/pgcenter/lib/utils"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Open Postgres log in $PAGER program.
func showPgLog(g *gocui.Gui, _ *gocui.View) error {
	if !conninfo.ConnLocal {
		printCmdline(g, "Show log is not supported for remote hosts")
		return nil
	}

	var currentLogfile = readLogPath()
	if currentLogfile == "" {
		printCmdline(g, "Can't assemble absolute path to log file")
		return nil
	}

	var pager string
	if pager = os.Getenv("PAGER"); pager == "" {
		pager = utils.DefaultPager
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

// Get an absolute path of current Postgres log.
func readLogPath() string {
	var logfileRealpath, pgDatadir, pgLoggingCollector string

	conn.QueryRow(stat.PgGetSingleSettingQuery, "logging_collector").Scan(&pgLoggingCollector)
	if pgLoggingCollector == "on" {
		// An easiest way to get logfile is using pg_current_logfile() function, but it's available since PG 10.
		if stats.PgVersionNum >= 100000 {
			conn.QueryRow(stat.PgGetCurrentLogfileQuery).Scan(&logfileRealpath)

			if logfileRealpath != "" {
				// Even pg_current_logfile() might return relative path
				if !strings.HasPrefix(logfileRealpath, "/") {
					conn.QueryRow(stat.PgGetSingleSettingQuery, "data_directory").Scan(&pgDatadir)
					logfileRealpath = pgDatadir + "/" + logfileRealpath
				}
			}
		}

		// if we're here, it means we are connected to an older Postgres, or logging is not properly configured
		logfileRealpath = lookupPostgresLogfile()
	} else {
		// log file is configured outside of Postgres
		logfileRealpath = lookupSystemLogfile()
	}

	return logfileRealpath
}

// lookupPostgresLogfiles tries to assemble in a hard way an absolute path to Postgres logfile
func lookupPostgresLogfile() (absLogfilePath string) {
	var pgDatadir, pgLogdir, pgLogfile, pgLogfileFallback string
	conn.QueryRow(stat.PgGetSingleSettingQuery, "data_directory").Scan(&pgDatadir)
	conn.QueryRow(stat.PgGetSingleSettingQuery, "log_directory").Scan(&pgLogdir)
	conn.QueryRow(stat.PgGetSingleSettingQuery, "log_filename").Scan(&pgLogfile)

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
		conn.QueryRow(stat.PgPostmasterStartTimeQuery).Scan(&pgStartTime)
		// rotated logfile, fallback to it in case when above isn't exist
		pgLogfileFallback = strings.Replace(absLogfilePath, "%H%M%S", "000000", 1)
		// logfile created today
		absLogfilePath = strings.Replace(absLogfilePath, "%H%M%S", pgStartTime, 1)
	}

	if strings.Contains(absLogfilePath, "%") {
		var pgLogTz = "timezone"
		conn.QueryRow(stat.PgGetSingleSettingQuery, pgLogTz).Scan(&pgLogTz)

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

// lookupSystemLogfiles tries to assemble an absolute path to logfile configured outside of Postgres
func lookupSystemLogfile() string {
	var pgDatadir, pgClusterName, newLogfilePath string

	if path, err := os.Stat(conninfo.Logpath); err != nil {
		// print invalid path
		return conninfo.Logpath
	} else {
		if !path.IsDir() {
			return conninfo.Logpath
		}
	}

	if stats.PgVersionNum >= 90500 {
		conn.QueryRow(stat.PgGetSingleSettingQuery, "cluster_name").Scan(&pgClusterName)

		pgClusterName = strings.Replace(pgClusterName, "/", "-", -1)
		newLogfilePath = conninfo.Logpath + "/postgresql-" + pgClusterName + ".log"
		if _, err := os.Stat(newLogfilePath); err == nil {
			return newLogfilePath
		}
	} else {
		conn.QueryRow(stat.PgGetSingleSettingQuery, "data_directory").Scan(&pgDatadir)

		pgClusterName = filepath.Base(filepath.Dir(pgDatadir)) + "-" + filepath.Base(pgDatadir)
		newLogfilePath = conninfo.Logpath + "/postgresql-" + pgClusterName + ".log"
		if _, err := os.Stat(newLogfilePath); err == nil {
			return newLogfilePath
		}

		pgClusterName = filepath.Base(pgDatadir)
		newLogfilePath = conninfo.Logpath + "/postgresql-" + pgClusterName + ".log"
		if _, err := os.Stat(newLogfilePath); err == nil {
			return newLogfilePath
		}
	}

	newLogfilePath = conninfo.Logpath + "/postgresql.log"
	if _, err := os.Stat(newLogfilePath); err == nil {
		return newLogfilePath
	}

	newLogfilePath = conninfo.Logpath + "/postgres.log"
	if _, err := os.Stat(newLogfilePath); err == nil {
		return newLogfilePath
	}

	return ""
}
