package query

import (
	"database/sql"
	"fmt"
	"strconv"
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
		// The selector ignores its argument by verified fact, so the invariant is "any argument",
		// not "these six". A future major, a version below the project floor and the zero value all
		// have to come back identical, or a branch was added.
		{version: 200000, wantNcols: 9, wantDiffIntvl: [2]int{0, 0}},
		{version: 130000, wantNcols: 9, wantDiffIntvl: [2]int{0, 0}},
		{version: 0, wantNcols: 9, wantDiffIntvl: [2]int{0, 0}},
	}

	for _, tc := range testcases {
		t.Run(fmt.Sprintf("version/%d", tc.version), func(t *testing.T) {
			gotQuery, gotNcols, gotDiffIntvl := SelectStatArchiverQuery(tc.version)

			// Guards only against a version branch being introduced - the query text itself is
			// pinned by Test_StatArchiverQuery_Structure.
			assert.Equal(t, PgStatArchiverDefault, gotQuery)
			assert.Equal(t, tc.wantNcols, gotNcols)
			assert.Equal(t, tc.wantDiffIntvl, gotDiffIntvl)
		})
	}
}

// Test_StatArchiverQuery_Structure pins the query's shape with no server, so the locked column order
// and the privileged .ready predicate stay guarded on a plain host run, where every live test skips.
// The predicate needs this test specifically: the fixtures have an empty archive status directory,
// so count(*) FILTER (WHERE name LIKE '%.ready') and a bare count(*) are both 0 on every cluster and
// no live assertion can tell them apart.
func Test_StatArchiverQuery_Structure(t *testing.T) {
	// The .ready filter is the only logic in the query and the feature's headline number.
	assert.Contains(t, PgStatArchiverDefault, "count(*) FILTER (WHERE name LIKE '%.ready')",
		"the backlog must count .ready files only - a bare count(*) or a wider pattern is a different number")
	assert.Contains(t, PgStatArchiverDefault, "FROM pg_ls_archive_statusdir()",
		"the backlog must come from pg_ls_archive_statusdir(), the pg_monitor-granted function")

	// Locked column order (Decision 14): every alias present, and in this exact sequence. The needle
	// carries the alias's delimiter so "archived" cannot match inside "archived_age" (nor "failed"
	// inside "failed_age") and blame the wrong column.
	prev := -1
	for i, col := range archiverColumns {
		needle := " AS " + col + ","
		if i == len(archiverColumns)-1 {
			needle = " AS " + col + " FROM"
		}

		idx := strings.Index(PgStatArchiverDefault, needle)
		assert.NotEqual(t, -1, idx, "query must select the %q column", col)
		assert.Greater(t, idx, prev, "%q must follow the previous locked column", col)
		prev = idx
	}

	assert.True(t, strings.HasSuffix(PgStatArchiverDefault, "FROM pg_stat_archiver"),
		"pg_stat_archiver must be the outer relation")

	// Decision 3: nothing on this screen is diffed, so calculateDelta short-circuits before diff()
	// and the strconv.ParseInt("") trap that forces coalesce(...,0) elsewhere does not apply. A
	// blank cell is the honest rendering of "this cluster has never archived".
	assert.NotContains(t, strings.ToLower(PgStatArchiverDefault), "coalesce",
		"no column on this screen is diffed, so no column may be coalesced (Decision 3)")
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

			conn := connectArchiverFixture(t, version)
			defer conn.Close()

			descs, nrows, err := runArchiverQuery(conn, q)
			assert.NoError(t, err)

			assert.Len(t, descs, wantNcols)
			assert.Equal(t, archiverColumns, descs, "live column names must match the locked order")
			assert.Equal(t, 1, nrows, "pg_stat_archiver is a single-row view")
		})
	}
}

// Test_StatArchiverQuery_NullsStayNull verifies Decision 3 against a live cluster: the four columns
// that are NULL on a cluster that has never archived stay SQL NULL and are not coalesced into an
// invented value. It also pins the values that are fixed on such a cluster, so the 'Archiver' row
// identity, the two counters and the date_trunc truncation are falsifiable rather than assumed.
// The no-coalesce guard on the query text itself lives in Test_StatArchiverQuery_Structure, which
// needs no server.
func Test_StatArchiverQuery_NullsStayNull(t *testing.T) {
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

			conn := connectArchiverFixture(t, version)
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

			assert.Equal(t, "Archiver", values[0].String,
				"column 0 is the row identity the single-row screen matches itself by across samples")
			assert.Equal(t, "0", values[2].String, "archived counter on a cluster that never archived")
			assert.Equal(t, "0", values[5].String, "failed counter on a cluster that never archived")
			assert.Regexp(t, `^-?(\d+ days? )?\d{2}:\d{2}:\d{2}$`, values[8].String,
				"stats_age must be truncated to whole seconds by date_trunc")

			// ready counts a live directory, so assert its type rather than a value - pinning 0
			// would couple the suite to the state of the archive status directory.
			ready, err := strconv.Atoi(values[1].String)
			assert.NoError(t, err, "ready must be an integer, got %q", values[1].String)
			assert.GreaterOrEqual(t, ready, 0)
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

			conn := connectArchiverFixture(t, version)
			defer conn.Close()

			err = postgres.SetupTestRole(conn, archiverRoleMonitor, true)
			assert.NoError(t, err)
			if err != nil {
				return
			}
			// RESET ROLE immediately after the successful SET ROLE, so it runs even when an
			// assertion below fails - a leaked SET ROLE would silently change what later
			// assertions on this connection see.
			defer resetRole(t, conn, archiverRoleMonitor)

			if !assertRestrictedSession(t, conn, archiverRoleMonitor, true) {
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

			conn := connectArchiverFixture(t, version)
			defer conn.Close()

			err = postgres.SetupTestRole(conn, archiverRoleNoRole, false)
			assert.NoError(t, err)
			if err != nil {
				return
			}
			defer resetRole(t, conn, archiverRoleNoRole)

			if !assertRestrictedSession(t, conn, archiverRoleNoRole, false) {
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

// assertRestrictedSession asserts the session really runs as the named non-superuser role AND that
// the role's privileges are exactly what the caller intends. It is the load-bearing guard of both
// privilege tests: deleting the SET ROLE would otherwise leave them silently passing as the fixture
// superuser - the exact failure mode that let a wrong privilege assumption survive a whole release.
//
// The membership assertion matters because the roles are cluster-global and SetupTestRole never
// normalises a role that already exists: without it the positive test could silently decay from
// "pg_monitor is sufficient" to "some privileged role works" after any stray GRANT.
//
// Returns false when the session is not as intended, so the caller can stop before the query and
// redden on the guard rather than on the query.
func assertRestrictedSession(t *testing.T, conn *postgres.DB, wantRole string, wantPgMonitor bool) bool {
	t.Helper()

	var (
		currentUser string
		isSuper     bool
		hasMonitor  bool
		memberOf    []string
	)
	err := conn.QueryRow(
		"SELECT current_user::text, "+
			"(SELECT rolsuper FROM pg_roles WHERE rolname = current_user), "+
			"pg_has_role(current_user, 'pg_monitor', 'USAGE'), "+
			// DISTINCT because PG 16+ stores one pg_auth_members row per grantor, so a role
			// re-granted by hand (the mutation procedure does exactly that) would list twice.
			"coalesce((SELECT array_agg(DISTINCT r.rolname::text ORDER BY r.rolname::text) "+
			"FROM pg_auth_members m JOIN pg_roles r ON r.oid = m.roleid "+
			"WHERE m.member = (SELECT oid FROM pg_roles WHERE rolname = current_user)), ARRAY[]::text[])",
	).Scan(&currentUser, &isSuper, &hasMonitor, &memberOf)
	if !assert.NoError(t, err) {
		return false
	}

	okUser := assert.Equal(t, wantRole, currentUser, "session must run as the test role")
	okSuper := assert.False(t, isSuper, "the test role must not be a superuser")
	okMonitor := assert.Equal(t, wantPgMonitor, hasMonitor, "pg_monitor membership of the test role")

	var okMembers bool
	if wantPgMonitor {
		okMembers = assert.Equal(t, []string{"pg_monitor"}, memberOf,
			"the test role must hold pg_monitor and nothing else")
	} else {
		okMembers = assert.Empty(t, memberOf, "the deny role must hold no role membership at all")
	}

	return okUser && okSuper && okMonitor && okMembers
}

// resetRole restores the session to the fixture superuser and asserts the reset took effect.
// Both privilege tests use a dedicated connection they close at the end of the subtest, so a leaked
// SET ROLE cannot currently reach a later test - RESET ROLE is required by Decision 18 and is the
// belt to that braces. Should the two tests ever share one connection, this is the guard they rely on.
func resetRole(t *testing.T, conn *postgres.DB, role string) {
	t.Helper()

	_, err := conn.Exec("RESET ROLE")
	assert.NoError(t, err)

	var currentUser string
	if assert.NoError(t, conn.QueryRow("SELECT current_user::text").Scan(&currentUser)) {
		assert.NotEqual(t, role, currentUser, "RESET ROLE must leave the test role")
	}
}

// Test_SetupTestRole_RejectsUnsafeName pins the role-name guard in postgres.SetupTestRole. A role
// name is an SQL identifier, so it cannot travel as a $1 placeholder and the helper interpolates it;
// the guard is what keeps "callers pass literal constants" an invariant instead of a doc comment, in
// a file that has no build tag and ships in the released binary. Validation runs before the
// connection is touched, so a nil *postgres.DB suffices - and without the guard these names would
// reach db.Exec rather than being refused.
//
// It lives in this file rather than in internal/postgres because this task may modify only three
// files (acceptance criterion 1), and the helper's only callers are here.
func Test_SetupTestRole_RejectsUnsafeName(t *testing.T) {
	unsafe := map[string]string{
		"statement separator": "a; DROP ROLE victim",
		"trailing newline":    "role\n; DROP ROLE victim",
		"quote and comment":   "role'--",
		"dollar sign":         "pgcenter_test$x",
		"upper case":          "PgCenter_Test",
		"leading digit":       "1role",
		"empty":               "",
	}

	for name, role := range unsafe {
		t.Run(name, func(t *testing.T) {
			err := postgres.SetupTestRole(nil, role, false)
			assert.Error(t, err, "unsafe role name must be refused before any statement is built")
			assert.Contains(t, fmt.Sprint(err), "invalid test role name")
		})
	}
}

// connectArchiverFixture connects to the fixture cluster of the given version. A version missing
// from the port map fails instead of skipping: a forgotten entry would otherwise make every subtest
// for a new version pass while exercising nothing at all (internal/postgres/testing_test.go).
func connectArchiverFixture(t *testing.T, version int) *postgres.DB {
	t.Helper()

	conn, err := postgres.NewTestConnectVersion(version)
	if err != nil {
		assert.NotContains(t, err.Error(), "no test cluster port mapping",
			"version %d is missing from the test port map", version)
		t.Skipf("postgres %d not available in test environment", version)
	}

	return conn
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
