// 'pgcenter top' - top-like stats viewer.

package top

import (
	"context"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/stat"
)

// RunMain is the main entry point for 'pgcenter top' command
func RunMain(dbConfig *postgres.Config) error {
	// Connect to Postgres.
	db, err := postgres.Connect(dbConfig)
	if err != nil {
		return err
	}
	defer db.Close()

	// Create application instance.
	app := newApp(db, newConfig())

	// Setup application.
	err = app.setup()
	if err != nil {
		return err
	}

	// Run application workers and UI.
	return mainLoop(context.TODO(), app)
}

// app defines application and all necessary dependencies.
type app struct {
	postgresProps stat.PostgresProperties
	config        *config
	ui            *gocui.Gui
	db            *postgres.DB
	uiExit        chan int
}

// newApp creates new application instance.
func newApp(db *postgres.DB, config *config) *app {
	return &app{
		config: config,
		db:     db,
	}
}

// setup performs initial application setup based on Postgres settings to which application connected to.
func (app *app) setup() error {
	// Fetch Postgres properties.
	props, err := stat.GetPostgresProperties(app.db)
	if err != nil {
		return err
	}

	// Select proper queries depending on Postgres version and settings.
	app.config.views.Configure(props.VersionNum, props.GucTrackCommitTimestamp)
	app.config.queryOptions.Configure(props.VersionNum, props.Recovery, "top")

	// Compile query texts from templates using previously adjusted query options.
	for k, v := range app.config.views {
		q, err := query.Format(v.QueryTmpl, app.config.queryOptions)
		if err != nil {
			return err
		}
		v.Query = q
		app.config.views[k] = v
	}

	// Set default view.
	app.config.view = app.config.views["activity"]

	app.postgresProps = props
	app.uiExit = make(chan int)

	return nil
}

// quit performs graceful application quit.
func (app *app) quit() func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		close(app.uiExit)
		g.Close()
		app.db.Close()

		return gocui.ErrQuit
	}
}
