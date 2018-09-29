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

// Single unit of stats context.
type ContextUnit struct {
	Name      string                 // Context name
	Query     string                 // Query used by default
	DiffIntvl [2]int                 // Columns interval for diff
	Ncols     int                    // Number of columns returned by query, used as a right border for OrderKey
	OrderKey  int                    // Number of column used for order
	OrderDesc bool                   // Order direction: descending (true) or ascending (false)
	UniqueKey int                    // Unique key that used on rows comparing when building diffs, by default it's zero which is OK in almost all contexts
	Msg       string                 // Show this text in Cmdline when switching to this unit
	Filters   map[int]*regexp.Regexp // Storage for filter patterns: key is the column index, value - regexp pattern
}

// List of used context units.
type ContextList map[string]*ContextUnit

var (
	PgStatDatabaseUnit = ContextUnit{
		Name:      DatabaseView,
		Query:     PgStatDatabaseQueryDefault,
		DiffIntvl: [2]int{1, 15},
		Ncols:     17,
		OrderKey:  0,
		OrderDesc: true,
		Msg:       "Show databases statistics",
		Filters:   map[int]*regexp.Regexp{},
	}

	PgStatReplicationUnit = ContextUnit{
		Name:      ReplicationView,
		Query:     PgStatReplicationQueryDefault,
		DiffIntvl: [2]int{6, 6},
		Ncols:     15,
		OrderKey:  0,
		OrderDesc: true,
		Msg:       "Show replication statistics",
		Filters:   map[int]*regexp.Regexp{},
	}

	PgStatTablesUnit = ContextUnit{
		Name:      TablesView,
		Query:     PgStatTablesQueryDefault,
		DiffIntvl: [2]int{1, 18},
		Ncols:     19,
		OrderKey:  0,
		OrderDesc: true,
		Msg:       "Show tables statistics",
		Filters:   map[int]*regexp.Regexp{},
	}

	PgStatIndexesUnit = ContextUnit{
		Name:      IndexesView,
		Query:     PgStatIndexesQueryDefault,
		DiffIntvl: [2]int{1, 5},
		Ncols:     6,
		OrderKey:  0,
		OrderDesc: true,
		Msg:       "Show indexes statistics",
		Filters:   map[int]*regexp.Regexp{},
	}

	PgTablesSizesUnit = ContextUnit{
		Name:      SizesView,
		Query:     PgTablesSizesQueryDefault,
		DiffIntvl: [2]int{4, 6},
		Ncols:     7,
		OrderKey:  0,
		OrderDesc: true,
		Msg:       "Show tables sizes statistics",
		Filters:   map[int]*regexp.Regexp{},
	}

	PgStatFunctionsUnit = ContextUnit{
		Name:      FunctionsView,
		Query:     PgStatFunctionsQueryDefault,
		DiffIntvl: [2]int{3, 3},
		Ncols:     8,
		OrderKey:  0,
		OrderDesc: true,
		Msg:       "Show functions statistics",
		Filters:   map[int]*regexp.Regexp{},
	}

	PgStatVacuumUnit = ContextUnit{
		Name:  VacuumView,
		Query: PgStatVacuumQueryDefault,
		//DiffIntvl: NoDiff,
		DiffIntvl: [2]int{9, 10},
		Ncols:     14,
		OrderKey:  0,
		OrderDesc: true,
		Msg:       "Show vacuum statistics",
		Filters:   map[int]*regexp.Regexp{},
	}

	PgStatActivityUnit = ContextUnit{
		Name:      ActivityView,
		Query:     PgStatActivityQueryDefault,
		DiffIntvl: NoDiff,
		Ncols:     14,
		OrderKey:  0,
		OrderDesc: true,
		Msg:       "Show activity statistics",
		Filters:   map[int]*regexp.Regexp{},
	}

	PgSSTimingUnit = ContextUnit{
		Name:      StatementsTimingView,
		Query:     PgStatStatementsTimingQueryDefault,
		DiffIntvl: [2]int{6, 10},
		Ncols:     13,
		OrderKey:  0,
		OrderDesc: true,
		UniqueKey: 11,
		Msg:       "Show statements timings statistics",
		Filters:   map[int]*regexp.Regexp{},
	}

	PgSSGeneralUnit = ContextUnit{
		Name:      StatementsGeneralView,
		Query:     PgStatStatementsGeneralQueryDefault,
		DiffIntvl: [2]int{4, 5},
		Ncols:     8,
		OrderKey:  0,
		OrderDesc: true,
		UniqueKey: 6,
		Msg:       "Show statements general statistics",
		Filters:   map[int]*regexp.Regexp{},
	}

	PgSSIoUnit = ContextUnit{
		Name:      StatementsIOView,
		Query:     PgStatStatementsIoQueryDefault,
		DiffIntvl: [2]int{6, 10},
		Ncols:     13,
		OrderKey:  0,
		OrderDesc: true,
		UniqueKey: 11,
		Msg:       "Show statements IO statistics",
		Filters:   map[int]*regexp.Regexp{},
	}

	PgSSTempUnit = ContextUnit{
		Name:      StatementsTempView,
		Query:     PgStatStatementsTempQueryDefault,
		DiffIntvl: [2]int{4, 6},
		Ncols:     9,
		OrderKey:  0,
		OrderDesc: true,
		UniqueKey: 7,
		Msg:       "Show statements temp files statistics",
		Filters:   map[int]*regexp.Regexp{},
	}

	PgSSLocalUnit = ContextUnit{
		Name:      StatementsLocalView,
		Query:     PgStatStatementsLocalQueryDefault,
		DiffIntvl: [2]int{6, 10},
		Ncols:     13,
		OrderKey:  0,
		OrderDesc: true,
		UniqueKey: 11,
		Msg:       "Show statements temp tables statistics (local IO)",
		Filters:   map[int]*regexp.Regexp{},
	}
)

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
		}
	}
}
