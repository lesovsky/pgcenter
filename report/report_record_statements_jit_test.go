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

// Test_app_doReport_StatementsJIT exercises the full doReport pipeline for the
// version-aware statements_jit report against a synthetic in-memory tar. Each
// subcase pins one recorded PostgreSQL version (15/17) and feeds a tar of two
// cumulative ticks plus a meta record whose version_num drives report-time
// view.Configure (report.go:250-252 -> query.SelectStatStatementsJITQuery).
//
// statements_jit is version-aware: PG17+ adds the jit_deform_time/jit_deform_count
// columns, shifting the interval block, functions, queryid and query right by
// one. The recorded layout therefore differs:
//   - PG15: 13 cols, DiffIntvl {6,10}, UniqueKey 11 (trailing md5 queryid).
//   - PG17: 15 cols, DiffIntvl {7,12}, UniqueKey 13 (trailing md5 queryid).
//
// The UniqueKey index shifts with the version, and this test proves that the
// report-time Configure picks the right per-version layout: rows must pair on
// the correct trailing queryid index (11 on v15, 13 on v17) for diff() to
// subtract the phase-time *,ms columns. The first tick is discarded by
// processData as the prev snapshot; the second becomes curr and produces the
// data row. diff() subtracts prev from curr in the DiffIntvl columns (the
// phase-time *,ms columns plus functions) while copying the absolute *_total
// text columns, queryid and query verbatim from curr.
//
// The two ticks are exactly one second apart, so the rate divisor itv == 1 and
// each diffed column equals tick2 - tick1 with no scaling. The output is
// compared against a per-version golden so the version-aware layout switch is
// proven without a live PostgreSQL.
func Test_app_doReport_StatementsJIT(t *testing.T) {
	// Both ticks share the same md5 queryid at the version-specific UniqueKey
	// index so diff() pairs the single row across ticks.
	const queryid = "a1b2c3d4e5"

	// The recorded version_num values 150004/170001 are representative
	// patch-level versions (vs the task's illustrative 150000/170000), chosen
	// to exercise the < PostgresV17 (13-col) and >= PostgresV17 (15-col)
	// branches of query.SelectStatStatementsJITQuery at report time.

	testcases := []struct {
		name       string
		versionNum string
		versionStr string
		cols       []string
		// prevVals / currVals are the cumulative values for the two ticks in
		// column order. The diffed columns (DiffIntvl) must grow from prev to
		// curr; absolute / text columns are taken from curr verbatim.
		prevVals []string
		currVals []string
		wantFile string
	}{
		{
			// PG15: 13 cols, DiffIntvl [6,10]. Absolute: 0 user, 1 database,
			// 2-5 *_total text, 11 queryid (UniqueKey), 12 query. Diffed:
			// 6..10 (gen,ms inline,ms opt,ms emit,ms functions).
			name:       "version_15",
			versionNum: "150004",
			versionStr: "15.4",
			cols: []string{
				"user", "database",
				"gen_total", "inline_total", "opt_total", "emit_total",
				"gen,ms", "inline,ms", "opt,ms", "emit,ms",
				"functions", "queryid", "query",
			},
			prevVals: []string{
				"postgres", "testdb",
				"00:00:01", "00:00:02", "00:00:03", "00:00:04",
				"100", "200", "300", "400",
				"5", queryid, "select * from t",
			},
			currVals: []string{
				"postgres", "testdb",
				"00:00:02", "00:00:04", "00:00:06", "00:00:08",
				"150", "260", "390", "520",
				"9", queryid, "select * from t",
			},
			wantFile: "testdata/report_statements_jit_v15.golden",
		},
		{
			// PG17: 15 cols, DiffIntvl [7,12]. Absolute: 0 user, 1 database,
			// 2-6 *_total text (incl. deform_total), 13 queryid (UniqueKey),
			// 14 query. Diffed: 7..12 (gen,ms inline,ms opt,ms emit,ms
			// deform,ms functions).
			name:       "version_17",
			versionNum: "170001",
			versionStr: "17.1",
			cols: []string{
				"user", "database",
				"gen_total", "inline_total", "opt_total", "emit_total", "deform_total",
				"gen,ms", "inline,ms", "opt,ms", "emit,ms", "deform,ms",
				"functions", "queryid", "query",
			},
			prevVals: []string{
				"postgres", "testdb",
				"00:00:01", "00:00:02", "00:00:03", "00:00:04", "00:00:05",
				"100", "200", "300", "400", "500",
				"5", queryid, "select * from t",
			},
			currVals: []string{
				"postgres", "testdb",
				"00:00:02", "00:00:04", "00:00:06", "00:00:08", "00:00:10",
				"150", "260", "390", "520", "650",
				"9", queryid, "select * from t",
			},
			wantFile: "testdata/report_statements_jit_v17.golden",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ncols := len(tc.cols)

			// Meta result mirrors SelectCommonProperties (7-column shape;
			// readMeta only consumes column index 1 for version_num, which
			// drives the version-aware view.Configure at report time).
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
				row := make([]sql.NullString, ncols)
				for i, v := range vals {
					row[i] = sql.NullString{String: v, Valid: true}
				}
				return row
			}

			// Tick 1 (prev): discarded by processData's first-snapshot rule
			// (!prevStat.Valid -> continue). Its queryid at the UniqueKey
			// index matches curr so diff() pairs the row.
			statPrev := stat.PGresult{
				Valid: true, Ncols: ncols, Nrows: 1, Cols: tc.cols,
				Values: [][]sql.NullString{mkRow(tc.prevVals)},
			}
			prevBytes, err := json.Marshal(statPrev)
			assert.NoError(t, err)

			// Tick 2 (curr): cumulative values larger than tick 1 in the
			// diffed columns; produces the reported data row.
			statCurr := stat.PGresult{
				Valid: true, Ncols: ncols, Nrows: 1, Cols: tc.cols,
				Values: [][]sql.NullString{mkRow(tc.currVals)},
			}
			currBytes, err := json.Marshal(statCurr)
			assert.NoError(t, err)

			sysinfoBytes := []byte(`{"ticks":100,"cpu_count":4}`)

			// Compose tar (two ticks; per-tick layout matches
			// tarRecorder.write(): meta + statements_jit + sysinfo). The
			// timestamp in each filename uses the recorder's
			// 20060102T150405.000 format and the two ticks are one second
			// apart so itv == 1.
			var tarBuf bytes.Buffer
			tw := tar.NewWriter(&tarBuf)
			writeEntry := func(name string, payload []byte) {
				hdr := &tar.Header{Name: name, Size: int64(len(payload)), Mode: 0644}
				assert.NoError(t, tw.WriteHeader(hdr))
				_, err := tw.Write(payload)
				assert.NoError(t, err)
			}
			writeEntry("meta.20260519T100000.000.json", metaBytes)
			writeEntry("statements_jit.20260519T100000.000.json", prevBytes)
			writeEntry("sysinfo.20260519T100000.000.json", sysinfoBytes)
			writeEntry("meta.20260519T100001.000.json", metaBytes)
			writeEntry("statements_jit.20260519T100001.000.json", currBytes)
			writeEntry("sysinfo.20260519T100001.000.json", sysinfoBytes)
			assert.NoError(t, tw.Close())

			config := Config{
				ReportType: "statements_jit",
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

			// Explicit assertions beyond the golden, localizing failures.
			//
			// Isolate the single data row (the line carrying the queryid) so the
			// delta sentinels below can't be satisfied by a substring landing in
			// the header or timestamp line. The presence of this line also proves
			// the row paired on the version-shifted trailing queryid (UniqueKey
			// 11 on v15, 13 on v17) — diff() only emits a diffed row when the
			// UniqueKey cells match across ticks.
			assert.Contains(t, out, queryid)
			var dataLine string
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, queryid) {
					dataLine = line
					break
				}
			}
			assert.NotEmpty(t, dataLine, "data row with queryid must be present")

			// Interval phase-time *,ms deltas are curr - prev (itv == 1, no
			// scaling), pinned as a column-anchored token sequence so a value
			// shifted into the wrong column would fail. Shared phases across
			// both versions: gen 150-100=50, inline 260-200=60, opt 390-300=90,
			// emit 520-400=120; functions 9-5=4 (DiffIntvl includes functions).
			assert.Regexp(t, regexp.MustCompile(`\b50\s+60\s+90\s+120\b`), dataLine)
			assert.Regexp(t, regexp.MustCompile(`\b4\b\s+`+queryid), dataLine)

			// Version-specific layout: the deform column exists only on PG17+.
			if tc.versionNum == "170001" {
				// Header carries the deform_total / deform,ms column labels.
				assert.Contains(t, out, "deform,ms")
				// deform,ms delta = 650-500 = 150, sitting between emit (120)
				// and functions (4) in the diffed block of the data row.
				assert.Regexp(t, regexp.MustCompile(`\b120\s+150\s+4\b`), dataLine)
			} else {
				assert.NotContains(t, out, "deform", "PG15 layout must not contain deform column")
			}

			if *update {
				assert.NoError(t, os.WriteFile(tc.wantFile, buf.Bytes(), 0644))
				return
			}

			want, err := os.ReadFile(tc.wantFile)
			assert.NoError(t, err)
			assert.Equal(t, string(want), out)
		})
	}
}
