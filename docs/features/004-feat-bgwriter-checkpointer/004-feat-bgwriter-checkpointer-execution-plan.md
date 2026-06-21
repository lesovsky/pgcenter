# Execution Plan: pg_stat_bgwriter + pg_stat_checkpointer screen

**Создан:** 2026-06-21
**Branch:** develop

---

## Wave 1 (независимые)

### Task 01: bgwriter query layer + tests
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `make test` (bgwriter unit + integration tests pass)

### Task 02: Correct overview.md
- **Skill:** documentation-writing
- **Reviewers:** dev-code-reviewer
- **Verify:** bash — grep overview.md (stale claim gone, new screen mentioned)

## Wave 2 (зависит от Wave 1: Task 01)

### Task 03: Register view + TUI wiring
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** user — open `b` on PG17/PG18, confirm columns + absolute/delta + `?` help

## Wave 3 (Final Wave — зависит от 01, 02, 03)

### Task 04: Pre-deploy QA
- **Skill:** pre-deploy-qa
- **Reviewers:** none
- **Verify:** bash — `make test` && `make lint` && `make build`; acceptance criteria

## Проверки, требующие участия пользователя

- [ ] Task 03: пользователь открывает экран `b` на живом PG17 (и, по возможности, PG18) — состав
      колонок, абсолютные счётчики vs дельты, наличие `b` в справке `?`
- [ ] После всех волн: финальное приёмочное тестирование (Task 04)

## Известные ограничения окружения

- На машине поднят только **PG17** (порт 5432). Integration-тесты Task 01/04 для PG14/15/16/18
  локально сделают `t.Skipf` — это штатно. Юнит-тесты, `make build`, `make lint` проходят локально.
- **PG18 `slru_written` нельзя верифицировать локально.** Teammate пишет PG18-ветку по
  документации PostgreSQL; живая проверка откладывается на CI-матрицу (где есть PG14–18). Teammate
  НЕ блокируется на этом — фиксирует как deviation в `decisions.md`.
- Полный прогон PG14–18 локально возможен после `testing/prepare-test-environment.sh`.
