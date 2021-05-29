package query

const (
	// PgStatDatabaseDefault is the default query for getting databases' stats from pg_stat_database view
	PgStatDatabaseDefault = "SELECT datname, numbackends AS backends, " +
		"coalesce(xact_commit, 0) AS commits, coalesce(xact_rollback, 0) AS rollbacks, " +
		"coalesce(blks_read * (SELECT current_setting('block_size')::int / 1024), 0) AS reads, " +
		"coalesce(blks_hit, 0) AS hits, coalesce(tup_returned, 0) AS returned, " +
		"coalesce(tup_fetched, 0) AS fetched, coalesce(tup_inserted, 0) AS inserts, " +
		"coalesce(tup_updated, 0) AS updates, coalesce(tup_deleted, 0) AS deletes, " +
		"coalesce(conflicts, 0) AS conflicts, coalesce(deadlocks, 0) AS deadlocks, " +
		"coalesce(checksum_failures, 0) AS csum_fails, coalesce(temp_files, 0) AS temp_files, " +
		"coalesce(temp_bytes, 0) AS temp_bytes, coalesce(blk_read_time, 0)::numeric(20,2) AS read_t, " +
		"coalesce(blk_write_time, 0)::numeric(20,2) AS write_t, " +
		"date_trunc('seconds', now() - stats_reset)::text AS stats_age " +
		"FROM pg_stat_database ORDER BY datname DESC"

	// PgStatDatabasePG11 is the query for getting databases' stats from pg_stat_database view for versions 11 and older.
	PgStatDatabasePG11 = "SELECT datname, numbackends AS backends, " +
		"coalesce(xact_commit, 0) AS commits, coalesce(xact_rollback, 0) AS rollbacks, " +
		"coalesce(blks_read * (SELECT current_setting('block_size')::int / 1024), 0) AS reads, " +
		"coalesce(blks_hit, 0) AS hits, coalesce(tup_returned, 0) AS returned, " +
		"coalesce(tup_fetched, 0) AS fetched, coalesce(tup_inserted, 0) AS inserts, " +
		"coalesce(tup_updated, 0) AS updates, coalesce(tup_deleted, 0) AS deletes, " +
		"coalesce(conflicts, 0) AS conflicts, coalesce(deadlocks, 0) AS deadlocks, " +
		"coalesce(temp_files, 0) AS temp_files, coalesce(temp_bytes, 0) AS temp_bytes, " +
		"coalesce(blk_read_time, 0)::numeric(20,2) AS read_t, " +
		"coalesce(blk_write_time, 0)::numeric(20,2) AS write_t, " +
		"date_trunc('seconds', now() - stats_reset)::text AS stats_age " +
		"FROM pg_stat_database ORDER BY datname DESC"
)

// SelectStatDatabaseQuery returns proper query, number of columns and diff interval depending on Postgres version.
func SelectStatDatabaseQuery(version int) (string, int, [2]int) {
	switch {
	case version < 120000:
		return PgStatDatabasePG11, 18, [2]int{2, 16}
	default:
		return PgStatDatabaseDefault, 19, [2]int{2, 17}
	}
}
