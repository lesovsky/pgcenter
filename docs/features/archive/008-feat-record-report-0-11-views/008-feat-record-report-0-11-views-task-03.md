---
status: done                       # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — `go test ./cmd/report/...`
reviewers: [dev-code-reviewer, dev-test-reviewer]
teammate_name:
---

# Task 03: CLI report flags for the new screens

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Фича снимает `NotRecordable` с четырёх экранов 0.11.0 (5 report-типов: `bgwriter`,
`replslots`, `stat_io`, `stat_io_time`, `statements_jit`), чтобы `pgcenter record` их собирал,
а `pgcenter report` — проигрывал. Эта задача добавляет CLI-поверхность в `cmd/report/report.go`:
флаги выбора отчёта для новых экранов и их проводку через `selectReport`, чтобы пользователь
мог запросить новые типы отчётов из командной строки.

Это атомарная единица работы из Wave 1 — она касается только `cmd/report/report.go` и его теста,
непересекающихся с файлами задач 01 и 02, поэтому может выполняться параллельно. Никаких изменений
в движке record/report не требуется: новые report-типы просто маппятся в строки, которые уже
являются ключами `view.New()` и `describeReport`.

## What to do

1. В struct `options` (report.go:15-38) добавить три поля выбора отчёта:
   - `showBgwriter bool` (флаг `-B`),
   - `showReplSlots bool` (флаг `-L`),
   - `showStatIO string` (флаг `-J`, значения `c`/`t`).
   JIT переиспользует существующее поле `showStatements string` (значение `j`) — новое поле не нужно.
2. В `init()` (report.go:59-82) зарегистрировать три новых флага рядом с существующими флагами
   выбора отчёта: `-B`/`--bgwriter` (bool), `-L`/`--replslots` (bool), `-J`/`--io` (string `c|t`).
   Короткие буквы `B`, `L`, `J` подтверждены свободными (code-research §4).
3. В `selectReport()` (report.go:126-184) добавить кейсы:
   - `case opts.showBgwriter:` → `"bgwriter"`,
   - `case opts.showReplSlots:` → `"replslots"`,
   - `case opts.showStatIO != "":` с внутренним switch `"c"` → `"stat_io"`, `"t"` → `"stat_io_time"`,
   - расширить существующий switch по `opts.showStatements` (report.go:152-165) кейсом
     `case "j":` → `"statements_jit"`.
   Невалидные значения `-J`/`-X` должны (как и сейчас для строковых флагов) проваливаться во
   внешний `return ""` — путь ошибки "report type is not specified".
4. В `Test_selectReport` (report_test.go:34-66) добавить тест-кейсы для всех пяти валидных форм
   плюс кейс с невалидным значением (`-J x` / `-X z`), проверяющий резолв в пустую строку.

## TDD Anchor

Тесты пишем/расширяем ДО реализации: добавить кейсы в существующий table-driven
`Test_selectReport`, убедиться что падают, затем дописать поля/флаги/кейсы и убедиться что проходят.

- `cmd/report/report_test.go::Test_selectReport` (`options{showBgwriter: true}` → `"bgwriter"`)
- `cmd/report/report_test.go::Test_selectReport` (`options{showReplSlots: true}` → `"replslots"`)
- `cmd/report/report_test.go::Test_selectReport` (`options{showStatIO: "c"}` → `"stat_io"`)
- `cmd/report/report_test.go::Test_selectReport` (`options{showStatIO: "t"}` → `"stat_io_time"`)
- `cmd/report/report_test.go::Test_selectReport` (`options{showStatements: "j"}` → `"statements_jit"`)
- `cmd/report/report_test.go::Test_selectReport` (`options{showStatIO: "x"}` → `""` — невалидное значение, путь ошибки)
- `cmd/report/report_test.go::Test_selectReport` (`options{showStatements: "z"}` → `""` — невалидное значение, путь ошибки)

## Acceptance Criteria

- [ ] В struct `options` добавлены поля `showBgwriter bool`, `showReplSlots bool`, `showStatIO string`.
- [ ] В `init()` зарегистрированы флаги `-B`/`--bgwriter`, `-L`/`--replslots`, `-J`/`--io` с осмысленными usage-строками.
- [ ] `selectReport` возвращает `"bgwriter"` для `-B`, `"replslots"` для `-L`, `"stat_io"` для `-J c`, `"stat_io_time"` для `-J t`, `"statements_jit"` для `-X j`.
- [ ] Невалидные значения `-J`/`-X` резолвятся в `""` (путь ошибки в `validate()`).
- [ ] `Test_selectReport` содержит кейсы для всех пяти валидных форм плюс минимум один невалидный кейс на `-J` и один на `-X`.
- [ ] `go test ./cmd/report/...` зелёный.

## Context Files

**Feature artifacts:**
- [008-feat-record-report-0-11-views.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views.md) — user-spec
- [008-feat-record-report-0-11-views-tech-spec.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-tech-spec.md) — tech-spec (Task 3, §Architecture, Decision 2)
- [008-feat-record-report-0-11-views-decisions.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-decisions.md) — decisions log
- [008-feat-record-report-0-11-views-code-research.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-code-research.md) — §4 (точные изменения CLI), §5e (тест-кейсы)

**Project knowledge:**
- [project.md](.claude/skills/project-knowledge/overview.md) — overview (фичи, поддерживаемые stats)
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, поток данных, версии PG
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — код-паттерны, testing-конвенции, version branching

**Code files:**
- [cmd/report/report.go](cmd/report/report.go) — изменить: struct `options` (15-38), `init()` (59-82), `selectReport()` (126-184)
- [cmd/report/report_test.go](cmd/report/report_test.go) — изменить: `Test_selectReport` (34-66)
- [report/report.go](report/report.go) — читать: ключи `describeReport`/`view.New()`, с которыми должны совпадать возвращаемые строки

## Verification Steps

- Запустить `go test ./cmd/report/...` — все тесты зелёные, включая новые кейсы `Test_selectReport`.
- Проверить, что новые тест-кейсы реально покрывают пять валидных форм и невалидные `-J`/`-X` (резолв в `""`).
- Опционально: `make lint` — без новых замечаний по `cmd/report/report.go`.

## Details

**Files:**
- `cmd/report/report.go`:
  - struct `options` (report.go:15-38) — поля выбора отчёта идут блоком (`showActivity` … `showProcPidStat`).
    Добавить три новых поля выбора рядом с ними, сохранив стиль выровненных комментариев.
    Текущее состояние: одно поле на report-family; `showStatements string` уже существует (значения m/g/i/t/l/w) и переиспользуется для `j`.
  - `init()` (report.go:59-82) — регистрация флагов. Bool-флаги через `BoolVarP`, string — через `StringVarP`
    (см. `showWAL`/`-W` как образец bool-флага и `showStatements`/`-X`, `showProgress`/`-P` как образец string-субселектора).
    Рекомендованные usage-строки (code-research §4b):
    `--bgwriter`/`-B` → "show pg_stat_bgwriter / pg_stat_checkpointer report";
    `--replslots`/`-L` → "show pg_replication_slots / pg_stat_replication_slots report";
    `--io`/`-J` → "show pg_stat_io report (c - count, t - time)".
  - `selectReport()` (report.go:126-184) — большой `switch{}`. bool-кейсы (`opts.showBgwriter`,
    `opts.showReplSlots`) добавить как простые `case ...: return "..."`. Для `-J` — `case opts.showStatIO != "":`
    с внутренним switch (образец — `opts.showDatabases`/`opts.showProgress`). Для JIT — добавить
    `case "j": return "statements_jit"` в уже существующий switch по `opts.showStatements` (152-165).
    `validate()` (85-123) менять НЕ нужно — она уже делегирует в `selectReport` и пробрасывает `ReportType`.

- `cmd/report/report_test.go`:
  - `Test_selectReport` (34-66) — table-driven `[]struct{ opts options; want string }`. Добавить 5 валидных
    строк и невалидные строки (`showStatIO: "x"` → `""`, `showStatements: "z"` → `""`).
    Кейс `{opts: options{}, want: ""}` уже есть (строка 60) — оставить.

**Dependencies:** нет зависимостей от других задач (depends_on пустой). Возвращаемые строки должны
точно совпадать с ключами `view.New()` и `describeReport` (`bgwriter`, `replslots`, `stat_io`,
`stat_io_time`, `statements_jit`) — эти ключи уже существуют в кодовой базе.

**Edge cases:**
- Невалидное значение `-J` (например `-J x`) → внутренний switch не матчит → внешний `switch` тоже
  не возвращает → `return ""` → `validate()` отдаёт ошибку "report type is not specified". Это
  желаемое поведение, идентичное существующим строковым флагам (`-D`, `-X`, `-P`).
- `statements_jit` без pg_stat_statements не записывается на record-уровне (pgss-gate), но это вне
  скоупа CLI — `selectReport` всё равно корректно отдаёт `"statements_jit"` для `-X j`.

**Implementation hints:**
- Короткие буквы `B`, `L`, `J` подтверждены свободными (code-research §4: занятые —
  `d,A,R,T,I,S,F,W,D,X,P,N,f,s,e,o,g,l,t`).
- Минимальное изменение: только три новых поля, три `…VarP` вызова и кейсы в одном `switch`.
  Не трогать `validate()`, не рефакторить соседний код.
- Порядок кейсов в `switch` selectReport не критичен (взаимоисключающие булевы/строковые поля),
  но держать рядом с однотипными для читаемости.

## Reviewers

- **dev-code-reviewer** → `docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-task-03-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-task-03-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [008-feat-record-report-0-11-views-decisions.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
