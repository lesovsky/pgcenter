package query

const (
	// PgStatProgressCopyDefault defines default query for getting stats from pg_stat_progress_copy view.
	PgStatProgressCopyDefault = "SELECT a.pid, date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age, " +
		"p.datname, p.relid::regclass AS relation, a.state, " +
		"coalesce((a.wait_event_type ||'.'|| a.wait_event), 'f') AS waiting, p.command, p.type, " +
		`pg_relation_size(p.relid) / 1024 AS "size_total,KiB", ` +
		`p.bytes_total / 1024 AS "source_total,KiB", p.bytes_processed / 1024 AS "processed,KiB", ` +
		`round(100 * p.bytes_processed / nullif(p.bytes_total, 0), 2)::text AS "processed,%", ` +
		"p.tuples_processed, p.tuples_excluded " +
		"FROM pg_stat_progress_copy p INNER JOIN pg_stat_activity a ON p.pid = a.pid " +
		"WHERE a.pid <> pg_backend_pid() AND NOT EXISTS (SELECT 1 FROM pg_locks WHERE relation = p.relid AND mode = 'AccessExclusiveLock' AND granted) " +
		"ORDER BY a.pid DESC"
)
