package view

import (
	"github.com/lesovsky/pgcenter/internal/query"
	"regexp"
)

// View describes how stats received from Postgres should be displayed.
type View struct {
	Name      string                 // Context name
	Query     string                 // Query used by default
	DiffIntvl [2]int                 // Columns interval for diff
	Cols      []string               // Columns names
	Ncols     int                    // Number of columns returned by query, used as a right border for OrderKey
	OrderKey  int                    // Number of column used for order
	OrderDesc bool                   // Order direction: descending (true) or ascending (false)
	UniqueKey int                    // Unique key that used on rows comparing when building diffs, by default it's zero which is OK in almost all contexts
	ColsWidth map[int]int            // Set width for columns and control an aligning
	Aligned   bool                   // Is aligning calculated?
	Msg       string                 // Show this text in Cmdline when switching to this unit
	Filters   map[int]*regexp.Regexp // Storage for filter patterns: key is the column index, value - regexp pattern
}

// Views is a list of all used context units.
type Views map[string]View

func New() Views {
	return map[string]View{
		"databases": {
			Name:      "databases",
			Query:     query.PgStatDatabaseQueryDefault,
			DiffIntvl: [2]int{1, 16},
			Ncols:     18,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show databases statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		"replication": {
			Name:      "replication",
			Query:     query.PgStatReplicationQueryDefault,
			DiffIntvl: [2]int{6, 6},
			Ncols:     15,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show replication statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		// PgStatTablesUnit describes how to handle pg_stat_all_tables and pg_statio_all_tables views
		"tables": {
			Name:      "tables",
			Query:     query.PgStatTablesQueryDefault,
			DiffIntvl: [2]int{1, 18},
			Ncols:     19,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show tables statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		// PgStatIndexesUnit describes how to handle pg_stat_all_indexes and pg_statio_all_indexes views
		"indexes": {
			Name:      "indexes",
			Query:     query.PgStatIndexesQueryDefault,
			DiffIntvl: [2]int{1, 5},
			Ncols:     6,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show indexes statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		// PgTablesSizesUnit describes how to handle statistics about tables sizes
		"sizes": {
			Name:      "sizes",
			Query:     query.PgTablesSizesQueryDefault,
			DiffIntvl: [2]int{4, 6},
			Ncols:     7,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show tables sizes statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		// PgStatFunctionsUnit describes how to handle pg_stat_user_functions view
		"functions": {
			Name:      "functions",
			Query:     query.PgStatFunctionsQueryDefault,
			DiffIntvl: [2]int{3, 3},
			Ncols:     8,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show functions statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		// PgStatVacuumUnit describes how to handle pg_stat_progress_vacuum view
		"progress_vacuum": {
			Name:      "progress_vacuum",
			Query:     query.PgStatProgressVacuumQueryDefault,
			DiffIntvl: [2]int{10, 11},
			Ncols:     13,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show vacuum progress statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		// PgStatProgressClusterUnit describes how to handle pg_stat_progress_cluster view
		"progress_cluster": {
			Name:      "progress_cluster",
			Query:     query.PgStatProgressClusterQueryDefault,
			DiffIntvl: [2]int{10, 11},
			Ncols:     13,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show cluster/vacuum full progress statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		"progress_index": {
			Name:      "progress_index",
			Query:     query.PgStatProgressCreateIndexQueryDefault,
			DiffIntvl: [2]int{0, 0},
			Ncols:     14,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show create index/reindex progress statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		// PgStatActivityUnit describes how to handle pg_stat_activity view
		"activity": {
			Name:      "activity",
			Query:     query.PgStatActivityQueryDefault,
			DiffIntvl: [2]int{0, 0},
			Ncols:     14,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show activity statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		// PgSSTimingUnit describes how to handle pg_stat_statements view with timing stats
		"statements_timings": {
			Name:      "statements_timings",
			Query:     query.PgStatStatementsTimingQueryDefault,
			DiffIntvl: [2]int{6, 10},
			Ncols:     13,
			OrderKey:  0,
			OrderDesc: true,
			UniqueKey: 11,
			ColsWidth: map[int]int{},
			Msg:       "Show statements timings statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		// PgSSGeneralUnit describes how to handle pg_stat_statements view with general stats
		"statements_general": {
			Name:      "statements_general",
			Query:     query.PgStatStatementsGeneralQueryDefault,
			DiffIntvl: [2]int{4, 5},
			Ncols:     8,
			OrderKey:  0,
			OrderDesc: true,
			UniqueKey: 6,
			ColsWidth: map[int]int{},
			Msg:       "Show statements general statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		// PgSSIoUnit describes how to handle pg_stat_statements view with stats related to buffers IO
		"statements_io": {
			Name:      "statements_io",
			Query:     query.PgStatStatementsIoQueryDefault,
			DiffIntvl: [2]int{6, 10},
			Ncols:     13,
			OrderKey:  0,
			OrderDesc: true,
			UniqueKey: 11,
			ColsWidth: map[int]int{},
			Msg:       "Show statements IO statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		// PgSSTempUnit describes how to handle pg_stat_statements view with stats related to temp files IO
		"statements_temp": {
			Name:      "statements_temp",
			Query:     query.PgStatStatementsTempQueryDefault,
			DiffIntvl: [2]int{4, 6},
			Ncols:     9,
			OrderKey:  0,
			OrderDesc: true,
			UniqueKey: 7,
			ColsWidth: map[int]int{},
			Msg:       "Show statements temp files statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		// PgSSLocalUnit describes how to handle pg_stat_statements view with stats related to local buffers IO
		"statements_local": {
			Name:      "statements_local",
			Query:     query.PgStatStatementsLocalQueryDefault,
			DiffIntvl: [2]int{6, 10},
			Ncols:     13,
			OrderKey:  0,
			OrderDesc: true,
			UniqueKey: 11,
			ColsWidth: map[int]int{},
			Msg:       "Show statements temp tables statistics (local IO)",
			Filters:   map[int]*regexp.Regexp{},
		},
	}
}

// Configure performs adjusting of queries accordingly to Postgres version
func (v Views) Configure(version int, trackCommit string) {
	var track bool
	if trackCommit == "on" {
		track = true
	}
	for k, view := range v {
		switch k {
		case "activity":
			switch {
			case version < 90600:
				view.Query = query.PgStatActivityQuery95
				view.Ncols = 12
				v[k] = view
			case version < 100000:
				view.Query = query.PgStatActivityQuery96
				view.Ncols = 13
				v[k] = view
			}
		case "replication":
			switch {
			case version < 90500:
				// Use query for 9.6 but with no 'track_commit_timestamp' fields
				view.Query = query.PgStatReplicationQuery96
				view.Ncols = 12
				v[k] = view
			case version < 100000:
				// Check is 'track_commit_timestamp' enabled or not and use corresponding query for 9.6
				if track {
					view.Query = query.PgStatReplicationQuery96Extended
					view.Ncols = 14
				} else {
					view.Query = query.PgStatReplicationQuery96
					view.Ncols = 12
				}
				v[k] = view
			default:
				// Check is 'track_commit_timestamp' enabled or not and use corresponding query for 10 and above
				if track {
					view.Query = query.PgStatReplicationQueryExtended
					view.Ncols = 17
				} else {
					// use defaults assigned in context unit
				}
				v[k] = view
			}
		case "databases":
			switch {
			// versions prior 12 don't have  checksum_failures column
			case version < 120000:
				view.Query = query.PgStatDatabaseQuery11
				view.Ncols = 17
				view.DiffIntvl = [2]int{1, 15}
				v[k] = view
			}
		case "statements_timings":
			switch {
			case version < 130000:
				view.Query = query.PgStatStatementsTimingQuery12
				v[k] = view
			}
		}
	}
}
