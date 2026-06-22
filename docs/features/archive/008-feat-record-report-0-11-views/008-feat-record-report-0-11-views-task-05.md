---
status: done                       # planned -> in_progress -> done
depends_on: ["01"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 2                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 05: replslots replay golden test

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

После того как Task 01 снял `NotRecordable: true` с view `replslots`, отчёт по нему
прогоняется через существующий report-движок без изменений. Эта задача добавляет
golden-based replay-тест, который подтверждает, что `report` корректно воспроизводит
записанные кумулятивные снимки `replslots` и считает диффы.

Тест строится по образцу `Test_app_doReport_procpidstat` (report/report_test.go:604): мы
синтезируем in-memory tar из двух кумулятивных тиков (meta + replslots на тик), скармливаем
его `app.doReport(tar.NewReader(...))` и сравниваем вывод с golden-файлом.

Селектор replslots версионно-независим: `query.SelectStatReplicationSlotsQuery(_ int)`
(internal/query/replication_slots.go:39) всегда возвращает `(PgStatReplicationSlots, 15,
[2]int{6,13})`. View-конфиг: `Ncols 15`, `DiffIntvl [6,13]`, `OrderKey 4` (retained,KiB,
desc), `OrderDesc true`, `UniqueKey` не задан → дефолт 0 = `slot_name`. Поэтому достаточно
одной версии (PG14), второй вариант не нужен.

Тест должен доказать три свойства, выделенные в Decision 3 / tech-debt [007]:
1. Кумулятивные счётчики в диффуемом блоке (колонки 6–13) вычитаются корректно.
2. Строки сопоставляются между снимками по идентичности `slot_name` (UniqueKey col 0).
3. Порядок сортировки по `retained,KiB DESC` сохраняется.
А также важный пограничный случай: физический слот, у которого все 8 диффуемых счётчиков
записаны как coalesced `"0"` (LEFT JOIN c `pg_stat_replication_slots` даёт NULL → recorder
хранит `"0"`), диффится в чистый ноль и не «обнуляет» экран — это поведенческая половина
контракта coalesce, которую закрывает feature.

Отдельно тестируется zero-slots / пустой архив: tar, в котором нет ни одной строки
replslots, должен печатать только заголовок (Decision 5 — пустой результат это нормальное
состояние, без INFO/WARNING).

Тест и его golden-файлы лежат в отдельном новом `_test.go` файле и отдельных golden-файлах,
чтобы per-screen задачи Wave 2 не конфликтовали при параллельном выполнении.

## What to do

1. Создать новый файл `report/report_record_replslots_test.go` (package `report`).
2. Написать replay-тест `Test_app_doReport_replslots`:
   - Собрать meta-`stat.PGresult` по форме `SelectCommonProperties` (7 колонок; readMeta
     читает только col 1 = version_num). Версия — PG14 (`"140009"`).
   - Собрать два кумулятивных снимка `replslots` как `stat.PGresult` с `Ncols: 15` и
     колонками по layout из `internal/query/replication_slots.go` (0 slot_name, 1 slot_type,
     2 active, 3 wal_status, 4 retained,KiB, 5 safe,KiB, 6 spill_txns, 7 spill_count,
     8 spill,KiB, 9 stream_txns, 10 stream_count, 11 stream,KiB, 12 total_txns, 13 total,KiB,
     14 stats_age).
   - Включить как минимум два слота с одинаковыми `slot_name` в обоих снимках (для проверки
     сопоставления по UniqueKey): один логический слот с растущими счётчиками 6–13, и один
     физический слот, у которого все 8 диффуемых счётчиков (6–13) записаны как `"0"` в обоих
     снимках (проверка чистого нулевого дельта-кейса).
   - Подобрать значения `retained,KiB` (col 4) так, чтобы у слотов был разный объём и был
     виден детерминированный порядок DESC.
   - Замаршалить через `json.Marshal`, собрать tar (две пары записей meta + replslots по
     образцу `writeEntry` из существующего теста; имена файлов вида
     `meta.YYYYMMDDThhmmss.mmm.json` и `replslots.<same-ts>.json`).
   - Прогнать `app.doReport`, сравнить с golden через флаг `-update` (как в
     `Test_app_doReport`).
3. Написать тест пустого/нулевого случая `Test_app_doReport_replslots_empty`:
   - tar с двумя тиками meta + replslots, где `Nrows: 0` (нет строк) в обоих снимках.
   - Утверждать, что вывод — только заголовок (timestamp/колонки), без строк данных и без
     INFO/WARNING. Можно проверить через отдельный golden header-only или ассертами:
     заголовок присутствует, ни одного имени слота нет.
4. Сгенерировать golden-файлы один раз: `go test ./report/... -run ReplSlots -update`, затем
   перепроверить без `-update`.
5. Проверить, что общий прогон `go test ./report/...` зелёный (не сломаны соседние тесты).

## TDD Anchor

Тесты пишем ДО генерации goldens, прогоняем с `-update` для фиксации эталона, затем без флага
проверяем, что вывод стабилен.

- `report/report_record_replslots_test.go::Test_app_doReport_replslots` — два кумулятивных
  тика replslots через `doReport` → дифф колонок 6–13 корректен; строки сопоставлены по
  `slot_name` (UniqueKey 0); физический слот с coalesced `"0"` даёт чистый нулевой дельта;
  порядок по `retained,KiB DESC` сохранён; вывод совпадает с golden.
- `report/report_record_replslots_test.go::Test_app_doReport_replslots_empty` — архив без
  строк replslots печатает только заголовок, без строк данных и без INFO/WARNING.

## Acceptance Criteria

- [ ] Новый файл `report/report_record_replslots_test.go` создан, в package `report`.
- [ ] `Test_app_doReport_replslots` строит синтетический in-memory tar из 2 кумулятивных
      тиков (meta + replslots), прогоняет `app.doReport`, сравнивает с golden.
- [ ] Тест содержит физический слот с 8 coalesced `"0"` диффуемыми счётчиками — дельта чистый
      ноль, экран не обнуляется.
- [ ] Тест подтверждает сопоставление строк между снимками по `slot_name` (UniqueKey col 0) и
      порядок по `retained,KiB DESC`.
- [ ] Добавлен zero-slots / пустой-архив кейс, который печатает только заголовок (без
      INFO/WARNING).
- [ ] Golden-файлы лежат в `report/testdata/` с уникальными именами (например
      `report_record_replslots.golden`), не пересекаются с другими задачами.
- [ ] `go test ./report/... -run ReplSlots` зелёный; `go test ./report/...` без регрессий.

## Context Files

**Feature artifacts:**
- [008-feat-record-report-0-11-views.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views.md) — user-spec
- [008-feat-record-report-0-11-views-tech-spec.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-tech-spec.md) — tech-spec (Task 5, Decision 3/5/6)
- [008-feat-record-report-0-11-views-decisions.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-decisions.md) — decisions log
- [008-feat-record-report-0-11-views-code-research.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-code-research.md) — §8 (Configure consumes Version only), §[007] (zero-cell diff)

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — overview/features
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow, PG version handling
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — testing conventions, golden-file pattern

**Code files:**
- [report/report_record_replslots_test.go](report/report_record_replslots_test.go) — НОВЫЙ файл с тестом
- [report/testdata/](report/testdata/) — НОВЫЕ golden-фикстуры
- [report/report_test.go](report/report_test.go) — образец: `Test_app_doReport_procpidstat` (синтетический tar, строки 604-716), флаг `-update` (строка 22), `Test_app_doReport` (golden-сравнение)
- [internal/query/replication_slots.go](internal/query/replication_slots.go) — layout колонок и селектор
- [internal/view/view.go](internal/view/view.go) — `replslots` view-конфиг (строки 153-165)

## Verification Steps

- `go test ./report/... -run ReplSlots` — оба теста зелёные (golden replay + empty case).
- `go test ./report/...` — нет регрессий в соседних тестах.
- Убедиться, что в выводе golden видны: дельты колонок 6–13, физический слот с нулевыми
  дельтами, порядок строк по retained,KiB DESC.

## Details

<!-- All details for task execution — technical, organizational, any other. -->

**Files:**
- `report/report_record_replslots_test.go` — новый тест-файл. Зеркалит структуру
  `Test_app_doReport_procpidstat`: вспомогательная `writeEntry(name, payload)` для tar,
  `json.Marshal` каждого `stat.PGresult`, два тика, `tar.NewReader(&tarBuf)`,
  `app := newApp(config)`, `app.writer = &buf`, `app.doReport(tr)`, сравнение с golden под
  флагом `-update` (`*update` уже объявлен в report_test.go того же пакета — НЕ объявлять
  повторно).
- `report/testdata/report_record_replslots.golden` (и при необходимости
  `report/testdata/report_record_replslots_empty.golden`) — генерируются через `-update`.

**Replslots column layout (0-based, Ncols=15)** из internal/query/replication_slots.go:
0 slot_name, 1 slot_type, 2 active, 3 wal_status, 4 `retained,KiB`, 5 `safe,KiB`,
6 spill_txns, 7 spill_count, 8 `spill,KiB`, 9 stream_txns, 10 stream_count, 11 `stream,KiB`,
12 total_txns, 13 `total,KiB`, 14 stats_age.

**View config (internal/view/view.go:153-165):** `DiffIntvl [2]int{6,13}` (диффуются колонки
6..13 включительно), `OrderKey 4`, `OrderDesc true`, `UniqueKey` не задан → 0 (`slot_name`),
`MinRequiredVersion query.PostgresV14`.

**Dependencies:**
- Task 01 (depends_on) — снимает `NotRecordable: true` с `replslots`. До этого view всё равно
  присутствует в `view.New()`, так что тест компилируется и проходит независимо (report-путь
  читает записанный JSON, а не фильтрует view). Зависимость нужна логически — фича включает
  запись; тест проверяет воспроизведение.
- Внешних пакетов нет. Используем `archive/tar`, `bytes`, `encoding/json`, `os`, `testing`,
  `time`, `database/sql`, `github.com/lesovsky/pgcenter/internal/stat`, testify/assert — всё
  уже в report_test.go.

**Edge cases:**
- Первый тик — это prev-снимок (processData: `!prevStat.Valid → continue`), он не печатается;
  второй становится curr и даёт строки. Оба снимка должны нести ОДИНАКОВЫЕ `slot_name`, иначе
  строки не спарятся по UniqueKey 0.
- Физический слот: все колонки 6–13 = `"0"` в ОБОИХ снимках → дельта `"0"`, без аборта диффа
  (диффуемый блок не должен содержать пустых строк — иначе ParseInt("") уронит сэмпл).
- `retained,KiB` (col 4) — абсолютная, не диффуемая (вне [6,13]), проходит passthrough; задать
  разные значения для детерминированного DESC-порядка.
- Имена tar-файлов: timestamp в имени meta и replslots в рамках одного тика должен совпадать
  (readTar группирует по тику); формат `20060102T150405.000` (см. recorder.newFilenameString).
- `Config.TsStart/TsEnd` должны охватывать timestamps записей, иначе isFilenameTimestampOK
  отфильтрует их.

**Implementation hints:**
- Имена тестов содержат подстроку `ReplSlots`, чтобы `-run ReplSlots` ловил оба.
- Для empty-кейса можно либо отдельный golden, либо ассерты `assert.Regexp` на заголовок +
  `assert.NotContains` на имена слотов — выбрать в соответствии с test-master (предпочтительно
  golden для консистентности с остальными replay-тестами).
- Не вызывать живой PostgreSQL и не исполнять SQL: report только реплеит записанный JSON,
  `Configure(Options{Version})` строит layout, но саму SQL-строку не исполняет.
- Запустить генерацию goldens один раз с `-update`, затем зафиксировать и прогнать без флага.

## Reviewers

- **dev-code-reviewer** → `docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-task-05-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-task-05-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [008-feat-record-report-0-11-views-decisions.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
