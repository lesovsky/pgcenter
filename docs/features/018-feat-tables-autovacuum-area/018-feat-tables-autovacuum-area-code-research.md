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
