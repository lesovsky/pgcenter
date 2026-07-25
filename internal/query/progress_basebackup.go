package query

const (
	// PgStatProgressBasebackupDefault defines default query for getting stats from pg_stat_progress_basebackup view.
	PgStatProgressBasebackupDefault = "SELECT " +
		"a.pid, host(a.client_addr) AS started_from, " +
		"to_char(backend_start, 'YYYY-MM-DD HH24:MI:SS') AS started_at, " +
		"date_trunc('seconds', clock_timestamp() - backend_start)::text AS duration, a.state, " +
		"coalesce((a.wait_event_type ||'.'|| a.wait_event), 'f') AS waiting, p.phase, " +
		`p.backup_total / 1024 AS "size_total,KiB", ` +
		`round(100 * p.backup_streamed / greatest(p.backup_total,1), 2)::text AS "streamed,%", ` +
		`coalesce(p.backup_streamed / 1024, 0) AS "streamed,KiB", ` +
		`p.tablespaces_total||'/'|| p.tablespaces_streamed::text AS "tablespaces_total/streamed" ` +
		"FROM pg_stat_progress_basebackup p INNER JOIN pg_stat_activity a ON p.pid = a.pid " +
		"WHERE a.pid <> pg_backend_pid() ORDER BY a.pid DESC"

	// PgStatProgressBasebackupPG19 defines query for getting stats from pg_stat_progress_basebackup on PG 19 and newer.
	// PG 19 adds backup_type (full/incremental), placed right after duration; this shifts the diffed
	// streamed,KiB column from 9 to 10.
	PgStatProgressBasebackupPG19 = "SELECT " +
		"a.pid, host(a.client_addr) AS started_from, " +
		"to_char(backend_start, 'YYYY-MM-DD HH24:MI:SS') AS started_at, " +
		"date_trunc('seconds', clock_timestamp() - backend_start)::text AS duration, " +
		"p.backup_type, a.state, " +
		"coalesce((a.wait_event_type ||'.'|| a.wait_event), 'f') AS waiting, p.phase, " +
		`p.backup_total / 1024 AS "size_total,KiB", ` +
		`round(100 * p.backup_streamed / greatest(p.backup_total,1), 2)::text AS "streamed,%", ` +
		`coalesce(p.backup_streamed / 1024, 0) AS "streamed,KiB", ` +
		`p.tablespaces_total||'/'|| p.tablespaces_streamed::text AS "tablespaces_total/streamed" ` +
		"FROM pg_stat_progress_basebackup p INNER JOIN pg_stat_activity a ON p.pid = a.pid " +
		"WHERE a.pid <> pg_backend_pid() ORDER BY a.pid DESC"
)

// SelectStatProgressBasebackupQuery returns the query, number of columns and diff interval for the
// basebackup progress screen, depending on Postgres version.
func SelectStatProgressBasebackupQuery(version int) (string, int, [2]int) {
	if version >= PostgresV19 {
		return PgStatProgressBasebackupPG19, 12, [2]int{10, 10}
	}
	return PgStatProgressBasebackupDefault, 11, [2]int{9, 9}
}
