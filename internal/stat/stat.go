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
)

type Stat struct {
	System
	Pgstat
	Iostat  /* not refactored yet */
	Nicstat /* not refactored yet */
}

//
type System struct {
	LoadAvg
	CpuStat
	Meminfo
}

//
type Collector struct {
	config      Config
	prevCpuStat CpuStat
	currCpuStat CpuStat
	prevPgStat  Pgstat
	currPgStat  Pgstat
}

type Config struct {
	ticks              float64 // value of system setting CLK_TCK
	PostgresProperties         // postgres variables and constants which are not changed in runtime (but might change between Postgres restarts)
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
	props, err := ReadPostgresProperties(db)
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
	loadavg, err := readLoadAverage(db, c.config.SchemaPgcenterAvail)
	if err != nil {
		return Stat{}, err
	}

	meminfo, err := readMeminfo(db, c.config.SchemaPgcenterAvail)
	if err != nil {
		return Stat{}, err
	}

	cpustat, err := readCpuStat(db, c.config.SchemaPgcenterAvail)
	if err != nil {
		return Stat{}, err
	}

	c.prevCpuStat = c.currCpuStat
	c.currCpuStat = cpustat

	cpuusage := countCpuUsage(c.prevCpuStat, c.currCpuStat, c.config.ticks)

	pgstat, err := collectPostgresStat(db, view.Query, c.prevPgStat)
	if err != nil {
		return Stat{}, err
	}

	c.prevPgStat = c.currPgStat
	c.currPgStat = pgstat

	//
	diff, err := calculateDelta(c.currPgStat.Result, c.prevPgStat.Result, 1, view.DiffIntvl, view.OrderKey, view.OrderDesc, view.UniqueKey)
	if err != nil {
		return Stat{}, err
	}

	return Stat{
		System: System{
			LoadAvg: loadavg,
			Meminfo: meminfo,
			CpuStat: cpuusage,
		},
		Pgstat: Pgstat{
			Activity: c.currPgStat.Activity,
			Result:   diff,
		},
	}, nil
}

// sValue routine calculates percent ratio of calculated metric within specified time interval
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
