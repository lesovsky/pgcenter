package view

import (
	"github.com/lesovsky/pgcenter/internal/query"
	"regexp"
	"time"
)

// View describes how stats received from Postgres should be displayed.
type View struct {
	Name      string                 // View name
	QueryTmpl string                 // Query template used for making particular query.
	Query     string                 // Query based on template and runtime options.
	DiffIntvl [2]int                 // Columns interval for diff
	Cols      []string               // Columns names
	Ncols     int                    // Number of columns returned by query, used as a right border for OrderKey
	OrderKey  int                    // Index of column used for order
	OrderDesc bool                   // Order direction: descending (true) or ascending (false)
	UniqueKey int                    // index of column used as unique key when comparing rows during diffs, by default it's zero which is OK in almost all views
	ColsWidth map[int]int            // Width used for columns and control an aligning
	Aligned   bool                   // Flag shows aligning is calculated or not
	Msg       string                 // Show this text in Cmdline when switching to this view
	Filters   map[int]*regexp.Regexp // Filter patterns: key is the column index, value - regexp pattern
	Refresh   time.Duration          // Number of seconds between update view.
	ShowExtra int                    // Specifies extra stats should be enabled on the view.
}

// Views is a list of all used context units.
type Views map[string]View

// New returns set of predefined views.
func New() Views {
	return map[string]View{
		"activity": {
			Name:      "activity",
			QueryTmpl: query.PgStatActivityDefault,
			DiffIntvl: [2]int{0, 0},
			Ncols:     14,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show activity statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		"replication": {
			Name:      "replication",
			QueryTmpl: query.PgStatReplicationDefault,
			DiffIntvl: [2]int{6, 6},
			Ncols:     15,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show replication statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		"databases": {
			Name:      "databases",
			QueryTmpl: query.PgStatDatabaseDefault,
			DiffIntvl: [2]int{1, 16},
			Ncols:     18,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show databases statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		"tables": {
			Name:      "tables",
			QueryTmpl: query.PgStatTablesDefault,
			DiffIntvl: [2]int{1, 18},
			Ncols:     19,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show tables statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		"indexes": {
			Name:      "indexes",
			QueryTmpl: query.PgStatIndexesDefault,
			DiffIntvl: [2]int{1, 5},
			Ncols:     6,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show indexes statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		"sizes": {
			Name:      "sizes",
			QueryTmpl: query.PgTablesSizesDefault,
			DiffIntvl: [2]int{7, 11},
			Ncols:     12,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show tables sizes statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		"functions": {
			Name:      "functions",
			QueryTmpl: query.PgStatFunctionsDefault,
			DiffIntvl: [2]int{3, 3},
			Ncols:     8,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show functions statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		"statements_timings": {
			Name:      "statements_timings",
			QueryTmpl: query.PgStatStatementsTimingDefault,
			DiffIntvl: [2]int{6, 10},
			Ncols:     13,
			OrderKey:  0,
			OrderDesc: true,
			UniqueKey: 11,
			ColsWidth: map[int]int{},
			Msg:       "Show statements timings statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		"statements_general": {
			Name:      "statements_general",
			QueryTmpl: query.PgStatStatementsGeneralDefault,
			DiffIntvl: [2]int{4, 5},
			Ncols:     8,
			OrderKey:  0,
			OrderDesc: true,
			UniqueKey: 6,
			ColsWidth: map[int]int{},
			Msg:       "Show statements general statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		"statements_io": {
			Name:      "statements_io",
			QueryTmpl: query.PgStatStatementsIoDefault,
			DiffIntvl: [2]int{6, 10},
			Ncols:     13,
			OrderKey:  0,
			OrderDesc: true,
			UniqueKey: 11,
			ColsWidth: map[int]int{},
			Msg:       "Show statements IO statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		"statements_temp": {
			Name:      "statements_temp",
			QueryTmpl: query.PgStatStatementsTempDefault,
			DiffIntvl: [2]int{4, 6},
			Ncols:     9,
			OrderKey:  0,
			OrderDesc: true,
			UniqueKey: 7,
			ColsWidth: map[int]int{},
			Msg:       "Show statements temp files statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		"statements_local": {
			Name:      "statements_local",
			QueryTmpl: query.PgStatStatementsLocalDefault,
			DiffIntvl: [2]int{6, 10},
			Ncols:     13,
			OrderKey:  0,
			OrderDesc: true,
			UniqueKey: 11,
			ColsWidth: map[int]int{},
			Msg:       "Show statements temp tables statistics (local IO)",
			Filters:   map[int]*regexp.Regexp{},
		},
		"statements_wal": {
			Name:      "statements_wal",
			QueryTmpl: query.PgStatStatementsWalDefault,
			DiffIntvl: [2]int{3, 6},
			Ncols:     9,
			OrderKey:  0,
			OrderDesc: true,
			UniqueKey: 7,
			ColsWidth: map[int]int{},
			Msg:       "Show statements WAL statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		"progress_vacuum": {
			Name:      "progress_vacuum",
			QueryTmpl: query.PgStatProgressVacuumDefault,
			DiffIntvl: [2]int{10, 11},
			Ncols:     13,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show vacuum progress statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		"progress_cluster": {
			Name:      "progress_cluster",
			QueryTmpl: query.PgStatProgressClusterDefault,
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
			QueryTmpl: query.PgStatProgressCreateIndexDefault,
			DiffIntvl: [2]int{0, 0},
			Ncols:     14,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show create index/reindex progress statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		"progress_analyze": {
			Name:      "progress_analyze",
			QueryTmpl: query.PgStatProgressAnalyzeDefault,
			DiffIntvl: [2]int{0, 0},
			Ncols:     12,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show analyze progress statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
		"progress_basebackup": {
			Name:      "progress_basebackup",
			QueryTmpl: query.PgStatProgressBasebackupDefault,
			DiffIntvl: [2]int{9, 9},
			Ncols:     11,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show basebackup progress statistics",
			Filters:   map[int]*regexp.Regexp{},
		},
	}
}

// Configure performs adjusting of queries accordingly to Postgres version.
//   IN opts Options: struct with additional Postgres properties required for formatting necessary queries
//   IN gucTrackCommitTS string: value of track_commit_timestamp GUC (on/off)
func (v Views) Configure(opts query.Options) error {
	var track bool
	if opts.GucTrackCommitTS == "on" {
		track = true
	}
	for k, view := range v {
		switch k {
		case "activity":
			view.QueryTmpl, view.Ncols = query.SelectStatActivityQuery(opts.Version)
			v[k] = view
		case "replication":
			view.QueryTmpl, view.Ncols = query.SelectStatReplicationQuery(opts.Version, track)
			v[k] = view
		case "databases":
			view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatDatabaseQuery(opts.Version)
			v[k] = view
		case "statements_timings":
			view.QueryTmpl = query.SelectStatStatementsTimingQuery(opts.Version)
			v[k] = view
		}
	}

	// Build query texts based on templates.
	for k, view := range v {
		q, err := query.Format(view.QueryTmpl, opts)
		if err != nil {
			return err
		}
		view.Query = q
		v[k] = view
	}

	return nil
}
