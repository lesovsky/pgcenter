---
status: planned
depends_on: []
wave: 1
skills: [code-writing]
verify: "bash — go test ./internal/stat/... -run BuildProcPidResult|FormatProc|GetSysticks|SysInfo → all pass"
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
---

# Task 01: MVC split of buildProcPidResult + export GetSysticksLocal

## Required Skills

Before starting, load:
- `/skill:code-writing` — [SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

This task establishes the architectural foundation for making `procpidstat` recordable.

`buildProcPidResult` in `internal/stat/procpidstat.go` is a single large function that does two things at once: assembles raw numeric values from procfs and SQL (jiffies, bytes, ticks) and converts them into display strings (HH:MM:SS, %, KiB/s). The recorder (task 02) needs to call this function, but it must store display strings computed at collection time — which matches exactly what `buildProcPidResult` already produces. However, the MVC split is needed to keep the two responsibilities cleanly separated so future callers can invoke only the raw assembly step if needed, and so the existing tests continue to cover the public behavior without change.

This task:
1. Extracts `buildProcPidResultRaw` (private) — assembles a 19-col PGresult: cols 0–5 are SQL labels (pid, datname, usename, state, wait_etype, wait_event), cols 6–11 hold raw float strings (utime jiffies, stime jiffies, utime+stime jiffies, read_bytes, write_bytes, iodelay_ticks), cols 12–17 are already-computed rate strings (%all, %us, %sy, read KiB/s, write KiB/s, %iodelay — same calculation as today), col 18 is query text.
2. Extracts `formatProcPidResultForDisplay` (private) — converts a raw PGresult to a display PGresult: cols 6–8 become HH:MM:SS, cols 9–10 become KiB integers (raw bytes divided by 1024), col 11 becomes HH:MM:SS. Cols 12–17 (rate strings) and all other cols pass through unchanged.
3. Makes `BuildProcPidResult` a composition: `return formatProcPidResultForDisplay(buildProcPidResultRaw(...))`.
4. Exports `getSysticksLocal` → `GetSysticksLocal` so the `record` package can call it at startup.
5. Exports `buildProcPidResult` → `BuildProcPidResult`, `readProcPidStat` → `ReadProcPidStat`, `readProcPidIO` → `ReadProcPidIO` so the `record` package (task 02) can call them directly.
6. Defines `SysInfo` struct for use by the recorder (writes it to tar) and the reporter (reads it).
7. Updates all internal call sites of `getSysticksLocal` (in `stat.go`, `netdev_test.go`, `diskstats_test.go`, `stat_test.go`) to use `GetSysticksLocal`. Updates internal call sites of `buildProcPidResult`, `readProcPidStat`, `readProcPidIO` to use their exported names.
8. Adds unit tests for `buildProcPidResultRaw`, `formatProcPidResultForDisplay`, `GetSysticksLocal`, and `SysInfo` JSON round-trip.

## What to do

1. Write the TDD anchor tests in `internal/stat/procpidstat_test.go` (see TDD Anchor section). Run them — they must fail before implementation.

2. In `internal/stat/procpidstat.go`, define `SysInfo` struct with `Ticks float64` (json:"ticks") and `CPUCount int` (json:"cpu_count") fields.

3. In `internal/stat/procpidstat.go`, extract `buildProcPidResultRaw` from the body of `buildProcPidResult`:
   - Same signature as `buildProcPidResult` (all same parameters).
   - Cols 0–5: verbatim SQL labels (unchanged).
   - Cols 6–8: raw float64 strings of jiffies (Utime+Stime, Utime, Stime) — NOT HH:MM:SS. Use `strconv.FormatFloat(..., 'f', 6, 64)` or similar. For invalid PID: `"0"`.
   - Cols 9–10: accumulated IO in bytes as float strings (ReadBytes, WriteBytes from curr). For unavailable/invalid: `""`.
   - Col 11: accumulated iodelay ticks as float string (IODelay from curr). For unavailable: `""`. For invalid PID: `"0"`.
   - Cols 12–17: rates computed exactly as today (float strings, same calculation as current `buildProcPidResult`). These are already display-ready; `formatProcPidResultForDisplay` passes them through unchanged.
   - Col 18: query text (unchanged).

4. In `internal/stat/procpidstat.go`, extract `formatProcPidResultForDisplay` that takes the raw PGresult and returns a display PGresult:
   - Cols 6–8: parse the raw float string, call `formatCPUTime(value, ticks)` → HH:MM:SS string.
   - Cols 9–10: parse the raw bytes float, divide by 1024, format as integer string (KiB). Empty string passthrough.
   - Col 11: parse the raw iodelay ticks float, call `formatCPUTime(value, ticks)` → HH:MM:SS string. Empty string passthrough.
   - Cols 12–17: pass through unchanged (already display strings from `buildProcPidResultRaw`).
   - All other cols (0–5, 18): pass through unchanged.
   - `ticks` must be a parameter since the function needs it for `formatCPUTime`. No `cpuCount` needed — rate cols are already computed.

5. Make `BuildProcPidResult` a thin composition: call `buildProcPidResultRaw(...)`, then `formatProcPidResultForDisplay(rawResult, ticks)`, return the display result.

6. In `internal/stat/procpidstat.go`, export `buildProcPidResult` → `BuildProcPidResult`, `readProcPidStat` → `ReadProcPidStat`, `readProcPidIO` → `ReadProcPidIO`. Update all call sites within `procpidstat.go` to use the exported names (the composition in step 5 becomes `BuildProcPidResult`, etc.).

7. In `internal/stat/stat.go`, rename `getSysticksLocal` → `GetSysticksLocal`. Update the call in `NewCollector` to use the new name.

8. In `internal/stat/netdev_test.go`, `internal/stat/diskstats_test.go`, and `internal/stat/stat_test.go`, replace all calls to `getSysticksLocal()` with `GetSysticksLocal()`.

9. Run all tests — verify all pass including the existing `TestBuildProcPidResult_*` suite.

## TDD Anchor

Write these tests in `internal/stat/procpidstat_test.go` BEFORE implementation. Run, confirm they fail, implement, confirm they pass.

- `internal/stat/procpidstat_test.go::TestBuildProcPidResultRaw` — verifies that `buildProcPidResultRaw` returns a 19-col PGresult where col 0 is the pid string, cols 6–8 are raw float strings (not HH:MM:SS format — no ":" separator), cols 9–10 are raw bytes as float strings (not KiB), col 11 is raw iodelay ticks as float string, col 18 is query text.
- `internal/stat/procpidstat_test.go::TestFormatProcPidResultForDisplay` — verifies that `formatProcPidResultForDisplay` converts a known raw PGresult to a display PGresult: cols 6–8 contain HH:MM:SS strings (e.g. "00:00:01"), cols 9–10 are KiB integers (bytes/1024), col 11 is HH:MM:SS, cols 12–17 are unchanged pass-through strings (already display-ready from raw), col 18 is query text unchanged.
- `internal/stat/procpidstat_test.go::TestSysInfoRoundTrip` — marshal `SysInfo{Ticks: 100, CPUCount: 4}` to JSON, unmarshal back, verify both fields match. Also verify JSON keys are "ticks" and "cpu_count".
- `internal/stat/stat_test.go::TestGetSysticksLocal` — rename of existing `Test_getSysticksLocal`; calls `GetSysticksLocal()`, verifies result > 0 and error is nil (smoke test that the exported symbol exists and works).

## Acceptance Criteria

- [ ] `buildProcPidResultRaw` is defined (private) and returns a 19-col PGresult with float strings in cols 6–11 (no HH:MM:SS in those columns)
- [ ] `formatProcPidResultForDisplay` is defined (private) and converts raw cols 6–8 to HH:MM:SS, cols 9–10 to KiB integers, col 11 to HH:MM:SS
- [ ] `BuildProcPidResult` (exported) signature and behavior is unchanged — all existing `TestBuildProcPidResult_*` tests pass after updating their call sites to `BuildProcPidResult`
- [ ] `SysInfo` struct is defined in `internal/stat/procpidstat.go` with `Ticks float64` (json:"ticks") and `CPUCount int` (json:"cpu_count")
- [ ] `GetSysticksLocal() (float64, error)` is exported and returns value > 0 on Linux
- [ ] `BuildProcPidResult`, `ReadProcPidStat`, `ReadProcPidIO` are exported (capitalized) so task 02's `record` package can call them without import issues
- [ ] All call sites of the former `getSysticksLocal` (stat.go NewCollector, netdev_test.go x3, diskstats_test.go x3, stat_test.go x2) are updated to `GetSysticksLocal`
- [ ] All internal call sites of `buildProcPidResult`, `readProcPidStat`, `readProcPidIO` within `internal/stat` are updated to their exported names
- [ ] `go test ./internal/stat/... -run BuildProcPidResult\|FormatProc\|GetSysticks\|SysInfo` passes
- [ ] `go test ./internal/stat/...` passes (no regressions in the full package)

## Context Files

**Feature artifacts:**
- [003-feat-procpidstat-record-report.md](003-feat-procpidstat-record-report.md) — user-spec
- [003-feat-procpidstat-record-report-tech-spec.md](003-feat-procpidstat-record-report-tech-spec.md) — tech-spec
- [003-feat-procpidstat-record-report-decisions.md](003-feat-procpidstat-record-report-decisions.md) — decisions log

**Project knowledge:**
- [architecture.md](../../.claude/skills/project-knowledge/architecture.md)
- [patterns.md](../../.claude/skills/project-knowledge/patterns.md)

**Code files to modify:**
- [internal/stat/procpidstat.go](../../../../internal/stat/procpidstat.go) — extract `buildProcPidResultRaw`, `formatProcPidResultForDisplay`; add `SysInfo` struct; rename `buildProcPidResult` → `BuildProcPidResult`, `readProcPidStat` → `ReadProcPidStat`, `readProcPidIO` → `ReadProcPidIO`
- [internal/stat/procpidstat_test.go](../../../../internal/stat/procpidstat_test.go) — add `TestBuildProcPidResultRaw`, `TestFormatProcPidResultForDisplay`, `TestSysInfoRoundTrip`
- [internal/stat/stat.go](../../../../internal/stat/stat.go) — rename `getSysticksLocal` → `GetSysticksLocal`; update `NewCollector` call site
- [internal/stat/netdev_test.go](../../../../internal/stat/netdev_test.go) — update 3 call sites from `getSysticksLocal` → `GetSysticksLocal`
- [internal/stat/diskstats_test.go](../../../../internal/stat/diskstats_test.go) — update 3 call sites from `getSysticksLocal` → `GetSysticksLocal`
- [internal/stat/stat_test.go](../../../../internal/stat/stat_test.go) — update 2 call sites from `getSysticksLocal` → `GetSysticksLocal`; rename `Test_getSysticksLocal` → `TestGetSysticksLocal`

## Verification Steps

- Run targeted tests: `go test ./internal/stat/... -run 'BuildProcPidResult|FormatProc|GetSysticks|SysInfo'` — all must pass.
- Run full package tests: `go test ./internal/stat/...` — no regressions.
- Confirm `GetSysticksLocal` is exported: `go vet ./internal/stat/...` — no "undefined" errors.
- Confirm build is clean: `go build ./...` — no compile errors.

## Details

**Files:**

`internal/stat/procpidstat.go` — current state: one large `buildProcPidResult` function (lines 225–371) that assembles SQL labels, reads procfs maps, formats display strings (HH:MM:SS, KiB, %) all in one pass. Also contains `formatCPUTime`, `nullString`, `delta` helpers and `ProcPidStat`/`ProcPidIO` structs. What to add/change:
- Add `SysInfo` struct after the existing struct definitions.
- Extract `buildProcPidResultRaw` with the same parameter list as `buildProcPidResult`. It fills cols 6–8 with raw jiffies as float strings, cols 9–10 with raw bytes as float strings, col 11 with raw IODelay ticks as float string. Cols 12–17 (rate columns) are computed exactly as today and remain as display strings — they don't need a second pass since they are already deltas.
- Extract `formatProcPidResultForDisplay(raw PGresult, ticks float64) PGresult` — converts cols 6–11 from raw floats to display strings. Needs `ticks` to call `formatCPUTime`. Does NOT need `cpuCount` since rate cols 12–17 are passed through unchanged.
- Rename `buildProcPidResult` → `BuildProcPidResult`, `readProcPidStat` → `ReadProcPidStat`, `readProcPidIO` → `ReadProcPidIO`. These are needed as exported symbols so the `record` package (task 02) can call them directly. Update all call sites within the file (the internal composition call, the Collector.Update block in `stat.go` if it calls them, and any test helpers that reference them by old name).
- Rewrite `BuildProcPidResult` as: `raw := buildProcPidResultRaw(...)` then `return formatProcPidResultForDisplay(raw, ticks)`.

`internal/stat/stat.go` — current state: `getSysticksLocal()` at line 372 (unexported). `NewCollector` at line 87 calls it. Change: rename to `GetSysticksLocal`. No signature change. No gosec annotation exists on this function — do not add one.

`internal/stat/netdev_test.go` — current state: 3 calls to `getSysticksLocal()` at lines 14, 37, 106 inside `Test_readNetdevs`, `Test_readNetdevsLocal`, `Test_countNetdevsUsage`. Change: rename all 3 calls to `GetSysticksLocal()`.

`internal/stat/diskstats_test.go` — current state: 3 calls to `getSysticksLocal()` at lines 14, 37, 145 inside `Test_readDiskstats`, `Test_readDiskstatsLocal`, `Test_countDiskstatsUsage`. Change: rename all 3 calls to `GetSysticksLocal()`.

`internal/stat/stat_test.go` — current state: has 2 calls to `getSysticksLocal()` (line 102 inside `TestCollector_collectDiskstats`, and line 115 inside the existing `Test_getSysticksLocal`). Change: rename both calls to `GetSysticksLocal()`. Also rename `Test_getSysticksLocal` to `TestGetSysticksLocal` to match the new exported name convention.

`internal/stat/procpidstat_test.go` — current state: has `TestBuildProcPidResult_*` suite (7 tests), `TestFormatCPUTime`, `TestReadProcPidStat*`, `TestCheckIOAvailable`, `TestCheckDelayAcctAvailable`. Add `TestBuildProcPidResultRaw`, `TestFormatProcPidResultForDisplay`, `TestSysInfoRoundTrip`.

**Edge cases:**

- `buildProcPidResultRaw` must preserve the same "invalid PID" and "unavailable" sentinel values as today: `"0"` for invalid-PID CPU cols, `""` for unavailable IO/iodelay.
- `formatProcPidResultForDisplay`: when a raw col value is `""` (empty sentinel), pass it through as-is without attempting float parse. Only convert non-empty values.
- Rate columns 12–17 in raw result: these are already computed as display-ready float strings in `buildProcPidResultRaw` (they represent deltas, not accumulated totals). `formatProcPidResultForDisplay` must pass them through unchanged.
- `TestBuildProcPidResultRaw` must verify that cols 6–8 do NOT contain ":" to distinguish from HH:MM:SS format.
- The existing `TestBuildProcPidResult_*` tests call `buildProcPidResult` directly — they must be updated to call `BuildProcPidResult` (the exported name). Their assertions remain unchanged; only the call site changes.
- `getSysticksLocal` is called in test files inside the `stat` package (package-internal), so after renaming to `GetSysticksLocal` the test files can call it directly without import changes.

**Dependencies:** No new external packages. This task has no task dependencies (wave 1, independent).

**Implementation hints:**

- In `buildProcPidResultRaw` for cols 6–8: use `strconv.FormatFloat(curCPU.Utime+curCPU.Stime, 'f', 6, 64)` — exact float representation. The `formatProcPidResultForDisplay` will then parse this back and call `formatCPUTime`.
- In `formatProcPidResultForDisplay` for cols 9–10: parse the raw bytes float, divide by 1024, then `strconv.FormatFloat(..., 'f', 0, 64)` (integer KiB). This matches the current `buildProcPidResult` behavior: `strconv.FormatFloat(curIOs.ReadBytes/1024, 'f', 0, 64)`.

## Reviewers

- **dev-code-reviewer** → `docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report-task-01-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report-task-01-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report-task-01-dev-test-reviewer-review.json`

## Post-completion

- [ ] Write a brief report to [003-feat-procpidstat-record-report-decisions.md](003-feat-procpidstat-record-report-decisions.md) (Summary: 1-3 sentences, review rounds with links to JSON, no findings tables or dumps)
- [ ] If deviated from spec — describe the deviation and reason
- [ ] Update user-spec/tech-spec if anything changed
