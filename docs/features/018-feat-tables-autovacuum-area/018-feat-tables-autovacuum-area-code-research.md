# [018] Tables and autovacuum area — code research

**Date:** 2026-08-06
**Branch:** `develop` (HEAD `49f9f90`, the [017] squash merge)
**Scope researched:** new `autovacuum_scores` screen on a `t`/`T` cycle group, `stats_age` on
`tables`/`indexes`/`functions`, `dead` column on the new screen.

Facts about the PG 19 catalog stated in the task prompt are taken as given and are **not**
re-derived here. Everything below is about the pgcenter code that has to receive them.

---

## 1. Entry Points

### The `t` hotkey path, end to end (research question 1)

| Step | Location | What it does today |
|---|---|---|
| 1. binding | `top/keybindings.go:50` | `{"sysstat", 't', switchViewTo(app, "tables")}` |
| 2. registration | `top/keybindings.go:17-27` (`keybindings`) | iterates `keybindingsList(app)` and calls `SetKeybinding`; the split exists so a test can fetch a handler ([017]) |
| 3. table | `top/keybindings.go:33-103` (`keybindingsList`) | the assertable slice of `key{viewname, key, handler}` |
| 4. dispatch | `top/config_view.go:232-265` (`switchViewTo`) | `switch c` — `"databases"`, `"statements"`, `"progress"`, `"statio"`, `"wal"` have cycle cases; **everything else, including `"tables"`, falls to `default: viewSwitchHandler(app.config, c)`** (`:258-260`) |
| 5. switch | `top/config_view.go:370-379` (`viewSwitchHandler`) | saves current view back into `config.views`, loads the target, zeroes `scrollOffset` and `autoScrollToOrderKey`, lifts pause, pushes on `viewCh` |
| 6. cmdline | `top/config_view.go:262` | `printCmdline(g, "%s", app.config.view.Msg)` |

`T` is **not bound today** — `top/keybindings.go:33-103` contains no `'T'` row. It is free.

### What a new cycle group costs — measured from the [017] `w`/`W` diff

`git show 49f9f90 -- top/ internal/view/ cmd/help.go` is the complete evidence. Production edits,
in order of the diff:

| File | Edit | Size |
|---|---|---|
| `internal/view/view.go` | new registry entry in `New()` (after `"wal"`, before `"bgwriter"`) | +12 lines |
| `internal/view/view.go` | new `case "archiver"` in `Configure()` | +7 lines |
| `top/config_view.go` | new `case "wal"` in `switchViewTo` + a 6-line comment | +8 lines |
| `top/config_view.go` | new `walNextView(current string) string` | +15 lines |
| `top/menu.go` | new `menuWAL` in the `menuType` iota block | +1 line |
| `top/menu.go` | new `case menuWAL` in `selectMenuStyle` | +10 lines |
| `top/menu.go` | new `case menuWAL` in `menuSelect` | +11 lines |
| `top/keybindings.go` | one row `{"sysstat", 'W', menuOpen(menuWAL, app.config, "")}` | +1 line |
| `top/help.go` | one help row; plus edits to the `r` row and the `Q` shared-stats caveat | 3 lines touched |
| `cmd/help.go` | report-flag docs (only because 017 also changed `-W`) | 2 lines |

Production total for the *navigation* half: **~65 lines across 5 files.** The test half of the same
commit was `top/config_view_test.go` +27, `top/help_test.go` +42, `top/keybindings_test.go` +143,
`top/menu_test.go` +71 — i.e. **the tests cost 4× the production code**, which is the honest
estimate to carry into decomposition.

For 018 the same shape applies, with one difference: 017's group name `"wal"` collided with a view
name and needed the comment at `top/config_view.go:250-255`. **`"tables"` has exactly the same
collision** — it is the `report -T` report type (`cmd/report/report.go:146`), the tar entry prefix
in recorded archives, and the `describeReport` key (`report/report.go:671`). So the new
`case "tables":` in `switchViewTo` is the direct analogue of 017's `case "wal":`, comment included,
and `tablesNextView`'s `default` arm must return `"tables"` so `t` from any other screen behaves
exactly as before.

### The `T` menu

`menuOpen` (`top/menu.go:116-147`) takes `(menuType, *config, pgssSchema string)`. The third
argument is a capability gate used only by `menuPgss` (`:121-124`); every other call site passes
`""` (`top/keybindings.go:61-66`). `selectMenuStyle` (`top/menu.go:37-113`) is a pure
`menuType → menuStyle` function; `menuSelect` (`top/menu.go:150-251`) maps the cursor index `cy`
to a view name per menu, with a `default` arm.

---

## 2. Data Layer

### The three screens gaining `stats_age` (research question 4)

| View | Registry | Query const | Ncols | DiffIntvl | OrderKey | UniqueKey |
|---|---|---|---|---|---|---|
| `tables` | `internal/view/view.go:85-95` | `query.PgStatTablesDefault`, `internal/query/tables.go:5-20` | 19 | `{1,18}` | 0 | 0 (default) |
| `indexes` | `internal/view/view.go:96-106` | `query.PgStatIndexesDefault`, `internal/query/indexes.go:5-11` | 6 | `{1,5}` | 0 | 0 |
| `functions` | `internal/view/view.go:118-128` | `query.PgStatFunctionsDefault`, `internal/query/functions.go:5-11` | 8 | `{3,3}` | 0 | 0 |

None of the three has a `Select*Query` selector and none has a `case` in
`Views.Configure` (`internal/view/view.go:379-433`) — they are the *only* recordable screens with
no version branch at all.

Exact column layouts (0-based):

- **`tables`** — 0 `relation`; 1 `seq_scan`, 2 `seq_read`, 3 `idx_scan`, 4 `idx_fetch`, 5 `inserts`,
  6 `updates`, 7 `deletes`, 8 `hot_updates`, 9 `live`, 10 `dead`, 11 `heap_read,KiB`, 12 `heap_hit`,
  13 `idx_read,KiB`, 14 `idx_hit`, 15 `toast_read,KiB`, 16 `toast_hit`, 17 `tidx_read,KiB`,
  18 `tidx_hit`. Source: `pg_stat_{{.ViewType}}_tables t, pg_statio_{{.ViewType}}_tables i
  WHERE t.relid = i.relid` (`internal/query/tables.go:19-20`).
- **`indexes`** — 0 `index`; 1 `scan`, 2 `tuples_read`, 3 `tuples_fetch`, 4 `hit`, 5 `read,KiB`.
  Source: `pg_stat_{{.ViewType}}_indexes s, pg_statio_{{.ViewType}}_indexes i
  WHERE s.indexrelid = i.indexrelid` (`internal/query/indexes.go:10-11`).
- **`functions`** — 0 `funcid`, 1 `function`, 2 `calls_total`, 3 `calls` (the only diffed column),
  4 `total`, 5 `self`, 6 `total_avg,ms`, 7 `self_avg,ms`. Source: `pg_stat_user_functions`.

**Where a trailing `stats_age` lands.** Every existing `stats_age` in the codebase is the **last**
column and sits **outside** `DiffIntvl` — no exceptions:

| Screen | Ncols | stats_age index | DiffIntvl | file:line |
|---|---|---|---|---|
| `wal` PG14 / PG18 / PG19 | 11 / 7 / 8 | 10 / 6 / 7 | `{2,9}` / `{2,5}` / `{2,6}` | `internal/query/wal.go:38-49` |
| `bgwriter` PG14/17/18 | 12 / 13 / 14 | 11 / 12 / 13 | `{3,10}` / `{6,11}` / `{6,12}` | `internal/query/bgwriter.go:41-52` |
| `archiver` | 9 | 8 | `{0,0}` | `internal/query/archiver.go:57-59` |
| `stat_io` | 16 | 15 | `{4,14}` | `internal/query/io.go` |
| `stat_io_time` | 10 | 9 | `{4,8}` | `internal/query/io.go` |
| `replslots` | 15 | 14 | `{6,13}` | `internal/query/replication_slots.go:39-41` |
| `databases_general` | 19 | 18 | `{2,17}` | `internal/query/databases.go:45-52` |

So the pattern is unambiguous: **append at the tail, leave `DiffIntvl` and `UniqueKey` untouched.**
Concretely — `tables` 19→20 cols, `DiffIntvl {1,18}` unchanged; `indexes` 6→7, `{1,5}` unchanged;
`functions` 8→9, `{3,3}` unchanged. This is materially cheaper than [012]'s progress-screen
mid-layout insertion (ADR `docs/decisions-log.md:801`), which had to move `DiffIntvl` per version.

**Which `stats_reset` feeds it (the "source must be chosen" question).** Per the prompt's verified
catalog, PG 19 added `stats_reset` to `pg_stat_all_tables`, `pg_stat_all_indexes`,
`pg_stat_user_functions` and `pg_statio_all_tables`. Mapping that onto the three joins:

- `functions` — one candidate (`pg_stat_user_functions`). **No decision.**
- `indexes` — the query joins `pg_stat_*_indexes` with `pg_statio_*_indexes`, and only the former
  is in the list. **No decision** — `s.stats_reset`. *(Worth one live `\d pg_statio_all_indexes`
  before writing the spec: if `pg_statio_all_indexes` also gained the column, this becomes a
  decision after all.)*
- `tables` — the query joins `pg_stat_*_tables` (has it) with `pg_statio_*_tables` (has it too).
  **This is the only genuine choice**, and the governing precedent is ADR
  `[004-feat-bgwriter-checkpointer] stats_age sourced from pg_stat_checkpointer on PG 17+`
  (`docs/decisions-log.md:239`): when two independent reset timestamps exist, pick the one whose
  data dominates the screen and say so in the user-spec. On `tables` that is `t.stats_reset`
  (`pg_stat_*_tables` supplies 10 of the 18 metric columns, and it is the one
  `pg_stat_reset_single_table_counters()` targets). Showing both was rejected in [004] as column
  noise; the same argument applies verbatim.

### The new `autovacuum_scores` view

Nothing exists yet. Shape implied by the interview + the codebase conventions:

- New file `internal/query/autovacuum_scores.go` with `PgStatAutovacuumScoresDefault` and
  `SelectStatAutovacuumScoresQuery(version int) (string, int, [2]int)` — the 3-tuple selector
  signature used by every multi-row view since [005] (`internal/query/replication_slots.go:39`).
  Version-independent today, so `_ int` per revive, with the `archiver.go:50-57` doc comment as the
  template for saying *why* the parameter is unused.
- Registry entry in `internal/view/view.go: New()` with `MinRequiredVersion: query.PostgresV19`
  (`internal/query/query.go:22`), `NotRecordable: true`, non-nil `ColsWidth`/`Filters` (see
  `patterns.md:186-190` — three writers in `top/config_view.go` write into those maps **in place**,
  so a nil map is a panic, not a wrong number).
- `case "autovacuum_scores"` in `Configure()` — mandatory, not cosmetic: `TestViews_Configure`
  asserts `v.Query != ""` for every registered view over all 56 matrix rows
  (`internal/view/view_test.go:303-305`).

**`DiffIntvl {0,0}`** is the right value and should be stated as a decision, not left as a
placeholder — the `archiver.go:50-57` reasoning transfers exactly: every column is a gauge
(current score, current boolean, current `n_dead_tup`), and `{0,0}` makes `calculateDelta`
short-circuit at `internal/stat/postgres.go:591-598` before `diff()`, which is what makes NULL and
non-numeric cells safe without `coalesce`. It also makes `UniqueKey` irrelevant (it is read only
inside `diff`, `internal/stat/postgres.go:625`), so the default 0 is fine.

**The `dead` column.** `n_dead_tup` lives in `pg_stat_all_tables`, which has no user/sys/all
filtering problem of its own. Join `LEFT JOIN pg_stat_all_tables ON relid` (the `all` variant
unconditionally — filtering must stay in the outer `schemaname` predicate, or the `all` ViewType
would silently drop rows). Under `DiffIntvl {0,0}` a LEFT-JOIN NULL renders as a blank cell and is
safe — the `coalesce(...,0)` rule from ADR [005] (`docs/decisions-log.md:285`) applies **only**
inside a diffed range.

---

## 3. Similar Features

**The nearest precedent for the whole feature is [017] itself** (`git show 49f9f90`), and after it
[006] (`j`/`J` statio split, `docs/decisions-log.md:349`). Reusable as-is:

- `walNextView` (`top/config_view.go:298-310`) — 13-line pure function, table-tested. Copy shape
  verbatim for `tablesNextView`.
- `menuWAL` (`top/menu.go:22`, `:97-105`, `:214-223`) — the three-touchpoint menu addition.
- `SelectStatArchiverQuery` (`internal/query/archiver.go:57-59`) — the version-independent 3-tuple
  selector with a doc comment explaining the unused parameter.
- `internal/query/archiver_test.go` — the 017-standard query test: package-level version list
  (`:18`), locked column-name constant (`:23-26`), server-free structure test (`:83-94`),
  `connectArchiverFixture` that distinguishes "cluster down" (skip) from "version missing from the
  port map" (fail) (`:371-382`), and `runArchiverQuery` returning names + row count + drain error
  (`:388-407`).

For the `stats_age` half, the reference is `internal/query/wal.go` (the PG19 branch at `:27-34`
plus `SelectStatWALQuery` at `:38-49`) and `internal/query/bgwriter.go:41-52` — both return
`(query, Ncols, DiffIntvl)` and both keep `stats_age` outside the range.

---

## 4. Integration Points

### The ViewType toggle (research question 2)

`toggleSysTables` — `top/config_view.go:450-493`:

1. **Guard** (`:452-455`): `if name != "tables" && name != "indexes" && name != "sizes" { return nil }`
   — a closed set of three view names. The new screen must be added here or `,` is a silent no-op
   on it.
2. **Flip** (`:457-462`): `config.queryOptions.ViewType` toggles `"user"` ↔ `"all"`.
   Seeded to `"user"` by `query.NewOptions` (`internal/query/query.go:47`).
3. **Reformat** (`:464-472`): loops over the literal `[]string{"tables", "indexes", "sizes"}`,
   re-runs `query.Format(config.views[t].QueryTmpl, config.queryOptions)`.
4. **Write-back** (`:474-479`): same literal list, writes `v.Query` back into `config.views[t]`.
5. `config.view = config.views[name]`, lift pause, push on `viewCh`, cmdline
   `"Show relations: %s"`.

Both literal lists at `:466` and `:475` must gain the new name. Note the guard list and the
reformat lists are three separate literals — an easy partial edit.

**How the value reaches the query.** Only through the `{{.ViewType}}` template variable. Three
templates use it today: `internal/query/tables.go:19`, `internal/query/indexes.go:10`,
`internal/query/sizes.go:17` — all of them by string-splicing the view name
(`pg_stat_{{.ViewType}}_tables`). `query.Format` (`internal/query/query.go:91-104`) is plain
`text/template`, so **any** template construct works, not only name splicing.

**How a view without user/sys/all variants can honor it.** Since `Format` is full `text/template`,
either of these is Format-safe and needs no new plumbing:

```
{{if ne .ViewType "all"}}WHERE schemaname NOT IN ('pg_catalog','pg_toast','information_schema'){{end}}
```

or a value-substituting predicate that avoids a conditional block entirely:

```
WHERE ('{{.ViewType}}' = 'all' OR schemaname NOT IN ('pg_catalog','pg_toast','information_schema'))
```

Two constraints on the choice:

- The template must stay Format-safe when `ViewType` is `""`. It **is** empty in one real call
  path: `report/report.go:285-287` calls `views.Configure(query.Options{Version: d.meta.version})`
  with no `ViewType`. Harmless for this screen (`NotRecordable`, never replayed) but the second
  form degrades to "filter on" while the first degrades to "filter on" too — both fine; the
  `pg_stat_{{.ViewType}}_tables` splice used by `tables`/`indexes`/`sizes` degrades to the
  nonexistent `pg_stat__tables`, which is pre-existing and only survives because report never
  executes the query.
- `Test_toggleSysTables` (`top/config_view_test.go:749-794`) asserts on the substrings
  `"pg_stat_all"` / `"pg_stat_user"` in the formatted query. A new row for `autovacuum_scores`
  needs a different marker (e.g. presence/absence of `pg_catalog`), so the test table's shape
  changes, not just its length.

### Version guard: what the TUI actually shows (research question 3)

Traced end to end:

1. `internal/stat/stat.go:316-319` — `if !view.VersionOK(c.config.VersionNum) { return s,
   fmt.Errorf("selected statistics is not supported by current version of Postgres") }`. This sits
   **before** `collectPostgresStat` (`:322`), so no query is sent and nothing appears in the PG log
   — the comment at `:316` says exactly that. The returned `s` already carries `LoadAvg`,
   `Meminfo`, `CPUStat` and `Pgstat.Activity`, collected at `:206-314`.
2. `top/stat.go:66-69` — `collectStat` sets `stats.Error = err` and **continues the loop**; the
   error does not stop collection.
3. `top/stat.go:793-802` — `printDbstat` sees `s.Error != nil`, writes `formatError(s.Error)` into
   the `dbstat` view and returns early. No header, no rows, no alignment.
4. `top/stat.go:858-870` — `formatError` renders a non-`*pgconn.PgError` as
   `ERROR: selected statistics is not supported by current version of Postgres`.

**Net effect:** the two summary panels keep updating normally; the whole table area is replaced by
that one line, repeated every refresh tick, until the user presses another screen key. The cmdline
still shows the view's `Msg` written by `switchViewTo` (`top/config_view.go:262`). No cmdline
warning, no auto-revert. The immediate first frame also comes through the guarded send at
`top/stat.go:128-141`.

**Is any existing navigation version-aware? No.** Verified:

- `menuStatIO` offers `stat_io`/`stat_io_time` (`MinRequiredVersion: PostgresV16`,
  `internal/view/view.go:179`, `:192`) and `J` is bound unconditionally
  (`top/keybindings.go:65`) — on a PG 14 fixture the menu opens and the screen errors.
- `menuProgress` offers `progress_copy` (`PostgresV14`, `internal/view/view.go:315`) on PG 12.
- `walNextView` cycles into `archiver` (`PostgresV14`) regardless of version.
- `selectMenuStyle`, `menuOpen` and `menuSelect` take no version and no `*app`; `menuOpen` does
  take `*config`, and `config.queryOptions.Version` **is** populated (`top/top.go:81`,
  `top/config.go:21`), so a version is reachable there without a signature change.

So making the `T` menu version-aware would introduce a **new pattern**, and it is not free:
filtering `selectMenuStyle`'s `items` shifts the `cy` → view-name mapping that `menuSelect`
(`top/menu.go:150-251`) hardcodes, and both are pinned by tests
(`top/menu_test.go:18-24`, `:53-61`).

The cycle is a different matter and there is a real argument for it: `t` is the primary
"show me tables" key. If `tablesNextView` is version-blind, a DBA on PG 18 who presses `t` twice
lands on a permanent error screen and must press `t` a third time to escape — worse than a menu
item that is merely offered. Making **only** `tablesNextView` version-aware keeps the function pure
and table-testable:

```go
func tablesNextView(current string, version int) string
```

with the version read from `app.postgresProps.VersionNum` (`top/top.go:46`, set at `:82`) or
`app.config.queryOptions.Version` at the single call site in `switchViewTo`. That is a
one-parameter change confined to one file plus its test table.

### Report / replay (research question 5)

`tables`, `indexes` and `functions` **are** recordable and **do** have report layouts:

- CLI flags `-T` / `-I` / `-F` — `cmd/report/report.go:66-69`, mapped to view names at `:146-150`.
- Descriptions — `report/describe.go:82` (`pgStatTablesDescription`), `:110`
  (`pgStatIndexesDescription`), `:125` (`pgStatFunctionsDescription`); registered in the
  `describeReport` map at `report/report.go:671-673`.

**Does a column-count change ripple into the replay path?** Traced through
`report/report.go:248-400` (`processData`):

- Rendering is entirely **result-driven**: `formatStatSample` (`:528-538`) calls
  `align.SetAlign(*d, ...)` on the *result* and writes `view.Cols`/`view.ColsWidth`;
  `printStatHeader` (`:560-577`) iterates `v.Cols`. **`view.Ncols` is never read in the `report`
  package** — confirmed by grep: its only non-`Configure` readers are
  `top/config_view.go:26` and `:48` (the sort-key wrap).
- `view.DiffIntvl` **is** read, via `countDiff` → `stat.Compare` → `calculateDelta`
  (`internal/stat/postgres.go:576-603`). With `stats_age` appended at the tail, `{1,18}` / `{1,5}` /
  `{3,3}` stay correct for both the PG ≤ 18 and PG 19 layouts, so **no `DiffIntvl` change and no
  version-dependent interval** — the trap ADR [012] documents (`docs/decisions-log.md:801`, a stale
  interval landing on parseable columns and producing plausible nonsense) does not arise here.
- The version-change path at `:276-319` re-runs `views.Configure(query.Options{Version:
  d.meta.version})` and resets `Aligned`, the header counter and the seed sort key
  (`patterns.md:212-235`). Adding `case "tables"/"indexes"/"functions"` to `Configure` is what
  makes a mid-archive PG18→PG19 transition re-derive the right query — and, more importantly, it is
  what makes the TUI's `view.Ncols` correct so `→` can reach the new last column.

**Conclusion:** the ripple is small but the `Configure` cases are still required (for `view.Ncols`
in `top`, and for `TestViews_Configure`'s per-version assertions to be able to pin the PG 19
branch). What is *not* required: a `DiffIntvl` change, a `UniqueKey` change, or new golden files —
though `report/testdata/report_record_tables_pg19.golden` alongside a `_pg18` one would follow the
[017] `report_record_wal_pg18/pg19.golden` precedent and is the only way to pin the branch in
replay.

The `describe.go` texts for the three screens should gain a `stats_age` line; note
`report/report_test.go:1217-1412` already has four column-order describer tests
(`Test_describeProgressColumnOrder`, `..._describeActivityColumnOrder`,
`..._describeArchiverColumnOrder`, `..._describeWALColumnOrder`) — there is a live convention of
pinning describe text against the query's column order.

`autovacuum_scores` needs **none** of this: `NotRecordable: true` means `filterViews`
(`record/record.go:204-212`) drops it before recording, so no `-flag`, no describe entry, no
golden. Per the roadmap's cross-cutting policy (`docs/roadmap-0.12.0.md:73-77`) the retrofit is an
explicitly planned 0.13.0 item.

---

## 5. Existing Tests

### The complete count-based blast radius (research question 6)

**(a) Registering a new view.**

| file:line | pins | today | after |
|---|---|---|---|
| `internal/view/view_test.go:11` | `assert.Equal(t, 28, len(v))` in `TestNew` | 28 | **29** |
| `internal/view/view_test.go:314-320` | `TestView_VersionOK` table `{version, total}` | `{190000,28} {160000,28} {140000,25} {130000,19} {120000,16} {110000,14} {100000,14}` | only the **`190000` row** → 29 (`MinRequiredVersion: PostgresV19`); every other row unchanged. This mirrors feature 007's PG15+ view bumping only the `160000` row (`patterns.md:181`) |
| `internal/view/view_test.go:303-305` | `TestViews_Configure`: `v.Query != ""` for **every** view across all 56 matrix rows | — | fails unless the `Configure` case exists |
| `record/record_test.go:142-148` | `Test_filterViews` table `{version, pgssSchema, wantN, wantV, wantArchiver}` | `wantN+wantV == 28` on every row | `NotRecordable: true` is dropped unconditionally (`record/record.go:208-212`), so **`wantN` +1 on every one of the 7 rows and `wantV` unchanged** — `{190000, 0→1, 28}`, `{140000/"", 9→10, 19}`, `{140000/public, 3→4, 25}`, `{130000, 9→10, 19}`, `{120000, 12→13, 16}`, `{110000, 14→15, 14}`, `{100000, 14→15, 14}`. Runs without Postgres (`patterns.md:182`) |
| `record/recorder_test.go:30-57` | `Test_tarRecorder` passes **unfiltered** `view.New()` to `tc.collect` | — | **see Potential Problems #1 — this one breaks and is not obvious** |
| `report/report_test.go:1172-1205` | `Test_describeReport`, explicit rows | 27 view rows + `invalid` (note: `statements_wal` is already missing) | no automatic break; adding a row is a choice |

**(b) A column-count change on `tables`/`indexes`/`functions`.** Nothing in `internal/view/view_test.go`
pins these three views' `Ncols` today (the per-view pins at `:24-128` cover `statements_jit`,
`stat_io`, `stat_io_time`, `replslots`, `bgwriter`, `archiver` only), and
`internal/query/tables_test.go` / `indexes_test.go` / `functions_test.go` assert nothing about
shape (see below). So **the column change is currently unguarded** — new assertions have to be
*added*, not updated. Adjacent pins that would move if a `Configure` case is added:
`internal/view/view_test.go:218-256` (the PG19 / PG14 arms of `TestViews_Configure`, where the
per-version `Ncols`+`DiffIntvl` for `wal`/`archiver`/`progress_*` already live) is the natural home
for the new assertions.

**(c) A new hotkey cycle group / menu.**

| file:line | pins |
|---|---|
| `top/menu_test.go:18-24` | `Test_selectMenuStyle` — `{menuNone 0, menuDatabases 2, menuPgss 7, menuProgress 6, menuConf 4, menuStatIO 2, menuWAL 2}`; new row `{menuTables, 2}` |
| `top/menu_test.go:53-91` | `Test_menuSelectWAL` — cursor→view mapping incl. the `cy 5` default arm and the `menuNone` reset |
| `top/config_view_test.go:598-630` | `Test_switchViewTo` — 30 rows. Note the comment at `:625-627`: only the **first** row of a cycle (`{current:"wal", to:"wal", want:"archiver"}`) proves the dispatch case exists; the other two stay green through the `default` arm. The same holds for `t`: only `{current:"tables", to:"tables", want:"autovacuum_scores"}` is load-bearing |
| `top/config_view_test.go:695-708` | `Test_walNextView` — the shape to copy for `Test_tablesNextView` |
| `top/config_view_test.go:749-794` | `Test_toggleSysTables` — 6 rows over the closed `{tables, indexes, sizes}` set + a no-op row on `activity` |
| `top/keybindings_test.go:31-42` | `boundHandler(t, app, viewname, k)` — scans `keybindingsList(app)` and **returns the closure** (closures are not comparable), which the test then runs and asserts the effect of. Used at `:102` (`'W'` → menu title + items) and `:135` (`'w'` from the `wal` screen → `"archiver"` on `viewCh`). This is the only mechanism that makes "which handler a key carries" assertable — before the 017 split, binding `W` to the wrong menu left the suite green (`patterns.md:111-117`) |
| `top/keybindings_test.go:50-87` | `Test_keybindingsWAL` — registers on a zero-value `&gocui.Gui{}` and uses a double `DeleteKeybinding` (second must fail with the literal `"keybinding not found"`) to prove exactly-once registration, plus that `"", "menu", "dialog", "help"` do not bind the key |
| `top/help_test.go:118-131` | `Test_helpTemplate_walEntry` — marker `"'w' "` must match exactly one line, prefix `"    w,W"`, description verbatim, **and the line must sit at `statioIdx+1`**. A `t,T` row inserted anywhere in that block breaks the relative-index assertions at `:68-70` and `:129` |
| `top/help_test.go:14-30` | `helpEntryLine()` fails if its marker matches 0 **or ≥2** lines |
| `top/help_test.go:149-153` | the `Q` caveat line must contain the literal `"pg_stat_io, bgwriter, wal, archiver"` |
| `top/help_test.go:161-170` | `strings.Count(helpTemplate, "%") == 1` — any literal `%` in a new help row breaks it |

Untested and therefore a silent doc gap: `cmd/help.go:163-178` (report flag documentation; already
stale — `-X` selectors do not list `'j'` for JIT).

### Multi-version query-test utilities (research question 7)

`internal/postgres/testing.go` — no build tag, links into the release binary, must not import
`testing`:

| Symbol | line | notes |
|---|---|---|
| `testRoleNameRE` | `:12` | `^[a-z_][a-z0-9_]*$` — role names are SQL identifiers, not bindable |
| `NewTestConfig()` | `:15` | `127.0.0.1:21917`, user `postgres`, db `pgcenter_fixtures` |
| `NewTestConnect()` | `:20` | wrapper → `NewTestConnectVersion(170000)` |
| `NewTestConnectVersion(version)` | `:27` | ports `190000→21919 … 140000→21914` active; PG 9.4–13 mapped but absent from the image (tech debt [024]). Unmapped version → `"postgres version %d has no test cluster port mapping"` **before** connecting (`:48`) |
| `SetupTestRole(db, name, pgMonitor)` | `:68` | idempotent `CREATE ROLE ... NOLOGIN NOSUPERUSER`, optional `GRANT pg_monitor`, then `SET ROLE`; caller owns `RESET ROLE`. Returns `error`, takes no `*testing.T` (ADR `[017] One shared test-role helper`) |

Version-list styles in `internal/query/*_test.go`:

- **Legacy inline 12-version list** `{90500…190000}` — used by `tables_test.go:11`,
  `indexes_test.go:11`, `functions_test.go:11` (and 8 others). Six of the twelve always skip.
- **017 package-level `var` with a rationale comment** — `archiver_test.go:18`
  (`archiverVersions = []int{140000…190000}`), `overview_test.go:15`.
- **Server-free selector table** — `wal_test.go:14-28`, `archiver_test.go:36-53`, including
  out-of-range probes (`200000`, `130000`, `0`) that prove a branch fires for all future majors.

The hardened skip idiom (017, `archiver_test.go:371-382`) distinguishes "cluster not running" from
"version missing from the port map":

```go
conn, err := postgres.NewTestConnectVersion(version)
if err != nil {
    assert.NotContains(t, err.Error(), "no test cluster port mapping",
        "version %d is missing from the test port map", version)
    t.Skipf("postgres %d not available in test environment", version)
}
```

**What the three target tests do today** — `internal/query/tables_test.go:10-31`,
`indexes_test.go:10-31`, `functions_test.go:10-31` are byte-for-byte the same shape and the
*weakest* tier in the repo: `Format(...)` must succeed and `conn.Exec(q)` must not error. They do
**not** assert `Ncols`, `DiffIntvl`, column names or row counts (`Exec` discards the result set),
do not assert `NotContains(q, "{{")`, and do not `defer conn.Close()` (`:29`), so an assertion
failure leaks the connection. Any PG-19 branch added to these three files needs the assertions
built from scratch, at the `archiver_test.go` level (locked column-name list + `assert.Equal` on
the live `FieldDescriptions()` order).

There is **no shared exported "run a query, return column names" helper**; each test does it inline
or via a file-local one (`runArchiverQuery`, `archiver_test.go:388-407`, the most complete: names +
row count + drain error, because a privilege error may surface only on drain). `PGresult` is
defined at `internal/stat/postgres.go:444-451` and its `Cols` is filled from
`rows.FieldDescriptions()` at `:496-500`.

Report golden-file pattern (017): goldens in `report/testdata/report_record_<screen>[_<variant>].golden`,
`-update` flag at `report/report_test.go:24`, in-memory tar with `meta.<ts>.json` +
`<screen>.<ts>.json` + `sysinfo.<ts>.json` per tick, **two ticks one second apart** (the first is
discarded by `processData`'s first-snapshot rule), `TsStart`/`TsEnd` must bracket the filenames or
the test degenerates into an empty report while looking real
(`report/report_record_archiver_test.go:209-288` is the reference skeleton).

---

## 6. Shared Utilities

- `query.Format(tmpl, opts)` — `internal/query/query.go:91-104`. Plain `text/template`; any
  construct is available, including `{{if}}`.
- `query.NewOptions(version, recovery, track, querylen, pgssSchema)` —
  `internal/query/query.go:42-64`. Seeds `ViewType: "user"`.
- `view.Views.Configure(opts)` — `internal/view/view.go:379-446`. Per-view `switch`, then a second
  loop that formats every template into `view.Query`.
- `view.View.VersionOK(version)` — `internal/view/view.go:449-451`. Plain `>=`.
- `stat.Compare` / `calculateDelta` / `diff` / `PGresult.sort` — `internal/stat/postgres.go:576-603`,
  `:606-661`, `:679+`.
- `align.SetAlign(r, truncLimit, dynamic)` — `internal/align/align.go:14-79`. Column width floor is
  `max(len(colname), 8)`; `top` uses fixed aligning (`dynamic=false`), `report` dynamic.
- `top/config_view.go:370-379` `viewSwitchHandler` — the one screen-switch helper; lifts pause,
  resets scroll.
- `postgres.SetupTestRole` — `internal/postgres/testing.go:68-94`.

Formatting of numerics (research question 8) — **there is no Go-side float formatter for stats
table cells.** Every value reaches the table as a string produced by SQL. Established idioms:

| Idiom | Example |
|---|---|
| `round(expr, N)::text` | `internal/query/functions.go:9-10` (`round((total_time / greatest(calls,1))::numeric(20,2), 4)::text`), `progress_vacuum.go:8-9` (`round(100 * ... , 2)::text`) |
| `::numeric(20,2)` cast | `internal/query/databases.go:13-14` |
| integer division + `::bigint` | `internal/query/io.go` (`(coalesce(read_bytes,0)/1024)::bigint`) — used explicitly so a `numeric` source does not render decimals |
| `date_trunc('seconds', now() - stats_reset)::text` | the universal `stats_age` idiom, 7 screens |

`internal/pretty` (`Size`, `SizeWidth`, `RateUnit`, `ReserveWidth`, `Ceil`) exists but is used
**only** by the summary panels (`top/stat.go` `printSysstat`/`printPgstat`), never by the main
table.

**Boolean rendering — one precedent exists.** `internal/query/replication_slots.go:16` renders
`s.active::text AS active`, i.e. literal `true`/`false`. The other convention in the codebase is a
one-character sentinel: `coalesce((a.wait_event_type ||'.'|| a.wait_event), 'f') AS waiting`
(`internal/query/progress_create_index.go:7`, `progress_copy.go:21`) — but that is a
NULL-substitution, not a boolean cast. So for `do_vacuum`/`do_analyze`/`for_wraparound` the
existing precedent is `::text` → `true`/`false`; a `case when ... then 't' else 'f' end` would be
new. Width is not a constraint either way: `align` floors at `max(len("for_wraparound"), 8) = 14`.

---

## 7. Potential Problems

Ordered by how likely they are to be missed.

### 1. `record/recorder_test.go: Test_tarRecorder` breaks on a PG19-only view — and the failure is opaque

`record/recorder_test.go:30-57` connects via `postgres.NewTestConnect()` (**PG 17**,
`internal/postgres/testing.go:20`), runs `views.Configure(opts)` on the **unfiltered**
`view.New()`, then calls `tc.collect(dbConfig, views)`. `tarRecorder.collect`
(`record/recorder.go:116+`) loops over every view and does
`stat.NewPGresultQuery(db, v.Query)`, returning `nil, err` on the first failure — it has **no
version gate and no per-view error tolerance**. Today nothing in the registry fails on PG 17 (the
highest floor is `PostgresV16`). `autovacuum_scores` will: `pg_stat_autovacuum_scores` does not
exist on PG 17, so `collect` aborts and the test fails with a bare relation-does-not-exist error
that names no view.

This is *not* covered by `patterns.md`'s "Adding a New View" checklist and was not on the [017]
list because `archiver`'s floor is PG 14. **Severity: blocking for `make test`.** Options: filter
by `VersionOK` in the test before calling `collect`, or gate at `collect`. Prefer the test-side
change — changing `collect` to skip is a behaviour change for `pgcenter record` and needs its own
decision (today a failing view aborts the whole recording by design, `architecture.md:93-94`).

### 2. Rounding `score` to 2 decimals collapses the column and destroys the default sort

The interview settled "2 decimals" (`…-interview.yml:61-63`) against the observation that raw
values render as `1.3e-07`. But `round(1.3e-07::numeric, 2)` is `0.00` — and on a healthy cluster
**most rows are in that range**. The consequence is not cosmetic: `PGresult.sort`
(`internal/stat/postgres.go:679+`) picks its comparator from the first non-empty cell, parses
`"0.00"` as a float, and then has 100+ exact ties. `sort.SliceStable` (per `patterns.md:200-202`)
preserves input order, so the visible order becomes whatever the SQL returned — arbitrary unless
the query says otherwise.

Two things to settle in the spec:
- **Keep an `ORDER BY score DESC` in the SQL** so the client-side stable sort inherits the true
  (unrounded) order for the ties. Precedent: `replication_slots.go:31`
  (`ORDER BY "retained,KiB" DESC NULLS LAST`).
- **Reconsider the precision.** 2 decimals is right for the actionable range (score ≈ 1 is
  "autovacuum is about to come"), wrong for everything below it. Alternatives worth naming:
  `round(...,4)`, or `to_char(score, '9.99EEEE')` (keeps `1.3e-07` readable and comparable),
  or presenting the score as a percentage of threshold. This is a product decision, not a
  formatting detail.

### 3. Tech debt [035] fires on `stats_age` with near-certainty, unlike on `archiver`

`docs/tech-debt.md:28-43` — column widths are computed from the first batch
(`top/stat.go:782-790`, `alignViewToResult` returns early once `Aligned`), and `printDataCell`
(`top/stat.go:1222-1239`) truncates anything longer than the frozen width with `~`.

On `archiver` this needed a cluster that had never archived. On `tables`/`indexes`/`functions` the
prompt's verified fact is that **`stats_reset` is NULL until that relation's stats are actually
reset — 0 of 116 populated on a fresh cluster.** So the *default* state is a blank column, which
freezes at `max(len("stats_age"), 8) = 9`. A later value of `3 days 04:12:33` (15 chars) renders as
`3 days 0~`. `00:05:12` (8 chars) fits, so the truncation appears only once the age exceeds a day —
i.e. exactly when the column is telling you something.

Pre-existing debt, not opened by this feature, but it should be called out in the user-spec rather
than discovered on the stand. Cheap partial mitigation inside the feature: none that does not touch
`align` (out of scope). Honest mitigation: name it in the release notes, as [017] did for [034].

### 4. `"tables"` is a group name that is also a view name, a report type and a tar prefix

Exactly 017's `"wal"` situation (`top/config_view.go:250-255`, ADR
`docs/decisions-log.md:1229`). `"tables"` is the `report -T` type (`cmd/report/report.go:146`), the
`describeReport` key (`report/report.go:671`), the recorded tar entry prefix, and the
`toggleSysTables` guard name (`top/config_view.go:453`). **It cannot be renamed.** Carry 017's
comment verbatim so the next reader does not "fix" it into a separate group name.

### 5. `dead` duplicates a column that already exists two keystrokes away

`tables` already shows `dead` at column 10 (`internal/query/tables.go:10`) — but **diffed**
(inside `DiffIntvl {1,18}`), so it renders as dead tuples *per interval*, not the absolute count.
The proposed `dead` on `autovacuum_scores` would be absolute. Two screens, same column name,
different meaning. This is precisely the shape of registered tech debt [023]
(`docs/tech-debt.md:171`, "`horizon_xacts` names two different quantities on two screens"). If the
column ships, name it distinguishably (`dead_total`?) or document the divergence deliberately.

### 6. Three separate literal lists in `toggleSysTables`

`top/config_view.go:453` (guard), `:466` (reformat loop), `:475` (write-back loop). A partial edit
compiles, passes `Test_toggleSysTables` (which only iterates the three old names), and produces a
screen where `,` flips the indicator but not the data. Worth a single shared `var` or an explicit
task-level checklist.

### 7. The version-guard error frame is silent about *why*

`ERROR: selected statistics is not supported by current version of Postgres` names neither the
screen nor the required version. On a `t`-cycle this is worse than on a menu item because `t` is
the reflex key for "show me tables" (see §4, research question 3). Not a defect in this feature,
but the argument for a version-aware `tablesNextView`.

### 8. ADRs that constrain the implementation (settled — do not re-litigate)

- `[004] stats_age sourced from pg_stat_checkpointer on PG 17+` (`docs/decisions-log.md:239`) —
  the governing precedent for choosing one `stats_reset` of two and saying so in the user-spec.
  Showing both was already rejected as column noise.
- `[004] Per-version column sets, not NULL-padded unified columns` (`:191`) — a PG 19 branch means
  a second query constant, not a `CASE WHEN version …`.
- `[005] coalesce(...,0) on the diffed counters` (`:285`) — applies **only** inside `DiffIntvl`; with
  `{0,0}` the new screen needs none, and `stats_age` outside the range needs none either.
- `[006] Split one wide stats view into two registered sub-views` + its 2026-06-23 update (`:349`) —
  horizontal scroll exists, so a 10–11-column screen is fine as one screen; do **not** split.
- `[006] Synthetic md5 key for composite row identity; column hiding is not available` (`:367`) —
  `relid` cannot be hidden if it were kept, which is one more reason the interview's "drop `relid`"
  is right. With `DiffIntvl {0,0}` there is no cross-sample matching at all, so no synthetic key is
  needed.
- `[012] Progress screens: new columns mid-layout, version-aware DiffIntvl` (`:801`) — the
  cautionary tale for *not* appending at the tail. Appending avoids the whole class.
- `[013] Empty cells sort last in every comparator mode` (`:898`) — a blank `stats_age` or a blank
  `dead` will sort last in both directions. Intended, and worth stating in the acceptance criteria
  so it is not filed as a bug.
- `[017] Report flag values are a closed whitelist` (`:1229`) — not triggered here
  (`autovacuum_scores` is `NotRecordable`), but the reason `"tables"` must keep its name.

### 9. Active tech debt in the modules touched

| Item | Severity | Bearing on 018 |
|---|---|---|
| [035] width frozen from first batch (`tech-debt.md:28`) | Low → **effectively Medium here** | See #3. Directly on `stats_age`. |
| [029] row values reach the terminal unsanitised (`:82`) | Low | `schemaname`/`relname` are server-supplied and reach `printDataCell`. Not widened by this feature (`tables` already prints `schemaname||'.'||relname`), but the new screen adds one more surface. Do not close it here; do not silently widen it either. |
| [024] test port map promises absent clusters (`:194`) | Low | The three legacy test files use the 12-version list; six always skip. If a PG 19 branch is added to `tables_test.go`, use the 017-hardened skip so a missing `190000` mapping fails instead of skipping. |
| [019] nine tests skip every version when one cluster is unavailable (`:252`) | Low | Not in the files this feature touches, but the same trap: use `t.Run` per version. |
| [020] diff loop indexes prev by curr's width (`:268`) | Low | Only reachable on screens with a non-empty `DiffIntvl`. `autovacuum_scores` uses `{0,0}`, so it cannot trigger it. `tables`/`indexes`/`functions` already can — unchanged by appending a column. |
| [023] one name, two quantities on two screens (`:171`) | Low | The precedent for #5 above. |
| [034] `report` exits 0 on every failure path (`:10`) | Medium | Not touched — this feature adds no report flag. |

---

## 8. Constraints & Infrastructure

- **Go 1.25+, testify, gocui, pgx/v5.** `make build` / `make test` (race detector, 300 s timeout) /
  `make lint` (golangci-lint + gosec) / `make vuln`.
- **Lint config** — `.golangci.yml`: errcheck, gocritic, gosimple, govet, ineffassign, revive,
  staticcheck, unused. revive is why an unused selector parameter must be named `_`
  (`internal/query/archiver.go:57`).
- **Test fixtures** — `lesovsky/pgcenter-testing`, clusters on 21914–21919 (PG 14–19), db
  `pgcenter_fixtures`, user `postgres`. PG 19 is beta2 from the `jammy-pgdg-testing 19` component
  (tech debt [017]). Local full-suite instructions are in the user's memory note
  `local_test_fixtures.md`.
- **`internal/postgres/testing.go` has no build tag** and links into the release binary — it must
  not import `testing` (`architecture.md:237`).
- **PG version constants** — `internal/query/query.go:10-22`; `PostgresV19 = 190000` already exists.
- **`NotRecordable` mechanism** — `internal/view/view.go:31` + `record/record.go:204-212`. As of
  [008] **no production view sets it**; `architecture.md:182` says the mechanism survives only for
  a synthetic test. This feature reintroduces the first real user, so that architecture note needs
  updating at `/done` time.
- **Manual QA** — `make build` first, always (`patterns.md:237-243`); remote-stand regimen with
  tmux `-x 190 -y 52` and `capture-pane -p -e` for colour, plus a second binary built from `master`
  for A/B (`patterns.md:245-293`). The `stats_age` truncation (#3) and the score rendering (#2) are
  both stand-only checks.
- **Commit trailers** — exactly one `Co-Authored-By:` after a squash (`patterns.md:362-375`).

---

## 9. External Libraries

None new. Context7 was not consulted: the feature adds no dependency, and the only external
contract is the PostgreSQL 19 catalog, whose shape the task prompt states as verified against a
live 19beta2 cluster (which the roadmap's cross-cutting principle requires —
`docs/roadmap-0.12.0.md:452-453`: *"New-view column sets are settled against the live catalog, not
the release notes"*).

One caveat carried from the interview (`…-interview.yml:134-135`): PG 19 is **beta2**. The catalog
can still move in beta3/RC. Anything asserted about `pg_stat_autovacuum_scores` column names should
be pinned in a locked column-name list in the test
(`internal/query/archiver_test.go:23-26` pattern) so a catalog change reddens a named assertion
rather than producing a silently wrong screen.

---

## Open decisions to carry into the tech-spec

1. Precision/format of the six `*_score` columns — 2 decimals collapses the whole idle range to
   `0.00` (Potential Problems #2). Also: which of the six columns actually earn a slot, given
   `score = greatest(the five sub-scores)`.
2. `OrderKey` for the new screen. `0` (relation) is the multi-row default; `1` (score, desc) is the
   domain-appropriate choice and has precedent in `replslots` `OrderKey: 4`
   (`internal/view/view.go:171`, ADR `docs/decisions-log.md:317`).
3. Boolean rendering: `::text` → `true`/`false` (the `replslots.active` precedent) vs `t`/`f`.
4. Whether `tablesNextView` is version-aware (recommended, §4/Q3) and whether the `T` menu is
   (recommended **not** — no menu in the codebase is).
5. Position of `autovacuum_scores` in the `t` cycle relative to `sizes` — `s` is its own key today,
   so the cycle should stay `tables ↔ autovacuum_scores`, two items, matching `menuStatIO`/`menuWAL`.
6. `dead` column: in or out, and if in, its name (Potential Problems #5).
7. `stats_age` source on `tables`: `t.stats_reset` recommended per ADR [004]; confirm
   `pg_statio_all_indexes` really did *not* gain `stats_reset` before treating `indexes` as
   decision-free.
8. Whether `sizes` (which also uses `pg_stat_{{.ViewType}}_tables`, `internal/query/sizes.go:17`)
   gets `stats_age` too. The roadmap names three screens, not four — an explicit scope boundary
   worth stating rather than leaving implicit.

---

# Updated: 2026-08-10 — second pass, implementation depth

**Branch:** `develop` (HEAD `49f9f90`). **Authority on scope:** the approved user-spec
`018-feat-tables-autovacuum-area.md`.

**Scope corrections that invalidate parts of the first pass.** Read these before using anything
above:

- `functions` is **out of scope** (user-spec "Технические решения", and the negative acceptance
  criterion "экран `functions` не изменился"). Every mention of `functions` in §2 / §4 / §5 above
  is stale. `sizes` is out of scope too, with its own negative criterion.
- The scale-of-work column is **`dead_total`**, not `dead` (user-spec: `dead` on `tables` is the
  diffed per-interval value; one name for two quantities is registered debt [023]). Potential
  Problem #5 above is therefore **resolved by the spec**, not open.
- The new screen's column order is fixed by the spec and is **not** the interview's order:
  `relation, score, do_vacuum, do_analyze, for_wraparound, dead_total, xid_score, mxid_score,
  vacuum_score, vacuum_insert_score, analyze_score`. §"Open decisions" item 6 and the interview's
  ordering (`…-interview.yml:616-618`) are superseded.
- `OrderKey: 1` / `OrderDesc: true`, `DiffIntvl {0,0}`, `NotRecordable: true`,
  `MinRequiredVersion: query.PostgresV19`. Open decisions 2, 3 and 5 are settled.
- Precision is settled at 2 decimals (Potential Problem #2 above is closed as a product decision),
  but its *sorting* consequence is not — see the `ORDER BY` alias trap in §12.4 below, which is a
  real defect in the acceptance criterion as written.
- Terminal width is explicitly **not** a design constraint (user-spec "Дизайн и интерфейс").

---

## U1. Query selectors for `tables` and `indexes`

### What exists today

Both views are the "no selector at all" tier:

| | file:line | today |
|---|---|---|
| `tables` template | `internal/query/tables.go:5-20` | one const `PgStatTablesDefault`, 19 cols |
| `tables` registry | `internal/view/view.go:85-95` | `Ncols: 19`, `DiffIntvl: [2]int{1,18}`, `OrderKey: 0`, `UniqueKey` default 0 |
| `indexes` template | `internal/query/indexes.go:5-11` | one const `PgStatIndexesDefault`, 6 cols |
| `indexes` registry | `internal/view/view.go:96-106` | `Ncols: 6`, `DiffIntvl: [2]int{1,5}`, `OrderKey: 0`, `UniqueKey` default 0 |
| `Configure()` | `internal/view/view.go:379-433` | **no `case "tables"`, no `case "indexes"`** |

### The selector signature: 3-tuple, not 4-tuple

```go
func SelectStatTablesQuery(version int) (string, int, [2]int)
func SelectStatIndexesQuery(version int) (string, int, [2]int)
```

Returned values:

| version | tables | indexes |
|---|---|---|
| `>= PostgresV19` | `PgStatTablesPG19, 20, [2]int{1,18}` | `PgStatIndexesPG19, 7, [2]int{1,5}` |
| below | `PgStatTablesDefault, 19, [2]int{1,18}` | `PgStatIndexesDefault, 6, [2]int{1,5}` |

**Why 3-tuple and not the 4-tuple `(query, Ncols, DiffIntvl, UniqueKey)` form.** The 4-tuple exists
in exactly one place — `SelectStatStatementsJITQuery` (`internal/query/statements.go`, wired at
`internal/view/view.go:398-400`) — and only because `statements_jit`'s `UniqueKey` points at a
*trailing* md5 `queryid` whose index moves when `Ncols` moves (`patterns.md:37-41`). Here the row
identity is column **0** (`relation` / `index`) in both layouts, so `UniqueKey` stays 0 and must
**not** be in the selector. Compare `SelectStatWALQuery` (`internal/query/wal.go:38-49`) and
`SelectStatIOQuery` — both 3-tuple, both leave `UniqueKey` alone.

**Why `DiffIntvl` is still returned even though it is constant across versions.** Two reasons:
`SelectStatArchiverQuery` (`internal/query/archiver.go:57-59`) and
`SelectStatReplicationSlotsQuery` set the precedent that a version-independent value is still
returned for signature symmetry; and `Configure` assigns all three in one statement, so dropping
one would make this the only selector shape in the package. The constancy is the *point* and should
be asserted: `stats_age` appended at the tail keeps `{1,18}` / `{1,5}` valid on both versions,
which is what avoids ADR [012]'s version-aware-`DiffIntvl` class entirely
(`docs/decisions-log.md:801`).

### `Configure()` wiring — byte-for-byte the `wal` shape

Insert into the switch at `internal/view/view.go:384-432`, alongside `case "wal":` at `:401-403`:

```go
case "tables":
    view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatTablesQuery(opts.Version)
    v[k] = view
case "indexes":
    view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatIndexesQuery(opts.Version)
    v[k] = view
```

**Fields patched in lockstep: `QueryTmpl`, `Ncols`, `DiffIntvl`. Nothing else.** Explicitly *not*
`UniqueKey` (stays 0), *not* `OrderKey` (stays 0), *not* `Cols`/`ColsWidth` (render-path state).

The static `New()` entries at `:85-95` / `:96-106` must keep the **PG ≤ 18** values, because every
consumer that does not call `Configure` sees them: `report.newApp` (`report/report.go:83-92`) seeds
`app.view` from a raw `view.New()` and only `processData` Configures it (`report/report.go:282-292`).

### The Configure case is load-bearing here, unlike `archiver`'s

Correcting the first pass: `TestViews_Configure`'s `assert.NotEqual(t, "", v.Query)`
(`internal/view/view_test.go:304`) does **not** force a Configure case — `New()` already seeds a
non-empty `QueryTmpl`, and the second loop at `internal/view/view.go:436-443` formats it. The
`archiver` case is documented in-code as a pure drift guard (`internal/view/view.go:405-408`, and
the test comment at `internal/view/view_test.go:230-235` says the same).

For `tables`/`indexes` the case **is** load-bearing, and the failure mode is precise: without it,
`view.Ncols` stays 19/6 while the PG 19 query returns 20/7 columns. `visibleColumns` reads
`s.Result.Ncols` (`top/stat.go:839`) so the *render* is fine — but `orderKeyRight`
(`top/config_view.go:48`) wraps on `config.view.Ncols`, so the `Right` arrow could never select the
new last column, and `orderKeyLeft` (`:26`) would land on index 18/5 as "the last one". A silent,
render-invisible defect. Say this in the tech-spec so the case is not later "simplified" away.

---

## U2. The ViewType conditional

### A template conditional is already used in `internal/query` — twice

- `internal/query/activity.go:27, 41, 54, 67` — `"{{ if .ShowNoIdle }} AND state != 'idle' {{ end }} ORDER BY pid DESC"`
- `internal/query/procpidstat.go:35` — `{{ if .ShowNoIdle }}AND state != 'idle'{{ end }}`, with the
  contract spelled out in the doc comment at `:17-19`.

So `{{if}}` in a query template is established, not novel. What is new is `ne` on a **string**
field. `query.Format` (`internal/query/query.go:91-104`) is `template.New("query").Parse(tmpl)` with
**no `Funcs()` call**, so only text/template builtins are available — `eq`, `ne`, `and`, `or`, `not`
all are. `{{if ne .ViewType "all"}}…{{end}}` needs no new plumbing.

### How it composes with the existing substitution — it does not have to

`pg_stat_{{.ViewType}}_tables` is a *name splice* and lives only in `tables.go:19`, `indexes.go:10`,
`sizes.go:17`. `pg_stat_autovacuum_scores` has **no** `user`/`sys`/`all` variants
(`…-interview.yml:795`), so the new screen never splices a name — the conditional stands alone in
its own `WHERE` clause and the two constructs never meet in one template. On `tables`/`indexes` the
splice is untouched by this feature.

Recommended form (a block conditional, matching the `ShowNoIdle` precedent):

```
{{if ne .ViewType "all"}}WHERE (s.schemaname NOT IN ('pg_catalog','pg_toast','information_schema') OR s.for_wraparound){{end}}
```

Note the escape hatch is inside the conditional, not outside it: in `all` mode there is no
predicate at all, so `for_wraparound` needs no special mention there.

### Where `ViewType` is populated, and every value it can hold

| path | value | file:line |
|---|---|---|
| seed | `"user"` | `query.NewOptions`, `internal/query/query.go:47` |
| `top` stores opts | `"user"` initially | `top/top.go:67` → `:81` (`app.config.queryOptions = opts`) |
| `,` toggle | flips `"user"` ↔ `"all"` | `top/config_view.go:458-462` |
| `record` | `"user"` | `record/record.go:84` |
| **`report` replay** | **`""`** | `report/report.go:285-287` — `views.Configure(query.Options{Version: d.meta.version})`, no `ViewType` |

The `""` case matters for Format-safety only. With `ne .ViewType "all"` an empty value evaluates
true → the filter is ON, which is the safe degradation. (The existing name-splice degrades to the
nonexistent `pg_stat__tables` on that same path; it survives only because `report` never executes
the query. Pre-existing, not widened here.)

### Wiring the `,` toggle onto the new screen — three separate literals

`toggleSysTables` (`top/config_view.go:450-493`) has the name in **three** places:

1. `:453` — the guard `if name != "tables" && name != "indexes" && name != "sizes" { return nil }`
2. `:466` — `for i, t := range []string{"tables", "indexes", "sizes"}` (reformat loop, and
   `queries := make([]string, 3)` at `:465` is a fourth literal — the **length**)
3. `:475` — the same literal slice again (write-back loop)

A partial edit compiles, passes today's `Test_toggleSysTables`, and produces a screen where `,`
flips the indicator and the header but not the rows. The `make([]string, 3)` at `:465` is the
sharpest of the four: leaving it at 3 while the range list has 4 entries is an index-out-of-range
panic inside a key handler, which gocui does not recover.

`Test_toggleSysTables` (`top/config_view_test.go:749-794`) asserts on the substrings
`"pg_stat_all"` / `"pg_stat_user"`. Neither appears in the new screen's query, so a new row needs a
different marker (presence/absence of `'pg_catalog'`) — the table's **shape** changes, not just its
length.

---

## U3. Version plumbing into the TUI

### Where the version number lives at each site

One number, two homes, both written in `app.setup()` (`top/top.go:57-84`):

- `app.postgresProps.VersionNum` — field declared `top/top.go:44` (`postgresProps stat.PostgresProperties`),
  assigned at `:81` from `stat.GetPostgresProperties(app.db)` (`:60`).
- `app.config.queryOptions.Version` — field `top/config.go:21`, assigned at `top/top.go:80` from
  `query.NewOptions(props.VersionNum, …)` built at `:67`.

Reachability at each site:

| site | file:line | has | version reachable today? |
|---|---|---|---|
| `switchViewTo` | `top/config_view.go:232-265` | `app *app` | **yes**, both homes, no signature change |
| `menuOpen` | `top/menu.go:116-147` | `config *config` | **yes**, `config.queryOptions.Version` |
| `selectMenuStyle` | `top/menu.go:37-113` | `menuType` only | **no** |
| `menuSelect` | `top/menu.go:150-251` | `app *app` | **yes**, both homes |

So exactly **one** function needs a new parameter for the menu half.

### Minimal signature changes

```go
func tablesNextView(current string, version int) string      // top/config_view.go
func selectMenuStyle(t menuType, version int) menuStyle      // top/menu.go:37
```

`tablesNextView` stays a pure, table-testable function — the `walNextView` shape
(`top/config_view.go:298-310`) plus a guard:

```go
case "tables":
    if version >= query.PostgresV19 { next = "autovacuum_scores" } else { next = "tables" }
```

Call site: `switchViewTo`'s new `case "tables":` (the direct analogue of `case "wal":` at
`top/config_view.go:256-257`, comment at `:250-255` included verbatim — `"tables"` collides with the
`report -T` type at `cmd/report/report.go:66,146`, the `describeReport` key at
`report/report.go:671`, and the tar entry prefix).

`selectMenuStyle` call sites — only **three** in production, all trivially fixable:

- `top/menu.go:118` — inside `menuOpen`, which already has `config` → `config.queryOptions.Version`
- `top/menu.go:248` — the `selectMenuStyle(menuNone)` reset at the end of `menuSelect`; `menuNone`
  falls to the `default` arm at `:106-109`, so any version argument works (pass `0`, or the real
  one for readability)
- tests: `top/menu_test.go:28`, `:76`

The version only changes the **label** of item 1 in `menuTables` (`" pg_stat_autovacuum_scores"` vs
`" pg_stat_autovacuum_scores — requires PostgreSQL 19"`). It must **not** filter the item out: the
user-spec keeps the item present, which is what preserves the `cy` → view mapping `menuSelect`
hardcodes and keeps `Test_menuSelectTables` a straight copy of `Test_menuSelectWAL`.

### The test that breaks silently

`Test_switchViewTo` (`top/config_view_test.go:588-661`) builds `app := &app{config: newConfig()}`
and sets only `app.postgresProps.ExtPGSSSchema` per row (`:638`). **`VersionNum` is zero**, and
`newConfig()` (`top/config.go:52-59`) leaves `queryOptions` zero-valued too. So *whichever* home
`tablesNextView` reads, the load-bearing row `{current:"tables", to:"tables", want:"autovacuum_scores"}`
fails until the table gains a version dimension. Same for `Test_keybindingsWALCycles`'s analogue
(`top/keybindings_test.go:128-145`), which builds the app the same way.

Prefer `app.postgresProps.VersionNum` over `app.config.queryOptions.Version`: it is the field the
other version-dependent handler already uses (`showPgLog(app.db, app.postgresProps.VersionNum, …)`,
`top/keybindings.go:67`).

---

## U4. The menu refusal path

### Composition with the pinned `cy` → view mapping

`menuSelect` (`top/menu.go:150-251`) is a `switch app.config.menu.menuType` whose arms are
`switch cy` with a `default` fall-back. The `menuWAL` arm (`:214-223`) is the template. Because the
spec keeps the marked item **present**, `cy` is untouched:

```go
case menuTables:
    switch cy {
    case 0:
        viewSwitchHandler(app.config, "tables")
        printCmdline(app.ui, "%s", app.config.view.Msg)
    case 1:
        if app.postgresProps.VersionNum >= query.PostgresV19 {
            viewSwitchHandler(app.config, "autovacuum_scores")
            printCmdline(app.ui, "%s", app.config.view.Msg)
        } else {
            printCmdline(app.ui, "NOTICE: pg_stat_autovacuum_scores requires PostgreSQL 19")
        }
    default:
        viewSwitchHandler(app.config, "tables")
        printCmdline(app.ui, "%s", app.config.view.Msg)
    }
```

**The shape is deliberate and the naive version is wrong.** Every existing arm ends with one
shared `printCmdline(app.ui, "%s", app.config.view.Msg)` *after* the inner switch (`:213`, `:223`,
`:203`, `:185`, `:165`). Keeping that shared line and adding a refusal message inside `case 1`
produces **two** `printCmdline` calls on one path — the exact defect class recorded at
`patterns.md:323-330`: `g.Update` enqueues each write from its own goroutine, order is not
guaranteed, so only one survives and which one is a coin flip. The refusal message would
intermittently vanish, which is precisely the acceptance criterion "не закрывает меню молча". Hence
the shared write must be pushed **into** each branch (the 4-branch `switch` idiom
`switchViewToProcPidStat` uses at `top/config_view.go:435-444`).

### What the `menuOpen` NOTICE path gives, and what it does not

`top/menu.go:120-124`:

```go
if pgssSchema == "" && s.menuType == menuPgss {
    printCmdline(g, "NOTICE: pg_stat_statements not found")
    return nil
}
```

**Reusable:** the message *shape* (`NOTICE: ` prefix, one `printCmdline`, no `viewCh` push, no state
mutation) and the principle that a refusal changes nothing.

**Not reusable:** the placement and the `return nil`. That guard runs *before* the menu window is
built, so returning early is correct there. In `menuSelect` the menu window already exists and has
focus; an early `return nil` would skip `app.config.menu = selectMenuStyle(menuNone)` (`:248`) and
`menuClose(g, v)` (`:249`), leaving the menu drawn and the cursor trapped in it. The refusal branch
must **fall through** to both.

Also note the receiver differs: `menuOpen` writes via `g`, `menuSelect` via `app.ui`. Same `*gocui.Gui`
in production; in `Test_menuSelectWAL` (`top/menu_test.go:52-99`) `app.ui` is the zero-value Gui that
`printCmdline`'s `g.Update` goroutine parks on forever — the documented intentional leak
(`top/menu_test.go:44-47`). A refusal test inherits that leak; do not "fix" it.

`viewSwitchHandler` (`top/config_view.go:370-379`) also calls `liftPause`. The refusal skips it,
which is correct and matches the convention documented at `:367-369` ("callers with an early return
of their own keep it ABOVE their call to this helper").

---

## U5. CRITICAL — the diff path across a recorded version change

**Answer: no. `diff()` cannot see a PG-18-width `prev` paired with a PG-19-width `curr` through the
version-change path. The mechanism that prevents it is not [021]'s layout-state reset — it is
older and separate, and it is not a leftover.** But this feature is the first to exercise that
mechanism on a screen with a non-empty `DiffIntvl`, and that is worth stating as such.

### End-to-end trace of `report/report.go: processData`

1. **Every tick carries its own version.** `tarRecorder.collect` (`record/recorder.go:113-146`)
   re-runs `query.SelectCommonProperties` per tick (`:120-126`) and stores it as `stats["meta"]`;
   `write()` emits `meta.<ts>.json` beside `<screen>.<ts>.json`. So a mid-archive version change is
   representable and `readTar` reads it: `report/report.go:182-193` decodes `meta.*` into
   `metadata.version`, and `:233-235` refuses to send a `data` until **both** `metaOK` and `statOK`
   are set, resetting both after each send (`:240`).
2. **The boundary detector.** `report/report.go:276` —
   `versionChanged := prevStat.Valid && prevMeta.version != d.meta.version`.
3. **The drop.** `:277-320` — `if !prevStat.Valid || versionChanged { prevMeta = d.meta;
   prevStat = d.res; prevTs = d.ts; … Configure(…); … continue }`. The **previous snapshot is
   replaced by the first sample of the new version, and that sample is `continue`d — never diffed,
   never printed.** The next sample is therefore PG-19-width `curr` against PG-19-width `prev`.
4. **`prevMeta` is updated only inside that branch.** The normal path at `:381-383` swaps `prevStat`
   and `prevTs` and deliberately leaves `prevMeta` alone — correct, because the version cannot
   change without taking the branch.
5. `countDiff` (`report/report.go:502-511`) → `stat.Compare` → `calculateDelta`
   (`internal/stat/postgres.go:581-603`) → `diff` (`:606-661`). The unguarded index is
   `prev.Values[j][l]` at `:639`, walked over `l < curr.Ncols` (`:633`) — reachable only with a
   mixed-width pair, which step 3 prevents for a version change.

### The single-process upgrade case, checked separately

`record` calls `views.Configure(opts)` **once**, in `app.setup()` (`record/record.go:129`), while
`collect()` opens a **fresh connection every tick** (`record/recorder.go:114`). So a `record -a`
process running across an in-place major upgrade keeps emitting the *PG-18-shaped query text* while
`meta.version` flips to 19. Result: the recorded widths stay constant, `versionChanged` still fires,
one sample is dropped. Safe, at the cost of one tick — the pre-existing behaviour ADR
`[017] The report always discards the first sample` already documents
(`docs/decisions-log.md:1180`).

### What remains open, and it is genuinely [020]

The guarantee is **version-keyed, not width-keyed**. A pair whose widths differ while
`meta.version` is *equal* still reaches `diff()`. Debt [020] (`docs/tech-debt.md:268-292`) records
that both malformed shapes were **reproduced** during 013's audit. This feature does not create
that shape — `Configure` is deterministic per version — but it does make `tables` the first
non-empty-`DiffIntvl` screen whose width is version-dependent, so the class becomes reachable on a
screen where it was previously impossible.

Directly relevant evidence already in the repo:
`Test_app_doReport_errorPathDoesNotHang` (`report/report_test.go:1882-1908`) builds an archive that
widens between two samples of the **same** version and asserts the command returns rather than
hangs — i.e. a same-version width change today produces an *aborted report*, not a crash, because
`printStatSample`'s zero-width guard fires first. That is `activity` (`DiffIntvl{0,0}`); on
`tables` the same archive would reach `diff()` at `internal/stat/postgres.go:639` first.

**Recommendation for the tech-spec:** do **not** carry a `diff()` bounds fix inside this feature —
it is shared code across every screen and [020] explicitly notes `align.SetAlign` is a third unsafe
consumer, so fixing `diff` alone would not close the class. Do carry:

- the 18 → 19 `tables` replay test the acceptance criteria already require (see §U6f) — it is the
  **first** exercise of the version-change branch on a diffed screen, and it is the honest proof
  that the drop works there;
- an update to [020]'s "why deferred" note recording that its reachability now includes
  `tables`/`indexes`.

### The TUI live path — not affected

`Collector` holds one `c.config.VersionNum` for the session
(`internal/stat/stat.go:316`), `Reset()` (`:186-188`) blanks `prevPgStat`/`currPgStat` on a view
switch, and `calculateDelta` is called with the same view's `DiffIntvl` on both snapshots
(`:440`). The version cannot change under a live `top` session.

---

## U6. Test inventory — exact literals

### (a) View registry counts

| file:line | assertion | old → new |
|---|---|---|
| `internal/view/view_test.go:11` | `assert.Equal(t, 28, len(v))` | **28 → 29** |
| `internal/view/view_test.go:314` | `{version: 190000, total: 28}` | **28 → 29** |
| `internal/view/view_test.go:315` | `{version: 160000, total: 28}` | unchanged |
| `internal/view/view_test.go:316` | `{version: 140000, total: 25}` | unchanged |
| `internal/view/view_test.go:317` | `{version: 130000, total: 19}` | unchanged |
| `internal/view/view_test.go:318` | `{version: 120000, total: 16}` | unchanged |
| `internal/view/view_test.go:319` | `{version: 110000, total: 14}` | unchanged |
| `internal/view/view_test.go:320` | `{version: 100000, total: 14}` | unchanged |

Only the `190000` row moves — `MinRequiredVersion: PostgresV19`. Same shape as feature 007's PG15+
view bumping only the `160000` row (`patterns.md:181`).

Also add, per `patterns.md:186-190`, a `TestNew_AutovacuumScoresView` in the shape of
`TestNew_ArchiverView` (`internal/view/view_test.go:102-129`) pinning `key == v.Name`, non-nil
`ColsWidth`/`Filters`, `NotRecordable: true`, `MinRequiredVersion == query.PostgresV19`,
`Ncols == 11`, `DiffIntvl == [2]int{0,0}`, `OrderKey == 1`, `OrderDesc == true`.

### (b) Record filter rows — `record/record_test.go`, all seven

`NotRecordable: true` is dropped unconditionally (`record/record.go:205-212`) **before** the
version gate, so `wantN` rises by one on every row and `wantV` never moves:

| file:line | old | new |
|---|---|---|
| `:142` | `{version: 190000, pgssSchema: "public", wantN: 0, wantV: 28, wantArchiver: true}` | `wantN: 1` |
| `:143` | `{version: 140000, pgssSchema: "", wantN: 9, wantV: 19, …}` | `wantN: 10` |
| `:144` | `{version: 140000, pgssSchema: "public", wantN: 3, wantV: 25, …}` | `wantN: 4` |
| `:145` | `{version: 130000, pgssSchema: "public", wantN: 9, wantV: 19, …}` | `wantN: 10` |
| `:146` | `{version: 120000, pgssSchema: "public", wantN: 12, wantV: 16, …}` | `wantN: 13` |
| `:147` | `{version: 110000, pgssSchema: "public", wantN: 14, wantV: 14, …}` | `wantN: 15` |
| `:148` | `{version: 100000, pgssSchema: "public", wantN: 14, wantV: 14, …}` | `wantN: 15` |

017 added a per-row `wantArchiver bool` because counts alone can be satisfied by an arithmetic
coincidence (`patterns.md:184-186`). Add the same for this feature: a `wantAutovacuumScores bool`
that is **false on every row** — the whole point is that it is never kept. `Test_filterViews` runs
without Postgres.

### (c) `tables` / `indexes` Ncols assertions — there are none today

Grep result: nothing in the repo pins `views["tables"].Ncols` or `views["indexes"].Ncols`, and
`internal/query/tables_test.go` / `indexes_test.go` assert only that `Format` succeeds and
`conn.Exec(q)` does not error (they discard the result set entirely and do not `defer conn.Close()`,
`tables_test.go:29`). **The column change is currently unguarded — assertions must be added, not
updated.** Natural home: the `case 190000:` and `case 140000:` arms of `TestViews_Configure`
(`internal/view/view_test.go:214-256`), where the per-version `Ncols`+`DiffIntvl` pins for
`wal`/`archiver`/`progress_*` already live:

```
190000 arm:  views["tables"].QueryTmpl == query.PgStatTablesPG19,   Ncols 20, DiffIntvl {1,18}
             views["indexes"].QueryTmpl == query.PgStatIndexesPG19, Ncols  7, DiffIntvl {1,5}
140000 arm:  views["tables"].QueryTmpl == query.PgStatTablesDefault,   Ncols 19, DiffIntvl {1,18}
             views["indexes"].QueryTmpl == query.PgStatIndexesDefault, Ncols  6, DiffIntvl {1,5}
```

Unlike the `archiver` asserts in the same arms, these **can** redden on a deleted `Configure` case
(the static entry carries the PG ≤ 18 values), so they are a genuine wiring gate, not a drift guard.
Write that boundary next to the assertion — `patterns.md:98-104` records that mislabelling it is a
repeat mistake.

Negative criteria "`functions` не изменился" / "`sizes` не изменился" have the same home: assert
`views["functions"].Ncols == 8` / `views["sizes"].Ncols == 12` and
`QueryTmpl == query.PgStatFunctionsDefault` / `query.PgTablesSizesDefault` in the `190000` arm.
Today nothing pins them, so the negative criteria are currently untestable as written.

### (d) Menu and view-switch tests

| file:line | what it pins | change |
|---|---|---|
| `top/menu_test.go:13-30` | `Test_selectMenuStyle` — `{menuNone 0, menuDatabases 2, menuPgss 7, menuProgress 6, menuConf 4, menuStatIO 2, menuWAL 2}` | add `{menuTables, 2}`; **all 7 existing rows change call shape** if `selectMenuStyle` gains the version parameter (`:28`) |
| `top/menu_test.go:52-99` | `Test_menuSelectWAL` — `{cy 0 → "wal"}, {cy 1 → "archiver"}, {cy 5 → "wal"}` + `menuNone` reset | copy to `Test_menuSelectTables` with `{0 → tables}, {1 → autovacuum_scores}, {5 → tables}`; needs a **version dimension** and a refusal row (`version 180000, cy 1` → nothing on `viewCh`, view unchanged, menu still closed) |
| `top/menu_test.go:76` | `app.config.menu = selectMenuStyle(menuWAL)` | call-shape change |
| `top/config_view_test.go:588-661` | `Test_switchViewTo`, 30 rows | add 3: `{tables → tables ⇒ autovacuum_scores}` (the only load-bearing one, per the comment at `:625-627`), `{autovacuum_scores → tables ⇒ tables}`, `{activity → tables ⇒ tables}`; **add a version column and set `app.postgresProps.VersionNum` per row** (see §U3) |
| `top/config_view_test.go:695-708` | `Test_walNextView` | shape to copy for `Test_tablesNextView(current, version)`; include out-of-range version probes (`200000`, `180000`, `0`) the way `archiver_test.go:48-52` does |
| `top/config_view_test.go:749-794` | `Test_toggleSysTables`, 6 rows + `activity` no-op | markers `"pg_stat_all"`/`"pg_stat_user"` do not exist in the new query — new rows need `'pg_catalog'` presence/absence |

### (e) Keybinding and help tests

| file:line | what it pins | change |
|---|---|---|
| `top/keybindings.go:50` | `{"sysstat", 't', switchViewTo(app, "tables")}` | unchanged bytes, changed meaning (the `case "tables"` in `switchViewTo`) — exactly 017's `'w'` situation |
| `top/keybindings.go` (new row) | `{"sysstat", 'T', menuOpen(menuTables, app.config, "")}` | `'T'` is free — verified: `keybindingsList` (`:33-103`) has no `'T'` row |
| `top/keybindings_test.go:50-84` | `Test_keybindingsWAL` — double-`DeleteKeybinding` uniqueness for `'W'`, `'w'`, plus `"", "menu", "dialog", "help"` non-claim | copy as `Test_keybindingsTables` for `'T'` / `'t'` |
| `top/keybindings_test.go:99-115` | `Test_keybindingsWALOpensMenu` — runs the bound handler, asserts `menuType`, title string, items slice, and `mv.Buffer()` contains each item | copy; the **items slice literal is version-dependent** now, so this test needs a version too |
| `top/keybindings_test.go:128-145` | `Test_keybindingsWALCycles` | copy; needs `app.postgresProps.VersionNum = query.PostgresV19` |
| `top/help.go:15` | `    s,t,i             's' tables sizes, 't' tables, 'i' indexes.` | `'t' tables,` must be **removed** here — otherwise the marker `"'t' "` matches two lines and `helpEntryLine` (`top/help_test.go:14-30`) fails on ambiguity |
| `top/help.go:20` | `    w,W               'w' pg_stat_wal / pg_stat_archiver switch, 'W' WAL statistics menu.` | the new `t,T` row must go **after** this line — `Test_helpTemplate_walEntry` (`top/help_test.go:118-131`) asserts `entryIdx == statioIdx+1`, i.e. `w,W` sits directly after `j,J` |
| `top/help_test.go:52-82` | `Test_helpTemplate_pauseEntry` — `scrollIdx+1 == entryIdx`, `entryIdx+1 == contIdx`, `lines[contIdx+1]` starts with `"    C,E,R"` | the new row must not land between `[,]` and `C,E,R`; the slot between `w,W` (`:20`) and `S` (`:21`) is free |
| `top/help_test.go:136-143` | `Test_helpTemplate_replicationEntry` uses `helpEntryLine(t, "'s' tables sizes")` for the description column | still resolves after the `'t' tables,` removal |
| `top/help_test.go:161-170` | `strings.Count(helpTemplate, "%") == 1` | no literal `%` in the new row |
| `top/help_test.go:149-153` | `Test_helpTemplate_resetCaveat` — `"pg_stat_io, bgwriter, wal, archiver"` | **unchanged**: `pg_stat_autovacuum_scores` has no counters to reset, and `Q` *does* affect `stats_age` on `tables`/`indexes`, which is the feature's point, not an exception |

Add a `Test_helpTemplate_tablesEntry` in the `Test_helpTemplate_walEntry` shape (marker `"'t' "`,
prefix `"    t,T"`, description pinned word for word, `descColumn` equality with its neighbour).

Untested and therefore a silent doc gap, unchanged from the first pass: `cmd/help.go:163-178`
(report flag documentation). No new report flag here, so nothing to add.

### (f) Report / replay

- **`report/report_test.go:1172-1205` `Test_describeReport`** — no change required. It compares by
  identity, so editing `pgStatTablesDescription` / `pgStatIndexesDescription` stays consistent
  automatically, and `autovacuum_scores` gets no describe entry (`NotRecordable`).
- **Goldens** — `report/testdata/report_tables.golden`, `report_indexes.golden` and the legacy
  `pgcenter.stat.golden.tar` are unaffected: `report` renders from recorded data, never reads
  `view.Ncols` (only two `.Ncols` reads exist in the package, `report/report.go:412` and `:447`,
  both procpidstat/meta-specific), and `DiffIntvl` does not change.
- **The 18 → 19 replay test the AC demands** needs the existing harness generalized. Today
  `buildActivityTar` (`report/report_test.go:1602-1655`) hardcodes the entry name
  `"activity."+ts+".json"` (`:1649`) and `activityReplayConfig` (`report/report_test.go:1705-1713`)
  hardcodes `ReportType: "activity"`. Both need a screen-name parameter. Everything else transfers:
  `runProcessDataOnTar` (`:1660-1702`) drives `processData` **directly** rather than through
  `doReport`, because `doReport` runs it in a goroutine where a panic kills the whole test binary
  (`patterns.md:228-230`); ticks are one second apart so `itv == 1`; **two ticks per version are
  mandatory** because the first of each version is consumed by the version-change branch.
  The three existing version-change tests (`report/report_test.go:1725`, `:1791`, `:1837`) are all
  `activity`, i.e. `DiffIntvl{0,0}` — this feature's test is the first to put a diffed screen
  through that branch.

---

## U7. The `record/recorder_test.go:30` trap — exact mechanism and fix

**Mechanism, step by step:**

1. `Test_tarRecorder` (`record/recorder_test.go:30-57`) connects via `postgres.NewTestConnect()`
   (`:35`), which is `NewTestConnectVersion(170000)` → port 21917 → **PostgreSQL 17**
   (`internal/postgres/testing.go:20-22`, port map `:28-42`).
2. It builds `views := view.New()` at `:39` — the **unfiltered** registry. `filterViews` is never
   called; the test does not mirror `record/record.go: setup()`, which calls it at `record.go:86`
   *before* `Configure` at `:129`.
3. `views.Configure(opts)` at `:41` with `opts.Version == 170000`. The new
   `case "autovacuum_scores"` returns the single PG 19 query text regardless of version (the
   selector is version-independent, like `SelectStatArchiverQuery`), and the format loop at
   `internal/view/view.go:436-443` writes it into `view.Query`.
4. `tc.collect(dbConfig, views)` at `:47` → `tarRecorder.collect` (`record/recorder.go:113-146`)
   loops `for k, v := range views` (`:128`) and does `stat.NewPGresultQuery(db, v.Query)`, returning
   `nil, err` on the **first** failure (`:130-132`). There is no version gate and no per-view error
   tolerance — that is by design (`architecture.md:93-94`: a failing view aborts the whole
   recording).
5. On PG 17 `pg_stat_autovacuum_scores` does not exist → `assert.NoError(t, err)` at `:48` fails
   with a bare `relation "pg_stat_autovacuum_scores" does not exist` that **names no view**, and
   `assert.NotNil(t, stats)` at `:49` fails too. Map iteration order is random, so which other
   views were collected first varies run to run.

**Minimal correct fix — make the test mirror production:**

```go
views := view.New()
_, views = filterViews(props.VersionNum, "public", views)
```

placed at `recorder_test.go:39`, before `views.Configure(opts)`. This is the same call
`record/record.go:86` makes, in the same order, and it drops `autovacuum_scores` twice over
(`NotRecordable` first at `record.go:208`, and the version gate at `:214` would too). The test's
subject is `tarRecorder.collect`, not the registry, so nothing is weakened.

**Rejected alternatives:**

- `t.Skip` — hides a real regression in `collect` for every PG version.
- Filtering by `VersionOK` only — leaves the view in on a PG 19 fixture, where `collect` would then
  record a screen the spec says must never be recorded. `filterViews` is the one function that
  encodes both rules.
- Adding a version gate inside `tarRecorder.collect` — a behaviour change for `pgcenter record`
  (today a failing view aborts the whole recording, deliberately) and needs its own ADR. Do not do
  this inside 018.

Note the sibling test `TestFilterViews_dropsExplicitNotRecordable` (`record/record_test.go`, the
synthetic guard `architecture.md:182` mentions) keeps working, and `architecture.md:182` itself
becomes stale at `/done` time — this feature reintroduces the first production `NotRecordable` view
since feature 008 cleared them.

---

## U8. Locked column-name test — the pattern to copy

Source: `internal/query/archiver_test.go`. Four pieces, all directly transferable:

1. **Package-level version list with a rationale comment** — `:15-18`
   (`archiverVersions = []int{140000, …, 190000}`). Here the list is `[]int{190000}` only, with the
   comment saying why: `MinRequiredVersion: PostgresV19`, the view does not exist below it.
2. **The locked column slice** — `:20-26`:
   ```go
   var archiverColumns = []string{"source", "ready", …, "stats_age"}
   ```
   Comment states *why by name and not by length*: a column inserted mid-layout keeps the count
   right while shifting every index the view/record/report layers depend on.
3. **The server-free structure test** — `Test_StatArchiverQuery_Structure`, `:69-108`. Walks the
   locked slice building the needle `" AS " + col + ","`, with the last element switched to
   `" AS " + col + " FROM"` (`:84-87`), asserts `strings.Index != -1` **first** (presence, because
   `-1` is less than everything and an ordering-only assertion passes on a missing row, `:89-90`)
   and then `assert.Greater(idx, prev)`. Plus targeted `assert.Contains` on the query's one piece of
   real logic and an `assert.NotContains(…, "coalesce")` tied to `DiffIntvl{0,0}`.
   **Adaptation for 018:** the tail needle `" AS analyze_score FROM"` still works if
   `analyze_score` is the last SELECT item and `FROM` follows it — check that when the LEFT JOIN is
   written. The `NotContains "coalesce"` assertion transfers verbatim (nothing is diffed, so nothing
   may be coalesced). Add `assert.Contains(q, "ORDER BY")` for the explicit ordering criterion and
   `assert.NotContains(PgStatAutovacuumScoresDefault, " AS relid")` for "relid отсутствует".
4. **The live assertion** — `Test_StatArchiverQueries`, `:107-129`:
   `assert.Len(descs, wantNcols)` **and** `assert.Equal(t, archiverColumns, descs, "live column
   names must match the locked order")`. Fed by two file-local helpers:
   - `connectArchiverFixture` (`:371-382`) — the hardened skip that **fails** rather than skips when
     the version is missing from the port map (`assert.NotContains(err.Error(), "no test cluster
     port mapping")`), then `t.Skipf`;
   - `runArchiverQuery` (`:388-407`) — returns names + row count + `rows.Err()`, because a server
     error may surface only on drain.

   There is no shared exported "run a query, return column names" helper in the repo; every test
   file carries its own. Follow that — a file-local `runAutovacuumScoresQuery`.

Also copy `assert.NotContains(t, q, "{{", "formatted query must carry no template artifacts")`
(`:117`) — with a `{{if}}` in the template that assertion becomes load-bearing rather than
decorative, and it should be run for **both** `ViewType` values.

---

## U9. The wraparound fixture

### Where it should live: file-local, not `internal/postgres/testing.go`

`internal/postgres/testing.go` carries **no build tag** and links into the released binary
(`architecture.md:237`, and the doc comment at `testing.go:60-63`), which is why `SetupTestRole`
returns an `error` and takes no `*testing.T`. A helper that burns 120 000 transactions would be
compiled into the shipped `pgcenter` — technically harmless, semantically wrong.

`SetupTestRole` is shared because **two** test files need it. The wraparound fixture is needed by
exactly one (`internal/query/autovacuum_scores_test.go`), and the dominant precedent for
single-file fixtures is file-local with a `defer` cleanup:

`internal/query/sizes_test.go:44-54`
```go
_, err = conn.Exec(`CREATE SCHEMA IF NOT EXISTS test_dbo`)
_, err = conn.Exec(`CREATE TABLE IF NOT EXISTS test_dbo.t1hlog (id int)`)
defer func() {
    _, _ = conn.Exec(`DROP TABLE IF EXISTS test_dbo.t1hlog`)
    _, _ = conn.Exec(`DROP SCHEMA IF EXISTS test_dbo`)
}()
```

Note it uses `CREATE … IF NOT EXISTS` (idempotent, like `SetupTestRole`) and ignores cleanup errors.
`testing/fixtures.sql` (193 lines) creates only the databases, extensions and the PL/Perl `pgcenter`
schema — **no tables at all** — so the image is not the place for this either.

### The fixture itself, and what it costs

Measured and recorded in the interview (`…-interview.yml:909-926`): `ALTER TABLE toast_probe SET
(toast.autovacuum_freeze_max_age = 100000)` + a procedure with `COMMIT` in a loop burning 120 000
transactions — 262 ms, yielding **exactly one** flagged row, and it lives in `pg_toast`, i.e. a
system schema. Deleting `OR for_wraparound` from the `WHERE` reddens it. That is a test that can
fail, which is the acceptance criterion the user-spec's risk section demands.

Constraints to write into the tech-spec:

- **`autovacuum_freeze_max_age`'s minimum is 100000** (`pg_settings.min_val`,
  `…-interview.yml:905-907`), so 120 000 burned XIDs is the floor, not a round number.
- **Burning XIDs is not reversible and is cluster-global.** It raises `age(relfrozenxid)` for every
  relation on the PG 19 fixture cluster (port 21919). `DROP TABLE` in the defer removes the probe
  table; the XID consumption stays. On an ephemeral CI container that is fine — the same argument
  `SetupTestRole`'s doc comment makes about never dropping roles (`testing.go:60-63`). On a
  long-lived local cluster it accumulates across runs. Say so.
- **Only `internal/query` tests touch port 21919** in the current suite; `record`/`report`/`stat`
  go through `NewTestConnect()` → 21917. Go runs tests within a package sequentially unless
  `t.Parallel()` is called, and none of these do — but say it, because a future `t.Parallel()` would
  make the burn race with the column-name assertions.
- The procedure with `COMMIT` in a loop requires PG 11+; the test is PG 19-only anyway.
- Use the hardened skip (`connectArchiverFixture` shape) so a missing `190000` port mapping
  **fails** instead of quietly skipping — tech debt [024] (`docs/tech-debt.md:194`) is exactly this
  trap, and `patterns.md:22-26` records that a skip is honest but still green in CI.

---

## U10. Rendering

No Go-side numeric formatting exists for main-table cells: every value arrives as a string produced
by SQL. Path: `alignViewToResult` (`top/stat.go:779-790`) → `align.SetAlign(r, 1000, false)` →
`printDataCell` (`top/stat.go:1222-1239`).

**The width floor.** `align.SetAlign` (`internal/align/align.go:14-79`):

- `valuelen = math.Max(len(value), 1)` (`:32`) — an **empty cell counts as 1**, never 0.
- `colnamelen = math.Max(len(colname), 8)` (`:35`) — the 8-char floor.
- `aligningIsLessThanColname(vlen, cnlen, width)` = `vlen > 0 && vlen <= cnlen && vlen >= width`
  (`:82-84`) → `widthes[colidx] = colnamelen`.

So a column whose values are all shorter than its name is exactly `max(len(name), 8)` wide, and an
all-empty column is exactly that too.

**Applied to the new screen** — every header is already ≥ 8, so the 8-floor never binds:

| column | header len | widest realistic value | fits? |
|---|---|---|---|
| `relation` | 8 | `pg_toast.pg_toast_16532` (23) | widens to 23 |
| `score` | 8 | `12.47` (5) | 8 |
| `do_vacuum` | 9 | `false` (5) | 9 |
| `do_analyze` | 10 | `false` | 10 |
| `for_wraparound` | 14 | `false` | 14 |
| `dead_total` | 10 | 9 digits | 10 |
| `xid_score` … `analyze_score` | 9–13 | `1234.56` | header width |
| `vacuum_insert_score` | 19 | — | 19 |

`round(x, 2)::text` and `bool::text` both produce values that fit inside the header width in every
realistic case; nothing special is needed. `printDataCell` prints `%-*s` at `ColsWidth[i]+2` and
truncates with `~` only when `len(value) > ColsWidth[i]`, returning an error when the width is
`<= 0` (`:1225-1233`).

**Tech debt [035] does *not* bite the new screen.** `alignViewToResult` early-returns once
`Aligned && len(ColsWidth) == r.Ncols` (`top/stat.go:780-782`), so widths freeze on the first frame.
On `autovacuum_scores` every column has a value on the first frame — the six scores and the three
booleans are never NULL, and `dead_total`'s row set coincides with the scores view's
(`…-interview.yml:838`: 119 vs 119), so the outer join in practice never misses.

**It does bite `stats_age`, and worse than on `archiver`.** Entering `tables`/`indexes` on a cluster
that has never reset statistics freezes `stats_age` at `max(len("stats_age"), 8) = 9`. `00:05:12`
(8 chars) fits; `1 day 00:05:12` (14) renders as `1 day 0~`. That is exactly the risk the user-spec
accepts and asks to fold into [035]'s scope — "Проявление: нужен вход на экран при пустой колонке
**и** больше суток на нём" is literally the `> 9 chars` boundary.

**Sorting interaction.** ADR `[013] Empty cells sort last in every comparator mode`
(`docs/decisions-log.md:898`, implemented at `internal/stat/postgres.go:679-700`): the comparator
mode is chosen from the first **non-empty** cell, and a blank orders last in both directions. A
blank `stats_age` therefore sorts last whichever way the user flips it — intended, and worth
naming in the acceptance criteria so it is not filed as a bug.

---

## U11. `report/describe.go`

The two constants and their exact tab geometry (verified with `cat -A`):

**`pgStatTablesDescription`** (`report/describe.go:82-107`). Header
`  column\t\torigin\t\t\tdescription`; origin starts at column 24, description at column 48. The
existing `- heap_hit\t\theap_blks_hit\t\t…` row has the same name length class as `stats_age`, so
copy its tabbing exactly. The new last row, inserted after `- tidx_hit` and before the blank line
preceding `Details:`:

```
- stats_age<TAB><TAB>stats_reset<TAB><TAB>Age of collected statistics in the moment when stats are taken (PG 19+)
```

(`- stats_age` = 11 chars → 2 tabs → col 24; `stats_reset` = 11 chars → 2 tabs → col 48.)

**`pgStatIndexesDescription`** (`report/describe.go:109-122`). Header
`  column\torigin\t\t\t\tdescription`; origin starts at column **16**, description at column 48.
Copy `- read,KiB\tidx_blks_read\t\t\t…`:

```
- stats_age<TAB>stats_reset<TAB><TAB><TAB>Age of collected statistics in the moment when stats are taken (PG 19+)
```

(`- stats_age` = 11 chars → 1 tab → col 16; `stats_reset` = 11 chars, 16→27 → 3 tabs → col 48.)

**The `(PG 19+)` suffix is the established convention** for a version-gated row, not an invention:
`pgStatWALDescription` (`report/describe.go:149`) carries
`- fpi,KiB\twal_fpi_bytes\t\tAmount of WAL generated by full page images, in KiB (PG 19+)`.
`describe` is version-blind — it prints the same text whatever the archive's recorded version — so
the suffix is the only honest way to say "this row is not in every layout".

The wording `Age of collected statistics in the moment when stats are taken` is the project's
verbatim `stats_age` sentence; it appears on seven screens (`describe.go:28, 155, 172, 489, 515,
540, 560`). Do not reword it.

**No `describeReport` map change** (`report/report.go:665-690`) and **no `Test_describeReport` row
change** (`report/report_test.go:1172-1205`): `tables` and `indexes` are already registered and the
test compares by identity. What is missing is a `Test_describeTablesColumnOrder` /
`…IndexesColumnOrder` in the `Test_describeProgressColumnOrder` shape
(`report/report_test.go:1217-1256`) — presence-then-order over `"\n- " + col + "\t"` markers — which
is the only mechanism in the repo that would catch `stats_age` landing anywhere but last.
`autovacuum_scores` needs **no** describe entry at all (`NotRecordable`, no report flag).

---

## U12. Acceptance criteria the code cannot currently deliver

Adversarial pass over the user-spec's "Критерии приёмки".

### 12.1 "Подпись экрана печатается ровно один раз на каждом из двух путей входа"

This is in the **agent-checked** "Навигация" block, but it is not automatable with the current
seams. `printCmdline` writes through `g.Update`, and on the zero-value `&gocui.Gui{}` the spawned
goroutine parks forever on a nil `userEvents` channel (documented at `top/menu_test.go:44-47`), so a
test can neither count calls nor observe the buffer. There is no `io.Writer` seam and no counter.
`patterns.md:295-330` says the invariant is "exactly one `printCmdline` per code path" and treats it
as a **review + stand** rule. The spec's own "Пользователь проверяет" section already lists this
check — the duplicate in the agent block should be moved or restated as "code review + stand", or a
counting seam has to be built (out of scope for this feature).

### 12.2 "После сброса статистики … эта длительность растёт от обновления к обновлению"

Requires calling `pg_stat_reset()` on the shared PG 19 fixture cluster and sleeping ≥ 1 s between
two samples. `pg_stat_reset()` is database-wide and irreversible; the `internal/query` package has
no test isolation from it, and the PG 19 cluster is the one that the new locked-column and
wraparound tests also read. Automating it means either accepting cross-test contamination or
serialising the whole PG 19 subtree. Realistically a stand check — which the spec's user section
already lists. Flag it in the tech-spec rather than letting decomposition discover it.

### 12.3 The `for_wraparound` fixture as an *agent-checked* criterion

Proven to work (§U9), but it is a 120 000-transaction, cluster-mutating fixture on a shared server,
and the spec's own risk section requires it be **shown red** before it counts. That is fine — but
note the second-order trap `patterns.md:93-104` records: name the mutation *and* check **which**
assertion goes red. Deleting `OR for_wraparound` from the `WHERE` must redden the "flagged row is
present in user mode" assertion, not the fixture setup. Reddening because the burn failed or the
probe table was not created proves nothing.

### 12.4 "Запрос содержит явный `ORDER BY score DESC`, так что порядок … детерминирован" — **this one is wrong as written**

If the SELECT list contains `round(s.score, 2) AS score` and the query ends `ORDER BY score DESC`,
PostgreSQL resolves a **bare** `ORDER BY` identifier against the **output** column list first. So
`ORDER BY score DESC` sorts by the *rounded* value — which is exactly the tie set the criterion
exists to break. The stated intent ("десятки неинтересных отношений схлопываются в `0.00`, и их
порядок не должен зависеть от того, как их вернул планировщик") is not achieved.

The fix is one character class: a **qualified** reference is always an input column, so
`ORDER BY s.score DESC` sorts by the raw `double precision`. Then the client-side re-sort —
`PGresult.sort` (`internal/stat/postgres.go:679+`) with `sort.SliceStable`
(`patterns.md:200-202`) — parses the rendered `"0.00"`, finds a hundred exact ties, and preserves
the SQL order inside them. Precedent for an explicit SQL order on a multi-row screen:
`internal/query/replication_slots.go:31` (`ORDER BY "retained,KiB" DESC NULLS LAST` — note *that*
one deliberately orders by the output alias, because the alias is the value).

The acceptance criterion should say "`ORDER BY` on the **unrounded** score", and the locked
structure test should assert the qualified form specifically. Without this the criterion passes
while the behaviour it names does not happen.

### 12.5 "`dead_total` … соединение внешнее, так что ни одна строка очереди не может пропасть"

Correct as long as the **left** side is `pg_stat_autovacuum_scores` *and* `schemaname` in the
`WHERE` clause comes from the left side too. If `schemaname` is taken from the joined
`pg_stat_all_tables`, an unmatched row yields `schemaname IS NULL`, `NOT IN (…)` evaluates to NULL,
and the row is filtered out — the outer join would then silently do the opposite of what the
criterion promises. Per `…-interview.yml:791-793` the scores view carries `relid, schemaname,
relname` itself, so this is achievable; it needs to be stated as a constraint, and the structure
test should assert `FROM pg_stat_autovacuum_scores` is the **outer** relation (the archiver test's
`assert.True(strings.HasSuffix(…, "FROM pg_stat_archiver"))` at `archiver_test.go:104-105` is the
same idea inverted).

### 12.6 Negative criteria "`functions` не изменился" / "`sizes` не изменился"

Nothing in the repo pins either view's `Ncols` or `QueryTmpl` today (§U6c). As written these
criteria are unfalsifiable — a test has to be **added** for them to mean anything. Cheap: two lines
each in the `case 190000:` arm of `TestViews_Configure`.

### 12.7 "На версиях ниже PG 19 колонки `stats_age` на этих экранах нет, и число колонок прежнее"

Testable, and worth noting *how*: `Test_StatTablesQueries` /
`Test_StatIndexesQueries` (`internal/query/tables_test.go:10-31`, `indexes_test.go:10-31`) currently
`conn.Exec(q)` and discard the result set, so they cannot see column counts at all. Both need to be
rewritten to the `runArchiverQuery` tier (`FieldDescriptions()` → names). While rewriting, fix the
two pre-existing defects in place: no `defer conn.Close()` (a failed assertion leaks the connection,
`tables_test.go:29`) and the legacy 12-version list `{90500 … 190000}` where six versions always
skip (tech debt [024]). Use the hardened skip.

### 12.8 "`stats_age` не участвует в вычислении разниц между выборками ни на одной версии"

Testable as an assertion that `DiffIntvl[1] < Ncols-1` on both versions — but note the assertion
that *looks* right and is not: asserting `DiffIntvl == [2]int{1,18}` on PG 19 passes even if
`stats_age` were inserted mid-layout at index 5, because the interval literal would be unchanged.
The load-bearing assertion is on the **column order** (last name is `stats_age`) combined with
`DiffIntvl[1] == Ncols-2`. Say which one is the proof.

### 12.9 "Экран не диффует ни одну колонку, поэтому пустое значение в `dead_total` не может прервать выборку"

Provable directly: `calculateDelta` short-circuits at `internal/stat/postgres.go:591-598` when
`interval == [2]int{0,0}` and returns `curr` untouched, so `diffPair`/`strconv.ParseInt` are never
reached. A unit test that feeds a `PGresult` with an empty `dead_total` cell through
`stat.Compare(curr, prev, 1, [2]int{0,0}, 1, true, 0)` and asserts no error is the honest form —
and it must be shown to **fail** when the interval is changed to a non-zero pair, or it proves
nothing about the screen.

### 12.10 Not in the criteria but required by the design

- The `,` toggle on the new screen (§U2) — four literals in `toggleSysTables`, one of which is a
  slice length that panics if missed. The spec says the toggle works there; no criterion covers it.
- The `Configure` case for `tables`/`indexes` being load-bearing for the `Right`-arrow sort wrap
  (§U1) — invisible to every existing test and to the render path.
- `architecture.md:182` ("no production view sets `NotRecordable` anymore") becomes false with this
  feature and must be corrected at `/done`.
- Tech debt [020]'s "why deferred" note needs updating: its reachability now includes
  `tables`/`indexes`, the first diffed screens whose width is version-dependent (§U5).
