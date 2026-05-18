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

The `"procpidstat"` view (`Shift+S`) is a hybrid: it runs a 7-column `pg_stat_activity` SQL query, then enriches each row with procfs metrics from `/proc/[pid]/stat` and `/proc/[pid]/io`, producing a 17-column `PGresult`. This enrichment is local-only (procfs is not available over remote connections).

**CollectExtra mechanism:** `view.View.CollectExtra int` carries a typed constant (`CollectProcPidStat = 6`) from the `switchViewToProcPidStat` handler through `viewCh` to `Collector.Update()`. The collector checks `view.CollectExtra` directly — not via `ToggleCollectExtra` — so `top/stat.go:collectStat()` maintains a separate `prevCollectExtra` variable to call `c.Reset()` on view switches.

**IO availability probe:** `stat.CheckIOAvailable(pid int)` opens `/proc/[pid]/io` for a real PG backend PID queried from `pg_stat_activity`. `/proc/self/io` is always accessible to the owner process and is NOT a valid probe for cross-process IO access. On Linux with `ptrace_scope=1` (the default on Ubuntu/Debian), `/proc/[pid]/io` is readable only when the caller's effective UID matches the target process UID or the caller has `CAP_SYS_PTRACE`. Because pgcenter and the PG backends run under different OS users (`postgres` vs the invoking user), the probe must use an actual backend PID — a probe against the caller's own PID will always succeed and will never reveal the real permission constraint.

**procpidstat is not recordable** (`NotRecordable: true` on the view). The recorder skips it via `filterViews()` in `record/record.go`.

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
