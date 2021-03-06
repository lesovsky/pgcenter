package query

const (
	// PgStatProgressAnalyzeDefault is the default query for getting stats from pg_stat_progress_analyze view
	PgStatProgressAnalyzeDefault = "SELECT " +
		"a.pid, date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age, p.datname, p.relid::regclass AS relation," +
		"a.state, coalesce((a.wait_event_type ||'.'|| a.wait_event), 'f') AS waiting, p.phase," +
		"p.sample_blks_total * (SELECT current_setting('block_size')::int / 1024) AS t_size," +
		`round(100 * p.sample_blks_scanned / greatest(p.sample_blks_total,1), 2)::text AS "scanned_%",` +
		`p.ext_stats_total ||'/'|| p.ext_stats_computed::text AS "ext_total/done",` +
		`p.child_tables_total||'/'|| round(100 * p.child_tables_done / greatest(p.child_tables_total, 1), 2)::text AS "child_total/done_%",` +
		"current_child_table_relid::regclass AS child_in_progress " +
		"FROM pg_stat_progress_analyze p INNER JOIN pg_stat_activity a ON p.pid = a.pid " +
		"WHERE a.pid <> pg_backend_pid() ORDER BY a.pid DESC"
)
