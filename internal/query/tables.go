package query

const (
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
)
