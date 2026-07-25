package query

const (
	// PgStatProgressAnalyzeDefault defines default query for getting stats from pg_stat_progress_analyze view.
	PgStatProgressAnalyzeDefault = "SELECT " +
		"a.pid, date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age, p.datname, p.relid::regclass AS relation, " +
		"a.state, coalesce((a.wait_event_type ||'.'|| a.wait_event), 'f') AS waiting, p.phase, " +
		`p.sample_blks_total * (SELECT current_setting('block_size')::int / 1024) AS "sample_size,KiB", ` +
		`round(100 * p.sample_blks_scanned / greatest(p.sample_blks_total,1), 2)::text AS "scanned,%", ` +
		`p.ext_stats_total ||'/'|| p.ext_stats_computed::text AS "ext_total/done", ` +
		`p.child_tables_total||'/'|| round(100 * p.child_tables_done / greatest(p.child_tables_total, 1), 2)::text AS "child_total/done,%", ` +
		"current_child_table_relid::regclass AS child_in_progress " +
		"FROM pg_stat_progress_analyze p INNER JOIN pg_stat_activity a ON p.pid = a.pid " +
		"WHERE a.pid <> pg_backend_pid() ORDER BY a.pid DESC"

	// PgStatProgressAnalyzePG19 defines query for getting stats from pg_stat_progress_analyze on PG 19 and newer.
	// PG 19 adds started_by (manual/autovacuum), placed right after relation. The view is a snapshot one,
	// so the diff interval stays empty.
	PgStatProgressAnalyzePG19 = "SELECT " +
		"a.pid, date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age, p.datname, p.relid::regclass AS relation, " +
		"p.started_by, a.state, coalesce((a.wait_event_type ||'.'|| a.wait_event), 'f') AS waiting, p.phase, " +
		`p.sample_blks_total * (SELECT current_setting('block_size')::int / 1024) AS "sample_size,KiB", ` +
		`round(100 * p.sample_blks_scanned / greatest(p.sample_blks_total,1), 2)::text AS "scanned,%", ` +
		`p.ext_stats_total ||'/'|| p.ext_stats_computed::text AS "ext_total/done", ` +
		`p.child_tables_total||'/'|| round(100 * p.child_tables_done / greatest(p.child_tables_total, 1), 2)::text AS "child_total/done,%", ` +
		"current_child_table_relid::regclass AS child_in_progress " +
		"FROM pg_stat_progress_analyze p INNER JOIN pg_stat_activity a ON p.pid = a.pid " +
		"WHERE a.pid <> pg_backend_pid() ORDER BY a.pid DESC"
)

// SelectStatProgressAnalyzeQuery returns the query, number of columns and diff interval for the
// analyze progress screen, depending on Postgres version.
func SelectStatProgressAnalyzeQuery(version int) (string, int, [2]int) {
	if version >= PostgresV19 {
		return PgStatProgressAnalyzePG19, 13, [2]int{0, 0}
	}
	return PgStatProgressAnalyzeDefault, 12, [2]int{0, 0}
}
