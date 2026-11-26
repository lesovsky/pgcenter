---
created: 2026-08-10
status: approved
branch: feature/018-feat-tables-autovacuum-area
size: L
---

# Tech Spec: Tables and autovacuum area — one pass

## Solution

Three changes, all inside established patterns of this codebase:

1. **A new registered view `autovacuum_scores`** over `pg_stat_autovacuum_scores` (PG 19+), with a
   new query file `internal/query/autovacuum_scores.go`, `NotRecordable: true`,
   `MinRequiredVersion: query.PostgresV19`, `DiffIntvl: [2]int{0,0}`, `OrderKey: 1` (score),
   `OrderDesc: true`, `Ncols: 11`.

2. **A `stats_age` tail column on `tables` and `indexes`** via two new version selectors,
   `SelectStatTablesQuery` / `SelectStatIndexesQuery`, wired through `view.Configure()`. Both views
   have no selector at all today — this is the "promote from static to version-aware" step, the
   `wal.go` shape.

3. **A version-aware `t`/`T` hotkey group.** This is the only genuinely new pattern: no cycle and no
   menu in the codebase knows the PostgreSQL version today. `t` cycles `tables ↔ autovacuum_scores`
   on PG 19 and behaves exactly as today below it; `T` opens a two-item menu whose second item
   carries a `— requires PostgreSQL 19` suffix and refuses selection below PG 19.

The critical open question from the user-spec phase — whether a recorded PG 18 → PG 19 boundary can
feed `diff()` a narrow previous snapshot against a wide current one — was traced end to end and
answered: **no**. `report/report.go:276-320` computes
`versionChanged := prevStat.Valid && prevMeta.version != d.meta.version` and, on that branch, sets
`prevStat = d.res` and `continue`s. The first sample of the new version replaces the previous
snapshot and is never diffed. No crash, no design work needed.

A first draft of this spec claimed that `tables`/`indexes` become the first diffed screens with a
version-dependent width, and that tech debt **[020]** therefore becomes newly reachable. **That is
false** and was corrected in review: `wal` already returns 8/7/11 columns with `DiffIntvl`
`{2,6}`/`{2,5}`/`{2,9}`, `bgwriter` 14/13/12, and `databases_general` 19/18 — all diffed, all
recordable. [020]'s reachability is unchanged by this feature and its note needs no edit on that
account.

The true, narrower statement is about **test coverage**: the three existing version-change replay
tests are all `activity`, which runs `DiffIntvl{0,0}`, so the replay test this feature adds is the
first to drive a *diffed* screen through the version-change branch. That is a gap worth closing, not
a defect being introduced.

## Architecture

### What we're building/modifying

- **`internal/query/autovacuum_scores.go`** (new) — one query constant plus
  `SelectStatAutovacuumScoresQuery(version int) (string, int, [2]int)`. Left outer join from
  `pg_stat_autovacuum_scores` to `pg_stat_all_tables` for `dead_total`; a `{{if ne .ViewType "all"}}`
  block carrying the schema filter and the `for_wraparound` escape hatch.
- **`internal/query/tables.go` / `indexes.go`** — a second constant each (`…PG19`) with the
  `stats_age` tail column, plus a 3-tuple selector.
- **`internal/view/view.go`** — one new registry entry; three new `Configure()` cases
  (`tables`, `indexes`, `autovacuum_scores`).
- **`top/config_view.go`** — `tablesNextView(current string, version int)`, a `case "tables"` in
  `switchViewTo`, and the `toggleSysTables` wiring (four literals).
- **`top/menu.go`** — `menuTables`, a version parameter reaching `selectMenuStyle`, and a refusal
  branch in `menuSelect`.
- **`top/keybindings.go`** — one new row for `'T'`; the `'t'` row is unchanged in bytes and changed
  in meaning.
- **`top/help.go`** — the `t,T` row; `'t' tables,` must be removed from the existing `s,t,i` line.
- **`record/recorder_test.go`** — the unfiltered-view-map trap; fixed in the same task that registers
  the view, because that registration is what breaks it.
- **`record/record.go`** — the in-code comment claiming no production view sets `NotRecordable` any
  more, plus the deterministic named live-TUI-only INFO from Decision 11.
- **`report/describe.go`** — `stats_age` lines for the two screens.
- **`report/report.go`** — recorded column-layout boundary detection and state reset from Decision 9.
- **`report/report_test.go`** — the replay harness generalized from `activity` to any screen name,
  including same-version layout changes in both directions.

### How it works

Unchanged data flow — `query → stat → view → gocui`. The only new edge is the version number
reaching two places it did not reach before: `tablesNextView` (via `switchViewTo`, which already
receives the whole `app` and reads `app.postgresProps.ExtPGSSSchema` from it, so
`app.postgresProps.VersionNum` is in hand with no new plumbing) and the menu construction.

The `,` toggle reaches the new screen through the same `queryOptions.ViewType` field the other three
screens use; the difference is that `pg_stat_autovacuum_scores` has no `user`/`sys`/`all` name
variants, so instead of splicing a relation name the template switches a whole `WHERE` clause on
and off.

## Decisions

### Decision 1: `stats_age` appended at the tail, so `DiffIntvl` does not become version-dependent
**Decision:** append `stats_age` as the last column on both screens. `tables` goes 19 → 20 columns
with `DiffIntvl` staying `[2]int{1,18}`; `indexes` goes 6 → 7 with `[2]int{1,5}` unchanged.
**Rationale:** ADR [012-feat-pg19-compatibility-baseline] ("Progress screens: new columns
 mid-layout, version-aware DiffIntvl") chose the opposite for the progress screens — inserting new columns
mid-layout for readability and paying with a version-dependent `DiffIntvl` — and recorded the hazard
plainly: a stale interval on the newer layout lands on columns whose values still parse as numbers,
so the wrong diff succeeds silently and prints plausible nonsense. Here the readability argument does
not apply: all seven existing `stats_age` columns in the codebase are last, so the tail is where a
reader already looks for it. Taking the tail keeps the interval literal valid on both versions and
avoids that entire class.
**This decision is also a defence-in-depth memory-safety boundary, not a complete replay fix.**

`record -a` can append pre-feature and feature samples while both declare PG 19. With tail placement,
the highest diff index remains valid against the narrower previous row (`18` against 19 columns for
`tables`, `5` against 6 for `indexes`); mid-layout insertion would remove that margin. Assert
**`DiffIntvl[1] < min(Ncols)` across every version** of both screens.

Staying in bounds is not sufficient: the replay renderer also retains `Aligned`/`ColsWidth` across a
same-server-version width change. Decision 9 therefore makes any column-layout change a replay boundary;
no incompatible pair reaches diff or stale rendering merely because the arithmetic happens to fit.

**Alternatives considered:** mid-layout insertion next to the other timestamps (rejected — buys
nothing, costs a version-dependent interval, and removes the defensive bounds margin); no selector at
all, letting `Ncols` stay static (rejected — see Decision 5); relying on tail placement alone for replay
safety (rejected — it does not reset renderer/layout state).

### Decision 2: 3-tuple selectors, `UniqueKey` deliberately excluded
**Decision:** `SelectStatTablesQuery(version) (string, int, [2]int)` and the same for `indexes` and
`autovacuum_scores`. `Configure()` patches exactly `QueryTmpl`, `Ncols`, `DiffIntvl` — not
`UniqueKey`, not `OrderKey`, not `Cols`/`ColsWidth`.
**Rationale:** the 4-tuple form exists in exactly one place, `SelectStatStatementsJITQuery`, and only
because that screen's `UniqueKey` points at a *trailing* md5 `queryid` whose index moves with
`Ncols`. Here row identity is column 0 on every layout, so `UniqueKey` stays 0. `DiffIntvl` is still
returned although constant, matching `SelectStatArchiverQuery` and
`SelectStatReplicationSlotsQuery`, which return version-independent values for signature symmetry —
and the constancy is the point of Decision 1, so it should be visible at the call site.
**Alternatives considered:** a 2-tuple for the two screens whose interval never changes (rejected —
ADR [012-feat-pg19-compatibility-baseline] already rejected sibling selectors differing for a reason invisible at the call site).

### Decision 3: `ORDER BY` on the unrounded score, by qualified reference
**Decision:** the query ends `ORDER BY s.score DESC`, referencing the input column, while the SELECT
list emits `round(s.score::numeric, 2) AS score`.
**Rationale:** measured, not reasoned. PostgreSQL resolves a **bare** `ORDER BY` identifier against
the output column list first, so `ORDER BY score DESC` would sort by the *rounded* value — precisely
the tie set the ordering exists to break. Demonstrated on three tables whose true scores are
0.00299850, 0.00199900 and 0.00099950 and which all round to `0.00`: the bare form returned them in
physical order, the qualified form in true-value order. A qualified reference is always an input
column. Downstream, `PGresult.sort` re-sorts client-side on the *rendered* `"0.00"` with
`sort.SliceStable`, so it finds exact ties and preserves the SQL order inside them — which is what
makes the SQL order load-bearing rather than cosmetic.
**Alternatives considered:** ordering by the output alias, as
`internal/query/replication_slots.go:31` does (rejected — there the alias *is* the value; here it is
a lossy rendering of it); no SQL `ORDER BY` at all (rejected — leaves a hundred rows in planner
order).

### Decision 4: the schema filter reads `schemaname` from the outer relation
**Decision:** `pg_stat_autovacuum_scores` is the left side of the join, and the `WHERE` clause
references *its* `schemaname`, never the joined `pg_stat_all_tables`.
**Rationale:** taking `schemaname` from the joined side would make an unmatched row yield
`schemaname IS NULL`; `NOT IN (…)` then evaluates to NULL and the row is filtered out. The outer
join would silently do the opposite of what it is there for — dropping exactly the rows it exists to
preserve. The scores view carries `relid`, `schemaname` and `relname` itself, so nothing is lost.
**Alternatives considered:** none — the alternative is a defect.

### Decision 5: the `Configure()` cases for `tables`/`indexes` are load-bearing, not drift guards
**Decision:** add `case "tables"` and `case "indexes"` to `view.Configure()`, and record in-code that
they are required, not cosmetic.
**Rationale:** the `archiver` case is documented in-code as a pure drift guard, because `New()`
already seeds the same values — and the same reading would be wrong here. Without the case,
`view.Ncols` stays 19/6 while the PG 19 query returns 20/7 columns. The render path is unaffected
(`visibleColumns` reads `s.Result.Ncols`), but `orderKeyRight` wraps on `config.view.Ncols`, so the
`Right` arrow could never select the new last column and `orderKeyLeft` would treat index 18/5 as
"the last one". A silent, render-invisible defect that no existing test sees. The static `New()`
entries must keep the PG ≤ 18 values, because `report.newApp` seeds from a raw `view.New()` and only
`processData` calls `Configure`.
**Alternatives considered:** relying on the static entry (rejected — produces the defect above).

### Decision 6: version-aware navigation, confined to the `t` group — and it is *not* the first of its kind
**Decision:** `tablesNextView(current string, version int)` and a version parameter reaching
`selectMenuStyle`; no other cycle or menu is touched. The gap in the other two cycles is registered
as tech debt rather than fixed here.

**Rationale.** The honest starting point, corrected during architecture review: this shape already
exists in the codebase **twice**, and both times it was left version-blind.

| cycle | member requiring a newer server | cycle function |
|---|---|---|
| `x` — `statementsNextView` | `statements_jit`, `MinRequiredVersion: PostgresV15` (`view.go:241`) | `config_view.go:313` — takes `current string` only |
| `p` — `progressNextView` | `progress_copy`, `MinRequiredVersion: PostgresV14` (`view.go:315`) | `config_view.go:338` — takes `current string` only |

`VersionOK` is consulted in exactly two places, `record/record.go:214` and
`internal/stat/stat.go:317` — never in a cycle — so on a server below the member's minimum both
cycles walk the operator onto a screen that renders `ERROR: selected statistics is not supported by
current version of Postgres` in place of the whole table, every tick. So the claim that the existing
precedents "do not cover this case" is wrong: they cover it and answer it the other way.

What justifies answering differently here is **the size of the affected version span**, not novelty.
Active support is PG 14–19. `progress_copy` is unavailable only below PG 14, i.e. on EOL servers
alone; `statements_jit` is unavailable on exactly one supported version, PG 14. `autovacuum_scores`
would be unavailable on **five of the six supported versions**. The second difference is authorship:
`t` has no cycle today, so a version-blind cycle here is a defect this feature *introduces*, whereas
the other two are inherited.

**Why not retrofit `p` and `x` in the same pass.** Their gap fires on EOL servers and on one
supported version, they are not the area this feature entered, and generalizing would put three
cycles and two more menus into a feature that already carries the only new pattern in it. That is the
opposite of the release's "enter each code area exactly once" principle — the `p`/`x` area is not
this feature's area. Registered in the debt register so it is not rediscovered as a surprise.

**Alternatives considered:** leaving `t` version-blind for consistency with `p`/`x` (rejected — it
makes the reflex key useless on five of six supported versions, and consistency with a known defect
is not a virtue); fixing all three cycles here (rejected — see above); hiding the menu item below
PG 19 (rejected during the user-spec interview — explicit beats implicit).

### Decision 7: `menuSelect`'s cmdline write moves inside each branch
**Decision:** when adding the refusal branch, push the shared
`printCmdline(app.ui, "%s", app.config.view.Msg)` call into each arm rather than leaving it after the
inner switch.
**Rationale:** the refusal arm must print its own message. Leaving the shared call in place would put
two `printCmdline` calls on one code path, and `patterns.md` records that each write goes through its
own `g.Update` goroutine with no ordering guarantee, so one of the two intermittently never renders.
Two known defects in the codebase are exactly this shape.
**Alternatives considered:** returning early from the refusal arm before the shared call (rejected —
works, but leaves the trap armed for the next person adding an arm).

### Decision 8: tech debt [035] and [029] are recorded, not fixed; [020] needs no edit
**Decision:** extend the scope notes of [035] and [029]; leave [020] alone; change no shared TUI
alignment/rendering or cell-sanitisation code.
**Rationale:** [035] now also affects `stats_age`, which is empty on entry for most clusters — so the column
freezes at header width and truncates once values pass a day. The fix lives in the shared alignment
path and would change behaviour on every screen, which the roadmap owner explicitly refused.
[029]'s existing unsanitised server-to-terminal weakness gains one reachable surface through
`relation`; the feature deliberately records that exposure rather than expanding into a shared TUI and
report sanitiser. [020] gets no edit: its reachability is **not** widened by this feature, because
diffed screens with version-dependent widths already exist (`wal`, `bgwriter`, `databases_general`).
Decision 9 handles valid adjacent layouts, not attacker-declared malformed `Ncols`.
**Alternatives considered:** fixing [035] or [029] here (rejected — shared behaviour outside this area
pass); amending [020] (rejected — the valid-layout boundary does not close or widen the malformed-shape
class); silently inheriting the new terminal surface without a debt update (rejected).

### Decision 9: any recorded column-layout change is a replay boundary
**Decision:** in `report.processData`, compare the previous and incoming result column names/layout in
addition to recorded server version. When either changes, replace `prevStat`/`prevMeta`, skip diffing
that first sample, rerun `Configure`, clear `Aligned` and stale `Cols`/`ColsWidth`, reset header cadence,
and restore/re-resolve the requested sort key exactly as for a version change. This applies in both
width-change directions and to blank as well as populated `stats_age`.
**Rationale:** a same-PG19 archive appended across the pgcenter upgrade is ordinary valid input, but the
existing version-only branch reuses stale renderer state and can omit the new column or fail on zero
width. Treating layout identity as the boundary prevents both cross-layout deltas and stale output.
**Alternatives considered:** reject mixed pgcenter layouts with a validation error and release-note
workaround (rejected while a deterministic replay boundary is available); keep version-only reset plus
the `DiffIntvl` bounds assertion (rejected — confirmed not to cover rendering).

### Decision 10: the wraparound fixture runs only in a dedicated single-use PG 19 cluster
**Decision:** the targeted integration test is skipped in the ordinary shared suite unless the external
harness explicitly supplies a dedicated-cluster marker and expected server identity. The entire targeted
`go test` process runs inside the dedicated disposable container/network namespace, so the existing
connector's `127.0.0.1:21919` is necessarily the single-use PG 19 and no shared fixture is reachable at
that endpoint. Immediately before any mutation, the test fails closed unless `pg_control_system()`'s
`system_identifier` equals the harness-supplied value for this container. No test-only endpoint override
is added. An external supervisor owns the cluster lifetime, runs no concurrent package against it, and
destroys the container after completion, timeout, or test-process SIGKILL; in-test cleanup is never the
last line of defence. The test still captures/restores and verifies the original autovacuum setting,
registers cleanup immediately, removes feature-owned probe/procedure objects, starts idempotently,
identifies the toast relation through `reltoastrelid`, and asserts a safe margin below the global freeze
threshold before burning XIDs.
**Rationale:** `t.Cleanup` alone cannot recover from runner loss, and package-parallel tests share port
21919. External cluster ownership makes the irreversible XID and persistent-configuration operations
safe; targeted PASS evidence is mandatory and an ordinary-suite skip is not a pass.
**Alternatives considered:** cross-package locking and shared-cluster execution (rejected — still not
interruption-safe); manual-only verification (rejected — the discriminating integration assertion must
remain executable and mutation-checked).

### Decision 11: recording exclusions must not lie about version support
**Decision:** keep version/extension filtering aggregate, but emit a deterministic separate INFO for each
deliberate `NotRecordable` exclusion. On PG 19 the sanitized line for this feature contains
`autovacuum_scores`, `not recordable`, and `live TUI only`; it must not attribute that exclusion to the
server version. Recording selection rules are unchanged. Add PG 19 setup/CLI evidence for both the named
reason and absence of a false version-unsupported explanation.
**Rationale:** on an otherwise fully available PG 19 setup, one shared "unsupported by version" counter
would misdescribe the intentionally unsettled recording format, while silence would hide the exclusion.
**Alternatives considered:** one generic reworded aggregate without view names (rejected — does not tell
the operator what was omitted); separate public counters/messages for every version/extension reason
(rejected as more surface than needed); leave the misleading message (rejected).

### Decision 12: acceptance evidence is synthetic, minimised, and sanitized
**Decision:** use a newly created no-secret database and synthetic workload. Keep raw full archives and
terminal/session transcripts outside the repository with restrictive local permissions and trap-based
deletion. Commit only required sanitized report output, tar member listings, and hashes; redact and
secret-scan commands, environment output, SQL text, hostnames, and captures before publication.
`--strlimit` is not a redactor, and credential values are prohibited in evidence.
**Rationale:** full recordings may contain activity/statements query text and client metadata unrelated
to the tables/indexes assertion.
**Alternatives considered:** retaining complete raw archives as feature evidence (rejected — unnecessary
sensitive-data exposure); relying on fixture convention without an explicit gate (rejected).

## Data Models

No database schema, no new Go types beyond the query constants and selector functions.

`view.View` fields set for the new screen:

| field | value |
|---|---|
| `Name` | `"autovacuum_scores"` |
| `Msg` | `"Show autovacuum scores"` — **not optional**: this is the string both entry paths print, all 28 existing views set it, and an empty one passes the whole automated suite and surfaces only on the stand run |
| `QueryTmpl` | `query.PgStatAutovacuumScoresDefault` |
| `Ncols` | `11` |
| `DiffIntvl` | `[2]int{0,0}` |
| `OrderKey` | `1` (score) |
| `OrderDesc` | `true` |
| `UniqueKey` | `0` (default — relation) |
| `NotRecordable` | `true` |
| `MinRequiredVersion` | `query.PostgresV19` |
| `ColsWidth` | `map[int]int{}` (non-nil — two writers mutate it in place; `alignViewToResult` replaces the map before the first frame, so a nil value is a narrower hazard than "any column-width change", but the registry invariant is pinned for every view and this one is no exception) |
| `Filters` | `map[int]*regexp.Regexp{}` (same) |

Column layout, in order: `relation`, `score`, `do_vacuum`, `do_analyze`, `for_wraparound`,
`dead_total`, `xid_score`, `mxid_score`, `vacuum_score`, `vacuum_insert_score`, `analyze_score`.

### Value contract of the new query

Each of these is a user-spec acceptance criterion, and none of them is observable from the column
list alone — an implementation can produce all eleven columns in the right order and still get every
one of these wrong.

| column(s) | contract |
|---|---|
| `relation` | `schemaname \|\| '.' \|\| relname`, both taken from the **outer** relation, matching the `tables` idiom |
| all **six** scores — `score`, `xid_score`, `mxid_score`, `vacuum_score`, `vacuum_insert_score`, `analyze_score` | `round(…::numeric, 2)`, in SQL, not in Go. Decision 3 names the rounding only for `score` because that is where it interacts with `ORDER BY`; the other five round identically |
| `do_vacuum`, `do_analyze`, `for_wraparound` | `::text`, yielding `true`/`false` — the `replslots.active` idiom, not `t`/`f` |
| `dead_total` | `n_dead_tup` from `pg_stat_all_tables`, joined on `relid`, rendered as-is with no unit suffix |
| join | `LEFT JOIN` with `pg_stat_autovacuum_scores` on the **outer** side; the `WHERE` clause's `schemaname` also comes from the outer side (Decision 4) |
| ordering | `ORDER BY` the **unrounded** score by qualified reference (Decision 3) |

Selector return values:

| version | `tables` | `indexes` | `autovacuum_scores` |
|---|---|---|---|
| `>= PostgresV19` | `PgStatTablesPG19`, 20, `{1,18}` | `PgStatIndexesPG19`, 7, `{1,5}` | `PgStatAutovacuumScoresDefault`, 11, `{0,0}` |
| below | `PgStatTablesDefault`, 19, `{1,18}` | `PgStatIndexesDefault`, 6, `{1,5}` | same — see below |

`SelectStatAutovacuumScoresQuery` has **one branch, not two**: the screen is gated by
`MinRequiredVersion` before its query is ever run, so there is no older layout to return. It returns
the single constant unconditionally and keeps the `version` parameter for signature symmetry with the
rest of the selector family — exactly what `SelectStatArchiverQuery(_ int)` and
`SelectStatReplicationSlotsQuery(_ int)` already do. Leaving the below-19 return undefined would be a
gap; returning a fabricated older layout would be worse.

## Dependencies

### New packages
None.

### Using existing (from project)
- `internal/query` — `Format` (plain `text/template`, no `Funcs()`, so `ne`/`if` builtins are
  available. `activity.go` and `procpidstat.go` already use `{{if}}` for `ShowNoIdle`; `ne` itself is
  new to the package, though it is a plain text/template builtin needing no registration).
- `internal/view` — registry and `Configure()`.
- `internal/postgres/testing.go` — `NewTestConnectVersion` for the PG 19 fixture.
- `top/` — cycle, menu, keybinding and help machinery established by the `w`/`W` group.

## Test Data and Identifier Map

| Concern | Contract / Evidence |
|---------|---------------------|
| Relation identity | `relid` is internal join evidence only and is not displayed. Public row identity is `schemaname.relname` from the outer scores view; `UniqueKey` remains column 0. |
| Score values | PostgreSQL exposes raw `double precision` scores. SQL renders all six as `round(...::numeric, 2)`; default ordering uses qualified unrounded `s.score`, while stable client-side sorting preserves that order for rounded ties. |
| `dead_total` evidence | Absolute `n_dead_tup` comes from the outer join to `pg_stat_all_tables` on `relid`; a missing partner renders NULL/blank and must not drop the scores row. |
| `stats_reset` evidence | PG 19 `stats_reset` comes from the `pg_stat_*` side of each relation query. NULL renders blank; a populated value is truncated to seconds and lies outside `DiffIntvl`. |
| Wraparound fixture | The targeted test runs wholly inside the dedicated disposable container/network namespace; fixed `127.0.0.1:21919` cannot reach the shared fixture. Marker plus expected `system_identifier` are mandatory and checked fail-closed immediately before mutation. An external supervisor destroys the container after test completion/timeout/SIGKILL; no concurrent packages share it. The probe uses `reltoastrelid`, feature-owned idempotent objects, freeze-age margin, and config/object cleanup. Ordinary-suite skip is not PASS. |
| Privilege fixtures | `SetupTestRole` creates named NOLOGIN roles with and without `pg_monitor`; callers prove the restricted session and always `RESET ROLE`. The scores query is expected to remain readable in both sessions. |
| Replay fixtures | Synthetic `tables` and `indexes` archives contain at least two ticks on each side of recorded versions 180000 and 190000, plus same-190000 old/new pgcenter layouts in both directions × blank/populated `stats_age`, always with `-o stats_age`. New→old must restore seed/default sort after the requested column disappears; old→new resolves its new index. Filenames and report time bounds bracket every sample. |
| Acceptance reset | SC-1 uses a fresh no-secret PG 19 container and chosen active/untouched relations. DB `stats_reset` and clock readings immediately around recording define expected age bounds with ±1 second truncation tolerance; shared integration tests do not perform this reset. |
| Evidence handling | Decision 12: synthetic no-secret workload; raw full archives/transcripts stay outside Git and are deleted; only minimized sanitized output, member listings, and hashes survive after redaction/secret-scan. |
| PostgreSQL fixtures | Active clusters use ports 21914–21919, database `pgcenter_fixtures`, user `postgres`. Unavailable clusters skip only where the established fixture contract permits it; an unmapped version is a failure. |
| Manual terminal fixtures | PG 19 and PG 18 stand runs use deterministic tmux geometry (190×52, plus a narrow pass where relevant) and a second `master` binary for A/B classification. |
| External providers | None. There are no provider DTOs, host credentials, asynchronous provider states, or polling contracts beyond bounded PostgreSQL/TUI refresh observation. |

## Testing Strategy

**Feature size:** L

### Unit tests
- Selector branch coverage for all three selectors across the version list, including boundary and
  out-of-range probes (`0`, `180000`, `190000`, `200000`), in the `archiver_test.go` shape.
- Locked column-name list for the new query — the guard against PG 19 catalog drift in beta3/RC.
  Assert names and order, outer `FROM pg_stat_autovacuum_scores`, all six `round(...::numeric, 2)`
  expressions, all three boolean `::text` casts, outer `schemaname||'.'||relname`, `LEFT JOIN
  pg_stat_all_tables` on `relid`, outer `s.schemaname` in the filter, and qualified `ORDER BY s.score
  DESC`. Mutations to INNER JOIN, joined-side schema, rounded alias ordering, or dropped casts must
  redden the corresponding named assertion.
- `stats_age` is the last column on both PG 19 layouts. The load-bearing assertion is the **column
  order** combined with `DiffIntvl[1] == Ncols-2`; asserting `DiffIntvl == {1,18}` alone passes even
  if `stats_age` were inserted mid-layout, so it proves nothing on its own.
- The bounds invariant from Decision 1: `DiffIntvl[1] < min(Ncols)` over every version of both
  screens. This is defence in depth only; Decision 9 owns successful same-version layout replay.
- `stats_age` is sourced from the `pg_stat_*` half of the join, not `pg_statio_*`. Both halves carry
  the column on PG 19 and the wrong one returns a plausible value, so this needs an assertion on the
  query text — nothing else would ever catch it.
- `tablesNextView(current, version)` table test.
- Menu: item count, the version-dependent suffix, and a refusal row that asserts nothing is pushed
  on `viewCh` and the view is unchanged.
- Keybinding registration for `'T'` and `'t'` via `keybindingsList` — the only seam that makes
  *which handler a key carries* assertable.
- Registry invariants for the new view (`key == v.Name`, non-nil maps, `NotRecordable`,
  `MinRequiredVersion`, `Ncols`, `DiffIntvl`, `OrderKey`, `OrderDesc`, **and `Msg` pinned word for
  word** — `view_test.go` has no blanket `Msg` check, and the archiver entry is the precedent for
  pinning it on a single view).
- `calculateDelta` short-circuit: a `PGresult` with an empty `dead_total` cell through
  `DiffIntvl{0,0}` produces no error. **Must be shown to fail** when the interval is changed to a
  non-zero pair, otherwise it proves nothing about the screen. The same test covers the empty
  `stats_age` cell, which reaches the render path by the same route.
- The menu refusal test proves no `viewCh` send, unchanged current view, and closed menu. Exact notice
  text and exactly-one-write behaviour are code-review plus manual SC-7 checks because `printCmdline`
  through `g.Update` is not observable in this unit seam; do not claim a composed-line unit assertion.

### Integration tests
- `tables`/`indexes` query execution against PG 14–19 fixtures, rewritten from the current
  `conn.Exec`-and-discard tier to the `FieldDescriptions()` tier so column counts are actually
  observable. While rewriting, fix two pre-existing defects in place: the missing `defer
  conn.Close()` and the legacy 12-version list where six versions always skip (tech debt [024]).
- The new query against PG 19, asserting the 11 column names in order. Port the measured three-relation
  rounded-tie fixture (raw scores approximately 0.00299850, 0.00199900, and 0.00099950, all rendered
  `0.00`) and assert the exact raw-score descending row order; replacing qualified `s.score` with the
  rounded output alias must redden that order assertion. Unmatched LEFT JOIN behavior is pinned by the
  structural assertions rather than assumed to occur naturally in a live catalog.
- PG 19 recording filter/setup: `autovacuum_scores` is absent while deterministic sanitized INFO contains
  `autovacuum_scores`, `not recordable`, and `live TUI only`, with no false version-unsupported reason.
- **The `for_wraparound` escape hatch** follows Decision 10. The flagged relation must be in `pg_toast`
  and identified through the probe's `reltoastrelid`; user mode must return that row, and deleting `OR
  s.for_wraparound` must redden exactly that assertion rather than fixture setup. The entire test process
  runs inside the dedicated container/network namespace and verifies marker plus expected
  `system_identifier` before mutation; fixed port 21919 therefore cannot address the shared fixture.
  The external supervisor destroys the container after completion, timeout, or test SIGKILL. Within that
  disposable cluster the fixture uses the measured toast reloption and committed
  XID burn, checks the global freeze-age safety margin, restores/verifies the prior autovacuum setting,
  and cleans feature-owned objects idempotently. Ordinary shared-suite skip is not PASS evidence.
- **Privilege behaviour of the new view, in both directions**, following `archiver_test.go`: run the
  query through `SetupTestRole(…, true)` and `SetupTestRole(…, false)`. Its name must match the
  `AutovacuumScores` filter that Task 1's verify uses, or it silently never runs — `go test -run` is
  case-sensitive and a filter matching nothing looks exactly like a clean pass. The measurement that the view
  needs no privileges was taken on beta2; the locked column-name test is blind to an ACL change,
  which is the likeliest axis to move for a brand-new per-relation view between beta and GA. Every
  other test connects as `postgres` over `trust` and would never notice.
- The PG 18 → PG 19 replay of **both** `tables` and `indexes` — the user-spec names both, and both
  change width. Requires `buildActivityTar` and `activityReplayConfig` to take a screen-name
  parameter; once parameterized, the second screen is one more table row. Two ticks per version are
  mandatory — the first of each version is consumed by the version-change branch. This is the first
  time a diffed screen goes through that branch; the three existing version-change tests are all
  `activity` with `DiffIntvl{0,0}`.
- Same-recorded-version 190000 replay for both screens, both width-change directions, and both blank and
  populated `stats_age` in every direction, per Decision 9. Use at least two ticks per layout and `-o
  stats_age`: old→new resolves the requested column at its new index; new→old removes it and restores the
  view's seed/default sort. For every boundary, the first new-layout sample is not diffed against the old
  one, the second same-layout sample yields correct metric deltas, widths/headers recompute, blank ages
  stay blank, and populated ages appear only in new-layout segments.

### Acceptance and E2E tests
SC-1–SC-4 and SC-10 run as black-box CLI and regression scenarios in the feature acceptance harness. Interactive
TUI behaviour in SC-5–SC-8 and the documentation review in SC-9 remain manual; the terminal scenarios
use a second binary built from `master` for A/B.
Unit and integration tests support these contracts but are not presented as external acceptance evidence.

## Acceptance Design

| SC | AC | Type | Setup/provider/fixture checkpoints | Expected base checkpoint | Final observable assertion |
|----|----|------|------------------------------------|--------------------------|----------------------------|
| SC-1 | AC-21, AC-22, AC-26–AC-28, AC-31 | new | Fresh no-secret PG 19 acceptance container; chosen active/untouched relations; capture `stats_reset` and DB clock bounds immediately before/after recording; at least three ticks; sanitized record stdout | `pg19_stats_age_report_unavailable` | Active-row ages meet DB-derived bounds ±1 second and untouched rows are blank; tar omits the view; INFO contains `autovacuum_scores`, `not recordable`, `live TUI only` and no false version reason |
| SC-2 | AC-33 | regression | Synthetic archives for both screens, two ticks per recorded version, valid time bounds and metadata | N/A — must pass before and after the feature | Replay preserves each version's headers, widths, deltas, and requested sort column |
| SC-3 | AC-36 | new | Build the current pgcenter binary | `relation_stats_age_descriptions_missing` | `report -d -T` and `report -d -I` include trailing `stats_age` descriptions marked PG 19+ |
| SC-4 | AC-23, AC-29, AC-30 | regression | Identical PG 17/18 archives plus `master` and feature binaries; structural query/Ncols assertions remain separate | N/A — must pass before and after the feature | `report -T`, `-I`, `-F`, and `-S` output is byte-identical between binaries for each archive |
| SC-5 | AC-1–AC-6, AC-13–AC-17, AC-35 | new, manual | PG 19 stand with representative relations, deterministic terminal, feature and `master` binaries | `autovacuum_screen_unavailable` | Both `t` and `T` paths show the contracted screen once, with correct observable values/order/help and working frozen-column scrolling |
| SC-6 | AC-10, AC-11 | new, manual | PG 19 system-schema wraparound fixture, stable for the observation window | `wraparound_exception_unavailable` | The flagged system row is visible in user mode; ordinary system relations still follow the `,` toggle |
| SC-7 | AC-15, AC-18–AC-20 | new + regression, manual | PG 18 stand and a `master` comparison binary | `version_aware_tables_menu_unavailable` | `t` preserves legacy behaviour; the marked menu item refuses selection with exactly one explanatory message |
| SC-8 | AC-21, AC-22, AC-26–AC-28 | new, manual | PG 19 stand with chosen active/untouched relations and direct `stats_reset`/DB-clock evidence | `pg19_stats_age_display_unavailable` | Active values match DB-derived ages within one refresh ±1 second and grow between captures; untouched relations remain blank on both screens |
| SC-9 | AC-37–AC-40 | new, manual documentation review | Final feature behaviour and all five affected documentation/register files | `feature_documentation_missing` | Feature catalog, both Project Knowledge references, release notes, and debts [035]/[029] match the shipped behaviour |
| SC-10 | AC-41 | new | For each screen: both directions × blank/populated `stats_age`, two ticks per layout, `-o stats_age` | `same_pg19_layout_change_unsupported` | First new-layout sample is a no-diff boundary and second yields metric deltas; old→new resolves the new sort index and shows ages, new→old hides ages and restores seed/default sort; blank stays blank and populated ages occur only in new-layout segments |

Only SC-1–SC-4 and SC-10 enter `acceptance/scenarios/` and the freeze manifest. SC-5–SC-9 require terminal,
timing, or documentation judgement and remain owner checklist items. AC-32 is an independent mandatory
quality gate: full tests, build, E2E, lint, vulnerability checks, zero races, and separate targeted PASS
evidence from the dedicated wraparound cluster; a shared-suite skip is not a pass. No failed, unobserved,
or tooling-error result may be relabelled as a pass.

## Verification Environment

The automated gate is the project CI image with live PG 14–19 fixtures. Required commands are
`make test`, `make lint`, `make vuln`, `make build`, and `testing/e2e.sh`, plus the frozen acceptance
runner. Host-only runs with skipped PostgreSQL tests are compile checks, not final evidence. The stand
run owns the manual SC table and leaves server configuration, files, and tmux sessions as they were.

## Backward Compatibility

**Breaking changes:** no.

Unlike feature 017, which turned `-W` into a string flag, this feature adds no CLI flag and changes
no existing one. The `'t'` keybinding keeps its byte-for-byte definition; what changes is that
pressing it a second time on the `tables` screen now cycles instead of re-entering `tables`. On
versions below PG 19 even that is invisible.

**Migration strategy:** none needed.

**DB migration compatibility:** N/A — pgcenter owns no schema.

**Consumer impact:**
- Recorded archives: old single-layout archives replay as before. Mixed PG 19 layouts created by
  `record -a` across a pgcenter upgrade are supported through Decision 9's layout boundary; they do not
  reuse stale alignment or produce cross-layout deltas. `autovacuum_scores` remains `NotRecordable`.
- `report -T` / the `tables` tar prefix / the `describeReport` key: the group name `"tables"` is
  simultaneously all three, so the view cannot be renamed. Same collision the `wal` group carries;
  the same in-code comment applies.
- `architecture.md`'s statement that no production view sets `NotRecordable` any more becomes false
  and is corrected in the feature's documentation work, not deferred to release finalization.

## Risks

| Risk | Mitigation |
|------|-----------|
| PG 19 catalog drifts between beta2 and GA, changing column names or the set | Locked column-name test on the new query — drift reddens a test instead of silently corrupting the screen. The release already plans a CI matrix re-run at RC/GA. |
| The `for_wraparound` test passes without testing anything, because the probe relation sits in `public` | The fixture puts the flagged relation in `pg_toast` via the `toast.` reloption. The mutation must be run and the **named** assertion observed red — not merely "no FAIL lines", which a broken build also produces. |
| `toggleSysTables` partially edited — the toggle flips the indicator but not the rows | Four literals, one of which is a slice **length** (`make([]string, 3)`) that panics inside a key handler, which gocui does not recover. Enumerated in the task; the test table's shape changes too, since the existing markers do not appear in the new query. |
| Help-screen edits break the pinned adjacency tests | `'t' tables,` must be removed from the `s,t,i` line or the entry-lookup helper fails on marker ambiguity; the new `t,T` row cannot sit between `j,J` and `w,W`. The free slot is after `w,W`. |
| `Test_switchViewTo` builds the app with `VersionNum` zero, so the load-bearing cycle row fails once the cycle is version-aware | The test table gains a version column; this is expected churn, not a regression, and it is called out in the task so it is not "fixed" by weakening the assertion. |
| The full suite breaks with an error naming no view, because `recorder_test` passes an unfiltered view map against a PG 17 fixture | Explicit task item; the minimal fix mirrors `record/record.go`'s own call: filter the map before handing it to the recorder. |
| A same-pgcenter-version width mismatch reaches replay via `record -a` across a pgcenter upgrade | Decision 9 treats any recorded column-layout change as a boundary and resets all derived render/sort state before the next sample. Decision 1's bounds invariant remains defence in depth. Same-version tests cover both directions and blank/populated `stats_age`; tech debt [020] remains a separate malformed-archive issue. |

## Acceptance Criteria

Технические критерии приёмки (дополняют пользовательские из user-spec):

- [ ] `make test` зелёный в CI-образе проекта на кластерах PG 14–19, гонок нет.
- [ ] `make lint` и `make vuln` чистые.
- [ ] Нет регрессий в существующих тестах; все count-based литералы обновлены согласованно
      (реестр вью, доступность по версиям, фильтр записи, пункты меню, циклы переключения,
      help-экран).
- [ ] Селекторы `tables`/`indexes`/`autovacuum_scores` возвращают 3-кортеж; `UniqueKey`
      и `OrderKey` не патчатся в `Configure()`.
- [ ] Статические записи `New()` для `tables`/`indexes` сохраняют значения для PG ≤ 18.
- [ ] Утверждения на `Ncols` для `tables`/`indexes` **добавлены** (сегодня их нет вообще),
      как и негативные утверждения на `functions`/`sizes`.
- [ ] Тест на `DiffIntvl{0,0}` показан красным при ненулевом интервале.
- [ ] Тест escape-hatch показан красным при удалении `OR s.for_wraparound`, и красной становится
      именно проверка присутствия строки, а не установка фикстуры.
- [ ] Записи техдолга [035] и [029] обновлены: области действия расширены на `stats_age` и
      `autovacuum_scores` соответственно. Запись [020] НЕ трогаем — её достижимость этой фичей
      не меняется, потому что диффуемые экраны с версионно-зависимой шириной уже существуют
      (`wal`, `bgwriter`, `databases_general`).
- [ ] Заведена новая запись техдолга на version-blind циклы `x` и `p`: `statements_jit` (PG 15+)
      и `progress_copy` (PG 14+) достижимы циклом на серверах, где их нет, и экран отдаёт ошибку
      каждый тик. Эта фича их не чинит — она чинит только собственную группу `t`.
- [ ] Same-PG19 replay обрабатывает обе смены pgcenter-layout как boundary: нет cross-layout delta,
      stale widths/headers или потери blank/populated `stats_age`; sort key пересчитывается (Decision 9).
- [ ] SQL structure/order tests различают qualified unrounded ordering, LEFT JOIN/relid, outer
      `schemaname`, все rounding/boolean casts; rounded-tie integration fixture падает при ORDER BY alias.
- [ ] PG 19 recording INFO содержит `autovacuum_scores`, `not recordable`, `live TUI only` и не содержит
      ложного version-unsupported объяснения для deliberate exclusion.
- [ ] Wraparound targeted test имеет отдельный PASS внутри одноразового PG 19 network namespace с
      fail-closed `system_identifier` preflight и external supervisor cleanup; skip в общем наборе не
      засчитывается (Decision 10).
- [ ] Acceptance evidence минимизировано, sanitized и не содержит raw full archives/transcripts или
      секретов; временные сырые артефакты удалены (Decision 12).
- [ ] Все scriptable SC имеют одобренный готовый acceptance harness и проходят с сохранёнными
      base/freeze/final evidence; ручные SC явно отмечены и не выдаются за автоматические.

## Proposed Follow-ups

| Proposal | Why out of current scope | Rough cost |
|----------|--------------------------|------------|
| Record/report support for `autovacuum_scores` | The recorded layout is deliberately allowed to settle through use; the 0.13.0 roadmap owns the retrofit | M |
| Version-aware `x` and `p` cycles | They belong to other screen groups and would expand this area pass; register the debt instead | M |
| Recompute frozen column widths while a screen remains open | Shared rendering behaviour affects every screen; extend debt [035], do not fix it here | M |

## Requirements Coverage

| Requirement | Design elements |
|-------------|-----------------|
| US-1: threshold and actual autovacuum verdict | New view and query; Data Models value contract; Decisions 2–4 |
| US-2: queue priority and absolute work | `OrderKey=1`, unrounded SQL ordering, `dead_total` outer join; Decision 3 |
| US-3: accumulation period on relation screens | PG 19 selectors and trailing `stats_age`; Decisions 1, 2, and 5 |
| US-4: visible wraparound pressure | ViewType schema predicate with `for_wraparound` exception; Decision 4 and fixture contract |
| US-5: honest behaviour below PG 19 | Version-aware cycle/menu and refusal path; Decisions 6 and 7 |
| AC-1, AC-2, AC-5 | Eleven-column Data Models contract and locked live column-name assertions |
| AC-3, AC-4 | SQL rounding and boolean text value contract |
| AC-6, AC-7 | Registry ordering plus qualified unrounded SQL `ORDER BY`; Decision 3 |
| AC-8, AC-9 | Outer relation/join and outer `schemaname` predicate; Decision 4 |
| AC-10, AC-11 | ViewType conditional and system-schema wraparound fixture |
| AC-12 | `DiffIntvl{0,0}` short-circuit and mutation-checked empty-cell test |
| AC-13–AC-17 | `tablesNextView`, two-item menu, keybindings, single cmdline write; Decisions 6 and 7 |
| AC-18–AC-20 | Version-labelled menu item and refusal without view/channel mutation; Decisions 6 and 7 |
| AC-21, AC-22, AC-25, AC-26 | PG 19 tail-column selectors, `pg_stat_*` source, nullable rendering; Decisions 1 and 2 |
| AC-23, AC-29, AC-30 | Static legacy layouts and negative `functions`/`sizes` assertions; Decision 5 |
| AC-24 | Tail placement outside unchanged `DiffIntvl`, including bounds invariant; Decision 1 |
| AC-27, AC-28 | Disposable acceptance reset plus manual growth/untouched-row observation |
| AC-31 | `NotRecordable: true` registry entry, record filtering, deterministic named live-TUI-only INFO; Decision 11 |
| AC-32 | PG 14–19 integration matrix, filtered recorder fixture, dedicated wraparound PASS, full quality gate |
| AC-33 | Version-aware replay fixtures and layout-state reset; Backward Compatibility |
| AC-34 | Locked query column list and live `FieldDescriptions()` order assertion |
| AC-35 | `t`/`T` keybinding and help contracts |
| AC-36 | `report/describe.go` trailing PG 19+ rows and column-order tests |
| AC-37–AC-40 | Feature catalog, both Project Knowledge references, release notes, and debts [035]/[029] documentation contracts |
| AC-41 | Recorded layout-change boundary, renderer/sort reset, and same-version bidirectional replay tests; Decision 9 |
| SC-1 | Disposable PG 19 acceptance reset, live record/report, and tar-entry observation |
| SC-2 | Synthetic mixed-version replay fixtures and report layout-state contract |
| SC-3 | Built-binary `describe` observation for both relation report types |
| SC-4 | Byte-identical A/B CLI replay on identical legacy archives; structural assertions remain separate |
| SC-5 | Observable PG 19 view/navigation/help manual contract; Decisions 6 and 7 |
| SC-6 | System-schema filter and stable wraparound fixture; Decision 4 |
| SC-7 | PG 18 A/B legacy cycle and new disabled-menu behaviour; Decisions 6 and 7 |
| SC-8 | Manual reset/growth observation on both relation screens; tail-column selectors |
| SC-9 | Documentation/register review covering AC-37–AC-40 |
| SC-10 | Same-version bidirectional pgcenter-layout replay boundary; Decision 9 |

## Appendix: Unapproved Legacy Task Draft

The remainder predates the current Kimi decomposition phase. It is retained only as input for the
next session, is not part of the approved technical contract, and must not be executed as written.
The next phase must produce reviewed decomposition/task artifacts, model tiers, an Acceptance
Execution Map, and a ready frozen harness. Legacy Task 8 maps to the separate `freeze-qa` phase.
Review rounds added Decisions 9–12 and SC-10 after this draft was written: decomposition must explicitly
own `report/report.go` layout-boundary work, the dedicated wraparound cluster harness, honest recording
INFO, discriminating SQL/DB-time assertions, and sanitized evidence. The legacy file lists are therefore
input, not complete coverage.

**Reviewer note.** Tasks 1, 2 and 5 carry `dev-security-auditor` because they touch SQL
construction or the recorded-archive replay path, which is this project's only untrusted input.
Tasks 3, 4 and 6 omit it deliberately: they wire registry entries, test literals, keybindings and
help text, and introduce no new data path — Task 3's `record` edits are test-only and change no
production parsing. The omission is a choice, not an oversight.

### Wave 1 (независимые)

#### Task 1: New `autovacuum_scores` query and selector
- **Description:** Create `internal/query/autovacuum_scores.go` with the 11-column query over
  `pg_stat_autovacuum_scores` left-joined to `pg_stat_all_tables`, and its version selector, per
  Decisions 2, 3 and 4. Includes the locked column-name test that guards against PG 19 catalog drift
  and the wraparound escape-hatch test with its system-schema fixture.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor
- **Verify:** bash — `go test ./internal/query/ -run AutovacuumScores` in the CI image
- **Files to modify:** `internal/query/autovacuum_scores.go`, `internal/query/autovacuum_scores_test.go`
- **Files to read:** `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area.md`
  (the acceptance criteria the value contract comes from), `internal/query/archiver.go`,
  `internal/query/archiver_test.go`, `internal/query/replication_slots.go`,
  `internal/query/activity.go`, `internal/query/query.go`, `internal/postgres/testing.go`

#### Task 2: `stats_age` on `tables` and `indexes`
- **Description:** Add a PG 19 constant to each of `internal/query/tables.go` and `indexes.go` with
  `stats_age` as the tail column sourced from the `pg_stat_*` half of the join, plus a 3-tuple
  selector each. Rewrite both query tests from the `conn.Exec`-and-discard tier to the
  `FieldDescriptions()` tier so column counts become observable, fixing the missing
  connection close — it is called but not deferred, so a failed assertion leaks it — and the legacy
  version list where six versions always skip.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor
- **Verify:** bash — `go test ./internal/query/ -run 'StatTables|StatIndexes'` in the CI image
- **Files to modify:** `internal/query/tables.go`, `internal/query/indexes.go`,
  `internal/query/tables_test.go`, `internal/query/indexes_test.go`
- **Files to read:** `internal/query/wal.go`, `internal/query/wal_test.go`,
  `internal/query/bgwriter.go`, `internal/query/archiver_test.go`

### Wave 2 (зависит от Wave 1)

#### Task 3: View registry, `Configure()` wiring and count-based tests
- **Description:** Register `autovacuum_scores` in `view.New()` and add the three `Configure()` cases.
  Update every count-based test literal that moves, and add the `Ncols` assertions for
  `tables`/`indexes` plus the negative ones for `functions`/`sizes` — none of these exist today, so
  they must be written, not edited. Includes the one-line recorder-test fix: registering a PG 19-only
  view breaks a test that hands an unfiltered view map to the recorder against a PG 17 fixture, so
  the fix belongs in the same task that breaks it, not a wave later. Also owns the `calculateDelta`
  short-circuit test, which proves an empty cell cannot abort a sample on a screen that diffs
  nothing. Record in-code that the `tables`/`indexes` cases are load-bearing rather than drift guards.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer
- **Verify:** bash — `go test ./internal/view/ ./record/ ./internal/stat/`
- **Files to modify:** `internal/view/view.go`, `internal/view/view_test.go`, `record/record_test.go`,
  `record/recorder_test.go`, `internal/stat/postgres_test.go`
- **Files to read:** `internal/query/autovacuum_scores.go`, `internal/query/tables.go`,
  `internal/query/indexes.go`, `record/record.go`, `top/config_view.go`

### Wave 3 (зависит от Wave 2)

#### Task 4: Version-aware `t`/`T` navigation
- **Description:** Add the cycle, the menu and the keybinding for the new hotkey group, all
  version-aware: `t` yields the new screen only on PG 19, `T` shows a two-item menu whose second
  entry carries the availability suffix below PG 19 and refuses selection there. This is a new
  pattern in the codebase and is deliberately confined to this group.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer
- **Verify:** bash — `go test ./top/ -run 'Tables|Menu|Keybindings'`
- **Files to modify:** `top/config_view.go`, `top/menu.go`, `top/keybindings.go`,
  `top/config_view_test.go`, `top/menu_test.go`, `top/keybindings_test.go`
- **Files to read:** `internal/view/view.go`, `internal/stat/stat.go`, `top/top.go`,
  `docs/decisions-log.md`

#### Task 5: Record and report side
- **Description:** Add the `stats_age` lines to the per-column descriptions of both widened screens,
  and generalize the replay test harness from a hardcoded `activity` screen name so the PG 18 → PG 19
  replay of `tables` and `indexes` can be exercised — the first time a diffed screen goes through
  that branch. Also refresh the stale in-code comments claiming no production view sets
  `NotRecordable` any more.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor
- **Verify:** bash — `go test ./record/ ./report/`
- **Files to modify:** `report/describe.go`, `report/report.go`, `report/report_test.go`, `record/record.go`
- **Files to read:** `record/record_test.go`, `report/report.go`, `internal/view/view.go`

### Wave 4 (зависит от Wave 3)

#### Task 6: `,` toggle wiring and the help screen
- **Description:** Wire the new screen into the show-system-relations toggle — four literals in
  `toggleSysTables`, one of them a slice length whose omission panics inside a key handler — and add
  the `t,T` help row, removing the now-ambiguous `'t' tables,` marker from the existing line. The
  toggle test's table changes shape, not just length: its current markers do not appear in the new
  query.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer
- **Verify:** bash — `go test ./top/ -run 'ToggleSysTables|help'`
- **Files to modify:** `top/config_view.go`, `top/help.go`, `top/config_view_test.go`,
  `top/help_test.go`
- **Files to read:** `internal/query/autovacuum_scores.go`, `top/menu.go`

### Wave 5 (зависит от Wave 4)

#### Task 7: Documentation and registers
- **Description:** Update the user-facing documentation for the new screen and the two new columns,
  extend the scope of debt entry [035] to cover `stats_age` — and leave [020] alone — fix the architecture
  note claiming no production view sets `NotRecordable` any more — this feature makes that false —
  and register a new debt item for the two version-blind cycles (`x` and `p`) that this feature
  deliberately does not fix. Release notes are required: a new screen is user-visible even though no
  CLI flag moved.
- **Skill:** documentation-writing
- **Reviewers:** dev-code-reviewer
- **Verify:** bash — `grep` the five documents for the new screen name, the two new columns and the
  updated debt entries; each must be present
- **Files to modify:** `docs/features-catalog.md`,
  `.kimi-code/skills/project-knowledge/references/project.md`,
  `.kimi-code/skills/project-knowledge/references/architecture.md`, `doc/release-notes/v0.12.0.md`,
  `docs/tech-debt.md`
- **Files to read:** `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area.md`,
  `docs/roadmap-0.12.0.md`

### Final Wave

#### Task 8: Pre-deploy QA
- **Description:** Acceptance testing: `make test` in the CI image against PG 14–19, plus `make lint`
  and `make vuln` — this task owns all three, no earlier task runs them. Verification of every
  acceptance criterion from user-spec and tech-spec. The interactive TUI criteria are handed to the
  stand run, including the A/B comparison against a `master`-built binary.
- **Skill:** freeze-qa
- **Reviewers:** none
