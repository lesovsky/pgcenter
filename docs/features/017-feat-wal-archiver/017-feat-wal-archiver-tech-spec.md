---
created: 2026-08-05
status: draft
branch: feature/017-feat-wal-archiver
size: M
---

# Tech Spec: WAL and archiving area — one pass

## Solution

Four independent pieces, all inside the WAL/archiving code area, entered once (roadmap [017]):

1. **New `archiver` view.** A single-row, version-independent screen over `pg_stat_archiver` plus a
   `.ready` backlog count from `pg_ls_archive_statusdir()`. Registered in `view.New()` alongside `wal`
   and `bgwriter`, reached by cycling the existing `w` hotkey and by a new two-item `W` menu. Nothing
   is diffed (`DiffIntvl{0,0}`), so the whole row passes through `calculateDelta` untouched.
2. **record/report support**, which for a pure-SQL view is "do nothing extra" (ADR
   [008] *Lift NotRecordable only*) plus CLI wiring: `-W` changes from `bool` to `string`
   (`-W w` → wal, `-W a` → archiver), mirroring `-J c|t`.
3. **PG 19 `fpi,KiB` on the `wal` screen.** A third branch in `SelectStatWALQuery`, adding
   `round(wal_fpi_bytes / 1024, 2)` right after the `fpi` counter, inside the diffed range.
4. **Verbose-panel backlog moves to a pg_monitor-accessible function.**
   `OverviewArchivingBacklog` switches from `pg_ls_dir('pg_wal/archive_status')` (superuser-only) to
   `pg_ls_archive_statusdir()` (superuser + `pg_monitor`), keeping its output — bytes — identical.

No new package, no new architectural mechanism: every piece copies a precedent that already exists in
the codebase.

## Architecture

### What we're building/modifying

- **`internal/query/archiver.go` (new)** — the `pg_stat_archiver` + backlog query and a
  version-independent selector `SelectStatArchiverQuery(_ int) (string, int, [2]int)`, following the
  `SelectStatIOTimeQuery` form for a selector that keeps its unused `version` parameter.
- **`internal/query/wal.go`** — a PG 19 query constant and a third branch in `SelectStatWALQuery`.
- **`internal/query/overview.go`** — `OverviewArchivingBacklog` switches source function.
- **`internal/view/view.go`** — registers the `archiver` view in `New()` and configures both
  `archiver` and the PG 19 `wal` layout in `Configure()`.
- **`top/`** — the `w` cycle (`config_view.go`), the `W` menu (`menu.go`, `keybindings.go`) and two
  help-screen lines (`help.go`).
- **`cmd/report/report.go`** — `showWAL` becomes a string; `selectReport` maps `w`/`a`.
- **`report/`** — a describe entry and constant for the archiver screen, plus the new `fpi,KiB` row in
  the wal description; two new golden replay tests.

### How it works

Collection is unchanged: the collector runs the current view's query each tick, and because the
`archiver` view declares `DiffIntvl{0,0}`, `calculateDelta` (`internal/stat/postgres.go:589-597`)
short-circuits before `diff()` and hands the raw sample to the renderer. This is what makes the
`'Archiver'` literal in column 0 and the SQL NULLs in the WAL-name/age columns safe without any
`coalesce`.

Navigation reuses the `pg_stat_io` precedent exactly: the `w` key passes a *cycle name* to
`switchViewTo`, which maps it through `walNextView` into `viewSwitchHandler`; the `W` key opens a
two-item menu whose `menuSelect` branch calls `viewSwitchHandler` directly. Both paths print the
target view's static `Msg` exactly once.

Recording needs no recorder change: `filterViews` keeps any view whose `NotRecordable` is false and
whose `MinRequiredVersion` is satisfied, the recorder stores one `PGresult` per view under the view
name, and the report resolves the tar entry prefix from the report type — which equals the view name.

## Decisions

### Decision 1: `archiver` is a separate registered view, not extra columns on `wal`

**Decision:** register a new `archiver` view rather than widening `wal`.
**Rationale:** widening `wal` would change the recorded column layout for `report -W w`, breaking
compatibility with existing archives. A separate view leaves recorded `wal` data untouched. This is
the roadmap's own reasoning and the user-spec locks it.
**Alternatives considered:** extra columns on `wal` — rejected on archive compatibility.

### Decision 2: nothing is diffed — `DiffIntvl{0,0}`

**Decision:** the archiver view declares `DiffIntvl: [2]int{0,0}`; both counters render cumulative.
**Rationale:** during an incident the operator needs the fact of growth and the absolute value for
cross-checking with the PostgreSQL log, not a per-second rate. `calculateDelta` short-circuits on a
`{0,0}` interval and never enters `diff()`, so the `'Archiver'` literal at column 0 is never parsed
and the NULL columns never reach `strconv.ParseInt`. Verified by reading both call sites
(`internal/stat/stat.go:440` for the TUI, `report/report.go:505` for the report).
**Alternatives considered:** placing the counters inside a diffed range (the bgwriter idiom for
work columns) — rejected by the user-spec; a non-empty diffed range purely to avoid diffing column 0
— unnecessary, since `diff()` is not reached at all.

### Decision 3: no `coalesce` on the NULL columns

**Decision:** `last_archived_wal`, `last_failed_wal` and both age columns render blank when the
cluster has never archived.
**Rationale:** `patterns.md` prescribes `coalesce(...,0)` only for **diffed** columns, where an empty
string would abort the sample in `ParseInt`. Nothing here is diffed. A blank is the honest rendering
of "never happened" — the same decision ADR [013] took for `backend_xid`.
**Alternatives considered:** `coalesce(..., '-')` or `0` — rejected as inventing a value.

### Decision 4: privilege failure takes down the whole screen, by design

**Decision:** the archiver query calls `pg_ls_archive_statusdir()` unconditionally; without
`pg_monitor` the whole screen renders the PostgreSQL error and retries next tick.
**Rationale:** the `wal` screen already behaves exactly this way with `pg_ls_waldir()`. More
importantly, hiding the call behind a privilege check **does not work**: PostgreSQL checks EXECUTE at
function-node initialisation, so `CASE` with an uncorrelated subquery (which becomes an InitPlan
evaluated first), `CASE` with a correlated subquery, and `LEFT JOIN LATERAL ... ON
has_function_privilege(...)` all fail alike under a privilege-less role. Measured on a live PG 18
with a purpose-made role.
**Alternatives considered:** two query variants of equal width selected in Go from a connect-time
privilege probe — technically sound but rejected in the user-spec: it would fix half the WAL area for
a role that cannot use the other half anyway, since `wal`'s `pg_ls_waldir()` has no such guard.

### Decision 5: the archive_mode notice is a static `Msg`, not a conditional hint

**Decision:** the view's `Msg` reads `Show archiver statistics (requires archive_mode=on)` and is
printed unconditionally on both entry paths.
**Rationale:** the project's "hint" mechanism is exactly this — `stat_io_time` carries
`(requires track_io_timing=on)` in its `Msg` (`internal/view/view.go:188`) and no GUC is read anywhere
in the codebase. A conditional variant would need the `archive_mode` GUC threaded through
`SelectCommonProperties` → `PostgresProperties` → `Scan` → consumer, plus a branch on both entry
points, on a cmdline surface that already carries two registered debt items about lost messages
([027], [028]).
**Alternatives considered:** a live-GUC cmdline hint; an `archive_mode` column in the query. Both
rejected in the user-spec — the first as fragile, the second as a redundant column.

### Decision 6: the `w` cycle reuses the string `"wal"` as its group name

**Decision:** the `w` binding keeps passing `"wal"` to `switchViewTo`, and a new `case "wal":`
dispatches it through `walNextView`.
**Rationale:** every other cycle uses a group name that is not a view name (`statio`, `databases`,
`progress`, `statements`), but here the group's natural name *is* the first view's name, and the view
cannot be renamed — it is the report type and the tar entry prefix. Only one dispatch site is
affected, the menu path bypasses `switchViewTo` entirely, and the existing
`Test_switchViewTo` expectation `sizes → wal → wal` still holds because the cycle's default arm
returns `"wal"`.
**Alternatives considered:** a distinct group name (`walgroup`) — rejected as an entity the user never
sees.

### Decision 7: `-W` becomes a string flag — an accepted breaking change

**Decision:** `showWAL` changes from `bool` to `string`; `-W w` selects wal, `-W a` selects archiver.
**Rationale:** one hotkey group, one report flag — the `-J c|t` shape blessed by ADR [008]. The
roadmap owner accepted the breakage knowingly after being told that `-J` was born a string and is
therefore not a precedent for breaking an existing flag.
**Alternatives considered:** a new free short letter (no breakage); cobra `NoOptDefVal="w"` (bare `-W`
keeps working, but `-W a` with a space silently fails). Both rejected by the roadmap owner.

### Decision 8: the verbose backlog moves off `pg_ls_dir` — superseding ADR [010]'s function choice

**Decision:** `OverviewArchivingBacklog` counts `.ready` entries via `pg_ls_archive_statusdir()`
instead of `pg_ls_dir('pg_wal/archive_status')`. Output stays bytes
(`count(.ready) × wal_segment_size`), and the `n/a` degradation path is untouched.
**Rationale:** ADR [010] assumed `pg_monitor` was sufficient for this aggregate. It is not —
measured on live PG 14 and PG 18: `pg_ls_dir` has ACL `{postgres=X/postgres}` (superuser only), while
`pg_ls_waldir` and `pg_ls_archive_statusdir` are `{postgres, pg_monitor}`. So the most common
monitoring role sees `n/a` and never gets the first signal that archiving has stopped. The roadmap's
one-pass mandate makes this the right moment to fix it.
**Rejected cost, accepted knowingly:** `pg_ls_dir` returns names only, `pg_ls_archive_statusdir`
returns `SETOF record` and stats every file, and the verbose panel rides every screen — so the
lstat walk is paid on all screens, not only on `archiver`.
**Alternatives considered:** leaving the panel alone and correcting the user-spec's framing —
rejected by the roadmap owner in favour of fixing the signal.

### Decision 9: no throttling of the directory listing

**Decision:** the `.ready` count runs every tick, in the TUI and in `pgcenter record`, with no cache
and no latency guard.
**Rationale:** decided by the roadmap owner. The exposure is genuine — the screen is opened exactly
when the directory is largest — but it is the same class and roughly the same cardinality as the
`pg_ls_waldir()` call the `wal` screen already makes in the same incident (both are `SETOF record`,
verified), since an unarchived segment produces one file in each directory.
**Follow-up, not a blocker:** the stand run measures the real cost on ~200 000 `.ready` files,
including the view-switch latency caused by the unbuffered `viewCh`. If the numbers are bad,
throttling returns as its own decision, reusing the [010] `latencyGuardThreshold` machinery.

### Decision 10 (Autopilot assumption): `MinRequiredVersion` is `PostgresV14`

**Autopilot assumption:** the user-spec says the archiver view needs no version gate beyond the
project's own floor, without naming a constant. Assumed `query.PostgresV14`, because that is the
floor every recent view uses (`wal`, `bgwriter`, `replslots`), it is the oldest cluster in the test
image, and it keeps `TestView_VersionOK`'s ≤PG13 rows untouched. `pg_stat_archiver` (PG 9.0+) and
`pg_ls_archive_statusdir()` (PG 12+) both predate it, so nothing is actually gated out.

### Decision 11 (Autopilot assumption): column header names

**Autopilot assumption:** the user-spec fixes the column order and semantics but writes headers in
prose. Assumed SQL aliases exactly as the user-spec's mock-up prints them: `source`, `ready`,
`archived`, `last_archived`, `archived_age`, `failed`, `last_failed`, `failed_age`, `stats_age`, and
`fpi,KiB` on the wal screen. The comma-unit form (`fpi,KiB`) matches the existing `wal,KiB`
convention; `stats_age` matches every other screen.

## Data Models

No database schema, no Go types added. Two SQL shapes:

**`archiver` view — 9 columns, single row, nothing diffed:**

| # | alias | source | notes |
|---|---|---|---|
| 0 | `source` | literal `'Archiver'` | row identity, `UniqueKey=0`, frozen column |
| 1 | `ready` | `count(*) FILTER (WHERE name LIKE '%.ready')` over `pg_ls_archive_statusdir()` | privileged |
| 2 | `archived` | `archived_count` | cumulative |
| 3 | `last_archived` | `last_archived_wal` | NULL → blank |
| 4 | `archived_age` | `date_trunc('seconds', now() - last_archived_time)::text` | NULL → blank |
| 5 | `failed` | `failed_count` | cumulative |
| 6 | `last_failed` | `last_failed_wal` | NULL → blank |
| 7 | `failed_age` | `date_trunc('seconds', now() - last_failed_time)::text` | NULL → blank |
| 8 | `stats_age` | `date_trunc('seconds', now() - stats_reset)::text` | never NULL |

`Ncols: 9`, `DiffIntvl: [2]int{0,0}`, `OrderKey: 0`, `UniqueKey: 0`, `NotRecordable: false`.

**`wal` view on PG 19 — 8 columns:** `source`, `waldir_size`, `wal,KiB`, `records`, `fpi`,
`fpi,KiB`, `buffers_full`, `stats_age`; `Ncols: 8`, `DiffIntvl: [2]int{2,6}` (0-based, from `wal,KiB`
through `buffers_full`). PG 14–17 and PG 18 branches are untouched.

## Dependencies

### New packages

None.

### Using existing (from project)

- `internal/query` — selector pattern, `query.Format`, `PostgresV14`/`PostgresV19` constants.
- `internal/view` — view registration and `Configure`.
- `internal/stat` — `NewPGresultQuery`, `calculateDelta` (pass-through path).
- `top/` — `viewSwitchHandler`, `switchViewTo`, menu machinery, help template.
- `record`/`report` — `filterViews`, the recorder loop, `describeReport`, golden-replay test shape
  (ADR [008]).

## Testing Strategy

**Feature size:** M

### Unit tests

- `SelectStatArchiverQuery` returns the same query, `Ncols=9` and `DiffIntvl{0,0}` for every version
  in the PG 14–19 matrix (a version-independent selector still gets the matrix, per `io.go`'s
  precedent).
- `SelectStatWALQuery` returns the PG 19 branch with `Ncols=8`/`DiffIntvl{2,6}` at 190000 and the
  unchanged PG 18 and PG 14 branches below it — the existing 190000 row must change, not be added.
- `walNextView` maps `wal → archiver`, `archiver → wal`, and anything else → `wal`.
- View registration: the archiver entry's `MinRequiredVersion`, `Ncols`, `DiffIntvl`, `OrderKey`,
  `UniqueKey`, `NotRecordable` and the `archive_mode` substring in its `Msg`.
- Count-based tests updated in lockstep: total view count, per-version availability, `filterViews`
  keep/drop counts, menu item counts, and the `switchViewTo` transition table extended with the two
  new cycle transitions.
- `selectReport` maps `-W w` → `wal`, `-W a` → `archiver`, and an unknown value → `""`.
- The describe map returns the new archiver description.

### Integration tests

Yes — the established `*_test.go` form that executes each query against live clusters PG 14–19 from
the test image, with `t.Skipf` on unavailable versions. They prove the archiver query is valid on
every supported version and returns 9 columns, and that the PG 19 wal query is valid. They **cannot**
prove archiving behaviour: the fixtures run `archive_mode=off` with no `archive_command`, so the row
is all zeros and NULLs. That is what the stand run is for.

### E2E tests

No new harness. The end-to-end role is played by the golden replay tests (synthetic in-memory tar →
`report`, ADR [008]) and by the manual stand run:

- `report_record_archiver_test.go` + one golden — the screen is version-independent.
- `report_record_wal_test.go` + goldens at PG 18 and PG 19 — the screen is version-aware and its
  layout is changing; it has no replay coverage today.

## Agent Verification Plan

**Source:** user-spec "Как проверить".

### Verification approach

Everything except live archiving behaviour is verified by the agent inside the project CI image,
which carries PG 14–19 fixtures. Live archiving states, TUI navigation and the cost measurement are
verified by driving the TUI over ssh/tmux on the stand.

### Per-task verification

| Task | verify: | What to check |
|------|---------|--------------|
| 1 | bash | `go test ./internal/query/...` in the CI image — archiver selector + live query on PG 14–19 |
| 2 | bash | `go test ./internal/view/... ./record/...` — registration and filterViews counts |
| 3 | bash | `go test ./top/...` — cycle, menu, help; runs without PostgreSQL |
| 4 | bash | `go test ./cmd/report/... ./report/...` — flag mapping and describe |
| 5 | bash | `go test ./report/...` — golden replay for archiver and wal PG18/PG19 |
| 6 | bash | `go test ./internal/query/... ./internal/stat/...` — verbose backlog query on PG 14–19 |
| 7 | bash | full `make test` in the CI image + `make lint` + `make vuln` on the host |
| 8 | user | stand run: archiving states, navigation, narrow terminal, cost measurement |

### Tools required

`docker` (project CI image `lesovsky/pgcenter-testing:0.0.11` for the PG 14–19 fixtures), `bash`,
`ssh` + `tmux` for the stand. No MCP tooling.

## Backward Compatibility

**Breaking changes:** yes — one, in the CLI.

`pgcenter report -W` changes from a boolean flag to a string flag. Two observable failure shapes for
existing invocations:

- `pgcenter report -W` with `-W` as the last token → cobra: `flag needs an argument: 'W' in -W`.
- `pgcenter report -W -f dump.tar` (the common legacy shape) → pflag consumes `-f` as the flag's
  value, `selectReport` returns `""`, and the command exits with `report type is not specified, quit`.

Both exit non-zero; neither silently changes meaning. The release notes must describe the second
shape, because that is what users will actually hit.

**Migration strategy:** none beyond documentation — the roadmap owner rejected both a deprecation
period and a `NoOptDefVal` compatibility shim. Flag help text, README and the 0.12.0 release notes are
updated in the same feature.

**DB migration compatibility:** N/A — pgcenter has no schema of its own here.

**Consumer impact:**

- `showWAL` has exactly four references in the tree (`cmd/report/report.go` field, flag definition,
  `selectReport` case, and one test case) — all four change together.
- Recorded archives: unaffected for PG 14–18, because the report picks the layout from each sample's
  recorded PostgreSQL version.
- **One known incompatibility:** a `wal` recording made **on PG 19** by a pre-0.12 pgcenter has 7
  columns but will be replayed against the new 8-column PG 19 layout, so `stats_age` falls inside
  `DiffIntvl{2,6}` and `report -W w` fails with `diff failed`. Narrow (PG 19 is still beta) and
  accepted in the user-spec; it goes to the release notes rather than being fixed by
  column-count-aware replay.

## Risks

| Risk | Mitigation |
|------|-----------|
| `wal_fpi_bytes` is verified against PG 19 **beta2**; the name or semantics may move at beta3/RC | Name taken from the live catalog, not release notes. Blast radius is one query line plus one golden file; the other three pieces do not depend on PG 19 at all and the FPI piece can be detached and shipped at RC/GA. Roadmap finalization already carries a re-verification item, and tech-debt [017] tracks the beta channel. |
| The unthrottled listing runs on a cluster already in trouble, and the verbose panel now pays an lstat walk on every screen | Accepted by the roadmap owner. The stand run measures the query on ~200 000 `.ready` files, the view-switch latency, and the verbose-panel cost against a `master`-built binary. Bad numbers reopen throttling as its own decision. |
| A slow collector query blocks the next view switch (unbuffered `viewCh`) | Pre-existing structural property, equally true of the `wal` screen. Documented in the user-spec, measured on the stand; not fixed here. |
| The breaking `-W` change hits scripts, and the common legacy form fails with a message that does not explain why | Release notes describe that exact form; flag help text and README updated. |
| Integration tests cannot exercise archiving at all (fixtures run `archive_mode=off`) | Behavioural gate is the stand run with `/bin/true` then `/bin/false`. The test image is deliberately not changed — that would need an image bump and would alter what every existing test sees. |
| The stand's TTL is 24h from 2026-08-05 and manual QA comes last | The archiver screen is exercisable as soon as Wave 2 lands, before record/report and FPI. If the stand expires, a new one must be requested — the pipeline does not silently skip the manual gate. |

## Acceptance Criteria

Technical criteria, complementing the user-facing ones in the user-spec:

- [ ] `go test -race -p 1 ./...` is green inside the CI image with PG 14–19 fixtures up, with zero
      race reports.
- [ ] `make lint` (golangci-lint + gosec) and `make vuln` are clean on the host.
- [ ] No existing test's expectations are weakened to make the suite pass: every count-based test
      (`TestNew`, `TestView_VersionOK`, `Test_filterViews`, `Test_selectMenuStyle`,
      `Test_switchViewTo`) is updated to a new **correct** number, not deleted or loosened.
- [ ] The archiver query executes on every version PG 14–19 and returns exactly 9 columns.
- [ ] The PG 19 wal query executes on PG 19 and returns exactly 8 columns; the PG 14–17 and PG 18
      branches return their current column counts unchanged.
- [ ] `SelectStatArchiverQuery` is version-independent — its unused parameter is named `_`, per the
      project's revive settings.
- [ ] The archiver view sets `NotRecordable: false` and requires no change in `record/record.go`.
- [ ] Golden replay tests exist for archiver (one golden) and for wal at PG 18 and PG 19, and they
      fail if the corresponding layout changes.
- [ ] Column-name-driven assertions: tests reference columns by header name where the layout is
      pinned, so a future column insertion cannot silently shift an index.

## Implementation Tasks

### Wave 1 (независимые — разные файлы, ничего общего)

#### Task 1: Archiver query and selector
- **Description:** Add `internal/query/archiver.go` with the 9-column `pg_stat_archiver` + `.ready`
  backlog query and a version-independent selector returning query, `Ncols` and `DiffIntvl`. This is
  the data source for the whole feature; no view wiring here.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/query/...` in the CI image, query executes on PG 14–19
- **Files to modify:** `internal/query/archiver.go`, `internal/query/archiver_test.go`
- **Files to read:** `internal/query/wal.go`, `internal/query/io.go`, `internal/query/wal_test.go`,
  `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-code-research.md`

#### Task 2: PG 19 FPI column on the wal screen
- **Description:** Add the PG 19 branch to `internal/query/wal.go` with the `fpi,KiB` column placed
  after the `fpi` counter and inside the diffed range, and update the wal selector tests including the
  existing PG 19 row. Keeps PG 14–18 byte-identical.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/query/...` in the CI image, PG 19 query returns 8 columns
- **Files to modify:** `internal/query/wal.go`, `internal/query/wal_test.go`
- **Files to read:** `internal/query/bgwriter.go`, `internal/query/query.go`

#### Task 3: Verbose panel backlog on a pg_monitor-accessible function
- **Description:** Switch `OverviewArchivingBacklog` from `pg_ls_dir('pg_wal/archive_status')` to
  `pg_ls_archive_statusdir()` so roles with `pg_monitor` see the backlog instead of `n/a`. Output
  stays bytes and the degradation path is unchanged.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/query/... ./internal/stat/...`; aggregate runs under a
  `pg_monitor` role
- **Files to modify:** `internal/query/overview.go`, `internal/query/overview_test.go`
- **Files to read:** `internal/stat/postgres.go`, `docs/decisions-log.md`

#### Task 4: report CLI — `-W` becomes a string flag
- **Description:** Change `showWAL` from bool to string, map `w` → wal and `a` → archiver in
  `selectReport`, and update the flag description. An unknown value must fall through to the existing
  "report type is not specified" path.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./cmd/report/...`; `-W w`, `-W a`, `-W x` behave as specified
- **Files to modify:** `cmd/report/report.go`, `cmd/report/report_test.go`
- **Files to read:** `report/report.go`

### Wave 2 (зависит от Wave 1 — регистрация вью и всё, что от неё зависит)

#### Task 5: Register the archiver view and configure both layouts
- **Description:** Register the `archiver` view in `view.New()` with its static parameters and `Msg`,
  wire both `archiver` and the PG 19 `wal` layout into `Configure()`, and update every count-based
  view and record test to its new correct value.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/view/... ./record/...`
- **Files to modify:** `internal/view/view.go`, `internal/view/view_test.go`, `record/record_test.go`
- **Files to read:** `internal/query/archiver.go`, `internal/query/wal.go`, `record/record.go`

### Wave 3 (зависит от Wave 2)

#### Task 6: TUI navigation — `w` cycle, `W` menu, help
- **Description:** Add the `walNextView` cycle and its dispatch case, the two-item `W` menu with its
  keybinding, and the two help-screen lines. Copies the `j`/`J` machinery for `pg_stat_io`.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./top/...` (runs without PostgreSQL)
- **Files to modify:** `top/config_view.go`, `top/menu.go`, `top/keybindings.go`, `top/help.go`,
  `top/config_view_test.go`, `top/menu_test.go`
- **Files to read:** `internal/view/view.go`

#### Task 7: report describe text for archiver and the wal FPI row
- **Description:** Add the archiver description constant and its entry in the describe map, and add
  the `fpi,KiB` row to the wal description so `report -d -W w` documents the PG 19 column.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./report/...`; `report -d -W a` and `-d -W w` print the new text
- **Files to modify:** `report/describe.go`, `report/report.go`, `report/report_test.go`
- **Files to read:** `internal/query/archiver.go`, `internal/query/wal.go`

#### Task 8: Golden replay tests for archiver and wal
- **Description:** Add replay coverage built from synthetic in-memory tars: one golden for the
  version-independent archiver screen, and PG 18 + PG 19 goldens for the version-aware wal screen,
  which has no replay coverage today and whose layout this feature changes.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./report/...`; goldens fail when the layout is perturbed
- **Files to modify:** `report/report_record_archiver_test.go`, `report/report_record_wal_test.go`,
  `report/testdata/`
- **Files to read:** `report/report_record_bgwriter_test.go`, `report/report_record_statio_test.go`

#### Task 9: User-facing documentation
- **Description:** Update the README's `-W` flag description and add the 0.12.0 release-notes entries
  for the breaking flag change (including the legacy-invocation failure shape) and the PG 19 archive
  replay limitation.
- **Skill:** documentation-writing
- **Reviewers:** dev-code-reviewer
- **Verify:** bash — grep the README and release notes for the new flag form
- **Files to modify:** `README.md`, `docs/roadmap-0.12.0.md`
- **Files to read:** `docs/features/017-feat-wal-archiver/017-feat-wal-archiver.md`

### Final Wave

#### Task 10: Pre-deploy QA
- **Description:** Acceptance testing: full `make test` on the PG 14–19 fixtures inside the CI image,
  `make lint` and `make vuln`, then verification of every acceptance criterion from the user-spec and
  this tech-spec, including the manual stand run (archiving states, navigation, narrow terminal, and
  the cost measurement on a large `.ready` directory).
- **Skill:** pre-deploy-qa
- **Reviewers:** none
