package top

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/align"
	"github.com/lesovsky/pgcenter/internal/math"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/pretty"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
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
	// Track CollectExtra separately — it is read directly from view.View in
	// Collector.Update() (not via ToggleCollectExtra), so changes must trigger
	// a Reset() to discard stale per-PID snapshots.
	prevCollectExtra := v.CollectExtra
	// Track Verbose separately too — toggling it only changes how the top panels are
	// rendered, not what is collected, so a verbose-only view update must NOT Reset()
	// the collector (which would blank the CPU/mem/load deltas for one frame).
	prevVerbose := v.Verbose

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
			// Branch order is load-bearing: refresh -> ShowExtra -> Verbose -> CollectExtra
			// -> unconditional Reset. The render-only early-outs (refresh, Verbose) MUST
			// precede both Reset() paths below, otherwise a toggle that changes nothing the
			// collector reads would still wipe the "previous" snapshot and blank the deltas.
			// Do not move the Verbose early-out below a Reset.

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

			// Detect a verbose-only toggle. Verbose changes only how the top panels are
			// rendered, not what is collected, so it must not fall through to either Reset()
			// path below — doing so would discard the "previous" snapshot and blank the
			// CPU/mem/load deltas for one frame. Update the tracked value and skip the resets.
			if prevVerbose != v.Verbose {
				prevVerbose = v.Verbose
				continue
			}

			// Detect CollectExtra change. CollectExtra is read directly from
			// view.View by Collector.Update() (it does not flow through
			// ToggleCollectExtra), so a change here means the per-PID snapshot
			// maps belong to a different enrichment kind and must be cleared
			// before the next Update() to avoid stale-PID rate values.
			if prevCollectExtra != v.CollectExtra {
				c.Reset()
				prevCollectExtra = v.CollectExtra
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

// firstTickHint decides the cmdline first-tick hint from the collected Stat. While the collector's
// first-tick flag is set (propagated via Stat.System.VerboseFirstTick, the single source of truth — no
// duplicate first-tick flag in top/), the dear/delta-based verbose rows render n/a, so the cmdline
// shows "collecting...". The flag clears after the first successful refresh and re-arms on every verbose
// OFF->ON re-enable (Task 7), so the hint reappears on re-enable too — not only after a screen switch.
func firstTickHint(s stat.Stat) (string, bool) {
	if s.System.VerboseFirstTick {
		return "collecting...", true
	}
	return "", false
}

// printStat prints collected stats in UI.
func printStat(app *app, s stat.Stat, props stat.PostgresProperties) {
	// First-tick cmdline hint, keyed on the collector's first-tick flag (via Stat). printCmdline runs
	// its own g.Update, so it is called here (outside the panel-render g.Update below) exactly once per
	// path — only when the hint is shown — respecting the printCmdline mutual-exclusion (one call per
	// path, no overwrite). The cmdline is event-driven (it is NOT rewritten on every refresh; it stays
	// empty until something prints to it), so there is intentionally no explicit "clear" here: the hint
	// self-clears via printCmdline's own 2s timer, and once the first-tick flag clears on the next refresh
	// it is simply not re-emitted. On an OFF->ON re-enable the flag re-arms, so the hint reappears.
	if msg, show := firstTickHint(s); show {
		printCmdline(app.ui, "%s", msg)
	}

	app.ui.Update(func(g *gocui.Gui) error {
		v, err := g.View("sysstat")
		if err != nil {
			return fmt.Errorf("set focus on sysstat view failed: %w", err)
		}
		v.Clear()
		err = printSysstat(v, s, app.config.verbose, app.db.Local, props.DataDirectory)
		if err != nil {
			return fmt.Errorf("print sysstat failed: %w", err)
		}

		v, err = g.View("pgstat")
		if err != nil {
			return fmt.Errorf("set focus on pgstat view failed: %w", err)
		}
		v.Clear()
		err = printPgstat(v, s, props, app.db, app.config.verbose)
		if err != nil {
			return fmt.Errorf("print summary postgres stat failed: %w", err)
		}

		v, err = g.View("dbstat")
		if err != nil {
			return fmt.Errorf("set focus on dbstat view failed: %w", err)
		}
		v.Clear()

		err = printDbstat(v, app.config, s)
		if err != nil {
			return fmt.Errorf("print main postgres stat failed: %w", err)
		}

		if app.config.view.ShowExtra > stat.CollectNone {
			v, err := g.View("extra")
			if err != nil {
				return fmt.Errorf("set focus on extra view failed: %w", err)
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

// printSysstat prints system stats on UI. It is a thin wrapper that delegates to the
// writer-based renderSysstat (*gocui.View implements io.Writer), so the render core can be
// unit-tested without a live terminal — mirroring the printDbstat → renderDbstat precedent.
func printSysstat(v *gocui.View, s stat.Stat, verbose bool, local bool, dataDir string) error {
	return renderSysstat(v, s, verbose, local, dataDir)
}

// renderSysstat is the writer-based core of printSysstat: it prints the system stats to w.
// When verbose is set it appends three extended rows (iostat/nicstat/filesyst) consistent with the
// full B/N/F side panels: the iostat/nicstat rows select the max-%util device reusing the struct
// math already computed by count*Usage (never recomputed), and filesyst shows the data_directory's
// filesystem. local/dataDir drive the filesyst mount-prefix match (data_directory symlinks are
// resolved only when local).
func renderSysstat(w io.Writer, s stat.Stat, verbose bool, local bool, dataDir string) error {
	var err error

	/* line1: current time and load average */
	_, err = fmt.Fprintf(w, "pgcenter: %s, load average: %.2f, %.2f, %.2f\n",
		time.Now().Format("2006-01-02 15:04:05"),
		s.LoadAvg.One, s.LoadAvg.Five, s.LoadAvg.Fifteen)
	if err != nil {
		return err
	}

	/* line2: cpu usage */
	_, err = fmt.Fprintf(w, "    %%cpu: \033[37;1m%4.1f\033[0m us, \033[37;1m%4.1f\033[0m sy, \033[37;1m%4.1f\033[0m ni, \033[37;1m%4.1f\033[0m id, \033[37;1m%4.1f\033[0m wa, \033[37;1m%4.1f\033[0m hi, \033[37;1m%4.1f\033[0m si, \033[37;1m%4.1f\033[0m st\n",
		s.CPUStat.User, s.CPUStat.Sys, s.CPUStat.Nice, s.CPUStat.Idle,
		s.CPUStat.Iowait, s.CPUStat.Irq, s.CPUStat.Softirq, s.CPUStat.Steal)
	if err != nil {
		return err
	}

	/* line3: memory usage */
	_, err = fmt.Fprintf(w, " MiB mem: \033[37;1m%6d\033[0m total, \033[37;1m%6d\033[0m free, \033[37;1m%6d\033[0m used, \033[37;1m%8d\033[0m buff/cached\n",
		s.Meminfo.MemTotal, s.Meminfo.MemFree, s.Meminfo.MemUsed,
		s.Meminfo.MemCached+s.Meminfo.MemBuffers+s.Meminfo.MemSlab)
	if err != nil {
		return err
	}

	/* line4: swap usage, dirty and writeback */
	_, err = fmt.Fprintf(w, "MiB swap: \033[37;1m%6d\033[0m total, \033[37;1m%6d\033[0m free, \033[37;1m%6d\033[0m used, \033[37;1m%6d/%d\033[0m dirty/writeback\n",
		s.Meminfo.SwapTotal, s.Meminfo.SwapFree, s.Meminfo.SwapUsed,
		s.Meminfo.MemDirty, s.Meminfo.MemWriteback)
	if err != nil {
		return err
	}

	if verbose {
		if err := renderSysstatVerbose(w, s, local, dataDir); err != nil {
			return err
		}
	}

	return nil
}

// naLiteral is the literal rendered for an unavailable signal — never "0" and never empty, so a
// DBA can tell a missing signal from a real zero (user-spec degradation requirement).
const naLiteral = "n/a"

// rateField formats a disk/net rate value (already in MB/s or Mbps) into a fixed-digit-reserve field
// followed by a r/w-prefixed unit, switching to the next higher unit when the rounded integer no
// longer fits the reserve. It is the dynamic-suffix companion to pretty.RateUnit, differing only in
// that the read/write prefix is placed between the digits and the unit (the user-spec layout shows
// "1135 rMB/s", "1546 wMB/s"). The value is rounded up (ceil) so verbose stays whole-number.
func rateField(v float64, family string, prefix string, width int) string {
	base, high := "MB/s", "GB/s"
	var divisor float64 = 1024
	if family == pretty.FamilyNet {
		base, high = "Mbps", "Gbps"
		divisor = 1000
	}

	max := 1
	for i := 0; i < width; i++ {
		max *= 10
	}
	max-- // largest integer that fits the reserve, e.g. width 4 -> 9999

	if pretty.Ceil(v) <= max {
		return pretty.ReserveWidth(pretty.Ceil(v), width) + " " + prefix + base
	}
	return pretty.ReserveWidth(pretty.Ceil(v/divisor), width) + " " + prefix + high
}

// renderSysstatVerbose appends the three verbose system rows to w. Each row degrades independently:
// no active device / first tick / no mount match renders n/a for that row without aborting the others.
func renderSysstatVerbose(w io.Writer, s stat.Stat, local bool, dataDir string) error {
	// iostat row: select the max-%util device among active ones (Completed != 0), the same device
	// set printIostat shows. The first verbose tick (s.VerboseFirstTick) has no valid prev, so the
	// delta fields render n/a rather than a misleading zero — NOT keyed on len(slice) (the slice is
	// populated zero-delta on the first tick).
	if idx := maxUtilDisk(s.Diskstats); idx < 0 || s.VerboseFirstTick {
		if _, err := fmt.Fprintf(w, "  iostat: %s devices, %s max util, %s, %s, %s, %s\n",
			pretty.ReserveWidth(activeDiskCount(s.Diskstats), 2), naLiteral, naLiteral, naLiteral, naLiteral, naLiteral); err != nil {
			return err
		}
	} else {
		d := s.Diskstats[idx]
		if _, err := fmt.Fprintf(w, "  iostat: %s devices, %s%% max util, %s, %s r/s, %s, %s w/s\n",
			pretty.ReserveWidth(activeDiskCount(s.Diskstats), 2),
			pretty.ReserveWidth(pretty.Ceil(d.Util), 3),
			rateField(d.Rsectors, pretty.FamilyDisk, "r", 4),
			pretty.ReserveWidth(pretty.Ceil(d.Rcompleted), 5),
			rateField(d.Wsectors, pretty.FamilyDisk, "w", 4),
			pretty.ReserveWidth(pretty.Ceil(d.Wcompleted), 5)); err != nil {
			return err
		}
	}

	// nicstat row: select the max-Utilization interface among active ones (Packets != 0), the same
	// set printNetdev shows. rMbps/wMbps replicate printNetdev's print-time Rbytes/1024/128 exactly.
	if idx := maxUtilNet(s.Netdevs); idx < 0 || s.VerboseFirstTick {
		if _, err := fmt.Fprintf(w, " nicstat: %s devices, %s max util, %s, %s, %s err/coll\n",
			pretty.ReserveWidth(activeNetCount(s.Netdevs), 2), naLiteral, naLiteral, naLiteral, naLiteral); err != nil {
			return err
		}
	} else {
		n := s.Netdevs[idx]
		if _, err := fmt.Fprintf(w, " nicstat: %s devices, %s%% max util, %s, %s, %s/%s err/coll\n",
			pretty.ReserveWidth(activeNetCount(s.Netdevs), 2),
			pretty.ReserveWidth(pretty.Ceil(n.Utilization), 3),
			rateField(n.Rbytes/1024/128, pretty.FamilyNet, "r", 4),
			rateField(n.Tbytes/1024/128, pretty.FamilyNet, "w", 4),
			pretty.ReserveWidth(pretty.Ceil(n.Rerrs+n.Terrs), 4),
			pretty.ReserveWidth(pretty.Ceil(n.Tcolls), 4)); err != nil {
			return err
		}
	}

	// filesyst row: the data_directory's filesystem by longest mount-prefix. Any match failure
	// (no mount, empty data_directory, EvalSymlinks failure) renders n/a.
	if fs, ok := stat.MatchDataDirFs(dataDir, s.Fsstats, local); ok {
		if _, err := fmt.Fprintf(w, "filesyst: %s on %s (%s), %s size, %s used, %s%% use\n",
			fs.Mount.Device, truncate(fs.Mount.Mountpoint, 10), fs.Mount.Fstype,
			pretty.Size(fs.Size), pretty.Size(fs.Used), pretty.ReserveWidth(pretty.Ceil(fs.Pused), 3)); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(w, "filesyst: %s\n", naLiteral); err != nil {
			return err
		}
	}

	return nil
}

// maxUtilDisk returns the index of the active disk (Completed != 0, the printIostat filter) with the
// highest Util, or -1 when no device is active. Util is read as-is from countDiskstatsUsage (never
// recomputed) so the verbose row matches the full B panel exactly (Decision 5).
func maxUtilDisk(s stat.Diskstats) int {
	best, bestUtil := -1, -1.0
	for i := range s {
		if s[i].Completed == 0 {
			continue
		}
		if s[i].Util > bestUtil {
			best, bestUtil = i, s[i].Util
		}
	}
	return best
}

// activeDiskCount counts active disks (Completed != 0), the device set printIostat displays.
func activeDiskCount(s stat.Diskstats) int {
	n := 0
	for i := range s {
		if s[i].Completed != 0 {
			n++
		}
	}
	return n
}

// maxUtilNet returns the index of the active interface (Packets != 0, the printNetdev filter) with
// the highest Utilization, or -1 when none is active. Utilization is read as-is from
// countNetdevsUsage (never recomputed) so the verbose row matches the full N panel exactly.
func maxUtilNet(s stat.Netdevs) int {
	best, bestUtil := -1, -1.0
	for i := range s {
		if s[i].Packets == 0 {
			continue
		}
		if s[i].Utilization > bestUtil {
			best, bestUtil = i, s[i].Utilization
		}
	}
	return best
}

// activeNetCount counts active interfaces (Packets != 0), the set printNetdev displays.
func activeNetCount(s stat.Netdevs) int {
	n := 0
	for i := range s {
		if s[i].Packets != 0 {
			n++
		}
	}
	return n
}

// truncate shortens s to at most n runes (the filesyst "mounted" field is capped at 10).
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// printPgstat prints summary Postgres stats on UI. It is a thin wrapper that delegates to the
// writer-based renderPgstat (*gocui.View implements io.Writer), so the render core can be
// unit-tested without a live terminal — mirroring the printDbstat → renderDbstat precedent.
func printPgstat(v *gocui.View, s stat.Stat, props stat.PostgresProperties, db *postgres.DB, verbose bool) error {
	return renderPgstat(v, s, props, db, verbose)
}

// renderPgstat is the writer-based core of printPgstat: it prints the summary Postgres stats to w.
// When verbose is set it appends five extended rows (workload/databases/workers/replication/bgwr-ckpt)
// from the PgstatOverview aggregate, consistent with the full d/r/b screens. Delta-based metrics with
// no prev snapshot (Overview.HasPrev == false) and unavailable sources (availability flags) render n/a.
func renderPgstat(w io.Writer, s stat.Stat, props stat.PostgresProperties, db *postgres.DB, verbose bool) error {
	// line1: details of used connection, version, uptime and recovery status
	_, err := fmt.Fprintln(w, formatInfoString(db.Config, s.Activity.State, props.Version, s.Activity.Uptime, props.Recovery))
	if err != nil {
		return err
	}

	// line2: current state of connections: total, idle, idle xacts, active, waiting, others
	_, err = fmt.Fprintf(w, "  activity:\033[37;1m%3d/%d\033[0m conns,\033[37;1m%3d/%d\033[0m prepared,\033[37;1m%3d\033[0m idle,\033[37;1m%3d\033[0m idle_xact,\033[37;1m%3d\033[0m active,\033[37;1m%3d\033[0m waiting,\033[37;1m%3d\033[0m others\n",
		s.Activity.ConnTotal, props.GucMaxConnections, s.Activity.ConnPrepared, props.GucMaxPrepXacts,
		s.Activity.ConnIdle, s.Activity.ConnIdleXact, s.Activity.ConnActive,
		s.Activity.ConnWaiting, s.Activity.ConnOthers)
	if err != nil {
		return err
	}

	// line3: current state of autovacuum: number of workers, anti-wraparound, manual vacuums and time of oldest vacuum
	_, err = fmt.Fprintf(w, "autovacuum: \033[37;1m%2d/%d\033[0m workers/max, \033[37;1m%2d\033[0m manual, \033[37;1m%2d\033[0m wraparound, \033[37;1m%s\033[0m vac_maxtime\n",
		s.Activity.AVWorkers, props.GucAVMaxWorkers,
		s.Activity.AVUser, s.Activity.AVAntiwrap, s.Activity.AVMaxTime)
	if err != nil {
		return err
	}

	// line4: current workload
	_, err = fmt.Fprintf(w, "statements: \033[37;1m%3d\033[0m stmt/s, \033[37;1m%3.3f\033[0m stmt_avgtime, \033[37;1m%s\033[0m xact_maxtime, \033[37;1m%s\033[0m prep_maxtime\n",
		s.Activity.CallsRate, s.Activity.StmtAvgTime, s.Activity.XactMaxTime, s.Activity.PrepMaxTime)
	if err != nil {
		return err
	}

	if verbose {
		if err := renderPgstatVerbose(w, s.Pgstat.Overview, props); err != nil {
			return err
		}
	}

	return nil
}

// naInt renders an int rate field as a fixed-width number, or n/a when this tick has no prev
// snapshot (hasPrev == false) — so a first-tick delta is distinguishable from a real zero.
func naInt(v int64, width int, hasPrev bool) string {
	if !hasPrev {
		return naLiteral
	}
	return pretty.ReserveWidth(int(v), width)
}

// renderPgstatVerbose appends the five verbose pgstat rows from the PgstatOverview aggregate. Each
// field degrades independently to n/a (first tick or unavailable source) without aborting the rest.
func renderPgstatVerbose(w io.Writer, o stat.PgstatOverview, props stat.PostgresProperties) error {
	hp := o.HasPrev

	// workload row. All fields are interval rates (no prev -> n/a); others is the interval value.
	if _, err := fmt.Fprintf(w, "    workload: %s tps, %s ins/s, %s upd/s, %s del/s, %s ret/s, %s tmp/s, %s others\n",
		naInt(o.TPSRate, 4, hp), naInt(o.InsertsRate, 4, hp), naInt(o.UpdatesRate, 4, hp),
		naInt(o.DeletesRate, 4, hp), naInt(o.ReturnedRate, 4, hp), naInt(o.TempFilesRate, 4, hp),
		naInt(o.OthersInterval, 3, hp)); err != nil {
		return err
	}

	// databases row. Size/growth are n/a when the privileged aggregate failed; cache hit ratio is
	// n/a on the first tick or when there was no I/O in the interval.
	size, growth := naLiteral, naLiteral
	if o.TotalSizeValid {
		size = pretty.Size(float64(o.TotalSize))
		if hp {
			growth = pretty.Size(float64(o.GrowthPerSec))
		}
	}
	hit := naLiteral
	if o.CacheHitRatioValid {
		hit = fmt.Sprintf("%.2f%%", o.CacheHitRatio)
	}
	if _, err := fmt.Fprintf(w, "   databases: %s per %s databases, %s growth/s, %s cache hit ratio\n",
		size, pretty.ReserveWidth(int(o.DatabasesCount), 2), growth, hit); err != nil {
		return err
	}

	// workers row. Active counts / GUC limits (umbrella max_worker_processes, logical, parallel).
	if _, err := fmt.Fprintf(w, "     workers: %s/%d workers/max, %s/%d logical workers, %s/%d parallel workers\n",
		pretty.ReserveWidth(o.WorkersUmbrellaActive, 2), props.GucMaxWorkerProcesses,
		pretty.ReserveWidth(o.WorkersLogicalActive, 2), props.GucMaxLogicalReplicationWorkers,
		pretty.ReserveWidth(o.WorkersParallelActive, 2), props.GucMaxParallelWorkers); err != nil {
		return err
	}

	// replication row. lag/slots-retain/archiving-backlog are n/a when their source is unavailable
	// (no standby, no slots, archive_mode=off / missing privilege).
	lag := naLiteral
	if o.LagBytesValid {
		lag = pretty.Size(float64(o.LagBytes))
	}
	retain := naLiteral
	if o.RetainedValid {
		retain = pretty.Size(float64(o.RetainedBytes))
	}
	backlog := naLiteral
	if o.ArchivingBacklogValid {
		backlog = pretty.Size(float64(o.ArchivingBacklog))
	}
	if _, err := fmt.Fprintf(w, " replication: %s wal size, %s lag, %s/%s slots/retain, %s archiving backlog, %d/%d send/recv\n",
		pretty.Size(float64(o.WalSize)), lag,
		pretty.ReserveWidth(int(o.SlotsCount), 2), retain,
		backlog, o.Senders, o.Receivers); err != nil {
		return err
	}

	// bgwr/ckpt row. timed/req are absolute cumulative; write/sync ms are interval deltas (n/a on
	// the first tick); maxwritten is the interval delta count.
	writeMs, syncMs, maxw := naLiteral, naLiteral, naLiteral
	if hp {
		writeMs = pretty.ReserveWidth(pretty.Ceil(o.CkptWriteMsDelta), 3)
		syncMs = pretty.ReserveWidth(pretty.Ceil(o.CkptSyncMsDelta), 3)
		maxw = pretty.ReserveWidth(int(o.MaxWrittenDelta), 2)
	}
	if _, err := fmt.Fprintf(w, "   bgwr/ckpt: %s/%s timed/req, %s/%s ms write/sync, %s maxwritten\n",
		pretty.ReserveWidth(int(o.CkptTimed), 2), pretty.ReserveWidth(int(o.CkptReq), 2),
		writeMs, syncMs, maxw); err != nil {
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

// alignViewToResult ensures config.view.ColsWidth is consistent with r.Ncols.
// It is called before every render. The Aligned flag alone is insufficient because
// after a view switch the first stat batch may still carry the OLD view's column
// count: SetAlign then populates ColsWidth for the wrong number of columns, and the
// next batch (with the correct column count) skips realignment and reads zero widths
// for missing keys — causing "slice bounds out of range [:-1]" (issue #99).
func alignViewToResult(config *config, r stat.PGresult) {
	if config.view.Aligned && len(config.view.ColsWidth) == r.Ncols {
		return
	}
	widthes, cols := align.SetAlign(r, 1000, false) // high limit avoids truncating the last value
	config.view.Cols = cols
	config.view.ColsWidth = widthes
	config.view.Aligned = true
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
	alignViewToResult(config, s.Result)

	// Terminal width drives the visible-column window. dbstat is created with
	// Frame=false, so Size() returns the true drawing width.
	termWidth, _ := v.Size()

	return renderDbstat(v, config, s, termWidth)
}

// renderDbstat is the writer-based core of printDbstat: it clamps the scroll offset,
// then prints the windowed header and data. It is separated from printDbstat (which only
// resolves the terminal width from the gocui view) so the render can be unit-tested
// without a live terminal.
func renderDbstat(w io.Writer, config *config, s stat.Stat, termWidth int) error {
	// Compute the visible window ONCE here and pass it to the header/data printers. This is
	// the single source of truth for the render: it avoids re-running visibleColumns three
	// times per frame (which risked the header and data disagreeing on the window) and it
	// guarantees both rows reserve the SAME space for the edge markers (alignment invariant).
	win := visibleColumns(s.Result.Ncols, config.view.ColsWidth, termWidth, config.scrollOffset)

	// Re-clamp the scroll offset on every render and write it back into config. config
	// is shared by pointer, so this persists across renders — the fix for runaway offset:
	// without write-back, repeated scroll-right at the visual maximum inflates the field
	// unboundedly (visibleColumns clamps its own result but the source field keeps growing),
	// after which scroll-left "sticks" until the offset drifts back into range.
	config.scrollOffset = win.clamped

	// Print header.
	err := printStatHeader(w, s, config, win)
	if err != nil {
		return err
	}

	// Print data.
	return printStatData(w, s, config, isFilterRequired(config.view.Filters), win)
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

// markerWidth is the printed (visible-rune) width reserved for one edge marker (‹ or ›).
// The markers are single runes, so each reserves exactly one cell. Both the header (which
// prints the marker rune) and the data rows (which print this many blank spaces in its
// place) use this constant, keeping the visible width of every row identical.
const markerWidth = 1

// columnWindow describes the visible window of scrollable columns for one render, plus the
// edge-marker reservation. It is computed once per frame (renderDbstat) and shared by the
// header and data printers so they never disagree on the window or on marker placement.
//
// first..last is the absolute index range of visible scrollable columns (last < first means
// an empty window — only the frozen column fits). clamped is the offset re-clamped into the
// valid range. hiddenLeft/hiddenRight report whether scrollable columns are hidden to the
// corresponding side; they double as "print this side's marker" flags. The header prints the
// marker rune; the data rows print markerWidth spaces on the same side, so both rows keep the
// same visible width and the columns stay aligned beneath their names.
type columnWindow struct {
	first, last, clamped    int
	hiddenLeft, hiddenRight bool
}

// visibleColumns computes the visible window of scrollable columns for horizontal
// scrolling. Column 0 is frozen (always part of the width budget); the remaining
// columns (1..ncols-1) form a sliding window selected by offset.
//
// It is the single source of truth for the visible range: it re-clamps offset into
// [0, maxOffset] on every call, where maxOffset is the smallest offset at which the
// last column (ncols-1) is still visible. This guards against a stale offset after an
// auto-refresh changed the column count.
//
// Edge markers (‹ / ›) are visible runes, so the space they occupy is subtracted from the
// scroll budget here — the single source of truth — and the same space is later emitted as
// blanks in the data rows. To avoid a circular dependency (a marker exists only if a side
// has hidden columns, but the window — and thus what is hidden — depends on the budget
// already deducted for that marker) the budget is reserved conservatively: the left marker
// is reserved whenever the clamped offset is > 0, and the right marker whenever a first
// pass over the full budget already shows columns hidden to the right (shrinking the budget
// can only keep them hidden, never reveal them, so the reservation is always justified).
//
// Widths are read strictly by index in [0, ncols); the map is never ranged over. A
// missing/zero key is treated as width 0 (still costing the +2 print gap), keeping the
// math bounded even with sparse widths (issue #99 class).
func visibleColumns(ncols int, colsWidth map[int]int, termWidth, offset int) columnWindow {
	// colWidth returns the printed cell budget for a column (value width + the +2 gap
	// that printing adds), reading the dense map strictly within [0, ncols).
	colWidth := func(i int) int {
		w := 0
		if i >= 0 && i < ncols {
			w = colsWidth[i]
		}
		if w < 0 {
			w = 0
		}
		return w + 2
	}

	// No scrollable columns: only the frozen column exists (or none at all).
	if ncols <= 1 {
		return columnWindow{first: 1, last: 0}
	}

	// Budget left for scrollable columns after reserving the frozen column 0.
	baseBudget := termWidth - colWidth(0)

	// countFit walks scrollable columns from index "from" toward "stop" (exclusive) in the
	// given step direction (+1 forward, -1 backward) and returns how many consecutive columns
	// have their START position inside the budget. A column counts as visible whenever the
	// width already consumed BEFORE it (its start) is still within budget — even if the column
	// itself overflows. The last counted column may therefore be only partially visible: it is
	// printed at full cell width and the terminal (gocui) truncates it at the screen edge.
	//
	// This mirrors the pre-scroll behaviour for the very wide trailing "query" column of the
	// activity/statements screens (aligned by content, almost never fitting in full): the
	// column stays visible truncated instead of disappearing, and no right marker is drawn when
	// the only thing past the edge is that column's own tail (issue #14 QA).
	countFit := func(from, stop, step, budget int) int {
		count, used := 0, 0
		for i := from; i != stop; i += step {
			if used >= budget {
				break
			}
			count++
			used += colWidth(i)
		}
		return count
	}

	// maxOffset is the smallest offset at which the last column (ncols-1) is still visible.
	// It is found by a backward walk: count how many trailing columns fit, the rest must be
	// scrolled past. The walk must reserve the SAME markers the forward-walk will reserve for
	// the window it produces at that offset, otherwise the two disagree and the last column
	// becomes unreachable (a › that never clears).
	//
	// At a non-zero max offset the left marker ‹ is always present (clamped > 0), while the
	// right marker is absent by definition (the last column is visible). So the trailing
	// columns must fit into baseBudget - markerWidth, not the full baseBudget. The all-fit case
	// (maxOffset == 0) has no left marker and uses the full budget.
	//
	// Reserving the left marker shrinks the budget, which can only push maxOffset up (never
	// down), so once a first full-budget pass shows scrolling is needed (maxOffset > 0) the
	// reservation is justified and re-running the walk against the reduced budget reaches the
	// fixpoint: the value can only grow and stays > 0, keeping the left marker present.
	maxOffset := 0
	if baseBudget > 0 {
		tailCount := countFit(ncols-1, 0, -1, baseBudget) // walk columns ncols-1..1 backwards
		maxOffset = math.Max((ncols-1)-tailCount, 0)
		if maxOffset > 0 {
			tailBudget := baseBudget - markerWidth // reserve the guaranteed left marker
			if tailBudget > 0 {
				tailCount = countFit(ncols-1, 0, -1, tailBudget)
				maxOffset = math.Max((ncols-1)-tailCount, 0)
			} else {
				maxOffset = ncols - 1 // no room for any trailing column beside the marker
			}
		}
	}
	clamped := math.Min(math.Max(offset, 0), maxOffset)

	hiddenLeft := clamped > 0
	probeFirst := 1 + clamped
	probeCount := countFit(probeFirst, ncols, +1, baseBudget)
	hiddenRight := probeFirst+probeCount-1 < ncols-1

	// Reserve marker space on each side that actually shows a marker, then recompute the
	// window inside the reduced budget so its visible width (data side) leaves room for the
	// marker(s) printed by the header.
	scrollBudget := baseBudget
	if hiddenLeft {
		scrollBudget -= markerWidth
	}
	if hiddenRight {
		scrollBudget -= markerWidth
	}

	first := probeFirst
	count := countFit(first, ncols, +1, scrollBudget) // walk columns first..ncols-1 forwards
	last := first + count - 1                         // last < first when no scrollable column fits

	// Recompute hiddenRight against the final window (reserving the right marker may have
	// pushed the last visible column off-screen; it cannot have revealed a new one).
	hiddenRight = last < ncols-1

	return columnWindow{first: first, last: last, clamped: clamped, hiddenLeft: hiddenLeft, hiddenRight: hiddenRight}
}

// printStatHeader prints the stats header for the visible column window: the frozen
// column 0 followed by the scrollable columns inside the window computed by
// visibleColumns. Edge markers ‹ / › are drawn when columns are hidden to the
// corresponding side; the markers are visible runes and are accounted for in the cell
// budget so the header stays aligned with the data rows.
func printStatHeader(w io.Writer, s stat.Stat, config *config, win columnWindow) error {
	// Frozen column 0 is always printed first, independent of offset.
	if err := printHeaderCell(w, s, config, 0); err != nil {
		return err
	}

	// Left edge marker: scrollable columns hidden to the left.
	if win.hiddenLeft {
		if _, err := fmt.Fprint(w, "‹"); err != nil {
			return err
		}
	}

	// Scrollable columns inside the visible window.
	for i := win.first; i <= win.last; i++ {
		if err := printHeaderCell(w, s, config, i); err != nil {
			return err
		}
	}

	// Right edge marker: scrollable columns hidden to the right.
	if win.hiddenRight {
		if _, err := fmt.Fprint(w, "›"); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(w, "\n"); err != nil {
		return err
	}

	return nil
}

// printHeaderCell prints a single header cell for column i, applying the filter prefix,
// the ordered-column highlight, and the frozen-column bold. The sort highlight has
// priority over frozen-bold on column 0 (Decision 4): when column 0 is the ordered
// column, only the sort highlight is applied.
func printHeaderCell(w io.Writer, s stat.Stat, config *config, i int) error {
	name := s.Result.Cols[i]

	// mark filtered column
	pname := name
	if config.view.Filters[i] != nil && config.view.Filters[i].String() != "" {
		pname = "*" + name
	}

	width := config.view.ColsWidth[i] + 2

	switch {
	case i == config.view.OrderKey:
		// ordered column highlight (also wins over frozen-bold on column 0, Decision 4)
		_, err := fmt.Fprintf(w, "\033[%d;%dm%-*s\033[0m", 47, 1, width, pname)
		return err
	case i == 0:
		// frozen column name in bold (when not the ordered column)
		_, err := fmt.Fprintf(w, "\033[%d;%d;%dm%-*s\033[0m", 30, 47, 1, width, pname)
		return err
	default:
		_, err := fmt.Fprintf(w, "\033[%d;%dm%-*s\033[0m", 30, 47, width, pname)
		return err
	}
}

// printStatData prints the stats data for the visible column window: the frozen column 0
// followed by the scrollable columns inside the window computed by visibleColumns. Values
// and widths are indexed strictly by the ABSOLUTE column index i (the previous independent
// colnum counter is removed) so windowed rendering keeps each value aligned with its
// column.
func printStatData(w io.Writer, s stat.Stat, config *config, filter bool, win columnWindow) error {
	// Blank fillers mirroring the header's edge markers: the header prints a marker rune on
	// each hidden side, so each data row prints markerWidth spaces in the same place. This is
	// the alignment invariant — the visible width of the header row equals that of every data
	// row, so scrollable columns line up under their names (review round 1, MAJOR #1).
	leftMarker := ""
	if win.hiddenLeft {
		leftMarker = strings.Repeat(" ", markerWidth)
	}
	rightMarker := ""
	if win.hiddenRight {
		rightMarker = strings.Repeat(" ", markerWidth)
	}

	var doPrint bool
	for rownum := 0; rownum < s.Result.Nrows; rownum++ {
		// be optimistic, we want to print the row.
		doPrint = true

		// apply filters using regexp
		if filter {
			for i := 0; i < s.Result.Ncols; i++ {
				if config.view.Filters[i] != nil {
					if config.view.Filters[i].MatchString(s.Result.Values[rownum][i].String) {
						doPrint = true
						break
					}
					doPrint = false
				}
			}
		}

		if !doPrint {
			continue
		}

		// print frozen column 0 value first, then the windowed columns.
		if err := printDataCell(w, s, config, rownum, 0); err != nil {
			return err
		}

		// Blank filler for the left edge marker, keeping data aligned with the header.
		if leftMarker != "" {
			if _, err := fmt.Fprint(w, leftMarker); err != nil {
				return err
			}
		}

		for i := win.first; i <= win.last; i++ {
			if err := printDataCell(w, s, config, rownum, i); err != nil {
				return err
			}
		}

		// Blank filler for the right edge marker.
		if rightMarker != "" {
			if _, err := fmt.Fprint(w, rightMarker); err != nil {
				return err
			}
		}

		if _, err := fmt.Fprintf(w, "\n"); err != nil {
			return err
		}
	}

	return nil
}

// printDataCell prints the value of column i for the given row, truncating values longer
// than the column width (replacing the last character with '~') and padding to the column
// width plus the +2 gap. Returns an error for a zero or negative column width.
func printDataCell(w io.Writer, s stat.Stat, config *config, rownum, i int) error {
	// truncate values that are longer than column width
	valuelen := len(s.Result.Values[rownum][i].String)
	if valuelen > config.view.ColsWidth[i] {
		width := config.view.ColsWidth[i]
		if width <= 0 {
			return fmt.Errorf("zero or negative width, skip")
		}

		// truncate value up to column width and replace last character with '~' symbol
		s.Result.Values[rownum][i].String = s.Result.Values[rownum][i].String[:width-1] + "~"
	}

	// print value
	_, err := fmt.Fprintf(w, "%-*s", config.view.ColsWidth[i]+2, s.Result.Values[rownum][i].String)
	return err
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
