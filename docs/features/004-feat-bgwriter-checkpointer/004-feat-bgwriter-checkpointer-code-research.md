# Code Research: 004-feat-bgwriter-checkpointer

Feature: new single-row TUI screen `bgwriter` (hotkey `b`) combining `pg_stat_bgwriter` +
`pg_stat_checkpointer`. Follows the `pg_stat_wal` view pattern exactly. TUI (`top`) only,
`NotRecordable: true`. Checkpoint/restartpoint EVENT counters shown ABSOLUTE; work/time/buffer
columns DIFFed.

Research date: 2026-06-21. PG column inventory for PG17 verified against a live local PostgreSQL
17.7 cluster; PG14/15/16/18 columns cited from PostgreSQL documentation and the existing
`internal/query/wal.go` version-split precedent (the same PG18 stats-system rewrite that affected
`pg_stat_wal`).

---

## 1. Reference Template — `pg_stat_wal` (the canonical pattern)

### `internal/query/wal.go`
Two query constants + one version selector. This is the exact template for a new
`internal/query/bgwriter.go`.

- `PgStatWALPG14` (`wal.go:5-11`) — query for PG 14-17. Shape:
  `SELECT 'WAL' AS source, <pretty/derived cols>, <counter cols ...>, date_trunc('seconds', now() - stats_reset)::text AS stats_age FROM pg_stat_wal`.
- `PgStatWALDefault` (`wal.go:15-21`) — PG 18+ variant (PG18 removed `wal_write/wal_sync/wal_write_time/wal_sync_time`).
- `SelectStatWALQuery(version int) (string, int, [2]int)` (`wal.go:25-32`):
  ```go
  func SelectStatWALQuery(version int) (string, int, [2]int) {
      if version >= 180000 {
          return PgStatWALDefault, 7, [2]int{2, 5}
      }
      return PgStatWALPG14, 11, [2]int{2, 9}
  }
  ```

Key mechanics confirmed:
- **Return tuple**: `(queryTemplate, ncols, DiffIntvl)`.
- **`ncols`** = total columns the SELECT returns (used as the right border for `OrderKey` clamping).
- **`DiffIntvl [2]int`** = inclusive `[firstCol, lastCol]` zero-based column range that gets diffed.
  For WAL PG14: `[2,9]` — col 0 = `source` (text, not diffed), col 1 = `waldir_size` (pretty,
  not diffed), cols 2..9 = the eight counter columns, col 10 = `stats_age` (excluded). So
  `stats_age` is excluded simply by being **outside** the `DiffIntvl` upper bound.
- **How exclusion works at runtime**: `internal/stat/postgres.go:diff()` (`postgres.go:303-358`).
  Inner loop `for l := 0; l < curr.Ncols; l++`: if `l < interval[0] || l > interval[1]` the value
  is copied as-is (line 331-333); otherwise `diffPair()` subtracts prev from curr (line 336).
  **Implication for this feature**: any column NOT inside `DiffIntvl` is rendered as the raw
  current (absolute) value. This is exactly the mechanism we exploit to show
  `ckpt_timed/ckpt_req/rstpt_*` as absolute — they must sit OUTSIDE the diff range.

### Constraint this imposes on column layout (critical design point)
`DiffIntvl` is a **single contiguous range** `[lo, hi]`. There is no way to express "diff cols
2-4 and 8-12 but not 5-7". Therefore the layout MUST be:

```
col 0:        source            (text, absolute)
cols 1..A:    absolute event-counters: ckpt_timed, ckpt_req, rstpt_timed, rstpt_req, rstpt_done
cols A+1..Z:  diffed work/time/buffer columns: write_time, sync_time, buffers_*  (DiffIntvl = [A+1, Z])
col Z+1:      stats_age         (excluded, absolute, sits past DiffIntvl upper bound)
```

This matches the feature summary: absolute block right after `source`, one contiguous `DiffIntvl`,
`stats_age` last.

### `internal/query/wal_test.go` (test template)
- `Test_SelectStatWALQuery` (`wal_test.go:10-30`): table-driven. Each case asserts
  `{version, wantNcols, wantDiffIntvl}`. Versions covered: 140000, 150000, 170000, 180000.
  Only asserts the `ncols` and `DiffIntvl` returns (query string ignored via `_`).
- `Test_StatWALQueries` (`wal_test.go:33-55`): integration. Iterates
  `versions := []int{140000,150000,160000,170000,180000}`; formats the template via `Format(tmpl, opts)`
  with `NewOptions(version, "f", "off", 256, "public")`; connects with
  `postgres.NewTestConnectVersion(version)`; `t.Skipf` if the version container is unavailable;
  executes the query and asserts no error. **This is the exact template for `bgwriter_test.go`** —
  same two tests, swapped names and expected ncols/DiffIntvl per version.

---

## 2. View Registration — `internal/view/view.go`

### `View` struct (`view.go:9-31`)
Relevant fields for the new view: `Name`, `MinRequiredVersion`, `QueryTmpl`, `DiffIntvl`, `Ncols`,
`OrderKey`, `OrderDesc`, `ColsWidth`, `Msg`, `Filters`, and `NotRecordable bool` (`view.go:30`:
"When true, record/record.go:filterViews() skips this view.").

### The `wal` entry (`view.go:128-139`) — exact template
```go
"wal": {
    Name:               "wal",
    MinRequiredVersion: query.PostgresV14,
    QueryTmpl:          query.PgStatWALPG14,
    DiffIntvl:          [2]int{2, 9},
    Ncols:              11,
    OrderKey:           0,
    OrderDesc:          true,
    ColsWidth:          map[int]int{},
    Msg:                "Show WAL statistics",
    Filters:            map[int]*regexp.Regexp{},
},
```

### New `bgwriter` entry needed
A `"bgwriter"` map entry mirroring `wal`, with:
- `Name: "bgwriter"`
- `MinRequiredVersion: query.PostgresV14` (pgcenter floor; bgwriter exists on all supported versions)
- `QueryTmpl: query.PgStatBgwriterPG14` (the pre-17 default; `Configure()` overrides it per version)
- `DiffIntvl` / `Ncols`: placeholder values matching the PG14 branch (overridden by `Configure()`)
- `OrderKey: 0`, `OrderDesc: true`, `ColsWidth: map[int]int{}`, `Filters: map[int]*regexp.Regexp{}`
- `Msg: "Show bgwriter / checkpointer statistics"`
- **`NotRecordable: true`** — keeps it out of recording (see §5).

> Note: `Configure()` (below) overrides `QueryTmpl/Ncols/DiffIntvl` at runtime, so the literal map
> values are only the static defaults. The `wal` entry uses the PG14 values as defaults — follow
> the same convention.

### `Configure()` wiring (`view.go:302-338`)
The first loop (`view.go:307-325`) switches on view key `k` and calls the version-aware selector,
re-assigning `QueryTmpl/Ncols/DiffIntvl`. The `wal` case (`view.go:321-324`):
```go
case "wal":
    view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatWALQuery(opts.Version)
    v[k] = view
```
**New case needed** in the same switch:
```go
case "bgwriter":
    view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatBgwriterQuery(opts.Version)
    v[k] = view
```
The second loop (`view.go:328-335`) calls `query.Format(view.QueryTmpl, opts)` for ALL views to
build the final `Query` string — no change needed there; the bgwriter query goes through it
automatically.

### `NotRecordable` usage across views
Currently NO view in `view.go` sets `NotRecordable: true` (the field exists and `procpidstat` once
used it, but per `docs/decisions-log.md` [003-feat-procpidstat-record-report] it was removed when
procpidstat recording became supported). The bgwriter view will be the **only** view setting
`NotRecordable: true`. The field defaults to `false` (Go zero value) so no other view is affected.

### `VersionOK` (`view.go:341-343`)
`return version >= v.MinRequiredVersion`. With `MinRequiredVersion = query.PostgresV14`, the view
is available on all pgcenter-supported versions (PG14+).

---

## 3. Keybinding — `top/keybindings.go`

### Registration pattern (`keybindings.go:29-38`)
Each switch hotkey is a one-liner: `{"sysstat", '<char>', switchViewTo(app, "<viewname>")}`.
The `wal` binding (`keybindings.go:35`): `{"sysstat", 'w', switchViewTo(app, "wal")}`.

### Is `b` free? YES.
Full lowercase-key audit of the `"sysstat"` context (`keybindings.go:22-62`):
- Switch keys: `d, r, t, i, s, f, w, p, a, x` (lines 29-38).
- Other lowercase: `q(quit), <, ,(comma), l, ~, m, n, k, z, h, /, -, _`.
- **`b` is NOT bound.** (`B` uppercase IS bound — `keybindings.go:47` `showExtra diskstats` —
  but lowercase `b` is free. gocui treats `b` and `B` as distinct keys.)

### One-line addition needed (after `keybindings.go:35`, the `wal` line):
```go
{"sysstat", 'b', switchViewTo(app, "bgwriter")},
```

### `switchViewTo` handler (`top/config_view.go:101-125`)
`switchViewTo(app, c)` — the `default` branch (line 118-119) calls
`viewSwitchHandler(app.config, c)` directly for any plain view name (no multi-view cycling logic).
`bgwriter` falls into `default`, exactly like `wal`/`activity`/`replication`. No `*NextView`
helper or special-casing required. `viewSwitchHandler` (`config_view.go:189-193`) saves current
view, loads the requested one from `config.views`, and sends it on `viewCh`.

### Help text — `top/help.go`
`top/help.go:13-14` lists the general-action mode keys. To keep the help current, add `b` to the
mode list (e.g. line referencing `w WAL`). Cosmetic but expected for parity with `wal`.

---

## 4. Reset behavior — `top/reset.go`

### `resetStat()` (`reset.go:13-40`)
Header comment (`reset.go:10-12`): *"Reset statistics that belongs to current database and
pg_stat_statements stats. Don't reset shared stats, such as bgwriter or archiver."*

Mechanics: `Q` executes `query.ExecResetStats` (database-level reset via
`pg_stat_reset()`) plus optionally `ExecResetPgStatStatements`. It deliberately does **NOT** reset
the shared `bgwriter`/`checkpointer` counters (those require `pg_stat_reset_shared('bgwriter')` /
`pg_stat_reset_shared('checkpointer')`, which pgcenter never calls).

**Implication for the new view**: the `bgwriter` screen needs NO reset handling. Pressing `Q`
while on the bgwriter screen will reset DB-level stats but leave bgwriter/checkpointer counters
untouched. The absolute event-counters (`ckpt_timed`, etc.) will keep growing monotonically — which
is the intended UX (DBAs want the cumulative timed-vs-requested ratio). `stats_reset` /
`stats_age` only changes if an operator runs `pg_stat_reset_shared(...)` out-of-band. No code
change in `reset.go`.

---

## 5. Recording skip — `record/record.go` `filterViews()`

### `filterViews()` (`record.go:200-233`)
First check in the loop (`record.go:206-212`):
```go
if v.NotRecordable {
    delete(views, k)
    filtered++
    continue
}
```
This runs in `app.setup()` (`record.go:86`) before `views.Configure(opts)`. Setting
`NotRecordable: true` on the bgwriter view guarantees:
- `pgcenter record` **never collects** it (deleted from the views map → never queried, never written
  to the tar archive).
- Because it is never recorded, `pgcenter report` can never encounter a bgwriter entry → no
  report-side parsing path. This is the **issue #122 avoidance**: report-side breakage (the
  "invalid result" tar-file bug, ref. commit f055b3a) is structurally impossible for a view that
  was never recorded.

### Report-side confirmation — `cmd/report/report.go`
The report command maps a CLI flag to a view name in a `switch` (`report.go:138-166`,
e.g. `case opts.showWAL: return "wal"` at line 145). For bgwriter we add **no** flag and **no**
case — there is intentionally no `--bgwriter` report flag. The view is TUI-only by design.

---

## 6. Version constants — `internal/query/query.go`

Defined in `query.go:9-18`:
```go
PostgresV94 = 90400
PostgresV95 = 90500
PostgresV96 = 90600
PostgresV10 = 100000
PostgresV11 = 110000
PostgresV12 = 120000
PostgresV13 = 130000
PostgresV14 = 140000
```

**Note**: there is NO `PostgresV17` or `PostgresV18` constant. The `wal.go` selector uses the raw
numeric literal `180000` for the PG18 branch (`wal.go:26`). Follow the same convention in
`bgwriter.go`: use `query.PostgresV14` for `MinRequiredVersion`, and raw literals `170000` / `180000`
inside `SelectStatBgwriterQuery` for the version branches. (Do NOT add new constants unless the
feature explicitly calls for it — matches "minimum code" rule.)

---

## 7. Column Inventory — `pg_stat_bgwriter` and `pg_stat_checkpointer` per version

### PG14, PG15, PG16 — everything in `pg_stat_bgwriter` (no `pg_stat_checkpointer`)
`pg_stat_checkpointer` does NOT exist before PG17. Columns of `pg_stat_bgwriter` (identical across
PG14/15/16; type bigint unless noted):

| column | meaning | semantics in feature |
|---|---|---|
| `checkpoints_timed` | scheduled checkpoints | ABSOLUTE (event counter) |
| `checkpoints_req` | requested checkpoints | ABSOLUTE (event counter) |
| `checkpoint_write_time` (double precision, ms) | time writing checkpoint buffers | DIFFed |
| `checkpoint_sync_time` (double precision, ms) | time syncing checkpoint files | DIFFed |
| `buffers_checkpoint` | buffers written by checkpoints | DIFFed |
| `buffers_clean` | buffers written by bgwriter | DIFFed |
| `maxwritten_clean` | times bgwriter stopped (hit `bgwriter_lru_maxpages`) | DIFFed |
| `buffers_backend` | buffers written directly by backends | DIFFed |
| `buffers_backend_fsync` | backend fsync calls | DIFFed |
| `buffers_alloc` | buffers allocated | DIFFed |
| `stats_reset` (timestamptz) | last reset | → `stats_age`, excluded |

> Note: `pg_stat_bgwriter` has no restartpoint columns pre-17.

### PG17 — split into two views (VERIFIED against live PostgreSQL 17.7)
Queried `pg_attribute` on a running PG17.7 cluster:

`pg_stat_bgwriter` (PG17) — 4 columns:
```
buffers_clean, maxwritten_clean, buffers_alloc, stats_reset
```
(`checkpoints_*`, `checkpoint_*_time`, `buffers_checkpoint`, `buffers_backend`,
`buffers_backend_fsync` all REMOVED from bgwriter. `buffers_backend*` moved to `pg_stat_io`,
out of scope per the spec.)

`pg_stat_checkpointer` (PG17) — 9 columns (verified with types):
```
num_timed            bigint
num_requested        bigint
restartpoints_timed  bigint
restartpoints_req    bigint
restartpoints_done   bigint
write_time           double precision
sync_time            double precision
buffers_written      bigint
stats_reset          timestamp with time zone
```

Mapping PG17 → feature columns:
- `num_timed` → `ckpt_timed` (was `checkpoints_timed`) — ABSOLUTE
- `num_requested` → `ckpt_req` (was `checkpoints_req`) — ABSOLUTE
- `restartpoints_timed` → `rstpt_timed` — ABSOLUTE
- `restartpoints_req` → `rstpt_req` — ABSOLUTE
- `restartpoints_done` → `rstpt_done` — ABSOLUTE
- `write_time` (ms) → `write_time` (was `checkpoint_write_time`) — DIFFed
- `sync_time` (ms) → `sync_time` (was `checkpoint_sync_time`) — DIFFed
- `buffers_written` → `buffers_ckpt` (was `buffers_checkpoint`) — DIFFed
- bgwriter `buffers_clean`, `maxwritten_clean`, `buffers_alloc` — DIFFed
- `stats_reset` → `stats_age` (feature decision: use checkpointer's `stats_reset`; documented)

> `restartpoints_timed/req/done` were INTRODUCED in PG17 (alongside `pg_stat_checkpointer`). They
> do NOT exist pre-17.

### PG18 — `pg_stat_checkpointer` GAINED a column (verify before final query)
PG18 added **`slru_written`** to `pg_stat_checkpointer` (buffers written during SLRU checkpoints),
in addition to the PG17 set. Expected PG18 `pg_stat_checkpointer` columns:
```
num_timed, num_requested, restartpoints_timed, restartpoints_req, restartpoints_done,
write_time, sync_time, buffers_written, slru_written, stats_reset
```
`pg_stat_bgwriter` in PG18 is unchanged from PG17 (`buffers_clean, maxwritten_clean, buffers_alloc,
stats_reset`).

**ACTION FOR IMPLEMENTER — verify on a live PG18 cluster before writing the query.** The local box
has only PG17 installed; PG14/15/16/18 clusters were NOT running at research time
(`pg_lsclusters` shows only `17 main`). Run `testing/prepare-test-environment.sh` (creates PG14-18
local clusters on ports 21914-21918) or use the CI matrix, then confirm with:
```sql
SELECT attname, format_type(atttypid, atttypmod)
FROM pg_attribute
WHERE attrelid IN ('pg_stat_bgwriter'::regclass, 'pg_stat_checkpointer'::regclass)
  AND attnum > 0 AND NOT attisdropped ORDER BY attrelid, attnum;
```
Decision needed: whether to surface `slru_written` as an extra DIFFed column on PG18, or keep the
unified "variant A" header set and ignore it. The interview specifies **variant A: unified column
headers across versions** (`interview.yml:106-107`), which argues for OMITTING `slru_written` to
keep headers identical PG14-18. Confirm with the user during tech-spec.

### Summary table of query branches needed in `SelectStatBgwriterQuery`
| version range | source views | checkpoint counters | restartpoints | notes |
|---|---|---|---|---|
| 14000–169999 (PG14-16) | `pg_stat_bgwriter` only | `checkpoints_timed/_req` | — (n/a) | restartpoint cols emitted as `NULL`/`0` literals to keep unified header, OR omitted (variant A decision) |
| 170000–179999 (PG17) | `pg_stat_bgwriter` + `pg_stat_checkpointer` | `num_timed/_requested` | `restartpoints_*` | join the two views (single-row each, cross join) |
| 180000+ (PG18) | `pg_stat_bgwriter` + `pg_stat_checkpointer` | `num_timed/_requested` | `restartpoints_*` | `slru_written` available; decide per variant A |

> Variant A (unified headers) means pre-17 must emit placeholder columns for restartpoints (which
> don't exist before PG17). The query for PG14-16 will need literal `NULL AS rstpt_timed` etc., or
> the column set differs and `Ncols`/`DiffIntvl` differ per branch (like wal.go already does for
> PG18). Clarify the exact unified-header contract in tech-spec; both approaches are mechanically
> supported by `SelectStatBgwriterQuery` returning per-version `(query, ncols, DiffIntvl)`.

---

## 8. Integration Points (summary of files to touch)

| File | Change | Reference line |
|---|---|---|
| `internal/query/bgwriter.go` | NEW: query consts + `SelectStatBgwriterQuery(version) (string,int,[2]int)` | template: `internal/query/wal.go` |
| `internal/query/bgwriter_test.go` | NEW: unit + integration tests | template: `internal/query/wal_test.go` |
| `internal/view/view.go` | add `"bgwriter"` map entry (~`view.go:128`) + `case "bgwriter"` in `Configure()` (~`view.go:321`) | `wal` entry/case |
| `top/keybindings.go` | add `{"sysstat", 'b', switchViewTo(app, "bgwriter")}` after line 35 | `wal` binding line 35 |
| `top/help.go` | add `b` to mode help line | `help.go:13` |
| `top/reset.go` | NO change (shared stats already excluded) | comment `reset.go:10-12` |
| `record/record.go` | NO change (`NotRecordable` already handled by `filterViews`) | `record.go:206-212` |
| `cmd/report/report.go` | NO change (no report flag, view is TUI-only) | mapping `report.go:138-166` |

The data path is purely SQL: view → `Configure()` formats query → Collector runs it → `diff()`
applies `DiffIntvl`. No enrichment, no procfs, no `CollectExtra`. Strictly simpler than
`procpidstat`; structurally identical to `wal`.

---

## 9. Existing Tests in the area

- Framework: `testify/assert`, plain `go test`, run via `make test` (`-race -p 1 -timeout 300s`,
  `Makefile:33-35`). Note `-p 1` (serial packages) — relevant because integration tests hit shared
  PG clusters.
- Test connection helper: `internal/postgres/testing.go` —
  `NewTestConnectVersion(version)` (`testing.go:16-44`) maps version → port
  (`140000:21914 … 180000:21918`), returns error if unavailable; callers `t.Skipf`.
- Diff regression coverage already exists for the WAL/stats_age boundary:
  `internal/stat/postgres_test.go:Test_diff_pg18_wal_stats_age` (`postgres_test.go:341`) — proves
  that putting `stats_age` outside `DiffIntvl` is the tested, correct approach. Same guarantee
  applies to bgwriter's absolute columns.
- Representative signatures (from `wal_test.go`):
  - `Test_SelectStatWALQuery(t *testing.T)` — table of `{version, wantNcols, wantDiffIntvl}`.
  - `Test_StatWALQueries(t *testing.T)` — loops PG14-18, formats, connects, executes, asserts.

---

## 10. Potential Problems / Risks

1. **PG18 `slru_written` unverified.** Local box has only PG17. The PG18 column set MUST be
   confirmed on a live PG18 before finalizing the query (see §7). Do not write the PG18 branch from
   memory.
2. **Variant A unified-header vs. per-version columns tension.** Pre-17 has no restartpoint columns;
   PG18 has an extra `slru_written`. A truly unified header forces placeholder/omitted columns and
   careful `Ncols`/`DiffIntvl` bookkeeping per branch. `wal.go` already sets a precedent for
   *different* `Ncols`/`DiffIntvl` per version (PG18 = 7 cols vs PG14 = 11), so per-version layouts
   are acceptable if variant A proves awkward. Pin this down in tech-spec.
3. **PG17 two-view join.** `pg_stat_bgwriter` and `pg_stat_checkpointer` are each single-row views;
   the PG17/18 query must cross-join them (`FROM pg_stat_bgwriter, pg_stat_checkpointer`) — single
   row × single row = one row, consistent with the single-row view contract.
4. **`stats_age` source ambiguity on PG17+.** The two views have independent `stats_reset` timestamps
   (separate `pg_stat_reset_shared('bgwriter')` vs `('checkpointer')`). Feature decision
   (`interview.yml:42-44`): show the **checkpointer's** `stats_age`, and DOCUMENT it so a divergent
   bgwriter reset is not a surprise. Implementer must select `stats_reset` from the checkpointer
   view, not bgwriter, on PG17+.
5. **`NotRecordable: true` is temporary** (per spec, recording deferred). This is the same field
   pattern documented in `docs/decisions-log.md` [001] (added) and superseded by [003] (removed for
   procpidstat). bgwriter will be the sole current user of the flag. When recording is added later,
   the flag is simply dropped — no architectural debt.
6. **overview.md inaccuracy** (`interview.yml:77-78`): project knowledge wrongly claims bgwriter is
   already supported. The `done`/documentation step should correct `project-knowledge/overview.md`.
7. **No ADR conflicts.** `docs/decisions-log.md` [001]/[003] (the `NotRecordable` lineage) are the
   only relevant entries and they SUPPORT this design (the flag exists and works). No settled
   decision is contradicted.
8. **No active tech-debt in touched modules.** `docs/tech-debt.md` active items [003] (self-reviews)
   and the resolved [002] (procpidstat recording) do not touch `internal/query/`,
   `internal/view/view.go`, or the bgwriter path. No debt to inherit.

---

## 11. Constraints & Infrastructure

- Go 1.25+, cobra CLI, pgx/v5 driver, gocui TUI, testify (per `.claude/CLAUDE.md`).
- Test clusters: created by `testing/prepare-test-environment.sh` as **local PG clusters** (not
  Docker) on ports 21914-21918 (PG14-18), DB `pgcenter_fixtures`, user `postgres`, trust auth,
  `shared_preload_libraries = 'pg_stat_statements'`. CI provides the full matrix; the dev box here
  had only PG17 running.
- `make test` runs with `-p 1` (serial) and a 300s timeout; integration tests skip gracefully on
  missing versions via `NewTestConnectVersion` + `t.Skipf`.
- Lint gates: `make lint` (golangci-lint + gosec), `make vuln` (govulncheck). The new query strings
  are static (no user interpolation into the bgwriter SQL), so gosec SQL-injection findings are not
  expected — match the `wal.go` style exactly (plain const strings, `Format()` only substitutes
  template vars, none needed for bgwriter).
