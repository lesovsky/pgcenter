package stat

import (
	"database/sql"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadProcPidStatSpaceInComm(t *testing.T) {
	got, err := readProcPidStatFile("testdata/proc/pid_stat_space_comm")
	assert.NoError(t, err)
	assert.Equal(t, float64(1500), got.Utime)
	assert.Equal(t, float64(750), got.Stime)
}

func TestReadProcPidStatNormalComm(t *testing.T) {
	got, err := readProcPidStatFile("testdata/proc/pid_stat_normal_comm")
	assert.NoError(t, err)
	assert.Equal(t, float64(2500), got.Utime)
	assert.Equal(t, float64(1250), got.Stime)
}

func TestReadProcPidStatMalformed(t *testing.T) {
	got, err := readProcPidStatFile("testdata/proc/pid_stat_malformed")
	assert.Error(t, err)
	assert.Equal(t, ProcPidStat{}, got)
}

func TestReadProcPidIOValid(t *testing.T) {
	got, err := readProcPidIOFile("testdata/proc/pid_io_valid")
	assert.NoError(t, err)
	assert.Equal(t, float64(4096), got.ReadBytes)
	assert.Equal(t, float64(8192), got.WriteBytes)
}

func TestReadProcPidIOMissingKey(t *testing.T) {
	got, err := readProcPidIOFile("testdata/proc/pid_io_missing_key")
	assert.Error(t, err)
	assert.Equal(t, ProcPidIO{}, got)
}

func TestReadProcPidStatIntegration(t *testing.T) {
	got, err := ReadProcPidStat(os.Getpid())
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, got.Utime+got.Stime, float64(0))
}

func TestReadProcPidIOIntegration(t *testing.T) {
	got, err := ReadProcPidIO(os.Getpid())
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, got.ReadBytes+got.WriteBytes, float64(0))
}

func TestCheckIOAvailable(t *testing.T) {
	// CheckIOAvailable probes /proc/[pid]/io for a cross-process PID. Both nil
	// (access granted) and EACCES (permission denied) are valid outcomes depending
	// on the user running the test. We only verify no panic and a sensible error type.
	err := CheckIOAvailable(1) // PID 1 always exists and belongs to root
	if err != nil {
		assert.True(t, os.IsPermission(err), "expected permission error, got: %v", err)
	}
}

func TestReadProcPidStatFileMissing(t *testing.T) {
	_, err := readProcPidStatFile("testdata/proc/does_not_exist")
	assert.Error(t, err)
}

func TestReadProcPidIOFileMissing(t *testing.T) {
	_, err := readProcPidIOFile("testdata/proc/does_not_exist")
	assert.Error(t, err)
}

// expectedProcPidCols is the canonical 19-column header for BuildProcPidResult output.
var expectedProcPidCols = []string{
	"pid", "datname", "usename", "state", "wait_etype", "wait_event",
	"all_total,s", "us_total,s", "sy_total,s",
	"read_total,KiB", "write_total,KiB",
	"iodelay_total,s",
	"%all", "%us", "%sy",
	"read,KiB/s", "write,KiB/s",
	"%iodelay",
	"query",
}

// newTestActivityResult returns a 7-column PGresult mimicking the simplified
// pg_stat_activity output produced by task 02.
func newTestActivityResult(rows [][]string) PGresult {
	values := make([][]sql.NullString, 0, len(rows))
	for _, r := range rows {
		row := make([]sql.NullString, len(r))
		for i, v := range r {
			row[i] = sql.NullString{String: v, Valid: true}
		}
		values = append(values, row)
	}
	return PGresult{
		Valid: true,
		Ncols: 7,
		Nrows: len(rows),
		Cols:  []string{"pid", "datname", "usename", "state", "wait_etype", "wait_event", "query"},
		Values: values,
	}
}

func TestFormatCPUTime(t *testing.T) {
	tests := []struct {
		name    string
		jiffies float64
		ticks   float64
		want    string
	}{
		{"zero", 0, 100, "00:00:00"},
		{"one hour", 360000, 100, "01:00:00"},
		{"one minute", 6000, 100, "00:01:00"},
		{"one second", 100, 100, "00:00:01"},
		{"mixed hh:mm:ss", 366100, 100, "01:01:01"},
		{"100h overflow", 36006000, 100, "100:01:00"}, // 36006000/100 = 360060s = 100h 1m 0s
		{"fractional jiffies floor", 199, 100, "00:00:01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatCPUTime(tt.jiffies, tt.ticks))
		})
	}
}

func TestBuildProcPidResult_FirstTick(t *testing.T) {
	activity := newTestActivityResult([][]string{
		{"100", "postgres", "alice", "active", "Lock", "transactionid", "SELECT 1"},
	})
	currStats := map[int]ProcPidStat{
		100: {Utime: 100, Stime: 50},
	}
	currIO := map[int]ProcPidIO{
		100: {ReadBytes: 4096, WriteBytes: 8192},
	}

	got := BuildProcPidResult(activity, nil, currStats, nil, currIO, true, false, 100, 1, 4)

	assert.True(t, got.Valid)
	assert.Equal(t, 19, got.Ncols)
	assert.Equal(t, 1, got.Nrows)
	assert.Equal(t, expectedProcPidCols, got.Cols)
	assert.Len(t, got.Values, 1)
	assert.Len(t, got.Values[0], 19)

	row := got.Values[0]
	assert.Equal(t, "100", row[0].String)
	assert.Equal(t, "postgres", row[1].String)
	assert.Equal(t, "alice", row[2].String)
	assert.Equal(t, "active", row[3].String)
	assert.Equal(t, "Lock", row[4].String)
	assert.Equal(t, "transactionid", row[5].String)
	// accumulated CPU columns computed from curr.
	assert.Equal(t, "00:00:01", row[6].String) // 150/100=1s
	assert.Equal(t, "00:00:01", row[7].String) // 100/100=1s
	assert.Equal(t, "00:00:00", row[8].String) // 50/100=0s
	// IO totals computed from curr.
	assert.Equal(t, "4", row[9].String)
	assert.Equal(t, "8", row[10].String)
	// iodelay_total,s — delayAcctAvailable=false → "".
	assert.Equal(t, "", row[11].String)
	// Rate columns are "0" / "" on first tick.
	assert.Equal(t, "0", row[12].String)
	assert.Equal(t, "0", row[13].String)
	assert.Equal(t, "0", row[14].String)
	assert.Equal(t, "0.00", row[15].String)
	assert.Equal(t, "0.00", row[16].String)
	// %iodelay — delayAcctAvailable=false → "".
	assert.Equal(t, "", row[17].String)
	assert.Equal(t, "SELECT 1", row[18].String)
}

func TestBuildProcPidResult_IOUnavailable(t *testing.T) {
	activity := newTestActivityResult([][]string{
		{"200", "db", "bob", "idle", "", "", "SELECT 2"},
	})
	currStats := map[int]ProcPidStat{
		200: {Utime: 200, Stime: 100},
	}

	got := BuildProcPidResult(activity, nil, currStats, nil, nil, false, false, 100, 1, 4)

	assert.Equal(t, 19, got.Ncols)
	assert.Len(t, got.Values[0], 19)
	row := got.Values[0]

	// IO columns are empty strings.
	assert.Equal(t, "", row[9].String)
	assert.Equal(t, "", row[10].String)
	assert.Equal(t, "", row[15].String)
	assert.Equal(t, "", row[16].String)
	// NullString values are still Valid=true even when string is empty.
	assert.True(t, row[9].Valid)
	assert.True(t, row[15].Valid)

	// CPU columns are populated normally.
	assert.Equal(t, "00:00:03", row[6].String) // 300/100=3s
	assert.Equal(t, "00:00:02", row[7].String) // 200/100=2s
	assert.Equal(t, "00:00:01", row[8].String) // 100/100=1s
}

func TestBuildProcPidResult_ItvZero(t *testing.T) {
	activity := newTestActivityResult([][]string{
		{"300", "db", "carol", "active", "", "", "SELECT 3"},
	})
	prevStats := map[int]ProcPidStat{
		300: {Utime: 100, Stime: 50},
	}
	currStats := map[int]ProcPidStat{
		300: {Utime: 200, Stime: 100},
	}
	prevIO := map[int]ProcPidIO{
		300: {ReadBytes: 1024, WriteBytes: 2048},
	}
	currIO := map[int]ProcPidIO{
		300: {ReadBytes: 4096, WriteBytes: 8192},
	}

	// itv=0 must NOT panic and must yield "0" / "0.00" rate columns.
	got := BuildProcPidResult(activity, prevStats, currStats, prevIO, currIO, true, false, 100, 0, 4)

	assert.Equal(t, 19, got.Ncols)
	row := got.Values[0]
	// iodelay_total,s — delayAcctAvailable=false → "".
	assert.Equal(t, "", row[11].String)
	assert.Equal(t, "0", row[12].String)
	assert.Equal(t, "0", row[13].String)
	assert.Equal(t, "0", row[14].String)
	assert.Equal(t, "0.00", row[15].String)
	assert.Equal(t, "0.00", row[16].String)
	// %iodelay — delayAcctAvailable=false → "".
	assert.Equal(t, "", row[17].String)
}

func TestBuildProcPidResult_NcolsGuarantee(t *testing.T) {
	tests := []struct {
		name        string
		activity    PGresult
		prevStats   map[int]ProcPidStat
		currStats   map[int]ProcPidStat
		prevIO      map[int]ProcPidIO
		currIO      map[int]ProcPidIO
		ioAvailable bool
		itv         float64
	}{
		{
			name:        "zero rows",
			activity:    newTestActivityResult(nil),
			ioAvailable: true,
			itv:         1,
		},
		{
			name:        "io unavailable",
			activity:    newTestActivityResult([][]string{{"1", "d", "u", "a", "", "", "q"}}),
			currStats:   map[int]ProcPidStat{1: {Utime: 1, Stime: 1}},
			ioAvailable: false,
			itv:         1,
		},
		{
			name:        "first tick",
			activity:    newTestActivityResult([][]string{{"1", "d", "u", "a", "", "", "q"}}),
			currStats:   map[int]ProcPidStat{1: {Utime: 1, Stime: 1}},
			currIO:      map[int]ProcPidIO{1: {ReadBytes: 1, WriteBytes: 1}},
			ioAvailable: true,
			itv:         1,
		},
		{
			name:     "normal tick",
			activity: newTestActivityResult([][]string{{"1", "d", "u", "a", "", "", "q"}}),
			prevStats: map[int]ProcPidStat{1: {Utime: 1, Stime: 1}},
			currStats: map[int]ProcPidStat{1: {Utime: 10, Stime: 5}},
			prevIO:   map[int]ProcPidIO{1: {ReadBytes: 1024, WriteBytes: 1024}},
			currIO:   map[int]ProcPidIO{1: {ReadBytes: 2048, WriteBytes: 4096}},
			ioAvailable: true,
			itv:        1,
		},
		{
			name:     "invalid pid",
			activity: newTestActivityResult([][]string{{"abc", "d", "u", "a", "", "", "q"}}),
			ioAvailable: true,
			itv:        1,
		},
		{
			name:     "negative pid",
			activity: newTestActivityResult([][]string{{"-1", "d", "u", "a", "", "", "q"}}),
			ioAvailable: true,
			itv:        1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildProcPidResult(tt.activity, tt.prevStats, tt.currStats, tt.prevIO, tt.currIO, tt.ioAvailable, false, 100, tt.itv, 4)
			assert.Equal(t, 19, got.Ncols)
			assert.Equal(t, tt.activity.Nrows, got.Nrows)
			assert.Len(t, got.Cols, 19)
			for i, row := range got.Values {
				assert.Lenf(t, row, 19, "row %d has wrong width", i)
			}
		})
	}
}

func TestBuildProcPidResult_TwoTicks(t *testing.T) {
	activity := newTestActivityResult([][]string{
		{"400", "postgres", "dave", "active", "", "", "SELECT 4"},
	})
	prevStats := map[int]ProcPidStat{
		400: {Utime: 100, Stime: 50},
	}
	currStats := map[int]ProcPidStat{
		400: {Utime: 200, Stime: 100}, // delta utime=100, delta stime=50
	}
	prevIO := map[int]ProcPidIO{
		400: {ReadBytes: 0, WriteBytes: 0},
	}
	currIO := map[int]ProcPidIO{
		400: {ReadBytes: 10240, WriteBytes: 20480}, // delta read=10240, delta write=20480
	}

	// ticks=100, itv=1s, cpuCount=4
	// %all = (100+50)/(1*100)*100/4 = 150/100*100/4 = 37.5
	// %us  = 100/(1*100)*100/4 = 25.00
	// %sy  =  50/(1*100)*100/4 = 12.50
	// read,KiB/s = 10240/1/1024 = 10.00
	// write,KiB/s = 20480/1/1024 = 20.00
	// all_total,s = (200+100)/100 = 3 sec = "00:00:03"
	got := BuildProcPidResult(activity, prevStats, currStats, prevIO, currIO, true, false, 100, 1, 4)

	assert.Equal(t, 19, got.Ncols)
	row := got.Values[0]
	assert.Equal(t, "00:00:03", row[6].String)
	assert.Equal(t, "00:00:02", row[7].String)
	assert.Equal(t, "00:00:01", row[8].String)
	assert.Equal(t, "10", row[9].String)
	assert.Equal(t, "20", row[10].String)
	// iodelay_total,s — delayAcctAvailable=false → "".
	assert.Equal(t, "", row[11].String)
	assert.Equal(t, "37.50", row[12].String)
	assert.Equal(t, "25.00", row[13].String)
	assert.Equal(t, "12.50", row[14].String)
	assert.Equal(t, "10.00", row[15].String)
	assert.Equal(t, "20.00", row[16].String)
	// %iodelay — delayAcctAvailable=false → "".
	assert.Equal(t, "", row[17].String)
	assert.Equal(t, "SELECT 4", row[18].String)
}

func TestBuildProcPidResult_InvalidPID(t *testing.T) {
	activity := newTestActivityResult([][]string{
		{"abc", "postgres", "eve", "active", "", "", "BAD"},
		{"0", "postgres", "eve", "active", "", "", "ZERO"},
		{"-7", "postgres", "eve", "active", "", "", "NEG"},
	})
	currStats := map[int]ProcPidStat{}
	currIO := map[int]ProcPidIO{}

	got := BuildProcPidResult(activity, nil, currStats, nil, currIO, true, false, 100, 1, 4)

	assert.Equal(t, 19, got.Ncols)
	assert.Equal(t, 3, got.Nrows)
	for i, row := range got.Values {
		assert.Lenf(t, row, 19, "row %d has wrong width", i)
		// SQL columns intact (verbatim from activity).
		assert.Equal(t, activity.Values[i][0].String, row[0].String)
		// Procfs CPU columns = "0" for invalid PID.
		assert.Equal(t, "0", row[6].String)
		assert.Equal(t, "0", row[7].String)
		assert.Equal(t, "0", row[8].String)
		// IO columns = "" for invalid PID.
		assert.Equal(t, "", row[9].String)
		assert.Equal(t, "", row[10].String)
		// iodelay_total,s — delayAcctAvailable=false → "".
		assert.Equal(t, "", row[11].String)
		// Rate columns = "0" / "".
		assert.Equal(t, "0", row[12].String)
		assert.Equal(t, "", row[15].String)
		// %iodelay — delayAcctAvailable=false → "".
		assert.Equal(t, "", row[17].String)
		// Query preserved.
		assert.Equal(t, activity.Values[i][6].String, row[18].String)
	}
}

// TestBuildProcPidResult_NewSignature is the TDD anchor for task 01: it pins
// the new 10-argument signature (delayAcctAvailable inserted after ioAvailable)
// and the expanded 19-column result. Detailed iodelay rendering coverage lives
// in task 02 (TestBuildProcPidResult_DelayAvailable / _DelayUnavailable).
func TestBuildProcPidResult_NewSignature(t *testing.T) {
	activity := newTestActivityResult([][]string{
		{"500", "postgres", "alice", "active", "", "", "SELECT 5"},
	})
	currStats := map[int]ProcPidStat{
		500: {Utime: 100, Stime: 50, IODelay: 200},
	}
	currIO := map[int]ProcPidIO{
		500: {ReadBytes: 1024, WriteBytes: 2048},
	}

	got := BuildProcPidResult(activity, nil, currStats, nil, currIO, true, false, 100, 1, 4)

	assert.True(t, got.Valid)
	assert.Equal(t, 19, got.Ncols)
	assert.Equal(t, 1, got.Nrows)
}

// TestReadProcPidStatIODelay reads the iodelay golden file where suffix[39] is
// set to 500 and asserts the parser surfaces the value via ProcPidStat.IODelay.
// Utime/Stime are inherited from pid_stat_normal_comm (2500 / 1250) and must
// still parse correctly after the new field was added.
func TestReadProcPidStatIODelay(t *testing.T) {
	got, err := readProcPidStatFile("testdata/proc/pid_stat_iodelay")
	assert.NoError(t, err)
	assert.Equal(t, float64(2500), got.Utime)
	assert.Equal(t, float64(1250), got.Stime)
	assert.Equal(t, float64(500), got.IODelay)
}

// TestReadProcPidStatTruncated exercises the `len(suffix) < 40` guard in
// readProcPidStatFile. The golden file has exactly 39 suffix fields — utime
// and stime are present at suffix[11]/[12], but field 42 (delayacct_blkio_ticks)
// is absent. The parser must return a valid ProcPidStat with IODelay==0 and
// no error so callers degrade gracefully on older kernels or short proc lines.
func TestReadProcPidStatTruncated(t *testing.T) {
	got, err := readProcPidStatFile("testdata/proc/pid_stat_truncated")
	assert.NoError(t, err)
	assert.Equal(t, float64(2500), got.Utime)
	assert.Equal(t, float64(1250), got.Stime)
	assert.Equal(t, float64(0), got.IODelay)
}

// TestCheckDelayAcctAvailable verifies the probe returns a bool without panic
// and matches the live /proc/sys/kernel/task_delayacct sysctl when readable.
// If the file is absent (older kernels / non-Linux test runners) the function
// must return false. Mirrors the pattern used by TestCheckIOAvailable.
func TestCheckDelayAcctAvailable(t *testing.T) {
	got := CheckDelayAcctAvailable()

	data, err := os.ReadFile("/proc/sys/kernel/task_delayacct")
	if err != nil {
		// File absent → probe must return false; both branches end here.
		assert.False(t, got, "probe must return false when sysctl is absent")
		return
	}
	want := strings.TrimSpace(string(data)) == "1"
	assert.Equal(t, want, got, "probe result must match live sysctl value")
}

// TestBuildProcPidResult_DelayAvailable covers the delayAcctAvailable=true
// path with a non-zero IODelay delta. Expected:
//   - col 11 (iodelay_total,s) = formatCPUTime(curr.IODelay, ticks) — "00:00:01" for 100/100
//   - col 17 (%iodelay) = ΔIODelay/(itv*ticks)*100 = 100/(1*100)*100 = "100.00"
//
// %iodelay is intentionally NOT divided by cpuCount (tech-spec Decision 3):
// delayacct_blkio_ticks is wall-clock time blocked, not per-CPU time.
func TestBuildProcPidResult_DelayAvailable(t *testing.T) {
	activity := newTestActivityResult([][]string{
		{"600", "postgres", "alice", "active", "", "", "SELECT 6"},
	})
	prevStats := map[int]ProcPidStat{
		600: {Utime: 100, Stime: 50, IODelay: 0},
	}
	currStats := map[int]ProcPidStat{
		600: {Utime: 200, Stime: 100, IODelay: 100},
	}

	// ticks=100, itv=1, cpuCount=4, delayAcctAvailable=true.
	got := BuildProcPidResult(activity, prevStats, currStats, nil, nil, false, true, 100, 1, 4)

	assert.Equal(t, 19, got.Ncols)
	row := got.Values[0]
	// iodelay_total,s = formatCPUTime(100, 100) = "00:00:01".
	assert.Equal(t, "00:00:01", row[11].String)
	// %iodelay = 100/(1*100)*100 = 100.00 (no cpuCount division).
	assert.Equal(t, "100.00", row[17].String)
}

// TestBuildProcPidResult_DelayUnavailable covers the delayAcctAvailable=false
// path. Both iodelay columns must render as "" (empty string with Valid=true)
// regardless of what currStats contains.
func TestBuildProcPidResult_DelayUnavailable(t *testing.T) {
	activity := newTestActivityResult([][]string{
		{"700", "postgres", "alice", "active", "", "", "SELECT 7"},
	})
	prevStats := map[int]ProcPidStat{
		700: {Utime: 100, Stime: 50, IODelay: 0},
	}
	currStats := map[int]ProcPidStat{
		700: {Utime: 200, Stime: 100, IODelay: 100},
	}

	got := BuildProcPidResult(activity, prevStats, currStats, nil, nil, false, false, 100, 1, 4)

	assert.Equal(t, 19, got.Ncols)
	row := got.Values[0]
	assert.Equal(t, "", row[11].String)
	assert.True(t, row[11].Valid)
	assert.Equal(t, "", row[17].String)
	assert.True(t, row[17].Valid)
}

// TestBuildProcPidResultRaw verifies that the private raw-stage builder
// produces a 19-col PGresult where cols 6-8 contain raw jiffies as float
// strings (no HH:MM:SS — no ":" separator), cols 9-10 contain raw bytes as
// float strings (not KiB-divided), col 11 contains raw iodelay ticks as a
// float string, and the SQL-derived cols (0-5, 18) are unchanged. Cols 12-17
// are already display-ready rate strings produced inside the raw stage —
// they pass through formatProcPidResultForDisplay unchanged.
func TestBuildProcPidResultRaw(t *testing.T) {
	activity := newTestActivityResult([][]string{
		{"800", "postgres", "alice", "active", "Lock", "transactionid", "SELECT 8"},
	})
	prevStats := map[int]ProcPidStat{
		800: {Utime: 100, Stime: 50, IODelay: 0},
	}
	currStats := map[int]ProcPidStat{
		800: {Utime: 200, Stime: 100, IODelay: 100},
	}
	prevIO := map[int]ProcPidIO{
		800: {ReadBytes: 0, WriteBytes: 0},
	}
	currIO := map[int]ProcPidIO{
		800: {ReadBytes: 10240, WriteBytes: 20480},
	}

	raw := buildProcPidResultRaw(activity, prevStats, currStats, prevIO, currIO, true, true, 100, 1, 4)

	assert.True(t, raw.Valid)
	assert.Equal(t, 19, raw.Ncols)
	assert.Equal(t, 1, raw.Nrows)
	row := raw.Values[0]
	assert.Len(t, row, 19)

	// Cols 0-5: verbatim SQL labels.
	assert.Equal(t, "800", row[0].String)
	assert.Equal(t, "postgres", row[1].String)
	assert.Equal(t, "alice", row[2].String)
	assert.Equal(t, "active", row[3].String)
	assert.Equal(t, "Lock", row[4].String)
	assert.Equal(t, "transactionid", row[5].String)

	// Cols 6-8: raw float strings, NO ":" separator, and must round-trip
	// back to the source jiffies (Utime+Stime=150, Utime=200, Stime=100…
	// wait: prevStats had Utime=100/Stime=50, curr has Utime=200/Stime=100,
	// so raw cols hold curr — 300, 200, 100).
	assert.NotContains(t, row[6].String, ":", "raw col 6 must not be HH:MM:SS")
	assert.NotContains(t, row[7].String, ":", "raw col 7 must not be HH:MM:SS")
	assert.NotContains(t, row[8].String, ":", "raw col 8 must not be HH:MM:SS")
	parsed6, err := strconv.ParseFloat(row[6].String, 64)
	assert.NoError(t, err)
	assert.Equal(t, float64(300), parsed6, "raw col 6 must equal curr.Utime+curr.Stime")
	parsed7, err := strconv.ParseFloat(row[7].String, 64)
	assert.NoError(t, err)
	assert.Equal(t, float64(200), parsed7, "raw col 7 must equal curr.Utime")
	parsed8, err := strconv.ParseFloat(row[8].String, 64)
	assert.NoError(t, err)
	assert.Equal(t, float64(100), parsed8, "raw col 8 must equal curr.Stime")

	// Cols 9-10: raw bytes as float strings (NOT KiB-divided).
	// 10240 bytes raw, not 10 KiB.
	read, err := strconv.ParseFloat(row[9].String, 64)
	assert.NoError(t, err)
	assert.Equal(t, float64(10240), read, "raw col 9 must contain bytes, not KiB")
	write, err := strconv.ParseFloat(row[10].String, 64)
	assert.NoError(t, err)
	assert.Equal(t, float64(20480), write, "raw col 10 must contain bytes, not KiB")

	// Col 11: raw iodelay ticks as float string, NO ":" separator.
	assert.NotContains(t, row[11].String, ":", "raw col 11 must not be HH:MM:SS")
	iod, err := strconv.ParseFloat(row[11].String, 64)
	assert.NoError(t, err)
	assert.Equal(t, float64(100), iod, "raw col 11 must contain raw iodelay ticks")

	// Col 18: query text (unchanged).
	assert.Equal(t, "SELECT 8", row[18].String)
}

// TestBuildProcPidResultRaw_InvalidPID pins the sentinel contract of the raw
// stage: invalid PID rows must produce "0" for cols 6-8 (CPU), "" for cols
// 9-10 (IO), and the delayAcct-aware sentinel for col 11. Without this
// directly-on-raw assertion, regressions to the sentinel scheme can hide
// behind the format-stage composition and only surface as confusing display
// glitches downstream.
func TestBuildProcPidResultRaw_InvalidPID(t *testing.T) {
	activity := newTestActivityResult([][]string{
		{"abc", "postgres", "alice", "active", "", "", "BAD"},
		{"0", "postgres", "alice", "active", "", "", "ZERO"},
		{"-7", "postgres", "alice", "active", "", "", "NEG"},
	})

	// With delayAcctAvailable=false → col 11 must be "" sentinel.
	rawNoDelay := buildProcPidResultRaw(activity, nil, map[int]ProcPidStat{}, nil, map[int]ProcPidIO{}, true, false, 100, 1, 4)
	assert.Equal(t, 3, rawNoDelay.Nrows)
	for i, row := range rawNoDelay.Values {
		assert.Equalf(t, "0", row[6].String, "row %d col 6 must be '0' sentinel", i)
		assert.Equalf(t, "0", row[7].String, "row %d col 7 must be '0' sentinel", i)
		assert.Equalf(t, "0", row[8].String, "row %d col 8 must be '0' sentinel", i)
		assert.Equalf(t, "", row[9].String, "row %d col 9 must be '' sentinel", i)
		assert.Equalf(t, "", row[10].String, "row %d col 10 must be '' sentinel", i)
		assert.Equalf(t, "", row[11].String, "row %d col 11 must be '' sentinel (delayAcct=false)", i)
	}

	// With delayAcctAvailable=true → col 11 must be "0" sentinel (raw stage),
	// which the format stage converts to "00:00:00".
	rawWithDelay := buildProcPidResultRaw(activity, nil, map[int]ProcPidStat{}, nil, map[int]ProcPidIO{}, true, true, 100, 1, 4)
	for i, row := range rawWithDelay.Values {
		assert.Equalf(t, "0", row[11].String, "row %d col 11 must be '0' sentinel (delayAcct=true, invalid PID)", i)
	}
}

// TestFormatProcPidResultForDisplay verifies that the format stage converts a
// known raw PGresult into the display PGresult: cols 6-8 become HH:MM:SS
// strings, cols 9-10 become KiB integers (raw bytes / 1024), col 11 becomes
// HH:MM:SS, cols 12-17 (rate strings already produced by raw stage) pass
// through unchanged, and col 18 is preserved.
func TestFormatProcPidResultForDisplay(t *testing.T) {
	// Build a synthetic raw 19-col PGresult by hand to isolate the format
	// step from the raw builder.
	raw := PGresult{
		Valid: true,
		Ncols: procPidResultNcols,
		Nrows: 1,
		Cols:  append([]string(nil), procPidResultCols...),
		Values: [][]sql.NullString{{
			nullString("900"), nullString("postgres"), nullString("alice"),
			nullString("active"), nullString(""), nullString(""),
			nullString("150"),    // col 6: utime+stime jiffies
			nullString("100"),    // col 7: utime jiffies
			nullString("50"),     // col 8: stime jiffies
			nullString("10240"),  // col 9: read bytes
			nullString("20480"),  // col 10: write bytes
			nullString("100"),    // col 11: iodelay ticks
			nullString("37.50"),  // col 12: %all (pass-through)
			nullString("25.00"),  // col 13: %us
			nullString("12.50"),  // col 14: %sy
			nullString("10.00"),  // col 15: read,KiB/s
			nullString("20.00"),  // col 16: write,KiB/s
			nullString("100.00"), // col 17: %iodelay
			nullString("SELECT 9"),
		}},
	}

	got := formatProcPidResultForDisplay(raw, 100)

	assert.True(t, got.Valid)
	assert.Equal(t, 19, got.Ncols)
	assert.Equal(t, 1, got.Nrows)
	row := got.Values[0]

	// Cols 0-5: unchanged.
	assert.Equal(t, "900", row[0].String)
	assert.Equal(t, "postgres", row[1].String)
	assert.Equal(t, "alice", row[2].String)
	assert.Equal(t, "active", row[3].String)

	// Cols 6-8: HH:MM:SS — 150/100=1s, 100/100=1s, 50/100=0s.
	assert.Equal(t, "00:00:01", row[6].String)
	assert.Equal(t, "00:00:01", row[7].String)
	assert.Equal(t, "00:00:00", row[8].String)

	// Cols 9-10: KiB integers — 10240/1024=10, 20480/1024=20.
	assert.Equal(t, "10", row[9].String)
	assert.Equal(t, "20", row[10].String)

	// Col 11: HH:MM:SS — 100/100=1s.
	assert.Equal(t, "00:00:01", row[11].String)

	// Cols 12-17: pass-through unchanged from raw rate strings.
	assert.Equal(t, "37.50", row[12].String)
	assert.Equal(t, "25.00", row[13].String)
	assert.Equal(t, "12.50", row[14].String)
	assert.Equal(t, "10.00", row[15].String)
	assert.Equal(t, "20.00", row[16].String)
	assert.Equal(t, "100.00", row[17].String)

	// Col 18: query unchanged.
	assert.Equal(t, "SELECT 9", row[18].String)
}

// TestFormatProcPidResultForDisplay_TicksZero pins the defensive ticks<=0
// branch in formatIODelayCell. In production GetSysticksLocal always returns
// a positive value, but the branch exists to mirror the pre-split
// buildProcPidResult fallback ("0:00:00") and must not regress to a divide-
// by-zero or "00:00:00".
func TestFormatProcPidResultForDisplay_TicksZero(t *testing.T) {
	raw := PGresult{
		Valid: true,
		Ncols: procPidResultNcols,
		Nrows: 1,
		Cols:  append([]string(nil), procPidResultCols...),
		Values: [][]sql.NullString{{
			nullString("100"), nullString("d"), nullString("u"),
			nullString("a"), nullString(""), nullString(""),
			nullString("0"), nullString("0"), nullString("0"),
			nullString(""), nullString(""),
			nullString("100"), // col 11: non-empty iodelay → must hit ticks<=0 fallback
			nullString("0"), nullString("0"), nullString("0"),
			nullString(""), nullString(""), nullString(""),
			nullString("q"),
		}},
	}

	got := formatProcPidResultForDisplay(raw, 0)
	assert.Equal(t, "0:00:00", got.Values[0][11].String)
}

// TestFormatProcPidResultForDisplay_Sentinels verifies that the format stage
// passes through the sentinel values used by the raw stage to signal missing
// data: "" for unavailable IO/iodelay columns, "0" for invalid-PID CPU cols.
// This contract keeps BuildProcPidResult output bit-for-bit identical to the
// pre-MVC-split implementation.
func TestFormatProcPidResultForDisplay_Sentinels(t *testing.T) {
	raw := PGresult{
		Valid: true,
		Ncols: procPidResultNcols,
		Nrows: 1,
		Cols:  append([]string(nil), procPidResultCols...),
		Values: [][]sql.NullString{{
			nullString("abc"), nullString("d"), nullString("u"),
			nullString("a"), nullString(""), nullString(""),
			nullString("0"), nullString("0"), nullString("0"), // invalid-PID CPU sentinel
			nullString(""), nullString(""),                    // unavailable IO sentinel
			nullString(""),                                    // unavailable iodelay sentinel
			nullString("0"), nullString("0"), nullString("0"),
			nullString(""), nullString(""), nullString(""),
			nullString("BAD"),
		}},
	}

	got := formatProcPidResultForDisplay(raw, 100)
	row := got.Values[0]

	// Cols 6-8: "0" sentinel passes through (preserves pre-split behavior).
	assert.Equal(t, "0", row[6].String)
	assert.Equal(t, "0", row[7].String)
	assert.Equal(t, "0", row[8].String)
	// Cols 9-10: empty-string sentinel passes through.
	assert.Equal(t, "", row[9].String)
	assert.Equal(t, "", row[10].String)
	assert.True(t, row[9].Valid)
	// Col 11: empty-string sentinel passes through.
	assert.Equal(t, "", row[11].String)
	assert.True(t, row[11].Valid)
}

// TestSysInfoRoundTrip verifies that SysInfo marshals to JSON with the
// expected keys ("ticks", "cpu_count") and round-trips back to the original
// struct. The recorder writes one sysinfo entry per tick; the reporter reads
// it back into the metadata struct — a stable JSON contract is required.
func TestSysInfoRoundTrip(t *testing.T) {
	orig := SysInfo{Ticks: 100, CPUCount: 4}

	data, err := json.Marshal(orig)
	assert.NoError(t, err)
	// Keys must be the snake_case forms documented in the tech-spec.
	assert.Contains(t, string(data), `"ticks"`)
	assert.Contains(t, string(data), `"cpu_count"`)

	var got SysInfo
	err = json.Unmarshal(data, &got)
	assert.NoError(t, err)
	assert.Equal(t, orig.Ticks, got.Ticks)
	assert.Equal(t, orig.CPUCount, got.CPUCount)
}

// TestProcPidColIndexConstants locks the exported IO/iodelay column-index
// constants to the canonical procPidResultCols order. The constants carry a
// doc-comment asserting this mapping; this test makes a future column reorder
// fail loudly instead of silently shifting the indices that report relies on.
func TestProcPidColIndexConstants(t *testing.T) {
	assert.Equal(t, "read_total,KiB", procPidResultCols[ColReadTotalKiB])
	assert.Equal(t, "write_total,KiB", procPidResultCols[ColWriteTotalKiB])
	assert.Equal(t, "iodelay_total,s", procPidResultCols[ColIODelayTotalS])
}
