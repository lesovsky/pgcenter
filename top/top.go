// 'pgcenter top' - top-like stats viewer.

package top

import (
	"context"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
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
	return mainLoop(context.Background(), app)
}

// app defines application and all necessary dependencies.
type app struct {
	config        *config                 // runtime configuration.
	ui            *gocui.Gui              // UI instance.
	uiExit        chan int                // used for signaling when to need exiting from UI.
	uiError       error                   // hold error occurred during executing UI.
	db            *postgres.DB            // connection to Postgres.
	postgresProps stat.PostgresProperties // properties of Postgres to which connected to.
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

	// Create and configure stats views depending on running Postgres.
	err = app.config.views.Configure(props.VersionNum, props.Recovery, props.GucTrackCommitTimestamp, "top")
	if err != nil {
		return err
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
