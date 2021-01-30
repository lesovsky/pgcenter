package top

import (
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
			cfg:  postgres.Config{&pgx.ConnConfig{Config: pgconn.Config{Host: "127.0.0.1", Port: 5432, User: "test", Database: "testdb"}}},
			want: "state [up]: 127.0.0.1:5432 test@testdb (ver: 13.1 on x86_64-~, up 01:23:48, recovery: f)",
		},
		{
			cfg:  postgres.Config{&pgx.ConnConfig{Config: pgconn.Config{Host: "127.0.0.1", Port: 5432, User: "test", Database: ""}}},
			want: "state [up]: 127.0.0.1:5432 test@test (ver: 13.1 on x86_64-~, up 01:23:48, recovery: f)",
		},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, formatInfoString(tc.cfg, "up", "13.1 on x86_64-pc-linux-gnu Debian", "01:23:48", "f"))
	}
}
