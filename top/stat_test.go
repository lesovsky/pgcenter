package top

import (
	"database/sql"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
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

// Test_alignViewToResult reproduces issue #99: pressing 'x' to cycle pg_stat_statements
// views caused "slice bounds out of range [:-1]" / "zero or negative width, skip".
//
// Root cause: after a view switch, the first stat batch may still carry the OLD view's
// column count. SetAlign fires on it (Aligned=false), populating ColsWidth for N columns.
// The next batch has M > N columns from the new view, but Aligned=true skips SetAlign,
// so ColsWidth[N..M-1] returns 0 → panic or error on truncation.
//
// Fix: alignViewToResult also re-aligns when len(ColsWidth) != r.Ncols.
func Test_alignViewToResult(t *testing.T) {
	makeResult := func(ncols int) stat.PGresult {
		cols := make([]string, ncols)
		row := make([]sql.NullString, ncols)
		for i := 0; i < ncols; i++ {
			cols[i] = fmt.Sprintf("col%d", i+1)
			row[i] = sql.NullString{String: "value", Valid: true}
		}
		return stat.PGresult{Valid: true, Ncols: ncols, Nrows: 1, Cols: cols,
			Values: [][]sql.NullString{row}}
	}

	t.Run("first render sets alignment", func(t *testing.T) {
		cfg := &config{view: view.View{Aligned: false, ColsWidth: map[int]int{}}}
		alignViewToResult(cfg, makeResult(8))
		assert.True(t, cfg.view.Aligned)
		assert.Equal(t, 8, len(cfg.view.ColsWidth))
	})

	t.Run("no re-alignment when column count matches", func(t *testing.T) {
		original := map[int]int{0: 99, 1: 99, 2: 99}
		cfg := &config{view: view.View{Aligned: true, ColsWidth: original}}
		alignViewToResult(cfg, makeResult(3))
		assert.Equal(t, 99, cfg.view.ColsWidth[0], "ColsWidth must not change when counts match")
	})

	t.Run("re-aligns when new result has MORE columns than ColsWidth (was: panic)", func(t *testing.T) {
		// Simulate: view was aligned with 5 columns (e.g. statements_general),
		// then a batch with 13 columns arrives (e.g. statements_timings after rapid 'x').
		// Before fix: ColsWidth[5..12] = 0 → "slice bounds out of range [:-1]".
		cfg := &config{view: view.View{
			Aligned:   true,
			ColsWidth: map[int]int{0: 10, 1: 10, 2: 10, 3: 10, 4: 10},
		}}
		alignViewToResult(cfg, makeResult(13))
		assert.Equal(t, 13, len(cfg.view.ColsWidth))
		for i := 0; i < 13; i++ {
			assert.Greater(t, cfg.view.ColsWidth[i], 0, "ColsWidth[%d] must be > 0", i)
		}
	})

	t.Run("re-aligns when new result has FEWER columns than ColsWidth", func(t *testing.T) {
		cfg := &config{view: view.View{
			Aligned:   true,
			ColsWidth: map[int]int{0: 10, 1: 10, 2: 10, 3: 10, 4: 10, 5: 10, 6: 10},
		}}
		alignViewToResult(cfg, makeResult(4))
		assert.Equal(t, 4, len(cfg.view.ColsWidth))
	})
}
