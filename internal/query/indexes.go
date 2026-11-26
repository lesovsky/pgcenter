package query

const (
	// PgStatIndexesDefault defines default query for getting indexes' stats from pg_stat_all_indexes and pg_statio_all_indexes views.
	PgStatIndexesDefault = "SELECT s.schemaname ||'.'|| s.relname ||'.'|| s.indexrelname AS index, " +
		"coalesce(s.idx_scan, 0) AS scan, coalesce(s.idx_tup_read, 0) AS tuples_read, " +
		"coalesce(s.idx_tup_fetch, 0) AS tuples_fetch, " +
		"coalesce(i.idx_blks_hit, 0) AS hit, " +
		`coalesce(i.idx_blks_read * (SELECT current_setting('block_size')::int / 1024), 0) AS "read,KiB" ` +
		"FROM pg_stat_{{.ViewType}}_indexes s, pg_statio_{{.ViewType}}_indexes i " +
		"WHERE s.indexrelid = i.indexrelid ORDER BY (s.schemaname ||'.'|| s.relname ||'.'|| s.indexrelname) DESC"

	// PgStatIndexesPG19 defines query for getting indexes' stats on PG 19 and later, including stats_age.
	PgStatIndexesPG19 = "SELECT s.schemaname ||'.'|| s.relname ||'.'|| s.indexrelname AS index, " +
		"coalesce(s.idx_scan, 0) AS scan, coalesce(s.idx_tup_read, 0) AS tuples_read, " +
		"coalesce(s.idx_tup_fetch, 0) AS tuples_fetch, " +
		"coalesce(i.idx_blks_hit, 0) AS hit, " +
		`coalesce(i.idx_blks_read * (SELECT current_setting('block_size')::int / 1024), 0) AS "read,KiB", ` +
		"date_trunc('seconds', now() - s.stats_reset)::text AS stats_age " +
		"FROM pg_stat_{{.ViewType}}_indexes s, pg_statio_{{.ViewType}}_indexes i " +
		"WHERE s.indexrelid = i.indexrelid ORDER BY (s.schemaname ||'.'|| s.relname ||'.'|| s.indexrelname) DESC"
)

// SelectStatIndexesQuery returns the proper query, column count and diff interval for pg_stat_*_indexes based on PG version.
func SelectStatIndexesQuery(version int) (string, int, [2]int) {
	if version >= PostgresV19 {
		return PgStatIndexesPG19, 7, [2]int{1, 5}
	}
	return PgStatIndexesDefault, 6, [2]int{1, 5}
}
