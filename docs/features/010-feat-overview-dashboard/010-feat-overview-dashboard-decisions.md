# Decisions Log: Overview Dashboard (verbose mode)

Отчёты агентов о выполнении задач. Каждая запись создаётся агентом, выполнившим задачу.

---

## Task 01: Net-new formatting helpers

**Status:** Done
**Commit:** 43c7229
**Agent:** fmt-dev
**Summary:** Добавлены три чистые функции в `internal/pretty/pretty.go` — `Ceil` (округление вверх через `math.Ceil`), `ReserveWidth` (фиксированная ширина `%*d`, никогда не усекает) и `RateUnit` (динамический суффикс с одношаговым промоушеном единицы при переполнении резерва). Делители: диск `MB/s→GB/s` = 1024 (бинарный, консистентно с `Size`); сеть `Mbps→Gbps` = **1000** — десятичный SI, так как Mbps/Gbps по сетевой конвенции десятичные, а сама величина Mbps в панели nicstat считается из `Rbytes/1024/128` (`top/stat.go:741`), и переход между Mbps↔Gbps — чисто единичный масштаб порядка. `pretty.Size` не тронут.
**Deviations:** Нет. (Шорткаты/отложенные находки: нет; `make lint` не прогнан — golangci-lint/gosec не установлены в окружении, `go vet` и `gofmt` чисты, lint остаётся на CI.)
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 3 minor (опциональные косметики, не применялись) → [010-feat-overview-dashboard-task-01-dev-code-reviewer-round1.json](010-feat-overview-dashboard-task-01-dev-code-reviewer-round1.json)
- dev-test-reviewer: needs_improvement, 2 major + 3 minor → [010-feat-overview-dashboard-task-01-dev-test-reviewer-round1.json](010-feat-overview-dashboard-task-01-dev-test-reviewer-round1.json)

*Round 2 (после исправлений, commit 7b1b72c):*
- dev-test-reviewer: passed, 0 findings → [010-feat-overview-dashboard-task-01-dev-test-reviewer-round2.json](010-feat-overview-dashboard-task-01-dev-test-reviewer-round2.json)

**Verification:**
- `go test ./internal/pretty/...` → ok (TestSize, TestCeil, TestReserveWidth, TestRateUnit, TestRateUnit_boundary, TestRateUnit_property)
- `go build ./...` → OK
- `go vet ./internal/pretty/...` + `gofmt -l` → чисто

---

## Task 03: io.Writer refactor of printSysstat/printPgstat

**Status:** Done
**Commit:** 7762a7e
**Agent:** render-dev
**Summary:** Поведенчески-сохраняющий enabling-рефакторинг: `printSysstat`/`printPgstat` (`top/stat.go`) стали тонкими обёртками над `*gocui.View`, делегирующими в новые `renderSysstat(w io.Writer, …)`/`renderPgstat(w io.Writer, …)` — по образцу `printDbstat → renderDbstat`. Все `fmt.Fprintf`/`Fprintln` перенесены дословно (изменён только получатель `v → w`), форматные строки/порядок аргументов/ANSI-коды/переводы строк не тронуты. Compact-вывод байт-в-байт идентичен, что зафиксировано writer-based golden-тестами (`Test_renderSysstat_compact`/`Test_renderPgstat_compact`) на `bytes.Buffer`. Verbose-строки не добавлялись — это Task 8.
**Deviations:** Нет. (`make lint` не прогнан — golangci-lint в окружении несовместим с конфигом проекта (migration v2); `go vet` и `gofmt -l` чисты, lint остаётся на CI.)
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 1 minor (стилистическая асимметрия `var err error`, унаследована из исходника — оставлена как есть для byte-identical рефакторинга) → [010-feat-overview-dashboard-task-03-dev-code-reviewer-round1.json](010-feat-overview-dashboard-task-03-dev-code-reviewer-round1.json)
- dev-test-reviewer: passed, 1 minor (pgstat line1 — tie-test на `formatInfoString`, намеренно по TDD-anchor; литеральный pinning в `Test_formatInfoString`) → [010-feat-overview-dashboard-task-03-dev-test-reviewer-round1.json](010-feat-overview-dashboard-task-03-dev-test-reviewer-round1.json)

**Verification:**
- `go test ./top/...` → ok (включая новые golden-тесты и существующие)
- `go build ./...` → OK
- `go vet ./top/...` + `gofmt -l top/stat.go top/stat_test.go` → чисто

---

## Task 04: GUC + data_directory reads

**Status:** Done
**Commit:** 8ca1188
**Agent:** guc-dev
**Summary:** `SelectCommonProperties` (`internal/query/common.go`) расширен пятью net-new чтениями в конце SELECT-списка: `max_worker_processes`/`max_logical_replication_workers`/`max_parallel_workers` (`::int`), `wal_segment_size` обёрнут в `pg_size_bytes(current_setting('wal_segment_size'))::int8` для получения int64-байт (приём из `wal.go:6`, а не pretty-строка), и `data_directory` (text). Соответствующие пять полей добавлены в `PostgresProperties` и пять scan-целей — в `.Scan(...)` функции `GetPostgresProperties` (`internal/stat/postgres.go`) строго в lockstep-порядке с SELECT (8 старых + 5 новых = 13). `TestGetPostgresProperties` расширен 4 ассертами; `GucMaxLogicalReplicationWorkers` намеренно не ассертится на значение (дефолт 4, но 0 допустим — важен только успех скана). `GucMaxPrepXacts` (declared-but-never-scanned placeholder) не тронут. `data_directory` нигде не логируется — только пробрасывается в поле структуры.
**Deviations:** `gofmt -w` при выравнивании struct-комментариев также нормализовал две предсуществующие неровные строки комментариев в `Test_parseDuration` (postgres_test.go) — out-of-scope, но это чистая gofmt-нормализация (никакого изменения поведения), оставлена ради gofmt-чистоты файла. `make lint` не прогнан — golangci-lint в окружении — битый симлинк; вместо него прогнаны `gosec` (0 issues), `go vet` и `gofmt -l` (чисто), полный lint остаётся на CI.
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 0 critical/major, 2 minor (информативные, действий не требуют) → [010-feat-overview-dashboard-task-04-dev-code-reviewer-round1.json](010-feat-overview-dashboard-task-04-dev-code-reviewer-round1.json)
- dev-security-auditor: approved, 0 findings (A03 injection — статический const, ввод не интерполируется; data_directory не утекает в логи) → [010-feat-overview-dashboard-task-04-dev-security-auditor-round1.json](010-feat-overview-dashboard-task-04-dev-security-auditor-round1.json)
- dev-test-reviewer: passed, 0 findings (litmus 4/4) → [010-feat-overview-dashboard-task-04-dev-test-reviewer-round1.json](010-feat-overview-dashboard-task-04-dev-test-reviewer-round1.json)

Исправлений не потребовалось — round 2 не запускался.

**Verification:**
- `go test ./internal/query/... ./internal/stat/...` → ok (live-PG кластер доступен)
- `go build ./...` → OK
- `gosec ./internal/query/... ./internal/stat/...` → 0 issues; `go vet` + `gofmt -l` → чисто

---

## Task 02: Verbose toggle plumbing

**Status:** Done
**Commit:** 2af9dd7 (impl) + 389ea24 (round 1 fixes)
**Agent:** toggle-dev
**Summary:** Реализована control-plane обвязка verbose-режима (Decision 2): добавлены `View.Verbose bool` (`internal/view/view.go`) и `config.verbose bool` (`top/config.go`). Новый `top/verbose.go::toggleVerbose(app)` зеркалит флаг по паттерну `showExtra` (write-into-all-views: каждая запись `config.views` + `config.view` + `config.verbose`), пушит вью в `viewCh` и печатает статус через `printCmdline` (`Verbose mode: on/off`). Хоткей `v` навешен на `sysstat` (`top/keybindings.go`), добавлена строка help (`top/help.go`). Ключевая, рисковая часть — в `collectStat()` (`top/stat.go`): seed `prevVerbose := v.Verbose` рядом с `prevCollectExtra`, и ранний `if prevVerbose != v.Verbose { … continue }` в ветке `case v = <-viewCh:` — размещён **до обоих** Reset-путей (условный CollectExtra-Reset и безусловный Reset), так что verbose-only toggle не вайпит prev-снапшот и не блэнкит дельты CPU/mem/load на один кадр. Персистентность через переключение экранов — бесплатно: флаг просто никогда не зануляется в `viewSwitchHandler` (в отличие от `scrollOffset`).
**Deviations:** `make lint` не прогнан — golangci-lint не установлен в окружении (`command not found`); вместо него `go vet ./top/... ./internal/view/...` и `gofmt -l` чисты, полный lint остаётся на CI. Шорткатов и отложенных находок нет.
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 0 critical/major, 2 minor (опциональные) → [010-feat-overview-dashboard-task-02-dev-code-reviewer-round1.json](010-feat-overview-dashboard-task-02-dev-code-reviewer-round1.json)
- dev-test-reviewer: passed, 3 minor (litmus 2/2, ничего блокирующего) → [010-feat-overview-dashboard-task-02-dev-test-reviewer-round1.json](010-feat-overview-dashboard-task-02-dev-test-reviewer-round1.json)

Применены два сходящихся minor-найдинга (commit 389ea24): grouping-комментарий о порядке веток в `collectStat()` (защита load-bearing инварианта от будущих перестановок) и явный ассерт персистентности через реальный `viewSwitchHandler` в `Test_toggleVerbose`. Найдинг про ассерт cmdline-сообщения и про отсутствие изолированного теста ветки `collectStat` — отклонены как scoped (прецеденты `showExtra`/`toggleIdleConns` cmdline не ассертят; у `collectStat` нет изолированного харнесса по спеку, проверка — в ручном QA Final Wave). Round 2 не запускался: оба ревьюера уже approved, правки — некостыльное hardening без изменения поведения.

**Verification:**
- `go test ./top/...` → ok (`Test_toggleVerbose` зелёный, регрессий по keybindings/view-count нет); `go test ./internal/view/...` → ok
- `go build ./...` → OK
- `go vet ./top/... ./internal/view/...` + `gofmt -l` → чисто

---

## Task 05: New aggregate SQL queries + verbose collection

**Status:** Done
**Commit:** ca65a37 (impl) + 70e0256 (round 1 fixes)
**Agent:** agg-dev
**Summary:** Добавлен слой данных для пяти verbose-строк правой панели `pgstat`. Новый `internal/query/overview.go` держит версионно-корректные агрегаты: `workload`/`databases`/`databases-size`/`workers`/`wal-size`/`send-recv` — статический SQL; `replication-lag` и `replication-slots` — рекавери-aware шаблоны (`{{.WalFunction1/2}}`), прогоняются через `query.Format` с `opts` из `NewOptions`. `bgwr/ckpt` переиспользует готовый `SelectStatBgwriterQuery(version)` без новой SQL. Плоская структура `PgstatOverview` + `collectOverviewStat(db, props, itv, prev)` (`internal/stat/postgres.go`) повторяют цепочку `collectActivityStat`: каждый независимый агрегат — собственный `QueryRow`, Go-side rate `(curr-prev)/itv`. tps = `(Δcommit+Δrollback)/itv`; `others` = интервальная дельта `deadlocks+conflicts+checksum_failures` (без `/s`); cache hit ratio — per-interval `Δhit/Δ(hit+read)` через чистый хелпер `cacheHitRatio()` (деление на ноль → `n/a`, не `NaN`). `bgwr/ckpt` сканируется by-name (раскладка колонок различается PG14-16/17/18). Недоступные сигналы (нет репликации, нет слотов, `archive_mode=off`, нехватка прав, первый тик) помечаются sentinel-флагами (`*Valid`/`sql.NullInt64`), отличимыми от реального `0`. Сбор проведён в `Collector.Update` под гейтом `if view.Verbose` — prev читается из `c.currPgStat.Overview` ДО сдвига `prevPgStat = currPgStat`; compact-путь не тронут.

**SECURITY:** дорогие/привилегированные агрегаты — каждый своим `QueryRow`: `sum(pg_database_size)` (`OverviewDatabasesSize`) и archiving backlog (`pg_ls_dir('pg_wal/archive_status')`, требует `pg_monitor`). Ошибка одного (42501 / `archive_mode=off`) деградирует только своё поле в `n/a`, не роняя сэмпл; `collectOverviewStat` вообще не возвращает error, сырой текст ошибки PG (с путями) нигде не пробрасывается и не логируется. SQL — статические `const`-литералы, единственная динамика — подстановка имён WAL-функций по `version`/`recovery` (не пользовательский ввод).

**Deviations:**
1. **`databases` разнесён на два запроса.** User-spec показывает строку `databases` как единое `XGB per Y databases, growth/s, cache hit ratio`, но AC/Decision 6/10 требуют, чтобы дорогой `sum(pg_database_size)` шёл отдельным `QueryRow`. Поэтому добавлены ДВА запроса: `OverviewDatabases` (count + cache-counters, дёшево, всегда заполняется) и `OverviewDatabasesSize` (размер, отдельный `QueryRow`, только он управляет `TotalSizeValid`). Падение size-агрегата теперь не гасит count/cache. Изначально (commit ca65a37) они были склеены — исправлено по critical-находке code-review в round 1.
2. **recovery-`t` тестируется substring-only.** Шаблонные WAL-fn запросы на standby-ветке (`pg_last_wal_receive_lsn`) проверяются только подстановкой через `Format`, без живого исполнения: fixture-кластеры 21914-21918 — primaries, и запуск standby-функции на primary вернёт ошибку. Исполнение — на recovery `f`.
3. **`make lint` не прогнан** — golangci-lint/gosec не установлены в окружении (как в Task 01/02); вместо них `go vet ./internal/query/... ./internal/stat/...` и `gofmt -l` чисты, полный lint остаётся на CI.

**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: changes_required — 1 critical (`pg_database_size` не вынесен в отдельный `QueryRow`), 2 major, 2 minor → [010-feat-overview-dashboard-task-05-dev-code-reviewer-round1.json](010-feat-overview-dashboard-task-05-dev-code-reviewer-round1.json)
- dev-security-auditor: approved — 0 critical/major, 1 minor (осознанное тотальное глушение ошибок ради требования «не светить пути») → [010-feat-overview-dashboard-task-05-dev-security-auditor-round1.json](010-feat-overview-dashboard-task-05-dev-security-auditor-round1.json)
- dev-test-reviewer: needs_improvement — 4 major (Query без Scan; tps/others тавтология; backlog-деградация не проверена через collect; division-by-zero под флакающим `if`), 4 minor → [010-feat-overview-dashboard-task-05-dev-test-reviewer-round1.json](010-feat-overview-dashboard-task-05-dev-test-reviewer-round1.json)

Все critical/major приняты и исправлены (commit 70e0256): split `databases` на два `QueryRow`, чистый хелпер `cacheHitRatio()` + табличный тест на деление на ноль, query-тесты теперь `QueryRow().Scan(...)` в точные приёмники collect-функции, exact-formula ассерт tps/others против синтетического prev (`itv=2`), degradation-тест ассертит backlog/size, `Test_OverviewBgwriterColumns` проверяет by-name колонки на PG14-18, verbose-off проверяет нетронутость compact-пути. Отклонены как scoped: security-minor (осознанный компромисс по спеку), recovery-`t` live-исполнение (fixture-primaries).

*Round 2:*
- dev-code-reviewer: approved — все находки round 1 устранены, регрессий нет → [010-feat-overview-dashboard-task-05-dev-code-reviewer-round2.json](010-feat-overview-dashboard-task-05-dev-code-reviewer-round2.json)
- dev-test-reviewer: passed — все 4 major закрыты, ассерты осмысленные; 2 оставшихся minor (division-by-zero блок под `if` в live-тесте — дублируется безусловным `Test_cacheHitRatio`; recovery-`t` substring-only) не блокируют → [010-feat-overview-dashboard-task-05-dev-test-reviewer-round2.json](010-feat-overview-dashboard-task-05-dev-test-reviewer-round2.json)

Round 3 не запускался: оба ревьюера approved/passed.

**Verification:**
- `go test ./internal/...` → ok (live-PG кластер доступен; новые тесты зелёные на PG 14-18, пути деградации покрыты)
- `go build ./...` → OK
- `go vet ./internal/query/... ./internal/stat/...` + `gofmt -l` → чисто
