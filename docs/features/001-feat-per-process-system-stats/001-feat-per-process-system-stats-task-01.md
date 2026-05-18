---
status: planned
depends_on: []
wave: 1
skills: [code-writing]
verify: "bash — go test ./internal/stat/... -run ProcPid"
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
---

# Task 01: Procfs parser types and reader functions

## Required Skills

Before starting, load:
- `/skill:code-writing` — [SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

This task creates the foundational procfs parsing layer for the per-process system stats screen (Task 1 of 7, Wave 1).

The new screen `"procpidstat"` (opened by `Shift+S`) joins `pg_stat_activity` with per-process CPU and IO data from Linux procfs. This task is responsible for the lowest layer: reading and parsing `/proc/[pid]/stat` and `/proc/[pid]/io` into typed Go structs, plus checking IO availability at startup.

Two files are created from scratch:
- `internal/stat/procpidstat.go` — struct definitions, three reader functions
- `internal/stat/procpidstat_test.go` — unit tests (golden files) and integration tests

This task produces no behavior visible to the user yet. The parsers are consumed by Tasks 3 and 5 (result builder and collector integration). The test suite for this task uses the `ProcPid` filter, so it must pass cleanly in isolation.

## What to do

1. Create `internal/stat/procpidstat.go` with:
   - `ProcPidStat` struct with fields `Utime float64` and `Stime float64` (jiffies from `/proc/[pid]/stat`)
   - `ProcPidIO` struct with fields `ReadBytes float64` and `WriteBytes float64` (from `/proc/[pid]/io`)
   - `readProcPidStat(pid int) (ProcPidStat, error)` — opens `/proc/<pid>/stat`, finds the last `)` to handle comm names with spaces, splits the suffix, extracts utime (suffix index 11) and stime (suffix index 12)
   - `readProcPidIO(pid int) (ProcPidIO, error)` — opens `/proc/<pid>/io`, reads key-value pairs line by line, extracts `read_bytes` and `write_bytes`
   - `checkIOAvailable() error` — opens `/proc/self/io` and returns nil if readable, or the OS error (expected: `EACCES` when not running as the postgres user)

2. Create golden test data files in `internal/stat/testdata/proc/`:
   - A golden file for `/proc/[pid]/stat` with a comm name that includes spaces (e.g., `(my proc name)`)
   - A golden file for `/proc/[pid]/stat` with a normal (no-space) comm name
   - A malformed/truncated `/proc/[pid]/stat` golden file for error path coverage
   - A golden file for `/proc/[pid]/io` with valid `read_bytes` and `write_bytes`
   - A golden file for `/proc/[pid]/io` with a missing key for error path coverage

3. Create `internal/stat/procpidstat_test.go` with:
   - Unit tests for `readProcPidStat` using each golden file (space-in-comm, normal, malformed)
   - Unit tests for `readProcPidIO` using each golden file (valid, missing key)
   - Integration tests: `readProcPidStat(os.Getpid())` returns non-error with Utime+Stime >= 0; `readProcPidIO(os.Getpid())` returns non-error with ReadBytes+WriteBytes >= 0; `checkIOAvailable()` returns nil

## TDD Anchor

Write these tests first (they must fail before implementation, pass after):

- `internal/stat/procpidstat_test.go::TestReadProcPidStatSpaceInComm` — golden file with `(my proc name)` in comm field; verifies correct Utime and Stime extraction despite spaces
- `internal/stat/procpidstat_test.go::TestReadProcPidStatNormalComm` — golden file with single-word comm; verifies Utime and Stime values
- `internal/stat/procpidstat_test.go::TestReadProcPidStatMalformed` — truncated/invalid file; verifies that an error is returned (not a panic or silent zero)
- `internal/stat/procpidstat_test.go::TestReadProcPidIOValid` — golden file with `read_bytes` and `write_bytes` keys; verifies correct ReadBytes and WriteBytes values
- `internal/stat/procpidstat_test.go::TestReadProcPidIOMissingKey` — golden file missing `write_bytes`; verifies error is returned
- `internal/stat/procpidstat_test.go::TestReadProcPidStatIntegration` — calls `readProcPidStat(os.Getpid())`; verifies no error and Utime+Stime >= 0
- `internal/stat/procpidstat_test.go::TestReadProcPidIOIntegration` — calls `readProcPidIO(os.Getpid())`; verifies no error and ReadBytes+WriteBytes >= 0
- `internal/stat/procpidstat_test.go::TestCheckIOAvailable` — calls `checkIOAvailable()`; verifies no error (test process can always read `/proc/self/io`)

## Acceptance Criteria

- [ ] `ProcPidStat` struct exists in `internal/stat/procpidstat.go` with `Utime float64` and `Stime float64`
- [ ] `ProcPidIO` struct exists with `ReadBytes float64` and `WriteBytes float64`
- [ ] `readProcPidStat(pid int)` correctly extracts utime and stime when comm contains spaces (last-`)` method)
- [ ] `readProcPidStat(pid int)` returns an error on malformed input (no panic, no silent zero)
- [ ] `readProcPidIO(pid int)` correctly extracts `read_bytes` and `write_bytes` from key-value pairs
- [ ] `readProcPidIO(pid int)` returns an error when a required key is missing
- [ ] `checkIOAvailable()` returns nil when `/proc/self/io` is readable
- [ ] All unit tests pass using golden files from `internal/stat/testdata/proc/`
- [ ] Integration tests pass: `readProcPidStat(os.Getpid())` and `readProcPidIO(os.Getpid())` return no error
- [ ] `go test ./internal/stat/... -run ProcPid` passes with no failures
- [ ] `make lint` produces no new warnings for the new files
- [ ] File handles are closed with `defer func() { _ = f.Close() }()` pattern (errcheck-compliant)

## Context Files

**Feature artifacts:**
- [001-feat-per-process-system-stats.md](docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats.md) — user-spec
- [001-feat-per-process-system-stats-tech-spec.md](docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-tech-spec.md) — tech-spec (Data Models section, Testing Strategy section)

**Project knowledge:**
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout and data flow
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — error wrapping, linting, naming conventions, testing conventions

**Code files (read for patterns):**
- [internal/stat/cpu.go](internal/stat/cpu.go) — primary pattern: file open, `bufio.Scanner`, `defer func() { _ = f.Close() }()`, `fmt.Errorf` wrapping
- [internal/stat/diskstats.go](internal/stat/diskstats.go) — secondary pattern: structured field parsing, error handling
- [internal/stat/stat.go](internal/stat/stat.go) — `CollectNone`/`CollectDiskstats` iota constants (for context), `sValue` helper

**Files to create:**
- `internal/stat/procpidstat.go` (new) — structs and reader functions
- `internal/stat/procpidstat_test.go` (new) — unit and integration tests
- `internal/stat/testdata/proc/` (new directory) — golden test data files

## Verification Steps

- Run `go test ./internal/stat/... -run ProcPid` — all tests must pass, zero failures
- Run `make lint` — no new golangci-lint or gosec warnings from the two new files
- Manually confirm: `go test ./internal/stat/... -run ProcPid -v` shows integration test output with real PID values (Utime+Stime > 0)

## Details

**Files:**

`internal/stat/procpidstat.go` (new):
- Package `stat`, same as `cpu.go` and `diskstats.go`
- Imports: `bufio`, `fmt`, `os`, `path/filepath`, `strconv`, `strings`
- `ProcPidStat` — two float64 fields: `Utime`, `Stime` (raw jiffies, not seconds)
- `ProcPidIO` — two float64 fields: `ReadBytes`, `WriteBytes` (raw bytes)
- `readProcPidStat(pid int)`: open `/proc/<pid>/stat` via `fmt.Sprintf` with integer pid; read the single line; find the last `)`; everything after `") "` is the suffix; split suffix by whitespace; utime is at suffix index 11, stime at index 12 (0-based); parse both as float64 via `strconv.ParseFloat`
- `readProcPidIO(pid int)`: open `/proc/<pid>/io`; scan line by line; split each line on `": "`; accumulate `read_bytes` and `write_bytes` values; return error if either key is not found
- `checkIOAvailable()`: open `/proc/self/io` and close it immediately; return the error (nil = readable, EACCES = not permitted)

`internal/stat/testdata/proc/` (new directory):
- `pid_stat_space_comm` — realistic `/proc/[pid]/stat` line where field 2 is `(my proc name)` with spaces; must have at least 15 fields after stripping `pid (comm) state`
- `pid_stat_normal_comm` — realistic line with single-word comm, e.g., `(bash)`
- `pid_stat_malformed` — line truncated before the utime/stime fields
- `pid_io_valid` — six-line file matching actual Linux `/proc/[pid]/io` format, including `read_bytes: NNN` and `write_bytes: NNN`
- `pid_io_missing_key` — valid format but `write_bytes` line omitted

`internal/stat/procpidstat_test.go` (new):
- Use `testify/assert` (already used throughout the package)
- Unit tests use a helper that calls the reader with a fake file path pointing to the golden file — use an unexported variant of the reader that accepts a path string, or open the file and adapt as done in `cpu_test.go`; look at how existing tests in `internal/stat/` pass file paths before deciding approach
- Integration tests are gated by no special build tag — they run on CI as part of `make test`

**Dependencies:**
- No new external packages; `bufio`, `fmt`, `os`, `path/filepath`, `strconv`, `strings` are stdlib and already imported elsewhere in the package
- Task 3 (result builder) and Task 5 (collector integration) depend on the types and functions defined here

**Edge cases:**
- Comm with spaces: `(my great process)` — naive `strings.Fields` split would miscount field indices; must use last `)` to find the boundary
- Process disappears between `pg_stat_activity` query and procfs read: `os.Open` returns an error; caller (Task 3/5) skips the row — this parser just returns the error
- `/proc/[pid]/io` EACCES: `checkIOAvailable()` surfaces this; individual `readProcPidIO` calls may also fail per-PID if the race is between startup check and collection tick
- Empty or zero-byte procfs lines are not expected on Linux, but a short line should still return an error rather than index out of bounds

**Implementation hints:**
- Follow `cpu.go` exactly for file open, defer close, and scanner loop patterns — the linter checks for `_ = f.Close()` form
- For finding the last `)` use `strings.LastIndex(line, ")")` not `strings.Index`; slice from there+2 onward to get the suffix starting after `) `
- The suffix fields are separated by single spaces; `strings.Fields` handles this correctly
- Utime is kernel ABI field 14 (1-based), stime is field 15. After consuming `pid (comm) state` (3 fields), the zero-based suffix indices are 11 and 12 respectively — double-check with a real `/proc/self/stat` on the dev machine
- For `readProcPidIO`, using a boolean flag per key or a counter (increment when found, check == 2 at end) is cleaner than repeated string comparisons
- `filepath.Clean` on the path before `os.Open` is required (gosec G304 lint rule) — follow `diskstats.go` pattern
- Use `fmt.Sprintf("/proc/%d/stat", pid)` to construct the path — integer formatting avoids any path traversal; do NOT use string concatenation with the pid

## Reviewers

- **dev-code-reviewer** → `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-task-01-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-task-01-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-task-01-dev-test-reviewer-review.json`

## Post-completion

- [ ] Write a brief report to [001-feat-per-process-system-stats-decisions.md](docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-decisions.md) (1-3 sentences summary, review links, no finding tables or dumps)
- [ ] If deviated from spec — describe deviation and reason
- [ ] Update user-spec/tech-spec if anything changed
