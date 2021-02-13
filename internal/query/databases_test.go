package query

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSelectStatDatabaseQuery(t *testing.T) {
	testcases := []struct {
		version int
		wantQ   string
		wantN   int
		wantD   [2]int
	}{
		{version: 90500, wantQ: PgStatDatabasePG11, wantN: 17, wantD: [2]int{1, 15}},
		{version: 90600, wantQ: PgStatDatabasePG11, wantN: 17, wantD: [2]int{1, 15}},
		{version: 100000, wantQ: PgStatDatabasePG11, wantN: 17, wantD: [2]int{1, 15}},
		{version: 110000, wantQ: PgStatDatabasePG11, wantN: 17, wantD: [2]int{1, 15}},
		{version: 120000, wantQ: PgStatDatabaseDefault, wantN: 18, wantD: [2]int{1, 16}},
		{version: 130000, wantQ: PgStatDatabaseDefault, wantN: 18, wantD: [2]int{1, 16}},
	}

	for _, tc := range testcases {
		gotQ, gotN, gotD := SelectStatDatabaseQuery(tc.version)
		assert.Equal(t, tc.wantQ, gotQ)
		assert.Equal(t, tc.wantN, gotN)
		assert.Equal(t, tc.wantD, gotD)
	}
}

func Test_StatDatabaseQueries(t *testing.T) {
	versions := []int{90500, 90600, 100000, 110000, 120000, 130000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_database/%d", version), func(t *testing.T) {
			tmpl, _, _ := SelectStatDatabaseQuery(version)

			opts := NewOptions(version, "f", "off", 256)
			q, err := Format(tmpl, opts)
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			assert.NoError(t, err)

			_, err = conn.Exec(q)
			assert.NoError(t, err)

			conn.Close()
		})
	}
}
