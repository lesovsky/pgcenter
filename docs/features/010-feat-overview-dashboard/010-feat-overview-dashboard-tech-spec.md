---
created: 2026-06-25
status: draft
branch: feature/010-overview-dashboard
size: L
---

# Tech Spec: Verbose Mode for the Top Summary Panels (Instance Overview)

## Solution

Add a persistent **verbose toggle** (hotkey `v`) that expands the two existing top panels —
`sysstat` (left, +3 rows) and `pgstat` (right, +5 rows) — with extra `label:value` rows of
aggregated instance-health metrics. Verbose is a display *mode*, not a new screen, so it reuses the
existing free-form `Fprintf` render path and registers **no new view** (zero view-count test churn).

The work splits into a render layer (the easy part — extend `printSysstat`/`printPgstat`), an
**invasive core** (verbose-aware `layout()` geometry that grows the top band and pushes
`cmdline`/`dbstat` down, plus a verbose-gated *all-three* system-stat collection branch), a data
layer (~8 new aggregate SQL queries + 4 new GUC reads + `data_directory`), and net-new formatting
helpers (integer ceil rounding, reserved-digit fixed width, dynamic rate-unit suffix). Collection
cost is hidden behind the single existing `z` interval knob via per-source tiering + a latency guard.

## Architecture

### What we're building/modifying

- **`view.View.Verbose bool` + `top.config.verbose bool`** — the mode flag, mirrored across all views
  (persists on screen switch, like `ShowExtra`) and readable by both collector (off the view) and
  renderer/layout (off config).
- **`v` keybinding + `toggleVerbose` handler** (`top/keybindings.go`, new `top/verbose.go`) — mirrors
  the `showExtra` pattern; help-screen entry.
- **`topBandLayout` pure function + `layout()` rewiring** (`top/ui.go` / new `top/layout.go`) — band
  height becomes a function of the verbose flag; `cmdline`/`dbstat` shift down; height-guard.
- **`printSysstat`/`printPgstat` → `io.Writer` split + verbose row composers** (`top/stat.go`) —
  behavior-preserving refactor, then verbose rows gated on the flag.
- **All-three system collection branch** (`internal/stat/stat.go`) — verbose-gated, after the existing
  mutually-exclusive `collectExtra` switch, `== nil` guards; does not disturb the side panels.
- **New aggregate queries + structs + GUC reads** (`internal/query/`, `internal/stat/postgres.go`,
  `internal/query/common.go`) — workload/databases/workers/replication/archiving + bgwr-ckpt reuse.
- **New formatters** (`internal/pretty` additions or a `top`-local helper) — ceil, reserved-digit,
  dynamic unit suffix.
- **Tiering/guard state on `Collector`** (`internal/stat/stat.go`) — per-source cadence for the dear
  aggregates (db sizes/growth), stale-value on throttle, first-tick `n/a` + `collecting...` hint.

### How it works

1. `v` → `toggleVerbose` flips `config.verbose` and `view.Verbose` (mirrored across the views map),
   pushes `config.view` on `viewCh` (redraw + collector sees the new flag). Not reset on screen switch.
2. `layout()` computes `topBandLayout(config.verbose, maxY)` → sizes `sysstat`/`pgstat`, shifts
   `cmdline`/`dbstat` down; if the terminal is too short, falls back to compact + cmdline hint.
3. `Collector.Update` sees `view.Verbose`: runs the all-three system collection (disk+net+fs, `==nil`
   guarded) every tick; runs the dear pgstat aggregates gated by the per-source tiering/latency guard,
   reusing cached (stale) values when throttled.
4. `printStat` → `printSysstat`/`printPgstat` render the 4 compact rows, then (if verbose) the extra
   rows from the same structs the full panels/screens use (consistency), rounded to integers.

## Decisions

### Decision 1: Verbose as a display mode, not a new view
**Decision:** Implement as a config/view flag toggled by `v`, expanding the existing panels.
**Rationale:** The two top panels already are a mini-overview; a separate screen would duplicate them
(antipattern) and trigger view-count test churn. A mode reuses the free-form render path and registers
no view → no `view_test.go`/`Test_filterViews` changes.
**Alternatives considered:** Separate full-screen `overview` view with a card grid — rejected (duplication
antipattern, invasive `printStat` branch, view-count test churn). Documented in user-spec pivot Q6.

### Decision 2: `view.Verbose bool` + `config.verbose bool`, not overloading `CollectExtra`
**Decision:** A dedicated boolean on both `view.View` (rides `viewCh` to the collector) and `top.config`
(read by renderer/layout), kept in sync by the handler (the `showExtra` mirror-into-all-views pattern).
**Rationale:** `CollectExtra` is a single mutually-exclusive `int` (with `CollectProcPidStat`); verbose
must coexist with the active view's enrichment and needs no `c.Reset()`. A separate bool is cleaner.
**Alternatives considered:** A `CollectVerboseSystem` `CollectExtra` constant (ADR [001]) — rejected (mutual
exclusivity + would fire the `prevCollectExtra` Reset path).

### Decision 3: Pure `topBandLayout` geometry function
**Decision:** Extract the band-height/coord arithmetic into a pure `topBandLayout(verbose, maxY)` and
keep `layout()` to `SetView` plumbing.
**Rationale:** The geometry (compact vs verbose, height-guard) is the invasive core touching the shared
UI loop; a pure function is table-testable without gocui (the [009] `visibleColumns` precedent).
**Alternatives considered:** Inline literal coords in `layout()` — rejected (untestable, error-prone).

### Decision 4: All-three system collection in a separate verbose branch
**Decision:** Add a verbose-gated branch *after* the existing `collectExtra` switch, collecting
disk+net+fs each tick with `== nil` guards.
**Rationale:** The side-panel switch is mutually exclusive (one panel at a time); leaving it untouched and
adding a parallel branch avoids disturbing iostat/nicstat/fsstat. Per-source prev/curr snapshots already
live on `Collector`, so collecting all three does not interfere. `==nil` avoids double-collecting when a
side panel already populated one source.
**Alternatives considered:** Modify the mutual-exclusion switch to collect all three — rejected (R1: would
change side-panel behavior).

### Decision 5: Reuse identical panel math for consistency; round to integers
**Decision:** Verbose system aggregates select the max-`%util` device and read the same
`Diskstats`/`Netdevs`/`Fsstats` structs the full panels render; verbose rounds to integers (ceil).
**Rationale:** The device/number a DBA sees in the verbose row must match the full `B`/`N`/`F` panel
exactly. Note `nicstat` rMbps/wMbps is computed at print time (`Rbytes/1024/128`) — verbose must replicate
that exact conversion.
**Alternatives considered:** Recompute aggregates independently — rejected (divergence risk).

### Decision 6: Archiving backlog via `count(.ready) × wal_segment_size`
**Decision:** `count(*)` of `.ready` entries in `pg_wal/archive_status` × `wal_segment_size`, pure SQL.
**Rationale:** Operationally that *is* the backlog; pure SQL (`pg_ls_dir`) works remote with no PL/Perl;
adapts the exact existing precedent `count(1) * pg_size_bytes(current_setting('wal_segment_size'))`
(`wal.go:6`). `n/a` on `archive_mode=off` / insufficient privileges (`pg_monitor`).
**Alternatives considered:** LSN-diff `current_lsn − LSN(last_archived_wal)` — rejected (filename→LSN
conversion is fiddly; lags non-linearly on archiver failure).

### Decision 7: `filesyst` = data_directory FS only; symlink resolved local-only
**Decision:** Show the filesystem hosting `data_directory`, matched by longest mount-prefix; resolve the
symlink via `filepath.EvalSymlinks` only when `db.Local`; remote matches the unresolved path.
**Rationale:** Covers the primary disk-fill case; remote realpath over the wire is out of scope.
WAL/tablespaces on other filesystems are a documented limitation.
**Alternatives considered:** Show all filesystems (multi-row — not a one-line aggregate); PL/Perl realpath
helper (deferred).

### Decision 8: New formatters (ceil / reserved-digit / dynamic unit suffix)
**Decision:** Add pure formatters: integer ceil rounding, reserved-digit fixed width (static layout,
only digits change), and a dynamic rate-unit suffix (MB/s→GB/s, Mbps→Gbps on digit overflow).
**Rationale:** `pretty.Size` switches the byte unit but has no rate suffix, no fixed width, and rounds to
one decimal; `internal/math` has no ceil. All three are net-new. Pure functions → property/table tests
at overflow boundaries.
**Alternatives considered:** Reuse `pretty.Size` as-is — rejected (insufficient).

### Decision 9: Per-source tiering + latency guard on the Collector; single knob
**Decision:** Throttle only the dear aggregates without a live panel twin (db sizes, growth) via
per-source cadence/latency state on `Collector`; system rows collect every tick (consistency). Throttled
source keeps its last (stale) value, not `n/a`. All behind the single `z` interval.
**Rationale:** System rows must stay consistent with the full panels a DBA cross-checks, so they are never
throttled; only the no-twin aggregates are. No second user knob. Extensible source registry for v.next.
**Alternatives considered:** A generic multi-rhythm scheduler now — rejected (YAGNI; the seam suffices).

### Decision 10: bgwr/ckpt reuses the version-split query; counters absolute
**Decision:** Reuse `SelectStatBgwriterQuery(version)` (PG14-16 / 17 / 18 split); show `ckpt_timed`/`req`
absolute, `write,ms`/`sync,ms` as delta, plus `maxwritten`; drop buffers.
**Rationale:** ADR [004] "absolute event counters"; buffers would duplicate the bgwriter screen.
**Alternatives considered:** Full buffer breakdown — rejected (dup of bgwriter screen, width).

## Data Models

- `view.View`: `+ Verbose bool` (zero value false → no behavior change).
- `top.config`: `+ verbose bool` (non-reset on screen switch — unlike `scrollOffset`).
- `stat.PostgresProperties`: `+ GucMaxWorkerProcesses`, `+ GucMaxLogicalReplicationWorkers`,
  `+ GucMaxParallelWorkers int`, `+ GucWalSegmentSize int64`, `+ DataDirectory string`.
- `stat.Collector`: `+` per-source cadence/guard state (e.g. `lastSizesRun time.Time`, skip divisors) and
  cached last values for throttled aggregates.
- New aggregate struct(s) for the pgstat verbose rows (workload/databases/workers/replication/bgwr-ckpt) —
  extend `Activity` or a new `Pgstat` sub-struct on `stat.Stat`; Go-side rates vs a `prev` snapshot.
- Verbose system aggregates: computed from existing `Diskstats`/`Netdevs`/`Fsstats` — **no new struct**.

## Dependencies

### New packages
- None — stdlib only (`math.Ceil`, `path/filepath.EvalSymlinks`).

### Using existing (from project)
- `internal/pretty.Size` — for `filesyst` size/used (consistent with the full fs panel).
- `internal/query.selectWalFunctions` — recovery-aware WAL fn names for lag/slots/wal-size.
- `internal/query.SelectStatBgwriterQuery` — version-split bgwr/ckpt query.
- `count*Usage` structs + `%util` math (`diskstats.go`, `netdev.go`, `fsstat.go`) — reused verbatim.
- `showExtra`/`ShowExtra` mirror-into-views pattern; `visibleColumns` pure-function precedent ([009]).

## Testing Strategy

**Feature size:** L

### Unit tests
- `topBandLayout` — table test: compact vs verbose coords, asymmetric panel heights, height-guard fallback.
- Formatters — ceil rounding; reserved-digit fixed width; dynamic unit-suffix switch at overflow boundaries
  (property/table).
- Verbose row composers vs `bytes.Buffer` — max-`%util` device selection (disk/net), `nicstat` `/1024/128`
  conversion parity, `filesyst` data_directory mount-prefix matching, `n/a` sentinels (first-tick / unavailable).
- pgstat aggregate math — per-interval cache hit (`Δhit/Δ(hit+read)`), tps=commit+rollback, `others` interval value.
- Tiering/guard — mock slow source → throttled, keeps stale value, auto-resumes; first-tick → `n/a`.

### Integration tests
- PG 14–18: new aggregate queries execute and scan; `SelectCommonProperties` + scan lockstep with new GUCs;
  version splits (`pg_stat_bgwriter`/`pg_stat_checkpointer`); degradation paths (no replication,
  `archive_mode=off`, remote without PL/Perl schema, insufficient privileges → `n/a`).
- All-three collection branch populates `Diskstats`/`Netdevs`/`Fsstats` when `view.Verbose=true` (local).

### E2E tests
- None automated (no TUI E2E harness in the project). Manual TUI QA in the Final Wave.

## Agent Verification Plan

**Source:** user-spec "Как проверить".

### Verification approach
Automated: `make build` / `make test` (race+coverage) / `make lint` / `govulncheck`; `go test` for the pure
functions (geometry, formatters, composers). Manual: TUI behavior (verbose toggle, panel growth, consistency
with full panels, height-guard, first-tick hint) — by the user, since no TUI E2E harness exists.

### Per-task verification
| Task | verify: | What to check |
|------|---------|--------------|
| 1 | bash | `go test ./internal/pretty/...` — formatter ceil/width/suffix |
| 2 | bash | `go test ./top/...` — toggleVerbose flips + persists flag |
| 3 | bash | `go test ./top/...` — compact output byte-identical (writer tests) |
| 4 | bash | `go test ./internal/query/... ./internal/stat/...` — GUC scan lockstep |
| 5 | bash | `go test ./internal/...` — new aggregate queries (live PG, t.Skipf) |
| 6 | user | verbose band geometry + height-guard on several terminal sizes |
| 7 | bash | `go test ./internal/stat/...` — all-three populate under verbose |
| 8 | user | pgstat verbose rows render correct values |
| 9 | user | sysstat verbose rows consistent with `B`/`N`/`F` panels |
| 10 | user | slow source stays stale; first-tick `collecting...` hint |
| 11 | bash | `make build && make test && make lint` |

### Tools required
bash (go test, make). No MCP/Playwright/curl — terminal TUI app verified manually.

## Backward Compatibility

**Breaking changes:** no.

**Migration strategy:** N/A — additive. `view.View.Verbose` and `config.verbose` default false (compact =
current behavior). `printSysstat`/`printPgstat` are refactored to thin `io.Writer` wrappers with byte-identical
compact output. `SelectCommonProperties` gains columns — internal only; the `GetPostgresProperties` `.Scan(...)`
is updated in the same task (lockstep) to avoid a scan-arity failure.

**DB migration compatibility:** N/A — no DB schema owned by pgcenter; only reads PG catalogs/`/proc`.

**Consumer impact:** none — pgcenter is a CLI binary, no library consumers. The new aggregate queries run
against the monitored instance only when verbose is on; `pg_ls_dir('pg_wal/archive_status')` needs
`pg_monitor`/superuser → `n/a` otherwise.

## Risks

| Risk | Mitigation |
|------|-----------|
| `layout()` geometry rework touches the shared UI loop (all screens) | Pure `topBandLayout` + table tests; compact path unchanged; manual QA across terminal sizes |
| R1: all-three system collection disturbing the mutually-exclusive side panels | Separate verbose branch after the switch, `==nil` guards, reuse same readers/structs; regression-test side panels |
| `SelectCommonProperties` scan-arity break | Update the `.Scan(...)` in `GetPostgresProperties` in the same task as the SELECT (lockstep); covered by `postgres_test.go` |
| `%util`/nicstat consistency (rMbps computed at print time) | Reuse exact panel math; replicate `/1024/128`; unit-test parity vs panel output |
| First verbose tick has no prev (reads 0) | Detect empty `Diskstats`/`Netdevs` (count*Usage returns nil on len mismatch) → emit `n/a`, not `0` |
| Dynamic unit-suffix new pattern | Pure formatter + property/table tests at overflow boundaries |
| Vertical space on short terminals | Height-guard: don't expand if band+cmdline+header+≥1 row doesn't fit; cmdline hint |
| New live-PG tests panic without a cluster (tech-debt [005]/[008]) | New tests use the `t.Skipf` guard pattern, not panic |

## Acceptance Criteria

Технические критерии (дополняют пользовательские из user-spec):

- [ ] `v` toggles verbose; flag persists across screen switches (not reset in `viewSwitchHandler`).
- [ ] Compact mode output is byte-identical to current behavior (refactor is behavior-preserving).
- [ ] Verbose system rows match the full `B`/`N`/`F` panels for the same device (consistency).
- [ ] Unavailable metrics render literal `n/a`; one failing source does not blank the other rows.
- [ ] Height-guard prevents a broken layout on short terminals (falls back to compact + hint).
- [ ] No view-count / keybinding / existing-panel test regressions; all unit + integration tests pass.
- [ ] `make lint` (golangci-lint + gosec) and `govulncheck` clean.
- [ ] New aggregate queries are version-correct on PG 14–18 and degrade to `n/a` on missing features.

## Implementation Tasks

### Wave 1 (независимые)

#### Task 1: Net-new formatting helpers
- **Description:** Add pure formatters for the verbose rows: integer ceil rounding, reserved-digit fixed-width
  columns (static layout, only digits change), and a dynamic rate-unit suffix that switches MB/s→GB/s and
  Mbps→Gbps on digit overflow. Needed because `pretty.Size` has no rate suffix / fixed width / ceil and
  `internal/math` has no ceil.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./internal/pretty/...`
- **Files to modify:** `internal/pretty/pretty.go`
- **Files to read:** `internal/pretty/pretty.go`, `internal/math/math.go`, `top/stat.go`

#### Task 2: Verbose toggle plumbing
- **Description:** Add the `verbose` mode flag on `view.View` (rides `viewCh`) and `top.config` (read by
  renderer/layout), the `v` keybinding and a `toggleVerbose` handler mirroring `showExtra` (write the flag
  into every view in the map for persistence, then push `viewCh`), and a help-screen entry. The flag must NOT
  be reset on screen switch.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./top/...` (toggleVerbose flips + persists)
- **Files to modify:** `internal/view/view.go`, `top/config.go`, `top/keybindings.go`, `top/verbose.go`, `top/help.go`
- **Files to read:** `top/extra.go`, `top/config_view.go`, `internal/view/view.go`, `top/keybindings.go`

#### Task 3: io.Writer refactor of printSysstat/printPgstat
- **Description:** Split `printSysstat`/`printPgstat` into thin `*gocui.View` wrappers calling
  `renderSysstat`/`renderPgstat` that take `io.Writer` (the `renderDbstat` precedent), so the panel rows
  become unit-testable against a `bytes.Buffer`. Behavior-preserving: compact output stays byte-identical.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./top/...` (compact output unchanged)
- **Files to modify:** `top/stat.go`, `top/stat_test.go`
- **Files to read:** `top/stat.go`, `top/stat_test.go`

#### Task 4: GUC + data_directory reads
- **Description:** Extend `SelectCommonProperties` with `max_worker_processes`,
  `max_logical_replication_workers`, `max_parallel_workers`, `wal_segment_size`, `data_directory`; add the
  matching fields to `PostgresProperties` and update the `GetPostgresProperties` `.Scan(...)` in lockstep
  (else scan arity fails). These feed the workers / archiving / filesyst verbose rows.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/query/... ./internal/stat/...`
- **Files to modify:** `internal/query/common.go`, `internal/stat/postgres.go`
- **Files to read:** `internal/query/common.go`, `internal/stat/postgres.go`, `internal/query/common_test.go`, `internal/stat/postgres_test.go`

#### Task 5: New aggregate SQL queries + collection
- **Description:** Add the version-aware aggregate queries and structs for the pgstat verbose rows —
  workload (`sum` over `pg_stat_database`), databases (`sum(pg_database_size)`+count+growth+per-interval cache
  hit), workers (active `backend_type` counts), replication (wal size, lag, slots/retain, `.ready` archiving
  backlog, send/recv), and bgwr/ckpt (reuse `SelectStatBgwriterQuery`) — with Go-side rate computation vs a
  prev snapshot. Wire the collect calls into `Collector.Update` gated on the verbose flag.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/...` (live PG, t.Skipf)
- **Files to modify:** `internal/query/common.go`, `internal/query/overview.go`, `internal/stat/postgres.go`, `internal/stat/stat.go`
- **Files to read:** `internal/query/databases.go`, `internal/query/replication.go`, `internal/query/replication_slots.go`, `internal/query/wal.go`, `internal/query/bgwriter.go`, `internal/query/query.go`, `internal/stat/postgres.go`

### Wave 2 (зависит от Wave 1)

#### Task 6: Verbose-aware layout() geometry
- **Description:** Extract a pure `topBandLayout(verbose, maxY)` returning the band/cmdline/dbstat
  y-coordinates and an `expanded` flag, and rewire `layout()` to use it: panels grow (`sysstat` +3, `pgstat`
  +5), `cmdline` and `dbstat` shift down, with a height-guard that falls back to compact + a cmdline hint when
  the terminal is too short. This is the invasive core touching the shared UI loop.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** user — verbose band geometry + height-guard on several terminal sizes
- **Files to modify:** `top/ui.go`, `top/layout.go`, `top/layout_test.go`
- **Files to read:** `top/ui.go`, `top/stat.go`

#### Task 7: All-three system collection branch (R1)
- **Description:** Add a verbose-gated branch in `Collector.Update`, after the existing mutually-exclusive
  `collectExtra` switch, that collects disk+net+fs each tick with `==nil` guards (no double-collect when a
  side panel already populated one) and records per-source availability instead of aborting the sample. The
  existing switch stays untouched so the side panels are unaffected.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./internal/stat/...` (all-three populate under verbose)
- **Files to modify:** `internal/stat/stat.go`
- **Files to read:** `internal/stat/stat.go`, `internal/stat/diskstats.go`, `internal/stat/netdev.go`, `internal/stat/fsstat.go`

#### Task 8: pgstat verbose row composers
- **Description:** Render the 5 right-panel verbose rows (workload, databases, workers, replication,
  bgwr/ckpt) from the Wave-1 aggregates + GUCs, using the new formatters, gated on the verbose flag inside
  `renderPgstat`. Includes the `n/a` sentinels for unavailable sources (no replication, `archive_mode=off`,
  missing privileges).
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** user — pgstat verbose rows render correct values
- **Files to modify:** `top/stat.go`, `top/stat_test.go`
- **Files to read:** `top/stat.go`, `internal/stat/postgres.go`, `internal/pretty/pretty.go`

### Wave 3 (зависит от Wave 2)

#### Task 9: sysstat verbose row composers + data_directory FS matching
- **Description:** Render the 3 left-panel verbose rows (iostat, nicstat, filesyst) by selecting the
  max-`%util` device from the existing `Diskstats`/`Netdevs` structs (reusing the exact panel math, including
  nicstat's print-time `/1024/128` conversion) and the `data_directory` filesystem via longest mount-prefix
  matching (`filepath.EvalSymlinks` local-only). Emits `n/a` on first tick (no prev) and when a source is
  unavailable.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** user — sysstat verbose rows consistent with `B`/`N`/`F` panels
- **Files to modify:** `top/stat.go`, `top/stat_test.go`, `internal/stat/fsstat.go`
- **Files to read:** `top/stat.go`, `internal/stat/diskstats.go`, `internal/stat/netdev.go`, `internal/stat/fsstat.go`

#### Task 10: Tiering + latency guard + first-tick handling
- **Description:** Add per-source cadence/latency-guard state on `Collector` so the dear no-twin aggregates
  (db sizes, growth) are throttled (keeping a cached stale value, not `n/a`) while system rows collect every
  tick; all behind the single `z` knob. Wire the first-tick `collecting...` cmdline hint that clears after the
  first successful refresh.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** user — slow source stays stale; first-tick `collecting...` hint clears
- **Files to modify:** `internal/stat/stat.go`, `top/stat.go`
- **Files to read:** `internal/stat/stat.go`, `top/stat.go`, `top/config_view.go`

### Final Wave

#### Task 11: Pre-deploy QA
- **Description:** Acceptance testing: run all tests, verify acceptance criteria from user-spec and tech-spec
  (verbose toggle, panel growth, consistency with full panels, `n/a` paths, height-guard, first-tick hint,
  remote/standby/archive_mode=off degradation) on a fresh `make build`.
- **Skill:** pre-deploy-qa
- **Reviewers:** none
