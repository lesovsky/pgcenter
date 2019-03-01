// Stuff related to CPU usage stats

package stat

/* Struct for raw stat data read from file/sql sources */
import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
)

const (
	procStatFile       = "/proc/stat"
	pgProcCpuStatQuery = "SELECT * FROM pgcenter.sys_proc_stat WHERE cpu = 'cpu'"
)

// CpuRawstat is a container for raw values collected from cpu-stat source
type CpuRawstat struct {
	Entry   string
	User    float64
	Nice    float64
	Sys     float64
	Idle    float64
	Iowait  float64
	Irq     float64
	Softirq float64
	Steal   float64
	Guest   float64
	GstNice float64
	Total   float64
}

// CpuUsage is a container for calculated CPU usage stats
type CpuUsage struct {
	User    float64
	Sys     float64
	Nice    float64
	Idle    float64
	Iowait  float64
	Irq     float64
	Softirq float64
	Steal   float64
}

// Cpustat contains current, previous CPU stats snapshots, and its delta as calculated 'usage' stats
type Cpustat struct {
	CurrCpuSample CpuRawstat
	PrevCpuSample CpuRawstat
	CpuUsage
}

// Read method reads CPU raw stats
func (s *CpuRawstat) Read(conn *sql.DB, isLocal bool, pgcAvail bool) {
	if isLocal {
		s.ReadLocal()
	} else if pgcAvail{
		s.ReadRemote(conn)
	}
}

// ReadLocal reads CPU raw stats from local 'procfs' filesystem
func (s *CpuRawstat) ReadLocal() {
	content, err := ioutil.ReadFile(procStatFile)
	if err != nil {
		return
	}

	reader := bufio.NewReader(bytes.NewBuffer(content))
	for {
		line, _, err := reader.ReadLine()
		if err == io.EOF {
			break
		}

		fields := strings.Fields(string(line))
		if len(fields) > 0 {
			if fields[0] == "cpu" {
				_, err = fmt.Sscanln(string(line),
					&s.Entry, &s.User, &s.Nice, &s.Sys, &s.Idle,
					&s.Iowait, &s.Irq, &s.Softirq, &s.Steal, &s.Guest, &s.GstNice)
				if err != nil {
					return
				}

				/* Use total instead of uptime, because of separate reading of /proc/uptime and /proc/stat leads to stat's skew */
				s.Total = s.User + s.Nice + s.Sys + s.Idle + s.Iowait + s.Irq + s.Softirq + s.Steal + s.Guest
			}
		}
	}
	return
}

// ReadRemote method reads CPU raw stats from Postgres instance
func (s *CpuRawstat) ReadRemote(conn *sql.DB) {
	conn.QueryRow(pgProcCpuStatQuery).Scan(&s.Entry, &s.User, &s.Nice, &s.Sys, &s.Idle,
		&s.Iowait, &s.Irq, &s.Softirq, &s.Steal, &s.Guest, &s.GstNice)

	s.Total = s.User + s.Nice + s.Sys + s.Idle + s.Iowait + s.Irq + s.Softirq + s.Steal + s.Guest
}

// Diff method calculates 'CPU usage' human-readable stat using current and previous snapshots
func (u *CpuUsage) Diff(prev CpuRawstat, curr CpuRawstat) {
	itv := curr.Total - prev.Total

	u.User = sValue(prev.User, curr.User, itv, SysTicks)
	u.Nice = sValue(prev.Nice, curr.Nice, itv, SysTicks)
	u.Sys = sValue(prev.Sys, curr.Sys, itv, SysTicks)
	u.Idle = sValue(prev.Idle, curr.Idle, itv, SysTicks)
	u.Iowait = sValue(prev.Iowait, curr.Iowait, itv, SysTicks)
	u.Irq = sValue(prev.Irq, curr.Irq, itv, SysTicks)
	u.Softirq = sValue(prev.Softirq, curr.Softirq, itv, SysTicks)
	u.Steal = sValue(prev.Steal, curr.Steal, itv, SysTicks)

	return
}
