---
status: done                       # planned -> in_progress -> done
depends_on: ["01"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 2                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 02: Register statements_jit view + Configure + count-test fixes

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Регистрируем новый экран `statements_jit` (7-й под-экран `pg_stat_statements`, JIT-компиляция)
как полноценный view в pgcenter: добавляем запись в карту `view.New()` и ветку в
`view.Views.Configure()`, которая на старте подставляет в view версионно-выбранный query и его
layout-метаданные (`QueryTmpl`/`Ncols`/`DiffIntvl`/`UniqueKey`).

View завязан на селектор `query.SelectStatStatementsJITQuery(version)` и JIT-консты из задачи 01
(зависимость по Wave 1). View гейтится `MinRequiredVersion: query.PostgresV15`, помечается
`NotRecordable: true` (TUI-first принцип релиза 0.11.0, прямой прецедент —
`bgwriter`/`replslots`/`stat_io`), сортируется по `gen_total` desc (`OrderKey: 2`,
`OrderDesc: true`), а `Msg` несёт подсказку про пустой экран при `jit=off`.

Поскольку JIT-колонки и их количество отличаются между PG15/16 (13 колонок) и PG17+ (15 колонок),
`Configure()` должна патчить все четыре поля (как существующая ветка `stat_io`, которая зовёт
`SelectStatIOQuery` и патчит `QueryTmpl`/`Ncols`/`DiffIntvl`), а не только `QueryTmpl` как ветка
`statements_timings` (view.go:373) — у timings количество колонок неизменно (13), а у JIT меняется.

Добавление любого view ломает count-тесты, которые считают общее число view — их нужно
подправить: `view_test.go::TestNew` (26 → 27) и `record/record_test.go::Test_filterViews`
(`wantN +1` на всех 6 строках, потому что `NotRecordable` отбрасывает view ещё ДО версионного
гейта в `filterViews`).

`report.go` НЕ трогаем — `NotRecordable` view не записывается, значит и описания для отчёта не
нужно (тот же прецедент: bgwriter/replslots/stat_io не имеют записи в `doDescribe`).

## What to do

1. **Добавить запись `"statements_jit"`** в карту, возвращаемую `view.New()`
   (internal/view/view.go). Моделировать по записи `statements_io` (view.go:218-229) с добавлением
   полей `MinRequiredVersion` + `NotRecordable` по образцу `stat_io` (view.go:166-179):
   - `Name: "statements_jit"`
   - `MinRequiredVersion: query.PostgresV15`
   - `NotRecordable: true`
   - `OrderKey: 2`, `OrderDesc: true` (сортировка по первому `*_total`, `gen_total`)
   - `Msg`: подсказка про пустой экран, например
     `"Show statements JIT compilation statistics (no rows when jit=off)"`
   - `ColsWidth: map[int]int{}`, `Filters: map[int]*regexp.Regexp{}`
   - Статические `QueryTmpl`/`Ncols`/`DiffIntvl`/`UniqueKey` — это плейсхолдеры, которые патчит
     `Configure()`. Задать им PG15-базовые значения как разумный дефолт: `QueryTmpl:
     query.PgStatStatementsJITPG15`, `Ncols: 13`, `DiffIntvl: [2]int{6, 10}`, `UniqueKey: 11`
     (значения для PG15/16 из tech-spec Decision 2; селектор/консты приходят из задачи 01).

2. **Добавить `case "statements_jit":`** в `view.Views.Configure()` (internal/view/view.go,
   рядом с другими pgss/stat_io кейсами, view.go:373-390). Вызвать
   `query.SelectStatStatementsJITQuery(opts.Version)` и присвоить все четыре возвращаемых значения
   в `view.QueryTmpl`, `view.Ncols`, `view.DiffIntvl`, `view.UniqueKey`, затем `v[k] = view`.
   Моделировать по ветке `stat_io` (view.go:385-387), но JIT возвращает 4 значения (с
   `UniqueKey`), а не 3 — это форма селектора из задачи 01.

3. **Поправить `internal/view/view_test.go::TestNew`** (view_test.go:9-12): поднять ожидаемое
   число view 26 → 27 и обновить комментарий. Опционально добавить guard-тест
   `TestNew_StatementsJITView` по образцу `TestNew_StatIOView` (view_test.go:17-30): проверить
   наличие view `statements_jit`, `NotRecordable == true`, `MinRequiredVersion == query.PostgresV15`,
   PG15-дефолты `Ncols`/`DiffIntvl`/`OrderKey`/`OrderDesc`/`UniqueKey` и непустой `Msg`.

4. **Поправить `record/record_test.go::Test_filterViews`** (record_test.go:101-136): поднять
   `wantN` на +1 на ВСЕХ 6 строках (`wantV` без изменений), и дополнить пояснительный комментарий.
   Причина: новый view `NotRecordable: true`, а в `filterViews` (record.go:208) ветка
   `NotRecordable` срабатывает РАНЬШЕ версионного гейта (record.go:214) — поэтому view
   отбрасывается на каждой версии независимо от `MinRequiredVersion: PostgresV15`. Конкретные
   строки: см. таблицу в разделе Details.

5. НЕ менять `report/report.go` (нет описания для NotRecordable view).

## TDD Anchor

Тесты, которые нужно написать ДО реализации (точнее — обновить существующие count-тесты): они
начинают падать как только новый view добавлен/сконфигурирован, и проходят после корректных
правок. Пишем правки тестов → запускаем → убеждаемся что падают на старом коде → добавляем
view + Configure → убеждаемся что проходят.

- `internal/view/view_test.go::TestNew` — `len(New())` должно быть 27 после добавления
  `statements_jit` в карту view.
- `internal/view/view_test.go::TestNew_StatementsJITView` (опционально, новый) — `statements_jit`
  зарегистрирован, `NotRecordable=true`, `MinRequiredVersion=query.PostgresV15`, PG15-дефолты
  `Ncols=13` / `DiffIntvl={6,10}` / `UniqueKey=11`, `OrderKey=2`, `OrderDesc=true`, `Msg` непустой.
- `record/record_test.go::Test_filterViews` — на каждой из 6 строк `wantN` увеличено на 1
  (NotRecordable отбрасывает view до версионного гейта), `wantV` неизменно.

## Acceptance Criteria

- [ ] В `view.New()` добавлена запись `statements_jit`: `MinRequiredVersion: query.PostgresV15`,
      `NotRecordable: true`, `OrderKey: 2`, `OrderDesc: true`, `Msg` с подсказкой про `jit=off`,
      `ColsWidth`/`Filters` — пустые карты, PG15-дефолтные `QueryTmpl`/`Ncols: 13`/`DiffIntvl: {6,10}`/`UniqueKey: 11`.
- [ ] В `Configure()` добавлен `case "statements_jit":`, патчащий `QueryTmpl`/`Ncols`/`DiffIntvl`/`UniqueKey`
      из `query.SelectStatStatementsJITQuery(opts.Version)`.
- [ ] `view_test.go::TestNew` ожидает 27 (комментарий обновлён).
- [ ] `record/record_test.go::Test_filterViews` имеет `wantN +1` на всех 6 строках, `wantV` без изменений.
- [ ] `report/report.go` НЕ изменён (нет записи описания для NotRecordable view).
- [ ] `go test ./internal/view/... ./record/...` зелёный; нет регрессий в существующих view/record тестах.

## Context Files

**Feature artifacts:**
- [007-feat-pg-stat-statements-jit.md](007-feat-pg-stat-statements-jit.md) — user-spec
- [007-feat-pg-stat-statements-jit-tech-spec.md](007-feat-pg-stat-statements-jit-tech-spec.md) — tech-spec (Solution, Decision 1/4/5, How it works)
- [007-feat-pg-stat-statements-jit-code-research.md](007-feat-pg-stat-statements-jit-code-research.md) — code-research (§3 view registration, §4 count-tests, §6 NotRecordable)
- [007-feat-pg-stat-statements-jit-decisions.md](007-feat-pg-stat-statements-jit-decisions.md) — decisions log
- [007-feat-pg-stat-statements-jit-task-01.md](007-feat-pg-stat-statements-jit-task-01.md) — задача-зависимость (JIT консты + `SelectStatStatementsJITQuery`)

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — фичи, поддерживаемая статистика
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — раскладка пакетов, поток данных, обработка версий PG
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — паттерны кода, конвенции тестов, версионное ветвление

**Code files:**
- [internal/view/view.go](internal/view/view.go) — добавить запись `statements_jit` + `case` в `Configure()`
- [internal/view/view_test.go](internal/view/view_test.go) — поднять `TestNew` 26→27 (+ опц. guard-тест)
- [record/record_test.go](record/record_test.go) — `Test_filterViews` `wantN +1` на всех строках
- [internal/query/io.go](internal/query/io.go) — образец формы селектора `SelectStatIOQuery` (читать)
- [record/record.go](record/record.go) — `filterViews`: порядок `NotRecordable` до версионного гейта (читать)

## Verification Steps

- Запустить `go test ./internal/view/... ./record/...` — должно быть зелёным.
- Проверить, что `TestNew` ожидает 27, `Test_filterViews` проходит со всеми 6 поднятыми `wantN`.
- Убедиться, что `report/report.go` не изменён (git diff пуст по этому файлу).
- (опц.) `make build` — компиляция чистая; новый `case` в `Configure()` использует селектор/консты из задачи 01.

## Details

<!-- All details for task execution — technical, organizational, any other. -->

**Files:**
- `internal/view/view.go` — (1) новая запись в карте `New()` (текущая карта — 26 записей,
  view.go:38-350; pgss-блок 194-266). Образец структуры: `statements_io` (view.go:218-229) для
  pgss-формы; `stat_io` (view.go:166-179) для `MinRequiredVersion`+`NotRecordable`+`UniqueKey`
  вместе. (2) Новый `case "statements_jit":` в `Configure()` (switch view.go:363-391); образец —
  ветка `stat_io` (view.go:385-387), но с 4 присваиваниями (включая `UniqueKey`).
- `internal/view/view_test.go` — `TestNew` на view_test.go:9-12 (`assert.Equal(t, 26, len(v))` →
  27, комментарий). Образец guard-теста — `TestNew_StatIOView` (view_test.go:17-30).
- `record/record_test.go` — таблица `Test_filterViews` на record_test.go:123-128. Поднять `wantN`
  по таблице ниже, `wantV` оставить как есть, дополнить комментарий-пояснение (record_test.go:108-122).

**Таблица правок `Test_filterViews` (wantN old → new, wantV без изменений):**

| version | pgssSchema | wantN old → new | wantV |
|---------|-----------|-----------------|-------|
| 140000  | ""        | 10 → 11         | 16    |
| 140000  | public    | 4 → 5           | 22    |
| 130000  | public    | 7 → 8           | 19    |
| 120000  | public    | 10 → 11         | 16    |
| 110000  | public    | 12 → 13         | 14    |
| 100000  | public    | 12 → 13         | 14    |

**Dependencies:**
- Задача 01 (Wave 1): должна предоставить консты `query.PgStatStatementsJITPG15` /
  `query.PgStatStatementsJITDefault` и селектор
  `query.SelectStatStatementsJITQuery(version) (string, int, [2]int, int)` (возвращает
  query + `Ncols` + `DiffIntvl` + `UniqueKey`). Этот таск компилируется и проходит тесты только
  после слияния задачи 01.
- Никаких новых пакетов.

**Edge cases:**
- `Test_filterViews` останавливается на версии 140000 (нет строки 150000), но `+1` применяется
  единообразно на всех строках, потому что `NotRecordable`-сброс не зависит от версии (срабатывает
  до версионного гейта). PG<15 строки тоже +1.
- `Test_filterViews` НЕ имеет отдельной name-map, которую надо пополнять. Другие тесты в
  `record_test.go` (`TestFilterViews_NotRecordable`, `TestFilterViews_dropsExplicitNotRecordable`,
  `TestFilterViews_Recordable`) строят собственные ad-hoc `view.Views{}` литералы и НЕ затронуты.
- `Test_app_record` (record_test.go:32) использует `countRecordable(view.New())` — `NotRecordable`
  view автоматически исключается, дополнительных правок не требует.
- `TestView_VersionOK` (view_test.go:205-229) считает view, проходящие `VersionOK` для версии.
  Новый view с `MinRequiredVersion: PostgresV15` пройдёт `VersionOK` только на 150000+; в таблице
  теста есть строка 160000 (`total: 26`), где `statements_jit.VersionOK(160000) == true` —
  значит `total` на этой строке нужно поднять 26 → 27. ВНИМАНИЕ: это третий count-тест, который
  ломается; обязательно прогнать весь `./internal/view/...` и поправить строку 160000. Остальные
  строки (140000 и ниже) не меняются — там `VersionOK(<150000) == false`.

**Implementation hints:**
- Форма селектора JIT отличается от `stat_io`: `stat_io` возвращает 3 значения
  `(string, int, [2]int)` (io.go:87), а JIT — 4 (добавлен `UniqueKey`, т.к. md5-ключ у JIT стоит
  в конце и сдвигается с `Ncols`, тогда как у `stat_io` он фиксирован на col 0). `Configure()`
  должна присвоить все 4.
- Ветка `statements_timings` в `Configure()` (view.go:373-375) патчит только `QueryTmpl` — это НЕ
  образец для JIT, потому что у timings `Ncols`/`DiffIntvl`/`UniqueKey` константны. Образец —
  ветка `stat_io` (view.go:385-387), но расширенная до `UniqueKey`.
- `Msg` служит двойную роль (Decision 4): это и подпись экрана в cmdline, и подсказка про пустой
  экран при `jit=off` — текст должен явно упоминать пустое состояние.
- `report/report.go::doDescribe` НЕ трогать: bgwriter/replslots/stat_io (все `NotRecordable`) не
  имеют там записей — это прямой прецедент (code-research §6).

## Reviewers

- **dev-code-reviewer** → `007-feat-pg-stat-statements-jit-task-02-dev-code-reviewer-review.json`
- **dev-security-auditor** → `007-feat-pg-stat-statements-jit-task-02-dev-security-auditor-review.json`
- **dev-test-reviewer** → `007-feat-pg-stat-statements-jit-task-02-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [007-feat-pg-stat-statements-jit-decisions.md](007-feat-pg-stat-statements-jit-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
