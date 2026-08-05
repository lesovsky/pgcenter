# Code Research — [017] WAL and archiving area

**Date:** 2026-08-05
**Feature base:** `docs/features/017-feat-wal-archiver/017-feat-wal-archiver`
**Branch:** `develop` @ `41fb586`
**Scope researched:** (1) new `archiver` view + `w` cycle / `W` menu, (2) record/report with `-W w|a`,
(3) PG 19 FPI-bytes column on `wal`.

All file:line references are against the tree at `41fb586`. Catalog facts in section B were verified
against **live clusters** started from `lesovsky/pgcenter-testing:0.0.11` (PG 14/17/18/19), not from
release notes.

---

## 1. Entry Points

### 1.1 Query layer

| File | Role |
|---|---|
| `internal/query/wal.go:3-22` | `PgStatWALPG14` (PG 14–17, 11 cols) and `PgStatWALDefault` (PG 18+, 7 cols) query constants. Both are plain strings — no `{{.Template}}` placeholders — but still pass through `query.Format`. |
| `internal/query/wal.go:25-32` | `SelectStatWALQuery(version int) (string, int, [2]int)` — returns `(PgStatWALDefault, 7, {2,5})` for `version >= 180000`, else `(PgStatWALPG14, 11, {2,9})`. Note: uses the numeric literal `180000`, not `PostgresV18`. |
| `internal/query/bgwriter.go:41-52` | `SelectStatBgwriterQuery(version int) (string, int, [2]int)` — the reference three-branch single-row selector; also the reference for **absolute counters placed outside `DiffIntvl`** (`ckpt_timed`/`ckpt_req` at cols 1–2, diffed block starts at col 3). |
| `internal/query/io.go:87-101` | `SelectStatIOQuery` / `SelectStatIOTimeQuery` — the reference for a **version-independent selector that keeps the `version` parameter** (`func SelectStatIOTimeQuery(_ int) (string, int, [2]int)`, `io.go:99`), with the rationale spelled out in the doc comment at `io.go:94-98`. |
| `internal/query/query.go:9-23` | Version constants. **`PostgresV19 = 190000` already exists** (`query.go:22`). |
| `internal/query/overview.go:100-102` | `OverviewArchivingBacklog` — the existing `.ready` backlog aggregate (`pg_ls_dir('pg_wal/archive_status')`), used by the [010] verbose panel. Documents the privilege caveat at `overview.go:97-99`. |

**What a new `internal/query/archiver.go` must provide.**

`pg_stat_archiver` is schema-identical on PG 14, 17, 18 and 19 (verified live, section B), so the
selector needs **no version branch**. The established shape for that case is `io.go:99`:

```go
func SelectStatArchiverQuery(_ int) (string, int, [2]int)
```

Keeping the `int` parameter preserves selector symmetry and lets `Configure()` call it exactly like the
others. Revive requires the unused parameter be named `_` (`patterns.md` "Naming Conventions").

`Ncols`/`DiffIntvl`/`UniqueKey`/`OrderKey` for a single-row view are set in **two places** and must agree:

- static defaults in `internal/view/view.go:New()` (see `wal` at `view.go:129-140`, `bgwriter` at
  `view.go:141-152`) — this is the value the **report** path starts from (`report/report.go:84`);
- runtime override in `internal/view/view.go:Configure()` (`view.go:389-394` for `wal`/`bgwriter`).

Single-row views use `UniqueKey: 0` (the zero value; `wal`/`bgwriter` do not set it at all — see the
field comment at `view.go:20`) and `OrderKey: 0, OrderDesc: true`. Column 0 on both is the constant
`'WAL'`/`'Bgwriter'` literal, which is what makes `UniqueKey=0` work: the single row matches itself
across samples.

**Diff interval for `archiver`.** Per the locked scope every column is cumulative or textual —
`archived_count`, `failed_count` cumulative; two WAL names; three age strings; the `.ready` backlog is a
point-in-time gauge, not a counter. So `DiffIntvl: [2]int{0,0}`, i.e. **nothing is diffed** — the
`activity` / `progress_copy` / `progress_index` idiom (`view.go:43`, `:305`, `:317`).
`calculateDelta` (`internal/stat/postgres.go:589-597`) short-circuits on a `{0,0}` interval and never
enters `diff()`, so every column — including the `'Archiver'` literal at column 0 — is copied
verbatim. See the corrected §7.1.

### 1.2 View registration

`internal/view/view.go:38-361` — `New() Views` returns the static map of **27 views** today. A new
`"archiver"` entry goes here. `MinRequiredVersion`: `pg_stat_archiver` exists since PG 9.0 and
`pg_ls_archive_statusdir()` since PG 12; the project floor for new views has been `query.PostgresV14`
(`wal`, `bgwriter`, `replslots`), and PG 14 is the oldest cluster in the test image — `PostgresV14` is
the consistent choice and keeps `TestView_VersionOK`'s ≤PG13 rows untouched.

`internal/view/view.go:367-427` — `Configure(opts query.Options)`: one `case` per version-aware view in
the `switch k` at `view.go:373-413`, then a second loop (`view.go:417-424`) that runs every view's
`QueryTmpl` through `query.Format`. A new `case "archiver":` goes in the first switch alongside
`case "wal":` (`view.go:389-391`).

### 1.3 TUI hotkeys

| Location | Current binding |
|---|---|
| `top/keybindings.go:38` | `{"sysstat", 'w', switchViewTo(app, "wal")}` — direct, not a cycle. |
| `top/keybindings.go:43` | `{"sysstat", 'j', switchViewTo(app, "statio")}` — cycle key (the `"statio"` string is a *cycle name*, not a view name). |
| `top/keybindings.go:49` | `{"sysstat", 'J', menuOpen(menuStatIO, app.config, "")}` — menu key. |
| `top/help.go:13-19` | Help text; `'w' WAL` at `help.go:14`, the `j,J` line at `help.go:19` is the model for a new `w,W` line. |
| `top/help.go:45` | `'Q' does not reset shared stats: pg_stat_io, bgwriter, wal` — the archiver is likewise not reset by `Q` (out of scope), so this line should gain `archiver`. |

---

## 2. Data Layer

### 2.1 `pg_stat_archiver` — verified live

Identical on PG 14, PG 17, PG 18 and PG 19beta2 (queried via `pg_attribute`):

| # | column | type |
|---|---|---|
| 1 | `archived_count` | `bigint` |
| 2 | `last_archived_wal` | `text` |
| 3 | `last_archived_time` | `timestamp with time zone` |
| 4 | `failed_count` | `bigint` |
| 5 | `last_failed_wal` | `text` |
| 6 | `last_failed_time` | `timestamp with time zone` |
| 7 | `stats_reset` | `timestamp with time zone` |

On an untouched cluster (fixtures, `archive_mode=off`): `archived_count = 0`, `failed_count = 0`
(bigint counters are **never NULL**), `last_archived_wal`/`last_failed_wal`/`last_archived_time`/
`last_failed_time` are **NULL**, `stats_reset` is **NOT NULL**.

### 2.2 `pg_ls_archive_statusdir()` — verified live

- Exists on **PG 14 through PG 19** (added in PG 12), signature `SETOF record`, OUT columns
  `name text, size bigint, modification timestamptz`.
- ACL (identical on PG 14 and PG 19): `{postgres=X/postgres,pg_monitor=X/postgres}` — i.e. superuser
  and `pg_monitor` only. **Same privilege class as `pg_ls_waldir()`**, already used unconditionally by
  the `wal` screen (`internal/query/wal.go:6`, `:16`).
- On the fixtures it returns 0 rows; `count(*) FILTER (...)` over an empty set yields `0`, not NULL.

### 2.3 Candidate archiver query — executed successfully on PG 14 and PG 19

```sql
SELECT 'Archiver' AS source,
 archived_count,
 last_archived_wal,
 date_trunc('seconds', now() - last_archived_time)::text AS archived_age,
 failed_count,
 last_failed_wal,
 date_trunc('seconds', now() - last_failed_time)::text AS failed_age,
 (SELECT count(*) FILTER (WHERE name LIKE '%.ready') FROM pg_ls_archive_statusdir()) AS ready,
 date_trunc('seconds', now() - stats_reset)::text AS stats_age
FROM pg_stat_archiver;
```

Result on the fixtures (both versions):
`Archiver | 0 | (null) | (null) | 0 | (null) | (null) | 0 | 00:00:16`.

That is 9 columns with the `source` literal — the 8 feature columns plus the `source` column every
single-row screen carries (`wal.go:5`, `bgwriter.go:7`). Ncols is therefore **9**, not 8, if the
`source` convention is kept (recommended: `wal` and `bgwriter` both do it, and `UniqueKey=0` depends on
a stable column-0 value).

The `.ready` count is expressed here via `pg_ls_archive_statusdir()`; the existing [010] aggregate uses
`pg_ls_dir('pg_wal/archive_status')` (`internal/query/overview.go:100-102`). Both are pg_monitor-gated;
`pg_ls_archive_statusdir()` is the narrower, purpose-built function and is what the locked scope names.

### 2.4 PG 19 `pg_stat_wal` — verified live

`pg_attribute` on **PostgreSQL 19beta2** (`19~beta2-1.pgdg22.04+1`), port 21919:

| # | column | type |
|---|---|---|
| 1 | `wal_records` | bigint |
| 2 | `wal_fpi` | bigint |
| 3 | `wal_bytes` | numeric |
| 4 | **`wal_fpi_bytes`** | **numeric** |
| 5 | `wal_buffers_full` | bigint |
| 6 | `stats_reset` | timestamptz |

PG 18 has the same list minus `wal_fpi_bytes` (5 columns). PG 17 has 9 columns (adds
`wal_write`/`wal_sync`/`wal_write_time`/`wal_sync_time`, no `wal_fpi_bytes`).

**The exact name is `wal_fpi_bytes`, type `numeric`** — confirmed against a live PG 19 catalog, not
inferred. Caveat to carry into the spec: this is **beta2**; catalog names can still move at beta3/RC, so
a re-verification pass at RC/GA is cheap and warranted (roadmap already flags this).

A PG 19 wal query was executed successfully on port 21919:
`round(wal_fpi_bytes / 1024, 2)` returns e.g. `12928.94` — the same shape as the existing
`round(wal_bytes / 1024, 2) AS "wal,KiB"` (`wal.go:7`, `:17`). Placing it next to `wal,KiB` gives PG 19
an **8-column** wal layout with `DiffIntvl {2,6}` (source 0, waldir_size 1, diffed 2..6 =
wal,KiB / fpi,KiB / records / fpi / buffers_full, stats_age 7) — the exact numbers are a spec decision;
what is fixed is that `stats_age` must stay outside the interval (`wal.go:27` comment) and the new
column must be inside it (per user answer Q3).

### 2.5 `PGresult` and the diff/sort/render path

- `internal/stat/postgres.go:443-450` — `PGresult{Values [][]sql.NullString, Cols, Ncols, Nrows, Valid}`.
  SQL NULL scans as `sql.NullString{String:"", Valid:false}` and every render path prints `.String`
  alone, so a NULL is indistinguishable from an empty string on screen.
- `internal/stat/postgres.go:diff()` — copies columns outside `DiffIntvl` verbatim
  (`.String` and `.Valid`), and for columns inside the interval calls `diffPair(curr, prev, itv)`.
- `diffPair` → `parsePairInt` → `strconv.ParseInt("")` fails on an empty cell and **returns an error
  that aborts the whole sample** (the trap recorded in `patterns.md` and closed by `coalesce(...,0)`
  in `io.go` / `replication_slots.go`).
- `PGresult.sort` rules (`patterns.md` "Sorting", ADR [013]): comparator mode chosen from the **first
  non-empty cell**; an empty cell orders **last** in both directions and all three modes; the sort key
  is bounds-checked.

---

## 3. Similar Features

### 3.1 `pg_stat_io` split — the exact precedent for `w`/`W`

The feature says "modelled on the `j`/`J` precedent". End-to-end, that precedent is five pieces:

1. **Two registered views** — `internal/view/view.go:165-177` (`stat_io`) and `:178-190`
   (`stat_io_time`).
2. **A cycle function** — `top/config_view.go:274-287`:
   ```go
   func statioNextView(current string) string   // stat_io -> stat_io_time -> stat_io; default stat_io
   ```
3. **A dispatch case in `switchViewTo`** — `top/config_view.go:241-252`. The hotkey passes a *cycle
   name* (`"statio"`), the `switch c` maps it to `viewSwitchHandler(app.config, statioNextView(...))`,
   and everything not in the switch falls to `default: viewSwitchHandler(app.config, c)` — which is how
   `'w' → "wal"` works today (`keybindings.go:38`).
4. **A menu type + style + select branch** — `top/menu.go:21` (`menuStatIO` in the iota block),
   `top/menu.go:87-95` (title + 2 items), `top/menu.go:194-203` (`case menuStatIO:` mapping cursor
   index → `viewSwitchHandler`).
5. **A keybinding for the uppercase letter** — `top/keybindings.go:49`.

`viewSwitchHandler` (`top/config_view.go:347-356`) is what both the cycle and the menu funnel into: it
saves the outgoing view back into `config.views`, loads the new one, resets `scrollOffset` and
`autoScrollToOrderKey`, lifts the pause and pushes on `viewCh`. A cycling hotkey needs nothing extra —
`switchViewTo` prints `app.config.view.Msg` afterwards (`config_view.go:254`), so each of the two views'
`Msg` becomes its own cmdline line.

**What a `walNextView` needs (exactly):**

```go
func walNextView(current string) string {
    switch current {
    case "wal":      return "archiver"
    case "archiver": return "wal"
    default:         return "wal"
    }
}
```
plus `case "wal":` (or a new cycle name) in `switchViewTo`'s switch, a `menuWAL` constant, a
`selectMenuStyle` branch with 2 items, a `menuSelect` branch, and `{"sysstat", 'W', menuOpen(menuWAL, app.config, "")}`.

**`W` is free** — `keybindings.go:18-85` has no `'W'` binding. (`-W` in `cmd/profile/profile.go:52` is a
different command's CLI flag and unrelated.)

**Naming caution.** `switchViewTo`'s `default` branch treats `c` as a *view name*. If the cycle name is
literally `"wal"`, then `case "wal": viewSwitchHandler(app.config, walNextView(...))` shadows the
direct-switch semantics that `Test_switchViewTo` currently asserts at `top/config_view_test.go:604`
(`{current: "sizes", to: "wal", want: "wal"}` — still true, because from `sizes` the cycle default
returns `"wal"`) and `:605` (`{current: "wal", to: "replication", ...}` — unaffected). Either name is
workable; the `statio` precedent uses a distinct cycle name, which is the lower-surprise option.

### 3.2 `bgwriter` — the single-row screen shape

`internal/query/bgwriter.go` is the closest structural sibling to a new `archiver.go`: constant
per-version query strings, a selector returning `(query, Ncols, DiffIntvl)`, absolute event counters
placed *before* the diffed block so they render cumulative, `stats_age` last and outside the interval.
It is registered at `view.go:141-152`, configured at `view.go:392-394`, recordable with `report -B`.

### 3.3 `-J c|t` — the exact precedent for `-W w|a`

- Flag definition: `cmd/report/report.go:73`
  ```go
  CommandDefinition.Flags().StringVarP(&opts.showStatIO, "io", "J", "", "show pg_stat_io report (c - count, t - time)")
  ```
- Field: `cmd/report/report.go:28` — `showStatIO string`.
- Mapping: `cmd/report/report.go:157-163`
  ```go
  case opts.showStatIO != "":
      switch opts.showStatIO {
      case "c": return "stat_io"
      case "t": return "stat_io_time"
      }
  ```
  An unrecognised value falls out of the inner switch **and out of the outer switch**, so
  `selectReport` returns `""` → `validate()` errors with "report type is not specified, quit"
  (`cmd/report/report.go:94-96`). Test coverage of that path exists:
  `cmd/report/report_test.go:66` (`{opts: options{showStatIO: "x"}, want: ""}`).
- ADR: `docs/decisions-log.md:446-460` — "report CLI: reuse -X for JIT, one string flag for the two IO
  screens".

---

## 4. Integration Points

### 4.1 Record

- `record/record.go:86` — `filterViews(props.VersionNum, props.ExtPGSSSchema, view.New())`.
- `record/record.go:200-233` — `filterViews`: drops `NotRecordable` views (`:208`), version-incompatible
  views (`:214`), and `statements_*` when pgss is absent (`:221`). **A new recordable view needs no
  change here** — leaving `NotRecordable` at its zero value is sufficient (ADR
  `docs/decisions-log.md:429` "Lift NotRecordable only — pure-SQL views need no recorder change").
- `record/recorder.go:116-153` — `tarRecorder.collect` iterates `for k, v := range views` and stores one
  `PGresult` per view under the view's key (`:135-142`). The tar entry name comes from
  `newFilenameString(ts, name)` (`record/recorder.go:290`), i.e. `archiver.TIMESTAMP.mmm.json`.
  **`collect` returns on the first query error (`:137-139`), aborting the whole recording.**

### 4.2 Report

- `report/report.go:84` — `views := view.New()`, then the requested view is picked by `config.ReportType`.
- `report/report.go:459-476` — `isFilenameOK(name, report)`: accepts an entry when `s[0] == report`
  (or `meta`/`sysinfo`). **No change needed** for a new pure-SQL view — the report type string equals
  the view name equals the tar entry prefix.
- `report/report.go:280-296` — `processData` calls `views.Configure(query.Options{Version: d.meta.version})`
  on the first sample and on every recorded-version change, so a version-aware layout replays correctly
  (this is what makes the PG 19 wal column safe for `report -W w`; see `patterns.md` "Report replay
  across a recorded version change" for the three things that must be reset).
- `report/report.go:665-693` — `describeReport`'s map. A new `"archiver": pgStatArchiverDescription`
  entry goes here next to `"wal"` (`report/report.go:674`).
- `report/describe.go:141` — `pgStatWALDescription`. A new `pgStatArchiverDescription` constant follows
  the same `column / origin / description` table format and ends with a docs URL.
  **`pgStatWALDescription` also needs the PG 19 `fpi,KiB` row added** — it currently documents the
  PG 14 baseline including `write`/`sync` columns removed in PG 18, so it is already
  version-approximate; adding the FPI row keeps it no worse.

### 4.3 GUC access (for the `archive_mode` hint)

Chain, in lockstep — all four must change together:

1. `internal/query/common.go:43-55` — `SelectCommonProperties`, a flat 13-column `SELECT`.
   `archive_mode` is **not** fetched today.
2. `internal/stat/postgres.go:381-400` — `PostgresProperties` struct; would gain
   `GucArchiveMode string`.
3. `internal/stat/postgres.go:403-419` — `GetPostgresProperties`'s single positional
   `.Scan(...)` — the new column must be appended to the query **and** to the `Scan` list in the same
   order.
4. Consumers of `props`: `top/top.go:60` (stored in `app.postgresProps`, `top/top.go:46`),
   `record/record.go:78`, `profile/profile.go:112`. Only `top` needs the new field.

**Backward compatibility of widening `SelectCommonProperties`:** the recorder stores its result as the
`meta.*` tar entry (`record/recorder.go:127-132`) and `report/report.go:readMeta` (`report/report.go:445-457`)
reads only `Values[0][1]` and accepts `Ncols >= 2` — precisely because this query was widened before
(issue #122, `report/report_test.go:344-346`). So adding a column is safe for replay of old archives.
`internal/query/common_test.go:78` executes `SelectCommonProperties` against every mapped version and
will exercise the new column for free.

`archive_mode` exists on every supported version and is `PGC_POSTMASTER`, so
`current_setting('archive_mode')` cannot fail — no version branch needed. Values: `off` / `on` /
`always`; the hint condition is `== "off"`.

### 4.4 The `stat_io_time` "hint" — **it is not GUC-conditional**

Answering D directly, because the assumption in the brief does not hold:

- `internal/view/view.go:188` — `Msg: "Show pg_stat_io timings statistics (requires track_io_timing=on)"`.
- That string is printed unconditionally by `switchViewTo` (`top/config_view.go:254`,
  `printCmdline(g, "%s", app.config.view.Msg)`) and by `menuSelect` (`top/menu.go:203`).
- `internal/view/view_test.go:66` pins it: `assert.Contains(t, statioTime.Msg, "track_io_timing")`.

There is **no** `track_io_timing` GUC read anywhere in the codebase (grep: the only occurrences are the
`Msg` string, describe text, SQL comments and the test). So the existing "hint" is a static screen
subtitle, not a conditional message.

Two implementation options for the archive_mode hint, and they differ materially:

- **(a) Static, copy the precedent exactly** — put "(requires archive_mode=on)" in the `archiver`
  view's `Msg`. Zero new plumbing, zero risk, and it is what the `stat_io_time` precedent actually is.
  But it says "requires", not "your cluster has it off".
- **(b) Conditional on the live GUC** — needs §4.3's four-place change plus a branch in
  `switchViewTo`/`menuSelect`. Feasible: `switchViewTo` already receives `*app` and already has a
  precedent for a conditional cmdline message on a view switch (`config_view.go:235-238`, the
  pgss-unavailable notice). **Constraint:** `printCmdline` must be called **exactly once per code
  path** (`patterns.md` "The cmdline"; `switchViewToProcPidStat`'s 4-branch switch at
  `top/config_view.go:412-421` is the reference) — so the archive_mode branch must *replace* the
  `Msg` print, not precede it. And the menu path (`top/menu.go:194-203`) is a second entry point that
  needs the same treatment or it will silently differ.

The user asked for "a cmdline hint when `archive_mode` is off", which reads as (b). The spec must pick
one explicitly; this research does not.

---

## 5. Existing Tests

Framework: `testify/assert`, table-driven subtests, no mocks/factories. Integration tests connect via
`internal/postgres/testing.go` and `t.Skipf` on unavailable versions.

### 5.1 Count-based tests that break when a view is registered

| Test | File:line | Current value | Effect of adding `archiver` (MinRequiredVersion=PostgresV14) |
|---|---|---|---|
| `TestNew` | `internal/view/view_test.go:9-12` | `assert.Equal(t, 27, len(v))` | → **28** |
| `TestView_VersionOK` | `internal/view/view_test.go:255-277` | rows: `190000:27`, `160000:27`, `140000:24`, `130000:19`, `120000:16`, `110000:14`, `100000:14` | rows at version **≥ 140000** each `+1` → `190000:28`, `160000:28`, `140000:25`. The `130000` and below rows are **unchanged**. |
| `Test_filterViews` | `record/record_test.go:109-152` | `{190000,"public",wantN:0,wantV:27}`, `{140000,"",9,18}`, `{140000,"public",3,24}`, `{130000,"public",8,19}`, `{120000,"public",11,16}`, `{110000,"public",13,14}`, `{100000,"public",13,14}` | `archiver` is recordable, so it is **kept** on ≥PG14 and **dropped by the version gate** on ≤PG13: `wantV +1` on the three ≥PG14 rows (27→28, 18→19, 24→25); `wantN +1` on the four ≤PG13 rows (8→9, 11→12, 13→14, 13→14). `wantN` on the PG14/PG19 rows and `wantV` on the ≤PG13 rows are **unchanged**. |
| `TestFilterViews_dropsExplicitNotRecordable` | `record/record_test.go:180-192` | synthetic view, count-independent | unaffected |

`Test_filterViews` runs **without PostgreSQL** — a stale count is a real failure even though most of the
`record` package skips without a fixture (`patterns.md` "Adding a New View").

### 5.2 `top` tests that break when a menu/cycle entry is added

These **do** run locally without PostgreSQL.

| Test | File:line | Current | Effect |
|---|---|---|---|
| `Test_selectMenuStyle` | `top/menu_test.go:8-25` | `{menuNone:0, menuDatabases:2, menuPgss:7, menuProgress:6, menuConf:4, menuStatIO:2}` | add `{menu: menuWAL, want: 2}` |
| `Test_switchViewTo` | `top/config_view_test.go:588-653` | 26 cases; `wal`-related at `:604` (`sizes→wal→wal`) and `:605` (`wal→replication`); statio cycle at `:621-623` | add `wal→<cycle>→archiver`, `archiver→<cycle>→wal`, `activity→<cycle>→wal`. The existing `:604` case survives only if the cycle default is `"wal"`. |
| `Test_statioNextView` | `top/config_view_test.go:670-683` | 3 cases (`stat_io`, `stat_io_time`, `unknown`) | model for a new `Test_walNextView` with 3 cases |
| `Test_databasesNextView` / `Test_statementsNextView` | `top/config_view_test.go:655`, `:685` | — | unaffected |

Also `internal/view/view_test.go` has one guard test per non-trivial view — `TestNew_StatIOView`
(`:36-49`), `TestNew_StatIOTimeView` (`:54-67`), `TestNew_BgwriterView` (`:87-97`) — pinning
`MinRequiredVersion`, `Ncols`, `DiffIntvl`, `OrderKey`, `UniqueKey`, `NotRecordable` and `Msg`. A
`TestNew_ArchiverView` following that template is the expected addition.

### 5.3 Query selector test style

`internal/query/wal_test.go` is the canonical two-test pair:

```go
func Test_SelectStatWALQuery(t *testing.T)   // wal_test.go:10  — table of {version, wantNcols, wantDiffIntvl}, no DB
func Test_StatWALQueries(t *testing.T)       // wal_test.go:34  — versions := []int{140000,...,190000}, per-version t.Run,
                                             //                   Format() then conn.Exec(q), t.Skipf when unavailable
```

Version matrix lists to extend (`patterns.md` step 2): `internal/query/*_test.go` all carry
`versions := []int{140000, 150000, 160000, 170000, 180000, 190000}`.
`internal/query/common_test.go:64` carries the wider list starting at `90500`.
`internal/view/view_test.go:99-...` (`TestViews_Configure`) carries a per-version × recovery ×
trackCommit matrix starting with the v19 block at `:107-110`.

`Test_SelectStatWALQuery` (`wal_test.go:16-21`) must gain a PG 19 row with the new Ncols/DiffIntvl and
its existing `{version: 190000, wantNcols: 7, wantDiffIntvl: [2]int{2,5}}` row (`:21`) must change.

### 5.4 Report tests

| Test | File:line | What to add |
|---|---|---|
| `Test_selectReport` | `cmd/report/report_test.go:34-72` | `{opts: options{showWAL: true}, want: "wal"}` at **`:46`** becomes `{showWAL: "w"}`; add `{showWAL: "a", want: "archiver"}` and `{showWAL: "x", want: ""}` (mirroring `:66`). |
| `Test_options_validate` | `cmd/report/report_test.go:10-32` | five cases use `showActivity: true`; none use `showWAL` — **unaffected**. |
| `Test_describeReport` | `report/report_test.go:1172-1213` | `{report: "wal", want: pgStatWALDescription}` at `:1184`; add `{report: "archiver", want: pgStatArchiverDescription}`. |
| `Test_app_doReport` | `report/report_test.go:26-...` | case `{ReportType: "wal", wantFile: "testdata/report_wal.golden"}` at `:73-77` — driven by the legacy PG13-era `testdata/pgcenter.stat.golden.tar`. **This golden is not affected by a PG 19 branch** (its meta version is ~13), but confirm on the run. |
| Replay golden test | `report/report_record_bgwriter_test.go:1-...` | The template for a new `report_record_archiver_test.go`: synthetic in-memory tar, two ticks one second apart, meta entry whose `version_num` drives `Configure`, output compared to `testdata/report_record_archiver.golden`. ADR at `docs/decisions-log.md:462-477`. A version-**independent** screen gets one golden (see `report_record_stat_io_time.golden` / `report/report_record_statio_test.go:177-206`); a version-aware one gets one per version (`report_record_bgwriter_pg14/17/18.golden`). |
| PG 19 wal replay | — | The wal report has **no** `report_record_wal_*` golden test today. Adding the PG 19 FPI column is exactly the case that ADR [008]'s per-version goldens exist for; a new `report_record_wal_test.go` with pg18/pg19 goldens is the consistent move. |

---

## 6. Shared Utilities

| Utility | Location | Use here |
|---|---|---|
| `query.Format(tmpl, opts)` | `internal/query/query.go:91-104` | Every view's `QueryTmpl` goes through it (`view.go:417-424`) even with no placeholders. A stray `{{`/`}}` in a new query text is a startup error. |
| `query.NewOptions(...)` | `internal/query/query.go:42-64` | Builds `Options` incl. the recovery-aware WAL function names. `archiver` needs no template variables. |
| `stat.NewPGresultQuery(db, q)` | `internal/stat/postgres.go:453-...` | Sole query→`PGresult` path, shared by TUI and recorder. |
| `stat.Compare(...)` / `countDiff` | `report/report.go:501-512` | Report-side diff; takes `DiffIntvl`, `OrderKey`, `OrderDesc`, `UniqueKey` from the view. |
| `align.SetAlign(r, 1000, false)` | via `alignViewToResult`, `top/stat.go:782-791` | Column widths; floors width at 8 (so no column can be hidden — `patterns.md`). |
| `visibleColumns` | `top/stat.go` | Horizontal window; a 9-column screen is comfortably within reach, no special handling. |
| `printCmdline` / `printCmdlinePersist` | `top/ui.go:527`, `:534` | One call per code path — see §4.4. |
| `formatError(err)` | `top/stat.go:859-870` | Renders a failed sample; `pgconn.PgError` → `SEVERITY: message / DETAIL / HINT`, otherwise `ERROR: <text>`. |
| `postgres.NewTestConnectVersion(v)` | `internal/postgres/testing.go:18-47` | PG14=21914 … PG19=21919; returns an error (→ `t.Skipf`) for unmapped/unavailable versions. |

---

## 7. Potential Problems

### 7.1 `DiffIntvl{0,0}` — NOT a problem (corrected 2026-08-05 by the lead agent)

**The original finding in this section was wrong and has been replaced.** It claimed that with
`DiffIntvl: [2]int{0,0}` column 0 is pushed through `diffPair`, so the `'Archiver'` literal would
abort every sample via `strconv.ParseInt("Archiver")`, and called this the feature's highest risk.

That reading looked at `diff()` in isolation and missed the guard one level up. `diff()` is never
called for a `{0,0}` interval:

```go
// internal/stat/postgres.go:589-597, calculateDelta()
if interval != [2]int{0, 0} {
    delta, err = diff(curr, prev, itv, interval, ukey)
    ...
} else {
    delta = curr        // full pass-through, diff() not entered at all
}
```

Both consumers go through it: the TUI collector (`internal/stat/stat.go:440`, direct
`calculateDelta` call) and the report path (`report/report.go:505` → `stat.Compare` →
`calculateDelta`, `internal/stat/postgres.go:575-577`). The codebase already states this in a comment
at `internal/stat/stat.go:378-379`, describing `procpidstat`'s `DiffIntvl=[0,0]` as making
`calculateDelta()` a pass-through.

**Consequence for this feature:** `DiffIntvl: [2]int{0,0}` on `archiver` means exactly what the
interview settled — nothing is diffed, every column (including the `'Archiver'` literal at column 0
and the NULL age/WAL-name columns) is copied verbatim. No red test is owed for this, and no
diffed-range workaround is needed. §7.2's conclusion is unchanged and now holds unconditionally: the
NULL columns are safe because *no* column is diffed.

The tech-debt item **[020]** claim referenced in §7.7 ("`activity`'s `DiffIntvl {0,0}` means `diff()`
is never the trigger there") is therefore **correct as written** — no update to that entry is owed.

### 7.2 NULLs from an untouched `pg_stat_archiver`

Verified: `last_archived_wal`, `last_failed_wal`, `last_archived_time`, `last_failed_time` are NULL on
a cluster that has never archived, and `date_trunc('seconds', now() - NULL)` is NULL → empty cell.

- **Diff:** safe *only* if those columns stay outside `DiffIntvl` (they do, per §7.1's resolution).
  `patterns.md` records the class: NULLs in diffed columns abort the sample at `strconv.ParseInt("")`.
- **Sort:** an empty cell orders **last** in both directions (ADR [013]) — correct behaviour, and on a
  1-row screen invisible. But the comparator mode is chosen from the first non-empty cell; with one row
  and an empty cell, the column has no non-empty cell at all. Not a crash, but worth a test.
- **Align:** `align.SetAlign` floors at width 8, and the column names (`last_archived_wal` etc.) are
  longer than any empty value, so no width surprise.
- **Deliberate choice to record:** do NOT `coalesce` these to `'-'` or `0`. Column 3.2 of
  `patterns.md` and ADR [013] both argue that a blank is the honest rendering for "never happened";
  `coalesce(...,0)` is prescribed only for **diffed** columns.

### 7.3 Privileges — how a failing stats query surfaces (answers G)

`pg_ls_archive_statusdir()` is superuser/`pg_monitor` only (§2.2). Behaviour when the role lacks it:

- **TUI:** `stat.Collector.Update` → `collectPostgresStat` (`internal/stat/stat.go:322-325`) returns the
  error; `top/stat.go:68` sets `stats.Error = err`; `printDbstat` (`top/stat.go:793-802`) **replaces the
  whole table area** with `formatError(s.Error)` — for a pgx error that renders as
  `ERROR: permission denied for function pg_ls_archive_statusdir` plus `DETAIL:`/`HINT:` lines. The
  summary panels keep updating and the next tick retries; there is no crash and no error latch.
- **`pgcenter record`:** `record/recorder.go:136-139` returns the error from the views loop →
  `app.record` returns → **the whole recording stops**, not just the archiver entry.
- **Precedent, not a regression:** the `wal` screen already calls `pg_ls_waldir()` unconditionally
  (`internal/query/wal.go:6`, `:16`), same ACL class, and `record` has always aborted on it. The
  [010] verbose panel took the opposite route (own `QueryRow`, degrade to `n/a`,
  `internal/query/overview.go:97-99`, ADR `docs/decisions-log.md:654`), but that pattern belongs to the
  free-form panel renderer, not to the `PGresult` table path — there is no per-column degradation
  mechanism for a stats screen.

The spec should state explicitly that the archiver screen inherits the `wal` screen's
all-or-nothing privilege behaviour, since the roadmap's "do not make an incident worse" principle was
already invoked once in this interview (Q5) and the answer was "accept as-is".

### 7.4 Cost of the unthrottled `.ready` listing

Locked as accepted (interview Q5, option A). Worth stating the exposure in the spec anyway: the screen
is opened precisely when `archive_status/` holds a large backlog, and the directory listing runs on
every tick alongside the `wal` screen's `pg_ls_waldir()`. Existing precedent: `pg_ls_waldir()` on the
`wal` screen and `pg_ls_dir('pg_wal/archive_status')` in the [010] verbose panel, both unthrottled.

### 7.5 Breaking `-W` bool → string

`showWAL` references, complete (grep over the tree, excluding docs):

| File:line | Current |
|---|---|
| `cmd/report/report.go:25` | `showWAL bool   // Show stats from pg_stat_wal` |
| `cmd/report/report.go:70` | `BoolVarP(&opts.showWAL, "wal", "W", false, "show pg_stat_wal report")` |
| `cmd/report/report.go:151-152` | `case opts.showWAL: return "wal"` |
| `cmd/report/report_test.go:46` | `{opts: options{showWAL: true}, want: "wal"}` |

That is **all four**. After the change: `showWAL string`, `StringVarP(..., "wal", "W", "", "show pg_stat_wal / pg_stat_archiver report (w - wal, a - archiver)")`, and
`case opts.showWAL != "": switch { case "w": "wal"; case "a": "archiver" }` placed where the current
bool case sits (`report.go:151`) — note the **order of cases in `selectReport` is significant**: it is a
`switch { case ... }` chain, first match wins, so `-A` beats `-W` today and will continue to.

Silent-failure risk to call out: a script using bare `pgcenter report -W -f x.tar` will now consume
`-f` as `-W`'s value (`showWAL = "-f"`) → `selectReport` returns `""` → "report type is not specified,
quit". That is a loud failure, which is the good outcome; the bad variant (cobra `NoOptDefVal`) was
explicitly rejected in interview Q2.

### 7.6 PG 19 wal layout change ripples

Adding `wal_fpi_bytes` changes the PG 19 `wal` layout relative to PG 18 (7 → 8 columns). Consequences:

- `SelectStatWALQuery` gains a third branch; `wal.go:26` currently tests `version >= 180000` with a
  numeric literal — the new branch should use `PostgresV19` (and the existing one is a candidate for
  `PostgresV18`, though changing it is out of scope).
- `report -W w` replay across a recorded-version change is handled by `processData`
  (`report/report.go:280-296`) — but `patterns.md` "Report replay across a recorded version change"
  lists three things that must reset (alignment flag, header counter, **resolved sort column**), all
  already implemented at `report/report.go:296-...`. Nothing new to build; a golden test at both
  versions is the verification.
- `report/describe.go:141` `pgStatWALDescription` needs the new row.

### 7.7 Registered tech debt touching this feature's area

| Item | `docs/tech-debt.md` | Relevance | Suggested handling |
|---|---|---|---|
| **[020]** Diff loop indexes prev snapshot by curr's width | `:233-258`, Low | Directly adjacent: the feature adds a screen and changes a diffed layout. The item explicitly notes `activity`'s `DiffIntvl {0,0}` means `diff()` is never the trigger there — a claim §7.1 shows needs re-reading for a screen whose column 0 is not numeric. | Do **not** attempt to close. Re-read the [020] entry while settling §7.1; if §7.1's investigation contradicts it, update the debt entry. |
| **[019]** Nine tests skip every version when one cluster is unavailable | `:217-231`, Low | `internal/query/common_test.go` is on the list, and §4.3 adds a column to `SelectCommonProperties` that this test is the only executor of. | Out of scope to fix; be aware the new column's live verification may silently skip. Run the container matrix explicitly. |
| **[024]** Test port map promises clusters the image does not contain | `:159-183`, Low | New selector tests will list PG 14–19 only, all of which do exist. | No action. |
| **[017]** Beta apt channel after PG 19 GA | `:186-199`, Low | The `wal_fpi_bytes` name was verified against **beta2** from that channel. | Re-verify at RC/GA; note it in the spec's risks. |
| **[027]** Messages after a dialog closes are never visible / **[028]** verbose height-guard hint loses a race | `:10-45`, Low | Only if the archive_mode hint is made conditional (§4.4b) — a hint that loses a race to `collecting...` is a known class here. | Place the hint on the view-switch path (which owns its cmdline write), not on a render path. |
| **[029]** Row values reach the terminal unsanitised | `:47-65`, Low | `last_archived_wal`/`last_failed_wal` are server-supplied text reaching the table. WAL filenames are hex, so exposure is nil in practice. | No action; note only. |

### 7.8 ADRs that constrain the implementation

From `docs/decisions-log.md` — settled, do not re-open:

- `:349` **[006] Split one wide stats view into two registered sub-views** — the `stat_io`/`stat_io_time`
  precedent this feature copies for `w`/`W`.
- `:429` **[008] Lift `NotRecordable` only — pure-SQL views need no recorder change** — the archiver is
  pure SQL, so record support is "do nothing extra".
- `:446` **[008] report CLI: one string flag for two screens** — `-W w|a` is the same idiom.
- `:462` **[008] Replay tests: synthetic in-memory tar + golden files** — the required test shape.
- `:654` **[010] Archiving backlog via `count(.ready) × wal_segment_size`** — an existing, *different*
  rendering of the same signal (bytes, in the verbose panel, degrading to `n/a`). The archiver screen
  shows a **count**, not bytes. Two renderings of one signal is a deliberate product choice, but the
  spec should say so, because a reader will notice the divergence.
- `:801` **[012] Progress screens: new columns mid-layout, version-aware DiffIntvl** — the precedent
  for the PG 19 wal column insertion (columns inserted mid-layout shift the diffed pair).
- `:898` **[013] Empty cells sort last in every comparator mode** — governs the NULL age columns.

---

## 8. Constraints & Infrastructure

- **Go 1.25+**, cobra, pgx/v5 (`QueryExecModeSimpleProtocol`), gocui, testify.
- **Build/test:** `make build`, `make test` (race, `-p 1`, timeout 300s), `make lint`
  (golangci-lint + gosec), `make vuln`. `golangci-lint` lives in `$(go env GOPATH)/bin` and is not on
  the default PATH.
- **Lint config** `.golangci.yml`: errcheck, gocritic, gosimple, govet, ineffassign, revive,
  staticcheck, unused. Revive is what forces `_` for the unused `version` parameter in a
  version-independent selector.
- **Test clusters:** `make test` needs PG 14–19 on ports 21914–21919. Locally they do not exist; the
  full run reproduces inside the project CI image:

  ```bash
  docker run --rm -v "$PWD":/work -v "$HOME/go/pkg/mod":/gomod \
    -e GOROOT=/gomod/golang.org/toolchain@v0.0.1-go1.25.12.linux-amd64 \
    -e GOPATH=/gopath -e GOFLAGS=-mod=mod -e HOME=/root \
    -w /work lesovsky/pgcenter-testing:0.0.11 bash -c '
      export PATH=$GOROOT/bin:$PATH
      prepare-test-environment.sh > /tmp/prep.log 2>&1 || { tail -30 /tmp/prep.log; exit 1; }
      go test -race -p 1 -timeout 300s ./...'
  ```
  (`lesovsky/pgcenter-testing:0.0.11` is present locally; 0.0.8/0.0.9/0.0.10 also present.)

- **Test fixtures do NOT enable archiving.** `testing/prepare-test-environment.sh:17-33` writes
  `postgresql.auto.conf` with `listen_addresses`, `port`, `shared_buffers`, `ssl*`,
  `logging_collector`, `log_directory`, `log_filename`, `track_io_timing=on`, `track_functions=all`,
  `shared_preload_libraries='pg_stat_statements'`, `wal_level=logical`. **No `archive_mode`, no
  `archive_command`.** Verified live: `SHOW archive_mode` → `off`, `SHOW archive_command` →
  `(disabled)` on PG 14/17/18/19.

  Consequence: integration tests can prove the archiver query **executes and returns one row**, and can
  prove NULL handling on the empty case, but **cannot** exercise a non-zero `archived_count`,
  `last_archived_wal`, or a non-empty `.ready` backlog. The interview (Q4) already settles this: the
  behavioural verification is a manual stand run with `archive_command=/bin/true` then `/bin/false`.
  Whether to also add `archive_mode=on` to the test image is a spec decision — it would require an
  image bump (0.0.12) and a `pg_createcluster` archive directory, and would change what every existing
  test sees. Recommendation: do **not** change the image; keep the stand run as the behavioural gate.

- **PG 19 is beta2** in the image (`testing/Dockerfile:26`, `jammy-pgdg-testing 19` channel). Catalog
  names may move before GA.
- **Manual TUI verification:** `make build` first, always (`patterns.md` "Manual Testing / QA Phase").
  Stand address is ephemeral and must be asked for. Drive with tmux
  (`tmux new-session -d -s cap -x 190 -y 52`, `send-keys`, `capture-pane -p` / `-p -e`), and bring a
  `master`-built binary for A/B.
- **Git:** work on `develop`, PR to `master`, squash merge, single `Co-Authored-By` trailer.

---

## 9. External Libraries

No new dependency. The only external surface is PostgreSQL's own catalog, verified live above.
Context7 was not used — the relevant contract is the PG 19 system catalog, and a live cluster is a
strictly better source than documentation for that.

---

## Appendix — direct answers to the research questions

**A.** `internal/query/wal.go` = two query constants + `SelectStatWALQuery(version) (string,int,[2]int)`
(`:25`), wired at `internal/view/view.go:389-391`. A new `archiver.go` needs **no version branch**
(schema identical PG 14→19, verified) — use the `SelectStatIOTimeQuery(_ int)` signature form
(`io.go:99`). Layout fields live twice: static in `view.New()` (`view.go:129-152` for wal/bgwriter) and
overridden in `Configure()`.

**B.** **`wal_fpi_bytes`, type `numeric`**, attnum 4, verified against a live PG 19beta2 catalog
(port 21919 in `lesovsky/pgcenter-testing:0.0.11`), and a `round(wal_fpi_bytes/1024,2)` query was
executed successfully against it. **`PostgresV19 = 190000` already exists** at
`internal/query/query.go:22`. Caveat: beta2, re-verify at RC/GA.

**C.** `internal/view/view_test.go:9` (`TestNew`, 27→28), `internal/view/view_test.go:255`
(`TestView_VersionOK`, +1 on the ≥140000 rows only), `record/record_test.go:109` (`Test_filterViews`,
`wantV+1` on ≥PG14 rows, `wantN+1` on ≤PG13 rows), `top/menu_test.go:8` (`Test_selectMenuStyle`, add a
2-item row), `top/config_view_test.go:588` (`Test_switchViewTo`), `top/config_view_test.go:670`
(`Test_statioNextView` — the template for `Test_walNextView`). Exact current numbers in §5.1/§5.2.

**D.** `statioNextView` at `top/config_view.go:274-287`, dispatched from `switchViewTo`'s
`case "statio":` (`:249`), funnelling into `viewSwitchHandler` (`:347`). `menuStatIO` at
`top/menu.go:21` / `:87-95` / `:194-203`, opened by `top/keybindings.go:49`. **The `track_io_timing`
"hint" is a static `view.Msg` string (`internal/view/view.go:188`), not a GUC-conditional message — no
GUC is read anywhere.** See §4.4 for the two options and the one-`printCmdline`-per-path constraint.

**E.** §4.1/§4.2/§7.5. `filterViews` needs **no** change; `describeReport` map at
`report/report.go:665-693`; description constants at `report/describe.go` (`pgStatWALDescription:141`);
`-J c|t` parsing at `cmd/report/report.go:73` + `:157-163`. `showWAL` has exactly **four** references,
listed in §7.5.

**F.** `SelectCommonProperties` at `internal/query/common.go:43-55` (13 columns) →
`PostgresProperties` at `internal/stat/postgres.go:381-400` → `GetPostgresProperties`'s positional
`.Scan` at `:405-419` → `top/top.go:60` / `app.postgresProps` (`top/top.go:46`).
**`archive_mode` is NOT fetched today**; adding it is a 3-place lockstep change plus the one consumer.
Widening the query is safe for report replay (`readMeta` accepts `Ncols >= 2`,
`report/report.go:445-457`).

**G.** §7.3. TUI: the dbstat table area is replaced by `formatError(s.Error)`
(`top/stat.go:793-802`, `:859-870`), retried next tick. `record`: the whole recording aborts
(`record/recorder.go:136-139`). Same class as the `pg_ls_waldir()` already used unconditionally by
`wal`.

**H.** §5.3 and §8. Selector test style: `internal/query/wal_test.go:10` (table, no DB) +
`:34` (per-version `t.Run` + `Exec` + `t.Skipf`). Version lists: `[]int{140000..190000}` in
`internal/query/*_test.go`, the wider `90500..190000` list in `common_test.go:64`, the
version×recovery×trackCommit matrix in `internal/view/view_test.go:99`. Helpers:
`internal/postgres/testing.go:18-47`. **Fixtures have `archive_mode=off` and no `archive_command`** —
verified live on PG 14/17/18/19, and `testing/prepare-test-environment.sh:17-33` confirms it is never
set.

**I.** §7. **Correction:** the originally-reported highest risk (§7.1, `DiffIntvl{0,0}` routing column
0 through `diffPair`) does not exist — `calculateDelta` short-circuits on a `{0,0}` interval and never
calls `diff()`. See the rewritten §7.1. Remaining items, none of them blocking: §7.2 (NULLs — safe,
and do not `coalesce` them away), §7.3 (privileges: all-or-nothing, same as the `wal` screen),
§7.5 (breaking flag), §7.6 (PG 19 layout ripple), §7.7 (tech debt [019], [017]; [020] needs no update).
