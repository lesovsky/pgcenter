// Context defines what kind of stats should be displayed to user. Context described through 'contextUnit' structs.
// A single unit defines the following settings:
// 1. SQL query used to get stats;
// 2. Which columns should be displayed as a delta between current and previous stats, and which columns should be displayed as is;
// 3. Which order should be used -- descending or ascending;
// 4. patterns used for filtering.
// All available contexts grouped into a single list. When user switches between displayed stats, internally he switches
// between context units. Current context and its settings are saved into the list, and new context with its
// settings is loaded instead of current. Hence, all settings such as ordering, filtering amd others are permanent between switches.

package stat

import "regexp"

// ContextUnit describes a single unit of stats context.
type ContextUnit struct {
	Name      string                 // Context name
	Query     string                 // Query used by default
	DiffIntvl [2]int                 // Columns interval for diff
	Ncols     int                    // Number of columns returned by query, used as a right border for OrderKey
	OrderKey  int                    // Number of column used for order
	OrderDesc bool                   // Order direction: descending (true) or ascending (false)
	UniqueKey int                    // Unique key that used on rows comparing when building diffs, by default it's zero which is OK in almost all contexts
	ColsWidth map[int]int            // Set width for columns and control an aligning
	Aligned   bool                   // Is aligning calculated?
	Msg       string                 // Show this text in Cmdline when switching to this unit
	Filters   map[int]*regexp.Regexp // Storage for filter patterns: key is the column index, value - regexp pattern
}

// ContextList is a list of all used context units.
type ContextList map[string]*ContextUnit

var (
	// PgStatDatabaseUnit describes how to handle pg_stat_database view
	PgStatDatabaseUnit = ContextUnit{
		Name:      DatabaseView,
		Query:     PgStatDatabaseQueryDefault,
		DiffIntvl: [2]int{1, 16},
		Ncols:     18,
		OrderKey:  0,
		OrderDesc: true,
		ColsWidth: map[int]int{},
		Msg:       "Show databases statistics",
		Filters:   map[int]*regexp.Regexp{},
	}
	// PgStatReplicationUnit describes how to handle pg_stat_replication view
	PgStatReplicationUnit = ContextUnit{
		Name:      ReplicationView,
		Query:     PgStatReplicationQueryDefault,
		DiffIntvl: [2]int{6, 6},
		Ncols:     15,
		OrderKey:  0,
		OrderDesc: true,
		ColsWidth: map[int]int{},
		Msg:       "Show replication statistics",
		Filters:   map[int]*regexp.Regexp{},
	}
	// PgStatTablesUnit describes how to handle pg_stat_all_tables and pg_statio_all_tables views
	PgStatTablesUnit = ContextUnit{
		Name:      TablesView,
		Query:     PgStatTablesQueryDefault,
		DiffIntvl: [2]int{1, 18},
		Ncols:     19,
		OrderKey:  0,
		OrderDesc: true,
		ColsWidth: map[int]int{},
		Msg:       "Show tables statistics",
		Filters:   map[int]*regexp.Regexp{},
	}
	// PgStatIndexesUnit describes how to handle pg_stat_all_indexes and pg_statio_all_indeexs views
	PgStatIndexesUnit = ContextUnit{
		Name:      IndexesView,
		Query:     PgStatIndexesQueryDefault,
		DiffIntvl: [2]int{1, 5},
		Ncols:     6,
		OrderKey:  0,
		OrderDesc: true,
		ColsWidth: map[int]int{},
		Msg:       "Show indexes statistics",
		Filters:   map[int]*regexp.Regexp{},
	}
	// PgTablesSizesUnit describes how to handle statistics about tables sizes
	PgTablesSizesUnit = ContextUnit{
		Name:      SizesView,
		Query:     PgTablesSizesQueryDefault,
		DiffIntvl: [2]int{4, 6},
		Ncols:     7,
		OrderKey:  0,
		OrderDesc: true,
		ColsWidth: map[int]int{},
		Msg:       "Show tables sizes statistics",
		Filters:   map[int]*regexp.Regexp{},
	}
	// PgStatFunctionsUnit describes how to handle pg_stat_user_functions view
	PgStatFunctionsUnit = ContextUnit{
		Name:      FunctionsView,
		Query:     PgStatFunctionsQueryDefault,
		DiffIntvl: [2]int{3, 3},
		Ncols:     8,
		OrderKey:  0,
		OrderDesc: true,
		ColsWidth: map[int]int{},
		Msg:       "Show functions statistics",
		Filters:   map[int]*regexp.Regexp{},
	}
	// PgStatVacuumUnit describes how to handle pg_stat_progress_vacuum view
	PgStatProgressVacuumUnit = ContextUnit{
		Name:  ProgressVacuumView,
		Query: PgStatProgressVacuumQueryDefault,
		//DiffIntvl: NoDiff,
		DiffIntvl: [2]int{10, 11},
		Ncols:     13,
		OrderKey:  0,
		OrderDesc: true,
		ColsWidth: map[int]int{},
		Msg:       "Show vacuum progress statistics",
		Filters:   map[int]*regexp.Regexp{},
	}
	// PgStatProgressClusterUnit describes how to handle pg_stat_progress_cluster view
	PgStatProgressClusterUnit = ContextUnit{
		Name:  ProgressClusterView,
		Query: PgStatProgressClusterQueryDefault,
		//DiffIntvl: NoDiff,
		DiffIntvl: [2]int{10, 11},
		Ncols:     13,
		OrderKey:  0,
		OrderDesc: true,
		ColsWidth: map[int]int{},
		Msg:       "Show cluster/vacuum full progress statistics",
		Filters:   map[int]*regexp.Regexp{},
	}
	PgStatProgressCreateIndexUnit = ContextUnit{
		Name:      ProgressCreateIndexView,
		Query:     PgStatProgressCreateIndexQueryDefault,
		DiffIntvl: NoDiff,
		Ncols:     14,
		OrderKey:  0,
		OrderDesc: true,
		ColsWidth: map[int]int{},
		Msg:       "Show create index/reindex progress statistics",
		Filters:   map[int]*regexp.Regexp{},
	}
	// PgStatActivityUnit describes how to handle pg_stat_activity view
	PgStatActivityUnit = ContextUnit{
		Name:      ActivityView,
		Query:     PgStatActivityQueryDefault,
		DiffIntvl: NoDiff,
		Ncols:     14,
		OrderKey:  0,
		OrderDesc: true,
		ColsWidth: map[int]int{},
		Msg:       "Show activity statistics",
		Filters:   map[int]*regexp.Regexp{},
	}
	// PgSSTimingUnit describes how to handle pg_stat_statements view with timing stats
	PgSSTimingUnit = ContextUnit{
		Name:      StatementsTimingView,
		Query:     PgStatStatementsTimingQueryDefault,
		DiffIntvl: [2]int{6, 10},
		Ncols:     13,
		OrderKey:  0,
		OrderDesc: true,
		UniqueKey: 11,
		ColsWidth: map[int]int{},
		Msg:       "Show statements timings statistics",
		Filters:   map[int]*regexp.Regexp{},
	}
	// PgSSGeneralUnit describes how to handle pg_stat_statements view with general stats
	PgSSGeneralUnit = ContextUnit{
		Name:      StatementsGeneralView,
		Query:     PgStatStatementsGeneralQueryDefault,
		DiffIntvl: [2]int{4, 5},
		Ncols:     8,
		OrderKey:  0,
		OrderDesc: true,
		UniqueKey: 6,
		ColsWidth: map[int]int{},
		Msg:       "Show statements general statistics",
		Filters:   map[int]*regexp.Regexp{},
	}
	// PgSSIoUnit describes how to handle pg_stat_statements view with stats related to buffers IO
	PgSSIoUnit = ContextUnit{
		Name:      StatementsIOView,
		Query:     PgStatStatementsIoQueryDefault,
		DiffIntvl: [2]int{6, 10},
		Ncols:     13,
		OrderKey:  0,
		OrderDesc: true,
		UniqueKey: 11,
		ColsWidth: map[int]int{},
		Msg:       "Show statements IO statistics",
		Filters:   map[int]*regexp.Regexp{},
	}
	// PgSSTempUnit describes how to handle pg_stat_statements view with stats related to temp files IO
	PgSSTempUnit = ContextUnit{
		Name:      StatementsTempView,
		Query:     PgStatStatementsTempQueryDefault,
		DiffIntvl: [2]int{4, 6},
		Ncols:     9,
		OrderKey:  0,
		OrderDesc: true,
		UniqueKey: 7,
		ColsWidth: map[int]int{},
		Msg:       "Show statements temp files statistics",
		Filters:   map[int]*regexp.Regexp{},
	}
	// PgSSLocalUnit describes how to handle pg_stat_statements view with stats related to local buffers IO
	PgSSLocalUnit = ContextUnit{
		Name:      StatementsLocalView,
		Query:     PgStatStatementsLocalQueryDefault,
		DiffIntvl: [2]int{6, 10},
		Ncols:     13,
		OrderKey:  0,
		OrderDesc: true,
		UniqueKey: 11,
		ColsWidth: map[int]int{},
		Msg:       "Show statements temp tables statistics (local IO)",
		Filters:   map[int]*regexp.Regexp{},
	}
)

// AdjustQueries performs adjusting of queries accordingly to Postgres version
func (cl ContextList) AdjustQueries(pi PgInfo) {
	for c := range cl {
		switch c {
		case ActivityView:
			switch {
			case pi.PgVersionNum < 90600:
				cl[c].Query = PgStatActivityQuery95
				cl[c].Ncols = 12
			case pi.PgVersionNum < 100000:
				cl[c].Query = PgStatActivityQuery96
				cl[c].Ncols = 13
			}
		case ReplicationView:
			switch {
			case pi.PgVersionNum < 90500:
				// Use query for 9.6 but with no 'track_commit_timestamp' fileds
				cl[c].Query = PgStatReplicationQuery96
				cl[c].Ncols = 12
			case pi.PgVersionNum < 100000:
				// Check is 'track_commit_timestamp' enabled or not and use corresponding query for 9.6
				if pi.PgTrackCommitTs == "on" {
					cl[c].Query = PgStatReplicationQuery96Extended
					cl[c].Ncols = 14
				} else {
					cl[c].Query = PgStatReplicationQuery96
					cl[c].Ncols = 12
				}
			default:
				// Check is 'track_commit_timestamp' enabled or not and use corresponding query for 10 and above
				if pi.PgTrackCommitTs == "on" {
					cl[c].Query = PgStatReplicationQueryExtended
					cl[c].Ncols = 17
				} else {
					// use defaults assigned in context unit
				}
			}
		case DatabaseView:
			switch {
			// versions prior 12 don't have  checksum_failures column
			case pi.PgVersionNum < 120000:
				cl[c].Query = PgStatDatabaseQuery11
				cl[c].Ncols = 17
				cl[c].DiffIntvl = [2]int{1, 15}
			}
		case StatementsTimingView:
			switch {
			case pi.PgVersionNum < 130000:
				cl[c].Query = PgStatStatementsTimingQuery12
			}
		}
	}
}
