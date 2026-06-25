package top

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test_topBandLayout covers the pure geometry function that drives the verbose-aware
// top-band layout. It is gocui-free (mirrors the visibleColumns precedent): pure integer
// arithmetic over (verbose, maxY) returning the per-view y-coordinates plus an expanded flag.
//
// Compact (verbose=false OR height-guard tripped) must reproduce today's literals exactly:
// sysstatY1=4, pgstatY1=4, cmdlineY0=3, cmdlineY1=5, dbstatY0=4.
// Verbose grows sysstat by +3 (y1=7) and pgstat by +5 (y1=9); cmdline/dbstat clear the
// taller (pgstat) panel: bandTop=max(7,9)-1=8 => cmdlineY0=8, cmdlineY1=10, dbstatY0=10.
// Height-guard: verbose expands only when maxY can fit dbstatY0 + header(1) + >=1 data row,
// i.e. dbstatY0 + 2 <= maxY - 1 => maxY >= 13. Otherwise fall back to compact.
func Test_topBandLayout(t *testing.T) {
	testcases := []struct {
		name      string
		verbose   bool
		maxY      int
		sysstatY1 int
		pgstatY1  int
		cmdlineY0 int
		cmdlineY1 int
		dbstatY0  int
		expanded  bool
	}{
		{
			// compact: verbose off, tall terminal -> today's exact literals, not expanded.
			name: "compact", verbose: false, maxY: 50,
			sysstatY1: 4, pgstatY1: 4, cmdlineY0: 3, cmdlineY1: 5, dbstatY0: 4, expanded: false,
		},
		{
			// verbose: tall enough -> asymmetric heights, cmdline/dbstat cleared below pgstat.
			name: "verbose", verbose: true, maxY: 50,
			sysstatY1: 7, pgstatY1: 9, cmdlineY0: 8, cmdlineY1: 10, dbstatY0: 10, expanded: true,
		},
		{
			// height-guard: verbose requested but terminal too short -> graceful compact fallback.
			name: "height-guard", verbose: true, maxY: 12,
			sysstatY1: 4, pgstatY1: 4, cmdlineY0: 3, cmdlineY1: 5, dbstatY0: 4, expanded: false,
		},
		{
			// boundary: smallest maxY that still expands (dbstatY0=10 + 2 <= maxY-1 => maxY>=13).
			name: "boundary-expands", verbose: true, maxY: 13,
			sysstatY1: 7, pgstatY1: 9, cmdlineY0: 8, cmdlineY1: 10, dbstatY0: 10, expanded: true,
		},
		{
			// boundary: largest maxY that still falls back (maxY=12 is one short of the threshold).
			name: "boundary-fallback", verbose: true, maxY: 12,
			sysstatY1: 4, pgstatY1: 4, cmdlineY0: 3, cmdlineY1: 5, dbstatY0: 4, expanded: false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			sysstatY1, pgstatY1, cmdlineY0, cmdlineY1, dbstatY0, expanded := topBandLayout(tc.verbose, tc.maxY)
			assert.Equal(t, tc.sysstatY1, sysstatY1, "sysstatY1")
			assert.Equal(t, tc.pgstatY1, pgstatY1, "pgstatY1")
			assert.Equal(t, tc.cmdlineY0, cmdlineY0, "cmdlineY0")
			assert.Equal(t, tc.cmdlineY1, cmdlineY1, "cmdlineY1")
			assert.Equal(t, tc.dbstatY0, dbstatY0, "dbstatY0")
			assert.Equal(t, tc.expanded, expanded, "expanded")
		})
	}
}
