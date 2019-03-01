// Stuff related to network interfaces stats

package stat

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"io/ioutil"
	"math"
)

// Netdev is the container for stats related to a single network interface
type Netdev struct {
	Ifname string /* interface name */
	Speed  uint32 /* interface network speed */
	Duplex uint8  /* interface duplex */
	// receive
	Rbytes      float64 /* total number of received bytes */
	Rpackets    float64 /* total number of received packets */
	Rerrs       float64 /* total number of receive errors */
	Rdrop       float64 /* total number of dropped packets */
	Rfifo       float64 /* total number of fifo buffers errors */
	Rframe      float64 /* total number of packet framing errors */
	Rcompressed float64 /* total number of received compressed packets */
	Rmulticast  float64 /* total number of received multicast packets */
	// transmit
	Tbytes      float64 /* total number of transmitted bytes */
	Tpackets    float64 /* total number of transmitted packets */
	Terrs       float64 /* total number of transmitted errors */
	Tdrop       float64 /* total number of dropped packets */
	Tfifo       float64 /* total number of fifo buffers errors */
	Tcolls      float64 /* total number of detected collisions */
	Tcarrier    float64 /* total number of carrier losses */
	Tcompressed float64 /* total number of received multicast packets */
	// enhanced
	Packets     float64 /* total number of received or transmitted packets */
	Raverage    float64 /* average size of received packets */
	Taverage    float64 /* average size of transmitted packets */
	Saturation  float64 /* saturation - the number of errors/second seen for the interface */
	Rutil       float64 /* percentage utilization for bytes received */
	Tutil       float64 /* percentage utilization for bytes transmitted */
	Utilization float64 /* percentage utilization of the interface */
	Uptime      float64 /* system uptime */
}

// Netdevs is the container for all stats of all network interfaces
type Netdevs []Netdev

// Nicstat is the container for previous, current and delta snapshots of stats
type Nicstat struct {
	CurrNetdevs Netdevs
	PrevNetdevs Netdevs
	DiffNetdevs Netdevs
}

const (
	// ProcNetdevFile is the location of network interfaces statistics in 'procfs' filesystem
	ProcNetdevFile = "/proc/net/dev"
	// pgProcLinkSettingsQuery quering network interfaces' details from Postgres instance
	pgProcLinkSettingsQuery = "SELECT speed::bigint * 1000000, duplex::bigint FROM pgcenter.get_netdev_link_settings($1);"
	// pgProcNetdevQuery queries network interfaces stats from Postgres instance
	pgProcNetdevQuery = "SELECT left(iface,-1),* FROM pgcenter.sys_proc_netdev ORDER BY iface"
)

// New creates a stats container of specified size
func (c *Nicstat) New(size int) {
	c.CurrNetdevs = make(Netdevs, size)
	c.PrevNetdevs = make(Netdevs, size)
	c.DiffNetdevs = make(Netdevs, size)
}

// Read stats into container
func (c Netdevs) Read(conn *sql.DB, isLocal bool, pgcAvail bool) error {
	if isLocal {
		if err := c.ReadLocal(); err != nil {
			return err
		}
	} else if pgcAvail {
		c.ReadRemote(conn)
	}

	return nil
}

// ReadLocal reads stats from local 'procfs' filesystem
func (c Netdevs) ReadLocal() error {
	var j = 0
	content, err := ioutil.ReadFile(ProcNetdevFile)
	if err != nil {
		return fmt.Errorf("failed to read %s", ProcNetdevFile)
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
		if j < 2 { // skip first 2 lines - it's stats header
			j++
			i--
			continue
		}

		var ifs = Netdev{}

		_, err = fmt.Sscanln(string(line),
			&ifs.Ifname,
			&ifs.Rbytes, &ifs.Rpackets, &ifs.Rerrs, &ifs.Rdrop, &ifs.Rfifo, &ifs.Rframe, &ifs.Rcompressed, &ifs.Rmulticast,
			&ifs.Tbytes, &ifs.Tpackets, &ifs.Terrs, &ifs.Tdrop, &ifs.Tfifo, &ifs.Tcolls, &ifs.Tcarrier, &ifs.Tcompressed)
		if err != nil {
			return fmt.Errorf("failed to scan data from %s", ProcNetdevFile)
		}

		ifs.Saturation = ifs.Rerrs + ifs.Rdrop + ifs.Tdrop + ifs.Tfifo + ifs.Tcolls + ifs.Tcarrier

		ifs.Uptime = uptime

		// Get interface's speed and duplex, perhaps it's too expensive to poll interface in every execution of the function.
		ifs.Speed, ifs.Duplex, _ = GetLinkSettings(ifs.Ifname) /* use zeros if errors */

		c[i] = ifs
	}
	return nil
}

// ReadRemote reads stats from remote Postgres instance
func (c Netdevs) ReadRemote(conn *sql.DB) {
	var uptime float64
	conn.QueryRow(pgProcUptimeQuery).Scan(&uptime)

	rows, err := conn.Query(pgProcNetdevQuery)
	if err != nil {
		return
	} /* ignore errors, zero stat is ok for us */
	defer rows.Close()

	var i int
	var dummy string
	for rows.Next() {
		var ifs = Netdev{}

		err := rows.Scan(&ifs.Ifname, &dummy,
			&ifs.Rbytes, &ifs.Rpackets, &ifs.Rerrs, &ifs.Rdrop, &ifs.Rfifo, &ifs.Rframe, &ifs.Rcompressed, &ifs.Rmulticast,
			&ifs.Tbytes, &ifs.Tpackets, &ifs.Terrs, &ifs.Tdrop, &ifs.Tfifo, &ifs.Tcolls, &ifs.Tcarrier, &ifs.Tcompressed)
		if err != nil {
			return
		}

		ifs.Uptime = uptime
		// Get interface's speed and duplex, perhaps it's too expensive to poll interface in every execution of the function.
		conn.QueryRow(pgProcLinkSettingsQuery, ifs.Ifname).Scan(&ifs.Speed, &ifs.Duplex)

		c[i] = ifs
		i++
	}
}

// Diff compares stats between two container and create delta
func (c Netdevs) Diff(curr Netdevs, prev Netdevs) {
	for i := 0; i < len(curr); i++ {
		// Skip inactive interfaces
		if curr[i].Rpackets+curr[i].Tpackets == 0 {
			continue
		}

		itv := curr[i].Uptime - prev[i].Uptime
		c[i].Ifname = curr[i].Ifname
		c[i].Rbytes = sValue(prev[i].Rbytes, curr[i].Rbytes, itv, SysTicks)
		c[i].Tbytes = sValue(prev[i].Tbytes, curr[i].Tbytes, itv, SysTicks)
		c[i].Rpackets = sValue(prev[i].Rpackets, curr[i].Rpackets, itv, SysTicks)
		c[i].Tpackets = sValue(prev[i].Tpackets, curr[i].Tpackets, itv, SysTicks)
		c[i].Rerrs = sValue(prev[i].Rerrs, curr[i].Rerrs, itv, SysTicks)
		c[i].Terrs = sValue(prev[i].Terrs, curr[i].Terrs, itv, SysTicks)
		c[i].Tcolls = sValue(prev[i].Tcolls, curr[i].Tcolls, itv, SysTicks)
		c[i].Saturation = sValue(prev[i].Saturation, curr[i].Saturation, itv, SysTicks)

		c[i].Speed = curr[i].Speed
		c[i].Duplex = curr[i].Duplex

		if c[i].Rpackets > 0 {
			c[i].Raverage = c[i].Rbytes / c[i].Rpackets
		} else {
			c[i].Raverage = 0
		}
		if c[i].Tpackets > 0 {
			c[i].Taverage = c[i].Tbytes / c[i].Tpackets
		} else {
			c[i].Taverage = 0
		}

		c[i].Packets = curr[i].Rpackets + curr[i].Tpackets

		/* Calculate utilization */
		if curr[i].Speed > 0 {
			/* The following have a mysterious "800", it is 100 for the % conversion, and 8 for bytes2bits. */
			c[i].Rutil = math.Min(c[i].Rbytes*800/float64(curr[i].Speed), 100)
			c[i].Tutil = math.Min(c[i].Tbytes*800/float64(curr[i].Speed), 100)

			switch curr[i].Duplex {
			case duplexFull:
				c[i].Utilization = math.Max(c[i].Rutil, c[i].Tutil)
			case duplexHalf:
				c[i].Utilization = math.Min((c[i].Rbytes+c[i].Tbytes)*800/float64(curr[i].Speed), 100)
			case duplexUnknown:
			}
		} else {
			c[i].Rutil, c[i].Tutil, c[i].Utilization = 0, 0, 0
		}
	}
}

// Print stats from container
func (c Netdevs) Print() {
	fmt.Printf("    Interface:   rMbps   wMbps    rPk/s    wPk/s     rAvs     wAvs     IErr     OErr     Coll      Sat   %%rUtil   %%wUtil    %%Util\n")
	for i := 0; i < len(c); i++ {
		fmt.Printf("%14s%8.2f%8.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f\n",
			c[i].Ifname,
			c[i].Rbytes/1024/128, c[i].Tbytes/1024/128, // conversion to Mbps
			c[i].Rpackets, c[i].Tpackets, c[i].Raverage, c[i].Taverage,
			c[i].Rerrs, c[i].Terrs, c[i].Tcolls,
			c[i].Saturation, c[i].Rutil, c[i].Tutil, c[i].Utilization)
	}
}
