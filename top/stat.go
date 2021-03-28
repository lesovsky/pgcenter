package top

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgconn"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/align"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/pretty"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
	"os"
	"regexp"
	"strconv"
	"time"
)

// collectStat
func collectStat(ctx context.Context, db *postgres.DB, statCh chan<- stat.Stat, viewCh <-chan view.View) {
	c, err := stat.NewCollector(db)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Get current view.
	v := <-viewCh

	// Enable collecting of extra stats if it's specified in the view.
	c.ToggleCollectExtra(v.ShowExtra)

	// Set refresh interval from received view.
	refresh := v.Refresh

	// Run first update to prefill "previous" snapshot.
	_, err = c.Update(db, v, refresh)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Wait a bit, to allow Postgres counters increments. Also we don't want to wait for
	// the whole refresh interval - it looks like program freezes at start.
	time.Sleep(100 * time.Millisecond)

	// Set settings related to extra stats.
	extra := v.ShowExtra

	// Collect stat in loop and send it to stat channel.
	for {
		// Collect stats.
		stats, err := c.Update(db, v, refresh)
		if err != nil {
			stats.Error = err
		}

		// Sending collected stats. Also checking for context cancel, it could be received due to failed UI.
		select {
		case statCh <- stats:
			// ok, stats received.
		case <-ctx.Done():
			// quit received, close channel and return.
			close(statCh)
			return
		}

		// Waiting for receiving new view until refresh interval expired. When new view has been received, use its
		// settings to adjust collector's behavior.
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

			// When view has been updated, stop the ticker and re-initialize stats.
			ticker.Stop()

			c.Reset()
			_, err = c.Update(db, v, refresh)
			if err != nil {
				statCh <- stat.Stat{Error: err}
			}

			continue
		case <-ctx.Done():
			ticker.Stop()
			close(statCh)
			return
		case <-ticker.C:
			continue
		}
	}
}

// printStat prints collected stats in UI.
func printStat(app *app, s stat.Stat, props stat.PostgresProperties) {
	app.ui.Update(func(g *gocui.Gui) error {
		v, err := g.View("sysstat")
		if err != nil {
			return fmt.Errorf("set focus on sysstat view failed: %s", err)
		}
		v.Clear()
		err = printSysstat(v, s)
		if err != nil {
			return fmt.Errorf("print sysstat failed: %s", err)
		}

		v, err = g.View("pgstat")
		if err != nil {
			return fmt.Errorf("set focus on pgstat view failed: %s", err)
		}
		v.Clear()
		err = printPgstat(v, s, props, app.db)
		if err != nil {
			return fmt.Errorf("print summary postgres stat failed: %s", err)
		}

		v, err = g.View("dbstat")
		if err != nil {
			return fmt.Errorf("set focus on dbstat view failed: %s", err)
		}
		v.Clear()

		err = printDbstat(v, app.config, s)
		if err != nil {
			return fmt.Errorf("print main postgres stat failed: %s", err)
		}

		if app.config.view.ShowExtra > stat.CollectNone {
			v, err := g.View("extra")
			if err != nil {
				return fmt.Errorf("set focus on extra view failed: %s", err)
			}

			switch app.config.view.ShowExtra {
			case stat.CollectDiskstats:
				v.Clear()
				err := printIostat(v, s.Diskstats)
				if err != nil {
					return err
				}
			case stat.CollectNetdev:
				v.Clear()
				err := printNetdev(v, s.Netdevs)
				if err != nil {
					return err
				}
			case stat.CollectFsstats:
				v.Clear()
				err := printFsstats(v, s.Fsstats)
				if err != nil {
					return err
				}
			case stat.CollectLogtail:
				size, buf, err := readLogfileRecent(v, app.config.logtail)
				if err != nil {
					printCmdline(g, "Tail Postgres log failed: %s", err)
					return err
				}

				if size < app.config.logtail.Size {
					v.Clear()
					err := app.config.logtail.Reopen(app.db, app.postgresProps.VersionNum)
					if err != nil {
						printCmdline(g, "Tail Postgres log failed: %s", err)
						return err
					}
				}

				// Update info about logfile size.
				app.config.logtail.Size = size

				err = printLogtail(v, app.config.logtail.Path, buf)
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// printSysstat prints system stats on UI.
func printSysstat(v *gocui.View, s stat.Stat) error {
	var err error

	/* line1: current time and load average */
	_, err = fmt.Fprintf(v, "pgcenter: %s, load average: %.2f, %.2f, %.2f\n",
		time.Now().Format("2006-01-02 15:04:05"),
		s.LoadAvg.One, s.LoadAvg.Five, s.LoadAvg.Fifteen)
	if err != nil {
		return err
	}

	/* line2: cpu usage */
	_, err = fmt.Fprintf(v, "    %%cpu: \033[37;1m%4.1f\033[0m us, \033[37;1m%4.1f\033[0m sy, \033[37;1m%4.1f\033[0m ni, \033[37;1m%4.1f\033[0m id, \033[37;1m%4.1f\033[0m wa, \033[37;1m%4.1f\033[0m hi, \033[37;1m%4.1f\033[0m si, \033[37;1m%4.1f\033[0m st\n",
		s.CpuStat.User, s.CpuStat.Sys, s.CpuStat.Nice, s.CpuStat.Idle,
		s.CpuStat.Iowait, s.CpuStat.Irq, s.CpuStat.Softirq, s.CpuStat.Steal)
	if err != nil {
		return err
	}

	/* line3: memory usage */
	_, err = fmt.Fprintf(v, " MiB mem: \033[37;1m%6d\033[0m total, \033[37;1m%6d\033[0m free, \033[37;1m%6d\033[0m used, \033[37;1m%8d\033[0m buff/cached\n",
		s.Meminfo.MemTotal, s.Meminfo.MemFree, s.Meminfo.MemUsed,
		s.Meminfo.MemCached+s.Meminfo.MemBuffers+s.Meminfo.MemSlab)
	if err != nil {
		return err
	}

	/* line4: swap usage, dirty and writeback */
	_, err = fmt.Fprintf(v, "MiB swap: \033[37;1m%6d\033[0m total, \033[37;1m%6d\033[0m free, \033[37;1m%6d\033[0m used, \033[37;1m%6d/%d\033[0m dirty/writeback\n",
		s.Meminfo.SwapTotal, s.Meminfo.SwapFree, s.Meminfo.SwapUsed,
		s.Meminfo.MemDirty, s.Meminfo.MemWriteback)
	if err != nil {
		return err
	}

	return nil
}

// printPgstat prints summary Postgres stats on UI.
func printPgstat(v *gocui.View, s stat.Stat, props stat.PostgresProperties, db *postgres.DB) error {
	// line1: details of used connection, version, uptime and recovery status
	_, err := fmt.Fprintln(v, formatInfoString(db.Config, s.Activity.State, props.Version, s.Activity.Uptime, props.Recovery))
	if err != nil {
		return err
	}

	// line2: current state of connections: total, idle, idle xacts, active, waiting, others
	_, err = fmt.Fprintf(v, "  activity:\033[37;1m%3d/%d\033[0m conns,\033[37;1m%3d/%d\033[0m prepared,\033[37;1m%3d\033[0m idle,\033[37;1m%3d\033[0m idle_xact,\033[37;1m%3d\033[0m active,\033[37;1m%3d\033[0m waiting,\033[37;1m%3d\033[0m others\n",
		s.Activity.ConnTotal, props.GucMaxConnections, s.Activity.ConnPrepared, props.GucMaxPrepXacts,
		s.Activity.ConnIdle, s.Activity.ConnIdleXact, s.Activity.ConnActive,
		s.Activity.ConnWaiting, s.Activity.ConnOthers)
	if err != nil {
		return err
	}

	// line3: current state of autovacuum: number of workers, anti-wraparound, manual vacuums and time of oldest vacuum
	_, err = fmt.Fprintf(v, "autovacuum: \033[37;1m%2d/%d\033[0m workers/max, \033[37;1m%2d\033[0m manual, \033[37;1m%2d\033[0m wraparound, \033[37;1m%s\033[0m vac_maxtime\n",
		s.Activity.AVWorkers, props.GucAVMaxWorkers,
		s.Activity.AVUser, s.Activity.AVAntiwrap, s.Activity.AVMaxTime)
	if err != nil {
		return err
	}

	// line4: current workload
	_, err = fmt.Fprintf(v, "statements: \033[37;1m%3d\033[0m stmt/s, \033[37;1m%3.3f\033[0m stmt_avgtime, \033[37;1m%s\033[0m xact_maxtime, \033[37;1m%s\033[0m prep_maxtime\n",
		s.Activity.CallsRate, s.Activity.StmtAvgTime, s.Activity.XactMaxTime, s.Activity.PrepMaxTime)
	if err != nil {
		return err
	}

	return nil
}

// formatInfoString combines connection's and general Postgres properties and provides info string.
func formatInfoString(cfg postgres.Config, state, version, uptime, recovery string) string {
	props := []string{cfg.Config.Host, strconv.Itoa(int(cfg.Config.Port)), cfg.Config.User, cfg.Config.Database, version}
	for i, v := range props {
		if len(props[i]) >= 20 {
			props[i] = v[0:15] + "~"
		}
	}

	// If database is empty, use database name as a user name.
	if props[3] == "" {
		props[3] = props[2]
	}

	return fmt.Sprintf(
		"state [%s]: %s:%s %s@%s (ver: %s, up %s, recovery: %.1s)",
		state, props[0], props[1], props[2], props[3], props[4], uptime, recovery,
	)
}

// printDbstat prints main Postgres stats on UI.
func printDbstat(v *gocui.View, config *config, s stat.Stat) error {
	// If reading stats failed, print the error occurred and return.
	if s.Error != nil {
		_, err := fmt.Fprint(v, formatError(s.Error))
		if err != nil {
			return err
		}
		s.Error = nil
		return nil
	}

	// Align values within columns, use fixed aligning instead of dynamic.
	if !config.view.Aligned {
		widthes, cols := align.SetAlign(s.Result, 1000, false) // use high limit (1000) to avoid truncating last value.
		config.view.Cols = cols
		config.view.ColsWidth = widthes
		config.view.Aligned = true
	}

	// Print header.
	err := printStatHeader(v, s, config)
	if err != nil {
		return err
	}

	// Print data.
	err = printStatData(v, s, config, isFilterRequired(config.view.Filters))
	if err != nil {
		return err
	}

	return nil
}

// formatError returns formatted error string depending on its type.
func formatError(err error) string {
	if err == nil {
		return ""
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return fmt.Sprintf("%s: %s\nDETAIL: %s\nHINT: %s", pgErr.Severity, pgErr.Message, pgErr.Detail, pgErr.Hint)
	}

	return fmt.Sprintf("ERROR: %s", err.Error())
}

// printStatHeader prints stats header.
func printStatHeader(v *gocui.View, s stat.Stat, config *config) error {
	var pname string
	for i := 0; i < s.Result.Ncols; i++ {
		name := s.Result.Cols[i]

		// mark filtered column
		if config.view.Filters[i] != nil && config.view.Filters[i].String() != "" {
			pname = "*" + name
		} else {
			pname = name
		}

		// mark ordered column with foreground color
		if i != config.view.OrderKey {
			_, err := fmt.Fprintf(v, "\033[%d;%dm%-*s\033[0m", 30, 47, config.view.ColsWidth[i]+2, pname)
			if err != nil {
				return err
			}
		} else {
			_, err := fmt.Fprintf(v, "\033[%d;%dm%-*s\033[0m", 47, 1, config.view.ColsWidth[i]+2, pname)
			if err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintf(v, "\n")
	if err != nil {
		return err
	}

	return nil
}

// printStatData prints stats data.
func printStatData(v *gocui.View, s stat.Stat, config *config, filter bool) error {
	var doPrint bool
	for colnum, rownum := 0, 0; rownum < s.Result.Nrows; rownum, colnum = rownum+1, 0 {
		// be optimistic, we want to print the row.
		doPrint = true

		// apply filters using regexp
		if filter {
			for i := 0; i < s.Result.Ncols; i++ {
				if config.view.Filters[i] != nil {
					if config.view.Filters[i].MatchString(s.Result.Values[rownum][i].String) {
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
				if valuelen > config.view.ColsWidth[i] {
					width := config.view.ColsWidth[i]
					if width <= 0 {
						return fmt.Errorf("zero or negative width, skip")
					}

					// truncate value up to column width and replace last character with '~' symbol
					s.Result.Values[rownum][colnum].String = s.Result.Values[rownum][colnum].String[:width-1] + "~"
				}

				// print value
				_, err := fmt.Fprintf(v, "%-*s", config.view.ColsWidth[i]+2, s.Result.Values[rownum][colnum].String)
				if err != nil {
					return err
				}
				colnum++
			}
		}
		if doPrint {
			_, err := fmt.Fprintf(v, "\n")
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// printIostat prints extra 'iostat' - block IO devices stats.
func printIostat(v *gocui.View, s stat.Diskstats) error {
	// print header
	_, err := fmt.Fprintf(v, "\033[30;47m             Device:     rrqm/s     wrqm/s        r/s        w/s      rMB/s      wMB/s   avgrq-sz   avgqu-sz      await    r_await    w_await      %%util\033[0m\n")
	if err != nil {
		return err
	}

	for i := 0; i < len(s); i++ {
		// skip devices which never do IOs
		if s[i].Completed == 0 {
			continue
		}

		// print stats
		_, err := fmt.Fprintf(v, "%20s\t%10.2f %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f %10.2f\n",
			s[i].Device,
			s[i].Rmerged, s[i].Wmerged, s[i].Rcompleted, s[i].Wcompleted,
			s[i].Rsectors, s[i].Wsectors, s[i].Arqsz, s[i].Tweighted,
			s[i].Await, s[i].Rawait, s[i].Wawait, s[i].Util,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// printNetdev prints 'nicstat' stats - network interfaces stats.
func printNetdev(v *gocui.View, s stat.Netdevs) error {
	// print header
	_, err := fmt.Fprintf(v, "\033[30;47m          Interface:   rMbps   wMbps    rPk/s    wPk/s     rAvs     wAvs     IErr     OErr     Coll      Sat   %%rUtil   %%wUtil    %%Util\033[0m\n")
	if err != nil {
		return err
	}

	for i := 0; i < len(s); i++ {
		// skip interfaces which never seen packets
		if s[i].Packets == 0 {
			continue
		}

		// print stats
		_, err := fmt.Fprintf(v, "%20s%8.2f%8.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f%9.2f\n",
			s[i].Ifname,
			s[i].Rbytes/1024/128, s[i].Tbytes/1024/128, // conversion to Mbps
			s[i].Rpackets, s[i].Tpackets, s[i].Raverage, s[i].Taverage,
			s[i].Rerrs, s[i].Terrs, s[i].Tcolls,
			s[i].Saturation, s[i].Rutil, s[i].Tutil, s[i].Utilization,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// printFsstats prints stats similar to 'df -h', 'df -i' - mounted filesystems stats.
func printFsstats(v *gocui.View, s stat.Fsstats) error {
	// print header
	_, err := fmt.Fprintf(v, "\033[30;47m             Filesystem:       size       used      avail   reserved     use%%      inodes       iused       ifree    iuse%%   fstype  mounted on\033[0m\n")
	if err != nil {
		return err
	}

	for i := 0; i < len(s); i++ {
		// print stats
		_, err := fmt.Fprintf(v, "%24s%11s%11s%11s%11s%8.0f%%%12.0f%12.0f%12.0f%8.0f%%%9s  %-24s\n",
			s[i].Mount.Device,
			pretty.Size(s[i].Size), pretty.Size(s[i].Used), pretty.Size(s[i].Avail), pretty.Size(s[i].Reserved), s[i].Pused,
			s[i].Files, s[i].Filesused, s[i].Filesfree, s[i].Filespused,
			s[i].Mount.Fstype, s[i].Mount.Mountpoint,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// readLogfileRecent reads necessary number of recent lines in logfile and return them.
func readLogfileRecent(v *gocui.View, logfile stat.Logfile) (int64, []byte, error) {
	// Calculate necessary number of lines and buffer size depending on size available screen.
	x, y := v.Size()
	linesLimit := y - 1  // available number of lines
	bufsize := x * y * 2 // max size of used buffer - don't need to read log more than that amount

	info, err := os.Stat(logfile.Path)
	if err != nil {
		return 0, nil, err
	}

	// Do nothing if logfile is not changed or empty.
	if info.Size() == logfile.Size || info.Size() == 0 {
		return info.Size(), nil, nil
	}

	// Read the log for necessary number of lines or until bufsize reached.
	buf, err := logfile.Read(linesLimit, bufsize)
	if err != nil {
		return 0, nil, err
	}

	// return the log's size and buffer content
	return info.Size(), buf, nil
}

// printLogtail prints 'logtail' - last lines of Postgres log.
func printLogtail(v *gocui.View, path string, buf []byte) error {
	if len(string(buf)) > 0 {
		// clear view's content and read the log
		v.Clear()

		_, err := fmt.Fprintf(v, "\033[30;47m%s:\033[0m\n", path)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(v, "%s", string(buf))
		if err != nil {
			return err
		}
	}

	return nil
}

// isFilterRequired returns true if at least one filter regexp is specified.
func isFilterRequired(f map[int]*regexp.Regexp) bool {
	for _, v := range f {
		if v != nil {
			return true
		}
	}
	return false
}
