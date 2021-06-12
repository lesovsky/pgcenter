package query

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

// Test_StatWALQueries tests query, executing it against all supported Postgres versions.
func Test_StatWALQueries(t *testing.T) {
	versions := []int{140000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_wal/%d", version), func(t *testing.T) {
			tmpl := PgStatWALDefault

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
