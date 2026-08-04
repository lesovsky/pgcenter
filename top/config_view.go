package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/math"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
	"regexp"
	"strconv"
	"time"
)

const (
	colsWidthMax  = 256 // max width allowed for column, they can't be wider than that value
	colsWidthStep = 4   // minimal step of changing column's width, 1 is too boring and 4 looks good
	maxInt        = int(^uint(0) >> 1)
)

// orderKeyLeft switches sort order to left column.
func orderKeyLeft(config *config) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		config.view.OrderKey--
		if config.view.OrderKey < 0 {
			config.view.OrderKey = config.view.Ncols - 1
		}

		// Ask the next render to bring the new sort column into the visible window. The offset
		// itself is not computed here: column widths are known only after alignment against real
		// data, which happens on the render path.
		config.autoScrollToOrderKey = true

		// Re-sorting is the collector's job, so the frozen frame has to give way. The refreshing
		// variant: this handler writes no cmdline of its own, so nobody else would repaint the
		// [PAUSED] marker away.
		liftPauseRefresh(g, config)

		config.viewCh <- config.view
		return nil
	}
}

// orderKeyRight switches sort order to right column.
func orderKeyRight(config *config) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		config.view.OrderKey++
		if config.view.OrderKey >= config.view.Ncols {
			config.view.OrderKey = 0
		}

		// See orderKeyLeft: the scroll is deferred to the next render, which knows the widths.
		config.autoScrollToOrderKey = true

		// See orderKeyLeft: the refreshing variant, for the same reason.
		liftPauseRefresh(g, config)

		config.viewCh <- config.view
		return nil
	}
}

// scrollLeft scrolls the columns window one step to the left.
// It decrements config.scrollOffset, clamped at the lower bound 0, and sends the
// view on viewCh solely to trigger an immediate redraw — the view itself is not
// mutated (render reads scrollOffset directly from *config).
func scrollLeft(config *config) func(_ *gocui.Gui, _ *gocui.View) error {
	return func(_ *gocui.Gui, _ *gocui.View) error {
		config.scrollOffset = math.Max(config.scrollOffset-1, 0)

		config.viewCh <- config.view
		return nil
	}
}

// scrollRight scrolls the columns window one step to the right.
// It increments config.scrollOffset without an upper bound in the handler — the
// terminal/column widths are not available here. The true upper clamp is enforced
// at render time, which writes the clamped offset back into config.scrollOffset.
// The view is not mutated; sending it on viewCh only triggers an immediate redraw.
func scrollRight(config *config) func(_ *gocui.Gui, _ *gocui.View) error {
	return func(_ *gocui.Gui, _ *gocui.View) error {
		// Cheap guard against integer overflow only; this is not a user-facing limit.
		// The true upper bound is enforced by the render-time write-back.
		if config.scrollOffset < maxInt {
			config.scrollOffset++
		}

		config.viewCh <- config.view
		return nil
	}
}

// increaseWidth increases visible width of current column.
func increaseWidth(config *config) func(_ *gocui.Gui, _ *gocui.View) error {
	return func(_ *gocui.Gui, _ *gocui.View) error {
		idx := config.view.OrderKey // index of the current selected column

		// Increase the width using current width. Clamp the value, it should not be greater than max allowed limit.
		config.view.ColsWidth[idx] = math.Min(config.view.ColsWidth[idx]+colsWidthStep, colsWidthMax)

		config.viewCh <- config.view
		return nil
	}
}

// decreaseWidth decreases visible width of current column.
func decreaseWidth(config *config) func(_ *gocui.Gui, _ *gocui.View) error {
	return func(_ *gocui.Gui, _ *gocui.View) error {
		idx := config.view.OrderKey // index of the current selected column

		// The lower bound of the width is the length of the column's NAME, and names are known only
		// after a frame has been rendered - config.view.Cols is populated on the render path, while
		// view.New() leaves it nil, and metadata left over from a wider screen can be shorter than
		// the current sort key. Indexing it unconditionally panics inside a key handler, which gocui
		// does not recover. There is no sensible fallback floor to invent, so the handler returns
		// having changed nothing and having asked for no frame: with no columns on screen there is
		// nothing to narrow. Bounds, not a nil check - stale metadata is non-nil.
		if idx < 0 || idx >= len(config.view.Cols) {
			return nil
		}

		// Decrease the width using current width. Clamp the value, it should not be less than width of column's name.
		config.view.ColsWidth[idx] = math.Max(config.view.ColsWidth[idx]-colsWidthStep, len(config.view.Cols[idx]))

		config.viewCh <- config.view
		return nil
	}
}

// switchSortOrder switches sort order of current column between DESC and ASC.
func switchSortOrder(config *config) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		config.view.OrderDesc = !config.view.OrderDesc

		// Re-sorting is done by the collector. Silent variant: the write below repaints the prefix.
		liftPause(config)

		printCmdline(g, "Switch sort order")

		config.viewCh <- config.view
		return nil
	}
}

// setFilter adds pattern for filtering values in the current column.
func setFilter(answer string, view view.View) string {
	// Clear used pattern if empty string is entered. Report success only when a filter
	// has really been removed — 'delete' on a missing key is a no-op and reporting a
	// successful clear when nothing was cleared misleads the user.
	if answer == "\n" || answer == "" {
		if !isFilterActive(view.Filters[view.OrderKey]) {
			return "Filters: no filter on this column"
		}

		delete(view.Filters, view.OrderKey)
		return "Filters: regular expression cleared"
	}

	// Compile regexp and store to filters.
	re, err := regexp.Compile(answer)
	if err != nil {
		return fmt.Sprintf("Filters: %s", err)
	}

	view.Filters[view.OrderKey] = re
	return "Filters: ok"
}

// isFilterActive reports whether the passed pattern is an active filter.
// The predicate must stay identical to the one used by printHeaderCell when marking
// a filtered column with '*' — otherwise the marker, the counter of cleared filters
// and the cmdline filter indicator could disagree about what "filtered" means.
func isFilterActive(re *regexp.Regexp) bool {
	return re != nil && re.String() != ""
}

// activeFilterCount returns the number of active filters in the passed map.
// A nil map is valid input and yields zero.
func activeFilterCount(filters map[int]*regexp.Regexp) int {
	var n int
	for _, re := range filters {
		if isFilterActive(re) {
			n++
		}
	}
	return n
}

// clearAllFilters removes all active filters of the passed view and returns the message
// for cmdline and the number of removed filters. Filters are deleted in place: the map
// header is shared with the view's copy stored in config.views, so assigning a fresh map
// would leave the stored copy with the old filters and they would come back on the next
// view switch.
//
// Only active entries are counted and removed, which keeps the reported number equal to
// the number of '*' markers the user actually sees. This relies on the invariant that
// setFilter never stores an inert entry (a nil or empty pattern): an empty answer takes
// the clear branch above, so an inactive entry cannot appear in the map in the first place.
func clearAllFilters(v view.View) (string, int) {
	n := activeFilterCount(v.Filters)
	if n == 0 {
		return "Filters: no active filters", 0
	}

	for k, re := range v.Filters {
		if isFilterActive(re) {
			delete(v.Filters, k)
		}
	}

	return fmt.Sprintf("Filters: cleared %d filter(s)", n), n
}

// clearFilters removes all filters of the current view at once.
func clearFilters(config *config) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		msg, n := clearAllFilters(config.view)

		// Notify the stats goroutine only when something has been removed. viewCh is
		// unbuffered and nobody is expected to read an update that changes nothing.
		if n > 0 {
			config.viewCh <- config.view
		}

		printCmdline(g, "%s", msg)
		return nil
	}
}

// switchViewTo switches from current view to requested using high-level logic.
func switchViewTo(app *app, c string) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		// in case of switching to pg_stat_statements and it isn't available - keep current view
		if app.postgresProps.ExtPGSSSchema == "" && c == "statements" {
			printCmdline(g, "NOTICE: pg_stat_statements is not available in this database")
			return nil
		}

		// Switch to requested view.
		switch c {
		case "databases":
			viewSwitchHandler(app.config, databasesNextView(app.config.view.Name))
		case "statements":
			viewSwitchHandler(app.config, statementsNextView(app.config.view.Name))
		case "progress":
			viewSwitchHandler(app.config, progressNextView(app.config.view.Name))
		case "statio":
			viewSwitchHandler(app.config, statioNextView(app.config.view.Name))
		default:
			viewSwitchHandler(app.config, c)
		}

		printCmdline(g, "%s", app.config.view.Msg)
		return nil
	}
}

// databasesNextView depending on current databases view returns next view.
func databasesNextView(current string) string {
	var next string

	switch current {
	case "databases_general":
		next = "databases_sessions"
	case "databases_sessions":
		next = "databases_general"
	default:
		next = "databases_general"
	}
	return next
}

// statioNextView depending on current pg_stat_io view returns next view.
func statioNextView(current string) string {
	var next string

	switch current {
	case "stat_io":
		next = "stat_io_time"
	case "stat_io_time":
		next = "stat_io"
	default:
		next = "stat_io"
	}
	return next
}

// statementsNextView depending on current statements view returns next view.
func statementsNextView(current string) string {
	var next string

	switch current {
	case "statements_timings":
		next = "statements_general"
	case "statements_general":
		next = "statements_io"
	case "statements_io":
		next = "statements_temp"
	case "statements_temp":
		next = "statements_local"
	case "statements_local":
		next = "statements_wal"
	case "statements_wal":
		next = "statements_jit"
	case "statements_jit":
		next = "statements_timings"
	default:
		next = "statements_timings"
	}
	return next
}

// progressNextView depending on current progress view returns next view.
func progressNextView(current string) string {
	var next string

	switch current {
	case "progress_vacuum":
		next = "progress_cluster"
	case "progress_cluster":
		next = "progress_index"
	case "progress_index":
		next = "progress_analyze"
	case "progress_analyze":
		next = "progress_basebackup"
	case "progress_basebackup":
		next = "progress_copy"
	case "progress_copy":
		next = "progress_vacuum"
	default:
		next = "progress_vacuum"
	}
	return next
}

// viewSwitchHandler is routine handler which switches views and notify channel.
//
// The pause is lifted HERE rather than in the ~26 places that call this helper: a new screen is
// filled by the collector, and doing it inside covers every screen switch - letter keys and the
// D/X/P/J menus alike - without touching a single call site. Silent variant: every caller writes
// the cmdline immediately afterwards, which repaints the token prefix.
//
// Callers with an early return of their own keep it ABOVE their call to this helper (switchViewTo's
// "pg_stat_statements is not available" guard, top/config_view.go), so a switch that did not happen
// does not lift the pause.
func viewSwitchHandler(config *config, c string) {
	config.views[config.view.Name] = config.view
	config.view = config.views[c]
	config.scrollOffset = 0             // horizontal scroll is ephemeral; reset on view switch
	config.autoScrollToOrderKey = false // a pending auto-scroll must not fire on the new screen

	liftPause(config)

	config.viewCh <- config.view
}

// switchViewToProcPidStat switches to the per-process system stats view.
// The handler enforces the local-mode guard, probes /proc/[pid]/io availability
// using a real PG backend PID from pg_stat_activity, and patches
// CollectExtra/IOAvailable onto the view before sending it on viewCh.
// It must NOT delegate to viewSwitchHandler — that helper reloads the view from
// the static map and would discard the runtime patches.
func switchViewToProcPidStat(app *app) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		// Horizontal scroll is ephemeral; reset it when entering the per-process
		// screen. This path bypasses viewSwitchHandler, so the reset is done here.
		app.config.scrollOffset = 0

		// Same for a pending auto-scroll request: it belongs to the outgoing screen's sort
		// column. Reset before the local-mode guard below, so the switch cannot leave it armed.
		app.config.autoScrollToOrderKey = false

		if !app.db.Local {
			printCmdline(g, "Per-process stats available in local mode only")
			return nil
		}

		// The screen really is being switched now, so the frozen frame has to give way. This
		// handler bypasses viewSwitchHandler on purpose (see the doc comment), so it has nobody to
		// inherit the lift from. Silent variant: the switch below writes the cmdline itself, on
		// every one of its branches.
		liftPause(app.config)

		// Probe IO access using the first real PG backend PID from pg_stat_activity.
		// /proc/self/io is always readable by the owner process, so it is not a
		// useful probe — we need a PID that belongs to a different OS user (postgres).
		var probePID int
		_ = app.db.QueryRow("SELECT pid FROM pg_stat_activity WHERE pid != pg_backend_pid() LIMIT 1").Scan(&probePID)
		if probePID == 0 {
			probePID = 1 // fallback: init/systemd is always running under a different user
		}
		ioErr := stat.CheckIOAvailable(probePID)
		ioAvailable := ioErr == nil
		delayAcctAvailable := stat.CheckDelayAcctAvailable()

		// Save current view back to the views map so per-view state (sort, filters, etc.) is preserved.
		app.config.views[app.config.view.Name] = app.config.view

		// Load the procpidstat view and patch runtime-only fields.
		v := app.config.views["procpidstat"]
		v.CollectExtra = stat.CollectProcPidStat
		v.IOAvailable = ioAvailable
		v.DelayAcctAvailable = delayAcctAvailable

		app.config.view = v
		app.config.viewCh <- v

		// Show a warning combining IO and iodelay availability. printCmdline must
		// be called exactly once per execution path (calling it twice overwrites
		// the first message before the user can read it).
		switch {
		case !ioAvailable && !delayAcctAvailable:
			printCmdline(g, "IO stats and iodelay unavailable: run as postgres user + sysctl -w kernel.task_delayacct=1, then re-open screen")
		case !ioAvailable:
			printCmdline(g, "IO stats unavailable (cannot read /proc/%d/io): run as postgres user or via sudo.", probePID)
		case !delayAcctAvailable:
			printCmdline(g, "iodelay unavailable (task_delayacct=0): run sysctl -w kernel.task_delayacct=1, then re-open screen")
		default:
			printCmdline(g, "%s", v.Msg)
		}
		return nil
	}
}

// toggleSysTables toggles showing system tables/indexes.
func toggleSysTables(config *config) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		name := config.view.Name
		if name != "tables" && name != "indexes" && name != "sizes" {
			return nil
		}

		// If current view type is 'user' - switch to 'all', and vice versa.
		if config.queryOptions.ViewType == "user" {
			config.queryOptions.ViewType = "all"
		} else {
			config.queryOptions.ViewType = "user"
		}

		// Format new queries depending on requested view type.
		queries := make([]string, 3)
		for i, t := range []string{"tables", "indexes", "sizes"} {
			q, err := query.Format(config.views[t].QueryTmpl, config.queryOptions)
			if err != nil {
				return err
			}
			queries[i] = q
		}

		// Update queries in view.
		for i, t := range []string{"tables", "indexes", "sizes"} {
			v := config.views[t]
			v.Query = queries[i]
			config.views[t] = v
		}

		config.view = config.views[name]

		// Below the guard AND below the reformatting loop's error return: a query that could not be
		// reformatted changed nothing, and the collector was never asked for anything. Silent
		// variant: the write below repaints the prefix.
		liftPause(config)

		config.viewCh <- config.view

		printCmdline(g, "Show relations: %s", config.queryOptions.ViewType)
		return nil
	}
}

// changeQueryAge changes age threshold for showing queries and transactions (pg_stat_activity only).
func changeQueryAge(answer string, config *config) string {
	// Reset threshold if empty answer.
	if answer == "" {
		answer = "00:00:00"
	}

	// Parse user input.
	err := parseHumanTimeString(answer)
	if err != nil {
		return fmt.Sprintf("Activity age: do nothing, %s", err.Error())
	}

	// Remember current age to restore it if formatting new query will fail.
	fallbackAge := config.queryOptions.QueryAgeThresh

	// Update query options and format activity query.
	config.queryOptions.QueryAgeThresh = answer
	q, err := query.Format(config.view.QueryTmpl, config.queryOptions)
	if err != nil {
		config.queryOptions.QueryAgeThresh = fallbackAge // restore fallback
		return fmt.Sprintf("Activity age: do nothing, %s", err.Error())
	}

	// Update query and view.
	config.view.Query = q

	// Below both early returns - a rejected input and a query that would not format changed
	// nothing. Silent variant: dialogFinish prints the string returned below (top/dialog.go), and
	// that write is the one that repaints the prefix.
	liftPause(config)

	config.viewCh <- config.view

	return "Activity age: set " + answer
}

// parseHumanTimeString parses time in human-readable format and validates its correctness.
func parseHumanTimeString(t string) error {
	pattern := `^([0-9]{1,2}):([0-9]{1,2}):([0-9]{1,2})(\.[0-9]{1,6})?$`
	re := regexp.MustCompile(pattern)

	if !re.MatchString(t) {
		return fmt.Errorf("invalid input")
	}

	parts := re.FindStringSubmatch(t)
	if len(parts) != 5 {
		return fmt.Errorf("invalid input")
	}

	var hour, mins, sec, msec int
	if parts[4] == "" {
		_, err := fmt.Sscanf(t, "%d:%d:%d", &hour, &mins, &sec)
		if err != nil {
			return err
		}
	} else {
		_, err := fmt.Sscanf(t, "%d:%d:%d.%d", &hour, &mins, &sec, &msec)
		if err != nil {
			return err
		}
	}

	if (hour < 0 || hour > 23) || (mins < 0 || mins > 59) || (sec < 0 || sec > 59) || (msec < 0 || msec > 999999) {
		return fmt.Errorf("invalid input")
	}

	return nil
}

// A toggle to show 'idle' connections (pg_stat_activity and procpidstat views).
func toggleIdleConns(config *config) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		if config.view.Name != "activity" && config.view.Name != "procpidstat" {
			return nil
		}

		config.queryOptions.ShowNoIdle = !config.queryOptions.ShowNoIdle

		q, err := query.Format(config.view.QueryTmpl, config.queryOptions)
		if err != nil {
			return err
		}

		config.view.Query = q

		// Below the screen guard AND below the query.Format error return, for the same reason as in
		// toggleSysTables. Silent variant: both branches below write the cmdline.
		liftPause(config)

		config.viewCh <- config.view

		if config.queryOptions.ShowNoIdle {
			printCmdline(g, "Show idle connections: off.")
		} else {
			printCmdline(g, "Show idle connections: on.")
		}

		return nil
	}
}

// changeRefresh changes current refresh interval.
func changeRefresh(answer string, config *config) string {
	if answer == "" {
		return "Refresh: do nothing"
	}

	interval, err := strconv.Atoi(answer)
	if err != nil {
		return "Refresh: do nothing, invalid input"
	}

	if interval < 1 || interval > 300 {
		return "Refresh: input value should be between 1 and 300"
	}

	// Set refresh interval, send it to stats channel and reset interval in the view.
	// Refresh interval should not be saved as a per-view setting. It's used as a setting for stats goroutine.
	config.view.Refresh = time.Duration(interval) * time.Second
	config.viewCh <- config.view
	config.view.Refresh = 0

	// Keep the durable copy in sync - it is what the sysstat header displays, since view.Refresh is
	// zeroed above. Written only here, on the success path, so an invalid input never changes it.
	config.refresh = time.Duration(interval) * time.Second

	return "Refresh: ok"
}
