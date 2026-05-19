---
status: planned
depends_on: ["04"]
wave: 4
skills: [pre-deploy-qa]
verify: "bash — Full E2E: make build, make test, make lint, record 3 ticks + report -N → ≥1 line output with timestamp"
reviewers: []
---

# Task 05: Pre-deploy QA

## Required Skills

Before starting, load:
- `/skill:pre-deploy-qa` — [SKILL.md](~/.claude/skills/pre-deploy-qa/SKILL.md)

## Description

This is the final acceptance gate for feature `003-feat-procpidstat-record-report`. After tasks 01–04 have implemented the MVC split, recorder enrichment, report pipeline, and test suite, this task validates that the full feature works end-to-end and that no regressions were introduced in the existing functionality.

The task covers three classes of validation:

1. **Build and test suite**: `make build`, `make test`, `make lint` — all must be clean. This confirms that the unit and integration test additions from tasks 01–04 pass and that no new lint issues were introduced.

2. **E2E functional verification**: Record 3 snapshots with `pgcenter record -c 3 -i 1s`, then run each `report` variant (`-N`, `-d -N`, `-A`, `-o`, `-l`, `-N` on old tar) and check the output matches the expected behavior from the user-spec. This exercises the full data path: procfs collection → tar write → tar read → report formatting → CLI output.

3. **TUI regression check**: Open `pgcenter top`, press Shift+S, verify the per-process screen loads with 19 columns intact. This is an explicit blocking criterion from the user-spec: the MVC refactor of `buildProcPidResult` must not break the live TUI view.

The task does not modify any code files. If any check fails, the agent must report what failed and what the actual output was, so the issue can be traced back to the responsible task (01–04).

## What to do

1. Run `make build` — binary `./bin/pgcenter` is produced without errors.
2. Run `make test` — all tests pass (green), including the new procpidstat unit tests from task 01, recorder tests from task 02, report tests from task 03, and the updated suite from task 04.
3. Run `make lint` — no new warnings beyond the baseline.
4. Record 3 snapshots: `./bin/pgcenter record -c 3 -i 1s -f /tmp/test.tar` — file `/tmp/test.tar` is created, no errors printed.
5. Run `./bin/pgcenter report -N -f /tmp/test.tar` — output contains at least one data row with a timestamp in `YYYY/MM/DD HH:MM:SS` format and numeric `%all` values.
6. Run `./bin/pgcenter report -d -N` — output shows the procpidstat column description text.
7. Run `./bin/pgcenter report -A -f /tmp/test.tar` — activity report is shown without errors (backward compat with existing `-A` flag).
8. Run `./bin/pgcenter report -N -f report/testdata/pgcenter.stat.golden.tar` — output contains `INFO: no procpidstat data`, no panic, exit 0.
9. Run `./bin/pgcenter report -N -f /tmp/test.tar -o "%all"` — rows within each snapshot are sorted by `%all` descending.
10. Run `./bin/pgcenter report -N -f /tmp/test.tar -l 2` — at most 2 rows per snapshot in the output.
11. TUI regression: start `./bin/pgcenter top` connected to local PostgreSQL, press Shift+S — the per-process stats screen opens with 19 columns and no panic.


## Acceptance Criteria

- [ ] `make build` succeeds — `./bin/pgcenter` binary produced
- [ ] `make test` passes — all tests green, no new failures
- [ ] `make lint` passes — no new warnings
- [ ] `pgcenter record -c 3 -i 1s -f /tmp/test.tar` creates the tar file without errors
- [ ] `pgcenter report -N -f /tmp/test.tar` outputs at least one data row with a timestamp and numeric `%all` values
- [ ] `pgcenter report -d -N` shows the procpidstat describe text
- [ ] `pgcenter report -A -f /tmp/test.tar` produces the activity report unchanged (backward compat)
- [ ] `pgcenter report -N -f report/testdata/pgcenter.stat.golden.tar` outputs `INFO: no procpidstat data`, exits 0, no panic
- [ ] `pgcenter report -N -f /tmp/test.tar -o "%all"` rows within each snapshot are sorted by `%all` descending
- [ ] `pgcenter report -N -f /tmp/test.tar -l 2` outputs at most 2 rows per snapshot
- [ ] TUI regression: `pgcenter top → Shift+S` opens the per-process screen with 19 columns, no panic

## Context Files

**Feature artifacts:**
- [003-feat-procpidstat-record-report.md](docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report.md) — user-spec
- [003-feat-procpidstat-record-report-tech-spec.md](docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report-tech-spec.md) — tech-spec
- [003-feat-procpidstat-record-report-decisions.md](docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report-decisions.md) — decisions log

**Project knowledge:**
- [architecture.md](.claude/skills/project-knowledge/architecture.md)
- [patterns.md](.claude/skills/project-knowledge/patterns.md)
- [deployment.md](.claude/skills/project-knowledge/deployment.md)

## Verification Steps

- `make build` — exits 0, `./bin/pgcenter` exists
- `make test` — exits 0, all test output is `PASS`
- `make lint` — exits 0, no new warning lines
- `./bin/pgcenter record -c 3 -i 1s -f /tmp/test.tar` — exits 0, `/tmp/test.tar` exists
- `./bin/pgcenter report -N -f /tmp/test.tar` — stdout contains a line matching `^\d{4}/\d{2}/\d{2}` (timestamp) and at least one data row
- `./bin/pgcenter report -d -N` — stdout contains the text `"Per-process system stats"` (or the full describe text from Decision 7 in tech-spec)
- `./bin/pgcenter report -A -f /tmp/test.tar` — stdout is non-empty, no error, exit 0
- `./bin/pgcenter report -N -f report/testdata/pgcenter.stat.golden.tar` — stdout contains `no procpidstat data`, exit 0
- `./bin/pgcenter report -N -f /tmp/test.tar -o "%all"` — manual inspection of first snapshot block: rows are in descending `%all` order
- `./bin/pgcenter report -N -f /tmp/test.tar -l 2` — no snapshot block in output has more than 2 data rows
- TUI Shift+S: confirmed by user — screen opens without crash, shows 19 columns

## Details

**Files:** N/A — QA task, no code changes

**Implementation hints:** Run verification steps in order. Local PostgreSQL must be running on port 21917 (test default). If make test fails on golden tar, run: go test ./report/... -update to regenerate. Stale /tmp/test.tar from previous runs must be deleted before each record invocation.

**No code files to modify.** This task is pure acceptance testing. If any check fails, identify which prior task (01–04) owns the broken component and report the failure with actual vs expected output.

**Dependencies:**
- Task 01: `internal/stat/procpidstat.go` — MVC split and `SysInfo` struct must be complete
- Task 02: `record/recorder.go`, `record/record.go` — stateful recorder and sysinfo write must be complete
- Task 03: `report/report.go`, `cmd/report/report.go`, `internal/view/view.go` — `-N` flag and report pipeline must be complete
- Task 04: updated test suite must pass; golden tar must include procpidstat entries if needed

**Prerequisite:** Local PostgreSQL instance must be running and accessible with default connection settings (for the E2E record step and TUI Shift+S check). Use `pgcenter record` without `-h` to ensure local mode (`db.Local == true`) so procpidstat is collected.

**Golden tar location:** `report/testdata/pgcenter.stat.golden.tar` — used for backward compat check (step 8). This is the old-format tar that should not contain procpidstat entries.

**Edge cases to verify manually:**
- If `/tmp/test.tar` exists from a previous run, delete it before the record step to avoid appending to stale data.
- If `%all` values are all `0.00` in the report output, the recorder may not have computed rates correctly (first-tick skip should hide zeros — if zeros appear, task 02 or 03 has a bug).
- If `report -N` output is empty (no data rows), check whether the tar contains `procpidstat.*` entries: `tar -tf /tmp/test.tar | grep procpidstat`.
- If `make lint` reports new warnings, compare against the lint baseline from before task 01 (check git log for pre-feature lint state).

**TUI regression note:** The MVC split in task 01 refactored `buildProcPidResult` while keeping the public signature unchanged. The TUI `Collector.Update()` calls `buildProcPidResult` for Shift+S — if the refactor broke anything, the screen will either panic or show wrong column count. Verify all 19 columns are present: `pid`, `datname`, `usename`, `state`, `wait_etype`, `wait_event`, `all_total,s`, `us_total,s`, `sy_total,s`, `read_total,KiB`, `write_total,KiB`, `iodelay_total,s`, `%all`, `%us`, `%sy`, `read,KiB/s`, `write,KiB/s`, `%iodelay`, `query`.

## Reviewers

No reviewers for this task.

## Post-completion

- [ ] Write report to [003-feat-procpidstat-record-report-decisions.md](docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report-decisions.md) — list each checked AC with pass/fail result, note any deviations
- [ ] If any AC failed and was fixed inline — describe what was fixed and why
- [ ] Update user-spec/tech-spec if anything changed during verification
