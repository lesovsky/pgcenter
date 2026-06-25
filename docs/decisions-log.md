# Architecture Decision Log

Cross-feature log of key architectural decisions. Updated after each feature is finalized.
Used by tech-spec planning and code research to avoid repeating mistakes and re-litigating settled choices.

---

## [001-feat-per-process-system-stats] CollectExtra int on view.View for main-area enrichment

**Date:** 2026-05-19
**Feature:** 001-feat-per-process-system-stats
**Status:** Accepted

**Context:** The procpidstat view needed to signal `Collector.Update()` to run procfs enrichment after the SQL query. The existing `ShowExtra` mechanism triggers side-panel collection but also creates a side panel in the TUI — wrong for a main-area view.

**Decision:** Add `CollectExtra int` field to `view.View`. Set it to a typed constant (`CollectProcPidStat = 6`) in the switch handler and read it directly in `Collector.Update()`. Maintain a separate `prevCollectExtra` variable in `collectStat()` for change-detection and `Reset()` calls.

**Rationale:** `CollectExtra` reuses the `view.View` channel as a transport (same as `ShowExtra`) without triggering the side-panel creation. Avoids string-coupling (`view.Name == "procpidstat"`) between `internal/stat` and view names, which would violate package separation.

---

## [001-feat-per-process-system-stats] NotRecordable bool on view.View

**Date:** 2026-05-19
**Feature:** 001-feat-per-process-system-stats
**Status:** Superseded by [003-feat-procpidstat-record-report]

**Context:** The procpidstat view uses a hybrid data source (SQL + procfs). The recorder only collects SQL results and cannot capture the enriched 17-column result. Adding procpidstat to recording would produce misleading 7-column output.

**Decision:** Add `NotRecordable bool` field to `view.View` with zero-value `false` (all existing views remain recordable). Set `NotRecordable: true` on procpidstat. `record/record.go:filterViews()` skips views where `NotRecordable` is set.

**Rationale:** Go zero value makes this backwards-compatible — no existing view definitions need to change. Cleaner than checking by view name in the recorder.

**Superseded by:** [003-feat-procpidstat-record-report] — procpidstat recording is now supported via a stateful recorder. `NotRecordable: true` removed from procpidstat view; local/remote gate moved to `record.app.setup()` via `db.Local`.

---

## [001-feat-per-process-system-stats] IO probe uses real PG backend PID, not /proc/self/io

**Date:** 2026-05-19
**Feature:** 001-feat-per-process-system-stats
**Status:** Accepted

**Context:** Needed to detect at screen-open time whether `/proc/[pid]/io` is accessible for postgres backend processes. `/proc/self/io` is always readable by the process owner and gives a false-positive.

**Decision:** `stat.CheckIOAvailable(pid int)` receives a real backend PID queried from `pg_stat_activity` (first row, `pid != pg_backend_pid()`). Falls back to PID 1 if no backends are active.

**Rationale:** On Linux with `ptrace_scope=1`, cross-process `/proc/[pid]/io` access requires matching effective UID or `CAP_SYS_PTRACE`. Only probing a PID owned by a different OS user reveals whether the constraint applies.

---

## [001-feat-per-process-system-stats] CPU normalization 0–100% via runtime.NumCPU()

**Date:** 2026-05-19
**Feature:** 001-feat-per-process-system-stats
**Status:** Accepted

**Context:** Per-process CPU % can exceed 100% on multi-core systems (e.g., `htop`-style). DBA audience expects 0–100% range similar to `top` output.

**Decision:** Divide CPU rate by `runtime.NumCPU()` before display. Formula: `%cpu = (Δjiffies) / (refresh_seconds × ticks × cpuCount) × 100`.

**Rationale:** 0–100% is more intuitive for troubleshooting than N×100%. `runtime.NumCPU()` returns logical CPUs available to the process, which is the correct denominator for wall-clock CPU%.

---

## [001-feat-per-process-system-stats] iodelay (per-process iowait) deferred — requires Netlink taskstats

**Date:** 2026-05-19
**Feature:** 001-feat-per-process-system-stats
**Status:** Superseded by [002-feat-iodelay-procpidstat]

**Context:** Per-process iowait was planned but `/proc/[pid]/delays` does not exist in Linux. Delay accounting is only available via Netlink taskstats (`AF_NETLINK/NETLINK_GENERIC`), which is not in the codebase.

**Decision:** Defer iodelay to a separate future issue. v1 ships with CPU + IO bytes only.

**Rationale:** Implementing Netlink taskstats from scratch would significantly increase scope. The most valuable troubleshooting metrics (CPU%, IO throughput) are available without it.

**Superseded by:** [002-feat-iodelay-procpidstat] data source ADR — `delayacct_blkio_ticks` turned out to be available in `/proc/[pid]/stat` field 42, so Netlink was not required after all.

---

## [002-feat-iodelay-procpidstat] `/proc/[pid]/stat` field 42 instead of Netlink taskstats

**Date:** 2026-05-19
**Feature:** 002-feat-iodelay-procpidstat
**Status:** Accepted
**Supersedes:** [001-feat-per-process-system-stats] "iodelay (per-process iowait) deferred — requires Netlink taskstats"

**Context:** The prior ADR for [001-feat-per-process-system-stats] deferred iodelay on the assumption that delay accounting required the Netlink taskstats API. Re-investigation showed `delayacct_blkio_ticks` is exposed as field 42 of `/proc/[pid]/stat` (kernel-numbered from 1; `suffix[39]` after dropping `pid` and `comm`) — the same file already parsed each tick for CPU times.

**Decision:** Read `delayacct_blkio_ticks` from `suffix[39]` in `/proc/[pid]/stat`. Add `IODelay float64` to the existing `ProcPidStat` struct; no new collector maps or transport.

**Rationale:** No new dependencies (no Netlink socket, no `golang.org/x/sys/unix` package), minimal implementation delta (one extra field parsed from a file already opened each tick), and sufficient precision (clock ticks) for DBA troubleshooting.

**Alternatives considered:**
- Netlink taskstats (`AF_NETLINK/NETLINK_GENERIC`): nanosecond precision, but requires Generic Netlink socket, a new dependency, and significantly larger implementation scope. Rejected.

---

## [002-feat-iodelay-procpidstat] Availability probe via `/proc/sys/kernel/task_delayacct` sysctl

**Date:** 2026-05-19
**Feature:** 002-feat-iodelay-procpidstat
**Status:** Accepted

**Context:** The screen handler needs to decide at open time whether the iodelay columns can be populated. `delayacct_blkio_ticks` is always present in `/proc/[pid]/stat` but reads as `0` when delay accounting is disabled, so the field value itself is not a reliable signal.

**Decision:** `stat.CheckDelayAcctAvailable()` reads `/proc/sys/kernel/task_delayacct` and returns `true` iff the content is `"1"`. No PID argument needed. Called once in `switchViewToProcPidStat`; result is stored on the view.

**Rationale:** This sysctl is the authoritative runtime state of delay accounting — readable without root (`-rw-r--r-- 1 root root`). A single probe covers all cases: kernel built without `CONFIG_TASK_DELAY_ACCT` (file absent → read error → `false`), sysctl disabled (`"0"` → `false`), sysctl enabled (`"1"` → `true`).

**Alternatives considered:**
- Parse `/boot/config-$(uname -r)` for `CONFIG_TASK_DELAY_ACCT=y`: brittle, requires shell invocation, not authoritative for runtime state. Rejected.
- Check field 42 value after two ticks (non-zero = available): unreliable — zero is a valid accumulated value for a new or non-IO process. Rejected.

---

## [002-feat-iodelay-procpidstat] `%iodelay` not normalized by `cpuCount`

**Date:** 2026-05-19
**Feature:** 002-feat-iodelay-procpidstat
**Status:** Accepted

**Context:** CPU rate columns (`%all`, `%us`, `%sy`) in the procpidstat screen are normalized by `runtime.NumCPU()` to keep them in the 0–100% range — see ADR [001-feat-per-process-system-stats] "CPU normalization 0–100% via runtime.NumCPU()". The question for `%iodelay` was whether to apply the same normalization.

**Decision:** Formula `ΔIODelay / (itv × ticks) × 100` with no division by `cpuCount`. `%iodelay` may legitimately exceed 100% on multi-threaded blocking patterns; this is documented behavior, not a bug.

**Rationale:** `delayacct_blkio_ticks` counts wall-clock ticks the process spent blocked in D-state, regardless of CPU count. A single-threaded process can be 100% IO-blocked whether the machine has 1 or 64 cores. Normalizing by `cpuCount` would produce misleadingly small numbers (e.g., 1.56% on a 64-core machine for a fully IO-blocked process). The CPU-rate columns normalize correctly because CPU time is shared across cores; IO-blocked time is not.

---

## [003-feat-procpidstat-record-report] Option B: store display strings, DiffIntvl=[0,0] for procpidstat recording

**Date:** 2026-05-19
**Feature:** 003-feat-procpidstat-record-report
**Status:** Accepted

**Context:** procpidstat enrichment produces a 19-column PGresult where cols 6–11 contain display strings (`HH:MM:SS`, KiB) and cols 12–17 contain pre-computed rate strings. Two approaches were considered for what to store in the tar archive.

**Decision:** Recorder stores the display-ready 19-column PGresult (Option B). Report reads with `DiffIntvl=[0,0]` — pass-through, no column subtraction. Rate columns are pre-computed at recording time.

**Rationale:** Established pgcenter pattern for snapshot views (`activity`, `progress_*`). No report pipeline changes needed beyond sysinfo reading. Cols 6–11 contain `HH:MM:SS` strings that `diffPair` cannot parse — Option A (raw jiffies + DiffIntvl=[6,11]) would require a second formatter in the report pipeline.

**Alternatives considered:**
- Option A (store raw jiffies, recompute in report via DiffIntvl=[6,11]): rejected — `HH:MM:SS` strings in cols 6–11 are not parseable by `diffPair`. Would require additional formatter in report.

---

## [003-feat-procpidstat-record-report] isLocal propagated through tarConfig, not checked per-tick

**Date:** 2026-05-19
**Feature:** 003-feat-procpidstat-record-report
**Status:** Accepted

**Context:** `tarRecorder.collect()` opens a fresh DB connection per tick and has no access to the `app.db` struct (which doesn't exist — `app` only stores `dbConfig`). Need to know at collect time whether recording is local.

**Decision:** `db.Local` is captured in `app.setup()` before `db.Close()` and passed into `tarConfig.isLocal`, stored as `tarRecorder.isLocal`. The procfs enrichment branch in `collect()` gates on `c.isLocal`.

**Rationale:** `isLocalhost()` is a string test on `Config.Host` — re-checking per tick is wasteful and architecturally wrong (not a live probe). Struct fields on `tarRecorder` persist across ticks since the same instance is reused by the `record()` loop.

---

## [003-feat-procpidstat-record-report] sysinfo as separate tar entry, not merged into meta

**Date:** 2026-05-19
**Feature:** 003-feat-procpidstat-record-report
**Status:** Accepted

**Context:** `ticks` (CLK_TCK) and `cpuCount` are system properties needed to document the recording environment. The existing `meta.*` entry holds PostgreSQL properties (version, recovery status, etc.).

**Decision:** Write `sysinfo.TIMESTAMP.json` as a separate tar entry per tick containing `stat.SysInfo{Ticks float64, CPUCount int}`. Report reads it alongside `meta.*` and merges into `metadata` struct.

**Rationale:** System properties and PostgreSQL metadata are different bounded contexts. Keeping them separate avoids widening `SelectCommonProperties` SQL with non-PG values. Under Option B, sysinfo is informational — rates are pre-computed and absent sysinfo has no effect on report output.

---

## [003-feat-procpidstat-record-report] Local/remote gate in app.setup(), not in filterViews

**Date:** 2026-05-19
**Feature:** 003-feat-procpidstat-record-report
**Status:** Accepted

**Context:** After removing `NotRecordable: true` from procpidstat view, needed a runtime mechanism to skip procpidstat when recording against a remote PostgreSQL (where procfs is not available).

**Decision:** `app.setup()` removes `procpidstat` from `views` and prints INFO when `!db.Local`, before `filterViews()` is called. `filterViews()` handles only static unsuitability (PG version, missing extension).

**Rationale:** Local/remote is a runtime property, not a static view property. Keeps `filterViews()` focused on one concern. `db.Local` is already computed by `postgres.Connect()` via `isLocalhost()`.

---

## [004-feat-bgwriter-checkpointer] Per-version column sets, not NULL-padded unified columns

**Date:** 2026-06-21
**Feature:** 004-feat-bgwriter-checkpointer
**Status:** Accepted

**Context:** `pg_stat_bgwriter` changes shape across versions: PG 17 splits checkpoint metrics into `pg_stat_checkpointer`, drops `buffers_backend`/`buffers_backend_fsync`, and PG 18 adds `slru_written`. The bgwriter screen had to decide whether to present one unified column set (NULL-padding versions that lack a column) or version-specific sets.

**Decision:** Each version branch returns only the columns that exist on that version; shared columns keep identical headers and order. `Ncols`/`DiffIntvl` differ per version (PG 14–16: 12 cols / `[3,10]`; PG 17: 13 / `[6,11]`; PG 18: 14 / `[6,12]`).

**Rationale:** Follows the `wal.go` precedent, which already returns different `Ncols`/`DiffIntvl` per version. NULL-padding pre-17 with empty restartpoint columns would show misleading blank columns to a PG 15 DBA.

**Alternatives considered:** Unified header set with `NULL AS rstpt_*` placeholders on PG 14–16 — rejected: clutters the screen with always-empty columns and contradicts the wal precedent.

---

## [004-feat-bgwriter-checkpointer] Absolute event counters via DiffIntvl placement

**Date:** 2026-06-21
**Feature:** 004-feat-bgwriter-checkpointer
**Status:** Accepted

**Context:** Checkpoint/restartpoint event counters (`ckpt_timed`, `ckpt_req`, and PG 17+ `rstpt_timed/req/done`) must render as absolute cumulative values — the timed-vs-requested ratio is the signal, and a per-interval delta on a short refresh is almost always 0. The work/time/buffer columns must render as deltas.

**Decision:** Place the event counters in a contiguous block right after the `source` label, **outside** the `DiffIntvl` range; the diffed work/time/buffer columns form the single contiguous diff range; `stats_age` is last, also outside the range.

**Rationale:** `DiffIntvl` is a single contiguous `[lo,hi]` range (`internal/stat/postgres.go:diff()`), which copies out-of-range columns as-is and subtracts in-range ones. Keeping event counters outside the range is the only way to render them absolute without changing the diff machinery.

**Alternatives considered:** Diff everything (wal-style) — rejected for event counters; they would flicker between 0 and 1.

---

## [004-feat-bgwriter-checkpointer] NotRecordable: true for TUI-only scope

**Date:** 2026-06-21
**Feature:** 004-feat-bgwriter-checkpointer
**Status:** Accepted

**Context:** Supporting `record`/`report` for the bgwriter screen pulls in the storage format and the report pipeline — a separate layer that would roughly double the feature. The 0.11.0 roadmap mandates TUI-first for every new view.

**Decision:** Register the view with `NotRecordable: true`; `record/record.go:filterViews()` skips it. Record/report support is deferred to a backlog feature (`docs/roadmap-0.11.0.md`, "Out of scope / backlog").

**Rationale:** Keeps the feature size-M. Reuses the `NotRecordable` field whose lineage is in ADR [001-feat-per-process-system-stats] / [003-feat-procpidstat-record-report]; after procpidstat dropped the flag in feature 003, bgwriter is its sole live user.

**Alternatives considered:** Ship record/report in the same feature — rejected as scope creep.

---

## [004-feat-bgwriter-checkpointer] stats_age sourced from pg_stat_checkpointer on PG 17+

**Date:** 2026-06-21
**Feature:** 004-feat-bgwriter-checkpointer
**Status:** Accepted

**Context:** On PG 17+ there are two independent `stats_reset` timestamps — `pg_stat_bgwriter.stats_reset` and `pg_stat_checkpointer.stats_reset` — reset separately via `pg_stat_reset_shared('bgwriter'|'checkpointer')`. The single-column `stats_age` must derive from one of them.

**Decision:** On PG 17+ `stats_age` derives from `pg_stat_checkpointer.stats_reset`.

**Rationale:** The screen's primary content on modern versions is checkpoint data; one column is cleaner than two reset ages. Documented in the user-spec so an independently-reset bgwriter is not a surprise.

**Alternatives considered:** Show both reset ages, or the older of the two — rejected as needless column noise for a secondary signal.

---

## [004-feat-bgwriter-checkpointer] Go toolchain bump 1.25.10 → 1.25.11 in CI

**Date:** 2026-06-21
**Feature:** 004-feat-bgwriter-checkpointer
**Status:** Accepted

**Context:** `govulncheck` in CI flagged GO-2026-5037, a `crypto/x509` stdlib vulnerability fixed in Go 1.25.11. Surfaced during feature 004 execution; unrelated to the feature code.

**Decision:** Bump the Go toolchain from 1.25.10 to 1.25.11 in the CI workflows to close GO-2026-5037.

**Rationale:** Stdlib vuln in a transitive code path; the cheapest fix is the patch-version toolchain bump the CI gate requires.

---

## [005-feat-replication-slots] Hybrid pg_replication_slots ⟕ pg_stat_replication_slots, not the literal view

**Date:** 2026-06-21
**Feature:** 005-feat-replication-slots
**Status:** Accepted

**Context:** The roadmap line item named `pg_stat_replication_slots` but framed the value as retained-WAL / disk-fill triage. Retained WAL (`restart_lsn`, `wal_status`, `safe_wal_size`) lives in `pg_replication_slots`, and physical slots are absent from `pg_stat_replication_slots` entirely — so the literally-named view cannot deliver the stated value.

**Decision:** Source the screen from `pg_replication_slots s LEFT JOIN pg_stat_replication_slots ss ON s.slot_name = ss.slot_name` — one row per slot (physical + logical). State columns absolute; the eight logical-decoding counters diffed.

**Rationale:** Only the hybrid covers all slots plus retained WAL, which is the disk-fill signal the feature exists for.

**Alternatives considered:** Pure `pg_stat_replication_slots` (logical-only spill/stream, no retained WAL, no physical slots) — rejected: does not solve the disk-fill use case.

---

## [005-feat-replication-slots] coalesce(...,0) on the diffed counters for LEFT-JOIN-NULL safety

**Date:** 2026-06-21
**Feature:** 005-feat-replication-slots
**Status:** Accepted

**Context:** Physical slots are absent from `pg_stat_replication_slots`, so the LEFT JOIN yields NULL for the 8 counter columns. A physical slot matches itself across samples by `slot_name`, entering the diff branch; an empty-string NULL inside `DiffIntvl` reaches `diffPair` → `strconv.ParseInt("")` → error → the whole sample is aborted (`internal/stat/postgres.go`).

**Decision:** Wrap the 8 diffed counters in `coalesce(..., 0)` in SQL. Physical slots render `0`. Absolute columns (retained/safe/stats_age, outside `DiffIntvl`) stay nullable and render blank.

**Rationale:** Mandatory for correctness, verified live (a physical slot would otherwise blank the screen). Rendering `-`/blank for physical rows would need per-cell view logic pgcenter lacks; the adjacent `slot_type` column disambiguates the `0`.

**Alternatives considered:** Per-cell custom rendering of physical-slot counters — rejected as scope creep.

---

## [005-feat-replication-slots] Single query for PG 14–18, no version branching

**Date:** 2026-06-21
**Feature:** 005-feat-replication-slots
**Status:** Accepted

**Context:** `pg_replication_slots` gained `conflicting` (PG 16), `failover`/`synced` (PG 17), `invalidation_reason` (PG 18). Including any of them would force per-version query strings (the [004] ADR situation).

**Decision:** Use only the subset stable across PG 14–18 (`slot_name, slot_type, active, wal_status, restart_lsn, safe_wal_size` + the whole of `pg_stat_replication_slots`). `SelectStatReplicationSlotsQuery(_ int)` returns one query, `15`, `[2]int{6,13}` for all versions (param kept for signature symmetry / future branch point).

**Rationale:** Invalidation **state** is carried by `wal_status` (`lost`/`unreserved`); the finer **cause** columns are version-fragmented and a niche signal. One query keeps the feature size-M and avoids upgrade-surprise column shifts.

**Alternatives considered:** Add `invalidation_reason` (PG 18 branch) / `conflicting` (PG 16 branch) — rejected; deferrable if cause-attribution is ever requested.

---

## [005-feat-replication-slots] Default sort by retained,KiB desc (OrderKey=4), deviating from col-0

**Date:** 2026-06-21
**Feature:** 005-feat-replication-slots
**Status:** Accepted

**Context:** Every other multi-row view defaults `OrderKey=0`. The replslots feature exists for disk-fill triage, where the most relevant slot is the one holding the most WAL.

**Decision:** `OrderKey=4` (`retained,KiB`), `OrderDesc=true`; SQL `ORDER BY "retained,KiB" DESC NULLS LAST` for the first frame. Documented so it is not read as a bug.

**Rationale:** Incident-first ordering puts the offender on top without the DBA re-sorting. The Go-side `sort()` governs each subsequent frame.

**Alternatives considered:** `OrderKey=0` (slot_name) for consistency — rejected: buries the offender.

---

## [005-feat-replication-slots] Test image wal_level=logical + bump 0.0.10, decoupled by defensive t.Skipf

**Date:** 2026-06-21
**Feature:** 005-feat-replication-slots
**Status:** Accepted

**Context:** Exercising a real logical slot (`pg_create_logical_replication_slot(..., 'test_decoding')`) needs `wal_level=logical`, which the `pgcenter-testing` image did not set. The image build/push is a manual maintainer step (no CI image-build job).

**Decision:** Add `wal_level=logical` to `testing/prepare-test-environment.sh` and bump the image `0.0.9 → 0.0.10` (Dockerfile + both workflows + deployment.md). The tier-3 logical-slot test `t.Skipf`s unless `wal_level=logical`, so CI stays green on the old image until the maintainer publishes `0.0.10`.

**Rationale:** The defensive skip removes a fragile ordering dependency between the manual image push and the code merge. `test_decoding` ships in the PGDG `postgresql-NN` packages, so no extra package is needed. Verified live: tier-1/2 pass on `replica`, tier-3 passes on `logical` across PG 14–18.

**Alternatives considered:** Hard ordering (push image, then merge code that assumes `wal_level=logical`) — rejected: any gap leaves CI red on transient infra state.

---

## [006-feat-pg-stat-io] Split one wide stats view into two registered sub-views

**Date:** 2026-06-21
**Feature:** 006-feat-pg-stat-io
**Status:** Accepted

**Context:** `pg_stat_io` has ~20 counters. pgcenter has no horizontal column scroll — columns past the terminal width are silently cut. The same constraint is why `pg_stat_statements` is split into sub-screens.

**Decision:** Register two views, `stat_io` (count: operations + KiB) and `stat_io_time` (timings), navigated by a lowercase toggle (`j` → `statioNextView`) plus an uppercase menu (`J` → `menuStatIO`), mirroring the `databases`/`pgss` lowercase-switch / uppercase-menu idiom.

**Rationale:** Keeps each screen readable within a normal terminal without inventing a horizontal-scroll mechanism in the shared align/print path. A DBA's questions are sequential anyway ("how much IO" then "how slow"), so not seeing `reads` and `read_time` on one row is acceptable.

**Alternatives considered:** One wide table (columns cut — unreadable); aggregation by `backend_type` (loses the `object`/`context` breakdown, the core value); a reduced single-screen column set (kills the evictions/reuses and WAL-fsync stories). All rejected.

**Update (2026-06-23, [009-feat-horizontal-scroll]):** Horizontal column scroll was later implemented for the main table, so the "no horizontal scroll" premise no longer holds. The two-screen split is **kept deliberately** — it is a product decision (logical grouping of count vs latency, sequential DBA workflow), not a workaround for the missing mechanism. Scroll exists for narrow terminals; collapsing the sub-screens into one wide view is explicitly out of scope. The same applies to the [007] `pg_stat_statements` JIT sub-screen split and the synthetic `io_key`.

---

## [006-feat-pg-stat-io] Synthetic md5 io_key for composite row identity; column hiding is not available

**Date:** 2026-06-21
**Feature:** 006-feat-pg-stat-io
**Status:** Accepted

**Context:** `view.UniqueKey` is a single column index and `stat.diff()` matches rows across samples on exactly one column. `pg_stat_io` identity is composite (`backend_type × object × context`). The user-spec assumed the key column could be hidden.

**Decision:** Emit `left(md5(backend_type||object||context),10) AS io_key` as column 0 and point `UniqueKey` at it (the `statements_io` `queryid` precedent). Display it like the pgss `queryid`; keep the three dimensions as separate sortable/filterable columns.

**Rationale:** Collapsing the composite identity into one column is mandatory for correct cross-sample deltas (matching on `backend_type` alone would be a silent correctness bug). Column hiding is impossible without new code: `internal/align.SetAlign` floors every column at width 8 and `ColsWidth` is a runtime width cache, not a preset — a hide mechanism would touch the shared align/print path for every screen (scope creep).

**Alternatives considered:** `ColsWidth[0]=0` to hide the key (does not work — see above); a single readable `backend_type/object/context` concat column instead of separate dims (loses per-dimension filtering — `/vacuum` would match both `context=vacuum` and `autovacuum worker`). Rejected.

---

## [006-feat-pg-stat-io] Per-version query branch PG 16/17 vs PG 18; time selector version-independent

**Date:** 2026-06-21
**Feature:** 006-feat-pg-stat-io
**Status:** Accepted

**Context:** PG 18 removed `op_bytes` (always BLCKSZ) in favor of native `read_bytes`/`write_bytes`/`extend_bytes`, and added `object='wal'` rows. PG 16 and 17 expose an identical `pg_stat_io` column set.

**Decision:** `SelectStatIOQuery(version)` branches at `PostgresV18`: pre-18 derives KiB as `count*op_bytes/1024`, 18+ uses native `*_bytes/1024`; the logical column shape, `Ncols`, and `DiffIntvl` are identical across branches. `SelectStatIOTimeQuery` is version-independent (timing columns are identical PG 16–18). Both screens use the same count-based `WHERE sum(counters)>0` and `coalesce(...,0)` on every diffed column.

**Rationale:** Follows the [004] per-version-branch ADR (not NULL-padded). Identical headers keep the DBA's table stable across versions. `coalesce` is mandatory — `pg_stat_io` returns NULL pervasively (e.g. `fsyncs` for `temp relation`), and a NULL inside `DiffIntvl` aborts the whole sample ([005] lesson).

**Alternatives considered:** Single NULL-padded query across versions (rejected by [004]); a separate PG 17 branch (unnecessary — 16==17).

## [007-feat-pg-stat-statements-jit] JIT query selector returns a 4-tuple (query, Ncols, DiffIntvl, UniqueKey)

**Date:** 2026-06-22
**Feature:** 007-feat-pg-stat-statements-jit
**Status:** Accepted

**Context:** The JIT sub-screen's column COUNT differs across versions (PG 15/16 → 13 columns; PG 17+ → 15, adding `deform_total`/`deform,ms`). Because `UniqueKey` points at the trailing synthetic md5 `queryid`, its index shifts with the column count, and `DiffIntvl` shifts too. The existing `statements_timings` selector returns only a query string (its count is constant at 13 across versions), and the `pg_stat_io` selector returns `(string, int, [2]int)` (its `UniqueKey` is a fixed col 0).

**Decision:** `SelectStatStatementsJITQuery(version) (string, int, [2]int, int)` returns query + `Ncols` + `DiffIntvl` + `UniqueKey`; `view.Configure()` patches all four. Two-way branch: `>= PostgresV17` → (Default, 15, {7,12}, 13); else → (PG15, 13, {6,10}, 11). Gate `MinRequiredVersion: query.PostgresV15`.

**Rationale:** A trailing-key layout whose index moves with version forces the selector to carry the full layout — returning only the query (timings model) would leave stale `Ncols`/`DiffIntvl`/`UniqueKey`; returning a 3-tuple (io model) would leave a stale `UniqueKey`. Extending to a 4-tuple keeps the layout invariant explicit and unit-testable, and column hiding remains unavailable (`internal/align` floors width at 8, ADR [006]).

**Alternatives considered:** Compute `UniqueKey = Ncols-2` in `Configure()` (works, but hides a layout invariant in arithmetic). Hide `deform` columns on PG 15 (impossible per ADR [006]). Both rejected.

---

## [007-feat-pg-stat-statements-jit] JIT column layout: timings-style total+interval, drop the *_count phase counters

**Date:** 2026-06-22
**Feature:** 007-feat-pg-stat-statements-jit
**Status:** Accepted

**Context:** `pg_stat_statements` exposes 8 JIT metrics on PG 15/16 (4 counts + 4 times) and 10 on PG 17+. Mirroring `statements_io`'s total+interval doubling across all of them would be ~22–26 columns; pgcenter has no horizontal column scroll (ADR [006]).

**Decision:** Model the screen on `statements_timings`: cumulative `*_total` text durations + per-interval `*,ms` columns for the four (PG 17+: five) phase TIMES, plus a single `functions` counter. Drop the `inlining_count`/`optimization_count`/`emission_count` (and `deform_count`) metrics. Filter rows to `WHERE jit_functions > 0` (pg_stat_io zero-row precedent), surfacing the `jit=off` empty state via the view `Msg`.

**Rationale:** The per-phase TIME is the cost signal a DBA acts on; the phase counts are secondary and `jit_functions` already represents "how much JIT work". Dropping the counts keeps the screen at 13/15 columns — within terminal width. The `Msg`-as-hint reuses the existing `track_io_timing` note mechanism (no new GUC-detection code).

**Alternatives considered:** All counts+times with total+interval doubling (too wide). Splitting into count/time sub-screens like `pg_stat_io` (out of scope — JIT is the low-risk release closer, and the time-only set fits one screen). Both rejected.

---

## [008-feat-record-report-0-11-views] Lift NotRecordable only — pure-SQL views need no recorder change

**Date:** 2026-06-22
**Feature:** 008-feat-record-report-0-11-views
**Status:** Accepted
**Supersedes:** [004-feat-bgwriter-checkpointer] "NotRecordable: true for TUI-only scope" (and the equivalent NotRecordable scoping in [005]/[006]/[007])

**Context:** The four 0.11.0 screens (5 report types: bgwriter, replslots, stat_io, stat_io_time, statements_jit) shipped `NotRecordable: true` as a deliberate TUI-first scope cut. Enabling record/report could have been read as a large effort (the procpidstat [003] precedent needed a stateful recorder, sysinfo entries, local/remote gating).

**Decision:** These are pure-SQL views, so collection needed only the removal of `NotRecordable: true` from the 5 view definitions — `record/filterViews()` then keeps them and the existing SQL collect/write path records them like wal/tables. The recorder, the tar storage format, and the report replay engine are unchanged. report-time `view.Configure(query.Options{Version})` already selects the version-correct layout; the rebuilt SQL string is never executed in report, so passing only `Version` is sufficient.

**Rationale:** No procfs/derived data is involved, so the procpidstat stateful machinery does not apply. The procpidstat enrichment branch is keyed on the literal `"procpidstat"` view name and never fires for these views. After this feature, no production view sets `NotRecordable: true`; the field and `filterViews` drop-branch remain, guarded solely by the synthetic `TestFilterViews_dropsExplicitNotRecordable`.

**Alternatives considered:** A procpidstat-style stateful recorder path — rejected as dead complexity for pure-SQL views.

---

## [008-feat-record-report-0-11-views] report CLI: reuse -X for JIT, one string flag for the two IO screens

**Date:** 2026-06-22
**Feature:** 008-feat-record-report-0-11-views
**Status:** Accepted

**Context:** Five new report types needed CLI selectors in `cmd/report`. The existing idioms are bool flags for single screens (`-W` wal) and string sub-selector flags for screen families (`-X` statements, `-D` databases, `-P` progress).

**Decision:** `-B`/`--bgwriter` (bool), `-L`/`--replslots` (bool), `-J`/`--io` (string `c|t` → stat_io / stat_io_time), and extend the existing `-X`/`--statements` with value `j` → statements_jit. Short letters B/L/J were verified free.

**Rationale:** JIT is a pg_stat_statements sub-screen, so it belongs under `-X` like the TUI `x`-cycle rather than getting its own flag. The two pg_stat_io screens map to one string sub-selector (`-J c|t`), mirroring `-X`/`-D`/`-P`, instead of burning two bool flags.

**Alternatives considered:** Two bool flags for the IO screens (breaks the string-subselector idiom); a dedicated JIT flag (it is a statements sub-screen). Both rejected.

---

## [008-feat-record-report-0-11-views] Replay tests: synthetic in-memory tar + golden files, not the legacy fixture

**Date:** 2026-06-22
**Feature:** 008-feat-record-report-0-11-views
**Status:** Accepted

**Context:** The new screens' replay/diff logic needed test coverage, including the version-aware layout switch. The existing report golden tests are fed by one shared fixture `testdata/pgcenter.stat.golden.tar`.

**Decision:** Verify replay with golden files fed by a purpose-built synthetic in-memory tar (the `Test_app_doReport_procpidstat` pattern), one new `_test.go` file + its own goldens per screen. Version-aware screens (bgwriter, stat_io, statements_jit) get golden variants at recorded version 14/17/18, 16/18, 15/17 respectively; version-independent screens (replslots, stat_io_time) get one. No live PostgreSQL needed; the recording's meta version drives the report-time `Configure` layout switch.

**Rationale:** The legacy `pgcenter.stat.golden.tar` predates these views, is a ~PG13 recording that cannot hold modern-view data, and regenerating it would churn all ~30 existing goldens. Synthetic input is version-parametric for free. Separate per-screen test files + goldens keep the tasks conflict-free for parallel authoring.

**Alternatives considered:** A live PG14-18 record→report end-to-end harness (none exists; building one is disproportionate — the live record↔report seam is covered by a manual check); regenerating the shared fixture (mass golden churn). Both rejected.

---

## [009-feat-horizontal-scroll] Scroll offset on top.config, not on view.View

**Date:** 2026-06-23
**Feature:** 009-feat-horizontal-scroll
**Status:** Accepted

**Context:** The horizontal scroll position needs a home. `view.View` values are stored in `config.views`, and `viewSwitchHandler` deliberately persists per-view state (`OrderKey`, `Filters`, `ColsWidth`) into that map so it is restored when the user returns to a screen. But the user-spec requires the opposite for scroll — it must reset to 0 on every screen switch.

**Decision:** Store the offset as a new `scrollOffset int` field on `top.config`, not on `view.View`. It is ephemeral "current screen" UI state; `config` is already threaded into both the render path and the key handlers.

**Rationale:** Putting the offset on `view.View` would make it inherit the per-view persistence and survive switches — the wrong behavior. Persistence vs ephemerality is the deciding axis: per-view state belongs on the view, transient view-independent UI state belongs on `config`.

**Alternatives considered:** Field on `view.View` with explicit zeroing on load — fights the intentional persistence mechanism and needs zeroing in every load path. Rejected as conceptually wrong (offset is not part of a view's definition).

---

## [009-feat-horizontal-scroll] Manual column window, not gocui viewport scroll

**Date:** 2026-06-23
**Feature:** 009-feat-horizontal-scroll
**Status:** Accepted

**Context:** Freezing the first column (the row identifier) while scrolling the rest is the core requirement. gocui offers a built-in horizontal viewport shift (`SetOrigin`/origin-x).

**Decision:** Compute the visible column window in a pure function (`visibleColumns`) and render the frozen column + window manually inside `printStatHeader`/`printStatData`. Do not use gocui's viewport scroll.

**Rationale:** gocui's viewport-x shift moves the *entire* buffered row, which cannot keep the first column fixed. The print functions already iterate columns, so manual windowing fits the existing structure, and extracting the window math into a pure function makes the only non-trivial logic unit-testable without a live terminal.

**Alternatives considered:** gocui viewport `origin.x` (cannot freeze the first column). Hiding columns via `ColsWidth[i]=0` (impossible — `internal/align.SetAlign` floors width at 8, per ADR [006-feat-pg-stat-io]). Both rejected.

---

## [009-feat-horizontal-scroll] Reset offset on both view-switch paths

**Date:** 2026-06-23
**Feature:** 009-feat-horizontal-scroll
**Status:** Accepted

**Context:** Scroll must reset to 0 on every screen switch. There are two switch paths: the normal `switchViewTo → viewSwitchHandler`, and `switchViewToProcPidStat` (`Shift+S`), which patches runtime fields onto the view manually and does **not** delegate to `viewSwitchHandler`.

**Decision:** Zero `config.scrollOffset` in `viewSwitchHandler` and, separately, in `switchViewToProcPidStat`. In the latter, place the reset **before** the `db.Local` guard.

**Rationale:** A single reset in `viewSwitchHandler` would miss the per-process screen, leaving a stale offset on entry. Placing the reset before the `db.Local` guard is required for testability — the code after the guard calls `app.db.QueryRow` (nil Conn → panic without a live PostgreSQL), and on the remote path the reset is idempotent and harmless.

**Alternatives considered:** Reset only in `viewSwitchHandler` (misses procpidstat); reset inside the render path on a detected view-name change (adds hidden state tracking to the render loop). Both rejected.

---

## [009-feat-horizontal-scroll] Partial last column + marker reservation in both walk directions

**Date:** 2026-06-23
**Feature:** 009-feat-horizontal-scroll
**Status:** Accepted (refinement surfaced by manual QA after the initial implementation)

**Context:** The first implementation admitted a scrollable column into the window only when it fit *whole*. Manual narrow-terminal testing found two bugs with one root cause: a deliberately wide trailing column (`query` on activity/statements) vanished entirely (it never fit whole, where gocui previously printed it truncated at the edge), and a spurious `›` marker appeared on a wide terminal.

**Decision:** Change `visibleColumns` window semantics to "start-in-budget" — the last column may render partially (truncated at the edge); `hiddenRight` is false when nothing follows it. The `‹`/`›` marker glyphs are visible runes, so reserve their cell width in **both** the forward walk (window from offset) and the backward walk (computing `maxOffset`) — otherwise the last column becomes unreachable at the right edge.

**Rationale:** Dropping a column whole-or-nothing is worse than the prior truncate-at-edge behavior. Reserving the marker only in the forward walk leaves a forward/backward asymmetry that makes `maxOffset` undershoot, so the last column can never be fully reached and `›` never turns off — a correctness bug the user observed visually. A property test over `ncols × widths × termWidth` now proves "last column reachable at `maxOffset`, `›` off there".

**Alternatives considered:** Keep whole-column-only semantics and accept the disappearing `query` (rejected — regresses a long-standing behavior); reserve marker width in the forward walk only (rejected — the asymmetry bug above). This refinement was invisible to the original unit tests, which encoded the whole-column semantics; only manual visual QA exposed it.

---

## [009-feat-horizontal-scroll] Hotkeys `[` / `]`, not Shift/Ctrl/Alt + arrow

**Date:** 2026-06-23
**Feature:** 009-feat-horizontal-scroll
**Status:** Accepted

**Context:** Scroll needs two new keys. Arrows are already taken (sort column / column width). The TUI stack is gocui + nsf/termbox-go (`OutputNormal`), whose key modifiers are only `ModNone`/`ModAlt`.

**Decision:** Use the printable keys `[` (left) and `]` (right), registered on the `sysstat` view.

**Rationale:** termbox does not distinguish `Shift`/`Ctrl`+arrow (so e.g. `Shift+S` arrives as the plain rune `'S'`, not a modifier combo), and `Alt`+arrow is frequently intercepted by tiling terminal emulators and readline. Printable `[`/`]` work reliably across all terminals and read naturally as "move the window".

**Alternatives considered:** `Shift`/`Ctrl`+arrow (indistinguishable in termbox); `Alt`+arrow (terminal-intercepted). Both rejected.

---

## [009-feat-horizontal-scroll] Sort-column highlight takes priority over frozen-column bold

**Date:** 2026-06-23
**Feature:** 009-feat-horizontal-scroll
**Status:** Accepted

**Context:** The frozen first column's header name is rendered bold. The first column can simultaneously be the active sort column (`OrderKey == 0`), which `printStatHeader` already renders with a distinct escape sequence (`\033[47;1m`).

**Decision:** When both apply on column 0, the sort highlight wins; the frozen-bold adds nothing.

**Rationale:** Layering two bold/background escape sequences on one cell risks a garbled render. Sort state is the more actionable signal, and both states reading as "emphasized" is visually consistent.

**Alternatives considered:** A third combined style for the overlap — extra escape-sequence handling for a rare case, rejected as over-engineering.

---

## [010-feat-overview-dashboard] Instance overview as a verbose panel mode, not a new view

**Date:** 2026-06-25
**Feature:** 010-feat-overview-dashboard
**Status:** Accepted

**Context:** The roadmap asked for an instance-overview "dashboard". The two top panels (`sysstat`/`pgstat`) already are an always-on mini-overview, so a separate full-screen card-grid view would duplicate them and also incur view-count test churn (`TestNew`, `Test_filterViews`, `TestView_VersionOK`).

**Decision:** Implement the overview as a display **mode** toggled by `v` that expands the two existing panels with extra `label:value` rows. It reuses the free-form `printSysstat`/`printPgstat` render path and registers **no** `view.View`.

**Rationale:** A separate screen would duplicate the always-visible header (antipattern) and force a new `printStat` branch + view-count test churn. A mode adds zero registered views and rides with the DBA over any detail screen — the actual differentiator vs pg_activity, whose rich block is pinned only over the process list.

**Alternatives considered:** A separate full-screen `overview` view with a card grid — rejected (duplication antipattern, invasive `printStat` branch, view-count test churn).

---

## [010-feat-overview-dashboard] `view.Verbose` + `config.verbose` dual boolean, not an overloaded `CollectExtra`

**Date:** 2026-06-25
**Feature:** 010-feat-overview-dashboard
**Status:** Accepted

**Context:** The mode flag must reach both the collector (to gate the dear collection) and the renderer/layout, and it must coexist with the active view's existing enrichment. The closest precedent is `CollectExtra` (ADR [001]).

**Decision:** A dedicated `view.View.Verbose bool` (rides `viewCh` to `Collector.Update`) mirrored into `top.config.verbose` (read by renderer/layout), kept in sync by `toggleVerbose` via the `showExtra` write-into-all-views idiom.

**Rationale:** `CollectExtra` is a single mutually-exclusive `int` (paired with `CollectProcPidStat`) — verbose must coexist with the active view's enrichment, not displace it. A separate boolean is cleaner and avoids the `prevCollectExtra` Reset path entirely (see the next ADR).

**Alternatives considered:** A `CollectVerboseSystem` `CollectExtra` constant — rejected (mutual exclusivity + would fire the `prevCollectExtra` Reset).

---

## [010-feat-overview-dashboard] Pure `topBandLayout` geometry function

**Date:** 2026-06-25
**Feature:** 010-feat-overview-dashboard
**Status:** Accepted

**Context:** Historically the panel heights were hard-coded literals (`y1=4`) and `cmdline` sat on `y=3..5`. Verbose grows the band to 7/9 rows, which requires recomputing the band height, shifting `cmdline`/`dbstat` down, and a height-guard — the invasive core, touching the shared UI loop on every screen.

**Decision:** Extract the band/cmdline/dbstat coordinate arithmetic into a pure `topBandLayout(verbose, maxY)` (`top/layout.go`) and keep `layout()` to `SetView` plumbing. Compact (and the guard fallback) reproduces the historical literals byte-identically.

**Rationale:** A pure function makes the only non-trivial geometry table-testable without gocui (the [009] `visibleColumns` precedent); the compact path stays unchanged.

**Alternatives considered:** Inline literal coords in `layout()` — rejected (untestable, error-prone).

---

## [010-feat-overview-dashboard] All-three system collection in a branch after (not modifying) the `collectExtra` switch

**Date:** 2026-06-25
**Feature:** 010-feat-overview-dashboard
**Status:** Accepted

**Context:** Verbose shows iostat+nicstat+filesyst simultaneously, but the existing `switch c.config.collectExtra` collects disk/net/fs **mutually exclusively** (one side panel at a time). Reusing that switch for all three (R1) would alter side-panel behavior.

**Decision:** Add a verbose-gated branch **after** the untouched switch, collecting disk+net+fs each tick with `== nil` guards (a source already populated by an active side panel is not re-collected); a per-source error leaves that source `nil` (→ `n/a`) without aborting the sample.

**Rationale:** Leaving the mutual-exclusion switch untouched and adding a parallel branch keeps the side panels unaffected. Per-source prev/curr snapshots already live on `Collector`, so collecting all three does not interfere.

**Alternatives considered:** Modify the mutual-exclusion switch to collect all three — rejected (R1: would change side-panel behavior).

---

## [010-feat-overview-dashboard] `verboseCollectState` first-tick re-arm without relying on `Reset()`

**Date:** 2026-06-25
**Feature:** 010-feat-overview-dashboard
**Status:** Accepted

**Context:** Delta rows have no prior sample on the first tick and on every OFF→ON re-enable, and must render `n/a` (not `0`) then. The natural "first tick" signal would be `c.Reset()`, but `toggleVerbose` deliberately does **not** call `Reset()` (ADR above), so re-arm cannot depend on it.

**Decision:** Group the verbose collection state in a named `verboseCollectState` sub-struct on `Collector`. Set `verboseFirstTick = !prevVerboseActive` on entry to the verbose branch (covers the very first tick and each OFF→ON re-enable without a view change), bridged to the renderer via the public `System.VerboseFirstTick`. `Reset()` clears the sub-struct in lockstep with prev/curr, but the re-arm does not rely on it.

**Rationale:** A named sub-struct keeps verbose-specific fields from leaking across the shared `Collector`. Deriving the flag from `prevVerboseActive` (not `Reset()`, not `len(slice)==0` — the first-tick slice is already filled with a zero delta) makes the first-tick `n/a` correct regardless of the toggle's no-Reset semantics.

**Alternatives considered:** Key first-tick off `c.Reset()` (broken — verbose toggle skips Reset) or off `len(Diskstats)==0` (broken — slice is non-empty on the first tick). Both rejected.

---

## [010-feat-overview-dashboard] Archiving backlog via `count(.ready) × wal_segment_size`

**Date:** 2026-06-25
**Feature:** 010-feat-overview-dashboard
**Status:** Accepted

**Context:** The replication row needs a WAL-archiving backlog signal that works over the network (no PL/Perl) and degrades cleanly when archiving is off.

**Decision:** `count(*)` of `.ready` entries in `pg_wal/archive_status` × `wal_segment_size` — pure SQL via `pg_ls_dir`, run as its own `QueryRow`. `n/a` on `archive_mode=off` / insufficient privileges (`pg_monitor`).

**Rationale:** Operationally that *is* the backlog, and pure SQL works remote with no PL/Perl. It adapts the exact existing precedent `count(1) * pg_size_bytes(current_setting('wal_segment_size'))` (`wal.go:6`).

**Alternatives considered:** LSN-diff `current_lsn − LSN(last_archived_wal)` — rejected (filename→LSN conversion is fiddly; lags non-linearly on archiver failure).

---

## [010-feat-overview-dashboard] Split `databases` into two queries to isolate the size aggregate

**Date:** 2026-06-25
**Feature:** 010-feat-overview-dashboard
**Status:** Accepted

**Context:** The `databases` row shows count + cache-hit (cheap, always available) **and** `sum(pg_database_size)` (expensive). One combined query would let a size-aggregate failure (or its cost) blank the cheap, always-available signals.

**Decision:** Two queries — `OverviewDatabases` (count + cache counters, cheap, always populated) and `OverviewDatabasesSize` (size sum, its own `QueryRow`, the only thing that gates `TotalSizeValid` and feeds the latency guard).

**Rationale:** Isolating the dear/fallible size aggregate means its failure or throttling degrades only the size/growth fields, leaving count + cache-hit intact. It is also the single source the latency guard throttles.

**Alternatives considered:** One combined query — rejected (a size failure would blank count/cache, and the dear aggregate could not be throttled independently).

---

## [010-feat-overview-dashboard] Latency guard threshold `max(refresh/4, 500ms)`, system rows never throttled

**Date:** 2026-06-25
**Feature:** 010-feat-overview-dashboard
**Status:** Accepted

**Context:** The dear no-twin aggregate (DB sizes/growth) can be costly on a loaded instance, but the system rows (iostat/nicstat/filesyst) must stay consistent with the full panels a DBA cross-checks. All cadence is hidden behind the single `z` interval knob — no second user knob.

**Decision:** Throttle **only** the dear no-twin aggregate via a per-source latency guard: skip its next collection when its last query exceeded `latencyGuardThreshold(refresh) = max(refresh/4, 500ms)` (named `verboseGuardFraction = 4`, `verboseGuardFloor = 500ms`), serving its last (**stale**) value — not `n/a` — and auto-resuming after a one-refresh cadence budget (`dbSizeThrottled` pure function). System rows collect every tick.

**Rationale:** System rows must never diverge from the full panels, so only the no-twin aggregate is throttled. A stale value is more honest than `n/a` for a value that merely lagged. The floor/fraction keeps the guard sane at both short (`refresh=1s` → 500ms floor) and long (`refresh=4s` → 1s) intervals; the exact constant was deferred from the tech-spec to task-decomposition.

**Alternatives considered:** A generic multi-rhythm scheduler now — rejected (YAGNI; the per-source seam suffices). Throttling system rows too — rejected (breaks the consistency promise).
