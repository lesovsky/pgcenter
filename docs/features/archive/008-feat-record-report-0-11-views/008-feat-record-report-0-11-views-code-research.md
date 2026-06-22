# Code Research — 008-feat-record-report-0-11-views

Research date: 2026-06-22. Maps the implementation surfaces for lifting `NotRecordable`
from 4 screens (5 report types: bgwriter, replslots, stat_io, stat_io_time, statements_jit)
so `pgcenter record` collects them and `pgcenter report` replays them.

All assumptions in the task brief were verified against code. Where an assumption is
imprecise, it is flagged inline as **CORRECTION**.

---

## 1. Entry Points

### `record/record.go` — recorder setup + view filter
- `filterViews(version int, pgssSchema string, views view.Views) (int, view.Views)` — **record/record.go:200-233**.
  - The `NotRecordable` drop branch: **record.go:208-212** (`if v.NotRecordable { delete; filtered++; continue }`).
  - Version gate: **record.go:214-218** (`if !v.VersionOK(version)`).
  - pgss gate: **record.go:221-225** (`if strings.HasPrefix(k, "statements_") && pgssSchema == ""`).
  - **Confirmed:** removing `NotRecordable: true` from the 4 view defs is *sufficient* for collection — the drop branch is the only thing keeping them out. Once lifted, each view falls through to the version gate (and, for `statements_jit`, the pgss gate).
  - **statements_jit + pgss gate interaction:** `statements_jit` has key prefix `statements_`, so when `pgssSchema == ""` it is dropped at record.go:221 (same as all other `statements_*`). This matches edge-case decision (4): jit without pg_stat_statements is never recorded → `report -X j` produces header-only. No special handling needed.
  - **Comment update needed:** record.go:206-207 names `bgwriter` as the example NotRecordable view. After this feature *no production view* sets `NotRecordable: true` — same situation feature 003 created for procpidstat. The drop branch stays (guarded by `TestFilterViews_dropsExplicitNotRecordable`), but the comment example becomes stale.

- `app.setup()` — **record.go:65-148**. Builds `query.NewOptions(...)` (line 84), calls `filterViews` (line 86), runs the procpidstat local/remote gate (lines 91-127), then `views.Configure(opts)` (line 129). **The 4 new views are pure-SQL and need no setup changes** — they are configured by the same `views.Configure(opts)` call (version-aware selectors already wired in view.go, see §8).

### `cmd/report/report.go` — CLI flag layer
- `options` struct — **cmd/report/report.go:15-38**. Holds one field per report family.
- `init()` flag registration — **report.go:59-82**.
- `selectReport(opts options) string` — **report.go:126-184**. Maps options → report-type string.
- `validate()` — **report.go:85-123**. Wraps `selectReport` and builds `report.Config`.

### `report/report.go` — report pipeline
- `describeReport(w, report)` — **report/report.go:605-644**. The `map[string]string` at lines 606-629 needs 5 new entries.
- `newApp` — **report.go:83-92**: `views := view.New(); v := views[config.ReportType]` — the report type string must match a `view.New()` key (so `stat_io`, `stat_io_time`, `bgwriter`, `replslots`, `statements_jit`).
- `processData` — **report.go:225-339**. Replay loop; calls `views.Configure(query.Options{Version: d.meta.version})` at report.go:250-252 (see §8).

---

## 2. Data Layer (view definitions)

All 5 view defs live in `internal/view/view.go` inside `view.New()` (the big `Views{...}` literal). `NotRecordable: true` lines to delete:

| Report type      | view.go block | `NotRecordable: true` line | Static Ncols / DiffIntvl / Keys |
|------------------|---------------|----------------------------|----------------------------------|
| `bgwriter`       | 140-152       | **view.go:151**            | Ncols 12, DiffIntvl `[3,10]`, OrderKey 0 (PG14 baseline; version-patched) |
| `replslots`      | 153-165       | **view.go:164**            | Ncols 15, DiffIntvl `[6,13]`, OrderKey 4 (desc), no UniqueKey field set |
| `stat_io`        | 166-179       | **view.go:178**            | Ncols 16, DiffIntvl `[4,14]`, OrderKey 4, UniqueKey 0 |
| `stat_io_time`   | 180-193       | **view.go:192**            | Ncols 10, DiffIntvl `[4,8]`, OrderKey 4, UniqueKey 0 |
| `statements_jit` | 230-243       | **view.go:242**            | Ncols 13, DiffIntvl `[6,10]`, OrderKey 2, UniqueKey 11 (PG15 baseline; version-patched) |

**Version gates (MinRequiredVersion):**
- bgwriter: `query.PostgresV14` (view.go:142)
- replslots: `query.PostgresV14` (view.go:155)
- stat_io: `query.PostgresV16` (view.go:168)
- stat_io_time: `query.PostgresV16` (view.go:182)
- statements_jit: `query.PostgresV15` (view.go:232)

These gates take effect in `filterViews` only *after* `NotRecordable` is removed (the NotRecordable branch currently fires first and masks them). This is why the per-version `Test_filterViews` deltas are uneven (see §5).

### tar storage format
- `tarRecorder.write(stats)` — **record/recorder.go:237-275**. The SQL-view loop is **recorder.go:240-256**: for every key in `stats` it marshals `stat.PGresult` to JSON and writes `newFilenameString(now, name)` → `{name}.{TIMESTAMP}.json` (recorder.go:246, `newFilenameString` at recorder.go:290-292: `"%s.%s.json"` with `20060102T150405.000`). So the new types are written as `bgwriter.TS.json`, `replslots.TS.json`, `stat_io.TS.json`, `stat_io_time.TS.json`, `statements_jit.TS.json` automatically — no write() change.
- Stored cells are the **raw cumulative SQL result** (`stat.NewPGresultQuery`), diffed at report time via `DiffIntvl`. This is the standard recordable-view path (same as tables/wal/statements_*).

---

## 3. Similar Features (reusable patterns)

- **`wal` + `statements_*` are the closest precedents** — pure-SQL recordable views with version-aware selectors. `wal` (view.go:128-139) is recordable (no `NotRecordable`), uses `SelectStatWALQuery(version)` in Configure (view.go:393-395), and has a describe entry (`pgStatWALDescription`) + golden test (`report_wal.golden`).
- **`replication`** is cited in the interview as the working precedent: recordable, report-time-Configured via `Options{Version}` (Configure at view.go:381-383).
- **procpidstat (feature 003)** is the lineage for *lifting* `NotRecordable`, but it is NOT a clean template here — procpidstat needed a stateful recorder + sysinfo + INFO/WARNING handling because it is a hybrid procfs view. **The 4 new views are pure-SQL and need none of that** (see §7).

---

## 4. CLI Mapping — exact changes to `cmd/report/report.go`

**Verified: short flags `B`, `L`, `J` are all FREE.** Used short flags today:
`d, A, R, T, I, S, F, W, D, X, P, N, f, s, e, o, g, l, t` (grep-confirmed, report.go init()).

### 4a. `options` struct (report.go:15-38) — add 3 fields
```go
showBgwriter  bool   // -B
showReplSlots bool   // -L
showStatIO    string // -J  values: c (count) / t (time)
```
(`statements_jit` reuses the existing `showStatements string` field — value `j`.)

### 4b. `init()` (report.go:59-82) — register 3 flags
```go
CommandDefinition.Flags().BoolVarP(&opts.showBgwriter, "bgwriter", "B", false, "show pg_stat_bgwriter / pg_stat_checkpointer report")
CommandDefinition.Flags().BoolVarP(&opts.showReplSlots, "replslots", "L", false, "show pg_replication_slots / pg_stat_replication_slots report")
CommandDefinition.Flags().StringVarP(&opts.showStatIO, "io", "J", "", "show pg_stat_io report (c - count, t - time)")
```

### 4c. `selectReport()` (report.go:126-184) — add cases
- New `case opts.showBgwriter: return "bgwriter"`.
- New `case opts.showReplSlots: return "replslots"`.
- New `case opts.showStatIO != "":` with inner switch `"c" → "stat_io"`, `"t" → "stat_io_time"`.
- Extend the existing `opts.showStatements` switch (report.go:152-165) with `case "j": return "statements_jit"`.

### 4d. `validate()` (report.go:85-123)
No structural change required — it already delegates to `selectReport` and passes through `ReportType`. The new fields need no validation beyond the switch.

---

## 5. Test surfaces that WILL break (with exact literals)

### 5a. `record/record_test.go: Test_filterViews` — **record_test.go:101-140** (MUST change)
Current testcase literals (record_test.go:127-132):
```go
{version: 140000, pgssSchema: "",       wantN: 11, wantV: 16},
{version: 140000, pgssSchema: "public", wantN: 5,  wantV: 22},
{version: 130000, pgssSchema: "public", wantN: 8,  wantV: 19},
{version: 120000, pgssSchema: "public", wantN: 11, wantV: 16},
{version: 110000, pgssSchema: "public", wantN: 13, wantV: 14},
{version: 100000, pgssSchema: "public", wantN: 13, wantV: 14},
```
**New literals after lifting NotRecordable** (computed by running the new filter logic against `view.New()`; only PG14 rows change — the lifted views are PG14/15/16-gated, so on PG13 and below the version gate drops them anyway):
```go
{version: 140000, pgssSchema: "",       wantN: 9, wantV: 18},   // was 11/16  (-2 dropped, +2 kept)
{version: 140000, pgssSchema: "public", wantN: 3, wantV: 24},   // was 5/22
{version: 130000, pgssSchema: "public", wantN: 8, wantV: 19},   // UNCHANGED
{version: 120000, pgssSchema: "public", wantN: 11, wantV: 16},  // UNCHANGED
{version: 110000, pgssSchema: "public", wantN: 13, wantV: 14},  // UNCHANGED
{version: 100000, pgssSchema: "public", wantN: 13, wantV: 14},  // UNCHANGED
```
**Why PG14 deltas are -2/+2, not -5/+5:** at PG14 only `bgwriter` and `replslots` (both PostgresV14) become recordable. `stat_io`/`stat_io_time` (V16) and `statements_jit` (V15) are still dropped — but now by the *version gate* instead of the NotRecordable branch, so they stay counted in `wantN`. Net: 2 views move from filtered→kept, the other 3 just change *why* they're filtered. For the `s=""` row, the +2 kept are bgwriter+replslots (not statements-prefixed); statements_jit stays filtered by the pgss gate regardless.
The large doc comment at record_test.go:108-126 explaining the NotRecordable contributions must be rewritten.

### 5b. `internal/view/view_test.go: TestNew` — **view_test.go:9-12** (does NOT change)
`assert.Equal(t, 27, len(v))` — **CONFIRMED unchanged.** Lifting `NotRecordable` does not add/remove views; total stays 27. ✅ (assumption correct)

The per-view assertions DO change:
- `view_test.go:21` `assert.True(t, jit.NotRecordable)` → must become `assert.False(...)`.
- `view_test.go:40` `assert.True(t, statio.NotRecordable)` → `assert.False(...)`.
- `view_test.go:58` `assert.True(t, statioTime.NotRecordable)` → `assert.False(...)`.
- `view_test.go:75` `assert.True(t, replslots.NotRecordable)` → `assert.False(...)`.
- `view_test.go:91` `assert.True(t, bgwriter.NotRecordable)` → `assert.False(...)`.
The comments at view_test.go:15, 34, 52, 70, 86 ("excluded from recording (NotRecordable)") become factually wrong and need editing.

### 5c. `record/record_test.go: Test_app_record` — **record_test.go:32-99** (auto-adjusts, but reads against live PG)
Uses `countRecordable(view.New())` (record_test.go:37, helper at 200-210) which dynamically counts `!NotRecordable` views — so `totalViews` auto-increases by however many of the 5 are recordable on the *test cluster's* PG version. **No literal to change**, but the test records against a real PG (`postgres.NewTestConfig()`), so the count is version-dependent at runtime — it self-corrects.
`countRecordable` itself stays correct (it already keys on `NotRecordable`).

### 5d. `record/record_test.go: TestFilterViews_dropsExplicitNotRecordable` — **record_test.go:171-183** (stays, still valuable)
Uses a synthetic `explicit_not_recordable` view. After this feature no production view sets `NotRecordable: true`, so this synthetic test becomes the *only* coverage of the drop branch — keep it.

### 5e. `cmd/report/report_test.go: Test_selectReport` — **report_test.go:34-66** (SHOULD add entries)
Current testcases do NOT enumerate every flag (procpidstat `-N` is absent). Add:
```go
{opts: options{showBgwriter: true},  want: "bgwriter"},
{opts: options{showReplSlots: true}, want: "replslots"},
{opts: options{showStatIO: "c"},     want: "stat_io"},
{opts: options{showStatIO: "t"},     want: "stat_io_time"},
{opts: options{showStatements: "j"}, want: "statements_jit"},
```

### 5f. `report/report_test.go: Test_describeReport` — **report_test.go:1033-1070** (MUST add 5 entries)
Testcase list at report_test.go:1038-1059. Add 5 rows pointing at the new description constants (whatever they are named — see §3 below). Note: this list is also missing `statements_wal`/`progress_copy` currently — do not "fix" those (out of scope).

### 5g. `report/report_test.go: Test_app_doReport` — **report_test.go:24-198** (golden roundtrip)
Uses fixed fixture `testdata/pgcenter.stat.golden.tar` (report_test.go:181), recorded long before these views existed. **The fixture contains NO entries for the 5 new types**, so a golden testcase for them would produce *header-only* output (validating the empty-archive path, edge-case decision 2). Two options for the integration test: (a) add header-only golden cases against the existing fixture; (b) write a fresh record→report roundtrip test against live PG (see §6). The locked testing strategy (interview Q4) calls for record→report roundtrip per screen PG14-18 → option (b).

---

## 6. Existing record→report roundtrip harness

**There is no live-PG record→report roundtrip test today.** The two existing pipelines are decoupled:
- `record/record_test.go` records against live PG (`postgres.NewTestConfig()`, record_test.go:69) but only counts tar entries — it never *reports* them.
- `report/report_test.go` reports against a static golden tar fixture — it never *records*.
- The procpidstat report pipeline (`report/report_test.go:597-716` `Test_app_doReport_procpidstat`) builds a **synthetic in-memory tar** (two ticks: meta + procpidstat + sysinfo) and runs `app.doReport` — this is the closest reusable harness for an in-process roundtrip that does NOT need a live PG. `writeEntry` helper builds `{type}.{TS}.json` entries; meta entry carries the version; `app.writer = &buf` captures output.

**Recommended harness pattern (mirrors `Test_app_doReport_procpidstat`):** build a synthetic tar with two ticks of a hand-built `stat.PGresult` for each new view type (cumulative counters in tick 1 and tick 2), include a `meta.*` entry with the target version, run `doReport`, assert the diffed output. This needs no live PG and is version-parametric via the meta entry — directly exercising the report-time `Configure(Options{Version})` layout switch (§8).

For a *true* live record→report (PG14-18), the pattern would be: `postgres.NewTestConfig()` per port (the test clusters expose per-version ports; see `internal/postgres` test config and the `t.Skipf` skip-on-unavailable idiom used across the suite, e.g. tech-debt [005] notes `top/reload_test.go` is the *exception* that panics instead of skipping). No existing test does this end-to-end yet, so the harness would be new.

---

## 7. Recorder collect/write path — NO changes needed

- `tarRecorder.collect(dbConfig, views)` — **record/recorder.go:116-153**.
  - SQL loop: **recorder.go:135-142** — `for k, v := range views { res := stat.NewPGresultQuery(db, v.Query); stats[k] = res }`. Pure SQL; collects every recordable view including the 5 new ones with zero special-casing.
  - **procpidstat enrichment branch — recorder.go:148-150:** `if pp, ok := stats["procpidstat"]; ok && c.config.isLocal && pp.Valid { c.enrichProcPidStat(...) }`. **CONFIRMED the enrichment branch is gated on the literal key `"procpidstat"`** — it cannot trigger for bgwriter/replslots/stat_io/stat_io_time/statements_jit. ✅ (assumption correct)
- `tarRecorder.write(stats)` — **recorder.go:237-275**. SQL-view loop at recorder.go:240-256 writes `{reporttype}.TIMESTAMP.json`; sysinfo appended at recorder.go:258-272 (unconditional, but harmless to the new types — report's `isFilenameOK` only matches the requested report type + `meta` + `sysinfo`).
- **Conclusion: no recorder changes.** The new views ride the existing pure-SQL collect/write path exactly like wal/tables.

---

## 8. Report-time `Configure` — version-aware selectors

- `processData` calls `views.Configure(query.Options{Version: d.meta.version})` at **report.go:250-252** (inside the `!prevStat.Valid || prevMeta.version != d.meta.version` branch — also handles mid-archive version change, edge case 3). **CONFIRMED.** ✅
- `Views.Configure(opts)` switch — **internal/view/view.go:371-409**. The 5 relevant cases:
  - `statements_jit` (view.go:390-392): `query.SelectStatStatementsJITQuery(opts.Version)` returns **4-tuple** `(QueryTmpl, Ncols, DiffIntvl, UniqueKey)` — signature at **internal/query/statements.go:347** `func SelectStatStatementsJITQuery(version int) (string, int, [2]int, int)`. Branches `>=PostgresV17 → (Default,15,{7,12},13)` else `(PG15,13,{6,10},11)` (statements.go:348-351). **Version alone is sufficient.** ✅
  - `bgwriter` (view.go:396-398): `query.SelectStatBgwriterQuery(opts.Version)` returns `(string,int,[2]int)` — **internal/query/bgwriter.go:41**. Three-way: V18→`(PG18,14,{6,12})`, V17→`(PG17,13,{6,11})`, else→`(PG14,12,{3,10})` (bgwriter.go:42-51). **Version alone sufficient.** ✅
  - `replslots` (view.go:399-401): `query.SelectStatReplicationSlotsQuery(opts.Version)` — **internal/query/replication_slots.go:39** `func SelectStatReplicationSlotsQuery(_ int)`. **Ignores version** (single query for PG14-18, ADR [005]). Returns fixed `(query,15,{6,13})`. **No version dependency at all** — works under empty Options too. ✅
  - `stat_io` (view.go:402-404): `query.SelectStatIOQuery(opts.Version)` — **internal/query/io.go:87**. Branches at PG18 (KiB derivation) but Ncols/DiffIntvl identical across branches. **Version alone sufficient.** ✅
  - `stat_io_time` (view.go:405-407): `query.SelectStatIOTimeQuery(_ int)` — **internal/query/io.go:99**. **Ignores version** (timings identical PG16-18). ✅
- **No selector needs more than `Version`.** `replslots` and `stat_io_time` ignore version entirely; the other three branch on version only. The report pipeline passes `Version` only (report.go:251) and never sets `GucTrackCommitTS`/`ExtPGSSSchema` — but none of the 5 selectors read those, and the rebuilt `view.Query` (Configure's second loop, view.go:412-419) is **never executed in report** (report replays recorded JSON; only `Ncols`/`DiffIntvl`/`UniqueKey`/`OrderKey` are consumed by `countDiff`). So a structurally-broken SQL string from empty `pgss`/`track` Options is harmless (interview constraints, confirmed).

  - **Note on `statements_jit` static UniqueKey:** view.go:238 sets `UniqueKey: 11` statically, but `Configure` overwrites it from the selector (view.go:391). In report, `Configure` always runs before the first diff (report.go:250-257 on the first sample), so the correct version-specific UniqueKey is in place. No reliance on the static value.

---

## 9. `describeReport` — description constants location & naming

- **All description constants live in `report/describe.go`** (single `const (...)` block, describe.go:3-onwards). Naming convention: `pgStat<Thing>Description` (e.g. `pgStatWALDescription` at describe.go:141, `pgStatStatementsTimingsDescription` at describe.go:327). The procpidstat one breaks the pattern slightly: `procPidStatDescription` (describe.go:423) — a one-line string, not the multi-line `column/origin/description` table format used by the others.
- The `describeReport` map (report/report.go:606-629) wires `"reporttype" → constant`.
- **5 new constants to add to `report/describe.go`**, following the multi-line table convention (like `pgStatWALDescription`). Suggested names (match existing convention):
  - `pgStatBgwriterDescription` → map key `"bgwriter"`
  - `pgStatReplicationSlotsDescription` → map key `"replslots"`
  - `pgStatIODescription` → map key `"stat_io"`
  - `pgStatIOTimeDescription` → map key `"stat_io_time"`
  - `pgStatStatementsJITDescription` → map key `"statements_jit"`
- Add the 5 map entries at report/report.go:606-629 and the 5 testcases at report_test.go:1038-1059.

---

## 10. Tech-debt payoff locations (interview: pay [007] + [004])

### [007] — behavioral `diff()` NULL-cell test for pg_stat_io
- **Where the test goes:** `internal/stat/postgres_test.go`, next to `Test_diff` (**postgres_test.go:283-336**) and `Test_diff_pg18_wal_stats_age` (postgres_test.go:341+). These already build `PGresult{}` literals and call the unexported `diff(curr, prev, itv, interval, ukey)` directly (postgres_test.go:317, 334).
- **The function under test:** `diff()` at **internal/stat/postgres.go:303-358**. The vulnerable line is **postgres.go:336**: `diffPair(curr.Values[i][l].String, prev.Values[j][l].String, itv)` — an empty in-interval cell reaches `diffPair → ParseInt("")` (postgres.go:478) → error → `return diff, err` (postgres.go:337-339) → aborts the whole sample. `Test_diff` already proves the error path (postgres_test.go:327-335 with `"invalid"`).
- **The debt:** the *production* protection is `coalesce(...,0)` in `internal/query/io.go` (stored cells are `"0"`, never `""`), asserted only **structurally** (SQL contains `coalesce`). The behavioral half — `diff()` correctly subtracts `"0"`-coalesced cells and does NOT blank the screen — is unverified. The payoff test: feed a `PGresult` whose diffed cells are `"0"` (the coalesced value) through `diff()` and assert a clean `"0"` delta (and, per the debt note, that an `io_key`-style UniqueKey matches rows). This is now *directly relevant* because report replays the recorded coalesced cells through `countDiff → Compare → diff` (report.go:452 → postgres.go:273 → :303).
- **Compare wrapper:** `stat.Compare(curr, prev, itv, interval, skey, desc, ukey)` — **postgres.go:273**, called by report's `countDiff` (report.go:449-458).

### [004] — duplicated procpidstat col-index constants
- **The duplicates:** `report/report.go:342-346`:
  ```go
  const (
      procPidStatColReadTotalKiB  = 9
      procPidStatColWriteTotalKiB = 10
      procPidStatColIODelayTotalS = 11
  )
  ```
  Used only by `emitProcPidStatAvailabilityWarnings` (report.go:355-387, refs at :376, :381, :359).
- **Authoritative source:** `internal/stat/procpidstat.go`. The canonical column order is `procPidResultCols` (**procpidstat.go:18-27**) — an *unexported* `[]string`; the indices 9/10/11 correspond to `"read_total,KiB"`, `"write_total,KiB"`, `"iodelay_total,s"` (procpidstat.go:21-22). There are currently **no exported index constants** there.
- **Payoff:** export named index constants from `internal/stat/procpidstat.go` (e.g. `ColReadTotalKiB = 9`, `ColWriteTotalKiB = 10`, `ColIODelayTotalS = 11`), delete the local block at report.go:342-346, and reference `stat.Col*` in `emitProcPidStatAvailabilityWarnings`. `report` already imports `internal/stat` (report.go:9). No import cycle (report → stat is one-way; stat does not import report).
- **Scope note (interview):** [004] is in scope because the feature touches `report/report.go` anyway. [006] (replslots standby) and [005] (`top/reload_test.go` panic) are explicitly OUT of scope.

---

## 11. Potential Problems

1. **`Test_app_doReport` golden fixture has no new-type data.** `testdata/pgcenter.stat.golden.tar` predates these views; any golden case for the 5 types yields header-only output. Don't expect non-trivial golden roundtrip from the existing fixture — use the synthetic-tar harness (§6) for real diff coverage. Low severity, but a likely trap.

2. **`Test_filterViews` comment block (record_test.go:108-126) is dense and now wrong.** It enumerates each NotRecordable view's `+1` contribution. After the change the narrative inverts; a careless edit that only changes the literals but leaves the comment will mislead future readers. Medium-low.

3. **Tech-debt [007] (Active, Low).** Directly in this feature's path: report replays coalesced `pg_stat_io` cells through `diff()`. Interview locked it as in-scope. The behavioral test must live in `internal/stat/postgres_test.go` (import-cycle prevents it living in `internal/query`). See §10.

4. **Tech-debt [004] (Active, Low).** In-scope; clean export from `internal/stat`. See §10.

5. **Tech-debt [006] (Active, Low) — replslots standby `retained,KiB`.** Touches a view this feature records. NOT in scope (needs live standby) but worth noting: the recorded value is whatever the recording host's `{{.WalFunction2}}()` produced; report just diffs it. No new risk introduced by recording.

6. **ADR [004-feat-bgwriter-checkpointer] "NotRecordable: true for TUI-only scope" (Accepted) is being reversed by this feature.** This is the intended action (the feature exists to lift it), not a conflict to flag for re-litigation — but the ADR log should be updated at /done time. Same applies to the `NotRecordable` mentions in ADR [006]/[007] context. The `NotRecordable` *field and mechanism* stay (still used by the synthetic drop-branch test); only the production `: true` settings are removed.

7. **No production view will set `NotRecordable: true` after this feature** (same state feature 003 left for procpidstat, before 004 re-introduced it). The drop branch (record.go:208-212) and its guard test (`TestFilterViews_dropsExplicitNotRecordable`, record_test.go:171-183) must remain or the mechanism becomes dead/untested. The stale comment at record.go:206-207 (naming bgwriter) should be made generic.

8. **`statements_jit` OrderKey vs report ordering.** Static `OrderKey: 2` (view.go:236). On report, `processData` overrides OrderKey only if `config.OrderColName != ""` (report.go:288-294); otherwise the view's static OrderKey/OrderDesc is used by `Compare` (report.go:452). This is consistent with TUI behavior — no action needed, just be aware the default report sort differs per screen (replslots OrderKey 4, stat_io/time OrderKey 4, jit OrderKey 2).

---

## 12. Constraints & Infrastructure

- **Go 1.25.11** toolchain (ADR [004], CI bumped for GO-2026-5037). `make test` runs race + coverage; `make lint` (golangci-lint + gosec); `make vuln` (govulncheck). All gates must stay green on the PG14-18 matrix.
- **No new dependencies** — pure-SQL views reuse existing pgx/stat/view machinery.
- **`query.Format` tolerates empty Options** (plain text/template, no `missingkey=error`) — confirmed relevant because report passes only `Version` (interview constraint). The rebuilt SQL is never executed in report anyway (§8).
- **Test clusters:** the suite runs against per-version PG fixtures and uses `t.Skipf` to skip unavailable versions (except the [005] `top/reload_test.go` panic exception). Any new live roundtrip test must follow the `t.Skipf`-on-unavailable idiom.
- **Docs to update inside this feature** (interview decision 3): `.claude/skills/project-knowledge/overview.md`, `.claude/skills/project-knowledge/architecture.md`, and the features-catalog — they currently state these 4 screens are TUI-only/NotRecordable and become factually wrong once the flag is lifted.

---

## Summary of files to change

| File | Change |
|------|--------|
| `internal/view/view.go` | Delete `NotRecordable: true` at lines 151, 164, 178, 192, 242 |
| `internal/view/view_test.go` | Flip `assert.True→False` at 21, 40, 58, 75, 91; fix comments 15/34/52/70/86 |
| `cmd/report/report.go` | options struct (add 3 fields); init() (3 flags B/L/J); selectReport (bgwriter/replslots/stat_io[c\|t]/statements_jit[j]) |
| `cmd/report/report_test.go` | Add 5 `Test_selectReport` cases |
| `report/describe.go` | Add 5 description constants |
| `report/report.go` | Add 5 `describeReport` map entries (606-629); [004] delete const block 342-346, use `stat.Col*` |
| `report/report_test.go` | Add 5 `Test_describeReport` cases; (optional) synthetic-tar roundtrip tests for 5 types |
| `record/record_test.go` | Update `Test_filterViews` literals (127-132) + comment (108-126) |
| `internal/stat/procpidstat.go` | [004] export `Col*` index constants |
| `internal/stat/postgres_test.go` | [007] behavioral `diff()` NULL/coalesced-cell test near `Test_diff` (283) |
| project-knowledge docs | overview.md, architecture.md, features-catalog |

**No changes to:** `record/record.go` (filterViews mechanism already correct — only comment), `record/recorder.go` (pure-SQL path unchanged), `internal/view/view.go` Configure switch (selectors already wired), `internal/query/*.go` selectors (already version-aware), `report/report.go` processData pipeline (already Configures + diffs).
