---
status: planned                    # planned -> in_progress -> done
depends_on: ["01"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 2                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 02: View registration — internal/view/view.go

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Регистрируем два новых view для экрана `pg_stat_io` в слое отображения. Экран `pg_stat_io`
доступен с PostgreSQL 16+ и разбит на два под-экрана (Decision 1): `stat_io` (счётчики операций +
KiB-пропускная способность, count) и `stat_io_time` (тайминги операций, time). Это аналогично тому,
как `pg_stat_statements` разбит на под-экраны, а `replslots` — version-aware multi-row view.

Эта задача делает ровно две вещи в `internal/view/view.go`:
1. Добавляет два элемента в статическую карту `view.New()` с PG14-default-значениями полей (как у
   `bgwriter`/`replslots`: значения, которые `Configure()` потом перезапишет per-version, плюс
   стабильные поля `OrderKey`/`UniqueKey`/`NotRecordable`/`MinRequiredVersion`/`Msg`).
2. Добавляет два `case` в `Views.Configure()`, которые через селекторы `query.SelectStatIOQuery` и
   `query.SelectStatIOTimeQuery` (созданы в Task 01) выставляют version-aware `QueryTmpl`, `Ncols`,
   `DiffIntvl`.

После switch'а `Configure()` уже сам прогоняет `query.Format(view.QueryTmpl, opts)` для каждого view
(существующий цикл, строки 360–367) — отдельно вызывать форматирование не нужно.

Задача зависит от Task 01: селекторы `query.SelectStatIOQuery(version) (string, int, [2]int)` /
`query.SelectStatIOTimeQuery(version) (string, int, [2]int)`, константа `query.PostgresV16` и
константы запросов (например `query.PgStatIO_*`) должны уже существовать. Имена view (`stat_io`,
`stat_io_time`) и имена селекторов — это фиксированный контракт между задачами (Decision 7
"Naming contract").

Навигация (`j`/`J`), меню (`menuStatIO`), help и `switchViewTo`-токен `"statio"` — НЕ в этой задаче,
они в Task 3 (`top/`).

## What to do

1. В `view.New()` (статическая карта) добавь два элемента, по образцу существующих
   `replslots`/`bgwriter`:

   **`"stat_io"` (count):**
   - `Name: "stat_io"`
   - `MinRequiredVersion: query.PostgresV16`
   - `QueryTmpl:` PG16-default-константа запроса из Task 01 (значение, которое `Configure()`
     перезапишет per-version; используй ту же константу, что селектор возвращает для PG16/17 —
     загляни в `internal/query/io.go`)
   - `DiffIntvl: [2]int{4, 14}`
   - `Ncols: 16`
   - `OrderKey: 4`
   - `OrderDesc: true`
   - `UniqueKey: 0` (синтетическая колонка `io_key`)
   - `ColsWidth: map[int]int{}`
   - `Msg:` короткое описание count-экрана для cmdline, например `"Show pg_stat_io operations statistics"`
   - `Filters: map[int]*regexp.Regexp{}`
   - `NotRecordable: true`

   **`"stat_io_time"` (time):**
   - `Name: "stat_io_time"`
   - `MinRequiredVersion: query.PostgresV16`
   - `QueryTmpl:` PG16-default time-константа запроса из Task 01
   - `DiffIntvl: [2]int{4, 8}`
   - `Ncols: 10`
   - `OrderKey: 4`
   - `OrderDesc: true`
   - `UniqueKey: 0`
   - `ColsWidth: map[int]int{}`
   - `Msg:` time-экран должен нести подсказку про `track_io_timing` (Decision 9), например
     `"pg_stat_io timings (require track_io_timing=on)"`
   - `Filters: map[int]*regexp.Regexp{}`
   - `NotRecordable: true`

2. В `Views.Configure()` в per-view `switch` (рядом с `case "replslots":`) добавь два case'а:
   - `case "stat_io":` → `view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatIOQuery(opts.Version)`; затем `v[k] = view`
   - `case "stat_io_time":` → `view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatIOTimeQuery(opts.Version)`; затем `v[k] = view`

3. Обнови существующие счётчики количества view в `internal/view/view_test.go`:
   `TestNew` (`assert.Equal(t, 24, len(v))` → 26) и `TestView_VersionOK` (две версии-строки
   `{version: 140000, total: 24}` и `{version: 130000, ...}` и т.д.) — на PG14/15 новые view НЕ
   проходят `VersionOK` (`MinRequiredVersion = PostgresV16 = 160000`), поэтому `total` для версий
   <16 НЕ меняется, а для версий ≥16 в таблице (если они там есть) увеличивается на 2. Проверь
   фактические числа после изменения и при необходимости добавь строки PG16+.

4. Добавь новый guard-тест для обоих view (по образцу `TestNew_ReplslotsView` /
   `TestNew_BgwriterView`), пиннящий стабильные поля карты.

## TDD Anchor

Тесты пишем/обновляем ДО кода (либо вместе с регистрацией — это unit-уровень слоя view). Пишем →
запускаем → убеждаемся что падают → правим код → убеждаемся что проходят.

- `internal/view/view_test.go::TestNew_StatIOView` — `v["stat_io"]` зарегистрирован: `NotRecordable==true`, `MinRequiredVersion==query.PostgresV16`, `Ncols==16`, `DiffIntvl==[2]int{4,14}`, `OrderKey==4`, `OrderDesc==true`, `UniqueKey==0`, `Msg` непустой (pinning PG16-default карты, как `TestNew_ReplslotsView`).
- `internal/view/view_test.go::TestNew_StatIOTimeView` — `v["stat_io_time"]` зарегистрирован: `NotRecordable==true`, `MinRequiredVersion==query.PostgresV16`, `Ncols==10`, `DiffIntvl==[2]int{4,8}`, `OrderKey==4`, `OrderDesc==true`, `UniqueKey==0`, `Msg` содержит подсказку про `track_io_timing`.
- `internal/view/view_test.go::TestNew` — обновлённый счётчик: `len(New())==26` (было 24, +2 новых view).
- `internal/view/view_test.go::TestView_VersionOK` — на версиях <16 (`140000`, `130000`, …) `total` НЕ увеличивается (новые view гейтятся `MinRequiredVersion=160000`); пересчитать ожидаемые `total` после добавления view.
- `internal/view/view_test.go::TestViews_Configure` — после `views.Configure(opts)` для `stat_io`/`stat_io_time` поле `Query` непусто (покрыто существующим финальным циклом `for _, v := range views { assert.NotEqual(t, "", v.Query) }`); убедиться, что новые view не ломают этот инвариант на всех версиях из матрицы (включая <16).

## Acceptance Criteria

- [ ] View `stat_io` и `stat_io_time` зарегистрированы в `view.New()` с `NotRecordable: true`, `MinRequiredVersion: query.PostgresV16`, `UniqueKey: 0`, `OrderKey: 4`, `OrderDesc: true`, `ColsWidth: map[int]int{}`, `Filters: map[int]*regexp.Regexp{}`.
- [ ] `stat_io`: `DiffIntvl == [2]int{4, 14}`, `Ncols == 16` (PG16-default в карте).
- [ ] `stat_io_time`: `DiffIntvl == [2]int{4, 8}`, `Ncols == 10` (PG16-default в карте); `Msg` несёт подсказку про `track_io_timing` (Decision 9).
- [ ] В `Configure()` есть два case'а, вызывающие `query.SelectStatIOQuery(opts.Version)` и `query.SelectStatIOTimeQuery(opts.Version)` и присваивающие `QueryTmpl`/`Ncols`/`DiffIntvl`.
- [ ] `TestNew` обновлён до корректного количества view (26); guard-тесты `TestNew_StatIOView`/`TestNew_StatIOTimeView` добавлены и проходят.
- [ ] `TestView_VersionOK`: новые view не считаются доступными на PG <16; ожидаемые `total` пересчитаны.
- [ ] `go test ./internal/view/...` зелёный; `go build ./...` без ошибок.
- [ ] Нет регрессий в существующих тестах слоя view.

## Context Files

**Feature artifacts:**
- [006-feat-pg-stat-io.md](docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io.md) — user-spec
- [006-feat-pg-stat-io-tech-spec.md](docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io-tech-spec.md) — tech-spec (Decisions 1,2,4,8,9,10,11; Data Models; Architecture "How it works")
- [006-feat-pg-stat-io-code-research.md](docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io-code-research.md) — code-research (§2 view registration & wiring, с line-references)
- [006-feat-pg-stat-io-decisions.md](docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io-decisions.md) — decisions log (создаётся при выполнении)

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — features, supported stats, target audience (проектный контекст; в этом репо роль project.md выполняет overview.md)
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow, PG version handling
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — Version-Specific Query Pattern, "Wire selector into view.Configure()", multi-row view (replslots) testing conventions

**Code files:**
- [internal/view/view.go](internal/view/view.go) — изменяем: `New()` (+2 элемента карты), `Configure()` (+2 case'а)
- [internal/view/view_test.go](internal/view/view_test.go) — изменяем: счётчики (`TestNew`, `TestView_VersionOK`) + новые guard-тесты
- [internal/query/io.go](internal/query/io.go) — читаем: имена селекторов, сигнатуры, имена PG16-default констант запросов (из Task 01)

## Verification Steps

- Шаг 1: `go build ./...` — компилируется без ошибок (имена селекторов/констант из Task 01 совпадают).
- Шаг 2: `go test ./internal/view/...` — все тесты зелёные, включая обновлённые `TestNew`/`TestView_VersionOK` и новые guard-тесты `TestNew_StatIOView`/`TestNew_StatIOTimeView`.
- Шаг 3: убедиться, что `TestViews_Configure` проходит на всей матрице версий (новые view не дают пустой `Query` и не падают на PG <16).

## Details

<!-- All details for task execution — technical, organizational, any other. -->

**Files:**
- `internal/view/view.go`
  - Текущее состояние: `View` struct (строки 10–31) с полями `Name, MinRequiredVersion, QueryTmpl, DiffIntvl, Ncols, OrderKey, OrderDesc, UniqueKey, ColsWidth, Msg, Filters, NotRecordable`. `New()` (строки 37–323) — статическая карта; последний элемент `procpidstat` заканчивается на строке 321, закрывающая `}` карты на 323. `Configure()` (строки 328–370) — per-view `switch` (334–356), за ним общий Format-цикл (360–367). `replslots` зарегистрирован на 153–165 (точный шаблон для новых count/time view: multi-row, `OrderKey:4`, `NotRecordable`, `UniqueKey`). `replslots`-case в Configure — строки 353–355 (точный шаблон для двух новых case'ов с сигнатурой `(string, int, [2]int)`).
  - Что сделать: добавить два элемента в карту (рекомендуется рядом с `replslots`/`bgwriter`, но место в карте не важно — она статическая), добавить два case'а в Configure-switch рядом с `case "replslots":`.
- `internal/view/view_test.go`
  - Текущее состояние: `TestNew` (строка 11) пиннит `len(v)==24`. `TestNew_ReplslotsView` (16–28) и `TestNew_BgwriterView` (32–42) — образцы guard-тестов. `TestView_VersionOK` (169–192) — таблица `{version,total}`: `{140000,24},{130000,19},{120000,16},{110000,14},{100000,14}`. `TestViews_Configure` (44–167) заканчивается инвариантом `assert.NotEqual(t, "", v.Query)` для всех view.
  - Что сделать: обновить `24 → 26` в `TestNew`; добавить `TestNew_StatIOView` и `TestNew_StatIOTimeView`; пересчитать/дополнить `TestView_VersionOK`. ВАЖНО: в существующей таблице `TestView_VersionOK` максимальная версия = `140000`, на ней новые view (min 160000) НЕ доступны → `total` для `140000` остаётся `24` (НЕ менять). Чтобы покрыть доступность новых view, опционально добавь строку `{version: 160000, total: 26}` (на PG16 доступны все 24 старых + 2 новых; проверь, что среди существующих 24 нет ничего с min >14, кроме уже посчитанных — все существующие view доступны на 140000, значит на 160000 их тоже 24, плюс 2 новых = 26).

**Dependencies:**
- Task 01 (wave 1) — должен быть выполнен первым: предоставляет `query.SelectStatIOQuery`, `query.SelectStatIOTimeQuery` (сигнатура `(string, int, [2]int)`, как `SelectStatReplicationSlotsQuery`), константу `query.PostgresV16` и PG16-default-константы запросов. Прочитай `internal/query/io.go`, чтобы взять точные имена констант для поля `QueryTmpl` в карте.
- Никаких новых пакетов.

**Edge cases:**
- PG14/15: view зарегистрированы, но `VersionOK` (`version >= MinRequiredVersion`) вернёт false → коллектор отдаст стандартную "not supported" ошибку (это поведение Task 5/коллектора, в этой задаче только проверь, что `MinRequiredVersion = PostgresV16` и тест `TestView_VersionOK` корректно это отражает).
- `Configure()` форматирует `QueryTmpl` для ВСЕХ view на любой версии (включая <16). PG16-default-константа в `QueryTmpl` должна быть Format-safe (обычный SELECT без падающих template-полей) — это обеспечивается Task 01; здесь убедись лишь, что `TestViews_Configure` не падает на версиях <16.
- `OrderKey: 4` < `Ncols` (16 и 10) — валиден как правая граница сортировки.
- `UniqueKey: 0` указывает на синтетическую колонку `io_key` — НЕ перепутать с `OrderKey`.

**Implementation hints:**
- Шаблон элемента карты — копируй `replslots` (153–165): он уже имеет `OrderKey:4`, `OrderDesc:true`, `UniqueKey`-семантику (replslots использует дефолтный 0), `NotRecordable:true`, `MinRequiredVersion` через `query.PostgresVxx`-константу. Отличие: у новых view `UniqueKey: 0` указан явно (синтетический ключ), а `Msg` у time-экрана несёт подсказку про `track_io_timing`.
- Шаблон case'а в Configure — копируй `case "replslots":` (353–355): идентичная сигнатура `(string, int, [2]int)` и присвоение `view.QueryTmpl, view.Ncols, view.DiffIntvl = ...`, затем `v[k] = view`.
- Не трогай общий Format-цикл (360–367) — он уже обрабатывает новые view автоматически.
- Числа `Ncols`/`DiffIntvl` в карте — это PG16/17-default (count: 16/[4,14], time: 10/[4,8]); `Configure()` подтвердит их через селектор. По tech-spec Decision 3 форма (Ncols, DiffIntvl) одинакова для PG16/17 и PG18 — селектор вернёт те же числа на обеих ветках.
- Не реализуй навигацию/меню/help (`j`/`J`, `menuStatIO`, `statioNextView`, `switchViewTo "statio"`) — это Task 3.

## Reviewers

- **dev-code-reviewer** → `docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io-task-02-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io-task-02-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [006-feat-pg-stat-io-decisions.md](docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
