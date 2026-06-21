# Code Research: 006-feat-pg-stat-io

A new TUI screen for `pg_stat_io` (PG16+). Multi-row (one row per `backend_type × object × context`),
cumulative counters shown as per-interval rates, split into two sub-screens (count / time) to fit width.
Modeled on `replslots` (multi-row hybrid, version-aware selector) + `pg_stat_statements` (sub-screen menu).

This research is greenfield: `grep -rin "pg_stat_io"` across the repo (incl. `.claude/worktrees/`) returns
**zero** references to the PostgreSQL `pg_stat_io` view. All `read_bytes`/`write_bytes`/`writeback` hits are
unrelated Linux `/proc` parsing (`internal/stat/procpidstat.go`, `internal/stat/memstat.go`).

---

## 1. Reference Implementations to Model On

### 1.1 `replslots` — multi-row, hybrid, single version-independent query

`internal/query/replication_slots.go`
- `PgStatReplicationSlots` (const, lines 14–31): one big SELECT. The eight diffed counters are each wrapped in
  `coalesce(..., 0)` (lines 20–27) — **the load-bearing lesson**: a diffed column that yields NULL/empty string
  reaches `diffPair → strconv.ParseInt("")` and aborts the whole sample. Any column inside `DiffIntvl` must be
  non-NULL.
- `SelectStatReplicationSlotsQuery(_ int) (string, int, [2]int)` (lines 39–41): returns
  `(PgStatReplicationSlots, 15, [2]int{6,13})`. Version param is `_` because the chosen subset is schema-stable
  PG14–18. **This is the exact selector signature pg_stat_io's `io.go` must follow.**
- `stats_age` is the last column (14), **outside** the `[6,13]` diff range, so it renders absolute.
- Registered in `view.go` (lines 153–165): `OrderKey:4`, `OrderDesc:true`, `NotRecordable:true`,
  `MinRequiredVersion: query.PostgresV14`.

### 1.2 `bgwriter` + `wal` — single-row, version-aware, "variant A", stats_age outside DiffIntvl

`internal/query/bgwriter.go`
- Three version branches `PgStatBgwriterPG14` / `PG17` / `PG18` (lines 7–36).
- `SelectStatBgwriterQuery(version int) (string, int, [2]int)` (lines 41–52): branches on
  `version >= 180000` / `>= 170000` / else, **returning different `Ncols` and `DiffIntvl` per version**. This is
  the precedent for pg_stat_io PG16/17 (with `op_bytes`) vs PG18 (native `*_bytes` + WAL rows).
- Event counters sit **before** the diff block to render absolute (ADR [004]: "Absolute event counters via
  DiffIntvl placement"). For pg_stat_io, `stats_age` and the dimension columns play this role.

`internal/query/wal.go`
- Two branches `PgStatWALPG14` (PG14–17) / `PgStatWALDefault` (PG18). `SelectStatWALQuery` (lines 25–32) returns
  `(PgStatWALDefault, 7, [2]int{2,5})` on PG18, `(PgStatWALPG14, 11, [2]int{2,9})` otherwise. Cleanest minimal
  two-branch precedent.
- Note literal version ints `170000`/`180000` are used directly — **there are no `PostgresVxx` constants above
  `PostgresV14`** in `internal/query/query.go` (lines 9–18). pg_stat_io selector should use literal `160000` /
  `180000` to match house style.

### 1.3 Composite UniqueKey precedent — `statements_io` (THE pattern to copy)

CRITICAL FINDING for the composite-key question. pgcenter's `UniqueKey int` is a **single column index**
(`view.View.UniqueKey`, `view.go:20`). The diff (`internal/stat/postgres.go:diff()`) matches rows across samples
by comparing exactly one column:

```go
// internal/stat/postgres.go:322
if cv[ukey].String != pv[ukey].String { continue }
```

`pg_stat_statements` already solves the "identity is composite" problem the same way pg_stat_io needs: it builds a
**synthetic single column** from three identity fields. `internal/query/statements.go:65` (`PgStatStatementsIoDefault`):

```sql
left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid
```

and registers `UniqueKey: 11` (`view.go:197`) pointing at that synthetic column.

**=> pg_stat_io should emit a synthetic first column, e.g.**
`backend_type || '/' || object || '/' || context AS io_key` (or md5-hashed like pgss), and set `UniqueKey` to its
index. A plain concatenation is sufficient (the three dimensions are short, fixed-vocabulary text) and is more
readable in the unique-key role than an md5 hash; md5 is only needed in pgss because `query` text is huge. Either
works; concatenation is recommended.

---

## 2. View Registration & Wiring

### 2.1 `view.View` struct — `internal/view/view.go:10–31`
Fields a new view sets: `Name`, `MinRequiredVersion`, `QueryTmpl`, `DiffIntvl [2]int`, `Ncols`, `OrderKey`,
`OrderDesc`, `UniqueKey`, `ColsWidth: map[int]int{}`, `Msg`, `Filters: map[int]*regexp.Regexp{}`,
`NotRecordable bool`.

### 2.2 `view.New()` — `internal/view/view.go:37–323`
Static map of all views. **A NEW view must be added here per sub-screen.** Because pg_stat_io is split, register
**two** entries (mirroring the six `statements_*` entries, `view.go:166–238`):

- `"stat_io"` (count sub-screen) and `"stat_io_time"` (time sub-screen) — names illustrative.
- Both: `MinRequiredVersion: 160000` (no `PostgresV16` constant exists — use literal, matching bgwriter/wal
  selector style; note `MinRequiredVersion` in the map elsewhere uses `query.PostgresVxx`, so the cleanest path is
  to add `PostgresV16 = 160000` to `query.go:9–18` and reference it — see §8).
- Both: `UniqueKey:` = index of the synthetic `io_key` column, `OrderKey:` (default 0 = the key col, or pick a
  rate col), `NotRecordable: true`, `ColsWidth: map[int]int{}`, `Filters: map[int]*regexp.Regexp{}`.
- The two entries differ only in `QueryTmpl`, `Ncols`, `DiffIntvl`, `Msg`.

### 2.3 `Views.Configure(opts)` — `internal/view/view.go:328–370`
The version-aware selector is wired in the per-view `switch` (lines 334–356). Add cases:

```go
case "stat_io":
    view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatIOQuery(opts.Version)
    v[k] = view
case "stat_io_time":
    view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatIOTimeQuery(opts.Version)
    v[k] = view
```

After the switch, `Configure` runs `query.Format(view.QueryTmpl, opts)` for every view (lines 360–367) — the
pg_stat_io query can use template fields (e.g. `{{.Version}}`) but a plain non-templated string is fine (replslots
uses `{{.WalFunction1}}`, bgwriter/wal use none).

### 2.4 `opts.Version` source — `top/top.go:54–63`
`stat.GetPostgresProperties(app.db)` → `props.VersionNum` → `query.NewOptions(props.VersionNum, ...)` →
`app.config.views.Configure(opts)`. So **`VersionNum` is available at Configure() time**. It is also on
`app.postgresProps.VersionNum`, available in every keybinding handler (e.g. `top/keybindings.go:46` passes it to
`showPgLog`).

### Every place a new view must be registered (checklist, mirroring `replslots`)
1. `internal/query/io.go` — new file: query consts + `SelectStatIOQuery` / `SelectStatIOTimeQuery`.
2. `internal/view/view.go` `New()` — two map entries.
3. `internal/view/view.go` `Configure()` — two switch cases.
4. `top/keybindings.go` — `j` and `J` bindings (§3).
5. `top/menu.go` — new `menuStatIO` type + style + `menuSelect` cases (§3).
6. `top/config_view.go` — toggle helper for `j` (§3).
7. `top/help.go` — `helpTemplate` line for `j`/`J` (lines 13–18).
8. `internal/query/io_test.go` — selector + live tests (§7).
9. `internal/stat/help.go` — optional `PgStatIODescription` block (other views have one; not wired into the TUI
   help template, used for documentation).

`replslots` was registered in exactly these places (sans menu — it had no menu). The pgss sub-screens
(`statements_io` etc.) demonstrate the menu + multi-entry path.

---

## 3. Sub-screen Menu Pattern & `j`/`J` Keybindings

### 3.1 Menu machinery — `top/menu.go`
- `menuType` iota (lines 14–25): `menuNone, menuDatabases, menuPgss, menuProgress, menuConf`. **Add `menuStatIO`.**
- `selectMenuStyle(t menuType)` (lines 35–92): big switch returning `menuStyle{menuType, title, items}`. Add a
  `case menuStatIO` with **2 items** (`menuPgss` at lines 48–60 is the 6-item template):
  ```go
  case menuStatIO:
      s = menuStyle{menuType: menuStatIO,
          title: " Choose pg_stat_io mode (Enter to choose, Esc to exit): ",
          items: []string{" pg_stat_io operations (counts)", " pg_stat_io timings"}}
  ```
- `menuOpen(m, config, pgssSchema)` (lines 95–126): generic; the `pgssSchema == ""` guard (lines 100–103) is
  pgss-specific and won't fire for `menuStatIO`. No change needed beyond passing `menuStatIO`.
- `menuSelect(app)` (lines 129–208): switch on `app.config.menu.menuType`. **Add `case menuStatIO`** with two
  `cy` branches calling `viewSwitchHandler(app.config, "stat_io")` / `"stat_io_time")` (mirror `menuPgss`,
  lines 145–162). Follow with `printCmdline(app.ui, "%s", app.config.view.Msg)`.

### 3.2 Keybindings — `top/keybindings.go:18–75`
Existing lowercase-switch / uppercase-menu pairs:
- `'d'` → `switchViewTo(app,"databases")` (line 29) / `'D'` → `menuOpen(menuDatabases,...)` (line 43).
- `'x'` → `switchViewTo(app,"statements")` (line 40) / `'X'` → `menuOpen(menuPgss,...)` (line 44).
- `'p'`/`'P'` similarly (lines 38, 45).

**`j` and `J` are both free** (verified — no binding uses them). Add:
```go
{"sysstat", 'j', switchViewTo(app, "statio")},          // lowercase = enter + toggle count<->time
{"sysstat", 'J', menuOpen(menuStatIO, app.config, "")}, // uppercase = open 2-item menu
```
The `"menu"` view bindings (Esc/Up/Down/Enter, lines 67–70) are generic and already drive any menu including
`menuStatIO` — no change.

### 3.3 Lowercase TOGGLE feasibility (the `j` cycle question)
`switchViewTo(app, c)` (`top/config_view.go:102–125`) handles "family switch" keys by mapping `c` to a
`*NextView(current)` helper:
- `"databases"` → `databasesNextView` (lines 127–140): 2-way toggle general↔sessions.
- `"statements"` → `statementsNextView` (lines 142–163): 6-way cycle.
- `"progress"` → `progressNextView` (lines 165–186).
- `default:` → `viewSwitchHandler(app.config, c)` switches directly to view named `c`.

**A lowercase 2-way toggle is directly feasible**: add a `statioNextView(current string) string` helper (2-way,
copy `databasesNextView`):
```go
func statioNextView(current string) string {
    switch current {
    case "stat_io":      return "stat_io_time"
    case "stat_io_time": return "stat_io"
    default:             return "stat_io"   // first entry from any other view
    }
}
```
and a `case "statio": viewSwitchHandler(app.config, statioNextView(app.config.view.Name))` in `switchViewTo`
(lines 111–120). On first press from an unrelated view, `default` returns the count sub-screen (enter). On
subsequent presses while already on a sub-screen, it toggles. This exactly satisfies "j enters the view and
toggles count↔time".

`viewSwitchHandler(config, c)` (`top/config_view.go:189–193`) is the low-level switch: saves current view back to
`config.views`, loads `config.views[c]`, sends on `config.viewCh`. It reloads the view **from the static map**, so
it must NOT be used when runtime patches matter (cf. `switchViewToProcPidStat` warning, lines 196–200) — pg_stat_io
has no runtime patches, so plain `viewSwitchHandler` is correct.

---

## 4. `pg_stat_io` Schema Across Versions (VERIFIED against postgresql.org)

Sources: docs/{16,17,18}/monitoring-stats.html + release/18.0. Confidence: high. One spot-check flagged below.

### PG16 & PG17 — identical, 18 columns
`backend_type` (text), `object` (text), `context` (text), `reads`, `read_time`, `writes`, `write_time`,
`writebacks`, `writeback_time`, `extends`, `extend_time`, `op_bytes`, `hits`, `evictions`, `reuses`, `fsyncs`,
`fsync_time`, `stats_reset` (timestamptz).
- **`op_bytes` EXISTS in 16 AND 17** (verified). It is a **constant block size (= BLCKSZ, default 8192), NOT a
  counter** — must NOT be diffed.
- PG17 view column set is **unchanged vs PG16** (PG17's IO additions were internal: more `backend_type` rows +
  `pg_stat_get_backend_io()`; no new view columns).

### PG18 — 20 columns; `op_bytes` REMOVED, 3 native `*_bytes` ADDED, WAL rows ADDED
`backend_type`, `object`, `context`, `reads`, **`read_bytes`**, `read_time`, `writes`, **`write_bytes`**,
`write_time`, `writebacks`, `writeback_time`, `extends`, **`extend_bytes`**, `extend_time`, `hits`, `evictions`,
`reuses`, `fsyncs`, `fsync_time`, `stats_reset`.
- `op_bytes` **removed** (release-note: "always equaled BLCKSZ, has been removed" — ops can now vary in size via
  `io_combine_limit`).
- New `read_bytes` / `write_bytes` / `extend_bytes` are **cumulative counters** of type **`numeric`** (NOT bigint).
- New `object = 'wal'` rows; new `context = 'init'` value (WAL segment creation). No new *time* columns — WAL rows
  reuse existing `read_time`/`write_time`/`extend_time`/`fsync_time`, gated by a **separate GUC
  `track_wal_io_timing`** (distinct from `track_io_timing`, not merged in PG18).
- **SPOT-CHECK before coding scan types:** docs say `read_bytes/write_bytes/extend_bytes` are `numeric`. pgcenter
  scans everything as `sql.NullString` (`postgres.go:193`) so Go typing is moot, but `numeric` text may render
  with a decimal point — `diffPair` treats values containing `.` as floats (`postgres.go:446`), which is fine.
  Confirm `\d pg_stat_io` on live PG18 in CI.

### Cumulative vs absolute (all versions)
- **Counters (diffable):** reads, read_time, writes, write_time, writebacks, writeback_time, extends, extend_time,
  hits, evictions, reuses, fsyncs, fsync_time; **PG18 also** read_bytes, write_bytes, extend_bytes.
- **Constant/dimension (NOT diffable):** backend_type, object, context, **op_bytes** (PG16/17 only).
- **Absolute/metadata:** stats_reset (→ `stats_age = now() - stats_reset`, last column, outside DiffIntvl).

### Dimension allowed values
- `object`: PG16/17 `relation`, `temp relation`; PG18 adds `wal`.
- `context`: PG16/17 `normal`, `vacuum`, `bulkread`, `bulkwrite`; PG18 adds `init`.
- `backend_type`: open text (same set as `pg_stat_activity`: `client backend`, `autovacuum worker`,
  `background writer`, `checkpointer`, `walwriter`, …), grows version to version.

### Hybrid bytes throughput (D3 in interview)
- PG16/17: bytes = `<count> * op_bytes` (e.g. `reads * op_bytes`), computed in SQL → KiB, as a derived diffable
  counter. Because `op_bytes` is constant, `Δ(reads*op_bytes)/itv == (Δreads/itv)*op_bytes` — diffing the product
  is correct.
- PG18: use native `read_bytes`/`write_bytes`/`extend_bytes` directly → KiB, diffable.
- Keep identical headers across versions so the column shape stays uniform (the ADR-[004] "shared columns keep
  identical headers" principle). Note PG18 has **no `writeback_bytes`/`hit_bytes`/etc.** — only read/write/extend
  have native byte counters; pre-18 only those three can be derived too (writebacks/hits/evictions/reuses/fsyncs
  are block-count operations whose byte size you'd still multiply by op_bytes — but PG18 dropped that, so for a
  uniform set keep bytes throughput limited to read/write/extend on both branches).

---

## 5. Version Gating / "Not Available" UX

**There is NO UI-level version guard at switch time.** `switchViewTo` / keybinding handlers do not check
`VersionOK` (verified: `grep VersionOK top/*.go` → none). The single gate is in the collector:

`internal/stat/stat.go:198–201`
```go
if !view.VersionOK(c.config.VersionNum) {
    return s, fmt.Errorf("selected statistics is not supported by current version of Postgres")
}
```
`view.VersionOK(version)` = `version >= v.MinRequiredVersion` (`view.go:373–375`). The returned error flows:
`Update → stats.Error` (`top/stat.go:59–61`) → `printDbstat` checks `s.Error != nil` and renders
`formatError(s.Error)` into the main pane (`top/stat.go:322–328`). `formatError` (lines 349–361) wraps a non-Pg
error as `"ERROR: <msg>"`.

**=> For PG14/15, set `MinRequiredVersion: 160000` (or `query.PostgresV16`) on both stat_io views.** Pressing `j`
on PG14/15 switches the view, the collector returns the not-supported error, and the user sees
`ERROR: selected statistics is not supported by current version of Postgres` in the main pane. This is the SAME UX
`replslots` (PG14+), `wal`/`bgwriter` (PG14+), and the progress views already rely on — no new mechanism needed.
The interview's "clear not supported message" requirement is met by this existing path (the generic message is
acceptable; a custom message would require a UI-level guard pgcenter currently lacks).

`VersionNum` is available in handlers via `app.postgresProps.VersionNum` if a nicer custom message is later wanted
(could add a guard in `switchViewTo`/`menuOpen` like the pgss `ExtPGSSSchema == ""` guard at
`config_view.go:105–108` / `menu.go:100–103`). Not required for v1.

---

## 6. Hidden / Zero Rows

### 6.1 Existing row-hide mechanisms
- `,` `toggleSysTables` (`top/config_view.go:250–287`): only for `tables`/`indexes`/`sizes`; toggles
  `queryOptions.ViewType` user↔all, which re-formats those three queries (SQL-side WHERE via template). **Not
  reusable** for pg_stat_io (hardcoded to three view names, line 253).
- `Filters map[int]*regexp.Regexp` (`view.go:24`) set via `/` (`dialogFilter`): client-side regex filter per
  column, applied at print time (`isFilterRequired`, `printStatData`, `top/stat.go:341`). User-driven, not for
  always-hiding all-NULL rows.
- There is **no generic "hide row where columns all zero" mechanism** in `internal/stat`. `diff()` and `sort()`
  never drop rows by value.

### 6.2 Recommended: SQL-side WHERE (preferred)
Hide rows where all count counters are NULL/0 **in the query** with a `WHERE` clause:
```sql
WHERE coalesce(reads,0)+coalesce(writes,0)+coalesce(writebacks,0)+coalesce(extends,0)
    + coalesce(hits,0)+coalesce(evictions,0)+coalesce(reuses,0)+coalesce(fsyncs,0) > 0
```
This is preferable to client-side filtering because (a) pgcenter has no client-side "drop empty rows" hook, (b) it
keeps the row-set identical on both sub-screens (the time screen reuses the same WHERE on the same count counters —
interview D5: "same row-set on both, do NOT hide for zero timings"), and (c) it reduces diff work. This mirrors how
pgcenter already pushes filtering to SQL (`toggleSysTables` re-formats queries; `pg_stat_activity` ShowNoIdle).

**Interaction with `coalesce` lesson:** every counter inside `DiffIntvl` must still be `coalesce(...,0)` because
`pg_stat_io` legitimately returns NULL for impossible ops (e.g. `fsyncs` NULL for `temp relation`, `reads` NULL for
`background writer`). A NULL inside the diff range → `diffPair → ParseInt("")` → sample abort (ADR [005],
`replication_slots.go:9–11`). So: `coalesce(reads,0) AS reads`, etc., for all diffed columns; the WHERE then
filters truly-empty rows.

---

## 7. Tests

### 7.1 Selector unit-test pattern (`internal/query/io_test.go`, new)
Copy `bgwriter_test.go:11–33` / `replication_slots_test.go:10–30`. Table-driven over
`version ∈ {140000,150000,160000,170000,180000}` asserting `(Ncols, DiffIntvl)` per branch. For PG14/15 the
selector still returns a query (the gate is `MinRequiredVersion` at the view layer, not the selector), so the test
asserts the PG16-shaped or a degenerate query — decide whether `SelectStatIOQuery` returns the PG16 query for
<16 or an empty string; **recommendation: return the PG16 query for all `<180000` and let `MinRequiredVersion`
gate execution** (the selector is never called for an unsupported view because the collector errors first; but
Configure() formats every view's template regardless, so the template must be Format-safe on PG14/15 — a plain
non-erroring SELECT is fine, it just never runs).

### 7.2 Live integration-test pattern (`internal/query/io_test.go`)
Copy `bgwriter_test.go:37–63`:
```go
versions := []int{140000,150000,160000,170000,180000}
... conn, err := postgres.NewTestConnectVersion(version)
if err != nil { t.Skipf("postgres %d not available", version) }
rows, _ := conn.Query(q)
assert.Len(t, rows.FieldDescriptions(), wantNcols)  // schema-divergence gate
```
For PG14/15 the live test should **skip the Ncols assertion** (pg_stat_io doesn't exist) or only run for `>=16`.
The PG18 run is the real gate that the native `*_bytes` + `object='wal'` shape exists (the one thing the local
PG17-only env can't verify — same role `slru_written` plays in `bgwriter_test.go:55–57`).

### 7.3 Test infra helpers
- `internal/postgres/testing.go`: `NewTestConnectVersion(version)` (lines 16–44) maps version→port
  (180000→21918, 170000→21917, 160000→21916, 150000→21915, 140000→21914). Returns error for unavailable versions →
  caller `t.Skipf`s. `NewTestConfig()` / `NewTestConnect()` default to PG17.
- `query.NewOptions(version, "f", "off", 256, "public")` + `query.Format(tmpl, opts)` to build the live query
  (replication_slots_test.go:40–41).
- Locally only PG17 is up; CI runs the full PG14–18 matrix (interview testing_strategy; tech-debt [005] notes
  `Test_doReload` panics instead of skipping when fixtures are down — unrelated, but be aware `make test` may fail
  locally for reasons outside this feature).

### 7.4 Menu test (`top/menu_test.go`)
`Test_selectMenuStyle` (line 8) asserts item counts: `{menuDatabases:2}`, `{menuPgss:6}`. **Add `{menuStatIO:2}`.**

---

## 8. Potential Problems & Constraints

1. **Composite-key row matching (resolved pattern, but must be implemented).** `UniqueKey` is a single column
   index; pg_stat_io identity is 3 columns. Solution: synthetic combined first column
   (`backend_type||'/'||object||'/'||context AS io_key`), `UniqueKey` = its index. Precedent: `statements_io`
   `queryid` (md5 of 3 fields), `view.go:197 UniqueKey:11`. **If this column is omitted, diff matches all rows by
   `backend_type` alone (col 0) and produces wrong deltas** — silent correctness bug.

2. **NULL inside DiffIntvl aborts the sample (ADR [005]).** pg_stat_io returns NULL for impossible ops
   (`fsyncs` for `temp relation`, `reads` for `background writer`, all time columns can be 0 not NULL, but byte/op
   counters can be NULL). **Every diffed column MUST be `coalesce(...,0)`** or one NULL cell blanks the entire
   screen via `diffPair → ParseInt("")` (`internal/stat/postgres.go:336, 446, 477`). This is the single most
   likely bug; it bit replslots (physical-slot NULLs) and is now an established ADR.

3. **`numeric` `*_bytes` in PG18 render with decimals.** `diffPair` routes values containing `.`/`e` to
   `parsePairFloat` (`postgres.go:446–453`) — correct, but the delta prints with 2 decimals
   (`FormatFloat(...,'f',2,64)`). Converting bytes→KiB via integer `/1024::bigint` in SQL (as replslots does,
   `replication_slots.go:18`) keeps them integer-typed and avoids float formatting. Recommended.

4. **op_bytes multiplication for bytes throughput (PG16/17).** `reads * op_bytes` is safe to diff because
   `op_bytes` is constant per row (Δ(a·k)=k·Δa). But `op_bytes` can be NULL on some rows in principle → wrap:
   `coalesce(reads,0)*coalesce(op_bytes,0)`. Keep the derived bytes columns inside DiffIntvl with the raw counters.

5. **Width budget (the reason for the split).** pgcenter has no horizontal scroll; columns past the terminal width
   are cut (`Fprint`/`printStatData` just keep writing). Count screen base (4) + 8 ops + 3 bytes-throughput = 15
   columns; backend_type values are long ("autovacuum worker", "background writer"). The synthetic `io_key` column
   duplicates the three dimensions — consider hiding it from display or making it the (short) key while still
   showing readable `backend_type`/`object`/`context` separately (pgss shows the human `query` separately from the
   `queryid` key). Verify the chosen column set fits ~120–160 cols; if not, drop derived bytes columns or shorten
   `backend_type` labels in SQL.

6. **`track_io_timing` / `track_wal_io_timing` = off → time screen all zeros.** Not an error (columns return 0,
   not NULL). Interview accepts this; a one-line cmdline hint is optional. Don't hide rows for zero timings
   (interview D5).

7. **reset.go — shared stats NOT reset.** `top/reset.go:13` `resetStat` runs `query.ExecResetStats` (per-database)
   + pg_stat_statements; comment line 12 explicitly says "Don't reset shared stats, such as bgwriter or archiver."
   `pg_stat_io` is **shared/cluster-wide stats** (reset only via `pg_stat_reset_shared('io')`). So the `Q` key will
   NOT reset pg_stat_io — consistent with bgwriter/wal/replslots, no action needed, but document it so the absolute
   `stats_age` not jumping on `Q` isn't read as a bug.

8. **restartpoint / standby concerns.** On a standby, `pg_stat_io` is populated (recovery does IO:
   `backend_type='startup'`, `context='normal'/'vacuum'`). No recovery-aware function needed (unlike replslots'
   `WalFunction2`). `stats_age = now() - stats_reset` works on standby. No special handling required; standby is
   not a blocker.

9. **No `PostgresV16/17/18` constants.** `internal/query/query.go:9–18` stops at `PostgresV14`. bgwriter/wal/pgss
   selectors use literal `170000`/`180000`. For consistency either (a) use literals in `io.go`, or (b) add
   `PostgresV16 = 160000` (and 17/18) to query.go and reference `query.PostgresV16` in the `MinRequiredVersion`
   field (the `New()` map uses `query.PostgresVxx` constants, so adding the constant is the cleaner fit there).
   **Decision for tech-spec.**

10. **NotRecordable.** Set `NotRecordable: true` on both views (interview constraint, roadmap 0.11.0 TUI-first;
    ADR [004] "NotRecordable: true for TUI-only scope"). `record/record.go:filterViews()` skips them. Currently
    bgwriter + replslots are the live users of this flag.

### Relevant ADRs (settled — do not re-litigate)
- **[004] Per-version column sets, not NULL-padded** — apply: PG16/17 (op_bytes) vs PG18 (native bytes) are
  separate branches with identical shared headers; different Ncols/DiffIntvl.
- **[004] Absolute counters via DiffIntvl placement** — apply: dimensions + io_key + stats_age outside the diff
  range; counters form one contiguous `[lo,hi]`.
- **[004] NotRecordable: true for TUI-only** — apply directly.
- **[005] coalesce(...,0) on diffed counters for NULL safety** — apply directly (pg_stat_io NULLs are pervasive).
- **[005] Single query when subset is version-stable** — does NOT apply here: pg_stat_io IS version-divergent
  (op_bytes removed, *_bytes/WAL added in 18), so the [004] per-version-branch pattern governs.

### Active tech-debt touching this area
- **[005] `Test_doReload` panics when PG fixture absent** (`top/reload_test.go`, Low): environmental; may make
  `make test` red locally when clusters are down — not caused by this feature, but will surface during local TDD.
  Suggested handling: ignore for this feature (or replace its panic with `t.Skipf` opportunistically).
- No active debt in `internal/query`, `internal/view`, `top/menu.go`, `top/config_view.go`.

---

## 9. External Libraries
None new. pg_stat_io is a built-in PostgreSQL system view; all access is via the existing `pgx/v5` connection
(`internal/postgres`) and the existing query/format/diff pipeline. No Context7 lookup needed beyond the PG
documentation already consulted in §4 (postgresql.org/docs/{16,17,18}/monitoring-stats.html, release/18.0).

---

## Key file:line index
- `internal/query/replication_slots.go:14–41` — multi-row hybrid query + selector signature.
- `internal/query/bgwriter.go:7–52` — per-version branches + selector returning per-version Ncols/DiffIntvl.
- `internal/query/wal.go:5–32` — minimal two-branch precedent; literal version ints.
- `internal/query/statements.go:65` — `queryid = left(md5(userid||dbid||queryid),10)` synthetic composite key.
- `internal/query/query.go:9–18` — `PostgresVxx` constants (stop at V14).
- `internal/view/view.go:10–31` — `View` struct (UniqueKey, NotRecordable, MinRequiredVersion).
- `internal/view/view.go:153–238` — replslots + six statements_* registrations (template for two stat_io entries).
- `internal/view/view.go:328–370` — `Configure()` version-selector switch + Format loop.
- `internal/view/view.go:373–375` — `VersionOK`.
- `internal/stat/postgres.go:303–358` — `diff()`, `ukey` single-column row matching (line 322).
- `internal/stat/postgres.go:444–488` — `diffPair`/`parsePairInt`/`parsePairFloat` (NULL/"" abort path).
- `internal/stat/stat.go:198–201` — `VersionOK` collector gate → not-supported error.
- `top/keybindings.go:29–45` — lowercase-switch / uppercase-menu pairs; `j`/`J` free.
- `top/menu.go:14–25, 35–92, 129–208` — menuType iota, selectMenuStyle, menuSelect.
- `top/config_view.go:102–193` — switchViewTo, *NextView helpers, viewSwitchHandler.
- `top/stat.go:319–361` — printDbstat error path + formatError.
- `top/help.go:9–44` — helpTemplate (add j/J line).
- `top/reset.go:10–40` — resetStat (shared stats not reset).
- `internal/postgres/testing.go:16–44` — NewTestConnectVersion port map.
- `internal/query/bgwriter_test.go:11–63` — selector + live Ncols-gate test pattern.
- `top/menu_test.go:8–21` — selectMenuStyle item-count test.
