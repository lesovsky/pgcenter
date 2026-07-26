# Tech Debt Register

Known shortcuts, deferred improvements, and fragile areas. Updated after each feature.
Reviewed at the start of tech-spec planning to avoid worsening existing debt.

---

## Active Debt

### [022] Stats descriptions in `internal/stat/help.go` are dead code, and stale on top of it

**Added:** 2026-07-25 (surfaced during feature: 013-feat-activity-xmin-horizon)
**Severity:** Low
**Area:** `internal/stat/help.go`, `report/describe.go`

**What:** the file exports fifteen `PgStat*Description` constants and nothing in the repository reads any
of them — the descriptions `report -d` actually prints live in `report/describe.go`. Dead would be
harmless; dead and wrong is not. The replication block of `help.go` (lines 59-60) still calls the two
xmin-horizon columns `xact_age*` and `time_age*`, while the live file names the same two values
`horizon_xacts` and `horizon_age` (`report/describe.go:75-76`). Whoever opens the exported constant
gets column names the tool does not print. The activity block of `help.go` is not stale in this way:
`xact_age` there means transaction duration, exactly as in the live file.

**Why deferred:** the naming divergence predates this feature and has nothing to do with the activity
column it adds; the feature only decided (Decision 8) to document its three new columns in
`report/describe.go` alone instead of mirroring them into unreachable code. Deleting the file is the
right fix, but removing an exported API surface deserves its own change and its own review. Action at
that point: drop `internal/stat/help.go` outright — there is no consumer to migrate.

---

### [023] `horizon_xacts` names two different quantities on two screens

**Added:** 2026-07-25 (surfaced during feature: 013-feat-activity-xmin-horizon)
**Severity:** Low
**Area:** `internal/query/activity.go`, `internal/query/replication.go`

**What:** on the activity screen `horizon_xacts` is `age(backend_xmin)` — needs no GUC, never negative,
correct across xid wraparound. On the replication screen the column of the same name is
`(pg_last_committed_xact()).xid - backend_xmin`: it exists only when `track_commit_timestamp` is on
(the selector picks the extended query on that GUC alone, so with it off the column is simply absent),
it is a plain subtraction that goes negative when the standby's reported xmin is ahead of the last
locally committed xact, and it is not wraparound-safe. An operator moving between the two screens sees
one column name and is entitled to assume one quantity.

**Why deferred:** reconciling them is not a rename — one of the two definitions has to change, and the
replication one cannot simply adopt `age()` without deciding what that screen should show when
`track_commit_timestamp` is off. The backlog item that forces the decision already exists: the full
"who holds the xmin horizon" aggregate in [roadmap-0.12.0.md](roadmap-0.12.0.md) gathers all horizon
sources into one place and cannot put them side by side until the two formulas agree. Registered so
that work starts from a known divergence rather than discovering one.

---

### [024] Test port map promises clusters the test image does not contain

**Added:** 2026-07-25 (surfaced during feature: 013-feat-activity-xmin-horizon)
**Severity:** Low
**Area:** `internal/postgres/testing.go`, `testing/Dockerfile`

**What:** `NewTestConnectVersion` maps thirteen majors to ports — PG 14–19 on 21914–21919, plus PG 10–13
on 21910–21913 and PG 9.4–9.6 on 21994–21996 (two separate ranges, not one). The `pgcenter-testing`
image builds PG 14–19 only. A version table listing a mapped-but-absent version gets a connection
error, and callers turn that into `t.Skip` as the function's own doc comment instructs — which CI
reports as a pass. The map's comment ("EOL versions kept for reference") is honest about the intent,
but nothing signals that the coverage those tables appear to declare does not exist. Adjacent to [019],
which is about a skip swallowing the *remaining* versions rather than about the version being absent
in the first place.

**Why deferred:** the entries cannot just be deleted — version tables in `internal/query` and
`internal/stat` iterate over them, so removing a mapping changes what those tables assert, and how far
back pgcenter claims to be tested is a product decision, not a cleanup. This feature is directly
exposed to it: the new activity branch boundary sits at PG 13 (Decision 2), exactly where no live
cluster exists, so the only guard on that boundary is a table test. Action at that point: settle the
supported floor, then either drop the unreachable entries or make a mapped-but-absent version fail
instead of skip.

---



### [017] Beta apt channel left in the test image after PG 19 GA

**Added:** 2026-07-25 (feature: 012-feat-pg19-compatibility-baseline)
**Severity:** Low
**Area:** `testing/Dockerfile`

**What:** PG 19 is installed from the `jammy-pgdg-testing 19` channel because it is a beta. Package names
are explicit, so no foreign major version can arrive on its own — but while that source file is present,
PG 19 keeps coming from the beta channel on every rebuild, including the rebuild meant to verify GA.

**Why deferred:** the removal only makes sense once GA packages exist in the stable channel. Action at
that point: delete the source file and let PG 19 install from `jammy-pgdg main` alongside 14–18.

---

### [018] delay_time not exposed on the vacuum and analyze progress screens

**Added:** 2026-07-25 (feature: 012-feat-pg19-compatibility-baseline)
**Severity:** Low
**Area:** `internal/query/progress_vacuum.go`, `internal/query/progress_analyze.go`

**What:** both views carry `delay_time` — total time the operation slept due to cost-based delay. It is not
shown.

**Why deferred:** it arrived in PG 18, not PG 19, so it is not part of the PG 19 catch-up; showing it
honestly needs a third version branch in two selectors; and it reads zero unless `track_cost_delay_timing`
is on, which is off by default — a column of zeros on a typical installation. Worth its own decision about
whether the signal justifies the column.

---

### [019] Nine tests skip every version when one cluster is unavailable

**Added:** 2026-07-25 (surfaced during feature: 012-feat-pg19-compatibility-baseline)
**Severity:** Low
**Area:** `internal/query/common_test.go`, `internal/query/overview_test.go`, `internal/stat/postgres_test.go`

**What:** nine tests call `t.Skipf` inside their version loop but have no per-version `t.Run` wrapper, so
the skip fires on the parent test and every remaining version is skipped too. `common_test.go`'s
full-range list therefore already dead-skips in CI at `90500` — these tests provide no coverage today.

**Why deferred:** pre-existing, not opened by this feature, and the fix is adding the subtest wrapper to
nine tests in files this feature otherwise only appends a literal to — a refactor with its own review
surface.

---

### [020] Diff loop indexes the previous snapshot by the current snapshot's width

**Added:** 2026-07-25 (surfaced during feature: 012-feat-pg19-compatibility-baseline, security audit)
**Severity:** Low
**Area:** `internal/stat/postgres.go` (`diff`), `report/report.go`

**What:** the diff loop walks `curr.Ncols` and indexes `prev.Values[j][l]` without checking the previous
row's width. A mixed-width pair would dereference past the end.

**Why deferred:** the previous justification rested on the replay loop dropping the previous snapshot on
a version change, and concluded a mixed-width pair cannot occur. That guarantee is narrower than it
looked. `PGresult.validate` checks each row's width against `len(Cols)` and `Nrows` against the number
of decoded rows, and nothing else — not `Ncols` against `Cols`, not one sample's width against the
next's. A recorded result can therefore declare more columns than its rows carry, or widen between two
samples of the same version, and reach the diff loop on any screen with a non-empty `DiffIntvl`.
Both shapes were **reproduced** during feature 013's task-03 security audit — a result declaring
`Ncols: 999` over two real columns, and two same-version samples of differing width — so this is a
demonstrated panic rather than an inferred one. Severity stays Low for consistency with [009], which
covered a comparable malformed-archive class; what changed is that the reachability is now measured.
It stays deferred rather than closed because the screen this feature touches cannot be the trigger:
`activity` runs with `DiffIntvl {0,0}`, so `diff()` is never called there at all. Closing it means
validating archive-declared shapes generally — in `validate()`, which is the one place that
sees a result before any consumer does. Its siblings [021], [025] and [026] are now resolved; this is
the last of the family still open, and the code review of feature 013 noted that `align.SetAlign` is a
third unsafe consumer alongside `diff`, so fixing `diff` alone would not close the class.

---
### [016] Collector/parsers swallow errors silently — no logging facility

**Added:** 2026-06-25 (surfaced during debt audit; pre-existing since original pgcenter)
**Severity:** Low
**Area:** `internal/stat/*` (postgres.go, memstat.go, netdev.go, fsstat.go), `internal/postgres/postgres.go`

**What:** A cluster of `// TODO: log error` / `// TODO: handle errors` markers across the stat collectors and parsers (e.g. `internal/stat/postgres.go:486,656,825,838`, `memstat.go:59,65,115`, `netdev.go:122`, `fsstat.go:129`, `internal/postgres/postgres.go:163`) where parse/collection errors are dropped on the floor. A malformed `/proc` line or a failed side collection degrades silently to `n/a` with no operator-visible trace.

**Why deferred:** pgcenter is a full-screen gocui TUI that owns the terminal — there is nowhere to write logs while rendering. Closing this needs a product decision (a file-based logger / `--log-file` flag) before the TODOs can be wired up; it is not a one-line fix and touches every collector.

---

### [010] verbose recovery-`t` WAL standby path verified by substitution only

**Added:** 2026-06-25 (feature: 010-feat-overview-dashboard)
**Severity:** Low
**Area:** `internal/query/overview.go` (replication-lag/slots templates), integration tests

**What:** The verbose replication aggregates use the recovery-aware `{{.WalFunction1/2}}` templates, whose standby branch resolves to `pg_last_wal_receive_lsn()`. The fixture clusters (21914–21918) are all primaries, so the standby branch is verified only by string substitution through `query.Format`, not by live execution (running a standby-only function on a primary errors). Direct sibling of [006] (replslots `retained,KiB` standby path).

**Why deferred:** The test harness has no standby cluster; adding one is disproportionate for a path that reuses the already-proven `replication`-screen template. Manual standby check is the practical verification.

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

### [025] `PGresult.sort` does not bounds-check its sort key

**Added:** 2026-07-25 (surfaced during feature: 013-feat-activity-xmin-horizon, security audit)
**Resolved:** 2026-07-26 (feature: 013-feat-activity-xmin-horizon, code review)
**Severity:** Low
**Area:** `internal/stat/postgres.go` (`sort`, `validate`), `report/report.go`

**What:** `sort` indexes `r.Values[i][key]` without checking `key` against the row width, and the key
never comes from the data being sorted — it is either the screen's seed `OrderKey` from `view.New()`
(0 for most screens, 2 for `progress_index`, 4 for the `statements_*` ones) or an index resolved
against an earlier sample. `validate` does not close the gap: it compares each row's width to
`len(Cols)`, so a recorded result whose `Cols` is empty and whose rows are zero-width is accepted and
then panics on the first sort. Verified end to end during the task-03 security review — a crafted
archive aborts `pgcenter report` with `index out of range`, on any report type, with or without `-o`,
and with no version change involved.

**Resolution:** the guard landed in the stat layer rather than at the call site, exactly as this entry
proposed: `sort` now returns early when the key is negative or outside the row, keeping the input
order. One place covers the seed-`OrderKey` route this feature added, the pre-existing routes, and the
`top` caller. Code review reproduced the new route against `develop` before the fix — an archive whose
`-o` column is absent from a later layout panicked with `index out of range [4] with length 3` — and
`Test_sort_keyOutOfRange` was shown red on the unguarded code first.
---

### [026] Report error paths leave the reader blocked, so the command hangs instead of exiting

**Added:** 2026-07-25 (surfaced during feature: 013-feat-activity-xmin-horizon, security audit)
**Resolved:** 2026-07-26 (feature: 013-feat-activity-xmin-horizon, code review)
**Severity:** Low
**Area:** `report/report.go` (`doReport`, `readTar`, `processData`)

**What:** every `return err` in `processData` abandons the pipeline while `readTar` is still running.
The data channel is unbuffered and no one drains it, so the reader blocks on its next send, `doReport`
waits on the WaitGroup forever, and the command neither prints the error nor exits. This stopped being
theoretical here: the zero-width guard restored in task 03 ([021]) turns a slice-bounds panic into a
returned error, so a same-version archive whose samples widen — [020]'s territory — now hangs where it
used to crash. For a CLI that is arguably the worse of the two: a crash at least says that something
happened.

**Resolution:** fixed for every error path at once rather than for the newest one, as this entry
required. `doReport` now drains `dataCh` until `readTar` signals completion, so an error out of
`processData` lets the command print and exit instead of blocking on two unbuffered channels. Covered
by `Test_app_doReport_errorPathDoesNotHang`, which drives the real `doReport` — the other tests in that
file deliberately bypass it, which is why the hang went unnoticed until code review reproduced it.
---

### [021] Column widths not recomputed after a mid-archive version change

**Added:** 2026-07-25 (surfaced during feature: 012-feat-pg19-compatibility-baseline, architecture review)
**Resolved:** 2026-07-25 (feature: 013-feat-activity-xmin-horizon, task 03)
**Severity:** Low
**Area:** `report/report.go` (`formatStatSample`)

**What:** the alignment flag is set on the first printed sample and never reset, and `Configure` does not
clear it on a version change. An archive spanning a major upgrade (`record -a` across the upgrade) renders
its later samples with the earlier layout's widths.

**Resolution:** the replay loop now separates "no previous sample yet" from "the recorded version
changed" and, on the latter, drops three pieces of state derived from the layout it is leaving: the
alignment latch (together with the `ColsWidth`/`Cols` pair it produced, so the next sample recomputes
both), the header-repeat counter (reset to its seed value, so the new layout prints its own header
immediately), and the resolved `-o` column index. The third was not in the original report and is the
worst of the three: a stale index silently sorts the remaining samples by whatever column now sits at
that position. It is re-resolved against the next sample's column list, falling back meanwhile to the
screen's seed sort key captured before the first `Configure`, because the requested column may be
absent from the new layout. Landed with it: the zero-width guard in the report truncation path,
restored to match its twin in `top/printDataCell` (Decision 6) — a column the alignment never saw reads
as width 0 out of the `ColsWidth` map, and slicing on that panicked. That guard has its own
consequence, registered as [026]: it returns an error into a pipeline that does not unwind. The sibling
entry [020] stays open — see its own note for why this resolution does not close it.

---

### [012] verbose pgstat Size-formatted fields width-breathe between values

**Added:** 2026-06-25 (feature: 010-feat-overview-dashboard)
**Resolved:** 2026-06-25 (feature: 011-refactor-tech-debt-paydown, task 03, commit c89b686)
**Severity:** Low
**Area:** `top/stat.go` (verbose pgstat composers), `internal/pretty/pretty.go`

**What:** The verbose pgstat fields formatted via `pretty.Size` (databases size/growth, replication lag/retain/backlog) were variable-width, so the fields and their trailing labels shifted horizontally between samples; `naReserve` covered only the fixed-width fields.

**Resolution:** Added `pretty.SizeWidth(v, width)` (right-aligns `Size(v)` via `%*s`, never truncating — the `ReserveWidth` model) and applied it with a single `sizeFieldWidth = 8` const to the five Size fields, replacing their bare `naLiteral` n/a fallbacks with `naReserve(sizeFieldWidth)`. Value and n/a now share the reserve, so labels hold position across ticks and value↔n/a. `wal size` stays a bare `Size` (first field on its row, pushes no label). Locked by a value-vs-n/a byte-offset test (RED before, GREEN after) and updated goldens (padding only). Manual `v` check confirmed.

---

### [011] rateField duplicates pretty.RateUnit overflow logic

**Added:** 2026-06-25 (feature: 010-feat-overview-dashboard)
**Resolved:** 2026-06-25 (feature: 011-refactor-tech-debt-paydown, task 02, commit ee623fa)
**Severity:** Low
**Area:** `top/stat.go` (`rateField`), `internal/pretty/pretty.go`

**What:** `top/stat.go:rateField` re-implemented the overflow/divisor/ceil logic of `pretty.RateUnit`, differing only by the `" " + r/w` prefix between digits and unit (`1135 rMB/s`).

**Resolution:** Extracted the shared logic into an unexported `pretty.rateUnitParts(v, family, width) (field, unit)` core; `RateUnit` (byte-identical `9999MB/s` form) and a new exported `pretty.RateUnitPrefixed(v, family, prefix, width)` both delegate to it. `rateField` deleted, its four verbose disk/net call sites repointed. Byte-identity locked by a `TestRateUnitPrefixed` boundary table (hardcoded literals, not a circular call to the deleted func); the verbose disk/net goldens needed no edits — a positive equivalence signal.

---

### [009] tar entry size trusted for allocation in stat.NewPGresultFile

**Added:** 2026-06-22 (surfaced during feature: 008-feat-record-report-0-11-views, security audit)
**Resolved:** 2026-06-25 (feature: 011-refactor-tech-debt-paydown, task 01, commit 9a3c630)
**Severity:** Low
**Area:** `internal/stat/postgres.go` (`NewPGresultFile`), `report/report.go`

**What:** `NewPGresultFile` did `make([]byte, hdr.Size)` from the tar header size — an attacker-influenceable value for an archive received from a third party (A08 / CWE-789, unbounded allocation).

**Resolution:** Added exported `stat.MaxResultFileSize int64 = 256 << 20` (≈300× the largest real ~817 KB entry) and an int64-only guard in `NewPGresultFile` rejecting `bufsz < 0` (distinct error) and `bufsz > MaxResultFileSize` (`result file size %d exceeds limit %d bytes`) **before** `make` — no `int()` narrowing, so no gosec G115. The two real pre-alloc sinks (`report.readTar` `meta.*`/stat) inherit the cap via `NewPGresultFile`; the `sysinfo.*` branch (`io.ReadAll`, not a pre-alloc sink) got the same cap inline as defense-in-depth. Tests cover under/at/over limit, negative, and all three tar branches.

---

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
