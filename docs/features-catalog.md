# Features Catalog

User-facing capabilities of the product. Updated after each feature is finalized.
Used by spec-writer to understand existing functionality and avoid duplication or conflicts.

---

### [001-feat-per-process-system-stats] Per-process System Stats Screen

**What it does:** Opens a new TUI screen (`Shift+S`) showing all PostgreSQL backends with their real-time CPU utilization and IO activity read from Linux procfs, alongside the pg_stat_activity columns. Lets DBA immediately see whether a specific backend is CPU-bound or IO-bound without leaving pgcenter.

**Key scenarios:**
- Press `Shift+S` to switch to the per-process stats screen showing 19 columns: activity info (pid, datname, usename, state, wait_etype, wait_event) + accumulated CPU time (`all_total,s`, `us_total,s`, `sy_total,s`) + accumulated IO (`read_total,KiB`, `write_total,KiB`, `iodelay_total,s`) + CPU rates (`%all`, `%us`, `%sy`) + IO rates (`read,KiB/s`, `write,KiB/s`, `%iodelay`) + query. The two `iodelay*` columns are added by [002-feat-iodelay-procpidstat].
- Press `I` to hide idle backends, press `A` to filter by minimum age threshold — same controls as the activity screen
- Run pgcenter without root or postgres-user privileges: IO columns show empty, CPU columns work normally, warning message explains the required permissions

**Limitations:**
- Local mode only — procfs is not available over remote PostgreSQL connections
- Auxiliary PostgreSQL processes (checkpointer, bgwriter, WAL writer, archiver) are not shown — they are absent from `pg_stat_activity`
- IO metrics are syscall-level bytes (includes page cache), not actual disk IO

**Touches:** Activity screen (shares `I` and `A` filter hotkeys and their guard logic in `top/config_view.go` and `top/dialog.go`).

---

### [002-feat-iodelay-procpidstat] Per-process IO Delay Columns

**What it does:** Adds two columns to the per-process stats screen (`Shift+S`) — `iodelay_total,s` (accumulated time the backend spent blocked on block IO, `HH:MM:SS`) and `%iodelay` (share of wall-clock time spent blocked in D-state between ticks, percent). Source is `/proc/[pid]/stat` field 42 (`delayacct_blkio_ticks`). Lets DBA distinguish a backend genuinely waiting on disk from one that drives high IO throughput through the page cache, without leaving pgcenter and without `iotop`.

**Key scenarios:**
- Delay accounting enabled (`kernel.task_delayacct=1`): both columns show real values. On the first tick after opening the screen, `iodelay_total,s` shows the accumulated value and `%iodelay` is `""` (no previous sample for the delta). On subsequent ticks both columns update.
- Delay accounting disabled or unsupported (`kernel.task_delayacct=0`, file absent on kernels without `CONFIG_TASK_DELAY_ACCT`): both columns render as `""` and a warning appears in the cmdline area: `"iodelay unavailable (task_delayacct=0): run sysctl -w kernel.task_delayacct=1, then re-open screen"`. If `/proc/[pid]/io` is also unavailable, a single combined warning is shown instead.
- `%iodelay` greater than 100% on multi-threaded blocking patterns: documented behavior, not a bug — `delayacct_blkio_ticks` is wall-clock blocked time and is not normalized by `runtime.NumCPU()`.

**Limitations:**
- Requires `CONFIG_TASK_DELAY_ACCT=y` in the kernel and `kernel.task_delayacct=1` at runtime
- Local mode only — procfs is not available over remote PostgreSQL connections
- Availability probe is single-shot at screen open (`switchViewToProcPidStat`); flipping the sysctl in runtime requires closing the screen and pressing `Shift+S` again to re-probe
- Inherits the [001-feat-per-process-system-stats] limitations: not shown for auxiliary PostgreSQL processes
- iodelay columns are included when recording with `pgcenter record` (see [003-feat-procpidstat-record-report])

**Touches:** Per-process stats screen from [001-feat-per-process-system-stats] (column count 17→19, screen handler grows a 4-branch warning composition, `ProcPidStat` struct gains an `IODelay` field).

---

### [003-feat-procpidstat-record-report] Per-process Stats Record/Report

**What it does:** Enables `pgcenter record` to automatically capture per-process system stats (CPU, IO, iodelay) alongside all other statistics, and `pgcenter report -N` to replay recorded data for post-mortem analysis. Each recorded snapshot contains the full 19-column procpidstat result with pre-computed per-interval rates.

**Key scenarios:**
- Run `pgcenter record -f stats.tar -i 10s` overnight; next morning run `pgcenter report -N -f stats.tar -s 03:10 -e 03:50 -o "%all"` to see which backend was CPU-heavy during an incident window
- Filter by query pattern: `pgcenter report -N -f stats.tar -g "query:SELECT.*orders"` to see history of a specific query's resource usage
- All formatting options (`-s`, `-e`, `-o`, `-g`, `-l`, `-t`) work with `-N` the same as with `-A`

**Limitations:**
- Local mode only — `pgcenter record` skips procpidstat automatically when connecting to a remote PostgreSQL; the tar will contain all other stats but not procpidstat
- Rate columns (`%all`, `read,KiB/s`, `%iodelay`) reflect the recording interval — the report does not recompute rates for custom `-s`/`-e` windows
- Accumulated columns (`all_total,s`, `read_total,KiB`) show absolute values since process start, not per-interval deltas
- IO and iodelay columns are empty (`""`) if the recorder lacked permissions or kernel support; report shows WARNING in header

**Touches:** [001-feat-per-process-system-stats] (uses the same procpidstat 19-column pipeline); [002-feat-iodelay-procpidstat] (iodelay columns included in recorded data).

---

### [004-feat-bgwriter-checkpointer] Background Writer / Checkpointer Screen

**What it does:** Adds a new single-row TUI screen (`bgwriter`, hotkey `b`) showing background-writer and checkpoint activity from `pg_stat_bgwriter` and (on PG 17+) `pg_stat_checkpointer`. Lets DBA watch checkpoint frequency, the timed-vs-requested ratio, checkpoint write/sync cost, and who flushes buffers (checkpointer / bgwriter / backends) without leaving pgcenter for `psql`.

**Key scenarios:**
- Press `b` to open the screen. Event counters (`ckpt_timed`, `ckpt_req`, and on PG 17+ `rstpt_timed`/`rstpt_req`/`rstpt_done`) show as absolute cumulative values; work/time/buffer columns (`ckpt_write,ms`, `ckpt_sync,ms`, `buf_ckpt`, `buf_clean`, `maxwritten`, `buf_alloc`, …) update as per-interval deltas; `stats_age` shows how long counters have accumulated.
- Diagnose forced checkpoints: watch `ckpt_req` climb faster than `ckpt_timed` — checkpoints are forced by WAL pressure, raise `max_wal_size`.
- Tune bgwriter on PG ≤ 16 by comparing `buf_clean` (bgwriter) against `buf_backend` (backends) and watching `maxwritten`.
- Monitor restartpoints on a standby (PG 17+): `ckpt_*` stay 0 while `rstpt_*` accumulate.

**Limitations:**
- Recordable since [008-feat-record-report-0-11-views] — `pgcenter record` captures it and `pgcenter report -B` replays it (version-aware by the recording's PG version).
- `buf_backend` / `buf_backend_fsync` columns appear only on PG ≤ 16 (the data moved to `pg_stat_io` on PG 17+).
- The column set differs per server version (PG 18 adds a `slru_written` column); no NULL placeholders for columns absent on a given version.
- `stats_age` on PG 17+ comes from `pg_stat_checkpointer`; a separate `pg_stat_bgwriter` reset is not reflected.

**Touches:** Shares the single-row version-aware view model with the `pg_stat_wal` screen. Record/report support added later in [008-feat-record-report-0-11-views].

---

### [005-feat-replication-slots] Replication Slots Screen

**What it does:** Adds a new multi-row TUI screen (`replslots`, hotkey `o`) showing one row per replication slot — physical and logical — from a hybrid `pg_replication_slots` + `pg_stat_replication_slots` query. Lets a DBA find which slot is retaining WAL (the classic disk-fill incident) and watch logical-decoding spill/stream pressure without dropping to `psql`. Same 15 columns on PostgreSQL 14–18.

**Key scenarios:**
- Press `o` to open the screen, sorted by `retained,KiB` descending — the slot holding the most WAL is on top. Columns: `slot_name`, `slot_type`, `active`, `wal_status` (reserved/extended/unreserved/lost), `retained,KiB`, `safe,KiB` (absolute state); `spill_txns/count`, `spill,KiB`, `stream_txns/count`, `stream,KiB`, `total_txns`, `total,KiB` (per-interval deltas); `stats_age`.
- Diagnose a disk-fill incident: a slot with high `retained,KiB` and `active=false` (a disconnected standby or subscription) is the culprit — revive the consumer or drop the slot.
- Catch a slot before it breaks: watch `wal_status` move to `unreserved`/`lost`.
- Tune logical decoding: rising `spill,KiB` per interval means decoding spills to disk — raise `logical_decoding_work_mem`.
- Re-sort (arrows) and filter (`/`) like the other multi-row screens.

**Limitations:**
- Recordable since [008-feat-record-report-0-11-views] — `pgcenter record` captures it and `pgcenter report -L` replays it, which matters most for this feature (disk-fill incidents are often forensic).
- Physical slots show `0` (not blank) in the spill/stream/total columns — those metrics are logical-decoding-only; the adjacent `slot_type=physical` disambiguates.
- Invalidation **cause** is not shown (`conflicting`/`invalidation_reason` omitted to keep one query across PG 14–18); `wal_status` conveys the state (`lost`/`unreserved`).
- `pg_stat_subscription_stats` (subscriber-side) is out of scope — a separate future feature.

**Touches:** Shares the multi-row sort/filter/diff view model with the `pg_stat_replication` and `tables` screens. Record/report support added later in [008-feat-record-report-0-11-views].

---

### [006-feat-pg-stat-io] pg_stat_io Screen

**What it does:** Adds a new multi-row TUI screen for `pg_stat_io` (PostgreSQL 16+) showing the unified IO breakdown by `backend_type × object × context` — one row per combination, cumulative counters as per-interval rates. Because the view is too wide for one screen (pgcenter has no horizontal scroll), it is split into two sub-screens: **count** (operations + KiB throughput) and **time** (read/write/fsync latencies). Lets a DBA see during an IO spike who drives the IO (vacuum vs client backends vs checkpointer) and through which context, and — on PG 18 — watch WAL IO that left `pg_stat_wal`.

**Key scenarios:**
- Press `j` to open the count screen (sorted by `reads` desc); press `j` again to toggle to the time screen; press `J` for the 2-item mode menu. Columns: `io_key`, `backend_type`, `object`, `context` (identity); count screen adds `reads`/`read,KiB`/`writes`/`write,KiB`/`extends`/`ext,KiB`/`hits`/`evictions`/`writebacks`/`reuses`/`fsyncs`; time screen adds `read_time`/`write_time`/`writeback_time`/`extend_time`/`fsync_time`; both end with `stats_age`.
- Triage an IO spike: filter (`/`) by `backend_type` or `object` to isolate, e.g., `autovacuum worker` reads in the `vacuum` context vs client-backend reads.
- See WAL IO on PG 18: filter `object=wal` to watch WAL write/fsync pressure that is no longer in `pg_stat_wal`.
- Judge buffer pressure: sort by `evictions`/`reuses` for ring-buffer churn.
- Find slow IO paths: on the time screen (with `track_io_timing=on`), sort by `read_time`/`fsync_time`.

**Limitations:**
- Recordable since [008-feat-record-report-0-11-views] — `pgcenter record` captures both views and `pgcenter report -J c` (count) / `-J t` (time) replays them.
- PG 16+ only (the view does not exist on PG 14/15 — those show "not supported"). PG 16 and 17 share one column shape; PG 18 differs (native `*_bytes`, `object='wal'`, `context='init'`).
- The time screen is empty (all zeros) unless `track_io_timing=on`; a cmdline hint says so. WAL timings on PG 18 also need `track_wal_io_timing=on`.
- Rows where all operation counters are zero are hidden in SQL. The synthetic `io_key` (md5 of the three dimensions) is shown like the `pg_stat_statements` `queryid`; per-dimension columns are shown separately for sorting/filtering.
- `Q` does not reset `pg_stat_io` (it is shared/cluster-wide stats) — noted in the help screen.

**Touches:** Shares the multi-row view model with `replslots`/`tables`. Record/report support for both views added later in [008-feat-record-report-0-11-views]. Closes the visibility gaps left by [004-feat-bgwriter-checkpointer] (`buffers_backend` on PG 17+) and the `pg_stat_wal` screen (WAL IO timings on PG 18).

### [007-feat-pg-stat-statements-jit] pg_stat_statements JIT Screen

**What it does:** Adds a 7th `pg_stat_statements` sub-screen — **JIT** — under the `X` menu (and the `x` cycle), showing per-statement JIT compilation cost. Lets a DBA find which normalized queries pay heavy JIT generation/inlining/optimization/emission time (the classic cause of "mysterious" latency on short queries when `jit=on`) without dropping to `psql`.

**Key scenarios:**
- Press `X` → choose `pg_stat_statements JIT compilation` (or press `x` to cycle `… wal → jit → timings …`). The screen opens sorted by `gen_total` (cumulative generation time) descending — the heaviest JIT compilers on top.
- Columns: `user`, `database`, cumulative phase totals `gen_total`/`inline_total`/`opt_total`/`emit_total` (`+deform_total` on PG 17+), per-interval `gen,ms`/`inline,ms`/`opt,ms`/`emit,ms` (`+deform,ms` on PG 17+), `functions` (JIT-compiled functions this interval), `queryid`, `query`.
- Decide whether JIT pays off: a query with large phase times but few rows is a candidate for raising `jit_above_cost`/`jit_optimize_above_cost` or turning JIT off for the workload.
- Re-sort (arrows — `*_total` text durations sort numerically) and filter (`/`) like the other pgss sub-screens.

**Limitations:**
- Recordable since [008-feat-record-report-0-11-views] — `pgcenter record` captures it and `pgcenter report -X j` replays it.
- PG 15+ only (JIT columns appeared in PG 15; `deform_*` in PG 17). On PG < 15 the sub-screen reports "not supported" via the runtime version guard.
- Rows with no JIT activity (`jit_functions = 0`) are hidden in SQL; under `jit=off` the screen is empty and the command line shows a hint.
- The `*_count` phase counters (inlining/optimization/emission) are omitted to fit the screen width (no horizontal scroll) — only `functions` is shown; the value is in the phase times.

**Touches:** Shares the pgss sub-screen model (synthetic md5 `queryid` `UniqueKey`, total+interval columns) with `statements_timings`/`statements_io`. The last of the five 0.11.0 screens (after bgwriter, replslots, stat_io ×2) to get record/report support in [008-feat-record-report-0-11-views]. Closes release 0.11.0.

---

### [008-feat-record-report-0-11-views] Record/Report for the 0.11.0 Screens

**What it does:** Lets `pgcenter record` capture the four screens introduced in 0.11.0 —
background writer/checkpointer, replication slots, pg_stat_io (count + time), and
pg_stat_statements JIT — and `pgcenter report` replay them offline, the same way it already
handles tables/activity/wal/statements. These screens were live-TUI-only before this feature.

**Key scenarios:**
- Record an incident window (`pgcenter record -f stats.tar`) and later replay any of the new
  screens: `report -B` (bgwriter), `-L` (replslots), `-J c` / `-J t` (pg_stat_io count/time),
  `-X j` (statements JIT). Deltas match what the live TUI showed.
- `report -d <flag>` prints the column descriptions for each new report type.
- Replay archives recorded on any of PostgreSQL 14–18 — the report picks the version-correct
  column layout from the recording's metadata.

**Limitations:**
- Empty or pre-0.11 archives print a header only for the new flags (no data, no error).
- The live record↔report seam (recording and replaying against the same running server) is
  verified manually, not by an automated end-to-end test (synthetic-tar + golden tests cover
  the replay/diff logic version-parametrically).

**Touches:** [004]/[005]/[006]/[007] — lifts the `NotRecordable` flag those screens shipped with
and adds their report flags. Reuses the existing pure-SQL record/report pipeline (no recorder or
storage-format change). Pays off tech-debt [004] and [007].

---

### [009-feat-horizontal-scroll] Horizontal Scroll of the Stats Table

**What it does:** Adds by-column horizontal scrolling to the main stats table in `pgcenter top`
so columns that don't fit a narrow terminal can be brought into view. `]` scrolls right, `[`
scrolls left; the first column (the row identifier — PID / database / table name) is **frozen**
and stays in place at any offset. Edge markers `‹`/`›` on the header row show when columns are
hidden to the left/right. Closes issue #14, the most-requested item, open since 2015.

**Key scenarios:**
- On a ~100-column terminal the `query` column on the activity screen is off the right edge and
  the header shows `›`. Press `]` a few times: the window scrolls right, `pid` stays put, `query`
  comes into view, and `‹` now shows on the left. Press `[` to scroll back; `‹` disappears at offset 0.
- Switch to another screen (including the per-process screen via `Shift+S`): the new screen
  opens unscrolled (offset reset to 0). The offset persists across auto-refresh ticks within one screen.
- On a wide terminal where everything fits, `[`/`]` are no-ops and no markers are shown.
- Sorting (`←`/`→`) and scrolling (`[`/`]`) are independent in the sense that a manual scroll is
  never undone by a refresh. **Superseded in part by [015-feat-tui-papercuts]:** changing the sort
  column now scrolls the window to bring that column into view, so "the sort column may sit outside
  the visible window" is no longer expected behaviour.

**Limitations:**
- **Main stats table only.** The side extra-panels (iostat / netdev / fsstats / logtail) do not
  scroll — intentional scope: issue #14 is about the main table, and the panels have a fixed
  narrow field set.
- The `pg_stat_io` two-screen split, the seven `pg_stat_statements` sub-screens, and the
  synthetic `io_key` (see [006]) are **kept as-is** — they are a deliberate logical grouping, not
  a workaround for the (now-removed) lack of scroll. Scroll is for narrow terminals, not for
  collapsing those into one wide view.
- Scrolling is by **column**, not by character — you cannot peek at half a wide column. The last
  visible column may render partially (truncated at the edge) so a wide trailing column like
  `query` is never dropped entirely.
- `record`/`report` is plain stdout, not a TUI, so scroll does not apply there.

**Touches:** Every stat screen of the main table (activity, databases, tables, indexes, sizes,
functions, replication, replslots, wal, bgwriter, progress, all `statements` sub-screens, both
`pg_stat_io` screens, procpidstat) — all share the `printStatHeader`/`printStatData` render path.
Relates to [006-feat-pg-stat-io], whose ADRs cited "no horizontal scroll" as the reason for the
sub-screen splits; this feature removes that constraint but the splits are deliberately retained.

---

### [010-feat-overview-dashboard] Verbose Mode for the Top Summary Panels

**What it does:** Adds a `v` toggle to `pgcenter top` that expands the two always-on summary
panels — `sysstat` (left) and `pgstat` (right) — from 4 rows each into an extended instance-health
overview: `sysstat` gains 3 rows (iostat / nicstat / filesyst), `pgstat` gains 5 rows (workload /
databases / workers / replication / bgwr-ckpt). It is a display mode layered over the current
screen (like the `B`/`F`/`N`/`L` side-panels), **not** a separate screen, so the extended header
rides with the DBA over any detail table (activity, tables, …). Lets a DBA see disk/net/FS
saturation, aggregate workload, total DB size + growth + cache-hit, worker-pool pressure,
replication lag/slots/retained-WAL/archiving-backlog, and checkpoint cost at a glance without
visiting the individual side-panels and screens.

**Key scenarios:**
- Press `v` to expand both panels; the stats table shrinks but stays visible. Press `v` again to
  collapse. The verbose state **persists across screen switches** — turn it on and the health
  summary stays above whatever detail screen you navigate to.
- Verbose aggregates are **consistent with the full panels/screens** — the iostat/nicstat/filesyst
  rows pick the max-`%util` device and match the full `B`/`N`/`F` panels for that device; the
  pgstat rows match the `d`/`r`/`b` screens. Verbose just rounds rate fields up to integers.
- Unavailable signals (no PL/Perl schema over the network, `archive_mode=off`, no replication, no
  privileges, PG version without a needed view, first tick with no prior delta) render the literal
  `n/a` — never `0` or blank — and one failing source does not blank the other rows. The first tick
  shows a transient `collecting...` cmdline hint that clears after the first refresh.

**Limitations:**
- **TUI-only** — verbose is a display mode; it does not participate in `record`/`report`. The view
  registers no new view (so no view-count test churn) and carries no `NotRecordable` flag.
- **Host IO/disk per-device deferred (variant A)** — the iostat row aggregates the existing
  disk/net/fs panel data; a richer host IO/disk breakdown is out of scope for this feature.
- **filesyst shows the data-directory filesystem only** — WAL/tablespaces on other filesystems are
  not shown; the `data_directory` symlink is resolved only in local mode (remote shows the
  unresolved path).
- **Wide-terminal assumption** — verbose rows are long and truncate at the panel edge on narrow
  terminals; on a too-short terminal verbose does not expand (height-guard) and a cmdline hint says so.

**Touches:** The `sysstat`/`pgstat` summary panels (always shown in `top`) and the
iostat/nicstat/fsstat side-panels plus the `databases`/`replication`/`bgwriter` screens whose
aggregates the verbose rows mirror. Builds on the [001] `CollectExtra` enrichment mechanism (the
verbose collection is gated similarly but via a separate `view.Verbose` boolean) and the [009]
pure-render-function precedent (`topBandLayout`).

---

### [011-refactor-tech-debt-paydown] Tech-debt Paydown — Safer Report Replay & Stable Verbose Panel

**What it does:** Internal-quality cleanup of three registered debt items. Bounds the memory `pgcenter report` will allocate when replaying a recorded archive, and stabilises the verbose (`v`) summary panel so its size/lag columns no longer drift horizontally as values change.

**Key scenarios:**
- Replaying a `pgcenter record` archive received from a third party: an entry declaring an absurd size is rejected with a clear error (`result file size … exceeds limit …`) instead of exhausting memory; legitimate archives replay unchanged.
- Verbose mode (`v`) in `pgcenter top`: in the `databases:` and `replication:` rows the size/growth/lag/retain/backlog values are right-aligned in fixed-width slots, so trailing labels hold their horizontal position across ticks and when a value drops to `n/a`.

**Limitations:** No new screens or commands; displayed numbers and units are unchanged (padding only). The `wal size` field is intentionally left variable-width (first field on its row). The internal rate-formatter consolidation is invisible to users.

**Touches:** Hardens the `record`/`report` replay path ([003]/[008]); refines the verbose overview panel introduced by [010-feat-overview-dashboard] (reuses its `naReserve` reserved-width contract).

### [012-feat-pg19-compatibility-baseline] PostgreSQL 19 Support

**What it does:** pgcenter works on PostgreSQL 19 — verified, not assumed: every screen is exercised
against a live PG 19 cluster in CI. On PG 19 the vacuum progress screen also answers "is this vacuum mine
or autovacuum's, and is it aggressive?" without leaving for psql.

**Key scenarios:**
- A DBA who upgraded to PG 19 runs `pgcenter top` and finds every screen working.
- Watching a long vacuum, the DBA reads `mode` (`normal` / `aggressive` / `failsafe`) and `started_by`
  (`manual` / `autovacuum` / `autovacuum_wraparound`) on the left of the screen instead of parsing the
  query text on the right.
- On the analyze progress screen, `started_by` distinguishes a manual analyze from autovacuum's.
- On the basebackup progress screen, `backup_type` distinguishes a full backup from an incremental one.
- Everything above is also recorded by `pgcenter record` and replayed by `report -P v|a|b`.

**Limitations:**
- Values are shown exactly as the server reports them — no abbreviations, and NULL renders blank.
- Archives recorded on PG 19 need pgcenter 0.12.0 or newer; older binaries do not know the new layout and
  would print nonsense rather than an error.
- PG 14–18 are untouched: same columns, same numbers, same layout as before.
- Support is verified against PG 19 beta; a catalog change before GA would be fixed by a patch release.
- The two progress views new in PG 19 (`pg_stat_progress_repack`, `pg_stat_progress_data_checksums`) are
  not shown — they are separate features in the roadmap backlog. Nothing breaks without them: the
  compatibility view `pg_stat_progress_cluster` still reports REPACK operations.

**Touches:** [009-feat-horizontal-scroll] (wider progress layouts are acceptable because the table
scrolls), [008-feat-record-report-0-11-views] (report replay picks the layout from the archive's recorded
version, which is what keeps old archives correct).


---

### [013-feat-activity-xmin-horizon] Activity Screen: xmin Horizon and Parallel Worker Grouping

**What it does:** Adds three columns to the activity screen on PostgreSQL 13 and newer — `leader`,
`backend_xid` and `horizon_xacts` — so a DBA can answer, without leaving pgcenter for `psql`, which
session is holding the xmin horizon and blocking vacuum, how long it has been there, and whether
terminating it is cheap. Closes issue #148.

**Key scenarios:**
- Sort by `horizon_xacts` descending: the top row is the session holding the oldest horizon.
  Read `xact_age` beside it for how long the transaction has been open and `state` to tell a working
  session from one stuck in `idle in transaction`.
- Check `backend_xid` before terminating: blank means the transaction has written nothing and can be
  killed cheaply. This is the one place that answers it — for an `idle in transaction` session the
  `query` column shows the *last* statement, so a transaction that wrote can look read-only.
- Sort by `leader` to collapse a parallel query: the leader and all its workers share one value, so
  thirty rows read as one query rather than thirty clients. A backend that is not part of a parallel
  group shows its own PID.
- `pgcenter report -d -A` explains all three columns and the four things about them that are easy to
  read wrongly.

**Limitations:**
- The columns need PostgreSQL 13 or newer. On PG 10–12 the screen keeps its previous 14 columns with
  no message — this is the older layout, not a disabled feature.
- The horizon is shown **for backends only**. Replication slots, prepared transactions and standby
  feedback hold it back too and do not appear in `pg_stat_activity` at all, so a blank column does not
  mean nobody holds the horizon. The full aggregate across all four sources is a 0.13.0 backlog item.
- A blank cell may also mean the viewer lacks privileges to see another session's state — PostgreSQL
  returns NULL rather than an error for unprivileged roles.
- `horizon_xacts` is measured in transactions only. There is no honest conversion to wall-clock time;
  the time dimension is the neighbouring `xact_age` column.
- The same column name on the `replication` screen is computed by a different formula and the two
  numbers are not directly comparable (tech debt [023]).
- `query` now sits 38 characters further right for everyone, including users who never look at the
  horizon.

**Touches:** Every screen of the main table, through the shared sort function — sorting a column that
is blank for some rows now places those rows last in both directions instead of treating a blank as
zero. Most visible on [005-feat-replication-slots], whose default sort key is sparse.
[009-feat-horizontal-scroll] is what makes the wider screen usable. The `report` replay path gained
correct handling of a recorded version change, which matters for archives spanning a major upgrade.

---

### [015-feat-tui-papercuts] Interactive-mode Papercuts

**What it does:** Removes seven frictions that made `pgcenter top` slower to drive than it should
be. Sorting now scrolls the column window to whatever column you sorted by; an active filter is
announced permanently in the command line instead of only by a `*` on a header that may itself be
off-screen; `\` clears every filter of the current screen at once; the refresh interval is shown in
the header; the verbose panels stop wasting a screen row and highlight their values the way the
normal rows always did; and dialog prompts line up with their input field in both display modes.

**Key scenarios:**
- On a narrow terminal, press `→` past the right edge: the window follows the sort column and brings
  it into view, highlight included. Scroll away with `[`/`]` afterwards and it stays where you put
  it — the window is only moved when the sort column changes.
- Set a filter, then scroll its column off-screen or switch to another screen and back: the command
  line keeps showing `[F:datname]`, so a filtered view can no longer be mistaken for the whole
  picture. With several filters it lists them all: `[F:datname,usename]`.
- Press `\` to drop every filter on the screen at once, instead of hunting down each column that set
  one. The message says how many were removed, or says plainly that there were none.
- Press `v`: the verbose panels expand with no blank gap above the table, values are highlighted,
  and degraded `n/a` fields stay plain so a missing signal still looks different from a real number.
- Open any dialog with a filter active: the prompt and the input field stay aligned, in both compact
  and verbose mode. An overlong prompt is cut with an ellipsis rather than covered by the field.

**Limitations:**
- Pause on `Space` was planned for this batch and **split into its own feature** — delivered as
  [016-feat-pause-display]. Both blockers named here were resolved there: sorting lifts the pause
  instead of re-sorting a frozen frame, and the frame store survives the UI rebuild so the pager
  returns to the frozen frame rather than a blank screen.
- The auto-scroll fires only when the sort column changes. Entering a screen whose default sort
  column sits off-screen leaves it there until the first arrow press — reachable only on terminals
  narrower than about 80 columns.
- Filters still combine as OR, not AND. The indicator makes that visible for the first time but does
  not change it.
- Messages printed after a dialog closes remain invisible (pre-existing, registered as tech debt),
  so one of the three filter messages can only be seen in tests, not on screen.
- The verbose threshold moved from 13 terminal rows to 12 — verbose now engages one row earlier.

**Touches:** [009-feat-horizontal-scroll] — supersedes its "no auto-scroll to the sort column"
limitation and reuses its column-window function. [010-feat-overview-dashboard] — fixes two defects
in the verbose panels it introduced. The command-line composer added here is the foundation the
deferred pause feature builds on.

---

### [016-feat-pause-display] Pause the Display on `Space`

**What it does:** `Space` freezes the whole picture — the stats table, both top panels and the header
clock — so a row can be read without it scrolling away, while collection keeps running underneath.
`[PAUSED]` appears in the command line. The frozen frame is the moment `Space` was pressed, together
with the timestamp of that moment, and it holds indefinitely.

**Key scenarios:**
- Freeze a busy `activity` screen to read a long query text or copy a pid, then resume with `Space`.
- Work with the frozen frame: filter (`/`), clear filters (`\`), scroll columns (`[` / `]`) and
  change column width (`↑` / `↓`) all keep working — this is what separates it from `tmux copy-mode`.
- Resize the terminal or step into the pager or config editor and come back: the same frozen frame
  is redrawn for the new width, marker included.
- Actions that need fresh data — sorting, screen switch, `,`, `I`, `A`, `v`, `B`/`N`/`F`/`L` — resume
  the display on their own; the built-in help (`h`) lists them.
- Open the log panel (`L`) before pausing: it stops taking new lines for as long as the pause holds.

**Limitations:**
- Pressing `Space` before the first frame arrives leaves the screen empty with the marker shown —
  there is nothing to freeze yet. Returning from a pager in that state also stays empty.
- A collection error or a lost connection during the pause is not shown until the pause is lifted.
- An action that changed nothing (a key that hit its own early return, e.g. `,` on a screen without
  system tables) does not resume the display.
- The command line keeps its previous text until something writes to it again, so right after a
  terminal resize the marker line can appear clipped until the next update.
- No new screens, columns or SQL. `record`/`report` archives are unaffected.

**Touches:** [015-feat-tui-papercuts] — resolves the pause limitation it recorded and adds the second
token (`[PAUSED]`) to the command-line composer it introduced, left of the filter indicator.
[009-feat-horizontal-scroll] — its column scroll and width keys are the ones that keep working on a
frozen frame.
