# Code Research: 002-feat-iodelay-procpidstat

## Critical Finding: ADR Is Not an Obstacle

`decisions-log.md` [001] deferred "iodelay via Netlink taskstats". This feature uses `/proc/[pid]/stat` field 42 instead — different, simpler source, no Netlink. The ADR does not block this implementation. Tech debt [001] is directly resolved.

## 1. Entry Points

**`internal/stat/procpidstat.go`** — primary file. All sections need modification:
- `ProcPidStat` struct
- `readProcPidStatFile` — parse `suffix[39]`
- `buildProcPidResult` — new parameter + 2 columns
- `procPidResultCols` / `procPidResultNcols`
- `CheckIOAvailable` — model for `CheckDelayAcctAvailable`

**`top/config_view.go`** — `switchViewToProcPidStat` (lines 201–237): set `v.DelayAcctAvailable` alongside `v.IOAvailable`.

**`internal/stat/stat.go`** — `Collector.Update` line 267: pass `view.DelayAcctAvailable` to `buildProcPidResult`.

**`internal/view/view.go`** — `View` struct: add `DelayAcctAvailable bool`. Procpidstat entry: `Ncols: 17` → `19`.

## 2. Data Layer

### `/proc/[pid]/stat` field 42 — `delayacct_blkio_ticks`

After stripping `pid` and `(comm)` with `strings.LastIndex(line, ")")`:
- `suffix[11]` = utime (currently parsed)
- `suffix[12]` = stime (currently parsed)
- `suffix[39]` = `delayacct_blkio_ticks` ← new

Current length guard: `len(suffix) < 13`. Must extend to `len(suffix) < 40`.

Both existing golden test files (`pid_stat_normal_comm`, `pid_stat_space_comm`) have 50 suffix fields — `suffix[39]` exists and is `0` in both.

### Detection Probe: `/proc/sys/kernel/task_delayacct`

Authoritative runtime sysctl — readable without root (`-rw-r--r-- 1 root root`):
- `"1"` — delay accounting active
- `"0"` — compiled in but disabled at runtime

**On dev machine**: `CONFIG_TASK_DELAY_ACCT=y` but `kernel.task_delayacct = 0` → columns will show `""` during development.

```go
func CheckDelayAcctAvailable() bool {
    data, err := os.ReadFile("/proc/sys/kernel/task_delayacct")
    if err != nil { return false }
    return strings.TrimSpace(string(data)) == "1"
}
```

No PID needed (unlike `CheckIOAvailable`). Enable for testing: `sysctl -w kernel.task_delayacct=1`.

### IODelay in ProcPidStat

Add `IODelay float64` to `ProcPidStat`. Since IODelay comes from the same `/proc/[pid]/stat` file, **no new Collector maps needed** — `prevProcPidStats[pid].IODelay` / `currProcPidStats[pid].IODelay` carry values automatically.

## 3. 19-Column Layout (Final)

| Index | Name | Source |
|-------|------|--------|
| 0 | pid | SQL col 0 |
| 1 | datname | SQL col 1 |
| 2 | usename | SQL col 2 |
| 3 | state | SQL col 3 |
| 4 | wait_etype | SQL col 4 |
| 5 | wait_event | SQL col 5 |
| 6 | all_total,s | formatCPUTime(Utime+Stime) |
| 7 | us_total,s | formatCPUTime(Utime) |
| 8 | sy_total,s | formatCPUTime(Stime) |
| 9 | read_total,KiB | curIOs.ReadBytes/1024 |
| 10 | write_total,KiB | curIOs.WriteBytes/1024 |
| **11** | **iodelay_total,s** | **formatCPUTime(IODelay, ticks)** |
| 12 | %all | Δ(Utime+Stime)/(itv×ticks)×100/CPUs |
| 13 | %us | ΔUtime/(itv×ticks)×100/CPUs |
| 14 | %sy | ΔStime/(itv×ticks)×100/CPUs |
| 15 | read,KiB/s | ΔReadBytes/itv/1024 |
| 16 | write,KiB/s | ΔWriteBytes/itv/1024 |
| **17** | **%iodelay** | **ΔIODelay/(itv×ticks)×100** |
| 18 | query | SQL col 6 |

**Important**: `%iodelay` is NOT normalized by `cpuCount` (unlike %all/%us/%sy). Delay ticks are wall-clock blocked time, not CPU consumption.

Inserting col 11 shifts current cols 11–16 → 12–17. All index references in `buildProcPidResult` must be renumbered.

## 4. Tests to Update

**`internal/stat/procpidstat_test.go`**:
- All `== 17` assertions → `== 19` (lines 142, 146, 181, 182, 220, 287, 289, 291, 323, 349, 352)
- `expectedProcPidCols` — add `"iodelay_total,s"` at pos 11, `"%iodelay"` at pos 17, shift `"query"` to pos 18
- All `buildProcPidResult` calls — add `delayAcctAvailable bool` parameter
- All `row[N]` index assertions — shift old 11–16 → 12–17

New tests:
- `TestCheckDelayAcctAvailable` — probe function
- `TestReadProcPidStatIODelay` — golden file with non-zero `suffix[39]`
- `TestBuildProcPidResult_DelayAvailable` — cols 11 and 17 with non-zero iodelay
- `TestBuildProcPidResult_DelayUnavailable` — cols 11 and 17 render as `""`

New golden file: `internal/stat/testdata/proc/pid_stat_iodelay` — same structure, `suffix[39] = 500`.

**`internal/stat/stat_test.go`**:
- `TestCollectorUpdateProcPidStat17Cols` → rename to `...19Cols`
- `Ncols: 17` → `19`, add `DelayAcctAvailable: true`
- Both `assert.Equal`/`Len` for 17 → 19

**`record/record_test.go`**: line 145 `== 17` → `== 19`.

**`record/record.go`**: comment "7 of 17" → "7 of 19".

## 5. Complete File Change List

| File | Change |
|------|--------|
| `internal/stat/procpidstat.go` | `ProcPidStat +IODelay`; `readProcPidStatFile +suffix[39]`; `CheckDelayAcctAvailable()`; `procPidResultCols` 19 names; `procPidResultNcols` 19; `buildProcPidResult` new param + body |
| `internal/view/view.go` | `View +DelayAcctAvailable bool`; procpidstat `Ncols 17→19` |
| `internal/stat/stat.go` | Pass `view.DelayAcctAvailable` to `buildProcPidResult`; update comment |
| `top/config_view.go` | `switchViewToProcPidStat`: call `CheckDelayAcctAvailable()`, set `v.DelayAcctAvailable` |
| `internal/stat/procpidstat_test.go` | Update all 17→19; shift indices; 3–4 new test functions |
| `internal/stat/testdata/proc/pid_stat_iodelay` | New golden file, `suffix[39]=500` |
| `internal/stat/stat_test.go` | Rename test, `17→19`, `+DelayAcctAvailable: true` |
| `record/record_test.go` | `17→19` |
| `record/record.go` | Comment `17→19` |

## 6. Shared Utilities (No Changes Needed)

- `formatCPUTime(jiffies, ticks float64)` — reuse for `iodelay_total,s`
- `delta(prev, curr float64)` — reuse for `%iodelay` rate
- `nullString(s string)` — reuse for new columns
- `align.SetAlign` — operates on `r.Ncols` dynamically, transparent
