# Tech Debt Register

Known shortcuts, deferred improvements, and fragile areas. Updated after each feature.
Reviewed at the start of tech-spec planning to avoid worsening existing debt.

---

## Active Debt

### [004] procpidstat col-index constants duplicated in report package

**Added:** 2026-05-19 (feature: 003-feat-procpidstat-record-report)
**Severity:** Low
**Area:** `report/report.go`, `internal/stat/procpidstat.go`

**What:** Column indices for procpidstat IO columns (9 = `read_total,KiB`, 10 = `write_total,KiB`, 11 = `iodelay_total,s`) are declared as local constants `procPidStatColReadTotal`, `procPidStatColWriteTotal`, `procPidStatColIODelay` in `report/report.go`. The authoritative source (`internal/stat/procpidstat.go`) does not export these indices. If column order changes, both places need updating.

**Why deferred:** Non-blocking; report package has its own test coverage for the WARNING detection path. A small cleanup — export the constants from `internal/stat` and import them in report.

---

### [003] All task reviews were self-reviews — real reviewer agents not run

**Added:** 2026-05-19 (feature: 001-feat-per-process-system-stats)
**Severity:** Low
**Area:** Entire feature codebase

**What:** All task reviewer subagents (dev-code-reviewer, dev-security-auditor, dev-test-reviewer) were run as structured self-reviews because the `Task`/`SendMessage` tools were not available in worktree agent contexts. Self-review JSON reports are present but were not produced by independent reviewer agents.

**Why deferred:** Tool availability constraint in the worktree agent execution environment. Code was manually verified via `make test`, `make lint`, `make vuln`, and user TUI testing.

---

## Resolved Debt

### [002] procpidstat record/report — not integrated with recorder

**Added:** 2026-05-19 (feature: 001-feat-per-process-system-stats)
**Resolved:** 2026-05-19 (feature: 003-feat-procpidstat-record-report)
**Severity:** Low
**Area:** `record/`, `report/`, `internal/stat/procpidstat.go`

**What:** The procpidstat screen could not be recorded with `pgcenter record` or replayed in `pgcenter report`. The recorder only worked with SQL-sourced views; the procpidstat enrichment (procfs join) happened in the TUI layer and was not captured.

**Resolution:** Resolved by 003-feat-procpidstat-record-report: `tarRecorder` is now stateful (prev/curr procfs maps); `collect()` runs procfs enrichment after the SQL loop; `write()` appends `sysinfo.TIMESTAMP.json`; `report -N` flag reads the recorded data. Local/remote gate in `app.setup()` via `db.Local`.

---

### [001] procpidstat iodelay — Netlink taskstats not implemented

**Added:** 2026-05-19 (feature: 001-feat-per-process-system-stats)
**Resolved:** 2026-05-19 (feature: 002-feat-iodelay-procpidstat)
**Severity:** Low
**Area:** `internal/stat/procpidstat.go`, issues #118/#123

**What:** Per-process iowait (`wa%`, `iodelay` columns) was absent from the procpidstat screen. Delay accounting data was assumed to require the Netlink taskstats API (`AF_NETLINK/NETLINK_GENERIC`), which is not in the codebase. Placeholder issues #118 and #123 originally requested this metric.

**Why deferred:** Implementing a Netlink taskstats client from scratch would have doubled the feature scope. The most actionable metrics (CPU%, IO throughput) are available without it.

**Resolution:** Resolved by 002-feat-iodelay-procpidstat: implemented via `/proc/[pid]/stat` field 42 (`delayacct_blkio_ticks`) — no Netlink required. Availability is probed once at screen open via `/proc/sys/kernel/task_delayacct` (`CheckDelayAcctAvailable()`). The procpidstat screen now exposes two new columns (`iodelay_total,s` and `%iodelay`).
