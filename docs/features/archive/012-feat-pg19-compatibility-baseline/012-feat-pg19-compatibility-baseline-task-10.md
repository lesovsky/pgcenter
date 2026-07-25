---
status: planned
depends_on: ["05", "06", "07", "09"]
wave: 7
skills: [pre-deploy-qa]
verify: "bash + user — full QA per the user-spec «Как проверить» section"
reviewers: []
teammate_name:
---

# Task 10: Pre-deploy QA

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:pre-deploy-qa` — [skills/pre-deploy-qa/SKILL.md](~/.claude/skills/pre-deploy-qa/SKILL.md)

## Description

Final acceptance gate for the feature. Everything before this task proved that the code *builds* and that the
*unit tables* say the right thing; this task proves that pgcenter actually works against a live PostgreSQL 19
server, which is the entire point of the feature. The user-spec is explicit about this: the value of
"PostgreSQL 19 compatibility baseline" is **provenness**, not new functionality — the version selectors were
written as inequalities from the start, so the code was ready by construction. What was missing was anyone
ever pointing pgcenter at a PG 19 cluster.

The task has two halves.

**Automated half.** The full Go suite, lint, vulnerability check and the end-to-end script, executed against
the published testing image so that PG 14 through PG 19 are all exercised. This is mostly a re-run of what
Tasks 4, 6 and 9 already verified in isolation, but run once, together, on the merged result.

**Manual half.** A scripted walk over **all 27 registered `top` screens** on a live PG 19 cluster, driven
through `tmux` so that every screen leaves a captured text artifact. The artifact matters: a screen that
renders an empty column and a screen that renders correct data look identical to an agent that only checks
"did the process crash", and the captures are what makes the difference reviewable after the fact. Load has
to be generated **before** the walk — the six progress screens are empty on an idle cluster, and walking them
idle would verify nothing while looking like a pass.

Beyond "renders without errors", the user-spec names four checks that a naive pass would miss, and each of
them exists because its failure mode is silent:

- **No column is empty where `psql` returns data for the same moment.** A query that lost a column returns
  rows with a blank field, not an error.
- **The new columns (`started_by`, `mode`, `backup_type`) match what `psql` reports at that moment.** These
  three column names were read from a **beta** catalog; if any of them was renamed before GA, the PG 19 query
  branch errors out and the screen is empty for every PG 19 user.
- **The summary panel's `waiting` counter increments under a deliberately created lock.** This counter is
  built on a hardcoded wait-event-type literal (`Lock`). PG 19 renamed `BUFFERPIN` → `BUFFER`, which does not
  touch this literal — but if the assumption is wrong, the counter shows a silent zero, panics nothing, and
  sails through a "renders without errors" pass.
- **A running `REPACK` appears on the cluster progress screen.** PG 19 replaced `VACUUM FULL` / `CLUSTER` with
  the new `REPACK` command and kept `pg_stat_progress_cluster` as a backwards-compatibility view. The docs
  promise this works; this is where we confirm it by hand.

Verification depth is **deliberately limited to the mechanism, not the whole value domain.** The rare values —
`mode = failsafe` and `started_by = autovacuum_wraparound` — are not reproduced. The first needs a transaction
age above the failsafe threshold, the second needs hours of transaction generation or a GUC change; both cost
far more to set up than the risk they retire, because it is the same code path either way: the column either
reaches the screen or it does not, and that is provable on reachable values.

Any breakage this task finds is **routed, not silently accepted** — see the routing rule in Details.

## What to do

### A. Automated verification

1. **Bring up the environment.** The image does not come up ready: its `CMD` is a bare `echo`, the clusters
   and fixtures are created by an explicit script run, and the Go toolchain and lint tools are installed at
   run time (CI does exactly this, see `.github/workflows/default.yml`). Reproduce that sequence:
   - Read the image tag Task 9 actually put into `.github/workflows/default.yml` (`container:`) rather than
     assuming `0.0.11`; the workflow is the source of truth.
   - Start the container detached with a long-lived command (the default `CMD` exits immediately), publishing
     ports 21914–21919 bound to loopback (`-p 127.0.0.1:219NN:219NN`) — the clusters use `trust`
     authentication — and mounting the repo checkout so the same tree is visible inside and outside.
   - `docker exec` the cluster bootstrap: `prepare-test-environment.sh`. Nothing exists before this step.
   - Install the Go toolchain and lint tools inside the container the same way the workflow does, so
     `make test` / `make lint` / `make vuln` can run there.
   - Verify all six clusters accept connections and `pgcenter_fixtures` exists on each.

   Do **not** rebuild the image from `testing/Dockerfile`: the published image is the artifact under test,
   and a local rebuild would pull different beta packages. See the `pre-deploy-qa` deviation note in Details
   for how this maps onto the skill's phases.
2. Run the full suite: `make test`, `make lint`, `make vuln`. All must be clean.
3. Run `./testing/e2e.sh` and confirm it covers port 21919 in both loops (record and report) — a green run
   that never touched 21919 proves nothing about PG 19. `pgcenter` must be on `PATH` first (`make install`).
4. Confirm the PG 19 subtests **executed** rather than skipped. `t.Skipf` on an unreachable cluster is the
   normal mechanism, so a green suite with a stopped 21919 cluster is also green — and `make test` is not
   verbose, so it cannot answer this. Run a named verbose invocation instead:

   ```bash
   go test -v -p 1 ./internal/query/... ./internal/stat/... ./internal/postgres/... 2>&1 | grep -E '190000'
   ```

   Version subtests are named `<subject>/<version>` (e.g. `pg_stat_progress_vacuum/190000`). Require
   `--- PASS` lines for `190000` and no `--- SKIP` for it.

### B. Version reachability

5. Confirm that a test connection request for a version with no entry in the port map returns an **error**
   rather than silently connecting to another cluster. There is a unit test from Task 2 for this — confirm it
   exists and passes, and confirm the map contains the PG 19 entry.
6. One-off manual check: stop the 21919 cluster, run the suite, confirm PG 19 subtests **skip** rather than
   pass. Restart the cluster afterwards. **Expected collateral, not damage:** the nine tests that lack a
   per-version subtest wrapper (Decision 7 — `common_test.go` ×4, `overview_test.go` ×4,
   `internal/stat/postgres_test.go`) fire their `t.Skipf` on the *parent* test, so with 21919 down they skip
   wholesale, for every version, not just for PG 19. That is the pre-existing hole Decision 7 deliberately
   left in place; it is not a regression and not a finding.

### C. Load generation (before any screen walk)

7. On the PG 19 cluster, create enough load that every progress screen has something to show. Progress rows
   are transient, so the load has to be running concurrently with the walk, not before it — a large table plus
   a long-running `VACUUM`, a long `ANALYZE`, a `CREATE INDEX`, a bulk `COPY`, a `REPACK`, and a
   `pg_basebackup`. Size the table so each operation lasts long enough to be captured (tens of seconds, not
   hundreds of milliseconds).

   **If `pg_basebackup` fails with `no pg_hba.conf entry for replication connection`, that is a missing
   Task 1 change, not a PG 19 regression — do not file it as one.** The environment script writes a
   `pg_hba.conf` whose `host` line uses the `all` database keyword, which does not match physical
   replication connections; Task 1 owns the fix (adding `replication` entries for both the local socket and
   TCP). Without it, `progress_basebackup` cannot be loaded and the `backup_type` criterion cannot be
   verified. Report it as a blocked dependency on Task 1 and get the script fixed, rather than recording a
   PG 19 finding or marking the criterion passed.
8. Generate query activity so the `pg_stat_statements` screens are non-empty, and confirm the extension is
   present in the fixtures database.

### D. Screen walk — all 27 screens

9. **Prepare the walk harness.** The image ships neither `tmux` nor a `pgcenter` binary, so both have to be
   put there first:
   - `tmux` — `apt-get update && apt-get install -y tmux` inside the running container. The update is not
     optional: the image's build layer ends by deleting the apt lists, so the package index is empty and a
     bare install fails with "unable to locate package".
   - the binary — build it from the checkout mounted in step 1 with the Go toolchain installed in step 1
     (`make build && make install`, giving `/usr/bin/pgcenter`). Building on the host and `docker cp`-ing the
     binary in also works — it is a static Go binary and the container is Ubuntu 22.04 — but building inside
     keeps toolchain and target identical to CI.

   Then drive it: start a detached `tmux` session running `pgcenter top` against the live PG 19 cluster,
   switch screens with `tmux send-keys`, and dump the pane after each switch with `tmux capture-pane -p`
   into its own file. Every one of the 27 registered views must produce a capture — the full hotkey map is
   in Details.
10. The per-process screen (`S`) is only available on a **local** connection: the gate is
    `app.db.Local` (`top/config_view.go`), set from `isLocalhost(config.Config.Host)`
    (`internal/postgres/postgres.go`), which decides purely on the **host string** — a unix socket path,
    `localhost`, `127.0.0.1`, `::1` or a local interface address. A host-side run against the published
    `127.0.0.1:21919` would pass that string check but read the *host's* `/proc`, not the backends'. So this
    part of the walk genuinely has to run inside the container, connecting over the unix socket or
    `127.0.0.1` there, where the host string is local **and** `/proc` of the PostgreSQL backends is the real
    one. This is the reason the whole walk is done in-container rather than from the host.
11. For each capture, check: the screen rendered, there was no panic, and no column is blank where `psql`
    returns data for the same moment. Cross-check against `psql` for the screens where blankness is plausible
    — do not eyeball 27 screens' worth of columns, but do cross-check every screen this feature touched and
    every screen whose underlying view changed in PG 19.

### E. Targeted checks

12. **New columns vs `psql`.** With the vacuum / analyze / basebackup operations running, confirm
    `progress_vacuum` shows `started_by` and `mode`, `progress_analyze` shows `started_by`,
    `progress_basebackup` shows `backup_type`, and that the values equal what `psql` returns from the same
    catalog view at the same moment.
13. **NULL renders blank.** On a vacuum row that has no progress record yet (the vacuum screen right-joins
    `pg_stat_activity`, so such rows exist), confirm `started_by` / `mode` are empty — no dashes, no zeros, no
    `unknown`.
14. **Lock counter.** Create a real lock with two sessions (one holds a lock, another blocks on it) and
    confirm the `waiting` counter in the summary panel increments.
15. **REPACK.** With a `REPACK` running, open the cluster progress screen and confirm the row is visible.
    **Expect an apparent mismatch and do not file it as a bug:** pgcenter never selects that view's `command`
    column, so the screen shows the `REPACK …` text in the query column while `psql` reports the translated
    older command name (`CLUSTER`). This is Decision 6 in the tech-spec, documented precisely so it is not
    re-litigated.

### F. Regression

16. Walk the three progress screens on **PG 18** (port 21918) and **PG 14** (port 21914) and confirm they look
    exactly as they do today: same columns, same column count, no `started_by` / `mode` / `backup_type`.
17. Replay an existing pre-0.12 archive with `pgcenter report` and confirm the output is unchanged and that
    **no golden file was regenerated** — a golden diff here is a red flag, not expected churn. Confirm via
    `git status` / `git diff` that `report/testdata/` is clean apart from the two goldens Task 6 legitimately
    added.
18. Confirm `pgcenter report -d -P v|a|b` describes the new columns.

### G. Report

19. Produce the QA report per the `pre-deploy-qa` skill (`logs/working/qa-report.json`), covering every
    acceptance criterion from both the user-spec and the tech-spec, with evidence per criterion. **Note that
    this path is git-tracked and already holds the QA report of a previous feature** — writing it overwrites
    that content. That is the skill's convention and is fine (the previous report stays in git history), but
    do it knowingly: the overwrite goes into the commit diff and must be mentioned in the decisions entry, so
    the earlier content is not silently lost.
20. Route any breakage found per the rule in Details, and name every routed item explicitly in the report.

## Acceptance Criteria

- [ ] `make test`, `make lint`, `make vuln` clean; `./testing/e2e.sh` passes including port 21919.
- [ ] PG 19 subtests demonstrably **executed** (not skipped) against the live 21919 cluster.
- [ ] A connection request for a version absent from the port map returns an error, not a connection to
      another cluster; with 21919 stopped, PG 19 subtests skip rather than pass.
- [ ] All 27 registered `top` screens rendered on live PG 19 without errors and without panics — including
      both `pg_stat_io` sub-screens, all seven `pg_stat_statements` sub-screens and all six progress screens,
      with the per-process screen checked over a local connection. Each screen has a captured text artifact.
- [ ] No column is blank where `psql` returns data for the same moment.
- [ ] `progress_vacuum` shows `started_by` and `mode`, `progress_analyze` shows `started_by`,
      `progress_basebackup` shows `backup_type`; values match `psql` at the same moment. If `pg_basebackup`
      cannot connect (`no pg_hba.conf entry for replication connection`), this criterion is **blocked on
      Task 1's environment-script fix** — not failed on PG 19, and not passed.
- [ ] A vacuum row without a progress record renders `started_by` / `mode` as empty — no dashes, no zeros, no
      `unknown`.
- [ ] The `waiting` counter in the summary panel increments under a deliberately created lock.
- [ ] A running `REPACK` is visible on the cluster progress screen; the query-column / `psql`-command
      difference is recorded as expected behaviour, not as a finding.
- [ ] The three progress screens on PG 18 and PG 14 are identical to today — same columns, same count.
- [ ] Replaying a pre-0.12 archive produces unchanged output; no existing golden was regenerated.
- [ ] `pgcenter report -d -P v|a|b` describes the new columns.
- [ ] `overview.md`, `deployment.md`, `architecture.md` list PG 19.
- [ ] Every acceptance criterion from the user-spec and the tech-spec has a verdict with evidence in
      `logs/working/qa-report.json`.
- [ ] Any breakage found is routed per the rule in Details and named explicitly in the report with its owning
      feature.

## Context Files

**Feature artifacts:**
- [012-feat-pg19-compatibility-baseline.md](012-feat-pg19-compatibility-baseline.md) — user-spec; sections
  «Критерии приёмки» and «Как проверить» are the source of truth for this task
- [012-feat-pg19-compatibility-baseline-tech-spec.md](012-feat-pg19-compatibility-baseline-tech-spec.md) —
  tech-spec; Acceptance Criteria, Risks table (routing rule), Decision 6 (REPACK)
- [012-feat-pg19-compatibility-baseline-decisions.md](012-feat-pg19-compatibility-baseline-decisions.md) —
  decisions log; read for deviations recorded by Tasks 1–9 before judging a criterion failed

**Project knowledge** (`.claude/skills/project-knowledge/` — note this project has `overview.md` in place of
the usual `project.md`):
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — features, supported stats, supported
  PostgreSQL versions
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout, data flow, PG
  version handling, view registry
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — testing conventions, version
  branching, the skip mechanism for live-cluster tests
- [deployment.md](../../../.claude/skills/project-knowledge/deployment.md) — testing image contents, cluster
  port list, CI container tag

**Code files (read-only — this task writes no code):**
- [internal/view/view.go](../../../internal/view/view.go) — the registry of the 27 views; the authoritative
  list to walk
- [top/keybindings.go](../../../top/keybindings.go) — hotkey → view mapping
- [top/menu.go](../../../top/menu.go) — the four sub-menus and their item order
- [top/config_view.go](../../../top/config_view.go) — the per-process screen's local-connection gate
- [internal/postgres/postgres.go](../../../internal/postgres/postgres.go) — `isLocalhost`, which decides
  whether the per-process screen is reachable
- [internal/postgres/testing.go](../../../internal/postgres/testing.go) — version → port map
- [testing/e2e.sh](../../../testing/e2e.sh) — the end-to-end script and its port loops
- [testing/prepare-test-environment.sh](../../../testing/prepare-test-environment.sh) — the cluster bootstrap
  the container needs (step 1); also the file that owns the `pg_hba.conf` replication entries fixed by Task 1
- [.github/workflows/default.yml](../../../.github/workflows/default.yml) — the image tag under test and the
  run-time Go / lint tool installation this task reproduces locally
- [Makefile](../../../Makefile) — `test`, `lint`, `vuln`, `build`, `install` targets

## Verification Steps

- Confirm the container is up with all six clusters bootstrapped (`prepare-test-environment.sh` has run) and
  the Go toolchain, lint tools and `tmux` are present inside it.
- Run `make test`, `make lint`, `make vuln`, `./testing/e2e.sh` — all clean, e2e covers 21919.
- Run the verbose invocation from step 4 and confirm `--- PASS` lines for `190000` subtests with no
  `--- SKIP` for them, i.e. PG 19 subtests executed against the live cluster.
- Confirm 27 screen capture files exist, one per registered view, and inspect each for rendering errors,
  panics and blank columns.
- Confirm the four targeted checks (new columns vs `psql`, NULL-as-blank, lock counter, REPACK) each have
  captured evidence.
- Confirm PG 18 / PG 14 progress screens match today's layouts and `git status` shows no regenerated goldens.
- Confirm `logs/working/qa-report.json` exists, covers every user-spec and tech-spec criterion, and its status
  reflects the findings.
- **User confirmation:** the user spot-checks the progress screens on their own environment and judges whether
  the `started_by` column width is comfortable at their usual terminal size. This is a matter of taste, not
  correctness — an automated check can prove the column is present but not that the screen reads well.

## Details

<!-- All details for task execution — technical, organizational, any other. -->

**Files:** none modified. This task produces `logs/working/qa-report.json`, the screen captures (suggested:
`logs/working/qa-pg19-screens/<view-name>.txt`), and a decisions.md entry. `logs/working/qa-report.json` is
git-tracked and currently holds a previous feature's report — see step 19.

**Dependencies:**
- **Task 9** — CI pointed at the published image, e2e script covering 21919; depends in turn on Task 8
  (image published) and Task 1 (the environment itself).
- **Task 5** — the `report -d -P v|a|b` describe texts, verified in step 18.
- **Task 6** — the report replay coverage and the two legitimately added goldens, verified in step 17.
- **Task 7** — `overview.md` / `deployment.md` / `architecture.md` listing PG 19, verified in the criteria.

These four are the tasks whose artifacts this task's acceptance criteria actually check, which is why all
four are in `depends_on` — Task 9 alone would let this run before the describe texts, the replay goldens or
the documentation exist. Tasks 2, 3 and 4 are upstream of them and merged by then.

**Blocked-by note (Task 1).** `pg_basebackup` is required for the `backup_type` criterion and currently
cannot connect — see step 7. The fix belongs to Task 1's environment script, not here.

**The 27 screens and how to reach them.** Derived from `internal/view/view.go`, `top/keybindings.go` and
`top/menu.go`. Sub-menus open with a capital letter and are navigated with arrow keys + Enter; the item
numbers below are positions in the menu as defined in `top/menu.go`.

| # | View name | How to reach |
|---|---|---|
| 1 | `activity` | `a` |
| 2 | `replication` | `r` |
| 3 | `databases_general` | `d` (also `D` → item 1) |
| 4 | `databases_sessions` | `D` → item 2 |
| 5 | `tables` | `t` |
| 6 | `indexes` | `i` |
| 7 | `sizes` | `s` |
| 8 | `functions` | `f` |
| 9 | `wal` | `w` |
| 10 | `bgwriter` | `b` |
| 11 | `replslots` | `o` |
| 12 | `stat_io` | `j` (also `J` → item 1) |
| 13 | `stat_io_time` | `J` → item 2 |
| 14 | `statements_timings` | `x` (also `X` → item 1) |
| 15 | `statements_general` | `X` → item 2 |
| 16 | `statements_io` | `X` → item 3 |
| 17 | `statements_temp` | `X` → item 4 (temp files I/O) |
| 18 | `statements_local` | `X` → item 5 (temp tables / local I/O) |
| 19 | `statements_wal` | `X` → item 6 |
| 20 | `statements_jit` | `X` → item 7 |
| 21 | `progress_vacuum` | `p` (also `P` → item 1) |
| 22 | `progress_cluster` | `P` → item 2 |
| 23 | `progress_index` | `P` → item 3 (`pg_stat_progress_create_index`) |
| 24 | `progress_analyze` | `P` → item 4 |
| 25 | `progress_basebackup` | `P` → item 5 |
| 26 | `progress_copy` | `P` → item 6 |
| 27 | `procpidstat` | `S` — **local connection only** |

Cross-check the count against `internal/view/view_test.go`, which asserts `27` explicitly. If the registry has
grown since this task was written, walk what the registry actually contains and note the discrepancy.

**Breakage routing rule (tech-spec Risks table, last row).** A real breakage found on PG 19 is:
- **fixed inside this feature** — the default; or
- **named explicitly in the QA report with its owning feature**, if the affected area is re-entered by a later
  feature of the same 0.12.0 release. The tech-spec row names exactly three such features — [016] WAL and
  archiving, [017] tables and autovacuum, [018] replication and recovery — so route only to those three. This
  is acceptable *only* because all of them ship in the same release, so no screen is broken at release time.
  It is re-verified at release finalization.
- **returned here and fixed before release** if the owning feature drops out of 0.12.0 — the promise "works on
  PG 19" cannot depend on unreleased work.

Never silently accept a breakage. "Documented in the QA report with an owning feature" is the only alternative
to fixing it here.

**How this maps onto the `pre-deploy-qa` skill's phases.**
- **Phase 1 (environment rebuild).** The skill's detection order is Makefile → docker-compose. This repo has
  no compose file at all, and its Makefile's `build` target builds the Go binary, not a container image —
  there is no rebuild/up/down/restart target to drive. So the skill's from-scratch `--no-cache` rebuild does
  not apply to this repo in the first place; nothing is being skipped that the skill would otherwise have
  done. What replaces it is step 1: run the **published** image, bootstrap the clusters, verify all six
  respond. The deliberate deviation is narrower than "we skip Phase 1" — it is that we do **not** build the
  image from `testing/Dockerfile` even though we could: the published image is the artifact under test and CI
  runs on it, and a local rebuild would resolve different beta packages from the PG 19 beta channel, so a
  locally built image would be a different environment than the one the release is verified on.
- **Phase 2 (test suite)** and **Phase 3 (acceptance criteria)** apply unchanged; Phase 3 is the bulk of this
  task.
- **Phase 4 (coverage verification)** is kept, in verifying rather than deriving form: Tasks 4 and 6 already
  added the PG 19 coverage, so run Phase 4 against the tech-spec's "Files to modify" list to confirm each
  touched file has a test that actually exercises the PG 19 path — the `190000` subtests from step 4 are the
  evidence — instead of re-deriving coverage from scratch. Anything on that list without such a test is a
  `critical` finding exactly as the skill says.

**Edge cases and expected non-findings:**
- **Empty progress screens are normal on an idle cluster** — that is why load generation comes first. An empty
  screen after load generation *is* a finding.
- **Replication and replication-slot screens will be empty.** The test environment has no standby; the standby
  fixture belongs to [018] by the user-spec. Empty here means "renders without errors", which is the bar.
- **The REPACK query-column mismatch is expected** (Decision 6) — see step 15.
- **The JIT statements sub-screen (`statements_jit`, `X` → item 7) will be empty.** JIT only kicks in above
  `jit_above_cost`, and the fixtures database is far too small to reach it — the view's own message says
  "no rows when `jit=off`". Empty here means "renders without errors", which is the bar. Do not force JIT on
  to fill it; that is not what this feature verifies.
- **`mode` as a column name now exists on two screens** — the replication screen already uses it as an alias
  for `sync_state`. No conflict today; it is a handoff note for [014], not a finding here.
- **`failsafe` and `autovacuum_wraparound` are deliberately not reproduced** — see the Description. Not
  observing them is not a gap.
- **A stopped 21919 cluster makes the suite green by skipping.** Treat "green" as insufficient evidence; look
  for executed PG 19 subtests.

**Implementation hints:**
- Drive the TUI with `tmux send-keys` and capture with `tmux capture-pane -p`. Allow at least one refresh
  interval after each switch before capturing, or the capture shows the previous screen.
- Keep the load-generating sessions in separate `tmux` panes or background `psql` processes so they survive the
  whole walk.
- For the `psql` cross-checks, query the same catalog view the screen reads (`pg_stat_progress_vacuum`,
  `pg_stat_progress_analyze`, `pg_stat_progress_basebackup`) within the same refresh window — progress rows move fast,
  and a mismatch caused by sampling at different moments is not a finding.
- For the lock check, the simplest reliable setup is one session holding an explicit table lock in an open
  transaction and a second session attempting a conflicting statement.
- Read the decisions log before marking any criterion failed: earlier tasks may have recorded a deliberate,
  approved deviation.

## Reviewers

None — this task is itself the review gate. Its output is the QA report, not code, so no reviewer agents run
against it.

## Post-completion

- [ ] Записать краткий отчёт в
      [012-feat-pg19-compatibility-baseline-decisions.md](012-feat-pg19-compatibility-baseline-decisions.md)
      (Summary: 1-3 предложения, ссылка на `logs/working/qa-report.json`, без таблиц файндингов и дампов)
- [ ] Перечислить в отчёте все поломки, отправленные в другие фичи релиза, с указанием фичи-исполнителя
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
