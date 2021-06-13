package query

const (
	// PgStatProgressVacuumDefault defines default query for getting stats from pg_stat_progress_vacuum view.
	PgStatProgressVacuumDefault = "SELECT a.pid, date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age, " +
		"v.datname, v.relid::regclass AS relation, a.state, coalesce((a.wait_event_type ||'.'|| a.wait_event), 'f') AS waiting, " +
		`v.phase, v.heap_blks_total * (SELECT current_setting('block_size')::int / 1024) AS "size_total,KiB", ` +
		`round(100 * v.heap_blks_scanned / v.heap_blks_total, 2)::text AS "scanned_total,%", ` +
		`round(100 * v.heap_blks_vacuumed / v.heap_blks_total, 2)::text AS "vacuumed_total,%", ` +
		`coalesce(v.heap_blks_scanned * (SELECT current_setting('block_size')::int / 1024), 0) AS "scanned,KiB", ` +
		`coalesce(v.heap_blks_vacuumed * (SELECT current_setting('block_size')::int / 1024), 0) AS "vacuumed,KiB", a.query ` +
		"FROM pg_stat_progress_vacuum v RIGHT JOIN pg_stat_activity a ON v.pid = a.pid " +
		"WHERE (a.query ~* '^autovacuum:' OR a.query ~* '^vacuum') AND a.pid <> pg_backend_pid() ORDER BY a.pid DESC"
)
