// 'pgcenter top' - top-like stats viewer.

package top

import (
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/lib/stat"
)

// app defines stuff required for application.
type app struct {
	config   *config
	ui       *gocui.Gui
	db       *postgres.DB
	stats    *stat.Stat
	doExit   chan int
	doUpdate chan int
}

// RunMain is the main entry point for 'pgcenter top' command
func RunMain(dbConfig *postgres.Config) error {
	config := newConfig()

	// Connect to Postgres.
	db, err := postgres.Connect(dbConfig)
	if err != nil {
		return err
	}

	app := &app{
		config: config,
		db:     db,
		stats:  &stat.Stat{},
	}

	defer db.Close()

	// Get necessary information about Postgres, such as version, recovery status, settings, etc.
	app.stats.ReadPgInfoNew(db)

	// Setup context - which kind of stats should be displayed
	app.Setup()

	// Run terminal user interface.
	return uiLoop(app)
}

// Initial setup of the context. Set defaults and override settings which depends on Postgres version, recovery status, etc.
func (app *app) Setup() {
	// Aux stats is not displayed by default
	app.config.aux = auxNone

	// Adjust queries depending on Postgres version
	app.config.views.Configure(app.stats.PgInfo.PgVersionNum, app.stats.PgInfo.PgTrackCommitTs)
	app.config.sharedOptions.Adjust(app.stats.PgInfo, "top")

	app.doExit = make(chan int)
	app.doUpdate = make(chan int)
}
