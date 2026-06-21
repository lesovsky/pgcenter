---
status: planned                    # planned -> in_progress -> done
depends_on: ["02"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 3                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-test-reviewer]     # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 03: Bump Test_filterViews for the new NotRecordable view

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Task 02 регистрирует новый TUI-экран `replslots` в `view.New()` с флагом `NotRecordable: true`
(Decision 6 в tech-spec — фича работает только в live-TUI в релизе 0.11.0, запись/отчёт отложены
на следующую фазу). Функция `record/record.go:filterViews()` отбрасывает любой view с
`NotRecordable == true` на каждой версии Postgres — это происходит в самой первой ветке цикла,
ДО проверки версии и до фильтра `statements_*`.

Тест `Test_filterViews` в `record/record_test.go` жёстко зафиксированными числами проверяет,
сколько views отфильтровано (`wantN`) и сколько осталось (`wantV`) для шести версий Postgres.
Поскольку `replslots` отбрасывается на каждой версии, `wantN` на каждой из шести строк должен
вырасти на 1. `wantV` НЕ меняется: NotRecordable-view никогда не попадает в оставшийся набор,
поэтому количество оставшихся views не зависит от его добавления.

Это test-only задача: продакшн-код `record.go` НЕ меняется — механизм отбрасывания
NotRecordable уже существует (добавлен в feature 004 для `bgwriter`). Меняется только тестовое
ожидание плюс соседний поясняющий комментарий.

## What to do

1. В `record/record_test.go`, в `Test_filterViews`, увеличить `wantN` на 1 в каждой из шести
   строк таблицы testcases (см. точные текущие/целевые значения в TDD Anchor ниже). `wantV`
   оставить без изменений во всех шести строках.
2. Обновить соседний блок комментария над таблицей testcases (строки с пояснением логики
   `wantN`/`wantV`): добавить упоминание вклада `replslots` в `wantN` рядом с существующей
   заметкой про `bgwriter`. Сформулировать так же, как написана заметка про bgwriter:
   `replslots` — `NotRecordable=true`, всегда отбрасывается `filterViews` на каждой версии,
   добавляет 1 к `wantN` на каждой строке, `wantV` не меняется.
3. Не трогать остальные тесты в файле (`Test_app_setup`, `Test_app_record`,
   `TestFilterViews_NotRecordable`, `TestFilterViews_dropsExplicitNotRecordable`,
   `TestFilterViews_Recordable`, `countRecordable`) — они корректно учитывают NotRecordable
   через хелпер `countRecordable` и динамику `view.New()`, поэтому не зависят от хардкода.

## TDD Anchor

Для этой задачи изменение ассерта И ЕСТЬ тест — обновлённый `Test_filterViews` сам является
TDD-якорем. После регистрации view (task 02) тест становится красным (фактический `n` теперь на
1 больше ожидаемого на каждой строке) → правим `wantN` → тест зелёный.

- `record/record_test.go::Test_filterViews` — для каждой из шести версий Postgres проверяет,
  что число отфильтрованных views (`wantN`) и число оставшихся (`wantV = len(v)`) совпадают с
  ожидаемыми. После регистрации `replslots` (NotRecordable) `wantN` каждой строки растёт на 1.

Точные изменения (текущее → целевое значение `wantN`; `wantV` неизменно):
- `{version: 140000, pgssSchema: ""}`        — `wantN: 7 → 8`,  `wantV: 16` (без изменений)
- `{version: 140000, pgssSchema: "public"}`  — `wantN: 1 → 2`,  `wantV: 22` (без изменений)
- `{version: 130000, pgssSchema: "public"}`  — `wantN: 4 → 5`,  `wantV: 19` (без изменений)
- `{version: 120000, pgssSchema: "public"}`  — `wantN: 7 → 8`,  `wantV: 16` (без изменений)
- `{version: 110000, pgssSchema: "public"}`  — `wantN: 9 → 10`, `wantV: 14` (без изменений)
- `{version: 100000, pgssSchema: "public"}`  — `wantN: 9 → 10`, `wantV: 14` (без изменений)

## Acceptance Criteria

- [ ] `wantN` увеличен ровно на 1 в каждой из шести строк testcases `Test_filterViews`.
- [ ] `wantV` не изменён ни в одной строке.
- [ ] Комментарий над таблицей testcases документирует вклад `replslots` в `wantN` рядом с
      заметкой про `bgwriter`.
- [ ] Продакшн-файл `record/record.go` не изменён.
- [ ] `make test` зелёный, в частности `Test_filterViews`.
- [ ] Остальные тесты файла (`TestFilterViews_*`, `Test_app_record`) не сломаны.

## Context Files

**Feature artifacts:**
- [005-feat-replication-slots.md](005-feat-replication-slots.md) — user-spec
- [005-feat-replication-slots-tech-spec.md](005-feat-replication-slots-tech-spec.md) — tech-spec (Task 3, Decision 6)
- [005-feat-replication-slots-decisions.md](005-feat-replication-slots-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](/home/lesovsky/Git/github.com/lesovsky/pgcenter/.claude/skills/project-knowledge/overview.md) — project context (в этом проекте PK использует overview.md, не project.md)
- [architecture.md](/home/lesovsky/Git/github.com/lesovsky/pgcenter/.claude/skills/project-knowledge/architecture.md) — package layout, data flow
- [patterns.md](/home/lesovsky/Git/github.com/lesovsky/pgcenter/.claude/skills/project-knowledge/patterns.md) — testing conventions

**Code files:**
- [record/record_test.go](/home/lesovsky/Git/github.com/lesovsky/pgcenter/record/record_test.go) — модифицировать `Test_filterViews` (wantN +1 на каждой строке) и комментарий над testcases
- [record/record.go](/home/lesovsky/Git/github.com/lesovsky/pgcenter/record/record.go) — читать: `filterViews()` (строки 200–233), ветка NotRecordable :208; НЕ менять
- [internal/view/view.go](/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/view/view.go) — читать: подтвердить, что task 02 зарегистрировал `replslots` с `NotRecordable: true` в `New()`

## Verification Steps

- Шаг 1: убедиться, что task 02 завершён — `replslots` присутствует в `view.New()` с
  `NotRecordable: true` (без этого фактический `n` не вырастет и правка ассерта сделает тест
  красным).
- Шаг 2: запустить `make test` — `Test_filterViews` зелёный на всех шести версиях.
- Шаг 3: убедиться, что `git diff` затрагивает только `record/record_test.go` (продакшн
  `record.go` не изменён).

## Details

<!-- All details for task execution — technical, organizational, any other. -->

**Files:**
- `record/record_test.go` — единственный изменяемый файл.
  - Текущее состояние таблицы testcases в `Test_filterViews` (строки 116–121): шесть строк со
    значениями `wantN` = `{7, 1, 4, 7, 9, 9}` и `wantV` = `{16, 22, 19, 16, 14, 14}`. Нужно
    поднять каждый `wantN` на 1 → `{8, 2, 5, 8, 10, 10}`. `wantV` оставить как есть.
  - Комментарий над таблицей (строки 108–115) уже описывает логику для `procpidstat` и
    `bgwriter`. Добавить параллельную заметку про `replslots` (NotRecordable=true, всегда
    отбрасывается, +1 к каждому wantN, wantV неизменно).
- `record/record.go` — НЕ менять. `filterViews()` (строки 200–233) уже отбрасывает любой view с
  `NotRecordable == true` в первой ветке цикла (строки 208–212), до version-gate и фильтра
  `statements_*`. Механизм добавлен в feature 004.
- `internal/view/view.go` — НЕ менять. Только подтвердить регистрацию `replslots`
  (зависит от task 02).

**Dependencies:**
- Task 02 — должен быть завершён первым: `replslots` обязан быть в `view.New()` с
  `NotRecordable: true`. Если task 02 не выполнен, `view.New()` не вернёт новый view и правка
  ассерта сделает тест красным.
- Пакеты: новых не требуется (testify уже подключён).

**Edge cases:**
- `wantV` НЕ меняется — частая ошибка скопировать инкремент и на `wantV`. NotRecordable-view
  никогда не попадает в оставшийся набор, так что `len(v)` не зависит от его регистрации.
- `Test_app_record` использует хелпер `countRecordable(view.New()) + 2` и динамически считает
  recordable-views, поэтому корректно вычитает NotRecordable-view сам — его трогать не нужно.
- `TestFilterViews_dropsExplicitNotRecordable` использует синтетический view, не зависит от
  `replslots`.

**Implementation hints:**
- Шесть строк правятся по одной точечным поиском по полной строке testcase — ориентируйся на
  строку целиком (version + pgssSchema + wantN + wantV), чтобы правка была однозначной (некоторые
  значения wantN/wantV повторяются между строками).
- Сверь итоговые значения с таблицей в TDD Anchor перед запуском `make test`.

## Reviewers

- **dev-test-reviewer** → `005-feat-replication-slots-task-03-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [005-feat-replication-slots-decisions.md](005-feat-replication-slots-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
