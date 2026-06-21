---
created: 2026-06-21
status: draft
branch: develop
size: M
---

# Tech Spec: pg_stat_bgwriter + pg_stat_checkpointer screen

## Solution

Add a new single-row TUI screen `bgwriter` to `pgcenter top`, structurally identical to the
existing `pg_stat_wal` screen. A new `internal/query/bgwriter.go` provides version-aware query
constants and a `SelectStatBgwriterQuery(version) (string, int, [2]int)` selector returning the
query template, column count, and the diff interval. The view is registered in
`internal/view/view.go` (with `NotRecordable: true`) and wired into `Configure()`; hotkey `b` is
bound in `top/keybindings.go`; help text is updated in `top/help.go`. The data path is pure SQL —
no enrichment, no `CollectExtra` — so the existing collector/diff/render pipeline handles it
unchanged.

The screen reads `pg_stat_bgwriter` on all supported versions and cross-joins
`pg_stat_checkpointer` on PG 17+. Event counters (`ckpt_*`, `rstpt_*`) are placed in a contiguous
absolute block immediately after the `source` label and left OUTSIDE `DiffIntvl`, so they render
as absolute cumulative values; work/time/buffer columns form the contiguous diffed block;
`stats_age` is the last column, outside the diff range.

## Architecture

### What we're building/modifying

- **`internal/query/bgwriter.go`** (NEW) — query constants per version branch + selector
  `SelectStatBgwriterQuery`. Template: `internal/query/wal.go`.
- **`internal/query/bgwriter_test.go`** (NEW) — unit (per-version `Ncols`/`DiffIntvl`) +
  integration (execute on live PG 14–18) tests. Template: `internal/query/wal_test.go`.
- **`internal/view/view.go`** (MODIFY) — add `"bgwriter"` view entry with `NotRecordable: true`;
  add `case "bgwriter"` in `Configure()` calling the selector.
- **`top/keybindings.go`** (MODIFY) — add `{"sysstat", 'b', switchViewTo(app, "bgwriter")}`.
- **`top/help.go`** (MODIFY) — add `b` to the mode-key help row (sorted row `a,b,f,r,w`).
- **`.claude/skills/project-knowledge/overview.md`** (MODIFY) — correct the false claim that
  `pg_stat_bgwriter` is already supported; mention the new screen.

No changes to `top/reset.go`, `record/record.go`, or `cmd/report/report.go` — `NotRecordable: true`
keeps the view out of recording (and structurally out of the report path), and shared
bgwriter/checkpointer counters are already excluded from `pgcenter`'s `Q` reset.

### How it works

1. User presses `b` → `switchViewTo(app, "bgwriter")` → `viewSwitchHandler` loads the `bgwriter`
   view and sends it on `viewCh`.
2. At connection time, `view.Configure(opts)` calls `SelectStatBgwriterQuery(opts.Version)` and
   assigns the version-specific `QueryTmpl`, `Ncols`, `DiffIntvl`; the generic second loop runs
   `query.Format()` to produce the final query string.
3. The collector executes the query (single row), and `internal/stat/postgres.go:diff()` applies
   `DiffIntvl`: columns inside the range are subtracted vs the previous sample (deltas), columns
   outside (the `source` label, the absolute event counters, and `stats_age`) are copied as-is.
4. The TUI renders the single row each refresh.

## Decisions

### Decision 1: Mirror the pg_stat_wal pattern instead of inventing a view type
**Decision:** Build the screen as a single-row, version-aware view using the exact
`pg_stat_wal` structure (`SelectStatWALQuery` → `SelectStatBgwriterQuery`).
**Rationale:** `pg_stat_bgwriter`/`pg_stat_checkpointer` are single-row global-counter views, the
same shape as `pg_stat_wal`. The pattern is proven, tested, and keeps the collector/diff/render
path untouched.
**Alternatives considered:** A multi-row or hybrid (`CollectExtra`) view — rejected: there is no
per-object dimension and no non-SQL data source; that machinery is unnecessary.

### Decision 2: Per-version column sets, not NULL-padded unified columns
**Decision:** Each version branch returns only the columns that actually exist on that version;
shared columns keep identical headers and order. `Ncols`/`DiffIntvl` differ per version (PG14-16:
12 cols; PG17: 13; PG18: 14).
**Rationale:** `wal.go` already returns different `Ncols`/`DiffIntvl` per version (11 on PG14, 7 on
PG18) — this is the established precedent. NULL-padding pre-17 with empty restartpoint columns
would show misleading blank columns to a PG15 DBA.
**Alternatives considered:** Unified header set with `NULL AS rstpt_*` placeholders on PG14-16 —
rejected: clutters the screen with always-empty columns and contradicts the wal precedent.

### Decision 3: Absolute event counters via DiffIntvl placement
**Decision:** Place `ckpt_timed`, `ckpt_req`, and (PG17+) `rstpt_timed/req/done` in a contiguous
block right after `source`, outside `DiffIntvl`; the diffed work/time/buffer columns form the
single contiguous diff range; `stats_age` is last.
**Rationale:** `DiffIntvl` is a single contiguous `[lo,hi]` range (`postgres.go:diff()`), so the
only way to render event counters as absolute is to keep them outside the range. Checkpoints
increment rarely; a per-interval delta would almost always be 0, while the cumulative
timed-vs-requested ratio is what the DBA needs.
**Alternatives considered:** Diff everything (wal-style) — rejected for event counters per the
user-spec; they would flicker between 0 and 1.

### Decision 4: stats_age sourced from pg_stat_checkpointer on PG17+
**Decision:** On PG17+ the `stats_age` column derives from `pg_stat_checkpointer.stats_reset`.
**Rationale:** The screen's primary content on modern versions is checkpoint data; one column is
cleaner than two reset ages. Documented in the user-spec so an independently-reset bgwriter is not
a surprise.
**Alternatives considered:** Show both reset ages, or the older of the two — rejected as needless
column noise for a secondary signal.

### Decision 5: No security reviewer on the query task
**Decision:** Task 1 (query layer) uses `dev-code-reviewer` + `dev-test-reviewer` only.
**Rationale:** The bgwriter SQL is static `const` strings with no user interpolation (identical
style to `wal.go`); `Format()` substitutes no user-controlled values into it. There is no
injection surface, so a security audit adds no signal.
**Alternatives considered:** Full default trio — rejected as noise for static SQL.

## Data Models

Column layouts (0-based). Absolute = outside `DiffIntvl`; Diff = inside.

**PG 14–16** — `FROM pg_stat_bgwriter` — `Ncols = 12`, `DiffIntvl = [3,10]`:
```
0  source            'Bgwriter'                       (absolute/text)
1  ckpt_timed        checkpoints_timed                (absolute)
2  ckpt_req          checkpoints_req                  (absolute)
3  ckpt_write,ms     checkpoint_write_time            (diff)
4  ckpt_sync,ms      checkpoint_sync_time             (diff)
5  buf_ckpt          buffers_checkpoint               (diff)
6  buf_clean         buffers_clean                    (diff)
7  maxwritten        maxwritten_clean                 (diff)
8  buf_backend       buffers_backend                  (diff)
9  buf_backend_fsync buffers_backend_fsync            (diff)
10 buf_alloc         buffers_alloc                    (diff)
11 stats_age         date_trunc(... now()-stats_reset) (absolute/text, excluded)
```

**PG 17** — `FROM pg_stat_bgwriter, pg_stat_checkpointer` (cross join, single×single row) —
`Ncols = 13`, `DiffIntvl = [6,11]`:
```
0  source       'Bgwriter'
1  ckpt_timed   checkpointer.num_timed            (absolute)
2  ckpt_req     checkpointer.num_requested        (absolute)
3  rstpt_timed  checkpointer.restartpoints_timed  (absolute)
4  rstpt_req    checkpointer.restartpoints_req    (absolute)
5  rstpt_done   checkpointer.restartpoints_done   (absolute)
6  ckpt_write,ms checkpointer.write_time          (diff)
7  ckpt_sync,ms  checkpointer.sync_time           (diff)
8  buf_ckpt     checkpointer.buffers_written      (diff)
9  buf_clean    bgwriter.buffers_clean            (diff)
10 maxwritten   bgwriter.maxwritten_clean         (diff)
11 buf_alloc    bgwriter.buffers_alloc            (diff)
12 stats_age    checkpointer.stats_reset → age    (excluded)
```

**PG 18** — as PG17 plus `slru_written` (diffed) — `Ncols = 14`, `DiffIntvl = [6,12]`:
```
... cols 0-8 as PG17 ...
9  slru_written checkpointer.slru_written         (diff)   <-- inserted in the diffed block
10 buf_clean    bgwriter.buffers_clean            (diff)
11 maxwritten   bgwriter.maxwritten_clean         (diff)
12 buf_alloc    bgwriter.buffers_alloc            (diff)
13 stats_age    checkpointer.stats_reset → age    (excluded)
```
> `slru_written` placement within the diffed block is flexible (the range stays contiguous);
> grouped next to `buf_ckpt` for readability. Exact PG18 column set MUST be verified on a live
> PG18 cluster before finalizing the query (see Risks).

`SelectStatBgwriterQuery(version int) (string, int, [2]int)` branches: `>= 180000` → PG18 const,
14, `[6,12]`; `>= 170000` → PG17 const, 13, `[6,11]`; else → PG14-16 const, 12, `[3,10]`. Version
literals `170000`/`180000` (no `PostgresV17/18` constants exist); `MinRequiredVersion = query.PostgresV14`.

## Dependencies

### New packages
- None.

### Using existing (from project)
- `internal/query` — query const + selector pattern (`wal.go`), `Format()`, version constants.
- `internal/view` — `View` struct (incl. `NotRecordable`), `Configure()` wiring.
- `internal/stat` — `diff()` applies `DiffIntvl` (no change).
- `internal/postgres` — `NewTestConnectVersion` for integration tests.
- `top` — `switchViewTo`/`viewSwitchHandler`, keybindings, help.

## Testing Strategy

**Feature size:** M

### Unit tests
- `Test_SelectStatBgwriterQuery` — table-driven over PG 14/15/16/17/18: assert returned
  `Ncols` and `DiffIntvl` per version (12/[3,10], 12/[3,10], 12/[3,10], 13/[6,11], 14/[6,12]).
- Assert the selector picks the correct query const at the 170000 and 180000 boundaries.

### Integration tests
- `Test_StatBgwriterQueries` — loop PG 14–18: `Format()` the template, `NewTestConnectVersion`,
  `t.Skipf` if the version is unavailable, execute the query, assert no error. This is where the
  PG18 `slru_written` column set is verified against a live PG18 cluster.

### E2E tests
- None — there is no user flow beyond opening the screen; covered by unit + integration + manual
  TUI check.

## Agent Verification Plan

**Source:** user-spec "Как проверить" section.

### Verification approach
Automated: `make test` (unit + integration, gracefully skipping unavailable PG versions),
`make build`, `make lint`. Manual: open the screen on a live PG17 and PG18 and confirm the column
set and absolute-vs-delta behaviour, and that `b` appears in the `?` help.

### Per-task verification
| Task | verify: | What to check |
|------|---------|--------------|
| 1 (query layer) | bash | `make test` — bgwriter query unit + integration tests pass (PG14-18 where available) |
| 2 (view + keybinding + help) | user | open `b` screen on PG17/PG18: columns correct, event counters absolute, work columns delta; `b` in `?` help |
| 3 (overview.md) | bash | `overview.md` no longer claims pre-existing bgwriter support; mentions new screen |
| Final (QA) | bash | full `make test` + `make lint` + `make build`; acceptance criteria from user-spec |

### Tools required
`bash` only. No web/MCP tooling — this is a terminal TUI. No post-deploy verification task.

## Backward Compatibility

N/A — adding new code only. The changes are additive: a new query file, a new view map entry, a
new keybinding line, a help-text line, and a documentation correction. No existing public function,
SQL view contract, CLI flag, or config is modified. `NotRecordable` is an existing field (Go
zero-value `false`), so no other view is affected.

**Breaking changes:** no.

## Risks

| Risk | Mitigation |
|------|-----------|
| Unverified external dependency: PG18 `pg_stat_checkpointer` column set — assumed to include `slru_written` (per PostgreSQL docs), must be confirmed before implementation. | Verify on a live PG18 cluster via the integration test / CI matrix (`SELECT ... FROM pg_attribute WHERE attrelid IN ('pg_stat_bgwriter'::regclass,'pg_stat_checkpointer'::regclass)`) before finalizing the PG18 query branch. Do not write the PG18 branch from memory. |
| External `pg_stat_reset_shared(...)` during a session → one-tick negative delta in a diffed column. | Accepted existing pgcenter behaviour (same as `pg_stat_wal`); not fixed in this feature. |
| Screen density: `rstpt_*` add 3 columns on PG17+. | Documented as first-to-drop if the screen proves overcrowded; no action now. |
| `stats_age` from checkpointer does not reflect a separate bgwriter reset on PG17+. | Documented limitation in user-spec; conscious decision. |

## Acceptance Criteria

Технические критерии (дополняют пользовательские из user-spec):

- [ ] `SelectStatBgwriterQuery` returns correct `(query, Ncols, DiffIntvl)` for PG 14/15/16/17/18
      (unit tests green).
- [ ] The bgwriter query executes without error on every available PG 14–18 container
      (integration tests green / skipped only when a version is unavailable).
- [ ] PG18 column set (incl. `slru_written`) verified against a live PG18 cluster.
- [ ] Event counters render as absolute (outside `DiffIntvl`); work/time/buffer columns render as
      per-interval deltas; `stats_age` is pass-through text.
- [ ] `pgcenter record` does not collect the view (`NotRecordable: true` honored by `filterViews`).
- [ ] Hotkey `b` switches to the screen; `b` listed in the `?` help row (`a,b,f,r,w`).
- [ ] `overview.md` corrected.
- [ ] `make test`, `make lint`, `make build` pass; no regressions in existing tests.

## Implementation Tasks

### Wave 1 (независимые)

#### Task 1: bgwriter query layer + tests
- **Description:** Create `internal/query/bgwriter.go` with the per-version query constants and the
  `SelectStatBgwriterQuery(version) (string, int, [2]int)` selector, plus `bgwriter_test.go` with
  unit (per-version `Ncols`/`DiffIntvl`) and integration (execute on live PG 14–18) tests,
  following the `wal.go`/`wal_test.go` pattern. The PG18 branch's exact column set is verified
  against a live PG18 cluster during the integration test.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `make test` (bgwriter unit + integration tests pass)
- **Files to modify:** `internal/query/bgwriter.go` (new), `internal/query/bgwriter_test.go` (new)
- **Files to read:** `internal/query/wal.go`, `internal/query/wal_test.go`, `internal/query/query.go`, `internal/postgres/testing.go`, `internal/stat/postgres.go`

#### Task 2: Correct overview.md
- **Description:** Fix `.claude/skills/project-knowledge/overview.md`, which wrongly lists
  `pg_stat_bgwriter` as already supported; replace it with an accurate entry describing the new
  bgwriter/checkpointer screen. Documentation-only, independent of the code path.
- **Skill:** documentation-writing
- **Reviewers:** dev-code-reviewer
- **Verify:** bash — grep `overview.md`: no stale bgwriter-supported claim; new screen mentioned
- **Files to modify:** `.claude/skills/project-knowledge/overview.md`
- **Files to read:** `docs/features/004-feat-bgwriter-checkpointer/004-feat-bgwriter-checkpointer.md`

### Wave 2 (зависит от Wave 1)

#### Task 3: Register view + TUI wiring
- **Description:** Register the `"bgwriter"` view in `internal/view/view.go` with
  `NotRecordable: true` and add the `case "bgwriter"` branch in `Configure()` calling
  `SelectStatBgwriterQuery`; bind hotkey `b` in `top/keybindings.go`; add `b` to the mode-key help
  row in `top/help.go`. Depends on the selector from Task 1.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer
- **Verify:** user — open `b` on PG17/PG18, confirm columns + absolute/delta behaviour + `?` help
- **Files to modify:** `internal/view/view.go`, `top/keybindings.go`, `top/help.go`
- **Files to read:** `internal/query/bgwriter.go`, `top/config_view.go`, `internal/view/view.go`

### Final Wave

#### Task 4: Pre-deploy QA
- **Description:** Acceptance testing: run `make test` + `make lint` + `make build`, verify the
  acceptance criteria from the user-spec and this tech-spec (column sets per version, absolute vs
  delta, `NotRecordable`, help text, overview.md).
- **Skill:** pre-deploy-qa
- **Reviewers:** none
