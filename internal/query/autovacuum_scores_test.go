package query

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var autovacuumScoresColumns = []string{
	"relation", "score", "do_vacuum", "do_analyze", "for_wraparound", "dead_total",
	"xid_score", "mxid_score", "vacuum_score", "vacuum_insert_score", "analyze_score",
}

const (
	autovacuumScoresRoleMonitor = "pgcenter_test_autovacuum_scores_monitor"
	autovacuumScoresRoleNoRole  = "pgcenter_test_autovacuum_scores_norole"

	autovacuumWraparoundDedicatedEnv = "PGCENTER_AUTOVACUUM_WRAPAROUND_DEDICATED"
	autovacuumWraparoundIdentityEnv  = "PGCENTER_AUTOVACUUM_WRAPAROUND_SYSTEM_IDENTIFIER"
	autovacuumWraparoundSelfTestEnv  = "PGCENTER_AUTOVACUUM_WRAPAROUND_SELF_TEST"
	autovacuumWraparoundProbe        = "pgcenter_test_autovacuum_wraparound_probe"
	autovacuumWraparoundBurn         = "pgcenter_test_autovacuum_wraparound_burn"
	autovacuumWraparoundBurnXIDs     = 120000
)

func Test_SelectStatAutovacuumScoresQuery(t *testing.T) {
	for _, version := range []int{0, 180000, 190000, 200000} {
		t.Run(fmt.Sprintf("version/%d", version), func(t *testing.T) {
			gotQuery, gotNcols, gotDiffIntvl := SelectStatAutovacuumScoresQuery(version)
			assert.Equal(t, PgStatAutovacuumScoresDefault, gotQuery)
			assert.Equal(t, 11, gotNcols)
			assert.Equal(t, [2]int{0, 0}, gotDiffIntvl)
		})
	}
}

func Test_StatAutovacuumScoresQuery_Structure(t *testing.T) {
	t.Run("locked column order", func(t *testing.T) {
		previous := -1
		for i, column := range autovacuumScoresColumns {
			needle := " AS " + column + ", "
			if i == len(autovacuumScoresColumns)-1 {
				needle = " AS " + column + " FROM "
			}
			index := strings.Index(PgStatAutovacuumScoresDefault, needle)
			require.NotEqual(t, -1, index, "query must select %q", column)
			assert.Greater(t, index, previous, "%q must follow the previous locked column", column)
			previous = index
		}
		assert.NotContains(t, PgStatAutovacuumScoresDefault, " AS relid", "relid is internal join evidence only")
	})

	t.Run("outer left join", func(t *testing.T) {
		assert.Contains(t, PgStatAutovacuumScoresDefault,
			"FROM pg_stat_autovacuum_scores s LEFT JOIN pg_stat_all_tables a ON s.relid = a.relid ")
	})

	t.Run("outer schema filter and wraparound exception", func(t *testing.T) {
		assert.Contains(t, PgStatAutovacuumScoresDefault,
			`s.schemaname NOT IN ('pg_catalog', 'pg_toast', 'information_schema') OR s.for_wraparound`)
		assert.NotContains(t, PgStatAutovacuumScoresDefault, "a.schemaname NOT IN")
	})

	t.Run("qualified raw score order", func(t *testing.T) {
		assert.True(t, strings.HasSuffix(PgStatAutovacuumScoresDefault, "ORDER BY s.score DESC"),
			"ordering must use the unrounded input score, not the rounded output alias")
	})

	t.Run("six scores rounded in SQL", func(t *testing.T) {
		for _, column := range []string{"score", "xid_score", "mxid_score", "vacuum_score", "vacuum_insert_score", "analyze_score"} {
			assert.Contains(t, PgStatAutovacuumScoresDefault,
				"round(s."+column+"::numeric, 2) AS "+column)
		}
	})

	t.Run("booleans rendered as text", func(t *testing.T) {
		for _, column := range []string{"do_vacuum", "do_analyze", "for_wraparound"} {
			assert.Contains(t, PgStatAutovacuumScoresDefault, "s."+column+"::text AS "+column)
		}
	})

	t.Run("absolute dead tuples without coalesce", func(t *testing.T) {
		assert.Contains(t, PgStatAutovacuumScoresDefault, "a.n_dead_tup AS dead_total")
		assert.NotContains(t, strings.ToLower(PgStatAutovacuumScoresDefault), "coalesce")
	})

	t.Run("view type template", func(t *testing.T) {
		assert.Contains(t, PgStatAutovacuumScoresDefault, `{{if ne .ViewType "all"}}`)
		for _, viewType := range []string{"user", "all"} {
			opts := NewOptions(PostgresV19, "f", "off", 256, "public")
			opts.ViewType = viewType
			formatted, err := Format(PgStatAutovacuumScoresDefault, opts)
			require.NoError(t, err)
			assert.NotContains(t, formatted, "{{")
			if viewType == "all" {
				assert.NotContains(t, formatted, "WHERE")
			} else {
				assert.Contains(t, formatted, "WHERE")
			}
			assert.True(t, strings.HasSuffix(formatted, "ORDER BY s.score DESC"))
		}
	})
}

func Test_StatAutovacuumScoresQuery_LiveColumnsAndValues(t *testing.T) {
	conn := connectAutovacuumScoresFixture(t)
	defer conn.Close()

	for _, viewType := range []string{"user", "all"} {
		t.Run(viewType, func(t *testing.T) {
			query := formatAutovacuumScoresQuery(t, viewType)
			names, rows := runAutovacuumScoresQuery(t, conn, query)
			assert.Equal(t, autovacuumScoresColumns, names, "live PG 19 columns must match the locked order")

			sawSystem := false
			for _, row := range rows {
				require.Len(t, row, len(autovacuumScoresColumns))
				assert.True(t, row[0].Valid)
				assert.Contains(t, row[0].String, ".")
				for _, index := range []int{1, 6, 7, 8, 9, 10} {
					require.True(t, row[index].Valid, "score column %s must not be NULL", names[index])
					assert.Regexp(t, `^[0-9]+\.[0-9]{2}$`, row[index].String,
						"score column %s must be rounded to two decimals", names[index])
				}
				for _, index := range []int{2, 3, 4} {
					assert.Contains(t, []string{"true", "false"}, row[index].String, "boolean column %s", names[index])
				}
				if row[5].Valid {
					_, err := strconv.ParseInt(row[5].String, 10, 64)
					assert.NoError(t, err, "dead_total must be an absolute integer without a unit")
				}

				schema, _, ok := strings.Cut(row[0].String, ".")
				require.True(t, ok)
				system := schema == "pg_catalog" || schema == "pg_toast" || schema == "information_schema"
				sawSystem = sawSystem || system
				if viewType == "user" && system {
					assert.Equal(t, "true", row[4].String,
						"only a wraparound system relation may escape the user-mode schema filter")
				}
			}
			if viewType == "all" {
				assert.True(t, sawSystem, "all mode must expose the system catalog")
			}
		})
	}
}

func Test_StatAutovacuumScoresQuery_PgMonitorRoleSucceeds(t *testing.T) {
	testAutovacuumScoresPrivileges(t, autovacuumScoresRoleMonitor, true)
}

func Test_StatAutovacuumScoresQuery_WithoutPgMonitorSucceeds(t *testing.T) {
	testAutovacuumScoresPrivileges(t, autovacuumScoresRoleNoRole, false)
}

func testAutovacuumScoresPrivileges(t *testing.T, role string, pgMonitor bool) {
	t.Helper()
	conn := connectAutovacuumScoresFixture(t)
	defer conn.Close()

	require.NoError(t, postgres.SetupTestRole(conn, role, pgMonitor))
	defer resetRole(t, conn, role)
	require.True(t, assertRestrictedSession(t, conn, role, pgMonitor))

	query := formatAutovacuumScoresQuery(t, "user")
	names, _ := runAutovacuumScoresQuery(t, conn, query)
	assert.Equal(t, autovacuumScoresColumns, names)
}

func Test_StatAutovacuumScoresQuery_RoundedTieRawOrder(t *testing.T) {
	conn := connectAutovacuumScoresFixture(t)
	t.Cleanup(conn.Close)

	fixtures := []struct {
		name  string
		rows  int
		score float64
	}{
		{name: "pgcenter_test_autovacuum_score_low", rows: 9995, score: 0.00099950},
		{name: "pgcenter_test_autovacuum_score_medium", rows: 19990, score: 0.00199900},
		{name: "pgcenter_test_autovacuum_score_high", rows: 29985, score: 0.00299850},
	}

	_, err := conn.Exec("DROP TABLE IF EXISTS public.pgcenter_test_autovacuum_score_low, " +
		"public.pgcenter_test_autovacuum_score_medium, public.pgcenter_test_autovacuum_score_high")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, cleanupErr := conn.Exec("DROP TABLE IF EXISTS public.pgcenter_test_autovacuum_score_low, " +
			"public.pgcenter_test_autovacuum_score_medium, public.pgcenter_test_autovacuum_score_high")
		assert.NoError(t, cleanupErr)
	})

	for _, fixture := range fixtures {
		_, err = conn.Exec("CREATE TABLE public." + fixture.name + " (id integer) WITH (" +
			"autovacuum_enabled=false, autovacuum_analyze_threshold=10000000, " +
			"autovacuum_analyze_scale_factor=0, autovacuum_vacuum_insert_threshold=2147483647, " +
			"autovacuum_vacuum_insert_scale_factor=0)")
		require.NoError(t, err)
		_, err = conn.Exec(fmt.Sprintf("INSERT INTO public.%s SELECT generate_series(1, %d)", fixture.name, fixture.rows))
		require.NoError(t, err)
	}

	for _, fixture := range fixtures {
		waitForAutovacuumScore(t, conn, fixture.name, fixture.score)
	}

	t.Run("raw descending score order survives rounded ties", func(t *testing.T) {
		_, rows := runAutovacuumScoresQuery(t, conn, formatAutovacuumScoresQuery(t, "user"))
		var gotOrder []string
		for _, row := range rows {
			for _, fixture := range fixtures {
				if row[0].String == "public."+fixture.name {
					gotOrder = append(gotOrder, fixture.name)
					assert.Equal(t, "0.00", row[1].String, "all three raw scores must form a rounded tie")
					assert.Equal(t, "false", row[3].String)
					assert.Equal(t, "0", row[5].String)
				}
			}
		}
		assert.Equal(t, []string{
			"pgcenter_test_autovacuum_score_high",
			"pgcenter_test_autovacuum_score_medium",
			"pgcenter_test_autovacuum_score_low",
		}, gotOrder, "raw descending score order assertion")
	})
}

func Test_StatAutovacuumScoresQuery_WraparoundException(t *testing.T) {
	if os.Getenv(autovacuumWraparoundDedicatedEnv) != "1" {
		t.Skip("requires the dedicated disposable PG 19 supervisor; ordinary-suite skip is not PASS evidence")
	}
	expectedIdentity := os.Getenv(autovacuumWraparoundIdentityEnv)
	if expectedIdentity == "" {
		t.Fatal("dedicated marker requires the expected system_identifier")
	}
	if mode := os.Getenv(autovacuumWraparoundSelfTestEnv); mode != "" {
		if mode != "timeout" && mode != "sigkill" {
			t.Fatalf("unknown supervisor self-test mode %q", mode)
		}
		t.Logf("self-test %s: blocked before database mutation", mode)
		for {
			time.Sleep(time.Second)
		}
	}

	conn := connectAutovacuumScoresFixture(t)
	t.Cleanup(conn.Close)

	originalAutovacuum := showAutovacuum(t, conn)
	if originalAutovacuum != "on" && originalAutovacuum != "off" {
		t.Fatalf("unexpected original autovacuum setting %q", originalAutovacuum)
	}

	var freezeMaxAge, oldestDatabaseAge int64
	require.NoError(t, conn.QueryRow(
		"SELECT current_setting('autovacuum_freeze_max_age')::bigint, "+
			"coalesce(max(age(datfrozenxid)), 0)::bigint FROM pg_database",
	).Scan(&freezeMaxAge, &oldestDatabaseAge))
	require.Less(t, oldestDatabaseAge+autovacuumWraparoundBurnXIDs+1000000, freezeMaxAge,
		"refusing to burn XIDs without a global freeze-age safety margin")

	var identity string
	require.NoError(t, conn.QueryRow("SELECT system_identifier::text FROM pg_control_system()").Scan(&identity))
	require.Equal(t, expectedIdentity, identity,
		"fail-closed dedicated-server identity check immediately before mutation")

	t.Cleanup(func() {
		_, err := conn.Exec("DROP PROCEDURE IF EXISTS public." + autovacuumWraparoundBurn + "(integer)")
		assert.NoError(t, err)
		_, err = conn.Exec("DROP TABLE IF EXISTS public." + autovacuumWraparoundProbe)
		assert.NoError(t, err)
		assert.NoError(t, setAutovacuum(conn, originalAutovacuum),
			"the original autovacuum setting must be restored and verified")
	})

	require.NoError(t, setAutovacuum(conn, "off"))
	_, err := conn.Exec("DROP PROCEDURE IF EXISTS public." + autovacuumWraparoundBurn + "(integer)")
	require.NoError(t, err)
	_, err = conn.Exec("DROP TABLE IF EXISTS public." + autovacuumWraparoundProbe)
	require.NoError(t, err)
	_, err = conn.Exec("CREATE TABLE public." + autovacuumWraparoundProbe + " (payload text)")
	require.NoError(t, err)
	_, err = conn.Exec("INSERT INTO public." + autovacuumWraparoundProbe + " VALUES (repeat('x', 10000))")
	require.NoError(t, err)
	_, err = conn.Exec("ALTER TABLE public." + autovacuumWraparoundProbe +
		" SET (toast.autovacuum_freeze_max_age = 100000)")
	require.NoError(t, err)
	_, err = conn.Exec("CREATE PROCEDURE public." + autovacuumWraparoundBurn + `(n integer)
LANGUAGE plpgsql AS $$
BEGIN
    FOR i IN 1..n LOOP
        PERFORM pg_current_xact_id();
        COMMIT;
    END LOOP;
END
$$`)
	require.NoError(t, err)

	var toastOID, toastName, toastSchema string
	require.NoError(t, conn.QueryRow(
		"SELECT c.reltoastrelid::bigint::text, t.relname, n.nspname "+
			"FROM pg_class c JOIN pg_class t ON t.oid = c.reltoastrelid "+
			"JOIN pg_namespace n ON n.oid = t.relnamespace "+
			"WHERE c.oid = 'public."+autovacuumWraparoundProbe+"'::regclass",
	).Scan(&toastOID, &toastName, &toastSchema))
	require.NotEqual(t, "", toastOID)
	require.NotEqual(t, "0", toastOID)
	require.Equal(t, "pg_toast", toastSchema, "the flagged relation must be in a filtered system schema")

	var toastAge int64
	require.NoError(t, conn.QueryRow("SELECT age(relfrozenxid)::bigint FROM pg_class WHERE oid = $1::oid", toastOID).Scan(&toastAge))
	require.GreaterOrEqual(t, toastAge+autovacuumWraparoundBurnXIDs, int64(100000),
		"the burn must cross the probe's toast freeze threshold")
	require.Less(t, toastAge+autovacuumWraparoundBurnXIDs+1000000, freezeMaxAge,
		"the probe burn must retain the global freeze-age safety margin")

	_, err = conn.Exec(fmt.Sprintf("CALL public.%s(%d)", autovacuumWraparoundBurn, autovacuumWraparoundBurnXIDs))
	require.NoError(t, err)

	var forWraparound bool
	require.NoError(t, conn.QueryRow(
		"SELECT for_wraparound FROM pg_stat_autovacuum_scores WHERE relid = $1::oid", toastOID,
	).Scan(&forWraparound))
	require.True(t, forWraparound, "fixture setup must produce a flagged toast relation before testing the query")

	_, rows := runAutovacuumScoresQuery(t, conn, formatAutovacuumScoresQuery(t, "user"))
	flaggedRelation := "pg_toast." + toastName
	t.Run("system row presence assertion", func(t *testing.T) {
		found := false
		for _, row := range rows {
			if row[0].String == flaggedRelation {
				found = true
				assert.Equal(t, "true", row[4].String)
			}
			if strings.HasPrefix(row[0].String, "pg_catalog.") ||
				strings.HasPrefix(row[0].String, "information_schema.") {
				assert.Fail(t, "ordinary system relation escaped the user-mode filter", row[0].String)
			}
		}
		assert.True(t, found, "wraparound system-row presence assertion: %s must remain visible in user mode", flaggedRelation)
	})
}

func connectAutovacuumScoresFixture(t *testing.T) *postgres.DB {
	t.Helper()
	conn, err := postgres.NewTestConnectVersion(PostgresV19)
	if err != nil {
		assert.NotContains(t, err.Error(), "no test cluster port mapping")
		t.Skipf("postgres 19 not available in test environment: %s", err)
	}
	return conn
}

func formatAutovacuumScoresQuery(t *testing.T, viewType string) string {
	t.Helper()
	opts := NewOptions(PostgresV19, "f", "off", 256, "public")
	opts.ViewType = viewType
	query, err := Format(PgStatAutovacuumScoresDefault, opts)
	require.NoError(t, err)
	require.NotContains(t, query, "{{")
	return query
}

func runAutovacuumScoresQuery(t *testing.T, conn *postgres.DB, query string) ([]string, [][]sql.NullString) {
	t.Helper()
	rows, err := conn.Query(query)
	require.NoError(t, err)
	defer rows.Close()

	descriptions := rows.FieldDescriptions()
	names := make([]string, len(descriptions))
	for i, description := range descriptions {
		names[i] = string(description.Name)
	}

	var result [][]sql.NullString
	for rows.Next() {
		values := make([]sql.NullString, len(names))
		pointers := make([]any, len(values))
		for i := range values {
			pointers[i] = &values[i]
		}
		require.NoError(t, rows.Scan(pointers...))
		result = append(result, values)
	}
	require.NoError(t, rows.Err())
	return names, result
}

func waitForAutovacuumScore(t *testing.T, conn *postgres.DB, relation string, expected float64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last float64
	for time.Now().Before(deadline) {
		err := conn.QueryRow("SELECT score FROM pg_stat_autovacuum_scores WHERE relid = $1::regclass",
			"public."+relation).Scan(&last)
		require.NoError(t, err)
		if math.Abs(last-expected) < 0.000000001 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("relation %s has raw score %.10f, want %.10f", relation, last, expected)
}

func showAutovacuum(t *testing.T, conn *postgres.DB) string {
	t.Helper()
	var value string
	require.NoError(t, conn.QueryRow("SHOW autovacuum").Scan(&value))
	return value
}

func setAutovacuum(conn *postgres.DB, value string) error {
	if value != "on" && value != "off" {
		return fmt.Errorf("invalid autovacuum setting %q", value)
	}
	if _, err := conn.Exec("ALTER SYSTEM SET autovacuum = " + value); err != nil {
		return fmt.Errorf("set autovacuum: %w", err)
	}
	if _, err := conn.Exec("SELECT pg_reload_conf()"); err != nil {
		return fmt.Errorf("reload configuration: %w", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		if err := conn.QueryRow("SHOW autovacuum").Scan(&last); err != nil {
			return fmt.Errorf("verify autovacuum setting: %w", err)
		}
		if last == value {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("autovacuum setting is %q after reload, want %q", last, value)
}
