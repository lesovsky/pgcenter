---
status: planned                    # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 03: io.Writer refactor of printSysstat/printPgstat

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Это чисто включающий (enabling) рефакторинг под верхние панели верхнего экрана. Сейчас две верхние
панели рисуются функциями `printSysstat(v *gocui.View, s stat.Stat)` (`top/stat.go:214`) и
`printPgstat(v *gocui.View, s stat.Stat, props, db)` (`top/stat.go:253`) — обе принимают конкретный
`*gocui.View` и пишут в него через `fmt.Fprintf`. Из-за привязки к `*gocui.View` их вывод нельзя
проверить в unit-тесте без живого терминала.

Задача — расщепить каждую функцию по тому же образцу, что уже применён для `printDbstat` →
`renderDbstat(w io.Writer, …)` (`top/stat.go:324`/`:349`): оставить `printSysstat`/`printPgstat` тонкими
обёртками над `*gocui.View`, которые просто делегируют в новые `renderSysstat(w io.Writer, s stat.Stat)`
и `renderPgstat(w io.Writer, s stat.Stat, props stat.PostgresProperties, db *postgres.DB)`. Вся
`fmt.Fprintf`-логика переезжает в `render*`-функции, которые принимают `io.Writer` и поэтому
тестируются против `bytes.Buffer`.

Рефакторинг поведенчески-сохраняющий: compact-вывод (текущие 4 строки в каждой панели) должен остаться
**байт-в-байт идентичным**. Никаких verbose-строк здесь ещё нет — это фундамент, на который опираются
последующие задачи (verbose-row composers в Task 8 дописывают строки внутрь `renderSysstat`/`renderPgstat`).

Эта задача file-disjoint в рамках Wave 1: трогает только `top/stat.go` и `top/stat_test.go`.

## What to do

1. В `top/stat.go` ввести `renderSysstat(w io.Writer, s stat.Stat) error`, перенеся в неё все четыре
   `fmt.Fprintf`-строки из текущего `printSysstat` без изменения форматных строк и аргументов.
2. Превратить `printSysstat(v *gocui.View, s stat.Stat) error` в тонкую обёртку, которая вызывает
   `renderSysstat(v, s)` (как `printDbstat` вызывает `renderDbstat`). `*gocui.View` уже реализует
   `io.Writer`, так что вызов прямой.
3. Аналогично ввести `renderPgstat(w io.Writer, s stat.Stat, props stat.PostgresProperties, db *postgres.DB) error`,
   перенеся туда все четыре `fmt.Fprintf`/`fmt.Fprintln`-строки из текущего `printPgstat`, и сделать
   `printPgstat` тонкой обёрткой над `renderPgstat`.
4. Сохранить точки вызова в `printStat` (`top/stat.go:133` и `:143`) без изменений — они продолжают
   звать `printSysstat`/`printPgstat`. Сигнатуры обёрток не меняются.
5. В `top/stat_test.go` добавить writer-based тесты, утверждающие текущие 4 compact-строки каждой панели
   против `bytes.Buffer` (golden-проверка байт-в-байт). Использовать уже импортированный в тест-файле
   `bytes` и существующие хелперы для `postgres.Config`/`stat.Stat`.

## TDD Anchor

Тесты пишем ДО рефакторинга: фиксируют текущий байт-в-байт вывод как golden, затем рефакторинг должен
оставить их зелёными.

- `top/stat_test.go::Test_renderSysstat_compact` — `renderSysstat(&buf, s)` для известного `stat.Stat`
  (заданные `LoadAvg`, `CPUStat`, `Meminfo`) выдаёт ровно 4 строки с теми же форматами/ANSI-кодами, что
  и текущий `printSysstat` (golden-строка с `\033[37;1m…` и `\n`-разделителями).
- `top/stat_test.go::Test_renderPgstat_compact` — `renderPgstat(&buf, s, props, db)` для известных
  `Activity`/`PostgresProperties`/`postgres.Config` выдаёт ровно 4 строки: line1 = вывод
  `formatInfoString`, далее activity/autovacuum/statements с теми же форматами и ANSI-кодами.
- `top/stat_test.go::Test_renderPgstat_infoLine` (опционально) — line1 `renderPgstat` совпадает с
  `formatInfoString(db.Config, s.Activity.State, props.Version, s.Activity.Uptime, props.Recovery)` для
  тех же входов (связка с уже существующим `Test_formatInfoString`).

## Acceptance Criteria

- [ ] Введены `renderSysstat(w io.Writer, s stat.Stat) error` и `renderPgstat(w io.Writer, s stat.Stat, props stat.PostgresProperties, db *postgres.DB) error`.
- [ ] `printSysstat`/`printPgstat` стали тонкими обёртками над соответствующими `render*`-функциями; их сигнатуры не изменились.
- [ ] Compact-вывод обеих панелей байт-в-байт идентичен прежнему (golden-тесты на `bytes.Buffer` зелёные).
- [ ] Точки вызова в `printStat` не тронуты, поведение `printStat` не изменилось.
- [ ] Verbose-строки НЕ добавлены (это не входит в эту задачу).
- [ ] `go test ./top/...` зелёный; `make lint` без новых замечаний.

## Context Files

**Feature artifacts:**
- [010-feat-overview-dashboard.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard.md) — user-spec
- [010-feat-overview-dashboard-tech-spec.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-tech-spec.md) — tech-spec (Task 3, Decision 1)
- [010-feat-overview-dashboard-code-research.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-code-research.md) — code-research (section "3-new": printSysstat/printPgstat/renderDbstat precedent)
- [010-feat-overview-dashboard-decisions.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-decisions.md) — decisions log (создаётся при выполнении)

**Project knowledge:**
- [project.md](.claude/skills/project-knowledge/overview.md) — обзор проекта (файл называется `overview.md`)
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — раскладка пакетов, поток данных
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — паттерны кода и конвенции тестирования (testify)

**Code files:**
- [top/stat.go](top/stat.go) — `printSysstat` (`:214`), `printPgstat` (`:253`), образец `renderDbstat` (`:349`), точки вызова в `printStat` (`:133`, `:143`)
- [top/stat_test.go](top/stat_test.go) — добавить writer-based тесты (уже импортирует `bytes`, есть `Test_formatInfoString`)

## Verification Steps

- Шаг 1: написать golden-тесты `Test_renderSysstat_compact` / `Test_renderPgstat_compact` ДО рефакторинга, убедиться что они зелёные на текущем коде (через временный прямой вызов или путём фиксации ожидаемого golden из реального вывода).
- Шаг 2: выполнить рефакторинг (extract `render*`, обёртки).
- Шаг 3: `go test ./top/...` — все тесты зелёные, включая существующие `Test_formatInfoString`, `Test_alignViewToResult`, `Test_visibleColumns*` и др.
- Шаг 4: `make lint` — без новых замечаний.
- Ожидаемый результат: compact-вывод обеих панелей не изменился ни на байт; новые `render*`-функции покрыты тестами.

## Details

**Files:**
- `top/stat.go` — извлечь два `render*(w io.Writer, …)`; `printSysstat`/`printPgstat` сделать обёртками. Образец уже в файле: `printDbstat` (`:324`) резолвит ширину терминала и делегирует в `renderDbstat(w io.Writer, …)` (`:349`).
- `top/stat_test.go` — добавить writer-based golden-тесты против `bytes.Buffer`.

**Dependencies:**
- Задач-зависимостей нет (Wave 1, `depends_on: []`).
- Параллельно в Wave 1 идёт Task 2 (тоже трогает `top/stat.go` в списке "files to read", но не модифицирует) — конфликта по файлам нет: Task 2 модифицирует `top/verbose.go`/`top/keybindings.go`/`top/help.go`/`top/config.go`/`internal/view/view.go`, не `top/stat.go`.
- `io` уже импортирован в `top/stat.go` (строка 7). `bytes` уже импортирован в `top/stat_test.go` (строка 4). `postgres` уже импортирован в обоих.

**Edge cases:**
- `*gocui.View` реализует `io.Writer` — обёртки передают `v` напрямую, без адаптера.
- `s.Activity` доступно через встроенный (embedded) `Pgstat` в `stat.Stat` (промоушн поля), поэтому в тестах `stat.Stat` собирается как `stat.Stat{Pgstat: stat.Pgstat{Activity: …}}` (см. как тест-файл уже конструирует `stat.Stat{Pgstat: stat.Pgstat{Result: …}}` в `makeRenderResult`).
- line1 `renderPgstat` — это `fmt.Fprintln` (с переводом строки), остальные три — `fmt.Fprintf` с явным `\n`; сохранить ровно как есть, чтобы байты совпали.
- ANSI-последовательности (`\033[37;1m…\033[0m`) — часть golden-вывода; в ожиданиях тестов их надо прописать буквально.

**Implementation hints:**
- Поведенчески-сохраняющий рефакторинг: НЕ менять форматные строки, порядок аргументов, ANSI-коды и переводы строк. Любое расхождение ловится golden-тестом.
- Имена `renderSysstat`/`renderPgstat` заданы tech-spec (Task 3) и переиспользуются в Task 8 — назвать именно так.
- Не добавлять verbose-строки, флаги, параметры verbose — это Task 8. Сигнатуры `render*` здесь без verbose-аргументов.
- Для golden-строк можно сначала запустить временный тест, печатающий реальный вывод, скопировать его в ожидание (типовой приём фиксации golden), затем зафиксировать.

## Reviewers

- **dev-code-reviewer** → `docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-task-03-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-task-03-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [010-feat-overview-dashboard-decisions.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
