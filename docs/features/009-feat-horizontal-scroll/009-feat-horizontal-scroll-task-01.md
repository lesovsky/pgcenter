---
status: done                    # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 01: Pure column-window function + scroll-offset state

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Это тестируемое ядро фичи горизонтального скролла таблицы статистики в `pgcenter top`. Задача добавляет два независимых от рендеринга и хоткеев элемента:

1. **Поле состояния `scrollOffset int`** в структуру `config` (`top/config.go`) — эфемерная позиция горизонтального скролла текущего экрана. Это индекс по *скроллируемым* колонкам (тем, что идут после замороженной первой колонки): `0` означает «скролла нет». Поле живёт на `config`, а не на `view.View` — потому что `viewSwitchHandler` намеренно сохраняет per-view состояние (`OrderKey`, `Filters`, `ColsWidth`) в `config.views`, а скролл по требованию user-spec должен сбрасываться при каждом переключении (Decision 1 tech-spec).

2. **Чистая функция вычисления видимого окна колонок** в `top/stat.go`. По числу колонок, ширинам колонок (`ColsWidth`), ширине терминала и текущему offset она возвращает: видимый диапазон скроллируемых колонок, зажатый (clamped) offset и флаги `hiddenLeft`/`hiddenRight`. Первая колонка всегда заморожена (рендерится независимо от offset); остальные образуют скользящее окно. Эта функция — **единственный источник истины** для видимого диапазона: она пере-зажимает offset на каждом вызове (а не только в хоткей-хендлерах), что снимает риск устаревшего offset после авто-рефреша с изменившимся числом колонок (Decision 2, Risks tech-spec).

Это ядро вычислений извлекается в чистую функцию специально, чтобы единственную нетривиальную логику можно было покрыть unit-тестами без живого терминала. Рендеринг (Task 2) и хоткеи/сброс (Task 3) — отдельные задачи и в этой задаче НЕ реализуются.

## What to do

1. Добавить поле `scrollOffset int` в структуру `config` (`top/config.go`) с комментарием о смысле (индекс по скроллируемым колонкам `1..Ncols-1`, `0` = без скролла). Инициализация по умолчанию нулём; `newConfig` менять не нужно (нулевое значение корректно).

2. Реализовать чистую функцию вычисления видимого окна колонок в `top/stat.go` (рядом с `printStatHeader`/`printStatData`). Сигнатура — иллюстративна в tech-spec, финализируется здесь; ориентир:
   `func visibleColumns(ncols int, colsWidth map[int]int, termWidth, offset int) (first, last, clamped int, hiddenLeft, hiddenRight bool)`.
   Функция:
   - всегда включает колонку 0 (заморожена) в бюджет ширины;
   - вычисляет, сколько скроллируемых колонок (`1..ncols-1`) помещается, начиная с `1+offset`, суммируя `colsWidth[i]+2` (тот же `+2`-зазор, что использует печать) и сравнивая с `termWidth`;
   - зажимает offset в `[0, maxOffset]`, где `maxOffset` — максимальный сдвиг, при котором последняя колонка ещё видна (если всё помещается — `clamped == 0`);
   - возвращает видимый диапазон скроллируемых колонок `[first, last]` (абсолютные индексы) и флаги `hiddenLeft` (есть скрытые слева, т.е. `clamped > 0`) / `hiddenRight` (есть скрытые справа от окна);
   - читает `colsWidth` строго по индексам `[0, ncols)` — не итерировать по map, не обращаться к отсутствующим ключам (класс issue #99: чтение отсутствующего ключа map возвращает 0 и тихо ломает математику). Трактовать ширины как плотные для присутствующих колонок.

3. Написать unit-тесты (TDD — до реализации) в `top/stat_test.go`, переиспользуя/адаптируя хелпер `makeResult(ncols)` либо аналогичный для синтетических `ColsWidth`. Покрыть все кейсы из TDD Anchor.

## TDD Anchor

Тесты пишем ДО реализации: пишем → запускаем → убеждаемся что падают → пишем код → убеждаемся что проходят. Все — в `top/stat_test.go`, table-driven через `t.Run`.

- `top/stat_test.go::Test_visibleColumns` / "all columns fit" — широкий терминал, всё помещается → `clamped == 0`, `hiddenLeft == false`, `hiddenRight == false`, диапазон покрывает все скроллируемые колонки.
- `top/stat_test.go::Test_visibleColumns` / "narrow width, offset 0" — узкий терминал, offset 0 → видны замороженная + ведущие скроллируемые колонки, `hiddenRight == true`, `hiddenLeft == false`.
- `top/stat_test.go::Test_visibleColumns` / "mid offset" — offset в середине → `hiddenLeft == true` и `hiddenRight == true`, корректный диапазон `[first, last]`.
- `top/stat_test.go::Test_visibleColumns` / "offset past end" — offset больше валидного → зажат до `maxOffset`, `hiddenRight == false`, `hiddenLeft == true`.
- `top/stat_test.go::Test_visibleColumns` / "very narrow only frozen fits" — терминал вмещает только замороженную колонку → graceful диапазон, без паники, без отрицательной/нулевой ширины окна.
- `top/stat_test.go::Test_visibleColumns` / "missing or zero ColsWidth key" — в map отсутствует/нулевой ключ для колонки внутри `[0, ncols)` → без паники, математика остаётся ограниченной (issue #99 class).

## Acceptance Criteria

- [ ] Поле `config.scrollOffset int` добавлено в `top/config.go` с поясняющим комментарием; компиляция пакета `top` проходит.
- [ ] Реализована чистая функция вычисления видимого окна колонок в `top/stat.go`: возвращает видимый диапазон скроллируемых колонок, зажатый offset, флаги `hiddenLeft`/`hiddenRight`.
- [ ] Функция пере-зажимает offset в `[0, maxOffset]` на каждом вызове (single source of truth для диапазона).
- [ ] Первая колонка (индекс 0) всегда входит в бюджет ширины; offset индексирует только колонки `1..Ncols-1`.
- [ ] Чтение ширин происходит строго по индексам `[0, ncols)`; нет обращений к отсутствующим ключам map и нет итерации по map (защита от issue #99 class).
- [ ] Unit-тесты покрывают: all-fit (clamp к 0), scroll-right (`hiddenRight`), mid-scroll (оба флага), past-end clamp, very-narrow (только frozen, без паники), missing/zero ColsWidth key (без паники).
- [ ] `go test ./top/...` зелёный; существующие тесты (`Test_alignViewToResult`, `Test_formatError`, `Test_orderKeyLeft/Right`, и пр.) не сломаны.
- [ ] Рендеринг (`printStatHeader`/`printStatData`) и хоткеи в этой задаче НЕ менялись (это Task 2/Task 3).

## Context Files

**Feature artifacts:**
- [009-feat-horizontal-scroll.md](009-feat-horizontal-scroll.md) — user-spec
- [009-feat-horizontal-scroll-tech-spec.md](009-feat-horizontal-scroll-tech-spec.md) — tech-spec (Task 1, Decision 1/2, Testing Strategy, Risks)
- [009-feat-horizontal-scroll-code-research.md](009-feat-horizontal-scroll-code-research.md) — code research (§2 state, §5 line clipping, §7 align, §8 problems)
- [009-feat-horizontal-scroll-decisions.md](009-feat-horizontal-scroll-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — features, supported stats, target audience
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow, PG version handling
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — code patterns, testing conventions (table-driven, testify)

**Code files:**
- [top/config.go](../../../top/config.go) — добавить поле `scrollOffset int` в struct `config`
- [top/stat.go](../../../top/stat.go) — реализовать чистую функцию видимого окна колонок
- [top/stat_test.go](../../../top/stat_test.go) — unit-тесты функции (рядом с `Test_alignViewToResult`, `makeResult`)
- [top/config_view.go](../../../top/config_view.go) — read-only: `orderKeyLeft`/`orderKeyRight` как образец clamp (но scroll *clamp*, не wrap)
- [internal/align/align.go](../../../internal/align/align.go) — read-only: `SetAlign` флорит ширину колонки на 8; печать добавляет `+2`-зазор
- [internal/view/view.go](../../../internal/view/view.go) — read-only: `View.ColsWidth map[int]int`, `Ncols int` (runtime, версионно-зависимый)

## Verification Steps

- Запустить `go test ./top/...` — все unit-тесты функции видимого окна проходят, регрессий в существующих тестах нет.
- Проверить, что в `top/stat.go` нет обращений к `colsWidth` через range по map и нет чтения вне `[0, ncols)` (визуальная проверка кода функции).
- Убедиться, что `printStatHeader`/`printStatData` остались без изменений (diff затрагивает только новую функцию и новое поле + тесты).

## Details

<!-- All details for task execution — technical, organizational, any other. -->

**Files:**
- `top/config.go` — структура `config` (строки 10-19) сейчас содержит `view, views, queryOptions, viewCh, logtail, dialog, menu, procMask`. Добавить поле `scrollOffset int` с комментарием. `newConfig` (строки 22-29) менять не нужно — нулевое значение корректно.
- `top/stat.go` — добавить новую чистую функцию рядом с `printStatHeader` (строка 364) / `printStatData` (строка 398). Эти функции в Task 1 НЕ меняются; они получат окно из этой функции в Task 2.
- `top/stat_test.go` — уже содержит `Test_alignViewToResult` с локальным хелпером `makeResult(ncols)` (строки 64-73), создающим синтетический `stat.PGresult`. Для тестов функции окна нужен только `map[int]int` ширин + числа — можно сделать отдельный компактный хелпер или строить map инлайн. Стиль файла: table-driven через `t.Run`, `testify/assert`.

**Dependencies:**
- Зависимостей от других задач нет (`depends_on: []`, Wave 1). Task 2 (рендеринг) и Task 3 (хоткеи/сброс) зависят от результата этой задачи.
- Пакеты: `internal/math` (`math.Min`/`math.Max`, int-хелперы) для зажима offset — уже используется в `top/config_view.go`. Новых внешних пакетов нет.

**Edge cases:**
- Всё помещается → `clamped == 0`, оба флага `false`, окно = все скроллируемые колонки.
- Очень узкий терминал, вмещается только колонка 0 → не паниковать, не возвращать отрицательную/нулевую ширину окна; диапазон должен быть валиден (пустое окно скроллируемых колонок допустимо).
- `ncols == 1` (только замороженная колонка, нет скроллируемых) → `hiddenLeft == false`, `hiddenRight == false`, без паники.
- Отсутствующий/нулевой ключ в `ColsWidth` для колонки из `[0, ncols)` → без паники (issue #99 class); при штатном рендере `alignViewToResult` гарантирует плотность ширин, но функция должна быть устойчива сама по себе.
- offset больше `maxOffset` (например, после авто-рефреша с меньшим числом колонок) → зажать до `maxOffset`, `hiddenRight == false`.
- offset отрицательный (теоретически) → зажать до 0.

**Implementation hints (НЕ псевдокод):**
- Инвариант ширины терминала: при рендере ширина берётся из `v.Size()`; view `dbstat` создан с `Frame=false`, поэтому `Size()` отдаёт истинную ширину рисования. Функция принимает `termWidth` как параметр — она остаётся чистой и не знает про gocui; передача ширины — забота Task 2.
- Бюджет ширины: каждая печатаемая колонка занимает `ColsWidth[i]+2` ячеек (зазор `+2` добавляет печать, а не `SetAlign`). ANSI-escape-последовательности ячейки не занимают (§5 code-research), но это касается рендера, а не этой функции — здесь считаем только видимые ячейки.
- Ширина колонки всегда `>= 8` после `SetAlign` (флор в `align.go:36`) — но не закладывайся на это жёстко: функция должна быть устойчива к нулю/отсутствию ключа.
- `maxOffset` — это минимальный offset, при котором последняя колонка (`ncols-1`) ещё попадает в окно; вычислять его из ширин, а не угадывать. Зажим — через `math.Min`/`math.Max`.
- Образец clamp-логики: `orderKeyLeft`/`orderKeyRight` (`top/config_view.go`) — но там wrap-around по границе; скролл должен **clamp**, не wrap.
- Не реализуй здесь рендеринг (`‹`/`›` маркеры, bold замороженной колонки, абсолютная индексация значений) и не трогай хоткеи/сброс offset — это Task 2 и Task 3.

## Reviewers

- **dev-code-reviewer** → `009-feat-horizontal-scroll-task-01-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `009-feat-horizontal-scroll-task-01-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [009-feat-horizontal-scroll-decisions.md](009-feat-horizontal-scroll-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
