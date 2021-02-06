package stat

import (
	"bufio"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// pgProcDiskstatsQuery is the SQL for retrieving IO stats from Postgres instance
	pgProcDiskstatsQuery = "SELECT * FROM pgcenter.sys_proc_diskstats ORDER BY (maj,min)"
)

// Diskstat describes pre-device IO statistics based on /proc/diskstats.
// See details https://www.kernel.org/doc/Documentation/ABI/testing/procfs-diskstats
type Diskstat struct {
	/* basic */
	Major, Minor int     // 1 - major number; 2 - minor number
	Device       string  // 3 - device name
	Rcompleted   float64 // 4 - reads completed successfully
	Rmerged      float64 // 5 - reads merged
	Rsectors     float64 // 6 - sectors read
	Rspent       float64 // 7 - time spent reading (ms)
	Wcompleted   float64 // 8 - writes completed
	Wmerged      float64 // 9 - writes merged
	Wsectors     float64 // 10 - sectors written
	Wspent       float64 // 11 - time spent writing (ms)
	Ioinprogress float64 // 12 - I/Os currently in progress
	Tspent       float64 // 13 - time spent doing I/Os (ms)
	Tweighted    float64 // 14 - weighted time spent doing I/Os (ms)
	Dcompleted   float64 // 15 - discards completed successfully
	Dmerged      float64 //	16 - discards merged
	Dsectors     float64 //	17 - sectors discarded
	Dspent       float64 //	18 - time spent discarding
	Fcompleted   float64 // 19 - flush requests completed successfully
	Fspent       float64 // 20 - time spent flushing
	/* extended, based on basic */
	// System uptime, used for usage calculation.
	Uptime float64
	// Total number of completed read and write requests.
	Completed float64
	// Average time (ms) of read requests issued to the device to be served.
	// This includes the time spent by the requests in queue and the time spent servicing them.
	Rawait float64
	// Average time (ms) of write requests issued to the device to be served.
	// This includes the time spent by the requests in queue and the time spent servicing them.
	Wawait float64
	// Average time (in ms) of read/write requests issued to the device to be served.
	// This includes the time spent by the requests in queue and the time spent servicing them.
	Await float64
	// Average size (in sectors) of the requests that were issued to the device.
	Arqsz float64
	// Percentage of elapsed time during which I/O requests were issued to the device (bandwidth utilization for the device).
	// Device saturation occurs when this value is close to 100% for devices serving requests sequentially. For devices
	// serving requests concurrently, such as RAID/SSD/NVMe, this number does not reflect its performance limits.
	Util float64
}

// Diskstats is the container for all stats related to all block devices.
type Diskstats []Diskstat

// readDiskstats returns block devices stats depending on type of passed DB connection.
func readDiskstats(db *postgres.DB, config Config) (Diskstats, error) {
	if db.Local {
		return readDiskstatsLocal("/proc/diskstats", config.ticks)
	} else if config.SchemaPgcenterAvail {
		return readDiskstatsRemote(db)
	}

	return Diskstats{}, nil
}

// readDiskstatsLocal return block devices stats read from local proc file.
func readDiskstatsLocal(statfile string, ticks float64) (Diskstats, error) {
	var stat Diskstats
	f, err := os.Open(filepath.Clean(statfile))
	if err != nil {
		return stat, err
	}
	defer func() {
		_ = f.Close()
	}()

	uptime, err := readUptimeLocal("/proc/uptime", ticks)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		values := strings.Fields(line)

		// Linux kernel <= 4.18 have 14 columns, 4.18+ have 18, 5.5+ have 20 columns
		// for details see https://www.kernel.org/doc/Documentation/ABI/testing/procfs-diskstats)
		if len(values) != 14 && len(values) != 18 && len(values) != 20 {
			return nil, fmt.Errorf("%s bad content: unknown file format, wrong number of columns in line: %s", statfile, line)
		}

		var d = Diskstat{}

		switch len(values) {
		case 14:
			_, err = fmt.Sscan(line,
				&d.Major, &d.Minor, &d.Device,
				&d.Rcompleted, &d.Rmerged, &d.Rsectors, &d.Rspent, &d.Wcompleted, &d.Wmerged, &d.Wsectors, &d.Wspent,
				&d.Ioinprogress, &d.Tspent, &d.Tweighted,
			)
		case 18:
			_, err = fmt.Sscan(line,
				&d.Major, &d.Minor, &d.Device,
				&d.Rcompleted, &d.Rmerged, &d.Rsectors, &d.Rspent, &d.Wcompleted, &d.Wmerged, &d.Wsectors, &d.Wspent,
				&d.Ioinprogress, &d.Tspent, &d.Tweighted, &d.Dcompleted, &d.Dmerged, &d.Dsectors, &d.Dspent,
			)
		case 20:
			_, err = fmt.Sscan(line,
				&d.Major, &d.Minor, &d.Device,
				&d.Rcompleted, &d.Rmerged, &d.Rsectors, &d.Rspent, &d.Wcompleted, &d.Wmerged, &d.Wsectors, &d.Wspent,
				&d.Ioinprogress, &d.Tspent, &d.Tweighted, &d.Dcompleted, &d.Dmerged, &d.Dsectors, &d.Dspent,
				&d.Fcompleted, &d.Fspent,
			)
		default:
			// should not be here, but anyway check for that
			err = fmt.Errorf("unknown file format, wrong number of columns in line: %s", line)
		}
		if err != nil {
			return nil, fmt.Errorf("%s bad content: %w", statfile, err)
		}

		// skip pseudo block devices.
		re := regexp.MustCompile(`^(ram|loop|fd)`)
		if re.MatchString(d.Device) {
			continue
		}

		d.Uptime = uptime
		stat = append(stat, d)
	}

	return stat, nil
}

// readDiskstatsRemote returns block devices stats from SQL stats schema.
func readDiskstatsRemote(db *postgres.DB) (Diskstats, error) {
	var uptime float64
	err := db.QueryRow(pgProcUptimeQuery).Scan(&uptime)
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(pgProcDiskstatsQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stat Diskstats
	for rows.Next() {
		var d = Diskstat{}

		err := rows.Scan(&d.Major, &d.Minor, &d.Device,
			&d.Rcompleted, &d.Rmerged, &d.Rsectors, &d.Rspent,
			&d.Wcompleted, &d.Wmerged, &d.Wsectors, &d.Wspent,
			&d.Ioinprogress, &d.Tspent, &d.Tweighted,
			&d.Dcompleted, &d.Dmerged, &d.Dsectors, &d.Dspent,
			&d.Fcompleted, &d.Fspent)
		if err != nil {
			return nil, err
		}

		// skip pseudo block devices.
		re := regexp.MustCompile(`^(ram|loop|fd)`)
		if re.MatchString(d.Device) {
			continue
		}

		d.Uptime = uptime
		stat = append(stat, d)
	}

	return stat, nil
}

// countDiskstatsUsage compares block devices stats snapshots and returns devices usage stats over time interval.
func countDiskstatsUsage(prev Diskstats, curr Diskstats, ticks float64) Diskstats {
	if len(curr) != len(prev) {
		// do nothing and return
		return nil
	}

	stat := make([]Diskstat, len(curr))

	for i := 0; i < len(curr); i++ {
		// Skip inactive devices.
		if curr[i].Rcompleted+curr[i].Wcompleted == 0 {
			continue
		}

		stat[i].Major = curr[i].Major
		stat[i].Minor = curr[i].Minor
		stat[i].Device = curr[i].Device
		itv := curr[i].Uptime - prev[i].Uptime

		stat[i].Completed = curr[i].Rcompleted + curr[i].Wcompleted

		stat[i].Util = sValue(prev[i].Tspent, curr[i].Tspent, itv, ticks) / 10

		if ((curr[i].Rcompleted + curr[i].Wcompleted) - (prev[i].Rcompleted + prev[i].Wcompleted)) > 0 {
			stat[i].Await = ((curr[i].Rspent - prev[i].Rspent) + (curr[i].Wspent - prev[i].Wspent)) /
				((curr[i].Rcompleted + curr[i].Wcompleted) - (prev[i].Rcompleted + prev[i].Wcompleted))
		} else {
			stat[i].Await = 0
		}

		if ((curr[i].Rcompleted + curr[i].Wcompleted) - (prev[i].Rcompleted + prev[i].Wcompleted)) > 0 {
			stat[i].Arqsz = ((curr[i].Rsectors - prev[i].Rsectors) + (curr[i].Wsectors - prev[i].Wsectors)) /
				((curr[i].Rcompleted + curr[i].Wcompleted) - (prev[i].Rcompleted + prev[i].Wcompleted))
		} else {
			stat[i].Arqsz = 0
		}

		if (curr[i].Rcompleted - prev[i].Rcompleted) > 0 {
			stat[i].Rawait = (curr[i].Rspent - prev[i].Rspent) / (curr[i].Rcompleted - prev[i].Rcompleted)
		} else {
			stat[i].Rawait = 0
		}

		if (curr[i].Wcompleted - prev[i].Wcompleted) > 0 {
			stat[i].Wawait = (curr[i].Wspent - prev[i].Wspent) / (curr[i].Wcompleted - prev[i].Wcompleted)
		} else {
			stat[i].Wawait = 0
		}

		stat[i].Rmerged = sValue(prev[i].Rmerged, curr[i].Rmerged, itv, ticks)
		stat[i].Wmerged = sValue(prev[i].Wmerged, curr[i].Wmerged, itv, ticks)
		stat[i].Rcompleted = sValue(prev[i].Rcompleted, curr[i].Rcompleted, itv, ticks)
		stat[i].Wcompleted = sValue(prev[i].Wcompleted, curr[i].Wcompleted, itv, ticks)
		stat[i].Rsectors = sValue(prev[i].Rsectors, curr[i].Rsectors, itv, ticks) / 2048
		stat[i].Wsectors = sValue(prev[i].Wsectors, curr[i].Wsectors, itv, ticks) / 2048
		stat[i].Tweighted = sValue(prev[i].Tweighted, curr[i].Tweighted, itv, ticks) / 1000
	}

	return stat
}
