---
status: done                    # planned -> in_progress -> done
depends_on: ["01"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 2                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 03: Scroll hotkeys, offset reset, and help text

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Эта задача добавляет пользовательское управление горизонтальным скроллом, который рендерит Task 2.
Task 1 уже добавил поле `config.scrollOffset` и чистую функцию вычисления видимого окна колонок с зажимом
(clamping); эта задача делает офсет управляемым с клавиатуры и гарантирует его сброс при переключении экранов.

Конкретно:
- Два хендлера `scrollLeft` (`[`) и `scrollRight` (`]`) декрементируют/инкрементируют `config.scrollOffset`
  с зажимом по нижней границе (offset не уходит в минус) и отправляют `config.view` на `config.viewCh`
  **исключительно ради немедленной перерисовки**. Сам view не меняется — в отличие от `orderKeyLeft`/`orderKeyRight`,
  которые мутируют `view.OrderKey`. Render читает `config.scrollOffset` напрямую из проброшенного `*config`
  (Decision 1, секция "How it works" п.1). Без отправки на канал изменение проявилось бы только на следующем
  тике авто-обновления (до целого интервала refresh позже).
- Клавиши `[` и `]` регистрируются на view `sysstat` в слайсе `keys` (`top/keybindings.go`), как все
  навигационные хоткеи.
- `config.scrollOffset` сбрасывается в 0 на **обоих** путях переключения экрана: `viewSwitchHandler`
  (через который идут все `switchViewTo`-ветки) И `switchViewToProcPidStat` (он намеренно обходит
  `viewSwitchHandler`, патчит view вручную — Decision 3). Сброс только в одном месте оставил бы устаревший
  офсет при входе на per-process экран (клавиша `S`).
- Клавиши `[` / `]` документируются в help-экране (`top/help.go`).

Скролл орто́гонален сортировке: `[`/`]` не трогают `OrderKey`, а сортировка не трогает `scrollOffset`.

Важно про зажим верхней границы (модель, согласованная с Task 2): максимальный валидный офсет зависит от
текущих ширин колонок и ширины терминала, которые в хендлере недоступны (нет `v.Size()` контекста), — и это
правильно, хендлер про ширину терминала знать не должен. Поэтому хендлер `scrollRight` **только инкрементирует**
`config.scrollOffset`; нижнюю границу 0 держит `scrollLeft`. ИСТИННЫЙ верхний зажим обеспечивается тем, что при
рендере вычисленный (clamped) офсет записывается обратно в `config.scrollOffset` — это делает Task 2 в
`printDbstat` через `visibleColumns`. То есть `]` на максимуме фактически остаётся на максимуме, потому что
следующий рендер перезапишет `config.scrollOffset` зажатым значением; для пользователя предела как такового в
хендлере нет, и не нужно. НЕ вводи «предохранитель `Ncols`» как верхний предел: в узком терминале реальный
`maxOffset` много меньше `Ncols`, и такой «предохранитель» не лечит UX (офсет всё равно убежал бы далеко за
видимое). Допустима лишь дешёвая защита от целочисленного переполнения, но это не пользовательский верхний
предел. Место верхнего clamp/write-back — Task 2. `scrollLeft` зажимает по 0 прямо в хендлере.

## What to do

1. В `top/config_view.go` добавь два хендлера рядом с `orderKeyLeft`/`orderKeyRight`:
   - `scrollLeft(config *config) func(*gocui.Gui, *gocui.View) error` — уменьшает `config.scrollOffset` на 1,
     зажимает по нижней границе 0 (не уходить в минус), отправляет `config.view` на `config.viewCh`.
   - `scrollRight(config *config) func(*gocui.Gui, *gocui.View) error` — увеличивает `config.scrollOffset` на 1,
     **без верхнего предела в хендлере** (фактический верхний зажим делает рендер из Task 2: записывает clamped
     офсет обратно в `config.scrollOffset`). НЕ использовать `Ncols`-предохранитель как верхний предел; допустима
     лишь дешёвая защита от int-переполнения. Отправляет `config.view` на `config.viewCh`.
   - View при этом **не мутируется** — меняется только `config.scrollOffset`. Отправка на канал нужна только
     для немедленной перерисовки.
2. В `top/keybindings.go` в слайсе `keys` зарегистрируй на view `"sysstat"`:
   `{"sysstat", '[', scrollLeft(app.config)}` и `{"sysstat", ']', scrollRight(app.config)}`.
3. В `top/config_view.go` сбрось `config.scrollOffset = 0` на обоих путях переключения:
   - в `viewSwitchHandler` (покрывает все ветки `switchViewTo`);
   - в `switchViewToProcPidStat` (он не делегирует `viewSwitchHandler` — сбрось отдельно, до/при загрузке
     целевого view).
4. В `top/help.go` добавь в `helpTemplate` документацию клавиш `[` / `]` (горизонтальный скролл колонок),
   рядом со строкой про `Left,Right,<,/` навигацию по колонкам.
5. В `top/config_view_test.go` напиши тесты (см. TDD Anchor): зажим нижней границы, орто́гональность,
   сброс офсета на обоих путях переключения.

## TDD Anchor

Тесты, которые нужно написать ДО реализации. Пишем → запускаем → убеждаемся что падают → пишем код → убеждаемся что проходят.

Используй существующий паттерн хендлер-тестов из `top/config_view_test.go`: горутина читает один `view.View`
с `config.viewCh`, затем запускается хендлер, после чего ассертится результат (см. `Test_orderKeyLeft`).
Офсет проверяется на `config.scrollOffset` (он на `config`, не на отправленном view).

- `top/config_view_test.go::Test_scrollLeft` — `[` на offset 0 оставляет 0 (нижний зажим); `[` на offset N>0
  даёт N-1; хендлер отправляет view на `viewCh` (немедленная перерисовка) и не меняет `OrderKey`.
- `top/config_view_test.go::Test_scrollRight` — `]` инкрементирует `config.scrollOffset` на 1; хендлер
  отправляет view на `viewCh` и не меняет `OrderKey`. (Верхний зажим `]` на максимуме обеспечивает рендер
  из Task 2 — write-back clamped офсета в `printDbstat`; проверяется там/визуально, не в этом хендлер-тесте.)
- `top/config_view_test.go::Test_scrollOrthogonalToSort` — скролл (`[`/`]`) не меняет `OrderKey`;
  сортировка (`orderKeyLeft`/`orderKeyRight`) не меняет `config.scrollOffset`.
- `top/config_view_test.go::Test_viewSwitchResetsScrollOffset` — при ненулевом `config.scrollOffset`
  вызов `viewSwitchHandler` сбрасывает его в 0.
- `top/config_view_test.go::Test_switchViewToProcPidStatResetsScrollOffset` — при ненулевом
  `config.scrollOffset` путь `switchViewToProcPidStat` сбрасывает его в 0 (этот путь обходит
  `viewSwitchHandler`).

## Acceptance Criteria

- [ ] `scrollLeft` (`[`) уменьшает `config.scrollOffset`, зажат по 0 (на 0 остаётся 0).
- [ ] `scrollRight` (`]`) увеличивает `config.scrollOffset` без верхнего предела в хендлере; верхний зажим обеспечен write-back clamped офсета при рендере (Task 2). Нет `Ncols`-предохранителя как пользовательского предела.
- [ ] Оба хендлера отправляют `config.view` на `config.viewCh` ради немедленной перерисовки и не мутируют view.
- [ ] `[` и `]` зарегистрированы на view `sysstat` в слайсе `keys` (`top/keybindings.go`).
- [ ] `config.scrollOffset` сбрасывается в 0 и в `viewSwitchHandler`, и в `switchViewToProcPidStat`.
- [ ] Скролл орто́гонален сортировке: `[`/`]` не трогают `OrderKey`, сортировка не трогает `scrollOffset`.
- [ ] Help-экран (`helpTemplate`) документирует `[` / `]`.
- [ ] Все существующие тесты пакета `top` проходят без регрессий (`Test_switchViewTo`, `Test_orderKeyLeft/Right`).
- [ ] `go test ./top/...` зелёный; `make build` собирается.

## Context Files

**Feature artifacts:**
- [009-feat-horizontal-scroll.md](009-feat-horizontal-scroll.md) — user-spec
- [009-feat-horizontal-scroll-tech-spec.md](009-feat-horizontal-scroll-tech-spec.md) — tech-spec (Task 3, Decision 1/3, "How it works")
- [009-feat-horizontal-scroll-code-research.md](009-feat-horizontal-scroll-code-research.md) — code research (§1, §2, §6)
- [009-feat-horizontal-scroll-decisions.md](009-feat-horizontal-scroll-decisions.md) — decisions log
- [009-feat-horizontal-scroll-task-01.md](009-feat-horizontal-scroll-task-01.md) — зависимость: поле `scrollOffset` и функция окна с зажимом

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md)
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md)
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — testing conventions, handler patterns

**Code files:**
- [top/config_view.go](../../../top/config_view.go) — добавить `scrollLeft`/`scrollRight`; сбросить офсет в `viewSwitchHandler` и `switchViewToProcPidStat`
- [top/keybindings.go](../../../top/keybindings.go) — зарегистрировать `[` / `]` на `sysstat`
- [top/help.go](../../../top/help.go) — документировать `[` / `]` в `helpTemplate`
- [top/config_view_test.go](../../../top/config_view_test.go) — тесты хендлеров и сброса
- [top/config.go](../../../top/config.go) — read: поле `scrollOffset` (добавлено в Task 1)
- [top/stat.go](../../../top/stat.go) — read: функция окна колонок с зажимом (Task 1), write-back офсета при рендере (Task 2)

## Verification Steps

- `go test ./top/...` — новые тесты хендлеров/сброса проходят, существующие тесты `top` без регрессий.
- `make build` — бинарь собирается без ошибок.
- Проверить вручную (опционально, основное — у пользователя в Task 2/Task 4): `[`/`]` меняют офсет,
  переключение экранов сбрасывает скролл.

## Details

**Files:**
- `top/config_view.go` — текущее состояние: хендлеры `orderKeyLeft` (`:21`), `orderKeyRight` (`:34`) —
  паттерн для копирования (мутируют `config.view`, потом `config.viewCh <- config.view`). Скролл-хендлеры
  мутируют **`config.scrollOffset`** (не view) и отправляют view только для перерисовки. `viewSwitchHandler`
  (`:208`) — добавить `config.scrollOffset = 0`. `switchViewToProcPidStat` (`:220`) — добавить
  `app.config.scrollOffset = 0` (он обходит `viewSwitchHandler`, патчит view вручную на `:240-249`).
- `top/keybindings.go` — слайс `keys` начинается на `:18`; навигация на `sysstat` (`orderKeyLeft` `:22`,
  `orderKeyRight` `:23`). Добавить две строки `{"sysstat", '[', ...}` / `{"sysstat", ']', ...}`. Клавиши
  `[`/`]` свободны (подтверждено в code research §1). Регистрация — plain rune, `gocui.ModNone` (общий цикл `:81`).
- `top/help.go` — `helpTemplate` const (`:10`); строка про навигацию колонок `Left,Right,<,/` (`:21`),
  `Up,Down` ширина (`:22`). Добавить упоминание `[`/`]` (горизонтальный скролл колонок). Соблюди ASCII-выравнивание
  столбцов шаблона.
- `top/config_view_test.go` — паттерн хендлер-теста: `Test_orderKeyLeft` (`:11`) — горутина читает с `viewCh`,
  ассерт. Для офсета ассертить `config.scrollOffset` (на config, не на отправленном view). `newConfig()`
  даёт `config` с инициализированным `viewCh` и `views`; `config.view = config.views["activity"]`.

**Dependencies:**
- Task 01 — поле `config.scrollOffset` и чистая функция окна с верхним зажимом. Сверься с её итоговой
  сигнатурой/именованием перед реализацией (имя поля, где живёт max-clamp). `internal/math` (`Max`)
  уже импортирован в `config_view.go` для зажима по нижней границе, если понадобится.

**Edge cases:**
- `[` на offset 0 → остаётся 0 (нижний зажим в хендлере).
- `]` на максимуме → хендлер по-прежнему инкрементирует офсет, но следующий рендер (Task 2) запишет clamped
  значение обратно в `config.scrollOffset`, поэтому реально офсет не убегает за видимый максимум. Единственный
  источник истины по верхней границе — write-back в `printDbstat` через `visibleColumns` (Task 2). НЕ ставить
  верхний предел `Ncols` в хендлере: в узком терминале реальный `maxOffset` много меньше `Ncols`.
- Переключение на per-process экран (`S` → `switchViewToProcPidStat`) — офсет должен сброситься (отдельный путь).
- Авто-обновление (тик refresh) НЕ трогает офсет — скролл сохраняется в пределах экрана (это уже так,
  ничего делать не нужно; не сломать).

**Implementation hints:**
- Скролл-хендлер отправляет view на канал ТОЛЬКО для немедленной перерисовки; view не мутируется
  (отличие от `orderKey*`). Render читает `config.scrollOffset` из проброшенного `*config`.
- termbox v1.1.1 не различает Shift/Ctrl — поэтому plain-rune `[`/`]`, а не модификаторы (code research §9).
- НЕ добавляй верхний зажим в `scrollRight` через `v.Size()`/ширины — этой информации в хендлере нет;
  верхний зажим централизован в write-back при рендере (Task 2: `printDbstat` пишет clamped офсет обратно
  в `config.scrollOffset` через `visibleColumns`).
- НЕ используй `Ncols` (или иное «дешёвое» число) как верхний предел в `scrollRight` — это не лечит UX
  (в узком терминале реальный `maxOffset` много меньше `Ncols`, офсет всё равно убежит). Допустима лишь
  дешёвая защита от целочисленного переполнения, но не как пользовательский верхний предел.

## Reviewers

- **dev-code-reviewer** → `009-feat-horizontal-scroll-task-03-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `009-feat-horizontal-scroll-task-03-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [009-feat-horizontal-scroll-decisions.md](009-feat-horizontal-scroll-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
