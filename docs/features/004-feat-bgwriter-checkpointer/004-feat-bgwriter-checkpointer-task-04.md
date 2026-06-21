---
status: planned                    # planned -> in_progress -> done
depends_on: ["01", "02", "03"]     # ID задач-зависимостей
wave: 3                            # волна параллельного выполнения
skills: [pre-deploy-qa]            # МАССИВ скиллов для загрузки
verify: bash                       # `make test` && `make lint` && `make build` all pass; acceptance criteria met
reviewers: []                      # QA-задача — ревьюеров нет
teammate_name:
---

# Task 04: Pre-deploy QA

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:pre-deploy-qa` — [skills/pre-deploy-qa/SKILL.md](~/.claude/skills/pre-deploy-qa/SKILL.md)

## Description

Финальная приёмочная проверка фичи `pg_stat_bgwriter + pg_stat_checkpointer screen` перед выпуском.
Это завершающая задача волны: Task 1 (query-слой), Task 2 (overview.md) и Task 3 (view/keybinding/help)
уже выполнены и должны быть в дереве. Задача — прогнать весь набор проверок (`make test` + `make lint`
+ `make build`) и провалидировать **каждый** критерий приёмки из user-spec и tech-spec: корректный
набор колонок / `Ncols` / `DiffIntvl` по версиям PG, абсолютные счётчики событий против дифф-колонок,
pass-through `stats_age`, соблюдение `NotRecordable` (экран не собирается командой `record`),
работа горячей клавиши `b` и её наличие в справке `?`, исправленный `overview.md`, а также верификация
колонки `slru_written` на живом PG18.

Результат задачи — **не код**, а вердикт приёмки: список критериев со статусом pass/fail и подтверждающими
данными (вывод тестов, конкретные значения `Ncols`/`DiffIntvl`, факт исполнения PG18-ветки запроса).
Если что-то не сходится — фиксируем точное расхождение, чтобы вернуть на доработку соответствующую задачу.

## What to do

1. Убедиться, что изменения всех трёх задач волны присутствуют в рабочем дереве (новые файлы
   `internal/query/bgwriter.go`, `internal/query/bgwriter_test.go`; правки в `internal/view/view.go`,
   `top/keybindings.go`, `top/help.go`, `record/record.go`, `.claude/skills/project-knowledge/overview.md`).
2. Прогнать `make test` — собрать и выполнить весь юнит- и интеграционный набор (race + coverage),
   включая PG14–18 на доступных контейнерах. Зафиксировать, какие версии реально исполнялись, а какие
   были пропущены (`t.Skipf`).
3. Прогнать `make lint` (golangci-lint + gosec) и `make build`. Оба должны пройти без замечаний/ошибок.
4. Провалидировать каждый критерий приёмки из user-spec и tech-spec (см. блок Acceptance Criteria ниже),
   опираясь на код и вывод тестов: набор колонок и `Ncols`/`DiffIntvl` по версиям, размещение абсолютных
   счётчиков вне `DiffIntvl` и дифф-блока внутри, pass-through `stats_age`, `NotRecordable: true` у view,
   биндинг клавиши `b`, её наличие в строке справки `a,b,f,r,w`, корректность `overview.md`.
5. Отдельно подтвердить, что PG18-ветка запроса (с `slru_written`) реально **исполнилась** на живом PG18
   (не была молча пропущена `t.Skipf`). Если локально PG18 недоступен — явно отметить это и указать, что
   гейт закрывается в CI на матрице PG14–18 (`lesovsky/pgcenter-testing`).
6. Сформировать итоговый вердикт приёмки: каждый критерий — pass/fail с подтверждением. При любом fail —
   указать конкретную задачу-источник проблемы и характер расхождения.

## Acceptance Criteria

- [ ] `make test` проходит зелёным (race + coverage); зафиксировано, какие версии PG исполнены, какие пропущены.
- [ ] `make lint` проходит без замечаний.
- [ ] `make build` собирает бинарь без ошибок.
- [ ] `SelectStatBgwriterQuery` возвращает корректные `(query, Ncols, DiffIntvl)` для PG 14/15/16/17/18:
      PG14–16 → `Ncols=12`, `DiffIntvl=[3,10]`; PG17 → `Ncols=13`, `DiffIntvl=[6,11]`; PG18 → `Ncols=14`,
      `DiffIntvl=[6,12]` (подтверждено юнит-тестами).
- [ ] Запрос исполняется без ошибок на каждом доступном контейнере PG 14–18 (интеграция зелёная или
      пропущена только при недоступности версии).
- [ ] PG18-ветка запроса (включая `slru_written`) реально исполнена на живом PG18 (не пропущена) —
      локально либо в CI; факт подтверждён.
- [ ] Счётчики событий (`ckpt_*`, `rstpt_*`) находятся вне `DiffIntvl` (абсолютные кумулятивные);
      колонки объёма/времени — внутри `DiffIntvl` (дельта за интервал); `stats_age` — последняя, вне диффа (pass-through текст).
- [ ] Набор колонок PG14–16 и PG17+ соответствует user-spec (см. «Критерии приёмки» user-spec).
- [ ] Экран НЕ собирается командой `pgcenter record`: view помечен `NotRecordable: true`, и это поведение
      соблюдается фильтрацией view (`filterViews`); отчёт не ломается.
- [ ] Горячая клавиша `b` переключает на экран `bgwriter`; `b` присутствует в строке справки `?` в ряду `a,b,f,r,w`.
- [ ] `overview.md` исправлен: убрано ложное заявление о готовой поддержке `pg_stat_bgwriter`, добавлено
      упоминание нового экрана bgwriter/checkpointer.
- [ ] Регрессий в существующих тестах нет.
- [ ] Сформирован итоговый вердикт приёмки (по каждому критерию — pass/fail с подтверждением).

## Context Files

**Feature artifacts:**
- [004-feat-bgwriter-checkpointer.md](004-feat-bgwriter-checkpointer.md) — user-spec (Критерии приёмки, Как проверить)
- [004-feat-bgwriter-checkpointer-tech-spec.md](004-feat-bgwriter-checkpointer-tech-spec.md) — tech-spec (Acceptance Criteria, Agent Verification Plan, Data Models)
- [004-feat-bgwriter-checkpointer-decisions.md](004-feat-bgwriter-checkpointer-decisions.md) — decisions log

**Project knowledge:**
- [project.md](.claude/skills/project-knowledge/overview.md) — overview (commands, supported stats, PG version support)
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow, PG version handling
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — testing conventions, version branching
- [overview.md](.claude/skills/project-knowledge/overview.md) — должен быть исправлен Task 2; проверяется здесь

**Code files (проверяются, не модифицируются):**
- [internal/query/bgwriter.go](internal/query/bgwriter.go) — селектор `SelectStatBgwriterQuery`, query-константы по версиям
- [internal/query/bgwriter_test.go](internal/query/bgwriter_test.go) — юнит + интеграционные тесты
- [internal/view/view.go](internal/view/view.go) — view-entry `bgwriter` с `NotRecordable: true`, ветка `Configure()`
- [top/keybindings.go](top/keybindings.go) — биндинг клавиши `b`
- [top/help.go](top/help.go) — строка справки с `b` (ряд `a,b,f,r,w`)
- [record/record.go](record/record.go) — `filterViews`, обновлённый комментарий-пример про `NotRecordable`

## Verification Steps

- Прогнать `make test` — все юнит- и интеграционные тесты зелёные; зафиксировать исполненные/пропущенные версии PG.
- Прогнать `make lint` — без замечаний.
- Прогнать `make build` — бинарь собирается.
- По коду `internal/query/bgwriter.go` и тестам подтвердить значения `Ncols`/`DiffIntvl` по версиям и
  размещение абсолютных счётчиков вне `DiffIntvl`, дифф-блока — внутри, `stats_age` — вне.
- В `internal/view/view.go` подтвердить `NotRecordable: true` у entry `bgwriter`; в `record/record.go`
  подтвердить, что `filterViews` исключает такие view.
- В `top/keybindings.go` подтвердить биндинг `b → switchViewTo(app, "bgwriter")`; в `top/help.go` —
  наличие `b` в ряду `a,b,f,r,w`.
- В `.claude/skills/project-knowledge/overview.md` подтвердить отсутствие ложного заявления и наличие
  упоминания нового экрана.
- Подтвердить, что PG18-ветка запроса исполнилась на живом PG18 (вывод интеграционного теста), либо явно
  отметить, что закрывается в CI-матрице.
- Ожидаемый результат: все критерии приёмки — pass; итоговый вердикт приёмки сформирован.

## Details

**Files:** изменений в код не вносится — задача только верифицирует. Все правки сделаны задачами 1–3.

**Dependencies:**
- Зависит от Task 01 (query-слой + тесты), Task 02 (overview.md), Task 03 (view + keybinding + help + record комментарий).
  Все три должны быть завершены и присутствовать в дереве до запуска QA.
- Тулинг: `make test` (race + coverage), `make lint` (golangci-lint + gosec), `make build`. Интеграционные
  тесты используют контейнеры PG 14–18 (`lesovsky/pgcenter-testing`); недоступные версии пропускаются `t.Skipf`.

**Edge cases:**
- Локальная среда может не иметь всех контейнеров PG14–18 — часть интеграционных тестов пропустится
  `t.Skipf`. Это допустимо для локального прогона, НО PG18-ветка с `slru_written` должна быть подтверждена:
  если PG18 локально нет, явно зафиксировать, что верификация колоночного набора PG18 закрывается реальным
  исполнением в CI (матрица там полная), а не молчаливым пропуском.
- `stats_reset IS NULL` (свежий кластер): `stats_age` пустой — приемлемо, как у `pg_stat_wal`.
- Внешний `pg_stat_reset_shared(...)` во время сессии → одно-тиковая отрицательная дельта — принятое
  существующее поведение, не баг данной фичи.
- На PG17+ колонки `buf_backend`/`buf_backend_fsync` отсутствуют (ушли в `pg_stat_io`); на PG≤16 — есть.

**Implementation hints:**
- Эталонные значения по версиям (из Data Models tech-spec): PG14–16 — `Ncols=12`, `DiffIntvl=[3,10]`,
  абсолютные `ckpt_timed/ckpt_req` (индексы 1–2), дифф-блок 3..10, `stats_age` индекс 11. PG17 — `Ncols=13`,
  `DiffIntvl=[6,11]`, абсолютные `ckpt_timed/ckpt_req/rstpt_timed/rstpt_req/rstpt_done` (индексы 1–5),
  дифф-блок 6..11, `stats_age` индекс 12. PG18 — `Ncols=14`, `DiffIntvl=[6,12]`, добавлен `slru_written`
  в дифф-блок, `stats_age` индекс 13.
- Граница выбора ветки в `SelectStatBgwriterQuery`: `>= 180000` → PG18; `>= 170000` → PG17; иначе → PG14–16.
- `NotRecordable` — существующее поле `View`; bgwriter — первый его реальный пользователь. Проверять, что
  `filterViews` в `record/record.go` действительно исключает view с этим флагом.
- Для подтверждения PG18-исполнения смотреть на отсутствие `SKIP` у соответствующего подтеста в выводе
  `make test` (либо лог CI).

## Reviewers

Нет — это QA-задача (skill `pre-deploy-qa`), ревьюеры не назначаются.

## Post-completion

- [ ] Записать краткий отчёт приёмки в [004-feat-bgwriter-checkpointer-decisions.md](004-feat-bgwriter-checkpointer-decisions.md)
      (Summary: 1–3 предложения; результат `make test`/`make lint`/`make build`; вердикт по критериям приёмки;
      какие версии PG исполнены/пропущены; статус верификации PG18 `slru_written`).
- [ ] Если какой-либо критерий — fail: описать расхождение и указать задачу-источник для доработки.
- [ ] Если по ходу QA выявлено отклонение от спека — описать отклонение и причину; обновить user-spec/tech-spec
      при необходимости.
