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
	"strings"
	"time"
)

// Open Postgres log in $PAGER program.
func showPgLog(g *gocui.Gui, _ *gocui.View) error {
	if !conninfo.ConnLocal {
		printCmdline(g, "Show log is not supported for remote hosts")
		return nil
	}

	var currentLogfile, error = readLogPath()
	if currentLogfile == "" {
		printCmdline(g, "Can't determine log file: %s", error)
		return nil
	}

	var pager string
	if pager = os.Getenv("PAGER"); pager == "" {
		pager = utils.DefaultPager
	}

	// exit from UI and stats loop... will restore it after $PAGER is closed.
	do_exit <- 1
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
func readLogPath() (string, error) {
	var logfileRealpath, pgDatadir string

	// An easiest way to get logfile is using pg_current_logfile() function, but it's available since PG 10.
	conn.QueryRow(stat.PgGetCurrentLogfileQuery).Scan(&logfileRealpath)

	if logfileRealpath != "" {
		// Even pg_current_logfile() might return relative path
		if logfileRealpath[0] != byte('/') {
			conn.QueryRow(stat.PgGetSingleSettingQuery, "data_directory").Scan(&pgDatadir)
			logfileRealpath = pgDatadir + "/" + logfileRealpath
		}
		return logfileRealpath, nil
	}

	// if we're here, it means we are connected to Postgres that has no pg_current_logfile() function (9.6 and older).
	// Anyway, after September 2021 years, all Postgres 9.x will become EOL and code below could be deleted.
	var pgLogdir, pgLogfile, logfileFallback string
	conn.QueryRow(stat.PgGetSingleSettingQuery, "data_directory").Scan(&pgDatadir)
	conn.QueryRow(stat.PgGetSingleSettingQuery, "log_directory").Scan(&pgLogdir)
	conn.QueryRow(stat.PgGetSingleSettingQuery, "log_filename").Scan(&pgLogfile)

	if pgLogdir[0] == byte('/') {
		logfileRealpath = pgLogdir + "/" + pgLogfile // absolute path
	} else {
		logfileRealpath = pgDatadir + "/" + pgLogdir + "/" + pgLogfile // relative to DATADIR path
	}

	// If log_filename GUC contains %H%M%S part (Ubuntu-style default), it has to be replaced to timestamp of Postgres startup time.
	// Here, you're on thin fuc*ing ice, my pedigree chums (c).
	// General assumption here is that there will be Ubuntu-default '%H%M%S' expression, but potentially there may be
	// different variations of that: %H_%M_%S, %H-%M-%S or similar, and code below will not work.
	// Also things becomes a bit tricky if logfile rotated through pg_rotate_logfile(), hence instead of Postgres startup time
	// the time when rotation occured will be use. This use case is not covered here.
	if strings.Contains(logfileRealpath, "%H%M%S") {
		var pgStartTime string
		conn.QueryRow(stat.PgPostmasterStartTimeQuery).Scan(&pgStartTime)
		// rotated logfile, fallback to it in case when above isn't exist
		logfileFallback = strings.Replace(logfileRealpath, "%H%M%S", "000000", 1)
		// logfile created today
		logfileRealpath = strings.Replace(logfileRealpath, "%H%M%S", pgStartTime, 1)
	} else {
		return "", fmt.Errorf("can't parse log_filename format")
	}

	var pgLogTz string = "timezone"
	conn.QueryRow(stat.PgGetSingleSettingQuery, pgLogTz).Scan(&pgLogTz)

	t := time.Now()
	tz, _ := time.LoadLocation(pgLogTz)
	t = t.In(tz)
	logfileRealpath = strftime.Format(logfileRealpath, t)

	// check the logfile exists, if not -- use fallback name.
	if _, err := os.Stat(logfileRealpath); err != nil {
		logfileRealpath = strftime.Format(logfileFallback, t)
	}

	return logfileRealpath, nil
}
