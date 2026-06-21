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
