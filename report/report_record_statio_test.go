package report

import (
	"archive/tar"
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/stretchr/testify/assert"
)

// statIOCols is the canonical 16-column layout of the stat_io count report,
// matching internal/query/io.go (SelectStatIOQuery returns Ncols=16, DiffIntvl
// [4,14] for both PG16/17 and PG18). The PG18 branch only changes how the KiB
// columns are derived in SQL (op_bytes vs native *_bytes counters); the column
// shape is identical, so the same Cols slice describes a recorded sample for any
// version. Columns 4..14 (reads..fsyncs) are diffed; io_key + dimensions (0..3)
// and stats_age (15) pass through verbatim.
var statIOCols = []string{
	"io_key", "backend_type", "object", "context",
	"reads", "read,KiB", "writes", "write,KiB", "extends", "ext,KiB",
	"hits", "evictions", "writebacks", "reuses", "fsyncs",
	"stats_age",
}

// statIOTimeCols is the canonical 10-column layout of the stat_io_time report,
// matching internal/query/io.go (SelectStatIOTimeQuery returns Ncols=10,
// DiffIntvl [4,8], version-independent). Columns 4..8 (read_time..fsync_time)
// are diffed; io_key + dimensions (0..3) and stats_age (9) pass through.
var statIOTimeCols = []string{
	"io_key", "backend_type", "object", "context",
	"read_time", "write_time", "writeback_time", "extend_time", "fsync_time",
	"stats_age",
}

// statIOAnsiRE matches the SGR color escapes printStatHeader/printStatSample
// wrap around cells so golden output can be normalized for value assertions.
var statIOAnsiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

// statIOStripANSI removes terminal color escapes from report output.
func statIOStripANSI(s string) string { return statIOAnsiRE.ReplaceAllString(s, "") }

// Test_app_doReport_StatIO_v16 and _v18 exercise the full doReport pipeline for
// the version-aware stat_io count report against a synthetic in-memory tar. Each
// test pins one recorded PostgreSQL version (16 / 18) via the meta record's
// version_num, which drives report-time view.Configure → SelectStatIOQuery.
//
// Because report replays recorded JSON and never runs SQL, the v16↔v18 OUTPUT
// divergence here is a property of the hand-built synthetic fixtures, not of the
// selector switch: both SelectStatIOQuery branches return the identical shape
// (Ncols=16, DiffIntvl [4,14]), so this replay test would not fail if Configure
// picked the wrong query for a given version_num. Selector-branch correctness is
// asserted elsewhere (internal/view view_test.go pins the version-aware Configure
// map; SelectStatIOQuery is unit-testable in internal/query). What this test pins
// is that the replay engine diffs each recorded sample cleanly under its recorded
// version. The v16 and v18 samples are deliberately made distinct: v18 carries
// different counter/KiB values AND an extra object='wal' row (a PG18-only
// dimension), so each golden is genuinely meaningful rather than a byte-identical
// copy of the other.
//
// The two ticks are exactly one second apart so the rate divisor itv == 1 and
// each diffed column equals tick2 - tick1 with no scaling. Rows pair across
// snapshots by io_key (UniqueKey 0). Selected diffed cells are coalesced "0" in
// both ticks (mirroring pg_stat_io NULLs: reads for background writer, fsyncs for
// the WAL/bulkread rows) — they must diff to a clean "0" without aborting the
// sample (an empty string there would crash diffPair → ParseInt("")).
func Test_app_doReport_StatIO_v16(t *testing.T) {
	// io_key values are arbitrary-but-stable 10-char strings; identity across
	// ticks is what matters for UniqueKey-0 pairing (the value is never parsed
	// as a number). View OrderKey=4 (reads) DESC governs printed row order.
	//
	// client_norm: a normal client-backend relation row; all diffed counters
	//   grow.  reads 1000->1500 (=500) drives it to the top by OrderKey 4.
	// bgwriter_z: background-writer row — reads/read,KiB/hits/evictions are
	//   coalesced "0" in both ticks (a bgwriter never reads/hits), exercising
	//   the zero-cell diff; only writes/write,KiB/writebacks grow.
	clientPrev := []string{"k_client01", "client backend", "relation", "normal",
		"1000", "8000", "200", "1600", "50", "400", "9000", "10", "5", "3", "2", "01:00:00"}
	clientCurr := []string{"k_client01", "client backend", "relation", "normal",
		"1500", "12000", "260", "2080", "70", "560", "9600", "14", "8", "5", "4", "02:00:00"}
	bgwPrev := []string{"k_bgwrit01", "background writer", "relation", "normal",
		"0", "0", "300", "2400", "0", "0", "0", "0", "40", "0", "0", "01:00:00"}
	bgwCurr := []string{"k_bgwrit01", "background writer", "relation", "normal",
		"0", "0", "360", "2880", "0", "0", "0", "0", "55", "0", "0", "02:00:00"}

	runStatIOReplay(t, statIOReplayCase{
		reportType: "stat_io",
		ncols:      16,
		versionNum: "160000",
		versionStr: "16.4",
		cols:       statIOCols,
		// prev/curr row sets carry the SAME io_keys (pairing by UniqueKey 0).
		prevRows:  [][]string{clientPrev, bgwPrev},
		currRows:  [][]string{clientCurr, bgwCurr},
		wantFile:  "testdata/report_record_stat_io_v16.golden",
		entryName: "stat_io",
		// Expected normalized data rows (ANSI-stripped, whitespace-collapsed),
		// in OrderKey=4 (reads) DESC order: client (reads delta 500) first,
		// bgwriter (reads delta 0) second.
		//   client diffs cols 4..14:
		//     reads 1500-1000=500   read,KiB 12000-8000=4000
		//     writes 260-200=60     write,KiB 2080-1600=480
		//     extends 70-50=20      ext,KiB 560-400=160
		//     hits 9600-9000=600    evictions 14-10=4
		//     writebacks 8-5=3      reuses 5-3=2   fsyncs 4-2=2
		//   bgwriter coalesced-zero cells (reads,read,KiB,extends,ext,KiB,hits,
		//     evictions,reuses,fsyncs) all diff to 0; writes 360-300=60,
		//     write,KiB 2880-2400=480, writebacks 55-40=15.
		wantRows: []string{
			"k_client01 client backend relation normal 500 4000 60 480 20 160 600 4 3 2 2",
			"k_bgwrit01 background writer relation normal 0 0 60 480 0 0 0 0 15 0 0",
		},
	})
}

func Test_app_doReport_StatIO_v18(t *testing.T) {
	// v18 sample is intentionally distinct from v16: different counter/KiB
	// values AND an extra object='wal' row (PG18 emits WAL I/O via pg_stat_io).
	clientPrev := []string{"k_client01", "client backend", "relation", "normal",
		"2000", "16000", "400", "3200", "100", "800", "18000", "20", "10", "6", "4", "01:00:00"}
	clientCurr := []string{"k_client01", "client backend", "relation", "normal",
		"2700", "21600", "490", "3920", "130", "1040", "18900", "26", "16", "10", "7", "02:00:00"}
	bgwPrev := []string{"k_bgwrit01", "background writer", "relation", "normal",
		"0", "0", "600", "4800", "0", "0", "0", "0", "80", "0", "0", "01:00:00"}
	bgwCurr := []string{"k_bgwrit01", "background writer", "relation", "normal",
		"0", "0", "690", "5520", "0", "0", "0", "0", "110", "0", "0", "02:00:00"}
	// PG18-only WAL row: writes/extends grow; reads/hits/evictions/reuses/fsyncs
	// are coalesced "0" (WAL I/O has no reads/hits) — another zero-cell case.
	walPrev := []string{"k_wal00001", "walwriter", "wal", "normal",
		"0", "0", "5000", "40000", "1000", "8000", "0", "0", "0", "0", "0", "01:00:00"}
	walCurr := []string{"k_wal00001", "walwriter", "wal", "normal",
		"0", "0", "5800", "46400", "1200", "9600", "0", "0", "0", "0", "0", "02:00:00"}

	runStatIOReplay(t, statIOReplayCase{
		reportType: "stat_io",
		ncols:      16,
		versionNum: "180000",
		versionStr: "18.0",
		cols:       statIOCols,
		prevRows:   [][]string{clientPrev, bgwPrev, walPrev},
		currRows:   [][]string{clientCurr, bgwCurr, walCurr},
		wantFile:   "testdata/report_record_stat_io_v18.golden",
		entryName:  "stat_io",
		// OrderKey=4 (reads) DESC: client (reads delta 700) first; bgwriter and
		// wal both have reads delta 0 — their relative order is the stable-sort
		// order of equal keys (input curr order [client,bgwriter,wal] preserved by
		// SliceStable). The golden pins the exact order; orderChecks additionally
		// guards the bgwriter-before-wal tie independently so a stable-sort
		// regression is caught even if the golden is regenerated.
		//   client diffs cols 4..14:
		//     reads 2700-2000=700   read,KiB 21600-16000=5600
		//     writes 490-400=90     write,KiB 3920-3200=720
		//     extends 130-100=30    ext,KiB 1040-800=240
		//     hits 18900-18000=900  evictions 26-20=6
		//     writebacks 16-10=6    reuses 10-6=4   fsyncs 7-4=3
		//   bgwriter: writes 90, write,KiB 720, writebacks 30; rest 0.
		//   wal: writes 800, write,KiB 6400, extends 200, ext,KiB 1600; rest 0.
		wantRows: []string{
			"k_client01 client backend relation normal 700 5600 90 720 30 240 900 6 6 4 3",
			"k_bgwrit01 background writer relation normal 0 0 90 720 0 0 0 0 30 0 0",
			"k_wal00001 walwriter wal normal 0 0 800 6400 200 1600 0 0 0 0 0",
		},
		// Tie-break order guard, independent of the golden: among the two
		// reads-delta-0 rows, bgwriter must print before wal (stable sort of equal
		// OrderKey-4 keys preserves curr input order).
		orderChecks: [][2]string{{"k_bgwrit01", "k_wal00001"}},
	})
}

// Test_app_doReport_StatIOTime exercises the full doReport pipeline for the
// version-independent stat_io_time report. SelectStatIOTimeQuery ignores the PG
// version (timing columns are schema-stable across PG16/17/18), so one recorded
// version (16) suffices. Layout: Ncols=10, DiffIntvl [4,8], UniqueKey 0.
//
// Rows pair across the two cumulative ticks by io_key (UniqueKey 0); the diffed
// timing block (cols 4..8) subtracts correctly. The background-writer row has
// read_time/fsync_time coalesced "0" (track_io_timing off for those ops) in both
// ticks — they diff to a clean "0" without blanking the row.
func Test_app_doReport_StatIOTime(t *testing.T) {
	// client_norm: all timing counters grow; read_time drives OrderKey 4 DESC.
	// bgwriter_z: read_time/fsync_time coalesced "0" in both ticks (zero-cell).
	clientPrev := []string{"k_client01", "client backend", "relation", "normal",
		"1000", "500", "100", "200", "50", "01:00:00"}
	clientCurr := []string{"k_client01", "client backend", "relation", "normal",
		"1600", "800", "160", "280", "75", "02:00:00"}
	bgwPrev := []string{"k_bgwrit01", "background writer", "relation", "normal",
		"0", "300", "60", "120", "0", "01:00:00"}
	bgwCurr := []string{"k_bgwrit01", "background writer", "relation", "normal",
		"0", "420", "90", "160", "0", "02:00:00"}

	runStatIOReplay(t, statIOReplayCase{
		reportType: "stat_io_time",
		ncols:      10,
		versionNum: "160000",
		versionStr: "16.4",
		cols:       statIOTimeCols,
		prevRows:   [][]string{clientPrev, bgwPrev},
		currRows:   [][]string{clientCurr, bgwCurr},
		wantFile:   "testdata/report_record_stat_io_time.golden",
		entryName:  "stat_io_time",
		// OrderKey=4 (read_time) DESC: client (read_time delta 600) first,
		// bgwriter (read_time delta 0) second.
		//   client diffs cols 4..8:
		//     read_time 1600-1000=600   write_time 800-500=300
		//     writeback_time 160-100=60 extend_time 280-200=80
		//     fsync_time 75-50=25
		//   bgwriter: read_time/fsync_time coalesced-zero → 0;
		//     write_time 420-300=120  writeback_time 90-60=30  extend_time 160-120=40
		wantRows: []string{
			"k_client01 client backend relation normal 600 300 60 80 25",
			"k_bgwrit01 background writer relation normal 0 120 30 40 0",
		},
	})
}

// statIOReplayCase describes one stat_io / stat_io_time replay scenario.
type statIOReplayCase struct {
	reportType string
	ncols      int
	versionNum string
	versionStr string
	cols       []string
	prevRows   [][]string
	currRows   [][]string
	wantFile   string
	entryName  string
	wantRows   []string
	// orderChecks pins relative print order of row pairs ({before, after})
	// independently of the golden — used for OrderKey ties the wantRows
	// Contains-assertions cannot express.
	orderChecks [][2]string
}

// runStatIOReplay builds a synthetic two-tick tar from the case, runs the full
// doReport pipeline, asserts the computed data rows independently of the golden,
// then compares (or updates) the golden file.
func runStatIOReplay(t *testing.T, tc statIOReplayCase) {
	t.Helper()

	// Meta result mirrors SelectCommonProperties (7-column shape; readMeta only
	// consumes column index 1 for version_num, which drives the version-aware
	// view.Configure at report time).
	metaRes := stat.PGresult{
		Valid: true, Ncols: 7, Nrows: 1,
		Cols: []string{"version", "version_num", "track_commit_timestamp", "max_connections", "autovacuum_max_workers", "recovery", "start_time_unix"},
		Values: [][]sql.NullString{
			{
				{String: tc.versionStr, Valid: true}, {String: tc.versionNum, Valid: true},
				{String: "off", Valid: true}, {String: "100", Valid: true}, {String: "3", Valid: true},
				{String: "false", Valid: true}, {String: "1622828486655396e-6", Valid: true},
			},
		},
	}
	metaBytes, err := json.Marshal(metaRes)
	assert.NoError(t, err)

	mkRow := func(vals []string) []sql.NullString {
		row := make([]sql.NullString, tc.ncols)
		for i, v := range vals {
			row[i] = sql.NullString{String: v, Valid: true}
		}
		return row
	}
	mkResult := func(rows [][]string) []byte {
		vals := make([][]sql.NullString, len(rows))
		for i, r := range rows {
			vals[i] = mkRow(r)
		}
		res := stat.PGresult{
			Valid: true, Ncols: tc.ncols, Nrows: len(rows), Cols: tc.cols, Values: vals,
		}
		b, e := json.Marshal(res)
		assert.NoError(t, e)
		return b
	}

	// Tick 1 (prev) is discarded by processData's first-snapshot rule
	// (!prevStat.Valid -> continue); tick 2 (curr) produces the data rows.
	// prev carries the same io_keys as curr, listed in the OPPOSITE order, so
	// the test actually exercises io_key pairing (UniqueKey 0): diff() matches
	// curr rows to prev rows by io_key, not by position.
	prevReversed := make([][]string, len(tc.prevRows))
	for i, r := range tc.prevRows {
		prevReversed[len(tc.prevRows)-1-i] = r
	}
	prevBytes := mkResult(prevReversed)
	currBytes := mkResult(tc.currRows)

	sysinfoBytes := []byte(`{"ticks":100,"cpu_count":4}`)

	// Compose tar (two ticks; per-tick layout matches tarRecorder.write(): meta
	// + <report> + sysinfo). Filenames use the recorder's 20060102T150405.000
	// format and the two ticks are one second apart so itv == 1.
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	writeEntry := func(name string, payload []byte) {
		hdr := &tar.Header{Name: name, Size: int64(len(payload)), Mode: 0644}
		assert.NoError(t, tw.WriteHeader(hdr))
		_, e := tw.Write(payload)
		assert.NoError(t, e)
	}
	writeEntry("meta.20260519T100000.000.json", metaBytes)
	writeEntry(tc.entryName+".20260519T100000.000.json", prevBytes)
	writeEntry("sysinfo.20260519T100000.000.json", sysinfoBytes)
	writeEntry("meta.20260519T100001.000.json", metaBytes)
	writeEntry(tc.entryName+".20260519T100001.000.json", currBytes)
	writeEntry("sysinfo.20260519T100001.000.json", sysinfoBytes)
	assert.NoError(t, tw.Close())

	// Note: OrderColName is deliberately unset so the view's OrderKey=4 DESC
	// default sort governs row order.
	config := Config{
		ReportType: tc.reportType,
		TruncLimit: 32,
		TsStart:    time.Date(2026, 5, 19, 0, 0, 0, 0, time.Now().Location()),
		TsEnd:      time.Date(2026, 5, 19, 23, 59, 59, 0, time.Now().Location()),
	}

	app := newApp(config)
	var buf bytes.Buffer
	app.writer = &buf

	tr := tar.NewReader(&tarBuf)
	assert.NoError(t, app.doReport(tr))

	out := buf.String()
	assert.NotEmpty(t, out)
	// Timestamp header line emitted by printStatSample matches "YYYY/MM/DD".
	assert.Regexp(t, regexp.MustCompile(`\d{4}/\d{2}/\d{2}`), out)

	// Pin the diff math independently of the golden so the deltas are still
	// guarded if the golden is ever regenerated against buggy code. Strip ANSI
	// color codes and collapse whitespace, then assert each expected data row.
	normalized := strings.Join(strings.Fields(statIOStripANSI(out)), " ")
	for _, row := range tc.wantRows {
		assert.Contains(t, normalized, row)
	}
	for _, oc := range tc.orderChecks {
		assert.Less(t, strings.Index(normalized, oc[0]), strings.Index(normalized, oc[1]),
			"%q must print before %q", oc[0], oc[1])
	}

	if *update {
		assert.NoError(t, os.WriteFile(tc.wantFile, buf.Bytes(), 0644))
		return
	}

	want, err := os.ReadFile(tc.wantFile)
	assert.NoError(t, err)
	assert.Equal(t, string(want), out)
}
