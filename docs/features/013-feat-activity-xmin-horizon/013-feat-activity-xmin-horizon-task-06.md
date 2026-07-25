---
status: planned
depends_on: ["01", "02", "03", "04", "05"]
wave: 3
skills: [pre-deploy-qa]
verify: "bash + user — full QA per the user-spec «Как проверить» section, plus the string-column sort, the replslots default sort, a measured few-hundred-session run, and a hand check of the tech-debt register"
reviewers: []
teammate_name:
---

# Task 06: Pre-deploy QA

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:pre-deploy-qa` — [skills/pre-deploy-qa/SKILL.md](~/.claude/skills/pre-deploy-qa/SKILL.md)

## Description

Final acceptance gate for the feature. Tasks 1–5 proved that the code compiles, that the unit tables say
the right thing and that the goldens are unchanged. This task proves that a DBA sitting in front of
`pgcenter top` on a live cluster actually gets the answer the feature promises — which is the whole point,
because the feature's value is a **reading** of the activity screen, and no Go assertion can produce one.

The task has two halves.

**Automated half.** `make test`, `make lint`, `make vuln`, plus a deliberate re-run of `go test ./report/...`.
That last one is not redundant with Task 2: the tech-spec's Backward Compatibility section states plainly
that the "archives replay byte-identically" measurement was taken on a tree **without** the sort fix, and
the sort fix touches the shared engine. Confirming the goldens still pass *with the fix present* is the
step that turns a stale measurement into a current one. The tech-spec is equally explicit that a green
golden run is **weak** evidence here — the corpus sorts activity by `pid`, which is never blank, and its
one sparse key is sorted in the single direction where old and new behaviour coincide — so treat it as a
regression check, not as coverage.

**Manual half.** A walk on the fixture clusters. Three things cannot be asserted from Go at all: whether
the widened screen still reads, whether a blank cell renders blank rather than `0`, and whether the
parallel-query group visually collapses into one block. The user-spec's «Как проверить» table is the
baseline for this walk.

Beyond that table, this task carries **three checks the user-spec's own checklist does not contain**, each
added because the tech-spec identifies it as an uncovered edge:

- **Sorting the activity screen by a *string* column that is blank for many rows.** Decision 4 applies the
  empty-last rule to the string comparator too, and states in as many words that no golden covers a
  string-column sort on activity — this half of the change lands with no automated cover. `wait_event` and
  `wait_etype` are the natural subjects: blank on every backend that is not waiting, which on an idle
  fixture cluster is most of them. This is also the one place where the change is a **product choice about
  presentation** rather than a correctness fix (an empty `application_name` is genuinely that backend's
  name, not a missing value), so a human judging that it reads well is exactly the right instrument.
- **The `replslots` default sort.** `OrderKey: 4` is `retained,KiB` — per Decision 4 the only default sort
  key in the repository that is both sparse and numeric, which makes `replslots` the screen where the
  shared sort change is most visible without anyone asking for it. The screen's own SQL already declares
  `ORDER BY "retained,KiB" DESC NULLS LAST`, so the expected outcome is that the Go ordering now agrees
  with what the query was already asking for.
- **The widened activity screen under a few hundred sessions.** The roadmap rule is "do not make an
  incident worse": this screen is the one a DBA opens *during* an incident, and an incident is exactly when
  there are hundreds of sessions. Three extra columns plus a sort that now scans a column for its first
  non-empty cell should cost nothing perceptible — but that should be **checked rather than argued**.

**Honest limit, to be recorded rather than worked around.** The PG 12/13 branch boundary — the single most
consequential line in Task 1 — **cannot be verified live**: the `pgcenter-testing` image carries only
PG 14–19, so writing `version < 140000` instead of `version < 130000` would pass every live check in this
task. The boundary rests entirely on the table test from Task 1 (Decision 2 says this in the same words).
Do not report it as verified, and do not report it as failed; report it as resting on the table test, and
confirm that the table test pins the boundary from **both** sides.

Any problem a manual check reveals is **routed through the tech-spec's Risks table, not silently
accepted** — see the routing rule in Details.

## What to do

### A. Environment

1. **Bring up the fixture clusters.** The `pgcenter-testing` image does not come up ready: its `CMD` exits
   immediately, and the clusters, the `pgcenter_fixtures` database and the Go/lint toolchain are all
   created by explicit steps at run time — CI does exactly this, and `.github/workflows/default.yml` is the
   source of truth for the image tag (read it; do not assume a tag). Start the container detached with a
   long-lived command, publish the cluster ports bound to loopback, mount the repo checkout, then run
   `testing/prepare-test-environment.sh` inside it. Verify every cluster in the image accepts connections
   before running anything.

   Do **not** rebuild the image from `testing/Dockerfile` — the published image is the artifact CI verifies
   against. See the `pre-deploy-qa` deviation note in Details for how this maps onto the skill's Phase 1.
2. Build and install the binary from the mounted checkout (`make build && make install`) so `pgcenter` is on
   `PATH` inside the container. The manual walk has to happen **in-container**, against a local connection.

### B. Automated verification

3. Run `make test`, `make lint`, `make vuln`. All must be clean.
4. Run `go test ./report/...` explicitly and confirm the existing goldens pass **with the sort fix in the
   tree**. Confirm via `git status` / `git diff` that no pre-existing golden under `report/testdata/` was
   regenerated — a golden diff here is a red flag, not expected churn. Goldens legitimately added by
   Tasks 2 and 3 are the exception; identify them from the decisions log rather than assuming.
5. Confirm the PG 14–19 subtests **executed** rather than skipped. `t.Skipf` on an unreachable cluster is
   the normal mechanism, so a green suite with a stopped cluster is also green, and `make test` is not
   verbose enough to tell the difference. Run a verbose, filtered invocation over `./internal/query/...`
   (`Test_StatActivityQueries`) and read the per-version subtest lines.

   The expected picture is **mixed, not uniformly green**: the test iterates `90500 … 190000`, and the
   image carries only PG 14–19, so the split is fixed by the environment —

   - `pg_stat_activity/140000` … `pg_stat_activity/190000` — must show `--- PASS`. Six of them, all six.
   - `pg_stat_activity/90500`, `90600`, `100000`, `110000`, `120000`, `130000` — **expected** `--- SKIP`
     (`postgres N not available in test environment`). These are not failures and not something to chase:
     no such clusters exist in the image, which is the same fact that makes the PG 12/13 boundary
     non-live-verifiable. Record them as expected skips.

   A `--- SKIP` on any of PG 14–19 *is* a finding — it means a cluster that should be up is not, and the
   suite went green without touching that version.
6. Confirm Task 1's live assertion actually compares **column names and their order** against the server,
   not just the column count and not just "the query did not error" — that assertion is what makes the
   17-column layout a measured fact on PG 14–19 rather than a claim.

### C. Manual walk — the user-spec checklist

7. **Column layout.** Open `pgcenter top` on each available version and confirm the activity screen shows
   the 17 columns in the exact order fixed in the tech-spec's Data Models table and the user-spec's layout
   table: `pid, leader, cl_addr, cl_port, datname, usename, appname, backend_type, wait_etype, wait_event,
   state, backend_xid, horizon_xacts, xact_age, query_age, change_age, query`. Use `[` / `]` to scroll and
   capture the whole row. Any deviation from that order is a finding, however sensible the alternative looks.
8. **Horizon holder.** In a second session run `BEGIN; SELECT ...;` and leave it idle in transaction.
   Confirm on the activity screen that `horizon_xacts` is **non-empty** and `backend_xid` is **empty** for
   that pid.
9. **Writing transaction.** In the *same* still-open transaction run an `INSERT`, then confirm
   `backend_xid` becomes non-empty for that pid. The two states have to be observed on one session in
   sequence — that ordering is the whole point of the column, and observing them on two different sessions
   would not distinguish "this transaction wrote" from "this is a different backend".
10. **Parallel group.** Set `max_parallel_workers_per_gather` and run a heavy sequential scan so workers are
    spawned. Confirm several rows share one `leader` value, that the leader's own `leader` equals its `pid`,
    and that sorting by `leader` places the leader and its workers **contiguously**. Expected and **not** a
    finding: the workers show an empty `backend_xid` even when the transaction is writing (the xid belongs
    to the leader), and all of them show the *same* `horizon_xacts` as the leader (workers inherit the
    leader's snapshot — the user-spec lists this as an edge case, not a defect).
11. **Blank is never zero.** On rows where `backend_xid` / `horizon_xacts` have no value, confirm the cell
    renders as blank — no `0`, no dash, no `unknown`. Then confirm the converse is reachable: a session
    holding a snapshot taken right now shows `horizon_xacts = 0`, which is a *different* state from blank
    and must be visibly different on screen.
12. **Numeric sort on a sparse column.** With sessions producing a mix of populated and blank
    `horizon_xacts`, sort by that column (`Left`/`Right` move the sort column, `<` toggles direction).
    Confirm: the ordering is numeric, not lexicographic (`1000000` above `9`, not below); it does not depend
    on which row happens to be first; and the blanks sit at the **end in both directions** and never mingle
    with a genuine `0`.
13. **Describe output.** Run `pgcenter report -d -A` and confirm it lists the three new columns, carries the
    PG 13+ note, and carries **all four** caveats from the tech-spec's Acceptance Criteria: `leader` is
    derived rather than the raw `leader_pid`; the horizon covers backend sources only; `horizon_xacts` is
    computed differently here than on the `replication` screen; and a blank cell may mean the viewer lacks
    the privilege to see another session's state rather than that the session holds nothing. Three caveats
    plus the note is a **finding** — the privileges caveat is the one at risk of being dropped as an
    unlisted extra, and it is the one that matters most, because this screen is where a DBA decides whether
    to terminate a backend.

### D. The three checks the user-spec does not carry

14. **String-column sort with many blanks.** On the activity screen sort by `wait_event` (or `wait_etype`)
    in **both** directions on a cluster where most backends are not waiting. Confirm the blank rows go last
    in both directions and that the named values remain correctly ordered among themselves. Then judge, as a
    human, whether the screen reads better or worse than before — this is a presentation choice, so record
    the judgement, not just a pass/fail.
15. **`replslots` default sort.** Open the `replslots` screen (its default sort key is `retained,KiB`,
    column 4) with at least one slot that has reserved WAL and one that has not, so the column is genuinely
    sparse. Confirm the empty rows sit at the bottom under the default DESC ordering **and** stay at the
    bottom after toggling to ASC — the latter is the direction that changed. Cross-check against
    `pg_replication_slots` in `psql`: the screen's own SQL asks for `NULLS LAST`, so the screen should now
    agree with the query. Also look at `safe,KiB`, which is empty for **every** row on a stock cluster
    (`max_slot_wal_keep_size = -1` is the default): sorting by an all-empty column must be a no-op that
    preserves the existing order, not a reshuffle and not a crash.
16. **A few hundred sessions.** Open several hundred idle and idle-in-transaction sessions against one
    cluster, then open the activity screen. Confirm it renders, that scrolling with `[` / `]` and switching
    the sort column keep working, and that sorting by `horizon_xacts` at that row count produces the same
    correct ordering as at ten rows.

    Do not settle for "felt fine" — measure two things:

    - **The screen keeps its refresh interval.** The sample updates once per configured interval with no
      visible stall between redraws. Record the interval used and how the observation was made (a capture
      loop with timestamps is enough); a redraw that visibly stretches past the interval is a finding.
    - **The activity query is not the slowest thing on the screen.** Time the screen's own SQL in `psql`
      at that session count and compare it against the other screens' queries timed the same way. It must
      stay comfortably inside the refresh interval and must not become the most expensive query pgcenter
      runs. Report the numbers, not an impression.

### E. Report

17. Produce the QA report per the `pre-deploy-qa` skill, covering **every** acceptance criterion from both
    the user-spec and the tech-spec, with evidence per criterion. Criteria that only the table test can
    reach — the PG 12/13 boundary above all — get an explicit `not_verifiable` verdict naming the test that
    carries them, not a `passed`.
18. Route any problem found per the rule in Details and name every routed item explicitly in the report.

## Acceptance Criteria

- [ ] `make test`, `make lint`, `make vuln` clean.
- [ ] `go test ./report/...` green **with the sort fix present in the tree**, and no pre-existing golden
      regenerated (`git status` clean under `report/testdata/` apart from goldens Tasks 2–3 legitimately
      added). Recorded as a regression check, not as coverage of the sort change.
- [ ] In verbose `Test_StatActivityQueries` output all six PG 14–19 subtests show `--- PASS`, and the
      PG 9.5–13 subtests show `--- SKIP` — the latter is the **expected** result on this image (no such
      clusters exist in it), not a defect. A skip on any of PG 14–19 is a finding.
- [ ] The live assertion compares column **names and order** against the server, not merely the count.
- [ ] `report/report_test.go::Test_processData_versionChange_recomputesLayout` (Task 3's synthetic
      two-version archive) passes, and the decisions log confirms all three version-change resets landed —
      not only the alignment flag.
- [ ] `report/report_test.go::Test_describeActivityColumnOrder` (Task 4's describe-order test) passes,
      including its presence-before-order check.
- [ ] On every available version (PG 14–19) the activity screen shows the 17 columns in the exact order
      fixed in the tech-spec — verified by eye against a captured row, not inferred from the count.
- [ ] An idle-in-transaction session shows a non-empty `horizon_xacts` and an **empty** `backend_xid`; after
      an `INSERT` in that same transaction `backend_xid` becomes non-empty.
- [ ] A parallel query collapses into one group: several rows share one `leader`, the leader's `leader`
      equals its own `pid`, and sorting by `leader` places them contiguously. Empty `backend_xid` on the
      workers and an identical `horizon_xacts` across the group are recorded as expected, not as findings.
- [ ] Blank cells render blank — never `0`, never a dash — and a genuine `horizon_xacts = 0` is visibly
      distinct from a blank.
- [ ] Sorting activity by `horizon_xacts` is numeric in both directions, independent of which row is first,
      with blanks last in **both** directions and never mixed with a genuine `0`.
- [ ] `pgcenter report -d -A` lists the three new columns, carries the PG 13+ note and carries **all four**
      caveats, the missing-privileges one included.
- [ ] Sorting the activity screen by `wait_event` (or `wait_etype`) — a **string** column blank on most
      backends — gives, in **both** directions: rows with a blank value at the **end** (never at the top,
      never interleaved), and the non-blank values ordered normally among themselves (alphabetically
      ascending under `<`-ascending, reversed under descending). Checked by hand on a cluster where most
      backends are not waiting, with the human judgement on readability recorded — this half of the sort
      change has no automated cover at all.
- [ ] The `replslots` default sort (`retained,KiB`) puts empty rows last in **both** directions and agrees
      with the screen's own `NULLS LAST`; sorting by an all-empty column (`safe,KiB`) is a no-op preserving
      input order.
- [ ] Under a few hundred sessions the activity screen renders, scrolls and re-sorts, and both measured
      thresholds hold: the screen keeps redrawing once per configured refresh interval with no visible
      stall (interval and method of observation recorded), and the screen's own SQL — timed in `psql` at
      that session count — stays comfortably inside that interval and is not the slowest query pgcenter
      runs. Numbers recorded, not impressions.
- [ ] Task 5's register work is in place and verified by hand: three new entries in `docs/tech-debt.md`,
      `[021]` moved to **Resolved Debt**, the "why deferred" text of `[020]` corrected, and the
      `architecture.md` sentence describing the activity selector's version branches refreshed (it no
      longer says "branches at PG 9.6, PG 10"). This criterion is closed by neither the test suite nor the
      manual walk, so it has to be checked explicitly and reported explicitly.
- [ ] The PG 12/13 boundary is reported as **not live-verifiable** on this image, resting on Task 1's table
      test, and that table test is confirmed to pin the boundary from both sides.
- [ ] Every user-spec and tech-spec acceptance criterion has a verdict with evidence in the QA report.
- [ ] Every problem found is routed per the rule in Details and named explicitly in the report.

## Context Files

**Feature artifacts:**
- [013-feat-activity-xmin-horizon.md](013-feat-activity-xmin-horizon.md) — user-spec; «Критерии приёмки»
  and «Как проверить» are the baseline for this task, and the layout table is the source of truth for the
  column order
- [013-feat-activity-xmin-horizon-tech-spec.md](013-feat-activity-xmin-horizon-tech-spec.md) — tech-spec;
  Acceptance Criteria, Decision 4 (empty-last, blast radius, the stated coverage gap), Decision 2 (the
  non-live-verifiable boundary), Decision 6 (the version-change resets), Risks table (routing rule),
  Backward Compatibility (why the golden re-run matters)
- [013-feat-activity-xmin-horizon-decisions.md](013-feat-activity-xmin-horizon-decisions.md) — decisions
  log; read it **before** judging any criterion failed, since Tasks 1–5 may have recorded a deliberate,
  approved deviation
- [013-feat-activity-xmin-horizon-code-research.md](013-feat-activity-xmin-horizon-code-research.md) —
  code research; §2 enumerates every sparse column on every screen (useful when deciding what else to
  spot-check) and its live PG 16 measurement of a parallel-worker group is the reference the manual
  observations are compared against

**Project knowledge** (`.claude/skills/project-knowledge/` — this project has `overview.md` in place of the
usual `project.md`):
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — what pgcenter is, which screens
  and statistics it covers, supported PostgreSQL versions
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout, data flow,
  PG version handling, the view registry and the version-branching selectors
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — testing conventions, the
  live-cluster skip mechanism, version-branching idioms
- [deployment.md](../../../.claude/skills/project-knowledge/deployment.md) — the testing image, its
  clusters and ports, CI container tag, release process

**Code and infrastructure files (read-only — this task writes no code):**
- [internal/query/activity.go](../../../internal/query/activity.go) — the query branch under test; the
  column list in the new constant is what the screen must match
- [internal/stat/postgres.go](../../../internal/stat/postgres.go) — `PGresult.sort`, the shared machinery
  the string-column and `replslots` checks exercise
- [internal/view/view.go](../../../internal/view/view.go) — the view registry; `activity`'s `OrderKey`/
  `Ncols` seed and `replslots`' default `OrderKey: 4` (`retained,KiB`)
- [internal/query/replication_slots.go](../../../internal/query/replication_slots.go) — the
  `ORDER BY "retained,KiB" DESC NULLS LAST` the Go sort must now agree with, and the source of the
  all-empty `safe,KiB` column
- [report/describe.go](../../../report/describe.go) — the text `pgcenter report -d -A` prints
- [top/help.go](../../../top/help.go) — the hotkey cheat-sheet: `Left`/`Right` change the sort column,
  `<` toggles direction, `[`/`]` scroll columns, `I`/`A` are the filters
- [internal/postgres/testing.go](../../../internal/postgres/testing.go) — version → port map for the
  fixture clusters, including the two EOL groups that never connect
- [internal/query/activity_test.go](../../../internal/query/activity_test.go) — `Test_StatActivityQueries`
  and its `90500 … 190000` loop: the source of the expected PASS/SKIP split in step 5
- [docs/tech-debt.md](../../../docs/tech-debt.md) — Task 5's three new entries, `[021]` under Resolved
  Debt and the corrected `[020]` justification, all checked by hand in this task
- [testing/prepare-test-environment.sh](../../../testing/prepare-test-environment.sh) — the cluster and
  fixtures bootstrap the container needs before anything works
- [.github/workflows/default.yml](../../../.github/workflows/default.yml) — the image tag under test and
  the run-time toolchain installation this task reproduces locally
- [Makefile](../../../Makefile) — `test`, `lint`, `vuln`, `build`, `install` targets

## Verification Steps

- Confirm the container is up, `prepare-test-environment.sh` has run, every fixture cluster accepts
  connections, and `pgcenter` is built and installed inside the container.
- Run `make test`, `make lint`, `make vuln` — all clean.
- Run `go test ./report/...` and `git status` — green, and no pre-existing golden regenerated.
- Run the verbose filtered invocation from step 5 and confirm `Test_StatActivityQueries` shows `--- PASS`
  on all six PG 14–19 subtests and `--- SKIP` on the PG 9.5–13 ones (expected — those clusters are not in
  the image).
- Confirm `Test_processData_versionChange_recomputesLayout` and `Test_describeActivityColumnOrder` each
  appear as passed in the verbose output, individually and by name.
- Confirm by hand that `docs/tech-debt.md` carries the three new entries with `[021]` under Resolved Debt
  and `[020]`'s justification corrected, and that `architecture.md` no longer describes the activity
  selector as branching only at PG 9.6 / PG 10.
- Confirm a captured activity row from each available version shows the 17 columns in the fixed order.
- Confirm captured evidence exists for each manual check: horizon holder before/after `INSERT`, the
  parallel group, blank-vs-zero, the numeric sort in both directions, the string-column sort in both
  directions, the `replslots` default sort in both directions, and the few-hundred-session run.
- Confirm `pgcenter report -d -A` output contains the three columns, the PG 13+ note and four caveats.
- Confirm the QA report exists, covers every user-spec and tech-spec criterion, and marks the PG 12/13
  boundary `not_verifiable` rather than passed.
- **User confirmation:** the user opens the activity screen on their own terminal and judges whether three
  new, mostly empty columns made the most-used screen in pgcenter harder to read, and whether the shift of
  `query` to the right is acceptable in daily use. The user-spec names this as the one thing that cannot be
  checked automatically — it is a matter of taste on a screen the user reads every day, and an automated
  check can prove the columns are present but not that the screen still reads well.

## Details

**Files:** none modified — this task writes no code. It produces the QA report at
`logs/working/qa-report.json`, the screen captures (suggested:
`logs/working/qa-013-activity/<check-name>.txt`) and a decisions-log entry. Note that
`logs/working/qa-report.json` is **git-tracked and currently holds the QA report of feature 003**; writing
it overwrites that content. That is the `pre-deploy-qa` skill's convention and is fine — the previous
report stays in git history — but do it knowingly: the overwrite lands in the commit diff and must be
mentioned in the decisions entry rather than being a silent deletion.

**Dependencies:** all five preceding tasks, because this task's criteria check their artifacts directly.

- **Task 1** — the PG 13+ query branch and the live name/order assertion. Steps 6 and 7 verify it, and the
  PG 12/13 boundary claim rests on its table test.
- **Task 2** — the sort fix. Steps 12, 14 and 15 are its only screen-level verification, and step 4 is the
  re-measurement the tech-spec's Backward Compatibility section demands.
- **Task 3** — the version-change resets and the zero-width guard. Not directly exercisable by hand on
  these clusters (no archive spans a real 12→13 upgrade here); it is carried by
  `Test_processData_versionChange_recomputesLayout`, which has its own acceptance criterion, plus a check
  in the decisions log that all three resets landed and not just the alignment flag.
- **Task 4** — the `describe.go` text checked in step 13, plus `Test_describeActivityColumnOrder`, which
  has its own acceptance criterion.
- **Task 5** — the tech-debt register entries and the Project Knowledge correction. Nothing in the test
  suite or the manual walk touches these, so they carry an explicit acceptance criterion of their own and
  are checked by reading `docs/tech-debt.md` and `architecture.md` directly:
  `grep -n "help.go" docs/tech-debt.md`, `grep -n "\[021\]" docs/tech-debt.md` (must land under Resolved
  Debt), and `grep -n "SelectStatActivityQuery" .claude/skills/project-knowledge/architecture.md`.

**Environment specifics** (verify each against the repo rather than trusting these numbers — the workflow
is the source of truth and tags move):

- Image: `lesovsky/pgcenter-testing`, tag pinned in `.github/workflows/default.yml` at `container:`
  (`0.0.11` at the time of writing). Ubuntu 22.04 with PostgreSQL 14–19; **PG 12 and PG 13 are not in the
  image**, which is precisely why the branch boundary is not live-verifiable.
- Cluster ports: PG14→21914 … PG19→21919 (`internal/postgres/testing.go`). Two groups of EOL entries sit
  in the same map for reference only and will not connect: **21910–21913** (PG 10–13) and **21994–21996**
  (PG 9.4–9.6, which break the numbering pattern). Together they are why `Test_StatActivityQueries` skips
  six of its twelve subtests here.
- `testing/prepare-test-environment.sh` creates the clusters, writes `postgresql.auto.conf`
  (`shared_preload_libraries='pg_stat_statements'`, `wal_level = logical`, trust `pg_hba.conf` including
  replication lines), starts everything and loads `fixtures.sql`. The fixtures database is
  **`pgcenter_fixtures`**; connections are `trust`, user `postgres`.
- The image ships neither the Go toolchain nor lint tools; the workflow installs them at run time
  (Go tarball into `/opt/go` plus symlinks; `golangci-lint` v2, `gosec`, `govulncheck` via `go install`,
  symlinked into `/usr/local/bin`). Reproduce that sequence before running `make test` / `make lint` /
  `make vuln`.
- `make test` is `go test -race -p 1 -timeout 300s ./...` — `-p 1` matters, the clusters are shared state.
  `make install` copies into `/usr/bin` and needs root, which is the default user in the container.
- `tmux` is not in the image either and the apt lists are deleted in the build layer, so an install needs
  `apt-get update` first.

**How to reach the screens.** `a` → `activity`, `o` → `replslots`. On any screen: `←` / `→` move the sort
column, `<` toggles ascending/descending, `[` / `]` scroll columns horizontally, `↑` / `↓` change the
current column's width (easy to hit by accident while aiming for the sort keys), `/` sets a regex filter on
the current sort column, `A` sets the age threshold and `I` toggles idle sessions. The age threshold
defaults to `00:00:00.0`, i.e. no filtering, so every session with an `xact_start` or a `query_start` is on
screen without touching `A`.

**Edge cases and expected non-findings:**

- **Blank `backend_xid` on parallel workers, even in a writing transaction** — the xid belongs to the
  leader's transaction. Measured on a live PG 16 during code research. Expected.
- **Identical `horizon_xacts` across a parallel group** — workers inherit the leader's snapshot, so the
  group occupies N+1 adjacent rows with one value when sorted by that column. Named as an edge case in the
  user-spec. Expected.
- **`horizon_xacts = 0`** is a real state ("holds a snapshot taken right now"), not a blank. Seeing it is
  confirmation, not a finding — but seeing it *where a blank belongs* is a finding.
- **Background workers (checkpointer, walwriter, bgwriter) absent from the screen** — they have neither an
  `xact_start` nor a `query_start`, and the screen's WHERE clause filters on exactly those. Expected; not
  a symptom of the new columns.
- **An autovacuum worker with a non-empty `horizon_xacts`** — correct, autovacuum holds a horizon too.
- **`safe,KiB` empty for every row on `replslots`** — `max_slot_wal_keep_size = -1` is the PostgreSQL
  default, so this column is legitimately all-empty on a stock cluster. That makes it the natural subject
  for the "sorting an all-empty column is a no-op" half of step 15, not a defect to report.
- **`cl_addr` empty for every row on a purely local cluster** — same situation, same non-finding.
- **`cl_port` showing `-1`** for unix-socket connections is real data; per Decision 4 blanks now sort
  *below* that genuine `-1` on descending sort where previously they sorted above it. That reordering is
  the intended correction, not a regression.
- **The `replication` screen also has a column named `horizon_xacts`, computed by a different formula** —
  the two are deliberately not reconciled (documented instead, Task 5's debt entry). A numeric difference
  between the two screens is expected, not a finding.
- **A green test suite with a stopped cluster is still green** — `t.Skipf` is the mechanism, and
  `patterns.md` says outright that proving a version is actually reached needs a deliberate check. Treat
  "green" as insufficient; look for executed subtests.
- **Six skipped subtests in `Test_StatActivityQueries`** — the test iterates `90500 … 190000` while the
  image carries PG 14–19, so PG 9.5, 9.6, 10, 11, 12 and 13 skip on every run, on CI as well. Expected;
  do not "fix" it by narrowing the version list or by pointing a skipped version at a live port — the
  version list is what pins the branch boundary, and the gap between it and the image is already
  registered as debt by Task 5.

**Problem routing.** Every manual check in this task exists because the tech-spec's Risks table named a
specific failure mode; a check that reveals a problem is therefore reported **against the risk whose
mitigation it just falsified**, not filed as a loose observation and not quietly accepted.

| Check | Risk row it tests |
|---|---|
| Column layout (step 7) | "Column order in the SQL drifts from the specified layout" |
| Blank-not-zero (step 11), four caveats (step 13) | "Blank cells read as 'holds no horizon' when the real cause is missing privileges" |
| Numeric sparse sort (step 12) | Risk 1 in the user-spec — the feature's primary story |
| String-column sort (step 14), `replslots` sort (step 15) | "Sort fix silently changes ordering on unrelated screens" and "Passing goldens are mistaken for evidence that the sort change is safe" |
| Boundary not live-verifiable (Description) | "Branch written as `< 140000` instead of `< 130000`" |

Unlike feature 012, there is no other feature to route to: everything these checks touch is inside this
feature's own scope. So a real problem is **fixed here before the feature is declared done**, or — if the
fix is genuinely out of scope — recorded as a new row in the tech-spec's Risks table with an owner and a
decision, made by the user. "Noted in passing" is not one of the options.

The one check with no existing risk row is the few-hundred-session run (step 16): it comes from the
roadmap's "do not make an incident worse" rule, not from the Risks table. A finding there therefore opens
a new row rather than filling an existing one — say so explicitly if it happens.

**How this maps onto the `pre-deploy-qa` skill's phases.**

- **Phase 1 (environment rebuild).** The skill's detection order is Makefile → docker-compose. This repo
  has no compose file, and its Makefile's `build` target builds the Go binary, not a container image —
  there is no rebuild/up/down/restart target to drive, so the from-scratch `--no-cache` rebuild does not
  apply here. Step 1 replaces it: run the **published** image, bootstrap the clusters, verify they all
  respond. The deliberate deviation is narrow — we do not build the image from `testing/Dockerfile` even
  though we could, because the published image is what CI verifies against and a local rebuild would
  resolve different PG 19 packages from the beta channel.
- **Phase 2 (test suite)** applies unchanged.
- **Phase 3 (acceptance criteria)** is the bulk of the task — sections C and D.
- **Phase 4 (coverage verification)** is kept in verifying rather than deriving form: Tasks 1–4 added the
  coverage, so run Phase 4 against the tech-spec's per-task "Files to modify" list to confirm each touched
  file has a test that actually exercises the new path, instead of re-deriving coverage from scratch. Apply
  its `critical` severity as written — but with one honest exception already recorded by the tech-spec:
  the string-comparator half of the sort change has **no** automated cover by design (Decision 4), and step
  14 is what stands in for it. That is a stated, accepted gap, not a Phase 4 critical.

**Implementation hints:**

- Drive the TUI through `tmux`: a detached session running `pgcenter top`, `tmux send-keys` to switch
  screens and move the sort column, `tmux capture-pane -p` into a file per check. Allow at least one
  refresh interval after each keypress before capturing, or the capture shows the previous state. The
  captures are what makes the manual half reviewable afterwards — a screen that renders a wrong value and
  a screen that renders a right one look identical in a report that only says "checked".
- Keep the load-generating sessions in separate panes or background `psql` processes so they survive the
  whole walk. An idle-in-transaction session dies with its shell.
- For the horizon holder: `BEGIN; SELECT ...;` then leave the session sitting. `horizon_xacts` needs an
  active snapshot; the `INSERT` in step 9 must run in that **same** transaction, before any `COMMIT`.
- For the parallel group: `SET max_parallel_workers_per_gather = 4;` plus a sequential scan over a table
  big enough that the query lasts long enough to be sampled. Check `backend_type = 'parallel worker'` on
  the rows to be sure workers actually spawned rather than the planner deciding against them.
- To make `replslots` sparse: create one logical slot that has reserved WAL and one physical slot created
  **without** reserving WAL — a physical slot with no `restart_lsn` is exactly the "slot that reserved no
  WAL" case that leaves `retained,KiB` empty. The clusters are configured with `wal_level = logical`, so
  logical slots work. Drop the slots afterwards; a forgotten slot retains WAL indefinitely.
- For a few hundred sessions, a loop of backgrounded `psql` processes each holding an open transaction is
  enough. Watch `max_connections` on the cluster and raise it deliberately if needed rather than
  discovering the ceiling as a mysterious failure.
- Read the decisions log before marking any criterion failed — an earlier task may have recorded a
  deliberate, approved deviation, and judging it a defect would be wrong.
- Prefer capturing a `psql` cross-check alongside each screen capture at the same moment. `pg_stat_activity`
  moves fast, and a mismatch caused by sampling two different instants is not a finding.

## Reviewers

None — this task is itself the review gate. Its output is the QA report, not code, so no reviewer agents
run against it.

## Post-completion

- [ ] Записать краткий отчёт в
      [013-feat-activity-xmin-horizon-decisions.md](013-feat-activity-xmin-horizon-decisions.md)
      (Summary: 1-3 предложения, ссылка на `logs/working/qa-report.json`, без таблиц файндингов и дампов)
- [ ] Отметить в записи перезапись `logs/working/qa-report.json` (там лежал отчёт фичи 003) — чтобы
      удаление не выглядело случайным в диффе коммита
- [ ] Явно перечислить критерии со статусом `not_verifiable` и то, на чём они держатся вместо живой
      проверки (в первую очередь границу PG 12/13)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
