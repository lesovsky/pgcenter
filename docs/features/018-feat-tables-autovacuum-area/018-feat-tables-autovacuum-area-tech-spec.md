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
snapshot and is never diffed. No crash, no design work needed. What *is* new is that `tables` and
`indexes` become the first screens with a non-empty `DiffIntvl` whose width is version-dependent, so
tech debt **[020]** (same-version malformed-width archives) becomes reachable on a diffed screen
where it previously could not be. We do **not** fix [020] here — its own note records that
`align.SetAlign` is a third unsafe consumer, so fixing `diff` alone would not close the class. We
update its "why deferred" note instead.

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
- **`record/recorder_test.go`** — the unfiltered-view-map trap.
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
**Rationale:** ADR [012] chose the opposite for the progress screens — inserting new columns
mid-layout for readability and paying with a version-dependent `DiffIntvl` — and recorded the hazard
plainly: a stale interval on the newer layout lands on columns whose values still parse as numbers,
so the wrong diff succeeds silently and prints plausible nonsense. Here the readability argument does
not apply: all seven existing `stats_age` columns in the codebase are last, so the tail is where a
reader already looks for it. Taking the tail keeps the interval literal valid on both versions and
avoids that entire class.
**Alternatives considered:** mid-layout insertion next to the other timestamps (rejected — buys
nothing, costs a version-dependent interval); no selector at all, letting `Ncols` stay static
(rejected — see Decision 5).

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
ADR [012] already rejected sibling selectors differing for a reason invisible at the call site).

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

### Decision 6: version-aware navigation is a new pattern, and it is confined to the `t` group
**Decision:** `tablesNextView(current string, version int)` and a version parameter reaching
`selectMenuStyle`; no other cycle or menu is touched.
**Rationale:** this is the first case where the anchor screen of a cycle (`tables`) is available on
every supported version while the second member requires PG 19. The existing precedents do not
cover it — `j`/`J` and `w`/`W` gate the whole group. Leaving it version-blind would send the reflex
key `t` into a screen that renders `ERROR: selected statistics is not supported by current version
of Postgres` in place of the whole table, every tick, on every version below 19. Generalizing the
pattern to the other cycles is explicitly out of scope: they have no unavailable members.
**Alternatives considered:** letting the screen error as `archiver` does on PG ≤ 11 (rejected — there
the affected versions are EOL, here they are the majority of supported ones); hiding the menu item
below PG 19 (rejected during the user-spec interview — explicit beats implicit).

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

### Decision 8: tech debt [020] and [035] are recorded, not fixed
**Decision:** update the register entries for both; change no shared code.
**Rationale:** [035] (column widths frozen from the first batch) now also affects `stats_age`, which
is empty on entry for most clusters — but the fix lives in the shared alignment path and would change
behaviour on every screen. [020]'s reachability widens because `tables`/`indexes` become the first
diffed screens with a version-dependent width; fixing `diff` alone would not close the class, since
`align.SetAlign` is a third unsafe consumer. Both were weighed by the roadmap owner during the
user-spec interview, and the decision was to keep the feature's blast radius inside its own area.
**Alternatives considered:** fixing [035] here (rejected explicitly by the roadmap owner —
"боюсь расширять скоуп фиксом в общем коде").

## Data Models

No database schema, no new Go types beyond the query constants and selector functions.

`view.View` fields set for the new screen:

| field | value |
|---|---|
| `Name` | `"autovacuum_scores"` |
| `QueryTmpl` | `query.PgStatAutovacuumScoresDefault` |
| `Ncols` | `11` |
| `DiffIntvl` | `[2]int{0,0}` |
| `OrderKey` | `1` (score) |
| `OrderDesc` | `true` |
| `UniqueKey` | `0` (default — relation) |
| `NotRecordable` | `true` |
| `MinRequiredVersion` | `query.PostgresV19` |
| `ColsWidth` | `map[int]int{}` (non-nil — three writers mutate it in place) |
| `Filters` | `map[int]*regexp.Regexp{}` (same) |

Column layout, in order: `relation`, `score`, `do_vacuum`, `do_analyze`, `for_wraparound`,
`dead_total`, `xid_score`, `mxid_score`, `vacuum_score`, `vacuum_insert_score`, `analyze_score`.

Selector return values:

| version | `tables` | `indexes` |
|---|---|---|
| `>= PostgresV19` | `PgStatTablesPG19`, 20, `{1,18}` | `PgStatIndexesPG19`, 7, `{1,5}` |
| below | `PgStatTablesDefault`, 19, `{1,18}` | `PgStatIndexesDefault`, 6, `{1,5}` |

## Dependencies

### New packages
None.

### Using existing (from project)
- `internal/query` — `Format` (plain `text/template`, no `Funcs()`, so `ne`/`if` builtins are
  available and already used by `activity.go` and `procpidstat.go` for `ShowNoIdle`).
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
  through a procedure with `COMMIT` inside the loop (262 ms measured). The flagged relation **must**
  be in a system schema — a table in `public` is visible in user mode with or without the escape
  hatch, so a test built on one passes either way. Discriminating assertion: user mode returns the
  flagged row; deleting `OR s.for_wraparound` from the `WHERE` must redden *that* assertion, not the
  fixture setup.
- The PG 18 → PG 19 replay of `tables`, requiring `buildActivityTar` and `activityReplayConfig` to
  take a screen-name parameter. Two ticks per version are mandatory — the first of each version is
  consumed by the version-change branch. This is the first time a diffed screen goes through that
  branch; the three existing version-change tests are all `activity` with `DiffIntvl{0,0}`.

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
| 3 | bash | `go test ./internal/view/ ./record/` — registry counts, Configure arms, filter rows |
| 4 | bash | `go test ./top/ -run 'Tables\|Menu\|Keybindings'` — cycle, menu suffix, refusal, bindings |
| 5 | bash | `go test ./record/ ./report/` — recorder trap fixed, 18→19 replay of `tables` |
| 6 | bash | `go test ./top/ -run 'ToggleSysTables\|help'` — four literals, help adjacency |
| 7 | bash | `make lint && make vuln` |
| 8 | user | stand run per the user-spec checklist |

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
  and must be corrected at feature finalization.

## Risks

| Risk | Mitigation |
|------|-----------|
| PG 19 catalog drifts between beta2 and GA, changing column names or the set | Locked column-name test on the new query — drift reddens a test instead of silently corrupting the screen. The release already plans a CI matrix re-run at RC/GA. |
| The `for_wraparound` test passes without testing anything, because the probe relation sits in `public` | The fixture puts the flagged relation in `pg_toast` via the `toast.` reloption. The mutation must be run and the **named** assertion observed red — not merely "no FAIL lines", which a broken build also produces. |
| `toggleSysTables` partially edited — the toggle flips the indicator but not the rows | Four literals, one of which is a slice **length** (`make([]string, 3)`) that panics inside a key handler, which gocui does not recover. Enumerated in the task; the test table's shape changes too, since the existing markers do not appear in the new query. |
| Help-screen edits break the pinned adjacency tests | `'t' tables,` must be removed from the `s,t,i` line or the entry-lookup helper fails on marker ambiguity; the new `t,T` row cannot sit between `j,J` and `w,W`. The free slot is after `w,W`. |
| `Test_switchViewTo` builds the app with `VersionNum` zero, so the load-bearing cycle row fails once the cycle is version-aware | The test table gains a version column; this is expected churn, not a regression, and it is called out in the task so it is not "fixed" by weakening the assertion. |
| The full suite breaks with an error naming no view, because `recorder_test` passes an unfiltered view map against a PG 17 fixture | Explicit task item; the minimal fix mirrors `record/record.go`'s own call: filter the map before handing it to the recorder. |
| Tech debt [020] becomes reachable on a diffed screen for the first time | Recorded, not fixed — see Decision 8. The legitimate version-change path is safe (traced in Solution); what widens is the malformed-archive class, which needs a fix in `validate()` covering three consumers. |

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
- [ ] Записи техдолга [020] и [035] обновлены: у [020] расширена область достижимости,
      у [035] — область действия на `stats_age`.

## Implementation Tasks

### Wave 1 (независимые)

#### Task 1: New `autovacuum_scores` query and selector
- **Description:** Create `internal/query/autovacuum_scores.go` with the 11-column query over
  `pg_stat_autovacuum_scores` left-joined to `pg_stat_all_tables`, and its version selector. The
  query carries the schema filter and the `for_wraparound` escape hatch in a `{{if ne .ViewType
  "all"}}` block, rounds the six scores to 2 decimals, renders booleans via `::text`, and orders by
  the unrounded score through a qualified reference. Includes the locked column-name test and the
  wraparound fixture test.
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
  `defer conn.Close()` and the legacy version list in place.
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
  they must be written, not edited. Record in-code that the `tables`/`indexes` cases are load-bearing
  rather than drift guards.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./internal/view/ ./record/`
- **Files to modify:** `internal/view/view.go`, `internal/view/view_test.go`, `record/record_test.go`
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
- **Description:** Fix the recorder test that feeds an unfiltered view map to the recorder against a
  PG 17 fixture, add the `stats_age` lines to the per-column descriptions of both widened screens,
  and generalize the replay test harness from a hardcoded `activity` screen name so the PG 18 → PG 19
  replay of `tables` can be exercised — the first time a diffed screen goes through that branch.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./record/ ./report/`
- **Files to modify:** `record/recorder_test.go`, `report/describe.go`, `report/report_test.go`
- **Files to read:** `record/record.go`, `report/report.go`, `internal/view/view.go`

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
  and correct the two tech-debt entries this feature changes the reachability of. Release notes are
  required — a new screen is a visible user-facing change even though no CLI flag moved.
- **Skill:** documentation-writing
- **Reviewers:** dev-code-reviewer
- **Verify:** bash — `git diff --stat` and manual read-through
- **Files to modify:** `docs/features-catalog.md`, `.claude/skills/project-knowledge/overview.md`,
  `doc/release-notes/v0.12.0.md`, `docs/tech-debt.md`
- **Files to read:** `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area.md`,
  `docs/roadmap-0.12.0.md`

### Final Wave

#### Task 8: Pre-deploy QA
- **Description:** Acceptance testing: full suite in the CI image against PG 14–19, lint and
  vulnerability checks, and verification of every acceptance criterion from user-spec and tech-spec.
  The interactive TUI criteria are handed to the stand run, including the A/B comparison against a
  `master`-built binary.
- **Skill:** pre-deploy-qa
- **Reviewers:** none
