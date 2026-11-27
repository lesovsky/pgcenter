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

// Test_app_doReport_WAL exercises the full doReport pipeline for the
// version-aware wal report against a synthetic in-memory tar. Each subcase pins
// one recorded PostgreSQL version (18 / 19) via the meta record's version_num,
// which drives report-time view.Configure -> SelectStatWALQuery. The tar carries
// two cumulative ticks; the first is discarded by processData's first-snapshot
// rule (!prevStat.Valid -> continue) and the second produces the single data
// row. countDiff subtracts prev from curr inside the DiffIntvl range and copies
// everything else — waldir_size, stats_age — from the curr tick verbatim.
//
// Unlike the stat_io pair, whose two branches share one shape, the wal branches
// differ in BOTH Ncols and DiffIntvl (PG 18: 7 cols, {2,5}; PG 19: 8 cols,
// {2,6}, the extra fpi,KiB sitting inside the diffed range). So this replay does
// prove the version switch: feed the 8-column PG 19 sample and let Configure
// pick the PG 18 branch, and the diffed range lands on the wrong columns and the
// golden moves.
//
// What this replay does NOT prove: report replays recorded stat.PGresult JSON,
// so the column names and their order come from the fixtures below, never from
// the SQL. Reordering the aliases inside internal/query/wal.go would not redden
// anything here; that layout is pinned by the query and view unit tests
// (internal/query, internal/view). What is pinned here is the rendering and diff
// pipeline plus the version-driven Configure switch — Ncols, DiffIntvl,
// OrderKey, UniqueKey.
//
// The two ticks are exactly one second apart, so the rate divisor itv == 1 and
// each diffed column equals tick2 - tick1 with no scaling. That spacing is
// load-bearing, not incidental: at two seconds every delta halves.
//
// The fixture values are hostile to a wrongly widened diff range: waldir_size is
// a pretty string ("1040 MB" / "1088 MB") and stats_age an interval ("01:00:00" /
// "02:00:00"), both of which fail strconv.ParseInt — so a DiffIntvl that
// swallows either aborts the sample with "diff failed" rather than producing a
// plausible-looking number.
func Test_app_doReport_WAL(t *testing.T) {
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
		// wantContains / wantNotContains are the pre-golden sentinels: they make
		// a failure read as "row missing" or "delta wrong" rather than "golden
		// differs".
		wantContains    []string
		wantNotContains []string
		wantFile        string
	}{
		{
			// PG18: 7 cols, DiffIntvl [2,5]. Absolute: 0 source, 1 waldir_size,
			// 6 stats_age. Diffed: 2..5 (wal,KiB .. buffers_full).
			name:       "pg18",
			versionNum: "180000",
			versionStr: "18.0",
			cols: []string{
				"source", "waldir_size", "wal,KiB",
				"records", "fpi", "buffers_full",
				"stats_age",
			},
			prevVals: []string{
				"WAL", "1040 MB", "2048.50",
				"1000", "300", "12",
				"01:00:00",
			},
			currVals: []string{
				"WAL", "1088 MB", "3072.75",
				"1500", "420", "19",
				"02:00:00",
			},
			// records delta 1500-1000=500 is the cross-version sentinel; the
			// waldir_size string and stats_age are pass-through, and their
			// presence proves the diffed range did not swallow them (it would
			// have failed the whole sample, emptying the buffer).
			wantContains: []string{"WAL", "500", "1088 MB", "02:00:00"},
			// The PG 18 layout has no fpi,KiB column: seeing one here would mean
			// Configure picked the PG 19 branch.
			wantNotContains: []string{"fpi,KiB"},
			wantFile:        "testdata/report_record_wal_pg18.golden",
		},
		{
			// PG19: 8 cols, DiffIntvl [2,6]. Absolute: 0 source, 1 waldir_size,
			// 7 stats_age. Diffed: 2..6, including the new fpi,KiB at index 5 —
			// which pushed the end of the diffed range from 5 to 6 so that
			// buffers_full stayed inside it.
			name:       "pg19",
			versionNum: "190000",
			versionStr: "19.0",
			cols: []string{
				"source", "waldir_size", "wal,KiB",
				"records", "fpi", "fpi,KiB", "buffers_full",
				"stats_age",
			},
			prevVals: []string{
				"WAL", "1040 MB", "2048.50",
				"1000", "300", "600.00", "12",
				"01:00:00",
			},
			currVals: []string{
				"WAL", "1088 MB", "3072.75",
				"1500", "420", "840.25", "19",
				"02:00:00",
			},
			// 240.25 = 840.25 - 600.00, formatted by diffPair as %.2f because
			// the value contains a dot. Its presence — and the absence of the
			// absolute 840.25 — is what proves fpi,KiB landed INSIDE the diffed
			// range.
			wantContains:    []string{"WAL", "500", "fpi,KiB", "240.25", "1088 MB", "02:00:00"},
			wantNotContains: []string{"840.25"},
			wantFile:        "testdata/report_record_wal_pg19.golden",
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

			// Tick 1 (prev): discarded by processData's first-snapshot rule.
			// UniqueKey defaults to 0 (the constant "WAL" source), so the single
			// row pairs with curr.
			statPrev := stat.PGresult{
				Valid: true, Ncols: ncols, Nrows: 1, Cols: tc.cols,
				Values: [][]sql.NullString{mkRow(tc.prevVals)},
			}
			prevBytes, err := json.Marshal(statPrev)
			assert.NoError(t, err)

			// Tick 2 (curr): cumulative values larger than tick 1 in the diffed
			// columns; produces the reported data row.
			statCurr := stat.PGresult{
				Valid: true, Ncols: ncols, Nrows: 1, Cols: tc.cols,
				Values: [][]sql.NullString{mkRow(tc.currVals)},
			}
			currBytes, err := json.Marshal(statCurr)
			assert.NoError(t, err)

			sysinfoBytes := []byte(`{"ticks":100,"cpu_count":4}`)

			// Compose tar (two ticks; per-tick layout matches
			// tarRecorder.write(): meta + wal + sysinfo). The timestamp in each
			// filename uses the recorder's 20060102T150405.000 format — four
			// dot-separated parts, as isFilenameOK requires — and the two ticks
			// are one second apart so itv == 1.
			var tarBuf bytes.Buffer
			tw := tar.NewWriter(&tarBuf)
			writeEntry := func(name string, payload []byte) {
				hdr := &tar.Header{Name: name, Size: int64(len(payload)), Mode: 0644}
				assert.NoError(t, tw.WriteHeader(hdr))
				_, e := tw.Write(payload)
				assert.NoError(t, e)
			}
			writeEntry("meta.20260519T100000.000.json", metaBytes)
			writeEntry("wal.20260519T100000.000.json", prevBytes)
			writeEntry("sysinfo.20260519T100000.000.json", sysinfoBytes)
			writeEntry("meta.20260519T100001.000.json", metaBytes)
			writeEntry("wal.20260519T100001.000.json", currBytes)
			writeEntry("sysinfo.20260519T100001.000.json", sysinfoBytes)
			assert.NoError(t, tw.Close())

			// TsStart/TsEnd must bracket the filename dates or
			// isFilenameTimestampOK silently skips every entry and the test
			// degenerates into an empty report while looking like a real one.
			config := Config{
				ReportType: "wal",
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
			// Timestamp header line emitted by printStatSample matches
			// "YYYY/MM/DD".
			assert.Regexp(t, regexp.MustCompile(`\d{4}/\d{2}/\d{2}`), out)

			for _, s := range tc.wantContains {
				assert.Contains(t, out, s)
			}
			for _, s := range tc.wantNotContains {
				assert.NotContains(t, out, s)
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
