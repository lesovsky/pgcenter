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
- **`top/`** — the `w` cycle (`config_view.go`), the `W` menu (`menu.go`, `keybindings.go`) and three
  help-screen lines (`help.go`): the `w` entry moves out of the plain-switch line into its own
  `w,W` line, and the `Q`-does-not-reset caveat gains `archiver`.
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
and the NULL columns never reach `strconv.ParseInt`. Verified by reading both call sites: the TUI calls `calculateDelta` directly
(`internal/stat/stat.go:441`), and the report reaches it through the `stat.Compare` wrapper
(`report/report.go:505` → `internal/stat/postgres.go:575-577`).
**Alternatives considered:** placing the counters inside a diffed range (the bgwriter idiom for
work columns) — rejected by the user-spec; a non-empty diffed range purely to avoid diffing column 0
— unnecessary, since `diff()` is not reached at all.

### Decision 3: no `coalesce` on the NULL columns

**Decision:** `last_archived_wal`, `last_failed_wal` and both age columns render blank when the
cluster has never archived.
**Rationale:** `patterns.md` prescribes `coalesce(...,0)` only for **diffed** columns, where an empty
string would abort the sample in `ParseInt`. Nothing here is diffed. A blank is the honest rendering
of "never happened". ADR [013] is the nearest precedent: it kept `backend_xid` a raw column
precisely because blank-versus-set is the information, rather than synthesising a value.
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
including the view-switch latency caused by the unbuffered `viewCh`.
**The measurement must be taken under the right role, or it measures the wrong baseline.** For a
superuser the verbose panel already pays one `stat`-walk of comparable cardinality every tick
(`OverviewWalSize` calls `pg_ls_waldir()`), so Decision 8 takes it from one walk to two. But for a
role holding only `pg_monitor` — precisely the role Decision 8 exists to serve — today's aggregate
fails instantly with 42501 and costs **nothing**; afterwards it pays the full walk. So the honest
comparison is zero → full, not 1× → 2×, and a measurement run as `postgres` would miss it entirely.
The stand run therefore fixes its conditions: a `pg_monitor`-only role, verbose mode on, and a
concurrent `pgcenter record` (the recorder runs every registered view's query every tick regardless
of which screen is displayed).
**Outcome agreed in advance, so the measurement cannot end in a shrug:** if the numbers are bad, the
throttle is applied using the machinery that already exists for exactly this
(`verboseCollectState` + `latencyGuardThreshold`, today used only for the DB-size aggregate); if they
are acceptable, the remainder is written into the tech-debt register rather than left implicit.
**Alternatives considered:** a cached value behind a latency guard, reusing [010]'s
`latencyGuardThreshold` + `dbSizeThrottled` machinery — rejected for now by the roadmap owner as
complexity ahead of evidence, and explicitly re-openable if the stand numbers are bad.

### Decision 10: `MinRequiredVersion: PostgresV14` is mandatory, not cosmetic

**Decision:** the archiver view sets `MinRequiredVersion: query.PostgresV14`.
**Rationale:** the user-spec's phrasing ("no minimum version beyond the common one") does not survive
contact with the code — **there is no common floor.** The view registry still serves versions down to
PG 9.4 and `TestViews_Configure` exercises `90400`. Left at its zero value, the view would be offered
on PG ≤11, where `pg_ls_archive_statusdir()` does not exist (PG 12+): the screen would error every
tick, and worse, `pgcenter record` would abort the **entire recording** on the first query error
(`record/recorder.go:136-139`). `PostgresV14` is also the floor every recent view uses (`wal`,
`bgwriter`, `replslots`) and the oldest cluster in the test image, so it keeps `TestView_VersionOK`'s
≤PG13 rows untouched.
**Alternatives considered:** `PostgresV12` (the true function floor) — rejected: pgcenter does not
test below PG 14, so it would promise a version nobody verifies.

### Decision 11: `pg_ls_archive_statusdir()` tolerates a missing directory — `n/a` becomes `0`

**Decision:** accept the behavioural difference introduced by Decision 8 and record it.
**What changes:** `pg_ls_dir('pg_wal/archive_status')` is `missing_ok=false`, while
`pg_ls_archive_statusdir()` is `missing_ok=true`. Proven by moving `$PGDATA/pg_wal/archive_status`
aside on a live PG 18.4: the old query errors, the new one returns `0`. So on a cluster whose
`archive_status` directory is gone, the verbose panel flips from `n/a` to a confident `0 B`.
**Rationale:** a missing `archive_status` means a damaged or hand-edited data directory — a state in
which the backlog number is the least of the operator's problems, and one that no supported
PostgreSQL configuration produces on its own. Weighed against the gain (the most common monitoring
role finally sees the backlog at all), the trade is worth it.
**Alternatives considered:** wrapping the call to distinguish "empty" from "absent" — rejected: it
cannot be done in SQL without another privileged call, and it would re-introduce the degradation
machinery Decision 4 already ruled out.

### Decision 12: the first recorded sample is not printed — pre-existing, now documented

**Decision:** `report -W a` over an N-tick recording prints N−1 rows, and this is accepted as-is.
**Rationale:** `report/report.go:315-320` discards the first sample unconditionally, because for a
diffed screen the first sample has no predecessor to diff against. The `archiver` screen is pure
pass-through, so its first sample *is* printable data and is nonetheless dropped. This is not
introduced here — every `DiffIntvl{0,0}` screen behaves this way today, `activity` included — and
changing it would alter shared report behaviour and every existing golden.
**Consequence carried:** the user-spec's scenario 3 originally said "one row per tick"; it has been
corrected to "one row per tick, except the first".
**Alternatives considered:** skipping the discard when `DiffIntvl == {0,0}` — rejected as
out-of-scope shared-path surgery with golden churn across unrelated screens.

### Decision 13 (Autopilot assumption): the documentation target is `doc/release-notes/v0.12.0.md`

**Decision:** the breaking change is documented in a new `doc/release-notes/v0.12.0.md`, plus the
flag's own help string in `cmd/report/report.go` (what `pgcenter report --help` prints). The
user-spec's "update the README" criterion is dropped.
**Rationale:** there is nothing to update — `-W` is documented nowhere in the tree, and
`doc/pgcenter-report-readme.md` carries no flag reference at all, not for `-W` and not for `-J`.
`doc/release-notes/` is the project's established home for exactly this kind of note: `v0.9.0.md`
documents a hotkey change (`g`/`G`) in prose, which is the same class of user-visible break. The
directory has been unused since v0.9.0, so this revives a convention rather than inventing one.
The file must quote the literal messages users will hit: `report type is not specified, quit` for a
legacy `-W -f …` invocation, and `diff failed` for the PG 19 legacy-archive case.
**Alternatives considered:** `docs/roadmap-0.12.0.md` — rejected, it is a planning document that gets
archived when the release ships, and its own Finalization section says the project keeps no CHANGELOG
because GoReleaser generates GitHub release notes from commits (which is exactly why a prose note for
a breaking change needs its own home). Writing full CLI flag documentation for the `doc/` tree —
rejected as new, unestimated scope of a different kind; the omission is recorded here so it stays
visible.

### Decision 14 (Autopilot assumption): column header names

**Decision:** the SQL aliases are exactly the headers the user-spec's mock-up prints — `source`,
`ready`, `archived`, `last_archived`, `archived_age`, `failed`, `last_failed`, `failed_age`,
`stats_age` — plus `fpi,KiB` on the wal screen.
**Rationale (autopilot assumption):** the user-spec fixes column order and semantics but writes the
headers in prose rather than naming aliases. The mock-up is the most direct reading of intent, the
comma-unit form matches the existing `wal,KiB`, and `stats_age` matches every other screen.
**Alternatives considered:** longer, more explicit names (`last_archived_wal`, `ready_files`) —
rejected: they widen a screen that already carries two 24-character WAL names, and `ready` was chosen
over `ready_files`/`backlog` by the roadmap owner during the interview.

### Decision 15: an archive with no archiver data prints nothing, and that stays

**Decision:** `report -W a` over an archive containing no `archiver` entries prints an empty output —
no rows and no header — and exits 0.
**Rationale:** the user-spec originally promised "header only", which the code contradicts:
`printStatHeader` returns early unless the view has been aligned, and alignment happens inside the
data branch, so with no samples nothing is printed at all. A "no data" notice exists for exactly one
screen, `procpidstat` (`report/report.go:391`), and giving `archiver` one would introduce behaviour no
other screen has. The user-spec's edge case and acceptance criterion were corrected to match the code.
**Alternatives considered:** emitting an INFO line for empty archiver reports — rejected as
inconsistent with every other screen and outside this feature's mandate; making it consistent for all
screens is its own change.

### Decision 16: the two WAL-name columns need no escape sanitisation

**Decision:** `last_archived_wal` and `last_failed_wal` are rendered as-is, with no escaping, and the
query carries a comment saying why.
**Rationale:** they are server-supplied text reaching the terminal, which is the shape of tech-debt
item [029] — but PostgreSQL only ever reports a name that passed its own `VALID_XFN_CHARS` filter
(hex digits plus the `.history`/`.backup`/`.partial` suffixes) before recording it in the archiver
statistics. That character set contains no ESC and no control characters, so these two columns cannot
carry a terminal escape sequence even if an operator hand-places a bogus `.ready` file. Debt [029] is
therefore not widened here.
**Alternatives considered:** sanitising the two columns defensively — rejected: it would add a
transformation on a value that is already constrained at the source, and would diverge from every
other text column on every other screen, none of which sanitise.

### Decision 17: the `-W` mapping is a closed whitelist

**Decision:** `selectReport` maps only `w` and `a`; every other value falls through and the command
exits with "report type is not specified, quit".
**Rationale:** `ReportType` is not an inert label — it is the tar-entry filter in `isFilenameOK` and
the key into the view map. An unmapped value leaking through would select a zero-value `view.View`
and produce a silently empty report rather than an error, which is the worst outcome for a
report tool: a clean exit that shows nothing. The existing `-D`/`-J` flags already fail closed; this
keeps the family consistent. Tests cover other flags' letters (`c`, `t`, `g`) explicitly, not just an
arbitrary unknown value.
**Alternatives considered:** defaulting an unrecognised value to `wal` — rejected: it would silently
run a different report than the operator asked for.

### Decision 18: the SET ROLE test roles are created at test time, idempotently

**Decision:** the privilege tests create their own roles at runtime on whichever fixture cluster the
test connects to, tolerate a role that already exists, and always `RESET ROLE` afterwards.
**Rationale:** the test image is deliberately not changed (Risks), so the roles cannot be baked into
the fixtures, and the tree today contains no `CREATE ROLE`/`GRANT` at all — this is a new pattern, so
it needs stating rather than assuming. The tests run against six clusters and may run repeatedly, so
creation must be idempotent and the session must not leak an assumed role into later assertions.
**Alternatives considered:** baking the roles into the test image — rejected, it would need an image
bump and change what every existing test sees; skipping the privilege tests and relying on the stand
— rejected, that is exactly the gap that let the wrong `pg_ls_dir` privilege assumption survive.

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
- **Privilege behaviour is tested, not asserted.** Using `SET ROLE` against the fixture cluster: the
  archiver query succeeds and returns 9 columns under a role holding only `pg_monitor`, and fails with
  a permission error under a role holding neither. The same two-direction check covers the verbose
  backlog aggregate, whose whole justification is that `pg_monitor` could not run the old one. This
  closes the gap that the fixture superuser role would otherwise hide — it is precisely why the
  current `pg_ls_dir` query passes today. The roles are created by the tests themselves, idempotently,
  with `RESET ROLE` afterwards (Decision 18) — the test image stays frozen.

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
| 1 | bash | `go test ./internal/query/...` in the CI image — archiver selector + live query returns 9 columns on PG 14–19 |
| 2 | bash | `go test ./internal/query/...` in the CI image — PG 19 wal query returns 8 columns; PG 14–18 counts unchanged |
| 3 | bash | `go test ./internal/query/... ./internal/stat/...` — verbose backlog aggregate runs under a `pg_monitor` role |
| 4 | bash | `go test ./cmd/report/...` — `-W w`, `-W a`, `-W x` map as specified |
| 5 | bash | `go test ./internal/view/... ./record/...` — registration, availability and filterViews counts |
| 6 | bash | `go test ./top/...` — cycle, menu, help; runs without PostgreSQL |
| 7 | bash | `go test ./report/...` — describe text for archiver and the wal FPI row |
| 8 | bash | `go test ./report/...` — golden replay for archiver and for wal at PG 18 and PG 19 |
| 9 | bash | grep the release notes for both literal messages (`report type is not specified, quit`, `diff failed`) |
| 10a | bash | full `make test` in the CI image + `make lint` + `make vuln` on the host |
| 10b | user | stand run: archiving states, navigation, narrow terminal, cost measurement |

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
period and a `NoOptDefVal` compatibility shim. The flag's help string and a new
`doc/release-notes/v0.12.0.md` are written in the same feature; there is no README text to update
because the report command's flags are documented nowhere (Decision 13).

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
| The breaking `-W` change hits scripts, and the common legacy form fails with a message that does not explain why | `doc/release-notes/v0.12.0.md` quotes that exact failure text, and the flag's help string is updated. No README change — the flags are undocumented there today (Decision 13). |
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
- [ ] A test proves the archiver query succeeds under a `pg_monitor`-only role and fails without it,
      and that the verbose backlog aggregate returns a number under the same `pg_monitor`-only role.
      Running as the fixture superuser only would hide exactly the defect piece 4 exists to fix.
- [ ] The two comments in `internal/stat/` that state `pg_ls_dir` requires "pg_monitor/superuser" are
      corrected — they are the reason the wrong privilege assumption survived into ADR [010].

## Implementation Tasks

### Wave 1 (независимые — разные файлы, ничего общего)

#### Task 1: Archiver query and selector
- **Description:** Add `internal/query/archiver.go` with the 9-column `pg_stat_archiver` + `.ready`
  backlog query and a version-independent selector returning query, `Ncols` and `DiffIntvl`. This is
  the data source for the whole feature; no view wiring here. The privilege behaviour the design rests
  on is proven by test, both directions, using roles created at test time per Decision 18 — not
  asserted in prose. The query carries a
  comment pointing at Decision 16, which is why its two server-supplied text columns need no
  sanitisation.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/query/...` in the CI image: the query returns 9 columns on
  PG 14–19, succeeds under a `pg_monitor`-only role and fails with a permission error without it
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
  stays bytes and the degradation path is unchanged. Two existing comments assert the wrong privilege
  requirement for the old function and justify swallowing the error text with an argument that no
  longer applies; correct both in the same task, since they are what a future reader would trust.
  **Only the comments and the function change — the degrade-to-`n/a` behaviour itself is untouched.**
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/query/... ./internal/stat/...`; the aggregate returns a number
  under a role holding only `pg_monitor`
- **Files to modify:** `internal/query/overview.go`, `internal/query/overview_test.go`,
  `internal/stat/postgres.go`, `internal/stat/postgres_test.go`
- **Files to read:** `docs/decisions-log.md`

#### Task 4: report CLI — `-W` becomes a string flag
- **Description:** Change `showWAL` from bool to string, map `w` → wal and `a` → archiver in
  `selectReport`, and update the flag description. The mapping is a closed whitelist that fails
  closed on any unmapped value, for the reason recorded in Decision 17.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./cmd/report/...`; `-W w`, `-W a` map correctly and every unmapped value
  (including other flags' letters, e.g. `c`, `t`, `g`) fails closed
- **Files to modify:** `cmd/report/report.go`, `cmd/report/report_test.go`
- **Files to read:** `report/report.go`

### Wave 2 (зависит от Wave 1 — регистрация вью и всё, что от неё зависит)

#### Task 5: Register the archiver view and update every layout-pinning test
- **Description:** Register the `archiver` view in `view.New()` with its static parameters and `Msg`,
  and add its `case` to `Configure()`. Update the view-count, per-version availability and
  record-filter counts, and add a per-view guard test pinning the new view's parameters, following
  the existing guard tests for the `stat_io` and `bgwriter` views. The `wal` case in `Configure()`
  needs no edit — it already delegates to the selector.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/view/... ./record/...`
- **Files to modify:** `internal/view/view.go`, `internal/view/view_test.go`, `record/record_test.go`
- **Files to read:** `internal/query/archiver.go`, `internal/query/wal.go`, `record/record.go`

### Wave 3 (зависит от Wave 2)

#### Task 6: TUI navigation — `w` cycle, `W` menu, help
- **Description:** Add the `walNextView` cycle and its dispatch case, the two-item `W` menu with its
  keybinding, and the three help-screen lines, copying the `j`/`J` machinery for `pg_stat_io`. The
  help screen is the project's only user-facing hotkey documentation and is pinned by test rather
  than by review, so the new `w,W` entry and the `archiver` addition to the `Q` line are pinned the
  same way, following the existing entry tests.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./top/...` (runs without PostgreSQL)
- **Files to modify:** `top/config_view.go`, `top/menu.go`, `top/keybindings.go`, `top/help.go`,
  `top/config_view_test.go`, `top/menu_test.go`, `top/help_test.go`
- **Files to read:** `internal/view/view.go`

#### Task 7: report describe text for archiver and the wal FPI row
- **Description:** Add the archiver description constant and its entry in the describe map, and add
  the `fpi,KiB` row to the wal description so `report -d -W w` documents the PG 19 column. The
  describe text is a single static constant per report type with no version awareness — it already
  documents `write`/`sync`, removed in PG 18 — so the new row will also be printed for PG 14–18
  archives. That is the existing contract, not a regression to fix here.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./report/...`; `report -d -W a` and `-d -W w` print the new text
- **Files to modify:** `report/describe.go`, `report/report.go`, `report/report_test.go`
- **Files to read:** `internal/query/archiver.go`, `internal/query/wal.go`

#### Task 8: Golden replay tests for archiver and wal
- **Description:** Add replay coverage built from synthetic in-memory tars: one golden for the
  version-independent archiver screen, and PG 18 + PG 19 goldens for the version-aware wal screen,
  which has no replay coverage today and whose layout this feature changes.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./report/...`; goldens fail when the layout is perturbed
- **Files to modify:** `report/report_record_archiver_test.go`, `report/report_record_wal_test.go`,
  `report/testdata/report_record_archiver.golden`, `report/testdata/report_record_wal_pg18.golden`,
  `report/testdata/report_record_wal_pg19.golden`
- **Files to read:** `report/report_record_bgwriter_test.go`, `report/report_record_statio_test.go`

#### Task 9: User-facing documentation
- **Description:** Add the 0.12.0 release-notes entries for the breaking `-W` change and the PG 19
  legacy-archive limitation, quoting the literal messages users will see (`report type is not
  specified, quit` and `diff failed`), and note that on a cluster whose `archive_status` directory is
  missing the verbose panel now reports `0 B` instead of `n/a`. Target, scope and what is deliberately
  left out are fixed by Decision 13.
- **Skill:** documentation-writing
- **Reviewers:** dev-code-reviewer
- **Verify:** bash — grep the release notes for both literal messages
- **Files to modify:** `doc/release-notes/v0.12.0.md`
- **Files to read:** `doc/release-notes/v0.9.0.md`,
  `docs/features/017-feat-wal-archiver/017-feat-wal-archiver.md`

### Final Wave

#### Task 10: Pre-deploy QA
- **Description:** Acceptance testing: full `make test` on the PG 14–19 fixtures inside the CI image,
  `make lint` and `make vuln`, then verification of every acceptance criterion from the user-spec and
  this tech-spec. Includes the manual stand run — archiving states, navigation, narrow terminal — and
  the cost measurement, taken under the conditions fixed in Decision 9.
- **Skill:** pre-deploy-qa
- **Reviewers:** none
