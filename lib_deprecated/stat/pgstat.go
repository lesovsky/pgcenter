// Stuff related to PostgreSQL stats

package stat

import (
	"bytes"
	"database/sql"
	"fmt"
	"github.com/jackc/pgx/v4"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/lib_deprecated/utils"
	"github.com/pkg/errors"
	"sort"
	"strconv"
	"strings"
)

const (
	// DatabaseView is the name of view with databases' stats
	DatabaseView = "pg_stat_database"
	// ReplicationView is the name of view with replication stats
	ReplicationView = "pg_stat_replication"
	// TablesView is the name of view with tables' stats, it's a simplified name for the views: pg_stat_*_tables and pg_statio_*_tables
	TablesView = "pg_stat_tables"
	// IndexesView is the name of view with indexes' stats, it's a simplified name for the views: pg_stat_*_indexes and pg_statio_*_indexes
	IndexesView = "pg_stat_indexes"
	// SizesView is the name of pseudo-view with tables' sizes stats
	SizesView = "pg_stat_sizes"
	// FunctionsView is the name of view with functions stats
	FunctionsView = "pg_stat_user_functions"
	// ProgressView is the name of pseudo-view with progress stats
	ProgressView = "pg_stat_progress"
	// ProgressVacuumView is the name of pg_stat_progress_vacuum view
	ProgressVacuumView = "pg_stat_progress_vacuum"
	// ProgressClusterView is the name of pg_stat_progress_cluster view
	ProgressClusterView = "pg_stat_progress_cluster"
	// ProgressCreateIndexView is the name of pg_stat_progress_create_index view
	ProgressCreateIndexView = "pg_stat_progress_create_index"
	// ActivityView is the name of view with activity stats
	ActivityView = "pg_stat_activity"
	// StatementsView is the name of view with statements stats
	StatementsView = "pg_stat_statements"
	// StatementsTimingView is the name of pseudo-view with statements' timing stats
	StatementsTimingView = "pg_stat_statements_timing"
	// StatementsGeneralView is the name of pseudo-view with statements' general stats
	StatementsGeneralView = "pg_stat_statements_general"
	// StatementsIOView is the name of pseudo-view with statements' buffers IO stats
	StatementsIOView = "pg_stat_statements_io"
	// StatementsTempView is the name of pseudo-view with statements' temp files IO stats
	StatementsTempView = "pg_stat_statements_temp"
	// StatementsLocalView is the name of pseudo-view with statements' local buffers IO stats
	StatementsLocalView = "pg_stat_statements_local"
	// colsTruncMinLimit is the  minimal allowed value for values truncation
	colsTruncMinLimit = 1
	// colsWidthMin is the base width for columns (used by default, if column name too short)
	colsWidthMin = 8
	// GucMainConfFile is the name of GUC which stores Postgres config file location
	GucMainConfFile = "config_file"
	// GucHbaFile is the name of GUC which stores Postgres HBA file location
	GucHbaFile = "hba_file"
	// GucIdentFile is the name of GUC which stores ident file location
	GucIdentFile = "ident_file"
	// GucRecoveryFile is the name of pseudo-GUC which stores recovery settings location
	GucRecoveryFile = "recovery.conf"
	// GucDataDir is the name of GUC which stores data directory location
	GucDataDir = "data_directory"
	// PgProcSysTicksQuery queries system timer's frequency from Postgres instance
	PgProcSysTicksQuery = "SELECT pgcenter.get_sys_clk_ticks()"
)

var (
	// errDiffFailed error means that creating delta, using two stats snapshots, is failed
	errDiffFailed = errors.New("ERR_DIFF_CHANGED")

	// NoDiff is the special value means to don't diff PGresults snapshots
	NoDiff = [2]int{99, 99}
)

// Pgstat is the container for all collected Postgres stats
type Pgstat struct {
	PgInfo
	PgActivityStat
	CurrPGresult PGresult
	PrevPGresult PGresult
	DiffPGresult PGresult
}

// PgInfo is the container for details about Postgres
type PgInfo struct {
	PgAlive               string /* is Postgres alive or not? */
	PgVersionNum          uint   /* Postgres version in format XXYYZZ */
	PgVersionNum2         int    /* Postgres version in format XXYYZZ */
	PgVersion             string /* Postgres version in format X.Y.Z */
	PgUptime              string /* Postgres uptime */
	PgRecovery            string /* is Postgres master or standby? */
	PgTrackCommitTs       string /* track_commit_timestamp value */
	PgAVMaxWorkers        uint   /* autovacuum_max_workers value */
	PgMaxConns            uint   /* max_connections value */
	PgMaxPrepXacts        uint   /* max_prepared_transactions value */
	PgStatStatementsAvail bool   /* is pg_stat_statements available? */
	PgcenterSchemaAvail   bool   /* is pgcenter's stats schema available? */
}

// PgActivityStat is the container for Postgres' activity stats
type PgActivityStat struct {
	ConnTotal    uint    /* total number of connections */
	ConnIdle     uint    /* number of idle connections */
	ConnIdleXact uint    /* number of idle transactions */
	ConnActive   uint    /* number of active connections */
	ConnWaiting  uint    /* number of waiting backends */
	ConnOthers   uint    /* connections with misc. states */
	ConnPrepared uint    /* number of prepared transactions */
	AVWorkers    uint    /* number of regular autovacuum workers */
	AVAntiwrap   uint    /* number of antiwraparound vacuum workers */
	AVManual     uint    /* number of vacuums started by user */
	XactMaxTime  string  /* duration of the longest running xact or query */
	PrepMaxTime  string  /* duration of the longest running prepared xact */
	AVMaxTime    string  /* duration of the longest (auto)vacuum */
	StmtAvgTime  float32 /* average duration of queries */
	StmtPerSec   uint    /* current number of queries per second */
	CallsCurr    uint    /* total number of queries: current value */
	CallsPrev    uint    /* total number of queries: previous value */
}

// PGresult is the container for basic Postgres stats collected from pg_stat_* views
type PGresult struct {
	Result [][]sql.NullString /* values */
	Cols   []string           /* list of columns' names*/
	Ncols  int                /* numbers of columns in Result */
	Nrows  int                /* number of rows in Result */
	Valid  bool               /* Used for result invalidations, on context switching for example */
	Err    error              /* Error returned by query, if any */
}

// GetPgState gets Postgres connection status - is it alive or not?
func GetPgState(db *postgres.DB) string {
	err := utils.PQstatusNew(db)
	if err != nil {
		return "failed"
	}
	return "ok"
}

// Uptime method gets Postgres uptime
func (s *Pgstat) Uptime(db *postgres.DB) {
	if err := db.QueryRow(PgGetUptimeQuery).Scan(&s.PgUptime); err != nil {
		s.PgUptime = "--:--:--"
	}
}

// ReadPgInfo method gets some details about Postgres: version, GUCs, etc...
func (s *Pgstat) ReadPgInfo(conn *sql.DB, isLocal bool) {
	conn.QueryRow(PgGetVersionQuery).Scan(&s.PgVersion, &s.PgVersionNum)
	conn.QueryRow(PgGetSingleSettingQuery, "track_commit_timestamp").Scan(&s.PgTrackCommitTs)
	conn.QueryRow(PgGetSingleSettingQuery, "max_connections").Scan(&s.PgMaxConns)
	conn.QueryRow(PgGetSingleSettingQuery, "autovacuum_max_workers").Scan(&s.PgAVMaxWorkers)
	conn.QueryRow(PgGetRecoveryStatusQuery).Scan(&s.PgRecovery)

	// Is pg_stat_statement available?
	s.UpdatePgStatStatementsStatus(conn)

	// In case of remote Postgres we should to know remote CLK_TCK
	if !isLocal {
		s.IsPgcSchemaInstalled(conn)
		if s.PgcenterSchemaAvail {
			conn.QueryRow(PgProcSysTicksQuery).Scan(&SysTicks)
		}
	}
}

func (s *Pgstat) ReadPgInfoNew(db *postgres.DB) {
	// TODO: add errors handling
	db.QueryRow(PgGetVersionQuery).Scan(&s.PgVersion, &s.PgVersionNum)
	db.QueryRow("SELECT current_setting('track_commit_timestamp')").Scan(&s.PgTrackCommitTs)
	db.QueryRow("SELECT current_setting('max_connections')::int").Scan(&s.PgMaxConns)
	db.QueryRow("SELECT current_setting('autovacuum_max_workers')::int").Scan(&s.PgAVMaxWorkers)
	db.QueryRow(PgGetRecoveryStatusQuery).Scan(&s.PgRecovery)

	// Is pg_stat_statement available?
	s.UpdatePgStatStatementsStatusNew(db)

	// In case of remote Postgres we should to know remote CLK_TCK
	if !db.Local {
		s.IsPgcSchemaInstalledNew(db)
		if s.PgcenterSchemaAvail {
			db.QueryRow(PgProcSysTicksQuery).Scan(&SysTicks)
		}
	}
}

// UpdatePgStatStatementsStatus method refreshes info about pg_stat_statements
func (s *Pgstat) UpdatePgStatStatementsStatus(conn *sql.DB) {
	conn.QueryRow(PgCheckPGSSExists).Scan(&s.PgStatStatementsAvail)
}

func (s *Pgstat) UpdatePgStatStatementsStatusNew(db *postgres.DB) {
	// TODO: add error handling
	db.QueryRow(PgCheckPGSSExists).Scan(&s.PgStatStatementsAvail)
}

// IsPgcSchemaInstalled method checks pgcenter's stats schema existence
func (s *Pgstat) IsPgcSchemaInstalled(conn *sql.DB) {
	var avail bool
	if err := conn.QueryRow(PgCheckPgcenterSchemaQuery).Scan(&avail); err != nil {
		// in case of error, just tells the schema is not available
		s.PgcenterSchemaAvail = false
	}
	s.PgcenterSchemaAvail = avail
}

func (s *Pgstat) IsPgcSchemaInstalledNew(db *postgres.DB) {
	var avail bool
	if err := db.QueryRow(PgCheckPgcenterSchemaQuery).Scan(&avail); err != nil {
		// in case of error, just tells the schema is not available
		s.PgcenterSchemaAvail = false
	}
	s.PgcenterSchemaAvail = avail
}

// Reset method discards activity stats if Postgres restart detected
func (s *Pgstat) Reset() {
	s.PgActivityStat.ConnTotal = 0
	s.PgActivityStat.ConnIdle = 0
	s.PgActivityStat.ConnIdleXact = 0
	s.PgActivityStat.ConnActive = 0
	s.PgActivityStat.ConnWaiting = 0
	s.PgActivityStat.ConnOthers = 0
	s.PgActivityStat.ConnPrepared = 0
	s.PgActivityStat.AVWorkers = 0
	s.PgActivityStat.AVAntiwrap = 0
	s.PgActivityStat.AVManual = 0
	s.PgActivityStat.XactMaxTime = "--:--:--"
	s.PgActivityStat.PrepMaxTime = "--:--:--"
	s.PgActivityStat.AVMaxTime = "--:--:--"
	s.PgActivityStat.StmtAvgTime = 0.0
	s.PgActivityStat.StmtPerSec = 0
	s.PgActivityStat.CallsCurr = 0
	s.PgActivityStat.CallsPrev = 0
}

// GetPgstatActivity method collects Postgres' activity stats
func (s *Pgstat) GetPgstatActivity(db *postgres.DB, refresh uint) {
	// First of all check Postgres status: is it dead or alive?
	// Remember the previous state of Postgres, if it's restored from 'failed' to 'ok', PgInfo have to be updated
	// because of there are might be changed version, important GUCs, etc.
	prevState := s.PgAlive
	s.PgAlive = GetPgState(db)
	if prevState == "failed" && s.PgAlive == "ok" {
		s.ReadPgInfoNew(db)
	} else if s.PgAlive == "failed" {
		s.Reset()
		return // No reasons to continue if Postgres is down
	}

	s.Uptime(db)

	db.QueryRow(PgGetRecoveryStatusQuery).Scan(&s.PgRecovery)

	queryActivity := PgActivityQueryDefault
	queryAutovac := PgAutovacQueryDefault
	switch {
	case s.PgVersionNum < 90400:
		queryActivity = PgActivityQueryBefore94
		queryAutovac = PgAutovacQueryBefore94
	case s.PgVersionNum < 90600:
		queryActivity = PgActivityQueryBefore96
	case s.PgVersionNum < 100000:
		queryActivity = PgActivityQueryBefore10
	default:
		// use defaults
	}

	db.QueryRow(queryActivity).Scan(
		&s.ConnTotal, &s.ConnIdle, &s.ConnIdleXact,
		&s.ConnActive, &s.ConnWaiting, &s.ConnOthers,
		&s.ConnPrepared)

	db.QueryRow(queryAutovac).Scan(
		&s.AVWorkers, &s.AVAntiwrap, &s.AVManual, &s.AVMaxTime)

	// read pg_stat_statements only if it's available
	if s.PgStatStatementsAvail == true {
		db.QueryRow(PgStatementsQuery).Scan(&s.StmtAvgTime, &s.CallsCurr)
		s.StmtPerSec = (s.CallsCurr - s.CallsPrev) / refresh
		s.CallsPrev = s.CallsCurr
	}

	db.QueryRow(PgActivityTimeQuery).Scan(
		&s.XactMaxTime, &s.PrepMaxTime)
}

// GetPgstatDiff method reads stat from pg_stat_* views, does diff with previous stats snapshot and sort final resulting stats.
func (s *Pgstat) GetPgstatDiff(db *postgres.DB, query string, itv uint, interval [2]int, skey int, d bool, ukey int) error {
	// Read stat
	if err := s.GetPgstatSampleNew(db, query); err != nil {
		return err
	}

	// Make prev snapshot using current snap, at startup or at context switching
	if !s.PrevPGresult.Valid {
		s.PrevPGresult = s.CurrPGresult
	}

	// Diff previous and current stats snapshot
	if interval != NoDiff {
		if err := s.DiffPGresult.Diff(&s.PrevPGresult, &s.CurrPGresult, itv, interval, ukey); err != nil {
			return errDiffFailed
		}
	} else {
		s.DiffPGresult = s.CurrPGresult
	}

	// Sort
	s.DiffPGresult.Sort(skey, d)

	// Swap stats
	s.PrevPGresult = s.CurrPGresult

	return nil
}

// GetPgstatSample method reads stat from pg_stat_* views and creates PGresult struct
func (s *Pgstat) GetPgstatSample(conn *sql.DB, query string) error {
	s.CurrPGresult = PGresult{}
	rows, err := conn.Query(query)
	// Queries' errors aren't critical for us, remember and show them to the user. Return after the error, because
	// there is no reason to continue.
	if err != nil {
		s.CurrPGresult.Err = err
		return nil
	}

	if err := s.CurrPGresult.New(rows); err != nil {
		return err
	}

	return nil
}

func (s *Pgstat) GetPgstatSampleNew(db *postgres.DB, query string) error {
	s.CurrPGresult = PGresult{}
	rows, err := db.Query(query)
	// Queries' errors aren't critical for us, remember and show them to the user. Return after the error, because
	// there is no reason to continue.
	if err != nil {
		s.CurrPGresult.Err = err
		return nil
	}

	if err := s.CurrPGresult.New2(rows); err != nil {
		return err
	}

	return nil
}

// New method parses a result of the query and creates PGresult struct
func (r *PGresult) New(rs *sql.Rows) (err error) {
	var container []sql.NullString
	var pointers []interface{}

	if r.Cols, err = rs.Columns(); err != nil {
		r.Valid = false
		return fmt.Errorf("failed to read columns names: %s", err)
	}
	r.Ncols = len(r.Cols)

	for rs.Next() {
		pointers = make([]interface{}, r.Ncols)
		container = make([]sql.NullString, r.Ncols)

		for i := range pointers {
			pointers[i] = &container[i]
		}

		err = rs.Scan(pointers...)
		if err != nil {
			r.Valid = false
			return fmt.Errorf("failed to scan row: %s", err)
		}

		// Yes, it's better to avoid append() here, but we can't pre-allocate array of required size due to there is no
		// simple way (built-in in db driver/package) to know how many rows are returned by query.
		r.Result = append(r.Result, container)
		r.Nrows++
	}

	// parsing successful
	r.Valid = true
	return nil
}

func (r *PGresult) New2(rows pgx.Rows) (err error) {
	descs := rows.FieldDescriptions()
	cols := make([]string, len(descs))
	for i, desc := range descs {
		cols[i] = string(desc.Name)
	}

	r.Cols = cols
	r.Ncols = len(cols)

	var container []sql.NullString
	var pointers []interface{}

	for rows.Next() {
		pointers = make([]interface{}, r.Ncols)
		container = make([]sql.NullString, r.Ncols)

		for i := range pointers {
			pointers[i] = &container[i]
		}

		err = rows.Scan(pointers...)
		if err != nil {
			r.Valid = false
			return fmt.Errorf("failed to scan row: %s", err)
		}

		// Yes, it's better to avoid append() here, but we can't pre-allocate array of required size due to there is no
		// simple way (built-in in db driver/package) to know how many rows are returned by query.
		r.Result = append(r.Result, container)
		r.Nrows++
	}

	// parsing successful
	r.Valid = true
	return nil
}

// Diff method takes two snapshots, current and previous, and make delta
func (r *PGresult) Diff(prev *PGresult, curr *PGresult, itv uint, interval [2]int, ukey int) error {
	var found bool

	r.Result = make([][]sql.NullString, curr.Nrows)
	r.Cols = curr.Cols
	r.Ncols = len(curr.Cols)
	r.Nrows = curr.Nrows

	// Take every row from 'current' snapshot and check its existing in 'previous' snapshot. If row exists in both snapshots
	// make diff between them. If target row is not found in 'previous' snapshot, no diff needed, hence append this row
	// as-is into 'result' snapshot.
	// Thus in the end, all rows that aren't exist in the 'current' snapshot, but exist in 'previous', will be skipped.
	for i, cv := range curr.Result {
		// Allocate container for target row and reset 'found' flag
		r.Result[i] = make([]sql.NullString, curr.Ncols)
		found = false

		for j, pv := range prev.Result {
			if cv[ukey].String == pv[ukey].String {
				// Row exists in both snapshots
				found = true

				// Do diff
				for l := 0; l < curr.Ncols; l++ {
					if l < interval[0] || l > interval[1] {
						r.Result[i][l].String = curr.Result[i][l].String // don't diff, copy value as-is
					} else {
						// Values with dots or in scientific notation consider as floats and integer otherwise.
						if strings.Contains(curr.Result[i][l].String, ".") || strings.Contains(curr.Result[i][l].String, "e") {
							cv, err := strconv.ParseFloat(curr.Result[i][l].String, 64)
							if err != nil {
								return fmt.Errorf("failed to convert to float [%d:%d]: %s", i, l, err)
							}
							pv, err := strconv.ParseFloat(prev.Result[j][l].String, 64)
							if err != nil {
								return fmt.Errorf("failed to convert to float [%d:%d]: %s", j, l, err)
							}
							r.Result[i][l].String = strconv.FormatFloat((cv-pv)/float64(itv), 'f', 2, 64)
						} else {
							cv, err := strconv.ParseInt(curr.Result[i][l].String, 10, 64)
							if err != nil {
								return fmt.Errorf("failed to convert to integer [%d:%d]: %s", i, l, err)
							}
							pv, err := strconv.ParseInt(prev.Result[j][l].String, 10, 64)
							if err != nil {
								return fmt.Errorf("failed to convert to integer [%d:%d]: %s", j, l, err)
							}
							r.Result[i][l].String = strconv.FormatInt((cv-pv)/int64(itv), 10)
						}
					}
				}
				break // Go to searching next row from current snapshot
			}
		}

		// End of the searching in 'previous' snapshot, if we reached here it means row not found and it simply should be added as is.
		if found == false {
			for l := 0; l < curr.Ncols; l++ {
				r.Result[i][l].String = curr.Result[i][l].String // don't diff, copy value as-is
			}
		}
	}

	return nil
}

// Sort method does stats sorting using predetermined order key
func (r *PGresult) Sort(key int, desc bool) {
	if r.Nrows == 0 {
		return /* nothing to sort */
	}

	_, err := strconv.ParseFloat(r.Result[0][key].String, 64)
	if err == nil {
		// value is numeric
		sort.Slice(r.Result, func(i, j int) bool {
			l, _ := strconv.ParseFloat(r.Result[i][key].String, 64)
			r, _ := strconv.ParseFloat(r.Result[j][key].String, 64)
			if desc {
				return l > r /* desc order: 10 -> 0 */
			}
			return l < r /* asc order: 0 -> 10 */
		})
	} else {
		// value is string
		sort.Slice(r.Result, func(i, j int) bool {
			if desc {
				return r.Result[i][key].String > r.Result[j][key].String /* desc order: 'z' -> 'a' */
			}
			return r.Result[i][key].String < r.Result[j][key].String /* asc order: 'a' -> 'z' */
		})
	}
}

// SetAlign method aligns length of values depending of the columns width
func (r *PGresult) SetAlign(widthes map[int]int, truncLimit int, dynamic bool) (err error) {
	var lastColTruncLimit, lastColMaxWidth int
	lastColTruncLimit = utils.Max(truncLimit, colsTruncMinLimit)
	truncLimit = utils.Max(truncLimit, colsTruncMinLimit)

	// no rows in result, set width using length of a column name and return with error (because not aligned using result's values)
	if len(r.Result) == 0 {
		for colidx, colname := range r.Cols { // walk per-column
			widthes[colidx] = utils.Max(len(colname), colsTruncMinLimit)
		}
		return fmt.Errorf("RESULT_NO_ROWS")
	}

	/* calculate max length of columns based on the longest value of the column */
	var valuelen, colnamelen int
	for colidx, colname := range r.Cols { // walk per-column
		for rownum := 0; rownum < len(r.Result); rownum++ { // walk through rows
			valuelen = utils.Max(len(r.Result[rownum][colidx].String), colsTruncMinLimit)
			colnamelen = utils.Max(len(colname), colsWidthMin)

			switch {
			// if value is empty, e.g. NULL - set width based on colname length
			case aligningIsValueEmpty(valuelen, colnamelen, widthes[colidx]):
				widthes[colidx] = colnamelen
			// for non-empty values, but for those whose length less than length of colnames, use length based on length of column name, but no longer than already set
			case aligningIsLessThanColname(valuelen, colnamelen, widthes[colidx]):
				widthes[colidx] = colnamelen
			// for non-empty values, but for those whose length longer than length of colnames, use length based on length of value, but no longer than already set
			case aligningIsMoreThanColname(valuelen, colnamelen, widthes[colidx], truncLimit, colidx, r.Ncols):
				// dynamic aligning is used in 'report' when you can't adjust width on the fly
				// fixed aligning is used in 'top' because it's quite uncomfortable when width is changing constantly
				if dynamic {
					widthes[colidx] = valuelen
				} else {
					if valuelen > colnamelen*2 {
						widthes[colidx] = utils.Min(valuelen, 32)
					} else {
						widthes[colidx] = colnamelen
					}
				}
			// for last column set width using truncation limit
			case colidx == r.Ncols-1:
				// if truncation disabled, use width of the longest value, otherwise use the user-defined truncation limit
				if lastColTruncLimit == 0 {
					if lastColMaxWidth < valuelen {
						lastColMaxWidth = valuelen
					}
					widthes[colidx] = lastColMaxWidth
				} else {
					widthes[colidx] = truncLimit
				}
			// do nothing if length of value or column is less (or equal) than already specified width
			case aligningIsLengthLessOrEqualWidth(valuelen, colnamelen, widthes[colidx]):

			// for very long values, truncate value and set length limited by truncLimit value,
			case valuelen >= truncLimit:
				r.Result[rownum][colidx].String = r.Result[rownum][colidx].String[:truncLimit-1] + "~"
				widthes[colidx] = truncLimit
				//default:	// default case is used for debug purposes for catching cases that don't meet upper conditions
				//	fmt.Printf("*** DEBUG %s -- %s, %d:%d:%d ***", colname, r.Result[rownum][colnum].String, widthes[colidx], colnamelen, valuelen)
			}
		}
	}
	return nil
}

// aligningIsValueEmpty is the aligning helper: return true if value is empty, e.g. NULL - set width based on colname length
func aligningIsValueEmpty(vlen, cnlen, width int) bool {
	return vlen == 0 && cnlen >= width
}

// aligningIsLessThanColname is the aligning helper: returns true if passed non-empty values, but if its length less than length of colnames
func aligningIsLessThanColname(vlen, cnlen, width int) bool {
	return vlen > 0 && vlen <= cnlen && vlen >= width
}

// aligningIsMoreThanColname is the aligning helper: returns true if passed non-empty values, but for if its length longer than length of colnames
func aligningIsMoreThanColname(vlen, cnlen, width, trunclim, colidx, cols int) bool {
	return vlen > 0 && vlen > cnlen && vlen < trunclim && vlen >= width && colidx < cols-1
}

// aligningIsLengthLessOrEqualWidth is the aligning helper: returns true if length of value or column is less (or equal) than already specified width
func aligningIsLengthLessOrEqualWidth(vlen, cnlen, width int) bool {
	return vlen <= width && cnlen <= width
}

// Fprint method print content of PGresult container to buffer
func (r *PGresult) Fprint(buf *bytes.Buffer) {
	// do simple ad-hoc aligning for current PGresult, do align using the longest value in the column
	widthMap := map[int]int{}
	var valuelen int
	for colnum := range r.Cols {
		for rownum := 0; rownum < len(r.Result); rownum++ {
			valuelen = len(r.Result[rownum][colnum].String)
			if valuelen > widthMap[colnum] {
				widthMap[colnum] = valuelen
			}
		}
	}

	/* print header */
	for colidx, colname := range r.Cols {
		fmt.Fprintf(buf, "%-*s", widthMap[colidx]+2, colname)
	}
	fmt.Fprintf(buf, "\n\n")

	/* print data to buffer */
	for colnum, rownum := 0, 0; rownum < r.Nrows; rownum, colnum = rownum+1, 0 {
		for range r.Cols {
			/* m[row][column] */
			fmt.Fprintf(buf, "%-*s", widthMap[colnum]+2, r.Result[rownum][colnum].String)
			colnum++
		}
		fmt.Fprintf(buf, "\n")
	}
}

// Print method prints content of PGresult container to stdout
// DEPRECATION WARNING since v0.5.0: This function is used nowhere, seems it should be removed.
func (r *PGresult) Print() {
	/* print header */
	for _, name := range r.Cols {
		fmt.Printf("\033[%d;%dm%-*s \033[0m", 37, 1, len(name)+2, name)
	}
	fmt.Printf("\n")

	/* print data to buffer */
	for colnum, rownum := 0, 0; rownum < r.Nrows; rownum, colnum = rownum+1, 0 {
		for range r.Cols {
			/* m[row][column] */
			fmt.Printf("%-*s ", len(r.Result[rownum][colnum].String)+2, r.Result[rownum][colnum].String)
			colnum++
		}
		fmt.Printf("\n")
	}
}
