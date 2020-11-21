// Stuff related to diskstats which is located at /proc/diskstats.

package stat

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"io"
	"io/ioutil"
)

// Diskstat is the container for storing stats per single block device
type Diskstat struct {
	/* diskstats basic */
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
	/* diskstats advanced */
	Uptime    float64 // system uptime, used for interval calculation
	Completed float64 // reads and writes completed
	Rawait    float64 // average time (in milliseconds) for read requests issued to the device to be served. This includes the time spent by the requests in queue and the time spent servicing them.
	Wawait    float64 // average time (in milliseconds) for write requests issued to the device to be served. This includes the time spent by the requests in queue and the time spent servicing them.
	Await     float64 // average time (in milliseconds) for I/O requests issued to the device to be served. This includes the time spent by the requests in queue and the time spent servicing them.
	Arqsz     float64 // average size (in sectors) of the requests that were issued to the device.
	Util      float64 // percentage of elapsed time during which I/O requests were issued to the device (bandwidth utilization for the device). Device saturation occurs when this value is close to 100% for devices serving requests serially.
	// But for devices serving requests in parallel, such as RAID arrays and modern SSDs, this number does not reflect their performance limits.
}

// Diskstats is the container for all stats related to all block devices
type Diskstats []Diskstat

// Iostat is the container for previous, current and delta snapshots of stats
type Iostat struct {
	CurrDiskstats Diskstats
	PrevDiskstats Diskstats
	DiffDiskstats Diskstats
}

const (
	// ProcDiskstats provides IO statistics of block devices. For more details refer to Linux kernel's Documentation/iostats.txt.
	ProcDiskstats = "/proc/diskstats"
	// pgProcDiskstatsQuery is the SQL for retrieving IO stats from Postgres instance
	pgProcDiskstatsQuery = "SELECT * FROM pgcenter.sys_proc_diskstats ORDER BY (maj,min)"
)

// New creates a stats container of specified size
func (c *Iostat) New(size int) {
	c.CurrDiskstats = make(Diskstats, size)
	c.PrevDiskstats = make(Diskstats, size)
	c.DiffDiskstats = make(Diskstats, size)
}

// Read method reads stats from the source
func (c Diskstats) Read(db *postgres.DB, pgcAvail bool) error {
	if db.Local {
		if err := c.ReadLocal(); err != nil {
			return err
		}
	} else if pgcAvail {
		c.ReadRemote(db)
	}

	return nil
}

// ReadLocal method reads stats from local 'procfs' filesystem
func (c Diskstats) ReadLocal() error {
	content, err := ioutil.ReadFile(ProcDiskstats)
	if err != nil {
		return fmt.Errorf("failed to read %s", ProcDiskstats)
	}
	reader := bufio.NewReader(bytes.NewBuffer(content))

	uptime, err := uptime()
	if err != nil {
		return err
	}
	for i := 0; i < len(c); i++ {
		line, _, err := reader.ReadLine()
		if err == io.EOF {
			break
		}
		var ios = Diskstat{}

		_, err = fmt.Sscan(string(line),
			&ios.Major, &ios.Minor, &ios.Device,
			&ios.Rcompleted, &ios.Rmerged, &ios.Rsectors, &ios.Rspent,
			&ios.Wcompleted, &ios.Wmerged, &ios.Wsectors, &ios.Wspent,
			&ios.Ioinprogress, &ios.Tspent, &ios.Tweighted)
		if err != nil {
			return fmt.Errorf("failed to scan data from %s", ProcDiskstats)
		}

		ios.Uptime = uptime
		c[i] = ios
	}

	return nil
}

// ReadRemote method reads stats from remote Postgres instance
func (c Diskstats) ReadRemote(db *postgres.DB) {
	var uptime float64
	db.QueryRow(pgProcUptimeQuery).Scan(&uptime)

	rows, err := db.Query(pgProcDiskstatsQuery)
	if err != nil {
		return
	} /* ignore errors, zero stat is ok for us */
	defer rows.Close()

	var i int
	for rows.Next() {
		var ios = Diskstat{}

		err := rows.Scan(&ios.Major, &ios.Minor, &ios.Device,
			&ios.Rcompleted, &ios.Rmerged, &ios.Rsectors, &ios.Rspent,
			&ios.Wcompleted, &ios.Wmerged, &ios.Wsectors, &ios.Wspent,
			&ios.Ioinprogress, &ios.Tspent, &ios.Tweighted)
		if err != nil {
			return
		}

		ios.Uptime = uptime
		c[i] = ios
		i++
	}
}

// Diff method compares stats snapshots and creates delta
func (c Diskstats) Diff(curr Diskstats, prev Diskstats) {
	var ticks float64 = 100

	for i := 0; i < len(curr); i++ {
		// Skip inactive devices
		if curr[i].Rcompleted+curr[i].Wcompleted == 0 {
			continue
		}

		itv := curr[i].Uptime - prev[i].Uptime
		c[i].Device = curr[i].Device
		c[i].Completed = curr[i].Rcompleted + curr[i].Wcompleted

		c[i].Util = sValue(prev[i].Tspent, curr[i].Tspent, itv, ticks) / 10

		if ((curr[i].Rcompleted + curr[i].Wcompleted) - (prev[i].Rcompleted + prev[i].Wcompleted)) > 0 {
			c[i].Await = ((curr[i].Rspent - prev[i].Rspent) + (curr[i].Wspent - prev[i].Wspent)) /
				((curr[i].Rcompleted + curr[i].Wcompleted) - (prev[i].Rcompleted + prev[i].Wcompleted))
		} else {
			c[i].Await = 0
		}

		if ((curr[i].Rcompleted + curr[i].Wcompleted) - (prev[i].Rcompleted + prev[i].Wcompleted)) > 0 {
			c[i].Arqsz = ((curr[i].Rsectors - prev[i].Rsectors) + (curr[i].Wsectors - prev[i].Wsectors)) /
				((curr[i].Rcompleted + curr[i].Wcompleted) - (prev[i].Rcompleted + prev[i].Wcompleted))
		} else {
			c[i].Arqsz = 0
		}

		if (curr[i].Rcompleted - prev[i].Rcompleted) > 0 {
			c[i].Rawait = (curr[i].Rspent - prev[i].Rspent) / (curr[i].Rcompleted - prev[i].Rcompleted)
		} else {
			c[i].Rawait = 0
		}

		if (curr[i].Wcompleted - prev[i].Wcompleted) > 0 {
			c[i].Wawait = (curr[i].Wspent - prev[i].Wspent) / (curr[i].Wcompleted - prev[i].Wcompleted)
		} else {
			c[i].Wawait = 0
		}

		c[i].Rmerged = sValue(prev[i].Rmerged, curr[i].Rmerged, itv, ticks)
		c[i].Wmerged = sValue(prev[i].Wmerged, curr[i].Wmerged, itv, ticks)
		c[i].Rcompleted = sValue(prev[i].Rcompleted, curr[i].Rcompleted, itv, ticks)
		c[i].Wcompleted = sValue(prev[i].Wcompleted, curr[i].Wcompleted, itv, ticks)
		c[i].Rsectors = sValue(prev[i].Rsectors, curr[i].Rsectors, itv, ticks) / 2048
		c[i].Wsectors = sValue(prev[i].Wsectors, curr[i].Wsectors, itv, ticks) / 2048
		c[i].Tweighted = sValue(prev[i].Tweighted, curr[i].Tweighted, itv, ticks) / 1000
	}
}

// Print method prints IO stats
func (c Diskstats) Print() {
	for i := 0; i < len(c); i++ {
		if c[i].Completed == 0 {
			continue
		}

		fmt.Printf("%6s\t\t%8.2f %8.2f %8.2f %8.2f %8.2f %8.2f %8.2f %8.2f %8.2f %8.2f %8.2f %8.2f\n",
			c[i].Device,
			c[i].Rmerged, c[i].Wmerged,
			c[i].Rcompleted, c[i].Wcompleted,
			c[i].Rsectors, c[i].Wsectors, c[i].Arqsz, c[i].Tweighted,
			c[i].Await, c[i].Rawait, c[i].Wawait,
			c[i].Util)
	}
}
