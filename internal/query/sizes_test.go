package query

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_StatSizesQueries(t *testing.T) {
	versions := []int{90500, 90600, 100000, 110000, 120000, 130000, 140000, 150000, 160000, 170000, 180000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("sizes/%d", version), func(t *testing.T) {
			tmpl := PgTablesSizesDefault

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

// Test_StatSizesQueries_NonDefaultSchema reproduces issue #116: the old query used
// (schemaname||'.'||relname)::regclass which failed for tables in non-default schemas
// when the name required quoting (mixed case, special chars) or the schema wasn't in
// search_path. The fix was to use s.relid (OID) instead.
func Test_StatSizesQueries_NonDefaultSchema(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	if err != nil {
		t.Skipf("postgres not available in test environment")
	}
	defer conn.Close()

	// Create a non-default schema and a table inside it.
	_, err = conn.Exec(`CREATE SCHEMA IF NOT EXISTS test_dbo`)
	assert.NoError(t, err)

	_, err = conn.Exec(`CREATE TABLE IF NOT EXISTS test_dbo.t1hlog (id int)`)
	assert.NoError(t, err)

	defer func() {
		_, _ = conn.Exec(`DROP TABLE IF EXISTS test_dbo.t1hlog`)
		_, _ = conn.Exec(`DROP SCHEMA IF EXISTS test_dbo`)
	}()

	opts := NewOptions(170000, "f", "off", 256, "public")
	q, err := Format(PgTablesSizesDefault, opts)
	assert.NoError(t, err)

	// Must not error with "relation does not exist" for tables in non-default schemas.
	_, err = conn.Exec(q)
	assert.NoError(t, err)
}
