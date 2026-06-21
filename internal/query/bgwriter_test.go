package query

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_SelectStatBgwriterQuery(t *testing.T) {
	testcases := []struct {
		version       int
		wantNcols     int
		wantDiffIntvl [2]int
	}{
		{version: 140000, wantNcols: 12, wantDiffIntvl: [2]int{3, 10}},
		{version: 150000, wantNcols: 12, wantDiffIntvl: [2]int{3, 10}},
		{version: 160000, wantNcols: 12, wantDiffIntvl: [2]int{3, 10}},
		// PG 17: pg_stat_checkpointer added; restartpoint counters appear.
		{version: 170000, wantNcols: 13, wantDiffIntvl: [2]int{6, 11}},
		// PG 18: slru_written added to the diffed block.
		{version: 180000, wantNcols: 14, wantDiffIntvl: [2]int{6, 12}},
	}

	for _, tc := range testcases {
		t.Run(fmt.Sprintf("version/%d", tc.version), func(t *testing.T) {
			_, gotNcols, gotDiffIntvl := SelectStatBgwriterQuery(tc.version)
			assert.Equal(t, tc.wantNcols, gotNcols)
			assert.Equal(t, tc.wantDiffIntvl, gotDiffIntvl)
		})
	}
}

// Test_StatBgwriterQueries tests query execution against all supported Postgres versions.
func Test_StatBgwriterQueries(t *testing.T) {
	versions := []int{140000, 150000, 160000, 170000, 180000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_bgwriter/%d", version), func(t *testing.T) {
			tmpl, _, _ := SelectStatBgwriterQuery(version)

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
