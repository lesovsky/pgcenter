# Tech Debt Register

Known shortcuts, deferred improvements, and fragile areas. Updated after each feature.
Reviewed at the start of tech-spec planning to avoid worsening existing debt.

---

## Active Debt

### [015] govulncheck GO-2026-5037 (crypto/x509 stdlib) — toolchain bump pending

**Added:** 2026-06-25 (surfaced during feature: 010-feat-overview-dashboard, pre-deploy QA)
**Severity:** Low
**Area:** CI toolchain (`go.mod` / workflows)

**What:** `govulncheck` flags GO-2026-5037 in the stdlib `crypto/x509`, fixed in Go 1.25.11; the local toolchain is 1.25.10. Not project code — the resolution is a toolchain bump in CI. Same class as the [004]-era bump already applied once; re-surfaced because the local environment trails the patch version.

**Why deferred:** Stdlib transitive path, not project code; the fix is the patch-version toolchain bump the CI gate requires (no source change).

---

### [014] bin/pgcenter is a tracked build artifact

**Added:** 2026-06-25 (surfaced during feature: 010-feat-overview-dashboard)
**Severity:** Low
**Area:** repository root (`bin/pgcenter`), `.gitignore`

**What:** `bin/pgcenter` is committed to the repo, so every `make build` rewrites it and dirties the working tree (and risks an accidental binary commit). Pre-existing, not introduced by feature 010 — only noticed because the manual-QA rebuild churned it.

**Why deferred:** Removing a tracked artifact and gitignoring it is a repo-hygiene change orthogonal to the feature; left for a dedicated cleanup.

---

### [013] golangci-lint v1 config vs locally-installed v2 tool — lint runs only in CI

**Added:** 2026-06-25 (surfaced during feature: 010-feat-overview-dashboard, every task)
**Severity:** Low
**Area:** `.golangci.yml`, local dev environment

**What:** The repo's `.golangci.yml` is a v1-schema config, but the locally-installed `golangci-lint` is v2 (`unsupported version of the configuration`), so `make lint` cannot run locally — every task in feature 010 substituted `go vet` + `gofmt -l` (and `gosec` where available) and deferred the full golangci-lint run to CI. The proper fix migrates the config to v2 (or pins the tool version).

**Why deferred:** Config migration is a cross-cutting change unrelated to the feature; CI still enforces the full lint, so coverage is not lost — only local convenience.

---

### [012] verbose pgstat Size-formatted fields width-breathe between values

**Added:** 2026-06-25 (feature: 010-feat-overview-dashboard)
**Severity:** Low
**Area:** `top/stat.go` (verbose pgstat composers)

**What:** The `n/a`-width reservation that keeps trailing labels static (`naReserve`) was applied only to fixed-width fields (cache-hit ratio, the `%d` workload rates). The verbose fields formatted via `pretty.Size` (databases size/growth, replication lag/retain/backlog) are inherently variable-width, so an `n/a`↔value width match is ill-defined and those fields/labels still shift horizontally between samples.

**Why deferred:** Fixing it needs a fixed-width `Size` variant (or per-field reserved budgets), which the feature did not size; the exact pgstat digit budgets were left to user verification. Cosmetic, no correctness impact.

---

### [011] rateField duplicates pretty.RateUnit overflow logic

**Added:** 2026-06-25 (feature: 010-feat-overview-dashboard)
**Severity:** Low
**Area:** `top/stat.go` (`rateField`), `internal/pretty/pretty.go` (`RateUnit`)

**What:** `top/stat.go:rateField` re-implements the overflow/divisor logic of `pretty.RateUnit` (it differs only in placing the r/w prefix *between* the digits and the unit, as the spec layout `1135 rMB/s` requires). Consolidating into one shared helper would touch `internal/pretty/pretty.go`, which was outside the allowed file set for the row-composer task.

**Why deferred:** Out of scope for the task that introduced it; the duplication is small and documented. A candidate for consolidation the next time `internal/pretty` is touched.

---

### [010] verbose recovery-`t` WAL standby path verified by substitution only

**Added:** 2026-06-25 (feature: 010-feat-overview-dashboard)
**Severity:** Low
**Area:** `internal/query/overview.go` (replication-lag/slots templates), integration tests

**What:** The verbose replication aggregates use the recovery-aware `{{.WalFunction1/2}}` templates, whose standby branch resolves to `pg_last_wal_receive_lsn()`. The fixture clusters (21914–21918) are all primaries, so the standby branch is verified only by string substitution through `query.Format`, not by live execution (running a standby-only function on a primary errors). Direct sibling of [006] (replslots `retained,KiB` standby path).

**Why deferred:** The test harness has no standby cluster; adding one is disproportionate for a path that reuses the already-proven `replication`-screen template. Manual standby check is the practical verification.

---

### [009] tar entry size trusted for allocation in stat.NewPGresultFile

**Added:** 2026-06-22 (surfaced during feature: 008-feat-record-report-0-11-views, security audit)
**Severity:** Low
**Area:** `internal/stat/postgres.go` (`NewPGresultFile`), `report/`

**What:** Report replay deserializes recorded archives via `NewPGresultFile`, which does `make([]byte, hdr.Size)` from the tar header size — an attacker-controlled value if the archive is untrusted (A08 / CWE-789, unbounded allocation). Pre-existing pattern shared by all recordable views; feature 008 did not introduce it but widened the set of view types flowing through this trust boundary.

**Why deferred:** Out of scope for feature 008 and low risk in practice — the operator owns and controls their own `pgcenter record` archives. A proper fix bounds `hdr.Size` against a sane maximum (or `io.LimitReader`) before allocating.

---

### [008] record.Test_app_record panics instead of skipping without a live PG

**Added:** 2026-06-22 (surfaced during feature: 008-feat-record-report-0-11-views)
**Severity:** Low
**Area:** `record/record_test.go` (`Test_app_record`), `record/record.go`

**What:** `Test_app_record` panics (nil-pointer in `app.record`, record.go:167) instead of `t.Skipf` when no live PostgreSQL is available, so `go test ./record/...` fails locally whenever the test clusters are down. Sibling of [005] (`top/reload_test.go`); the rest of the suite skips unavailable versions gracefully. Pre-existing, not caused by feature 008. CI is unaffected (the container provides live PG).

**Why deferred:** Pre-existing and environmental; the fix is the same `t.Skipf` guard pattern as the rest of the suite. Non-blocking.

---

### [006] replslots retained,KiB standby path not verified on a live standby

**Added:** 2026-06-21 (feature: 005-feat-replication-slots)
**Severity:** Low
**Area:** `internal/query/replication_slots.go`, integration tests

**What:** `retained,KiB` uses the recovery-aware `{{.WalFunction2}}()` template, which resolves to `pg_last_wal_receive_lsn()` on a standby. The integration tests (tier-1/2/3) run only against primaries, so the standby branch is correct-by-construction (same template the `replication` screen already uses on standbys) but not exercised by a dedicated live-standby test. Recorded as deferred-to-post-deploy in the QA report.

**Why deferred:** The test harness has no standby cluster; adding one is disproportionate for a path that reuses an already-proven template. Manual standby check is the practical verification.

---

### [005] Test_doReload panics instead of skipping when PG fixture is absent

**Added:** 2026-06-21 (surfaced during feature: 004-feat-bgwriter-checkpointer)
**Severity:** Low
**Area:** `top/reload_test.go`

**What:** `Test_doReload` panics (instead of `t.Skipf`) when the PG fixture on port 21917 is not running, so `make test` fails locally whenever the test clusters are down — unlike the rest of the suite, which skips unavailable versions gracefully. Pre-existing (confirmed on a clean baseline via `git stash`), not caused by feature 004. During feature 004 this panic masked local detection of a `record`-package test regression that CI later caught.

**Why deferred:** Pre-existing and environmental; the fix is to replace the panic with a `t.Skipf` guard matching the rest of the suite. Non-blocking for the feature.

---

### [003] All task reviews were self-reviews — real reviewer agents not run

**Added:** 2026-05-19 (feature: 001-feat-per-process-system-stats)
**Severity:** Low
**Area:** Entire feature codebase

**What:** All task reviewer subagents (dev-code-reviewer, dev-security-auditor, dev-test-reviewer) were run as structured self-reviews because the `Task`/`SendMessage` tools were not available in worktree agent contexts. Self-review JSON reports are present but were not produced by independent reviewer agents.

**Why deferred:** Tool availability constraint in the worktree agent execution environment. Code was manually verified via `make test`, `make lint`, `make vuln`, and user TUI testing.

---

## Resolved Debt

### [007] pg_stat_io NULL-safety covered structurally, no behavioral diff() test

**Added:** 2026-06-21 (feature: 006-feat-pg-stat-io)
**Resolved:** 2026-06-22 (feature: 008-feat-record-report-0-11-views, task 08)
**Severity:** Low
**Area:** `internal/stat/postgres_test.go`

**What:** The `coalesce(...,0)` NULL-safety of the diffed pg_stat_io/replslots columns was asserted only structurally (SQL contains `coalesce`); the behavioral half — `diff()` survives a zero-filled diffed cell and does not blank the screen — was unverified (an `internal/query`→`internal/stat` import cycle blocked a co-located test).

**Resolution:** `Test_DiffZeroFilledCells` added to `internal/stat/postgres_test.go` (task 08): feeds coalesced-`"0"` cumulative cells through `diff()`/`Compare`, asserting clean `"0"` deltas with no sample abort, io_key-style UniqueKey row pairing (non-positional), and a mixed zero-cell/counter row. Directly relevant since report replay runs recorded coalesced cells through `countDiff → Compare → diff`.

---

### [004] procpidstat col-index constants duplicated in report package

**Added:** 2026-05-19 (feature: 003-feat-procpidstat-record-report)
**Resolved:** 2026-06-22 (feature: 008-feat-record-report-0-11-views, task 09)
**Severity:** Low
**Area:** `report/report.go`, `internal/stat/procpidstat.go`

**What:** The procpidstat IO/iodelay column indices (9/10/11) were duplicated as an unexported local const block in `report/report.go` while the authoritative order lived only in the unexported `procPidResultCols` in `internal/stat/procpidstat.go`.

**Resolution:** Exported `ColReadTotalKiB`/`ColWriteTotalKiB`/`ColIODelayTotalS` from `internal/stat/procpidstat.go`; deleted the local block in `report/report.go` and referenced `stat.Col*` in `emitProcPidStatAvailabilityWarnings` (task 09). Added `TestProcPidColIndexConstants` to lock the index↔column-name invariant. No import cycle (report→stat is one-way).

---

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
