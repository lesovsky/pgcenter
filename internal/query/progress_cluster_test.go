package query

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_StatProgressClusterQueries(t *testing.T) {
	versions := []int{120000, 130000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_progress_cluster/%d", version), func(t *testing.T) {
			tmpl := PgStatProgressClusterDefault

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
