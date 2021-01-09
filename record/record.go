// 'pgcenter record' - collects Postgres statistics and write it to file.

package record

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
	"os"
	"os/signal"
	"time"
)

// Config defines config container for configuring 'pgcenter record'.
type Config struct {
	Interval     time.Duration // Statistics collecting interval
	Count        int           // Number of statistics snapshot to record
	OutputFile   string        // File where statistics will be saved
	TruncateFile bool          // Truncate a file before beginning
	StringLimit  int           // Limit of the length, to which query should be trimmed
}

// RunMain is the 'pgcenter record' main entry point.
func RunMain(dbConfig *postgres.Config, config Config) error {
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
	config    Config
	dbConfig  *postgres.Config
	views     view.Views
	collector collector
}

// newApp creates new 'pgcenter record' app.
func newApp(config Config, dbConfig *postgres.Config) *app {
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

	views, err := configureViews(db, view.New())
	app.views = views

	// Create tar collector.
	app.collector = newTarCollector(tarConfig{
		filename: app.config.OutputFile,
		truncate: app.config.TruncateFile,
	})

	return nil
}

// record collects statistics and stores into file.
func (app *app) record(doQuit chan os.Signal) error {
	var (
		count    = app.config.Count
		interval = app.config.Interval
	)

	// record the number of snapshots requested by user (or record continuously until SIGINT will be received)
	for i := count; i > 0; i-- {
		err := app.collector.open()
		if err != nil {
			return err
		}

		stats, err := app.collector.collect(app.dbConfig, app.views)
		if err != nil {
			return err
		}

		err = app.collector.write(stats)
		if err != nil {
			return err
		}

		err = app.collector.close()
		if err != nil {
			return err
		}

		select {
		case <-time.After(interval):
			continue
		case <-doQuit:
			err = app.collector.close()
			if err != nil {
				fmt.Println(err)
			}
			return fmt.Errorf("got interrupt")
		}
	}

	return nil
}

// configureViews create necessary stats views depending on Postgres settings and version.
// TODO: should be moved into 'view' package
func configureViews(db *postgres.DB, views view.Views) (view.Views, error) {
	props, err := stat.GetPostgresProperties(db)
	if err != nil {
		return nil, err
	}

	// Setup recordOptions - adjust queries for used Postgres version.
	views.Configure(props.VersionNum, props.GucTrackCommitTimestamp)

	queryOptions := query.Options{}
	queryOptions.Configure(props.VersionNum, props.Recovery, "record")

	// Compile query texts from templates using previously adjusted query options.
	for k, v := range views {
		q, err := query.Format(v.QueryTmpl, queryOptions)
		if err != nil {
			return nil, err
		}
		v.Query = q
		views[k] = v
	}

	return views, nil
}
