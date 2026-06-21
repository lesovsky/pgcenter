# Decisions Log: pg_stat_io screen

Отчёты агентов о выполнении задач. Каждая запись создаётся агентом, выполнившим задачу.

---

## Task 01: Query layer — internal/query/io.go + version constants

**Status:** Done
**Commit:** e1888fc
**Agent:** io-query-dev (general-purpose)
**Summary:** Добавлены константы `PostgresV15/16/17/18` в `query.go`; создан `internal/query/io.go` с тремя SQL-константами (`PgStatIOPG16`, `PgStatIOPG18`, `PgStatIOTime`) и двумя селекторами (`SelectStatIOQuery` ветвится на `>= PostgresV18`; `SelectStatIOTimeQuery` версионно-независим). Точные раскладки Data Models (count 16/[4,14], time 10/[4,8]), синтетический `io_key` с coalesce внутри md5, `coalesce(...,0)` на всех 11 diff-колонках, KiB через integer `/1024`, общий count-based `WHERE` на обоих экранах. Покрыто unit (форма per-version, NULL-safety структурно) + live integration (`t.Skipf`).
**Deviations:** Time-селектор сделан версионно-независимым (`_ int`, как `SelectStatReplicationSlotsQuery`) — набор timing-колонок идентичен на PG16/17/18; tech-spec это явно разрешал. Count и time ветвятся асимметрично (документировано в doc-комментах для Wave 2). Live-пути PG16 (op_bytes) и PG18 (нативные bytes + `object='wal'`) локально не исполнялись (нет PG-кластера в окружении) — гейтятся CI-матрицей PG14–18.
**Tech debt:** Поведенческий NULL-тест отсутствует: package `query` не может импортировать `stat` (import cycle), поэтому NULL-safety проверена структурно (coalesce в SQL-строке). Рекомендован follow-up — behavioral-тест в `internal/stat/postgres_test.go`, что `diff()` переживает пустую diff-ячейку (вторая половина контракта Decision 5), плюс live-ассерт уникальности/non-NULL `io_key` (Decision 2). Low priority, вне scope Task 01.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 0 critical/major, 3 minor (санкционированы спекой) → [task-01-dev-code-reviewer-round1.json]
- dev-security-auditor: approved, 0 findings → [task-01-dev-security-auditor-round1.json]
- dev-test-reviewer: passed, 0 critical/major, 4 minor + follow-up tech-debt → [task-01-dev-test-reviewer-round1.json]

**Verification:**
- `go test ./internal/query/...` → ok (live PG-тесты t.Skipf — кластер недоступен локально)
- `go vet ./internal/query/...` → clean; `gofmt -l internal/query/io.go io_test.go` → clean
- Независимое подтверждение lead'ом: commit e1888fc содержит 3 файла (+290), сборка зелёная

---

## Task 02: View registration — internal/view/view.go

**Status:** Done
**Commit:** 3a8ddf8 (impl) + ad55bfd (review fix)
**Agent:** io-view-dev (general-purpose)
**Summary:** Зарегистрированы два view в `view.New()` — `stat_io` (Ncols 16, DiffIntvl [4,14]) и `stat_io_time` (Ncols 10, DiffIntvl [4,8]); оба `UniqueKey 0`, `OrderKey 4`, `OrderDesc true`, `NotRecordable true`, `MinRequiredVersion query.PostgresV16`. В `Configure()` добавлены два case, вызывающие `SelectStatIOQuery`/`SelectStatIOTimeQuery`. time-view Msg несёт подсказку про `track_io_timing` (Decision 9). `TestNew` 24→26, добавлены guard-тесты обоих view + строка `{160000,26}` в `TestView_VersionOK`.
**Deviations:** Нет (Msg-формат выбран под общий стиль «Show … statistics»).
**Tech debt:** Нет. Замечание test-reviewer (assert `QueryTmpl==PgStatIOPG18` на view-слое для PG18) принято как уже покрытое на query-уровне — Task 01 NULL-safety тест ассертит `read_bytes` на v=180000 vs `op_bytes` на v<18, branch-selection проверена там, где живёт. View-слой лишь вызывает селектор. Опциональное усиление, не долг.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 0 critical/major, 3 info-minor (грамматика Msg исправлена в ad55bfd) → [task-02-dev-code-reviewer-round1.json]
- dev-test-reviewer: passed, 0 critical/major, 2 minor (PG18 view-assert — покрыто на query-уровне; {150000} row — low prio) → [task-02-dev-test-reviewer-round1.json]

**Verification:**
- `go test ./internal/view/...` → ok; `go build ./...` → clean; `gofmt` → clean
- Независимое подтверждение lead'ом: commit 3a8ddf8 (2 файла), build всего проекта зелёный, TestNew 24→26 подтверждён
