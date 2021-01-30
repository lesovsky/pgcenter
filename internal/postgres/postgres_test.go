package postgres

import (
	"fmt"
	"github.com/jackc/pgx/v4"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestNewConfig(t *testing.T) {
	var testcases = []struct {
		name   string
		valid  bool
		host   string
		port   int
		user   string
		dbname string
	}{
		{name: "all values", valid: true, host: "127.0.0.1", port: 5432, user: "postgres", dbname: "postgres"},
		{name: "no host", valid: true, port: 5432, user: "postgres", dbname: "postgres"},
		{name: "no port", valid: true, host: "127.0.0.1", user: "postgres", dbname: "postgres"},
		{name: "no user", valid: true, host: "127.0.0.1", port: 5432, dbname: "postgres"},
		{name: "no dbname", valid: true, host: "127.0.0.1", port: 5432, user: "postgres"},
		{name: "no host/port", valid: true, user: "postgres", dbname: "postgres"},
		{name: "no user/dbname", valid: true, host: "127.0.0.1", port: 5432},
		{name: "all empty", valid: true},
		{name: "unix socket", valid: true, host: "/var/run/postgresql"},
		{name: "invalid", valid: false, host: "invalid, invalid"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewConfig(tc.host, tc.port, tc.user, tc.dbname)
			if tc.valid {
				assert.NotEqual(t, Config{}, got)
				assert.NoError(t, err)
			} else {
				assert.Equal(t, Config{}, got)
				assert.Error(t, err)
				fmt.Println(err)
			}
		})
	}
}

func TestNewConfig_LibPQ_Env(t *testing.T) {
	testcases := []struct {
		envvar      string
		envvalue    string
		host        string
		port        int
		user        string
		dbname      string
		wantHost    string
		wantPort    int
		wantUser    string
		wantDbname  string
		wantOptions string
	}{
		{
			envvar: "PGHOST", envvalue: "1.2.3.4", host: "", port: 5432, user: "test", dbname: "testdb",
			wantHost: "1.2.3.4", wantPort: 5432, wantUser: "test", wantDbname: "testdb",
		},
		{
			envvar: "PGPORT", envvalue: "1122", host: "127.0.0.1", port: 1122, user: "test", dbname: "testdb",
			wantHost: "127.0.0.1", wantPort: 1122, wantUser: "test", wantDbname: "testdb",
		},
		{
			envvar: "PGUSER", envvalue: "example", host: "127.0.0.1", port: 5432, user: "", dbname: "testdb",
			wantHost: "127.0.0.1", wantPort: 5432, wantUser: "example", wantDbname: "testdb",
		},
		{
			envvar: "PGDATABASE", envvalue: "example_db", host: "127.0.0.1", port: 5432, user: "test", dbname: "",
			wantHost: "127.0.0.1", wantPort: 5432, wantUser: "test", wantDbname: "example_db",
		},
		{
			envvar: "PGOPTIONS", envvalue: "-c work_mem=100MB", host: "127.0.0.1", port: 5432, user: "test", dbname: "testdb",
			wantHost: "127.0.0.1", wantPort: 5432, wantUser: "test", wantDbname: "testdb", wantOptions: "-c work_mem=100MB",
		},
	}

	for _, tc := range testcases {
		assert.NoError(t, os.Setenv(tc.envvar, tc.envvalue))

		got, err := NewConfig(tc.host, tc.port, tc.user, tc.dbname)
		assert.NoError(t, err)
		assert.Equal(t, tc.wantHost, got.Config.Host)
		assert.Equal(t, tc.wantPort, int(got.Config.Port))
		assert.Equal(t, tc.wantUser, got.Config.User)
		assert.Equal(t, tc.wantDbname, got.Config.Database)
		assert.Equal(t, tc.wantOptions, got.Config.RuntimeParams["options"])
	}
}

func TestConnect(t *testing.T) {
	var testcases = []struct {
		name    string
		connStr string
		valid   bool
	}{
		{
			name:    "available postgres",
			connStr: "host=127.0.0.1 port=5432 user=postgres dbname=pgcenter_fixtures",
			valid:   true,
		},
		{
			name:    "unavailable postgres",
			connStr: "host=127.0.0.1 port=1 user=postgres dbname=pgcenter_fixtures",
			valid:   false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			config, err := pgx.ParseConfig(tc.connStr)
			assert.NoError(t, err)

			db, err := Connect(Config{Config: config})
			if tc.valid {
				assert.NoError(t, err)
				assert.NotNil(t, db)
				db.Close()
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestReconnect(t *testing.T) {
	c1, err := NewTestConnect()
	assert.NoError(t, err)

	c2, err := NewTestConnect()
	assert.NoError(t, err)

	var pid int
	assert.NoError(t, c1.QueryRow("SELECT pg_backend_pid()").Scan(&pid))

	_, err = c2.Exec("SELECT pg_terminate_backend($1)", pid)
	assert.NoError(t, err)

	err = c1.PQstatus()
	assert.Error(t, err)

	err = Reconnect(c1)
	assert.NoError(t, err)
	assert.NoError(t, c1.QueryRow("SELECT pg_backend_pid()").Scan(&pid))
	assert.Greater(t, pid, 0)

	c1.Close()
	c2.Close()
}

func TestDB_ALL(t *testing.T) {
	conn, err := NewTestConnect()
	assert.NoError(t, err)

	assert.NoError(t, conn.PQstatus())

	tag, err := conn.Exec("SELECT count(*) FROM pg_class")
	assert.NoError(t, err)
	assert.NotEqual(t, "", tag.String())

	var count int
	err = conn.QueryRow("SELECT count(*) FROM pg_class").Scan(&count)
	assert.NoError(t, err)
	assert.NotEqual(t, 0, count)

	rows, err := conn.Query("SELECT relname FROM pg_class")
	assert.NoError(t, err)
	assert.NotNil(t, rows)
	rows.Close()

	conn.Close()
}
