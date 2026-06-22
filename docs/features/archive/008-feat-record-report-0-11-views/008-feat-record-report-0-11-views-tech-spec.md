---
created: 2026-06-22
status: approved
branch: feature/record-report-0-11-views
size: M
---

# Tech Spec: record/report for the 0.11.0 screens (bgwriter, replslots, pg_stat_io, JIT)

## Solution

Lift `NotRecordable: true` from the four 0.11.0 screens (5 report types: `bgwriter`,
`replslots`, `stat_io`, `stat_io_time`, `statements_jit`) so `pgcenter record` collects them
through the existing pure-SQL recording path and `pgcenter report` replays them through the
existing report pipeline. These are pure-SQL views — the recorder, the tar storage format, and
the report replay engine need **no changes**; the work is removing a single boolean per view,
wiring the CLI/describe surfaces, fixing the count-based tests that the removal shifts, adding
golden-based replay tests, and paying off two adjacent tech-debt items.

The report replay engine already configures all 5 views per recorded version
(`view.Configure(Options{Version})`) and already diffs recorded cumulative samples. The only
genuinely new artifacts are: 3 CLI flags, 5 describe strings, and per-screen golden replay
tests fed by a synthetic in-memory tar.

## Architecture

### What we're building/modifying

- **`internal/view/view.go`** — remove `NotRecordable: true` from the 5 view definitions. This
  is the one change that makes `record/filterViews` stop dropping them.
- **`cmd/report/report.go`** — 3 new report-selection flags (`-B`/`--bgwriter`,
  `-L`/`--replslots`, `-J`/`--io` with `c|t`) plus extending `-X`/`--statements` with value `j`.
- **`report/describe.go` + `report/report.go`** — 5 new column-description constants and 5 new
  entries in the `describeReport` map.
- **Tests** — update the count-based tests shifted by the boolean removal (`view_test`,
  `Test_filterViews`); add per-screen golden replay tests (new `_test.go` files) and per-screen
  golden fixtures; add CLI/describe unit cases.
- **Tech-debt payoff** — [007] behavioral diff test on zero-filled cells; [004] export the
  procpidstat column-index constants and dedupe the report-side copy.
- **Docs** — `overview.md`, `architecture.md`, features-catalog: drop the "TUI-only /
  NotRecordable" claims for the 4 screens.

### How it works

Record path (unchanged engine): `record.app.setup()` builds full `query.Options`, `filterViews`
keeps every recordable view, the recorder runs each view's SQL and writes
`{reporttype}.{TS}.json` per tick. Once `NotRecordable` is gone, the 5 new types ride this path
exactly like `wal`/`tables`. The procpidstat enrichment branch is gated on the literal
`"procpidstat"` key and never fires for them.

Report path (unchanged engine): `report` reads the tar, and on the first sample (and on any
version change) calls `view.Configure(Options{Version})`, which selects the version-correct
layout (`Ncols`/`DiffIntvl`/`UniqueKey`) for the version-aware screens. It then diffs
consecutive recorded samples and prints. The rebuilt SQL string is never executed in report, so
the partial `Options` (only `Version`) is harmless.

## Decisions

### Decision 1: Recorder and storage format unchanged — lift NotRecordable only
**Decision:** Implement collection by removing `NotRecordable: true`; do not touch the recorder
or tar format.
**Rationale:** These are pure-SQL views; the existing SQL collect/write path handles them like
any other recordable view. The procpidstat enrichment (the only stateful branch) is keyed on
its own view name.
**Alternatives considered:** A procpidstat-style stateful recorder path — rejected: there is no
procfs/derived data here, so it would be dead complexity.

### Decision 2: CLI mapping — reuse `-X` for JIT, one string flag for the two IO screens
**Decision:** `-B`/`--bgwriter` (bool), `-L`/`--replslots` (bool), `-J`/`--io` (string `c|t` →
`stat_io`/`stat_io_time`), and extend `-X`/`--statements` with `j` → `statements_jit`.
**Rationale:** Mirrors the existing flag idioms: bool flags for single screens (`-W`), a string
sub-selector for a family of screens (`-X`, `-D`, `-P`). JIT is a pg_stat_statements sub-screen,
so it belongs under `-X` like the TUI `x`-cycle. Short letters `B`/`L`/`J` are free.
**Alternatives considered:** Two bool flags for the IO screens — rejected: breaks the
string-subselector idiom and burns two letters. A new flag for JIT — rejected: it is a
statements sub-screen.

### Decision 3: Replay tests via synthetic in-memory tar + golden files, not the legacy fixture
**Decision:** Verify replay diffs with golden files fed by a purpose-built synthetic in-memory
tar (the `Test_app_doReport_procpidstat` pattern). Version-aware screens (bgwriter, stat_io,
statements_jit) get golden variants at recorded `version` 14/17/18; version-independent screens
(replslots, stat_io_time) get one. Each screen's replay test lives in its **own new `_test.go`
file** with its **own golden fixtures**.
**Rationale:** The legacy `testdata/pgcenter.stat.golden.tar` predates these views, is a ~PG13
recording that cannot hold modern-view data, and regenerating it would churn all 30 existing
goldens. Synthetic input is version-parametric for free and exercises the report-time
`Configure` layout switch directly. Separate test files + separate goldens keep the per-screen
tasks conflict-free for parallel execution.
**Alternatives considered:** Live PG14-18 record→report end-to-end — rejected (interview Q6): no
such harness exists and building one is disproportionate; the seam is covered by manual check.
Regenerating the shared fixture — rejected (mass golden churn).

### Decision 4: report-time Configure consumes only `Version`
**Decision:** Rely on the existing `Configure(Options{Version})` call; do not thread extra
options into report metadata.
**Rationale:** All 5 selectors branch on version only (replslots and stat_io_time ignore version
entirely); the rebuilt SQL is never executed in report. Confirmed in code research §8.
**Alternatives considered:** Persist recovery/track/pgss into meta for report — rejected:
unused by these selectors; needless format change.

### Decision 5: No remote gate, no special no-data handling
**Decision:** No local/remote gate (unlike procpidstat); empty/old archives print header-only
like `tables`/`wal`, no INFO/WARNING.
**Rationale:** Pure-SQL views are available remotely; an empty result is a normal report state,
not an error condition.
**Alternatives considered:** A friendly "no data" INFO for JIT — rejected (interview Q2):
inconsistent with existing SQL-view reports.

### Decision 6: Pay tech-debt [007] and [004]; defer [006] and [005]
**Decision:** Add the behavioral zero-cell diff test [007] and export/dedupe the procpidstat
column-index constants [004]. Leave [006] (replslots standby) and [005] (`top/reload_test.go`
panic) as debt.
**Rationale:** [007] is directly on this feature's path (report replays zero-filled cells
through the diff engine); [004] is in `report/report.go`, which this feature edits anyway. [006]
needs a live standby (orthogonal to recording); [005] is unrelated to record/report.
**Alternatives considered:** Defer all debt — rejected: [007]/[004] are cheap and on-path.

## Data Models

No new data models. Recorded entries reuse `stat.PGresult` serialized to
`{reporttype}.{TS}.json` (existing format). No tar-format version change.

## Dependencies

### New packages
- None.

### Using existing (from project)
- `internal/view` — `view.New()` definitions and `Views.Configure` (already wires all 5 selectors).
- `internal/query` — version-aware selectors `SelectStatBgwriterQuery`,
  `SelectStatReplicationSlotsQuery`, `SelectStatIOQuery`, `SelectStatIOTimeQuery`,
  `SelectStatStatementsJITQuery` (already exist, already version-correct).
- `record/recorder.go` — pure-SQL collect/write path (unchanged).
- `report/report.go` — replay pipeline `doReport`/`processData`/`countDiff` (unchanged engine).
- `internal/stat` — `diff`/`Compare` (debt [007] test target); procpidstat column constants
  (debt [004] export source).

## Testing Strategy

**Feature size:** M

### Unit tests
- `Test_filterViews` (record): update the per-version counts shifted by lifting NotRecordable —
  only the PG14 rows change; lower-version rows are unchanged (the views are version-gated).
  Rewrite the now-inverted explanatory comment.
- `view_test`: flip the 5 `NotRecordable` assertions and fix their comments; total view count
  (`TestNew`) is unchanged.
- `Test_selectReport` (cmd/report): add cases for `-B`, `-L`, `-J c`, `-J t`, `-X j`.
- `Test_describeReport` (report): add cases for the 5 new report types.
- Keep the synthetic drop-branch test (`TestFilterViews_dropsExplicitNotRecordable`) — after
  this feature it is the only coverage of the NotRecordable mechanism.
- [007] behavioral diff test: feed zero-filled diffed cells (the coalesced recorded value)
  through `diff()`/`Compare` and assert clean zero deltas with correct UniqueKey row matching.

### Integration tests
- Per-screen golden replay tests fed by a synthetic in-memory tar (2 cumulative ticks + meta):
  `bgwriter` (versions 14/17/18), `replslots` (one version + a zero-slots/empty case),
  `stat_io` (versions 16/18) + `stat_io_time` (one version), `statements_jit` (versions 15/17).
  Each in its own `report/report_record_<screen>_test.go` with its own golden fixtures.

### E2E tests
- None. No live record→report harness exists; the synthetic-tar + golden tests cover replay
  diffs version-parametrically. The live record↔report seam is covered by the manual check in
  the user-spec.

## Agent Verification Plan

**Source:** user-spec "Как проверить" section.

### Verification approach
Automated gates carry the proof: `make test` (race + coverage), `make lint`, `make vuln`, plus
the new golden replay tests across the version variants. The CI PG14-18 matrix is the final gate.

### Per-task verification
| Task | verify: | What to check |
|------|---------|--------------|
| 1 | bash | `go test ./internal/view/... ./record/...` — view/filter count tests green |
| 2 | bash | `go test ./report/... -run Test_describeReport` — 5 new describe cases pass |
| 3 | bash | `go test ./cmd/report/...` — selectReport maps the 5 new flag forms |
| 4 | bash | `go test ./report/... -run Bgwriter` — bgwriter golden replay matches (14/17/18) |
| 5 | bash | `go test ./report/... -run ReplSlots` — replslots golden + zero-slots empty case |
| 6 | bash | `go test ./report/... -run StatIO` — stat_io (16/18) + stat_io_time goldens match |
| 7 | bash | `go test ./report/... -run StatementsJIT` — JIT golden replay matches (15/17) |
| 8 | bash | `go test ./internal/stat/... -run Diff` — zero-cell diff behavioral test passes |
| 9 | bash | `go test ./report/... ./internal/stat/...` — dedup compiles, warnings test green |
| 10 | bash | `make lint` + manual doc read — claims removed |
| 11 | bash | `make test && make lint && make vuln` — full gate green |

### Tools required
bash (go test, make). No MCP/Playwright/curl — this is a CLI tool with no network surface.

## Backward Compatibility

**Breaking changes:** no.

**Migration strategy:** N/A — no format/API break. New CLI flags are additive. New archives gain
extra entries; old archives lack them and report header-only for the new flags. The tar format
version is unchanged.

**DB migration compatibility:** N/A — no DB schema involved.

**Consumer impact:** `report/report.go`'s `describeReport` map and `cmd/report` options gain
entries (additive). `internal/stat/procpidstat.go` gains exported index constants; the report
package switches from local copies to the exported ones ([004]) — internal refactor, no external
consumer (pgcenter is an application, not a library).

## Risks

| Risk | Mitigation |
|------|-----------|
| Report-time `Configure` builds invalid SQL for replslots/JIT under partial `Options` | SQL is not executed in report — only layout fields are read; `text/template` tolerates empty values. Covered by golden replay tests. |
| `Test_filterViews` per-version counts shift unevenly (version-gated views) | Only PG14 rows change; lower-version rows verified unchanged in code research; CI PG14-18 gate catches drift. |
| Parallel per-screen tasks editing the same file | Per-screen golden tests live in separate new `_test.go` files with separate goldens; shared report wiring isolated to one task. |
| Recorded zero-filled cells abort the diff on replay | Recorded SQL already coalesces NULL→0, so stored cells are `"0"`; behavioral test [007] proves `diff()` survives them. |
| [004] dedup touches `report/report.go` concurrently with the describe-wiring task | Sequenced into a later wave than the describe-wiring task (disjoint timing). |

## Acceptance Criteria

Технические критерии приёмки (дополняют пользовательские из user-spec):

- [ ] `NotRecordable` removed from the 5 view definitions; `Test_filterViews` and `view_test`
      updated and green; `TestNew` unchanged at 27.
- [ ] `pgcenter record` produces a tar entry per new report type per tick (recorder unchanged).
- [ ] `report` flags `-B`/`-L`/`-J c|t`/`-X j` map to the correct report types; unknown
      `-J`/`-X` values error like existing string flags.
- [ ] `report -d <flag>` prints a column description for each of the 5 types.
- [ ] Per-screen golden replay tests pass, including version variants (bgwriter 14/17/18,
      stat_io 16/18, statements_jit 15/17) and the replslots zero-slots empty case.
- [ ] Tech-debt [007] behavioral diff test added and green; [004] column-index constants
      exported from `internal/stat` and the report-side copy removed.
- [ ] Docs (`overview.md`, `architecture.md`, features-catalog) no longer claim the 4 screens
      are TUI-only / NotRecordable.
- [ ] No regressions: `make test` / `make lint` / `make vuln` and CI PG14-18 all green.

## Implementation Tasks

### Wave 1 (независимые — непересекающиеся файлы, параллельно)

#### Task 1: Enable recording + fix view/filter count tests
- **Description:** Remove `NotRecordable: true` from the 5 view definitions so the recorder
  collects them, and update the count-based tests that the removal shifts: all 5 per-view
  `NotRecordable` assertions and their comments in `view_test`, and the per-version counts +
  the now-inverted explanatory comment in `Test_filterViews`. Also fix the stale comment in
  `record.go` that names bgwriter as a NotRecordable example. This is the single change that
  turns the four screens into recordable views.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./internal/view/... ./record/...`
- **Files to modify:** `internal/view/view.go`, `internal/view/view_test.go`, `record/record.go`, `record/record_test.go`
- **Files to read:** `docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-code-research.md`

#### Task 2: Report describe wiring for the 5 new types
- **Description:** Add 5 column-description constants and register them in the `describeReport`
  map so `report -d` works for the new screens. Add the matching `Test_describeReport` cases.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./report/... -run Test_describeReport`
- **Files to modify:** `report/describe.go`, `report/report.go`, `report/report_test.go`
- **Files to read:** `internal/view/view.go`, `docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-code-research.md`

#### Task 3: CLI report flags for the new screens
- **Description:** Add the report-selection flags `-B`/`--bgwriter`, `-L`/`--replslots`,
  `-J`/`--io` (`c|t`) and extend `-X`/`--statements` with value `j`, wiring them through
  `selectReport`. Add the `Test_selectReport` cases for all five forms plus an invalid-value
  case (`-J x` / `-X z`) that must resolve to the empty-string error path.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./cmd/report/...`
- **Files to modify:** `cmd/report/report.go`, `cmd/report/report_test.go`
- **Files to read:** `report/report.go`, `docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-code-research.md`

### Wave 2 (per-screen golden replay — отдельные test-файлы, параллельно; после Wave 1)

#### Task 4: bgwriter replay golden tests
- **Description:** Add a golden-based replay test for the `bgwriter` report fed by a synthetic
  in-memory tar, with golden variants at recorded version 14/17/18 to prove the version-aware
  layout switch. Verifies cumulative counters diff correctly and absolute/text columns pass
  through.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./report/... -run Bgwriter`
- **Files to modify:** `report/report_record_bgwriter_test.go`, `report/testdata/` (new goldens)
- **Files to read:** `report/report_test.go`, `internal/query/bgwriter.go`, `docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-code-research.md`

#### Task 5: replslots replay golden test
- **Description:** Add a golden-based replay test for the `replslots` report fed by a synthetic
  in-memory tar (single version — selector is version-independent), including a zero-slots
  empty-archive case that must print header-only. Verifies coalesced (zero-filled) counters
  diff cleanly, that rows match across samples on the `slot_name` identity (UniqueKey col 0),
  and that the retained-WAL sort order holds.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./report/... -run ReplSlots`
- **Files to modify:** `report/report_record_replslots_test.go`, `report/testdata/` (new goldens)
- **Files to read:** `report/report_test.go`, `internal/query/replication_slots.go`, `docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-code-research.md`

#### Task 6: pg_stat_io replay golden tests (count + time)
- **Description:** Add golden-based replay tests for `stat_io` (version variants 16/18 for the
  bytes-derivation difference) and `stat_io_time` (single version), fed by synthetic in-memory
  tars. Verifies the synthetic md5 `io_key` row identity matches across samples and the diff is
  correct.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./report/... -run StatIO`
- **Files to modify:** `report/report_record_statio_test.go`, `report/testdata/` (new goldens)
- **Files to read:** `report/report_test.go`, `internal/query/io.go`, `docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-code-research.md`

#### Task 7: statements_jit replay golden tests
- **Description:** Add a golden-based replay test for the `statements_jit` report fed by a
  synthetic in-memory tar, with golden variants at recorded version 15/17 to cover the column-
  count change (the version-shifting trailing UniqueKey). Verifies phase-time deltas diff
  correctly.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./report/... -run StatementsJIT`
- **Files to modify:** `report/report_record_statements_jit_test.go`, `report/testdata/` (new goldens)
- **Files to read:** `report/report_test.go`, `internal/query/statements.go`, `docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-code-research.md`

### Wave 3 (техдолг + доки — непересекающиеся файлы, параллельно; после Wave 1)

#### Task 8: Tech-debt [007] — behavioral zero-cell diff test
- **Description:** Add a behavioral test next to `Test_diff` that feeds zero-filled diffed cells
  (the coalesced value the recorder stores for pg_stat_io/replslots) through `diff()`/`Compare`
  and asserts clean zero deltas with correct UniqueKey row matching — closing the unverified
  behavioral half of the coalesce contract.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./internal/stat/... -run Diff`
- **Files to modify:** `internal/stat/postgres_test.go`
- **Files to read:** `internal/stat/postgres.go`, `docs/tech-debt.md`, `docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-code-research.md`

#### Task 9: Tech-debt [004] — export procpidstat column-index constants
- **Description:** Export the procpidstat IO/iodelay column-index constants from
  `internal/stat/procpidstat.go` and replace the duplicated local copy in `report/report.go`
  with references to them, removing the cross-package drift risk.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./report/... ./internal/stat/...`
- **Files to modify:** `internal/stat/procpidstat.go`, `report/report.go`
- **Files to read:** `docs/tech-debt.md`, `docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-code-research.md`

#### Task 10: Update documentation
- **Description:** Update `overview.md`, `architecture.md`, and the features-catalog so they no
  longer describe the four screens as TUI-only / NotRecordable, and document the new
  record/report capability and CLI flags.
- **Skill:** documentation-writing
- **Reviewers:** dev-code-reviewer
- **Verify:** bash — manual doc read; `make lint`
- **Files to modify:** `.claude/skills/project-knowledge/overview.md`, `.claude/skills/project-knowledge/architecture.md`, `docs/features-catalog.md`
- **Files to read:** `docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views.md`

### Final Wave

#### Task 11: Pre-deploy QA
- **Description:** Acceptance testing: run the full gate (`make test` / `make lint` /
  `make vuln`), verify the user-spec and tech-spec acceptance criteria, and do the manual
  record→report check against a live PG (compare one cumulative screen's report deltas with the
  TUI).
- **Skill:** pre-deploy-qa
- **Reviewers:** none
