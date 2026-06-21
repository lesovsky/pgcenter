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
- TUI-only in 0.11.0 — the view is `NotRecordable`, so `pgcenter record` skips it and it does not appear in `pgcenter report`. Record/report support is deferred to a backlog feature.
- `buf_backend` / `buf_backend_fsync` columns appear only on PG ≤ 16 (the data moved to `pg_stat_io` on PG 17+).
- The column set differs per server version (PG 18 adds a `slru_written` column); no NULL placeholders for columns absent on a given version.
- `stats_age` on PG 17+ comes from `pg_stat_checkpointer`; a separate `pg_stat_bgwriter` reset is not reflected.

**Touches:** Shares the single-row version-aware view model with the `pg_stat_wal` screen.

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
- TUI-only in 0.11.0 — the view is `NotRecordable`, so `pgcenter record` skips it and it does not appear in `pgcenter report`. This hurts retrospective analysis most for this feature (disk-fill incidents are often forensic); record/report is the planned next phase.
- Physical slots show `0` (not blank) in the spill/stream/total columns — those metrics are logical-decoding-only; the adjacent `slot_type=physical` disambiguates.
- Invalidation **cause** is not shown (`conflicting`/`invalidation_reason` omitted to keep one query across PG 14–18); `wal_status` conveys the state (`lost`/`unreserved`).
- `pg_stat_subscription_stats` (subscriber-side) is out of scope — a separate future feature.

**Touches:** Shares the multi-row sort/filter/diff view model with the `pg_stat_replication` and `tables` screens; second view (after [004-feat-bgwriter-checkpointer]) registered `NotRecordable`.
