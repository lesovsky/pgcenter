---
status: done
depends_on: [01, 02]
wave: 2
model_tier: standard
covers: [US-1, US-2, US-4, US-5, AC-1, AC-6, AC-12, AC-23, AC-29, AC-30, AC-31, AC-32]
acceptance_scenarios: [SC-1, SC-2, SC-3, SC-4, SC-5, SC-6, SC-7, SC-8, SC-10]
skills: [code-writing]
verify: go test ./internal/view/ ./record/ ./internal/stat/
reviewers: [dev-code-reviewer]
---

# Task 03: Register views and make recording exclusions honest

## Outcome

The three query layouts are wired into the view registry, `autovacuum_scores` is PG 19-only and live-TUI-only, recording excludes it with a deterministic truthful INFO, and legacy `functions`/`sizes` behavior remains pinned.

## What to do

- Register `autovacuum_scores` with the exact Data Models fields: `Msg: "Show autovacuum scores"`, 11 columns, `{0,0}`, sort column 1 descending, `NotRecordable`, `MinRequiredVersion: PostgresV19`, and non-nil maps.
- Add `Configure()` cases for `tables`, `indexes`, and `autovacuum_scores`; patch only `QueryTmpl`, `Ncols`, and `DiffIntvl`. Keep static tables/indexes entries at the PG ≤18 layout and record in code why their cases are load-bearing.
- Update all registry/version/filter counts and add explicit Ncols/layout assertions for tables/indexes plus negative `functions`/`sizes` assertions. Pin the new view's Msg and registry invariants.
- Fix `record/recorder_test.go` to pass the same filtered view map production uses before collecting against PG 17. Do not add skip behavior to production `tarRecorder.collect`.
- Implement Decision 11: retain aggregate version/extension filtering, but emit one deterministic INFO per deliberate NotRecordable exclusion containing the view name, `not recordable`, and `live TUI only`. Sort names before output; do not attribute the exclusion to server version.
- Refresh the stale NotRecordable comment in `record/record.go`. Add the `calculateDelta`/`Compare` empty-cell proof for the new no-diff screen and blank stats_age; show it red under a non-zero diff interval.

## Behaviors to Test

- Raw registry and configured PG 19/PG 18 views → correct Ncols, query, DiffIntvl, sort, availability, maps, and Msg.
- Filter at every existing test version → new view is always dropped by name, with counts updated and no arithmetic-only assertion.
- PG 19 recording setup → sanitized deterministic INFO names `autovacuum_scores`, `not recordable`, and `live TUI only`, with no false version-unsupported explanation; tar selection excludes the view.
- PG 17 recorder test → filtered map records normally and never attempts the PG 19-only query.
- Empty `dead_total` and `stats_age` under `{0,0}` → sample passes through unchanged; non-zero interval mutation → expected test failure.

## Acceptance Criteria

- [ ] AC-1, AC-6, AC-12, AC-23, AC-29–AC-32 registry/recording contracts are covered.
- [ ] No `UniqueKey`, `OrderKey`, `Cols`, or `ColsWidth` patching is added to the three Configure cases.
- [ ] Count-based literals, named filtering, Ncols assertions, and negative screens are updated together.
- [ ] The no-diff empty-cell test has observed red mutation evidence.

## Context

**Feature artifacts:**
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area.md` — user-spec.
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-tech-spec.md` — Data Models and Decisions 1, 2, 5, 11.
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-decomposition.md` — waves and Acceptance Execution Map.
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-decisions.md` — decisions and deviations.

**Project knowledge:**
- `.kimi-code/skills/project-knowledge/references/architecture.md`
- `.kimi-code/skills/project-knowledge/references/patterns.md`

**Starting points (verified, not restrictive):**
- `internal/view/view.go` and `view_test.go` — registry, Configure and count tests.
- `record/record.go`, `record_test.go`, `recorder_test.go` — filter contract and unfiltered-map trap.
- `internal/stat/postgres.go` and `postgres_test.go` — no-diff short circuit.
- Task 01/02 query files and selectors.

## Verification Steps

- `go test ./internal/view/ ./record/ ./internal/stat/ -count=1` → all targeted packages pass in the documented test environment.
- `go test ./record/ -run 'Filter|filter|tarRecorder' -count=1` → named exclusion and recorder tests execute; inspect output for the deterministic INFO contract.
- Run the no-diff mutation temporarily → the named empty-cell assertion fails for a non-zero interval; revert it.
- `git diff -- internal/query/functions.go internal/query/sizes.go` → empty.

## Notes

Task 04 owns replay layout boundaries and descriptions. Task 05 owns TUI navigation. Architecture documentation is deliberately deferred to Task 06, not left stale at feature completion.

## Reviewers

- **dev-code-reviewer** → `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-task-03-dev-code-reviewer-round{R}.json`
