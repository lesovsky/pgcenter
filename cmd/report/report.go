// Entry point for 'pgcenter report' command

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
	showActivity    bool   // Show stats from pg_stat_activity
	showReplication bool   // Show stats from pg_stat_replication
	showDatabases   bool   // Show stats from pg_stat_database
	showTables      bool   // Show stats from pg_stat_user_tables, pg_statio_user_tables
	showIndexes     bool   // Show stats from pg_stat_user_indexes, pg_statio_user_indexes
	showFunctions   bool   // Show stats from pg_stat_user_functions
	showSizes       bool   // Show tables sizes
	showStatements  string // Show stats from pg_stat_statements
	showProgress    string // Show stats from pg_stat_progress_* stats

	tsStart, tsEnd string // Show stats within an interval
	filter         string // Perform filtering

	inputFile    string        // Input file with statistics
	orderColName string        // Name of the column used for sorting
	rowLimit     int           // Number of rows per timestamp
	strLimit     int           // Trim all strings longer than this limit
	interval     time.Duration // Interval between statistics
}

var (
	opts     options
	describe bool // Show description of requested stats view

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
	CommandDefinition.Flags().StringVarP(&opts.inputFile, "file", "f", "pgcenter.stat.tar", "read stats from file")
	CommandDefinition.Flags().StringVarP(&opts.orderColName, "order", "o", "", "order values by column (desc by default)")
	CommandDefinition.Flags().IntVarP(&opts.rowLimit, "limit", "l", 0, "print only limited number of rows per sample")
	CommandDefinition.Flags().IntVarP(&opts.strLimit, "strlimit", "t", 32, "maximum string size for long lines to print (default: 32)")
	CommandDefinition.Flags().DurationVarP(&opts.interval, "interval", "i", 1*time.Second, "delta interval (default: 1s)")

	CommandDefinition.Flags().BoolVarP(&opts.showActivity, "activity", "A", false, "show pg_stat_activity report")
	CommandDefinition.Flags().BoolVarP(&opts.showSizes, "sizes", "S", false, "show tables sizes report")
	CommandDefinition.Flags().BoolVarP(&opts.showDatabases, "databases", "D", false, "show pg_stat_database report")
	CommandDefinition.Flags().BoolVarP(&opts.showFunctions, "functions", "F", false, "show pg_stat_user_functions report")
	CommandDefinition.Flags().BoolVarP(&opts.showReplication, "replication", "R", false, "show pg_stat_replication report")
	CommandDefinition.Flags().BoolVarP(&opts.showTables, "tables", "T", false, "show pg_stat_user_tables and pg_statio_user_tables report")
	CommandDefinition.Flags().BoolVarP(&opts.showIndexes, "indexes", "I", false, "show pg_stat_user_indexes and pg_statio_user_indexes report")
	CommandDefinition.Flags().StringVarP(&opts.showProgress, "progress", "P", "", "show pg_stat_progress_* report")
	CommandDefinition.Flags().StringVarP(&opts.showStatements, "statements", "X", "", "show pg_stat_statements report")
	CommandDefinition.Flags().StringVarP(&opts.tsStart, "start", "s", "", "starting time of the report")
	CommandDefinition.Flags().StringVarP(&opts.tsEnd, "end", "e", "", "ending time of the report")
	CommandDefinition.Flags().StringVarP(&opts.filter, "grep", "g", "", "grep values in specified column (format: colname:filter_pattern)")

	CommandDefinition.Flags().BoolVarP(&describe, "describe", "d", false, "describe columns of specified statistics")
}

// validate parses and validates options passed by user and returns options ready for 'pgcenter report'.
func (opts options) validate() (report.Config, error) {
	// Select report type
	r := selectReport(opts)
	if r == "" {
		return report.Config{}, fmt.Errorf("report type is not specified, quit")
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

	return report.Config{
		InputFile:     opts.inputFile,
		TsStart:       tsStart,
		TsEnd:         tsEnd,
		OrderColName:  opts.orderColName,
		OrderDesc:     true,
		FilterColName: colname,
		FilterRE:      re,
		TruncLimit:    opts.strLimit,
		RowLimit:      opts.rowLimit,
		ReportType:    r,
		Interval:      opts.interval,
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
		}
	case opts.showProgress != "":
		switch opts.showProgress {
		case "v":
			return "progress_vacuum"
		case "c":
			return "progress_cluster"
		case "i":
			return "progress_index"
		}
	}

	return ""
}

// setReportInterval validates start and end times for report and returns start/end time.Time.
func setReportInterval(tsStartStr, tsEndStr string) (time.Time, time.Time, error) {
	layout := "20060102-150405" // default layout includes date and time

	// Processing report start timestamp.
	// if user specified start time, try to split timestamp to date and time, if date not found use today-date.
	// if start time is not specified, default value will be used - 0001-01-01 00:00:00
	var tsStart time.Time
	if tsStartStr != "" {
		tsStartParts := strings.Split(tsStartStr, "-")

		// only time specified without date - prepend timestamp with today
		if len(tsStartParts) == 1 {
			tsStartStr = fmt.Sprint(time.Now().Format("20060102") + "-" + tsStartStr)
		}

		// prepare start time for report program
		parsedStart, err := time.Parse(layout, tsStartStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		tsStart = parsedStart
	} else {
		// by default use "zero" which equals to 0001-01-01 00:00:00
		tsStart = time.Time{}
	}

	// Processing report end timestamp.
	// if user specified end time, try to split timestamp to date and time, if date not found use today-date.
	// Use current date and time (now) if end time is not specified
	var tsEnd time.Time
	if tsEndStr != "" {
		tsEndParts := strings.Split(tsEndStr, "-")

		// only time specified without date - prepend with today date
		if len(tsEndParts) == 1 {
			tsEndStr = fmt.Sprint(time.Now().Format("20060102") + "-" + tsEndStr)
		}

		// prepare end time for report program
		parsedEnd, err := time.Parse(layout, tsEndStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		tsEnd = parsedEnd
	} else {
		// use now() as default
		tsEnd = time.Now()
	}

	return tsStart, tsEnd, nil
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

// doDescribe shows detailed description of the requested stats
func doDescribe() {
	return
	//var m = map[string]string{
	//	stat.DatabaseView:            stat.PgStatDatabaseDescription,
	//	stat.ActivityView:            stat.PgStatActivityDescription,
	//	stat.ReplicationView:         stat.PgStatReplicationDescription,
	//	stat.TablesView:              stat.PgStatTablesDescription,
	//	stat.IndexesView:             stat.PgStatIndexesDescription,
	//	stat.FunctionsView:           stat.PgStatFunctionsDescription,
	//	stat.SizesView:               stat.PgStatSizesDescription,
	//	stat.ProgressVacuumView:      stat.PgStatProgressVacuumDescription,
	//	stat.ProgressClusterView:     stat.PgStatProgressClusterDescription,
	//	stat.ProgressCreateIndexView: stat.PgStatProgressCreateIndexDescription,
	//	stat.StatementsTimingView:    stat.PgStatStatementsTimingDescription,
	//	stat.StatementsGeneralView:   stat.PgStatStatementsGeneralDescription,
	//	stat.StatementsIOView:        stat.PgStatStatementsIODescription,
	//	stat.StatementsTempView:      stat.PgStatStatementsTempDescription,
	//	stat.StatementsLocalView:     stat.PgStatStatementsLocalDescription,
	//}
	//
	//if description, ok := m[opts.ReportType]; ok {
	//	fmt.Println(description)
	//} else {
	//	fmt.Println("Unknown description requested")
	//}
}
