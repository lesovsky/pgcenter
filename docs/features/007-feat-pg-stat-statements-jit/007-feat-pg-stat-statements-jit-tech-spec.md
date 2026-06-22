---
created: 2026-06-22
status: approved
branch: feature/pg-stat-statements-jit
size: S
---

# Tech Spec: pg_stat_statements JIT screen

## Solution

Add a 7th `pg_stat_statements` sub-screen (`statements_jit`) showing per-statement JIT
compilation cost, modeled on the existing `statements_timings` sub-screen. The screen is built
from three layers, all reusing established pgcenter patterns:

1. **Query layer** (`internal/query/statements.go`) — two version-specific query consts
   (PG15/16 base, PG17+ with deform columns) and a `SelectStatStatementsJITQuery(version)`
   selector returning query + `Ncols` + `DiffIntvl` + `UniqueKey` (the `SelectStatIOQuery`
   model, because the JIT column *count* — not just names — changes across versions). The
   SELECT follows the canonical pgss row layout (`user, database, <*_total cumulative>,
   <*_ms + functions interval>, queryid, query`) and filters to rows with JIT activity via
   `WHERE jit_functions > 0`.
2. **View registration** (`internal/view/view.go`) — a `statements_jit` view entry gated by
   `MinRequiredVersion: query.PostgresV15`, marked `NotRecordable: true`, sorted by `gen_total`
   desc, with a `Msg` that doubles as the empty-screen hint; plus a `Configure()` case that
   patches `QueryTmpl/Ncols/DiffIntvl/UniqueKey` from the selector per detected version.
3. **TUI wiring** (`top/menu.go`, `top/config_view.go`) — a 7th `menuPgss` item + `menuSelect`
   case, and insertion of `statements_jit` into the `x`-cycle (`… wal → jit → timings …`).

No record/report wiring (TUI-first principle, `NotRecordable`). No keybinding changes (`X`/`x`
already route to the pgss menu/cycle). PG<15 degrades gracefully via the existing runtime
`VersionOK` guard.

## Architecture

### What we're building/modifying

- **`internal/query/statements.go`** — add `PgStatStatementsJITPG15` and
  `PgStatStatementsJITDefault` consts + `SelectStatStatementsJITQuery(version) (string, int,
  [2]int, int)` selector. Purpose: produce the version-correct JIT query and its layout
  metadata.
- **`internal/view/view.go`** — add the `statements_jit` view entry and a `Configure()` case.
  Purpose: register the screen, gate it to PG15+, mark it non-recordable, and bind the
  version-selected query/layout at startup.
- **`top/menu.go`** — add the menu item + select case. Purpose: make JIT reachable from the
  `X` menu.
- **`top/config_view.go`** — extend `statementsNextView`. Purpose: include JIT in the `x`
  cycle.
- **Test files** — `internal/query/statements_test.go` (selector + exec coverage),
  `internal/view/view_test.go` (count bump + optional view guard), `record/record_test.go`
  (filtered-count bump).

### How it works

1. At startup `view.New()` registers `statements_jit` in the view map (regardless of PG
   version). `view.Views.Configure(opts)` runs the `statements_jit` case, calls
   `SelectStatStatementsJITQuery(opts.Version)`, and patches the view's `QueryTmpl`, `Ncols`,
   `DiffIntvl`, `UniqueKey`; then `query.Format()` resolves `{{.PGSSSchema}}` /
   `{{.PgSSQueryLenFn}}`.
2. The DBA presses `X` → selects "pg_stat_statements JIT" (or cycles with `x`).
   `viewSwitchHandler` swaps the active view; `printCmdline` shows the view `Msg`.
3. The collector runs `view.VersionOK(version)` first — on PG<15 it returns "selected
   statistics is not supported by current version of Postgres" and never queries (same as
   `statements_wal` on PG<13). On PG15+ it runs `view.Query`, diffs the `DiffIntvl` columns,
   matches rows across samples on the `UniqueKey` (md5 queryid), and sorts by `OrderKey` (2,
   `gen_total`, desc) — the duration-aware branch in `internal/stat/postgres.go::sort` (via
   `parseDuration`, handling `HH:MM:SS` with 3+ digit hours and `N days HH:MM:SS`) orders the
   `*_total` text durations numerically, the same path the timings-screen totals rely on. This
   is an undocumented invariant: `OrderKey` pointing at a `*_total` text column sorts correctly
   only because of that branch.
4. Rows are pre-filtered in SQL by `WHERE jit_functions > 0`, so under `jit=off`/low activity
   the screen is empty; the `Msg` text explains why.

## Decisions

### Decision 1: Selector returns layout metadata (query + Ncols + DiffIntvl + UniqueKey)
**Decision:** `SelectStatStatementsJITQuery(version int) (string, int, [2]int, int)` returns the
query template, `Ncols`, `DiffIntvl`, and `UniqueKey`; `Configure()` patches all four onto the
view. Two-way branch: `version >= 170000` → `PgStatStatementsJITDefault` (15 cols), else →
`PgStatStatementsJITPG15` (13 cols).
**Rationale:** Unlike `statements_timings` (whose column *count* stays 13 across PG12/13/17
variants, so only `QueryTmpl` is swapped), the JIT column count changes (13 vs 15) because PG17
adds `deform_total`/`deform_ms`. With the synthetic md5 `queryid` as `UniqueKey`, columns cannot
be hidden (the align path floors every column at width 8 and is positional — ADR
`[006-feat-pg-stat-io]`), so each version needs a distinct column *set*, and `Ncols`/`DiffIntvl`/
`UniqueKey` must move with it. Returning all four is explicit and mirrors `SelectStatIOQuery`
(which returns `(string, int, [2]int)`; we add `UniqueKey` because, unlike `stat_io`'s key at a
fixed col 0, the JIT key sits at the end and shifts with `Ncols`).
**Alternatives considered:** (a) Return only the query (timings model) — rejected: leaves stale
`Ncols`/`DiffIntvl`/`UniqueKey` for PG17. (b) Return 3-tuple and compute `UniqueKey = Ncols-2`
in `Configure()` — works but hides a layout invariant in arithmetic; explicit return is clearer
and test-checkable. (c) Hide deform columns on PG15 via `ColsWidth` — impossible (ADR [006]).

### Decision 2: Column layout — single cumulative-total + single interval block (no doubling)
**Decision:** Columns: `user(0), database(1), gen_total, inline_total, opt_total, emit_total
[, deform_total], gen_ms*, inline_ms*, opt_ms*, emit_ms* [, deform_ms*], functions*, queryid,
query`. `*_total` are cumulative text durations (`date_trunc('seconds', round(<time>)/1000 *
'1 second')::text`); `*_ms` and `functions` are diffed interval columns. PG15/16: 13 cols,
`DiffIntvl {6,10}`, `UniqueKey 11`. PG17+: 15 cols, `DiffIntvl {7,12}`, `UniqueKey 13`.
**Rationale:** Mirrors the `statements_timings` shape (familiar to DBAs) and fits the terminal
width: pgcenter has no horizontal column scroll (ADR [006]), so the full total+interval doubling
of `statements_io` applied to 8–10 JIT metrics (~22–26 cols) would overflow. The per-phase
*time* breakdown is the cost signal; the four `*_count` metrics are dropped, keeping
`jit_functions` as the single representative counter.
**Alternatives considered:** All counts+times with total+interval doubling — rejected (too
wide). Splitting into count/time sub-screens like `pg_stat_io` — rejected: out of scope, JIT is
the lowest-risk closing feature and the time-only set fits one screen.

### Decision 3: Filter rows to JIT-active statements (`WHERE jit_functions > 0`)
**Decision:** Both query consts end with `WHERE p.jit_functions > 0`.
**Rationale:** Under default `jit=on`, only large-cost plans trigger JIT, so the overwhelming
majority of normalized statements have zero JIT activity — an unfiltered screen would be near-
useless. Follows the `pg_stat_io` count-based zero-row filter (ADR [006]). `jit_functions > 0`
is sufficient: any JIT work implies `jit_functions > 0`. It also cleanly yields an empty screen
under `jit=off`.
**Alternatives considered:** No filter (current `statements_io`/`statements_wal` behavior) —
rejected (mostly empty rows). Summing all counters in the predicate — unnecessary;
`jit_functions` already covers it.

### Decision 4: `jit=off` hint is the static view `Msg`, not a dynamic detector
**Decision:** Surface the empty-screen explanation as the view's `Msg`, e.g. `"Show statements
JIT compilation statistics (no rows when jit=off)"`, printed on the command line when the view
opens.
**Rationale:** pgcenter has no dynamic GUC-detection hint mechanism — the existing
"requires track_io_timing=on" note on `stat_io_time` is simply that view's `Msg` string
(view.go:190). Reusing `Msg` matches precedent and needs zero new machinery.
**Alternatives considered:** Query `current_setting('jit')` and print a conditional cmdline
notice — rejected: new code path for marginal value; no precedent.

### Decision 5: `NotRecordable: true`, no record/report wiring
**Decision:** Set `NotRecordable: true`; add no `report.go` description entry.
**Rationale:** TUI-first principle of release 0.11.0 (ADR `[004-feat-bgwriter-checkpointer]`);
direct precedent — `bgwriter`/`replslots`/`stat_io` are all `NotRecordable` with no `report.go`
entry. `record/record.go::filterViews` drops the view before the version/schema gates, so no
recorded data exists to report.
**Alternatives considered:** Wire record/report now — rejected (doubles scope; deferred to a
dedicated follow-up feature per roadmap).

## Data Models

No DB schema changes (read-only consumer of `pg_stat_statements`). Touched in-memory types:

- `view.View` — new map entry `statements_jit`; fields used: `Name, MinRequiredVersion,
  QueryTmpl, DiffIntvl [2]int, Ncols, OrderKey, OrderDesc, UniqueKey, ColsWidth, Msg, Filters,
  NotRecordable`.
- Query consts (`string`) + selector returning `(string, int, [2]int, int)`.

JIT source columns (confirmed vs PG15/PG17 official docs, see code-research §1):
PG15/16 — `jit_functions`(bigint), `jit_generation_time`,`jit_inlining_count`,
`jit_inlining_time`,`jit_optimization_count`,`jit_optimization_time`,`jit_emission_count`,
`jit_emission_time` (times = double ms). PG17+ adds `jit_deform_count`,`jit_deform_time`.

## Dependencies

### New packages
- None.

### Using existing (from project)
- `internal/query` — `query.Format()`, `PostgresV15/V17` constants, selector pattern.
- `internal/view` — `View` struct, `New()`, `Views.Configure()`, `VersionOK()`.
- `internal/stat` — collector `VersionOK` guard and duration-aware `sort`.
- `top/menu.go`, `top/config_view.go` — pgss menu + `x`-cycle wiring.

## Testing Strategy

**Feature size:** S

### Unit tests
- `SelectStatStatementsJITQuery`: PG15/16 → base const, `Ncols 13`, `DiffIntvl {6,10}`,
  `UniqueKey 11`; PG17/18 → default const, `Ncols 15`, `DiffIntvl {7,12}`, `UniqueKey 13`
  (mirror `TestSelectStatStatementsTimingQuery`).
- `view.New()` count assertion bumped `26 → 27`; optional `statements_jit` presence/field guard
  (mirror `TestNew_StatIOView`).
- `record.filterViews`: `Test_filterViews` `wantN +1` on all 6 rows (NotRecordable drops the
  view on every version; `wantV` unchanged).

### Integration tests
- None as a separate suite. The live query against `pg_stat_statements` is exercised by the
  existing version-matrix exec test (mirror the WAL PG13+ gated loop): add a JIT exec sub-test
  gated PG15+, run by the CI matrix PG14–18 (skips locally without PG via `t.Skipf`). This is
  the only place the Ncols/DiffIntvl/UniqueKey-vs-real-columns consistency is verified.

### E2E tests
- None — pgcenter has no automated TUI E2E layer; manual TUI check on local PG17.

## Agent Verification Plan

**Source:** user-spec "Как проверить" section.

### Verification approach
Automated: `make build`, `make test` (race+coverage), `make lint`. The version-branch query
correctness is verified by the gated exec test on the CI PG14–18 matrix. The TUI menu/cycle and
empty-screen hint are verified manually on local PG17.

### Per-task verification
| Task | verify: | What to check |
|------|---------|--------------|
| 1 | bash | `go test ./internal/query/...` — selector returns expected query/Ncols/DiffIntvl/UniqueKey per version |
| 2 | bash | `go test ./internal/view/... ./record/...` — count tests pass (TestNew 27, filterViews +1) |
| 3 | user | local PG17: `X` → JIT opens `statements_jit`; `x` cycles `wal → jit → timings`; columns + sort + `/` filter work; `make build` clean |
| 4 | bash | `make test && make lint` green; CI matrix PG14–18 green (PG14 → "not supported", PG17/18 → deform columns) |

### Tools required
bash (go test, make). No MCP/Playwright/curl — TUI feature.

## Backward Compatibility

N/A — adding new code only. New view entry, new query consts/selector, additive menu item and
cycle link. No existing API, query, view, or config is modified in a breaking way. The two
count-test edits are test expectations tracking the additive change, not behavior changes.

One user-visible behavior shift, non-breaking: the `x`-cycle order changes from `… wal →
timings …` to `… wal → jit → timings …` (one extra stop inserted). No screen is removed or
reordered relative to each other; existing muscle memory for every other transition is
preserved.

**Breaking changes:** no
**Consumer impact:** none found — `statements_jit` is a new view name; no existing caller
references it.

## Risks

| Risk | Mitigation |
|------|-----------|
| `Ncols`/`DiffIntvl`/`UniqueKey` out of sync with the real column count (esp. PG17 15-col branch) — silent diff/align bug | Selector returns all four explicitly + unit test asserts them per version; gated exec test on CI PG15–18 validates against real columns |
| Count-test breakage (`TestNew`, `Test_filterViews`) masked locally — caught only when tests run | Both run without PG, so `make test` catches them locally; edits are in Task 2 scope + acceptance criteria (lesson from feature 006) |
| PG<15 menu/cycle target a view that can't be collected | Existing `VersionOK` runtime guard (`stat.go:199`) returns "not supported" — verified; no zero-value `View{}` (view always in map) |
| `jit=off` shows an empty screen that reads as a bug | `Msg` text explains the empty state (Decision 4) |

## Acceptance Criteria

Технические критерии (дополняют пользовательские из user-spec):

- [ ] `SelectStatStatementsJITQuery` returns correct query + `Ncols` + `DiffIntvl` + `UniqueKey`
      for PG15/16 (13/`{6,10}`/11) and PG17/18 (15/`{7,12}`/13); unit-tested.
- [ ] Both JIT query consts include `WHERE jit_functions > 0` and follow the canonical pgss
      column layout (md5 queryid as `UniqueKey`, `query` last).
- [ ] `statements_jit` view: `MinRequiredVersion: query.PostgresV15`, `NotRecordable: true`,
      `OrderKey: 2`, `OrderDesc: true`, `Msg` carrying the `jit=off` empty-state note.
- [ ] `Configure()` patches `QueryTmpl/Ncols/DiffIntvl/UniqueKey` from the selector.
- [ ] `X` menu shows a 7th item; selecting it opens `statements_jit`. `x` cycles
      `… wal → jit → timings …`.
- [ ] `view_test.go::TestNew` expects 27; `record/record_test.go::Test_filterViews` `wantN +1`
      on all rows.
- [ ] No `report.go` description entry added (NotRecordable precedent).
- [ ] `make build`, `make test`, `make lint` green locally; CI PG14–18 matrix green.
- [ ] No regressions in existing view/record/query tests.

## Implementation Tasks

### Wave 1 (независимые)

#### Task 1: JIT query consts + version selector
- **Description:** Add `PgStatStatementsJITPG15` (PG15/16, 8 JIT metrics) and
  `PgStatStatementsJITDefault` (PG17+, +deform) query consts following the canonical pgss
  layout with `WHERE jit_functions > 0`, plus `SelectStatStatementsJITQuery(version)` returning
  query + `Ncols` + `DiffIntvl` + `UniqueKey`. This is the version-correct data source for the
  new screen and the only layer where the PG15-vs-PG17 column difference lives.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/query/...`
- **Files to modify:** `internal/query/statements.go`, `internal/query/statements_test.go`
- **Files to read:** `internal/query/statements.go` (timings/io consts + `SelectStatStatementsTimingQuery`), `internal/query/io.go` (`SelectStatIOQuery` return shape), `internal/query/query.go` (version constants), `internal/query/statements_test.go`

#### Task 2: Register `statements_jit` view + Configure + count-test fixes
- **Description:** Add the `statements_jit` view entry (PG15+ gate, `NotRecordable`, `OrderKey 2`
  desc, hint `Msg`) and a `Configure()` case that binds the version-selected query and layout
  metadata. Update the two count-tests that break when a view is added. This makes the screen a
  first-class, version-aware, non-recordable view.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/view/... ./record/...`
- **Files to modify:** `internal/view/view.go`, `internal/view/view_test.go`, `record/record_test.go`
- **Files to read:** `internal/view/view.go` (statements_io/statements_wal/stat_io entries + `Configure()`), `internal/query/io.go`, `record/record.go` (`filterViews`), `record/record_test.go`

#### Task 3: TUI menu item + `x`-cycle wiring
- **Description:** Add a 7th `menuPgss` item and its `menuSelect` case for `statements_jit`, and
  insert `statements_jit` into the `statementsNextView` cycle between `wal` and `timings`. This
  exposes the screen via the `X` menu and the `x` toggle; no keybinding changes are needed.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** user — local PG17: `X` → JIT opens; `x` cycles `wal → jit → timings`; `make build` clean
- **Files to modify:** `top/menu.go`, `top/config_view.go`
- **Files to read:** `top/menu.go` (`selectMenuStyle` menuPgss + `menuSelect`), `top/config_view.go` (`statementsNextView`, `switchViewTo`), `top/keybindings.go`

### Final Wave

#### Task 4: Pre-deploy QA
- **Description:** Acceptance testing: run `make build`, `make test`, `make lint`; verify the
  acceptance criteria from user-spec and tech-spec; confirm the JIT screen behavior on local
  PG17 and that the CI PG14–18 matrix is green.
- **Skill:** pre-deploy-qa
- **Reviewers:** none
