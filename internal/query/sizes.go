package query

const (
	// PgTablesSizesDefault is the default query for getting stats related to tables' sizes.
	PgTablesSizesDefault = "SELECT s.schemaname ||'.'|| s.relname AS relation," +
		"(SELECT count(*) FROM pg_index i WHERE i.indrelid = s.relid) AS n_indexes," +
		"pg_total_relation_size(s.relid) / 1024 AS total_size," +
		"pg_relation_size(s.relid, 'main') / 1024 AS rel_main_size," +
		"(pg_relation_size(s.relid, 'fsm') + pg_relation_size(s.relid, 'vm') + pg_relation_size(s.relid, 'init')) / 1024 AS rel_meta_size," +
		"coalesce(pg_relation_size((SELECT reltoastrelid FROM pg_class c WHERE c.oid = s.relid )) / 1024, 0) AS toast_size," +
		"pg_indexes_size(s.relid) / 1024 AS idx_size," +
		"pg_total_relation_size(s.relid) / 1024 AS total_change," +
		"pg_relation_size(s.relid, 'main') / 1024 AS rel_main_change," +
		"(pg_relation_size(s.relid, 'fsm') + pg_relation_size(s.relid, 'vm') + pg_relation_size(s.relid, 'init')) / 1024 AS rel_meta_change," +
		"coalesce(pg_relation_size((SELECT reltoastrelid FROM pg_class c WHERE c.oid = s.relid )) / 1024, 0) AS toast_change," +
		"pg_indexes_size(s.relid) / 1024 AS idx_change " +
		"FROM pg_stat_{{.ViewType}}_tables s, pg_class c " +
		"WHERE s.relid = c.oid AND NOT EXISTS (SELECT 1 FROM pg_locks WHERE relation = s.relid AND mode = 'AccessExclusiveLock' AND granted) " +
		"ORDER BY (s.schemaname || '.' || s.relname) DESC"
)
