// Code related to 'pgcenter record' command

package record

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
	"io"
	"os"
	"os/signal"
	"time"
)

// RecordOptions is the container for recorder settings
type Options struct {
	Interval   time.Duration // Statistics collecting interval
	Count      int32         // Number of statistics snapshot to record
	OutputFile string        // File where statistics will be saved
	AppendFile bool          // Append to a file, or create/truncate at the beginning
	TruncLimit int           // Limit of the length, to which query should be truncated
}

// RunMain is the 'pgcenter record' main entry point.
func RunMain(dbConfig *postgres.Config, opts Options) error {
	var err error
	fmt.Printf("INFO: recording to %s\n", opts.OutputFile)

	// Setup connection to Postgres
	db, err := postgres.Connect(dbConfig)
	if err != nil {
		return err
	}
	defer db.Close()

	// Open file for statistics
	var flags int
	if opts.AppendFile {
		flags = os.O_CREATE | os.O_RDWR
	} else {
		flags = os.O_CREATE | os.O_RDWR | os.O_TRUNC
	}

	f, err := os.OpenFile(opts.OutputFile, flags, 0640)
	if err != nil {
		return err
	}

	if opts.AppendFile {
		_, err = f.Seek(0, io.SeekEnd)
		if err != nil {
			return err
		}
	}

	defer func() {
		err := f.Close()
		if err != nil {
			fmt.Printf("closing file failed: %s, ignore it", err)
		}
	}()

	// Initialize tar writer
	tw := tar.NewWriter(f)
	defer func() {
		err := tw.Close()
		if err != nil {
			fmt.Printf("closing tar writer failed: %s, ignore it", err)
		}
	}()

	// In case of SIGINT stop program gracefully
	doQuit := make(chan os.Signal, 1)
	signal.Notify(doQuit, os.Interrupt)

	// Get necessary information about Postgres: version, recovery status, settings, etc.
	props, err := stat.GetPostgresProperties(db)
	if err != nil {
		return err
	}

	// Setup recordOptions - adjust queries for used Postgres version
	views := view.New()
	views.Configure(props.VersionNum, props.GucTrackCommitTimestamp)

	queryOptions := query.Options{}
	queryOptions.Configure(props.VersionNum, props.Recovery, "top")

	// Compile query texts from templates using previously adjusted query options.
	for k, v := range views {
		q, err := query.Format(v.QueryTmpl, queryOptions)
		if err != nil {
			return err
		}
		v.Query = q
		views[k] = v
	}

	// Run recording loop
	return recordLoop(tw, db, views, opts, doQuit)
}

// Record stats with an interval
func recordLoop(w *tar.Writer, db *postgres.DB, views view.Views, opts Options, doQuit chan os.Signal) error {
	// record the number of snapshots requested by user (or record continuously until SIGINT will be received)
	for i := opts.Count; i != 0; i-- {
		if err := doWork(w, db, views); err != nil {
			return err
		}

		select {
		case <-time.After(opts.Interval):
			break
		case <-doQuit:
			return fmt.Errorf("interrupt")
		}
	}

	return nil
}

// Read stats from Postgres and write it to a file
func doWork(w *tar.Writer, db *postgres.DB, views view.Views) error {
	now := time.Now()

	// loop over available contexts
	for k, v := range views {
		res, err := stat.NewPGresult(db, v.Query)
		if err != nil {
			return err
		}

		// write stats to a file
		name := fmt.Sprintf("%s.%s.json", k, now.Format("20060102T150405"))
		if err := writeToTar(w, res, name); err != nil {
			return err
		}
	}
	return nil
}

// Write statistics to a tar-file
func writeToTar(w *tar.Writer, object interface{}, name string) error {
	data, err := json.Marshal(object)
	if err != nil {
		return err
	}

	hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(len(data)), ModTime: time.Now()}
	if err := w.WriteHeader(hdr); err != nil {
		return err
	}

	if _, err := w.Write(data); err != nil {
		return err
	}

	return nil
}
