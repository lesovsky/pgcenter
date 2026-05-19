package stat

import (
	"database/sql"
	"os"
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
	got, err := readProcPidStat(os.Getpid())
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, got.Utime+got.Stime, float64(0))
}

func TestReadProcPidIOIntegration(t *testing.T) {
	got, err := readProcPidIO(os.Getpid())
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

// expectedProcPidCols is the canonical 19-column header for buildProcPidResult output.
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

	got := buildProcPidResult(activity, nil, currStats, nil, currIO, true, false, 100, 1, 4)

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

	got := buildProcPidResult(activity, nil, currStats, nil, nil, false, false, 100, 1, 4)

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
	got := buildProcPidResult(activity, prevStats, currStats, prevIO, currIO, true, false, 100, 0, 4)

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
			got := buildProcPidResult(tt.activity, tt.prevStats, tt.currStats, tt.prevIO, tt.currIO, tt.ioAvailable, false, 100, tt.itv, 4)
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
	got := buildProcPidResult(activity, prevStats, currStats, prevIO, currIO, true, false, 100, 1, 4)

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

	got := buildProcPidResult(activity, nil, currStats, nil, currIO, true, false, 100, 1, 4)

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

	got := buildProcPidResult(activity, nil, currStats, nil, currIO, true, false, 100, 1, 4)

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
	got := buildProcPidResult(activity, prevStats, currStats, nil, nil, false, true, 100, 1, 4)

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

	got := buildProcPidResult(activity, prevStats, currStats, nil, nil, false, false, 100, 1, 4)

	assert.Equal(t, 19, got.Ncols)
	row := got.Values[0]
	assert.Equal(t, "", row[11].String)
	assert.True(t, row[11].Valid)
	assert.Equal(t, "", row[17].String)
	assert.True(t, row[17].Valid)
}
