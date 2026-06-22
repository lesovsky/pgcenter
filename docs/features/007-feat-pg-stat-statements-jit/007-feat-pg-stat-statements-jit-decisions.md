# Decisions Log: pg_stat_statements JIT screen

Отчёты агентов о выполнении задач. Каждая запись создаётся агентом, выполнившим задачу.

---

## Task 01: JIT query consts + version selector — internal/query/statements.go

**Status:** Done
**Commit:** 79514ea
**Agent:** jit-query-dev (general-purpose)
**Summary:** Добавлены две SQL-константы по образцу `PgStatStatementsTimingPG13`/`PgStatStatementsIoDefault` — `PgStatStatementsJITPG15` (PG15/16, 13 колонок) и `PgStatStatementsJITDefault` (PG17+, 15 колонок, +`deform_total`/`deform,ms` через `jit_deform_time`); `*_total` через `date_trunc('seconds', round(p.jit_*_time)/1000 * '1 second'::interval)::text`, `*_ms` через `round(p.jit_*_time)`, `functions` = `p.jit_functions`, md5-`queryid`, оба заканчиваются `WHERE p.jit_functions > 0` (Decision 3). Добавлен селектор `SelectStatStatementsJITQuery(version int) (string, int, [2]int, int)` (4-tuple по модели `SelectStatIOQuery` + `UniqueKey`): `>= PostgresV17` → `(Default, 15, {7,12}, 13)`, иначе → `(PG15, 13, {6,10}, 11)`. Покрыто unit-тестом обеих веток + JIT exec-подтест gated PG15+ (`t.Skipf` без PG).
**Deviations:** Нет. Колоночные алиасы, индексы, токены и стиль строки точно соответствуют tech-spec (Decision 2) и образцовым timing/io-константам.
**Tech debt:** Нет. Двойная константа (отличаются только deform-колонками) — намеренная по позиционному align (ADR [006]), флаг code-reviewer'а только чтобы будущий рефактор случайно не слил их в одну строку. Live exec-пути PG15–18 локально не исполнялись (нет тестового PG-кластера) — гейтятся CI-матрицей; подтверждено `--- SKIP` на всех четырёх версиях.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 0 critical/major, 1 minor (намеренное дублирование консты — не менять) → [task-01-dev-code-reviewer-review.json]
- dev-security-auditor: approved, 0 findings (статические шаблоны над server-controlled токенами, нет injection-поверхности) → [task-01-dev-security-auditor-review.json]
- dev-test-reviewer: passed, 0 critical/major, 1 minor (нет проверки границы PG<15 — by design, ветка отсутствует) → [task-01-dev-test-reviewer-review.json]

**Verification:**
- `go test ./internal/query/...` → ok (JIT exec-подтесты `t.Skipf` — PG-кластер недоступен локально; `TestSelectStatStatementsJITQuery` зелёный)
- `go build ./...` → clean; `gofmt -l` → clean; `go vet ./internal/query/...` → clean
- Commit 79514ea: 2 файла, +81 строка

---
