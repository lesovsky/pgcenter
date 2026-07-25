---
status: planned
depends_on: ["03"]
wave: 4
skills: [code-writing]
verify: "bash — make test"
reviewers: [dev-code-reviewer, dev-test-reviewer]
teammate_name:
---

# Task 04: Thread PG 19 through the rest of the test suite

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Tasks 1–3 gave the project a PG 19 cluster, a version constant, a port mapping and three version-aware
progress selectors. None of that is actually exercised until the test suite is told PG 19 exists: the
per-version lists that drive every live-connection test still stop at `180000`, so today the whole suite
would stay green against a PG 19 cluster it never connects to. This task closes that gap for every
`_test.go` file the earlier tasks do not own.

The edits look mechanical but fall into **three groups that are not interchangeable**, and treating them as
one kind is the main way this task goes wrong:

1. **Plain live-connection version loops** (27 sites in 20 files). A `[]int{…}` fed to
   `postgres.NewTestConnectVersion` inside or around a `t.Run`. Append `190000` to the tail, nothing else.
   Purely additive.

2. **Per-version assertion tables** (8 tables). Pure unit tables with no PostgreSQL involved, asserting
   `(query constant, Ncols, DiffIntvl)` per version. A new row needs *expected values*, and those values
   must be **derived from the selector code** — six of the eight selectors branch on version and their
   newest branch is `>= PostgresV18` or lower, so a `190000` row proves this feature did not accidentally
   move a boundary; the other two ignore the version entirely (see Group 3). Getting a value wrong here
   converts a guard test into a rubber stamp.

3. **The recording filter test** (`record/record_test.go: Test_filterViews`) — one row, and the only place
   in this task where copying a neighbouring row produces a **wrong but plausible-looking** number. This
   test runs *without* a database, so a bad value fails hard in CI rather than skipping. Its expectation
   must be computed from the actual per-view `MinRequiredVersion` gates in `view.New()`: at PG 19 strictly
   more views pass the gate than at PG 14, so the PG 14 row's numbers are not the PG 19 row's numbers.

Two further constraints shape the diff:

- **Three lists must not be touched at all** — the string-inspection loops in `io_test.go` that never open a
  connection. They are enumerated below; each element there stands for one *query branch*, so `190000` would
  land on the same `>= PostgresV18` branch as `180000` and duplicate an assertion rather than add one. This
  exclusion covers string loops only: a single-version list that *does* open a connection
  (`databases_test.go:56`) is in Group 1, not here.
- **Edit points and loops are not one-to-one.** Two package-level lists feed several tests each
  (`overview_test.go`'s `overviewVersions` drives four tests; `common_test.go`'s list drives four subtests,
  one of them through a `versions[3:]` slice), so a single edit changes several tests — and, for the slice,
  the append must stay at the tail so the slice bound keeps meaning "PG 10+".

**Ownership boundary:** `internal/view/view_test.go` and the three `internal/query/progress_{vacuum,analyze,
basebackup}_test.go` files are **Task 3's**, not this task's. Their PG 19 rows prove Task 3's `Configure`
wiring and selectors, so they land with the code they prove. Do not edit them here even though they contain
version lists of exactly the shape this task appends to.

## What to do

### Group 1 — append `190000` to plain live-connection loops (27 sites, 20 files)

Append the value at the **tail** of the existing list; change nothing else in these tests.

| # | file:line (as researched) | current tail | note |
|---|---|---|---|
| 1 | `internal/query/activity_test.go:29` | …170000, 180000 | full 90500→ list |
| 2 | `internal/query/bgwriter_test.go:36` | …180000 | asserts `Len(FieldDescriptions()) == wantNcols` — the shape that catches a catalog rename |
| 3 | `internal/query/common_test.go:64` | …180000 | drives 4 subtests, one via `versions[3:]` at :99 — appending at the tail keeps the slice bound valid |
| 4 | `internal/query/databases_test.go:34` | …180000 | the *general* loop |
| 5 | `internal/query/databases_test.go:56` | `[]int{140000}` | the **sessions** loop — `for _, version := range []int{140000}` around a `t.Run` that formats `PgStatDatabaseSessionsDefault`, opens a connection and runs `conn.Exec`. This is the **only** place in the repo that query is ever executed against a server, so without `190000` here `pg_stat_database`'s sessions shape gets zero PG 19 coverage in a PG-19-compatibility feature. Append `190000` → `[]int{140000, 190000}` |
| 6 | `internal/query/functions_test.go:11` | …180000 | |
| 7 | `internal/query/indexes_test.go:11` | …180000 | |
| 8 | `internal/query/io_test.go:112` | `{160000,170000,180000}` | the PG 19 run also re-exercises the `version >= PostgresV18` `object='wal'` assertion at :138 — correct, it stays true; do not gate it off |
| 9 | `internal/query/io_test.go:150` | `{160000,170000,180000}` | |
| 10 | `internal/query/overview_test.go:13` | `overviewVersions` | package-level var, feeds 4 tests (`:18,:68,:128,:155`) — one edit, four call sites |
| 11 | `internal/query/pgcenter_schema_test.go:21` | …180000 | the `plperlu` fixture gate; first thing to fail if PG 19's `plperlu` misbehaves |
| 12 | `internal/query/procpidstat_test.go:81` | …180000 | |
| 13 | `internal/query/progress_cluster_test.go:11` | `{120000…180000}` | append only — no PG 19 selector for this screen (Decision 6) |
| 14 | `internal/query/progress_copy_test.go:11` | …180000 | append only |
| 15 | `internal/query/progress_create_index_test.go:11` | `{120000…180000}` | append only |
| 16-18 | `internal/query/replication_slots_test.go:34,113,161` | …180000 | three separate lists in one file |
| 19 | `internal/query/replication_test.go:39` | …180000 | |
| 20 | `internal/query/sizes_test.go:11` | …180000 | |
| 21 | `internal/query/statements_test.go:59` | full | |
| 22 | `internal/query/statements_test.go:110` | inline `{130000…180000}` | WAL section, PG13+ |
| 23 | `internal/query/statements_test.go:129` | inline `{150000…180000}` | JIT section, PG15+ |
| 24 | `internal/query/statements_test.go:176` | full | |
| 25 | `internal/query/tables_test.go:11` | …180000 | |
| 26 | `internal/query/wal_test.go:34` | …180000 | |
| 27 | `internal/stat/postgres_test.go:90` | …180000 | `Test_collectOverviewStat`; its skip sits above the loop — see Decision 7 below |

Line numbers are from the code-research pass; re-locate by content, not by line number, in case an earlier
wave shifted them.

### Group 2 — three sites that must NOT be touched

All three are in `io_test.go`, all three are **string inspection with no connection**, and in all three the
list has one element **per query branch**, not per supported version. `190000` resolves to the same
`>= PostgresV18` branch as `180000`, so adding it re-asserts an already-asserted string.

| file:line | list | why |
|---|---|---|
| `internal/query/io_test.go:68` | `{160000,180000}` | `Test_SelectStatIOQuery_NullSafety` — one element per branch (`<V18` / `>=V18`) → `190000` duplicates the `>=V18` assertion |
| `internal/query/io_test.go:96` | `{160000,180000}` | `Test_SelectStatIOTimeQuery_NullSafety` — same |
| `internal/query/io_test.go:179` | `{160000,180000}` | `Test_SelectStatIOQuery_NoTemplateArtifacts` — same |

The rule that decides membership here is **"does the loop body open a connection?"**, not "is the list short"
and not "does the selector branch on version". A one-element list that calls `postgres.NewTestConnectVersion`
belongs in Group 1 (site #5).

### Group 3 — add a `190000` row to eight per-version assertion tables

These run without PostgreSQL. For each table: **open the selector it calls, evaluate it at `190000`, and
write the row from what the code returns.** Do not carry the `180000` row down — a copied row asserts that
the test file agrees with itself, which is true no matter what the selector does. Deriving it is what makes
the row detect a moved boundary.

The eight tables and the selector each one calls:

| table | selector to derive from | where |
|---|---|---|
| `internal/query/bgwriter_test.go:16-22` | `SelectStatBgwriterQuery` | `internal/query/bgwriter.go:41` |
| `internal/query/wal_test.go:16-20` | `SelectStatWALQuery` | `internal/query/wal.go:25` |
| `internal/query/replication_slots_test.go:16-20` | `SelectStatReplicationSlotsQuery` | `internal/query/replication_slots.go:39` — **takes `_ int`** |
| `internal/query/io_test.go:18-24` | `SelectStatIOQuery` | `internal/query/io.go:87` |
| `internal/query/io_test.go:42-46` | `SelectStatIOTimeQuery` | `internal/query/io.go:99` — **takes `_ int`** |
| `internal/query/statements_test.go:15-25` | `SelectStatStatementsTimingQuery` | `internal/query/statements.go:333` |
| `internal/query/statements_test.go:42-46` | `SelectStatStatementsJITQuery` | `internal/query/statements.go:347` |
| `internal/query/statements_test.go:154-164` | `SelectQueryReportQuery` | `internal/query/statements.go:355` |

**Two of these selectors have no branch to read.** `SelectStatReplicationSlotsQuery` and
`SelectStatIOTimeQuery` are declared with an **ignored version parameter** (`_ int`) and return one constant
triple unconditionally — do not go looking for a version condition in them, there is none. Their `190000`
rows therefore pin nothing about version handling; they exist only so the table stays uniform with its
siblings and so a future version branch added to either selector has to update this table. Say exactly that
in the row comment (not "PG 19 keeps the PG 18 answer", which would imply a branch that does not exist).

For the other six, read the condition and note which branch `190000` lands in — that is the fact the row
records. Once every row is written, cross-check it against the researched values in **Details →
"Group 3 cross-check"** before running anything; a mismatch means either the derivation or the research is
wrong, and it must be resolved rather than papered over.

Follow each table's existing comment convention: where a row marks a boundary the neighbouring rows explain
(`// PG 18: …`), a short comment on the new row is in keeping with the file. Comment the new row only, not
every row.

**Out of scope for Group 3:** `internal/query/databases_test.go:17-22` and `internal/query/activity_test.go:16-18`.
Both tables deliberately stop at their last branch boundary (130000 / 100000) and never listed 170000 or
180000 either; the tech-spec fixes the count at eight tables. Leave their convention intact.

### Group 4 — the derived row in `record/record_test.go: Test_filterViews`

Add one row for `190000` with `pgssSchema: "public"`, and **derive `wantN` / `wantV` yourself**:

1. Read `filterViews` (`record/record.go:200-233`) — a view is dropped by `NotRecordable`, by
   `!v.VersionOK(version)` (`internal/view/view.go:421`), or (for `statements_*` keys) by an empty
   `pgssSchema`.
2. Read `view.New()` (`internal/view/view.go`) — count the registered views and read every
   `MinRequiredVersion`.
3. Compute how many of them a version of `190000` with a non-empty pgss schema drops, and how many remain.

The highest existing row is `{140000, "public", wantN: 3, wantV: 24}` (`record/record_test.go:133`); at PG 14 three views are still
dropped by their PG15/PG16 gates. Copying that row is the specific mistake this task exists to avoid — at
PG 19 those three views pass. The code-research document's §B.5 table cell for this row contains the copied
(wrong) values while its surrounding prose states the correct reasoning; **trust the derivation, not that
cell**. A correct derivation from today's registry yields no dropped views and every registered view kept.

Keep the row's placement consistent with the table (existing rows run high→low from `140000`) and add a
one-line comment saying why the PG 19 row's numbers differ from the PG 14 row's — otherwise the next reader
will "fix" it back.

### Explicitly not in this task

- `internal/view/view_test.go` — Task 3 (`TestView_VersionOK`, `TestViews_Configure` PG 19 rows).
- `internal/query/progress_{vacuum,analyze,basebackup}_test.go` — Task 3 (they also switch from the bare
  constant to the selector; that switch is meaningless without Task 3's selectors).
- **Do not restructure the nine tests whose `t.Skipf` fires on the parent test** (Decision 7):
  `common_test.go` ×4 subtests, `overview_test.go` ×4 tests, `internal/stat/postgres_test.go`. One
  unavailable version there skips every remaining version too — a pre-existing coverage hole, already
  recorded as a deferred item. Append the version; do not add `t.Run` wrappers, do not move the skip.
- No production (non-`_test.go`) file is touched by this task.

## TDD Anchor

This task *is* the test work, so the usual red→green ordering inverts: the artifact under test is the suite
itself, and the anchor is evidence that each new version row actually executes (rather than silently
skipping) and that no expectation was copied instead of derived. Establish that evidence in this order.

**Write first, before any list append — the one genuinely new assertion:**

- `record/record_test.go::Test_filterViews` (the `190000` row) — derived from `view.New()`'s
  `MinRequiredVersion` gates plus the pgss gate. Runs with **no PostgreSQL**. Sanity-check the derivation by
  temporarily substituting the PG 14 row's `wantN`/`wantV`: the test must **fail** with those values. If it
  passes with both the copied and the derived numbers, the derivation is not being exercised and something
  is wrong.

**Then the pure unit rows (no PostgreSQL needed, must be green immediately — a red one means the value was
guessed, not derived):**

- `internal/query/bgwriter_test.go::Test_SelectStatBgwriterQuery/version/190000`
- `internal/query/wal_test.go::Test_SelectStatWALQuery/version/190000`
- `internal/query/replication_slots_test.go::Test_SelectStatReplicationSlotsQuery/version/190000`
- `internal/query/io_test.go::Test_SelectStatIOQuery/version/190000`
- `internal/query/io_test.go::Test_SelectStatIOTimeQuery/version/190000`
- `internal/query/statements_test.go::TestSelectStatStatementsTimingQuery` (190000 case)
- `internal/query/statements_test.go::TestSelectStatStatementsJITQuery` (190000 case)
- `internal/query/statements_test.go::TestSelectQueryReportQuery` (190000 case)

**Then the live-connection rows — verified by observing them run, not by a green summary line:**

- `internal/query/bgwriter_test.go::Test_StatBgwriterQueries/pg_stat_bgwriter/190000` — with the Task 1
  cluster up, `go test -v -run Test_StatBgwriterQueries ./internal/query/` must show this subtest as
  **PASS**, not SKIP; its `assert.Len(rows.FieldDescriptions(), wantNcols)` is what would catch a beta→GA
  catalog rename.
- `internal/query/io_test.go::Test_StatIOQueries/pg_stat_io/190000` — PASS with the cluster up, and its
  `version >= PostgresV18` `object='wal'` assertion still holds on PG 19.
- `internal/query/databases_test.go::Test_SelectStatDatabaseGeneralQuery/pg_stat_database/sessions/190000` —
  PASS with the cluster up. This subtest did not exist before this task and is the **only** PG 19 execution
  of `PgStatDatabaseSessionsDefault` anywhere in the suite; if it reports SKIP or is absent from `-v` output,
  that query has no PG 19 coverage at all and the task is not done.
- `internal/query/pgcenter_schema_test.go` PG 19 subtest — PASS; this is the `plperlu` canary. A failure
  here is an environment/compatibility finding to route per the tech-spec Risks table, **not** a reason to
  drop `190000` from the list.
- With the cluster **down**, every one of the above must report SKIP and the packages must stay green — the
  self-skip contract in `patterns.md` "Adding a New PostgreSQL Version".

## Acceptance Criteria

- [ ] All 27 Group 1 live-connection lists carry `190000` at the tail; no other change in those loops.
- [ ] `databases_test.go:56` — the single-version **sessions** loop — carries `190000`, so
      `PgStatDatabaseSessionsDefault` is executed against PG 19 at least once.
- [ ] The three Group 2 sites are byte-identical to `develop` (`io_test.go:68/96/179`) — they are the only
      excluded sites, and each is a string-inspection loop that opens no connection.
- [ ] All eight Group 3 assertion tables have a `190000` row derived from the selector code: six record which
      branch `190000` lands in, and the two version-independent ones (`SelectStatReplicationSlotsQuery`,
      `SelectStatIOTimeQuery`) say in their comment that the selector ignores the version rather than
      claiming a branch.
- [ ] `record/record_test.go: Test_filterViews` has a `190000` row whose `wantN`/`wantV` were computed from
      `filterViews` + `view.New()`, not copied from the `140000` row, with a comment recording why they
      differ.
- [ ] `internal/view/view_test.go` and the three `progress_{vacuum,analyze,basebackup}_test.go` files are
      untouched by this task's diff.
- [ ] `databases_test.go:17-22` and `activity_test.go:16-18` are untouched (eight tables, per tech-spec).
- [ ] The nine tests named in Decision 7 gained only the appended version — no `t.Run` wrapper, no moved
      skip, no restructuring.
- [ ] With the PG 19 cluster running, the PG 19 subtests execute (observed with `-v`) rather than skipping.
- [ ] With no cluster running, the live-PG packages skip cleanly and `record`, `report`, `top` and
      `internal/view` stay green.
- [ ] `make test` green; `make lint` clean.
- [ ] Only `_test.go` files changed — no production file in the diff.

## Context Files

**Feature artifacts:**
- [012-feat-pg19-compatibility-baseline.md](012-feat-pg19-compatibility-baseline.md) — user-spec
- [012-feat-pg19-compatibility-baseline-tech-spec.md](012-feat-pg19-compatibility-baseline-tech-spec.md) —
  tech-spec; this task is "Task 4" in Implementation Tasks. **Decision 7** (leave the nine tests alone) and
  the "Integration tests" bullets in Testing Strategy are binding here
- [012-feat-pg19-compatibility-baseline-decisions.md](012-feat-pg19-compatibility-baseline-decisions.md) —
  decisions log (append the task report here)
- [012-feat-pg19-compatibility-baseline-code-research.md](012-feat-pg19-compatibility-baseline-code-research.md)
  — **§B is the per-file source of truth for locating sites** (B.1 append list, B.2 do-not-touch, B.3
  assertion tables, B.5 view/record rows). §A.3 explains the two skip shapes. §4.1's older table conflated
  the groups and is superseded by §B. **Two known errors in §B, both corrected here and in the tech-spec:**
  (a) §B.2 (and its line 259 summary) lists `databases_test.go:56` as do-not-touch on the grounds that it
  "re-runs an identical query" — but that loop opens a connection and `Exec`s the sessions query, and is its
  only execution site, so it belongs in Group 1 (site #5); §B.2 is a 3-site list, not 4. (b) §B.5's table
  cell for the `Test_filterViews` PG 19 row carries copied PG 14 values that contradict its own prose —
  see Group 4

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — what pgcenter is (this project has
  no `project.md`; `overview.md` is its equivalent)
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout and how PG
  versions are handled
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — "Adding a New PostgreSQL Version"
  (step 2 is literally this task), "Version-Specific Query Pattern", and **"Adding a New View — test counts
  that must be updated"**, which warns that `Test_filterViews` runs without Postgres so a stale count is a
  real failure — a red `record` package is not automatically "just the connection tests"
- [deployment.md](../../../.claude/skills/project-knowledge/deployment.md) — the `pgcenter-testing` image
  and cluster ports the tests connect to

**Code files (modify — all `_test.go`):**
- [internal/query/activity_test.go](../../../internal/query/activity_test.go) — Group 1 (:29)
- [internal/query/bgwriter_test.go](../../../internal/query/bgwriter_test.go) — Group 1 (:36) + Group 3 (:22)
- [internal/query/common_test.go](../../../internal/query/common_test.go) — Group 1 (:64, tail append keeps `versions[3:]` valid)
- [internal/query/databases_test.go](../../../internal/query/databases_test.go) — Group 1 twice: the general
  loop (:34) and the sessions loop (:56), the sole execution site of `PgStatDatabaseSessionsDefault`
- [internal/query/functions_test.go](../../../internal/query/functions_test.go) — Group 1 (:11)
- [internal/query/indexes_test.go](../../../internal/query/indexes_test.go) — Group 1 (:11)
- [internal/query/io_test.go](../../../internal/query/io_test.go) — Group 1 (:112, :150) + Group 3 (:18-24, :42-46); :68/:96/:179 are Group 2 (the only excluded sites in the task)
- [internal/query/overview_test.go](../../../internal/query/overview_test.go) — Group 1 (`overviewVersions`, :13) — one edit, four tests
- [internal/query/pgcenter_schema_test.go](../../../internal/query/pgcenter_schema_test.go) — Group 1 (:21), `plperlu` canary
- [internal/query/procpidstat_test.go](../../../internal/query/procpidstat_test.go) — Group 1 (:81)
- [internal/query/progress_cluster_test.go](../../../internal/query/progress_cluster_test.go) — Group 1 (:11)
- [internal/query/progress_copy_test.go](../../../internal/query/progress_copy_test.go) — Group 1 (:11)
- [internal/query/progress_create_index_test.go](../../../internal/query/progress_create_index_test.go) — Group 1 (:11)
- [internal/query/replication_slots_test.go](../../../internal/query/replication_slots_test.go) — Group 1 (:34, :113, :161) + Group 3 (:20)
- [internal/query/replication_test.go](../../../internal/query/replication_test.go) — Group 1 (:39)
- [internal/query/sizes_test.go](../../../internal/query/sizes_test.go) — Group 1 (:11)
- [internal/query/statements_test.go](../../../internal/query/statements_test.go) — Group 1 (:59, :110, :129, :176) + Group 3 (:25, :46, :164)
- [internal/query/tables_test.go](../../../internal/query/tables_test.go) — Group 1 (:11)
- [internal/query/wal_test.go](../../../internal/query/wal_test.go) — Group 1 (:34) + Group 3 (:20)
- [internal/stat/postgres_test.go](../../../internal/stat/postgres_test.go) — Group 1 (:90, `Test_collectOverviewStat`)
- [record/record_test.go](../../../record/record_test.go) — Group 4 (`Test_filterViews`, :109-145), the derived row

**Code files (read — do not modify):**
- [record/record.go](../../../record/record.go) — `filterViews` (:200-233): the three drop reasons the
  derivation must account for
- [internal/view/view.go](../../../internal/view/view.go) — `New()` registry and every `MinRequiredVersion`;
  `VersionOK` (:421). The other input to the derivation
- [internal/query/io.go](../../../internal/query/io.go),
  [internal/query/bgwriter.go](../../../internal/query/bgwriter.go),
  [internal/query/wal.go](../../../internal/query/wal.go),
  [internal/query/replication_slots.go](../../../internal/query/replication_slots.go),
  [internal/query/statements.go](../../../internal/query/statements.go) — the selectors whose branches the
  Group 3 expected values are derived from
- [internal/postgres/testing.go](../../../internal/postgres/testing.go) — `NewTestConnectVersion` and the
  port map Task 2 hardened; the reason an unmapped version now errors instead of silently using PG 14

## Verification Steps

1. **Derivation first.** Before touching a list: compute the `Test_filterViews` PG 19 expectation from
   `filterViews` + `view.New()` and write it down (it goes in the decisions report). Substitute the PG 14
   row's values temporarily and confirm `go test -run Test_filterViews ./record/` **fails** — evidence the
   row is load-bearing. Restore the derived values; it must pass.
2. **Unit rows, no database:** `go test -run 'TestSelect|Test_Select' ./internal/query/` — all eight new
   table rows green on the first run. A failure here means a value was guessed.
3. **Live rows, cluster up:** with the Task 1 image running,
   `go test -v -run 'Test_Stat|Test_Common|Test_Overview|Test_collectOverviewStat|Test_SelectStatDatabaseGeneralQuery|Test_QueryPgcenterSchema|Test_ArchivingBacklogQuery_Degrades' ./internal/query/ ./internal/stat/`
   — the last two matter: the schema test is the plperlu canary this task's own anchor calls for, and the
   archiving-backlog degradation test is otherwise missed by the pattern. Simply running the whole package
   verbosely is also acceptable; the point is that neither is silently skipped from the filter.
   and read the output: each `…/190000` subtest reports `--- PASS`, not `--- SKIP` — including
   `pg_stat_database/sessions/190000`, which is the site most likely to be silently missing because it is
   the one that was previously (wrongly) listed as do-not-touch. A green package summary alone is not
   evidence — a skipped subtest also reports green.
4. **Live rows, cluster down:** stop the clusters and run `go test ./internal/query/... ./internal/stat/...`
   — packages green, PG 19 subtests SKIP. Then `go test ./record/... ./report/... ./top/... ./internal/view/...`
   — these run without PostgreSQL and must be green regardless; a failure here is a real stale count, not a
   missing fixture.
5. **Boundary check:** `git diff --name-only` lists exactly the 21 files in this task and nothing else — in
   particular no `internal/view/view_test.go`, no `progress_{vacuum,analyze,basebackup}_test.go`, no
   production file.
6. **Do-not-touch check:** `git diff internal/query/io_test.go` shows no change on the three Group 2 lines
   (:68, :96, :179) — while :112, :150 and both assertion tables in the same file *are* changed.
7. **Coverage arithmetic:** the diff should add `190000` in 27 Group 1 lists + 8 Group 3 rows + 1
   `Test_filterViews` row = 36 new occurrences. A different count means a site was missed or one of the
   three excluded sites was edited — reconcile before proceeding. `databases_test.go` alone accounts for two
   of the 27 (the general loop at :34 and the sessions loop at :56).
8. `make test` — the gate for this task. Then `make lint`.

## Details

**Files:**
- 19 files in `internal/query/*_test.go` + `internal/stat/postgres_test.go` — append `190000` to the
  live-connection lists per Group 1 (27 lists across these 20 files: `databases_test.go` holds two,
  `io_test.go` two, `replication_slots_test.go` three, `statements_test.go` four, the rest one each).
  **Five** of these files additionally gain Group 3 assertion-table rows: `bgwriter_test.go`, `wal_test.go`,
  `replication_slots_test.go`, `io_test.go` (two tables) and `statements_test.go` (three tables) — 5 files,
  8 tables.
- `record/record_test.go` — one derived row in `Test_filterViews`. This is the only file outside
  `internal/` in the task and the only edit that needs arithmetic.
- Nothing else. No production code, no new test file. Total: 21 files, 36 new `190000` occurrences.

**Group 3 cross-check (use only AFTER deriving each row from the selector):**

These are the values a correct derivation produces. They are here, not in "What to do", on purpose: read
them *after* you have written the row from the selector, as confirmation. If your derived row differs from
the line below, stop and find out which one is wrong — do not silently adopt either.

| table | expected row |
|---|---|
| `bgwriter_test.go:16-22` | `{version: 190000, wantNcols: 14, wantDiffIntvl: [2]int{6, 12}}` — `>= 180000` branch |
| `wal_test.go:16-20` | `{version: 190000, wantNcols: 7, wantDiffIntvl: [2]int{2, 5}}` — `>= 180000` branch |
| `replication_slots_test.go:16-20` | `{version: 190000, wantNcols: 15, wantDiffIntvl: [2]int{6, 13}}` — no branch, selector ignores version |
| `io_test.go:18-24` | `{version: 190000, wantNcols: 16, wantDiffIntvl: [2]int{4, 14}}` — `>= PostgresV18` branch |
| `io_test.go:42-46` | `{version: 190000, wantNcols: 10, wantDiffIntvl: [2]int{4, 8}}` — no branch, selector ignores version |
| `statements_test.go:15-25` | `{version: 190000, want: PgStatStatementsTimingDefault}` — `>= 170000` case |
| `statements_test.go:42-46` | `{version: 190000, wantQuery: PgStatStatementsJITDefault, wantNcols: 15, wantDiff: [2]int{7, 12}, wantKey: 13}` — `>= PostgresV17` branch |
| `statements_test.go:154-164` | `{version: 190000, want: PgStatStatementsReportQueryDefault}` — `>= 170000` case |

**Dependencies:**
- Task 3 (wave 3) — file-ownership boundary: it owns `internal/view/view_test.go` and the three progress
  test files, and it introduces the selectors those tests call. Running this task before Task 3 lands would
  make `make test` mix this task's failures with Task 3's unfinished work.
- Task 2 (wave 2) — `PostgresV19` and the port map entry; without them `NewTestConnectVersion(190000)`
  returns the "no test cluster port mapping" error Task 2 added, so every PG 19 subtest would skip rather
  than run.
- Task 1 (wave 1) — the locally built image with the running PG 19 cluster. Without it the PG 19 subtests
  skip and this task cannot be verified beyond compilation.
- No new Go packages.

**Edge cases:**
- `common_test.go:99` slices `versions[3:]` to mean "PG 10 and newer". Appending at the tail is safe;
  inserting anywhere else silently changes which versions that subtest covers.
- `overview_test.go`'s `overviewVersions` is a package-level var read by four tests — one edit, four
  behaviour changes. Do not inline it or duplicate it per test.
- `io_test.go:112`'s loop body has a `version >= PostgresV18` branch asserting `object='wal'` rows exist.
  PG 19 takes that branch and the assertion remains true — leave the gate as is; do not narrow it to `== 18`.
- `pgcenter_schema_test.go` depends on the `plperlu` fixture. If PG 19 fails there, it is an image /
  compatibility finding for the Risks table — report it, do not remove `190000` to make the suite green.
- `Test_collectOverviewStat` and the nine Decision 7 tests will report SKIP as a whole when the PG 19
  cluster is missing, *after* having asserted PG 14–18. That is expected and pre-existing; do not "fix" it.
- `common_test.go`'s list starts at `90500`, which never runs in CI, so its subtests already dead-skip on
  their first iteration today. Appending `190000` neither helps nor hurts; it is recorded as a deferred item.
- The suite runs `-p 1` (`Makefile`) because packages share live clusters — do not parallelise anything.

**Implementation hints:**
- Work group by group, not file by file: do all of Group 1, run the suite, then Group 3, then Group 4. Each
  group has a different failure signature, and mixing them makes a red run ambiguous.
- For Group 3, open the selector next to the test before writing the row. Six of the eight branch on version
  and their newest branch is `>= PostgresV18` or lower, which is *why* the PG 19 row repeats the PG 18
  values — state that in the row comment so the next reader knows it is intentional, not a copy-paste. The
  remaining two (`SelectStatReplicationSlotsQuery`, `SelectStatIOTimeQuery`) are declared `(_ int)` and have
  no branch at all; their comment should say the selector is version-independent, so the row is a
  regression net for a future branch rather than a boundary assertion.
- For Group 4, the two inputs are `filterViews`'s drop conditions and every `MinRequiredVersion` in
  `view.New()`. Write the count out in the decisions report so a reviewer can check the arithmetic rather
  than re-deriving it.
- `Test_filterViews` calls `filterViews(tc.version, tc.pgssSchema, view.New())` with a fresh registry per
  row, and `filterViews` deletes from the map it is given — rows do not interfere; the new row can go
  anywhere in the table, but keep the existing high→low ordering.
- Existing per-version tables carry short `// PG NN: …` comments explaining each boundary. Match that
  convention; do not add a comment to every row, only to the new one.

## Reviewers

- **dev-code-reviewer** → `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline-task-04-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline-task-04-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в
      [012-feat-pg19-compatibility-baseline-decisions.md](012-feat-pg19-compatibility-baseline-decisions.md)
      (Summary: 1-3 предложения + ревью со ссылками на JSON; отдельно зафиксировать **выведенные** значения
      `wantN`/`wantV` для строки 190000 в `Test_filterViews` и как они получены)
- [ ] Если отклонились от спека — описать отклонение и причину (в частности, если какой-то PG 19 сабтест
      пришлось пропустить или список не удалось дополнить — назвать сайт и причину)
- [ ] Обновить user-spec/tech-spec если что-то изменилось
