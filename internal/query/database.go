package query

const (
	// PgStatDatabaseDefault is the default query for getting databases' stats from pg_stat_database view
	// { Name: "pg_stat_database", Query: common.PgStatDatabaseQueryDefault, DiffIntvl: [2]int{1,16}, Ncols: 18, OrderKey: 0, OrderDesc: true }
	PgStatDatabaseDefault = "SELECT datname, " +
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

	// PgStatDatabase11 is the query for getting databases' stats from pg_stat_database view for versions prior 12
	// { Name: "pg_stat_database", Query: common.PgStatDatabaseQuery11, DiffIntvl: [2]int{1,15}, Ncols: 17, OrderKey: 0, OrderDesc: true }
	PgStatDatabase11 = "SELECT datname, " +
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
