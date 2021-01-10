package report

import (
	"github.com/lesovsky/pgcenter/report"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_options_validate(t *testing.T) {
	testcases := []struct {
		valid bool
		opts  options
		want  report.Options
	}{
		{valid: true, opts: options{showActivity: true, tsStart: "20210101-120000", tsEnd: "20210101-130000"}},
		{valid: false, opts: options{tsStart: "20210101-120000", tsEnd: "20210101-130000"}}, // no report type specified
		{valid: false, opts: options{showActivity: true, tsStart: "20210132"}},              // invalid report start timestamp
		{valid: false, opts: options{showActivity: true, filter: `colname:"["`}},            // invalid regexp
	}

	for _, tc := range testcases {
		got, err := tc.opts.validate()
		if tc.valid {
			assert.NoError(t, err)
			assert.NotNil(t, got)
		} else {
			assert.Error(t, err)
		}
	}
}

func Test_selectReport(t *testing.T) {
	testcases := []struct {
		opts options
		want string
	}{
		{opts: options{showActivity: true}, want: "activity"},
		{opts: options{showReplication: true}, want: "replication"},
		{opts: options{showDatabases: true}, want: "databases"},
		{opts: options{showTables: true}, want: "tables"},
		{opts: options{showIndexes: true}, want: "indexes"},
		{opts: options{showFunctions: true}, want: "functions"},
		{opts: options{showSizes: true}, want: "sizes"},
		{opts: options{showStatements: "m"}, want: "statements_timings"},
		{opts: options{showStatements: "g"}, want: "statements_general"},
		{opts: options{showStatements: "i"}, want: "statements_io"},
		{opts: options{showStatements: "t"}, want: "statements_temp"},
		{opts: options{showStatements: "l"}, want: "statements_local"},
		{opts: options{showProgress: "v"}, want: "progress_vacuum"},
		{opts: options{showProgress: "c"}, want: "progress_cluster"},
		{opts: options{showProgress: "i"}, want: "progress_index"},
		{opts: options{}, want: ""},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, selectReport(tc.opts))
	}
}

func Test_selectReportInterval(t *testing.T) {
	layout := "20060102-150405"

	// open start/end
	start, end, err := selectReportInterval("", "")
	assert.NoError(t, err)
	assert.Equal(t, "00010101-000000", start.Format(layout))
	assert.NotEqual(t, "00010101-000000", end.Format(layout))

	// open start, valid end
	start, end, err = selectReportInterval("", "20210110-130000")
	assert.NoError(t, err)
	assert.Equal(t, "00010101-000000", start.Format(layout))
	assert.Equal(t, "20210110-130000", end.Format(layout))

	// valid start, open end
	start, end, err = selectReportInterval("20210110-120000", "")
	assert.NoError(t, err)
	assert.Equal(t, "20210110-120000", start.Format(layout))
	assert.NotEqual(t, "00010101-000000", end.Format(layout))

	// valid start, valid end
	start, end, err = selectReportInterval("20210110-120000", "20210110-130000")
	assert.NoError(t, err)
	assert.Equal(t, "20210110-120000", start.Format(layout))
	assert.Equal(t, "20210110-130000", end.Format(layout))

	// valid start (no date), open end
	start, end, err = selectReportInterval("120000", "")
	assert.NoError(t, err)
	assert.NotEqual(t, "00010101-000000", start.Format(layout))
	assert.NotEqual(t, "00010101-000000", end.Format(layout))

	// open start, valid end (no date)
	start, end, err = selectReportInterval("", "130000")
	assert.NoError(t, err)
	assert.Equal(t, "00010101-000000", start.Format(layout))
	assert.NotEqual(t, "00010101-000000", end.Format(layout))

	// valid start (no date), valid end (no date)
	start, end, err = selectReportInterval("120000", "130000")
	assert.NoError(t, err)
	assert.NotEqual(t, "00010101-000000", start.Format(layout))
	assert.NotEqual(t, "00010101-000000", end.Format(layout))

	testcases := []struct {
		start string
		end   string
	}{
		{start: "241011", end: ""},
		{start: "126011", end: ""},
		{start: "121060", end: ""},
		{start: "", end: "241011"},
		{start: "", end: "126011"},
		{start: "", end: "121060"},
		{start: "20211301-1200000", end: ""},
		{start: "20210132-1200000", end: ""},
		{start: "", end: "20211301-1200000"},
		{start: "", end: "20210132-1200000"},
		{start: "20210101-", end: ""},
		{start: "20210101", end: ""}, // TODO: date with no time should be valid
		{start: "-120000", end: ""},
		{start: "", end: "20210101-"},
		{start: "", end: "20210101"}, // TODO: date with no time should be valid
		{start: "", end: "-130000"},
		{start: "invalid", end: ""},
		{start: "", end: "invalid"},
		{start: "invalid", end: "invalid"},
	}
	for _, tc := range testcases {
		_, _, err := selectReportInterval(tc.start, tc.end)
		assert.Error(t, err)
	}
}

func Test_parseFilterString(t *testing.T) {
	testcases := []struct {
		valid       bool
		filter      string
		wantColname string
	}{
		{valid: true, filter: "", wantColname: ""},
		{valid: true, filter: "testcol:testre", wantColname: "testcol"},
		{valid: true, filter: `testcol:"test1|test2"`, wantColname: "testcol"},
		{valid: true, filter: `testcol:"test[0-9a-f]+"`, wantColname: "testcol"},
		{valid: false, filter: "testcol:"},
		{valid: false, filter: ":testre"},
		{valid: false, filter: ":testre1:testre2:testre3"},
		{valid: false, filter: "testcol:["},
	}

	for _, tc := range testcases {
		gotColname, gotRE, err := parseFilterString(tc.filter)
		if tc.valid {
			assert.NoError(t, err)
			assert.Equal(t, tc.wantColname, gotColname)
			if tc.wantColname != "" {
				assert.NotNil(t, gotRE)
			}
		} else {
			assert.Error(t, err)
		}
	}
}
