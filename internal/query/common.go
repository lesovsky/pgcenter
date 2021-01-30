package query

const (
	// GetSetting queries specified Postgres configuration setting
	GetSetting = "SELECT current_setting($1)"
	// GetRecoveryStatus queries current Postgres recovery status.
	GetRecoveryStatus = "SELECT pg_is_in_recovery()"
	// GetUptime queries Postgres uptime.
	GetUptime = "SELECT date_trunc('seconds', now() - pg_postmaster_start_time())"
	// CheckSchemaExists checks schema exists in the database.
	CheckSchemaExists = "SELECT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)"
	// CheckExtensionExists checks extension is installed in the database.
	CheckExtensionExists = "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = $1)"
	// GetAllSettings queries current Postgres configuration
	GetAllSettings = "SELECT name, setting, unit, category FROM pg_settings ORDER BY 4"
	// GetCurrentLogfile queries current Postgres logfile
	GetCurrentLogfile = "SELECT pg_current_logfile()"
	// ExecReloadConf does Postgres reload
	ExecReloadConf = "SELECT pg_reload_conf()"
	// ExecCancelQuery cancels query executed by backend with specified PID
	ExecCancelQuery = "SELECT pg_cancel_backend($1)"
	// ExecTerminateBackend terminates the backend with specified PID
	ExecTerminateBackend = "SELECT pg_terminate_backend($1)"
	// ExecCancelQueryGroup cancels a group of queries based on specified criteria
	ExecCancelQueryGroup = "SELECT count(pg_cancel_backend(pid)) " +
		"FROM pg_stat_activity WHERE {{.BackendState}} " +
		"AND ((clock_timestamp() - xact_start) > '{{.QueryAgeThresh}}'::interval " +
		"OR (clock_timestamp() - query_start) > '{{.QueryAgeThresh}}'::interval) " +
		"AND pid != pg_backend_pid()"
	// ExecTerminateBackendGroup terminate a group of backends based on specified criteria
	ExecTerminateBackendGroup = "SELECT count(pg_terminate_backend(pid)) " +
		"FROM pg_stat_activity WHERE {{.BackendState}} " +
		"AND ((clock_timestamp() - xact_start) > '{{.QueryAgeThresh}}'::interval " +
		"OR (clock_timestamp() - query_start) > '{{.QueryAgeThresh}}'::interval) " +
		"AND pid != pg_backend_pid()"
	// ExecResetStats resets statistics counter in the current database
	ExecResetStats = "SELECT pg_stat_reset()"
	// ExecResetPgStatStatements resets pg_stat_statements statistics
	ExecResetPgStatStatements = "SELECT pg_stat_statements_reset()"

	// SelectCommonProperties used for getting Postgres settings necessary during pgcenter runtime.
	//   Notes: track_commit_timestamp introduced in 9.5
	SelectCommonProperties = "SELECT current_setting('server_version'), current_setting('server_version_num')::int, " +
		"current_setting('track_commit_timestamp'), " +
		"current_setting('max_connections')::int, " +
		"current_setting('autovacuum_max_workers')::int, " +
		"pg_is_in_recovery(), " +
		"extract(epoch from pg_postmaster_start_time())"

	// SelectActivityDefault is the default query for getting stats about connected clients from pg_stat_activity
	//   Postgres 10: The 'backend_type' has been introduced.
	SelectActivityDefault = "SELECT count(*) FILTER (WHERE state IS NOT NULL) AS total, " +
		"count(*) FILTER (WHERE state = 'idle') AS idle, " +
		"count(*) FILTER (WHERE state IN ('idle in transaction', 'idle in transaction (aborted)')) AS idle_in_xact, " +
		"count(*) FILTER (WHERE state = 'active') AS active, " +
		"count(*) FILTER (WHERE wait_event_type = 'Lock') AS waiting, " +
		"count(*) FILTER (WHERE state IN ('fastpath function call','disabled')) AS others, " +
		"(SELECT count(*) FROM pg_prepared_xacts) AS total_prepared " +
		"FROM pg_stat_activity WHERE backend_type = 'client backend'"

	// SelectActivityPG96 queries activity stats about connected clients for versions 9.6 and earlier. The 'backend_type' has been introduced in 10.
	//   Postgres 9.6: wait_events have been introduced.
	SelectActivityPG96 = "SELECT count(*) FILTER (WHERE state IS NOT NULL) AS total, " +
		"count(*) FILTER (WHERE state = 'idle') AS idle, " +
		"count(*) FILTER (WHERE state IN ('idle in transaction', 'idle in transaction (aborted)')) AS idle_in_xact, " +
		"count(*) FILTER (WHERE state = 'active') AS active, " +
		"count(*) FILTER (WHERE wait_event_type = 'Lock') AS waiting, " +
		"count(*) FILTER (WHERE state IN ('fastpath function call','disabled')) AS others, " +
		"(SELECT count(*) FROM pg_prepared_xacts) AS total_prepared " +
		"FROM pg_stat_activity"

	// SelectActivityPG95 queries stats activity about connected clients for versions 9.5 and earlier.
	//   Postgres 9.4: 'FILTER (WHERE ...)' has been introduced.
	SelectActivityPG95 = "SELECT count(*) FILTER (WHERE state IS NOT NULL) AS total, " +
		"count(*) FILTER (WHERE state = 'idle') AS idle, " +
		"count(*) FILTER (WHERE state IN ('idle in transaction', 'idle in transaction (aborted)')) AS idle_in_xact, " +
		"count(*) FILTER (WHERE state = 'active') AS active, " +
		"count(*) FILTER (WHERE waiting) AS waiting, " +
		"count(*) FILTER (WHERE state IN ('fastpath function call','disabled')) AS others, " +
		"(SELECT count(*) FROM pg_prepared_xacts) AS total_prepared " +
		"FROM pg_stat_activity"

	// SelectActivityPG93 queries stats activity about connected clients for versions 9.3 and earlier.
	SelectActivityPG93 = "WITH pgsa AS (SELECT * FROM pg_stat_activity) SELECT " +
		"(SELECT count(*) FROM pgsa) AS total, " +
		"(SELECT count(*) FROM pgsa WHERE state = 'idle') AS idle, " +
		"(SELECT count(*) FROM pgsa WHERE state IN ('idle in transaction', 'idle in transaction (aborted)')) AS idle_in_xact, " +
		"(SELECT count(*) FROM pgsa WHERE state = 'active') AS active, " +
		"(SELECT count(*) FROM pgsa WHERE waiting) AS waiting, " +
		"(SELECT count(*) FROM pgsa WHERE state IN ('fastpath function call','disabled')) AS others, " +
		"(SELECT count(*) FROM pg_prepared_xacts) AS total_prepared"

	// SelectAutovacuumDefault is the default query for getting stats about autovacuum activity from pg_stat_activity
	//   Postgres 9.4: 'FILTER (WHERE ...)' has been introduced.
	SelectAutovacuumDefault = "SELECT count(*) FILTER (WHERE query ~* '^autovacuum:') AS av_workers, " +
		"count(*) FILTER (WHERE query ~* '^autovacuum:.*to prevent wraparound') AS av_wrap, " +
		"count(*) FILTER (WHERE query ~* '^vacuum' AND state != 'idle') AS v_manual, " +
		"coalesce(date_trunc('seconds', max(now() - xact_start)), '00:00:00') AS av_maxtime " +
		"FROM pg_stat_activity WHERE (query ~* '^autovacuum:' OR query ~* '^vacuum') AND pid <> pg_backend_pid()"

	// SelectAutovacuumPG93 queries stats about autovacuum activity for versions 9.3 and earlier.
	SelectAutovacuumPG93 = "WITH pgsa AS (SELECT * FROM pg_stat_activity) SELECT " +
		"(SELECT count(*) FROM pgsa WHERE query ~* '^autovacuum:' AND pid <> pg_backend_pid()) AS av_workers, " +
		"(SELECT count(*) FROM pgsa WHERE query ~* '^autovacuum:.*to prevent wraparound' AND pid <> pg_backend_pid()) AS av_wrap, " +
		"(SELECT count(*) FROM pgsa WHERE query ~* '^vacuum' AND pid <> pg_backend_pid()) AS v_manual, " +
		"(SELECT coalesce(date_trunc('seconds', max(now() - xact_start)), '00:00:00') FROM pgsa " +
		"WHERE (query ~* '^autovacuum:' OR query ~* '^vacuum') AND pid <> pg_backend_pid()) AS av_maxtime"

	// SelectActivityTimes queries stats about longest activity
	SelectActivityTimes = "SELECT (SELECT coalesce(date_trunc('seconds', max(now() - xact_start)), '00:00:00') AS xact_maxtime " +
		"FROM pg_stat_activity WHERE (query !~* '^autovacuum:' AND query !~* '^vacuum') AND pid <> pg_backend_pid()), " +
		"(SELECT COALESCE(date_trunc('seconds', max(clock_timestamp() - prepared)), '00:00:00') AS prep_maxtime " +
		"FROM pg_prepared_xacts)"

	// SelectActivityStatements queries general stats from pg_stat_statements
	//   Postgres 13: total_time replaced to total_exec_time, total_plan_time.
	SelectActivityStatementsPG12   = "SELECT (sum(total_time) / sum(calls))::numeric(20,2) AS avg_query, sum(calls) AS total_calls FROM pg_stat_statements"
	SelectActivityStatementsLatest = "SELECT (sum(total_exec_time) / sum(calls))::numeric(20,2) AS avg_query, sum(calls) AS total_calls FROM pg_stat_statements"

	// SelectRemoteProcSysTicks queries system timer's frequency from Postgres instance
	SelectRemoteProcSysTicks = "SELECT pgcenter.get_sys_clk_ticks()::float"
)

// SelectActivityActivityQuery returns activity main query depending on used version.
func SelectActivityActivityQuery(version int) string {
	switch {
	case version < 90400:
		return SelectActivityPG93
	case version < 90600:
		return SelectActivityPG95
	case version < 100000:
		return SelectActivityPG96
	default:
		return SelectActivityDefault
	}
}

// SelectActivityAutovacuumQuery returns autovacuum activity query depending on used version.
func SelectActivityAutovacuumQuery(version int) string {
	switch {
	case version < 90400:
		return SelectAutovacuumPG93
	default:
		return SelectAutovacuumDefault
	}
}

// SelectActivityStatementsQuery returns statements activity query depending on used version.
func SelectActivityStatementsQuery(version int) string {
	switch {
	case version < 130000:
		return SelectActivityStatementsPG12
	default:
		return SelectActivityStatementsLatest
	}
}
