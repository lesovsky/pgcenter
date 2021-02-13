package query

const (
	// NOTES:
	// 1. regexp_replace() removes extra spaces, tabs and newlines from queries

	// PgStatStatementsTimingDefault is the default query for getting timings stats from pg_stat_statements view
	// { Name: "pg_stat_statements_timing", Query: common.PgStatStatementsTimingQueryDefault, DiffIntvl: [2]int{6,10}, Ncols: 13, OrderKey: 0, OrderDesc: true }
	PgStatStatementsTimingDefault = "SELECT pg_get_userbyid(p.userid) AS user, d.datname AS database, " +
		"date_trunc('seconds', round(p.total_plan_time + p.total_exec_time) / 1000 * '1 second'::interval)::text AS t_all_t, " +
		"date_trunc('seconds', round(p.blk_read_time) / 1000 * '1 second'::interval)::text AS t_read_t, " +
		"date_trunc('seconds', round(p.blk_write_time) / 1000 * '1 second'::interval)::text AS t_write_t, " +
		"date_trunc('seconds', round((p.total_plan_time + p.total_exec_time) - (p.blk_read_time + p.blk_write_time)) / 1000 * '1 second'::interval)::text AS t_cpu_t, " +
		"round(p.total_plan_time + p.total_exec_time) AS all_t, " +
		"round(p.blk_read_time) AS read_t, round(p.blk_write_time) AS write_t, " +
		"round((p.total_plan_time + p.total_exec_time) - (p.blk_read_time + p.blk_write_time)) AS cpu_t, " +
		"p.calls AS calls, left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid, " +
		`regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query ` +
		"FROM pg_stat_statements p JOIN pg_database d ON d.oid=p.dbid"

	// pg_stat_statements timing query for Postgres 12 and older.
	PgStatStatementsTimingPG12 = "SELECT pg_get_userbyid(p.userid) AS user, d.datname AS database, " +
		"date_trunc('seconds', round(p.total_time) / 1000 * '1 second'::interval)::text AS t_all_t, " +
		"date_trunc('seconds', round(p.blk_read_time) / 1000 * '1 second'::interval)::text AS t_read_t, " +
		"date_trunc('seconds', round(p.blk_write_time) / 1000 * '1 second'::interval)::text AS t_write_t, " +
		"date_trunc('seconds', round(p.total_time - (p.blk_read_time + p.blk_write_time)) / 1000 * '1 second'::interval)::text AS t_cpu_t, " +
		"round(p.total_time) AS all_t, round(p.blk_read_time) AS read_t, round(p.blk_write_time) AS write_t, " +
		"round(p.total_time - (p.blk_read_time + p.blk_write_time)) AS cpu_t, p.calls AS calls, " +
		"left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid, " +
		`regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query ` +
		"FROM pg_stat_statements p JOIN pg_database d ON d.oid=p.dbid"

	// PgStatStatementsGeneralDefault is the default query for getting general stats from pg_stat_statements
	// { Name: "pg_stat_statements_general", Query: common.PgStatStatementsGeneralQueryDefault, DiffIntvl: [2]int{4,5}, Ncols: 8, OrderKey: 0, OrderDesc: true }
	PgStatStatementsGeneralDefault = "SELECT pg_get_userbyid(p.userid) AS user, d.datname AS database, p.calls AS t_calls, " +
		"p.rows AS t_rows, p.calls AS calls, p.rows AS rows, left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid, " +
		`regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query ` +
		"FROM pg_stat_statements p JOIN pg_database d ON d.oid=p.dbid"

	// PgStatStatementsIoDefault is the default query for getting IO stats from pg_stat_statements
	// { Name: "pg_stat_statements_io", Query: common.PgStatStatementsIoQueryDefault, DiffIntvl: [2]int{6,10}, Ncols: 13, OrderKey: 0, OrderDesc: true }
	PgStatStatementsIoDefault = "SELECT pg_get_userbyid(p.userid) AS user, d.datname AS database, " +
		"p.shared_blks_hit + p.local_blks_hit AS t_hits, " +
		"(p.shared_blks_read + p.local_blks_read) * (SELECT current_setting('block_size')::int / 1024) AS t_reads, " +
		"(p.shared_blks_dirtied + p.local_blks_dirtied) * (SELECT current_setting('block_size')::int / 1024) AS t_dirtied, " +
		"(p.shared_blks_written + p.local_blks_written) * (SELECT current_setting('block_size')::int / 1024) AS t_written, " +
		"p.shared_blks_hit + p.local_blks_hit AS hits, " +
		"(p.shared_blks_read + p.local_blks_read) * (SELECT current_setting('block_size')::int / 1024) AS reads, " +
		"(p.shared_blks_dirtied + p.local_blks_dirtied) * (SELECT current_setting('block_size')::int / 1024) AS dirtied, " +
		"(p.shared_blks_written + p.local_blks_written) * (SELECT current_setting('block_size')::int / 1024) AS written, " +
		"p.calls AS calls, left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid, " +
		`regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query ` +
		"FROM pg_stat_statements p JOIN pg_database d ON d.oid=p.dbid"

	// PgStatStatementsTempDefault is the default query for getting stats about temp files IO from pg_stat_statements
	// { Name: "pg_stat_statements_temp", Query: common.PgStatStatementsTempQueryDefault, DiffIntvl: [2]int{4,6}, Ncols: 9, OrderKey: 0, OrderDesc: true }
	PgStatStatementsTempDefault = "SELECT pg_get_userbyid(p.userid) AS user, d.datname AS database, " +
		"p.temp_blks_read * (SELECT current_setting('block_size')::int / 1024) AS t_tmp_read, " +
		"p.temp_blks_written * (SELECT current_setting('block_size')::int / 1024) AS t_tmp_write, " +
		"p.temp_blks_read * (SELECT current_setting('block_size')::int / 1024) AS tmp_read, " +
		"p.temp_blks_written * (SELECT current_setting('block_size')::int / 1024) AS tmp_write, " +
		"p.calls AS calls, left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid, " +
		`regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query ` +
		"FROM pg_stat_statements p JOIN pg_database d ON d.oid=p.dbid"

	// PgStatStatementsLocalDefault is the default query for getting stats about local buffers IO from pg_stat_statements
	// { Name: "pg_stat_statements_local", Query: common.PgStatStatementsLocalQueryDefault, DiffIntvl: [2]int{6,10}, Ncols: 13, OrderKey: 0, OrderDesc: true }
	PgStatStatementsLocalDefault = "SELECT pg_get_userbyid(p.userid) AS user, d.datname AS database, " +
		"p.local_blks_hit AS t_lo_hits, p.local_blks_read * (SELECT current_setting('block_size')::int / 1024) AS t_lo_reads, " +
		"p.local_blks_dirtied * (SELECT current_setting('block_size')::int / 1024) AS t_lo_dirtied, " +
		"p.local_blks_written * (SELECT current_setting('block_size')::int / 1024) AS t_lo_written, " +
		"p.local_blks_hit AS lo_hits, " +
		"p.local_blks_read * (SELECT current_setting('block_size')::int / 1024) AS lo_reads, " +
		"p.local_blks_dirtied * (SELECT current_setting('block_size')::int / 1024) AS lo_dirtied, " +
		"p.local_blks_written * (SELECT current_setting('block_size')::int / 1024) AS lo_written, " +
		"p.calls AS calls, left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid, " +
		`regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query ` +
		"FROM pg_stat_statements p JOIN pg_database d ON d.oid=p.dbid"

	// PgStatStatementsReportQuery defines query used for calculating per-statement report based on pg_stat_statements.
	PgStatStatementsReportQueryDefault = "WITH totals AS (SELECT " +
		"sum(calls) AS total_calls," +
		"sum(rows) AS total_rows," +
		"sum(total_plan_time + total_exec_time) AS total_all_time," +
		"sum(total_plan_time) AS total_plan_time," +
		"sum(total_exec_time - blk_read_time + blk_write_time) AS total_cpu_time," +
		"sum(blk_read_time + blk_write_time) AS total_io_time " +
		"FROM pg_stat_statements)," +
		"stmt AS (" +
		"SELECT " +
		"query, queryid, userid, dbid," +
		"calls AS calls," +
		"rows AS rows," +
		"total_plan_time + total_exec_time AS all_time," +
		"total_plan_time AS plan_time," +
		"total_exec_time - blk_read_time + blk_write_time AS cpu_time," +
		"blk_read_time + blk_write_time AS io_time " +
		"FROM pg_stat_statements " +
		"WHERE left(md5(userid::text || dbid::text || queryid::text), 10) = $1) " +
		"SELECT s.query, s.queryid::text AS queryid, s.userid::regrole AS usename, d.datname," +
		"to_char((SELECT total_calls FROM totals), 'FM999,999,999,990') AS total_calls," +
		"to_char((SELECT total_rows FROM totals), 'FM999,999,999,990') AS total_rows," +
		"to_char(interval '1 millisecond' * (SELECT total_all_time FROM totals), 'HH24:MI:SS') AS total_all_time," +
		"to_char(interval '1 millisecond' * (SELECT coalesce(nullif(total_plan_time, 0), 1) FROM totals), 'HH24:MI:SS') AS total_plan_time," +
		"to_char(100 * (SELECT total_plan_time FROM totals) / (SELECT coalesce(nullif(total_all_time, 0), 1) FROM totals), 'FM990.00') AS total_plan_time_dist_ratio," +
		"to_char(interval '1 millisecond' * (SELECT total_cpu_time FROM totals), 'HH24:MI:SS') AS total_cpu_time," +
		"to_char(100 * (SELECT total_cpu_time FROM totals) / (SELECT coalesce(nullif(total_all_time, 0), 1) FROM totals), 'FM990.00') AS total_cpu_time_dist_ratio," +
		"to_char(interval '1 millisecond' * (SELECT total_io_time FROM totals), 'HH24:MI:SS') AS total_io_time," +
		"to_char(100 * (SELECT total_io_time FROM totals) / (SELECT coalesce(nullif(total_all_time, 0), 1) FROM totals), 'FM990.00') AS total_io_time_dist_ratio," +
		"to_char(s.calls, 'FM999,999,999,990') AS calls," +
		"to_char(100*s.calls/(SELECT total_calls FROM totals), 'FM990.00') AS calls_ratio," +
		"to_char(s.rows, 'FM999,999,999,990') AS rows," +
		"to_char(100*s.rows/(SELECT coalesce(nullif(total_rows, 0), 1) FROM totals), 'FM990.00') AS rows_ratio," +
		"to_char(interval '1 millisecond' * s.all_time, 'HH24:MI:SS.MS') AS all_time," +
		"to_char(100*s.all_time/(SELECT coalesce(nullif(total_all_time, 0), 1) FROM totals), 'FM990.00') AS all_time_ratio," +
		"to_char(interval '1 millisecond' * s.plan_time, 'HH24:MI:SS.MS') AS plan_time," +
		"to_char(100*s.plan_time/(SELECT coalesce(nullif(total_plan_time, 0), 1) FROM totals), 'FM990.00') AS plan_time_ratio," +
		"to_char(interval '1 millisecond' * s.cpu_time, 'HH24:MI:SS.MS') AS cpu_time," +
		"to_char(100*s.cpu_time/(SELECT coalesce(nullif(total_cpu_time, 0), 1) FROM totals), 'FM990.00') AS cpu_time_ratio," +
		"to_char(interval '1 millisecond' * s.io_time, 'HH24:MI:SS.MS') AS io_time," +
		"to_char(100*s.io_time/(SELECT coalesce(nullif(total_io_time, 0), 1) FROM totals), 'FM990.00') AS io_time_ratio," +
		"(s.all_time / s.calls)::numeric(20,2) AS avg_all_time," +
		"(s.plan_time / s.calls)::numeric(20,2) AS avg_plan_time," +
		"(s.cpu_time / s.calls)::numeric(20,2) AS avg_cpu_time," +
		"(s.io_time / s.calls)::numeric(20,2) AS avg_io_time," +
		"to_char(100*s.plan_time / s.all_time, 'FM990.00') AS plan_time_dist_ratio," +
		"to_char(100*s.cpu_time / s.all_time, 'FM990.00') AS cpu_time_dist_ratio," +
		"to_char(100*s.io_time / s.all_time, 'FM990.00') AS io_time_dist_ratio " +
		"FROM stmt s JOIN pg_database d ON d.oid=s.dbid LIMIT 1"

	// PgStatStatementsReportQueryPG12 defines query used for calculating per-statement report based on pg_stat_statements for postgres 12 and earlier.
	PgStatStatementsReportQueryPG12 = "WITH totals AS (SELECT " +
		"sum(calls) AS total_calls," +
		"sum(rows) AS total_rows," +
		"sum(total_time) AS total_all_time," +
		"0 AS total_plan_time," +
		"sum(total_time - blk_read_time + blk_write_time) AS total_cpu_time," +
		"sum(blk_read_time+blk_write_time) AS total_io_time " +
		"FROM pg_stat_statements)," +
		"stmt AS (" +
		"SELECT " +
		"query, queryid, userid, dbid," +
		"calls AS calls," +
		"rows AS rows," +
		"total_time AS all_time," +
		"0 AS plan_time," +
		"total_time - blk_read_time + blk_write_time AS cpu_time," +
		"blk_read_time + blk_write_time AS io_time " +
		"FROM pg_stat_statements " +
		"WHERE left(md5(userid::text || dbid::text || queryid::text), 10) = $1) " +
		"SELECT s.query, s.queryid::text AS queryid, s.userid::regrole AS usename, d.datname," +
		"to_char((SELECT total_calls FROM totals), 'FM999,999,999,990') AS total_calls," +
		"to_char((SELECT total_rows FROM totals), 'FM999,999,999,990') AS total_rows," +
		"to_char(interval '1 millisecond' * (SELECT total_all_time FROM totals), 'HH24:MI:SS') AS total_all_time," +
		"to_char(interval '1 millisecond' * (SELECT total_plan_time FROM totals), 'HH24:MI:SS') AS total_plan_time," +
		"to_char(100 * (SELECT total_plan_time FROM totals) / (SELECT coalesce(nullif(total_all_time, 0), 1) FROM totals), 'FM990.00') AS total_plan_time_dist_ratio," +
		"to_char(interval '1 millisecond' * (SELECT total_cpu_time FROM totals), 'HH24:MI:SS') AS total_cpu_time," +
		"to_char(100 * (SELECT total_cpu_time FROM totals) / (SELECT coalesce(nullif(total_all_time, 0), 1) FROM totals), 'FM990.00') AS total_cpu_time_dist_ratio," +
		"to_char(interval '1 millisecond' * (SELECT total_io_time FROM totals), 'HH24:MI:SS') AS total_io_time," +
		"to_char(100 * (SELECT total_io_time FROM totals) / (SELECT coalesce(nullif(total_all_time, 0), 1) FROM totals), 'FM990.00') AS total_io_time_dist_ratio," +
		"to_char(s.calls, 'FM999,999,999,990') AS calls," +
		"to_char(100*s.calls/(SELECT total_calls FROM totals), 'FM990.00') AS calls_ratio," +
		"to_char(s.rows, 'FM999,999,999,990') AS rows," +
		"to_char(100*s.rows/(SELECT coalesce(nullif(total_rows, 0), 1) FROM totals), 'FM990.00') AS rows_ratio," +
		"to_char(interval '1 millisecond' * s.all_time, 'HH24:MI:SS.MS') AS all_time," +
		"to_char(100*s.all_time/(SELECT coalesce(nullif(total_all_time, 0), 1) FROM totals), 'FM990.00') AS all_time_ratio," +
		"to_char(interval '1 millisecond' * s.plan_time, 'HH24:MI:SS.MS') AS plan_time," +
		"to_char(100*s.plan_time/(SELECT coalesce(nullif(total_plan_time, 0), 1) FROM totals), 'FM990.00') AS plan_time_ratio," +
		"to_char(interval '1 millisecond' * s.cpu_time, 'HH24:MI:SS.MS') AS cpu_time," +
		"to_char(100*s.cpu_time/(SELECT coalesce(nullif(total_cpu_time, 0), 1) FROM totals), 'FM990.00') AS cpu_time_ratio," +
		"to_char(interval '1 millisecond' * s.io_time, 'HH24:MI:SS.MS') AS io_time," +
		"to_char(100*s.io_time/(SELECT coalesce(nullif(total_io_time, 0), 1) FROM totals), 'FM990.00') AS io_time_ratio," +
		"(s.all_time / s.calls)::numeric(20,2) AS avg_all_time," +
		"(s.plan_time / s.calls)::numeric(20,2) AS avg_plan_time," +
		"(s.cpu_time / s.calls)::numeric(20,2) AS avg_cpu_time," +
		"(s.io_time / s.calls)::numeric(20,2) AS avg_io_time," +
		"to_char(100*s.plan_time / s.all_time, 'FM990.00') AS plan_time_dist_ratio," +
		"to_char(100*s.cpu_time / s.all_time, 'FM990.00') AS cpu_time_dist_ratio," +
		"to_char(100*s.io_time / s.all_time, 'FM990.00') AS io_time_dist_ratio " +
		"FROM stmt s JOIN pg_database d ON d.oid=s.dbid LIMIT 1"
)

// SelectStatStatementsTimingQuery returns proper statements_timing query depending on Postgres version.
func SelectStatStatementsTimingQuery(version int) string {
	switch {
	case version < 130000:
		return PgStatStatementsTimingPG12
	default:
		return PgStatStatementsTimingDefault
	}
}

// SelectQueryReportQuery returns proper query report query depending on Postgres version.
func SelectQueryReportQuery(version int) string {
	switch {
	case version < 130000:
		return PgStatStatementsReportQueryPG12
	default:
		return PgStatStatementsReportQueryDefault
	}
}
