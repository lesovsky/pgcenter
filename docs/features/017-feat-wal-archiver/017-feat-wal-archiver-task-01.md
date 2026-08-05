---
status: planned                    # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — `go test ./internal/query/...` in the CI image: 9 columns on PG 14-19, succeeds under a pg_monitor-only role, fails with a permission error without it
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 01: Archiver query and selector

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Create `internal/query/archiver.go` — the single 9-column `pg_stat_archiver` + `.ready`-backlog query
that the whole `archiver` feature reads from, plus its version-independent selector
`SelectStatArchiverQuery(_ int) (string, int, [2]int)`. This is the data source for everything that
follows: Task 5 registers the view against this constant, Task 7 documents its columns, Task 8 replays
it in a golden. **No view wiring, no `top/`, no `report/` here** — this task creates exactly two new
files and touches nothing that exists.

Two properties make this a real task rather than a copy of `bgwriter.go`:

**1. The selector takes no version branch, and that is a verified fact, not a convenience.**
`pg_stat_archiver` is byte-identical on PG 14 through PG 19 (checked against live `pg_attribute` on
14.23 / 17.10 / 18.4 / 19beta2 — code-research §2.1), and `pg_ls_archive_statusdir()` exists on every
one of them. So the selector follows the `SelectStatIOTimeQuery` form (`internal/query/io.go:99`):
the `version` parameter is kept for symmetry with every other `Select*Query` and named `_`, which is
what revive requires for an unused parameter (`patterns.md` → Naming Conventions). Do not invent a
branch, and do not drop the parameter.

**2. The privilege behaviour the whole design rests on must be proven by test, in both directions.**
`pg_ls_archive_statusdir()` is superuser + `pg_monitor` (ACL `{postgres=X/postgres,pg_monitor=X/postgres}`,
verified on live PG 14/18/19). Decision 4 accepts that a role without `pg_monitor` loses the whole
screen — the `wal` screen already behaves this way with `pg_ls_waldir()`, and hiding the call behind
`has_function_privilege()` was **measured** not to work (PostgreSQL checks EXECUTE at function-node
initialisation, so `CASE` with an uncorrelated subquery, `CASE` with a correlated subquery and
`LEFT JOIN LATERAL … ON has_function_privilege(...)` all fail alike). That acceptance is only honest
if it is tested: the fixture connection is the **superuser** `postgres`
(`internal/postgres/testing.go:42`), which is precisely why the wrong `pg_ls_dir` privilege assumption
in ADR [010] survived unnoticed for a whole release. So the tests create their own roles at runtime,
idempotently, `SET ROLE` into them and `RESET ROLE` afterwards (Decision 18) — the test image stays
frozen.

Also load-bearing, and cheap to get wrong: **SQL NULLs must stay NULL.** Nothing on this screen is
diffed (`DiffIntvl{0,0}`), so `calculateDelta` short-circuits before `diff()`
(`internal/stat/postgres.go:589-597`) and the `strconv.ParseInt("")` trap that forces `coalesce(...,0)`
elsewhere does not apply here. A blank cell is the honest rendering of "this cluster has never
archived" (Decision 3, precedent ADR [013] `backend_xid`). No `coalesce` anywhere in this query.

## What to do

- Create `internal/query/archiver.go` with one exported constant `PgStatArchiverDefault` holding the
  9-column query in the order the user-spec locks — `source, ready, archived, last_archived,
  archived_age, failed, last_failed, failed_age, stats_age` — and one exported selector
  `SelectStatArchiverQuery(_ int) (string, int, [2]int)` returning that constant, `9` and
  `[2]int{0, 0}`.
- Follow `internal/query/wal.go` / `internal/query/bgwriter.go` for file shape: a `const (…)` block
  with a doc comment above the constant, the selector below it with its own doc comment.
- The constant's doc comment must state, in this order: that the view is schema-stable on PG 14–19 so
  there is no version branch; that the `.ready` count comes from `pg_ls_archive_statusdir()`, same
  superuser/`pg_monitor` privilege class as the `pg_ls_waldir()` the `wal` screen already calls
  unconditionally, so the screen is all-or-nothing (Decision 4); that nothing is diffed
  (`DiffIntvl{0,0}`), which is what keeps the four NULL-able columns safe without `coalesce`
  (Decision 3); and **why the two server-supplied WAL-name columns need no escape sanitisation**
  (Decision 16 — PostgreSQL only records names that passed its own `VALID_XFN_CHARS` filter, a set
  containing no ESC and no control characters; tech-debt [029] is neither widened nor closed here).
- Create `internal/query/archiver_test.go` with the five tests listed in TDD Anchor, written **before**
  the production file and confirmed red first.
- Privilege tests create their own roles at test time (Decision 18): creation must be idempotent
  (safe on a re-run and on all six clusters), the `pg_monitor` role must hold `pg_monitor` and nothing
  else, the second role must hold neither superuser nor `pg_monitor`, and every test must `RESET ROLE`
  even when an assertion fails — a leaked `SET ROLE` would silently change what later assertions on
  the same connection see.
- The negative privilege test must assert the *specific* failure — SQLSTATE `42501` and a message
  naming `pg_ls_archive_statusdir` — not merely "an error occurred". An assertion on "some error"
  would pass on a syntax typo and prove nothing.
- Do not touch `internal/view/`, `top/`, `record/`, `report/` or any existing file. Task 5 owns the
  view registration; Task 3 owns `internal/query/overview.go`.

## TDD Anchor

<!-- Fill if task includes writing code. For non-code tasks (user instructions, deploy, config) — delete this section. -->

Тесты, которые нужно написать ДО реализации. Пишем → запускаем → убеждаемся что падают → пишем код → убеждаемся что проходят.

All five live in `internal/query/archiver_test.go`. Tests 1 and 3 run without PostgreSQL; 2, 4 and 5
connect to the fixtures and `t.Skipf` when the cluster is unavailable, exactly as
`Test_StatWALQueries` (`internal/query/wal_test.go:34-56`) does.

- `internal/query/archiver_test.go::Test_SelectStatArchiverQuery` — table over
  `{140000, 150000, 160000, 170000, 180000, 190000}`: every version returns the **same** query string
  (`PgStatArchiverDefault`), `Ncols == 9` and `DiffIntvl == [2]int{0, 0}`. A version-independent
  selector still gets the full matrix — the `io.go` precedent (`Test_SelectStatIOTimeQuery`).
- `internal/query/archiver_test.go::Test_StatArchiverQueries` — per-version live run over PG 14–19:
  `Format(tmpl, NewOptions(version, "f", "off", 256, "public"))` succeeds and leaves no `{{`, the
  query executes, `rows.FieldDescriptions()` has length 9, its names are exactly
  `source, ready, archived, last_archived, archived_age, failed, last_failed, failed_age, stats_age`
  **in that order**, and the result carries exactly one row.
- `internal/query/archiver_test.go::Test_StatArchiverQuery_NullsStayNull` — on a fixture cluster
  (which has never archived): scanning into `sql.NullString` receivers, `last_archived`,
  `archived_age`, `last_failed` and `failed_age` come back `Valid == false`, while `source`, `ready`,
  `archived`, `failed` and `stats_age` come back `Valid == true`; and the query text contains no
  `coalesce` at all.
- `internal/query/archiver_test.go::Test_StatArchiverQuery_PgMonitorRoleSucceeds` — creates a
  `pg_monitor`-only role idempotently, `SET ROLE`s into it, runs the query, asserts 9 field
  descriptions and one row, and `RESET ROLE`s in a `defer` so the assertion path cannot leak the role.
- `internal/query/archiver_test.go::Test_StatArchiverQuery_WithoutPgMonitorFails` — creates a role
  holding neither superuser nor `pg_monitor` idempotently, `SET ROLE`s into it, runs the query, and
  asserts the error is a `*pgconn.PgError` with `Code == "42501"` whose message names
  `pg_ls_archive_statusdir`; `RESET ROLE` in a `defer`.

## Acceptance Criteria

Written as mutations, per `patterns.md` → "Extract the decision out of the unreachable closure": a
test that guards an invariant is believed only after the named mutation has been applied to the
production code and the suite observed **red**. Apply each one, see red, revert.

- [ ] `internal/query/archiver.go` and `internal/query/archiver_test.go` exist; no other file in the
      tree is modified.
- [ ] `SelectStatArchiverQuery` has signature `func SelectStatArchiverQuery(_ int) (string, int, [2]int)` —
      unused parameter named `_`, per the project's revive settings and the `io.go:99` precedent.
- [ ] The query returns 9 columns on every live cluster PG 14–19, in the locked order.
- [ ] Mutation — change the selector's returned `9` to `8`: `Test_SelectStatArchiverQuery` turns red.
- [ ] Mutation — change the selector's returned `[2]int{0, 0}` to `[2]int{2, 5}`:
      `Test_SelectStatArchiverQuery` turns red.
- [ ] Mutation — delete the `ready` sub-select column from `PgStatArchiverDefault`:
      `Test_StatArchiverQueries` turns red on the column count **and** on the column-name list.
- [ ] Mutation — swap the `ready` and `archived` columns in the SQL: `Test_StatArchiverQueries` turns
      red on the column-name order (the count alone must not be what catches it).
- [ ] Mutation — wrap `last_archived_wal` in `coalesce(last_archived_wal, '-')`:
      `Test_StatArchiverQuery_NullsStayNull` turns red.
- [ ] Mutation — replace `pg_ls_archive_statusdir()` with `pg_ls_dir('pg_wal/archive_status')`:
      `Test_StatArchiverQuery_PgMonitorRoleSucceeds` turns red with a permission error. This is the
      mutation that proves the positive privilege test is exercising privileges and not the superuser
      fixture connection.
- [ ] Mutation — replace the `ready` sub-select with the literal `0` (no privileged call at all):
      `Test_StatArchiverQuery_WithoutPgMonitorFails` turns red. This proves the negative test is
      pinned to the privileged call rather than to any error.
- [ ] Both privilege tests `RESET ROLE` even when they fail: after a deliberately broken assertion the
      subsequent tests in the package still pass.
- [ ] The privilege tests are re-runnable: `go test ./internal/query/...` twice in a row is green
      (role creation is idempotent).
- [ ] The query contains no `coalesce` and the NULL-able columns render as SQL NULL.
- [ ] The constant's doc comment states the no-version-branch fact, the all-or-nothing privilege
      behaviour (Decision 4), the no-`coalesce` rationale (Decision 3) and the
      no-escape-sanitisation rationale (Decision 16).
- [ ] `go test ./internal/query/...` is green inside the CI image with PG 14–19 fixtures up.
- [ ] `make lint` (golangci-lint + gosec) is clean on the host — in particular revive raises nothing
      about the unused selector parameter.

## Context Files

**Feature artifacts:**
- [017-feat-wal-archiver.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver.md) — user-spec
  (locked column order, the `archive_mode=off` edge case, the two privilege acceptance criteria)
- [017-feat-wal-archiver-tech-spec.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver-tech-spec.md) —
  tech-spec: Task 1, the Data Models table (exact 9-column layout with aliases), Decisions 2, 3, 4,
  10, 14, 16, 18
- [017-feat-wal-archiver-decisions.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver-decisions.md) —
  decisions log (created on completion of the first task)
- [017-feat-wal-archiver-code-research.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver-code-research.md) —
  §2.1–2.3 the verified catalog + the candidate query executed on PG 14/17/18/19, §5.3 the selector
  test style, §7.2–7.3 NULLs and privileges, §8 the CI-image docker command, §10.C.1 the
  implementation-level detail

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — what pgcenter is, supported statistics
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow,
  PG version handling
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — "Version-Specific Query Pattern"
  (selector returning `(string, int, [2]int)`), "Extract the decision out of the unreachable closure"
  (the mutation rule for invariant-guarding tests), "Naming Conventions" (`_` for unused parameters),
  "Linting", "Adding a New PostgreSQL Version"

**Code files:**
- [internal/query/archiver.go](internal/query/archiver.go) — **NEW**, create: constant + selector
- [internal/postgres/testing.go](internal/postgres/testing.go) — **MODIFY**: add the single shared
  role-creation / `SET ROLE` test helper that both this task's privilege tests and Task 03's use
  (tech-spec Decision 19). It lives here, not in `internal/query`, because both `internal/query` and
  `internal/stat` need it and this file is already the shared home for test helpers. Do not duplicate
  it per package.
- [internal/query/archiver_test.go](internal/query/archiver_test.go) — **NEW**, create: the five tests
- [internal/query/io.go](internal/query/io.go) — `SelectStatIOTimeQuery(_ int)` at `:99` with its
  rationale comment at `:94-98`: the exact form to copy for a version-independent selector
- [internal/query/wal.go](internal/query/wal.go) — the single-row screen shape: `'WAL' AS source`
  literal at column 0, `stats_age` last and outside the interval, the unconditional `pg_ls_waldir()`
  call whose privilege class this task inherits
- [internal/query/wal_test.go](internal/query/wal_test.go) — the canonical two-test pair (table-driven
  selector test + per-version live execution with `t.Skipf`)
- [internal/query/io_test.go](internal/query/io_test.go) — `rows.FieldDescriptions()` length assertion
  against the selector's declared `Ncols` (`:135`)
- [internal/query/bgwriter_test.go](internal/query/bgwriter_test.go) — same live-column-count shape
  for a single-row screen (`:59`)
- [internal/query/overview_test.go](internal/query/overview_test.go) — `Test_ArchivingBacklogQuery_Degrades`
  (`:123-146`): the existing test over the same directory listing, and the comment at `:124-125` that
  states the privilege requirement wrongly — do **not** edit it, Task 3 owns that file
- [internal/postgres/testing.go](internal/postgres/testing.go) — `NewTestConnectVersion`, the
  port map PG14=21914 … PG19=21919, and the fact that the fixture user is `postgres` (superuser)
- [internal/postgres/postgres.go](internal/postgres/postgres.go) — `Exec` / `Query` / `QueryRow` on
  `*DB` (`:119-133`), the only API the tests need

## Verification Steps

<!-- How to verify task is complete. For code — run tests. For deploy — check logs. For user-action — user confirmation. -->

- Step 1 — red first: write the five tests, run them, confirm every one fails before
  `internal/query/archiver.go` exists.
- Step 2 — full package run inside the CI image (PG 14–19 fixtures do not exist on the host):

  ```bash
  docker run --rm -v "$PWD":/work -v "$HOME/go/pkg/mod":/gomod \
    -e GOROOT=/gomod/golang.org/toolchain@v0.0.1-go1.25.12.linux-amd64 \
    -e GOPATH=/gopath -e GOFLAGS=-mod=mod -e HOME=/root \
    -w /work lesovsky/pgcenter-testing:0.0.11 bash -c '
      export PATH=$GOROOT/bin:$PATH
      prepare-test-environment.sh > /tmp/prep.log 2>&1 || { tail -30 /tmp/prep.log; exit 1; }
      go test -race -p 1 -timeout 300s ./internal/query/...'
  ```

  Expected: green, and the per-version subtests for PG 14–19 **ran** rather than skipped — a silent
  `t.Skipf` on every version would make the whole live half of this task vacuous (tech-debt [019] is
  exactly this failure mode). Check the subtest names in `-v` output before believing the green.
- Step 3 — idempotency: run the same command a second time without recreating the container's
  clusters is not possible (the image starts fresh), so re-run `go test -run Archiver ./internal/query/...`
  twice **inside one** container session and confirm both runs are green.
- Step 4 — mutation gates: apply each mutation listed in Acceptance Criteria one at a time inside the
  container, confirm the named test turns **red**, revert. Green-only evidence does not close this
  task.
- Step 5 — `make lint` and `make vuln` on the host: clean, with no revive complaint about the unused
  selector parameter.

## Details

<!-- All details for task execution — technical, organizational, any other. -->

**Files:**

- `internal/query/archiver.go` — **does not exist**, create. Package `query`. One `const (…)` block
  with `PgStatArchiverDefault`, then `SelectStatArchiverQuery`. The 9 columns, aliases fixed by
  Decision 14 and the Data Models table:

  | # | alias | source |
  |---|---|---|
  | 0 | `source` | literal `'Archiver'` — row identity, stable across samples |
  | 1 | `ready` | `count(*) FILTER (WHERE name LIKE '%.ready')` over `pg_ls_archive_statusdir()` |
  | 2 | `archived` | `archived_count` |
  | 3 | `last_archived` | `last_archived_wal` (NULL → blank) |
  | 4 | `archived_age` | `date_trunc('seconds', now() - last_archived_time)::text` (NULL → blank) |
  | 5 | `failed` | `failed_count` |
  | 6 | `last_failed` | `last_failed_wal` (NULL → blank) |
  | 7 | `failed_age` | `date_trunc('seconds', now() - last_failed_time)::text` (NULL → blank) |
  | 8 | `stats_age` | `date_trunc('seconds', now() - stats_reset)::text` (never NULL) |

  The `ready` column is a scalar sub-select over `pg_ls_archive_statusdir()`; `FROM pg_stat_archiver`
  is the outer relation. This exact SQL was executed on live PG 14.23 / 17.10 / 18.4 / 19beta2 and
  returns the shape above (code-research §2.3) — it does not need re-deriving, only transcribing into
  the project's string-concatenation style.

- `internal/query/archiver_test.go` — **does not exist**, create. Same imports as
  `internal/query/wal_test.go` plus `database/sql` (NullString receivers) and
  `github.com/jackc/pgx/v5/pgconn` (`*pgconn.PgError` for the SQLSTATE assertion).

**Dependencies:**

- No task dependencies — wave 1, two brand-new files, no shared file with Task 2 (`wal.go`), Task 3
  (`overview.go`) or Task 4 (`cmd/report/`).
- No new Go packages. `github.com/jackc/pgx/v5/pgconn` is a package of the pgx/v5 module the project
  already depends on, and `*pgconn.PgError` is already used in `internal/postgres/postgres.go:77`,
  `top/stat.go:864` and `top/stat_test.go:259` — nothing to add to `go.mod`.
- Downstream: Task 5 consumes `query.PgStatArchiverDefault` and `query.SelectStatArchiverQuery` by
  name — do not rename either after this task lands without telling Task 5.

**Edge cases:**

- **`%` in the SQL literal.** `'%.ready'` is safe: `query.Format` is `text/template`
  (`internal/query/query.go:91-104`) and only reacts to `{{`/`}}`, and every `printCmdline` call in
  the tree passes an explicit `"%s"` verb. The identical literal already lives in
  `OverviewArchivingBacklog`. Still run the query through `Format` in the live test — that is what
  catches a stray `{{` typo at test time rather than at pgcenter start-up.
- **Fixtures never archived.** `archive_mode=off`, no `archive_command`
  (`testing/prepare-test-environment.sh:17-33`, verified live). So `archived_count`/`failed_count` are
  `0` (bigint counters are never NULL), the four name/time columns are NULL, `stats_reset` is NOT
  NULL, and `pg_ls_archive_statusdir()` returns zero rows → `count(*) FILTER (…)` is `0`, not NULL.
  The tests can prove shape and NULL handling; they **cannot** prove archiving behaviour — that is the
  stand run's job, and this task must not pretend otherwise.
- **`SET ROLE` requires the target role to exist on that cluster**, and the roles are cluster-global —
  each of the six fixture clusters needs its own creation. Creation must tolerate "already exists"
  (the container may run the package twice, and six clusters share the test code path). `GRANT`
  is naturally idempotent; bare `CREATE ROLE` is not.
- **A `NOLOGIN` role is fine** — `SET ROLE` does not need `LOGIN`, and the superuser fixture connection
  may `SET ROLE` to any role without an explicit membership grant.
- **The negative role must really hold nothing.** Give it no `GRANT` at all; do not reuse the positive
  role with a `REVOKE`, which would make the two tests order-dependent.
- **A leaked `SET ROLE` poisons later tests on the same connection.** `RESET ROLE` belongs in a
  `defer` placed immediately after the successful `SET ROLE`, not at the end of the happy path.
  Opening a dedicated connection per privilege test and closing it is the cheap belt-and-braces on
  top; `RESET ROLE` is still required by Decision 18.
- **Unavailable cluster.** `postgres.NewTestConnectVersion` returns an error for an unmapped or
  down cluster — `t.Skipf` with the same message shape the neighbouring tests use. Do not let a skip
  masquerade as a pass in the verification step (see Verification Step 2).

**Implementation hints:**

- Byte-for-byte reference for the file shape and comment density: `internal/query/io.go` (comment
  above each constant explaining the column layout 0-based and why each guard exists) and
  `internal/query/wal.go` (the compact single-row form).
- The selector doc comment should say what `SelectStatIOTimeQuery`'s says: the version parameter is
  unused because the view is schema-stable on every supported version, is named `_` per revive, and is
  kept for signature symmetry with the other selectors.
- `DiffIntvl{0,0}` is not a placeholder value — it is the statement "nothing is diffed", and
  `calculateDelta` (`internal/stat/postgres.go:589-597`) short-circuits on it and never enters
  `diff()`. That is what makes the `'Archiver'` literal at column 0 and the four NULL columns safe.
  A reviewer who reads `{0,0}` as "unset" should find the answer in the comment.
- Column names in the live test come from `rows.FieldDescriptions()[i].Name` — assert the list, not
  just the length, so a future column insertion cannot silently shift an index (tech-spec AC
  "Column-name-driven assertions").
- Naming the test roles: pick something unmistakably owned by this suite (e.g. a `pgcenter_test_`
  prefix) so a role left behind in a long-lived cluster is attributable.
- Idempotent creation is a `DO $$ … $$` block checking `pg_roles` before `CREATE ROLE`; run it through
  `conn.Exec`.
- This task does **not** add the view, so nothing renders yet. Verification is entirely
  `go test ./internal/query/...` — resist the urge to wire `view.go` "just to see it", that is Task 5
  and would create a wave conflict.

## Reviewers

- **dev-code-reviewer** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-01-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-01-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-01-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [017-feat-wal-archiver-decisions.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
