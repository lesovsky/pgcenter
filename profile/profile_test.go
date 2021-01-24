package profile

import (
	"bytes"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"testing"
	"time"
)

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
		_, err = target.Exec("SELECT 1, pg_sleep(1)")
		assert.NoError(t, err)

		_, err = target.Exec("SELECT 2, pg_sleep(1)")
		assert.NoError(t, err)

		_, err = target.Exec("SELECT 3, pg_sleep(1)")
		assert.NoError(t, err)
		target.Close()
	}()

	var buf bytes.Buffer
	err = profileLoop(&buf, db, Config{Pid: pid, Frequency: 50 * time.Millisecond, Strsize: 64}, nil)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), fmt.Sprintf("LOG: Profiling process %d with 50ms sampling", pid))
	assert.Contains(t, buf.String(), "% time      seconds wait_event                     query: SELECT 1, pg_sleep(1)")
	assert.Contains(t, buf.String(), "% time      seconds wait_event                     query: SELECT 2, pg_sleep(1)")
	assert.Contains(t, buf.String(), "Timeout.PgSleep")
	assert.Contains(t, buf.String(), fmt.Sprintf("LOG: Stop profiling, process with pid %d doesn't exist (no rows in result set)", pid))
	fmt.Println(buf.String())
	db.Close()

}

func Test_getProfileSnapshot(t *testing.T) {
	target, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	var pid int
	err = target.QueryRow("SELECT pg_backend_pid()").Scan(&pid)
	assert.NoError(t, err)

	db, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	// go sleep in profiled connection
	go func() {
		_, err := target.Exec("SELECT pg_sleep(1)")
		assert.NoError(t, err)
	}()

	// try profile 'target' backend
	got, err := getProfileSnapshot(db, pid)
	assert.NoError(t, err)
	assert.NotNil(t, got)
	assert.Equal(t, "SELECT pg_sleep(1)", got.queryText)

	db.Close()
	target.Close()
}

func Test_countWaitings(t *testing.T) {
	// wait events distribution at the beginning: query was working 1 second, running 0.5s (50%), test entry 0.5s (50%)
	s := stats{
		durations: map[string]float64{
			"Running":     0.5,
			"Test.Entry1": 0.5,
		},
		ratios: map[string]float64{
			"Running":     50,
			"Test.Entry1": 50,
		},
	}
	ps := profileStat{
		queryDurationSec: 1.0, // query was working 1 second
		state:            "active",
		waitEntry:        "Test.Entry1",
		queryText:        "SELECT 1",
	}

	// current snapshot - query waits 1 extra second in Test.Entry1
	cs := profileStat{
		queryDurationSec: 2.0, // +1 second
		state:            "active",
		waitEntry:        "Test.Entry1",
		queryText:        "SELECT 1",
	}

	// all waiting time accounted for Test.Entry1 - running 0.5s (25%), wait entry - 1.5s (75%)
	want := stats{
		durations: map[string]float64{
			"Running":     0.5,
			"Test.Entry1": 1.5,
		},
		ratios: map[string]float64{
			"Running":     25,
			"Test.Entry1": 75,
		},
	}

	got := countWaitings(s, cs, ps)
	assert.Equal(t, want, got)

	// swap prev with curr and update curr with new value - query is running 1 extra second (running means no wait)
	ps = cs
	cs = profileStat{
		queryDurationSec: 3.0, // +1 second
		state:            "active",
		waitEntry:        "", // no wait
		queryText:        "SELECT 1",
	}

	// all running time should be accounted to 'Running' - running 1.5s (50%), wait entry - 1.5s (50%)
	want = stats{
		durations: map[string]float64{
			"Running":     1.5,
			"Test.Entry1": 1.5,
		},
		ratios: map[string]float64{
			"Running":     50,
			"Test.Entry1": 50,
		},
	}

	got = countWaitings(s, cs, ps)
	assert.Equal(t, want, got)
}

func Test_resetCounters(t *testing.T) {
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

	resetCounters(s)
	assert.Equal(t, 0, len(s.durations))
	assert.Equal(t, 0, len(s.ratios))
}

func Test_printHeader(t *testing.T) {
	stat := profileStat{queryText: "SELECT f1, f2, f3 FROM t1, t2 WHERE t1.f1 = t2.f1"}

	want, err := ioutil.ReadFile("testdata/profile_header.golden")
	assert.NoError(t, err)

	var buf bytes.Buffer
	assert.NoError(t, printHeader(&buf, stat, 64))
	assert.Equal(t, string(want), buf.String())
}

func Test_printStat(t *testing.T) {
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

	want, err := ioutil.ReadFile("testdata/profile_stats.golden")
	assert.NoError(t, err)

	var buf bytes.Buffer
	assert.NoError(t, printStat(&buf, s))
	assert.Equal(t, string(want), buf.String())
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
