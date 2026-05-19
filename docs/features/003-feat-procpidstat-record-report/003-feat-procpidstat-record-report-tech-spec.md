---
created: 2026-05-19
status: draft
branch: feature/procpidstat-record-report
size: M
---

# Tech Spec: Record/Report for Per-Process System Stats

## Solution

Add `pgcenter record` / `pgcenter report` support for the `procpidstat` view (Shift+S,
per-process system stats). Implements **Option B**: the recorder enriches procpidstat data
per-tick using the existing `buildProcPidResult` pipeline, stores a 19-column display
`PGresult` alongside a `sysinfo` entry. The report reads with `DiffIntvl=[0,0]`
(pass-through, no column diff), matching the established pattern of the `activity` view.
Resolves tech-debt [002].

## Architecture

### What we're building/modifying

- **`stat.SysInfo` struct** — JSON-serializable container for `ticks` (CLK_TCK) and
  `cpu_count` recorded at the time of collection. Written per-tick as `sysinfo.TIMESTAMP.json`.
- **`tarRecorder` stateful fields** — six fields added to the struct:
  `isLocal bool`, `ticks float64`, `cpuCount int`,
  `ioAvailable bool`, `delayAcctAvailable bool`,
  `prevProcPidStats / currProcPidStats map[int]stat.ProcPidStat`,
  `prevProcPidIO / currProcPidIO map[int]stat.ProcPidIO`.
  All persist across the `open→collect→write→close` loop iterations.

### How it works — modified components

| Component | Change |
|-----------|--------|
| `internal/stat/procpidstat.go` | Extract private `buildProcPidResultRaw` + `formatProcPidResultForDisplay`; `buildProcPidResult` becomes their composition. Add exported `SysInfo` struct. |
| `internal/stat/stat.go` | Export `getSysticksLocal` → `GetSysticksLocal` so `record` package can call it. |
| `internal/view/view.go` | Remove `NotRecordable: true` from `procpidstat` view. Local/remote gate moves to `record.app.setup()`. |
| `record/recorder.go` | `tarRecorder` gains stateful fields; `collect()` gains procfs enrichment branch gated on `c.isLocal`; `write()` appends `sysinfo` entry. |
| `record/record.go` | `app.setup()` checks `db.Local`, calls `GetSysticksLocal()`, probes IO/delayacct availability, passes result into `tarConfig`. If `!db.Local`, removes `procpidstat` from `views` before handing off to recorder and prints INFO. |
| `report/report.go` | `metadata` struct gains `ticks float64` and `cpuCount int`. `isFilenameOK` accepts `sysinfo` prefix. `readTar` handles `sysinfo.*` entries by decoding into `SysInfo` and merging into `metadata`. `describeReport` map gains `"procpidstat"` entry. |
| `cmd/report/report.go` | New `showProcPidStat bool` option, `-N` / `--proc-stats` flag, `case opts.showProcPidStat: return "procpidstat"` in `selectReport`. |

### How it works — data flow

```
pgcenter record (local mode):
  app.setup()
    └─ postgres.Connect → db.Local == true
    └─ GetSysticksLocal() → ticks
    └─ runtime.NumCPU() → cpuCount
    └─ pg_stat_activity query → first backend PID
    └─ CheckIOAvailable(pid) → ioAvailable
    └─ CheckDelayAcctAvailable() → delayAcctAvailable
    └─ tarConfig{isLocal, ticks, cpuCount, ioAvailable, delayAcctAvailable, ...}

  per-tick collect():
    ├─ SQL views (activity, tables, …) → PGresult per view
    └─ if c.isLocal && "procpidstat" in views:
         SQL pg_stat_activity → 7-col result (PIDs of active backends)
         map rotation (mirrors Collector.Update logic):
           newPrev = {pid: currStats[pid] for pid in SQL result if pid in currStats}
           prevProcPidStats ← newPrev   // only PIDs still alive
           currProcPidStats ← fresh empty map
           (same rotation for prevProcPidIO / currProcPidIO)
         for each PID in SQL result (pid > 0):
           readProcPidStat(pid) → currStats[pid]       (always)
           readProcPidIO(pid)  → currIO[pid]           (if ioAvailable)
         itv = time.Since(c.lastCollect).Seconds(); c.lastCollect = now
         buildProcPidResult(result, prev, curr, io, delay, ticks, itv, cpuCount)
         → stats["procpidstat"] = 19-col display PGresult
         (first tick: itv=0 → rate cols = "0"; processData skips first snapshot)

  write():
    ├─ stats entries → VIEWNAME.TIMESTAMP.json
    └─ SysInfo{ticks, cpuCount} → sysinfo.TIMESTAMP.json

pgcenter report -N:
  readTar():
    ├─ meta.*    → readMeta()   → metadata.version
    ├─ sysinfo.* → json.Unmarshal → metadata.{ticks, cpuCount}
    └─ procpidstat.* → stat.NewPGresultFile → PGresult

  processData() — on first valid data pair:
    if cols 9,10 (read_total,KiB / write_total,KiB) all "" in first result:
      fmt.Fprintf(writer, "WARNING: IO stats unavailable in recorded data\n")
    if col 11 (iodelay_total,s) all "" in first result:
      fmt.Fprintf(writer, "WARNING: iodelay stats unavailable in recorded data\n")
    DiffIntvl=[0,0] → countDiff returns curr unchanged (pass-through)
    printStatHeader / printStatSample → 19-col output
```

### procpidstat interval calculation in recorder

The interval for rate computation (`itv`) is the duration between successive `collect()` calls,
measured by `time.Since(prevTs)` stored as a new `lastCollect time.Time` field. On the first
tick `itv=0`, which causes `buildProcPidResult` to output `"0"` for all rate columns — the
standard first-tick behavior. `processData` in report skips the first snapshot
(`if !prevStat.Valid → continue`), so first-tick zeros are never shown.

## Decisions

### Decision 1: Option B — store display strings, DiffIntvl=[0,0]

**Decision:** Recorder computes rates at collection time and stores display strings
(19-col PGresult with `HH:MM:SS`, `%`, `KiB/s`). Report uses `DiffIntvl=[0,0]`
(pass-through, no column subtraction).

**Rationale:** Established pgcenter pattern for snapshot views (activity, progress_*).
No report pipeline changes required beyond adding sysinfo reading. Rate columns are
already per-interval — report displays them as-is.

**Alternatives considered:**
- Option A (store raw jiffies, DiffIntvl=[6,11], recompute in report): rejected because
  cols 6–11 in current `buildProcPidResult` output contain `HH:MM:SS` strings that
  `diffPair` cannot parse. Would require a second formatter in report and access to
  recording-time `ticks`/`cpuCount` for correct rate computation.

### Decision 2: isLocal propagated through tarConfig

**Decision:** `db.Local` is captured in `app.setup()` before `db.Close()` and passed
into `tarConfig`, then stored in `tarRecorder.isLocal`.

**Rationale:** `tarRecorder.collect()` opens a fresh DB connection each tick and has no
access to the `app.db` struct (which doesn't exist — `app` only stores `dbConfig`).
The `tarRecorder` instance persists across ticks so struct fields survive.

**Alternatives considered:**
- Re-checking locality inside `collect()` each tick: wasteful and wrong — `isLocalhost()`
  is a string test on `Config.Host`, not a live connection probe.

### Decision 3: Export GetSysticksLocal

**Decision:** `getSysticksLocal()` in `internal/stat/stat.go` is exported as
`GetSysticksLocal() (float64, error)`.

**Rationale:** `record` package needs CLK_TCK at startup. The function already does
exactly what's needed (`getconf CLK_TCK`). Exporting avoids duplicating the logic.

**Alternatives considered:**
- Duplicate `getconf CLK_TCK` call in recorder: code duplication, harder to maintain.
- Move initialization into stat package API: over-engineering; no other caller needs it.

### Decision 4: SysInfo struct in stat package

**Decision:** `type SysInfo struct { Ticks float64; CPUCount int }` defined in
`internal/stat/procpidstat.go`.

**Rationale:** Co-located with `ProcPidStat` / `ProcPidIO` structs; logically part of
the same "system metrics" domain. Recorder and report both import `internal/stat`.

**Alternatives considered:**
- Define in `record` package: report would need to import `record` to decode sysinfo —
  creates an import cycle (report→record→stat). Rejected.
- Define in a new `internal/sysinfo` package: over-engineering for a 2-field struct
  with a single use site. Rejected.

### Decision 5: Local/remote gate in app.setup(), not in filterViews

**Decision:** `app.setup()` removes `procpidstat` from `views` and prints INFO when
`!db.Local`, before passing views to `filterViews()` and the recorder. `NotRecordable:
true` is removed from the static view definition in `view.go`.

**Rationale:** Local/remote is a runtime property, not a static view property.
`filterViews()` handles static unsuitability (version, missing extension); runtime
locality is orthogonal. Keeping them separate avoids coupling procpidstat view config
to recorder-specific logic.

### Decision 6: sysinfo merged into metadata struct

**Decision:** `isFilenameOK` is extended to accept `"sysinfo"` prefix alongside `"meta"`.
`readTar` handles `sysinfo.*` entries identically to `meta.*`, decoding into `SysInfo`
and storing `ticks`/`cpuCount` in the existing `metadata` struct.

**Rationale:** Avoids adding a third boolean flag (`sysinfoOK`) to `readTar` and
extending the `data` channel type. `metadata` is already threaded through the pipeline.
Sysinfo is informational under Option B — absent sysinfo has no effect on report output
(rates are pre-computed strings).

### Decision 7: procpidstat describe text

**Decision:** Description entry: `"Per-process system stats: CPU utilization, IO
activity, and IO delay per PostgreSQL backend. Local mode only."`

**Rationale:** Consistent with other describe entries in the map (one-line, lists key
metrics, notes constraints). Matches the format already used by `activity`,
`statements_timings`, etc.

**Alternatives considered:** Multi-line describe with column-by-column detail — out of
scope for user-spec; other entries don't do this.

### Decision 8: IO/delayacct probes in app.setup(), not per-tick

**Decision:** `CheckIOAvailable` and `CheckDelayAcctAvailable` are called once in
`app.setup()`, stored in `tarConfig`, reused every tick. If no backends are active at
setup time, `CheckIOAvailable` is skipped and `ioAvailable = false`.

**Rationale:** Probe status does not change within a recording session. Per-tick probing
is wasteful and inconsistent with TUI behavior (probed once at screen open).

### Decision 9: PID validation before procfs path construction

**Decision:** Before calling `CheckIOAvailable(pid)` and before constructing
`/proc/[pid]/stat` paths in `collect()`, validate `pid > 0`. Skip PIDs that fail
validation silently (the row will appear in the SQL result with no procfs data).

**Rationale:** `pg_stat_activity` can theoretically return a zero or null PID for
walsender/autovacuum workers before they fully initialize. `readProcPidStat(0)` would
open `/proc/0/stat` (swapper), producing nonsense data. `buildProcPidResult` already
guards `pid > 0` internally, but the outer collection loop must validate before the
filesystem path is constructed.

### Decision 10: WARNING detection in report via column inspection

**Decision:** After the first valid data pair is received in `processData`, scan the
first result's IO columns (9–10: `read_total,KiB`, `write_total,KiB`) and iodelay
column (11: `iodelay_total,s`). If all values in a column set are `""` (empty string,
`Valid=true`), emit a WARNING line before printing the first data row. Check is
one-shot per report run (set a flag after first pair).

**Rationale:** Report has no separate metadata flag for IO/delayacct availability.
The `""` sentinel is the only signal stored in the tar. Column inspection is
consistent with how the TUI communicates availability via the same `""` pattern.
Checking only the first result avoids scanning every snapshot.

## Data Models

```go
// internal/stat/procpidstat.go — new
type SysInfo struct {
    Ticks    float64 `json:"ticks"`
    CPUCount int     `json:"cpu_count"`
}

// report/report.go — extended
type metadata struct {
    version  int
    ticks    float64   // from sysinfo.*; 0 if absent (informational under Option B)
    cpuCount int       // from sysinfo.*; 0 if absent
}

// record/recorder.go — extended
type tarConfig struct {
    filename            string
    append              bool
    isLocal             bool
    ticks               float64
    cpuCount            int
    ioAvailable         bool
    delayAcctAvailable  bool
}

type tarRecorder struct {
    config              tarConfig
    file                *os.File
    fileFlags           int
    writer              *tar.Writer
    // procpidstat stateful fields
    prevProcPidStats    map[int]stat.ProcPidStat
    currProcPidStats    map[int]stat.ProcPidStat
    prevProcPidIO       map[int]stat.ProcPidIO
    currProcPidIO       map[int]stat.ProcPidIO
    lastCollect         time.Time
}
```

## Dependencies

### New packages
None. No new external dependencies.

### Using existing (from project)
- `internal/stat` — `ProcPidStat`, `ProcPidIO`, `buildProcPidResult`, `readProcPidStat`,
  `readProcPidIO`, `CheckIOAvailable`, `CheckDelayAcctAvailable`, `GetSysticksLocal` (newly exported)
- `runtime` (stdlib) — `runtime.NumCPU()`
- `encoding/json` — `json.Unmarshal` for sysinfo decoding in report
- `internal/postgres` — `postgres.DB.Local` field (via `isLocalhost()`)

## Testing Strategy

**Feature size:** M

### Unit tests

- `TestBuildProcPidResultRaw`: verify cols 0–5 are SQL labels, cols 6–11 contain raw
  jiffies/bytes as float strings (no `HH:MM:SS`), col 18 is query.
- `TestFormatProcPidResultForDisplay`: verify cols 6–8 convert to `HH:MM:SS`,
  cols 9–10 to KiB integers, col 11 to `HH:MM:SS`, cols 12–17 to float strings.
- `TestSysInfoRoundTrip`: marshal `SysInfo{Ticks:100, CPUCount:4}` → JSON → unmarshal →
  verify fields match.
- `TestGetSysticksLocal`: call exported function, verify result > 0 (smoke test).
- `TestTarRecorder_WriteSysinfo`: verify sysinfo entry appears in tar with correct JSON.
- `TestFilterViews_NotRecordable` (update): assert procpidstat is NOT filtered out after
  `NotRecordable: false`.
- `Test_filterViews` (update): decrement `wantN` by 1, increment `wantV` by 1 per table row.
- `Test_readMeta_with_sysinfo`: verify `readTar` correctly populates `metadata.ticks`
  and `metadata.cpuCount` when a `sysinfo.*` entry is present.
- `Test_app_doReport_procpidstat`: end-to-end report test using a synthetic tar with
  procpidstat + sysinfo entries; verify non-empty output.

### Integration tests
None — procfs data is non-deterministic in CI (established pattern from features 001/002).

### E2E tests
Agent runs `pgcenter record -c 3 -i 1s -f /tmp/test.tar` on a local PG instance,
then `pgcenter report -N -f /tmp/test.tar` — verifies non-empty output with timestamps.

## Agent Verification Plan

**Source:** user-spec "Как проверить" section.

### Verification approach
Agent runs binary-level verification after build: record 3 snapshots, run report, check output.

### Per-task verification

| Task | verify | What to check |
|------|--------|---------------|
| 01 | bash | `go test ./internal/stat/... -run BuildProcPidResult\|FormatProc\|SysInfo\|GetSysticks` → pass |
| 02 | bash | `go test ./record/... -run FilterViews\|TarRecorder` → pass; `go build ./...` clean |
| 03 | bash | `go test ./report/... -run ReadMeta\|doReport` → pass; `go build ./cmd/...` clean |
| 04 | bash | `make test` → all green; `make lint` → no new warnings |
| Final | bash | Full E2E: record 3 ticks + `report -N` → ≥1 line of output with timestamp |

### Tools required
bash — `make build`, `make test`, `make lint`, `./bin/pgcenter record/report` direct invocation.

## Backward Compatibility

**Breaking changes:** no.

**Migration strategy:** Additive only. Existing tar files continue to work; `report -N`
on an old tar prints INFO "no procpidstat data" and exits cleanly. Existing `-A`, `-X`,
`-D`, `-R`, etc. flags are unchanged.

**DB migration compatibility:** N/A — no database schema changes.

**Consumer impact:**
- `internal/stat.getSysticksLocal` → renamed to `GetSysticksLocal`. No external callers
  (unexported symbol); the one internal caller `NewCollector` in `stat.go` is updated.
- `view.View.NotRecordable` on procpidstat: changes from `true` to `false`. Only
  `filterViews()` in `record/record.go` reads this field; behavior is preserved by the
  new local/remote gate in `app.setup()`.
- `tarRecorder` struct grows new fields — zero-value safe (`nil` maps, `false` bools,
  `0.0` float64). Existing recorder tests use `newTarRecorder(tarConfig{...})` — no
  existing test breaks because new tarConfig fields are zero-valued.

## Risks

| Risk | Mitigation |
|------|-----------|
| MVC split introduces regression in TUI (`Shift+S`) | `buildProcPidResult` public signature unchanged; `TestBuildProcPidResult_*` suite runs on CI; TUI regression is blocking AC |
| tarRecorder stateful maps leak memory on very long recording sessions | `map[int]ProcPidStat` entries are bounded by active PG backend count; backends exit and are removed from curr maps each tick; Go GC handles freed entries |
| `GetSysticksLocal()` fails (`getconf` not in PATH) | Same risk exists in TUI `NewCollector`; accepted by existing code; if it fails, recorder returns error at startup (not silently) |
| report golden tar needs new entries — tests fail until regenerated | `report_test.go` has `-update` flag; task 04 explicitly regenerates golden files as part of its scope |
| First-tick zero rates pollute report output | `processData` always skips first snapshot (`!prevStat.Valid → continue`); verified by existing behavior of all other views |

## Acceptance Criteria

- [ ] `buildProcPidResultRaw` returns numeric float strings in cols 6–11 (not `HH:MM:SS`)
- [ ] `buildProcPidResult` output is unchanged for existing callers (`TestBuildProcPidResult_*` pass)
- [ ] `GetSysticksLocal()` is exported and returns value > 0 on Linux
- [ ] `tarRecorder.collect()` produces `stats["procpidstat"]` with 19 columns when `isLocal=true`
- [ ] `tarRecorder.write()` produces `sysinfo.TIMESTAMP.json` entry in tar
- [ ] `app.setup()` with remote dbConfig: `views["procpidstat"]` absent, INFO printed
- [ ] `report -N` on tar with procpidstat entries: ≥1 data row with timestamp
- [ ] `report -N` on tar without procpidstat: INFO message, exit 0
- [ ] `isFilenameOK` accepts `sysinfo` prefix without error
- [ ] `metadata.ticks` and `metadata.cpuCount` populated from `sysinfo.*` entries
- [ ] `-N` flag accepted by `cmd/report`; `-A` unchanged
- [ ] `report -N -o "%all"` output rows are sorted by `%all` descending within each snapshot
- [ ] `report -N -g "state:active"` output contains only rows where state matches pattern
- [ ] `report -N -l 3` output contains at most 3 rows per snapshot
- [ ] `report -N -s HH:MM -e HH:MM` output excludes snapshots outside the time range
- [ ] `report -N` on tar with empty IO columns: WARNING printed before first data row
- [ ] `report -d -N` outputs procpidstat column descriptions
- [ ] `make test` passes (no new failures)
- [ ] `make lint` passes (no new warnings)
- [ ] `TestFilterViews_NotRecordable` asserts procpidstat passes through filter
- [ ] `Test_filterViews` table counts updated to match new recordable set
- [ ] `Test_app_record` formula updated from `countRecordable()+1` to `countRecordable()+2` (procpidstat via countRecordable + sysinfo extra entry)

## Implementation Tasks

### Wave 1 — Foundation (independent)

#### Task 01: MVC split of buildProcPidResult + export GetSysticksLocal

- **Description:** Extract private `buildProcPidResultRaw` (assembles raw numeric
  values: SQL labels in cols 0–5, jiffies/bytes as float strings in cols 6–11, query
  in col 18) and `formatProcPidResultForDisplay` (converts raw → display: `HH:MM:SS`,
  `%`, `KiB/s`) from `buildProcPidResult`, which becomes their composition. Export
  `getSysticksLocal` → `GetSysticksLocal`. Adds `SysInfo` struct. Add unit tests for
  both new private functions. This establishes the architectural foundation required
  before the recorder can call buildProcPidResult correctly.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/stat/... -run BuildProcPidResult\|FormatProc\|GetSysticks\|SysInfo` → all pass
- **Files to modify:** `internal/stat/procpidstat.go`, `internal/stat/procpidstat_test.go`, `internal/stat/stat.go`, `internal/stat/netdev_test.go`, `internal/stat/diskstats_test.go`, `internal/stat/stat_test.go`
- **Files to read:** `internal/stat/stat.go` (getSysticksLocal, NewCollector, Collector.Update procpidstat block), `internal/stat/netdev_test.go`, `internal/stat/diskstats_test.go` (call sites of getSysticksLocal)

### Wave 2 — Recorder + Report (parallel, independent of each other)

#### Task 02: tarRecorder — stateful procfs enrichment + sysinfo write + local/remote gate

- **Description:** Make `tarRecorder` stateful for procpidstat collection: add prev/curr
  `ProcPidStat`/`ProcPidIO` maps, `lastCollect` timestamp, and locality/availability
  flags to the struct; propagate them from `app.setup()` via `tarConfig`. In
  `collect()`, enrich the procpidstat SQL result with per-tick procfs data using the
  map-rotation protocol (see Architecture data flow). In `write()`, append a
  `sysinfo.TIMESTAMP.json` entry. This makes procpidstat recordable for local-mode
  sessions with correct per-interval rates.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./record/... -run TarRecorder\|FilterViews\|app_record` → pass; `go build ./cmd/pgcenter` → clean
- **Files to modify:** `record/recorder.go`, `record/record.go`
- **Files to read:** `internal/stat/procpidstat.go` (buildProcPidResult, readProcPidStat, readProcPidIO, CheckIOAvailable, CheckDelayAcctAvailable, SysInfo), `internal/stat/stat.go` (Collector.Update procpidstat block — map rotation and enrichment logic), `internal/postgres/postgres.go` (DB.Local field)

#### Task 03: Report pipeline + -N flag + view config

- **Description:** Remove `NotRecordable: true` from `procpidstat` view in `view.go`
  (gate now lives in recorder). Extend `report/report.go`: add `ticks`/`cpuCount` to
  `metadata` struct, add `"sysinfo"` to `isFilenameOK` accepted prefixes, handle
  `sysinfo.*` in `readTar` by decoding `SysInfo` into `metadata`, add `"procpidstat"`
  entry to `describeReport`. In `cmd/report/report.go`, add `showProcPidStat bool` with
  `-N` / `--proc-stats` flag and `case opts.showProcPidStat` in `selectReport`. This
  completes the report-side pipeline so `-N` is a usable flag.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go build ./cmd/pgcenter` → clean; `go test ./report/... -run readMeta\|isFilename` → pass
- **Files to modify:** `internal/view/view.go`, `report/report.go`, `cmd/report/report.go`
- **Files to read:** `report/report.go` (readTar, isFilenameOK, metadata struct, describeReport), `internal/stat/procpidstat.go` (SysInfo struct), `internal/view/view.go` (procpidstat view block)

### Wave 3 — Tests

#### Task 04: Test suite update

- **Description:** Update `record/record_test.go`: invert `TestFilterViews_NotRecordable`
  (including its trailing `assert.True(t, pp.NotRecordable)` which must become `False`);
  update `Test_filterViews` table (`wantN -= 1`, `wantV += 1` per row); update
  `Test_app_record` expected file count (+2 per tick: one procpidstat entry + one sysinfo
  entry). Update `report/report_test.go`: add `Test_app_doReport_procpidstat` and
  `Test_readMeta_with_sysinfo` using synthetic in-memory tar; regenerate
  `pgcenter.stat.golden.tar` if needed. This closes the test coverage gap and ensures
  no silent count regressions from the recorder and report changes in tasks 02–03.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `make test` → all green; `make lint` → no new warnings; `./bin/pgcenter record -c 3 -i 1s -f /tmp/test.tar && ./bin/pgcenter report -N -f /tmp/test.tar` → ≥1 line output
- **Files to modify:** `record/record_test.go`, `report/report_test.go`, `report/testdata/` (golden tar)
- **Files to read:** `record/record.go` (filterViews), `report/report.go` (readTar, processData)

### Final Wave

#### Task 05: Pre-deploy QA

- **Description:** Acceptance testing: run full test suite, verify all acceptance
  criteria from user-spec and tech-spec. Confirm TUI regression (`Shift+S`) is absent.
  Verify backward compat with old tar. Confirm `-A` flag unchanged.
- **Skill:** pre-deploy-qa
- **Reviewers:** none
