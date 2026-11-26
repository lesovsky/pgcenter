---
status: planned
depends_on: []
wave: 1
model_tier: standard
covers: [US-3, AC-21, AC-22, AC-23, AC-24, AC-25, AC-26, AC-27, AC-28]
acceptance_scenarios: [SC-1, SC-2, SC-3, SC-4, SC-8, SC-10]
skills: [code-writing]
verify: go test ./internal/query/ -run 'StatTables|StatIndexes' in the PG 14–19 environment
reviewers: [dev-code-reviewer, dev-security-auditor]
---

# Task 02: Version-aware stats_age queries for tables and indexes

## Outcome

`tables` and `indexes` have PG 19 query layouts with a trailing, nullable `stats_age`, while earlier versions retain their exact existing layouts and diff intervals.

## What to do

- Add `PgStatTablesPG19` and `PgStatIndexesPG19` plus `SelectStatTablesQuery` and `SelectStatIndexesQuery`, following Decisions 1 and 2.
- Append `date_trunc('seconds', now() - <pg_stat alias>.stats_reset)::text AS stats_age` as the final column. Use the `pg_stat_*` half of each join, never `pg_statio_*`.
- Return `(query, Ncols, DiffIntvl)`: PG 19 tables `20/{1,18}`, indexes `7/{1,5}`; older and out-of-range versions retain `19/{1,18}` and `6/{1,5}`. Do not add `UniqueKey` to the selectors.
- Rewrite both query tests to observe `FieldDescriptions()` and drain errors, with per-version subtests, deferred close, active PG 14–19 versions, and hardened handling of unavailable versus unmapped fixtures.
- Pin the last-column and diff-bound relationship, the `pg_stat_*` source, NULL rendering, and the defence-in-depth invariant `DiffIntvl[1] < min(Ncols)` across both versions. Do not change `functions` or `sizes`.

## Behaviors to Test

- Versions 0, 180000, 190000, 200000 and the active supported range → correct query, Ncols and unchanged DiffIntvl.
- Live PG 14–19 execution → exact expected column names and counts, with no template residue and no drain error.
- PG 19 layouts → `stats_age` is last and `DiffIntvl[1] == Ncols-2`; the interval upper bound is valid against every layout.
- `stats_reset` NULL → blank cell; reset/active versus untouched behavior is observed externally in SC-1/SC-8 rather than resetting shared integration clusters.
- Query-source mutation from the `pg_stat_*` alias to `pg_statio_*` → named source assertion reddens.

## Acceptance Criteria

- [ ] AC-21–AC-26 are implemented for both screens without changing earlier-version layouts.
- [ ] AC-27/AC-28 are enabled for frozen SC-1 and manual SC-8; no shared-fixture reset is introduced.
- [ ] Live tests prove field names/counts rather than merely executing and discarding results.
- [ ] `functions` and `sizes` source files are untouched.

## Context

**Feature artifacts:**
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area.md` — user-spec.
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-tech-spec.md` — Decisions 1, 2 and 5.
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-decomposition.md` — waves and Acceptance Execution Map.
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-decisions.md` — decisions and deviations.

**Project knowledge:**
- `.kimi-code/skills/project-knowledge/references/architecture.md`
- `.kimi-code/skills/project-knowledge/references/patterns.md`
- `.kimi-code/skills/project-knowledge/references/deployment.md`

**Starting points (verified, not restrictive):**
- `internal/query/tables.go` and `indexes.go` — current 19/6-column layouts.
- `internal/query/wal.go` and `bgwriter.go` — version selectors and trailing stats_age.
- `internal/query/archiver_test.go` — live field-description and fixture handling.
- `internal/postgres/testing.go` — active version-to-port map.

## Verification Steps

- `go test ./internal/query/ -run 'StatTables|StatIndexes' -count=1` → named selector and live tests pass against available PG 14–19 fixtures; unmapped versions fail rather than skip.
- `go test ./internal/query/ -count=1` → package passes in the documented CI environment.
- Confirm `git diff -- internal/query/functions.go internal/query/sizes.go` is empty.

## Notes

Registry `Configure()` wiring belongs to Task 03. Replay boundaries and describe text belong to Task 04. Do not expand this task to either one.

## Reviewers

- **dev-code-reviewer** → `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-task-02-dev-code-reviewer-round{R}.json`
- **dev-security-auditor** → `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-task-02-dev-security-auditor-round{R}.json`
