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

// Container for system stats - CPU usage, load average, mem/swap.
type Sysstat struct {
	LoadAvg
	Cpustat
	Meminfo
}

// Container for all stats - System and Postgres
type Stat struct {
	Sysstat
	Pgstat
	Iostat
	Nicstat
}

const (
	PROC_UPTIME = "/proc/uptime"

	pgProcUptimeQuery = `SELECT
		(seconds_total * pgcenter.get_sys_clk_ticks()) +
		((seconds_total - floor(seconds_total)) * pgcenter.get_sys_clk_ticks() / 100)
		FROM pgcenter.sys_proc_uptime`
	pgProcCountDiskstatsQuery = "SELECT count(1) FROM pgcenter.sys_proc_diskstats"
	pgProcCountNetdevQuery    = "SELECT count(1) FROM pgcenter.sys_proc_netdev"
)

var (
	SysTicks float64 = 100
)

func init() {
	cmdOutput, err := exec.Command("getconf", "CLK_TCK").Output()
	if err != nil {
		SysTicks, _ = strconv.ParseFloat(string(cmdOutput), 64)
	}
}

// Read all required stat. Ignore any errors during reading stat, just print zeroes
func (s *Stat) GetSysStat(conn *sql.DB, isLocal bool) {
	s.LoadAvg.Read(conn, isLocal)

	s.CurrCpuSample.Read(conn, isLocal)
	s.CpuUsage.Diff(s.PrevCpuSample, s.CurrCpuSample)
	s.PrevCpuSample = s.CurrCpuSample

	s.Meminfo.Read(conn, isLocal)
}

// Calculates percent ratio of calculated metric within specified time interval
func s_value(prev, curr, itv, ticks float64) float64 {
	if curr > prev {
		return (curr - prev) / itv * ticks
	} else {
		return 0
	}
}

// Read uptime value from local procfile
func uptime() (float64, error) {
	var upsec, upcent float64

	content, err := ioutil.ReadFile(PROC_UPTIME)
	if err != nil {
		return 0, fmt.Errorf("failed to read %s", PROC_UPTIME)
	}

	reader := bufio.NewReader(bytes.NewBuffer(content))

	line, _, err := reader.ReadLine()
	if err != nil {
		return 0, fmt.Errorf("failed to scan data from %s", PROC_UPTIME)
	}
	fmt.Sscanf(string(line), "%f.%f", &upsec, &upcent)

	return (upsec * SysTicks) + (upcent * SysTicks / 100), nil
}

// Count lines in specified source
func CountLines(f string, conn *sql.DB, isLocal bool) (int, error) {
	if isLocal {
		return CountLinesLocal(f)
	} else {
		return CountLinesRemote(f, conn)
	}
}

// Count lines in local file
func CountLinesLocal(f string) (int, error) {
	content, err := ioutil.ReadFile(f)
	if err != nil {
		return 0, fmt.Errorf("failed to read %s", f)
	}
	r := bufio.NewReader(bytes.NewBuffer(content))

	buf := make([]byte, 128)
	count := 0
	lineSep := []byte{'\n'}

	if f == PROC_NETDEV {
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

// Count lines in remote SQL source
func CountLinesRemote(f string, conn *sql.DB) (int, error) {
	var count int

	switch f {
	case PROC_DISKSTATS:
		err := conn.QueryRow(pgProcCountDiskstatsQuery).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("failed to count rows: %s", err)
		}
	case PROC_NETDEV:
		err := conn.QueryRow(pgProcCountNetdevQuery).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("failed to count rows: %s", err)
		}
	}

	return count, nil
}
