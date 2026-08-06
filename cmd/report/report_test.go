package report

import (
	"github.com/lesovsky/pgcenter/report"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"testing"
	"time"
)

func Test_options_validate(t *testing.T) {
	testcases := []struct {
		valid bool
		opts  options
		want  report.Config
	}{
		{valid: true, opts: options{showActivity: true, tsStart: "2021-01-01 12:00:00", tsEnd: "2021-01-01 13:00:00"}},
		{valid: true, opts: options{showActivity: true, tsStart: "2021-01-01 12:00:00", tsEnd: "2021-01-01 13:00:00"}},
		{valid: false, opts: options{tsStart: "2021-01-01 12:00:00", tsEnd: "2021-01-01 13:00:00"}}, // no report type specified
		{valid: false, opts: options{showActivity: true, tsStart: "2021-01-32"}},                    // invalid report start timestamp
		{valid: false, opts: options{showActivity: true, filter: `colname:"["`}},                    // invalid regexp
		{valid: false, opts: options{showWAL: "-f"}},                                                // unmapped -W value, see Test_selectReport_WALWhitelistIsClosed
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
		{opts: options{showDatabases: "g"}, want: "databases_general"},
		{opts: options{showDatabases: "s"}, want: "databases_sessions"},
		{opts: options{showTables: true}, want: "tables"},
		{opts: options{showIndexes: true}, want: "indexes"},
		{opts: options{showFunctions: true}, want: "functions"},
		{opts: options{showWAL: "w"}, want: "wal"},
		{opts: options{showWAL: "a"}, want: "archiver"},
		{opts: options{showSizes: true}, want: "sizes"},
		{opts: options{showStatements: "m"}, want: "statements_timings"},
		{opts: options{showStatements: "g"}, want: "statements_general"},
		{opts: options{showStatements: "i"}, want: "statements_io"},
		{opts: options{showStatements: "t"}, want: "statements_temp"},
		{opts: options{showStatements: "l"}, want: "statements_local"},
		{opts: options{showStatements: "w"}, want: "statements_wal"},
		{opts: options{showProgress: "v"}, want: "progress_vacuum"},
		{opts: options{showProgress: "c"}, want: "progress_cluster"},
		{opts: options{showProgress: "i"}, want: "progress_index"},
		{opts: options{showProgress: "a"}, want: "progress_analyze"},
		{opts: options{showProgress: "b"}, want: "progress_basebackup"},
		{opts: options{showProgress: "y"}, want: "progress_copy"},
		{opts: options{showBgwriter: true}, want: "bgwriter"},
		{opts: options{showReplSlots: true}, want: "replslots"},
		{opts: options{showStatIO: "c"}, want: "stat_io"},
		{opts: options{showStatIO: "t"}, want: "stat_io_time"},
		{opts: options{showStatements: "j"}, want: "statements_jit"},
		{opts: options{showStatIO: "x"}, want: ""},     // invalid -J value
		{opts: options{showStatements: "z"}, want: ""}, // invalid -X value
		{opts: options{}, want: ""},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, selectReport(tc.opts))
	}
}

// Test_selectReport_WALWhitelistIsClosed proves the -W mapping is a closed whitelist: only 'w' and
// 'a' map, everything else yields "". This matters because ReportType is not an inert label - it is
// the tar-entry filter in report.isFilenameOK and the key into the view map in report.newApp, so a
// value leaking through would select a zero-value view and print a silently empty report instead of
// erroring.
func Test_selectReport_WALWhitelistIsClosed(t *testing.T) {
	testcases := []struct {
		value string
		why   string
	}{
		{value: "c", why: "valid for -J (pg_stat_io count) - a user mixing up flags would get a wal report"},
		{value: "t", why: "valid for -J (pg_stat_io time) and is also -t (strlimit) - same mix-up"},
		{value: "g", why: "valid for -D and -X - and is also -g (grep) shorthand"},
		{value: "W", why: "case variant of the flag letter itself - no case normalisation is done on purpose"},
		{value: "wal", why: "spelled-out report type - no full-word aliases on purpose"},
		{value: "archiver", why: "spelled-out report type - no full-word aliases on purpose"},
		{value: "x", why: "arbitrary typo - the plain unknown-value case"},
		{value: "-f", why: "the value pflag assigns on the legacy 'pgcenter report -W -f dump.tar' invocation"},
		{value: "w ", why: "trailing whitespace - no trimming is done on purpose, so a quoted '-W \"w \"' must not map"},
		{value: " a", why: "leading whitespace - the other half of the no-trimming rule; both sides must stay closed"},
	}

	for _, tc := range testcases {
		assert.Equal(t, "", selectReport(options{showWAL: tc.value}), "value %q must not map: %s", tc.value, tc.why)
	}

	// The exact error message users see on the legacy '-W -f dump.tar' shape; it is quoted verbatim
	// in the release notes. Test_options_validate's table has no field for a message, so the literal
	// is pinned here.
	_, err := options{showWAL: "-f"}.validate()
	assert.EqualError(t, err, "report type is not specified, quit")

	// The mapped values must survive validate() into Config.ReportType unchanged. selectReport being
	// correct is not enough on its own: ReportType is what keys the view map and filters tar entries,
	// so anything rewriting it in between (a stray 'archiver' -> 'wal' alias, say) is the same
	// silently-wrong-report failure the whitelist exists to prevent.
	cfg, err := options{showWAL: "a"}.validate()
	assert.NoError(t, err)
	assert.Equal(t, "archiver", cfg.ReportType)

	cfg, err = options{showWAL: "w"}.validate()
	assert.NoError(t, err)
	assert.Equal(t, "wal", cfg.ReportType)
}

// Test_selectReport_WALPrecedence pins both boundaries of the -W arm's slot in the first-match-wins
// switch chain: it must stay below showFunctions and above showBgwriter. Asserting only that -A wins
// would leave the arm free to move anywhere below showActivity without reddening a test.
func Test_selectReport_WALPrecedence(t *testing.T) {
	testcases := []struct {
		opts options
		want string
		why  string
	}{
		{opts: options{showActivity: true, showWAL: "a"}, want: "activity", why: "-A heads the chain and must keep beating -W"},
		{opts: options{showDatabases: "g", showWAL: "a"}, want: "databases_general", why: "the arm may not rise above showDatabases"},
		{opts: options{showFunctions: true, showWAL: "a"}, want: "functions", why: "the arm may not rise above showFunctions, its upper neighbour"},
		{opts: options{showWAL: "a", showBgwriter: true}, want: "archiver", why: "the arm may not sink below showBgwriter, its lower neighbour"},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, selectReport(tc.opts), "flag precedence changed: %s", tc.why)
	}
}

// Test_walFlagDefinition guards the user-visible shape of -W: its type, shorthand, default and the
// help text printed by 'pgcenter report --help'. It is also what keeps the local FlagSet mirror in
// Test_walFlagPflagFailureShapes honest.
func Test_walFlagDefinition(t *testing.T) {
	f := CommandDefinition.Flags().Lookup("wal")
	// require, not assert: the assertions below dereference f, so a renamed flag must stop this test
	// rather than panic and take the rest of the package's tests down with it.
	require.NotNil(t, f)
	assert.Equal(t, "string", f.Value.Type())
	assert.Equal(t, "W", f.Shorthand)
	assert.Equal(t, "", f.DefValue)
	assert.Equal(t, "show pg_stat_wal / pg_stat_archiver report (w - wal, a - archiver)", f.Usage)
	// NoOptDefVal is what decides whether -W demands an argument at all, so it is the property the
	// pflag mirror below actually rests on. A non-empty value is the cobra shim Decision 7 rejected:
	// it keeps bare '-W' working while making '-W a' silently drop the 'a' and report wal.
	assert.Equal(t, "", f.NoOptDefVal, "-W must demand an argument; a non-empty NoOptDefVal is the rejected shim from Decision 7")
}

// Test_walFlagPflagFailureShapes pins the two user-visible failure shapes of the breaking change
// from a boolean -W to a string one. Parsing runs through a locally constructed FlagSet that mirrors
// the -W and -f definitions - CommandDefinition's flags are bound to the package-level opts var, so
// parsing through them would mutate shared state and make the suite order-dependent.
// Test_walFlagDefinition is what keeps this mirror honest.
func Test_walFlagPflagFailureShapes(t *testing.T) {
	newMirror := func() (*pflag.FlagSet, *string) {
		fs := pflag.NewFlagSet("report", pflag.ContinueOnError)
		fs.SetOutput(io.Discard)
		wal := fs.StringP("wal", "W", "", "show pg_stat_wal / pg_stat_archiver report (w - wal, a - archiver)")
		fs.StringP("file", "f", "pgcenter.stat.tar", "read stats from file")
		return fs, wal
	}

	// '-W' as the last token: pflag errors out before RunE ever runs. The message is pflag's own.
	fs, _ := newMirror()
	assert.EqualError(t, fs.Parse([]string{"-W"}), "flag needs an argument: 'W' in -W")

	// '-W -f dump.tar' - the shape legacy scripts have. A string flag consumes the next token
	// unconditionally, so '-f' lands in -W's value even though -f is itself a defined flag: there is
	// no flag error at all and the whitelist is the only thing that catches it. ('-f' is defined in
	// the mirror to keep the scenario realistic, not because the outcome depends on it.)
	fs, wal := newMirror()
	assert.NoError(t, fs.Parse([]string{"-W", "-f", "dump.tar"}))
	assert.Equal(t, "-f", *wal)
	assert.Equal(t, "", selectReport(options{showWAL: *wal}))
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
