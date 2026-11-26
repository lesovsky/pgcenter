package query

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var statIndexesColumns = []string{"index", "scan", "tuples_read", "tuples_fetch", "hit", "read,KiB"}

func Test_SelectStatIndexesQuery(t *testing.T) {
	testcases := []struct {
		version       int
		wantQuery     string
		wantNcols     int
		wantDiffIntvl [2]int
	}{
		{version: 0, wantQuery: PgStatIndexesDefault, wantNcols: 6, wantDiffIntvl: [2]int{1, 5}},
		{version: 140000, wantQuery: PgStatIndexesDefault, wantNcols: 6, wantDiffIntvl: [2]int{1, 5}},
		{version: 150000, wantQuery: PgStatIndexesDefault, wantNcols: 6, wantDiffIntvl: [2]int{1, 5}},
		{version: 160000, wantQuery: PgStatIndexesDefault, wantNcols: 6, wantDiffIntvl: [2]int{1, 5}},
		{version: 170000, wantQuery: PgStatIndexesDefault, wantNcols: 6, wantDiffIntvl: [2]int{1, 5}},
		{version: 180000, wantQuery: PgStatIndexesDefault, wantNcols: 6, wantDiffIntvl: [2]int{1, 5}},
		{version: 190000, wantQuery: PgStatIndexesPG19, wantNcols: 7, wantDiffIntvl: [2]int{1, 5}},
		{version: 200000, wantQuery: PgStatIndexesPG19, wantNcols: 7, wantDiffIntvl: [2]int{1, 5}},
	}

	minNcols := 0
	diffIntervals := make([][2]int, len(testcases))
	for i, tc := range testcases {
		t.Run(fmt.Sprintf("version/%d", tc.version), func(t *testing.T) {
			gotQuery, gotNcols, gotDiffIntvl := SelectStatIndexesQuery(tc.version)

			assert.Equal(t, tc.wantQuery, gotQuery)
			assert.Equal(t, tc.wantNcols, gotNcols)
			assert.Equal(t, tc.wantDiffIntvl, gotDiffIntvl)
			if tc.version >= PostgresV19 {
				assert.Equal(t, gotNcols-2, gotDiffIntvl[1], "stats_age must immediately follow the diff interval")
			}
		})

		_, gotNcols, gotDiffIntvl := SelectStatIndexesQuery(tc.version)
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

func Test_StatIndexesQuery_StatsAgeSource(t *testing.T) {
	q, _, _ := SelectStatIndexesQuery(PostgresV19)

	assert.Contains(t, q, "date_trunc('seconds', now() - s.stats_reset)::text AS stats_age",
		"stats_age must come from pg_stat_* alias s, not pg_statio_* alias i")
	assert.NotContains(t, q, "now() - i.stats_reset")
	assert.True(t, strings.HasSuffix(q, "AS stats_age FROM pg_stat_{{.ViewType}}_indexes s, pg_statio_{{.ViewType}}_indexes i WHERE s.indexrelid = i.indexrelid ORDER BY (s.schemaname ||'.'|| s.relname ||'.'|| s.indexrelname) DESC"),
		"stats_age must be the final selected column")
	assert.NotContains(t, PgStatIndexesDefault, "stats_age", "pre-PG19 layout must remain unchanged")
}

func Test_StatIndexesQueries(t *testing.T) {
	for _, version := range statRelationVersions {
		t.Run(fmt.Sprintf("pg_stat_indexes/%d", version), func(t *testing.T) {
			tmpl, wantNcols, _ := SelectStatIndexesQuery(version)

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

			wantColumns := append([]string(nil), statIndexesColumns...)
			if version >= PostgresV19 {
				wantColumns = append(wantColumns, "stats_age")
			}
			assert.Len(t, names, wantNcols)
			assert.Equal(t, wantColumns, names, "live column names must match the locked order")
		})
	}
}

func Test_StatIndexesQuery_NullStatsAge(t *testing.T) {
	const (
		relation = "pgcenter_test_indexes_stats_age_null"
		index    = "pgcenter_test_indexes_stats_age_null_idx"
	)

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
	_, err = conn.Exec("CREATE INDEX " + index + " ON " + relation + " (id)")
	require.NoError(t, err)

	tmpl, ncols, _ := SelectStatIndexesQuery(PostgresV19)
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

		if values[0].String == "public."+relation+"."+index {
			found = true
			assert.False(t, values[ncols-1].Valid, "an untouched index must have NULL stats_reset")
			assert.Empty(t, values[ncols-1].String, "NULL stats_age must render as a blank cell")
		}
	}
	rows.Close()
	require.NoError(t, rows.Err())
	assert.True(t, found, "the newly created index must be present in pg_stat_user_indexes")
}
