package query

const (
	// PgStatDatabaseGeneralDefault defines default query for getting general databases' stats from pg_stat_database view.
	PgStatDatabaseGeneralDefault = "SELECT datname, numbackends AS backends_total, " +
		"coalesce(xact_commit, 0) AS commits, coalesce(xact_rollback, 0) AS rollbacks, " +
		"coalesce(blks_read * (SELECT current_setting('block_size')::int / 1024), 0) AS reads, " +
		"coalesce(blks_hit, 0) AS hits, coalesce(tup_returned, 0) AS returned, " +
		"coalesce(tup_fetched, 0) AS fetched, coalesce(tup_inserted, 0) AS inserts, " +
		"coalesce(tup_updated, 0) AS updates, coalesce(tup_deleted, 0) AS deletes, " +
		"coalesce(conflicts, 0) AS conflicts, coalesce(deadlocks, 0) AS deadlocks, " +
		"coalesce(checksum_failures, 0) AS csum_fails, coalesce(temp_files, 0) AS temp_files, " +
		`coalesce(temp_bytes, 0) AS temp_bytes, coalesce(blk_read_time, 0)::numeric(20,2) AS "read,ms", ` +
		`coalesce(blk_write_time, 0)::numeric(20,2) AS "write,ms", ` +
		"date_trunc('seconds', now() - stats_reset)::text AS stats_age " +
		"FROM pg_stat_database ORDER BY datname DESC"

	// PgStatDatabaseGeneralPG11 defines query for getting general databases' stats from pg_stat_database view for versions 11 and older.
	PgStatDatabaseGeneralPG11 = "SELECT datname, numbackends AS backends_total, " +
		"coalesce(xact_commit, 0) AS commits, coalesce(xact_rollback, 0) AS rollbacks, " +
		"coalesce(blks_read * (SELECT current_setting('block_size')::int / 1024), 0) AS reads, " +
		"coalesce(blks_hit, 0) AS hits, coalesce(tup_returned, 0) AS returned, " +
		"coalesce(tup_fetched, 0) AS fetched, coalesce(tup_inserted, 0) AS inserts, " +
		"coalesce(tup_updated, 0) AS updates, coalesce(tup_deleted, 0) AS deletes, " +
		"coalesce(conflicts, 0) AS conflicts, coalesce(deadlocks, 0) AS deadlocks, " +
		"coalesce(temp_files, 0) AS temp_files, coalesce(temp_bytes, 0) AS temp_bytes, " +
		`coalesce(blk_read_time, 0)::numeric(20,2) AS "read,ms", ` +
		`coalesce(blk_write_time, 0)::numeric(20,2) AS "write,ms", ` +
		"date_trunc('seconds', now() - stats_reset)::text AS stats_age " +
		"FROM pg_stat_database ORDER BY datname DESC"

	// PgStatDatabaseSessionsDefault defines query for getting sessions stats from pg_stat_database view (available since Postgres 14).
	PgStatDatabaseSessionsDefault = "SELECT datname, numbackends AS backends_total, " +
		"date_trunc('seconds', session_time / 1000 * '1 second'::interval)::text AS session_total, " +
		"date_trunc('seconds', active_time / 1000 * '1 second'::interval)::text AS active_total, " +
		"date_trunc('seconds', idle_in_transaction_time / 1000 * '1 second'::interval)::text AS idle_xact_total, " +
		`session_time AS "session,ms", ` +
		`active_time AS "active,ms", ` +
		`idle_in_transaction_time AS "idle_xact,ms", ` +
		"sessions, sessions_abandoned AS abandoned, sessions_fatal AS fatal, sessions_killed AS killed " +
		"FROM pg_stat_database"
)

// SelectStatDatabaseGeneralQuery returns proper query, number of columns and diff interval depending on Postgres version.
func SelectStatDatabaseGeneralQuery(version int) (string, int, [2]int) {
	switch {
	case version < 120000:
		return PgStatDatabaseGeneralPG11, 18, [2]int{2, 16}
	default:
		return PgStatDatabaseGeneralDefault, 19, [2]int{2, 17}
	}
}
