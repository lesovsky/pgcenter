// Stuff related to diskstats which is located at /proc/diskstats.

package stat

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"io/ioutil"
)

// Used for storing stats per single device
type Diskstat struct {
	/* diskstats basic */
	Major, Minor int     // 1 - major number; 2 - minor mumber
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

// Container for all stats from proc-file
type Diskstats []Diskstat

// Container for previous, current and delta snapshots of stats
type Iostat struct {
	CurrDiskstats Diskstats
	PrevDiskstats Diskstats
	DiffDiskstats Diskstats
}

const (
	// The file provides IO statistics of block devices. For more details refer to Linux kernel's Documentation/iostats.txt.
	PROC_DISKSTATS       = "/proc/diskstats"
	pgProcDiskstatsQuery = "SELECT * FROM pgcenter.sys_proc_diskstats ORDER BY (maj,min)"
)

// Create a stats container of specified size
func (c *Iostat) New(size int) {
	c.CurrDiskstats = make(Diskstats, size)
	c.PrevDiskstats = make(Diskstats, size)
	c.DiffDiskstats = make(Diskstats, size)
}

// Read stats into container
func (c Diskstats) Read(conn *sql.DB, isLocal bool) error {
	if isLocal {
		if err := c.ReadLocal(); err != nil {
			return err
		}
	} else {
		c.ReadRemote(conn)
	}

	return nil
}

// Read stats from local procfs source
func (c Diskstats) ReadLocal() error {
	content, err := ioutil.ReadFile(PROC_DISKSTATS)
	if err != nil {
		return fmt.Errorf("failed to read %s", PROC_DISKSTATS)
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

		_, err = fmt.Sscanln(string(line),
			&ios.Major, &ios.Minor, &ios.Device,
			&ios.Rcompleted, &ios.Rmerged, &ios.Rsectors, &ios.Rspent,
			&ios.Wcompleted, &ios.Wmerged, &ios.Wsectors, &ios.Wspent,
			&ios.Ioinprogress, &ios.Tspent, &ios.Tweighted)
		if err != nil {
			return fmt.Errorf("failed to scan data from %s", PROC_DISKSTATS)
		}

		ios.Uptime = uptime
		c[i] = ios
	}

	return nil
}

// Read stats from remote SQL schema
func (c Diskstats) ReadRemote(conn *sql.DB) {
	var uptime float64
	conn.QueryRow(pgProcUptimeQuery).Scan(&uptime)

	rows, err := conn.Query(pgProcDiskstatsQuery)
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

// Compare stats between two containers and create delta
func (d Diskstats) Diff(c Diskstats, p Diskstats) {
	for i := 0; i < len(c); i++ {
		// Skip inactive devices
		if c[i].Rcompleted+c[i].Wcompleted == 0 {
			continue
		}

		itv := c[i].Uptime - p[i].Uptime
		d[i].Device = c[i].Device
		d[i].Completed = c[i].Rcompleted + c[i].Wcompleted

		d[i].Util = s_value(p[i].Tspent, c[i].Tspent, itv, SysTicks) / 10

		if ((c[i].Rcompleted + c[i].Wcompleted) - (p[i].Rcompleted + p[i].Wcompleted)) > 0 {
			d[i].Await = ((c[i].Rspent - p[i].Rspent) + (c[i].Wspent - p[i].Wspent)) /
				((c[i].Rcompleted + c[i].Wcompleted) - (p[i].Rcompleted + p[i].Wcompleted))
		} else {
			d[i].Await = 0
		}

		if ((c[i].Rcompleted + c[i].Wcompleted) - (p[i].Rcompleted + p[i].Wcompleted)) > 0 {
			d[i].Arqsz = ((c[i].Rsectors - p[i].Rsectors) + (c[i].Wsectors - p[i].Wsectors)) /
				((c[i].Rcompleted + c[i].Wcompleted) - (p[i].Rcompleted + p[i].Wcompleted))
		} else {
			d[i].Arqsz = 0
		}

		if (c[i].Rcompleted - p[i].Rcompleted) > 0 {
			d[i].Rawait = (c[i].Rspent - p[i].Rspent) / (c[i].Rcompleted - p[i].Rcompleted)
		} else {
			d[i].Rawait = 0
		}

		if (c[i].Wcompleted - p[i].Wcompleted) > 0 {
			d[i].Wawait = (c[i].Wspent - p[i].Wspent) / (c[i].Wcompleted - p[i].Wcompleted)
		} else {
			d[i].Wawait = 0
		}

		d[i].Rmerged = s_value(p[i].Rmerged, c[i].Rmerged, itv, SysTicks)
		d[i].Wmerged = s_value(p[i].Wmerged, c[i].Wmerged, itv, SysTicks)
		d[i].Rcompleted = s_value(p[i].Rcompleted, c[i].Rcompleted, itv, SysTicks)
		d[i].Wcompleted = s_value(p[i].Wcompleted, c[i].Wcompleted, itv, SysTicks)
		d[i].Rsectors = s_value(p[i].Rsectors, c[i].Rsectors, itv, SysTicks) / 2048
		d[i].Wsectors = s_value(p[i].Wsectors, c[i].Wsectors, itv, SysTicks) / 2048
		d[i].Tweighted = s_value(p[i].Tweighted, c[i].Tweighted, itv, SysTicks) / 1000
	}
}

// Print stats from specified container
func (d Diskstats) Print() {
	for i := 0; i < len(d); i++ {
		if d[i].Completed == 0 {
			continue
		}

		fmt.Printf("%6s\t\t%8.2f %8.2f %8.2f %8.2f %8.2f %8.2f %8.2f %8.2f %8.2f %8.2f %8.2f %8.2f\n",
			d[i].Device,
			d[i].Rmerged, d[i].Wmerged,
			d[i].Rcompleted, d[i].Wcompleted,
			d[i].Rsectors, d[i].Wsectors, d[i].Arqsz, d[i].Tweighted,
			d[i].Await, d[i].Rawait, d[i].Wawait,
			d[i].Util)
	}
}

// Function returns value of particular stat of a block device
func (d Diskstat) SingleStat(stat string) (value float64) {
	switch stat {
	case "rcompleted":
		value = d.Rcompleted
	case "rmerged":
		value = d.Rmerged
	case "rsectors":
		value = d.Rsectors
	case "rspent":
		value = d.Rspent
	case "wcompleted":
		value = d.Wspent
	case "wmerged":
		value = d.Wmerged
	case "wsectors":
		value = d.Wsectors
	case "wspent":
		value = d.Wspent
	case "ioinprogress":
		value = d.Ioinprogress
	case "tspent":
		value = d.Tspent
	case "tweighted":
		value = d.Tweighted
	case "uptime":
		value = d.Uptime
	default:
		value = 0
	}
	return value
}
