package query

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
)

// overviewVersions enumerates the actively supported Postgres versions for live-PG tests.
var overviewVersions = []int{140000, 150000, 160000, 170000, 180000, 190000}

// Role names owned by the archiving-backlog tests. They are deliberately distinct from the archiver
// tests' roles: roles are cluster-global and never dropped, so sharing them would couple the cluster
// state of two independent tasks.
const (
	backlogRoleMonitor = "pgcenter_test_backlog_monitor"
	backlogRoleNoRole  = "pgcenter_test_backlog_norole"
)

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
	// The archiving backlog aggregate reads archive_status via pg_ls_archive_statusdir(), which
	// superuser and pg_monitor may execute. The fixtures role is postgres, a superuser, so this test
	// says nothing about privileges (Test_ArchivingBacklogQuery_PgMonitorRole does): it asserts only
	// that the query either executes successfully or fails with an error the caller can catch,
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

// Test_ArchivingBacklogQuery_Structure pins the aggregate's arithmetic without a server, mirroring
// Test_StatArchiverQuery_Structure. The fixtures run archive_mode=off with an empty status
// directory, so every live assertion on the backlog reduces to 0 >= 0: dropping the .ready FILTER or
// the wal_segment_size multiplication would keep all the live tests green. Only substring fixation
// reddens on those two mutations.
func Test_ArchivingBacklogQuery_Structure(t *testing.T) {
	assert.Contains(t, OverviewArchivingBacklog, "count(*) FILTER (WHERE name LIKE '%.ready')",
		"only .ready files are backlog - a bare count(*) is a different number")
	assert.Contains(t, OverviewArchivingBacklog, "pg_size_bytes(current_setting('wal_segment_size'))",
		"the backlog is bytes, not a segment count")
	assert.Contains(t, OverviewArchivingBacklog, "FROM pg_ls_archive_statusdir()",
		"the pg_monitor-executable function is the whole point of Decision 8")
	assert.NotContains(t, OverviewArchivingBacklog, "pg_ls_dir",
		"the superuser-only predecessor must not come back")
}

// Test_ArchivingBacklogQuery_PgMonitorRole is the whole point of the aggregate's rewrite: a role
// holding only pg_monitor - the role the verbose panel exists to serve - must get a number, not n/a.
// pg_ls_dir is superuser-only, so the old query 42501'd for that role on every tick and the operator
// never saw the first signal that archiving had stopped; pg_ls_archive_statusdir() is granted to
// pg_monitor.
//
// The fixture connection is the superuser postgres, so the restricted-session guard runs BEFORE the
// aggregate: without it the test would pass identically as superuser and prove nothing about
// privileges - which is exactly how the wrong assumption survived into ADR [010].
func Test_ArchivingBacklogQuery_PgMonitorRole(t *testing.T) {
	for _, version := range overviewVersions {
		t.Run(fmt.Sprintf("backlog/%d", version), func(t *testing.T) {
			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}
			defer conn.Close()

			err = postgres.SetupTestRole(conn, backlogRoleMonitor, true)
			assert.NoError(t, err)
			if err != nil {
				return
			}
			// RESET ROLE immediately after the successful SET ROLE, so it runs even when an
			// assertion below fails.
			defer resetRole(t, conn, backlogRoleMonitor)

			if !assertRestrictedSession(t, conn, backlogRoleMonitor, true) {
				return
			}

			var backlog int64
			err = conn.QueryRow(OverviewArchivingBacklog).Scan(&backlog)
			assert.NoError(t, err, "pg_monitor must be able to read the archiving backlog")
			assert.GreaterOrEqual(t, backlog, int64(0))
		})
	}
}

// Test_ArchivingBacklogQuery_NoPrivilegeRole is the negative half: moving off pg_ls_dir widens who
// can read the backlog, and this pins how far. A role holding neither superuser nor pg_monitor must
// still be refused, so the change is a privilege fix and not a privilege downgrade. The assertion is
// pinned to SQLSTATE 42501 and to the function name - "an error occurred" would also pass on a typo.
func Test_ArchivingBacklogQuery_NoPrivilegeRole(t *testing.T) {
	for _, version := range overviewVersions {
		t.Run(fmt.Sprintf("backlog/%d", version), func(t *testing.T) {
			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}
			defer conn.Close()

			err = postgres.SetupTestRole(conn, backlogRoleNoRole, false)
			assert.NoError(t, err)
			if err != nil {
				return
			}
			defer resetRole(t, conn, backlogRoleNoRole)

			if !assertRestrictedSession(t, conn, backlogRoleNoRole, false) {
				return
			}

			var backlog int64
			err = conn.QueryRow(OverviewArchivingBacklog).Scan(&backlog)
			assert.Error(t, err)

			var pgErr *pgconn.PgError
			if assert.ErrorAs(t, err, &pgErr) {
				assert.Equal(t, "42501", pgErr.Code, "must fail with insufficient_privilege")
				assert.Contains(t, pgErr.Message, "pg_ls_archive_statusdir",
					"the failure must name the privileged call, not just any error")
			}
		})
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
