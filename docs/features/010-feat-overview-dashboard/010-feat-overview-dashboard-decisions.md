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
