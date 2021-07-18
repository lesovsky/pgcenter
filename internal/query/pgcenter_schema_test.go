package query

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_QueryPgcenterSchema(t *testing.T) {
	queries := []string{
		"SELECT * FROM pgcenter.sys_proc_diskstats",
		"SELECT * FROM pgcenter.sys_proc_loadavg",
		"SELECT * FROM pgcenter.sys_proc_meminfo",
		"SELECT * FROM pgcenter.sys_proc_netdev",
		"SELECT * FROM pgcenter.sys_proc_stat",
		"SELECT * FROM pgcenter.sys_proc_uptime",
		"SELECT * FROM pgcenter.sys_proc_mounts",
	}

	versions := []int{90500, 90600, 100000, 110000, 120000, 130000, 140000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("query-pgcenter-schema/%d", version), func(t *testing.T) {
			conn, err := postgres.NewTestConnectVersion(version)
			assert.NoError(t, err)

			for _, q := range queries {
				_, err = conn.Exec(q)
				assert.NoError(t, err)
			}

			conn.Close()
		})
	}
}
