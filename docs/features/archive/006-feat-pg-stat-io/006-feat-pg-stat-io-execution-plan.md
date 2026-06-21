# Execution Plan: pg_stat_io screen

**Создан:** 2026-06-21
**Ветка:** feature/pg-stat-io (от develop)

> Оркестрация адаптирована: `TeamCreate`/`SendMessage` недоступны в этом окружении.
> Lead на каждую задачу спавнит агента-исполнителя (general-purpose, code-writing/TDD,
> коммитит сам), затем независимых ревьюеров на диф (JSON-отчёты); при находках — fix-агент
> (до 3 раундов). Пауза на подтверждение пользователя перед каждой волной.

---

## Wave 1 (независимые)

### Task 01: Query layer — internal/query/io.go + version constants
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/query/...`

## Wave 2 (зависит от Wave 1) — задачи 02 и 03 параллельны (непересекающиеся файлы)

### Task 02: View registration — internal/view/view.go
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./internal/view/...` + `go build ./...`

### Task 03: TUI navigation, menu & help — top/
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./top/...` + `make build`

## Wave 3 (Final) — зависит от Wave 2

### Task 04: Pre-deploy QA
- **Skill:** pre-deploy-qa
- **Reviewers:** none
- **Verify:** bash — `make build && make test && make lint && make vuln`; ручной обход US1–US4

## Проверки, требующие участия пользователя

- [ ] Перед каждой волной: явное подтверждение пользователя на запуск.
- [ ] Task 04 (QA): ручной обход US1–US4 на живом PG17 (j-toggle, J-меню, `/` фильтр, сортировка),
      проверка PG14/15 «not supported»; PG18-путь (`object='wal'` + нативные bytes) — гейт CI.
- [ ] После всех волн: финальный ревью и решение о merge в develop.
