---
status: planned                    # planned -> in_progress -> done
depends_on: ["02"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 3                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: "bash — go test ./internal/query/... ./internal/view/..."
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
teammate_name:
---

# Task 03: Version-aware selectors for the three progress screens

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

This is the only production-behaviour task of the feature. PostgreSQL 19 adds columns to three progress
views, and pgcenter has to render them on PG 19 while leaving PG 14–18 byte-identical:

- `pg_stat_progress_vacuum` — `started_by`, `mode`
- `pg_stat_progress_analyze` — `started_by`
- `pg_stat_progress_basebackup` — `backup_type`

Each of the three screens today has exactly one query constant (`PgStatProgressVacuumDefault` and
siblings) referenced directly from the static view registry in `internal/view/view.go: New()`, with no
selector at all — they are static query constants today. This task gives
each screen a second query constant for PG 19 and a `SelectStatProgressXxxQuery(version) (string, int,
[2]int)` selector, and wires all three into `view.Configure()`, following the `io.go` / `bgwriter.go` idiom
already documented in `patterns.md`.

The new columns are inserted **mid-layout**, not appended, so both `Ncols` and `DiffIntvl` shift. `Ncols`
being stale is loud — the screen misbehaves visibly. `DiffIntvl` being stale is silent: printing and
alignment walk the result's own column count, but a diff interval pointing at the wrong pair produces
plausible nonsense (per-interval deltas computed over `scanned_total,%` instead of `scanned,KiB`). That is
why the selectors are mandatory rather than cosmetic, and why the diff interval is asserted explicitly in
the unit tables.

This task also owns `internal/view/view_test.go` — deliberately excluded from Task 4 — so the PG 19 rows
that prove the `Configure()` wiring land in the same task and the same wave as the wiring itself.

## What to do

1. **Verify the column names against the live PG 19 catalog first.** The names below were read from a beta
   catalog (Risks: "Beta catalog drift before GA"). Before writing any query, connect to the PG 19 cluster
   Task 1 left running on port 21919 and confirm with `\d pg_stat_progress_vacuum` /
   `\d pg_stat_progress_analyze` / `\d pg_stat_progress_basebackup` that `started_by`, `mode` and
   `backup_type` exist with those exact names. If a name has drifted, use the catalog's name and record the
   deviation in the decisions log.

2. **Add one PG 19 query constant per screen**, beside the existing constant, following the
   `PgStatXxxPGNN` naming of the family (`PgStatBgwriterPG18`, `PgStatIOPG18`). The pre-19 constant is not
   touched (Decision 8 / ADR [004]) — two constants per screen, never one query with `NULL AS started_by`
   for older versions. The PG 19 constant is a copy of the pre-19 one with the new columns inserted at the
   positions in the Details table; everything else (joins, `WHERE`, `ORDER BY`, aliases, `coalesce` usage)
   stays character-for-character identical.

3. **Add one selector per screen**, named exactly `SelectStatProgressVacuumQuery`,
   `SelectStatProgressAnalyzeQuery`, `SelectStatProgressBasebackupQuery`, each
   `func(version int) (string, int, [2]int)`. All three carry the 3-tuple, including analyze whose diff
   interval is `{0,0}` on both branches (Decision 2) — the uniform arity is the point. Branch on
   `version >= PostgresV19` — unqualified, since the selectors live in the same package as the constant, so a package-qualified reference would not compile. Matches the `>=` idiom used by every other
   selector, and return the pre-19 triple on the fallthrough path.

4. **Wire all three into `view.Configure()`** as three new `case` blocks in the existing switch, in the
   same three-field assignment form the `bgwriter` / `stat_io` cases use. Leave the static `New()` map
   entries at their pre-19 values — they are the pre-Configure defaults and are pinned by tests.

5. **Switch the three progress execution tests from the bare constant to the selector.** Each of
   `Test_StatProgressVacuumQueries`, `Test_StatProgressAnalyzeQueries` and
   `Test_StatProgressBasebackupQueries` currently opens with `tmpl := PgStatProgressXxxDefault`. Left as-is,
   the PG 19 subtest would execute the PG 18 query against the PG 19 cluster, pass, and prove nothing. Take
   the query and the expected column count from the selector, add `190000` to each version list, and follow
   `Test_StatBgwriterQueries`: use `conn.Query` rather than `conn.Exec` and assert
   `rows.FieldDescriptions()` has the selector's `Ncols`. That assertion is what actually proves the new
   columns exist on the server — it is the only automated guard against beta catalog drift.

6. **Add the three selector unit tables** (see TDD Anchor), modelled on `Test_SelectStatBgwriterQuery`.

7. **Add the PG 19 rows to `internal/view/view_test.go`:** a `190000` block in `TestViews_Configure`
   asserting the three progress views resolve to their PG 19 templates with the new `Ncols` and `DiffIntvl`,
   a `180000` block asserting they still resolve to the pre-19 constants and layouts, and a `190000` row in
   `TestView_VersionOK` whose `total` is **derived** from the actual `MinRequiredVersion` gates in `New()`,
   not copied from the row above it.

**Explicitly out of scope:** `progress_cluster`, `progress_copy`, `progress_index` (unchanged in PG 19 —
Decision 6); `delay_time` on vacuum/analyze (a PG 18 column, deferred by the user-spec); `report/describe.go`
(Task 5); the replay test and goldens (Task 6); every `_test.go` file outside the four this task owns
(Task 4). Do not add a width guard to the shared diff loop (Decision 5b).

## TDD Anchor

Тесты, которые нужно написать ДО реализации. Пишем → запускаем → убеждаемся что падают → пишем код →
убеждаемся что проходят.

Selector unit tables (no database required — these must fail before the selectors exist and pass after):

- `internal/query/progress_vacuum_test.go::Test_SelectStatProgressVacuumQuery` — table over
  `{140000, 170000, 180000, 190000}`: 180000 → `PgStatProgressVacuumDefault`, `Ncols` 13, `DiffIntvl`
  `{10,11}`; 190000 → the PG 19 constant, `Ncols` 15, `DiffIntvl` `{12,13}`.
- `internal/query/progress_analyze_test.go::Test_SelectStatProgressAnalyzeQuery` — same shape: 180000 →
  default constant, `Ncols` 12, `DiffIntvl` `{0,0}`; 190000 → PG 19 constant, `Ncols` 13, `DiffIntvl`
  `{0,0}` (unchanged — assert it explicitly rather than omitting it).
- `internal/query/progress_basebackup_test.go::Test_SelectStatProgressBasebackupQuery` — 180000 → default
  constant, `Ncols` 11, `DiffIntvl` `{9,9}`; 190000 → PG 19 constant, `Ncols` 12, `DiffIntvl` `{10,10}`.

View-layer wiring (no database required):

- `internal/view/view_test.go::TestViews_Configure` — `190000` matrix block: `views["progress_vacuum"]`
  has the PG 19 template, `Ncols` 15, `DiffIntvl` `{12,13}`; `progress_analyze` 13 / `{0,0}`;
  `progress_basebackup` 12 / `{10,10}`.
- `internal/view/view_test.go::TestViews_Configure` — `180000` matrix block: the same three views keep the
  pre-19 constants, `Ncols` 13/12/11 and `DiffIntvl` `{10,11}`/`{0,0}`/`{9,9}`. This is the no-regression
  half of the boundary and is what fails if a selector's inequality is written the wrong way round.
- `internal/view/view_test.go::TestView_VersionOK` — a `190000` row with a derived `total`.

Execution tests (require the PG 19 cluster; skip cleanly without it):

- `internal/query/progress_vacuum_test.go::Test_StatProgressVacuumQueries/pg_stat_progress_vacuum/190000` —
  the selector's query executes on PG 19 and returns exactly 15 columns.
- `internal/query/progress_analyze_test.go::Test_StatProgressAnalyzeQueries/pg_stat_progress_analyze/190000`
  — executes and returns exactly 13 columns.
- `internal/query/progress_basebackup_test.go::Test_StatProgressBasebackupQueries/pg_stat_progress_basebackup/190000`
  — executes and returns exactly 12 columns.

## Acceptance Criteria

- [ ] Column names confirmed against the live PG 19 catalog before the queries were written; any drift from
      `started_by` / `mode` / `backup_type` recorded in the decisions log.
- [ ] Each of the three files has exactly two query constants; the pre-19 constant is unchanged.
- [ ] The new constants are static SQL with **no** template placeholders, like the constants they sit
      beside (`Format` must be a no-op on them).
- [ ] New columns sit immediately before the state column: after `relation` on vacuum and analyze, after
      `duration` on basebackup — matching the user-spec layouts, not section 2.5 of the code-research doc.
- [ ] `SelectStatProgressVacuumQuery`, `SelectStatProgressAnalyzeQuery`, `SelectStatProgressBasebackupQuery`
      exist with those exact names, all three returning `(string, int, [2]int)`, and return the documented
      triples on both branches.
- [ ] `view.Configure()` patches `QueryTmpl`, `Ncols` and `DiffIntvl` for all three views; the static
      `New()` map still holds the pre-19 values (13/`{10,11}`, 12/`{0,0}`, 11/`{9,9}`).
- [ ] `OrderKey` stays 0 and `UniqueKey` stays 0 on all three views — `pid` is column 0 in every layout, so
      no 4-tuple selector is needed (ADR [007] does not apply here).
- [ ] No `coalesce` added around the new columns — they sit outside `DiffIntvl` and NULL must render blank.
- [ ] The three execution tests call the selector, include `190000` in their version lists, and assert the
      live column count equals the selector's `Ncols`.
- [ ] `internal/view/view_test.go` has the `190000` and `180000` Configure blocks and a derived `190000`
      row in `TestView_VersionOK`.
- [ ] `go test ./internal/query/... ./internal/view/...` green; `make lint` and `make build` clean.

## Context Files

**Feature artifacts:**
- [012-feat-pg19-compatibility-baseline.md](012-feat-pg19-compatibility-baseline.md) — user-spec; section
  «Дизайн и интерфейс» is the **authoritative** column layout, and «Значения колонок» the value domains
- [012-feat-pg19-compatibility-baseline-tech-spec.md](012-feat-pg19-compatibility-baseline-tech-spec.md) —
  tech-spec: Decisions 1, 2 and 8, the Data Models table, Task 3 in Implementation Tasks
- [012-feat-pg19-compatibility-baseline-decisions.md](012-feat-pg19-compatibility-baseline-decisions.md) —
  decisions log (Task 1 and Task 2 entries; this task appends its own in Post-completion)

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — features, supported stats,
  supported PG versions (this project has no `project.md`; `overview.md` is the project-level document)
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — «PostgreSQL Version
  Handling»: the inventory of existing selectors this task extends
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — «Version-Specific Query Pattern»
  (when the selector must carry `DiffIntvl`) and «Adding a New PostgreSQL Version»

**Code files (modify):**
- [internal/query/progress_vacuum.go](../../../internal/query/progress_vacuum.go) — one constant today
  (`PgStatProgressVacuumDefault`, 13 cols, `RIGHT JOIN pg_stat_activity`); add the PG 19 constant + selector
- [internal/query/progress_analyze.go](../../../internal/query/progress_analyze.go) — one constant
  (`PgStatProgressAnalyzeDefault`, 12 cols); add the PG 19 constant + selector
- [internal/query/progress_basebackup.go](../../../internal/query/progress_basebackup.go) — one constant
  (`PgStatProgressBasebackupDefault`, 11 cols); add the PG 19 constant + selector
- [internal/query/progress_vacuum_test.go](../../../internal/query/progress_vacuum_test.go) — currently only
  `Test_StatProgressVacuumQueries`, versions `90600…180000`, `tmpl := PgStatProgressVacuumDefault`,
  `conn.Exec`; add the unit table, switch to the selector + `conn.Query` + `FieldDescriptions` assertion,
  add `190000`
- [internal/query/progress_analyze_test.go](../../../internal/query/progress_analyze_test.go) — same shape,
  versions `140000…180000`
- [internal/query/progress_basebackup_test.go](../../../internal/query/progress_basebackup_test.go) — same
  shape, versions `140000…180000`
- [internal/view/view.go](../../../internal/view/view.go) — `New()` lines 277–348 hold the three progress
  entries (leave their values alone); `Configure()` lines 367–405 hold the switch that gains three cases
- [internal/view/view_test.go](../../../internal/view/view_test.go) — `TestViews_Configure` (version matrix
  + per-version switch) and `TestView_VersionOK` (version → count table)

**Code files (read — reference implementations):**
- [internal/query/bgwriter.go](../../../internal/query/bgwriter.go) — the model: per-version constants +
  a `(string, int, [2]int)` selector with a comment per branch explaining the diffed block
- [internal/query/bgwriter_test.go](../../../internal/query/bgwriter_test.go) — the model for both test
  shapes: `Test_SelectStatBgwriterQuery` (unit table) and `Test_StatBgwriterQueries` (execution test that
  takes the query from the selector and asserts `FieldDescriptions` length)
- [internal/query/io.go](../../../internal/query/io.go) — second selector example; shows how a
  version-dependent layout is documented in the constant's doc comment

## Verification Steps

- `go test ./internal/query/... ./internal/view/...` — all green. The three new selector unit tables and
  the view Configure blocks run without a database and must pass unconditionally.
- With the PG 19 cluster from Task 1 running on 21919: the `190000` subtests of the three
  `Test_StatProgress*Queries` **execute** (not skip) and their `FieldDescriptions` assertions pass — confirm
  by running with `-run 'Test_StatProgress.*/190000' -v` and reading the output. A `SKIP` here means the
  cluster is down and the task is not verified.
- With the PG 19 cluster stopped: the same subtests **skip** rather than fail, and every other test stays
  green.
- `go test ./...` — nothing outside the two packages regressed (notably `report/`: the existing progress
  goldens carry `version_num = 140000` and must stay byte-identical; a golden diff means a selector branch
  is wrong).
- `make lint` and `make build` clean.

## Details

**Files:**

- `internal/query/progress_vacuum.go` — add `PgStatProgressVacuumPG19` (or the family-consistent name) and
  `SelectStatProgressVacuumQuery`. Note this is the one query using `RIGHT JOIN pg_stat_activity`, so rows
  matched only by query text carry NULL in every `v.*` column, including the two new ones.
- `internal/query/progress_analyze.go` — add the PG 19 constant and `SelectStatProgressAnalyzeQuery`.
- `internal/query/progress_basebackup.go` — add the PG 19 constant and `SelectStatProgressBasebackupQuery`.
- `internal/query/progress_{vacuum,analyze,basebackup}_test.go` — unit table + reworked execution test each.
- `internal/view/view.go` — three `case` blocks in `Configure()`; `New()` untouched.
- `internal/view/view_test.go` — `190000`/`180000` Configure blocks + derived `TestView_VersionOK` row.

**Exact layouts** (0-based indices; the new columns are in **bold**):

`progress_vacuum` — 13 → 15 cols, `DiffIntvl {10,11}` → `{12,13}`:

| idx | PG ≤ 18 | PG 19 |
|---|---|---|
| 0 | `a.pid` | `a.pid` |
| 1 | `xact_age` | `xact_age` |
| 2 | `v.datname` | `v.datname` |
| 3 | `relation` | `relation` |
| 4 | `a.state` | **`started_by`** |
| 5 | `waiting` | **`mode`** |
| 6 | `v.phase` | `a.state` |
| 7 | `size_total,KiB` | `waiting` |
| 8 | `scanned_total,%` | `v.phase` |
| 9 | `vacuumed_total,%` | `size_total,KiB` |
| 10 | `scanned,KiB` *(diff)* | `scanned_total,%` |
| 11 | `vacuumed,KiB` *(diff)* | `vacuumed_total,%` |
| 12 | `a.query` | `scanned,KiB` *(diff)* |
| 13 | — | `vacuumed,KiB` *(diff)* |
| 14 | — | `a.query` |

`progress_analyze` — 12 → 13 cols, `DiffIntvl {0,0}` on both: `started_by` is inserted at index 4, between
`relation` and `a.state`; everything after shifts by one.

`progress_basebackup` — 11 → 12 cols, `DiffIntvl {9,9}` → `{10,10}`: `backup_type` is inserted at index 4,
between `duration` and `a.state`; `streamed,KiB` (the single diffed column) moves 9 → 10.

**Where the layout comes from:** the user-spec's «Дизайн и интерфейс» section and the tech-spec's Data
Models table. Section 2.5 of `012-feat-pg19-compatibility-baseline-code-research.md` predates Decision 1
and places the columns after `datname`/`pid` — it is superseded. The `(Ncols, DiffIntvl)` arithmetic is
identical either way, so the tests cannot catch a wrong position; only reading the user-spec can.

**Dependencies:** Task 2 (the `PostgresV19` constant, port 21919 in the test-connection map, `NewTestConnectVersion`
erroring on unmapped versions). Task 1 for the live cluster. No new packages. Downstream: Task 5 mirrors the
emitted column order in `report/describe.go`; Task 6's replay test relies on these selectors.

**Edge cases:**

- **NULL renders blank.** Vacuum rows without a progress record (`RIGHT JOIN`, matched on query text only)
  have NULL `started_by`/`mode`. The user-spec requires an empty cell — no dashes, no `unknown`, no zero.
  Since the columns are outside `DiffIntvl`, NULL passes through as an empty string harmlessly. Do **not**
  wrap them in `coalesce` — that pattern exists only for diffed columns (`replication_slots.go`), where an
  empty string would abort the sample in `strconv.ParseInt`.
- **Values render raw.** `autovacuum_wraparound`, `aggressive`, `failsafe`, `incremental` and friends are
  printed exactly as the server reports them — no abbreviation, no renaming, no `CASE` mapping.
- **`mode` as an identifier.** It is a non-reserved keyword in PostgreSQL and the reference is qualified
  (`v.mode`), so no quoting is needed. Do not alias it to something else: the column name is the user-facing
  header, and the user-spec names it `mode`. (It collides with the `mode` alias on the replication screen —
  harmless today, already recorded as a handoff note to feature [014].)
- **Column width.** `autovacuum_wraparound` is 21 chars. No width machinery is added: horizontal scroll from
  [009] handles narrow terminals, and the new columns' left placement keeps them inside the initial window.
- **Version floor.** `progress_vacuum` is gated at PG 9.6 and its execution test loops from `90600`; the
  selector's fallthrough branch must serve every one of those versions unchanged.

**Implementation hints:**

- Write the PG 19 constant by copying the pre-19 one and splicing the new columns in — not by retyping it.
  A stray whitespace or alias change would ripple into `report -d` texts and goldens for no reason.
- Give each new constant a doc comment in the style of `PgStatBgwriterPG18` / `PgStatIOPG18`: which PG
  version added the columns, where they sit, and why they are outside `DiffIntvl`.
- Give each selector branch a one-line comment naming the diffed columns by name, as `SelectStatBgwriterQuery`
  does — the numbers alone are unreviewable.
- In the execution tests, keep the existing `t.Skipf` inside the `t.Run` body: the per-version subtest
  wrapper is what makes one unavailable cluster skip only its own version. (The nine tests that lack this
  wrapper are Decision 7's deferred item — not these three.)
- `TestView_VersionOK`'s `190000` total: derive it by checking the `MinRequiredVersion` of every entry in
  `New()` rather than assuming; the highest gate in the registry is currently `PostgresV16`.
- `TestViews_Configure` iterates a matrix of `(version, recovery, trackCommit, querylen)` and asserts inside
  a `switch tc.version`. Add the eight-row blocks in the existing style; the trailing
  `assert.NotEqual(t, "", v.Query)` loop then also proves `Format` succeeds on the new constants.

## Reviewers

- **dev-code-reviewer** → `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline-task-03-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline-task-03-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline-task-03-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в
      [012-feat-pg19-compatibility-baseline-decisions.md](012-feat-pg19-compatibility-baseline-decisions.md)
      (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину (в частности: любое расхождение имён колонок
      с бета-каталогом)
- [ ] Обновить user-spec/tech-spec если что-то изменилось
