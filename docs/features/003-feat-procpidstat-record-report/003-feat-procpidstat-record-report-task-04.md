---
status: planned
depends_on: ["02", "03"]
wave: 3
skills: [code-writing]
verify: "bash — make test → all green; make lint → no new warnings; ./bin/pgcenter record -c 3 -i 1s -f /tmp/test.tar && ./bin/pgcenter report -N -f /tmp/test.tar → ≥1 line output"
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
teammate_name:
---

# Task 04: Test suite update

## Required Skills

Before starting, load: `/skill:code-writing` — [SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Tasks 02 and 03 changed two behavioral invariants that existing tests assert: (1) `procpidstat`
was filtered out by `filterViews` because `NotRecordable=true` — task 03 removes that flag, so
`filterViews` no longer drops it; (2) `tarRecorder.write()` now emits a `sysinfo.TIMESTAMP.json`
entry per tick in addition to stats entries, changing the file count from `countRecordable()+1`
to `countRecordable()+2` per tick.

This task fixes those breakages and adds the two new unit tests specified in the tech-spec
(`Test_readMeta_with_sysinfo`, `Test_app_doReport_procpidstat`) using synthetic in-memory tars,
so no real PostgreSQL connection or live procfs data is required. If the existing golden tar
`report/testdata/pgcenter.stat.golden.tar` triggers failures in `Test_app_doReport` after task 03
changes to `isFilenameOK` (it now accepts `sysinfo` prefix), the golden file must be regenerated
via `go test ./report/... -update`.

The task is purely test code — no production logic changes.

## What to do

1. In `record/record_test.go`:
   - Update `TestFilterViews_NotRecordable`: the view under test must have `NotRecordable: false`
     (the field is now `false` in `view.New()`); the trailing `assert.True(t, pp.NotRecordable)`
     must become `assert.False`; assert that `filterViews` does NOT remove it (`wantN=0`, `wantV=1`).
   - Update `Test_filterViews` table: decrement `wantN` by 1 and increment `wantV` by 1 in every
     row (procpidstat now passes through `filterViews`).
   - Update `Test_app_record`: change the `totalViews` formula from `countRecordable(view.New()) + 1`
     to `countRecordable(view.New()) + 2` (meta + sysinfo per tick). Update the comment above it.

2. In `report/report_test.go`:
   - Add `Test_readMeta_with_sysinfo`: build an in-memory `tar.Writer` with a single
     `sysinfo.20260519T100000.000.json` entry containing `{"ticks":100,"cpu_count":4}` and a
     matching `meta.*` entry; run `readTar`; assert `metadata.ticks==100` and
     `metadata.cpuCount==4`.
   - Add `Test_app_doReport_procpidstat`: build an in-memory tar with two `procpidstat.*` entries
     (snapshot 1: first-tick zeros for rate cols; snapshot 2: real non-zero values) plus one
     `sysinfo.*` entry and one `meta.*` entry; construct `Config{ReportType: "procpidstat", ...}`;
     run `app.doReport`; assert the output contains a non-empty timestamp line and at least one
     data row.

3. Regenerate `report/testdata/pgcenter.stat.golden.tar` if any existing `Test_app_doReport`
   sub-test fails after the task 03 pipeline changes (run `go test ./report/... -update` and
   commit the updated tar).

4. Run `make test` and confirm all tests pass. Run `make lint` and confirm no new warnings.

5. Build and run the E2E smoke check:
   `make build && ./bin/pgcenter record -c 3 -i 1s -f /tmp/test.tar && ./bin/pgcenter report -N -f /tmp/test.tar`
   — confirm output contains at least one line with a timestamp.

## TDD Anchor

Write the new test first, verify it fails (because task 02/03 code is not yet merged or because
the assertions are wrong in the old state), then confirm it passes after all wave-2 changes are in.

- `report/report_test.go::Test_app_doReport_procpidstat` — `doReport` on a synthetic tar with
  two procpidstat snapshots + sysinfo + meta produces non-empty output (timestamp line + data row)

The record-side changes (`TestFilterViews_NotRecordable`, `Test_filterViews`, `Test_app_record`)
are updates to existing tests, not new ones. Run `make test` first to see which assertions fail,
then fix them.

## Acceptance Criteria

- [ ] `make test` passes — no new failures
- [ ] `make lint` passes — no new warnings
- [ ] `TestFilterViews_NotRecordable` asserts procpidstat passes through `filterViews` (not filtered)
- [ ] The trailing assertion in `TestFilterViews_NotRecordable` is `assert.False(t, pp.NotRecordable)`
- [ ] `Test_filterViews` table rows: each `wantN` decreased by 1, each `wantV` increased by 1
- [ ] `Test_app_record` formula: `countRecordable(view.New()) + 2` (not +1)
- [ ] `Test_readMeta_with_sysinfo` exists and passes
- [ ] `Test_app_doReport_procpidstat` exists and passes
- [ ] E2E smoke: `./bin/pgcenter report -N -f /tmp/test.tar` produces ≥1 line of output

## Context Files

**Feature artifacts:**
- [003-feat-procpidstat-record-report.md](docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report.md) — user-spec
- [003-feat-procpidstat-record-report-tech-spec.md](docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report-tech-spec.md) — tech-spec
- [003-feat-procpidstat-record-report-decisions.md](docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report-decisions.md) — decisions log

**Project knowledge:**
- [project.md](.claude/skills/project-knowledge/overview.md)
- [architecture.md](.claude/skills/project-knowledge/architecture.md)
- [patterns.md](.claude/skills/project-knowledge/patterns.md)

**Code files (modify):**
- [record/record_test.go](record/record_test.go) — fix `TestFilterViews_NotRecordable`, `Test_filterViews`, `Test_app_record`
- [report/report_test.go](report/report_test.go) — add `Test_readMeta_with_sysinfo`, `Test_app_doReport_procpidstat`
- [report/testdata/](report/testdata/) — regenerate `pgcenter.stat.golden.tar` if needed

**Code files (read for context):**
- [record/record.go](record/record.go) — `filterViews` logic (gate moved to `app.setup()`; `NotRecordable` check still present in filter but flag changed in view)
- [report/report.go](report/report.go) — `readTar`, `processData`, `metadata` struct (ticks/cpuCount fields added in task 03)

## Verification Steps

1. `make test` — all tests green (including new and updated ones)
2. `make lint` — no new warnings compared to baseline
3. `make build` — binary builds successfully
4. `./bin/pgcenter record -c 3 -i 1s -f /tmp/test.tar` — exits cleanly, file created
5. `./bin/pgcenter report -N -f /tmp/test.tar` — outputs at least one timestamp line and one data row

## Details

**Files — current state and what to change:**

`record/record_test.go` — currently has three test functions that break after task 02/03:

- `TestFilterViews_NotRecordable` (line 125): constructs a Views map with `NotRecordable: true`
  and asserts `filterViews` drops it. After task 03 removes `NotRecordable: true` from `view.New()`,
  this test must be inverted: use `NotRecordable: false`, assert `n==0, len(v)==1`.
  The sanity-check block at the bottom (lines 140-145) asserts `pp.NotRecordable` is `true` — must
  become `false`. The comment on line 126-127 must be updated accordingly.

- `Test_filterViews` (line 100): table rows have these values:
  `{140000, "",      7, 15}`, `{140000, "public", 1, 21}`, `{130000, "public", 4, 18}`,
  `{120000, "public", 7, 15}`, `{110000, "public", 9, 13}`, `{100000, "public", 9, 13}`
  After procpidstat passes through, each row becomes:
  `{140000, "",      6, 16}`, `{140000, "public", 0, 22}`, `{130000, "public", 3, 19}`,
  `{120000, "public", 6, 16}`, `{110000, "public", 8, 14}`, `{100000, "public", 8, 14}`
  The comment block above the table (lines 107-110) must be updated to reflect that procpidstat
  is no longer in the filtered count.

- `Test_app_record` (line 32): `totalViews := countRecordable(view.New()) + 1`. After task 02
  adds sysinfo per tick, change to `countRecordable(view.New()) + 2`. Also update the comment
  (line 34-35) to say `+2` accounts for meta + sysinfo.

`report/report_test.go` — two new tests to add:

- `Test_readMeta_with_sysinfo`: needs `archive/tar`, `bytes`, `encoding/json`. Create a
  `bytes.Buffer`, wrap with `tar.NewWriter`. Add a `meta.*` entry (use an existing valid
  PGresult JSON from `Test_readMeta` as payload). Add a `sysinfo.20260519T100000.000.json`
  entry with content `{"ticks":100,"cpu_count":4}`. Close the tar writer. Wrap the buffer
  with `tar.NewReader`. Call `readTar` with a `Config{ReportType: "procpidstat", ...}` and
  collect `data` items from the channel. Assert the `metadata` in the received data item has
  `ticks==100.0` and `cpuCount==4`. Note: `readTar` sends on `dataCh` only when both `metaOK`
  and `statOK` are true — for procpidstat report the stat entry has prefix `"procpidstat"`, so
  add a `procpidstat.20260519T100000.000.json` entry alongside meta and sysinfo.

- `Test_app_doReport_procpidstat`: needs two snapshots so `processData` has a prev/curr pair.
  Snapshot 1 (first tick): a valid 19-col `PGresult` for procpidstat with all rate columns set
  to `"0"` (or any value — it will be discarded as prev). Snapshot 2: same structure, non-zero
  values in at least one column. Both must be valid JSON that `stat.NewPGresultFile` can decode.
  Use timestamps in 2026 within the time range you set in `Config.TsStart`/`TsEnd`. The test
  runs `app.doReport(tr)` and asserts `buf.String()` is non-empty and contains a `/` (timestamp
  line character).

**Dependencies:**
- Task 02 — `tarRecorder.write()` adds sysinfo entry; `Test_app_record` formula change depends on it
- Task 03 — `view.go` removes `NotRecordable: true` from procpidstat; `isFilenameOK` accepts `"sysinfo"`; `metadata` struct gains `ticks`/`cpuCount`; `readTar` handles `sysinfo.*`

**Edge cases:**
- `Test_app_doReport_procpidstat` uses in-memory tar — no filesystem or DB dependency
- `Test_readMeta_with_sysinfo` must add a procpidstat stat entry so `statOK` becomes true and `readTar` actually sends on the channel; otherwise the test hangs waiting for data
- Golden tar regeneration: only needed if `Test_app_doReport` subtests fail after task 03 changes. The `-update` flag is already wired in `report_test.go` (line 22: `var update = flag.Bool("update", false, "update golden files")`). Run: `go test ./report/... -update -run Test_app_doReport`
- `countRecordable` is a helper defined at the bottom of `record_test.go` (line 165): it counts views with `NotRecordable==false`. After task 03 flips procpidstat to `NotRecordable=false`, `countRecordable(view.New())` automatically includes it — the `+2` in `Test_app_record` accounts for meta and sysinfo (the extra two non-stat entries per tick)

**Implementation hints:**
- For in-memory tar construction in tests: `var buf bytes.Buffer; tw := tar.NewWriter(&buf)` then `tw.WriteHeader(&tar.Header{Name: ..., Size: int64(len(content))})` + `tw.Write(content)` + `tw.Close()`, then `tar.NewReader(&buf)`
- The `stat.PGresult` JSON encoding used by `stat.NewPGresultFile` can be understood by looking at how `stat.NewPGresultFile` parses it — or simply marshal a `stat.PGresult` struct with `encoding/json` to get a valid payload for the test tar
- `readTar` sends a `data` item only when `metaOK && statOK` both true; to receive metadata fields in the test, the goroutine reading `dataCh` must inspect `d.meta`

## Reviewers

- **dev-code-reviewer** → `docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report-task-04-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report-task-04-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report-task-04-dev-test-reviewer-review.json`

## Post-completion

- [ ] Write brief report to [003-feat-procpidstat-record-report-decisions.md](docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report-decisions.md) (Summary: 1-3 sentences, review rounds with links to JSON, no findings tables or dumps)
- [ ] If deviated from spec — describe deviation and reason
- [ ] Update user-spec/tech-spec if anything changed
