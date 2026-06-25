---
status: planned                    # planned -> in_progress -> done
depends_on: ["04"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 2                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 05: New aggregate SQL queries + collection

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Добавляем слой данных для пяти verbose-строк правой панели `pgstat` (workload, databases, workers,
replication, bgwr/ckpt). Это **только данные** — SQL-запросы, структуры, Go-side подсчёт rate'ов против
prev-снимка и проводка сбора в `Collector.Update`, гейтящаяся флагом `view.Verbose`. Рендер строк (Task 8) и
tiering/guard (Task 9) — в следующих волнах; здесь сбор работает каждый тик (throttle добавится позже),
данные кладутся в `currPgStat`/новую под-структуру, откуда их потом читает рендер.

Половина метрик — net-new агрегаты (`SELECT sum(...) FROM pg_stat_database`, `count(.ready) × wal_segment_size`,
`count(*) FILTER (backend_type ...)`), половина — лифт существующих выражений (waldir-subselect, lag-diff,
retained-WAL). bgwr/ckpt переиспользует готовый версионный `SelectStatBgwriterQuery(version)` без новой
SQL. Все агрегаты — single-row, собираются по образцу цепочки `collectActivityStat`
(`internal/stat/postgres.go:56-104`): диспетчер версии → `QueryRow().Scan(...)` в плоскую структуру →
Go-side rate `(curr - prev) / itv`.

**Ключевое требование безопасности и устойчивости.** Дорогие/привилегированные агрегаты (в частности
archiving backlog через `pg_ls_dir('pg_wal/archive_status')`, требующий `pg_monitor`/superuser, и sum размеров
БД) выполняются **каждый своим отдельным `QueryRow`**, а не в общем сканировании, где первая ошибка
прерывает весь сбор. Привилегия (`42501`), `archive_mode=off`, отсутствие репликации или standby деградируют
**только одну строку до `n/a`**, не роняя весь сэмпл; сырой текст ошибки PG (содержащий пути) **никогда** не
всплывает наружу и не логируется.

Зависит от Task 4: GUC'и `GucMaxWorkerProcesses`, `GucMaxLogicalReplicationWorkers`, `GucMaxParallelWorkers`,
`GucWalSegmentSize` уже добавлены в `PostgresProperties` и читаются `GetPostgresProperties` — этот таск их
потребляет (лимиты воркеров, множитель backlog).

## What to do

1. Создать новый файл `internal/query/overview.go` с константами запросов и диспетчер-функциями для
   verbose-агрегатов pgstat. Запросы, использующие `{{.WalFunction1}}`/`{{.WalFunction2}}`, — это шаблоны и
   обязаны проходить через `query.Format(tmpl, opts)` (как replication/wal); рекавери-aware имена WAL-функций
   берутся из `selectWalFunctions` через `NewOptions`.
   - **workload** — `SELECT sum(xact_commit), sum(xact_rollback), sum(tup_inserted), sum(tup_updated),
     sum(tup_deleted), sum(tup_returned), sum(temp_files), sum(deadlocks), sum(conflicts),
     sum(checksum_failures) FROM pg_stat_database` (имена колонок — из `databases.go:5-16`). `tps` =
     `commit + rollback`; `ins/upd/del/ret/tmp` — per-second rate; `others` = `deadlocks + conflicts +
     checksum_failures` **за интервал** (значение, без `/s`).
   - **databases** — `SELECT count(*), sum(pg_database_size(datname)), sum(blks_hit), sum(blks_read)
     FROM pg_stat_database WHERE datname IS NOT NULL` (или эквивалент). `growth/s` = Go-side дельта суммарного
     размера; `cache hit ratio` = **per-interval** `Δhit / Δ(hit + read)` (кумулятив дал бы вечные ~99.9%).
   - **workers** — `count(*) FILTER (WHERE backend_type = 'parallel worker')`,
     `count(*) FILTER (WHERE backend_type = 'logical replication worker')`, и общая занятость зонтика
     `max_worker_processes` (активные слоты) из `pg_stat_activity` по `backend_type` (идиома `common.go:61`).
     Лимиты — из новых GUC.
   - **replication** — собрать из нескольких источников: `wal size` (лифт waldir-subselect `wal.go:6`),
     `lag` bytes worst-case (`max` от `{{.WalFunction1}}({{.WalFunction2}}(), replay_lsn)` по
     `pg_stat_replication`, `replication.go:5-15`), `slots/retain` (`count(*)` + `max(retained)` через
     `{{.WalFunction1}}({{.WalFunction2}}(), s.restart_lsn)`, `replication_slots.go:14-31`),
     **archiving backlog** = `count(*) * pg_size_bytes(current_setting('wal_segment_size'))` по
     `pg_ls_dir('pg_wal/archive_status')` с фильтром на суффикс `.ready` (адаптация точного прецедента
     `wal.go:6` `count(1) * pg_size_bytes(current_setting('wal_segment_size'))`), `send/recv` =
     `count(*) FILTER (backend_type LIKE 'walsender%')` / `= 'walreceiver'`.
   - **bgwr/ckpt** — **не** писать новую SQL: вызвать `query.SelectStatBgwriterQuery(version)` и сканировать
     нужные колонки (`ckpt_timed`/`ckpt_req` — absolute; `ckpt_write,ms`/`ckpt_sync,ms` — delta; `maxwritten`),
     учитывая разную раскладку колонок для PG14-16 / 17 / 18.

2. Добавить плоскую структуру (или несколько) под verbose-агрегаты — расширить `Activity` либо завести новую
   под-структуру `Pgstat` (например `PgstatOverview`) в `internal/stat/postgres.go`. Поля — абсолютные
   значения (для rate против prev) и/или уже посчитанные rate'ы, по образцу `Activity.Calls`/`CallsRate`.
   Для недоступных сигналов — sentinel-поле (например `sql.NullInt64`/доступность-флаг), отличимое от `0`.

3. Добавить collect-функцию(и) в `internal/stat/postgres.go` (например `collectOverviewStat(db, props, itv,
   prev ...)`) по образцу `collectActivityStat`: диспетчер версии → отдельные `QueryRow().Scan(...)` на каждый
   независимый агрегат → Go-side rate `(curr - prev) / itv` и per-interval cache-hit ratio. Дорогие/привилеги-
   рованные агрегаты (archiving backlog, sum размеров БД) — **каждый своим `QueryRow`**; ошибка на одном →
   sentinel `n/a` для этой строки, не возврат ошибки из всей функции.

4. Провести вызов сбора в `Collector.Update` (`internal/stat/stat.go`), **гейтя на `view.Verbose`** (этот флаг
   приезжает с view и в Task 2 добавлен на `view.View`). Класть результат в `currPgStat` (расширив `Pgstat`
   или добавив поле на `Collector`), снимок prev обновляется в том же месте, где уже идёт
   `c.prevPgStat = c.currPgStat`. Сбор не должен затрагивать compact-путь (когда `view.Verbose == false` —
   ничего не собираем).

5. Написать live-PG тесты (`internal/query/overview_test.go`, дополнения в `internal/stat/postgres_test.go`)
   по паттерну `t.Skipf` через PG 14-18, включая пути деградации.

## TDD Anchor

Тесты пишем ДО реализации. Все интеграционные тесты используют guard `t.Skipf` (не panic, см. tech-debt
[005]/[008]) и `postgres.NewTestConnectVersion(version)` по списку версий `[140000, 150000, 160000, 170000,
180000]` (паттерн `internal/query/common_test.go:63-108`).

- `internal/query/overview_test.go::Test_OverviewQueries` — каждый новый агрегат (workload / databases /
  workers / replication-wal-size / replication-lag / replication-slots / send-recv) исполняется и сканируется
  на PG 14-18; шаблонные (WAL-fn) запросы прогоняются через `Format` с recovery `f` и `t`.
- `internal/query/overview_test.go::Test_ArchivingBacklogQuery_Degrades` — archiving backlog запрос: на
  `archive_mode=off`/без прав возвращает `n/a`-путь (или ошибку, которую collect ловит) — не падает, не светит
  сырой текст PG.
- `internal/stat/postgres_test.go::Test_collectOverviewStat` — collect-функция на живом PG: заполняет структуру,
  считает per-interval cache-hit ratio (`Δhit/Δ(hit+read)`), tps = commit+rollback, `others` за интервал;
  первый тик (prev пустой) → дельты не паникуют.
- `internal/stat/postgres_test.go::Test_collectOverviewStat_Degradation` — отсутствие репликации / standby /
  `archive_mode=off`: соответствующие поля деградируют в `n/a`, остальные строки заполнены (одна упавшая строка
  не гасит сэмпл).
- `internal/stat/stat_test.go::TestCollector_Update_VerboseAggregates` — при `view.Verbose=true` overview-агрегаты
  собираются; при `false` — нет (compact-путь не трогается).

## Acceptance Criteria

- [ ] Новый файл `internal/query/overview.go` содержит версионно-корректные агрегатные запросы для workload,
      databases, workers, replication (wal size / lag / slots-retain / archiving backlog / send-recv); WAL-fn
      запросы проходят через `Format` и рекавери-aware.
- [ ] bgwr/ckpt переиспользует `SelectStatBgwriterQuery(version)` (PG14-16 / 17 / 18); новой SQL для bgwr/ckpt
      не добавлено; timed/req — absolute, write/sync ms — delta, плюс maxwritten.
- [ ] `workload`: tps = `commit + rollback`/s; ins/upd/del/ret/tmp — per-second; `others` =
      `deadlocks + conflicts + checksum_failures` **за интервал** (без `/s`).
- [ ] `databases`: cache hit ratio считается **per-interval** (`Δhit/Δ(hit+read)`), не кумулятив; growth/s —
      Go-side дельта суммарного `pg_database_size`.
- [ ] `replication.archiving backlog` = `count(*.ready) × wal_segment_size`; деградирует в `n/a` на
      `archive_mode=off`/нехватке прав (`42501`), не роняя сэмпл.
- [ ] Дорогие/привилегированные агрегаты (archiving backlog, sum размеров БД) выполняются каждый отдельным
      `QueryRow`; падение одного → `n/a` этой строки, остальные строки заполнены.
- [ ] Сырой текст ошибки PG (с путями) не всплывает наружу и не логируется; недоступный сигнал → литерал-маркер
      `n/a`, отличимый от `0`.
- [ ] Сбор overview-агрегатов в `Collector.Update` гейтится `view.Verbose`; при `false` — не собираются,
      compact-путь не изменён.
- [ ] Go-side rate'ы считаются против prev-снимка и `itv`; первый тик (нет prev) не паникует; деление на ноль в
      cache-hit ratio → `n/a`, не `NaN`/паника.
- [ ] Live-PG тесты зелёные на PG 14-18 (через `t.Skipf`), включая пути деградации; `go test ./internal/...`
      проходит.

## Context Files

**Feature artifacts:**
- [010-feat-overview-dashboard.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard.md) — user-spec (см. «Состав и источники строк»: точная раскладка полей)
- [010-feat-overview-dashboard-tech-spec.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-tech-spec.md) — tech-spec (Task 5, Decisions 6 и 10)
- [010-feat-overview-dashboard-code-research.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-code-research.md) — code-research (§6-new: таблица per-row EXISTS/liftable/NET-NEW)
- [010-feat-overview-dashboard-decisions.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — фичи, поддерживаемые статистики
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow, PG version handling
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — code patterns, testing conventions, version branching

**Code files (modify):**
- [internal/query/common.go](internal/query/common.go) — добавить/обновить общие константы и диспетчеры запросов при необходимости
- [internal/query/overview.go](internal/query/overview.go) — НОВЫЙ файл: verbose-агрегатные запросы + диспетчеры
- [internal/stat/postgres.go](internal/stat/postgres.go) — структуры под overview + collect-функция (образец `collectActivityStat`)
- [internal/stat/stat.go](internal/stat/stat.go) — проводка сбора в `Collector.Update`, гейт на `view.Verbose`

**Code files (read):**
- [internal/query/databases.go](internal/query/databases.go) — имена колонок `pg_stat_database` для sum-агрегатов
- [internal/query/replication.go](internal/query/replication.go) — lag-diff выражение, WAL-fn шаблоны
- [internal/query/replication_slots.go](internal/query/replication_slots.go) — retained-WAL выражение
- [internal/query/wal.go](internal/query/wal.go) — waldir-subselect + прецедент `count(1) * pg_size_bytes(...)` для backlog
- [internal/query/bgwriter.go](internal/query/bgwriter.go) — `SelectStatBgwriterQuery` (PG14-16/17/18), раскладка колонок
- [internal/query/query.go](internal/query/query.go) — `NewOptions`, `selectWalFunctions`, `Format`, версии-константы
- [internal/stat/postgres.go](internal/stat/postgres.go) — образец цепочки сбора и Go-side rate

## Verification Steps

- Запустить `go test ./internal/...` — все юнит/интеграционные тесты зелёные (live-PG тесты сами скипаются
  через `t.Skipf`, если кластер недоступен; при доступном кластере — выполняются и проходят на PG 14-18).
- Убедиться, что новые live-PG тесты используют `t.Skipf` (не panic) и покрывают пути деградации
  (`archive_mode=off`, нет репликации, нехватка прав).
- `make lint` (golangci-lint + gosec) — чисто; gosec не должен флагать новый SQL/обход директорий (запросы —
  чистый SQL, без конкатенации пользовательского ввода).

## Details

**Files:**
- `internal/query/overview.go` (НОВЫЙ) — константы запросов + диспетчер-функции по образцу
  `SelectStatBgwriterQuery`/`SelectStatReplicationQuery` (возвращают строку, при необходимости версионную).
  Шаблонные запросы (WAL-fn) — через `Format`.
- `internal/query/common.go` — при необходимости общие/версионно-независимые константы; основной объём запросов
  держать в `overview.go`.
- `internal/stat/postgres.go` — новая плоская структура(ы) под overview + `collectOverviewStat(...)` по образцу
  `collectActivityStat` (`:56-104`); Go-side rate `(curr - prev) / itv`, per-interval cache-hit ratio.
- `internal/stat/stat.go` — вызов `collectOverviewStat` в `Update`, гейт `if view.Verbose { ... }`; хранение
  результата в `currPgStat` (расширить `Pgstat`) и обновление prev там же, где `c.prevPgStat = c.currPgStat`.

**Dependencies:**
- Task 4 (wave 1) — GUC'и `GucMaxWorkerProcesses` / `GucMaxLogicalReplicationWorkers` / `GucMaxParallelWorkers`
  / `GucWalSegmentSize` уже на `PostgresProperties` (читаются `GetPostgresProperties`). Этот таск их потребляет.
- Task 2 (wave 1) — `view.View.Verbose bool` уже существует (флаг гейта сбора).
- Только stdlib + существующие пакеты (`internal/query`, `internal/postgres`). Новых зависимостей нет.

**Edge cases:**
- Первый тик (нет prev) → дельты/rate не паникуют; rate либо `0`, либо помечается так, чтобы Task 8 показал
  `n/a` (sentinel, отличимый от реального `0`).
- `archive_mode=off` / `pg_wal/archive_status` недоступен / нет прав (`42501`) → archiving backlog → `n/a`,
  отдельный `QueryRow`, сэмпл не падает.
- Standby / нет репликации → lag/slots/send/recv пустые/`n/a`; WAL/lag рекавери-aware через `selectWalFunctions`
  (на standby — `pg_last_wal_receive_lsn`).
- Версии PG: bgwr/ckpt раскладка колонок различается PG14-16 vs 17 vs 18 — сканировать ровно по той раскладке,
  что вернул `SelectStatBgwriterQuery`. `pg_stat_database.checksum_failures`/`temp_files`/`conflicts`/`deadlocks`
  доступны на всех поддерживаемых версиях.
- Деление на ноль в cache-hit ratio (`Δ(hit+read) == 0`) → `n/a`/sentinel, не паника, не `NaN`.

**Implementation hints (НЕ псевдокод):**
- Образец цепочки сбора и Go-side rate — `collectActivityStat` (`internal/stat/postgres.go:56-104`), строка
  `s.CallsRate = (s.Calls - prevCalls) / itv` (`:94`). Снимок prev — поле на `Collector` (`prevPgStat`).
- WAL-fn запросы ОБЯЗАТЕЛЬНО через `query.Format(tmpl, opts)` с `opts` из `query.NewOptions(version, recovery,
  track, querylen, pgssSchema)` — иначе `{{.WalFunction1/2}}` не подставятся (см. replication/wal).
- archiving backlog адаптирует точный прецедент `wal.go:6`:
  `count(1) * pg_size_bytes(current_setting('wal_segment_size'))`, заменяя источник `pg_ls_waldir()` на
  `pg_ls_dir('pg_wal/archive_status')` с фильтром на суффикс `.ready`.
- backend_type-фильтры — идиома `count(*) FILTER (WHERE backend_type ...)` уже в `SelectActivityDefault`
  (`common.go:54-61`).
- Маркер `n/a` — sentinel-значение в структуре (например `sql.NullInt64`/`sql.NullString` с `.Valid=false` или
  выделенный bool-флаг доступности на поле), который Task 8 рендерит как литерал `n/a`. Не использовать `0`/`-1`
  как «недоступно».
- SECURITY: каждый привилегированный/дорогой агрегат — собственный `db.QueryRow(...).Scan(...)`; на ошибке —
  пометить поле недоступным и продолжить; **не** возвращать ошибку наверх (не как первый-scan-error-аборт), и
  **не** логировать/пробрасывать `err.Error()` (содержит пути/имена объектов PG).

## Reviewers

- **dev-code-reviewer** → `docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-task-05-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-task-05-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-task-05-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [010-feat-overview-dashboard-decisions.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
