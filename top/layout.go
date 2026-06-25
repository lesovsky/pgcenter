package top

// Compact geometry of the top band — the literals layout() used before verbose mode.
// gocui coordinates are inclusive and -1 means one row off-screen; the band top stays at -1.
const (
	compactSysstatY1 = 4 // sysstat panel bottom (left, compact)
	compactPgstatY1  = 4 // pgstat panel bottom (right, compact)
	compactCmdlineY0 = 3 // cmdline top, overlapping the bottom of the panels (drawn after them)
	compactCmdlineY1 = 5 // cmdline bottom
	compactDbstatY0  = 4 // dbstat (main table) top
)

// Extra label:value rows verbose mode adds to each panel. Asymmetric: sysstat grows by 3,
// pgstat by 5 (it has more verbose rows). Must match the rows actually printed by the
// sysstat/pgstat renderers — see code-research §1-new.
const (
	sysstatVerboseExtra = 3
	pgstatVerboseExtra  = 5
)

// topBandLayout computes the per-view y-coordinates of the top band (sysstat, pgstat,
// cmdline, dbstat) for the compact or verbose mode. It is a pure integer function (no gocui,
// no app) so it is table-testable; layout() does only the SetView plumbing with the result.
//
// Compact (verbose=false) reproduces the historical literals exactly. Verbose grows each
// panel from the top by its extra-row count and pushes cmdline and dbstat down to clear the
// taller of the two panels — so dbstat loses rows at the top, not the bottom.
//
// Height-guard: verbose expands only if maxY can fit the expanded band plus cmdline (2 rows),
// the dbstat header (1 row) and at least 1 data row. Since dbstat's y1 is maxY-1, that needs
// dbstatY0 + 1 (header) + 1 (data row) <= maxY - 1. If it cannot, the compact coordinates are
// returned with expanded=false (graceful fallback — never inverted/negative/overlapping views).
func topBandLayout(verbose bool, maxY int) (sysstatY1, pgstatY1, cmdlineY0, cmdlineY1, dbstatY0 int, expanded bool) {
	if !verbose {
		return compactSysstatY1, compactPgstatY1, compactCmdlineY0, compactCmdlineY1, compactDbstatY0, false
	}

	// Verbose: grow each panel independently, then clear the taller one.
	sysstatY1 = compactSysstatY1 + sysstatVerboseExtra
	pgstatY1 = compactPgstatY1 + pgstatVerboseExtra

	tallest := sysstatY1
	if pgstatY1 > tallest {
		tallest = pgstatY1
	}

	bandTop := tallest - 1
	cmdlineY0 = bandTop
	cmdlineY1 = bandTop + 2
	dbstatY0 = tallest + 1

	// minDbstatRows is the rows dbstat needs below dbstatY0 to be usable: 1 header + >=1 data row.
	const minDbstatRows = 2

	// Height-guard: dbstat's y1 is maxY-1, so it fits only if dbstatY0 + minDbstatRows <= maxY-1.
	// This also degrades safely for non-positive maxY (the band never expands into broken coords).
	if dbstatY0+minDbstatRows > maxY-1 {
		return compactSysstatY1, compactPgstatY1, compactCmdlineY0, compactCmdlineY1, compactDbstatY0, false
	}

	return sysstatY1, pgstatY1, cmdlineY0, cmdlineY1, dbstatY0, true
}
