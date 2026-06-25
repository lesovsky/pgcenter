---
status: planned                    # planned -> in_progress -> done
depends_on: ["05", "07", "08"]     # ID задач-зависимостей (строки: ["01", "02"])
wave: 5                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: user                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 09: Tiering + latency guard + first-tick handling

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Финальный штрих в коллекторе verbose-режима: спрятать стоимость дорогих агрегатов за единственным
существующим knob `z` (интервал обновления) через **per-source тиринг + latency guard**, и довести
до конца обработку первого тика.

Контекст: в Wave 1–4 уже сделано — verbose-флаг на `view.View`/`config` (Task 2), all-three system
collection branch в `Collector.Update` (Task 7), новые pgstat-агрегаты и их collect-вызовы (Task 5),
composer'ы строк с `n/a`-семантикой (Task 8). Сейчас все verbose-источники собираются каждый тик
синхронно. Часть из них дорогие: **db sizes / growth** (`Σ pg_database_size` + Go-side дельта роста)
— у них **нет живой панели-двойника**, по которой DBA сверял бы значение, поэтому их можно
троттлить без потери консистентности. Остальные строки (system iostat/nicstat/filesyst, workload,
workers, replication, bgwr/ckpt) либо дешёвые, либо сверяются с полными панелями `B`/`N`/`F` и
экранами — их **нельзя** троттлить (Decision 9: «system rows every tick»).

Что делаем:
1. Группируем per-source cadence/latency state в именованный sub-struct `verboseCollectState` на
   `Collector` (вместо россыпи полей по shared-структуре — Decision 9: «named sub-struct avoids
   leaking verbose-specific fields»). Внутри: время последнего запуска дорогого источника,
   замеренная latency его последнего запроса, и кешированное последнее (stale) значение.
2. Latency guard: после замера времени `QueryRow` дорогого источника, если оно превысило порог —
   пропускаем следующие сборы этого источника и **отдаём кешированное (stale) значение, не `n/a`**.
   Порог finalize'им здесь как именованную константу: skip следующего сбора источника, если его
   последний запрос занял больше `max(25% от refresh interval, 500ms floor)`. Авто-resume, когда
   latency восстановилась.
3. First-tick: на самом первом verbose-тике у дорогих/дельтовых источников ещё нет prev → значение
   `n/a`; в это время в cmdline показываем подсказку `collecting...`, которая **очищается после
   первого успешного refresh**. Подсказка живёт в `top/stat.go` (рядом с `printStat`/`printCmdline`),
   не вводит нового user-knob.

Всё это — за единственным `z`. Никакого нового пользовательского переключателя интервала.

## What to do

- В `internal/stat/stat.go`:
  - Объявить sub-struct `verboseCollectState` с полями для per-source cadence (момент последнего
    сбора дорогого источника), per-source latency последнего запроса и кешированного последнего
    (stale) значения дорогого агрегата (db sizes / growth). Встроить его в `Collector`.
  - Объявить именованную константу порога latency guard здесь же (финализация Decision 9):
    относительный порог 25% от refresh interval с абсолютным полом 500ms. Дать ей говорящее имя и
    doc-комментарий, объясняющий «25% interval или 500ms floor».
  - В сборе дорогих no-twin агрегатов (db sizes / growth) — точка, добавленная в Task 5 — обернуть
    вызов: если источник троттлится (последняя latency превысила порог И с момента последнего
    реального сбора не прошёл достаточный интервал), не делать запрос, а вернуть кешированное stale
    значение. Иначе — замерить время запроса, обновить latency, обновить кеш и время последнего
    сбора. System-строки (all-three branch) и остальные pgstat-агрегаты НЕ троттлить — собираются
    каждый тик как сейчас.
  - На первом verbose-тике (нет prev / `verboseCollectState` ещё пустой) дорогие/дельтовые источники
    дают `n/a` через существующую composer-семантику Task 8 — здесь нужно лишь корректно сигналить
    «ещё собираем» (через флаг/состояние, доступное рендеру), не ломая последующие тики.
  - Авто-resume: когда замеренная latency источника снова в пределах порога, троттлинг этого
    источника снимается естественным образом (state хранит последнюю latency).
- В `top/stat.go`:
  - Завести признак первого verbose-тика и вывести в cmdline подсказку `collecting...`, пока первый
    успешный refresh ещё не отрисован; после первого успешного refresh подсказку очистить (вернуть
    cmdline к нормальному сообщению вида). Соблюсти mutual-exclusion `printCmdline` (см.
    patterns.md «printCmdline() — Mutual Exclusion»): один `printCmdline` на путь, через `if/else`,
    а не два последовательных вызова.

## TDD Anchor

Тесты пишем ДО реализации (no live PG — мокаем медленный источник и время). Пишем → запускаем →
убеждаемся что падают → пишем код → убеждаемся что проходят.

- `internal/stat/stat_test.go::Test_verboseCollectState_throttlesSlowSource` — источник, чья
  предыдущая latency превысила порог, на следующем тике НЕ запрашивается и отдаёт кешированное
  (stale) значение, а не `n/a`.
- `internal/stat/stat_test.go::Test_verboseCollectState_autoResumes` — когда latency источника
  возвращается в пределы порога, троттлинг снимается и источник снова собирается.
- `internal/stat/stat_test.go::Test_verboseCollectState_firstTickNA` — на первом verbose-тике
  (пустой state, нет prev) дорогой/дельтовый источник даёт `n/a`, а не stale-0.
- `internal/stat/stat_test.go::Test_latencyGuardThreshold` — table-тест границы порога:
  `max(25% interval, 500ms floor)` для нескольких значений refresh (напр. при 1s интервале активен
  500ms-floor; при 4s — активны 25% = 1s).
- `top/stat_test.go::Test_firstTickCollectingHint` — пока первый verbose-refresh не отрисован,
  cmdline содержит `collecting...`; после первого успешного refresh подсказка очищается.

## Acceptance Criteria

- [ ] На `Collector` есть именованный sub-struct `verboseCollectState` с per-source cadence/latency и
      кешем последнего stale-значения дорогих агрегатов; verbose-специфичные поля не размазаны по
      `Collector` россыпью.
- [ ] Порог latency guard finalized как именованная константа: `max(25% refresh interval, 500ms floor)`,
      с doc-комментарием.
- [ ] Медленный источник (db sizes / growth) при превышении порога троттлится и отдаёт **stale**
      кешированное значение, не `n/a`; авто-resume при восстановлении latency.
- [ ] System-строки и остальные pgstat-агрегаты собираются каждый тик (НЕ троттлятся) —
      консистентность с полными панелями сохранена (Decision 9).
- [ ] Первый verbose-тик: дорогие/дельтовые источники → `n/a`; в cmdline показана подсказка
      `collecting...`.
- [ ] Подсказка `collecting...` очищается после первого успешного refresh.
- [ ] Никакого нового пользовательского knob — всё за существующим `z`.
- [ ] `printCmdline` mutual-exclusion соблюдён (один вызов на путь, без перезаписи).
- [ ] `go test ./internal/stat/... ./top/...` зелёные; `make lint` и `govulncheck` чистые;
      компактный режим и поведение остальных экранов не задеты.

## Context Files

**Feature artifacts:**
- [010-feat-overview-dashboard.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard.md) — user-spec
- [010-feat-overview-dashboard-tech-spec.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-tech-spec.md) — tech-spec (Task 9, Decision 9)
- [010-feat-overview-dashboard-decisions.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-decisions.md) — decisions log
- [010-feat-overview-dashboard-code-research.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-code-research.md) — §5 (collection seam), §7-new (tiering/guard seam), first-tick note (строки 661–665)

**Project knowledge:**
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — пакетная раскладка, поток данных, collection seam
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — «printCmdline() — Mutual Exclusion», testable TUI rendering, error wrapping
- [overview.md](.claude/skills/project-knowledge/overview.md) — фичи и целевая аудитория

**Code files:**
- [internal/stat/stat.go](internal/stat/stat.go) — `Collector` struct + `Update`; добавить `verboseCollectState`, latency-guard константу, троттлинг дорогих агрегатов
- [top/stat.go](top/stat.go) — `collectStat`/`printStat`; first-tick `collecting...` подсказка в cmdline
- [top/config_view.go](top/config_view.go) — read-only: примеры `printCmdline` mutual-exclusion и view-switch wiring

## Verification Steps

verify: **user** — `make build`, затем ручная TUI-проверка:
- Шаг 1: открыть pgcenter, включить verbose (`v`). На первом тике в дорогих строках (db sizes /
  growth) — `n/a`, в cmdline — подсказка `collecting...`.
- Шаг 2: после первого успешного refresh подсказка `collecting...` исчезает, дорогие строки
  заполняются.
- Шаг 3: смоделировать «медленный источник» (большой инстанс / много БД или искусственная задержка) —
  убедиться, что дорогая строка остаётся со **stale** значением (не мигает в `n/a`), system-строки
  при этом обновляются каждый тик.
- Шаг 4: когда задержка спадает — троттлинг снимается, строка снова обновляется (auto-resume).
- Перед ручной проверкой обязательно `make build` (patterns.md: один QA-сеанс = одна свежая сборка).
- Авто-часть для исполнителя: `go test ./internal/stat/... ./top/...`, `make lint`, `govulncheck`.

## Details

<!-- All details for task execution — technical, organizational, any other. -->

**Files:**
- `internal/stat/stat.go` — текущее состояние: `Collector` (строки 51–74) хранит per-source prev/curr
  снапшоты и `config Config`. `Update` (строки 122–289) собирает каждый источник синхронно каждый тик;
  Task 5 уже добавил сюда collect-вызовы новых pgstat-агрегатов (включая дорогие db sizes / growth),
  Task 7 — all-three system branch. Что сделать: добавить sub-struct `verboseCollectState` и встроить
  в `Collector`; объявить именованную latency-guard константу; обернуть **только** дорогой no-twin
  агрегат (db sizes / growth) троттлингом со stale-кешем; system-строки и прочие pgstat-агрегаты
  оставить каждый тик. `Reset()` (строки 111–119) — учесть: сброс при view-switch не должен
  оставлять «висящий» throttle-state, мешающий первому тику (вероятно, сбрасывать/инициализировать
  `verboseCollectState` согласованно с prev/curr).
- `top/stat.go` — `collectStat` (строки 25–123) гоняет единственный ticker; `printStat` (строки
  126+) рисует панели; `printCmdline` определён в `top/ui.go:187`. Что сделать: признак первого
  verbose-тика и вывод `collecting...` в cmdline до первого успешного refresh, очистка после.
- `top/config_view.go` — только для чтения: образцы корректного `printCmdline` (строки 108, 156,
  260, 288–299) и 4-веточный switch при нескольких независимых пробах.

**Dependencies:**
- Task 5 — дорогие pgstat-агрегаты (db sizes / growth) и их collect-вызовы в `Update` (то, что
  троттлим).
- Task 7 — all-three system collection branch (то, что НЕ троттлим).
- Task 8 — composer'ы строк с `n/a`-семантикой первого тика (рендер `n/a` уже есть; здесь —
  сигнал/состояние + cmdline-подсказка).
- Пакеты: stdlib `time` (уже импортирован в `stat.go`). Новых пакетов нет.

**Edge cases:**
- Первый verbose-тик: prev пустой → дорогие/дельтовые источники `n/a` + `collecting...`; не показывать
  stale-0.
- Источник восстановился (latency снова в норме) → auto-resume без ручного действия.
- View-switch / `Reset()` при включённом verbose: throttle-state не должен «застрять» и блокировать
  первый сбор после переключения экрана.
- Очень короткий `z` (напр. 1s): срабатывает 500ms-floor, а не 25%; очень длинный — 25% interval.
- Ошибка/недоступность дорогого источника отдельно от троттлинга → её обрабатывает `n/a`-семантика
  Task 8; здесь не маскировать ошибку под stale.
- `printCmdline` mutual-exclusion: не вызывать `printCmdline(hint)` и затем `printCmdline(msg)` в
  одном пути (второй перезатрёт первый) — ветвить через `if/else`.

**Implementation hints:**
- §7-new code-research: per-source rhythm/latency-guard state живёт на `Collector` (long-lived
  per-session объект, уже держит prev/curr). Дорогой агрегат гейтится на
  `time.Since(lastRun) >= budget` И/ИЛИ замеренной latency; иначе — кешированное stale-значение.
- §5 code-research: «no second user knob» — все divisors/budget'ы выводятся из переданного в
  `Update` `refresh`, не из нового настраиваемого поля.
- Decision 9 (tech-spec): троттлить ТОЛЬКО no-twin агрегаты (db sizes, growth); system rows every
  tick; throttled source keeps last (stale) value, not `n/a`; named sub-struct; default guard
  threshold ~25% refresh или ~500ms floor — finalize константу в этой задаче.
- Замер latency: засечь `time.Now()` вокруг `db.QueryRow`/scan дорогого источника (паттерн рядом с
  `collectActivityStat`), сохранить в `verboseCollectState`.
- patterns.md «Testable TUI Rendering»: держать решение «троттлить/отдать stale» по возможности
  чистым (тестируемым без live PG) — выделить функцию, принимающую last-latency/last-run/refresh и
  возвращающую «collect или stale», чтобы покрыть table-тестом порога без gocui и без Postgres.
- patterns.md «Error Wrapping»: `fmt.Errorf("…: %w", err)` в production; `printCmdline` — исключение
  (`%s`).
- Не логировать сырой текст ошибки PG в cmdline-подсказке (риск утечки путей — см. tech-spec Risks).

## Reviewers

- **dev-code-reviewer** → `docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-task-09-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-task-09-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [010-feat-overview-dashboard-decisions.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Зафиксировать финализированную константу порога latency guard (точное значение 25% / 500ms-floor) в decisions, если отклонились от Decision 9
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
