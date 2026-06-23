# Execution Plan: Horizontal Scroll (009)

**Создан:** 2026-06-23
**Ветка:** feature/horizontal-scroll
**Оркестрация:** team-инструменты недоступны → team-lead (я) спавнит исполнителя и ревьюеров через subagent-механизм, управляя ревью-циклами (diff → JSON-отчёты → фиксы, ≤3 раунда на задачу).

---

## Wave 1 (независимые)

### Task 01: Pure column-window function + scroll-offset state
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./top/...`

## Wave 2 (зависит от Wave 1)

### Task 02: Render frozen column + visible window in header and data
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** user — узкий терминал: скролл `[`/`]`, заморозка первой колонки, маркеры `‹`/`›`, bold

### Task 03: Scroll hotkeys, offset reset, and help text
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./top/...` + `make build`

## Wave 3 (Final — зависит от Wave 1+2)

### Task 04: Pre-deploy QA
- **Skill:** pre-deploy-qa
- **Reviewers:** none
- **Verify:** bash — `make test && make lint && make vuln`

## Проверки, требующие участия пользователя

- [ ] Task 02 / Task 04: ручная проверка в узком терминале — скролл `[`/`]` на activity и pg_stat_io, заморозка первой колонки, маркеры `‹`/`›`, bold-имя замороженной колонки, сброс offset при переключении view (включая `S` → procpidstat), no-op на широком терминале.
- [ ] После всех волн: финальное приёмочное тестирование (make test/lint/vuln зелёные).
