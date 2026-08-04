// Pause state and the objects owned by the gocui goroutine.
//
// Everything in this file runs on the gocui MainLoop goroutine - in a key handler or inside a
// g.Update closure. The one value that crosses goroutines is config.paused, which is an atomic for
// exactly that reason; nothing else here may be touched from the worker goroutine.

package top

import (
	"time"

	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/stat"
)

// frameStore holds the frame that is currently on screen, so it can be redrawn while the collector's
// new frames are being discarded.
//
// OWNERSHIP: written AND read only on the gocui MainLoop goroutine - it is published at the tail of
// the render closure (renderFrame, top/stat.go) and read inside the repaint closure below. It
// therefore needs no synchronisation primitive at all, and adding one would be a design error
// rather than extra safety. Two happens-before edges make that race-free: the collector's statCh
// send is ordered before the worker's receive, and the worker's g.Update(f) is ordered before
// MainLoop running f - so everything the worker captured is visible inside the closure, and the
// store appears in exactly one goroutine's instruction stream. The same discipline as cmdlineCfg
// (top/ui.go:18-22) and writeCmdline (top/ui.go:437-448).
//
// The worker goroutine cannot violate this even by accident: statLoop receives neither *app nor the
// store, and its repaint parameter is a bare func() (Decision 12), so the gate has nothing to read
// the store from. A read on the worker goroutine would be a torn read - Nrows from one frame and
// Values from another - i.e. a slice-bounds panic inside a g.Update closure, which gocui does not
// recover.
//
// Unexported fields and no getters in production code: the only way to draw the store is
// repaintStored, which enters g.Update itself.
type frameStore struct {
	stats   stat.Stat // the frozen frame, as received from the collector. Retained, not copied: printDataCell is non-mutating (Decision 10), so rendering it is idempotent.
	at      time.Time // when it was rendered - this is what freezes the header clock (task 04 consumes it).
	logBuf  []byte    // last NON-EMPTY logtail read; nil when the panel is not showing the log. See syncLogtail.
	logPath string    // the path that buffer came from.
	valid   bool      // false until the first frame has been rendered.

	// repaintFailing latches "the previous repaint failed", so a persistently failing repaint
	// reports itself exactly once instead of on every refresh interval. See recordRepaint.
	repaintFailing bool
}

// recordRepaint takes the outcome of a repaint and reports whether a cmdline message must be
// emitted, updating the latch as a side effect: exactly one message on the no-failure -> failure
// transition, silence while the failure persists, and re-arming after a repaint succeeds again.
//
// It is a separately callable unit rather than a bare if inside the repaint closure because that
// closure cannot be driven from a test: its only failure site is g.View(...), and neither a
// *gocui.Gui nor a *gocui.View can be constructed outside the gocui package. Splitting the state
// out is what makes the transition behaviour testable at all.
func (f *frameStore) recordRepaint(err error) bool {
	if err == nil {
		f.repaintFailing = false
		return false
	}

	if f.repaintFailing {
		return false
	}

	f.repaintFailing = true
	return true
}

// renderParams carries the four things the shared render core (renderFrame, top/stat.go) must do
// differently on the live path and on the repaint path. It exists as DATA, built by the two
// constructors below, rather than as a bool inside the core, for two reasons: the invariant "the
// repaint path never publishes" becomes checkable by a test that never needs a *gocui.Gui, and the
// value is the extension point later waves attach to.
//
// EXTENSION POINT - do not "simplify" this away. Task 09 (wave 5) attaches the logtail capture to
// the live side of the publish step, and task 04 threads the at field down into the summary panel so
// the frozen frame stops showing a live wall clock. Both seams are cut here deliberately, before
// their consumers exist: task 04 runs in wave 3 and task 09 in wave 5, and neither may edit this
// file - so a seam cut later would force a later wave to reopen a file it does not own.
type renderParams struct {
	at       time.Time // the render timestamp: freshly captured on the live path, the stored one on repaint.
	fromFile bool      // logtail source: read the log file (live) or draw the stored buffer (repaint).
	logBuf   []byte    // the stored logtail content; meaningful only when fromFile is false.
	logPath  string    // the path that content came from.
	publish  bool      // whether the rendered frame is published into the store. Live only (Decision 2).
}

// liveRender builds the render params of the live path: at is the stamp captured for this frame,
// the logtail comes from the file, and what is rendered becomes the stored frame.
func liveRender(at time.Time) renderParams {
	return renderParams{at: at, fromFile: true, publish: true}
}

// storedRender builds the render params of the repaint path out of the store: the frozen stamp, the
// stored logtail buffer, and NO publication.
//
// The repaint must not republish. It would restamp the frozen frame with the repaint's own clock -
// the header clock would tick again, defeating task 04 - and the logtail buffer would be re-stored
// from itself. While paused the store is immutable by construction (Decision 2).
func storedRender(f *frameStore) renderParams {
	return renderParams{at: f.at, fromFile: false, logBuf: f.logBuf, logPath: f.logPath}
}

// syncLogtail is the ONE decision about the stored logtail pair, and both of its predicates live
// here rather than as two ifs at different nesting depths inside the render closure.
//
// show is the live path's current view.ShowExtra, path/buf are the pair that path just drew (zero
// values when it drew no log at all). Three cases:
//
//   - the panel is not showing the log - closed, or switched to B/N/F: DROP the pair. A repaint must
//     never redraw the previous file's lines under the previous file's header. The closed panel
//     (stat.CollectNone) is the important half: the live render's extra section is wrapped in
//     `if ShowExtra > stat.CollectNone`, so a drop written inside that block would never run for it.
//   - the panel shows the log but the read was empty: KEEP the previous pair. readLogfileRecent
//     returns a nil buffer whenever the file has not changed or is empty - the common case - and
//     printLogtail then draws nothing and does not even clear the view, so the lines on screen are
//     still the previous read's. Storing the nil would blank the panel on a quiet log.
//   - the panel shows the log and the read had content: CAPTURE both fields together. The predicate
//     is deliberately the same one printLogtail uses, so the store and the screen can never disagree
//     about what "shown" means, and the path travels with the buffer because it is the panel's
//     header line and Reopen can change it.
//
// Size is not stored: it is change-detection bookkeeping, never rendered. It freezes at the last
// live frame's value, which is exactly what the rotation detector needs on resume.
func (f *frameStore) syncLogtail(show int, path string, buf []byte) {
	if show != stat.CollectLogtail {
		f.logBuf, f.logPath = nil, ""
		return
	}

	if len(buf) == 0 {
		return
	}

	f.logBuf, f.logPath = buf, path
}

// publishFrame stores the frame that has just been rendered - but only when p says this is the live
// path. It is called at the TAIL of renderFrame, after the panels have been drawn, so the stored
// frame is by construction the frame that was on screen: a render that failed halfway returns
// before reaching this point and leaves the previous frame stored.
//
// show/logPath/logBuf carry the logtail capture, which is why it is attached HERE rather than at its
// own call site in renderFrame: this is the only step the repaint path is already known to skip. An
// unconditional syncLogtail in renderFrame would compile, pass every store-level test, and quietly
// let a repaint write the store - contradicting Decision 2.
func publishFrame(f *frameStore, p renderParams, s stat.Stat, show int, logPath string, logBuf []byte) {
	if !p.publish {
		return
	}

	f.stats, f.at, f.valid = s, p.at, true
	f.syncLogtail(show, logPath, logBuf)
}

// repaintStored asks for a redraw of the stored frame.
//
// It deliberately extracts NOTHING from the store before entering g.Update - not even the valid
// flag. It is called from the worker goroutine (the pause gate), where reading a gocui-owned field
// would reintroduce, in miniature, exactly the race Decision 1 removes. It only asks.
func repaintStored(app *app) {
	app.ui.Update(repaintStoredFrame(app))
}

// repaintStoredFrame is the body of the repaint closure. It is a named function rather than a
// literal inside repaintStored so a test can drive the real closure instead of a stub - the only
// mechanical check the ownership rule has, since -race cannot catch a violation (statLoop's tests
// use stub render/repaint functions and never touch the store from two goroutines).
//
// It returns nil UNCONDITIONALLY. An error out of a g.Update closure tears down MainLoop
// (gocui/gui.go:377-379), which rebuilds the UI, which repaints again, which fails again - bounded
// only by the errorRate guard that kills the process. A failure is reported through the latch
// instead, once per transition.
func repaintStoredFrame(app *app) func(g *gocui.Gui) error {
	return func(g *gocui.Gui) error {
		// valid == false is a NORMAL state, not an error: the pause was engaged before the first
		// frame ever rendered, which the user-spec explicitly allows. Draw nothing, report nothing,
		// touch no latch - the screen stays empty under a live [PAUSED] and the first frame after
		// the pause is lifted renders normally.
		//
		// The check lives HERE, inside the closure, and not in the gate: testing it there would
		// read a gocui-owned field from the worker goroutine.
		if !app.frame.valid {
			return nil
		}

		err := renderFrame(g, app, app.frame.stats, app.postgresProps, storedRender(&app.frame))
		if app.frame.recordRepaint(err) {
			printCmdline(g, "repaint of the paused screen failed: %s", err)
		}

		return nil
	}
}

// pauseToken builds the "[PAUSED]" token of the cmdline reserved prefix. ok is false when the
// display is not paused, so the marker is absent rather than empty.
//
// The token carries exactly ONE variant on purpose: composeCmdline degrades a token by stepping to
// a shorter rendering, and a token without a shorter rendering is dropped whole instead. That is
// what makes the marker either fully readable or absent - never a truncated "[PAUSE".
//
// Nil-safe, mirroring cmdlineTokens' contract: it is called with the package ambient cmdlineCfg,
// which unit tests deliberately leave nil.
func pauseToken(config *config) (cmdlineToken, bool) {
	if config == nil || !config.paused.Load() {
		return cmdlineToken{}, false
	}

	return cmdlineToken{variants: []string{"[PAUSED]"}}, true
}

// togglePause flips the pause flag and re-renders the cmdline so the marker appears or disappears
// on the same keypress.
//
// The re-render is a single write with an empty message: writeCmdline recomposes the whole line,
// token prefix included, and skips arming its clear timer when the message is empty (top/ui.go:461)
// - so the prefix-only line stays until something else writes it. No text message accompanies the
// toggle: the user-spec forbids one, and a second write on the same path is defect class [027].
//
// g is never dereferenced here - printCmdline returns silently on a nil Gui, which is what keeps
// the handler unit-testable without a terminal. The flip side is that the write itself is not
// observable in a unit test: a nil Gui swallows it. Only the flag flip is covered by
// Test_togglePause; that there is exactly ONE write and no message is kept by review and by the
// stand run - so do not add a second cmdline call here on the assumption that a test would catch it.
//
// The read-modify-write is deliberately not a CAS loop: every writer of config.paused runs on the
// gocui goroutine (this handler and the handlers that lift the pause), so no other writer can
// interleave. The atomic is there for the worker goroutine that READS the flag.
func togglePause(app *app) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		app.config.paused.Store(!app.config.paused.Load())

		printCmdline(g, "")

		return nil
	}
}

// liftPause clears the pause flag and does nothing else. It is the variant for handlers that write
// the cmdline themselves, or whose caller writes it for them.
//
// It takes no *gocui.Gui on purpose: viewSwitchHandler and changeQueryAge have none in scope, and
// giving the helper a parameter they cannot supply would force one of them to grow one.
//
// PLACEMENT, the same two rules for both helpers:
//
//  1. Call it AFTER the handler's early returns. An action that changed nothing - 'x' without
//     pg_stat_statements, 'S' on a remote connection, 'L' on an unreadable log - requested no fresh
//     data, so it must leave the freeze intact.
//  2. Every lifting path must end up performing EXACTLY ONE cmdline write: its own, its caller's,
//     or the one liftPauseRefresh does. A path with none strands [PAUSED] over live data; a path
//     with two overwrites the first message before it can be read (defect class [027]).
//
// The position relative to the handler's own cmdline write is irrelevant, before or after: the flag
// is cleared synchronously here, in the handler's goroutine, while printCmdline only enqueues a
// closure that MainLoop runs later - necessarily after the handler has returned. So any write on
// the path composes the token prefix with the flag already down.
func liftPause(config *config) {
	config.paused.Store(false)
}

// liftPauseRefresh clears the pause flag and, when this call is the one that cleared it, re-renders
// the cmdline once so the [PAUSED] marker disappears. It is the variant for handlers on whose path
// nobody writes the cmdline at all.
//
// The re-render is the same mechanism togglePause uses: a single write with an empty message, which
// recomposes the whole line including the token prefix and skips arming the clear timer
// (top/ui.go:515). See liftPause for the placement rules; they apply here unchanged.
//
// The clear goes through CompareAndSwap, and the re-render happens ONLY when the swap succeeded. In
// live mode - which is most keypresses - the flag is already down, and an unconditional re-render
// would wipe a transient message that still had up to two seconds to live: today orderKeyLeft and
// orderKeyRight write no cmdline at all, so "pressed '<', read the message, pressed the arrow, the
// message vanished" would be new behaviour nobody asked for. Rule 2 is not weakened by this: a path
// whose flag was already down is not a lifting path, and on it the cmdline is written by exactly
// whoever wrote it before this feature.
func liftPauseRefresh(g *gocui.Gui, config *config) {
	if config.paused.CompareAndSwap(true, false) {
		printCmdline(g, "")
	}
}
