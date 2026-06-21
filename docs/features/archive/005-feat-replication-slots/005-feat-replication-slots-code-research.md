# Code Research: 005-feat-replication-slots

New multi-row TUI screen `replslots` (hotkey `o`) in `pgcenter top`, one row per replication slot.
HYBRID source: `pg_replication_slots LEFT JOIN pg_stat_replication_slots ON slot_name`. State columns
absolute, logical-decoding cumulative counters diffed. TUI-only (`NotRecordable: true`), PG 14-18.

Research date: 2026-06-21. Code reading only — no production code written.

All paths absolute. Ignore `.claude/worktrees/*` copies (stale agent snapshots) — only the live tree is authoritative.

---

## 1. Entry Points & Wiring (add a new view + hotkey)

This is the same surface area touched by feature 004 (bgwriter, `b`). Five files change.

### 1.1 View definition — `/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/view/view.go`

- `View` struct (lines 9-31): fields relevant to this feature — `MinRequiredVersion`, `QueryTmpl`,
  `DiffIntvl [2]int`, `Ncols int` ("right border for OrderKey"), `OrderKey int`, `OrderDesc bool`,
  `UniqueKey int` ("index of column used as unique key when comparing rows during diffs, default 0"),
  `Filters map[int]*regexp.Regexp`, `NotRecordable bool`.
- `New() Views` (lines 37-310): the views map. Add a `"replslots"` entry. The closest template is
  `bgwriter` (lines 140-152): `MinRequiredVersion: query.PostgresV14`, `QueryTmpl: query.PgStat<...>PG14`,
  `DiffIntvl`, `Ncols`, `OrderKey: 0`, `OrderDesc: true`, `ColsWidth: map[int]int{}`, `Msg`, `Filters: map[int]*regexp.Regexp{}`,
  `NotRecordable: true`.
- `Configure(opts query.Options) error` (lines 315-354): the per-version selector switch. Add a
  `case "replslots":` that calls `query.SelectStatReplicationSlotsQuery(opts.Version)` and assigns the
  returned `(QueryTmpl, Ncols, DiffIntvl)` — identical to the `case "bgwriter":` block at lines 337-339:
  ```go
  case "bgwriter":
      view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatBgwriterQuery(opts.Version)
      v[k] = view
  ```
  After this switch, the second loop (lines 344-351) runs `query.Format(view.QueryTmpl, opts)` for every
  view — this is where `{{.WalFunction1}}`/`{{.WalFunction2}}` get substituted (see §2.3). The view's
  `QueryTmpl` MUST contain the template placeholders, not a finished query, for recovery-aware LSN to work.
- `query.PostgresV14 = 140000` is defined in query.go; there are NO `PostgresV15..V18` constants — the
  codebase uses raw ints (`170000`, `180000`) in selectors (see bgwriter/wal). Match that.

### 1.2 Hotkey — `/home/lesovsky/Git/github.com/lesovsky/pgcenter/top/keybindings.go`

- `keybindings(app *app) error` builds a `[]key` slice; each entry is `{context, rune, handler}`.
- View-switch bindings are lines 29-38, all `{"sysstat", '<rune>', switchViewTo(app, "<viewname>")}`. bgwriter is
  line 36: `{"sysstat", 'b', switchViewTo(app, "bgwriter")}`.
- Add: `{"sysstat", 'o', switchViewTo(app, "replslots")}`.
- **Hotkey `o` is FREE.** Bound lowercase: a,b,d,f,h,i,k,l,m,n,p,q,r,s,t,w,x,z. Bound uppercase:
  A,B,C,D,E,F,G,I,K,L,N,P,Q,R,S,X. `o` appears nowhere in keybindings.go.

### 1.3 Switch handler (no new code needed) — `/home/lesovsky/Git/github.com/lesovsky/pgcenter/top/config_view.go`

- `switchViewTo(app *app, c string) func(...)` (lines 102-125). The `switch c` has special cases only for
  the multi-sub-view families: `databases`, `statements`, `progress` (which call `*NextView()` cyclers).
  Every direct single-screen view (`replication`, `wal`, `bgwriter`) falls through to `default:` →
  `viewSwitchHandler(app.config, c)` (line 119). `replslots` is a direct view: **it needs only the keybinding,
  no NextView function and no case in switchViewTo.**
- `viewSwitchHandler(config *config, c string)` (lines 189-193) does `config.view = config.views[c]; config.viewCh <- config.view`.
  The string `c` ("replslots") must equal the views-map key in view.go.

### 1.4 Help text — `/home/lesovsky/Git/github.com/lesovsky/pgcenter/top/help.go`

- `helpTemplate` const (lines 10-42), printed via `fmt.Fprintf(v, helpTemplate, versionStr)` in `showHelp` (line 57).
- The view-hotkey list is lines 13-14:
  ```
  a,b,f,r,w   mode: 'a' activity, 'b' bgwriter/checkpointer, 'f' functions, 'r' replication, 'w' WAL,
  s,t,i             's' tables sizes, 't' tables, 'i' indexes.
  ```
  Add `'o' replication slots` to the mode line (e.g. extend line 13 or add `o` to the leading key list).
  This is plain text — no per-key registry, just edit the string.

### 1.5 Record filter + test — `/home/lesovsky/Git/github.com/lesovsky/pgcenter/record/record.go` & `record_test.go`

- `filterViews(version int, pgssSchema string, views view.Views) (int, view.Views)` — record.go lines 200-233.
  For each view with `NotRecordable: true` it does `delete(views, k); filtered++; continue` (lines 204-212).
  So a `NotRecordable` view is dropped on EVERY version.
- `Test_filterViews` — record_test.go lines 101-129. Asserts `wantN` (filtered count, line 126) and
  `wantV` (remaining count, `len(v)`, line 127). Test cases lines 116-121.
- **Required test bump:** adding `replslots` with `NotRecordable: true` increments `wantN` by 1 on EVERY
  row; `wantV` is unchanged (the view never joins the recordable set). This exactly mirrors what commit
  `435f54c` did for bgwriter ("raising wantN by 1 per row"). CI will fail if not bumped. Current values
  to bump (all `wantN` +1): {140000,"":7→8/wantV 16}, {140000,"public":1→2/22}, {130000:4→5/19},
  {120000:7→8/16}, {110000:9→10/14}, {100000:9→10/14}. Note: `replslots` has `MinRequiredVersion=PostgresV14`,
  so on <14 rows it is ALSO version-filtered — but it is still counted exactly once in `wantN` (the
  `NotRecordable` branch `continue`s before the version branch), so +1 holds uniformly.

---

## 2. Query Layer

### 2.1 bgwriter precedent — `/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/query/bgwriter.go`

- File is a `const (...)` block of query strings (`PgStatBgwriterPG14/PG17/PG18`) plus one selector.
- **Selector signature to follow:**
  ```go
  func SelectStatBgwriterQuery(version int) (string, int, [2]int)   // (query, Ncols, DiffIntvl)
  ```
  Returns the version-appropriate query string, its column count, and the `[lo,hi]` diff interval
  (bgwriter.go lines 41-52). The new selector should be `SelectStatReplicationSlotsQuery(version int) (string, int, [2]int)`.
- Absolute-vs-diffed via column placement: absolute state counters sit OUTSIDE `[lo,hi]`, diffed counters
  inside. bgwriter PG14: `[2]int{3,10}` — ckpt counters at cols 1-2 absolute, stats_age col 11 absolute.

### 2.2 wal precedent (closest shape) — `/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/query/wal.go`

- `SelectStatWALQuery(version int) (string, int, [2]int)` (lines 26-35). Demonstrates the `stats_age`
  column placed as the LAST column, OUTSIDE the diff range: PG14 `Ncols=11, DiffIntvl=[2]int{2,9}` (col 10
  = `date_trunc('seconds', now() - stats_reset)::text AS stats_age`, absolute). **This is the exact pattern
  for replslots' `stats_age` (from `pg_stat_replication_slots.stats_reset`).**

### 2.3 replication.go template mechanism — `/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/query/replication.go`

- Query strings embed `{{.WalFunction1}}` and `{{.WalFunction2}}` (e.g. lines 5-15). `WalFunction1` =
  the LSN-diff function (`pg_wal_lsn_diff`); `WalFunction2` = the current-position function. Usage:
  `({{.WalFunction1}}({{.WalFunction2}}(), replay_lsn) / 1024)::bigint`.
- `SelectStatReplicationQuery(version int, track bool) (string, int)` (lines 56-69) — note this returns
  only `(string, int)`, NOT `[2]int`; `replication`'s `DiffIntvl` is hardcoded `[2]int{6,6}` in view.go
  (line 53) and is NOT touched by `Configure`. For replslots, prefer the bgwriter/wal shape that returns
  `DiffIntvl` too, so per-version column shifts are handled in one place.

### 2.4 Template substitution — `/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/query/query.go`

- `Options` struct (lines 21-34): carries `Version`, `Recovery`, `WalFunction1`, `WalFunction2`, etc.
- `NewOptions(version int, recovery string, track string, querylen int, pgssSchema string) Options`
  (lines 37-59): sets `opts.WalFunction1, opts.WalFunction2 = selectWalFunctions(opts.Version, opts.Recovery)`.
- `selectWalFunctions(version, recovery)` (lines 64-83) — **recovery-aware** (see §5):
  - `version >= PostgresV10`: `fn1 = "pg_wal_lsn_diff"`; `fn2 = "pg_current_wal_lsn"` if `recovery=="f"`
    else `"pg_last_wal_receive_lsn"`.
- `Format(tmpl string, o Options) (string, error)` (lines 86-99): `text/template` execution that resolves
  the `{{.WalFunction*}}` placeholders. Called by `view.Configure` for every view (view.go line 345).

**Retained WAL implication:** `retained_wal = {{.WalFunction1}}({{.WalFunction2}}(), restart_lsn)` gives the
recovery-correct byte distance with zero new recovery logic — the template already chooses
`pg_current_wal_lsn()` (primary) vs `pg_last_wal_receive_lsn()` (standby). This is an ABSOLUTE column → place
it OUTSIDE `DiffIntvl`. `safe_wal_size` (bytes, nullable) is also absolute → outside `DiffIntvl`.

### 2.5 Query tests — `bgwriter_test.go` / `replication_test.go`

- `/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/query/bgwriter_test.go`:
  - `Test_SelectStatBgwriterQuery` (lines 10-32): table of `{version, wantNcols, wantDiffIntvl}`, asserts the
    `(_, Ncols, DiffIntvl)` return for each PG version. **This is the unit test to mirror.**
  - `Test_StatBgwriterQueries` (lines 35-63): integration. For each version it `Format`s the query, calls
    `postgres.NewTestConnectVersion(version)` (skips if unavailable), runs `conn.Query(q)`, and asserts
    `assert.Len(t, rows.FieldDescriptions(), wantNcols)` — a live schema-divergence gate.
- `/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/query/replication_test.go`:
  - `Test_StatReplicationQueries` (lines 38-67): uses `conn.Exec(q)` (execute-only, tolerates empty result
    set — the tier-1 pattern for views that are empty in test clusters).

---

## 3. DiffIntvl Mechanics & Multi-Row Row Identity (CRITICAL)

`/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/stat/postgres.go`

- `Compare(curr, prev, itv, interval [2]int, skey, desc, ukey)` (line 273) → `calculateDelta` (line 278) →
  `diff` (line 303). If `interval == [2]int{0,0}` diffing is skipped (no-diff views).
- **DiffIntvl is a single contiguous `[lo,hi]` range.** In `diff` (lines 330-343):
  ```go
  for l := 0; l < curr.Ncols; l++ {
      if l < interval[0] || l > interval[1] {
          // copy value as-is (absolute)
      } else {
          // diffPair(curr, prev, itv)  (diffed)
      }
  }
  ```
  Columns outside `[lo,hi]` pass through unchanged. Confirms the absolute-outside / diffed-inside design and
  forces a contiguous diffed block — so the column ORDER in the SELECT must group the 8 logical-decoding
  counters together, with state + retained_wal + safe_wal_size + stats_age outside that block.
- **Row identity across samples (the slot appear/disappear/reorder problem):** `diff` matches rows by a KEY,
  not by position. Lines 321-325:
  ```go
  for j, pv := range prev.Values {
      if cv[ukey].String != pv[ukey].String { continue }  // match on column ukey
      ...
  }
  ```
  `ukey` is the view's `UniqueKey` field (default 0 = first column). Rows present in curr but absent in prev
  are appended as-is (no diff); rows present only in prev are dropped (lines 348-353). Sorting happens after
  (`delta.sort(skey, desc)`).
  - `replication` view uses default `UniqueKey: 0` (pid is col 0 → stable key).
  - `tables`/`databases_general` use `UniqueKey: 0` (col 0 is the object name/oid → stable).
  - `statements_*` set `UniqueKey` explicitly (e.g. `statements_timings` `UniqueKey: 11` = queryid) because
    col 0 is not unique.
  - **For replslots: `slot_name` is the natural unique, stable identity.** Put `slot_name` at col 0 and
    leave `UniqueKey: 0` (default). This makes slot add/drop/reorder between samples safe: a newly-appeared
    slot shows raw counters for one interval, a dropped slot vanishes — exactly the existing `replication`
    behavior. No new diffing code is required.

---

## 4. Sorting / Filtering for Multi-Row Views

- `sort(key, desc)` (postgres.go lines 361-399): auto-detects numeric / duration / string per the sample
  value of the sort column. `OrderKey` selects the column, `OrderDesc` the direction.
- `Ncols` is the right border for `OrderKey` cycling (left/right arrow keys). It MUST equal the actual column
  count of the version's query (the integration test `assert.Len(rows.FieldDescriptions(), Ncols)` enforces this).
- `Filters map[int]*regexp.Regexp` — per-column regexp filters, applied via the `/` hotkey; initialize empty
  `map[int]*regexp.Regexp{}` like every other view.
- **Recommended defaults:** `OrderKey: 0` (slot_name) `OrderDesc: true`, matching `replication`/`tables`. (A
  case can be made to default-sort by retained WAL bytes descending for incident response, but that requires
  `OrderKey` = the retained_wal column index; the established default across all multi-row views is col 0.)

---

## 5. Recovery Mode (standby detection)

**Already fully handled by the existing template machinery — no recovery branch / CASE needed in the query.**

- Recovery state originates from `GetRecoveryStatus = "SELECT pg_is_in_recovery()"`
  (`/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/query/common.go:7`) and from `SelectCommonProperties`
  (common.go ~line 49: `pg_is_in_recovery() AS recovery`). Value is the boolean text `t`/`f`.
- It is stored in `stat.PostgresProperties.Recovery` (`internal/stat/postgres.go:111`) and flows:
  `top/top.go:60` → `query.NewOptions(props.VersionNum, props.Recovery, ...)` → `selectWalFunctions` →
  `Options.WalFunction2` = `pg_current_wal_lsn` (primary, `recovery=="f"`) or `pg_last_wal_receive_lsn`
  (standby) → `view.Configure` runs `Format` substituting the placeholder.
- So `retained_wal = {{.WalFunction1}}({{.WalFunction2}}(), restart_lsn)` is automatically recovery-correct.
  Same path is used by `record.go:84` and `report/report.go:250`, so any future record support inherits it.
- Caveat: on a standby, `pg_replication_slots` only shows slots that exist on that standby (cascading /
  failover-synced slots). That is correct PG behavior, not a pgcenter concern.

---

## 6. Version Differences: `pg_replication_slots` & `pg_stat_replication_slots` (PG 14-18)

Source: PostgreSQL 18 docs (Context7), cross-checked against the chosen column subset.

### 6.1 `pg_replication_slots` — chosen subset is stable across PG 14-18

The chosen columns and their introduction version:
- `slot_name`, `slot_type` (physical/logical), `active` — all present since the view's inception (pre-9.4 era).
- `restart_lsn` — present PG 9.4+. Needed for retained WAL.
- `wal_status` — **PG 13+** (`reserved`/`extended`/`unreserved`/`lost`). Present on all of 14-18. OK.
- `safe_wal_size` — **PG 13+** (bytes before slot risks `lost`; NULL when wal_status is `lost`/unbounded). OK on 14-18.

Columns NOT in the chosen subset (so NO per-version branching is required if you stop here):
- `two_phase` — PG 14+ (would be safe on 14-18, but not in subset).
- `conflicting` — **PG 16+** (boolean; would break PG 14-15 if selected).
- `failover`, `synced` — **PG 17+** (would break PG 14-16 if selected).
- `invalidation_reason` — **PG 18+** (would break PG 14-17 if selected).

**Conclusion:** With the subset `{slot_name, slot_type, active, wal_status, restart_lsn (→retained_wal),
safe_wal_size}`, the `pg_replication_slots` half needs ZERO per-version branching across PG 14-18. The
selector can return a single query string for all versions. (Adding `conflicting`/`invalidation_reason` would
force version branching à la bgwriter — out of scope per the interview's "exact subset" gate; flag for the spec.)

### 6.2 `pg_stat_replication_slots` — schema stable PG 14-18, LOGICAL-ONLY

- Introduced PG 14; columns unchanged through 18: `slot_name`, `spill_txns`, `spill_count`, `spill_bytes`,
  `stream_txns`, `stream_count`, `stream_bytes`, `total_txns`, `total_bytes`, `stats_reset`.
- **One row per LOGICAL slot only.** Physical slots are absent → the `LEFT JOIN ... ON slot_name` yields NULL
  for all 8 counters + `stats_reset` on physical-slot rows. The diff machinery copies NULLs through unchanged
  (sql.NullString with Valid=false), so physical rows render empty in the diffed block — matches the requirement.
- `stats_reset` → `stats_age` column: `date_trunc('seconds', now() - s.stats_reset)::text` (per wal.go pattern),
  absolute, placed last/outside DiffIntvl. NULL for physical slots.
- Reset is per-slot via `pg_stat_reset_replication_slot(text)` (superuser by default) — informational only;
  pgcenter's existing `Q` reset hotkey targets `pg_stat_reset()`, unrelated.

### 6.3 Proposed column layout (illustrative — final set is the spec's Cycle-2 decision)

```
col 0  slot_name            (absolute, UniqueKey/OrderKey)
col 1  slot_type            (absolute)
col 2  active               (absolute)
col 3  wal_status           (absolute)
col 4  retained_wal,KiB     (absolute; {{.WalFunction1}}({{.WalFunction2}}(),restart_lsn))
col 5  safe_wal_size        (absolute, nullable)
col 6  spill_txns           ┐
col 7  spill_count          │
col 8  spill_bytes          │
col 9  stream_txns          │ DIFFED  → DiffIntvl=[2]int{6,13}
col 10 stream_count         │
col 11 stream_bytes         │
col 12 total_txns           │
col 13 total_bytes          ┘
col 14 stats_age            (absolute)            → Ncols=15
```
Single query for all PG 14-18 → `SelectStatReplicationSlotsQuery(version)` returns the same string but the
function/test still version-keys for symmetry and future `conflicting`/`invalidation_reason` additions.

---

## 7. Testing Harness

- Connection helpers — `/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/postgres/testing.go`:
  `NewTestConnectVersion(version)` maps versions→ports (14→21914 … 18→21918); active versions are 14-18 in
  image `pgcenter-testing:0.0.9+`. Returns error (caller `t.Skipf`s) if a version isn't running.
- Cluster setup — `/home/lesovsky/Git/github.com/lesovsky/pgcenter/testing/prepare-test-environment.sh`:
  creates PG 14-18 clusters; `auto.conf` sets only `shared_preload_libraries = 'pg_stat_statements'`. There
  is **no explicit `wal_level`**, so it defaults to `replica`. There is **no `test_decoding` / contrib** loaded.
- **Tier 1 (execute-only, empty set):** works as-is. Mirror `Test_StatReplicationQueries` using `conn.Exec(q)`
  for unit-level execution validity, and the bgwriter-style `conn.Query(q)` + `assert.Len(FieldDescriptions, Ncols)`
  for the schema gate. The LEFT JOIN runs fine on zero slots.
- **Tier 2 (physical slot, MANDATORY per interview):** `wal_level=replica` is sufficient for
  `pg_create_physical_replication_slot('name', true)` (the `true` = immediately reserve WAL, so `restart_lsn`
  is non-NULL and retained WAL is exercised). **No image change.** Creatable as-is. Logical-decoding columns
  stay NULL (physical slot absent from `pg_stat_replication_slots`) — validates the LEFT-JOIN-NULL path.
  Test must drop the slot in cleanup (`pg_drop_replication_slot`) or it retains WAL on the shared fixture.
- **Tier 3 (logical slot — requires image bump, PENDING user decision):**
  `pg_create_logical_replication_slot('name','test_decoding')` needs **(a) `wal_level=logical`** in
  prepare-test-environment.sh AND **(b) the `test_decoding` output plugin**, shipped in the
  `postgresql-NN` PGDG package's contrib (`$libdir/test_decoding`) — present in the standard PGDG server
  packages, but the pgcenter-testing image must be rebuilt with `wal_level=logical` (image bump 0.0.9→0.0.10
  noted in the interview). Only Tier 3 exercises the diffed spill/stream/total counters with non-NULL data.
- No `view` package count-assertion test exists (grep found none), so adding the view to `New()` does not
  break a view-package test — only `record_test.go:Test_filterViews` must be bumped (§1.5).

---

## 8. Potential Problems / Constraints

- **`Test_filterViews` WILL fail in CI if not bumped** (`wantN` +1 on every row). This exact miss happened for
  bgwriter (fixed post-hoc in commit `435f54c`). Bump it in the same change. (§1.5)
- **Tech-debt [005] (Low) — `Test_doReload` panics instead of skipping** (`top/reload_test.go`) when PG
  fixture on 21917 is down. Pre-existing, unrelated to this feature, but it masks local detection of
  record-package regressions — meaning a missed `Test_filterViews` bump may not surface locally, only in CI.
  Suggested handling: be aware; run `make test` against live clusters or trust CI. Do not fix here unless asked.
- **ADR [004-feat-bgwriter-checkpointer] "Per-version column sets, not NULL-padded unified columns"** is a
  SETTLED decision. It applies here only if `conflicting`/`invalidation_reason` are added (they would force
  version-specific query strings). The chosen 14-18-stable subset sidesteps it — keep it that way unless the
  spec expands the column set.
- **ADR [004] "Absolute event counters via DiffIntvl placement"** and **"stats_age sourced from ... stats_reset"**
  directly govern this feature's column layout — follow them (state/retained/safe_wal/stats_age outside DiffIntvl;
  8 logical counters inside).
- **ADR [004] "NotRecordable: true for TUI-only scope"** — replslots inherits this; record/report deferred to
  the 0.11.0 backlog (roadmap-0.11.0.md line 86). After bgwriter, replslots becomes the second live `NotRecordable` user.
- **NULL handling in diff:** `diffPair` (postgres.go:445) calls `strconv.ParseInt/ParseFloat` and returns an
  error on a non-numeric string. For physical-slot rows the 8 counters are NULL → `sql.NullString.String == ""`.
  Because such a row has no prev match by `slot_name`? No — a physical slot DOES match itself across samples
  (same slot_name in both snaps), so it enters the diff branch with `curr=="" , prev==""`. **`diffPair("","",itv)`
  will hit `strconv.ParseInt("",...)` → error → `diff` returns that error → stats collection row skipped.**
  This is a REAL risk: verify whether empty-string NULLs in the diffed block break diffing. Mitigations to
  evaluate in the spec: (a) `coalesce(...,0)` the 8 counters in SQL so physical rows carry `0` not NULL (then
  they diff to 0 — clean, and matches "empty/zero for physical"); (b) confirm whether NULLs render as empty in
  the TUI without entering diffPair. Option (a) is the safe, low-risk choice and should be called out.
- **Shared-fixture side effects:** Tier-2 physical slot retains WAL on the shared test cluster; tests MUST
  drop it in cleanup to avoid WAL accumulation across the suite.
- **Privileges:** both views are world-readable (no superuser needed to SELECT); works over PgBouncer simple
  protocol. `safe_wal_size`/counters may be NULL but never permission-denied.

---

## 9. External Libraries

No new external libraries. PostgreSQL system views only (`pg_replication_slots`, `pg_stat_replication_slots`),
WAL-LSN functions (`pg_wal_lsn_diff`, `pg_current_wal_lsn`, `pg_last_wal_receive_lsn`) already wired through
`Options.WalFunction1/2`. Driver is pgx/v5 (unchanged); `sql.NullString` scanning in `NewPGresultQuery`
(postgres.go:168) already handles NULL columns from the LEFT JOIN.
