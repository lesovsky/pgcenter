// Stuff related to per-process stats read from /proc/[pid]/stat and /proc/[pid]/io.

package stat

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// procPidResultCols is the canonical 19-column header used by BuildProcPidResult.
// The order matches the view config in internal/view; any deviation here causes
// a panic in align.SetAlign() because the view declares Ncols=19.
var procPidResultCols = []string{
	"pid", "datname", "usename", "state", "wait_etype", "wait_event",
	"all_total,s", "us_total,s", "sy_total,s",
	"read_total,KiB", "write_total,KiB",
	"iodelay_total,s",
	"%all", "%us", "%sy",
	"read,KiB/s", "write,KiB/s",
	"%iodelay",
	"query",
}

const procPidResultNcols = 19

// ProcPidStat describes raw per-process CPU usage values from /proc/[pid]/stat.
// Values are unscaled (jiffies), not seconds.
type ProcPidStat struct {
	Utime   float64 // user mode time
	Stime   float64 // kernel mode time
	IODelay float64 // block IO delay (delayacct_blkio_ticks), /proc/[pid]/stat field 42
}

// ProcPidIO describes raw per-process IO bytes from /proc/[pid]/io.
type ProcPidIO struct {
	ReadBytes  float64 // bytes physically read from storage
	WriteBytes float64 // bytes physically written to storage
}

// SysInfo is the JSON-serializable container for runtime values captured by
// the recorder at session start and replayed by the reporter: CLK_TCK ticks
// and the local CPU count. One sysinfo.TIMESTAMP.json entry is written per
// tick alongside the procpidstat result.
type SysInfo struct {
	Ticks    float64 `json:"ticks"`
	CPUCount int     `json:"cpu_count"`
}

// ReadProcPidStat reads /proc/<pid>/stat and returns ProcPidStat.
func ReadProcPidStat(pid int) (ProcPidStat, error) {
	return readProcPidStatFile(fmt.Sprintf("/proc/%d/stat", pid))
}

// readProcPidStatFile reads a stat file from the given path and parses utime/stime.
// It is split out so unit tests can exercise the parser with golden files.
func readProcPidStatFile(statfile string) (ProcPidStat, error) {
	var stat ProcPidStat

	f, err := os.Open(filepath.Clean(statfile))
	if err != nil {
		return stat, err
	}
	defer func() {
		_ = f.Close()
	}()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return stat, fmt.Errorf("%s bad content: %w", statfile, err)
		}
		return stat, fmt.Errorf("%s bad content: empty file", statfile)
	}

	line := scanner.Text()

	// The comm field (process name) is wrapped in parentheses and may contain
	// spaces and even ')'. Use the LAST ')' to locate the field boundary so we
	// can safely split the rest on whitespace.
	idx := strings.LastIndex(line, ")")
	if idx == -1 || idx+2 > len(line) {
		return stat, fmt.Errorf("%s bad content: missing comm boundary in '%s'", statfile, line)
	}

	suffix := strings.Fields(line[idx+2:])
	// Indexes inside the suffix (0-based, comm and pid stripped):
	//   0 = state, 1 = ppid, 2 = pgrp, 3 = session, 4 = tty_nr, 5 = tpgid,
	//   6 = flags, 7 = minflt, 8 = cminflt, 9 = majflt, 10 = cmajflt,
	//   11 = utime, 12 = stime, ..., 39 = delayacct_blkio_ticks (field 42).
	if len(suffix) < 13 {
		return stat, fmt.Errorf("%s bad content: not enough fields in '%s'", statfile, line)
	}

	utime, err := strconv.ParseFloat(suffix[11], 64)
	if err != nil {
		return stat, fmt.Errorf("%s bad content: parse utime: %w", statfile, err)
	}
	stime, err := strconv.ParseFloat(suffix[12], 64)
	if err != nil {
		return stat, fmt.Errorf("%s bad content: parse stime: %w", statfile, err)
	}

	stat.Utime = utime
	stat.Stime = stime

	// delayacct_blkio_ticks lives at suffix[39] (field 42). Older kernels or
	// truncated proc files may not include it — return what we have without an
	// error so callers degrade gracefully (IODelay stays at 0).
	if len(suffix) >= 40 {
		iodelay, err := strconv.ParseFloat(suffix[39], 64)
		if err != nil {
			return stat, fmt.Errorf("%s bad content: parse delayacct_blkio_ticks: %w", statfile, err)
		}
		stat.IODelay = iodelay
	}

	return stat, nil
}

// ReadProcPidIO reads /proc/<pid>/io and returns ProcPidIO.
func ReadProcPidIO(pid int) (ProcPidIO, error) {
	return readProcPidIOFile(fmt.Sprintf("/proc/%d/io", pid))
}

// readProcPidIOFile reads an io file from the given path and parses read_bytes/write_bytes.
// It is split out so unit tests can exercise the parser with golden files.
func readProcPidIOFile(iofile string) (ProcPidIO, error) {
	var stat ProcPidIO

	f, err := os.Open(filepath.Clean(iofile))
	if err != nil {
		return stat, err
	}
	defer func() {
		_ = f.Close()
	}()

	var found int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}

		switch key {
		case "read_bytes":
			v, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return ProcPidIO{}, fmt.Errorf("%s bad content: parse read_bytes: %w", iofile, err)
			}
			stat.ReadBytes = v
			found++
		case "write_bytes":
			v, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return ProcPidIO{}, fmt.Errorf("%s bad content: parse write_bytes: %w", iofile, err)
			}
			stat.WriteBytes = v
			found++
		}
	}
	if err := scanner.Err(); err != nil {
		return ProcPidIO{}, fmt.Errorf("%s bad content: %w", iofile, err)
	}
	if found != 2 {
		return ProcPidIO{}, fmt.Errorf("%s bad content: missing read_bytes or write_bytes", iofile)
	}

	return stat, nil
}

// CheckIOAvailable probes /proc/[pid]/io to verify cross-process IO accounting
// is readable. The caller should supply a PID that belongs to a different user
// (e.g. a PostgreSQL backend) — /proc/self/io is always accessible to the owner
// process and therefore not a useful probe.
// Returns nil on success, EACCES (or another OS error) on failure.
func CheckIOAvailable(pid int) error {
	f, err := os.Open(filepath.Clean(fmt.Sprintf("/proc/%d/io", pid)))
	if err != nil {
		return err
	}
	return f.Close()
}

// CheckDelayAcctAvailable reports whether delay accounting is active at runtime.
// It reads /proc/sys/kernel/task_delayacct; returns false if the file is absent
// (CONFIG_TASK_DELAY_ACCT=n or kernel < 2.6.18) or contains "0". The read is
// bounded to 4 bytes — sufficient for "0\n" or "1\n" — to avoid unbounded reads
// on a procfs virtual file.
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

// formatCPUTime converts accumulated CPU jiffies to a HH:MM:SS string. Hours
// are not capped — values >= 100h render with extra digits, matching the
// behaviour of ps(1). ticks is CLK_TCK obtained at startup; callers must pass
// a positive value.
func formatCPUTime(jiffies, ticks float64) string {
	secs := int64(jiffies / ticks)
	return fmt.Sprintf("%02d:%02d:%02d", secs/3600, (secs%3600)/60, secs%60)
}

// BuildProcPidResult joins a 7-column pg_stat_activity PGresult with prev/curr
// procfs snapshots (CPU + IO) and produces the 19-column PGresult consumed by
// the rendering pipeline. The function never returns fewer than 19 columns —
// missing data is rendered as "0" (CPU/rate) or "" (IO/iodelay). Callers pass:
//
//   - activity            — 7-column PGresult from pg_stat_activity.
//   - prevStats           — utime/stime/iodelay from the previous tick keyed by PID; may be nil.
//   - currStats           — utime/stime/iodelay from the current tick keyed by PID.
//   - prevIO/currIO       — read/write bytes; may be nil when ioAvailable=false.
//   - ioAvailable         — false on PG <17 or when /proc/[pid]/io is not readable;
//     causes IO columns to render as "" (empty string with Valid=true).
//   - delayAcctAvailable  — false when /proc/sys/kernel/task_delayacct is absent
//     or set to "0"; causes iodelay columns to render as "".
//   - ticks               — CLK_TCK from sysconf.
//   - itv                 — refresh interval in seconds; 0 skips rate columns.
//   - cpuCount            — number of CPUs used to normalize %all/%us/%sy.
//
// BuildProcPidResult is a composition: it assembles raw numeric values via
// buildProcPidResultRaw (cols 6-11 hold float strings, cols 12-17 hold
// already-display rate strings), then formats raw cols 6-11 for display via
// formatProcPidResultForDisplay. The split keeps recorder-side raw assembly
// reusable while preserving the established display contract.
func BuildProcPidResult(
	activity PGresult,
	prevStats, currStats map[int]ProcPidStat,
	prevIO, currIO map[int]ProcPidIO,
	ioAvailable bool,
	delayAcctAvailable bool,
	ticks float64,
	itv float64,
	cpuCount int,
) PGresult {
	raw := buildProcPidResultRaw(
		activity,
		prevStats, currStats,
		prevIO, currIO,
		ioAvailable, delayAcctAvailable,
		ticks, itv, cpuCount,
	)
	return formatProcPidResultForDisplay(raw, ticks)
}

// buildProcPidResultRaw is the model stage of the BuildProcPidResult pipeline.
// It produces the same 19-column PGresult as BuildProcPidResult except that
// cols 6-11 hold *raw* numeric values as float strings instead of
// display-formatted ones:
//
//   - cols 6-8  — accumulated CPU jiffies (utime+stime, utime, stime) as
//     decimal float strings; "0" sentinel when the PID is invalid.
//   - cols 9-10 — accumulated IO bytes (read, write) as decimal float strings;
//     "" sentinel when ioAvailable=false or the PID is invalid.
//   - col 11    — accumulated iodelay ticks as a decimal float string; "0"
//     sentinel when the PID is invalid; "" sentinel when delay accounting
//     is unavailable.
//
// Cols 12-17 are computed as today (delta rates) and already display-ready;
// formatProcPidResultForDisplay passes them through unchanged. SQL-derived
// cols 0-5 and col 18 (query) are copied verbatim.
func buildProcPidResultRaw(
	activity PGresult,
	prevStats, currStats map[int]ProcPidStat,
	prevIO, currIO map[int]ProcPidIO,
	ioAvailable bool,
	delayAcctAvailable bool,
	ticks float64,
	itv float64,
	cpuCount int,
) PGresult {
	values := make([][]sql.NullString, 0, activity.Nrows)

	for _, src := range activity.Values {
		row := make([]sql.NullString, procPidResultNcols)

		// Cols 0..5 — verbatim copy of SQL columns.
		for i := 0; i < 6; i++ {
			var s string
			if i < len(src) {
				s = src[i].String
			}
			row[i] = sql.NullString{String: s, Valid: true}
		}

		// Parse PID; on failure or non-positive value, render procfs columns
		// as "0"/"" but still keep the row so the SQL columns are visible.
		// Guard against a short activity row that lacks the pid column.
		var pidStr string
		if len(src) > 0 {
			pidStr = strings.TrimSpace(src[0].String)
		}
		pid, perr := strconv.Atoi(pidStr)
		validPID := perr == nil && pid > 0

		// Default procfs cell values.
		curCPU, prevCPU, haveCPU, havePrevCPU := ProcPidStat{}, ProcPidStat{}, false, false
		curIOs, prevIOs, haveIO, havePrevIO := ProcPidIO{}, ProcPidIO{}, false, false
		if validPID {
			curCPU, haveCPU = currStats[pid]
			prevCPU, havePrevCPU = prevStats[pid]
			if ioAvailable {
				curIOs, haveIO = currIO[pid]
				prevIOs, havePrevIO = prevIO[pid]
			}
		}

		// Cols 6..8 — accumulated CPU jiffies as float strings.
		// "0" sentinel when the PID is invalid (no procfs data).
		if validPID {
			row[6] = nullString(strconv.FormatFloat(curCPU.Utime+curCPU.Stime, 'f', 6, 64))
			row[7] = nullString(strconv.FormatFloat(curCPU.Utime, 'f', 6, 64))
			row[8] = nullString(strconv.FormatFloat(curCPU.Stime, 'f', 6, 64))
		} else {
			row[6] = nullString("0")
			row[7] = nullString("0")
			row[8] = nullString("0")
		}

		// Cols 9..10 — accumulated IO bytes as float strings.
		// "" sentinel when ioAvailable=false or PID invalid.
		if ioAvailable && validPID {
			row[9] = nullString(strconv.FormatFloat(curIOs.ReadBytes, 'f', 6, 64))
			row[10] = nullString(strconv.FormatFloat(curIOs.WriteBytes, 'f', 6, 64))
		} else {
			row[9] = nullString("")
			row[10] = nullString("")
		}

		// Col 11 — accumulated iodelay ticks as a float string.
		// "" sentinel when delay accounting is unavailable; "0" sentinel when
		// PID is invalid. The format stage maps "0" → "00:00:00" and the
		// float-string path through formatCPUTime.
		switch {
		case !delayAcctAvailable:
			row[11] = nullString("")
		case !validPID:
			row[11] = nullString("0")
		default:
			row[11] = nullString(strconv.FormatFloat(curCPU.IODelay, 'f', 6, 64))
		}

		// Cols 12..14 — CPU rate %all, %us, %sy. "0" on first tick / itv==0 / invalid PID.
		// Formula: Δjiffies / (itv * ticks) * 100 / cpuCount.
		if validPID && haveCPU && havePrevCPU && itv > 0 && ticks > 0 && cpuCount > 0 {
			denom := itv * ticks
			scale := 100.0 / float64(cpuCount)
			dUtime := delta(prevCPU.Utime, curCPU.Utime)
			dStime := delta(prevCPU.Stime, curCPU.Stime)
			row[12] = nullString(strconv.FormatFloat((dUtime+dStime)/denom*scale, 'f', 2, 64))
			row[13] = nullString(strconv.FormatFloat(dUtime/denom*scale, 'f', 2, 64))
			row[14] = nullString(strconv.FormatFloat(dStime/denom*scale, 'f', 2, 64))
		} else {
			row[12] = nullString("0")
			row[13] = nullString("0")
			row[14] = nullString("0")
		}

		// Cols 15..16 — IO rate read,KiB/s, write,KiB/s. "" if !ioAvailable, "0.00" if no prev / itv==0.
		switch {
		case !ioAvailable || !validPID:
			row[15] = nullString("")
			row[16] = nullString("")
		case haveIO && havePrevIO && itv > 0:
			dRead := delta(prevIOs.ReadBytes, curIOs.ReadBytes)
			dWrite := delta(prevIOs.WriteBytes, curIOs.WriteBytes)
			row[15] = nullString(strconv.FormatFloat(dRead/itv/1024, 'f', 2, 64))
			row[16] = nullString(strconv.FormatFloat(dWrite/itv/1024, 'f', 2, 64))
		default:
			row[15] = nullString("0.00")
			row[16] = nullString("0.00")
		}

		// Col 17 — %iodelay rate. "" when delay accounting unavailable or first
		// tick / itv<=0 / ticks<=0; "0.00" on invalid PID. Not normalised by
		// cpuCount: delayacct_blkio_ticks is wall-clock time spent blocked, not
		// per-CPU time (see tech-spec Decision 3).
		switch {
		case !delayAcctAvailable:
			row[17] = nullString("")
		case !validPID:
			row[17] = nullString("0.00")
		case haveCPU && havePrevCPU && itv > 0 && ticks > 0:
			dIO := delta(prevCPU.IODelay, curCPU.IODelay)
			row[17] = nullString(strconv.FormatFloat(dIO/(itv*ticks)*100, 'f', 2, 64))
		default:
			row[17] = nullString("")
		}

		// Col 18 — query (last column of activity, index 6).
		var q string
		if len(src) > 6 {
			q = src[6].String
		}
		row[18] = sql.NullString{String: q, Valid: true}

		values = append(values, row)
	}

	cols := make([]string, procPidResultNcols)
	copy(cols, procPidResultCols)

	return PGresult{
		Valid:  true,
		Ncols:  procPidResultNcols,
		Nrows:  activity.Nrows,
		Cols:   cols,
		Values: values,
	}
}

// formatProcPidResultForDisplay is the view stage of the BuildProcPidResult
// pipeline. It converts raw cols 6-11 into display strings:
//
//   - cols 6-8  — raw jiffies float string → HH:MM:SS via formatCPUTime.
//     The "0" sentinel (invalid PID) is passed through unchanged so the
//     display row distinguishes "no data" from "00:00:00".
//   - cols 9-10 — raw bytes float string → integer KiB string (raw/1024).
//     The "" sentinel (IO unavailable / invalid PID) is passed through.
//   - col 11    — raw iodelay ticks float string → HH:MM:SS via formatCPUTime
//     when ticks > 0. The "" sentinel (delay accounting unavailable) is
//     passed through. When ticks <= 0 (defensive — getSysticksLocal always
//     yields >0 in production) the format falls back to "0:00:00".
//
// All other cols (0-5, 12-17, 18) are passed through unchanged: cols 12-17
// are already display-ready rate strings produced by buildProcPidResultRaw.
//
// Invariant for callers: cells in cols 6-11 of a raw PGresult are either the
// documented sentinel ("0" for cols 6-8/11, "" for cols 9-11) or a valid
// strconv.ParseFloat-parseable float string. The formatXxxCell helpers below
// pass through any other value unchanged — silent passthrough is intentional
// for sentinel safety; an unexpected value indicates a bug in the raw stage,
// not a runtime condition to format.
func formatProcPidResultForDisplay(raw PGresult, ticks float64) PGresult {
	values := make([][]sql.NullString, 0, raw.Nrows)

	for _, src := range raw.Values {
		row := make([]sql.NullString, procPidResultNcols)
		// Cols 0..5: pass-through SQL labels.
		for i := 0; i < 6; i++ {
			if i < len(src) {
				row[i] = src[i]
			} else {
				row[i] = nullString("")
			}
		}

		// Cols 6..8 — raw jiffies → HH:MM:SS.
		for _, idx := range []int{6, 7, 8} {
			row[idx] = formatJiffiesCell(src[idx], ticks)
		}

		// Cols 9..10 — raw bytes → KiB integer string.
		row[9] = formatBytesCell(src[9])
		row[10] = formatBytesCell(src[10])

		// Col 11 — raw iodelay ticks → HH:MM:SS.
		row[11] = formatIODelayCell(src[11], ticks)

		// Cols 12..17 — pass-through rate strings (already display-ready).
		for i := 12; i <= 17; i++ {
			if i < len(src) {
				row[i] = src[i]
			} else {
				row[i] = nullString("")
			}
		}

		// Col 18 — pass-through query text.
		if len(src) > 18 {
			row[18] = src[18]
		} else {
			row[18] = nullString("")
		}

		values = append(values, row)
	}

	cols := make([]string, procPidResultNcols)
	copy(cols, procPidResultCols)

	return PGresult{
		Valid:  true,
		Ncols:  procPidResultNcols,
		Nrows:  raw.Nrows,
		Cols:   cols,
		Values: values,
	}
}

// formatJiffiesCell converts a raw CPU-jiffies float string to HH:MM:SS.
// The "0" sentinel (used by buildProcPidResultRaw to mark an invalid PID)
// is passed through unchanged. Parse failures are also passed through to
// avoid silently corrupting unexpected upstream values.
func formatJiffiesCell(cell sql.NullString, ticks float64) sql.NullString {
	if cell.String == "0" {
		return cell
	}
	v, err := strconv.ParseFloat(cell.String, 64)
	if err != nil {
		return cell
	}
	return nullString(formatCPUTime(v, ticks))
}

// formatBytesCell converts a raw IO-bytes float string to an integer KiB
// string. Empty-string ("" sentinel: IO unavailable / invalid PID) is passed
// through. Parse failures are passed through unchanged for the same reason as
// formatJiffiesCell.
func formatBytesCell(cell sql.NullString) sql.NullString {
	if cell.String == "" {
		return cell
	}
	v, err := strconv.ParseFloat(cell.String, 64)
	if err != nil {
		return cell
	}
	return nullString(strconv.FormatFloat(v/1024, 'f', 0, 64))
}

// formatIODelayCell converts a raw iodelay-ticks float string to HH:MM:SS.
// Empty-string ("" sentinel: delay accounting unavailable) passes through.
// ticks <= 0 falls back to "0:00:00" matching the pre-split behaviour for the
// defensive divide-by-zero guard.
func formatIODelayCell(cell sql.NullString, ticks float64) sql.NullString {
	if cell.String == "" {
		return cell
	}
	v, err := strconv.ParseFloat(cell.String, 64)
	if err != nil {
		return cell
	}
	if ticks <= 0 {
		return nullString("0:00:00")
	}
	return nullString(formatCPUTime(v, ticks))
}

// nullString wraps s in a Valid sql.NullString.
func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: true}
}

// delta returns curr-prev, or 0 if curr <= prev (counters can reset on backend exit).
func delta(prev, curr float64) float64 {
	if curr > prev {
		return curr - prev
	}
	return 0
}
