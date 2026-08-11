---
created: 2026-08-10
status: draft
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
  more, which this feature makes false.
- **`report/describe.go`** — `stats_age` lines for the two screens.
- **`report/report_test.go`** — the replay harness generalized from `activity` to any screen name.

### How it works

Unchanged data flow — `query → stat → view → gocui`. The only new edge is the version number
reaching two places it did not reach before: `tablesNextView` (via `switchViewTo`, which already
holds `app.postgresProps.VersionNum` for its `ExtPGSSSchema` guard) and the menu construction.

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
**This decision is load-bearing for memory safety, not only for tidiness** — surfaced by the security
review and not visible in the original rationale. `record -a` can append to an archive across a
*pgcenter* upgrade, producing two samples of the **same** PostgreSQL version with different widths;
`versionChanged` watches only the server version, so that pair reaches `diff()`. It does not panic,
and the margin is exactly zero: `diff`'s interval is inclusive, so the highest index it passes to
`prev.Values[j]` is `DiffIntvl[1]` = 18, against a 19-column PG ≤ 18 row — the last valid index.
`indexes` is the same shape, 5 against 6. Tail-appending is the only thing keeping that loop in
bounds. Mid-layout insertion would have pushed the interval to `{1,19}` and made the pair a panic.
The invariant must therefore be asserted, not assumed: **`DiffIntvl[1] < min(Ncols)` across every
version** of both screens.

**Alternatives considered:** mid-layout insertion next to the other timestamps (rejected — buys
nothing, costs a version-dependent interval, and as above turns a benign width mismatch into an
out-of-range read); no selector at all, letting `Ncols` stay static (rejected — see Decision 5).

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
| `x` — `statementsNextView` | `statements_jit`, `MinRequiredVersion: PostgresV15` (`view.go:240`) | `config_view.go:313` — takes `current string` only |
| `p` — `progressNextView` | `progress_copy`, `MinRequiredVersion: PostgresV14` (`view.go:314`) | `config_view.go:338` — takes `current string` only |

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

### Decision 8: tech debt [035] is recorded, not fixed; [020] needs no edit
**Decision:** extend the scope note of [035]; leave [020] alone; change no shared code.
**Rationale:** [035] (column widths frozen from the first batch and never recomputed while a screen
stays open) now also affects `stats_age`, which is empty on entry for most clusters — so the column
freezes at header width and truncates once values pass a day. The fix lives in the shared alignment
path and would change behaviour on every screen, which the roadmap owner explicitly refused:
"боюсь расширять скоуп фиксом в общем коде". [020] gets no edit: its reachability is **not** widened
by this feature, because diffed screens with version-dependent widths already exist (`wal`,
`bgwriter`, `databases_general`) — an earlier draft of this spec claimed otherwise and was wrong.
**Alternatives considered:** fixing [035] here (rejected by the roadmap owner); amending [020]
(rejected — the amendment would have recorded something untrue).

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
| `ColsWidth` | `map[int]int{}` (non-nil — two writers mutate it in place; a nil map is a panic on the first column-width change, not a wrong number) |
| `Filters` | `map[int]*regexp.Regexp{}` (same) |

Column layout, in order: `relation`, `score`, `do_vacuum`, `do_analyze`, `for_wraparound`,
`dead_total`, `xid_score`, `mxid_score`, `vacuum_score`, `vacuum_insert_score`, `analyze_score`.

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

## Testing Strategy

**Feature size:** L

### Unit tests
- Selector branch coverage for all three selectors across the version list, including boundary and
  out-of-range probes (`0`, `180000`, `190000`, `200000`), in the `archiver_test.go` shape.
- Locked column-name list for the new query — the guard against PG 19 catalog drift in beta3/RC.
  Assert the names *and* the order, and assert `FROM pg_stat_autovacuum_scores` is the outer
  relation.
- `stats_age` is the last column on both PG 19 layouts. The load-bearing assertion is the **column
  order** combined with `DiffIntvl[1] == Ncols-2`; asserting `DiffIntvl == {1,18}` alone passes even
  if `stats_age` were inserted mid-layout, so it proves nothing on its own.
- The bounds invariant from Decision 1: `DiffIntvl[1] < min(Ncols)` over every version of both
  screens. This is what keeps a same-pgcenter-version width mismatch a benign mismatch rather than an
  out-of-range read.
- `stats_age` is sourced from the `pg_stat_*` half of the join, not `pg_statio_*`. Both halves carry
  the column on PG 19 and the wrong one returns a plausible value, so this needs an assertion on the
  query text — nothing else would ever catch it.
- `tablesNextView(current, version)` table test.
- Menu: item count, the version-dependent suffix, and a refusal row that asserts nothing is pushed
  on `viewCh` and the view is unchanged.
- Keybinding registration for `'T'` and `'t'` via `keybindingsList` — the only seam that makes
  *which handler a key carries* assertable.
- Registry invariants for the new view (`key == v.Name`, non-nil maps, `NotRecordable`,
  `MinRequiredVersion`, `Ncols`, `DiffIntvl`, `OrderKey`, `OrderDesc`).
- `calculateDelta` short-circuit: a `PGresult` with an empty `dead_total` cell through
  `DiffIntvl{0,0}` produces no error. **Must be shown to fail** when the interval is changed to a
  non-zero pair, otherwise it proves nothing about the screen.

### Integration tests
- `tables`/`indexes` query execution against PG 14–19 fixtures, rewritten from the current
  `conn.Exec`-and-discard tier to the `FieldDescriptions()` tier so column counts are actually
  observable. While rewriting, fix two pre-existing defects in place: the missing `defer
  conn.Close()` and the legacy 12-version list where six versions always skip (tech debt [024]).
- The new query against PG 19, asserting the 11 column names in order.
- **The `for_wraparound` escape hatch**, with the fixture proven during the user-spec phase:
  `ALTER TABLE … SET (toast.autovacuum_freeze_max_age = 100000)` plus ~120k committed transactions
  through a procedure with `COMMIT` inside the loop (262 ms measured). Three constraints, each of
  which the test is worthless without:
  - The flagged relation **must** be in a system schema. A table in `public` is visible in user mode
    with or without the escape hatch, so a test built on one passes either way.
  - The flag **must be made stable for the duration of the assertion**. Autovacuum clears
    `for_wraparound` within one `autovacuum_naptime`, and `autovacuum_enabled = false` does not
    protect it — wraparound-prevention vacuums ignore that reloption by design. So the test runs with
    `autovacuum` off at the cluster level (a SIGHUP-level GUC: `ALTER SYSTEM` + `pg_reload_conf()`),
    and restores it afterwards. Without this, a run where the flag was already cleared is
    indistinguishable from the mutation run that is supposed to be red.
  - Discriminating assertion: user mode returns the flagged row; deleting `OR s.for_wraparound` from
    the `WHERE` must redden *that* assertion, not the fixture setup.
- **Privilege behaviour of the new view, in both directions**, following `archiver_test.go`: run the
  query through `SetupTestRole(…, true)` and `SetupTestRole(…, false)`. The measurement that the view
  needs no privileges was taken on beta2; the locked column-name test is blind to an ACL change,
  which is the likeliest axis to move for a brand-new per-relation view between beta and GA. Every
  other test connects as `postgres` over `trust` and would never notice.
- The PG 18 → PG 19 replay of **both** `tables` and `indexes` — the user-spec names both, and both
  change width. Requires `buildActivityTar` and `activityReplayConfig` to take a screen-name
  parameter; once parameterized, the second screen is one more table row. Two ticks per version are
  mandatory — the first of each version is consumed by the version-change branch. This is the first
  time a diffed screen goes through that branch; the three existing version-change tests are all
  `activity` with `DiffIntvl{0,0}`.

### E2E tests
No automated E2E. Interactive TUI behaviour is verified by hand on a stand, with a second binary
built from `master` for A/B — without it, long-standing defects get filed against this feature.

## Agent Verification Plan

**Source:** user-spec "Как проверить" section.

### Verification approach
Everything except the interactive TUI paths is covered by the test suite run against the project's
CI image, which carries PG 14–19 clusters. Two user-spec criteria are **not** agent-verifiable and
are stated as such:

- *"the screen caption prints exactly once on each entry path"* — `printCmdline` writes through
  `g.Update`, and on a zero-value `&gocui.Gui{}` the spawned goroutine parks forever on a nil
  channel, so a test can neither count calls nor read the buffer. Code review plus stand run.
- *"`stats_age` grows between refreshes"* — requires `pg_stat_reset()` on the shared PG 19 fixture,
  which is database-wide and irreversible, plus a ≥ 1 s sleep between samples. Automating it means
  either cross-test contamination or serialising the whole PG 19 subtree. Stand run.

### Per-task verification
| Task | verify: | What to check |
|------|---------|--------------|
| 1 | bash | `go test ./internal/query/ -run AutovacuumScores` — column names, order, outer relation, selector branches |
| 2 | bash | `go test ./internal/query/ -run 'StatTables\|StatIndexes'` — 19/20 and 6/7 per version |
| 3 | bash | `go test ./internal/view/ ./record/` — registry counts, Configure arms, filter rows, recorder trap fixed |
| 4 | bash | `go test ./top/ -run 'Tables\|Menu\|Keybindings'` — cycle, menu suffix, refusal, bindings |
| 5 | bash | `go test ./record/ ./report/` — describe entries, 18→19 replay of `tables` and `indexes` |
| 6 | bash | `go test ./top/ -run 'ToggleSysTables\|help'` — four literals, help adjacency |
| 7 | bash | `grep` over all five changed documents — including the architecture note — for the new screen name, the two new columns, the extended [035] entry and the new cycle-debt entry |
| 8 | bash + user | `make test` in the CI image, `make lint`, `make vuln`; then the stand run per the user-spec checklist |

### Tools required
bash (project CI image for the PG-backed suite), git. No MCP tools, no deploy.

## Backward Compatibility

**Breaking changes:** no.

Unlike feature 017, which turned `-W` into a string flag, this feature adds no CLI flag and changes
no existing one. The `'t'` keybinding keeps its byte-for-byte definition; what changes is that
pressing it a second time on the `tables` screen now cycles instead of re-entering `tables`. On
versions below PG 19 even that is invisible.

**Migration strategy:** none needed.

**DB migration compatibility:** N/A — pgcenter owns no schema.

**Consumer impact:**
- Recorded archives: unaffected. `autovacuum_scores` is `NotRecordable`, and the two widened screens
  render from recorded data — `report` never reads `view.Ncols` for them, and `DiffIntvl` is
  unchanged, so old archives replay exactly as before.
- `report -T` / the `tables` tar prefix / the `describeReport` key: the group name `"tables"` is
  simultaneously all three, so the view cannot be renamed. Same collision the `wal` group carries;
  the same in-code comment applies.
- `architecture.md`'s statement that no production view sets `NotRecordable` any more becomes false
  and is corrected by Task 7, not deferred to finalization.

## Risks

| Risk | Mitigation |
|------|-----------|
| PG 19 catalog drifts between beta2 and GA, changing column names or the set | Locked column-name test on the new query — drift reddens a test instead of silently corrupting the screen. The release already plans a CI matrix re-run at RC/GA. |
| The `for_wraparound` test passes without testing anything, because the probe relation sits in `public` | The fixture puts the flagged relation in `pg_toast` via the `toast.` reloption. The mutation must be run and the **named** assertion observed red — not merely "no FAIL lines", which a broken build also produces. |
| `toggleSysTables` partially edited — the toggle flips the indicator but not the rows | Four literals, one of which is a slice **length** (`make([]string, 3)`) that panics inside a key handler, which gocui does not recover. Enumerated in the task; the test table's shape changes too, since the existing markers do not appear in the new query. |
| Help-screen edits break the pinned adjacency tests | `'t' tables,` must be removed from the `s,t,i` line or the entry-lookup helper fails on marker ambiguity; the new `t,T` row cannot sit between `j,J` and `w,W`. The free slot is after `w,W`. |
| `Test_switchViewTo` builds the app with `VersionNum` zero, so the load-bearing cycle row fails once the cycle is version-aware | The test table gains a version column; this is expected churn, not a regression, and it is called out in the task so it is not "fixed" by weakening the assertion. |
| The full suite breaks with an error naming no view, because `recorder_test` passes an unfiltered view map against a PG 17 fixture | Explicit task item; the minimal fix mirrors `record/record.go`'s own call: filter the map before handing it to the recorder. |
| A same-pgcenter-version width mismatch reaches `diff()` via `record -a` across a pgcenter upgrade | Survives only because the tail append keeps `DiffIntvl[1]` at the last valid index — zero margin, see Decision 1. Guarded by an explicit `DiffIntvl[1] < min(Ncols)` invariant test rather than left implicit. Tech debt [020] itself is untouched: its reachability is unchanged by this feature, since diffed screens with version-dependent widths already exist. |

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
- [ ] Запись техдолга [035] обновлена: область действия расширена на `stats_age`.
      Запись [020] НЕ трогаем — её достижимость этой фичей не меняется, потому что диффуемые
      экраны с версионно-зависимой шириной уже существуют (`wal`, `bgwriter`, `databases_general`).
- [ ] Заведена новая запись техдолга на version-blind циклы `x` и `p`: `statements_jit` (PG 15+)
      и `progress_copy` (PG 14+) достижимы циклом на серверах, где их нет, и экран отдаёт ошибку
      каждый тик. Эта фича их не чинит — она чинит только собственную группу `t`.

## Implementation Tasks

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
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/query/ -run AutovacuumScores` in the CI image
- **Files to modify:** `internal/query/autovacuum_scores.go`, `internal/query/autovacuum_scores_test.go`
- **Files to read:** `internal/query/archiver.go`, `internal/query/archiver_test.go`,
  `internal/query/replication_slots.go`, `internal/query/activity.go`, `internal/query/query.go`,
  `internal/postgres/testing.go`

#### Task 2: `stats_age` on `tables` and `indexes`
- **Description:** Add a PG 19 constant to each of `internal/query/tables.go` and `indexes.go` with
  `stats_age` as the tail column sourced from the `pg_stat_*` half of the join, plus a 3-tuple
  selector each. Rewrite both query tests from the `conn.Exec`-and-discard tier to the
  `FieldDescriptions()` tier so column counts become observable, fixing the missing
  connection close — it is called but not deferred, so a failed assertion leaks it — and the legacy
  version list where six versions always skip.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
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
  the fix belongs in the same task that breaks it, not a wave later. Record in-code that the
  `tables`/`indexes` cases are load-bearing rather than drift guards.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./internal/view/ ./record/`
- **Files to modify:** `internal/view/view.go`, `internal/view/view_test.go`, `record/record_test.go`,
  `record/recorder_test.go`
- **Files to read:** `internal/query/autovacuum_scores.go`, `internal/query/tables.go`,
  `internal/query/indexes.go`, `record/record.go`, `top/config_view.go`

### Wave 3 (зависит от Wave 2)

#### Task 4: Version-aware `t`/`T` navigation
- **Description:** Add the cycle, the menu and the keybinding for the new hotkey group, all
  version-aware: `t` yields the new screen only on PG 19, `T` shows a two-item menu whose second
  entry carries the availability suffix below PG 19 and refuses selection there. This is a new
  pattern in the codebase and is deliberately confined to this group.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
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
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./record/ ./report/`
- **Files to modify:** `report/describe.go`, `report/report_test.go`, `record/record.go`
- **Files to read:** `record/record_test.go`, `report/report.go`, `internal/view/view.go`

### Wave 4 (зависит от Wave 3)

#### Task 6: `,` toggle wiring and the help screen
- **Description:** Wire the new screen into the show-system-relations toggle — four literals in
  `toggleSysTables`, one of them a slice length whose omission panics inside a key handler — and add
  the `t,T` help row, removing the now-ambiguous `'t' tables,` marker from the existing line. The
  toggle test's table changes shape, not just length: its current markers do not appear in the new
  query.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
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
- **Verify:** bash — `grep` the four documents for the new screen name, the two new columns and the
  updated debt entries; each must be present
- **Files to modify:** `docs/features-catalog.md`, `.claude/skills/project-knowledge/overview.md`,
  `.claude/skills/project-knowledge/architecture.md`, `doc/release-notes/v0.12.0.md`,
  `docs/tech-debt.md`
- **Files to read:** `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area.md`,
  `docs/roadmap-0.12.0.md`

### Final Wave

#### Task 8: Pre-deploy QA
- **Description:** Acceptance testing: `make test` in the CI image against PG 14–19, plus `make lint`
  and `make vuln` — this task owns all three, no earlier task runs them. Verification of every
  acceptance criterion from user-spec and tech-spec. The interactive TUI criteria are handed to the
  stand run, including the A/B comparison against a `master`-built binary.
- **Skill:** pre-deploy-qa
- **Reviewers:** none
