---
status: done                       # planned -> in_progress -> done
depends_on: ["01"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 2                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 06: pg_stat_io replay golden tests (count + time)

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

После того как Task 01 снял `NotRecordable: true` с view `stat_io` и `stat_io_time`, эти отчёты
проходят через тот же report-replay-конвейер, что и `wal`/`tables`. Нужно покрыть их
golden-based replay-тестами, чтобы зафиксировать, что движок воспроизведения корректно
диффит записанные кумулятивные сэмплы и переключает version-aware layout.

`stat_io` — version-aware: `SelectStatIOQuery` ветвится на PG18 (на PG16/17 KiB-троугпут
выводится из `op_bytes`, на PG18 — из нативных счётчиков `read_bytes/write_bytes/extend_bytes`).
При этом **shape колонок одинаков на всех ветках**: `Ncols=16`, `DiffIntvl={4,14}`,
`UniqueKey=0` (синтетический `io_key = left(md5(backend_type||object||context),10)`). Разница
PG16↔PG18 проявляется только в данных записанного сэмпла (значения KiB-колонок и наличие
дополнительных строк `object='wal'`/`context='init'` на PG18), не в layout. Поэтому делаем golden
variants на recorded version 16 и 18 — чтобы доказать, что report-time `Configure(Options{Version})`
выбирает корректный селектор и что diff одинаково чист на обеих ветках.

`stat_io_time` — version-independent: `SelectStatIOTimeQuery` игнорирует версию (тайминги
стабильны на PG16-18), `Ncols=10`, `DiffIntvl={4,8}`, `UniqueKey=0`. Достаточно одной версии.

Тесты живут в **новом** файле `report/report_record_statio_test.go` с **собственными** golden-
фикстурами, чтобы per-screen задачи Wave 2 не пересекались по файлам и шли параллельно. Паттерн —
синтетический in-memory tar (2 кумулятивных тика + meta), как в существующем
`Test_app_doReport_procpidstat` (report/report_test.go:604).

## What to do

- Создать новый файл `report/report_record_statio_test.go` (package `report`), не трогая
  `report/report_test.go` и другие существующие test-файлы.
- Написать replay-тест(ы) для `stat_io` с golden-вариантами при recorded version 16 и 18:
  - собрать синтетический tar с двумя тиками; каждый тик содержит `meta.<TS>.json` (версия
    160000 / 180000 соответственно) и `stat_io.<TS>.json` с hand-built `stat.PGresult`;
  - `stat.PGresult`: `Ncols=16`, `Cols` в порядке селектора из `internal/query/io.go`
    (`io_key, backend_type, object, context, reads, "read,KiB", writes, "write,KiB", extends,
    "ext,KiB", hits, evictions, writebacks, reuses, fsyncs, stats_age`);
  - значения diffed-блока (cols 4-14) — кумулятивные: тик 1 меньше тика 2; `io_key` (col 0)
    одинаков в обоих тиках, чтобы строки спарились по `UniqueKey=0`;
  - часть diffed-ячеек выставить в `"0"` (имитируя `coalesce(...,0)` для NULL-колонок
    pg_stat_io вроде `fsyncs` у temp / `reads` у bgwriter) — убедиться, что они дают чистую
    нулевую дельту и не блэнкают строку;
  - запустить `doReport` против tar, сравнить вывод с golden-файлом (через флаг `-update`,
    как остальные report-тесты).
- Написать replay-тест для `stat_io_time` (одна версия, напр. 160000): синтетический tar с
  двумя тиками, `stat.PGresult` `Ncols=10`, `Cols` в порядке `SelectStatIOTimeQuery`
  (`io_key, backend_type, object, context, read_time, write_time, writeback_time, extend_time,
  fsync_time, stats_age`), diffed-блок cols 4-8 кумулятивный, `io_key` стабилен.
- Сгенерировать новые golden-фикстуры в `report/testdata/` (отдельные имена, напр.
  `report_record_stat_io_v16.golden`, `report_record_stat_io_v18.golden`,
  `report_record_stat_io_time.golden`) запуском теста с `-update`, затем перепроверить
  стабильность повторным прогоном без `-update`.
- Имена тестовых функций должны попадать под `-run StatIO` (verify-команда), напр.
  `Test_app_doReport_StatIO_v16`, `Test_app_doReport_StatIO_v18`, `Test_app_doReport_StatIOTime`.

## TDD Anchor

Тесты ЯВЛЯЮТСЯ артефактом этой задачи (это test-only задача). Сначала пишем тест с пустым/
отсутствующим golden → запускаем с `-update` → фиксируем golden → повторный прогон без `-update`
должен проходить детерминированно.

- `report/report_record_statio_test.go::Test_app_doReport_StatIO_v16` — replay stat_io при
  recorded version 16: строки спариваются по `io_key` (UniqueKey 0), diffed-счётчики (cols 4-14)
  дают корректные дельты, coalesced-ячейки `"0"` → чистая нулевая дельта; вывод == golden v16.
- `report/report_record_statio_test.go::Test_app_doReport_StatIO_v18` — то же при version 18
  (нативная KiB-деривация); layout идентичен v16, diff корректен; вывод == golden v18.
- `report/report_record_statio_test.go::Test_app_doReport_StatIOTime` — replay stat_io_time
  (version-independent): diffed timing-блок (cols 4-8) корректен, строки матчатся по io_key;
  вывод == golden.

## Acceptance Criteria

- [ ] Новый файл `report/report_record_statio_test.go` создан; существующие test-файлы не тронуты.
- [ ] Есть golden-варианты stat_io для recorded version 16 и 18; layout (16 cols, DiffIntvl {4,14},
      UniqueKey 0) проверяется через реальный `doReport`-replay.
- [ ] Есть golden для stat_io_time (10 cols, DiffIntvl {4,8}, UniqueKey 0), одна версия.
- [ ] Строки спариваются по синтетическому `io_key` (UniqueKey col 0); кумулятивные счётчики
      диффятся корректно; coalesced-ячейки `"0"` дают чистые нулевые дельты без блэнка строки.
- [ ] Golden-фикстуры лежат в `report/testdata/` под уникальными именами (не пересекаются с
      существующими и с другими Wave 2 задачами).
- [ ] `go test ./report/... -run StatIO` зелёный и детерминирован (повторный прогон без `-update`
      проходит).

## Context Files

**Feature artifacts:**
- [008-feat-record-report-0-11-views.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views.md) — user-spec
- [008-feat-record-report-0-11-views-tech-spec.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-tech-spec.md) — tech-spec (Task 6, Decision 3)
- [008-feat-record-report-0-11-views-decisions.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-decisions.md) — decisions log
- [008-feat-record-report-0-11-views-code-research.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-code-research.md) — §7 (harness pattern), §8 (Configure version-aware), §10 (debt [007] io_key matching)

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md)
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow, PG version handling
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — testing conventions, golden-file / version-branching patterns

**Code files:**
- [report/report_record_statio_test.go](report/report_record_statio_test.go) — NEW: replay-тесты stat_io (v16/v18) + stat_io_time
- [report/testdata/](report/testdata/) — NEW golden-фикстуры
- [report/report_test.go](report/report_test.go) — READ: эталон `Test_app_doReport_procpidstat` (стр. 604) — синтетический tar, meta-result, `writeEntry`, `-update`
- [internal/query/io.go](internal/query/io.go) — READ: точный порядок колонок и DiffIntvl/Ncols для stat_io (PG16/PG18) и stat_io_time

## Verification Steps

- `go test ./report/... -run StatIO` — все StatIO replay-тесты проходят.
- Повторный прогон БЕЗ `-update` — тесты остаются зелёными (golden детерминирован).
- `go test ./report/...` — нет регрессий в существующих report-тестах.
- `make lint` — новый test-файл проходит линтер.

## Details

**Files:**
- `report/report_record_statio_test.go` (NEW) — package `report`. Содержит 3 теста (или
  table-driven эквивалент с подтестами `v16`/`v18`/`time`, лишь бы попадали под `-run StatIO`).
  Импорты повторяют `report_test.go`: `archive/tar`, `bytes`, `database/sql`, `encoding/json`,
  `time`, `github.com/lesovsky/pgcenter/internal/stat`, `testify`.
- `report/testdata/report_record_stat_io_v16.golden`, `..._v18.golden`,
  `..._stat_io_time.golden` (NEW) — генерируются `-update`.

**Dependencies:**
- depends_on Task 01 — `stat_io`/`stat_io_time` должны быть recordable (без `NotRecordable`),
  иначе логически тест валиден только после снятия флага. (Для самого report-replay layout/diff
  флаг не критичен — `view.New()[config.ReportType]` отдаёт view в любом случае, — но запускать
  тест имеет смысл на ветке после Task 01.)
- Никаких новых пакетов.

**Edge cases:**
- coalesced NULL-ячейки: pg_stat_io отдаёт NULL широко (fsyncs у temp, reads у bgwriter);
  recorder хранит их как `"0"`. Тест ДОЛЖЕН включать строку, где часть diffed-ячеек = `"0"` в
  обоих тиках, и проверять, что дельта чистый `"0"` и строка не пропадает (это пересекается с
  debt [007], но здесь — на уровне полного replay, а не unit-diff).
- io_key как UniqueKey 0: оба тика ОБЯЗАНЫ иметь одинаковый `io_key` в строке, иначе `diff()`
  не спарит строки и дельта будет неверной/нулевой по другой причине. Используй реальный
  10-символьный md5-префикс или любую стабильную 10-символьную строку — главное идентичность
  между тиками (значение ключа не парсится как число).
- PG18-специфика — это только данные (значения KiB и, опционально, лишние строки object='wal'),
  НЕ изменение shape: `Ncols=16`, `DiffIntvl={4,14}` идентичны v16. Не вводить разный Ncols.
- Первый тик потребляется как prev (правило `!prevStat.Valid -> continue`), вывод даёт только
  второй тик — golden фиксирует дельту тик2-тик1.

**Implementation hints:**
- Дословно копируй каркас из `Test_app_doReport_procpidstat` (report/report_test.go:604-716):
  `metaRes` (7-колоночный shape, readMeta берёт только col 1 `version_num`), `writeEntry`-
  замыкание, порядок entries `meta → stat_io → meta → stat_io`, `Config{ReportType, TruncLimit,
  TsStart, TsEnd}`, `app := newApp(config)`, `app.writer = &buf`, `app.doReport(tar.NewReader(...))`.
- ReportType в Config: `"stat_io"` и `"stat_io_time"` (должны совпадать с ключами `view.New()`).
- version_num в meta: `"160000"` для v16, `"180000"` для v18 (формат как `"140009"` в эталоне).
- Колонки бери ИЗ `internal/query/io.go` (16 имён для count, 10 для time) — порядок строго по
  SELECT. Кавычки в именах колонок (`"read,KiB"`) — это часть имени, сохрани как есть в строке.
- Для golden используй существующий механизм `*update` флага (см. начало report_test.go,
  `var update = flag.Bool("update", ...)`) — пиши вывод в файл при `-update`, иначе сравнивай.
- Значения подбирай так, чтобы дельта была наглядной и стабильной (без now()/таймстемпов внутри
  diffed-колонок); `stats_age` — абсолютная текстовая колонка вне DiffIntvl, можно фиксированной
  строкой.

## Reviewers

- **dev-code-reviewer** → `docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-task-06-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-task-06-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [008-feat-record-report-0-11-views-decisions.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
