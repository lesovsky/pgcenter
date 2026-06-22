# Decisions Log: record/report for 0.11.0 screens (008)

Отчёты агентов о выполнении задач.

---

## Task 01: Enable recording + fix view/filter count tests

**Status:** Done
**Commit:** 12db931
**Agent:** opus implementer (do-feature, wave 1)
**Summary:** Removed `NotRecordable: true` from the 5 view defs (bgwriter, replslots, stat_io, stat_io_time, statements_jit) so the recorder collects them; updated view_test (5 assertions True→False) and Test_filterViews PG14 counts (11/16→9/18, 5/22→3/24, lower-version rows unchanged); generalized the stale bgwriter comment in record.go. TestNew stays 27; the drop-branch guard test is retained as the sole NotRecordable coverage.
**Deviations:** Нет. Predicted PG14 literals (9/18, 3/24) matched the live filter output exactly (TDD red→green).
**Tech debt:** Нет (new). Observed pre-existing: `record.Test_app_record` panics instead of t.Skipf without a live PG (sibling of [005]'s `Test_doReload`) — to log at /done, not caused by this change.

**Reviews:**

*Round 1:*
- dev-code-reviewer: OK (0 findings) → [task-01-dev-code-reviewer-review.json](008-feat-record-report-0-11-views-task-01-dev-code-reviewer-review.json)
- dev-test-reviewer: OK (0 findings; verified wantN+wantV=27 invariant per row) → [task-01-dev-test-reviewer-review.json](008-feat-record-report-0-11-views-task-01-dev-test-reviewer-review.json)

**Verification:**
- `go test ./internal/view/...` → ok
- `go test ./record/ -run 'Test_filterViews|TestFilterViews_dropsExplicitNotRecordable'` → PASS
- `Test_app_record` panic = no live PG (pre-existing, environmental)

---

## Task 02: Report describe wiring for the 5 new types

**Status:** Done
**Commit:** 351bcb4
**Agent:** opus implementer (code-writing, wave 1)
**Summary:** Added 5 column-description constants to report/describe.go (`pgStatBgwriterDescription`, `pgStatReplicationSlotsDescription`, `pgStatIODescription`, `pgStatIOTimeDescription`, `pgStatStatementsJITDescription`) in the multi-line column/origin/description table style of `pgStatWALDescription`, columns grounded in the actual query layouts (PG14 bgwriter / PG15 JIT baselines); registered them in the `describeReport` map under keys bgwriter/replslots/stat_io/stat_io_time/statements_jit; added 5 matching `Test_describeReport` cases with the invalid fallback kept last.
**Deviations:** Нет.
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved (2 minor optional) → [task-02-dev-code-reviewer-review.json](008-feat-record-report-0-11-views-task-02-dev-code-reviewer-review.json). Applied #1 (added PG18 op_bytes Note to pgStatIODescription for version-note symmetry); #2 was an awareness note, no change required.
- dev-test-reviewer: passed (0 findings) → [task-02-dev-test-reviewer-review.json](008-feat-record-report-0-11-views-task-02-dev-test-reviewer-review.json)

**Verification:**
- `go test ./report/... -run Test_describeReport` → PASS
- `go test ./report/...` → ok
- `go vet ./report/...` → clean; `gofmt -l report/` → empty
- procpidstat col-index const block (report.go ~342-346) untouched (Task 09)

---

## Task 03: CLI report flags for the new screens

**Status:** Done
**Commit:** e44c324
**Agent:** opus implementer (code-writing, wave 1)
**Summary:** Added 3 `options` fields (`showBgwriter bool`, `showReplSlots bool`, `showStatIO string`; JIT reuses existing `showStatements`); registered flags `-B`/`--bgwriter`, `-L`/`--replslots`, `-J`/`--io` (c|t) in `init()` next to `-W`, matching BoolVarP/StringVarP style; extended `selectReport` with bool cases (bgwriter, replslots), a `showStatIO != ""` inner switch (c→stat_io, t→stat_io_time) placed next to the bgwriter/replslots family, and a `case "j": return "statements_jit"` in the existing statements switch. Added 7 `Test_selectReport` cases (5 valid forms + invalid `-J x` and `-X z` resolving to `""`). Short flags B/L/J verified free; returned strings match `view.New()`/`describeReport` keys.
**Deviations:** Нет.
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved (1 minor optional) → [task-03-dev-code-reviewer-review.json](008-feat-record-report-0-11-views-task-03-dev-code-reviewer-review.json). Suggestion (regroup `-J` with other string sub-selectors) skipped — task hint recommends keeping it next to bgwriter/replslots; reviewer confirms current placement defensible.
- dev-test-reviewer: passed (1 minor pre-existing) → [task-03-dev-test-reviewer-review.json](008-feat-record-report-0-11-views-task-03-dev-test-reviewer-review.json). Note (no `t.Run`/case names) skipped — pre-existing table-test pattern across the file, out of scope for minimal change.

**Verification:**
- `go test ./cmd/report/...` → ok
- `go vet ./cmd/report/...` → clean

---

## Task 04: bgwriter record/report golden replay tests (PG14/17/18)

**Status:** Done
**Commit:** b56cb7a
**Agent:** opus implementer (code-writing, wave 2)
**Summary:** Added new test-only file `report/report_record_bgwriter_test.go` (package `report`) with `Test_app_doReport_Bgwriter`, a table-driven golden-based replay test mirroring the `Test_app_doReport_procpidstat` harness (in-memory `tar.NewWriter`, local `writeEntry` closure, meta record whose `version_num` drives report-time `views.Configure`, two ticks 1s apart so itv=1, first tick discarded as prev). Three subcases (PG14/17/18) each build a hand-made `stat.PGresult` matching the exact `SelectStatBgwriterQuery` layout (12 cols [3,10] / 13 cols [6,11] / 14 cols [6,12] incl. `slru_written`) and assert against per-version goldens in `report/testdata/` (`report_record_bgwriter_pg14/17/18.golden`). Reused the package-level `update` flag from report_test.go. Manually re-derived every diff delta in all three goldens (e.g. PG14 buf_ckpt 1500-1000=500, ckpt_write 150.5-100.5=50.00; PG18 slru_written 90-50=40) and confirmed absolute counters + the `stats_age` text column pass through verbatim from tick 2 before locking.
**Deviations:** Test function named `Test_app_doReport_Bgwriter` (capital B, diverging from the lowercase screen-name convention of sibling tests) so the AC-mandated `go test -run Bgwriter` matches; reviewers confirmed this is acceptable.
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved (0 critical, 0 major, 2 minor optional) → [task-04-dev-code-reviewer-review.json](008-feat-record-report-0-11-views-task-04-dev-code-reviewer-review.json). Applied the shared suggestion (added independent `assert.Contains` row + delta sentinel to localize row-suppression regressions, mirroring the procpidstat harness); the naming-casing note was acknowledged as intentional.
- dev-test-reviewer: passed (0 critical, 0 major, 2 minor optional; litmus 3/3) → [task-04-dev-test-reviewer-review.json](008-feat-record-report-0-11-views-task-04-dev-test-reviewer-review.json). Applied the same row-pin suggestion; the ANSI-escape-in-golden note is a residual snapshot-review risk only — no change, follows project-wide golden convention and deltas were hand-verified.

**Verification:**
- `go test ./report/... -run Bgwriter -v` → PASS (pg14/pg17/pg18)
- `go test ./report/...` → ok (no regressions)
- `go vet ./report/...` → clean; `gofmt -l report/report_record_bgwriter_test.go` → empty

---

## Task 05: replslots record/report golden replay test (+ zero-slots)

**Status:** Done
**Commit:** 1dc5498
**Agent:** opus implementer (code-writing, wave 2)
**Summary:** Added new test-only file `report/report_record_replslots_test.go` (package `report`) with two golden-based replay tests mirroring the bgwriter harness. `Test_app_doReport_ReplSlots` builds a synthetic in-memory tar of two cumulative ticks (meta + replslots) for the version-independent `replslots` screen (single PG14 meta suffices: `SelectStatReplicationSlotsQuery` ignores version → Ncols 15, DiffIntvl [6,13]), two slots paired by `slot_name` (UniqueKey default 0): a logical slot whose 8 diffed counters (cols 6-13) grow and a physical slot whose 8 coalesced `"0"` counters diff to clean `"0"` ([007] zero-cell contract, no sample abort). Asserts retained,KiB DESC order (OrderKey 4) via `assert.Less` and pins the exact diff deltas (5/8/15/12/20/30/30/50 and all-zero) independent of the golden via an ANSI-stripped/whitespace-collapsed `assert.Contains`. `OrderColName` deliberately unset so the OrderKey=4 default governs sort. `Test_app_doReport_ReplSlots_empty` replays two zero-row ticks and asserts header-only output (column header present, no `, rate: ` data line, no slot names, no INFO/WARNING — those are procpidstat-only). Manually re-derived every delta in both goldens before locking; the physical-slot row is clean `0` across all 8 cells.
**Deviations:** Test functions named `..._ReplSlots` / `..._ReplSlots_empty` (CamelCase) vs the task's lowercase `..._replslots`, so the AC-mandated `-run ReplSlots` matches both (task hint line 182 requires only the `ReplSlots` substring; report_test.go already mixes casings). Empty case prints column-header-only with NO per-sample timestamp line, because `printStatSample` (which emits the `YYYY/MM/DD ..., rate:` line) never runs for zero rows — the empty-case assertion was adjusted to `NotRegexp` on the timestamp accordingly.
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved (0 critical, 0 major, 3 minor optional) → [task-05-dev-code-reviewer-review.json](008-feat-record-report-0-11-views-task-05-dev-code-reviewer-review.json). All three minor notes optional (test-name casing → logged here; empty-case boilerplate duplication and `time.Now().Location()` → no change, both match existing harnesses).
- dev-test-reviewer: needs_improvement (1 major + 3 minor; litmus 4/5) → [task-05-dev-test-reviewer-review.json](008-feat-record-report-0-11-views-task-05-dev-test-reviewer-review.json). Major: slot_name pairing not distinguished from positional pairing (both ticks listed slots in identical order). Applied: prev tick now lists slots in OPPOSITE order from curr — golden regenerated byte-identical (sha256 unchanged), proving diff() pairs by key. Applied all 3 minors (independent delta-row assertion, `, rate: ` no-rows guard, tightened doReport-scope comment).

*Round 2:*
- dev-test-reviewer: passed (0 findings; litmus 5/5) → [task-05-dev-test-reviewer-review-round2.json](008-feat-record-report-0-11-views-task-05-dev-test-reviewer-review-round2.json). Confirmed the pairing gap is genuinely fixed and all minor fixes correct.

**Verification:**
- `go test ./report/... -run ReplSlots -v` → PASS (replay + empty)
- `go test ./report/...` → ok (no regressions)
- `go vet ./report/...` → clean; `gofmt -l report/report_record_replslots_test.go` → empty
- Manual delta verification: logical_a cols 6-13 = 5/8/15/12/20/30/30/50 (curr-prev, itv=1); physical_b all 8 coalesced-`0` cells diff to clean `0`; retained,KiB DESC holds (2048 before 1024)

---

## Task 06: pg_stat_io record/report golden replay tests (count 16/18 + time)

**Status:** Done
**Commit:** d97007d
**Agent:** opus implementer (code-writing, wave 2)
**Summary:** Added new test-only file `report/report_record_statio_test.go` (package `report`) with three golden-based replay tests mirroring the bgwriter/replslots harness (in-memory two-tick tar, meta `version_num` drives report-time `view.Configure`, ticks 1s apart so itv=1, first tick discarded as prev). `Test_app_doReport_StatIO_v16` and `_v18` replay the version-aware count screen `stat_io` (Ncols=16, DiffIntvl [4,14], UniqueKey 0) at recorded versions 160000/180000; `Test_app_doReport_StatIOTime` replays the version-independent `stat_io_time` (Ncols=10, DiffIntvl [4,8], UniqueKey 0) at 160000. Columns are built strictly from `internal/query/io.go` SELECT order (quoted `"read,KiB"` names preserved). v16 and v18 fixtures are deliberately distinct — different counter/KiB values AND a PG18-only `object='wal'` row — so the goldens are byte-distinct (confirmed via `cmp`/reviewer), not redundant copies. Each case includes coalesced-`"0"` cells in both ticks (bgwriter reads/hits, WAL reads/fsyncs/hits) to exercise the [007] zero-cell contract: deltas come out clean `0` without aborting the sample (`ParseInt("")` crash) or blanking the row. prev rows are listed in OPPOSITE order from curr to prove `diff()` pairs by io_key (UniqueKey 0), not by position. `OrderColName` deliberately unset so the view OrderKey=4 DESC default governs order. wantRows assert computed deltas (ANSI-stripped, whitespace-collapsed) BEFORE the golden equality so the diff math is guarded even if a golden is regenerated against buggy code. Goldens (`report_record_stat_io_v16.golden`, `..._v18.golden`, `..._stat_io_time.golden`) generated via `-update`; every delta manually re-derived before locking (e.g. v16 client reads 1500-1000=500, read,KiB 12000-8000=4000; v18 client reads=700, WAL writes=800, write,KiB=6400; time client read_time 1600-1000=600). Repeat run without `-update` (`-count=2`) deterministic.
**Deviations:** Test functions named `..._StatIO_v16`/`_v18`/`..._StatIOTime` (CamelCase) vs the task's lowercase screen-name convention, so the AC-mandated `-run StatIO` matches (task hint requires only the `StatIO` substring; report_test.go already mixes casings).
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved (0 critical, 0 major, 2 minor optional) → [task-06-dev-code-reviewer-review.json](008-feat-record-report-0-11-views-task-06-dev-code-reviewer-review.json). Column layout/order vs io.go verified clean (16/10 cols, quoted names, DiffIntvl, OrderKey); harness consistent with siblings; no dead code. Both minors no-action (shared ANSI-helper consolidation deferred to post-wave merge; Contains-row safety documented).
- dev-test-reviewer: passed (0 critical, 0 major, 3 minor optional; litmus 3/3) → [task-06-dev-test-reviewer-review.json](008-feat-record-report-0-11-views-task-06-dev-test-reviewer-review.json). Confirmed wantRows independently guard diff math, io_key pairing genuinely tested (non-positional), zero-cell coalesce exercised+asserted, v16/v18 byte-distinct, deterministic. Applied 2 of 3 minors: tightened the header comment to state v16/v18 output divergence is a fixture property (selector-branch correctness lives in view_test.go/internal/query), and added an `orderChecks` `assert.Less` guard pinning bgwriter-before-wal among the v18 reads-delta-0 tie independently of the golden. Skipped #3 (no stat_io empty-case test) — out of scope: shared no-data report.go path already covered by `Test_app_doReport_ReplSlots_empty`.

**Verification:**
- `go test ./report/... -run StatIO -v` → PASS (v16/v18/time)
- `go test ./report/... -run StatIO -count=2` → PASS (deterministic, no `-update`)
- `go test ./report/...` → ok (no regressions)
- `go vet ./report/...` → clean; `gofmt -l report/report_record_statio_test.go` → empty
- Manual delta verification: v16 client cols 4-14 = 500/4000/60/480/20/160/600/4/3/2/2, bgwriter zero-cells clean `0` (writes 60, write,KiB 480, writebacks 15); v18 client = 700/5600/90/720/30/240/900/6/6/4/3, WAL writes 800/write,KiB 6400/extends 200/ext,KiB 1600 rest `0`; time client cols 4-8 = 600/300/60/80/25, bgwriter read_time/fsync_time clean `0`. v16↔v18 goldens byte-distinct (extra WAL row + divergent values).

## Task 07: statements_jit record/report golden replay tests (PG15/17)

**Status:** Done
**Commit:** f63c4df
**Agent:** opus implementer (code-writing, wave 2)
**Summary:** Added new test-only file `report/report_record_statements_jit_test.go` (package `report`) with a table-driven golden replay test (`Test_app_doReport_StatementsJIT`, subcases `version_15`/`version_17`) mirroring the bgwriter harness: a synthetic in-memory two-tick tar (meta + statements_jit + sysinfo, ticks 1s apart so itv=1, first tick discarded as prev) flows through `app.doReport`, with meta `version_num` driving the report-time `view.Configure` → `query.SelectStatStatementsJITQuery` layout switch. The test's core proof is the version-shifted layout: v15 → 13 cols / DiffIntvl {6,10} / UniqueKey 11 (trailing md5 queryid at col 11); v17 → 15 cols / DiffIntvl {7,12} / UniqueKey 13 (deform_total + deform,ms shift the interval block, functions, queryid and query right by one, so the UniqueKey index moves to 13). Each tick is a hand-built `stat.PGresult` with the exact per-version column set from `internal/query/statements.go`; both rows carry the same queryid at the version-specific UniqueKey so `diff()` pairs them. Phase-time `*,ms` columns inside DiffIntvl diff as curr-prev (gen 50, inline 60, opt 90, emit 120, deform 150 on v17, functions 4); the `*_total` text columns pass through verbatim from curr. Goldens `report_statements_jit_v15.golden` (13 cols, no deform) / `report_statements_jit_v17.golden` (15 cols, with deform) generated via `-update` and every delta manually re-derived before locking. The structural significance of the UniqueKey shift is genuinely exercised: had Configure left the default v15 UniqueKey (11) under v17, the queryid would be read from the `emit,ms` column (differing values across ticks), the rows would not pair, and the `*,ms` cells would print as absolute curr values instead of deltas — the delta assertions would fail. `Config.OrderColName` deliberately unset (view OrderKey default governs). Reused the existing `update` flag from `report_test.go` (not re-declared).
**Deviations:** Test function named `..._StatementsJIT` (CamelCase) vs the task's lowercase `..._statements_jit`, so the AC-mandated `-run StatementsJIT` matches (report_test.go already mixes casings). Recorded `version_num` values 150004/170001 used instead of the task's illustrative 150000/170000 — representative patch-level versions that route to the same `< PostgresV17` / `>= PostgresV17` branches; documented with an in-code comment.
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved_with_suggestions (0 critical, 0 major, 3 minor optional) → [task-07-dev-code-reviewer-review.json](008-feat-record-report-0-11-views-task-07-dev-code-reviewer-review.json). Cross-file consistency vs `SelectStatStatementsJITQuery` verified exact; harness consistent with siblings. Applied all 3 minors: anchored the bare-substring delta sentinels to the isolated data line via `assert.Regexp` with word boundaries (fixes `"50"`⊂`"150"` ambiguity on v17, adds the previously golden-only `functions` delta sentinel), switched the v15 deform check to `assert.NotContains`, and added a comment documenting the intentional 150004/170001 version_num choice.
- dev-test-reviewer: passed (0 critical, 0 major, 3 minor optional; litmus 2/2, pyramid healthy) → [task-07-dev-test-reviewer-review.json](008-feat-record-report-0-11-views-task-07-dev-test-reviewer-review.json). Confirmed real end-to-end pipeline with zero mocks, both version goldens correct and human-meaningful, golden equality pins every cell (litmus passes). Same 3 minors as code-reviewer — all applied; goldens byte-unchanged after the assertion-only refinements.

**Verification:**
- `go test ./report/... -run StatementsJIT -v` → PASS (version_15, version_17)
- `go test ./report/...` → ok (no regressions)
- `go vet ./report/...` → clean; `gofmt -l report/report_record_statements_jit_test.go` → empty
- `make test`: only pre-existing `top.Test_doReload` panic (nil DB, needs live PG; confirmed failing on a stashed clean tree) — unrelated to this test-only change; the `report` package is fully green
- Manual delta verification: v15 cols 6-10 = 50/60/90/120/4 (curr-prev, itv=1), `*_total` text pass through; v17 cols 7-12 = 50/60/90/120/150/4 (deform,ms 650-500=150), 15 cols incl deform_total; queryid `a1b2c3d4e5` paired on the version-shifted UniqueKey (11 v15, 13 v17)

---

## Task 08: Tech-debt [007] — behavioral zero-cell diff() test

**Status:** Done
**Commit:** 9ffbc3a
**Agent:** opus implementer (code-writing, wave 3)
**Summary:** Added one test-only function `Test_DiffZeroFilledCells` to `internal/stat/postgres_test.go` (next to `Test_diff`) that pays off the behavioral half of tech-debt [007]. It builds synthetic prev/curr `stat.PGresult` samples imitating recorded coalesced-`"0"` cumulative cells (pg_stat_io / pg_replication_slots) with an io_key-style string UniqueKey at col 0, runs them through the unexported `diff(curr, prev, 1, [2]int{2,3}, 0)` and the public `Compare` wrapper, and asserts: clean `"0"` deltas with no error / no sample abort; non-positional row pairing by UniqueKey (prev rows listed in OPPOSITE order from curr); a mixed row (coalesced-`"0"` reads → `"0"`, normal counter hits 100→150 → `"50"`); skip/copy-as-is for rows present in only one sample; and that an empty `""` in-interval cell errors with `convert '' to int failed` — documenting WHY the SQL coalesce is required (the `ParseInt("")` abort it prevents). The `Compare` assertion uses `desc=true` so it genuinely exercises the wrapper's `sort()` step (diff order a1,b2,d4 → DESC d4,b2,a1). No production code touched — the contract was already correct (coalesce in SQL + integer diff of `"0"`); this test locks the behavior.
**Deviations:** Test named `Test_DiffZeroFilledCells` (capital `Diff`) per the orchestrator's fixed constraint that the name match `-run Diff`. Go's `-run` is case-sensitive, so `-run Diff` selects ONLY the new test, NOT the lowercase siblings `Test_diff` / `Test_diff_pg18_wal_stats_age` — contradicting the task's AC wording that the mask re-runs all three. Resolved as a documented deviation: an in-code NOTE directs readers to `-run '[Dd]iff'` for full regression coverage, and verification was run under both masks (both green). The task body's claim (line 160) that `-run Diff` catches the lowercase siblings is factually incorrect for case-sensitive `-run`; the dual-mask approach preserves the AC's regression-safety intent without renaming.
**Tech debt:** [007] — behavioral half now resolved (this test locks the `diff()`-survives-coalesce-`"0"` contract; the structural `internal/query` coalesce assertion already covered the SQL half). The `docs/tech-debt.md` register itself is updated at /done, not here.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved_with_suggestions (0 critical, 1 major, 1 minor) → [task-08-dev-code-reviewer-review.json](008-feat-record-report-0-11-views-task-08-dev-code-reviewer-review.json). Major (verify mask `-run Diff` case-sensitivity excludes the siblings) → handled as documented deviation (in-code NOTE + dual-mask verification; name fixed by orchestrator). Minor (incidental gofmt comment-realignment churn in `Test_parseDuration`) → applied: reverted those lines so the diff is purely additive (0 deletions).
- dev-test-reviewer: needs_improvement (0 critical, 1 major, 2 minor; litmus 4/4) → [task-08-dev-test-reviewer-review.json](008-feat-record-report-0-11-views-task-08-dev-test-reviewer-review.json). Same major (verify mask) → same documented-deviation handling. Minor #2 (Compare no-op sort) → applied: switched to `desc=true` and assert rows reorder to d4,b2,a1, genuinely exercising `sort()`. Minor #3 (bare `assert.Error`) → applied: tightened to `assert.ErrorContains(t, err, "convert '' to int failed")`, binding the error to the empty cell.

*Round 2:*
- dev-code-reviewer: approved (0 critical, 0 major; 1 minor doc-accuracy note now resolved) → [task-08-dev-code-reviewer-review-round2.json](008-feat-record-report-0-11-views-task-08-dev-code-reviewer-review-round2.json). Confirmed diff is purely additive (89/0) and the documented-deviation handling matches reality (verified `-run Diff` vs `-run '[Dd]iff'` empirically).
- dev-test-reviewer: passed (0 findings; litmus passing) → [task-08-dev-test-reviewer-review-round2.json](008-feat-record-report-0-11-views-task-08-dev-test-reviewer-review-round2.json). Confirmed both minors fixed and the major's documented-deviation mitigation preserves regression-safety intent.

**Verification:**
- `go test ./internal/stat/... -run Diff -v` → PASS (Test_DiffZeroFilledCells)
- `go test ./internal/stat/... -run '[Dd]iff' -v` → PASS (Test_diff, Test_DiffZeroFilledCells, Test_diff_pg18_wal_stats_age, Test_diffPair) — sibling regression coverage
- `go vet ./internal/stat/...` → clean
- Litmus (test-reviewer): neutralizing diff()'s delta computation breaks the mixed-row `"50"` assertion → test is genuinely behavioral, not mock-wiring

---

## Task 09: Tech-debt [004] — export procpidstat col-index constants

**Status:** Done
**Commit:** 06deb96
**Agent:** opus implementer (code-writing, wave 3)
**Summary:** Pure behavior-preserving refactor paying off tech-debt [004]. Exported a named index const block (`ColReadTotalKiB=9`, `ColWriteTotalKiB=10`, `ColIODelayTotalS=11`) in `internal/stat/procpidstat.go` next to `procPidResultNcols`, with a doc-comment tying the values to the `procPidResultCols` order. Deleted the duplicated local const block (and its comment) in `report/report.go` and switched `emitProcPidStatAvailabilityWarnings` to reference `stat.Col*` at all three sites (Ncols guard + two `allEmpty` calls). Updated `report_test.go`'s `mkRow` helper to index via `stat.Col*` so it tracks the single source of truth instead of re-hardcoding 9/10/11. Added one guard test `TestProcPidColIndexConstants` (per code-reviewer suggestion) asserting `procPidResultCols[ColReadTotalKiB] == "read_total,KiB"` (and the two siblings) so a future column reorder fails loudly instead of silently shifting the indices report depends on. No behavior change.
**Deviations:** Beyond the task's stated scope (which named only the two production files), also touched `report/report_test.go` (mandatory — it referenced the now-removed local constants and would not compile otherwise) and added one test (`internal/stat/procpidstat_test.go`) acting on the code-reviewer's optional minor suggestion to lock the comment-only invariant. Both are in keeping with the refactor's intent.
**Tech debt:** [004] resolved — the indices now live in one exported place in `internal/stat` and are referenced by `report`; the `docs/tech-debt.md` register is moved Active→Resolved at /done, not here.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved (0 critical, 0 major, 1 minor optional) → [task-09-dev-code-reviewer-review.json](008-feat-record-report-0-11-views-task-09-dev-code-reviewer-review.json). Verified constant values match `procPidResultCols` order, no behavior change, no leftover dead references, no import cycle. Minor (optional): add an invariant test mapping the constants back to the slice — applied as `TestProcPidColIndexConstants`.
- dev-test-reviewer: passed (0 findings; litmus 6/6, pyramid healthy) → [task-09-dev-test-reviewer-review.json](008-feat-record-report-0-11-views-task-09-dev-test-reviewer-review.json). Confirmed WARNING-path coverage unchanged, assertions not weakened, and that using `stat.Col*` in the test is correct (not a tautology) for an index-mapping refactor — hardcoding 9/10/11 would re-introduce the very duplication [004] removes.

**Verification:**
- `go test ./report/... -run Test_emitProcPidStatAvailabilityWarnings -v` → PASS (6 subtests)
- `go test ./internal/stat/... -run TestProcPidColIndexConstants -v` → PASS
- `go test ./report/...` → ok (no regressions)
- `go vet ./report/... ./internal/stat/...` → clean; `gofmt -l` on all four changed files → empty
- Pre-existing/unrelated: `go test ./internal/stat/...` (whole package) and `top.Test_doReload` panic on a nil DB (need live PG) — confirmed environmental, untouched by this task; the `report` package and all procpidstat parser/build tests are green

---

## Task 10: Update project docs for record/report of 0.11.0 screens

**Status:** Done
**Commit:** 4c7fbd0
**Agent:** opus implementer (documentation-writing, wave 3)
**Summary:** Docs-only change bringing three knowledge files in line with the now-shipped record/report support. `overview.md`: dropped the "TUI-only, not recordable in 0.11.0" tails on bgwriter/replslots/pg_stat_io and the JIT pgss fragment, replacing them with the report flag each screen is reachable by (`-B`/`-L`/`-J c|t`/`-X j`). `architecture.md`: reworded the three paragraphs that called bgwriter/replslots/pg_stat_io the "first/second/third+fourth NotRecordable" views into past-tense "was TUI-only in 0.11.0, became recordable via feature 008", and added one sentence stating the `NotRecordable` field + `filterViews()` drop-branch still exist but no production view sets them anymore (kept only for the synthetic `TestFilterViews_dropsExplicitNotRecordable` guard). `features-catalog.md`: rewrote the Limitations + Touches lines in entries [004]/[005]/[006]/[007] so record/report is documented as added in [008] rather than as a current limitation, with the per-screen report flags. All claims verified against code: report flags exist in `cmd/report/report.go` (158-183), `NotRecordable` removed from every production view in `internal/view/view.go`, guard test names confirmed in `record/record_test.go`. `make lint` skipped — `golangci-lint` not installed in this environment (and it does not lint markdown).
**Deviations:** None vs the task spec. One review correction applied (see below).

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved_with_suggestions (0 critical, 1 major, 2 minor) → [task-10-dev-code-reviewer-review.json](008-feat-record-report-0-11-views-task-10-dev-code-reviewer-review.json). Verified every flag/mapping claim against `report.go`, confirmed no production `NotRecordable` setter remains and the field/branch survive only for the synthetic guard. Major (non-optional): `architecture.md` originally said feature 008 cleared "the last five" views, but it cleared six — the five 0.11.0 screens plus `procpidstat` (whose stale flag was removed in task 03, per `TestFilterViews_NotRecordable`). Fixed by rewording to "the last of them — the five 0.11.0 screens, plus a stale flag on `procpidstat`". The two minor suggestions (cross-link `internal/view/view_test.go`; note framing distinction between the two "five" statements) left unapplied — both optional and would add bloat against the minimal-edit constraint.

**Verification:**
- `grep -i "TUI-only|NotRecordable|not recordable|deferred"` over the three files → no false labels on the four/five screens; only historical past-tense mentions and the factual `NotRecordable`-mechanism note remain.
- Flags confirmed in `cmd/report/report.go`; `NotRecordable` absent from production views in `internal/view/view.go`; guard test name confirmed in `record/record_test.go:168`.
- `make lint` skipped (golangci-lint not installed; docs-only, no Go touched).

---

## Task 11: Pre-deploy QA

**Status:** Done
**Commit:** (orchestrator QA — no code change)
**Agent:** основной агент (do-feature lead, wave 4)
**Summary:** Ran the locally-available gate for the whole feature: `go build ./...` and `go vet ./...` clean; every feature-touched package green (internal/view, cmd/report, report incl. all golden replay + describe, record filterViews subset, internal/stat new diff/guard tests). End-to-end CLI smoke confirmed: `report -d -B/-L/-J c/-J t/-X j` each print the correct description, and unknown `-J x`/`-X z` correctly error ("report type is not specified, quit").
**Deviations:** Full `make test`/`make lint`/`make vuln` not runnable locally — `golangci-lint` and `govulncheck` are absent in this environment, and `record.Test_app_record` / `top.Test_doReload` panic without a live PostgreSQL (pre-existing, env). These run on CI (PG14-18 matrix) on push. The manual record→report-vs-TUI comparison (user, needs live PG) remains a user step.
**Tech debt:** Observed (pre-existing, to log at /done): (1) `record.Test_app_record` and `top.Test_doReload` panic instead of t.Skipf without a live PG — sibling of [005]; (2) `internal/stat/postgres_test.go` and `procpidstat_test.go` are gofmt-dirty on develop baseline (not introduced here; feature additions are gofmt-clean); (3) tar deserialization `make([]byte, hdr.Size)` from security audit. None block this feature.

**Verification:**
- `go build ./...` → OK; `go vet ./...` → clean
- `go test ./internal/view/ ./cmd/report/ ./report/` → ok
- `go test ./record/ -run Test_filterViews|...` → PASS; `go test ./internal/stat/ -run Test_DiffZeroFilledCells|TestProcPidColIndexConstants` → PASS
- CLI smoke (built binary): 5 new `report -d` flags → correct descriptions; unknown values → error
- Full gate (make test/lint/vuln) + PG14-18 matrix → deferred to CI on push
