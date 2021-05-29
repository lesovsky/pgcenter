package query

import (
	"bytes"
	"fmt"
	"text/template"
)

const (
	PostgresV94 = 90400
	PostgresV95 = 90500
	PostgresV96 = 90600
	PostgresV10 = 100000
	PostgresV11 = 110000
	PostgresV12 = 120000
	PostgresV13 = 130000
	PostgresV14 = 140000
)

// Options contains queries' settings that used depending on user preferences.
type Options struct {
	Version          int    // Postgres version (numeric format)
	Recovery         string // Recovery state
	GucTrackCommitTS string // Value of track_commit_timestamp GUC
	ViewType         string // Show stats including system tables/indexes
	WalFunction1     string // Use old pg_xlog_* or newer pg_wal_* functions
	WalFunction2     string // Use old pg_xlog_* or newer pg_wal_* functions
	QueryAgeThresh   string // Show only queries with duration more than specified
	BackendState     string // Backend state's selector for cancel/terminate function
	ShowNoIdle       bool   // don't show IDLEs, background workers)
	PGSSSchema       string // Schema where pg_stat_statements installed
	PgSSQueryLen     int    // Specify the length of query to show in pg_stat_statements
	PgSSQueryLenFn   string // Specify exact func to truncating query
}

// NewOptions creates query options used for queries customization depending on Postgres version and other important settings.
func NewOptions(version int, recovery string, track string, querylen int, pgssSchema string) Options {
	opts := Options{
		Version:          version,
		Recovery:         recovery,
		GucTrackCommitTS: track,
		ViewType:         "user",       // System tables and indexes aren't shown by default
		QueryAgeThresh:   "00:00:00.0", // Don't filter queries by age
		ShowNoIdle:       true,         // Don't show idle clients and background workers
		PGSSSchema:       pgssSchema,
		PgSSQueryLen:     querylen,
	}

	opts.WalFunction1, opts.WalFunction2 = selectWalFunctions(opts.Version, opts.Recovery)

	// Define length limit for pg_stat_statement.query.
	if opts.PgSSQueryLen > 0 {
		opts.PgSSQueryLenFn = fmt.Sprintf("left(p.query, %d)", opts.PgSSQueryLen)
	} else {
		opts.PgSSQueryLenFn = "p.query"
	}

	return opts
}

// selectWalFunctions returns proper function names for getting WAL locations.
// 1. WAL-related functions have been renamed in Postgres 10, functions' names between 9.x and 10 are different.
// 2. Depending on recovery status, for obtaining WAL location different functions have to be used.
func selectWalFunctions(version int, recovery string) (string, string) {
	var fn1, fn2 string
	switch {
	case version < PostgresV10:
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
	t, err := template.New("query").Parse(tmpl)
	if err != nil {
		return "", err
	}

	buf := &bytes.Buffer{}
	err = t.Execute(buf, o)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
