---
status: done                       # planned -> in_progress -> done
depends_on: ["01", "02", "03"]     # ID задач-зависимостей (строки: ["01", "02"])
wave: 4                            # волна параллельного выполнения
skills: [pre-deploy-qa]            # МАССИВ скиллов для загрузки
verify: bash — make test && make lint green; CI matrix PG14-18 green
reviewers: []                      # QA/verification task — ревьюеров нет
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 04: Pre-deploy QA

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:pre-deploy-qa` — [skills/pre-deploy-qa/SKILL.md](~/.claude/skills/pre-deploy-qa/SKILL.md)

## Description

Финальная волна фичи 007 (`pg_stat_statements` JIT screen) — приёмочное тестирование уже
реализованного функционала (задачи 01–03 завершены и закоммичены). Цель: убедиться, что фича
собирается, проходит тесты и линт, и что выполнены **все** критерии приёмки из user-spec
(«Критерии приёмки») и tech-spec («Acceptance Criteria»).

Это QA/верификационная задача — **код не меняется**. Если в ходе проверки находится дефект,
он не правится здесь: задача фиксирует факт и возвращает результат QA наверх (исправление —
отдельной итерацией в соответствующей задаче 01–03).

Фича — TUI-only, `NotRecordable`. Часть критериев (поведение JIT-экрана, цикл `x`, фильтр `/`,
deform-колонки на реальной БД) проверяется не юнит-тестами, а вручную на локальном PG17 и через
CI-матрицу PG14–18. Локальная среда без PostgreSQL даёт `connection refused` на exec-тестах —
это нормально (тесты гейтятся через `t.Skipf`); реальная версионная проверка — на CI.

## What to do

- Прогнать сборку и проверки: `make build`, `make test` (race + coverage), `make lint`. Все три
  должны быть зелёными. Зафиксировать фактический вывод (pass/fail, новые замечания линта).
- Пройтись по каждому критерию приёмки из user-spec и tech-spec (полный список — в секции
  Acceptance Criteria ниже) и отметить, выполнен он или нет, с указанием, чем подтверждается
  (тест / ручная проверка / CI).
- Проверить версионную ветку селектора по факту тестов: `go test ./internal/query/...` — обе
  ветки (PG15/16: Ncols 13, DiffIntvl {6,10}, UniqueKey 11; PG17+: 15, {7,12}, 13) проходят.
- Проверить count-тесты: `go test ./internal/view/... ./record/...` — `TestNew` ожидает 27,
  `Test_filterViews` `wantN +1` на всех строках.
- Убедиться, что в `report.go` **не** добавлена description-запись для `statements_jit`
  (NotRecordable-прецедент).
- Вручную на локальном PG17 (с `pg_stat_statements` и `jit=on`) проверить поведение JIT-экрана:
  `X` → 7-й пункт открывает `statements_jit`; `x` циклит `wal → jit → timings`; колонки,
  сортировка по `gen_total` desc, фильтр `/` работают; при отсутствии JIT-активности экран пуст
  и показан хинт.
- Подтвердить, что CI-матрица PG14–18 зелёная: PG14 → под-экран недоступен («not supported»),
  PG17/18 → присутствуют deform-колонки.
- Собрать итог QA: PASS/FAIL по каждому критерию + общий вердикт (можно ли деплоить).

## Acceptance Criteria

Полный чек-лист приёмки фичи (объединение user-spec и tech-spec). Каждый пункт нужно проверить
и подтвердить источником (тест / ручная проверка / CI):

- [ ] `make build` — бинарь собирается без ошибок.
- [ ] `make test` (race + coverage) — зелёный; новый тест селектора проходит; count-тесты
      обновлены и проходят.
- [ ] `make lint` — без новых замечаний.
- [ ] В меню `X` — 7-й пункт `pg_stat_statements JIT`; выбор открывает `statements_jit`.
- [ ] Цикл `x` проходит через JIT: `… wal → jit → timings …`.
- [ ] На PG15/16 — базовый набор фаз (13 колонок); на PG17/18 — он же + `deform_total`/`deform_ms`
      (15 колонок).
- [ ] Интервальные колонки (`*_ms`, `functions`) диффятся; `*_total` — кумулятивные текст-итоги.
- [ ] Строки с `jit_functions = 0` не показываются (SQL-фильтр `WHERE jit_functions > 0`).
- [ ] Дефолтная сортировка — по `gen_total` (desc); `OrderKey: 2`, `OrderDesc: true`.
- [ ] Смена колонки сортировки переупорядочивает строки; `*_total` сортируются численно по
      длительности (3+ значные часы и `N days …` — не лексикографически).
- [ ] Фильтр `/` по `query`/`database` сужает набор строк.
- [ ] На PG < 15 под-экран недоступен, приложение не падает, меню/цикл деградируют корректно
      («not supported», без пустого `View{}`).
- [ ] При `jit=off`/пустом наборе показывается хинт об отсутствии JIT-активности (`Msg`).
- [ ] View помечен `NotRecordable: true` — `pgcenter record`/`report` его пропускают; в
      `report.go` нет description-записи.
- [ ] `SelectStatStatementsJITQuery` покрыт юнит-тестом на обе ветки (PG15/16 и PG17+):
      query + Ncols + DiffIntvl + UniqueKey.
- [ ] Count-тесты обновлены: `view_test.go::TestNew` → 27; `record/record_test.go::Test_filterViews`
      → `wantN +1`.
- [ ] Нет регрессий в существующих view/record/query тестах.
- [ ] CI-матрица PG14–18 зелёная (PG14 → «not supported»; PG17/18 → deform-колонки присутствуют).
- [ ] Итоговый вердикт QA сформирован (PASS/FAIL по каждому пункту + готовность к деплою).

## Context Files

**Feature artifacts:**
- [007-feat-pg-stat-statements-jit.md](docs/features/007-feat-pg-stat-statements-jit/007-feat-pg-stat-statements-jit.md) — user-spec (секции «Критерии приёмки», «Как проверить»)
- [007-feat-pg-stat-statements-jit-tech-spec.md](docs/features/007-feat-pg-stat-statements-jit/007-feat-pg-stat-statements-jit-tech-spec.md) — tech-spec (секции «Acceptance Criteria», «Agent Verification Plan»)
- [007-feat-pg-stat-statements-jit-decisions.md](docs/features/007-feat-pg-stat-statements-jit/007-feat-pg-stat-statements-jit-decisions.md) — decisions log (отчёты задач 01–03)

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — фичи, поддерживаемая статистика, аудитория
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — раскладка пакетов, поток данных, обработка версий PG
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — паттерны кода, конвенции тестирования, версионное ветвление
- [deployment.md](.claude/skills/project-knowledge/deployment.md) — релизный процесс, CI/CD, версионная матрица

**Code files (под проверку, не под правку):**
- [internal/query/statements.go](internal/query/statements.go) — JIT-консты + `SelectStatStatementsJITQuery` (задача 01)
- [internal/view/view.go](internal/view/view.go) — view `statements_jit` + `Configure()` (задача 02)
- [top/menu.go](top/menu.go), [top/config_view.go](top/config_view.go) — меню + `x`-цикл (задача 03)

## Verification Steps

- Шаг 1: `make build` → бинарь собирается без ошибок.
- Шаг 2: `make test` → зелёный; селектор-тест и count-тесты проходят; нет регрессий.
- Шаг 3: `make lint` → без новых замечаний.
- Шаг 4: `go test ./internal/query/...` → обе версионные ветки возвращают ожидаемые
  Ncols/DiffIntvl/UniqueKey.
- Шаг 5: `go test ./internal/view/... ./record/...` → `TestNew` 27, `Test_filterViews` +1.
- Шаг 6: grep по `report.go` → нет description-записи для `statements_jit`.
- Шаг 7: ручная проверка на локальном PG17 → JIT-экран открывается из `X`, циклится `x`,
  сортировка/фильтр/колонки/хинт работают.
- Шаг 8: CI-матрица PG14–18 → все джобы зелёные (PG14 «not supported», PG17/18 deform-колонки).
- Результат: чек-лист приёмки полностью отмечен; общий вердикт сформирован.

## Details

**Files:** код не меняется. Все перечисленные файлы — под чтение/проверку, а не под правку.

**Dependencies:** зависит от задач 01, 02, 03 — должны быть завершены и закоммичены до старта QA.
Локально требуется Go 1.25+, `make`. Для ручной проверки — локальный PostgreSQL 17 с установленным
расширением `pg_stat_statements` и `jit=on`. CI-матрица PG14–18 — на стороне CI после push.

**Edge cases:**
- Локально без PostgreSQL exec-тесты (версионная матрица) гейтятся `t.Skipf` — это ожидаемо,
  не считать за провал. Реальная версионная проверка — на CI.
- PG14 (ниже `MinRequiredVersion PostgresV15`) → под-экран должен отдавать «statistics is not
  supported by current version of Postgres», не пустой `View{}`, без паники.
- `jit=off` / нет запросов выше `jit_above_cost` → экран пуст по дизайну (фильтр
  `WHERE jit_functions > 0`); подтверждается наличие хинта в `Msg`, а не баг.
- Count-тесты (`TestNew`, `Test_filterViews`) ловятся локально без PG — если они красные, это
  реальный провал, а не skip (урок фичи 006).

**Implementation hints:**
- Источник критериев приёмки — секция «Критерии приёмки» user-spec и «Acceptance Criteria»
  tech-spec; «Agent Verification Plan» tech-spec задаёт, что проверяется автоматикой, а что
  вручную/на CI.
- Ручную проверку JIT-генерации можно спровоцировать тяжёлым аналитическим запросом с
  `SET jit_above_cost = 0` (см. user-spec «Пользователь проверяет»).
- Если найден дефект — зафиксировать (что, где, чем подтверждается) и вернуть в итоге QA; не
  править код в этой задаче.

## Reviewers

Нет — QA/верификационная задача. Верификация = сам результат QA (вердикт по чек-листу).

## Post-completion

- [ ] Записать краткий отчёт в [007-feat-pg-stat-statements-jit-decisions.md](docs/features/007-feat-pg-stat-statements-jit/007-feat-pg-stat-statements-jit-decisions.md) (Summary: вердикт QA + статус критериев, без дампов)
- [ ] Если найдены отклонения от спека или дефекты — описать их и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось по итогам QA
