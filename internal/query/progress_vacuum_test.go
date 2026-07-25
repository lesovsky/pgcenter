package query

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_StatProgressVacuumQueries(t *testing.T) {
	versions := []int{90600, 100000, 110000, 120000, 130000, 140000, 150000, 160000, 170000, 180000, 190000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_progress_vacuum/%d", version), func(t *testing.T) {
			tmpl, wantNcols, _ := SelectStatProgressVacuumQuery(version)

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

func Test_SelectStatProgressVacuumQuery(t *testing.T) {
	testcases := []struct {
		version int
		wantQ   string
		wantN   int
		wantD   [2]int
	}{
		{version: 140000, wantQ: PgStatProgressVacuumDefault, wantN: 13, wantD: [2]int{10, 11}},
		{version: 180000, wantQ: PgStatProgressVacuumDefault, wantN: 13, wantD: [2]int{10, 11}},
		{version: 190000, wantQ: PgStatProgressVacuumPG19, wantN: 15, wantD: [2]int{12, 13}},
	}

	for _, tc := range testcases {
		gotQ, gotN, gotD := SelectStatProgressVacuumQuery(tc.version)
		assert.Equal(t, tc.wantQ, gotQ)
		assert.Equal(t, tc.wantN, gotN)
		assert.Equal(t, tc.wantD, gotD)
	}
}
