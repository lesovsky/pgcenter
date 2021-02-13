package query

const (
	// PgStatProgressVacuumDefault is the default query for getting stats from pg_stat_progress_vacuum view
	// { Name: "pg_stat_vacuum", Query: common.PgStatVacuumQueryDefault, DiffIntvl: [2]int{10,11}, Ncols: 13, OrderKey: 0, OrderDesc: true }
	PgStatProgressVacuumDefault = "SELECT a.pid, date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age, " +
		"v.datname, v.relid::regclass AS relation, a.state, coalesce((a.wait_event_type ||'.'|| a.wait_event), 'f') AS waiting, " +
		"v.phase, v.heap_blks_total * (SELECT current_setting('block_size')::int / 1024) AS t_size, " +
		`round(100 * v.heap_blks_scanned / v.heap_blks_total, 2)::text AS "t_scanned_%", ` +
		`round(100 * v.heap_blks_vacuumed / v.heap_blks_total, 2)::text AS "t_vacuumed_%", ` +
		"coalesce(v.heap_blks_scanned * (SELECT current_setting('block_size')::int / 1024), 0) AS scanned, " +
		"coalesce(v.heap_blks_vacuumed * (SELECT current_setting('block_size')::int / 1024), 0) AS vacuumed, a.query " +
		"FROM pg_stat_progress_vacuum v RIGHT JOIN pg_stat_activity a ON v.pid = a.pid " +
		"WHERE (a.query ~* '^autovacuum:' OR a.query ~* '^vacuum') AND a.pid <> pg_backend_pid() ORDER BY a.pid DESC"
)
