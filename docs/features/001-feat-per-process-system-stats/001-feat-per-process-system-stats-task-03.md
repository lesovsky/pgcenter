---
status: planned
depends_on: ["01", "02"]
wave: 2
skills: [code-writing]
verify: "bash — go test ./internal/stat/... -run BuildProcPid|FormatCPU"
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
teammate_name:
---

# Task 03: Result builder, CPU formatter, and PID validation

## Required Skills

Before starting, load:
- `/skill:code-writing` — [SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

This task implements the core data assembly logic for the per-process system stats screen: the result builder that joins PostgreSQL activity data with procfs snapshots, the CPU time formatter, and PID validation.

Task 01 created `ProcPidStat`, `ProcPidIO` structs, and the reader functions (`readProcPidStat`, `readProcPidIO`, `checkIOAvailable`). This task adds `buildProcPidResult()` on top of those primitives — it takes the 7-column `PGresult` from `pg_stat_activity` and two pairs of prev/curr procfs snapshot maps, and produces the final 17-column `PGresult` that flows into the existing rendering pipeline unchanged.

The 17-column output is a hard invariant. Any deviation — even one missing column — causes a panic in `align.SetAlign()` (issue #99 class), because the view config declares `Ncols: 17` and the result must match. The builder must always return exactly 17 columns, even on first tick when no prev data exists.

CPU times are formatted as `HH:MM:SS` (familiar to DBA from `ps` output) via `formatCPUTime(jiffies, ticks float64) string`. Rate columns use the `sValue()` helper from `internal/stat/stat.go` for delta computation with guards for zero `itv`. PID validation via `strconv.Atoi` + `pid > 0` is a security guard that prevents path traversal when building `/proc/%d/stat` paths.

## What to do

1. Add `formatCPUTime(jiffies, ticks float64) string` to `internal/stat/procpidstat.go`. Converts accumulated jiffies to `HH:MM:SS`: `secs = int64(jiffies/ticks)`, then format as `fmt.Sprintf("%02d:%02d:%02d", secs/3600, (secs%3600)/60, secs%60)`.

2. Add `buildProcPidResult()` to `internal/stat/procpidstat.go`. Signature:
   `func buildProcPidResult(activity PGresult, prevStats, currStats map[int]ProcPidStat, prevIO, currIO map[int]ProcPidIO, ioAvailable bool, ticks float64, itv float64, cpuCount int) PGresult`

   The function iterates over activity rows and for each row:
   - Validates PID from col 0 via `strconv.Atoi` + `pid > 0`; on failure sets all procfs columns to `"0"` / `""`
   - Copies 6 SQL columns (pid, datname, usename, state, wait_etype, wait_event) as-is
   - Computes accumulated CPU columns from `currStats[pid]` using `formatCPUTime`
   - Computes IO total columns from `currIO[pid]` — empty string `""` if `!ioAvailable`
   - Computes rate CPU columns: `(Δutime+Δstime) / (float64(itv) * ticks) * 100 / float64(cpuCount)` — `"0"` if no prev entry or `itv == 0`
   - Computes rate IO columns: `ΔReadBytes / float64(itv) / 1024` — `""` if `!ioAvailable`
   - Appends query column (activity col 6) as last column
   - Always produces exactly 17 columns per row, regardless of procfs availability

3. Add unit tests in `internal/stat/procpidstat_test.go`:
   - `TestBuildProcPidResult_FirstTick` — empty prev maps → rate cols `"0"`, accumulated cols computed, Ncols=17
   - `TestBuildProcPidResult_IOUnavailable` — `ioAvailable=false` → IO cols are `""`, CPU cols work normally
   - `TestBuildProcPidResult_ItvZero` — `itv=0` → rate cols `"0"`, no division by zero
   - `TestBuildProcPidResult_NcolsGuarantee` — always returns Ncols=17 for various inputs
   - `TestBuildProcPidResult_TwoTicks` — known prev/curr values → verify exact `%all`, `read,KiB/s`, `all_total,s`
   - `TestFormatCPUTime` — table-driven: `(0, 100)→"00:00:00"`, `(360000, 100)→"01:00:00"`, `(6000, 100)→"00:01:00"`, `(36006000, 100)→"100:10:00"`

## TDD Anchor

Write these tests first, verify they fail, then implement until they pass.

- `internal/stat/procpidstat_test.go::TestFormatCPUTime` — table-driven tests: zero input → `"00:00:00"`, 3600s of jiffies at 100 ticks → `"01:00:00"`, 60s → `"00:01:00"`, 100h+ → correct HH overflow
- `internal/stat/procpidstat_test.go::TestBuildProcPidResult_FirstTick` — empty prev maps → all rate cols are `"0"`, accumulated cols are non-empty, result has Ncols=17 and Nrows matching input
- `internal/stat/procpidstat_test.go::TestBuildProcPidResult_IOUnavailable` — `ioAvailable=false` → `read_total,KiB`, `write_total,KiB`, `read,KiB/s`, `write,KiB/s` are all `""`, CPU cols are populated
- `internal/stat/procpidstat_test.go::TestBuildProcPidResult_ItvZero` — `itv=0` with prev data present → rate cols `"0"`, no panic
- `internal/stat/procpidstat_test.go::TestBuildProcPidResult_NcolsGuarantee` — verify Ncols=17 with varied inputs (zero rows, IO unavailable, first tick, normal tick)
- `internal/stat/procpidstat_test.go::TestBuildProcPidResult_TwoTicks` — known prev/curr Utime/Stime/ReadBytes/WriteBytes, known itv and ticks → verify `%all`, `%us`, `%sy`, `read,KiB/s`, `write,KiB/s`, `all_total,s` exactly match expected values

## Acceptance Criteria

- [ ] `formatCPUTime(0, 100)` returns `"00:00:00"`; `formatCPUTime(360000, 100)` returns `"01:00:00"`
- [ ] `buildProcPidResult` always returns `PGresult` with `Ncols=17`, regardless of input or guard conditions
- [ ] On first tick (empty prev maps), rate columns (`%all`, `%us`, `%sy`, `read,KiB/s`, `write,KiB/s`) are `"0"`; accumulated columns are correctly computed from curr data
- [ ] When `ioAvailable=false`, columns `read_total,KiB`, `write_total,KiB`, `read,KiB/s`, `write,KiB/s` are empty string `""`
- [ ] When `itv=0`, all rate columns are `"0"` (no division by zero)
- [ ] CPU rate columns are normalized by `cpuCount` (use `runtime.NumCPU()` value passed as argument)
- [ ] PID parsed via `strconv.Atoi`; rows with non-numeric or non-positive PID produce `"0"` / `""` for procfs columns but are not skipped entirely
- [ ] Column order in output matches spec: pid, datname, usename, state, wait_etype, wait_event, all_total,s, us_total,s, sy_total,s, read_total,KiB, write_total,KiB, %all, %us, %sy, read,KiB/s, write,KiB/s, query
- [ ] All new tests pass: `go test ./internal/stat/... -run BuildProcPid|FormatCPU`
- [ ] No linter warnings introduced (`make lint`)

## Context Files

**Feature artifacts:**
- [001-feat-per-process-system-stats.md](001-feat-per-process-system-stats.md) — user-spec
- [001-feat-per-process-system-stats-tech-spec.md](001-feat-per-process-system-stats-tech-spec.md) — tech-spec
- [001-feat-per-process-system-stats-decisions.md](001-feat-per-process-system-stats-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](../../.claude/skills/project-knowledge/overview.md) — project overview
- [architecture.md](../../.claude/skills/project-knowledge/architecture.md)
- [patterns.md](../../.claude/skills/project-knowledge/patterns.md)

**Code files to modify:**
- [internal/stat/procpidstat.go](../../../internal/stat/procpidstat.go) — add `formatCPUTime` and `buildProcPidResult` (file created by task 01)
- [internal/stat/procpidstat_test.go](../../../internal/stat/procpidstat_test.go) — add unit tests for new functions

**Code files to read:**
- [internal/stat/stat.go](../../../internal/stat/stat.go) — `sValue()` helper, `PGresult` pattern, `Collector.config.ticks` field
- [internal/stat/postgres.go](../../../internal/stat/postgres.go) — `PGresult` struct definition, `sql.NullString`, how rows/cols are built
- [internal/stat/cpu.go](../../../internal/stat/cpu.go) — `countCPUUsage()` pattern using `sValue()`, reference for rate calculation style

## Verification Steps

1. Run targeted tests: `go test ./internal/stat/... -run 'BuildProcPid|FormatCPU' -v` — all tests must pass, including `TestBuildProcPidResult_FirstTick`, `TestBuildProcPidResult_IOUnavailable`, `TestBuildProcPidResult_ItvZero`, `TestBuildProcPidResult_NcolsGuarantee`, `TestBuildProcPidResult_TwoTicks`, `TestFormatCPUTime`
2. Run the full stat package test suite: `go test ./internal/stat/... -race` — no failures, no races
3. Run linter: `make lint` — no new warnings from the added code

## Details

**Files:**

- `internal/stat/procpidstat.go` — Created by task 01. It already contains `ProcPidStat`, `ProcPidIO` structs, `readProcPidStat(pid int)`, `readProcPidIO(pid int)`, and `CheckIOAvailable()`. Add `formatCPUTime` and `buildProcPidResult` here. The file is in `package stat`.

- `internal/stat/procpidstat_test.go` — Created by task 01. Extend with new test functions for `formatCPUTime` and `buildProcPidResult`.

**Dependencies:**

- Task 01 must be complete before this task — the `ProcPidStat` and `ProcPidIO` types must exist in `procpidstat.go`.
- `sValue(prev, curr, itv, ticks float64) float64` from `stat.go` — computes `(curr-prev)/itv*ticks`, returns 0 if curr <= prev. Use it for per-process CPU rate calculation, but note the CPU rate formula for per-process differs from the system-level CPU formula: per-process rate uses `float64(itv)*ticks` as denominator (refresh interval in seconds × CLK_TCK), not the total CPU time delta.
- `runtime.NumCPU()` from stdlib `runtime` package — pass as `cpuCount int` argument to `buildProcPidResult` (caller will pass `runtime.NumCPU()`; keep the function pure/testable).
- `strconv` is needed in `procpidstat.go` — add to the import block of the new file (each Go file requires its own import). Use `strconv.Atoi` for PID parsing and `strconv.FormatFloat` for rate columns.
- `fmt` — `fmt.Sprintf` for `HH:MM:SS` formatting and float formatting.
- `database/sql` — `sql.NullString{String: v, Valid: true}` is how all values are stored in `PGresult.Values`.

**Column formulas (17 columns in order):**

| Col | Name | Formula |
|-----|------|---------|
| 0 | pid | `activity.Values[row][0]` verbatim |
| 1 | datname | `activity.Values[row][1]` verbatim |
| 2 | usename | `activity.Values[row][2]` verbatim |
| 3 | state | `activity.Values[row][3]` verbatim |
| 4 | wait_etype | `activity.Values[row][4]` verbatim |
| 5 | wait_event | `activity.Values[row][5]` verbatim |
| 6 | all_total,s | `formatCPUTime(curr.Utime+curr.Stime, ticks)` |
| 7 | us_total,s | `formatCPUTime(curr.Utime, ticks)` |
| 8 | sy_total,s | `formatCPUTime(curr.Stime, ticks)` |
| 9 | read_total,KiB | `strconv.FormatFloat(curr.ReadBytes/1024, 'f', 0, 64)` or `""` if `!ioAvailable` |
| 10 | write_total,KiB | `strconv.FormatFloat(curr.WriteBytes/1024, 'f', 0, 64)` or `""` if `!ioAvailable` |
| 11 | %all | `(Δutime+Δstime) / (float64(itv)*ticks) * 100 / float64(cpuCount)`, 2 decimal places, or `"0"` |
| 12 | %us | `Δutime / (float64(itv)*ticks) * 100 / float64(cpuCount)` or `"0"` |
| 13 | %sy | `Δstime / (float64(itv)*ticks) * 100 / float64(cpuCount)` or `"0"` |
| 14 | read,KiB/s | `ΔReadBytes / float64(itv) / 1024`, 2 decimal places, or `""` if `!ioAvailable` |
| 15 | write,KiB/s | `ΔWriteBytes / float64(itv) / 1024`, 2 decimal places, or `""` if `!ioAvailable` |
| 16 | query | `activity.Values[row][6]` verbatim |

**PGresult construction:** The result must have:
- `Ncols: 17` (hard-coded, not derived)
- `Nrows:` number of activity rows (same as `activity.Nrows`)
- `Cols:` slice of 17 column name strings in order: `[]string{"pid", "datname", "usename", "state", "wait_etype", "wait_event", "all_total,s", "us_total,s", "sy_total,s", "read_total,KiB", "write_total,KiB", "%all", "%us", "%sy", "read,KiB/s", "write,KiB/s", "query"}`
- `Values:` `[][]sql.NullString` where each row has exactly 17 `sql.NullString` elements with `Valid: true`
- `Valid: true`

**Edge cases:**

- `itv == 0`: guard before any rate division; set all rate columns to `"0"` or `""` for IO
- No prev entry for a PID (first tick or new backend): rate columns are `"0"` / `""`, accumulated columns still computed from curr
- `!ioAvailable`: cols 9, 10, 14, 15 are `""` (empty string, Valid: true — the NullString has an empty string value, it is not a SQL NULL)
- Invalid PID string (non-numeric or <= 0): skip procfs lookup; set procfs-derived columns to `"0"` for CPU and `""` for IO; still include the row with SQL columns intact
- Backend exited between procfs read and result build: prev/curr maps simply won't have the PID; rate = `"0"`, accumulated = `formatCPUTime(0, ticks)` = `"00:00:00"`

**Implementation hints:**

- `sValue` in `stat.go` divides by `itv * ticks` with the final multiplication by ticks being the rate scaler for system-level CPU. For per-process CPU %, the denominator is `float64(itv) * ticks` (refresh_s × CLK_TCK), and you multiply by `100 / float64(cpuCount)`. Do NOT reuse `sValue` directly for CPU % — compute the delta directly to apply the correct formula.
- For IO rate, denominator is just `float64(itv)` (seconds), and divide by 1024 for KiB/s. You can compute delta as `currIO.ReadBytes - prevIO.ReadBytes` (guard for negative delta → use 0).
- Format float rate values with 2 decimal places: `strconv.FormatFloat(v, 'f', 2, 64)`.
- Keep `buildProcPidResult` pure — it takes all inputs as parameters (no global state, no `runtime.NumCPU()` call inside). The caller passes `runtime.NumCPU()` as `cpuCount`.
- The `PGresult.Cols` slice in the output is not read from `activity.Cols` — set it explicitly to the 17-element name slice. The activity result has only 7 columns.
- Use `sql.NullString{String: value, Valid: true}` for all output cells, including empty strings for IO-unavailable columns.

## Reviewers

- **dev-code-reviewer** → `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-task-03-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-task-03-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-task-03-dev-test-reviewer-review.json`

## Post-completion

- [ ] Write brief report to [001-feat-per-process-system-stats-decisions.md](001-feat-per-process-system-stats-decisions.md) (1-3 sentence summary, review links, no finding tables or code dumps)
- [ ] If deviated from spec — describe deviation and reason
- [ ] Update user-spec/tech-spec if anything changed
