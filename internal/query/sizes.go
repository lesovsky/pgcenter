package query

const (
	// PgTablesSizesDefault defines default query for getting stats related to tables' sizes.
	PgTablesSizesDefault = "SELECT s.schemaname ||'.'|| s.relname AS relation," +
		"(SELECT count(*) FROM pg_index i WHERE i.indrelid = s.relid) AS indexes_total," +
		`pg_total_relation_size(s.relid) / 1024 AS "size_total,KiB",` +
		`pg_relation_size(s.relid, 'main') / 1024 AS "table_total,KiB",` +
		`(pg_relation_size(s.relid, 'fsm') + pg_relation_size(s.relid, 'vm') + pg_relation_size(s.relid, 'init')) / 1024 AS "meta_total,KiB",` +
		`coalesce(pg_relation_size((SELECT reltoastrelid FROM pg_class c WHERE c.oid = s.relid )) / 1024, 0) AS "toast_total,KiB",` +
		`pg_indexes_size(s.relid) / 1024 AS "indexes_total,KiB",` +
		`pg_total_relation_size(s.relid) / 1024 AS "size,KiB",` +
		`pg_relation_size(s.relid, 'main') / 1024 AS "table,KiB",` +
		`(pg_relation_size(s.relid, 'fsm') + pg_relation_size(s.relid, 'vm') + pg_relation_size(s.relid, 'init')) / 1024 AS "meta,KiB",` +
		`coalesce(pg_relation_size((SELECT reltoastrelid FROM pg_class c WHERE c.oid = s.relid )) / 1024, 0) AS "toast,KiB",` +
		`pg_indexes_size(s.relid) / 1024 AS "indexes,KiB" ` +
		"FROM pg_stat_{{.ViewType}}_tables s, pg_class c " +
		"WHERE s.relid = c.oid AND NOT EXISTS (SELECT 1 FROM pg_locks WHERE relation = s.relid AND mode = 'AccessExclusiveLock' AND granted) " +
		"ORDER BY (s.schemaname || '.' || s.relname) DESC"
)
