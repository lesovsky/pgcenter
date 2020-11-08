package query

import (
	"bytes"
	"fmt"
	"text/template"
)

// Options contains queries' settings that used depending on user preferences.
type Options struct {
	ViewType       string // Show stats including system tables/indexes
	WalFunction1   string // Use old pg_xlog_* or newer pg_wal_* functions
	WalFunction2   string // Use old pg_xlog_* or newer pg_wal_* functions
	QueryAgeThresh string // Show only queries with duration more than specified
	BackendState   string // Backend state's selector for cancel/terminate function
	ShowNoIdle     bool   // don't show IDLEs, background workers)
	PgSSQueryLen   int    // Specify the length of query to show in pg_stat_statements
	PgSSQueryLenFn string // Specify exact func to truncating query
}

// PrepareQuery transforms query's template to a particular query
func PrepareQuery(s string, o Options) (string, error) {
	t := template.Must(template.New("query").Parse(s))
	buf := &bytes.Buffer{}
	if err := t.Execute(buf, o); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// Adjust method used for adjusting query's options depending on Postgres version.
func (o *Options) Adjust(version int, recovery string, util string) {
	// System tables and indexes aren't shown by default
	o.ViewType = "user"
	// Don't filter queries by age
	o.QueryAgeThresh = "00:00:00.0"
	// Don't show idle clients and background workers
	o.ShowNoIdle = true

	// Select proper WAL functions
	// 1. WAL-related functions have been renamed in Postgres 10, hence functions' names between 9.x and 10 are differ.
	// 2. Depending on recovery status, for obtaining WAL location different functions have to be used.
	switch {
	case version < 100000:
		o.WalFunction1 = "pg_xlog_location_diff"
		if recovery == "f" {
			o.WalFunction2 = "pg_current_xlog_location"
		} else {
			o.WalFunction2 = "pg_last_xlog_receive_location"
		}
	default:
		o.WalFunction1 = "pg_wal_lsn_diff"
		if recovery == "f" {
			o.WalFunction2 = "pg_current_wal_lsn"
		} else {
			o.WalFunction2 = "pg_last_wal_receive_lsn"
		}
	}

	// Queries settings that are specific for particular utilities
	switch util {
	case "top":
		// we want truncate query length of pg_stat_statements.query, because it make no sense to process full query when sizes of user's screen is limited
		o.PgSSQueryLenFn = "left(p.query, 256)"
	case "record":
		// in case of record program we want to record full length of the query, if user doesn't specified exact length
		if o.PgSSQueryLen != 0 {
			o.PgSSQueryLenFn = fmt.Sprintf("left(p.query, %d)", o.PgSSQueryLen)
		} else {
			o.PgSSQueryLenFn = "p.query"
		}
	}
}
