package query

const (
	// PgStatIOPG16 defines the count-screen query for pg_stat_io (PG 16-17).
	// One row per backend_type x object x context. Column layout (0-based):
	//   0  io_key      = left(md5(backend_type||object||context),10)  - synthetic row key (Decision 2).
	//                    object/context are coalesced to '' inside md5 so a NULL dimension cannot
	//                    produce a NULL key (NULL would break stat.diff() row matching on column 0).
	//   1  backend_type, 2 object, 3 context - dimensions (absolute, outside DiffIntvl).
	//   4-14 the diffed counter block: reads, read,KiB, writes, write,KiB, extends, ext,KiB,
	//        hits, evictions, writebacks, reuses, fsyncs.
	//   15 stats_age - absolute, outside DiffIntvl.
	// Every diffed column is wrapped in coalesce(...,0): pg_stat_io returns NULL widely (fsyncs for
	// temp relation, reads for background writer); a NULL reaches the diff machinery as an empty
	// string and aborts the whole sample at strconv.ParseInt("") (Decision 5). KiB throughput is
	// derived from op_bytes (a constant block size, NOT a counter) via integer /1024 so the value
	// renders as an integer; op_bytes is itself coalesced in case it is NULL on some row. The
	// count-based WHERE drops all-zero rows so the screen stays compact and the time screen (which
	// shares this WHERE) yields an identical row-set.
	PgStatIOPG16 = "SELECT left(md5(backend_type || coalesce(object,'') || coalesce(context,'')), 10) AS io_key, " +
		"backend_type, object, context, " +
		"coalesce(reads,0) AS reads, " +
		`coalesce(reads,0)*coalesce(op_bytes,0)/1024 AS "read,KiB", ` +
		"coalesce(writes,0) AS writes, " +
		`coalesce(writes,0)*coalesce(op_bytes,0)/1024 AS "write,KiB", ` +
		"coalesce(extends,0) AS extends, " +
		`coalesce(extends,0)*coalesce(op_bytes,0)/1024 AS "ext,KiB", ` +
		"coalesce(hits,0) AS hits, " +
		"coalesce(evictions,0) AS evictions, " +
		"coalesce(writebacks,0) AS writebacks, " +
		"coalesce(reuses,0) AS reuses, " +
		"coalesce(fsyncs,0) AS fsyncs, " +
		"date_trunc('seconds', now() - stats_reset)::text AS stats_age " +
		"FROM pg_stat_io " +
		"WHERE coalesce(reads,0)+coalesce(writes,0)+coalesce(writebacks,0)+coalesce(extends,0)+coalesce(hits,0)+coalesce(evictions,0)+coalesce(reuses,0)+coalesce(fsyncs,0) > 0"

	// PgStatIOPG18 defines the count-screen query for pg_stat_io (PG 18+).
	// Same 16-column layout and DiffIntvl as PG16, but op_bytes was removed and KiB throughput comes
	// from the native cumulative byte counters read_bytes/write_bytes/extend_bytes (type numeric).
	// They are coalesced and integer-divided by 1024 with a ::bigint cast so the numeric source does
	// not render with decimals. PG18 additionally emits object='wal' and context='init' rows - those
	// are extra rows, not extra columns, so the column shape is identical to PG16/17.
	PgStatIOPG18 = "SELECT left(md5(backend_type || coalesce(object,'') || coalesce(context,'')), 10) AS io_key, " +
		"backend_type, object, context, " +
		"coalesce(reads,0) AS reads, " +
		`(coalesce(read_bytes,0)/1024)::bigint AS "read,KiB", ` +
		"coalesce(writes,0) AS writes, " +
		`(coalesce(write_bytes,0)/1024)::bigint AS "write,KiB", ` +
		"coalesce(extends,0) AS extends, " +
		`(coalesce(extend_bytes,0)/1024)::bigint AS "ext,KiB", ` +
		"coalesce(hits,0) AS hits, " +
		"coalesce(evictions,0) AS evictions, " +
		"coalesce(writebacks,0) AS writebacks, " +
		"coalesce(reuses,0) AS reuses, " +
		"coalesce(fsyncs,0) AS fsyncs, " +
		"date_trunc('seconds', now() - stats_reset)::text AS stats_age " +
		"FROM pg_stat_io " +
		"WHERE coalesce(reads,0)+coalesce(writes,0)+coalesce(writebacks,0)+coalesce(extends,0)+coalesce(hits,0)+coalesce(evictions,0)+coalesce(reuses,0)+coalesce(fsyncs,0) > 0"

	// PgStatIOTime defines the time-screen query for pg_stat_io (PG 16-18).
	// The timing column set (read_time/write_time/writeback_time/extend_time/fsync_time) is identical
	// across PG16/17/18, so a single version-independent query is used (the PG18 *_bytes/WAL changes
	// touch only the count screen). Column layout (0-based):
	//   0 io_key, 1 backend_type, 2 object, 3 context,
	//   4-8 the diffed timing block: read_time, write_time, writeback_time, extend_time, fsync_time,
	//   9 stats_age (absolute).
	// Timing columns are NULL unless track_io_timing is on (and per-row for ops that never occur), so
	// each is coalesced to 0 (Decision 5). The WHERE is the SAME count-based filter as the count
	// screen so both sub-views share an identical row-set.
	PgStatIOTime = "SELECT left(md5(backend_type || coalesce(object,'') || coalesce(context,'')), 10) AS io_key, " +
		"backend_type, object, context, " +
		"coalesce(read_time,0) AS read_time, " +
		"coalesce(write_time,0) AS write_time, " +
		"coalesce(writeback_time,0) AS writeback_time, " +
		"coalesce(extend_time,0) AS extend_time, " +
		"coalesce(fsync_time,0) AS fsync_time, " +
		"date_trunc('seconds', now() - stats_reset)::text AS stats_age " +
		"FROM pg_stat_io " +
		"WHERE coalesce(reads,0)+coalesce(writes,0)+coalesce(writebacks,0)+coalesce(extends,0)+coalesce(hits,0)+coalesce(evictions,0)+coalesce(reuses,0)+coalesce(fsyncs,0) > 0"
)

// SelectStatIOQuery returns the count-screen query, column count and diff interval for pg_stat_io
// based on PG version. Cols 4-14 (reads..fsyncs) are diffed; io_key + dimensions (0-3) and stats_age
// (15) are excluded. PG18 uses native read_bytes/write_bytes/extend_bytes; PG16/17 derive KiB from
// op_bytes. Versions < PG16 (where pg_stat_io does not exist) still get the PG16 form - the
// MinRequiredVersion gate lives at the view layer, and the SQL must stay Format-safe regardless.
func SelectStatIOQuery(version int) (string, int, [2]int) {
	if version >= PostgresV18 {
		return PgStatIOPG18, 16, [2]int{4, 14}
	}
	return PgStatIOPG16, 16, [2]int{4, 14}
}

// SelectStatIOTimeQuery returns the time-screen query, column count and diff interval for pg_stat_io.
// The timing column set is schema-stable across PG16/17/18, so a single version-independent query is
// returned and the version parameter is unused (named _ per revive); it is kept for signature
// symmetry with SelectStatIOQuery and the other selectors. Cols 4-8 (read_time..fsync_time) are
// diffed; io_key + dimensions (0-3) and stats_age (9) are excluded.
func SelectStatIOTimeQuery(_ int) (string, int, [2]int) {
	return PgStatIOTime, 10, [2]int{4, 8}
}
