package top

import (
	"fmt"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_formatInfoString(t *testing.T) {
	testcases := []struct {
		cfg  postgres.Config
		want string
	}{
		{
			cfg:  postgres.Config{Config: &pgx.ConnConfig{Config: pgconn.Config{Host: "127.0.0.1", Port: 1234, User: "test", Database: "testdb"}}},
			want: "state [up]: 127.0.0.1:1234 test@testdb (ver: 13.1 on x86_64-~, up 01:23:48, recovery: f)",
		},
		{
			cfg:  postgres.Config{Config: &pgx.ConnConfig{Config: pgconn.Config{Host: "127.0.0.1", Port: 1234, User: "test", Database: ""}}},
			want: "state [up]: 127.0.0.1:1234 test@test (ver: 13.1 on x86_64-~, up 01:23:48, recovery: f)",
		},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, formatInfoString(tc.cfg, "up", "13.1 on x86_64-pc-linux-gnu Debian", "01:23:48", "f"))
	}
}

func Test_formatError(t *testing.T) {
	testcases := []struct {
		err  error
		want string
	}{
		{err: nil, want: ""},
		{
			err:  &pgconn.PgError{Severity: "TEST", Message: "test message", Detail: "test detail", Hint: "test hint"},
			want: "TEST: test message\nDETAIL: test detail\nHINT: test hint",
		},
		{err: fmt.Errorf("example error"), want: "ERROR: example error"},
	}

	for _, tc := range testcases {
		got := formatError(tc.err)
		assert.Equal(t, tc.want, got)
	}
}
