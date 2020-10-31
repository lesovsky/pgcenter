// Stuff related to particular SQL queries that used for gathering stats

package stat

import (
	"bytes"
	"fmt"
	"text/template"
)

const (
	// PgGetSingleSettingQuery queries specified Postgres configuration setting
	PgGetSingleSettingQuery = "SELECT current_setting($1)"
	// PgGetVersionQuery queries Postgres versions
	PgGetVersionQuery = "SELECT current_setting('server_version'),current_setting('server_version_num')"
	// PgGetRecoveryStatusQuery queries current Postgres recovery status
	PgGetRecoveryStatusQuery = "SELECT pg_is_in_recovery()"
	// PgGetUptimeQuery queries Postgres uptime
	PgGetUptimeQuery = "SELECT date_trunc('seconds', now() - pg_postmaster_start_time())"
	// PgCheckPGSSExists checks that pg_stat_statements view exists
	PgCheckPGSSExists = "SELECT EXISTS (SELECT 1 FROM information_schema.views WHERE table_name = 'pg_stat_statements')"
	// PgCheckPgcenterSchemaQuery checks existence of pgcenter's stats schema
	PgCheckPgcenterSchemaQuery = "SELECT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = 'pgcenter')"
	// PgGetConfigAllQuery queries current Postgres configuration
	PgGetConfigAllQuery = "SELECT name, setting, unit, category FROM pg_settings ORDER BY 4"
	// PgGetCurrentLogfileQuery queries current Postgres logfile
	PgGetCurrentLogfileQuery = "SELECT pg_current_logfile();"
	// PgReloadConfQuery does Postgres reload
	PgReloadConfQuery = "SELECT pg_reload_conf()"
	// PgPostmasterStartTimeQuery queries time when Postgres has been started
	PgPostmasterStartTimeQuery = "SELECT to_char(pg_postmaster_start_time(), 'HH24MISS')"
	// PgCancelSingleQuery cancels query executed by backend with specified PID
	PgCancelSingleQuery = `SELECT pg_cancel_backend($1)`
	// PgTerminateSingleQuery terminates the backend with specified PID
	PgTerminateSingleQuery = `SELECT pg_terminate_backend($1)`
	// PgCancelGroupQuery cancels a group of queries based on specified criteria
	PgCancelGroupQuery = `SELECT
count(pg_cancel_backend(pid))
FROM pg_stat_activity
WHERE {{.BackendState}}
AND ((clock_timestamp() - xact_start) > '{{.QueryAgeThresh}}'::interval OR (clock_timestamp() - query_start) > '{{.QueryAgeThresh}}'::interval)
AND pid != pg_backend_pid()`
	// PgTerminateGroupQuery terminate a group of backends based on specified crteria
	PgTerminateGroupQuery = `SELECT
count(pg_terminate_backend(pid))
FROM pg_stat_activity
WHERE {{.BackendState}}
AND ((clock_timestamp() - xact_start) > '{{.QueryAgeThresh}}'::interval OR (clock_timestamp() - query_start) > '{{.QueryAgeThresh}}'::interval)
AND pid != pg_backend_pid()`
	// PgResetStats resets statistics counter in the current database
	PgResetStats = "SELECT pg_stat_reset()"
	// PgResetPgss resets pg_stat_statements statistics
	PgResetPgss = "SELECT pg_stat_statements_reset()"

	// PgActivityQueryDefault is the default query for getting stats about connected clients from pg_stat_activity
	PgActivityQueryDefault = `SELECT
count(*) FILTER (WHERE state IS NOT NULL) AS total,
count(*) FILTER (WHERE state = 'idle') AS idle,
count(*) FILTER (WHERE state IN ('idle in transaction', 'idle in transaction (aborted)')) AS idle_in_xact,
count(*) FILTER (WHERE state = 'active') AS active,
count(*) FILTER (WHERE wait_event_type = 'Lock') AS waiting,
count(*) FILTER (WHERE state IN ('fastpath function call','disabled')) AS others,
(SELECT count(*) FROM pg_prepared_xacts) AS total_prepared
FROM pg_stat_activity WHERE backend_type = 'client backend'`

	// PgActivityQueryBefore10 queries activity stats about connected clients for versions prior 10. The 'backend_type' has been introduced in 10.
	PgActivityQueryBefore10 = `SELECT
count(*) FILTER (WHERE state IS NOT NULL) AS total,
count(*) FILTER (WHERE state = 'idle') AS idle,
count(*) FILTER (WHERE state IN ('idle in transaction', 'idle in transaction (aborted)')) AS idle_in_xact,
count(*) FILTER (WHERE state = 'active') AS active,
count(*) FILTER (WHERE wait_event_type = 'Lock') AS waiting,
count(*) FILTER (WHERE state IN ('fastpath function call','disabled')) AS others,
(SELECT count(*) FROM pg_prepared_xacts) AS total_prepared
FROM pg_stat_activity`

	// PgActivityQueryBefore96 queries stats activity about connected clients for versions prior 9.6. There wait_events have been introduced in 9.6.
	PgActivityQueryBefore96 = `SELECT
count(*) FILTER (WHERE state IS NOT NULL) AS total,
count(*) FILTER (WHERE state = 'idle') AS idle,
count(*) FILTER (WHERE state IN ('idle in transaction', 'idle in transaction (aborted)')) AS idle_in_xact,
count(*) FILTER (WHERE state = 'active') AS active,
count(*) FILTER (WHERE waiting) AS waiting,
count(*) FILTER (WHERE state IN ('fastpath function call','disabled')) AS others,
(SELECT count(*) FROM pg_prepared_xacts) AS total_prepared
FROM pg_stat_activity`

	// PgActivityQueryBefore94 queries stats activity about connected clients for versions prior 9.4. There 'FILTER (WHERE ...)' has been introduced in 9.4.
	PgActivityQueryBefore94 = `WITH pgsa AS (SELECT * FROM pg_stat_activity)
SELECT
(SELECT count(*) FROM pgsa) AS total,
(SELECT count(*) FROM pgsa WHERE state = 'idle') AS idle,
(SELECT count(*) FROM pgsa WHERE state IN ('idle in transaction', 'idle in transaction (aborted)')) AS idle_in_xact,
(SELECT count(*) FROM pgsa WHERE state = 'active') AS active,
(SELECT count(*) FROM pgsa WHERE waiting) AS waiting,
(SELECT count(*) FROM pgsa WHERE state IN ('fastpath function call','disabled')) AS others,
(SELECT count(*) FROM pg_prepared_xacts) AS total_prepared`

	// PgAutovacQueryDefault is the default query for getting stats about autovacuum activity from pg_stat_activity
	PgAutovacQueryDefault = `SELECT
count(*) FILTER (WHERE query ~* '^autovacuum:') AS av_workers,
count(*) FILTER (WHERE query ~* '^autovacuum:.*to prevent wraparound') AS av_wrap,
count(*) FILTER (WHERE query ~* '^vacuum' AND state != 'idle') AS v_manual,
coalesce(date_trunc('seconds', max(now() - xact_start)), '00:00:00') AS av_maxtime
FROM pg_stat_activity
WHERE (query ~* '^autovacuum:' OR query ~* '^vacuum') AND pid <> pg_backend_pid()`

	// PgAutovacQueryBefore94 queries stats about autovacuum activity for versions prior 9.4. There 'FILTER (WHERE ...)' has been introduced.
	PgAutovacQueryBefore94 = `WITH pgsa AS (SELECT * FROM pg_stat_activity)
SELECT
(SELECT count(*) FROM pgsa WHERE query ~* '^autovacuum:' AND pid <> pg_backend_pid()) AS av_workers,
(SELECT count(*) FROM pgsa WHERE query ~* '^autovacuum:.*to prevent wraparound' AND pid <> pg_backend_pid()) AS av_wrap,
(SELECT count(*) FROM pgsa WHERE query ~* '^vacuum' AND pid <> pg_backend_pid()) AS v_manual,
(SELECT coalesce(date_trunc('seconds', max(now() - xact_start)), '00:00:00') FROM pgsa
WHERE (query ~* '^autovacuum:' OR query ~* '^vacuum') AND pid <> pg_backend_pid()) AS av_maxtime`

	// PgActivityTimeQuery queries stats about longest transactions
	PgActivityTimeQuery = `SELECT
(SELECT coalesce(date_trunc('seconds', max(now() - xact_start)), '00:00:00') AS xact_maxtime
FROM pg_stat_activity
WHERE (query !~* '^autovacuum:' AND query !~* '^vacuum') AND pid <> pg_backend_pid()),
(SELECT COALESCE(date_trunc('seconds', max(clock_timestamp() - prepared)), '00:00:00') AS prep_maxtime
FROM pg_prepared_xacts)`

	// PgStatementsQuery queries general stats from pg_stat_statements
	PgStatementsQuery12     = `SELECT (sum(total_time) / sum(calls))::numeric(20,2) AS avg_query, sum(calls) AS total_calls FROM pg_stat_statements`
	PgStatementsQueryLatest = `SELECT (sum(total_exec_time+total_plan_time) / sum(calls))::numeric(20,2) AS avg_query, sum(calls) AS total_calls FROM pg_stat_statements`

	// PgStatDatabaseQueryDefault is the default query for getting databases' stats from pg_stat_database view
	// { Name: "pg_stat_database", Query: common.PgStatDatabaseQueryDefault, DiffIntvl: [2]int{1,16}, Ncols: 18, OrderKey: 0, OrderDesc: true }
	PgStatDatabaseQueryDefault = `SELECT
datname,
coalesce(xact_commit, 0) AS commits,
coalesce(xact_rollback, 0) AS rollbacks,
coalesce(blks_read * (SELECT current_setting('block_size')::int / 1024), 0) AS reads,
coalesce(blks_hit, 0) AS hits,
coalesce(tup_returned, 0) AS returned,
coalesce(tup_fetched, 0) AS fetched,
coalesce(tup_inserted, 0) AS inserts,
coalesce(tup_updated, 0) AS updates,
coalesce(tup_deleted, 0) AS deletes,
coalesce(conflicts, 0) AS conflicts,
coalesce(deadlocks, 0) AS deadlocks,
coalesce(checksum_failures, 0) AS csum_fails,
coalesce(temp_files, 0) AS temp_files,
coalesce(temp_bytes, 0) AS temp_bytes,
coalesce(blk_read_time, 0)::numeric(20,2) AS read_t,
coalesce(blk_write_time, 0)::numeric(20,2) AS write_t,
date_trunc('seconds', now() - stats_reset)::text AS stats_age
FROM pg_stat_database
ORDER BY datname DESC`

	// PgStatDatabaseQuery11 is the query for getting databases' stats from pg_stat_database view for versions prior 12
	// { Name: "pg_stat_database", Query: common.PgStatDatabaseQuery11, DiffIntvl: [2]int{1,15}, Ncols: 17, OrderKey: 0, OrderDesc: true }
	PgStatDatabaseQuery11 = `SELECT
datname,
coalesce(xact_commit, 0) AS commits,
coalesce(xact_rollback, 0) AS rollbacks,
coalesce(blks_read * (SELECT current_setting('block_size')::int / 1024), 0) AS reads,
coalesce(blks_hit, 0) AS hits,
coalesce(tup_returned, 0) AS returned,
coalesce(tup_fetched, 0) AS fetched,
coalesce(tup_inserted, 0) AS inserts,
coalesce(tup_updated, 0) AS updates,
coalesce(tup_deleted, 0) AS deletes,
coalesce(conflicts, 0) AS conflicts,
coalesce(deadlocks, 0) AS deadlocks,
coalesce(temp_files, 0) AS temp_files,
coalesce(temp_bytes, 0) AS temp_bytes,
coalesce(blk_read_time, 0)::numeric(20,2) AS read_t,
coalesce(blk_write_time, 0)::numeric(20,2) AS write_t,
date_trunc('seconds', now() - stats_reset)::text AS stats_age
FROM pg_stat_database
ORDER BY datname DESC`

	// PgStatReplicationQueryDefault is the default query for getting replication stats from pg_stat_replication view
	// { Name: "pg_stat_replication", Query: common.PgStatReplicationQueryDefault, DiffIntvl: [2]int{6,6}, Ncols: 15, OrderKey: 0, OrderDesc: true }
	PgStatReplicationQueryDefault = `SELECT
pid AS pid,
client_addr AS client,
usename AS user,
application_name AS name,
state,
sync_state AS mode,
({{.WalFunction1}}({{.WalFunction2}}(),'0/0') / 1024)::bigint AS wal,
({{.WalFunction1}}({{.WalFunction2}}(),sent_lsn) / 1024)::bigint AS pending,
({{.WalFunction1}}(sent_lsn,write_lsn) / 1024)::bigint AS write,
({{.WalFunction1}}(write_lsn,flush_lsn) / 1024)::bigint AS flush,
({{.WalFunction1}}(flush_lsn,replay_lsn) / 1024)::bigint AS replay,
({{.WalFunction1}}({{.WalFunction2}}(),replay_lsn))::bigint / 1024 AS total_lag,
coalesce(date_trunc('seconds', write_lag), '0 seconds'::interval) AS write_lag,
coalesce(date_trunc('seconds', flush_lag), '0 seconds'::interval) AS flush_lag,
coalesce(date_trunc('seconds', replay_lag), '0 seconds'::interval) AS replay_lag
FROM pg_stat_replication
ORDER BY pid DESC`

	// PgStatReplicationQueryExtended is the extended query for getting replication stats from pg_stat_replication view
	// { Name: "pg_stat_replication", Query: common.PgStatReplicationQueryExtended, DiffIntvl: [2]int{6,6}, Ncols: 17, OrderKey: 0, OrderDesc: true }
	PgStatReplicationQueryExtended = `SELECT
pid AS pid,
client_addr AS client,
usename AS user,
application_name AS name,
state,
sync_state AS mode,
({{.WalFunction1}}({{.WalFunction2}}(),'0/0') / 1024)::bigint AS wal,
({{.WalFunction1}}({{.WalFunction2}}(),sent_lsn) / 1024)::bigint AS pending,
({{.WalFunction1}}(sent_lsn,write_lsn) / 1024)::bigint AS write,
({{.WalFunction1}}(write_lsn,flush_lsn) / 1024)::bigint AS flush,
({{.WalFunction1}}(flush_lsn,replay_lsn) / 1024)::bigint AS replay,
({{.WalFunction1}}({{.WalFunction2}}(),replay_lsn) / 1024)::bigint AS total_lag,
coalesce(date_trunc('seconds', write_lag), '0 seconds'::interval) AS write_lag,
coalesce(date_trunc('seconds', flush_lag), '0 seconds'::interval) AS flush_lag,
coalesce(date_trunc('seconds', replay_lag), '0 seconds'::interval) AS replay_lag,
(pg_last_committed_xact()).xid::text::bigint - backend_xmin::text::bigint as xact_age,
date_trunc('seconds', (pg_last_committed_xact()).timestamp - pg_xact_commit_timestamp(backend_xmin)) as time_age
FROM pg_stat_replication
ORDER BY pid DESC`

	// PgStatReplicationQuery96 is the query for getting replication stats from versions prior 9.6
	// { Name: "pg_stat_replication", Query: common.PgStatReplicationQuery96, DiffIntvl: [2]int{6,6}, Ncols: 12, OrderKey: 0, OrderDesc: true }
	PgStatReplicationQuery96 = `SELECT
pid AS pid,
client_addr AS client,
usename AS user,
application_name AS name,
state,
sync_state AS mode,
({{.WalFunction1}}({{.WalFunction2}}(),'0/0') / 1024)::bigint AS wal,
({{.WalFunction1}}({{.WalFunction2}}(),sent_location) / 1024)::bigint AS pending,
({{.WalFunction1}}(sent_location,write_location) / 1024)::bigint AS write,
({{.WalFunction1}}(write_location,flush_location) / 1024)::bigint AS flush,
({{.WalFunction1}}(flush_location,replay_location) / 1024)::bigint AS replay,
({{.WalFunction1}}({{.WalFunction2}}(),replay_location))::bigint / 1024 AS total_lag
FROM pg_stat_replication
ORDER BY pid DESC`

	// PgStatReplicationQuery96Extended is the extended query for getting replication stats from versions prior 9.6
	// { Name: "pg_stat_replication", Query: common.PgStatReplicationQuery96Extended, DiffIntvl: [2]int{6,6}, Ncols: 14, OrderKey: 0, OrderDesc: true }
	PgStatReplicationQuery96Extended = `SELECT
pid AS pid,
client_addr AS client,
usename AS user,
application_name AS name,
state,
sync_state AS mode,
({{.WalFunction1}}({{.WalFunction2}}(),'0/0') / 1024)::bigint AS wal,
({{.WalFunction1}}({{.WalFunction2}}(),sent_location) / 1024)::bigint AS pending,
({{.WalFunction1}}(sent_location,write_location) / 1024)::bigint AS write,
({{.WalFunction1}}(write_location,flush_location) / 1024)::bigint AS flush,
({{.WalFunction1}}(flush_location,replay_location) / 1024)::bigint AS replay,
({{.WalFunction1}}({{.WalFunction2}}(),replay_location))::bigint / 1024 AS total_lag,
(pg_last_committed_xact()).xid::text::bigint - backend_xmin::text::bigint as xact_age,
date_trunc('seconds', (pg_last_committed_xact()).timestamp - pg_xact_commit_timestamp(backend_xmin)) as time_age
FROM pg_stat_replication
ORDER BY pid DESC`

	// PgStatTablesQueryDefault is the default query for getting tables' stats from pg_stat_all_tables and pg_statio_all_tables views
	// { Name: "pg_stat_tables", Query: common.PgStatTablesQueryDefault, DiffIntvl: [2]int{1,18}, Ncols: 19, OrderKey: 0, OrderDesc: true }
	PgStatTablesQueryDefault = `SELECT
t.schemaname || '.' || t.relname AS relation,
coalesce(t.seq_scan, 0) AS seq_scan,
coalesce(t.seq_tup_read, 0) AS seq_read,
coalesce(t.idx_scan, 0) AS idx_scan,
coalesce(t.idx_tup_fetch, 0) AS idx_fetch,
coalesce(t.n_tup_ins, 0) AS inserts,
coalesce(t.n_tup_upd, 0) AS updates,
coalesce(t.n_tup_del, 0) AS deletes,
coalesce(t.n_tup_hot_upd, 0) AS hot_updates,
coalesce(t.n_live_tup, 0) AS live,
coalesce(t.n_dead_tup, 0) AS dead,
coalesce(i.heap_blks_read * (SELECT current_setting('block_size')::int / 1024), 0) AS heap_read,
coalesce(i.heap_blks_hit, 0) AS heap_hit,
coalesce(i.idx_blks_read * (SELECT current_setting('block_size')::int / 1024), 0) AS idx_read,
coalesce(i.idx_blks_hit, 0) AS idx_hit,
coalesce(i.toast_blks_read * (SELECT current_setting('block_size')::int / 1024), 0) AS toast_read,
coalesce(i.toast_blks_hit, 0) AS toast_hit,
coalesce(i.tidx_blks_read * (SELECT current_setting('block_size')::int / 1024), 0) AS tidx_read,
coalesce(i.tidx_blks_hit, 0) AS tidx_hit
FROM pg_stat_{{.ViewType}}_tables t, pg_statio_{{.ViewType}}_tables i
WHERE t.relid = i.relid
ORDER BY (t.schemaname || '.' || t.relname) DESC`

	// PgStatIndexesQueryDefault is the default query for getting indexes' stats from pg_stat_all_indexes and pg_statio_all_indexes views
	// { Name: "pg_stat_indexes", Query: common.PgStatIndexesQueryDefault, DiffIntvl: [2]int{1,5}, Ncols: 6, OrderKey: 0, OrderDesc: true }
	PgStatIndexesQueryDefault = `SELECT
s.schemaname ||'.'|| s.relname ||'.'|| s.indexrelname AS index,
coalesce(s.idx_scan, 0) AS idx_scan,
coalesce(s.idx_tup_read, 0) AS idx_tup_read,
coalesce(s.idx_tup_fetch, 0) AS idx_tup_fetch,
coalesce(i.idx_blks_read * (SELECT current_setting('block_size')::int / 1024), 0) AS idx_read,
coalesce(i.idx_blks_hit, 0) AS idx_hit
FROM pg_stat_{{.ViewType}}_indexes s, pg_statio_{{.ViewType}}_indexes i
WHERE s.indexrelid = i.indexrelid
ORDER BY (s.schemaname ||'.'|| s.relname ||'.'|| s.indexrelname) DESC`

	// PgTablesSizesQueryDefault is the defaulr query for getting stats related to tables' sizes
	// { Name: "pg_tables_sizes", Query: common.PgTablesSizesQueryDefault, DiffIntvl: [2]int{4,6}, Ncols: 7, OrderKey: 0, OrderDesc: true }
	PgTablesSizesQueryDefault = `SELECT
s.schemaname ||'.'|| s.relname AS relation,
pg_total_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024 AS total_size,
pg_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024 AS rel_size,
(pg_total_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024) -
(pg_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024) AS idx_size,
pg_total_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024 AS total_change,
pg_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024 AS rel_change,
(pg_total_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024) -
(pg_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024) AS idx_change
FROM pg_stat_{{.ViewType}}_tables s, pg_class c
WHERE s.relid = c.oid AND NOT EXISTS (SELECT 1 FROM pg_locks WHERE relation = s.relid AND mode = 'AccessExclusiveLock' and granted)
ORDER BY (s.schemaname || '.' || s.relname) DESC`

	// PgStatFunctionsQueryDefault is the default query for getting stats from pg_stat_user_functions view
	// { Name: "pg_stat_functions", Query: common.PgStatFunctionsQueryDefault, DiffIntvl: [2]int{3,3}, Ncols: 8, OrderKey: 0, OrderDesc: true }
	PgStatFunctionsQueryDefault = `SELECT
funcid,
schemaname ||'.'||funcname AS function,
calls AS total_calls,
calls AS calls,
date_trunc('seconds', total_time / 1000 * '1 second'::interval)::text AS total_t,
date_trunc('seconds', self_time / 1000 * '1 second'::interval)::text AS self_t,
round((total_time / greatest(calls, 1))::numeric(20,2), 4) AS avg_t,
round((self_time / greatest(calls, 1))::numeric(20,2), 4) AS avg_self_t
FROM pg_stat_user_functions
ORDER BY funcid DESC`

	// PgStatProgressVacuumQueryDefault is the default query for getting stats from pg_stat_progress_vacuum view
	// { Name: "pg_stat_vacuum", Query: common.PgStatVacuumQueryDefault, DiffIntvl: [2]int{10,11}, Ncols: 13, OrderKey: 0, OrderDesc: true }
	PgStatProgressVacuumQueryDefault = `SELECT
a.pid,
date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age,
v.datname,
v.relid::regclass AS relation,
a.state,
coalesce((a.wait_event_type ||'.'|| a.wait_event), 'f') AS waiting,
v.phase,
v.heap_blks_total * (SELECT current_setting('block_size')::int / 1024) AS t_size,
round(100 * v.heap_blks_scanned / v.heap_blks_total, 2) AS "t_scanned_%",
round(100 * v.heap_blks_vacuumed / v.heap_blks_total, 2) AS "t_vacuumed_%",
coalesce(v.heap_blks_scanned * (SELECT current_setting('block_size')::int / 1024), 0) AS scanned,
coalesce(v.heap_blks_vacuumed * (SELECT current_setting('block_size')::int / 1024), 0) AS vacuumed,
a.query
FROM pg_stat_progress_vacuum v
RIGHT JOIN pg_stat_activity a ON v.pid = a.pid
WHERE (a.query ~* '^autovacuum:' OR a.query ~* '^vacuum') AND a.pid <> pg_backend_pid()
ORDER BY a.pid DESC`

	// PgStatProgressClusterQueryDefault is the default query for getting stats from pg_stat_progress_cluster view
	// { Name: "pg_stat_progress_cluster", Query: common.PgStatProgressClusterQueryDefault, DiffIntvl: [2]int{10,11}, Ncols: 13, OrderKey: 0, OrderDesc: true }
	PgStatProgressClusterQueryDefault = `SELECT
a.pid,
date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age,
p.datname,
p.relid::regclass AS relation,
p.cluster_index_relid::regclass AS index,
a.state,
coalesce((a.wait_event_type ||'.'|| a.wait_event), 'f') AS waiting,
p.phase,
p.heap_blks_total * (SELECT current_setting('block_size')::int / 1024) AS t_size,
round(100 * p.heap_blks_scanned / greatest(p.heap_blks_total,1), 2) AS "scanned_%",
coalesce(p.heap_tuples_scanned, 0) AS tup_scanned,
coalesce(p.heap_tuples_written, 0) AS tup_written,
a.query
FROM pg_stat_progress_cluster p
INNER JOIN pg_stat_activity a ON p.pid = a.pid
WHERE a.pid <> pg_backend_pid()
ORDER BY a.pid DESC`

	// PgStatProgressCreateIndexQueryDefault is the default query for getting stats from pg_stat_progress_cluster view
	// { Name: "pg_stat_progress_create_index", Query: common.PgStatProgressCreateIndexQueryDefault, DiffIntvl: [2]int{99,99}, Ncols: 14, OrderKey: 0, OrderDesc: true }
	PgStatProgressCreateIndexQueryDefault = `SELECT
a.pid,
date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age,
p.datname,
p.relid::regclass AS relation,
p.index_relid::regclass AS index,
a.state,
coalesce((a.wait_event_type ||'.'|| a.wait_event), 'f') AS waiting,
p.phase,
current_locker_pid AS locker_pid,
lockers_total ||'/'|| lockers_done AS lockers,
p.blocks_total * (SELECT current_setting('block_size')::int / 1024) ||'/'|| round(100 * p.blocks_done / greatest(p.blocks_total, 1), 2) AS "size_total/done_%",
p.tuples_total ||'/'|| round(100 * p.tuples_done / greatest(p.tuples_total, 1), 2) AS "tup_total/done_%",
p.partitions_total ||'/'|| round(100 * p.partitions_done / greatest(p.partitions_total, 1), 2) AS "parts_total/done_%",
a.query
FROM pg_stat_progress_create_index p
INNER JOIN pg_stat_activity a ON p.pid = a.pid
WHERE a.pid <> pg_backend_pid()
ORDER BY a.pid DESC`

	// PgStatActivityQueryDefault is the default query for getting stats from pg_stat_activity view
	// { Name: "pg_stat_activity", Query: common.PgStatActivityQueryDefault, DiffIntvl: [2]int{99,99}, Ncols: 14, OrderKey: 0, OrderDesc: true }
	// regexp_replace() removes extra spaces, tabs and newlines from queries
	PgStatActivityQueryDefault = `SELECT
pid,
client_addr AS cl_addr,
client_port AS cl_port,
datname,
usename,
left(application_name, 16) AS appname,
backend_type,
wait_event_type AS wait_etype,
wait_event,
state,
date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age,
date_trunc('seconds', clock_timestamp() - query_start)::text AS query_age,
date_trunc('seconds', clock_timestamp() - state_change)::text AS change_age,
regexp_replace(
regexp_replace(query,
E'( |\t)+', ' ', 'g'),
E'\n', '', 'g') AS query
FROM pg_stat_activity
{{ if .ShowNoIdle }}
WHERE ((clock_timestamp() - xact_start) > '{{.QueryAgeThresh}}'::interval OR (clock_timestamp() - query_start) > '{{.QueryAgeThresh}}'::interval)
AND state != 'idle'
{{ end }}
ORDER BY pid DESC`

	// PgStatActivityQuery96 queries for getting stats from pg_stat_activity view for versions prior 9.6
	// { Name: "pg_stat_activity", Query: common.PgStatActivityQuery96, DiffIntvl: [2]int{99,99}, Ncols: 13, OrderKey: 0, OrderDesc: true }
	// regexp_replace() removes extra spaces, tabs and newlines from queries
	PgStatActivityQuery96 = `SELECT
pid,
client_addr AS cl_addr,
client_port AS cl_port,
datname,
usename,
left(application_name, 16) AS appname,
wait_event_type AS wait_etype,
wait_event,
state,
date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age,
date_trunc('seconds', clock_timestamp() - query_start)::text AS query_age,
date_trunc('seconds', clock_timestamp() - state_change)::text AS change_age,
regexp_replace(
regexp_replace(query,
E'( |\t)+', ' ', 'g'),
E'\n', '', 'g') AS query
FROM pg_stat_activity
{{ if .ShowNoIdle }}
WHERE ((clock_timestamp() - xact_start) > '{{.QueryAgeThresh}}'::interval OR (clock_timestamp() - query_start) > '{{.QueryAgeThresh}}'::interval)
AND state != 'idle'
{{ end }}
ORDER BY pid DESC`

	// PgStatActivityQuery95 queries activity stats from pg_stat_activity view from versions prior 9.5
	// { Name: "pg_stat_activity", Query: common.PgStatActivityQuery95, DiffIntvl: [2]int{99,99}, Ncols: 12, OrderKey: 0, OrderDesc: true }
	// regexp_replace() removes extra spaces, tabs and newlines from queries
	PgStatActivityQuery95 = `SELECT
pid,
client_addr AS cl_addr,
client_port AS cl_port,
datname,
usename,
left(application_name, 16) AS appname,
waiting,
state,
date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age,
date_trunc('seconds', clock_timestamp() - query_start)::text AS query_age,
date_trunc('seconds', clock_timestamp() - state_change)::text AS change_age,
regexp_replace(
regexp_replace(query,
E'( |\t)+', ' ', 'g'),
E'\n', '', 'g') AS query
FROM pg_stat_activity
{{ if .ShowNoIdle }}
WHERE ((clock_timestamp() - xact_start) > '{{.QueryAgeThresh}}'::interval OR (clock_timestamp() - query_start) > '{{.QueryAgeThresh}}'::interval)
AND state != 'idle'
{{ end }}
ORDER BY pid DESC`

	// Some notes about pg_stat_statements-related queries:
	// 1. regexp_replace() removes extra spaces, tabs and newlines from queries

	// PgStatStatementsTimingQueryDefault is the default query for getting timings stats from pg_stat_statements view
	// { Name: "pg_stat_statements_timing", Query: common.PgStatStatementsTimingQueryDefault, DiffIntvl: [2]int{6,10}, Ncols: 13, OrderKey: 0, OrderDesc: true }
	PgStatStatementsTimingQueryDefault = `SELECT
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

// Options contains queries' settings that used depending on user preferences.
type Options struct {
	ViewType       string // Show stats including system tables/indexes
	WalFunction1   string // Use old pg_xlog_* or newer pg_wal_* functions
	WalFunction2   string // Use old pg_xlog_* or newer pg_wal_* functions
	QueryAgeThresh string // Show only queries with duration more than specified
	BackendState   string // Backend state's selector for cancel/terminate function
	ShowNoIdle     bool   // don't show IDLEs, background workers)
	PgSSQueryLen   int    // Specify the length of query to show in pg_stat_statements
	PgSSQueryLenFn string // Specify exact func to truncating query
}

// PrepareQuery transforms query's template to a particular query
func PrepareQuery(s string, o Options) (string, error) {
	t := template.Must(template.New("query").Parse(s))
	buf := &bytes.Buffer{}
	if err := t.Execute(buf, o); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// Adjust method used for adjusting query's options depending on Postgres version.
func (o *Options) Adjust(pi PgInfo, util string) {
	// System tables and indexes aren't shown by default
	o.ViewType = "user"
	// Don't filter queries by age
	o.QueryAgeThresh = "00:00:00.0"
	// Don't show idle clients and background workers
	o.ShowNoIdle = true

	// Select proper WAL functions
	// 1. WAL-related functions have been renamed in Postgres 10, hence functions' names between 9.x and 10 are differ.
	// 2. Depending on recovery status, for obtaining WAL location different functions have to be used.
	switch {
	case pi.PgVersionNum < 100000:
		o.WalFunction1 = "pg_xlog_location_diff"
		if pi.PgRecovery == "false" {
			o.WalFunction2 = "pg_current_xlog_location"
		} else {
			o.WalFunction2 = "pg_last_xlog_receive_location"
		}
	default:
		o.WalFunction1 = "pg_wal_lsn_diff"
		if pi.PgRecovery == "false" {
			o.WalFunction2 = "pg_current_wal_lsn"
		} else {
			o.WalFunction2 = "pg_last_wal_receive_lsn"
		}
	}

	// Queries settings that are specific for particular utilities
	switch util {
	case "top":
		// we want truncate query length of pg_stat_statements.query, because it make no sense to process full query when sizes of user's screen is limited
		o.PgSSQueryLenFn = "left(p.query, 256)"
	case "record":
		// in case of record program we want to record full length of the query, if user doesn't specified exact length
		if o.PgSSQueryLen != 0 {
			o.PgSSQueryLenFn = fmt.Sprintf("left(p.query, %d)", o.PgSSQueryLen)
		} else {
			o.PgSSQueryLenFn = "p.query"
		}
	}
}
