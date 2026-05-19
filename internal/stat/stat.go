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
	systicks, err := getSysticksLocal()
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
	// joined result produced by buildProcPidResult(). Individual PID errors
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
			if st, err := readProcPidStat(pid); err == nil {
				c.currProcPidStats[pid] = st
			}
			if view.IOAvailable {
				if io, err := readProcPidIO(pid); err == nil {
					c.currProcPidIO[pid] = io
				}
			}
		}

		// Replace the 7-col SQL result with the 19-col enriched one. The same
		// 19-col PGresult flows through calculateDelta() below — with
		// DiffIntvl=[0,0] (set on the procpidstat view) calculateDelta() acts
		// as identity, leaving the column count intact.
		enriched := buildProcPidResult(
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

	c.prevPgStat = c.currPgStat
	c.currPgStat = Pgstat{Activity: activity, Result: res}

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

// getSysticksLocal return local value of ticks returned by 'getconf CLK_TCK' command.
func getSysticksLocal() (float64, error) {
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
