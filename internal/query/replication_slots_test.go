package query

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_SelectStatReplicationSlotsQuery(t *testing.T) {
	testcases := []struct {
		version       int
		wantNcols     int
		wantDiffIntvl [2]int
	}{
		{version: 140000, wantNcols: 15, wantDiffIntvl: [2]int{6, 13}},
		{version: 150000, wantNcols: 15, wantDiffIntvl: [2]int{6, 13}},
		{version: 160000, wantNcols: 15, wantDiffIntvl: [2]int{6, 13}},
		{version: 170000, wantNcols: 15, wantDiffIntvl: [2]int{6, 13}},
		{version: 180000, wantNcols: 15, wantDiffIntvl: [2]int{6, 13}},
	}

	for _, tc := range testcases {
		t.Run(fmt.Sprintf("version/%d", tc.version), func(t *testing.T) {
			_, gotNcols, gotDiffIntvl := SelectStatReplicationSlotsQuery(tc.version)
			assert.Equal(t, tc.wantNcols, gotNcols)
			assert.Equal(t, tc.wantDiffIntvl, gotDiffIntvl)
		})
	}
}

// Test_StatReplicationSlotsQueries tests query execution against all supported Postgres versions.
func Test_StatReplicationSlotsQueries(t *testing.T) {
	versions := []int{140000, 150000, 160000, 170000, 180000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_replication_slots/%d", version), func(t *testing.T) {
			tmpl, wantNcols, _ := SelectStatReplicationSlotsQuery(version)

			opts := NewOptions(version, "f", "off", 256, "public")
			q, err := Format(tmpl, opts)
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}
			defer conn.Close()

			rows, err := conn.Query(q)
			assert.NoError(t, err)

			// Assert the live column count matches the declared Ncols. The hybrid
			// pg_replication_slots LEFT JOIN pg_stat_replication_slots subset is stable across
			// PG 14-18, so this gate verifies no schema divergence broke the 15-column shape.
			assert.Len(t, rows.FieldDescriptions(), wantNcols)
			rows.Close()
			assert.NoError(t, rows.Err())
		})
	}
}
