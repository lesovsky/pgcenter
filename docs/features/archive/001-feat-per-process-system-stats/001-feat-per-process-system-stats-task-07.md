---
status: done
depends_on: ["05", "06"]
wave: 4
skills: [pre-deploy-qa]
verify: "bash — make test"
reviewers: []
teammate_name:
---

# Task 07: Pre-deploy QA

## Required Skills

Before starting, load:
- `/skill:pre-deploy-qa` — [skills/pre-deploy-qa/SKILL.md](~/.claude/skills/pre-deploy-qa/SKILL.md)

## Description

Final quality gate for the per-process system stats feature (tasks 01-06). This task runs the full
test suite with race detector, verifies all acceptance criteria from both user-spec and tech-spec,
and confirms no regressions in existing pgcenter screens (activity, tables, statements, diskstats).

This is a pre-deploy-qa task — no code is written. The implementing agent runs automated checks
(`make test`, `make lint`, `make vuln`) and documents the results. Manual TUI verification items
(listed in "What to do") require user confirmation before the task can be marked done.

The procpidstat screen is only available in local mode (direct PostgreSQL connection on the same
host). All automated checks run against the test container (`lesovsky/pgcenter-testing:0.0.9`)
which has PostgreSQL 14-18 available on ports 21914-21918.

## What to do

**Automated checks (agent):**

1. Run `make test` — full test suite with race detector and 300 s timeout. All tests must pass,
   no new failures, no race conditions detected.
2. Run `make lint` — golangci-lint + gosec. No new warnings compared to the baseline (commits
   before this feature branch).
3. Run `make vuln` — govulncheck. No new vulnerabilities.
4. Run `make build` — binary builds cleanly.
5. Verify that targeted sub-packages pass in isolation:
   - `go test ./internal/stat/... -run ProcPid` — procfs parsers
   - `go test ./internal/stat/... -run BuildProcPid\|FormatCPU` — result builder and CPU formatter
   - `go test ./internal/query/... -run ProcPidStat` — SQL query template
   - `go test ./record/...` — record subsystem does not collect procpidstat data
6. Document results: which checks passed, any failures with their error output.

**Manual TUI checks (user must confirm):**

7. Run `pgcenter top` as `postgres` user (or via `sudo`), press `Shift+S`:
   - Screen "Per-process system stats" opens in the main area
   - Exactly 17 columns visible: pid, datname, usename, state, wait_etype, wait_event,
     all_total,s, us_total,s, sy_total,s, read_total,KiB, write_total,KiB,
     %all, %us, %sy, read,KiB/s, write,KiB/s, query
8. Run a CPU-heavy query (e.g. recursive CTE or large full-scan), find its PID in the screen,
   compare `%all` visually with `top -p <pid>`. Values should be in the same order of magnitude.
9. Check `read,KiB/s` and `write,KiB/s` against `pidstat -d 1 -p <pid>`.
10. Press `I` — idle backends disappear; press `I` again — they reappear.
11. Press `A`, enter `10` — only backends older than 10 seconds are shown.
12. Run `pgcenter top` as an unprivileged user (not postgres, not root), press `Shift+S`:
    - Warning shown in status bar about missing IO permissions
    - CPU columns (%all, %us, %sy, all_total,s, us_total,s, sy_total,s) display correctly
    - IO columns (read,KiB/s, write,KiB/s, read_total,KiB, write_total,KiB) are empty
13. Connect to a remote PostgreSQL instance, press `Shift+S`:
    - Warning shown: "Per-process stats available in local mode only"
    - Screen does NOT switch to procpidstat
14. Confirm no regression in existing screens:
    - `pgcenter top` activity screen (Shift+A or default)
    - tables screen (Shift+T)
    - statements screen (Shift+Q)

## Acceptance Criteria

- [ ] `make test` passes with race detector, no new test failures
- [ ] `make lint` and `make vuln` clean
- [ ] `Shift+S` switches to procpidstat view in local mode
- [ ] `Shift+S` in remote mode prints warning, does not switch view
- [ ] Screen displays 17 columns in correct order (pid through query)
- [ ] `all_total,s` / `us_total,s` / `sy_total,s` formatted as HH:MM:SS, sort correctly
- [ ] `%all` / `%us` / `%sy` in range 0-100 under CPU workload
- [ ] Rate columns show "0" on first tick, increase on subsequent ticks under load
- [ ] IO columns empty when `/proc/self/io` returns EACCES; warning shown once per session
- [ ] CPU columns work normally when IO is unavailable
- [ ] `I` filter hides `state='idle'` backends on procpidstat screen
- [ ] `A` filter applies age threshold on procpidstat screen
- [ ] Cancel/terminate/mask dialogs NOT available on procpidstat screen
- [ ] `pgcenter record` does not write procpidstat data
- [ ] No panic when a backend exits between ticks
- [ ] No memory growth in Collector after many ticks with high backend churn
- [ ] No regression in activity, tables, statements, diskstats screens

## Context Files

**Feature artifacts:**
- [001-feat-per-process-system-stats.md](docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats.md) — user-spec
- [001-feat-per-process-system-stats-tech-spec.md](docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-tech-spec.md) — tech-spec
- [001-feat-per-process-system-stats-decisions.md](docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md)
- [architecture.md](.claude/skills/project-knowledge/architecture.md)
- [deployment.md](.claude/skills/project-knowledge/deployment.md)

## Verification Steps

1. Run `make test` — exits 0, no race errors in output, no FAIL lines.
2. Run `make lint` — exits 0, no new lint warnings.
3. Run `make vuln` — exits 0 or reports only pre-existing vulnerabilities.
4. Run `make build` — binary at `./bin/pgcenter` is produced.
5. Run targeted sub-package tests and verify they exit 0:
   - `go test ./internal/stat/... -run ProcPid`
   - `go test ./internal/stat/... -run BuildProcPid\|FormatCPU`
   - `go test ./internal/query/... -run ProcPidStat`
   - `go test ./record/...`
6. User confirms manual TUI checks 7-14 from "What to do" above — all pass.

## Details

**No code changes in this task.** This is a pure QA task — read, run, verify, report.

**Dependencies:** Tasks 05 and 06 must be complete and merged before this task starts. The
full implementation is: procfs parsers (01), SQL query (02), result builder (03), view
registration + record skip (04), Collector integration (05), hotkey + guards (06).

**Test infrastructure:** The test suite requires a running PostgreSQL instance. Integration tests
connect to PG 17 by default (port 21917, localhost). The test container
`lesovsky/pgcenter-testing:0.0.9` provides PG 14-18 on ports 21914-21918. Tests use
`internal/postgres.NewTestConnect()` which targets PG 17 unless overridden.

**`make test` internals:** Runs `go test -race -timeout 300s ./...` with coverage. The race
detector finds concurrent map access — any new race condition is a blocker.

**IO unavailability test:** The integration test for `checkIOAvailable()` always succeeds in the
test environment (test process can read its own `/proc/self/io`). The EACCES path is verified
by unit tests using mocked/injected file paths. Manual verification by the user is required
for the real EACCES case.

**Memory growth check:** No automated memory leak test exists. The snapshot map cleanup logic
(Decision 5) prevents unbounded growth. The code reviewer in Task 05 was responsible for
verifying this logic. This task confirms the test suite runs cleanly without leak-related
failures; extended load testing is out of scope.

**Regression scope:** Existing screens (activity, tables, statements, diskstats) are covered by
existing tests in `./...`. The `make test` run is sufficient to confirm no regressions at the
unit/integration level. Visual regression in the TUI is user-confirmed in step 14.

**Cancel/terminate/mask dialogs:** These are bound to specific keys (`k`, `K`, backspace) and
are gated in `top/dialog.go` to the `"activity"` view only (Decision 7). The procpidstat
screen must not show these dialogs. This is verified structurally by the guard refactoring in
Task 06 and confirmed by the user during manual TUI testing.

**`pgcenter record` skip:** The `NotRecordable: true` flag on the procpidstat view causes
`record/record.go:filterViews()` to omit it. This is covered by `go test ./record/...`.

**Edge cases to watch in test output:**
- Any `panic:` in test output is an immediate blocker
- Any `DATA RACE` in test output is a blocker
- Any `FAIL` on a previously passing test is a blocker
- New lint warnings in `internal/stat/procpidstat.go`, `internal/view/view.go`,
  `internal/stat/stat.go`, `top/` packages, `record/record.go` must be addressed

## Reviewers

No reviewers for this task.

## Post-completion

- [ ] Write report to [001-feat-per-process-system-stats-decisions.md](docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-decisions.md) — include automated check results (make test/lint/vuln exit codes), and which manual TUI checks passed
- [ ] If any automated check failed — describe the failure and how it was resolved, or mark as known issue with reason
- [ ] If any acceptance criterion could not be verified — document why
