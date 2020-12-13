package stat

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/view"
	"github.com/stretchr/testify/assert"
	"testing"
)

// Test_StatQueries is the integration test against all supported Postgres versions (from 9.5 to 13).
// Test run all views' queries and checks number of returned columns.
func Test_StatQueries(t *testing.T) {
	testcases := []struct {
		version     int
		trackCommit string
		viewname    string
		want        []string
	}{
		// *** pg_stat_activity ***
		{
			version: 130000, viewname: "activity",
			want: []string{"pid", "cl_addr", "cl_port", "datname", "usename", "appname", "backend_type", "wait_etype", "wait_event", "state", "xact_age", "query_age", "change_age", "query"},
		},
		{
			version: 120000, viewname: "activity",
			want: []string{"pid", "cl_addr", "cl_port", "datname", "usename", "appname", "backend_type", "wait_etype", "wait_event", "state", "xact_age", "query_age", "change_age", "query"},
		},
		{
			version: 110000, viewname: "activity",
			want: []string{"pid", "cl_addr", "cl_port", "datname", "usename", "appname", "backend_type", "wait_etype", "wait_event", "state", "xact_age", "query_age", "change_age", "query"},
		},
		{
			version: 100000, viewname: "activity",
			want: []string{"pid", "cl_addr", "cl_port", "datname", "usename", "appname", "backend_type", "wait_etype", "wait_event", "state", "xact_age", "query_age", "change_age", "query"},
		},
		{
			version: 90600, viewname: "activity",
			want: []string{"pid", "cl_addr", "cl_port", "datname", "usename", "appname", "wait_etype", "wait_event", "state", "xact_age", "query_age", "change_age", "query"},
		},
		{
			version: 90500, viewname: "activity",
			want: []string{"pid", "cl_addr", "cl_port", "datname", "usename", "appname", "waiting", "state", "xact_age", "query_age", "change_age", "query"},
		},
		// *** pg_stat_databases ***
		{
			version: 130000, viewname: "databases",
			want: []string{"datname", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes", "conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age"},
		},
		{
			version: 120000, viewname: "databases",
			want: []string{"datname", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes", "conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age"},
		},
		{
			version: 110000, viewname: "databases",
			want: []string{"datname", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes", "conflicts", "deadlocks", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age"},
		},
		{
			version: 100000, viewname: "databases",
			want: []string{"datname", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes", "conflicts", "deadlocks", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age"},
		},
		{
			version: 90600, viewname: "databases",
			want: []string{"datname", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes", "conflicts", "deadlocks", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age"},
		},
		{
			version: 90500, viewname: "databases",
			want: []string{"datname", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes", "conflicts", "deadlocks", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age"},
		},
		// *** pg_stat_user_functions ***
		{
			version: 130000, viewname: "functions",
			want: []string{"funcid", "function", "total_calls", "calls", "total_t", "self_t", "avg_t", "avg_self_t"},
		},
		{
			version: 120000, viewname: "functions",
			want: []string{"funcid", "function", "total_calls", "calls", "total_t", "self_t", "avg_t", "avg_self_t"},
		},
		{
			version: 110000, viewname: "functions",
			want: []string{"funcid", "function", "total_calls", "calls", "total_t", "self_t", "avg_t", "avg_self_t"},
		},
		{
			version: 100000, viewname: "functions",
			want: []string{"funcid", "function", "total_calls", "calls", "total_t", "self_t", "avg_t", "avg_self_t"},
		},
		{
			version: 90600, viewname: "functions",
			want: []string{"funcid", "function", "total_calls", "calls", "total_t", "self_t", "avg_t", "avg_self_t"},
		},
		{
			version: 90500, viewname: "functions",
			want: []string{"funcid", "function", "total_calls", "calls", "total_t", "self_t", "avg_t", "avg_self_t"},
		},
		// *** pg_stat_all_indexes ***
		{
			version: 130000, viewname: "indexes",
			want: []string{"index", "idx_scan", "idx_tup_read", "idx_tup_fetch", "idx_read", "idx_hit"},
		},
		{
			version: 120000, viewname: "indexes",
			want: []string{"index", "idx_scan", "idx_tup_read", "idx_tup_fetch", "idx_read", "idx_hit"},
		},
		{
			version: 110000, viewname: "indexes",
			want: []string{"index", "idx_scan", "idx_tup_read", "idx_tup_fetch", "idx_read", "idx_hit"},
		},
		{
			version: 100000, viewname: "indexes",
			want: []string{"index", "idx_scan", "idx_tup_read", "idx_tup_fetch", "idx_read", "idx_hit"},
		},
		{
			version: 90600, viewname: "indexes",
			want: []string{"index", "idx_scan", "idx_tup_read", "idx_tup_fetch", "idx_read", "idx_hit"},
		},
		{
			version: 90500, viewname: "indexes",
			want: []string{"index", "idx_scan", "idx_tup_read", "idx_tup_fetch", "idx_read", "idx_hit"},
		},
		// *** pg_stat_progress_cluster ***
		{
			version: 130000, viewname: "progress_cluster",
			want: []string{"pid", "xact_age", "datname", "relation", "index", "state", "waiting", "phase", "t_size", "scanned_%", "tup_scanned", "tup_written", "query"},
		},
		{
			version: 120000, viewname: "progress_cluster",
			want: []string{"pid", "xact_age", "datname", "relation", "index", "state", "waiting", "phase", "t_size", "scanned_%", "tup_scanned", "tup_written", "query"},
		},
		// *** pg_stat_progress_indexes ***
		{
			version: 130000, viewname: "progress_index",
			want: []string{"pid", "xact_age", "datname", "relation", "index", "state", "waiting", "phase", "locker_pid", "lockers", "size_total/done_%", "tup_total/done_%", "parts_total/done_%", "query"},
		},
		{
			version: 120000, viewname: "progress_index",
			want: []string{"pid", "xact_age", "datname", "relation", "index", "state", "waiting", "phase", "locker_pid", "lockers", "size_total/done_%", "tup_total/done_%", "parts_total/done_%", "query"},
		},
		// *** pg_stat_progress_vacuum ***
		{
			version: 130000, viewname: "progress_vacuum",
			want: []string{"pid", "xact_age", "datname", "relation", "state", "waiting", "phase", "t_size", "t_scanned_%", "t_vacuumed_%", "scanned", "vacuumed", "query"},
		},
		{
			version: 120000, viewname: "progress_vacuum",
			want: []string{"pid", "xact_age", "datname", "relation", "state", "waiting", "phase", "t_size", "t_scanned_%", "t_vacuumed_%", "scanned", "vacuumed", "query"},
		},
		{
			version: 110000, viewname: "progress_vacuum",
			want: []string{"pid", "xact_age", "datname", "relation", "state", "waiting", "phase", "t_size", "t_scanned_%", "t_vacuumed_%", "scanned", "vacuumed", "query"},
		},
		{
			version: 100000, viewname: "progress_vacuum",
			want: []string{"pid", "xact_age", "datname", "relation", "state", "waiting", "phase", "t_size", "t_scanned_%", "t_vacuumed_%", "scanned", "vacuumed", "query"},
		},
		{
			version: 90600, viewname: "progress_vacuum",
			want: []string{"pid", "xact_age", "datname", "relation", "state", "waiting", "phase", "t_size", "t_scanned_%", "t_vacuumed_%", "scanned", "vacuumed", "query"},
		},
		// *** pg_stat_replication ***
		{
			version: 130000, trackCommit: "off", viewname: "replication",
			want: []string{"pid", "client", "user", "name", "state", "mode", "wal", "pending", "write", "flush", "replay", "total_lag", "write_lag", "flush_lag", "replay_lag"},
		},
		{
			version: 130000, trackCommit: "on", viewname: "replication",
			want: []string{"pid", "client", "user", "name", "state", "mode", "wal", "pending", "write", "flush", "replay", "total_lag", "write_lag", "flush_lag", "replay_lag", "xact_age", "time_age"},
		},
		{
			version: 120000, trackCommit: "off", viewname: "replication",
			want: []string{"pid", "client", "user", "name", "state", "mode", "wal", "pending", "write", "flush", "replay", "total_lag", "write_lag", "flush_lag", "replay_lag"},
		},
		{
			version: 120000, trackCommit: "on", viewname: "replication",
			want: []string{"pid", "client", "user", "name", "state", "mode", "wal", "pending", "write", "flush", "replay", "total_lag", "write_lag", "flush_lag", "replay_lag", "xact_age", "time_age"},
		},
		{
			version: 110000, trackCommit: "off", viewname: "replication",
			want: []string{"pid", "client", "user", "name", "state", "mode", "wal", "pending", "write", "flush", "replay", "total_lag", "write_lag", "flush_lag", "replay_lag"},
		},
		{
			version: 110000, trackCommit: "on", viewname: "replication",
			want: []string{"pid", "client", "user", "name", "state", "mode", "wal", "pending", "write", "flush", "replay", "total_lag", "write_lag", "flush_lag", "replay_lag", "xact_age", "time_age"},
		},
		{
			version: 100000, trackCommit: "off", viewname: "replication",
			want: []string{"pid", "client", "user", "name", "state", "mode", "wal", "pending", "write", "flush", "replay", "total_lag", "write_lag", "flush_lag", "replay_lag"},
		},
		{
			version: 100000, trackCommit: "on", viewname: "replication",
			want: []string{"pid", "client", "user", "name", "state", "mode", "wal", "pending", "write", "flush", "replay", "total_lag", "write_lag", "flush_lag", "replay_lag", "xact_age", "time_age"},
		},
		{
			version: 90600, trackCommit: "off", viewname: "replication",
			want: []string{"pid", "client", "user", "name", "state", "mode", "wal", "pending", "write", "flush", "replay", "total_lag"},
		},
		{
			version: 90600, trackCommit: "on", viewname: "replication",
			want: []string{"pid", "client", "user", "name", "state", "mode", "wal", "pending", "write", "flush", "replay", "total_lag", "xact_age", "time_age"},
		},
		{
			version: 90500, trackCommit: "off", viewname: "replication",
			want: []string{"pid", "client", "user", "name", "state", "mode", "wal", "pending", "write", "flush", "replay", "total_lag"},
		},
		{
			version: 90500, trackCommit: "on", viewname: "replication",
			want: []string{"pid", "client", "user", "name", "state", "mode", "wal", "pending", "write", "flush", "replay", "total_lag", "xact_age", "time_age"},
		},
		// *** relation sizes pseudo-stat  ***
		{
			version: 130000, viewname: "sizes",
			want: []string{"relation", "total_size", "rel_size", "idx_size", "total_change", "rel_change", "idx_change"},
		},
		{
			version: 120000, viewname: "sizes",
			want: []string{"relation", "total_size", "rel_size", "idx_size", "total_change", "rel_change", "idx_change"},
		},
		{
			version: 110000, viewname: "sizes",
			want: []string{"relation", "total_size", "rel_size", "idx_size", "total_change", "rel_change", "idx_change"},
		},
		{
			version: 100000, viewname: "sizes",
			want: []string{"relation", "total_size", "rel_size", "idx_size", "total_change", "rel_change", "idx_change"},
		},
		{
			version: 90600, viewname: "sizes",
			want: []string{"relation", "total_size", "rel_size", "idx_size", "total_change", "rel_change", "idx_change"},
		},
		{
			version: 90500, viewname: "sizes",
			want: []string{"relation", "total_size", "rel_size", "idx_size", "total_change", "rel_change", "idx_change"},
		},
		// *** pg_stat_statements timings ***
		{
			version: 130000, viewname: "statements_timings",
			want: []string{"user", "database", "t_all_t", "t_read_t", "t_write_t", "t_cpu_t", "all_t", "read_t", "write_t", "cpu_t", "calls", "queryid", "query"},
		},
		{
			version: 120000, viewname: "statements_timings",
			want: []string{"user", "database", "t_all_t", "t_read_t", "t_write_t", "t_cpu_t", "all_t", "read_t", "write_t", "cpu_t", "calls", "queryid", "query"},
		},
		{
			version: 110000, viewname: "statements_timings",
			want: []string{"user", "database", "t_all_t", "t_read_t", "t_write_t", "t_cpu_t", "all_t", "read_t", "write_t", "cpu_t", "calls", "queryid", "query"},
		},
		{
			version: 100000, viewname: "statements_timings",
			want: []string{"user", "database", "t_all_t", "t_read_t", "t_write_t", "t_cpu_t", "all_t", "read_t", "write_t", "cpu_t", "calls", "queryid", "query"},
		},
		{
			version: 90600, viewname: "statements_timings",
			want: []string{"user", "database", "t_all_t", "t_read_t", "t_write_t", "t_cpu_t", "all_t", "read_t", "write_t", "cpu_t", "calls", "queryid", "query"},
		},
		{
			version: 90500, viewname: "statements_timings",
			want: []string{"user", "database", "t_all_t", "t_read_t", "t_write_t", "t_cpu_t", "all_t", "read_t", "write_t", "cpu_t", "calls", "queryid", "query"},
		},
		// *** pg_stat_statements general ***
		{
			version: 130000, viewname: "statements_general",
			want: []string{"user", "database", "t_calls", "t_rows", "calls", "rows", "queryid", "query"},
		},
		{
			version: 120000, viewname: "statements_general",
			want: []string{"user", "database", "t_calls", "t_rows", "calls", "rows", "queryid", "query"},
		},
		{
			version: 110000, viewname: "statements_general",
			want: []string{"user", "database", "t_calls", "t_rows", "calls", "rows", "queryid", "query"},
		},
		{
			version: 100000, viewname: "statements_general",
			want: []string{"user", "database", "t_calls", "t_rows", "calls", "rows", "queryid", "query"},
		},
		{
			version: 90600, viewname: "statements_general",
			want: []string{"user", "database", "t_calls", "t_rows", "calls", "rows", "queryid", "query"},
		},
		{
			version: 90500, viewname: "statements_general",
			want: []string{"user", "database", "t_calls", "t_rows", "calls", "rows", "queryid", "query"},
		},
		// *** pg_stat_statements IO ***
		{
			version: 130000, viewname: "statements_io",
			want: []string{"user", "database", "t_hits", "t_reads", "t_dirtied", "t_written", "hits", "reads", "dirtied", "written", "calls", "queryid", "query"},
		},
		{
			version: 120000, viewname: "statements_io",
			want: []string{"user", "database", "t_hits", "t_reads", "t_dirtied", "t_written", "hits", "reads", "dirtied", "written", "calls", "queryid", "query"},
		},
		{
			version: 110000, viewname: "statements_io",
			want: []string{"user", "database", "t_hits", "t_reads", "t_dirtied", "t_written", "hits", "reads", "dirtied", "written", "calls", "queryid", "query"},
		},
		{
			version: 100000, viewname: "statements_io",
			want: []string{"user", "database", "t_hits", "t_reads", "t_dirtied", "t_written", "hits", "reads", "dirtied", "written", "calls", "queryid", "query"},
		},
		{
			version: 90600, viewname: "statements_io",
			want: []string{"user", "database", "t_hits", "t_reads", "t_dirtied", "t_written", "hits", "reads", "dirtied", "written", "calls", "queryid", "query"},
		},
		{
			version: 90500, viewname: "statements_io",
			want: []string{"user", "database", "t_hits", "t_reads", "t_dirtied", "t_written", "hits", "reads", "dirtied", "written", "calls", "queryid", "query"},
		},
		// *** pg_stat_statements temp IO ***
		{
			version: 130000, viewname: "statements_temp",
			want: []string{"user", "database", "t_tmp_read", "t_tmp_write", "tmp_read", "tmp_write", "calls", "queryid", "query"},
		},
		{
			version: 120000, viewname: "statements_temp",
			want: []string{"user", "database", "t_tmp_read", "t_tmp_write", "tmp_read", "tmp_write", "calls", "queryid", "query"},
		},
		{
			version: 110000, viewname: "statements_temp",
			want: []string{"user", "database", "t_tmp_read", "t_tmp_write", "tmp_read", "tmp_write", "calls", "queryid", "query"},
		},
		{
			version: 100000, viewname: "statements_temp",
			want: []string{"user", "database", "t_tmp_read", "t_tmp_write", "tmp_read", "tmp_write", "calls", "queryid", "query"},
		},
		{
			version: 90600, viewname: "statements_temp",
			want: []string{"user", "database", "t_tmp_read", "t_tmp_write", "tmp_read", "tmp_write", "calls", "queryid", "query"},
		},
		{
			version: 90500, viewname: "statements_temp",
			want: []string{"user", "database", "t_tmp_read", "t_tmp_write", "tmp_read", "tmp_write", "calls", "queryid", "query"},
		},
		// *** pg_stat_statements local IO ***
		{
			version: 130000, viewname: "statements_local",
			want: []string{"user", "database", "t_lo_hits", "t_lo_reads", "t_lo_dirtied", "t_lo_written", "lo_hits", "lo_reads", "lo_dirtied", "lo_written", "calls", "queryid", "query"},
		},
		{
			version: 120000, viewname: "statements_local",
			want: []string{"user", "database", "t_lo_hits", "t_lo_reads", "t_lo_dirtied", "t_lo_written", "lo_hits", "lo_reads", "lo_dirtied", "lo_written", "calls", "queryid", "query"},
		},
		{
			version: 110000, viewname: "statements_local",
			want: []string{"user", "database", "t_lo_hits", "t_lo_reads", "t_lo_dirtied", "t_lo_written", "lo_hits", "lo_reads", "lo_dirtied", "lo_written", "calls", "queryid", "query"},
		},
		{
			version: 100000, viewname: "statements_local",
			want: []string{"user", "database", "t_lo_hits", "t_lo_reads", "t_lo_dirtied", "t_lo_written", "lo_hits", "lo_reads", "lo_dirtied", "lo_written", "calls", "queryid", "query"},
		},
		{
			version: 90600, viewname: "statements_local",
			want: []string{"user", "database", "t_lo_hits", "t_lo_reads", "t_lo_dirtied", "t_lo_written", "lo_hits", "lo_reads", "lo_dirtied", "lo_written", "calls", "queryid", "query"},
		},
		{
			version: 90500, viewname: "statements_local",
			want: []string{"user", "database", "t_lo_hits", "t_lo_reads", "t_lo_dirtied", "t_lo_written", "lo_hits", "lo_reads", "lo_dirtied", "lo_written", "calls", "queryid", "query"},
		},
	}

	for _, tc := range testcases {
		t.Run(fmt.Sprintf("%s/%d", tc.viewname, tc.version), func(t *testing.T) {
			views := view.New()
			views.Configure(tc.version, tc.trackCommit)

			opts := query.Options{}
			opts.Configure(tc.version, "f", "top")

			q, err := query.Format(views[tc.viewname].QueryTmpl, opts)
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(tc.version)
			assert.NoError(t, err)

			res, err := NewPGresult(conn, q)
			assert.NoError(t, err)
			assert.Equal(t, tc.want, res.Cols)

			conn.Close()
		})
	}
}
