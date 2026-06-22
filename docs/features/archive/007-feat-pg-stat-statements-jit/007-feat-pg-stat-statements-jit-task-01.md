---
status: done                       # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — go test ./internal/query/...
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
teammate_name:
---

# Task 01: JIT query consts + version selector

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Добавить в `internal/query/statements.go` два версионно-зависимых query-конста для нового
sub-screen `statements_jit` (JIT-компиляция в `pg_stat_statements`), плюс селектор
`SelectStatStatementsJITQuery(version)`, возвращающий query + `Ncols` + `DiffIntvl` +
`UniqueKey`.

Это фундамент фичи (Wave 1, без зависимостей) и единственный слой, где живёт разница колонок
между PG15/16 и PG17+. PG15/16 показывает 8 JIT-метрик (база), PG17+ добавляет
`jit_deform_count`/`jit_deform_time`. Из-за синтетического md5-`queryid` как `UniqueKey`
колонки нельзя скрывать по версии (выравнивание позиционное, ADR `[006-feat-pg-stat-io]`),
поэтому каждая версия получает свой набор колонок, а `Ncols`/`DiffIntvl`/`UniqueKey` должны
двигаться вместе с ним — отсюда мульти-return по модели `SelectStatIOQuery` (а не по модели
timing-селектора, который возвращает только string).

Колоночный дизайн (Decision 2): один блок кумулятивных текстовых длительностей `*_total`
(показывается, не диффится) + один interval-блок `*_ms` + `functions` (диффится). Из 4 пар
phase count/time показываем только 4 phase TIME (как total+interval), а из счётчиков —
только `jit_functions` как единственный представительный counter; остальные `*_count`
метрики (inlining/optimization/emission/deform count) намеренно НЕ выводятся. Это укладывается
в ширину терминала (горизонтального скролла нет).

Фильтрация (Decision 3): оба конста заканчиваются `WHERE p.jit_functions > 0`, чтобы под
дефолтным `jit=on` экран не был забит нулевыми строками, а под `jit=off` был пустым.

## What to do

1. Добавить в блок `const (...)` файла `internal/query/statements.go` (рядом с остальными
   `PgStatStatements*` константами, по образцу `PgStatStatementsIoDefault` и timing-констант)
   две новые константы:
   - `PgStatStatementsJITPG15` — база PG15/16, 13 колонок.
   - `PgStatStatementsJITDefault` — PG17+, 15 колонок (база + deform).
2. Обе константы используют шаблонные токены `{{.PGSSSchema}}` и `{{.PgSSQueryLenFn}}` и
   заканчиваются `FROM {{.PGSSSchema}}.pg_stat_statements p JOIN pg_database d ON d.oid=p.dbid
   WHERE p.jit_functions > 0`. Стиль строки — конкатенация `+ "..."` с `E'\\s+'` в
   `regexp_replace` (как у `PgStatStatementsIoDefault`).
3. Добавить функцию `SelectStatStatementsJITQuery(version int) (string, int, [2]int, int)`
   рядом с `SelectStatStatementsTimingQuery` (низ файла). Двухветочный switch по образцу
   `SelectStatIOQuery` из `internal/query/io.go`. Возврат `UniqueKey` — это четвёртый элемент
   (отличие от 3-tuple `SelectStatIOQuery`, т.к. ключ JIT стоит в конце и сдвигается с
   `Ncols`).
4. Добавить в `internal/query/statements_test.go` тест `TestSelectStatStatementsJITQuery`
   (по образцу `TestSelectStatStatementsTimingQuery`), проверяющий обе ветки: query, Ncols,
   DiffIntvl, UniqueKey.
5. Добавить в `Test_StatStatementsQueries` exec-подтест для JIT, gated PG15+ (цикл по
   версиям `[]int{150000, 160000, 170000, 180000}`, по образцу WAL-цикла PG13+), который
   локально без PG скипается через `t.Skipf`.

## TDD Anchor

Тесты пишем ДО реализации, убеждаемся что падают (констант/функции ещё нет), потом пишем код.

- `internal/query/statements_test.go::TestSelectStatStatementsJITQuery` — для PG15/16
  (150000, 160000) возвращает `(PgStatStatementsJITPG15, 13, [2]int{6,10}, 11)`; для PG17/18
  (170000, 180000) возвращает `(PgStatStatementsJITDefault, 15, [2]int{7,12}, 13)`.
- `internal/query/statements_test.go::Test_StatStatementsQueries` (JIT exec sub-test) — для
  каждой версии PG15+ форматирует выбранный template через `Format()` и выполняет на реальном
  PG без ошибки (валидирует имена JIT-колонок против реальной схемы); без PG — `t.Skipf`.

## Acceptance Criteria

- [ ] `PgStatStatementsJITPG15` (13 колонок) и `PgStatStatementsJITDefault` (15 колонок)
      добавлены, следуют канонической pgss-разметке (user, database, *_total, *_ms +
      functions interval, md5 queryid, query последним), оба содержат `WHERE p.jit_functions > 0`.
- [ ] Колонки `*_total` — кумулятивные текстовые длительности
      `date_trunc('seconds', round(<jit_*_time>)/1000 * '1 second'::interval)::text`;
      `*_ms` — `round(p.jit_*_time)`; `functions` — `p.jit_functions`.
- [ ] PG17+ конст дополнительно содержит `deform_total` (idx 6) и `deform_ms` (interval),
      использует `jit_deform_time`.
- [ ] `SelectStatStatementsJITQuery(version)` возвращает `(string, int, [2]int, int)`:
      `version >= 170000` → `(Default, 15, [2]int{7,12}, 13)`; иначе → `(PG15, 13, [2]int{6,10}, 11)`.
- [ ] `TestSelectStatStatementsJITQuery` покрывает обе ветки (query/Ncols/DiffIntvl/UniqueKey).
- [ ] JIT exec sub-test добавлен в `Test_StatStatementsQueries`, gated PG15+, скипается без PG.
- [ ] `go test ./internal/query/...` зелёный локально (exec-подтесты скипаются без PG).
- [ ] Нет регрессий в существующих query-тестах.

## Context Files

**Feature artifacts:**
- [007-feat-pg-stat-statements-jit.md](007-feat-pg-stat-statements-jit.md) — user-spec
- [007-feat-pg-stat-statements-jit-tech-spec.md](007-feat-pg-stat-statements-jit-tech-spec.md) — tech-spec (Solution, Decision 1/2/3, Data Models)
- [007-feat-pg-stat-statements-jit-code-research.md](007-feat-pg-stat-statements-jit-code-research.md) — code-research (§1 JIT schema, §2 reference pattern, §8 anchors)
- 007-feat-pg-stat-statements-jit-decisions.md — decisions log (создаётся при завершении)

**Project knowledge:**
- [project.md](.claude/skills/project-knowledge/overview.md)
- [architecture.md](.claude/skills/project-knowledge/architecture.md)
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — code patterns, testing conventions, version branching

**Code files:**
- [internal/query/statements.go](internal/query/statements.go) — добавить 2 конста + селектор (модель: `PgStatStatementsIoDefault` строки 55-67, timing-консты, `SelectStatStatementsTimingQuery` строки 305-315)
- [internal/query/io.go](internal/query/io.go) — образец мульти-return: `SelectStatIOQuery` строки 87-92 `(string, int, [2]int)`
- [internal/query/query.go](internal/query/query.go) — version-константы `PostgresV15`..`PostgresV18` строки 18-21
- [internal/query/statements_test.go](internal/query/statements_test.go) — добавить тест (модель: `TestSelectStatStatementsTimingQuery` строки 10-32, WAL exec-цикл строки 86-102)

## Verification Steps

- Запустить `go test ./internal/query/...` — все тесты зелёные, JIT exec-подтесты скипаются
  без PG (`t.Skipf`).
- Убедиться, что `TestSelectStatStatementsJITQuery` падал ДО реализации селектора и прошёл
  ПОСЛЕ (TDD).
- Проверить, что существующие тесты (`TestSelectStatStatementsTimingQuery`,
  `Test_StatStatementsQueries`, `TestSelectQueryReportQuery`) не сломались.

## Details

**Files:**
- `internal/query/statements.go` — добавить в `const (...)` блок (заканчивается строкой 303)
  две константы и в конец файла (после `SelectQueryReportQuery`, строка 327) — селектор.
- `internal/query/statements_test.go` — добавить `TestSelectStatStatementsJITQuery` и
  JIT exec-цикл внутри `Test_StatStatementsQueries`.

**Точные имена JIT-колонок (verified vs PG official docs, code-research §1):**
- PG15/16: `jit_functions`(bigint), `jit_generation_time`, `jit_inlining_count`,
  `jit_inlining_time`, `jit_optimization_count`, `jit_optimization_time`,
  `jit_emission_count`, `jit_emission_time` (все `*_time` — double precision, миллисекунды).
- PG17+ добавляет: `jit_deform_count`(bigint), `jit_deform_time`(double).
- ВАЖНО: `*_count` метрики (inlining/optimization/emission/deform count) в выводе НЕ
  используются — показывается только `jit_functions` как единственный counter; 4 (PG17+: 5)
  phase TIME показываются как total + interval.

**Колоночная разметка (Decision 2):**
- PG15/16 (13 колонок, 0-based): `user`(0), `database`(1), `gen_total`(2), `inline_total`(3),
  `opt_total`(4), `emit_total`(5), `gen_ms`(6), `inline_ms`(7), `opt_ms`(8), `emit_ms`(9),
  `functions`(10), `queryid`(11), `query`(12). `DiffIntvl {6,10}` (interval-блок 6-9 +
  functions 10), `UniqueKey 11`.
- PG17+ (15 колонок): `user`(0), `database`(1), `gen_total`(2), `inline_total`(3),
  `opt_total`(4), `emit_total`(5), `deform_total`(6), `gen_ms`(7), `inline_ms`(8),
  `opt_ms`(9), `emit_ms`(10), `deform_ms`(11), `functions`(12), `queryid`(13), `query`(14).
  `DiffIntvl {7,12}`, `UniqueKey 13`.

**Выражения колонок (по образцу timing-констант statements.go:8-18):**
- `*_total`: `date_trunc('seconds', round(p.jit_generation_time) / 1000 * '1 second'::interval)::text AS gen_total`
  (аналогично inline/opt/emit/deform с соответствующими `jit_*_time`).
- `*_ms` (interval): `round(p.jit_generation_time) AS "gen,ms"` (alias-нейминг по образцу
  соседей — см. как timing использует `"all,ms"`).
- `functions`: `p.jit_functions AS functions`.
- `queryid`: `left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid`.
- `query`: `regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query`.
- `user`/`database`: `pg_get_userbyid(p.userid) AS user, d.datname AS database`.

**Селектор (модель `SelectStatIOQuery` io.go:87-92, но 4-tuple):**
```go
// SelectStatStatementsJITQuery returns the statements_jit query, column count,
// diff interval and unique-key index depending on Postgres version.
func SelectStatStatementsJITQuery(version int) (string, int, [2]int, int) {
    if version >= PostgresV17 {
        return PgStatStatementsJITDefault, 15, [2]int{7, 12}, 13
    }
    return PgStatStatementsJITPG15, 13, [2]int{6, 10}, 11
}
```
(`version >= PostgresV17` либо `version >= 170000` — `PostgresV17 = 170000`, query.go:20;
match neighbours.)

**Dependencies:** нет внешних пакетов. Использует существующие `query.Format()`,
`PostgresV15`..`PostgresV18`. Селектор вызывается только из `Configure()` для PG15+ (gate на
view-слое в Task 2), поэтому двухветочный switch достаточен — ветка PG<15 не нужна.

**Edge cases:**
- Ncols/DiffIntvl/UniqueKey должны быть строго консистентны с реальным числом колонок —
  главный риск (silent diff/align bug). Проверяется юнит-тестом + exec-тестом на CI PG15-18.
- `*_time` — double precision в мс; `round()` обязателен (как в timing), иначе дробные мс.
- `WHERE p.jit_functions > 0` — обязателен в обоих константах (Decision 3).

**Implementation hints:**
- Пиши `regexp_replace(..., E'\\s+', ...)` с двойным бэкслешем (конкатенированный стиль), как
  у `PgStatStatementsIoDefault` (statements.go:66), НЕ raw-backtick стиль с одинарным.
- JOIN — именно `JOIN pg_database d ON d.oid=p.dbid` (как у всех pgss-диффабельных констант).
- В exec-цикле используй `NewOptions(version, "f", "off", 256, "public")` и
  `postgres.NewTestConnectVersion(version)` (как WAL-цикл statements_test.go:86-102).
- Не трогай существующие константы/селекторы — только добавление.

## Reviewers

- **dev-code-reviewer** → `007-feat-pg-stat-statements-jit-task-01-dev-code-reviewer-review.json`
- **dev-security-auditor** → `007-feat-pg-stat-statements-jit-task-01-dev-security-auditor-review.json`
- **dev-test-reviewer** → `007-feat-pg-stat-statements-jit-task-01-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в `007-feat-pg-stat-statements-jit-decisions.md` (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
