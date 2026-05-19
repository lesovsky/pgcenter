---
status: done
depends_on: ["03", "04"]
wave: 3
skills: [code-writing]
verify: "bash — go test ./internal/stat/... -run TestCollector"
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
teammate_name:
---

# Task 05: Collector integration — snapshot management, enrichment, and Reset()

## Required Skills

Before starting, load:
- `/skill:code-writing` — [SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

This task wires the procfs enrichment pipeline into the `Collector` — the central stats-collection engine in `internal/stat/stat.go`. It is Wave 3, depends on Task 03 (result builder + `readProcPidStat`/`readProcPidIO`) and Task 04 (view registration + `CollectProcPidStat` constant + `view.View.CollectExtra` field).

The task has three parts:

**Part 1 — Collector snapshot fields (`internal/stat/stat.go`):** Add four map fields to the `Collector` struct: `prevProcPidStats`, `currProcPidStats map[int]ProcPidStat` and `prevProcPidIO`, `currProcPidIO map[int]ProcPidIO`. Initialize them as empty non-nil maps in `NewCollector()`. Extend `Collector.Reset()` to clear all four maps so stale PID state is discarded when the user switches views.

**Part 2 — Enrichment branch in `Collector.Update()` (`internal/stat/stat.go`):** After `collectPostgresStat()` returns the 7-column activity `PGresult` and stores it in `s.Pgstat.Result`, check `view.CollectExtra == CollectProcPidStat`. When true, run the full per-PID enrichment sequence: cleanup stale PIDs from prev maps, swap prev←curr, collect procfs data per PID into curr maps, then call `buildProcPidResult()` to produce the 17-column result that replaces `s.Pgstat.Result`. Individual PID errors are skipped silently. The subsequent `calculateDelta()` with `DiffIntvl=[0,0]` acts as identity and leaves the 17-col result intact.

**Part 3 — CollectExtra change-detection in `top/stat.go` (`collectStat()`):** `CollectExtra` is read directly from `view.View` in `Collector.Update()` and does NOT flow through `ToggleCollectExtra`. Therefore, when the user switches away from `"procpidstat"` and back, stale PID maps would produce wrong first-tick values. Fix: in `collectStat()`, add a `prevCollectExtra int` variable alongside the existing `extra` variable. In the `case v = <-viewCh:` block, if `prevCollectExtra != v.CollectExtra`, call `c.Reset()` and update `prevCollectExtra`. This mirrors the existing `ShowExtra` change-detection pattern.

## What to do

1. In `internal/stat/stat.go`, add four map fields to the `Collector` struct:
   - `prevProcPidStats map[int]ProcPidStat`
   - `currProcPidStats map[int]ProcPidStat`
   - `prevProcPidIO    map[int]ProcPidIO`
   - `currProcPidIO    map[int]ProcPidIO`

2. In `NewCollector()`, initialize all four as empty non-nil maps via `make(map[int]ProcPidStat)` / `make(map[int]ProcPidIO)`.

3. In `Collector.Reset()`, assign new empty maps to all four fields (clearing stale PID state).

4. In `Collector.Update()`, after `s.Pgstat.Result = res` (the raw 7-col SQL result), add the `CollectProcPidStat` enrichment branch:
   - Parse the PID list from the activity result (col 0 of each row, `strconv.Atoi`, guard `pid > 0`)
   - Cleanup: rebuild `prevProcPidStats` and `prevProcPidIO` as new maps retaining only PIDs present in the current activity result's curr maps
   - Swap: `prevProcPidStats = newPrevStats`, `prevProcPidIO = newPrevIO`, then `currProcPidStats = make(...)`, `currProcPidIO = make(...)`
   - Collect: for each valid PID, call `readProcPidStat(pid)` → `currProcPidStats[pid]` (skip on error); if `view.IOAvailable`, call `readProcPidIO(pid)` → `currProcPidIO[pid]` (skip on error)
   - Build: `s.Pgstat.Result = buildProcPidResult(res, prevProcPidStats, currProcPidStats, prevProcPidIO, currProcPidIO, view.IOAvailable, c.config.ticks, itv, runtime.NumCPU())`

5. In `top/stat.go`, in `collectStat()`:
   - Declare `prevCollectExtra int` alongside `extra := v.ShowExtra`
   - In `case v = <-viewCh:`, after the `ShowExtra` change-detection block, add: if `prevCollectExtra != v.CollectExtra`, call `c.Reset()`, set `prevCollectExtra = v.CollectExtra`
   - This check is in addition to (not replacing) the existing fallthrough `c.Reset()` + `c.Update()` path for genuine view switches

6. Write tests for the new behavior in `internal/stat/stat_test.go` (or a new `collector_test.go`).

## TDD Anchor

Write these tests first, verify they fail before implementation, then pass after:

- `internal/stat/stat_test.go::TestCollectorResetClearsPIDMaps` — after populating all four PID maps manually via direct struct field assignment (requires test in same package), call `c.Reset()`; assert all four maps have `len == 0` and are non-nil
- `internal/stat/stat_test.go::TestCollectorUpdateNoEnrichment` — call `c.Update()` with a view where `CollectExtra == CollectNone` and a real PG test connection; assert `s.Pgstat.Result.Ncols` does not equal 17 (it equals the column count of the SQL query for that view)
- `internal/stat/stat_test.go::TestCollectorUpdateProcPidStat17Cols` — call `c.Update()` with a view where `CollectExtra == CollectProcPidStat`, `IOAvailable == true`, using a real local PG connection (`postgres.NewTestConnect()`); assert `s.Pgstat.Result.Ncols == 17` and no panic occurs

## Acceptance Criteria

- [ ] `Collector` struct has four new fields: `prevProcPidStats`, `currProcPidStats map[int]ProcPidStat`, `prevProcPidIO`, `currProcPidIO map[int]ProcPidIO`
- [ ] `NewCollector()` initializes all four as non-nil empty maps
- [ ] `Collector.Reset()` clears all four maps (len becomes 0, maps remain non-nil)
- [ ] `Collector.Update()` with `CollectExtra == CollectProcPidStat` produces `s.Pgstat.Result.Ncols == 17`
- [ ] Rate columns are `"0"` on the first tick (prev maps empty)
- [ ] PIDs absent from the current activity result are pruned from prev maps before each swap
- [ ] Per-PID procfs read errors skip that PID's row silently; `Update()` does not return an error because of them
- [ ] `collectStat()` in `top/stat.go` detects `CollectExtra` changes and calls `c.Reset()` on change
- [ ] Existing `ShowExtra` change-detection behavior is unaffected
- [ ] `go test ./internal/stat/... -run TestCollector` passes
- [ ] `make build` succeeds

## Context Files

**Feature artifacts:**
- [001-feat-per-process-system-stats.md](001-feat-per-process-system-stats.md) — user-spec
- [001-feat-per-process-system-stats-tech-spec.md](001-feat-per-process-system-stats-tech-spec.md) — tech-spec
- [001-feat-per-process-system-stats-decisions.md](001-feat-per-process-system-stats-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md)
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md)
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md)

**Code files:**
- [internal/stat/stat.go](../../../internal/stat/stat.go) — Collector struct, NewCollector(), Update(), Reset() — all three modified in this task
- [top/stat.go](../../../top/stat.go) — collectStat() — add CollectExtra change-detection
- [internal/stat/procpidstat.go](../../../internal/stat/procpidstat.go) — readProcPidStat(), readProcPidIO(), buildProcPidResult() — consumed by Update() enrichment branch (produced by Tasks 01 and 03)

## Verification Steps

1. Run targeted tests: `go test ./internal/stat/... -run TestCollector` — all new and existing tests pass
2. Run full stat package: `go test ./internal/stat/...` — no regressions
3. Run full build: `make build` — binary produced, no compile errors
4. Run linter: `make lint` — no new warnings
5. Run full test suite with race detector: `make test` — all pass

## Details

**Files:**

`internal/stat/stat.go` — current state: `Collector` has `prevPgStat`/`currPgStat Pgstat` for SQL snapshot management. `Reset()` clears only those two fields. `Update()` first collects system stats (CPU, disk, net, fs) via the `c.config.collectExtra` switch, then calls `collectPostgresStat()` and `calculateDelta()`. The variable `itv` is `int` (`itv := int(refresh / time.Second)` near the top of `Update()`). `buildProcPidResult` expects `itv float64`, so pass `float64(itv)` at the call site. The enrichment branch goes AFTER `s.Pgstat.Result = res` and BEFORE the existing `c.prevPgStat = c.currPgStat` / `calculateDelta()` block. `c.config.ticks` is the CLK_TCK value stored from `getSysticksLocal()` in `NewCollector()`.

`top/stat.go` — current state: `collectStat()` tracks `extra := v.ShowExtra` and detects changes in `case v = <-viewCh:`. When `extra != v.ShowExtra`, it calls `c.ToggleCollectExtra(extra)` and `continue`s (skipping the full Reset). The full `c.Reset()` + `c.Update()` path runs for all other view changes. For `CollectExtra`, add a separate `prevCollectExtra` variable. The change-detection must fire when the user switches from `"procpidstat"` to another view (CollectExtra 6→0) or back (0→6), calling `c.Reset()` in each case to clear stale PID maps. This `c.Reset()` can be the same call as the one already in the fallthrough path — evaluate whether it needs to be an additional call or whether the existing `c.Reset()` in the fallthrough covers the case.

**Dependencies:**
- Task 03: `buildProcPidResult()`, `readProcPidStat()`, `readProcPidIO()` must exist in `internal/stat/procpidstat.go`
- Task 04: `CollectProcPidStat = 6` constant in `internal/stat/stat.go`; `CollectExtra int` and `IOAvailable bool` fields on `view.View`
- Standard library imports to add to `stat.go` if not already present: `runtime` (for `runtime.NumCPU()`) and `strconv` (for `strconv.Atoi()`)

**Edge cases:**
- `itv == 0`: `buildProcPidResult()` guards against division by zero — rate cols = `"0"`; no special handling needed in `Update()`
- PID string parse failure (`strconv.Atoi` error on activity col 0): skip procfs read for that row, continue to next row
- `readProcPidStat(pid)` error (process exited mid-tick): PID absent from `currProcPidStats`; cleanup will also remove it from `prevProcPidStats` on next tick
- `readProcPidIO(pid)` error when `IOAvailable == true` (race: process exited or different user): PID absent from `currProcPidIO`; `buildProcPidResult()` emits `""` for IO cols of that row
- `view.IOAvailable == false`: skip all `readProcPidIO()` calls; `currProcPidIO` stays empty; IO cols are `""` for all rows
- `calculateDelta()` after enrichment: with `DiffIntvl=[0,0]`, `calculateDelta()` returns the input result unchanged — the 17-col result flows through correctly; no column-count mismatch

**Implementation hints:**
- The cleanup-before-swap pattern from the tech-spec: build `newPrevStats` map by iterating activity PIDs and copying from `currProcPidStats` if present; assign `c.prevProcPidStats = newPrevStats`; then assign `c.currProcPidStats = make(map[int]ProcPidStat)`. Apply same pattern for IO maps. Cleanup before swap, not after.
- `runtime.NumCPU()` call site: inline in the `buildProcPidResult()` call — no need to store in a variable.
- In `top/stat.go`, the `CollectExtra` change-detection check does NOT need to `continue` like the `ShowExtra` check does. The user may switch to a new view that happens to also have a different `CollectExtra` value — in that case the standard fallthrough `c.Reset()` + `c.Update()` already handles it. Add the check to also reset when `CollectExtra` changes even without a full view switch (e.g., future use). At minimum, ensure `c.Reset()` is called when `CollectExtra` changes.

## Reviewers

- **dev-code-reviewer** → `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-task-05-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-task-05-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-task-05-dev-test-reviewer-review.json`

## Post-completion

- [ ] Write report to [001-feat-per-process-system-stats-decisions.md](001-feat-per-process-system-stats-decisions.md) (summary: 1-3 sentences, review round links, no findings dumps)
- [ ] If deviated from spec — describe the deviation and reason
- [ ] Update user-spec/tech-spec if anything changed
