---
status: planned                    # planned -> in_progress -> done
depends_on: ["02", "05"]           # ID задач-зависимостей (строки: ["01", "02"])
wave: 3                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 07: All-three system collection branch

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Когда verbose-режим включён, верхняя панель `sysstat` показывает три расширенные системные строки —
iostat (диск), nicstat (сеть) и filesyst (ФС). Для их отрисовки (Task 8) коллектору нужно на КАЖДОМ тике
собирать все три источника одновременно: `Diskstats`, `Netdevs`, `Fsstats`.

Сегодня в `Collector.Update` (`internal/stat/stat.go:156-175`) стоит `switch c.config.collectExtra`, который
собирает РОВНО ОДИН из источников — потому что боковые панели (`B`/`N`/`F`) показывают по одной за раз и
взаимоисключаемы. Этот `switch` трогать нельзя (R1: иначе изменится поведение боковых панелей).

Задача — добавить отдельную verbose-ветку ПОСЛЕ существующего `switch`, читающую `view.Verbose` (новый флаг
из Task 2, приезжает на коллектор по `viewCh` как часть `view.View`). Ветка собирает все три источника c
`== nil` guard'ами (чтобы не пересобрать тот, что уже наполнила активная боковая панель), переиспользуя те же
методы `c.collectDiskstats`/`c.collectNetdevs`/`c.collectFsstats` — а значит те же структуры и ту же
математику `%util`, что и полные панели (consistency, Decision 5). Per-source prev/curr-снимки уже живут на
`Collector`, так что сбор всех трёх не мешает боковым панелям.

Ключевое отличие от бокового `switch`: ветка НЕ должна делать `return s, err` при ошибке одного источника —
по требованию спека одна сбойная подсистема (нет сети, EACCES на `/proc`, нет remote-схемы) не должна
обнулять остальные строки. Ошибка фиксируется как недоступность источника (источник остаётся `nil` →
рендер выдаст `n/a` в Task 8), а сбор остальных продолжается.

**Сигнал первого тика (first-tick `n/a`).** Дельта-метрики (iostat/nicstat) на самом первом verbose-тике
не имеют валидной предыдущей точки. ВАЖНО: нельзя полагаться на `s.Diskstats == nil`/`s.Netdevs == nil`
как на признак первого тика. `collectDiskstats`/`collectNetdevs` (`internal/stat/stat.go`, см. строки
~303-331 — диапазон приблизительный, сверяйся с актуальным файлом) при несовпадении длин снимков делают
`prev = curr` ДО вызова `count*Usage`, поэтому на честном первом verbose-тике композер получает НАПОЛНЕННЫЙ
срез с нулевой дельтой, а не `nil`. Значит признак первого тика должен быть явным: эта задача добавляет на
`Collector` флаг `verboseFirstTick bool`, который выставляется на первом verbose-сборе всех трёх источников и
сбрасывается после этого одного тика. Композеры строк из Task 8 читают этот флаг и рисуют `n/a` в
дельта-зависимых системных ячейках на первом кадре вместо обманчивого `0`.

**Re-arm флага при OFF→ON (КРИТИЧНО).** Нельзя завязывать перевзвод `verboseFirstTick` на `c.Reset()`:
по Decision 2 `toggleVerbose` намеренно НЕ вызывает `Reset()` (verbose-only-тоггл не переключает view), а
значит при повторном включении verbose БЕЗ смены экрана `Reset()` не сработает. Если положиться на него,
сценарий «verbose ON → OFF → ON без переключения view» оставит флаг невыставленным, и новая verbose-сессия
посчитает дельты против УСТАРЕВШЕГО prev-снимка (фиктивный скачок через OFF-паузу) вместо честного `n/a`.

Поэтому эта задача добавляет на `Collector` ВТОРОЕ явное поле — `prevVerboseActive bool` (был ли verbose
активен на ПРЕДЫДУЩЕМ тике). Логика в verbose-ветке: если verbose-ветка выполняется (verbose ON) И
`prevVerboseActive == false` (на прошлом тике verbose был выключен) → это либо самый первый тик, либо
OFF→ON re-enable → выставить `verboseFirstTick = true`. После отработки тика: сбросить `verboseFirstTick`
обратно в `false` и выставить `prevVerboseActive = true`. Когда verbose выключен (ветка не выполняется) —
сбросить `prevVerboseActive = false`, чтобы следующий OFF→ON снова перевзвёл флаг. Этот механизм НЕ зависит
от `Reset()`.

Оба поля (`verboseFirstTick` и `prevVerboseActive`) держать forward-compatible: Task 9 позже сгруппирует их
в `verboseCollectState`, поэтому не завязывать на них ничего лишнего и не делать их частью публичного
контракта (приватные поля `Collector`, без геттеров).

## What to do

1. Добавить в `Collector.Update` (`internal/stat/stat.go`), сразу ПОСЛЕ закрывающей скобки существующего
   `switch c.config.collectExtra` (строка ~175) и до строки `itv := int(refresh / time.Second)`, новую
   ветку `if view.Verbose { ... }`.
2. Внутри ветки последовательно собрать три источника, каждый под своим `== nil` guard'ом:
   - если `s.Diskstats == nil` → вызвать `c.collectDiskstats(db)`, при успехе записать в `s.Diskstats`;
   - если `s.Netdevs == nil` → вызвать `c.collectNetdevs(db)`, при успехе записать в `s.Netdevs`;
   - если `s.Fsstats == nil` → вызвать `c.collectFsstats(db)`, при успехе записать в `s.Fsstats`.
3. На ошибке любого источника НЕ возвращать `s, err` из `Update`. Вместо этого зафиксировать недоступность
   источника так, чтобы одна сбойная подсистема не обнуляла остальные две строки и не прерывала сэмпл.
   Соответствующее поле `s.*` остаётся `nil` (Task 8 отрисует `n/a`).
4. Добавить на `Collector` ДВА явных приватных поля рядом с per-source снимками (строки ~57-64):
   `verboseFirstTick bool` (нужно ли композеру рисовать `n/a` на этом тике) и `prevVerboseActive bool`
   (был ли verbose активен на предыдущем тике). Логика re-arm в verbose-ветке:
   - если verbose-ветка выполняется И `prevVerboseActive == false` → ВЫСТАВИТЬ `verboseFirstTick = true`
     (покрывает и самый первый тик, и каждый OFF→ON re-enable БЕЗ смены view);
   - после отработки тика СБРОСИТЬ `verboseFirstTick = false` и выставить `prevVerboseActive = true`.
   Когда verbose ВЫКЛЮЧЕН (ветка `if view.Verbose` не выполняется): сбросить `prevVerboseActive = false`,
   чтобы следующий OFF→ON снова перевзвёл флаг. НЕ полагаться на `c.Reset()` для перевзвода — по Decision 2
   `toggleVerbose` его не зовёт. НЕ полагаться на `s.Diskstats == nil` — на первом тике поле уже наполнено
   нулевой дельтой (см. Description). Оба поля forward-compatible под будущий `verboseCollectState` (Task 9):
   без публичного контракта, минимально.
5. Существующий `switch c.config.collectExtra` оставить полностью без изменений (R1).
6. Написать новый live-PG тест `TestCollector_Update_Verbose` в `internal/stat/stat_test.go` по паттерну
   `TestCollector_Update` (connect-or-fail через `postgres.NewTestConnect()` + `assert.NoError`), но с
   `view.Verbose = true` и проверками: (а) все три — `Diskstats`, `Netdevs`, `Fsstats` — наполнены; (б) флаг
   `verboseFirstTick` выставлен после первого `Update` и сброшен после второго; (в) OFF→ON re-enable БЕЗ
   смены view: после нескольких verbose-тиков выполнить один `Update` с `Verbose=false`, затем снова
   `Verbose=true` (НЕ дёргая `c.Reset()`) — `verboseFirstTick` должен снова стать `true` на этом тике и
   сброситься на следующем. Существующий `TestCollector_Update` и `TestCollector_collectDiskstats` должны
   остаться зелёными без изменений.

## TDD Anchor

Пишем тесты ДО реализации, убеждаемся что падают, пишем код, убеждаемся что проходят.

- `internal/stat/stat_test.go::TestCollector_Update_Verbose` — на live-PG (connect-or-fail через
  `postgres.NewTestConnect()` + `assert.NoError`, как в существующих stat-тестах; `t.Skipf` они НЕ
  используют — если нужен skip-guard, использовать тот, что принят в проекте) собрать `Collector.Update`
  для view с `Verbose=true`; ассертить, что `len(stat.System.Diskstats) != 0`, `len(stat.System.Netdevs) != 0`,
  `len(stat.System.Fsstats) != 0` одновременно в одном сэмпле.
- `internal/stat/stat_test.go::TestCollector_Update_Verbose` (first-tick flag) — после ПЕРВОГО verbose
  `Update` флаг первого тика на коллекторе выставлен (`c.verboseFirstTick == true`); после ВТОРОГО verbose
  `Update` он сброшен (`c.verboseFirstTick == false`). Это защищает механизм `n/a` Task 8: на первом тике
  дельта-источники уже наполнены нулевой дельтой (не `nil`), поэтому признак первого тика обязан быть явным.
- `internal/stat/stat_test.go::TestCollector_Update_Verbose` (OFF→ON re-arm, без смены view) — после
  установившейся verbose-сессии (флаг уже `false`) сделать один `Update` с `Verbose=false`, затем снова
  `Verbose=true` БЕЗ вызова `c.Reset()`; на этом re-enable-тике `c.verboseFirstTick == true`, на следующем
  verbose-тике снова `false`. Это закрывает дыру: т.к. `toggleVerbose` не зовёт `Reset()` (Decision 2),
  перевзвод нельзя строить на `Reset()` — иначе re-enable посчитал бы дельту против устаревшего prev-снимка
  вместо `n/a`.
- `internal/stat/stat_test.go::TestCollector_Update` (существующий) — остаётся зелёным: при
  `collectExtra = CollectDiskstats` и view без verbose всё ещё наполняется только `Diskstats`, поведение
  боковой панели не изменилось.

## Acceptance Criteria

- [ ] При `view.Verbose == true` за один тик наполняются все три источника: `Diskstats`, `Netdevs`, `Fsstats`.
- [ ] Существующий `switch c.config.collectExtra` не изменён (боковые панели `B`/`N`/`F` работают как раньше).
- [ ] `== nil` guard'ы предотвращают повторный сбор источника, уже наполненного активной боковой панелью.
- [ ] Ошибка одного источника не приводит к `return s, err` и не обнуляет два других (источник остаётся `nil`).
- [ ] На `Collector` есть явный флаг первого verbose-тика (`verboseFirstTick`): выставлен после первого verbose `Update`, сброшен после второго (механизм `n/a` Task 8 НЕ полагается на `s.Diskstats == nil`).
- [ ] Перевзвод флага НЕ зависит от `c.Reset()`: есть поле `prevVerboseActive`, и сценарий «verbose OFF→ON без смены view» (без `Reset()`) снова выставляет `verboseFirstTick = true` на re-enable-тике и сбрасывает на следующем.
- [ ] При `view.Verbose == false` поведение `Update` байт-в-байт прежнее (ветка не выполняется).
- [ ] `go test ./internal/stat/...` зелёный; существующие stat-тесты не падают.

## Context Files

**Feature artifacts:**
- [010-feat-overview-dashboard.md](010-feat-overview-dashboard.md) — user-spec
- [010-feat-overview-dashboard-tech-spec.md](010-feat-overview-dashboard-tech-spec.md) — tech-spec (Task 7, Decision 4, R1)
- [010-feat-overview-dashboard-code-research.md](010-feat-overview-dashboard-code-research.md) — §5-new (all-three branch)
- [010-feat-overview-dashboard-decisions.md](010-feat-overview-dashboard-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — project context (features, audience)
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow, PG version handling
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — Error Wrapping, testing conventions

**Code files:**
- [internal/stat/stat.go](internal/stat/stat.go) — изменить: добавить verbose-ветку после `collectExtra` switch
- [internal/stat/stat_test.go](internal/stat/stat_test.go) — добавить: `TestCollector_Update_Verbose`
- [internal/stat/diskstats.go](internal/stat/diskstats.go) — прочитать: `collectDiskstats`/`countDiskstatsUsage`
- [internal/stat/netdev.go](internal/stat/netdev.go) — прочитать: `collectNetdevs`/`countNetdevsUsage`
- [internal/stat/fsstat.go](internal/stat/fsstat.go) — прочитать: `collectFsstats`/`readFsstats`

## Verification Steps

- Запустить `go test ./internal/stat/...` — все тесты зелёные (live-PG тесты могут `t.Skipf` без кластера).
- Убедиться, что новый `TestCollector_Update_Verbose` наполняет все три источника одновременно.
- Убедиться, что существующие `TestCollector_Update` и `TestCollector_collectDiskstats` проходят без правок.
- `make lint` — чисто (golangci-lint + gosec) по затронутым файлам.

## Details

**Files:**
- `internal/stat/stat.go` — текущее состояние: `Update` (строки 122-289) после сбора CPU/mem/load делает
  `switch c.config.collectExtra` (156-175), который собирает один из disk/net/fs и при ошибке делает
  `return s, err`. Снимки `prev/currDiskstats`, `prev/currNetdevs`, `currFsstats` уже на `Collector`
  (57-64). Методы `c.collectDiskstats` (297), `c.collectNetdevs` (317), `c.collectFsstats` (337) каждый
  ведут свой prev/curr и возвращают `(usage, error)`. Что сделать: (а) добавить на `Collector` рядом с
  per-source снимками (строки ~57-64) ДВА приватных поля: `verboseFirstTick bool` и `prevVerboseActive bool`;
  (б) вставить `if view.Verbose { ... } else { c.prevVerboseActive = false }` сразу после строки 175
  (закрытие switch), до `itv := ...` (178). `view` — параметр `Update` (`view view.View`), поле
  `view.Verbose` приходит из Task 2. В verbose-ветке: при входе, если `!c.prevVerboseActive` →
  `c.verboseFirstTick = true` (первый тик ИЛИ OFF→ON re-enable); в конце ветки → `c.verboseFirstTick = false`,
  `c.prevVerboseActive = true`. Перевзвод НЕ через `c.Reset()` (Decision 2: `toggleVerbose` его не зовёт).
  Оба поля forward-compatible под `verboseCollectState` (Task 9).
- `internal/stat/stat_test.go` — текущее состояние: `TestCollector_Update` (27-69) задаёт
  `c.config.collectExtra = CollectDiskstats` и ассертит наполнение `Diskstats`. Что сделать: добавить
  сиблинг-тест `TestCollector_Update_Verbose` по тому же скелету (та же `views`-карта, `NewOptions`,
  `Configure`, connect-or-fail через `postgres.NewTestConnect()` — `t.Skipf` существующие stat-тесты НЕ
  используют), но выставить `Verbose = true` на используемом view перед `c.Update(...)`, ассертить все три
  источника и поведение флага: первый verbose `Update` → `c.verboseFirstTick == true`, второй → `false`;
  затем OFF→ON без `c.Reset()` (один `Update` с `Verbose=false`, потом снова `Verbose=true`) → флаг снова
  `true`, на следующем тике → `false`.

**Dependencies:**
- Task 2 — вводит поле `view.View.Verbose bool` и доставку флага на коллектор по `viewCh`. Без него
  `view.Verbose` не скомпилируется. (depends_on: 02)
- Task 5 — последняя задача, менявшая `internal/stat/stat.go` в Wave 2 (тот же файл, предыдущая волна);
  работать поверх её версии файла, не откатывая её изменения. (depends_on: 05)
- Stdlib only, новых пакетов нет.

**Edge cases:**
- Пользователь на боковой панели `B` (iostat) И включил verbose: `s.Diskstats` уже наполнен switch'ем →
  `== nil` guard пропускает повторный `collectDiskstats`, остальные два собираются.
- Нет сети / нет интерфейсов → `collectNetdevs` ошибка или пустой результат: `s.Netdevs` остаётся `nil`,
  `Diskstats`/`Fsstats` собираются нормально, сэмпл не прерывается.
- Remote-подключение без pgcenter-схемы: `read*` возвращают пустые срезы (см. `readDiskstats`: возвращает
  `Diskstats{}` когда `!db.Local && !SchemaPgcenterAvail`) — это не ошибка, не должно ронять ветку.
- Первый тик verbose: ВНИМАНИЕ — `count*Usage` НЕ возвращает `nil`. `collectDiskstats`/`collectNetdevs`
  (строки ~303-331, диапазон приблизительный — сверяйся с актуальным файлом) при несовпадении длин снимков
  делают `prev = curr` ДО `count*Usage`, поэтому на первом тике дельта = 0, а срез НАПОЛНЕН (не `nil`).
  Признак первого тика для `n/a` (Task 8) нельзя выводить из `s.Diskstats == nil` — нужен явный флаг
  `verboseFirstTick` на коллекторе, который эта задача выставляет на первом verbose-проходе и сбрасывает
  после. Сам рендер `n/a` — в Task 8, но СИГНАЛ предоставляет эта задача.
- OFF→ON re-enable без смены view: пользователь выключил verbose, затем снова включил, оставаясь на том же
  экране. `toggleVerbose` НЕ зовёт `c.Reset()` (Decision 2), поэтому перевзвод `verboseFirstTick` нельзя
  строить на `Reset()`. На OFF-тике `prevVerboseActive` сбрасывается в `false`; на следующем ON-тике ветка
  видит `!prevVerboseActive` и снова выставляет `verboseFirstTick = true` → `n/a` вместо фиктивного скачка
  дельты через OFF-паузу против устаревшего prev-снимка.
- `view.Verbose == false` — verbose-ветка не выполняется; единственное действие в `else` — сброс
  `prevVerboseActive = false`. Прежнее поведение боковых панелей не меняется.

**Implementation hints:**
- Не копировать паттерн бокового switch с `return s, err` — это прямо нарушает требование «одна сбойная
  подсистема не обнуляет остальные» (tech-spec Risks, R1 и строка про per-source error).
- `collect*` методы уже инкапсулируют prev/curr-свопы и `count*Usage` — звать именно их, а не `read*`
  напрямую, чтобы получить те же usage-структуры, что и полные панели (Decision 5, consistency).
- Не логировать сырой текст ошибки PG/пути (может содержать пути ФС) — фиксировать только факт недоступности.
- Соблюдать порядок: ветка строго ПОСЛЕ существующего switch, чтобы `== nil` guard видел уже наполненный
  боковой панелью источник.
- Флаги `verboseFirstTick` и `prevVerboseActive` держать приватными полями `Collector` и минимальными:
  Task 9 сгруппирует их (и, возможно, соседние verbose-снимки) в `verboseCollectState`, поэтому не плодить
  вокруг них геттеры/публичный контракт. Логика: при входе в verbose-ветку, если `!c.prevVerboseActive` —
  выставить `c.verboseFirstTick = true` (покрывает и первый тик, и OFF→ON re-enable); в конце ветки —
  `c.verboseFirstTick = false` и `c.prevVerboseActive = true`; в `else`-ветке (verbose off) —
  `c.prevVerboseActive = false`. НЕ выводить «первый тик» из `s.Diskstats == nil` и НЕ опираться на
  `c.Reset()` (Decision 2: `toggleVerbose` его не вызывает).

## Reviewers

- **dev-code-reviewer** → `010-feat-overview-dashboard-task-07-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `010-feat-overview-dashboard-task-07-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [010-feat-overview-dashboard-decisions.md](010-feat-overview-dashboard-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
