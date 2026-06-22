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
