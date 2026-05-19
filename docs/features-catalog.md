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
