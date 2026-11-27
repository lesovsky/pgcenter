package query

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_SelectStatWALQuery(t *testing.T) {
	testcases := []struct {
		version       int
		wantNcols     int
		wantDiffIntvl [2]int
	}{
		{version: 140000, wantNcols: 11, wantDiffIntvl: [2]int{2, 9}},
		{version: 150000, wantNcols: 11, wantDiffIntvl: [2]int{2, 9}},
		{version: 170000, wantNcols: 11, wantDiffIntvl: [2]int{2, 9}},
		// PG 18: removed wal_write/wal_sync; stats_age must be outside the diff interval.
		{version: 180000, wantNcols: 7, wantDiffIntvl: [2]int{2, 5}},
		// PG 19: added wal_fpi_bytes as "fpi,KiB"; stats_age shifts to col 7 and stays outside.
		{version: 190000, wantNcols: 8, wantDiffIntvl: [2]int{2, 6}},
		// Forward version: the PG 19 branch must fire for every future major, not only 190000.
		{version: 200000, wantNcols: 8, wantDiffIntvl: [2]int{2, 6}},
	}

	for _, tc := range testcases {
		t.Run(fmt.Sprintf("version/%d", tc.version), func(t *testing.T) {
			_, gotNcols, gotDiffIntvl := SelectStatWALQuery(tc.version)
			assert.Equal(t, tc.wantNcols, gotNcols)
			assert.Equal(t, tc.wantDiffIntvl, gotDiffIntvl)
		})
	}
}

// Test_SelectStatWALQuery_PG19ColumnOrder pins the position of the "fpi,KiB" column in the PG 19
// select list: it must sit right after the wal_fpi counter (count and volume adjacent) and before
// wal_buffers_full, so it lands inside DiffIntvl {2,6} and renders as a per-interval delta. It also
// pins stats_age as the last selected column — a text date_trunc value pulled inside the interval
// aborts the whole sample at strconv.ParseInt.
func Test_SelectStatWALQuery_PG19ColumnOrder(t *testing.T) {
	q, _, _ := SelectStatWALQuery(PostgresV19)

	idxFpi := strings.Index(q, "wal_fpi AS fpi")
	idxFpiKiB := strings.Index(q, `AS "fpi,KiB"`)
	idxBuffersFull := strings.Index(q, "wal_buffers_full")

	assert.NotEqual(t, -1, idxFpi, "PG 19 query must keep the wal_fpi counter")
	assert.NotEqual(t, -1, idxFpiKiB, `PG 19 query must select the "fpi,KiB" column`)
	assert.NotEqual(t, -1, idxBuffersFull, "PG 19 query must keep wal_buffers_full")

	assert.Less(t, idxFpi, idxFpiKiB, `"fpi,KiB" must follow the wal_fpi counter`)
	assert.Less(t, idxFpiKiB, idxBuffersFull, `"fpi,KiB" must precede wal_buffers_full`)

	assert.True(t, strings.HasSuffix(q, "AS stats_age FROM pg_stat_wal"),
		"stats_age must remain the last selected column, outside the diff interval")

	// The PG 19 query is the PG 18 query plus exactly one expression. Deriving it here pins what the
	// name/position checks above cannot see: the body of the new expression (notably the /1024 that
	// makes the "fpi,KiB" header truthful) and the fact that none of the other seven columns drifted.
	assert.Equal(t, PgStatWALPG19, strings.Replace(PgStatWALDefault,
		"wal_fpi AS fpi, wal_buffers_full",
		`wal_fpi AS fpi, round(wal_fpi_bytes / 1024, 2) AS "fpi,KiB", wal_buffers_full`, 1),
		"PG 19 query must be the PG 18 query plus exactly the fpi,KiB expression")
}

// Test_SelectStatWALQuery_LegacyBranchesUntouched proves the PG 19 work did not leak into the older
// branches: the selector still returns exactly the pre-existing constants below PG 19, and neither of
// them mentions wal_fpi_bytes (a column that does not exist before PG 19).
func Test_SelectStatWALQuery_LegacyBranchesUntouched(t *testing.T) {
	for _, version := range []int{140000, 150000, 170000, 179999} {
		q, _, _ := SelectStatWALQuery(version)
		assert.Equal(t, PgStatWALPG14, q, "version %d must return PgStatWALPG14", version)
	}

	for _, version := range []int{180000, 189999} {
		q, _, _ := SelectStatWALQuery(version)
		assert.Equal(t, PgStatWALDefault, q, "version %d must return PgStatWALDefault", version)
	}

	assert.NotContains(t, PgStatWALPG14, "wal_fpi_bytes", "wal_fpi_bytes does not exist before PG 19")
	assert.NotContains(t, PgStatWALDefault, "wal_fpi_bytes", "wal_fpi_bytes does not exist before PG 19")
}

// Test_StatWALQueries tests query execution against all supported Postgres versions.
func Test_StatWALQueries(t *testing.T) {
	versions := []int{140000, 150000, 160000, 170000, 180000, 190000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_wal/%d", version), func(t *testing.T) {
			tmpl, wantNcols, diffIntvl := SelectStatWALQuery(version)

			opts := NewOptions(version, "f", "off", 256, "public")
			q, err := Format(tmpl, opts)
			require.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}
			defer conn.Close()

			// Fatal, not just failed: if wal_fpi_bytes is renamed at a later PG 19 beta/RC, the
			// undefined-column error is the signal and must not be buried under derived failures.
			rows, err := conn.Query(q)
			require.NoError(t, err)

			var names []string
			for _, fd := range rows.FieldDescriptions() {
				names = append(names, string(fd.Name))
			}
			rows.Close()
			assert.NoError(t, rows.Err())

			// The live result must have exactly as many columns as the selector declared — the view
			// is configured from Ncols, so a mismatch misaligns the whole screen.
			assert.Len(t, names, wantNcols)

			// stats_age must stay outside the diff interval on every version.
			require.Greater(t, len(names), diffIntvl[1]+1,
				"DiffIntvl upper bound must leave at least one column (stats_age) outside")
			assert.Equal(t, "stats_age", names[diffIntvl[1]+1],
				"the column after the diff interval must be stats_age")

			if version >= PostgresV19 {
				assert.Equal(t,
					[]string{"source", "waldir_size", "wal,KiB", "records", "fpi", "fpi,KiB", "buffers_full", "stats_age"},
					names)
				assert.Equal(t, "buffers_full", names[diffIntvl[1]],
					"the last diffed column must be buffers_full")
			}
		})
	}
}
