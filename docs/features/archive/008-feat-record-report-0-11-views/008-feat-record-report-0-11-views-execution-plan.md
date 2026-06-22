# Execution Plan: record/report for 0.11.0 screens (008)

**Создан:** 2026-06-22
**Branch:** feature/record-report-0-11-views

> Environment note: team-orchestration tools (TeamCreate/SendMessage) are unavailable.
> Adapted model — tasks executed sequentially in wave/dependency order via implementer
> sub-agents; lead (this session) runs reviewer agents per task and commits per task.
> Within-wave execution is sequential (shared Go build graph makes concurrent edits unsafe).
> Per-task verify uses targeted package tests; full gate (make test/lint/vuln) runs on CI PG14-18.

## Wave 1 (foundation)

### Task 01: Enable recording + fix view/filter count tests
- **Skill:** code-writing · **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./internal/view/... ./record/...`

### Task 02: Report describe wiring for the 5 new types
- **Skill:** code-writing · **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./report/... -run Test_describeReport`

### Task 03: CLI report flags for the new screens
- **Skill:** code-writing · **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./cmd/report/...`

## Wave 2 (per-screen golden replay; depends on 01)

### Task 04: bgwriter replay golden tests (14/17/18)
- **Skill:** code-writing · **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./report/... -run Bgwriter`

### Task 05: replslots replay golden test (+ zero-slots)
- **Skill:** code-writing · **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./report/... -run ReplSlots`

### Task 06: pg_stat_io replay golden tests (count 16/18 + time)
- **Skill:** code-writing · **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./report/... -run StatIO`

### Task 07: statements_jit replay golden tests (15/17)
- **Skill:** code-writing · **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./report/... -run StatementsJIT`

## Wave 3 (tech-debt + docs; 09 depends on 02)

### Task 08: Tech-debt [007] — behavioral zero-cell diff test
- **Skill:** code-writing · **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./internal/stat/... -run Diff`

### Task 09: Tech-debt [004] — export procpidstat col-index constants
- **Skill:** code-writing · **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./report/... ./internal/stat/...`

### Task 10: Update documentation
- **Skill:** documentation-writing · **Reviewers:** dev-code-reviewer
- **Verify:** bash — manual doc read; `make lint`

## Wave 4 (Final)

### Task 11: Pre-deploy QA
- **Skill:** pre-deploy-qa · **Reviewers:** none
- **Verify:** bash — `make test && make lint && make vuln`

## Проверки, требующие участия пользователя

- [ ] Task 11: ручная сверка отчёта record→report с TUI для одного кумулятивного экрана на живом PG
- [ ] После всех волн: финальное приёмочное тестирование
