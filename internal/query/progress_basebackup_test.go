package query

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_StatProgressBasebackupQueries(t *testing.T) {
	versions := []int{140000, 150000, 160000, 170000, 180000, 190000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_progress_basebackup/%d", version), func(t *testing.T) {
			tmpl, wantNcols, _ := SelectStatProgressBasebackupQuery(version)

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

func Test_SelectStatProgressBasebackupQuery(t *testing.T) {
	testcases := []struct {
		version int
		wantQ   string
		wantN   int
		wantD   [2]int
	}{
		{version: 140000, wantQ: PgStatProgressBasebackupDefault, wantN: 11, wantD: [2]int{9, 9}},
		{version: 180000, wantQ: PgStatProgressBasebackupDefault, wantN: 11, wantD: [2]int{9, 9}},
		{version: 190000, wantQ: PgStatProgressBasebackupPG19, wantN: 12, wantD: [2]int{10, 10}},
	}

	for _, tc := range testcases {
		gotQ, gotN, gotD := SelectStatProgressBasebackupQuery(tc.version)
		assert.Equal(t, tc.wantQ, gotQ)
		assert.Equal(t, tc.wantN, gotN)
		assert.Equal(t, tc.wantD, gotD)
	}
}
