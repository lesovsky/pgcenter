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

// Test_visibleColumns covers the pure column-window function used by the horizontal
// scroll feature. The function freezes column 0 and computes a sliding window over the
// scrollable columns (1..ncols-1) that fits into termWidth, re-clamping the offset on
// every call. Width budget per column is colsWidth[i]+2 (the +2 gap added by printing).
func Test_visibleColumns(t *testing.T) {
	// uniformWidths builds a dense map[int]int with the same width for columns [0, ncols).
	uniformWidths := func(ncols, width int) map[int]int {
		m := make(map[int]int, ncols)
		for i := 0; i < ncols; i++ {
			m[i] = width
		}
		return m
	}

	t.Run("all columns fit", func(t *testing.T) {
		// 5 columns of width 10 => each costs 12; total 60 fits easily into 1000.
		first, last, clamped, hiddenLeft, hiddenRight := visibleColumns(5, uniformWidths(5, 10), 1000, 0)
		assert.Equal(t, 0, clamped)
		assert.False(t, hiddenLeft)
		assert.False(t, hiddenRight)
		assert.Equal(t, 1, first)
		assert.Equal(t, 4, last)
	})

	t.Run("narrow width, offset 0", func(t *testing.T) {
		// Each column costs 12. termWidth 40 => frozen(12) + scrollable: 28/12 = 2 columns (1,2).
		first, last, clamped, hiddenLeft, hiddenRight := visibleColumns(6, uniformWidths(6, 10), 40, 0)
		assert.Equal(t, 0, clamped)
		assert.False(t, hiddenLeft)
		assert.True(t, hiddenRight)
		assert.Equal(t, 1, first)
		assert.Equal(t, 2, last)
	})

	t.Run("mid offset", func(t *testing.T) {
		// 6 columns, cost 12 each, termWidth 40 => after frozen, room for 2 scrollable.
		// offset 2 => window starts at column 3, shows columns 3 and 4. Column 5 hidden right,
		// columns 1,2 hidden left. maxOffset = 3 (columns 4,5 fit at the end), so 2 is valid.
		first, last, clamped, hiddenLeft, hiddenRight := visibleColumns(6, uniformWidths(6, 10), 40, 2)
		assert.Equal(t, 2, clamped)
		assert.True(t, hiddenLeft)
		assert.True(t, hiddenRight)
		assert.Equal(t, 3, first)
		assert.Equal(t, 4, last)
	})

	t.Run("offset past end", func(t *testing.T) {
		// 6 columns, room for 2 scrollable. The last two scrollable columns are 4 and 5,
		// so maxOffset = 3 (window starts at column 4). offset 99 clamps to 3.
		first, last, clamped, hiddenLeft, hiddenRight := visibleColumns(6, uniformWidths(6, 10), 40, 99)
		assert.Equal(t, 3, clamped)
		assert.True(t, hiddenLeft)
		assert.False(t, hiddenRight)
		assert.Equal(t, 4, first)
		assert.Equal(t, 5, last)
	})

	t.Run("very narrow only frozen fits", func(t *testing.T) {
		// termWidth 12 fits exactly the frozen column (cost 12), no room for scrollable.
		// Window must be empty: first=1, last=0 (last < first), no panic, no negative width.
		first, last, clamped, hiddenLeft, hiddenRight := visibleColumns(6, uniformWidths(6, 10), 12, 0)
		assert.Equal(t, 0, clamped)
		assert.False(t, hiddenLeft)
		assert.True(t, hiddenRight)
		assert.Equal(t, 1, first)
		assert.Equal(t, 0, last)
	})

	t.Run("negative scroll budget (term narrower than frozen column)", func(t *testing.T) {
		// termWidth 5 is smaller than the frozen column cost (12) => scrollBudget < 0.
		// Must be graceful: empty window first=1, last=0, no panic.
		first, last, clamped, hiddenLeft, hiddenRight := visibleColumns(6, uniformWidths(6, 10), 5, 0)
		assert.Equal(t, 0, clamped)
		assert.False(t, hiddenLeft)
		assert.True(t, hiddenRight)
		assert.Equal(t, 1, first)
		assert.Equal(t, 0, last)
	})

	t.Run("single frozen column only", func(t *testing.T) {
		// ncols == 1: only the frozen column exists, no scrollable columns at all.
		first, last, clamped, hiddenLeft, hiddenRight := visibleColumns(1, uniformWidths(1, 10), 1000, 0)
		assert.Equal(t, 0, clamped)
		assert.False(t, hiddenLeft)
		assert.False(t, hiddenRight)
		assert.Less(t, last, first, "no scrollable columns => empty window")
	})

	t.Run("missing or zero ColsWidth key", func(t *testing.T) {
		// Sparse map: keys for some columns in [0, ncols) are absent (read as 0).
		// Math must stay bounded (each missing column costs the +2 gap), no panic.
		// Deterministic budget for termWidth 40: frozen col0 costs 12, scrollBudget=28.
		// Costs from col1: 2,12,2,12 (=28, all four fit), col5 (+2) overflows.
		// => window covers columns 1..4, col5 hidden right.
		widths := map[int]int{0: 10, 2: 10, 4: 10} // columns 1, 3, 5 missing
		first, last, clamped, hiddenLeft, hiddenRight := visibleColumns(6, widths, 40, 0)
		assert.Equal(t, 1, first)
		assert.Equal(t, 4, last)
		assert.Equal(t, 0, clamped)
		assert.False(t, hiddenLeft)
		assert.True(t, hiddenRight)
	})

	t.Run("negative offset clamps to zero", func(t *testing.T) {
		// offset -5 must clamp up to 0 (covers math.Max(offset, 0) lower bound).
		// 6 columns cost 12 each, termWidth 40 => 2 scrollable fit from the start.
		first, last, clamped, hiddenLeft, hiddenRight := visibleColumns(6, uniformWidths(6, 10), 40, -5)
		assert.Equal(t, 0, clamped)
		assert.Equal(t, 1, first)
		assert.Equal(t, 2, last)
		assert.False(t, hiddenLeft)
		assert.True(t, hiddenRight)
	})

	t.Run("last column visible at max offset", func(t *testing.T) {
		// Invariant: at the maximum offset the last column (ncols-1) is visible and
		// nothing is hidden to the right. Narrow term 40 with 6 uniform columns has
		// maxOffset 3; passing a large offset clamps to it.
		ncols := 6
		first, last, clamped, hiddenLeft, hiddenRight := visibleColumns(ncols, uniformWidths(ncols, 10), 40, 1<<30)
		assert.Equal(t, 3, clamped)
		assert.Equal(t, ncols-1, last, "last visible column must be the final column at max offset")
		assert.False(t, hiddenRight, "nothing hidden to the right at max offset")
		assert.True(t, hiddenLeft)
		assert.Equal(t, 4, first)
	})
}
