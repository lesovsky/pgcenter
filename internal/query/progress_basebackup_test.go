package query

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_StatProgressBasebackupQueries(t *testing.T) {
	versions := []int{140000, 150000, 160000, 170000, 180000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_progress_basebackup/%d", version), func(t *testing.T) {
			tmpl := PgStatProgressBasebackupDefault

			opts := NewOptions(version, "f", "off", 256, "public")
			q, err := Format(tmpl, opts)
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}

			_, err = conn.Exec(q)
			assert.NoError(t, err)

			conn.Close()
		})
	}
}
