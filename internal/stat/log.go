package stat

import (
	"github.com/jehiah/go-strftime"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"io"
	"os"
	"strings"
	"time"
)

// Logfile describes Postgres log file and its properties.
type Logfile struct {
	Path string   // Absolute path to logfile
	File *os.File // Pointer to opened logfile
	Size int64    // Size of the logfile (read file's content only when size grows)
}

// Open opens log file specified in Path and defines File object.
func (l *Logfile) Open() error {
	f, err := os.Open(l.Path)
	if err != nil {
		return err
	}

	l.File = f
	return nil
}

// Close closes log file.
func (l *Logfile) Close() error {
	return l.File.Close()
}

// ReOpen closes log file and open it again in case of rotate.
func (l *Logfile) Reopen(db *postgres.DB) error {
	if err := l.Close(); err != nil {
		return err
	}

	// Update path on case if it changed. (What are cases when it is required?)
	l.Path = ReadLogPath(db)

	return l.Open()
}

// Read methods reads logfile until required number of newlines aren't collected
func (l *Logfile) Read(linesLimit int, bufsize int) ([]byte, error) {
	var offset int64 = -1 // offset used for per-byte backward reading of the logfile
	var position int64    // position within the logfile from which reading starts
	var startpos int64    // final position from which reading of required amount of lines will start
	var newlines int      // newlines counter

	// Start reading from the end of file
	position, err := l.File.Seek(offset, 2)
	if err != nil {
		return nil, err
	}

	for i := 0; i < bufsize; i++ {
		// The beginning of the file is reached, stop the reading
		if position < 0 {
			startpos = 0
			break
		}

		// Read 1 byte and check - is it a newline symbol? If symbol is a newline, remember this position - when number
		// of required newlines is reached, will start reading of logfile from this position to the buffer.
		c := make([]byte, 1)
		_, err := l.File.ReadAt(c, position)
		if err != nil {
			return nil, err
		}
		if string(c) == "\n" {
			newlines++
			startpos = position + 1 // +1 here, means that reading will start from symbol which is next after newline
		}

		// Stop reading when required number of newlines is reached
		if newlines > linesLimit {
			break
		}

		offset--   // move 1 byte back
		position-- // shift position too
	}

	// Final reading of the logfile to buffer from calculated position
	buf := make([]byte, bufsize)
	_, err = l.File.ReadAt(buf, startpos)
	if err != nil && err != io.EOF {
		return nil, err
	}

	return buf, nil
}

// Get an absolute path of current Postgres log.
func ReadLogPath(db *postgres.DB) string {
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
	// Anyway, after November 2021, all Postgres 9.x will become EOL and code below could be deleted.
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
