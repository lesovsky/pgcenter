package top

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// helpEntryLine finds the single line of helpTemplate containing the given marker.
// It fails the test when the marker is absent or ambiguous, so the callers below can
// assert against one exact line instead of the whole template.
func helpEntryLine(t *testing.T, marker string) (int, string) {
	t.Helper()

	var idx = -1
	lines := strings.Split(helpTemplate, "\n")
	for i, line := range lines {
		if strings.Contains(line, marker) {
			assert.Equal(t, -1, idx, "marker %q must appear on exactly one line", marker)
			idx = i
		}
	}
	assert.NotEqual(t, -1, idx, "marker %q not found in helpTemplate", marker)
	if idx == -1 {
		return -1, ""
	}
	return idx, lines[idx]
}

// descColumn returns the zero-based column where the description of a help entry
// starts: leading indent, key token, padding. Alignment is checked by comparing this
// value between entries, never against a magic number.
func descColumn(line string) int {
	trimmed := strings.TrimLeft(line, " ")
	indent := len(line) - len(trimmed)

	key := strings.Index(trimmed, " ")
	if key == -1 {
		return indent + len(trimmed)
	}

	rest := trimmed[key:]
	pad := len(rest) - len(strings.TrimLeft(rest, " "))

	return indent + key + pad
}

// The built-in help is the only user-facing hotkey documentation in the project, so the
// Space entry, its placement and its alignment are pinned by a test rather than by review.
func Test_helpTemplate_pauseEntry(t *testing.T) {
	lines := strings.Split(helpTemplate, "\n")

	entryIdx, entry := helpEntryLine(t, "'Space' pause/resume display")
	contIdx, cont := helpEntryLine(t, "(sort,")
	scrollIdx, scroll := helpEntryLine(t, "'[' scroll columns left")

	assert.True(t, strings.HasPrefix(entry, "    Space"), "entry line is %q", entry)

	// The whole first line is pinned word for word, so a reworded clause fails here instead
	// of shipping. Only the description is compared, not the raw line: the padding in front
	// of it is the alignment check's business, below.
	assert.Equal(t, "'Space' pause/resume display; actions that need fresh data", entry[descColumn(entry):])

	// The entry sits in the 'general actions:' block right after the '[,]' row and before
	// the 'C,E,R config:' row.
	assert.Equal(t, scrollIdx+1, entryIdx)
	assert.Equal(t, entryIdx+1, contIdx)
	assert.True(t, strings.HasPrefix(lines[contIdx+1], "    C,E,R"), "next line is %q", lines[contIdx+1])

	// Descriptions of the whole block line up in one column; the continuation carries no
	// key of its own and starts at the same column.
	assert.Equal(t, descColumn(scroll), descColumn(entry))
	assert.Equal(t, descColumn(scroll), len(cont)-len(strings.TrimLeft(cont, " ")))

	// Both lines of the entry stay within 80 columns.
	assert.LessOrEqual(t, len(entry), 80)
	assert.LessOrEqual(t, len(cont), 80)
}

// An unexplained lift of the pause reads as a defect, so the help must name every action
// that lifts it. Assertions run against the continuation line only: single letters would
// match anywhere in the template and the test would stop checking anything.
func Test_helpTemplate_pauseLiftingActions(t *testing.T) {
	_, cont := helpEntryLine(t, "(sort,")

	testcases := []struct {
		name      string
		substring string
	}{
		{name: "sort order", substring: "sort"},
		{name: "screen switch", substring: "screen switch"},
		{name: "system tables", substring: "','"},
		{name: "idle connections", substring: " I,"},
		{name: "activity age", substring: " A,"},
		{name: "verbose mode", substring: " v,"},
		{name: "diskstat panel", substring: " B,"},
		{name: "nicstat panel", substring: " N,"},
		{name: "filesystems panel", substring: " F,"},
		{name: "logtail panel", substring: " L)"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Contains(t, cont, tc.substring)
		})
	}

	assert.True(t, strings.HasSuffix(cont, "resume it."), "continuation line is %q", cont)
}

// helpTemplate is a format string for fmt.Fprintf (see showHelp). An ordinary stray or
// mismatched verb is already caught harder and earlier by go vet's printf check, which
// go test runs as part of the build — this test does not add to that. What it does add is
// the case vet accepts: a doubled '%%' is valid escaping to vet, yet in a help screen it
// is a typo that renders as a literal percent sign. That is what the count assertions pin;
// the Sprintf round-trip below keeps the single-argument contract of showHelp visible.
func Test_helpTemplate_formatVerbs(t *testing.T) {
	assert.Equal(t, 1, strings.Count(helpTemplate, "%"))
	assert.Equal(t, 1, strings.Count(helpTemplate, "%s"))

	rendered := fmt.Sprintf(helpTemplate, "pgcenter")
	assert.Contains(t, rendered, "pgcenter")
	assert.NotContains(t, rendered, "%!")
	assert.NotContains(t, rendered, "(MISSING)")
	assert.NotContains(t, rendered, "(EXTRA")
}
