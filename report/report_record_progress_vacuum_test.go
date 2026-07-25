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

// Test_app_doReport_ProgressVacuum proves the vacuum progress layout is chosen by the version recorded in
// the archive rather than by the running server.
//
// This is the guard for the one field that fails silently instead of loudly. Printing and alignment walk
// the recorded result's own column count, so a stale layout cannot panic — but a stale DiffIntvl diffs the
// wrong pair of columns. On the PG 19 layout the pre-19 interval {10,11} lands on scanned_total,% and
// vacuumed_total,%, whose values parse as numbers, so the wrong diff succeeds and prints plausible nonsense.
func Test_app_doReport_ProgressVacuum(t *testing.T) {
	testcases := []struct {
		name       string
		versionNum string
		versionStr string
		cols       []string
		prevVals   []string
		currVals   []string
		wantFile   string
	}{
		{
			// Pre-19: 13 cols, DiffIntvl {10,11} — scanned,KiB and vacuumed,KiB.
			name:       "pg18",
			versionNum: "180004",
			versionStr: "18.4",
			cols: []string{
				"pid", "xact_age", "datname", "relation", "state", "waiting", "phase",
				"size_total,KiB", "scanned_total,%", "vacuumed_total,%",
				"scanned,KiB", "vacuumed,KiB", "query",
			},
			prevVals: []string{
				"12345", "00:05:00", "pgcenter_fixtures", "public.pgbench_accounts", "active", "f", "scanning heap",
				"8192", "50.00", "25.00",
				"1000", "2000", "autovacuum: VACUUM public.pgbench_accounts",
			},
			currVals: []string{
				"12345", "00:05:01", "pgcenter_fixtures", "public.pgbench_accounts", "active", "f", "scanning heap",
				"8192", "50.00", "25.00",
				"1700", "2900", "autovacuum: VACUUM public.pgbench_accounts",
			},
			wantFile: "testdata/report_record_progress_vacuum_pg18.golden",
		},
		{
			// PG 19: 15 cols, started_by and mode inserted before state, DiffIntvl {12,13}.
			name:       "pg19",
			versionNum: "190000",
			versionStr: "19beta2",
			cols: []string{
				"pid", "xact_age", "datname", "relation", "started_by", "mode", "state", "waiting", "phase",
				"size_total,KiB", "scanned_total,%", "vacuumed_total,%",
				"scanned,KiB", "vacuumed,KiB", "query",
			},
			prevVals: []string{
				"12345", "00:05:00", "pgcenter_fixtures", "public.pgbench_accounts", "autovacuum", "aggressive",
				"active", "f", "scanning heap",
				"8192", "50.00", "25.00",
				"1000", "2000", "autovacuum: VACUUM public.pgbench_accounts",
			},
			currVals: []string{
				"12345", "00:05:01", "pgcenter_fixtures", "public.pgbench_accounts", "autovacuum", "aggressive",
				"active", "f", "scanning heap",
				"8192", "50.00", "25.00",
				"1700", "2900", "autovacuum: VACUUM public.pgbench_accounts",
			},
			wantFile: "testdata/report_record_progress_vacuum_pg19.golden",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ncols := len(tc.cols)

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

			// The pid must be identical in both ticks: UniqueKey is 0, and with different keys diff()
			// falls into the not-found branch and copies rows verbatim — a green test that diffs nothing.
			statPrev := stat.PGresult{Valid: true, Ncols: ncols, Nrows: 1, Cols: tc.cols,
				Values: [][]sql.NullString{mkRow(tc.prevVals)}}
			prevBytes, err := json.Marshal(statPrev)
			assert.NoError(t, err)

			statCurr := stat.PGresult{Valid: true, Ncols: ncols, Nrows: 1, Cols: tc.cols,
				Values: [][]sql.NullString{mkRow(tc.currVals)}}
			currBytes, err := json.Marshal(statCurr)
			assert.NoError(t, err)

			sysinfoBytes := []byte(`{"ticks":100,"cpu_count":4}`)

			// The tar entry basename must equal the report type, otherwise isFilenameOK skips the
			// entries silently. Ticks are exactly one second apart so the interval is 1.
			var tarBuf bytes.Buffer
			tw := tar.NewWriter(&tarBuf)
			writeEntry := func(name string, payload []byte) {
				hdr := &tar.Header{Name: name, Size: int64(len(payload)), Mode: 0644}
				assert.NoError(t, tw.WriteHeader(hdr))
				_, err := tw.Write(payload)
				assert.NoError(t, err)
			}
			writeEntry("meta.20260519T100000.000.json", metaBytes)
			writeEntry("progress_vacuum.20260519T100000.000.json", prevBytes)
			writeEntry("sysinfo.20260519T100000.000.json", sysinfoBytes)
			writeEntry("meta.20260519T100001.000.json", metaBytes)
			writeEntry("progress_vacuum.20260519T100001.000.json", currBytes)
			writeEntry("sysinfo.20260519T100001.000.json", sysinfoBytes)
			assert.NoError(t, tw.Close())

			config := Config{
				ReportType: "progress_vacuum",
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
			assert.Regexp(t, regexp.MustCompile(`\d{4}/\d{2}/\d{2}`), out)
			// Sentinels localise a future failure before the reader has to read a golden diff.
			assert.Contains(t, out, "12345")
			// The scanned,KiB delta. 1700-1000=700 and no printed value contains "700" — the raw 1700
			// is never printed because that column is diffed, so this cannot pass on a wrong delta.
			assert.Contains(t, out, "700")

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
