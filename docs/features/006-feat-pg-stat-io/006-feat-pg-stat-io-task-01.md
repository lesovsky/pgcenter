---
status: done                       # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # go test ./internal/query/... passes (selector shape per version + NULL-safety + live PG16/18 shape)
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
teammate_name:
---

# Task 01: Query layer — internal/query/io.go + version constants

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Строим слой данных для нового экрана `pg_stat_io` (PostgreSQL 16+). Это первая (Wave 1) задача фичи 006 — она поставляет per-version SQL и два селектора, на которые опираются задачи Wave 2 (регистрация view — Task 02; TUI-навигация — Task 03). Сама по себе задача не трогает TUI и не регистрирует view — только query-слой.

Экран `pg_stat_io` многострочный: одна строка на комбинацию `backend_type × object × context`. Кумулятивные счётчики показываются как per-interval rates через существующий pipeline query → format → diff. Экран разбит на два sub-view: **count** (счётчики операций + KiB throughput) и **time** (тайминги операций) — потому что в pgcenter нет горизонтальной прокрутки и полный набор колонок не помещается. Соответственно нужно два селектора с разной формой колонок.

Ключевые сложности, заложенные в Decisions tech-spec:
- **Per-version SQL ветки (Decision 3):** PG16/17 выводят KiB из `op_bytes` (`reads*op_bytes/1024`); PG18 использует нативные `read_bytes/write_bytes/extend_bytes` (`read_bytes/1024`). Логический набор колонок, `Ncols` и `DiffIntvl` идентичны между ветками — отличается только SQL-источник KiB-колонок. PG18 дополнительно возвращает строки `object='wal'` и `context='init'` (это лишние строки, не лишние колонки).
- **Синтетический `io_key` (Decision 2):** `view.UniqueKey` — это индекс одной колонки, а `stat.diff()` матчит строки между сэмплами ровно по одной колонке. Идентичность `backend_type × object × context` нужно свернуть в одну колонку 0 — `left(md5(backend_type||object||context),10) AS io_key` (тот же паттерн, что `pg_stat_statements` использует для `queryid`).
- **NULL-safety (Decision 5):** `pg_stat_io` повсеместно возвращает NULL (`fsyncs` для `temp relation`, `reads` для `background writer`). NULL внутри `DiffIntvl` доходит до `strconv.ParseInt("")` и обрывает весь сэмпл → пустой экран. Поэтому каждая диффуемая колонка обёрнута в `coalesce(...,0)`.
- **Фильтр пустых строк (Decision 5):** SQL-side `WHERE` отбрасывает строки, где все count-счётчики нулевые (сумма count-счётчиков > 0). Time-экран использует тот же count-based `WHERE`, чтобы набор строк на обоих sub-view был идентичен.
- **KiB через целочисленное деление (Decision 6):** KiB вычисляется целочисленным `(...)/1024`, не float — иначе PG18 `numeric` `*_bytes` отрендерятся с десятичными знаками.
- **Version-константы (Decision 8):** добавить `PostgresV15/16/17/18` в `internal/query/query.go` и использовать их в ветках селекторов (`version >= PostgresV18` / `>= PostgresV16`).

## What to do

1. В `internal/query/query.go` добавить в блок `const (...)` четыре числовые version-константы: `PostgresV15 = 150000`, `PostgresV16 = 160000`, `PostgresV17 = 170000`, `PostgresV18 = 180000` — после существующей `PostgresV14`.

2. Создать `internal/query/io.go` (package `query`) с per-version SQL-константами:
   - **count, PG16/17:** SELECT, выводящий ровно 16 колонок в порядке Data Models (см. таблицу в Details). KiB-колонки выводятся из `op_bytes`: `reads*op_bytes/1024`, `writes*op_bytes/1024`, `extends*op_bytes/1024`.
   - **count, PG18:** те же 16 колонок в том же порядке, но KiB из нативных `read_bytes/1024`, `write_bytes/1024`, `extend_bytes/1024`.
   - **time, PG16/17 и PG18:** SELECT, выводящий ровно 10 колонок в порядке Data Models (тайминги одинаковы между версиями — здесь SQL может не требовать отдельной PG18-ветки; но WHERE привязан к count-счётчикам, которые есть на всех версиях). Реши, нужна ли отдельная PG18-константа для time, исходя из того, отличается ли SQL: если идентичен — одной константы достаточно (как `replslots`); если нет — две.
   - Колонка 0 во всех запросах: `left(md5(backend_type||object||context),10) AS io_key`.
   - Каждая диффуемая колонка обёрнута в `coalesce(...,0)`.
   - KiB через целочисленное `/1024`.
   - `stats_age` — последняя колонка: `date_trunc('seconds', now() - stats_reset)::text AS stats_age` (по образцу `bgwriter`/`wal`/`replslots`).
   - `WHERE` (count-based, на обоих экранах): сумма всех count-счётчиков `coalesce(reads,0)+coalesce(writes,0)+coalesce(writebacks,0)+coalesce(extends,0)+coalesce(hits,0)+coalesce(evictions,0)+coalesce(reuses,0)+coalesce(fsyncs,0) > 0`.

3. В `internal/query/io.go` реализовать два селектора с сигнатурой как у `SelectStatBgwriterQuery`:
   - `SelectStatIOQuery(version int) (string, int, [2]int)` — возвращает `(query, 16, [2]int{4,14})`; ветвится на `version >= PostgresV18` → PG18-SQL, иначе → PG16/17-SQL.
   - `SelectStatIOTimeQuery(version int) (string, int, [2]int)` — возвращает `(query, 10, [2]int{4,8})`; ветвится так же.
   - Для версий < PG16 селектор всё равно возвращает PG16-форму query (гейт по `MinRequiredVersion` живёт на уровне view, не селектора; `Configure()` форматирует шаблон каждого view независимо от версии, поэтому SQL должен быть Format-безопасен).
   - Каждую константу и ветку снабдить комментарием в стиле `bgwriter.go`/`replication_slots.go`, объясняющим раскладку колонок (0-based) и почему `coalesce` обязателен.

4. Создать `internal/query/io_test.go`:
   - **Table-driven селектор-тесты** для обоих селекторов над версиями `{140000,150000,160000,170000,180000}`, проверяющие возвращаемые `(Ncols, DiffIntvl)`: count → `(16, [4,14])`, time → `(10, [4,8])` на всех версиях (PG16/17 и PG18 имеют одинаковую форму). Образец — `Test_SelectStatBgwriterQuery`.
   - **NULL-row diff-safety unit-тест:** строка с NULL в диффуемой колонке не обрывает сэмпл (NULL→0). См. подсказки в Details — паттерн через `diff` на синтетических данных или через выполнение запроса с гарантированно NULL-содержащей строкой; выбрать подход, который реально проверяет «NULL→0», а не просто «запрос выполнился».
   - **Live integration-тесты** (по образцу `Test_StatBgwriterQueries`): `NewTestConnectVersion(version)`, `t.Skipf` при недоступности. Для каждого доступного PG16–18 выполнить запрос обоих селекторов и проверить `assert.Len(rows.FieldDescriptions(), wantNcols)`. PG16 run гейтит `op_bytes`-путь; PG18 run гейтит нативные `*_bytes` И наличие строк `object='wal'` (форма, которую локальная PG17-only среда не проверяет). Для PG14/15 пропустить Ncols-проверку (`pg_stat_io` там не существует) либо запускать только для `>=16`.

## TDD Anchor

Тесты пишутся ДО реализации SQL/селекторов. Пишем → запускаем → убеждаемся что падают → пишем код → убеждаемся что проходят.

- `internal/query/io_test.go::Test_SelectStatIOQuery` — `SelectStatIOQuery(v)` возвращает `(16, [4,14])` для v ∈ {140000,150000,160000,170000,180000}.
- `internal/query/io_test.go::Test_SelectStatIOTimeQuery` — `SelectStatIOTimeQuery(v)` возвращает `(10, [4,8])` для тех же версий.
- `internal/query/io_test.go::Test_SelectStatIOQuery_NullSafety` — строка с NULL в диффуемой колонке не обрывает сэмпл (NULL→0 через `coalesce`); сэмпл диффуется без ошибки.
- `internal/query/io_test.go::Test_StatIOQueries` — live: для каждого доступного PG16–18 запрос `SelectStatIOQuery` выполняется, `FieldDescriptions()` имеет 16 колонок; PG18 run дополнительно содержит строки `object='wal'`.
- `internal/query/io_test.go::Test_StatIOTimeQueries` — live: для каждого доступного PG16–18 запрос `SelectStatIOTimeQuery` выполняется, `FieldDescriptions()` имеет 10 колонок.

## Acceptance Criteria

- [ ] В `internal/query/query.go` добавлены константы `PostgresV15=150000`, `PostgresV16=160000`, `PostgresV17=170000`, `PostgresV18=180000`.
- [ ] `internal/query/io.go` содержит per-version SQL: PG16/17-ветка с `op_bytes`-производными KiB, PG18-ветка с нативными `read_bytes/write_bytes/extend_bytes`.
- [ ] Count-запрос выводит ровно 16 колонок в порядке Data Models, колонка 0 = `left(md5(backend_type||object||context),10) AS io_key`.
- [ ] Time-запрос выводит ровно 10 колонок в порядке Data Models, колонка 0 = `io_key`.
- [ ] Каждая диффуемая колонка обёрнута в `coalesce(...,0)`; KiB вычисляется целочисленным `/1024`.
- [ ] Оба запроса используют один и тот же count-based `WHERE` (сумма count-счётчиков > 0).
- [ ] `SelectStatIOQuery(version)` возвращает `(query, 16, [2]int{4,14})`, ветвится на `PostgresV18`/`PostgresV16`.
- [ ] `SelectStatIOTimeQuery(version)` возвращает `(query, 10, [2]int{4,8})`, ветвится так же.
- [ ] Селектор-тесты проходят на версиях {14,15,16,17,18} (форма одинакова на всех).
- [ ] NULL в диффуемой колонке не обрывает сэмпл — покрыто unit-тестом.
- [ ] Live integration-тесты: PG16 гейтит `op_bytes`-путь, PG18 гейтит нативные `*_bytes` + наличие строк `object='wal'`; недоступные версии — `t.Skipf`.
- [ ] `go test ./internal/query/...` зелёный (с учётом `t.Skipf` для недоступных кластеров).

## Context Files

**Feature artifacts:**
- [006-feat-pg-stat-io.md](docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io.md) — user-spec
- [006-feat-pg-stat-io-tech-spec.md](docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io-tech-spec.md) — tech-spec (Decisions 2,3,4,5,6,8,11 + Data Models)
- [006-feat-pg-stat-io-code-research.md](docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io-code-research.md) — code research (§1, §4 schema across versions, §6 zero rows, §7 tests, §8 constraints)
- [006-feat-pg-stat-io-decisions.md](docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — features, supported stats
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow, PG version handling
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — code patterns, testing conventions, version branching

**Code files:**
- [internal/query/query.go](internal/query/query.go) — add `PostgresV15/16/17/18` constants to the const block
- [internal/query/io.go](internal/query/io.go) — NEW: query constants + `SelectStatIOQuery` / `SelectStatIOTimeQuery`
- [internal/query/io_test.go](internal/query/io_test.go) — NEW: selector + NULL-safety + live tests
- [internal/query/bgwriter.go](internal/query/bgwriter.go) — model: per-version branches + selector returning per-version Ncols/DiffIntvl + stats_age last
- [internal/query/wal.go](internal/query/wal.go) — model: minimal two-branch selector
- [internal/query/replication_slots.go](internal/query/replication_slots.go) — model: multi-row, coalesce on diffed cols, KiB via integer /1024, stats_age outside DiffIntvl
- [internal/query/statements.go](internal/query/statements.go) — model: `left(md5(...),10)` synthetic composite key (line 65)
- [internal/query/bgwriter_test.go](internal/query/bgwriter_test.go) — model: selector table-test + live FieldDescriptions Ncols-gate
- [internal/postgres/testing.go](internal/postgres/testing.go) — `NewTestConnectVersion` port map (PG14–18)

## Verification Steps

- Запустить `go test ./internal/query/...` — селектор-тесты и NULL-safety зелёные; live-тесты для недоступных версий `t.Skipf`-нуты, не падают.
- Если локально доступен PG17 — live-тест выполняет оба запроса, `FieldDescriptions()` == 16 (count) и 10 (time).
- На CI-матрице PG14–18 (вне локального прогона): PG16 run проходит `op_bytes`-путь; PG18 run подтверждает нативные `*_bytes` и наличие строк `object='wal'`.
- Проверить, что `go build ./internal/query/...` чистый (новые константы используются, нет неиспользуемых импортов).
- Ожидаемый результат: `go test ./internal/query/...` зелёный.

## Details

**Files:**
- `internal/query/query.go` — текущее состояние: блок `const (...)` (строки 9–18) останавливается на `PostgresV14 = 140000`. Добавить четыре строки `PostgresV15`…`PostgresV18` после `PostgresV14`. Больше ничего в файле не трогать.
- `internal/query/io.go` — НОВЫЙ. По образцу `bgwriter.go`: блок `const (...)` с SQL-шаблонами + функции-селекторы. В этом же пакете `query`, поэтому константы версий без префикса: `PostgresV18`, `PostgresV16`.
- `internal/query/io_test.go` — НОВЫЙ. По образцу `bgwriter_test.go`: imports `fmt`, `github.com/lesovsky/pgcenter/internal/postgres`, `github.com/stretchr/testify/assert`, `testing`. Table-driven селектор-тесты + live-тесты с `NewTestConnectVersion` + `t.Skipf`.

**Точные раскладки колонок (Data Models tech-spec — соблюсти ИМЕННО так):**

count screen (`SelectStatIOQuery`) — Ncols 16, `DiffIntvl [4,14]`:
```
0  io_key        = left(md5(backend_type||object||context),10)
1  backend_type
2  object
3  context
4  reads          (diff)
5  read,KiB       (diff)  = reads*op_bytes/1024 (PG16/17) | read_bytes/1024 (PG18)
6  writes         (diff)
7  write,KiB      (diff)  = writes*op_bytes/1024 | write_bytes/1024
8  extends        (diff)
9  ext,KiB        (diff)  = extends*op_bytes/1024 | extend_bytes/1024
10 hits           (diff)
11 evictions      (diff)
12 writebacks     (diff)
13 reuses         (diff)
14 fsyncs         (diff)
15 stats_age      (absolute, outside DiffIntvl)
```
DiffIntvl `[4,14]` = колонки reads…fsyncs включительно. Колонки 0–3 (io_key + dims) и 15 (stats_age) — вне диапазона.

time screen (`SelectStatIOTimeQuery`) — Ncols 10, `DiffIntvl [4,8]`:
```
0  io_key
1  backend_type
2  object
3  context
4  read_time      (diff)
5  write_time     (diff)
6  writeback_time (diff)
7  extend_time    (diff)
8  fsync_time     (diff)
9  stats_age      (absolute)
```
DiffIntvl `[4,8]` = read_time…fsync_time. WHERE на time-экране — тот же count-based, что на count-экране (Decision 5), чтобы набор строк совпадал.

**Dependencies:** нет (Wave 1, `depends_on: []`). Эта задача — контракт для Wave 2 (Task 02 регистрирует view и зовёт `SelectStatIOQuery`/`SelectStatIOTimeQuery`; Task 03 — TUI-навигация). Имена селекторов и форма возврата `(string, int, [2]int)` зафиксированы — не менять.

**Edge cases:**
- NULL в любой диффуемой колонке (`fsyncs` NULL для `temp relation`, `reads` NULL для `background writer`) → обязателен `coalesce(...,0)`, иначе `diffPair → ParseInt("")` обрывает сэмпл → пустой экран. Это центральный риск задачи.
- `op_bytes` теоретически может быть NULL на строке → в PG16/17-ветке обернуть множитель: `coalesce(reads,0)*coalesce(op_bytes,0)/1024`.
- PG18 `*_bytes` имеют тип `numeric` → целочисленное `/1024` с приведением к bigint (как `replslots` `(... / 1024)::bigint`) держит значения integer-typed, без десятичных знаков при рендере.
- Версии < PG16: `pg_stat_io` не существует. Селектор всё равно возвращает PG16-форму query (гейт — `MinRequiredVersion` на view-слое). В live-тестах для PG14/15 пропустить Ncols-проверку или ограничить прогон `>=16`.
- Все-нулевые строки отфильтровываются SQL-side `WHERE` (count-based) — должны быть одинаковыми на обоих sub-view.

**Implementation hints (НЕ псевдокод):**
- Структуру `io.go` копировать с `bgwriter.go`: const-блок с именованными SQL-строками (напр. `PgStatIOPG16`, `PgStatIOPG18`, `PgStatIOTime…`), затем селекторы со `if version >= PostgresV18 { ... }` / иначе PG16-ветка — ровно как `SelectStatBgwriterQuery`.
- Синтетический ключ — образец `statements.go:65`: `left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10)`. Для io: `left(md5(backend_type || object || context), 10) AS io_key`. Учесть, что `object`/`context` могут быть NULL на некоторых строках — при необходимости `coalesce(object,'')` внутри md5, чтобы конкатенация не давала NULL-ключ.
- KiB / stats_age / coalesce-паттерны — образец `replication_slots.go` (строки 18–28): `(... / 1024)::bigint AS "...,KiB"`, `date_trunc('seconds', now() - stats_reset)::text AS stats_age`, `coalesce(ss.col, 0) AS col`.
- Имена колонок с запятой/спецсимволами оборачивать в двойные кавычки в SQL и использовать backtick-строки в Go (как `"read,KiB"` в bgwriter/wal/replslots).
- Запрос можно делать плоской не-шаблонной строкой (как `bgwriter`/`wal`); `{{...}}`-поля не нужны (нет recovery-aware функций). Но `Configure()` всё равно прогонит строку через `query.Format` — убедиться, что в SQL нет случайных `{{`/`}}`.
- Селектор-тесты: образец `Test_SelectStatBgwriterQuery` (`bgwriter_test.go:10–32`) — table-driven, `assert.Equal` на Ncols и DiffIntvl.
- Live-тесты: образец `Test_StatBgwriterQueries` (`bgwriter_test.go:35–63`) — `NewOptions(version, "f", "off", 256, "public")`, `Format(tmpl, opts)`, `NewTestConnectVersion(version)` + `t.Skipf`, `conn.Query(q)`, `assert.Len(rows.FieldDescriptions(), wantNcols)`.
- Для PG18-проверки `object='wal'`: после выполнения запроса просканировать строки и убедиться, что хотя бы одна имеет `object='wal'` (форма, недоступная на локальном PG17). Допустимо отдельным sub-запросом `SELECT count(*) FROM pg_stat_io WHERE object='wal'` под version-гейтом.
- NULL-safety тест: сконструировать сэмпл из строк с NULL в диффуемой колонке и прогнать `diff`, либо выполнить live-запрос и убедиться, что внутри `DiffIntvl` нет пустых строк (все обёрнуты coalesce). Выбрать подход, который реально проверяет «NULL→0», а не просто «запрос выполнился». Посмотреть, как diff обрабатывает значения (`internal/stat/postgres.go` diffPair/parsePairInt), и переиспользовать минимально.
- Локально поднят только PG17 — live PG16/PG18 проверки уйдут в `t.Skipf`; их реальный гейт — CI-матрица. Не подгонять тест под локальную доступность.
- Tech-debt [005]: `Test_doReload` может паниковать локально при отсутствии фикстур — это не относится к `internal/query`, прогоняй таргетно `go test ./internal/query/...`.

## Reviewers

- **dev-code-reviewer** → `docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io-task-01-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io-task-01-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io-task-01-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [006-feat-pg-stat-io-decisions.md](docs/features/006-feat-pg-stat-io/006-feat-pg-stat-io-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека (напр. понадобилась отдельная PG18-константа для time-запроса, или md5 вместо плоской конкатенации) — описать отклонение и причину
- [ ] Обновить tech-spec, если форма селекторов или контракт имён изменились (Wave 2/3 на них опираются)
