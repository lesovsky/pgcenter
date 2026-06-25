# Execution Plan — 011-refactor-tech-debt-paydown

**Branch:** develop · **Size:** M · **Orchestration:** direct (no TeamCreate infra in this env;
lead spawns implementer + reviewer agents and manages review rounds manually).

## Waves

### Wave 1 (parallel — disjoint files)
- **Task 01 [009]** allocation cap — `internal/stat/postgres.go`, `report/report.go` (+ tests).
  Reviewers: dev-code-reviewer, dev-security-auditor, dev-test-reviewer.
- **Task 02 [011]** rate-helper consolidation — `internal/pretty/pretty.go`, `top/stat.go` (+ tests).
  Reviewers: dev-code-reviewer, dev-test-reviewer.

### Wave 2 (after Task 02 — shared files)
- **Task 03 [012]** fixed-width verbose Size — `internal/pretty/pretty.go`, `top/stat.go` (+ tests).

### Final Wave
- **Task 04** Pre-deploy QA — `make test` + `make lint` + `make vuln` + mandatory manual `v` check.

## Per-task gate
Each task: TDD (tests first) → implement → `go test` touched packages → `make lint`
(golangci-lint v2 at ~/go/bin + gosec, no G115) → `make vuln` → independent commit → review rounds (≤3).

## User checks
- Task 03 [012]: manual `v` verbose check in `pgcenter top` — Size columns/labels must not shift.

## Status
- Wave 1: in progress (this run; user gates each wave — stop after Wave 1).
