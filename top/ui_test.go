package top

import (
	"context"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
	"github.com/stretchr/testify/assert"
)

// tokenOf is a test helper building a token from its renderings, longest first.
func tokenOf(variants ...string) cmdlineToken {
	return cmdlineToken{variants: variants}
}

// Test_composeCmdline covers the truncation ladder of the single cmdline composition point.
// The line is built as <tokens left to right><single space><message>, clamped to width RUNES.
// Ladder order: (1) the message is cut; (2) the RIGHTMOST token steps through its variants;
// (3) tokens are dropped from the right; (4) last resort, the message is hard rune-truncated.
// A token is never rendered partially: it is either whole, degraded to a whole shorter variant,
// or absent.
func Test_composeCmdline(t *testing.T) {
	// The filter token's full ladder: 19, 13 and 5 runes.
	filter := tokenOf("[F:datname,usename]", "[F:datname,…]", "[F:…]")
	// A single-variant token - the shape [PAUSED] will have - must never shrink.
	pause := tokenOf("[PAUSED]")

	testcases := []struct {
		name   string
		tokens []cmdlineToken
		msg    string
		width  int
		want   string
	}{
		{
			// no prefix at all: the line is the message, no leading separator.
			name: "no tokens, message fits",
			msg:  "Refresh: ok", width: 80, want: "Refresh: ok",
		},
		{
			// no prefix, message longer than the line: hard truncate by runes.
			name: "no tokens, message truncated",
			msg:  "Refresh: ok", width: 4, want: "Refr",
		},
		{
			// prefix and message both fit: exactly one space between them.
			name: "one token and message fit", tokens: []cmdlineToken{tokenOf("[F:datname]")},
			msg: "Refresh: ok", width: 80, want: "[F:datname] Refresh: ok",
		},
		{
			// exactly the prefix fits: message dropped whole, no trailing separator.
			name: "only prefix fits exactly", tokens: []cmdlineToken{tokenOf("[F:datname]")},
			msg: "Refresh: ok", width: 11, want: "[F:datname]",
		},
		{
			// one spare column is not enough for separator + at least one message rune:
			// the separator must not dangle at the end of the line.
			name: "one spare column, no dangling separator", tokens: []cmdlineToken{tokenOf("[F:datname]")},
			msg: "Refresh: ok", width: 12, want: "[F:datname]",
		},
		{
			// two spare columns: separator plus a single message rune.
			name: "two spare columns", tokens: []cmdlineToken{tokenOf("[F:datname]")},
			msg: "Refresh: ok", width: 13, want: "[F:datname] R",
		},
		{
			// the message is cut before the prefix is touched: the prefix stays at its longest
			// variant even though degrading it would have let the whole message through.
			name: "message cut before prefix", tokens: []cmdlineToken{filter},
			msg: "Refresh: ok", width: 22, want: "[F:datname,usename] Re",
		},
		{
			// prefix does not fit: the rightmost token steps to its second variant.
			name: "rightmost token degrades one step", tokens: []cmdlineToken{filter},
			msg: "", width: 15, want: "[F:datname,…]",
		},
		{
			// still does not fit: the token steps to its shortest variant.
			name: "rightmost token degrades to shortest", tokens: []cmdlineToken{filter},
			msg: "", width: 6, want: "[F:…]",
		},
		{
			// variants exhausted: the token is dropped from the right, the message survives.
			name: "variants exhausted, token dropped", tokens: []cmdlineToken{filter},
			msg: "hello", width: 4, want: "hell",
		},
		{
			// two tokens: the rightmost degrades while the single-variant left one stays whole.
			name: "left token stays whole while right degrades", tokens: []cmdlineToken{pause, filter},
			msg: "", width: 21, want: "[PAUSED][F:datname,…]",
		},
		{
			// two tokens: the right one is dropped entirely, the left one still fits.
			name: "right token dropped, left kept", tokens: []cmdlineToken{pause, filter},
			msg: "", width: 12, want: "[PAUSED]",
		},
		{
			// a single-variant token is never rendered partially: it is dropped instead.
			name: "single variant token never shrinks", tokens: []cmdlineToken{pause},
			msg: "hello", width: 7, want: "hello",
		},
		{
			// ... and it is rendered whole as soon as it fits.
			name: "single variant token fits whole", tokens: []cmdlineToken{pause},
			msg: "", width: 8, want: "[PAUSED]",
		},
		{
			// degenerate view geometry must not panic and must not emit anything.
			name: "zero width", tokens: []cmdlineToken{filter}, msg: "Refresh: ok", width: 0, want: "",
		},
		{
			name: "negative width", tokens: []cmdlineToken{filter}, msg: "Refresh: ok", width: -5, want: "",
		},
		{
			// no tokens and no message: an empty line, not a stray separator.
			name: "nothing to render", msg: "", width: 80, want: "",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, composeCmdline(tc.tokens, tc.msg, tc.width))
		})
	}
}

// Test_composeCmdlineTwoTokens is the user-spec acceptance criterion: the composer accepts a
// second token - the [PAUSED] one - and assembles the line without the function itself being
// edited. The pause token stays a literal here even now that pauseToken exists: this test is about
// the composer, and building the token from feature code would make it test that instead.
func Test_composeCmdlineTwoTokens(t *testing.T) {
	pause := cmdlineToken{variants: []string{"[PAUSED]"}}

	filter, ok := filterToken(
		map[int]*regexp.Regexp{0: regexp.MustCompile("^pgcenter")},
		[]string{"datname", "usename"},
	)
	assert.True(t, ok)

	assert.Equal(t,
		"[PAUSED][F:datname] сообщение",
		composeCmdline([]cmdlineToken{pause, filter}, "сообщение", 80),
	)
}

// Test_composeCmdlineRunes pins Decision 12: every width computation counts runes, not bytes.
// Each case has a byte length above the width and a rune length within it, so a len()-based
// implementation renders something different.
func Test_composeCmdlineRunes(t *testing.T) {
	// 9 runes / 18 bytes, width 10: byte arithmetic would truncate it.
	assert.Equal(t, "сообщение", composeCmdline(nil, "сообщение", 10))

	// The ellipsis is 1 column and 3 bytes: "[F:…]" is 5 runes / 7 bytes, so with a 2-rune
	// message and a separator the line is exactly 8 columns wide.
	assert.Equal(t, "[F:…] ok", composeCmdline([]cmdlineToken{tokenOf("[F:…]")}, "ok", 8))

	// Truncation itself is by runes and never cuts a rune in half.
	got := composeCmdline(nil, "привет мир", 6)
	assert.Equal(t, "привет", got)
	assert.True(t, utf8.ValidString(got))
}

// Test_filterToken covers building the [F:...] token out of a view's filters and the column
// names known at that moment.
func Test_filterToken(t *testing.T) {
	re := regexp.MustCompile("^a")
	cols := []string{"datname", "usename", "state"}

	testcases := []struct {
		name     string
		filters  map[int]*regexp.Regexp
		cols     []string
		wantOK   bool
		variants []string
	}{
		{
			name: "no filters at all", filters: map[int]*regexp.Regexp{}, cols: cols, wantOK: false,
		},
		{
			name: "nil filters map", filters: nil, cols: cols, wantOK: false,
		},
		{
			// the predicate must match printHeaderCell's '*' marker, not the weaker nil-only one.
			name:    "nil regexp is not an active filter",
			filters: map[int]*regexp.Regexp{0: nil}, cols: cols, wantOK: false,
		},
		{
			// same: an empty pattern draws no '*' in the header, so it must draw no indicator.
			name:    "empty pattern is not an active filter",
			filters: map[int]*regexp.Regexp{0: regexp.MustCompile("")}, cols: cols, wantOK: false,
		},
		{
			// a single name has no honest middle variant: "[F:datname,…]" would claim a second filter.
			name:    "single filter",
			filters: map[int]*regexp.Regexp{0: re}, cols: cols, wantOK: true,
			variants: []string{"[F:datname]", "[F:…]"},
		},
		{
			// names ordered by ascending column index, which is also left-to-right screen order.
			name:    "several filters",
			filters: map[int]*regexp.Regexp{2: re, 0: re}, cols: cols, wantOK: true,
			variants: []string{"[F:datname,state]", "[F:datname,…]", "[F:…]"},
		},
		{
			// first frame: Cols is filled by the first successful render, not by view.New().
			name:    "nil cols",
			filters: map[int]*regexp.Regexp{0: re}, cols: nil, wantOK: false,
		},
		{
			// one frame after a screen switch the column count can lag the filters (issue #99).
			name:    "index out of range is skipped",
			filters: map[int]*regexp.Regexp{0: re, 7: re}, cols: cols, wantOK: true,
			variants: []string{"[F:datname]", "[F:…]"},
		},
		{
			name:    "negative index is skipped",
			filters: map[int]*regexp.Regexp{-1: re, 1: re}, cols: cols, wantOK: true,
			variants: []string{"[F:usename]", "[F:…]"},
		},
		{
			// an empty "[F:]" would claim a nameless filter - worse than no indicator at all.
			name:    "all indexes out of range",
			filters: map[int]*regexp.Regexp{7: re, 9: re}, cols: cols, wantOK: false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := filterToken(tc.filters, tc.cols)
			assert.Equal(t, tc.wantOK, ok)
			if !tc.wantOK {
				assert.Empty(t, got.variants)
				return
			}
			assert.Equal(t, tc.variants, got.variants)
		})
	}
}

// Test_filterTokenDeterministicOrder pins Decision 6. Go randomises map iteration, so an
// unsorted implementation is green on a single pass roughly as often as not - the loop is the
// substance of this check. Keys are inserted in a non-ascending order so insertion order differs
// from the expected one.
func Test_filterTokenDeterministicOrder(t *testing.T) {
	re := regexp.MustCompile("^a")
	cols := []string{"datname", "usename", "state", "waiting"}

	for i := 0; i < 50; i++ {
		filters := map[int]*regexp.Regexp{}
		filters[3] = re
		filters[1] = re
		filters[0] = re
		filters[2] = re

		got, ok := filterToken(filters, cols)
		assert.True(t, ok)
		assert.Equal(t, "[F:datname,usename,state,waiting]", got.variants[0])
		assert.Equal(t, "[F:datname,…]", got.variants[1])
	}
}

// Test_filterTokenStripsControlRunes pins Decision 13. Column names come from the server's row
// description, and the cmdline emits no SGR of its own to heal an unterminated sequence. Control
// runes are what gets removed - the printable tail "[31m" is ordinary text and stays.
func Test_filterTokenStripsControlRunes(t *testing.T) {
	re := regexp.MustCompile("^a")

	t.Run("escape sequence loses its control rune only", func(t *testing.T) {
		got, ok := filterToken(map[int]*regexp.Regexp{0: re}, []string{"dat\033[31mname"})
		assert.True(t, ok)
		assert.Equal(t, "[F:dat[31mname]", got.variants[0])

		for _, r := range got.variants[0] {
			assert.False(t, unicode.IsControl(r), "control rune %q left in the token", r)
		}
		// The ladder's arithmetic depends on the rune count matching the visible width.
		assert.Equal(t, len("[F:dat[31mname]"), utf8.RuneCountInString(got.variants[0]))
	})

	t.Run("other control runes are removed too", func(t *testing.T) {
		got, ok := filterToken(map[int]*regexp.Regexp{0: re}, []string{"dat\rna\x00me"})
		assert.True(t, ok)
		assert.Equal(t, "[F:datname]", got.variants[0])
	})

	t.Run("name of control runes only is dropped", func(t *testing.T) {
		got, ok := filterToken(map[int]*regexp.Regexp{0: re}, []string{"\r\n\x00"})
		assert.False(t, ok)
		assert.Empty(t, got.variants)
	})

	t.Run("blanked name does not blank its neighbours", func(t *testing.T) {
		got, ok := filterToken(
			map[int]*regexp.Regexp{0: re, 1: re},
			[]string{"\x00\r", "use\x01name"},
		)
		assert.True(t, ok)
		assert.Equal(t, "[F:usename]", got.variants[0])
		assert.False(t, strings.Contains(got.variants[0], ","))
	})
}

// Test_cmdlineTokens covers reading the state tokens off a config. It must be nil-safe: unit
// tests hand it a bare config and the ambient can be empty.
func Test_cmdlineTokens(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		assert.Empty(t, cmdlineTokens(nil))
	})

	t.Run("bare config", func(t *testing.T) {
		assert.Empty(t, cmdlineTokens(newConfig()))
	})

	t.Run("config with active filters", func(t *testing.T) {
		re := regexp.MustCompile("^a")
		c := newConfig()
		c.view = view.View{
			Cols:    []string{"datname", "usename"},
			Filters: map[int]*regexp.Regexp{1: re},
		}

		want, ok := filterToken(c.view.Filters, c.view.Cols)
		assert.True(t, ok)

		got := cmdlineTokens(c)
		assert.Equal(t, []cmdlineToken{want}, got)
	})

	t.Run("paused config", func(t *testing.T) {
		c := newConfig()
		c.paused.Store(true)

		// The marker is spelled out rather than taken from pauseToken: an expected value built by
		// the function under test would follow a wrong rendering instead of catching it.
		got := cmdlineTokens(c)
		assert.Equal(t, []cmdlineToken{{variants: []string{"[PAUSED]"}}}, got)
	})

	t.Run("paused config with active filters", func(t *testing.T) {
		re := regexp.MustCompile("^a")
		c := newConfig()
		c.paused.Store(true)
		c.view = view.View{
			Cols:    []string{"datname", "usename"},
			Filters: map[int]*regexp.Regexp{1: re},
		}

		// Order is the assertion: the marker is left of the filter indicator, so the composer -
		// which degrades and drops from the RIGHT - reaches the filter token first.
		got := cmdlineTokens(c)
		assert.Len(t, got, 2)
		assert.Equal(t, []string{"[PAUSED]"}, got[0].variants)
		assert.Equal(t, "[F:usename]", got[1].variants[0])
	})
}

// Test_cmdlineTokensPauseNeverDegrades walks the real tokens of a paused, filtered config down the
// composer's ladder. The filter token steps through its variants and is eventually dropped while
// [PAUSED] stays whole; below the marker's own width it disappears rather than being cut. Nothing
// in composeCmdline enforces this - it follows from the marker having a single variant and sitting
// leftmost, so this test is what keeps both properties from regressing.
func Test_cmdlineTokensPauseNeverDegrades(t *testing.T) {
	re := regexp.MustCompile("^a")
	c := newConfig()
	c.paused.Store(true)
	c.view = view.View{
		Cols:    []string{"datname", "usename"},
		Filters: map[int]*regexp.Regexp{0: re, 1: re},
	}

	tokens := cmdlineTokens(c)

	// 8 + 19 runes: everything fits.
	assert.Equal(t, "[PAUSED][F:datname,usename]", composeCmdline(tokens, "", 27))
	// One column short: the filter steps down, the marker does not.
	assert.Equal(t, "[PAUSED][F:datname,…]", composeCmdline(tokens, "", 26))
	// The filter's last variant.
	assert.Equal(t, "[PAUSED][F:…]", composeCmdline(tokens, "", 20))
	// Too narrow even for that: the filter is dropped, the marker survives whole.
	assert.Equal(t, "[PAUSED]", composeCmdline(tokens, "", 12))
	assert.Equal(t, "[PAUSED]", composeCmdline(tokens, "", 8))

	// One column below the marker's width: absent, never a partial "[PAUSE".
	assert.Equal(t, "", composeCmdline(tokens, "", 7))
}

// Test_cmdlineMarkerAfterUIRebuild pins Decision 8, which deliberately ships no code. After a
// pager/editor return the UI is rebuilt only through a non-nil app.uiError, and layout's
// cmdline-creation branch writes it with printCmdline(app.ui, "%s", app.uiError). The message on
// that path is empty, and the write still re-renders the token prefix - which is how the marker
// comes back without a restoration branch.
//
// The error is taken from layout itself rather than hand-built, so a change to what layout returns
// for a 0x0 terminal reaches this test. The write that follows cannot be: it needs a live Gui, and
// layout returns before creating the cmdline view here. So this test guards the two halves a unit
// test can reach - the message is empty, and an empty message still composes the marker - while the
// branch that performs the write is verified on the stand.
func Test_cmdlineMarkerAfterUIRebuild(t *testing.T) {
	prev := cmdlineCfg
	t.Cleanup(func() { cmdlineCfg = prev })

	c := newConfig()
	c.paused.Store(true)
	setCmdlineConfig(c)

	// A zero-size terminal is what layout sees right after a pager or editor closed the Gui, and
	// the error it returns there is what mainLoop stores in app.uiError. gocui.Gui.Size() only
	// reads two struct fields, so a bare Gui reaches that branch without a terminal.
	uiError := layout(&app{config: c, ui: &gocui.Gui{}})(nil)
	assert.Error(t, uiError)

	// "%s" of an error is its Error(): the message layout's write carries is empty.
	msg := uiError.Error()
	assert.Equal(t, "", msg)

	assert.Equal(t, "[PAUSED]", composeCmdline(cmdlineTokens(cmdlineCfg), msg, 80))
}

// Test_setCmdlineConfig checks the ambient is published by the named setter and read back by
// cmdlineTokens. The previous value is restored - leaving a test's config in a package-level
// pointer is exactly what Decision 1 forbids by keeping the setter out of newApp.
func Test_setCmdlineConfig(t *testing.T) {
	prev := cmdlineCfg
	t.Cleanup(func() { cmdlineCfg = prev })

	c := newConfig()
	c.view = view.View{
		Cols:    []string{"datname"},
		Filters: map[int]*regexp.Regexp{0: regexp.MustCompile("^a")},
	}

	setCmdlineConfig(c)
	assert.Same(t, c, cmdlineCfg)

	tokens := cmdlineTokens(cmdlineCfg)
	assert.Len(t, tokens, 1)
	assert.Equal(t, "[F:datname]", tokens[0].variants[0])
}

// Test_printCmdlineNilGui covers the only part of the writers a unit test can reach: gocui.View
// cannot be constructed here. Both writers must return silently on a nil Gui without touching
// the ambient - a nil ambient is left in place on purpose to catch a deref before g.Update.
func Test_printCmdlineNilGui(t *testing.T) {
	prev := cmdlineCfg
	t.Cleanup(func() { cmdlineCfg = prev })
	cmdlineCfg = nil

	assert.NotPanics(t, func() { printCmdline(nil, "%s", "message") })
	assert.NotPanics(t, func() { printCmdlinePersist(nil, "%s", "prompt") })
	assert.NotPanics(t, func() { printCmdline(nil, "") })
}

// statLoopHarness drives statLoop on its own goroutine with stub render/repaint steps.
//
// The stubs are the reason the loop is testable at all: statLoop takes function values instead of
// *app, so app.ui.Update - which panics on a nil *gocui.Gui (gocui/gui.go:312) - is never reached.
//
// The call counters are written ONLY by the loop goroutine and must be read after it has returned
// (wait()), which is what keeps the tests themselves -race clean.
type statLoopHarness struct {
	statCh    chan stat.Stat
	uiExit    chan int
	paused    atomic.Bool
	cancel    context.CancelFunc
	exit      chan statLoopExit
	rendered  chan struct{}
	repainted chan struct{}

	renderCalls  int
	repaintCalls int
}

// newStatLoopHarness starts statLoop against an UNBUFFERED statCh. The lack of buffering is the
// whole argument of the drain test: on an unbuffered channel a completed send is proof that the
// loop performed a receive.
func newStatLoopHarness(paused bool) *statLoopHarness {
	h := &statLoopHarness{
		statCh:    make(chan stat.Stat),
		uiExit:    make(chan int),
		exit:      make(chan statLoopExit, 1),
		rendered:  make(chan struct{}, 1),
		repainted: make(chan struct{}, 1),
	}
	h.paused.Store(paused)

	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel

	go func() {
		h.exit <- statLoop(ctx, h.uiExit, h.statCh, &h.paused,
			func(stat.Stat) {
				h.renderCalls++
				signalStep(h.rendered)
			},
			func() {
				h.repaintCalls++
				signalStep(h.repainted)
			},
		)
	}()

	return h
}

// signalStep reports a step to the test WITHOUT ever blocking the loop goroutine. A blocking send would
// park the loop inside its own render/repaint step, which is precisely the state the drain test
// exists to prove impossible - the test would then deadlock on the thing it is measuring instead of
// measuring it. The counters, not this channel, carry the totals.
func signalStep(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// wait returns the loop's exit reason, failing the test rather than hanging the suite if the loop
// never returns.
func (h *statLoopHarness) wait(t *testing.T) statLoopExit {
	t.Helper()
	select {
	case reason := <-h.exit:
		return reason
	case <-time.After(5 * time.Second):
		t.Fatal("statLoop did not return")
		return exitCtx
	}
}

// Test_statLoop_drainsWhilePaused is the feature's central concurrency assertion: while paused the
// gate keeps RECEIVING. collectStat reaches its viewCh receive only after a successful statCh send
// (top/stat.go:72-84) and ~17 key handlers push on the unbuffered viewCh, so a gate that stopped
// receiving would park the collector and the first keypress would block MainLoop forever - with
// Space itself unable to recover it.
//
// The producer is shaped like the real collector: it records a completion only AFTER the send
// returns. On an unbuffered channel that recorded completion IS the proof that a receive happened.
// The test fails by timeout instead of hanging the suite if the gate ever stops draining.
func Test_statLoop_drainsWhilePaused(t *testing.T) {
	const frames = 5 // the user-spec's "no fewer than five consecutive sends" criterion

	h := newStatLoopHarness(true)

	sent := make(chan int, 1)
	go func() {
		n := 0
		for i := 0; i < frames; i++ {
			h.statCh <- stat.Stat{}
			n++
		}
		sent <- n
	}()

	select {
	case n := <-sent:
		assert.Equal(t, frames, n)
	case <-time.After(5 * time.Second):
		t.Fatal("collector blocked: the paused gate stopped receiving")
	}

	// The loop is sequential: the fifth send completed, so the fifth receive happened, so the
	// fifth repaint has run (or is running) before the loop reaches its next select. Cancelling
	// here therefore cannot race the count below.
	h.cancel()
	assert.Equal(t, exitCtx, h.wait(t))

	assert.Equal(t, 0, h.renderCalls, "a discarded frame must never be rendered")
	assert.Equal(t, frames, h.repaintCalls, "every discarded frame repaints the store")
}

// Test_statLoop_rendersAfterResume covers the other half of the gate: with the flag cleared the
// next frame goes to the live render path and does not repaint the store.
//
// The handshake on h.repainted before flipping the flag is load-bearing: a completed send only
// means the loop received the frame, not that it has read the flag yet, so flipping immediately
// after the send would make the first frame's fate a race.
func Test_statLoop_rendersAfterResume(t *testing.T) {
	h := newStatLoopHarness(true)

	h.statCh <- stat.Stat{}
	<-h.repainted

	h.paused.Store(false)

	h.statCh <- stat.Stat{}
	<-h.rendered

	h.cancel()
	assert.Equal(t, exitCtx, h.wait(t))

	assert.Equal(t, 1, h.renderCalls)
	assert.Equal(t, 1, h.repaintCalls)
}

// Test_statLoop_exitOnUIExit pins the exit reason of the pager/editor path. doWork keys wg.Wait()
// off this value - it must NOT wait here, because ctx is not cancelled on that path and the
// collector may still be parked on its send - so the reason itself is the assertion.
func Test_statLoop_exitOnUIExit(t *testing.T) {
	h := newStatLoopHarness(false)

	// A producer parked on the send, exactly like collectStat when the pager is opened. It is
	// JOINED rather than abandoned: whichever branch of its select wins - the loop happening to
	// take the frame before uiExit, or the release below - the goroutine returns, so the join
	// cannot hang and the test leaves nothing running. Release first, then wait, in one deferred
	// step; this changes nothing the test asserts, which is only the exit reason.
	stop := make(chan struct{})
	producerDone := make(chan struct{})
	defer func() {
		close(stop)
		<-producerDone
	}()
	go func() {
		defer close(producerDone)
		select {
		case h.statCh <- stat.Stat{}:
		case <-stop:
		}
	}()

	h.uiExit <- 1
	assert.Equal(t, exitUI, h.wait(t))
}

// Test_statLoop_exitOnContextCancel covers the shutdown/UI-rebuild path, whose reason is what makes
// doWork wait for the collector goroutine.
func Test_statLoop_exitOnContextCancel(t *testing.T) {
	h := newStatLoopHarness(false)

	h.cancel()
	assert.Equal(t, exitCtx, h.wait(t))

	assert.Equal(t, 0, h.renderCalls)
	assert.Equal(t, 0, h.repaintCalls)
}

// observation is one layout pass as the resize detector sees it: the pause flag and the size that
// pass read from the terminal, plus whether that pass must ask for a repaint.
type observation struct {
	paused bool
	x, y   int
	want   bool
}

// Test_resizeDetector_observe drives the detector as a SEQUENCE, because what it pins is a state
// machine, not a single comparison. Three properties, and every row below belongs to one of them:
//
//   - a size change while paused asks for exactly one repaint - the frozen frame's visible-column
//     window was computed for the old width, so it has to be redrawn once for the new one;
//   - the repaint chain TERMINATES: the size is recorded before the answer is given, so the layout
//     pass that follows the repaint sees no change and asks for nothing;
//   - the size is recorded even while live, so resuming a pause after a resize does not manufacture
//     a repaint out of a size that was already on screen.
//
// The detector is what makes any of this testable: layout itself calls app.ui.Size(), and a
// *gocui.Gui with a non-zero size cannot be built outside the gocui package. The wiring inside
// layout is covered by the stand run of task 10.
func Test_resizeDetector_observe(t *testing.T) {
	testcases := []struct {
		name string
		seq  []observation
	}{
		{
			// The zero value of the detector is the state of a freshly built Gui: mainLoop
			// recreates layout's closure for every Gui, so the first pass after a return from the
			// pager, the editor or psql counts as a change and puts the frozen frame back.
			name: "zero start state repaints, the pass after it does not",
			seq: []observation{
				{paused: true, x: 190, y: 52, want: true},
				{paused: true, x: 190, y: 52, want: false},
			},
		},
		{
			name: "width only",
			seq: []observation{
				{paused: true, x: 190, y: 52, want: true},
				{paused: true, x: 60, y: 52, want: true},
				{paused: true, x: 60, y: 52, want: false},
			},
		},
		{
			// A height-only resize re-lays out the panel bands, so it is a change like any other.
			name: "height only",
			seq: []observation{
				{paused: true, x: 190, y: 52, want: true},
				{paused: true, x: 190, y: 24, want: true},
				{paused: true, x: 190, y: 24, want: false},
			},
		},
		{
			name: "both dimensions, growth then shrink",
			seq: []observation{
				{paused: true, x: 60, y: 24, want: true},
				{paused: true, x: 190, y: 52, want: true},
				{paused: true, x: 60, y: 24, want: true},
				{paused: true, x: 60, y: 24, want: false},
			},
		},
		{
			// Live mode redraws every tick on its own, and a repaint there would race the natural
			// frame.
			name: "live mode never asks for a repaint",
			seq: []observation{
				{paused: false, x: 190, y: 52, want: false},
				{paused: false, x: 60, y: 52, want: false},
				{paused: false, x: 60, y: 52, want: false},
			},
		},
		{
			// The resize happened while live, so its size is already on screen: pressing Space
			// afterwards freezes exactly what the operator is looking at and needs no repaint.
			name: "a resize while live is recorded, so a later pause does not repaint",
			seq: []observation{
				{paused: false, x: 190, y: 52, want: false},
				{paused: false, x: 60, y: 52, want: false},
				{paused: true, x: 60, y: 52, want: false},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var d resizeDetector

			for i, o := range tc.seq {
				assert.Equalf(t, o.want, d.observe(o.paused, o.x, o.y),
					"step %d: paused=%v size=%dx%d", i, o.paused, o.x, o.y)
			}
		})
	}
}
