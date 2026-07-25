---
status: planned                    # planned -> in_progress -> done
depends_on: ["03"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 4                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: "bash — go test ./report/...; the three existing progress goldens unchanged"
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 06: Report replay coverage for the new layout

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

`pgcenter report` replays a recorded archive. The column layout it uses is **not** taken from the machine
running the report — it is taken from the `version_num` stored in the archive's `meta.*` entry, which
`report/report.go` feeds into `views.Configure()` on the first sample of a run. Task 3 made
`progress_vacuum` version-aware: on PG 19 the query returns 15 columns with `started_by`/`mode` inserted at
indexes 4–5, and the diffed pair `scanned,KiB` / `vacuumed,KiB` moves from `{10,11}` to `{12,13}`.

This task adds the test that proves the replay path actually picks the layout from the archive. It runs the
same vacuum-progress report over two synthetic in-memory archives — one recorded on a pre-19 server, one on
PG 19 — and pins each against its own golden file.

**Why this test and not another.** Of the three things the selector returns, the diff interval is the only
one that fails silently. The whole print/align/diff chain walks the *recorded result's* own column count
(`internal/stat/postgres.go:611` — `diff.Ncols = len(curr.Cols)`), so a stale `Ncols` cannot cause an index
panic and a stale `DiffIntvl` cannot crash either: `diff()` simply subtracts the wrong pair of columns. On
the PG 19 layout a stale `{10,11}` points at `scanned_total,%` / `vacuumed_total,%` — text percentages like
`"50.00"`, which `diffPair` happily parses as floats. The report then prints diffed percentages and raw
cumulative KiB values, with no error anywhere. That is the "stale `DiffIntvl` silently corrupts PG 19 report
replay" row of the tech-spec Risks table, and this test is its named mitigation.

**Scope boundary — read before touching anything outside `report/`.** This task adds test coverage only. It
must **not** add a width guard, a length check, or any other defensive edit to the shared diff engine in
`internal/stat/postgres.go`. That was proposed by the security audit and explicitly rejected in tech-spec
Decision 5b: the premise (that version-dependent widths make mixed-width diffing reachable) is false — the
replay loop drops the previous snapshot and skips the sample whenever the archive's recorded version
changes — and the underlying pre-existing defect is a deferred item the user-spec declines. If the
implementation appears to need such a guard, that is a signal the test harness is wrong, not the engine.

## What to do

1. Create `report/report_record_progress_vacuum_test.go` with a table-driven
   `Test_app_doReport_ProgressVacuum` modelled directly on `Test_app_doReport_Bgwriter`
   (`report/report_record_bgwriter_test.go:31`) — same testcase struct shape, same synthetic in-memory tar,
   same sentinel asserts before the golden comparison, same `*update` golden-regeneration branch.
2. Give it two subcases: `pg18` (`versionNum: "180000"`, 13 columns, diffed pair at 10–11) and `pg19`
   (`versionNum: "190000"`, 15 columns with `started_by`/`mode` at 4–5, diffed pair at 12–13). Column
   names must match the SQL aliases the two query constants actually emit — read them from
   `internal/query/progress_vacuum.go` after Task 3 rather than transcribing from any spec table.
3. Build each archive as six tar entries — `meta`, `progress_vacuum`, `sysinfo`, twice — with the report-type
   basename, timestamps exactly one second apart, and a single data row carrying the **same `pid` in both
   ticks**.
4. Pick fixture values that make the diff visible and cross-checkable: the two diffed columns must grow from
   tick 1 to tick 2, the same `scanned,KiB` delta in both subcases so one `assert.Contains` works as a
   cross-version sentinel, and realistic domain values for the new columns (`started_by = autovacuum`,
   `mode = aggressive`, per the user-spec value table).
5. Generate both goldens with the package `-update` flag — never hand-write them; they carry ANSI escape
   sequences in the header line.
6. Prove the `pg19` subcase is actually load-bearing before trusting it (see TDD Anchor), then confirm the
   three pre-existing progress goldens are byte-identical.

## TDD Anchor

The two replay subcases *are* the anchor — but because goldens are generated rather than hand-written, the
usual red-then-green loop is inverted. Do it this way:

- `report/report_record_progress_vacuum_test.go::Test_app_doReport_ProgressVacuum/pg18` — replaying an
  archive whose meta records `180000` produces the 13-column pre-19 layout; columns 10–11 (`scanned,KiB`,
  `vacuumed,KiB`) come out as `curr - prev`, every other column verbatim from the second tick.
- `report/report_record_progress_vacuum_test.go::Test_app_doReport_ProgressVacuum/pg19` — replaying an
  archive whose meta records `190000` produces the 15-column layout with `started_by`/`mode` present at
  indexes 4–5; the diffed pair is now columns 12–13, and the percentage columns 10–11 are copied verbatim,
  not diffed.

Loop to run:

1. Write both subcases against the Task 3 selector. Run without `-update` — they fail on the missing golden
   file. Confirm the failure is "golden missing", not a panic or a `diff failed` error.
2. Generate the goldens with `-update`. **Read both files by eye** before accepting them: the `pg18` header
   must list 13 columns and the `pg19` header 15 with `started_by`/`mode` in place; the diffed cells must
   show the deltas you chose, and the percentage cells must show the unchanged tick-2 values.
3. Prove the `pg19` case would catch the failure it exists for: temporarily force the stale interval
   (e.g. return `[2]int{10,11}` from the PG 19 branch of the selector, or drop the `progress_vacuum` case
   from `view.Configure`), re-run **without** `-update`, and confirm `pg19` goes red while `pg18` stays
   green. Revert the sabotage. If `pg19` stays green under sabotage the harness is broken — most likely the
   two ticks do not share a `pid` — and the test proves nothing.
4. Confirm the `pg18` golden reproduces today's semantics: `go test ./report/...` fully green with
   `git status` clean apart from the three new files.

## Acceptance Criteria

- [ ] `report/report_record_progress_vacuum_test.go` exists with `Test_app_doReport_ProgressVacuum` and
      exactly two subcases, `pg18` and `pg19`, driven by `versionNum` `"180000"` / `"190000"`.
- [ ] Both subcases run the full `app.doReport` pipeline over a synthetic in-memory tar — no live
      PostgreSQL, no fixture file on disk.
- [ ] The `pg19` subcase asserts the 15-column layout including `started_by` and `mode`; the `pg18` subcase
      asserts today's 13-column layout.
- [ ] Each subcase produces a delta in the correct diffed pair — 10–11 on pg18, 12–13 on pg19 — with the
      remaining columns copied from the second tick.
- [ ] `report/testdata/report_record_progress_vacuum_pg18.golden` and `…_pg19.golden` exist, are
      `-update`-generated, and are checked in.
- [ ] The sabotage check from the TDD Anchor step 3 was actually run and `pg19` went red for it.
- [ ] `go test ./report/...` is green.
- [ ] `report/testdata/report_progress_vacuum.golden`, `report_progress_analyze.golden` and
      `report_progress_basebackup.golden` are **byte-identical** to their committed versions — `git diff`
      shows nothing for them. A diff in any of these three is a red flag that Task 3's pre-19 branch
      changed behaviour, not expected churn; stop and report it rather than regenerating.
- [ ] `report/testdata/pgcenter.stat.golden.tar` is untouched.
- [ ] No file outside `report/` is modified. In particular `internal/stat/postgres.go` has no new guard —
      tech-spec Decision 5b.

## Context Files

**Feature artifacts:**
- [012-feat-pg19-compatibility-baseline.md](012-feat-pg19-compatibility-baseline.md) — user-spec; PG 19
  column layouts (`:86`), value domains (`:107-110`), and the criterion that existing goldens must not be
  regenerated (`:150`)
- [012-feat-pg19-compatibility-baseline-tech-spec.md](012-feat-pg19-compatibility-baseline-tech-spec.md) —
  Task 6 entry, Testing Strategy, Decision 5b, Risks table
- [012-feat-pg19-compatibility-baseline-code-research.md](012-feat-pg19-compatibility-baseline-code-research.md) —
  **section F** is the primary source for this task: harness anatomy (F.1), exact subcase content (F.2),
  golden naming (F.3), what must not change (F.4). Section 3.3 explains how `DiffIntvl` is consumed.
- [012-feat-pg19-compatibility-baseline-decisions.md](012-feat-pg19-compatibility-baseline-decisions.md) —
  decisions log; check Task 3's entry for any deviation in the final column order

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — what pgcenter is, supported versions
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout, record/report
  data flow, PG version handling
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — testing conventions, version-specific
  query pattern, "Adding a New PostgreSQL Version"

**Code files:**
- [report/report_record_progress_vacuum_test.go](../../../report/report_record_progress_vacuum_test.go) — new
  file, the test to write
- [report/testdata/report_record_progress_vacuum_pg18.golden](../../../report/testdata/report_record_progress_vacuum_pg18.golden) — new, generated
- [report/testdata/report_record_progress_vacuum_pg19.golden](../../../report/testdata/report_record_progress_vacuum_pg19.golden) — new, generated
- [report/report_record_bgwriter_test.go](../../../report/report_record_bgwriter_test.go) — the model to copy
- [report/report.go](../../../report/report.go) — `newApp` (`:83`), the replay loop and `views.Configure` from
  the archive's version (`:249-267`), `readMeta` (`:394`), `isFilenameOK`
- [report/report_test.go](../../../report/report_test.go) — package-level `var update` flag (`:22`); the
  golden-tar cases for the three progress reports (`:107`, `:124`, `:129`)
- [internal/query/progress_vacuum.go](../../../internal/query/progress_vacuum.go) — the two query constants
  and the selector from Task 3; source of truth for column aliases
- [internal/stat/postgres.go](../../../internal/stat/postgres.go) — `Compare`/`diff` (`:575-660`); read for
  understanding only, do not modify

## Verification Steps

- `go test ./report/ -run Test_app_doReport_ProgressVacuum -v` — both subcases pass.
- `go test ./report/...` — the whole report package is green, including the golden-tar cases for
  `progress_vacuum`, `progress_analyze` and `progress_basebackup`.
- `git status --short report/testdata/` — shows only the two new golden files. If any pre-existing
  `.golden` appears as modified, the task is not done: investigate, do not regenerate.
- Sabotage rerun (TDD Anchor step 3) was performed and reverted — state the observed result in the
  decisions-log entry.
- `make test` — full suite still green (this test needs no database, so it runs everywhere).

## Details

**Files:**

- `report/report_record_progress_vacuum_test.go` (new) — package `report`. One table-driven test,
  `Test_app_doReport_ProgressVacuum`, two subcases. Structure it exactly like
  `report_record_bgwriter_test.go`: testcase fields `name`, `versionNum`, `versionStr`, `cols []string`,
  `prevVals`, `currVals`, `wantFile`; a `metaRes` `stat.PGresult` with the 7-column
  `SelectCommonProperties` shape (only index 1, `version_num`, is read by `readMeta`); two `stat.PGresult`
  ticks with identical `Cols` and growing `Values`; a `sysinfo` payload; the tar assembly; a `Config`
  bracketing the tick timestamps; `newApp` + `app.writer = &buf` + `app.doReport`. Keep the
  file-level doc comment in the same spirit as the bgwriter one — it is what tells the next reader why the
  two ticks are one second apart.
- `report/testdata/report_record_progress_vacuum_pg18.golden`,
  `report/testdata/report_record_progress_vacuum_pg19.golden` (new) — generated with
  `go test ./report/ -run Test_app_doReport_ProgressVacuum -update`. Flat directory, `pgNN` naming matching
  the `report_record_bgwriter_pg{14,17,18}.golden` trio. They contain ANSI SGR escapes in the header line
  (compare `report/testdata/report_progress_vacuum.golden:1`) — hand-writing them will not reproduce.

**Dependencies:**

- Task 3 must be merged first: it owns `PgStatProgressVacuumPG19`, `SelectStatProgressVacuumQuery` and the
  `progress_vacuum` case in `view.Configure()`. Without the `Configure` case the `pg19` subcase produces the
  stale-interval output this test exists to reject.
- Task 5 (`report/describe.go`) is a sibling in the same wave and touches a different file — no conflict.
- No new Go modules. `archive/tar`, `bytes`, `database/sql`, `encoding/json`, `os`, `regexp`, `testing`,
  `time`, `internal/stat`, `testify/assert` — all already used by the model file.

**Edge cases — each of these makes the test green while proving nothing:**

- **Different `pid` between the two ticks.** `UniqueKey` for `progress_vacuum` is 0, i.e. `pid`. `diff()`
  matches rows by that key; when no match is found it copies the whole row verbatim
  (`internal/stat/postgres.go:650-656`). The test still passes, the golden still looks plausible, and no
  diffing happened at all. Use one row with a fixed `pid` in both ticks.
- **Tar entry basename ≠ report type.** `isFilenameOK` filters entries by the leading component; a
  mismatched basename means the stat entries are skipped and only headers are printed. Entries must be
  `progress_vacuum.<ts>.json` with `ReportType: "progress_vacuum"`.
- **Ticks not exactly one second apart.** The interval becomes the rate divisor; at anything other than 1 s
  the golden numbers stop being bare `curr - prev` and the cross-version delta sentinel breaks. Use the
  recorder's `20060102T150405.000` timestamp format with a one-second gap.
- **`TsStart`/`TsEnd` not bracketing the tick timestamps** — samples are filtered out and the report is
  empty.
- **Only one tick.** The first sample is always swallowed as the `prev` snapshot; two ticks are the minimum
  that prints a data row.
- **Column names transcribed from a spec table instead of the query.** The aliases carry commas and units
  (`"size_total,KiB"`, `"scanned_total,%"`); they must match what Task 3's constants emit, since printing
  and alignment are driven by them.
- **Same delta value chosen for both diffed columns** — a single `Contains` assert then cannot distinguish
  them. Give `scanned,KiB` and `vacuumed,KiB` different deltas, and keep the `scanned,KiB` delta identical
  across the two subcases so it works as the cross-version sentinel.

**Implementation hints:**

- Regenerate goldens with: `go test ./report/ -run Test_app_doReport_ProgressVacuum -update`. The `update`
  flag is declared once for the package at `report/report_test.go:22` — do not declare another one.
- Sentinel asserts before the golden comparison (mirroring `report_record_bgwriter_test.go:220-228`): a
  `\d{4}/\d{2}/\d{2}` regexp for the timestamp header line, one `Contains` on a stable cell (the `pid`),
  and one `Contains` on the known delta. Their job is to localise a future failure to "row missing" vs
  "header only" before the reader has to stare at a golden diff.
- Fixture value guidance from research section F.2 and the user-spec value table: `started_by =
  autovacuum`, `mode = aggressive`, percentages as text (`"50.00"`, `"25.00"`), `scanned,KiB` growing by a
  fixed amount in **both** subcases, `vacuumed,KiB` a different growth. Pick the numbers so the delta's
  digit string is **not** a substring of any raw value the report prints — with `1000` → `1500` the delta
  `500` also appears inside `1500`, so a `Contains` sentinel on it passes even when the delta is wrong,
  which is precisely the failure this test exists to catch. `1000` → `1700` has no such overlap. `query` a short
  `autovacuum: VACUUM …` string that survives `TruncLimit: 32`.
- The reason `pg18` and not `pg14` is the pre-19 subcase: `180000` is the boundary version below the
  selector's `>= PostgresV19` branch, so the pair `180000` / `190000` pins the switch exactly where it
  happens. This mirrors the bgwriter table's boundary choices.
- Do not add subcases for `progress_analyze` or `progress_basebackup` — the tech-spec scopes this task to
  the vacuum screen, which is the one whose `DiffIntvl` moves.

## Reviewers

- **dev-code-reviewer** → `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline-task-06-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline-task-06-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [012-feat-pg19-compatibility-baseline-decisions.md](012-feat-pg19-compatibility-baseline-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов) — включая результат sabotage-проверки и подтверждение, что три существующих progress-эталона не изменились
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
