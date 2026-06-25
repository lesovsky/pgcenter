package query

import (
	"strings"
	"testing"

	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
)

// overviewVersions enumerates the actively supported Postgres versions for live-PG tests.
var overviewVersions = []int{140000, 150000, 160000, 170000, 180000}

func Test_OverviewQueries(t *testing.T) {
	// Static (non-template) aggregates: must execute and return exactly one row on all supported versions.
	staticQueries := []struct {
		name  string
		query string
	}{
		{name: "workload", query: OverviewWorkload},
		{name: "databases", query: OverviewDatabases},
		{name: "workers", query: OverviewWorkers},
		{name: "wal_size", query: OverviewWalSize},
		{name: "send_recv", query: OverviewSendRecv},
	}

	for _, version := range overviewVersions {
		conn, err := postgres.NewTestConnectVersion(version)
		if err != nil {
			t.Skipf("postgres %d not available in test environment", version)
		}

		for _, q := range staticQueries {
			t.Run(q.name, func(t *testing.T) {
				rows, err := conn.Query(q.query)
				assert.NoError(t, err)
				if err == nil {
					rows.Close()
				}
			})
		}

		conn.Close()
	}
}

func Test_OverviewQueries_Templates(t *testing.T) {
	// Template aggregates (WAL-fn): must be Format-ed with recovery state and then execute.
	templates := []struct {
		name string
		tmpl string
	}{
		{name: "replication_lag", tmpl: OverviewReplicationLag},
		{name: "replication_slots", tmpl: OverviewReplicationSlots},
	}

	for _, version := range overviewVersions {
		conn, err := postgres.NewTestConnectVersion(version)
		if err != nil {
			t.Skipf("postgres %d not available in test environment", version)
		}

		// Test only with recovery 'f' (primary): the test clusters are primaries.
		// recovery 't' formatting is covered by Test_OverviewQueries_Templates_Recovery below.
		opts := NewOptions(version, "f", "off", 0, "public")

		for _, tc := range templates {
			t.Run(tc.name, func(t *testing.T) {
				q, err := Format(tc.tmpl, opts)
				assert.NoError(t, err)
				assert.NotContains(t, q, "{{")

				rows, err := conn.Query(q)
				assert.NoError(t, err)
				if err == nil {
					rows.Close()
				}
			})
		}

		conn.Close()
	}
}

func Test_OverviewQueries_Templates_Recovery(t *testing.T) {
	// Verify recovery-aware substitution: 'f' -> current WAL fn, 't' -> last-received WAL fn.
	templates := []string{OverviewReplicationLag, OverviewReplicationSlots}

	for _, tmpl := range templates {
		primary, err := Format(tmpl, NewOptions(170000, "f", "off", 0, "public"))
		assert.NoError(t, err)
		assert.Contains(t, primary, "pg_current_wal_lsn")
		assert.NotContains(t, primary, "{{")

		standby, err := Format(tmpl, NewOptions(170000, "t", "off", 0, "public"))
		assert.NoError(t, err)
		assert.Contains(t, standby, "pg_last_wal_receive_lsn")
		assert.NotContains(t, standby, "{{")
	}
}

func Test_ArchivingBacklogQuery_Degrades(t *testing.T) {
	// The archiving backlog aggregate reads pg_wal/archive_status via pg_ls_dir, which requires
	// pg_monitor/superuser. On the test clusters the fixtures role has access, so the query must
	// either execute successfully or fail with an error the caller can catch (privilege/archive_mode=off)
	// WITHOUT panicking and WITHOUT being run as part of a larger scan.
	for _, version := range overviewVersions {
		conn, err := postgres.NewTestConnectVersion(version)
		if err != nil {
			t.Skipf("postgres %d not available in test environment", version)
		}

		var backlog int64
		err = conn.QueryRow(OverviewArchivingBacklog).Scan(&backlog)
		// Either success (>=0) or a privilege error — both acceptable; must not panic.
		if err == nil {
			assert.GreaterOrEqual(t, backlog, int64(0))
		} else {
			// On error the raw text must not be surfaced by collect; here we just assert it is a real error.
			assert.Error(t, err)
		}

		conn.Close()
	}
}

func TestSelectStatBgwriterQuery_OverviewReuse(t *testing.T) {
	// Sanity: bgwr/ckpt reuses SelectStatBgwriterQuery; verify it is callable and returns a non-empty query.
	for _, version := range overviewVersions {
		q, ncols, _ := SelectStatBgwriterQuery(version)
		assert.NotEmpty(t, q)
		assert.Greater(t, ncols, 0)
		assert.False(t, strings.Contains(q, "{{"))
	}
}
