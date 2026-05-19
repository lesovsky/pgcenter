---
status: planned
depends_on: []
wave: 1
skills: [code-writing]
verify: "bash — make build && make lint"
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
---

# Task 01: Extend procpidstat stat layer and screen handler

## Required Skills

Before starting, load:
- `/skill:code-writing` — [SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Extend the procpidstat data layer (`internal/stat/procpidstat.go`) with IO delay accounting support sourced from `/proc/[pid]/stat` field 42 (`delayacct_blkio_ticks`). The current screen has 17 columns; after this task it will have 19 — two new columns `iodelay_total,s` (index 11, accumulated HH:MM:SS) and `%iodelay` (index 17, per-interval rate).

The task covers the full vertical slice of changes that must land together to keep the codebase compilable: struct extension, parser update, new probe function, column definitions, `buildProcPidResult` signature change, wiring through `view.View` and `stat.go`, the 4-branch warning logic in `top/config_view.go`, and call-site fixes in `procpidstat_test.go`. The test file changes in this task are limited to adding the new `delayAcctAvailable` parameter to existing `buildProcPidResult` call sites — new iodelay-specific test functions are written in Task 2.

`make lint` runs golangci-lint which compiles `_test.go` files; call-site updates in the test file are therefore required for the verification command (`make build && make lint`) to pass.

## What to do

1. **`internal/stat/procpidstat.go` — struct and parser.** Add `IODelay float64` field to `ProcPidStat`. In `readProcPidStatFile`, after parsing `stime` (currently at `suffix[12]`), add a guard `if len(suffix) >= 40` and parse `suffix[39]` as `IODelay`. The current guard is `len(suffix) < 13`; do not change that existing guard — add a separate one for the new field only.

2. **`internal/stat/procpidstat.go` — probe function.** Add `CheckDelayAcctAvailable() bool` that opens `/proc/sys/kernel/task_delayacct`, reads up to 4 bytes, and returns `true` iff `strings.TrimSpace(string(buf[:n])) == "1"`. Return `false` on any error. Use a fixed-size `[4]byte` buffer (not `os.ReadFile`) consistent with the defensive procfs reading pattern in the existing parsers.

3. **`internal/stat/procpidstat.go` — column definitions.** Update `procPidResultCols` from 17 to 19 names by inserting `"iodelay_total,s"` at index 11 and `"%iodelay"` at index 17, shifting `"query"` to index 18. Update the constant `procPidResultNcols` from `17` to `19`. Update the package-level comment on `buildProcPidResult` to reflect 19 columns and the new parameter.

4. **`internal/stat/procpidstat.go` — `buildProcPidResult` signature and body.** Add `delayAcctAvailable bool` as a new parameter after `ioAvailable bool`. Shift the existing cols 11–16 (%all/%us/%sy, read,KiB/s, write,KiB/s) to cols 12–17. Shift the query column from index 16 to 18. Insert col 11 (`iodelay_total,s`) and col 17 (`%iodelay`) rendering using the four-branch table from the tech-spec. Guard `ticks > 0` before every `formatCPUTime(curr.IODelay, ticks)` call to prevent `int64(+Inf) = MinInt64` corruption. Use `delta(prevCPU.IODelay, curCPU.IODelay)` for the `%iodelay` rate (no `cpuCount` division — IO delay is per-process wall-clock time, not CPU utilization).

5. **`internal/view/view.go` — `View` struct.** Add `DelayAcctAvailable bool` field after `IOAvailable bool`. In `New()`, update the `"procpidstat"` entry: change `Ncols: 17` to `Ncols: 19`. No other view entries need changes.

6. **`internal/stat/stat.go` — `Collector.Update`.** In the `CollectProcPidStat` branch, add `view.DelayAcctAvailable` as the new argument when calling `buildProcPidResult`, positioned after `view.IOAvailable`.

7. **`top/config_view.go` — `switchViewToProcPidStat`.** After the existing `ioErr := stat.CheckIOAvailable(probePID)` call, add `delayAcctAvailable := stat.CheckDelayAcctAvailable()`. Set `v.DelayAcctAvailable = delayAcctAvailable` on the view before sending it on `viewCh`. Replace the current 2-branch if/else for `printCmdline` with a 4-branch if/else: `!ioAvailable && !delayAcctAvailable` → combined message; `!ioAvailable` → existing IO-only message; `!delayAcctAvailable` → delayacct-only message; else → `printCmdline(g, "%s", v.Msg)`.

8. **`record/record.go` — comment update.** In `filterViews`, update the comment `"7 of 17 columns"` to `"7 of 19 columns"`.

9. **`internal/stat/procpidstat_test.go` — call-site updates only.** Add `false` as the `delayAcctAvailable` argument to every existing `buildProcPidResult` call in the test file. Do not add new test functions — those belong to Task 2.

## TDD Anchor

Write these failing tests first, then implement until they pass. All tests are in `internal/stat/procpidstat_test.go` (call-site parameter additions are part of the compilation fix, not new tests).

- `internal/stat/procpidstat_test.go::TestCheckDelayAcctAvailable` — call `CheckDelayAcctAvailable()` against the live `/proc/sys/kernel/task_delayacct` path; assert that the function returns a bool without panicking, and that the value matches the actual sysctl state (the test can read the file independently to verify). This test must compile after step 2 of implementation.
- `internal/stat/procpidstat_test.go::TestBuildProcPidResult_DelayUnavailable` — call `buildProcPidResult` with `delayAcctAvailable=false`, assert that `result.Ncols == 19`, `result.Values[0][11].String == ""`, and `result.Values[0][17].String == ""`. Must compile and fail before step 4 (function still has 17 cols), then pass after.

Note: the two additional iodelay tests (`TestReadProcPidStatIODelay`, `TestReadProcPidStatTruncated`, `TestBuildProcPidResult_DelayAvailable`) and their golden files are written in Task 2.

## Acceptance Criteria

- [ ] `make build` succeeds on the feature branch with no compilation errors
- [ ] `make lint` passes clean — golangci-lint compiles `_test.go` files, so all call-site updates must be in place
- [ ] `procPidResultNcols == 19` and `procPidResultCols` has 19 entries with `"iodelay_total,s"` at index 11 and `"%iodelay"` at index 17
- [ ] `ProcPidStat` struct has `IODelay float64` field
- [ ] `CheckDelayAcctAvailable()` function exists and returns `false` when `/proc/sys/kernel/task_delayacct` is absent or contains `"0"`
- [ ] `buildProcPidResult` has signature with `delayAcctAvailable bool` parameter after `ioAvailable bool`
- [ ] With `delayAcctAvailable=false`: col 11 and col 17 in result rows are `""` (empty valid NullString)
- [ ] With `delayAcctAvailable=true` and a valid PID with known IODelay: col 11 is a `HH:MM:SS` string, col 17 is a `"%.2f"` string
- [ ] `view.View` struct has `DelayAcctAvailable bool` field; `"procpidstat"` view entry has `Ncols: 19`
- [ ] `Collector.Update` passes `view.DelayAcctAvailable` to `buildProcPidResult`
- [ ] `switchViewToProcPidStat` calls `stat.CheckDelayAcctAvailable()`, sets `v.DelayAcctAvailable`, and uses a 4-branch if/else for `printCmdline` (no double calls on any code path)
- [ ] `record/record.go` comment reads `"7 of 19 columns"`
- [ ] All existing `buildProcPidResult` call sites in `procpidstat_test.go` compile with the added `false` argument

## Context Files

**Feature artifacts:**
- [002-feat-iodelay-procpidstat.md](docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat.md) — user-spec
- [002-feat-iodelay-procpidstat-tech-spec.md](docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-tech-spec.md) — tech-spec
- [002-feat-iodelay-procpidstat-decisions.md](docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-decisions.md) — decisions log

**Project knowledge:**
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, procpidstat hybrid view description, IO availability probe pattern
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — `printCmdline` mutual exclusion rule, hybrid view wiring steps

**Code files:**
- [internal/stat/procpidstat.go](internal/stat/procpidstat.go) — primary file: `ProcPidStat`, `readProcPidStatFile`, `CheckIOAvailable`, `buildProcPidResult`, `procPidResultCols`
- [internal/stat/stat.go](internal/stat/stat.go) — `Collector`, `Collector.Update`, `CollectProcPidStat` branch
- [internal/view/view.go](internal/view/view.go) — `View` struct, `New()` with `"procpidstat"` entry
- [top/config_view.go](top/config_view.go) — `switchViewToProcPidStat`
- [record/record.go](record/record.go) — `filterViews` comment
- [internal/stat/procpidstat_test.go](internal/stat/procpidstat_test.go) — call-site updates only

## Verification Steps

1. Run `make build` — must succeed with zero errors. Binary at `./bin/pgcenter`.
2. Run `make lint` — golangci-lint + gosec must pass clean. This compiles `_test.go` files, so call-site parameter additions are verified here.
3. Optionally run `make test` to catch any regressions in existing tests (existing iodelay test functions from Task 2 are not present yet — that is expected).

## Details

**Files and current state:**

- `internal/stat/procpidstat.go` — `ProcPidStat` has only `Utime` and `Stime` (no `IODelay`). `readProcPidStatFile` parses `suffix[11]` and `suffix[12]` and returns after setting those two fields — `suffix[39]` is never read. `procPidResultCols` has 17 entries, `procPidResultNcols = 17`. `buildProcPidResult` has 8 parameters (no `delayAcctAvailable`); cols 11–13 are CPU rate (%all/%us/%sy), cols 14–15 are IO rate, col 16 is query. `CheckDelayAcctAvailable` does not exist yet.

- `internal/stat/stat.go` — `Collector.Update` in the `CollectProcPidStat` branch calls `buildProcPidResult` with 9 arguments ending with `view.IOAvailable, c.config.ticks, float64(itv), runtime.NumCPU()`. The new `delayAcctAvailable` argument goes between `view.IOAvailable` and `c.config.ticks`.

- `internal/view/view.go` — `View` struct ends with `NotRecordable bool` (after `IOAvailable bool`). The `"procpidstat"` entry in `New()` has `Ncols: 17` and no `DelayAcctAvailable` field. Add `DelayAcctAvailable bool` after `IOAvailable bool`; change `Ncols: 17` to `Ncols: 19`.

- `top/config_view.go` — `switchViewToProcPidStat` currently has a 2-branch if/else at the bottom: `if ioErr != nil { printCmdline(IO warning) } else { printCmdline(v.Msg) }`. Replace with the 4-branch version. The new `delayAcctAvailable` local variable is obtained by calling `stat.CheckDelayAcctAvailable()` (no arguments). Then `v.DelayAcctAvailable = delayAcctAvailable` must be set before `app.config.view = v` and before `app.config.viewCh <- v`. Derive `ioAvailable` from `ioErr == nil` — note that `v.IOAvailable` is already set to `(ioErr == nil)` by the line above; you can use either `ioErr != nil` or `!v.IOAvailable` consistently.

- `record/record.go` — in `filterViews`, the comment on the `NotRecordable` skip block says `"7 of 17 columns"` — change to `"7 of 19 columns"`.

- `internal/stat/procpidstat_test.go` — the file contains multiple calls to `buildProcPidResult` (currently with 9 arguments). Each call needs `false` added as the new `delayAcctAvailable` argument (between `ioAvailable` and `ticks` arguments). Do not add new test functions here.

**Column index mapping (before and after):**

| Index | Before (17 cols) | After (19 cols) |
|-------|------------------|-----------------|
| 0–5 | pid…wait_event (SQL passthrough) | unchanged |
| 6–8 | all_total,s us_total,s sy_total,s | unchanged |
| 9–10 | read_total,KiB write_total,KiB | unchanged |
| 11 | %all | iodelay_total,s (NEW) |
| 12 | %us | %all (shifted) |
| 13 | %sy | %us (shifted) |
| 14 | read,KiB/s | %sy (shifted) |
| 15 | write,KiB/s | read,KiB/s (shifted) |
| 16 | query | write,KiB/s (shifted) |
| 17 | — | %iodelay (NEW) |
| 18 | — | query (shifted) |

**Column rendering rules for new columns (from tech-spec):**

| Condition | col 11 `iodelay_total,s` | col 17 `%iodelay` |
|-----------|--------------------------|-------------------|
| `!delayAcctAvailable` | `""` | `""` |
| `delayAcctAvailable && !validPID` | `"00:00:00"` | `"0.00"` |
| `delayAcctAvailable && validPID && (!havePrevCPU \|\| ticks <= 0)` | `formatCPUTime(curr.IODelay, ticks)` if `ticks>0` else `"0:00:00"` | `""` |
| `delayAcctAvailable && validPID && havePrevCPU && itv > 0 && ticks > 0` | `formatCPUTime(curr.IODelay, ticks)` | `delta(prevCPU.IODelay, curCPU.IODelay) / (itv * ticks) * 100` formatted `"%.2f"` |

Note: `iodelay_total,s` uses `curr.IODelay` (accumulated counter, not delta). `%iodelay` uses the `delta()` helper. No `cpuCount` division — IO delay is per-process wall-clock time (Decision 3 in tech-spec).

**Dependencies:** No new Go packages required. `strings` is already imported in `procpidstat.go`. `os` is already imported.

**Edge cases:**
- `suffix[39]` index: `/proc/[pid]/stat` strips `pid` (field 1) and `comm` (field 2, the parenthesized name) before splitting on whitespace, so `suffix` is 0-indexed starting from field 3 (`state`). `utime` is field 14 (1-indexed) → `suffix[11]`. `delayacct_blkio_ticks` is field 42 (1-indexed) → `suffix[39]`. The guard must be `len(suffix) < 40` to require at least indices 0–39.
- `formatCPUTime(jiffies, ticks)` divides by `ticks` via `int64(jiffies / ticks)`. If `ticks == 0`, this produces `int64(+Inf) = MinInt64` and a corrupt time string. Guard `ticks > 0` before every invocation for the new `IODelay` field.
- `printCmdline` mutual exclusion: exactly one call per execution path through the 4-branch if/else. Do not call `printCmdline` and then fall through to another call.

## Reviewers

- **dev-code-reviewer** → `docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-task-01-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-task-01-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-task-01-dev-test-reviewer-review.json`

## Post-completion

- [ ] Write a brief report to [002-feat-iodelay-procpidstat-decisions.md](docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-decisions.md) (summary: 1-3 sentences, review rounds with links to JSON files, no finding tables or code dumps)
- [ ] If deviated from spec — describe the deviation and reason
- [ ] Update user-spec/tech-spec if anything changed
