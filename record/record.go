// 'pgcenter record' - collects Postgres statistics and write it to file.

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

// Config defines config container for configuring 'pgcenter record'.
type Config struct {
	Interval   time.Duration // Statistics collecting interval
	Count      int32         // Number of statistics snapshot to record
	OutputFile string        // File where statistics will be saved
	AppendFile bool          // Append to a file, or create/truncate at the beginning
	TruncLimit int           // Limit of the length, to which query should be truncated
}

// RunMain is the 'pgcenter record' main entry point.
func RunMain(dbConfig *postgres.Config, config Config) error {
	app := newApp(config, dbConfig)

	err := app.setup()
	if err != nil {
		return err
	}

	// Open file for statistics
	var flags int
	if config.AppendFile {
		flags = os.O_CREATE | os.O_RDWR
	} else {
		flags = os.O_CREATE | os.O_RDWR | os.O_TRUNC
	}

	f, err := os.OpenFile(config.OutputFile, flags, 0640)
	if err != nil {
		return err
	}

	if config.AppendFile {
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

	fmt.Printf("INFO: recording to %s\n", config.OutputFile)

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

	// Run recording loop
	return app.record(tw, doQuit)
}

// app defines 'pgcenter record' runtime dependencies.
type app struct {
	config   Config
	dbConfig *postgres.Config
	views    view.Views
}

// newApp creates new 'pgcenter record' app.
func newApp(config Config, dbConfig *postgres.Config) *app {
	return &app{
		config:   config,
		dbConfig: dbConfig,
		views:    view.New(),
	}
}

// setup configures necessary queries depending on Postgres version.
func (app *app) setup() error {
	db, err := postgres.Connect(app.dbConfig)
	if err != nil {
		return err
	}
	defer db.Close()

	// Get necessary information about Postgres: version, recovery status, settings, etc.
	props, err := stat.GetPostgresProperties(db)
	if err != nil {
		return err
	}

	// Setup recordOptions - adjust queries for used Postgres version
	app.views.Configure(props.VersionNum, props.GucTrackCommitTimestamp)

	queryOptions := query.Options{}
	queryOptions.Configure(props.VersionNum, props.Recovery, "record")

	// Compile query texts from templates using previously adjusted query options.
	for k, v := range app.views {
		q, err := query.Format(v.QueryTmpl, queryOptions)
		if err != nil {
			return err
		}
		v.Query = q
		app.views[k] = v
	}

	return nil
}

// record collects statistics and stores into file.
func (app *app) record(w *tar.Writer, doQuit chan os.Signal) error {
	var (
		count    = app.config.Count
		interval = app.config.Interval
	)

	// record the number of snapshots requested by user (or record continuously until SIGINT will be received)
	for i := count; i != 0; i-- {
		if err := collectStat(w, app.dbConfig, app.views); err != nil {
			return err
		}

		select {
		case <-time.After(interval):
			break
		case <-doQuit:
			// TODO: close tar writer, file gracefully
			return fmt.Errorf("interrupt")
		}
	}

	return nil
}

// collectStat connects to Postgres, queries stats and write it to tar file.
func collectStat(w *tar.Writer, dbConfig *postgres.Config, views view.Views) error {
	now := time.Now()

	db, err := postgres.Connect(dbConfig)
	if err != nil {
		return err
	}

	// loop over available contexts
	for k, v := range views {
		res, err := stat.NewPGresult(db, v.Query)
		if err != nil {
			return err
		}

		// write stats to a file
		name := fmt.Sprintf("%s.%s.json", k, now.Format("20060102T150405"))
		err = writeToTar(w, res, name)
		if err != nil {
			return err
		}
	}
	return nil
}

// writeToTar marshals statistics into JSON and write it to a tar file.
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
