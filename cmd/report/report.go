// Entry point for 'pgcenter report' command

package report

import (
	"fmt"
	"github.com/lesovsky/pgcenter/lib/stat"
	"github.com/lesovsky/pgcenter/report"
	"github.com/spf13/cobra"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	defaultReportFile = "pgcenter.stat.tar"
	filterDelimiter   = ":"
)

var (
	opts           report.ReportOptions // Settings for the report program
	tsStart, tsEnd string               // Show stats within an interval
	doFilter       string               // Perform filtering

	showActivity    bool   // Show stats from pg_stat_activity
	showDatabases   bool   // Show stats from pg_stat_database
	showFunctions   bool   // Show stats from pg_stat_user_functions
	showReplication bool   // Show stats from pg_stat_replication
	showTables      bool   // Show stats from pg_stat_user_tables, pg_statio_user_tables
	showIndexes     bool   // Show stats from pg_stat_user_indexes, pg_statio_user_indexes
	showVacuum      bool   // Show stats from pg_stat_progress_vacuum
	showStatements  string // Show stats from pg_stat_statements
	showSizes       bool   // Show tables sizes
	describe        bool   // Show description of requested stats view

	CommandDefinition = &cobra.Command{
		Use:     "report",
		Short:   "make report based on previously saved statistics",
		Long:    `'pgcenter report' reads statistics from file and prints reports.`,
		Version: "dummy", // use constants from 'cmd' package
		PreRun:  preFlightSetup,
		Run: func(command *cobra.Command, args []string) {
			report.RunMain(args, opts)
		},
	}
)

func init() {
	CommandDefinition.Flags().StringVarP(&opts.InputFile, "file", "f", defaultReportFile, "read stats from file")
	CommandDefinition.Flags().BoolVarP(&showActivity, "activity", "A", false, "show pg_stat_activity stats")
	CommandDefinition.Flags().BoolVarP(&showSizes, "sizes", "S", false, "show tables sizes stats")
	CommandDefinition.Flags().BoolVarP(&showDatabases, "databases", "D", false, "show pg_stat_database stats")
	CommandDefinition.Flags().BoolVarP(&showFunctions, "functions", "F", false, "show pg_stat_user_functions stats")
	CommandDefinition.Flags().BoolVarP(&showReplication, "replication", "R", false, "show pg_stat_replication stats")
	CommandDefinition.Flags().BoolVarP(&showTables, "tables", "T", false, "show pg_stat_user_tables and pg_statio_user_tables stats")
	CommandDefinition.Flags().BoolVarP(&showIndexes, "indexes", "I", false, "show pg_stat_user_indexes and pg_statio_user_indexes stats")
	CommandDefinition.Flags().BoolVarP(&showVacuum, "vacuum", "V", false, "show pg_stat_progress_vacuum stats")
	CommandDefinition.Flags().StringVarP(&showStatements, "statements", "X", "", "show pg_stat_statements stats")
	CommandDefinition.Flags().StringVarP(&tsStart, "start", "s", "", "starting time of the report")
	CommandDefinition.Flags().StringVarP(&tsEnd, "end", "e", "", "ending time of the report")
	CommandDefinition.Flags().StringVarP(&opts.OrderColName, "order", "o", "", "order values by column (desc by default)")
	CommandDefinition.Flags().IntVarP(&opts.RowLimit, "limit", "l", 0, "print only limited number of rows per sample")
	CommandDefinition.Flags().StringVarP(&doFilter, "grep", "g", "", "grep values in specified column (format: colname:filtertext)")
	CommandDefinition.Flags().IntVarP(&opts.TruncLimit, "truncate", "t", 32, "maximum string size to print")
	CommandDefinition.Flags().BoolVarP(&describe, "describe", "d", false, "describe columns of specified statistics")
	CommandDefinition.Flags().DurationVarP(&opts.Interval, "interval", "i", 1*time.Second, "delta interval (default: 1s)")
}

// Analyze startup parameters and prepare settings for report program
func preFlightSetup(_ *cobra.Command, _ []string) {
	// use descending order by default
	opts.OrderDesc = true

	// setup starting and ending times
	checkStartEndTimestamps()

	// determine column name where values should be filtered and compile regexp
	parseFilterString()

	// select appropriate report and context with settings
	selectReport()

	// if user asks to describe a stat view, show a description and exit
	if describe {
		doDescribe()
	}
}

// Setup start and end times for report, don't show stats before start time and after end time
func checkStartEndTimestamps() {
	var err error
	var layout = "20060102-150405" // default layout includes date and time

	// if start time is not specified, default value will be used - 0001-01-01 00:00:00
	// if user specified start time, try to split timestamp to date and time, if date not found use today-date.
	if tsStart != "" {
		tsStartParts := strings.Split(tsStart, "-")
		if len(tsStartParts) == 1 { // only time specified without date
			var today time.Time
			if today, err = time.Parse("20060102", time.Now().Format("20060102")); err != nil {
				fmt.Printf("ERROR: failed parse today to date: %s", err)
			}
			tsStart = fmt.Sprint(today.Format("20060102") + "-" + tsStart)
		}

		// prepare start time for report program
		opts.TsStart, err = time.Parse(layout, tsStart)
		if err != nil {
			fmt.Printf("WARNING: invalid start time: %s, ignoring... (default: %s)\n", tsStart, opts.TsStart)
		}
	}

	// use current date and time (now) if end time is not specified
	// Here is dirty trick is used for dropping timezone from time returned from time.Now(). At first, translate value
	// to a string and then parse that string to a time.Time back.
	// In fact, we don't need info about time zone, because we relies on timestamp from stats file.
	if opts.TsEnd, err = time.Parse(layout, time.Now().Format(layout)); err != nil {
		fmt.Printf("ERROR: failed time parse: %s", err)
	}

	if tsEnd != "" {
		tsEndParts := strings.Split(tsEnd, "-")
		if len(tsEndParts) == 1 { // only time specified without date
			var today time.Time
			if today, err = time.Parse("20060102", time.Now().Format("20060102")); err != nil {
				fmt.Printf("ERROR: failed parse today to date: %s", err)
			}
			tsEnd = fmt.Sprint(today.Format("20060102") + "-" + tsEnd)
		}

		// prepare end time for report program
		opts.TsEnd, err = time.Parse(layout, tsEnd)
		if err != nil {
			// if failed to parse, use time.Now as default
			if opts.TsEnd, err = time.Parse(layout, time.Now().Format(layout)); err != nil {
				fmt.Printf("ERROR: failed time parse: %s", err)
			}
			fmt.Printf("WARNING: invalid end time: %s, ignoring... (default: %s)\n", tsEnd, opts.TsEnd)
		}
	}
}

// Setup filtering options. Split a value entered by user to column name and filter pattern.
func parseFilterString() {
	if doFilter != "" {
		var err error

		s := strings.SplitN(doFilter, filterDelimiter, 2)
		if len(s) == 2 {
			opts.FilterColName = s[0]
			if opts.Regexp, err = regexp.Compile(s[1]); err != nil {
				fmt.Printf("WARNING: failed to compile regexp: %s\n", err)
				opts.FilterColName = ""
			}
		} else {
			fmt.Println("WARNING: ignoring wrong input for --grep option, see usage for details")
			opts.FilterColName = ""
		}

		fmt.Printf("DEBUG: do filter -- colname: %s, pattern: %s\n", opts.FilterColName, s[1])
	}
}

// Select appropriate type of the report
func selectReport() {
	switch {
	case showDatabases == true:
		opts.ReportType = stat.DatabaseView
		opts.Context = stat.PgStatDatabaseUnit
	case showTables == true:
		opts.ReportType = stat.TablesView
		opts.Context = stat.PgStatDatabaseUnit
	case showReplication == true:
		opts.ReportType = stat.ReplicationView
		opts.Context = stat.PgStatReplicationUnit
	case showIndexes == true:
		opts.ReportType = stat.IndexesView
		opts.Context = stat.PgStatIndexesUnit
	case showSizes == true:
		opts.ReportType = stat.SizesView
		opts.Context = stat.PgTablesSizesUnit
	case showFunctions == true:
		opts.ReportType = stat.FunctionsView
		opts.Context = stat.PgStatFunctionsUnit
	case showVacuum == true:
		opts.ReportType = stat.VacuumView
		opts.Context = stat.PgStatVacuumUnit
	case showActivity == true:
		opts.ReportType = stat.ActivityView
		opts.Context = stat.PgStatActivityUnit
	}

	if showStatements != "" {
		switch {
		case showStatements == "m":
			opts.ReportType = stat.StatementsTimingView
			opts.Context = stat.PgSSTimingUnit
		case showStatements == "g":
			opts.ReportType = stat.StatementsGeneralView
			opts.Context = stat.PgSSGeneralUnit
		case showStatements == "i":
			opts.ReportType = stat.StatementsIOView
			opts.Context = stat.PgSSIoUnit
		case showStatements == "t":
			opts.ReportType = stat.StatementsTempView
			opts.Context = stat.PgSSTempUnit
		case showStatements == "l":
			opts.ReportType = stat.StatementsLocalView
			opts.Context = stat.PgSSLocalUnit
		default:
			fmt.Printf("WARNING: unknown modificator '%s', ignore...\n", showStatements)
		}
	}
}

// Show columns description of the used stats
// TODO: feature not completed, other descriptions should be added
func doDescribe() {
	if describe {
		switch {
		case opts.ReportType == stat.DatabaseView:
			fmt.Println(stat.PgStatDatabaseDescription)
		default:
			fmt.Println("SORRY, NOT IMPLEMENTED YET")
		}
	}
	os.Exit(0)
}
