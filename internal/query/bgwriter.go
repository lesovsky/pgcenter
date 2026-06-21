package query

const (
	// PgStatBgwriterPG14 defines query for pg_stat_bgwriter (PG 14-16).
	// pg_stat_checkpointer does not exist before PG17; all columns come from pg_stat_bgwriter.
	// Event counters (ckpt_timed, ckpt_req) sit before the diffed block to render as absolute values.
	PgStatBgwriterPG14 = "SELECT 'Bgwriter' AS source, " +
		"checkpoints_timed AS ckpt_timed, checkpoints_req AS ckpt_req, " +
		`checkpoint_write_time AS "ckpt_write,ms", checkpoint_sync_time AS "ckpt_sync,ms", ` +
		"buffers_checkpoint AS buf_ckpt, buffers_clean AS buf_clean, maxwritten_clean AS maxwritten, " +
		"buffers_backend AS buf_backend, buffers_backend_fsync AS buf_backend_fsync, buffers_alloc AS buf_alloc, " +
		"date_trunc('seconds', now() - stats_reset)::text AS stats_age " +
		"FROM pg_stat_bgwriter"

	// PgStatBgwriterPG17 defines query for pg_stat_bgwriter + pg_stat_checkpointer (PG 17).
	// checkpoint/restartpoint counters moved to pg_stat_checkpointer; bgwriter retains buffers_clean,
	// maxwritten_clean, buffers_alloc. The two single-row views are cross-joined. stats_age is taken
	// from the checkpointer's stats_reset (Decision 4).
	PgStatBgwriterPG17 = "SELECT 'Bgwriter' AS source, " +
		"num_timed AS ckpt_timed, num_requested AS ckpt_req, " +
		"restartpoints_timed AS rstpt_timed, restartpoints_req AS rstpt_req, restartpoints_done AS rstpt_done, " +
		`write_time AS "ckpt_write,ms", sync_time AS "ckpt_sync,ms", ` +
		"buffers_written AS buf_ckpt, buffers_clean AS buf_clean, maxwritten_clean AS maxwritten, buffers_alloc AS buf_alloc, " +
		"date_trunc('seconds', now() - pg_stat_checkpointer.stats_reset)::text AS stats_age " +
		"FROM pg_stat_bgwriter, pg_stat_checkpointer"

	// PgStatBgwriterPG18 defines query for pg_stat_bgwriter + pg_stat_checkpointer (PG 18+).
	// As PG17 plus the diffed slru_written column (SLRU buffers written during checkpoints),
	// inserted into the diffed block next to buf_ckpt.
	PgStatBgwriterPG18 = "SELECT 'Bgwriter' AS source, " +
		"num_timed AS ckpt_timed, num_requested AS ckpt_req, " +
		"restartpoints_timed AS rstpt_timed, restartpoints_req AS rstpt_req, restartpoints_done AS rstpt_done, " +
		`write_time AS "ckpt_write,ms", sync_time AS "ckpt_sync,ms", ` +
		"buffers_written AS buf_ckpt, slru_written, buffers_clean AS buf_clean, maxwritten_clean AS maxwritten, buffers_alloc AS buf_alloc, " +
		"date_trunc('seconds', now() - pg_stat_checkpointer.stats_reset)::text AS stats_age " +
		"FROM pg_stat_bgwriter, pg_stat_checkpointer"
)

// SelectStatBgwriterQuery returns the proper query, column count and diff interval for
// pg_stat_bgwriter (+ pg_stat_checkpointer on PG17+) based on PG version.
func SelectStatBgwriterQuery(version int) (string, int, [2]int) {
	if version >= 180000 {
		// PG18: cols 6-12 diffed (incl. slru_written); ckpt/rstpt counters and stats_age excluded.
		return PgStatBgwriterPG18, 14, [2]int{6, 12}
	}
	if version >= 170000 {
		// PG17: cols 6-11 diffed; ckpt/rstpt counters (1-5) and stats_age (12) excluded.
		return PgStatBgwriterPG17, 13, [2]int{6, 11}
	}
	// PG14-16: cols 3-10 diffed; ckpt counters (1-2) and stats_age (11) excluded.
	return PgStatBgwriterPG14, 12, [2]int{3, 10}
}
