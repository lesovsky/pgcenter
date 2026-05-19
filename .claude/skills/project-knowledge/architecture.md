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
