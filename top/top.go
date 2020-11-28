// 'pgcenter top' - top-like stats viewer.

package top

import (
	"context"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/stat"
)

// app defines stuff required for application.
type app struct {
	postgresProps stat.PostgresProperties
	config        *config
	ui            *gocui.Gui
	db            *postgres.DB
	doExit        chan int // TODO: следует переименовать в uiExit, т.к. используется для выхода из UI при запуске less/pager/psql утилит.
	doUpdate      chan int // TODO: бесполезная штука, надо удалить.
}

// RunMain is the main entry point for 'pgcenter top' command
func RunMain(dbConfig *postgres.Config) error {
	config := newConfig()

	// Connect to Postgres.
	db, err := postgres.Connect(dbConfig)
	if err != nil {
		return err
	}
	defer db.Close()

	app := &app{
		config: config,
		db:     db,
	}

	// Setup context - which kind of stats should be displayed
	err = app.Setup()
	if err != nil {
		return err
	}

	// Run application workers and UI.
	return mainLoop(context.TODO(), app)
}

// Initial setup of the context. Set defaults and override settings which depends on Postgres version, recovery status, etc.
func (app *app) Setup() error {
	// Read details about Postgres
	props, err := stat.ReadPostgresProperties(app.db)
	if err != nil {
		return err
	}

	// Adjust queries depending on Postgres version
	app.config.views.Configure(props.VersionNum, props.GucTrackCommitTimestamp)
	app.config.queryOptions.Adjust(props.VersionNum, props.Recovery, "top")

	// Compile query text from templates using previously adjusted query options.
	for k, v := range app.config.views {
		q, err := query.PrepareQuery(v.QueryTmpl, app.config.queryOptions)
		if err != nil {
			return err
		}
		v.Query = q
		app.config.views[k] = v
	}

	// Set default view.
	app.config.view = app.config.views["databases"]

	app.postgresProps = props
	app.doExit = make(chan int)
	app.doUpdate = make(chan int)

	return nil
}
