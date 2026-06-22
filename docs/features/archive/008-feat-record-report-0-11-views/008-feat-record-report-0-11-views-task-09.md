---
status: done                       # planned -> in_progress -> done
depends_on: ["02"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 3                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 09: Tech-debt [004] — export procpidstat column-index constants

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Закрываем техдолг **[004]**: индексы колонок procpidstat для IO/iodelay-метрик
(9 = `read_total,KiB`, 10 = `write_total,KiB`, 11 = `iodelay_total,s`) сейчас живут в двух
местах. Авторитетный порядок колонок задан в `internal/stat/procpidstat.go` неэкспортируемым
срезом `procPidResultCols` (procpidstat.go:18-27), но сами индексы там не экспортированы. В
`report/report.go` те же числа продублированы локальным const-блоком (report.go:342-346:
`procPidStatColReadTotalKiB`, `procPidStatColWriteTotalKiB`, `procPidStatColIODelayTotalS`),
используемым только в `emitProcPidStatAvailabilityWarnings` (report.go:355-387).

Если порядок колонок изменится, придётся править оба места — и без перекрёстной ссылки легко
получить рассинхрон. Задача: экспортировать именованные индексные константы из
`internal/stat/procpidstat.go`, удалить локальный дубликат в `report.go` и переключить
`emitProcPidStatAvailabilityWarnings` на `stat.Col*`. `report` уже импортирует `internal/stat`
(report.go:9), цикла импортов нет (`report → stat` односторонний; `stat` не импортирует
`report`).

Эта задача — чистый внутренний рефакторинг без изменения поведения. Внешних потребителей нет
(pgcenter — приложение, не библиотека). `depends_on: ["02"]` — Task 02 тоже правит
`report/report.go` (другой участок: `describeReport` map / новые константы описаний), задачи
секвенированы, чтобы избежать параллельного конфликта по файлу.

## What to do

1. В `internal/stat/procpidstat.go` рядом с `procPidResultCols` (и существующим
   `procPidResultNcols`) добавить экспортируемые именованные константы индексов колонок для
   IO/iodelay-метрик: `ColReadTotalKiB = 9`, `ColWriteTotalKiB = 10`, `ColIODelayTotalS = 11`.
   Снабдить их doc-комментарием, поясняющим, что индексы должны соответствовать порядку в
   `procPidResultCols`.
2. В `report/report.go` удалить локальный const-блок (report.go:342-346) с
   `procPidStatColReadTotalKiB` / `procPidStatColWriteTotalKiB` / `procPidStatColIODelayTotalS`.
3. В `emitProcPidStatAvailabilityWarnings` (report.go:355-387) заменить все три ссылки на
   локальные константы (на строках :359, :376, :381) на `stat.ColReadTotalKiB`,
   `stat.ColWriteTotalKiB`, `stat.ColIODelayTotalS` соответственно.
4. Убедиться, что `report` по-прежнему компилируется, импорт `internal/stat` остаётся
   используемым, удалённых импортов не появилось.
5. Прогнать целевые тесты — поведение `emitProcPidStatAvailabilityWarnings` не меняется,
   существующие тесты WARNING-пути в `report` должны оставаться зелёными.

## TDD Anchor

Это рефакторинг с сохранением поведения — новые тесты не добавляются. Регрессию ловят
существующие тесты WARNING-пути procpidstat в `report` и компиляция `internal/stat`.

- `report` тесты WARNING-детекции (`emitProcPidStatAvailabilityWarnings`) — должны проходить
  без изменений после переключения на `stat.Col*` (одинаковые индексы → одинаковое поведение).
- `go test ./report/... ./internal/stat/...` — оба пакета компилируются и проходят.

## Acceptance Criteria

- [ ] В `internal/stat/procpidstat.go` экспортированы именованные индексные константы
      `ColReadTotalKiB = 9`, `ColWriteTotalKiB = 10`, `ColIODelayTotalS = 11` с doc-комментарием.
- [ ] Локальный const-блок `procPidStatColReadTotalKiB`/`...WriteTotalKiB`/`...IODelayTotalS`
      в `report/report.go` (342-346) удалён.
- [ ] `emitProcPidStatAvailabilityWarnings` ссылается на `stat.Col*` вместо локальных констант;
      поведение функции не изменилось.
- [ ] Значения констант (9/10/11) совпадают с порядком в `procPidResultCols`.
- [ ] `go test ./report/... ./internal/stat/...` зелёный; `make lint` без новых замечаний.

## Context Files

**Feature artifacts:**
- [008-feat-record-report-0-11-views.md](008-feat-record-report-0-11-views.md) — user-spec
- [008-feat-record-report-0-11-views-tech-spec.md](008-feat-record-report-0-11-views-tech-spec.md) — tech-spec
- [008-feat-record-report-0-11-views-decisions.md](008-feat-record-report-0-11-views-decisions.md) — decisions log
- [008-feat-record-report-0-11-views-code-research.md](008-feat-record-report-0-11-views-code-research.md) — code research (см. §10 [004])

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md)
- [architecture.md](.claude/skills/project-knowledge/architecture.md)
- [patterns.md](.claude/skills/project-knowledge/patterns.md)

**Tech-debt register:**
- [docs/tech-debt.md](docs/tech-debt.md) — пункт [004]

**Code files:**
- [internal/stat/procpidstat.go](internal/stat/procpidstat.go) — добавить экспортируемые индексные константы рядом с `procPidResultCols`
- [report/report.go](report/report.go) — удалить локальный const-блок (342-346), переключить `emitProcPidStatAvailabilityWarnings` на `stat.Col*`

## Verification Steps

- Запустить `go test ./report/... ./internal/stat/...` — оба пакета компилируются и тесты зелёные.
- Проверить, что в `report/report.go` больше нет идентификаторов `procPidStatColReadTotalKiB`,
  `procPidStatColWriteTotalKiB`, `procPidStatColIODelayTotalS` (grep — ноль вхождений).
- Проверить, что `emitProcPidStatAvailabilityWarnings` использует `stat.ColReadTotalKiB`,
  `stat.ColWriteTotalKiB`, `stat.ColIODelayTotalS`.
- `make lint` — без новых замечаний.

## Details

**Files:**
- `internal/stat/procpidstat.go` — текущее состояние: канонический порядок колонок в
  неэкспортируемом `var procPidResultCols []string` (строки 18-27); рядом `const
  procPidResultNcols = 19` (строка 29). Экспортируемых индексных констант нет. Что сделать:
  добавить экспортируемые `ColReadTotalKiB = 9`, `ColWriteTotalKiB = 10`, `ColIODelayTotalS = 11`
  (порядок в срезе: индекс 9 = `read_total,KiB`, 10 = `write_total,KiB`, 11 = `iodelay_total,s`,
  строки 21-22 среза). Doc-комментарий должен привязывать их к `procPidResultCols`.
- `report/report.go` — текущее состояние: локальный `const (...)` блок на 342-346
  (`procPidStatColReadTotalKiB=9`, `procPidStatColWriteTotalKiB=10`,
  `procPidStatColIODelayTotalS=11`); используется только в `emitProcPidStatAvailabilityWarnings`
  (355-387), конкретно в guard `res.Ncols <= procPidStatColIODelayTotalS` (:359) и в вызовах
  `allEmpty(...)` (:376, :381). Импорт `internal/stat` уже присутствует (строка 9). Что сделать:
  удалить const-блок 342-346 (включая комментарий над ним, который дублирует ту же информацию,
  что и новый doc-комментарий в `procpidstat.go`), заменить три ссылки на `stat.Col*`.

**Dependencies:**
- depends_on Task 02 — обе задачи правят `report/report.go` (Task 02: `describeReport` map и
  новые константы описаний; этот таск: const-блок procpidstat + `emitProcPidStatAvailabilityWarnings`).
  Участки непересекающиеся, но секвенирование снимает риск конфликта при параллельном выполнении.
- Пакеты: новых зависимостей нет.

**Edge cases:**
- Нет цикла импортов: `report → stat` односторонний, `stat` не импортирует `report` (проверено
  в code-research §10 [004]).
- Поведение `emitProcPidStatAvailabilityWarnings` должно остаться идентичным — значения 9/10/11
  не меняются, меняется только источник констант.
- Не «улучшать» соседний код (`allEmpty`, тексты WARNING) — только перенос констант.

**Implementation hints:**
- Имена `Col*` намеренно короткие, т.к. читаются как `stat.ColReadTotalKiB` — пакет даёт
  смысловой префикс. Не вводить лишний префикс вроде `stat.ProcPidCol*`, если этого не требует
  существующий стиль пакета (в `internal/stat` экспортируемые имена не префиксируются именем
  фичи).
- Точные ссылки и номера строк — в code-research §10 [004] и в `docs/tech-debt.md` [004].
- После правок убедиться, что не остался «мёртвый» комментарий, ссылающийся на удалённые
  локальные константы.

## Reviewers

- **dev-code-reviewer** → `008-feat-record-report-0-11-views-task-09-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `008-feat-record-report-0-11-views-task-09-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [008-feat-record-report-0-11-views-decisions.md](008-feat-record-report-0-11-views-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
- [ ] Отметить техдолг [004] как Resolved в [docs/tech-debt.md](docs/tech-debt.md) при финализации фичи
