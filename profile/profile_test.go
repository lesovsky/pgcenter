package profile

import (
	"bytes"
	"database/sql"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
	"time"
)

func Test_newStatsStore(t *testing.T) {
	s := newStatsStore()
	assert.NotNil(t, s.durations)
	assert.NotNil(t, s.ratios)
}

func Test_resetStatsStore(t *testing.T) {
	s := stats{
		durations: map[string]float64{
			"Test.Entry3": 140,
			"Test.Entry2": 330,
			"Test.Entry1": 1020,
			"Running":     8510,
		},
		ratios: map[string]float64{
			"Test.Entry2": 3.3,
			"Running":     85.1,
			"Test.Entry3": 1.4,
			"Test.Entry1": 10.2,
		},
	}

	resetStatsStore(s)
	assert.Equal(t, 0, len(s.durations))
	assert.Equal(t, 0, len(s.ratios))
}

func Test_profileLoop(t *testing.T) {
	target, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	var pid int
	err = target.QueryRow("SELECT pg_backend_pid()").Scan(&pid)
	assert.NoError(t, err)

	db, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	// go sleep in profiled connection
	go func() {
		// waiting for to start profiling outside this goroutine
		time.Sleep(time.Second)

		// run query 1
		_, err = target.Exec("SELECT 1, pg_sleep(1)")
		assert.NoError(t, err)

		// immediately run query 2
		_, err = target.Exec("SELECT 2, pg_sleep(1)")
		assert.NoError(t, err)

		// be idle for a bit
		time.Sleep(200 * time.Millisecond)

		// run query 3
		_, err = target.Exec("SELECT 3, pg_sleep(1)")
		assert.NoError(t, err)

		// close DB connection - profiler will exit.
		target.Close()
	}()

	var buf bytes.Buffer
	err = profileLoop(&buf, db, Config{Pid: pid, Frequency: 50 * time.Millisecond, Strsize: 64}, nil)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), fmt.Sprintf("LOG: Profiling process %d with 50ms sampling", pid))
	assert.Contains(t, buf.String(), "% time      seconds wait_event                     query: SELECT 1, pg_sleep(1)")
	assert.Contains(t, buf.String(), "% time      seconds wait_event                     query: SELECT 2, pg_sleep(1)")
	assert.Contains(t, buf.String(), "% time      seconds wait_event                     query: SELECT 3, pg_sleep(1)")
	assert.Contains(t, buf.String(), "Timeout.PgSleep")
	assert.Contains(t, buf.String(), fmt.Sprintf("LOG: Stop profiling, no process with pid %d", pid))
	//fmt.Println(buf.String())
	db.Close()
}

func Test_parseActivitySnapshot(t *testing.T) {
	res := stat.PGresult{
		Valid: true, Ncols: 6, Nrows: 3, Cols: []string{"pid", "query_duration", "state_change_time", "state", "wait_entry", "query"},
		Values: [][]sql.NullString{
			{
				{String: "123456", Valid: true}, {String: "2.000", Valid: true},
				{String: "2021-01-01T00:00:00.000+05:00", Valid: true}, {String: "active", Valid: true},
				{String: "", Valid: true}, {String: "SELECT 1", Valid: true},
			},
			{
				{String: "123457", Valid: true}, {String: "1.000", Valid: true},
				{String: "2021-01-01T00:00:01.000+05:00", Valid: true}, {String: "active", Valid: true},
				{String: "example:entry1", Valid: true}, {String: "SELECT 1", Valid: true},
			},
			{
				{String: "123458", Valid: true}, {String: "0.500", Valid: true},
				{String: "2021-01-01T00:00:01.500+05:00", Valid: true}, {String: "active", Valid: true},
				{String: "example:entry2", Valid: true}, {String: "SELECT 1", Valid: true},
			},
		},
	}

	want := map[int]profileStat{
		123456: {queryDurationSec: 2.000, changeStateTime: "2021-01-01T00:00:00.000+05:00", state: "active", queryText: "SELECT 1"},
		123457: {queryDurationSec: 1.000, changeStateTime: "2021-01-01T00:00:01.000+05:00", state: "active", waitEntry: "example:entry1", queryText: "SELECT 1"},
		123458: {queryDurationSec: 0.500, changeStateTime: "2021-01-01T00:00:01.500+05:00", state: "active", waitEntry: "example:entry2", queryText: "SELECT 1"},
	}

	assert.Equal(t, want, parseActivitySnapshot(res))
}

func Test_countWaitEvents(t *testing.T) {
	testcases := []map[int]profileStat{
		{
			1085637: {queryDurationSec: 0.000},
			1085638: {queryDurationSec: 0.000},
			1085639: {queryDurationSec: 0.000},
		},
		{
			1085637: {queryDurationSec: 0.500, waitEntry: "example::event1"},
			1085638: {queryDurationSec: 0.500, waitEntry: "example::event1"},
			1085639: {queryDurationSec: 0.500, waitEntry: "example::event2"},
		},
		{
			1085637: {queryDurationSec: 1.000, waitEntry: "example::event1"},
			1085638: {queryDurationSec: 1.000, waitEntry: "example::event2"},
			1085640: {queryDurationSec: 0.500, waitEntry: "example::event2"},
		},
		{
			1085637: {queryDurationSec: 1.500, waitEntry: "example::event3"},
			1085638: {queryDurationSec: 1.500, waitEntry: "example::event3"},
			1085640: {queryDurationSec: 1.000}, // count as Running
			1085641: {queryDurationSec: 0.500}, // count as Running
		},
	}

	want := stats{
		real:        1.5,
		accumulated: 5,
		durations: map[string]float64{
			"Running":         1,
			"example::event1": 1.5,
			"example::event2": 1.5,
			"example::event3": 1,
		},
		ratios: map[string]float64{
			"Running":         20,
			"example::event1": 30,
			"example::event2": 30,
			"example::event3": 20,
		},
	}

	s := stats{
		durations: map[string]float64{},
		ratios:    map[string]float64{},
	}

	for i := 1; i < len(testcases); i++ {
		s = countWaitEvents(s, 1085637, testcases[i], testcases[i-1])
	}

	assert.Equal(t, want, s)

}

func Test_printHeader(t *testing.T) {
	s := profileStat{queryText: "SELECT f1, f2, f3 FROM t1, t2 WHERE t1.f1 = t2.f1"}

	want, err := os.ReadFile("testdata/profile_header.golden")
	assert.NoError(t, err)

	var buf bytes.Buffer
	assert.NoError(t, printHeader(&buf, s, 64))
	assert.Equal(t, string(want), buf.String())
}

func Test_printStat(t *testing.T) {
	s := stats{
		real:        10000,
		accumulated: 30000,
		durations: map[string]float64{
			"Test.Entry3": 140,
			"Test.Entry2": 330,
			"Test.Entry1": 1020,
			"Running":     8510,
		},
		ratios: map[string]float64{
			"Test.Entry2": 3.3,
			"Running":     85.1,
			"Test.Entry3": 1.4,
			"Test.Entry1": 10.2,
		},
	}

	want, err := os.ReadFile("testdata/profile_stats.golden")
	assert.NoError(t, err)

	buf := bytes.NewBuffer([]byte{})
	assert.NoError(t, printStat(buf, s))
	assert.Equal(t, string(want), buf.String())

	// Test with empty stats.
	buf = bytes.NewBuffer([]byte{})
	assert.NoError(t, printStat(buf, stats{durations: map[string]float64{}}))
	assert.Equal(t, "", buf.String())
}

func Test_truncateQuery(t *testing.T) {
	testcases := []struct {
		in    string
		limit int
		want  string
	}{
		{in: "SELECT version();", limit: 10, want: "SELECT ver"},
		{in: "SELECT version();", limit: 20, want: "SELECT version();"},
	}

	for _, tc := range testcases {
		got := truncateQuery(tc.in, tc.limit)
		assert.Equal(t, tc.want, got)
	}
}

func Test_selectQuery(t *testing.T) {
	testcases := []struct {
		exclusive bool
		version   int
		want      string
	}{
		{exclusive: true, version: 120000, want: fmt.Sprintf(exclusiveQuery, 123456)},
		{exclusive: true, version: 130000, want: fmt.Sprintf(exclusiveQuery, 123456)},
		{exclusive: false, version: 120000, want: fmt.Sprintf(exclusiveQuery, 123456)},
		{exclusive: false, version: 130000, want: fmt.Sprintf(inclusiveQuery, 123456, 123456)},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, selectQuery(123456, tc.exclusive, tc.version))
	}
}
