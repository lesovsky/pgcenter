package postgres

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewDB(t *testing.T) {
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
		{name: "test", valid: false, host: "test, test"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewDB(tc.host, tc.port, tc.user, tc.dbname)
			if tc.valid {
				assert.NotNil(t, got)
				assert.NoError(t, err)
			} else {
				assert.Nil(t, got)
				assert.Error(t, err)
				fmt.Println(err)
			}
		})
	}
}
