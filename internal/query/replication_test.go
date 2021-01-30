package query

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSelectStatReplicationQuery(t *testing.T) {
	testcases := []struct {
		version int
		track   bool
		wantQ   string
		wantN   int
	}{
		{version: 90500, track: false, wantQ: PgStatReplication96, wantN: 12},
		{version: 90500, track: true, wantQ: PgStatReplication96Extended, wantN: 14},
		{version: 90600, track: false, wantQ: PgStatReplication96, wantN: 12},
		{version: 90600, track: true, wantQ: PgStatReplication96Extended, wantN: 14},
		{version: 100000, track: false, wantQ: PgStatReplicationDefault, wantN: 15},
		{version: 100000, track: true, wantQ: PgStatReplicationExtended, wantN: 17},
		{version: 110000, track: false, wantQ: PgStatReplicationDefault, wantN: 15},
		{version: 110000, track: true, wantQ: PgStatReplicationExtended, wantN: 17},
		{version: 120000, track: false, wantQ: PgStatReplicationDefault, wantN: 15},
		{version: 120000, track: true, wantQ: PgStatReplicationExtended, wantN: 17},
		{version: 130000, track: false, wantQ: PgStatReplicationDefault, wantN: 15},
		{version: 130000, track: true, wantQ: PgStatReplicationExtended, wantN: 17},
	}

	for _, tc := range testcases {
		gotQ, gotN := SelectStatReplicationQuery(tc.version, tc.track)
		assert.Equal(t, tc.wantQ, gotQ)
		assert.Equal(t, tc.wantN, gotN)
	}
}

func Test_StatReplicationQueries(t *testing.T) {
	versions := []int{90500, 90600, 100000, 110000, 120000, 130000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_replication/%d", version), func(t *testing.T) {
			tmpl1, _ := SelectStatReplicationQuery(version, false)
			tmpl2, _ := SelectStatReplicationQuery(version, true)

			opts := Options{}
			opts.Configure(version, "f", "top")
			q1, err := Format(tmpl1, opts)
			assert.NoError(t, err)

			q2, err := Format(tmpl2, opts)
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			assert.NoError(t, err)

			_, err = conn.Exec(q1)
			assert.NoError(t, err)

			_, err = conn.Exec(q2)
			assert.NoError(t, err)

			conn.Close()
		})
	}
}
