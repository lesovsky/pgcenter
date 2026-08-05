---
status: planned                    # planned -> in_progress -> done
depends_on: ["05"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 3                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — `go test ./report/...`; the goldens turn red under each named mutation
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 08: Golden replay tests for archiver and wal

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Give the two screens this feature touches end-to-end replay coverage: `report` fed by a synthetic
in-memory tar, output compared against golden files. This is the shape ADR [008] fixed for exactly this
situation (`docs/decisions-log.md:462`) — one purpose-built `_test.go` per screen plus its own goldens,
no live PostgreSQL, the recording's `meta` version driving the report-time `view.Configure` layout
switch. The legacy shared fixture `report/testdata/pgcenter.stat.golden.tar` is a 2021 PG14beta1
recording that contains no `archiver` entries and cannot be extended without churning ~30 existing
goldens; it is not touched here.

Two files, three goldens:

- **`report/report_record_archiver_test.go` + one golden.** The `archiver` screen is
  version-independent (`SelectStatArchiverQuery` ignores its parameter), so it gets a single golden with
  no version suffix — the `report_record_stat_io_time.golden` convention
  (`report/report_record_statio_test.go:177-206`). What this screen makes testable, and no other replay
  test in the tree does, is **pass-through**: `DiffIntvl{0,0}` short-circuits `calculateDelta` before
  `diff()` is ever entered (`internal/stat/postgres.go:589-597`), so every column — the `'Archiver'`
  literal, the two cumulative counters, the two 24-character WAL names, the three ages — is copied from
  the current tick verbatim. The assertions must therefore pin **absolute current values, not deltas**.
  That is the assertion that catches someone later "helpfully" giving the screen a diffed range.

- **`report/report_record_wal_test.go` + PG 18 and PG 19 goldens.** The `wal` screen is version-aware and
  this feature changes its PG 19 layout (Task 2 adds `fpi,KiB` inside the diffed range, 7 cols/`{2,5}` →
  8 cols/`{2,6}`). It has **no replay coverage at all today** — pre-existing test debt, and closing it is
  the reason this task exists as a separate task rather than a line in Task 2. Unlike the `stat_io` pair,
  whose two branches share one shape and whose own doc comment admits the replay test cannot detect a
  wrong branch, the `wal` branches differ in both `Ncols` and `DiffIntvl`, so this replay **does** prove
  the version switch: feed an 8-column PG 19 sample, let `Configure` pick the PG 18 branch, and the
  diffed range lands on the wrong columns and the golden moves.

Two behaviours of the report path the tests must respect rather than fight — both read from the code,
both already decided:

1. **The first sample of a run is discarded.** `processData` takes the `!prevStat.Valid || versionChanged`
   branch and ends it with `continue` (`report/report.go:277-319`), so an N-tick recording prints N−1
   rows — including on `archiver`, which diffs nothing (Decision 12). Build every fixture with at least
   two ticks and expect one row per surplus tick.
2. **A recording with no matching entries prints nothing at all** — not a header, not a notice
   (Decision 15). `printStatHeader` returns early while `v.Aligned` is false, and alignment happens
   inside the data branch. Note also that the three `INFO:` lines live in `printReportHeader`
   (`report/report.go:63`, `:541`), *outside* `doReport`, so a test driving `app.doReport` directly sees
   an empty buffer for an empty archive.

**The honesty boundary, which must be stated in the test files and must not be overclaimed in the
report:** replay reads recorded `stat.PGresult` JSON. The column names and their order come from the
fixture the test itself writes, never from the SQL. So reordering aliases inside
`internal/query/archiver.go` or `internal/query/wal.go` does **not** redden these tests — that layout is
pinned by the query and view unit tests of Tasks 1, 2 and 5. What these tests pin is the rendering and
diff pipeline plus the version-driven `Configure` switch (`Ncols`, `DiffIntvl`, `OrderKey`, `UniqueKey`).
Say so in each file's doc comment, the way `report_record_statio_test.go:50-60` says it about `stat_io`.

## What to do

- Write `report/report_record_archiver_test.go` with `Test_app_doReport_Archiver`, three subcases:
  - **populated** — a cluster that has archived and has failures; compared against
    `report/testdata/report_record_archiver.golden`;
  - **never archived** — the four NULL-able columns arrive as `sql.NullString{Valid: false}` in both
    ticks; **no golden**, pinned by a field-count assertion (see TDD Anchor for why that is the stronger
    pin here, and Details for why no fourth golden is added);
  - **empty archive** — a tar carrying `meta.*` and `sysinfo.*` but no `archiver.*` entry; `doReport`
    returns nil and writes nothing.
- Write `report/report_record_wal_test.go` with `Test_app_doReport_WAL` in the table form of
  `Test_app_doReport_Bgwriter`: two subcases (`pg18`, `pg19`), one golden each,
  `report/testdata/report_record_wal_pg18.golden` and `report/testdata/report_record_wal_pg19.golden`.
- Build every fixture the way `report/report_record_bgwriter_test.go:141-202` does: a 7-column `meta`
  `PGresult` whose `Values[0][1]` carries `version_num`, per-tick entries named
  `<view>.20060102T150405.000.json`, the per-tick order `meta.*` → `<view>.*` → `sysinfo.*`, and the two
  ticks **exactly one second apart** so the rate divisor `itv == 1` and every delta is a bare
  `curr - prev`.
- Generate the three goldens with the package's shared `-update` flag
  (`report/report_test.go:24`), then **read them** — a golden accepted without being looked at pins
  whatever the code did, including a bug. Check by eye: the header line carries the expected column
  names in order, the archiver row shows absolute values, the pg19 row shows `fpi,KiB` with a delta.
- Add the cheap pre-golden invariants each existing replay test carries (`bgwriter_test.go:218-228`):
  non-empty output, a `\d{4}/\d{2}/\d{2}` timestamp line, and one or two `assert.Contains` sentinels
  chosen so a failure reads as "row missing" or "delta wrong" rather than "golden differs".
- Run every mutation listed in Acceptance Criteria, confirm the named golden goes **red**, revert. A
  golden that cannot fail is worth nothing, and that is a rule in this project
  (`patterns.md` → "Extract the decision out of the unreachable closure").
- Touch nothing else. `report/report.go`, `report/describe.go` and `report/report_test.go` belong to
  Task 7 in this same wave; the two new `_test.go` files and the three new goldens are this task's whole
  footprint.

## TDD Anchor

<!-- Fill if task includes writing code. For non-code tasks (user instructions, deploy, config) — delete this section. -->

Тесты, которые нужно написать ДО реализации. Пишем → запускаем → убеждаемся что падают → пишем код → убеждаемся что проходят.

This task *is* tests, so "red first" means something specific: write each subcase with its assertions
**before** generating its golden, run it, and confirm it fails on the missing golden file and on the
value sentinels. Only then run `-update`. Never write the assertion after seeing the output — that is how
a test ends up asserting the bug.

- `report/report_record_archiver_test.go::Test_app_doReport_Archiver/populated` — two ticks, 9 columns,
  `versionNum: "170000"` (any supported version; 17 makes it plain the screen does not branch).
  Values are chosen so that a pass-through row and a diffed row are impossible to confuse:
  `archived` `100000` → `100003`, `ready` `10` → `14`, `failed` `5` → `8`, `last_archived`
  `000000010000000000000021` → `000000010000000000000024`. Asserts the output contains `Archiver`,
  `100003` and `000000010000000000000024` — the **current absolute** values — and matches
  `testdata/report_record_archiver.golden`. A delta sentinel is deliberately not used: the would-be
  delta `3` is a substring of `100003`, so `NotContains` on it would be vacuous.
- `report/report_record_archiver_test.go::Test_app_doReport_Archiver/never_archived` — two ticks where
  `last_archived`, `archived_age`, `last_failed`, `failed_age` are `sql.NullString{Valid: false}` and the
  counters are `0`. Asserts the header line still names all nine columns, and that the single data line,
  ANSI-stripped, splits into exactly **five** `strings.Fields` — `Archiver`, `0`, `0`, `0`, `02:00:00`.
  That is the direct machine reading of the user-spec's "колонки пустые (не `0` и не прочерк)": if the
  cells rendered `0`, `-` or `n/a` the count would be nine. Also asserts the output contains no `n/a`.
- `report/report_record_archiver_test.go::Test_app_doReport_Archiver/no_archiver_entries` — a tar with
  `meta.*` + `sysinfo.*` for two ticks and no `archiver.*` entry at all. Asserts `doReport` returns nil
  and the buffer is **empty** — no column header, no notice (Decision 15; the `INFO:` lines are printed
  outside `doReport`, so "empty" is literal here).
- `report/report_record_wal_test.go::Test_app_doReport_WAL/pg18` — `versionNum: "180000"`, 7 columns
  `source, waldir_size, wal,KiB, records, fpi, buffers_full, stats_age`, `DiffIntvl{2,5}` supplied by
  `Configure`. Asserts the output contains `WAL`, the `records` delta `500`, the pass-through
  `waldir_size` string `1088 MB` and the pass-through `stats_age` `02:00:00`; asserts it does **not**
  contain `fpi,KiB`; matches `testdata/report_record_wal_pg18.golden`.
- `report/report_record_wal_test.go::Test_app_doReport_WAL/pg19` — `versionNum: "190000"`, 8 columns with
  `fpi,KiB` between `fpi` and `buffers_full`, `DiffIntvl{2,6}`. Asserts the same cross-version `500`
  sentinel, that the header contains `fpi,KiB`, and that its **diffed** value `240.25` is present
  (`840.25 - 600.00`, formatted by `diffPair` as `%.2f`) while the absolute `840.25` is not; matches
  `testdata/report_record_wal_pg19.golden`.

## Acceptance Criteria

Written as mutations, per `patterns.md` → "Extract the decision out of the unreachable closure": a golden
is believed only after the named change has been applied and the suite observed **red**. Apply each one,
see red, revert. Class A mutations touch production code — they prove the tests guard real behaviour.
Class B mutations touch the test's own fixture — they prove the goldens are compared byte-for-byte and are
not vacuous. Both classes are required.

- [ ] `report/report_record_archiver_test.go` and `report/report_record_wal_test.go` exist; the three
      goldens exist; no other file in the tree is modified.
- [ ] `go test ./report/...` is green on the host, with no PostgreSQL running.
- [ ] The archiver test has all three subcases and the wal test both version subcases; every subcase
      actually ran (check `-v` output, do not trust a bare `ok`).
- [ ] Goldens were generated with `-update` and then read by a human eye; the archiver golden shows the
      absolute values, the pg19 golden shows an `fpi,KiB` column.
- [ ] **A1** — change `SelectStatArchiverQuery`'s returned `DiffIntvl` from `[2]int{0, 0}` to
      `[2]int{2, 2}` (`internal/query/archiver.go`): `Test_app_doReport_Archiver/populated` turns red —
      `archived` renders the delta `3` instead of `100003`. This is the pass-through guard.
- [ ] **A2** — delete the PG 19 branch from `SelectStatWALQuery` so PG 19 falls through to the PG 18
      return (`PgStatWALDefault, 7, [2]int{2, 5}`): `Test_app_doReport_WAL/pg19` turns red. This is what
      makes the version switch a tested claim.
- [ ] **A3** — narrow the PG 19 `DiffIntvl` from `[2]int{2, 6}` to `[2]int{2, 5}`:
      `Test_app_doReport_WAL/pg19` turns red — `buffers_full` renders absolute instead of diffed. This
      pins that inserting `fpi,KiB` moved the end of the diffed range, i.e. that the new column sits
      **inside** it and did not push `buffers_full` out.
- [ ] **A4** — make `SelectStatWALQuery`'s PG 18 branch return the PG 14 values (`PgStatWALPG14, 11,
      [2]int{2, 9}`): `Test_app_doReport_WAL/pg18` turns red.
- [ ] **A5** — remove the `archiver` entry from `view.New()` (`internal/view/view.go`, Task 5's
      registration): `Test_app_doReport_Archiver` turns red on all three subcases. This is the guard on
      the dependency that makes this task Wave 3 — `newApp` resolves the view by report type
      (`report/report.go:83-85`) and an unregistered name yields a zero-value `view.View`.
- [ ] **B1** — swap two adjacent columns in the archiver fixture's `cols` slice and the matching values
      (e.g. `failed` and `last_failed`): the archiver golden turns red. Proves the golden pins column
      order and not merely the presence of a row.
- [ ] **B2** — change one digit of the archiver fixture's `curr` `archived` value: the golden turns red
      *and* the `100003` sentinel turns red.
- [ ] **B3** — move the wal pg19 ticks two seconds apart instead of one: the pg19 golden turns red (the
      rate divisor `itv` becomes 2 and every delta halves). Proves the one-second spacing is load-bearing
      and documented, not accidental.
- [ ] Fixture values are hostile to a wrongly widened diff range: `waldir_size` is a pretty string
      (`1040 MB` / `1088 MB`) and `stats_age` an interval (`01:00:00` / `02:00:00`), both of which fail
      `strconv.ParseInt` — so a `DiffIntvl` that swallows either produces `diff failed` rather than a
      plausible-looking number.
- [ ] The never-archived subcase asserts on the **field count** of the ANSI-stripped data line, not on a
      whole-line string equality, and the assertion fails if the blank cells render `0`, `-` or `n/a`.
- [ ] The empty-archive subcase asserts `doReport` returns nil **and** the buffer is empty — it does not
      assert on `INFO:` lines, which `doReport` never writes.
- [ ] Each file's doc comment states what the replay does and does not prove — in particular that column
      names come from the fixture, so a reordering of SQL aliases would not be caught here.
- [ ] `make lint` (golangci-lint + gosec) is clean on the host.
- [ ] `go test -race -p 1 ./report/...` inside the CI image is green (goldens carry raw bytes; a run under
      the race detector and a different `TZ`/locale must produce the same output — the fixtures use fixed
      timestamps for exactly this reason).

## Context Files

**Feature artifacts:**
- [017-feat-wal-archiver.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver.md) — user-spec:
  the archiver column set, the "пустые колонки" criterion, the PG 14–18 "набор колонок не изменился"
  criterion, the corrected scenario 3 ("one row per tick, except the first")
- [017-feat-wal-archiver-tech-spec.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver-tech-spec.md) —
  Task 8, Testing Strategy (E2E section), Data Models (the exact 9-column archiver layout and the PG 19
  wal layout), Decisions 2, 12, 14, 15
- [017-feat-wal-archiver-decisions.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver-decisions.md) —
  decisions log; read what Tasks 1, 2 and 5 recorded before writing fixtures against their names
- [017-feat-wal-archiver-code-research.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver-code-research.md) —
  §5.4 the report-test inventory, §10.F the harness anatomy and the per-file requirements, §8 the CI-image
  docker command
- [docs/decisions-log.md](docs/decisions-log.md) — ADR [008] "Replay tests: synthetic in-memory tar +
  golden files, not the legacy fixture" (`:462`), the governing decision for this task

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — what pgcenter is, which statistics it
  reports
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, the
  record → tar → report data flow, PG version handling
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — Testing section (golden files, the
  `-update` flag, table-driven subtests, the `io.Writer` seam that makes `doReport` testable),
  "Extract the decision out of the unreachable closure" (the mutation rule applied above)

**Code files:**
- [report/report_record_archiver_test.go](report/report_record_archiver_test.go) — **NEW**, create
- [report/report_record_wal_test.go](report/report_record_wal_test.go) — **NEW**, create
- [report/testdata/report_record_archiver.golden](report/testdata/report_record_archiver.golden) —
  **NEW**, generated with `-update`
- [report/testdata/report_record_wal_pg18.golden](report/testdata/report_record_wal_pg18.golden) —
  **NEW**, generated with `-update`
- [report/testdata/report_record_wal_pg19.golden](report/testdata/report_record_wal_pg19.golden) —
  **NEW**, generated with `-update`
- [report/report_record_bgwriter_test.go](report/report_record_bgwriter_test.go) — the template to copy:
  the version-aware table (`:31-132`), the meta `PGresult` (`:141-151`), `mkRow` (`:155-161`), the tar
  composition (`:188-202`), the `Config`/`newApp`/`doReport` drive (`:204-216`), the pre-golden
  invariants (`:218-228`), the `-update` branch (`:230-237`)
- [report/report_record_statio_test.go](report/report_record_statio_test.go) — the version-**independent**
  single-golden precedent (`:177-206`), the package-level ANSI strip helper `statIOStripANSI` (`:43-46`)
  that the never-archived subcase reuses, and the doc comment (`:50-60`) that states plainly what a
  replay test cannot prove — the model for this task's own honesty note
- [report/report.go](report/report.go) — read-only here: `newApp` view lookup (`:83-85`), the
  first-sample/version-change `continue` (`:277-319`), `formatStatSample`'s align-once rule (`:528-538`),
  `printStatHeader`'s `!v.Aligned` early return (`:560-563`), `printStatSample`'s timestamp line and cell
  padding (`:579-650`), `printReportHeader` living outside `doReport` (`:541`), `isFilenameOK` (`:460`)
- [report/report_test.go](report/report_test.go) — the shared `var update = flag.Bool("update", …)`
  (`:24`); **do not edit this file**, Task 7 owns it this wave
- [internal/query/wal.go](internal/query/wal.go) — the PG 14/18 constants and `SelectStatWALQuery`; Task 2
  adds the PG 19 branch this task replays
- [internal/query/archiver.go](internal/query/archiver.go) — Task 1's constant and selector; the source of
  the 9-column order the archiver fixture must mirror
- [internal/view/view.go](internal/view/view.go) — the `wal` registration (`:129-140`) and its `Configure`
  case (`:389-391`), plus Task 5's `archiver` registration that `newApp` resolves
- [internal/stat/postgres.go](internal/stat/postgres.go) — `calculateDelta`'s `{0,0}` pass-through
  (`:589-597`), `diff` (row pairing by `UniqueKey`, per-column interval test), `diffPair` (floats via
  `%.2f`, integers via integer division by `itv`)

## Verification Steps

<!-- How to verify task is complete. For code — run tests. For deploy — check logs. For user-action — user confirmation. -->

- Step 1 — red first: write both test files with their assertions and **no** goldens; run
  `go test ./report/ -run 'Test_app_doReport_(Archiver|WAL)' -v` and confirm every subcase fails on the
  missing golden file and that the value sentinels are present in the failure output.
- Step 2 — generate: `go test ./report/ -run 'Test_app_doReport_(Archiver|WAL)' -update`, then open all
  three goldens (`cat -v` shows the SGR escapes) and verify the header names, their order, the archiver
  row's absolute values and the pg19 `fpi,KiB` column and its delta.
- Step 3 — green: `go test ./report/...` on the host. Expected green with no PostgreSQL running; this
  package's replay tests need none.
- Step 4 — mutation gates, one at a time, reverting after each: A1–A5 (production code) and B1–B3
  (fixture). Each must turn the **named** subcase red. Green-only evidence does not close this task; the
  report must list which mutations were run and what turned red.
- Step 5 — for the record, the full suite inside the CI image (this task needs no cluster, but the goldens
  must survive the race detector and the container's environment):

  ```bash
  docker run --rm -v "$PWD":/work -v "$HOME/go/pkg/mod":/gomod \
    -e GOROOT=/gomod/golang.org/toolchain@v0.0.1-go1.25.12.linux-amd64 \
    -e GOPATH=/gopath -e GOFLAGS=-mod=mod -e HOME=/root \
    -w /work lesovsky/pgcenter-testing:0.0.11 bash -c '
      export PATH=$GOROOT/bin:$PATH
      prepare-test-environment.sh > /tmp/prep.log 2>&1 || { tail -30 /tmp/prep.log; exit 1; }
      go test -race -p 1 -timeout 300s ./report/...'
  ```

- Step 6 — `make lint` on the host: clean.
- Step 7 — confirm the pre-existing `Test_app_doReport` wal case
  (`report/report_test.go:73-77`, driven by the legacy PG13-era tar) is still green and its golden
  unchanged — the PG 19 branch must not reach a ~PG13 recording.

## Details

<!-- All details for task execution — technical, organizational, any other. -->

**Files:**

- `report/report_record_archiver_test.go` — **does not exist**, create. Package `report`. Imports mirror
  `report_record_bgwriter_test.go` (`archive/tar`, `bytes`, `database/sql`, `encoding/json`, `os`,
  `regexp`, `testing`, `time`, `internal/stat`, `testify/assert`) plus `strings` for the field-count
  assertion. Columns, in the order Task 1 locked and the Data Models table fixes:

  | # | column | populated prev → curr | never-archived (both ticks) |
  |---|---|---|---|
  | 0 | `source` | `Archiver` | `Archiver` |
  | 1 | `ready` | `10` → `14` | `0` |
  | 2 | `archived` | `100000` → `100003` | `0` |
  | 3 | `last_archived` | `…021` → `…024` (24-char WAL names) | NULL |
  | 4 | `archived_age` | `00:00:41` → `00:00:07` | NULL |
  | 5 | `failed` | `5` → `8` | `0` |
  | 6 | `last_failed` | `…019` (unchanged) | NULL |
  | 7 | `failed_age` | `00:12:02` → `00:14:02` | NULL |
  | 8 | `stats_age` | `01:00:00` → `02:00:00` | `01:00:00` → `02:00:00` |

- `report/report_record_wal_test.go` — **does not exist**, create. Table-driven over two versions:

  | subcase | `versionNum` / `versionStr` | Ncols | `DiffIntvl` (from `Configure`) | columns |
  |---|---|---|---|---|
  | `pg18` | `180000` / `18.0` | 7 | `{2,5}` | `source, waldir_size, wal,KiB, records, fpi, buffers_full, stats_age` |
  | `pg19` | `190000` / `19.0` | 8 | `{2,6}` | `source, waldir_size, wal,KiB, records, fpi, **fpi,KiB**, buffers_full, stats_age` |

  Suggested values (prev → curr), chosen so that every delta is unambiguous and the cross-version
  sentinel is identical:

  - `source` `WAL` (constant; `UniqueKey` defaults to 0, so the single row pairs across ticks)
  - `waldir_size` `1040 MB` → `1088 MB` — a pretty string at column 1, outside `DiffIntvl` in every branch
  - `wal,KiB` `2048.50` → `3072.75` → delta `1024.25`
  - `records` `1000` → `1500` → delta **`500`**, the shared sentinel
  - `fpi` `300` → `420` → delta `120`
  - `fpi,KiB` (pg19 only) `600.00` → `840.25` → delta `240.25`
  - `buffers_full` `12` → `19` → delta `7`
  - `stats_age` `01:00:00` → `02:00:00`, last column, outside `DiffIntvl`

- The three goldens land in `report/testdata/` as raw bytes including SGR escapes
  (`\033[37;1m…\033[0m` around each header cell) — never hand-edit them, always regenerate.

**Dependencies:**

- **Task 5** (`depends_on`) — `newApp` does `view.New()[config.ReportType]` (`report/report.go:83-85`),
  so without the `archiver` registration the report runs against a zero-value view. Transitively this
  also needs Task 1 (the selector `Configure` calls) and Task 2 (the PG 19 wal branch): both landed in
  Wave 1, both are prerequisites of Task 5.
- **Task 7** runs in this same wave and owns `report/report.go`, `report/describe.go` and
  `report/report_test.go`. This task creates only new files — no shared file, no conflict. Do not add
  cases to `Test_app_doReport` and do not touch the `update` flag declaration.
- No new Go packages, no `go.mod` change, no change to the test image.

**Edge cases:**

- **Two ticks yield one row, always** (Decision 12). If a subcase needs two printed rows it needs three
  ticks. Do not "fix" the discard.
- **Alignment is computed once, from the first printed sample** (`formatStatSample` returns early when
  `view.Aligned`), and values longer than the resulting width are truncated with a trailing `~`
  (`report/report.go:617-631`). This is why the never-archived case is its own recording rather than a
  second row appended to the populated one: a NULL-first recording would size the columns from blank
  cells and then truncate the 24-character WAL names of the later row. Keep the two recordings separate.
- **Why there is no fourth golden.** Code-research §10.F.3 suggested `report_record_archiver_null.golden`;
  the tech-spec's Task 8 fixes three goldens and this task holds to that. Nothing is lost: for the
  "cells are blank, not `0`/`-`/`n/a`" criterion, the field-count assertion is a *stronger* pin than a
  golden — a golden reddens for any reason at all and says nothing about which, while
  `len(strings.Fields(line)) == 5` reddens for exactly the reason the criterion is about. Record this in
  the decisions report as a deliberate, argued choice, not as an omission.
- **`TruncLimit: 32`** in `Config` — the widest cell here is a 24-character WAL name, comfortably under
  it. Do not lower it "to be safe": that would start truncating names and make the golden's meaning
  depend on the limit.
- **`TsStart`/`TsEnd` must bracket the filename dates** or `isFilenameTimestampOK` silently skips every
  entry and the test degenerates into the empty-archive case while looking like a real one. Use the
  bgwriter form: the same calendar day, `00:00:00` to `23:59:59`.
- **`isFilenameOK` requires exactly four dot-separated parts** and matches `s[0]` against the report type,
  `meta` or `sysinfo` (`report/report.go:460-476`). An entry named `archiver.20260519T100000.json`
  (three parts) is skipped without an error — another way to accidentally write a vacuous test. The
  `.000` millisecond field is what makes the count four.
- **The never-archived NULLs must be `sql.NullString{Valid: false}`, not `{String: "", Valid: true}`.**
  Both render as an empty cell today, but only the first is what the recorder actually writes for a SQL
  NULL, and the difference is exactly what the test claims to be about.
- **`diffPair` treats a value containing `.` or `e` as a float** and formats the result `%.2f`; integers
  divide by `itv`. With `itv == 1` the pg19 `fpi,KiB` delta is the literal `240.25`.
- **Do not assert on `INFO:` lines.** They are written by `printReportHeader` on the CLI path, not by
  `doReport`.

**Implementation hints:**

- Start by copying `report_record_bgwriter_test.go` wholesale into `report_record_wal_test.go` and
  reducing it — the wal test is the same table with different columns. Then write the archiver test,
  which is the same harness with a single version and different assertions; a small shared local helper
  inside each file is fine, but do not refactor the existing bgwriter or statio tests to extract a common
  harness. That is a different change, in files this task does not own.
- `statIOStripANSI` / `statIOAnsiRE` already exist at package scope in `report_record_statio_test.go` and
  are usable from the new files (same package). Reuse rather than declaring a second identical regexp.
- To isolate the data line for the field-count assertion: strip ANSI, split the output on `\n`, and take
  the line after the one containing `, rate: ` — the timestamp is printed on its own line
  (`report/report.go:607-613`), so the data row is never mixed with it.
- The `sysinfo` payload is a hand-written literal `[]byte(`{"ticks":100,"cpu_count":4}`)`; it is
  informational under Option B and nothing in these reports reads it, but leaving it out changes the tar
  shape a real recording has.
- The `meta` payload is the same bytes for every tick of a subcase — marshal once, write twice. A
  *changing* `version_num` between ticks would exercise the `versionChanged` path, which is a different
  screen's test and out of scope here.
- Name the tests `Test_app_doReport_Archiver` and `Test_app_doReport_WAL`, matching
  `Test_app_doReport_Bgwriter` / `Test_app_doReport_StatIOTime`. Subtest names appear in `-v` output and
  are how Step 4's mutation evidence is read.
- When a mutation does **not** turn a test red, that is a finding, not a nuisance: it means the golden
  does not cover what the acceptance criterion claims. Fix the test, then re-run the mutation.

## Reviewers

- **dev-code-reviewer** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-08-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-08-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-08-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [017-feat-wal-archiver-decisions.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
