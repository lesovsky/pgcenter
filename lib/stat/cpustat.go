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
	PROC_STAT          = "/proc/stat"
	pgProcCpuStatQuery = "SELECT * FROM pgcenter.sys_proc_stat WHERE cpu = 'cpu'"
)

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

/* Struct for calculated CPU usage stats */
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

type Cpustat struct {
	CurrCpuSample CpuRawstat
	PrevCpuSample CpuRawstat
	CpuUsage
}

func (s *CpuRawstat) Read(conn *sql.DB, isLocal bool) {
	if isLocal {
		s.ReadLocal()
	} else {
		s.ReadRemote(conn)
	}
}

/* Read CPU usage raw values from statfile and save to pre-calculation struct */
func (s *CpuRawstat) ReadLocal() {
	content, err := ioutil.ReadFile(PROC_STAT)
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

func (s *CpuRawstat) ReadRemote(conn *sql.DB) {
	conn.QueryRow(pgProcCpuStatQuery).Scan(&s.Entry, &s.User, &s.Nice, &s.Sys, &s.Idle,
		&s.Iowait, &s.Irq, &s.Softirq, &s.Steal, &s.Guest, &s.GstNice)

	s.Total = s.User + s.Nice + s.Sys + s.Idle + s.Iowait + s.Irq + s.Softirq + s.Steal + s.Guest
}

/* Calculate CPU usage human-readable stat using raw values from pre-calculation struct */
func (u *CpuUsage) Diff(prev CpuRawstat, curr CpuRawstat) {
	itv := curr.Total - prev.Total

	u.User = s_value(prev.User, curr.User, itv, SysTicks)
	u.Nice = s_value(prev.Nice, curr.Nice, itv, SysTicks)
	u.Sys = s_value(prev.Sys, curr.Sys, itv, SysTicks)
	u.Idle = s_value(prev.Idle, curr.Idle, itv, SysTicks)
	u.Iowait = s_value(prev.Iowait, curr.Iowait, itv, SysTicks)
	u.Irq = s_value(prev.Irq, curr.Irq, itv, SysTicks)
	u.Softirq = s_value(prev.Softirq, curr.Softirq, itv, SysTicks)
	u.Steal = s_value(prev.Steal, curr.Steal, itv, SysTicks)

	return
}

// Function return number of ticks for particular mode
func (s *CpuRawstat) SingleStat(mode string) (ticks float64) {
	switch mode {
	case "user":
		ticks = s.User
	case "nice":
		ticks = s.Nice
	case "system":
		ticks = s.Sys
	case "idle":
		ticks = s.Idle
	case "iowait":
		ticks = s.Iowait
	case "irq":
		ticks = s.Irq
	case "softirq":
		ticks = s.Softirq
	case "steal":
		ticks = s.Steal
	case "guest":
		ticks = s.Guest
	case "guest_nice":
		ticks = s.GstNice
	case "total":
		ticks = s.Total
	default:
		ticks = 0
	}
	return ticks
}
