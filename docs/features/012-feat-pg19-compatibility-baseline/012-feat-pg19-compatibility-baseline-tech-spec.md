---
created: 2026-07-25
status: draft
branch: feature/pg19-compatibility-baseline
size: M
---

# Tech Spec: PostgreSQL 19 compatibility baseline

## Solution

Three independent workstreams, sequenced so the riskiest one retires first.

1. **Test/CI infrastructure.** Add `PostgresV19 = 190000`, map port 21919, rebuild the
   `lesovsky/pgcenter-testing` image with a PG 19 cluster from the beta apt channel, and thread `190000`
   through the test suite. Separately, replace the silent PG 14 fallback in `NewTestConnectVersion` with an
   error — the function's own doc comment already promises that behaviour.
2. **Three version-aware progress selectors.** Each of `progress_vacuum`, `progress_analyze` and
   `progress_basebackup` gains a second query constant and a `Select…Query(version) (string, int, [2]int)`
   selector wired into `view.Configure()`, following the `io.go` / `bgwriter.go` idiom. New columns are
   inserted mid-layout, so `Ncols` and `DiffIntvl` both become version-dependent.
3. **Verification pass** over every registered `top` screen on a live PG 19 cluster, plus report-replay
   coverage proving that pre-0.12 archives are unaffected.

No new views, hotkeys, menu entries or recorder changes. The `p`/`P` progress group, `record`, and the
`report -P` flag surface are untouched.

## Architecture

### What we're building/modifying

- **`internal/query/query.go`** — `PostgresV19 = 190000`. The single seam; every version branch in the tree
  is an inequality with `>=` on the newest arm, so 190000 lands correctly by construction.
- **`internal/postgres/testing.go`** — `190000: 21919` in the ports map, and `NewTestConnectVersion`
  returns an error for an unmapped version instead of falling back to the PG 14 cluster.
- **`internal/query/progress_vacuum.go` / `progress_analyze.go` / `progress_basebackup.go`** — one PG 19
  query constant and one selector each.
- **`internal/view/view.go`** — three new `case` blocks in `Configure()`. The static `New()` map keeps its
  pre-19 values; it is the pre-Configure default and is pinned by tests.
- **`report/describe.go`** — the three progress description texts gain rows for the new columns.
- **`testing/Dockerfile`, `testing/prepare-test-environment.sh`, `testing/e2e.sh`,
  `.github/workflows/{default,release}.yml`** — PG 19 cluster in the image, six version loops, two port
  loops, two image-tag references.

### How it works

At connect time `view.Configure(query.Options{Version})` calls each selector, which returns the query
template, column count and diff interval for that server version. The TUI, the recorder and the report
replay path all go through the same `Configure` call — the report path keyed on the version recorded in the
archive's `meta` entry rather than on the live server — so one selector serves all three consumers.

`DiffIntvl` is the only field where a stale value is silently wrong rather than loudly broken: printing and
alignment walk the *result's* own `Ncols`, but a diff interval pointing at the wrong columns produces
plausible nonsense. That is why the selectors are mandatory rather than cosmetic.

## Decisions

### Decision 1: Column positions come from the user-spec, not from the first research pass
**Decision:** New columns are inserted immediately before `a.state` in all three queries — after `relation`
on vacuum and analyze, after `duration` on basebackup. Resulting layouts: vacuum 15 cols `DiffIntvl {12,13}`,
analyze 13 cols `{0,0}`, basebackup 12 cols `{10,10}`.
**Rationale:** This is what the approved user-spec specifies and what the user confirmed. The first
code-research pass (written mid-interview) placed them after `datname`/`pid`; the `(Ncols, DiffIntvl)`
arithmetic is identical either way, so the error is invisible to the tests and would have shipped a layout
nobody approved.
**Alternatives considered:** Appending at the tail — rejected during the interview: the columns read as row
attributes, and the tail is where `query` lives, which horizontal scroll can push out of view.

### Decision 2: All three selectors return the 3-tuple `(string, int, [2]int)`
**Decision:** Uniform arity across the three, including `progress_analyze`, whose `DiffIntvl` stays `{0,0}`.
**Rationale:** ADR [007] requires the 4-tuple only when `UniqueKey` moves; here `pid` stays at column 0 in
every layout, so the 3-tuple is correct. Giving one sibling a 2-tuple would make the family's signatures
differ for a reason invisible at the call site.
**Alternatives considered:** 2-tuple for analyze (`SelectStatActivityQuery` precedent) — rejected: saves
nothing and asymmetry costs a reader's attention on every future edit.

### Decision 3: `describe.go` — rows for the new columns plus a trailing version note
**Decision:** Add a normal description row for each new column and append a trailing
`Note: started_by, mode are available since PostgreSQL 19.`-style line, mirroring the existing note lines in
the bgwriter and IO descriptions.
**Rationale:** The existing precedent for version-varying columns is a trailing `Note:` line with the
columns *not* listed as rows at all. The user-spec requires the new columns to be described, so rows are
mandatory; the note preserves the version information the rows cannot carry. Descriptions stay single flat
strings — no version-aware describe plumbing.
**Alternatives considered:** Rows only (loses the version caveat); note only (violates the spec); a
version-aware describe path (disproportionate — `report -d` is a static help text).

### Decision 4: `e2e.sh` and the two image-tag bumps merge together, after the image is published
**Decision:** `testing/e2e.sh` and the `container:` tag in both workflows land in the same merge, and only
after `lesovsky/pgcenter-testing:0.0.11` is pushed. Everything else may merge earlier.
**Rationale:** ADR [005] decoupled the manual image push from the code merge with defensive `t.Skipf`, and
that still holds for the Go tests, the port map and the version loops — they all skip cleanly against the
old image. It does **not** transfer to `e2e.sh`: CI runs it from the checkout (not from the image), it has
`set -euxo pipefail`, and it has no skip mechanism, so `pgcenter record -p 21919` against a nonexistent
cluster aborts the whole script. `prepare-test-environment.sh` is the mirror case — CI runs the copy baked
into the image, so editing it in the repo is inert until the image ships.
**Alternatives considered:** Reusing [005]'s decoupling wholesale — rejected: it would turn CI red the
moment the `e2e.sh` edit merged. Guarding the e2e loop with a port probe — rejected as machinery for a
one-time ordering problem.

### Decision 5a: The beta apt channel is pinned, not merely added
**Decision:** The beta channel goes into its own `.list` file with `signed-by=/usr/share/keyrings/pgdg.gpg`
(the key already in the image, same host), carries `Pin-Priority: 100` in an apt preferences file, and the
PG 19 packages are installed with an explicit `-t` target release.
**Rationale:** apt resolves a package by the highest available version across all enabled sources, not by
source priority. Without a pin, enabling the beta channel can silently replace `postgresql-14..18` and
`postgresql-common` itself with beta builds — which would also invalidate the claim that `pg_createcluster`
comes from the stable channel. The pin keeps the beta channel reachable only for the packages explicitly
requested from it. Reusing the existing signing key is correct — same publisher, same host — and
`[trusted=yes]` must not be used.
**Alternatives considered:** Adding the channel unpinned — rejected: it silently changes five existing
clusters. A separate keyring — rejected: same publisher, no security gain.

### Decision 5b: Guard the diff against mixed-width snapshots
**Decision:** Before diffing a matched row pair, skip the pair when the previous snapshot's row is narrower
than the current snapshot's column count. Covered by a test in the same task that builds the two-version
replay harness.
**Rationale:** The diff loop indexes the previous snapshot by the *current* snapshot's column count. That
was unreachable while every view had one fixed width; making the width version-dependent makes a
mixed-width archive representable, and with a matching row key it dereferences past the end of the previous
row — a panic in `pgcenter report -f` on an archive spanning a major upgrade. The feature creates the
reachability, so the feature carries the guard.
**Alternatives considered:** Leaving it — rejected: the crash is introduced by this change, not inherited.
Rewriting the mid-archive width handling properly (the `Aligned`-flag issue noted in Backward
Compatibility) — rejected as a separate concern; the guard prevents the crash without touching the layout
logic.

### Decision 5: `NewTestConnectVersion` returns an error for an unmapped version
**Decision:** Replace the `ports[140000]` fallback with
`fmt.Errorf("postgres version %d has no test cluster port mapping", version)`, no sentinel error type.
**Rationale:** The function's doc comment already documents the error behaviour — the code contradicts its
own contract. Every call site lives in a test file and passes a written-out literal; the union of the values
passed is a subset of the port map, so no caller changes behaviour (the exact count is to be re-derived
during implementation rather than trusted from this document). Without this, a forgotten map entry makes every "PG 19" subtest pass while exercising
PG 14.
**Alternatives considered:** Keeping the fallback and covering it with an acceptance check — rejected by the
user: fix the cause rather than guard it.

### Decision 6: `progress_cluster` is not touched, and REPACK is verified through `a.query`
**Decision:** No code change for the PG 19 `REPACK` command. The manual check confirms a running `REPACK`
appears on the cluster progress screen.
**Rationale:** All nine columns the query reads still exist in PG 19's backwards-compatibility
`pg_stat_progress_cluster` view. pgcenter never selects that view's `command` column, so the REPACK→CLUSTER
translation is invisible on screen: the screen shows `REPACK …` in `a.query` while `psql` reports
`command = CLUSTER`. This mismatch is expected and is documented here so it is not re-litigated as a bug.
**Alternatives considered:** Adding `pg_stat_progress_repack` — out of scope by the user-spec; it is a new
screen, recorded in the roadmap backlog.

### Decision 7: The nine tests that skip above their version loop are left alone
**Decision:** Do not restructure the tests whose `t.Skipf` sits above the version loop
(`common_test.go` ×4, `overview_test.go` ×4, `internal/stat/postgres_test.go`). Record the finding as tech
debt at feature finalization.
**Rationale:** In those tests one unavailable version skips the entire test rather than one subtest, so
`common_test.go`'s full-range list already dead-skips in CI today at `90500` — a pre-existing coverage hole,
not one this feature opens. Fixing it means restructuring nine tests in files this feature otherwise only
appends a literal to, which is a refactor with its own review surface.
**Alternatives considered:** Fixing it here — rejected as scope creep into unrelated tests; deliberately
omitting `190000` from those lists — rejected: it would hide PG 19 from tests that are supposed to cover it
once the hole is fixed.

### Decision 8: Per-version query constants, never NULL-padded unified columns
**Decision:** Two constants per screen; the pre-19 constant is untouched.
**Rationale:** ADR [004] settled this for `bgwriter`: a `NULL AS started_by` column on PG 18 would show a
permanently blank column to users of a version that simply does not have the data.

### Decision 9: Autopilot assumption — branch and task granularity
**Decision:** Work happens on `feature/pg19-compatibility-baseline`. The three progress screens are one task
rather than three, and the test-suite sweep is a separate task that owns every `_test.go` file the selector
task does not.
**Rationale:** Autopilot default branch strategy. The three screens share `internal/view/view.go`, so
parallel tasks would collide in the same file; file ownership is stated per task so waves can run
concurrently without conflicts.

### Deferred items to record at feature finalization

Carried from the user-spec so `/done` files them rather than losing them:

- **Remove the beta apt channel** from the test image at the PG 19 GA rebuild — while it is present, PG 19
  keeps arriving from the beta channel on every rebuild, including the one meant to verify GA.
- **`delay_time`** on the vacuum and analyze progress views — deliberately out of scope (a PG 18 column,
  zero without `track_cost_delay_timing`); revisit separately.
- **The nine tests that skip above their version loop** (Decision 7) — a pre-existing coverage hole.
- **Handoff to [014]:** `mode` as a column name is already used on the replication screen, and [014]'s
  colorization rules key off column names, so a rule named `mode` would hit both screens.

## Data Models

No persistent schema. The affected in-memory layouts:

| View | PG ≤ 18 | PG 19 | New columns (position) |
|---|---|---|---|
| `progress_vacuum` | 13 cols, `DiffIntvl {10,11}` | 15 cols, `{12,13}` | `started_by`, `mode` (after `relation`) |
| `progress_analyze` | 12 cols, `{0,0}` | 13 cols, `{0,0}` | `started_by` (after `relation`) |
| `progress_basebackup` | 11 cols, `{9,9}` | 12 cols, `{10,10}` | `backup_type` (after `duration`) |

`OrderKey` stays 0 and `UniqueKey` stays 0 (`pid` is column 0 in every layout).

Value domains, verified against the PG 19 catalog documentation: `mode` ∈ {`normal`, `aggressive`,
`failsafe`}; vacuum `started_by` ∈ {`manual`, `autovacuum`, `autovacuum_wraparound`}; analyze `started_by` ∈
{`manual`, `autovacuum`}; `backup_type` ∈ {`full`, `incremental`}. NULL renders blank — the columns sit
outside `DiffIntvl`, so no `coalesce` is needed and none should be added.

## Dependencies

### New packages
None.

### Using existing (from project)
- `internal/query` — version constants, `Format`, the selector idiom from `io.go` / `bgwriter.go`.
- `internal/view` — `Configure()` patch point; `New()` static defaults.
- `report/` — the version-aware replay path and the describe texts.
- `internal/postgres/testing.go` — the sole test entry point to a specific cluster.

### External
- PG 19 beta packages from the `pgdg-testing` apt channel. `pg_createcluster` comes from
  `postgresql-common` in the **main** pgdg channel and must already understand the PG 19 layout — a third
  probe failure mode alongside "packages missing" and "cluster will not start".

## Testing Strategy

**Feature size:** M

### Unit tests
- Three new per-version selector tables (`Test_SelectStatBgwriterQuery` model): for each screen assert the
  query constant, `Ncols` and `DiffIntvl` at the 180000/190000 boundary, plus one older version.
- `NewTestConnectVersion` with a version absent from the map returns an error and a nil connection.
- `internal/view` `TestViews_Configure` gains a 190000 block asserting the three progress views resolve to
  their PG 19 templates with `Ncols` 15/13/12.

### Integration tests
- The 29 live-connection version loops gain `190000`, so every existing query test executes against the
  PG 19 cluster. The three `progress_*_test.go` execution tests must be switched from the bare pre-19
  constant to the selector — otherwise the PG 19 subtest silently runs the PG 18 query.
- Eight per-version assertion tables gain a `190000` row.
- `TestView_VersionOK` and `record/Test_filterViews` gain a `190000` row. The `Test_filterViews` expectation
  must be **derived** from the actual per-view version gates, not copied from a lower row.
- New replay test `Test_app_doReport_ProgressVacuum` with `versionNum` `"180000"` and `"190000"`, built on
  the synthetic in-memory tar harness: tar entry basename equals the report type, ticks exactly one second
  apart, and the same `pid` in both ticks (with `UniqueKey = 0`, differing pids make the test pass while
  diffing nothing). Goldens `report/testdata/report_record_progress_vacuum_pg{18,19}.golden`.
- The three existing progress goldens must not change — their archive carries `version_num = 140000`, which
  takes the pre-19 selector branch. A golden diff is a red flag, not expected churn.

### E2E tests
`testing/e2e.sh` gains port 21919 in both loops; its existing `-Pv -Pa -Pb` arguments then exercise
record→report for the new columns end to end on PG 19. No new e2e script.

## Agent Verification Plan

**Source:** user-spec "Как проверить".

### Verification approach
Automated suites prove the layouts and the replay; a scripted TUI walk proves the screens. The walk runs
`pgcenter top` inside `tmux`, switches screens by hotkey and captures the pane after each switch, so every
screen leaves a text artifact that shows both panics and silently empty columns. Load is generated first —
progress screens are empty on an idle cluster and would otherwise verify nothing.

### Per-task verification
| Task | verify: | What to check |
|------|---------|--------------|
| 1 | bash | Image builds locally; PG 19 cluster starts on 21919; `fixtures.sql` loads |
| 2 | bash | `go test ./internal/postgres/... ./internal/query/...`; unmapped version returns an error |
| 3 | bash | `go test ./internal/query/... ./internal/view/...`; selector tables green on both branches |
| 4 | bash | `make test` — full suite, PG 19 subtests execute (or skip cleanly without the cluster) |
| 5 | bash | `go test ./report/...`; `pgcenter report -d -P v\|a\|b` lists the new columns |
| 6 | bash | `go test ./report/...` — new replay test green, existing progress goldens unchanged |
| 7 | bash | `grep` the three docs for PG 19; no stale "14–18" ranges left |
| 8 | user | Image pushed to DockerHub; CI green against unmodified `develop` |
| 9 | bash | `./testing/e2e.sh` passes including port 21919 |
| 10 | bash + user | Full QA per user-spec: suite, TUI walk, REPACK, lock counter, reachability |

### Tools required
bash, docker, tmux, psql. No MCP tooling.

## Backward Compatibility

**Breaking changes:** no, for every supported direction.

- **PG 14–18 rendering and diffing:** unchanged — the pre-19 selector branch returns exactly today's
  `(query, Ncols, DiffIntvl)`.
- **Replay of pre-0.12 archives:** unchanged. `report` configures the view from the archive's recorded
  version, `view.Ncols` is never read in the report package, and printing/alignment/diffing all walk the
  recorded result's own column count.
- **`report -d` describe texts:** the only user-visible change on old versions — the texts now describe the
  superset of columns, following the existing bgwriter/IO precedent.
- **Forward direction is not compatible and cannot be:** an archive recorded on PG 19 and replayed by a
  pre-0.12 binary diffs the old column pair and prints nonsense rather than an error. Old binaries cannot be
  taught the new layout. Stated as a limitation in the user-spec.

**Migration strategy:** none needed — no config, no state, no archive rewriting. The only operational
migration is the testing image tag, covered by Decision 4's merge order.

**Consumer impact:** `view.Configure` callers (`top/top.go`, `record/record.go`, `report/report.go`) are
unchanged — they already pass the server version. 41 `NewTestConnectVersion` call sites verified: all pass
mapped literals, none changes behaviour.

## Risks

| Risk | Mitigation |
|------|-----------|
| PG 19 beta packages unavailable for jammy | Probe is task 1, before any Go code. Ladder from the user-spec: jammy → noble inside this feature → pause the feature entirely |
| Packages install but the cluster will not start, or `fixtures.sql` fails on `plperlu` | A different failure class from package availability; investigated as a compatibility problem, not treated as probe failure (a base-image change would not fix it) |
| `pg_createcluster` does not know the PG 19 layout | `postgresql-common` comes from the main pgdg channel; verified during the probe alongside package installation |
| Image rebuild breaks PG 14–18 for unrelated reasons | Rebuilt image runs against unmodified `develop` before any feature code merges |
| `e2e.sh` edit merges before the image is published → CI red with no skip path | Decision 4: `e2e.sh` plus both tag bumps merge together, after the push |
| Forgotten port map entry silently tests PG 14 | Decision 5 removes the fallback; covered by a unit test |
| Stale `DiffIntvl` silently corrupts PG 19 report replay | Version-aware selectors plus the two-version replay test |
| Beta catalog drift before GA | Column names re-checked against the live catalog during implementation; GA re-verification is a release finalization item, closed by patch releases |
| The three progress execution tests keep using the bare pre-19 constant | Called out explicitly in the sweep task — otherwise the PG 19 subtest proves nothing |
| The verification pass finds a real breakage on PG 19 | Fixed inside this feature, except where the area is re-entered by a later feature of the same release ([016]/[017]/[018]) — then it is named in this feature's QA report with the executing feature, re-verified at release finalization, and returns here if that feature drops out of 0.12.0 |

## Acceptance Criteria

- [ ] `PostgresV19 = 190000` present; no other production file needs a new version branch.
- [ ] `NewTestConnectVersion` returns an error for an unmapped version; covered by a test; all existing
      callers still compile and behave identically.
- [ ] Three selectors return the documented `(query, Ncols, DiffIntvl)` triples on both branches; unit
      tables cover the 180000/190000 boundary.
- [ ] `view.Configure()` patches all three views; the static `New()` map still holds pre-19 values.
- [ ] Every live-connection version loop includes `190000`; the three progress execution tests call the
      selector, not the bare constant.
- [ ] `TestView_VersionOK` and `Test_filterViews` have derived (not copied) `190000` rows.
- [ ] New replay test green on both `180000` and `190000`; the three existing progress goldens are byte-identical.
- [ ] `report -d -P v|a|b` describes the new columns with the version note.
- [ ] The new query constants contain no template placeholders — they stay static SQL, like the constants
      they sit beside.
- [ ] Installing PG 19 leaves the PG 14–18 package versions in the image unchanged (the beta channel pin
      works).
- [ ] `make test`, `make lint`, `make vuln` clean; `testing/e2e.sh` passes including port 21919.
- [ ] All user-spec acceptance criteria satisfied (verified in the Final Wave QA task).

## Implementation Tasks

### Wave 1 (проба — до любого Go-кода)

#### Task 1: PG 19 probe and test-image environment
- **Description:** Add a PG 19 cluster to the testing image: the beta apt channel in its own source file,
  pinned per Decision 5a so it cannot displace the existing clusters' packages, the PG 19 packages
  installed from it explicitly, and PG 19 in the six version loops of the environment preparation script. Build the
  image locally and confirm the cluster starts and the fixtures load. This is the feature's first task and
  runs before any Go code, because its outcome decides whether the feature proceeds, absorbs a base-image
  migration, or pauses. Does not touch the e2e script or the workflows (Decision 4).
- **Skill:** infrastructure-setup
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-infrastructure-reviewer
- **Verify:** bash — build the image, start the PG 19 cluster, load fixtures
- **Files to modify:** `testing/Dockerfile`, `testing/prepare-test-environment.sh`
- **Files to read:** `testing/fixtures.sql`, `.claude/skills/project-knowledge/deployment.md`

### Wave 2 (зависит от Wave 1)

#### Task 2: Version constant, port mapping, and test-connection hardening
- **Description:** Add the PG 19 version constant and its test cluster port. Make the test-connection helper
  return an error for a version it has no port mapping for, instead of falling back to the oldest cluster
  (Decision 5).
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/postgres/...`
- **Files to modify:** `internal/query/query.go`, `internal/postgres/testing.go`
- **Files to read:** `internal/postgres/postgres.go`, `internal/query/io.go`

### Wave 3 (зависит от Wave 2)

#### Task 3: Version-aware selectors for the three progress screens
- **Description:** Give each of the vacuum, analyze and basebackup progress screens a PG 19 query constant
  with the new columns in the positions the user-spec specifies, a selector returning query plus column
  count plus diff interval, and the matching wiring in the view configuration. Owns the three progress query
  files and their test files, including switching their execution tests from the bare constant to the
  selector.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/query/... ./internal/view/...`
- **Files to modify:** `internal/query/progress_vacuum.go`, `internal/query/progress_analyze.go`,
  `internal/query/progress_basebackup.go`, `internal/query/progress_vacuum_test.go`,
  `internal/query/progress_analyze_test.go`, `internal/query/progress_basebackup_test.go`,
  `internal/view/view.go`
- **Files to read:** `internal/query/io.go`, `internal/query/bgwriter.go`, `internal/query/bgwriter_test.go`,
  `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline.md`

### Wave 4 (зависит от Wave 3)

#### Task 4: Thread PG 19 through the rest of the test suite
- **Description:** Add the PG 19 version to every live-connection version loop and every per-version
  assertion table outside the three progress test files, and add PG 19 rows to the two count-based tests
  that run without a database, deriving their expectations from the actual per-view version gates rather
  than copying a lower row.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `make test`
- **Files to modify:** `internal/query/activity_test.go`, `internal/query/bgwriter_test.go`,
  `internal/query/common_test.go`, `internal/query/databases_test.go`, `internal/query/functions_test.go`,
  `internal/query/indexes_test.go`, `internal/query/io_test.go`, `internal/query/overview_test.go`,
  `internal/query/pgcenter_schema_test.go`, `internal/query/procpidstat_test.go`,
  `internal/query/progress_cluster_test.go`, `internal/query/progress_copy_test.go`,
  `internal/query/progress_create_index_test.go`, `internal/query/replication_slots_test.go`,
  `internal/query/replication_test.go`, `internal/query/sizes_test.go`,
  `internal/query/statements_test.go`, `internal/query/tables_test.go`, `internal/query/wal_test.go`,
  `internal/view/view_test.go`, `record/record_test.go`
  <!-- internal/stat/postgres_test.go is deliberately excluded — owned by Task 6, which also adds its PG 19 row -->

- **Files to read:** `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline-code-research.md`,
  `.claude/skills/project-knowledge/patterns.md`

#### Task 5: Report describe texts for the new columns
- **Description:** Describe the new columns in the three progress report descriptions, each with a trailing
  note naming the PostgreSQL version they appeared in, per Decision 3.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./report/...`; `pgcenter report -d -P v|a|b` lists the new columns
- **Files to modify:** `report/describe.go`
- **Files to read:** `report/report.go`, `report/report_test.go`

#### Task 6: Report replay coverage for the new layout
- **Description:** Add a replay test for the vacuum progress report on both a pre-19 and a PG 19 recording,
  proving the layout is chosen by the version stored in the archive. Also add the mixed-width diff guard
  from Decision 5b with its own case on the same harness, since this feature is what makes a mixed-width
  archive representable. Owns the stats-package test file, so it also adds that file's PG 19 version entry.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./report/... ./internal/stat/...`; the three existing progress goldens unchanged
- **Files to modify:** `report/report_record_progress_vacuum_test.go` (new), `report/testdata/` (new goldens),
  `internal/stat/postgres.go`, `internal/stat/postgres_test.go`
- **Files to read:** `report/report_record_bgwriter_test.go`, `report/report.go`

#### Task 7: Documentation update
- **Description:** Record PG 19 support in the project knowledge base: supported versions, the test image
  contents and port list, and the query selector inventory gaining the three progress selectors.
- **Skill:** documentation-writing
- **Reviewers:** dev-code-reviewer
- **Verify:** bash — no stale "14–18" version ranges remain in the three files
- **Files to modify:** `.claude/skills/project-knowledge/overview.md`,
  `.claude/skills/project-knowledge/deployment.md`, `.claude/skills/project-knowledge/architecture.md`
- **Files to read:** `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline.md`

### Wave 5 (зависит от Wave 4 — действие пользователя)

#### Task 8: Publish the test image (user action)
- **Description:** Build and push the new testing image tag to DockerHub, then run it against unmodified
  `develop` to confirm the rebuild did not break PG 14–18 on its own. Requires the maintainer's registry
  credentials, so it cannot be automated. Task 9 and the Final Wave depend on it.
- **Skill:** none — user instruction
- **Reviewers:** none
- **Verify:** user — image available in the registry; CI green on unmodified `develop`
- **Files to modify:** none
- **Files to read:** `.claude/skills/project-knowledge/deployment.md`, `testing/Dockerfile`

### Wave 6 (зависит от Wave 5)

#### Task 9: Switch CI to the new image and extend the e2e script
- **Description:** Point both workflows at the new image tag and add the PG 19 port to the end-to-end
  script. Both land together and only after the image is published, per Decision 4.
- **Skill:** deploy-pipeline
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-deploy-reviewer
- **Verify:** bash — `./testing/e2e.sh` passes including the PG 19 port
- **Files to modify:** `testing/e2e.sh`, `.github/workflows/default.yml`, `.github/workflows/release.yml`
- **Files to read:** `testing/prepare-test-environment.sh`

### Final Wave

#### Task 10: Pre-deploy QA
- **Description:** Acceptance testing against the user-spec and this tech-spec. Automated: full suite, lint,
  vulnerability check, end-to-end script. Manual on live PG 19: a scripted walk over all 27 registered
  screens (including both `pg_stat_io` sub-screens, all seven `pg_stat_statements` sub-screens and all six
  progress screens, with the per-process screen checked over a local connection) under generated load, the
  new columns cross-checked against psql, the REPACK backwards-compatibility check, the lock-counter check
  and the version-reachability check. Any breakage found is routed per the Risks table rather than silently
  accepted.
- **Skill:** pre-deploy-qa
- **Reviewers:** none
- **Verify:** bash + user — full QA per the user-spec "Как проверить" section
