---
created: 2026-05-19
status: draft
branch: feature/iodelay-procpidstat
size: S
---

# Tech Spec: iodelay Columns in procpidstat Screen

## Solution

Extend the existing procpidstat screen (`Shift+S`) with two new columns sourced from
`/proc/[pid]/stat` field 42 (`delayacct_blkio_ticks`): `iodelay_total,s` (accumulated
IO wait in `HH:MM:SS`) and `%iodelay` (fraction of time spent in D-state between ticks,
in percent). A new availability probe reads `/proc/sys/kernel/task_delayacct` at screen-open
time; when the sysctl is disabled or absent, both columns render as `""` and a warning
is shown in the cmdline area.

Total column count grows from 17 to 19. No new dependencies, no new Collector maps —
`IODelay` rides in the existing `ProcPidStat` struct and `prevProcPidStats`/`currProcPidStats` maps.

## Architecture

### What we're building/modifying

- **`internal/stat/procpidstat.go`** — primary file: extend `ProcPidStat` struct, parser, probe function, column definitions, and `buildProcPidResult`
- **`internal/view/view.go`** — add `DelayAcctAvailable bool` to `View` struct; update procpidstat `Ncols` 17→19
- **`internal/stat/stat.go`** — pass `view.DelayAcctAvailable` to `buildProcPidResult`
- **`top/config_view.go`** — call probe at screen-open, set `v.DelayAcctAvailable`, update warning logic
- **`record/record.go`** — comment update (7 of 17 → 7 of 19)

### How it works

```
Shift+S pressed
  → switchViewToProcPidStat()
      → CheckIOAvailable(probePID)         // existing probe
      → CheckDelayAcctAvailable()          // new: reads /proc/sys/kernel/task_delayacct
      → patch v.IOAvailable, v.DelayAcctAvailable
      → send v on viewCh
      → printCmdline(g, ...) — one of 4 branches (see Decisions)

Every refresh tick (Collector.Update):
  → collectPostgresStat() — 7-col pg_stat_activity result
  → collectProcPidStat():
      for each PID in result:
        readProcPidStatFile() → ProcPidStat{Utime, Stime, IODelay}   // IODelay new
        readProcPidIOFile()   → ProcPidIO{ReadBytes, WriteBytes}
  → buildProcPidResult(activity, prevStats, currStats,
                        prevIO, currIO,
                        ioAvailable, delayAcctAvailable,   // delayAcctAvailable new
                        ticks, itv, cpuCount)
      → 19-column PGresult (see Data Models for canonical index→name mapping):
          cols 0–5:   SQL passthrough (pid…wait_event)
          cols 6–11:  accumulated stats (CPU: 6–8, IO bytes: 9–10, iodelay: 11)
          cols 12–17: rate stats (CPU%: 12–14, IO KiB/s: 15–16, iodelay%: 17)
          col  18:    query (SQL passthrough)
  → render via gocui
```

**Column rendering rules for new columns:**

| Condition | `iodelay_total,s` | `%iodelay` |
|-----------|-------------------|------------|
| `!delayAcctAvailable` | `""` | `""` |
| `delayAcctAvailable && !validPID` | `"00:00:00"` | `"0.00"` |
| `delayAcctAvailable && validPID && (!havePrevCPU \|\| ticks <= 0)` (first tick) | `formatCPUTime(curr.IODelay, ticks)` if `ticks > 0`, else `"0:00:00"` | `""` |
| `delayAcctAvailable && validPID && havePrevCPU && itv > 0 && ticks > 0` | `formatCPUTime(curr.IODelay, ticks)` | `ΔIODelay/(itv×ticks)×100` formatted `%.2f` |

Note: `iodelay_total,s` uses `curr.IODelay` (not delta) — it's an accumulated counter.
`%iodelay` uses `delta(prev.IODelay, curr.IODelay)` via existing `delta()` helper.
Guard `ticks > 0` required before `formatCPUTime` to prevent division-by-zero producing
`int64(+Inf) = MinInt64` and a corrupt `HH:MM:SS` string.

## Decisions

### Decision 1: `/proc/[pid]/stat` field 42 instead of Netlink taskstats

**Decision:** Read `delayacct_blkio_ticks` from `suffix[39]` in `/proc/[pid]/stat`.

**Rationale:** No new dependencies (no Netlink socket, no `golang.org/x/sys/unix` package),
minimal implementation delta (one extra field parsed from a file already opened each tick),
and sufficient precision (clock ticks) for DBA troubleshooting. Supersedes ADR [001-feat-per-process-system-stats] which deferred iodelay assuming Netlink was required.

**Alternatives considered:**
- Netlink taskstats (`AF_NETLINK/NETLINK_GENERIC`): nanosecond precision, but requires
  Generic Netlink socket, new dependency, and significantly larger implementation scope. Rejected.

---

### Decision 2: Availability probe via `/proc/sys/kernel/task_delayacct` sysctl

**Decision:** `CheckDelayAcctAvailable()` reads `/proc/sys/kernel/task_delayacct`; returns
`true` iff content is `"1"`. No PID argument needed.

**Rationale:** This sysctl is the authoritative runtime state of delay accounting — readable
without root (`-rw-r--r-- 1 root root`). If the file is absent (`CONFIG_TASK_DELAY_ACCT=n`
or kernel < 2.6.18), `os.ReadFile` returns an error → function returns `false` → columns
render as `""`. This single probe covers all cases: kernel support absent, sysctl disabled,
sysctl enabled.

**Alternatives considered:**
- Parse `/boot/config-$(uname -r)` for `CONFIG_TASK_DELAY_ACCT=y`: brittle, requires
  shell invocation, not authoritative for runtime state. Rejected.
- Check field 42 value after two ticks (non-zero = available): unreliable (zero is a valid
  accumulated value for a new process). Rejected.

---

### Decision 3: `%iodelay` not normalized by `cpuCount`

**Decision:** Formula `ΔIODelay / (itv × ticks) × 100` with no division by `cpuCount`.

**Rationale:** `delayacct_blkio_ticks` counts wall-clock ticks the process spent blocked,
regardless of CPU count. A single-threaded process can be 100% IO-blocked whether the
machine has 1 or 64 cores. Normalizing by cpuCount would produce misleadingly small numbers
(e.g., 1.56% on a 64-core machine for a fully IO-blocked process). Contrast with `%all/%us/%sy`
which measure CPU utilization and correctly normalize because CPU time is shared across cores.

---

### Decision 4: Single-shot probe at screen open; no periodic re-probe

**Decision:** `CheckDelayAcctAvailable()` is called once in `switchViewToProcPidStat`, result
stored in `v.DelayAcctAvailable`. Not re-checked on each tick.

**Rationale:** `kernel.task_delayacct` changes at runtime only via explicit `sysctl -w`, which
is rare and intentional. Polling the sysctl file on every tick adds unnecessary syscalls.
Re-opening the screen (`Shift+S` → different view → `Shift+S`) triggers a fresh probe, which
is the natural UX affordance when the user changes system configuration.

---

### Decision 5: Combined warning when both IO and delayacct unavailable

**Decision:** `switchViewToProcPidStat` uses a single 4-branch `if/else` for `printCmdline`:

```
!ioAvailable && !delayAcctAvailable → combined message
!ioAvailable                        → IO-only message (existing)
!delayAcctAvailable                 → delayacct-only message
else                                → v.Msg (normal)
```

**Warning texts:**
- IO only (existing, no change): `"IO stats unavailable (cannot read /proc/%d/io): run as postgres user or via sudo."`
- delayacct only (new): `"iodelay unavailable (task_delayacct=0): run sysctl -w kernel.task_delayacct=1, then re-open screen"`
- combined (new): `"IO stats and iodelay unavailable: run as postgres user + sysctl -w kernel.task_delayacct=1, then re-open screen"`

**Rationale:** `printCmdline` requires mutual exclusion (calling it twice in the same handler
overwrites the first message before the user can read it — established constraint in `patterns.md`).
Combined message avoids losing critical information when both probes fail.

---

### Decision 6: `IODelay float64` in existing `ProcPidStat`; no new maps

**Decision:** Add `IODelay float64` to the existing `ProcPidStat` struct. Prev/curr values
are carried by the already-existing `prevProcPidStats map[int]ProcPidStat` and
`currProcPidStats map[int]ProcPidStat` in `internal/stat/stat.go`.

**Rationale:** `delayacct_blkio_ticks` comes from the same `/proc/[pid]/stat` file as
`utime`/`stime`, parsed in the same `readProcPidStatFile` call. Introducing separate maps
would duplicate the data transport path with no benefit. Adding a field to the existing struct
is the minimal, idiomatic change.

## Data Models

### `ProcPidStat` struct extension

```go
// internal/stat/procpidstat.go
type ProcPidStat struct {
    Utime   float64 // user mode jiffies (existing)
    Stime   float64 // kernel mode jiffies (existing)
    IODelay float64 // block IO delay jiffies, /proc/[pid]/stat field 42 (new)
}
```

### `View` struct extension

```go
// internal/view/view.go
type View struct {
    // ... existing fields ...
    IOAvailable       bool // existing
    DelayAcctAvailable bool // new: /proc/sys/kernel/task_delayacct == "1"
}
```

### `procPidResultCols` (19 columns)

```go
var procPidResultCols = []string{
    "pid", "datname", "usename", "state", "wait_etype", "wait_event",
    "all_total,s", "us_total,s", "sy_total,s",
    "read_total,KiB", "write_total,KiB",
    "iodelay_total,s",                           // new, index 11
    "%all", "%us", "%sy",
    "read,KiB/s", "write,KiB/s",
    "%iodelay",                                   // new, index 17
    "query",
}
const procPidResultNcols = 19
```

### `buildProcPidResult` signature

```go
func buildProcPidResult(
    activity      PGresult,
    prevStats, currStats map[int]ProcPidStat,
    prevIO, currIO       map[int]ProcPidIO,
    ioAvailable         bool,
    delayAcctAvailable  bool,   // new
    ticks               float64,
    itv                 float64,
    cpuCount            int,
) PGresult
```

### `CheckDelayAcctAvailable` function

```go
// internal/stat/procpidstat.go
// CheckDelayAcctAvailable reports whether delay accounting is active at runtime.
// It reads /proc/sys/kernel/task_delayacct; returns false if the file is absent
// (CONFIG_TASK_DELAY_ACCT=n or kernel < 2.6.18) or contains "0".
func CheckDelayAcctAvailable() bool {
    f, err := os.Open("/proc/sys/kernel/task_delayacct")
    if err != nil {
        return false
    }
    defer func() { _ = f.Close() }()
    var buf [4]byte
    n, _ := f.Read(buf[:])
    return strings.TrimSpace(string(buf[:n])) == "1"
}
```

Read is bounded to 4 bytes (sufficient for `"0\n"` or `"1\n"`) to avoid unbounded
`os.ReadFile` on a procfs virtual file (defensive practice consistent with existing parsers).

## Dependencies

### New packages
None.

### Using existing (from project)
- `os.ReadFile` (stdlib) — probe sysctl file
- `strings.TrimSpace` (stdlib) — parse sysctl value
- `formatCPUTime(jiffies, ticks float64) string` — reuse for `iodelay_total,s`
- `delta(prev, curr float64) float64` — reuse for `%iodelay` rate
- `nullString(s string) sql.NullString` — reuse for new columns

## Testing Strategy

**Feature size:** S

### Unit tests

**`internal/stat/procpidstat_test.go`** — update + extend:
- Update `expectedProcPidCols` slice: insert `"iodelay_total,s"` at index 11, `"%iodelay"` at index 17, shift `"query"` to index 18
- Update all `== 17` / `Len(..., 17)` assertions → `19` (lines 142, 146, 181, 182, 220, 287, 289, 291, 323, 349, 352)
- Shift all `row[N]` index assertions: former cols 11–16 → 12–17; `query` col 16 → 18
- Add `delayAcctAvailable bool` parameter to all existing `buildProcPidResult` call sites in tests
- Add `TestCheckDelayAcctAvailable`: call with a synthetic sysctl path (or test against live `/proc/sys/kernel/task_delayacct`), assert bool result
- Add `TestReadProcPidStatIODelay`: read new golden file `pid_stat_iodelay`, assert `IODelay == 500`
- Add `TestReadProcPidStatTruncated`: golden file with exactly 39 suffix fields; assert graceful return (`IODelay == 0`, no panic) — guards against off-by-one in the `len(suffix) < 40` guard
- Add `TestBuildProcPidResult_DelayAvailable`: `delayAcctAvailable=true`, non-zero IODelay in currStats; assert col 11 is `HH:MM:SS` and col 17 is `"%.2f"` string
- Add `TestBuildProcPidResult_DelayUnavailable`: `delayAcctAvailable=false`; assert col 11 and col 17 are `""`

**New golden file `internal/stat/testdata/proc/pid_stat_iodelay`:**
Same format as `pid_stat_normal_comm` but with `suffix[39] = 500` (all other fields identical).

**`internal/stat/stat_test.go`** — update:
- Rename `TestCollectorUpdateProcPidStat17Cols` → `TestCollectorUpdateProcPidStat19Cols`
- Change `Ncols: 17` → `19` in the view config literal
- Add `DelayAcctAvailable: true` to the view config literal
- Change `assert.Equal(t, 17, ...)` and `assert.Len(t, ..., 17)` → `19`

**`record/record_test.go`** — update:
- Line 145: `assert.Equal(t, 17, pp.Ncols)` → `assert.Equal(t, 19, pp.Ncols)`

### Integration tests
None — procfs data is non-deterministic in CI. The unit tests with golden files provide sufficient parser coverage.

### E2E tests
None — S-size feature, manual TUI verification covers the rendered output.

## Agent Verification Plan

**Source:** user-spec "Как проверить" section.

### Verification approach

Agent verifies via `bash` after each implementation wave: build succeeds, tests pass, lint clean.
User verifies the rendered TUI manually with sysctl toggle.

### Per-task verification

| Task | verify | What to check |
|------|--------|--------------|
| 1 (core impl) | bash | `make build && make test && make lint` — no errors |
| 2 (tests) | bash | `make test` — new iodelay tests appear and pass; no regressions |
| 3 (docs) | bash | `git diff --stat docs/` — tech-debt [001] resolved, ADR entry added |

### Tools required
`bash` only — no Playwright, no curl, no MCP tools needed.

## Backward Compatibility

**Breaking changes:** no — all changed types are internal to the `stat` package or additive struct fields.

**`buildProcPidResult` signature change:** internal function; all callers (`internal/stat/stat.go`) are updated in the same task. No public API is affected.

**`view.View.DelayAcctAvailable bool`:** zero value `false` — all existing views silently inherit "unavailable", which is the correct default (they don't use delay accounting).

**`procPidResultNcols` 17→19:** affects test assertions in `procpidstat_test.go`, `stat_test.go`, and `record/record_test.go` — all updated in Task 2.

**`record/record.go` comment:** cosmetic text update only.

## Risks

| Risk | Mitigation |
|------|-----------|
| `Ncols` mismatch between `view.go` and `procPidResultNcols` causes panic in `align.SetAlign()` | Updated synchronously in Task 1; `TestCollectorUpdateProcPidStat19Cols` in Task 2 catches divergence |
| `printCmdline` mutual exclusion: calling it twice in one handler silently discards the first message | 4-branch `if/else` guarantees exactly one call per execution path; covered by code review |
| `dev` machine has `kernel.task_delayacct=0` — iodelay columns always `""` during development | Enable with `sysctl -w kernel.task_delayacct=1` for manual testing; unit tests use golden files and don't depend on live sysctl |
| `suffix[39]` field index: off-by-one error during implementation | Explicit guard `len(suffix) < 40`; `TestReadProcPidStatIODelay` and `TestReadProcPidStatTruncated` golden file tests catch wrong index |
| `formatCPUTime` called with `ticks=0` produces corrupt `HH:MM:SS` via `int64(+Inf)=MinInt64` | Guard `ticks > 0` on all `iodelay_total,s` rendering paths (see Architecture rendering table) |
| `probePID` from `pg_stat_activity` used to probe `/proc/<pid>/io` (pre-existing behavior, not introduced by this feature) | Out of scope — pre-existing risk in `switchViewToProcPidStat`; fallback to PID 1 caps worst case |

## Acceptance Criteria

- [ ] `make build` succeeds on the feature branch
- [ ] `make test` passes — including all 4 new iodelay unit tests and no regressions
- [ ] `make lint` and `make vuln` pass clean
- [ ] `procPidResultNcols == 19` and `view.Ncols == 19` for procpidstat view
- [ ] `buildProcPidResult` with `delayAcctAvailable=true` returns 19 columns with non-empty col 11 and col 17
- [ ] `buildProcPidResult` with `delayAcctAvailable=false` returns 19 columns with `""` at col 11 and col 17
- [ ] `CheckDelayAcctAvailable()` returns false when sysctl file is absent (tested via golden path or mocked path)
- [ ] Tech debt `[001]` marked `resolved` in `docs/tech-debt.md`
- [ ] New ADR entry added to `docs/decisions-log.md` superseding [001-feat] iodelay decision

## Implementation Tasks

### Wave 1: Core implementation

#### Task 1: Extend procpidstat stat layer and screen handler

- **Description:** Extend the procpidstat data layer with iodelay support: add `IODelay` to `ProcPidStat`, parse `suffix[39]`, add `CheckDelayAcctAvailable()`, grow `buildProcPidResult` to 19 columns with `delayAcctAvailable` parameter. Wire `DelayAcctAvailable` through `view.View` and `stat.go`. Replace the 2-branch warning in `switchViewToProcPidStat` with a 4-branch `if/else`. Also update all existing `buildProcPidResult` call sites in test files to add the new parameter (required for package compilation — `make lint` runs golangci-lint which compiles `_test.go`).
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `make build && make lint`
- **Files to modify:** `internal/stat/procpidstat.go`, `internal/view/view.go`, `internal/stat/stat.go`, `top/config_view.go`, `record/record.go`, `internal/stat/procpidstat_test.go` (call-site updates only)
- **Files to read:** `internal/stat/procpidstat.go`, `internal/stat/stat.go`, `internal/view/view.go`, `top/config_view.go`, `docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-tech-spec.md`

### Wave 2: Tests and documentation

#### Task 2: Add new tests and golden files

- **Description:** Add new test functions to `procpidstat_test.go` (`TestCheckDelayAcctAvailable`, `TestReadProcPidStatIODelay`, `TestReadProcPidStatTruncated`, `TestBuildProcPidResult_DelayAvailable`, `TestBuildProcPidResult_DelayUnavailable`) and two golden files (`pid_stat_iodelay` with `suffix[39]=500`, `pid_stat_truncated` with 39 suffix fields). Update `stat_test.go` (rename test `17Cols→19Cols`, `Ncols 17→19`, add `DelayAcctAvailable: true`, fix `assert.NotEqual(t, 17, ...)` → `19` at line 215). Update `record/record_test.go` (`Ncols 17→19`).
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `make test`
- **Files to modify:** `internal/stat/procpidstat_test.go`, `internal/stat/testdata/proc/pid_stat_iodelay` (new), `internal/stat/testdata/proc/pid_stat_truncated` (new), `internal/stat/stat_test.go`, `record/record_test.go`
- **Files to read:** `internal/stat/procpidstat_test.go`, `internal/stat/stat_test.go`, `internal/stat/testdata/proc/pid_stat_normal_comm`, `internal/stat/procpidstat.go`, `record/record_test.go`

#### Task 3: Update project knowledge and ADR log

- **Description:** Mark tech debt `[001]` as resolved in `docs/tech-debt.md` (move to Resolved section with resolution note). Add new ADR entry to `docs/decisions-log.md` documenting the `/proc/[pid]/stat` field 42 approach, explicitly superseding the prior "iodelay deferred" decision. Update `docs/features-catalog.md` to reflect the new iodelay columns and remove the "Per-process iowait not available" limitation note.
- **Skill:** documentation-writing
- **Reviewers:** dev-code-reviewer
- **Verify:** bash — `git diff --stat docs/`
- **Files to modify:** `docs/tech-debt.md`, `docs/decisions-log.md`, `docs/features-catalog.md`
- **Files to read:** `docs/tech-debt.md`, `docs/decisions-log.md`, `docs/features-catalog.md`, `docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat.md`

### Final Wave

#### Task 4: Pre-deploy QA

- **Description:** Acceptance testing: run full test suite, verify all acceptance criteria from user-spec and tech-spec, perform manual TUI verification with `kernel.task_delayacct=1` (positive) and `=0` (negative) scenarios.
- **Skill:** pre-deploy-qa
- **Reviewers:** none
- **Verify:** bash — `make test && make lint && make vuln`
- **Files to modify:** none
- **Files to read:** `docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat.md`, `docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-tech-spec.md`
