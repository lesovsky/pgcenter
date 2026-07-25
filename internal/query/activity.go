package query

const (
	// PgStatActivityPG13 queries activity stats from pg_stat_activity view for versions 13 and newer.
	// Compared to PgStatActivityDefault it adds three columns:
	// - leader: coalesce(leader_pid, pid), so a parallel query's leader and its workers share one
	//   identifier and the whole group collapses when sorted by it (raw leader_pid is NULL for the
	//   leader itself; the column exists since PG 13, which is what the version boundary is about).
	// - backend_xid: the transaction id assigned once the transaction has written, cast to text
	//   because xid does not scan into the string matrix. No coalesce: a backend that has not
	//   written must render blank, never 0.
	// - horizon_xacts: age(backend_xmin), how many transactions back the session holds the xmin
	//   horizon. Returns integer, so no cast is needed. No coalesce, for the same reason.
	// - regexp_replace() removes extra spaces, tabs and newlines from queries.
	PgStatActivityPG13 = "SELECT pid, coalesce(leader_pid, pid) AS leader, " +
		"host(client_addr) AS cl_addr, client_port AS cl_port, " +
		"datname, usename, application_name AS appname, backend_type, " +
		"wait_event_type AS wait_etype, wait_event, state, " +
		"backend_xid::text, age(backend_xmin) AS horizon_xacts, " +
		"date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age, " +
		"date_trunc('seconds', clock_timestamp() - query_start)::text AS query_age, " +
		"date_trunc('seconds', clock_timestamp() - state_change)::text AS change_age, " +
		`regexp_replace(query, E'\\s+', ' ', 'g') AS query ` +
		"FROM pg_stat_activity " +
		"WHERE ((clock_timestamp() - xact_start) > '{{.QueryAgeThresh}}'::interval " +
		"OR (clock_timestamp() - query_start) > '{{.QueryAgeThresh}}'::interval) " +
		"{{ if .ShowNoIdle }} AND state != 'idle' {{ end }} ORDER BY pid DESC"

	// PgStatActivityDefault is the default query for getting stats from pg_stat_activity view.
	// - regexp_replace() removes extra spaces, tabs and newlines from queries.
	PgStatActivityDefault = "SELECT pid, host(client_addr) AS cl_addr, client_port AS cl_port, " +
		"datname, usename, application_name AS appname, backend_type, " +
		"wait_event_type AS wait_etype, wait_event, state, " +
		"date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age, " +
		"date_trunc('seconds', clock_timestamp() - query_start)::text AS query_age, " +
		"date_trunc('seconds', clock_timestamp() - state_change)::text AS change_age, " +
		`regexp_replace(query, E'\\s+', ' ', 'g') AS query ` +
		"FROM pg_stat_activity " +
		"WHERE ((clock_timestamp() - xact_start) > '{{.QueryAgeThresh}}'::interval " +
		"OR (clock_timestamp() - query_start) > '{{.QueryAgeThresh}}'::interval) " +
		"{{ if .ShowNoIdle }} AND state != 'idle' {{ end }} ORDER BY pid DESC"

	// PgStatActivity96 queries for getting stats from pg_stat_activity view for versions 9.6.*.
	// - regexp_replace() removes extra spaces, tabs and newlines from queries.
	PgStatActivity96 = "SELECT pid, host(client_addr) AS cl_addr, client_port AS cl_port, datname, " +
		"usename, application_name AS appname, wait_event_type AS wait_etype, " +
		"wait_event, state, date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age, " +
		"date_trunc('seconds', clock_timestamp() - query_start)::text AS query_age, " +
		"date_trunc('seconds', clock_timestamp() - state_change)::text AS change_age, " +
		`regexp_replace(query, E'\\s+', ' ', 'g') AS query ` +
		"FROM pg_stat_activity " +
		"WHERE ((clock_timestamp() - xact_start) > '{{.QueryAgeThresh}}'::interval " +
		"OR (clock_timestamp() - query_start) > '{{.QueryAgeThresh}}'::interval) " +
		"{{ if .ShowNoIdle }} AND state != 'idle' {{ end }} ORDER BY pid DESC"

	// PgStatActivity95 queries activity stats from pg_stat_activity view from versions for 9.5.* and later.
	// - regexp_replace() removes extra spaces, tabs and newlines from queries.
	PgStatActivity95 = "SELECT pid, host(client_addr) AS cl_addr, client_port AS cl_port, datname, " +
		"usename, application_name AS appname, waiting, state, " +
		"date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age, " +
		"date_trunc('seconds', clock_timestamp() - query_start)::text AS query_age, " +
		"date_trunc('seconds', clock_timestamp() - state_change)::text AS change_age, " +
		`regexp_replace(query, E'\\s+', ' ', 'g') AS query ` +
		"FROM pg_stat_activity " +
		"WHERE ((clock_timestamp() - xact_start) > '{{.QueryAgeThresh}}'::interval " +
		"OR (clock_timestamp() - query_start) > '{{.QueryAgeThresh}}'::interval) " +
		"{{ if .ShowNoIdle }} AND state != 'idle' {{ end }} ORDER BY pid DESC"
)

// SelectStatActivityQuery returns proper query and number of columns, depending on Postgres version.
func SelectStatActivityQuery(version int) (string, int) {
	// leader_pid exists since PG 13, so the widened query is handled here, above the historical
	// ladder below - which consequently covers PG 10-12 in its default case.
	if version >= PostgresV13 {
		return PgStatActivityPG13, 17
	}

	switch {
	case version < 90600:
		return PgStatActivity95, 12
	case version < 100000:
		return PgStatActivity96, 13
	default:
		return PgStatActivityDefault, 14
	}
}
