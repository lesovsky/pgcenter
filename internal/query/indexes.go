package query

const (
	// PgStatIndexesDefault is the default query for getting indexes' stats from pg_stat_all_indexes and pg_statio_all_indexes views
	// { Name: "pg_stat_indexes", Query: common.PgStatIndexesQueryDefault, DiffIntvl: [2]int{1,5}, Ncols: 6, OrderKey: 0, OrderDesc: true }
	PgStatIndexesDefault = "SELECT s.schemaname ||'.'|| s.relname ||'.'|| s.indexrelname AS index, " +
		"coalesce(s.idx_scan, 0) AS idx_scan, coalesce(s.idx_tup_read, 0) AS idx_tup_read, " +
		"coalesce(s.idx_tup_fetch, 0) AS idx_tup_fetch, " +
		"coalesce(i.idx_blks_read * (SELECT current_setting('block_size')::int / 1024), 0) AS idx_read, " +
		"coalesce(i.idx_blks_hit, 0) AS idx_hit " +
		"FROM pg_stat_{{.ViewType}}_indexes s, pg_statio_{{.ViewType}}_indexes i " +
		"WHERE s.indexrelid = i.indexrelid ORDER BY (s.schemaname ||'.'|| s.relname ||'.'|| s.indexrelname) DESC"
)
