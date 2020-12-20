// config defines 'top' program runtime configuration - selected screen and its settings like columns order, used
// aligning, filters, etc.

package top

import (
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
)

// config defines 'top' program runtime configuration.
type config struct {
	view         view.View      // Current active view.
	views        view.Views     // List of all available views.
	queryOptions query.Options  // Queries' settings that might depend on Postgres version.
	viewCh       chan view.View // Channel used for passing view settings to stats goroutine.
	logtail      stat.Logfile   // Logfile used for working with Postgres log file.
	dialog       dialogType     // Remember current user-started dialog, used for selecting needed dialog handler.
	menu         menuStyle      // When working with menus, keep properties of the menu.
}

// newConfig creates 'top' initial configuration.
func newConfig() *config {
	views := view.New()

	return &config{
		views:  views,
		viewCh: make(chan view.View),
	}
}
