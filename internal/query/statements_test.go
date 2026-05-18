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
		{version: 140000, want: PgStatStatementsTimingDefault},
		{version: 160000, want: PgStatStatementsTimingDefault},
		// PG 17+: blk_read_time/blk_write_time replaced by shared_blk_read_time etc.
		{version: 170000, want: PgStatStatementsTimingPG17},
		{version: 180000, want: PgStatStatementsTimingPG17},
	}

	for _, tc := range testcases {
		got := SelectStatStatementsTimingQuery(tc.version)
		assert.Equal(t, tc.want, got, "version %d", tc.version)
	}
}

func Test_StatStatementsQueries(t *testing.T) {
	versions := []int{90500, 90600, 100000, 110000, 120000, 130000, 140000, 150000, 160000, 170000, 180000}

	queries := []string{
		PgStatStatementsGeneralDefault,
		PgStatStatementsIoDefault,
		PgStatStatementsTempDefault,
		PgStatStatementsLocalDefault,
	}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_statements/%d", version), func(t *testing.T) {
			for _, query := range queries {
				opts := NewOptions(version, "f", "off", 256, "public")
				q, err := Format(query, opts)
				assert.NoError(t, err)

				conn, err := postgres.NewTestConnectVersion(version)
				if err != nil {
					t.Skipf("postgres %d not available in test environment", version)
				}

				_, err = conn.Exec(q)
				assert.NoError(t, err)

				conn.Close()
			}
		})
	}

	// Each version in its own sub-test so a missing PG instance skips only that version,
	// not the entire timing suite (fixes the t.Skipf-in-loop bug).
	for _, version := range versions {
		version := version
		t.Run(fmt.Sprintf("pg_stat_statements_timing/%d", version), func(t *testing.T) {
			tmpl := SelectStatStatementsTimingQuery(version)
			opts := NewOptions(version, "f", "off", 256, "public")
			q, err := Format(tmpl, opts)
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}
			defer conn.Close()

			_, err = conn.Exec(q)
			assert.NoError(t, err)
		})
	}

	// WAL stats are available since PG 13; test all supported versions.
	for _, version := range []int{130000, 140000, 150000, 160000, 170000, 180000} {
		version := version
		t.Run(fmt.Sprintf("pg_stat_statements_wal/%d", version), func(t *testing.T) {
			opts := NewOptions(version, "f", "off", 256, "public")
			q, err := Format(PgStatStatementsWalDefault, opts)
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}
			defer conn.Close()

			_, err = conn.Exec(q)
			assert.NoError(t, err)
		})
	}
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
		{version: 140000, want: PgStatStatementsReportQueryDefault},
		{version: 160000, want: PgStatStatementsReportQueryDefault},
		// PG 17+: blk_read_time/blk_write_time replaced by shared_blk_read_time etc.
		{version: 170000, want: PgStatStatementsReportQueryPG17},
		{version: 180000, want: PgStatStatementsReportQueryPG17},
	}

	for _, tc := range testcases {
		got := SelectQueryReportQuery(tc.version)
		assert.Equal(t, tc.want, got, "version %d", tc.version)
	}
}

// Test_StatStatementsReportQueries runs each version in its own sub-test so a missing
// PG instance skips only that version, not the entire suite (fixes the t.Skipf-in-loop bug).
func Test_StatStatementsReportQueries(t *testing.T) {
	versions := []int{90500, 90600, 100000, 110000, 120000, 130000, 140000, 150000, 160000, 170000, 180000}

	for _, version := range versions {
		version := version
		t.Run(fmt.Sprintf("version/%d", version), func(t *testing.T) {
			tmpl := SelectQueryReportQuery(version)
			opts := NewOptions(version, "f", "off", 256, "public")
			q, err := Format(tmpl, opts)
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}
			defer conn.Close()

			// Use fake query_id, just test queries are executed with no errors.
			_, err = conn.Exec(q, "1234567890")
			assert.NoError(t, err)
		})
	}
}
