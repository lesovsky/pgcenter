package stat

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Pgstat describes collected Postgres stats.
type Pgstat struct {
	Activity Activity
	Result   PGresult
}

// collectPostgresStat collect Postgres activity stats and stats returned by passed query.
func collectPostgresStat(db *postgres.DB, query string) (PGresult, error) {
	res, err := NewPGresultQuery(db, query)
	if err != nil {
		return PGresult{}, err
	}

	return res, nil
}

// Activity describes Postgres' current activity stats.
type Activity struct {
	State        string  // state of Postgres - up or down
	ConnTotal    int     // total number of connections
	ConnIdle     int     // number of idle connections
	ConnIdleXact int     // number of idle transactions
	ConnActive   int     // number of active connections
	ConnWaiting  int     // number of waiting backends
	ConnOthers   int     // connections with misc. states
	ConnPrepared int     // number of prepared transactions
	AVWorkers    int     // number of regular autovacuum workers
	AVAntiwrap   int     // number of antiwraparound vacuum workers
	AVUser       int     // number of vacuums started by user
	XactMaxTime  string  // duration of the longest running xact or query
	PrepMaxTime  string  // duration of the longest running prepared xact
	AVMaxTime    string  // duration of the longest (auto)vacuum
	StmtAvgTime  float32 // average duration of queries
	Uptime       string  // Postgres uptime (since start)
	Recovery     string  // Postgres recovery state
	Calls        int     // Number of calls
	CallsRate    int     // Number of calls per refresh interval
}

// collectActivityStat collects Postgres runtime activity about connected clients and workload.
func collectActivityStat(db *postgres.DB, version int, pgssSchema string, itv int, prevCalls int) (Activity, error) {
	var s Activity

	if err := db.QueryRow(query.GetUptime).Scan(&s.Uptime); err != nil {
		s.Uptime = "--:--:--"
	}

	if err := db.QueryRow(query.GetRecoveryStatus).Scan(&s.Recovery); err != nil {
		return s, err
	}

	// Depending on Postgres version select proper queries.
	queryActivity := query.SelectActivityActivityQuery(version)
	queryAutovacuum := query.SelectActivityAutovacuumQuery(version)

	err := db.QueryRow(queryActivity).Scan(
		&s.ConnTotal, &s.ConnIdle, &s.ConnIdleXact, &s.ConnActive, &s.ConnWaiting, &s.ConnOthers, &s.ConnPrepared)
	if err != nil {
		return s, err
	}

	err = db.QueryRow(queryAutovacuum).Scan(&s.AVWorkers, &s.AVAntiwrap, &s.AVUser, &s.AVMaxTime)
	if err != nil {
		return s, err
	}

	// read pg_stat_statements only if it's available
	if pgssSchema != "" {
		tmpl := query.SelectActivityStatementsQuery(version)
		q, err := query.Format(tmpl, query.Options{PGSSSchema: pgssSchema})
		if err != nil {
			return s, err
		}

		err = db.QueryRow(q).Scan(&s.StmtAvgTime, &s.Calls)
		if err != nil {
			return s, err
		}
		s.CallsRate = (s.Calls - prevCalls) / itv
	}

	err = db.QueryRow(query.SelectActivityTimes).Scan(&s.XactMaxTime, &s.PrepMaxTime)
	if err != nil {
		return s, err
	}

	s.State = "ok"
	return s, nil
}

// PostgresProperties is the container for details about Postgres
type PostgresProperties struct {
	VersionNum              int     // Numeric representation of Postgres version, e.g. XXYYZZ
	Version                 string  // String representation of Postgres version, e.g. X.Y.Z
	StartTime               float64 // Postgres start time
	Recovery                string  // Recovery state
	GucTrackCommitTimestamp string  // value of track_commit_timestamp GUC
	GucAVMaxWorkers         int     // value of autovacuum_max_workers GUC
	GucMaxConnections       int     // value of max_connections GUC
	GucMaxPrepXacts         int     // value of max_prepared_transactions GUC
	GucSharedPreLibraries   string  // value of shared_preload_libraries GUC
	ExtPGSSSchema           string  // Schema where 'pg_stat_statements' extension installed (empty if not installed)
	SchemaPgcenterAvail     bool    // is 'pgcenter' schema installed?
	SysTicks                float64 // ad-hoc implementation of GET_CLK for cases when Postgres is remote
}

// GetPostgresProperties queries necessary properties from Postgres about it.
func GetPostgresProperties(db *postgres.DB) (PostgresProperties, error) {
	props := PostgresProperties{}
	err := db.QueryRow(query.SelectCommonProperties).Scan(
		&props.Version,
		&props.VersionNum,
		&props.GucTrackCommitTimestamp,
		&props.GucMaxConnections,
		&props.GucAVMaxWorkers,
		&props.GucSharedPreLibraries,
		&props.Recovery,
		&props.StartTime,
	)
	if err != nil {
		return PostgresProperties{}, err
	}

	// Is pg_stat_statement available?
	if strings.Contains(props.GucSharedPreLibraries, "pg_stat_statements") {
		props.ExtPGSSSchema = extensionSchema(db, "pg_stat_statements")
	}

	// In case of remote Postgres we should to know remote CLK_TCK
	if !db.Local {
		if isSchemaExists(db, "pgcenter") {
			props.SchemaPgcenterAvail = true
			err := db.QueryRow(query.SelectRemoteProcSysTicks).Scan(&props.SysTicks)
			if err != nil {
				return PostgresProperties{}, err
			}
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
}

// NewPGresultQuery creates PGresult using passed database connection and query.
func NewPGresultQuery(db *postgres.DB, query string) (PGresult, error) {
	if query == "" {
		return PGresult{}, fmt.Errorf("no query defined")
	}

	rows, err := db.Query(query)
	if err != nil {
		return PGresult{}, err
	}

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
			//log.Warnf("skip collecting stats: %s", err) // TODO: add error handling and notification
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

	res := PGresult{
		Nrows:  nrows,
		Ncols:  ncols,
		Cols:   colnames,
		Values: rowsStore,
		Valid:  true,
	}

	err = res.validate()
	if err != nil {
		return PGresult{}, err
	}

	return res, nil
}

// NewPGresultFile creates PGresult using reader interface.
func NewPGresultFile(r io.Reader, bufsz int64) (PGresult, error) {
	data := make([]byte, bufsz)

	if _, err := io.ReadFull(r, data); err != nil {
		return PGresult{}, err
	}

	// initialize an empty struct and unmarshal data from the buffer
	res := PGresult{}
	err := json.Unmarshal(data, &res)
	if err != nil {
		return PGresult{}, err
	}

	err = res.validate()
	if err != nil {
		return PGresult{}, err
	}

	return res, nil
}

// validate validates content of PGresult
func (r *PGresult) validate() error {
	// Check that number or values in rows are equal to number of columns names.
	for _, row := range r.Values {
		if len(row) != len(r.Cols) {
			return fmt.Errorf("invalid number of values in row")
		}
	}

	// Check number of rows is the same as declared
	if r.Nrows != len(r.Values) {
		return fmt.Errorf("invalid number of rows and values")
	}

	return nil
}

// Compare is public wrapper over calculateDelta.
func Compare(curr, prev PGresult, itv int, interval [2]int, skey int, desc bool, ukey int) (PGresult, error) {
	return calculateDelta(curr, prev, itv, interval, skey, desc, ukey)
}

// calculateDelta compares two PGresult structs and returns ordered delta PGresult.
func calculateDelta(curr, prev PGresult, itv int, interval [2]int, skey int, desc bool, ukey int) (PGresult, error) {
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
			return PGresult{}, fmt.Errorf("diff failed: %s", err)
		}
	} else {
		delta = curr
	}

	delta.sort(skey, desc)

	return delta, nil
}

// diff compares two PGresult values and produces new differential PGresult.
func diff(curr PGresult, prev PGresult, itv int, interval [2]int, ukey int) (PGresult, error) {
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
			if cv[ukey].String != pv[ukey].String {
				// Row is not exist in previous snapshot
				continue
			}

			found = true

			// Do diff
			for l := 0; l < curr.Ncols; l++ {
				if l < interval[0] || l > interval[1] {
					diff.Values[i][l].String = curr.Values[i][l].String // don't diff, copy value as-is
					diff.Values[i][l].Valid = curr.Values[i][l].Valid
				} else {
					// Values with dots or in scientific notation consider as floats and integer otherwise.
					v, err := diffPair(curr.Values[i][l].String, prev.Values[j][l].String, itv)
					if err != nil {
						return diff, err
					}
					diff.Values[i][l].String = v
					diff.Values[i][l].Valid = true
				}
			}
			break // Go to searching next row from current snapshot
		}

		// End of the searching in 'previous' snapshot, if we reached here it means row not found and it simply should be added as is.
		if !found {
			for l := 0; l < curr.Ncols; l++ {
				diff.Values[i][l].String = curr.Values[i][l].String // don't diff, copy value as-is
				diff.Values[i][l].Valid = curr.Values[i][l].Valid
			}
		}
	}

	diff.Valid = true
	return diff, nil
}

// sort performs sorting of PGresult using order key and order.
func (r *PGresult) sort(key int, desc bool) {
	if r.Nrows == 0 {
		return /* nothing to sort */
	}

	_, err := strconv.ParseFloat(r.Values[0][key].String, 64)
	if err == nil {
		// value is numeric
		sort.Slice(r.Values, func(i, j int) bool {
			// TODO: handle errors
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

// diffPair produces a delta of two string values.
func diffPair(curr, prev string, itv int) (string, error) {
	if strings.Contains(prev, ".") || strings.Contains(prev, "e") ||
		strings.Contains(curr, ".") || strings.Contains(curr, "e") {
		cv, pv, err := parsePairFloat(curr, prev)
		if err != nil {
			return "", err
		}
		return strconv.FormatFloat((cv-pv)/float64(itv), 'f', 2, 64), nil
	}

	cv, pv, err := parsePairInt(curr, prev)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt((cv-pv)/int64(itv), 10), nil
}

// parsePairFloat parses pair of string and return two float64 values.
func parsePairFloat(curr, prev string) (float64, float64, error) {
	cv, err := strconv.ParseFloat(curr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("convert '%s' to float64 failed: %s", curr, err)
	}
	pv, err := strconv.ParseFloat(prev, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("convert '%s' to float64 failed: %s", prev, err)
	}

	return cv, pv, nil
}

// parsePairInt parses pair of string and return two int64 values.
func parsePairInt(curr, prev string) (int64, int64, error) {
	cv, err := strconv.ParseInt(curr, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("convert '%s' to int failed: %s", curr, err)
	}
	pv, err := strconv.ParseInt(prev, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("convert '%s' to int failed: %s", curr, err)
	}

	return cv, pv, nil
}

// Fprint prints content of PGresult container to buffer.
func (r *PGresult) Fprint(buf *bytes.Buffer) error {
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
		_, err := fmt.Fprintf(buf, "%-*s", widthMap[colidx]+2, colname)
		if err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(buf, "\n\n")
	if err != nil {
		return err
	}

	/* print data to buffer */
	for colnum, rownum := 0, 0; rownum < r.Nrows; rownum, colnum = rownum+1, 0 {
		for range r.Cols {
			/* m[row][column] */
			_, err := fmt.Fprintf(buf, "%-*s", widthMap[colnum]+2, r.Values[rownum][colnum].String)
			if err != nil {
				return err
			}
			colnum++
		}
		_, err := fmt.Fprintf(buf, "\n")
		if err != nil {
			return err
		}
	}

	return nil
}

// extensionSchema returns schema where the requested extension is installed in the database. Return empty string if not found.
func extensionSchema(db *postgres.DB, name string) string {
	var schema string
	err := db.QueryRow(query.GetExtensionSchema, name).Scan(&schema)
	if err != nil {
		// TODO: enable when proper logging will be implemented
		//fmt.Println("failed to check extensions in pg_extension: ", err)
		schema = ""
	}

	return schema
}

// isSchemaExists returns 'true' if requested schema exists in the database, and 'false' if not.
func isSchemaExists(db *postgres.DB, name string) bool {
	var exists bool
	err := db.QueryRow(query.CheckSchemaExists, name).Scan(&exists)
	if err != nil {
		// TODO: enable when proper logging will be implemented
		//fmt.Println("failed to check schema in information_schema: ", err)
		exists = false
	}

	return exists
}
