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
		// The PG 13 boundary is not verifiable against a live server: the test image carries PG 14-19
		// only, so a branch written as '>= 140000' would pass every live check. These cases are the
		// only guard and therefore pin the boundary from both sides.
		{version: 120000, wantQ: PgStatActivityDefault, wantN: 14},
		{version: 130000, wantQ: PgStatActivityPG13, wantN: 17},
		{version: 190000, wantQ: PgStatActivityPG13, wantN: 17},
	}

	for _, tc := range testcases {
		gotQ, gotN := SelectStatActivityQuery(tc.version)
		assert.Equal(t, tc.wantQ, gotQ)
		assert.Equal(t, tc.wantN, gotN)
	}
}

func Test_StatActivityQueries(t *testing.T) {
	versions := []int{90500, 90600, 100000, 110000, 120000, 130000, 140000, 150000, 160000, 170000, 180000, 190000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_activity/%d", version), func(t *testing.T) {
			tmpl, wantNcols := SelectStatActivityQuery(version)

			opts := NewOptions(version, "f", "off", 256, "public")
			q, err := Format(tmpl, opts)
			assert.NoError(t, err)

			wantNames := wantStatActivityColumns(version)
			assert.Len(t, wantNames, wantNcols)

			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}
			defer conn.Close()

			rows, err := conn.Query(q)
			assert.NoError(t, err)

			// Executing the query is not enough: the claimed layout only becomes a measured one when
			// the server is asked which columns it returns and in which order.
			var gotNames []string
			for _, d := range rows.FieldDescriptions() {
				gotNames = append(gotNames, d.Name)
			}
			assert.Equal(t, wantNames, gotNames)

			// Names and order alone would let the semantics of the three new columns drift
			// unnoticed: replacing coalesce(leader_pid, pid) with a raw leader_pid, or wrapping
			// backend_xid in a coalesce(..., '0'), keeps the layout identical while breaking the
			// two rules the feature exists for. Read the values back and pin them.
			if version >= PostgresV13 {
				var rowsSeen int
				for rows.Next() {
					values, err := rows.Values()
					assert.NoError(t, err)
					rowsSeen++

					// leader is derived, never the raw leader_pid: the connection running this
					// test is nobody's parallel worker, so a raw leader_pid would be NULL here.
					assert.NotNil(t, values[1], "leader must never be NULL — raw leader_pid is NULL for a non-worker")

					// backend_xid stays blank until the transaction writes. This query does not,
					// so a non-nil value here means a coalesce crept in and "blank, never 0" is gone.
					assert.Nil(t, values[11], "backend_xid must be NULL for a transaction that has not written")
				}
				assert.NotZero(t, rowsSeen, "query returned no rows, so the value assertions proved nothing")
			}

			rows.Close()
			assert.NoError(t, rows.Err())
		})
	}
}

// wantStatActivityColumns returns the column names the activity query is expected to produce, in
// order. Boundaries mirror SelectStatActivityQuery. The lists below PG 14 are unreachable in the
// current test environment, but they are kept so that the expectation is selected before the skip
// rather than after it.
func wantStatActivityColumns(version int) []string {
	switch {
	case version < 90600:
		return []string{"pid", "cl_addr", "cl_port", "datname", "usename", "appname", "waiting",
			"state", "xact_age", "query_age", "change_age", "query"}
	case version < 100000:
		return []string{"pid", "cl_addr", "cl_port", "datname", "usename", "appname", "wait_etype",
			"wait_event", "state", "xact_age", "query_age", "change_age", "query"}
	case version < 130000:
		return []string{"pid", "cl_addr", "cl_port", "datname", "usename", "appname", "backend_type",
			"wait_etype", "wait_event", "state", "xact_age", "query_age", "change_age", "query"}
	default:
		return []string{"pid", "leader", "cl_addr", "cl_port", "datname", "usename", "appname",
			"backend_type", "wait_etype", "wait_event", "state", "backend_xid", "horizon_xacts",
			"xact_age", "query_age", "change_age", "query"}
	}
}
