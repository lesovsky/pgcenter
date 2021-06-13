package query

const (
	// PgStatReplicationDefault defines default query for getting replication stats from pg_stat_replication view.
	PgStatReplicationDefault = "SELECT pid AS pid, client_addr AS client, usename AS user, application_name AS name, " +
		`state, sync_state AS mode, ({{.WalFunction1}}({{.WalFunction2}}(),'0/0') / 1024)::bigint AS "wal,KiB", ` +
		`({{.WalFunction1}}({{.WalFunction2}}(),sent_lsn) / 1024)::bigint AS "pending,KiB", ` +
		`({{.WalFunction1}}(sent_lsn,write_lsn) / 1024)::bigint AS "write,KiB", ` +
		`({{.WalFunction1}}(write_lsn,flush_lsn) / 1024)::bigint AS "flush,KiB", ` +
		`({{.WalFunction1}}(flush_lsn,replay_lsn) / 1024)::bigint AS "replay,KiB", ` +
		`({{.WalFunction1}}({{.WalFunction2}}(),replay_lsn))::bigint / 1024 AS "total,KiB", ` +
		"coalesce(date_trunc('seconds', write_lag), '0 seconds'::interval)::text AS write, " +
		"coalesce(date_trunc('seconds', flush_lag), '0 seconds'::interval)::text AS flush, " +
		"coalesce(date_trunc('seconds', replay_lag), '0 seconds'::interval)::text AS replay " +
		"FROM pg_stat_replication ORDER BY pid DESC"

	// PgStatReplicationExtended defines extended query for getting replication stats (including horizon ages) from pg_stat_replication view.
	PgStatReplicationExtended = "SELECT pid AS pid, client_addr AS client, usename AS user, application_name AS name, " +
		`state, sync_state AS mode, ({{.WalFunction1}}({{.WalFunction2}}(),'0/0') / 1024)::bigint AS "wal,KiB", ` +
		`({{.WalFunction1}}({{.WalFunction2}}(),sent_lsn) / 1024)::bigint AS "pending,KiB", ` +
		`({{.WalFunction1}}(sent_lsn,write_lsn) / 1024)::bigint AS "write,KiB", ` +
		`({{.WalFunction1}}(write_lsn,flush_lsn) / 1024)::bigint AS "flush,KiB", ` +
		`({{.WalFunction1}}(flush_lsn,replay_lsn) / 1024)::bigint AS "replay,KiB", ` +
		`({{.WalFunction1}}({{.WalFunction2}}(),replay_lsn) / 1024)::bigint AS "total,KiB", ` +
		"coalesce(date_trunc('seconds', write_lag), '0 seconds'::interval)::text AS write, " +
		"coalesce(date_trunc('seconds', flush_lag), '0 seconds'::interval)::text AS flush, " +
		"coalesce(date_trunc('seconds', replay_lag), '0 seconds'::interval)::text AS replay, " +
		"(pg_last_committed_xact()).xid::text::bigint - backend_xmin::text::bigint as horizon_xacts, " +
		"date_trunc('seconds', (pg_last_committed_xact()).timestamp - pg_xact_commit_timestamp(backend_xmin)) as horizon_age " +
		"FROM pg_stat_replication ORDER BY pid DESC"

	// PgStatReplication96 defines query for getting replication stats from versions for 9.6 and older.
	PgStatReplication96 = "SELECT pid AS pid, client_addr AS client, usename AS user, application_name AS name, " +
		`state, sync_state AS mode, ({{.WalFunction1}}({{.WalFunction2}}(),'0/0') / 1024)::bigint AS "wal,KiB", ` +
		`({{.WalFunction1}}({{.WalFunction2}}(),sent_location) / 1024)::bigint AS "pending,KiB", ` +
		`({{.WalFunction1}}(sent_location,write_location) / 1024)::bigint AS "write,KiB", ` +
		`({{.WalFunction1}}(write_location,flush_location) / 1024)::bigint AS "flush,KiB", ` +
		`({{.WalFunction1}}(flush_location,replay_location) / 1024)::bigint AS "replay,KiB", ` +
		`({{.WalFunction1}}({{.WalFunction2}}(),replay_location))::bigint / 1024 AS "total,KiB" ` +
		"FROM pg_stat_replication ORDER BY pid DESC"

	// PgStatReplication96Extended defines extended query for getting replication stats (including horizon ages) for 9.6 and older.
	PgStatReplication96Extended = "SELECT pid AS pid, client_addr AS client, usename AS user, application_name AS name, " +
		`state, sync_state AS mode, ({{.WalFunction1}}({{.WalFunction2}}(),'0/0') / 1024)::bigint AS "wal,KiB", ` +
		`({{.WalFunction1}}({{.WalFunction2}}(),sent_location) / 1024)::bigint AS "pending,KiB", ` +
		`({{.WalFunction1}}(sent_location,write_location) / 1024)::bigint AS "write,KiB", ` +
		`({{.WalFunction1}}(write_location,flush_location) / 1024)::bigint AS "flush,KiB", ` +
		`({{.WalFunction1}}(flush_location,replay_location) / 1024)::bigint AS "replay,KiB", ` +
		`({{.WalFunction1}}({{.WalFunction2}}(),replay_location))::bigint / 1024 AS "total,KiB", ` +
		"(pg_last_committed_xact()).xid::text::bigint - backend_xmin::text::bigint as horizon_xacts, " +
		"date_trunc('seconds', (pg_last_committed_xact()).timestamp - pg_xact_commit_timestamp(backend_xmin)) as horizon_age " +
		"FROM pg_stat_replication ORDER BY pid DESC"
)

// SelectStatReplicationQuery returns proper query and number of columns depending on Postgres version.
func SelectStatReplicationQuery(version int, track bool) (string, int) {
	switch {
	case version < 100000:
		if track {
			return PgStatReplication96Extended, 14
		}
		return PgStatReplication96, 12
	default:
		if track {
			return PgStatReplicationExtended, 17
		}
		return PgStatReplicationDefault, 15
	}
}
