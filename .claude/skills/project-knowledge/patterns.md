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

When versions differ by **column count** (not just names), the selector must also carry the
layout: return `(string, int, [2]int)` (`+DiffIntvl`, see `io.go`) or `(string, int, [2]int, int)`
(`+UniqueKey`, see the `statements_jit` selector in `internal/query/statements.go` — UniqueKey
points at the trailing md5 `queryid`, whose index shifts with ncols) and patch all returned
fields in `Configure()`. When only names change but the count is constant (e.g. `statements_timings`),
`Configure()` swaps `QueryTmpl` alone and the static `Ncols`/`DiffIntvl`/`UniqueKey` stay valid.

Reference implementations of the single-row version-aware view: `internal/query/wal.go` and `internal/query/bgwriter.go`. The bgwriter screen is notable for placing absolute event-counter columns (`ckpt_*`, `rstpt_*`) **outside** the contiguous `DiffIntvl` range so they render cumulative, while the work/time/buffer columns inside the range render as per-interval deltas.

For a **multi-row hybrid view** that LEFT JOINs two stats views, see `internal/query/replication_slots.go` (the `replslots` screen). Two patterns it establishes:
- **`coalesce(...,0)` on diffed columns fed by a LEFT JOIN.** A row present in both samples enters `diff()`/`diffPair()`; if an outer-joined diffed column is SQL NULL it scans as an empty string and `strconv.ParseInt("")` aborts the whole sample. Coalescing NULL→0 in SQL keeps such rows diff-safe (physical slots, absent from `pg_stat_replication_slots`, render `0`). Only diffed columns need this — absolute columns outside `DiffIntvl` pass NULLs through as empty.
- **Recovery-aware WAL distance for free** via the `{{.WalFunction1}}({{.WalFunction2}}(), lsn)` template (`selectWalFunctions` in `query.go` picks `pg_current_wal_lsn` on a primary, `pg_last_wal_receive_lsn` on a standby) — no recovery branch in the query.
A multi-row view sets `UniqueKey` to the stable row identity (slot_name, col 0) for cross-sample row matching, and may set a non-default `OrderKey` (replslots: 4 = retained,KiB desc) for a domain-appropriate default sort.

When the row identity is **composite** (more than one column), emit a synthetic key column and point `UniqueKey` at it — `internal/query/io.go` (the `pg_stat_io` screen) does `left(md5(backend_type||object||context),10) AS io_key` at column 0, following `statements_io`'s `queryid`. Column hiding is still not available (`internal/align` floors width at 8), so the key column is shown, not hidden. `io.go` is also the reference for splitting one wide stats view into two registered sub-views (`stat_io` count / `stat_io_time` time) navigated by a lowercase toggle (`statioNextView`) plus an uppercase menu (`menuStatIO`) — the pattern to copy when a view's columns are better presented as logically grouped screens. (Since 009-feat-horizontal-scroll the main table also scrolls horizontally; the sub-screen split is kept as a deliberate grouping choice, not because columns would otherwise be unreachable — see `architecture.md`.)

## Testable TUI Rendering — pure window function + io.Writer printers (009-feat-horizontal-scroll)

`gocui.View` cannot be constructed in a unit test, which historically left the `dbstat` print path untested. Two patterns make TUI rendering testable without a live terminal — copy them for any non-trivial render logic:

- **Pure layout function as the single source of truth.** Extract the only non-trivial arithmetic (the visible-column window) into a pure function `visibleColumns(...)` (`top/stat.go`) that takes plain inputs (counts, widths, terminal width, offset) and returns a value (`columnWindow` + clamped offset + flags). It is exhaustively unit-tested, including a property test that walks the parameter space (`ncols × widths × termWidth`) to prove an invariant — here, "the last column is reachable at `maxOffset`". The function re-clamps on every call, so render and key handlers never hold an authoritative copy of derived state.
- **Write back the clamped value at render time.** The render path (`renderDbstat`) calls the pure function once, renders from its result, and writes the clamped offset back to `config.scrollOffset`. Key handlers only nudge the raw offset (and guard against int overflow); the upper bound is enforced solely by the render-time clamp. This keeps the "what fits" logic in one place rather than duplicated in handlers.
- **Printers take `io.Writer`, not `gocui.View`.** `printStatHeader`/`printStatData` accept an `io.Writer` and the precomputed `columnWindow` instead of reading `v.Size()` internally, so tests assert rendered output against a `bytes.Buffer`. The width-and-window decision is hoisted to the caller, which is the only piece that needs the live view.

**Caveat learned in manual QA:** a window function that admits a scrollable column only when it fits *whole* silently drops a deliberately wide trailing column (e.g. `query`). Allow the last column to render *partially* (start-in-budget), and reserve marker-glyph width in **both** the forward and backward walk, or the last column becomes unreachable at the right edge. This class of bug is invisible to unit tests written against the original (whole-column) semantics — a litmus test that fails on the wrong semantics is the guard.

## Verbose display-mode toggle (010-feat-overview-dashboard)

When adding an on/off *display mode* that layers extra rows over the current screen (not a new screen),
mirror the verbose top-panel mode rather than registering a view:

- **Dual-home the flag like `showExtra`.** A mode that both the collector and the renderer must see needs
  two homes: `view.View.Verbose bool` (rides `viewCh` to `Collector.Update`) and `top.config.verbose bool`
  (read by the renderer/layout in the gocui handler goroutine). The toggle handler (`top/verbose.go:toggleVerbose`)
  writes the flag into **every** view in `config.views` (the `showExtra` write-into-all-views idiom) so the
  mode **persists across screen switches** — persistence is free because `viewSwitchHandler` simply never
  zeroes it (unlike `scrollOffset`, which it deliberately resets). Prefer a dedicated boolean over
  overloading `CollectExtra` (a mutually-exclusive `int` whose toggle path fires `Reset()` — see ADR [010]).
- **Skip the `collectStat()` Reset on a mode-only toggle.** `collectStat()` calls `c.Reset()` on the
  `viewCh` push, which blanks the `prev*` snapshots (one frame of empty CPU/mem/load deltas). Add an early
  `if prevVerbose != v.Verbose { … continue }` branch (mirroring the existing `ShowExtra` branch, placed
  **before** both Reset paths) so toggling the mode does not wipe the snapshots.

## Panel/screen consistency — reuse the struct math (010-feat-overview-dashboard)

When a summary row must show the *same* number a detail panel/screen shows, read the **same struct** the
full panel renders and replicate any print-time conversion, do not recompute from scratch:

- The verbose iostat/nicstat rows select the max-`%util` device and read `Util`/`Utilization` AS-IS from
  the existing `count*Usage` structs (the full `B`/`N`/`F` panels' math), filtering active devices the same
  way `printIostat`/`printNetdev` do. nicstat's rMbps/wMbps is **computed at print time** in `printNetdev`
  (`Rbytes/1024/128`), so the verbose row replicates that exact conversion — recomputing independently is a
  divergence bug.
- The verbose filesyst `use%` uses panel parity (`fs.Pused` via `%3.0f`, **not** `Ceil`) so it matches the
  full fsstat panel (a `Ceil` would read 75% where the panel reads 74%). The ceil rule applies only to rate
  fields, not to percentages already computed by the struct.

## Reserved-width `n/a` for static trailing labels (010-feat-overview-dashboard)

A degraded field that renders `n/a` (3 chars) where a value (e.g. ` 99.99%`, 7 chars) would otherwise sit
makes the **trailing label jump horizontally** as the signal appears/disappears. Reserve the `n/a` to the
value's reserved width so it is a drop-in: `naReserve(width)` = `fmt.Sprintf("%*s", …)` right-aligned (the
mirror of `pretty.ReserveWidth`'s `%*d`), with a `len("n/a")` min-width guard. Apply it only to
**fixed-width** fields (cache-hit ratio `%6.2f%%`, the `%d` workload rates). Variable-width `pretty.Size`
fields are made drop-in the same way via `pretty.SizeWidth(v, width)` (see the rate-formatter section) —
since 011-refactor-tech-debt-paydown the verbose Size fields use it under a single `sizeFieldWidth = 8`
const with `naReserve(sizeFieldWidth)` fallbacks, so value and `n/a` share the reserve. A row's first
field (e.g. `wal size`) pushes no trailing label and stays a bare `Size`.

## Dynamic unit-suffix rate formatter (010-feat-overview-dashboard)

For a fixed-digit-budget rate column that must not break layout on top-end hardware (NVMe arrays >9.7 GB/s,
25/40/100GbE), `internal/pretty` has three net-new pure formatters: `Ceil` (round up via `math.Ceil` —
`internal/math` had no ceil), `ReserveWidth` (`%*d` fixed width, never truncates), and `RateUnit` (promotes
the unit one step on reserved-digit overflow — MB/s→GB/s with binary 1024, Mbps→Gbps with **decimal 1000**
per network convention). Pure functions → property/table tests at the overflow boundary.

Since 011-refactor-tech-debt-paydown the overflow/divisor/ceil computation lives in one unexported core
`rateUnitParts(v, family, width) (field, unit)`; `RateUnit` (no separator, `9999MB/s`) and
`RateUnitPrefixed(v, family, prefix, width)` (a `" "+r/w` marker between digits and unit, `1135 rMB/s`,
used by the verbose disk/net rows) both delegate to it — add a new assembly form there, never a second
copy of the overflow logic. Also added: `pretty.SizeWidth(v, width)` = `fmt.Sprintf("%*s", width, Size(v))`,
the fixed-width drop-in for `Size` (right-align, never truncate, digits/units unchanged) — `Size` itself
stays variable-width for its other callers.

## Adding a New View — test counts that must be updated

Registering a view in `view.New()` couples to count-based tests that fail in CI (not always locally) if missed:
- `internal/view/view_test.go: TestNew` pins the total view count. `TestView_VersionOK` pins per-version availability — its row at a version **≥ the new view's `MinRequiredVersion`** also increases by one (feature 007's PG15+ view bumped only the `160000` row, not the `≤140000` rows).
- `record/record_test.go: Test_filterViews` pins, per version, how many views `filterViews` drops vs keeps. A `NotRecordable: true` view is always dropped, so every `wantN` row increases by the number of new `NotRecordable` views (feature 006 added 2 → `+2` each row; feature 007 added 1 → `+1`; `wantV` unchanged). This test runs without Postgres, so a stale count is a real failure even though the rest of the `record` package skips/fails on a missing PG fixture — do not assume a red `record` package is only the connection-refused tests.

Adding a `pg_stat_statements` **sub-screen** (or any `menuPgss`/cycle entry) additionally breaks `top` tests — `Test_selectMenuStyle` (pins each menu's item count), `Test_statementsNextView`, and `Test_switchViewTo` (pin the `x`-cycle transitions). These `top` tests DO run locally without Postgres, so they catch the miss in `make test` — but feature 007's code-research overlooked them (the task wrongly assumed the TUI layer had no tests). When touching `top/menu.go` or `top/config_view.go`, grep `top/*_test.go` for the function you changed before assuming it is untested.

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

### Commit trailers — single `Co-Authored-By`

A commit message carries **at most one** `Co-Authored-By:` trailer, on the last line. When
collapsing several commits into one, **deduplicate the trailer to a single line** — do not let the
concatenated commit bodies stack N copies. The per-commit trailer on the feature branch is fine;
the pile-up only appears when bodies are joined, so the fix lives at the squash/merge step:

- **GitHub PR squash merge** (the usual path here — see the `(#NNN)` merge commits on `develop`):
  GitHub's default squash body concatenates every source commit message, so each per-commit
  trailer stacks (plus GitHub appends its own `Co-authored-by` lines). **Override the squash
  commit body** — `gh pr merge --squash --body "…"` (or edit it in the merge UI) so the final
  message has exactly one `Co-Authored-By:` at the end.
- **Local squash** — `git merge --squash {branch}` then `git commit` with a hand-written body
  (one trailer), not the auto-generated `.git/SQUASH_MSG` that lists every commit.

## Linting

`.golangci.yml` enables: errcheck, gocritic, gosimple, govet, ineffassign, revive, staticcheck, unused.
Run locally: `make lint` (golangci-lint + gosec) and `make vuln` (govulncheck).
Known suppressions: `// #nosec G204,G702` on `exec.Command` calls (pager/editor from env vars).

## Naming Conventions

Go acronyms: `CPUStat` not `CpuStat`, `PGresult` not `PgResult`.
Unused function parameters in callbacks: rename to `_`.
