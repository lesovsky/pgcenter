package report

import (
	"bytes"
	"database/sql"
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

func Test_formatReport(t *testing.T) {
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

	formatReport(res, &v, Config{})

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
		"\x1b[37;1mdatname  \x1b[0m\x1b[37;1mcommits  \x1b[0m\x1b[37;1mrollbacks  \x1b[0m\x1b[37;1mreads  \x1b[0m\x1b[37;1mhits  \x1b[0m\x1b[37;1mreturned  \x1b[0m\x1b[37;1mfetched  \x1b[0m\x1b[37;1minserts  \x1b[0m\x1b[37;1mupdates  \x1b[0m\x1b[37;1mdeletes  \x1b[0m\x1b[37;1mconflicts  \x1b[0m\x1b[37;1mdeadlocks  \x1b[0m\x1b[37;1mcsum_fails  \x1b[0m\x1b[37;1mtemp_files  \x1b[0m\x1b[37;1mtemp_bytes  \x1b[0m\x1b[37;1mread_t  \x1b[0m\x1b[37;1mwrite_t  \x1b[0m\x1b[37;1mstats_age  \x1b[0m\n",
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
