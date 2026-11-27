package top

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/lesovsky/pgcenter/internal/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allDialogTypes is every dialog that has a prompt - the input domain of dialogPrompts.
var allDialogTypes = []dialogType{
	dialogPgReload,
	dialogFilter,
	dialogCancelQuery,
	dialogTerminateBackend,
	dialogCancelGroup,
	dialogTerminateGroup,
	dialogSetMask,
	dialogChangeAge,
	dialogQueryReport,
	dialogChangeRefresh,
}

// activeFilterTokens builds the cmdline prefix a user with an active filter on 'datname' sees.
// Built through filterToken, not by hand, so the test tracks the real token rendering.
func activeFilterTokens(t *testing.T) []cmdlineToken {
	t.Helper()

	tok, ok := filterToken(
		map[int]*regexp.Regexp{0: regexp.MustCompile("^pgcenter")},
		[]string{"datname"},
	)
	require.True(t, ok, "filter token must be built")

	return []cmdlineToken{tok}
}

// Test_dialogInputX0 covers the pure geometry of the dialog input field's left edge. gocui's
// usable width is x1-x0-1 and x1 is maxX-1, so the base value reproduces today's "start right
// after the printed text" while the upper clamp keeps minWidth usable columns and the lower one
// keeps x0 on (or just off) screen.
func Test_dialogInputX0(t *testing.T) {
	testcases := []struct {
		name       string
		cmdlineLen int
		maxX       int
		minWidth   int
		want       int
	}{
		{
			// wide terminal, ordinary prompt: the field starts right behind the text, exactly
			// where the pre-existing len(prompt)-1 put it.
			name: "starts right after the text", cmdlineLen: 12, maxX: 200, minWidth: minDialogInputWidth, want: 11,
		},
		{
			// line longer than the terminal: clamped so that exactly minWidth usable columns
			// remain (80-1 - 68 - 1 == 10).
			name: "clamped to keep minWidth columns", cmdlineLen: 300, maxX: 80, minWidth: minDialogInputWidth, want: 68,
		},
		{
			// terminal narrower than minWidth: the upper clamp goes below -1, so the lower bound
			// takes over and the field gets whatever columns exist.
			name: "lower bound on a tiny terminal", cmdlineLen: 50, maxX: 5, minWidth: minDialogInputWidth, want: -1,
		},
		{
			// nothing printed on the cmdline: the field starts at the leftmost usable column.
			name: "empty cmdline", cmdlineLen: 0, maxX: 80, minWidth: minDialogInputWidth, want: -1,
		},
		{
			// boundary: the longest line that still needs no clamping on an 80-column terminal.
			name: "longest unclamped line", cmdlineLen: 69, maxX: 80, minWidth: minDialogInputWidth, want: 68,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got := dialogInputX0(tc.cmdlineLen, tc.maxX, tc.minWidth)
			assert.Equal(t, tc.want, got)
		})
	}
}

// Test_dialogInputX0AlwaysLeftOfX1 walks the whole declared input domain (maxX >= 1, which is what
// dialogOpen's degenerate-geometry guard admits) and asserts the invariant whose violation is not
// a cosmetic glitch: SetView rejects x0 >= x1 with "invalid dimensions", the error leaves the key
// handler, reaches MainLoop and makes mainLoop tear down and rebuild the entire UI.
func Test_dialogInputX0AlwaysLeftOfX1(t *testing.T) {
	for maxX := 1; maxX <= 300; maxX++ {
		x1 := maxX - 1
		for cmdlineLen := 0; cmdlineLen <= 300; cmdlineLen++ {
			x0 := dialogInputX0(cmdlineLen, maxX, minDialogInputWidth)
			if x0 >= x1 {
				t.Fatalf("x0 >= x1: cmdlineLen=%d maxX=%d -> x0=%d, x1=%d", cmdlineLen, maxX, x0, x1)
			}
		}
	}
}

// Test_dialogInputX0DegenerateGeometry documents why dialogOpen needs an early return and not just
// the clamp: at maxX <= 0 the right edge x1 = maxX-1 is itself invalid, so no x0 can satisfy
// x0 < x1. maxX >= 1 is therefore the lower end of the function's domain.
func Test_dialogInputX0DegenerateGeometry(t *testing.T) {
	for _, maxX := range []int{0, -1} {
		x0 := dialogInputX0(0, maxX, minDialogInputWidth)
		assert.GreaterOrEqual(t, x0, maxX-1,
			"no x0 is valid at maxX=%d - dialogOpen must refuse to open the dialog", maxX)
	}
}

// Test_dialogPromptNeverOverlappedByInput is the alignment criterion, checked on every prompt the
// program has, with and without the persistent state prefix. The line is built with the same two
// calls dialogOpen makes, so the test catches any drift between "what we measured" and "what the
// writer will draw". Two things must hold at once: the field's first content column (x0+1) is not
// inside the printed text, and the field keeps at least minDialogInputWidth usable columns.
func Test_dialogPromptNeverOverlappedByInput(t *testing.T) {
	prefixes := map[string][]cmdlineToken{
		"no tokens":     nil,
		"filter active": activeFilterTokens(t),
	}

	for name, tokens := range prefixes {
		for _, d := range allDialogTypes {
			for _, maxX := range []int{40, 60, 80, 120, 200} {
				prompt := dialogPrompts(d)

				shown := dialogPromptFit(tokens, prompt, maxX, minDialogInputWidth)
				line := composeCmdline(tokens, shown, maxX)
				x0 := dialogInputX0(utf8.RuneCountInString(line), maxX, minDialogInputWidth)
				x1 := maxX - 1

				assert.GreaterOrEqualf(t, x0+1, utf8.RuneCountInString(line),
					"%s, dialog %d, maxX %d: input field starts inside the prompt (line %q)", name, d, maxX, line)
				assert.GreaterOrEqualf(t, x1-x0-1, minDialogInputWidth,
					"%s, dialog %d, maxX %d: input field lost its usable columns", name, d, maxX)

				// Apart from the truncation marker, the shown prompt is a prefix of the
				// original one - the ladder never rewrites the text, it only cuts it.
				assert.Truef(t, strings.HasPrefix(prompt, strings.TrimSuffix(shown, "…")),
					"%s, dialog %d, maxX %d: shown prompt %q is not a prefix of %q", name, d, maxX, shown, prompt)
			}
		}
	}
}

// Test_dialogSetMaskOpensAt80Columns is the named regression for the live bug: the 93-character
// state-mask prompt on an 80-column terminal put x0 (92) to the right of x1 (79), and the
// resulting SetView error rebuilt the whole UI. With the persistent filter indicator in front of
// the prompt the old arithmetic was even further off.
func Test_dialogSetMaskOpensAt80Columns(t *testing.T) {
	const maxX = 80

	tokens := activeFilterTokens(t)
	prompt := dialogPrompts(dialogSetMask)
	require.Equal(t, 93, utf8.RuneCountInString(prompt), "prompt length assumed by this regression test")

	shown := dialogPromptFit(tokens, prompt, maxX, minDialogInputWidth)
	line := composeCmdline(tokens, shown, maxX)
	x0 := dialogInputX0(utf8.RuneCountInString(line), maxX, minDialogInputWidth)
	x1 := maxX - 1

	// Valid coordinates: this alone is the difference between an open dialog and a UI teardown.
	assert.Less(t, x0, x1)

	// A field the user can actually type into.
	assert.GreaterOrEqual(t, x1-x0-1, minDialogInputWidth)

	// The prompt was truncated, not overlaid: it is still readable and the indicator survived.
	assert.True(t, strings.HasPrefix(line, "[F:datname] "))
	assert.NotEmpty(t, shown)
	assert.LessOrEqual(t, utf8.RuneCountInString(line), maxX-minDialogInputWidth-1)
}

// Test_dialogInputX0CountsRunes pins Decision 12. Lengths are rune counts: the truncation ladder
// emits '…' and the header emits '‹'/'›', all multi-byte. The two answers must differ
// numerically, otherwise the test would pass over a byte-based implementation.
func Test_dialogInputX0CountsRunes(t *testing.T) {
	t.Run("rune count differs from byte count", func(t *testing.T) {
		// 10 runes / 14 bytes: '…' costs 3 bytes and '‹' costs 3.
		line := "[F:…] ‹abc"
		require.Equal(t, 10, utf8.RuneCountInString(line))
		require.Equal(t, 14, len(line))

		assert.Equal(t, 9, dialogInputX0(utf8.RuneCountInString(line), 80, minDialogInputWidth))
		assert.Equal(t, 13, dialogInputX0(len(line), 80, minDialogInputWidth))
	})

	t.Run("whole pipeline counts runes", func(t *testing.T) {
		tok, ok := filterToken(
			map[int]*regexp.Regexp{0: regexp.MustCompile("^a")},
			[]string{"имя"},
		)
		require.True(t, ok)
		tokens := []cmdlineToken{tok}

		const maxX = 80
		shown := dialogPromptFit(tokens, dialogPrompts(dialogFilter), maxX, minDialogInputWidth)
		line := composeCmdline(tokens, shown, maxX)
		require.Less(t, utf8.RuneCountInString(line), len(line), "line must contain multi-byte runes")

		assert.Equal(t, utf8.RuneCountInString(line)-1,
			dialogInputX0(utf8.RuneCountInString(line), maxX, minDialogInputWidth))
	})
}

// Test_dialogYCoordsFromLayout pins Decision 11: the dialog takes its y coordinates from the same
// layout function that positions the cmdline view. In compact mode topBandLayout returns exactly
// the literals dialogOpen used to hard-wire, so that path did not change; in verbose it returns
// coordinates further down, which is where the input field must follow the prompt. This test goes
// red if somebody restores the literals or moves the band without thinking about the dialog.
func Test_dialogYCoordsFromLayout(t *testing.T) {
	_, _, compactY0, compactY1, _, _ := topBandLayout(false, 50)
	assert.Equal(t, 3, compactY0, "compact dialog geometry must stay byte-identical to the old literals")
	assert.Equal(t, 5, compactY1, "compact dialog geometry must stay byte-identical to the old literals")

	_, _, verboseY0, verboseY1, _, expanded := topBandLayout(true, 50)
	require.True(t, expanded, "a 50-row terminal must expand the verbose band")
	assert.Greater(t, verboseY0, compactY0, "verbose moves the cmdline - and the dialog - down")
	assert.Greater(t, verboseY1, compactY1, "verbose moves the cmdline - and the dialog - down")

	// Whatever the mode, the band always leaves the dialog a single content row and valid coords.
	assert.Equal(t, 1, compactY1-compactY0-1)
	assert.Equal(t, 1, verboseY1-verboseY0-1)
}

// Test_dialogPromptFit covers the width budget itself: the prompt is cut to what is left of
// maxX - minDialogInputWidth - 1 after the state prefix and its separating space, so the composed
// line never eats into the room reserved for the input field.
func Test_dialogPromptFit(t *testing.T) {
	tokens := activeFilterTokens(t)

	t.Run("no truncation when it fits", func(t *testing.T) {
		prompt := dialogPrompts(dialogFilter)
		assert.Equal(t, prompt, dialogPromptFit(tokens, prompt, 200, minDialogInputWidth))
	})

	t.Run("no separator is accounted for without tokens", func(t *testing.T) {
		// Budget is 80-10-1 = 69 and there is no prefix, so a 69-rune prompt survives whole.
		prompt := strings.Repeat("x", 69)
		assert.Equal(t, prompt, dialogPromptFit(nil, prompt, 80, minDialogInputWidth))
		assert.Len(t, []rune(dialogPromptFit(nil, prompt+"y", 80, minDialogInputWidth)), 69)
	})

	t.Run("prefix and separator are subtracted", func(t *testing.T) {
		// "[F:datname]" is 11 runes plus one separating space: 69-12 = 57 runes left.
		prompt := strings.Repeat("x", 100)
		assert.Len(t, []rune(dialogPromptFit(tokens, prompt, 80, minDialogInputWidth)), 57)
	})

	t.Run("no room at all", func(t *testing.T) {
		// A terminal narrower than the reservation leaves nothing for the prompt; the empty
		// string keeps the field usable instead of pushing it off the screen.
		assert.Empty(t, dialogPromptFit(tokens, dialogPrompts(dialogFilter), 8, minDialogInputWidth))
	})
}

// Test_dialogPromptFitMarksTruncation covers the visual half of the criterion: a prompt that did
// not fit is cut WITH an ellipsis, so the user can tell the text is incomplete - the indicator's
// own ladder already marks its cuts the same way. The marker is spent from the same budget the
// prompt is cut into, never added on top of it, so it cannot push the input field off the screen.
func Test_dialogPromptFitMarksTruncation(t *testing.T) {
	tokens := activeFilterTokens(t)

	t.Run("truncated prompt is marked", func(t *testing.T) {
		// "[F:datname]" is 11 runes plus one separating space: 69-12 = 57 runes left, of which
		// the last one goes to the marker.
		prompt := strings.Repeat("x", 100)
		assert.Equal(t, strings.Repeat("x", 56)+"…", dialogPromptFit(tokens, prompt, 80, minDialogInputWidth))
	})

	t.Run("untruncated prompt is left alone", func(t *testing.T) {
		// Budget is 80-10-1 = 69 and there is no prefix: a 69-rune prompt is complete, so there
		// is nothing to mark; one rune more and the marker appears.
		prompt := strings.Repeat("x", 69)
		assert.Equal(t, prompt, dialogPromptFit(nil, prompt, 80, minDialogInputWidth))
		assert.Equal(t, strings.Repeat("x", 68)+"…", dialogPromptFit(nil, prompt+"y", 80, minDialogInputWidth))
	})

	t.Run("the live state mask prompt is marked", func(t *testing.T) {
		shown := dialogPromptFit(tokens, dialogPrompts(dialogSetMask), 80, minDialogInputWidth)
		assert.True(t, strings.HasSuffix(shown, "…"), "truncated prompt %q carries no ellipsis", shown)
	})

	t.Run("marker never widens the line", func(t *testing.T) {
		// The property that must survive the marker: whatever is shown still fits the reserved
		// budget, on every prompt and every width - the ellipsis is spent from it, not added.
		//
		// The sweep starts at 24: below that the budget does not even cover the state prefix,
		// which dialogPromptFit cannot shrink, so no prompt - empty or marked - can satisfy the
		// bound. That regime is the "no room at all" case, covered by its own subtest.
		for name, tk := range map[string][]cmdlineToken{"no tokens": nil, "filter active": tokens} {
			for _, d := range allDialogTypes {
				for maxX := 24; maxX <= 200; maxX++ {
					prompt := dialogPrompts(d)
					shown := dialogPromptFit(tk, prompt, maxX, minDialogInputWidth)
					line := composeCmdline(tk, shown, maxX)

					assert.LessOrEqualf(t, utf8.RuneCountInString(line), maxX-minDialogInputWidth-1,
						"%s, dialog %d, maxX %d: line %q outgrew the budget", name, d, maxX, line)

					// Marked or not, the visible text is still the head of the original prompt.
					assert.Truef(t, strings.HasPrefix(prompt, strings.TrimSuffix(shown, "…")),
						"%s, dialog %d, maxX %d: shown prompt %q rewrites %q", name, d, maxX, shown, prompt)
				}
			}
		}
	})

	t.Run("degenerate budgets stay sane", func(t *testing.T) {
		// maxX 24 leaves exactly one column for the prompt: too little for text plus marker, and
		// a lone ellipsis would waste that column without telling the user anything. maxX 25
		// leaves two - the smallest budget that carries both.
		assert.Empty(t, dialogPromptFit(tokens, dialogPrompts(dialogFilter), 24, minDialogInputWidth))
		assert.Equal(t, "S…", dialogPromptFit(tokens, dialogPrompts(dialogFilter), 25, minDialogInputWidth))
		assert.Empty(t, dialogPromptFit(tokens, dialogPrompts(dialogFilter), 8, minDialogInputWidth))
	})
}

// newFilterConfig builds the config the filter dialog path needs. newConfig() fills only views and
// viewCh, so config.view stays the zero view.View whose Filters map is NIL - and setFilter's
// success branch writes into that map, which panics on a nil one before any assertion can run. Each
// caller gets a FRESH view for the same reason a shared fixture would be wrong here: the
// valid-pattern case mutates Filters, so one table row would otherwise decide another row's
// outcome. Idiom of Test_cmdlineTokens (top/ui_test.go).
func newFilterConfig() *config {
	c := newConfig()
	c.view = view.View{
		Cols:     []string{"datname", "usename"},
		Filters:  map[int]*regexp.Regexp{},
		OrderKey: 1,
	}

	return c
}

// pausedCmdlineTokens builds the cmdline prefix of a paused display through cmdlineTokens, so the
// tests exercise the real token ordering and survive a rename of the marker's literal. With
// filtered set, the view also carries an active filter on 'datname' - the second half of the
// prefix the narrow-terminal criterion is about.
func pausedCmdlineTokens(t *testing.T, filtered bool) []cmdlineToken {
	t.Helper()

	c := newConfig()
	c.paused.Store(true)
	if filtered {
		c.view = view.View{
			Cols:    []string{"datname"},
			Filters: map[int]*regexp.Regexp{0: regexp.MustCompile("^pgcenter")},
		}
	}

	tokens := cmdlineTokens(c)
	require.NotEmpty(t, tokens, "a paused config must produce at least the pause marker")

	return tokens
}

// renderedPauseMarker returns the marker as the cmdline writer renders it, on a terminal wide
// enough that no degradation applies. Tests compare against THIS instead of a hardcoded
// "[PAUSED]", so they keep testing the geometry rather than the literal.
func renderedPauseMarker(t *testing.T) string {
	t.Helper()

	marker := composeCmdline(pausedCmdlineTokens(t, false), "", 200)
	require.NotEmpty(t, marker)

	return marker
}

// Test_applyFilter_repaintsWhenPaused is the core of Decision 4. Applying a filter edits
// view.Filters and returns a message; it pushes nothing on viewCh, so under pause - where the
// collector's frames are discarded - nothing would redraw the table with the new predicate. The
// dialog branch therefore asks for a repaint itself, and ONLY while paused: in live mode the next
// collector tick renders the filter anyway and an extra repaint would race it.
func Test_applyFilter_repaintsWhenPaused(t *testing.T) {
	testcases := []struct {
		name         string
		paused       bool
		wantRepaints int
	}{
		{name: "paused repaints once", paused: true, wantRepaints: 1},
		{name: "live repaints never", paused: false, wantRepaints: 0},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			// Own view per row: the valid pattern below mutates Filters.
			c := newFilterConfig()
			c.paused.Store(tc.paused)

			var repaints int
			message := applyFilter("^pgcenter", c, func() { repaints++ })

			assert.Equal(t, tc.wantRepaints, repaints)

			// Whatever the pause state, the outcome is exactly setFilter's: same message, same map.
			assert.Equal(t, "Filters: ok", message)
			require.Contains(t, c.view.Filters, c.view.OrderKey)
			assert.Equal(t, "^pgcenter", c.view.Filters[c.view.OrderKey].String())
			assert.Len(t, c.view.Filters, 1)
		})
	}
}

// Test_applyFilter_repaintsOnUnchangedFilter pins the deliberate edge cases: an answer that changes
// no filter at all still repaints while paused. Both are cheap - the repaint redraws the stored
// frame without a collector round-trip - and skipping them would need the helper to interpret
// setFilter's message, which is a far worse coupling than one redundant redraw.
func Test_applyFilter_repaintsOnUnchangedFilter(t *testing.T) {
	t.Run("invalid regexp", func(t *testing.T) {
		c := newFilterConfig()
		c.paused.Store(true)

		var repaints int
		message := applyFilter("(", c, func() { repaints++ })

		assert.Equal(t, 1, repaints)
		assert.Equal(t, "Filters: error parsing regexp: missing closing ): `(`", message)
		assert.Empty(t, c.view.Filters, "an invalid pattern must not land in the map")
	})

	t.Run("empty answer on an unfiltered column", func(t *testing.T) {
		// The map is built here too, so this test drives the same object as its sibling: reading
		// and deleting are safe on a nil map, and a broken fixture would pass here while the
		// valid-pattern case panics.
		c := newFilterConfig()
		c.paused.Store(true)

		var repaints int
		message := applyFilter("", c, func() { repaints++ })

		assert.Equal(t, 1, repaints)
		assert.Equal(t, "Filters: no filter on this column", message)
		assert.Empty(t, c.view.Filters)
	})
}

// Test_dialogPromptFitWithPauseMarker re-checks the [015] width budget with the pause marker in the
// reserved prefix. The marker is token-agnostic to the budget by construction, so this test
// verifies rather than re-tunes: if it goes red, the fix belongs in dialogPromptFit, not in a
// special case for the marker. This is the user-spec's "подсказка обрезается раньше обычного - это
// ожидаемое поведение" expressed as an invariant.
func Test_dialogPromptFitWithPauseMarker(t *testing.T) {
	prefixes := map[string][]cmdlineToken{
		"paused":                 pausedCmdlineTokens(t, false),
		"paused + active filter": pausedCmdlineTokens(t, true),
	}

	for name, tokens := range prefixes {
		// The lower sweep bound is derived, not copied: dialogPromptFit cannot shrink the prefix,
		// so below prefix + minDialogInputWidth + 1 columns no prompt - empty or marked - can
		// satisfy the budget. That regime is the "no room at all" case of the [015] tests, and
		// with the marker in front it simply starts at a wider terminal than the 24 they use.
		prefix := composeCmdline(tokens, "", 200)
		minWidth := utf8.RuneCountInString(prefix) + minDialogInputWidth + 1

		for _, d := range allDialogTypes {
			for maxX := minWidth; maxX <= 200; maxX++ {
				prompt := dialogPrompts(d)

				shown := dialogPromptFit(tokens, prompt, maxX, minDialogInputWidth)
				line := composeCmdline(tokens, shown, maxX)
				x0 := dialogInputX0(utf8.RuneCountInString(line), maxX, minDialogInputWidth)
				x1 := maxX - 1

				assert.LessOrEqualf(t, utf8.RuneCountInString(line), maxX-minDialogInputWidth-1,
					"%s, dialog %d, maxX %d: line %q outgrew the budget", name, d, maxX, line)
				assert.GreaterOrEqualf(t, x0+1, utf8.RuneCountInString(line),
					"%s, dialog %d, maxX %d: input field starts inside the prompt (line %q)", name, d, maxX, line)
				assert.GreaterOrEqualf(t, x1-x0-1, minDialogInputWidth,
					"%s, dialog %d, maxX %d: input field lost its usable columns", name, d, maxX)
				assert.Truef(t, strings.HasPrefix(prompt, strings.TrimSuffix(shown, "…")),
					"%s, dialog %d, maxX %d: shown prompt %q rewrites %q", name, d, maxX, shown, prompt)
			}
		}
	}
}

// Test_dialogPromptShorterUnderPause measures the cost the user-spec accepts: while the marker sits
// in the reserved prefix the prompt loses exactly the marker's width plus its separating space -
// "примерно на 8 символов". The state-mask prompt is used because at 93 runes it is truncated in
// BOTH states; a short prompt like "Set filter: " fits either way and would prove nothing.
func Test_dialogPromptShorterUnderPause(t *testing.T) {
	const maxX = 80

	prompt := dialogPrompts(dialogSetMask)
	require.Equal(t, 93, utf8.RuneCountInString(prompt), "prompt length assumed by this test")

	live := dialogPromptFit(cmdlineTokens(newConfig()), prompt, maxX, minDialogInputWidth)
	paused := dialogPromptFit(pausedCmdlineTokens(t, false), prompt, maxX, minDialogInputWidth)

	// Both must actually be cut, otherwise the comparison below is vacuous.
	require.True(t, strings.HasSuffix(live, "…"), "live prompt %q was not truncated", live)
	require.True(t, strings.HasSuffix(paused, "…"), "paused prompt %q was not truncated", paused)

	assert.Less(t, utf8.RuneCountInString(paused), utf8.RuneCountInString(live))

	// The difference is the rendered marker plus the single space composeCmdline puts after the
	// prefix - derived from the marker, never from the literal 9.
	want := utf8.RuneCountInString(renderedPauseMarker(t)) + 1
	assert.Equal(t, want, utf8.RuneCountInString(live)-utf8.RuneCountInString(paused))
}

// Test_dialogMarkerSurvivesNarrowDialog is the narrow-terminal acceptance criterion: at 60 columns,
// paused and with an active filter, the dialog still opens with valid coordinates, the input field
// keeps its usable columns and the marker is still the first thing on the line - the composer
// degrades tokens from the right, so the single-variant marker on the left is the last to go.
func Test_dialogMarkerSurvivesNarrowDialog(t *testing.T) {
	const maxX = 60

	tokens := pausedCmdlineTokens(t, true)
	shown := dialogPromptFit(tokens, dialogPrompts(dialogFilter), maxX, minDialogInputWidth)
	line := composeCmdline(tokens, shown, maxX)
	x0 := dialogInputX0(utf8.RuneCountInString(line), maxX, minDialogInputWidth)
	x1 := maxX - 1

	assert.True(t, strings.HasPrefix(line, renderedPauseMarker(t)),
		"line %q lost the pause marker on a narrow terminal", line)
	assert.Less(t, x0, x1)
	assert.GreaterOrEqual(t, x1-x0-1, minDialogInputWidth)
	assert.NotEmpty(t, shown, "a 60-column terminal still has room for the filter prompt")
}
