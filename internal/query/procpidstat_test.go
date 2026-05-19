package query

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
)

func TestPgStatActivityProcPidStat(t *testing.T) {
	opts := NewOptions(170000, "f", "off", 256, "public")
	q, err := Format(PgStatActivityProcPidStat, opts)
	assert.NoError(t, err)
	assert.NotEmpty(t, q)

	// Sanity: the 7 expected columns (in order) appear in the formatted query.
	wantInOrder := []string{
		"SELECT pid",
		"AS datname",
		"AS usename",
		"AS state",
		"AS wait_etype",
		"AS wait_event",
		"AS query",
	}
	idx := 0
	for _, fragment := range wantInOrder {
		pos := strings.Index(q[idx:], fragment)
		assert.GreaterOrEqualf(t, pos, 0, "fragment %q not found in expected order", fragment)
		idx += pos + len(fragment)
	}

	// Required clauses.
	assert.Contains(t, q, "FROM pg_stat_activity")
	assert.Contains(t, q, "WHERE pid != pg_backend_pid()")
	assert.Contains(t, q, "ORDER BY pid")
	assert.Contains(t, q, "regexp_replace(coalesce(query, '')")
}

func TestPgStatActivityProcPidStat_ShowNoIdle(t *testing.T) {
	opts := NewOptions(170000, "f", "off", 256, "public")
	opts.ShowNoIdle = true

	q, err := Format(PgStatActivityProcPidStat, opts)
	assert.NoError(t, err)
	assert.Contains(t, q, "AND state != 'idle'")
}

func TestPgStatActivityProcPidStat_ShowNoIdleDisabled(t *testing.T) {
	opts := NewOptions(170000, "f", "off", 256, "public")
	opts.ShowNoIdle = false

	q, err := Format(PgStatActivityProcPidStat, opts)
	assert.NoError(t, err)
	assert.NotContains(t, q, "AND state != 'idle'")
}

func TestPgStatActivityProcPidStat_QueryAgeThresh(t *testing.T) {
	opts := NewOptions(170000, "f", "off", 256, "public")
	opts.QueryAgeThresh = "00:05:00"

	q, err := Format(PgStatActivityProcPidStat, opts)
	assert.NoError(t, err)
	assert.Contains(t, q, "00:05:00")
	assert.Contains(t, q, "::interval")
}

func TestPgStatActivityProcPidStat_QueryAgeThreshDefault(t *testing.T) {
	// Default options must still embed QueryAgeThresh (no {{ if }} guard).
	opts := NewOptions(170000, "f", "off", 256, "public")

	q, err := Format(PgStatActivityProcPidStat, opts)
	assert.NoError(t, err)
	assert.Contains(t, q, "00:00:00.0")
	assert.Contains(t, q, "::interval")
}

func Test_StatProcPidStatQuery(t *testing.T) {
	versions := []int{90500, 90600, 100000, 110000, 120000, 130000, 140000, 150000, 160000, 170000, 180000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_activity/procpidstat/%d", version), func(t *testing.T) {
			opts := NewOptions(version, "f", "off", 256, "public")
			q, err := Format(PgStatActivityProcPidStat, opts)
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
