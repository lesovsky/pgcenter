---
created: 2026-07-25
status: draft
branch: feature/activity-xmin-horizon
size: M
---

# Tech Spec: Activity screen — xmin horizon and parallel worker grouping

## Solution

Add a PG-13+ branch to the `activity` query carrying three new columns — `leader`,
`backend_xid`, `horizon_xacts` — inserted at fixed positions in the existing column list. The
existing `PgStatActivityDefault` becomes the PG 10–12 branch, unchanged.

Two defects that this feature walks into are fixed alongside it:

1. **Sort-mode detection** (`internal/stat/postgres.go`) picks the comparator by inspecting a
   single cell and lets empty cells parse as `0`. Sparse columns therefore sort wrongly, which
   breaks the feature's primary user story.
2. **Tech debt [021]** — column widths are not recomputed when an archive's recorded PG version
   changes mid-replay. This feature adds a second version boundary to that path, and the
   failure mode is a panic, not a cosmetic defect.

Everything else the feature needs already exists: `view.Configure` already calls the activity
selector, `report/` never reads `view.Ncols`, and horizontal scroll already removed the width
constraint on this screen.

## Architecture

### What we're building/modifying

- **`internal/query/activity.go`** — new `PgStatActivity13` constant; `SelectStatActivityQuery`
  gains a `version < 130000` branch. Signature stays `(string, int)`.
- **`internal/stat/postgres.go`** — `PGresult.sort` gains non-empty sample selection and
  empty-last ordering across all three comparator modes.
- **`report/report.go`** — reset the alignment flag and the header-repeat counter when a replayed
  sample's PG version changes.
- **`report/describe.go`** — three new column descriptions plus three caveats in the activity block.
- **`docs/tech-debt.md`** — three new register entries.

Not modified: `internal/view/view.go` (the `Configure` wiring already exists and the seed must
stay at 14 — see Decision 3), `internal/stat/help.go` (dead code — see Decision 7),
`internal/align/`, the `procpidstat` screen, the `replication` screen.

### How it works

At connect time `view.Configure(opts)` calls `SelectStatActivityQuery(opts.Version)`, which now
returns the 17-column query on PG 13+ and the unchanged 14-column query below it. The collector,
diff engine and renderer need no changes: `activity` has `DiffIntvl {0,0}`, so `calculateDelta`
takes the pass-through branch and never calls `diff()`; `UniqueKey` stays 0 because `pid` remains
column 0.

The three columns are produced entirely in SQL:

- `coalesce(leader_pid, pid) AS leader`
- `backend_xid::text` — explicit cast, matching the `replication.go` precedent
- `age(backend_xmin) AS horizon_xacts` — returns `integer`, no cast needed

NULL reaches the renderer as an empty string because no `coalesce` is applied to the xid columns;
this is what satisfies the "blank, never 0" requirement.

Sorting is where the new columns interact with shared machinery. `PGresult.sort` currently reads
`r.Values[0][key]` to choose between numeric, duration and string comparators. On a sparse column
that first cell is usually empty, so the choice is effectively random, and inside the numeric
comparator `strconv.ParseFloat("")` fails silently and yields `0`. Both halves are fixed: the
sample is taken from the first non-empty cell, and empty cells are ordered last independently of
the comparator and of the sort direction.

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

### Decision 3: Leave the `view.New()` seed at 14 columns

**Decision:** the `activity` entry in `view.New()` keeps `QueryTmpl: query.PgStatActivityDefault`
and `Ncols: 14`.

**Rationale:** `Configure` overwrites both at connect time, so the seed is a placeholder, not a
fact. Measured: leaving it alone keeps `internal/view`, `internal/query`, `report`, `top`,
`record` and `internal/align` green; raising it to 17 breaks exactly two assertions in
`top/config_view_test.go`. Feature 012 made the same choice for the same reason.

**Alternatives considered:** raising the seed to 17 for cosmetic consistency (rejected — it
changes nothing at runtime and breaks tests that legitimately assert the pre-connect state).

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

**Blast radius, stated deliberately:** this changes sort behaviour on every screen with a sparse
column. The most visible case is `replslots`, whose default sort key `OrderKey: 4` is
`retained,KiB` — sparse and numeric. That screen's SQL already declares `ORDER BY "retained,KiB"
DESC NULLS LAST` (`internal/query/replication_slots.go:31`), so the change brings the Go
comparator into agreement with what the query already asks for. That is the honest framing: a
correction, not a behaviour change invented here.

**Alternatives considered:** fixing only the sample selection (rejected — leaves blanks colliding
with genuine zeros, so the primary user story stays broken on ascending sort); restricting
empty-last to numeric and duration modes (rejected — more code, and an exception a maintainer would
have to re-justify); teaching the columns to emit a sentinel instead of blank (rejected — violates
the user-spec's "blank, never 0" rule at its source).

### Decision 5: Tech debt [021] needs two lines, and its test bypasses the goroutine

**Decision:** on a replayed version change, reset both the alignment flag and the header-repeat
counter. Test the formatting function directly rather than through the replay pipeline.

**Rationale:** resetting alignment alone recomputes the widths but leaves the previous header on
screen for another 20 rows, because the header is redrawn on a counter. Both resets are needed for
the output to be correct.

The test detail is load-bearing: the panic (`slice bounds out of range [:-1]`, from a zero width
reaching a `[:width-1]` slice) happens inside a goroutine, so a failing case takes down the whole
`go test` process instead of reddening one test. Driving the formatting function directly keeps
the red step an ordinary test failure.

**Alternatives considered:** adding a zero-width guard mirroring `top/printDataCell` (rejected —
treats the symptom; the widths should not be stale in the first place, and the guard would hide
the next instance); leaving [021] in the register (rejected — this feature adds a second boundary
to the affected path, and the failure mode is a crash).

### Decision 6: Cast only `backend_xid`

**Decision:** `backend_xid::text`; no cast on `age(backend_xmin)`.

**Rationale:** measured against a live server — `age(xid)` returns `integer`, which scans cleanly;
`xid` does not, hence the cast, matching `replication.go:28`. A cast on the `age()` result would be
noise a reader would have to evaluate.

### Decision 7: Documentation goes only to `report/describe.go`

**Decision:** the three new column descriptions and the three caveats are added to
`report/describe.go`. `internal/stat/help.go` is left untouched and recorded as tech debt.

**Rationale:** `internal/stat/help.go` has no consumers anywhere in the repository and is already
stale — it still calls `horizon_xacts` by its old name. Editing it would spread the new columns
into dead code and imply it is live.

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
- `PGresult.sort` on a sparse numeric column: correct numeric ordering regardless of which row is
  first; empty cells last in both directions; an empty cell never orders together with a genuine
  `0`.
- `PGresult.sort` on a sparse duration column and a sparse string column — the rule is uniform.
- `PGresult.sort` on a fully empty column — no-op, input order preserved.
- Report formatting across a version change: widths recomputed, header redrawn immediately, no
  panic. Driven against the formatting function directly, not the replay goroutine.
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
- Existing golden replay tests must pass unchanged — but they have to be re-run **with** the sort
  fix present, since the earlier measurement predates it.

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
| 4 | bash | `go test ./report/...` — describe order test green; `pgcenter report -d -A` shows the three caveats |
| 5 | bash | register entries present in `docs/tech-debt.md` |
| 6 | bash + user | full QA per the user-spec "Как проверить" section |

### Tools required

bash (`make test`, `make lint`, `make vuln`, `go test`), a running fixture image for the live
assertions, and a terminal for the acceptance walk. No MCP tooling.

## Backward Compatibility

**Breaking changes:** no.

**Migration strategy:** none needed. The recorded archive format is untouched; `report/` derives
widths, column names and sort keys from the archive itself and never reads `view.Ncols`, so
archives written before 0.12 replay exactly as before. This was verified empirically by running the
report suite against the PG 14 golden archive with the widened query in place.

**DB migration compatibility:** N/A — pgcenter reads statistics views and owns no schema.

**Consumer impact:** the sort fix changes ordering on every screen with a sparse column, most
visibly `replslots` sorted by its default key. No API or exported signature changes:
`SelectStatActivityQuery` keeps its shape, and `PGresult.sort` is unexported.

## Risks

| Risk | Mitigation |
|------|-----------|
| Branch written as `< 140000` instead of `< 130000` — every live check would still pass | Table test pins the boundary from both sides; called out in Decision 2 |
| Sort fix silently changes ordering on unrelated screens | Blast radius enumerated in Decision 4; goldens re-run with the fix present; `replslots` framed against its own SQL, which already declares NULLS LAST |
| Column order in the SQL drifts from the specified layout | Asserted against a live server on PG 14–19; residual exposure only on PG 12/13, where the spec text is the sole source |
| Two-version archive test passes on an empty report | Archive must carry ≥2 samples after the version change; the replay path consumes the first |
| `[021]` fix appears to work but leaves a stale header | Both resets required — alignment flag *and* header-repeat counter (Decision 5) |
| Blank cells read as "holds no horizon" when the real cause is missing privileges | Documented caveat in `describe.go`; `pg_stat_activity` returns NULL rather than an error, so there is nothing to trap |

## Acceptance Criteria

- [ ] `SelectStatActivityQuery` returns the 17-column query on PG 13+ and the unchanged
      14-column query on PG 10–12, pinned by table test on both sides of the boundary
- [ ] Live servers PG 14–19 return exactly the specified column names in the specified order
- [ ] `DiffIntvl` and `UniqueKey` for `activity` are unchanged; the selector keeps its 2-tuple shape
- [ ] `view.New()` seed untouched; `internal/view`, `top`, `record` suites green
- [ ] Sorting a sparse column is numeric regardless of the first row; empty cells last in both
      directions; empty never collides with a genuine `0`
- [ ] Sorting a fully empty column is a no-op preserving input order
- [ ] Existing golden replay tests pass **with the sort fix applied**
- [ ] A two-version synthetic archive replays with recomputed widths, an immediately redrawn
      header, and no panic
- [ ] `report -d -A` lists the three columns and carries the three caveats
- [ ] Three entries added to `docs/tech-debt.md`
- [ ] `make test`, `make lint`, `make vuln` green

## Implementation Tasks

### Wave 1 (независимые)

#### Task 1: PG 13+ activity query branch
- **Description:** Add the PG-13+ query constant carrying `leader`, `backend_xid` and
  `horizon_xacts` at the positions fixed in this spec, and branch the selector at 130000 so
  PG 10–12 keeps today's query. Pin the boundary from both sides in the table test, and upgrade the
  live query test so it asserts column names and their order instead of merely checking the query
  runs.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/query/... ./internal/view/...`
- **Files to modify:** `internal/query/activity.go`, `internal/query/activity_test.go`
- **Files to read:** `internal/query/replication.go`, `internal/query/bgwriter.go`,
  `internal/query/progress_vacuum_test.go`, `internal/view/view.go`

#### Task 2: Sort empty-last and non-empty mode selection
- **Description:** Fix `PGresult.sort` to choose its comparator from the first non-empty cell and
  to order empty cells after all non-empty ones in every mode and both directions, so a blank never
  collides with a genuine zero. This is a shared-engine change: re-run the report goldens with it
  in place, since the earlier compatibility measurement was taken without it.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/stat/... ./report/...`
- **Files to modify:** `internal/stat/postgres.go`, `internal/stat/postgres_test.go`
- **Files to read:** `internal/query/replication_slots.go`, `internal/view/view.go`,
  `.claude/skills/project-knowledge/patterns.md`

#### Task 3: Recompute report layout on a mid-archive version change
- **Description:** Close tech debt [021] — when a replayed sample's PG version changes, reset both
  the alignment flag and the header-repeat counter so widths are recomputed and the header is
  redrawn immediately instead of 20 rows later. Cover it with a synthetic two-version archive
  carrying at least two samples after the change, driving the formatting function directly so the
  failure is a test failure rather than a panic inside a goroutine.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./report/...`
- **Files to modify:** `report/report.go`, `report/report_test.go`
- **Files to read:** `internal/align/align.go`, `top/stat.go`, `docs/tech-debt.md`

### Wave 2 (зависит от Wave 1)

#### Task 4: Describe the new columns and their caveats
- **Description:** Add the three columns to the activity describe block with the three caveats the
  user-spec requires: `leader` is derived rather than the raw `leader_pid`, the horizon covers only
  backend sources, and `horizon_xacts` is computed differently here than on the replication screen.
  Pin the ordering of the description lines with a test following the existing progress-screen
  precedent.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./report/...` and `pgcenter report -d -A`
- **Files to modify:** `report/describe.go`, `report/report_test.go`
- **Files to read:** `internal/query/activity.go`, `internal/query/replication.go`

#### Task 5: Register the three findings as tech debt
- **Description:** Record the debt this feature surfaced but deliberately did not fix: the dead and
  stale `internal/stat/help.go`, the divergent formulas behind the same `horizon_xacts` name on two
  screens, and the gap between the test port map and the versions actually present in the test
  image. Mark [021] resolved.
- **Skill:** documentation-writing
- **Reviewers:** dev-code-reviewer
- **Verify:** bash — entries present in `docs/tech-debt.md`
- **Files to modify:** `docs/tech-debt.md`
- **Files to read:** `internal/stat/help.go`, `internal/postgres/testing.go`,
  `internal/query/replication.go`

### Final Wave

#### Task 6: Pre-deploy QA
- **Description:** Acceptance testing against the user-spec and this tech-spec. Automated: full
  suite, lint, vulnerability check. Manual on the fixture clusters: the widened activity screen on
  PG 14–19, blank-versus-zero rendering, a parallel query collapsing into one `leader` group, an
  idle-in-transaction session holding the horizon with and without a write, sorting behaviour on
  the sparse columns in both directions, and the `replslots` default sort after the shared sort
  change.
- **Skill:** pre-deploy-qa
- **Reviewers:** none
- **Verify:** bash + user — full QA per the user-spec "Как проверить" section
- **Files to modify:** none
- **Files to read:** `docs/features/013-feat-activity-xmin-horizon/013-feat-activity-xmin-horizon.md`,
  `docs/features/013-feat-activity-xmin-horizon/013-feat-activity-xmin-horizon-tech-spec.md`
