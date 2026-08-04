package top

import (
	"sync/atomic"
	"time"

	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
)

// defaultRefresh is the initial refresh interval. It is the single literal for that value: both
// the collector seed (doWork) and the displayed copy (app.setup) use it, so the number shown in
// the header and the interval the collector actually runs at cannot drift apart.
const defaultRefresh = time.Second

// config defines 'top' program runtime configuration.
type config struct {
	view         view.View      // Current active view.
	views        view.Views     // List of all available views.
	queryOptions query.Options  // Queries' settings that might depend on Postgres version.
	viewCh       chan view.View // Channel used for passing view settings to stats goroutine.
	logtail      stat.Logfile   // Logfile used for working with Postgres log file.
	dialog       dialogType     // Remember current user-started dialog, used for selecting needed dialog handler.
	menu         menuStyle      // When working with menus, keep properties of the menu.
	procMask     int            // Process mask used for selecting group of process.
	scrollOffset int            // Horizontal scroll position: index into scrollable columns (1..Ncols-1); 0 means no scroll. Ephemeral, reset on view switch.
	verbose      bool           // Verbose display mode for the top summary panels. Persistent: unlike scrollOffset, it is NOT reset on view switch (mirrored into every views entry).
	// autoScrollToOrderKey is a one-shot request to bring the sort column into the visible window.
	// It is set by the sort handlers (orderKeyLeft/orderKeyRight) and consumed — and cleared — by
	// renderDbstat on the next frame, so manual [ / ] scrolling afterwards is never undone by the
	// following refresh. Ephemeral like scrollOffset: reset on BOTH view-switch paths.
	autoScrollToOrderKey bool
	// paused freezes the displayed statistics: while it is set the collector keeps running and the
	// screen keeps being repainted from the last rendered frame instead of the incoming one.
	// It is the only cross-goroutine value of the pause feature, which is why it is an atomic and
	// not a plain bool: written on the gocui goroutine (the Space handler, and the handlers that
	// lift the pause) and read on the worker goroutine (statLoop). Same discipline as uiGeneration,
	// top/ui.go:24-29. Zero value is "not paused", so newConfig needs no change.
	paused atomic.Bool
	// refresh is the durable copy of the current refresh interval, kept solely for displaying it in
	// the sysstat header. It cannot be read back from view.Refresh: that field is a transient courier
	// for the stats goroutine — both writers (doWork in ui.go and changeRefresh in config_view.go)
	// set it, send the view on viewCh and zero it on the very next line, because collectStat treats a
	// changed non-zero Refresh on an incoming view as "the interval changed" and returns early. So at
	// rest view.Refresh is always 0. Written on the gocui goroutine (changeRefresh, seeded in
	// app.setup before any goroutine starts) and read there too (printStat's g.Update closure).
	refresh time.Duration
}

// newConfig creates 'top' initial configuration.
func newConfig() *config {
	views := view.New()

	return &config{
		views:  views,
		viewCh: make(chan view.View),
	}
}
