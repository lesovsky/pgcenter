package query

const (
	// NOTES:
	// 1. regexp_replace() removes extra spaces, tabs and newlines from queries

	// PgStatStatementsTimingPG13 defines timing query for pg_stat_statements (PG 13-16).
	PgStatStatementsTimingPG13 = "SELECT pg_get_userbyid(p.userid) AS user, d.datname AS database, " +
		"date_trunc('seconds', round(p.total_plan_time + p.total_exec_time) / 1000 * '1 second'::interval)::text AS all_total, " +
		"date_trunc('seconds', round(p.blk_read_time) / 1000 * '1 second'::interval)::text AS read_total, " +
		"date_trunc('seconds', round(p.blk_write_time) / 1000 * '1 second'::interval)::text AS write_total, " +
		"date_trunc('seconds', round((p.total_plan_time + p.total_exec_time) - (p.blk_read_time + p.blk_write_time)) / 1000 * '1 second'::interval)::text AS exec_total, " +
		`round(p.total_plan_time + p.total_exec_time) AS "all,ms", ` +
		`round(p.blk_read_time) AS "read,ms", round(p.blk_write_time) AS "write,ms", ` +
		`round((p.total_plan_time + p.total_exec_time) - (p.blk_read_time + p.blk_write_time)) AS "exec,ms",` +
		"p.calls AS calls, left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid, " +
		`regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query ` +
		"FROM {{.PGSSSchema}}.pg_stat_statements p JOIN pg_database d ON d.oid=p.dbid"

	// PgStatStatementsTimingPG12 defines pg_stat_statements timing query for Postgres 12 and older.
	PgStatStatementsTimingPG12 = "SELECT pg_get_userbyid(p.userid) AS user, d.datname AS database, " +
		"date_trunc('seconds', round(p.total_time) / 1000 * '1 second'::interval)::text AS all_total, " +
		"date_trunc('seconds', round(p.blk_read_time) / 1000 * '1 second'::interval)::text AS read_total, " +
		"date_trunc('seconds', round(p.blk_write_time) / 1000 * '1 second'::interval)::text AS write_total, " +
		"date_trunc('seconds', round(p.total_time - (p.blk_read_time + p.blk_write_time)) / 1000 * '1 second'::interval)::text AS exec_total, " +
		`round(p.total_time) AS "all,ms", round(p.blk_read_time) AS "read,ms", round(p.blk_write_time) AS "write,ms", ` +
		`round(p.total_time - (p.blk_read_time + p.blk_write_time)) AS "exec,ms", p.calls AS calls, ` +
		"left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid, " +
		`regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query ` +
		"FROM {{.PGSSSchema}}.pg_stat_statements p JOIN pg_database d ON d.oid=p.dbid"

	// PgStatStatementsTimingDefault defines timing query for pg_stat_statements (PG 17+).
	// PG 17 splits read/write timing into shared_blk_*, local_blk_*, temp_blk_* columns.
	PgStatStatementsTimingDefault = `
SELECT pg_get_userbyid(p.userid) AS user, d.datname AS database,
	date_trunc('seconds', round(p.total_exec_time + p.total_plan_time) / 1000 * '1 second'::interval)::text AS all_total,
	date_trunc('seconds', round(p.shared_blk_read_time + p.local_blk_read_time + p.temp_blk_read_time) / 1000 * '1 second'::interval)::text AS read_total,
	date_trunc('seconds', round(p.shared_blk_write_time + p.local_blk_write_time + p.temp_blk_write_time) / 1000 * '1 second'::interval)::text AS write_total,
	date_trunc('seconds', round(p.total_exec_time - (p.shared_blk_read_time + p.local_blk_read_time + p.temp_blk_read_time + p.shared_blk_write_time + p.local_blk_write_time + p.temp_blk_write_time)) / 1000 * '1 second'::interval)::text AS exec_total,
	round(p.total_exec_time + p.total_plan_time) AS "all,ms",
	round(p.shared_blk_read_time + p.local_blk_read_time + p.temp_blk_read_time) AS "read,ms",
	round(p.shared_blk_write_time + p.local_blk_write_time + p.temp_blk_write_time) AS "write,ms",
	round(p.total_exec_time - (p.shared_blk_read_time + p.local_blk_read_time + p.temp_blk_read_time + p.shared_blk_write_time + p.local_blk_write_time + p.temp_blk_write_time)) AS "exec,ms",
	p.calls AS calls, left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid,
	regexp_replace({{.PgSSQueryLenFn}}, '\s+', ' ', 'g') AS query
FROM {{.PGSSSchema}}.pg_stat_statements p JOIN pg_database d ON d.oid=p.dbid
`

	// PgStatStatementsGeneralDefault defines default query for getting general stats from pg_stat_statements.
	PgStatStatementsGeneralDefault = "SELECT pg_get_userbyid(p.userid) AS user, d.datname AS database, p.calls AS calls_total, " +
		"p.rows AS rows_total, p.calls AS calls, p.rows AS rows, left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid, " +
		`regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query ` +
		"FROM {{.PGSSSchema}}.pg_stat_statements p JOIN pg_database d ON d.oid=p.dbid"

	// PgStatStatementsIoDefault defines default query for getting IO stats from pg_stat_statements.
	PgStatStatementsIoDefault = "SELECT pg_get_userbyid(p.userid) AS user, d.datname AS database, " +
		"p.shared_blks_hit + p.local_blks_hit AS hits_total, " +
		`(p.shared_blks_read + p.local_blks_read) * (SELECT current_setting('block_size')::int / 1024) AS "read_total,KiB", ` +
		`(p.shared_blks_dirtied + p.local_blks_dirtied) * (SELECT current_setting('block_size')::int / 1024) AS "dirtied_total,KiB", ` +
		`(p.shared_blks_written + p.local_blks_written) * (SELECT current_setting('block_size')::int / 1024) AS "written_total,KiB", ` +
		"p.shared_blks_hit + p.local_blks_hit AS hits, " +
		`(p.shared_blks_read + p.local_blks_read) * (SELECT current_setting('block_size')::int / 1024) AS "read,KiB", ` +
		`(p.shared_blks_dirtied + p.local_blks_dirtied) * (SELECT current_setting('block_size')::int / 1024) AS "dirtied,KiB", ` +
		`(p.shared_blks_written + p.local_blks_written) * (SELECT current_setting('block_size')::int / 1024) AS "written,KiB", ` +
		"p.calls AS calls, left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid, " +
		`regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query ` +
		"FROM {{.PGSSSchema}}.pg_stat_statements p JOIN pg_database d ON d.oid=p.dbid"

	// PgStatStatementsTempDefault defines default query for getting stats about temp files IO from pg_stat_statements.
	PgStatStatementsTempDefault = "SELECT pg_get_userbyid(p.userid) AS user, d.datname AS database, " +
		`p.temp_blks_read * (SELECT current_setting('block_size')::int / 1024) AS "read_total,kiB", ` +
		`p.temp_blks_written * (SELECT current_setting('block_size')::int / 1024) AS "write_total,KiB", ` +
		`p.temp_blks_read * (SELECT current_setting('block_size')::int / 1024) AS "read,KiB", ` +
		`p.temp_blks_written * (SELECT current_setting('block_size')::int / 1024) AS "write,KiB", ` +
		"p.calls AS calls, left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid, " +
		`regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query ` +
		"FROM {{.PGSSSchema}}.pg_stat_statements p JOIN pg_database d ON d.oid=p.dbid"

	// PgStatStatementsLocalDefault defines default query for getting stats about local buffers IO from pg_stat_statements.
	PgStatStatementsLocalDefault = "SELECT pg_get_userbyid(p.userid) AS user, d.datname AS database, " +
		`p.local_blks_hit AS hits_total, p.local_blks_read * (SELECT current_setting('block_size')::int / 1024) AS "read_total,KiB", ` +
		`p.local_blks_dirtied * (SELECT current_setting('block_size')::int / 1024) AS "dirtied_total,KiB", ` +
		`p.local_blks_written * (SELECT current_setting('block_size')::int / 1024) AS "written,KiB", ` +
		"p.local_blks_hit AS hits, " +
		`p.local_blks_read * (SELECT current_setting('block_size')::int / 1024) AS "read,KiB", ` +
		`p.local_blks_dirtied * (SELECT current_setting('block_size')::int / 1024) AS "dirtied,KiB", ` +
		`p.local_blks_written * (SELECT current_setting('block_size')::int / 1024) AS "written,KiB", ` +
		"p.calls AS calls, left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid, " +
		`regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query ` +
		"FROM {{.PGSSSchema}}.pg_stat_statements p JOIN pg_database d ON d.oid=p.dbid"

	// PgStatStatementsWalDefault defines default query for getting stats about WAL usage from pg_stat_statements.
	PgStatStatementsWalDefault = "SELECT pg_get_userbyid(p.userid) AS user, d.datname AS database," +
		`(wal_bytes / 1024)::numeric(20,2)::text AS "wal_total,KiB", (wal_bytes / 1024)::numeric(20,2)::text AS "wal,KiB",` +
		"wal_records AS records, wal_fpi AS fpi, p.calls AS calls," +
		"left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid," +
		`regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query ` +
		"FROM {{.PGSSSchema}}.pg_stat_statements p JOIN pg_database d ON d.oid=p.dbid"

	// PgStatStatementsReportQueryPG13 defines report query for pg_stat_statements (PG 13-16).
	PgStatStatementsReportQueryPG13 = "WITH totals AS (SELECT " +
		"sum(calls) AS total_calls," +
		"sum(rows) AS total_rows," +
		"sum(total_plan_time + total_exec_time) AS total_all_time," +
		"sum(total_plan_time) AS total_plan_time," +
		"sum(total_exec_time - blk_read_time + blk_write_time) AS total_cpu_time," +
		"sum(blk_read_time + blk_write_time) AS total_io_time," +
		"sum(wal_records) AS total_wal_records," +
		"sum(wal_fpi) AS total_wal_fpi," +
		"sum(wal_bytes) AS total_wal_bytes " +
		"FROM {{.PGSSSchema}}.pg_stat_statements)," +
		"stmt AS (" +
		"SELECT " +
		"query, queryid, userid, dbid," +
		"calls AS calls," +
		"rows AS rows," +
		"total_plan_time + total_exec_time AS all_time," +
		"total_plan_time AS plan_time," +
		"total_exec_time - blk_read_time + blk_write_time AS cpu_time," +
		"wal_records, wal_fpi, wal_bytes," +
		"blk_read_time + blk_write_time AS io_time " +
		"FROM {{.PGSSSchema}}.pg_stat_statements " +
		"WHERE left(md5(userid::text || dbid::text || queryid::text), 10) = $1) " +
		"SELECT s.query, s.queryid::text AS queryid, s.userid::regrole AS usename, d.datname," +
		"to_char((SELECT total_calls FROM totals), 'FM999,999,999,990') AS total_calls," +
		"to_char((SELECT total_rows FROM totals), 'FM999,999,999,990') AS total_rows," +
		"pg_size_pretty((SELECT total_wal_bytes FROM totals)::bigint) AS total_wal_bytes," +
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
		"to_char(100*s.io_time / s.all_time, 'FM990.00') AS io_time_dist_ratio," +
		"to_char(s.wal_records, 'FM999,999,999,990') AS wal_records," +
		"to_char(100*s.wal_records/(SELECT coalesce(nullif(total_wal_records, 0), 1) FROM totals), 'FM990.00') AS wal_records_ratio," +
		"to_char(s.wal_fpi, 'FM999,999,999,990') AS wal_fpi," +
		"to_char(100*s.wal_fpi/(SELECT coalesce(nullif(total_wal_fpi, 0), 1) FROM totals), 'FM990.00') AS wal_fpi_ratio," +
		"pg_size_pretty(s.wal_bytes::bigint) AS wal_bytes," +
		"to_char(100*s.wal_bytes/(SELECT coalesce(nullif(total_wal_bytes, 0), 1) FROM totals), 'FM990.00') AS wal_bytes_ratio " +
		"FROM stmt s JOIN pg_database d ON d.oid=s.dbid LIMIT 1"

	// PgStatStatementsReportQueryDefault defines report query for pg_stat_statements (PG 17+).
	// PG 17 splits read/write timing into shared_blk_*, local_blk_*, temp_blk_* columns.
	PgStatStatementsReportQueryDefault = `
WITH totals AS (
	SELECT
		sum(calls) AS total_calls,
		sum(rows) AS total_rows,
		sum(total_plan_time + total_exec_time) AS total_all_time,
		sum(total_plan_time) AS total_plan_time,
		sum(total_exec_time - (
			(shared_blk_read_time + local_blk_read_time + temp_blk_read_time)
			+ (shared_blk_write_time + local_blk_write_time + temp_blk_write_time)
		)) AS total_cpu_time,
		sum((shared_blk_read_time + local_blk_read_time + temp_blk_read_time)
			+ (shared_blk_write_time + local_blk_write_time + temp_blk_write_time)
		) AS total_io_time,
		sum(wal_records) AS total_wal_records,
		sum(wal_fpi) AS total_wal_fpi,
		sum(wal_bytes) AS total_wal_bytes
	FROM {{.PGSSSchema}}.pg_stat_statements
),
stmt AS (
	SELECT
		query, queryid, userid, dbid,
		calls AS calls,
		rows AS rows,
		total_plan_time + total_exec_time AS all_time,
		total_plan_time AS plan_time,
		total_exec_time - (
			(shared_blk_read_time + local_blk_read_time + temp_blk_read_time)
			+ (shared_blk_write_time + local_blk_write_time + temp_blk_write_time)
		) AS cpu_time,
		wal_records, wal_fpi, wal_bytes,
		(shared_blk_read_time + local_blk_read_time + temp_blk_read_time)
			+ (shared_blk_write_time + local_blk_write_time + temp_blk_write_time) as io_time
	FROM {{.PGSSSchema}}.pg_stat_statements
		WHERE left(md5(userid::text || dbid::text || queryid::text), 10) = $1
)

SELECT s.query, s.queryid::text AS queryid, s.userid::regrole AS usename, d.datname,
	to_char((SELECT total_calls FROM totals), 'FM999,999,999,990') AS total_calls,
	to_char((SELECT total_rows FROM totals), 'FM999,999,999,990') AS total_rows,
	pg_size_pretty((SELECT total_wal_bytes FROM totals)::bigint) AS total_wal_bytes,
	to_char(interval '1 millisecond' * (SELECT total_all_time FROM totals), 'HH24:MI:SS') AS total_all_time,
	to_char(interval '1 millisecond' * (SELECT coalesce(nullif(total_plan_time, 0), 1) FROM totals), 'HH24:MI:SS') AS total_plan_time,
	to_char(100 * (SELECT total_plan_time FROM totals) / (SELECT coalesce(nullif(total_all_time, 0), 1) FROM totals), 'FM990.00') AS total_plan_time_dist_ratio,
	to_char(interval '1 millisecond' * (SELECT total_cpu_time FROM totals), 'HH24:MI:SS') AS total_cpu_time,
	to_char(100 * (SELECT total_cpu_time FROM totals) / (SELECT coalesce(nullif(total_all_time, 0), 1) FROM totals), 'FM990.00') AS total_cpu_time_dist_ratio,
	to_char(interval '1 millisecond' * (SELECT total_io_time FROM totals), 'HH24:MI:SS') AS total_io_time,
	to_char(100 * (SELECT total_io_time FROM totals) / (SELECT coalesce(nullif(total_all_time, 0), 1) FROM totals), 'FM990.00') AS total_io_time_dist_ratio,
	to_char(s.calls, 'FM999,999,999,990') AS calls,
	to_char(100*s.calls/(SELECT total_calls FROM totals), 'FM990.00') AS calls_ratio,
	to_char(s.rows, 'FM999,999,999,990') AS rows,
	to_char(100*s.rows/(SELECT coalesce(nullif(total_rows, 0), 1) FROM totals), 'FM990.00') AS rows_ratio,
	to_char(interval '1 millisecond' * s.all_time, 'HH24:MI:SS.MS') AS all_time,
	to_char(100*s.all_time/(SELECT coalesce(nullif(total_all_time, 0), 1) FROM totals), 'FM990.00') AS all_time_ratio,
	to_char(interval '1 millisecond' * s.plan_time, 'HH24:MI:SS.MS') AS plan_time,
	to_char(100*s.plan_time/(SELECT coalesce(nullif(total_plan_time, 0), 1) FROM totals), 'FM990.00') AS plan_time_ratio,
	to_char(interval '1 millisecond' * s.cpu_time, 'HH24:MI:SS.MS') AS cpu_time,
	to_char(100*s.cpu_time/(SELECT coalesce(nullif(total_cpu_time, 0), 1) FROM totals), 'FM990.00') AS cpu_time_ratio,
	to_char(interval '1 millisecond' * s.io_time, 'HH24:MI:SS.MS') AS io_time,
	to_char(100*s.io_time/(SELECT coalesce(nullif(total_io_time, 0), 1) FROM totals), 'FM990.00') AS io_time_ratio,
	(s.all_time / s.calls)::numeric(20,2) AS avg_all_time,
	(s.plan_time / s.calls)::numeric(20,2) AS avg_plan_time,
	(s.cpu_time / s.calls)::numeric(20,2) AS avg_cpu_time,
	(s.io_time / s.calls)::numeric(20,2) AS avg_io_time,
	to_char(100*s.plan_time / s.all_time, 'FM990.00') AS plan_time_dist_ratio,
	to_char(100*s.cpu_time / s.all_time, 'FM990.00') AS cpu_time_dist_ratio,
	to_char(100*s.io_time / s.all_time, 'FM990.00') AS io_time_dist_ratio,
	to_char(s.wal_records, 'FM999,999,999,990') AS wal_records,
	to_char(100*s.wal_records/(SELECT coalesce(nullif(total_wal_records, 0), 1) FROM totals), 'FM990.00') AS wal_records_ratio,
	to_char(s.wal_fpi, 'FM999,999,999,990') AS wal_fpi,
	to_char(100*s.wal_fpi/(SELECT coalesce(nullif(total_wal_fpi, 0), 1) FROM totals), 'FM990.00') AS wal_fpi_ratio,
	pg_size_pretty(s.wal_bytes::bigint) AS wal_bytes,
	to_char(100*s.wal_bytes/(SELECT coalesce(nullif(total_wal_bytes, 0), 1) FROM totals), 'FM990.00') AS wal_bytes_ratio
FROM stmt s
	JOIN pg_database d ON d.oid=s.dbid
LIMIT 1
`

	// PgStatStatementsReportQueryPG12 defines query used for calculating per-statement report based on pg_stat_statements for postgres 12 and earlier.
	PgStatStatementsReportQueryPG12 = "WITH totals AS (SELECT " +
		"sum(calls) AS total_calls," +
		"sum(rows) AS total_rows," +
		"sum(total_time) AS total_all_time," +
		"0 AS total_plan_time," +
		"sum(total_time - blk_read_time + blk_write_time) AS total_cpu_time," +
		"sum(blk_read_time+blk_write_time) AS total_io_time," +
		"0 AS total_wal_records," +
		"0 AS total_wal_fpi," +
		"0 AS total_wal_bytes " +
		"FROM {{.PGSSSchema}}.pg_stat_statements)," +
		"stmt AS (" +
		"SELECT " +
		"query, queryid, userid, dbid," +
		"calls AS calls," +
		"rows AS rows," +
		"total_time AS all_time," +
		"0 AS plan_time," +
		"total_time - blk_read_time + blk_write_time AS cpu_time," +
		"blk_read_time + blk_write_time AS io_time," +
		"0 AS wal_records, 0 AS wal_fpi, 0 AS wal_bytes " +
		"FROM {{.PGSSSchema}}.pg_stat_statements " +
		"WHERE left(md5(userid::text || dbid::text || queryid::text), 10) = $1) " +
		"SELECT s.query, s.queryid::text AS queryid, s.userid::regrole AS usename, d.datname," +
		"to_char((SELECT total_calls FROM totals), 'FM999,999,999,990') AS total_calls," +
		"to_char((SELECT total_rows FROM totals), 'FM999,999,999,990') AS total_rows," +
		"pg_size_pretty((SELECT total_wal_bytes FROM totals)::bigint) AS total_wal_bytes," +
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
		"to_char(100*s.io_time / s.all_time, 'FM990.00') AS io_time_dist_ratio," +
		"to_char(s.wal_records, 'FM999,999,999,990') AS wal_records," +
		"to_char(100*s.wal_records/(SELECT coalesce(nullif(total_wal_records, 0), 1) FROM totals), 'FM990.00') AS wal_records_ratio," +
		"to_char(s.wal_fpi, 'FM999,999,999,990') AS wal_fpi," +
		"to_char(100*s.wal_fpi/(SELECT coalesce(nullif(total_wal_fpi, 0), 1) FROM totals), 'FM990.00') AS wal_fpi_ratio," +
		"pg_size_pretty(s.wal_bytes::bigint) AS wal_bytes," +
		"to_char(100*s.wal_bytes/(SELECT coalesce(nullif(total_wal_bytes, 0), 1) FROM totals), 'FM990.00') AS wal_bytes_ratio " +
		"FROM stmt s JOIN pg_database d ON d.oid=s.dbid LIMIT 1"

	// PgStatStatementsJITPG15 defines JIT compilation query for pg_stat_statements (PG 15-16).
	PgStatStatementsJITPG15 = "SELECT pg_get_userbyid(p.userid) AS user, d.datname AS database, " +
		"date_trunc('seconds', round(p.jit_generation_time) / 1000 * '1 second'::interval)::text AS gen_total, " +
		"date_trunc('seconds', round(p.jit_inlining_time) / 1000 * '1 second'::interval)::text AS inline_total, " +
		"date_trunc('seconds', round(p.jit_optimization_time) / 1000 * '1 second'::interval)::text AS opt_total, " +
		"date_trunc('seconds', round(p.jit_emission_time) / 1000 * '1 second'::interval)::text AS emit_total, " +
		`round(p.jit_generation_time) AS "gen,ms", round(p.jit_inlining_time) AS "inline,ms", ` +
		`round(p.jit_optimization_time) AS "opt,ms", round(p.jit_emission_time) AS "emit,ms", ` +
		"p.jit_functions AS functions, left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid, " +
		`regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query ` +
		"FROM {{.PGSSSchema}}.pg_stat_statements p JOIN pg_database d ON d.oid=p.dbid WHERE p.jit_functions > 0"

	// PgStatStatementsJITDefault defines JIT compilation query for pg_stat_statements (PG 17+).
	// PG 17 adds jit_deform_count/jit_deform_time columns.
	PgStatStatementsJITDefault = "SELECT pg_get_userbyid(p.userid) AS user, d.datname AS database, " +
		"date_trunc('seconds', round(p.jit_generation_time) / 1000 * '1 second'::interval)::text AS gen_total, " +
		"date_trunc('seconds', round(p.jit_inlining_time) / 1000 * '1 second'::interval)::text AS inline_total, " +
		"date_trunc('seconds', round(p.jit_optimization_time) / 1000 * '1 second'::interval)::text AS opt_total, " +
		"date_trunc('seconds', round(p.jit_emission_time) / 1000 * '1 second'::interval)::text AS emit_total, " +
		"date_trunc('seconds', round(p.jit_deform_time) / 1000 * '1 second'::interval)::text AS deform_total, " +
		`round(p.jit_generation_time) AS "gen,ms", round(p.jit_inlining_time) AS "inline,ms", ` +
		`round(p.jit_optimization_time) AS "opt,ms", round(p.jit_emission_time) AS "emit,ms", ` +
		`round(p.jit_deform_time) AS "deform,ms", ` +
		"p.jit_functions AS functions, left(md5(p.userid::text || p.dbid::text || p.queryid::text), 10) AS queryid, " +
		`regexp_replace({{.PgSSQueryLenFn}}, E'\\s+', ' ', 'g') AS query ` +
		"FROM {{.PGSSSchema}}.pg_stat_statements p JOIN pg_database d ON d.oid=p.dbid WHERE p.jit_functions > 0"
)

// SelectStatStatementsTimingQuery returns proper statements_timing query depending on Postgres version.
func SelectStatStatementsTimingQuery(version int) string {
	switch {
	case version < 130000:
		return PgStatStatementsTimingPG12
	case version >= 170000:
		return PgStatStatementsTimingDefault
	default:
		return PgStatStatementsTimingPG13
	}
}

// SelectStatStatementsJITQuery returns the statements_jit query, column count, diff interval
// and unique-key index depending on Postgres version. PG17+ adds the jit_deform_* columns,
// shifting the interval block, functions, queryid and query right by one.
func SelectStatStatementsJITQuery(version int) (string, int, [2]int, int) {
	if version >= PostgresV17 {
		return PgStatStatementsJITDefault, 15, [2]int{7, 12}, 13
	}
	return PgStatStatementsJITPG15, 13, [2]int{6, 10}, 11
}

// SelectQueryReportQuery returns proper query report query depending on Postgres version.
func SelectQueryReportQuery(version int) string {
	switch {
	case version < 130000:
		return PgStatStatementsReportQueryPG12
	case version >= 170000:
		return PgStatStatementsReportQueryDefault
	default:
		return PgStatStatementsReportQueryPG13
	}
}
