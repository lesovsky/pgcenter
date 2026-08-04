package top

import (
	"context"
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/stat"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"
)

// cmdlineCfg is the ambient runtime configuration the cmdline composer reads its state tokens
// from. It is written exactly once by setCmdlineConfig, before any goroutine exists, and it is
// dereferenced ONLY in the gocui MainLoop goroutine - inside g.Update closures, in layout() or in
// key handlers. See setCmdlineConfig for why it exists at all.
var cmdlineCfg *config

// uiGeneration counts the Gui instances mainLoop has created. It is written by mainLoop's
// goroutine and read by cmdline clear timers, which capture its value when they are armed: on the
// UI restart path mainLoop abandons the old Gui without closing it, so nothing drains that Gui's
// update channel any more and a timer firing against it would leak. This is the only
// synchronisation primitive in the package.
var uiGeneration atomic.Uint64

// setCmdlineConfig publishes config as the ambient source of the cmdline state prefix.
//
// Write discipline: called from RunMain only, once, before any goroutine exists. Deliberately NOT
// called from newApp - newApp also runs in unit tests, where it would leave this package-level
// pointer aimed at one test's config for every test that follows.
//
// Read discipline: the published value is dereferenced only in the gocui MainLoop goroutine, so
// the config fields the composer reads need no locking.
func setCmdlineConfig(config *config) {
	cmdlineCfg = config
}

// mainLoop starts application worker and UI loop.
func mainLoop(ctx context.Context, app *app) error {
	var e errorRate

	// Run in infinite loop - if UI crashes then reinitialize it.
	for {
		// Init UI
		g, err := gocui.NewGui(gocui.OutputNormal)
		if err != nil {
			return fmt.Errorf("create UI failed: %w", err)
		}

		// A new Gui starts a new UI generation. Clear timers armed against the previous one
		// compare their captured generation with this counter and give up.
		uiGeneration.Add(1)

		app.ui = g

		// Setup UI layout.
		app.ui.SetManagerFunc(layout(app))

		// Setup keybindings.
		if err := keybindings(app); err != nil {
			return err
		}

		var wg sync.WaitGroup
		ctx, cancel := context.WithCancel(ctx)

		// Run backend workers which collect and print stats.
		wg.Add(1)
		go func() {
			doWork(ctx, app)
			wg.Done()
		}()

		// Run UI management loop.
		err = app.ui.MainLoop()
		if err != nil {
			// quit received.
			if err == gocui.ErrQuit {
				cancel()
				return nil
			}

			// remember error, will show it to user in cmdLine
			app.uiError = err

			// check errors rate - quit if them too much (allow no more than 5 errors within 1 second)
			if err := e.check(1*time.Second, 5); err != nil {
				cancel()
				return fmt.Errorf("too many UI errors occurred: %w (%d errors within %.0f seconds)", err, e.errCnt, e.timeElapsed.Seconds())
			}
		}

		// If there are too few errors, just restart worker and UI.
		cancel()

		// Wait until doWork() finish.
		wg.Wait()
	}
}

func doWork(ctx context.Context, app *app) {
	var wg sync.WaitGroup
	statCh := make(chan stat.Stat)

	wg.Add(1)
	go func() {
		collectStat(ctx, app.db, statCh, app.config.viewCh)
		wg.Done()
	}()

	// Send default view and default refresh interval to stats collector goroutine.
	app.config.view.Refresh = defaultRefresh
	app.config.viewCh <- app.config.view

	// Reset refresh interval, it should not be saved as per-view setting.
	app.config.view.Refresh = 0

	// Wait for the collector ONLY on the ctx exit. On the uiExit (pager/editor) path ctx is not
	// cancelled, so collectStat may still be parked on its statCh send with nobody receiving -
	// waiting there would hang the pager path. mainLoop cancels and waits on every path that
	// needs it (top/ui.go:99-102), so the uiExit branch needs no wait of its own.
	if statLoop(ctx, app.uiExit, statCh, &app.config.paused,
		func(s stat.Stat) { printStat(app, s, app.postgresProps) },
		func() { repaintStored(app) },
	) == exitCtx {
		wg.Wait()
	}
}

// statLoopExit tells doWork WHY the loop returned. The distinction is load-bearing: it is what
// keeps wg.Wait() off the pager path - see doWork.
type statLoopExit int

const (
	exitUI  statLoopExit = iota // app.uiExit - the pager/editor path.
	exitCtx                     // ctx.Done() - shutdown or UI rebuild.
)

// statLoop is doWork's receive loop with the pause gate, lifted out of doWork so it can be driven
// by a unit test: statCh and uiExit are plain channels and the two render steps are function
// values, so no *gocui.Gui is reachable from here (printStat's app.ui.Update panics on a nil one).
//
// The parameter list is the feature's ONE structural guarantee and must not be widened. render and
// repaint are opaque function values and neither *app nor the frame store is passed in, so from
// inside the gate there is physically nothing to read the store from - the worker goroutine cannot
// touch a structure the gocui goroutine owns (Decision 1), because it has no way to name it.
//
// The receive is unconditional by design. collectStat reaches its viewCh receive only after a
// successful statCh send (top/stat.go:72-84) and ~17 key handlers push on the unbuffered viewCh, so
// a gate that stopped receiving while paused would park the collector, the first keypress would
// block MainLoop forever, and Space itself could not recover it. Drain-and-discard is the only
// non-hanging shape: the frame is received, dropped, and answered with a repaint of the store.
func statLoop(
	ctx context.Context,
	uiExit <-chan int,
	statCh <-chan stat.Stat,
	paused *atomic.Bool,
	render func(stat.Stat),
	repaint func(),
) statLoopExit {
	for {
		select {
		case <-uiExit:
			// used for exit from UI (not the program) in case when need to open $PAGER or $EDITOR programs.
			return exitUI
		case s := <-statCh:
			if paused.Load() {
				// The frame is DROPPED - never stored, never rendered. A frame carrying
				// Stat.Error is discarded like any other; it surfaces on the first frame
				// after the pause is lifted.
				repaint()
				continue
			}
			render(s)
		case <-ctx.Done():
			return exitCtx
		}
	}
}

// resizeDetector remembers the terminal size of the previous layout pass and answers, once per
// pass, whether the frozen frame has to be repainted for a new geometry. Three facts a reader
// cannot recover from the code:
//
//  1. This is the ONLY place in the program that ever learns the terminal was resized. gocui
//     delivers no resize event: handleEvent (gocui/gui.go:410-419) dispatches EventKey, EventMouse
//     and EventError, and termbox.EventResize falls into its default branch. The new size becomes
//     visible only through flush() (gui.go:422-432), which reads termbox.Size() and only then calls
//     the managers' Layout - so layout's app.ui.Size() is guaranteed to be the new size.
//  2. The state is per-Gui. It lives in layout's enclosing closure, which mainLoop recreates for
//     every Gui it builds (top/ui.go:62), so its zero value makes the first pass of a new Gui count
//     as a change. That is what puts the frozen frame back on screen after a return from the pager,
//     the editor or psql - a separate acceptance criterion, satisfied here at no extra cost.
//  3. observe records the new size BEFORE it answers true. That is the termination argument: a
//     repaint does not change the terminal size, so the pass that follows it sees the size already
//     recorded and asks for nothing. One resize, one repaint.
//
// It touches neither app nor gocui, which is what lets a table test drive the decision without a
// terminal - the same seam as topBandLayout (top/layout.go).
type resizeDetector struct {
	lastX, lastY int
}

// observe records the size this layout pass saw and reports whether the stored frame must be
// repainted for it.
//
// The size is recorded on every change regardless of paused: in live mode gocui redraws each tick
// anyway, but a size seen only while paused would turn the next Space press into a phantom resize
// of a screen that never changed.
func (d *resizeDetector) observe(paused bool, x, y int) bool {
	if x == d.lastX && y == d.lastY {
		return false
	}

	d.lastX, d.lastY = x, y

	return paused
}

// layout defines UI layout - set of screen areas and their locations.
func layout(app *app) func(g *gocui.Gui) error {
	// Remembers whether the previous frame already showed the "terminal too short" hint, so the
	// height-guard hint is emitted only when the guard state flips — not on every redraw frame.
	var verboseTooShortShown bool

	// Per-Gui state, deliberately reset on a UI rebuild - see resizeDetector.
	var resize resizeDetector

	return func(_ *gocui.Gui) error {
		maxX, maxY := app.ui.Size()

		// Screen dimensions could be equal to zero after executing external programs like pager/editor/psql.
		// Just return empty error and allow UI manager to redraw screen at next loop iteration.
		if maxX == 0 || maxY == 0 {
			return fmt.Errorf("")
		}

		// Verbose-aware band geometry. Pure arithmetic (see topBandLayout); config.verbose is
		// read in the gocui handler goroutine, same as app.config.view.ShowExtra below — no race.
		sysstatY1, pgstatY1, cmdlineY0, cmdlineY1, dbstatY0, expanded := topBandLayout(app.config.verbose, maxY)

		// Sysstat view.
		v, err := app.ui.SetView("sysstat", -1, -1, (maxX-1)/2, sysstatY1)
		if err != nil {
			if err != gocui.ErrUnknownView {
				return fmt.Errorf("set sysstat view on layout failed: %w", err)
			}
			if _, err := app.ui.SetCurrentView("sysstat"); err != nil {
				return fmt.Errorf("set sysstat view as current on layout failed: %w", err)
			}
		}
		if v != nil {
			v.Frame = false
		}

		// Postgres activity view.
		v, err = app.ui.SetView("pgstat", maxX/2, -1, maxX, pgstatY1)
		if err != nil {
			if err != gocui.ErrUnknownView {
				return fmt.Errorf("set pgstat view on layout failed: %w", err)
			}
		}
		if v != nil {
			v.Frame = false
		}

		// Command line.
		v, err = app.ui.SetView("cmdline", -1, cmdlineY0, maxX, cmdlineY1)
		if err != nil {
			if err != gocui.ErrUnknownView {
				return fmt.Errorf("set cmdline view on layout failed: %w", err)
			}
			// show saved error to user if any
			if app.uiError != nil {
				printCmdline(app.ui, "%s", app.uiError)
				app.uiError = nil
			}
		}
		if v != nil {
			v.Frame = false
		}

		// Postgres main stats view.
		v, err = app.ui.SetView("dbstat", -1, dbstatY0, maxX, maxY-1)
		if err != nil {
			if err != gocui.ErrUnknownView {
				return fmt.Errorf("set dbstat view on layout failed: %w", err)
			}
		}
		if v != nil {
			v.Frame = false
		}

		// Height-guard hint: verbose was requested but the terminal is too short to expand the
		// band, so layout() stayed compact. Emit a one-line hint only when this state flips on,
		// to avoid spamming printCmdline on every redraw frame.
		if app.config.verbose && !expanded {
			if !verboseTooShortShown {
				printCmdline(app.ui, "terminal too short for verbose mode")
				verboseTooShortShown = true
			}
		} else {
			verboseTooShortShown = false
		}

		// Extra stats view.
		if app.config.view.ShowExtra > stat.CollectNone {
			v, err := app.ui.SetView("extra", -1, 3*maxY/5-1, maxX, maxY-1)
			if err != nil {
				if err != gocui.ErrUnknownView {
					return fmt.Errorf("set extra view on layout failed: %w", err)
				}
				_, err := fmt.Fprintln(v, "")
				if err != nil {
					return fmt.Errorf("print extra stats failed: %w", err)
				}
			}
			if v != nil {
				v.Frame = false
			}
		}

		// While paused nothing else draws, so a resize would leave the frozen text with a
		// visible-column window computed for the old width. Ask for one repaint of the stored
		// frame; repaintStored enters g.Update itself, so the redraw lands in the next flush
		// instead of this pass - drawing here would make an error abort flush and rebuild the UI.
		// Last thing in the pass, after every SetView, and below the zero-geometry guard: a 0x0
		// size is neither recorded nor repainted into.
		if resize.observe(app.config.paused.Load(), maxX, maxY) {
			repaintStored(app)
		}

		return nil
	}
}

// cmdlineToken is a single token of the cmdline reserved prefix. variants holds the token's
// renderings ordered from longest to shortest; composeCmdline degrades a token to a shorter
// variant only when the line does not fit. A token that must never shrink supplies exactly one
// variant - that is how a state token like [PAUSED] is added without touching composeCmdline.
type cmdlineToken struct{ variants []string }

// composeCmdline builds the cmdline content: tokens rendered left to right, then a single space,
// then msg - the whole clamped to width RUNES (not bytes: the ellipsis and the column names are
// multi-byte). Truncation ladder, in order:
//  1. msg is cut;
//  2. the rightmost token steps to its next, shorter variant;
//  3. the rightmost token is dropped;
//  4. last resort, msg is hard rune-truncated.
//
// A token is never rendered partially - it is whole, degraded to a whole shorter variant, or
// absent. Pure: knows nothing about gocui or about the config.
func composeCmdline(tokens []cmdlineToken, msg string, width int) string {
	// A view can report zero width right after returning from a pager/editor.
	if width <= 0 {
		return ""
	}

	// Per-token index into its variants; kept is how many tokens, counted from the left, are
	// still rendered.
	idx := make([]int, len(tokens))
	kept := len(tokens)

	var prefix string
	for {
		prefix = renderCmdlineTokens(tokens[:kept], idx)
		if utf8.RuneCountInString(prefix) <= width {
			break
		}

		// Does not fit: degrade the rightmost token, or drop it when it has no shorter variant
		// left - which a single-variant token never has.
		last := kept - 1
		if idx[last] < len(tokens[last].variants)-1 {
			idx[last]++
			continue
		}
		kept--
	}

	if prefix == "" {
		return truncateRunes(msg, width)
	}
	if msg == "" {
		return prefix
	}

	rest := width - utf8.RuneCountInString(prefix)
	if rest < 2 {
		// No room for the separator plus at least one rune of the message: drop the message
		// entirely rather than leave a dangling separator at the end of the line.
		return prefix
	}

	return prefix + " " + truncateRunes(msg, rest-1)
}

// renderCmdlineTokens concatenates the currently selected variant of every passed token.
func renderCmdlineTokens(tokens []cmdlineToken, idx []int) string {
	var sb strings.Builder
	for i, t := range tokens {
		if len(t.variants) == 0 {
			continue
		}
		sb.WriteString(t.variants[idx[i]])
	}
	return sb.String()
}

// truncateRunes cuts s down to n runes, never splitting a rune.
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// filterToken builds the "[F:datname,usename]" token out of a view's filters and the column names
// known at that moment. ok is false when no active filter has a name to show.
//
// Names are emitted in ascending column-index order: view.Filters is a map, Go randomises map
// iteration, and an unsorted indicator would flicker between frames. Ascending index is also
// left-to-right screen order.
//
// Filter indices outside cols are skipped rather than indexed: cols belongs to the last successful
// render, so it is nil before the first one and can lag the filters by a frame after a screen
// switch. Pure: knows nothing about gocui or about the config.
func filterToken(filters map[int]*regexp.Regexp, cols []string) (cmdlineToken, bool) {
	keys := make([]int, 0, len(filters))
	for i, re := range filters {
		// The predicate must stay the one printHeaderCell marks a column with '*' by, otherwise
		// the header marker and the indicator would disagree about what "filtered" means.
		if !isFilterActive(re) {
			continue
		}
		if i < 0 || i >= len(cols) {
			continue
		}
		keys = append(keys, i)
	}
	sort.Ints(keys)

	names := make([]string, 0, len(keys))
	for _, i := range keys {
		name := stripControlRunes(cols[i])
		if name == "" {
			continue
		}
		names = append(names, name)
	}

	// An empty "[F:]" would claim a nameless filter exists - worse than showing no indicator.
	if len(names) == 0 {
		return cmdlineToken{}, false
	}

	// Ladder of renderings. A single name gets no "[F:datname,…]" step: an ellipsis after the only
	// name would read as "there are more filters".
	variants := []string{"[F:" + strings.Join(names, ",") + "]"}
	if len(names) > 1 {
		variants = appendCmdlineVariant(variants, "[F:"+names[0]+",…]")
	}
	variants = appendCmdlineVariant(variants, "[F:…]")

	return cmdlineToken{variants: variants}, true
}

// appendCmdlineVariant appends v unless it repeats the previous rendering - a ladder step that
// changes nothing would just cost a redraw.
func appendCmdlineVariant(variants []string, v string) []string {
	if len(variants) > 0 && variants[len(variants)-1] == v {
		return variants
	}
	return append(variants, v)
}

// stripControlRunes removes control runes from s. Column names come from the server's row
// description, so a crafted object name can carry escape sequences. The header wraps every cell in
// SGR and heals at the cell boundary, but the indicator emits no SGR of its own and
// gocui.View.Clear() does not reset the terminal's escape interpreter - an unterminated sequence
// reaching the cmdline would persist for the rest of the session. Zero-width control runes would
// also corrupt the rune count the truncation ladder depends on. Only control runes are removed: a
// printable tail such as "[31m" is ordinary text and stays.
func stripControlRunes(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// cmdlineTokens collects the active tokens of the cmdline reserved prefix, in left-to-right order.
//
// MUST be called from the gocui MainLoop goroutine only - inside a g.Update closure, in layout()
// or in a key handler: it reads config.view.Filters and config.view.Cols, which are written
// exclusively from that same goroutine. Nil-safe, because unit tests pass a bare config and the
// ambient can still be unset.
func cmdlineTokens(config *config) []cmdlineToken {
	if config == nil {
		return nil
	}

	var tokens []cmdlineToken
	// The pause marker goes first: the composer degrades and drops tokens from the RIGHT, so a
	// leftmost single-variant token is the last thing to go on a narrow terminal.
	if t, ok := pauseToken(config); ok {
		tokens = append(tokens, t)
	}
	if t, ok := filterToken(config.view.Filters, config.view.Cols); ok {
		tokens = append(tokens, t)
	}

	return tokens
}

// printCmdline prints formatted message on cmdline, behind the reserved state prefix, and arms the
// timer that restores the prefix-only line after 2 seconds.
func printCmdline(g *gocui.Gui, format string, s ...any) {
	writeCmdline(g, true, format, s...)
}

// printCmdlinePersist prints formatted message on cmdline WITHOUT arming the clear timer. Used for
// the dialog prompt, which has to survive until the dialog is closed - arming the timer there
// would erase the prompt from under the user's cursor after two seconds.
func printCmdlinePersist(g *gocui.Gui, format string, s ...any) {
	writeCmdline(g, false, format, s...)
}

// writeCmdline is the shared core of both cmdline writers: it composes the reserved prefix with
// the message, clamps the result to the view's width and prints it, optionally arming the timer
// that renders the prefix-only line afterwards.
func writeCmdline(g *gocui.Gui, arm bool, format string, s ...any) {
	// Do nothing if Gui is not defined.
	if g == nil {
		return
	}

	// Format the message in the caller's goroutine - one caller is reached from doWork. No config
	// field may be touched here; everything below runs in the gocui MainLoop goroutine instead.
	msg := fmt.Sprintf(format, s...)

	g.Update(func(g *gocui.Gui) error {
		v, err := g.View("cmdline")
		if err != nil {
			return fmt.Errorf("set focus on cmdline failed: %w", err)
		}

		width, _ := v.Size()
		v.Clear()
		_, err = fmt.Fprint(v, composeCmdline(cmdlineTokens(cmdlineCfg), msg, width))
		if err != nil {
			return fmt.Errorf("print on cmdline failed: %w", err)
		}

		// Restore the prefix-only line after 2 seconds. The condition looks at the ready message,
		// not at the format: printCmdline(g, "%s", "") would otherwise arm a timer to redraw what
		// is already on screen.
		if arm && msg != "" {
			// Capture the generation HERE, before starting the goroutine: read inside the
			// goroutine body it would observe whatever is current when the timer fires, i.e. it
			// would compare a value with itself.
			gen := uiGeneration.Load()

			t := time.NewTimer(2 * time.Second)
			go func() {
				<-t.C

				// Generation moved on: mainLoop has abandoned the Gui this timer was armed
				// against, and nobody drains its update channel any more.
				if uiGeneration.Load() != gen {
					return
				}

				g.Update(func(g *gocui.Gui) error {
					// Take the view by name again: over two seconds the UI could have been
					// rebuilt, and a captured *gocui.View would be written off the gocui
					// goroutine. A missing view means the line is no longer relevant - there is
					// nobody to report the error to, so return quietly.
					v, err := g.View("cmdline")
					if err != nil {
						return nil
					}

					width, _ := v.Size()
					v.Clear()
					_, err = fmt.Fprint(v, composeCmdline(cmdlineTokens(cmdlineCfg), "", width))
					if err != nil {
						return fmt.Errorf("print on cmdline failed: %w", err)
					}

					return nil
				})
			}()
		}

		return nil
	})
}
