# Decisions log — 013 feat activity xmin horizon

## Task 02: Sort empty-last and non-empty mode selection

**Summary:** `PGresult.sort` now picks its comparator mode from the first non-empty cell of the
column and orders empty cells after every non-empty one in all three modes and both directions,
with emptiness decided by the rendered `String` and never by `sql.NullString.Valid`. The rule is
written once as a wrapper above the three comparators, `sort.SliceStable` is kept, and a column
with no non-empty cell returns early as a no-op.

**Replay case — variant A chosen.** `Test_app_doReport_ReplSlots_EmptyRetained` places a genuine
`"0"` next to a blank `retained,KiB` under the default `OrderKey 4` DESC sort. Variant A was
picked over B because it reproduces exactly the defect that breaks the feature's primary user
story (`horizon_xacts = 0` colliding with "holds no snapshot") on recorded data, whereas B pins
mode selection, which the unit anchors already cover directly. Both of A's preconditions are set
explicitly in the `curr` tick: the first row carries a non-empty numeric `1024` (so the old code
picks the numeric comparator rather than string mode) and the blank row sits above the `"0"` row
(so under the old code, where both parse to `0` and compare equal, `SliceStable` keeps the blank
first). The `2048`/`1024` pair from the neighbouring test was deliberately avoided — it orders
identically lexicographically and numerically, so it produces no divergence.

**Red step — what each new test showed on the unfixed comparator:**

| Test | Failure on old code |
|------|---------------------|
| `Test_sort_sparse_numeric_firstEmpty` | desc gave `9, 1000000, 100, "", ""` — string mode, lexicographic |
| `Test_sort_sparse_numeric_emptyLast` | asc gave `"", "", 100, 1000000, 9`; desc gave `9, 1000000, 100, "", ""` |
| `Test_sort_empty_not_zero` | asc gave `"", 0, 3, 5` (blank equal to a genuine zero and leading); desc gave `5, 3, "", 0` |
| `Test_sort_sparse_duration` | desc gave `96:58:35, 791:04:45, ...`; asc gave `"", "", 00:05:23, 791:04:45, 96:58:35` |
| `Test_sort_sparse_string` | asc gave `"", "", alpha, beta`; desc already correct, so red was demonstrated on asc |
| `Test_sort_null_and_empty_together` | asc gave `"", "", delta, gamma` (tags `tag1, tag3, tag2, tag0`); desc already correct |
| `Test_sort_fully_empty_column` | **no red step by design** — green before and after; regression pin on the new early return |
| `Test_app_doReport_ReplSlots_EmptyRetained` | `slot_zero` printed at offset 584, `slot_blank` at 408 — the blank ranked above the genuine `0` |

**Golden files:** all existing `report/testdata/` goldens pass without `-update`; `git status
report/testdata/` is clean. As the tech-spec states, that is a regression check only — the corpus
sorts activity by `pid` and its one sparse key runs in the single direction where old and new
behaviour agree.

**Deviations:** none. No new golden was added for the replay case (value-level normalized
assertions only), which the task explicitly permits.

**Reviews:** pending — see task file for reviewer report paths.

## Task 03: Recompute report layout on a mid-archive version change

**Summary:** `processData` now distinguishes "first sample" from "recorded version changed" and, on
the latter only, drops the three states that used to survive the boundary — the alignment flag
(with `ColsWidth`/`Cols`), the header-repeat counter, and the resolved sort index (latch cleared
*and* the view's seed `OrderKey`/`OrderDesc`, captured before the loop, restored). Independently,
`printStatSample` regained the zero-width guard of its twin `top/printDataCell`: inside
`if valuelen > width`, returning `0, fmt.Errorf("zero or negative width, skip")` so the cell is not
printed rather than rendered blank. Closes tech debt [021] (the register entry itself is Task 5's).

**Order of work.** The guard landed first, as the task mandates. Confirmed necessary in practice:
`Test_printStatSample_zeroWidthGuard` reddened as `panic: slice bounds out of range [:-1]` at
`report.go:570`, but contained in the one test written to expect it. With the guard in, the red step
of the three layout tests was an ordinary `FAIL` whose diff showed the defect verbatim — old header
retained, `very_long_database_name` truncated to `very_lo~` by the stale width, row aborting at
index 14.

**Restoring the seed is not optional.** Clearing `orderConfigured` alone leaves `OrderKey` resolved
against the old layout whenever the requested `-o` column is absent from the new one, because
`getColumnIndex` fails and the latch simply stays down. `Test_processData_versionChange_orderColumnMissing`
separates the two: it fails when the seed is not restored even though the latch is cleared.

**Mutation matrix (7, each applied to `report.go`, tests re-run, then reverted):**

| Mutation | Result |
|---|---|
| remove `orderConfigured = false`, keep the seed restore | `reresolvesOrderColumn` FAILS |
| hoist the guard out of `if valuelen > width` | `zeroWidthGuard` FAILS |
| remove the `linesPrinted` reset | `recomputesLayout` FAILS (widths right, header stale) |
| remove the whole order reset | both order tests FAIL |
| clear the latch, do not restore the seed | `orderColumnMissing` FAILS |
| remove the `Aligned` reset | all three version-change tests FAIL |
| remove the guard | `zeroWidthGuard` panics `[:-1]` |

The first two were **not** detected in round 1 and were found by review, not by me: the order
fixture separated `state` from `wait_event` but not from the seed fallback (all three candidate
orders now differ), and no subtest had an empty value at a zero-width column, so guard placement
inside the truncation branch was unpinned. Both were fixed and re-verified independently by the
reviewers.

**Scope of the "load-bearing" claim.** Only `Aligned`, `linesPrinted`, `orderConfigured` and the
seed restore are mutation-observable. `v.ColsWidth = map[int]int{}` / `v.Cols = nil` are not:
`formatStatSample` rewrites both unconditionally once `Aligned` is down, and nothing reads them in
between. They are kept because the task's implementation hints call for them, and the comment was
reworded to state they are hygiene rather than a dependency of any current caller.

**Goldens:** all existing `report/testdata/` goldens pass without `-update`; `git status
report/testdata/` clean.

**Deviations:** none from the task. Three review findings deliberately not applied, each recorded
below as a tech-debt candidate for Task 5 rather than silently dropped.

**Tech-debt candidates surfaced (for Task 5 — `docs/tech-debt.md` untouched by this task):**

1. **`PGresult.sort` does not bounds-check its key** (security audit, major). `r.Values[i][key]` is
   indexed with no check; an archive with `Cols: []` passes `validate()` and panics end-to-end
   through `processData`, *without any version change*. Pre-existing and present at HEAD; the
   restored seed re-enters the unguarded path but introduces no new class. Same DoS family as
   resolved [009]. The auditor explicitly recommended not widening Task 03 for it.
2. **`processData`'s error path hangs the pipeline.** It returns without draining, leaving `readTar`
   blocked and `wg.Wait()` never returning. Shape of every `return err` there, not of this guard —
   but it means a same-version widening archive now hangs where it used to panic. The new test
   helper works around it with a drain goroutine.
3. **`Ncols` decoded from the archive is never cross-checked against `len(Cols)`**, so [020] is
   reachable without a version change — a refinement of this task's claim about [020].
4. Minor/structural: `processData` is now ~152 lines (threshold was already exceeded before this
   task; the project constitution forbids refactoring adjacent working code), and
   `buildActivityTar` duplicates the tar harness in `report_record_replslots_test.go` (extracting it
   would touch a file sibling Task 02 is concurrently editing).

**Reviews:**
- dev-code-reviewer: [round 1](013-feat-activity-xmin-horizon-task-03-dev-code-reviewer-review.json) `changes_required` (1 critical) → [round 2](013-feat-activity-xmin-horizon-task-03-dev-code-reviewer-review-round2.json) `approved_with_suggestions`
- dev-security-auditor: [round 1](013-feat-activity-xmin-horizon-task-03-dev-security-auditor-review.json) `approved` (0 critical; all findings pre-existing and outside the diff)
- dev-test-reviewer: [round 1](013-feat-activity-xmin-horizon-task-03-dev-test-reviewer-review.json) `needs_improvement` (2 major) → [round 2](013-feat-activity-xmin-horizon-task-03-dev-test-reviewer-review-round2.json) `passed`
