package query

const (
	// PgStatWALDefault defines query for pg_stat_wal (PG 14-17).
	PgStatWALDefault = "SELECT 'WAL' AS source, " +
		"(SELECT pg_size_pretty(count(1) * pg_size_bytes(current_setting('wal_segment_size'))) AS waldir_size  FROM pg_ls_waldir()) AS waldir_size, " +
		`round(wal_bytes / 1024, 2) AS "wal,KiB", ` +
		"wal_records AS records, wal_fpi AS fpi, " +
		`wal_write AS write, wal_sync AS sync, wal_write_time AS "write,ms", wal_sync_time AS "sync,ms", wal_buffers_full AS buffers_full, ` +
		"date_trunc('seconds', now() - stats_reset)::text AS stats_age " +
		"FROM pg_stat_wal"

	// PgStatWALPG18 defines query for pg_stat_wal (PG 18+).
	// wal_write, wal_sync, wal_write_time, wal_sync_time removed in PG 18.
	PgStatWALPG18 = "SELECT 'WAL' AS source, " +
		"(SELECT pg_size_pretty(count(1) * pg_size_bytes(current_setting('wal_segment_size'))) AS waldir_size  FROM pg_ls_waldir()) AS waldir_size, " +
		`round(wal_bytes / 1024, 2) AS "wal,KiB", ` +
		"wal_records AS records, wal_fpi AS fpi, " +
		"wal_buffers_full AS buffers_full, " +
		"date_trunc('seconds', now() - stats_reset)::text AS stats_age " +
		"FROM pg_stat_wal"
)

// SelectStatWALQuery returns the proper query and column count for pg_stat_wal based on PG version.
func SelectStatWALQuery(version int) (string, int) {
	if version >= 180000 {
		return PgStatWALPG18, 7
	}
	return PgStatWALDefault, 11
}
