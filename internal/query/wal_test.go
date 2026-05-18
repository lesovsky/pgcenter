package query

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_SelectStatWALQuery(t *testing.T) {
	testcases := []struct {
		version      int
		wantNcols    int
		wantDiffIntvl [2]int
	}{
		{version: 140000, wantNcols: 11, wantDiffIntvl: [2]int{2, 9}},
		{version: 150000, wantNcols: 11, wantDiffIntvl: [2]int{2, 9}},
		{version: 170000, wantNcols: 11, wantDiffIntvl: [2]int{2, 9}},
		// PG 18: removed wal_write/wal_sync; stats_age must be outside the diff interval.
		{version: 180000, wantNcols: 7, wantDiffIntvl: [2]int{2, 5}},
	}

	for _, tc := range testcases {
		t.Run(fmt.Sprintf("version/%d", tc.version), func(t *testing.T) {
			_, gotNcols, gotDiffIntvl := SelectStatWALQuery(tc.version)
			assert.Equal(t, tc.wantNcols, gotNcols)
			assert.Equal(t, tc.wantDiffIntvl, gotDiffIntvl)
		})
	}
}

// Test_StatWALQueries tests query execution against all supported Postgres versions.
func Test_StatWALQueries(t *testing.T) {
	versions := []int{140000, 150000, 160000, 170000, 180000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_wal/%d", version), func(t *testing.T) {
			tmpl, _, _ := SelectStatWALQuery(version)

			opts := NewOptions(version, "f", "off", 256, "public")
			q, err := Format(tmpl, opts)
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}

			_, err = conn.Exec(q)
			assert.NoError(t, err)

			conn.Close()
		})
	}
}
