// Stuff related to PostgreSQL stats

package stat

import (
	"bytes"
	"database/sql"
	"fmt"
	"github.com/jackc/pgx/v4"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/lib/utils"
	"github.com/pkg/errors"
	"sort"
	"strconv"
	"strings"
)

const (
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
	Properties PostgresProperties
	Activity   PostgresActivity
	Curr       PGresult
	Prev       PGresult
	Diff       PGresult
}

// Activity describes Postgres' current activity stats
type PostgresActivity struct {
	ConnTotal    int     /* total number of connections */
	ConnIdle     int     /* number of idle connections */
	ConnIdleXact int     /* number of idle transactions */
	ConnActive   int     /* number of active connections */
	ConnWaiting  int     /* number of waiting backends */
	ConnOthers   int     /* connections with misc. states */
	ConnPrepared int     /* number of prepared transactions */
	AVWorkers    int     /* number of regular autovacuum workers */
	AVAntiwrap   int     /* number of antiwraparound vacuum workers */
	AVManual     int     /* number of vacuums started by user */
	XactMaxTime  string  /* duration of the longest running xact or query */
	PrepMaxTime  string  /* duration of the longest running prepared xact */
	AVMaxTime    string  /* duration of the longest (auto)vacuum */
	StmtAvgTime  float32 /* average duration of queries */
	/* lessqqmorepewpew: new fields added doing refactoring */
	Uptime    string
	Recovery  string
	Calls     int /* замена для CallsCurr и CallsPrev */
	CallsRate int /* замена для StmtPerSec */
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

// getPgState gets Postgres connection status - is it alive or not?
func getPgState(db *postgres.DB) string {
	err := utils.PQstatusNew(db)
	if err != nil {
		return "failed"
	}
	return "ok"
}

//
func readPostgresProperties(db *postgres.DB) (PostgresProperties, error) {
	// TODO: add errors handling
	props := PostgresProperties{}
	db.QueryRow(PgGetVersionQuery).Scan(&props.Version, &props.VersionNum)
	db.QueryRow("SELECT current_setting('track_commit_timestamp')").Scan(&props.GucTrackCommitTimestamp)
	db.QueryRow("SELECT current_setting('max_connections')::int").Scan(&props.GucMaxConnections)
	db.QueryRow("SELECT current_setting('autovacuum_max_workers')::int").Scan(&props.GucAVMaxWorkers)
	db.QueryRow(PgGetRecoveryStatusQuery).Scan(&props.Recovery)
	db.QueryRow("select extract(epoch from pg_postmaster_start_time())").Scan(&props.StartTime)

	// Is pg_stat_statement available?
	props.ExtPGSSAvail = isExtensionAvailable(db, "pg_stat_statements")

	// In case of remote Postgres we should to know remote CLK_TCK
	if !db.Local {
		if isSchemaAvailable(db, "pgcenter") {
			props.SchemaPgcenterAvail = true
			db.QueryRow(PgProcSysTicksQuery).Scan(&props.SysTicks)
		}
	}

	return props, nil
}

func isExtensionAvailable(db *postgres.DB, name string) bool {
	var exists bool
	err := db.QueryRow("SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = $1)", name).Scan(&exists)
	if err != nil {
		fmt.Println("failed to check extensions in pg_extension: ", err)
		return false
	}

	// Return true if installed, and false if not.
	return exists
}

//
func isSchemaAvailable(db *postgres.DB, name string) bool {
	var exists bool
	err := db.QueryRow("SELECT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = $1))", name).Scan(&exists)
	if err != nil {
		fmt.Println("failed to check schema in information_schema: ", err)
		return false
	}

	// Return true if installed, and false if not.
	return exists
}

// GetPgstatDiff method reads stat from pg_stat_* views, does diff with previous stats snapshot and sort final resulting stats.
func (s *Pgstat) GetPgstatDiff(db *postgres.DB, query string, itv uint, interval [2]int, skey int, d bool, ukey int) error {
	// Read stat
	if err := s.GetPgstatSample(db, query); err != nil {
		return err
	}

	// Make prev snapshot using current snap, at startup or at context switching
	if !s.Prev.Valid {
		s.Prev = s.Curr
	}

	// Diff previous and current stats snapshot
	if interval != NoDiff {
		if err := s.Diff.Diff(&s.Prev, &s.Curr, itv, interval, ukey); err != nil {
			return errDiffFailed
		}
	} else {
		s.Diff = s.Curr
	}

	// Sort
	s.Diff.Sort(skey, d)

	// Swap stats
	s.Prev = s.Curr

	return nil
}

func (s *Pgstat) GetPgstatSample(db *postgres.DB, query string) error {
	s.Curr = PGresult{}
	rows, err := db.Query(query)
	// Queries' errors aren't critical for us, remember and show them to the user. Return after the error, because
	// there is no reason to continue.
	if err != nil {
		s.Curr.Err = err
		return nil
	}

	if err := s.Curr.New(rows); err != nil {
		return err
	}

	return nil
}

func (r *PGresult) New(rows pgx.Rows) (err error) {
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
