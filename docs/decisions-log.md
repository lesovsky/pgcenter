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
**Status:** Accepted

**Context:** The procpidstat view uses a hybrid data source (SQL + procfs). The recorder only collects SQL results and cannot capture the enriched 17-column result. Adding procpidstat to recording would produce misleading 7-column output.

**Decision:** Add `NotRecordable bool` field to `view.View` with zero-value `false` (all existing views remain recordable). Set `NotRecordable: true` on procpidstat. `record/record.go:filterViews()` skips views where `NotRecordable` is set.

**Rationale:** Go zero value makes this backwards-compatible — no existing view definitions need to change. Cleaner than checking by view name in the recorder.

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
**Status:** Accepted

**Context:** Per-process iowait was planned but `/proc/[pid]/delays` does not exist in Linux. Delay accounting is only available via Netlink taskstats (`AF_NETLINK/NETLINK_GENERIC`), which is not in the codebase.

**Decision:** Defer iodelay to a separate future issue. v1 ships with CPU + IO bytes only.

**Rationale:** Implementing Netlink taskstats from scratch would significantly increase scope. The most valuable troubleshooting metrics (CPU%, IO throughput) are available without it.
