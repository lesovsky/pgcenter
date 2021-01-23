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
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func Test_isFilenameOK(t *testing.T) {
	testcases := []struct {
		valid  bool
		name   string
		report string
	}{
		{valid: true, name: "databases.20210116T140630.json", report: "databases"},
		{valid: false, name: "databases.20210116T140630.json", report: "replication"},
		{valid: false, name: "databases.json", report: "databases"},
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
		{valid: true, name: "databases.20210116T140630.json", start: "14:00:00", end: "15:00:00", want: "20210116 14:06:30"},
		{valid: false, name: "invalid.json", start: "14:00:00", end: "15:00:00", want: "20210116 14:06:30"},
		{valid: false, name: "invalid.invalid-ts.json", start: "14:00:00", end: "15:00:00", want: "20210116 14:06:30"},
		{valid: false, name: "databases.20210116T140630.json", start: "14:30:00", end: "15:00:00", want: "20210116 14:06:30"},
		{valid: false, name: "databases.20210116T140630.json", start: "13:30:00", end: "14:00:00", want: "20210116 14:06:30"},
	}

	zone, _ := time.Now().Zone()

	for _, tc := range testcases {
		start, err := time.Parse("20060102 15:04:05 -07", fmt.Sprintf("20210116 %s %s", tc.start, zone))
		assert.NoError(t, err)

		end, err := time.Parse("20060102 15:04:05 -07", fmt.Sprintf("20210116 %s %s", tc.end, zone))
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

func Test_readFileStat(t *testing.T) {
	testcases := []struct {
		valid    bool
		filename string
	}{
		{valid: true, filename: "testdata/pgcenter.stat.golden.tar"},
		{valid: false, filename: "testdata/pgcenter.stat.invalid.tar"},
	}

	for _, tc := range testcases {
		t.Run(tc.filename, func(t *testing.T) {
			f, err := os.Open(tc.filename)
			assert.NoError(t, err)

			r := tar.NewReader(f)

			for {
				hdr, err := r.Next()
				if err == io.EOF {
					break
				} else if err != nil {
					assert.Fail(t, "unexpected error", err)
				}

				got, err := readFileStat(r, hdr.Size)
				if tc.valid {
					assert.NoError(t, err)
					assert.NotNil(t, got.Values)
					assert.NotNil(t, got.Cols)
				} else {
					assert.Error(t, err)
					assert.Equal(t, stat.PGresult{}, got)
				}
			}
		})
	}
}

func Test_countDiff(t *testing.T) {
	prev := stat.PGresult{
		Valid: true, Ncols: 18, Nrows: 1,
		Cols: []string{
			"datname", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes",
			"conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age",
		},
		Values: [][]sql.NullString{
			{
				{String: "example_db", Valid: true}, {String: "1000", Valid: true}, {String: "10", Valid: true}, {String: "4000", Valid: true},
				{String: "20000", Valid: true}, {String: "2000", Valid: true}, {String: "6000", Valid: true}, {String: "8000", Valid: true},
				{String: "12000", Valid: true}, {String: "3000", Valid: true}, {String: "50", Valid: true}, {String: "60", Valid: true},
				{String: "0", Valid: true}, {String: "100", Valid: true}, {String: "50000", Valid: true}, {String: "500", Valid: true},
				{String: "5", Valid: true}, {String: "11 days 10:10:10", Valid: true},
			},
		},
	}
	curr := stat.PGresult{
		Valid: true, Ncols: 18, Nrows: 1,
		Cols: []string{
			"datname", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes",
			"conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age",
		},
		Values: [][]sql.NullString{
			{
				{String: "example_db", Valid: true}, {String: "1500", Valid: true}, {String: "15", Valid: true}, {String: "6000", Valid: true},
				{String: "30000", Valid: true}, {String: "3000", Valid: true}, {String: "9000", Valid: true}, {String: "12000", Valid: true},
				{String: "18000", Valid: true}, {String: "4500", Valid: true}, {String: "75", Valid: true}, {String: "90", Valid: true},
				{String: "1", Valid: true}, {String: "150", Valid: true}, {String: "75000", Valid: true}, {String: "750", Valid: true},
				{String: "8", Valid: true}, {String: "11 days 10:10:11", Valid: true},
			},
		},
	}

	want := stat.PGresult{
		Valid: true, Ncols: 18, Nrows: 1,
		Cols: []string{
			"datname", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes",
			"conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age",
		},
		Values: [][]sql.NullString{
			{
				{String: "example_db", Valid: true}, {String: "500", Valid: true}, {String: "5", Valid: true}, {String: "2000", Valid: true},
				{String: "10000", Valid: true}, {String: "1000", Valid: true}, {String: "3000", Valid: true}, {String: "4000", Valid: true},
				{String: "6000", Valid: true}, {String: "1500", Valid: true}, {String: "25", Valid: true}, {String: "30", Valid: true},
				{String: "1", Valid: true}, {String: "50", Valid: true}, {String: "25000", Valid: true}, {String: "250", Valid: true},
				{String: "3", Valid: true}, {String: "11 days 10:10:11", Valid: true},
			},
		},
	}

	views := view.New()
	v := views["databases"]

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
	v := views["databases"]

	formatStatSample(res, &v, Config{})

	assert.True(t, v.Aligned)
	assert.NotNil(t, v.ColsWidth)
	assert.NotNil(t, v.Cols)
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
	v := views["databases"]

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

func Test_printStatReport(t *testing.T) {
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
	v := views["databases"]

	widthes, cols := align.SetAlign(*res, 32, true)
	v.ColsWidth = widthes
	v.Cols = cols
	v.Aligned = true

	f, err := ioutil.TempFile("/tmp", "pgcenter-test-report-")
	assert.NoError(t, err)
	fname := f.Name()

	// print report
	n, err := printStatReport(f, res, v, Config{}, time.Time{})
	assert.NoError(t, err)
	assert.Equal(t, 2, n)

	// rewind to beginning
	_, err = f.Seek(0, io.SeekStart)
	assert.NoError(t, err)

	// read file
	got, err := ioutil.ReadAll(f)
	assert.NoError(t, err)

	// read wanted
	want, err := ioutil.ReadFile("testdata/report_entry_sample.golden")
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
		{report: "databases", want: pgStatDatabaseDescription},
		{report: "activity", want: pgStatActivityDescription},
		{report: "replication", want: pgStatReplicationDescription},
		{report: "tables", want: pgStatTablesDescription},
		{report: "indexes", want: pgStatIndexesDescription},
		{report: "functions", want: pgStatFunctionsDescription},
		{report: "sizes", want: pgStatSizesDescription},
		{report: "progress_vacuum", want: pgStatProgressVacuumDescription},
		{report: "progress_cluster", want: pgStatProgressClusterDescription},
		{report: "progress_index", want: pgStatProgressCreateIndexDescription},
		{report: "statements_timing", want: pgStatStatementsTimingDescription},
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
