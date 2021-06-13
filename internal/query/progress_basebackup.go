package query

const (
	// PgStatProgressBasebackupDefault defines default query for getting stats from pg_stat_progress_basebackup view.
	PgStatProgressBasebackupDefault = "SELECT " +
		"a.pid, a.client_addr AS started_from, " +
		"to_char(backend_start, 'YYYY-MM-DD HH24:MI:SS') AS started_at, " +
		"date_trunc('seconds', clock_timestamp() - backend_start)::text AS duration, a.state, " +
		"coalesce((a.wait_event_type ||'.'|| a.wait_event), 'f') AS waiting, p.phase, " +
		`p.backup_total / 1024 AS "size_total,KiB", ` +
		`round(100 * p.backup_streamed / greatest(p.backup_total,1), 2)::text AS "streamed,%", ` +
		`coalesce(p.backup_streamed / 1024, 0) AS "streamed,KiB", ` +
		`p.tablespaces_total||'/'|| p.tablespaces_streamed::text AS "tablespaces_total/streamed" ` +
		"FROM pg_stat_progress_basebackup p INNER JOIN pg_stat_activity a ON p.pid = a.pid " +
		"WHERE a.pid <> pg_backend_pid() ORDER BY a.pid DESC"
)
