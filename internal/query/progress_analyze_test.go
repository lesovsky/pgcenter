package query

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_StatProgressAnalyzeQueries(t *testing.T) {
	versions := []int{140000, 150000, 160000, 170000, 180000, 190000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_progress_analyze/%d", version), func(t *testing.T) {
			tmpl, wantNcols, _ := SelectStatProgressAnalyzeQuery(version)

			opts := NewOptions(version, "f", "off", 256, "public")
			q, err := Format(tmpl, opts)
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}

			rows, err := conn.Query(q)
			assert.NoError(t, err)
			// Executing the query is not enough: assert the column count the server returns matches
			// what the selector promises, so a version branch that silently returns the wrong layout
			// cannot pass.
			assert.Equal(t, wantNcols, len(rows.FieldDescriptions()))
			rows.Close()

			conn.Close()
		})
	}
}

func Test_SelectStatProgressAnalyzeQuery(t *testing.T) {
	testcases := []struct {
		version int
		wantQ   string
		wantN   int
		wantD   [2]int
	}{
		{version: 140000, wantQ: PgStatProgressAnalyzeDefault, wantN: 12, wantD: [2]int{0, 0}},
		{version: 180000, wantQ: PgStatProgressAnalyzeDefault, wantN: 12, wantD: [2]int{0, 0}},
		{version: 190000, wantQ: PgStatProgressAnalyzePG19, wantN: 13, wantD: [2]int{0, 0}},
	}

	for _, tc := range testcases {
		gotQ, gotN, gotD := SelectStatProgressAnalyzeQuery(tc.version)
		assert.Equal(t, tc.wantQ, gotQ)
		assert.Equal(t, tc.wantN, gotN)
		assert.Equal(t, tc.wantD, gotD)
	}
}
