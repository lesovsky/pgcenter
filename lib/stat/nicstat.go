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

// Container for stats per single interface
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
	// enchanced
	Packets     float64 /* total number of received or transmited packets */
	Raverage    float64 /* average size of received packets */
	Taverage    float64 /* average size of transmitted packets */
	Saturation  float64 /* saturation - the number of errors/second seen for the interface */
	Rutil       float64 /* percentage utilization for bytes received */
	Tutil       float64 /* percentage utilization for bytes transmitted */
	Utilization float64 /* percentage utilization of the interface */
	Uptime      float64 /* system uptime */
}

// Container for all stats from proc-file
type Netdevs []Netdev

// Container for previous, current and delta snapshots of stats
type Nicstat struct {
	CurrNetdevs Netdevs
	PrevNetdevs Netdevs
	DiffNetdevs Netdevs
}

const (
	PROC_NETDEV             = "/proc/net/dev"
	pgProcLinkSettingsQuery = "SELECT speed::bigint * 1000000, duplex::bigint FROM pgcenter.get_netdev_link_settings($1);"
	pgProcNetdevQuery       = "SELECT left(iface,-1),* FROM pgcenter.sys_proc_netdev ORDER BY iface"
)

// Create a stats container of specified size
func (c *Nicstat) New(size int) {
	c.CurrNetdevs = make(Netdevs, size)
	c.PrevNetdevs = make(Netdevs, size)
	c.DiffNetdevs = make(Netdevs, size)
}

// Read stats into container
func (c Netdevs) Read(conn *sql.DB, isLocal bool) error {
	if isLocal {
		if err := c.ReadLocal(); err != nil {
			return err
		}
	} else {
		c.ReadRemote(conn)
	}

	return nil
}

// Read stats from local procfile source
func (c Netdevs) ReadLocal() error {
	var j = 0
	content, err := ioutil.ReadFile(PROC_NETDEV)
	if err != nil {
		return fmt.Errorf("failed to read %s", PROC_NETDEV)
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
			return fmt.Errorf("failed to scan data from %s", PROC_NETDEV)
		}

		ifs.Saturation = ifs.Rerrs + ifs.Rdrop + ifs.Tdrop + ifs.Tfifo + ifs.Tcolls + ifs.Tcarrier

		ifs.Uptime = uptime

		// Get interface's speed and duplex, perhaps it's too expensive to poll interface in every execution of the function.
		ifs.Speed, ifs.Duplex, _ = GetLinkSettings(ifs.Ifname) /* use zeros if errors */

		c[i] = ifs
	}
	return nil
}

// Read stats from remote SQL schema
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

// Compare stats between two container and create delta
func (d Netdevs) Diff(c Netdevs, p Netdevs) {
	for i := 0; i < len(c); i++ {
		// Skip inactive interfaces
		if c[i].Rpackets+c[i].Tpackets == 0 {
			continue
		}

		itv := c[i].Uptime - p[i].Uptime
		d[i].Ifname = c[i].Ifname
		d[i].Rbytes = s_value(p[i].Rbytes, c[i].Rbytes, itv, SysTicks)
		d[i].Tbytes = s_value(p[i].Tbytes, c[i].Tbytes, itv, SysTicks)
		d[i].Rpackets = s_value(p[i].Rpackets, c[i].Rpackets, itv, SysTicks)
		d[i].Tpackets = s_value(p[i].Tpackets, c[i].Tpackets, itv, SysTicks)
		d[i].Rerrs = s_value(p[i].Rerrs, c[i].Rerrs, itv, SysTicks)
		d[i].Terrs = s_value(p[i].Terrs, c[i].Terrs, itv, SysTicks)
		d[i].Tcolls = s_value(p[i].Tcolls, c[i].Tcolls, itv, SysTicks)
		d[i].Saturation = s_value(p[i].Saturation, c[i].Saturation, itv, SysTicks)

		d[i].Speed = c[i].Speed
		d[i].Duplex = c[i].Duplex

		if d[i].Rpackets > 0 {
			d[i].Raverage = d[i].Rbytes / d[i].Rpackets
		} else {
			d[i].Raverage = 0
		}
		if d[i].Tpackets > 0 {
			d[i].Taverage = d[i].Tbytes / d[i].Tpackets
		} else {
			d[i].Taverage = 0
		}

		d[i].Packets = c[i].Rpackets + c[i].Tpackets

		/* Calculate utilization */
		if c[i].Speed > 0 {
			/* The following have a mysterious "800", it is 100 for the % conversion, and 8 for bytes2bits. */
			d[i].Rutil = math.Min(d[i].Rbytes*800/float64(c[i].Speed), 100)
			d[i].Tutil = math.Min(d[i].Tbytes*800/float64(c[i].Speed), 100)

			switch c[i].Duplex {
			case DUPLEX_FULL:
				d[i].Utilization = math.Max(d[i].Rutil, d[i].Tutil)
			case DUPLEX_HALF:
				d[i].Utilization = math.Min((d[i].Rbytes+d[i].Tbytes)*800/float64(c[i].Speed), 100)
			case DUPLEX_UNKNOWN:
			}
		} else {
			d[i].Rutil, d[i].Tutil, d[i].Utilization = 0, 0, 0
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

// Function returns value of particular stat of an interface
func (c Netdev) SingleStat(stat string) (value float64) {
	switch stat {
	case "rbytes":
		value = c.Rbytes
	case "rpackets":
		value = c.Rpackets
	case "rerrs":
		value = c.Rerrs
	case "rdrop":
		value = c.Rdrop
	case "rfifo":
		value = c.Rfifo
	case "rframe":
		value = c.Rframe
	case "rcompressed":
		value = c.Rcompressed
	case "rmulticast":
		value = c.Rmulticast
	case "tbytes":
		value = c.Tbytes
	case "tpackets":
		value = c.Tpackets
	case "terrs":
		value = c.Terrs
	case "tdrop":
		value = c.Tdrop
	case "tfifo":
		value = c.Tfifo
	case "tcolls":
		value = c.Tcolls
	case "tcarrier":
		value = c.Tcarrier
	case "tcompressed":
		value = c.Tcompressed
	case "saturation":
		value = c.Saturation
	case "uptime":
		value = c.Uptime
	case "speed":
		value = float64(c.Speed)
	case "duplex":
		value = float64(c.Duplex)
	default:
		value = 0
	}
	return value
}
