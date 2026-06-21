---
status: planned                    # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # make test (bgwriter unit + integration tests pass)
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 01: bgwriter query layer + tests

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Create the query layer for the new `bgwriter` TUI screen that combines `pg_stat_bgwriter`
(all supported versions) and `pg_stat_checkpointer` (PG17+). This is a pure SQL/selector layer —
no view registration or TUI wiring (that is Task 3). The task delivers two new files:

- `internal/query/bgwriter.go` — three version-aware query constants (PG14-16, PG17, PG18) plus the
  selector `SelectStatBgwriterQuery(version int) (string, int, [2]int)`, structurally identical to
  the existing `internal/query/wal.go` (`SelectStatWALQuery`).
- `internal/query/bgwriter_test.go` — a table-driven unit test asserting the per-version
  `(Ncols, DiffIntvl)` tuples, plus an integration test that executes the query against live
  PG14-18 clusters, mirroring `internal/query/wal_test.go`.

The screen design relies on a single mechanism: `internal/stat/postgres.go:diff()` diffs only the
columns inside the contiguous `DiffIntvl [lo,hi]` range and copies every column outside it as-is.
This is exploited so that the checkpoint/restartpoint EVENT counters (`ckpt_timed`, `ckpt_req`,
`rstpt_*`) and `stats_age` render as absolute values, while the work/time/buffer columns render as
per-interval deltas. Because `DiffIntvl` is a single contiguous range, the column layout is fixed:
`source` (col 0) → absolute event-counter block → contiguous diffed block → `stats_age` last.

The PG18 branch adds a `slru_written` column to `pg_stat_checkpointer`; its exact presence MUST be
verified against a live PG18 cluster during the integration test (the code-research could only
confirm PG17 live; PG18 is from docs). Do not finalize the PG18 query from memory.

## What to do

1. Write the unit test `Test_SelectStatBgwriterQuery` first (table-driven over PG 14/15/16/17/18),
   asserting the returned `Ncols` and `DiffIntvl` per version. Run it, confirm it fails (selector
   does not exist yet).
2. Create `internal/query/bgwriter.go` with three `const` query strings:
   - **`PgStatBgwriterPG14`** (PG 14-16) — `FROM pg_stat_bgwriter`, 12 columns, `DiffIntvl [3,10]`.
   - **`PgStatBgwriterPG17`** (PG 17) — cross join `FROM pg_stat_bgwriter, pg_stat_checkpointer`,
     13 columns, `DiffIntvl [6,11]`.
   - **`PgStatBgwriterPG18`** (PG 18+) — as PG17 plus the diffed `slru_written` column, 14 columns,
     `DiffIntvl [6,12]`.
   Each query begins with `'Bgwriter' AS source` and ends with
   `date_trunc('seconds', now() - stats_reset)::text AS stats_age`. Use clear column aliases matching
   the Data Models layout (e.g. `checkpoint_write_time AS "ckpt_write,ms"`).
3. Implement `SelectStatBgwriterQuery(version int) (string, int, [2]int)` with the branch order:
   `>= 180000` → `(PgStatBgwriterPG18, 14, [2]int{6,12})`; `>= 170000` →
   `(PgStatBgwriterPG17, 13, [2]int{6,11})`; else → `(PgStatBgwriterPG14, 12, [2]int{3,10})`.
4. Run the unit test, confirm it passes.
5. Write the integration test `Test_StatBgwriterQueries` mirroring `Test_StatWALQueries`: loop
   `[]int{140000,150000,160000,170000,180000}`, `Format()` the template, `NewTestConnectVersion`,
   `t.Skipf` on unavailable version, `conn.Exec(q)`, assert no error, `conn.Close()`.
6. Run `make test`. On the live PG18 cluster, confirm the PG18 query (with `slru_written`) executes
   without error — this is the live verification of the PG18 column set. If `slru_written` does not
   exist on the live PG18 cluster, adjust the PG18 branch and its `Ncols`/`DiffIntvl` accordingly
   and record the deviation in the decisions log.

## TDD Anchor

Тесты, которые нужно написать ДО реализации. Пишем → запускаем → убеждаемся что падают → пишем код → убеждаемся что проходят.

- `internal/query/bgwriter_test.go::Test_SelectStatBgwriterQuery` — table-driven over
  PG 14/15/16/17/18; asserts the returned `(Ncols, DiffIntvl)` tuple per version:
  140000→`(12, [3,10])`, 150000→`(12, [3,10])`, 160000→`(12, [3,10])`, 170000→`(13, [6,11])`,
  180000→`(14, [6,12])`. Verifies the selector picks the correct const at the 170000 and 180000
  boundaries.
- `internal/query/bgwriter_test.go::Test_StatBgwriterQueries` — integration; loops PG14-18,
  `Format()`s the template, connects via `NewTestConnectVersion`, `t.Skipf` on unavailable version,
  executes the query, asserts no error. This is where the live PG18 `slru_written` column set is
  validated (real gate in CI where the full PG14-18 matrix runs).

## Acceptance Criteria

- [ ] `internal/query/bgwriter.go` exists with three query consts and `SelectStatBgwriterQuery`.
- [ ] `SelectStatBgwriterQuery` returns `(query, Ncols, DiffIntvl)` per version: PG14/15/16 →
      `12, [3,10]`; PG17 → `13, [6,11]`; PG18 → `14, [6,12]`.
- [ ] Branch boundaries use raw literals `170000`/`180000` (no `PostgresV17`/`PostgresV18` constants).
- [ ] Event counters (`ckpt_timed`, `ckpt_req`, `rstpt_*`) sit in a contiguous block right after
      `source`, OUTSIDE `DiffIntvl`; the diffed work/time/buffer block is contiguous; `stats_age` is
      the last column, outside `DiffIntvl`.
- [ ] PG17 query cross-joins `pg_stat_bgwriter` and `pg_stat_checkpointer`; PG18 query adds the
      diffed `slru_written` column.
- [ ] `stats_age` on PG17+ derives from `pg_stat_checkpointer.stats_reset` (Decision 4).
- [ ] All SQL is static `const` strings (no user interpolation), mirroring `wal.go`.
- [ ] `Test_SelectStatBgwriterQuery` (unit) is green.
- [ ] `Test_StatBgwriterQueries` (integration) is green on every available PG14-18 cluster, skipped
      only when a version is unavailable; PG18 `slru_written` set verified live.
- [ ] `make test` passes; no regressions in existing tests.

## Context Files

**Feature artifacts:**
- [004-feat-bgwriter-checkpointer.md](004-feat-bgwriter-checkpointer.md) — user-spec
- [004-feat-bgwriter-checkpointer-tech-spec.md](004-feat-bgwriter-checkpointer-tech-spec.md) —
  tech-spec (Task 1, Data Models with exact per-version layouts/Ncols/DiffIntvl, Decisions 1-5,
  Testing Strategy)
- [004-feat-bgwriter-checkpointer-code-research.md](004-feat-bgwriter-checkpointer-code-research.md) —
  §1 wal.go template, §6 version constants, §7 exact column inventory per PG version, §9 test patterns
- [004-feat-bgwriter-checkpointer-decisions.md](004-feat-bgwriter-checkpointer-decisions.md) —
  decisions log (created during execution)

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — project features and supported stats
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow,
  PG version handling
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — code patterns, testing conventions,
  version branching

**Code files:**
- [internal/query/bgwriter.go](internal/query/bgwriter.go) — NEW: query consts + selector
- [internal/query/bgwriter_test.go](internal/query/bgwriter_test.go) — NEW: unit + integration tests
- [internal/query/wal.go](internal/query/wal.go) — exact template for the new query file
- [internal/query/wal_test.go](internal/query/wal_test.go) — exact template for the new test file
- [internal/query/query.go](internal/query/query.go) — version constants (`PostgresV14`), `Format()`,
  `NewOptions()`
- [internal/postgres/testing.go](internal/postgres/testing.go) — `NewTestConnectVersion(version)`
  for integration tests (ports 21914-21918)
- [internal/stat/postgres.go](internal/stat/postgres.go) — `diff()` applies `DiffIntvl` (read for
  understanding; not modified)

## Verification Steps

- Run `make test` — both `Test_SelectStatBgwriterQuery` (unit) and `Test_StatBgwriterQueries`
  (integration) pass; integration cases skip only for unavailable PG versions.
- Confirm the live PG18 cluster executes the PG18 query (with `slru_written`) without error
  (the `slru_written` schema-divergence gate is real where PG18 is available).
- Run `make build` and `make lint` — compile clean, no new gosec/golangci-lint findings (static
  const SQL, no user interpolation — same style as `wal.go`).

## Details

**Files:**
- `internal/query/bgwriter.go` (NEW) — package `query`. Three `const` strings + selector. Template
  is `internal/query/wal.go` (two consts + `SelectStatWALQuery`). Column layouts (0-based) from the
  tech-spec Data Models:
  - PG14-16 (`Ncols=12`, `DiffIntvl=[3,10]`): `0 source`, `1 ckpt_timed`, `2 ckpt_req`,
    `3 ckpt_write,ms`, `4 ckpt_sync,ms`, `5 buf_ckpt`, `6 buf_clean`, `7 maxwritten`,
    `8 buf_backend`, `9 buf_backend_fsync`, `10 buf_alloc`, `11 stats_age`. Source columns:
    `checkpoints_timed`, `checkpoints_req`, `checkpoint_write_time`, `checkpoint_sync_time`,
    `buffers_checkpoint`, `buffers_clean`, `maxwritten_clean`, `buffers_backend`,
    `buffers_backend_fsync`, `buffers_alloc`, `stats_reset` — all from `pg_stat_bgwriter`.
  - PG17 (`Ncols=13`, `DiffIntvl=[6,11]`): `0 source`, `1 ckpt_timed`, `2 ckpt_req`,
    `3 rstpt_timed`, `4 rstpt_req`, `5 rstpt_done`, `6 ckpt_write,ms`, `7 ckpt_sync,ms`,
    `8 buf_ckpt`, `9 buf_clean`, `10 maxwritten`, `11 buf_alloc`, `12 stats_age`. From
    `pg_stat_checkpointer`: `num_timed`, `num_requested`, `restartpoints_timed`,
    `restartpoints_req`, `restartpoints_done`, `write_time`, `sync_time`, `buffers_written`,
    `stats_reset`. From `pg_stat_bgwriter`: `buffers_clean`, `maxwritten_clean`, `buffers_alloc`.
    Cross join: `FROM pg_stat_bgwriter, pg_stat_checkpointer` (single row × single row).
  - PG18 (`Ncols=14`, `DiffIntvl=[6,12]`): PG17 plus diffed `slru_written` inserted in the diffed
    block (grouped next to `buf_ckpt` for readability): `... 8 buf_ckpt, 9 slru_written,
    10 buf_clean, 11 maxwritten, 12 buf_alloc, 13 stats_age`.
- `internal/query/bgwriter_test.go` (NEW) — package `query`. Two tests mirroring `wal_test.go`:
  `Test_SelectStatBgwriterQuery` (table of `{version, wantNcols, wantDiffIntvl}`, query string
  ignored via `_`) and `Test_StatBgwriterQueries` (integration loop). Use
  `NewOptions(version, "f", "off", 256, "public")` for `Format()`, identical to `wal_test.go:40`.

**Dependencies:** none (no new packages). No dependency on other tasks (wave 1, `depends_on: []`).
Task 3 depends on this task's `SelectStatBgwriterQuery` selector and the `PgStatBgwriterPG14` const.

**Edge cases:**
- Version branch boundaries: exactly 170000 → PG17 branch; exactly 180000 → PG18 branch; below
  170000 → PG14-16 branch. The unit test covers 160000 (highest pre-17) and the 170000/180000
  boundaries.
- `stats_reset` source on PG17+: select from `pg_stat_checkpointer`, NOT `pg_stat_bgwriter`
  (Decision 4) — `pg_stat_bgwriter` on PG17+ retains its own `stats_reset`, so be explicit.
- `pg_stat_checkpointer` does not exist pre-17 → PG14-16 query must NOT reference it.
- A version not in the integration loop (or unavailable cluster) → `t.Skipf`, not a failure.

**Implementation hints:**
- Diff exclusion mechanism: `internal/stat/postgres.go:diff()` (line 330-333) copies any column
  with index `< interval[0]` or `> interval[1]` as-is; columns inside are subtracted. Keep the
  absolute event-counter block immediately after `source` and `stats_age` last so both fall outside
  the single contiguous `DiffIntvl`.
- `wal.go` is the exact structural template: leading `'<label>' AS source`, trailing
  `date_trunc('seconds', now() - stats_reset)::text AS stats_age`, string-concatenated `const`,
  selector returning `(string, int, [2]int)` with a `version >= 180000` / `>= 170000` ladder.
- Use raw numeric literals `170000` / `180000` in the selector — no `PostgresV17`/`PostgresV18`
  constants exist in `query.go` (only up to `PostgresV14`). Do not add new constants.
- Keep all SQL as plain static `const` strings — no `text/template` placeholders are needed for
  bgwriter (`Format()` will run but substitutes nothing), matching `wal.go` and avoiding gosec
  SQL-injection findings.
- `make test` runs serial (`-p 1`) with `-race` and a 300s timeout; integration tests hit shared
  local PG clusters on ports 21914-21918.

## Reviewers

- **dev-code-reviewer** → `004-feat-bgwriter-checkpointer-task-01-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `004-feat-bgwriter-checkpointer-task-01-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [004-feat-bgwriter-checkpointer-decisions.md](004-feat-bgwriter-checkpointer-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека (особенно если live PG18 `slru_written` column set отличается) — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось (например, если PG18 column set отличается от документированного)
