// Stuff related to PostgreSQL stats

package stat

import (
	"bytes"
	"database/sql"
	"fmt"
	"github.com/lesovsky/pgcenter/lib/utils"
	"github.com/pkg/errors"
	"sort"
	"strconv"
	"strings"
)

const (
	DatabaseView          = "pg_stat_database"
	ReplicationView       = "pg_stat_replication"
	TablesView            = "pg_stat_tables"  // simplified name for set of views: pg_stat_*_tables and pg_statio_*_tables
	IndexesView           = "pg_stat_indexes" // simplified name for set of views: pg_stat_*_indexes and pg_statio_*_indexes
	SizesView             = "pg_stat_sizes"   // fictional name, no such view really exists
	FunctionsView         = "pg_stat_user_functions"
	VacuumView            = "pg_stat_progress_vacuum"
	ActivityView          = "pg_stat_activity"
	StatementsView        = "pg_stat_statements"
	StatementsTimingView  = "pg_stat_statements_timing"  // fictional name, based on pg_stat_statements
	StatementsGeneralView = "pg_stat_statements_general" // fictional name, based on pg_stat_statements
	StatementsIOView      = "pg_stat_statements_io"      // fictional name, based on pg_stat_statements
	StatementsTempView    = "pg_stat_statements_temp"    // fictional name, based on pg_stat_statements
	StatementsLocalView   = "pg_stat_statements_local"   // fictional name, based on pg_stat_statements

	ConfMain     = "postgresql.conf"
	ConfHba      = "pg_hba.conf"
	ConfIdent    = "pg_ident.conf"
	ConfRecovery = "recovery.conf"

	GucMainConfFile = "config_file"
	GucHbaFile      = "hba_file"
	GucIdentFile    = "ident_file"
	GucRecoveryFile = "recovery.conf" // fake GUC, it isn't exist
	GucDataDir      = "data_directory"
)

const (
	PgProcSysTicksQuery = "SELECT pgcenter.get_sys_clk_ticks()"
)

var (
	// Error means that creating delta is failed
	ERR_DIFF_FAILED = errors.New("ERR_DIFF_CHANGED")

	// Special value means to don't diff PGresults
	NoDiff = [2]int{99, 99}
)

// Container for stats
type Pgstat struct {
	PgInfo
	PgActivityStat
	CurrPGresult PGresult
	PrevPGresult PGresult
	DiffPGresult PGresult
}

// Container for details about Postgres
type PgInfo struct {
	PgAlive         string /* is Postgres alive or not? */
	PgVersionNum    uint   /* Postgres version in format XXYYZZ */
	PgVersion       string /* Postgres version in format X.Y.Z */
	PgUptime        string /* Postgres uptime */
	PgRecovery      string /* is Postgres master or standby? */
	PgTrackCommitTs string /* track_commit_timestamp value */
	PgAVMaxWorkers  uint   /* autovacuum_max_workers value */
	PgMaxConns      uint   /* max_connections value */
	PgMaxPrepXacts  uint   /* max_prepared_transactions value */
}

// Container for Postgres' activity stats
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

// Container for basic Postgres stats collected from pg_stat_* views
type PGresult struct {
	Result    [][]sql.NullString /* values */
	Cols      []string           /* list of columns' names*/
	Ncols     int                /* numbers of columns in Result */
	Nrows     int                /* number of rows in Result */
	Colmaxlen map[string]int     /* lengths of the longest value for each column */
	Valid     bool               /* Used for result invalidations, on context switching for example */
	Err       error              /* Error returned by query, if any */
}

// Get Postgres connection status - is it alive or not?
func GetPgState(conn *sql.DB) string {
	err := utils.PQstatus(conn)
	if err != nil {
		return "failed"
	} else {
		return "ok"
	}
}

// Get Postgres uptime
func (s *Pgstat) Uptime(conn *sql.DB) {
	if err := conn.QueryRow(PgGetUptimeQuery).Scan(&s.PgUptime); err != nil {
		s.PgUptime = "--:--:--"
	}
}

// Get information about Postgres: version, something else?
func (s *Pgstat) ReadPgInfo(conn *sql.DB, isLocal bool) {
	conn.QueryRow(PgGetVersionQuery).Scan(&s.PgVersion, &s.PgVersionNum)
	conn.QueryRow(PgGetSingleSettingQuery, "track_commit_timestamp").Scan(&s.PgTrackCommitTs)
	conn.QueryRow(PgGetSingleSettingQuery, "max_connections").Scan(&s.PgMaxConns)
	conn.QueryRow(PgGetSingleSettingQuery, "autovacuum_max_workers").Scan(&s.PgAVMaxWorkers)
	conn.QueryRow(PgGetSingleSettingQuery, "max_connections").Scan(&s.PgMaxConns)
	conn.QueryRow(PgGetRecoveryStatusQuery).Scan(&s.PgRecovery)

	// In case of remote Postgres we should to know remote CLK_TCK
	if !isLocal {
		conn.QueryRow(PgProcSysTicksQuery).Scan(&SysTicks)
	}
}

// Reset activity stats during Postgres restarts
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

// Collects activity stats
func (s *Pgstat) GetPgstatActivity(conn *sql.DB, refresh uint, isLocal bool) {
	// First of all check Postgres status: is it dead or alive?
	// Remember the previous state of Postgres, if it's restored from 'failed' to 'ok', PgInfo have to be updated
	// because of there are might be changed version, important GUCs, etc.
	prevState := s.PgAlive
	s.PgAlive = GetPgState(conn)
	if prevState == "failed" && s.PgAlive == "ok" {
		s.ReadPgInfo(conn, isLocal)
	} else if s.PgAlive == "failed" {
		s.Reset()
		return // No reasons to continue if Postgres is down
	}

	s.Uptime(conn)

	conn.QueryRow(PgGetRecoveryStatusQuery).Scan(&s.PgRecovery)

	q_activity := PgActivityQueryDefault
	q_autovac := PgAutovacQueryDefault
	switch {
	case s.PgVersionNum < 90400:
		q_activity = PgActivityQueryBefore94
		q_autovac = PgAutovacQueryBefore94
	case s.PgVersionNum < 90600:
		q_activity = PgActivityQueryBefore96
	case s.PgVersionNum < 100000:
		q_activity = PgActivityQueryBefore10
	default:
		// use defaults
	}

	conn.QueryRow(q_activity).Scan(
		&s.ConnTotal, &s.ConnIdle, &s.ConnIdleXact,
		&s.ConnActive, &s.ConnWaiting, &s.ConnOthers,
		&s.ConnPrepared)

	conn.QueryRow(q_autovac).Scan(
		&s.AVWorkers, &s.AVAntiwrap, &s.AVManual, &s.AVMaxTime)

	conn.QueryRow(PgStatementsQuery).Scan(&s.StmtAvgTime, &s.CallsCurr)
	s.StmtPerSec = (s.CallsCurr - s.CallsPrev) / refresh
	s.CallsPrev = s.CallsCurr

	conn.QueryRow(PgActivityTimeQuery).Scan(
		&s.XactMaxTime, &s.PrepMaxTime)
}

// Read stat from pg_stat_* views, diff with previous stats snapshot and sort final resulting stats.
func (s *Pgstat) GetPgstatDiff(conn *sql.DB, query string, itv uint, interval [2]int, skey int, d bool, ukey int) error {
	// Read stat
	if err := s.GetPgstatSample(conn, query); err != nil {
		return err
	}

	// Make prev snapshot using current snap, at startup or at context switching
	if !s.PrevPGresult.Valid {
		s.PrevPGresult = s.CurrPGresult
	}

	// Diff previous and current stats snapshot
	if interval != NoDiff {
		if err := s.DiffPGresult.Diff(&s.PrevPGresult, &s.CurrPGresult, itv, interval, ukey); err != nil {
			return ERR_DIFF_FAILED
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

// Read stat from pg_stat_* views and create PGresult struct
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

// Parse a result of the query and create PGresult struct
func (r *PGresult) New(rs *sql.Rows) error {
	var container []sql.NullString
	var pointers []interface{}

	r.Cols, _ = rs.Columns()
	r.Ncols = len(r.Cols)

	for rs.Next() {
		pointers = make([]interface{}, r.Ncols)
		container = make([]sql.NullString, r.Ncols)

		for i := range pointers {
			pointers[i] = &container[i]
		}

		err := rs.Scan(pointers...)
		if err != nil {
			r.Valid = false
			return fmt.Errorf("Failed to scan row: %s", err)
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

// Take current and previous stats snapshots and make delta
func (d *PGresult) Diff(p *PGresult, c *PGresult, itv uint, interval [2]int, ukey int) error {
	var found bool = false

	d.Result = make([][]sql.NullString, c.Nrows)
	d.Cols = c.Cols
	d.Ncols = len(c.Cols)
	d.Nrows = c.Nrows
	d.Colmaxlen = c.Colmaxlen // use lengthes of current values

	// Take every row from 'current' snapshot and check its existing in 'previous' snapshot. If row exists in both snapshots
	// make diff between them. If target row is not found in 'previous' snapshot, no diff needed, hence append this row
	// as-is into 'result' snapshot.
	// Thus in the end, all rows that aren't exist in the 'current' snapshot, but exist in 'previous', will be skipped.
	for i, cv := range c.Result {
		// Allocate container for target row and reset 'found' flag
		d.Result[i] = make([]sql.NullString, c.Ncols)
		found = false

		for j, pv := range p.Result {
			if cv[ukey].String == pv[ukey].String {
				// Row exists in both snapshots
				found = true

				// Do diff
				for l := 0; l < c.Ncols; l++ {
					if l < interval[0] || l > interval[1] {
						d.Result[i][l].String = c.Result[i][l].String // don't diff, copy value as-is
					} else {
						if strings.Contains(c.Result[i][l].String, ".") {
							cv, err := strconv.ParseFloat(c.Result[i][l].String, 64)
							if err != nil {
								return fmt.Errorf("failed to convert to float [%d:%d]: %s", i, l, err)
							}
							pv, err := strconv.ParseFloat(p.Result[j][l].String, 64)
							if err != nil {
								return fmt.Errorf("failed to convert to float [%d:%d]: %s", j, l, err)
							}
							d.Result[i][l].String = strconv.FormatFloat((cv-pv)/float64(itv), 'f', 2, 64)
						} else {
							cv, err := strconv.ParseInt(c.Result[i][l].String, 10, 64)
							if err != nil {
								return fmt.Errorf("failed to convert to integer [%d:%d]: %s", i, l, err)
							}
							pv, err := strconv.ParseInt(p.Result[j][l].String, 10, 64)
							if err != nil {
								return fmt.Errorf("failed to convert to integer [%d:%d]: %s", j, l, err)
							}
							d.Result[i][l].String = strconv.FormatInt((cv-pv)/int64(itv), 10)
						}
					}
				}
				break // Go to searching next row from current snapshot
			}
		}

		// End of the searching in 'previous' snapshot, if we reached here it means row not found and it simply should be added as is.
		if found == false {
			for l := 0; l < c.Ncols; l++ {
				d.Result[i][l].String = c.Result[i][l].String // don't diff, copy value as-is
			}
		}
	}

	return nil
}

// Sort content using predetermined order key
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
			} else {
				return l < r /* asc order: 0 -> 10 */
			}
		})
	} else {
		// value is string
		sort.Slice(r.Result, func(i, j int) bool {
			if desc {
				return r.Result[i][key].String > r.Result[j][key].String /* desc order: 'z' -> 'a' */
			} else {
				return r.Result[i][key].String < r.Result[j][key].String /* asc order: 'a' -> 'z' */
			}
		})
	}
}

// DEPRECATED in favor SetAlignCustom() // TODO: clean out code
// Calculate column width used at result formatting
func (r *PGresult) SetAlign() {
	r.Colmaxlen = make(map[string]int)

	/* calculate base length of columns which is based on the length of the title */
	for _, n := range r.Cols {
		r.Colmaxlen[n] = len(n)
	}

	/* calculate max length of columns based on the longest value of the column */
	colnum := 0
	for _, colname := range r.Cols { // walk per-column   // don't use Collen here - it's unordered
		for rownum := 0; rownum < len(r.Result); rownum++ { // walk through rows
			/* m[row][column] */
			if len(r.Result[rownum][colnum].String) > r.Colmaxlen[colname] {
				r.Colmaxlen[colname] = len(r.Result[rownum][colnum].String)
			}
		}

		/* add 2 extra spaces to column's length */
		r.Colmaxlen[colname] = r.Colmaxlen[colname] + 2

		colnum++
	}
}

// DEPRECATED in favor SetAlignCustom()

// Calculate column width used at result formatting and truncate too long values
func (r *PGresult) SetAlignCustom(truncLimit int) {
	r.Colmaxlen = make(map[string]int)

	/* calculate max length of columns based on the longest value of the column */
	colnum := 0
	for _, colname := range r.Cols { // walk per-column   // don't use Collen here - it's unordered
		for rownum := 0; rownum < len(r.Result); rownum++ { // walk through rows
			//
			valuelen := len(r.Result[rownum][colnum].String)
			colnamelen := len(colname)
			switch {
			// if value is empty, e.g. NULL - set length based on colname length, but no longer that already set
			case valuelen == 0 && colnamelen >= r.Colmaxlen[colname]:
				r.Colmaxlen[colname] = colnamelen
			// for non-empty values, but for those whose length less than length of colnames, use length based on length of column name, bit no longer than already set
			case valuelen > 0 && valuelen <= colnamelen && valuelen >= r.Colmaxlen[colname]:
				r.Colmaxlen[colname] = colnamelen
			// for non-empty values, but for those whose length longer than length of colnames, use length based on length of value, bit no longer than already set
			case valuelen > 0 && valuelen > colnamelen && valuelen < truncLimit && valuelen >= r.Colmaxlen[colname]:
				r.Colmaxlen[colname] = valuelen
			// for very long values, truncate value and set length limited by truncLimit value,
			case valuelen >= truncLimit:
				r.Result[rownum][colnum].String = r.Result[rownum][colnum].String[:truncLimit]
				r.Colmaxlen[colname] = truncLimit
				//default:	// default case is used for debug purposes for catching cases that don't met upper conditions
				//	fmt.Printf("*** DEBUG %s -- %s***", colname, r.Result[rownum][colnum].String)
			}
		}

		/* add 2 extra spaces to column's length */
		r.Colmaxlen[colname] = r.Colmaxlen[colname] + 2

		colnum++
	}
}

// Print content of PGresult container to buffer
func (r *PGresult) Fprint(buf *bytes.Buffer) {
	/* print header */
	for _, name := range r.Cols {
		fmt.Fprintf(buf, "%-*s", r.Colmaxlen[name], name)
	}
	fmt.Fprintf(buf, "\n\n")

	/* print data to buffer */
	for colnum, rownum := 0, 0; rownum < r.Nrows; rownum, colnum = rownum+1, 0 {
		for _, colname := range r.Cols {
			/* m[row][column] */
			fmt.Fprintf(buf, "%-*s", r.Colmaxlen[colname], r.Result[rownum][colnum].String)
			colnum++
		}
		fmt.Fprintf(buf, "\n")
	}
}

// Print content of PGresult container to stdout
func (r *PGresult) Print() {
	/* print header */
	for _, name := range r.Cols {
		fmt.Printf("\033[%d;%dm%-*s \033[0m", 37, 1, r.Colmaxlen[name], name)
	}
	fmt.Printf("\n")

	/* print data to buffer */
	for colnum, rownum := 0, 0; rownum < r.Nrows; rownum, colnum = rownum+1, 0 {
		for _, colname := range r.Cols {
			/* m[row][column] */
			fmt.Printf("%-*s ", r.Colmaxlen[colname], r.Result[rownum][colnum].String)
			colnum++
		}
		fmt.Printf("\n")
	}
}
