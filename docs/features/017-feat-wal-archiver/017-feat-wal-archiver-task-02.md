---
status: planned                    # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # gate = `go test -race -p 1 ./internal/query/...` INSIDE the CI image (PG 14-19 fixtures): PG 19 wal query returns 8 named columns; PG 14-18 counts unchanged
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 02: PG 19 FPI column on the wal screen

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

PostgreSQL 19 adds a `wal_fpi_bytes` column (type `numeric`) to `pg_stat_wal` — the volume of WAL
written as full page images, next to the already-exposed `wal_fpi` count. This was verified live
against a PG 19beta2 catalog (code-research §2.4): PG 18's `pg_stat_wal` has 5 columns, PG 19 has 6.

pgcenter's `wal` screen is version-aware: `SelectStatWALQuery(version)` returns the query template,
the column count and the diff interval, and `internal/view/view.go: Configure()` patches all three
into the registered view. Today the selector has two branches — PG 14–17 (11 columns) and PG 18+
(7 columns) — so on PG 19 the screen silently shows the PG 18 layout and the new column is invisible.

This task adds a third branch: a `PgStatWALPG19` constant that selects `round(wal_fpi_bytes / 1024, 2)
AS "fpi,KiB"` immediately after the `fpi` counter, so the number of full page images and the volume
they cost sit next to each other, and both are inside the diffed range so they render as per-interval
deltas. The resulting PG 19 layout is 8 columns with `DiffIntvl{2,6}`; `stats_age` moves to column 7
and must stay outside the interval (it is a text `date_trunc` value — a diff would abort the sample).

Scope boundaries, all verified rather than assumed:
- **PG 14–17 and PG 18 stay byte-identical.** Both existing constants and both existing return
  statements are untouched; the new branch goes in front of them.
- **`internal/view/view.go` needs no edit** — its `case "wal":` (view.go:389-391) already delegates to
  the selector, so the new branch is picked up for free in both the TUI and the report replay. This is
  also why this task cannot collide with the view-registration work in Wave 2.
- **`internal/view/view_test.go` belongs to Task 05.** Do not touch it here. Be precise about what
  that means: `TestViews_Configure` has **no** `wal` and no `archiver` assertions today
  (`grep wal internal/view/view_test.go` returns nothing), so the `Configure` hop is not covered by
  anything this task writes. Task 05 is the sole owner of that file in this feature and owns the
  view-level assertion that `case "wal":` hands a PG 19 `Options.Version` the 8-column layout. What
  *this* task pins is the selector's own return values (`internal/query/wal_test.go`). Do not assume
  the delegation is covered here.
- Golden replay coverage for the wal screen at PG 18/PG 19 is Task 08; the `fpi,KiB` describe row is
  Task 07. Neither belongs here.

Caveat to carry: `wal_fpi_bytes` was verified against **beta2**. If the catalog name moves at
beta3/RC, the blast radius is exactly the one query line changed here.

## What to do

1. Add a `PgStatWALPG19` constant to the existing `const (...)` block in `internal/query/wal.go`,
   after `PgStatWALDefault`. It is `PgStatWALDefault` plus one selected expression —
   `round(wal_fpi_bytes / 1024, 2)` aliased `"fpi,KiB"` — placed immediately after
   `wal_fpi AS fpi` and before `wal_buffers_full AS buffers_full`. Copy the surrounding lines
   verbatim, including the double space in `AS waldir_size  FROM`, so the rendered header does not
   change. Carry a comment naming the PG version that introduced the column and why the new column
   sits where it does (count and volume adjacent, inside the diffed range).

2. Add a third branch at the top of `SelectStatWALQuery` returning `PgStatWALPG19, 8, [2]int{2, 6}`
   for `version >= PostgresV19`, giving the same three-branch shape as `SelectStatBgwriterQuery`.
   Use the existing `query.PostgresV19` constant, not a numeric literal. Leave the two existing
   branches — their constants, counts, intervals and comments — exactly as they are.

3. Update `Test_SelectStatWALQuery` in `internal/query/wal_test.go`: the **existing** `190000` row
   changes to `{190000, 8, {2,6}}` — it is not duplicated or added alongside. Rows `140000`,
   `150000`, `170000`, `180000` stay untouched. Add a forward-version row (`200000`) expecting the
   PG 19 layout, so the branch condition is pinned as `>=` and not `==`.

4. Add the layout guard tests described in TDD Anchor: a no-Postgres test pinning the position of
   `fpi,KiB` in the PG 19 select list relative to its neighbours, and a no-Postgres test proving the
   PG 14 and PG 18 constants were not edited (in particular that neither mentions `wal_fpi_bytes`).

5. Strengthen the existing `Test_StatWALQueries` from an execution smoke test into a column-name
   assertion: query instead of exec, assert the live column count equals the `Ncols` the selector
   declared, and for PG 19 assert the full ordered header list plus the two header names at the
   `DiffIntvl` boundary (last diffed column is `buffers_full`, the column after the interval is
   `stats_age`). `internal/query/io_test.go: Test_StatIOQueries` is the precedent for reading
   `rows.FieldDescriptions()` from a live fixture; keep the existing `t.Skipf` on an unavailable
   version and keep the version list as it is.

6. Run the mutation checks listed in Acceptance Criteria — apply each mutation to production code,
   observe the named test go red, revert. Green tests that were never seen red do not count as proof.

## TDD Anchor

Tests are written first, run and observed failing, then the production code makes them pass.

- `internal/query/wal_test.go::Test_SelectStatWALQuery` — the edited `190000` row expects
  `Ncols=8`, `DiffIntvl={2,6}`; the new `200000` row expects the same; `140000`/`150000`/`170000`
  return `11`/`{2,9}` and `180000` returns `7`/`{2,5}` unchanged.
- `internal/query/wal_test.go::Test_SelectStatWALQuery_PG19ColumnOrder` (new, no Postgres needed) —
  in the query returned at `PostgresV19` the `"fpi,KiB"` alias appears after the `wal_fpi AS fpi`
  expression and before `wal_buffers_full`, and `stats_age` is the last selected column.
- `internal/query/wal_test.go::Test_SelectStatWALQuery_LegacyBranchesUntouched` (new, no Postgres
  needed) — the selector returns exactly `PgStatWALPG14` below 180000 and exactly `PgStatWALDefault`
  at 180000–189999, and neither constant contains `wal_fpi_bytes`.
- `internal/query/wal_test.go::Test_StatWALQueries` (existing, extended) — per version the live
  result's `FieldDescriptions()` length equals the selector's `Ncols`; at PG 19 the ordered header
  names are exactly `source, waldir_size, wal,KiB, records, fpi, fpi,KiB, buffers_full, stats_age`,
  the header at `DiffIntvl[1]` is `buffers_full` and the header at `DiffIntvl[1]+1` is `stats_age`.

## Acceptance Criteria

- [ ] `SelectStatWALQuery(190000)` returns `PgStatWALPG19`, `8`, `[2]int{2, 6}`; any version above
      190000 returns the same.
- [ ] The PG 19 query executes on the PG 19 fixture and returns exactly 8 columns, in the order
      `source, waldir_size, wal,KiB, records, fpi, fpi,KiB, buffers_full, stats_age`.
- [ ] `SelectStatWALQuery` at 140000–179999 and at 180000–189999 returns the same constants, counts
      and intervals as before this task; the diff of `internal/query/wal.go` shows only additions.
- [ ] `internal/view/view.go` is not modified by this task, and neither is `internal/view/view_test.go`.
- [ ] The new branch uses `PostgresV19`, not the literal `190000`.
- [ ] The existing `190000` row in `Test_SelectStatWALQuery` was changed in place — the test table
      contains exactly one row per version.
- [ ] **Mutation M1 must turn a test red:** returning `[2]int{2, 7}` from the PG 19 branch (stats_age
      pulled inside the diffed range) reddens `Test_SelectStatWALQuery` and the `DiffIntvl` boundary
      assertion in `Test_StatWALQueries`.
- [ ] **Mutation M2 must turn a test red:** moving the `"fpi,KiB"` expression in `PgStatWALPG19` to
      the end of the select list, after `stats_age` (the new column outside the diffed range),
      reddens `Test_SelectStatWALQuery_PG19ColumnOrder` and the ordered-header assertion in
      `Test_StatWALQueries`.
- [ ] **Mutation M3 must turn a test red:** changing `version >= PostgresV19` to
      `version == PostgresV19` reddens the `200000` row of `Test_SelectStatWALQuery`.
- [ ] **Mutation M4 must turn a test red:** adding `wal_fpi_bytes` to `PgStatWALDefault` instead of
      creating a new constant reddens `Test_SelectStatWALQuery_LegacyBranchesUntouched`.
- [ ] Each mutation above was actually applied, the failure observed, and the mutation reverted —
      recorded in the decisions-log entry for this task.
- [ ] `go test -race ./internal/query/...` is green in the CI image with PG 14–19 fixtures up;
      `gofmt` clean and `make lint` clean for the touched files.

## Context Files

**Feature artifacts:**
- [017-feat-wal-archiver.md](017-feat-wal-archiver.md) — user-spec
- [017-feat-wal-archiver-tech-spec.md](017-feat-wal-archiver-tech-spec.md) — tech-spec: Task 2, Data Models (PG 19 layout), Decision 14 (header names), Acceptance Criteria
- [017-feat-wal-archiver-decisions.md](017-feat-wal-archiver-decisions.md) — decisions log (write the report here at the end)
- [017-feat-wal-archiver-code-research.md](017-feat-wal-archiver-code-research.md) — §2.4 verified PG 19 catalog, §10.B exact insertion points and the executed PG 19 output, §10.G current test values, §8 the CI-image test command

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — project context (there is no `project.md` in this repo's PK)
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout, data flow, "PostgreSQL Version Handling", "Testing"
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — "Version-Specific Query Pattern", "Adding a New PostgreSQL Version", "Extract the decision out of the unreachable closure" (the mutation rule this task's ACs implement)

**Code files:**
- [internal/query/wal.go](../../../internal/query/wal.go) — modify: add `PgStatWALPG19`, add the third selector branch
- [internal/query/wal_test.go](../../../internal/query/wal_test.go) — modify: edit the `190000` row, add the forward row and the two layout guards, extend `Test_StatWALQueries` to name-driven assertions
- [internal/query/bgwriter.go](../../../internal/query/bgwriter.go) — read: the three-branch selector shape and the "counters outside DiffIntvl" convention
- [internal/query/query.go](../../../internal/query/query.go) — read: `PostgresV19 = 190000` (line 22) and the `Format`/`NewOptions` helpers the tests use
- [internal/query/io_test.go](../../../internal/query/io_test.go) — read: `Test_StatIOQueries` is the precedent for asserting live column names via `rows.FieldDescriptions()`

## Verification Steps

Host runs are the fast inner loop, not the gate: the selector tests need no PostgreSQL, so
`go test ./internal/query/...` runs on the host, but the only run that closes this task is the one
inside the CI image, where the PG 14–19 fixtures exist. Note also that there is no runnable package
at the repository root (no `.go` files there) — `go run .` fails; the binary is `./cmd`
(`make build` → `./bin/pgcenter`). Nothing in this task needs it.

- Write the tests first, run `go test ./internal/query/... -run 'Test_SelectStatWALQuery'` on the host
  (no Postgres needed for the selector tests) and confirm they fail for the expected reason.
- Implement, re-run the same command, confirm green.
- **The gate** — run the full query-package suite against the PG 14–19 fixtures in the CI image:
  ```bash
  docker run --rm -v "$PWD":/work -v "$HOME/go/pkg/mod":/gomod \
    -e GOROOT=/gomod/golang.org/toolchain@v0.0.1-go1.25.12.linux-amd64 \
    -e GOPATH=/gopath -e GOFLAGS=-mod=mod -e HOME=/root \
    -w /work lesovsky/pgcenter-testing:0.0.11 bash -c '
      export PATH=$GOROOT/bin:$PATH
      prepare-test-environment.sh > /tmp/prep.log 2>&1 || { tail -30 /tmp/prep.log; exit 1; }
      go test -race -p 1 -timeout 300s ./internal/query/...'
  ```
  Expected: green, with the PG 19 subtest of `Test_StatWALQueries` running (not skipped) and
  asserting 8 named columns. A skipped PG 19 subtest is not a pass — check the subtest output.
- Run the four mutations M1–M4 from Acceptance Criteria one at a time, each time re-running the
  affected test and observing red, then reverting. M1, M3, M4 and the order half of M2 are provable
  on the host without Postgres; the `Test_StatWALQueries` halves need the CI image.
- Confirm the diff touches only `internal/query/wal.go` and `internal/query/wal_test.go`
  (`git diff --name-only`), and that the wal.go diff contains no deletions inside the PG 14 / PG 18
  constants or their return statements.
- `gofmt -l internal/query` prints nothing; `$(go env GOPATH)/bin/golangci-lint run ./internal/query/...`
  is clean (golangci-lint is not on the default PATH).

## Details

<!-- All details for task execution — technical, organizational, any other. -->

**Files:**
- `internal/query/wal.go` (33 lines today) — holds two constants, `PgStatWALPG14` (lines 5-11, PG 14-17,
  11 columns) and `PgStatWALDefault` (lines 15-21, PG 18+, 7 columns), plus
  `SelectStatWALQuery(version int) (string, int, [2]int)` (lines 25-32) with two branches: `>= 180000`
  → `(PgStatWALDefault, 7, {2,5})` and the fallthrough → `(PgStatWALPG14, 11, {2,9})`. Add the third
  constant and the third branch; change nothing else.
- `internal/query/wal_test.go` (56 lines today) — `Test_SelectStatWALQuery` (lines 10-31) is a
  five-row table over `(version, wantNcols, wantDiffIntvl)`; the `190000` row is line 21 and currently
  reads `{version: 190000, wantNcols: 7, wantDiffIntvl: [2]int{2, 5}}`. `Test_StatWALQueries`
  (lines 34-56) formats the template through `query.Format` with
  `NewOptions(version, "f", "off", 256, "public")`, connects with
  `postgres.NewTestConnectVersion(version)`, `t.Skipf`s when the version is unavailable, and calls
  `conn.Exec(q)`. The version list at line 35 already includes `190000` — leave it alone.

**Dependencies:** none. This task is independent of the other Wave 1 tasks (they touch
`internal/query/archiver.go`, `internal/query/overview.go` and `cmd/report/`), and nothing here waits
on them. No new Go packages.

**Target layout (0-based), which the whole task hangs on:**

| version | Ncols | DiffIntvl | columns |
|---|---|---|---|
| 14–17 | 11 | `{2,9}` | source, waldir_size, wal,KiB, records, fpi, write, sync, write,ms, sync,ms, buffers_full, stats_age |
| 18 | 7 | `{2,5}` | source, waldir_size, wal,KiB, records, fpi, buffers_full, stats_age |
| **19+** | **8** | **`{2,6}`** | source, waldir_size, wal,KiB, records, fpi, **fpi,KiB**, buffers_full, stats_age |

**Edge cases:**
- `stats_age` inside the diff interval is the classic failure of this screen: it is a `date_trunc`
  text value, `diffPair` → `strconv.ParseInt` fails on it and the error aborts the whole sample, not
  just the cell. The existing PG 18 comment says so; keep saying it in the PG 19 comment.
- `wal_fpi_bytes` is `numeric` and never NULL in `pg_stat_wal` (a single always-present row), so no
  `coalesce(...,0)` is needed here — unlike the LEFT JOIN screens. Do not add one; it would be
  unrequested code.
- Version boundary: the branch must fire for every version at or above `PostgresV19`, including
  future majors. The `200000` test row exists for exactly this.
- `Test_StatWALQueries` skips when a fixture is unavailable, and a skip is green. When you claim the
  PG 19 assertion passed, quote the subtest line showing it ran.
- The PG 19 fixture in the image is **beta2**. If `wal_fpi_bytes` is absent at runtime the live test
  fails with an undefined-column error — that is the correct signal, not something to guard against
  in the query.

**Implementation hints:**
- Model the new constant on `PgStatWALDefault` line by line; the only difference is the inserted
  `round(wal_fpi_bytes / 1024, 2) AS "fpi,KiB"` line, which mirrors the existing
  `round(wal_bytes / 1024, 2) AS "wal,KiB"` line. Aliases containing a comma must be double-quoted,
  which is why those two lines use Go backtick strings while their neighbours use plain quotes.
- The double space in `AS waldir_size  FROM pg_ls_waldir()` is deliberate in both existing constants;
  copy it as-is.
- `SelectStatBgwriterQuery` (`internal/query/bgwriter.go:41-52`) is the reference three-branch
  selector, and its PG 18 constant shows the same shape of change: one column inserted into the
  diffed block, `Ncols` and the interval upper bound each +1.
- The `180000` literal in the existing branch stays a literal. Normalising it to `PostgresV18` is a
  cosmetic change to code this task does not own — leave it.
- Ordered-header assertions read best as a `[]string` compared with `assert.Equal` against the names
  extracted from `rows.FieldDescriptions()`; comparing the whole slice in one assert gives a readable
  diff when a column moves.
- The two no-Postgres guard tests can operate on the query string returned by the selector (substring
  positions / `strings.Contains`), so they run everywhere including a bare `go test ./internal/query/...`
  on the host.

## Reviewers

- **dev-code-reviewer** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-02-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-02-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-02-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [017-feat-wal-archiver-decisions.md](017-feat-wal-archiver-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов) — включая результат прогона мутаций M1–M4
- [ ] Если отклонились от спека — описать отклонение и причину (в частности, если имя `wal_fpi_bytes` не подтвердилось на текущей PG 19 в образе)
- [ ] Обновить user-spec/tech-spec если что-то изменилось
