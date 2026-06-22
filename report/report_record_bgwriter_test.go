package report

import (
	"archive/tar"
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/stretchr/testify/assert"
)

// Test_app_doReport_bgwriter exercises the full doReport pipeline for the
// version-aware bgwriter report against a synthetic in-memory tar. Each
// subcase pins one recorded PostgreSQL version (14/17/18) and feeds a tar of
// two cumulative ticks plus a meta record whose version_num drives report-time
// view.Configure (report.go:250-252). The first tick is discarded by
// processData as the prev snapshot; the second becomes curr and produces the
// data row. countDiff subtracts prev from curr in the DiffIntvl columns while
// copying everything else (absolute event counters, the stats_age text column)
// verbatim from the curr tick.
//
// The two ticks are exactly one second apart, so the rate divisor itv == 1 and
// each diffed column equals tick2 - tick1 with no scaling. The output is
// compared against a per-version golden so the version-aware layout switch is
// proven without a live PostgreSQL.
func Test_app_doReport_Bgwriter(t *testing.T) {
	testcases := []struct {
		name       string
		versionNum string
		versionStr string
		cols       []string
		// prevVals / currVals are the cumulative values for the two ticks in
		// column order. The diffed columns must grow from prev to curr; the
		// absolute / text columns are taken from curr verbatim.
		prevVals []string
		currVals []string
		wantFile string
	}{
		{
			// PG14-16: 12 cols, DiffIntvl [3,10]. Absolute: 0 source, 1-2
			// ckpt counters, 11 stats_age. Diffed: 3..10.
			name:       "pg14",
			versionNum: "140009",
			versionStr: "14.9",
			cols: []string{
				"source", "ckpt_timed", "ckpt_req",
				"ckpt_write,ms", "ckpt_sync,ms",
				"buf_ckpt", "buf_clean", "maxwritten",
				"buf_backend", "buf_backend_fsync", "buf_alloc",
				"stats_age",
			},
			prevVals: []string{
				"Bgwriter", "10", "2",
				"100.5", "20.5",
				"1000", "500", "5",
				"300", "1", "2000",
				"01:00:00",
			},
			currVals: []string{
				"Bgwriter", "11", "3",
				"150.5", "30.5",
				"1500", "700", "8",
				"450", "2", "3000",
				"02:00:00",
			},
			wantFile: "testdata/report_record_bgwriter_pg14.golden",
		},
		{
			// PG17: 13 cols, DiffIntvl [6,11]. Absolute: 0 source, 1-5
			// ckpt/rstpt counters, 12 stats_age. Diffed: 6..11.
			name:       "pg17",
			versionNum: "170001",
			versionStr: "17.1",
			cols: []string{
				"source", "ckpt_timed", "ckpt_req",
				"rstpt_timed", "rstpt_req", "rstpt_done",
				"ckpt_write,ms", "ckpt_sync,ms",
				"buf_ckpt", "buf_clean", "maxwritten", "buf_alloc",
				"stats_age",
			},
			prevVals: []string{
				"Bgwriter", "10", "2",
				"4", "1", "3",
				"100.5", "20.5",
				"1000", "500", "5", "2000",
				"01:00:00",
			},
			currVals: []string{
				"Bgwriter", "11", "3",
				"5", "2", "4",
				"150.5", "30.5",
				"1500", "700", "8", "3000",
				"02:00:00",
			},
			wantFile: "testdata/report_record_bgwriter_pg17.golden",
		},
		{
			// PG18+: 14 cols, DiffIntvl [6,12]. Absolute: 0 source, 1-5
			// ckpt/rstpt counters, 13 stats_age. Diffed: 6..12 (incl. the
			// new slru_written at index 9).
			name:       "pg18",
			versionNum: "180000",
			versionStr: "18.0",
			cols: []string{
				"source", "ckpt_timed", "ckpt_req",
				"rstpt_timed", "rstpt_req", "rstpt_done",
				"ckpt_write,ms", "ckpt_sync,ms",
				"buf_ckpt", "slru_written", "buf_clean", "maxwritten", "buf_alloc",
				"stats_age",
			},
			prevVals: []string{
				"Bgwriter", "10", "2",
				"4", "1", "3",
				"100.5", "20.5",
				"1000", "50", "500", "5", "2000",
				"01:00:00",
			},
			currVals: []string{
				"Bgwriter", "11", "3",
				"5", "2", "4",
				"150.5", "30.5",
				"1500", "90", "700", "8", "3000",
				"02:00:00",
			},
			wantFile: "testdata/report_record_bgwriter_pg18.golden",
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
			// (!prevStat.Valid -> continue). UniqueKey defaults to 0 (the
			// constant "Bgwriter" source), so the single row pairs with curr.
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
			// tarRecorder.write(): meta + bgwriter + sysinfo). The timestamp
			// in each filename uses the recorder's 20060102T150405.000 format
			// and the two ticks are one second apart so itv == 1.
			var tarBuf bytes.Buffer
			tw := tar.NewWriter(&tarBuf)
			writeEntry := func(name string, payload []byte) {
				hdr := &tar.Header{Name: name, Size: int64(len(payload)), Mode: 0644}
				assert.NoError(t, tw.WriteHeader(hdr))
				_, err := tw.Write(payload)
				assert.NoError(t, err)
			}
			writeEntry("meta.20260519T100000.000.json", metaBytes)
			writeEntry("bgwriter.20260519T100000.000.json", prevBytes)
			writeEntry("sysinfo.20260519T100000.000.json", sysinfoBytes)
			writeEntry("meta.20260519T100001.000.json", metaBytes)
			writeEntry("bgwriter.20260519T100001.000.json", currBytes)
			writeEntry("sysinfo.20260519T100001.000.json", sysinfoBytes)
			assert.NoError(t, tw.Close())

			config := Config{
				ReportType: "bgwriter",
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
			// Pin the data row independently of the golden (mirrors the
			// procpidstat harness, report_test.go:710-715): the source value
			// localizes a future failure to "row missing" vs "header only".
			// The diffed buf_ckpt delta (1500-1000=500) is identical across
			// all three versions, doubling as a cheap delta sentinel.
			assert.Contains(t, out, "Bgwriter")
			assert.Contains(t, out, "500")

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
