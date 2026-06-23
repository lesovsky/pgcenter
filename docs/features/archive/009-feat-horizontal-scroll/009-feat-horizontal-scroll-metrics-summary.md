# Metrics Summary: horizontal-scroll

## Context

| Dimension | Value |
|-----------|-------|
| Model | Opus 4.8 (1M context) |
| Feature size | M |
| Started | 2026-06-23 |
| Completed | 2026-06-23 |

## Timeline

Single-session delivery. Per-phase wall-clock (minutes):

| Phase | Duration | Touch | Wait |
|-------|---------:|------:|-----:|
| user_spec | 52 | 37 | 15 |
| tech_spec | 9 | 7 | 2 |
| task_decomposition | 13 | 11 | 2 |
| feature_execution | 151 | 121 | 30 |
| done | 3 | 3 | 0 |
| **Total** | **228** | **179** | **49** |

Flow efficiency (touch / lead): **78.5%**.

## Quality

| Metric | Value |
|--------|-------|
| Validation rounds | user_spec: 1, tech_spec: 1, task_decomposition: 1, feature_execution: 3 |
| User-spec validation | quality + adequacy + customer all approved (minors only, resolved) |
| Tech-spec validation | 5 validators (skeptic/completeness/security/template/architecture), 0 critical/major, minors applied |
| Task validation | dev-task-validator (1 critical, fixed) + dev-reality-checker (1 major, addressed); minors cosmetic |
| Validation findings (total) | 1 critical, 1 major, 32 minor |
| Per-task code/test reviews | task-01: 1 round, task-02: 3 rounds, task-03: 1 round |
| Review findings (total) | 1 critical, 5 major, 22 minor |
| First pass rate | 66.7% (2/3 code tasks — 01, 03 — passed review round 1; task-02 took 3 rounds) |

The one critical and the round-3 churn on task-02 came from a single root cause surfaced in
**manual QA, not by the agents**: the visible-column window admitted a column only when it fit
whole, dropping a wide trailing column (`query`) and emitting a spurious `›`. The fix
(partial-last-column + two-pass marker reservation) is captured in the decisions-log and the
spec Post-implementation section.

## Volume

| Metric | Value |
|--------|-------|
| Interview questions | 8 |
| Tasks | 4 (in 3 waves) |
| Agents spawned | 38 (research, validators, 3 implementers + their reviewers, QA) |
| Commits (grep "horizontal") | 11 |

## Outcome

All 4 tasks done; feature merged to master (commit 9bbd158, PR #143). `make build`/`make lint`
green; feature unit tests (`go test ./top/`) green including a property test proving the last
column is reachable at `maxOffset`. Closes issue #14 (open since 2015). No descoped items.
One functional divergence from the spec (testability-driven `io.Writer` print signatures) and
one QA-driven refinement (partial last column). No new tech debt registered — the remaining
round-3 minors are optional cosmetics. Pre-existing tech-debt [005] (PG fixtures) and
GO-2026-5037 (local toolchain) are unrelated to this feature.
