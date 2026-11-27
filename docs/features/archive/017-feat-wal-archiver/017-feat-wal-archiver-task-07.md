---
status: done                       # planned -> in_progress -> done
depends_on: ["04", "05"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 3                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — `go test ./report/...`; `report -d -W a` и `report -d -W w` печатают новый текст # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 07: report describe text for archiver and the wal FPI row

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

`pgcenter report -d` prints a static, per-report-type column description. This task makes it cover the
two things the feature adds to the report side:

1. **A new `pgStatArchiverDescription` constant** documenting all **9** columns of the `archiver`
   screen, plus its `"archiver"` entry in `describeReport`'s map (`report/report.go:666-693`). Without
   the map entry, `pgcenter report -d -W a` prints `unknown description requested` **and exits zero** —
   `describeReport` returns a nil error for an unknown report type (that is why `Test_describeReport`
   asserts `NoError` on its `"invalid"` row). A missing entry is therefore invisible to every exit-code
   check; only a test that names `"archiver"` catches it.
2. **The `fpi,KiB` row** in `pgStatWALDescription` (`report/describe.go:141-157`), which is what
   satisfies the user-spec criterion "`pgcenter report -d -W w` описывает в том числе колонку
   `fpi,KiB`" for the PG 19 column added by Task 2.

**A property of the existing design that this task must not "fix".** The describe text is a **single
static constant per report type, with no version awareness at all** — `describeReport` receives only a
report-type string, never a version, and `RunMain` short-circuits on `c.Describe`
(`report/report.go:44-47`) before it ever opens an archive, so there is no version to be aware of on
this path. The constant already documents `write`, `sync`, `write,ms` and `sync,ms`, which PG 18
removed from `pg_stat_wal`. Consequently the new `fpi,KiB` row will also be printed when describing a
PG 14–18 archive. **That is the existing contract of this feature area, recorded as such in the
user-spec's acceptance criteria and in the tech-spec's Task 7 description — not a regression
introduced here.** Making describe version-aware is explicitly out of scope: do not add a version
parameter, do not split the constant, do not add conditional assembly. The cheapest honest option — and
the one the tech-spec settled on — is a `(PG 19+)` annotation on the new row, which leaves the constant
no less accurate than it is today.

The describe path is also the only place a user reads what the archiver columns mean: the TUI screen
shows bare headers, and the README documents no `pgcenter report` flags at all (Decision 13 keeps it
that way). So the row text and, above all, the **row order** are the documentation.

## What to do

**1. Add `pgStatArchiverDescription` to `report/describe.go`.**

- Place it next to `pgStatWALDescription` (`:140-157`), so a reader finds the two WAL-area descriptions
  together.
- Follow the existing format exactly: a leading sentence naming the source view, a
  `  column<TAB>origin<TAB>description` header, one `- name<TAB>origin<TAB>text` row per column, and a
  trailing `Details: <docs URL>` line. The URL for this screen is
  `https://www.postgresql.org/docs/current/monitoring-stats.html#PG-STAT-ARCHIVER-VIEW`.
- **The file uses literal tab characters as column separators.** Preserve them; do not let an editor
  expand them to spaces (that would silently break the tab-anchored order tests below and misalign the
  printed table).
- Cover all **9** columns, in the order the query emits them. Take the aliases and their order from the
  actual query in `internal/query/archiver.go` (Task 1's file, merged in wave 1) — not from the
  tech-spec table — so the description documents the layout that really ships. The expected set is
  `source`, `ready`, `archived`, `last_archived`, `archived_age`, `failed`, `last_failed`,
  `failed_age`, `stats_age`.
- Origins: `-` for the literal `source` column, `pg_ls_archive_statusdir` for `ready`, and the
  `pg_stat_archiver` field name for the remaining seven (`archived_count`, `last_archived_wal`,
  `last_archived_time`, `failed_count`, `last_failed_wal`, `last_failed_time`, `stats_reset`).
- Say in the `archived_age` / `last_archived` / `failed_age` / `last_failed` rows that the value is
  **empty when the cluster has never archived / never failed** — that is the blank-cell behaviour the
  user-spec promises, and describe is where a user finds out a blank is not an error.

**2. Add the map entry in `report/report.go`.** One line in `describeReport`'s map, next to
`"wal": pgStatWALDescription` (`:674`). Map literal order is semantically irrelevant; adjacency is for
the reader.

**3. Add the `fpi,KiB` row to `pgStatWALDescription`.** Insert it **immediately after** the `fpi` row
(`report/describe.go:148`), mirroring the on-screen position — the PG 19 query places `fpi,KiB` right
after the `fpi` counter. Origin `wal_fpi_bytes`; annotate the row `(PG 19+)`.

**4. Tests in `report/report_test.go`** — see TDD Anchor. Extend the existing `Test_describeReport`
table with the archiver row, and add the two column-order tests, following the shape already
established by `Test_describeActivityColumnOrder` (`report/report_test.go:1255-1279`) and
`Test_describeProgressColumnOrder` (`:1216-1254`) — including their tab-anchored markers and their
`require.NotEqual(-1, pos)` presence check before the ordering assertion.

**5. Change nothing else.** No new report type wiring, no `-W` parsing (Task 4), no golden files
(Task 8), no query edits (Tasks 1 and 2). If the archiver query's column names turn out to differ from
the list above, follow the query and say so in the completion report — do not edit the query.

## TDD Anchor

Тесты, которые нужно написать ДО реализации. Пишем → запускаем → убеждаемся что падают → пишем код →
убеждаемся что проходят.

- `report/report_test.go::Test_describeReport` — add `{report: "archiver", want: pgStatArchiverDescription}`
  next to the `"wal"` row (`:1184`). Written first, it fails to **compile** (the constant does not
  exist), which is this table's red state; once the constant exists but the map entry does not, it
  fails on the assertion, printing `unknown description requested`.
- `report/report_test.go::Test_describeArchiverColumnOrder` — the nine archiver rows are present and
  appear in the query's column order. Marker per column: `"\n- " + name + "\t"` — the leading `"\n- "`
  keeps the marker off names occurring inside prose, and the **trailing tab** keeps a short name off a
  longer row (without it `"\n- archived"` also matches the `- archived_age` row and the compared
  offsets are not the ones being checked; this is exactly the trap documented at
  `report_test.go:1266-1269`). Presence is asserted with `require.NotEqual(t, -1, pos, …)` **before**
  the ordering comparison, because `strings.Index` returns `-1` for a missing marker and `-1` is less
  than everything, so an ordering-only assertion passes on a row that is not there at all.
- `report/report_test.go::Test_describeWALColumnOrder` — the same shape for `pgStatWALDescription`,
  with the full marker list `source`, `waldir_size`, `wal,KiB`, `records`, `fpi`, `fpi,KiB`, `write`,
  `sync`, `write,ms`, `sync,ms`, `buffers_full`, `stats_age`. This is the PG 14 superset layout the
  constant documents (see Description); the point of the test is that `fpi,KiB` exists and sits between
  `fpi` and the next column. The trailing tab is load-bearing twice here: `"\n- fpi\t"` must not match
  the `- fpi,KiB` row, and `"\n- write\t"` must not match `- write,ms`.

**Each of these must be seen red before the code is written**, per patterns.md "Extract the decision out
of the unreachable closure": the acceptance criterion for a test that guards an invariant is a named
mutation that reddens it, run and observed — not a green suite. The three mutations are listed in
Acceptance Criteria and must actually be executed and reverted.

## Acceptance Criteria

- [ ] `pgStatArchiverDescription` exists in `report/describe.go`, documents all 9 archiver columns in
      the order `internal/query/archiver.go` emits them, uses literal tabs as separators, and ends with
      the `PG-STAT-ARCHIVER-VIEW` docs URL.
- [ ] `describeReport`'s map has an `"archiver"` entry, and `pgcenter report -d -W a` prints the
      description without `-f` and without touching any archive.
- [ ] `pgStatWALDescription` has an `fpi,KiB` row with origin `wal_fpi_bytes`, placed immediately after
      the `fpi` row and annotated `(PG 19+)`.
- [ ] **Mutation 1 — the map entry.** Deleting `"archiver": pgStatArchiverDescription` from
      `describeReport`'s map turns `Test_describeReport` **red**. It must not merely change the printed
      text: with the entry removed the command still exits zero, so a red test is the only detection
      there is. Run it, see red, restore.
- [ ] **Mutation 2 — a deleted row.** Deleting any single row from `pgStatArchiverDescription` turns
      `Test_describeArchiverColumnOrder` **red** on the presence assertion (not on ordering). Run it for
      at least one row, see red, restore.
- [ ] **Mutation 3 — the FPI row.** Deleting the `fpi,KiB` row from `pgStatWALDescription`, and
      separately moving it to before the `fpi` row, each turn `Test_describeWALColumnOrder` **red** —
      the first on presence, the second on order. Run both, see red, restore.
- [ ] No version awareness is introduced: `describeReport` keeps its `(w io.Writer, report string)`
      signature, and neither constant is assembled conditionally. The `fpi,KiB` row printing for a
      PG 14–18 archive is accepted, documented behaviour.
- [ ] Only `report/describe.go`, `report/report.go` and `report/report_test.go` are modified. No query,
      view, CLI, golden or record file is touched.
- [ ] `go test ./report/...` is green (this package needs no PostgreSQL fixtures), `make lint` and
      `make vuln` are clean, `make build` succeeds.

## Context Files

**Feature artifacts:**
- [017-feat-wal-archiver.md](017-feat-wal-archiver.md) — user-spec: `pgcenter report -d -W a` in
  "Команды CLI" (`:190`), and the acceptance criterion at `:306` that states the non-version-aware
  describe contract in so many words
- [017-feat-wal-archiver-tech-spec.md](017-feat-wal-archiver-tech-spec.md) — Implementation Tasks →
  Wave 3 → **Task 7** (`:591-601`); **Data Models** (`:317-339`) for the 9 archiver columns and the
  PG 19 wal layout; Decision 14 (column header names); Decision 13 (documentation scope — README is
  deliberately out)
- [017-feat-wal-archiver-decisions.md](017-feat-wal-archiver-decisions.md) — decisions log
- [017-feat-wal-archiver-code-research.md](017-feat-wal-archiver-code-research.md) — **§4.2** (the
  describe map and constant, and why `pgStatWALDescription` is already version-approximate) and
  **§10.E.4 / §10.E.5 / §10.E.6** (the exact map entry, a drafted constant, the insertion point of the
  FPI row, and the test row to add)

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — what pgcenter is and what
  `report` does (this repo has no `project.md`; `overview.md` is its equivalent)
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout and the
  record/report data flow
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — **"Extract the decision out of
  the unreachable closure"** (`:62-87`): when a test guards an invariant, name the mutation it must
  fail on and see red before believing the green; also "Version-Specific Query Pattern" and testing
  conventions

**Code files:**
- [report/describe.go](../../../report/describe.go) — add `pgStatArchiverDescription`; edit
  `pgStatWALDescription` (`:140-157`, the `fpi` row is `:148`). Format references:
  `pgStatBgwriterDescription` (`:456`), `pgStatIOTimeDescription` (`:529`)
- [report/report.go](../../../report/report.go) — `describeReport` (`:665-698`), its map (`:666-693`),
  the `"wal"` entry (`:674`); `RunMain`'s describe short-circuit (`:44-47`) — the reason `-d -W a`
  needs no `-f`
- [report/report_test.go](../../../report/report_test.go) — `Test_describeReport` (`:1172-1214`, the
  `"wal"` row at `:1184`), `Test_describeProgressColumnOrder` (`:1216-1254`) and
  `Test_describeActivityColumnOrder` (`:1255-1279`) — the marker/presence idiom to copy
- [internal/query/archiver.go](../../../internal/query/archiver.go) — **source of truth for the column
  names and their order** (created by Task 1, wave 1). Read-only here
- [internal/query/wal.go](../../../internal/query/wal.go) — the PG 14 / PG 18 / PG 19 branches; confirms
  `fpi,KiB` follows `fpi` and that the columns the constant documents differ per version. Read-only
- [cmd/report/report.go](../../../cmd/report/report.go) — Task 4's `-W w|a` mapping; read-only, needed
  only to run the CLI smoke check

## Verification Steps

- `go test ./report/...` — green. This package has no PostgreSQL fixture dependency (no
  `NewTestConnect` in any `report/*_test.go`), so a full package run is a valid gate here, unlike
  `top/`.
- `go test ./report/ -run Test_describe -v` — the three describe tests, including the two new
  column-order subtests, all pass and are actually executed (check they appear in the `-v` output; a
  misnamed test is silently skipped by `-run`).
- **Run the three mutations from Acceptance Criteria** and confirm each reddens the named test before
  restoring the code. Record in the completion report that they were run.
- `make build`, then:
  - `./bin/pgcenter report -d -W a` — prints the archiver description, with no `-f` and no archive
    present, exits 0.
  - `./bin/pgcenter report -d -W w` — prints the wal description including the `fpi,KiB` row.
  - Both depend on Task 4's string `-W` flag being merged (wave 1). If `-W` is still a bool, that is a
    dependency problem, not a defect in this task.
- `./bin/pgcenter report -d -W a | cat -A | head -20` — verify the separators are real tabs (`^I`), not
  spaces, and that the table aligns the same way `-d -W w` does.
- `git diff --stat` — exactly three files: `report/describe.go`, `report/report.go`,
  `report/report_test.go`.
- `make lint` and `make vuln` — clean.

## Details

**Files:**
- `report/describe.go` — one new constant next to `pgStatWALDescription`, plus one inserted row inside
  `pgStatWALDescription`. Both are raw string literals: `gofmt` will not touch their contents, so
  alignment is entirely on the author.
- `report/report.go` — exactly one added line in `describeReport`'s map.
- `report/report_test.go` — one added table row in `Test_describeReport`, two new test functions.

**Dependencies:**
- **Task 1** (wave 1) provides `internal/query/archiver.go` — the authoritative column names/order.
- **Task 2** (wave 1) adds the PG 19 `fpi,KiB` column this task documents.
- **Task 4** (wave 1) makes `-W` a string flag, which is what makes the `-d -W a` smoke check runnable.
- **Task 5** (wave 2, declared dependency) registers the `archiver` view, which is what makes
  `"archiver"` a real report type end to end; describe itself does not consult the view registry.
- Downstream: **Task 8** adds golden replay tests for the same two screens; it touches no file this
  task owns.
- No new packages. `strings`, `require` and `assert` are already imported in `report/report_test.go`.

**Edge cases:**
- **`-d` with no `-f`.** `RunMain` returns before `os.Open` (`report/report.go:44-47`), so the describe
  path never reads a file. Do not add an input-file requirement.
- **Unknown report type.** `describeReport` prints `unknown description requested` and returns **nil**.
  Keep that behaviour; it is what the existing `"invalid"` test row pins. It is also why the missing
  map entry is invisible without a test.
- **Describing a PG 14–18 archive.** The `fpi,KiB` row is printed anyway. Accepted, see Description.
- **Row-name collisions in the markers.** `fpi` / `fpi,KiB`, `write` / `write,ms`, `sync` / `sync,ms`,
  `archived` / `archived_age`, `last_archived` / `last_failed`. The trailing-tab anchor is what keeps
  each marker on its own row; drop it and the order test starts comparing the wrong offsets while
  staying green.
- **Trailing whitespace.** Two existing wal rows end with a trailing space (`:146`, `:151`). Do not
  clean them up — that is unrelated churn, and `golangci-lint` does not flag raw string contents.

**Implementation hints:**
- §10.E.5 of the code research carries a **drafted** constant and the exact FPI row text. Treat it as a
  starting point, then reconcile every alias against `internal/query/archiver.go` as it was actually
  merged — the draft predates Task 1's implementation.
- Column-width alignment: pick the tab stops so the `origin` column lines up for all nine rows, the way
  the neighbouring constants do. `last_archived` and `archived_age` are the long names that set the
  width. Check the result visually with `cat -A` or by running `-d -W a`, not by eyeballing the source.
- The `ready` column's origin is a **function**, not a `pg_stat_archiver` field —
  `pg_ls_archive_statusdir`. Naming it in the origin column is what tells a user why that one cell can
  fail on a role without `pg_monitor` (the screen-wide failure of Decision 4).
- Security surface of this change is essentially nil, and say so plainly if asked: `describeReport`
  writes a compile-time constant to `app.writer`, consumes no user input beyond the report-type
  string used as a map key, opens no file, and runs no SQL. The docs URL is text in a constant, never
  fetched.
- Do not extend `Test_describeProgressColumnOrder`'s table with the two new screens — it is scoped to
  the progress descriptions by its own comment. Separate, named tests keep a failure message that says
  which screen broke.

## Reviewers

- **dev-code-reviewer** → `017-feat-wal-archiver-task-07-dev-code-reviewer-review.json`
- **dev-security-auditor** → `017-feat-wal-archiver-task-07-dev-security-auditor-review.json`
- **dev-test-reviewer** → `017-feat-wal-archiver-task-07-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [017-feat-wal-archiver-decisions.md](017-feat-wal-archiver-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
