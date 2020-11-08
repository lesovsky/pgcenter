package stat

import (
	"bytes"
	"database/sql"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/math"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"sort"
	"strconv"
	"strings"
)

const (
	// colsTruncMinLimit is the  minimal allowed value for values truncation
	colsTruncMinLimit = 1
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

// Pgstat is the container for all collected Postgres stats
type Pgstat struct {
	Activity PostgresActivity
	Result   PGresult
}

func collectPostgresStat(db *postgres.DB, query string, prev Pgstat) (Pgstat, error) {
	activity, err := collectActivityStat(db, prev)
	if err != nil {
		return Pgstat{}, err
	}

	// Read stat
	res, err := NewPGresult(db, query)
	if err != nil {
		return Pgstat{}, err
	}

	return Pgstat{
		Activity: activity,
		Result:   res,
	}, nil
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
	AVUser       int     /* number of vacuums started by user */
	XactMaxTime  string  /* duration of the longest running xact or query */
	PrepMaxTime  string  /* duration of the longest running prepared xact */
	AVMaxTime    string  /* duration of the longest (auto)vacuum */
	StmtAvgTime  float32 /* average duration of queries */
	Uptime       string
	Recovery     string
	Calls        int /* замена для CallsCurr и CallsPrev */
	CallsRate    int /* замена для StmtPerSec */
}

func collectActivityStat(db *postgres.DB, prev Pgstat) (PostgresActivity, error) {
	s := PostgresActivity{}
	if state := getPgState(db); state != "ok" {
		return s, fmt.Errorf("postgres state is not ok")
	}

	if err := db.QueryRow(query.PgGetUptimeQuery).Scan(&s.Uptime); err != nil {
		s.Uptime = "--:--:--"
	}

	db.QueryRow(query.PgGetRecoveryStatusQuery).Scan(&s.Recovery)

	queryActivity := query.PgActivityQueryDefault
	queryAutovac := query.PgAutovacQueryDefault

	/* lessqqmorepewpew: доделать выбор запроса в зависимости от версии */
	//switch {
	//case s.PgVersionNum < 90400:
	//  queryActivity = PgActivityQueryBefore94
	//  queryAutovac = PgAutovacQueryBefore94
	//case s.PgVersionNum < 90600:
	//  queryActivity = PgActivityQueryBefore96
	//case s.PgVersionNum < 100000:
	//  queryActivity = PgActivityQueryBefore10
	//default:
	//  // use defaults
	//}

	db.QueryRow(queryActivity).Scan(
		&s.ConnTotal, &s.ConnIdle, &s.ConnIdleXact,
		&s.ConnActive, &s.ConnWaiting, &s.ConnOthers,
		&s.ConnPrepared)

	db.QueryRow(queryAutovac).Scan(&s.AVWorkers, &s.AVAntiwrap, &s.AVUser, &s.AVMaxTime)

	// read pg_stat_statements only if it's available
	//if s.PgStatStatementsAvail == true {
	/* lessqqmorepewpew: пока временно предполагаем что pg_stat_statements установлена в базе и наш интервал всегда 1 секунда */
	if true {
		db.QueryRow(query.PgStatementsQuery).Scan(&s.StmtAvgTime, &s.Calls)
		s.CallsRate = (s.Calls - prev.Activity.Calls) / 1
	}

	db.QueryRow(query.PgActivityTimeQuery).Scan(&s.XactMaxTime, &s.PrepMaxTime)

	return s, nil
}

// PostgresProperties is the container for details about Postgres
type PostgresProperties struct {
	State                   string  // state of Postgres - up or down
	VersionNum              int     // Numeric representation of Postgres version, e.g. XXYYZZ
	Version                 string  // String representation of Postgres version, e.g. X.Y.Z
	StartTime               float64 // Postgres start time
	Recovery                string  // Recovery state
	GucTrackCommitTimestamp string  // value of track_commit_timestamp GUC
	GucAVMaxWorkers         int     // value of autovacuum_max_workers GUC
	GucMaxConnections       int     // value of max_connections GUC
	GucMaxPrepXacts         int     // value of max_prepared_transactions GUC
	ExtPGSSAvail            bool    // is 'pg_stat_statements' extension installed?
	SchemaPgcenterAvail     bool    // is 'pgcenter' schema installed?
	SysTicks                float64 // ad-hoc implementation of GET_CLK for cases when Postgres is remote
}

func ReadPostgresProperties(db *postgres.DB) (PostgresProperties, error) {
	// TODO: add errors handling
	props := PostgresProperties{}
	db.QueryRow(query.PgGetVersionQuery).Scan(&props.Version, &props.VersionNum)
	db.QueryRow("SELECT current_setting('track_commit_timestamp')").Scan(&props.GucTrackCommitTimestamp)
	db.QueryRow("SELECT current_setting('max_connections')::int").Scan(&props.GucMaxConnections)
	db.QueryRow("SELECT current_setting('autovacuum_max_workers')::int").Scan(&props.GucAVMaxWorkers)
	db.QueryRow(query.PgGetRecoveryStatusQuery).Scan(&props.Recovery)
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

// PGresult is the container for basic Postgres stats collected from pg_stat_* views
type PGresult struct {
	Values [][]sql.NullString /* values */
	Cols   []string           /* list of columns' names */
	Ncols  int                /* numbers of columns in Result */
	Nrows  int                /* number of rows in Result */
	Valid  bool               /* Used for result invalidations, on context switching for example */
	Err    error              /* Error returned by query, if any */
}

func NewPGresult(db *postgres.DB, query string) (PGresult, error) {
	rows, err := db.Query(query)
	if err != nil {
		return PGresult{}, err
	}

	// Generic variables describe properties of query result.
	var (
		descs = rows.FieldDescriptions()
		ncols = len(descs)
		nrows int
	)

	// Storage used for data extracted from rows.
	// Scan operation supports only slice of interfaces, 'pointers' slice is the intermediate store where all values written.
	// Next values from 'pointers' associated with type-strict slice - 'values'. When Scan is writing to the 'pointers' it
	// also writing to the 'values' under the hood. When all pointers/values have been scanned, put them into 'rowsStore'.
	// Finally we get queryResult iterable store with data and information about stored rows, columns and columns names.
	var rowsStore = make([][]sql.NullString, 0, 10)

	for rows.Next() {
		pointers := make([]interface{}, ncols)
		values := make([]sql.NullString, ncols)

		for i := range pointers {
			pointers[i] = &values[i]
		}

		err = rows.Scan(pointers...)
		if err != nil {
			//log.Warnf("skip collecting stats: %s", err) // TODO: add error notification
			continue
		}
		rowsStore = append(rowsStore, values)
		nrows++
	}

	rows.Close()

	// Convert pgproto3.FieldDescription into string.
	colnames := make([]string, ncols)
	for i, d := range descs {
		colnames[i] = string(d.Name)
	}

	return PGresult{
		Nrows:  nrows,
		Ncols:  ncols,
		Cols:   colnames,
		Values: rowsStore,
		Valid:  true,
	}, nil
}

// calculateDelta produces differential PGresult based on current and previous snapshots
func calculateDelta(curr, prev PGresult, itv uint, interval [2]int, skey int, d bool, ukey int) (PGresult, error) {
	// Make prev snapshot using current snap, at startup or at context switching
	if !prev.Valid {
		return curr, nil
	}

	var delta PGresult
	var err error

	// Diff previous and current stats snapshot
	if interval != [2]int{0, 0} {
		delta, err = diff(curr, prev, itv, interval, ukey)
		if err != nil {
			return PGresult{}, fmt.Errorf("ERR_DIFF_CHANGED: %s", err)
		}
	} else {
		delta = curr
	}

	delta.sort(skey, d)

	return delta, nil
}

func diff(curr PGresult, prev PGresult, itv uint, interval [2]int, ukey int) (PGresult, error) {
	var diff PGresult
	var found bool

	diff.Values = make([][]sql.NullString, curr.Nrows)
	diff.Cols = curr.Cols
	diff.Ncols = len(curr.Cols)
	diff.Nrows = curr.Nrows

	// Take every row from 'current' snapshot and check its existing in 'previous' snapshot. If row exists in both snapshots
	// make diff between them. If target row is not found in 'previous' snapshot, no diff needed, hence append this row
	// as-is into 'result' snapshot.
	// Thus in the end, all rows that aren't exist in the 'current' snapshot, but exist in 'previous', will be skipped.
	for i, cv := range curr.Values {
		// Allocate container for target row and reset 'found' flag
		diff.Values[i] = make([]sql.NullString, curr.Ncols)
		found = false

		for j, pv := range prev.Values {
			if cv[ukey].String == pv[ukey].String {
				// Row exists in both snapshots
				found = true

				// Do diff
				for l := 0; l < curr.Ncols; l++ {
					if l < interval[0] || l > interval[1] {
						diff.Values[i][l].String = curr.Values[i][l].String // don't diff, copy value as-is
					} else {
						// Values with dots or in scientific notation consider as floats and integer otherwise.
						if strings.Contains(curr.Values[i][l].String, ".") || strings.Contains(curr.Values[i][l].String, "e") {
							cv, err := strconv.ParseFloat(curr.Values[i][l].String, 64)
							if err != nil {
								return diff, fmt.Errorf("failed to convert curr to float [%d:%d]: %s", i, l, err)
							}
							pv, err := strconv.ParseFloat(prev.Values[j][l].String, 64)
							if err != nil {
								return diff, fmt.Errorf("failed to convert prev to float [%d:%d]: %s", j, l, err)
							}
							diff.Values[i][l].String = strconv.FormatFloat((cv-pv)/float64(itv), 'f', 2, 64)
						} else {
							cv, err := strconv.ParseInt(curr.Values[i][l].String, 10, 64)
							if err != nil {
								return diff, fmt.Errorf("failed to convert curr to integer [%d:%d]: %s", i, l, err)
							}
							pv, err := strconv.ParseInt(prev.Values[j][l].String, 10, 64)
							if err != nil {
								return diff, fmt.Errorf("failed to convert prev to integer [%d:%d]: %s", j, l, err)
							}
							diff.Values[i][l].String = strconv.FormatInt((cv-pv)/int64(itv), 10)
						}
					}
				}
				break // Go to searching next row from current snapshot
			}
		}

		// End of the searching in 'previous' snapshot, if we reached here it means row not found and it simply should be added as is.
		if found == false {
			for l := 0; l < curr.Ncols; l++ {
				diff.Values[i][l].String = curr.Values[i][l].String // don't diff, copy value as-is
			}
		}
	}

	return diff, nil
}

// Sort method does stats sorting using predetermined order key
func (r *PGresult) sort(key int, desc bool) {
	if r.Nrows == 0 {
		return /* nothing to sort */
	}

	_, err := strconv.ParseFloat(r.Values[0][key].String, 64)
	if err == nil {
		// value is numeric
		sort.Slice(r.Values, func(i, j int) bool {
			l, _ := strconv.ParseFloat(r.Values[i][key].String, 64)
			r, _ := strconv.ParseFloat(r.Values[j][key].String, 64)
			if desc {
				return l > r /* desc order: 10 -> 0 */
			}
			return l < r /* asc order: 0 -> 10 */
		})
	} else {
		// value is string
		sort.Slice(r.Values, func(i, j int) bool {
			if desc {
				return r.Values[i][key].String > r.Values[j][key].String /* desc order: 'z' -> 'a' */
			}
			return r.Values[i][key].String < r.Values[j][key].String /* asc order: 'a' -> 'z' */
		})
	}
}

// SetAlign method aligns length of values depending of the columns width
// TODO: выравнивание не относится к статистике, а к её внешнему виду при выводе этой статистики, по идее оно должно
//   уехать куда-то из этого места. Но само выравнивание используется в нескольких под-командах, поэтому оно не может быть
//   частью какой-то отдельной подкоманды. В общем есть над чем подумать.
func (r *PGresult) SetAlign(truncLimit int, dynamic bool) (map[int]int, error) {
	var lastColTruncLimit, lastColMaxWidth int
	lastColTruncLimit = math.Max(truncLimit, colsTruncMinLimit)
	truncLimit = math.Max(truncLimit, colsTruncMinLimit)
	widthes := make(map[int]int)

	// no rows in result, set width using length of a column name and return with error (because not aligned using result's values)
	if len(r.Values) == 0 {
		for colidx, colname := range r.Cols { // walk per-column
			widthes[colidx] = math.Max(len(colname), colsTruncMinLimit)
		}
		return widthes, nil
	}

	/* calculate max length of columns based on the longest value of the column */
	var valuelen, colnamelen int
	for colidx, colname := range r.Cols { // walk per-column
		for rownum := 0; rownum < len(r.Values); rownum++ { // walk through rows
			valuelen = math.Max(len(r.Values[rownum][colidx].String), colsTruncMinLimit)
			colnamelen = math.Max(len(colname), 8) // eight is a minimal colname length, if column name too short.

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
						widthes[colidx] = math.Min(valuelen, 32)
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
				r.Values[rownum][colidx].String = r.Values[rownum][colidx].String[:truncLimit-1] + "~"
				widthes[colidx] = truncLimit
				//default:	// default case is used for debug purposes for catching cases that don't meet upper conditions
				//	fmt.Printf("*** DEBUG %s -- %s, %d:%d:%d ***", colname, r.Result[rownum][colnum].String, widthes[colidx], colnamelen, valuelen)
			}
		}
	}
	return widthes, nil
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
		for rownum := 0; rownum < len(r.Values); rownum++ {
			valuelen = len(r.Values[rownum][colnum].String)
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
			fmt.Fprintf(buf, "%-*s", widthMap[colnum]+2, r.Values[rownum][colnum].String)
			colnum++
		}
		fmt.Fprintf(buf, "\n")
	}
}

/* routines */

// getPgState gets Postgres connection status - is it alive or not?
func getPgState(db *postgres.DB) string {
	err := db.PQstatus()
	if err != nil {
		return "failed"
	}
	return "ok"
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
	err := db.QueryRow("SELECT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)", name).Scan(&exists)
	if err != nil {
		fmt.Println("failed to check schema in information_schema: ", err)
		return false
	}

	// Return true if installed, and false if not.
	return exists
}
