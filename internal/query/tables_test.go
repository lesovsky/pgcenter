package query

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var statRelationVersions = []int{140000, 150000, 160000, 170000, 180000, 190000}

var statTablesColumns = []string{
	"relation", "seq_scan", "seq_read", "idx_scan", "idx_fetch", "inserts", "updates", "deletes",
	"hot_updates", "live", "dead", "heap_read,KiB", "heap_hit", "idx_read,KiB", "idx_hit",
	"toast_read,KiB", "toast_hit", "tidx_read,KiB", "tidx_hit",
}

func Test_SelectStatTablesQuery(t *testing.T) {
	testcases := []struct {
		version       int
		wantQuery     string
		wantNcols     int
		wantDiffIntvl [2]int
	}{
		{version: 0, wantQuery: PgStatTablesDefault, wantNcols: 19, wantDiffIntvl: [2]int{1, 18}},
		{version: 140000, wantQuery: PgStatTablesDefault, wantNcols: 19, wantDiffIntvl: [2]int{1, 18}},
		{version: 150000, wantQuery: PgStatTablesDefault, wantNcols: 19, wantDiffIntvl: [2]int{1, 18}},
		{version: 160000, wantQuery: PgStatTablesDefault, wantNcols: 19, wantDiffIntvl: [2]int{1, 18}},
		{version: 170000, wantQuery: PgStatTablesDefault, wantNcols: 19, wantDiffIntvl: [2]int{1, 18}},
		{version: 180000, wantQuery: PgStatTablesDefault, wantNcols: 19, wantDiffIntvl: [2]int{1, 18}},
		{version: 190000, wantQuery: PgStatTablesPG19, wantNcols: 20, wantDiffIntvl: [2]int{1, 18}},
		{version: 200000, wantQuery: PgStatTablesPG19, wantNcols: 20, wantDiffIntvl: [2]int{1, 18}},
	}

	minNcols := 0
	diffIntervals := make([][2]int, len(testcases))
	for i, tc := range testcases {
		t.Run(fmt.Sprintf("version/%d", tc.version), func(t *testing.T) {
			gotQuery, gotNcols, gotDiffIntvl := SelectStatTablesQuery(tc.version)

			assert.Equal(t, tc.wantQuery, gotQuery)
			assert.Equal(t, tc.wantNcols, gotNcols)
			assert.Equal(t, tc.wantDiffIntvl, gotDiffIntvl)
			if tc.version >= PostgresV19 {
				assert.Equal(t, gotNcols-2, gotDiffIntvl[1], "stats_age must immediately follow the diff interval")
			}
		})

		_, gotNcols, gotDiffIntvl := SelectStatTablesQuery(tc.version)
		if minNcols == 0 || gotNcols < minNcols {
			minNcols = gotNcols
		}
		diffIntervals[i] = gotDiffIntvl
	}

	for i, diffIntvl := range diffIntervals {
		assert.Less(t, diffIntvl[1], minNcols,
			"version %d DiffIntvl upper bound must be valid against the narrowest layout", testcases[i].version)
	}
}

func Test_StatTablesQuery_StatsAgeSource(t *testing.T) {
	q, _, _ := SelectStatTablesQuery(PostgresV19)

	assert.Contains(t, q, "date_trunc('seconds', now() - t.stats_reset)::text AS stats_age",
		"stats_age must come from pg_stat_* alias t, not pg_statio_* alias i")
	assert.NotContains(t, q, "now() - i.stats_reset")
	assert.True(t, strings.HasSuffix(q, "AS stats_age FROM pg_stat_{{.ViewType}}_tables t, pg_statio_{{.ViewType}}_tables i WHERE t.relid = i.relid ORDER BY (t.schemaname || '.' || t.relname) DESC"),
		"stats_age must be the final selected column")
	assert.NotContains(t, PgStatTablesDefault, "stats_age", "pre-PG19 layout must remain unchanged")
}

func Test_StatTablesQueries(t *testing.T) {
	for _, version := range statRelationVersions {
		t.Run(fmt.Sprintf("pg_stat_tables/%d", version), func(t *testing.T) {
			tmpl, wantNcols, _ := SelectStatTablesQuery(version)

			q, err := Format(tmpl, NewOptions(version, "f", "off", 256, "public"))
			require.NoError(t, err)
			assert.NotContains(t, q, "{{", "formatted query must carry no template artifacts")
			assert.NotContains(t, q, "}}", "formatted query must carry no template artifacts")

			conn := connectStatRelationFixture(t, version)
			defer conn.Close()

			rows, err := conn.Query(q)
			require.NoError(t, err)
			defer rows.Close()

			descs := rows.FieldDescriptions()
			names := make([]string, len(descs))
			for i, d := range descs {
				names[i] = string(d.Name)
			}

			for rows.Next() {
			}
			rows.Close()
			require.NoError(t, rows.Err())

			wantColumns := append([]string(nil), statTablesColumns...)
			if version >= PostgresV19 {
				wantColumns = append(wantColumns, "stats_age")
			}
			assert.Len(t, names, wantNcols)
			assert.Equal(t, wantColumns, names, "live column names must match the locked order")
		})
	}
}

func Test_StatTablesQuery_NullStatsAge(t *testing.T) {
	const relation = "pgcenter_test_tables_stats_age_null"

	conn := connectStatRelationFixture(t, PostgresV19)
	defer conn.Close()

	_, err := conn.Exec("DROP TABLE IF EXISTS " + relation)
	require.NoError(t, err)
	_, err = conn.Exec("CREATE TABLE " + relation + " (id integer)")
	require.NoError(t, err)
	defer func() {
		_, err := conn.Exec("DROP TABLE IF EXISTS " + relation)
		assert.NoError(t, err)
	}()

	tmpl, ncols, _ := SelectStatTablesQuery(PostgresV19)
	q, err := Format(tmpl, NewOptions(PostgresV19, "f", "off", 256, "public"))
	require.NoError(t, err)

	rows, err := conn.Query(q)
	require.NoError(t, err)
	defer rows.Close()

	var found bool
	for rows.Next() {
		values := make([]sql.NullString, ncols)
		dest := make([]any, ncols)
		for i := range values {
			dest[i] = &values[i]
		}
		require.NoError(t, rows.Scan(dest...))

		if values[0].String == "public."+relation {
			found = true
			assert.False(t, values[ncols-1].Valid, "an untouched relation must have NULL stats_reset")
			assert.Empty(t, values[ncols-1].String, "NULL stats_age must render as a blank cell")
		}
	}
	rows.Close()
	require.NoError(t, rows.Err())
	assert.True(t, found, "the newly created relation must be present in pg_stat_user_tables")
}

func connectStatRelationFixture(t *testing.T, version int) *postgres.DB {
	t.Helper()

	conn, err := postgres.NewTestConnectVersion(version)
	if err != nil {
		require.NotContains(t, err.Error(), "no test cluster port mapping",
			"version %d is missing from the test port map", version)
		t.Skipf("postgres %d not available in test environment", version)
	}

	return conn
}
