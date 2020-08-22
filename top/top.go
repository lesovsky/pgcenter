// 'pgcenter top' - top-like stats viewer.

package top

import (
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/lib/stat"
	"time"
)

// 'top' program config.
type config struct {
	minRefresh      time.Duration
	refreshInterval time.Duration
}

// app defines stuff required for application.
type app struct {
	config   config
	context  *context
	ui       *gocui.Gui
	db       *postgres.DB
	stats    *stat.Stat
	doExit   chan int
	doUpdate chan int
}

// RunMain is the main entry point for 'pgcenter top' command
func RunMain(dbConfig *postgres.Config) error {
	config := config{
		minRefresh:      1 * time.Second,
		refreshInterval: 1 * time.Second,
	}

	// Connect to Postgres.
	db, err := postgres.Connect(dbConfig)
	if err != nil {
		return err
	}

	app := &app{
		config:  config,
		context: &context{},
		db:      db,
		stats:   &stat.Stat{},
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
	app.context.contextList = stat.ContextList{
		stat.DatabaseView:            &stat.PgStatDatabaseUnit,
		stat.ReplicationView:         &stat.PgStatReplicationUnit,
		stat.TablesView:              &stat.PgStatTablesUnit,
		stat.IndexesView:             &stat.PgStatIndexesUnit,
		stat.SizesView:               &stat.PgTablesSizesUnit,
		stat.FunctionsView:           &stat.PgStatFunctionsUnit,
		stat.ProgressVacuumView:      &stat.PgStatProgressVacuumUnit,
		stat.ProgressClusterView:     &stat.PgStatProgressClusterUnit,
		stat.ProgressCreateIndexView: &stat.PgStatProgressCreateIndexUnit,
		stat.ActivityView:            &stat.PgStatActivityUnit,
		stat.StatementsTimingView:    &stat.PgSSTimingUnit,
		stat.StatementsGeneralView:   &stat.PgSSGeneralUnit,
		stat.StatementsIOView:        &stat.PgSSIoUnit,
		stat.StatementsTempView:      &stat.PgSSTempUnit,
		stat.StatementsLocalView:     &stat.PgSSLocalUnit,
	}

	// Select default context unit
	app.context.current = app.context.contextList[stat.ActivityView]

	// Aux stats is not displayed by default
	app.context.aux = auxNone

	// Adjust queries depending on Postgres version
	app.context.contextList.AdjustQueries(app.stats.PgInfo)
	app.context.sharedOptions.Adjust(app.stats.PgInfo, "top")

	app.doExit = make(chan int)
	app.doUpdate = make(chan int)
}
