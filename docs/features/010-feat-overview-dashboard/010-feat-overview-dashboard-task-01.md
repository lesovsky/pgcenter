---
status: planned                    # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # go test ./internal/pretty/...
reviewers: [dev-code-reviewer, dev-test-reviewer]
teammate_name:
---

# Task 01: Net-new formatting helpers

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Verbose-режим (фича 010) добавляет к двум верхним панелям `top` (`sysstat` слева, `pgstat` справа)
дополнительные строки `label:value` с целочисленными агрегатами здоровья инстанса. Эти строки требуют
форматирования, которого в кодовой базе **нет**: округление вверх до целого, фиксированная ширина колонок
с резервом разрядов (раскладка и суффиксы статичны, меняются только цифры — как в существующих
`sysstat`/`pgstat`), и динамическое переключение единицы скорости при переполнении резерва
(`MB/s→GB/s`, `Mbps→Gbps`), чтобы топовое железо (NVMe-массивы >9.7 GB/s, 25/40/100GbE) не ломало раскладку.

Эта задача — **чистые форматтеры**, без какой-либо привязки к gocui, статистике или SQL. Они feed каждую
verbose-строку, которую соберёт Task 8 (композиция строк панелей). Реализация изолирована и параллельна
остальной Wave 1.

Граница reuse/net-new (из code-research §3a-new): `internal/pretty.Size` — **единственный** переиспользуемый
кусок (его будут звать для `filesyst` size/used в Task 8, его трогать не надо). Всё остальное net-new:
`internal/math` содержит только `Min`/`Max(int)` — `ceil` отсутствует по всему репозиторию; `pretty.Size`
переключает байтовую единицу, но не имеет суффикса скорости, не имеет фиксированной ширины и округляет
до одного знака после запятой, а не вверх до целого.

## What to do

В пакете `internal/pretty` (файл `internal/pretty/pretty.go`) добавить три чистые функции-форматтера:

1. **Целочисленное округление вверх (ceil).** Хелпер, принимающий `float64` и возвращающий целое,
   округлённое вверх (потолок). Полные панели показывают дробные значения; verbose показывает целые с
   округлением вверх (user-spec «Форматирование»). В `internal/math` нет ни одной обёртки над `math.Ceil`
   по всему репозиторию — это net-new.

2. **Фиксированная ширина с резервом разрядов.** Хелпер, форматирующий целое в строку фиксированной ширины
   (резерв N разрядов), где раскладка статична и меняются только цифры — паттерн `%Nd`, уже применяемый
   в `printSysstat`/`printPgstat` (`%6d`, `%3d` и т.п.), но вынесенный в переиспользуемую функцию. Резервы
   из user-spec «Состав и источники строк»: `devices` — 2 разряда, `max util` — 3, `r/s`/`w/s` — 5,
   скорости — 4 (см. динамический суффикс ниже), `err`/`coll` — по 4.

3. **Динамический суффикс единицы скорости.** Хелпер, форматирующий значение скорости в фиксированный
   резерв разрядов и переключающий единицу при переполнении этого резерва: `MB/s→GB/s` для дисковых
   потоков, `Mbps→Gbps` для сетевых. Семейство единиц (байтовая vs сетевая) — параметр; делитель на
   переход — 1024 для `MB/s→GB/s` (бинарный, консистентно с `pretty.Size`), для `Mbps→Gbps` — выбрать
   делитель по источнику (как nicstat считает `Mbps` в `netdev.go`) и задокументировать в decisions.
   На переполнении: значение делится, единица повышается, число снова укладывается в резерв.

Покрыть всё таблично-property-тестами в `internal/pretty/pretty_test.go`, с особым упором на **границы
переполнения** (значение ровно на пороге, чуть ниже, чуть выше), где переключается суффикс или меняется
число разрядов. Существующий `TestSize` не трогать.

Сигнатуры функций определи сам по месту использования — но они должны быть pure (вход → строка/целое, без
side effects), чтобы оставаться полностью unit-тестируемыми без живого терминала.

## TDD Anchor

Пишем тесты ДО реализации, убеждаемся что падают, затем пишем код.

- `internal/pretty/pretty_test.go::TestCeil` — таблица: `0→0`, `1.0→1`, `1.1→2`, `9.6→10`, целые на входе
  округляются корректно (потолок), не паникует на нуле/малых значениях.
- `internal/pretty/pretty_test.go::TestReserveWidth` — фиксированная ширина: значение умещается в резерв
  (паддинг), значение на границе резерва (ровно N разрядов), значение шире резерва (поведение
  задокументировано — не молча ломать раскладку).
- `internal/pretty/pretty_test.go::TestRateUnit` — динамический суффикс: значение ниже порога → `MB/s`;
  ровно на пороге переполнения резерва → переключение на `GB/s`; значение выше → `GB/s` с укладкой в резерв;
  аналогично `Mbps→Gbps`. Граничные значения вокруг порога (порог−1, порог, порог+1).
- `internal/pretty/pretty_test.go::TestRateUnit_property` (property/walk) — для диапазона значений
  инвариант «отформатированная числовая часть всегда укладывается в зарезервированные разряды» держится
  на всём диапазоне (прецедент property-теста — `visibleColumns` в [009], см. patterns.md).

## Acceptance Criteria

- [ ] Добавлен ceil-форматтер: округляет `float64` вверх до целого.
- [ ] Добавлен форматтер фиксированной ширины с резервом разрядов (статичная раскладка, меняются цифры).
- [ ] Добавлен динамический суффикс скорости: `MB/s→GB/s` и `Mbps→Gbps` при переполнении резерва.
- [ ] Все функции чистые (pure), без зависимостей от gocui/stat/SQL.
- [ ] Таблично-property тесты покрывают границы переполнения (порог−1 / порог / порог+1).
- [ ] Существующий `TestSize` и `pretty.Size` не изменены.
- [ ] `go test ./internal/pretty/...` зелёный; `make lint` чистый.

## Context Files

**Feature artifacts:**
- [010-feat-overview-dashboard.md](010-feat-overview-dashboard.md) — user-spec («Состав и источники строк», резервы разрядов, динамический суффикс)
- [010-feat-overview-dashboard-tech-spec.md](010-feat-overview-dashboard-tech-spec.md) — tech-spec (Decision 8, Task 1)
- [010-feat-overview-dashboard-code-research.md](010-feat-overview-dashboard-code-research.md) — §3a-new (граница reuse/net-new)
- [010-feat-overview-dashboard-decisions.md](010-feat-overview-dashboard-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — что за проект, аудитория
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — раскладка пакетов, поток данных
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — Testable TUI Rendering (pure function + property-test прецедент [009]); Naming Conventions

**Code files:**
- [internal/pretty/pretty.go](internal/pretty/pretty.go) — добавить три форматтера (НЕ менять `Size`)
- [internal/pretty/pretty_test.go](internal/pretty/pretty_test.go) — добавить таблично-property тесты (стиль `TestSize`)
- [internal/math/math.go](internal/math/math.go) — прочитать: подтверждает, что ceil отсутствует (только `Min`/`Max(int)`)
- [top/stat.go](top/stat.go) — прочитать `printSysstat`/`printPgstat` (строки 214–285): паттерн `%Nd` резерва разрядов, который форматтеры обобщают

## Verification Steps

- Запустить `go test ./internal/pretty/...` — все тесты (включая существующий `TestSize`) проходят.
- Убедиться, что новые тесты сначала падали на пустой реализации (TDD-порядок), затем прошли.
- `make lint` (golangci-lint + gosec) чистый по `internal/pretty`.

## Details

**Files:**
- `internal/pretty/pretty.go` — добавить три pure-функции (ceil, reserve-width, rate-unit-suffix). НЕ
  изменять `Size` (его переиспользует Task 8 для `filesyst`).
- `internal/pretty/pretty_test.go` — добавить таблично-property тесты; не трогать `TestSize`.

**Dependencies:**
- Стандартная библиотека: `math.Ceil` (stdlib, новых пакетов не нужно — tech-spec «Dependencies → None»).
- Зависимостей от других задач нет (Wave 1, `depends_on: []`). Потребители: Task 8 (композиция verbose-строк).

**Edge cases:**
- Значение ровно на пороге переполнения резерва — должно переключить суффикс (граничный тест).
- Значение шире резерва даже после переключения единицы (теоретический максимум) — поведение
  детерминировано и задокументировано, не молча ломает раскладку.
- Ноль и очень малые значения — ceil(0)=0, ceil(0.1)=1.
- Отрицательные значения скорости в норме не приходят, но ceil/width не должны паниковать.

**Implementation hints:**
- Паттерн резерва разрядов уже физически есть в `printSysstat`/`printPgstat` (`%6d`, `%3d`,
  `%4.1f`) — форматтеры лишь выносят его в переиспользуемые функции; смотри строки 214–285 `top/stat.go`
  как референс раскладки.
- `pretty.Size` (`pretty.go:8`) переключает байтовую единицу по магнитуде через `switch` — структурно
  это близкий прецедент для динамического суффикса скорости, но он `%.1f` и без резерва; не копировать
  целиком, взять только идею «switch по порогу → сменить единицу/делитель».
- Делитель `MB/s→GB/s` = 1024 (бинарный, консистентно с `pretty.Size`). Для `Mbps→Gbps` зафиксируй
  делитель явно и обоснуй в decisions (nicstat оперирует Mbps; сверься с тем, как считается `Mbps` в
  `netdev.go`, чтобы verbose был консистентен с панелью nicstat).
- Резервы разрядов (из user-spec): devices=2, max util=3, скорости (rMB/s, wMB/s, rMbps, wMbps)=4,
  r/s, w/s=5, err и coll по 4.
- Тесты — стиль существующего `TestSize`: срез `testcases` со структурой `{in, want}`, цикл с
  `assert.Equal`. Property-тест — прецедент `visibleColumns` walk-теста ([009], patterns.md §«Testable TUI
  Rendering»).

## Reviewers

- **dev-code-reviewer** → `010-feat-overview-dashboard-task-01-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `010-feat-overview-dashboard-task-01-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [010-feat-overview-dashboard-decisions.md](010-feat-overview-dashboard-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON; зафиксировать выбранный делитель `Mbps→Gbps`)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
