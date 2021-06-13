package query

const (
	// PgStatWALDefault defines default query for getting WAL stats from pg_stat_wal.
	PgStatWALDefault = "SELECT 'WAL' AS source, " +
		"(SELECT pg_size_pretty(count(1) * pg_size_bytes(current_setting('wal_segment_size'))) AS waldir_size  FROM pg_ls_waldir()) AS waldir_size, " +
		`round(wal_bytes / 1024, 2) AS "wal,KiB", ` +
		"wal_records AS records, wal_fpi AS fpi, " +
		`wal_write AS write, wal_sync AS sync, wal_write_time AS "write,ms", wal_sync_time AS "sync,ms", wal_buffers_full AS buffers_full, ` +
		"date_trunc('seconds', now() - stats_reset)::text AS stats_age " +
		"FROM pg_stat_wal"
)
