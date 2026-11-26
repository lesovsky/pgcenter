---
status: in_progress
depends_on: []
wave: 1
model_tier: strong
covers: [US-1, US-2, US-4, AC-1, AC-2, AC-3, AC-4, AC-5, AC-7, AC-8, AC-9, AC-10, AC-11, AC-32, AC-34]
acceptance_scenarios: [SC-5, SC-6]
skills: [code-writing]
verify: go test ./internal/query/ -run AutovacuumScores; testing/run-autovacuum-wraparound-test.sh with targeted PASS, cleanup self-tests and named mutation evidence
reviewers: [dev-code-reviewer, dev-security-auditor]
---

# Task 01: Autovacuum scores query and isolated integration proof

## Outcome

Provide the complete PG 19 query contract for `autovacuum_scores`: eleven public columns, readable values, true-score ordering, absolute dead tuples, schema filtering with the wraparound exception, and executable proof against the real catalog. The screen itself is registered and navigable in later tasks.

## What to do

- Add `internal/query/autovacuum_scores.go` with `PgStatAutovacuumScoresDefault` and `SelectStatAutovacuumScoresQuery(_ int) (string, int, [2]int)`, returning the single PG 19 layout unconditionally as specified by Decisions 2–4.
- Select `relation`, `score`, `do_vacuum`, `do_analyze`, `for_wraparound`, `dead_total`, `xid_score`, `mxid_score`, `vacuum_score`, `vacuum_insert_score`, `analyze_score` in that order. Do not expose `relid`.
- Keep `pg_stat_autovacuum_scores s` as the outer side of a `LEFT JOIN pg_stat_all_tables` on `relid`; derive `relation` and the schema predicate from `s`. Render six scores with SQL rounding to two decimals, booleans with `::text`, and `dead_total` from `n_dead_tup` without a unit or coalesce.
- End with qualified `ORDER BY s.score DESC`. Add the `ViewType != all` template predicate hiding `pg_catalog`, `pg_toast`, and `information_schema` except when `s.for_wraparound` is true; all mode has no predicate.
- Add locked structural and live `FieldDescriptions()` tests, the measured rounded-tie ordering fixture, and privilege tests with and without `pg_monitor`. Ensure the test name and verification filter both contain `AutovacuumScores`.
- Implement Decision 10's targeted wraparound test and add the external supervisor `testing/run-autovacuum-wraparound-test.sh`. Its normal invocation is exactly `testing/run-autovacuum-wraparound-test.sh`; it creates a fresh single-use `lesovsky/pgcenter-testing:0.0.11` container/network namespace, discovers that server's identity, supplies `PGCENTER_AUTOVACUUM_WRAPAROUND_DEDICATED=1` and `PGCENTER_AUTOVACUUM_WRAPAROUND_SYSTEM_IDENTIFIER=<discovered identity>`, and inside that namespace runs `go test ./internal/query/ -run '^Test_StatAutovacuumScoresQuery_WraparoundException$' -count=1 -v`. The entire targeted test process must run in the dedicated network namespace, require the explicit marker, and verify the expected `system_identifier` immediately before mutation. Keep fixed `127.0.0.1:21919`; no endpoint override or shared-cluster fallback.
- Keep fixture objects feature-owned and idempotent; locate the toast relation through `reltoastrelid`, check the freeze-age safety margin, restore and verify the original autovacuum setting, and register cleanup immediately. The external supervisor remains responsible for destruction after completion, timeout, or SIGKILL.

## Behaviors to Test

- PG 19 query → exactly the locked eleven names in order; no template artifacts in either ViewType.
- Structural mutations to inner join, joined-side schema, rounded alias order, missing casts/rounding, or changed column order → the corresponding named assertion fails.
- Three raw scores that all render `0.00` → rows retain raw descending score order; changing to the output alias reddens the order assertion.
- Restricted and `pg_monitor` sessions → both execute and drain the query successfully.
- System-schema `for_wraparound=true` relation in user mode → row is present while ordinary system rows remain filtered; deleting `OR s.for_wraparound` reddens the row-presence assertion, not fixture setup.
- Ordinary shared suite without the dedicated marker → targeted mutating test skips honestly; separate targeted run supplies PASS evidence.

## Acceptance Criteria

- [ ] AC-1–AC-5 and AC-7–AC-11 query semantics are implemented and tested.
- [ ] AC-34 locked names match both SQL and live PG 19 field descriptions.
- [ ] AC-32 targeted wraparound evidence is a real PASS with fail-closed identity and external cleanup, not an ordinary-suite skip.
- [ ] The rounded-order and wraparound tests have observed, named red mutation evidence.

## Context

**Feature artifacts:**
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area.md` — user-spec.
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-tech-spec.md` — Decisions 2–4, 10 and Testing Strategy.
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-decomposition.md` — waves and Acceptance Execution Map.
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-decisions.md` — decisions and deviations.

**Project knowledge:**
- `.kimi-code/skills/project-knowledge/references/architecture.md`
- `.kimi-code/skills/project-knowledge/references/patterns.md`
- `.kimi-code/skills/project-knowledge/references/deployment.md`

**Starting points (verified, not restrictive):**
- `internal/query/archiver.go` and `archiver_test.go` — selector, locked-column and hardened-fixture idioms.
- `internal/query/replication_slots.go` — boolean text rendering and explicit ordering.
- `internal/query/activity.go` — conditional query templates.
- `internal/postgres/testing.go` — fixed PG 19 endpoint and `SetupTestRole`.
- `testing/Dockerfile` — disposable PG 19 image contract.

## Verification Steps

- `go test ./internal/query/ -run AutovacuumScores -count=1` → named non-mutating tests run and pass; no empty test filter.
- `testing/run-autovacuum-wraparound-test.sh` → `Test_StatAutovacuumScoresQuery_WraparoundException` reports PASS inside the fresh namespace and the supervisor proves container destruction.
- `testing/run-autovacuum-wraparound-test.sh --self-test timeout` and `testing/run-autovacuum-wraparound-test.sh --self-test sigkill` → bounded supervisor controls prove external destruction on timeout and test-process SIGKILL; neither mode touches a shared fixture.
- In a temporary worktree, change qualified `ORDER BY s.score DESC` to the rounded output alias and run `go test ./internal/query/ -run AutovacuumScores -count=1` → the named raw-score order assertion is observed red; revert.
- In a fresh temporary worktree, delete `OR s.for_wraparound` and run `testing/run-autovacuum-wraparound-test.sh` → the named system-row presence assertion is observed red after fixture setup succeeds; revert and destroy the container.
- `go test ./internal/query/ -count=1` → package passes in the documented PG 14–19 environment.

## Notes

This task does not register the view or change navigation. Do not burn XIDs on a shared or long-lived fixture. Raw acceptance archives and terminal transcripts are not task evidence to commit.

## Reviewers

- **dev-code-reviewer** → `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-task-01-dev-code-reviewer-round{R}.json`
- **dev-security-auditor** → `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-task-01-dev-security-auditor-round{R}.json`
