---
status: planned
depends_on: [01, 02, 03, 04, 05]
wave: 4
model_tier: standard
covers: [AC-37, AC-38, AC-39, AC-40]
acceptance_scenarios: [SC-9]
skills: [documentation-writing]
verify: documentation consistency checks and manual SC-9 review
reviewers: [dev-code-reviewer]
---

# Task 06: Documentation and technical-debt registers

## Outcome

User-facing docs, stable Project Knowledge, release notes, and the debt register describe the shipped tables/autovacuum behavior without claiming out-of-scope shared fixes.

## What to do

- Update `docs/features-catalog.md` with the PG 19+ autovacuum_scores screen, its purpose, t/T navigation, and live-TUI-only status.
- Update Project Knowledge `project.md` with pg_stat_autovacuum_scores and stats_age on tables/indexes; update `architecture.md` so it no longer claims no production view uses NotRecordable and records the new selector/navigation facts at the appropriate stable level.
- Add the new screen and relation columns to `doc/release-notes/v0.12.0.md`, including the intentional recording exclusion and any accepted user-visible limitation already required by the approved spec.
- Extend debt [035] to stats_age and [029] to autovacuum_scores. Leave [020] unchanged and do not imply its malformed-archive class is fixed by layout boundaries.
- Register the new debt for version-blind `x`/`p` cycles covering statements_jit (PG 15+) and progress_copy (PG 14+), explicitly not fixed by this feature.
- Reconcile wording with actual Task 01–05 behavior and tests; do not copy stale code-research statements about functions, sizes, first-of-kind navigation, or debt [020].

## Behaviors to Test

Documentation-only task: use deterministic content and consistency checks, then manual SC-9 judgement against final behavior.

## Acceptance Criteria

- [ ] AC-37–AC-40 are satisfied in all five named documents/register areas.
- [ ] The new x/p debt is registered without expanding implementation scope.
- [ ] [020] is byte-untouched; shared renderer/sanitizer fixes are not claimed.
- [ ] Manual SC-9 review finds documentation consistent with final behavior.

## Context

**Feature artifacts:**
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area.md` — user-spec.
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-tech-spec.md` — Decision 8 and documentation requirements.
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-decomposition.md` — waves and Acceptance Execution Map.
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-decisions.md` — decisions and deviations.

**Project knowledge:**
- `.kimi-code/skills/project-knowledge/references/project.md`
- `.kimi-code/skills/project-knowledge/references/architecture.md`
- `.kimi-code/skills/project-knowledge/references/patterns.md`

**Starting points (verified, not restrictive):**
- `docs/features-catalog.md` — feature catalog format.
- `doc/release-notes/v0.12.0.md` — current release notes.
- `docs/tech-debt.md` — active [035], [029], [020] and register conventions.
- Final Task 01–05 diffs and review reports — authoritative implemented behavior.

## Verification Steps

- Search all required files for autovacuum_scores, stats_age, NotRecordable, and the x/p debt → each required contract is present in its designated document.
- `git diff --check` and `git diff -- docs/tech-debt.md` → no whitespace errors; [020] unchanged and only [035]/[029] plus the new entry change.
- Complete manual SC-9 against final behavior → reviewer records an explicit verdict.

## Notes

Do not modify the general user interface, report renderer, archive validator, or sanitizer as part of documentation cleanup.

## Reviewers

- **dev-code-reviewer** → `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-task-06-dev-code-reviewer-round{R}.json`
