package query

const (
	// PgStatArchiverDefault defines query for pg_stat_archiver plus the .ready archiving backlog.
	// One row, 9 columns, layout (0-based):
	//   0 source        - literal 'Archiver', the stable row identity across samples.
	//   1 ready         - count of *.ready files in the archive status directory.
	//   2 archived      - archived_count, cumulative.
	//   3 last_archived - last_archived_wal, NULL until something is archived.
	//   4 archived_age  - age of last_archived_time, NULL until something is archived.
	//   5 failed        - failed_count, cumulative.
	//   6 last_failed   - last_failed_wal, NULL until something fails.
	//   7 failed_age    - age of last_failed_time, NULL until something fails.
	//   8 stats_age     - age of stats_reset, never NULL.
	//
	// There is no version branch: pg_stat_archiver is schema-identical on PG 14 through PG 19
	// (verified against live pg_attribute on 14/17/18/19), and pg_ls_archive_statusdir() exists on
	// every one of them - so one query text serves all supported versions.
	//
	// The ready sub-select calls pg_ls_archive_statusdir(), which is superuser + pg_monitor only -
	// the same privilege class as the pg_ls_waldir() the wal screen already calls unconditionally.
	// A role without pg_monitor therefore loses the WHOLE screen, by design (Decision 4): PostgreSQL
	// checks EXECUTE at function-node initialisation, so hiding the call behind
	// has_function_privilege() was measured not to work - CASE with an uncorrelated subquery, CASE
	// with a correlated subquery and LEFT JOIN LATERAL all fail alike.
	//
	// Nothing here is diffed (the selector returns DiffIntvl{0,0}), so calculateDelta
	// short-circuits before diff() and the four NULL-able columns never reach strconv.ParseInt("").
	// That is what makes them safe WITHOUT coalesce, and a blank cell is the honest rendering of
	// "this cluster has never archived" (Decision 3; precedent ADR [013] backend_xid). The literal
	// at column 0 is safe for the same reason.
	//
	// The two server-supplied WAL-name columns are rendered as-is, with no escape sanitisation
	// (Decision 16): PostgreSQL only records names that passed its own VALID_XFN_CHARS filter (hex
	// digits plus the .history/.backup/.partial suffixes), a set containing no ESC and no control
	// characters, so these columns cannot carry a terminal escape sequence even if an operator
	// hand-places a bogus .ready file. Tech-debt [029] is neither widened nor closed here.
	PgStatArchiverDefault = "SELECT 'Archiver' AS source, " +
		"(SELECT count(*) FILTER (WHERE name LIKE '%.ready') FROM pg_ls_archive_statusdir()) AS ready, " +
		"archived_count AS archived, " +
		"last_archived_wal AS last_archived, " +
		"date_trunc('seconds', now() - last_archived_time)::text AS archived_age, " +
		"failed_count AS failed, " +
		"last_failed_wal AS last_failed, " +
		"date_trunc('seconds', now() - last_failed_time)::text AS failed_age, " +
		"date_trunc('seconds', now() - stats_reset)::text AS stats_age " +
		"FROM pg_stat_archiver"
)

// SelectStatArchiverQuery returns the query, column count and diff interval for the archiver screen.
// pg_stat_archiver is schema-stable across every supported version, so a single version-independent
// query is returned and the version parameter is unused (named _ per revive); it is kept for
// signature symmetry with SelectStatWALQuery and the other selectors. DiffIntvl{0,0} is not an unset
// placeholder - it states that nothing is diffed: both counters and the .ready backlog render as
// absolute values, which is what an operator cross-checks against the PostgreSQL log during an
// incident.
func SelectStatArchiverQuery(_ int) (string, int, [2]int) {
	return PgStatArchiverDefault, 9, [2]int{0, 0}
}
