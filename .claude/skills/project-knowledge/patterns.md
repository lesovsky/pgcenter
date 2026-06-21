# pgcenter — Code Patterns

## Adding a New PostgreSQL Version

1. Add port to `internal/postgres/testing.go` ports map
2. Add version to all `versions := []int{...}` lists in `internal/query/*_test.go`
3. Run tests — `t.Skipf` handles unavailable versions gracefully
4. If a stats view changed: add a new query constant and selector function in `internal/query/`
5. Wire selector into `internal/view/view.go: Configure()` if Ncols also changes
6. Update pgcenter-testing Docker image (see deployment.md)

## Version-Specific Query Pattern

When a PG version changes columns in a stats view:
- Add `PgStatXxxPGNN` constant in the relevant `internal/query/*.go` file
- Add `SelectStatXxxQuery(version int) (string, int)` returning template + ncols
- Call it in `view.Configure()` under the correct view name
- Add version-specific test cases in `*_test.go`

Reference implementations of the single-row version-aware view: `internal/query/wal.go` and `internal/query/bgwriter.go`. The bgwriter screen is notable for placing absolute event-counter columns (`ckpt_*`, `rstpt_*`) **outside** the contiguous `DiffIntvl` range so they render cumulative, while the work/time/buffer columns inside the range render as per-interval deltas.

For a **multi-row hybrid view** that LEFT JOINs two stats views, see `internal/query/replication_slots.go` (the `replslots` screen). Two patterns it establishes:
- **`coalesce(...,0)` on diffed columns fed by a LEFT JOIN.** A row present in both samples enters `diff()`/`diffPair()`; if an outer-joined diffed column is SQL NULL it scans as an empty string and `strconv.ParseInt("")` aborts the whole sample. Coalescing NULL→0 in SQL keeps such rows diff-safe (physical slots, absent from `pg_stat_replication_slots`, render `0`). Only diffed columns need this — absolute columns outside `DiffIntvl` pass NULLs through as empty.
- **Recovery-aware WAL distance for free** via the `{{.WalFunction1}}({{.WalFunction2}}(), lsn)` template (`selectWalFunctions` in `query.go` picks `pg_current_wal_lsn` on a primary, `pg_last_wal_receive_lsn` on a standby) — no recovery branch in the query.
A multi-row view sets `UniqueKey` to the stable row identity (slot_name, col 0) for cross-sample row matching, and may set a non-default `OrderKey` (replslots: 4 = retained,KiB desc) for a domain-appropriate default sort.

When the row identity is **composite** (more than one column), emit a synthetic key column and point `UniqueKey` at it — `internal/query/io.go` (the `pg_stat_io` screen) does `left(md5(backend_type||object||context),10) AS io_key` at column 0, following `statements_io`'s `queryid`. Column hiding is not available (`internal/align` floors width at 8), so the key column is shown, not hidden. `io.go` is also the reference for splitting one wide stats view into two registered sub-views (`stat_io` count / `stat_io_time` time) navigated by a lowercase toggle (`statioNextView`) plus an uppercase menu (`menuStatIO`) — the pattern to copy when a view has too many columns for one screen.

## Adding a New View — test counts that must be updated

Registering a view in `view.New()` couples to two count-based tests that fail in CI (not always locally) if missed:
- `internal/view/view_test.go: TestNew` pins the total view count (and `TestView_VersionOK` pins per-version availability).
- `record/record_test.go: Test_filterViews` pins, per version, how many views `filterViews` drops vs keeps. A `NotRecordable: true` view is always dropped, so every `wantN` row increases by the number of new `NotRecordable` views (feature 006 added 2 → `+2` each row; `wantV` unchanged). This test runs without Postgres, so a stale count is a real failure even though the rest of the `record` package skips/fails on a missing PG fixture — do not assume a red `record` package is only the connection-refused tests.

## Error Wrapping

Use `fmt.Errorf("context: %w", err)` for all error wrapping in production code.
Use `errors.Is(err, target)` for error comparison (not `==`).
Exception: `printCmdline()` and `fmt.Sprintf()` use `%s` (not error wrapping functions).

## Sorting

Use `sort.SliceStable` (not `sort.Slice`) in `internal/stat/postgres.go` to ensure deterministic ordering of rows with equal sort keys across Go versions.

## Manual Testing / QA Phase

Always run `make build` as the first step of any manual TUI verification, even if a previous
build completed earlier in the same session. Cherry-picks, rebases, and mid-session code
changes do not automatically update `./bin/pgcenter`. A stale binary silently invalidates every
visual check that follows. The rule: one manual verification session = one fresh build at the
start.

## printCmdline() — Mutual Exclusion

`printCmdline(g, msg)` calls `g.Update` followed by `v.Clear`. If it is called twice in the
same view-switch handler the second call immediately overwrites the first render. When a
handler needs to show either a warning or a normal message, these two cases must be mutually
exclusive — use an `if/else` branch, not two sequential calls. Calling `printCmdline(warning)`
and then `printCmdline(v.Msg)` in the same code path will always discard the warning before
the user can read it.

When multiple independent availability probes can fail (e.g., IO + delay accounting in
`switchViewToProcPidStat`), use a 4-branch `switch` covering all combinations, with a combined
message for the case where both are unavailable — still exactly one `printCmdline` call per path.

## Adding a Hybrid View (SQL + procfs enrichment)

When a view combines SQL and local system data (e.g., procpidstat = pg_stat_activity + /proc):

1. Define a `CollectExtra` constant in `internal/stat/stat.go` iota block. The iota is offset by 1 (`pgProcUptimeQuery` string constant precedes the group): existing values `CollectNone=1, ..., CollectLogtail=5`; next is 6.
2. Register the view in `view.New()` with `NotRecordable: true`, `DiffIntvl: [2]int{0,0}`, `Filters: map[int]*regexp.Regexp{}`. Leave `CollectExtra`/`IOAvailable` at zero — set at runtime by the switch handler.
3. The switch handler (`top/config_view.go`) must save/load/patch/send the view manually — NOT via `viewSwitchHandler`, which reloads from the static map and discards runtime patches.
4. In `Collector.Update()`, add a `view.CollectExtra == CollectXxx` branch after `collectPostgresStat` to enrich and replace the SQL result.
5. In `top/stat.go:collectStat()`, add `prevCollectExtra` change-detection alongside `ShowExtra` to call `c.Reset()` on view switches.
6. If the view should NOT be recordable: set `NotRecordable: true` in view definition; `filterViews()` skips it automatically.
   If the view SHOULD be recordable with procfs enrichment: leave `NotRecordable` at default `false` and follow the tarRecorder stateful pattern (step 7 below).
7. Reference implementation: `internal/stat/procpidstat.go`, `top/config_view.go:switchViewToProcPidStat`.

## Recording a Hybrid View (SQL + procfs, with pgcenter record)

When a hybrid view needs record/report support (reference: 003-feat-procpidstat-record-report):

1. Leave `NotRecordable: false` (default) on the view. Add local/remote gate in `record.app.setup()`: if `!db.Local`, delete the view from `views` and print INFO — procfs is not available over remote connections.
2. Add `isLocal`, `ticks`, `cpuCount`, availability flags, and `prev`/`curr` procfs maps to `tarRecorder` struct. Initialize in `app.setup()` via `GetSysticksLocal()`, `runtime.NumCPU()`, and `stat.CheckIOAvailable()` / `stat.CheckDelayAcctAvailable()` probes.
3. In `tarRecorder.collect()`, add an enrichment branch **after** the main views loop. Mirror the map-rotation protocol from `Collector.Update()`: build `newPrev` from current map filtered to PIDs in the SQL result, rotate maps, then read procfs for each PID (`stat.ReadProcPidStat`, `stat.ReadProcPidIO`). Compute `itv` via `time.Since(lastCollect)`.
4. In `tarRecorder.write()`, hoist `now := time.Now()` to the function top so all entries share the same timestamp. Append a `sysinfo.TIMESTAMP.json` entry (`stat.SysInfo{Ticks, CPUCount}`) for each tick — needed by the report pipeline to document recording environment.
5. In `report/report.go`: extend `isFilenameOK` to accept the new entry prefix; handle the entry in `readTar`; extend `metadata` struct if report-side metadata is needed. Use `DiffIntvl=[0,0]` if rates are pre-computed by the recorder (same as `activity` pattern).
6. Detect "no data" in `processData` via `anyDataPrinted bool` (not `linesPrinted` — initialized to `repeatHeaderAfter = 20`). Detect unavailable columns via empty-string sentinel on first result.

## Git Workflow

- Work in `develop`, open PRs to `master` with squash merge
- After squash: `git reset --hard master && git push --force-with-lease` to sync develop
- Release: tag on master → push to `release` branch → triggers release workflow

## Linting

`.golangci.yml` enables: errcheck, gocritic, gosimple, govet, ineffassign, revive, staticcheck, unused.
Run locally: `make lint` (golangci-lint + gosec) and `make vuln` (govulncheck).
Known suppressions: `// #nosec G204,G702` on `exec.Command` calls (pager/editor from env vars).

## Naming Conventions

Go acronyms: `CPUStat` not `CpuStat`, `PGresult` not `PgResult`.
Unused function parameters in callbacks: rename to `_`.
