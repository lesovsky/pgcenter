// Package 'stat' provides things for working with stats - reading and processing.

package stat

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"io/ioutil"
	"os/exec"
	"strconv"
)

// Sysstat is the container for system stats - CPU usage, load average, mem/swap.
type Sysstat struct {
	LoadAvg
	Cpustat
	Meminfo
}

// Stat is the container for all collected stats - system and postgres
type Stat struct {
	Sysstat
	Pgstat
	Iostat
	Nicstat
}

const (
	// procUptime is the location of system uptime file
	procUptime = "/proc/uptime"
	// pgProcUptimeQuery is the SQL for querying system uptime from Postgres instance
	pgProcUptimeQuery = `SELECT
		(seconds_total * pgcenter.get_sys_clk_ticks()) +
		((seconds_total - floor(seconds_total)) * pgcenter.get_sys_clk_ticks() / 100)
		FROM pgcenter.sys_proc_uptime`
	//pgProcCountDiskstatsQuery queries total number of block devices from Postgres instance
	pgProcCountDiskstatsQuery = "SELECT count(1) FROM pgcenter.sys_proc_diskstats"
	// pgProcCountNetdevQuery queries total number of network interfaces from Postgres instance
	pgProcCountNetdevQuery = "SELECT count(1) FROM pgcenter.sys_proc_netdev"
)

var (
	// SysTicks stores the system timer's frequency
	SysTicks float64 = 100
)

func init() {
	cmdOutput, err := exec.Command("getconf", "CLK_TCK").Output()
	if err != nil {
		SysTicks, _ = strconv.ParseFloat(string(cmdOutput), 64)
	}
}

// GetSysStat method read all required system stats. Ignore any errors during reading stat, just print zeroes
func (s *Stat) GetSysStat(conn *sql.DB, isLocal bool) {
	s.LoadAvg.Read(conn, isLocal, s.PgcenterSchemaAvail)

	s.CurrCpuSample.Read(conn, isLocal, s.PgcenterSchemaAvail)
	s.CpuUsage.Diff(s.PrevCpuSample, s.CurrCpuSample)
	s.PrevCpuSample = s.CurrCpuSample

	s.Meminfo.Read(conn, isLocal, s.PgcenterSchemaAvail)
}

// sValue routine calculates percent ratio of calculated metric within specified time interval
func sValue(prev, curr, itv, ticks float64) float64 {
	if curr > prev {
		return (curr - prev) / itv * ticks
	}
	return 0
}

// uptime reads uptime value from local 'procfs' filesystem
func uptime() (float64, error) {
	var upsec, upcent float64

	content, err := ioutil.ReadFile(procUptime)
	if err != nil {
		return 0, fmt.Errorf("failed to read %s", procUptime)
	}

	reader := bufio.NewReader(bytes.NewBuffer(content))

	line, _, err := reader.ReadLine()
	if err != nil {
		return 0, fmt.Errorf("failed to scan data from %s", procUptime)
	}
	fmt.Sscanf(string(line), "%f.%f", &upsec, &upcent)

	return (upsec * SysTicks) + (upcent * SysTicks / 100), nil
}

// CountLines just count lines in specified source
func CountLines(f string, conn *sql.DB, isLocal bool, pgcAvail bool) (int, error) {
	if isLocal {
		return CountLinesLocal(f)
	} else if pgcAvail {
		return CountLinesRemote(f, conn)
	}
	return 0, nil
}

// CountLinesLocal counts lines in local file
func CountLinesLocal(f string) (int, error) {
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

// CountLinesRemote counts lines in Postgres instance
func CountLinesRemote(f string, conn *sql.DB) (int, error) {
	var count int

	switch f {
	case ProcDiskstats:
		err := conn.QueryRow(pgProcCountDiskstatsQuery).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("failed to count rows: %s", err)
		}
	case ProcNetdevFile:
		err := conn.QueryRow(pgProcCountNetdevQuery).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("failed to count rows: %s", err)
		}
	}

	return count, nil
}
