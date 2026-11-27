package top

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
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

// testRenderTime is the fixed render stamp the pre-existing renderSysstat tests pass now that line
// 1's clock is a parameter. Its value is irrelevant to them — they assert the refresh field, the
// verbose rows or the compact prefix, never the timestamp itself; the two dedicated
// Test_renderSysstat_timestamp* tests are what pin the stamp.
var testRenderTime = time.Date(2021, 6, 15, 12, 30, 45, 0, time.UTC)

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
	err := renderSysstat(&buf, s, false, true, "", 5*time.Second, testRenderTime)
	assert.NoError(t, err)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if assert.Len(t, lines, 4, "compact sysstat must be exactly 4 lines") {
		// line1: dynamic timestamp, fixed refresh and load-average format.
		assert.Regexp(t,
			`^pgcenter: \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}, refresh: \d+s, load average: 1\.23, 0\.45, 6\.78$`,
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

// Test_renderSysstat_refreshFormat pins the refresh interval on line 1 to whole seconds. The
// interval must be formatted explicitly as "%ds": time.Duration's own String() renders 60s as
// "1m0s" and 300s as "5m0s", which is not what the header must show.
func Test_renderSysstat_refreshFormat(t *testing.T) {
	testcases := []struct {
		refresh time.Duration
		want    string
	}{
		{refresh: 1 * time.Second, want: "refresh: 1s,"},
		{refresh: 60 * time.Second, want: "refresh: 60s,"},
		{refresh: 300 * time.Second, want: "refresh: 300s,"},
	}

	for _, tc := range testcases {
		var buf bytes.Buffer
		assert.NoError(t, renderSysstat(&buf, stat.Stat{}, false, true, "", tc.refresh, testRenderTime))

		line1 := strings.SplitN(buf.String(), "\n", 2)[0]
		assert.Contains(t, line1, tc.want)
	}
}

// Test_renderSysstat_timestampFromParameter pins line 1's clock to the timestamp the caller passes
// in, not to the wall clock. This is what makes a repainted frozen frame coherent: the repaint
// path feeds the stored render time, so the header shows the age of the data on screen.
//
// The assertion is an EQUALITY against a stamp far from today, deliberately not a regexp: a
// `\d{4}-\d{2}-\d{2}` pattern matches time.Now() just as happily as the parameter, so a reinstated
// time.Now() would sail straight through it. Only the exact 2020 stamp can fail.
//
// This is one third of the frozen-clock property; see the COMPOSITIONAL PROOF note on
// Test_renderSysstat_timestampIsTheOnlySource below for the other two.
func Test_renderSysstat_timestampFromParameter(t *testing.T) {
	s := stat.Stat{System: stat.System{
		LoadAvg: stat.LoadAvg{One: 1.23, Five: 0.45, Fifteen: 6.78},
	}}

	// The location is irrelevant to the assertion: the "2006-01-02 15:04:05" layout carries no zone
	// and Format performs no conversion, so the rendered text is this value's own wall clock.
	at := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)

	var buf bytes.Buffer
	assert.NoError(t, renderSysstat(&buf, s, false, true, "", time.Second, at))

	line1 := strings.SplitN(buf.String(), "\n", 2)[0]
	assert.Equal(t,
		"pgcenter: 2020-01-02 03:04:05, refresh: 1s, load average: 1.23, 0.45, 6.78",
		line1)
}

// Test_renderSysstat_timestampIsTheOnlySource proves the parameter is genuinely the source of line
// 1 rather than being accepted and ignored — a failure mode a single-stamp test cannot distinguish,
// since one rendering alone cannot show that a DIFFERENT stamp produces a different line.
//
// Two distinct stamps are rendered from the same stat.Stat: each rendering carries its own stamp,
// and the two line-1 strings differ ONLY in the timestamp field, with rows 2..4 byte-identical.
//
// COMPOSITIONAL PROOF - neither this test nor the one above proves on its own that a repaint
// prints a frozen clock over frozen data. That property holds across three tests in two files:
// these two say renderSysstat is a pure function of (s, at) with no internal clock read, and
// Test_frameStore_publishOnlyOnLivePath / Test_frameStore_repaintRendersIdenticalBytes
// (top/pause_test.go) say a repaint never restamps the store. No single test can join them:
// renderFrame is reachable only through a live *gocui.Gui, so the end-to-end check exists only in
// the manual stand run. Change any of the three and the other two do not notice.
func Test_renderSysstat_timestampIsTheOnlySource(t *testing.T) {
	s := stat.Stat{System: stat.System{
		LoadAvg: stat.LoadAvg{One: 1.23, Five: 0.45, Fifteen: 6.78},
		CPUStat: stat.CPUStat{User: 1.1, Sys: 2.2, Nice: 3.3, Idle: 4.4},
		Meminfo: stat.Meminfo{MemTotal: 1000, MemFree: 200, MemUsed: 800},
	}}

	testcases := []struct {
		name string
		at   time.Time
		want string
	}{
		{
			name: "2020",
			at:   time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC),
			want: "pgcenter: 2020-01-02 03:04:05, refresh: 1s, load average: 1.23, 0.45, 6.78",
		},
		{
			name: "1999",
			at:   time.Date(1999, 12, 31, 23, 59, 58, 0, time.UTC),
			want: "pgcenter: 1999-12-31 23:59:58, refresh: 1s, load average: 1.23, 0.45, 6.78",
		},
	}

	var rendered [][]string

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			assert.NoError(t, renderSysstat(&buf, s, false, true, "", time.Second, tc.at))

			lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
			if assert.Len(t, lines, 4) {
				assert.Equal(t, tc.want, lines[0])
			}
			rendered = append(rendered, lines)
		})
	}

	if !assert.Len(t, rendered, 2) {
		return
	}

	// Everything on line 1 after the timestamp field is unaffected by the stamp...
	assert.Equal(t,
		strings.TrimPrefix(rendered[0][0], "pgcenter: "+testcases[0].at.Format("2006-01-02 15:04:05")),
		strings.TrimPrefix(rendered[1][0], "pgcenter: "+testcases[1].at.Format("2006-01-02 15:04:05")))
	// ...and so are rows 2..4, which the stamp must not reach at all.
	assert.Equal(t, rendered[0][1:], rendered[1][1:])
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

// boldOpen/boldReset are the SGR pair the compact summary rows wrap their values in
// (top/stat.go:280 etc.). The verbose rows must use the very same pair, so the tests spell it out
// literally instead of reusing the production helper.
const boldOpen, boldReset = "\033[37;1m", "\033[0m"

// boldSpanRe matches one complete bold span and captures what it wraps, so a test can inspect
// every value the renderer marked as bold (and prove no sentinel/identifier is among them).
var boldSpanRe = regexp.MustCompile("\033\\[37;1m(.*?)\033\\[0m")

// boldSpans returns the contents of every bold span on a rendered line.
func boldSpans(line string) []string {
	m := boldSpanRe.FindAllStringSubmatch(line, -1)
	spans := make([]string, 0, len(m))
	for _, s := range m {
		spans = append(spans, s[1])
	}
	return spans
}

// verboseLines renders sysstat in verbose mode against a buffer and returns the rows split by line.
func verboseSysstatLines(t *testing.T, s stat.Stat, local bool, dataDir string) []string {
	t.Helper()
	var buf bytes.Buffer
	assert.NoError(t, renderSysstat(&buf, s, true, local, dataDir, time.Second, testRenderTime))
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
		// Full-line golden locks the exact reserve-width layout and field order: 2 active devices
		// (sdb skipped), max util from sdc (80), sdc's rates. Every value carries the base rows'
		// bold pair; the '%' of "max util" is a unit and stays outside the span.
		assert.Equal(t,
			"  iostat: \033[37;1m 2\033[0m devices, \033[37;1m 80\033[0m% max util, \033[37;1m1135 rMB/s\033[0m, \033[37;1m34152\033[0m r/s, \033[37;1m1546 wMB/s\033[0m, \033[37;1m17852\033[0m w/s",
			lines[4])
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
		// Full-line golden: /1024/128 parity (4345/6543), err=Rerrs+Terrs=3451, coll=Tcolls=0.
		// err/coll is a composite value and is wrapped as ONE bold span.
		assert.Equal(t,
			" nicstat: \033[37;1m 1\033[0m devices, \033[37;1m 60\033[0m% max util, \033[37;1m4345 rMbps\033[0m, \033[37;1m6543 wMbps\033[0m, \033[37;1m3451/0\033[0m err/coll",
			lines[5])
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
		// The degraded branch still prints a real device count, so that count IS bold while the
		// n/a arguments beside it stay plain — the one place where "degraded branch" and "plain"
		// come apart (Decision 9: bold means "there is a real number here").
		assert.Contains(t, lines[4], boldOpen+" 1"+boldReset+" devices")
		assert.Contains(t, lines[5], boldOpen+" 1"+boldReset+" devices")
		for _, row := range []string{lines[4], lines[5]} {
			assert.Equal(t, 1, strings.Count(row, boldOpen), "degraded row bolds the device count only: %q", row)
			for _, span := range boldSpans(row) {
				assert.NotContains(t, span, naLiteral, "n/a sentinel must not be wrapped in bold: %q", row)
			}
		}
	}

	// Counter-case: genuinely idle device, real zero deltas, flag NOT set -> 0, not n/a.
	idle := base
	idle.VerboseFirstTick = false
	lines = verboseSysstatLines(t, stat.Stat{System: idle}, true, "")
	if assert.Len(t, lines, 7) {
		assert.NotContains(t, lines[4], "n/a")
		assert.NotContains(t, lines[5], "n/a")
		// The utilization value is bold, so the reset sits between the digit and the '%' — the
		// assertion is about what the user SEES, hence the SGR-stripped line.
		assert.Contains(t, stripSGR(lines[4]), "0% max util")
	}
}

// Test_renderSysstat_verboseFilesystMounted10 verifies the filesyst "mounted" field is truncated to
// 10 characters.
func Test_renderSysstat_verboseFilesystMounted10(t *testing.T) {
	s := stat.Stat{System: stat.System{
		Fsstats: stat.Fsstats{
			{Mount: stat.Mount{Device: "/dev/nvme0n1p2", Mountpoint: "/var/lib/postgresql/data", Fstype: "ext4"},
				Size: 1024, Used: 512, Pused: 74.3},
		},
	}}

	lines := verboseSysstatLines(t, s, false, "/var/lib/postgresql/data")
	if assert.Len(t, lines, 7) {
		// Full-line golden: mounted truncated to first 10 runes of "/var/lib/postgresql/data".
		// use-% mirrors printFsstats' %.0f of Pused exactly (74.3 -> 74, NOT Ceil's 75) — the
		// verbose row must agree with the full fsstat panel for the same filesystem. Only the three
		// numbers are bold — device, mounted and fstype are identifiers and stay plain.
		assert.Equal(t,
			"filesyst: /dev/nvme0n1p2 on /var/lib/p (ext4), \033[37;1m1.0K\033[0m size, \033[37;1m512B\033[0m used, \033[37;1m 74\033[0m% use",
			lines[6])
		assert.NotContains(t, lines[6], "/var/lib/postgresql")
		assert.NotContains(t, lines[6], "75")
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

// verboseSysstatBoldFixture returns a stat with one active disk, one active interface and one
// filesystem matching "/data", so all three verbose rows take their VALUE branch.
func verboseSysstatBoldFixture() stat.Stat {
	return stat.Stat{System: stat.System{
		Diskstats: stat.Diskstats{
			{Device: "sda", Completed: 100, Util: 80, Rsectors: 1135, Wsectors: 1546, Rcompleted: 34152, Wcompleted: 17852},
		},
		Netdevs: stat.Netdevs{
			{Ifname: "eth0", Packets: 10, Utilization: 60,
				Rbytes: 569425920, Tbytes: 857604096, Rerrs: 3000, Terrs: 451, Tcolls: 0},
		},
		Fsstats: stat.Fsstats{
			{Mount: stat.Mount{Device: "/dev/sda1", Mountpoint: "/data", Fstype: "ext4"},
				Size: 1024, Used: 512, Pused: 74.3},
		},
	}}
}

// Test_renderSysstatVerbose_boldOnValues locks the bold convention on the three verbose system
// rows: every numeric value carries the base rows' \033[37;1m…\033[0m pair. The spans are COUNTED
// per row against the exact expected number — asserting that a row merely contains an escape would
// pass with one value bolded and the rest plain. The visible text (escapes stripped) must stay
// byte-identical to the pre-bold golden: SGR sequences are zero-width.
func Test_renderSysstatVerbose_boldOnValues(t *testing.T) {
	lines := verboseSysstatLines(t, verboseSysstatBoldFixture(), false, "/data")

	if !assert.Len(t, lines, 7) {
		return
	}

	testcases := []struct {
		row     int
		spans   int // number of bold value spans expected on the row
		visible string
	}{
		// iostat: device count, max util, two rates, two completed counters.
		{row: 4, spans: 6, visible: "  iostat:  1 devices,  80% max util, 1135 rMB/s, 34152 r/s, 1546 wMB/s, 17852 w/s"},
		// nicstat: device count, max util, two rates, err/coll as ONE span.
		{row: 5, spans: 5, visible: " nicstat:  1 devices,  60% max util, 4345 rMbps, 6543 wMbps, 3451/0 err/coll"},
		// filesyst: size, used, use% — the identifiers are not values and stay plain.
		{row: 6, spans: 3, visible: "filesyst: /dev/sda1 on /data (ext4), 1.0K size, 512B used,  74% use"},
	}

	for _, tc := range testcases {
		line := lines[tc.row]
		assert.Equal(t, tc.spans, strings.Count(line, boldOpen), "line %d: bold span count: %q", tc.row, line)
		assert.Equal(t, tc.spans, strings.Count(line, boldReset), "line %d: unbalanced bold spans: %q", tc.row, line)
		assert.Equal(t, tc.visible, stripSGR(line), "line %d: bold must not change the visible text", tc.row)
	}

	// The '%' of "max util" is a unit, so it stays OUTSIDE the span (as in the compact rows).
	assert.Contains(t, lines[4], boldOpen+" 80"+boldReset+"% max util")
	assert.Contains(t, lines[5], boldOpen+" 60"+boldReset+"% max util")
	// err/coll is a composite value: one span for both sides, matching stat.go:472/481.
	assert.Contains(t, lines[5], boldOpen+"3451/0"+boldReset+" err/coll")
	// The rate formatter returns value and unit as one string, so the unit is bolded with the
	// number (Decision 9 accepts this explicitly — pretty.RateUnitPrefixed is not changed).
	assert.Contains(t, lines[4], boldOpen+"1135 rMB/s"+boldReset)
}

// Test_renderSysstatVerbose_identifiersNotBold verifies the negative half of the convention on the
// filesyst row: device, mountpoint and fstype are identifiers, not values, and render plain — while
// size/used/use% on the very same row are bold.
func Test_renderSysstatVerbose_identifiersNotBold(t *testing.T) {
	lines := verboseSysstatLines(t, verboseSysstatBoldFixture(), false, "/data")

	if !assert.Len(t, lines, 7) {
		return
	}
	row := lines[6]

	for _, id := range []string{"/dev/sda1", "/data", "ext4"} {
		for _, span := range boldSpans(row) {
			assert.NotContains(t, span, id, "identifier %q must not be wrapped in bold: %q", id, row)
		}
	}

	assert.Contains(t, row, boldOpen+"1.0K"+boldReset+" size")
	assert.Contains(t, row, boldOpen+"512B"+boldReset+" used")
	assert.Contains(t, row, boldOpen+" 74"+boldReset+"% use")
}

// Test_renderSysstatVerbose_degradedFilesystNotBold verifies the fully degraded filesyst row keeps
// its bare n/a plain — there is no value on that row at all.
func Test_renderSysstatVerbose_degradedFilesystNotBold(t *testing.T) {
	s := stat.Stat{System: stat.System{
		Fsstats: stat.Fsstats{
			{Mount: stat.Mount{Device: "/dev/sda1", Mountpoint: "/srv/other", Fstype: "ext4"}},
		},
	}}

	lines := verboseSysstatLines(t, s, false, "/var/lib/pgsql/data")
	if assert.Len(t, lines, 7) {
		assert.Equal(t, "filesyst: n/a", lines[6])
		assert.NotContains(t, lines[6], boldOpen)
	}
}

// Test_renderSysstat_compactUnchanged verifies that verbose=false adds no rows AND that turning
// verbose on does not perturb the compact rows: the verbose output's first 4 lines must equal the
// full compact output. This exercises the real invariant (verbose only appends), unlike comparing
// verbose=false to itself.
func Test_renderSysstat_compactUnchanged(t *testing.T) {
	s := stat.Stat{System: stat.System{
		LoadAvg:   stat.LoadAvg{One: 1, Five: 2, Fifteen: 3},
		Diskstats: stat.Diskstats{{Device: "sda", Completed: 100, Util: 50}},
		Netdevs:   stat.Netdevs{{Ifname: "eth0", Packets: 10, Utilization: 50}},
		Fsstats:   stat.Fsstats{{Mount: stat.Mount{Mountpoint: "/"}}},
	}}

	var compact, verbose bytes.Buffer
	// Both renderings take the SAME stamp: the assertion below is a byte-identity comparison of the
	// compact prefix, and two different stamps would break it on line 1 for the wrong reason.
	assert.NoError(t, renderSysstat(&compact, s, false, true, "/", time.Second, testRenderTime))
	assert.NoError(t, renderSysstat(&verbose, s, true, true, "/", time.Second, testRenderTime))

	compactLines := strings.Split(strings.TrimRight(compact.String(), "\n"), "\n")
	verboseLines := strings.Split(strings.TrimRight(verbose.String(), "\n"), "\n")

	// verbose=false: exactly 4 compact rows, no verbose rows leaked.
	assert.Len(t, compactLines, 4)
	// verbose=true: 4 compact + 3 verbose, and the compact prefix is byte-identical.
	if assert.Len(t, verboseLines, 7) {
		assert.Equal(t, compactLines, verboseLines[:4])
	}
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
		// Full-line golden per row. First-tick rates n/a; unavailable sources n/a; always-available
		// fields (count, wal size, workers, absolute ckpt counters, slots count, senders/receivers) render.
		// Width-matched n/a: the rate fields reserve width 4, so n/a renders as " n/a" (right-aligned
		// into 4) — the same columns the value occupies — keeping each unit label static. "others"
		// reserves width 3, where n/a fits exactly. cache hit ratio reserves width 7, so n/a renders
		// as "    n/a" (right-aligned into 7).
		assert.Equal(t,
			"    workload:  n/a tps,  n/a ins/s,  n/a upd/s,  n/a del/s,  n/a ret/s,  n/a tmp/s, n/a others",
			lines[4])
		// The always-real fields are bold; every n/a sentinel stays plain (Decision 9).
		assert.Equal(t,
			"   databases:      n/a per \033[37;1m 7\033[0m databases,      n/a growth/s,     n/a cache hit ratio",
			lines[5])
		assert.Equal(t,
			"     workers: \033[37;1m 1/8\033[0m workers/max, \033[37;1m 0/4\033[0m logical workers, \033[37;1m 2/8\033[0m parallel workers",
			lines[6])
		assert.Equal(t,
			" replication: \033[37;1m1.0K\033[0m wal size,      n/a lag, \033[37;1m 0\033[0m/     n/a slots/retain,      n/a archiving backlog, \033[37;1m0/0\033[0m senders/receivers",
			lines[7])
		assert.Equal(t,
			"   bgwr/ckpt: \033[37;1m12/3\033[0m timed/req, n/a/n/a ms write/sync, n/a maxwritten",
			lines[8])
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
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")

	if assert.Len(t, lines, 9) {
		// Full-line golden per row: every field renders its real value (no n/a), including the
		// bgwr write/sync ms deltas (245/30) and maxwritten delta count (4). Every value is bold,
		// as in the compact rows above.
		assert.Equal(t,
			"    workload: \033[37;1m1432\033[0m tps, \033[37;1m4132\033[0m ins/s, \033[37;1m5421\033[0m upd/s, \033[37;1m4235\033[0m del/s, \033[37;1m2341\033[0m ret/s, \033[37;1m 123\033[0m tmp/s, \033[37;1m  4\033[0m others",
			lines[4])
		// cache hit ratio reserves width 7 ("%6.2f%%"): 99.99 renders as " 99.99%" (leading space),
		// the same 7-column slot the n/a sentinel occupies — so the label position is identical to
		// the n/a state (see Test_renderPgstat_verboseNA).
		assert.Equal(t,
			"   databases: \033[37;1m    1.0T\033[0m per \033[37;1m 7\033[0m databases, \033[37;1m    1.0M\033[0m growth/s, \033[37;1m 99.99%\033[0m cache hit ratio",
			lines[5])
		assert.Equal(t,
			"     workers: \033[37;1m 0/8\033[0m workers/max, \033[37;1m 0/4\033[0m logical workers, \033[37;1m 0/8\033[0m parallel workers",
			lines[6])
		// slots/retain is two spans: an always-real count next to a size that can degrade.
		assert.Equal(t,
			" replication: \033[37;1m1.0G\033[0m wal size, \033[37;1m    1.0M\033[0m lag, \033[37;1m 1\033[0m/\033[37;1m    1.0G\033[0m slots/retain, \033[37;1m    1.0M\033[0m archiving backlog, \033[37;1m2/1\033[0m senders/receivers",
			lines[7])
		// write/sync is two adjacent spans too — both sides degrade together to their own n/a.
		assert.Equal(t,
			"   bgwr/ckpt: \033[37;1m12/3\033[0m timed/req, \033[37;1m245\033[0m/\033[37;1m30\033[0m ms write/sync, \033[37;1m 4\033[0m maxwritten",
			lines[8])
	}
	assert.NotContains(t, buf.String(), "n/a")
}

// verbosePgstatLines renders pgstat in verbose mode from the given overview and returns the rows.
func verbosePgstatLines(t *testing.T, o stat.PgstatOverview) []string {
	t.Helper()
	var buf bytes.Buffer
	assert.NoError(t, renderPgstat(&buf, stat.Stat{Pgstat: stat.Pgstat{Overview: o}}, pgstatTestProps(), pgstatTestDB(), true))
	return strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
}

// Test_renderPgstatVerbose_boldOnValues locks the bold convention on the five verbose pgstat rows:
// every numeric value carries the base rows' \033[37;1m…\033[0m pair. The spans are COUNTED per row
// against the exact expected number — asserting that a row merely contains an escape would pass
// with one value bolded and the rest plain. The visible text (escapes stripped) must stay
// byte-identical to the pre-bold golden: SGR sequences are zero-width.
func Test_renderPgstatVerbose_boldOnValues(t *testing.T) {
	o := stat.PgstatOverview{
		Valid: true, HasPrev: true,
		TPSRate: 1432, InsertsRate: 4132, UpdatesRate: 5421, DeletesRate: 4235,
		ReturnedRate: 2341, TempFilesRate: 123, OthersInterval: 4,
		DatabasesCount: 7, TotalSize: 1 << 40, TotalSizeValid: true, GrowthPerSec: 1 << 20,
		CacheHitRatio: 99.99, CacheHitRatioValid: true,
		WorkersUmbrellaActive: 1, WorkersLogicalActive: 2, WorkersParallelActive: 3,
		WalSize: 1 << 30, LagBytes: 1 << 20, LagBytesValid: true,
		SlotsCount: 1, RetainedBytes: 1 << 30, RetainedValid: true,
		ArchivingBacklog: 1 << 20, ArchivingBacklogValid: true, Senders: 2, Receivers: 1,
		CkptTimed: 12, CkptReq: 3, CkptWriteMsDelta: 245, CkptSyncMsDelta: 30, MaxWrittenDelta: 4,
	}

	lines := verbosePgstatLines(t, o)
	if !assert.Len(t, lines, 9) {
		return
	}

	testcases := []struct {
		row     int
		spans   int // number of bold value spans expected on the row
		visible string
	}{
		// workload: the seven naInt rates (bolded inside naInt's value branch).
		{row: 4, spans: 7,
			visible: "    workload: 1432 tps, 4132 ins/s, 5421 upd/s, 4235 del/s, 2341 ret/s,  123 tmp/s,   4 others"},
		// databases: size, databases count, growth, cache hit ratio.
		{row: 5, spans: 4,
			visible: "   databases:     1.0T per  7 databases,     1.0M growth/s,  99.99% cache hit ratio"},
		// workers: three "%s/%d" composites, one span each (matching stat.go:481).
		{row: 6, spans: 3,
			visible: "     workers:  1/8 workers/max,  2/4 logical workers,  3/8 parallel workers"},
		// replication: wal size, lag, slots count, retain, backlog, senders/receivers. The
		// slots/retain composite is TWO spans — an always-real count next to a possibly-n/a size.
		{row: 7, spans: 6,
			visible: " replication: 1.0G wal size,     1.0M lag,  1/    1.0G slots/retain,     1.0M archiving backlog, 2/1 senders/receivers"},
		// bgwr/ckpt: timed/req as one span, then writeMs, syncMs and maxwritten.
		{row: 8, spans: 4,
			visible: "   bgwr/ckpt: 12/3 timed/req, 245/30 ms write/sync,  4 maxwritten"},
	}

	for _, tc := range testcases {
		line := lines[tc.row]
		assert.Equal(t, tc.spans, strings.Count(line, boldOpen), "line %d: bold span count: %q", tc.row, line)
		assert.Equal(t, tc.spans, strings.Count(line, boldReset), "line %d: unbalanced bold spans: %q", tc.row, line)
		assert.Equal(t, tc.visible, stripSGR(line), "line %d: bold must not change the visible text", tc.row)
	}

	// Composite rules spelled out: one span for the workers/senders/timed pairs, two separate
	// spans for slots/retain.
	assert.Contains(t, lines[6], boldOpen+" 1/8"+boldReset+" workers/max")
	assert.Contains(t, lines[7], boldOpen+"2/1"+boldReset+" senders/receivers")
	assert.Contains(t, lines[7], boldOpen+" 1"+boldReset+"/"+boldOpen+"    1.0G"+boldReset+" slots/retain")
	assert.Contains(t, lines[8], boldOpen+"12/3"+boldReset+" timed/req")
	assert.Contains(t, lines[8], boldOpen+"245"+boldReset+"/"+boldOpen+"30"+boldReset+" ms write/sync")
}

// Test_renderPgstatVerbose_sentinelsNotBold is the negative half of the convention: with no prev
// snapshot and every source unavailable, not a single n/a rendering is wrapped in bold — asserted
// per sentinel (every bold span on the row is inspected), not as "the row contains n/a".
func Test_renderPgstatVerbose_sentinelsNotBold(t *testing.T) {
	o := stat.PgstatOverview{
		Valid:   true,
		HasPrev: false, // first tick: workload rates and the bgwr deltas degrade
		// TotalSizeValid/CacheHitRatioValid/LagBytesValid/RetainedValid/ArchivingBacklogValid false.
		DatabasesCount: 7, WalSize: 1024, CkptTimed: 12, CkptReq: 3,
	}

	lines := verbosePgstatLines(t, o)
	if !assert.Len(t, lines, 9) {
		return
	}

	// Expected number of n/a sentinels per verbose row: workload (7 rates), databases (size,
	// growth, cache hit ratio), workers (none — all fields are always real), replication (lag,
	// retain, archiving backlog), bgwr/ckpt (write ms, sync ms, maxwritten).
	naPerRow := []int{7, 3, 0, 3, 3}

	for i, wantNA := range naPerRow {
		line := lines[4+i]
		assert.Equal(t, wantNA, strings.Count(line, naLiteral), "line %d: n/a count: %q", 4+i, line)
		for _, span := range boldSpans(line) {
			assert.NotContains(t, span, naLiteral, "line %d: n/a sentinel wrapped in bold: %q", 4+i, span)
		}
	}
}

// Test_renderPgstat_verboseNAWidthStatic verifies the core of the visual-review fix: when a verbose
// field degrades to n/a, the n/a occupies the SAME reserved width as the value it replaces, so the
// label that follows it does not move between the two states. It renders the verbose rows once with
// the values available and once unavailable (n/a) and asserts that the trailing label sits at the
// identical byte offset in both — for cache hit ratio (the regression the user flagged) AND for a
// workload rate (the naInt path), locking both fixed-reserve paths against the same regression class.
func Test_renderPgstat_verboseNAWidthStatic(t *testing.T) {
	base := stat.PgstatOverview{Valid: true, HasPrev: true, DatabasesCount: 7}

	render := func(o stat.PgstatOverview) []string {
		var buf bytes.Buffer
		assert.NoError(t, renderPgstat(&buf, stat.Stat{Pgstat: stat.Pgstat{Overview: o}}, pgstatTestProps(), pgstatTestDB(), true))
		return strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	}

	withVal := base
	withVal.CacheHitRatio = 100.0
	withVal.CacheHitRatioValid = true
	withVal.TPSRate = 999
	valRows := render(withVal)
	// HasPrev false -> workload rates n/a; CacheHitRatioValid false -> cache hit ratio n/a.
	naBase := base
	naBase.HasPrev = false
	naRows := render(naBase)

	// Values are bold and n/a sentinels are plain, so the two states differ by 12 zero-width bytes
	// per value (\033[37;1m + \033[0m). The invariant this test states is about SCREEN columns, so
	// every comparison below runs over the SGR-stripped row (stripSGR reuses the ansiEscape regexp).

	// cache hit ratio (databases row, index 5): value in a fixed 7-column slot ("100.00%"); n/a
	// right-aligned into the same 7 ("    n/a"), so the trailing label offset is identical.
	assert.Contains(t, stripSGR(valRows[5]), "100.00% cache hit ratio")
	assert.Contains(t, stripSGR(naRows[5]), "    n/a cache hit ratio")
	assert.Equal(t,
		strings.Index(stripSGR(valRows[5]), "cache hit ratio"),
		strings.Index(stripSGR(naRows[5]), "cache hit ratio"),
		"cache hit ratio label must sit at the same offset in the value and n/a states")

	// workload rate (workload row, index 4): the naInt path reserves width 4, so n/a (" n/a") spans
	// the same columns as the value (" 999"), keeping the "tps" label static — a direct offset
	// assertion on the naInt path, not just an implied golden-string match.
	assert.Contains(t, stripSGR(valRows[4]), " 999 tps")
	assert.Contains(t, stripSGR(naRows[4]), " n/a tps")
	assert.Equal(t,
		strings.Index(stripSGR(valRows[4]), "tps"),
		strings.Index(stripSGR(naRows[4]), "tps"),
		"tps label must sit at the same offset in the value and n/a states")

	// The five verbose Size fields (databases: size before " per", growth before " growth/s";
	// replication: lag before " lag", retain before " slots/retain", backlog before " archiving
	// backlog") render via the fixed-width SizeWidth(width 8); their n/a fallback uses naReserve(8).
	// Two groups of offset assertions lock the column/label position:
	//   (a) identical between two value samples of DIFFERENT width (1.0M vs 1023.9G);
	//   (b) identical between the value state and the n/a state (the regression that breathes today).
	type sizeField struct {
		row   int    // line index of the row the field lives on
		label string // trailing label whose byte offset must stay static
	}
	sizeFields := []sizeField{
		{row: 5, label: " per"},               // databases size
		{row: 5, label: " growth/s"},          // databases growth
		{row: 7, label: " lag"},               // replication lag
		{row: 7, label: " slots/retain"},      // replication retain (post-slash)
		{row: 7, label: " archiving backlog"}, // replication backlog
	}

	// Sample A: all five Size sources valid with narrow values (1<<20 -> "1.0M").
	sampleA := base
	sampleA.TotalSize = 1 << 20
	sampleA.TotalSizeValid = true
	sampleA.GrowthPerSec = 1 << 20
	sampleA.LagBytes = 1 << 20
	sampleA.LagBytesValid = true
	sampleA.RetainedBytes = 1 << 20
	sampleA.RetainedValid = true
	sampleA.ArchivingBacklog = 1 << 20
	sampleA.ArchivingBacklogValid = true
	rowsA := render(sampleA)

	// Sample B: same fields valid but WIDER values (1000 GiB worth of bytes -> "1000.0G", 7 chars).
	wide := int64(1000) * (1 << 30) // 1000 GiB, just under 1 TiB -> "1000.0G"
	sampleB := base
	sampleB.TotalSize = wide
	sampleB.TotalSizeValid = true
	sampleB.GrowthPerSec = wide
	sampleB.LagBytes = wide
	sampleB.LagBytesValid = true
	sampleB.RetainedBytes = wide
	sampleB.RetainedValid = true
	sampleB.ArchivingBacklog = wide
	sampleB.ArchivingBacklogValid = true
	rowsB := render(sampleB)

	// n/a sample: every Size source unavailable -> naReserve(8) fallback.
	sampleNA := base
	sampleNA.TotalSizeValid = false
	sampleNA.LagBytesValid = false
	sampleNA.RetainedValid = false
	sampleNA.ArchivingBacklogValid = false
	rowsNA := render(sampleNA)

	for _, f := range sizeFields {
		// (a) two value samples of different width keep the label at the same visible offset.
		assert.Equal(t,
			strings.Index(stripSGR(rowsA[f.row]), f.label),
			strings.Index(stripSGR(rowsB[f.row]), f.label),
			"%q label must sit at the same offset across different-width Size samples", f.label)
		// (b) value vs n/a keep the label at the same visible offset (RED on bare naLiteral today).
		assert.Equal(t,
			strings.Index(stripSGR(rowsA[f.row]), f.label),
			strings.Index(stripSGR(rowsNA[f.row]), f.label),
			"%q label must sit at the same offset in the value and n/a states", f.label)
	}
}

// Test_renderPgstat_compactUnchanged verifies that verbose=false adds no rows AND that turning
// verbose on does not perturb the compact rows: the verbose output's first 4 lines must equal the
// full compact output (verbose only appends).
func Test_renderPgstat_compactUnchanged(t *testing.T) {
	s := stat.Stat{Pgstat: stat.Pgstat{
		Activity: stat.Activity{State: "up", Uptime: "01:00:00"},
		Overview: stat.PgstatOverview{Valid: true, HasPrev: true, TPSRate: 999},
	}}

	var compact, verbose bytes.Buffer
	assert.NoError(t, renderPgstat(&compact, s, pgstatTestProps(), pgstatTestDB(), false))
	assert.NoError(t, renderPgstat(&verbose, s, pgstatTestProps(), pgstatTestDB(), true))

	compactLines := strings.Split(strings.TrimRight(compact.String(), "\n"), "\n")
	verboseLines := strings.Split(strings.TrimRight(verbose.String(), "\n"), "\n")

	// verbose=false: exactly 4 compact rows.
	assert.Len(t, compactLines, 4)
	// verbose=true: 4 compact + 5 verbose, and the compact prefix is byte-identical.
	if assert.Len(t, verboseLines, 9) {
		assert.Equal(t, compactLines, verboseLines[:4])
	}
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

// stripSGR removes the ANSI SGR sequences from a rendered line so an assertion can measure the
// VISIBLE text and the VISIBLE column of a label: the escapes are zero-width on screen, so the
// alignment invariants (naReserve / ReserveWidth / SizeWidth) hold in columns, not in bytes. It
// reuses the ansiEscape regexp visibleRuneLen is built on — visibleRuneLen itself returns a rune
// count and cannot be used for offsets.
func stripSGR(line string) string {
	return ansiEscape.ReplaceAllString(line, "")
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

// Test_printDataCell_doesNotMutateSource verifies that rendering is read-only: after a render
// that truncates a too-long value, the source cell still holds the original, untruncated text.
// The truncation test above asserts on the rendered buffer only and cannot catch an in-place
// write into the result set.
func Test_printDataCell_doesNotMutateSource(t *testing.T) {
	cfg := makeRenderConfig(6, 5) // each scrollable/frozen column width 5
	s := makeRenderResult(6, 1)
	// Overwrite the frozen column value with one longer than width 5.
	s.Result.Values[0][0] = sql.NullString{String: "abcdefghij", Valid: true}

	var buf bytes.Buffer
	win := visibleColumns(s.Result.Ncols, cfg.view.ColsWidth, 40, cfg.scrollOffset)
	err := printStatData(&buf, s, cfg, false, win)
	assert.NoError(t, err)

	// The premise guard keeps the assertion below from passing vacuously if column 0 ever stops
	// being rendered: without a truncating render there is nothing for the mutation check to catch.
	if assert.Contains(t, buf.String(), "abcd~", "test premise: the render must have truncated the value") {
		assert.Equal(t, "abcdefghij", s.Result.Values[0][0].String,
			"rendering must not write the truncated value back into the result set")
	}
}

// Test_printDataCell_widenAfterTruncation verifies that a truncating render does not destroy the
// value for later renders: the same stat.Stat rendered again into a wider column shows the full
// original text. This is the user-spec criterion "расширение колонки во время паузы не приводит к
// навсегда обрезанному значению", expressed at the printer level.
func Test_printDataCell_widenAfterTruncation(t *testing.T) {
	s := makeRenderResult(6, 1)
	s.Result.Values[0][0] = sql.NullString{String: "abcdefghij", Valid: true}

	// First render: narrow column, value truncated.
	narrowCfg := makeRenderConfig(6, 5)
	var narrowBuf bytes.Buffer
	narrowWin := visibleColumns(s.Result.Ncols, narrowCfg.view.ColsWidth, 40, narrowCfg.scrollOffset)
	assert.NoError(t, printStatData(&narrowBuf, s, narrowCfg, false, narrowWin))
	if !assert.Contains(t, narrowBuf.String(), "abcd~", "test premise: the first render must truncate") {
		return
	}

	// Second render of the SAME stat.Stat: the column is now wide enough for the whole value.
	wideCfg := makeRenderConfig(6, 12)
	var wideBuf bytes.Buffer
	wideWin := visibleColumns(s.Result.Ncols, wideCfg.view.ColsWidth, 80, wideCfg.scrollOffset)
	assert.NoError(t, printStatData(&wideBuf, s, wideCfg, false, wideWin))
	assert.Contains(t, wideBuf.String(), "abcdefghij",
		"widening the column after a truncating render must show the full original value")
	assert.NotContains(t, wideBuf.String(), "abcd~",
		"no truncation marker may survive into the widened render")
}

// Test_printDataCell_zeroOrNegativeWidth verifies the zero/negative-width guard: for a value that
// overflows the column, printDataCell returns the "zero or negative width, skip" error and writes
// nothing. The evaluation order that puts this guard inside the overflow branch is pinned
// separately by Test_printDataCell_zeroWidthShortValueDoesNotError — both widths used here
// overflow, so this test alone cannot tell the two orderings apart.
func Test_printDataCell_zeroOrNegativeWidth(t *testing.T) {
	// makeRenderResult values are "rR-cC" (5 bytes), so len(value) > width holds for width 0 and -1.
	for _, width := range []int{0, -1} {
		cfg := makeRenderConfig(6, 5)
		cfg.view.ColsWidth[0] = width
		s := makeRenderResult(6, 1)

		var buf bytes.Buffer
		err := printDataCell(&buf, s, cfg, 0, 0)
		assert.EqualError(t, err, "zero or negative width, skip", "width %d", width)
		assert.Empty(t, buf.String(), "nothing may be printed for width %d", width)
		assert.Equal(t, "r0-c0", s.Result.Values[0][0].String,
			"the error path must not touch the source value either (width %d)", width)
	}
}

// Test_printDataCell_zeroWidthShortValueDoesNotError pins the evaluation order the rewrite had to
// preserve: the width guard lives INSIDE the overflow branch, so a zero-width column is an error
// only when the value actually overflows it. A short value there still renders. Hoisting the guard
// above the overflow check leaves every other test in the package green, yet would abort the whole
// render (printStatData propagates the error) on an empty cell in a zero-width column. This also
// covers the Valid: false case, whose .String is "".
func Test_printDataCell_zeroWidthShortValueDoesNotError(t *testing.T) {
	cfg := makeRenderConfig(6, 5)
	cfg.view.ColsWidth[0] = 0
	s := makeRenderResult(6, 1)
	s.Result.Values[0][0] = sql.NullString{Valid: false} // .String is ""

	var buf bytes.Buffer
	assert.NoError(t, printDataCell(&buf, s, cfg, 0, 0),
		"width 0 errors only when the value overflows it; a short value must still render")
	assert.Equal(t, "  ", buf.String(),
		"a short value in a zero-width column renders as the bare +2 gap")
}

// Test_printDataCell_exactWidthNotTruncated pins the len == width boundary: a value exactly
// filling its column is printed whole, with no '~'. The comparison is strictly greater, and a
// > → >= regression would otherwise stay green.
func Test_printDataCell_exactWidthNotTruncated(t *testing.T) {
	cfg := makeRenderConfig(6, 5) // width 5 == len("r0-c0")
	s := makeRenderResult(6, 1)

	var buf bytes.Buffer
	assert.NoError(t, printDataCell(&buf, s, cfg, 0, 0))
	assert.Equal(t, "r0-c0  ", buf.String(),
		"a value exactly filling the column must be printed whole, padded to width+2, with no '~'")
}

// Test_printDataCell_truncatedCellExactBytes pins the exact bytes of a truncated cell. The other
// truncation tests assert with Contains at the whole-line level; this one localises a truncation
// regression to the cell instead of surfacing it as a line-level or alignment failure, and makes
// the byte-identity contract of this change explicit. (Padding width itself is covered by
// Test_render_alignmentInvariant on the non-truncating path — a truncated value is by construction
// exactly ColsWidth bytes, so the two padding formulas cannot diverge here.)
func Test_printDataCell_truncatedCellExactBytes(t *testing.T) {
	cfg := makeRenderConfig(6, 5)
	s := makeRenderResult(6, 1)
	s.Result.Values[0][0] = sql.NullString{String: "abcdefghij", Valid: true}

	var buf bytes.Buffer
	assert.NoError(t, printDataCell(&buf, s, cfg, 0, 0))
	assert.Equal(t, "abcd~  ", buf.String(),
		"truncated cell must be width-1 bytes + '~', padded to ColsWidth+2")
}

// Test_printDataCell_multiByteIsByteSliced is a characterization test: truncation is BYTE-based
// today and may cut a UTF-8 sequence mid-rune. Task 01 requires byte-identical output, so this
// behaviour is preserved deliberately rather than fixed. Converting printDataCell to rune-based
// slicing is a separate, deliberate change — it must update this test, not silently break it.
func Test_printDataCell_multiByteIsByteSliced(t *testing.T) {
	cfg := makeRenderConfig(6, 4)
	s := makeRenderResult(6, 1)
	s.Result.Values[0][0] = sql.NullString{String: "αβγδε", Valid: true} // 10 bytes, 5 runes

	var buf bytes.Buffer
	assert.NoError(t, printDataCell(&buf, s, cfg, 0, 0))
	// value[:3] keeps "α" (2 bytes) plus the leading byte of "β", then '~' is appended. fmt pads
	// %-*s by RUNES, so the dangling byte counts as one and three spaces reach the width-4+2 gap.
	assert.Equal(t, "α\xce~   ", buf.String(),
		"multi-byte values are byte-sliced today; a mid-rune cut is preserved behaviour")
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

// Test_scrollOffsetFor pins the minimum-movement auto-scroll helper: given the current
// offset and the sort column, it returns the offset that brings that column into the
// visible window with the smallest movement, or leaves the offset alone when the column
// is already visible (fully or partially) or cannot be shown at all.
//
// The fixture is 7 uniform columns of width 10 (each costs 12 printed cells) on a 40-cell
// terminal: frozen column 0 takes 12, leaving a base budget of 28 for scrollable columns.
// At offset 1 the window is 2..4 (column 4 only partially visible), maxOffset is 3.
func Test_scrollOffsetFor(t *testing.T) {
	// uniformWidths builds a dense map[int]int with the same width for columns [0, ncols).
	uniformWidths := func(ncols, width int) map[int]int {
		m := make(map[int]int, ncols)
		for i := 0; i < ncols; i++ {
			m[i] = width
		}
		return m
	}

	const (
		ncols     = 7
		width     = 10
		termWidth = 40
	)
	widths := uniformWidths(ncols, width)

	t.Run("already visible", func(t *testing.T) {
		// Column 3 sits inside the window 2..4 at offset 1 => no movement at all.
		want := visibleColumns(ncols, widths, termWidth, 1).clamped
		assert.Equal(t, want, scrollOffsetFor(ncols, widths, termWidth, 1, 3))
	})

	t.Run("partially visible counts as visible", func(t *testing.T) {
		// Column 4 is the last one in the window at offset 1: its start fits the budget but
		// its tail is truncated by the terminal edge. Per [009] countFit semantics that is
		// "visible" and must not trigger a scroll.
		win := visibleColumns(ncols, widths, termWidth, 1)
		assert.Equal(t, 4, win.last, "fixture: column 4 must be the partially visible last one")
		assert.Equal(t, win.clamped, scrollOffsetFor(ncols, widths, termWidth, 1, 4))
	})

	t.Run("column to the right", func(t *testing.T) {
		// Column 6 lies past the window at offset 1. The result must be the SMALLEST offset
		// whose window admits it: visible at the returned offset, not visible one step before.
		got := scrollOffsetFor(ncols, widths, termWidth, 1, 6)
		assert.Greater(t, got, 1, "the window must move rightwards")

		win := visibleColumns(ncols, widths, termWidth, got)
		assert.True(t, 6 >= win.first && 6 <= win.last,
			"column 6 must be inside the window at the returned offset %d (first=%d last=%d)", got, win.first, win.last)

		prev := visibleColumns(ncols, widths, termWidth, got-1)
		assert.False(t, 6 >= prev.first && 6 <= prev.last,
			"column 6 must NOT be inside the window at offset %d (first=%d last=%d) — the result is not minimal",
			got-1, prev.first, prev.last)
	})

	t.Run("column to the left", func(t *testing.T) {
		// At offset 3 (maxOffset) the window starts at column 4; column 1 is hidden to the
		// left. The minimum movement puts it exactly at the left edge of the window.
		got := scrollOffsetFor(ncols, widths, termWidth, 3, 1)
		win := visibleColumns(ncols, widths, termWidth, got)
		assert.Equal(t, 1, win.first, "the sort column must land at the left edge of the window")
		assert.True(t, 1 <= win.last, "the sort column must be inside the window")
	})

	t.Run("frozen column zero", func(t *testing.T) {
		// Column 0 is always printed, so selecting it must never move the window.
		assert.Equal(t, 2, scrollOffsetFor(ncols, widths, termWidth, 2, 0))
	})

	t.Run("orderKey out of range", func(t *testing.T) {
		// Bounds are checked against the fresh result's column count: after a view switch
		// config.view.Ncols and s.Result.Ncols can disagree for one frame (issue #99 class).
		assert.Equal(t, 2, scrollOffsetFor(ncols, widths, termWidth, 2, ncols))
		assert.Equal(t, 2, scrollOffsetFor(ncols, widths, termWidth, 2, ncols+5))
		assert.Equal(t, 2, scrollOffsetFor(ncols, widths, termWidth, 2, -1))
	})

	t.Run("empty result", func(t *testing.T) {
		// Zero data rows: Ncols and ColsWidth still come from the aligned headers, so the
		// offset stays computable.
		cfg := makeRenderConfig(6, 10)
		s := makeRenderResult(6, 0)

		got := scrollOffsetFor(s.Result.Ncols, cfg.view.ColsWidth, termWidth, 0, 5)
		win := visibleColumns(s.Result.Ncols, cfg.view.ColsWidth, termWidth, got)
		assert.True(t, 5 >= win.first && 5 <= win.last,
			"last column must be brought into view (offset=%d first=%d last=%d)", got, win.first, win.last)
	})

	t.Run("terminal narrower than frozen column", func(t *testing.T) {
		// No offset can admit any scrollable column (the frozen column alone overflows the
		// terminal): leave the window alone instead of looping or landing on an edge.
		assert.Equal(t, 2, scrollOffsetFor(ncols, widths, 5, 2, 6))
		assert.Equal(t, 2, scrollOffsetFor(ncols, widths, 5, 2, 1))
	})
}

// Test_renderDbstat_autoScrollConsumesFlag verifies that a pending auto-scroll request is
// consumed by a single render: the window moves so the sort column becomes visible, and the
// flag is left cleared.
func Test_renderDbstat_autoScrollConsumesFlag(t *testing.T) {
	cfg := makeRenderConfig(7, 10)
	cfg.scrollOffset = 0
	cfg.view.OrderKey = 6 // last column, far outside the window at offset 0
	cfg.autoScrollToOrderKey = true
	s := makeRenderResult(7, 2)

	var buf bytes.Buffer
	assert.NoError(t, renderDbstat(&buf, cfg, s, 40))

	assert.False(t, cfg.autoScrollToOrderKey, "the request must be consumed by the render")
	assert.Contains(t, buf.String(), "col6", "the sort column header must be visible after auto-scroll")
	assert.Greater(t, cfg.scrollOffset, 0, "the window must have moved towards the sort column")
}

// Test_renderDbstat_autoScrollIsOneShot verifies the request fires exactly once: after the
// consuming render, a manual [ / ] scroll is never undone by the next refresh.
func Test_renderDbstat_autoScrollIsOneShot(t *testing.T) {
	cfg := makeRenderConfig(7, 10)
	cfg.scrollOffset = 0
	cfg.view.OrderKey = 6
	cfg.autoScrollToOrderKey = true
	s := makeRenderResult(7, 2)

	var buf bytes.Buffer
	assert.NoError(t, renderDbstat(&buf, cfg, s, 40))
	assert.Greater(t, cfg.scrollOffset, 0)

	// The user manually scrolls back to the left edge; the next refresh must keep it there.
	cfg.scrollOffset = 0
	buf.Reset()
	assert.NoError(t, renderDbstat(&buf, cfg, s, 40))

	assert.Equal(t, 0, cfg.scrollOffset, "manual scroll must survive the next refresh")
	assert.False(t, cfg.autoScrollToOrderKey)
	assert.NotContains(t, buf.String(), "col6", "the sort column must stay off-window after manual scroll")
}

// Test_firstTickCollectingHint pins the cmdline first-tick hint logic: while the collector's
// first-tick flag is set (propagated via Stat.System.VerboseFirstTick), the cmdline shows
// "collecting..."; after the first successful refresh (flag cleared) the hint goes away. Because
// the flag re-arms on every verbose OFF->ON re-enable (Task 7), the hint reappears on re-enable —
// not only after a screen switch.
func Test_firstTickCollectingHint(t *testing.T) {
	// First verbose tick: flag set -> hint shown.
	msg, show := firstTickHint(stat.Stat{System: stat.System{VerboseFirstTick: true}})
	assert.True(t, show, "hint must show while first-tick flag is set")
	assert.Equal(t, "collecting...", msg)

	// After first successful refresh: flag cleared -> hint not shown.
	_, show = firstTickHint(stat.Stat{System: stat.System{VerboseFirstTick: false}})
	assert.False(t, show, "hint must clear after first successful refresh")

	// Re-armed first tick after OFF->ON re-enable: flag set again -> hint reappears.
	msg, show = firstTickHint(stat.Stat{System: stat.System{VerboseFirstTick: true}})
	assert.True(t, show, "hint must reappear on a re-armed first tick (OFF->ON re-enable)")
	assert.Equal(t, "collecting...", msg)
}

// logtailHeader builds the panel's header line the way renderLogtail must write it. It exists so the
// three tests below spell the escape sequences out once: the bytes are a hard requirement (the panel
// draws the highlighted path through gocui's own escape handling), and a test that re-derived them
// from the implementation would assert nothing.
func logtailHeader(path string) string {
	return "\033[30;47m" + path + ":\033[0m\n"
}

// Test_renderLogtail_outputUnchanged pins the rendered bytes of the log panel: the highlighted path
// header followed by the raw buffer, and nothing else. printLogtail is now a thin *gocui.View
// wrapper over this core (the printSysstat -> renderSysstat precedent), so this is what both the
// live path and the repaint path put on screen.
func Test_renderLogtail_outputUnchanged(t *testing.T) {
	var out bytes.Buffer

	assert.NoError(t, renderLogtail(&out, "/var/log/postgresql/A.log", []byte("line1\nline2\n")))
	assert.Equal(t, logtailHeader("/var/log/postgresql/A.log")+"line1\nline2\n", out.String())
}

// Test_renderLogtail_emptyBufferPrintsNothing pins the other half of the pre-existing
// `if len(string(buf)) > 0` guard: on a quiet log readLogfileRecent returns a nil buffer, and the
// panel must then be left completely alone - not even a header line.
//
// The wrapper's half of that guard - that printLogtail does not v.Clear() either - needs a real
// *gocui.View and stays a review-by-inspection item. It is deliberately NOT faked with a nil
// *gocui.View here: a nil one wrapped in an io.Writer is non-nil at the interface level and would
// panic on the first write instead of being skipped.
func Test_renderLogtail_emptyBufferPrintsNothing(t *testing.T) {
	testcases := []struct {
		name string
		buf  []byte
	}{
		{name: "nil buffer", buf: nil},
		{name: "empty buffer", buf: []byte{}},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer

			assert.NoError(t, renderLogtail(&out, "/var/log/postgresql/A.log", tc.buf))
			assert.Equal(t, 0, out.Len(), "a quiet log must produce no bytes at all")
		})
	}
}

// Test_selectLogtail_repaintNeverInvokesTheRead is the strictest statement of Decision 7 available
// without a terminal: on the repaint path the whole file-reading thunk is not entered at all.
//
// The read closure fails the test the moment it is called. That is the assertion - everything else
// here is a consequence of it. This is the test that goes red when the routing regresses, i.e. when
// `if !p.fromFile` in selectLogtail is deleted, inverted or short-circuited; renderFrame's switch
// itself cannot be driven from a unit test, because a *gocui.Gui/*gocui.View cannot be constructed
// outside the gocui package, which is precisely why the selection was extracted out of it.
func Test_selectLogtail_repaintNeverInvokesTheRead(t *testing.T) {
	f := frameStore{logPath: "/var/log/postgresql/A.log", logBuf: []byte("line1\nline2\n")}

	path, buf, err := selectLogtail(storedRender(&f), func() (string, []byte, error) {
		t.Fatal("the repaint path must not touch the log file: no os.Stat, no Read, no Reopen, no Size write")
		return "", nil, nil
	})

	assert.NoError(t, err)
	assert.Equal(t, "/var/log/postgresql/A.log", path)
	assert.Equal(t, []byte("line1\nline2\n"), buf)
}

// Test_selectLogtail_livePathReadsTheFile is what keeps the test above honest: without it, a
// selectLogtail that never reads on either path would satisfy the repaint assertion perfectly and
// break the live panel completely. The live path must invoke the read exactly once and return what
// it produced, ignoring whatever the params happen to carry.
func Test_selectLogtail_livePathReadsTheFile(t *testing.T) {
	calls := 0

	// Live params deliberately built on a store that holds a DIFFERENT pair, so returning the stored
	// one instead of the read's result would be visible.
	p := liveRender(testRenderTime)
	p.logPath, p.logBuf = "/var/log/postgresql/stale.log", []byte("stale\n")

	path, buf, err := selectLogtail(p, func() (string, []byte, error) {
		calls++
		return "/var/log/postgresql/live.log", []byte("live1\nlive2\n"), nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, calls, "the live path must read the log file exactly once per frame")
	assert.Equal(t, "/var/log/postgresql/live.log", path)
	assert.Equal(t, []byte("live1\nlive2\n"), buf)
}

// Test_selectLogtail_livePathPropagatesReadError pins that extracting the read into a thunk did not
// swallow its error: renderFrame returns it, which on the live path is what surfaces an unreadable
// log. The repaint path has no error of that kind to report and adds no second cmdline write.
func Test_selectLogtail_livePathPropagatesReadError(t *testing.T) {
	boom := errors.New("tail postgres log failed")

	_, _, err := selectLogtail(liveRender(testRenderTime), func() (string, []byte, error) {
		return "", nil, boom
	})

	assert.ErrorIs(t, err, boom)
}

// Test_repaintLogtail_noFileAccess is the same freeze property proved by effect rather than by
// non-invocation: it hands selectLogtail a read thunk that REALLY reads a real file, and asserts
// that nothing of that file reaches the screen or the size bookkeeping.
//
// The adversary is deliberately a WORKING file, not a broken one. config.logtail points at a real,
// readable, OPENED file whose content differs from the stored buffer, and its sentinel Size is 1 -
// SMALLER than the file. Both properties are load-bearing: a smaller sentinel keeps the "unchanged
// file" early return from firing AND keeps the rotation branch (size < logtail.Size) shut, so a
// routing regression here SUCCEEDS and renders the adversary's bytes under the adversary's path
// instead of dying for an unrelated reason. That is what makes every assertion below discriminate:
// invert the routing and the path/buffer, the rendered bytes and the frozen Size all change at once.
//
// The thunk mirrors the live branch's file work (os.Stat, Logfile.Read, the Size write) minus the
// two gocui-bound bits - v.Size()-derived limits and Reopen's database round-trip - which is the
// most a unit test can execute of that branch.
//
// The exact bytes of a rendered pair are NOT re-asserted here; Test_renderLogtail_outputUnchanged
// owns that contract. What this test owns is which pair the repaint selects.
func Test_repaintLogtail_noFileAccess(t *testing.T) {
	dir := t.TempDir()
	adversaryPath := filepath.Join(dir, "adversary.log")
	assert.NoError(t, os.WriteFile(adversaryPath, []byte("ADVERSARY LINE - MUST NOT APPEAR\n"), 0o600))

	logfile := stat.Logfile{Path: adversaryPath, Size: 1}
	assert.NoError(t, logfile.Open())
	t.Cleanup(func() { _ = logfile.Close() })

	app := &app{config: newConfig()}
	app.config.logtail = logfile

	// The frozen frame carries a different pair: another file, another content.
	app.frame.logPath = "/var/log/postgresql/A.log"
	app.frame.logBuf = []byte("line1\nline2\n")
	app.frame.valid = true

	read := func() (string, []byte, error) {
		info, err := os.Stat(app.config.logtail.Path)
		if err != nil {
			return "", nil, err
		}

		buf, err := app.config.logtail.Read(4, 128)
		if err != nil {
			return "", nil, err
		}

		app.config.logtail.Size = info.Size()

		return app.config.logtail.Path, buf, nil
	}

	var path string
	var buf []byte
	var err error
	assert.NotPanics(t, func() {
		path, buf, err = selectLogtail(storedRender(&app.frame), read)
	})
	assert.NoError(t, err)

	assert.Equal(t, "/var/log/postgresql/A.log", path, "the repaint must draw the stored header path")
	assert.Equal(t, []byte("line1\nline2\n"), buf, "the repaint must draw the stored buffer")

	var out bytes.Buffer
	assert.NoError(t, renderLogtail(&out, path, buf))
	assert.NotContains(t, out.String(), "ADVERSARY", "the repaint must not put the log file's content on screen")
	assert.NotContains(t, out.String(), dir, "the repaint must not put the live logtail path in the header")

	assert.Equal(t, int64(1), app.config.logtail.Size, "the repaint must not advance the logfile size bookkeeping")
}
