package query

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_StatProgressVacuumQueries(t *testing.T) {
	versions := []int{90600, 100000, 110000, 120000, 130000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_progress_vacuum/%d", version), func(t *testing.T) {
			tmpl := PgStatProgressVacuumDefault

			opts := NewOptions(version, "f", 0, "top")
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
