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

// NewOptions creates query options used for queries customization depending on Postgres version and other important settings.
func NewOptions(version int, recovery string, querylen int) Options {
	opts := Options{
		ViewType:       "user",       // System tables and indexes aren't shown by default
		QueryAgeThresh: "00:00:00.0", // Don't filter queries by age
		ShowNoIdle:     true,         // Don't show idle clients and background workers
		PgSSQueryLen:   querylen,
	}

	opts.WalFunction1, opts.WalFunction2 = selectWalFunctions(version, recovery)

	// Define length limit for pg_stat_statement.query.
	if opts.PgSSQueryLen > 0 {
		opts.PgSSQueryLenFn = fmt.Sprintf("left(p.query, %d)", querylen)
	} else {
		opts.PgSSQueryLenFn = "p.query"
	}

	return opts
}

// selectWalFunctions returns proper function names for getting WAL locations.
// 1. WAL-related functions have been renamed in Postgres 10, hence functions' names between 9.x and 10 are differ.
// 2. Depending on recovery status, for obtaining WAL location different functions have to be used.
func selectWalFunctions(version int, recovery string) (string, string) {
	var fn1, fn2 string
	switch {
	case version < 100000:
		fn1 = "pg_xlog_location_diff"
		if recovery == "f" {
			fn2 = "pg_current_xlog_location"
		} else {
			fn2 = "pg_last_xlog_receive_location"
		}
	default:
		fn1 = "pg_wal_lsn_diff"
		if recovery == "f" {
			fn2 = "pg_current_wal_lsn"
		} else {
			fn2 = "pg_last_wal_receive_lsn"
		}
	}
	return fn1, fn2
}

// Format transforms query's template to a particular query.
func Format(tmpl string, o Options) (string, error) {
	t := template.Must(template.New("query").Parse(tmpl))
	buf := &bytes.Buffer{}
	if err := t.Execute(buf, o); err != nil {
		return "", err
	}

	return buf.String(), nil
}
