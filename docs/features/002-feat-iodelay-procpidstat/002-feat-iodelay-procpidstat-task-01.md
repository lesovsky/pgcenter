---
status: done
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

1. Add `IODelay float64` field to `ProcPidStat` and parse `suffix[39]` (field 42 in `/proc/[pid]/stat`, `delayacct_blkio_ticks`) in `readProcPidStatFile` — guarded so truncated proc files are handled safely.

2. Add `CheckDelayAcctAvailable() bool` probe function that reads `/proc/sys/kernel/task_delayacct` and returns whether delay accounting is active on this kernel.

3. Extend `procPidResultCols` to 19 entries by inserting `"iodelay_total,s"` at index 11 and `"%iodelay"` at index 17; update `procPidResultNcols` constant accordingly.

4. Add `delayAcctAvailable bool` parameter to `buildProcPidResult` after `ioAvailable bool`; render the two new columns using the four-branch logic from the tech-spec (empty when unavailable, formatted when available).

5. Add `DelayAcctAvailable bool` field to `view.View` struct; update the `"procpidstat"` entry in `New()` to `Ncols: 19`.

6. Wire `view.DelayAcctAvailable` through `Collector.Update` to the `buildProcPidResult` call in the `CollectProcPidStat` branch.

7. In `switchViewToProcPidStat`, call `stat.CheckDelayAcctAvailable()`, set `v.DelayAcctAvailable`, and replace the current 2-branch `printCmdline` if/else with a 4-branch one covering all combinations of IO and delay accounting availability.

8. Update the `filterViews` comment in `record/record.go` from `"7 of 17 columns"` to `"7 of 19 columns"`.

9. Add `false` as the `delayAcctAvailable` argument to every existing `buildProcPidResult` call in `procpidstat_test.go` (call-site fix only — new iodelay test functions belong to Task 2).

## TDD Anchor

Write this failing test first, then implement until it passes.

- `internal/stat/procpidstat_test.go::TestBuildProcPidResult_NewSignature` — call `buildProcPidResult` with the new 9-argument signature (adding `delayAcctAvailable bool` after `ioAvailable bool`) and assert that `result.Ncols == 19`. This test will fail to compile until the signature change in step 4 lands, and will fail the assertion until `procPidResultNcols` is updated to 19 in step 3. It drives the core signature and column-count changes for this task.

Note: `TestCheckDelayAcctAvailable`, `TestBuildProcPidResult_DelayUnavailable`, `TestBuildProcPidResult_DelayAvailable`, and golden-file tests belong to Task 2, which focuses on iodelay-specific behavior verification.

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
- [002-feat-iodelay-procpidstat-decisions.md](docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-decisions.md) — decisions log (created by Post-completion of this task)

**Project knowledge:**
- [project.md](.claude/skills/project-knowledge/project.md) — project overview, goals, tech stack
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

**Implementation hints:**

- Implement steps in order: struct field → parser guard → column definitions → `buildProcPidResult` signature → `view.View` field → `Collector.Update` wiring → `switchViewToProcPidStat` 4-branch logic → `record.go` comment → test call-site fixes. This ordering keeps the codebase compilable at each step.
- The column shift (17→19 entries, two inserts) is purely additive — existing column indices 0–10 are unchanged; indices 11–16 shift to 12–18. Verify the mapping table in this file before editing `procPidResultCols`.
- When writing the 4-branch `printCmdline` if/else, derive the two boolean flags (`ioAvailable`, `delayAcctAvailable`) before the block and use them directly; avoid calling `stat.CheckDelayAcctAvailable()` more than once.

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
