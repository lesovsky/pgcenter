// Stuff related to gathering, processing and displaying stats.

package top

import (
	"context"
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
	"github.com/lib/pq"
	"os"
	"regexp"
	"time"
)

func collectStat(ctx context.Context, db *postgres.DB, statCh chan<- stat.Stat, viewCh <-chan view.View) {
	c, err := stat.NewCollector(db)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Get current view.
	v := <-viewCh

	// Run first update to prefill "previous" snapshot.
	_, err = c.Update(db, v)
	if err != nil {
		fmt.Println(err)
	}
	time.Sleep(100 * time.Millisecond)

	// Set refresh interval from received view.
	refresh := v.Refresh

	// Set settings related to extra stats.
	extra := v.ShowExtra

	// Collect stat in loop and send it to stat channel.
	for {
		stats, err := c.Update(db, v)
		if err != nil {
			fmt.Println(err)
			continue
		} else {
			statCh <- stats
		}

		// Waiting for events until refresh interval expired.
		ticker := time.NewTicker(refresh)
		select {
		case v = <-viewCh:
			// Update refresh interval if it is changed.
			if refresh != v.Refresh && v.Refresh > 0 {
				refresh = v.Refresh
				continue
			}

			// Update settings related to collecting extra stats (enable, disable or switch)
			if extra != v.ShowExtra {
				extra = v.ShowExtra
				c.ToggleCollectExtra(extra)
				continue
			}

			// If view has been updated, stop ticker and re-initialize stats.
			ticker.Stop()

			c.Reset()
			_, err = c.Update(db, v)
			if err != nil {
				fmt.Println(err)
			}

			continue
		case <-ctx.Done():
			ticker.Stop()
			return
		case <-ticker.C:
			continue
		}
	}
}

func printStat(app *app, s stat.Stat, props stat.PostgresProperties) {
	app.ui.Update(func(g *gocui.Gui) error {
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
		printPgstat(v, s, props)

		v, err = g.View("dbstat")
		if err != nil {
			return fmt.Errorf("Set focus on dbstat view failed: %s", err)
		}
		v.Clear()
		printDbstat(v, app, s)

		if app.config.view.ShowExtra > stat.CollectNone {
			v, err := g.View("aux")
			if err != nil {
				return fmt.Errorf("Set focus on aux view failed: %s", err)
			}

			switch app.config.view.ShowExtra {
			case stat.CollectDiskstats:
				v.Clear()
				printIostat(v, s.Diskstats)
				//case auxNicstat:
				//  v.Clear()
				//  printNicstat(v, app.stats.DiffNetdevs)
				//case auxLogtail:
				//  // don't clear screen
				//  printLogtail(g, v)
			}
		}
		return nil
	})
}

func printSysstat(v *gocui.View, s stat.Stat) {
	/* line1: current time and load average */
	fmt.Fprintf(v, "pgcenter: %s, load average: %.2f, %.2f, %.2f\n",
		time.Now().Format("2006-01-02 15:04:05"),
		s.LoadAvg.One, s.LoadAvg.Five, s.LoadAvg.Fifteen)
	/* line2: cpu usage */
	fmt.Fprintf(v, "    %%cpu: \033[37;1m%4.1f\033[0m us, \033[37;1m%4.1f\033[0m sy, \033[37;1m%4.1f\033[0m ni, \033[37;1m%4.1f\033[0m id, \033[37;1m%4.1f\033[0m wa, \033[37;1m%4.1f\033[0m hi, \033[37;1m%4.1f\033[0m si, \033[37;1m%4.1f\033[0m st\n",
		s.CpuStat.User, s.CpuStat.Sys, s.CpuStat.Nice, s.CpuStat.Idle,
		s.CpuStat.Iowait, s.CpuStat.Irq, s.CpuStat.Softirq, s.CpuStat.Steal)
	/* line3: memory usage */
	fmt.Fprintf(v, " MiB mem: \033[37;1m%6d\033[0m total, \033[37;1m%6d\033[0m free, \033[37;1m%6d\033[0m used, \033[37;1m%8d\033[0m buff/cached\n",
		s.Meminfo.MemTotal, s.Meminfo.MemFree, s.Meminfo.MemUsed,
		s.Meminfo.MemCached+s.Meminfo.MemBuffers+s.Meminfo.MemSlab)
	/* line4: swap usage, dirty and writeback */
	fmt.Fprintf(v, "MiB swap: \033[37;1m%6d\033[0m total, \033[37;1m%6d\033[0m free, \033[37;1m%6d\033[0m used, \033[37;1m%6d/%d\033[0m dirty/writeback\n",
		s.Meminfo.SwapTotal, s.Meminfo.SwapFree, s.Meminfo.SwapUsed,
		s.Meminfo.MemDirty, s.Meminfo.MemWriteback)
}

func printPgstat(v *gocui.View, s stat.Stat, props stat.PostgresProperties) {
	/* line1: details of used connection, version, uptime and recovery status */
	fmt.Fprintf(v, "state [%s]: %.16s:%d %.16s@%.16s (ver: %s, up %s, recovery: %.1s)\n",
		props.State,
		//conninfo.Host, conninfo.Port, conninfo.User, conninfo.Dbname,
		"dummy", 0, "dummy", "dummy",
		props.Version, s.Activity.Uptime, props.Recovery)
	/* line2: current state of connections: total, idle, idle xacts, active, waiting, others */
	fmt.Fprintf(v, "  activity:\033[37;1m%3d/%d\033[0m conns,\033[37;1m%3d/%d\033[0m prepared,\033[37;1m%3d\033[0m idle,\033[37;1m%3d\033[0m idle_xact,\033[37;1m%3d\033[0m active,\033[37;1m%3d\033[0m waiting,\033[37;1m%3d\033[0m others\n",
		s.Activity.ConnTotal, props.GucMaxConnections, s.Activity.ConnPrepared, props.GucMaxPrepXacts,
		s.Activity.ConnIdle, s.Activity.ConnIdleXact, s.Activity.ConnActive,
		s.Activity.ConnWaiting, s.Activity.ConnOthers)
	/* line3: current state of autovacuum: number of workers, antiwraparound, manual vacuums and time of oldest vacuum */
	fmt.Fprintf(v, "autovacuum: \033[37;1m%2d/%d\033[0m workers/max, \033[37;1m%2d\033[0m manual, \033[37;1m%2d\033[0m wraparound, \033[37;1m%s\033[0m vac_maxtime\n",
		s.Activity.AVWorkers, props.GucAVMaxWorkers,
		s.Activity.AVUser, s.Activity.AVAntiwrap, s.Activity.AVMaxTime)
	/* line4: current workload*/
	fmt.Fprintf(v, "statements: \033[37;1m%3d\033[0m stmt/s, \033[37;1m%3.3f\033[0m stmt_avgtime, \033[37;1m%s\033[0m xact_maxtime, \033[37;1m%s\033[0m prep_maxtime\n",
		s.Activity.CallsRate, s.Activity.StmtAvgTime, s.Activity.XactMaxTime, s.Activity.PrepMaxTime)
}

func printDbstat(v *gocui.View, app *app, s stat.Stat) {
	// If query fails, show the corresponding error and return.
	if err, ok := s.Result.Err.(*pq.Error); ok {
		fmt.Fprintf(v, "%s: %s\nDETAIL: %s\nHINT: %s", err.Severity, err.Message, err.Detail, err.Hint)
		s.Result.Err = nil
		return
	}

	// configure aligning, use fixed aligning instead of dynamic
	if !app.config.view.Aligned {
		widthes, cols, err := s.Result.SetAlign(1000, false) // we don't want truncate lines here, so just use high limit
		if err == nil {
			app.config.view.Cols = cols
			app.config.view.ColsWidth = widthes
			app.config.view.Aligned = true
		}
	}

	// is filter required?
	var filter = isFilterRequired(app.config.view.Filters)

	/* print header - filtered column mark with star; ordered column make shadowed */
	printStatHeader(v, s, app)

	// print data
	printStatData(v, s, app, filter)
}

// Returns true if filtering is required
func isFilterRequired(f map[int]*regexp.Regexp) bool {
	for _, v := range f {
		if v != nil {
			return true
		}
	}
	return false
}

func printStatHeader(v *gocui.View, s stat.Stat, app *app) {
	var pname string
	for i := 0; i < s.Result.Ncols; i++ {
		name := s.Result.Cols[i]

		// mark filtered column
		if app.config.view.Filters[i] != nil && app.config.view.Filters[i].String() != "" {
			pname = "*" + name
		} else {
			pname = name
		}

		/* mark ordered column with foreground color */
		if i != app.config.view.OrderKey {
			fmt.Fprintf(v, "\033[%d;%dm%-*s\033[0m", 30, 47, app.config.view.ColsWidth[i]+2, pname)
		} else {
			fmt.Fprintf(v, "\033[%d;%dm%-*s\033[0m", 47, 1, app.config.view.ColsWidth[i]+2, pname)
		}
	}
	fmt.Fprintf(v, "\n")
}

func printStatData(v *gocui.View, s stat.Stat, app *app, filter bool) {
	var doPrint bool
	for colnum, rownum := 0, 0; rownum < s.Result.Nrows; rownum, colnum = rownum+1, 0 {
		// be optimistic, we want to print the row.
		doPrint = true

		// apply filters using regexp
		if filter {
			for i := 0; i < s.Result.Ncols; i++ {
				if app.config.view.Filters[i] != nil {
					if app.config.view.Filters[i].MatchString(s.Result.Values[rownum][i].String) {
						doPrint = true
						break
					} else {
						doPrint = false
					}
				}
			}
		}

		// print values
		for i := range s.Result.Cols {
			if doPrint {
				// truncate values that longer than column width
				valuelen := len(s.Result.Values[rownum][colnum].String)
				if valuelen > app.config.view.ColsWidth[i] {
					width := app.config.view.ColsWidth[i]
					// truncate value up to column width and replace last character with '~' symbol
					s.Result.Values[rownum][colnum].String = s.Result.Values[rownum][colnum].String[:width-1] + "~"
				}

				// print value
				fmt.Fprintf(v, "%-*s", app.config.view.ColsWidth[i]+2, s.Result.Values[rownum][colnum].String)
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
	fmt.Fprintf(v, "\033[30;47m             Device:     rrqm/s     wrqm/s        r/s        w/s      rMB/s      wMB/s   avgrq-sz   avgqu-sz      await    r_await    w_await      %%util\033[0m\n")

	for i := 0; i < len(s); i++ {
		// skip devices which never do IOs
		if s[i].Completed == 0 {
			continue
		}

		// print stats
		fmt.Fprintf(v, "%20s\t%10.2f %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f\n",
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
	fmt.Fprintf(v, "\033[30;47m          Interface:   rMbps   wMbps    rPk/s    wPk/s     rAvs     wAvs     IErr     OErr     Coll      Sat   %%rUtil   %%wUtil    %%Util\033[0m\n")

	for i := 0; i < len(s); i++ {
		// skip interfaces which never seen packets
		if s[i].Packets == 0 {
			continue
		}

		// print stats
		fmt.Fprintf(v, "%20s%8.2f%8.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f\n",
			s[i].Ifname,
			s[i].Rbytes/1024/128, s[i].Tbytes/1024/128, // conversion to Mbps
			s[i].Rpackets, s[i].Tpackets, s[i].Raverage, s[i].Taverage,
			s[i].Rerrs, s[i].Terrs, s[i].Tcolls,
			s[i].Saturation, s[i].Rutil, s[i].Tutil, s[i].Utilization)
	}
}

// Print logtail - last lines of Postgres log
func printLogtail(g *gocui.Gui, v *gocui.View) {
	// pgCenter builds multiline log-records into a single one and truncates resulting line to screen's length. But
	// it's possible to print them completely with v.Wrap = true. But with v.Wrap and v.Autoscroll, it's possible to
	// solve all issues - just read a quite big amount of logs, and limit this amount by size of the view - all
	// unneeded log records will be outside of the screen, thus user will see real tail of the logfile. But this approach
	// can't to be used, because the log-file path has to printed in the beginning, before log records.
	// Logfile path can be moved to the view's title, but in this case the view frame will be drawn.

	x, y := v.Size()
	pgLog.LinesLimit = y - 1  // size of tail in lines
	pgLog.Bufsize = x * y * 2 // max size of used buffer -- don't need to read log more than that amount

	info, err := os.Stat(pgLog.Path)
	if err != nil {
		printCmdline(g, "Failed to stat logfile: %s", err)
		return
	}

	// Update screen only if logfile is changed
	if info.Size() > pgLog.Size {
		// clear view's content and read the log
		v.Clear()
		buf, err := pgLog.Read()
		if err != nil {
			printCmdline(g, "Failed to read logfile: %s", err)
			return
		}

		// print the log's path and file name and log's latest lines
		if len(string(buf)) > 0 {
			fmt.Fprintf(v, "\033[30;47m%s:\033[0m\n", pgLog.Path)
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
			return
		}
		pgLog.Size = info.Size()
	}

	return
}
