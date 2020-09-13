// Stuff related to CPU usage stats

package stat

/* Struct for raw stat data read from file/sql sources */
import (
	"bufio"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"io"
	"os"
	"strings"
)

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
