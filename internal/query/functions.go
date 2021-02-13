package query

const (
	// PgStatFunctionsDefault is the default query for getting stats from pg_stat_user_functions view
	// { Name: "pg_stat_functions", Query: common.PgStatFunctionsQueryDefault, DiffIntvl: [2]int{3,3}, Ncols: 8, OrderKey: 0, OrderDesc: true }
	PgStatFunctionsDefault = "SELECT funcid, schemaname ||'.'||funcname AS function, " +
		"calls AS total_calls, calls AS calls, " +
		"date_trunc('seconds', total_time / 1000 * '1 second'::interval)::text AS total_t, " +
		"date_trunc('seconds', self_time / 1000 * '1 second'::interval)::text AS self_t, " +
		"round((total_time / greatest(calls, 1))::numeric(20,2), 4)::text AS avg_t, " +
		"round((self_time / greatest(calls, 1))::numeric(20,2), 4)::text AS avg_self_t " +
		"FROM pg_stat_user_functions ORDER BY funcid DESC"
)
