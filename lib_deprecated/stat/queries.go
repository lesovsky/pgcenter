// Stuff related to particular SQL queries that used for gathering stats

package stat

import (
	"bytes"
	"fmt"
	"text/template"
)

const (
	// PgGetSingleSettingQuery queries specified Postgres configuration setting
	PgGetSingleSettingQuery = "SELECT current_setting($1)"
	// PgGetVersionQuery queries Postgres versions
	PgGetVersionQuery = "SELECT current_setting('server_version'),current_setting('server_version_num')::int"
	// PgGetRecoveryStatusQuery queries current Postgres recovery status
	PgGetRecoveryStatusQuery = "SELECT pg_is_in_recovery()"
	// PgGetUptimeQuery queries Postgres uptime
	PgGetUptimeQuery = "SELECT date_trunc('seconds', now() - pg_postmaster_start_time())"
	// PgCheckPGSSExists checks that pg_stat_statements view exists
	PgCheckPGSSExists = "SELECT EXISTS (SELECT 1 FROM information_schema.views WHERE table_name = 'pg_stat_statements')"
	// PgCheckPgcenterSchemaQuery checks existence of pgcenter's stats schema
	PgCheckPgcenterSchemaQuery = "SELECT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = 'pgcenter')"
	// PgGetConfigAllQuery queries current Postgres configuration
	PgGetConfigAllQuery = "SELECT name, setting, unit, category FROM pg_settings ORDER BY 4"
	// PgGetCurrentLogfileQuery queries current Postgres logfile
	PgGetCurrentLogfileQuery = "SELECT pg_current_logfile();"
	// PgReloadConfQuery does Postgres reload
	PgReloadConfQuery = "SELECT pg_reload_conf()"
	// PgPostmasterStartTimeQuery queries time when Postgres has been started
	PgPostmasterStartTimeQuery = "SELECT to_char(pg_postmaster_start_time(), 'HH24MISS')"
	// PgCancelSingleQuery cancels query executed by backend with specified PID
	PgCancelSingleQuery = `SELECT pg_cancel_backend($1)`
	// PgTerminateSingleQuery terminates the backend with specified PID
	PgTerminateSingleQuery = `SELECT pg_terminate_backend($1)`
	// PgCancelGroupQuery cancels a group of queries based on specified criteria
	PgCancelGroupQuery = `SELECT
count(pg_cancel_backend(pid))
FROM pg_stat_activity
WHERE {{.BackendState}}
AND ((clock_timestamp() - xact_start) > '{{.QueryAgeThresh}}'::interval OR (clock_timestamp() - query_start) > '{{.QueryAgeThresh}}'::interval)
AND pid != pg_backend_pid()`
	// PgTerminateGroupQuery terminate a group of backends based on specified crteria
	PgTerminateGroupQuery = `SELECT
count(pg_terminate_backend(pid))
FROM pg_stat_activity
WHERE {{.BackendState}}
AND ((clock_timestamp() - xact_start) > '{{.QueryAgeThresh}}'::interval OR (clock_timestamp() - query_start) > '{{.QueryAgeThresh}}'::interval)
AND pid != pg_backend_pid()`
	// PgResetStats resets statistics counter in the current database
	PgResetStats = "SELECT pg_stat_reset()"
	// PgResetPgss resets pg_stat_statements statistics
	PgResetPgss = "SELECT pg_stat_statements_reset()"

	// PgActivityQueryDefault is the default query for getting stats about connected clients from pg_stat_activity
	PgActivityQueryDefault = `SELECT
count(*) FILTER (WHERE state IS NOT NULL) AS total,
count(*) FILTER (WHERE state = 'idle') AS idle,
count(*) FILTER (WHERE state IN ('idle in transaction', 'idle in transaction (aborted)')) AS idle_in_xact,
count(*) FILTER (WHERE state = 'active') AS active,
count(*) FILTER (WHERE wait_event_type = 'Lock') AS waiting,
count(*) FILTER (WHERE state IN ('fastpath function call','disabled')) AS others,
(SELECT count(*) FROM pg_prepared_xacts) AS total_prepared
FROM pg_stat_activity WHERE backend_type = 'client backend'`

	// PgActivityQueryBefore10 queries activity stats about connected clients for versions prior 10. The 'backend_type' has been introduced in 10.
	PgActivityQueryBefore10 = `SELECT
count(*) FILTER (WHERE state IS NOT NULL) AS total,
count(*) FILTER (WHERE state = 'idle') AS idle,
count(*) FILTER (WHERE state IN ('idle in transaction', 'idle in transaction (aborted)')) AS idle_in_xact,
count(*) FILTER (WHERE state = 'active') AS active,
count(*) FILTER (WHERE wait_event_type = 'Lock') AS waiting,
count(*) FILTER (WHERE state IN ('fastpath function call','disabled')) AS others,
(SELECT count(*) FROM pg_prepared_xacts) AS total_prepared
FROM pg_stat_activity`

	// PgActivityQueryBefore96 queries stats activity about connected clients for versions prior 9.6. There wait_events have been introduced in 9.6.
	PgActivityQueryBefore96 = `SELECT
count(*) FILTER (WHERE state IS NOT NULL) AS total,
count(*) FILTER (WHERE state = 'idle') AS idle,
count(*) FILTER (WHERE state IN ('idle in transaction', 'idle in transaction (aborted)')) AS idle_in_xact,
count(*) FILTER (WHERE state = 'active') AS active,
count(*) FILTER (WHERE waiting) AS waiting,
count(*) FILTER (WHERE state IN ('fastpath function call','disabled')) AS others,
(SELECT count(*) FROM pg_prepared_xacts) AS total_prepared
FROM pg_stat_activity`

	// PgActivityQueryBefore94 queries stats activity about connected clients for versions prior 9.4. There 'FILTER (WHERE ...)' has been introduced in 9.4.
	PgActivityQueryBefore94 = `WITH pgsa AS (SELECT * FROM pg_stat_activity)
SELECT
(SELECT count(*) FROM pgsa) AS total,
(SELECT count(*) FROM pgsa WHERE state = 'idle') AS idle,
(SELECT count(*) FROM pgsa WHERE state IN ('idle in transaction', 'idle in transaction (aborted)')) AS idle_in_xact,
(SELECT count(*) FROM pgsa WHERE state = 'active') AS active,
(SELECT count(*) FROM pgsa WHERE waiting) AS waiting,
(SELECT count(*) FROM pgsa WHERE state IN ('fastpath function call','disabled')) AS others,
(SELECT count(*) FROM pg_prepared_xacts) AS total_prepared`

	// PgAutovacQueryDefault is the default query for getting stats about autovacuum activity from pg_stat_activity
	PgAutovacQueryDefault = `SELECT
count(*) FILTER (WHERE query ~* '^autovacuum:') AS av_workers,
count(*) FILTER (WHERE query ~* '^autovacuum:.*to prevent wraparound') AS av_wrap,
count(*) FILTER (WHERE query ~* '^vacuum' AND state != 'idle') AS v_manual,
coalesce(date_trunc('seconds', max(now() - xact_start)), '00:00:00') AS av_maxtime
FROM pg_stat_activity
WHERE (query ~* '^autovacuum:' OR query ~* '^vacuum') AND pid <> pg_backend_pid()`

	// PgAutovacQueryBefore94 queries stats about autovacuum activity for versions prior 9.4. There 'FILTER (WHERE ...)' has been introduced.
	PgAutovacQueryBefore94 = `WITH pgsa AS (SELECT * FROM pg_stat_activity)
SELECT
(SELECT count(*) FROM pgsa WHERE query ~* '^autovacuum:' AND pid <> pg_backend_pid()) AS av_workers,
(SELECT count(*) FROM pgsa WHERE query ~* '^autovacuum:.*to prevent wraparound' AND pid <> pg_backend_pid()) AS av_wrap,
(SELECT count(*) FROM pgsa WHERE query ~* '^vacuum' AND pid <> pg_backend_pid()) AS v_manual,
(SELECT coalesce(date_trunc('seconds', max(now() - xact_start)), '00:00:00') FROM pgsa
WHERE (query ~* '^autovacuum:' OR query ~* '^vacuum') AND pid <> pg_backend_pid()) AS av_maxtime`

	// PgActivityTimeQuery queries stats about longest transactions
	PgActivityTimeQuery = `SELECT
(SELECT coalesce(date_trunc('seconds', max(now() - xact_start)), '00:00:00') AS xact_maxtime
FROM pg_stat_activity
WHERE (query !~* '^autovacuum:' AND query !~* '^vacuum') AND pid <> pg_backend_pid()),
(SELECT COALESCE(date_trunc('seconds', max(clock_timestamp() - prepared)), '00:00:00') AS prep_maxtime
FROM pg_prepared_xacts)`

	// PgStatementsQuery queries general stats from pg_stat_statements
	PgStatementsQuery = `SELECT (sum(total_time) / sum(calls))::numeric(20,2) AS avg_query, sum(calls) AS total_calls FROM pg_stat_statements`
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
func (o *Options) Adjust(pi PgInfo, util string) {
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
	case pi.PgVersionNum < 100000:
		o.WalFunction1 = "pg_xlog_location_diff"
		if pi.PgRecovery == "false" {
			o.WalFunction2 = "pg_current_xlog_location"
		} else {
			o.WalFunction2 = "pg_last_xlog_receive_location"
		}
	default:
		o.WalFunction1 = "pg_wal_lsn_diff"
		if pi.PgRecovery == "false" {
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
