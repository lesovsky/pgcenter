# Code Research — [012] PostgreSQL 19 compatibility baseline

**Date:** 2026-07-25
**Feature base:** `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline`
**Sources read:** `docs/roadmap-0.12.0.md` (§ [012]), the feature interview YAML, `docs/decisions-log.md`,
`docs/tech-debt.md`, `.claude/skills/project-knowledge/{patterns,architecture,deployment,overview}.md`,
and the production/test code cited below. No spec file exists yet (`{feature_base}.md` absent).

External catalog facts (PG 19 column names/values) were verified against
`https://www.postgresql.org/docs/19/progress-reporting.html` — not from memory.

---

## 1. Entry Points

### 1.1 Version constants — the single seam

`internal/query/query.go:9-22` — the `const` block ends at `PostgresV18 = 180000`.
`PostgresV19 = 190000` goes on line 22 (after `PostgresV18`). Nothing else in this file needs to change:
`selectWalFunctions()` (`query.go:68-87`) branches only at `< PostgresV10`.

### 1.2 Test cluster port map

`internal/postgres/testing.go:16-44` — `NewTestConnectVersion(version int) (*DB, error)`.
The `ports` map (`testing.go:17-32`) is keyed by numeric version; the "active versions" block
(`testing.go:18-23`) starts at `180000: 21918`. Add `190000: 21919` above it.

Two behaviours worth knowing:
- Unknown version falls back to `ports[140000]` (`testing.go:35-37`) — an unmapped 190000 would
  silently connect to the **PG 14** cluster and the tests would pass while proving nothing. This is the
  quiet failure mode if the port map entry is forgotten.
- `NewTestConnect()` (`testing.go:9-11`) hardcodes `170000`; `NewTestConfig()` (`testing.go:4-6`)
  hardcodes port `21917`. These are the "default" fixture used by `record`/`report`/`top` tests.
  **Do not repoint them at PG 19** — that would change the version under which the whole non-`query`
  suite runs.

### 1.3 View registry and the version-aware patch point

`internal/view/view.go:38-361` — `New() Views` returns the 27-view map (static, PG-oldest-shape defaults).
`internal/view/view.go:367-418` — `func (v Views) Configure(opts query.Options) error`:
a `switch k` over view names (lines 373-404) that patches version-dependent fields, then a second loop
(lines 408-415) that renders `QueryTmpl` → `Query` through `query.Format`.

Callers of `Configure` (3, all pass a real server version):
- `top/top.go:63` — once at TUI startup.
- `record/record.go:129` — once at `pgcenter record` setup.
- `report/report.go:258` — at **replay** time, keyed on the archive's recorded version (see §5).

`internal/view/view.go:421-423` — `func (v View) VersionOK(version int) bool { return version >= v.MinRequiredVersion }`.
Used at `record/record.go:214` (drop unsupported views from a recording) and `internal/stat/stat.go:317`
(TUI runtime guard → `"selected statistics is not supported by current version of Postgres"`).

### 1.4 The three progress screens in the TUI

Hotkey `p` cycles, `P` opens the menu:
- `top/keybindings.go:40` → `switchViewTo(app, "progress")`
- `top/config_view.go:148-149` → `progressNextView(...)`; the cycle itself at `top/config_view.go:216-232`
- `top/menu.go:66-73` (menu items), `top/menu.go:179-191` (dispatch)

No changes needed here — this feature adds columns, not screens.

---

## 2. Data Layer

### 2.1 `internal/query/progress_vacuum.go` — current layout (13 cols)

Single const `PgStatProgressVacuumDefault` (`progress_vacuum.go:5-13`).
`pg_stat_progress_vacuum v RIGHT JOIN pg_stat_activity a ON v.pid = a.pid`,
`WHERE (a.query ~* '^autovacuum:' OR a.query ~* '^vacuum') AND a.pid <> pg_backend_pid() ORDER BY a.pid DESC`.

| idx | column | source |
|----|--------|--------|
| 0 | `pid` | `a.pid` |
| 1 | `xact_age` | `clock_timestamp() - xact_start` |
| 2 | `datname` | `v.datname` |
| 3 | `relation` | `v.relid::regclass` |
| 4 | `state` | `a.state` |
| 5 | `waiting` | `coalesce(wait_event_type||'.'||wait_event,'f')` |
| 6 | `phase` | `v.phase` |
| 7 | `size_total,KiB` | `heap_blks_total` |
| 8 | `scanned_total,%` | `heap_blks_scanned` |
| 9 | `vacuumed_total,%` | `heap_blks_vacuumed` |
| **10** | `scanned,KiB` | diffed |
| **11** | `vacuumed,KiB` | diffed |
| 12 | `query` | `a.query` |

View entry `internal/view/view.go:277-288`: `MinRequiredVersion: query.PostgresV96`, `Ncols: 13`,
`DiffIntvl: [2]int{10,11}`, `OrderKey: 0`, `OrderDesc: true`, `UniqueKey` **unset (0 = pid)**.
Not in the `Configure()` switch today.

### 2.2 `internal/query/progress_analyze.go` — current layout (12 cols)

Const `PgStatProgressAnalyzeDefault` (`progress_analyze.go:5-14`). `INNER JOIN pg_stat_activity`.

`0 pid | 1 xact_age | 2 datname | 3 relation | 4 state | 5 waiting | 6 phase | 7 sample_size,KiB |
8 scanned,% | 9 ext_total/done | 10 child_total/done,% | 11 child_in_progress`

View entry `internal/view/view.go:325-336`: `MinRequiredVersion: query.PostgresV13`, `Ncols: 12`,
`DiffIntvl: [2]int{0,0}` (snapshot view — no diff at all, see §3.3), `OrderKey: 0`, `UniqueKey` unset.

### 2.3 `internal/query/progress_basebackup.go` — current layout (11 cols)

Const `PgStatProgressBasebackupDefault` (`progress_basebackup.go:5-15`). `INNER JOIN pg_stat_activity`.

`0 pid | 1 started_from | 2 started_at | 3 duration | 4 state | 5 waiting | 6 phase | 7 size_total,KiB |
8 streamed,% | **9 streamed,KiB** | 10 tablespaces_total/streamed`

View entry `internal/view/view.go:337-348`: `MinRequiredVersion: query.PostgresV13`, `Ncols: 11`,
`DiffIntvl: [2]int{9,9}`, `OrderKey: 0`, `UniqueKey` unset.

### 2.4 PG 19 catalog columns (verified against postgresql.org/docs/19)

| view | new column | type | values |
|---|---|---|---|
| `pg_stat_progress_vacuum` | `mode` | text | `normal`, `aggressive`, `failsafe` |
| `pg_stat_progress_vacuum` | `started_by` | text | `manual`, `autovacuum`, `autovacuum_wraparound` |
| `pg_stat_progress_analyze` | `started_by` | text | `manual`, `autovacuum` |
| `pg_stat_progress_basebackup` | `backup_type` | text | `full`, `incremental` |

Also present in the PG 19 catalog but **out of scope for this feature**: `delay_time double precision`
on both `pg_stat_progress_vacuum` and `pg_stat_progress_analyze`.

### 2.5 Target layouts under head placement (variant B, decided in interview batch 1 Q3)

**progress_vacuum, PG 19 — 15 cols**, inserting `started_by`, `mode` at idx 3-4 (after `datname`):

`0 pid | 1 xact_age | 2 datname | 3 started_by | 4 mode | 5 relation | 6 state | 7 waiting | 8 phase |
9 size_total,KiB | 10 scanned_total,% | 11 vacuumed_total,% | **12 scanned,KiB** | **13 vacuumed,KiB** | 14 query`

→ `(query, 15, [2]int{12,13})`. Matches the roadmap/interview figure `{10,11} → {12,13}`.

**progress_analyze, PG 19 — 13 cols**, inserting `started_by` at idx 3:

`0 pid | 1 xact_age | 2 datname | 3 started_by | 4 relation | … | 12 child_in_progress`

→ `(query, 13, [2]int{0,0})` — DiffIntvl unaffected (already `{0,0}`).

**progress_basebackup, PG 19 — 12 cols**, inserting `backup_type` at idx 1 (after `pid`):

`0 pid | 1 backup_type | 2 started_from | 3 started_at | 4 duration | 5 state | 6 waiting | 7 phase |
8 size_total,KiB | 9 streamed,% | **10 streamed,KiB** | 11 tablespaces_total/streamed`

→ `(query, 12, [2]int{10,10})`. Matches `{9,9} → {10,10}`.

`OrderKey` stays 0 (`pid`) in all three — column 0 is `pid` in every layout, so the default sort and
`UniqueKey=0` row matching are unchanged across the version boundary. **This is load-bearing**: had a
new column been inserted at index 0, `UniqueKey` would have had to become version-dependent too
(the [007] 4-tuple problem).

---

## 3. Similar Features — the version-aware selector pattern

### 3.1 The two existing shapes

`internal/query/io.go:87-92`:
```go
func SelectStatIOQuery(version int) (string, int, [2]int) {
	if version >= PostgresV18 {
		return PgStatIOPG18, 16, [2]int{4, 14}
	}
	return PgStatIOPG16, 16, [2]int{4, 14}
}
```

`internal/query/bgwriter.go:41-52` — same 3-tuple, three branches, raw literals `180000`/`170000`
(the constants did not exist when it was written; `io.go` uses `PostgresV18`). Both are wired into
`Configure()` identically:

- `internal/view/view.go:392-394` — `case "bgwriter": view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatBgwriterQuery(opts.Version)`
- `internal/view/view.go:398-400` — `case "stat_io": …SelectStatIOQuery(opts.Version)`

Also in the switch: `activity`/`replication` use a **2-tuple** `(string, int)` (`view.go:375,378`);
`statements_jit` uses a **4-tuple** `(string, int, [2]int, int)` incl. `UniqueKey` (`view.go:387`,
ADR `docs/decisions-log.md:397`).

### 3.2 What the three new selectors must look like

`internal/query/progress_vacuum.go` gains a second const + a selector:

```go
// PgStatProgressVacuumPG19 — as Default plus started_by/mode at idx 3-4.
const PgStatProgressVacuumPG19 = "SELECT a.pid, …, v.datname, v.started_by, v.mode, v.relid::regclass AS relation, …"

// SelectStatProgressVacuumQuery returns query, column count and diff interval for
// pg_stat_progress_vacuum. PG19 inserts started_by+mode at idx 3-4, shifting the
// diffed scanned/vacuumed,KiB pair from {10,11} to {12,13}.
func SelectStatProgressVacuumQuery(version int) (string, int, [2]int) {
	if version >= PostgresV19 {
		return PgStatProgressVacuumPG19, 15, [2]int{12, 13}
	}
	return PgStatProgressVacuumDefault, 13, [2]int{10, 11}
}
```

Analogously `SelectStatProgressBasebackupQuery(version) (string, int, [2]int)`
→ `(PG19, 12, {10,10})` / `(Default, 11, {9,9})`.

For `progress_analyze` the DiffIntvl never changes, so a 2-tuple
`SelectStatProgressAnalyzeQuery(version) (string, int)` is sufficient and has precedent
(`SelectStatActivityQuery`). **Recommendation: use the 3-tuple anyway** — all three screens are one
feature, one review, and a mixed arity across sibling selectors is the kind of asymmetry the [007] ADR
was written to avoid. Decide it in the tech-spec, do not leave it to the task author.

Wiring — three new cases in `internal/view/view.go` `Configure()` switch (alongside lines 398-403):

```go
case "progress_vacuum":
	view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatProgressVacuumQuery(opts.Version)
	v[k] = view
case "progress_analyze":
	view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatProgressAnalyzeQuery(opts.Version)
	v[k] = view
case "progress_basebackup":
	view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatProgressBasebackupQuery(opts.Version)
	v[k] = view
```

The static `view.New()` map entries **must keep their pre-19 values** (`13/{10,11}`, `12/{0,0}`,
`11/{9,9}`) — they are the pre-Configure defaults and are pinned by tests (§4).

### 3.3 How DiffIntvl is consumed

`internal/stat/postgres.go:590-597` (`Compare` → `diff`): if `interval == [2]int{0,0}` the diff is
**skipped entirely** (`delta = curr`). Otherwise `diff()` (`postgres.go:605-660`) walks
`0..curr.Ncols-1` and diffs only `interval[0] <= l <= interval[1]`, matching rows by `ukey`
(`postgres.go:624`). Note `diff.Ncols = len(curr.Cols)` (`postgres.go:611`) — **the result's own column
count, not the view's**. A stale `view.Ncols` therefore cannot cause an index panic in the diff path;
only a stale `DiffIntvl` produces wrong numbers (or a `strconv.ParseInt("")` abort if it points at a
text column). That is exactly what the version-aware selector prevents.

---

## 4. Existing Tests

**Framework:** testify (`assert`), stdlib `testing`, table-driven subtests. Runner: `make test`
(`go test -race -p 1 -timeout 300s -coverprofile=…`), `Makefile:38-41`. `-p 1` is deliberate — the
suite shares live clusters.

**Live-PG dependency:** everything in `internal/query/*_test.go`, `internal/stat/*_test.go`,
`record/record_test.go:Test_app_setup`/`Test_app_record` needs a running cluster and self-skips via
`t.Skipf` on connect failure (`internal/query/progress_vacuum_test.go:21-24` is the canonical form).
**Tests that run without PostgreSQL** — and therefore fail hard on a stale count:
`internal/view/view_test.go` (all), `record/record_test.go:Test_filterViews` and the three
`TestFilterViews_*`, all of `report/` (goldens + synthetic tars), all of `top/`.
`.claude/skills/project-knowledge/patterns.md:109` calls this out explicitly.

### 4.1 The exhaustive `versions := []int{...}` inventory

Every list below gains `190000` at the tail. 33 sites in 21 files:

| file:line | current list |
|---|---|
| `internal/query/activity_test.go:29` | 90500…180000 (full) |
| `internal/query/bgwriter_test.go:36` | 140000…180000 |
| `internal/query/common_test.go:64` | full |
| `internal/query/databases_test.go:34` | full |
| `internal/query/databases_test.go:56` | `[]int{140000}` — single-version, leave as is |
| `internal/query/functions_test.go:11` | full |
| `internal/query/indexes_test.go:11` | full |
| `internal/query/io_test.go:68` | `{160000, 180000}` |
| `internal/query/io_test.go:96` | `{160000, 180000}` |
| `internal/query/io_test.go:112` | `{160000, 170000, 180000}` |
| `internal/query/io_test.go:150` | `{160000, 170000, 180000}` |
| `internal/query/io_test.go:179` | `{160000, 180000}` |
| `internal/query/overview_test.go:13` | `overviewVersions = {140000…180000}` (package-level var) |
| `internal/query/pgcenter_schema_test.go:21` | full |
| `internal/query/procpidstat_test.go:81` | full |
| `internal/query/progress_analyze_test.go:11` | 140000…180000 |
| `internal/query/progress_basebackup_test.go:11` | 140000…180000 |
| `internal/query/progress_cluster_test.go:11` | 120000…180000 |
| `internal/query/progress_copy_test.go:11` | 140000…180000 |
| `internal/query/progress_create_index_test.go:11` | 120000…180000 |
| `internal/query/progress_vacuum_test.go:11` | 90600…180000 |
| `internal/query/replication_slots_test.go:34,113,161` | 140000…180000 (×3) |
| `internal/query/replication_test.go:39` | full |
| `internal/query/sizes_test.go:11` | full |
| `internal/query/statements_test.go:59,176` | full (×2) |
| `internal/query/statements_test.go:110` | 130000…180000 |
| `internal/query/statements_test.go:129` | 150000…180000 |
| `internal/query/tables_test.go:11` | full |
| `internal/query/wal_test.go:34` | 140000…180000 |
| `internal/stat/postgres_test.go:90` | 140000…180000 |

Plus the per-version **unit** tables (not connection loops) that assert `(Ncols, DiffIntvl)` per
version and should gain a `190000` row: `internal/query/bgwriter_test.go:22`,
`internal/query/wal_test.go:20`, `internal/query/replication_slots_test.go:20`,
`internal/query/statements_test.go:25,46,164`.

The three progress selector tests need a **new** unit table each (none exists today — the current
`Test_StatProgress*Queries` only executes the template), asserting the boundary at 180000 vs 190000.
Model: `Test_SelectStatBgwriterQuery` (`internal/query/bgwriter_test.go:10-33`).

### 4.2 Count-based tests (patterns.md § "Adding a New View")

This feature adds **no new view**, so the counts do not change — but rows should be added for 190000:

- `internal/view/view_test.go:9-12` `TestNew` — pins `27`. **Unchanged.**
- `internal/view/view_test.go:224-248` `TestView_VersionOK` — table `{version, total}`; highest row is
  `{160000, 27}`. Add `{version: 190000, total: 27}`. No existing row changes.
- `record/record_test.go:109-145` `Test_filterViews` — highest row is `{140000, …}`. Add
  `{version: 190000, pgssSchema: "public", wantN: 0, wantV: 27}` (on ≥16 every view passes the version
  gate, and with a pgss schema nothing is dropped). No existing row changes. This test runs without PG.
- `internal/view/view_test.go:99-222` `TestViews_Configure` — the matrix stops at 140000 and asserts
  per-version `QueryTmpl`/`Ncols` for old versions. Add a 190000 block asserting the three progress
  views resolve to their PG 19 templates and `Ncols` 15/13/12 — this is where the new `Configure` cases
  get their view-layer coverage (the [006] precedent, `docs/decisions-log.md:37`, says the branch is
  proven at the query layer and the view layer only needs to prove it calls the selector).
- `top/config_view_test.go:379-385,470-485` — pin the `p` cycle order. **Unchanged** (no new screen).

### 4.3 Representative signatures

```go
// internal/query/progress_vacuum_test.go:10 — live-execution loop, self-skipping
func Test_StatProgressVacuumQueries(t *testing.T)

// internal/query/bgwriter_test.go:10 — pure unit table over (version → Ncols, DiffIntvl)
func Test_SelectStatBgwriterQuery(t *testing.T)

// report/report_record_bgwriter_test.go:31 — synthetic in-memory tar + per-version goldens, no live PG
func Test_app_doReport_Bgwriter(t *testing.T)
```

---

## 5. Integration Points — the record/report path

### 5.1 Resolution chain for `report -P v|a|b`

1. `cmd/report/report.go:76` — `-P/--progress` string flag → `opts.showProgress`.
2. `cmd/report/report.go:185-199` — `selectReport()` maps `v|c|i|a|b|y` →
   `progress_vacuum|progress_cluster|progress_index|progress_analyze|progress_basebackup|progress_copy`.
   **No change needed** — the flag surface is unchanged by added columns.
3. `report/report.go:83-92` — `newApp()` takes `view.New()[config.ReportType]` (static, pre-19 shape).
4. `report/report.go:250-268` — on the first sample, and again whenever `prevMeta.version != d.meta.version`:
   ```go
   views := view.Views{config.ReportType: v}
   err := views.Configure(query.Options{Version: d.meta.version})
   v = views[config.ReportType]
   continue          // this sample becomes prev, nothing is printed yet
   ```
5. `report/report.go:305` → `countDiff(d.res, prevStat, itv, v)` → `stat.Compare(..., v.DiffIntvl, v.OrderKey, v.OrderDesc, v.UniqueKey)` (`report.go:453`).

### 5.2 Where the recorded version comes from

- Recorded: `internal/query/common.go:44` — `current_setting('server_version_num')::int AS version_num`
  is column **1** of the `meta.*` tar entry written per tick.
- Read back: `report/report.go:394-405` `readMeta()` parses `res.Values[0][1]` as the version, tolerating
  7- or 8-column meta records (`report.go:391-393`).

### 5.3 Answer to the roadmap's open question — replay of PRE-0.12 archives

**Verified against the code: replay stays correct. It does not depend on assumption.**

Three independent reasons:

1. **The layout is chosen by the archive's version, not the running server's.** `report/report.go:258-260`
   passes `d.meta.version` into `Configure`. A PG 14–18 archive therefore takes the pre-19 branch of the
   new selectors and gets exactly today's `(QueryTmpl, Ncols, DiffIntvl)`. This is the mechanism ADR
   `[008] Lift NotRecordable only` already relies on (`docs/decisions-log.md:429`: *"report-time
   `view.Configure(query.Options{Version})` already selects the version-correct layout; the rebuilt SQL
   string is never executed in report"*).

2. **Empirically confirmed on the existing fixture.** The meta entry of
   `report/testdata/pgcenter.stat.golden.tar` (`meta.20210614T115633.123.json`) carries
   `"version_num": "140000"`. Adding the three `Configure` cases makes replay call
   `SelectStatProgress*Query(140000)`, whose pre-19 branch returns the same values the static map holds
   today → `report/testdata/report_progress_{vacuum,analyze,basebackup}.golden` need **no regeneration**.
   A golden diff after implementation would be a red flag, not an expected churn.

3. **Even a stale `view.Ncols` is harmless in the report path.** Printing walks `res.Cols`
   (`report/report.go:564`), alignment walks `r.Ncols` (`internal/align/align.go:43,56` via
   `formatStatSample`, `report/report.go:482`), diffing uses `curr.Ncols`
   (`internal/stat/postgres.go:611,632`). `view.Ncols` is not read anywhere in `report/`. Ordering by
   name (`-o`) resolves through `getColumnIndex(d.res.Cols, …)` (`report/report.go:297`) and filtering
   (`-g`) matches `res.Cols` by name (`report/report.go:543-544`) — both version-agnostic.

The remaining sensitive field is `DiffIntvl`, and that is precisely what the selector fixes.
**Consequence for the tech-spec:** the version-aware selector + `Configure` case is not optional
polish — without it, a PG 19 recording replayed by a 0.12 binary would diff columns 10-11
(`scanned_total,%` / `vacuumed_total,%` on the PG 19 layout) instead of the KiB pair, silently
producing nonsense percentages. Add a `Test_app_doReport_ProgressVacuum` replay test with
`versionNum: "190000"` and `"180000"` variants, modelled on `report/report_record_bgwriter_test.go:31`.

### 5.4 Describe texts

`report/report.go:606-650` `describeReport()` maps report type → description const; the three relevant
entries are at `report.go:617`, `:620`, `:621`. The constants live in `report/describe.go`:

- `pgStatProgressVacuumDescription` — `report/describe.go:202-220`; the column table is
  `report/describe.go:204-217`. Insert `started_by` / `mode` rows between `datname` (`:207`) and
  `relation` (`:208`) so the doc order matches the emitted order.
- `pgStatProgressAnalyzeDescription` — `report/describe.go:266-283`; insert `started_by` after
  `datname` (`:271`).
- `pgStatProgressBasebackupDescription` — `report/describe.go:286-302`; insert `backup_type` after
  `pid` (`:289`).

These are single flat strings with no version branch — they will describe the **superset**. Note the
existing precedent: `pgStatBgwriterDescription` and `pgStatIODescription` already document
version-varying column sets in one text. Follow it; do not add version-aware describe plumbing.

`report/report_test.go:1184-1189` asserts `describeReport` returns the exact constants — it compares
identity, not content, so text edits do not break it.

### 5.5 Recording side

`record/record.go:65-148` `setup()` → `filterViews(props.VersionNum, …)` (`record.go:200-233`) →
`views.Configure(opts)` (`record.go:129`). The progress views are `NotRecordable: false` and pass the
version gate on PG 19, so the recorder picks up the wider result automatically — **no recorder change**
(the [008] ADR conclusion). `testing/e2e.sh:22-25` already exercises `-Pv -Pc -Pi -Pa -Pb -Pz`.

---

## 6. Shared Utilities

- `query.Format(tmpl string, o Options) (string, error)` — `internal/query/query.go:90-103`.
  `text/template` render of `{{.WalFunction1}}`-style placeholders. The progress queries contain no
  placeholders, so `Format` is an identity pass for them; it is still what the tests call.
- `query.NewOptions(version, recovery, track, querylen, pgssSchema) Options` — `internal/query/query.go:41-63`.
- `stat.Compare(curr, prev, itv, DiffIntvl, OrderKey, OrderDesc, UniqueKey)` — `internal/stat/postgres.go`
  (entry ~:580), delegating to `diff()` (`:605`) and `sort()` (`:663`).
- `align.SetAlign(r, truncLimit, dynamic) (map[int]int, []string)` — `internal/align/align.go:43,56`;
  drives both `top` (`top/stat.go:639`) and `report` (`report/report.go:482`) column widths from the
  **result's** `Ncols`.
- `top/stat.go:635-643` `alignViewToResult(config, r)` — realigns whenever
  `len(config.view.ColsWidth) != r.Ncols`. This is the guard that already protects the TUI against a
  column-count change mid-stream (issue #99); a PG 19 progress screen with 15 instead of 13 columns
  goes through it correctly.
- `postgres.NewTestConnectVersion(version)` — `internal/postgres/testing.go:16`; the sole test entry to
  a specific cluster.

---

## 7. Potential Problems

### 7.1 Version comparison audit — no `==` anywhere

Exhaustive grep of production code for version comparisons (`grep -rn "version <|version >=|version >|
Version ==|VersionNum ==" --include=*.go`, excluding `_test.go`) returns **zero `==` comparisons**.
Every site is an inequality and every "newest" branch is written `>=`, so 190000 lands in the newest
branch by construction:

| site | comparison |
|---|---|
| `internal/query/io.go:88` | `version >= PostgresV18` |
| `internal/query/bgwriter.go:42,46` | `>= 180000`, `>= 170000` |
| `internal/query/wal.go:26` | `>= 180000` |
| `internal/query/statements.go:335,337,348,357,359` | `< 130000`, `>= 170000`, `>= PostgresV17` |
| `internal/query/common.go:134,136,138,148,158` | `< PostgresV94/V96/V10/V13` |
| `internal/query/activity.go:48,50` | `< 90600`, `< 100000` |
| `internal/query/databases.go:47` | `< 120000` |
| `internal/query/replication.go:58` | `< 100000` |
| `internal/query/query.go:71` | `< PostgresV10` |
| `internal/stat/log.go:109` | `>= 100000` |
| `profile/profile.go:356` | `version < 130000` |
| `top/signal.go:86` | `VersionNum < 90600` |
| `internal/view/view.go:422` | `version >= v.MinRequiredVersion` |

This is the evidence behind the roadmap's "pgcenter does not break on PG 19" claim. It holds.

### 7.2 Wait-event type rename `BUFFERPIN` → `BUFFER` — no exposure

Grep for `bufferpin` / `BUFFER_PIN` (case-insensitive, all Go files): **zero hits**. Every wait-event
site concatenates whatever the server reports:
- `profile/profile.go:23,30` — `coalesce(wait_event_type ||'.'|| wait_event, '')`; `selectQuery()`
  (`profile.go:353-363`) branches only on `version < 130000` (leader_pid availability). The profile
  command aggregates `waitEntry` strings opaquely (`profile.go:93`).
- All six progress queries + `internal/query/procpidstat.go:29` — same concatenation.

The only hardcoded wait-event **type** literals are `'Lock'`:
`internal/query/common.go:63,74` (`count(*) FILTER (WHERE wait_event_type = 'Lock')` — the summary
panel's `waiting` counter) and `top/signal.go:77` (`groupWaiting: "wait_event_type = 'Lock'"` — the
cancel/terminate group filter). `Lock` is **not** among the PG 19 renames, so these are safe — but they
should be on the manual QA checklist since a silent zero there is invisible.

### 7.3 The `NewTestConnectVersion` silent fallback

`internal/postgres/testing.go:35-37`: an unmapped version falls back to the PG 14 cluster instead of
erroring. If `190000: 21919` is missed, every "PG 19" subtest connects to PG 14, passes, and proves
nothing. Verification step for the implementer: after adding the port entry, deliberately run one
PG 19 subtest against a stopped cluster and confirm it **skips** (rather than passes against PG 14).
Worth considering a follow-up hardening of that fallback into an error — but that is out of scope here
(it would change behaviour for the EOL versions the map deliberately keeps).

### 7.4 Active tech debt touching this area

From `docs/tech-debt.md` (Active Debt):
- **[006]** replslots `retained,KiB` standby path, **[010]** verbose recovery-`t` WAL standby path —
  both Low severity, both blocked on "the harness has no standby cluster". This feature adds a cluster
  to the harness. **Handling: do not fold the standby fixture in.** The roadmap assigns it to [018]
  (`docs/roadmap-0.12.0.md:264,268-271`) precisely so it is built once with `pg_stat_recovery`. Adding
  a PG 19 *primary* is orthogonal and must not be used as an excuse to grow this feature.
- **[016]** collectors swallow errors (Low) — untouched, no interaction.
- **[003]** self-reviews (Low) — process debt, no interaction.

**New debt this feature must register** (user asked for it explicitly, interview batch 3 Q1): the
`jammy-pgdg-testing` apt line in `testing/Dockerfile` is temporary. At the PG 19 GA rebuild it must be
removed and PG 19 installed from the main `jammy-pgdg` repo — otherwise the next image rebuild silently
pulls the **PG 20 beta** into the PG 19 slot.

### 7.5 ADRs that constrain the implementation (settled — do not re-open)

- `docs/decisions-log.md:191` **[004] Per-version column sets, not NULL-padded unified columns.**
  Forbids the "one query with `NULL AS started_by` for PG < 19" shortcut. Two consts per screen.
- `docs/decisions-log.md:383` **[006] Per-version query branch; time selector version-independent.**
  Establishes the `>= PostgresVNN` selector idiom this feature copies.
- `docs/decisions-log.md:397` **[007] 4-tuple selector when `UniqueKey` moves.** Here `UniqueKey`
  stays 0, so the 3-tuple is correct — but the ADR is the reason to check, not to assume.
- `docs/decisions-log.md:429` **[008] Lift NotRecordable only — report-time `Configure` picks the
  version-correct layout.** This is the ADR that answers §5.3.
- `docs/decisions-log.md:367` **[006] column hiding is not available** (`internal/align` floors width
  at 8). Consequence: the three new columns are *always* rendered on PG 19; there is no "hide on
  narrow terminal" option. Mitigated by [009]'s horizontal scroll — `started_by`/`mode` at idx 3-4 sit
  inside the initially visible window on a normal terminal, which is the whole point of head placement.

### 7.6 Smaller notes

- **`mode` is a column name already in use** — `internal/query/replication.go:6,19,34,44` aliases
  `sync_state AS mode`. Different view, so no conflict today; but [014] colorization is mandated to key
  off **column names, not indices** (`docs/roadmap-0.12.0.md:155`), so a future rule named `mode` would
  hit both screens. Worth one line in the tech-spec so [014] does not rediscover it.
- **`progress_vacuum` is a `RIGHT JOIN`** (`progress_vacuum.go:12`): rows matching `^vacuum` in
  `pg_stat_activity` with no `pg_stat_progress_vacuum` entry yield NULL `v.*`. `v.started_by`/`v.mode`
  will render blank for those rows. That is correct and needs no `coalesce` — they are **outside**
  `DiffIntvl`, so the [005]/[006] "NULL inside DiffIntvl aborts the sample" hazard does not apply.
  Do not reflexively wrap them in `coalesce(...,'')`; blank is the honest rendering (same policy the
  roadmap states for `backend_xid`/`backend_xmin` in [013], `roadmap-0.12.0.md:130-132`).
- **`delay_time`** exists on both PG 19 progress views and is *not* in scope. Expect it to be
  proposed during review; the answer is the roadmap's anti-rework boundary (`roadmap-0.12.0.md:86-96`).
- **`report/report.go:250`** re-runs `Configure` when the version changes mid-archive (append across an
  upgrade). Since `formatStatSample` sets `view.Aligned = true` on the first printed sample
  (`report.go:477-485`) and is never reset, a mid-archive column-count change would keep the **old**
  `ColsWidth`/`Cols`. This is a pre-existing latent issue, not introduced here, and requires a
  `record -a` across a major-version upgrade to trigger. Note it; do not fix it in this feature.

---

## 8. Constraints & Infrastructure

### 8.1 `testing/Dockerfile` (image `lesovsky/pgcenter-testing`)

Current: `FROM ubuntu:22.04`, `LABEL version="0.0.10"`, single pgdg apt line
`deb [signed-by=…] https://apt.postgresql.org/pub/repos/apt jammy-pgdg main`, installing
`postgresql-{14..18}` + matching `postgresql-plperl-*`. Final `CMD` echoes
`"pgcenter-testing 0.0.10: PostgreSQL 14-18 on Ubuntu 22.04"`.

Required changes:
1. Header comment `PostgreSQL 14-18` → `14-19`.
2. `LABEL version="0.0.10"` → `"0.0.11"`.
3. Add a second apt source line for the beta channel:
   `deb [signed-by=/usr/share/keyrings/pgdg.gpg] https://apt.postgresql.org/pub/repos/apt jammy-pgdg-testing main`
   (same signing key; a separate `.list` file keeps removal at GA a one-line delete — see §7.4 debt).
4. Add `postgresql-19 postgresql-plperl-19` to the install list.
5. Update the trailing `CMD` string.

**Probe first** (interview batch 1 Q1, accepted): build the image locally and confirm
`postgresql-19` resolves from `jammy-pgdg-testing` and the cluster starts on 21919 **before** any Go
code is written. Fallback if beta packages are not built for jammy: `ubuntu:24.04` / `noble-pgdg-testing`
— but the user explicitly scoped the noble bump **out** of this feature (batch 2 Q1); it would recreate
the environment for all of PG 14–18 right before a release.

Image publishing is manual (`docker build` + `docker push` by the maintainer); `Makefile:60-67`
`docker-build`/`docker-push` targets build the **pgcenter application** image, not the testing image,
and need no change.

### 8.2 `testing/prepare-test-environment.sh`

Iterates versions **explicitly, five times**, with the port derived as `219${v}`:
- `:6` create clusters — `for v in 14 15 16 17 18`
- `:13` configure (auto.conf + pg_hba) — same list
- `:43` start
- `:48` wait for readiness
- `:56` load `fixtures.sql`
- `:62` final `pg_isready`

Each of the six loops needs `19` appended. The `219${v}` convention makes port 21919 automatic — it
must match the new `internal/postgres/testing.go` map entry. The auto.conf block (`:19-32`) sets
`track_io_timing`, `track_functions=all`, `shared_preload_libraries='pg_stat_statements'`,
`wal_level=logical`; all valid on PG 19 — but `shared_preload_libraries` and `wal_level` are exactly
the settings whose acceptable values sometimes shift, so a startup failure of the 19 cluster should be
read as a GUC problem, not a packaging one.

### 8.3 `testing/fixtures.sql`

**Does not iterate versions** — it is a flat SQL script (193 lines) applied per cluster by
`prepare-test-environment.sh:56-59`. It creates the `pgcenter` schema (`plperlu` `get_proc_stats`
wrappers over `/proc`). No version literals. **No change required** — but it is the most likely thing
to break on PG 19 if `plperlu` semantics changed, so a failed fixtures load on 21919 is the second
signal to watch during the probe.

### 8.4 `testing/e2e.sh`

Two hardcoded port loops: `:15` (`pgcenter record`) and `:22` (`pgcenter report`), both
`for port in 21914 21915 21916 21917 21918`. Append `21919` to both. The inner `for arg in …` list
(`:23`) already covers `-Pv -Pc -Pi -Pa -Pb -Pz`, so the new columns get an end-to-end
record→report smoke on PG 19 for free.

### 8.5 GitHub Actions

There is **no PG version matrix** — the roadmap's phrase "add the version to the GitHub Actions matrix"
(`roadmap-0.12.0.md:83`) is imprecise. Versions come entirely from the container image. The only change
is the image tag:
- `.github/workflows/default.yml:8` — `container: lesovsky/pgcenter-testing:0.0.10` → `:0.0.11`
- `.github/workflows/release.yml:10` — same

Both workflows are otherwise identical in the `test` job (Go 1.25.11 pinned in three places each:
cache key, download URL, module cache key). Ordering matters operationally: the image must be **built
and pushed** before the tag bump lands on a branch that CI runs, or every push fails on image pull.

### 8.6 Makefile

No version-specific content. `test`, `lint`, `vuln`, `build`, `install` targets unchanged.
`make test` uses `-p 1` (serial packages) — relevant because the 6th cluster adds memory pressure to
the CI container (each cluster is `shared_buffers = 16MB`, so the increment is small).

### 8.7 Documentation touched by acceptance criteria

- `.claude/skills/project-knowledge/overview.md:34` — `Active support: PG 14, 15, 16, 17, 18.` → add 19.
  Also `:21,:22` say "PG 14–18" per screen.
- `.claude/skills/project-knowledge/deployment.md:36,38` — image contents "PostgreSQL 14–18" and the
  port list `PG14=21914 … PG18=21918`.
- `.claude/skills/project-knowledge/architecture.md:64-68,153,156` — the selector inventory (add the
  three new progress selectors) and the port map.
- `README.md:87` states support generically ("a wide range of PostgreSQL versions") with **no version
  list**, so the acceptance criterion "README lists PG 19" has nothing concrete to edit unless a
  version list is added. Flag this in the spec — either add a supported-versions line to README or drop
  it from the criteria.
- `doc/development.md:6` — a `docker run` example whose `-p` list stops at 21914. Cosmetic; already
  stale.

### 8.8 Dependencies

Go 1.25.11 (CI), Go 1.25+ (project). pgx/v5, gocui, cobra, testify — none is PG-version-aware in a way
that matters here; pgx wire protocol is stable across PG 19. No dependency bump is part of this feature.
Context7 was not consulted: the only "external library" involved is PostgreSQL itself, whose catalog was
verified directly against the version-19 documentation (§2.4).

---

## 9. Change Inventory (summary)

**Production code (7 files):**
1. `internal/query/query.go:22` — `PostgresV19 = 190000`
2. `internal/postgres/testing.go:19` — `190000: 21919`
3. `internal/query/progress_vacuum.go` — `PgStatProgressVacuumPG19` const + `SelectStatProgressVacuumQuery`
4. `internal/query/progress_analyze.go` — `PgStatProgressAnalyzePG19` const + `SelectStatProgressAnalyzeQuery`
5. `internal/query/progress_basebackup.go` — `PgStatProgressBasebackupPG19` const + `SelectStatProgressBasebackupQuery`
6. `internal/view/view.go:404` — three new `case` blocks in `Configure()` (static map entries unchanged)
7. `report/describe.go:204-217, 268-281, 288-299` — three describe tables gain rows

**Infrastructure (5 files):** `testing/Dockerfile`, `testing/prepare-test-environment.sh` (6 loops),
`testing/e2e.sh` (2 loops), `.github/workflows/default.yml:8`, `.github/workflows/release.yml:10`.

**Tests:** 33 `versions := []int{…}` sites (§4.1) + 6 per-version unit tables + 3 new selector unit
tables + `TestView_VersionOK` row + `Test_filterViews` row + `TestViews_Configure` 190000 block +
1 new report replay test (`Test_app_doReport_ProgressVacuum`, 180000/190000 variants).

**Docs:** `overview.md`, `deployment.md`, `architecture.md`, `features-catalog.md`, `tech-debt.md`
(new entry: remove `jammy-pgdg-testing` at GA).

**Not in scope** (roadmap anti-rework boundary, `roadmap-0.12.0.md:93-96`): WAL FPI column → [016];
`pg_stat_replication_slots` PG 19 columns → [018]; `stats_reset`/`stats_age` on
tables/indexes/functions → [017]; `delay_time` on the progress views; the `ubuntu:24.04` base bump.

---

# Updated: 2026-07-25 — implementation-planning deepening

Sources for this pass: the **approved** user-spec `{feature_base}.md` (which changed scope after §1–§9
above were written), the code cited inline, and `https://www.postgresql.org/docs/19/progress-reporting.html`
(re-fetched, not from memory). Everything below supersedes the earlier section it names.

## 0. Corrections to §2.5 — column placement changed with the approved spec

§2.5 was written from interview batch 1 ("head placement, variant B") and put the new columns
**before** `relation` / immediately after `pid`. The approved spec fixes a different order — the rule
stated at `{feature_base}.md:101` is *"сначала идентичность строки (`pid`, `datname`, `relation`),
затем происхождение и режим операции, затем состояние и метрики"*. **Use the spec's layouts:**

| screen | spec line | PG 19 layout | insert point | Ncols | DiffIntvl |
|---|---|---|---|---|---|
| `progress_vacuum` | `{feature_base}.md:86` | `0 pid · 1 xact_age · 2 datname · 3 relation · **4 started_by** · **5 mode** · 6 state · 7 waiting · 8 phase · 9 size_total,KiB · 10 scanned_total,% · 11 vacuumed_total,% · 12 scanned,KiB · 13 vacuumed,KiB · 14 query` | after `relation`, before `a.state` | 15 | `{12,13}` |
| `progress_analyze` | `{feature_base}.md:92` | `0 pid · 1 xact_age · 2 datname · 3 relation · **4 started_by** · 5 state · 6 waiting · 7 phase · 8 sample_size,KiB · 9 scanned,% · 10 ext_total/done · 11 child_total/done,% · 12 child_in_progress` | after `relation`, before `a.state` | 13 | `{0,0}` |
| `progress_basebackup` | `{feature_base}.md:98` | `0 pid · 1 started_from · 2 started_at · 3 duration · **4 backup_type** · 5 state · 6 waiting · 7 phase · 8 size_total,KiB · 9 streamed,% · 10 streamed,KiB · 11 tablespaces_total/streamed` | after `duration`, before `a.state` | 12 | `{10,10}` |

The `(Ncols, DiffIntvl)` triples in §3.2 are **unchanged** — the counts are the same because the same
number of columns is inserted ahead of the diffed block either way. Only the SQL column order and the
`describe.go` row positions move. `OrderKey=0`/`UniqueKey=0` conclusions in §2.5 still hold (`pid`
remains column 0 on all three).

Concretely, in each `SELECT` list the new columns go **immediately before `a.state`**:
- `internal/query/progress_vacuum.go:6` — `… v.relid::regclass AS relation, v.started_by, v.mode, a.state, …`
- `internal/query/progress_analyze.go:6-7` — `… p.relid::regclass AS relation, p.started_by, a.state, …`
- `internal/query/progress_basebackup.go:8` — `… AS duration, p.backup_type, a.state, …`

PG 19 catalog re-verified this pass: `pg_stat_progress_vacuum` has `mode` + `started_by` (and
`delay_time`, out of scope); `pg_stat_progress_analyze` has `started_by` (+ `delay_time`);
`pg_stat_progress_basebackup` has `backup_type`. Value domains match `{feature_base}.md:107-110` exactly.

---

## A. Blast radius of hardening `NewTestConnectVersion`

### A.1 The helper today

`internal/postgres/testing.go:13-15` — the doc comment **already promises the hardened behaviour**:

> `// Returns an error if the requested version is not available in the test environment.`
> `// Callers should use t.Skip() when this returns an error for EOL versions.`

…while `testing.go:34-37` does the opposite (`port = ports[140000]`). So this change makes the code
match its own contract; no doc-comment churn is needed beyond dropping "EOL versions" wording if desired.

### A.2 Every caller, and every version value reaching the ports map

Three entry points exist. `NewTestConfig()` (`testing.go:4-6`) does **not** go through the map (hardcoded
port 21917) — irrelevant. `NewTestConnect()` (`testing.go:9-11`) passes the literal `170000` — in the map.
That leaves `NewTestConnectVersion` direct callers: **41 call sites in 20 files**, every one of them a
`_test.go` file in `internal/query`, `internal/stat` or (via `NewTestConnect`) elsewhere. Full list from
`grep -rn "NewTestConnectVersion" --include=*.go`:

`internal/query/{activity,bgwriter,common(×4),databases(×2),functions,indexes,io(×2),overview(×4),pgcenter_schema,procpidstat,progress_analyze,progress_basebackup,progress_cluster,progress_copy,progress_create_index,progress_vacuum,replication,replication_slots(×3),sizes,statements(×5),tables,wal}_test.go`,
`internal/stat/postgres_test.go:93`.

Each of them passes a loop variable drawn from one of the literal lists inventoried in §B.1/§B.2 —
**there is no computed, arithmetic or derived version anywhere**. Grep for arithmetic on a version
variable in tests returns nothing; every list element is a written-out constant.

**Union of every value that can reach the map:**
`90500, 90600, 100000, 110000, 120000, 130000, 140000, 150000, 160000, 170000, 180000` (+ `190000` after
this feature). The map (`testing.go:17-32`) contains `90400, 90500, 90600, 100000, 110000, 120000, 130000,
140000, 150000, 160000, 170000, 180000`. **Every passed value is present; `90400` is mapped but never
passed.** Returning an error for an unmapped version therefore changes the behaviour of **zero existing
call sites**. Confirmed: no caller passes a version absent from the map.

### A.3 What callers do with the error — and the two shapes of the skip

The universal idiom (35 of 41 sites):

```go
conn, err := postgres.NewTestConnectVersion(version)
if err != nil {
    t.Skipf("postgres %d not available in test environment", version)
}
```

But **where** that idiom sits differs, and this matters for §G:

| shape | sites | consequence of one unavailable version |
|---|---|---|
| `t.Skipf` **inside a per-version `t.Run`** | activity, bgwriter, databases(×2), functions, indexes, io(×2), pgcenter_schema, procpidstat, all 6 progress_*, replication, replslots(×3), sizes, statements(×5), tables, wal | only that version's subtest is skipped — the others still assert |
| `t.Skipf` **outside/above the version loop** (whole test aborts) | `internal/query/common_test.go:86,102,114,128` (4 subtests, each looping all versions), `internal/query/overview_test.go:21,71,131,158` (4 tests), `internal/stat/postgres_test.go:95` (no `t.Run` at all) | the **entire** test/subtest is marked skipped at the first missing version |

Two pre-existing consequences worth naming (not introduced here, but they shape the merge plan):

- `internal/query/common_test.go:64` starts at `90500`, which is **never running in the CI image**
  (port 21995). So `Test_CommonQueries/common_queries`, `/activity_activity_queries` and
  `/activity_autovacuum_queries` already skip on their **first** iteration in CI today — they assert
  nothing on PG 14-18. Appending `190000` neither helps nor hurts them. Flag it; do not fix it here.
- `internal/query/overview_test.go` and `internal/stat/postgres_test.go:89` start at `140000`, so they
  **do** run today. Appending `190000` to `overviewVersions` / `postgres_test.go:90` makes them run
  14→18 and then skip on 19 when the PG 19 cluster is absent — assertions for 14-18 have already
  executed by then, but the test reports as SKIP. If the tech-spec wants clean per-version reporting,
  the minimal fix is to wrap those loop bodies in `t.Run(fmt.Sprintf("version/%d", version), …)`,
  matching the dominant shape. **This is the only structural test change §A implies**; it is optional
  for correctness and cheap.

### A.4 The error itself — style consistent with the file/package

`internal/postgres/testing.go` currently imports nothing. The package's single existing wrapped error is
`internal/postgres/postgres.go:90` — `fmt.Errorf("failed connection establishing: %w", err)`: lowercase,
no trailing punctuation, `fmt.Errorf`. There is no sentinel-error or `errors.New` precedent in the
package. Matching form:

```go
port, ok := ports[version]
if !ok {
    return nil, fmt.Errorf("postgres version %d has no test cluster port mapping", version)
}
```

This adds `"fmt"` as the file's only import. Two notes:
- Do **not** introduce an exported sentinel (`ErrUnmappedVersion`) — no caller inspects the error, all 41
  sites only branch on `err != nil`, and an exported symbol in a non-`_test.go` file widens the public
  surface of `internal/postgres` for no consumer.
- The acceptance criterion at `{feature_base}.md:142` ("закрыто тестом") wants a unit test in
  `internal/postgres`. There is no `testing_test.go` today; the natural home is a new
  `internal/postgres/testing_test.go` with a table of `{190001, 999999, 0}` asserting `assert.Error` and
  one mapped-but-not-running version asserting the error is a *connection* error, not a mapping error.
  This test needs no live PostgreSQL — the mapping check happens before `Connect`.

---

## B. The exact edit list for the `versions := []int{…}` sites

§4.1 said "33 sites in 21 files". Re-derived exhaustively this pass, the count is **29 live-connection
loop sites** that must gain `190000`, **4 sites that must not be touched**, and **8-10 per-version
assertion tables** that should gain a `190000` row. §4.1's table conflated the two kinds; the grouping
below replaces it.

### B.1 Group 1 — plain live-connection loops: append `190000`, nothing else (29 sites)

Each is a `[]int{…}` fed to `NewTestConnectVersion` inside (or around) a `t.Run`. The append is
mechanical; nothing else in these tests is version-shaped.

| # | file:line | current tail | note |
|---|---|---|---|
| 1 | `internal/query/activity_test.go:29` | …170000, 180000 | full 90500→ list |
| 2 | `internal/query/bgwriter_test.go:36` | …180000 | asserts `Len(FieldDescriptions()) == wantNcols` |
| 3 | `internal/query/common_test.go:64` | …180000 | drives 4 subtests incl. `versions[3:]` at :99 — slicing stays valid |
| 4 | `internal/query/databases_test.go:34` | …180000 | |
| 5 | `internal/query/functions_test.go:11` | …180000 | |
| 6 | `internal/query/indexes_test.go:11` | …180000 | |
| 7 | `internal/query/io_test.go:112` | `{160000,170000,180000}` | PG19 run also re-exercises the `>= PostgresV18` `object='wal'` assert at :138 — correct, it stays true |
| 8 | `internal/query/io_test.go:150` | `{160000,170000,180000}` | |
| 9 | `internal/query/overview_test.go:13` | `overviewVersions` | package-level var, feeds 4 tests (`:18,:68,:128,:155`) — one edit, four call sites |
| 10 | `internal/query/pgcenter_schema_test.go:21` | …180000 | the `plperlu` fixture gate; first thing to fail if PG 19 `plperlu` misbehaves |
| 11 | `internal/query/procpidstat_test.go:81` | …180000 | |
| 12 | `internal/query/progress_analyze_test.go:11` | …180000 | **must also switch from the bare const to the selector** — see B.4 |
| 13 | `internal/query/progress_basebackup_test.go:11` | …180000 | same |
| 14 | `internal/query/progress_cluster_test.go:11` | `{120000…180000}` | append only; see §E |
| 15 | `internal/query/progress_copy_test.go:11` | …180000 | append only |
| 16 | `internal/query/progress_create_index_test.go:11` | `{120000…180000}` | append only |
| 17 | `internal/query/progress_vacuum_test.go:11` | `{90600…180000}` | **must also switch to the selector** — see B.4 |
| 18-20 | `internal/query/replication_slots_test.go:34,113,161` | …180000 | ×3 |
| 21 | `internal/query/replication_test.go:39` | …180000 | |
| 22 | `internal/query/sizes_test.go:11` | …180000 | |
| 23 | `internal/query/statements_test.go:59` | full | |
| 24 | `internal/query/statements_test.go:110` | inline `{130000…180000}` | WAL section, PG13+ |
| 25 | `internal/query/statements_test.go:129` | inline `{150000…180000}` | JIT section, PG15+ |
| 26 | `internal/query/statements_test.go:176` | full | |
| 27 | `internal/query/tables_test.go:11` | …180000 | |
| 28 | `internal/query/wal_test.go:34` | …180000 | |
| 29 | `internal/stat/postgres_test.go:90` | …180000 | `Test_collectOverviewStat`; skip is above the loop (§A.3) |

### B.2 Group 2 — do **not** touch (4 sites)

| file:line | list | why |
|---|---|---|
| `internal/query/databases_test.go:56` | `[]int{140000}` | single-version smoke of `PgStatDatabaseSessionsDefault` (PG14+ shape, no version branch). Adding 190000 buys nothing and re-runs an identical query. |
| `internal/query/io_test.go:68` | `[]int{160000,180000}` | `Test_SelectStatIOQuery_NullSafety` — **string inspection, no connection.** Two elements = one per query branch (`<V18` / `>=V18`). 190000 hits the same `>=V18` branch as 180000 → duplicate assertion. |
| `internal/query/io_test.go:96` | `[]int{160000,180000}` | same, for the time selector. |
| `internal/query/io_test.go:179` | `[]int{160000,180000}` | `…_NoTemplateArtifacts` — same one-per-branch logic. |

(§4.1 listed the three `io_test.go` string loops among the "33 connection sites" — that was wrong; they
never open a connection.)

### B.3 Group 3 — per-version assertion tables needing a `190000` row

These are pure unit tables (no PG). A `190000` row proves the `>=` newest branch keeps returning the
PG 18 answer, i.e. that this feature did not accidentally move a boundary. Exact expected values:

| table | new row |
|---|---|
| `internal/query/bgwriter_test.go:16-22` | `{version: 190000, wantNcols: 14, wantDiffIntvl: [2]int{6, 12}}` |
| `internal/query/wal_test.go:16-20` | `{version: 190000, wantNcols: 7, wantDiffIntvl: [2]int{2, 5}}` |
| `internal/query/replication_slots_test.go:16-20` | `{version: 190000, wantNcols: 15, wantDiffIntvl: [2]int{6, 13}}` |
| `internal/query/io_test.go:18-24` | `{version: 190000, wantNcols: 16, wantDiffIntvl: [2]int{4, 14}}` |
| `internal/query/io_test.go:42-46` | `{version: 190000, wantNcols: 10, wantDiffIntvl: [2]int{4, 8}}` |
| `internal/query/statements_test.go:15-25` | `{version: 190000, want: PgStatStatementsTimingDefault}` |
| `internal/query/statements_test.go:42-46` | `{version: 190000, wantQuery: PgStatStatementsJITDefault, wantNcols: 15, wantDiff: [2]int{7, 12}, wantKey: 13}` |
| `internal/query/statements_test.go:154-164` | `{version: 190000, want: PgStatStatementsReportQueryDefault}` |

Optional (their tables deliberately stop at the last boundary and never listed 170000/180000 either —
adding 190000 is consistent but not required): `internal/query/databases_test.go:17-22`
(`{190000, PgStatDatabaseGeneralDefault, 19, [2]int{2,17}}`), `internal/query/activity_test.go:16-18`
(`{190000, PgStatActivityDefault, 14}`).

### B.4 Group 4 — three brand-new selector tables + a query-source switch

`internal/query/progress_{vacuum,analyze,basebackup}_test.go` today hardcode
`tmpl := PgStatProgressVacuumDefault` (`progress_vacuum_test.go:15`) etc. Once the selectors exist, line
15 of each becomes `tmpl, wantNcols, _ := SelectStatProgress…Query(version)` — otherwise the PG 19
subtest would execute the **PG 18** query against PG 19 and prove nothing about the new columns.
Recommended: also adopt the `bgwriter_test.go:52-60` shape (`conn.Query` + `assert.Len(rows.
FieldDescriptions(), wantNcols)`) instead of `conn.Exec`, so the live run gates the column count — that
is the assertion that catches a beta→GA catalog rename (`{feature_base}.md:159`).

New unit tables, modelled on `Test_SelectStatBgwriterQuery` (`internal/query/bgwriter_test.go:10-33`):

```go
{version: 180000, wantQuery: PgStatProgressVacuumDefault,     wantNcols: 13, wantDiffIntvl: [2]int{10, 11}},
{version: 190000, wantQuery: PgStatProgressVacuumPG19,        wantNcols: 15, wantDiffIntvl: [2]int{12, 13}},
{version: 180000, wantQuery: PgStatProgressAnalyzeDefault,    wantNcols: 12, wantDiffIntvl: [2]int{0, 0}},
{version: 190000, wantQuery: PgStatProgressAnalyzePG19,       wantNcols: 13, wantDiffIntvl: [2]int{0, 0}},
{version: 180000, wantQuery: PgStatProgressBasebackupDefault, wantNcols: 11, wantDiffIntvl: [2]int{9, 9}},
{version: 190000, wantQuery: PgStatProgressBasebackupPG19,    wantNcols: 12, wantDiffIntvl: [2]int{10, 10}},
```

Add a low row (e.g. `140000`, and `90600` for vacuum) so the table also pins the floor, matching how the
bgwriter table lists every supported version rather than just the boundary.

### B.5 View-layer rows (unchanged from §4.2, restated with verified line numbers)

- `internal/view/view_test.go:224-235` `TestView_VersionOK` — add `{version: 190000, total: 27}`.
  Highest existing row is `{160000, 27}`; no existing row changes.
- `record/record_test.go:132-137` `Test_filterViews` — add
  `{version: 190000, pgssSchema: "public", wantN: 3, wantV: 24}`. **Note the corrected values**: §4.2
  proposed `wantN: 0, wantV: 27`, which is wrong. The highest existing row is
  `{140000, "public", wantN: 3, wantV: 24}`; on ≥16 the three PG15/16-gated views (`stat_io`,
  `stat_io_time`, `statements_jit`) stop being dropped, so the ≥16 row is `wantN: 0, wantV: 27`. Since
  no ≥16 row exists today, **the implementer must compute this from `view.New()` rather than copy** —
  `filterViews` drops on `MinRequiredVersion` and the pgss gate only. Runs without PostgreSQL.
- `internal/view/view_test.go:99-222` `TestViews_Configure` — the matrix runs 140000 down to 90400 and
  its `switch tc.version` (`:177-216`) has **no case above 130000**. Add
  `{version: 190000, recovery: "f", trackCommit: "on", querylen: 256}` (+ the `"off"` twin if following
  the existing 8-row-per-version convention) and a `case 190000:` asserting
  `query.PgStatProgressVacuumPG19` / `Ncols 15`, `…AnalyzePG19` / `13`, `…BasebackupPG19` / `12`.
  The existing trailing loop `for _, v := range views { assert.NotEqual(t, "", v.Query) }` (`:218-220`)
  already proves the three new templates survive `query.Format` — no extra template test needed.

---

## C. `report/describe.go` — exact format and the version-varying precedent

### C.1 The table format

Every description is one raw-string const of the shape:

```
<one-line title ending in ':'>
<blank>
  column<TAB(s)>origin<TAB(s)>description
- <emitted column name><TABs><catalog column(s)><TABs><prose>
…
<blank>
Details: https://www.postgresql.org/docs/current/…
```

Columns are **tab-separated with hand-tuned tab counts** so they line up at 8-space tab stops. The
`origin` field names the underlying catalog column(s), comma-separated when a display column is derived
from several (`wait_event_type,wait_event`), and `-` when it is synthetic (`report/describe.go:163`,
`:426`, `:473`). Rows appear in **emitted order**, so a new row goes exactly where the column goes.

### C.2 Line ranges and insert points (per the corrected §0 placement)

| const | span | table span | insert |
|---|---|---|---|
| `pgStatProgressVacuumDescription` | `report/describe.go:202-220` | `:204-217` | two rows **after `relation` (`:208`), before `state` (`:209`)** |
| `pgStatProgressAnalyzeDescription` | `report/describe.go:266-283` | `:268-280` | one row **after `relation` (`:272`), before `state` (`:273`)** |
| `pgStatProgressBasebackupDescription` | `report/describe.go:286-302` | `:288-299` | one row **after `duration` (`:292`), before `state` (`:293`)** |

Note the analyze and basebackup tables use a **wider origin column** (three tabs after a short name;
see `:269` vs `:205`) — match the neighbours in the same const, not across consts.

### C.3 The precedent for version-only columns: a trailing `Note:` line, not a per-row marker

Two existing consts already document a superset, and both use the **same device**: the row table lists
the baseline columns, and a single `Note:` line before `Details:` states what newer versions add.

- `report/describe.go:439` (`pgStatBgwriterDescription`, header comment says "(PG14 baseline)" at `:422`):
  `Note: on PG17+ checkpoint/restartpoint counters come from pg_stat_checkpointer; PG18 adds slru_written.`
- `report/describe.go:490` (`pgStatIODescription`):
  `Note: on PG18 KiB throughput comes from read_bytes/write_bytes/extend_bytes; op_bytes was removed.`
- `report/describe.go:531` (`pgStatStatementsJITDescription`, "(PG15 baseline)" at `:513`):
  `Note: on PG17+ this section also includes the jit_deform_count/jit_deform_time columns.`

Note that in all three the version-only columns are **not** listed as rows at all — bgwriter's table has
no `slru_written` row, IO's has no `read_bytes` row. That is the strict precedent. But it exists because
those columns replaced or reshaped existing rows; here the three columns are pure additions the user
wants described (`{feature_base}.md:151`: *"`pgcenter report -d -P v|a|b` описывает новые колонки"*).

**Recommended reading of the precedent — list the rows AND add the Note:**

```
- started_by		started_by		Origin of the vacuum: manual, autovacuum or autovacuum_wraparound (PG19+)
- mode			mode			Vacuum mode: normal, aggressive or failsafe (PG19+)
…
Note: started_by and mode are available on PG19+ only; on earlier versions these columns are absent.
```

The `(PG19+)` suffix inside the row is a **new** micro-convention (no existing row carries one) — the
tech-spec should pick one of: rows-with-`(PG19+)`-suffix + Note, rows-without-suffix + Note (closest to
`pgStatBgwriterDescription`'s style, where the Note carries all the version information), or Note-only.
Do not leave it to the task author; three sibling consts must end up consistent.

Header comments: follow `:422` / `:513` and add `(PG14 baseline)`-style annotations to the three
progress consts' `//` lines so the reader knows the table is a baseline, not the PG 19 truth.

`report/report_test.go:1184-1189` compares `describeReport()` output **by identity against the const**,
so any text edit is invisible to it. There is no golden file for `-d` output. Confirmed: text edits here
cannot break a test.

---

## D. `testing/prepare-test-environment.sh` and `testing/e2e.sh`

### D.1 The six loops in `prepare-test-environment.sh` — all literally identical

`grep -c "for v in 14 15 16 17 18"` → **6**, at lines `6, 13, 43, 48, 56, 62`. Each becomes
`for v in 14 15 16 17 18 19; do`. Their purposes:

| line | loop | body |
|---|---|---|
| `:6` | create | `pg_lsclusters \| grep -q "^$v "` guard, then `pg_createcluster "$v" main` |
| `:13` | configure | appends the auto.conf block (`:18-33`), overwrites `/etc/postgresql/${v}/main/pg_hba.conf` (`:36-39`) |
| `:43` | start | `pg_ctlcluster "$v" main start` |
| `:48` | wait | `until pg_isready -h 127.0.0.1 -p "$port" -U postgres -t 5 -q; do … done` |
| `:56` | fixtures | `su - postgres -c "psql … -f /usr/local/testing/fixtures.sql"` |
| `:62` | final check | `pg_isready -t 10 … -d pgcenter_fixtures` |

The port is derived at `:14` and `:49,:57,:63` as `port="219${v}"` — so **21919 falls out automatically**
and matches the new `internal/postgres/testing.go` map entry by construction. No other version-dependent
logic exists in the script: no per-version `if`, no version-conditional GUC, no version in `fixtures.sql`
(flat 193-line SQL, applied identically to each cluster).

### D.2 What in the auto.conf block PG 19 could reject — and what it can't

The block is `prepare-test-environment.sh:19-32`:
`listen_addresses`, `port`, `shared_buffers=16MB`, `ssl=on` + snakeoil cert/key, `logging_collector=on`,
`log_directory`, `log_filename`, `track_io_timing=on`, `track_functions=all`,
`shared_preload_libraries='pg_stat_statements'`, `wal_level=logical`.

Checked against the PG 19 GUC set: **none of these is removed or renamed in PG 19**, and all keep their
current value domains (`wal_level` still accepts `logical`; `track_functions` still `all`). The two with
any historical volatility are `shared_preload_libraries` (fails hard at startup if the library is
missing) and `wal_level`. Practical reading during the probe:

- cluster fails to start **with a FATAL about `pg_stat_statements`** → the `postgresql-19` package did not
  ship contrib in the pgdg-testing build; a **packaging** problem, not a GUC one.
- cluster fails to start with `unrecognized configuration parameter` → a genuine GUC drift; fix in the
  script, this is the "not a probe failure" branch the spec calls out at `{feature_base}.md:165`.
- fixtures fail to load → `plperlu` semantics; also the `{feature_base}.md:165` branch. The canary test
  is `internal/query/pgcenter_schema_test.go` (B.1 row 10).

**Unmentioned risk worth adding to the probe checklist:** `pg_createcluster` and `pg_lsclusters` come
from `postgresql-common`, not from `postgresql-19`. Creating a version-19 cluster needs a
`postgresql-common` new enough to know the 19 layout. It is pulled from the pgdg **main** repo, not from
`pgdg-testing` — so a stale `postgresql-common` is a distinct, third failure mode of the probe, curable
by letting the beta repo supply `postgresql-common` too. Check `pg_lsclusters` output before blaming GUCs.

### D.3 `testing/e2e.sh` — two loops, and a hard-ordering trap

```bash
testing/e2e.sh:15:  for port in 21914 21915 21916 21917 21918; do   # pgcenter record
testing/e2e.sh:22:  for port in 21914 21915 21916 21917 21918; do   # pgcenter report
```

Both gain ` 21919`. The inner arg list at `:23` (`-A -R -D -T -I -S -F -Xm -Xg -Xi -Xt -Xl -Xw -Pv -Pc
-Pi -Pa -Pb -Pz`) already covers all six progress screens, so PG 19 gets a free record→report smoke over
the new columns.

**The trap:** `e2e.sh:7` is `set -euxo pipefail`. `pgcenter record` against a port with nothing listening
exits non-zero → the script aborts → **CI turns red**, it does not skip. See §G.

---

## E. REPACK / `pg_stat_progress_cluster` — no change required, verified column by column

`internal/query/progress_cluster.go:5-12` is a single const, no selector, `MinRequiredVersion:
query.PostgresV12` (`internal/view/view.go`), not in the `Configure()` switch. It reads exactly nine
columns from the view (aliased `p`):

`p.datname`, `p.relid`, `p.cluster_index_relid`, `p.phase`, `p.heap_blks_total`, `p.heap_blks_scanned`,
`p.heap_tuples_scanned`, `p.heap_tuples_written` — plus `p.pid` implicitly via the join predicate
(`progress_cluster.go:11`); everything else (`a.pid`, `xact_start`, `a.state`, `a.wait_event_*`,
`a.query`) comes from `pg_stat_activity`.

The PG 19 `pg_stat_progress_cluster` column list, re-fetched this pass, is:
`pid, datid, datname, relid, command, phase, cluster_index_relid, heap_tuples_scanned,
heap_tuples_written, heap_blks_total, heap_blks_scanned, index_rebuild_count`.

**All nine referenced columns are present.** Nothing removed, nothing renamed. `command` and
`index_rebuild_count` exist but pgcenter never selected them, so their presence is irrelevant.
**Conclusion: `progress_cluster.go` needs zero change on PG 19, and `progress_cluster_test.go:11` needs
only the mechanical `190000` append (B.1 row 14). No selector, no second const, no `Configure` case.**

PG 19 docs, verbatim: *"Whenever REPACK, CLUSTER or VACUUM FULL is running, the backwards-compatibility
`pg_stat_progress_cluster` view will contain a row for each backend that is currently running either
command… Because this view exists for backwards-compatibility purposes only, it will translate any
REPACK command into one of these other two."*

### What the manual verification step (`{feature_base}.md:218`, criterion `:145`) must look at

Because pgcenter does **not** select `command`, the translation is invisible on the screen. The thing to
verify is that a row appears at all and that its cells are populated:

1. Create a table big enough that `REPACK` takes >2s, open `pgcenter top` → `p` → cycle to the cluster
   progress screen.
2. Run `REPACK <table>;` in a second session.
3. Expect a row where: `relation` = the table, `phase` = one of the cluster phases
   (`seq scanning heap` / `index scanning heap` / `sorting tuples` / `writing new heap` / `swapping
   relation files` / `rebuilding index` / `performing final cleanup`), `size_total,KiB` non-zero, and
   `tuples_scanned` / `tuples_written` climbing between ticks.
4. `query` will show the literal `REPACK …` text (it comes from `pg_stat_activity.query`, untouched by
   the compat translation) — that is the visible proof the row is a REPACK, and the one thing to
   screenshot for the QA report.
5. Cross-check with `psql`: `SELECT command, phase FROM pg_stat_progress_cluster;` should report
   `command` as `CLUSTER` or `VACUUM FULL` (the translation) while `pg_stat_activity.query` says
   `REPACK`. That mismatch is expected, not a bug — note it in the QA report so it is not re-litigated.

Note also that `pg_stat_progress_repack` (the new native view) is explicitly out of scope
(`{feature_base}.md:170`) — it is a screen, not a column.

---

## F. The report replay test to add

Model: `report/report_record_bgwriter_test.go:31` `Test_app_doReport_Bgwriter` — a table of per-version
subcases, each building a synthetic in-memory tar and comparing against a per-version golden. No live
PostgreSQL. Companion example with a two-version table: `report/report_record_statio_test.go`.

### F.1 Anatomy of the harness (with the lines that matter)

| element | where | what to reproduce |
|---|---|---|
| testcase fields | `:32-43` | `name`, `versionNum`, `versionStr`, `cols []string`, `prevVals`, `currVals`, `wantFile` |
| meta record | `:141-151` | `stat.PGresult{Valid:true, Ncols:7, Nrows:1, Cols:[…7 SelectCommonProperties names…]}`; **only index 1 (`version_num`) is consumed** by `readMeta` (`report/report.go:394-405`) and it is what drives `views.Configure` at `report/report.go:250-262` |
| two ticks | `:166-180` | identical `Cols`, cumulative `Values`; tick 1 is swallowed by the `!prevStat.Valid` branch (`report.go:249-267`), tick 2 produces the printed row |
| tar layout | `:188-202` | six entries: `meta.<ts>.json`, `<reportType>.<ts>.json`, `sysinfo.<ts>.json` — twice, timestamps **exactly 1 s apart** so `itv == 1` and each diffed cell is a bare `curr - prev` |
| timestamp format | `:196-201` | `20060102T150405.000`, e.g. `progress_vacuum.20260519T100000.000.json` — **the entry basename must equal `config.ReportType`** |
| config | `:204-209` | `Config{ReportType: …, TruncLimit: 32, TsStart/TsEnd}` bracketing the tick timestamps |
| drive | `:211-216` | `app := newApp(config)`; `app.writer = &buf`; `app.doReport(tar.NewReader(&tarBuf))`. `newApp` (`report/report.go:82-91`) seeds `app.view` from the **static** `view.New()[ReportType]`; `processData` then overwrites it via `Configure(d.meta.version)` |
| sentinel asserts | `:220-228` | a `\d{4}/\d{2}/\d{2}` regexp for the header line + one `Contains` on a stable cell and one on a known delta — localises "row missing" vs "header only" before the golden diff |
| golden update | `:230-237` | `if *update { os.WriteFile(tc.wantFile, …); return }`; the flag is `var update = flag.Bool("update", …)` at **`report/report_test.go:22`** (package-level, shared) — regenerate with `go test ./report/ -run Test_app_doReport_ProgressVacuum -update` |

### F.2 What `Test_app_doReport_ProgressVacuum` needs concretely

Two subcases, `ReportType: "progress_vacuum"`, tar entry prefix `progress_vacuum.`:

**`pg18` — `versionNum: "180000"`, `versionStr: "18.0"`, 13 cols, DiffIntvl `{10,11}`**
```
cols: pid, xact_age, datname, relation, state, waiting, phase,
      "size_total,KiB", "scanned_total,%", "vacuumed_total,%",
      "scanned,KiB", "vacuumed,KiB", query
```
Absolute/text: 0-9 and 12 copied from `curr`. Diffed: 10, 11.

**`pg19` — `versionNum: "190000"`, `versionStr: "19.0"`, 15 cols, DiffIntvl `{12,13}`**
```
cols: pid, xact_age, datname, relation, started_by, mode, state, waiting, phase,
      "size_total,KiB", "scanned_total,%", "vacuumed_total,%",
      "scanned,KiB", "vacuumed,KiB", query
```
Absolute/text: 0-11 and 14. Diffed: 12, 13. `started_by: "autovacuum"`, `mode: "aggressive"` are good
fixture values (`{feature_base}.md:107-108`); pick the **same** `scanned,KiB` delta in both subcases
(e.g. prev `1000` → curr `1500`, delta `500`) so a single `assert.Contains(out, "500")` works as the
cross-version delta sentinel exactly as `:228` does for bgwriter.

`UniqueKey` defaults to 0 = `pid`, so keep `pid` identical between the two ticks or the rows will not
pair and both diffed cells come out as raw `curr` — this is the single most likely way to write a
green-but-meaningless test. Use one row with a fixed `pid`.

**The point of the `pg19` subcase** is precisely §5.3's hazard: without the version-aware selector,
`Configure(190000)` leaves `DiffIntvl` at `{10,11}`, which on the PG 19 layout points at
`scanned_total,%` / `vacuumed_total,%`. Those are `text` percentages — the diff would either print
nonsense or abort the sample. So write the test to **fail red first** against the un-selectored code.

### F.3 Golden files

Location: `report/testdata/` (flat, no subdirectories; 41 files today). Naming precedent is
`report_record_<screen>_<version>.golden`:
`report_record_bgwriter_pg14.golden`, `…_pg17.golden`, `…_pg18.golden`, `report_record_stat_io_v16.golden`,
`report_record_stat_io_v18.golden`, `report_record_replslots.golden`.

Use `report/testdata/report_record_progress_vacuum_pg18.golden` and `…_pg19.golden` (the `pgNN` form,
matching the bgwriter trio and the three-file majority). Goldens contain ANSI SGR escapes in the header
line (see `report/testdata/report_progress_vacuum.golden:1`) — always generate them with `-update`,
never hand-write.

### F.4 What must NOT change

`report/testdata/pgcenter.stat.golden.tar` — verified this pass: its `meta.20210614T115633.123.json`
carries `"version_num": "140000"` (and `version` `"14beta1 (Ubuntu 14~beta1-1.pgdg20.04+1)"`), 220 tar
entries. So the twenty-odd golden-tar cases in `report/report_test.go:35-160` — including
`progress_vacuum` (`:107-110`), `progress_analyze` (`:124-127`) and `progress_basebackup` (`:129-132`)
— replay through the **pre-19** branch of the new selectors and must produce byte-identical output.
`report/testdata/report_progress_vacuum.golden` still shows the 13-column header. **A diff in any of
these three goldens after implementation is a bug, not expected churn** (`{feature_base}.md:150`).

---

## G. Ordering / decoupling — the [005] precedent does **not** fully transfer

ADR `docs/decisions-log.md:333-347` ([005], accepted 2026-06-21) rejected hard ordering between the
manual image push and the code merge, because a *defensive `t.Skipf`* made the new capability
(`wal_level=logical`) optional at test time. Whether that transfers here depends on **which file** you
look at, and the answer differs per file.

### G.1 Where the tag actually lives

| file:line | content | pull-time effect |
|---|---|---|
| `.github/workflows/default.yml:9` | `container: lesovsky/pgcenter-testing:0.0.10` | **yes** — every push |
| `.github/workflows/release.yml:11` | `container: lesovsky/pgcenter-testing:0.0.10` | **yes** — push to `release` |
| `testing/Dockerfile:6` | `LABEL version="0.0.10"` | build-time metadata only |
| `testing/Dockerfile:38` | `CMD ["echo", "pgcenter-testing 0.0.10: PostgreSQL 14-18 on Ubuntu 22.04"]` | cosmetic |
| `testing/Dockerfile:2` | header comment `PostgreSQL 14-18` | cosmetic |
| `.claude/skills/project-knowledge/deployment.md:14,31,35` | prose | docs |
| `doc/development.md:5-6` | `lesovsky/pgcenter-testing:latest` + a `-p` list stopping at 21914 | docs, already stale |

Only the two `container:` lines can turn CI red on a missing image. Bumping them before the push fails
the job at "Initialize containers", **before any checkout or test**, i.e. a total red with no signal.

### G.2 The asymmetry CI's own step definitions create

`.github/workflows/default.yml:59-60`:
```yaml
      - name: Prepare test environment
        run: prepare-test-environment.sh          # ← bare name: resolved via PATH, i.e. /usr/local/bin
```
`.github/workflows/default.yml:80-81`:
```yaml
      - name: Run E2E tests
        run: ./testing/e2e.sh                     # ← path: resolved from the CHECKOUT
```

`testing/Dockerfile:35` bakes `prepare-test-environment.sh` into `/usr/local/bin/`. **So the repo copy of
`prepare-test-environment.sh` is inert in CI** — editing it has literally no effect until the image is
rebuilt from it and the tag is bumped. `e2e.sh` is the opposite: it is read from the working tree and
takes effect on the very next push.

### G.3 What happens under each merge shape

Scenario **"code merges, tag still `:0.0.10`"** (the [005]-style decoupling):

| change | behaviour on the old image | verdict |
|---|---|---|
| `PostgresV19` const, three selectors, `Configure` cases, `describe.go` rows | pure Go, no PG needed | green |
| `190000: 21919` in the port map | nothing listens on 21919 → `Connect` errors | fine — the map entry only *enables* the error |
| `190000` appended to the 29 connection loops | per-version `t.Run` sites: that subtest skips, others still assert. The 5 loop-level-skip sites (§A.3: `common_test.go` ×4 subtests, `overview_test.go` ×4 tests, `stat/postgres_test.go`) report the whole test as skipped after running 14-18 | **green, skips clean** |
| the hardened unmapped-version error | 190000 **is** mapped, so it returns a connection error exactly as before | green |
| the 8 assertion-table `190000` rows + 3 new selector tables + view/record rows | no PG | green |
| the new `Test_app_doReport_ProgressVacuum` | synthetic tar, no PG | green |
| `testing/prepare-test-environment.sh` `for v in … 19` | **inert** — CI runs the image's copy | green (and no PG 19 cluster) |
| **`testing/e2e.sh` ports gaining `21919`** | `set -euxo pipefail` + `pgcenter record -p 21919` against nothing → non-zero → **script aborts** | **RED** |

So the ADR-[005] decoupling holds for **everything except `e2e.sh`**. `e2e.sh` is the one file with a
hard ordering dependency, and the reason is structural, not incidental: e2e has no skip mechanism.

Scenario **"tag bumps before the image is pushed"**: both workflows fail at container init. Total red.

Scenario **"image pushed, nothing merged"**: harmless — the new cluster idles on 21919; PG 14-18 tests
are unaffected. This is exactly the *"прогон пересобранного образа на неизменённом `develop`"* step the
spec mandates (`{feature_base}.md:73`, `:167`, verification step 2 at `:212`).

### G.4 Safe merge order

1. **Probe + build the image locally** from the edited `testing/Dockerfile` + `testing/prepare-test-environment.sh` (the script edits are needed *for the build*, even though they are inert in CI until the tag moves).
2. **Push `lesovsky/pgcenter-testing:0.0.11`.**
3. **Run the new image against unmodified `develop`** (spec `:73`) — proves the rebuild carried no regression into PG 14-18.
4. **Merge the code**: `PostgresV19`, the three selectors + consts, the three `Configure` cases, `describe.go`, the port-map entry, the hardened error, all test edits, `testing/Dockerfile`, `testing/prepare-test-environment.sh`, **and the two `container:` tag bumps and `e2e.sh` in the same commit/PR.**

Step 4 can be a single merge precisely because step 2 already happened. **Do not split `e2e.sh` or the
tag bumps into an earlier commit** — either one alone turns CI red, and unlike the [005] case there is
no defensive skip that would rescue it.

If the team prefers the [005] two-phase shape anyway (code first, infra second), the only file that has
to move to phase 2 is `testing/e2e.sh`; everything else in the list skips cleanly on the old image. That
is the decision to record in the tech-spec — this ADR-[005] question has a different answer here than it
did for feature 005, and the difference deserves its own ADR line rather than a silent reuse.

### G.5 Secondary note

`doc/development.md:5-6` pulls `:latest`. If the maintainer also tags `0.0.11` as `latest`, developers
get PG 19 automatically but the documented `docker run -p …` list (which stops at 21914) stays wrong.
Cosmetic and pre-existing; mention it in the docs task or leave it.
