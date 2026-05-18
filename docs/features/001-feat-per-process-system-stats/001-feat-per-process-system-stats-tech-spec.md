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

The screen is registered as a regular view with a 7-column `pg_stat_activity` SQL query.
The enrichment step is triggered via `view.CollectExtra == stat.CollectProcPidStat`
(a new typed integer constant, parallel to how `CollectDiskstats` / `CollectNetdev` work).
After collecting the SQL result, `Collector.Update()` reads procfs for each PID, computes
rate/accumulated metrics, and replaces the SQL result with the merged 17-column one.
`view.IOAvailable bool` carries the IO capability flag from the screen-open handler to the
Collector via the existing view channel (same mechanism as `view.ShowExtra`).

## Architecture

### What we're building/modifying

- **`internal/stat/procpidstat.go`** (new) — parser types, reader functions, IO check, result builder
- **`internal/stat/procpidstat_test.go`** (new) — unit and integration tests
- **`internal/query/procpidstat.go`** (new) — 7-column `pg_stat_activity` SQL constant
- **`internal/stat/stat.go`** — new `CollectProcPidStat` constant; new snapshot fields on `Collector`; enrichment branch in `Collector.Update()`
- **`internal/view/view.go`** — new `"procpidstat"` view; new `CollectExtra int` and `IOAvailable bool` fields on `View`; new `NotRecordable bool` field
- **`top/keybindings.go`** — add `'S'` binding
- **`top/config_view.go`** — add `switchViewToProcPidStat()` with local-mode guard; extend `toggleIdleConns` guard
- **`top/dialog.go`** — isolate `dialogChangeAge` from compound dialog guard; extend for `"procpidstat"`
- **`top/help.go`** — add `'S'` entry
- **`record/record.go`** — skip `NotRecordable` views in `filterViews()`

### How it works

```
User presses 'S'
  → switchViewToProcPidStat(app)        [guard: db.Local == true]
      ioErr := checkIOAvailable()        [reads /proc/self/io once]
      if ioErr != nil → printCmdline(warning), ioAvail = false
      else ioAvail = true
      build view.View{
          Name:         "procpidstat",
          CollectExtra: stat.CollectProcPidStat,
          IOAvailable:  ioAvail,
          ...           (other fields from view.Views["procpidstat"])
      }
  → viewSwitchHandler(config, view)     [sends view on config.viewCh]

Collector.Update(view) tick:
  1. collectPostgresStat(db, view.Query) → activity PGresult (7 cols)
  2. if view.CollectExtra == CollectProcPidStat:
       a. cleanup: rebuild prev maps retaining only PIDs in current activity result
       b. swap: prevProcPidStats ← currProcPidStats, prevProcPidIO ← currProcPidIO
       c. for each pid in activity result (parse col 0 via strconv.Atoi, guard pid > 0):
            readProcPidStat(pid) → currProcPidStats[pid]  (skip row on error)
            if view.IOAvailable: readProcPidIO(pid) → currProcPidIO[pid]
       d. result = buildProcPidResult(activity, prev*, curr*, view.IOAvailable,
                                      c.config.ticks, itv, runtime.NumCPU())
          [always returns 17-col PGresult; rate cols = 0 if no prev entry]
  3. DiffIntvl = [0,0] → skip SQL-level diff engine
  4. Sort, align, render via existing printDbstat() unchanged
```

**Snapshot cleanup (step 2a):** After each tick, rebuild prev maps to contain only PIDs
present in the current SQL result. This prevents unbounded memory growth when backends
exit at high churn (e.g., OLTP with many short connections).

```go
// Before swap: keep only active PIDs
newPrev := make(map[int]ProcPidStat, len(activityPIDs))
for _, pid := range activityPIDs {
    if s, ok := c.currProcPidStats[pid]; ok {
        newPrev[pid] = s
    }
}
c.prevProcPidStats = newPrev
// same pattern for IO maps
```

**Column assembly in `buildProcPidResult`:**

| Output col | Value | Notes |
|---|---|---|
| pid | activity col 0 (string) | |
| datname | activity col 1 | |
| usename | activity col 2 | |
| state | activity col 3 | |
| wait_etype | activity col 4 | |
| wait_event | activity col 5 | |
| all_total,s | `formatCPUTime(curr.Utime+curr.Stime, ticks)` | HH:MM:SS |
| us_total,s | `formatCPUTime(curr.Utime, ticks)` | |
| sy_total,s | `formatCPUTime(curr.Stime, ticks)` | |
| read_total,KiB | `curr.ReadBytes/1024` | `""` if !IOAvailable |
| write_total,KiB | `curr.WriteBytes/1024` | `""` if !IOAvailable |
| %all | `sValue(prev.Utime+prev.Stime, curr.Utime+curr.Stime, itv, ticks)/cpuCount` | 0 if no prev |
| %us | `sValue(prev.Utime, curr.Utime, itv, ticks)/cpuCount` | 0 if no prev |
| %sy | `sValue(prev.Stime, curr.Stime, itv, ticks)/cpuCount` | 0 if no prev |
| read,KiB/s | `sValue(prev.ReadBytes, curr.ReadBytes, itv, 1)/1024` | `""` if !IOAvailable |
| write,KiB/s | `sValue(prev.WriteBytes, curr.WriteBytes, itv, 1)/1024` | `""` if !IOAvailable |
| query | activity col 6 | |

**First tick (no prev entry):** rate columns (`%all`, `%us`, `%sy`, `read,KiB/s`, `write,KiB/s`)
are set to `"0"`. Accumulated columns are computed from curr only and are correct from tick 1.
This is consistent with the user-spec requirement and avoids division by arbitrary prev values.

**`/proc/[pid]/stat` parsing:** field 2 is `(comm)` and may contain spaces. Find the last `)`
in the line, split the suffix by whitespace. `utime` = suffix index 11, `stime` = index 12
(0-based; field 14 and 15 in kernel ABI, minus 3 for the consumed `pid (comm) state` prefix).
On parse error (unexpected format, non-numeric fields) — return error → skip this PID's row.

**`/proc/[pid]/io` parsing:** read key-value pairs line by line; extract `read_bytes` and
`write_bytes`. On parse error — return error → IO columns for this row are `""`.

**`ticks` source:** `c.config.ticks` (already stored in `Collector` from `getSysticksLocal()`
called in `NewCollector()`). Passed as argument to `buildProcPidResult`.

**`itv` source:** elapsed duration between current and previous `Update()` call, in seconds,
already computed in the existing `Update()` loop (used for diskstats/netdev). Guard: if `itv == 0`,
rate columns = `"0"` (same as first-tick behavior) to avoid division by zero.

**First-tick 17-column guarantee:** `buildProcPidResult` always returns a `PGresult` with
`Ncols = 17`. On first tick, prev maps are empty; rate cols are `"0"`. This prevents the
`align.SetAlign()` mismatch panic (issue #99 class) when `Ncols` in view config differs from
actual column count in result.

## Decisions

### Decision 1: CollectExtra int on View — parallel to ShowExtra, avoids string coupling
**Decision:** Add `CollectExtra int` field to `view.View`. Define `CollectProcPidStat` constant
in `internal/stat/stat.go`. Set `CollectExtra: stat.CollectProcPidStat` on the view in
`switchViewToProcPidStat()`. In `Collector.Update()`, `switch view.CollectExtra`.
**Rationale:** Follows the established project pattern (`CollectDiskstats`, `CollectNetdev` etc.
are typed int constants checked via switch). Avoids string-coupling between `internal/stat`
and view names. No import cycles: `internal/view` stays independent, `top/` sets the constant.
**Alternatives considered:** String comparison `view.Name == "procpidstat"` (rejected — not the
established pattern, couples `internal/stat` to string names from another package); reuse
`ShowExtra` iota (rejected — `ShowExtra != 0` triggers side-panel creation in `top/ui.go`,
which is wrong for a main-area view).

### Decision 2: IOAvailable carried via view.View.IOAvailable bool
**Decision:** Add `IOAvailable bool` to `view.View`. Check `checkIOAvailable()` once in
`switchViewToProcPidStat()`, set the field, send the view on `viewCh`. Collector reads
`view.IOAvailable` each tick.
**Rationale:** `Collector` is not accessible from `top/` — it lives inside the `collectStat`
goroutine in `top/stat.go`. The only established communication channel is `viewCh chan view.View`.
Adding a field to `view.View` is the exact mechanism used for `ShowExtra`.
**Alternatives considered:** Store on `app.config.collector` (rejected — `Collector` is not
exposed in `app.config`; would require architectural changes elsewhere); global variable
(rejected — not goroutine-safe, not idiomatic).

### Decision 3: NotRecordable bool on View struct
**Decision:** Add `NotRecordable bool` to `view.View`. Set `NotRecordable: true` on the
`"procpidstat"` view. In `record/record.go:filterViews()`, skip views where `NotRecordable`.
**Rationale:** Go zero value `false` means all existing views remain recordable without changes.
Only the new view opts out. Record/report is explicitly excluded from v1 scope.
**Alternatives considered:** Check view name in recorder (rejected — fragile cross-package
name coupling); empty QueryTmpl (rejected — QueryTmpl IS used for SQL collection).

### Decision 4: First tick → rate = 0, not approximation
**Decision:** When prev PID is absent from the snapshot map, all rate columns are `"0"`.
**Rationale:** User-spec explicitly states rate columns show 0 on first tick. Using
`sValue(0, curr, itv, ticks)` would produce a large incorrect spike (current absolute
jiffies / short itv). Zero is safer, predictable, and matches user expectations.
**Alternatives considered:** Spike on first tick (rejected — misleading; violates user-spec).

### Decision 5: Snapshot map cleanup before swap
**Decision:** Before swapping prev←curr, rebuild the prev map retaining only PIDs present
in the current SQL result.
**Rationale:** Prevents unbounded memory growth at high backend churn. Without cleanup,
PIDs of exited backends accumulate in the map indefinitely.
**Alternatives considered:** Purge on every Nth tick (rejected — complex, doesn't bound
memory tightly); no cleanup (rejected — memory leak per security auditor finding).

### Decision 6: PID integer validation before procfs path construction
**Decision:** Parse PID string from SQL result via `strconv.Atoi(col[0].String)`. Guard
`pid > 0`. Use integer `pid` in `fmt.Sprintf("/proc/%d/stat", pid)`. Skip row on parse error.
**Rationale:** Prevents path traversal — a PID like `"../etc/passwd"` would be rejected by
`strconv.Atoi`. PostgreSQL's `integer` type guarantees numeric values under normal operation,
but defensive validation is required when constructing filesystem paths from user-reachable data.
**Alternatives considered:** Trust PostgreSQL type guarantees (rejected — defensive coding
required when building paths from any external data).

### Decision 7: dialogChangeAge guard — isolated check, not compound extension
**Decision:** In `top/dialog.go`, the existing compound guard `(d > dialogFilter && d <= dialogChangeAge) && name != "activity"` is restructured: the `dialogChangeAge`-specific view guard is extracted into a separate `if` block that also allows `"procpidstat"`. The cancel/terminate/mask dialogs remain gated to `"activity"` only.
**Rationale:** The compound guard naively extended to include `"procpidstat"` would enable backend termination dialogs on the procpidstat screen, which is unintended and dangerous.
**Alternatives considered:** Extend compound guard as-is (rejected — enables unintended dialogs on new screen).

### Decision 8: toggleIdleConns guard extended to "procpidstat"
**Decision:** In `top/config_view.go:toggleIdleConns()`, extend the guard from `name != "activity"` to `name != "activity" && name != "procpidstat"`.
**Rationale:** User-spec requires `I` filter to work on the procpidstat screen. The guard currently prevents it. Minimal targeted fix.

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

### New constant in `internal/stat/stat.go`

```go
const (
    // existing constants...
    CollectProcPidStat = 5 // or next available value in iota
)
```

### New View struct fields in `internal/view/view.go`

```go
type View struct {
    // ... existing fields ...
    CollectExtra  int  // signals non-SQL enrichment; 0 = none
    IOAvailable   bool // procfs /proc/[pid]/io readable; set by switchViewToProcPidStat
    NotRecordable bool // if true, skip this view in pgcenter record
}
```

### New view entry `"procpidstat"` in `view.New()`

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
    // CollectExtra and IOAvailable are set at runtime in switchViewToProcPidStat
},
```

### New SQL constant `internal/query/procpidstat.go`

```go
// PgStatActivityProcPidStat selects 7 columns for the per-process system stats screen.
// Column order: pid, datname, usename, state, wait_etype, wait_event, query.
// Reuses the same ShowNoIdle and QueryAgeThresh template conventions as the activity query.
const PgStatActivityProcPidStat = `SELECT pid,
    coalesce(datname, '') AS datname,
    coalesce(usename, '') AS usename,
    coalesce(state, '') AS state,
    coalesce(wait_event_type, '') AS wait_etype,
    coalesce(wait_event, '') AS wait_event,
    regexp_replace(coalesce(query, ''), E'\\s+', ' ', 'g') AS query
FROM pg_stat_activity
WHERE pid != pg_backend_pid()
{{if .ShowNoIdle}}AND state != 'idle'{{end}}
{{if ne .QueryAgeThresh ""}}AND query_start < now() - '{{.QueryAgeThresh}}'::interval{{end}}
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
}
```

Initialized as empty maps in `NewCollector()`. `IOAvailable` is NOT stored on Collector — it
is read from `view.IOAvailable` each tick (re-read from view in case of future view re-switches).

## Dependencies

### New packages
- `runtime` (stdlib) — `runtime.NumCPU()` for CPU normalization
- `strconv` (stdlib) — `strconv.Atoi()` for PID integer validation

### Using existing (from project)
- `internal/stat.sValue()` — rate formula, reused for CPU % and IO KiB/s
- `internal/stat.PGresult` — universal tabular container, output of `buildProcPidResult`
- `top.printCmdline()` — one-time warning for EACCES / remote-mode
- `top.viewSwitchHandler()` — standard view switching, called from `switchViewToProcPidStat`

## Testing Strategy

**Feature size:** L

### Unit tests (`internal/stat/procpidstat_test.go`)

- `readProcPidStat`: golden file with comm containing spaces → correct Utime/Stime
- `readProcPidStat`: golden file with normal comm → correct values
- `readProcPidStat`: truncated/malformed line → returns error
- `readProcPidIO`: golden file → correct ReadBytes/WriteBytes
- `readProcPidIO`: missing key → returns error
- `buildProcPidResult`: two ticks with known prev/curr → exact `%all`, `read,KiB/s`, `all_total,s`
- `buildProcPidResult`: first tick (empty prev map) → rate cols `"0"`, accumulated correct
- `buildProcPidResult`: `IOAvailable=false` → IO cols are `""`
- `buildProcPidResult`: `itv=0` → rate cols `"0"` (division guard)
- `buildProcPidResult`: always returns Ncols=17 regardless of input
- `formatCPUTime`: table-driven — 0 jiffies/100ticks→`"00:00:00"`, 360000/100→`"01:00:00"`

### Integration tests (`internal/stat/procpidstat_test.go`)

- `readProcPidStat(os.Getpid())` → Utime+Stime > 0, no error
- `readProcPidIO(os.Getpid())` → ReadBytes+WriteBytes ≥ 0, no error
- `checkIOAvailable()` → no error (test process always reads `/proc/self/io`)

### E2E tests
None — TUI cannot be automated.

## Agent Verification Plan

**Source:** user-spec "Как проверить" section.

### Verification approach
Agent verifies via `go test` and `make build` / `make lint`. TUI behavior verified by user.

### Per-task verification

| Task | Verify | What to check |
|------|--------|---------------|
| 1 (procfs parsers) | bash | `go test ./internal/stat/... -run ProcPid` passes |
| 2 (SQL query) | bash | `go test ./internal/query/... -run ProcPidStat` passes |
| 3 (result builder) | bash | `go test ./internal/stat/... -run BuildProcPid\|FormatCPU` passes |
| 4 (view + record) | bash | `go test ./record/... && make build` succeeds |
| 5 (collector) | bash | `go test ./internal/stat/... -run TestCollector` passes |
| 6 (hotkey + guards) | bash | `make build && make lint` clean |
| QA | bash | `make test` — all tests pass, no race conditions |

### Tools required
bash — all verification via go test and make commands.

## Backward Compatibility

N/A — new code only. Three new fields on `View` struct (`CollectExtra int`, `IOAvailable bool`,
`NotRecordable bool`) use Go zero values (`0`, `false`, `false`) that preserve all existing
behavior. `record/record.go:filterViews()` gains one `if !view.NotRecordable { continue }` check
that is a no-op for all existing views.

## Risks

| Risk | Mitigation |
|------|-----------|
| `/proc/[pid]/stat` comm with spaces → wrong field indices | Parse by finding last `)`, split suffix; golden file test covers this |
| `/proc/[pid]/io` EACCES (default Linux ptrace_scope=1) | `checkIOAvailable()` on screen open, single warning, graceful empty IO columns |
| `itv=0` division by zero in `sValue` | Guard in `buildProcPidResult`: if `itv==0` → rate = "0" |
| First-tick 17-col vs 7-col Ncols mismatch → panic (#99 class) | `buildProcPidResult` always returns 17-col result; guaranteed by test |
| Stale PID memory growth at high churn | Snapshot map cleanup before each swap (Decision 5) |
| `dialogChangeAge` compound guard enables cancel/terminate on procpidstat | Isolated guard extraction (Decision 7); covered by review |
| Path traversal via PID string from SQL | `strconv.Atoi` + `pid > 0` guard (Decision 6) |
| Record subsystem collects procpidstat data (wrong, no procfs metrics) | `NotRecordable: true` + `filterViews()` skip (Decision 3) |

## Acceptance Criteria

- [ ] `make test` passes with race detector, no new test failures
- [ ] `make lint` and `make vuln` clean
- [ ] `Shift+S` (`'S'`) switches to procpidstat view in local mode
- [ ] `Shift+S` in remote mode prints warning, does not switch view
- [ ] Screen displays 17 columns in order: pid, datname, usename, state, wait_etype, wait_event, all_total,s, us_total,s, sy_total,s, read_total,KiB, write_total,KiB, %all, %us, %sy, read,KiB/s, write,KiB/s, query
- [ ] `all_total,s` / `us_total,s` / `sy_total,s` formatted as `HH:MM:SS`, sort correctly
- [ ] `%all` / `%us` / `%sy` in range 0–100 under CPU workload
- [ ] Rate columns show `"0"` on first tick, increase on subsequent ticks under load
- [ ] IO columns empty (`""`) when `/proc/self/io` returns EACCES; EACCES warning shown once per session
- [ ] CPU columns work normally when IO is unavailable
- [ ] `I` filter hides `state='idle'` backends on procpidstat screen
- [ ] `A` filter applies age threshold on procpidstat screen
- [ ] Cancel/terminate/mask dialogs are NOT available on procpidstat screen
- [ ] `pgcenter record` does not write procpidstat data
- [ ] No panic when a backend exits between ticks
- [ ] No memory growth in Collector after many ticks with high backend churn

## Implementation Tasks

### Wave 1 (independent)

#### Task 1: Procfs parser types and reader functions
- **Description:** Create `internal/stat/procpidstat.go` with `ProcPidStat`, `ProcPidIO` structs, `readProcPidStat(pid int)` parsing `/proc/[pid]/stat` (handle comm with spaces via last-`)` method), `readProcPidIO(pid int)` parsing `/proc/[pid]/io`, and `checkIOAvailable()` reading `/proc/self/io`. Add unit tests with golden files in `internal/stat/testdata/proc/` and integration tests using `os.Getpid()`.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/stat/... -run ProcPid`
- **Files to modify:** `internal/stat/procpidstat.go` (new), `internal/stat/procpidstat_test.go` (new)
- **Files to read:** `internal/stat/cpu.go`, `internal/stat/diskstats.go`, `internal/stat/stat.go`

#### Task 2: Simplified pg_stat_activity SQL query
- **Description:** Create `internal/query/procpidstat.go` with constant `PgStatActivityProcPidStat` — a 7-column query returning `pid, datname, usename, state, wait_etype, wait_event, query` with `ShowNoIdle` and `QueryAgeThresh` template variables following the exact conventions of the existing activity query (use `{{if ne .QueryAgeThresh ""}}` not `{{if gt ...}}`).
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./internal/query/...`
- **Files to modify:** `internal/query/procpidstat.go` (new)
- **Files to read:** `internal/query/activity.go`, `internal/query/query.go`

### Wave 2 (depends on Wave 1)

#### Task 3: Result builder, CPU formatter, and PID validation
- **Description:** Add `buildProcPidResult()` to `internal/stat/procpidstat.go`: joins 7-col `PGresult` with prev/curr `ProcPidStat`/`ProcPidIO` maps to produce a 17-col `PGresult`. Rate cols = `"0"` when prev is absent or `itv==0`. IO cols = `""` when `!ioAvailable`. Add `formatCPUTime(jiffies, ticks float64) string` producing `HH:MM:SS`. PID column (col 0) is validated via `strconv.Atoi` + `pid > 0`; invalid PIDs skip procfs reads. Add unit tests covering first-tick, io-unavailable, itv=0, and correct 17-col guarantee.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/stat/... -run BuildProcPid\|FormatCPU`
- **Files to modify:** `internal/stat/procpidstat.go`, `internal/stat/procpidstat_test.go`
- **Files to read:** `internal/stat/stat.go` (sValue, Collector, ticks), `internal/stat/postgres.go` (PGresult), `internal/stat/cpu.go`

#### Task 4: View registration, new View fields, and record skip
- **Description:** Add `CollectExtra int`, `IOAvailable bool`, `NotRecordable bool` fields to the `View` struct in `internal/view/view.go`. Add `CollectProcPidStat` constant in `internal/stat/stat.go`. Register `"procpidstat"` view in `view.New()` with `QueryTmpl: query.PgStatActivityProcPidStat`, `DiffIntvl: [2]int{0,0}`, `Ncols: 17`, `NotRecordable: true`. In `record/record.go:filterViews()`, add skip for `NotRecordable` views.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./record/... && make build`
- **Files to modify:** `internal/view/view.go`, `internal/stat/stat.go`, `record/record.go`
- **Files to read:** `internal/view/view.go` (View struct, view.New()), `record/record.go` (filterViews), `internal/query/procpidstat.go`

### Wave 3 (depends on Wave 2)

#### Task 5: Collector integration — snapshot management and enrichment
- **Description:** Add `prevProcPidStats`, `currProcPidStats map[int]ProcPidStat`, `prevProcPidIO`, `currProcPidIO map[int]ProcPidIO` fields to `Collector`. Initialize as empty maps in `NewCollector()`. In `Collector.Update()`, add a `case CollectProcPidStat` branch: cleanup stale PIDs from prev maps, swap prev←curr, collect new procfs snapshots per PID, call `buildProcPidResult()`, replace the SQL result. Pass `c.config.ticks` and computed `itv` to the builder.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./internal/stat/... -run TestCollector`
- **Files to modify:** `internal/stat/stat.go`
- **Files to read:** `internal/stat/stat.go` (Collector, Update, collectExtra switch), `internal/stat/procpidstat.go`

#### Task 6: Hotkey, local-mode guard, and filter guard extensions
- **Description:** In `top/keybindings.go`, add `{"sysstat", 'S', switchViewToProcPidStat(app)}`. Implement `switchViewToProcPidStat(app)` in `top/config_view.go`: guard `db.Local`, call `checkIOAvailable()`, build a `view.View` with `CollectExtra: stat.CollectProcPidStat` and `IOAvailable` set, then call `viewSwitchHandler`. Extend `toggleIdleConns` guard to also allow `"procpidstat"`. In `top/dialog.go`, isolate the `dialogChangeAge` guard into a separate check that also allows `"procpidstat"` — without extending cancel/terminate/mask dialogs.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `make build && make lint`
- **Files to modify:** `top/keybindings.go`, `top/config_view.go`, `top/dialog.go`, `top/help.go`
- **Files to read:** `top/config_view.go` (switchViewTo, toggleIdleConns, viewSwitchHandler), `top/dialog.go` (compound guard), `top/pglog.go` (db.Local guard pattern), `top/ui.go` (printCmdline), `top/help.go`

### Final Wave

#### Task 7: Pre-deploy QA
- **Description:** Run all tests with race detector, verify acceptance criteria from user-spec and tech-spec. Confirm no regression in existing screens (activity, tables, statements). Manual TUI verification items are listed in user-spec "Пользователь проверяет" section.
- **Skill:** pre-deploy-qa
- **Reviewers:** none
