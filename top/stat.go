// Stuff related to gathering, processing and displaying stats.

package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/lib/stat"
	"github.com/lib/pq"
	"os"
	"time"
)

var (
	stats                 stat.Stat               // container for stats
	do_exit               = make(chan int)        // graceful quit of program or goroutines
	do_update             = make(chan int)        // force stat refresh/redraw (when context switched or something else)
	refreshMinGranularity = 1 * time.Second       // min resolution of stats refreshing
	refreshInterval       = refreshMinGranularity // default refreshing interval
)

// Main stat loop, which is refreshes with interval, gathers and display stats.
func statLoop(g *gocui.Gui) {
	defer wg.Done()

	for {
		select {
		case <-do_exit:
			return
		case <-do_update:
			// discard previous PGresult stats and re-read stats
			stats.PrevPGresult.Valid = false
			doWork(g)
		case <-time.After(refreshInterval):
			doWork(g)
		}
	}
}

// Read all required stats and print.
func doWork(g *gocui.Gui) {
	// Use approach with loop just in case if GetDbStat() returns error (for example, number of rows changed at
	// stats' context switching, or similar). Due to this, don't wait refreshing timeout or incoming do_update signal,
	// let start new iteration immediately.
	// But what, if GetDbStat() start to return permanent errors, and loop becomes infinite?
	for {
		stats.GetSysStat(conn, conninfo.ConnLocal)
		stats.GetPgstatActivity(conn, uint(refreshInterval/refreshMinGranularity), conninfo.ConnLocal)
		getAuxStat(g)

		// Ignore errors in template parsing; if query is bogus, user will see appropriate syntax error instead of stats.
		query, _ := stat.PrepareQuery(ctx.current.Query, ctx.sharedOptions)
		if err := stats.GetPgstatDiff(conn, query, uint(refreshInterval/refreshMinGranularity), ctx.current.DiffIntvl, ctx.current.OrderKey, ctx.current.OrderDesc, ctx.current.UniqueKey); err != nil {
			// If something is wrong, re-read stats immediately
			continue
		}

		// Reading of stats has been successful, escape from loop.
		break
	}
	printAllStat(g, stats)
}

// Print all stats.
func printAllStat(g *gocui.Gui, s stat.Stat) {
	g.Update(func(g *gocui.Gui) error {
		v, err := g.View("sysstat")
		if err != nil {
			return fmt.Errorf("Set focus on sysstat view failed: %s", err)
		}
		v.Clear()
		printSysstat(v, s)

		v, err = g.View("pgstat")
		if err != nil {
			return fmt.Errorf("Set focus on pgstat view failed: %s", err)
		}
		v.Clear()
		printPgstat(v, s)

		v, err = g.View("dbstat")
		if err != nil {
			return fmt.Errorf("Set focus on dbstat view failed: %s", err)
		}
		v.Clear()
		printDbstat(v, s)

		if ctx.aux > AUX_NONE {
			v, err := g.View("aux")
			if err != nil {
				return fmt.Errorf("Set focus on aux view failed: %s", err)
			}

			switch ctx.aux {
			case AUX_DISKSTAT:
				v.Clear()
				printIostat(v, stats.DiffDiskstats)
			case AUX_NICSTAT:
				v.Clear()
				printNicstat(v, stats.DiffNetdevs)
			case AUX_LOGTAIL:
				// don't clear screen
				printLogtail(g, v)
			}
		}
		return nil
	})
}

// Print sysstat.
func printSysstat(v *gocui.View, s stat.Stat) {
	/* line1: current time and load average */
	fmt.Fprintf(v, "pgcenter: %s, load average: %.2f, %.2f, %.2f\n",
		time.Now().Format("2006-01-02 15:04:05"),
		s.LoadAvg.One, s.LoadAvg.Five, s.LoadAvg.Fifteen)
	/* line2: cpu usage */
	fmt.Fprintf(v, "    %%cpu: %4.1f us, %4.1f sy, %4.1f ni, %4.1f id, %4.1f wa, %4.1f hi, %4.1f si, %4.1f st\n",
		s.CpuUsage.User, s.CpuUsage.Sys, s.CpuUsage.Nice, s.CpuUsage.Idle,
		s.CpuUsage.Iowait, s.CpuUsage.Irq, s.CpuUsage.Softirq, s.CpuUsage.Steal)
	/* line3: memory usage */
	fmt.Fprintf(v, " MiB mem: %6d total, %6d free, %6d used, %8d buff/cached\n",
		s.Meminfo.MemTotal, s.Meminfo.MemFree, s.Meminfo.MemUsed,
		s.Meminfo.MemCached+s.Meminfo.MemBuffers+s.Meminfo.MemSlab)
	/* line4: swap usage, dirty and writeback */
	fmt.Fprintf(v, "MiB swap: %6d total, %6d free, %6d used, %6d/%d dirty/writeback\n",
		s.Meminfo.SwapTotal, s.Meminfo.SwapFree, s.Meminfo.SwapUsed,
		s.Meminfo.MemDirty, s.Meminfo.MemWriteback)
}

// Print stats about Postgres activity.
func printPgstat(v *gocui.View, s stat.Stat) {
	/* line1: details of used connection, version, uptime and recovery status */
	fmt.Fprintf(v, "state [%s]: %.16s:%d %.16s@%.16s (ver: %s, up %s, recovery: %.1s)\n",
		s.PgInfo.PgAlive,
		conninfo.Host, conninfo.Port, conninfo.User, conninfo.Dbname,
		s.PgInfo.PgVersion, s.PgInfo.PgUptime, s.PgInfo.PgRecovery)
	/* line2: current state of connections: total, idle, idle xacts, active, waiting, others */
	fmt.Fprintf(v, "  activity:%3d/%d conns,%3d/%d prepared,%3d idle,%3d idle_xact,%3d active,%3d waiting,%3d others\n",
		s.PgActivityStat.ConnTotal, s.PgInfo.PgMaxConns, s.PgActivityStat.ConnPrepared, s.PgInfo.PgMaxPrepXacts,
		s.PgActivityStat.ConnIdle, s.PgActivityStat.ConnIdleXact, s.PgActivityStat.ConnActive,
		s.PgActivityStat.ConnWaiting, s.PgActivityStat.ConnOthers)
	/* line3: current state of autovacuum: number of workers, antiwraparound, manual vacuums and time of oldest vacuum */
	fmt.Fprintf(v, "autovacuum: %2d/%d workers/max, %2d manual, %2d wraparound, %s vac_maxtime\n",
		s.PgActivityStat.AVWorkers, s.PgInfo.PgAVMaxWorkers,
		s.PgActivityStat.AVManual, s.PgActivityStat.AVAntiwrap, s.PgActivityStat.AVMaxTime)
	/* line4: current workload*/
	fmt.Fprintf(v, "statements: %3d stmt/s, %3.3f stmt_avgtime, %s xact_maxtime, %s prep_maxtime\n",
		s.PgActivityStat.StmtPerSec, s.PgActivityStat.StmtAvgTime, s.PgActivityStat.XactMaxTime, s.PgActivityStat.PrepMaxTime)
}

// Print stats from Postgres pg_stat_* views.
func printDbstat(v *gocui.View, s stat.Stat) {
	// If query fails, show the corresponding error and return.
	if err, ok := s.CurrPGresult.Err.(*pq.Error); ok {
		fmt.Fprintf(v, "%s: %s\nDETAIL: %s\nHINT: %s", err.Severity, err.Message, err.Detail, err.Hint)
		s.CurrPGresult.Err = nil
		return
	}

	s.DiffPGresult.SetAlignCustom(1000) // we don't want truncate lines here, so just use high limit

	// is filter required?
	var filter bool
	for _, v := range ctx.current.Filters {
		if v != nil {
			filter = true
			break
		}
	}

	/* print header - filtered column mark with star; ordered column make shadowed */
	var pname string
	//for i, name := range s.CurrPGresult.Cols {		// TODO: remove before release, commented 18 sept 2018
	for i := 0; i < s.CurrPGresult.Ncols; i++ {
		name := s.CurrPGresult.Cols[i]

		// mark filtered column
		if ctx.current.Filters[i] != nil && ctx.current.Filters[i].String() != "" {
			pname = "*" + name
		} else {
			pname = name
		}

		/* mark ordered column with background color */
		if i != ctx.current.OrderKey {
			fmt.Fprintf(v, "%-*s ", s.DiffPGresult.Colmaxlen[name], pname)
		} else {
			fmt.Fprintf(v, "\033[%d;%dm%-*s \033[0m", 47, 1, s.DiffPGresult.Colmaxlen[name], pname)
		}
	}
	fmt.Fprintf(v, "\n")

	// print data
	var doPrint bool
	for colnum, rownum := 0, 0; rownum < s.DiffPGresult.Nrows; rownum, colnum = rownum+1, 0 {
		// be optimistic, we want to print the row.
		doPrint = true

		// apply filters OLD code // TODO: remove before release, commented 18 sept 2018
		//if filter {
		//	for i := 0; i < s.DiffPGresult.Ncols; i++ {
		//		if strings.Contains(s.DiffPGresult.Result[rownum][i].String, ctx.current.Filters[i]) && ctx.current.Filters[i] != "" {
		//			// pattern is not empty and it's found in the value, there is no reason to scan other values - print the whole row.
		//			doPrint = true
		//			break
		//		} else if ctx.current.Filters[i] == "" {
		//			// no pattern for this column - skip it
		//			continue
		//		} else {
		//			// pattern is not empty and it's not found in the value - assume to don't print the row,
		//			// but don't break the loop, because another pattern can be found for other values and we want to print the row.
		//			doPrint = false
		//		}
		//	}
		//}

		// apply filters using regexp
		if filter {
			for i := 0; i < s.DiffPGresult.Ncols; i++ {
				if ctx.current.Filters[i] != nil {
					if ctx.current.Filters[i].MatchString(s.DiffPGresult.Result[rownum][i].String) {
						doPrint = true
						break
					} else {
						doPrint = false
					}
				}
			}
		}

		// print values
		for _, colname := range s.DiffPGresult.Cols {
			if doPrint {
				/* m[row][column] */
				fmt.Fprintf(v, "%-*s ", s.DiffPGresult.Colmaxlen[colname], s.DiffPGresult.Result[rownum][colnum].String)
				colnum++
			}
		}
		if doPrint {
			fmt.Fprintf(v, "\n")
		}
	}
}

// Print iostat - block devices stats.
func printIostat(v *gocui.View, s stat.Diskstats) {
	// print header
	fmt.Fprintf(v, "       \033[37;1mDevice:     rrqm/s     wrqm/s        r/s        w/s      rMB/s      wMB/s   avgrq-sz   avgqu-sz      await    r_await    w_await      %%util\033[0m\n")

	for i := 0; i < len(s); i++ {
		// skip devices which never do IOs
		if s[i].Completed == 0 {
			continue
		}

		// print stats
		fmt.Fprintf(v, "%14s\t%10.2f %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f\n",
			s[i].Device,
			s[i].Rmerged, s[i].Wmerged,
			s[i].Rcompleted, s[i].Wcompleted,
			s[i].Rsectors, s[i].Wsectors, s[i].Arqsz, s[i].Tweighted,
			s[i].Await, s[i].Rawait, s[i].Wawait,
			s[i].Util)
	}
}

// Print nicstat - network interfaces stat.
func printNicstat(v *gocui.View, s stat.Netdevs) {
	// print header
	fmt.Fprintf(v, "    \033[37;1mInterface:   rMbps   wMbps    rPk/s    wPk/s     rAvs     wAvs     IErr     OErr     Coll      Sat   %%rUtil   %%wUtil    %%Util\033[0m\n")

	for i := 0; i < len(s); i++ {
		// skip interfaces which never seen packets
		if s[i].Packets == 0 {
			continue
		}

		// print stats
		fmt.Fprintf(v, "%14s%8.2f%8.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f\n",
			s[i].Ifname,
			s[i].Rbytes/1024/128, s[i].Tbytes/1024/128, // conversion to Mbps
			s[i].Rpackets, s[i].Tpackets, s[i].Raverage, s[i].Taverage,
			s[i].Rerrs, s[i].Terrs, s[i].Tcolls,
			s[i].Saturation, s[i].Rutil, s[i].Tutil, s[i].Utilization)
	}
}

// Print logtail - last lines of Postgres log
func printLogtail(g *gocui.Gui, v *gocui.View) error {
	// pgCenter builds multiline log-records into a single one and truncates resulting line to screen's length. But
	// it's possible to print them completely with v.Wrap = true. But with v.Wrap and v.Autoscroll, it's possible to
	// solve all issues - just read a quite big amount of logs, and limit this amount by size of the view - all
	// unneded log records will be outside of the screen, thus user will see real tail of the logfile. But this approach
	// can't to be used, because the log-file path has to printed in the begining, before log records.
	// Logfile path can be moved to the view's title, but in this case the view frame will be drawn.

	x, y := v.Size()
	pgLog.LinesLimit = y - 1  // size of tail in lines
	pgLog.Bufsize = x * y * 2 // max size of used buffer -- don't need to read log more than that amount

	info, err := os.Stat(pgLog.Path)
	if err != nil {
		printCmdline(g, "Failed to stat logfile: %s", err)
		return nil
	}

	// Update screen only if logfile is changed
	if info.Size() > pgLog.Size {
		// clear view's content and read the log
		v.Clear()
		buf, err := pgLog.Read()
		if err != nil {
			printCmdline(g, "Failed to read logfile: %s", err)
			return nil
		}

		// print the log's path and file name and log's latest lines
		if len(string(buf)) > 0 {
			fmt.Fprintf(v, "\033[37;1m%s:\033[0m\n", pgLog.Path)
			fmt.Fprintf(v, "%s", string(buf))
		}
		// remember log's size
		pgLog.Size = info.Size()
	} else if info.Size() < pgLog.Size {
		// size is less than it was - perhaps logfile is truncated and rotated
		v.Clear()
		err := pgLog.ReOpen()
		if err != nil {
			printCmdline(g, "Failed to reopen logfile: %s", err)
			return nil
		}
		pgLog.Size = info.Size()
	}

	return nil
}
