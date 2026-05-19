---
status: planned
depends_on: ["01"]
wave: 2
skills: [code-writing]
verify: "bash — go test ./record/... -run TarRecorder|FilterViews|app_record → pass; go build ./cmd/pgcenter → clean"
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
teammate_name:
---

# Task 02: tarRecorder — stateful procfs enrichment + sysinfo write + local/remote gate

## Required Skills

Before starting, load:
- `/skill:code-writing` — [SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

This task makes `tarRecorder` stateful so it can collect and store per-process system stats (`procpidstat`) during `pgcenter record` sessions.

Currently `tarRecorder` is entirely stateless — it has no memory between ticks, and `tarConfig` carries only `filename` and `append`. The `procpidstat` view was permanently excluded from recording via `NotRecordable: true`. This task implements the recorder side of lifting that restriction.

The work splits into three interleaved concerns:

1. **Extend `tarConfig` and `tarRecorder`** — add fields for locality, CLK_TCK, CPU count, IO/delayacct availability flags, and the four prev/curr maps needed for per-interval rate calculation. The maps mirror the structure already used by `Collector` in `internal/stat/stat.go`.

2. **Extend `app.setup()` in `record/record.go`** — capture `db.Local` before closing the setup connection; if remote, delete `procpidstat` from views and print an INFO message; if local, call `stat.GetSysticksLocal()` and `runtime.NumCPU()`, probe IO/delayacct availability, and pass all values into `tarConfig`.

3. **Implement the procpidstat branch in `collect()` and the sysinfo entry in `write()`** — `collect()` runs the map-rotation protocol (identical to `Collector.Update`), reads procfs per PID, calls `buildProcPidResult` to produce a 19-column display `PGresult`, and stores it under `stats["procpidstat"]`. `write()` appends a `sysinfo.TIMESTAMP.json` tar entry containing `stat.SysInfo{Ticks, CPUCount}`.

The task depends on Task 01 which exports `GetSysticksLocal`, adds `stat.SysInfo`, and ensures `buildProcPidResult` is callable from the `record` package with the correct signature.

## What to do

1. **Extend `tarConfig` struct** in `record/recorder.go` with fields: `isLocal bool`, `ticks float64`, `cpuCount int`, `ioAvailable bool`, `delayAcctAvailable bool`.

2. **Extend `tarRecorder` struct** in `record/recorder.go` with five new fields: `prevProcPidStats map[int]stat.ProcPidStat`, `currProcPidStats map[int]stat.ProcPidStat`, `prevProcPidIO map[int]stat.ProcPidIO`, `currProcPidIO map[int]stat.ProcPidIO`, `lastCollect time.Time`. These fields are zero-value safe — nil maps, zero time — so existing tests using `newTarRecorder(tarConfig{...})` continue to compile and pass without changes.

3. **Extend `app.setup()` in `record/record.go`** with a locality probe sequence after `postgres.Connect`:
   - Capture `db.Local` into a local variable before `db.Close()`.
   - If `!db.Local`: delete `views["procpidstat"]` from the views map and print `INFO: procpidstat skipped (remote mode: /proc not available)`. Skip steps below.
   - If `db.Local`: call `stat.GetSysticksLocal()` to obtain `ticks`; call `runtime.NumCPU()` to obtain `cpuCount`.
   - Query the first non-self backend PID from `pg_stat_activity` using a `SELECT pid FROM pg_stat_activity WHERE pid > 0 AND pid != pg_backend_pid() LIMIT 1` query on the still-open `db`. If no row is returned, set `ioAvailable = false`; otherwise call `stat.CheckIOAvailable(firstPID)` — `ioAvailable = (err == nil)`.
   - Call `stat.CheckDelayAcctAvailable()` to get `delayAcctAvailable`.
   - Pass all five values into `tarConfig{isLocal, ticks, cpuCount, ioAvailable, delayAcctAvailable, ...}`.

4. **Implement the procpidstat branch in `collect()`** in `record/recorder.go`. Inside the views loop, after the standard SQL query for each view, add a dedicated branch executed when the view key is `"procpidstat"` and `c.config.isLocal` is true:
   - Run the map-rotation protocol: build `newPrev` by keeping only PIDs from the SQL result that exist in `currProcPidStats`; assign `prevProcPidStats = newPrev`; reset `currProcPidStats` to a fresh empty map. Repeat for the IO maps.
   - For each PID in the SQL result (skip if `pid <= 0`): call `stat.ReadProcPidStat(pid)` and store in `currProcPidStats`; if `c.config.ioAvailable`, call `stat.ReadProcPidIO(pid)` and store in `currProcPidIO`. Silently skip per-PID errors.
   - Compute `itv = time.Since(c.lastCollect).Seconds()` and update `c.lastCollect = time.Now()`.
   - Call `stat.BuildProcPidResult` with the SQL result, prev/curr maps, `c.config.ioAvailable`, `c.config.delayAcctAvailable`, `c.config.ticks`, `itv`, `c.config.cpuCount`.
   - Store the 19-column result as `stats["procpidstat"]`, replacing the 7-column SQL result.

5. **Implement the sysinfo entry in `write()`** in `record/recorder.go`. After writing all stats entries, marshal `stat.SysInfo{Ticks: c.config.ticks, CPUCount: c.config.cpuCount}` to JSON and write it as a tar entry named `sysinfo.TIMESTAMP.json` with the same `now` timestamp used for the other entries in that write call.

6. **Write the TDD anchor test first** (`TestTarRecorder_WriteSysinfo`) before implementing `write()` changes — see TDD Anchor section below.

## TDD Anchor

Write these tests BEFORE implementing the corresponding production code. Run them, confirm they fail, implement the code, confirm they pass.

- `record/recorder_test.go::TestTarRecorder_WriteSysinfo` — create a `tarRecorder` with `tarConfig{ticks: 100, cpuCount: 4}`, call `write()` with an empty stats map, read the resulting tar, assert that exactly one entry exists with name matching `sysinfo.*.json`, unmarshal the entry body into `stat.SysInfo`, and verify `Ticks == 100` and `CPUCount == 4`.

This is the primary new test for this task. Additional tests covering `app.setup()` locality branching (`Test_app_record` count formula) and `filterViews` behavior are owned by Task 04 (test suite update wave) — do not duplicate them here.

## Acceptance Criteria

- [ ] `tarConfig` struct contains fields: `isLocal bool`, `ticks float64`, `cpuCount int`, `ioAvailable bool`, `delayAcctAvailable bool`
- [ ] `tarRecorder` struct contains fields: `prevProcPidStats`, `currProcPidStats`, `prevProcPidIO`, `currProcPidIO` (all `map[int]stat.*`), `lastCollect time.Time`
- [ ] `app.setup()` with a local `dbConfig` (unix socket or loopback): probes IO/delayacct, passes all flags into `tarConfig`
- [ ] `app.setup()` with a remote `dbConfig`: `views["procpidstat"]` is absent after setup; `INFO: procpidstat skipped (remote mode: /proc not available)` is printed to stdout
- [ ] `tarRecorder.collect()` when `isLocal=true` and `"procpidstat"` is in views: produces `stats["procpidstat"]` with `Ncols=19` and `Valid=true`
- [ ] `tarRecorder.collect()` map rotation: PIDs present in the previous tick's SQL result are promoted from `currProcPidStats` to `prevProcPidStats`; PIDs that disappeared are removed
- [ ] `tarRecorder.write()` produces a `sysinfo.TIMESTAMP.json` entry in the tar archive on every write call
- [ ] `sysinfo.TIMESTAMP.json` entry body is valid JSON that deserializes into `stat.SysInfo` with the correct `ticks` and `cpu_count` values
- [ ] `TestTarRecorder_WriteSysinfo` passes
- [ ] `go build ./cmd/pgcenter` succeeds with no errors
- [ ] `go test ./record/... -run TarRecorder|FilterViews|app_record` passes

## Context Files

**Feature artifacts:**
- [003-feat-procpidstat-record-report.md](003-feat-procpidstat-record-report.md) — user-spec
- [003-feat-procpidstat-record-report-tech-spec.md](003-feat-procpidstat-record-report-tech-spec.md) — tech-spec
- [003-feat-procpidstat-record-report-decisions.md](003-feat-procpidstat-record-report-decisions.md) — decisions log

**Project knowledge:**
- [architecture.md](../../.claude/skills/project-knowledge/architecture.md)
- [patterns.md](../../.claude/skills/project-knowledge/patterns.md)

**Code files to modify:**
- [record/recorder.go](../../../../record/recorder.go) — extend `tarConfig` and `tarRecorder` structs; implement procpidstat branch in `collect()`; add sysinfo entry in `write()`
- [record/record.go](../../../../record/record.go) — extend `app.setup()` with locality probe, IO/delayacct probes, and remote-mode guard

**Code files to read for context:**
- [internal/stat/procpidstat.go](../../../../internal/stat/procpidstat.go) — `stat.BuildProcPidResult`, `stat.ReadProcPidStat`, `stat.ReadProcPidIO`, `CheckIOAvailable`, `CheckDelayAcctAvailable`, `SysInfo` (exported by Task 01)
- [internal/stat/stat.go](../../../../internal/stat/stat.go) — `Collector.Update` procpidstat branch (map rotation and enrichment logic to mirror), `GetSysticksLocal` (exported by Task 01)
- [internal/postgres/postgres.go](../../../../internal/postgres/postgres.go) — `DB.Local` field and `isLocalhost` implementation

## Verification Steps

1. Run `go test ./record/... -run TarRecorder|FilterViews|app_record` — all tests pass, including `TestTarRecorder_WriteSysinfo`.
2. Run `go build ./cmd/pgcenter` — exits 0 with no errors or warnings.
3. Run `go vet ./record/...` — no issues reported.
4. Confirm that `go test ./internal/stat/...` still passes (no regression from importing `stat.SysInfo`).

## Details

### Files

**`record/recorder.go`** — currently 165 lines. Contains:
- `tarConfig struct` with only `filename string` and `append bool`.
- `tarRecorder struct` with only `config tarConfig`, `file *os.File`, `fileFlags int`, `writer *tar.Writer`.
- `collect()` — opens DB, runs SQL queries for all views in a loop, returns `map[string]stat.PGresult`. Has no locality logic. The views loop calls `stat.NewPGresultQuery(db, v.Query)` for each key.
- `write()` — iterates stats map, marshals each entry to JSON, writes tar header + body. Currently writes one tar entry per stats key.
- `newFilenameString(ts, name)` — formats entries as `name.20060102T150405.000.json`.

Changes needed:
- Extend both structs as described in What to do.
- Inside `collect()`, after the views loop, add the procpidstat enrichment branch (guarded by `c.config.isLocal && stats["procpidstat"].Valid`). The branch calls `stat.ReadProcPidStat`, `stat.ReadProcPidIO`, and `stat.BuildProcPidResult` to replace the 7-column SQL entry with the 19-column enriched result.
- Inside `write()`, after the stats loop, add a single extra tar entry for sysinfo using the same `now` timestamp.
- Add imports: `runtime`, `time` (already imported), `strconv` (check if needed). The `stat` package is already imported.

**`record/record.go`** — currently 182 lines. Contains:
- `app.setup()` — connects to DB, reads Postgres properties, calls `filterViews`, configures views, creates `newTarRecorder(tarConfig{filename, append})`. The DB connection is used only for `stat.GetPostgresProperties` and `views.Configure`, then closed via `defer db.Close()`.
- `filterViews()` — currently removes `NotRecordable` views, version-incompatible views, and views requiring pg_stat_statements when not installed.

Changes needed in `setup()`:
- Capture `db.Local` after `postgres.Connect` but before any deferred close (use a local `isLocal` variable, NOT defer — the defer runs after the function returns).
- After `views.Configure(opts)` but before creating the recorder: if `!isLocal`, run the remote-mode guard.
- If `isLocal`: obtain ticks, cpuCount, firstPID, ioAvailable, delayAcctAvailable.
- For the first-PID query, use `db.QueryRow(...)` directly — the DB object is still open at this point (the defer hasn't fired). Use `sql.ErrNoRows` detection to handle the case of no active backends.
- Pass all five values into `tarConfig`.

The `filterViews` function is NOT changed in this task — `NotRecordable` removal is in Task 03. The `procpidstat` view with `NotRecordable: true` is therefore still removed by `filterViews` at this point. The local/remote gate in `setup()` is an additional deletion (for remote mode). The `NotRecordable` path in `filterViews` means that in the current state, `procpidstat` never reaches `collect()` even if local. Task 03 removes `NotRecordable: true` from the view definition, which is the actual enablement. This task only prepares the recorder struct and `setup()` so that when Task 03 removes `NotRecordable`, everything works correctly end-to-end.

### Dependencies

- Depends on **Task 01** for: `stat.GetSysticksLocal()` (exported), `stat.SysInfo` struct (defined in `internal/stat/procpidstat.go`), `stat.BuildProcPidResult`, `stat.ReadProcPidStat`, `stat.ReadProcPidIO` (all exported by Task 01).
- New imports in `record/recorder.go`: `runtime` (stdlib).
- New imports in `record/record.go`: `runtime` (stdlib), `database/sql` (for `sql.ErrNoRows`).

### Edge Cases

- **No active backends at setup time:** `firstPID` query returns no rows → `ioAvailable = false`. Recorder starts normally; IO columns in procpidstat will be `""`. Do not fail `setup()`.
- **`GetSysticksLocal()` fails:** propagate error from `setup()` — recorder does not start. Same behavior as `NewCollector` in `internal/stat/stat.go`.
- **First tick of collect():** `lastCollect` is zero value (`time.Time{}`). `time.Since(zero)` returns a large positive duration (time since epoch start) — this is not meaningful. Guard: if `prevProcPidStats` is nil or empty (first tick), pass `itv = 0` explicitly to `buildProcPidResult`, which causes it to output `"0"` for all rate columns. This matches the TUI first-tick behavior. Alternatively initialize `lastCollect` in `newTarRecorder` to `time.Now()`, which gives a reasonable interval approximation even on the first tick — check tech-spec Decision commentary. The tech-spec data flow says "first tick: itv=0 → rate cols = '0'" — so pass `itv=0` when `lastCollect.IsZero()`.
- **PID validation in collect():** Check `pid > 0` before constructing `/proc/[pid]/stat` paths. Skip `pid <= 0` silently (Decision 9).
- **Per-PID procfs errors (process exited):** silently skip individual PID errors — the same pattern used in `Collector.Update`.
- **Remote mode and isLocal gate:** the deletion of `views["procpidstat"]` in `setup()` only affects the remote case. For local mode, `procpidstat` is still removed by `filterViews` via `NotRecordable: true` until Task 03 removes that flag. This is intentional — both tasks are in wave 2, Task 03 works independently on the view config.

### Implementation Hints

- The map-rotation logic in `Collector.Update` (lines 218–238 of `internal/stat/stat.go`) is the exact pattern to copy into `collect()`. Read it carefully — the rotation builds `newPrev` from `currStats` (not `prevStats`), then assigns and resets.
- `stat.CheckIOAvailable(pid)` returns `error`; nil means available. `stat.CheckDelayAcctAvailable()` returns `bool` directly.
- In `write()`, hoist `now := time.Now()` to the very top of the function, before the per-entry stats loop. Use this same `now` for all tar entry names in that write call, including the sysinfo entry — this ensures all entries from the same write call share an identical timestamp string.
- `newFilenameString(now, "sysinfo")` produces `sysinfo.20060102T150405.000.json` — the correct format expected by the report pipeline's `isFilenameOK` (after Task 03 adds `"sysinfo"` to accepted prefixes).
- The `collect()` method uses its own separate `time.Now()` call solely for computing `itv = time.Since(c.lastCollect).Seconds()` and updating `c.lastCollect`. This is independent of the `now` in `write()` — do not mix these two timestamps.

## Reviewers

- **dev-code-reviewer** → `docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report-task-02-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report-task-02-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/003-feat-procpidstat-record-report/003-feat-procpidstat-record-report-task-02-dev-test-reviewer-review.json`

## Post-completion

- [ ] Write a brief report to [003-feat-procpidstat-record-report-decisions.md](003-feat-procpidstat-record-report-decisions.md) (summary: 1-3 sentences, review rounds with links to JSON files, no findings tables or code dumps)
- [ ] If deviated from spec — describe the deviation and reason
- [ ] Update user-spec/tech-spec if anything changed
