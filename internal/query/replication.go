package query

const (
	// PgStatReplicationDefault is the default query for getting replication stats from pg_stat_replication view
	// { Name: "pg_stat_replication", Query: common.PgStatReplicationQueryDefault, DiffIntvl: [2]int{6,6}, Ncols: 15, OrderKey: 0, OrderDesc: true }
	PgStatReplicationDefault = "SELECT pid AS pid, client_addr AS client, usename AS user, application_name AS name, " +
		"state, sync_state AS mode, ({{.WalFunction1}}({{.WalFunction2}}(),'0/0') / 1024)::bigint AS wal, " +
		"({{.WalFunction1}}({{.WalFunction2}}(),sent_lsn) / 1024)::bigint AS pending, " +
		"({{.WalFunction1}}(sent_lsn,write_lsn) / 1024)::bigint AS write, " +
		"({{.WalFunction1}}(write_lsn,flush_lsn) / 1024)::bigint AS flush, " +
		"({{.WalFunction1}}(flush_lsn,replay_lsn) / 1024)::bigint AS replay, " +
		"({{.WalFunction1}}({{.WalFunction2}}(),replay_lsn))::bigint / 1024 AS total_lag, " +
		"coalesce(date_trunc('seconds', write_lag), '0 seconds'::interval)::text AS write_lag, " +
		"coalesce(date_trunc('seconds', flush_lag), '0 seconds'::interval)::text AS flush_lag, " +
		"coalesce(date_trunc('seconds', replay_lag), '0 seconds'::interval)::text AS replay_lag " +
		"FROM pg_stat_replication ORDER BY pid DESC"

	// PgStatReplicationExtended is the extended query for getting replication stats from pg_stat_replication view
	// { Name: "pg_stat_replication", Query: common.PgStatReplicationQueryExtended, DiffIntvl: [2]int{6,6}, Ncols: 17, OrderKey: 0, OrderDesc: true }
	PgStatReplicationExtended = "SELECT pid AS pid, client_addr AS client,  usename AS user, application_name AS name, " +
		"state, sync_state AS mode, ({{.WalFunction1}}({{.WalFunction2}}(),'0/0') / 1024)::bigint AS wal, " +
		"({{.WalFunction1}}({{.WalFunction2}}(),sent_lsn) / 1024)::bigint AS pending, " +
		"({{.WalFunction1}}(sent_lsn,write_lsn) / 1024)::bigint AS write, " +
		"({{.WalFunction1}}(write_lsn,flush_lsn) / 1024)::bigint AS flush, " +
		"({{.WalFunction1}}(flush_lsn,replay_lsn) / 1024)::bigint AS replay, " +
		"({{.WalFunction1}}({{.WalFunction2}}(),replay_lsn) / 1024)::bigint AS total_lag, " +
		"coalesce(date_trunc('seconds', write_lag), '0 seconds'::interval)::text AS write_lag, " +
		"coalesce(date_trunc('seconds', flush_lag), '0 seconds'::interval)::text AS flush_lag, " +
		"coalesce(date_trunc('seconds', replay_lag), '0 seconds'::interval)::text AS replay_lag, " +
		"(pg_last_committed_xact()).xid::text::bigint - backend_xmin::text::bigint as xact_age, " +
		"date_trunc('seconds', (pg_last_committed_xact()).timestamp - pg_xact_commit_timestamp(backend_xmin)) as time_age " +
		"FROM pg_stat_replication ORDER BY pid DESC"

	// PgStatReplication96 is the query for getting replication stats from versions for 9.5 and older
	// { Name: "pg_stat_replication", Query: common.PgStatReplicationQuery96, DiffIntvl: [2]int{6,6}, Ncols: 12, OrderKey: 0, OrderDesc: true }
	PgStatReplication96 = "SELECT pid AS pid, client_addr AS client, usename AS user, application_name AS name, " +
		"state, sync_state AS mode, ({{.WalFunction1}}({{.WalFunction2}}(),'0/0') / 1024)::bigint AS wal, " +
		"({{.WalFunction1}}({{.WalFunction2}}(),sent_location) / 1024)::bigint AS pending, " +
		"({{.WalFunction1}}(sent_location,write_location) / 1024)::bigint AS write, " +
		"({{.WalFunction1}}(write_location,flush_location) / 1024)::bigint AS flush, " +
		"({{.WalFunction1}}(flush_location,replay_location) / 1024)::bigint AS replay, " +
		"({{.WalFunction1}}({{.WalFunction2}}(),replay_location))::bigint / 1024 AS total_lag " +
		"FROM pg_stat_replication ORDER BY pid DESC"

	// PgStatReplication96Extended is the extended query for getting replication stats for 9.6 and older
	// { Name: "pg_stat_replication", Query: common.PgStatReplicationQuery96Extended, DiffIntvl: [2]int{6,6}, Ncols: 14, OrderKey: 0, OrderDesc: true }
	PgStatReplication96Extended = "SELECT pid AS pid, client_addr AS client, usename AS user, application_name AS name, " +
		"state, sync_state AS mode, ({{.WalFunction1}}({{.WalFunction2}}(),'0/0') / 1024)::bigint AS wal, " +
		"({{.WalFunction1}}({{.WalFunction2}}(),sent_location) / 1024)::bigint AS pending, " +
		"({{.WalFunction1}}(sent_location,write_location) / 1024)::bigint AS write, " +
		"({{.WalFunction1}}(write_location,flush_location) / 1024)::bigint AS flush, " +
		"({{.WalFunction1}}(flush_location,replay_location) / 1024)::bigint AS replay, " +
		"({{.WalFunction1}}({{.WalFunction2}}(),replay_location))::bigint / 1024 AS total_lag, " +
		"(pg_last_committed_xact()).xid::text::bigint - backend_xmin::text::bigint as xact_age, " +
		"date_trunc('seconds', (pg_last_committed_xact()).timestamp - pg_xact_commit_timestamp(backend_xmin)) as time_age " +
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
