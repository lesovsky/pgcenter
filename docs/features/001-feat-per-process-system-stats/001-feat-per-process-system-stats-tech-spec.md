---
created: 2026-05-18
status: draft
branch: feature/per-process-system-stats
size: L
---

# Tech Spec: Per-process System Stats Screen

## Solution

Add a new TUI screen `"procpidstat"` to `pgcenter top` that joins `pg_stat_activity` data
with per-process procfs metrics from `/proc/[pid]/stat` and `/proc/[pid]/io`.
The screen produces a 17-column `PGresult` (7 SQL columns + 5 accumulated + 4 rate + `query`)
that flows through the existing rendering pipeline unchanged.

The enrichment is triggered inside `Collector.Update()` when the active view name is
`"procpidstat"`: after collecting the 7-column SQL result, the collector reads procfs for
each PID, computes rate/accumulated metrics against the previous snapshot, and replaces the
SQL result with the merged 17-column one before it reaches the renderer.

All existing screen infrastructure (sorting, column width, idle/age filters, alignment)
works without modification once the filters' view-name guards are extended to include
`"procpidstat"`.

## Architecture

### What we're building/modifying

- **`internal/stat/procpidstat.go`** (new) — procfs parser types, reader functions,
  IO availability check, result builder
- **`internal/stat/procpidstat_test.go`** (new) — unit and integration tests
- **`internal/query/procpidstat.go`** (new) — simplified `pg_stat_activity` SQL constant (7 cols)
- **`internal/stat/stat.go`** — new `CollectProcPidStat` constant; new snapshot fields
  on `Collector`; enrichment call in `Collector.Update()`
- **`internal/view/view.go`** — new `"procpidstat"` view entry; new `NotRecordable bool`
  field on `View` struct
- **`top/keybindings.go`** — add `'S'` binding
- **`top/config_view.go`** — add `switchViewToProcPidStat()` with local-mode guard;
  extend `toggleIdleConns` view-name guard
- **`top/dialog.go`** — extend `dialogChangeAge` view-name guard
- **`top/help.go`** — add `'S'` entry to help text
- **`record/stat.go`** — skip views with `NotRecordable: true`

### How it works

```
User presses 'S'
  → switchViewToProcPidStat(app)    [guard: db.Local == true]
  → viewSwitchHandler(config, "procpidstat")
  → view config sent on config.viewCh

Collector.Update() tick:
  1. collectPostgresStat(db, view.Query)     → activity PGresult (7 cols, pid in col 0)
  2. if view.Name == "procpidstat":
       a. swap prevProcPidStats ← currProcPidStats
          swap prevProcPidIO   ← currProcPidIO
       b. for each pid in activity result:
            readProcPidStat(pid) → currProcPidStats[pid]
            if ioAvailable: readProcPidIO(pid) → currProcPidIO[pid]
       c. buildProcPidResult(activity, prev*, curr*, ioAvailable, ticks, itv, cpuCount)
            → 17-col PGresult (replaces activity result)
  3. DiffIntvl = [0,0] → skip SQL-level diff
  4. Sort, align, render via existing printDbstat()
```

**Column assembly in `buildProcPidResult`:**

| Output col | Source |
|---|---|
| pid | activity col 0 (string, as-is) |
| datname | activity col 1 |
| usename | activity col 2 |
| state | activity col 3 |
| wait_etype | activity col 4 |
| wait_event | activity col 5 |
| all_total,s | `formatCPUTime((curr.Utime + curr.Stime) / ticks)` → `HH:MM:SS` |
| us_total,s | `formatCPUTime(curr.Utime / ticks)` |
| sy_total,s | `formatCPUTime(curr.Stime / ticks)` |
| read_total,KiB | `curr.ReadBytes / 1024` (or `""` if !ioAvailable) |
| write_total,KiB | `curr.WriteBytes / 1024` (or `""`) |
| %all | `sValue(prev.Utime+prev.Stime, curr.Utime+curr.Stime, itv, ticks) / cpuCount` |
| %us | `sValue(prev.Utime, curr.Utime, itv, ticks) / cpuCount` |
| %sy | `sValue(prev.Stime, curr.Stime, itv, ticks) / cpuCount` |
| read,KiB/s | `sValue(prev.ReadBytes, curr.ReadBytes, itv, 1) / 1024` (or `""`) |
| write,KiB/s | `sValue(prev.WriteBytes, curr.WriteBytes, itv, 1) / 1024` (or `""`) |
| query | activity col 6 |

**First tick:** prev snapshot is empty map → delta = 0 for all rate columns. `ProcPidStat{}` zero value (Utime=0, Stime=0) produces `sValue(0, curr, itv, ticks)` which gives the current absolute rate from process start divided by itv — acceptable approximation for first tick.

**PID disappears:** not present in activity result → skipped in procfs loop → not in output. Stale entries in prev maps are harmless; they are overwritten next tick or garbage-collected when the PID reappears.

**`/proc/[pid]/stat` parsing:** field 2 is `(comm)` which may contain spaces. Parse by finding the last `)`, then splitting the remainder. `utime` = index 11, `stime` = index 12 in the post-paren token array (0-based, after discarding 3 consumed fields: state, ppid, pgrp... actually after `(comm) state` the rest starts at field index 3 from 1-based; `utime`=field14 = post-paren index 11).

## Decisions

### Decision 1: Enrichment triggered by view name, not a new channel
**Decision:** Detect `view.Name == "procpidstat"` inside `Collector.Update()` to trigger procfs enrichment.
**Rationale:** No new channel or configuration type needed. The view name is already available in the update loop. Adding a new flag or constant would require touching more files.
**Alternatives considered:** Use `collectExtra` constant (rejected — semantically meant for side-panel, not main view enrichment); add a new `Collector` config field (rejected — more surface area with no benefit).

### Decision 2: NotRecordable field on View struct
**Decision:** Add `NotRecordable bool` field to `internal/view/view.go:View`. Set `NotRecordable: true` on `"procpidstat"`. The recorder skips views where this flag is set.
**Rationale:** Go zero value for `bool` is `false`, so all existing views are recordable by default without touching them. Only the new view opts out. Cleaner than checking by name in the recorder.
**Alternatives considered:** Check view name in recorder (rejected — fragile, name-coupling across packages); add `Recordable: true` to all existing views (rejected — large mechanical diff with no value).

### Decision 3: Snapshot maps on Collector, not package-level
**Decision:** `prevProcPidStats`, `currProcPidStats`, `prevProcPidIO`, `currProcPidIO map[int]ProcPidStat/ProcPidIO` stored as fields on `Collector`.
**Rationale:** `Collector` already owns all other snapshot state. Consistent pattern, goroutine-safe (single collector goroutine).
**Alternatives considered:** Package-level vars (rejected — not goroutine-safe in theory, not idiomatic); separate struct (rejected — unnecessary abstraction).

### Decision 4: IO availability checked once per session on screen open
**Decision:** `checkIOAvailable()` (reads `/proc/self/io`) is called in `switchViewToProcPidStat()` and its result is stored as `ioAvailable bool` on `Collector`. Warning is printed once via `printCmdline()` if unavailable.
**Rationale:** Avoids repeated syscall overhead on every tick. Warning shown exactly once per session, not on every update. Matches user-spec requirement.
**Alternatives considered:** Check every tick (rejected — overhead, repeated warnings); check in test setup (rejected — leaks into test infra).

### Decision 5: DiffIntvl = [2]int{0, 0} — bypass SQL diff engine
**Decision:** Set `DiffIntvl: [2]int{0, 0}` on the `"procpidstat"` view. The existing `calculateDelta()` is bypassed; all diff/rate computation happens in `buildProcPidResult()`.
**Rationale:** SQL diff operates on two consecutive SQL results assuming stable column semantics. Per-pid procfs diff requires custom per-pid keying by PID, jiffies→% conversion, and IO byte→KiB/s conversion — none of which the SQL diff engine handles.
**Alternatives considered:** Use SQL diff engine (rejected — wrong granularity, no PID-keyed delta support).

### Decision 6: CPU normalization via runtime.NumCPU()
**Decision:** Divide CPU rate by `runtime.NumCPU()` to normalize to 0–100%.
**Rationale:** `runtime.NumCPU()` returns the number of logical CPUs available to the process, which is the correct denominator for wall-clock CPU%. Cheap call, no syscall overhead.
**Alternatives considered:** Count `/proc/cpuinfo` entries (rejected — more code, equivalent result); use CPUStat.Total from system snapshot (rejected — coupling to another collector, available at wrong time).

### Decision 7: toggleIdleConns and dialogChangeAge guards extended (not refactored)
**Decision:** Add `|| config.view.Name == "procpidstat"` to the existing guards in `config_view.go` and `dialog.go`. No refactoring of the guard mechanism.
**Rationale:** Minimal change, consistent with existing code style. The guards are short and self-documenting. User-spec incorrectly stated these were "global" — they are view-name-gated; this is the targeted fix.
**Alternatives considered:** Refactor to a `filterableViews` set (rejected — scope creep, not requested).

## Data Models

### New structs in `internal/stat/procpidstat.go`

```go
// ProcPidStat holds raw jiffie values from /proc/[pid]/stat.
type ProcPidStat struct {
    Utime float64 // user-mode CPU time, jiffies
    Stime float64 // kernel-mode CPU time, jiffies
}

// ProcPidIO holds raw byte counts from /proc/[pid]/io.
type ProcPidIO struct {
    ReadBytes  float64
    WriteBytes float64
}
```

### View struct addition in `internal/view/view.go`

```go
type View struct {
    // ... existing fields ...
    NotRecordable bool // if true, skip this view in pgcenter record
}
```

### New view entry `"procpidstat"`

```go
"procpidstat": {
    Name:          "procpidstat",
    QueryTmpl:     query.PgStatActivityProcPidStat,
    DiffIntvl:     [2]int{0, 0},
    Ncols:         17,
    OrderKey:      0,
    OrderDesc:     false,
    ColsWidth:     map[int]int{},
    Msg:           "Show per-process system stats",
    NotRecordable: true,
},
```

### New SQL constant `internal/query/procpidstat.go`

```go
// PgStatActivityProcPidStat returns 7 columns needed for the per-process system stats screen.
// Column order: pid, datname, usename, state, wait_event_type, wait_event, query
const PgStatActivityProcPidStat = `SELECT pid, datname, usename, state,
    coalesce(wait_event_type, '') AS wait_etype,
    coalesce(wait_event, '') AS wait_event,
    regexp_replace(query, E'\\s+', ' ', 'g') AS query
FROM pg_stat_activity
WHERE pid != pg_backend_pid()
{{if .ShowNoIdle}}AND state != 'idle'{{end}}
{{if gt .QueryAgeThresh 0}}AND query_start < now() - '{{.QueryAgeThresh}} seconds'::interval{{end}}
ORDER BY pid`
```

### New Collector fields in `internal/stat/stat.go`

```go
type Collector struct {
    // ... existing fields ...
    prevProcPidStats map[int]ProcPidStat
    currProcPidStats map[int]ProcPidStat
    prevProcPidIO    map[int]ProcPidIO
    currProcPidIO    map[int]ProcPidIO
    ioAvailable      bool
}
```

## Dependencies

### New packages
- `runtime` (stdlib) — `runtime.NumCPU()` for CPU normalization

### Using existing (from project)
- `internal/stat.sValue()` — rate formula, reused for CPU % and IO KiB/s
- `internal/stat.PGresult` — universal tabular container, output of `buildProcPidResult`
- `top.printCmdline()` — one-time warning for EACCES/remote-mode
- `top.viewSwitchHandler()` — standard view switching, reused in `switchViewToProcPidStat`

## Testing Strategy

**Feature size:** L

### Unit tests (`internal/stat/procpidstat_test.go`)

- Parse `/proc/[pid]/stat`: golden file with comm field containing spaces → correct `Utime`/`Stime`
- Parse `/proc/[pid]/stat`: golden file with normal comm → correct values
- Parse `/proc/[pid]/io`: golden file → correct `ReadBytes`/`WriteBytes`
- `buildProcPidResult()`: two ticks with known prev/curr → assert exact `%all`, `read,KiB/s`, `all_total,s` values
- `buildProcPidResult()`: first tick (empty prev map) → rate columns are 0, accumulated columns correct
- `buildProcPidResult()`: `ioAvailable=false` → IO columns are empty string
- `formatCPUTime()`: table-driven: 0 jiffies → `"00:00:00"`, 360000 jiffies at 100 ticks → `"01:00:00"`

### Integration tests (`internal/stat/procpidstat_test.go`)

- `readProcPidStat(os.Getpid())` → values > 0, no error (process is always readable by itself)
- `readProcPidIO(os.Getpid())` → values ≥ 0, no error (self-IO always readable)
- `checkIOAvailable()` in test environment → no error (test runs as same user as `/proc/self`)

### E2E tests
None — TUI cannot be automated.

## Agent Verification Plan

**Source:** user-spec "Как проверить" section.

### Verification approach
Agent verifies via `go test` and `make build`/`make lint`. TUI behavior verified by user manually.

### Per-task verification

| Task | Verify | What to check |
|------|--------|---------------|
| 1 (procfs parsers) | bash | `go test ./internal/stat/... -run ProcPid` passes |
| 2 (SQL query) | bash | `go test ./internal/query/... -run ProcPidStat` passes |
| 3 (result builder) | bash | `go test ./internal/stat/... -run BuildProcPid` passes |
| 4 (view registration) | bash | `make build` succeeds |
| 5 (collector integration) | bash | `go test ./internal/stat/... -run TestCollector` passes |
| 6 (hotkey + local guard) | bash | `make build` + `make lint` clean |
| 7 (filter guards + record) | bash | `go test ./record/...` passes |
| 8 (help text) | bash | `make build` succeeds |
| QA | bash | `make test` — all tests pass, no race conditions |

### Tools required
bash — all verification via go test and make commands.

## Backward Compatibility

N/A — adding new code only. No existing API, function signature, DB schema, or config changes.

Exception: `View` struct gains `NotRecordable bool` field (zero-value safe — existing views
keep default `false`, behave exactly as before). `record/stat.go` gains one `if` check.

## Risks

| Risk | Mitigation |
|------|-----------|
| `/proc/[pid]/stat` comm field contains spaces → wrong field offsets | Parse by finding last `)` in line before splitting; test with golden file containing spaces in comm |
| `/proc/[pid]/io` EACCES on standard Linux when not running as `postgres` | `checkIOAvailable()` on screen open, single warning, graceful empty IO columns |
| Rate metrics show 0 on first tick | Documented in user-spec; zero-initialized prev map produces zero delta, not a panic |
| `toggleIdleConns`/`dialogChangeAge` guards missed → filters silently broken | Explicit test: switch to procpidstat view, press `I`, assert filter applied |
| Record subsystem picks up procpidstat → polluted tar archives | `NotRecordable: true` + recorder skip check; covered by `record/stat.go` unit test |

## Acceptance Criteria

- [ ] `make test` passes with race detector, no new test failures
- [ ] `make lint` and `make vuln` clean
- [ ] `Shift+S` switches to "procpidstat" view in local mode
- [ ] `Shift+S` in remote mode prints warning, does not switch view
- [ ] Screen displays 17 columns in correct order
- [ ] `all_total,s` / `us_total,s` / `sy_total,s` formatted as `HH:MM:SS`, sortable
- [ ] `%all` / `%us` / `%sy` in range 0–100 under typical PG workload
- [ ] `read,KiB/s` / `write,KiB/s` increase when PG performs I/O
- [ ] IO columns empty when `/proc/self/io` returns EACCES; EACCES warning shown once
- [ ] CPU columns work normally when IO is unavailable
- [ ] `I` filter hides `state='idle'` backends on procpidstat screen
- [ ] `A` filter applies age threshold on procpidstat screen
- [ ] `pgcenter record` does not write procpidstat data (view skipped)
- [ ] No panic when a backend exits between ticks

## Implementation Tasks

### Wave 1 (independent)

#### Task 1: Procfs parser types and reader functions
- **Description:** Create `internal/stat/procpidstat.go` with `ProcPidStat` and `ProcPidIO` structs, `readProcPidStat(pid int)` parsing `/proc/[pid]/stat` (handling comm with spaces), `readProcPidIO(pid int)` parsing `/proc/[pid]/io`, and `checkIOAvailable()` reading `/proc/self/io`. Add unit and integration tests with golden files in `internal/stat/testdata/proc/`.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/stat/... -run ProcPid`
- **Files to modify:** `internal/stat/procpidstat.go` (new), `internal/stat/procpidstat_test.go` (new)
- **Files to read:** `internal/stat/cpu.go`, `internal/stat/diskstats.go`, `internal/stat/stat.go`

#### Task 2: Simplified pg_stat_activity SQL query
- **Description:** Create `internal/query/procpidstat.go` with constant `PgStatActivityProcPidStat` — a 7-column `pg_stat_activity` query returning `pid, datname, usename, state, wait_etype, wait_event, query`, with `ShowNoIdle` and `QueryAgeThresh` template variables matching the existing activity query conventions.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./internal/query/... -run ProcPidStat`
- **Files to modify:** `internal/query/procpidstat.go` (new)
- **Files to read:** `internal/query/activity.go`, `internal/query/query.go`

### Wave 2 (depends on Wave 1)

#### Task 3: Result builder and CPU time formatter
- **Description:** Add `buildProcPidResult()` to `internal/stat/procpidstat.go`. The function joins a 7-column `PGresult` from `pg_stat_activity` with `prev`/`curr` `ProcPidStat` and `ProcPidIO` maps to produce a 17-column `PGresult` with accumulated (HH:MM:SS, KiB) and rate (`%`, KiB/s) columns. Add `formatCPUTime(jiffies, ticks float64) string` helper producing `HH:MM:SS`. Add unit tests covering first-tick zero rates, `ioAvailable=false` empty IO columns, and exact metric values over two ticks.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./internal/stat/... -run BuildProcPid`
- **Files to modify:** `internal/stat/procpidstat.go`, `internal/stat/procpidstat_test.go`
- **Files to read:** `internal/stat/stat.go` (sValue), `internal/stat/postgres.go` (PGresult), `internal/stat/cpu.go` (countCPUUsage pattern)

#### Task 4: View registration and NotRecordable field
- **Description:** Add `NotRecordable bool` field to the `View` struct in `internal/view/view.go`. Register the `"procpidstat"` view in `view.New()` with `QueryTmpl: query.PgStatActivityProcPidStat`, `DiffIntvl: [2]int{0,0}`, `Ncols: 17`, `OrderKey: 0`, `NotRecordable: true`. Add the `record/stat.go` skip check for `NotRecordable` views.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./record/... && make build`
- **Files to modify:** `internal/view/view.go`, `record/stat.go`
- **Files to read:** `internal/view/view.go` (existing view entries), `record/stat.go`, `internal/query/procpidstat.go`

### Wave 3 (depends on Wave 2)

#### Task 5: Collector integration — snapshot management and enrichment
- **Description:** Add `prevProcPidStats`, `currProcPidStats map[int]ProcPidStat`, `prevProcPidIO`, `currProcPidIO map[int]ProcPidIO`, and `ioAvailable bool` fields to `Collector` in `internal/stat/stat.go`. In `Collector.Update()`, when the active view name is `"procpidstat"`, swap prev←curr maps, collect new procfs snapshots for each PID in the SQL result, call `buildProcPidResult()`, and replace the SQL result with the 17-column one. Initialize maps in `NewCollector()`.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./internal/stat/... -run TestCollector`
- **Files to modify:** `internal/stat/stat.go`
- **Files to read:** `internal/stat/stat.go` (Collector.Update full body), `internal/stat/procpidstat.go`

#### Task 6: Hotkey binding and local-mode guard
- **Description:** Add `{"sysstat", 'S', switchViewToProcPidStat(app)}` to `keybindings()` in `top/keybindings.go`. Implement `switchViewToProcPidStat(app)` in `top/config_view.go`: check `db.Local == true`; if false, print warning and return; if true, call `checkIOAvailable()`, store result in `app.config.collector.ioAvailable` (or pass through app state), then call `viewSwitchHandler(config, "procpidstat")`. Use `printCmdline(g, "...")` for the local-mode warning and the EACCES warning.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `make build && make lint`
- **Files to modify:** `top/keybindings.go`, `top/config_view.go`
- **Files to read:** `top/keybindings.go`, `top/config_view.go` (switchViewTo, viewSwitchHandler), `top/pglog.go` (db.Local guard pattern), `top/ui.go` (printCmdline)

#### Task 7: Idle and age filter guards + help text
- **Description:** In `top/config_view.go`, extend the `toggleIdleConns` view-name guard to also allow `"procpidstat"`. In `top/dialog.go`, extend the `dialogChangeAge` view-name guard similarly. In `top/help.go`, add `'S'` to the help text under the appropriate section. These changes ensure the existing `I` and `A` hotkeys work on the new screen exactly as on the activity screen.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `make build && make lint`
- **Files to modify:** `top/config_view.go`, `top/dialog.go`, `top/help.go`
- **Files to read:** `top/config_view.go` (toggleIdleConns at line ~302), `top/dialog.go` (line ~51), `top/help.go`

### Final Wave

#### Task 8: Pre-deploy QA
- **Description:** Run all tests with race detector, verify acceptance criteria from user-spec and tech-spec. Confirm no regression in existing screens.
- **Skill:** pre-deploy-qa
- **Reviewers:** none
