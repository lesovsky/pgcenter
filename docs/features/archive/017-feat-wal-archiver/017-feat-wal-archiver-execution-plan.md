# Execution Plan: 017-feat-wal-archiver

**Branch:** `feature/017-feat-wal-archiver` (from `develop`)
**Auto-approved** under autopilot.

## Waves

| Wave | Tasks | Parallel? | Notes |
|------|-------|-----------|-------|
| 1 | 01 archiver query, 02 PG 19 FPI, 04 report -W | yes | disjoint files; 01 also adds the shared test-role helper |
| 2 | 03 verbose backlog, 05 view registration | yes | 03 depends on 01's helper; 05 depends on 01+02 |
| 3 | 06 TUI w/W, 07 describe, 08 goldens, 09 release notes | yes | all depend on 05 (09 on 04) |
| 4 | 10 pre-deploy QA | — | full suite in the CI image + manual stand run |

## Verification environment

- Full suite: CI image `lesovsky/pgcenter-testing:0.0.11` with PG 14–19 fixtures.
- Host runs must be `-run` scoped: `./top/...` and `./record/...` panic without PostgreSQL.
- Build: `make build` (note: `go build ./cmd` fails — Go refuses to write an executable named `cmd` next to the `cmd/` directory; use `make build` or `go build -o /dev/null ./cmd` as a compile check). Lint needs `export PATH="$PATH:$(go env GOPATH)/bin"`.

## Review rule

Every task names concrete mutations of production code and the test that must redden on each.
Implementers run them, observe red, revert. "Looks correct" is not accepted.

## User checks (Wave 4)

Manual stand run on `pgpro@10.128.28.194`: three archiving states, navigation on both entry paths,
60-column terminal, and the cost measurement under a pg_monitor-only role with verbose on.
