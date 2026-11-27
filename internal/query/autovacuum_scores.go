package query

const (
	// PgStatAutovacuumScoresDefault defines the PG 19 autovacuum queue. The outer scores relation keeps
	// rows without a pg_stat_all_tables partner; the schema filter must therefore use s.schemaname.
	PgStatAutovacuumScoresDefault = "SELECT s.schemaname || '.' || s.relname AS relation, " +
		"round(s.score::numeric, 2) AS score, " +
		"s.do_vacuum::text AS do_vacuum, " +
		"s.do_analyze::text AS do_analyze, " +
		"s.for_wraparound::text AS for_wraparound, " +
		"a.n_dead_tup AS dead_total, " +
		"round(s.xid_score::numeric, 2) AS xid_score, " +
		"round(s.mxid_score::numeric, 2) AS mxid_score, " +
		"round(s.vacuum_score::numeric, 2) AS vacuum_score, " +
		"round(s.vacuum_insert_score::numeric, 2) AS vacuum_insert_score, " +
		"round(s.analyze_score::numeric, 2) AS analyze_score " +
		"FROM pg_stat_autovacuum_scores s " +
		"LEFT JOIN pg_stat_all_tables a ON s.relid = a.relid " +
		`{{if ne .ViewType "all"}}WHERE (s.schemaname NOT IN ('pg_catalog', 'pg_toast', 'information_schema') OR s.for_wraparound) {{end}}` +
		"ORDER BY s.score DESC"
)

// SelectStatAutovacuumScoresQuery returns the single PG 19 layout. The view registry gates older
// versions; the unused version parameter is retained for selector-signature symmetry.
func SelectStatAutovacuumScoresQuery(_ int) (string, int, [2]int) {
	return PgStatAutovacuumScoresDefault, 11, [2]int{0, 0}
}
