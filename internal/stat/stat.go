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

	// collect flags specifies what kind of extra stats should be collected.
	CollectNone = iota
	CollectDiskstats
	CollectNetdev
	CollectLogtail
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
	CpuStat
	Diskstats
	Netdevs
}

// Collector defines container for stats objects.
type Collector struct {
	config Config // collector configuration
	// cpu usage snapshots for previous and current intervals
	prevCpuStat CpuStat
	currCpuStat CpuStat
	// disk devices usage snapshots for previous and current intervals
	prevDiskstats Diskstats
	currDiskstats Diskstats
	// network interfaces snapshots for previous and current intervals
	prevNetdevs Netdevs
	currNetdevs Netdevs
	// postgres stats snapshots for previous and current intervals
	prevPgStat Pgstat
	currPgStat Pgstat
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
		return nil, fmt.Errorf("get systicks failed: %s", err)
	}

	// read Postgres properties
	props, err := GetPostgresProperties(db)
	if err != nil {
		return nil, fmt.Errorf("read postgres properties failed: %s", err)
	}

	return &Collector{
		config: Config{
			ticks:              systicks,
			PostgresProperties: props,
		},
	}, nil
}

// Reset clears stats snapshots.
func (c *Collector) Reset() {
	c.prevPgStat = Pgstat{}
	c.currPgStat = Pgstat{}
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
	cpustat, err := readCpuStat(db, c.config.SchemaPgcenterAvail)
	if err != nil {
		return s, err
	}

	c.prevCpuStat = c.currCpuStat
	c.currCpuStat = cpustat
	s.CpuStat = countCpuUsage(c.prevCpuStat, c.currCpuStat, c.config.ticks)

	// Collect extra stats if required.
	var diskstats Diskstats
	var netdevs Netdevs

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

	// Collect Postgres stats.
	pgstat, err := collectPostgresStat(db, c.config.VersionNum, c.config.ExtPGSSSchema, itv, view.Query, c.prevPgStat)
	if err != nil {
		s.Pgstat.Activity = pgstat.Activity
		return s, err
	}

	s.Pgstat.Activity = pgstat.Activity

	c.prevPgStat = c.currPgStat
	c.currPgStat = pgstat

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
