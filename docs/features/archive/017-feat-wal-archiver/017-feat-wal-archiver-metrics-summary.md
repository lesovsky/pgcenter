# Metrics Summary: 017-feat-wal-archiver

## Context

| Dimension | Value |
|-----------|-------|
| Model | Opus 5 (1M context) |
| Feature size | M |
| Started | 2026-08-05 |
| Completed | 2026-08-06 |

**Caveat:** phase timestamps were recorded manually by the orchestrator and are approximate to ~5
minutes. Human wait was instrumented only in the `user_spec` phase (the interview); the approval
gates of the later phases are counted as touch time, so flow efficiency below is an upper bound.

## Timeline

| Phase | Duration (min) | Touch (min) | Human wait (min) |
|-------|---------------|-------------|------------------|
| User Spec | 70 | 48 | 22 |
| Tech Spec | 50 | 50 | 0 |
| Task Decomposition | 295 | 295 | 0 |
| Feature Execution | 450 | 450 | 0 |
| Done | 17 | 17 | 0 |
| **Sum of phases** | **882** | **860** | **22** |
| Lead time (first start → last end) | 1130 | | |
| Idle between phases | 248 | | |

**Flow efficiency: 76.1%** (860 touch / 1130 lead). The 248 idle minutes are two overnight gaps
between decomposition, execution and finalization, not a queue.

Task decomposition took as long as it did for a reason worth keeping: two validation rounds found
tasks whose mutations targeted files those same tasks were forbidden to touch, which is a defect that
only shows up when a validator actually tries to run the plan.

## Quality

| Metric | Value |
|--------|-------|
| Validation rounds | user_spec: 2, tech_spec: 3, task_decomposition: 2 |
| Validation findings (crit/major/minor) | 1 / 18 / 74 (12 reports) |
| Review rounds by task | 01:2, 02:1, 03:2, 04:2, 05:2, 06:3, 07:2, 08:—, 09:—, 10:— |
| Review findings (crit/major/minor) | 0 / 15 / 96 (36 reports) |
| First pass rate | 0% (0 of 7 reviewed tasks cleared round 1 without a major) |

**A 0% first-pass rate here is a signal about the reviewers, not about broken code.** Every task
reached round 1 with a green suite; what the majors found was almost uniformly the same class — a
test that passes and cannot fail. Task 02: none of the four TDD-anchor asserts pinned the `/1024`
conversion, so replacing `round(wal_fpi_bytes / 1024, 2)` with the bare column left the suite green
while the screen would have shown bytes under a KiB header. Task 06: binding `W` to the wrong menu
left the entire filtered run green, because gocui keeps registered handlers unexported — which is
what forced `keybindings()` to be split so the table row itself became callable. Tasks 01 and 03: the
fixtures run with an empty `archive_status` directory, so a live check could not tell
`count(*) FILTER (WHERE name LIKE '%.ready')` from a bare `count(*)`, and the predicate had to be
pinned by a server-free structural test instead. Three tasks (08, 09, 10) had no reviewer cycle —
golden tests, documentation and QA — so the rate is computed over seven.

Tasks 06 needed a third round; every other reviewed task closed in two.

## Volume

| Metric | Value |
|--------|-------|
| Interview questions | 12 |
| Tasks | 10 (in 4 waves) |
| Agents spawned | ~90 (implementers, reviewers, validators) |
| Commits | 29 (on the feature branch) |

## Verification

| Gate | Result |
|------|--------|
| `make test` (`-race -p 1`, PG 14–19 fixtures in the CI image) | pass — 1085 PASS / 86 SKIP / 0 FAIL, 0 data races |
| `make lint` (golangci-lint + gosec) | pass, 0 issues |
| `make vuln` (govulncheck) | pass |
| Acceptance criteria, automated half | 24 of 34 |
| Acceptance criteria, stand run | 10 of 10, 0 FAIL |

All 86 skips are EOL-cluster subtests (PG 9.4–13) absent from the test image — debt [019] — with no
skip anywhere in the PG 14–19 range and none in the feature's own tests.

The manual gate ran twice: the first attempt found the stand unreachable and was reported as a
blocker rather than waved through, and the run was repeated on 2026-08-06 once it came back. That is
also where the `archive_status` cost measurement was taken (200 005 `.ready` files) and where the
truncation defect now registered as debt [035] was found by A/B against a `master`-built binary.
