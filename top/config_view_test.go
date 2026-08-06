package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/view"
	"github.com/stretchr/testify/assert"
	"regexp"
	"sync"
	"testing"
	"time"
)

func Test_orderKeyLeft(t *testing.T) {
	testcases := []struct {
		orderKey int
		want     int
	}{
		{orderKey: 0, want: 13}, // why 13? because of views["activity"].Ncols == 13
		{orderKey: 5, want: 4},
	}

	wg := sync.WaitGroup{}

	for i, tc := range testcases {
		t.Run(fmt.Sprintln(i), func(t *testing.T) {
			config := newConfig()
			config.view = config.views["activity"]
			config.view.OrderKey = tc.orderKey

			wg.Add(1)
			go func() {
				v := <-config.viewCh
				assert.Equal(t, tc.want, v.OrderKey)
				close(config.viewCh)
				wg.Done()
			}()

			fn := orderKeyLeft(config)
			assert.NoError(t, fn(nil, nil))
		})
		wg.Wait()
	}
}

func Test_orderKeyRight(t *testing.T) {
	testcases := []struct {
		orderKey int
		want     int
	}{
		{orderKey: 13, want: 0}, // 13 is the index of last column
		{orderKey: 5, want: 6},
	}

	wg := sync.WaitGroup{}

	for i, tc := range testcases {
		t.Run(fmt.Sprintln(i), func(t *testing.T) {
			config := newConfig()
			config.view = config.views["activity"]
			config.view.OrderKey = tc.orderKey

			wg.Add(1)
			go func() {
				v := <-config.viewCh
				assert.Equal(t, tc.want, v.OrderKey)
				close(config.viewCh)
				wg.Done()
			}()

			fn := orderKeyRight(config)
			assert.NoError(t, fn(nil, nil))
		})
		wg.Wait()
	}
}

func Test_scrollLeft(t *testing.T) {
	testcases := []struct {
		offset int
		want   int
	}{
		{offset: 0, want: 0}, // bottom clamp: stays at 0, never goes negative
		{offset: 3, want: 2}, // decrement
	}

	wg := sync.WaitGroup{}

	for i, tc := range testcases {
		t.Run(fmt.Sprintln(i), func(t *testing.T) {
			config := newConfig()
			config.view = config.views["activity"]
			config.view.OrderKey = 5 // sentinel: scroll must not touch sort
			config.scrollOffset = tc.offset

			wg.Add(1)
			go func() {
				v := <-config.viewCh
				assert.Equal(t, 5, v.OrderKey) // scroll does not mutate sort order
				close(config.viewCh)
				wg.Done()
			}()

			fn := scrollLeft(config)
			assert.NoError(t, fn(nil, nil))
			assert.Equal(t, tc.want, config.scrollOffset)
		})
		wg.Wait()
	}
}

func Test_scrollRight(t *testing.T) {
	testcases := []struct {
		offset int
		want   int
	}{
		{offset: 0, want: 1}, // increment from 0
		{offset: 3, want: 4}, // increment, no upper clamp in handler
	}

	wg := sync.WaitGroup{}

	for i, tc := range testcases {
		t.Run(fmt.Sprintln(i), func(t *testing.T) {
			config := newConfig()
			config.view = config.views["activity"]
			config.view.OrderKey = 5 // sentinel: scroll must not touch sort
			config.scrollOffset = tc.offset

			wg.Add(1)
			go func() {
				v := <-config.viewCh
				assert.Equal(t, 5, v.OrderKey) // scroll does not mutate sort order
				close(config.viewCh)
				wg.Done()
			}()

			fn := scrollRight(config)
			assert.NoError(t, fn(nil, nil))
			assert.Equal(t, tc.want, config.scrollOffset)
		})
		wg.Wait()
	}
}

// Test_scrollOrthogonalToSort verifies the handler-level contract between scroll and sort:
// scroll handlers do not mutate OrderKey, and sort handlers do not mutate scrollOffset —
// they only raise the one-shot autoScrollToOrderKey request. Sorting is therefore only
// DEFERREDLY orthogonal to scrolling: the handler leaves the window alone, but the next
// renderDbstat consumes the request and may move the window to reveal the sort column.
func Test_scrollOrthogonalToSort(t *testing.T) {
	wg := sync.WaitGroup{}

	// scroll does not change OrderKey
	for i, fnFactory := range []func(*config) func(*gocui.Gui, *gocui.View) error{scrollLeft, scrollRight} {
		t.Run(fmt.Sprintf("scroll-keeps-sort-%d", i), func(t *testing.T) {
			config := newConfig()
			config.view = config.views["activity"]
			config.view.OrderKey = 7
			config.scrollOffset = 2

			wg.Add(1)
			go func() {
				<-config.viewCh
				close(config.viewCh)
				wg.Done()
			}()

			fn := fnFactory(config)
			assert.NoError(t, fn(nil, nil))
			assert.Equal(t, 7, config.view.OrderKey)
			wg.Wait()
		})
	}

	// sort does not change scrollOffset
	for i, fnFactory := range []func(*config) func(*gocui.Gui, *gocui.View) error{orderKeyLeft, orderKeyRight} {
		t.Run(fmt.Sprintf("sort-keeps-scroll-%d", i), func(t *testing.T) {
			config := newConfig()
			config.view = config.views["activity"]
			config.view.OrderKey = 5
			config.scrollOffset = 3

			wg.Add(1)
			go func() {
				<-config.viewCh
				close(config.viewCh)
				wg.Done()
			}()

			fn := fnFactory(config)
			assert.NoError(t, fn(nil, nil))
			assert.Equal(t, 3, config.scrollOffset)
			assert.True(t, config.autoScrollToOrderKey, "the sort handler defers the scroll, it does not perform it")
			wg.Wait()
		})
	}
}

// Test_orderKeySetsAutoScrollFlag verifies both sort handlers raise the one-shot auto-scroll
// request and leave the offset itself to the render.
func Test_orderKeySetsAutoScrollFlag(t *testing.T) {
	wg := sync.WaitGroup{}

	for i, fnFactory := range []func(*config) func(*gocui.Gui, *gocui.View) error{orderKeyLeft, orderKeyRight} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			config := newConfig()
			config.view = config.views["activity"]
			config.view.OrderKey = 5
			config.scrollOffset = 2
			assert.False(t, config.autoScrollToOrderKey, "the flag must start cleared")

			wg.Add(1)
			go func() {
				<-config.viewCh
				close(config.viewCh)
				wg.Done()
			}()

			fn := fnFactory(config)
			assert.NoError(t, fn(nil, nil))
			assert.True(t, config.autoScrollToOrderKey)
			assert.Equal(t, 2, config.scrollOffset, "the handler must not touch the offset")
			wg.Wait()
		})
	}
}

// Test_viewSwitchResetsAutoScrollFlag verifies the common switch path clears the pending
// auto-scroll request: otherwise the first render of the NEW screen would consume it and
// scroll to a sort column the user never chose there.
func Test_viewSwitchResetsAutoScrollFlag(t *testing.T) {
	config := newConfig()
	config.view = config.views["activity"]
	config.autoScrollToOrderKey = true

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		<-config.viewCh
		close(config.viewCh)
		wg.Done()
	}()

	viewSwitchHandler(config, "databases_general")
	wg.Wait()

	assert.False(t, config.autoScrollToOrderKey)
}

// Test_switchViewToProcPidStatResetsAutoScrollFlag verifies the per-process path (which
// bypasses viewSwitchHandler) also clears the pending request. The non-local guard returns
// early without touching the DB, so the reset must sit before the guard to be observable
// without a live PostgreSQL backend.
func Test_switchViewToProcPidStatResetsAutoScrollFlag(t *testing.T) {
	app := &app{
		config: newConfig(),
		db:     &postgres.DB{Local: false}, // remote: early return, no DB access
	}
	app.config.view = app.config.views["activity"]
	app.config.autoScrollToOrderKey = true

	fn := switchViewToProcPidStat(app)
	assert.NoError(t, fn(nil, nil))

	assert.False(t, app.config.autoScrollToOrderKey)
}

// Test_viewSwitchResetsScrollOffset verifies the common switch path resets scroll.
func Test_viewSwitchResetsScrollOffset(t *testing.T) {
	config := newConfig()
	config.view = config.views["activity"]
	config.scrollOffset = 5

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		<-config.viewCh
		close(config.viewCh)
		wg.Done()
	}()

	viewSwitchHandler(config, "databases_general")
	wg.Wait()

	assert.Equal(t, 0, config.scrollOffset)
}

// Test_switchViewToProcPidStatResetsScrollOffset verifies the per-process path
// (which bypasses viewSwitchHandler) also resets scroll. The non-local guard
// returns early without touching the DB or viewCh, so the reset must happen
// before the guard to be observable without a live PostgreSQL backend.
func Test_switchViewToProcPidStatResetsScrollOffset(t *testing.T) {
	app := &app{
		config: newConfig(),
		db:     &postgres.DB{Local: false}, // remote: early return, no DB access
	}
	app.config.view = app.config.views["activity"]
	app.config.scrollOffset = 5

	fn := switchViewToProcPidStat(app)
	assert.NoError(t, fn(nil, nil))

	assert.Equal(t, 0, app.config.scrollOffset)
}

func Test_increaseWidth(t *testing.T) {
	testcases := []struct {
		colsWidth map[int]int
		want      int
	}{
		{colsWidth: map[int]int{0: 10}, want: 14},   // current width 10 chars, want to be 14
		{colsWidth: map[int]int{0: 254}, want: 256}, //  current width 254 chars, want to be 256
	}

	wg := sync.WaitGroup{}

	for i, tc := range testcases {
		t.Run(fmt.Sprintln(i), func(t *testing.T) {
			config := newConfig()
			config.view = config.views["activity"]
			config.view.OrderKey = 0
			config.view.ColsWidth = tc.colsWidth

			wg.Add(1)
			go func() {
				v := <-config.viewCh
				assert.Equal(t, tc.want, v.ColsWidth[0])
				close(config.viewCh)
				wg.Done()
			}()

			fn := increaseWidth(config)
			assert.NoError(t, fn(nil, nil))
		})
		wg.Wait()
	}
}

func Test_decreaseWidth(t *testing.T) {
	testcases := []struct {
		colsWidth map[int]int
		cols      []string
		want      int
	}{
		{colsWidth: map[int]int{0: 18}, cols: []string{"datname"}, want: 14}, // current width 18 chars, want to be 14
		{colsWidth: map[int]int{0: 8}, cols: []string{"datname"}, want: 7},   //  current width 8 chars, want to be 7
	}

	wg := sync.WaitGroup{}
	for i, tc := range testcases {
		t.Run(fmt.Sprintln(i), func(t *testing.T) {
			config := newConfig()
			config.view = config.views["activity"]
			config.view.OrderKey = 0
			config.view.ColsWidth = tc.colsWidth
			config.view.Cols = tc.cols

			wg.Add(1)
			go func() {
				v := <-config.viewCh
				assert.Equal(t, tc.want, v.ColsWidth[0])
				close(config.viewCh)
				wg.Done()
			}()

			fn := decreaseWidth(config)
			assert.NoError(t, fn(nil, nil))
		})
		wg.Wait()
	}
}

func Test_switchSortOrder(t *testing.T) {
	testcases := []struct {
		orderDesc bool
		want      bool
	}{
		{orderDesc: false, want: true},
		{orderDesc: true, want: false},
	}

	wg := sync.WaitGroup{}

	for i, tc := range testcases {
		t.Run(fmt.Sprintln(i), func(t *testing.T) {
			config := newConfig()
			config.view = config.views["activity"]
			config.view.OrderKey = 0
			config.view.OrderDesc = tc.orderDesc

			wg.Add(1)
			go func() {
				v := <-config.viewCh
				assert.Equal(t, tc.want, v.OrderDesc)
				close(config.viewCh)
				wg.Done()
			}()

			fn := switchSortOrder(config)
			assert.NoError(t, fn(nil, nil))
		})
		wg.Wait()
	}
}

// Test_setFilter — each case seeds its own precondition on a fresh view, so the cases
// are order-independent and individually runnable via '-run Test_setFilter/<name>'.
func Test_setFilter(t *testing.T) {
	testcases := []struct {
		name        string
		prefill     string // pattern installed on the ordered column before the case runs
		answer      string
		want        string
		wantFilters int // number of filters left on the view afterwards
	}{
		{name: "set", answer: "example", want: "Filters: ok", wantFilters: 1},
		{name: "clear-existing", prefill: "example", answer: "", want: "Filters: regular expression cleared", wantFilters: 0},
		{name: "clear-absent", answer: "\n", want: "Filters: no filter on this column", wantFilters: 0},
		{name: "invalid-regexp", answer: "[0-", want: "Filters: error parsing regexp: missing closing ]: `[0-`", wantFilters: 0},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			config := newConfig()
			config.view = config.views["activity"]
			config.view.OrderKey = 0
			if tc.prefill != "" {
				config.view.Filters[0] = regexp.MustCompile(tc.prefill)
			}

			assert.Equal(t, tc.want, setFilter(tc.answer, config.view))
			assert.Len(t, config.view.Filters, tc.wantFilters)
		})
	}
}

func Test_isFilterActive(t *testing.T) {
	testcases := []struct {
		name string
		re   *regexp.Regexp
		want bool
	}{
		{name: "nil", re: nil, want: false},
		{name: "empty-pattern", re: regexp.MustCompile(""), want: false},
		{name: "non-empty-pattern", re: regexp.MustCompile("example"), want: true},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isFilterActive(tc.re))
		})
	}
}

func Test_activeFilterCount(t *testing.T) {
	testcases := []struct {
		name    string
		filters map[int]*regexp.Regexp
		want    int
	}{
		{name: "nil-map", filters: nil, want: 0},
		{name: "empty-map", filters: map[int]*regexp.Regexp{}, want: 0},
		{name: "nil-value", filters: map[int]*regexp.Regexp{0: nil}, want: 0},
		{name: "empty-pattern", filters: map[int]*regexp.Regexp{0: regexp.MustCompile("")}, want: 0},
		{
			name:    "two-active",
			filters: map[int]*regexp.Regexp{0: regexp.MustCompile("example"), 3: regexp.MustCompile("test")},
			want:    2,
		},
		{
			name:    "mixed",
			filters: map[int]*regexp.Regexp{0: regexp.MustCompile("example"), 1: nil, 3: regexp.MustCompile("")},
			want:    1,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, activeFilterCount(tc.filters))
		})
	}
}

func Test_clearFilters_noActiveFilters(t *testing.T) {
	config := newConfig()
	config.view = config.views["activity"]

	msg, n := clearAllFilters(config.view)
	assert.Equal(t, "Filters: no active filters", msg)
	assert.Equal(t, 0, n)
	assert.Empty(t, config.view.Filters)

	// viewCh is unbuffered: an erroneous send would block INSIDE the handler and the
	// non-blocking select below would never be reached — the test would hang until the
	// 'go test' timeout instead of failing. Run the handler in a goroutine with a deadline.
	var err error
	done := make(chan struct{})
	go func() { defer close(done); err = clearFilters(config)(nil, nil) }()

	select {
	case <-done:
		// Only this branch is synchronized with the goroutine, so err is read here only.
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("handler blocked sending on viewCh with no filters to clear")
	}

	select {
	case <-config.viewCh:
		t.Fatal("unexpected send on viewCh")
	default:
	}

	assert.Empty(t, config.view.Filters) // the handler left the map alone too
}

func Test_clearFilters_clearsAll(t *testing.T) {
	// Filters live on columns 0 and 3, neither of them is the ordered column — a
	// per-column clear (the old behaviour) would leave both in place.
	newFilteredConfig := func() *config {
		config := newConfig()
		config.view = config.views["activity"]
		config.view.OrderKey = 5
		config.view.Filters[0] = regexp.MustCompile("example")
		config.view.Filters[3] = regexp.MustCompile("test")
		return config
	}

	t.Run("message-and-count", func(t *testing.T) {
		config := newFilteredConfig()

		msg, n := clearAllFilters(config.view)
		assert.Equal(t, "Filters: cleared 2 filter(s)", msg)
		assert.Equal(t, 2, n)
		assert.Empty(t, config.view.Filters)
	})

	t.Run("handler-sends-view", func(t *testing.T) {
		config := newFilteredConfig()

		// Deadline instead of a plain wg.Wait(): if a regression stops the handler from
		// sending, waiting unconditionally would hang the whole package until the
		// 'go test' timeout instead of failing this one case.
		received := make(chan view.View, 1)
		go func() { received <- <-config.viewCh }()

		fn := clearFilters(config)
		assert.NoError(t, fn(nil, nil))

		select {
		case v := <-received:
			assert.Empty(t, v.Filters)
		case <-time.After(time.Second):
			t.Fatal("handler did not send the cleared view on viewCh")
		}

		assert.Empty(t, config.view.Filters)
	})
}

// Test_clearFilters_inPlaceVisibleThroughStoredView catches an implementation which
// assigns a fresh filters map instead of deleting in place: the map header is shared
// with the copy stored in config.views, so a reassignment would leave the stored copy
// with the old filters and they would come back on the next view switch.
func Test_clearFilters_inPlaceVisibleThroughStoredView(t *testing.T) {
	config := newConfig()
	config.view = config.views["activity"]
	config.view.Filters[0] = regexp.MustCompile("example")
	config.views[config.view.Name] = config.view

	received := make(chan view.View, 1)
	go func() { received <- <-config.viewCh }()

	fn := clearFilters(config)
	assert.NoError(t, fn(nil, nil))

	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("handler did not send the cleared view on viewCh")
	}

	assert.Empty(t, config.views["activity"].Filters)
}

func Test_switchViewTo(t *testing.T) {
	app := &app{
		config: newConfig(),
	}
	testcases := []struct {
		current   string
		to        string
		want      string
		pgssAvail bool
	}{
		{current: "activity", to: "databases_general", want: "databases_general"},
		{current: "databases_general", to: "databases_sessions", want: "databases_sessions"},
		{current: "databases_sessions", to: "tables", want: "tables"},
		{current: "tables", to: "indexes", want: "indexes"},
		{current: "indexes", to: "sizes", want: "sizes"},
		{current: "sizes", to: "functions", want: "functions"},
		{current: "sizes", to: "wal", want: "wal"},
		{current: "wal", to: "replication", want: "replication"},
		{current: "replication", to: "statements", want: "statements_timings"},
		{current: "statements_timings", to: "statements", want: "statements_general"},
		{current: "statements_general", to: "statements", want: "statements_io"},
		{current: "statements_io", to: "statements", want: "statements_temp"},
		{current: "statements_temp", to: "statements", want: "statements_local"},
		{current: "statements_local", to: "statements", want: "statements_wal"},
		{current: "statements_wal", to: "statements", want: "statements_jit"},
		{current: "statements_jit", to: "statements", want: "statements_timings"},
		{current: "statements_timings", to: "progress", want: "progress_vacuum"},
		{current: "progress_vacuum", to: "progress", want: "progress_cluster"},
		{current: "progress_cluster", to: "progress", want: "progress_index"},
		{current: "progress_index", to: "progress", want: "progress_analyze"},
		{current: "progress_analyze", to: "progress", want: "progress_basebackup"},
		{current: "progress_basebackup", to: "progress", want: "progress_copy"},
		{current: "progress_copy", to: "progress", want: "progress_vacuum"},
		{current: "activity", to: "statio", want: "stat_io"},
		{current: "stat_io", to: "statio", want: "stat_io_time"},
		{current: "stat_io_time", to: "statio", want: "stat_io"},
		// The 'w' cycle. The first row is the one that proves the dispatch case exists: the other
		// two land on "wal" through walNextView's default arm as well, so they stay green even
		// without the case.
		{current: "wal", to: "wal", want: "archiver"},
		{current: "archiver", to: "wal", want: "wal"},
		{current: "activity", to: "wal", want: "wal"},
	}

	wg := sync.WaitGroup{}
	for i, tc := range testcases {
		t.Run(fmt.Sprintln(i), func(t *testing.T) {
			app.config.view = app.config.views[tc.current]
			app.postgresProps.ExtPGSSSchema = "public"

			wg.Add(1)
			go func() {
				v := <-app.config.viewCh
				assert.Equal(t, tc.want, v.Name)
				wg.Done()
			}()

			fn := switchViewTo(app, tc.to)
			assert.NoError(t, fn(nil, nil))

			// Inside the closure, not after it: the assertion lives in the goroutine above, and
			// waiting outside would let a failing row call t.Errorf on a finished subtest - which
			// panics ("Fail in goroutine after ... has completed") instead of naming the row.
			wg.Wait()
		})
	}
	close(app.config.viewCh)

	// Attempt to switch when pg_stat_statements is not available (should stay on current)
	app.config.view = app.config.views["databases_general"]
	app.postgresProps.ExtPGSSSchema = ""

	fn := switchViewTo(app, "statements")
	assert.NoError(t, fn(nil, nil))
	assert.Equal(t, "databases_general", app.config.view.Name)
}

func Test_databasesNextView(t *testing.T) {
	testcases := []struct {
		current string
		want    string
	}{
		{current: "databases_general", want: "databases_sessions"},
		{current: "databases_sessions", want: "databases_general"},
		{current: "unknown", want: "databases_general"},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, databasesNextView(tc.current))
	}
}

func Test_statioNextView(t *testing.T) {
	testcases := []struct {
		current string
		want    string
	}{
		{current: "stat_io", want: "stat_io_time"},
		{current: "stat_io_time", want: "stat_io"},
		{current: "unknown", want: "stat_io"},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, statioNextView(tc.current))
	}
}

func Test_walNextView(t *testing.T) {
	testcases := []struct {
		current string
		want    string
	}{
		{current: "wal", want: "archiver"},
		{current: "archiver", want: "wal"},
		{current: "unknown", want: "wal"},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, walNextView(tc.current))
	}
}

func Test_statementsNextView(t *testing.T) {
	testcases := []struct {
		current string
		want    string
	}{
		{current: "statements_timings", want: "statements_general"},
		{current: "statements_general", want: "statements_io"},
		{current: "statements_io", want: "statements_temp"},
		{current: "statements_temp", want: "statements_local"},
		{current: "statements_local", want: "statements_wal"},
		{current: "statements_wal", want: "statements_jit"},
		{current: "statements_jit", want: "statements_timings"},
		{current: "unknown", want: "statements_timings"},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, statementsNextView(tc.current))
	}
}

func Test_progressNextView(t *testing.T) {
	testcases := []struct {
		current string
		want    string
	}{
		{current: "progress_vacuum", want: "progress_cluster"},
		{current: "progress_cluster", want: "progress_index"},
		{current: "progress_index", want: "progress_analyze"},
		{current: "progress_analyze", want: "progress_basebackup"},
		{current: "progress_basebackup", want: "progress_copy"},
		{current: "progress_copy", want: "progress_vacuum"},
		{current: "unknown", want: "progress_vacuum"},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, progressNextView(tc.current))
	}
}

func Test_toggleSysTables(t *testing.T) {
	testcases := []struct {
		name    string
		current string
		want    string
		nowant  string
	}{
		{name: "tables", current: "user", want: "pg_stat_all", nowant: "pg_stat_user"},
		{name: "indexes", current: "user", want: "pg_stat_all", nowant: "pg_stat_user"},
		{name: "sizes", current: "user", want: "pg_stat_all", nowant: "pg_stat_user"},
		{name: "tables", current: "all", want: "pg_stat_user", nowant: "pg_stat_all"},
		{name: "indexes", current: "all", want: "pg_stat_user", nowant: "pg_stat_all"},
		{name: "sizes", current: "all", want: "pg_stat_user", nowant: "pg_stat_all"},
	}

	config := newConfig()
	wg := sync.WaitGroup{}

	for i, tc := range testcases {
		t.Run(fmt.Sprintln(i), func(t *testing.T) {
			config.view = config.views[tc.name]
			config.queryOptions.ViewType = tc.current

			wg.Add(1)
			go func() {
				v := <-config.viewCh
				assert.Contains(t, v.Query, tc.want)
				assert.NotContains(t, v.Query, tc.nowant)
				wg.Done()
			}()

			fn := toggleSysTables(config)
			assert.NoError(t, fn(nil, nil))
		})
		wg.Wait()
	}

	// when current view is not in ('tables','indexes','sizes') nothing changed.
	config.queryOptions.ViewType = "user"
	config.view = config.views["activity"]
	fn := toggleSysTables(config)
	assert.NoError(t, fn(nil, nil))
	assert.Equal(t, "user", config.queryOptions.ViewType)

	close(config.viewCh)
}

func Test_changeQueryAge(t *testing.T) {
	testcases := []struct {
		answer string
		want   string
		nowant string
	}{
		{answer: "00:01:00", want: "00:01:00", nowant: "00:00:00"},
		{answer: "", want: "00:00:00", nowant: "00:01:00"},
	}

	config := newConfig()
	config.view = config.views["activity"]
	wg := sync.WaitGroup{}

	for i, tc := range testcases {
		t.Run(fmt.Sprintln(i), func(t *testing.T) {
			wg.Add(1)
			go func() {
				v := <-config.viewCh
				assert.Contains(t, v.Query, tc.answer)
				assert.NotContains(t, v.Query, tc.nowant)
				wg.Done()
			}()

			got := changeQueryAge(tc.answer, config)
			assert.Equal(t, "Activity age: set "+tc.want, got)
		})
		wg.Wait()
	}

	t.Run("invalid time", func(t *testing.T) {
		config.queryOptions.QueryAgeThresh = "01:02:03"
		got := changeQueryAge("invalid", config)
		assert.Equal(t, "Activity age: do nothing, invalid input", got)
		assert.Equal(t, "01:02:03", config.queryOptions.QueryAgeThresh) // age should be the same as before calling changeQueryAge.
	})

	t.Run("break formatting", func(t *testing.T) {
		config.queryOptions.QueryAgeThresh = "11:12:13"
		config.view.QueryTmpl = "{{" // break query template leads breaking query formatting
		got := changeQueryAge("00:00:00", config)
		assert.Equal(t, "Activity age: do nothing, template: query:1: unclosed action", got)
		assert.Equal(t, "11:12:13", config.queryOptions.QueryAgeThresh) // age should be the same as before calling changeQueryAge.
	})

	close(config.viewCh)
}

func Test_parseHumanTimeString(t *testing.T) {
	testcases := []struct {
		valid bool
		t     string
	}{
		{valid: true, t: "00:00:00"},
		{valid: true, t: "01:01:01"},
		{valid: true, t: "01:01:01.100"},
		{valid: false, t: "invalid"},
		{valid: false, t: "01"},
		{valid: false, t: "01:"},
		{valid: false, t: "01:01"},
		{valid: false, t: "01:01:01:"},
		{valid: false, t: "01:01:01."},
		{valid: false, t: "-01:00:00"},
		{valid: false, t: "01:-01:00"},
		{valid: false, t: "01:01:-01"},
		{valid: false, t: "01:01:01.-100"},
		{valid: false, t: "25:00:00"},
		{valid: false, t: "01:60:00"},
		{valid: false, t: "01:01:60"},
		{valid: false, t: "01:01:01.1000000"},
	}

	for _, tc := range testcases {
		if tc.valid {
			assert.NoError(t, parseHumanTimeString(tc.t))
		} else {
			assert.Error(t, parseHumanTimeString(tc.t))
		}
	}
}

func Test_toggleIdleConns(t *testing.T) {
	testcases := []struct {
		showNoIdleInitial bool
	}{
		{showNoIdleInitial: true},  // don't show idle -> show idle
		{showNoIdleInitial: false}, // show idle -> don't show idle
	}

	config := newConfig()
	config.view = config.views["activity"]

	wg := sync.WaitGroup{}

	for i, tc := range testcases {
		t.Run(fmt.Sprintln(i), func(t *testing.T) {
			wg.Add(1)
			go func() {
				v := <-config.viewCh
				if tc.showNoIdleInitial {
					assert.False(t, config.queryOptions.ShowNoIdle)
					assert.NotContains(t, v.Query, "AND state != 'idle'")
				} else {
					assert.True(t, config.queryOptions.ShowNoIdle)
					assert.Contains(t, v.Query, "AND state != 'idle'")
				}
				wg.Done()
			}()

			config.queryOptions.ShowNoIdle = tc.showNoIdleInitial
			fn := toggleIdleConns(config)
			assert.NoError(t, fn(nil, nil))
			wg.Wait()
		})
	}

	// test attempt to change in other than activity view (should be unchanged).
	config.view = config.views["databases_general"]
	config.queryOptions.ShowNoIdle = true
	fn := toggleIdleConns(config)
	assert.NoError(t, fn(nil, nil))
	assert.True(t, config.queryOptions.ShowNoIdle) // is not changed

	close(config.viewCh)
}

func Test_changeRefresh(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		config := newConfig()
		config.view = config.views["activity"]
		wg := sync.WaitGroup{}

		wg.Add(1)
		go func() {
			v := <-config.viewCh
			assert.Equal(t, v.Refresh, 5*time.Second)
			wg.Done()
		}()

		assert.Equal(t, "Refresh: ok", changeRefresh("5", config))
		// The durable copy used by the header is kept in sync with the value sent to the collector.
		assert.Equal(t, 5*time.Second, config.refresh)
		wg.Wait()
		close(config.viewCh)
	})

	// test invalid input
	t.Run("invalid input", func(t *testing.T) {
		config := newConfig()
		config.view = config.views["activity"]

		testcases := map[string]string{
			"":    "Refresh: do nothing",
			"a":   "Refresh: do nothing, invalid input",
			"0.5": "Refresh: do nothing, invalid input",
			"-1":  "Refresh: input value should be between 1 and 300",
			"0":   "Refresh: input value should be between 1 and 300",
			"301": "Refresh: input value should be between 1 and 300",
		}
		for k, v := range testcases {
			config.view.Refresh = 1 * time.Second
			config.refresh = 2 * time.Second
			assert.Equal(t, v, changeRefresh(k, config))
			assert.Equal(t, 1*time.Second, config.view.Refresh) // should not be 0
			// Invalid input must not change what the header displays.
			assert.Equal(t, 2*time.Second, config.refresh)
		}
	})
}

// Test_decreaseWidthNoColumnMetadata covers pressing the width-down key before the first frame has
// ever been rendered: config.view.Cols is populated only on the render path, so view.New() leaves
// it nil and indexing it would panic inside a key handler, which gocui does not recover.
//
// The handler runs in a goroutine and the outcome is taken with a select over three channels: an
// implementation that "narrowed anyway" would push on the unbuffered viewCh and hang the whole
// package instead of failing this test.
func Test_decreaseWidthNoColumnMetadata(t *testing.T) {
	config := newConfig()
	config.view = config.views["activity"]
	config.view.OrderKey = 0
	config.view.ColsWidth = map[int]int{0: 18}

	assert.Nil(t, config.view.Cols, "precondition: a view carries no column names until it is rendered")

	done := make(chan error, 1)
	go func() { done <- decreaseWidth(config)(nil, nil) }()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-config.viewCh:
		t.Fatal("decreaseWidth must not ask for a frame when there is nothing to narrow")
	case <-time.After(2 * time.Second):
		t.Fatal("decreaseWidth neither returned nor pushed on viewCh")
	}

	assert.Equal(t, 18, config.view.ColsWidth[0], "the width must be left untouched")
}

// Test_decreaseWidthStaleColumnMetadata covers the same guard with a non-empty Cols that is too
// short - column names left over from a screen with more columns. A 'Cols != nil' check would let
// this case through, so the guard has to be a bounds check.
func Test_decreaseWidthStaleColumnMetadata(t *testing.T) {
	config := newConfig()
	config.view = config.views["activity"]
	config.view.OrderKey = 3
	config.view.ColsWidth = map[int]int{3: 18}
	config.view.Cols = []string{"datname"}

	done := make(chan error, 1)
	go func() { done <- decreaseWidth(config)(nil, nil) }()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-config.viewCh:
		t.Fatal("decreaseWidth must not ask for a frame when the column metadata is stale")
	case <-time.After(2 * time.Second):
		t.Fatal("decreaseWidth neither returned nor pushed on viewCh")
	}

	assert.Equal(t, 18, config.view.ColsWidth[3], "the width must be left untouched")
}
