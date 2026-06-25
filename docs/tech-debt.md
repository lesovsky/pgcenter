# Tech Debt Register

Known shortcuts, deferred improvements, and fragile areas. Updated after each feature.
Reviewed at the start of tech-spec planning to avoid worsening existing debt.

---

## Active Debt

### [016] Collector/parsers swallow errors silently — no logging facility

**Added:** 2026-06-25 (surfaced during debt audit; pre-existing since original pgcenter)
**Severity:** Low
**Area:** `internal/stat/*` (postgres.go, memstat.go, netdev.go, fsstat.go), `internal/postgres/postgres.go`

**What:** A cluster of `// TODO: log error` / `// TODO: handle errors` markers across the stat collectors and parsers (e.g. `internal/stat/postgres.go:486,656,825,838`, `memstat.go:59,65,115`, `netdev.go:122`, `fsstat.go:129`, `internal/postgres/postgres.go:163`) where parse/collection errors are dropped on the floor. A malformed `/proc` line or a failed side collection degrades silently to `n/a` with no operator-visible trace.

**Why deferred:** pgcenter is a full-screen gocui TUI that owns the terminal — there is nowhere to write logs while rendering. Closing this needs a product decision (a file-based logger / `--log-file` flag) before the TODOs can be wired up; it is not a one-line fix and touches every collector.

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

### [006] replslots retained,KiB standby path not verified on a live standby

**Added:** 2026-06-21 (feature: 005-feat-replication-slots)
**Severity:** Low
**Area:** `internal/query/replication_slots.go`, integration tests

**What:** `retained,KiB` uses the recovery-aware `{{.WalFunction2}}()` template, which resolves to `pg_last_wal_receive_lsn()` on a standby. The integration tests (tier-1/2/3) run only against primaries, so the standby branch is correct-by-construction (same template the `replication` screen already uses on standbys) but not exercised by a dedicated live-standby test. Recorded as deferred-to-post-deploy in the QA report.

**Why deferred:** The test harness has no standby cluster; adding one is disproportionate for a path that reuses an already-proven template. Manual standby check is the practical verification.

---

### [003] All task reviews were self-reviews — real reviewer agents not run

**Added:** 2026-05-19 (feature: 001-feat-per-process-system-stats)
**Severity:** Low
**Area:** Entire feature codebase

**What:** All task reviewer subagents (dev-code-reviewer, dev-security-auditor, dev-test-reviewer) were run as structured self-reviews because the `Task`/`SendMessage` tools were not available in worktree agent contexts. Self-review JSON reports are present but were not produced by independent reviewer agents.

**Why deferred:** Tool availability constraint in the worktree agent execution environment. Code was manually verified via `make test`, `make lint`, `make vuln`, and user TUI testing.

---

## Resolved Debt

### [013] golangci-lint v1 config vs locally-installed v2 tool — lint runs only in CI

**Added:** 2026-06-25 (surfaced during feature: 010-feat-overview-dashboard, every task)
**Resolved:** 2026-06-25 (debt audit)
**Severity:** Low
**Area:** `.golangci.yml`, `.github/workflows/{default,release}.yml`

**What:** The repo's `.golangci.yml` was a v1-schema config, but the locally-installed `golangci-lint` is v2 (`unsupported version of the configuration`), so `make lint` could not run locally — tasks substituted `go vet` + `gofmt -l` and deferred the full lint to CI. (CI also silently ran v1, since its install path `…/cmd/golangci-lint@latest` omits the `/v2/` module prefix and resolves to the last v1 release.)

**Resolution:** Migrated `.golangci.yml` to the v2 schema via `golangci-lint migrate`. v2 folds `stylecheck` (ST*) and the new quickfix (QF*) categories into `staticcheck`; the v1 config enabled neither, so `staticcheck.checks` now carries `-ST*` / `-QF*` to preserve the exact v1 effective rule set (verified: `make lint` reports 0 issues, same as before). Switched both CI workflows to install the v2 binary (`…/v2/cmd/golangci-lint@latest`) and bumped the lint-tools cache key (`lint-v2` → `lint-v3-golangciv2`) so the stale v1 binary is not restored. Local and CI now run the same v2 tool against the same config.

---

### [015] govulncheck GO-2026-5037 (crypto/x509 stdlib) — local toolchain trailed CI

**Added:** 2026-06-25 (surfaced during feature: 010-feat-overview-dashboard, pre-deploy QA)
**Resolved:** 2026-06-25 (debt audit)
**Severity:** Low
**Area:** `go.mod`

**What:** `govulncheck` flagged GO-2026-5037 in the stdlib `crypto/x509`, fixed in Go 1.25.11. CI already ran 1.25.11, but the local toolchain trailed at 1.25.10, so `make vuln` reported the finding locally.

**Resolution:** Added `toolchain go1.25.11` to `go.mod`. With `GOTOOLCHAIN=auto`, every environment (local included) now builds and runs under ≥1.25.11, where the stdlib fix is present. Verified `go version` reports 1.25.11 after the directive. No source change.

---

### [014] bin/pgcenter was a tracked build artifact

**Added:** 2026-06-25 (surfaced during feature: 010-feat-overview-dashboard)
**Resolved:** 2026-06-25 (debt audit)
**Severity:** Low
**Area:** repository root (`bin/pgcenter`), `.gitignore`

**What:** `bin/pgcenter` was committed to the repo, so every `make build` rewrote it and dirtied the working tree (and risked an accidental binary commit).

**Resolution:** `git rm --cached bin/pgcenter` (file kept on disk) and created `.gitignore` with `/bin/`. The build output is now ignored and no longer churns the working tree.

---

### [008] record.Test_app_record panicked instead of skipping without a live PG

**Added:** 2026-06-22 (surfaced during feature: 008-feat-record-report-0-11-views)
**Resolved:** 2026-06-25 (debt audit)
**Severity:** Low
**Area:** `record/record_test.go`

**What:** `Test_app_record` panicked (nil-pointer in `app.record`) instead of `t.Skipf` when no live PostgreSQL was available, so `go test ./record/...` failed locally whenever the test clusters were down. Sibling of [005].

**Resolution:** Added a `postgres.NewTestConnect()` probe before the test loop; on connect error the test `t.Skipf`s cleanly (matching the rest of the suite) instead of proceeding into a nil-connection panic.

---

### [005] Test_doReload panicked instead of skipping when PG fixture is absent

**Added:** 2026-06-21 (surfaced during feature: 004-feat-bgwriter-checkpointer)
**Resolved:** 2026-06-25 (debt audit)
**Severity:** Low
**Area:** `top/reload_test.go`

**What:** `Test_doReload` panicked (nil conn in `doReload`) instead of `t.Skipf` when the PG fixture on port 21917 was not running, so `make test` failed locally whenever the test clusters were down.

**Resolution:** Replaced `assert.NoError(t, err)` after `NewTestConnect()` with an `if err != nil { t.Skipf(...) }` guard, so the test skips cleanly instead of dereferencing a nil connection.

---

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
