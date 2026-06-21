---
created: 2026-06-21
status: draft
branch: feature/pg-stat-io
size: M
---

# Tech Spec: pg_stat_io screen (unified IO breakdown)

## Solution

Add a new TUI-only screen for `pg_stat_io` (PostgreSQL 16+) to `pgcenter top`, modeled on the
existing multi-row version-aware screens (`replslots`) and the sub-screen menu pattern
(`pg_stat_statements`). The screen is split into two sub-views — **count** (operation counters +
KiB throughput) and **time** (operation timings) — because pgcenter has no horizontal column
scroll and the full column set does not fit. Cumulative counters are shown as per-interval rates
through the existing query → format → diff pipeline; `stats_age` is the only absolute metric.

The data layer is a new `internal/query/io.go` with two version-aware selectors
(`SelectStatIOQuery`, `SelectStatIOTimeQuery`) returning `(query, Ncols, DiffIntvl)` — one branch
for PG 16/17 (`op_bytes`), one for PG 18 (native `*_bytes`, plus `object='wal'` / `context='init'`
rows). Row identity (`backend_type × object × context`) is collapsed into a synthetic md5 `io_key`
column used as `UniqueKey`, exactly as `pg_stat_statements` collapses `userid/dbid/queryid`. The
screen is registered `NotRecordable: true`; `record`/`report` support is a separate 0.11.0 feature.

## Architecture

### What we're building/modifying

- **`internal/query/io.go` (new)** — query constants and the two selectors. Per-version SQL,
  `coalesce(...,0)` on every diffed column, KiB derived via integer `/1024`, synthetic `io_key`,
  SQL-side `WHERE` to drop all-zero rows.
- **`internal/query/query.go`** — add `PostgresV15/16/17/18` numeric-version constants (the
  version list currently stops at `PostgresV14`).
- **`internal/view/view.go`** — register two views (`stat_io`, `stat_io_time`) in `New()` and wire
  the selectors in `Configure()`.
- **`top/menu.go`** — new `menuStatIO` type with a 2-item style and a `menuSelect` branch.
- **`top/keybindings.go`** — `j` (enter + toggle) and `J` (open menu) bindings.
- **`top/config_view.go`** — `statioNextView` 2-way toggle helper + a `case "statio"` in
  `switchViewTo`.
- **`top/help.go`** — help line for `j`/`J` and a note that `Q` does not reset `pg_stat_io`.
- **Tests** — `internal/query/io_test.go` (new), additions to `top/menu_test.go`.

### How it works

```
keypress j / J
  → switchViewTo "statio" (statioNextView toggle)  OR  menuOpen(menuStatIO) → menuSelect
  → viewSwitchHandler loads view "stat_io" / "stat_io_time" from the static map
  → Collector.Update(): VersionOK gate → run view.Query (already version-selected at Configure)
  → stat.diff(): rows matched by UniqueKey (io_key), DiffIntvl columns → per-interval rates
  → align + print: 16 (count) / 10 (time) columns rendered
```

Version selection happens once at connect time: `top.go` builds `query.Options{Version}` →
`views.Configure(opts)` calls `SelectStatIOQuery(version)` / `SelectStatIOTimeQuery(version)`,
which pick the PG 16/17 or PG 18 SQL and set `QueryTmpl`, `Ncols`, `DiffIntvl`. On PG 14/15 the
view is still configured, but the collector's `VersionOK` gate (`MinRequiredVersion = PostgresV16`)
returns the standard "not supported" error to the main pane.

## Decisions

### Decision 1: Two sub-screens (count / time), not one wide table
**Decision:** Split `pg_stat_io` into two registered views — `stat_io` (counts + KiB) and
`stat_io_time` (timings).
**Rationale:** pgcenter has no horizontal column scroll; the full ~20-counter set cannot fit one
screen. This mirrors how `pg_stat_statements` is split into sub-screens for the same reason.
**Alternatives considered:** One wide table (columns past terminal width are silently cut —
unreadable); aggregation by `backend_type` (loses the `object`/`context` breakdown, which is the
core value). Both rejected.

### Decision 2: Visible md5 `io_key` as `UniqueKey`, with 3 separate dimension columns
**Decision:** Emit a synthetic `left(md5(backend_type||object||context),10) AS io_key` as column 0,
set `UniqueKey: 0`, and also display `backend_type`, `object`, `context` as separate sortable
columns.
**Rationale:** `view.UniqueKey` is a single column index and `stat.diff()` matches rows on exactly
one column (`internal/stat/postgres.go`), so the 3-field identity must be collapsed into one key —
the proven `statements_io` `queryid` pattern. Separate dimension columns are kept so the per-column
`/` filter stays precise on the ~30 rows (user decision).
**Alternatives considered:** Hiding `io_key` via `ColsWidth[0]=0` — **not possible**: `SetAlign`
(`internal/align/align.go:36`) computes every column's width as `max(len(colname), 8)`, with no
zero-width / skip path, and `ColsWidth` holds those *computed* widths at runtime (it is not a preset
read during rendering). Hiding a column would require new code in the shared align/print path (scope
creep, touches every screen). A single readable `backend_type/object/context` concat column (no hash, narrower) — rejected
because it loses per-dimension sort and makes `/` filtering ambiguous (`/vacuum` would match both
`context=vacuum` and `autovacuum worker`).

### Decision 3: Per-version query branches PG 16/17 vs PG 18
**Decision:** Two SQL branches. PG 16/17 derives KiB from `op_bytes` (`reads*op_bytes/1024`);
PG 18 uses native `read_bytes/write_bytes/extend_bytes` (`read_bytes/1024`). The **logical column
set, `Ncols`, and `DiffIntvl` are identical** across branches — only the SQL source of the KiB
columns differs. PG 18 additionally returns `object='wal'` and `context='init'` rows (extra rows,
not extra columns).
**Rationale:** ADR [004] (per-version column sets, not NULL-padded). `op_bytes` was removed in PG 18
and replaced by native byte counters; a single query cannot serve both. Identical headers keep the
DBA's mental model stable across versions (variant A).
**Alternatives considered:** Single NULL-padded query — rejected by ADR [004]. A separate PG 17
branch — unnecessary: PG 16 and PG 17 expose an identical `pg_stat_io` column set (verified).

### Decision 4: Rates only; `stats_age` absolute outside `DiffIntvl`
**Decision:** All counters are diffed (per-interval rates). `stats_age = now() - stats_reset` is the
last column, outside the diff interval. Count screen `DiffIntvl=[4,14]`; time screen `DiffIntvl=[4,8]`.
**Rationale:** Live top-like tool — the signal is the rate, not the ever-growing cumulative value
(ADR [004] absolute-via-placement; same as `wal`/`bgwriter`/`replslots`).
**Alternatives considered:** pgss-style absolute totals + deltas — rejected (doubles width, not
actionable live).

### Decision 5: `coalesce(...,0)` on diffed columns + SQL-side empty-row filter
**Decision:** Wrap every diffed column in `coalesce(...,0)`. Drop all-zero rows in SQL:
`WHERE coalesce(reads,0)+...+coalesce(fsyncs,0) > 0`. The time screen uses the **same** count-based
`WHERE` so both sub-screens show an identical row-set.
**Rationale:** `pg_stat_io` returns NULL pervasively (`fsyncs` for `temp relation`, `reads` for
`background writer`); a NULL inside `DiffIntvl` reaches `strconv.ParseInt("")` and aborts the whole
sample → blank screen (ADR [005], the replslots lesson). pgcenter has no client-side empty-row hook,
so filtering belongs in SQL.
**Alternatives considered:** Client-side row dropping — no such hook exists; would need new stat-layer
code.

### Decision 6: KiB via integer division in SQL
**Decision:** Compute KiB as integer `(... )/1024`, not floating-point.
**Rationale:** PG 18 `*_bytes` are `numeric`; rendering them or their float division prints decimals
and routes through `parsePairFloat`. Integer `/1024` keeps values integer-typed and clean (same as
`replslots` `retained,KiB`).

### Decision 7: Navigation — `j` toggle + `J` menu
**Decision:** `j` → `switchViewTo(app,"statio")` backed by a `statioNextView` 2-way toggle
(`stat_io`↔`stat_io_time`, default `stat_io` from any other view). `J` → `menuOpen(menuStatIO,...)`
with a 2-item menu (`pg_stat_io operations`, `pg_stat_io timings`).
**Rationale:** Matches the established lowercase-switch / uppercase-menu pairs (`x`/`X`, `d`/`D`).
A lowercase 2-way toggle reuses the `databasesNextView` precedent. Both `j` and `J` are free keys.
**Alternatives considered:** Menu only (no toggle) — clunky for two states; toggle only (no menu) —
breaks the established uppercase-menu convention and discoverability.
**Naming contract (for the implementer):** view names `stat_io` / `stat_io_time`, `switchViewTo`
token `"statio"`, menu type `menuStatIO` — the same three-way naming the `databases` family uses
(`databases` / `databases_general` / `menuDatabases`).

### Decision 8: Add `PostgresV15/16/17/18` version constants
**Decision:** Add `PostgresV15=150000, PostgresV16=160000, PostgresV17=170000, PostgresV18=180000`
to `internal/query/query.go`; reference them everywhere — `MinRequiredVersion: query.PostgresV16` in
the view map AND the `io.go` selector branches (`version >= query.PostgresV18` / `>= query.PostgresV16`).
**Rationale:** The `view.New()` map uses `query.PostgresVxx` constants for `MinRequiredVersion`; the
constant list currently stops at `V14`. Adding them and using them consistently in both the map and
the selectors avoids a literal/constant split. (The `bgwriter`/`wal` selectors use bare literals only
because these constants did not exist yet; this feature introduces them.)

### Decision 9: `track_io_timing` hint as a static cmdline message
**Decision:** Surface the `track_io_timing` caveat via the time-view `Msg` (shown in the cmdline on
switch), e.g. `pg_stat_io timings (require track_io_timing=on)`.
**Rationale:** A static, always-present hint is simpler and more proactive than scanning every sample
for an all-zero condition; it tells the DBA up front why timings may be zero. Avoids per-refresh data
inspection.

### Decision 10: `NotRecordable: true` — record/report deferred
**Decision:** Both views set `NotRecordable: true`.
**Rationale:** Release 0.11.0 TUI-first principle (ADR [004]); `record`/`report` for the new screens
is a separate 0.11.0 feature. Keeps this feature size-M.

### Decision 11: Default sort by `reads` / `read_time` desc
**Decision:** `OrderKey: 4` (the first numeric column — `reads` on count, `read_time` on time),
`OrderDesc: true`.
**Rationale:** Busiest-first is the useful default for incident triage; user choice. Precedent for a
non-col-0 default sort: `replslots` (ADR [005], OrderKey deviation).

## Data Models

Synthetic SQL column emitted by both screens for row identity:
`left(md5(backend_type || object || context), 10) AS io_key`.

**count screen (`stat_io`)** — Ncols 16, `DiffIntvl [4,14]`, `UniqueKey 0`, `OrderKey 4`:

| idx | column | kind | source |
|-----|--------|------|--------|
| 0 | io_key | absolute (key) | md5(dims) |
| 1 | backend_type | absolute | view |
| 2 | object | absolute | view |
| 3 | context | absolute | view |
| 4 | reads | diff | view |
| 5 | read,KiB | diff | `reads*op_bytes/1024` (PG16/17) · `read_bytes/1024` (PG18) |
| 6 | writes | diff | view |
| 7 | write,KiB | diff | `writes*op_bytes/1024` · `write_bytes/1024` |
| 8 | extends | diff | view |
| 9 | ext,KiB | diff | `extends*op_bytes/1024` · `extend_bytes/1024` |
| 10 | hits | diff | view |
| 11 | evictions | diff | view |
| 12 | writebacks | diff | view |
| 13 | reuses | diff | view |
| 14 | fsyncs | diff | view |
| 15 | stats_age | absolute | `now()-stats_reset` |

Column order honours the user-spec priority: data-moving ops (`reads`/`writes`/`extends` + KiB) and
`hits`/`evictions` come first and stay visible on 120–160-col terminals; lower-priority
`writebacks`/`reuses`/`fsyncs` and `stats_age` trail and clip first. The whole counter block stays
one contiguous `DiffIntvl` regardless of internal order.

**time screen (`stat_io_time`)** — Ncols 10, `DiffIntvl [4,8]`, `UniqueKey 0`, `OrderKey 4`:

| idx | column | kind |
|-----|--------|------|
| 0 | io_key | absolute (key) |
| 1 | backend_type | absolute |
| 2 | object | absolute |
| 3 | context | absolute |
| 4 | read_time | diff |
| 5 | write_time | diff |
| 6 | writeback_time | diff |
| 7 | extend_time | diff |
| 8 | fsync_time | diff |
| 9 | stats_age | absolute |

Both screens share the same `WHERE coalesce(reads,0)+...+coalesce(fsyncs,0) > 0` (count-based) so
the row-set is identical.

## Dependencies

### New packages
- None.

### Using existing (from project)
- `internal/query` — selector + `Format` pipeline (model: `bgwriter.go`, `wal.go`, `replication_slots.go`).
- `internal/view` — `View` struct, `New()`, `Configure()`, `VersionOK`.
- `internal/stat` — `diff()` (UniqueKey row matching), `diffPair`/`coalesce` NULL-safety.
- `top/menu.go`, `top/config_view.go`, `top/keybindings.go`, `top/help.go` — TUI wiring.
- `internal/postgres/testing.go` — `NewTestConnectVersion` (port map PG14–18) for integration tests.

## Testing Strategy

**Feature size:** M

### Unit tests
- `SelectStatIOQuery` / `SelectStatIOTimeQuery` over versions {14,15,16,17,18}: assert returned
  `(Ncols, DiffIntvl)` per branch (16 / [4,14] count, 10 / [4,8] time; PG16/17 and PG18 share shape).
- Diff safety: a result row containing NULL in a diffed column does not abort the sample (NULL→0).
- `selectMenuStyle(menuStatIO)` returns exactly 2 items (`top/menu_test.go`).

### Integration tests
- Live query on the CI matrix PG 14–18 (`NewTestConnectVersion`, `t.Skipf` for unavailable):
  - PG 16 run gates the `op_bytes` KiB path.
  - PG 18 run gates native `*_bytes` and the presence of `object='wal'` rows (the shape the local
    PG17-only env cannot verify).
  - PG 14/15: assert the view is gated (not executed) or skip — `pg_stat_io` does not exist there.

### E2E tests
- None — pgcenter has no E2E layer for the TUI. Functional coverage = integration + manual QA.

## Agent Verification Plan

**Source:** user-spec "Как проверить" section.

### Verification approach
Automated: `make build`, `make test` (race + coverage), the selector/menu unit tests, and the
PG14–18 integration matrix in CI. Manual (user): walk US1–US4 on a live PG17 (and PG18 if available),
confirming row rendering, `j` toggle, `J` menu, `/` filter, and the PG14/15 "not supported" message.

### Per-task verification
| Task | verify: | What to check |
|------|---------|--------------|
| 1 | bash | `go test ./internal/query/...` — selector (Ncols/DiffIntvl) + NULL-safety pass on all versions |
| 2 | bash | `go test ./internal/view/...` — views configured, `go build ./...` clean |
| 3 | bash | `go test ./top/...` — `menuStatIO` has 2 items; `make build` succeeds |
| 4 | bash | `make build && make test && make lint` green; manual US1–US4 walk |

### Tools required
bash (go test, make). No MCP/browser tools — TUI feature.

## Backward Compatibility

N/A — adding new code only (new view, new query file, new keybindings on previously-free keys
`j`/`J`). Adding `PostgresV15/16/17/18` constants is additive. No existing API, public function, or
config is changed.

**Breaking changes:** no
**Consumer impact:** none found — `j`/`J` were unbound; no existing view/selector signatures change.

## Risks

| Risk | Mitigation |
|------|-----------|
| NULL inside `DiffIntvl` aborts the sample (blank screen) — pervasive in `pg_stat_io` | `coalesce(...,0)` on every diffed column + a dedicated NULL-row unit test (Decision 5) |
| Unverified against live PG 18 — native `*_bytes` (numeric), `object='wal'`, `op_bytes` removal are assumed from PG18 docs/release notes, not a local cluster | The CI PG 18 integration test is the gate; must pass before merge. PG16 test gates the `op_bytes` path. Marked explicitly as an unverified external dependency. |
| count screen ~170 cols → trailing columns clip on narrow terminals | Conscious tradeoff (user-spec); column priority documented; dims sortable/filterable; standard pgcenter truncation |
| Composite-key correctness — wrong `UniqueKey` silently produces wrong deltas | Synthetic `io_key` col 0, `UniqueKey:0` + an integration assertion that deltas are correct when ≥2 rows share a `backend_type` (Decision 2) |
| PG 18 `numeric` `*_bytes` render with decimals | Integer `/1024` in SQL keeps values integer-typed (Decision 6) |
| `Test_doReload` panics locally without the PG17 fixture (pre-existing tech-debt [005]) | Out of scope; may make `make test` red locally — run the targeted package tests, rely on CI |

## Acceptance Criteria

Технические критерии (дополняют пользовательские из user-spec):

- [ ] `SelectStatIOQuery`/`SelectStatIOTimeQuery` return correct `(Ncols, DiffIntvl)` for PG16/17 and PG18 branches; unit tests pass on versions {14,15,16,17,18}.
- [ ] A NULL in any diffed column does not abort the sample (NULL→0); covered by a unit test.
- [ ] Views `stat_io` / `stat_io_time` registered with `NotRecordable:true`, `MinRequiredVersion=PostgresV16`, `UniqueKey:0`, `OrderKey:4`, `OrderDesc:true`.
- [ ] `j` enters/toggles count↔time; `J` opens a 2-item menu; `menuStatIO` item-count test passes.
- [ ] Live integration query succeeds on PG16–18; PG18 run shows native bytes + `object='wal'` rows; PG14/15 gated with the standard "not supported" message.
- [ ] `make build`, `make test`, `make lint`, `make vuln` all green.
- [ ] No regressions in existing view/menu/query tests.

## Implementation Tasks

### Wave 1 (независимые)

#### Task 1: Query layer — `internal/query/io.go` + version constants
- **Description:** Build the data layer for `pg_stat_io`: per-version SQL constants and the two
  selectors `SelectStatIOQuery` / `SelectStatIOTimeQuery` returning `(query, Ncols, DiffIntvl)`,
  plus the `PostgresV15/16/17/18` constants. The SQL emits the synthetic `io_key`, derives KiB via
  integer `/1024` (op_bytes on PG16/17, native `*_bytes` on PG18), coalesces all diffed columns, and
  drops all-zero rows. Per Decisions 2/3/5/6/8.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/query/...` passes (selector shape per version + NULL-safety + live PG16/18 shape)
- **Files to modify:** `internal/query/query.go`
- **Files to create:** `internal/query/io.go`, `internal/query/io_test.go`
- **Files to read:** `internal/query/bgwriter.go`, `internal/query/wal.go`, `internal/query/replication_slots.go`, `internal/query/statements.go`, `internal/query/bgwriter_test.go`, `internal/postgres/testing.go`

### Wave 2 (зависит от Wave 1)

#### Task 2: View registration — `internal/view/view.go`
- **Description:** Register the two views `stat_io` and `stat_io_time` in `view.New()` and wire the
  selectors in `Configure()`, with the field values and cmdline `Msg` specified in the Decisions and
  Data Models sections. Depends on the selectors and view names contracted in Wave 1.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./internal/view/...` and `go build ./...` clean
- **Files to modify:** `internal/view/view.go`
- **Files to read:** `internal/query/io.go`, `internal/view/view_test.go`

#### Task 3: TUI navigation, menu & help — `top/`
- **Description:** Add the `j` (enter + `statioNextView` toggle) and `J` (`menuOpen(menuStatIO)`)
  keybindings, the `menuStatIO` type with a 2-item style and `menuSelect` branch, the `statioNextView`
  helper + `case "statio"` in `switchViewTo`, and the help-screen line for `j`/`J` plus the note that
  `Q` does not reset `pg_stat_io`. Switches to the view names registered in Task 2 (fixed contract).
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./top/...` (`menuStatIO` 2 items) and `make build` succeed
- **Files to modify:** `top/keybindings.go`, `top/menu.go`, `top/config_view.go`, `top/help.go`, `top/menu_test.go`
- **Files to read:** `top/config_view.go` (databasesNextView, switchViewTo, viewSwitchHandler), `top/menu.go` (menuPgss), `top/keybindings.go`

### Final Wave

#### Task 4: Pre-deploy QA
- **Description:** Acceptance testing: run the full suite and verify the acceptance criteria from
  user-spec and tech-spec — `make build`, `make test`, `make lint`, `make vuln`, and the manual
  US1–US4 walk on a live PG17 (and PG18 if available), including the PG14/15 "not supported" path.
- **Skill:** pre-deploy-qa
- **Reviewers:** none
