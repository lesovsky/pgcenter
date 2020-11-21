// config defines 'top' program runtime configuration - selected screen and its settings like columns order, used
// aligning, filters, etc.

package top

import (
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/view"
	"time"
)

// 'top' program config.
type config struct {
	// minRefresh is a minimal allowed screen refresh interval.
	minRefresh time.Duration
	// refreshInterval is a current refresh interval.
	refreshInterval time.Duration
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
		minRefresh:      1 * time.Second,
		refreshInterval: 1 * time.Second,
		views:           views,
		viewCh:          make(chan view.View),
	}
}
