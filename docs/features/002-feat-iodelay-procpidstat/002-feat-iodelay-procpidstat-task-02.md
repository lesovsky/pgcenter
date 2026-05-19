---
status: planned
depends_on: ["01"]
wave: 2
skills: [code-writing]
verify: "bash — make test"
reviewers: [dev-code-reviewer, dev-test-reviewer]
# dev-security-auditor omitted — task adds test code and golden fixture files only, no production logic.
---

# Task 02: Add new tests and golden files

## Required Skills

Before starting, load: `/skill:code-writing` — [SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Task 01 extended `procpidstat.go` with `IODelay` field, `CheckDelayAcctAvailable()`, updated
`buildProcPidResult` signature to 19 columns, and updated existing call sites in test files
to compile. This task adds the actual test coverage for the new functionality: five new test
functions, two new golden files, and numeric constant updates in `stat_test.go` and
`record/record_test.go`.

The goal is to make `make test` pass with full coverage of the iodelay code path: parser
(golden file), probe function, and builder (both delayAcctAvailable=true and =false branches).

## What to do

1. Create golden file `internal/stat/testdata/proc/pid_stat_iodelay` — same format as
   `pid_stat_normal_comm` but with `suffix[39]=500`. Field 42 in `/proc/[pid]/stat`
   (0-indexed as `suffix[39]` after stripping `pid` and `comm`) must be `500`.

2. Create golden file `internal/stat/testdata/proc/pid_stat_truncated` — same format as
   `pid_stat_normal_comm` but with only 39 suffix fields (omit field 42 and everything after).
   This tests the `len(suffix) < 40` guard — parser must return `IODelay=0`, no panic.

3. In `internal/stat/procpidstat_test.go`:
   - Update `expectedProcPidCols` from 17 to 19 columns: insert `"iodelay_total,s"` at index 11
     (after `"write_total,KiB"`) and `"%iodelay"` at index 17 (after `"write,KiB/s"`); shift
     `"query"` to index 18.
   - Update all column count assertions from `17` to `19`: lines 142, 146, 181, 182, 220,
     287, 289, 291, 323, 349, 352.
   - Shift all `row[N]` index assertions for the former cols 11–16 to 12–17 and query col 16
     to col 18 in all existing test functions.
   - Add `TestCheckDelayAcctAvailable`: probe against live `/proc/sys/kernel/task_delayacct`
     or use a temp file to cover both true and false branches; assert no panic regardless of
     kernel sysctl state.
   - Add `TestReadProcPidStatIODelay`: read `testdata/proc/pid_stat_iodelay`, assert
     `got.IODelay == 500.0` and `got.Utime == 2500`, `got.Stime == 1250` (values from
     `pid_stat_normal_comm` baseline).
   - Add `TestReadProcPidStatTruncated`: read `testdata/proc/pid_stat_truncated` (39 suffix
     fields), assert `got.IODelay == 0` and no error — the guard returns a valid struct
     with zero IODelay when field 42 is absent.
   - Add `TestBuildProcPidResult_DelayAvailable`: call `buildProcPidResult` with
     `delayAcctAvailable=true` and non-zero `IODelay` in `currStats`; assert col 11
     matches `HH:MM:SS` format and col 17 is a `"%.2f"` decimal string.
   - Add `TestBuildProcPidResult_DelayUnavailable`: call `buildProcPidResult` with
     `delayAcctAvailable=false`; assert `row[11].String == ""` and `row[17].String == ""`.

4. In `internal/stat/stat_test.go`:
   - Rename `TestCollectorUpdateProcPidStat17Cols` to `TestCollectorUpdateProcPidStat19Cols`.
   - Change `Ncols: 17` to `Ncols: 19` in the view literal inside that test.
   - Add `DelayAcctAvailable: true` to the same view literal.
   - Change `assert.Equal(t, 17, ...)` and `assert.Len(t, ..., 17)` assertions to `19`.
   - Fix `assert.NotEqual(t, 17, ...)` in `TestCollectorUpdateNoEnrichment` (line 215) to
     `assert.NotEqual(t, 19, ...)`.

5. In `record/record_test.go`:
   - Line 145: change `assert.Equal(t, 17, pp.Ncols)` to `assert.Equal(t, 19, pp.Ncols)`.

## TDD Anchor

Tests to write before touching implementation (though here the "implementation" is the golden
files — write them first, then write the test functions to verify them):

- `internal/stat/procpidstat_test.go::TestReadProcPidStatIODelay` — reads `pid_stat_iodelay`,
  asserts `IODelay == 500.0`; fails until golden file is created with correct suffix[39].
- `internal/stat/procpidstat_test.go::TestReadProcPidStatTruncated` — reads `pid_stat_truncated`,
  asserts `IODelay == 0`, no error; guards against off-by-one in the `len(suffix) < 40` check.
- `internal/stat/procpidstat_test.go::TestCheckDelayAcctAvailable` — asserts function returns
  a bool without panic; at minimum covers the code path.
- `internal/stat/procpidstat_test.go::TestBuildProcPidResult_DelayAvailable` — asserts
  `row[11]` is `HH:MM:SS` (e.g. `"00:00:01"`) and `row[17]` is `"100.00"` when `delayAcctAvailable=true`
  (prevIODelay=0, currIODelay=100, itv=1.0, ticks=100).
- `internal/stat/procpidstat_test.go::TestBuildProcPidResult_DelayUnavailable` — asserts
  `row[11] == ""` and `row[17] == ""` when `delayAcctAvailable=false`.

## Acceptance Criteria

- [ ] `make test` passes with zero failures
- [ ] `TestReadProcPidStatIODelay` passes: `IODelay == 500.0` from golden file
- [ ] `TestReadProcPidStatTruncated` passes: `IODelay == 0`, no panic, no error
- [ ] `TestCheckDelayAcctAvailable` passes: no panic; result matches live sysctl state
- [ ] `TestBuildProcPidResult_DelayAvailable` passes: col 11 is `"00:00:01"`, col 17 is `"100.00"`
- [ ] `TestBuildProcPidResult_DelayUnavailable` passes: col 11 and col 17 are `""`
- [ ] `TestCollectorUpdateProcPidStat19Cols` passes: result has 19 columns
- [ ] `record.TestFilterViews_NotRecordable` passes: `pp.Ncols == 19`
- [ ] All previously passing tests continue to pass (no regressions)
- [ ] `expectedProcPidCols` in `procpidstat_test.go` has exactly 19 entries in the correct order

## Context Files

**Feature artifacts:**
- [002-feat-iodelay-procpidstat.md](002-feat-iodelay-procpidstat.md) — user-spec
- [002-feat-iodelay-procpidstat-tech-spec.md](002-feat-iodelay-procpidstat-tech-spec.md) — tech-spec
- [002-feat-iodelay-procpidstat-decisions.md](docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-decisions.md) — decisions log

**Project knowledge:**
- [project.md](../../../../.claude/skills/project-knowledge/project.md)
- [architecture.md](../../../../.claude/skills/project-knowledge/architecture.md)
- [patterns.md](../../../../.claude/skills/project-knowledge/patterns.md)

**Code files (read for context):**
- [internal/stat/procpidstat.go](../../../../internal/stat/procpidstat.go) — parser and builder implementation from Task 01
- [internal/stat/procpidstat_test.go](../../../../internal/stat/procpidstat_test.go) — existing tests; add new functions here and update column counts
- [internal/stat/testdata/proc/pid_stat_normal_comm](../../../../internal/stat/testdata/proc/pid_stat_normal_comm) — baseline golden file format; base both new golden files on this
- [internal/stat/stat_test.go](../../../../internal/stat/stat_test.go) — rename test, update Ncols and assertions
- [record/record_test.go](../../../../record/record_test.go) — update Ncols assertion at line 145

## Verification Steps

- Run `make test` — all tests must pass, zero failures
- Confirm output includes `TestReadProcPidStatIODelay`, `TestReadProcPidStatTruncated`,
  `TestCheckDelayAcctAvailable`, `TestBuildProcPidResult_DelayAvailable`,
  `TestBuildProcPidResult_DelayUnavailable` in the passing list
- Confirm `TestCollectorUpdateProcPidStat19Cols` (renamed from `17Cols`) appears and passes
- Confirm `record.TestFilterViews_NotRecordable` passes with updated `Ncols == 19`

## Details

**Golden file `pid_stat_iodelay`:**
Based on `pid_stat_normal_comm` which currently reads:
```
5678 (bash) S 1 5678 5678 0 -1 4194304 200 0 0 0 2500 1250 0 0 20 0 1 0 2000 16384000 200 18446744073709551615 1 1 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0 0 0 0 0 0 0 0 0
```
The suffix (after the last `)`) is split on whitespace. Index mapping (0-based after stripping
`pid` and `comm`):
- suffix[11] = utime = `2500`
- suffix[12] = stime = `1250`
- suffix[39] = delayacct_blkio_ticks — currently `0` in `pid_stat_normal_comm`

Count the suffix fields in `pid_stat_normal_comm`: `S 1 5678 5678 0 -1 4194304 200 0 0 0 2500
1250 0 0 20 0 1 0 2000 16384000 200 18446744073709551615 1 1 0 0 0 0 0 0 0 0 0 0 17 0 0 0
0 0 0 0 0 0 0 0 0 0 0`. To create `pid_stat_iodelay`, change the value at suffix[39] from `0`
to `500`, keeping all other fields identical.

**Golden file `pid_stat_truncated`:**
Same line as `pid_stat_normal_comm` but with only 39 suffix fields — stop before suffix[39].
The parser's `len(suffix) < 40` guard must return `IODelay=0` without error. Utime and Stime
(at suffix[11] and suffix[12]) are still present and can be asserted normally in the test.

**`procpidstat_test.go` — column index shift details:**
After inserting `iodelay_total,s` at index 11, all former columns 11–18 shift by one:

| Former index | Former column    | New index | New column         |
|-------------|------------------|-----------|--------------------|
| 11          | `%all`           | 12        | `%all`             |
| 12          | `%us`            | 13        | `%us`              |
| 13          | `%sy`            | 14        | `%sy`              |
| 14          | `read,KiB/s`     | 15        | `read,KiB/s`       |
| 15          | `write,KiB/s`    | 16        | `write,KiB/s`      |
| 16          | `query`          | 18        | `query`            |
| (new)       | —                | 17        | `%iodelay`         |

After inserting `%iodelay` at index 17, `query` moves to 18.

Go through every `row[N]` assertion in every existing test function and apply this shift.
Pay special attention to `TestBuildProcPidResult_InvalidPID` which asserts `row[9]`, `row[10]`,
`row[11]` (IO totals, becomes IO total for `read`, `write` and then iodelay) and `row[14]`,
`row[16]` (rate columns and query) — these all need updating.

**`TestCheckDelayAcctAvailable` — probe approach:**
Two acceptable approaches:
1. Call `CheckDelayAcctAvailable()` directly and assert the result equals the live sysctl
   value (`cat /proc/sys/kernel/task_delayacct`). This tests real behavior but depends on
   kernel configuration.
2. Write a temp file with `"1\n"` to exercise the true branch, then write `"0\n"` and a
   non-existent path for the false branch. This requires the function to accept a path
   parameter — only do this if Task 01 made `CheckDelayAcctAvailable` accept a path.
   If the function reads `/proc/sys/kernel/task_delayacct` hardcoded (per tech-spec), use
   approach 1 and also test that the function returns false for a missing path by temporarily
   calling `os.Remove` in a test-only subtest (not recommended) or just test the live state.

The simplest compliant approach: call `CheckDelayAcctAvailable()`, read the live sysctl value
separately via `os.ReadFile`, and assert equality. This is what `TestCheckIOAvailable` does
for its analogous probe — follow the same pattern.

**`TestBuildProcPidResult_DelayAvailable` — concrete values:**
Use `ticks=100`, `itv=1.0`, `cpuCount=4`, `IODelay=100` in `currStats` and `IODelay=0` in
`prevStats`. Expected: col 11 = `formatCPUTime(100, 100)` = `"00:00:01"`, col 17 = `"100.00"`
(delta=100 / (1.0*100) * 100 = 100.00).
Choose values that produce a deterministic, non-zero result.

**Dependencies:**
- Task 01 must be complete before starting this task. Task 01 must have:
  - Added `IODelay float64` to `ProcPidStat`
  - Added `CheckDelayAcctAvailable() bool` to `procpidstat.go`
  - Updated `buildProcPidResult` signature with `delayAcctAvailable bool` parameter
  - Grown `procPidResultNcols` to 19 and `procPidResultCols` to 19 entries
  - Updated existing call sites in `procpidstat_test.go` to compile (parameter added but
    no new test functions yet)
  - Updated `Ncols: 19` in `internal/view/view.go` and `internal/stat/stat.go`

**Edge cases:**
- `pid_stat_truncated` must have exactly 39 suffix fields — not 38, not 40. The guard is
  `len(suffix) < 40`, so 39 fields means the check triggers and `IODelay` stays 0.
- In `TestBuildProcPidResult_DelayAvailable`, use `havePrevCPU=true` (provide both `prevStats`
  and `currStats`) and `itv > 0` so the rate path executes for col 17.
- `%iodelay` is NOT normalized by `cpuCount`. Formula: `ΔIODelay / (itv × ticks) × 100`.
  If `delayDelta=100`, `itv=1`, `ticks=100`: result is `100.00`, not `25.00` even with
  `cpuCount=4`. Verify this explicitly in `TestBuildProcPidResult_DelayAvailable`.

## Reviewers

- **dev-code-reviewer** → `docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-task-02-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-task-02-dev-test-reviewer-review.json`

Note: dev-security-auditor omitted — task adds test code and golden fixture files only, no production logic.

## Post-completion

- [ ] Write a brief report to [002-feat-iodelay-procpidstat-decisions.md](002-feat-iodelay-procpidstat-decisions.md) (summary: 1-3 sentences, review rounds with links to JSON, no finding tables or dumps)
- [ ] If deviated from spec — describe deviation and reason
- [ ] Update user-spec/tech-spec if anything changed
