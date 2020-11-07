package query

const (
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
)
