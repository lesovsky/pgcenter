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

## Task 04: Describe the new columns and their caveats

**Summary:** `pgStatActivityDescription` now lists all 17 PG 13+ columns in the order of
`query.PgStatActivityPG13` (row order transcribed from `internal/query/activity.go`, not from the
spec table), carries a `Note:` line for the PG 13+ boundary and a `Caveats:` block with the four
required caveats. Two new tests pin it: `Test_describeActivityColumnOrder` (presence-then-order over
the 17 rows) and `Test_describeActivityCaveats` (the note plus one subtest per caveat).

**Marker anchoring — deviation from the copied sample.** `Test_describeProgressColumnOrder` uses
`"\n- name"` markers; the activity block needs `"\n- name\t"` as well, because `"\n- query"` matches
the `- query_age` row first and the ordering check would then compare an offset that is not the one
under test. The trailing tab is the only difference from the sample; the `require.NotEqual(t, -1, …)`
presence check is kept verbatim.

**Caveat assertions.** Each caveat is a subtest asserting the phrases specific to its *claim*
(`leader is derived` + `leader_pid`; `prepared transactions` + `replication slots` +
`standby feedback`; `age(backend_xmin)` + `pg_last_committed_xact()` + `replication report`;
`not a proof` + `unprivileged`), so a rewrite that keeps the word "Note" but drops the assertion
still reddens.

**Red step.** `Test_describeActivityColumnOrder` failed on `description must contain a row for
"leader"`; `Test_describeActivityCaveats` failed on the PG 13+ note plus all four subtests. Both
green after the edit; `Test_describeReport` and `Test_describeProgressColumnOrder` untouched and
green throughout.

**"Delete a caveat" check (observed).** Removing the four privilege-caveat lines from `describe.go`
and re-running `go test ./report/ -run Test_describeActivityCaveats`:

```
--- FAIL: Test_describeActivityCaveats/an_empty_cell_may_mean_missing_privileges
    caveat "an empty cell may mean missing privileges" is missing or reworded: no "not a proof" in the block
    caveat "an empty cell may mean missing privileges" is missing or reworded: no "unprivileged" in the block
```

Only that subtest reddened, and its message names the caveat. Reverted afterwards.

**Rendering.** Verified by eye via `go run ./cmd report -d -A | expand -t 8`: the three new rows keep
`origin` at column 16 and `description` at column 40 in line with the fourteen existing ones. Caveat
prose wraps inside 80 columns; the bullets use `*` rather than `-` so they cannot collide with the
`"\n- "` row markers.

**Scope.** `internal/stat/help.go` not touched (Decision 8) — confirmed by an empty
`git diff internal/stat/help.go`. Code diff is `report/describe.go` and `report/report_test.go` only;
no golden changed (`git diff --name-only` shows no `report/testdata/`). `make lint` green (0 issues,
gosec clean), full `go test -count=1 ./report/...` green.

**Deviations:** the marker tab suffix described above; nothing else.

**Reviews:** pending — see task file for reviewer report paths.

## Task 05: Tech debt register and the selector inventory line

**Summary:** Five new Active entries — `[022]` dead-and-stale `internal/stat/help.go`, `[023]` one
`horizon_xacts` name over two formulas, `[024]` a test port map promising clusters the image lacks,
`[025]` `PGresult.sort` without a bounds check on its key, `[026]` report error paths that leave the
reader blocked — plus `[021]` moved to Resolved with the three resets and the restored zero-width
guard described from `report/report.go` rather than from the spec, `[020]`'s justification rebuilt on
what `PGresult.validate` actually checks, and the `SelectStatActivityQuery` line in
`architecture.md` rewritten to the PG 9.6 / PG 10 / PG 13 reality. Every claim was read out of the
code: constant names and the replication-vs-activity naming split in `help.go`, both horizon formulas
and the `track_commit_timestamp` gate, the thirteen port mappings against `testing/Dockerfile`'s
PG 14–19, and the two panics the task-03 security review reproduced end to end.

**Two entries beyond the three the task file named.** `[025]` and `[026]` come from the task-03
security audit, which recommended registering rather than widening that task. They are recorded here
because both are consequences the feature leaves behind: the restored seed `OrderKey` re-enters the
unguarded sort path (no new failure class, but no closure either), and the restored zero-width guard
converts a panic into a returned error inside a pipeline that then hangs — a silent failure mode
worse than the crash it replaced, and worth stating plainly next to the entry that celebrates the
guard.

**`[020]` stays open on a different reason than before.** The old text claimed unreachability from a
guarantee that does not exist; the register now names the actual boundary (`validate` checks row
width against `len(Cols)` and `Nrows` against the decoded rows, and neither `Ncols` against `Cols`
nor width across adjacent samples) and the actual reason for deferral (`activity` runs `DiffIntvl
{0,0}`, so `diff()` is never called on the screen this feature touches). Status, severity and `What`
untouched, as specified. Both cross-references between `[020]` and `[021]` were rewritten so neither
points at a section its target has left.

**Scope.** Documentation only, no code touched. `git diff --name-only` outside the feature directory
shows `docs/tech-debt.md` and `.claude/skills/project-knowledge/architecture.md` from this task
(`report/describe.go` and `report/report_test.go` in the same tree belong to task 04).

**Deviations:** none from the task file; the two additional entries are an addition to it, not a
departure from it.

**Reviews:** pending — see task file for reviewer report paths.
