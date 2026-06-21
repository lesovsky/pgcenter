---
status: planned                    # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # make test (selector unit test + tier-1 integration green on available PG versions)
reviewers: [dev-code-reviewer, dev-test-reviewer]
teammate_name:
---

# Task 01: Hybrid query + selector + unit/tier-1 tests

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Это data-core новой фичи «replication slots screen» (feature 005). Нужно добавить новый файл
запроса `internal/query/replication_slots.go` с единственной гибридной SQL-строкой
`pg_replication_slots LEFT JOIN pg_stat_replication_slots ON slot_name` (покрывает и физические,
и логические слоты) и version-aware-селектор `SelectStatReplicationSlotsQuery(_ int) (string, int, [2]int)`.

Запрос версионно-независим: выбранный набор колонок стабилен на PG 14–18, поэтому ветвления по
версии нет (Decision 1/4). Селектор повторяет сигнатуру `SelectStatBgwriterQuery` (симметрия +
точка расширения, если позже добавят `conflicting`/`invalidation_reason`), а неиспользуемый
параметр версии назван `_`, чтобы удовлетворить revive.

Восемь diff-колонок логического декодирования (spill/stream/total) оборачиваются в `coalesce(...,0)`
— иначе физический слот (отсутствующий в `pg_stat_replication_slots`) даёт NULL в diff-блоке, и
`diffPair("","")` → `strconv.ParseInt("")` падает с ошибкой и обрывает сэмпл (Decision 2). Колонки
вне `DiffIntvl` (retained, safe, stats_age) остаются nullable — пустые ячейки для физических слотов.

Эта задача — изолированный data-core: только query-строка, селектор и тесты. Регистрация view в
TUI и hotkey идут отдельной задачей (Task 2). Здесь же пишутся unit-тест на селектор и tier-1
интеграционный тест (execute + `FieldDescriptions == 15`) на PG 14–18.

## What to do

1. Создать `internal/query/replication_slots.go` (package `query`):
   - Константа `PgStatReplicationSlots` — гибридная SQL-строка (verbatim из tech-spec Decision 1,
     см. блок ниже в Details). Использует шаблонные плейсхолдеры `{{.WalFunction1}}`/`{{.WalFunction2}}`
     для recovery-корректного расчёта retained WAL, `coalesce(...,0)` на восьми diff-счётчиках,
     деление bytes-колонок на 1024 с алиасами `",KiB"`, `stats_age` последней колонкой,
     `ORDER BY "retained,KiB" DESC NULLS LAST`.
   - Селектор `SelectStatReplicationSlotsQuery(_ int) (string, int, [2]int)` — возвращает
     `(PgStatReplicationSlots, 15, [2]int{6, 13})` независимо от версии. Параметр назван `_`.
   - Добавить doc-комментарии в стиле `bgwriter.go` (что за запрос, почему coalesce, раскладка колонок).

2. Создать `internal/query/replication_slots_test.go` (package `query`):
   - `Test_SelectStatReplicationSlotsQuery` — unit-тест: для версий 140000, 150000, 160000, 170000,
     180000 проверить `gotNcols == 15` и `gotDiffIntvl == [2]int{6, 13}` (зеркало
     `Test_SelectStatBgwriterQuery`). Пинит инвариант «нет дивергенции по версиям».
   - `Test_StatReplicationSlotsQueries` — tier-1 интеграционный тест (зеркало `Test_StatBgwriterQueries`):
     для каждой версии PG 14–18 взять `tmpl` из селектора, `Format` через
     `NewOptions(version, "f", "off", 256, "public")`, подключиться через
     `postgres.NewTestConnectVersion(version)` (`t.Skipf` если версия недоступна), выполнить
     `conn.Query(q)`, проверить `assert.Len(rows.FieldDescriptions(), 15)`, закрыть rows, проверить
     `rows.Err()`.

3. Запустить `make test` — убедиться что unit-тест и tier-1 интеграция зелёные на доступных PG-версиях.

## TDD Anchor

Пишем тесты → запускаем (падают, т.к. файла ещё нет) → пишем `replication_slots.go` → тесты зелёные.

- `internal/query/replication_slots_test.go::Test_SelectStatReplicationSlotsQuery` — селектор
  возвращает `(_, 15, [2]int{6, 13})` для версий 140000/150000/160000/170000/180000 (инвариант
  «один запрос на все версии»).
- `internal/query/replication_slots_test.go::Test_StatReplicationSlotsQueries` — на каждой живой
  PG 14–18 отформатированный запрос исполняется без ошибки и `FieldDescriptions()` содержит ровно
  15 колонок (schema-divergence gate; `t.Skipf` для недоступных версий).

## Acceptance Criteria

- [ ] `internal/query/replication_slots.go` создан: константа `PgStatReplicationSlots` + селектор
      `SelectStatReplicationSlotsQuery(_ int) (string, int, [2]int)`.
- [ ] Селектор возвращает `(PgStatReplicationSlots, 15, [2]int{6, 13})` для PG 14–18; unit-тест зелёный.
- [ ] SQL-строка совпадает verbatim с tech-spec Decision 1: WAL-плейсхолдеры, `coalesce(...,0)` на
      восьми счётчиках, KiB-деления, `stats_age` последней колонкой, `ORDER BY "retained,KiB" DESC NULLS LAST`.
- [ ] Раскладка колонок (0-based): 0 slot_name, 1 slot_type, 2 active, 3 wal_status, 4 retained,KiB,
      5 safe,KiB, 6–13 восемь diff-счётчиков, 14 stats_age.
- [ ] Параметр версии назван `_` (revive clean).
- [ ] Tier-1 интеграционный тест: на доступных PG 14–18 запрос исполняется, `FieldDescriptions() == 15`.
- [ ] `make test`, `make lint`, `make build` чистые (нет регрессий).

## Context Files

**Feature artifacts:**
- [005-feat-replication-slots.md](005-feat-replication-slots.md) — user-spec
- [005-feat-replication-slots-tech-spec.md](005-feat-replication-slots-tech-spec.md) — tech-spec (Decision 1/2/4 — каноничный SQL и контракт селектора)
- [005-feat-replication-slots-decisions.md](005-feat-replication-slots-decisions.md) — decisions log (создаётся в Post-completion)

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — features, supported stats, target audience
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow, PG version handling
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — query/selector pattern, testing conventions, version branching

**Code files:**
- [internal/query/replication_slots.go](../../../internal/query/replication_slots.go) — НОВЫЙ: query-константа + селектор
- [internal/query/replication_slots_test.go](../../../internal/query/replication_slots_test.go) — НОВЫЙ: unit + tier-1 тесты
- [internal/query/bgwriter.go](../../../internal/query/bgwriter.go) — образец: селектор `(string, int, [2]int)`, DiffIntvl вне абсолютного блока
- [internal/query/bgwriter_test.go](../../../internal/query/bgwriter_test.go) — образец: unit-тест + `Test_StatBgwriterQueries` tier-1
- [internal/query/replication.go](../../../internal/query/replication.go) — образец: WAL-плейсхолдеры, KiB-алиасы, ORDER BY
- [internal/query/query.go](../../../internal/query/query.go) — `Options`, `NewOptions`, `selectWalFunctions`, `Format`

## Verification Steps

- `make test` — `Test_SelectStatReplicationSlotsQuery` и `Test_StatReplicationSlotsQueries` зелёные
  (tier-1 пропускает недоступные версии через `t.Skipf`).
- `make lint` — revive не ругается на неиспользуемый параметр (он назван `_`).
- `make build` — компилируется.

## Details

**Files:**
- `internal/query/replication_slots.go` (новый) — добавить константу и селектор. Зеркалить структуру
  `bgwriter.go`: блок `const (...)` с doc-комментарием, затем функция-селектор с doc-комментарием.
- `internal/query/replication_slots_test.go` (новый) — зеркалить `bgwriter_test.go`: тот же набор
  импортов (`fmt`, `internal/postgres`, `testify/assert`, `testing`), та же структура table-driven
  unit-теста и tier-1 теста.

**Каноничный SQL (verbatim, tech-spec Decision 1):**

```
SELECT s.slot_name AS slot_name,
       s.slot_type AS slot_type,
       s.active::text AS active,
       s.wal_status AS wal_status,
       ({{.WalFunction1}}({{.WalFunction2}}(), s.restart_lsn) / 1024)::bigint AS "retained,KiB",
       (s.safe_wal_size / 1024)::bigint AS "safe,KiB",
       coalesce(ss.spill_txns, 0)  AS spill_txns,
       coalesce(ss.spill_count, 0) AS spill_count,
       (coalesce(ss.spill_bytes, 0) / 1024)::bigint  AS "spill,KiB",
       coalesce(ss.stream_txns, 0)  AS stream_txns,
       coalesce(ss.stream_count, 0) AS stream_count,
       (coalesce(ss.stream_bytes, 0) / 1024)::bigint AS "stream,KiB",
       coalesce(ss.total_txns, 0)   AS total_txns,
       (coalesce(ss.total_bytes, 0) / 1024)::bigint  AS "total,KiB",
       date_trunc('seconds', now() - ss.stats_reset)::text AS stats_age
FROM pg_replication_slots s
LEFT JOIN pg_stat_replication_slots ss ON s.slot_name = ss.slot_name
ORDER BY "retained,KiB" DESC NULLS LAST
```

В Go это конкатенация строк (как в `bgwriter.go`/`replication.go`). Колонки с double-quote-алиасами
(`"retained,KiB"`, `"safe,KiB"`, `"spill,KiB"`, `"stream,KiB"`, `"total,KiB"`) и `ORDER BY "retained,KiB"`
требуют raw-string-литералов (backtick) для сегментов, содержащих кавычки — см. как это сделано в
`replication.go` (смешивание `"..."` и `` `...` ``).

**Контракт селектора (Decision 4):**
```go
func SelectStatReplicationSlotsQuery(_ int) (string, int, [2]int) {
    return PgStatReplicationSlots, 15, [2]int{6, 13}
}
```
Параметр именно `_` (не `version`), иначе revive флагнет unused parameter. Ncols=15, DiffIntvl=[6,13],
OrderKey=4, UniqueKey=0 — последние два используются в Task 2 при регистрации view, здесь не нужны.

**Dependencies:** нет зависимостей от других задач (wave 1, depends_on пуст). Использует существующие
`query.NewOptions`, `query.Format`, `postgres.NewTestConnectVersion(version)`.

**Edge cases:**
- Физический слот отсутствует в `pg_stat_replication_slots` → LEFT JOIN даёт NULL в восьми diff-колонках.
  `coalesce(...,0)` гарантирует, что они диффятся как `0`, а не падают на `ParseInt("")` (Decision 2).
  В Task 1 это покрывается косвенно (tier-1 толерантен к пустому набору слотов); физический/логический
  слоты проверяются в Task 5.
- retained,KiB / safe,KiB / stats_age — вне `DiffIntvl`, остаются nullable; NULL рендерится пустой
  строкой без краша.
- На тестовом окружении может быть ноль слотов — tier-1 это допускает (проверяется только число
  колонок через `FieldDescriptions()`, не строки).

**Implementation hints:**
- Образец селектора-сигнатуры `(string, int, [2]int)` — `SelectStatBgwriterQuery` в `bgwriter.go`.
- Образец tier-1 теста — `Test_StatBgwriterQueries` в `bgwriter_test.go`: `Format` → `NewTestConnectVersion`
  → `t.Skipf` при недоступности → `conn.Query` → `assert.Len(FieldDescriptions(), wantNcols)` → `rows.Close()`
  → `assert.NoError(rows.Err())`.
- Образец WAL-плейсхолдеров и KiB-алиасов — `PgStatReplicationDefault` в `replication.go`
  (`({{.WalFunction1}}({{.WalFunction2}}(), ...) / 1024)::bigint AS "...,KiB"`).
- `versions := []int{140000, 150000, 160000, 170000, 180000}` — тот же набор, что в `bgwriter_test.go`.
- НЕ добавлять ветвление по версии и НЕ добавлять колонки `conflicting`/`invalidation_reason`
  (явно отклонено в Decision 1 — это ввело бы per-version query strings).

## Reviewers

- **dev-code-reviewer** → `005-feat-replication-slots-task-01-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `005-feat-replication-slots-task-01-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [005-feat-replication-slots-decisions.md](005-feat-replication-slots-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
