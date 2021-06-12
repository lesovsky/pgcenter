package report

import (
	"archive/tar"
	"bytes"
	"database/sql"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/align"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
	"github.com/stretchr/testify/assert"
	"io"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"
)

func Test_app_doReport(t *testing.T) {
	testcases := []struct {
		start    string
		end      string
		config   Config
		wantFile string
	}{
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "activity", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_activity.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "replication", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_replication.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "databases_general", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_databases_general.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "databases_sessions", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_databases_sessions.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "tables", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_tables.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "indexes", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_indexes.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "sizes", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_sizes.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "functions", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_functions.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "wal", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_wal.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "statements_timings", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_statements_timings.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "statements_general", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_statements_general.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "statements_io", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_statements_io.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "statements_local", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_statements_local.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "statements_temp", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_statements_temp.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "statements_wal", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_statements_wal.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "progress_vacuum", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_progress_vacuum.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "progress_cluster", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_progress_cluster.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "progress_index", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_progress_index.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "progress_analyze", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_progress_analyze.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "progress_basebackup", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_progress_basebackup.golden",
		},
		{
			start: "2021-01-23 15:31:00", end: "2021-01-23 15:32:00",
			config:   Config{ReportType: "progress_copy", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_progress_copy.golden",
		},
		{ // start, end times within report interval
			start: "2021-01-23 15:31:26", end: "2021-01-23 15:31:27",
			config:   Config{ReportType: "activity", TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_activity_start_end.golden",
		},
		{ // start, end times within report interval, set order by pid (desc)
			start: "2021-01-23 15:31:26", end: "2021-01-23 15:31:27",
			config:   Config{ReportType: "activity", OrderColName: "pid", OrderDesc: true, TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_activity_order_pid_desc.golden",
		},
		{ // start, end times within report interval, set order by pid (asc)
			start: "2021-01-23 15:31:26", end: "2021-01-23 15:31:27",
			config:   Config{ReportType: "activity", OrderColName: "pid", OrderDesc: false, TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_activity_order_pid_asc.golden",
		},
		{ // start, end times within report interval, grep by query:UPDATE
			start: "2021-01-23 15:31:26", end: "2021-01-23 15:31:27",
			config:   Config{ReportType: "activity", FilterColName: "query", FilterRE: regexp.MustCompile("UPDATE"), TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_activity_grep.golden",
		},
		{ // start, end times within report interval, limit by number of rows
			start: "2021-01-23 15:31:26", end: "2021-01-23 15:31:27",
			config:   Config{ReportType: "statements_timings", RowLimit: 10, TruncLimit: 32, Rate: time.Second},
			wantFile: "testdata/report_statements_timings_limit.golden",
		},
		{ // start, end times within report interval, limit by number of rows, string limit
			start: "2021-01-23 15:31:26", end: "2021-01-23 15:31:27",
			config:   Config{ReportType: "statements_timings", RowLimit: 10, TruncLimit: 64, Rate: time.Second},
			wantFile: "testdata/report_statements_timings_limit_truncate.golden",
		},
	}

	for _, tc := range testcases {
		ts, err := time.ParseInLocation("2006-01-02 15:04:05", tc.start, time.Now().Location())
		assert.NoError(t, err)
		te, err := time.ParseInLocation("2006-01-02 15:04:05", tc.end, time.Now().Location())
		assert.NoError(t, err)

		tc.config.TsStart = ts
		tc.config.TsEnd = te

		app := newApp(tc.config)
		var buf bytes.Buffer
		app.writer = &buf

		f, err := os.Open("testdata/pgcenter.stat.golden.tar")
		assert.NoError(t, err)
		tr := tar.NewReader(f)

		err = app.doReport(tr)
		assert.NoError(t, err)

		want, err := os.ReadFile(tc.wantFile)
		assert.NoError(t, err)

		assert.Equal(t, string(want), buf.String())
	}
}

func Test_readTar(t *testing.T) {
	config := Config{
		ReportType: "databases_general",
		TsStart:    time.Date(2021, 01, 23, 00, 00, 00, 0, time.UTC),
		TsEnd:      time.Date(2021, 01, 23, 23, 59, 59, 0, time.UTC),
		TruncLimit: 32, Rate: 1 * time.Second}
	f, err := os.Open("testdata/pgcenter.stat.golden.tar")
	assert.NoError(t, err)
	tr := tar.NewReader(f)

	dataCh := make(chan data)
	doneCh := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		var count int
		for {
			select {
			case <-dataCh:
				count++
			case <-doneCh:
				assert.Equal(t, 10, count)
				wg.Done()
				break
			}
		}
	}()

	err = readTar(tr, config, dataCh, doneCh)
	assert.NoError(t, err)

	wg.Wait()

	assert.NoError(t, f.Close())
}

func Test_processData(t *testing.T) {
	prev := stat.PGresult{
		Valid: true, Ncols: 19, Nrows: 1,
		Cols: []string{
			"datname", "backends", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes",
			"conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age",
		},
		Values: [][]sql.NullString{
			{
				{String: "example_db", Valid: true}, {String: "15", Valid: true}, {String: "1000", Valid: true}, {String: "10", Valid: true},
				{String: "4000", Valid: true}, {String: "20000", Valid: true}, {String: "2000", Valid: true}, {String: "6000", Valid: true},
				{String: "8000", Valid: true}, {String: "12000", Valid: true}, {String: "3000", Valid: true}, {String: "50", Valid: true},
				{String: "60", Valid: true}, {String: "0", Valid: true}, {String: "100", Valid: true}, {String: "50000", Valid: true},
				{String: "500", Valid: true}, {String: "5", Valid: true}, {String: "11 days 10:10:10", Valid: true},
			},
		},
	}
	curr := stat.PGresult{
		Valid: true, Ncols: 19, Nrows: 1,
		Cols: []string{
			"datname", "backends", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes",
			"conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age",
		},
		Values: [][]sql.NullString{
			{
				{String: "example_db", Valid: true}, {String: "11", Valid: true}, {String: "1500", Valid: true}, {String: "15", Valid: true},
				{String: "6000", Valid: true}, {String: "30000", Valid: true}, {String: "3000", Valid: true}, {String: "9000", Valid: true},
				{String: "12000", Valid: true}, {String: "18000", Valid: true}, {String: "4500", Valid: true}, {String: "75", Valid: true},
				{String: "90", Valid: true}, {String: "1", Valid: true}, {String: "150", Valid: true}, {String: "75000", Valid: true},
				{String: "750", Valid: true}, {String: "8", Valid: true}, {String: "11 days 10:10:11", Valid: true},
			},
		},
	}

	config := Config{ReportType: "databases_general", TruncLimit: 32, Rate: 2 * time.Second, OrderColName: "datname"}
	app := newApp(config)
	var buf bytes.Buffer
	app.writer = &buf

	views := view.New()

	dataCh := make(chan data)
	doneCh := make(chan struct{})

	go func() {
		dataCh <- data{
			ts:   time.Date(2021, 01, 01, 00, 00, 00, 0, time.UTC),
			res:  prev,
			meta: metadata{version: 140000},
		}

		dataCh <- data{
			ts:   time.Date(2021, 01, 01, 00, 00, 01, 0, time.UTC),
			res:  curr,
			meta: metadata{version: 140000},
		}

		doneCh <- struct{}{}
	}()

	err := processData(app, views["activity"], config, dataCh, doneCh)
	assert.NoError(t, err)

	want, err := os.ReadFile("testdata/report_sample.golden")
	assert.NoError(t, err)

	assert.Equal(t, string(want), buf.String())
}

func Test_readMeta(t *testing.T) {
	testcases := []struct {
		valid bool
		res   stat.PGresult
		want  metadata
	}{
		{
			valid: true,
			res: stat.PGresult{
				Values: [][]sql.NullString{
					{
						{String: "14beta1 (Ubuntu 14~beta1-1.pgdg20.04+1)", Valid: true}, {String: "140000", Valid: true},
						{String: "off", Valid: true}, {String: "100", Valid: true}, {String: "3", Valid: true},
						{String: "false", Valid: true}, {String: "1622828486655396e-6", Valid: true},
					},
				},
				Cols:  []string{"version", "version_num", "track_commit_timestamp", "max_connections", "autovacuum_max_workers", "recovery", "start_time_unix"},
				Ncols: 7, Nrows: 1, Valid: true,
			},
			want: metadata{version: 140000},
		},
		{
			valid: false,
			res: stat.PGresult{
				Values: [][]sql.NullString{
					{
						{String: "14beta1 (Ubuntu 14~beta1-1.pgdg20.04+1)", Valid: true}, {String: "invalid", Valid: true},
						{String: "off", Valid: true}, {String: "100", Valid: true}, {String: "3", Valid: true},
						{String: "false", Valid: true}, {String: "1622828486655396e-6", Valid: true},
					},
				},
				Cols:  []string{"version", "version_num", "track_commit_timestamp", "max_connections", "autovacuum_max_workers", "recovery", "start_time_unix"},
				Ncols: 7, Nrows: 1, Valid: true,
			},
			want: metadata{version: 140000},
		},
		{valid: false, res: stat.PGresult{Ncols: 1, Nrows: 1, Valid: true}},
		{valid: false, res: stat.PGresult{Ncols: 7, Nrows: 0, Valid: true}},
	}

	for _, tc := range testcases {
		got, err := readMeta(tc.res)
		if tc.valid {
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		} else {
			assert.Error(t, err)
		}
	}
}

func Test_isFilenameOK(t *testing.T) {
	testcases := []struct {
		valid  bool
		name   string
		report string
	}{
		{valid: true, name: "databases_general.20210116T140630.json", report: "databases_general"},
		{valid: true, name: "meta.20210116T140630.json", report: "databases_general"},
		{valid: false, name: "databases_general.20210116T140630.json", report: "replication"},
		{valid: false, name: "databases_general.json", report: "databases_general"},
	}

	for _, tc := range testcases {
		if tc.valid {
			assert.NoError(t, isFilenameOK(tc.name, tc.report))
		} else {
			assert.Error(t, isFilenameOK(tc.name, tc.report))
		}
	}
}

func Test_isFilenameTimestampOK(t *testing.T) {
	testcases := []struct {
		valid bool
		name  string
		start string
		end   string
		want  string
	}{
		{valid: true, name: "databases_general.20210116T140630.json", start: "14:00:00", end: "15:00:00", want: "20210116 14:06:30"},
		{valid: false, name: "invalid.json", start: "14:00:00", end: "15:00:00", want: "20210116 14:06:30"},
		{valid: false, name: "invalid.invalid-ts.json", start: "14:00:00", end: "15:00:00", want: "20210116 14:06:30"},
		{valid: false, name: "databases_general.20210116T140630.json", start: "14:30:00", end: "15:00:00", want: "20210116 14:06:30"},
		{valid: false, name: "databases_general.20210116T140630.json", start: "13:30:00", end: "14:00:00", want: "20210116 14:06:30"},
	}

	loc := time.Now().Location()

	for _, tc := range testcases {
		start, err := time.ParseInLocation("20060102 15:04:05", fmt.Sprintf("20210116 %s", tc.start), loc)
		assert.NoError(t, err)

		end, err := time.ParseInLocation("20060102 15:04:05", fmt.Sprintf("20210116 %s", tc.end), loc)
		assert.NoError(t, err)

		got, err := isFilenameTimestampOK(tc.name, start, end)
		if tc.valid {
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got.Format("20060102 15:04:05"))
		} else {
			assert.Error(t, err)
		}
	}
}

func Test_countDiff(t *testing.T) {
	prev := stat.PGresult{
		Valid: true, Ncols: 19, Nrows: 1,
		Cols: []string{
			"datname", "backends", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes",
			"conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age",
		},
		Values: [][]sql.NullString{
			{
				{String: "example_db", Valid: true}, {String: "15", Valid: true}, {String: "1000", Valid: true}, {String: "10", Valid: true},
				{String: "4000", Valid: true}, {String: "20000", Valid: true}, {String: "2000", Valid: true}, {String: "6000", Valid: true},
				{String: "8000", Valid: true}, {String: "12000", Valid: true}, {String: "3000", Valid: true}, {String: "50", Valid: true},
				{String: "60", Valid: true}, {String: "0", Valid: true}, {String: "100", Valid: true}, {String: "50000", Valid: true},
				{String: "500", Valid: true}, {String: "5", Valid: true}, {String: "11 days 10:10:10", Valid: true},
			},
		},
	}
	curr := stat.PGresult{
		Valid: true, Ncols: 19, Nrows: 1,
		Cols: []string{
			"datname", "backends", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes",
			"conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age",
		},
		Values: [][]sql.NullString{
			{
				{String: "example_db", Valid: true}, {String: "11", Valid: true}, {String: "1500", Valid: true}, {String: "15", Valid: true},
				{String: "6000", Valid: true}, {String: "30000", Valid: true}, {String: "3000", Valid: true}, {String: "9000", Valid: true},
				{String: "12000", Valid: true}, {String: "18000", Valid: true}, {String: "4500", Valid: true}, {String: "75", Valid: true},
				{String: "90", Valid: true}, {String: "1", Valid: true}, {String: "150", Valid: true}, {String: "75000", Valid: true},
				{String: "750", Valid: true}, {String: "8", Valid: true}, {String: "11 days 10:10:11", Valid: true},
			},
		},
	}

	want := stat.PGresult{
		Valid: true, Ncols: 19, Nrows: 1,
		Cols: []string{
			"datname", "backends", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes",
			"conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age",
		},
		Values: [][]sql.NullString{
			{
				{String: "example_db", Valid: true}, {String: "11", Valid: true}, {String: "500", Valid: true}, {String: "5", Valid: true},
				{String: "2000", Valid: true}, {String: "10000", Valid: true}, {String: "1000", Valid: true}, {String: "3000", Valid: true},
				{String: "4000", Valid: true}, {String: "6000", Valid: true}, {String: "1500", Valid: true}, {String: "25", Valid: true},
				{String: "30", Valid: true}, {String: "1", Valid: true}, {String: "50", Valid: true}, {String: "25000", Valid: true},
				{String: "250", Valid: true}, {String: "3", Valid: true}, {String: "11 days 10:10:11", Valid: true},
			},
		},
	}

	views := view.New()
	v := views["databases_general"]

	got, err := countDiff(curr, prev, 1, v)
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func Test_getColumnIndex(t *testing.T) {
	testcases := []struct {
		colname string
		wantIdx int
		wantOk  bool
	}{
		{colname: "testcol2", wantIdx: 1, wantOk: true},
		{colname: "unknown", wantIdx: -1, wantOk: false},
		{colname: "", wantIdx: -1, wantOk: false},
	}

	for _, tc := range testcases {
		got, ok := getColumnIndex([]string{"testcol1", "testcol2", "testcol3"}, tc.colname)
		assert.Equal(t, tc.wantIdx, got)
		assert.Equal(t, tc.wantOk, ok)
	}
}

func Test_formatStatSample(t *testing.T) {
	res := &stat.PGresult{
		Valid: true,
		Ncols: 18,
		Nrows: 2,
		Cols: []string{
			"datname", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes",
			"conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age",
		},
		Values: [][]sql.NullString{
			{
				{String: "example_db", Valid: true}, {String: "5423", Valid: true}, {String: "24", Valid: true}, {String: "8452", Valid: true},
				{String: "8452145", Valid: true}, {String: "45214", Valid: true}, {String: "58452", Valid: true}, {String: "4521", Valid: true},
				{String: "45221", Valid: true}, {String: "45854", Valid: true}, {String: "248", Valid: true}, {String: "785", Valid: true},
				{String: "2", Valid: true}, {String: "4774", Valid: true}, {String: "698785411", Valid: true}, {String: "4582.02", Valid: true},
				{String: "42.12", Valid: true}, {String: "10 days 10:10:10", Valid: true},
			},
			{
				{String: "example_db2", Valid: true}, {String: "84521", Valid: true}, {String: "866", Valid: true}, {String: "59654", Valid: true},
				{String: "485421", Valid: true}, {String: "86421", Valid: true}, {String: "89642", Valid: true}, {String: "9869", Valid: true},
				{String: "45212", Valid: true}, {String: "96969", Valid: true}, {String: "124", Valid: true}, {String: "858", Valid: true},
				{String: "0", Valid: true}, {String: "8457", Valid: true}, {String: "6581546", Valid: true}, {String: "2445.77", Valid: true},
				{String: "458.01", Valid: true}, {String: "10 days 10:10:10", Valid: true},
			},
		},
	}

	views := view.New()
	v := views["databases_general"]

	formatStatSample(res, &v, Config{})

	assert.True(t, v.Aligned)
	assert.NotNil(t, v.ColsWidth)
	assert.NotNil(t, v.Cols)
}

func Test_printReportHeader(t *testing.T) {
	tsStart, err := time.Parse("2006-01-02 15:04:05 MST", "2021-01-18 05:00:00 +05")
	assert.NoError(t, err)
	tsEnd, err := time.Parse("2006-01-02 15:04:05 MST", "2021-01-18 06:00:00 +05")
	assert.NoError(t, err)

	c := Config{
		InputFile:  "test_example.stat.tar",
		ReportType: "test_example",
		TsStart:    tsStart,
		TsEnd:      tsEnd,
		Rate:       time.Second,
	}

	want := `INFO: reading from test_example.stat.tar
INFO: report test_example
INFO: start from: 2021-01-18 05:00:00 +05, to: 2021-01-18 06:00:00 +05, with rate: 1s
`

	var buf bytes.Buffer
	assert.NoError(t, printReportHeader(&buf, c))
	assert.Equal(t, want, buf.String())
}

func Test_printStatHeader(t *testing.T) {
	res := &stat.PGresult{
		Valid: true, Ncols: 18, Nrows: 0,
		Cols: []string{
			"datname", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes",
			"conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age",
		},
		Values: [][]sql.NullString{},
	}

	views := view.New()
	v := views["databases_general"]

	widthes, cols := align.SetAlign(*res, 32, true)
	v.ColsWidth = widthes
	v.Cols = cols
	v.Aligned = true

	var buf bytes.Buffer

	n, err := printStatHeader(&buf, 20, v)
	assert.Equal(t, 0, n)
	assert.Equal(t,
		"         \x1b[37;1mdatname  \x1b[0m\x1b[37;1mcommits  \x1b[0m\x1b[37;1mrollbacks  \x1b[0m\x1b[37;1mreads  \x1b[0m\x1b[37;1mhits  \x1b[0m\x1b[37;1mreturned  \x1b[0m\x1b[37;1mfetched  \x1b[0m\x1b[37;1minserts  \x1b[0m\x1b[37;1mupdates  \x1b[0m\x1b[37;1mdeletes  \x1b[0m\x1b[37;1mconflicts  \x1b[0m\x1b[37;1mdeadlocks  \x1b[0m\x1b[37;1mcsum_fails  \x1b[0m\x1b[37;1mtemp_files  \x1b[0m\x1b[37;1mtemp_bytes  \x1b[0m\x1b[37;1mread_t  \x1b[0m\x1b[37;1mwrite_t  \x1b[0m\x1b[37;1mstats_age  \x1b[0m\n",
		buf.String(),
	)
	assert.NoError(t, err)

	n, err = printStatHeader(&buf, 10, v)
	assert.Equal(t, 10, n)
	assert.NoError(t, err)
}

func Test_printStatSample(t *testing.T) {
	res := &stat.PGresult{
		Valid: true,
		Ncols: 18,
		Nrows: 2,
		Cols: []string{
			"datname", "commits", "rollbacks", "reads",
			"hits", "returned", "fetched", "inserts",
			"updates", "deletes", "conflicts", "deadlocks",
			"csum_fails", "temp_files", "temp_bytes", "read_t",
			"write_t", "stats_age",
		},
		Values: [][]sql.NullString{
			{
				{String: "example_db", Valid: true}, {String: "5423", Valid: true}, {String: "24", Valid: true}, {String: "8452", Valid: true},
				{String: "8452145", Valid: true}, {String: "45214", Valid: true}, {String: "58452", Valid: true}, {String: "4521", Valid: true},
				{String: "45221", Valid: true}, {String: "45854", Valid: true}, {String: "248", Valid: true}, {String: "785", Valid: true},
				{String: "2", Valid: true}, {String: "4774", Valid: true}, {String: "698785411", Valid: true}, {String: "4582.02", Valid: true},
				{String: "42.12", Valid: true}, {String: "10 days 10:10:10", Valid: true},
			},
			{
				{String: "example_db2", Valid: true}, {String: "84521", Valid: true}, {String: "866", Valid: true}, {String: "59654", Valid: true},
				{String: "485421", Valid: true}, {String: "86421", Valid: true}, {String: "89642", Valid: true}, {String: "9869", Valid: true},
				{String: "45212", Valid: true}, {String: "96969", Valid: true}, {String: "124", Valid: true}, {String: "858", Valid: true},
				{String: "0", Valid: true}, {String: "8457", Valid: true}, {String: "6581546", Valid: true}, {String: "2445.77", Valid: true},
				{String: "458.01", Valid: true}, {String: "10 days 10:10:10", Valid: true},
			},
		},
	}

	views := view.New()
	v := views["databases_general"]

	widthes, cols := align.SetAlign(*res, 32, true)
	v.ColsWidth = widthes
	v.Cols = cols
	v.Aligned = true

	f, err := os.CreateTemp("/tmp", "pgcenter-test-report-")
	assert.NoError(t, err)
	fname := f.Name()

	// print report
	n, err := printStatSample(f, res, v, Config{}, time.Time{})
	assert.NoError(t, err)
	assert.Equal(t, 2, n)

	// rewind to beginning
	_, err = f.Seek(0, io.SeekStart)
	assert.NoError(t, err)

	// read file
	got, err := io.ReadAll(f)
	assert.NoError(t, err)

	// read wanted
	want, err := os.ReadFile("testdata/report_entry_sample.golden")
	assert.NoError(t, err)

	// compare created and wanted
	assert.Equal(t, want, got)

	// cleanup
	assert.NoError(t, f.Close())
	assert.NoError(t, os.Remove(fname))
}

func Test_describeReport(t *testing.T) {
	testcases := []struct {
		report string
		want   string
	}{
		{report: "databases_general", want: pgStatDatabaseGeneralDescription},
		{report: "databases_sessions", want: pgStatDatabaseSessionsDescription},
		{report: "activity", want: pgStatActivityDescription},
		{report: "replication", want: pgStatReplicationDescription},
		{report: "tables", want: pgStatTablesDescription},
		{report: "indexes", want: pgStatIndexesDescription},
		{report: "functions", want: pgStatFunctionsDescription},
		{report: "wal", want: pgStatWALDescription},
		{report: "sizes", want: pgStatSizesDescription},
		{report: "progress_vacuum", want: pgStatProgressVacuumDescription},
		{report: "progress_cluster", want: pgStatProgressClusterDescription},
		{report: "progress_index", want: pgStatProgressCreateIndexDescription},
		{report: "progress_analyze", want: pgStatProgressAnalyzeDescription},
		{report: "progress_basebackup", want: pgStatProgressBasebackupDescription},
		{report: "progress_copy", want: pgStatProgressCopyDescription},
		{report: "statements_timings", want: pgStatStatementsTimingsDescription},
		{report: "statements_general", want: pgStatStatementsGeneralDescription},
		{report: "statements_io", want: pgStatStatementsIODescription},
		{report: "statements_local", want: pgStatStatementsTempDescription},
		{report: "statements_temp", want: pgStatStatementsLocalDescription},
		{report: "invalid", want: "unknown description requested"},
	}

	for _, tc := range testcases {
		var buf bytes.Buffer

		err := describeReport(&buf, tc.report)
		assert.NoError(t, err)
		assert.Equal(t, tc.want, buf.String())
	}

}
