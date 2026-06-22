---
status: done                       # planned -> in_progress -> done
depends_on: ["01"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 2                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 07: statements_jit replay golden tests

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Добавить golden-based replay-тест для отчёта `statements_jit` (`pgcenter report -X j`), который
проверяет, что движок воспроизведения корректно диффит записанные кумулятивные сэмплы JIT-метрик и
применяет версионно-зависимую раскладку колонок.

Экран `statements_jit` — version-aware: `query.SelectStatStatementsJITQuery(version)` возвращает
4-tuple `(QueryTmpl, Ncols, DiffIntvl, UniqueKey)`. На PG17+ pg_stat_statements добавляет колонки
`jit_deform_time`/`jit_deform_count`, что сдвигает блок интервальных колонок, `functions`, `queryid`
и `query` вправо на одну позицию. Поэтому раскладка меняется по версии:
- **PG15** (`version < 170000`): 13 колонок, `DiffIntvl={6,10}`, `UniqueKey=11`.
- **PG17+** (`version >= 170000`): 15 колонок, `DiffIntvl={7,12}`, `UniqueKey=13`.

Замыкающий md5 `queryid` (по которому строки матчатся между сэмплами) — это и есть `UniqueKey`, его
индекс сдвигается с числом колонок. На report-времени раскладка выбирается в
`view.Configure(Options{Version})`, который `processData` вызывает на первом сэмпле (и при смене
версии) — этот тест прямо проверяет этот переключатель через meta-запись с целевой версией.

Тест построен по образцу `Test_app_doReport_procpidstat` (синтетический in-memory tar, два
кумулятивных тика + meta), но живёт в **отдельном новом файле** `report/report_record_statements_jit_test.go`
со **своими golden-фикстурами** — это держит per-screen задачи Wave 2 бесконфликтными при
параллельном выполнении. Golden-варианты на записанной версии 15 и 17 покрывают изменение числа
колонок.

## What to do

- Создать новый файл `report/report_record_statements_jit_test.go` (package `report`).
- Реализовать table-driven тест с двумя вариантами по версии: записанный `version_num` = `150000`
  (golden 13-колоночной раскладки) и `170000` (golden 15-колоночной раскладки). Имя теста должно
  ловиться `-run StatementsJIT` (например, `Test_app_doReport_statements_jit`).
- Для каждого варианта собрать `stat.PGresult` руками с правильным набором колонок для версии
  (13 либо 15), две строки с одинаковым md5 `queryid` (чтобы `diff()` спарил их по `UniqueKey`),
  и двумя кумулятивными тиками (значения phase-time во втором тике больше → должны дать положительную
  дельту в интервальных `*,ms`-колонках внутри `DiffIntvl`).
- Собрать синтетический in-memory tar: на каждый тик записать `meta.{TS}.json` (7-колоночная форма
  `SelectCommonProperties`, `version_num` в колонке 1 = целевая версия) и `statements_jit.{TS}.json`
  (сериализованный `stat.PGresult`). Порядок записи в tar — как у `tarRecorder.write()`.
- Прогнать через `app.doReport(tr)` с `Config{ReportType: "statements_jit", ...}`, перехватить вывод
  в `bytes.Buffer` через `app.writer`.
- Сравнить вывод с golden-файлом; поддержать флаг `-update` (тот же `update`-флаг, что уже объявлен в
  `report_test.go`) для регенерации goldens.
- Сгенерировать golden-фикстуры в `report/testdata/` (запустить тест с `-update`, затем глазами
  проверить содержимое — корректные дельты, правильное число колонок на версию).
- Дополнительно (явными ассертами помимо golden) зафиксировать: интервальные `*,ms`-дельты
  посчитаны как `curr - prev`; `queryid` строки присутствует в выводе (строки спарились по UniqueKey);
  на PG17-варианте присутствует колонка `deform`, на PG15-варианте — нет.

## TDD Anchor

Это тестовая задача — артефакт и есть тест. Пишем тест → запускаем без golden (или с `-update`) →
проверяем падение/осмысленность вывода → фиксируем golden → тест зелёный.

- `report/report_record_statements_jit_test.go::Test_app_doReport_statements_jit/version_15` —
  replay двух кумулятивных тиков 13-колоночного результата при `version_num=150000`: вывод
  совпадает с `testdata/report_statements_jit_v15.golden`; интервальные `*,ms`-дельты = `curr-prev`;
  колонки `deform*` отсутствуют.
- `report/report_record_statements_jit_test.go::Test_app_doReport_statements_jit/version_17` —
  replay двух кумулятивных тиков 15-колоночного результата при `version_num=170000`: вывод
  совпадает с `testdata/report_statements_jit_v17.golden`; интервальные `*,ms`-дельты = `curr-prev`;
  колонка `deform` присутствует; строки спарены по сдвинутому `UniqueKey` (queryid в выводе).

## Acceptance Criteria

- [ ] Новый файл `report/report_record_statements_jit_test.go` (package `report`), имя теста ловится `go test ./report/... -run StatementsJIT`.
- [ ] Тест запускается без живого PostgreSQL и без чтения легаси-фикстуры `pgcenter.stat.golden.tar`.
- [ ] Два версионных варианта (15 и 17) с собственными golden-фикстурами в `report/testdata/`.
- [ ] Версионно-корректная раскладка применена на report-времени через `Configure(Options{Version})`: 13 колонок/`{6,10}`/`UniqueKey 11` для v15, 15 колонок/`{7,12}`/`UniqueKey 13` для v17.
- [ ] Интервальные phase-time `*,ms`-колонки (внутри `DiffIntvl`) диффятся как `curr - prev`; строки спарены по версионно-сдвинутому замыкающему md5 `queryid` (`UniqueKey`).
- [ ] Поддержан `-update` для регенерации goldens; сгенерированные goldens просмотрены и осмысленны.
- [ ] `go test ./report/... -run StatementsJIT` зелёный; `make test` без регрессий.

## Context Files

**Feature artifacts:**
- [008-feat-record-report-0-11-views.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views.md) — user-spec
- [008-feat-record-report-0-11-views-tech-spec.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-tech-spec.md) — tech-spec (Task 7, Decision 3, §Testing Strategy)
- [008-feat-record-report-0-11-views-code-research.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-code-research.md) — §8 (report-time Configure / version-aware selectors), §7 (harness pattern)
- [008-feat-record-report-0-11-views-decisions.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-decisions.md) — decisions log

**Project knowledge:**
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — PostgreSQL Version Handling, Testing (port map, синтетические тесты)
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — Version-Specific Query Pattern (4-tuple selector с UniqueKey, сдвиг queryid)

**Code files:**
- [report/report_record_statements_jit_test.go](report/report_record_statements_jit_test.go) — НОВЫЙ файл с тестом
- [report/testdata/](report/testdata/) — НОВЫЕ golden-фикстуры (`report_statements_jit_v15.golden`, `report_statements_jit_v17.golden`)
- [report/report_test.go](report/report_test.go) — образец: `Test_app_doReport_procpidstat` (синтетический tar + doReport), флаг `update`, хелпер `writeEntry`
- [internal/query/statements.go](internal/query/statements.go) — `SelectStatStatementsJITQuery`, константы `PgStatStatementsJITPG15` / `PgStatStatementsJITDefault` (точный список колонок на версию)

## Verification Steps

- Запустить `go test ./report/... -run StatementsJIT -v` — оба подтеста (version_15, version_17) зелёные.
- Регенерация: `go test ./report/... -run StatementsJIT -update`, затем просмотреть оба golden-файла —
  убедиться, что число колонок соответствует версии (13 vs 15), интервальные дельты = `curr-prev`,
  колонка `deform` есть только в v17.
- Запустить `make test` — нет регрессий в пакете `report` и в целом.

## Details

**Files:**
- `report/report_record_statements_jit_test.go` — новый тест-файл, package `report`. Build синтетический
  tar (2 тика meta + statements_jit), `app.doReport`, сравнение с golden, `-update`-поддержка, плюс
  явные ассерты на дельты/наличие колонок.
- `report/testdata/report_statements_jit_v15.golden`, `report/testdata/report_statements_jit_v17.golden` —
  новые golden-фикстуры, сгенерированные через `-update`.

**Dependencies:**
- Task 01 (depends_on) — снимает `NotRecordable` с `statements_jit` в `view.New()`. Без этого
  `views["statements_jit"]` остаётся, но раскладка/recordable-семантика выровнена именно в Task 01;
  тест должен запускаться против пост-Task-01 состояния.
- Использует существующее: `stat.PGresult`, `report.Config`, `report.newApp`, `app.doReport`,
  `view.New()`/`Configure`, флаг `update` из `report_test.go` (НЕ объявлять повторно — он уже есть
  в пакете).

**Колонки statements_jit по версии (из `internal/query/statements.go`):**
- PG15 (`PgStatStatementsJITPG15`, 13 колонок), индексы:
  `0 user, 1 database, 2 gen_total, 3 inline_total, 4 opt_total, 5 emit_total,`
  `6 gen,ms, 7 inline,ms, 8 opt,ms, 9 emit,ms, 10 functions, 11 queryid, 12 query`.
  `DiffIntvl={6,10}` (интервальные `*,ms` + functions), `UniqueKey=11` (queryid).
- PG17+ (`PgStatStatementsJITDefault`, 15 колонок) — добавлены `deform_total` после `emit_total` и
  `deform,ms` в блоке `*,ms`, индексы:
  `0 user, 1 database, 2 gen_total, 3 inline_total, 4 opt_total, 5 emit_total, 6 deform_total,`
  `7 gen,ms, 8 inline,ms, 9 opt,ms, 10 emit,ms, 11 deform,ms, 12 functions, 13 queryid, 14 query`.
  `DiffIntvl={7,12}`, `UniqueKey=13` (queryid).

**Edge cases:**
- Первый тик `processData` отбрасывает (first-snapshot rule: `!prevStat.Valid → continue`), вывод
  даёт второй тик — значения phase-time во втором тике должны быть строго больше первого, чтобы
  дельты были видимы и осмысленны.
- Обе строки в обоих тиках должны иметь **одинаковый** md5 `queryid` на позиции `UniqueKey`, иначе
  `diff()` не спарит их и дельта не посчитается.
- Колонки `*_total` (текстовые `HH:MM:SS`, вне `DiffIntvl`) проходят насквозь без диффа — их в
  golden видно как абсолютные значения второго тика.
- meta `version_num` (колонка индекс 1, форма `SelectCommonProperties`) — единственное, что управляет
  выбором раскладки на report-времени; больше ничего из `Options` селектор JIT не читает (см. §8).
- Rebuilt SQL в report не исполняется — частичные `Options{Version}` безопасны.

**Implementation hints:**
- Скопировать структуру из `Test_app_doReport_procpidstat` (report_test.go:604) — те же `writeEntry`,
  `tar.NewWriter`, marshaling `stat.PGresult` через `json.Marshal`, `newApp(config)` + `app.writer = &buf`.
- meta-фикстуру взять из того же теста (7-колоночная `SelectCommonProperties`-форма), подменив
  `version_num` в колонке 1 на `"150000"` / `"170000"`.
- Имя report-type должно быть строго `"statements_jit"` (ключ `view.New()`).
- Для golden-сравнения и `-update` повторить идиому из `Test_app_doReport` (читать/писать `wantFile`
  через `os.ReadFile`/`os.WriteFile`, ветвление по `*update`).
- НЕ редактировать `report_test.go` и общие goldens — только новый файл и новые фикстуры (parallel-safety).

## Reviewers

- **dev-code-reviewer** → `docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-task-07-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-task-07-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [008-feat-record-report-0-11-views-decisions.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
