package stat

import (
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/view"
	"github.com/stretchr/testify/assert"
	"regexp"
	"testing"
	"time"
)

func TestNewCollector(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	c, err := NewCollector(conn)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	conn.Close()
	c, err = NewCollector(conn)
	assert.Error(t, err)
	assert.Nil(t, c)
}

func TestCollector_Update(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)
	defer conn.Close()

	props, err := GetPostgresProperties(conn)
	assert.NoError(t, err)

	views := view.Views{
		"activity": {
			Name:      "activity",
			QueryTmpl: query.PgStatActivityDefault,
			DiffIntvl: [2]int{0, 0},
			Ncols:     14,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show activity statistics",
			Filters:   map[int]*regexp.Regexp{},
			Refresh:   1 * time.Second,
		},
	}
	opts := query.NewOptions(props.VersionNum, props.Recovery, props.GucTrackCommitTimestamp, 256, "public")
	assert.NoError(t, views.Configure(opts))

	c, err := NewCollector(conn)
	assert.NoError(t, err)
	assert.NotNil(t, c)
	c.config.collectExtra = CollectDiskstats

	stat, err := c.Update(conn, views["activity"], time.Second)
	assert.NoError(t, err)
	assert.NotNil(t, stat)

	assert.NotEqual(t, float64(0), stat.System.LoadAvg.One)
	assert.NotEqual(t, float64(0), stat.System.Meminfo.MemUsed)
	assert.NotEqual(t, float64(0), stat.System.CPUStat.User)
	assert.NotEqual(t, 0, len(stat.System.Diskstats))
	assert.NotEqual(t, float64(0), stat.Pgstat.Activity.ConnTotal)
	assert.True(t, stat.Pgstat.Result.Valid)
	assert.NotEqual(t, 0, len(stat.Pgstat.Result.Values))
	assert.NotEqual(t, 0, len(stat.Pgstat.Result.Cols))
}

func TestCollector_Update_VerboseAggregates(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)
	defer conn.Close()

	props, err := GetPostgresProperties(conn)
	assert.NoError(t, err)

	baseView := view.View{
		Name:      "activity",
		QueryTmpl: query.PgStatActivityDefault,
		DiffIntvl: [2]int{0, 0},
		Ncols:     14,
		OrderKey:  0,
		OrderDesc: true,
		ColsWidth: map[int]int{},
		Msg:       "Show activity statistics",
		Filters:   map[int]*regexp.Regexp{},
		Refresh:   1 * time.Second,
	}
	opts := query.NewOptions(props.VersionNum, props.Recovery, props.GucTrackCommitTimestamp, 256, "public")

	// Verbose OFF: overview aggregates must not be collected (compact path untouched).
	t.Run("verbose_off", func(t *testing.T) {
		c, err := NewCollector(conn)
		assert.NoError(t, err)

		v := baseView
		v.Verbose = false
		views := view.Views{"activity": v}
		assert.NoError(t, views.Configure(opts))

		stat, err := c.Update(conn, views["activity"], time.Second)
		assert.NoError(t, err)
		assert.False(t, stat.Pgstat.Overview.Valid, "overview must not be collected when verbose is off")
	})

	// Verbose ON: overview aggregates are collected and populated.
	t.Run("verbose_on", func(t *testing.T) {
		c, err := NewCollector(conn)
		assert.NoError(t, err)

		v := baseView
		v.Verbose = true
		views := view.Views{"activity": v}
		assert.NoError(t, views.Configure(opts))
		v = views["activity"]

		// First tick: no prev -> rates n/a but struct is valid.
		stat, err := c.Update(conn, v, time.Second)
		assert.NoError(t, err)
		assert.True(t, stat.Pgstat.Overview.Valid)
		assert.False(t, stat.Pgstat.Overview.HasPrev)
		assert.GreaterOrEqual(t, stat.Pgstat.Overview.DatabasesCount, int64(1))

		// Second tick: prev exists -> rates computed.
		stat2, err := c.Update(conn, v, time.Second)
		assert.NoError(t, err)
		assert.True(t, stat2.Pgstat.Overview.Valid)
		assert.True(t, stat2.Pgstat.Overview.HasPrev)
	})
}

func TestCollector_collectDiskstats(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)
	defer conn.Close()

	c, err := NewCollector(conn)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	diskstats, err := c.collectDiskstats(conn)
	assert.NoError(t, err)
	assert.NotNil(t, diskstats)
	assert.Greater(t, len(diskstats), 0)
}

func TestCollector_collectNetdevs(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)
	defer conn.Close()

	c, err := NewCollector(conn)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	netdevs, err := c.collectNetdevs(conn)
	assert.NoError(t, err)
	assert.NotNil(t, netdevs)
	assert.Greater(t, len(netdevs), 0)
}

func Test_readUptimeLocal(t *testing.T) {
	ticks, err := GetSysticksLocal()
	assert.NoError(t, err)
	assert.NotEqual(t, float64(0), ticks)

	got, err := readUptimeLocal("testdata/proc/uptime.golden", ticks)
	assert.NoError(t, err)
	assert.Equal(t, float64(170191868), got)

	_, err = readUptimeLocal("testdata/proc/stat.golden", ticks)
	assert.Error(t, err)
}

func TestGetSysticksLocal(t *testing.T) {
	ticks, err := GetSysticksLocal()
	assert.NoError(t, err)
	assert.NotEqual(t, float64(0), ticks)
	assert.Greater(t, ticks, float64(0))
}

func Test_sValue(t *testing.T) {
	testcases := []struct {
		prev  float64
		curr  float64
		itv   float64
		ticks float64
		want  float64
	}{
		{prev: 1000, curr: 2000, itv: 100, ticks: 100, want: 1000}, // delta 1000 per second within 1 second
		{prev: 1000, curr: 5000, itv: 100, ticks: 100, want: 4000}, // delta 4000 per second within 1 second
		{prev: 1000, curr: 5000, itv: 400, ticks: 100, want: 1000}, // delta 1000 per second within 4 second
		{prev: 2000, curr: 1000, want: 0},                          // nothing, current less than previous
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, sValue(tc.prev, tc.curr, tc.itv, tc.ticks))
	}
}

func TestCollectorResetClearsPIDMaps(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)
	defer conn.Close()

	c, err := NewCollector(conn)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	// All four PID maps must be initialized as non-nil empty maps.
	assert.NotNil(t, c.prevProcPidStats)
	assert.NotNil(t, c.currProcPidStats)
	assert.NotNil(t, c.prevProcPidIO)
	assert.NotNil(t, c.currProcPidIO)
	assert.Equal(t, 0, len(c.prevProcPidStats))
	assert.Equal(t, 0, len(c.currProcPidStats))
	assert.Equal(t, 0, len(c.prevProcPidIO))
	assert.Equal(t, 0, len(c.currProcPidIO))

	// Populate the maps directly via struct field access (same-package test).
	c.prevProcPidStats[1] = ProcPidStat{Utime: 10, Stime: 5}
	c.currProcPidStats[1] = ProcPidStat{Utime: 20, Stime: 10}
	c.prevProcPidIO[1] = ProcPidIO{ReadBytes: 100, WriteBytes: 200}
	c.currProcPidIO[1] = ProcPidIO{ReadBytes: 300, WriteBytes: 400}

	assert.Equal(t, 1, len(c.prevProcPidStats))
	assert.Equal(t, 1, len(c.currProcPidStats))
	assert.Equal(t, 1, len(c.prevProcPidIO))
	assert.Equal(t, 1, len(c.currProcPidIO))

	// Reset must clear all four maps but keep them non-nil.
	c.Reset()

	assert.NotNil(t, c.prevProcPidStats)
	assert.NotNil(t, c.currProcPidStats)
	assert.NotNil(t, c.prevProcPidIO)
	assert.NotNil(t, c.currProcPidIO)
	assert.Equal(t, 0, len(c.prevProcPidStats))
	assert.Equal(t, 0, len(c.currProcPidStats))
	assert.Equal(t, 0, len(c.prevProcPidIO))
	assert.Equal(t, 0, len(c.currProcPidIO))
}

func TestCollectorUpdateNoEnrichment(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)
	defer conn.Close()

	props, err := GetPostgresProperties(conn)
	assert.NoError(t, err)

	views := view.Views{
		"activity": {
			Name:      "activity",
			QueryTmpl: query.PgStatActivityDefault,
			DiffIntvl: [2]int{0, 0},
			Ncols:     14,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show activity statistics",
			Filters:   map[int]*regexp.Regexp{},
			Refresh:   1 * time.Second,
			// CollectExtra not set -> CollectNone (0), no enrichment.
		},
	}
	opts := query.NewOptions(props.VersionNum, props.Recovery, props.GucTrackCommitTimestamp, 256, "public")
	assert.NoError(t, views.Configure(opts))

	c, err := NewCollector(conn)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	s, err := c.Update(conn, views["activity"], time.Second)
	assert.NoError(t, err)
	// With CollectExtra==CollectNone, result must NOT be the 19-col procpidstat shape.
	assert.NotEqual(t, 19, s.Pgstat.Result.Ncols)
	// PID maps stay empty when enrichment is not active.
	assert.Equal(t, 0, len(c.currProcPidStats))
	assert.Equal(t, 0, len(c.currProcPidIO))
}

func TestCollectorUpdateProcPidStat19Cols(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)
	defer conn.Close()

	props, err := GetPostgresProperties(conn)
	assert.NoError(t, err)

	views := view.Views{
		"procpidstat": {
			Name:               "procpidstat",
			QueryTmpl:          query.PgStatActivityProcPidStat,
			DiffIntvl:          [2]int{0, 0},
			Ncols:              19,
			OrderKey:           0,
			OrderDesc:          false,
			ColsWidth:          map[int]int{},
			Msg:                "Show per-process system stats",
			Filters:            map[int]*regexp.Regexp{},
			Refresh:            1 * time.Second,
			CollectExtra:       CollectProcPidStat,
			IOAvailable:        true,
			DelayAcctAvailable: true,
		},
	}
	opts := query.NewOptions(props.VersionNum, props.Recovery, props.GucTrackCommitTimestamp, 256, "public")
	assert.NoError(t, views.Configure(opts))

	c, err := NewCollector(conn)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	// Must not panic and must produce a 19-column result.
	s, err := c.Update(conn, views["procpidstat"], time.Second)
	assert.NoError(t, err)
	assert.Equal(t, 19, s.Pgstat.Result.Ncols)
	assert.True(t, s.Pgstat.Result.Valid)
	assert.Len(t, s.Pgstat.Result.Cols, 19)
}
