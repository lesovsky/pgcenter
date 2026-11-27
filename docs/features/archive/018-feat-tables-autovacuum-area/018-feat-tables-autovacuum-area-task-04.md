---
status: done
depends_on: [02, 03]
wave: 3
model_tier: strong
covers: [US-3, AC-21, AC-22, AC-24, AC-33, AC-36, AC-41]
acceptance_scenarios: [SC-2, SC-3, SC-4, SC-10]
skills: [code-writing]
verify: go test ./report/ ./record/; frozen SC-2, SC-3, SC-4 and SC-10 are executed later by freeze-qa
reviewers: [dev-code-reviewer, dev-security-auditor]
---

# Task 04: Replay layout boundaries and relation descriptions

## Outcome

Recorded `tables`/`indexes` archives replay safely across both PostgreSQL version changes and same-PG19 pgcenter layout upgrades. Both describe outputs document trailing PG 19+ `stats_age` columns.

## What to do

- Implement Decision 9 in `report.processData`: treat any previous/incoming column-layout change as a boundary in addition to a recorded server-version change. Replace previous sample/metadata/timestamp and skip diffing the first new-layout sample.
- On every layout boundary rerun Configure, clear Aligned and stale Cols/ColsWidth, reset header cadence, restore the captured seed sort, and re-resolve requested `-o` against the new column list. Preserve existing first-sample and malformed-archive behavior outside this valid-layout contract.
- Generalize the activity-only synthetic replay helpers to take a screen name. Cover tables and indexes with at least two ticks per recorded version 180000/190000.
- Cover same-version 190000 old→new and new→old layouts for both screens, each with blank and populated stats_age and `-o stats_age`. Assert first-sample no-diff boundaries, second-sample metric deltas, recomputed headers/widths, old→new sort resolution, and new→old disappearance/seed-default restoration.
- Add trailing `stats_age` description rows to tables and indexes using the project's existing sentence and `(PG 19+)` convention. Pin description order against query order; do not add an autovacuum_scores describe entry or report flag.
- Keep the shared diff parser, archive validation, renderer, and sanitizer unchanged; Decision 9 is the scoped fix, not a fix for debt [020].

## Behaviors to Test

- PG 18→19 archive → each version uses its own headers, widths, deltas and sort; no narrow/wide pair reaches diff.
- Same PG 19 old→new → first wide sample seeds previous state; second produces metric deltas, stats_age is visible, and requested sort resolves at its new index.
- Same PG 19 new→old → first narrow sample seeds previous state; second produces metric deltas, stats_age disappears, and sort returns to the seed/default rather than a stale index.
- Blank/populated stats_age matrix → blank stays blank and populated values occur only in new-layout segments.
- `report -d -T`/`-I` → stats_age is the last described column with PG 19+ origin text.
- A/B legacy archives → tables/indexes/functions/sizes output remains byte-identical to the base binary through SC-4.

## Acceptance Criteria

- [ ] AC-33 and AC-41 pass for both screens and every required direction/value combination.
- [ ] AC-36 descriptions and order tests match the implemented PG 19 query layouts.
- [ ] AC-21/AC-22/AC-24 are preserved through replay; no cross-layout delta or stale rendering state remains.
- [ ] The frozen SC-2, SC-3, SC-4, and SC-10 scenarios are enabled for the later freeze-qa run; they are not a circular completion requirement for this task.

## Context

**Feature artifacts:**
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area.md` — user-spec.
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-tech-spec.md` — Decision 9 and replay fixtures.
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-decomposition.md` — waves and Acceptance Execution Map.
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-decisions.md` — decisions and deviations.

**Project knowledge:**
- `.kimi-code/skills/project-knowledge/references/architecture.md`
- `.kimi-code/skills/project-knowledge/references/patterns.md`

**Starting points (verified, not restrictive):**
- `report/report.go` — processData version-change branch and describe registry.
- `report/report_test.go` — buildActivityTar, activityReplayConfig and runProcessDataOnTar.
- `report/report_record_archiver_test.go` — two-tick replay harness.
- `report/describe.go` — existing stats_age wording and PG 19+ suffix convention.
- `internal/view/view.go` — configured layouts and seed sort.

## Verification Steps

- `go test ./report/ ./record/ -count=1` → all replay, description and recording tests pass.
- Inspect the test matrix output → both screens, both directions and blank/populated variants are named and executed, not skipped.
- Leave SC-2, SC-3, SC-4 and SC-10 for freeze-qa's exact `feature-r1` invocation recorded in the decomposition → no future acceptance result is required to close this wave-3 task.
- For any published replay artifact, run the harness `assert_sanitized` and secret-scan controls → only minimized output/member listings/hashes remain; raw archives stay outside Git in a mode-0700 temporary directory and are trap-deleted. Freeze-qa records the final evidence-gate verdict.
- `git diff -- internal/stat/postgres.go internal/align/ top/stat.go` → empty for this task.

## Notes

Recorded archives are untrusted input. Keep fixtures minimal and synthetic, bracket every sample with report time bounds, and never commit full raw acceptance recordings.

## Reviewers

- **dev-code-reviewer** → `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-task-04-dev-code-reviewer-round{R}.json`
- **dev-security-auditor** → `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-task-04-dev-security-auditor-round{R}.json`
