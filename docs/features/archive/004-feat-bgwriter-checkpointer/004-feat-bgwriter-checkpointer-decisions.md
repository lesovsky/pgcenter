# Decisions Log: bgwriter + checkpointer screen (004)

Отчёты агентов о выполнении задач. Каждая запись создаётся агентом, выполнившим задачу.

---

## Task 01: bgwriter query layer + tests

**Status:** Done
**Commit:** 4823e9b
**Agent:** query-dev
**Summary:** Created `internal/query/bgwriter.go` (three version-aware static `const` queries + `SelectStatBgwriterQuery(version) (string, int, [2]int)`) and `internal/query/bgwriter_test.go` (unit table test of per-version Ncols/DiffIntvl + integration loop PG14-18), mirroring the `wal.go`/`wal_test.go` pattern. Event counters sit outside `DiffIntvl` (absolute), the work/time/buffer block is contiguous and diffed, `stats_age` is last and excluded; PG17+ cross-joins `pg_stat_checkpointer` and sources `stats_age` from the checkpointer's `stats_reset` (Decision 4). Branch boundaries use raw literals `170000`/`180000`; all SQL is static const (zero injection surface, Decision 5).
**Deviations:** PG18 `slru_written` column was written from PostgreSQL documentation, NOT verified on a live PG18 cluster — only PG17 is available in the local dev environment (PG17.7 on port 5432; the test clusters on ports 21914-21918 were not running). The PG17 query branch WAS live-verified against the local PG17.7 cluster (13 columns, correct names, cross-join, and the new `len(FieldDescriptions()) == Ncols` assertion all confirmed). Live PG18 verification is deferred to the CI PG14-18 matrix, where `Test_StatBgwriterQueries` executes the PG18 query and now also asserts the live column count — this is the real `slru_written` schema-divergence gate.
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 0 critical / 0 major, 2 minor (optional) → [004-feat-bgwriter-checkpointer-task-01-dev-code-reviewer-round1.json](004-feat-bgwriter-checkpointer-task-01-dev-code-reviewer-round1.json)
- dev-test-reviewer: passed, 0 critical / 0 major, 2 minor (optional) → [004-feat-bgwriter-checkpointer-task-01-dev-test-reviewer-round1.json](004-feat-bgwriter-checkpointer-task-01-dev-test-reviewer-round1.json)

Both reviewers independently suggested the same optional hardening: assert the live column count against `Ncols`. Applied in commit 4823e9b (it directly strengthens the one named risk — the un-verifiable PG18 `slru_written`). The other minor suggestion (asserting query-string content in the unit test) was rejected as redundant with the integration test executing the real SQL, and to preserve consistency with the `wal_test.go` template. No round 2 — the fix was small, low-risk, lint-clean, and live-validated.

**Verification:**
- `go test -race ./internal/query/` → ok (unit green; integration skips PG14-18 — test clusters not running locally, accepted/expected)
- Live PG17.7 (port 5432): PG17 query executes, 13 columns, `Ncols` assertion passes
- `make build` → ok
- `golangci-lint run ./internal/query/` + `gosec -quiet ./internal/query/` → clean (exit 0, no findings)
- Note: `make test` has a pre-existing failure in `top/reload_test.go` (`Test_doReload` panics when the test PG cluster on port 21917 is absent) — verified present on a clean baseline via `git stash`, unrelated to this task.

## Task 02: Correct overview.md

**Status:** Done
**Commit:** 7745c80
**Agent:** docs-dev
**Summary:** Replaced the false `pg_stat_bgwriter — background writer stats` line in `overview.md` (Supported PostgreSQL Statistics) — which wrongly implied pre-existing as-is support — with an accurate entry for the new bgwriter/checkpointer screen this feature adds: single-row TUI screen, hotkey `b`, PG 14–18, `pg_stat_checkpointer` columns on PG 17+, TUI-only / not recordable in 0.11.0. Documentation-only; no code or other files touched.
**Deviations:** Нет.
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 0 critical / 0 major, 1 minor (optional) → [004-feat-bgwriter-checkpointer-task-02-dev-code-reviewer-round1.json](004-feat-bgwriter-checkpointer-task-02-dev-code-reviewer-round1.json)

The single optional suggestion (trim the dense bullet for tighter consistency with neighbors) was applied in commit 7745c80 by folding the caveats into a parenthetical in the `pg_stat_wal` style, without dropping any required fact (hotkey, PG range, PG17+ scoping, TUI-only/0.11.0). No round 2 — change is trivial and accuracy-preserving.

**Verification:**
- `grep -nE 'pg_stat_bgwriter[^+]*— background writer stats'` → empty (stale claim gone)
- `grep -niE 'pg_stat_checkpointer|bgwriter'` → new accurate entry present (line 21)

## Task 03: Register view + TUI wiring

**Status:** Done
**Commit:** 176e984
**Agent:** view-dev
**Summary:** Wired the bgwriter/checkpointer screen into `pgcenter top` by mirroring the `wal` screen exactly: added the `"bgwriter"` views-map entry (`NotRecordable: true`, `MinRequiredVersion: PostgresV14`, PG14 defaults Ncols 12 / DiffIntvl {3,10}) and a `case "bgwriter"` in `Configure()` calling `SelectStatBgwriterQuery(opts.Version)`; bound hotkey `b`; added `b` to the `?` help mode-key row (sorted `a,b,f,r,w`); and refreshed the stale `NotRecordable` example comment in `record/record.go` (procpidstat dropped the flag in feature 003, bgwriter is now its sole user). Added a guard test (`TestNew_BgwriterView`) and updated the existing view-count assertions 22→23.
**Deviations:** Screen column/render behaviour (event counters absolute, work/time/buffer columns delta, `stats_age` pass-through, hotkey opens the screen) is verified manually — `verify: user` is the acceptance gate, not automated here. `make test` is not fully green locally: a pre-existing, unrelated `top/reload_test.go::Test_doReload` panic fires when the local PG fixture on port 21917 is absent (confirmed present on the clean baseline via `git stash`, not caused by this change). `make lint` was not run via the Makefile target because the `golangci-lint` binary is missing in this environment; `go vet` on all changed packages is clean and `gofmt` flags only a pre-existing comment block in `view.go` (present on baseline, untouched per scope). `make build` and `go test ./internal/view/...` pass.
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 0 critical / 0 major / 0 minor → [004-feat-bgwriter-checkpointer-task-03-dev-code-reviewer-round1.json](004-feat-bgwriter-checkpointer-task-03-dev-code-reviewer-round1.json)
- dev-test-reviewer: passed, 0 critical / 0 major, 1 minor (optional) → [004-feat-bgwriter-checkpointer-task-03-dev-test-reviewer-round1.json](004-feat-bgwriter-checkpointer-task-03-dev-test-reviewer-round1.json)

The single optional suggestion (pin the PG14-default `Ncols`/`DiffIntvl`/`Msg` in the guard test) was applied in commit `3e0833a` (`fix: address review round 1 for task 03`) — zero-setup defense-in-depth that strengthens the wiring guard. No round 2: both reviewers approved with zero major findings and the fix is trivial and test-only.

**Verification:**
- `make build` → ok
- `go test ./internal/view/...` → ok (guard test green; counts 22→23 correct)
- `go vet ./internal/view/... ./top/... ./record/...` → clean
- Note: `make test` blocked by pre-existing environmental `Test_doReload` panic (port 21917 fixture absent), unrelated to this task.

## Task 04: Pre-deploy QA

**Status:** Done
**Agent:** qa
**Summary:** Final acceptance pass on the bgwriter/checkpointer feature (commit 867005a). All 9 locally-verifiable acceptance criteria PASS, 2 are CI-gated (PG18 `slru_written` + full PG14-18 integration matrix), 0 fail. Verdict: ready for CI, no blockers. `SelectStatBgwriterQuery` returns the correct (query, Ncols, DiffIntvl) for PG14-18 (12/[3,10], 12/[3,10], 12/[3,10], 13/[6,11], 14/[6,12]) — confirmed by `Test_SelectStatBgwriterQuery`. Counter placement (ckpt_*/rstpt_* absolute, work/time/buffer diffed, `stats_age` pass-through) verified by code layout and cross-checked live. `NotRecordable: true` honored by `filterViews` (3 filter tests green). Hotkey `b` bound and present in the `?` help row `a,b,f,r,w`. `overview.md` corrected. Live PG17.7 (port 5432) executed the PG17 query returning exactly 13 columns. Full report: [004-feat-bgwriter-checkpointer-qa-report.json](004-feat-bgwriter-checkpointer-qa-report.json).
**Deviations:** `make lint` not runnable locally — golangci-lint binary absent; substituted `go vet` (clean on all touched packages) and `gofmt` (the only gofmt finding is a pre-existing doc-comment in `view.go` Configure(), present on master/develop~5 baselines, untouched by this feature; the feature's added lines are gofmt-clean). The golangci-lint + gosec gate is deferred to CI. Full PG14-18 integration and the PG18 `slru_written` live check are deferred to the CI matrix (`lesovsky/pgcenter-testing`): local PG test clusters on ports 21914-21918 are not running and PG18 is unavailable locally, so `Test_StatBgwriterQueries` t.Skipf for all versions except the out-of-band live PG17.7 check; the `len(FieldDescriptions()) == Ncols` assertion is the schema-divergence gate that proves `slru_written` exists when CI runs PG18. `make test` is not fully green locally due to a PRE-EXISTING, unrelated panic in `top/reload_test.go::Test_doReload` (needs PG fixture on port 21917; confirmed on clean baseline in earlier tasks) and environmental record integration failures — neither is a feature defect.
**Tech debt:** Нет.

**Verification:**
- `make build` → ok (bin/pgcenter produced)
- `go test ./internal/query/... ./internal/view/...` → ok; named guards `Test_SelectStatBgwriterQuery`, `TestNew_BgwriterView`, `TestFilterViews_NotRecordable`, `TestFilterViews_dropsExplicitNotRecordable`, `TestFilterViews_Recordable` all PASS
- `go vet ./internal/query/... ./internal/view/... ./record/... ./top/...` → clean
- Live PG17.7 (port 5432): PG17 bgwriter query executes, returns exactly 13 columns in declared order
- Code grep: view.go:151 `NotRecordable: true`, record.go:208 filterViews drop branch, keybindings.go:36 `b → bgwriter`, help.go:13 `a,b,f,r,w` row, overview.md:21 corrected entry
