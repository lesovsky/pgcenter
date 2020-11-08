package query

const (
	// Some notes about pg_stat_statements-related queries:
	// 1. regexp_replace() removes extra spaces, tabs and newlines from queries

	// PgStatStatementsTimingQueryDefault is the default query for getting timings stats from pg_stat_statements view
	// { Name: "pg_stat_statements_timing", Query: common.PgStatStatementsTimingQueryDefault, DiffIntvl: [2]int{6,10}, Ncols: 13, OrderKey: 0, OrderDesc: true }
	PgStatStatementsTimingQueryDefault = `SELECT
pg_get_userbyid(p.userid) AS user,
d.datname AS database,
date_trunc('seconds', round(p.total_plan_time + p.total_exec_time) / 1000 * '1 second'::interval)::text AS t_all_t,
date_trunc('seconds', round(p.blk_read_time) / 1000 * '1 second'::interval)::text AS t_read_t,
date_trunc('seconds', round(p.blk_write_time) / 1000 * '1 second'::interval)::text AS t_write_t,
date_trunc('seconds', round((p.total_plan_time + p.total_exec_time) - (p.blk_read_time + p.blk_write_time)) / 1000 * '1 second'::interval)::text AS t_cpu_t,
round(p.total_plan_time + p.total_exec_time) AS all_t,
round(p.blk_read_time) AS read_t,
round(p.blk_write_time) AS write_t,
round((p.total_plan_time + p.total_exec_time) - (p.blk_read_time + p.blk_write_time)) AS cpu_t,
p.calls AS calls,
left(md5(p.dbid::text || p.userid || p.queryid), 10) AS queryid,
regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query
FROM pg_stat_statements p
JOIN pg_database d ON d.oid=p.dbid`

	// pg_stat_statements timing query for versions up to 12
	PgStatStatementsTimingQuery12 = `SELECT
pg_get_userbyid(p.userid) AS user,
d.datname AS database,
date_trunc('seconds', round(p.total_time) / 1000 * '1 second'::interval)::text AS t_all_t,
date_trunc('seconds', round(p.blk_read_time) / 1000 * '1 second'::interval)::text AS t_read_t,
date_trunc('seconds', round(p.blk_write_time) / 1000 * '1 second'::interval)::text AS t_write_t,
date_trunc('seconds', round(p.total_time - (p.blk_read_time + p.blk_write_time)) / 1000 * '1 second'::interval)::text AS t_cpu_t,
round(p.total_time) AS all_t,
round(p.blk_read_time) AS read_t,
round(p.blk_write_time) AS write_t,
round(p.total_time - (p.blk_read_time + p.blk_write_time)) AS cpu_t,
p.calls AS calls,
left(md5(p.dbid::text || p.userid || p.queryid), 10) AS queryid,
regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query
FROM pg_stat_statements p
JOIN pg_database d ON d.oid=p.dbid`

	// PgStatStatementsGeneralQueryDefault is the default query for getting general stats from pg_stat_statements
	// { Name: "pg_stat_statements_general", Query: common.PgStatStatementsGeneralQueryDefault, DiffIntvl: [2]int{4,5}, Ncols: 8, OrderKey: 0, OrderDesc: true }
	PgStatStatementsGeneralQueryDefault = `SELECT
pg_get_userbyid(p.userid) AS user,
d.datname AS database,
p.calls AS t_calls,
p.rows AS t_rows,
p.calls AS calls,
p.rows AS rows,
left(md5(p.dbid::text || p.userid || p.queryid), 10) AS queryid,
regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query
FROM pg_stat_statements p
JOIN pg_database d ON d.oid=p.dbid`

	// PgStatStatementsIoQueryDefault is the default query for getting IO stats from pg_stat_statements
	// { Name: "pg_stat_statements_io", Query: common.PgStatStatementsIoQueryDefault, DiffIntvl: [2]int{6,10}, Ncols: 13, OrderKey: 0, OrderDesc: true }
	PgStatStatementsIoQueryDefault = `SELECT
pg_get_userbyid(p.userid) AS user,
d.datname AS database,
p.shared_blks_hit + p.local_blks_hit AS t_hits,
(p.shared_blks_read + p.local_blks_read) * (SELECT current_setting('block_size')::int / 1024) AS t_reads,
(p.shared_blks_dirtied + p.local_blks_dirtied) * (SELECT current_setting('block_size')::int / 1024) AS t_dirtied,
(p.shared_blks_written + p.local_blks_written) * (SELECT current_setting('block_size')::int / 1024) AS t_written,
p.shared_blks_hit + p.local_blks_hit AS hits,
(p.shared_blks_read + p.local_blks_read) * (SELECT current_setting('block_size')::int / 1024) AS reads,
(p.shared_blks_dirtied + p.local_blks_dirtied) * (SELECT current_setting('block_size')::int / 1024) AS dirtied,
(p.shared_blks_written + p.local_blks_written) * (SELECT current_setting('block_size')::int / 1024) AS written,
p.calls AS calls,
left(md5(p.dbid::text || p.userid || p.queryid), 10) AS queryid,
regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query
FROM pg_stat_statements p
JOIN pg_database d ON d.oid=p.dbid`

	// PgStatStatementsTempQueryDefault is the default query for getting stats about temp files IO from pg_stat_statements
	// { Name: "pg_stat_statements_temp", Query: common.PgStatStatementsTempQueryDefault, DiffIntvl: [2]int{4,6}, Ncols: 9, OrderKey: 0, OrderDesc: true }
	PgStatStatementsTempQueryDefault = `SELECT
pg_get_userbyid(p.userid) AS user,
d.datname AS database,
p.temp_blks_read * (SELECT current_setting('block_size')::int / 1024) AS t_tmp_read,
p.temp_blks_written * (SELECT current_setting('block_size')::int / 1024) AS t_tmp_write,
p.temp_blks_read * (SELECT current_setting('block_size')::int / 1024) AS tmp_read,
p.temp_blks_written * (SELECT current_setting('block_size')::int / 1024) AS tmp_write,
p.calls AS calls,
left(md5(p.dbid::text || p.userid || p.queryid), 10) AS queryid,
regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query
FROM pg_stat_statements p
JOIN pg_database d ON d.oid=p.dbid`

	// PgStatStatementsLocalQueryDefault is the default query for getting stats about local buffers IO from pg_stat_statements
	// { Name: "pg_stat_statements_local", Query: common.PgStatStatementsLocalQueryDefault, DiffIntvl: [2]int{6,10}, Ncols: 13, OrderKey: 0, OrderDesc: true }
	PgStatStatementsLocalQueryDefault = `SELECT
pg_get_userbyid(p.userid) AS user,
d.datname AS database,
p.local_blks_hit AS t_lo_hits,
p.local_blks_read * (SELECT current_setting('block_size')::int / 1024) AS t_lo_reads,
p.local_blks_dirtied * (SELECT current_setting('block_size')::int / 1024) AS t_lo_dirtied,
p.local_blks_written * (SELECT current_setting('block_size')::int / 1024) AS t_lo_written,
p.local_blks_hit AS lo_hits,
p.local_blks_read * (SELECT current_setting('block_size')::int / 1024) AS lo_reads,
p.local_blks_dirtied * (SELECT current_setting('block_size')::int / 1024) AS lo_dirtied,
p.local_blks_written * (SELECT current_setting('block_size')::int / 1024) AS lo_written,
p.calls AS calls,
left(md5(p.dbid::text || p.userid || p.queryid), 10) AS queryid,
regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query
FROM pg_stat_statements p
JOIN pg_database d ON d.oid=p.dbid`
)
