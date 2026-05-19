# Code Research: 003-feat-procpidstat-record-report

**Date:** 2026-05-19
**Branch:** develop

---

## 1. Entry Points

### record/record.go — `filterViews()`
`func filterViews(version int, pgssSchema string, views view.Views) (int, view.Views)`

Currently skips any view with `v.NotRecordable == true`. To make procpidstat recordable this guard must be removed or conditioned. The procpidstat view is the only one with `NotRecordable: true`.

### record/recorder.go — `tarRecorder.collect()`
`func (c *tarRecorder) collect(dbConfig postgres.Config, views view.Views) (map[string]stat.PGresult, error)`

Opens a fresh DB connection per tick, runs SQL queries via `stat.NewPGresultQuery(db, v.Query)`, also writes a `"meta"` key with `SelectCommonProperties`. **No procfs enrichment happens here.** For procpidstat recording, this method needs: (a) to call `readProcPidStat` / `readProcPidIO` per PID, (b) to maintain prev/curr maps across calls (statefulness), and (c) to call `buildProcPidResult` before storing.

### record/recorder.go — `tarRecorder.write()`
`func (c *tarRecorder) write(stats map[string]stat.PGresult) error`

JSON-marshals each `PGresult` and packs it into a tar entry. A new `"sysinfo"` key must be written here alongside the regular entries, so the report pipeline can recover `ticks` and `cpuCount`.

### report/report.go — `readTar()`
`func readTar(r *tar.Reader, config Config, dataCh chan data, doneCh chan struct{}) error`

Reads tar entries; dispatches `meta.*` files into `readMeta()`, all other matching files into `stat.NewPGresultFile()`. A `sysinfo.*` entry has the same name prefix pattern (4-part dot-separated). Currently `isFilenameOK` would reject it for non-matching prefix. Must be extended to handle `sysinfo.*` alongside `meta.*`.

### report/report.go — `processData()`
`func processData(app *app, v view.View, config Config, dataCh chan data, doneCh chan struct{}) error`

Receives `data{ts, res, meta}` items and calls `countDiff()`. For procpidstat, `countDiff` with `DiffIntvl=[0,0]` is a pass-through — no column diff. Rates were already computed in the recorder (where prev/curr procfs state was maintained). So `processData` needs minimal change; however it does need the sysinfo (ticks + cpuCount) to recompute rates from raw values if the MVC split produces raw output from the recorder.

### cmd/report/report.go — `selectReport()` and `init()`
Flag definitions are in `init()`, parsed in `validate()`, dispatched in `selectReport()`. Needs a new `-N` bool flag (`showProcPidStat bool`) and a `case opts.showProcPidStat: return "procpidstat"` branch.

### internal/view/view.go — procpidstat view definition
```go
"procpidstat": {
    Name:          "procpidstat",
    QueryTmpl:     query.PgStatActivityProcPidStat,
    DiffIntvl:     [2]int{0, 0},
    Ncols:         19,
    OrderKey:      0,
    OrderDesc:     false,
    ColsWidth:     map[int]int{},
    Msg:           "Show per-process system stats",
    Filters:       map[int]*regexp.Regexp{},
    NotRecordable: true,   // ← must become false
},
```

---

## 2. Data Layer

### Column layout of buildProcPidResult (19 columns)

| Col | Name | Kind | Source |
|-----|------|------|--------|
| 0 | `pid` | identity | SQL col 0 |
| 1 | `datname` | label | SQL col 1 |
| 2 | `usename` | label | SQL col 2 |
| 3 | `state` | label | SQL col 3 |
| 4 | `wait_etype` | label | SQL col 4 |
| 5 | `wait_event` | label | SQL col 5 |
| 6 | `all_total,s` | accumulated, HH:MM:SS | `formatCPUTime(Utime+Stime, ticks)` |
| 7 | `us_total,s` | accumulated, HH:MM:SS | `formatCPUTime(Utime, ticks)` |
| 8 | `sy_total,s` | accumulated, HH:MM:SS | `formatCPUTime(Stime, ticks)` |
| 9 | `read_total,KiB` | accumulated, integer KiB | `ReadBytes/1024` |
| 10 | `write_total,KiB` | accumulated, integer KiB | `WriteBytes/1024` |
| 11 | `iodelay_total,s` | accumulated, HH:MM:SS | `formatCPUTime(IODelay, ticks)` |
| 12 | `%all` | rate, float | `(ΔUtime+ΔStime)/(itv×ticks)×100/cpuCount` |
| 13 | `%us` | rate, float | `ΔUtime/(itv×ticks)×100/cpuCount` |
| 14 | `%sy` | rate, float | `ΔStime/(itv×ticks)×100/cpuCount` |
| 15 | `read,KiB/s` | rate, float | `ΔReadBytes/itv/1024` |
| 16 | `write,KiB/s` | rate, float | `ΔWriteBytes/itv/1024` |
| 17 | `%iodelay` | rate, float | `ΔIODelay/(itv×ticks)×100` (no cpuCount) |
| 18 | `query` | label | SQL col 6 |

**Accumulated columns (6–11)**: converted from raw jiffies/bytes to display units inside `buildProcPidResult`. They are NOT suitable for diffing across snapshots because `formatCPUTime` produces HH:MM:SS strings and `diffPair` cannot parse them.

**Rate columns (12–17)**: already computed inside `buildProcPidResult` using prev/curr maps. If the recorder stores the already-computed display PGresult, the report's `countDiff` with `DiffIntvl=[0,0]` will pass them through unchanged — this is correct since rates are already expressed per-second.

**Problem for MVC split**: The current `buildProcPidResult` outputs display values (HH:MM:SS strings, float rates as strings). The recorder needs either:
- Option A: store raw jiffies/bytes in the tar and recompute in the report (true MVC split), or
- Option B: store the already-computed 19-col display result (same as TUI) and use DiffIntvl=[0,0] pass-through.

Option B is simpler and follows the existing pattern for `activity` (which also uses DiffIntvl=[0,0] and stores display strings).

### Raw structs

```go
type ProcPidStat struct {
    Utime   float64  // jiffies
    Stime   float64  // jiffies
    IODelay float64  // delayacct_blkio_ticks
}

type ProcPidIO struct {
    ReadBytes  float64  // bytes
    WriteBytes float64  // bytes
}
```

### SQL query (7 columns into procpidstat recorder)
`internal/query/procpidstat.go:PgStatActivityProcPidStat` — selects pid, datname, usename, state, wait_event_type, wait_event, query from `pg_stat_activity WHERE pid != pg_backend_pid()`.

### sysinfo — does not exist yet
No `sysinfo` entry is present in any existing tar file. Must be defined as a new JSON structure written per tick. Minimum required fields: `ticks float64`, `cpuCount int`. These are needed by the report pipeline to reconstruct rate calculations (Option A) or simply to document the recording environment (Option B).

---

## 3. Similar Features — DiffIntvl=[0,0] Pattern

**`activity` view** (`internal/view/view.go`):
```go
"activity": { DiffIntvl: [2]int{0, 0}, ... }
```
In `calculateDelta()`, when `interval == [2]int{0,0}`, the function sets `delta = curr` (identity, no per-column subtraction) and then sorts. The procpidstat view uses the same pattern. The report pipeline's `countDiff` calls the same `stat.Compare` → `calculateDelta`.

**`progress_copy`, `progress_index`, `progress_analyze`** — also use `DiffIntvl=[0,0]`. Their report data is passed through as-is.

Pattern is established: views with `DiffIntvl=[0,0]` never diff columns; the recorder stores display-ready snapshots; the report prints them without rate recalculation.

---

## 4. Integration Points

### Collector state management (internal/stat/stat.go)
The `Collector` struct holds `prevProcPidStats map[int]ProcPidStat` and `currProcPidStats map[int]ProcPidStat` (same for IO). These are rotated per-tick in `Collector.Update()` when `view.CollectExtra == CollectProcPidStat`. The recorder does **not** use `Collector` — it opens a fresh DB connection per tick and calls `stat.NewPGresultQuery` directly, with no per-process state.

**Impact**: The recorder's `tarRecorder` must become stateful. It needs `prevProcPidStats / currProcPidStats / prevProcPidIO / currProcPidIO` maps and a `ticks float64` and `cpuCount int` field that persist between `collect()` calls. The `tarRecorder` currently has no cross-tick state.

### view.Configure() — procpidstat case
`view.Configure()` in `internal/view/view.go` has no `case "procpidstat":` — the query template is fixed (no version branching). `query.Format(view.QueryTmpl, opts)` will succeed because `PgStatActivityProcPidStat` uses `{{.QueryAgeThresh}}` and `{{if .ShowNoIdle}}` which are supported by `query.Options`. The recorder calls `views.Configure(opts)` in `app.setup()`.

### report/report.go — `isFilenameOK()` gating
```go
func isFilenameOK(name string, report string) error {
    s := strings.Split(name, ".")
    if len(s) != 4 { return error }
    if s[0] != report && s[0] != "meta" { return error }
}
```
A `sysinfo.TIMESTAMP.json` entry has 4 dot-parts and `s[0] == "sysinfo"`. It would be skipped by `isFilenameOK` unless "sysinfo" is added as an accepted prefix alongside "meta". The `readTar` loop must handle `sysinfo.*` similarly to `meta.*`.

### report/report.go — `readMeta()` usage
`readMeta` extracts only `version` (col index 1) from the meta PGresult. The `metadata` struct currently has only one field: `version int`. For procpidstat reporting, sysinfo data (`ticks`, `cpuCount`) must be carried in either an extended `metadata` struct or a separate `sysinfo` struct threaded through the `data` channel.

### stat.getSysticksLocal()
`func getSysticksLocal() (float64, error)` — calls `getconf CLK_TCK`. Already available in `internal/stat/stat.go`. The recorder must call this once at startup (same as `NewCollector`) and store the value in `tarRecorder`.

---

## 5. Existing Tests

### Framework and runner
- **Testing**: `testify/assert` (no `require`), standard `testing.T`, table-driven tests.
- **Golden files**: `report/testdata/pgcenter.stat.golden.tar` (220 entries, 10 timestamps × 22 view types + meta); golden `.golden` text files for each report type.
- **Update flag**: `var update = flag.Bool("update", false, "update golden files")` in `report/report_test.go`.

### Relevant existing tests

**procpidstat unit tests** (`internal/stat/procpidstat_test.go`):
- `TestBuildProcPidResult_FirstTick` — 19 cols, first tick (no prev maps)
- `TestBuildProcPidResult_TwoTicks` — rate column math verification
- `TestBuildProcPidResult_IOUnavailable`, `_ItvZero`, `_NcolsGuarantee`, `_InvalidPID`
- `TestBuildProcPidResult_DelayAvailable`, `_DelayUnavailable` — iodelay column coverage
- `TestReadProcPidStatIODelay`, `_Truncated` — parser golden file tests

**record tests** (`record/record_test.go`):
- `TestFilterViews_NotRecordable` — asserts procpidstat is filtered out; must be updated when `NotRecordable` is removed.
- `Test_filterViews` — table-driven counts; wantN/wantV values reference the NotRecordable filter; will shift by 1 when procpidstat becomes recordable.
- `Test_app_record` — uses `countRecordable(view.New())` to compute expected file count; will auto-adjust.

**report tests** (`report/report_test.go`):
- `Test_app_doReport` — reads from `pgcenter.stat.golden.tar`; no procpidstat entries yet.
- `Test_readTar` — counts 10 data items per the current golden tar.
- `Test_readMeta` — unit tests for 7-col and 8-col meta; unaffected unless `metadata` struct is extended.

### What is NOT covered yet
- recorder statefulness across ticks
- tarRecorder collecting procfs data
- sysinfo write/read cycle
- report pipeline for procpidstat report type
- `describeReport` for "procpidstat"
- `-N` flag in cmd/report

---

## 6. Shared Utilities

| Function | File | Purpose |
|----------|------|---------|
| `stat.getSysticksLocal()` | `internal/stat/stat.go` | `getconf CLK_TCK` → float64; used in `NewCollector`; recorder needs same call |
| `stat.NewPGresultQuery(db, query)` | `internal/stat/postgres.go` | SQL query → PGresult; used by recorder collect |
| `stat.NewPGresultFile(r, size)` | `internal/stat/postgres.go` | JSON-unmarshal from tar reader → PGresult |
| `buildProcPidResult(...)` | `internal/stat/procpidstat.go` | Joins SQL 7-col + procfs maps → 19-col PGresult |
| `readProcPidStat(pid)` | `internal/stat/procpidstat.go` | `/proc/[pid]/stat` → ProcPidStat |
| `readProcPidIO(pid)` | `internal/stat/procpidstat.go` | `/proc/[pid]/io` → ProcPidIO |
| `CheckIOAvailable(pid)` | `internal/stat/procpidstat.go` | Probe cross-process IO access |
| `CheckDelayAcctAvailable()` | `internal/stat/procpidstat.go` | Probe `/proc/sys/kernel/task_delayacct` |
| `stat.Compare(curr, prev, itv, interval, skey, desc, ukey)` | `internal/stat/postgres.go` | Public wrapper for calculateDelta; called by report countDiff |
| `newFilenameString(ts, name)` | `record/recorder.go` | `name.YYYYMMDDTHHMMSS.mmm.json` format |
| `align.SetAlign(res, limit, dynamic)` | `internal/align/` | Computes column widths for display |
| `formatCPUTime(jiffies, ticks)` | `internal/stat/procpidstat.go` | jiffies → HH:MM:SS string |
| `delta(prev, curr)` | `internal/stat/procpidstat.go` | Saturating subtraction (returns 0 if curr≤prev) |

---

## 7. Potential Problems

### P1 — tarRecorder is stateless: cannot compute per-tick rates (High)
`tarRecorder` currently opens/closes a DB connection and discards all state between `record` loop iterations. To collect procpidstat with live rates, the recorder must retain `prevProcPidStats`, `currProcPidStats`, `prevProcPidIO`, `currProcPidIO` between calls. The `recorder` interface has no lifecycle hook between ticks — only `open/collect/write/close`. If `open/close` continues to be called every tick (see `record.go:record()`), the recorder must store procfs state as struct fields, not local variables inside `collect()`.

Alternative: Change `record()` loop to keep recorder open across ticks and only re-open when appending. This is a deeper refactor. Minimal-change approach: add procfs maps as fields of `tarRecorder`.

### P2 — DiffIntvl=[0,0] means rates from the recorder must already be per-second (Medium)
When the report reads two consecutive snapshots and calls `countDiff(curr, prev, itv, v)` with `DiffIntvl=[0,0]`, `calculateDelta` returns `curr` unchanged (pass-through). This means the `%all`, `%us`, `read,KiB/s` etc. columns in the tarred PGresult must already be per-second rates, not raw accumulated deltas. The recorder must call `buildProcPidResult` with the real interval (seconds between ticks) to produce correct rate strings. The report will then show the pre-computed rate directly.

**Consequence**: The report cannot re-derive rates from raw values unless a true MVC split is implemented (Option A). With Option B (store display strings), rates shown in the report always reflect the recording interval, not any custom replay interval.

### P3 — HH:MM:SS strings in cols 6–11 cannot be diffed (Medium)
If a future DiffIntvl is ever set on procpidstat, `diffPair` would be called on "HH:MM:SS" strings and return an error (parsePairInt/Float both fail on colon-separated strings). However with `DiffIntvl=[0,0]` this branch is never reached. Keep this constraint documented.

### P4 — sysinfo JSON schema must be stable (Low)
If ticks or cpuCount are stored as a custom struct, it must be JSON-serializable and backward-compatible. A minimal approach is to encode it as a PGresult (1 row, 2 cols: "ticks" and "cpu_count"), reusing `NewPGresultFile` on the read side.

Alternatively, define a dedicated struct:
```go
type SysInfo struct {
    Ticks    float64 `json:"ticks"`
    CPUCount int     `json:"cpu_count"`
}
```
And use `json.Marshal` / `json.Unmarshal` directly (same as write already does for PGresult). This is cleaner but requires adding a new field to the `data` channel struct or an extended `metadata`.

### P5 — ADR [001]: NotRecordable was intentional to avoid 7-col misleading output (Low — now resolved)
ADR `[001-feat-per-process-system-stats] NotRecordable bool on view.View` explicitly set `NotRecordable: true`. This feature (003) supersedes that decision. The tech-debt entry `[002] procpidstat record/report — not integrated with recorder` identifies this as the target fix. No architectural conflict; just needs explicit ADR supersession note.

### P6 — TestFilterViews_NotRecordable test will invert (Low)
`record/record_test.go:TestFilterViews_NotRecordable` explicitly asserts that procpidstat IS filtered out. When `NotRecordable` becomes `false`, this test must be updated to assert procpidstat passes through. Similarly `Test_filterViews` table counts will shift by 1.

### P7 — Report golden tar has no procpidstat entries (Low)
`report/testdata/pgcenter.stat.golden.tar` does not contain `procpidstat.*` or `sysinfo.*` entries. New test cases for `Test_app_doReport` require either: (a) creating a new tar with procpidstat entries synthetically, or (b) a separate test that builds an in-memory tar reader. The golden file approach (writing actual tar bytes) is used for all other report types.

### P8 — recorder.go open/close per tick: io overhead for procfs reads (Low)
`record.go:record()` calls `open()` → `collect()` → `write()` → `close()` on every tick. For procpidstat, `collect()` must iterate all PIDs in the activity result and call `readProcPidStat`/`readProcPidIO` per PID. This is the same work done by the TUI, and acceptable for the same interval. No concurrency concern.

---

## 8. Constraints & Infrastructure

### record() loop lifecycle — open/close every tick
```go
// record/record.go:record()
for { ... err := app.recorder.open(); ... app.recorder.collect(); ... app.recorder.write(); ... app.recorder.close() ... }
```
`tarRecorder` state added as struct fields will persist between loop iterations because the same `recorder` instance is reused across ticks. Only the `*tar.Writer` and `*os.File` are re-created each iteration. Procfs maps stored as struct fields will survive fine.

### report pipeline — data channel carries one `metadata` per pair
`readTar()` buffers `meta` and `stat` from the same timestamp and sends them together. A `sysinfo` entry with the same timestamp must also be buffered and included. Currently `metaOK` and `statOK` flags gate sending. Adding `sysinfoOK` would require matching all three before sending, which changes the pairing logic.

Alternative: include sysinfo in the `metadata` struct so it is carried without changing the `data` channel struct:
```go
type metadata struct {
    version  int
    ticks    float64
    cpuCount int
}
```
Then `readTar` sets `metaOK=true` for both `meta.*` and `sysinfo.*` entries (the last one wins, or merge them). This avoids changing the channel type.

### isFilenameOK — 4-part split assumption
`strings.Split(name, ".")` splits `procpidstat.20210614T115633.123.json` into 4 parts. A view named `procpidstat` has no embedded dots, so this works. `sysinfo` is also dot-free: both fit the 4-part format.

### cmd/report/report.go — flag letter `-N` availability
Existing flags: `-A`, `-R`, `-T`, `-I`, `-S`, `-F`, `-W`, `-D`, `-X`, `-P`. Letter `-N` is free (not a standard Go flag either).

### Build / test
- `make build` — no special requirements; recorder and report are CLI subcommands.
- `make test` — runs with `-race` flag; any concurrent map access in `tarRecorder` would be caught.
- `make lint` — golangci-lint; new exported types need godoc comments.
- No new external dependencies required: all needed packages (`archive/tar`, `encoding/json`, `runtime`, `os`, `strconv`) are already imported.

---

## 9. External Libraries

No new external libraries required.

`runtime.NumCPU()` is already used in `internal/stat/stat.go` (imported) for `cpuCount` in the TUI path. The recorder needs the same value for its `buildProcPidResult` call.

---

---

## Updated: 2026-05-19 — Implementation-Level Deepening

### IQ1 — postgres.DB struct; how db.Local is accessed in record/record.go

`/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/postgres/postgres.go` defines:

```go
type Config struct {
    Config *pgx.ConnConfig
}

type DB struct {
    Config Config
    Conn   *pgx.Conn
    Local  bool // is Postgres running on localhost?
}
```

`Local` is set in `Connect()` at line 101: `Local: isLocalhost(config.Config.Host)`.

In `record/record.go:setup()`, `postgres.Connect(app.dbConfig)` is called and the returned `*postgres.DB` is used **only** inside `setup()` for `stat.GetPostgresProperties(db)` and then immediately `defer db.Close()`-d. The `db` variable is local to `setup()`; it is **never stored in `app`** and never passed to `tarRecorder`. The `app` struct only stores `dbConfig postgres.Config` (the connection config, not the live connection).

`tarRecorder.collect()` receives `dbConfig postgres.Config` (the config value, not a `*DB`). It calls `postgres.Connect(dbConfig)` internally to open a fresh connection per tick, but does **not** access `db.Local`.

**Consequence for procpidstat recording**: The recorder has no access to `db.Local` at collect time. To gate procfs enrichment on "is local?", the recorder must either:
- Check `db.Local` from the freshly opened connection inside `collect()` — it calls `postgres.Connect(dbConfig)` and gets back a `*postgres.DB` whose `.Local` field is already set, so `db.Local` is directly readable there.
- Or store `isLocal bool` as a field of `tarRecorder`, populated once during `app.setup()` before `db.Close()`.

The second approach (store once at setup) is cleaner and avoids a per-tick check. `app.setup()` already has a live `db *postgres.DB` with `Local` populated — it can pass `db.Local` into `tarRecorder` fields via `newTarRecorder(tarConfig{..., local: db.Local})`.

---

### IQ2 — recorder.go collect(): exact operation sequence and procpidstat insertion point

Full body of `tarRecorder.collect()` (`record/recorder.go` lines 95–124):

```
1. postgres.Connect(dbConfig)  → *postgres.DB (defer Close)
2. stat.NewPGresultQuery(db, query.SelectCommonProperties)  → stats["meta"]
3. for k, v := range views {
       stat.NewPGresultQuery(db, v.Query)  → stats[k]
   }
4. return stats, nil
```

The map iteration order is non-deterministic. There is no ordering requirement between views.

**Insertion point for procpidstat enrichment**: After step 3, but gated on `k == "procpidstat"`:

```go
for k, v := range views {
    res, err := stat.NewPGresultQuery(db, v.Query)
    if err != nil {
        return nil, err
    }
    // Procpidstat special handling: enrich the 7-col SQL result with procfs data.
    if k == "procpidstat" && c.isLocal {
        // rotate maps
        // collect procfs per PID
        // enriched := buildProcPidResult(res, c.prev*, c.curr*, ...)
        // stats[k] = enriched
    } else {
        stats[k] = res
    }
}
```

The `tarRecorder` struct must gain fields: `isLocal bool`, `ticks float64`, `cpuCount int`, `ioAvailable bool`, `delayAcctAvailable bool`, `prevProcPidStats map[int]ProcPidStat`, `currProcPidStats map[int]ProcPidStat`, `prevProcPidIO map[int]ProcPidIO`, `currProcPidIO map[int]ProcPidIO`.

These fields survive between `collect()` calls because `tarRecorder` is a single struct reused across the record loop (only `file` and `writer` are recreated per `open()`/`close()`).

---

### IQ3 — getSysticksLocal() signature; runtime.NumCPU() location; Collector.Update() procpidstat block

**getSysticksLocal** (`internal/stat/stat.go` lines 372–384):
```go
func getSysticksLocal() (float64, error)
```
Executes `getconf CLK_TCK`, parses as float64, returns it. Unexported. Called only from `NewCollector()`. The recorder must call it once in `newTarRecorder()` or in `app.setup()` before the recorder is created.

**runtime.NumCPU()** is used in `internal/stat/stat.go` at line 271, inside `Collector.Update()`:
```go
enriched := buildProcPidResult(
    res,
    c.prevProcPidStats, c.currProcPidStats,
    c.prevProcPidIO, c.currProcPidIO,
    view.IOAvailable,
    view.DelayAcctAvailable,
    c.config.ticks,
    float64(itv),
    runtime.NumCPU(),   // ← line 271
)
```
The `runtime` package is already imported in `stat.go`. The recorder will need the same call.

**Collector.Update() procpidstat enrichment block** (`internal/stat/stat.go` lines 215–275): exact sequence:

```
1. if view.CollectExtra == CollectProcPidStat {
2.   Build newPrevStats, newPrevIO by iterating res.Values:
       for each row: parse pid from row[0]; if currProcPidStats[pid] exists → newPrevStats[pid]=it
3.   c.prevProcPidStats = newPrevStats; c.prevProcPidIO = newPrevIO
4.   c.currProcPidStats = make(map[int]ProcPidStat{}); c.currProcPidIO = make(map[int]ProcPidIO{})
5.   for each row in res.Values: parse pid; readProcPidStat(pid) → c.currProcPidStats[pid]
       if view.IOAvailable: readProcPidIO(pid) → c.currProcPidIO[pid]
6.   enriched := buildProcPidResult(res, c.prevProcPidStats, c.currProcPidStats, c.prevProcPidIO, c.currProcPidIO,
         view.IOAvailable, view.DelayAcctAvailable, c.config.ticks, float64(itv), runtime.NumCPU())
7.   s.Pgstat.Result = enriched; res = enriched
```

The recorder's `collect()` should mirror this exact pattern (steps 2–7) using `tarRecorder` fields instead of `Collector` fields.

---

### IQ4 — report/report.go readTar(): gating logic and sysinfo insertion point

**isFilenameOK** (`report/report.go` lines 309–323):
```go
func isFilenameOK(name string, report string) error {
    s := strings.Split(name, ".")
    if len(s) != 4 {
        return fmt.Errorf("bad file name format %s, skip", name)
    }
    if s[0] != report && s[0] != "meta" {
        return fmt.Errorf("skip sample")
    }
    return nil
}
```

A `sysinfo.20260519T120000.000.json` entry has `len(s)==4` and `s[0]=="sysinfo"`. It passes the length check but fails the `s[0] != report && s[0] != "meta"` check → **skipped silently**.

**readTar gating** (`report/report.go` lines 137–198):
```go
var metaOK, statOK bool
...
if strings.HasPrefix(hdr.Name, "meta.") {
    // reads meta, sets metaOK=true
} else {
    // reads stat, sets statOK=true
}
if !metaOK || !statOK { continue }
dataCh <- data{ts, res, meta}
metaOK, statOK = false, false
```

**Insertion point for sysinfo**: Two options:
- Option B-simple: treat `sysinfo.*` as an extension of `meta.*` — when `strings.HasPrefix(hdr.Name, "sysinfo.")` is detected, decode the JSON into a `SysInfo` struct and merge its fields into the current `metadata` value. This requires no change to the `data` channel struct or the `metaOK`/`statOK` pairing logic. The `metadata` struct gains `ticks float64` and `cpuCount int` fields.
- Option B-alt: add a third flag `sysinfoOK` and require all three before sending. More robust but changes the pairing invariant.

Option B-simple is lower risk. The `isFilenameOK` function must be extended to also accept `"sysinfo"` as a valid prefix (alongside `"meta"`).

---

### IQ5 — view.go Configure(): procpidstat special cases

`Configure()` (`internal/view/view.go` lines 303–339) uses a `switch k` over view names. There is **no `case "procpidstat":` branch**. The procpidstat view falls through to the default `query.Format(view.QueryTmpl, opts)` call.

`query.PgStatActivityProcPidStat` is the query template. Calling `query.Format` on it with a `query.Options` that has `QueryAgeThresh` and `ShowNoIdle` fields works correctly (the template uses both). No version branching is needed for procpidstat.

`Configure()` also has no special handling for `IOAvailable` or `DelayAcctAvailable` — those flags are set interactively by the TUI (`top/config_view.go:switchViewToProcPidStat`). In the recorder context, these flags must be set on the view **before** `views.Configure()` is called, or separately after. The recorder's `app.setup()` currently does not set them.

**Full procpidstat view definition** (lines 285–296 of `internal/view/view.go`):
```go
"procpidstat": {
    Name:          "procpidstat",
    QueryTmpl:     query.PgStatActivityProcPidStat,
    DiffIntvl:     [2]int{0, 0},
    Ncols:         19,
    OrderKey:      0,
    OrderDesc:     false,
    ColsWidth:     map[int]int{},
    Msg:           "Show per-process system stats",
    Filters:       map[int]*regexp.Regexp{},
    NotRecordable: true,
},
```
`CollectExtra`, `IOAvailable`, `DelayAcctAvailable`, `ShowExtra` all have zero/false values in `New()`. They are set dynamically by the TUI.

---

### IQ6 — record/record_test.go: test bodies and table structure

**TestFilterViews_NotRecordable** (lines 125–146):
```go
func TestFilterViews_NotRecordable(t *testing.T) {
    views := view.Views{
        "procpidstat": {Name: "procpidstat", NotRecordable: true},
    }
    n, v := filterViews(0, "", views)
    assert.Equal(t, 1, n)
    assert.Equal(t, 0, len(v))
    assert.NotContains(t, v, "procpidstat")

    all := view.New()
    pp, ok := all["procpidstat"]
    assert.True(t, ok)
    assert.True(t, pp.NotRecordable)
    assert.Equal(t, 19, pp.Ncols)
}
```

When `NotRecordable` is changed to `false`, this test must be updated: the `filterViews` call on a single `NotRecordable:true` view is a unit test for the filter, so the input view definition and expected values must change together. The sanity-check assertions at the bottom (`pp.NotRecordable == true`) will also fail.

**Test_filterViews table** (lines 100–123):
```go
testcases := []struct {
    version    int
    pgssSchema string
    wantN      int
    wantV      int
}{
    {version: 140000, pgssSchema: "",       wantN: 7, wantV: 15},
    {version: 140000, pgssSchema: "public", wantN: 1, wantV: 21},
    {version: 130000, pgssSchema: "public", wantN: 4, wantV: 18},
    {version: 120000, pgssSchema: "public", wantN: 7, wantV: 15},
    {version: 110000, pgssSchema: "public", wantN: 9, wantV: 13},
    {version: 100000, pgssSchema: "public", wantN: 9, wantV: 13},
}
```
Comments in the file state: "procpidstat (NotRecordable) is always filtered out, so wantN includes +1 for it and wantV stays at 21 max recordable." When `NotRecordable` becomes `false`, each `wantN` decreases by 1 and each `wantV` increases by 1.

**Test_app_record** (lines 32–98): uses `countRecordable(view.New()) + 1` (stats + metadata). `countRecordable` (lines 163–173) counts views with `!v.NotRecordable`. When procpidstat becomes recordable, `countRecordable` automatically returns one more — no direct change needed in this test body.

---

### IQ7 — SysInfo-like struct

No `SysInfo` struct exists anywhere in the codebase (`grep -rn "SysInfo\|sysinfo\|Sysinfo"` returns zero results in production code). It must be defined from scratch. Minimum definition:

```go
// SysInfo holds host-level parameters needed to interpret procpidstat rate columns.
type SysInfo struct {
    Ticks    float64 `json:"ticks"`
    CPUCount int     `json:"cpu_count"`
}
```

This can live in `record/recorder.go` (write side) and `report/report.go` (read side), or in `internal/stat/` if shared. Given it is a serialization-only struct, placing it in `record/` and `report/` separately (or a shared `internal/stat/sysinfo.go`) is equally valid. No existing code needs to be changed to accommodate it.

---

### IQ8 — report/report.go: metadata struct and readMeta() full body

**metadata struct** (lines 94–96):
```go
type metadata struct {
    version int
}
```
Single unexported field. The `data` channel struct (lines 99–103):
```go
type data struct {
    ts   time.Time
    res  stat.PGresult
    meta metadata
}
```

**readMeta() full body** (lines 295–306):
```go
func readMeta(res stat.PGresult) (metadata, error) {
    if res.Nrows != 1 || res.Ncols < 2 {
        return metadata{}, fmt.Errorf("invalid result")
    }
    version, err := strconv.ParseInt(res.Values[0][1].String, 10, 64)
    if err != nil {
        return metadata{}, err
    }
    return metadata{version: int(version)}, nil
}
```

Reads column index 1 (`version_num`) only. Accepts `Ncols >= 2` (tolerates both 7-col and 8-col meta). Is completely decoupled from sysinfo — adding `ticks` and `cpuCount` to `metadata` and populating them from a separate `sysinfo.*` tar entry will not touch `readMeta()`.

---

### IQ9 — CheckIOAvailable and CheckDelayAcctAvailable exact signatures

From `internal/stat/procpidstat.go` lines 171–198:

```go
func CheckIOAvailable(pid int) error
```
Opens `/proc/<pid>/io`. Returns `nil` on success, OS error (typically `EACCES`) on failure. The caller supplies a PID of a different-user process (a PostgreSQL backend). Returns error if that file is not readable by the current user.

```go
func CheckDelayAcctAvailable() bool
```
Opens `/proc/sys/kernel/task_delayacct`, reads up to 4 bytes, returns `true` iff content trims to `"1"`. Returns `false` if file is absent or contains `"0"`. No error return — failures are silently mapped to `false`.

Both are exported (uppercase first letter). Both live in `internal/stat/procpidstat.go`.

**How they are called in the TUI** (`top/config_view.go:switchViewToProcPidStat`, not shown here): the TUI calls `CheckIOAvailable(somePid)` and `CheckDelayAcctAvailable()` and stores results in `view.IOAvailable` and `view.DelayAcctAvailable`. The recorder must do the same before running `buildProcPidResult`. The PID to probe for `CheckIOAvailable` can be the first PID from the initial `pg_stat_activity` result.

---

### IQ10 — recorder app struct; db.Local accessibility from tarRecorder

`record/record.go` `app` struct (lines 46–51):
```go
type app struct {
    config   Config
    dbConfig postgres.Config
    views    view.Views
    recorder recorder
}
```

The `recorder` interface is `open/collect/write/close`. `tarRecorder` implements it. `app` holds the `recorder` as an interface value — `app` does not expose `tarRecorder` fields directly.

`db.Local` is **not accessible from `tarRecorder`** at `collect()` time via the `app` struct. The `tarRecorder` receives only `(dbConfig postgres.Config, views view.Views)` in `collect()`.

**Access path at collect time**: Inside `tarRecorder.collect()`, a fresh `*postgres.DB` is created via `postgres.Connect(dbConfig)`. That `db.Local` field is already correctly set (it calls `isLocalhost` on `dbConfig.Config.Host`). So `c.isLocal := db.Local` can be captured right there — but it's redundant to check every tick.

**Recommended approach**: Capture `db.Local` once in `app.setup()` before `defer db.Close()`, pass it into `newTarRecorder` via `tarConfig`. This requires:
1. Adding `isLocal bool` to `tarConfig`.
2. Setting `app.recorder = newTarRecorder(tarConfig{filename: ..., append: ..., isLocal: db.Local})`.
3. Adding `isLocal bool` to `tarRecorder` struct, set from `tarConfig` in `newTarRecorder`.

If `isLocal` is false (remote Postgres), procfs enrichment is skipped in `collect()` and `stats["procpidstat"]` gets the raw 7-col SQL result. This matches the TUI behavior where procpidstat is disabled for remote connections.

---

## Answers to Research Questions

**Q1: Exact column layout of buildProcPidResult output?**
19 columns. Cols 0–5 verbatim SQL labels; cols 6–11 accumulated (HH:MM:SS or KiB strings); cols 12–17 rate floats (already per-second); col 18 query string. See Data Layer table above.

**Q2: How does stat.Compare() handle DiffIntvl=[0,0]?**
`calculateDelta` checks `if interval != [2]int{0, 0}`. When interval IS `{0,0}`, it skips the `diff()` call entirely and sets `delta = curr` (identity). Then it sorts. No column subtraction happens. This means the recorder must store display-ready values — rates must be precomputed at record time.

**Q3: What does meta.* JSON look like in practice?**
A PGresult with `Ncols: 7` (older files) or `Ncols: 8` (current), `Nrows: 1`. Cols: `["version", "version_num", "track_commit_timestamp", "max_connections", "autovacuum_max_workers", "recovery"/"shared_preload_libraries", "recovery"/"start_time_unix"]`. `readMeta` extracts only col index 1 (`version_num`) as `metadata.version`. The sysinfo entry must be a separate tar entry (not embedded in meta) to avoid breaking `readMeta`.

**Q4: Golden files for procpidstat that will need updating?**
No golden files exist yet for procpidstat. `TestBuildProcPidResult_*` tests in `internal/stat/procpidstat_test.go` use inline assertions (no golden files) — they will NOT need updating for the MVC split if the public 19-col output of `buildProcPidResult` is preserved. The golden tar (`report/testdata/pgcenter.stat.golden.tar`) has no procpidstat entries and will need procpidstat + sysinfo entries added for the report integration test. `record/record_test.go` golden counts will shift.

**Q5: What other code calls buildProcPidResult?**
Only one caller in production code: `internal/stat/stat.go:Collector.Update()` at line 263. No other callers. An MVC split of the function's signature affects only this one call site.

**Q6: How does the report handle UniqueKey for row matching? procpidstat UniqueKey?**
`stat.diff()` matches rows between snapshots using `cv[ukey].String != pv[ukey].String`. For procpidstat the view defines `UniqueKey: 0` (default zero value, pid column). With `DiffIntvl=[0,0]`, `diff()` is never called — `calculateDelta` returns `curr` unchanged. UniqueKey=0 is correct if diffing is ever enabled in future.

**Q7: Full list of files touching procpidstat today?**

| File | Role |
|------|------|
| `internal/stat/procpidstat.go` | ProcPidStat/ProcPidIO structs, readProcPidStat, readProcPidIO, buildProcPidResult, helpers |
| `internal/stat/procpidstat_test.go` | Unit tests for above |
| `internal/stat/stat.go` | Collector.Update() enrichment block; CollectProcPidStat constant |
| `internal/stat/stat_test.go` | TestCollectorUpdateProcPidStat19Cols |
| `internal/stat/testdata/proc/` | 7 golden proc files (pid_stat_*, pid_io_*) |
| `internal/view/view.go` | procpidstat view definition (NotRecordable, DiffIntvl, CollectExtra) |
| `internal/query/procpidstat.go` | PgStatActivityProcPidStat query template |
| `record/record.go` | filterViews() — NotRecordable guard |
| `record/record_test.go` | TestFilterViews_NotRecordable, Test_filterViews counts |
| `top/config_view.go` | switchViewToProcPidStat(), toggle logic |
| `top/keybindings.go` | 'S' binding → switchViewToProcPidStat |
| `top/dialog.go` | procpidstat name check for age/idle toggles |
| `report/report.go` | describeReport map (procpidstat missing) |
| `report/report_test.go` | Test_app_doReport (procpidstat missing), Test_describeReport (missing) |
| `cmd/report/report.go` | selectReport(), init() flags (procpidstat missing) |
