---
status: done                       # planned -> in_progress -> done
depends_on: ["01"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 2                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 04: bgwriter replay golden tests

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

После Task 01 (снят `NotRecordable: true` с view `bgwriter`) экран pg_stat_bgwriter
становится recordable и проходит через стандартный report-pipeline. Эта задача добавляет
golden-based replay тест, который доказывает, что воспроизведение записанного архива bgwriter
работает корректно на трёх версиях PostgreSQL с разной раскладкой колонок.

`bgwriter` — version-aware экран: `SelectStatBgwriterQuery(version)`
([internal/query/bgwriter.go:41](../../../internal/query/bgwriter.go)) возвращает три разные
раскладки `(query, Ncols, DiffIntvl)`:
- **PG14-16:** Ncols 12, DiffIntvl `[3,10]` (ckpt-счётчики 1-2 и stats_age 11 — абсолютные)
- **PG17:** Ncols 13, DiffIntvl `[6,11]` (ckpt/rstpt-счётчики 1-5 и stats_age 12 — абсолютные)
- **PG18+:** Ncols 14, DiffIntvl `[6,12]` (как PG17 плюс диффуемый `slru_written`)

В report этот переключатель срабатывает через `views.Configure(query.Options{Version})`
(report.go:250-252), вызываемый на первом sample по версии из meta-записи. Тест синтетически
кормит pipeline in-memory tar'ом с двумя кумулятивными тиками и meta-записью с нужной версией,
запускает `doReport` и сверяет вывод с golden-файлом. Это доказывает version-aware layout switch
без живого PostgreSQL.

Тест и golden-файлы — в **отдельном новом** файле и с **отдельными** golden'ами, чтобы Wave 2
выполнялась параллельно без конфликтов с другими per-screen задачами (Task 05/06/07).

## What to do

1. Создать новый файл `report/report_record_bgwriter_test.go` (package `report`).
2. Зеркалить harness из `Test_app_doReport_procpidstat`
   ([report/report_test.go:604](../../../report/report_test.go)): `tar.NewWriter` +
   замыкание `writeEntry` + meta-запись, несущая `version_num`, + `doReport` с `app.writer=&buf`.
3. Построить для каждой целевой версии хэндмейд `stat.PGresult` с раскладкой колонок bgwriter
   ровно по `SelectStatBgwriterQuery` (имена и порядок колонок — из
   [internal/query/bgwriter.go](../../../internal/query/bgwriter.go)):
   - **PG14:** 12 колонок, version_num `140009`
   - **PG17:** 13 колонок, version_num `170001`
   - **PG18:** 14 колонок, version_num `180000`
4. Для каждой версии собрать 2 кумулятивных тика: тик 1 (prev, отбрасывается pipeline'ом как
   первый snapshot) и тик 2 (curr) с бо́льшими кумулятивными значениями в диффуемых колонках,
   чтобы получить корректные дельты. Абсолютные/текстовые колонки (ckpt/rstpt event-счётчики
   вне DiffIntvl, `stats_age`) должны проходить насквозь без диффа.
5. Сложить tar: для каждого тика записать `meta.<TS>.json` + `bgwriter.<TS>.json` (имена
   файлов с timestamp-форматом как в существующем тесте), запустить `doReport` с
   `Config{ReportType: "bgwriter", ...}`.
6. Сгенерировать golden-файлы в `report/testdata/` — по одному на версию (например
   `report_record_bgwriter_pg14.golden`, `..._pg17.golden`, `..._pg18.golden`). Использовать
   golden-write механизм проекта (флаг обновления golden'ов — см. существующие golden-тесты в
   report_test.go) или зафиксировать ожидаемый вывод в golden-файле, затем сверять.
7. Ассертить: диффуемые кумулятивные колонки дают корректную дельту (curr − prev); абсолютные
   event-счётчики и `stats_age` отображаются как есть из тика 2; timestamp-заголовок присутствует.

## TDD Anchor

Тесты пишутся ПЕРВЫМИ, до golden-файлов. Сначала фиксируем структуру теста и ожидаемые
ассерты, прогоняем (падает — нет golden'а / неверная раскладка), затем фиксируем golden.

- `report/report_record_bgwriter_test.go::Test_app_doReport_bgwriter` (table-driven по версиям) —
  для каждой версии (14/17/18) синтетический tar из 2 тиков воспроизводится через `doReport`,
  вывод совпадает с соответствующим golden-файлом.
- Подкейс PG14 — раскладка 12 колонок, DiffIntvl `[3,10]`: диффуются `ckpt_write,ms`..`buf_alloc`,
  `ckpt_timed`/`ckpt_req` и `stats_age` — абсолютные.
- Подкейс PG17 — раскладка 13 колонок, DiffIntvl `[6,11]`: добавлены rstpt-колонки (абсолютные),
  диффуются `buf_ckpt`..`buf_alloc`.
- Подкейс PG18 — раскладка 14 колонок, DiffIntvl `[6,12]`: дополнительно диффуется `slru_written`.

## Acceptance Criteria

- [ ] Новый файл `report/report_record_bgwriter_test.go` существует, package `report`.
- [ ] Тест строит синтетический in-memory tar (2 тика + meta), без живого PostgreSQL.
- [ ] Покрыты три версии: 14, 17, 18 — каждая со своей раскладкой колонок и своим golden.
- [ ] Кумулятивные диффуемые колонки воспроизводятся как корректная дельта (curr − prev).
- [ ] Абсолютные колонки (ckpt/rstpt event-счётчики вне DiffIntvl, `stats_age`) проходят насквозь.
- [ ] Golden-файлы лежат в `report/testdata/` и не пересекаются с другими per-screen тестами.
- [ ] `go test ./report/... -run Bgwriter` — зелёный.

## Context Files

**Feature artifacts:**
- [008-feat-record-report-0-11-views.md](008-feat-record-report-0-11-views.md) — user-spec
- [008-feat-record-report-0-11-views-tech-spec.md](008-feat-record-report-0-11-views-tech-spec.md) — tech-spec (Task 4, Decision 3, §Testing Strategy)
- [008-feat-record-report-0-11-views-decisions.md](008-feat-record-report-0-11-views-decisions.md) — decisions log
- [008-feat-record-report-0-11-views-code-research.md](008-feat-record-report-0-11-views-code-research.md) — §6 (harness pattern), §8 (version-aware Configure)

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — project features/audience
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout, data flow, PG version handling
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — testing conventions, version branching

**Code files:**
- [report/report_test.go](../../../report/report_test.go) — harness-образец `Test_app_doReport_procpidstat` (строка 604) + golden-тесты
- [internal/query/bgwriter.go](../../../internal/query/bgwriter.go) — точная раскладка колонок и DiffIntvl на версию
- [report/report_record_bgwriter_test.go](../../../report/report_record_bgwriter_test.go) — НОВЫЙ файл (создать)
- `report/testdata/` — НОВЫЕ golden-файлы (создать)

## Verification Steps

- Шаг 1: `go test ./report/... -run Bgwriter` — все подкейсы (14/17/18) проходят.
- Шаг 2: убедиться, что golden-файлы созданы в `report/testdata/` и тест их читает.
- Шаг 3: `make test` — нет регрессий в пакете report.

## Details

**Files:**
- `report/report_record_bgwriter_test.go` (новый) — package `report`. Зеркалит структуру
  `Test_app_doReport_procpidstat` (report_test.go:604-716): импорт `archive/tar`, `bytes`,
  `database/sql`, `encoding/json`, `regexp`, `time`, `testing`; `testify/assert`; `stat`.
- `report/testdata/` (новые golden'ы) — по одному файлу на версию. Существующие golden'ы там
  уже лежат (например `report_wal.golden`) — следовать их именованию/механизму.

**bgwriter column layout (источник истины — internal/query/bgwriter.go):**
- **PG14 (12 cols)**, DiffIntvl `[3,10]`:
  `source, ckpt_timed, ckpt_req, "ckpt_write,ms", "ckpt_sync,ms", buf_ckpt, buf_clean, maxwritten,
  buf_backend, buf_backend_fsync, buf_alloc, stats_age`.
  Индексы 0..11; диффуются 3..10 (`ckpt_write,ms`..`buf_alloc`); абсолютные: 0 source,
  1-2 ckpt-счётчики, 11 stats_age.
- **PG17 (13 cols)**, DiffIntvl `[6,11]`:
  `source, ckpt_timed, ckpt_req, rstpt_timed, rstpt_req, rstpt_done, "ckpt_write,ms",
  "ckpt_sync,ms", buf_ckpt, buf_clean, maxwritten, buf_alloc, stats_age`.
  Диффуются 6..11; абсолютные: 0-5 (source + ckpt/rstpt-счётчики), 12 stats_age.
- **PG18 (14 cols)**, DiffIntvl `[6,12]`:
  как PG17 + `slru_written` после `buf_ckpt` (индекс 9), общий сдвиг хвоста на 1.
  Диффуются 6..12 (вкл. `slru_written`); абсолютные: 0-5, 13 stats_age.

**Harness mechanics (из report_test.go:604):**
- meta-запись: `stat.PGresult{Valid:true, Ncols:7, Nrows:1, Cols:[...], Values:[...]}` —
  `readMeta` читает только индекс 1 (`version_num`); там и задаётся целевая версия.
- per-tick layout: `meta.<TS>.json` + `bgwriter.<TS>.json` (2 тика). Timestamp в имени —
  формат `20060102T150405.000` (как `meta.20260519T100000.000.json` в образце).
- первый тик отбрасывается `processData` (правило `!prevStat.Valid -> continue`), вывод даёт
  второй тик.
- `Config{ReportType:"bgwriter", TruncLimit:32, TsStart, TsEnd}`, `app := newApp(config)`,
  `app.writer = &buf`, `tr := tar.NewReader(...)`, `app.doReport(tr)`.

**Dependencies:**
- Task 01 (Wave 1) — снимает `NotRecordable` с view `bgwriter`; зависимость зафиксирована для
  корректного порядка волн. В report `view.New()["bgwriter"]` уже существует и version-aware
  Configure уже работает, так что при чтении тест функционально устойчив.
- Никаких новых пакетов.

**Edge cases:**
- DiffIntvl границы: проверить, что колонка ровно на границе интервала (нижняя и верхняя)
  диффуется, а соседняя вне интервала — нет.
- stats_age — текстовая колонка (`date_trunc(...)::text`): должна пройти насквозь из тика 2,
  не подвергаться числовому диффу.
- slru_written появляется только на PG18 — на PG14/PG17 колонки нет; не путать индексы.

**Implementation hints:**
- Раскладку колонок и DiffIntvl брать строго из `SelectStatBgwriterQuery` — не выдумывать.
  Имена колонок в `Cols` — это алиасы из SQL (`ckpt_timed`, `buf_ckpt`, `"ckpt_write,ms"` и т.д.).
- Кумулятивные значения: тик 2 > тик 1 в диффуемых колонках, чтобы дельта была положительной и
  легко проверяемой в golden.
- Для генерации golden-файлов посмотреть, как это делают существующие golden-тесты в
  report_test.go (механизм записи/обновления golden), и переиспользовать его.
- table-driven подкейсы по версии — одна тест-функция, три записи (14/17/18), каждая со своим
  golden-путём.

## Reviewers

- **dev-code-reviewer** → `008-feat-record-report-0-11-views-task-04-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `008-feat-record-report-0-11-views-task-04-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [008-feat-record-report-0-11-views-decisions.md](008-feat-record-report-0-11-views-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
