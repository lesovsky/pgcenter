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
		// Compact path must be untouched: regular activity/result collection still works.
		assert.True(t, stat.Pgstat.Result.Valid, "compact path (Result) must still be collected")
		assert.Greater(t, stat.Pgstat.Activity.ConnTotal, 0)
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

func TestCollector_Update_Verbose(t *testing.T) {
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

	v := baseView
	v.Verbose = true
	views := view.Views{"activity": v}
	assert.NoError(t, views.Configure(opts))
	v = views["activity"]

	c, err := NewCollector(conn)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	// First verbose tick: all three system sources populate in a single sample.
	stat, err := c.Update(conn, v, time.Second)
	assert.NoError(t, err)
	assert.NotEmpty(t, stat.System.Diskstats, "diskstats must populate under verbose")
	assert.NotEmpty(t, stat.System.Netdevs, "netdevs must populate under verbose")
	assert.NotEmpty(t, stat.System.Fsstats, "fsstats must populate under verbose")

	// First-tick flag is set after the first verbose Update.
	assert.True(t, c.verbose.verboseFirstTick)

	// Second verbose tick: flag is cleared.
	_, err = c.Update(conn, v, time.Second)
	assert.NoError(t, err)
	assert.False(t, c.verbose.verboseFirstTick)

	// OFF->ON re-enable WITHOUT Reset: one non-verbose tick, then verbose again.
	off := baseView
	off.Verbose = false
	offViews := view.Views{"activity": off}
	assert.NoError(t, offViews.Configure(opts))

	_, err = c.Update(conn, offViews["activity"], time.Second)
	assert.NoError(t, err)
	assert.False(t, c.verbose.verboseFirstTick)
	// The else-branch must disarm prevVerboseActive so the next OFF->ON re-enables the flag.
	assert.False(t, c.verbose.prevVerboseActive)

	// Re-enable verbose: flag must re-arm WITHOUT c.Reset().
	stat, err = c.Update(conn, v, time.Second)
	assert.NoError(t, err)
	assert.True(t, c.verbose.verboseFirstTick)
	assert.NotEmpty(t, stat.System.Diskstats, "diskstats must populate on re-enable")
	assert.NotEmpty(t, stat.System.Netdevs, "netdevs must populate on re-enable")
	assert.NotEmpty(t, stat.System.Fsstats, "fsstats must populate on re-enable")

	// Following verbose tick clears the flag again.
	_, err = c.Update(conn, v, time.Second)
	assert.NoError(t, err)
	assert.False(t, c.verbose.verboseFirstTick)

	// Coexistence with an active side panel: collectExtra populates Diskstats via the switch,
	// the == nil guard skips re-collecting it, and the verbose branch still fills Netdevs/Fsstats.
	c2, err := NewCollector(conn)
	assert.NoError(t, err)
	c2.config.collectExtra = CollectDiskstats

	stat, err = c2.Update(conn, v, time.Second)
	assert.NoError(t, err)
	assert.NotEmpty(t, stat.System.Diskstats, "diskstats populated by side-panel switch")
	assert.NotEmpty(t, stat.System.Netdevs, "netdevs populated by verbose branch")
	assert.NotEmpty(t, stat.System.Fsstats, "fsstats populated by verbose branch")
}

// TestCollector_Update_DbSizeThrottle verifies the latency guard end-to-end against live PG: once the
// db-size source is marked slow, the next verbose tick reuses the cached (stale) size/growth value
// instead of re-querying, while the other overview aggregates (workload/workers/...) still refresh.
func TestCollector_Update_DbSizeThrottle(t *testing.T) {
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

	v := baseView
	v.Verbose = true
	views := view.Views{"activity": v}
	assert.NoError(t, views.Configure(opts))
	v = views["activity"]

	c, err := NewCollector(conn)
	assert.NoError(t, err)

	// Tick 1: real collection populates the size cache.
	s1, err := c.Update(conn, v, time.Second)
	assert.NoError(t, err)
	assert.True(t, s1.Pgstat.Overview.TotalSizeValid, "first tick collects db size")
	cachedSize := s1.Pgstat.Overview.TotalSize
	assert.Greater(t, cachedSize, int64(0))
	assert.Equal(t, cachedSize, c.verbose.dbSizeCache.TotalSize, "size cached after real collection")

	// Force the guard: pretend the last db-size query was slow (over the 500ms floor for refresh=1s).
	// dbSizeLastRun was just stamped by tick 1, so the cadence budget (1s) has NOT elapsed -> tick 2 is
	// throttled (the immediate "next" collection is skipped).
	c.verbose.dbSizeLastLatency = 2 * time.Second

	// Tick 2: throttled — the size/growth fields keep the cached stale value (not n/a), while the
	// cheap aggregates still collect (DatabasesCount populated every tick).
	s2, err := c.Update(conn, v, time.Second)
	assert.NoError(t, err)
	assert.True(t, s2.Pgstat.Overview.TotalSizeValid, "throttled source keeps stale value, not n/a")
	assert.Equal(t, cachedSize, s2.Pgstat.Overview.TotalSize, "stale cached size reused while throttled")
	assert.GreaterOrEqual(t, s2.Pgstat.Overview.DatabasesCount, int64(1), "cheap aggregates collect every tick")
	// System rows still collect every tick (not throttled).
	assert.NotEmpty(t, s2.System.Diskstats, "system rows collect every tick")

	// Auto-resume (end-to-end through the real Update path): once the cadence budget has elapsed since
	// the last real collection, the guard force-collects to re-probe latency — even while dbSizeLastLatency
	// still records the old slow value. Backdate dbSizeLastRun beyond the budget to simulate the elapsed
	// cadence without sleeping. The re-probe re-measures the (now fast) live query and resumes.
	c.verbose.dbSizeLastRun = time.Now().Add(-2 * time.Second) // > budget (1s) in the past
	s3, err := c.Update(conn, v, time.Second)
	assert.NoError(t, err)
	assert.True(t, s3.Pgstat.Overview.TotalSizeValid, "re-probe collects a fresh value")
	assert.LessOrEqual(t, c.verbose.dbSizeLastLatency, latencyGuardThreshold(time.Second),
		"re-probe re-measured the fast live latency -> throttle auto-resumes")
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

// Test_latencyGuardThreshold pins the latency-guard threshold formula: max(25% of refresh, 500ms floor).
// Short refresh intervals fall back to the 500ms floor; longer ones use the 25% relative budget.
func Test_latencyGuardThreshold(t *testing.T) {
	testcases := []struct {
		refresh time.Duration
		want    time.Duration
	}{
		{refresh: 1 * time.Second, want: 500 * time.Millisecond},   // 25% = 250ms < floor -> floor wins
		{refresh: 2 * time.Second, want: 500 * time.Millisecond},   // 25% = 500ms == floor
		{refresh: 4 * time.Second, want: 1 * time.Second},          // 25% = 1s > floor -> relative wins
		{refresh: 10 * time.Second, want: 2500 * time.Millisecond}, // 25% = 2.5s
		{refresh: 0, want: 500 * time.Millisecond},                 // degenerate -> floor
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, latencyGuardThreshold(tc.refresh), "refresh=%s", tc.refresh)
	}
}

// Test_verboseCollectState_throttlesSlowSource verifies the pure throttle decision: with a real value
// already cached, a source whose last measured latency exceeded the guard threshold is throttled
// (skipped) while still inside the cadence budget, so the caller reuses the cached stale value.
func Test_verboseCollectState_throttlesSlowSource(t *testing.T) {
	threshold := latencyGuardThreshold(1 * time.Second) // 500ms
	budget := 1 * time.Second
	withinBudget := 200 * time.Millisecond // < budget

	// Slow last query (> threshold), within cadence budget, cache valid: throttle the next collection.
	st := verboseCollectState{dbSizeCacheValid: true, dbSizeLastLatency: 800 * time.Millisecond}
	assert.True(t, st.dbSizeThrottled(threshold, budget, withinBudget), "slow last query must throttle next collection")

	// Last query was fast (<= threshold): do not throttle.
	st = verboseCollectState{dbSizeCacheValid: true, dbSizeLastLatency: 100 * time.Millisecond}
	assert.False(t, st.dbSizeThrottled(threshold, budget, withinBudget), "fast last query must not throttle")

	// Exactly at threshold: not over, so not throttled.
	st = verboseCollectState{dbSizeCacheValid: true, dbSizeLastLatency: threshold}
	assert.False(t, st.dbSizeThrottled(threshold, budget, withinBudget), "at-threshold latency must not throttle")
}

// Test_verboseCollectState_firstTickNotThrottled pins the genuine-first-tick guard: when nothing has
// been cached yet (dbSizeCacheValid == false), the source is NEVER throttled regardless of a stale slow
// latency — there is no stale value to fall back to, so it must collect. Without this guard the first
// tick could skip and render n/a.
func Test_verboseCollectState_firstTickNotThrottled(t *testing.T) {
	threshold := latencyGuardThreshold(1 * time.Second)
	budget := 1 * time.Second

	// No cache yet, but a (leftover) slow latency and zero sinceLastRun: must still collect.
	st := verboseCollectState{dbSizeCacheValid: false, dbSizeLastLatency: 5 * time.Second}
	assert.False(t, st.dbSizeThrottled(threshold, budget, 0), "first tick (no cache) must never be throttled")
}

// Test_verboseCollectState_autoResumes verifies the throttle is NOT a permanent latch: once the cadence
// budget elapses since the last real collection, the slow source is re-probed (not throttled) even
// though dbSizeLastLatency still records the old slow value — this is the path that re-measures latency
// and lets the guard auto-resume when the source recovers.
func Test_verboseCollectState_autoResumes(t *testing.T) {
	threshold := latencyGuardThreshold(1 * time.Second)
	budget := 1 * time.Second

	st := verboseCollectState{dbSizeCacheValid: true, dbSizeLastLatency: 900 * time.Millisecond}

	// Inside the budget: still throttled.
	assert.True(t, st.dbSizeThrottled(threshold, budget, 500*time.Millisecond), "slow source throttled within budget")

	// Budget elapsed: force a re-probe (not throttled), even with the slow latency still recorded.
	assert.False(t, st.dbSizeThrottled(threshold, budget, budget), "elapsed cadence forces a re-probe (no permanent latch)")
	assert.False(t, st.dbSizeThrottled(threshold, budget, 2*budget), "well past the budget -> re-probe")
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
