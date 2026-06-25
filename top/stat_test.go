package top

import (
	"bytes"
	"database/sql"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
	"github.com/stretchr/testify/assert"
	"regexp"
	"strings"
	"testing"
)

func Test_formatInfoString(t *testing.T) {
	testcases := []struct {
		cfg  postgres.Config
		want string
	}{
		{
			cfg:  postgres.Config{Config: &pgx.ConnConfig{Config: pgconn.Config{Host: "127.0.0.1", Port: 1234, User: "test", Database: "testdb"}}},
			want: "state [up]: 127.0.0.1:1234 test@testdb (ver: 13.1 on x86_64-~, up 01:23:48, recovery: f)",
		},
		{
			cfg:  postgres.Config{Config: &pgx.ConnConfig{Config: pgconn.Config{Host: "127.0.0.1", Port: 1234, User: "test", Database: ""}}},
			want: "state [up]: 127.0.0.1:1234 test@test (ver: 13.1 on x86_64-~, up 01:23:48, recovery: f)",
		},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, formatInfoString(tc.cfg, "up", "13.1 on x86_64-pc-linux-gnu Debian", "01:23:48", "f"))
	}
}

// Test_renderSysstat_compact is the writer-based golden test for the system-stats panel.
// renderSysstat is the io.Writer core extracted from printSysstat (task 03 refactor); its
// compact output must stay byte-identical. Line 1 carries a dynamic timestamp, so it is
// matched by pattern (with the exact load-average format), while lines 2..4 are asserted
// byte-for-byte against the golden, including the ANSI SGR codes.
func Test_renderSysstat_compact(t *testing.T) {
	s := stat.Stat{System: stat.System{
		LoadAvg: stat.LoadAvg{One: 1.23, Five: 0.45, Fifteen: 6.78},
		CPUStat: stat.CPUStat{
			User: 1.1, Sys: 2.2, Nice: 3.3, Idle: 4.4,
			Iowait: 5.5, Irq: 6.6, Softirq: 7.7, Steal: 8.8,
		},
		Meminfo: stat.Meminfo{
			MemTotal: 1000, MemFree: 200, MemUsed: 800,
			MemCached: 10, MemBuffers: 20, MemSlab: 30,
			SwapTotal: 500, SwapFree: 400, SwapUsed: 100,
			MemDirty: 5, MemWriteback: 7,
		},
	}}

	var buf bytes.Buffer
	err := renderSysstat(&buf, s, false, true, "")
	assert.NoError(t, err)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if assert.Len(t, lines, 4, "compact sysstat must be exactly 4 lines") {
		// line1: dynamic timestamp, fixed load-average format.
		assert.Regexp(t,
			`^pgcenter: \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}, load average: 1\.23, 0\.45, 6\.78$`,
			lines[0])
		// line2..4: byte-identical golden, including ANSI codes.
		assert.Equal(t,
			"    %cpu: \033[37;1m 1.1\033[0m us, \033[37;1m 2.2\033[0m sy, \033[37;1m 3.3\033[0m ni, \033[37;1m 4.4\033[0m id, \033[37;1m 5.5\033[0m wa, \033[37;1m 6.6\033[0m hi, \033[37;1m 7.7\033[0m si, \033[37;1m 8.8\033[0m st",
			lines[1])
		assert.Equal(t,
			" MiB mem: \033[37;1m  1000\033[0m total, \033[37;1m   200\033[0m free, \033[37;1m   800\033[0m used, \033[37;1m      60\033[0m buff/cached",
			lines[2])
		assert.Equal(t,
			"MiB swap: \033[37;1m   500\033[0m total, \033[37;1m   400\033[0m free, \033[37;1m   100\033[0m used, \033[37;1m     5/7\033[0m dirty/writeback",
			lines[3])
	}
}

// Test_renderPgstat_compact is the writer-based golden test for the summary Postgres-stats
// panel. renderPgstat is the io.Writer core extracted from printPgstat (task 03 refactor);
// its compact output must stay byte-identical. Line 1 is formatInfoString output; lines 2..4
// are asserted byte-for-byte against the golden, including the ANSI SGR codes.
func Test_renderPgstat_compact(t *testing.T) {
	db := &postgres.DB{Config: postgres.Config{Config: &pgx.ConnConfig{Config: pgconn.Config{
		Host: "127.0.0.1", Port: 1234, User: "test", Database: "testdb",
	}}}}
	props := stat.PostgresProperties{
		Version:           "13.1 on x86_64-pc-linux-gnu Debian",
		Recovery:          "f",
		GucMaxConnections: 100,
		GucMaxPrepXacts:   0,
		GucAVMaxWorkers:   3,
	}
	s := stat.Stat{Pgstat: stat.Pgstat{Activity: stat.Activity{
		State: "up", Uptime: "01:23:48",
		ConnTotal: 5, ConnPrepared: 1, ConnIdle: 2, ConnIdleXact: 0,
		ConnActive: 3, ConnWaiting: 0, ConnOthers: 0,
		AVWorkers: 1, AVUser: 0, AVAntiwrap: 0, AVMaxTime: "00:00:01",
		CallsRate: 42, StmtAvgTime: 1.234, XactMaxTime: "00:00:02", PrepMaxTime: "00:00:00",
	}}}

	var buf bytes.Buffer
	err := renderPgstat(&buf, s, props, db, false)
	assert.NoError(t, err)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if assert.Len(t, lines, 4, "compact pgstat must be exactly 4 lines") {
		// line1: formatInfoString output (ties to Test_formatInfoString).
		assert.Equal(t,
			formatInfoString(db.Config, s.Activity.State, props.Version, s.Activity.Uptime, props.Recovery),
			lines[0])
		// line2..4: byte-identical golden, including ANSI codes.
		assert.Equal(t,
			"  activity:\033[37;1m  5/100\033[0m conns,\033[37;1m  1/0\033[0m prepared,\033[37;1m  2\033[0m idle,\033[37;1m  0\033[0m idle_xact,\033[37;1m  3\033[0m active,\033[37;1m  0\033[0m waiting,\033[37;1m  0\033[0m others",
			lines[1])
		assert.Equal(t,
			"autovacuum: \033[37;1m 1/3\033[0m workers/max, \033[37;1m 0\033[0m manual, \033[37;1m 0\033[0m wraparound, \033[37;1m00:00:01\033[0m vac_maxtime",
			lines[2])
		assert.Equal(t,
			"statements: \033[37;1m 42\033[0m stmt/s, \033[37;1m1.234\033[0m stmt_avgtime, \033[37;1m00:00:02\033[0m xact_maxtime, \033[37;1m00:00:00\033[0m prep_maxtime",
			lines[3])
	}
}

func Test_formatError(t *testing.T) {
	testcases := []struct {
		err  error
		want string
	}{
		{err: nil, want: ""},
		{
			err:  &pgconn.PgError{Severity: "TEST", Message: "test message", Detail: "test detail", Hint: "test hint"},
			want: "TEST: test message\nDETAIL: test detail\nHINT: test hint",
		},
		{err: fmt.Errorf("example error"), want: "ERROR: example error"},
	}

	for _, tc := range testcases {
		got := formatError(tc.err)
		assert.Equal(t, tc.want, got)
	}
}

// verboseLines renders sysstat in verbose mode against a buffer and returns the rows split by line.
func verboseSysstatLines(t *testing.T, s stat.Stat, local bool, dataDir string) []string {
	t.Helper()
	var buf bytes.Buffer
	assert.NoError(t, renderSysstat(&buf, s, true, local, dataDir))
	return strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
}

// Test_renderSysstat_verboseIostatMaxUtil verifies the iostat verbose row selects the active device
// with the highest Util (devices with Completed==0 are skipped, matching printIostat) and reads that
// device's rates as-is from the struct (Decision 5 consistency).
func Test_renderSysstat_verboseIostatMaxUtil(t *testing.T) {
	s := stat.Stat{System: stat.System{
		Diskstats: stat.Diskstats{
			{Device: "sda", Completed: 100, Util: 30, Rsectors: 11, Wsectors: 22, Rcompleted: 33, Wcompleted: 44},
			{Device: "sdb", Completed: 0, Util: 99}, // inactive -> skipped even though Util is highest
			{Device: "sdc", Completed: 200, Util: 80, Rsectors: 1135, Wsectors: 1546, Rcompleted: 34152, Wcompleted: 17852},
		},
	}}

	lines := verboseSysstatLines(t, s, true, "")
	// 4 compact + 3 verbose
	if assert.Len(t, lines, 7) {
		iostat := lines[4]
		// 2 active devices (sdb skipped), max util from sdc (80), sdc's rates.
		assert.Contains(t, iostat, " 2 devices")
		assert.Contains(t, iostat, "80% max util")
		assert.Contains(t, iostat, "1135 rMB/s")
		assert.Contains(t, iostat, "34152 r/s")
		assert.Contains(t, iostat, "1546 wMB/s")
		assert.Contains(t, iostat, "17852 w/s")
	}
}

// Test_renderSysstat_verboseNicstatConversion verifies the nicstat verbose row applies the same
// print-time Rbytes/1024/128 conversion as printNetdev, selects the max-Utilization active interface
// (Packets==0 skipped), and composes err/coll as (Rerrs+Terrs)/Tcolls.
func Test_renderSysstat_verboseNicstatConversion(t *testing.T) {
	// Rbytes/1024/128 = 4345.0 Mbps exactly (4345*128*1024 = 569425920);
	// Tbytes/1024/128 = 6543.0 exactly (6543*128*1024 = 857604096).
	s := stat.Stat{System: stat.System{
		Netdevs: stat.Netdevs{
			{Ifname: "eth0", Packets: 0, Utilization: 100}, // inactive -> skipped
			{Ifname: "eth1", Packets: 10, Utilization: 60,
				Rbytes: 569425920, Tbytes: 857604096, Rerrs: 3000, Terrs: 451, Tcolls: 0},
		},
	}}

	lines := verboseSysstatLines(t, s, true, "")
	if assert.Len(t, lines, 7) {
		nicstat := lines[5]
		assert.Contains(t, nicstat, " 1 devices")
		assert.Contains(t, nicstat, "60% max util")
		assert.Contains(t, nicstat, "4345 rMbps")
		assert.Contains(t, nicstat, "6543 wMbps")
		// err = Rerrs+Terrs = 3451, coll = Tcolls = 0
		assert.Contains(t, nicstat, "3451/   0 err/coll")
	}
}

// Test_renderSysstat_verboseFirstTickNA verifies the first verbose tick (VerboseFirstTick set, the
// slice already populated zero-delta) renders n/a for iostat/nicstat delta fields, NOT 0; and the
// counter-case: the same populated slice WITHOUT the flag renders 0 (the flag is the only n/a trigger).
func Test_renderSysstat_verboseFirstTickNA(t *testing.T) {
	base := stat.System{
		Diskstats: stat.Diskstats{
			{Device: "sda", Completed: 100, Util: 0, Rsectors: 0, Wsectors: 0, Rcompleted: 0, Wcompleted: 0},
		},
		Netdevs: stat.Netdevs{
			{Ifname: "eth0", Packets: 10, Utilization: 0, Rbytes: 0, Tbytes: 0},
		},
	}

	// First tick: flag set -> n/a.
	first := base
	first.VerboseFirstTick = true
	lines := verboseSysstatLines(t, stat.Stat{System: first}, true, "")
	if assert.Len(t, lines, 7) {
		assert.Contains(t, lines[4], "n/a")
		assert.Contains(t, lines[5], "n/a")
	}

	// Counter-case: genuinely idle device, real zero deltas, flag NOT set -> 0, not n/a.
	idle := base
	idle.VerboseFirstTick = false
	lines = verboseSysstatLines(t, stat.Stat{System: idle}, true, "")
	if assert.Len(t, lines, 7) {
		assert.NotContains(t, lines[4], "n/a")
		assert.NotContains(t, lines[5], "n/a")
		assert.Contains(t, lines[4], "0% max util")
	}
}

// Test_renderSysstat_verboseFilesystMounted10 verifies the filesyst "mounted" field is truncated to
// 10 characters.
func Test_renderSysstat_verboseFilesystMounted10(t *testing.T) {
	s := stat.Stat{System: stat.System{
		Fsstats: stat.Fsstats{
			{Mount: stat.Mount{Device: "/dev/nvme0n1p2", Mountpoint: "/var/lib/postgresql/data", Fstype: "ext4"},
				Size: 1024, Used: 512, Pused: 74},
		},
	}}

	lines := verboseSysstatLines(t, s, false, "/var/lib/postgresql/data")
	if assert.Len(t, lines, 7) {
		filesyst := lines[6]
		// mounted truncated to first 10 runes of "/var/lib/postgresql/data" -> "/var/lib/p".
		assert.Contains(t, filesyst, "/dev/nvme0n1p2 on /var/lib/p (ext4)")
		assert.NotContains(t, filesyst, "/var/lib/postgresql")
	}
}

// Test_renderSysstat_verboseFilesystNA verifies that when no mount matches the data_directory, the
// filesyst row renders n/a (the iostat/nicstat rows are still rendered).
func Test_renderSysstat_verboseFilesystNA(t *testing.T) {
	s := stat.Stat{System: stat.System{
		Fsstats: stat.Fsstats{
			{Mount: stat.Mount{Device: "/dev/sda1", Mountpoint: "/srv/other", Fstype: "ext4"}},
		},
	}}

	lines := verboseSysstatLines(t, s, false, "/var/lib/pgsql/data")
	if assert.Len(t, lines, 7) {
		assert.Equal(t, "filesyst: n/a", lines[6])
	}
}

// Test_renderSysstat_compactUnchanged verifies that verbose=false produces byte-identical output to
// the compact path (no verbose rows added).
func Test_renderSysstat_compactUnchanged(t *testing.T) {
	s := stat.Stat{System: stat.System{
		LoadAvg:   stat.LoadAvg{One: 1, Five: 2, Fifteen: 3},
		Diskstats: stat.Diskstats{{Device: "sda", Completed: 100, Util: 50}},
		Netdevs:   stat.Netdevs{{Ifname: "eth0", Packets: 10, Utilization: 50}},
		Fsstats:   stat.Fsstats{{Mount: stat.Mount{Mountpoint: "/"}}},
	}}

	var compact, verboseOff bytes.Buffer
	assert.NoError(t, renderSysstat(&compact, s, false, true, "/"))
	assert.NoError(t, renderSysstat(&verboseOff, s, false, true, "/"))
	assert.Equal(t, compact.String(), verboseOff.String())
	// exactly 4 lines, no verbose rows.
	assert.Len(t, strings.Split(strings.TrimRight(compact.String(), "\n"), "\n"), 4)
}

// pgstatTestProps returns properties with worker GUC limits for the verbose pgstat rows.
func pgstatTestProps() stat.PostgresProperties {
	return stat.PostgresProperties{
		Version: "13.1", Recovery: "f", GucMaxConnections: 100, GucAVMaxWorkers: 3,
		GucMaxWorkerProcesses: 8, GucMaxLogicalReplicationWorkers: 4, GucMaxParallelWorkers: 8,
	}
}

func pgstatTestDB() *postgres.DB {
	return &postgres.DB{Config: postgres.Config{Config: &pgx.ConnConfig{Config: pgconn.Config{
		Host: "127.0.0.1", Port: 1234, User: "test", Database: "testdb",
	}}}}
}

// Test_renderPgstat_verboseNA verifies that an unavailable pgstat source (sentinel flags false,
// HasPrev false) renders n/a while the always-available rows still render.
func Test_renderPgstat_verboseNA(t *testing.T) {
	o := stat.PgstatOverview{
		Valid:   true,
		HasPrev: false, // first tick: all delta fields n/a
		// TotalSizeValid/LagBytesValid/RetainedValid/ArchivingBacklogValid/CacheHitRatioValid all false.
		DatabasesCount: 7, WalSize: 1024,
		WorkersUmbrellaActive: 1, WorkersLogicalActive: 0, WorkersParallelActive: 2,
		CkptTimed: 12, CkptReq: 3,
	}
	s := stat.Stat{Pgstat: stat.Pgstat{Overview: o}}

	var buf bytes.Buffer
	assert.NoError(t, renderPgstat(&buf, s, pgstatTestProps(), pgstatTestDB(), true))
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")

	if assert.Len(t, lines, 9, "4 compact + 5 verbose pgstat rows") {
		workload := lines[4]
		assert.Contains(t, workload, "n/a tps") // first tick rates n/a
		databases := lines[5]
		assert.Contains(t, databases, "n/a per  7 databases") // size n/a, count present
		assert.Contains(t, databases, "n/a cache hit ratio")  // ratio n/a
		workers := lines[6]
		assert.Contains(t, workers, "/8 workers/max") // always available
		replication := lines[7]
		assert.Contains(t, replication, "n/a lag")               // no standby
		assert.Contains(t, replication, "n/a archiving backlog") // archive off
		bgwr := lines[8]
		assert.Contains(t, bgwr, "12/ 3 timed/req") // absolute counters present
		assert.Contains(t, bgwr, "n/a/n/a ms write/sync")
	}
}

// Test_renderPgstat_verboseAvailable verifies that with a prev snapshot and available sources the
// verbose pgstat rows render real values (not n/a).
func Test_renderPgstat_verboseAvailable(t *testing.T) {
	o := stat.PgstatOverview{
		Valid: true, HasPrev: true,
		TPSRate: 1432, InsertsRate: 4132, UpdatesRate: 5421, DeletesRate: 4235,
		ReturnedRate: 2341, TempFilesRate: 123, OthersInterval: 4,
		DatabasesCount: 7, TotalSize: 1 << 40, TotalSizeValid: true, GrowthPerSec: 1 << 20,
		CacheHitRatio: 99.99, CacheHitRatioValid: true,
		WorkersUmbrellaActive: 0, WorkersLogicalActive: 0, WorkersParallelActive: 0,
		WalSize: 1 << 30, LagBytes: 1 << 20, LagBytesValid: true,
		SlotsCount: 1, RetainedBytes: 1 << 30, RetainedValid: true,
		ArchivingBacklog: 1 << 20, ArchivingBacklogValid: true, Senders: 2, Receivers: 1,
		CkptTimed: 12, CkptReq: 3, CkptWriteMsDelta: 245, CkptSyncMsDelta: 30, MaxWrittenDelta: 4,
	}
	s := stat.Stat{Pgstat: stat.Pgstat{Overview: o}}

	var buf bytes.Buffer
	assert.NoError(t, renderPgstat(&buf, s, pgstatTestProps(), pgstatTestDB(), true))
	out := buf.String()

	assert.Contains(t, out, "1432 tps")
	assert.Contains(t, out, "99.99% cache hit ratio")
	assert.Contains(t, out, "2/1 send/recv")
	assert.Contains(t, out, "maxwritten")
	assert.NotContains(t, out, "n/a")
}

// Test_renderPgstat_compactUnchanged verifies verbose=false output is byte-identical to compact.
func Test_renderPgstat_compactUnchanged(t *testing.T) {
	s := stat.Stat{Pgstat: stat.Pgstat{
		Activity: stat.Activity{State: "up", Uptime: "01:00:00"},
		Overview: stat.PgstatOverview{Valid: true, HasPrev: true, TPSRate: 999},
	}}

	var compact, verboseOff bytes.Buffer
	assert.NoError(t, renderPgstat(&compact, s, pgstatTestProps(), pgstatTestDB(), false))
	assert.NoError(t, renderPgstat(&verboseOff, s, pgstatTestProps(), pgstatTestDB(), false))
	assert.Equal(t, compact.String(), verboseOff.String())
	assert.Len(t, strings.Split(strings.TrimRight(compact.String(), "\n"), "\n"), 4)
}

// Test_alignViewToResult reproduces issue #99: pressing 'x' to cycle pg_stat_statements
// views caused "slice bounds out of range [:-1]" / "zero or negative width, skip".
//
// Root cause: after a view switch, the first stat batch may still carry the OLD view's
// column count. SetAlign fires on it (Aligned=false), populating ColsWidth for N columns.
// The next batch has M > N columns from the new view, but Aligned=true skips SetAlign,
// so ColsWidth[N..M-1] returns 0 → panic or error on truncation.
//
// Fix: alignViewToResult also re-aligns when len(ColsWidth) != r.Ncols.
func Test_alignViewToResult(t *testing.T) {
	makeResult := func(ncols int) stat.PGresult {
		cols := make([]string, ncols)
		row := make([]sql.NullString, ncols)
		for i := 0; i < ncols; i++ {
			cols[i] = fmt.Sprintf("col%d", i+1)
			row[i] = sql.NullString{String: "value", Valid: true}
		}
		return stat.PGresult{Valid: true, Ncols: ncols, Nrows: 1, Cols: cols,
			Values: [][]sql.NullString{row}}
	}

	t.Run("first render sets alignment", func(t *testing.T) {
		cfg := &config{view: view.View{Aligned: false, ColsWidth: map[int]int{}}}
		alignViewToResult(cfg, makeResult(8))
		assert.True(t, cfg.view.Aligned)
		assert.Equal(t, 8, len(cfg.view.ColsWidth))
	})

	t.Run("no re-alignment when column count matches", func(t *testing.T) {
		original := map[int]int{0: 99, 1: 99, 2: 99}
		cfg := &config{view: view.View{Aligned: true, ColsWidth: original}}
		alignViewToResult(cfg, makeResult(3))
		assert.Equal(t, 99, cfg.view.ColsWidth[0], "ColsWidth must not change when counts match")
	})

	t.Run("re-aligns when new result has MORE columns than ColsWidth (was: panic)", func(t *testing.T) {
		// Simulate: view was aligned with 5 columns (e.g. statements_general),
		// then a batch with 13 columns arrives (e.g. statements_timings after rapid 'x').
		// Before fix: ColsWidth[5..12] = 0 → "slice bounds out of range [:-1]".
		cfg := &config{view: view.View{
			Aligned:   true,
			ColsWidth: map[int]int{0: 10, 1: 10, 2: 10, 3: 10, 4: 10},
		}}
		alignViewToResult(cfg, makeResult(13))
		assert.Equal(t, 13, len(cfg.view.ColsWidth))
		for i := 0; i < 13; i++ {
			assert.Greater(t, cfg.view.ColsWidth[i], 0, "ColsWidth[%d] must be > 0", i)
		}
	})

	t.Run("re-aligns when new result has FEWER columns than ColsWidth", func(t *testing.T) {
		cfg := &config{view: view.View{
			Aligned:   true,
			ColsWidth: map[int]int{0: 10, 1: 10, 2: 10, 3: 10, 4: 10, 5: 10, 6: 10},
		}}
		alignViewToResult(cfg, makeResult(4))
		assert.Equal(t, 4, len(cfg.view.ColsWidth))
	})
}

// Test_visibleColumns covers the pure column-window function used by the horizontal
// scroll feature. The function freezes column 0 and computes a sliding window over the
// scrollable columns (1..ncols-1) that fits into termWidth, re-clamping the offset on
// every call. Width budget per column is colsWidth[i]+2 (the +2 gap added by printing).
func Test_visibleColumns(t *testing.T) {
	// uniformWidths builds a dense map[int]int with the same width for columns [0, ncols).
	uniformWidths := func(ncols, width int) map[int]int {
		m := make(map[int]int, ncols)
		for i := 0; i < ncols; i++ {
			m[i] = width
		}
		return m
	}

	t.Run("all columns fit", func(t *testing.T) {
		// 5 columns of width 10 => each costs 12; total 60 fits easily into 1000.
		// Nothing hidden either side, so no marker space is reserved.
		win := visibleColumns(5, uniformWidths(5, 10), 1000, 0)
		assert.Equal(t, 0, win.clamped)
		assert.False(t, win.hiddenLeft)
		assert.False(t, win.hiddenRight)
		assert.Equal(t, 1, win.first)
		assert.Equal(t, 4, win.last)
	})

	t.Run("narrow width, offset 0", func(t *testing.T) {
		// Each column costs 12. termWidth 40 => frozen(12) + budget 28. Right marker is
		// reserved (columns hidden right) => budget 27. Partial-visibility semantics: columns
		// 1,2 fit fully (start used 0,12 < 27); column 3 starts at used 24 < 27 so it is in the
		// window (partially visible, cost would reach 36); column 4 would start at 36 >= 27 so it
		// is hidden. Window 1..3, columns 4,5 hidden right.
		win := visibleColumns(6, uniformWidths(6, 10), 40, 0)
		assert.Equal(t, 0, win.clamped)
		assert.False(t, win.hiddenLeft)
		assert.True(t, win.hiddenRight)
		assert.Equal(t, 1, win.first)
		assert.Equal(t, 3, win.last)
	})

	t.Run("mid offset", func(t *testing.T) {
		// BOTH markers active at a genuine mid offset. 7 columns cost 12 each, termWidth 40 =>
		// after frozen, base budget 28; maxOffset is 4 (the last column's start fits from
		// offset 4). offset 1 hides column 1 left and columns 5,6 right, so both markers are
		// reserved (budget 26). Columns 2,3 fit fully (start 0,12 < 26); column 4 starts at 24 <
		// 26 => partially visible and in the window; column 5 would start at 36 => hidden. Window
		// 2..4.
		win := visibleColumns(7, uniformWidths(7, 10), 40, 1)
		assert.Equal(t, 1, win.clamped)
		assert.True(t, win.hiddenLeft)
		assert.True(t, win.hiddenRight)
		assert.Equal(t, 2, win.first)
		assert.Equal(t, 4, win.last)
	})

	t.Run("offset past end", func(t *testing.T) {
		// 6 columns, budget 28. Partial-visibility shrinks maxOffset to 2: from offset 2 the
		// window starts at column 3 and the last column's (5) start fits, so nothing more is
		// revealed by scrolling further. offset 99 clamps to 2. Only the left marker is reserved
		// (nothing hidden right) => budget 27; columns 3,4 fit fully and column 5 is partially
		// visible => window 3..5.
		win := visibleColumns(6, uniformWidths(6, 10), 40, 99)
		assert.Equal(t, 2, win.clamped)
		assert.True(t, win.hiddenLeft)
		assert.False(t, win.hiddenRight)
		assert.Equal(t, 3, win.first)
		assert.Equal(t, 5, win.last)
	})

	t.Run("very narrow only frozen fits", func(t *testing.T) {
		// termWidth 12 fits exactly the frozen column (cost 12), no room for scrollable.
		// Window must be empty: first=1, last=0 (last < first), no panic, no negative width.
		win := visibleColumns(6, uniformWidths(6, 10), 12, 0)
		assert.Equal(t, 0, win.clamped)
		assert.False(t, win.hiddenLeft)
		assert.True(t, win.hiddenRight)
		assert.Equal(t, 1, win.first)
		assert.Equal(t, 0, win.last)
	})

	t.Run("negative scroll budget (term narrower than frozen column)", func(t *testing.T) {
		// termWidth 5 is smaller than the frozen column cost (12) => scrollBudget < 0.
		// Must be graceful: empty window first=1, last=0, no panic.
		win := visibleColumns(6, uniformWidths(6, 10), 5, 0)
		assert.Equal(t, 0, win.clamped)
		assert.False(t, win.hiddenLeft)
		assert.True(t, win.hiddenRight)
		assert.Equal(t, 1, win.first)
		assert.Equal(t, 0, win.last)
	})

	t.Run("single frozen column only", func(t *testing.T) {
		// ncols == 1: only the frozen column exists, no scrollable columns at all.
		win := visibleColumns(1, uniformWidths(1, 10), 1000, 0)
		assert.Equal(t, 0, win.clamped)
		assert.False(t, win.hiddenLeft)
		assert.False(t, win.hiddenRight)
		assert.Less(t, win.last, win.first, "no scrollable columns => empty window")
	})

	t.Run("missing or zero ColsWidth key", func(t *testing.T) {
		// Sparse map: keys for some columns in [0, ncols) are absent (read as 0).
		// Math must stay bounded (each missing column costs the +2 gap), no panic.
		// Budget for termWidth 40: frozen col0 costs 12, base budget 28. Columns hidden right
		// => right marker reserved (budget 27). Partial-visibility starts: col1 start 0, col2
		// start 2, col3 start 14, col4 start 16 (all < 27 => visible), col5 would start at 28 >=
		// 27 => hidden. Window 1..4, only col5 hidden right.
		widths := map[int]int{0: 10, 2: 10, 4: 10} // columns 1, 3, 5 missing
		win := visibleColumns(6, widths, 40, 0)
		assert.Equal(t, 1, win.first)
		assert.Equal(t, 4, win.last)
		assert.Equal(t, 0, win.clamped)
		assert.False(t, win.hiddenLeft)
		assert.True(t, win.hiddenRight)
	})

	t.Run("negative offset clamps to zero", func(t *testing.T) {
		// offset -5 must clamp up to 0 (covers math.Max(offset, 0) lower bound).
		// 6 columns cost 12 each, termWidth 40 => right marker reserved (budget 27); same window
		// as the "narrow width, offset 0" case: columns 1,2 full + column 3 partial => 1..3.
		win := visibleColumns(6, uniformWidths(6, 10), 40, -5)
		assert.Equal(t, 0, win.clamped)
		assert.Equal(t, 1, win.first)
		assert.Equal(t, 3, win.last)
		assert.False(t, win.hiddenLeft)
		assert.True(t, win.hiddenRight)
	})

	t.Run("last column visible at max offset", func(t *testing.T) {
		// Invariant: at the maximum offset the last column (ncols-1) is visible and
		// nothing is hidden to the right. Narrow term 40 with 6 uniform columns has
		// maxOffset 2 under partial-visibility semantics (from offset 2 the last column's
		// start already fits); passing a large offset clamps to it. Window 3..5.
		ncols := 6
		win := visibleColumns(ncols, uniformWidths(ncols, 10), 40, 1<<30)
		assert.Equal(t, 2, win.clamped)
		assert.Equal(t, ncols-1, win.last, "last visible column must be the final column at max offset")
		assert.False(t, win.hiddenRight, "nothing hidden to the right at max offset")
		assert.True(t, win.hiddenLeft)
		assert.Equal(t, 3, win.first)
	})

	t.Run("wide last column visible partially, no right marker", func(t *testing.T) {
		// Reproduces issue #14 QA bugs. Mimics the activity/statements layout: a handful of
		// narrow columns followed by a very wide last column ("query", aligned by content up
		// to ~1000 chars). termWidth is wide enough that all narrow columns and the START of
		// the wide query column fit, but the query column does not fit in full.
		//
		// Correct behaviour (partial visibility): the wide last column is part of the window
		// (last == ncols-1, shown truncated by the terminal edge) and nothing is hidden to the
		// right, so no right marker. Pre-fix (full-fit) semantics drop the wide column entirely
		// => last == ncols-2 and hiddenRight == true.
		widths := map[int]int{0: 10, 1: 10, 2: 10, 3: 10, 4: 2000} // col4 = wide "query"
		win := visibleColumns(5, widths, 200, 0)
		assert.Equal(t, 4, win.last, "wide last column must be in the window (partially visible)")
		assert.False(t, win.hiddenRight, "nothing is hidden past the partially-visible last column")
		assert.Equal(t, 1, win.first)
		assert.False(t, win.hiddenLeft)
	})
}

// Test_visibleColumns_maxOffsetReachesLastColumn is a property test for the core scroll
// invariant: for ANY column count, width set and terminal width, scrolling to the maximum
// offset must make the final column (ncols-1) visible with no right marker left "stuck".
//
// It feeds a deliberately huge offset so visibleColumns re-clamps it down to maxOffset, then
// asserts: win.last == ncols-1 AND win.hiddenRight == false. A user who keeps pressing the
// right-scroll key must always be able to reach the last column, after which › disappears.
//
// The case where even the FULL window does not reach the last column at offset 0 because the
// frozen column alone overflows the terminal (empty window, last < first) is excluded — there
// is no scrollable real estate at all, so the invariant does not apply.
func Test_visibleColumns_maxOffsetReachesLastColumn(t *testing.T) {
	uniformWidths := func(ncols, width int) map[int]int {
		m := make(map[int]int, ncols)
		for i := 0; i < ncols; i++ {
			m[i] = width
		}
		return m
	}

	widthSet := []int{5, 8, 10, 15, 20, 25, 30}

	for ncols := 2; ncols <= 8; ncols++ {
		for _, width := range widthSet {
			for termWidth := 40; termWidth <= 120; termWidth += 5 {
				colsWidth := uniformWidths(ncols, width)

				// Huge offset forces clamped == maxOffset.
				win := visibleColumns(ncols, colsWidth, termWidth, 1<<30)

				// Skip configs where no scrollable column fits at all (frozen column alone
				// overflows / leaves no budget): the window is empty and the invariant about
				// reaching the last column is vacuous.
				if win.last < win.first {
					continue
				}

				assert.Equalf(t, ncols-1, win.last,
					"ncols=%d width=%d term=%d: max offset must reach last column (clamped=%d, first=%d, last=%d)",
					ncols, width, termWidth, win.clamped, win.first, win.last)
				assert.Falsef(t, win.hiddenRight,
					"ncols=%d width=%d term=%d: no right marker may remain at max offset (clamped=%d, first=%d, last=%d)",
					ncols, width, termWidth, win.clamped, win.first, win.last)
			}
		}
	}
}

// Test_render_widePartialLastColumn is the render-level reproduction of issue #14 QA: on a
// wide terminal where the last column ("query") is wider than the remaining budget, that
// column must still be printed (truncated by the terminal edge) and NO right marker may be
// drawn. Pre-fix the wide column is dropped from the window and a spurious › appears.
func Test_render_widePartialLastColumn(t *testing.T) {
	cfg := makeRenderConfig(5, 10)
	cfg.view.ColsWidth[4] = 2000 // wide last column, like the aligned "query" column
	cfg.scrollOffset = 0
	s := makeRenderResult(5, 1)

	win := visibleColumns(s.Result.Ncols, cfg.view.ColsWidth, 200, cfg.scrollOffset)

	var hbuf, dbuf bytes.Buffer
	assert.NoError(t, printStatHeader(&hbuf, s, cfg, win))
	assert.NoError(t, printStatData(&dbuf, s, cfg, false, win))

	// The wide last column (absolute index 4) must be present in both header and data.
	assert.Contains(t, hbuf.String(), "col4", "wide last column header must be printed")
	assert.Contains(t, dbuf.String(), "r0-c4", "wide last column value must be printed")
	// No right marker on a wide screen where the only "hidden" part is the tail of the last
	// column (which the terminal simply truncates).
	assert.NotContains(t, hbuf.String(), "›", "no right marker when only the last column is partial")
}

// makeRenderConfig builds a config with a synthetic, already-aligned view of ncols
// columns each of the given width. Column names are col0..colN-1, so a column name
// can be matched back to its absolute index in render output.
func makeRenderConfig(ncols, width int) *config {
	cols := make([]string, ncols)
	colsWidth := make(map[int]int, ncols)
	for i := 0; i < ncols; i++ {
		cols[i] = fmt.Sprintf("col%d", i)
		colsWidth[i] = width
	}
	return &config{view: view.View{
		Ncols:     ncols,
		Cols:      cols,
		ColsWidth: colsWidth,
		Aligned:   true,
		Filters:   map[int]*regexp.Regexp{},
	}}
}

// makeRenderResult builds a synthetic PGresult with nrows rows of ncols columns.
// Each cell value is "rR-cC" so a printed value can be matched to its absolute
// (row, column) coordinates — this is how the absolute-index lookup is verified.
func makeRenderResult(ncols, nrows int) stat.Stat {
	cols := make([]string, ncols)
	for i := 0; i < ncols; i++ {
		cols[i] = fmt.Sprintf("col%d", i)
	}
	values := make([][]sql.NullString, nrows)
	for r := 0; r < nrows; r++ {
		row := make([]sql.NullString, ncols)
		for c := 0; c < ncols; c++ {
			row[c] = sql.NullString{String: fmt.Sprintf("r%d-c%d", r, c), Valid: true}
		}
		values[r] = row
	}
	return stat.Stat{Pgstat: stat.Pgstat{Result: stat.PGresult{
		Valid: true, Ncols: ncols, Nrows: nrows, Cols: cols, Values: values,
	}}}
}

// Test_printStatData_windowed_midOffset verifies windowed data rendering with a narrow
// terminal and a mid offset (columns hidden both left and right). The frozen column 0
// must be printed, and values must be looked up by the ABSOLUTE column index, not by the
// position within the visible window (the regression guarded by removing the colnum
// counter).
func Test_printStatData_windowed_midOffset(t *testing.T) {
	// 7 columns of width 10 (cost 12 each). termWidth 40 => frozen(12) + base budget 28; both
	// markers reserved (budget 26). offset 1 => window covers absolute columns 2,3,4 (2,3 full,
	// 4 partial). Column 1 is hidden left; columns 5,6 are hidden right.
	cfg := makeRenderConfig(7, 10)
	cfg.scrollOffset = 1
	s := makeRenderResult(7, 2)

	var buf bytes.Buffer
	win := visibleColumns(s.Result.Ncols, cfg.view.ColsWidth, 40, cfg.scrollOffset)
	err := printStatData(&buf, s, cfg, false, win)
	assert.NoError(t, err)

	out := buf.String()
	// Frozen column 0 value for row 0 present.
	assert.Contains(t, out, "r0-c0")
	// Windowed columns at absolute indices 2,3,4 present (value tagged by absolute col).
	assert.Contains(t, out, "r0-c2")
	assert.Contains(t, out, "r0-c3")
	assert.Contains(t, out, "r0-c4")
	// Hidden columns (1 left; 5,6 right) must NOT be printed.
	assert.NotContains(t, out, "r0-c1")
	assert.NotContains(t, out, "r0-c5")
	assert.NotContains(t, out, "r0-c6")
	// Second row rendered too.
	assert.Contains(t, out, "r1-c0")
	assert.Contains(t, out, "r1-c3")
}

// Test_printStatData_emptyResult verifies that a result with zero rows prints no data
// lines and does not panic (the outer loop simply does not execute).
func Test_printStatData_emptyResult(t *testing.T) {
	cfg := makeRenderConfig(6, 10)
	s := makeRenderResult(6, 0)

	var buf bytes.Buffer
	win := visibleColumns(s.Result.Ncols, cfg.view.ColsWidth, 40, cfg.scrollOffset)
	err := printStatData(&buf, s, cfg, false, win)
	assert.NoError(t, err)
	assert.Empty(t, buf.String())
}

// Test_printStatHeader_rightEdgeMarker verifies that with a narrow terminal and offset 0
// the header shows the right-edge marker (columns hidden to the right) but not the
// left-edge marker, and that the frozen column 0 name is present.
func Test_printStatHeader_rightEdgeMarker(t *testing.T) {
	cfg := makeRenderConfig(6, 10)
	cfg.scrollOffset = 0
	s := makeRenderResult(6, 1)

	var buf bytes.Buffer
	win := visibleColumns(s.Result.Ncols, cfg.view.ColsWidth, 40, cfg.scrollOffset)
	err := printStatHeader(&buf, s, cfg, win)
	assert.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "col0", "frozen column must be present")
	assert.Contains(t, out, "›", "right marker expected (columns hidden to the right)")
	assert.NotContains(t, out, "‹", "left marker not expected at offset 0")
	// The right marker must be at the END of the header line (rightmost position), not just
	// present somewhere — it marks columns hidden off the right edge.
	assert.True(t, strings.HasSuffix(strings.TrimRight(out, "\n"), "›"),
		"right marker must be the last visible rune on the header line")
}

// ansiEscape matches SGR escape sequences (\033[...m) so the visible width of a rendered
// line can be measured by counting runes after stripping them.
var ansiEscape = regexp.MustCompile("\033\\[[0-9;]*m")

// visibleRuneLen returns the number of visible runes in a rendered line, after removing
// ANSI SGR escape sequences and the trailing newline. Used to assert the alignment invariant.
func visibleRuneLen(line string) int {
	return len([]rune(ansiEscape.ReplaceAllString(strings.TrimRight(line, "\n"), "")))
}

// Test_printStatHeader_midOffset_bothMarkers verifies that at a mid offset BOTH edge markers
// are present: the left marker ‹ (columns hidden left) and the right marker › (columns hidden
// right). The TDD Anchor for task 02 requires both markers in the mid-offset case (review
// round 1, MAJOR #2).
func Test_printStatHeader_midOffset_bothMarkers(t *testing.T) {
	cfg := makeRenderConfig(7, 10)
	cfg.scrollOffset = 1 // window 2..4: column 1 hidden left, columns 5,6 hidden right
	s := makeRenderResult(7, 1)

	var buf bytes.Buffer
	win := visibleColumns(s.Result.Ncols, cfg.view.ColsWidth, 40, cfg.scrollOffset)
	err := printStatHeader(&buf, s, cfg, win)
	assert.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "‹", "left marker expected at mid offset (columns hidden to the left)")
	assert.Contains(t, out, "›", "right marker expected at mid offset (columns hidden to the right)")
}

// Test_render_alignmentInvariant is the litmus test for MAJOR #1: at a mid offset where both
// edge markers are drawn, the visible (ANSI-stripped) width of the header line must equal the
// visible width of every data line. The edge markers are visible runes printed by the header;
// the data rows must reserve the same space (blank fillers) or columns drift out of alignment.
// This test fails on the pre-fix implementation (header is wider than data by the marker runes)
// and passes after the marker width is reserved in the budget and mirrored as blanks in data.
func Test_render_alignmentInvariant(t *testing.T) {
	cfg := makeRenderConfig(7, 10)
	cfg.scrollOffset = 1 // both markers drawn (column 1 hidden left, columns 5,6 hidden right)
	s := makeRenderResult(7, 3)

	win := visibleColumns(s.Result.Ncols, cfg.view.ColsWidth, 40, cfg.scrollOffset)
	assert.True(t, win.hiddenLeft && win.hiddenRight, "test premise: both markers must be active")

	var hbuf, dbuf bytes.Buffer
	assert.NoError(t, printStatHeader(&hbuf, s, cfg, win))
	assert.NoError(t, printStatData(&dbuf, s, cfg, false, win))

	headerWidth := visibleRuneLen(hbuf.String())
	for i, line := range strings.Split(strings.TrimRight(dbuf.String(), "\n"), "\n") {
		assert.Equal(t, headerWidth, visibleRuneLen(line),
			"data row %d visible width must equal header visible width (alignment invariant)", i)
	}
}

// Test_printStatData_truncation verifies the truncation branch of printDataCell after the
// colnum→absolute-index reindex: a value longer than its column width is cut to width-1 and
// suffixed with '~'. Column 0 (frozen) is always printed, so its long value is the cleanest
// probe (review round 1, MINOR).
func Test_printStatData_truncation(t *testing.T) {
	cfg := makeRenderConfig(6, 5) // each scrollable/frozen column width 5
	s := makeRenderResult(6, 1)
	// Overwrite the frozen column value with one longer than width 5.
	s.Result.Values[0][0] = sql.NullString{String: "abcdefghij", Valid: true}

	var buf bytes.Buffer
	win := visibleColumns(s.Result.Ncols, cfg.view.ColsWidth, 40, cfg.scrollOffset)
	err := printStatData(&buf, s, cfg, false, win)
	assert.NoError(t, err)

	out := buf.String()
	// Truncated to width-1 (4 chars) + '~'.
	assert.Contains(t, out, "abcd~", "long value must be truncated to width-1 with '~' suffix")
	assert.NotContains(t, out, "abcde", "original untruncated value must not appear")
}

// Test_printStatHeader_frozenColumn verifies the frozen column 0 is always present in the
// header regardless of offset, and that when OrderKey == 0 the sort highlight escape
// sequence is applied to it (priority over frozen-bold, Decision 4) without doubling
// escape sequences.
func Test_printStatHeader_frozenColumn(t *testing.T) {
	s := makeRenderResult(6, 1)

	t.Run("frozen column present at large offset", func(t *testing.T) {
		cfg := makeRenderConfig(6, 10)
		cfg.scrollOffset = 99 // clamped internally; frozen col still rendered
		var buf bytes.Buffer
		win := visibleColumns(s.Result.Ncols, cfg.view.ColsWidth, 40, cfg.scrollOffset)
		err := printStatHeader(&buf, s, cfg, win)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "col0")
	})

	t.Run("frozen column is bold when not the ordered column", func(t *testing.T) {
		// Default OrderKey != 0 path: column 0 must carry the frozen-bold escape
		// (\033[30;47;1m). Plain Contains "col0" passes even without bold, so assert the
		// exact bold sequence precedes the frozen column name (review round 1, MINOR).
		cfg := makeRenderConfig(6, 10)
		cfg.view.OrderKey = 3 // some other column is the ordered one
		var buf bytes.Buffer
		win := visibleColumns(s.Result.Ncols, cfg.view.ColsWidth, 40, cfg.scrollOffset)
		err := printStatHeader(&buf, s, cfg, win)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "\033[30;47;1mcol0", "frozen column must be bold when not the ordered column")
	})

	t.Run("sort highlight has priority on column 0", func(t *testing.T) {
		cfg := makeRenderConfig(6, 10)
		cfg.view.OrderKey = 0
		var buf bytes.Buffer
		win := visibleColumns(s.Result.Ncols, cfg.view.ColsWidth, 40, cfg.scrollOffset)
		err := printStatHeader(&buf, s, cfg, win)
		assert.NoError(t, err)
		out := buf.String()
		// Sort highlight sequence (\033[47;1m) is the existing ordered-column escape.
		assert.Contains(t, out, "\033[47;1mcol0", "sort highlight must wrap frozen column when OrderKey==0")
		// The frozen-bold sequence must not be doubled onto the same column 0 segment.
		assert.Equal(t, 1, strings.Count(out[:strings.Index(out, "col0")+len("col0")], "col0"))
	})
}

// Test_printDbstat_clampsScrollOffset verifies the runaway-offset guard: rendering with a
// wildly inflated scrollOffset writes the clamped value back into config.scrollOffset, so
// repeated scroll-right at the visual maximum never accumulates the field beyond maxOffset.
func Test_printDbstat_clampsScrollOffset(t *testing.T) {
	cfg := makeRenderConfig(6, 10)
	cfg.scrollOffset = 1 << 20 // absurdly large, far beyond maxOffset
	s := makeRenderResult(6, 2)

	var buf bytes.Buffer
	// renderDbstat is the writer-based core of printDbstat (printDbstat feeds it the
	// terminal width from v.Size()). termWidth 40 => maxOffset 2 for 6 uniform columns under
	// partial-visibility semantics (from offset 2 the last column's start already fits).
	err := renderDbstat(&buf, cfg, s, 40)
	assert.NoError(t, err)
	assert.Equal(t, 2, cfg.scrollOffset, "scrollOffset must be clamped to maxOffset, not the inflated value")
}
