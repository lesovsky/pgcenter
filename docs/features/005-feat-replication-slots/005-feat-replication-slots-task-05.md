---
status: planned                    # planned -> in_progress -> done
depends_on: ["01", "04"]           # ID задач-зависимостей (строки: ["01", "02"])
wave: 2                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — make test (tier-2 green; tier-3 green on 0.0.10, else skipped)
reviewers: [dev-test-reviewer, dev-code-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:
---

# Task 05: Physical + logical slot integration tests

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Расширяем `internal/query/replication_slots_test.go` (файл создан в task-01 вместе с unit-тестом
селектора и tier-1 execute-тестом) двумя живыми интеграционными тестами поверх матрицы PG 14–18.
Цель — доказать, что гибридный запрос `pg_replication_slots LEFT JOIN pg_stat_replication_slots`
корректно отдаёт данные на реальных слотах, а не только на пустом наборе:

- **Tier 2 (физический слот)** проверяет, что физический слот появляется в результате, колонка
  `retained,KiB` непустая (slot создаётся с немедленным резервированием WAL), а восемь
  diff-колонок логического декодирования рендерятся как `0` благодаря `coalesce(...,0)`
  (Decision 2 — без coalesce `diffPair("","")` → `ParseInt("")` аварийно прерывал бы выборку).
- **Tier 3 (логический слот)** проверяет, что логический слот присутствует и spill/stream-колонки
  доступны. Он требует образ `pgcenter-testing:0.0.10` с `wal_level=logical` (task-04), поэтому
  защищён `t.Skipf` если `wal_level != logical` и если плагин `test_decoding` недоступен
  (Decision 5). Это разрывает жёсткую зависимость между ручным пушем образа мейнтейнером и мержем
  кода: на старом образе (`wal_level=replica`) tier-3 просто пропускается, CI остаётся зелёным.

Оба теста создают слоты идемпотентно (drop-if-exists перед create) и дропают их в `defer`, чтобы
прерванный SIGKILL'ом прогон не блокировал повторный запуск из-за дублирующегося имени слота и не
оставлял удерживаемый WAL на общем тестовом фикстуре.

## What to do

1. Открой созданный в task-01 файл `internal/query/replication_slots_test.go`, изучи как там уже
   оформлены unit-тест селектора и tier-1 integration-тест (имена функций, как `Format` +
   `NewOptions` готовят запрос, как `NewTestConnectVersion` + `t.Skipf` обходят недоступные версии,
   как читается `FieldDescriptions()`).
2. Добавь tier-2 интеграционный тест на физический слот (отдельная top-level test-функция, та же
   матрица версий `[]int{140000, 150000, 160000, 170000, 180000}`, по сабтесту на версию):
   - подключись через `postgres.NewTestConnectVersion(version)`, `t.Skipf` если версия недоступна,
     `defer conn.Close()`;
   - идемпотентно создай физический слот `pgcenter_test_phys`: сначала drop-if-exists (guarded —
     дроп только если слот уже есть в `pg_replication_slots`), затем
     `SELECT pg_create_physical_replication_slot('pgcenter_test_phys', true)` (второй аргумент
     `true` = немедленно зарезервировать WAL, чтобы `restart_lsn`/`retained,KiB` были non-NULL);
   - `defer` дроп слота (guarded, чтобы teardown не падал если тест уже дропнул);
   - сформируй и выполни форматированный replslots-запрос (тот же `Format(tmpl, opts)`, что и в
     tier-1), найди строку со `slot_name == "pgcenter_test_phys"`;
   - проверь: строка присутствует; `retained,KiB` (col 4) non-NULL/непустая; все восемь diff-колонок
     блока `[6,13]` рендерятся как `"0"` (а не пусто и не NULL).
3. Добавь tier-3 интеграционный тест на логический слот (отдельная top-level test-функция, та же
   матрица версий):
   - подключись, `t.Skipf` если версия недоступна;
   - выполни `SHOW wal_level` → `t.Skipf` если значение не `logical` (тогда tier-3 пропускается на
     старом образе и не требует координации с пушем 0.0.10);
   - идемпотентно создай логический слот `pgcenter_test_logical` через
     `pg_create_logical_replication_slot('pgcenter_test_logical', 'test_decoding')`; если создание
     падает из-за отсутствия плагина `test_decoding` — `t.Skipf` (graceful), не `assert.NoError`;
   - `defer` guarded-дроп слота;
   - выполни запрос, найди строку со `slot_name == "pgcenter_test_logical"`;
   - проверь: строка присутствует; spill/stream-колонки блока `[6,13]` присутствуют в результате
     (значения могут быть `0` — декодирование ещё не запускалось).
4. Прогони `make test` и убедись: tier-2 зелёный на доступных версиях; tier-3 пропускается на
   текущем образе (`wal_level=replica`) либо зелёный когда живёт `0.0.10`. Никаких регрессий в уже
   существующих unit/tier-1 тестах из task-01.

## TDD Anchor

Это test-задача: TDD Anchor — сами интеграционные тесты с конкретными assertions. Пиши тест →
запускай → убеждайся что падает/пропускается по правильной причине → доводи до зелёного.

- `internal/query/replication_slots_test.go::Test_StatReplicationSlotsPhysical` (PG 14–18) —
  создаёт физический слот `pgcenter_test_phys` с резервированием WAL, выполняет форматированный
  запрос; ассертит: строка слота присутствует, `retained,KiB` (col 4) non-NULL, восемь diff-колонок
  блока `[6,13]` равны `"0"`. Слот создаётся идемпотентно (drop-if-exists) и дропается в `defer`.
- `internal/query/replication_slots_test.go::Test_StatReplicationSlotsLogical` (PG 14–18) —
  `t.Skipf` если `SHOW wal_level != logical`; создаёт логический слот `pgcenter_test_logical`
  (плагин `test_decoding`), `t.Skipf` если плагин недоступен; ассертит: строка слота присутствует,
  spill/stream-колонки блока `[6,13]` присутствуют. Слот создаётся идемпотентно и дропается в
  `defer`.

(Точные имена test-функций — на усмотрение исполнителя в стиле уже существующих в файле; здесь
приведены как ориентир.)

## Acceptance Criteria

- [ ] Tier-2 physical: на доступных PG 14–18 строка `pgcenter_test_phys` присутствует в результате,
      `retained,KiB` non-NULL, восемь diff-колонок рендерятся как `"0"`.
- [ ] Tier-3 logical: при `wal_level=logical` строка `pgcenter_test_logical` присутствует и
      spill/stream-колонки доступны; при `wal_level != logical` тест корректно `t.Skipf`-ается.
- [ ] Tier-3 при отсутствии плагина `test_decoding` `t.Skipf`-ается gracefully, не падает.
- [ ] Оба теста создают слоты идемпотентно (drop-if-exists перед create) и дропают их в `defer` —
      повторный запуск после прерванного прогона проходит без ошибки «slot already exists».
- [ ] Недоступные версии PG обходятся через `t.Skipf` (как в tier-1).
- [ ] Существующие unit/tier-1 тесты из task-01 не сломаны; `make test` зелёный.
- [ ] `make lint`, `make vuln`, `make build` чисто.

## Context Files

**Feature artifacts:**
- [005-feat-replication-slots.md](docs/features/005-feat-replication-slots/005-feat-replication-slots.md) — user-spec
- [005-feat-replication-slots-tech-spec.md](docs/features/005-feat-replication-slots/005-feat-replication-slots-tech-spec.md) — tech-spec (Decision 2, Decision 5, Testing Strategy → tier-2/tier-3)
- [005-feat-replication-slots-decisions.md](docs/features/005-feat-replication-slots/005-feat-replication-slots-decisions.md) — decisions log

**Project knowledge:**
- [project.md](.claude/skills/project-knowledge/overview.md) — overview/features (в проекте файл называется `overview.md`)
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — PG version handling, Testing helpers (NewTestConnectVersion, port map)
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — Testing conventions: версии в `versions := []int{...}`, `t.Skipf` для недоступных версий

**Code files:**
- [internal/query/replication_slots_test.go](internal/query/replication_slots_test.go) — расширить (создан в task-01); добавить tier-2 и tier-3 тесты
- [internal/query/replication_test.go](internal/query/replication_test.go) — образец живого integration-теста (`Test_StatReplicationQueries`: матрица версий, `Format`+`NewOptions`, `NewTestConnectVersion`+`t.Skipf`, `conn.Exec`)
- [internal/query/bgwriter_test.go](internal/query/bgwriter_test.go) — образец `conn.Query` + `FieldDescriptions()`-ассерта + `defer conn.Close()` (`Test_StatBgwriterQueries`)
- [internal/postgres/testing.go](internal/postgres/testing.go) — `NewTestConnectVersion(version)`, port map PG 14–18
- [internal/postgres/postgres.go](internal/postgres/postgres.go) — DB API: `Exec`, `Query`, `QueryRow(...).Scan(&x)` (для `SHOW wal_level` и create/drop слота)

## Verification Steps

- Запусти `make test`. Ожидаемо: на текущем образе (`wal_level=replica`) tier-2 зелёный, tier-3
  выводит `--- SKIP` с причиной про `wal_level`. Недоступные версии PG — `--- SKIP`.
- Прогони тесты дважды подряд (`make test` ещё раз) — второй прогон не должен падать на «slot
  pgcenter_test_phys already exists» (доказательство идемпотентности + defer-teardown).
- Если доступен образ `0.0.10` с `wal_level=logical`: tier-3 зелёный, строка
  `pgcenter_test_logical` присутствует.
- `make lint`, `make vuln`, `make build` — без ошибок.

## Details

**Files:**
- `internal/query/replication_slots_test.go` — расширить. Файл уже содержит (из task-01): unit-тест
  селектора `SelectStatReplicationSlotsQuery` и tier-1 execute-тест (`Format` → `conn.Query` →
  `assert.Len(rows.FieldDescriptions(), 15)`). Добавить две новые top-level test-функции (tier-2,
  tier-3) — НЕ переписывать существующие. Использовать тот же `package query`, те же импорты
  (`postgres`, `assert`, `testing`, `fmt`).

**Dependencies:**
- task-01 — создаёт `replication_slots.go` (запрос + селектор + константа `PgStatReplicationSlots`)
  и сам файл `replication_slots_test.go`. Без него нечего расширять и нечего форматировать.
- task-04 — поставляет образ `pgcenter-testing:0.0.10` с `wal_level=logical` (для tier-3). На момент
  выполнения этой задачи образ может быть ещё не запушен — это нормально: tier-3 защищён `t.Skipf`
  (Decision 5), так что задача завершаема и зелёна даже на старом образе.
- Пакеты — все существующие, новых не добавлять: `internal/postgres`, `internal/query`, `testify`.

**DB API (из `internal/postgres/postgres.go`):**
- `conn.Exec(query string, args ...any)` — для `pg_create_*_replication_slot` / `pg_drop_replication_slot`.
- `conn.QueryRow(query).Scan(&dst)` — для `SHOW wal_level` (читать в `string`) и для guarded-проверки
  существования слота (`SELECT count(*) FROM pg_replication_slots WHERE slot_name = $1`).
- `conn.Query(q)` — для выполнения форматированного replslots-запроса; читать строки через
  `rows.Next()` + `rows.Scan(...)`, либо считать значения как текст. Колонки `retained,KiB` и
  diff-блок в SQL приведены к `::bigint`/`::text` — учитывай это при выборе типов для Scan
  (например `sql.NullInt64`/`sql.NullString` для nullable absolute-колонок, см. tech-spec Decision 2).

**Колоночная карта (0-based, из tech-spec Decision 1):** 0 slot_name, 1 slot_type, 2 active,
3 wal_status, 4 retained,KiB, 5 safe,KiB, 6–13 — восемь diff-колонок (spill_txns, spill_count,
spill,KiB, stream_txns, stream_count, stream,KiB, total_txns, total,KiB), 14 stats_age. Ncols=15.

**Edge cases:**
- Прерванный прошлый прогон оставил слот → drop-if-exists перед create (guarded по
  `pg_replication_slots`, иначе `pg_drop_replication_slot` на несуществующем имени даёт ошибку).
- `wal_level != logical` (текущий образ) → tier-3 `t.Skipf`, не fail.
- Плагин `test_decoding` отсутствует → `pg_create_logical_replication_slot` падает → `t.Skipf`, не
  `assert.NoError`.
- Версия PG недоступна в окружении → `t.Skipf` (как в tier-1/bgwriter).
- Физический слот отсутствует в `pg_stat_replication_slots` → diff-колонки приходят NULL из LEFT
  JOIN, но `coalesce(...,0)` в SQL делает их `0` — именно это и проверяет tier-2.
- Дроп слота в `defer` обязателен, иначе физический слот с зарезервированным WAL удерживает WAL на
  общем фикстуре (риск из tech-spec Risks).

**Implementation hints (НЕ псевдокод):**
- Скопируй каркас матрицы версий и `t.Skipf`-обвязку из `Test_StatBgwriterQueries`
  (`bgwriter_test.go`) — это ближайший образец `conn.Query`+`defer conn.Close()`+per-version
  сабтестов с тем же `[]int{140000,...,180000}`.
- Для idempotent-create вынеси маленький helper (drop-if-exists), чтобы tier-2 и tier-3 не
  дублировали guarded-логику; имя слота передавай параметром.
- Имена слотов фиксированные: `pgcenter_test_phys`, `pgcenter_test_logical` — статические литералы
  (не подставляются из пользовательского ввода), так что SQL-инъекции тут нет; для имени в
  `pg_drop_replication_slot` используй параметризацию `$1`, а не конкатенацию — следуй стилю
  существующих тестов (`pg_terminate_backend($1)` в `internal/postgres/postgres_test.go:143`).
- Для проверки «строка присутствует» собери результат запроса в map по `slot_name` или пройди
  `rows.Next()` с поиском нужного `slot_name`; assert через testify (`assert.True`/`assert.Equal`).
- Defer-дроп тоже guarded, чтобы teardown не падал если основной тест уже дропнул слот или слот не
  был создан (ранний `t.Skipf` по плагину).

## Reviewers

- **dev-test-reviewer** → `docs/features/005-feat-replication-slots/005-feat-replication-slots-task-05-dev-test-reviewer-review.json`
- **dev-code-reviewer** → `docs/features/005-feat-replication-slots/005-feat-replication-slots-task-05-dev-code-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [005-feat-replication-slots-decisions.md](docs/features/005-feat-replication-slots/005-feat-replication-slots-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
