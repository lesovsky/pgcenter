package top

import (
	"context"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/stat"
)

// RunMain is the main entry point for 'pgcenter top' command
func RunMain(dbConfig postgres.Config) error {
	// Connect to Postgres.
	db, err := postgres.Connect(dbConfig)
	if err != nil {
		return err
	}
	defer db.Close()

	// Create application instance.
	app := newApp(db, newConfig())

	// Publish the config as the ambient source of the cmdline state prefix. It belongs here and
	// not in newApp: newApp also runs in unit tests, where it would leave the package-level
	// pointer aimed at one test's config for every test that follows.
	setCmdlineConfig(app.config)

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
	frame         frameStore              // last rendered frame, repainted while the display is paused. Gocui-goroutine-only, see top/pause.go.
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

	// Create query options needed for formatting necessary queries.
	opts := query.NewOptions(props.VersionNum, props.Recovery, props.GucTrackCommitTimestamp, 256, props.ExtPGSSSchema)

	// Create and configure stats views adjusting them depending on running Postgres.
	err = app.config.views.Configure(opts)
	if err != nil {
		return err
	}

	// Set default view.
	app.config.view = app.config.views["activity"]

	// Seed the refresh interval displayed in the header. setup() is called from RunMain before
	// mainLoop starts any goroutine, so this write is race-free.
	app.config.refresh = defaultRefresh

	app.config.queryOptions = opts
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
