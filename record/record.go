// 'pgcenter record' - collects Postgres statistics and record to persistent store.

package record

import (
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"time"
)

// Config defines config container for configuring 'pgcenter record'.
type Config struct {
	Interval    time.Duration // Statistics recording interval
	Count       int           // Number of statistics snapshot to record
	OutputFile  string        // File where statistics will be saved
	AppendFile  bool          // Append data to file
	StringLimit int           // Limit of the length, to which query should be trimmed
}

// RunMain is the 'pgcenter record' main entry point.
func RunMain(dbConfig postgres.Config, config Config) error {
	app := newApp(config, dbConfig)

	err := app.setup()
	if err != nil {
		return err
	}

	fmt.Printf("INFO: recording to %s\n", config.OutputFile)

	// In case of SIGINT stop program gracefully
	doQuit := make(chan os.Signal, 1)
	signal.Notify(doQuit, os.Interrupt)

	// Run recording loop
	return app.record(doQuit)
}

// app defines 'pgcenter record' runtime dependencies.
type app struct {
	config   Config
	dbConfig postgres.Config
	views    view.Views
	recorder recorder
}

// newApp creates new 'pgcenter record' app.
func newApp(config Config, dbConfig postgres.Config) *app {
	return &app{
		config:   config,
		dbConfig: dbConfig,
	}
}

// setup configures necessary queries depending on Postgres version.
func (app *app) setup() error {
	db, err := postgres.Connect(app.dbConfig)
	if err != nil {
		return err
	}
	defer db.Close()

	// Capture locality before the deferred close — the procpidstat enrichment
	// branch in tarRecorder.collect() needs to know whether /proc is the same
	// host as Postgres. db.Local is a static property derived from the host
	// string in dbConfig, so reading it once at setup is sufficient.
	isLocal := db.Local

	props, err := stat.GetPostgresProperties(db)
	if err != nil {
		return err
	}

	// Create and configure stats views depending on running Postgres.
	opts := query.NewOptions(props.VersionNum, props.Recovery, props.GucTrackCommitTimestamp, app.config.StringLimit, props.ExtPGSSSchema)

	n, views := filterViews(props.VersionNum, props.ExtPGSSSchema, view.New())
	if n > 0 {
		fmt.Println("INFO: some statistics is not supported by the current version of Postgres and will be skipped")
	}

	// Local/remote gate for procpidstat — runtime locality is orthogonal to
	// the static NotRecordable filter in filterViews(). On a remote target the
	// /proc data would belong to the wrong host, so we strip the view here
	// regardless of whether filterViews kept it.
	var (
		ticks              float64
		cpuCount           int
		ioAvailable        bool
		delayAcctAvailable bool
	)
	if !isLocal {
		delete(views, "procpidstat")
		fmt.Println("INFO: procpidstat skipped (remote mode: /proc not available)")
	} else {
		ticks, err = stat.GetSysticksLocal()
		if err != nil {
			return fmt.Errorf("get systicks failed: %w", err)
		}
		cpuCount = runtime.NumCPU()

		// Probe IO accounting visibility against a real backend PID — the
		// owner process's own /proc/self/io is always readable and would
		// produce a false-positive availability signal. Skip the probe when
		// no other backends are active; ioAvailable stays false and IO
		// columns render as the "" sentinel.
		var firstPID int
		row := db.QueryRow("SELECT pid FROM pg_stat_activity WHERE pid > 0 AND pid != pg_backend_pid() LIMIT 1")
		if scanErr := row.Scan(&firstPID); scanErr != nil {
			if !errors.Is(scanErr, pgx.ErrNoRows) {
				return fmt.Errorf("probe backend pid: %w", scanErr)
			}
		} else {
			ioAvailable = stat.CheckIOAvailable(firstPID) == nil
		}

		delayAcctAvailable = stat.CheckDelayAcctAvailable()
	}

	err = views.Configure(opts)
	if err != nil {
		return err
	}

	app.views = views

	// Create tar recorder.
	app.recorder = newTarRecorder(tarConfig{
		filename:           app.config.OutputFile,
		append:             app.config.AppendFile,
		isLocal:            isLocal,
		ticks:              ticks,
		cpuCount:           cpuCount,
		ioAvailable:        ioAvailable,
		delayAcctAvailable: delayAcctAvailable,
	})

	return nil
}

// record collects statistics and stores into file.
func (app *app) record(doQuit chan os.Signal) error {
	var (
		count    = app.config.Count
		interval = app.config.Interval
	)

	t := time.NewTicker(interval)

	// record the number of snapshots requested by user (or record continuously until SIGINT will be received)
	var n int
	for {
		if count > 0 && n >= count {
			break
		}
		n++

		err := app.recorder.open()
		if err != nil {
			return err
		}

		stats, err := app.recorder.collect(app.dbConfig, app.views)
		if err != nil {
			return err
		}

		err = app.recorder.write(stats)
		if err != nil {
			return err
		}

		err = app.recorder.close()
		if err != nil {
			return err
		}

		select {
		case <-t.C:
			continue
		case sig := <-doQuit:
			t.Stop()
			return fmt.Errorf("got %s", sig.String())
		}
	}

	return nil
}

// filterViews removes views which are not suitable for specified version and used configuration.
func filterViews(version int, pgssSchema string, views view.Views) (int, view.Views) {
	var filtered int
	var pgssNotfound bool

	for k, v := range views {
		// Skip views explicitly marked as not recordable. No production view sets
		// NotRecordable=true currently; this branch is retained for views that are
		// only meaningful live in the TUI and is covered by a synthetic guard test.
		if v.NotRecordable {
			delete(views, k)
			filtered++
			continue
		}

		if !v.VersionOK(version) {
			delete(views, k)
			filtered++
			continue
		}

		// Skip statements views if schema, where pg_stat_statements is installed, not found.
		if strings.HasPrefix(k, "statements_") && pgssSchema == "" {
			delete(views, k)
			filtered++
			pgssNotfound = true
		}
	}

	if pgssNotfound {
		fmt.Println("INFO: pg_stat_statements not found, skip recording it")
	}

	return filtered, views
}
