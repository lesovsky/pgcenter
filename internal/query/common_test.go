package query

import (
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSelectActivityAutovacuumQuery(t *testing.T) {
	testcases := []struct {
		version int
		want    string
	}{
		{version: 90300, want: SelectAutovacuumPG93},
		{version: 90400, want: SelectAutovacuumDefault},
		{version: 90500, want: SelectAutovacuumDefault},
		{version: 90600, want: SelectAutovacuumDefault},
		{version: 100000, want: SelectAutovacuumDefault},
		{version: 110000, want: SelectAutovacuumDefault},
		{version: 120000, want: SelectAutovacuumDefault},
		{version: 130000, want: SelectAutovacuumDefault},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, SelectActivityAutovacuumQuery(tc.version))
	}
}

func TestSelectActivityActivityQuery(t *testing.T) {
	testcases := []struct {
		version int
		want    string
	}{
		{version: 90300, want: SelectActivityPG93},
		{version: 90400, want: SelectActivityPG95},
		{version: 90500, want: SelectActivityPG95},
		{version: 90600, want: SelectActivityPG96},
		{version: 100000, want: SelectActivityDefault},
		{version: 110000, want: SelectActivityDefault},
		{version: 120000, want: SelectActivityDefault},
		{version: 130000, want: SelectActivityDefault},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, SelectActivityActivityQuery(tc.version))
	}
}

func TestSelectActivityStatementsQuery(t *testing.T) {
	testcases := []struct {
		version int
		want    string
	}{
		{version: 120000, want: SelectActivityStatementsPG12},
		{version: 130000, want: SelectActivityStatementsLatest},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, SelectActivityStatementsQuery(tc.version))
	}
}

func Test_CommonQueries(t *testing.T) {
	versions := []int{90500, 90600, 100000, 110000, 120000, 130000}

	queries := []struct {
		query string
		args  []interface{}
	}{
		{query: GetSetting, args: []interface{}{"work_mem"}},
		{query: GetRecoveryStatus},
		{query: GetUptime},
		{query: CheckSchemaExists, args: []interface{}{"public"}},
		{query: CheckExtensionExists, args: []interface{}{"plpgsql"}},
		{query: GetAllSettings},
		{query: ExecReloadConf},
		{query: ExecResetStats},
		{query: ExecResetPgStatStatements},
		{query: SelectCommonProperties},
	}

	t.Run("common_queries", func(t *testing.T) {
		for _, version := range versions {
			for _, query := range queries {
				conn, err := postgres.NewTestConnectVersion(version)
				assert.NoError(t, err)

				_, err = conn.Exec(query.query, query.args...)
				assert.NoError(t, err)

				conn.Close()
			}
		}
	})

	// GetCurrentLogfile available since Postgres 10
	t.Run("current_logfiles", func(t *testing.T) {
		for _, version := range versions[3:] {
			conn, err := postgres.NewTestConnectVersion(version)
			assert.NoError(t, err)
			_, err = conn.Exec(GetCurrentLogfile)
			assert.NoError(t, err)
			conn.Close()
		}
	})

	t.Run("activity_activity_queries", func(t *testing.T) {
		for _, version := range versions {
			conn, err := postgres.NewTestConnectVersion(version)
			assert.NoError(t, err)

			_, err = conn.Exec(SelectActivityActivityQuery(version))
			assert.NoError(t, err)

			conn.Close()
		}
	})

	t.Run("activity_autovacuum_queries", func(t *testing.T) {
		for _, version := range versions {
			conn, err := postgres.NewTestConnectVersion(version)
			assert.NoError(t, err)

			_, err = conn.Exec(SelectActivityAutovacuumQuery(version))
			assert.NoError(t, err)

			conn.Close()
		}
	})

}
