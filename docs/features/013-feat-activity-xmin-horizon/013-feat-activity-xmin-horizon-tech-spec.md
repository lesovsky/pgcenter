---
created: 2026-07-25
status: approved
branch: feature/activity-xmin-horizon
size: M
---

# Tech Spec: Activity screen — xmin horizon and parallel worker grouping

## Solution

Add a PG-13+ branch to the `activity` query carrying three new columns — `leader`,
`backend_xid`, `horizon_xacts` — inserted at fixed positions in the existing column list. The
branch is an early return above the existing selector switch, which is left untouched, so
`PgStatActivityDefault` keeps naming that switch's `default:` case and now covers PG 10–12.

Two defects that this feature walks into are fixed alongside it:

1. **Sort-mode detection** (`internal/stat/postgres.go`) picks the comparator by inspecting a
   single cell and lets empty cells parse as `0`. Sparse columns therefore sort wrongly, which
   breaks the feature's primary user story.
2. **Stale layout state across a replayed version change** — tech debt [021] plus two neighbours
   it sits with: the header counter and the resolved sort-column index. This feature adds a
   boundary to that path where the column *positions* shift, so a latched index silently sorts a
   report by the wrong column.

Everything else the feature needs already exists: `view.Configure` already calls the activity
selector, `report/` never reads `view.Ncols`, and horizontal scroll already removed the width
constraint on this screen.

## Architecture

### What we're building/modifying

- **`internal/query/activity.go`** — new `PgStatActivityPG13` constant, returned from an early
  guard above the existing switch, which is left untouched (see Decision 3). Signature stays
  `(string, int)`. No new version constant is needed: `PostgresV13` already exists in `query.go`
  alongside the full `PostgresV10`–`PostgresV19` range.
- **`internal/stat/postgres.go`** — `PGresult.sort` gains non-empty sample selection and
  empty-last ordering across all three comparator modes.
- **`report/report.go`** — on a replayed version change, reset the alignment flag, the
  header-repeat counter, and the sort column (restoring the view's seed `OrderKey`/`OrderDesc`,
  not merely re-arming the latch); restore the zero-width guard in the truncation path.
- **`report/describe.go`** — three new column descriptions plus four caveats and a PG 13+ note in
  the activity block.
- **`docs/tech-debt.md`** — three new register entries.

Not modified: `internal/view/view.go` (the `Configure` wiring already exists and the seed stays as
it is — see Decision 3), `internal/stat/help.go` (dead code — see Decision 8), `internal/align/`,
the `replication` screen, and the `procpidstat` screen — whose *code* is untouched, though its
sorting changes through the shared function (Decision 4).

### How it works

At connect time `view.Configure(opts)` calls `SelectStatActivityQuery(opts.Version)`, which now
returns the 17-column query on PG 13+ and the unchanged 14-column query below it. The collector,
diff engine and renderer need no changes: `activity` has `DiffIntvl {0,0}`, so `calculateDelta`
takes the pass-through branch and never calls `diff()`; `UniqueKey` stays 0 because `pid` remains
column 0.

The three columns are produced entirely in SQL:

- `coalesce(leader_pid, pid) AS leader`
- `backend_xid::text` — explicit cast so the value scans (Decision 7)
- `age(backend_xmin) AS horizon_xacts` — returns `integer`, no cast needed

NULL reaches the renderer as an empty string because no `coalesce` is applied to the xid columns;
this is what satisfies the "blank, never 0" requirement.

Sorting is where the new columns interact with shared machinery. `PGresult.sort` reads
`r.Values[0][key]` to choose between numeric, duration and string comparators, and on a sparse
column that produces one of two wrong outcomes, deterministically but unpredictably from the
operator's side, since which row lands first is a property of the data:

- **First cell empty** — both `ParseFloat("")` and `parseDuration("")` fail, so the column falls to
  the **string** comparator and numbers order lexicographically, where `"9"` outranks `"1000000"`.
- **First cell non-empty and numeric** — the numeric comparator is chosen, but
  `strconv.ParseFloat("")` on the blank cells fails silently and yields `0`, so blanks become
  indistinguishable from a genuine zero and lead the screen on ascending sort.

Both halves are fixed: the sample is taken from the first non-empty cell, and empty cells are
ordered last independently of the comparator and of the sort direction.

`report/` needs no changes for the new columns: it derives widths, column names and sort keys from
the archive. It reads `stat.PGresult.Ncols` in two places — the procpidstat availability check and
`readMeta` — but never `view.Ncols`, which is the field the new branch changes.

## Decisions

### Decision 1: Insert the columns mid-layout, not at the tail

**Decision:** the PG 13+ layout is `pid, leader, cl_addr, cl_port, datname, usename, appname,
backend_type, wait_etype, wait_event, state, backend_xid, horizon_xacts, xact_age, query_age,
change_age, query`.

**Rationale:** follows ADR [012], which inserted the PG 19 progress columns mid-layout for the
same reason — the columns read as attributes of the row, and the tail is where `query` lives, so
appending would put them where horizontal scroll pushes them out of view. `leader` sits next to
`pid` because they are the same kind of fact about the process. `backend_xid → horizon_xacts →
xact_age` form a reading sequence: did it write, how far back does it hold the horizon, how long
has it been open.

Unlike [012] this costs nothing in layout metadata: `DiffIntvl` stays `{0,0}` and `UniqueKey`
stays 0, so the selector keeps its 2-tuple signature. The hazard [012] warned about — a stale
diff interval landing on numeric columns and printing plausible nonsense — cannot occur here,
because the pass-through branch never diffs.

**Alternatives considered:** appending at the tail (rejected — separates `horizon_xacts` from
`xact_age`, which are meant to be read together); a 3-tuple selector for symmetry with the other
version-aware selectors (rejected — it would return two constants, inviting the reader to think
they vary).

### Decision 2: Branch at PG 13, accepting that the boundary is not live-verifiable

**Decision:** the new branch triggers at `version >= 130000`.

**Rationale:** `leader_pid` was added in PG 13; `backend_xid`/`backend_xmin` exist since 9.4.
Branching at 14 to match the test matrix would withhold working columns from PG 13 for no reason
other than our own fixtures.

**Consequence that must be carried into implementation:** the test image has only PG 14–19, so
writing `version < 140000` instead of `version < 130000` would pass every live check. The table
test is the only guard, so it pins the boundary from **both** sides — PG 12 on the old branch,
PG 13 on the new one.

**Alternatives considered:** branching at 14 (rejected — correctness follows the catalog, not the
fixture set); adding a PG 13 cluster to the test image (rejected — disproportionate, and PG 13 is
past EOL).

### Decision 3: New branch as an early return; nothing is renamed and the seed keeps 14 columns

**Decision:** add `PgStatActivityPG13` and return it from an early `if version >= PostgresV13`
guard placed **above** the existing `switch`, which is left exactly as it is. Nothing is renamed.
The `activity` seed in `view.New()` is untouched — still `PgStatActivityDefault` with `Ncols: 14`.

**Rationale:** the naming question here is not what it first appears. There is **no consistent
project convention** to appeal to: `wal.go` returns `PgStatWALDefault` for the *newest* branch,
while `progress_vacuum.go` — feature 012, the most recent precedent — returns
`PgStatProgressVacuumPG19` for the newest and keeps `Default` for the older one. `bgwriter.go` and
`io.go` have no `Default` constant at all. Any claim that one of these is "the" convention would be
manufactured.

What *is* specific and real: in `activity.go` today `PgStatActivityDefault` literally names the
`switch`'s `default:` case. Appending a newer branch to that switch would quietly break that
correspondence. An early return above the switch keeps it intact — the newest version is handled
first and the historical ladder below is untouched — and it is the same shape both `wal.go` and
`progress_vacuum.go` already use. So the local meaning is preserved, the newest precedent is
followed, and no identifier moves.

The seed stays as it is because `Configure` overwrites it at connect time; it is a placeholder, not
a fact. Measured: raising `Ncols` to 17 breaks exactly two assertions in `top/config_view_test.go`,
which derive the last-column index from the seed, while `internal/view`, `internal/query`, `report`
and `internal/align` are unaffected either way. Feature 012 made the same choice for the same
reason.

Caveat on that measurement: `top` and `record` have tests that fail without live clusters
independently of this feature (`Test_getQueryReport`, `Test_app_setup`, `Test_tarRecorder`), so
"unaffected" means relative to that same baseline, not green in absolute terms.

**Alternatives considered:** renaming the existing constant to `PgStatActivityPG10` so `Default`
could name the new query (rejected — an earlier draft of this spec did exactly that, justified by a
convention that turned out not to exist; it churns five files to solve a problem the early return
solves for free); appending the new branch to the existing switch (rejected — leaves `Default`
naming a case that is no longer the default); raising the seed to 17 (rejected — changes nothing at
runtime and breaks tests that legitimately assert the pre-connect state).

### Decision 4: Empty values sort last in every comparator mode, including strings

**Decision:** `PGresult.sort` selects its mode from the first **non-empty** cell in the column, and
an empty cell orders after every non-empty one regardless of sort direction and regardless of
which comparator is in use — numeric, duration or string.

**Rationale:** two defects are being fixed, not one. Mode selection from row 0 is unreliable on a
sparse column; and inside the numeric comparator an empty string already parses to `0`, so today
blanks are not merely mis-ordered — they are indistinguishable from a genuine zero, and on
ascending sort they lead the screen. `horizon_xacts = 0` is a real state ("holds a snapshot taken
right now") that must not collide with "holds no snapshot".

Applying the rule to the string mode as well is the *simpler* implementation, not the broader one:
one wrapper around all three comparators, versus the same wrapper plus a carve-out. It also states
as one sentence with no exceptions — "an empty cell means no value, and rows without a value go
last" — which is what a future reader has to hold in their head. On the activity screen it
directly improves the common case: sorting by `wait_event` ascending currently leads with the
backends that are waiting on nothing.

**The string case deserves its own examination**, because there an empty value can be legitimate
rather than absent. `application_name` is the sharpest example: a client that sets none produces an
empty string, not NULL, so `''` is genuinely that backend's application name. Sorting it last is
still the right call — the operator scanning that column is looking for named applications, and a
block of nameless backends at the top is noise either way — but the honest framing is that this is
a **product choice about presentation**, not a correctness fix as it is for the numeric case.
Recorded so that a future reader does not mistake one for the other.

**Coverage gap, stated:** no golden covers a string-column sort on activity, so this half of the
change lands without regression cover. The targeted unit tests in Testing Strategy cover the rule
itself, and Task 6 carries an explicit acceptance step — sorting activity by a string column that
is blank for many rows — so the screen-level effect is checked by someone rather than assumed. An
earlier draft claimed that coverage without putting it in any task; it is named in Task 6 and in
the Acceptance Criteria now precisely so it cannot evaporate.

**Blast radius — enumerated, not gestured at.** This changes sort behaviour on every screen with a
sparse column. The known cases:

- **`replslots`, default sort key.** `OrderKey: 4` is `retained,KiB` — sparse and numeric, and it
  is the only default sort key in the repository with that property. The screen's SQL already
  declares `ORDER BY "retained,KiB" DESC NULLS LAST`
  (`internal/query/replication_slots.go:31`), so the change brings the Go comparator into
  agreement with what the query already asks for. This is the honest framing for the whole
  change: a correction, not a behaviour change invented here.
- **`activity.cl_port`.** Holds a real `-1` for unix-socket connections and blank for backends
  with no client. Today blanks parse to `0` and therefore sort *above* a genuine `-1`; after the
  change they move below it. Ordering changes on descending sort, and the new order is the correct
  one.
- **`procpidstat`.** Its IO and iodelay columns render blank when the availability probes fail, so
  its sort behaviour changes too. Note this carefully: the user-spec says the procpidstat screen
  is "not affected", and its *code* is not — but its sorting is, through this shared function.
  The statement in the user-spec is about columns and queries, and remains true in that sense.
- Any other screen whose column is blank for some rows — the rule is uniform by construction.

**Alternatives considered:** fixing only the sample selection (rejected — leaves blanks colliding
with genuine zeros, so the primary user story stays broken on ascending sort); restricting
empty-last to numeric and duration modes (rejected — more code, and an exception a maintainer would
have to re-justify); teaching the columns to emit a sentinel instead of blank (rejected — violates
the user-spec's "blank, never 0" rule at its source).

### Decision 5: Emptiness is decided by the rendered string, not by `sql.NullString.Valid`

**Decision:** the sort treats a cell as empty when its `String` is empty, not when `Valid` is
false.

**Rationale:** the codebase does carry nullness explicitly — `PGresult.Values` is
`[][]sql.NullString`, a NULL scans as `{String: "", Valid: false}`, and both fields survive the
archive round-trip because `sql.NullString` has no custom marshaller. So keying on `Valid` is
available and looks more precise.

It would nonetheless be wrong here, for two independent reasons.

**It would produce ordering nobody can explain.** Every render path prints `.String` alone, so a
SQL NULL and a genuine empty string are indistinguishable on screen. Sorting on `Valid` would order
two cells that look identical differently — blanks split between the top and the bottom with no
visible cause. The distinction is not hypothetical: `application_name` is an empty string rather
than NULL when a client sets none, while `backend_xid` is NULL. Both render blank.

**And `Valid` is not trustworthy across screens.** Inside the `DiffIntvl` range `diff()` sets
`Valid = true` unconditionally on the computed cell; outside the range it copies the source value
faithfully. So on any diffed screen `Valid` says "has a value" regardless of what the server
returned. It happens not to matter for `activity`, which never diffs — but the sort function is
shared, so a rule keyed on `Valid` would behave differently depending on whether the screen it runs
on diffs its columns. That is precisely the kind of invisible coupling the rule should avoid.

**Alternatives considered:** keying on `Valid` (rejected — produces visually unexplainable
ordering); rendering NULL and empty differently so `Valid` becomes visible (rejected — a screen-wide
display change nobody asked for, and it would collide with the user-spec's "blank, never 0" rule).

### Decision 6: A replayed version change must reset three states, and the render guard is restored

**Decision:** on a replayed version change reset the alignment flag, the header-repeat counter,
**and** the resolved sort-column index. Additionally restore the zero-width guard in the report
truncation path, mirroring `top/printDataCell`. Test the formatting function directly rather than
through the replay pipeline.

**Rationale:** three states survive the boundary today, not one.

1. **Alignment.** `Configure` does not clear it, so widths from the earlier layout are reused —
   this is tech debt [021] as registered.
2. **Header counter.** Resetting alignment alone recomputes widths but leaves the previous header
   on screen for another 20 rows, because the header is redrawn on a counter.
3. **Sort column.** `orderConfigured` is latched on the first sample and resolves the requested
   `-o` column name to an *index* against that sample's column list; the version-change branch
   never revisits it. The loud failure — a stale index exceeding a narrower row, so `PGresult.sort`
   reads past the end — needs the later layout to be narrower, which neither real boundary
   (9.6→10, 12→13) produces. **The quiet failure is the one that matters:** this feature inserts
   columns *mid-layout*, so after the boundary the same index denotes a different column and the
   report is silently sorted by something the operator did not ask for. A wrong answer with no
   symptom is worse than a crash, and it is reachable on exactly the archives this feature makes
   possible. Concretely, `state` is index 9 in the 14-column layout and index 10 in the
   17-column one, where index 9 is `wait_event` — so the mis-sort is demonstrable, not theoretical.

   **Clearing the latch is not sufficient.** If the requested column is absent from the new
   layout, `getColumnIndex` returns false, the latch simply stays down, and `OrderKey` keeps the
   index resolved against the *old* layout — which is the loud-failure path again. The reset must
   therefore restore the view's seed `OrderKey`/`OrderDesc`, captured once before the loop, rather
   than only re-arming the latch.

On the guard: an earlier draft of this spec rejected it as "treating the symptom". That was wrong,
and the reason is parity rather than defence in depth. `view.ColsWidth` is a `map[int]int`, so a
missing key yields `0` silently instead of failing — which is how a zero width becomes `[:-1]`
rather than an error. The twin of this code in `top/stat.go` already carries the guard, added after
a real crash (issue #99). Leaving `report/` as the only unguarded truncation point means keeping
two copies of the same code where one has learned the lesson and the other has not. The guard does
not replace the root fix; both land.

The test detail is load-bearing: the panic happens inside a goroutine, so a failing case takes down
the whole `go test` process instead of reddening one test. Driving the formatting function directly
keeps the red step an ordinary test failure — and note the consequence, that the pipeline route
itself stays uncovered, which is a further reason to keep the guard.

**Alternatives considered:** fixing only alignment as the register describes (rejected — leaves a
stale header and a stale sort index); guard only, no root fix (rejected — hides stale widths rather
than preventing them); leaving [021] in the register (rejected — this feature adds a boundary to
the affected path, and the failure mode is a crash).

### Decision 7: Cast only `backend_xid`

**Decision:** `backend_xid::text`; no cast on `age(backend_xmin)`.

**Rationale:** measured against a live server — `age(xid)` returns `integer`, which scans cleanly;
`xid` does not, hence the cast. A cast on the `age()` result would be noise a reader would have to
evaluate.

`replication.go` is a partial precedent only: it casts `backend_xmin::text::bigint`, but to do
arithmetic on the value rather than to make it scannable. The technique is established in the
codebase; the motivation here is different and worth stating so the two are not conflated.

**Alternatives considered:** casting both for visual symmetry (rejected — an unnecessary cast reads
as load-bearing and costs the next reader a lookup to discover it is not); casting neither and
relying on the driver (rejected — measured, `xid` does not scan into the string matrix).

### Decision 8: Documentation goes only to `report/describe.go`

**Decision:** the three new column descriptions and the four caveats are added to
`report/describe.go`. `internal/stat/help.go` is left untouched and recorded as tech debt.

**Rationale:** `internal/stat/help.go` has no consumers anywhere in the repository and is already
stale: in its **replication** block (`help.go:59`) it still calls the horizon column `xact_age*`,
where the live `report/describe.go:75` calls it `horizon_xacts`. Note precisely where that
staleness lives — `xact_age*` also appears in help.go's *activity* block, but there it legitimately
means the transaction's duration and has nothing to do with the horizon. Editing this file would
spread the new columns into dead code and imply it is live.

**Alternatives considered:** documenting in both files (rejected — doubles the surface and makes
dead code look maintained); deleting `internal/stat/help.go` in this feature (rejected — its
deadness is not this feature's doing, and a deletion deserves its own change; registered as debt
instead by Task 5).

## Data Models

No new types, no schema. The three columns are `text`/`integer` values arriving through the
existing `stat.PGresult` string matrix.

Column layout on PG 13+ (indices are load-bearing — the order is asserted against a live server):

| idx | column | source |
|-----|--------|--------|
| 0 | `pid` | `pid` |
| 1 | `leader` | `coalesce(leader_pid, pid)` |
| 2–10 | `cl_addr` … `state` | unchanged |
| 11 | `backend_xid` | `backend_xid::text` |
| 12 | `horizon_xacts` | `age(backend_xmin)` |
| 13–16 | `xact_age` … `query` | unchanged |

## Dependencies

### New packages

None.

### Using existing (from project)

- `internal/query` — version-branching selector idiom (`wal.go`, `bgwriter.go`, `io.go`)
- `internal/stat` — `PGresult.sort`, `sort.SliceStable` for deterministic ordering
- `report/` — synthetic in-memory tar archive pattern from feature [008] for the replay test
- `internal/postgres/testing.go` — `NewTestConnectVersion` for live-server assertions

## Testing Strategy

**Feature size:** M

### Unit tests

- `SelectStatActivityQuery` table test extended to pin the branch boundary from both sides:
  PG 12 → old query / 14 columns, PG 13 → new query / 17 columns. This is the only guard on the
  boundary, since no live PG 12 or 13 cluster exists.
- `TestViews_Configure` gains activity assertions on both sides of the boundary. The table test
  above proves the *selector* returns the right constant; it does not prove `Configure` carries it
  into the view. Feature 012 pinned its new boundary this way, and the file already has the
  `case 120000:` / `case 130000:` blocks — they currently speak only about `replication`.
- `PGresult.sort` on a sparse numeric column: correct numeric ordering regardless of which row is
  first; empty cells last in both directions; an empty cell never orders together with a genuine
  `0`.
- `PGresult.sort` on a sparse duration column and a sparse string column — the rule is uniform.
- `PGresult.sort` on a fully empty column — no-op, input order preserved.
- Report formatting across a version change: widths recomputed, header redrawn immediately, sort
  column re-resolved against the new column list — including the case where the requested column
  is absent from the later layout — and no panic. Driven against the formatting function directly,
  not the replay goroutine. The archive needs at least two samples on *each* side of the change:
  two before, or nothing latches and there is nothing to reset; two after, because the branch
  consumes the first.
- The zero-width guard in the truncation path: a width of zero or less returns an error and stops
  the cell from being printed, exactly as its twin in `top` does — not a silently empty cell.
- A replay case where a blank value meets a sparse **default** sort key — the empty
  `retained,KiB` path — since this is the only place the changed sort behaviour meets real
  recorded data. **The case must be constructed to actually diverge**, and this is easy to get
  wrong — two obvious constructions do not:
  - A lone blank under descending sort lands last under both old and new behaviour.
  - Putting the blank first so the old code falls into string mode only diverges if the remaining
    values *also* order differently lexicographically than numerically. `2048` before `1024` sorts
    the same either way; `512` vs `1024`, or `9` vs `1000000`, do not.

  So the case needs values that break lexicographic order, and if it relies on a genuine `"0"`
  sitting beside the blank, two further preconditions must hold: the first row must carry a
  non-empty numeric value (otherwise the old code picks string mode and the comparison changes
  shape), and the blank must sit above the `"0"` in the input.

  Any test written here must be shown to fail against the unfixed comparator, not merely to pass
  against the fixed one.
- Describe block ordering for the activity screen, following the existing progress-screen
  precedent.

### Integration tests

- Live-server assertion of column **names and their order** for the activity query on every
  available version (PG 14–19), replacing today's "the query does not error" check. This is what
  turns the claimed 17-column layout into a measured one, and it also closes the in-SQL half of
  the column-order risk.
- Replay of a synthetic two-version archive: at least two samples after the version change, since
  the replay path consumes the first sample of a new version and a shorter archive would leave the
  test green on an empty report.
- Existing golden replay tests must pass unchanged — but passing them is a **regression check, not
  coverage of the sort change**. The corpus sorts activity by `pid`, which is never blank, and its
  one sparse key is sorted in the only direction where old and new behaviour coincide. Coverage of
  the change therefore comes from a replay case where a blank value meets a sparse *default* sort
  key: the empty `retained,KiB` path in the replslots replay test.

### E2E tests

None — the project has no E2E harness for the TUI; screen behaviour is covered by acceptance
testing on live clusters.

## Agent Verification Plan

**Source:** user-spec "Как проверить" section.

### Verification approach

Automated suite first, then a live walk on the fixture clusters. The manual half exists because
three things cannot be asserted from Go: how the widened screen actually reads, whether blank
cells render blank rather than `0`, and whether the parallel-query group visually collapses.

### Per-task verification

| Task | verify: | What to check |
|------|---------|--------------|
| 1 | bash | `go test ./internal/query/...` — boundary pinned both sides; live name/order assertion green on PG 14–19 |
| 2 | bash | `go test ./internal/stat/... ./report/...` — sort rules hold; goldens still pass with the fix present |
| 3 | bash | `go test ./report/...` — two-version archive replays with correct widths and header, no panic |
| 4 | bash | `go test ./report/...` — describe order test green; `pgcenter report -d -A` shows all four caveats and the PG 13+ note |
| 5 | bash | register entries present in `docs/tech-debt.md` |
| 6 | bash + user | full QA per the user-spec "Как проверить" section, plus the string-column sort, the `replslots` default sort, and a few-hundred-session run |

### Tools required

bash (`make test`, `make lint`, `make vuln`, `go test`), a running fixture image for the live
assertions, and a terminal for the acceptance walk. No MCP tooling.

## Backward Compatibility

**Breaking changes:** no.

**Migration strategy:** none needed. The recorded archive format is untouched; `report/` derives
widths, column names and sort keys from the archive itself and never reads `view.Ncols`, so
archives written before 0.12 replay exactly as before. This was verified empirically by running the
report suite against the PG 14 golden archive with the widened query in place — **with the caveat
that the measurement predates the sort change**, which must therefore be re-measured with the fix
applied (Testing Strategy explains why an unchanged golden is weak evidence here).

**DB migration compatibility:** N/A — pgcenter reads statistics views and owns no schema.

**Consumer impact:** the sort fix changes ordering on every screen with a sparse column, most
visibly `replslots` sorted by its default key. No API or exported signature changes:
`SelectStatActivityQuery` keeps its shape, and `PGresult.sort` is unexported.

## Risks

| Risk | Mitigation |
|------|-----------|
| Branch written as `< 140000` instead of `< 130000` — every live check would still pass | Table test pins the boundary from both sides; called out in Decision 2 |
| Sort fix silently changes ordering on unrelated screens | Blast radius enumerated case by case in Decision 4. **The existing goldens do not cover this** — see the row below — so coverage comes from a targeted test on a sparse default sort key, plus the manual `replslots` check in Task 6 |
| Passing goldens are mistaken for evidence that the sort change is safe | Every `report_activity*` golden sorts by `pid`, which is never blank, and the one sparse key in the corpus (`datname`) is sorted descending — the single direction where old and new behaviour agree. Unchanged goldens therefore prove the corpus never exercises the change, not that the change is harmless. The replay case with an empty `retained,KiB` is the one place where the new behaviour meets a default sort key, and it is what must be asserted |
| Column order in the SQL drifts from the specified layout | Asserted against a live server on PG 14–19; residual exposure only on PG 12/13, where the spec text is the sole source |
| Two-version archive test passes on an empty report | Archive must carry ≥2 samples after the version change; the replay path consumes the first |
| `[021]` fix appears to work but leaves a stale header | Both resets required — alignment flag *and* header-repeat counter (Decision 6) |
| Blank cells read as "holds no horizon" when the real cause is missing privileges | An explicit fourth caveat in `describe.go`, named in both the Acceptance Criteria and Task 4 so it cannot be dropped as an unlisted extra. `pg_stat_activity` returns NULL rather than an error, so there is nothing to trap in code. The stake is real: this screen is where a DBA decides whether to terminate a backend |
| Stale sort index survives a replayed version change and indexes past the end of a narrower row | Third reset in Decision 6, alongside alignment and the header counter |

## Acceptance Criteria

- [ ] `SelectStatActivityQuery` returns the 17-column query on PG 13+ and the unchanged
      14-column query on PG 10–12, pinned by table test on both sides of the boundary
- [ ] Live servers PG 14–19 return exactly the specified column names in the specified order
- [ ] `DiffIntvl` and `UniqueKey` for `activity` are unchanged; the selector keeps its 2-tuple shape
- [ ] `view.New()` seed untouched; `internal/view`, `top`, `record` suites green
- [ ] Sorting a sparse column is numeric regardless of the first row; empty cells last in both
      directions; empty never collides with a genuine `0`
- [ ] Sorting a fully empty column is a no-op preserving input order
- [ ] A SQL NULL and a genuine empty string sort together — cells that render identically are
      never split between the top and the bottom of the screen
- [ ] Existing golden replay tests pass **with the sort fix applied**
- [ ] A two-version synthetic archive replays with recomputed widths, an immediately redrawn
      header, a re-resolved sort column, and no panic
- [ ] The report truncation path carries a zero-width guard, matching its twin in `top` — an error
      that stops the cell being printed, not a silently empty cell
- [ ] Sorting the activity screen by a string column that is blank for many rows behaves as
      specified, verified by hand — this half of the sort change has no automated cover
- [ ] `leader`, `backend_xid` and `horizon_xacts` carry the semantics the user stories rely on:
      the group identifier collapses a parallel query, a blank `backend_xid` means the transaction
      has not written, and `horizon_xacts` ranks sessions by how far back they hold the horizon
- [ ] `report -d -A` lists the three columns, notes that they require PG 13+, and carries four
      caveats: `leader` is derived rather than the raw `leader_pid`; the horizon covers backend
      sources only; `horizon_xacts` is computed differently here than on the `replication` screen;
      and a blank cell may mean the viewer lacks the privileges to see another session's state,
      not that the session holds nothing
- [ ] Three entries added to `docs/tech-debt.md`; `[021]` moved to Resolved Debt; the "why
      deferred" text of `[020]` corrected; the Project Knowledge sentence describing the activity
      selector's version branches brought up to date
- [ ] `make test`, `make lint`, `make vuln` green

## Implementation Tasks

### Wave 1 (независимые)

#### Task 1: PG 13+ activity query branch
- **Description:** Give the `activity` screen its three new columns on modern PostgreSQL by adding
  a version branch to the query selector, in the layout and under the names fixed in Data Models
  and Decisions 1–3. Extend the query tests so the branch boundary and the resulting column list
  are both actually asserted rather than assumed.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/query/... ./internal/view/...`
- **Files to modify:** `internal/query/activity.go`, `internal/query/activity_test.go`,
  `internal/query/query.go`, `internal/view/view_test.go`
- **Files to read:** `internal/query/replication.go`, `internal/query/wal.go`,
  `internal/query/progress_vacuum.go`, `internal/query/progress_vacuum_test.go`,
  `internal/query/bgwriter_test.go`, `internal/view/view.go`

#### Task 2: Sort empty-last and non-empty mode selection
- **Description:** Make sorting of sparse columns correct per Decisions 4 and 5, so that a blank
  cell never collides with a genuine zero and the feature's primary user story — ranking sessions
  by how far back they hold the horizon — actually works. This touches machinery shared by every
  screen, so the change has to be shown not to disturb existing replay output.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/stat/... ./report/...` (needs live fixture clusters:
  without them `./internal/stat/...` panics rather than failing — a nil-pointer in
  `postgres.DB.Close` after a failed connect, the family of the resolved [005]/[008], not [019])
- **Files to modify:** `internal/stat/postgres.go`, `internal/stat/postgres_test.go`,
  `report/report_record_replslots_test.go`
- **Files to read:** `internal/query/replication_slots.go`, `internal/view/view.go`,
  `.claude/skills/project-knowledge/patterns.md`

#### Task 3: Recompute report layout on a mid-archive version change
- **Description:** Close tech debt [021] so an archive spanning a major-version upgrade replays
  with a correct layout instead of a stale one, and restore the zero-width guard in the truncation
  path so a width mismatch fails loudly rather than crashing the process — both per Decision 6.
  Cover it with a synthetic two-version archive; note that the archive needs at least two samples
  after the version change, because the replay path consumes the first, and that the test drives
  the formatting function directly so a failure reddens a test instead of taking down the run.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./report/...`
- **Files to modify:** `report/report.go`, `report/report_test.go`
- **Files to read:** `internal/align/align.go`, `top/stat.go`, `docs/tech-debt.md`

### Wave 2 (зависит от Wave 1)

#### Task 4: Describe the new columns and their caveats
- **Description:** Document the three new columns where `report -d -A` will show them, including
  the four caveats listed in Acceptance Criteria and a note that the columns require PG 13+, as the
  house style of that file does for every version-dependent column. An operator must be able to
  tell what each column means, and what a blank one does *not* prove, without reading the query.
  Pin the ordering of the description lines with a test, following the existing progress-screen
  precedent.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./report/...` and `pgcenter report -d -A`
- **Files to modify:** `report/describe.go`, `report/report_test.go`
- **Files to read:** `internal/query/activity.go`, `internal/query/replication.go`

#### Task 5: Register the three findings as tech debt
- **Description:** Record the debt this feature surfaced but deliberately did not fix: the dead and
  stale `internal/stat/help.go`, the divergent formulas behind the same `horizon_xacts` name on two
  screens, and the gap between the test port map and the versions actually present in the test
  image. Mark [021] resolved, and correct the "why deferred" text of [020], which rests on a reason
  this feature has shown to be incomplete. Also refresh the Project Knowledge sentence describing
  the activity selector's version branches, which this feature makes stale.
- **Skill:** documentation-writing
- **Reviewers:** dev-code-reviewer
- **Verify:** bash — `grep -n "help.go" docs/tech-debt.md`, `grep -n "\[021\]" docs/tech-debt.md`
  shows it under Resolved Debt, and `grep -n "SelectStatActivityQuery" .claude/skills/project-knowledge/architecture.md`
  no longer says "branches at PG 9.6, PG 10"
- **Files to modify:** `docs/tech-debt.md`,
  `.claude/skills/project-knowledge/architecture.md`
- **Files to read:** `internal/stat/help.go`, `internal/postgres/testing.go`,
  `internal/query/replication.go`, `internal/query/activity.go`

### Final Wave

#### Task 6: Pre-deploy QA
- **Description:** Acceptance testing against the user-spec "Как проверить" section and this
  spec's Acceptance Criteria. Automated suite plus a manual walk on the fixture clusters, which
  must additionally cover three things the user-spec's own checklist does not: sorting the activity
  screen by a **string** column that is blank for many rows, which is the half of the sort change
  no automated test covers; the `replslots` default sort, the screen most visibly affected by that
  change; and the widened activity screen under a few hundred sessions, so the "do not make an
  incident worse" rule is checked rather than argued.
- **Skill:** pre-deploy-qa
- **Reviewers:** none
- **Verify:** bash + user — full QA per the user-spec "Как проверить" section
- **Files to modify:** none
- **Files to read:** `docs/features/013-feat-activity-xmin-horizon/013-feat-activity-xmin-horizon.md`,
  `docs/features/013-feat-activity-xmin-horizon/013-feat-activity-xmin-horizon-tech-spec.md`
