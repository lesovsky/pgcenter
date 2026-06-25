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
