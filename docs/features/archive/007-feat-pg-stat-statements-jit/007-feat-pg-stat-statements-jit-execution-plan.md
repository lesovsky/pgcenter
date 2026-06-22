# Execution Plan: pg_stat_statements JIT screen

**Создан:** 2026-06-22
**Branch:** feature/pg-stat-statements-jit

---

Цепочка последовательная (каждая задача оставляет билд зелёным; одна задача на волну).

## Wave 1

### Task 01: JIT query consts + version selector
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/query/...`
- **Files:** internal/query/statements.go, internal/query/statements_test.go

## Wave 2 (зависит от Wave 1)

### Task 02: Register statements_jit view + Configure + count-test fixes
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/view/... ./record/...`
- **Files:** internal/view/view.go, internal/view/view_test.go, record/record_test.go

## Wave 3 (зависит от Wave 2)

### Task 03: TUI menu item + x-cycle wiring
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** user — local PG17: `X` → JIT; `x` cycles wal→jit→timings; `make build`
- **Files:** top/menu.go, top/config_view.go

## Wave 4 (зависит от Wave 1–3)

### Task 04: Pre-deploy QA
- **Skill:** pre-deploy-qa
- **Reviewers:** none
- **Verify:** bash — `make test && make lint` green; CI matrix PG14–18 green

## Проверки, требующие участия пользователя

- [ ] Task 03: пользователь проверяет на локальном PG17 — `X` открывает JIT-под-экран; `x` циклит `wal→jit→timings`; колонки/сортировка/фильтр работают.
- [ ] После всех волн: финальное QA + подтверждение перед merge в develop.
