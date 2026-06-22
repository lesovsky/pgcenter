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
		{version: 130000, want: PgStatStatementsTimingPG13},
		{version: 140000, want: PgStatStatementsTimingPG13},
		{version: 160000, want: PgStatStatementsTimingPG13},
		// PG 17+: blk_read_time/blk_write_time replaced by shared_blk_read_time etc.
		{version: 170000, want: PgStatStatementsTimingDefault},
		{version: 180000, want: PgStatStatementsTimingDefault},
	}

	for _, tc := range testcases {
		got := SelectStatStatementsTimingQuery(tc.version)
		assert.Equal(t, tc.want, got, "version %d", tc.version)
	}
}

func TestSelectStatStatementsJITQuery(t *testing.T) {
	testcases := []struct {
		version   int
		wantQuery string
		wantNcols int
		wantDiff  [2]int
		wantKey   int
	}{
		{version: 150000, wantQuery: PgStatStatementsJITPG15, wantNcols: 13, wantDiff: [2]int{6, 10}, wantKey: 11},
		{version: 160000, wantQuery: PgStatStatementsJITPG15, wantNcols: 13, wantDiff: [2]int{6, 10}, wantKey: 11},
		// PG 17+: adds jit_deform_count/jit_deform_time columns.
		{version: 170000, wantQuery: PgStatStatementsJITDefault, wantNcols: 15, wantDiff: [2]int{7, 12}, wantKey: 13},
		{version: 180000, wantQuery: PgStatStatementsJITDefault, wantNcols: 15, wantDiff: [2]int{7, 12}, wantKey: 13},
	}

	for _, tc := range testcases {
		gotQuery, gotNcols, gotDiff, gotKey := SelectStatStatementsJITQuery(tc.version)
		assert.Equal(t, tc.wantQuery, gotQuery, "version %d", tc.version)
		assert.Equal(t, tc.wantNcols, gotNcols, "version %d", tc.version)
		assert.Equal(t, tc.wantDiff, gotDiff, "version %d", tc.version)
		assert.Equal(t, tc.wantKey, gotKey, "version %d", tc.version)
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

	// JIT stats are available since PG 15; test all supported versions.
	for _, version := range []int{150000, 160000, 170000, 180000} {
		version := version
		t.Run(fmt.Sprintf("pg_stat_statements_jit/%d", version), func(t *testing.T) {
			tmpl, _, _, _ := SelectStatStatementsJITQuery(version)
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
		{version: 130000, want: PgStatStatementsReportQueryPG13},
		{version: 140000, want: PgStatStatementsReportQueryPG13},
		{version: 160000, want: PgStatStatementsReportQueryPG13},
		// PG 17+: blk_read_time/blk_write_time replaced by shared_blk_read_time etc.
		{version: 170000, want: PgStatStatementsReportQueryDefault},
		{version: 180000, want: PgStatStatementsReportQueryDefault},
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
