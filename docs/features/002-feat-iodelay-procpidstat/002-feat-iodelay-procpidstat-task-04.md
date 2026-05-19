---
status: planned
depends_on: ["02", "03"]
wave: 3
skills: [pre-deploy-qa]
verify: "bash — make test && make lint && make vuln"
reviewers: []
---

# Task 04: Pre-deploy QA

## Required Skills

Before starting, load:
- `/skill:pre-deploy-qa` — [SKILL.md](~/.claude/skills/pre-deploy-qa/SKILL.md)

## Description

This task performs acceptance testing for the iodelay feature before it ships. Tasks 01–03
implement the feature (core stat layer + TUI wiring, tests + golden files, documentation
updates). This QA task gates the final merge by verifying that everything works end-to-end:
automated test suite passes, static analysis is clean, and the TUI renders correctly in both
positive (`kernel.task_delayacct=1`) and negative (`kernel.task_delayacct=0`) live scenarios.

The feature adds two new columns (`iodelay_total,s` and `%iodelay`) to the procpidstat screen
(Shift+S), growing the column count from 17 to 19. The probe for delay accounting availability
reads `/proc/sys/kernel/task_delayacct` once at screen-open time. When the sysctl is disabled
or absent, both columns render as `""` and a warning appears in the cmdline area.

## What to do

1. Run the full automated test suite and verify it passes with zero failures or regressions:
   `make build && make test && make lint && make vuln`

2. Confirm that all new iodelay unit tests introduced in Task 02 are present and green:
   - `TestCheckDelayAcctAvailable`
   - `TestReadProcPidStatIODelay`
   - `TestReadProcPidStatTruncated`
   - `TestBuildProcPidResult_DelayAvailable`
   - `TestBuildProcPidResult_DelayUnavailable`
   - `TestCollectorUpdateProcPidStat19Cols` (renamed from 17Cols in `stat_test.go`)

3. Verify structural correctness in code: `procPidResultNcols == 19` and the procpidstat
   view `Ncols == 19` — mismatch causes panic in `align.SetAlign()`.

4. Perform manual TUI verification — positive scenario (requires root or sudo to set sysctl):
   - Enable: `sysctl -w kernel.task_delayacct=1`
   - Launch pgcenter connected to a local PostgreSQL instance
   - Press `Shift+S` to open the procpidstat screen
   - Run an IO-heavy query (e.g., `SELECT * FROM pg_class, pg_class c2 LIMIT 1000000`)
   - Verify that `%iodelay > 0` appears for the backend PID and `iodelay_total,s` changes
     between refresh ticks
   - On the first tick after opening: `%iodelay` must be `""`, `iodelay_total,s` must show a
     non-zero `HH:MM:SS` value

5. Perform manual TUI verification — negative scenario:
   - Disable: `sysctl -w kernel.task_delayacct=0`
   - Press `Shift+S` (re-open screen to trigger fresh probe)
   - Verify that `iodelay_total,s` and `%iodelay` columns display `""` for all rows
   - Verify that the cmdline area shows the warning:
     `"iodelay unavailable (task_delayacct=0): run sysctl -w kernel.task_delayacct=1, then re-open screen"`
     (or the combined message if IO is also unavailable)

6. Verify documentation updates from Task 03:
   - `docs/tech-debt.md` — tech debt entry `[001]` is marked as resolved
   - `docs/decisions-log.md` — new ADR entry for the `/proc/[pid]/stat` field 42 approach is present

## Acceptance Criteria

- [ ] `make build` succeeds on the feature branch without errors
- [ ] `make test` passes — all new iodelay unit tests pass, no regressions in existing tests
- [ ] `make lint` passes clean (golangci-lint + gosec, zero issues)
- [ ] `make vuln` passes clean (govulncheck, no known vulnerabilities)
- [ ] `procPidResultNcols == 19` and the procpidstat view `Ncols == 19` (verified via tests)
- [ ] `buildProcPidResult` with `delayAcctAvailable=true` returns 19 columns with non-empty col 11 (`iodelay_total,s`) and col 17 (`%iodelay`)
- [ ] `buildProcPidResult` with `delayAcctAvailable=false` returns 19 columns with `""` at col 11 and col 17
- [ ] `CheckDelayAcctAvailable()` returns `false` when sysctl file is absent (covered by test)
- [ ] Manual positive scenario: `kernel.task_delayacct=1` — `%iodelay > 0` and `iodelay_total,s` changes between ticks for an IO-active backend
- [ ] Manual positive scenario: first tick — `%iodelay` is `""`, `iodelay_total,s` shows non-zero `HH:MM:SS`
- [ ] Manual negative scenario: `kernel.task_delayacct=0` — both iodelay columns are `""` and warning appears in cmdline area
- [ ] Tech debt `[001]` is marked resolved in `docs/tech-debt.md`
- [ ] New ADR entry is present in `docs/decisions-log.md` documenting the `/proc/[pid]/stat` field 42 approach

## Context Files

**Feature artifacts:**
- [002-feat-iodelay-procpidstat.md](docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat.md) — user-spec
- [002-feat-iodelay-procpidstat-tech-spec.md](docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-tech-spec.md) — tech-spec
- [002-feat-iodelay-procpidstat-decisions.md](docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-decisions.md) — decisions log

**Project knowledge:**
- [project.md](.claude/skills/project-knowledge/project.md) — project overview, goals, tech stack
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow, PG version handling
- [deployment.md](.claude/skills/project-knowledge/deployment.md) — release process, CI/CD, make targets

## Verification Steps

- Run `make build` — expect zero errors, binary produced at `./bin/pgcenter`
- Run `make test` — expect all tests pass including the 5 new iodelay tests; check output for `PASS` on `TestCheckDelayAcctAvailable`, `TestReadProcPidStatIODelay`, `TestReadProcPidStatTruncated`, `TestBuildProcPidResult_DelayAvailable`, `TestBuildProcPidResult_DelayUnavailable`
- Run `make lint` — expect zero golangci-lint and gosec findings
- Run `make vuln` — expect govulncheck reports no vulnerabilities
- Manual positive scenario: confirmed by user — `%iodelay > 0` visible in TUI for IO-active backend when `kernel.task_delayacct=1`
- Manual negative scenario: confirmed by user — iodelay columns show `""` and warning appears in cmdline area when `kernel.task_delayacct=0`
- Documentation: `grep -A3 "\[001\]" docs/tech-debt.md` shows `resolved` status; `docs/decisions-log.md` contains entry referencing `/proc/[pid]/stat` field 42

## Details

**Files:** none (read-only QA task — no source files are modified)

**Dependencies:**
- Task 01 must be complete: core implementation in `internal/stat/procpidstat.go`, `internal/view/view.go`, `internal/stat/stat.go`, `top/config_view.go`, `record/record.go`
- Task 02 must be complete: new unit tests in `internal/stat/procpidstat_test.go`, golden files `internal/stat/testdata/proc/pid_stat_iodelay` and `pid_stat_truncated`, updated `internal/stat/stat_test.go` and `record/record_test.go`
- Task 03 must be complete: `docs/tech-debt.md` and `docs/decisions-log.md` updated

**Manual testing prerequisites:**
- A local PostgreSQL instance running and accessible to pgcenter
- Root or sudo access to toggle `kernel.task_delayacct` via sysctl
- The feature branch checked out locally (binary built from `make build`)

**Implementation hints:**
- Run `make build` before `make test` to ensure the binary under test reflects the latest source.
- The probe runs once at screen-open (`Shift+S`). Changing sysctl after opening does not take effect until the screen is closed and reopened.
- On the very first tick after opening the screen: `%iodelay` must be `""` (no prev sample), `iodelay_total,s` must show the accumulated counter as `HH:MM:SS`.
- `%iodelay` is NOT normalized by `cpuCount` — a single fully IO-blocked backend on a 4-core machine shows `~100%`, not `~25%`. This is correct by design (Decision 3 in tech-spec).
- `%iodelay` can exceed 100% — this is expected wall-clock behaviour, not a bug.
- If both IO (`/proc/[pid]/io`) and delayacct are unavailable simultaneously, the cmdline area shows a single combined warning (not two separate warnings — `printCmdline` mutual exclusion constraint).

**Warning texts to verify (exact strings from tech-spec):**
- delayacct-only: `"iodelay unavailable (task_delayacct=0): run sysctl -w kernel.task_delayacct=1, then re-open screen"`
- combined (IO + delayacct): `"IO stats and iodelay unavailable: run as postgres user + sysctl -w kernel.task_delayacct=1, then re-open screen"`

**Edge cases:**
- Open S-screen when running as a non-postgres user (IO unavailable) AND `task_delayacct=0` — verify combined warning appears
- Verify the binary exits cleanly (no panic) when pgcenter is pointed at a PostgreSQL instance with no active sessions (empty `pg_stat_activity`)

**`make test` flags:** uses `-race` and `-count=1` — if tests are flaky under race detector, that's a bug in Task 02's implementation, not a QA environment issue.

## Reviewers

No automated reviewers for this task.

## Post-completion

- [ ] Write brief QA report to [002-feat-iodelay-procpidstat-decisions.md](docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-decisions.md) — summary of what was verified (automated + manual), any deviations found
- [ ] If any acceptance criteria failed — describe what failed, which task needs a fix, and why
- [ ] If the feature behaves differently from the spec in any observable way — document the deviation and reason
