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
				// Guarded exactly like the send at the top of the loop: this is the only send in
				// collectStat that used to be bare. A collector parked here makes mainLoop's
				// wg.Wait() on the UI-rebuild path (top/ui.go:99-102) eternal.
				select {
				case statCh <- stat.Stat{Error: err}:
					// ok, error received.
				case <-ctx.Done():
					// quit received, close channel and return.
					close(statCh)
					return
				}
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
		// The ONE stamp of this live frame: "when it was rendered", which is what the store's at
		// field documents. It is captured once here and fed to both consumers - the store now, and
		// the header clock once task 04 threads it into renderSysstat. Do not add a second
		// time.Now() call on this path.
		now := time.Now()

		return renderFrame(g, app, s, props, liveRender(now))
	})
}

// renderFrame is the render core shared by the live path (printStat) and the repaint path
// (repaintStoredFrame, top/pause.go). The two differ only in p: the error policy is the caller's
// (this function propagates, and the repaint closure swallows), while p carries the render
// timestamp, the logtail source and whether the result is published into the store.
//
// It exists because the repaint cannot go through printStat unchanged: this body returns errors at
// thirteen points, and an error out of a g.Update closure tears down MainLoop -> UI rebuild ->
// fresh repaint -> the same error, bounded only by the errorRate guard that kills the process.
//
// MUST be called on the gocui MainLoop goroutine only - it draws into views and publishes the
// gocui-owned frame store.
func renderFrame(g *gocui.Gui, app *app, s stat.Stat, props stat.PostgresProperties, p renderParams) error {
	v, err := g.View("sysstat")
	if err != nil {
		return fmt.Errorf("set focus on sysstat view failed: %w", err)
	}
	v.Clear()
	err = printSysstat(v, s, app.config.verbose, app.db.Local, props.DataDirectory, app.config.refresh, p.at)
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

	// The logtail pair this frame actually drew, filled in by the live logtail case below and left
	// zero by every other panel. Declared HERE, before the extra block, because the block is skipped
	// entirely when the panel is closed (ShowExtra == stat.CollectNone) - and a closed panel is
	// precisely the case the store's drop rule exists for.
	var logPath string
	var logBuf []byte

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
			// The logtail source is a render parameter (Decision 6), and selectLogtail is where that
			// parameter is honoured: on the repaint path the thunk below is NOT ENTERED, so there is
			// no os.Stat, no Read, no Reopen and no Size bookkeeping while paused. That is what keeps
			// the rotation machinery out of the paused state, where a Reopen would close the current
			// file before querying a database that may be exactly the thing that broke.
			//
			// The selection is a separate function rather than an if here because this switch needs a
			// live *gocui.Gui/*gocui.View, which cannot be constructed outside the gocui package - so
			// a routing regression written here would be caught by no test at all. Everything that
			// needs g or v stays inside the thunk; only the choice between the two sources moved out.
			//
			// ACCEPTED RESIDUAL, pre-existing and merely widened here: the descriptor is held for the
			// whole pause, so a rotated-away file keeps its inode pinned, and if the new file outgrows
			// the frozen logtail.Size before the pause is lifted the size-based rotation detector below
			// does not fire and the panel shows stale lines. The window is one refresh interval wide
			// today; closing it needs an identity check (os.SameFile/mtime) instead of a size
			// comparison, which is a separate change.
			path, buf, err := selectLogtail(p, func() (string, []byte, error) {
				size, buf, err := readLogfileRecent(v, app.config.logtail)
				if err != nil {
					printCmdline(g, "Tail Postgres log failed: %s", err)
					return "", nil, err
				}

				if size < app.config.logtail.Size {
					v.Clear()
					err := app.config.logtail.Reopen(app.db, app.postgresProps.VersionNum)
					if err != nil {
						printCmdline(g, "Tail Postgres log failed: %s", err)
						return "", nil, err
					}
				}

				// Update info about logfile size.
				app.config.logtail.Size = size

				// The path is read AFTER the rotation branch: Reopen re-resolves it, and it is what
				// printLogtail puts in the panel's header line.
				return app.config.logtail.Path, buf, nil
			})
			if err != nil {
				return err
			}

			// Hand the drawn pair to the sync call below. Whether it is stored is one decision, made
			// once, at the tail of this function - and only on the live path, since publishFrame
			// returns before the capture when the params describe a repaint.
			logPath, logBuf = path, buf

			err = printLogtail(v, path, buf)
			if err != nil {
				return err
			}
		}
	}

	// Publish what was just rendered into the store - at the TAIL, after the panels and the extra
	// block, so the stored frame is by construction the frame that reached the screen. p decides
	// whether this writes at all: only the live path publishes (Decision 2).
	//
	// Keep this a call to the named helper, in this position: the logtail capture rides on the live
	// side of exactly this step. ShowExtra is passed explicitly and the call is unconditional, so the
	// capture/drop decision is reached for EVERY ShowExtra value - including stat.CollectNone, whose
	// whole extra block above is skipped.
	publishFrame(&app.frame, p, s, app.config.view.ShowExtra, logPath, logBuf)

	return nil
}

// logtailReader produces the pair the log panel draws by reading the log file: the header path and
// the recent lines. It is a function value so that WHICH source is used can be decided separately
// from the read itself - the read needs a *gocui.View for its geometry and a *gocui.Gui for its
// error message, neither of which can be constructed outside the gocui package.
type logtailReader func() (string, []byte, error)

// selectLogtail chooses where the log panel's content comes from, and is the single place that
// decision is made.
//
// On the repaint path (p.fromFile == false) it returns the pair captured with the frozen frame and
// NEVER invokes read: while paused the log file is not touched at all - no os.Stat, no Logfile.Read,
// no Reopen, no logtail.Size write (Decision 7). On the live path it returns whatever read produced,
// propagating its error so renderFrame can surface an unreadable log.
//
// It exists as its own function purely so that property is testable: renderFrame's switch is
// unreachable from a unit test, so the same check written inline there could regress - be deleted,
// inverted, short-circuited - with every test in the package still green. Here a test can hand it a
// read that fails the test the moment it is called.
func selectLogtail(p renderParams, read logtailReader) (string, []byte, error) {
	if !p.fromFile {
		return p.logPath, p.logBuf, nil
	}

	return read()
}

// printSysstat prints system stats on UI. It is a thin wrapper that delegates to the
// writer-based renderSysstat (*gocui.View implements io.Writer), so the render core can be
// unit-tested without a live terminal — mirroring the printDbstat → renderDbstat precedent.
func printSysstat(v *gocui.View, s stat.Stat, verbose bool, local bool, dataDir string, refresh time.Duration, at time.Time) error {
	return renderSysstat(v, s, verbose, local, dataDir, refresh, at)
}

// renderSysstat is the writer-based core of printSysstat: it prints the system stats to w.
// When verbose is set it appends three extended rows (iostat/nicstat/filesyst) consistent with the
// full B/N/F side panels: the iostat/nicstat rows select the max-%util device reusing the struct
// math already computed by count*Usage (never recomputed), and filesyst shows the data_directory's
// filesystem. local/dataDir drive the filesyst mount-prefix match (data_directory symlinks are
// resolved only when local). refresh is the current refresh interval, shown on line 1 so the value
// set through the 'z' dialog stays visible after the dialog closes.
//
// at is the frame's RENDER TIME, supplied by the caller — it is deliberately not time.Now() here.
// The repaint path (repaintStoredFrame, top/pause.go) draws a frozen frame with its stored stamp,
// so a clock read inside this function would print the current time above statistics that are
// minutes old — the incoherence the pause feature exists to avoid. It is passed through as-is: the
// layout carries no zone and Format does not convert, so a local stamp renders as local time.
func renderSysstat(w io.Writer, s stat.Stat, verbose bool, local bool, dataDir string, refresh time.Duration, at time.Time) error {
	var err error

	/* line1: current time, refresh interval and load average */
	// The interval is printed as whole seconds explicitly: Duration.String() would render 60s as
	// "1m0s" and 300s (the validated maximum) as "5m0s". The conversion lives here, not at the call
	// site, so there is exactly one of it.
	_, err = fmt.Fprintf(w, "pgcenter: %s, refresh: %ds, load average: %.2f, %.2f, %.2f\n",
		at.Format("2006-01-02 15:04:05"), int(refresh/time.Second),
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

// cacheHitWidth is the reserved width of the verbose "cache hit ratio" value: 6 columns for the
// number ("%6.2f") plus 1 for the trailing '%' = 7 (e.g. "100.00%", " 99.99%"). The n/a sentinel
// is right-aligned into the same width so the trailing label stays static across ticks.
const cacheHitWidth = 7

// sizeFieldWidth is the reserved width of the verbose Size fields (databases size/growth,
// replication lag/retain/archiving-backlog). The widest realistic Size string is 7 chars
// ("1023.9M"/"1023.9G"/"1023.9T"); reserving 8 gives one column of margin and right-aligns
// cleanly. The value (pretty.SizeWidth) and its n/a sentinel (naReserve) share this width so
// the trailing label stays static across ticks and between the value and n/a states.
const sizeFieldWidth = 8

// bold wraps a rendered value in the same SGR sequence the compact summary rows use for their
// values (SGR 37 = white fg, 1 = bold; see the %cpu row at the top of renderSysstat). It exists as
// a single helper so the escape pair is not retyped at every verbose value site — and so the
// degraded renderings can stay deliberately unwrapped: bold must read as "there is a real number
// here", so the n/a sentinels (naLiteral, naReserve, including the one inside naInt) and the
// filesystem identifier fields (device, mountpoint, fstype) never go through it. Composite A/B
// values are wrapped as ONE span, matching the activity/autovacuum rows.
func bold(s string) string { return "\033[37;1m" + s + "\033[0m" }

// renderSysstatVerbose appends the three verbose system rows to w. Each row degrades independently:
// no active device / first tick / no mount match renders n/a for that row without aborting the others.
func renderSysstatVerbose(w io.Writer, s stat.Stat, local bool, dataDir string) error {
	// iostat row: select the max-%util device among active ones (Completed != 0), the same device
	// set printIostat shows. The first verbose tick (s.VerboseFirstTick) has no valid prev, so the
	// delta fields render n/a rather than a misleading zero — NOT keyed on len(slice) (the slice is
	// populated zero-delta on the first tick).
	if idx := maxUtilDisk(s.Diskstats); idx < 0 || s.VerboseFirstTick {
		// The device count is a real number even in this branch, so it is bold here too — the one
		// place where "degraded branch" and "plain" come apart. The delta fields are n/a: plain.
		if _, err := fmt.Fprintf(w, "  iostat: %s devices, %s max util, %s, %s, %s, %s\n",
			bold(pretty.ReserveWidth(activeDiskCount(s.Diskstats), 2)), naLiteral, naLiteral, naLiteral, naLiteral, naLiteral); err != nil {
			return err
		}
	} else {
		d := s.Diskstats[idx]
		if _, err := fmt.Fprintf(w, "  iostat: %s devices, %s%% max util, %s, %s r/s, %s, %s w/s\n",
			bold(pretty.ReserveWidth(activeDiskCount(s.Diskstats), 2)),
			bold(pretty.ReserveWidth(pretty.Ceil(d.Util), 3)),
			bold(pretty.RateUnitPrefixed(d.Rsectors, pretty.FamilyDisk, "r", 4)),
			bold(pretty.ReserveWidth(pretty.Ceil(d.Rcompleted), 5)),
			bold(pretty.RateUnitPrefixed(d.Wsectors, pretty.FamilyDisk, "w", 4)),
			bold(pretty.ReserveWidth(pretty.Ceil(d.Wcompleted), 5))); err != nil {
			return err
		}
	}

	// nicstat row: select the max-Utilization interface among active ones (Packets != 0), the same
	// set printNetdev shows. rMbps/wMbps replicate printNetdev's print-time Rbytes/1024/128 exactly.
	if idx := maxUtilNet(s.Netdevs); idx < 0 || s.VerboseFirstTick {
		// As in the iostat row: the interface count is real here, the delta fields are not.
		if _, err := fmt.Fprintf(w, " nicstat: %s devices, %s max util, %s, %s, %s err/coll\n",
			bold(pretty.ReserveWidth(activeNetCount(s.Netdevs), 2)), naLiteral, naLiteral, naLiteral, naLiteral); err != nil {
			return err
		}
	} else {
		n := s.Netdevs[idx]
		// err/coll is a composite value: one span for both sides, as in the activity row.
		errColl := fmt.Sprintf("%s/%s",
			pretty.ReserveWidth(pretty.Ceil(n.Rerrs+n.Terrs), 4),
			strconv.Itoa(pretty.Ceil(n.Tcolls)))
		if _, err := fmt.Fprintf(w, " nicstat: %s devices, %s%% max util, %s, %s, %s err/coll\n",
			bold(pretty.ReserveWidth(activeNetCount(s.Netdevs), 2)),
			bold(pretty.ReserveWidth(pretty.Ceil(n.Utilization), 3)),
			bold(pretty.RateUnitPrefixed(n.Rbytes/1024/128, pretty.FamilyNet, "r", 4)),
			bold(pretty.RateUnitPrefixed(n.Tbytes/1024/128, pretty.FamilyNet, "w", 4)),
			bold(errColl)); err != nil {
			return err
		}
	}

	// filesyst row: the data_directory's filesystem by longest mount-prefix. Any match failure
	// (no mount, empty data_directory, EvalSymlinks failure) renders n/a.
	if fs, ok := stat.MatchDataDirFs(dataDir, s.Fsstats, local); ok {
		// device/mountpoint/fstype are identifiers, not values — plain, like the panels' line 1.
		if _, err := fmt.Fprintf(w, "filesyst: %s on %s (%s), %s size, %s used, %s%% use\n",
			fs.Mount.Device, truncate(fs.Mount.Mountpoint, 10), fs.Mount.Fstype,
			bold(pretty.Size(fs.Size)), bold(pretty.Size(fs.Used)),
			bold(fmt.Sprintf("%3.0f", fs.Pused))); err != nil {
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

// naReserve renders the n/a sentinel right-aligned into the SAME reserved width a value would
// occupy, so n/a is a drop-in that preserves the column position of whatever label/field follows
// it on the same line (a degraded value must not shift the trailing label). The effective width is
// at least len(naLiteral) so the sentinel is never truncated; the value side must reserve the same
// effective width for the toggle to be static.
func naReserve(width int) string {
	if width < len(naLiteral) {
		width = len(naLiteral)
	}
	return fmt.Sprintf("%*s", width, naLiteral)
}

// naInt renders an int rate field as a fixed-width number, or n/a in the same reserved width when
// this tick has no prev snapshot (hasPrev == false) — so a first-tick delta is distinguishable from
// a real zero AND the n/a occupies the value's column slot, keeping the trailing label static.
//
// The bold lives inside the value branch on purpose: the sentinel path stays deliberately
// unwrapped (bold means "there is a real number here"), and putting the decision here covers every
// call site without touching any of them.
func naInt(v int64, width int, hasPrev bool) string {
	if !hasPrev {
		return naReserve(width)
	}
	return bold(pretty.ReserveWidth(int(v), width))
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
	// Bold is applied where the number is produced (inside the availability branch), never to the
	// variable afterwards — that keeps the naReserve default plain automatically.
	size, growth := naReserve(sizeFieldWidth), naReserve(sizeFieldWidth)
	if o.TotalSizeValid {
		size = bold(pretty.SizeWidth(float64(o.TotalSize), sizeFieldWidth))
		if hp {
			growth = bold(pretty.SizeWidth(float64(o.GrowthPerSec), sizeFieldWidth))
		}
	}
	// cache hit ratio is the trailing field before its label; reserve a fixed width for the value
	// (6 digits + the '%' = 7, e.g. "100.00%" / " 99.99%") so the n/a sentinel (right-aligned into
	// the same 7) is a drop-in and the "cache hit ratio" label never moves between ticks.
	hit := naReserve(cacheHitWidth)
	if o.CacheHitRatioValid {
		hit = bold(fmt.Sprintf("%6.2f%%", o.CacheHitRatio))
	}
	if _, err := fmt.Fprintf(w, "   databases: %s per %s databases, %s growth/s, %s cache hit ratio\n",
		size, bold(pretty.ReserveWidth(int(o.DatabasesCount), 2)), growth, hit); err != nil {
		return err
	}

	// workers row. Active counts / GUC limits (umbrella max_worker_processes, logical, parallel).
	// Each active/max pair is one bold span, matching the autovacuum row's workers/max.
	workers := func(active, guc int) string {
		return bold(fmt.Sprintf("%s/%d", pretty.ReserveWidth(active, 2), guc))
	}
	if _, err := fmt.Fprintf(w, "     workers: %s workers/max, %s logical workers, %s parallel workers\n",
		workers(o.WorkersUmbrellaActive, props.GucMaxWorkerProcesses),
		workers(o.WorkersLogicalActive, props.GucMaxLogicalReplicationWorkers),
		workers(o.WorkersParallelActive, props.GucMaxParallelWorkers)); err != nil {
		return err
	}

	// replication row. lag/slots-retain/archiving-backlog are n/a when their source is unavailable
	// (no standby, no slots, archive_mode=off / missing privilege).
	lag := naReserve(sizeFieldWidth)
	if o.LagBytesValid {
		lag = bold(pretty.SizeWidth(float64(o.LagBytes), sizeFieldWidth))
	}
	retain := naReserve(sizeFieldWidth)
	if o.RetainedValid {
		retain = bold(pretty.SizeWidth(float64(o.RetainedBytes), sizeFieldWidth))
	}
	backlog := naReserve(sizeFieldWidth)
	if o.ArchivingBacklogValid {
		backlog = bold(pretty.SizeWidth(float64(o.ArchivingBacklog), sizeFieldWidth))
	}
	// slots/retain is the one composite wrapped as TWO spans: an always-real count next to a size
	// that can degrade to n/a — a single span would drag the sentinel into the bold.
	if _, err := fmt.Fprintf(w, " replication: %s wal size, %s lag, %s/%s slots/retain, %s archiving backlog, %s senders/receivers\n",
		bold(pretty.Size(float64(o.WalSize))), lag,
		bold(pretty.ReserveWidth(int(o.SlotsCount), 2)), retain,
		backlog, bold(fmt.Sprintf("%d/%d", o.Senders, o.Receivers))); err != nil {
		return err
	}

	// bgwr/ckpt row. timed/req are absolute cumulative; write/sync ms are interval deltas (n/a on
	// the first tick); maxwritten is the interval delta count. The whole delta group toggles
	// together on the first tick (hp), and syncMs is the intentionally tight post-slash value (its
	// width varies by design — the A/B composite rule from 95656e8), so the downstream " maxwritten"
	// label is governed by that tight composite, not by a fixed-reserve n/a — left as-is.
	writeMs, syncMs, maxw := naLiteral, naLiteral, naLiteral
	if hp {
		writeMs = bold(pretty.ReserveWidth(pretty.Ceil(o.CkptWriteMsDelta), 3))
		syncMs = bold(strconv.Itoa(pretty.Ceil(o.CkptSyncMsDelta))) // tight post-slash value
		maxw = bold(pretty.ReserveWidth(int(o.MaxWrittenDelta), 2))
	}
	// timed/req is one span (both sides are always real); write/sync stays two adjacent spans
	// because both sides degrade together to their own plain n/a.
	timedReq := fmt.Sprintf("%s/%s", pretty.ReserveWidth(int(o.CkptTimed), 2), strconv.Itoa(int(o.CkptReq)))
	if _, err := fmt.Fprintf(w, "   bgwr/ckpt: %s timed/req, %s/%s ms write/sync, %s maxwritten\n",
		bold(timedReq), writeMs, syncMs, maxw); err != nil {
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
	// One-shot auto-scroll: a sort-column change asked for that column to be brought into the
	// window. It is consumed here rather than in the key handler because column widths are known
	// only after alignViewToResult has run against real data (printDbstat). The flag is cleared
	// BEFORE the offset is recomputed, which makes the request strictly one-shot even if the
	// computation returns early — so manual [ / ] scrolling afterwards is never undone by the next
	// refresh ([009] invariant). Note that printDbstat returns early on an error frame without
	// reaching this point: a pending request simply survives that frame and fires on the next good
	// one, which is the desired behaviour.
	if config.autoScrollToOrderKey {
		config.autoScrollToOrderKey = false
		config.scrollOffset = scrollOffsetFor(
			s.Result.Ncols, config.view.ColsWidth, termWidth,
			config.scrollOffset, config.view.OrderKey,
		)
	}

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

// scrollOffsetFor returns the scroll offset that brings column orderKey into the visible window
// with the SMALLEST movement from the current offset, or offset unchanged when that column is
// already visible. Column 0 is frozen and always printed (printStatHeader), so it never scrolls.
//
// The answer is found by probing visibleColumns rather than by repeating its walk: the marker
// reservation in both directions is the subtle part of that function (it already shipped one bug
// invisible to unit tests), and a second copy of the arithmetic is how the two would drift apart.
// ncols is at most a few dozen, so the probing is free.
//
// A partially visible column counts as visible — that is the window semantics of countFit above,
// and any stricter notion would fight maxOffset forever.
func scrollOffsetFor(ncols int, colsWidth map[int]int, termWidth, offset, orderKey int) int {
	// Bounds guard (issue #99 class): config.view.Ncols, which the sort handlers wrap on, and the
	// fresh result's Ncols can disagree for one frame after a view switch. Check against the
	// result's column count, which is what the window is computed from.
	if orderKey <= 0 || orderKey >= ncols {
		return offset
	}

	win := visibleColumns(ncols, colsWidth, termWidth, offset)

	// Empty window (last < first): the terminal has no room for any scrollable column beside the
	// frozen one, so no offset can reveal the sort column. Leave the window where it is.
	if win.last < win.first {
		return offset
	}

	// Already visible, fully or partially: no jerk.
	if orderKey >= win.first && orderKey <= win.last {
		return win.clamped
	}

	// Hidden to the left: win.first == 1 + clamped, so offset orderKey-1 puts the column exactly
	// at the left edge — the largest offset that reveals it, hence the smallest movement.
	if orderKey < win.first {
		return orderKey - 1
	}

	// Hidden to the right: walk offsets rightwards and take the first window that admits it. The
	// walk is bounded by the last possible offset; visibleColumns re-clamps each probe.
	for off := win.clamped + 1; off <= ncols-1; off++ {
		w := visibleColumns(ncols, colsWidth, termWidth, off)
		if orderKey >= w.first && orderKey <= w.last {
			return w.clamped
		}
	}

	// No offset admits the column (the window is too narrow at every position): leave the offset
	// alone rather than landing on an arbitrary edge.
	return offset
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
//
// The printer FORMATS the result set and never edits it: the value is read into a local and
// truncated there. Editing in place would make a stored frame lose its original text on the
// first render — a column widened afterwards could never show it again, since there is no
// fresh frame to restore it — and, for views with DiffIntvl == [0,0], the values array is the
// collector's own snapshot (calculateDelta returns curr unchanged), so the write would reach
// back into data the collector still holds.
func printDataCell(w io.Writer, s stat.Stat, config *config, rownum, i int) error {
	value := s.Result.Values[rownum][i].String

	// truncate values that are longer than column width
	if len(value) > config.view.ColsWidth[i] {
		width := config.view.ColsWidth[i]
		if width <= 0 {
			return fmt.Errorf("zero or negative width, skip")
		}

		// truncate value up to column width and replace last character with '~' symbol
		value = value[:width-1] + "~"
	}

	// print value
	_, err := fmt.Fprintf(w, "%-*s", config.view.ColsWidth[i]+2, value)
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

// printLogtail prints 'logtail' - last lines of Postgres log. It is the thin *gocui.View wrapper of
// renderLogtail (the printSysstat -> renderSysstat precedent) and owns the one thing a writer cannot
// express: whether the view is cleared at all.
//
// The Clear stays INSIDE the emptiness guard, exactly as before. That is what makes an empty buffer
// a true no-op on both paths: on the live path a quiet log leaves the previous lines on screen, and
// on the repaint path an empty store leaves the view as it is - blank after a UI rebuild ("nothing
// to show"), intact after an overlay closed over it. Both are correct; moving the Clear out would
// blank the panel on every quiet interval.
func printLogtail(v *gocui.View, path string, buf []byte) error {
	if len(buf) == 0 {
		return nil
	}

	// clear view's content and read the log
	v.Clear()

	return renderLogtail(v, path, buf)
}

// renderLogtail is the writer-based core of printLogtail: the highlighted path header followed by
// the raw buffer, and nothing else.
//
// It is also the logtail source of the REPAINT path - renderFrame calls it through printLogtail with
// the pair captured alongside the frozen frame, and performs no file access whatsoever while paused.
// The io.Writer sink is what makes that reachable from a test with a *bytes.Buffer; the *gocui.View
// is adapted at the wrapper boundary above.
func renderLogtail(w io.Writer, path string, buf []byte) error {
	if len(buf) == 0 {
		return nil
	}

	_, err := fmt.Fprintf(w, "\033[30;47m%s:\033[0m\n", path)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "%s", string(buf))
	return err
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
