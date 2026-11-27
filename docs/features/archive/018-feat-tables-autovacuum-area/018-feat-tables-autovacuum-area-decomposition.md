---
created: 2026-11-26
status: approved
---

# Decomposition: Tables and autovacuum area — one pass

## Dependency DAG and Waves

Task 01 and Task 02 own disjoint query files and test files. Task 03 consumes all three selectors and owns the registry plus recording exclusion. Tasks 04 and 05 are independent after Task 03: Task 04 changes only record/report replay and descriptions, while Task 05 changes only the TUI. Task 06 documents the final behavior after all product tasks.

```text
01 ─┬─> 03 ─┬─> 04 ─┐
02 ─┘       └─> 05 ─┴─> 06
```

| Wave | Tasks | Depends on prior waves | Integration verification |
|------|-------|------------------------|--------------------------|
| 1 | 01, 02 | None | Query unit tests; live PG 14–19 field-description tests; dedicated disposable PG 19 wraparound PASS for Task 01 |
| 2 | 03 | 01, 02 | `go test ./internal/view/ ./record/ ./internal/stat/`; registry, filtering, deterministic INFO and no-diff behavior |
| 3 | 04, 05 | 01–03 | Report replay matrix and CLI descriptions for 04; navigation, menu, toggle, keybinding and help tests for 05; no shared files between tasks |
| 4 | 06 | 01–05 | Documentation consistency checks and manual SC-9 review |

## Task Model and Review Policy

| Task | Model tier | Reason | Reviewers |
|------|------------|--------|-----------|
| 01 | strong | SQL value contract plus mutation-checked, irreversible-XID fixture isolated by an external supervisor | dev-code-reviewer, dev-security-auditor |
| 02 | standard | Established selector pattern; live multi-version query tests | dev-code-reviewer, dev-security-auditor |
| 03 | standard | Registry/configuration wiring and deterministic recording exclusion | dev-code-reviewer |
| 04 | strong | Stateful replay-boundary semantics over untrusted archives, including both layout directions and sort restoration | dev-code-reviewer, dev-security-auditor |
| 05 | standard | Fully specified TUI navigation and single-cmdline-write contract | dev-code-reviewer |
| 06 | standard | Documentation-only reconciliation against implemented behavior | dev-code-reviewer |

Every code task uses `code-writing` and meaningful mutation checks where the tech-spec requires them. Task 06 uses `documentation-writing`. Security review is assigned to the tasks that construct SQL or parse/replay recorded archives; it is deliberately omitted from static registry, TUI wiring, and documentation tasks.

## Acceptance Execution Map

A scenario becomes eligible only after every listed task is complete. Eligibility below describes the product state; it does not make acceptance evidence a completion prerequisite of a task that is itself required by that scenario. SC-4 is already executable on the pre-feature base and remains a continuous regression check.

| SC | Required tasks | Earliest eligible wave | Provider/fixture conformance |
|----|----------------|------------------------|------------------------------|
| SC-1 | 01, 02, 03 | 3 | Fresh no-secret PG 19 container; active/untouched relations; DB clock and `stats_reset` controls; real `record`/`report` and tar inspection |
| SC-2 | 02, 03, 04 | 4 | Synthetic mixed-version `tables`/`indexes` archives validated by base replay; two ticks per version |
| SC-3 | 02, 03, 04 | 4 | Built production CLI; exact `report -d -T` and `report -d -I` output |
| SC-4 | None — base executable | Base and every feature run | Identical PG 17/18 synthetic archives; independent `master` build and feature build; byte comparison |
| SC-10 | 02, 03, 04 | 4 | Both screens × both layout directions × blank/populated `stats_age`; two ticks per layout; `-o stats_age` |

### Harness and Execution Ownership

- **The current `decompose-tech-spec` phase owns harness production before any implementation task starts.** It creates `acceptance/scenarios/SC-1.sh`, `SC-2.sh`, `SC-3.sh`, `SC-4.sh`, `SC-10.sh`, all `acceptance/scenarios/lib/` helpers and fixture generators, and `acceptance/scenario-manifest.sha256`. It runs positive controls and the clean base proof, obtains independent `dev-acceptance-reviewer` freeze approval, and commits the frozen harness. These files already exist by the time Task 01 may start; no product task creates, edits, or re-freezes them.
- **Feature tasks own only their unit/integration verification.** No task is marked complete by referring to a future acceptance result. In particular, Task 04 completes on its replay/description tests in wave 3; the scenarios it enables run later.
- **The separate `freeze-qa` phase owns all feature acceptance execution after wave 4**, including SC-1. Its exact initial invocation is `${KIMI_CODE_HOME}/skills/freeze-qa/scripts/run-acceptance.sh docs/features/018-feat-tables-autovacuum-area/acceptance feature-r1 900`. It requires `0/pass/complete` for every scripted scenario, preserves sanitized result/log evidence, and runs SC-4 again as the continuous regression. Failures follow mandatory batch triage rather than becoming task-completion assumptions.
- **Decision 12 is an explicit freeze-qa evidence gate.** The acceptance harness provides `assert_sanitized` and secret-scan controls; the manual stand runner applies them to SC-5–SC-8. Raw archives and tmux/session transcripts stay outside Git in mode-0700 temporary directories with trap-based deletion. No credential values, raw environment, unredacted SQL text, or hostnames are published. The stand runner records the manual sanitization verdict and freeze-qa verifies both scripted and manual evidence.

There are no external providers or production-provider DTOs. Production CLI conformance is exercised directly by all five scripted scenarios, so no reference-client substitute or later provider-conformance gate exists.

### Manual Acceptance Map

| SC | Required tasks | Manual evidence |
|----|----------------|-----------------|
| SC-5 | 01, 03, 05 | PG 19 stand, 190×52 tmux, both `t`/`T` paths, exact values/order/help, one cmdline message per path, frozen-column scrolling, A/B against `master` |
| SC-6 | 01, 03, 05 | Stable system-schema wraparound fixture; user/all toggle observation |
| SC-7 | 03, 05 | PG 18 stand and `master` A/B; legacy `t`, marked disabled menu item, exactly one explanation |
| SC-8 | 02, 03, 05 | PG 19 active/untouched relations; direct `stats_reset`/DB-clock evidence and growth over refreshes |
| SC-9 | 06 | Documentation/register review against final behavior |

AC-32 is an independent mandatory quality gate, not an SC: `make test` in the PG 14–19 CI image, Task 01's separate targeted wraparound PASS, `testing/e2e.sh`, `make build`, `make lint`, and `make vuln`. A shared-suite skip is not wraparound evidence.

## Requirement Coverage

| Requirements | Owning tasks |
|--------------|--------------|
| US-1, US-2, US-4 | 01, 03, 05 |
| US-3 | 02, 03, 04, 05 |
| US-5 | 03, 05 |
| AC-1–AC-5, AC-7–AC-11, AC-34 | 01 (registry/navigation support also 03/05) |
| AC-6 | 01, 03 |
| AC-12 | 03 |
| AC-13–AC-20, AC-35 | 05 |
| AC-21–AC-28 | 02, 03; replay/CLI evidence 04; manual display evidence 05 |
| AC-29, AC-30 | 03; external regression SC-4 |
| AC-31 | 03; SC-1 |
| AC-32 | 01, 03 and final full quality gate |
| AC-33, AC-36, AC-41 | 04 |
| AC-37–AC-40 | 06 |

## Freeze Readiness Plan

- SC-1 uses a newly created synthetic database in a dedicated PG 19 acceptance container. It proves the active/untouched fixture and captures DB-derived reset/clock bounds before deciding whether the feature is absent. Raw archives remain in a restrictive temporary directory and are deleted by trap; committed logs contain only sanitized output, member names, and hashes.
- SC-2 and SC-10 generate deterministic synthetic tar archives with validated `PGresult` JSON shapes, metadata versions, timestamps, and two samples per layout. Positive controls prove fixture readability and expected base replay before asserting feature-missing checkpoints.
- SC-3 builds the actual binary and validates both describe commands and their PG 19+ markers.
- SC-4 builds the feature and `master` binaries independently, uses identical archives, and compares sanitized report bytes. Structural query/Ncols checks remain implementation tests rather than being hidden in this CLI comparison.
- Expected new-scenario base checkpoints are exactly `pg19_stats_age_report_unavailable`, `relation_stats_age_descriptions_missing`, and `same_pg19_layout_change_unsupported`; regressions SC-2 and SC-4 must end `0/pass/complete`.
- `acceptance/scenario-manifest.sha256` covers every script, library, fixture, and harness input. The final base run must be clean and pass the installed runner's marker contract before independent `dev-acceptance-reviewer` freeze review.
- SC-5–SC-9 remain explicit owner/manual checks; no terminal or documentation judgement is relabelled as scripted evidence. For SC-5–SC-8 the stand runner applies the same `assert_sanitized`/secret-scan controls, deletes raw mode-0700 transcripts by trap, and records the Decision 12 verdict for freeze-qa.
