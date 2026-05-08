// Stuff related to CPU usage stats

package stat

import (
	"bufio"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CPUStat describes CPU statistics based on /proc/stat.
type CPUStat struct {
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

// readCpuStat returns CPU stats based on type of passed DB connection.
func readCPUStat(db *postgres.DB, schemaExists bool) (CPUStat, error) {
	if db.Local {
		return readCPUStatLocal("/proc/stat")
	} else if schemaExists {
		return readCPUStatRemote(db)
	}

	return CPUStat{}, nil
}

// readCpuStatLocal returns CPU stats read from local proc file.
func readCPUStatLocal(statfile string) (CPUStat, error) {
	var stat CPUStat
	f, err := os.Open(filepath.Clean(statfile))
	if err != nil {
		return stat, err
	}
	defer func() {
		_ = f.Close()
	}()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		// Looking only for total stat, skip per-CPU stats.
		if parts[0] != "cpu" {
			continue
		}

		count, err := fmt.Sscanf(
			line,
			"%s %f %f %f %f %f %f %f %f %f %f",
			&stat.Entry, &stat.User, &stat.Nice, &stat.Sys, &stat.Idle, &stat.Iowait, &stat.Irq, &stat.Softirq, &stat.Steal, &stat.Guest, &stat.GstNice,
		)

		if err != nil && err != io.EOF {
			return stat, fmt.Errorf("%s bad content: %w", statfile, err)
		}
		if count != 11 {
			return stat, fmt.Errorf("%s bad content: not enough fields in '%s'", statfile, line)
		}

		stat.Total = stat.User + stat.Nice + stat.Sys + stat.Idle + stat.Iowait + stat.Irq + stat.Softirq + stat.Steal + stat.Guest

		// No reason to read next lines.
		break
	}

	return stat, scanner.Err()
}

// readCpuStatRemote returns CPU stats from SQL stats schema.
func readCPUStatRemote(db *postgres.DB) (CPUStat, error) {
	var stat CPUStat
	q := `SELECT cpu,us_time::numeric,ni_time::numeric,sy_time::numeric,id_time::numeric,wa_time::numeric,hi_time::numeric,si_time::numeric,st_time::numeric,quest_time::numeric,guest_ni_time::numeric FROM pgcenter.sys_proc_stat WHERE cpu = 'cpu'`
	err := db.QueryRow(q).Scan(&stat.Entry, &stat.User, &stat.Nice, &stat.Sys, &stat.Idle,
		&stat.Iowait, &stat.Irq, &stat.Softirq, &stat.Steal, &stat.Guest, &stat.GstNice)
	if err != nil {
		return stat, err
	}

	stat.Total = stat.User + stat.Nice + stat.Sys + stat.Idle + stat.Iowait + stat.Irq + stat.Softirq + stat.Steal + stat.Guest

	return stat, nil
}

// countCpuUsage compares CPU stats snapshots and returns CPU usage stats over time interval.
func countCPUUsage(prev CPUStat, curr CPUStat, ticks float64) CPUStat {
	var stat CPUStat
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
