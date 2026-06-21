package query

const (
	// PgStatReplicationSlots defines query for replication slots statistics (PG 14-18).
	// It is a hybrid of pg_replication_slots (covers both physical and logical slots, plus retained
	// WAL and safe_wal_size) LEFT JOIN-ed with pg_stat_replication_slots (logical-decoding counters),
	// matched by slot_name. retained WAL is computed via the recovery-aware WAL-function template,
	// and byte columns are converted to KiB. The eight logical-decoding counters (spill/stream/total)
	// are wrapped in coalesce(..., 0): physical slots are absent from pg_stat_replication_slots, so the
	// LEFT JOIN yields NULL there, and these columns sit inside the diffed block - without coalesce a
	// physical row would diff an empty string and abort the sample (diffPair -> ParseInt("")).
	// Column layout (0-based): 0 slot_name, 1 slot_type, 2 active, 3 wal_status, 4 retained,KiB,
	// 5 safe,KiB, 6-13 the eight diffed counters, 14 stats_age.
	PgStatReplicationSlots = "SELECT s.slot_name AS slot_name, " +
		"s.slot_type AS slot_type, " +
		"s.active::text AS active, " +
		"s.wal_status AS wal_status, " +
		`({{.WalFunction1}}({{.WalFunction2}}(), s.restart_lsn) / 1024)::bigint AS "retained,KiB", ` +
		`(s.safe_wal_size / 1024)::bigint AS "safe,KiB", ` +
		"coalesce(ss.spill_txns, 0) AS spill_txns, " +
		"coalesce(ss.spill_count, 0) AS spill_count, " +
		`(coalesce(ss.spill_bytes, 0) / 1024)::bigint AS "spill,KiB", ` +
		"coalesce(ss.stream_txns, 0) AS stream_txns, " +
		"coalesce(ss.stream_count, 0) AS stream_count, " +
		`(coalesce(ss.stream_bytes, 0) / 1024)::bigint AS "stream,KiB", ` +
		"coalesce(ss.total_txns, 0) AS total_txns, " +
		`(coalesce(ss.total_bytes, 0) / 1024)::bigint AS "total,KiB", ` +
		"date_trunc('seconds', now() - ss.stats_reset)::text AS stats_age " +
		"FROM pg_replication_slots s " +
		"LEFT JOIN pg_stat_replication_slots ss ON s.slot_name = ss.slot_name " +
		`ORDER BY "retained,KiB" DESC NULLS LAST`
)

// SelectStatReplicationSlotsQuery returns the query, column count and diff interval for replication
// slots statistics. The chosen pg_replication_slots subset and pg_stat_replication_slots are
// schema-stable on PG 14-18, so a single version-independent query is returned and no version branch
// is needed; the unused version parameter is named _ (revive) and kept for signature symmetry with
// SelectStatBgwriterQuery and as an extension point if conflicting/invalidation_reason are added later.
func SelectStatReplicationSlotsQuery(_ int) (string, int, [2]int) {
	return PgStatReplicationSlots, 15, [2]int{6, 13}
}
