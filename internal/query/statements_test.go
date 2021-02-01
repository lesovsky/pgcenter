package query

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSelectStatStatementsTimingQuery(t *testing.T) {
	testcases := []struct {
		version int
		want    string
	}{
		{version: 90500, want: PgStatStatementsTimingPG12},
		{version: 90600, want: PgStatStatementsTimingPG12},
		{version: 100000, want: PgStatStatementsTimingPG12},
		{version: 110000, want: PgStatStatementsTimingPG12},
		{version: 120000, want: PgStatStatementsTimingPG12},
		{version: 130000, want: PgStatStatementsTimingDefault},
	}

	for _, tc := range testcases {
		got := SelectStatStatementsTimingQuery(tc.version)
		assert.Equal(t, tc.want, got)
	}
}

func Test_StatStatementsQueries(t *testing.T) {
	versions := []int{90500, 90600, 100000, 110000, 120000, 130000}

	queries := []string{
		PgStatStatementsGeneralDefault,
		PgStatStatementsIoDefault,
		PgStatStatementsTempDefault,
		PgStatStatementsLocalDefault,
	}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_statements/%d", version), func(t *testing.T) {
			for _, query := range queries {
				opts := NewOptions(version, "f", 256)
				q, err := Format(query, opts)
				assert.NoError(t, err)

				conn, err := postgres.NewTestConnectVersion(version)
				assert.NoError(t, err)

				_, err = conn.Exec(q)
				assert.NoError(t, err)

				conn.Close()
			}
		})
	}

	t.Run("pg_stat_statements_timing", func(t *testing.T) {
		for _, version := range versions {
			tmpl := SelectStatStatementsTimingQuery(version)
			opts := NewOptions(version, "f", 256)
			q, err := Format(tmpl, opts)
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			assert.NoError(t, err)

			_, err = conn.Exec(q)
			assert.NoError(t, err)

			conn.Close()
		}
	})
}

func TestSelectQueryReportQuery(t *testing.T) {
	testcases := []struct {
		version int
		want    string
	}{
		{version: 90500, want: PgStatStatementsReportQueryPG12},
		{version: 90600, want: PgStatStatementsReportQueryPG12},
		{version: 100000, want: PgStatStatementsReportQueryPG12},
		{version: 110000, want: PgStatStatementsReportQueryPG12},
		{version: 120000, want: PgStatStatementsReportQueryPG12},
		{version: 130000, want: PgStatStatementsReportQueryDefault},
	}

	for _, tc := range testcases {
		got := SelectQueryReportQuery(tc.version)
		assert.Equal(t, tc.want, got)
	}
}

func Test_StatStatementsReportQueries(t *testing.T) {
	versions := []int{90500, 90600, 100000, 110000, 120000, 130000}

	for _, version := range versions {
		tmpl := SelectQueryReportQuery(version)
		opts := NewOptions(version, "f", 256)
		q, err := Format(tmpl, opts)
		assert.NoError(t, err)

		conn, err := postgres.NewTestConnectVersion(version)
		assert.NoError(t, err)

		// Use fake query_id, just test queries are executed with no errors.
		_, err = conn.Exec(q, "1234567890")
		assert.NoError(t, err)

		conn.Close()
	}
}
