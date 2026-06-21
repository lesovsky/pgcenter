# Release 0.11.0 — New PostgreSQL Statistics

**Theme:** Add support for PostgreSQL statistics views that appeared (or changed) in recent
releases. After v0.10.x closed the infrastructure/maintenance gap, 0.11.0 is the first
feature-focused release.

**Audience lens:** practicing PostgreSQL DBA doing daily ops — prioritize what gets looked at
during incidents and routine monitoring, not just what is newest.

Each feature goes through the full SDD pipeline (user-spec → tech-spec → decompose → implement
→ review) **one at a time**, in the order below. Features are numbered continuing from the
archive (last archived: `003`).

## Scope & order

Order is low-risk → flagship: build the "new global-counter view" muscle before tackling the
pg_stat_io display-design problem.

### [004] pg_stat_bgwriter + pg_stat_checkpointer — combined screen

- **Status:** done — merged 2026-06-21 (TUI-only, `NotRecordable`; record/report deferred to backlog). Next up: [005] pg_stat_replication_slots.
- **Value:** no current way to watch background writer / checkpoint activity — checkpoint
  frequency, timed vs requested (signals undersized `max_wal_size`), write/sync time, buffers
  flushed.
- **Key complexity — PG 17 split:**
  - PG < 17: everything in `pg_stat_bgwriter`.
  - PG ≥ 17: `pg_stat_bgwriter` reduced (buffers_clean, maxwritten_clean, buffers_alloc);
    checkpoint metrics moved to new `pg_stat_checkpointer`; `buffers_backend` /
    `buffers_backend_fsync` removed entirely (now only via pg_stat_io).
- **Product decisions (locked):**
  - **Layout: variant A** — one stable set of logical columns across all versions; map
    differing source names (checkpoints_timed ↔ num_timed, checkpoint_write_time ↔ write_time)
    onto the same headers. DBA sees the same table on PG 15 and PG 18.
  - **buffers_backend / buffers_backend_fsync:** show on PG < 17, **drop on PG 17+** (data moved
    to pg_stat_io — out of scope here). Documented limitation.
  - **Restartpoints:** include `restartpoints_timed/req/done` (PG 17+) — valuable for standby
    monitoring.
  - **stats_age column:** include, reusing the `pg_stat_wal` pattern (`stats_age` text column,
    NOT diffed — excluded from DiffIntvl, see `internal/query/wal.go`).
- **Scope: TUI (top) only.** Mark view **`NotRecordable: true` (temporary)** — otherwise
  `record` would collect it and `report` would choke (issue #122 pattern). record/report is a
  separate future feature (see below).
- **Plumbing:** new view `bgwriter`, hotkey `b` (free), `SelectStatBgwriterQuery(version)` in
  new `internal/query/bgwriter.go` with a PG 17 branch. Single-row cumulative counters,
  diffable. Cross-join bgwriter × checkpointer on PG 17+; bgwriter-only on PG < 17. Respect
  `top/reset.go` (shared stats not reset).
- **Open questions for spec:** exact PG 18 checkpointer columns (verify via docs/code research,
  do not assume).
- **Side effect:** fix `overview.md` which currently wrongly claims pg_stat_bgwriter is supported.

### [005] pg_stat_replication_slots

- **Status:** done — implemented 2026-06-21 (PR #140, branch `feature/replication-slots`).
  Shipped as a **hybrid** `pg_replication_slots ⟕ pg_stat_replication_slots` multi-row screen
  (hotkey `o`, all slots + retained WAL), TUI-only `NotRecordable`, single query PG 14–18.
  `pg_stat_subscription_stats` deferred to a separate feature (as anticipated below). Next up:
  [006] pg_stat_io.
- **Value:** high daily-ops value — slot retention / spill is a frequent disk-fill incident,
  especially with logical decoding. `spilled_bytes`, `streamed_bytes`, `total_bytes`, retained
  WAL. Currently not shown at all.
- **Shape:** multi-row view (one row per slot), cumulative counters, diffable. Same pattern as
  bgwriter but multi-row — a bridge toward pg_stat_io.
- **Adjacent (decide in spec):** `pg_stat_subscription_stats` (PG 15, subscriber-side errors) —
  include now or defer.

### [006] pg_stat_io — flagship

- **Value:** high — unified IO breakdown by backend_type × object × context (PG 16+; PG 18 added
  WAL IO timings that were removed from pg_stat_wal).
- **Main risk is UX, not plumbing:** tall narrow table (~30 rows/sample). Cumulative counters fit
  pgcenter's rate model, but the display/filtering decision (show all rows vs filter by
  backend_type/object) needs a dedicated design discussion before/within the spec.

### [007] pg_stat_statements — JIT screen

- **Value:** medium. JIT compilation cost visibility (functions, inlining, optimization,
  emission counts/times; PG 15+, extended in PG 17).
- **Shape:** new 7th pg_stat_statements sub-screen (existing: timings, general, io, temp, local,
  wal) under the `X` menu. Lowest risk — closes the release.

## Cross-cutting principle: TUI-first

To keep each feature from ballooning into XL, every new view in 0.11.0 ships **top (TUI) only**
first. record/report support (recorder collection + report pipeline) is deliberately deferred —
it is a separate layer (storage format, `report.go` wiring, "no data" handling) and would roughly
double each feature. New views are marked `NotRecordable: true` until a dedicated record/report
feature lifts them.

## Out of scope / backlog (post-0.11.0 candidates)

- **record/report for the new 0.11.0 views** (bgwriter, replication_slots, pg_stat_io, JIT) —
  one or more follow-up features.
- **pg_stat_subscription_stats** — unless folded into [005].

## Finalization

- [ ] Update `overview.md` (correct bgwriter claim, list new views)
- [ ] Update `features-catalog.md` per feature
- [ ] Version bump, CHANGELOG
- [ ] Release per deployment.md (tag on master → push to `release`)
