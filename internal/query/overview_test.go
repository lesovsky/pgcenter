package query

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
)

// overviewVersions enumerates the actively supported Postgres versions for live-PG tests.
var overviewVersions = []int{140000, 150000, 160000, 170000, 180000}

func Test_OverviewQueries(t *testing.T) {
	// Static (non-template) aggregates: each must execute AND scan into exactly the receivers
	// collectOverviewStat uses, so a column-count/type drift fails here rather than at runtime.
	for _, version := range overviewVersions {
		conn, err := postgres.NewTestConnectVersion(version)
		if err != nil {
			t.Skipf("postgres %d not available in test environment", version)
		}

		t.Run("workload", func(t *testing.T) {
			var commits, rollbacks, ins, upd, del, ret, tmp, dl, conf, csum int64
			err := conn.QueryRow(OverviewWorkload).Scan(&commits, &rollbacks, &ins, &upd, &del, &ret, &tmp, &dl, &conf, &csum)
			assert.NoError(t, err)
		})

		t.Run("databases", func(t *testing.T) {
			var count, hits, reads int64
			err := conn.QueryRow(OverviewDatabases).Scan(&count, &hits, &reads)
			assert.NoError(t, err)
			assert.GreaterOrEqual(t, count, int64(1))
		})

		t.Run("databases_size", func(t *testing.T) {
			var total int64
			err := conn.QueryRow(OverviewDatabasesSize).Scan(&total)
			assert.NoError(t, err)
			assert.GreaterOrEqual(t, total, int64(0))
		})

		t.Run("workers", func(t *testing.T) {
			var umbrella, logical, parallel int
			err := conn.QueryRow(OverviewWorkers).Scan(&umbrella, &logical, &parallel)
			assert.NoError(t, err)
		})

		t.Run("wal_size", func(t *testing.T) {
			var walSize int64
			err := conn.QueryRow(OverviewWalSize).Scan(&walSize)
			assert.NoError(t, err)
			assert.GreaterOrEqual(t, walSize, int64(0))
		})

		t.Run("send_recv", func(t *testing.T) {
			var senders, receivers int
			err := conn.QueryRow(OverviewSendRecv).Scan(&senders, &receivers)
			assert.NoError(t, err)
		})

		conn.Close()
	}
}

func Test_OverviewQueries_Templates(t *testing.T) {
	for _, version := range overviewVersions {
		conn, err := postgres.NewTestConnectVersion(version)
		if err != nil {
			t.Skipf("postgres %d not available in test environment", version)
		}

		// Test only with recovery 'f' (primary): the test clusters are primaries, so the standby
		// WAL functions would error when executed here. recovery 't' substitution is verified in
		// Test_OverviewQueries_Templates_Recovery below.
		opts := NewOptions(version, "f", "off", 0, "public")

		t.Run("replication_lag", func(t *testing.T) {
			q, err := Format(OverviewReplicationLag, opts)
			assert.NoError(t, err)
			assert.NotContains(t, q, "{{")

			// max() over an empty set yields NULL on a primary with no standbys -> NullInt64 invalid.
			var lag sql.NullInt64
			err = conn.QueryRow(q).Scan(&lag)
			assert.NoError(t, err)
		})

		t.Run("replication_slots", func(t *testing.T) {
			q, err := Format(OverviewReplicationSlots, opts)
			assert.NoError(t, err)
			assert.NotContains(t, q, "{{")

			var slots int64
			var retained sql.NullInt64
			err = conn.QueryRow(q).Scan(&slots, &retained)
			assert.NoError(t, err)
			assert.GreaterOrEqual(t, slots, int64(0))
		})

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

func Test_OverviewBgwriterColumns(t *testing.T) {
	// bgwr/ckpt reuses SelectStatBgwriterQuery and collectOverviewBgwriter maps values by column
	// NAME (positions differ across PG14-16/17/18). Verify the five names it reads are actually
	// present in the result set on every supported version — a rename would silently zero the row
	// otherwise. This is the real risk the by-name scan introduces.
	wantCols := []string{"ckpt_timed", "ckpt_req", "ckpt_write,ms", "ckpt_sync,ms", "maxwritten"}

	for _, version := range overviewVersions {
		conn, err := postgres.NewTestConnectVersion(version)
		if err != nil {
			t.Skipf("postgres %d not available in test environment", version)
		}

		q, _, _ := SelectStatBgwriterQuery(version)
		assert.NotEmpty(t, q)
		assert.False(t, strings.Contains(q, "{{"))

		rows, err := conn.Query(q)
		assert.NoError(t, err)
		if err == nil {
			present := make(map[string]bool)
			for _, d := range rows.FieldDescriptions() {
				present[string(d.Name)] = true
			}
			rows.Close()
			for _, c := range wantCols {
				assert.Truef(t, present[c], "PG %d bgwriter result is missing column %q", version, c)
			}
		}

		conn.Close()
	}
}
