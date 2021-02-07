package query

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSelectStatActivityQuery(t *testing.T) {
	testcases := []struct {
		version int
		wantQ   string
		wantN   int
	}{
		{version: 90500, wantQ: PgStatActivity95, wantN: 12},
		{version: 90600, wantQ: PgStatActivity96, wantN: 13},
		{version: 100000, wantQ: PgStatActivityDefault, wantN: 14},
	}

	for _, tc := range testcases {
		gotQ, gotN := SelectStatActivityQuery(tc.version)
		assert.Equal(t, tc.wantQ, gotQ)
		assert.Equal(t, tc.wantN, gotN)
	}
}

func Test_StatActivityQueries(t *testing.T) {
	versions := []int{90500, 90600, 100000, 110000, 120000, 130000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_activity/%d", version), func(t *testing.T) {
			tmpl, _ := SelectStatActivityQuery(version)

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
