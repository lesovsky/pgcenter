---
status: planned
depends_on: ["03"]
wave: 4
skills: [code-writing]
verify: "bash — go test ./report/...; pgcenter report -d -P v|a|b lists the new columns"
reviewers: [dev-code-reviewer, dev-test-reviewer]
teammate_name:
---

# Task 05: Report describe texts for the new columns

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

`pgcenter report -d -P v|a|b` prints a static help text that lists, for one report type, every column the
report emits: the displayed column name, the catalog column(s) it comes from, and a one-line prose
description. These texts live in `report/describe.go` as plain Go string constants and are the only place a
user can look up what a column means without reading the source.

Task 3 adds three new columns to the PG 19 branches of the three progress screens — `started_by` and `mode`
on vacuum, `started_by` on analyze, `backup_type` on basebackup. Right now those columns would appear in the
report output with no entry in the describe text at all. This task closes that gap: three description
constants gain four rows in total, plus a trailing note per constant naming the PostgreSQL version the
columns appeared in.

Two constraints make this less trivial than "add four lines".

1. **Row order carries meaning.** The tables are read side by side with the actual report output, so rows
   are listed in *emitted* order. A row in the wrong slot is not a cosmetic defect — it makes the help text
   describe a layout that does not exist. The emitted order comes from the user-spec's «Дизайн и интерфейс»
   section and from Decision 1 of the tech-spec: the new columns sit **after `relation`** on vacuum and
   analyze, and **after `duration`** on basebackup. The code-research document's own §5.4 gives a different
   insertion point (after `datname` on vacuum/analyze, after `pid` on basebackup) — that placement is from
   the first research pass, it was superseded by Decision 1, and it must not be followed. Appendix C.2 of
   the same document has the corrected placement.

2. **The existing test is blind to it.** `Test_describeReport` in `report/report_test.go` compares
   `describeReport()`'s output against the constants themselves — identity, not content. Any text edit
   passes it by construction, including a wrong row order or a typo. So the row order gets a guard of its
   own: a short ordering test (see TDD Anchor) asserting, inside each constant, that the new row sits after
   the row it must follow and before the row it must precede — by comparing substring positions. That
   covers order and only order; the wording, the origin column and the tab alignment are still checked by
   eye, against the rendered `report -d` output and the PG 19 query column order from Task 3.

The texts stay single flat strings describing the superset of columns, with no version-aware branching —
the same way `pgStatBgwriterDescription` and `pgStatIODescription` already handle their version-varying
column sets (Decision 3). `report -d` is static help text; a version-aware describe path would be
disproportionate machinery, and it was explicitly rejected.

## What to do

1. Read the PG 19 query constants that Task 3 wrote in `internal/query/progress_vacuum.go`,
   `progress_analyze.go` and `progress_basebackup.go`. Take two things from them: the exact position of
   each new column in the emitted list, and the exact catalog column each one is selected from. Both go into
   the describe rows — do not derive them from this document or from the code-research text.

2. Add the new rows to the three constants in `report/describe.go`, each in the slot matching the emitted
   column order:
   - `pgStatProgressVacuumDescription` — `started_by` and `mode`, in that order, after the `relation` row
     and before the `state` row.
   - `pgStatProgressAnalyzeDescription` — `started_by`, after the `relation` row and before the `state` row.
   - `pgStatProgressBasebackupDescription` — `backup_type`, after the `duration` row and before the `state`
     row.

   Each row follows the existing three-field format of the surrounding table: `- ` + displayed column name,
   the origin catalog column, and a prose description. Match the tab alignment of the neighbouring rows
   **inside the same constant** — the three tables do not use the same origin-column width, so alignment
   copied across constants will look wrong.

3. Write the prose so it names the value domain, since that is the part a user cannot guess: vacuum
   `started_by` is `manual` / `autovacuum` / `autovacuum_wraparound`, vacuum `mode` is `normal` /
   `aggressive` / `failsafe`, analyze `started_by` is `manual` / `autovacuum`, basebackup `backup_type` is
   `full` / `incremental`. Keep each row to one line, in the register the neighbouring rows use.

4. Append a trailing note to each of the three constants, naming the version the columns appeared in — e.g.
   `Note: started_by and mode are available since PostgreSQL 19.` for vacuum, and the equivalent single-column
   wording for analyze and basebackup. Place it exactly where the existing notes sit: after the last table
   row, separated by a blank line, and followed by a blank line before the `Details:` block — see
   `pgStatBgwriterDescription` and `pgStatIODescription` for the placement and the phrasing register.

5. Do **not** put a `(PG19+)` marker inside the row text. The tech-spec picked one of the three options the
   research offered — rows without a version suffix, plus the trailing note — so the note is the single
   place the version information lives, and all three constants stay consistent with each other and with the
   existing bgwriter/IO precedent.

6. Add the ordering test described in the TDD Anchor to `report/report_test.go` — written before the
   constants are edited, so it fails first for the right reason (rows absent) and turns green once the rows
   land in the right slots.

7. Verify the rest by eye, which the test cannot do: build the binary and run `report -d` for all three
   report types, then compare the printed rows against Task 3's PG 19 query constants column by column —
   origin names, wording, and column alignment.

**Out of scope:** the `//` doc comments above the three constants keep their current wording — do not add
`(PG14 baseline)`-style annotations. That device exists on bgwriter/IO/JIT because their tables list only
the baseline columns; here the table lists the superset, so the annotation would be inaccurate. No changes
to `describeReport()` in `report/report.go`, to the existing `Test_describeReport` (the ordering test is a
separate new function next to it), or to any other description constant.

## TDD Anchor

Write this before touching `report/describe.go`. One new test function in `report/report_test.go`, roughly
twenty lines, no PostgreSQL and no golden file — it reads the three constants directly (same package) and
compares `strings.Index` positions. It catches exactly the failure this task fears: a row in the wrong slot.

- `report/report_test.go::Test_describeProgressColumnOrder/vacuum` — inside
  `pgStatProgressVacuumDescription`, the position of `"\n- started_by"` is greater than that of
  `"\n- relation"` and less than that of `"\n- mode"`, which in turn is less than that of `"\n- state"`.
  Verifies the vacuum rows sit in emitted order.
- `report/report_test.go::Test_describeProgressColumnOrder/analyze` — inside
  `pgStatProgressAnalyzeDescription`, `"\n- started_by"` sits between `"\n- relation"` and `"\n- state"`.
- `report/report_test.go::Test_describeProgressColumnOrder/basebackup` — inside
  `pgStatProgressBasebackupDescription`, `"\n- backup_type"` sits between `"\n- duration"` and
  `"\n- state"`.

Three points that decide whether the test is worth anything:

- **Assert every marker was found before comparing.** `strings.Index` returns `-1` for a missing substring,
  and `-1 < anything` passes — a test that silently green-lights a row that was never added. Require each
  position to be non-negative (`assert.NotEqual(t, -1, pos)` or `require.Positive`) first, then compare.
- **Anchor markers on `"\n- "`, not on the bare column name.** The trailing note added in step 4 contains
  the words `started_by`, `mode` and `backup_type`; matching a bare name would find the note instead of the
  row. The leading newline plus `- ` pins the match to a table row.
- **Red first.** Run the test before editing `describe.go`: it must fail on the missing-marker assertion,
  not on the ordering one. A test that is green before the rows exist is matching the wrong thing.

Drive it as a table with the three subtests, in the style of the surrounding tests in the file. Do not fold
the assertions into `Test_describeReport` — that one is an identity check over the whole report map and has
a different reason to fail.

## Acceptance Criteria

- [ ] `pgStatProgressVacuumDescription` has `started_by` and `mode` rows, in that order, between `relation`
      and `state`.
- [ ] `pgStatProgressAnalyzeDescription` has a `started_by` row between `relation` and `state`.
- [ ] `pgStatProgressBasebackupDescription` has a `backup_type` row between `duration` and `state`.
- [ ] Each row's origin field names the catalog column actually selected by the corresponding PG 19 query
      constant from Task 3, and each row's prose names the column's value domain.
- [ ] Each of the three constants carries a trailing note naming PostgreSQL 19 as the version the new
      column(s) appeared in, placed between the last table row and the `Details:` block, blank-line
      separated, matching the existing note lines in the file.
- [ ] No `(PG19+)`-style version marker inside any row; the three constants are consistent with each other.
- [ ] Row order in the rendered `report -d -P v|a|b` output matches the column order of the PG 19 queries.
- [ ] Columns and alignment line up when the constants are printed — tab counts match the neighbouring rows
      within each constant.
- [ ] `Test_describeProgressColumnOrder` exists in `report/report_test.go` with the three subtests, checks
      that each marker was found before comparing positions, and genuinely fails when a new row is moved out
      of its slot (confirm once by temporarily moving one).
- [ ] No other constant in `report/describe.go` changed; `report/report.go` untouched; `report/report_test.go`
      changed only by the new ordering test — the existing `Test_describeReport` is not edited.
- [ ] `go test ./report/...` green; `make lint` clean.

## Context Files

**Feature artifacts:**
- [012-feat-pg19-compatibility-baseline.md](012-feat-pg19-compatibility-baseline.md) — user-spec; «Дизайн и интерфейс» has the authoritative column order, «Значения колонок» the value domains
- [012-feat-pg19-compatibility-baseline-tech-spec.md](012-feat-pg19-compatibility-baseline-tech-spec.md) — tech-spec; Decision 1 (positions), Decision 3 (rows + note), Data Models table
- [012-feat-pg19-compatibility-baseline-decisions.md](012-feat-pg19-compatibility-baseline-decisions.md) — decisions log
- [012-feat-pg19-compatibility-baseline-code-research.md](012-feat-pg19-compatibility-baseline-code-research.md) — Appendix C has the table format and the note precedent; **§5.4's insert points are superseded — ignore them**

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — project context, supported statistics, PostgreSQL version support
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout, report/replay data flow, PG version handling
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — «Adding a New PostgreSQL Version», version-specific query pattern, testing conventions

**Code files:**
- [report/describe.go](../../../report/describe.go) — the only production file to modify; three constants gain rows and a note
- [report/report.go](../../../report/report.go) — `describeReport()` at :606 maps report type → constant; read only, no change
- [report/report_test.go](../../../report/report_test.go) — `Test_describeReport` at :1170 compares by identity; read only except for appending the new ordering test
- [internal/query/progress_vacuum.go](../../../internal/query/progress_vacuum.go) — source of truth for the vacuum PG 19 column order and catalog names (written by Task 3)
- [internal/query/progress_analyze.go](../../../internal/query/progress_analyze.go) — same, for analyze
- [internal/query/progress_basebackup.go](../../../internal/query/progress_basebackup.go) — same, for basebackup

## Verification Steps

- `go build ./... && go vet ./...` — the constants still compile (a stray backtick in a raw string breaks the
  whole file).
- `go test -run 'Test_describe' ./report/ -v` — both green, and the three
  `Test_describeProgressColumnOrder` subtests visibly RUN. That covers row order. It says nothing about the
  rest of the text: `Test_describeReport` compares the returned value against the constant itself, so it
  cannot see wording, origin names or alignment — it only guards the mapping and the package.
- `go test ./report/...` — green.
- `make build`, then run all three describes and read the output:
  `./bin/pgcenter report -d -P v`, `./bin/pgcenter report -d -P a`, `./bin/pgcenter report -d -P b`.
  No archive file is needed — `describeReport` returns before any file is opened.
- For each of the three: the new column(s) appear as rows, in the same relative position they occupy in the
  PG 19 query constant from Task 3 (`started_by`/`mode` right after `relation` on vacuum; `started_by`
  right after `relation` on analyze; `backup_type` right after `duration` on basebackup), and the three
  fields line up in columns with the rows above and below.
- For each of the three: a note line naming PostgreSQL 19 sits between the table and the `Details:` block,
  with a blank line on each side, rendering the same way the bgwriter note does
  (`./bin/pgcenter report -d -B` for comparison).
- `make lint` clean.

## Details

**Files:**

- `report/describe.go` — the only production file changed. Three of its constants are touched:
  - `pgStatProgressVacuumDescription` (currently around `:202-220`, table rows `:204-217`): today 13 rows,
    `pid, xact_age, datname, relation, state, waiting, phase, size_total,KiB, scanned_total,%,
    vacuumed_total,%, scanned,KiB, vacuumed,KiB, query`. After this task, 15 rows — `started_by` and `mode`
    inserted between `relation` and `state`.
  - `pgStatProgressAnalyzeDescription` (around `:266-283`, rows `:268-280`): today 12 rows,
    `pid, xact_age, datname, relation, state, waiting, phase, sample_size,KiB, scanned,%, ext_total/done,
    child_total/done,%, child_in_progress`. After this task, 13 rows — `started_by` between `relation` and
    `state`.
  - `pgStatProgressBasebackupDescription` (around `:286-302`, rows `:288-299`): today 11 rows,
    `pid, started_from, started_at, duration, state, waiting, phase, size_total,KiB, streamed,%,
    streamed,KiB, tablespaces_total/streamed`. After this task, 12 rows — `backup_type` between `duration`
    and `state`.

  Line numbers are indicative — locate the rows by content, since Task 3 does not touch this file but a
  concurrent Wave 4 task might shift nothing here at all.

- `report/report_test.go` — one new test function appended, `Test_describeProgressColumnOrder` (see TDD
  Anchor). Same package as the constants, so it references them directly, no export needed. `strings` is
  not among the file's current imports (`:3-20`) — add it. `Test_describeReport` at `:1170` stays exactly
  as it is.

**Dependencies:**

- Task 3 (wave 3) must be merged first. Not a compile dependency — `describe.go` is a block of string
  constants and imports nothing — but a correctness one: the column order and the catalog column names in
  the new rows are read from the PG 19 query constants Task 3 writes. Without them the row order is a guess.
- No other Wave 4 task touches `report/describe.go`, so this task can run concurrently with Tasks 4, 6 and 7.
- No new packages.

**Edge cases:**

- **Only the row order is under test; everything else is eye-checked.** There is no golden file for `-d`
  output, and `Test_describeReport` compares by identity, so it cannot see content at all. The new ordering
  test closes the row-order hole and nothing more — a wrong origin column, a misleading description or
  broken alignment still passes CI. The eye-check against the query constant is not a formality.
- **Tab alignment differs between the three constants.** The vacuum table uses a narrower origin column than
  the analyze and basebackup tables (compare the header rows). Copy the tab pattern from the row directly
  above the insertion point, not from a different constant. Tabs render at 8-column stops; `started_by` is
  10 characters, so it may need one tab fewer than the shorter names around it — check the rendered output,
  not the source.
- **Raw string literals.** All three constants are backtick-quoted raw strings; a backtick or a stray
  closing quote in the new text terminates the literal and produces a confusing compile error far from the
  edit. Keep the prose plain ASCII with no backticks.
- **`mode` collides with the replication screen's `mode` column**, which is an alias of `sync_state` and is
  described in `pgStatReplicationDescription` (`:62`). Two unrelated columns now share a display name across
  two describe texts; that is expected and already recorded as a handoff to feature [014]. Do not try to
  disambiguate the names here.
- **Users on PG 14–18 read the same text.** The tech-spec's Backward Compatibility section names this as the
  only user-visible change on old versions, and accepts it. The trailing note is what keeps it honest: the
  table describes a superset, and the note says which rows are not there before PG 19.

**Implementation hints:**

- The `origin` field convention: name the underlying catalog column(s), comma-separated when a display
  column is derived from several (`wait_event_type,wait_event`), and `-` when the value is synthetic. For
  these four columns the display name and the catalog name are expected to coincide — confirm against Task
  3's query constants rather than assuming, since a query may alias.
- The note wording precedent, from the same file: `Note: on PG18 KiB throughput comes from
  read_bytes/write_bytes/extend_bytes; op_bytes was removed.` (`pgStatIODescription`) and
  `Note: on PG17+ checkpoint/restartpoint counters come from pg_stat_checkpointer; PG18 adds slru_written.`
  (`pgStatBgwriterDescription`). One line, sentence case, ends with a period, sits alone between blank
  lines.
- Keep the three notes phrased in parallel with one another — they are read as a set by anyone comparing
  the three progress screens.
- `report -d` output goes through `fmt.Fprint` verbatim, so what is in the constant is exactly what the user
  sees, trailing whitespace included. Some existing rows in this file carry accidental trailing spaces; do
  not copy that, and do not clean up the existing ones either.

## Reviewers

- **dev-code-reviewer** → `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline-task-05-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline-task-05-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [012-feat-pg19-compatibility-baseline-decisions.md](012-feat-pg19-compatibility-baseline-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
