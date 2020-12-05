package stat

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/view"
	"io"
	"io/ioutil"
	"os/exec"
	"strconv"
	"strings"
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

// Stat
type Stat struct {
	System
	Pgstat
}

// System
type System struct {
	LoadAvg
	Meminfo
	CpuStat
	Diskstats
	Netdevs
}

// Collector
type Collector struct {
	//
	config Config
	//
	prevCpuStat CpuStat
	currCpuStat CpuStat
	//
	prevDiskstats Diskstats
	currDiskstats Diskstats
	//
	prevNetdevs Netdevs
	currNetdevs Netdevs
	//
	prevPgStat Pgstat
	currPgStat Pgstat
}

type Config struct {
	ticks              float64 // value of system setting CLK_TCK
	collectExtra       int
	PostgresProperties // postgres variables and constants which are not changed in runtime (but might change between Postgres restarts)
}

func NewCollector(db *postgres.DB) (*Collector, error) {
	cmdOutput, err := exec.Command("getconf", "CLK_TCK").Output()
	if err != nil {
		return nil, fmt.Errorf("determine clock frequency failed: %s", err)
	}

	systicks, err := strconv.ParseFloat(strings.TrimSpace(string(cmdOutput)), 64)
	if err != nil {
		return nil, fmt.Errorf("parse clock frequency value failed: %s", err)
	}

	// In case of remote DB, check pgcenter schema exists. In case of error, just consider the schema is not exist.
	// TODO: we have a function for checking schema existence see isSchemaAvailable
	var exists bool
	if !db.Local {
		if err := db.QueryRow(query.PgCheckPgcenterSchemaQuery).Scan(&exists); err != nil {
			exists = false
		}
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

// Reset ...
func (c *Collector) Reset() {
	c.prevPgStat = Pgstat{}
	c.currPgStat = Pgstat{}
}

// Update ...
func (c *Collector) Update(db *postgres.DB, view view.View) (Stat, error) {
	// Collect load average stats.
	loadavg, err := readLoadAverage(db, c.config.SchemaPgcenterAvail)
	if err != nil {
		return Stat{}, err
	}

	// Collect memory/swap usage stats.
	meminfo, err := readMeminfo(db, c.config.SchemaPgcenterAvail)
	if err != nil {
		return Stat{}, err
	}

	// Collect CPU usage stats
	cpustat, err := readCpuStat(db, c.config.SchemaPgcenterAvail)
	if err != nil {
		return Stat{}, err
	}

	c.prevCpuStat = c.currCpuStat
	c.currCpuStat = cpustat

	cpuusage := countCpuUsage(c.prevCpuStat, c.currCpuStat, c.config.ticks)

	// Collect extra stats if required.
	var diskstats Diskstats
	var netdevs Netdevs

	switch c.config.collectExtra {
	case CollectDiskstats:
		diskstats, err = c.collectDiskstats(db)
		if err != nil {
			return Stat{}, err
		}
	case CollectNetdev:
		netdevs, err = c.collectNetdevs(db)
		if err != nil {
			return Stat{}, err
		}
	}

	// Collect Postgres stats.
	// TODO: interval is hardcoded
	pgstat, err := collectPostgresStat(db, c.config.VersionNum, c.config.ExtPGSSAvail, 1, view.Query, c.prevPgStat)
	if err != nil {
		return Stat{}, err
	}

	c.prevPgStat = c.currPgStat
	c.currPgStat = pgstat

	// Compare previous and current Postgres stats snapshots and calculate delta.
	// TODO: interval is hardcoded
	diff, err := calculateDelta(c.currPgStat.Result, c.prevPgStat.Result, 1, view.DiffIntvl, view.OrderKey, view.OrderDesc, view.UniqueKey)
	if err != nil {
		return Stat{}, err
	}

	return Stat{
		System: System{
			LoadAvg:   loadavg,
			Meminfo:   meminfo,
			CpuStat:   cpuusage,
			Diskstats: diskstats,
			Netdevs:   netdevs,
		},
		Pgstat: Pgstat{
			Activity: c.currPgStat.Activity,
			Result:   diff,
		},
	}, nil
}

// ToggleCollectExtra ...
func (c *Collector) ToggleCollectExtra(e int) {
	c.config.collectExtra = e
}

// collectDiskstats ...
func (c *Collector) collectDiskstats(db *postgres.DB) (Diskstats, error) {
	stats, err := readDiskstats(db, c.config.SchemaPgcenterAvail)
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

// collectNetdevs ...
func (c *Collector) collectNetdevs(db *postgres.DB) (Netdevs, error) {
	stats, err := readNetdevs(db, c.config.SchemaPgcenterAvail)
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

// sValue calculates percent ratio of calculated metric within specified time interval
func sValue(prev, curr, itv, ticks float64) float64 {
	if curr > prev {
		return (curr - prev) / itv * ticks
	}
	return 0
}

//uptime reads uptime value from local 'procfs' filesystem
// lessqqmorepewpew - DEPRECATED; this function is still used when calculating Iostat and Nicstat stats.
func uptime() (float64, error) {
	var upsec, upcent float64
	var ticks float64 = 100 // local implementation of SysTicks (GET_CLK) variable

	content, err := ioutil.ReadFile("/proc/uptime")
	if err != nil {
		return 0, fmt.Errorf("failed to read %s", "/proc/uptime")
	}

	reader := bufio.NewReader(bytes.NewBuffer(content))

	line, _, err := reader.ReadLine()
	if err != nil {
		return 0, fmt.Errorf("failed to scan data from %s", "/proc/uptime")
	}
	fmt.Sscanf(string(line), "%f.%f", &upsec, &upcent)

	return (upsec * ticks) + (upcent * ticks / 100), nil
}

// CountDevices counts number of devices in passed sources.
func CountDevices(f string, db *postgres.DB, pgcAvail bool) (int, error) {
	if db.Local {
		return countDevicesLocal(f)
	} else if pgcAvail {
		return countDevicesRemote(f, db)
	}
	return 0, nil
}

// countDevicesLocal counts devices in local files.
func countDevicesLocal(f string) (int, error) {
	content, err := ioutil.ReadFile(f)
	if err != nil {
		return 0, fmt.Errorf("failed to read %s", f)
	}
	r := bufio.NewReader(bytes.NewBuffer(content))

	buf := make([]byte, 128)
	count := 0
	lineSep := []byte{'\n'}

	if f == ProcNetdevFile {
		count = count - 2 // Shift the counter because '/proc/net/dev' contains 2 lines of header
	}

	for {
		c, err := r.Read(buf)
		count += bytes.Count(buf[:c], lineSep)

		switch {
		case err == io.EOF:
			return count, nil
		case err != nil:
			return count, fmt.Errorf("failed to count rows: %s", err)
		}
	}
}

// countDevicesRemote counts devices using remote Postgres instance
func countDevicesRemote(f string, db *postgres.DB) (int, error) {
	var count int

	switch f {
	case ProcDiskstats:
		err := db.QueryRow("SELECT count(1) FROM pgcenter.sys_proc_diskstats").Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("failed to count rows: %s", err)
		}
	case ProcNetdevFile:
		err := db.QueryRow("SELECT count(1) FROM pgcenter.sys_proc_netdev").Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("failed to count rows: %s", err)
		}
	}

	return count, nil
}
