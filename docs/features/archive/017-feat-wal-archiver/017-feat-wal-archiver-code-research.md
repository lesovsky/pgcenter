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

### 2.3 Archiver query in the LOCKED column order — executed on PG 14 / 17 / 18 / 19

Superseded 2026-08-05: the approved user-spec locks the column order as
`source, ready, archived, last_archived, archived_age, failed, last_failed, failed_age, stats_age`
(`ready` **second**, not eighth as in the first draft below). The locked form was executed against all
four live clusters:

```sql
SELECT 'Archiver' AS source,
 (SELECT count(*) FILTER (WHERE name LIKE '%.ready') FROM pg_ls_archive_statusdir()) AS ready,
 archived_count AS archived,
 last_archived_wal AS last_archived,
 date_trunc('seconds', now() - last_archived_time)::text AS archived_age,
 failed_count AS failed,
 last_failed_wal AS last_failed,
 date_trunc('seconds', now() - last_failed_time)::text AS failed_age,
 date_trunc('seconds', now() - stats_reset)::text AS stats_age
FROM pg_stat_archiver;
```

Result on the fixtures, identical shape on PG 14.23 / 17.10 / 18.4 / 19beta2:

```
  source  | ready | archived | last_archived | archived_age | failed | last_failed | failed_age | stats_age
----------+-------+----------+---------------+--------------+--------+-------------+------------+-----------
 Archiver |     0 |        0 |               |              |      0 |             |            | 00:00:15
```

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

### 4.3 GUC access (for the `archive_mode` hint) — **OUT OF SCOPE, kept for the record**

Superseded 2026-08-05 by the approved user-spec: "Мы решили **не** делать условную подсказку про
`archive_mode` в командной строке … требование вынесено в постоянную подпись экрана". The hint is a
**static `view.Msg` string** — option (a) of §4.4. **None of the four changes below is in scope**:
`SelectCommonProperties`, `PostgresProperties`, `GetPostgresProperties` and `app.postgresProps` stay
untouched, and no `archive_mode` GUC is read anywhere. The chain is documented only so a later feature
does not have to rediscover it.

Chain, in lockstep — all four would have to change together:

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

The user asked for "a cmdline hint when `archive_mode` is off", which reads as (b). **Settled
2026-08-05: the approved user-spec picks (a)** — a static
`Msg: "Show archiver statistics (requires archive_mode=on)"`, printed on every switch regardless of the
cluster's actual `archive_mode`, with the spec stating plainly that it is not a state indicator. Option
(b) and everything in §4.3 are out of scope.

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

**F.** *(out of scope — see §4.3/§4.4: the spec settled on a static `Msg`, no GUC is read.)*
`SelectCommonProperties` at `internal/query/common.go:43-55` (13 columns) →
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

---

## 10. Implementation-level detail (tech-spec pass)

## Updated: 2026-08-05

Written against `develop` @ `41fb586`, after the user-spec was approved. Everything below is at
file:line and, where SQL is involved, was **executed on live clusters** from
`lesovsky/pgcenter-testing:0.0.11` (PG 14.23 / 17.10 / 18.4 / 19beta2 on ports 21914/21917/21918/21919).
Sections A–I answer the tech-spec's questions in order.

---

### 10.A Piece (4) — verbose-panel archiving backlog: exact edit sites

#### 10.A.1 The single production edit

`internal/query/overview.go:100-102`, current text **verbatim**:

```go
	OverviewArchivingBacklog = "SELECT " +
		"count(*) FILTER (WHERE name LIKE '%.ready') * pg_size_bytes(current_setting('wal_segment_size')) AS backlog " +
		"FROM pg_ls_dir('pg_wal/archive_status') AS name"
```

Replacement that keeps the **identical output contract** (single bigint column, bytes,
`count(.ready) × wal_segment_size`):

```go
	OverviewArchivingBacklog = "SELECT " +
		"count(*) FILTER (WHERE name LIKE '%.ready') * pg_size_bytes(current_setting('wal_segment_size')) AS backlog " +
		"FROM pg_ls_archive_statusdir()"
```

Only the `FROM` clause changes. Two mechanical points the tech-spec must state:

1. **The `AS name` alias must be dropped.** `pg_ls_dir(text)` returns `SETOF text` with an *unnamed*
   column, so `AS name` was doing double duty — aliasing the function alias *and* supplying the column
   name that `FILTER (WHERE name LIKE …)` resolves against. `pg_ls_archive_statusdir()` is
   `SETOF record` with OUT parameters `name text, size bigint, modification timestamptz`, so it already
   provides `name`. Keeping `AS name` would rename the whole relation and `name` would fail to resolve
   (`column "name" does not exist`).
2. **Type and NULL-ness are unchanged.** `count(*)` is `bigint`, `pg_size_bytes()` is `bigint`, product
   is `bigint`; over an empty set `count(*)` is `0`, never NULL. The consumer's
   `sql.NullInt64` scan (`internal/stat/postgres.go:291-295`) needs no change.

The doc comment above the constant (`internal/query/overview.go:92-99`) says "replacing `pg_ls_waldir()`
with `pg_ls_dir('pg_wal/archive_status')`" and "`pg_ls_dir` requires pg_monitor/superuser" — **both
sentences are now wrong** and must be rewritten in the same edit. `pg_ls_dir` is superuser-only;
`pg_ls_archive_statusdir()` is superuser + `pg_monitor`. That wrong sentence is exactly the bug this
piece fixes.

#### 10.A.2 Live verification of the premise (run today, PG 14 / 18 / 19)

A role with only `GRANT pg_monitor` (no superuser):

| version | old query (`pg_ls_dir`) | new query (`pg_ls_archive_statusdir()`) |
|---|---|---|
| 14.23 | `ERROR: permission denied for function pg_ls_dir` | `0` |
| 18.4 | `ERROR: permission denied for function pg_ls_dir` | `0` |
| 19beta2 | `ERROR: permission denied for function pg_ls_dir` | `0` |

`pg_proc.proacl` on all three (both `pg_ls_dir` overloads):

```
pg_ls_dir                |{postgres=X/postgres}
pg_ls_dir                |{postgres=X/postgres}
pg_ls_waldir             |{postgres=X/postgres,pg_monitor=X/postgres}
pg_ls_archive_statusdir  |{postgres=X/postgres,pg_monitor=X/postgres}
```

A role with **neither** superuser nor `pg_monitor` gets
`ERROR: permission denied for function pg_ls_archive_statusdir` — so the new function is not a
privilege downgrade.

#### 10.A.3 The consumer — no change needed

`internal/stat/postgres.go:288-295`:

```go
	// replication: archiving backlog. OWN QueryRow: pg_ls_dir requires pg_monitor/superuser; ...
	var backlog sql.NullInt64
	if err := db.QueryRow(query.OverviewArchivingBacklog).Scan(&backlog); err == nil && backlog.Valid {
		s.ArchivingBacklog = backlog.Int64
		s.ArchivingBacklogValid = true
	}
```

The own-`QueryRow` + swallow-error + `…Valid` degradation path is **untouched** — same single scalar,
same `sql.NullInt64`, same `err == nil && backlog.Valid` gate. Only the comment's `pg_ls_dir` mention
needs updating. The renderer (`top/stat.go:722-723`, `pretty.SizeWidth(…, sizeFieldWidth)` under
`if o.ArchivingBacklogValid`) is not touched at all.

#### 10.A.4 Tests that pin the current text or behaviour

| Test | File:line | What it does | Effect |
|---|---|---|---|
| `Test_ArchivingBacklogQuery_Degrades` | `internal/query/overview_test.go:123-146` | runs `OverviewArchivingBacklog` over `overviewVersions` (`overview_test.go:13` = `{140000…190000}`), accepts either `>= 0` or an error | **passes unchanged**; its comment at `:124-125` names `pg_ls_dir` and must be reworded |
| `Test_collectOverviewStat_Degradation` | `internal/stat/postgres_test.go:206-236` | `if got.ArchivingBacklogValid { assert.GreaterOrEqual(got.ArchivingBacklog, 0) }` (`:227-229`) | **passes unchanged**; comment at `:224` says "the fixtures role has pg_monitor" — actually the fixtures role is `postgres` (superuser); harmless either way |
| `top/stat_test.go:578, 627, 687, 742, 840-863` | verbose-panel render tests | feed `ArchivingBacklog`/`ArchivingBacklogValid` **directly into the struct**, never through SQL | **unaffected** |

No golden file contains the SQL text; nothing pins the string itself. This is the cheapest of the four
pieces.

#### 10.A.5 One real behavioural difference the spec does not mention

`pg_ls_dir('pg_wal/archive_status')` (1-argument form) is `missing_ok = false`;
`pg_ls_archive_statusdir()` is `missing_ok = true`. **Verified live** on PG 18.4 by renaming
`$PGDATA/pg_wal/archive_status` away:

```
-- OLD pg_ls_dir with dir missing --
ERROR:  could not open directory "pg_wal/archive_status": No such file or directory
-- NEW pg_ls_archive_statusdir with dir missing --
0
```

So the panel's degradation surface shrinks by one case: a missing `archive_status/` used to render
`n/a` and will now render a real `0 B`. `initdb` always creates the directory, so the case is
pathological, but the user-spec's "Механизм деградации до `n/a` при любой другой ошибке сохраняется без
изменений" is not literally exhaustive. See §10.I.4.

---

### 10.B Piece (3) — PG 19 `fpi,KiB` on the `wal` screen: exact insertion points

#### 10.B.1 `internal/query/wal.go` — new constant

Insert after `PgStatWALDefault` (which ends at `wal.go:21`), inside the same `const (…)` block. Text,
modelled byte-for-byte on `PgStatWALDefault` (`wal.go:15-21`) — note the **double space** in
`AS waldir_size  FROM`, copied deliberately so the rendered header does not change:

```go
	// PgStatWALPG19 defines query for pg_stat_wal (PG 19+).
	// wal_fpi_bytes added in PG 19; placed right after the wal_fpi count so the number of full
	// page images and the volume they cost sit next to each other, inside the diffed range.
	PgStatWALPG19 = "SELECT 'WAL' AS source, " +
		"(SELECT pg_size_pretty(count(1) * pg_size_bytes(current_setting('wal_segment_size'))) AS waldir_size  FROM pg_ls_waldir()) AS waldir_size, " +
		`round(wal_bytes / 1024, 2) AS "wal,KiB", ` +
		"wal_records AS records, wal_fpi AS fpi, " +
		`round(wal_fpi_bytes / 1024, 2) AS "fpi,KiB", ` +
		"wal_buffers_full AS buffers_full, " +
		"date_trunc('seconds', now() - stats_reset)::text AS stats_age " +
		"FROM pg_stat_wal"
```

**Executed on PG 19beta2 (port 21919) today:**

```
 source | waldir_size | wal,KiB  | records | fpi  | fpi,KiB  | buffers_full | stats_age
--------+-------------+----------+---------+------+----------+--------------+-----------
 WAL    | 32 MB       | 17063.86 |   33220 | 2746 | 12928.94 |          947 | 00:00:18
```

8 columns, in the order the user-spec locks.

#### 10.B.2 `internal/query/wal.go:25-32` — selector

Current, verbatim:

```go
func SelectStatWALQuery(version int) (string, int, [2]int) {
	if version >= 180000 {
		// PG 18 removed wal_write/wal_sync columns; stats_age is col 6 and must not be diffed.
		return PgStatWALDefault, 7, [2]int{2, 5}
	}
	// cols 2-9: wal,KiB..buffers_full; col 10 (stats_age) excluded.
	return PgStatWALPG14, 11, [2]int{2, 9}
}
```

New first branch (three-branch shape, matching `SelectStatBgwriterQuery`, `bgwriter.go:41-52`):

```go
	if version >= PostgresV19 {
		// PG 19 added wal_fpi_bytes; cols 2-6 (wal,KiB..buffers_full) diffed, stats_age is col 7.
		return PgStatWALPG19, 8, [2]int{2, 6}
	}
```

`PostgresV19 = 190000` already exists (`internal/query/query.go:22`). The existing `180000` literal on
the next branch stays a literal unless the tech-spec decides to normalise it — a one-line cosmetic call,
not a requirement (research §7.6).

Resulting layout table (0-based, as the spec writes it):

| version | Ncols | DiffIntvl | columns |
|---|---|---|---|
| 14–17 | 11 | `{2,9}` | source, waldir_size, wal&#124;KiB, records, fpi, write, sync, write&#124;ms, sync&#124;ms, buffers_full, stats_age |
| 18 | 7 | `{2,5}` | source, waldir_size, wal&#124;KiB, records, fpi, buffers_full, stats_age |
| **19+** | **8** | **`{2,6}`** | source, waldir_size, wal&#124;KiB, records, fpi, **fpi&#124;KiB**, buffers_full, stats_age |

#### 10.B.3 `internal/view/view.go` — **no edit required for piece (3)**

`Configure()` already routes `wal` through the selector at `view.go:389-391`:

```go
		case "wal":
			view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatWALQuery(opts.Version)
			v[k] = view
```

so a new selector branch is picked up for free, in both the TUI (`top`) and the report replay
(`report/report.go:280-296`). The static seed at `view.go:129-140` (`QueryTmpl: query.PgStatWALPG14`,
`Ncols: 11`, `DiffIntvl: {2,9}`) stays as-is — it is the PG 14 baseline and is always overridden by
`Configure`. **This is the reason piece (3) does not collide with piece (1) on `view.go`.**

#### 10.B.4 Tests that pin the current values

| Test | File:line | Current value today | After |
|---|---|---|---|
| `Test_SelectStatWALQuery` | `internal/query/wal_test.go:10-31` | rows: `{140000, 11, {2,9}}` (`:16`), `{150000, 11, {2,9}}` (`:17`), `{170000, 11, {2,9}}` (`:18`), `{180000, 7, {2,5}}` (`:20`), **`{190000, 7, {2,5}}` (`:21`)** | `:21` becomes `{190000, 8, {2,6}}`; optionally add `{160000, 11, {2,9}}` and a `{200000, 8, {2,6}}` forward row |
| `Test_StatWALQueries` | `internal/query/wal_test.go:34-56` | `versions := []int{140000, 150000, 160000, 170000, 180000, 190000}` (`:35`); runs `Format` + `conn.Exec` | **no edit**; it picks up the new branch automatically and proves `PgStatWALPG19` executes on 21919 |
| `TestViews_Configure` | `internal/view/view_test.go:99-253` | the v19 block at `:183-192` asserts only the three progress screens; the v14 block at `:193-202` likewise. **Nothing asserts `wal` at any version today.** | add to `case 190000:` — `assert.Equal(t, query.PgStatWALPG19, views["wal"].QueryTmpl)`, `assert.Equal(t, 8, views["wal"].Ncols)`, `assert.Equal(t, [2]int{2,6}, views["wal"].DiffIntvl)`; and to `case 140000:` the PG 14 counterparts (`PgStatWALPG14`, 11, `{2,9}`) so the "PG 14–18 unchanged" AC has a test |
| `Test_app_doReport` (`wal` case) | `report/report_test.go:73-77` (`{ReportType: "wal", wantFile: "testdata/report_wal.golden"}`) | driven by `report/testdata/pgcenter.stat.golden.tar`, whose `meta` entry carries `"version_num":"140000"` (**checked today**), and whose golden has 11 columns | **unaffected** — confirmed, not assumed |

No `internal/view/view.go` line and no `record`/`top` test changes belong to piece (3).

---

### 10.C The `archiver` view registration block

#### 10.C.1 New file `internal/query/archiver.go`

```go
package query

const (
	// PgStatArchiverDefault defines query for pg_stat_archiver (all supported versions).
	// pg_stat_archiver is schema-identical on PG 14-19 (verified against live catalogs), so there
	// is no version branch. The .ready backlog comes from pg_ls_archive_statusdir() — same
	// superuser/pg_monitor privilege class as the pg_ls_waldir() the wal screen already calls
	// unconditionally, so the screen is all-or-nothing under a role without pg_monitor.
	// Every column is cumulative, a WAL name or an age string: nothing here is diffed
	// (DiffIntvl {0,0}), which is also what keeps the four NULL-able columns safe.
	PgStatArchiverDefault = "SELECT 'Archiver' AS source, " +
		"(SELECT count(*) FILTER (WHERE name LIKE '%.ready') FROM pg_ls_archive_statusdir()) AS ready, " +
		"archived_count AS archived, last_archived_wal AS last_archived, " +
		"date_trunc('seconds', now() - last_archived_time)::text AS archived_age, " +
		"failed_count AS failed, last_failed_wal AS last_failed, " +
		"date_trunc('seconds', now() - last_failed_time)::text AS failed_age, " +
		"date_trunc('seconds', now() - stats_reset)::text AS stats_age " +
		"FROM pg_stat_archiver"
)

// SelectStatArchiverQuery returns the query, column count and diff interval for pg_stat_archiver.
// The version parameter is unused (the view is schema-stable on every supported version) and kept
// for selector symmetry with the other Select*Query functions — see SelectStatIOTimeQuery.
func SelectStatArchiverQuery(_ int) (string, int, [2]int) {
	return PgStatArchiverDefault, 9, [2]int{0, 0}
}
```

Two notes for the writer:
- The `%` in `'%.ready'` is safe: `query.Format` is `text/template` (`internal/query/query.go:91-104`),
  which only reacts to `{{`/`}}`; and every `printCmdline` call in the tree uses an explicit `"%s"`
  verb. The identical literal already lives in `OverviewArchivingBacklog`.
- `_ int` (not `version int`) is what revive requires for an unused parameter — the precedent and its
  rationale are at `internal/query/io.go:94-99`.

#### 10.C.2 The `view.New()` entry

Goes into `internal/view/view.go:38-361`, alphabetically/structurally next to `wal` (`:129-140`) and
`bgwriter` (`:141-152`):

```go
		"archiver": {
			Name:               "archiver",
			MinRequiredVersion: query.PostgresV14,
			QueryTmpl:          query.PgStatArchiverDefault,
			DiffIntvl:          [2]int{0, 0},
			Ncols:              9,
			OrderKey:           0,
			OrderDesc:          true,
			ColsWidth:          map[int]int{},
			Msg:                "Show archiver statistics (requires archive_mode=on)",
			Filters:            map[int]*regexp.Regexp{},
		},
```

Field-by-field justification:

| field | value | why |
|---|---|---|
| `Name` | `"archiver"` | must equal the map key — it is also the report type string and the tar entry prefix (`report/report.go:459-476`, `record/recorder.go:290`) |
| `MinRequiredVersion` | `query.PostgresV14` | **not optional — see §10.I.1.** `pg_ls_archive_statusdir()` exists only from PG 12, and the registry still serves PG 9.4–13 clusters. `PostgresV14` matches the floor of every other new view (`wal`, `bgwriter`, `replslots`) and the oldest cluster in the test image |
| `QueryTmpl` | `query.PgStatArchiverDefault` | seed; `Configure` reassigns it via the selector |
| `DiffIntvl` | `[2]int{0,0}` | genuine pass-through — `calculateDelta` short-circuits (`internal/stat/postgres.go:589-597`) and never enters `diff()`. Same idiom as `activity` (`view.go:43`), `progress_copy` (`:305`), `progress_index` (`:317`) |
| `Ncols` | `9` | right border for `OrderKey` cycling; matches the live query |
| `OrderKey` / `OrderDesc` | `0` / `true` | single-row screen; identical to `wal` (`:135-136`) and `bgwriter` (`:147-148`) |
| `UniqueKey` | **omitted** (zero value `0`) | column 0 is the constant `'Archiver'` literal, which is what makes `UniqueKey=0` self-matching across samples. `wal`/`bgwriter` also omit it; the field comment is at `view.go:20`. With `DiffIntvl{0,0}` it is never even read |
| `ColsWidth` | `map[int]int{}` | required non-nil — `align.SetAlign` writes into it |
| `Msg` | `"Show archiver statistics (requires archive_mode=on)"` | verbatim from the user-spec ("Дизайн и интерфейс → Подпись экрана" and AC). Static, printed on every switch; the `stat_io_time` precedent is `view.go:188` |
| `Filters` | `map[int]*regexp.Regexp{}` | required non-nil — `setFilter`/`clearAllFilters` write into it in place (`top/config_view.go:147-213`) |
| `NotRecordable` | **omitted** (zero value `false`) | ADR `docs/decisions-log.md:429` "Lift `NotRecordable` only — pure-SQL views need no recorder change". `filterViews` (`record/record.go:200-233`) needs **no** edit |
| `Refresh`, `ShowExtra`, `CollectExtra`, `Verbose`, `IOAvailable`, `DelayAcctAvailable`, `Aligned`, `Cols`, `Query` | omitted | all runtime/zero-value, exactly as `wal` and `bgwriter` leave them |

#### 10.C.3 The `Configure()` case

Into the `switch k` at `internal/view/view.go:373-413`, next to `case "wal":` (`:389-391`):

```go
		case "archiver":
			view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatArchiverQuery(opts.Version)
			v[k] = view
```

This case is **functionally a no-op** (the selector is version-independent, so it re-assigns the same
three values the static entry already carries). It is still the right call: `stat_io_time` is likewise
version-independent and still has its case (`view.go:401-403`), and having the case means a future
version branch is a one-line change in one file instead of two. The tech-spec should state this
explicitly so a reviewer does not read it as dead code.

#### 10.C.4 Guard test to add

`internal/view/view_test.go`, modelled on `TestNew_BgwriterView` (`:87-97`):

```go
func TestNew_ArchiverView(t *testing.T) {
	v := New()
	archiver, ok := v["archiver"]
	assert.True(t, ok)
	assert.False(t, archiver.NotRecordable)
	assert.Equal(t, query.PostgresV14, archiver.MinRequiredVersion)
	assert.Equal(t, 9, archiver.Ncols)
	assert.Equal(t, [2]int{0, 0}, archiver.DiffIntvl)
	assert.Equal(t, 0, archiver.OrderKey)
	assert.True(t, archiver.OrderDesc)
	assert.Equal(t, 0, archiver.UniqueKey)
	// Msg is load-bearing: it is the only place the archive_mode requirement is stated.
	assert.Contains(t, archiver.Msg, "archive_mode=on")
}
```

---

### 10.D Piece (1) — the full `w`/`W` machinery diff list

Four files, five edits. The user-spec settles the naming: **the string `"wal"` is used both as the view
name and as the hotkey-group name**, with one dispatch point and an explicit code comment ("Мы решили не
вводить отдельное имя для группы экранов под хоткеем `w`").

#### 10.D.1 `top/keybindings.go`

- `:38` — `{"sysstat", 'w', switchViewTo(app, "wal")},` **stays byte-identical.** Its meaning changes
  only because `switchViewTo` gains a `case "wal":`.
- Add after `:49` (`{"sysstat", 'J', menuOpen(menuStatIO, app.config, "")},`):

  ```go
		{"sysstat", 'W', menuOpen(menuWAL, app.config, "")},
  ```

  `'W'` is free — a full read of `keybindings.go:18-85` shows no `'W'` binding. (`profile`'s `-W`
  at `cmd/profile/profile.go:52` is an unrelated CLI flag on a different sub-command.)

#### 10.D.2 `top/config_view.go` — dispatch case

Into `switchViewTo`'s `switch c` (`:241-252`), alongside `case "statio":` (`:248-249`):

```go
		case "wal":
			// "wal" is both a view name and the name of the w-hotkey group; this is the group's
			// single dispatch point. walNextView's default returns "wal", so 'w' pressed on any
			// other screen still lands on the wal screen, exactly as before this cycle existed.
			viewSwitchHandler(app.config, walNextView(app.config.view.Name))
```

Nothing else in `switchViewTo` moves: the pgss guard stays above (`:235-238`), and the single
`printCmdline(g, "%s", app.config.view.Msg)` at `:254` is what satisfies the "ровно один раз на путь"
acceptance criterion for the hotkey path.

#### 10.D.3 `top/config_view.go` — `walNextView`

Placed next to `statioNextView` (`:274-287`), same shape:

```go
// walNextView depending on current WAL-area view returns next view.
func walNextView(current string) string {
	var next string

	switch current {
	case "wal":
		next = "archiver"
	case "archiver":
		next = "wal"
	default:
		next = "wal"
	}
	return next
}
```

#### 10.D.4 `top/menu.go` — three edits

**(a) The iota block.** Current text, `top/menu.go:14-26`, verbatim:

```go
const (
	// Available menu types.
	menuNone      menuType = iota // no active menu
	menuDatabases                 // menu with pg_stat_databases stats
	menuPgss                      // menu with pg_stat_statements stats
	menuProgress                  // menu with pg_stat_progress_* stats
	menuConf                      // menu with configuration files
	menuStatIO                    // menu with pg_stat_io stats

	// Directions allowed when working with menu.
	moveUp   direction = iota // move up
	moveDown                  // move down
)
```

`menuWAL` goes **immediately after `menuStatIO`, above the blank line** — i.e. inside the menu group,
never in the directions group:

```go
	menuStatIO                    // menu with pg_stat_io stats
	menuWAL                       // menu with pg_stat_wal / pg_stat_archiver stats
```

Trap worth one sentence in the tech-spec: `moveUp`/`moveDown` share the block's `iota` counter, so this
insertion silently shifts them from `6`/`7` to `7`/`8`. That is harmless — `direction` is a distinct
type and its values are only ever compared against themselves (`moveCursor`, `menu.go:268-312`) — but
inserting `menuWAL` *below* the blank line would make `menuWAL` collide with `moveUp`'s intent and is
the mistake to guard against.

**(b) `selectMenuStyle` branch.** Copied from `case menuStatIO:` (`menu.go:87-95`, current text):

```go
	case menuStatIO:
		s = menuStyle{
			menuType: menuStatIO,
			title:    " Choose pg_stat_io mode (Enter to choose, Esc to exit): ",
			items: []string{
				" pg_stat_io operations",
				" pg_stat_io timings",
			},
		}
```

New branch, same two-item shape (leading space in each item is the established convention):

```go
	case menuWAL:
		s = menuStyle{
			menuType: menuWAL,
			title:    " Choose WAL / archiver mode (Enter to choose, Esc to exit): ",
			items: []string{
				" pg_stat_wal",
				" pg_stat_archiver",
			},
		}
```

The menu window is sized `g.SetView("menu", 0, 5, 72, 6+len(s.items))` (`menu.go:116`) — 2 items is
identical geometry to `menuStatIO`, and the title fits well within 72 columns.

**(c) `menuSelect` branch.** Copied from `case menuStatIO:` (`menu.go:194-203`):

```go
		case menuWAL:
			switch cy {
			case 0:
				viewSwitchHandler(app.config, "wal")
			case 1:
				viewSwitchHandler(app.config, "archiver")
			default:
				viewSwitchHandler(app.config, "wal")
			}
			printCmdline(app.ui, "%s", app.config.view.Msg)
```

Note it calls `viewSwitchHandler` **directly**, not `switchViewTo` — same as every other menu branch;
this is the second `printCmdline`-once-per-path site the AC covers.

#### 10.D.5 `top/help.go` — both lines

Current, `help.go:13-19`:

```
    a,b,f,o     mode: 'a' activity, 'b' bgwriter/checkpointer, 'f' functions, 'o' replication slots,
    r,w               'r' replication, 'w' WAL,
    s,t,i             's' tables sizes, 't' tables, 'i' indexes.
    d,D               'd' pg_stat_database switch, 'D' pg_stat_database menu.
    x,X               'x' pg_stat_statements switch, 'X' pg_stat_statements menu.
    p,P               'p' pg_stat_progress_* switch, 'P' pg_stat_progress_* menu.
    j,J               'j' pg_stat_io switch (operations/timings), 'J' pg_stat_io menu.
```

Edit 1 — `w` leaves line 14 and gets its own line after line 19 (`j,J`), in the `j,J` format the
user-spec dictates:

```
    r                 'r' replication,
    ...
    j,J               'j' pg_stat_io switch (operations/timings), 'J' pg_stat_io menu.
    w,W               'w' pg_stat_wal / pg_stat_archiver switch, 'W' WAL statistics menu.
```

Edit 2 — `help.go:45`, current:

```
                      ('Q' does not reset shared stats: pg_stat_io, bgwriter, wal).
```

becomes:

```
                      ('Q' does not reset shared stats: pg_stat_io, bgwriter, wal, archiver).
```

Two cosmetic decisions the tech-spec must make explicitly, because the user-spec does not:
- the leftover `r` on its own line (alternative: fold `r` into the `a,b,f,o` line above);
- the new line is 88 characters. The `j,J` neighbour is already 85, so it is consistent with the
  block; only the `Space` entry has an enforced ≤80 rule (`top/help_test.go:78-79`).

`top/help_test.go` needs **no** change: all three of its tests key off the `Space`/`(sort,`/`[,]`
markers and their *relative* line offsets (`:68-70`), which an inserted line above shifts uniformly, and
off `strings.Count(helpTemplate, "%") == 1` (`:120`), which the new text does not disturb. Adding a
`Test_helpTemplate_walEntry` in the `helpEntryLine` style would be the consistent (optional) move.

---

### 10.E Piece (2) — the `-W` flag change and every report-side addition

#### 10.E.1 The four `showWAL` sites — complete, re-verified today

| # | File:line | Current | New |
|---|---|---|---|
| 1 | `cmd/report/report.go:25` | `showWAL         bool   // Show stats from pg_stat_wal` | `showWAL         string // Show stats from pg_stat_wal / pg_stat_archiver` |
| 2 | `cmd/report/report.go:70` | `CommandDefinition.Flags().BoolVarP(&opts.showWAL, "wal", "W", false, "show pg_stat_wal report")` | `CommandDefinition.Flags().StringVarP(&opts.showWAL, "wal", "W", "", "show pg_stat_wal / pg_stat_archiver report (w - wal, a - archiver)")` (flag description text is verbatim from the user-spec) |
| 3 | `cmd/report/report.go:151-152` | `case opts.showWAL:` / `	return "wal"` | the block in §10.E.2 |
| 4 | `cmd/report/report_test.go:46` | `{opts: options{showWAL: true}, want: "wal"},` | `{opts: options{showWAL: "w"}, want: "wal"},` + two new rows |

Note the struct field alignment in `options` (`report.go:15-41`): today `showWAL bool` sits in the
`bool` column block; changing it to `string` means `gofmt` will re-align the whole comment column of
that struct. Expect a few incidental whitespace-only lines in the diff — that is gofmt, not scope creep.

#### 10.E.2 The new `selectReport` branch

Replaces `cmd/report/report.go:151-152` **in place** — position matters. `selectReport` is a
`switch { case … }` chain where **first match wins** (`report.go:132-203`), so `-A` beats `-W` today and
must continue to; keeping the branch at its current index between `showFunctions` (`:149-150`) and
`showBgwriter` (`:153-154`) preserves that precedence exactly.

```go
	case opts.showWAL != "":
		switch opts.showWAL {
		case "w":
			return "wal"
		case "a":
			return "archiver"
		}
```

Shape copied verbatim from `case opts.showStatIO != "":` (`report.go:157-163`), including the
deliberate absence of a `default`. An unrecognised value falls out of the inner switch **and** out of
the outer switch (Go has no implicit fallthrough), so `selectReport` returns `""` and `validate()`
(`report.go:94-96`) errors with `report type is not specified, quit`. That is exactly the behaviour the
user-spec's edge cases require for `-W x` and for `-W -f dump.tar` (where pflag consumes `-f` as the
flag value, leaving `showWAL == "-f"`).

The `-W`-as-last-token message is **pflag's**, not ours: `pflag v1.0.10`
(`go.mod:27`) formats it at `errors.go:75` as `flag needs an argument: %q in -%s`, which renders as
`flag needs an argument: 'W' in -W` — byte-identical to the user-spec's acceptance criterion. Confirmed
against the module in `$GOPATH/pkg/mod/github.com/spf13/pflag@v1.0.10/errors.go:75` and its own
`flag_test.go:595`.

#### 10.E.3 `Test_selectReport` rows (`cmd/report/report_test.go:34-73`)

- `:46` changes from `{opts: options{showWAL: true}, want: "wal"}` to `{opts: options{showWAL: "w"}, want: "wal"}`.
- add `{opts: options{showWAL: "a"}, want: "archiver"}`;
- add `{opts: options{showWAL: "x"}, want: ""}` — mirroring the `-J`/`-X` invalid-value rows at `:65-66`.

`Test_options_validate` (`report_test.go:10-32`) uses only `showActivity` — **unaffected**, confirmed by
reading all five cases.

#### 10.E.4 `describeReport` map entry

`report/report.go:665-693`. Add next to the `"wal"` entry at `:674`:

```go
		"wal":                 pgStatWALDescription,
		"archiver":            pgStatArchiverDescription,
```

(Map literal order is semantically irrelevant; adjacency is for the reader.) `report -d -W a` reaches
this without touching the archive at all — `RunMain` short-circuits on `c.Describe` before opening the
input file (`report/report.go:44-47`), so `pgcenter report -d -W a` works with no `-f`.

#### 10.E.5 `report/describe.go` — two edits

**(a) New constant.** Follows the existing `column / origin / description` table format and ends with a
docs URL, like `pgStatWALDescription` (`describe.go:140-157`) and `pgStatBgwriterDescription`
(`describe.go:456-…`). Note the file uses **literal tabs** as column separators:

```go
	// pgStatArchiverDescription is the detailed description of pg_stat_archiver view
	pgStatArchiverDescription = `WAL archiver statistics based on pg_stat_archiver view:

  column	origin			description
- source	-			Always has 'Archiver' value
- ready		pg_ls_archive_statusdir	Number of WAL segments waiting to be archived (*.ready files)
- archived	archived_count		Total number of WAL files successfully archived
- last_archived	last_archived_wal	Name of the last WAL file successfully archived
- archived_age	last_archived_time	Time elapsed since the last successful archiving (empty if never)
- failed	failed_count		Total number of failed attempts to archive a WAL file
- last_failed	last_failed_wal		Name of the WAL file of the last failed archiving attempt
- failed_age	last_failed_time	Time elapsed since the last failed attempt (empty if never)
- stats_age	stats_reset		Age of collected statistics in the moment when stats are taken

Details: https://www.postgresql.org/docs/current/monitoring-stats.html#PG-STAT-ARCHIVER-VIEW
`
```

**(b) The FPI row in `pgStatWALDescription`.** Current block, `describe.go:141-157`; the `fpi` row is
`describe.go:148`:

```
- fpi		wal_fpi			Number of WAL full page images generated
```

Insert immediately after it (mirroring the on-screen position — the column sits right after the count):

```
- fpi,KiB	wal_fpi_bytes		Amount of WAL generated by full page images, in KiB (PG 19+)
```

The constant already documents the PG 14 baseline including `write`/`sync`/`write,ms`/`sync,ms`
(`:149-152`), which PG 18 removed — it is version-approximate already, so a `(PG 19+)` annotation on the
new row keeps it no worse and is the cheapest honest option. This is what satisfies the AC
"`pgcenter report -d -W w` … описывает в том числе колонку `fpi,KiB`".

#### 10.E.6 `Test_describeReport` (`report/report_test.go:1172-1213`)

Add `{report: "archiver", want: pgStatArchiverDescription},` next to the `"wal"` row (`:1184`). The
test compares by identity, so it does not notice row ordering — `Test_describeProgressColumnOrder`
(`report_test.go:1216-…`) is the marker-order test that exists for the progress screens; extending it to
`archiver`/`wal` is optional but is the project's established way of pinning that `-d` documents the
layout that actually ships.

#### 10.E.7 What needs **no** change

- `record/record.go:200-233` `filterViews` — `NotRecordable` stays false (ADR `docs/decisions-log.md:429`).
- `record/recorder.go:116-153` `collect` — iterates the views map generically; the tar entry name is
  `newFilenameString(ts, "archiver")` → `archiver.20260805T120000.000.json` for free (`recorder.go:290`).
- `report/report.go:459-476` `isFilenameOK` — accepts `s[0] == report`; report type == view name == tar
  prefix.
- `record/record_test.go:32-107` `Test_app_record` — `filesWant` is computed from
  `countRecordable(view.New())` (`record_test.go:37`), **not hardcoded**, so it adapts on its own.

---

### 10.F Golden replay tests

`report/report_record_bgwriter_test.go:1-240` read end to end. Its anatomy, and what a new file must
reproduce:

#### 10.F.1 How the harness works

1. **Synthetic in-memory tar.** `bytes.Buffer` + `tar.NewWriter`; a local `writeEntry(name, payload)`
   closure writes `&tar.Header{Name: name, Size: int64(len(payload)), Mode: 0644}` then the bytes
   (`:188-202`). Six entries for two ticks, in the per-tick order `tarRecorder.write()` emits —
   `meta.*`, `<view>.*`, `sysinfo.*`:

   ```
   meta.20260519T100000.000.json      bgwriter.20260519T100000.000.json   sysinfo.20260519T100000.000.json
   meta.20260519T100001.000.json      bgwriter.20260519T100001.000.json   sysinfo.20260519T100001.000.json
   ```

   The filename timestamp format is the recorder's `20060102T150405.000`, and the two ticks are
   **exactly one second apart** so the rate divisor `itv == 1` and every diffed column is a bare
   `curr - prev` with no scaling (`:26-30`).

2. **Payloads are marshalled `stat.PGresult` values**, not SQL. `mkRow` (`:155-161`) wraps a
   `[]string` into `[]sql.NullString{{String: v, Valid: true}}`, and each tick is
   `stat.PGresult{Valid: true, Ncols: ncols, Nrows: 1, Cols: tc.cols, Values: …}` (`:166-180`).
   `sysinfo` is a hand-written literal `[]byte(`{"ticks":100,"cpu_count":4}`)` (`:182`).

3. **meta drives `Configure`.** The `meta` entry is a 7-column `PGresult` mirroring
   `SelectCommonProperties` (`:141-151`); only `Values[0][1]` (`version_num`) is consumed — `readMeta`
   accepts any `Ncols >= 2` (`report/report.go:445-457`). That version reaches
   `views.Configure(query.Options{Version: d.meta.version})` on the **first** sample and on every
   recorded-version change (`report/report.go:280-296`), which is the entire mechanism under test.

4. **The first tick is discarded**, always: `processData` takes the `!prevStat.Valid` branch and
   `continue`s (`report/report.go:319`). Two ticks therefore produce exactly one data row.

5. **Driving and comparing.** `Config{ReportType: …, TruncLimit: 32, TsStart/TsEnd}` bracketing the
   filename date, `app := newApp(config)`, `app.writer = &buf`, `app.doReport(tar.NewReader(&tarBuf))`
   (`:204-216`). Then three cheap invariants before the golden (`:218-228`): non-empty output, a
   `\d{4}/\d{2}/\d{2}` timestamp header, and one or two `assert.Contains` sentinels chosen so a failure
   reads as "row missing" vs "delta wrong" rather than "golden differs".

6. **Golden update.** `if *update { os.WriteFile(tc.wantFile, buf.Bytes(), 0644); return }` (`:230-233`)
   — `update` is the package-level `flag.Bool("update", …)` at `report/report_test.go:24`, shared by
   every golden test in the package. Regenerate with `go test ./report/ -run … -update`.

7. **Goldens live in `report/testdata/`** as raw bytes including ANSI SGR sequences (the header cells
   are wrapped in `\033[37;1m…\033[0m` — see `report_wal.golden`).

#### 10.F.2 Version-independent vs version-aware — the naming rule

The tree already shows both conventions, and they are the ADR `docs/decisions-log.md:462` shape:

- **version-independent → one golden**, no version in the name:
  `testdata/report_record_stat_io_time.golden`, driven by a single recorded version
  (`report/report_record_statio_test.go:177-206` pins `versionNum: "160000"` with the comment "one
  recorded version (16) suffices").
- **version-aware → one golden per version**, `_pgNN` suffix:
  `report_record_bgwriter_pg14/pg17/pg18.golden`, `report_record_progress_vacuum_pg18/pg19.golden`.
  (`report_record_stat_io_v16/v18.golden` uses a `_vNN` suffix — the `_pgNN` form is the newer and more
  common one; pick `_pgNN` for consistency with the most recent additions.)
- **A dedicated empty/degenerate case gets its own golden**: `report_record_replslots_empty.golden`.

#### 10.F.3 `report/report_record_archiver_test.go` — what it must contain

`archiver` is **version-independent** → a single subcase, a single golden. Concretely:

- one recorded version — `versionNum: "170000"`, `versionStr: "17.1"` (any supported one; 17 keeps it
  clear that the screen does not branch);
- `ReportType: "archiver"`, tar entry prefix `archiver.`, `ncols = 9`;
- `cols = []string{"source","ready","archived","last_archived","archived_age","failed","last_failed","failed_age","stats_age"}`;
- **the load-bearing property is pass-through, not diff.** `DiffIntvl{0,0}` means `calculateDelta`
  returns `curr` verbatim (`internal/stat/postgres.go:589-597`), so the two ticks must have *different*
  values in the numeric columns and the test must assert the **absolute curr value**, not a delta.
  E.g. prev `ready=10, archived=100, failed=5`, curr `ready=14, archived=103, failed=8`; the output row
  must contain `14`, `103`, `8` — and a `assert.NotContains(t, out, …)` on the would-be delta (`3`) is
  not usable as a sentinel because `3` appears inside `103`; use a distinctive pair instead, e.g.
  `archived` prev `100000` → curr `100003`, and assert the output contains `100003`. This assertion is
  what would catch someone "helpfully" giving the screen a diffed range;
- golden: **`report/testdata/report_record_archiver.golden`**.

A **second subcase** is worth its own golden, because the user-spec makes it an acceptance criterion
("колонки … **пустые** (не `0` и не прочерк)"): a never-archived cluster, where
`last_archived`, `archived_age`, `last_failed`, `failed_age` are `sql.NullString{String: "", Valid: false}`
in both ticks. It proves (a) the sample is not aborted, (b) the cells render blank, (c) `sort` keeps
input order when the whole key column is empty (`internal/stat/postgres.go` `sort`, the
`sample == ""` early return). Golden: **`report/testdata/report_record_archiver_null.golden`**.

`Test_app_doReport` in `report/report_test.go` should **not** gain an `archiver` case — the legacy
`testdata/pgcenter.stat.golden.tar` was recorded in 2021 on PG 14beta1 and contains no `archiver`
entries. A third, cheap subcase in the new file covering "archive with no archiver entries → header
only, no error, nil return" is the right home for that AC (precedent:
`report_record_replslots_empty.golden`).

#### 10.F.4 `report/report_record_wal_test.go` — what it must contain

`wal` is **version-aware** → the `Test_app_doReport_Bgwriter` table shape, one subcase and one golden
per version. Minimum per the user-spec's AC (PG 18 and PG 19); adding PG 14 is nearly free and closes
the largest branch, which currently has no replay coverage at all:

| subcase | versionNum / versionStr | ncols | DiffIntvl | cols | golden |
|---|---|---|---|---|---|
| `pg14` *(optional but recommended)* | `140009` / `14.9` | 11 | `{2,9}` | source, waldir_size, `wal,KiB`, records, fpi, write, sync, `write,ms`, `sync,ms`, buffers_full, stats_age | `testdata/report_record_wal_pg14.golden` |
| `pg18` | `180000` / `18.0` | 7 | `{2,5}` | source, waldir_size, `wal,KiB`, records, fpi, buffers_full, stats_age | `testdata/report_record_wal_pg18.golden` |
| `pg19` | `190000` / `19.0` | 8 | `{2,6}` | source, waldir_size, `wal,KiB`, records, fpi, **`fpi,KiB`**, buffers_full, stats_age | `testdata/report_record_wal_pg19.golden` |

Details that matter:

- `source` is the constant `"WAL"`; `UniqueKey` defaults to 0, so the single row pairs across ticks.
- **`waldir_size` is a pretty string** (`"1040 MB"`), at column 1, *outside* `DiffIntvl` in every
  branch. Give it a value that would fail `strconv.ParseInt` — that is the assertion that it is not
  being diffed.
- **`stats_age` is a text interval** (`"01:00:00"` → `"02:00:00"`), the last column, outside
  `DiffIntvl`. Same reasoning, and it is the exact cell that the §10.I.5 regression would poison.
- Sentinel: pick the `records` delta identical across all three versions (e.g. prev `1000` → curr
  `1500`, delta `500`) so one `assert.Contains(t, out, "500")` doubles as a cross-version delta check,
  exactly as the bgwriter test does with `buf_ckpt` (`report_record_bgwriter_test.go:226-228`).
- The pg19 subcase must additionally assert the new column: `assert.Contains(t, out, "fpi,KiB")` in the
  header, plus its own delta value; and the pg18 subcase `assert.NotContains(t, out, "fpi,KiB")`. That
  pair is what makes "на PG 14–18 набор колонок не изменился" a tested claim rather than a promise.

#### 10.F.5 Golden files to create — the complete list

```
report/testdata/report_record_archiver.golden
report/testdata/report_record_archiver_null.golden
report/testdata/report_record_wal_pg18.golden
report/testdata/report_record_wal_pg19.golden
report/testdata/report_record_wal_pg14.golden        (optional, recommended)
```

---

### 10.G Every test whose numbers change — current values read from the tree today

All values below were read from the files at `41fb586` while writing this section; the "after" column
assumes `archiver` is registered with `MinRequiredVersion: query.PostgresV14` and `NotRecordable: false`.

| # | Test | File:line | Current (today) | After |
|---|---|---|---|---|
| 1 | `TestNew` | `internal/view/view_test.go:9-12` | `assert.Equal(t, 27, len(v))` | `28` (and the trailing comment "27 is the total number of views" must move too) |
| 2 | `TestView_VersionOK` | `internal/view/view_test.go:255-280` | `{190000, 27}`, `{160000, 27}`, `{140000, 24}`, `{130000, 19}`, `{120000, 16}`, `{110000, 14}`, `{100000, 14}` | `{190000, **28**}`, `{160000, **28**}`, `{140000, **25**}`; **`130000`, `120000`, `110000`, `100000` unchanged** (the PG14 gate drops `archiver` there) |
| 3 | `Test_filterViews` | `record/record_test.go:109-149` | `{190000,"public",wantN:0,wantV:27}`, `{140000,"",9,18}`, `{140000,"public",3,24}`, `{130000,"public",8,19}`, `{120000,"public",11,16}`, `{110000,"public",13,14}`, `{100000,"public",13,14}` | ≥PG14 rows: `wantV +1` → `28`, `19`, `25` (`wantN` unchanged, `archiver` survives both gates). ≤PG13 rows: `wantN +1` → `9`, `12`, `14`, `14` (`wantV` unchanged, dropped by the version gate). The block comment at `:116-134` explicitly reasons about the counts and must be extended |
| 4 | `Test_selectMenuStyle` | `top/menu_test.go:8-25` | `{menuNone,0}`, `{menuDatabases,2}`, `{menuPgss,7}`, `{menuProgress,6}`, `{menuConf,4}`, `{menuStatIO,2}` | add `{menu: menuWAL, want: 2}` |
| 5 | `Test_switchViewTo` | `top/config_view_test.go:588-653` | 25 table rows (`:598-623`). WAL-relevant: `:604` `{current:"sizes", to:"wal", want:"wal"}`, `:605` `{current:"wal", to:"replication", want:"replication"}`. statio cycle model at `:621-623` | `:604` **survives unchanged** (`walNextView("sizes")` hits `default` → `"wal"`), `:605` unaffected. Add three rows mirroring `:621-623`: `{"wal","wal","archiver"}`, `{"archiver","wal","wal"}`, `{"activity","wal","wal"}` |
| 6 | `Test_statioNextView` | `top/config_view_test.go:670-683` | 3 cases | unchanged; it is the **template** for a new `Test_walNextView` with `{"wal"→"archiver"}`, `{"archiver"→"wal"}`, `{"unknown"→"wal"}` |
| 7 | `Test_SelectStatWALQuery` | `internal/query/wal_test.go:16-21` | `{190000, wantNcols: 7, wantDiffIntvl: {2,5}}` at `:21` | `{190000, 8, {2,6}}`; rows `:16-:20` unchanged |
| 8 | `Test_selectReport` | `cmd/report/report_test.go:46` | `{opts: options{showWAL: true}, want: "wal"}` | `{showWAL: "w"}` + two new rows (§10.E.3) |
| 9 | `Test_describeReport` | `report/report_test.go:1184` | `{report: "wal", want: pgStatWALDescription}` | add `{report: "archiver", want: pgStatArchiverDescription}` |
| 10 | `TestViews_Configure` | `internal/view/view_test.go:99-253` | the `case 190000:` block (`:183-192`) and `case 140000:` block (`:193-202`) assert only progress screens; **no `wal` assertion at any version** | add the `wal` assertions of §10.B.4 |

**Tests that pin `view.Msg` strings** — the complete set (grep for `.Msg` across all `*_test.go`):

| File:line | Assertion | Effect |
|---|---|---|
| `internal/view/view_test.go:30` | `assert.Contains(t, jit.Msg, "jit=off")` | unaffected |
| `internal/view/view_test.go:48` | `assert.NotEqual(t, "", statio.Msg)` | unaffected |
| `internal/view/view_test.go:66` | `assert.Contains(t, statioTime.Msg, "track_io_timing")` | unaffected |
| `internal/view/view_test.go:82` | `assert.Equal(t, "Show replication slots statistics", replslots.Msg)` | unaffected |
| `internal/view/view_test.go:96` | `assert.Equal(t, "Show bgwriter / checkpointer statistics", bgwriter.Msg)` | unaffected |

**No test pins the `wal` view's `Msg`** ("Show WAL statistics") today, and none pins the total set of
`Msg` strings. The new `TestNew_ArchiverView` (§10.C.4) is where the archiver `Msg` gets pinned.

**Does any test pin the total help-screen content?** No. `top/help_test.go` (128 lines, read in full) is
the only file that touches `helpTemplate`, and all four of its assertions are **local**:
`Test_helpTemplate_pauseEntry` (`:52-80`) pins one line's text and its position *relative* to its
neighbours (`:68-70`); `Test_helpTemplate_pauseLiftingActions` (`:85-111`) asserts substrings of the
`(sort,` continuation line only; `Test_helpTemplate_formatVerbs` (`:119-128`) counts `%` and `%s`.
There is no whole-template golden, no line-count assertion, and no width assertion outside the `Space`
entry (`:78-79`). Editing lines 14, 19 and 45 of `help.go` breaks nothing.

**Tests that do NOT need touching, verified rather than assumed:**
`record/record_test.go:32-107` (`Test_app_record` — `filesWant` derives from
`countRecordable(view.New())` at `:37`), `record/record_test.go:151-192`
(`TestFilterViews_NotRecordable`, `TestFilterViews_dropsExplicitNotRecordable` — synthetic view maps),
`report/report_test.go:73-77` (`Test_app_doReport` `wal` case — its archive's `meta` carries
`version_num` `140000`, checked by extracting the tar today), `internal/query/wal_test.go:34-56`
(`Test_StatWALQueries` — version list already includes `190000`),
`internal/query/overview_test.go:123-146` and `internal/stat/postgres_test.go:206-236` (backlog
degradation — behaviour-only assertions, §10.A.4).

---

### 10.H Proposed wave decomposition

Dependency structure first: the four pieces are almost independent, and the only genuine coupling is
that the TUI/CLI/report work all need the `archiver` **view to exist** and the PG 19 **wal numbers to be
settled**.

#### Wave 1 — three fully isolated tasks, safe in parallel

| Task | Files (exclusive) | Notes |
|---|---|---|
| **T1** — piece (4), verbose-panel backlog | `internal/query/overview.go` (constant + its 8-line doc comment), `internal/query/overview_test.go` (comment at `:124-125`), `internal/stat/postgres.go` (comment at `:288-290` only) | Zero production-logic change outside the SQL string. Independently shippable; nothing else in the feature depends on it |
| **T2** — piece (3a), PG 19 wal query layer | `internal/query/wal.go`, `internal/query/wal_test.go` | Produces the `PgStatWALPG19` name and the `(8, {2,6})` pair that T4/T7/T8 consume |
| **T3** — piece (1a), archiver query layer | `internal/query/archiver.go` **(new)**, `internal/query/archiver_test.go` **(new)** | Two brand-new files → cannot conflict with anything. Produces `PgStatArchiverDefault` / `SelectStatArchiverQuery` |

`internal/stat/postgres.go` is touched by T1 only, and only in a comment — but if the tech-spec decides
to leave that comment alone, T1 becomes a pure `internal/query/` task.

#### Wave 2 — single-owner task on the hotspot

| Task | Files (exclusive) | Depends on |
|---|---|---|
| **T4** — view registration + Configure + count tests | `internal/view/view.go`, `internal/view/view_test.go`, `record/record_test.go` | T3 (constant name), T2 (PG 19 numbers for the `TestViews_Configure` assertions) |

**`internal/view/view.go` is the hotspot and this is why it gets its own wave.** Piece (1) adds the
registry entry and the `Configure` case; piece (3) needs **no** `view.go` edit at all (§10.B.3). Piece
(3) *does* want assertions in `internal/view/view_test.go`, which is why T4 owns the PG 19 `wal`
assertions too rather than letting a later task reopen the file.

#### Wave 3 — four tasks in parallel, disjoint file sets

| Task | Files (exclusive) | Depends on |
|---|---|---|
| **T5** — TUI hotkeys, cycle, menu, help | `top/keybindings.go`, `top/config_view.go`, `top/config_view_test.go`, `top/menu.go`, `top/menu_test.go`, `top/help.go`, (`top/help_test.go` if a `w,W` test is added) | T4 |
| **T6** — `-W` flag | `cmd/report/report.go`, `cmd/report/report_test.go` | T4 |
| **T7** — descriptions | `report/describe.go`, `report/report.go` (map entry at `:674` only), `report/report_test.go` (`Test_describeReport` row) | T2 + T4 |
| **T8** — golden replay tests | `report/report_record_archiver_test.go` **(new)**, `report/report_record_wal_test.go` **(new)**, `report/testdata/report_record_*.golden` **(new)** | T2 + T4 |

#### Wave 4 — docs and the manual gate

| Task | Files |
|---|---|
| **T9** — user documentation | `README.md` (supported-statistics list + the `-W` note, see §10.I.3), `doc/pgcenter-report-readme.md` and/or `doc/examples.md`, `doc/release-notes/v0.12.0.md` |
| **T10** — manual stand run | none (the checklist in the user-spec's "Пользователь проверяет") |

#### Files two candidate tasks would both want — the conflict list

| File | Wanted by | Resolution |
|---|---|---|
| **`internal/view/view.go`** | piece (1) registry + Configure case | Only piece (1). **Piece (3) needs no edit here** — the `case "wal":` at `:389-391` already delegates to the selector. Single owner: T4 |
| **`internal/view/view_test.go`** | piece (1) (`TestNew`, `TestView_VersionOK`, `TestNew_ArchiverView`) **and** piece (3) (`TestViews_Configure` v19/v14 `wal` assertions) | **Real conflict.** Give both to T4 |
| **`report/describe.go`** | piece (2) (new `pgStatArchiverDescription`) **and** piece (3) (FPI row inside `pgStatWALDescription`) | **Real conflict.** Give both to T7 |
| **`report/report.go`** | piece (2) (`describeReport` map) | Single owner T7. T8 must not touch it |
| **`report/report_test.go`** | piece (2) (`Test_describeReport`) | Single owner T7. T8 writes only *new* files, and reuses the package-level `update` flag at `:24` read-only |
| **`cmd/report/report.go`** | piece (2) | Single owner T6 |
| **`internal/query/wal.go` / `wal_test.go`** | piece (3) | Single owner T2 |
| **`internal/query/overview.go`** | piece (4) | Single owner T1 |
| **`top/*`** | piece (1) | Single owner T5 — no other piece enters `top/` |
| **`internal/stat/postgres.go`** | piece (4), comment only | Single owner T1 |

Detachability note the risk register already implies: **T2 + the PG 19 half of T7/T8 are the only work
that depends on PG 19beta2 catalog names.** If `wal_fpi_bytes` moves at RC, the blast radius is one
constant, one selector branch, one describe row and one golden — nothing in T1, T3, T4, T5, T6 changes.
That is what makes the "FPI can be detached and shipped at RC/GA" claim in the user-spec's risk section
true in practice.

---

### 10.I What the code cannot deliver exactly as the user-spec is written

Five findings, ordered by how much they would cost if discovered during implementation instead of now.

#### 10.I.1 `MinRequiredVersion` is **not** optional — read literally, the spec breaks PG ≤ 11

The user-spec's Ограничения section says:

> `pg_stat_archiver` не менялся с PG 9.0, а `pg_ls_archive_statusdir()` существует с PG 12 — при
> поддержке PG 14–19 экрану `archiver` не нужны **ни минимальная версия сверх общей**, ни ветвление
> запроса по версиям.

There is **no "общая" (common) version floor in the registry.** `view.New()` registers views with
`MinRequiredVersion` ranging from the zero value (`activity`, `tables`, `sizes`, `statements_*`, …)
through `PostgresV96` (`progress_vacuum`, `view.go:279`) up to `PostgresV16` (`stat_io`, `:167`). The
runtime genuinely serves old clusters: `TestView_VersionOK` (`view_test.go:255-280`) has rows for
`130000`, `120000`, `110000`, `100000`; `Test_filterViews` (`record_test.go:135-141`) the same; and
`TestViews_Configure` (`view_test.go:99-174`) exercises the whole registry down to `90400`.

If `MinRequiredVersion` is left at its zero value, then on any cluster below PG 12 the `archiver` view
is registered and its query runs `pg_ls_archive_statusdir()`, which does not exist — the screen shows
`ERROR: function pg_ls_archive_statusdir() does not exist` on every tick, and `pgcenter record`
**aborts the entire recording** on the first sample (`record/recorder.go:136-139`).

**Resolution:** the tech-spec must state `MinRequiredVersion: query.PostgresV14` explicitly (§10.C.2),
and the spec sentence should be reread as "no floor *beyond* the one every new view already uses". This
is a wording risk, not a design error — but it is exactly the kind that survives into code.

#### 10.I.2 `report -W a` prints N−1 rows for N ticks, not one row per tick

Scenario 3 of the user-spec says:

> 2. Система печатает **по одной строке на тик** за указанный интервал.

`processData` unconditionally discards the first sample it sees — the `!prevStat.Valid` branch at
`report/report.go:315-320` ends in `continue`, before any printing. This is correct and desirable for a
diffed screen (the first tick has nothing to diff against), but `archiver` has `DiffIntvl{0,0}` and is a
**pure pass-through**: the first tick carries complete, printable information and is thrown away
anyway. For a 4-hour recording at a 1-second interval the loss is one row out of 14 400 — irrelevant in
practice, but the sentence as written is false, and the AC "`pgcenter report -W a` воспроизводит
записанные данные" should not be read as "all of them".

**Resolution:** restate as "one row per tick, starting from the second"; no code change (making
pass-through screens print the first tick would be a change to shared replay logic, well outside this
feature).

#### 10.I.3 The AC "Обновлены README (описание флага `-W`)" has no target — `-W` is undocumented today

Grepped the whole documentation tree: `README.md`, `doc/pgcenter-report-readme.md`,
`doc/pgcenter-record-readme.md`, `doc/pgcenter-top-readme.md`, `doc/examples.md`.

- `README.md` mentions WAL exactly once — `README.md:55`, a bullet in the "Supported statistics" list
  linking to the `pg_stat_wal` docs page. No flags anywhere.
- `doc/pgcenter-report-readme.md` (28 lines, read in full) documents *capabilities* in prose and gives
  one usage example (`pgcenter report -f /tmp/stats.tar --database`). **It contains no flag reference at
  all** — not for `-W`, not for `-J`, not for any other.
- `doc/examples.md` has five `pgcenter report` examples; none uses `-W`.
- `doc/pgcenter-top-readme.md` has zero hits for `wal` — the hotkeys are documented only inside the app.

So there is nothing to "update": the only place `-W` is described today is the cobra flag help string at
`cmd/report/report.go:70`. The AC as written cannot be satisfied by editing an existing description.

**Resolution — pick one and say so:**
(a) narrow the AC to "flag help string + release notes 0.12.0" (matching where the description actually
lives), or (b) accept that a *new* flag-reference section in `doc/pgcenter-report-readme.md` is in
scope — which is real, unestimated work and would beg the question of why only `-W` gets one. Note the
same gap applies to the `archiver` screen and the `w`/`W` hotkeys in `doc/pgcenter-top-readme.md`.
Recommendation: (a), plus the one-line `pg_stat_archiver` bullet in the `README.md:55` neighbourhood,
which *is* an existing list with an obvious slot.

#### 10.I.4 Piece (4) changes the degradation surface in one case the spec says is unchanged

The user-spec states:

> Для суперпользователя видимых изменений нет … Механизм деградации до `n/a` при **любой другой ошибке**
> сохраняется без изменений.

`pg_ls_dir('pg_wal/archive_status')` (the 1-argument form) is `missing_ok = false`;
`pg_ls_archive_statusdir()` is `missing_ok = true`. **Verified live** on PG 18.4 by moving
`$PGDATA/pg_wal/archive_status` aside:

```
-- OLD pg_ls_dir with dir missing --   ERROR:  could not open directory "pg_wal/archive_status": No such file or directory
-- NEW pg_ls_archive_statusdir with dir missing --   0
```

So a missing `archive_status/` flips from `n/a` to a real `0 B` in the verbose panel. `initdb` always
creates the directory, so this is pathological rather than practical — but it is a *reduction* in
honesty (a missing directory is a broken cluster, and the panel will now claim a healthy zero backlog).

**Resolution:** one sentence in the tech-spec acknowledging it, and no code. Guarding it would require a
separate directory-existence probe, which is worse than the disease.

#### 10.I.5 The "known limitation" about PG 19 archives recorded by an older pgcenter is real — traced to the line

The user-spec records it as accepted; it is worth confirming it is not merely plausible, because it is
the one place this feature can make an existing archive **stop working**:

An archive of the `wal` screen recorded on PG 19 by pgcenter ≤ 0.11 has **7** columns
(`source, waldir_size, wal,KiB, records, fpi, buffers_full, stats_age`) because the old selector's
`version >= 180000` branch matched. On replay, `report/report.go:280-296` calls `Configure` with the
recorded `version_num` `190000`, which after this feature returns `(PgStatWALPG19, 8, {2,6})`. Then:

- `diff()` iterates `for l := 0; l < curr.Ncols; l++` with `curr.Ncols == 7`
  (`internal/stat/postgres.go:632`) and diffs every `l` in `[interval[0], interval[1]] = [2,6]`;
- `l == 6` in the recorded layout is **`stats_age`**, a text interval such as `"01:00:00"`;
- `diffPair` → `strconv.ParseInt("01:00:00")` fails and the error propagates out of `diff()`;
- `calculateDelta` wraps it as `fmt.Errorf("diff failed: %w", err)`
  (`internal/stat/postgres.go:593-595`), which `countDiff` (`report/report.go:501-512`) returns and
  `processData` returns, aborting the report.

So the user-visible failure is exactly `diff failed: …` and the exit is non-zero. Confirmed by reading
the code path end to end, not inferred. Two things follow for the tech-spec:

- release notes must name the failure text (`diff failed`) so a user can recognise it;
- the reverse direction is **safe**, and that is already handled: an archive spanning a real version
  change resets the alignment latch, the header counter and the resolved sort column
  (`report/report.go:296-317`), which is tech-debt item **[021]**, *resolved* on 2026-07-25
  (`docs/tech-debt.md:356-380`). No new work there.

#### 10.I.6 Two smaller notes, not blockers

- **Recorder abort ordering is nondeterministic.** `tarRecorder.collect` iterates
  `for k, v := range views` (`record/recorder.go:135`) — Go map order is randomised. Under a role
  without `pg_monitor`, *which* view's privilege error aborts the recording (`wal` or `archiver`)
  varies run to run, so the error text a user sees is not stable. Pre-existing behaviour, newly
  reachable by a second view; worth one line in the release notes rather than a fix.
- **The `w` cycle and `Test_switchViewTo:604` coexist by luck, not design.** `{current:"sizes",
  to:"wal", want:"wal"}` keeps passing only because `walNextView`'s `default` returns `"wal"`. That is
  the intended semantics ("`w` from any other screen opens `wal`"), so it is fine — but the tech-spec
  should say so, otherwise the first person to reorder `walNextView`'s `default` will silently change a
  documented hotkey behaviour and see one unrelated-looking test fail.
