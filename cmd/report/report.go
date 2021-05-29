// Entry point for 'pgcenter report' command.

package report

import (
	"fmt"
	"github.com/lesovsky/pgcenter/report"
	"github.com/spf13/cobra"
	"regexp"
	"strings"
	"time"
)

// options defines all user-requested startup options.
type options struct {
	describe bool // Describe stats fields

	showActivity    bool   // Show stats from pg_stat_activity
	showReplication bool   // Show stats from pg_stat_replication
	showDatabases   bool   // Show stats from pg_stat_database
	showTables      bool   // Show stats from pg_stat_user_tables, pg_statio_user_tables
	showIndexes     bool   // Show stats from pg_stat_user_indexes, pg_statio_user_indexes
	showSizes       bool   // Show tables sizes
	showFunctions   bool   // Show stats from pg_stat_user_functions
	showStatements  string // Show stats from pg_stat_statements
	showProgress    string // Show stats from pg_stat_progress_* stats

	inputFile      string        // Input file with statistics
	tsStart, tsEnd string        // Show stats within an interval
	orderColName   string        // Name of the column used for sorting
	orderDesc      bool          // Specify to use descendant order
	orderAsc       bool          // Specify to use ascendant order
	filter         string        // Perform filtering
	rowLimit       int           // Number of rows per timestamp
	strLimit       int           // Trim all strings longer than this limit
	rate           time.Duration // Stats rate
}

var (
	opts options

	// CommandDefinition is the definition of 'report' CLI sub-command
	CommandDefinition = &cobra.Command{
		Use:   "report",
		Short: "make report based on previously saved statistics",
		Long:  `'pgcenter report' reads statistics from file and prints reports.`,
		RunE: func(command *cobra.Command, _ []string) error {
			reportOpts, err := opts.validate()
			if err != nil {
				return err
			}

			return report.RunMain(reportOpts)
		},
	}
)

func init() {
	CommandDefinition.Flags().BoolVarP(&opts.describe, "describe", "d", false, "describe columns of specified statistics")
	CommandDefinition.Flags().BoolVarP(&opts.showActivity, "activity", "A", false, "show pg_stat_activity report")
	CommandDefinition.Flags().BoolVarP(&opts.showReplication, "replication", "R", false, "show pg_stat_replication report")
	CommandDefinition.Flags().BoolVarP(&opts.showDatabases, "databases", "D", false, "show pg_stat_database report")
	CommandDefinition.Flags().BoolVarP(&opts.showTables, "tables", "T", false, "show pg_stat_user_tables and pg_statio_user_tables report")
	CommandDefinition.Flags().BoolVarP(&opts.showIndexes, "indexes", "I", false, "show pg_stat_user_indexes and pg_statio_user_indexes report")
	CommandDefinition.Flags().BoolVarP(&opts.showSizes, "sizes", "S", false, "show tables sizes report")
	CommandDefinition.Flags().BoolVarP(&opts.showFunctions, "functions", "F", false, "show pg_stat_user_functions report")
	CommandDefinition.Flags().StringVarP(&opts.showStatements, "statements", "X", "", "show pg_stat_statements report")
	CommandDefinition.Flags().StringVarP(&opts.showProgress, "progress", "P", "", "show pg_stat_progress_* report")

	CommandDefinition.Flags().StringVarP(&opts.inputFile, "file", "f", "pgcenter.stat.tar", "read stats from file")
	CommandDefinition.Flags().StringVarP(&opts.tsStart, "start", "s", "", "starting time of the report")
	CommandDefinition.Flags().StringVarP(&opts.tsEnd, "end", "e", "", "ending time of the report")
	CommandDefinition.Flags().StringVarP(&opts.orderColName, "order", "o", "", "sort values by column using descendant order")
	CommandDefinition.Flags().BoolVarP(&opts.orderDesc, "desc", "", true, "sort values by column using descendant order")
	CommandDefinition.Flags().BoolVarP(&opts.orderAsc, "asc", "", false, "sort values by column using ascendant order")
	CommandDefinition.Flags().StringVarP(&opts.filter, "grep", "g", "", "grep values in specified column (format: colname:filter_pattern)")
	CommandDefinition.Flags().IntVarP(&opts.rowLimit, "limit", "l", 0, "print only limited number of rows per sample")
	CommandDefinition.Flags().IntVarP(&opts.strLimit, "strlimit", "t", 32, "maximum string size for long lines to print (default: 32)")
	CommandDefinition.Flags().DurationVarP(&opts.rate, "rate", "r", time.Second, "statistics changes rate interval (default: 1s)")
}

// validate parses and validates options passed by user and returns options ready for 'pgcenter report'.
func (opts options) validate() (report.Config, error) {
	// Select report type
	r := selectReport(opts)
	if r == "" {
		return report.Config{}, fmt.Errorf("report type is not specified, quit")
	}

	if opts.rate < time.Second {
		fmt.Println("INFO: round rate interval to minimum allowed 1 second.")
		opts.rate = time.Second
	}

	// Define report start/end interval.
	tsStart, tsEnd, err := setReportInterval(opts.tsStart, opts.tsEnd)
	if err != nil {
		return report.Config{}, err
	}

	// Compile regexp if specified.
	colname, re, err := parseFilterString(opts.filter)
	if err != nil {
		return report.Config{}, err
	}

	// Define order settings.
	desc := opts.orderDesc
	if opts.orderAsc {
		desc = false
	}

	return report.Config{
		Describe:      opts.describe,
		ReportType:    r,
		InputFile:     opts.inputFile,
		TsStart:       tsStart,
		TsEnd:         tsEnd,
		OrderColName:  opts.orderColName,
		OrderDesc:     desc,
		FilterColName: colname,
		FilterRE:      re,
		RowLimit:      opts.rowLimit,
		TruncLimit:    opts.strLimit,
		Rate:          opts.rate,
	}, nil
}

// selectReport selects appropriate type of the report depending on user's choice.
func selectReport(opts options) string {
	switch {
	case opts.showActivity:
		return "activity"
	case opts.showReplication:
		return "replication"
	case opts.showDatabases:
		return "databases"
	case opts.showTables:
		return "tables"
	case opts.showIndexes:
		return "indexes"
	case opts.showFunctions:
		return "functions"
	case opts.showSizes:
		return "sizes"
	case opts.showStatements != "":
		switch opts.showStatements {
		case "m":
			return "statements_timings"
		case "g":
			return "statements_general"
		case "i":
			return "statements_io"
		case "t":
			return "statements_temp"
		case "l":
			return "statements_local"
		case "w":
			return "statements_wal"
		}
	case opts.showProgress != "":
		switch opts.showProgress {
		case "v":
			return "progress_vacuum"
		case "c":
			return "progress_cluster"
		case "i":
			return "progress_index"
		case "a":
			return "progress_analyze"
		case "b":
			return "progress_basebackup"
		case "y":
			return "progress_copy"
		}
	}

	return ""
}

// setReportInterval parses user-defined timestamp and returns start/end time.Times for report.
func setReportInterval(tsStartStr, tsEndStr string) (time.Time, time.Time, error) {
	var tsStart, tsEnd time.Time
	var err error

	// Parse start time string
	if tsStartStr != "" {
		tsStart, err = parseTimestamp(tsStartStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	} else {
		tsStart, err = time.ParseInLocation("2006-01-02 15:04:05", "0001-01-01 00:00:00", time.Now().Location())
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}

	// Parse end time string
	if tsEndStr != "" {
		tsEnd, err = parseTimestamp(tsEndStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	} else {
		tsEnd = time.Now()
	}

	return tsStart, tsEnd, nil
}

// parseTimestamp parses timestamp string and returns time.Time
func parseTimestamp(ts string) (time.Time, error) {
	if ts == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}

	parts := strings.Split(ts, " ")

	switch len(parts) {
	case 1:
		t, err := parseTimepart(parts[0])
		if err != nil {
			return time.Time{}, err
		}
		return t, nil
	case 2:
		t, err := time.ParseInLocation("2006-01-02 15:04:05", ts, time.Now().Location())
		if err != nil {
			return time.Time{}, err
		}
		return t, nil
	default:
		return time.Time{}, fmt.Errorf("bad timestamp")
	}
}

// parseTimepart parses string considered as date or time and return local timestamp. Time with no date considered as today.
func parseTimepart(s string) (time.Time, error) {
	loc := time.Now().Location()

	if parts := strings.Split(s, "-"); len(parts) == 3 {
		d, err := time.ParseInLocation("2006-01-02", s, loc)
		if err != nil {
			return time.Time{}, err
		}

		return d, nil
	}

	if parts := strings.Split(s, ":"); len(parts) == 3 {
		today := time.Now().Format("2006-01-02")
		t, err := time.ParseInLocation("2006-01-02 15:04:05", fmt.Sprintf("%s %s", today, s), loc)
		if err != nil {
			return time.Time{}, err
		}

		return t, nil
	}

	return time.Time{}, fmt.Errorf("invalid date/time: %s", s)
}

// parseFilterString parses and defines filtering options. Split a value entered by user to column name and filter pattern.
func parseFilterString(filter string) (string, *regexp.Regexp, error) {
	if filter == "" {
		return "", nil, nil
	}

	s := strings.SplitN(filter, ":", 2)
	if len(s) != 2 || (s[0] == "" || s[1] == "") {
		return "", nil, fmt.Errorf("invalid filter specified")
	}

	colname := s[0]

	re, err := regexp.Compile(s[1])
	if err != nil {
		return "", nil, err
	}

	return colname, re, nil
}
