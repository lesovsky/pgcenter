# Code Research — Feature 007: pg_stat_statements JIT sub-screen

New 7th pg_stat_statements sub-screen (`statements_jit`, under the `X` menu, hotkey `x`
cycle) showing JIT compilation metrics per (user, database, queryid) row. PG15+ feature;
PG17 adds `jit_deform_count` / `jit_deform_time`. TUI-only, `NotRecordable`. Built by
analogy with the existing `statements_io` sub-screen.

Research date: 2026-06-21. Model: opus-4-8[1m].

---

## 1. pg_stat_statements JIT column schema (exact, by PG version)

Verified against official PostgreSQL docs:
- PG17: https://www.postgresql.org/docs/17/pgstatstatements.html
- PG15: https://www.postgresql.org/docs/15/pgstatstatements.html
- Context7 `/websites/postgresql_17` release notes (E.11.3.11.1): PG17 added the JIT
  `deform_counter`, plus `local_blk_read_time`/`local_blk_write_time`,
  `stats_since`/`minmax_stats_since`.

### PG15 / PG16 base set (8 columns)

| Column                   | SQL type           | Diffable? | Kind     |
|--------------------------|--------------------|-----------|----------|
| `jit_functions`          | `bigint`           | yes       | count    |
| `jit_generation_time`    | `double precision` | yes (ms)  | time, ms |
| `jit_inlining_count`     | `bigint`           | yes       | count    |
| `jit_inlining_time`      | `double precision` | yes (ms)  | time, ms |
| `jit_optimization_count` | `bigint`           | yes       | count    |
| `jit_optimization_time`  | `double precision` | yes (ms)  | time, ms |
| `jit_emission_count`     | `bigint`           | yes       | count    |
| `jit_emission_time`      | `double precision` | yes (ms)  | time, ms |

### PG17+ additions (2 columns)

| Column             | SQL type           | Diffable? | Kind     |
|--------------------|--------------------|-----------|----------|
| `jit_deform_count` | `bigint`           | yes       | count    |
| `jit_deform_time`  | `double precision` | yes (ms)  | time, ms |

`jit_deform_count` / `jit_deform_time` do NOT exist in PG15/PG16 — confirmed by the PG15
docs (Table F.20 lacks them). All counts are cumulative `bigint` (safe to diff); all times
are cumulative `double precision` milliseconds. The existing pgss timing queries round such
ms values with `round(p.<col>)` and present them as both a cumulative `text` interval and an
interval-diffable `,ms` column (see `PgStatStatementsTimingPG13`, statements.go:8).

There is no PG18 JIT column change relative to PG17 in the release notes consulted — PG17 and
PG18 share the same JIT column set. So a two-branch selector (PG15/16 base; PG17+ adds
deform) covers PG15-18.

---

## 2. Reference sub-screen pattern (`internal/query/statements.go`)

File: `/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/query/statements.go`

### Const naming convention

`PgStatStatements<Mode><VersionSuffix>` where suffix is `Default` (covers all / latest),
`PG13`, or `PG12`. Examples in file:
- `PgStatStatementsTimingPG12` (statements.go:21) — PG ≤12
- `PgStatStatementsTimingPG13` (statements.go:8) — PG 13–16
- `PgStatStatementsTimingDefault` (statements.go:34) — PG 17+
- `PgStatStatementsGeneralDefault`, `PgStatStatementsIoDefault`, `PgStatStatementsTempDefault`,
  `PgStatStatementsLocalDefault`, `PgStatStatementsWalDefault` — single-version modes.

Plan: add `PgStatStatementsJITDefault` (PG15/16 base set) and
`PgStatStatementsJITPG17` (adds deform columns), or invert (`...PG15` base + `...Default`
for PG17+). Match the timing precedent which uses `Default` for the newest shape:
`PgStatStatementsJITPG15` (base) + `PgStatStatementsJITDefault` (PG17+).

### SELECT shape (canonical pgss row layout)

Every pgss "diffable" mode query follows this exact column order (see `PgStatStatementsIoDefault`,
statements.go:56, the closest analog):

```
pg_get_userbyid(p.userid) AS user,          -- col 0  (UniqueKey component, displayed)
d.datname AS database,                       -- col 1
<N "total" columns>      AS *_total / "*_total,unit",   -- cumulative snapshot (shown, not diffed)
<N "interval" columns>   AS *      / "*,unit",          -- DIFFED columns (DiffIntvl range)
p.calls AS calls,                            -- col after interval block
left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid,  -- synthetic key
regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query   -- last col
FROM {{.PGSSSchema}}.pg_stat_statements p JOIN pg_database d ON d.oid=p.dbid
```

`statements_io` column count breakdown (Ncols=13): user(0), database(1), 4 totals(2-5),
4 intervals(6-9), calls(10), queryid(11), query(12). `DiffIntvl: [2]int{6,10}` (view.go:221)
— note the diff range upper bound is `calls` index (10), i.e. it spans the 4 interval
columns 6-9 plus `calls` at 10. (`statements_wal` uses `DiffIntvl {3,6}` with only 2 interval
cols + records/fpi + calls; layout differs per mode.)

### Templating tokens

- `{{.PGSSSchema}}` — schema where pg_stat_statements is installed (`p.ExtPGSSSchema`).
- `{{.PgSSQueryLenFn}}` — expands to a query-text expression honoring the configured query
  length (e.g. `left(p.query, N)` or `p.query`). Resolved by `query.Format(tmpl, opts)`.
- Both are resolved in `view.Views.Configure()` → `query.Format()` (view.go:395-402).

The `Default` (PG17+) templates use a raw backtick string with `'\s+'` (single backslash);
the older concatenated `+ "..."` style uses `E'\\s+'`. Either works — match neighbours.

### Version-branch selector functions

Two selectors live at the bottom of statements.go:
- `SelectStatStatementsTimingQuery(version int) string` (statements.go:306) — branches:
  `version < 130000` → PG12; `version >= 170000` → Default; else → PG13.
- `SelectQueryReportQuery(version int) string` (statements.go:318) — same 3-way branch.

Note: `statements_general`, `statements_io`, `statements_temp`, `statements_local`,
`statements_wal` have NO selector — they use a single `*Default` const wired directly as
`QueryTmpl` in view.go and are never re-selected in `Configure()`. Only `statements_timings`
goes through a selector (view.go:373-375).

### Plan for `SelectStatStatementsJITQuery`

```go
// SelectStatStatementsJITQuery returns proper statements_jit query depending on Postgres version.
func SelectStatStatementsJITQuery(version int) string {
    switch {
    case version >= 170000:
        return PgStatStatementsJITDefault   // base 8 + jit_deform_count/time
    default:
        return PgStatStatementsJITPG15      // base 8 columns (PG15/16)
    }
}
```

The view's `MinRequiredVersion: query.PostgresV15` gate (see §3) means this selector is only
ever called with version ≥ 150000, so the two-way branch is sufficient — no PG<15 case
needed. `PostgresV15 = 150000` is confirmed present (query.go:18); also `PostgresV16`,
`PostgresV17`, `PostgresV18` all exist (query.go:19-21).

Wire it in `view.Views.Configure()` next to the timings case (view.go:373):
```go
case "statements_jit":
    view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatStatementsJITQuery(opts.Version)
    v[k] = view
```
Because Ncols/DiffIntvl differ between PG15/16 (8 JIT metrics) and PG17+ (10 JIT metrics),
the selector should ALSO return Ncols and DiffIntvl (like `SelectStatWALQuery`,
`SelectStatIOQuery` which return `(string, int, [2]int)`), not just the string like the
timing selector. This is the cleanest fit — see §3 for the UniqueKey constraint that forces
two different column SETs rather than hiding columns.

---

## 3. View registration (`internal/view/view.go`)

File: `/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/view/view.go`

### `View` struct fields (view.go:9-31)

`Name, MinRequiredVersion, QueryTmpl, Query, DiffIntvl [2]int, Cols []string, Ncols int,
OrderKey int, OrderDesc bool, UniqueKey int, ColsWidth map[int]int, Aligned bool, Msg string,
Filters map[int]*regexp.Regexp, Refresh, ShowExtra, CollectExtra, IOAvailable,
DelayAcctAvailable, NotRecordable bool`.

### `statements_io` entry (view.go:218-229) — primary template to copy

```go
"statements_io": {
    Name:      "statements_io",
    QueryTmpl: query.PgStatStatementsIoDefault,
    DiffIntvl: [2]int{6, 10},
    Ncols:     13,
    OrderKey:  0,
    OrderDesc: true,
    UniqueKey: 11,
    ColsWidth: map[int]int{},
    Msg:       "Show statements IO statistics",
    Filters:   map[int]*regexp.Regexp{},
},
```

### `statements_wal` entry (view.go:254-266) — has MinRequiredVersion + NotRecordable precedent

```go
"statements_wal": {
    Name:               "statements_wal",
    MinRequiredVersion: query.PostgresV13,
    QueryTmpl:          query.PgStatStatementsWalDefault,
    DiffIntvl:          [2]int{3, 6},
    Ncols:              9,
    OrderKey:           0,
    OrderDesc:          true,
    UniqueKey:          7,
    ColsWidth:          map[int]int{},
    Msg:                "Show statements WAL statistics",
    Filters:            map[int]*regexp.Regexp{},
},
```

(`statements_wal` is NOT NotRecordable — it predates the TUI-first principle. Our new view
MUST set `NotRecordable: true`; the closest NotRecordable+MinRequiredVersion+UniqueKey
precedent is `stat_io` view.go:166-179.)

### Field meanings

- **`DiffIntvl [2]int`** — `[start, end]` inclusive column-index range that the diff engine
  treats as deltas (per-interval values). Picks the "interval" block of the SELECT (and
  often includes `calls`). `statements_io` `{6,10}` covers interval cols 6-9 + calls 10.
- **`UniqueKey int`** — index of the synthetic `md5(...) queryid` column used to match rows
  across diff snapshots. For pgss it is the `queryid` column (`statements_io` → 11). The
  `user`/`database` text columns are NOT the unique key — the md5 is, because the same query
  appears once per (user,db) and the md5 folds all three.
- **`Ncols`** — total columns returned; right border for `OrderKey` wrap (config_view.go:37).
- **`OrderKey` / `OrderDesc`** — initial sort column / descending. pgss screens use
  `OrderKey: 0` (user) DESC. (stat_io uses `OrderKey: 4`, the first diffed counter.)
- **`ColsWidth: map[int]int{}`**, **`Filters: map[int]*regexp.Regexp{}`** — initialized empty.
- **`Cols`** — populated at runtime from the result header (top/stat.go:314), not set here.

### Synthetic md5 queryid → columns cannot be hidden per version

The `UniqueKey` points at the md5 `queryid` column whose index is fixed by the SELECT column
count. The diff/align machinery enforces a minimum column width of 8 on the md5 (10-char
hash) and treats the layout positionally. Therefore you cannot hide a column for an older PG
version while keeping the same query — the indices (and hence UniqueKey/DiffIntvl/Ncols)
would shift. The established pattern is to ship a DIFFERENT query (different column SET) per
version and have the selector return matching `Ncols`/`DiffIntvl`/`UniqueKey`.

How other pgss screens differ across versions: only `statements_timings` differs across
versions (PG12 vs PG13 vs PG17+), and it does so by swapping the WHOLE query
(`SelectStatStatementsTimingQuery`) — NOT by hiding columns. Its column COUNT happens to stay
constant (13) across those variants, so the static `Ncols/DiffIntvl/UniqueKey` in view.go
(view.go:197-201) remain valid and `Configure()` only swaps `QueryTmpl`. For JIT the column
count DOES change (8 vs 10 JIT metrics → ~Ncols differs), so the selector must also return
`Ncols`/`DiffIntvl` (and UniqueKey must be recomputed). This is exactly the `stat_io` model
(`SelectStatIOQuery` returns `(string, int, [2]int)`, view.go:386) — follow it, and set the
view's static `UniqueKey: 0`-style placeholder while the Configure path patches the rest.

Recommended JIT layouts (proposal, with `user`=0, `database`=1):

PG15/16 (8 JIT metrics, counts+times). Suggested compact set: show the 4 count metrics +
4 time metrics as interval columns. Mirroring statements_io's total+interval doubling would
make 8 totals + 8 intervals = very wide; given no horizontal scroll (see §7), prefer a
single interval block (like the timings `,ms` columns) rather than total+interval doubling.
Final column set is a tech-spec decision — but keep UniqueKey aligned to the md5 column index
and DiffIntvl spanning the count/time block + calls.

---

## 4. The two count-tests that break when adding a view

### (a) `internal/view/view_test.go::TestNew` (view_test.go:9-12)

```go
func TestNew(t *testing.T) {
    v := New()
    assert.Equal(t, 26, len(v)) // 26 is the total number of views have to be returned
}
```
**Fix:** bump `26` → `27` and update the comment. Adding `statements_jit` to `view.New()`
raises the map size by one.

### (b) `record/record_test.go::Test_filterViews` (record_test.go:101-136)

```go
testcases := []struct {
    version    int
    pgssSchema string
    wantN      int   // filtered-out count
    wantV      int   // remaining count
}{
    {version: 140000, pgssSchema: "",       wantN: 10, wantV: 16},
    {version: 140000, pgssSchema: "public", wantN: 4,  wantV: 22},
    {version: 130000, pgssSchema: "public", wantN: 7,  wantV: 19},
    {version: 120000, pgssSchema: "public", wantN: 10, wantV: 16},
    {version: 110000, pgssSchema: "public", wantN: 12, wantV: 14},
    {version: 100000, pgssSchema: "public", wantN: 12, wantV: 14},
}
```
The new view is `MinRequiredVersion: query.PostgresV15` (150000) AND `NotRecordable: true`.
In `filterViews` (record.go:200-233) the `NotRecordable` branch (record.go:208) fires BEFORE
the version gate — so on EVERY test row the new view counts as filtered-out: `wantN += 1`,
`wantV` unchanged (it never joins the remaining set), regardless of version or pgssSchema.

**Fix — bump every row's `wantN` by exactly 1** (`wantV` stays the same on all 6 rows):

| version | pgssSchema | wantN old → new | wantV |
|---------|-----------|-----------------|-------|
| 140000  | ""        | 10 → 11         | 16    |
| 140000  | public    | 4 → 5           | 22    |
| 130000  | public    | 7 → 8           | 19    |
| 120000  | public    | 10 → 11         | 16    |
| 110000  | public    | 12 → 13         | 14    |
| 100000  | public    | 12 → 13         | 14    |

There is NO separate name-map that must gain an entry in `Test_filterViews`. The other
`record_test.go` tests (`TestFilterViews_NotRecordable`, `TestFilterViews_dropsExplicitNotRecordable`)
build their own ad-hoc `view.Views{}` literals and are NOT affected.

Note: `Test_filterViews` cases stop at version 140000 (no 150000 row), but because the
`NotRecordable` drop is version-independent, the +1 applies uniformly anyway. (If a 150000+
row were added later it would also be +1.)

---

## 5. Menu + cycle wiring

### (a) Uppercase menu `X` — `top/menu.go`

File: `/home/lesovsky/Git/github.com/lesovsky/pgcenter/top/menu.go`

`menuPgss` items list (`selectMenuStyle`, menu.go:53-60) currently has 6 entries (indices
0-5). **Add a 7th**, index 6:
```go
" pg_stat_statements WAL usage",
" pg_stat_statements JIT compilation",   // NEW index 6
```

`menuSelect` `case menuPgss` switch (menu.go:155-172) currently handles cy 0-5 + default.
**Add `case 6`** before `default`:
```go
case 5:
    viewSwitchHandler(app.config, "statements_wal")
case 6:
    viewSwitchHandler(app.config, "statements_jit")   // NEW
default:
    viewSwitchHandler(app.config, "statements_timings")
```

The `X` keybinding already opens this menu: `{"sysstat", 'X', menuOpen(menuPgss, app.config,
app.postgresProps.ExtPGSSSchema)}` (keybindings.go:45). The menu height auto-sizes to
`len(s.items)` (menu.go:115) — no manual height edit needed.

### (b) Lowercase cycle `x` — `top/config_view.go`

File: `/home/lesovsky/Git/github.com/lesovsky/pgcenter/top/config_view.go`

`statementsNextView` (config_view.go:160-180) currently cycles
`...wal → timings`. **Insert `statements_jit`** between wal and timings:
```go
case "statements_wal":
    next = "statements_jit"   // CHANGED (was statements_timings)
case "statements_jit":        // NEW
    next = "statements_timings"
default:
    next = "statements_timings"
```

The `x` keybinding: `{"sysstat", 'x', switchViewTo(app, "statements")}` (keybindings.go:40).
`switchViewTo` → `statementsNextView(app.config.view.Name)` (config_view.go:115). It already
guards `ExtPGSSSchema == ""` (config_view.go:105). No keybindings.go edit required for `x`.

### (c) Version-aware availability note

`statements_jit` has `MinRequiredVersion: query.PostgresV15`. On PG<15 the view is filtered
out of the recordable set, but in the live TUI the menu/cycle target it unconditionally. Check
how `viewSwitchHandler` (config_view.go:206-210) behaves when switching to a view absent from
`config.views` on PG<15 — confirm `view.New()` / the top-level view filter (top/top.go setup)
removes sub-PG15 views from `config.views`, otherwise switching to `statements_jit` on PG14
yields a zero-value `View{}`. The tech-spec must verify the TUI's view-availability filter
(separate from record's `filterViews`) drops `statements_jit` on PG<15 and that the menu/cycle
degrade gracefully (skip it). Search target: where `top` builds `config.views` and applies
`VersionOK`.

---

## 6. NotRecordable mechanism

- Field: `NotRecordable bool` on `view.View` (view.go:30): "When true,
  record/record.go:filterViews() skips this view."
- Enforcement: `record/record.go:filterViews()` (record.go:200), the `if v.NotRecordable`
  branch at record.go:208 — `delete(views, k); filtered++; continue`. Fires BEFORE the
  version gate (record.go:214) and the pgss-schema gate (record.go:221).
- Precedent setters (all `NotRecordable: true`):
  - `bgwriter` (view.go:151) — feature 004.
  - `replslots` (view.go:164) — feature 005.
  - `stat_io` (view.go:178), `stat_io_time` (view.go:192) — feature 006.
- `statements_*` views are normally recordable (none set the flag) — our new
  `statements_jit` MUST set `NotRecordable: true` per the 0.11.0 TUI-first principle
  (`docs/roadmap-0.11.0.md:86-92`, ADR `[004-feat-bgwriter-checkpointer]`
  `docs/decisions-log.md:223-233`).
- **report.go**: `doDescribe` (report/report.go:604) maps view-name → description text
  (report.go:607+; includes `statements_io`, `statements_wal`, etc.). NotRecordable views
  are not recorded, so there is no recorded data to report — `statements_jit` does NOT need a
  description entry here. Confirm by precedent: bgwriter/replslots/stat_io (all NotRecordable)
  have NO entry in this map. Skip the report.go description for the JIT view.

---

## 7. jit=off / zero-JIT row filtering

- **`statements_io` / `statements_wal` / all current pgss screens do NOT filter zero rows.**
  Their SELECTs end at `FROM {{.PGSSSchema}}.pg_stat_statements p JOIN pg_database d ON
  d.oid=p.dbid` with NO `WHERE`/`HAVING` (statements.go:67, 98). Every statement is shown.
- **`pg_stat_io` (feature 006) DOES filter all-zero rows** via a count-based `WHERE`:
  `... WHERE coalesce(reads,0)+coalesce(writes,0)+...+coalesce(fsyncs,0) > 0`
  (io.go:35, io.go:58, io.go:79). The comment (io.go:17-19) explains the count-based WHERE
  keeps the screen compact and makes the count and time sub-screens share an identical
  row-set. This was ADR-backed (decisions-log §[006...]).
- **Recommendation for JIT:** filter to rows with actual JIT activity. With `jit=on` (default)
  the vast majority of normalized statements never trigger JIT (only large-cost plans do), so
  WITHOUT a filter the screen would be dominated by all-zero rows. Add a count-based
  `WHERE jit_functions > 0` (or
  `WHERE coalesce(jit_functions,0)+coalesce(jit_inlining_count,0)+coalesce(jit_optimization_count,0)+coalesce(jit_emission_count,0) > 0`)
  following the `pg_stat_io` precedent. This also gracefully handles `jit=off` (all-zero →
  empty screen) — consider an optional cmdline hint "no JIT activity (jit=off?)" when empty,
  matching the interview's "optional jit=off hint" open question. Using only `jit_functions
  > 0` is simplest and sufficient: a statement with any JIT work always has
  `jit_functions > 0`. This is a tech-spec decision but the strong recommendation is: FILTER.

---

## 8. Exact files to touch (with current line anchors)

| File | Change | Anchor |
|------|--------|--------|
| `internal/query/statements.go` | Add `PgStatStatementsJITPG15` + `PgStatStatementsJITDefault` consts; add `SelectStatStatementsJITQuery(version) (string, int, [2]int)` selector | consts block ends ~statements.go:303; selectors at statements.go:305-327 |
| `internal/view/view.go` | Add `"statements_jit"` View entry (MinRequiredVersion PostgresV15, NotRecordable true, UniqueKey=md5 idx); add `case "statements_jit":` in `Configure()` | view entries view.go:194-266; Configure switch view.go:373-391 |
| `top/menu.go` | Add 7th `menuPgss` item (index 6); add `case 6` in `menuSelect`/`case menuPgss` | items menu.go:53-60; switch menu.go:156-171 |
| `top/config_view.go` | Insert `statements_jit` into `statementsNextView` cycle (wal→jit→timings) | config_view.go:160-180 |
| `top/keybindings.go` | NO CHANGE (`x` at keybindings.go:40 and `X` at keybindings.go:45 already wired) | — |
| `report/report.go` | NO CHANGE (NotRecordable → no description entry; matches bgwriter/replslots/stat_io) | doDescribe map report.go:604-624 |

### Test files to touch

| File | Change | Anchor |
|------|--------|--------|
| `internal/view/view_test.go` | Bump `TestNew` count `26 → 27`; optionally add `TestNew_StatementsJITView` guard (mirror `TestNew_StatIOView` view_test.go:17) | view_test.go:9-12 |
| `record/record_test.go` | Bump `Test_filterViews` `wantN` +1 on all 6 rows (table in §4) | record_test.go:123-128 |
| `internal/query/statements_test.go` | Add `TestSelectStatStatementsJITQuery` (mirror `TestSelectStatStatementsTimingQuery` statements_test.go:10) + add JIT exec sub-test loop over versions 150000-180000 (mirror `Test_StatStatementsQueries` statements_test.go:34, gated PG15+ like the WAL PG13+ loop at statements_test.go:86) | statements_test.go:10-103 |

### PG version constants (confirmed present, `internal/query/query.go`)

`PostgresV13=130000` (query.go:16), `PostgresV14=140000` (17), `PostgresV15=150000` (18),
`PostgresV16=160000` (19), `PostgresV17=170000` (20), `PostgresV18=180000` (21). Use
`query.PostgresV15` for the gate.

---

## 9. Integration points & runtime data flow

- `view.Views.Configure(opts query.Options)` (view.go:357) runs the per-view selector switch
  then `query.Format(view.QueryTmpl, opts)` for every view (view.go:395). `opts.Version`
  drives JIT branch; `opts.ExtPGSSSchema` / query-len feed `{{.PGSSSchema}}` /
  `{{.PgSSQueryLenFn}}`.
- `top/stat.go:alignViewToResult` (stat.go:303) populates `view.Cols` from the live result
  header (stat.go:314) and computes `ColsWidth` — JIT columns are auto-aligned; the md5
  `queryid` min width is enforced there.
- Diff/ordering uses `DiffIntvl`, `UniqueKey`, `OrderKey`, `Ncols` — all must be internally
  consistent with the returned column count (see §3).

---

## 10. Potential problems / constraints

1. **No horizontal column scroll.** Columns past terminal width are silently truncated
   (decisions-log §[006...] context, decisions-log:355). With 10 JIT metrics on PG17+ plus
   user/database/calls/queryid/query, a total+interval doubling (like statements_io) would be
   far too wide. Prefer a SINGLE interval block (counts + times), like the timings `,ms`
   columns. This is the main column-design constraint for the tech-spec.

2. **Ncols changes across versions (8 vs 10 JIT metrics).** Because of the synthetic md5
   `UniqueKey` (§3), the JIT selector MUST return `(string, Ncols, DiffIntvl)` and the
   Configure case must patch all three (follow `SelectStatIOQuery` model, view.go:386 —
   NOT the timings model which keeps Ncols constant). Getting Ncols/UniqueKey/DiffIntvl out
   of sync with the actual column count is the highest-risk bug; covered only by an exec test
   against a real PG (CI matrix PG15-18) — local runs without PG skip these (`t.Skipf`,
   statements_test.go:53).

3. **Count-test breakage masked locally.** `TestNew` and `Test_filterViews` (§4) are the two
   count assertions that will fail. They run without a PG instance, so `make test` catches
   them locally — but they are easy to forget. Both are pure integer bumps.

4. **PG<15 TUI view availability.** The record-side `filterViews` handles PG<15 via
   `MinRequiredVersion`, but the live TUI menu/cycle (§5c) must also drop `statements_jit` on
   PG<15. Verify the `top` view-availability filter (separate from record's `filterViews`)
   removes sub-PG15 views from `config.views`; otherwise selecting JIT on PG14 yields a
   zero-value `View{}`. This is the one item needing live verification in the tech-spec.

5. **jit=off / no-activity empty screen.** With the recommended `WHERE jit_functions > 0`
   filter, `jit=off` and low-activity systems show an empty screen. Decide whether to surface
   an explanatory cmdline hint (interview open question). Not a blocker.

### Tech debt / ADRs in scope

- `docs/tech-debt.md` Active Debt: no items touch statements.go / view.go / menu.go /
  config_view.go (grep found none) — no debt to flag.
- ADRs that constrain this feature (settled — do not re-litigate):
  - `[004-feat-bgwriter-checkpointer]` (decisions-log:223) — TUI-first / `NotRecordable: true`
    rationale. Apply directly: set `NotRecordable: true`, no record/report wiring.
  - `[006-feat-pg-stat-io]` synthetic-md5-key + count-based zero-row WHERE (decisions-log:355,
    373) — direct precedent for the JIT `WHERE jit_functions > 0` filter and the
    UniqueKey-on-md5 layout.

---

## Summary

- **JIT schema (confirmed vs PG official docs):** PG15/16 base = 8 columns —
  `jit_functions`(bigint), `jit_generation_time`(double), `jit_inlining_count`(bigint),
  `jit_inlining_time`(double), `jit_optimization_count`(bigint), `jit_optimization_time`(double),
  `jit_emission_count`(bigint), `jit_emission_time`(double). PG17+ adds 2 —
  `jit_deform_count`(bigint), `jit_deform_time`(double). All `*_count`/`jit_functions` are
  cumulative bigint counts (diffable); all `*_time` are cumulative double-precision ms
  (round + diff, like the timing screen). PG18 == PG17 column set.
- **Version-branch plan:** two query consts (PG15 base / PG17+ Default) + selector
  `SelectStatStatementsJITQuery(version) (string, int, [2]int)` returning query + Ncols +
  DiffIntvl (stat_io model, because Ncols differs); gate `MinRequiredVersion:
  query.PostgresV15`; two-way branch (≥170000 → Default, else base) is sufficient.
- **Two count-test fixes:** `view_test.go::TestNew` `26 → 27`; `record_test.go::Test_filterViews`
  `wantN +1` on all 6 rows (wantV unchanged), because NotRecordable drops it on every version.
- **Zero-row filtering:** RECOMMEND filtering — add `WHERE jit_functions > 0` (pg_stat_io
  precedent). Current statements_io/wal do NOT filter, but JIT rows are overwhelmingly all-zero
  under default `jit=on`, so an unfiltered JIT screen would be near-useless; filtering also
  cleanly handles `jit=off`.
