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

// archiverVersions lists the Postgres majors the archiver query must work on. pg_stat_archiver is
// schema-identical on all of them and pg_ls_archive_statusdir() exists on all of them, so every
// version runs the same query text.
var archiverVersions = []int{140000, 150000, 160000, 170000, 180000, 190000}

// archiverColumns is the column order locked by the user-spec (tech-spec Data Models / Decision 14).
// The live tests assert this list by NAME, not just its length: a column inserted mid-layout would
// keep the count right while shifting every index the view, record and report layers depend on.
var archiverColumns = []string{
	"source", "ready", "archived", "last_archived", "archived_age",
	"failed", "last_failed", "failed_age", "stats_age",
}

// Role names are specific to this test file so a role left behind on a long-lived cluster is
// attributable and cannot be confused with the roles other test files create through the same helper.
const (
	archiverRoleMonitor = "pgcenter_test_archiver_monitor"
	archiverRoleNoRole  = "pgcenter_test_archiver_norole"
)

func Test_SelectStatArchiverQuery(t *testing.T) {
	testcases := []struct {
		version       int
		wantNcols     int
		wantDiffIntvl [2]int
	}{
		{version: 140000, wantNcols: 9, wantDiffIntvl: [2]int{0, 0}},
		{version: 150000, wantNcols: 9, wantDiffIntvl: [2]int{0, 0}},
		{version: 160000, wantNcols: 9, wantDiffIntvl: [2]int{0, 0}},
		{version: 170000, wantNcols: 9, wantDiffIntvl: [2]int{0, 0}},
		{version: 180000, wantNcols: 9, wantDiffIntvl: [2]int{0, 0}},
		{version: 190000, wantNcols: 9, wantDiffIntvl: [2]int{0, 0}},
	}

	for _, tc := range testcases {
		t.Run(fmt.Sprintf("version/%d", tc.version), func(t *testing.T) {
			gotQuery, gotNcols, gotDiffIntvl := SelectStatArchiverQuery(tc.version)

			// The selector is version-independent by verified fact, not by omission: every version
			// must return the very same query text, so a future branch cannot be added silently.
			assert.Equal(t, PgStatArchiverDefault, gotQuery)
			assert.Equal(t, tc.wantNcols, gotNcols)
			assert.Equal(t, tc.wantDiffIntvl, gotDiffIntvl)
		})
	}
}

// Test_StatArchiverQueries tests query execution against all supported Postgres versions and pins the
// live result shape: 9 columns in the locked order, exactly one row.
func Test_StatArchiverQueries(t *testing.T) {
	for _, version := range archiverVersions {
		t.Run(fmt.Sprintf("pg_stat_archiver/%d", version), func(t *testing.T) {
			tmpl, wantNcols, _ := SelectStatArchiverQuery(version)

			opts := NewOptions(version, "f", "off", 256, "public")
			q, err := Format(tmpl, opts)
			assert.NoError(t, err)
			assert.NotContains(t, q, "{{", "formatted query must carry no template artifacts")

			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}
			defer conn.Close()

			descs, nrows, err := runArchiverQuery(conn, q)
			assert.NoError(t, err)

			assert.Len(t, descs, wantNcols)
			assert.Equal(t, archiverColumns, descs, "live column names must match the locked order")
			assert.Equal(t, 1, nrows, "pg_stat_archiver is a single-row view")
		})
	}
}

// Test_StatArchiverQuery_NullsStayNull verifies Decision 3: the four columns that are NULL on a
// cluster that has never archived stay SQL NULL and are not coalesced into an invented value.
// Nothing on this screen is diffed (DiffIntvl{0,0}), so calculateDelta short-circuits before diff()
// and the strconv.ParseInt("") trap that forces coalesce(...,0) elsewhere does not apply here.
// The coalesce check needs no server; the Valid/NULL check runs against the fixtures, which have
// archive_mode=off and have therefore never archived a segment.
func Test_StatArchiverQuery_NullsStayNull(t *testing.T) {
	assert.NotContains(t, strings.ToLower(PgStatArchiverDefault), "coalesce",
		"no column on this screen is diffed, so no column may be coalesced (Decision 3)")

	// The fixtures never archived: the two WAL names and the two age columns are NULL, while the
	// literal, the .ready count, both bigint counters and stats_age are always set.
	wantValid := map[string]bool{
		"source":        true,
		"ready":         true,
		"archived":      true,
		"last_archived": false,
		"archived_age":  false,
		"failed":        true,
		"last_failed":   false,
		"failed_age":    false,
		"stats_age":     true,
	}

	for _, version := range archiverVersions {
		t.Run(fmt.Sprintf("pg_stat_archiver/%d", version), func(t *testing.T) {
			tmpl, _, _ := SelectStatArchiverQuery(version)

			q, err := Format(tmpl, NewOptions(version, "f", "off", 256, "public"))
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}
			defer conn.Close()

			// Scan into sql.NullString receivers - the very type the stats pipeline uses
			// (stat.PGresult.Values), so this asserts what the screen will actually see.
			values := make([]sql.NullString, len(archiverColumns))
			pointers := make([]any, len(values))
			for i := range pointers {
				pointers[i] = &values[i]
			}

			err = conn.QueryRow(q).Scan(pointers...)
			assert.NoError(t, err)
			if err != nil {
				return
			}

			for i, col := range archiverColumns {
				assert.Equal(t, wantValid[col], values[i].Valid, "column %q NULL-ness", col)
			}
		})
	}
}

// Test_StatArchiverQuery_PgMonitorRoleSucceeds proves Decision 4 in the positive direction: a role
// holding pg_monitor and nothing else can run the whole query, including the privileged
// pg_ls_archive_statusdir() call. The fixture connection is the superuser postgres, so the test
// asserts the session is really restricted BEFORE running the query - without that guard the test
// would pass identically as superuser and prove nothing about privileges.
func Test_StatArchiverQuery_PgMonitorRoleSucceeds(t *testing.T) {
	for _, version := range archiverVersions {
		t.Run(fmt.Sprintf("pg_stat_archiver/%d", version), func(t *testing.T) {
			tmpl, wantNcols, _ := SelectStatArchiverQuery(version)

			q, err := Format(tmpl, NewOptions(version, "f", "off", 256, "public"))
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}
			defer conn.Close()

			err = postgres.SetupTestRole(conn, archiverRoleMonitor, true)
			assert.NoError(t, err)
			if err != nil {
				return
			}
			// RESET ROLE immediately after the successful SET ROLE, so it runs even when an
			// assertion below fails - a leaked SET ROLE would silently change what later
			// assertions on this connection see.
			defer resetRole(t, conn)

			if !assertRestrictedSession(t, conn, archiverRoleMonitor) {
				return
			}

			descs, nrows, err := runArchiverQuery(conn, q)
			assert.NoError(t, err)

			assert.Len(t, descs, wantNcols)
			assert.Equal(t, 1, nrows)
		})
	}
}

// Test_StatArchiverQuery_WithoutPgMonitorFails proves Decision 4 in the negative direction: a role
// with neither superuser nor pg_monitor loses the whole screen, and it loses it on
// pg_ls_archive_statusdir() specifically. The assertion is pinned to SQLSTATE 42501 and to the
// function name - asserting merely "an error occurred" would pass on a syntax typo.
func Test_StatArchiverQuery_WithoutPgMonitorFails(t *testing.T) {
	for _, version := range archiverVersions {
		t.Run(fmt.Sprintf("pg_stat_archiver/%d", version), func(t *testing.T) {
			tmpl, _, _ := SelectStatArchiverQuery(version)

			q, err := Format(tmpl, NewOptions(version, "f", "off", 256, "public"))
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}
			defer conn.Close()

			err = postgres.SetupTestRole(conn, archiverRoleNoRole, false)
			assert.NoError(t, err)
			if err != nil {
				return
			}
			defer resetRole(t, conn)

			if !assertRestrictedSession(t, conn, archiverRoleNoRole) {
				return
			}

			_, _, err = runArchiverQuery(conn, q)
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

// assertRestrictedSession asserts the session really runs as the named non-superuser role. It is the
// load-bearing guard of both privilege tests: deleting the SET ROLE would otherwise leave them
// silently passing as the fixture superuser - the exact failure mode that let a wrong privilege
// assumption survive a whole release. Returns false when the session is not restricted, so the
// caller can stop before the query and redden on the guard rather than on the query.
func assertRestrictedSession(t *testing.T, conn *postgres.DB, wantRole string) bool {
	t.Helper()

	var (
		currentUser string
		isSuper     bool
	)
	err := conn.QueryRow(
		"SELECT current_user, (SELECT rolsuper FROM pg_roles WHERE rolname = current_user)",
	).Scan(&currentUser, &isSuper)
	if !assert.NoError(t, err) {
		return false
	}

	okUser := assert.Equal(t, wantRole, currentUser, "session must run as the test role")
	okSuper := assert.False(t, isSuper, "the test role must not be a superuser")

	return okUser && okSuper
}

// resetRole restores the session to the fixture superuser.
func resetRole(t *testing.T, conn *postgres.DB) {
	t.Helper()

	_, err := conn.Exec("RESET ROLE")
	assert.NoError(t, err)
}

// runArchiverQuery executes q and returns the result's column names, its row count and the first
// error the driver surfaced. A server error may be reported either by Query itself or only when the
// result is drained, so both are collected here - a negative privilege test that inspected only the
// Query error could otherwise miss the failure entirely.
func runArchiverQuery(conn *postgres.DB, q string) ([]string, int, error) {
	rows, err := conn.Query(q)
	if err != nil {
		return nil, 0, err
	}

	descs := rows.FieldDescriptions()
	names := make([]string, len(descs))
	for i, d := range descs {
		names[i] = string(d.Name)
	}

	var nrows int
	for rows.Next() {
		nrows++
	}
	rows.Close()

	return names, nrows, rows.Err()
}
