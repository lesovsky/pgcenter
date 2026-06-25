---
status: done                    # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — go test ./internal/query/... ./internal/stat/...
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
teammate_name:
---

# Task 04: GUC + data_directory reads

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Фича 010 (verbose-режим верхних панелей) добавляет строки workers / replication-archiving / filesyst,
которым нужны пять серверных настроек (GUC) и путь к каталогу данных, которые сейчас **не читаются** в
`PostgresProperties`. Эта задача расширяет общий запрос свойств инстанса `SelectCommonProperties` пятью
новыми значениями и пробрасывает их в структуру `PostgresProperties` через скан в `GetPostgresProperties`.

Новые значения (все подтверждены как net-new, нигде в `PostgresProperties` сегодня не читаются):
- `max_worker_processes` (int) — зонтичный лимит для строки workers.
- `max_logical_replication_workers` (int) — для строки workers.
- `max_parallel_workers` (int) — для строки workers.
- `wal_segment_size` (int64, в байтах) — множитель для archiving backlog (`count(.ready) × wal_segment_size`).
- `data_directory` (string) — путь к каталогу данных для строки filesyst (выбор ФС по longest mount-prefix).

Ключевой риск задачи — **scan-arity**: `GetPostgresProperties` сканирует результат `SelectCommonProperties`
позиционно. Если добавить колонки в SELECT, но не добавить соответствующие `&props.*` цели в `.Scan(...)`
(или наоборот), упадут live-PG тесты `Test_CommonQueries` / `TestGetPostgresProperties`. SELECT, поля
структуры и `.Scan(...)` должны меняться **в едином порядке (lockstep)** — по 3 правки на каждое значение.

Это самодостаточная подготовительная задача. Сами строки verbose-панелей (потребители этих полей)
реализуются в Task 5 и Task 8 — здесь только чтение данных, без рендера.

## What to do

1. Расширить константу `SelectCommonProperties` (`internal/query/common.go`) пятью новыми выражениями
   `current_setting('X')` — добавить **в конец** SELECT-списка, сохраняя существующий порядок колонок:
   - `current_setting('max_worker_processes')::int`
   - `current_setting('max_logical_replication_workers')::int`
   - `current_setting('max_parallel_workers')::int`
   - `pg_size_bytes(current_setting('wal_segment_size'))::int8` (вернуть размер сегмента WAL в байтах —
     `wal_segment_size` отдаётся как pretty-строка вида `16MB`; это в точности приём из `wal.go:6`)
   - `current_setting('data_directory')`
2. Добавить пять полей в структуру `PostgresProperties` (`internal/stat/postgres.go`) в том же порядке:
   `GucMaxWorkerProcesses int`, `GucMaxLogicalReplicationWorkers int`, `GucMaxParallelWorkers int`,
   `GucWalSegmentSize int64`, `DataDirectory string`. Снабдить каждое поле комментарием в стиле соседних полей.
3. Добавить пять целей скана в `.Scan(...)` внутри `GetPostgresProperties` (`internal/stat/postgres.go`) —
   **строго в том же позиционном порядке**, что и колонки SELECT: `&props.GucMaxWorkerProcesses`,
   `&props.GucMaxLogicalReplicationWorkers`, `&props.GucMaxParallelWorkers`, `&props.GucWalSegmentSize`,
   `&props.DataDirectory`.
4. Расширить существующие тесты для покрытия новых полей (TDD Anchor ниже): добавить assert-ы в
   `TestGetPostgresProperties` (`internal/stat/postgres_test.go`). `Test_CommonQueries`
   (`internal/query/common_test.go`) уже исполняет `SelectCommonProperties` против живых PG-версий с
   `t.Skipf` — добавление колонок оставляет его валидным без правок; править его не нужно, но убедиться,
   что он зелёный.

## TDD Anchor

Тесты пишем/расширяем ДО реализации — это live-PG тесты с `t.Skipf` guard (запускаются при доступном
тестовом кластере, см. tech-debt [005]/[008]).

- `internal/stat/postgres_test.go::TestGetPostgresProperties` — расширить: после успешного
  `GetPostgresProperties(conn)` проверить, что новые поля заполнены —
  `assert.NotEqual(t, 0, got.GucMaxWorkerProcesses)`, `assert.NotEqual(t, 0, got.GucMaxParallelWorkers)`,
  `assert.NotEqual(t, int64(0), got.GucWalSegmentSize)`, `assert.NotEqual(t, "", got.DataDirectory)`.
  (`max_logical_replication_workers` имеет дефолт 4, но допустимо 0 — проверять только что скан не упал.)
- `internal/query/common_test.go::Test_CommonQueries` — НЕ требует правок: уже исполняет
  `SelectCommonProperties`; после добавления колонок остаётся зелёным (проверить запуском). Падение здесь
  при реализации = индикатор scan-arity рассинхрона между SELECT и `.Scan(...)`.

## Acceptance Criteria

- [ ] `SelectCommonProperties` возвращает 5 новых колонок в порядке: max_worker_processes,
      max_logical_replication_workers, max_parallel_workers, wal_segment_size (в байтах), data_directory.
- [ ] `PostgresProperties` содержит 5 новых полей с корректными типами (`int`, `int`, `int`, `int64`, `string`).
- [ ] `.Scan(...)` в `GetPostgresProperties` дополнен 5 целями в том же позиционном порядке, что и SELECT.
- [ ] `wal_segment_size` читается как int64 байт (через `pg_size_bytes`), а не как pretty-строка.
- [ ] `go test ./internal/query/... ./internal/stat/...` зелёный (live-PG тесты — при доступном кластере;
      иначе `t.Skipf`, не panic).
- [ ] Нет регрессий по существующим скан-целям `GetPostgresProperties` (порядок прежних колонок не нарушен).
- [ ] `make lint` (golangci-lint + gosec) чистый.

## Context Files

**Feature artifacts:**
- [010-feat-overview-dashboard.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard.md) — user-spec
- [010-feat-overview-dashboard-tech-spec.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-tech-spec.md) — tech-spec (Task 4; Decisions 6 archiving, 7 filesyst)
- [010-feat-overview-dashboard-decisions.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-decisions.md) — decisions log
- [010-feat-overview-dashboard-code-research.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-code-research.md) — §6-new (add-pattern, 3 edits each), §7-new (which tests change)

**Project knowledge:**
- [project.md](.claude/skills/project-knowledge/overview.md) — обзор проекта (файл называется overview.md)
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — раскладка пакетов, поток данных, обработка версий PG
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — конвенции тестирования, version-branching, error wrapping

**Code files:**
- [internal/query/common.go](internal/query/common.go) — расширить `SelectCommonProperties` (строки 43-50)
- [internal/stat/postgres.go](internal/stat/postgres.go) — добавить поля в `PostgresProperties` (107-120) и цели в `.Scan(...)` в `GetPostgresProperties` (125-134)
- [internal/query/common_test.go](internal/query/common_test.go) — `Test_CommonQueries` (63-108), правок не требует
- [internal/stat/postgres_test.go](internal/stat/postgres_test.go) — расширить `TestGetPostgresProperties` (88-110)
- [internal/query/wal.go](internal/query/wal.go) — прецедент `pg_size_bytes(current_setting('wal_segment_size'))` (строка 6)

## Verification Steps

- Запустить `go test ./internal/query/... ./internal/stat/...` — все тесты зелёные (live-PG: при наличии
  кластера выполняются, иначе `t.Skipf`).
- Убедиться, что `Test_CommonQueries` и `TestGetPostgresProperties` не падают на scan-arity (рассинхрон
  числа колонок SELECT и целей `.Scan`).
- Запустить `make build` и `make lint` — компиляция и линт чистые.

## Details

**Files:**
- `internal/query/common.go` — константа `SelectCommonProperties` (строки 43-50). Сейчас 8 колонок
  (`version`, `version_num`, `track_commit_timestamp`, `max_connections`, `autovacuum_max_workers`,
  `shared_preload_libraries`, `recovery`, `start_time_unix`). Добавить 5 выражений `current_setting(...)`
  в конец, после `extract(epoch from pg_postmaster_start_time()) AS start_time_unix`. Внимание к
  конкатенации строк: каждая колонка — отдельный фрагмент через `+`; следить за запятыми/пробелами между
  колонками (строка 48 склеена без пробела перед `pg_is_in_recovery` — это уже работает; новые фрагменты
  добавляй с явными разделителями `", "`).
- `internal/stat/postgres.go` — структура `PostgresProperties` (107-120): добавить 5 полей после
  существующих GUC-полей (рядом с `GucMaxPrepXacts`/`GucSharedPreLibraries`). Функция
  `GetPostgresProperties` (123-156): дополнить `.Scan(...)` (125-134) 5 целями в том же порядке.

**Dependencies:** нет зависимостей от других задач (Wave 1, depends_on: []). Потребители новых полей —
Task 5 (archiving backlog, workers) и Task 8 (filesyst) — отдельные задачи, здесь только чтение.

**Edge cases:**
- `wal_segment_size` отдаётся `current_setting` как pretty-строка (`16MB`); чтобы получить int64 байт,
  оборачивать в `pg_size_bytes(...)::int8` (приём из `wal.go:6`). Скан в `int64`, не в строку.
- `max_logical_replication_workers` имеет ненулевой дефолт (обычно 4), но в тесте не ассертить конкретное
  значение — допустимо 0; проверять только что скан прошёл без ошибки.
- `data_directory` доступен суперпользователю / роли `pg_read_all_settings`; в тестовом окружении читается.
  Не логировать значение (путь — потенциально чувствительная инфраструктурная деталь) — просто пробросить
  в поле структуры.
- Существующее поле `GucMaxPrepXacts` (postgres.go:115) объявлено, но не сканируется (declared-but-never-
  scanned placeholder) — **не трогать** его, не пытаться «починить»: вне scope этой задачи.

**Implementation hints:**
- Add-pattern из code-research §6-new: ровно 3 правки на каждое значение — (1) колонка в SELECT,
  (2) поле в `PostgresProperties`, (3) цель в `.Scan(...)`. Порядок всех трёх списков должен совпадать.
- Типы скана pgx/v5: `::int` → Go `int`, `::int8` → Go `int64`, text → `string`.
- Прецедент `pg_size_bytes(current_setting('wal_segment_size'))` уже используется в `internal/query/wal.go`
  (строка 6) — копировать ровно это выражение.
- Не добавлять version-dispatch: все 5 GUC существуют на поддерживаемых версиях PG 14-18 (и ниже);
  `SelectCommonProperties` — единый запрос без version-split.

## Reviewers

- **dev-code-reviewer** → `docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-task-04-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-task-04-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-task-04-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [010-feat-overview-dashboard-decisions.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
