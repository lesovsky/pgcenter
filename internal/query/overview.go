package query

// This file defines the aggregate queries backing the verbose pgstat panel rows (Task 5):
// workload, databases, workers, replication and (via SelectStatBgwriterQuery) bgwr/ckpt.
//
// Each row is a single-row aggregate scanned into a flat struct by collectOverviewStat
// (internal/stat/postgres.go). Privileged/expensive aggregates (archiving backlog, sum of
// database sizes) run as their own QueryRow so a privilege error (42501) or archive_mode=off
// degrades just that row to n/a without aborting the sample. Raw PG error text (which may
// contain filesystem paths) is never surfaced.
//
// Template queries ({{.WalFunction1}}/{{.WalFunction2}}) MUST be passed through Format(tmpl, opts)
// with opts built by NewOptions so the recovery-aware WAL function names are substituted.

const (
	// OverviewWorkload aggregates per-database workload counters from pg_stat_database.
	// All columns are absolute cumulative counters; rates are computed Go-side against the prev
	// snapshot. tps = commits + rollbacks; ins/upd/del/ret/tmp are per-second; the three others
	// counters (deadlocks/conflicts/checksum_failures) are summed and shown per-interval.
	// Column layout (0-based): 0 commits, 1 rollbacks, 2 inserts, 3 updates, 4 deletes,
	// 5 returned, 6 temp_files, 7 deadlocks, 8 conflicts, 9 checksum_failures.
	OverviewWorkload = "SELECT " +
		"coalesce(sum(xact_commit), 0)::bigint AS commits, " +
		"coalesce(sum(xact_rollback), 0)::bigint AS rollbacks, " +
		"coalesce(sum(tup_inserted), 0)::bigint AS inserts, " +
		"coalesce(sum(tup_updated), 0)::bigint AS updates, " +
		"coalesce(sum(tup_deleted), 0)::bigint AS deletes, " +
		"coalesce(sum(tup_returned), 0)::bigint AS returned, " +
		"coalesce(sum(temp_files), 0)::bigint AS temp_files, " +
		"coalesce(sum(deadlocks), 0)::bigint AS deadlocks, " +
		"coalesce(sum(conflicts), 0)::bigint AS conflicts, " +
		"coalesce(sum(checksum_failures), 0)::bigint AS checksum_failures " +
		"FROM pg_stat_database"

	// OverviewDatabases aggregates the cheap, always-available database signals: database count and
	// the cache counters. blks_hit/blks_read are absolute cumulative counters; the cache hit ratio is
	// computed Go-side per-interval (Δhit / Δ(hit+read)). This runs separately from the expensive
	// size sum (OverviewDatabasesSize) so a size-aggregate failure cannot blank count/cache-hit
	// (Decision 6/10). Column layout (0-based): 0 count, 1 hits, 2 reads.
	OverviewDatabases = "SELECT " +
		"count(*)::bigint AS count, " +
		"coalesce(sum(blks_hit), 0)::bigint AS hits, " +
		"coalesce(sum(blks_read), 0)::bigint AS reads " +
		"FROM pg_stat_database WHERE datname IS NOT NULL"

	// OverviewDatabasesSize sums the on-disk size of all databases. pg_database_size is expensive and
	// can error for databases the role cannot access, so it runs as its OWN QueryRow: a failure
	// degrades only the size/growth field to n/a (TotalSizeValid=false), leaving count and cache-hit
	// intact (Decision 6/10). Single column.
	OverviewDatabasesSize = "SELECT " +
		"coalesce(sum(pg_database_size(datname)), 0)::bigint AS total_size " +
		"FROM pg_stat_database WHERE datname IS NOT NULL"

	// OverviewWorkers reports background-worker pool occupancy from pg_stat_activity by backend_type.
	// The umbrella (max_worker_processes) occupancy is the count of all background-worker backends;
	// logical and parallel are the specific subsets. Limits come from GUCs (PostgresProperties).
	// Column layout (0-based): 0 umbrella_active, 1 logical_active, 2 parallel_active.
	OverviewWorkers = "SELECT " +
		"count(*) FILTER (WHERE backend_type IN ('background worker', 'parallel worker', 'logical replication worker'))::int AS umbrella_active, " +
		"count(*) FILTER (WHERE backend_type = 'logical replication worker')::int AS logical_active, " +
		"count(*) FILTER (WHERE backend_type = 'parallel worker')::int AS parallel_active " +
		"FROM pg_stat_activity"

	// OverviewWalSize reports the size of the pg_wal directory in bytes (lifted from wal.go's
	// waldir subselect, returning raw bytes instead of pg_size_pretty). Single column.
	OverviewWalSize = "SELECT coalesce((SELECT count(1) * pg_size_bytes(current_setting('wal_segment_size')) FROM pg_ls_waldir()), 0)::bigint AS wal_size"

	// OverviewReplicationLag reports the worst-case replication lag in bytes across all standbys
	// (max of WalFunction1(WalFunction2(), replay_lsn)). On a primary with no standbys the aggregate
	// over an empty set yields NULL, scanned via sql.NullInt64 -> n/a. Template query.
	// Single column.
	OverviewReplicationLag = "SELECT " +
		"max({{.WalFunction1}}({{.WalFunction2}}(), replay_lsn))::bigint AS lag_bytes " +
		"FROM pg_stat_replication"

	// OverviewReplicationSlots reports slot count and the largest retained WAL in bytes across all
	// replication slots (retained = WalFunction1(WalFunction2(), s.restart_lsn)). With no slots the
	// max yields NULL, scanned via sql.NullInt64 -> n/a for the retained part. Template query.
	// Column layout (0-based): 0 slots_count, 1 retained_bytes.
	OverviewReplicationSlots = "SELECT " +
		"count(*)::bigint AS slots_count, " +
		"max({{.WalFunction1}}({{.WalFunction2}}(), s.restart_lsn))::bigint AS retained_bytes " +
		"FROM pg_replication_slots s"

	// OverviewSendRecv reports the number of active WAL senders and receivers from pg_stat_activity
	// by backend_type. Column layout (0-based): 0 senders, 1 receivers.
	OverviewSendRecv = "SELECT " +
		"count(*) FILTER (WHERE backend_type LIKE 'walsender%')::int AS senders, " +
		"count(*) FILTER (WHERE backend_type = 'walreceiver')::int AS receivers " +
		"FROM pg_stat_activity"

	// OverviewArchivingBacklog reports the WAL archiving backlog in bytes:
	// count(*.ready in pg_wal/archive_status) * wal_segment_size. This adapts the wal.go precedent
	// count(1) * pg_size_bytes(current_setting('wal_segment_size')), replacing pg_ls_waldir() with
	// pg_ls_dir('pg_wal/archive_status') filtered on the .ready suffix.
	//
	// pg_ls_dir requires pg_monitor/superuser; this query MUST be run as its OWN QueryRow so a 42501
	// privilege error (or archive_mode=off) degrades only the archiving-backlog field to n/a without
	// aborting the sample. The raw error (containing the path) must never be surfaced. Single column.
	OverviewArchivingBacklog = "SELECT " +
		"count(*) FILTER (WHERE name LIKE '%.ready') * pg_size_bytes(current_setting('wal_segment_size')) AS backlog " +
		"FROM pg_ls_dir('pg_wal/archive_status') AS name"
)
