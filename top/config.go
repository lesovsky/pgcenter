// config defines 'top' program runtime configuration - selected screen and its settings like columns order, used
// aligning, filters, etc.

package top

import (
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/view"
)

// 'top' program config.
type config struct {
	// active view
	view view.View
	// list of all available views
	views view.Views
	//
	queryOptions query.Options // Queries' settings that depends on Postgres version
	aux          auxType       // Type of current auxiliary stats
	//
	viewCh chan view.View
}

func newConfig() *config {
	views := view.New()

	return &config{
		views:  views,
		viewCh: make(chan view.View),
	}
}
