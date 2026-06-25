package stat

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/view"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// pgProcUptimeQuery is the SQL for querying system uptime from Postgres instance
	pgProcUptimeQuery = `SELECT
		(seconds_total * pgcenter.get_sys_clk_ticks()) +
		((seconds_total - floor(seconds_total)) * pgcenter.get_sys_clk_ticks() / 100)
		FROM pgcenter.sys_proc_uptime`

	// Collect flags specifies what kind of extra stats should be collected.
	CollectNone = iota
	CollectDiskstats
	CollectNetdev
	CollectFsstats
	CollectLogtail
	CollectProcPidStat
)

// Stat defines all stats collected during single reading.
type Stat struct {
	System       // system-related stats
	Pgstat       // postgres-related stats
	Error  error // error occurred during reading stats
}

// System defines system-related stats.
type System struct {
	LoadAvg
	Meminfo
	CPUStat
	Diskstats
	Netdevs
	Fsstats
	// VerboseFirstTick mirrors Collector.verboseFirstTick into the Stat handed to the render
	// goroutine: the delta-based verbose system rows (iostat/nicstat) have no valid prev point on
	// this tick, so the composer must render n/a instead of a misleading zero delta. The collector
	// and renderer share state only through Stat, so the flag is propagated here (Task 8).
	VerboseFirstTick bool
}

// verboseGuardFloor is the absolute lower bound of the latency-guard threshold: the dear no-twin
// aggregate (db sizes / growth) is never throttled for a latency below this floor, regardless of how
// short the refresh interval is. See latencyGuardThreshold.
const verboseGuardFloor = 500 * time.Millisecond

// verboseGuardFraction is the relative part of the latency-guard threshold: a quarter (25%) of the
// refresh interval. See latencyGuardThreshold.
const verboseGuardFraction = 4

// latencyGuardThreshold returns the latency budget for the dear no-twin aggregate (db sizes / growth):
// the larger of 25% of the refresh interval and the 500ms floor. When the source's last query exceeded
// this budget it is throttled (skipped next tick, reusing the cached stale value). Derived solely from
// the refresh interval — no second user knob. A short interval (e.g. 1s -> 25% = 250ms) clamps to the
// floor; a long interval (e.g. 4s -> 25% = 1s) uses the relative budget.
func latencyGuardThreshold(refresh time.Duration) time.Duration {
	relative := refresh / verboseGuardFraction
	if relative > verboseGuardFloor {
		return relative
	}
	return verboseGuardFloor
}

// verboseCollectState groups the verbose-mode collection state so verbose-specific fields do not leak
// across the shared Collector (Decision 9). It carries the first-tick re-arm flags (moved here from
// Collector, added forward-compatible by Task 7) and the per-source cadence/latency-guard state for the
// dear no-twin aggregate (db sizes / growth): only that source is throttled — system rows and the cheap
// aggregates collect every tick (consistency with the full panels a DBA cross-checks).
type verboseCollectState struct {
	// verboseFirstTick signals that delta-based verbose metrics have no valid prev point on this tick,
	// so the composer must render n/a instead of a misleading zero delta.
	verboseFirstTick bool
	// prevVerboseActive tracks whether verbose was active on the previous tick; it re-arms
	// verboseFirstTick on every OFF->ON re-enable without relying on c.Reset() (toggleVerbose skips Reset).
	prevVerboseActive bool
	// dbSizeLastLatency is the measured duration of the last real db-size query. When it exceeds the
	// latency-guard threshold, collection of this source is throttled (skipped) for at most the cadence
	// budget and the cached value below is reused; after the budget elapses one real collection is forced
	// to re-probe the latency — auto-resuming when it has recovered.
	dbSizeLastLatency time.Duration
	// dbSizeLastRun is the wall-clock time of the last REAL db-size collection. Together with
	// dbSizeLastLatency it bounds the throttle: a slow source is skipped only until the cadence budget
	// elapses since dbSizeLastRun, then re-probed. Without this cadence the guard would latch permanently
	// (a slow source would never be re-measured, so auto-resume could never happen).
	dbSizeLastRun time.Time
	// dbSizeCache holds the last successfully collected size/growth aggregate, reused (stale, not n/a)
	// while the source is throttled.
	dbSizeCache PgstatOverview
	// dbSizeCacheValid is true once dbSizeCache holds a real collected value.
	dbSizeCacheValid bool
}

// dbSizeThrottled reports whether the dear db-size source should be skipped on this tick. It is throttled
// when (a) a real value is already cached, (b) the last query exceeded the latency-guard threshold, AND
// (c) less than the cadence budget has elapsed since the last real collection. Condition (c) is what
// makes this a throttle rather than a permanent latch: once the budget elapses the source is force-
// collected to re-probe its latency, which either re-arms the throttle (still slow) or resumes it (now
// fast). Pure decision (sinceLastRun is passed in), so the guard is testable without live PG or a clock.
func (s verboseCollectState) dbSizeThrottled(threshold, budget, sinceLastRun time.Duration) bool {
	if !s.dbSizeCacheValid {
		return false // genuine first tick (nothing cached) — must collect.
	}
	if s.dbSizeLastLatency <= threshold {
		return false // last query was within budget — collect normally.
	}
	return sinceLastRun < budget // slow, but re-probe once the cadence budget elapses.
}

// Collector defines container for stats objects.
type Collector struct {
	config Config // collector configuration
	// cpu usage snapshots for previous and current intervals
	prevCPUStat CPUStat
	currCPUStat CPUStat
	// disk devices usage snapshots for previous and current intervals
	prevDiskstats Diskstats
	currDiskstats Diskstats
	// network interfaces snapshots for previous and current intervals
	prevNetdevs Netdevs
	currNetdevs Netdevs
	// mounted filesystems snapshot
	currFsstats Fsstats
	// verbose-mode collection state: first-tick re-arm flags + per-source latency-guard/cadence state.
	verbose verboseCollectState
	// postgres stats snapshots for previous and current intervals
	prevPgStat Pgstat
	currPgStat Pgstat
	// per-process CPU stats snapshots for previous and current intervals
	prevProcPidStats map[int]ProcPidStat
	currProcPidStats map[int]ProcPidStat
	// per-process IO stats snapshots for previous and current intervals
	prevProcPidIO map[int]ProcPidIO
	currProcPidIO map[int]ProcPidIO
}

// Config defines collector's runtime configuration.
type Config struct {
	// value of system setting CLK_TCK, required for local stats calculations.
	ticks float64
	// flag specifies that collecting extra stats required.
	collectExtra int
	// Postgres properties necessary for different purposes.
	PostgresProperties
}

// NewCollector creates new collector.
func NewCollector(db *postgres.DB) (*Collector, error) {
	systicks, err := GetSysticksLocal()
	if err != nil {
		return nil, fmt.Errorf("get systicks failed: %w", err)
	}

	// read Postgres properties
	props, err := GetPostgresProperties(db)
	if err != nil {
		return nil, fmt.Errorf("read postgres properties failed: %w", err)
	}

	return &Collector{
		config: Config{
			ticks:              systicks,
			PostgresProperties: props,
		},
		prevProcPidStats: make(map[int]ProcPidStat),
		currProcPidStats: make(map[int]ProcPidStat),
		prevProcPidIO:    make(map[int]ProcPidIO),
		currProcPidIO:    make(map[int]ProcPidIO),
	}, nil
}

// Reset clears stats snapshots.
func (c *Collector) Reset() {
	c.prevPgStat = Pgstat{}
	c.currPgStat = Pgstat{}
	c.prevProcPidStats = make(map[int]ProcPidStat)
	c.currProcPidStats = make(map[int]ProcPidStat)
	c.prevProcPidIO = make(map[int]ProcPidIO)
	c.currProcPidIO = make(map[int]ProcPidIO)
	// Clear the verbose latency-guard/cadence state in lockstep with the prev/curr snapshots so a
	// screen switch does not leave a "stuck" throttle blocking the first tick after the switch (the
	// stale cache and last-latency must not survive a Reset). The first-tick re-arm flags are also
	// cleared, but the re-arm itself does NOT rely on Reset — prevVerboseActive in the verbose branch
	// re-arms verboseFirstTick on every OFF->ON (toggleVerbose skips Reset, Decision 2).
	c.verbose = verboseCollectState{}
}

// Update implements stats collecting.
func (c *Collector) Update(db *postgres.DB, view view.View, refresh time.Duration) (Stat, error) {
	var s Stat

	// Collect load average stats.
	loadavg, err := readLoadAverage(db, c.config.SchemaPgcenterAvail)
	if err != nil {
		return s, err
	}

	s.LoadAvg = loadavg

	// Collect memory/swap usage stats.
	meminfo, err := readMeminfo(db, c.config.SchemaPgcenterAvail)
	if err != nil {
		return s, err
	}

	s.Meminfo = meminfo

	// Collect CPU usage stats
	cpustat, err := readCPUStat(db, c.config.SchemaPgcenterAvail)
	if err != nil {
		return s, err
	}

	c.prevCPUStat = c.currCPUStat
	c.currCPUStat = cpustat
	s.CPUStat = countCPUUsage(c.prevCPUStat, c.currCPUStat, c.config.ticks)

	// Collect extra stats if required.
	var diskstats Diskstats
	var netdevs Netdevs
	var fsstats Fsstats

	switch c.config.collectExtra {
	case CollectDiskstats:
		diskstats, err = c.collectDiskstats(db)
		if err != nil {
			return s, err
		}
		s.Diskstats = diskstats
	case CollectNetdev:
		netdevs, err = c.collectNetdevs(db)
		if err != nil {
			return s, err
		}
		s.Netdevs = netdevs
	case CollectFsstats:
		fsstats, err = c.collectFsstats(db)
		if err != nil {
			return s, err
		}
		s.Fsstats = fsstats
	}

	// Verbose mode renders all three extended system rows (disk/net/fs) at once, so collect every source
	// each tick. The == nil guards skip a source already populated by the side-panel switch above, reusing
	// the same collect* methods (and %util math) as the full panels (Decision 5). A per-source error must
	// NOT abort the sample: one failing subsystem (no network, EACCES on /proc, no remote schema) leaves
	// the source nil (rendered as n/a in Task 8) while the others still populate.
	if view.Verbose {
		// First verbose tick OR an OFF->ON re-enable (prevVerboseActive == false): delta metrics have no
		// valid prev point, so signal the composer (Task 8) to draw n/a on this tick instead of a misleading
		// zero delta. The flag persists for the rest of this Update so the composer can read it, and is
		// cleared on the next verbose tick. This does not depend on c.Reset() (toggleVerbose skips Reset).
		c.verbose.verboseFirstTick = !c.verbose.prevVerboseActive

		if s.Diskstats == nil {
			if diskstats, err := c.collectDiskstats(db); err == nil {
				s.Diskstats = diskstats
			}
		}
		if s.Netdevs == nil {
			if netdevs, err := c.collectNetdevs(db); err == nil {
				s.Netdevs = netdevs
			}
		}
		if s.Fsstats == nil {
			if fsstats, err := c.collectFsstats(db); err == nil {
				s.Fsstats = fsstats
			}
		}

		c.verbose.prevVerboseActive = true

		// Propagate the first-tick signal into the Stat handed to the render goroutine: the
		// composer (Task 8) draws n/a for delta-based system rows when this is set, since the
		// populated zero-delta slice is indistinguishable from a genuinely idle device otherwise.
		s.VerboseFirstTick = c.verbose.verboseFirstTick
	} else {
		c.verbose.prevVerboseActive = false
	}

	// Take refresh interval from view
	itv := int(refresh / time.Second)

	err = db.PQstatus()
	if err != nil {
		err = postgres.Reconnect(db)
		if err != nil {
			s.Pgstat.Activity.State = "down"
			return s, err
		}
	}

	// Collect Postgres current general activity stats.
	activity, err := collectActivityStat(db, c.config.VersionNum, c.config.ExtPGSSSchema, itv, c.prevPgStat.Activity.Calls)
	if err != nil {
		s.Pgstat.Activity = activity
		return s, err
	}

	s.Pgstat.Activity = activity

	// Check view is supported by current version. This helps to avoid unnecessary errors in Postgres logs.
	if !view.VersionOK(c.config.VersionNum) {
		return s, fmt.Errorf("selected statistics is not supported by current version of Postgres")
	}

	// Collect Postgres stats related to user's choice.
	res, err := collectPostgresStat(db, view.Query)
	if err != nil {
		return s, err
	}

	s.Pgstat.Result = res

	// Per-process system stats enrichment. When the active view requests
	// per-PID procfs data, replace the 7-column SQL result with the 19-column
	// joined result produced by BuildProcPidResult(). Individual PID errors
	// (process exited mid-tick, EACCES on /proc/[pid]/io) are skipped silently.
	if view.CollectExtra == CollectProcPidStat {
		// Build cleanup-before-swap: keep in prev only PIDs that are present
		// in the current activity result, then move curr → prev and start curr fresh.
		newPrevStats := make(map[int]ProcPidStat)
		newPrevIO := make(map[int]ProcPidIO)
		for _, row := range res.Values {
			if len(row) == 0 {
				continue
			}
			pid, err := strconv.Atoi(strings.TrimSpace(row[0].String))
			if err != nil || pid <= 0 {
				continue
			}
			if v, ok := c.currProcPidStats[pid]; ok {
				newPrevStats[pid] = v
			}
			if v, ok := c.currProcPidIO[pid]; ok {
				newPrevIO[pid] = v
			}
		}
		c.prevProcPidStats = newPrevStats
		c.prevProcPidIO = newPrevIO
		c.currProcPidStats = make(map[int]ProcPidStat)
		c.currProcPidIO = make(map[int]ProcPidIO)

		// Collect fresh procfs data per PID present in the activity result.
		for _, row := range res.Values {
			if len(row) == 0 {
				continue
			}
			pid, err := strconv.Atoi(strings.TrimSpace(row[0].String))
			if err != nil || pid <= 0 {
				continue
			}
			if st, err := ReadProcPidStat(pid); err == nil {
				c.currProcPidStats[pid] = st
			}
			if view.IOAvailable {
				if io, err := ReadProcPidIO(pid); err == nil {
					c.currProcPidIO[pid] = io
				}
			}
		}

		// Replace the 7-col SQL result with the 19-col enriched one. The same
		// 19-col PGresult flows through calculateDelta() below — with
		// DiffIntvl=[0,0] (set on the procpidstat view) calculateDelta() acts
		// as identity, leaving the column count intact.
		enriched := BuildProcPidResult(
			res,
			c.prevProcPidStats, c.currProcPidStats,
			c.prevProcPidIO, c.currProcPidIO,
			view.IOAvailable,
			view.DelayAcctAvailable,
			c.config.ticks,
			float64(itv),
			runtime.NumCPU(),
		)
		s.Pgstat.Result = enriched
		res = enriched
	}

	// Collect verbose overview aggregates only when verbose mode is enabled; the compact path is
	// untouched. Rates are computed against the previous snapshot's Overview. The collect function
	// never returns an error: each privileged/expensive aggregate degrades its own field to n/a.
	// c.currPgStat still holds the previous tick's snapshot at this point (the curr->prev shift
	// happens below), so its Overview is the correct prev snapshot for rate computation.
	var overview PgstatOverview
	if view.Verbose {
		// Latency guard for the dear no-twin aggregate (db sizes / growth): when the last db-size query
		// exceeded the threshold (max 25% of refresh, 500ms floor), skip it and reuse the cached stale
		// value (not n/a) — there is no live panel twin to cross-check, so staleness is acceptable
		// (Decision 9). The skip is bounded by a cadence budget (one refresh interval): once it elapses the
		// source is force-collected to re-probe its latency, so the guard auto-resumes when latency recovers
		// instead of latching forever. The cheap aggregates (workload/workers/...) and all system rows
		// always collect every tick. On the genuine first verbose tick there is nothing cached yet, so the
		// source must collect.
		threshold := latencyGuardThreshold(refresh)
		budget := refresh
		// On the very first tick dbSizeLastRun is the zero time, so time.Since(...) is enormous — but it is
		// deliberately unused: dbSizeThrottled short-circuits on !dbSizeCacheValid before consulting it.
		skipSize := c.verbose.dbSizeThrottled(threshold, budget, time.Since(c.verbose.dbSizeLastRun))

		var sizeLatency time.Duration
		overview, sizeLatency = collectOverviewStat(db, c.config.PostgresProperties, itv, c.currPgStat.Overview, skipSize)
		if skipSize {
			// Reuse the cached stale size/growth fields; everything else in overview is fresh.
			overview.TotalSize = c.verbose.dbSizeCache.TotalSize
			overview.TotalSizeValid = c.verbose.dbSizeCache.TotalSizeValid
			overview.GrowthPerSec = c.verbose.dbSizeCache.GrowthPerSec
		} else {
			// Real collection happened: record the db-size query's own latency (measured narrowly around
			// its QueryRow inside collectOverviewStat, so a slow neighbour aggregate cannot trip the guard
			// for the wrong source), stamp the cadence clock, and refresh the cache so the next throttled
			// tick has a stale value to reuse.
			c.verbose.dbSizeLastLatency = sizeLatency
			c.verbose.dbSizeLastRun = time.Now()
			c.verbose.dbSizeCache = overview
			c.verbose.dbSizeCacheValid = true
		}
	}

	c.prevPgStat = c.currPgStat
	c.currPgStat = Pgstat{Activity: activity, Result: res, Overview: overview}
	s.Pgstat.Overview = overview

	// Compare previous and current Postgres stats snapshots and calculate delta.
	diff, err := calculateDelta(c.currPgStat.Result, c.prevPgStat.Result, itv, view.DiffIntvl, view.OrderKey, view.OrderDesc, view.UniqueKey)
	if err != nil {
		return s, err
	}

	s.Pgstat.Result = diff

	return s, nil
}

// ToggleCollectExtra toggle collector's setting related to extra stats.
func (c *Collector) ToggleCollectExtra(e int) {
	c.config.collectExtra = e
}

// collectDiskstats implements collecting of disk devices stats.
func (c *Collector) collectDiskstats(db *postgres.DB) (Diskstats, error) {
	stats, err := readDiskstats(db, c.config)
	if err != nil {
		return nil, err
	}

	c.prevDiskstats = c.currDiskstats
	c.currDiskstats = stats

	// If number of block devices changed just replace previous snapshot with current one and continue.
	if len(c.prevDiskstats) != len(c.currDiskstats) {
		c.prevDiskstats = c.currDiskstats
	}

	usage := countDiskstatsUsage(c.prevDiskstats, c.currDiskstats, c.config.ticks)

	return usage, nil
}

// collectNetdevs implements collecting network interfaces stats.
func (c *Collector) collectNetdevs(db *postgres.DB) (Netdevs, error) {
	stats, err := readNetdevs(db, c.config)
	if err != nil {
		return nil, err
	}

	c.prevNetdevs = c.currNetdevs
	c.currNetdevs = stats

	// If number of network devices changed just replace previous snapshot with current one and continue.
	if len(c.prevNetdevs) != len(c.currNetdevs) {
		c.prevNetdevs = c.currNetdevs
	}

	usage := countNetdevsUsage(c.prevNetdevs, c.currNetdevs, c.config.ticks)

	return usage, nil
}

// collectFsstats implements collecting mounted filesystems stats.
func (c *Collector) collectFsstats(db *postgres.DB) (Fsstats, error) {
	stats, err := readFsstats(db, c.config)
	if err != nil {
		return nil, err
	}

	c.currFsstats = stats

	return c.currFsstats, nil
}

// readUptimeLocal returns uptime value from passed specified procfile.
func readUptimeLocal(procfile string, ticks float64) (float64, error) {
	var sec, csec int64

	content, err := os.ReadFile(filepath.Clean(procfile))
	if err != nil {
		return 0, err
	}

	reader := bufio.NewReader(bytes.NewBuffer(content))

	line, _, err := reader.ReadLine()
	if err != nil {
		return 0, err
	}
	_, err = fmt.Sscanf(string(line), "%d.%d", &sec, &csec)
	if err != nil {
		return 0, err
	}

	return (float64(sec) * ticks) + (float64(csec) * ticks / 100), nil
}

// GetSysticksLocal returns local value of ticks returned by 'getconf CLK_TCK'
// command. Exported so the record package can capture ticks at session start
// and persist them via SysInfo for the report stage.
func GetSysticksLocal() (float64, error) {
	cmdOutput, err := exec.Command("getconf", "CLK_TCK").Output()
	if err != nil {
		return 0, err
	}

	systicks, err := strconv.ParseFloat(strings.TrimSpace(string(cmdOutput)), 64)
	if err != nil {
		return 0, err
	}

	return systicks, nil
}

// sValue calculates delta within specified time interval.
func sValue(prev, curr, itv, ticks float64) float64 {
	if curr > prev {
		return (curr - prev) / itv * ticks
	}
	return 0
}
