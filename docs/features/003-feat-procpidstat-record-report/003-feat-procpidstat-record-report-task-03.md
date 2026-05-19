---
status: planned
depends_on: ["01"]
wave: 2
skills: [code-writing]
verify: "bash — go build ./cmd/pgcenter → clean; go test ./report/... -run ReadMeta|isFilename → pass"
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
---

# Task 03: Report pipeline + -N flag + view config

## Required Skills

Before starting, load:
- `/skill:code-writing` — [SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

This task completes the report-side pipeline for the `procpidstat` feature so that `pgcenter report -N` becomes a usable flag.

Three files are modified in this task, each serving a distinct role:

1. **`internal/view/view.go`** — remove `NotRecordable: true` from the `procpidstat` view definition. The field drops to its zero value (`false`), allowing `filterViews()` in the recorder to include procpidstat in the list of recordable views. The local/remote gate (that was the original reason for `NotRecordable`) now lives in `record/record.go:app.setup()` as a runtime check (Decision 5 from tech-spec).

2. **`report/report.go`** — four targeted changes:
   - Extend `metadata` struct with `ticks float64` and `cpuCount int` fields. Under Option B these are informational (rates are pre-computed by the recorder), but they must be populated for completeness and future use.
   - Extend `isFilenameOK` to accept the `"sysinfo"` prefix alongside `"meta"` and the requested report type. Without this, `sysinfo.*` tar entries are silently skipped, and `readTar` never reads them.
   - Add a `sysinfo.*` branch in `readTar`: when the tar entry name has a `"sysinfo."` prefix, decode it as a `stat.SysInfo` JSON blob and store `Ticks` / `CPUCount` into `meta`. The `metaOK` flag is left unchanged (it was already set by the `meta.*` branch — sysinfo is supplementary).
   - Add `"procpidstat"` entry to the `describeReport` map so that `pgcenter report -d -N` prints the column descriptions.
   - Add WARNING detection in `processData`: after the first valid data pair, scan cols 9–10 (`read_total,KiB`, `write_total,KiB`) and col 11 (`iodelay_total,s`) of the first result. If all values in a column group are empty strings (`Valid=true`), emit a WARNING line before printing the first data row. This is a one-shot check per report run (Decision 10).

3. **`cmd/report/report.go`** — wire the new flag into the CLI:
   - Add `showProcPidStat bool` field to the `options` struct.
   - Register `-N` / `--proc-stats` flag in `init()`.
   - Add `case opts.showProcPidStat: return "procpidstat"` in `selectReport()`.

This task runs in Wave 2, in parallel with Task 02 (recorder changes). It depends on Task 01 because it imports `stat.SysInfo` (defined there) for the `readTar` sysinfo branch.

## What to do

1. Write TDD tests first (see TDD Anchor below) — confirm they fail before writing production code.

2. In `internal/view/view.go`, remove the `NotRecordable: true` line from the `"procpidstat"` view block. The field becomes `false` (its zero value). No other changes to the view block.

3. In `report/report.go`:
   a. Add `ticks float64` and `cpuCount int` fields to the `metadata` struct.
   b. In `isFilenameOK`, extend the prefix check to also accept `"sysinfo"`: the condition `s[0] != report && s[0] != "meta"` becomes `s[0] != report && s[0] != "meta" && s[0] != "sysinfo"`.
   c. In `readTar`, add a new branch after the `meta.*` branch: when `strings.HasPrefix(hdr.Name, "sysinfo.")`, read all bytes from the reader, unmarshal as `stat.SysInfo`, and assign `si.Ticks` → `meta.ticks` and `si.CPUCount` → `meta.cpuCount`. Do not modify `metaOK` in this branch.
   d. In `describeReport`, add an entry: `"procpidstat": procPidStatDescription` (define the description constant alongside the other description constants in the file, or inline the string — follow the existing file style).
   e. In `processData`, add a one-shot WARNING check: after `prevStat.Valid` is true and before `countDiff`, on the first data pair, inspect the first result's columns at indices 9, 10, and 11. If all values at index 9 or 10 are empty strings, write `"WARNING: IO stats unavailable in recorded data\n"` to `app.writer`. If all values at index 11 are empty strings, write `"WARNING: iodelay stats unavailable in recorded data\n"`. Track a boolean flag so the check runs only once.

4. In `cmd/report/report.go`:
   a. Add `showProcPidStat bool` field to the `options` struct.
   b. In `init()`, call `CommandDefinition.Flags().BoolVarP(&opts.showProcPidStat, "proc-stats", "N", false, "show per-process system stats report")`.
   c. In `selectReport()`, add `case opts.showProcPidStat: return "procpidstat"` to the switch.

5. Run `go build ./cmd/pgcenter` — confirm clean build.

6. Run `go test ./report/... -run ReadMeta|isFilename` — confirm tests pass.

## TDD Anchor

Tests to write BEFORE implementation. Write them → run → confirm they fail → write code → confirm they pass.

- `report/report_test.go::Test_isFilenameOK_sysinfo` — call `isFilenameOK("sysinfo.20260519T100000.000.json", "procpidstat")` and assert no error is returned. Before the fix to `isFilenameOK` this call returns an error.

- `report/report_test.go::Test_readMeta_with_sysinfo` — build a synthetic in-memory tar containing a valid `meta.*` entry (standard 7-column PGresult with a version number) followed by a `sysinfo.20260519T100000.000.json` entry containing `{"ticks":100,"cpu_count":4}`. Call `readTar` with this tar (using a pipe or `bytes.Buffer`) and capture the `data` items sent on the channel. Assert that `meta.ticks == 100` and `meta.cpuCount == 4` in the received data item.

## Acceptance Criteria

- [ ] `isFilenameOK("sysinfo.20260519T100000.000.json", "procpidstat")` returns nil (no error)
- [ ] `readTar` populates `metadata.ticks` and `metadata.cpuCount` from `sysinfo.*` tar entries
- [ ] `NotRecordable` field is absent (false) in the `"procpidstat"` view definition in `view.go`
- [ ] `-N` / `--proc-stats` flag is registered and accepted by `cmd/report`; invoking `pgcenter report -N` does not error on flag parse
- [ ] `selectReport(opts{showProcPidStat: true})` returns `"procpidstat"`
- [ ] `-A` flag behavior is unchanged (backward compat)
- [ ] `pgcenter report -d -N` outputs procpidstat column descriptions (not "unknown description requested")
- [ ] WARNING line is printed before first data row when IO columns (indices 9–10) are all empty strings in the first result
- [ ] WARNING line is printed before first data row when iodelay column (index 11) is all empty strings in the first result
- [ ] WARNING check is one-shot — only fires once per report run, not for every snapshot
- [ ] `go build ./cmd/pgcenter` succeeds with no errors

## Context Files

**Feature artifacts:**
- [003-feat-procpidstat-record-report.md](003-feat-procpidstat-record-report.md) — user-spec
- [003-feat-procpidstat-record-report-tech-spec.md](003-feat-procpidstat-record-report-tech-spec.md) — tech-spec
- [003-feat-procpidstat-record-report-decisions.md](003-feat-procpidstat-record-report-decisions.md) — decisions log

**Project knowledge:**
- [architecture.md](../../.claude/skills/project-knowledge/architecture.md)
- [patterns.md](../../.claude/skills/project-knowledge/patterns.md)

**Code files (modify):**
- [internal/view/view.go](../../../internal/view/view.go) — remove `NotRecordable: true` from `"procpidstat"` view block
- [report/report.go](../../../report/report.go) — extend `metadata`, `isFilenameOK`, `readTar`, `describeReport`; add WARNING detection in `processData`
- [cmd/report/report.go](../../../cmd/report/report.go) — add `-N` flag, `showProcPidStat` field, `selectReport` case

**Code files (read for context):**
- [internal/stat/procpidstat.go](../../../internal/stat/procpidstat.go) — `SysInfo` struct (defined in Task 01), column layout constants

## Verification Steps

1. `go build ./cmd/pgcenter` — must exit 0 with no output.
2. `go test ./report/... -run ReadMeta|isFilename -v` — `Test_isFilenameOK_sysinfo` and `Test_readMeta_with_sysinfo` must pass.
3. `go test ./report/... -v` — no regressions in existing report tests.
4. `./bin/pgcenter report --help` — `-N, --proc-stats` flag must appear in the flag list.
5. `go test ./internal/view/...` — view tests must pass (NotRecordable change propagated).

## Details

### Files

**`internal/view/view.go`** — the `"procpidstat"` view block currently ends with `NotRecordable: true` (line 295 in the current file). Simply remove this line. The field defaults to `false`. All other fields in the view block (`Name`, `QueryTmpl`, `DiffIntvl`, `Ncols`, `OrderKey`, `OrderDesc`, `ColsWidth`, `Msg`, `Filters`) stay unchanged.

**`report/report.go`**:

- `metadata` struct (line 94): currently has one field `version int`. Add `ticks float64` and `cpuCount int` below it. No JSON tags needed — this struct is internal to the report pipeline.

- `isFilenameOK` (line 309): the check `s[0] != report && s[0] != "meta"` gates which tar entries are processed. Extend it to `s[0] != report && s[0] != "meta" && s[0] != "sysinfo"`. This allows sysinfo entries through without error.

- `readTar` (line 137): the current `meta.*` branch (line 165) reads the tar entry via `stat.NewPGresultFile` and calls `readMeta`. The new `sysinfo.*` branch must come either before or after the meta branch (after is fine). In the sysinfo branch, read the raw bytes with `io.ReadAll(r)` (the entry is tiny — SysInfo is just two fields). Unmarshal into `stat.SysInfo` with `json.Unmarshal`. Copy `si.Ticks` to `meta.ticks` and `si.CPUCount` to `meta.cpuCount`. Do not set `metaOK = true` here — `metaOK` is set by the `meta.*` branch and both must be true before a data item is sent to the channel. (Under Option B, if a tar has sysinfo but no meta, nothing is sent — which is correct because version is unknown.)

- `describeReport` map (line 506): add `"procpidstat": procPidStatDescription` to the map. Define `procPidStatDescription` as a package-level string constant alongside the other `pgStat*Description` constants in the file. Content: `"Per-process system stats: CPU utilization, IO activity, and IO delay per PostgreSQL backend. Local mode only."` (Decision 7 from tech-spec). Follow the multi-line format of other description constants if they include column listings — but a one-line entry is acceptable per the decision.

- `processData` WARNING check (line 201): add a `warningChecked bool` variable alongside `prevStat`. After the first valid data pair enters the select branch and the `continue` path is no longer taken (i.e., `prevStat.Valid` is true), add the one-shot check before `countDiff`. The check inspects `d.res` (the current result, not the diff). Iterate over `d.res.Values` and check whether all entries at column indices 9 and 10 are empty strings (`v.String == "" && v.Valid`). If yes, write the IO WARNING. Then check col 11 for iodelay. Set `warningChecked = true` after the first check so subsequent snapshots skip it.

**`cmd/report/report.go`**:

- `options` struct (line 15): add `showProcPidStat bool` after `showProgress string`. Keep it grouped with other `show*` fields.

- `init()` (line 58): add the flag registration line after the existing `showProgress` flag. Flag short name is `-N` (uppercase), long name is `proc-stats`.

- `selectReport()` (line 124): add the new case at the top of the switch alongside other bool flags (before `showDatabases` which is a string case). Placement: after `case opts.showWAL` and before `case opts.showDatabases` is a natural position.

### Dependencies

- Task 01 must be complete: `stat.SysInfo` struct must exist in `internal/stat/procpidstat.go` before this task can import and use it in `readTar`.
- Task 02 is parallel and independent — no ordering constraint between 02 and 03.
- `encoding/json` package must be imported in `report/report.go` for `json.Unmarshal` in the sysinfo branch. Check current imports — if json is not already there, add it.

### Edge Cases

- **Sysinfo entry present without meta entry:** `metaOK` stays false, no data item is sent. This is safe — metadata struct's zero values for `ticks` and `cpuCount` are fine under Option B (rates are pre-computed).
- **Multiple sysinfo entries in a tick group:** each sysinfo overrides the previous. In practice there is one sysinfo per tick group, but the code handles multiple gracefully.
- **Empty sysinfo JSON:** `json.Unmarshal` on `{}` leaves `SysInfo` at zero values. `meta.ticks = 0`, `meta.cpuCount = 0`. No error — proceed normally.
- **WARNING check when procpidstat result has no rows:** if `d.res.Nrows == 0`, the column scan finds nothing empty and no WARNING is emitted. This is correct — no data means nothing to warn about.
- **`-N` flag conflict with existing flags:** `-N` is not used by any existing flag in `cmd/report/report.go` (checked: `-A`, `-R`, `-T`, `-I`, `-S`, `-F`, `-W`, `-D`, `-X`, `-P`, `-f`, `-s`, `-e`, `-o`, `-g`, `-l`, `-t`, `-d` are taken). The `-N` short form is safe.

### Implementation Hints

- The `readTar` function uses a single `metaOK / statOK` pair to gate data channel sends. The sysinfo branch supplements `meta` but does not gate the send independently — no change to the gating logic.
- When adding `json` import to `report/report.go`: place it in the stdlib group (`"encoding/json"`) not in the project group. Follow the existing import block grouping (stdlib / project).
- The WARNING detection in `processData` is inspecting raw result values before `countDiff` — `d.res` is the unprocessed current snapshot. This is intentional: we want to detect empty columns in the source data, not in the computed diff.

## Reviewers

- **dev-code-reviewer** → `docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report-task-03-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report-task-03-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report-task-03-dev-test-reviewer-review.json`

## Post-completion

- [ ] Write brief report to [003-feat-procpidstat-record-report-decisions.md](003-feat-procpidstat-record-report-decisions.md) (summary: 1-3 sentences, review links with round numbers, no finding tables or code dumps)
- [ ] If deviated from spec — describe the deviation and reason
- [ ] Update user-spec/tech-spec if anything changed
