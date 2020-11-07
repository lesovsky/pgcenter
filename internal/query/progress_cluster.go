package query

const (
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
)
