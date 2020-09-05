// Stuff related to CPU usage stats

package stat

/* Struct for raw stat data read from file/sql sources */
import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"io"
	"io/ioutil"
	"os"
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
func (s *CpuRawstat) Read(db *postgres.DB, pgcAvail bool) {
	if db.Local {
		s.ReadLocal()
	} else if pgcAvail {
		s.ReadRemote(db)
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
func (s *CpuRawstat) ReadRemote(db *postgres.DB) {
	db.QueryRow(pgProcCpuStatQuery).Scan(&s.Entry, &s.User, &s.Nice, &s.Sys, &s.Idle,
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

/* new */

type CpuStat struct {
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

func readCpuStat(db *postgres.DB, schemaExists bool) (CpuStat, error) {
	if db.Local {
		return readCpuStatLocal("/proc/stat")
	} else if schemaExists {
		return readCpuStatRemote(db)
	}

	return CpuStat{}, nil
}

func readCpuStatLocal(statfile string) (CpuStat, error) {
	var stat CpuStat
	f, err := os.Open(statfile)
	if err != nil {
		return stat, err
	}

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		parts := strings.Fields(line)
		if len(parts) < 2 {
			//log.Debugf("/proc/stat bad line; skip")
			continue
		}

		// Looking only for total stat. We're not interested in per-CPU stats.
		if parts[0] != "cpu" {
			continue
		}

		count, err := fmt.Sscanf(
			line,
			"%s %f %f %f %f %f %f %f %f %f %f",
			&stat.Entry, &stat.User, &stat.Nice, &stat.Sys, &stat.Idle, &stat.Iowait, &stat.Irq, &stat.Softirq, &stat.Steal, &stat.Guest, &stat.GstNice,
		)

		if err != nil && err != io.EOF {
			return stat, fmt.Errorf("parse %s (cpu) failed: %s", line, err)
		}
		if count != 11 {
			return stat, fmt.Errorf("parse %s (cpu) failed: insufficient elements parsed", line)
		}

		stat.Total = stat.User + stat.Nice + stat.Sys + stat.Idle + stat.Iowait + stat.Irq + stat.Softirq + stat.Steal + stat.Guest

		// No reason to read next lines.
		break
	}

	return stat, scanner.Err()
}

func readCpuStatRemote(db *postgres.DB) (CpuStat, error) {
	var stat CpuStat
	q := `SELECT cpu,us_time::numeric,ni_time::numeric,sy_time::numeric,id_time::numeric,wa_time::numeric,hi_time::numeric,si_time::numeric,st_time::numeric,quest_time::numeric,guest_ni_time::numeric FROM pgcenter.sys_proc_stat WHERE cpu = 'cpu'`
	err := db.QueryRow(q).Scan(&stat.Entry, &stat.User, &stat.Nice, &stat.Sys, &stat.Idle,
		&stat.Iowait, &stat.Irq, &stat.Softirq, &stat.Steal, &stat.Guest, &stat.GstNice)
	if err != nil {
		return stat, err
	}

	stat.Total = stat.User + stat.Nice + stat.Sys + stat.Idle + stat.Iowait + stat.Irq + stat.Softirq + stat.Steal + stat.Guest

	return stat, nil
}

func countCpuUsage(prev CpuStat, curr CpuStat, ticks float64) CpuStat {
	var stat CpuStat
	itv := curr.Total - prev.Total

	stat.User = sValue(prev.User, curr.User, itv, ticks)
	stat.Nice = sValue(prev.Nice, curr.Nice, itv, ticks)
	stat.Sys = sValue(prev.Sys, curr.Sys, itv, ticks)
	stat.Idle = sValue(prev.Idle, curr.Idle, itv, ticks)
	stat.Iowait = sValue(prev.Iowait, curr.Iowait, itv, ticks)
	stat.Irq = sValue(prev.Irq, curr.Irq, itv, ticks)
	stat.Softirq = sValue(prev.Softirq, curr.Softirq, itv, ticks)
	stat.Steal = sValue(prev.Steal, curr.Steal, itv, ticks)

	return stat
}
