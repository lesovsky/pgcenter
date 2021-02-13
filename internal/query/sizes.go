package query

const (
	// PgTablesSizesDefault is the defaulr query for getting stats related to tables' sizes
	// { Name: "pg_tables_sizes", Query: common.PgTablesSizesQueryDefault, DiffIntvl: [2]int{4,6}, Ncols: 7, OrderKey: 0, OrderDesc: true }
	PgTablesSizesDefault = "SELECT s.schemaname ||'.'|| s.relname AS relation, " +
		"pg_total_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024 AS total_size, " +
		"pg_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024 AS rel_size, " +
		"(pg_total_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024) - " +
		"(pg_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024) AS idx_size, " +
		"pg_total_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024 AS total_change, " +
		"pg_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024 AS rel_change, " +
		"(pg_total_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024) - " +
		"(pg_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024) AS idx_change " +
		"FROM pg_stat_{{.ViewType}}_tables s, pg_class c " +
		"WHERE s.relid = c.oid AND NOT EXISTS (SELECT 1 FROM pg_locks WHERE relation = s.relid AND mode = 'AccessExclusiveLock' and granted) " +
		"ORDER BY (s.schemaname || '.' || s.relname) DESC"
)
