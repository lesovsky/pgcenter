---
status: done                       # planned -> in_progress -> done
depends_on: ["02", "03"]           # ID задач-зависимостей
wave: 3                            # волна параллельного выполнения
skills: [pre-deploy-qa]            # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: []                      # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 04: Pre-deploy QA

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:pre-deploy-qa` — [skills/pre-deploy-qa/SKILL.md](~/.claude/skills/pre-deploy-qa/SKILL.md)

## Description

Финальная волна фичи `pg_stat_io` — приёмочное тестирование перед деплоем. После завершения
Wave 1 (query-слой, Task 01) и Wave 2 (регистрация вьюх Task 02 + TUI-навигация/меню/help Task 03)
нужно собрать всё вместе и убедиться, что фича удовлетворяет критериям приёмки из
user-spec («Критерии приёмки», US1–US4) и техническим критериям из tech-spec («Acceptance
Criteria»).

Задача состоит из двух частей:
1. **Автоматическая** — прогон полного набора инструментов сборки и проверок (`make build`,
   `make test`, `make lint`, `make vuln`) и подтверждение, что юнит-тесты селекторов, NULL-safety
   и теста меню зелёные.
2. **Ручная** — прохождение пользовательских историй US1–US4 на живом PostgreSQL 17 (и PG18, если
   доступен), проверка деградации на PG14/15.

Важно: реальный гейт фичи — поведение на **живом PG18** (нативные `*_bytes` типа `numeric`,
строки `object='wal'`, отсутствие `op_bytes`). Это внешняя зависимость, которая локально (PG17)
не верифицируется, а проверяется только в CI на матрице PG14–18. Если PG18 локально недоступен —
это нужно явно зафиксировать в отчёте как непроверенный пункт, который закрывает CI.

## What to do

1. Загрузить skill `pre-deploy-qa` и следовать его процессу приёмочного тестирования.
2. Прогнать автоматические проверки и подтвердить, что все зелёные:
   - `make build` — бинарь `./bin/pgcenter` собирается без ошибок.
   - `make test` — race-детектор + coverage; юнит/интеграционные тесты зелёные (локально PG17;
     недоступные версии помечаются `t.Skipf`).
   - `make lint` — golangci-lint + gosec без ошибок.
   - `make vuln` — govulncheck без находок.
3. Подтвердить технические критерии приёмки из tech-spec по результатам тестов:
   - Юнит-тесты `SelectStatIOQuery` / `SelectStatIOTimeQuery` проходят на версиях {14,15,16,17,18}
     с корректными `(Ncols, DiffIntvl)` (count: 16 / `[4,14]`; time: 10 / `[4,8]`; PG16/17 и PG18
     дают одинаковую форму).
   - NULL-safety тест: строка с `NULL` в diff-колонке не обрушивает сэмпл (NULL→0).
   - `selectMenuStyle(menuStatIO)` возвращает ровно 2 пункта.
   - Вьюхи `stat_io` / `stat_io_time` зарегистрированы с корректными полями (`NotRecordable:true`,
     `MinRequiredVersion=PostgresV16`, `UniqueKey:0`, `OrderKey:4`, `OrderDesc:true`).
4. Пройти ручной обход пользовательских историй US1–US4 на живом PG17 (см. чеклист ниже).
5. Подтвердить деградацию на PG14/15: вход на экран даёт штатное сообщение «not supported»,
   без паники и без пустого экрана.
6. Если доступен PG18 — подтвердить наличие строк `object='wal'` и нативных `*_bytes`.
   Если недоступен — зафиксировать как CI-гейт.
7. Сформировать итоговый QA-вердикт (PASS / FAIL с перечнем проблем) и записать его в decisions log.

## Verification Checklist

Приёмочный чеклист (заменяет TDD Anchor — QA-задача):

**Автоматические проверки:**
- [ ] `make build` — `./bin/pgcenter` собран без ошибок.
- [ ] `make test` — зелёный (учитывая известный pre-existing `Test_doReload` локально, см. Edge cases).
- [ ] `make lint` — зелёный (golangci-lint + gosec).
- [ ] `make vuln` — govulncheck без находок.
- [ ] `go test ./internal/query/...` — селектор (Ncols/DiffIntvl) на всех версиях + NULL-safety.
- [ ] `go test ./internal/view/...` — вьюхи сконфигурированы.
- [ ] `go test ./top/...` — `menuStatIO` содержит 2 пункта; нет регрессий в существующих view/menu/query-тестах.

**Ручной обход US1–US4 (живой PG17):**
- [ ] US1 (триаж IO): `j` открывает count-экран; строки `backend_type/object/context` с рейтами;
      сортировка по `reads` (убыв.) по умолчанию; `/` фильтр по `backend_type` (напр. `autovacuum`) изолирует строки.
- [ ] US3 (латентность): повторное `j` → time-экран; сортировка по `fsync_time`; в командной строке
      подсказка про `track_io_timing`.
- [ ] `J` открывает меню из 2 пунктов (`pg_stat_io operations` / `pg_stat_io timings`); выбор пункта переключает экран.
- [ ] Набор строк одинаков на count- и time-экранах; пустые/all-zero строки скрыты.

**Деградация / версионные пути:**
- [ ] PG14/15: вход на экран → `ERROR: selected statistics is not supported by current version of Postgres`, без паники, без пустого экрана.
- [ ] US2/PG18 (если доступен): строки `object='wal'` присутствуют; нативные `*_bytes` отображаются как integer (без десятичных). Если PG18 недоступен локально — зафиксировано как CI-гейт.

## Acceptance Criteria

- [ ] `make build`, `make test`, `make lint`, `make vuln` — все зелёные.
- [ ] Юнит-тесты селекторов проходят на {14,15,16,17,18}; NULL-safety тест проходит; `menuStatIO` = 2 пункта.
- [ ] Вьюхи `stat_io`/`stat_io_time` зарегистрированы с корректными полями (см. Verification Checklist).
- [ ] US1–US4 пройдены на живом PG17: `j` toggle, `J` меню, `/` фильтр, сортировка по `reads`, переключение count↔time, подсказка `track_io_timing`.
- [ ] PG14/15 показывает «not supported» без паники.
- [ ] PG18 (если доступен) — строки `object='wal'` + нативные bytes; иначе явно помечено как CI-гейт.
- [ ] Нет регрессий в существующих view/menu/query-тестах.
- [ ] Сформирован QA-вердикт (PASS/FAIL) и записан в decisions log.

## Context Files

**Feature artifacts:**
- [006-feat-pg-stat-io.md](docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io.md) — user-spec (Критерии приёмки, US1–US4, «Как проверить»)
- [006-feat-pg-stat-io-tech-spec.md](docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io-tech-spec.md) — tech-spec (Acceptance Criteria, Testing Strategy, Agent Verification Plan)
- [006-feat-pg-stat-io-decisions.md](docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — overview: команды, поддерживаемые версии PG (14–18)
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow, PG version handling
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — testing conventions, version branching
- [deployment.md](.claude/skills/project-knowledge/deployment.md) — CI matrix PG14–18, тестовый контейнер, порты

**Code files:**
- [internal/query/io_test.go](internal/query/io_test.go) — селектор + NULL-safety тесты (из Task 01)
- [internal/view/view_test.go](internal/view/view_test.go) — конфигурация вьюх (из Task 02)
- [top/menu_test.go](top/menu_test.go) — тест количества пунктов `menuStatIO` (из Task 03)
- [internal/postgres/testing.go](internal/postgres/testing.go) — `NewTestConnectVersion`, port map PG14–18

## Verification Steps

- Шаг 1: выполнить `make build` — ожидается успешная сборка `./bin/pgcenter`.
- Шаг 2: выполнить `make test`, `make lint`, `make vuln` — ожидается зелёный результат
  (учесть pre-existing `Test_doReload` локально — см. Edge cases).
- Шаг 3: запустить целевые пакетные тесты `go test ./internal/query/... ./internal/view/... ./top/...`
  и убедиться в прохождении селекторов, NULL-safety, конфигурации вьюх и теста меню.
- Шаг 4: запустить `./bin/pgcenter top` против живого PG17 и пройти US1–US4 + проверку PG14/15
  (по чеклисту Verification Checklist).
- Шаг 5: если доступен PG18 — проверить `object='wal'` и нативные bytes; иначе пометить CI-гейт.
- Шаг 6: записать QA-вердикт в decisions log.

## Details

**Files:** изменений кода в этой задаче нет — только запуск проверок и ручной обход TUI.

**Dependencies:**
- Task 02 (регистрация вьюх `stat_io`/`stat_io_time` в `internal/view/view.go`).
- Task 03 (TUI: `j`/`J` биндинги, `menuStatIO`, `statioNextView`, help). Без обеих волн фича не
  собирается end-to-end и ручной обход невозможен.

**Edge cases:**
- `Test_doReload` может падать локально без PG17-фикстуры (pre-existing tech-debt [005]). Это не
  относится к фиче — при локально красном `make test` запускать целевые пакетные тесты
  (`internal/query`, `internal/view`, `top`) и опираться на CI. Зафиксировать факт в отчёте.
- Недоступные версии PG помечаются `t.Skipf` — это ожидаемо, не считается провалом.
- Реальный гейт PG18 (нативные `*_bytes`, `object='wal'`, отсутствие `op_bytes`) локально не
  верифицируем при PG17-only окружении — закрывается CI-прогоном PG18. Обязательно явно отметить.
- PG14/15: `pg_stat_io` не существует — вьюха гейтится `VersionOK` (`MinRequiredVersion=PostgresV16`),
  ожидается штатное сообщение «not supported», паники быть не должно.

**Implementation hints:**
- Тестовый контейнер `lesovsky/pgcenter-testing:0.0.10` содержит PG14–18 на портах
  PG14=21914 ... PG18=21918 (см. deployment.md). Для интеграционных тестов используется
  `NewTestConnectVersion`.
- При формировании вердикта различать: (a) автоматические проверки (объективный PASS/FAIL),
  (b) ручной обход US (требует живого PG17), (c) PG18-гейт (CI). Чётко указать, что проверено
  локально, а что делегировано CI.
- Не «чинить» найденные баги в этой задаче — QA фиксирует находки; исправления уходят в
  отдельный fix-цикл соответствующих задач.

## Post-completion

- [ ] Записать краткий отчёт в [006-feat-pg-stat-io-decisions.md](docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io-decisions.md) (Summary: QA-вердикт PASS/FAIL в 1-3 предложения; что проверено локально, что делегировано CI; без дампов вывода тестов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
