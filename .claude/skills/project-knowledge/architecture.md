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

The `bgwriter` view (hotkey `b`, `internal/query/bgwriter.go`) is a single-row version-aware screen modeled on `pg_stat_wal`. Introduced as TUI-only in 0.11.0, it is now recordable: feature 008-feat-record-report-0-11-views cleared its `NotRecordable` flag and added `report -B` (replayed version-aware by the recording's PG version).

The `replslots` view (hotkey `o`, `internal/query/replication_slots.go`) is a multi-row screen modeled on `replication`/`tables`; like `bgwriter` it was TUI-only in 0.11.0 and became recordable via feature 008 (`report -L`). It hybrid-joins `pg_replication_slots` (state: slot_type, active, wal_status, retained WAL via the recovery-aware `WalFunction` template, safe_wal_size) with `pg_stat_replication_slots` (logical-decoding spill/stream/total counters). Physical slots are absent from the stat view, so the 8 diffed counters are `coalesce(...,0)` — without it a physical slot's empty-string NULLs reach `diffPair`/`ParseInt` and abort the sample (`internal/stat/postgres.go`). State columns are absolute (outside `DiffIntvl=[6,13]`), counters diffed; `OrderKey=4` (retained,KiB desc) — the one multi-row view that deviates from the col-0 sort default.

The `pg_stat_io` screen (hotkey `j`/`J`, `internal/query/io.go`) is split into **two registered views** — `stat_io` (count) and `stat_io_time` (time) — for logical grouping of related counters (count vs latency), the same idiom `pg_stat_statements` uses for its sub-screens. `j` toggles between them via `statioNextView` (`top/config_view.go`), `J` opens `menuStatIO` (`top/menu.go`). Both were TUI-only in 0.11.0 and became recordable via feature 008 (`report -J c` → `stat_io`, `report -J t` → `stat_io_time`). Row identity is a composite (`backend_type × object × context`), but `view.UniqueKey` is a single column index, so the query emits a synthetic `left(md5(backend_type||object||context),10) AS io_key` as column 0 and points `UniqueKey` at it — the same trick `statements_io` uses for its `queryid`; `io_key` is displayed like the pgss `queryid`.

> **Note (009-feat-horizontal-scroll):** the main stats table now *has* horizontal column scroll (see "Horizontal Column Scroll" below), so the historical "no horizontal scroll" framing in the [006-feat-pg-stat-io] / [007] ADRs no longer holds as a constraint. The two-screen `pg_stat_io` split, the seven `pg_stat_statements` sub-screens, and the synthetic `io_key` are kept deliberately — they are a product decision (logical grouping and isolation of related data), not a workaround for a missing feature. Scroll exists for narrow terminals; it is not meant to collapse the sub-screens into one wide view.

## Horizontal Column Scroll (009-feat-horizontal-scroll)

The main stats table (the `dbstat` area, shared by every stat screen) scrolls horizontally by column. Hotkeys `]` (right) and `[` (left) move a sliding window over the columns; the first column is **frozen** (always rendered) so the row identifier — PID / database / table name — never scrolls off. Closes issue #14 (open since 2015). Scope is the main table only; side extra-panels (iostat/netdev/fsstats/logtail) and the record/report pipeline are untouched.

- **`config.scrollOffset int`** (`top/config.go`) — ephemeral scroll position, an index into the *scrollable* columns (`1..Ncols-1`). It lives on `top.config`, **not** on `view.View`: `viewSwitchHandler` deliberately persists per-view state (`OrderKey`, `Filters`, `ColsWidth`), but scroll must reset on every screen switch, so it is reset to 0 in both `viewSwitchHandler` and `switchViewToProcPidStat` (the second switch path that bypasses the handler). It survives auto-refresh ticks within a screen.
- **`visibleColumns` (pure function, `top/stat.go`)** — the single source of truth for the window. Given column count, per-column widths (`view.ColsWidth`), terminal width, and the current offset, it returns a `columnWindow` (visible scrollable range), the **clamped** offset, and `hiddenLeft`/`hiddenRight` flags. It re-clamps the offset on every call against the current `Ncols`/width, so a stale offset after a dataset shrinks (fewer columns on a refresh) self-corrects without handler involvement. Reads `ColsWidth` strictly over `[0,Ncols)` to avoid the map-zero-key pitfall (issue #99 class).
- **Partial last column.** A scrollable column enters the window when its *start* fits the budget; the last column may be shown partially (truncated at the edge). Without this, a deliberately wide trailing column (e.g. `query` on activity/statements) would vanish entirely once it could not fit whole. `hiddenRight` is false when nothing follows the partially-shown column. The marker-width reservation is resolved in a two-pass walk (forward and backward) so the last column stays reachable at `maxOffset` and `›` turns off there.
- **Windowed render.** `printStatHeader`/`printStatData` (`top/stat.go`) render column 0 (frozen) then iterate only the columns inside the window, indexing values by **absolute** column index. The header bolds the frozen column name (sort-column highlight takes priority on column 0) and draws `‹`/`›` edge markers near the edges when columns are hidden in that direction; the marker cells are budgeted into the width so header and data rows stay aligned. The print functions take an `io.Writer` and the precomputed `columnWindow` (instead of reading `v.Size()` internally) so they are unit-testable without a live gocui terminal; the window is computed once in `renderDbstat` and the clamped offset is written back to `config.scrollOffset`. Terminal width comes from `v.Size()`, which returns the true drawing width because the `dbstat` view is created with `Frame=false`.
- **Hotkeys** (`top/config_view.go`, `top/keybindings.go`): `scrollLeft`/`scrollRight` adjust `config.scrollOffset` and re-send `config.view` on `viewCh` only to retrigger an immediate re-render (the view itself is not mutated — unlike sort, which mutates `view.OrderKey`). Existing arrows (sort column / width), `<`, `/` are unchanged. Documented on the help screen (`top/help.go`).

## Verbose Top-Panel Mode (010-feat-overview-dashboard)

The `v` hotkey expands the two always-on summary panels — `sysstat` (left, +3 rows: iostat /
nicstat / filesyst) and `pgstat` (right, +5 rows: workload / databases / workers / replication /
bgwr-ckpt) — into an extended instance-health overview. It is a display **mode**, not a registered
view: it reuses the free-form `printSysstat`/`printPgstat` render path, registers no `view.View`, and
therefore has **zero view-count test impact** (no `TestNew`/`Test_filterViews` churn). It is gated by a
dedicated `view.View.Verbose bool` (rides `viewCh` to the collector) mirrored into `top.config.verbose`
(read by the renderer/layout) — see the patterns.md "Verbose display-mode toggle" pattern and ADR
[010] for why this is a separate boolean rather than an overloaded `CollectExtra`.

- **Pure geometry — `topBandLayout` (`top/layout.go:33`).** `topBandLayout(verbose, maxY)` returns the
  band/cmdline/dbstat y-coordinates plus an `expanded` flag; `layout()` (`top/ui.go`) feeds the result
  into the four `SetView` calls. Compact (and the height-guard fallback) reproduces the historical
  literals (`4/4/3/5/4`) byte-identically; verbose grows the panels asymmetrically (`sysstat` +3,
  `pgstat` +5) and shifts `cmdline`/`dbstat` down. The height-guard refuses to expand when the band +
  cmdline + table header + ≥1 data row would not fit (threshold `maxY ≥ 13`), falling back to compact +
  a one-shot cmdline hint. Pure-function, gocui-free, table-tested — the [009] `visibleColumns` precedent.
- **All-three system collection branch (`internal/stat/stat.go:262`, `:401`).** When `view.Verbose` is
  set, `Collector.Update` runs a verbose-gated branch placed **after** the existing mutually-exclusive
  `switch c.config.collectExtra` (`stat.go:236`) that collects disk+net+fs **every tick** via the same
  `collectDiskstats`/`collectNetdevs`/`collectFsstats` readers the side panels use (same `%util` math →
  consistency). Each source is `== nil` guarded so a source already populated by an active side panel is
  not re-collected; a per-source error leaves that source `nil` (rendered `n/a`) without aborting the
  sample. The existing switch is untouched, so the side panels are unaffected.
- **`verboseCollectState` sub-struct (`internal/stat/stat.go:83`, field at `:138`).** A named sub-struct
  on `Collector` groups all verbose-specific collection state so it does not leak across the shared
  `Collector`. It carries the first-tick flag + re-arm fields (`verboseFirstTick = !prevVerboseActive` on
  every OFF→ON, so a first tick or a re-enable without a view change emits `n/a` for deltas) — bridged to
  the renderer via the public `System.VerboseFirstTick` (collector and renderer talk only through
  `stat.Stat` on `statCh`) — and the per-source latency guard for the one dear no-twin aggregate (DB
  sizes/growth): `dbSizeThrottled(threshold, budget, sinceLastRun)` (a pure function) throttles it to a
  cached **stale** value (not `n/a`) when its last query exceeded `latencyGuardThreshold(refresh) =
  max(refresh/4, 500ms)`, auto-resuming after a one-refresh cadence budget. System rows are never
  throttled (consistency with the full panels). The re-arm does **not** rely on `c.Reset()` —
  `toggleVerbose` never calls it (ADR [010]); `Reset()` clears the sub-struct in lockstep with prev/curr.
- **Aggregate queries (`internal/query/overview.go`).** New flat single-row aggregates for the pgstat
  verbose rows: `OverviewWorkload`/`OverviewDatabases`/`OverviewDatabasesSize`/`OverviewWorkers`/
  WAL-size/send-recv (static SQL) and recovery-aware `{{.WalFunction1/2}}` templates for
  replication-lag/slots; the archiving backlog reuses the `count(.ready) × wal_segment_size` idiom from
  `wal.go`; bgwr/ckpt reuses `SelectStatBgwriterQuery(version)` (no new SQL). `collectOverviewStat`
  (`internal/stat/postgres.go`) scans each aggregate as its own `QueryRow` (a privilege/feature failure
  degrades one field to `n/a` via `*Valid`/`sql.NullInt64` sentinels, never the raw PG error text), with
  Go-side rates vs a prev snapshot (tps = `(Δcommit+Δrollback)/itv`; `others` = interval delta, no `/s`;
  cache hit = per-interval `Δhit/Δ(hit+read)`). Four GUCs (`max_worker_processes`,
  `max_logical_replication_workers`, `max_parallel_workers`, `wal_segment_size`) and `data_directory`
  were added to `SelectCommonProperties` + `PostgresProperties` + the `GetPostgresProperties` `.Scan(...)`
  in lockstep.

View configuration happens in `internal/view/view.go: Configure(opts)` which calls these selectors and updates `QueryTmpl` and `Ncols` per view at connection time.

The `view.View.NotRecordable` field and the `record/record.go:filterViews()` branch that honors it still exist, but no production view sets it anymore (feature 008 cleared the last of them — the five 0.11.0 screens, plus a stale flag on `procpidstat`). The mechanism is kept solely for the synthetic drop-branch test (`record.TestFilterViews_dropsExplicitNotRecordable`).

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
