package top

import (
	"fmt"
	"github.com/stretchr/testify/assert"
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

func Test_setFilter(t *testing.T) {
	testcases := []struct {
		answer string
		want   string
	}{
		{answer: "example", want: "Filters: ok"},
		{answer: "", want: "Filters: regular expression cleared"},
		{answer: "\n", want: "Filters: regular expression cleared"},
		{answer: "[0-", want: "Filters: error parsing regexp: missing closing ]: `[0-`"},
	}

	config := newConfig()
	config.view = config.views["activity"]
	config.view.OrderKey = 0

	for _, tc := range testcases {
		assert.Equal(t, tc.want, setFilter(tc.answer, config.view))
	}
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
		{current: "activity", to: "databases", want: "databases"},
		{current: "databases", to: "tables", want: "tables"},
		{current: "tables", to: "indexes", want: "indexes"},
		{current: "indexes", to: "sizes", want: "sizes"},
		{current: "sizes", to: "functions", want: "functions"},
		{current: "functions", to: "replication", want: "replication"},
		{current: "replication", to: "statements", want: "statements_timings"},
		{current: "statements_timings", to: "statements", want: "statements_general"},
		{current: "statements_general", to: "statements", want: "statements_io"},
		{current: "statements_io", to: "statements", want: "statements_temp"},
		{current: "statements_temp", to: "statements", want: "statements_local"},
		{current: "statements_local", to: "statements", want: "statements_timings"},
		{current: "statements_timings", to: "progress", want: "progress_vacuum"},
		{current: "progress_vacuum", to: "progress", want: "progress_cluster"},
		{current: "progress_cluster", to: "progress", want: "progress_index"},
		{current: "progress_index", to: "progress", want: "progress_vacuum"},
	}

	wg := sync.WaitGroup{}
	for i, tc := range testcases {
		t.Run(fmt.Sprintln(i), func(t *testing.T) {
			app.config.view = app.config.views[tc.current]
			app.postgresProps.ExtPGSSAvail = true

			wg.Add(1)
			go func() {
				v := <-app.config.viewCh
				assert.Equal(t, tc.want, v.Name)
				wg.Done()
			}()

			fn := switchViewTo(app, tc.to)
			assert.NoError(t, fn(nil, nil))
		})
		wg.Wait()
	}
	close(app.config.viewCh)

	// Attempt to switch when pg_stat_statements is not available (should stay on current)
	app.config.view = app.config.views["databases"]
	app.postgresProps.ExtPGSSAvail = false

	fn := switchViewTo(app, "statements")
	assert.NoError(t, fn(nil, nil))
	assert.Equal(t, "databases", app.config.view.Name)
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
		assert.Equal(t, "Activity age: do nothing, template: query:1: unexpected unclosed action in command", got)
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
	config.view = config.views["databases"]
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
			assert.Equal(t, v, changeRefresh(k, config))
			assert.Equal(t, 1*time.Second, config.view.Refresh) // should not be 0
		}
	})
}
