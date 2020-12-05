package stat

import (
	"fmt"
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
func (l *Logfile) Reopen(db *postgres.DB, version int) error {
	if err := l.Close(); err != nil {
		return err
	}

	// Update path on case if it changed. (What are cases when it is required?)
	logfile, err := GetPostgresCurrentLogfile(db, version)
	if err != nil {
		return err
	}
	l.Path = logfile

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
func GetPostgresCurrentLogfile(db *postgres.DB, version int) (string, error) {
	// Postgres 10 has pg_current_logfile() function which is easies way to get current logfile path.
	var logfile string
	var err error
	if version >= 100000 {
		err := db.QueryRow(query.PgGetCurrentLogfileQuery).Scan(&logfile)
		if err != nil {
			return "", err
		}
	} else {
		// Old Postgres versions have no pg_current_logfile() function, try to use more complicated way to get logfile.
		logfile, err = lookupPostgresLogfile(db)
		if err != nil {
			return "", err
		}
	}

	if logfile == "" {
		return "", fmt.Errorf("failed to get logfile path: empty response")
	}

	// Handle relative paths.
	var datadir string
	if !strings.HasPrefix(logfile, "/") {
		err := db.QueryRow(query.PgGetSingleSettingQuery, "data_directory").Scan(&datadir)
		if err != nil {
			return "", err
		}
		logfile = datadir + "/" + logfile
	}

	return logfile, nil
}

// lookupPostgresLogfiles tries to assemble in a hard way an absolute path to Postgres logfile
func lookupPostgresLogfile(db *postgres.DB) (string, error) {
	var datadir, logdir, logfilename, startTime, timezone string
	q := "select current_setting('data_directory') as data_directory, current_setting('log_directory') as log_directory, current_setting('log_filename') as log_filename, to_char(pg_postmaster_start_time(), 'HH24MISS') as start_time, current_setting('timezone') as timezone"
	if err := db.QueryRow(q).Scan(&datadir, &logdir, &logfilename, &startTime, &timezone); err != nil {
		return "", err
	}

	return assemblePostgresLogfile(datadir, logdir, logfilename, startTime, timezone), nil
}

//
func assemblePostgresLogfile(datadir, logdir, logfilename, startTime, timezone string) string {
	var logfile string

	// Handle relative value of log directory.
	if strings.HasPrefix(logdir, "/") {
		logfile = logdir + "/" + logfilename // log directory is an absolute path
	} else {
		logfile = datadir + "/" + logdir + "/" + logfilename // log directory is a relative to DATADIR path
	}

	// If log_filename GUC contains %H%M%S part (Ubuntu-style default), it has to be replaced to timestamp of Postgres startup time.
	// Here, you're on thin fuc*ing ice, my pedigree chums (c).
	// General assumption here is that there will be Ubuntu-default '%H%M%S' expression, but potentially there may be
	// different variations of that: %H_%M_%S, %H-%M-%S or similar, and code below will not work.
	// Also things becomes a bit tricky if logfile rotated through pg_rotate_logfile(), hence instead of Postgres startup time
	// the time when rotation occurred will be use. This use case is not covered here.

	if strings.Contains(logfile, "%") {
		t := time.Now()
		tz, _ := time.LoadLocation(timezone)
		t = t.In(tz)

		var logfileFallback string
		if strings.Contains(logfile, "%H%M%S") {
			logfile, logfileFallback = strings.Replace(logfile, "%H%M%S", startTime, 1), strings.Replace(logfile, "%H%M%S", "000000", 1)
		} else {
			logfile, logfileFallback = strftime.Format(logfile, t), ""
		}

		// check the logfile or fallback logfiles exist.
		if _, err := os.Stat(logfile); err != nil {
			logfile = logfileFallback
			if _, err := os.Stat(logfile); err != nil {
				logfile = "" // neither logfile, nor fallback logfile exists, return empty string.
			}
		}
	}

	return logfile
}
