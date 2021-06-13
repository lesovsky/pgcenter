package query

const (
	// PgStatFunctionsDefault is the default query for getting stats from pg_stat_user_functions view
	PgStatFunctionsDefault = "SELECT funcid, schemaname ||'.'||funcname AS function, " +
		"calls AS calls_total, calls AS calls, " +
		"date_trunc('seconds', total_time / 1000 * '1 second'::interval)::text AS total, " +
		"date_trunc('seconds', self_time / 1000 * '1 second'::interval)::text AS self, " +
		`round((total_time / greatest(calls, 1))::numeric(20,2), 4)::text AS "total_avg,ms", ` +
		`round((self_time / greatest(calls, 1))::numeric(20,2), 4)::text AS "self_avg,ms" ` +
		"FROM pg_stat_user_functions ORDER BY funcid DESC"
)
