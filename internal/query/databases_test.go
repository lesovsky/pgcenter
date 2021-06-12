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
		{version: 90500, wantQ: PgStatDatabaseGeneralPG11, wantN: 18, wantD: [2]int{2, 16}},
		{version: 90600, wantQ: PgStatDatabaseGeneralPG11, wantN: 18, wantD: [2]int{2, 16}},
		{version: 100000, wantQ: PgStatDatabaseGeneralPG11, wantN: 18, wantD: [2]int{2, 16}},
		{version: 110000, wantQ: PgStatDatabaseGeneralPG11, wantN: 18, wantD: [2]int{2, 16}},
		{version: 120000, wantQ: PgStatDatabaseGeneralDefault, wantN: 19, wantD: [2]int{2, 17}},
		{version: 130000, wantQ: PgStatDatabaseGeneralDefault, wantN: 19, wantD: [2]int{2, 17}},
	}

	for _, tc := range testcases {
		gotQ, gotN, gotD := SelectStatDatabaseGeneralQuery(tc.version)
		assert.Equal(t, tc.wantQ, gotQ)
		assert.Equal(t, tc.wantN, gotN)
		assert.Equal(t, tc.wantD, gotD)
	}
}

func Test_SelectStatDatabaseGeneralQuery(t *testing.T) {
	versions := []int{90500, 90600, 100000, 110000, 120000, 130000, 140000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_database/general/%d", version), func(t *testing.T) {
			tmpl, _, _ := SelectStatDatabaseGeneralQuery(version)

			opts := NewOptions(version, "f", "off", 256, "public")
			q, err := Format(tmpl, opts)
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			assert.NoError(t, err)

			_, err = conn.Exec(q)
			assert.NoError(t, err)

			conn.Close()
		})
	}

	for _, version := range []int{140000} {
		t.Run(fmt.Sprintf("pg_stat_database/sessions/%d", version), func(t *testing.T) {
			tmpl := PgStatDatabaseSessionsDefault

			opts := NewOptions(version, "f", "off", 256, "public")
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
