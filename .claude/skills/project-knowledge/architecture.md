# pgcenter — Architecture

## Package Layout

```
cmd/
  pgcenter.go         # root cobra command
  top/, record/, report/, profile/, config/  # subcommands

internal/
  postgres/           # PG connection, query execution, test helpers
  query/              # SQL query templates per stats view (version-aware)
  stat/               # stats collection, diff computation, CPUStat etc.
  view/               # TUI view definitions (columns, sort, filters)
  align/              # column width alignment
  pretty/             # human-readable formatting (bytes, intervals)
  math/               # rate calculation utilities
  version/            # version string injected via ldflags

top/                  # top command implementation (TUI logic)
record/, report/      # record/report command implementations
profile/              # wait events profiler
config/               # config command implementation
```

## Data Flow (top command)

```
postgres connection
  → query/         version-aware SQL template → formatted query
  → stat/          collect → compute diff vs previous sample
  → view/          apply column layout, sort, filter
  → gocui TUI      render to terminal
```

## Per-process System Stats (procpidstat view)

The `"procpidstat"` view (`Shift+S`) is a hybrid: it runs a 7-column `pg_stat_activity` SQL query, then enriches each row with procfs metrics from `/proc/[pid]/stat` and `/proc/[pid]/io`, producing a **19-column** `PGresult`. This enrichment is local-only (procfs is not available over remote connections).

Columns: pid, datname, usename, state, wait_etype, wait_event (SQL), accumulated CPU (`all_total,s`, `us_total,s`, `sy_total,s`), accumulated IO (`read_total,KiB`, `write_total,KiB`), accumulated iodelay (`iodelay_total,s`), CPU rates (`%all`, `%us`, `%sy`), IO rates (`read,KiB/s`, `write,KiB/s`), iodelay rate (`%iodelay`), query (SQL).

**CollectExtra mechanism:** `view.View.CollectExtra int` carries a typed constant (`CollectProcPidStat = 6`) from the `switchViewToProcPidStat` handler through `viewCh` to `Collector.Update()`. The collector checks `view.CollectExtra` directly — not via `ToggleCollectExtra` — so `top/stat.go:collectStat()` maintains a separate `prevCollectExtra` variable to call `c.Reset()` on view switches.

**Availability probes** (both run once at `Shift+S`, stored in `view.View`):
- `stat.CheckIOAvailable(pid int)` — opens `/proc/[pid]/io` for a real PG backend PID. Must use an actual backend PID, not `/proc/self/io` (always readable by the owner, giving a false positive). Sets `v.IOAvailable`.
- `stat.CheckDelayAcctAvailable()` — reads `/proc/sys/kernel/task_delayacct` (4-byte bounded read). Returns true iff content is `"1"`. Sets `v.DelayAcctAvailable`. No PID needed — it's a kernel sysctl.

When a probe fails, the corresponding columns render as `""`. `switchViewToProcPidStat` uses a 4-branch `if/else` for `printCmdline` covering all combinations of IO × delayacct availability (mutual exclusion — exactly one call per code path).

**iodelay source:** `/proc/[pid]/stat` field 42 (`delayacct_blkio_ticks`, `suffix[39]` after stripping pid and comm). Requires `CONFIG_TASK_DELAY_ACCT=y`. `%iodelay` is NOT normalized by `runtime.NumCPU()` — it is wall-clock blocked time, not CPU utilization.

**MVC split (003-feat-procpidstat-record-report):** `buildProcPidResult` (exported: `BuildProcPidResult`) is composed of two private functions: `buildProcPidResultRaw` (assembles 19-col PGresult with raw float strings in cols 6–11: jiffies for CPU, bytes for IO, ticks for iodelay) and `formatProcPidResultForDisplay` (converts cols 6–11 to display strings: `HH:MM:SS`, KiB). The TUI uses `BuildProcPidResult` directly; the recorder stores the display-ready result. Also exported: `ReadProcPidStat`, `ReadProcPidIO`, `GetSysticksLocal` (calls `getconf CLK_TCK`), `SysInfo{Ticks float64, CPUCount int}`.

**procpidstat in record/report (003-feat-procpidstat-record-report):** The recorder enriches procpidstat per-tick using the same map-rotation protocol as `Collector.Update()`. `tarRecorder` is stateful: it holds `prevProcPidStats`, `currProcPidStats`, `prevProcPidIO`, `currProcPidIO` maps and `lastCollect` timestamp across ticks. Each tick produces one `procpidstat.TIMESTAMP.json` (19-col display PGresult) and one `sysinfo.TIMESTAMP.json` (`SysInfo` JSON). Local/remote gate lives in `record.app.setup()` via `db.Local` (`isLocalhost()` check in `postgres.Connect`): if remote, `procpidstat` is removed from views before recording and an INFO message is printed. Report uses `DiffIntvl=[0,0]` (pass-through, same pattern as `activity` view) — rates are pre-computed by the recorder. `report -N` (`--proc-stats`) flag activates procpidstat report; `-d -N` shows describe.

## PostgreSQL Version Handling

Stats views change between PG versions. Version detection at connect time via `SELECT version()`.

Version-specific query selectors in `internal/query/`:
- `SelectStatActivityQuery(version)` — branches at PG 9.6, PG 10
- `SelectStatReplicationQuery(version, track)` — branches at PG 10
- `SelectStatDatabaseGeneralQuery(version)` — branches at PG 12
- `SelectStatStatementsTimingQuery(version)` — branches at PG 13, PG 17
- `SelectStatWALQuery(version)` — branches at PG 18 (columns removed)
- `SelectStatBgwriterQuery(version)` — branches at PG 17 (`pg_stat_checkpointer` split off `pg_stat_bgwriter`) and PG 18 (`slru_written` added). Returns `(query, Ncols, DiffIntvl)` — DiffIntvl also differs per version.
- `SelectStatReplicationSlotsQuery(_ int)` — version-independent on PG 14–18 (chosen column subset is stable), returns `(query, 15, [2]int{6,13})`; the `version` param is kept for selector-signature symmetry. Single hybrid `pg_replication_slots LEFT JOIN pg_stat_replication_slots` query.
- `SelectStatIOQuery(version)` — branches at PG 18 (`op_bytes` removed → native `read_bytes`/`write_bytes`/`extend_bytes`; `object='wal'` rows added), returns `(query, 16, [2]int{4,14})`. `SelectStatIOTimeQuery(_ int)` is version-independent (timing columns are identical PG 16–18), returns `(query, 10, [2]int{4,8})`. `internal/query/query.go` gained `PostgresV15/16/17/18` constants for these.

The `bgwriter` view (hotkey `b`, `internal/query/bgwriter.go`) is a single-row version-aware screen modeled on `pg_stat_wal`. It was the project's first view registered with `NotRecordable: true` — TUI-only, excluded from `pgcenter record`/`report`.

The `replslots` view (hotkey `o`, `internal/query/replication_slots.go`) is a multi-row screen modeled on `replication`/`tables`, the second `NotRecordable` user. It hybrid-joins `pg_replication_slots` (state: slot_type, active, wal_status, retained WAL via the recovery-aware `WalFunction` template, safe_wal_size) with `pg_stat_replication_slots` (logical-decoding spill/stream/total counters). Physical slots are absent from the stat view, so the 8 diffed counters are `coalesce(...,0)` — without it a physical slot's empty-string NULLs reach `diffPair`/`ParseInt` and abort the sample (`internal/stat/postgres.go`). State columns are absolute (outside `DiffIntvl=[6,13]`), counters diffed; `OrderKey=4` (retained,KiB desc) — the one multi-row view that deviates from the col-0 sort default.

The `pg_stat_io` screen (hotkey `j`/`J`, `internal/query/io.go`) is split into **two registered views** — `stat_io` (count) and `stat_io_time` (time) — because pgcenter has no horizontal column scroll and the full counter set does not fit one screen (same reason `pg_stat_statements` is split into sub-screens). `j` toggles between them via `statioNextView` (`top/config_view.go`), `J` opens `menuStatIO` (`top/menu.go`). The third and fourth `NotRecordable` views. Row identity is a composite (`backend_type × object × context`), but `view.UniqueKey` is a single column index, so the query emits a synthetic `left(md5(backend_type||object||context),10) AS io_key` as column 0 and points `UniqueKey` at it — the same trick `statements_io` uses for its `queryid`. There is no way to hide that key column (the `internal/align` package floors every column at width 8 and `ColsWidth` is a runtime cache, not a preset), so `io_key` is displayed like the pgss `queryid`.

View configuration happens in `internal/view/view.go: Configure(opts)` which calls these selectors and updates `QueryTmpl` and `Ncols` per view at connection time.

## PostgreSQL Driver

pgx/v5 (`github.com/jackc/pgx/v5`). Connection uses `QueryExecModeSimpleProtocol` for PgBouncer compatibility. Error types from `pgx/v5/pgconn`.

## Remote Monitoring Mode

When pgcenter runs against a remote PG, system stats (CPU, disk, network) are read via PL/Perl functions in the `pgcenter` schema — installed by `testing/fixtures.sql`. Requires `plperlu` + CPAN modules (`Linux::Ethtool::Settings`, `Filesys::Df`).

## Testing

Integration tests require a running PostgreSQL instance.
Test helpers in `internal/postgres/testing.go`:
- `NewTestConnect()` — connects to PG 17 (port 21917, default)
- `NewTestConnectVersion(version)` — connects to specific version; returns error for unavailable versions (callers use `t.Skipf`)

Port map: PG14=21914, PG15=21915, PG16=21916, PG17=21917, PG18=21918.
EOL entries (PG 9.5–13) kept in map but connections will fail gracefully.

Run with: `make test` (race detector, timeout 300s).
