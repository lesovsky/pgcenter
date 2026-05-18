---
status: done
depends_on: []
wave: 1
skills: [code-writing]
verify: "bash — go test ./internal/query/..."
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
---

# Task 02: Simplified pg_stat_activity SQL query

## Required Skills

Before starting, load:
- `/skill:code-writing` — [SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Create `internal/query/procpidstat.go` with a single exported Go constant `PgStatActivityProcPidStat` — a 7-column `pg_stat_activity` SQL query template for the new "Per-process system stats" screen.

This is the SQL layer for the procpidstat feature. The query returns exactly the columns that `buildProcPidResult()` (Task 3) expects at indices 0–6: `pid, datname, usename, state, wait_etype, wait_event, query`. The constant uses the same Go template conventions as the existing `PgStatActivityDefault` in `internal/query/activity.go`: `{{.QueryAgeThresh}}` is always embedded (no conditional guard; default `"00:00:00.0"` passes all rows), and `{{ if .ShowNoIdle }}` wraps the idle-filter clause conditionally.

The query is the foundation for Tasks 3–6. Getting the column count (exactly 7), column order, and template variable conventions right here prevents integration bugs in every downstream task.

## What to do

1. Create `internal/query/procpidstat.go` in package `query` with a single exported constant `PgStatActivityProcPidStat` containing the exact SQL query from the tech-spec Data Models section.

2. Create `internal/query/procpidstat_test.go` with:
   - A unit test (`TestPgStatActivityProcPidStat`) that formats the template with `NewOptions` and verifies it produces a valid string without errors.
   - An integration test (`Test_StatProcPidStatQuery`) that runs the formatted query against each available PostgreSQL version and asserts no SQL error is returned. Follow the pattern from `activity_test.go`: use `postgres.NewTestConnectVersion(version)` and `t.Skipf` for unavailable versions.

3. Verify `go test ./internal/query/... -run ProcPidStat` passes.

## TDD Anchor

Write these tests first, confirm they fail (compile error or assertion failure), then write the production code.

- `internal/query/procpidstat_test.go::TestPgStatActivityProcPidStat` — `Format(PgStatActivityProcPidStat, opts)` with default `NewOptions` returns no error and non-empty string
- `internal/query/procpidstat_test.go::TestPgStatActivityProcPidStat_ShowNoIdle` — same format call with `opts.ShowNoIdle = true` produces SQL containing `state != 'idle'`
- `internal/query/procpidstat_test.go::TestPgStatActivityProcPidStat_QueryAgeThresh` — format with `opts.QueryAgeThresh = "00:05:00"` produces SQL containing that value (confirms threshold is always embedded)
- `internal/query/procpidstat_test.go::Test_StatProcPidStatQuery` — integration: formatted query executes on each available PG version without error

## Acceptance Criteria

- [ ] `internal/query/procpidstat.go` exists in package `query` with exported constant `PgStatActivityProcPidStat`
- [ ] The constant contains exactly 7 selected columns in order: `pid, datname, usename, state, wait_etype, wait_event, query`
- [ ] `QueryAgeThresh` is always embedded (no `{{ if }}` guard around it)
- [ ] `ShowNoIdle` is conditional via `{{ if .ShowNoIdle }}AND state != 'idle'{{ end }}`
- [ ] `coalesce(..., '')` wraps all nullable columns (datname, usename, state, wait_event_type, wait_event, query)
- [ ] `regexp_replace(coalesce(query, ''), E'\\s+', ' ', 'g')` normalizes whitespace in the query column
- [ ] `WHERE pid != pg_backend_pid()` excludes the pgcenter backend itself
- [ ] `ORDER BY pid` (ascending) matches the default view sort order
- [ ] `Format(PgStatActivityProcPidStat, NewOptions(...))` produces a valid string without template errors
- [ ] Integration test executes the formatted query against PostgreSQL without SQL error
- [ ] `go test ./internal/query/...` passes

## Context Files

**Feature artifacts:**
- [001-feat-per-process-system-stats.md](docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats.md) — user-spec
- [001-feat-per-process-system-stats-tech-spec.md](docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-tech-spec.md) — tech-spec
- [001-feat-per-process-system-stats-decisions.md](docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-decisions.md) — decisions log

**Project knowledge:**
- [project.md](.claude/skills/project-knowledge/overview.md)
- [architecture.md](.claude/skills/project-knowledge/architecture.md)
- [patterns.md](.claude/skills/project-knowledge/patterns.md)

**Code files (read):**
- [internal/query/activity.go](internal/query/activity.go) — exact template conventions to follow (QueryAgeThresh always embedded, ShowNoIdle conditional)
- [internal/query/query.go](internal/query/query.go) — Options struct, NewOptions, Format function

**Code files (create):**
- `internal/query/procpidstat.go` — new file with `PgStatActivityProcPidStat` constant
- `internal/query/procpidstat_test.go` — new test file

## Verification Steps

- Run `go test ./internal/query/... -run ProcPidStat` — all unit and integration tests pass
- Run `go test ./internal/query/...` — full package test suite passes, no regressions in existing query tests
- Run `go build ./...` — project compiles without errors

## Details

**Files:**

`internal/query/procpidstat.go` — create new file. Package `query`. Single exported `const` block with `PgStatActivityProcPidStat`. Use a raw string literal (backtick) for the SQL body to avoid double-escaping backslashes. The exact SQL from the tech-spec:

```sql
SELECT pid,
    coalesce(datname, '') AS datname,
    coalesce(usename, '') AS usename,
    coalesce(state, '') AS state,
    coalesce(wait_event_type, '') AS wait_etype,
    coalesce(wait_event, '') AS wait_event,
    regexp_replace(coalesce(query, ''), E'\\s+', ' ', 'g') AS query
FROM pg_stat_activity
WHERE pid != pg_backend_pid()
AND ((clock_timestamp() - query_start) > '{{.QueryAgeThresh}}'::interval)
{{ if .ShowNoIdle }}AND state != 'idle'{{ end }}
ORDER BY pid
```

`internal/query/procpidstat_test.go` — create new file. Package `query`. Model after `activity_test.go`:
- Unit tests use `NewOptions(version, "f", "off", 256, "public")` to format the template and check output.
- Integration test: loop over versions `[]int{90500, 90600, 100000, 110000, 120000, 130000, 140000, 150000, 160000, 170000, 180000}`, connect via `postgres.NewTestConnectVersion(version)`, skip with `t.Skipf` on error, execute the formatted query with `conn.Exec(q)`, assert no error.

**Dependencies:**
- No dependency on other tasks in this wave — Task 02 is fully independent.
- Task 04 (`internal/view/view.go`) will reference `query.PgStatActivityProcPidStat` as `QueryTmpl` for the `"procpidstat"` view.

**Critical conventions (from `internal/query/activity.go`):**
- `QueryAgeThresh` appears unconditionally in the WHERE clause: `> '{{.QueryAgeThresh}}'::interval`. No `{{ if }}` guard. The default value `"00:00:00.0"` (set by `NewOptions`) means all rows pass.
- `ShowNoIdle` is conditional: `{{ if .ShowNoIdle }}AND state != 'idle'{{ end }}`. Use this exact form — no leading space before `AND`.
- Both template variables come from `query.Options` (defined in `query.go`), rendered by `query.Format()`.

**Column order matters:** Tasks 3–6 index into the SQL result by position (0–6). The order must be exactly: `pid(0), datname(1), usename(2), state(3), wait_etype(4), wait_event(5), query(6)`.

**Edge cases:**
- `wait_event_type` aliased as `wait_etype` — the alias is the column name downstream code uses.
- `regexp_replace` on `query` column uses `E'\\s+'` escape syntax inside a raw string literal — in a backtick literal, write `E'\\s+'` as-is (two backslashes render as one in SQL).
- The query targets `pg_stat_activity` which exists on all supported PG versions (9.5+). No version branching needed for this task.

**Implementation hints:**
- Copy the file header style from `activity.go` (package declaration, then `const` block with godoc comment).
- The godoc comment should mention: 7 columns, column order, template variable conventions (QueryAgeThresh always present, ShowNoIdle conditional).

## Reviewers

- **dev-code-reviewer** → `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-task-02-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-task-02-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-task-02-dev-test-reviewer-review.json`

## Post-completion

- [ ] Write a brief report to [001-feat-per-process-system-stats-decisions.md](docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-decisions.md) (summary: 1-3 sentences, review links, no finding dumps)
- [ ] If deviated from spec — describe the deviation and reason
- [ ] Update user-spec/tech-spec if anything changed
