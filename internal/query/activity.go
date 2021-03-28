package query

const (
	// PgStatActivityDefault is the default query for getting stats from pg_stat_activity view
	// { Name: "pg_stat_activity", Query: common.PgStatActivityQueryDefault, DiffIntvl: [2]int{99,99}, Ncols: 14, OrderKey: 0, OrderDesc: true }
	// regexp_replace() removes extra spaces, tabs and newlines from queries
	PgStatActivityDefault = "SELECT pid, client_addr AS cl_addr, client_port AS cl_port, " +
		"datname, usename, left(application_name, 16) AS appname, backend_type, " +
		"wait_event_type AS wait_etype, wait_event, state, " +
		"date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age, " +
		"date_trunc('seconds', clock_timestamp() - query_start)::text AS query_age, " +
		"date_trunc('seconds', clock_timestamp() - state_change)::text AS change_age, " +
		`regexp_replace(query, E'\\s+', ' ', 'g') AS query ` +
		"FROM pg_stat_activity " +
		"WHERE ((clock_timestamp() - xact_start) > '{{.QueryAgeThresh}}'::interval " +
		"OR (clock_timestamp() - query_start) > '{{.QueryAgeThresh}}'::interval) " +
		"{{ if .ShowNoIdle }} AND state != 'idle' {{ end }} ORDER BY pid DESC"

	// PgStatActivity96 queries for getting stats from pg_stat_activity view for versions 9.6.*
	// { Name: "pg_stat_activity", Query: common.PgStatActivityQuery96, DiffIntvl: [2]int{99,99}, Ncols: 13, OrderKey: 0, OrderDesc: true }
	// regexp_replace() removes extra spaces, tabs and newlines from queries
	PgStatActivity96 = "SELECT pid, client_addr AS cl_addr, client_port AS cl_port, datname, " +
		"usename, left(application_name, 16) AS appname, wait_event_type AS wait_etype, " +
		"wait_event, state, date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age, " +
		"date_trunc('seconds', clock_timestamp() - query_start)::text AS query_age, " +
		"date_trunc('seconds', clock_timestamp() - state_change)::text AS change_age, " +
		`regexp_replace(query, E'\\s+', ' ', 'g') AS query ` +
		"FROM pg_stat_activity " +
		"WHERE ((clock_timestamp() - xact_start) > '{{.QueryAgeThresh}}'::interval " +
		"OR (clock_timestamp() - query_start) > '{{.QueryAgeThresh}}'::interval) " +
		"{{ if .ShowNoIdle }} AND state != 'idle' {{ end }} ORDER BY pid DESC"

	// PgStatActivity95 queries activity stats from pg_stat_activity view from versions for 9.5.* and later
	// { Name: "pg_stat_activity", Query: common.PgStatActivityQuery95, DiffIntvl: [2]int{99,99}, Ncols: 12, OrderKey: 0, OrderDesc: true }
	// regexp_replace() removes extra spaces, tabs and newlines from queries
	PgStatActivity95 = "SELECT pid, client_addr AS cl_addr, client_port AS cl_port, datname, " +
		"usename, left(application_name, 16) AS appname, waiting, state, " +
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
	switch {
	case version < 90600:
		return PgStatActivity95, 12
	case version < 100000:
		return PgStatActivity96, 13
	default:
		return PgStatActivityDefault, 14
	}
}
