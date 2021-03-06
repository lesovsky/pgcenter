package report

import (
	"github.com/lesovsky/pgcenter/report"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func Test_options_validate(t *testing.T) {
	testcases := []struct {
		valid bool
		opts  options
		want  report.Config
	}{
		{valid: true, opts: options{showActivity: true, tsStart: "2021-01-01 12:00:00", tsEnd: "2021-01-01 13:00:00", rate: time.Second}},
		{valid: true, opts: options{showActivity: true, tsStart: "2021-01-01 12:00:00", tsEnd: "2021-01-01 13:00:00", rate: 0}},
		{valid: false, opts: options{tsStart: "2021-01-01 12:00:00", tsEnd: "2021-01-01 13:00:00", rate: time.Second}}, // no report type specified
		{valid: false, opts: options{showActivity: true, tsStart: "2021-01-32", rate: time.Second}},                    // invalid report start timestamp
		{valid: false, opts: options{showActivity: true, filter: `colname:"["`, rate: time.Second}},                    // invalid regexp
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
		{opts: options{showProgress: "a"}, want: "progress_analyze"},
		{opts: options{showProgress: "b"}, want: "progress_basebackup"},
		{opts: options{}, want: ""},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, selectReport(tc.opts))
	}
}

func Test_setReportInterval(t *testing.T) {
	today := time.Now().Format("2006-01-02")

	testcases := []struct {
		valid     bool
		start     string
		end       string
		startWant string
		endWant   string
	}{
		// both full start, end time
		{valid: true, start: "2021-01-23 10:11:12", end: "2021-01-23 11:12:13", startWant: "2021-01-23 10:11:12", endWant: "2021-01-23 11:12:13"},
		// empty start time
		{valid: true, start: "", end: "2021-01-23 11:12:13", startWant: "0001-01-01 00:00:00", endWant: "2021-01-23 11:12:13"},
		// no times
		{valid: true, start: "2021-01-23", end: "2021-01-24", startWant: "2021-01-23 00:00:00", endWant: "2021-01-24 00:00:00"},
		// no dates
		{valid: true, start: "10:11:12", end: "11:12:13", startWant: today + " 10:11:12", endWant: today + " 11:12:13"},
		// invalid input
		{valid: false, start: "2021-01-23 10:11:60"},
		{valid: false, end: "2021-01-23 10:11:60"},
	}

	for _, tc := range testcases {
		start, end, err := setReportInterval(tc.start, tc.end)
		if tc.valid {
			assert.NoError(t, err)
			assert.Equal(t, tc.startWant, start.Format("2006-01-02 15:04:05"))
			assert.Equal(t, tc.endWant, end.Format("2006-01-02 15:04:05"))
		} else {
			assert.Error(t, err)
		}
	}

	// test with empty start/end time
	s, e, err := setReportInterval("", "")
	assert.NoError(t, err)
	assert.Equal(t, "0001-01-01 00:00:00", s.Format("2006-01-02 15:04:05"))
	assert.WithinDuration(t, time.Now(), e, 5*time.Second)
}

func Test_parseTimestamp(t *testing.T) {
	today := time.Now().Format("2006-01-02")

	testcases := []struct {
		valid bool
		in    string
		want  string
	}{
		{valid: true, in: "2021-01-23 05:10:20", want: "2021-01-23 05:10:20"}, // full timestamp
		{valid: true, in: "2021-01-23", want: "2021-01-23 00:00:00"},          // date with no time
		{valid: true, in: "12:11:30", want: today + " 12:11:30"},              // time with no date
		{valid: false, in: "2021-01-23 12:11:30 garbage"},                     // time with no date
		{valid: false, in: "2021-01-32"},
		{valid: false, in: "2021-00-23"},
		{valid: false, in: "2021-13-23"},
		{valid: false, in: "12:11:60"},
		{valid: false, in: "12:60:30"},
		{valid: false, in: "24:11:30"},
		{valid: false, in: "invalid"},
		{valid: false, in: "2021-01-"},
		{valid: false, in: "2021-01"},
		{valid: false, in: "2021-"},
		{valid: false, in: "2021"},
		{valid: false, in: "12:11:"},
		{valid: false, in: "12:11"},
		{valid: false, in: "12:"},
		{valid: false, in: "12"},
		{valid: false, in: ""},
	}

	for _, tc := range testcases {
		got, err := parseTimestamp(tc.in)
		if tc.valid {
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got.Format("2006-01-02 15:04:05"))
		} else {
			assert.Error(t, err)
		}
	}
}

func Test_parseTimepart(t *testing.T) {
	today := time.Now().Format("2006-01-02")

	testcases := []struct {
		valid bool
		in    string
		want  string
	}{
		{valid: true, in: "2021-01-23", want: "2021-01-23 00:00:00"}, // date with no time
		{valid: true, in: "12:11:30", want: today + " 12:11:30"},     // time with no date
		{valid: false, in: "2021-01-32"},
		{valid: false, in: "2021-00-23"},
		{valid: false, in: "2021-13-23"},
		{valid: false, in: "12:11:60"},
		{valid: false, in: "12:60:30"},
		{valid: false, in: "24:11:30"},
		{valid: false, in: "invalid"},
		{valid: false, in: "2021-01-"},
		{valid: false, in: "2021-01"},
		{valid: false, in: "2021-"},
		{valid: false, in: "2021"},
		{valid: false, in: "12:11:"},
		{valid: false, in: "12:11"},
		{valid: false, in: "12:"},
		{valid: false, in: "12"},
	}

	for _, tc := range testcases {
		got, err := parseTimepart(tc.in)
		if tc.valid {
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got.Format("2006-01-02 15:04:05"))
		} else {
			assert.Error(t, err)
		}
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
