package top

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/stretchr/testify/assert"
)

// Test_pauseToken covers building the [PAUSED] marker off the config's pause state. The
// single-variant assertion is the substance of this test: a token with one rendering has no
// shorter step for composeCmdline's ladder to take, which is what makes the marker
// non-degradable. Nil-safety mirrors cmdlineTokens' contract - the ambient config is deliberately
// left nil in unit tests.
func Test_pauseToken(t *testing.T) {
	paused := newConfig()
	paused.paused.Store(true)

	testcases := []struct {
		name   string
		config *config
		wantOK bool
	}{
		{name: "nil config", config: nil, wantOK: false},
		{name: "fresh config", config: newConfig(), wantOK: false},
		{name: "paused config", config: paused, wantOK: true},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := pauseToken(tc.config)
			assert.Equal(t, tc.wantOK, ok)
			if !tc.wantOK {
				assert.Empty(t, got.variants)
				return
			}
			assert.Equal(t, []string{"[PAUSED]"}, got.variants)
		})
	}
}

// Test_togglePause covers the key handler: it flips the flag on every press. The handler must also
// tolerate a nil *gocui.Gui - both cmdline writers return silently on one, so the handler is
// reachable in a unit test without a terminal precisely because it never dereferences g itself.
func Test_togglePause(t *testing.T) {
	app := &app{config: newConfig()}
	handler := togglePause(app)

	assert.False(t, app.config.paused.Load())

	assert.NotPanics(t, func() {
		assert.NoError(t, handler(nil, nil))
	})
	assert.True(t, app.config.paused.Load())

	assert.NotPanics(t, func() {
		assert.NoError(t, handler(nil, nil))
	})
	assert.False(t, app.config.paused.Load())
}

// Test_repaintStored_invalidStoreIsNoop drives the REAL repaint closure, not a stub, with an empty
// store - the state that holds when the pause is engaged before the first frame ever rendered.
//
// Passing a nil *gocui.Gui is the assertion, not a shortcut: the closure would panic on the first
// g.View lookup, so the fact that it returns nil without panicking proves the valid check runs
// before any view is touched. It also proves the closure body is a named function - repaintStored
// itself cannot be called here, because g.Update dereferences the Gui.
//
// valid == false is a NORMAL state, so the failure latch must be left exactly as it was; the latch
// is pre-armed here so that a stray "success" report would clear it and fail the test.
func Test_repaintStored_invalidStoreIsNoop(t *testing.T) {
	app := &app{config: newConfig()}
	app.frame.repaintFailing = true

	closure := repaintStoredFrame(app)

	assert.NotPanics(t, func() {
		assert.NoError(t, closure(nil))
	})

	assert.False(t, app.frame.valid, "an empty store must stay empty")
	assert.True(t, app.frame.repaintFailing, "a no-op repaint must not touch the failure latch")
}

// Test_frameStore_publishOnlyOnLivePath is the immutability check of Decision 2, aimed at the one
// step that can actually regress: the publish helper.
//
// Both halves are needed. The first (repaint params must not write) fails if a repaint ever
// republishes - which would restamp the frozen frame with the repaint's own clock and make the
// header clock tick again. The second (live params must write) is what proves the first half is
// not asserting a helper that never writes at all.
func Test_frameStore_publishOnlyOnLivePath(t *testing.T) {
	t1 := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)

	frameA := makeRenderResult(3, 2)
	frameB := makeRenderResult(3, 5)

	var f frameStore

	publishFrame(&f, liveRender(t1), frameA, stat.CollectLogtail, "/var/log/postgresql/A.log", []byte("A lines\n"))
	assert.True(t, f.valid)
	assert.Equal(t, t1, f.at)
	assert.Equal(t, 2, f.stats.Result.Nrows)
	assert.Equal(t, "/var/log/postgresql/A.log", f.logPath)
	assert.Equal(t, []byte("A lines\n"), f.logBuf)

	// The repaint path's params, built from the store exactly as the repaint closure builds them.
	// The logtail pair offered here is a different one on purpose: a repaint must write NOTHING,
	// neither the frame nor the log buffer it is itself drawing from.
	publishFrame(&f, storedRender(&f), frameB, stat.CollectLogtail, "/var/log/postgresql/B.log", []byte("B lines\n"))
	assert.Equal(t, t1, f.at, "a repaint must not restamp the stored frame")
	assert.Equal(t, 2, f.stats.Result.Nrows, "a repaint must not replace the stored frame")
	assert.Equal(t, "/var/log/postgresql/A.log", f.logPath, "a repaint must not re-store the logtail path")
	assert.Equal(t, []byte("A lines\n"), f.logBuf, "a repaint must not re-store the logtail buffer")

	publishFrame(&f, liveRender(t2), frameB, stat.CollectLogtail, "/var/log/postgresql/B.log", []byte("B lines\n"))
	assert.Equal(t, t2, f.at)
	assert.Equal(t, 5, f.stats.Result.Nrows)
	assert.Equal(t, "/var/log/postgresql/B.log", f.logPath)
	assert.Equal(t, []byte("B lines\n"), f.logBuf)
}

// seededLogtailStore returns a store already holding a non-empty logtail pair - the state every
// syncLogtail case below starts from, since all four are about what happens to an EXISTING pair.
func seededLogtailStore() frameStore {
	return frameStore{logPath: "/var/log/postgresql/A.log", logBuf: []byte("line1\nline2\n")}
}

// Test_frameStore_syncLogtail_emptyReadKeepsBuffer is the Q24 defect expressed at the store level.
// readLogfileRecent returns a nil buffer whenever the log has not changed or is empty - the common
// case, not a rare one - and printLogtail then prints nothing and does not even clear the view, so
// the lines on screen are still the previous read's. Storing that nil would leave the panel empty on
// a quiet log, which is exactly the defect the user-spec closed: what must be stored is the last
// NON-EMPTY read, using the same predicate printLogtail uses.
func Test_frameStore_syncLogtail_emptyReadKeepsBuffer(t *testing.T) {
	testcases := []struct {
		name string
		buf  []byte
	}{
		{name: "nil buffer", buf: nil},
		{name: "empty buffer", buf: []byte{}},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			f := seededLogtailStore()

			f.syncLogtail(stat.CollectLogtail, "/var/log/postgresql/B.log", tc.buf)

			assert.Equal(t, []byte("line1\nline2\n"), f.logBuf, "an empty read must leave the shown lines stored")
			assert.Equal(t, "/var/log/postgresql/A.log", f.logPath, "an empty read must not move the header path either")
		})
	}
}

// Test_frameStore_syncLogtail_commitsPathAndBufferTogether pins that the two fields move as a pair.
// The path is the panel's header line and Reopen can change it, so a store that could hold one
// file's lines under another file's header would put a lie on the frozen screen.
func Test_frameStore_syncLogtail_commitsPathAndBufferTogether(t *testing.T) {
	f := seededLogtailStore()

	f.syncLogtail(stat.CollectLogtail, "/var/log/postgresql/B.log", []byte("B lines\n"))

	assert.Equal(t, []byte("B lines\n"), f.logBuf)
	assert.Equal(t, "/var/log/postgresql/B.log", f.logPath)
}

// Test_frameStore_syncLogtail_dropsForEveryNonLogtailShowExtra is the placement test. The store must
// be emptied whenever the live path is not showing the log, so a later repaint cannot redraw the
// previous file's lines under the previous file's header.
//
// CollectNone is the row that matters and the row a careless implementation misses: the live
// render's whole extra section is wrapped in `if app.config.view.ShowExtra > stat.CollectNone`
// (top/stat.go), so a drop written inside that block never executes for a CLOSED panel - the single
// most important case for the drop. Hence the table is driven over all four non-logtail values
// explicitly rather than over one representative.
//
// What this test pins is the FUNCTION's truth table, not its call site; that the single call sits
// outside the extra block, on the live side of publishFrame, stays a review-by-inspection item.
func Test_frameStore_syncLogtail_dropsForEveryNonLogtailShowExtra(t *testing.T) {
	testcases := []struct {
		name string
		show int
	}{
		{name: "panel closed", show: stat.CollectNone},
		{name: "switched to block devices", show: stat.CollectDiskstats},
		{name: "switched to network interfaces", show: stat.CollectNetdev},
		{name: "switched to filesystems", show: stat.CollectFsstats},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			f := seededLogtailStore()

			f.syncLogtail(tc.show, "", nil)

			assert.Nil(t, f.logBuf)
			assert.Empty(t, f.logPath)
		})
	}
}

// Test_frameStore_syncLogtail_capturesOnlyForLogtail closes the remaining row of the truth table:
// when the panel is not showing the log the drop wins over the buffer. A non-empty buffer arriving
// with a non-logtail ShowExtra is a stale local left over from an earlier frame, and committing it
// would file one panel's content under another panel's state.
func Test_frameStore_syncLogtail_capturesOnlyForLogtail(t *testing.T) {
	testcases := []struct {
		name string
		show int
	}{
		{name: "panel closed", show: stat.CollectNone},
		{name: "switched to block devices", show: stat.CollectDiskstats},
		{name: "switched to network interfaces", show: stat.CollectNetdev},
		{name: "switched to filesystems", show: stat.CollectFsstats},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			f := seededLogtailStore()

			f.syncLogtail(tc.show, "/var/log/postgresql/B.log", []byte("B lines\n"))

			assert.Nil(t, f.logBuf, "a non-logtail panel must not capture a buffer")
			assert.Empty(t, f.logPath)
		})
	}
}

// Test_frameStore_repaintRendersIdenticalBytes pins that the frozen frame renders identically
// before and after a frame is discarded by the paused gate: the gate cannot reach the store
// (Decision 1) and the render of a stored frame is idempotent (task 01 made printDataCell
// non-mutating), so the two byte streams must match exactly.
//
// renderSysstat is NOT part of this assertion, and since task 04 the reason is a scope boundary
// rather than a flakiness guard: the summary panel now takes the render timestamp as a parameter
// instead of reading the wall clock, and that the stored stamp is what it prints is asserted by
// task 04's own tests in top/stat_test.go. What this test owns is the table half - that the gate
// cannot disturb the stored frame - so it renders the table alone.
func Test_frameStore_repaintRendersIdenticalBytes(t *testing.T) {
	const termWidth = 40

	cfg := makeRenderConfig(6, 10)
	app := &app{config: cfg}

	frozen := makeRenderResult(6, 3)
	// No extra panel is open in this test's config, so the logtail arguments are the zero pair the
	// live path hands over when it drew no log at all.
	publishFrame(&app.frame, liveRender(time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)), frozen, stat.CollectNone, "", nil)

	var before bytes.Buffer
	assert.NoError(t, renderDbstat(&before, cfg, app.frame.stats, termWidth))

	// A frame arrives while paused and goes through the real gate.
	cfg.paused.Store(true)
	statCh := make(chan stat.Stat)
	repainted := make(chan struct{}, 1)
	exit := make(chan statLoopExit, 1)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		exit <- statLoop(ctx, make(chan int), statCh, &cfg.paused,
			func(stat.Stat) { panic("render must never be called while paused") },
			func() { repainted <- struct{}{} },
		)
	}()

	statCh <- makeRenderResult(6, 9) // a different frame, deliberately
	<-repainted
	cancel()
	assert.Equal(t, exitCtx, <-exit)

	var after bytes.Buffer
	assert.NoError(t, renderDbstat(&after, cfg, app.frame.stats, termWidth))

	assert.Equal(t, before.String(), after.String())
}

// Test_repaintFailureLatch_reportsOncePerTransition drives the latch directly. It has to be
// driveable without a *gocui.Gui: the repaint closure's only failure site is g.View(...), and
// neither a *gocui.Gui nor a *gocui.View can be constructed outside the gocui package (the same
// property the comment on Test_printCmdlineNilGui records, top/ui_test.go). So a test can never
// MAKE the real closure fail - which is exactly why the transition logic has to live in a unit of
// its own instead of a bare if inside the closure.
//
// The sequence is fail, fail, fail, success, fail: one report per no-failure -> failure transition,
// silence on repeats, and re-arming after a success.
func Test_repaintFailureLatch_reportsOncePerTransition(t *testing.T) {
	boom := errors.New("repaint failed")

	testcases := []struct {
		err  error
		want bool
	}{
		{err: boom, want: true},
		{err: boom, want: false},
		{err: boom, want: false},
		{err: nil, want: false},
		{err: boom, want: true},
	}

	var f frameStore
	for i, tc := range testcases {
		assert.Equal(t, tc.want, f.recordRepaint(tc.err), "step %d", i)
	}
}

// newPausedApp builds an application sitting on the requested screen with the display paused - the
// starting state of every lifting and no-op case below.
func newPausedApp(viewName string) *app {
	config := newConfig()
	config.view = config.views[viewName]
	config.paused.Store(true)

	return &app{config: config}
}

// Test_liftPause covers the silent helper: it clears the flag and does nothing else. Its signature
// is part of the assertion - it takes only a *config, because viewSwitchHandler and changeQueryAge
// have no *gocui.Gui to give it, so the compiler enforces that half.
//
// The second call is not a repetition: lifting runs on every classified keypress, and in live mode
// the flag is already down, so being harmless on an already-cleared flag is the common case.
func Test_liftPause(t *testing.T) {
	config := newConfig()
	config.paused.Store(true)

	liftPause(config)
	assert.False(t, config.paused.Load())

	assert.NotPanics(t, func() { liftPause(config) })
	assert.False(t, config.paused.Load())
}

// Test_liftPauseRefresh covers the refreshing helper. A nil *gocui.Gui is the point rather than a
// shortcut: writeCmdline returns on one (top/ui.go:491), which is what keeps the helper reachable
// from a unit test - and what makes the cmdline write itself unobservable here. So the observable
// contract is only "the flag is cleared, nothing panics"; that the CompareAndSwap suppresses the
// re-render on an already-cleared flag is a review item, not a test case.
func Test_liftPauseRefresh(t *testing.T) {
	config := newConfig()
	config.paused.Store(true)

	assert.NotPanics(t, func() { liftPauseRefresh(nil, config) })
	assert.False(t, config.paused.Load())

	assert.NotPanics(t, func() { liftPauseRefresh(nil, config) })
	assert.False(t, config.paused.Load())
}

// Test_liftingHandlers walks the handlers whose effect is produced by the collector and which are
// reachable without a live database. Each must leave the pause lifted, otherwise the operator
// presses the key and sees nothing change until they remember Space.
//
// Not covered here, deliberately: switchViewToProcPidStat on a LOCAL connection queries
// pg_stat_activity right after its guard, so only its remote no-op is unit-testable (see
// Test_noOpHandlersKeepPause); the positive path is step 7 of the stand run.
func Test_liftingHandlers(t *testing.T) {
	testcases := []struct {
		name string
		view string
		run  func(t *testing.T, app *app)
	}{
		{
			name: "viewSwitchHandler",
			view: "activity",
			run: func(_ *testing.T, app *app) {
				viewSwitchHandler(app.config, "databases_general")
			},
		},
		{
			name: "orderKeyLeft",
			view: "activity",
			run: func(t *testing.T, app *app) {
				assert.NoError(t, orderKeyLeft(app.config)(nil, nil))
			},
		},
		{
			name: "orderKeyRight",
			view: "activity",
			run: func(t *testing.T, app *app) {
				assert.NoError(t, orderKeyRight(app.config)(nil, nil))
			},
		},
		{
			name: "switchSortOrder",
			view: "activity",
			run: func(t *testing.T, app *app) {
				assert.NoError(t, switchSortOrder(app.config)(nil, nil))
			},
		},
		{
			name: "toggleSysTables",
			view: "tables",
			run: func(t *testing.T, app *app) {
				assert.NoError(t, toggleSysTables(app.config)(nil, nil))
			},
		},
		{
			name: "toggleIdleConns",
			view: "activity",
			run: func(t *testing.T, app *app) {
				assert.NoError(t, toggleIdleConns(app.config)(nil, nil))
			},
		},
		{
			name: "toggleVerbose",
			view: "activity",
			run: func(t *testing.T, app *app) {
				assert.NoError(t, toggleVerbose(app)(nil, nil))
			},
		},
		{
			name: "changeQueryAge",
			view: "activity",
			run: func(t *testing.T, app *app) {
				assert.Equal(t, "Activity age: set 00:01:00", changeQueryAge("00:01:00", app.config))
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			app := newPausedApp(tc.view)

			// viewCh is unbuffered: drain it until it is closed rather than receiving a fixed
			// number of views, so the case cannot deadlock on a handler that pushes more than once.
			drained := make(chan struct{})
			go func() {
				for range app.config.viewCh { //nolint:revive // draining, the values are asserted elsewhere
				}
				close(drained)
			}()

			tc.run(t, app)

			assert.False(t, app.config.paused.Load(), "the handler must lift the pause")

			close(app.config.viewCh)
			<-drained
		})
	}
}

// Test_noOpHandlersKeepPause is the other half of the contract: an action that changed nothing must
// leave the freeze intact. Each case is an early return of a handler that lifts on its normal path.
//
// The handler runs in a goroutine and the outcome is taken with a select over three channels. That
// is not ceremony: viewCh is unbuffered, so a handler that wrongly asked for a fresh frame would
// block forever, and "the test did not hang" on its own proves nothing about the send.
func Test_noOpHandlersKeepPause(t *testing.T) {
	testcases := []struct {
		name  string
		view  string
		setup func(app *app)
		run   func(t *testing.T, app *app)
	}{
		{
			// 'x' on an installation without pg_stat_statements: the guard lives in switchViewTo,
			// above the viewSwitchHandler call that carries the lift.
			name: "switchViewTo statements without pg_stat_statements",
			view: "activity",
			run: func(t *testing.T, app *app) {
				assert.NoError(t, switchViewTo(app, "statements")(nil, nil))
			},
		},
		{
			// 'S' on a remote connection: per-process stats are local-only.
			name: "switchViewToProcPidStat on a remote connection",
			view: "activity",
			setup: func(app *app) {
				app.db = &postgres.DB{Local: false}
			},
			run: func(t *testing.T, app *app) {
				assert.NoError(t, switchViewToProcPidStat(app)(nil, nil))
			},
		},
		{
			// 'L' on a remote connection: the first of the logtail no-ops.
			name: "showExtra logtail on a remote connection",
			view: "activity",
			setup: func(app *app) {
				app.db = &postgres.DB{Local: false}
			},
			run: func(t *testing.T, app *app) {
				assert.NoError(t, showExtra(app, stat.CollectLogtail)(nil, nil))
			},
		},
		{
			name: "toggleSysTables outside tables/indexes/sizes",
			view: "activity",
			run: func(t *testing.T, app *app) {
				assert.NoError(t, toggleSysTables(app.config)(nil, nil))
			},
		},
		{
			name: "toggleIdleConns outside activity/procpidstat",
			view: "databases_general",
			run: func(t *testing.T, app *app) {
				assert.NoError(t, toggleIdleConns(app.config)(nil, nil))
			},
		},
		{
			name: "changeQueryAge out of range",
			view: "activity",
			run: func(t *testing.T, app *app) {
				assert.Equal(t, "Activity age: do nothing, invalid input", changeQueryAge("25:00:00", app.config))
			},
		},
		{
			name: "changeQueryAge unparsable",
			view: "activity",
			run: func(t *testing.T, app *app) {
				assert.Equal(t, "Activity age: do nothing, invalid input", changeQueryAge("abc", app.config))
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			app := newPausedApp(tc.view)
			if tc.setup != nil {
				tc.setup(app)
			}

			done := make(chan struct{})
			go func() {
				tc.run(t, app)
				close(done)
			}()

			select {
			case <-done:
			case v := <-app.config.viewCh:
				t.Fatalf("a no-op handler asked the collector for a fresh frame: %s", v.Name)
			case <-time.After(2 * time.Second):
				t.Fatal("the handler neither returned nor pushed on viewCh")
			}

			assert.True(t, app.config.paused.Load(), "an action that changed nothing must not lift the pause")
		})
	}
}

// THE menuConf BRANCH ('E') HAS NO TEST HERE, AND CANNOT USEFULLY HAVE ONE. It opens an editor,
// i.e. a UI-rebuild path where the pause is required to survive (Decision 9), and it is the most
// error-prone exclusion in this feature - so it is worth being explicit about what protects it.
//
// It is protected STRUCTURALLY, by two signatures. menuConf's terminal call is
// editPgConfig(g, db, filename, uiExit) (top/pgconfig.go), which receives no *config and therefore
// has nothing to lift; and the branch calls it directly instead of going through viewSwitchHandler,
// which is where the lift for every other screen switch lives.
//
// menuSelect itself is unreachable from a unit test, but NOT because of its *gocui.View argument:
// a zero-value &gocui.View{} is constructible from outside the gocui package and answers v.Cursor()
// with (0,0), which is enough to route into the menuConf branch. The real blocker is one line
// later - menuSelect ends with an unconditional `return menuClose(g, v)` on EVERY branch, and
// menuClose calls g.DeleteView("menu") and g.SetCurrentView("sysstat") on the *gocui.Gui
// (top/menu.go). A nil Gui panics there, and a live one comes only from gocui.NewGui, which opens a
// real terminal backend. No formulation of the test can dodge it, because no branch skips
// menuClose.
//
// An earlier revision of this file did have a Test_menuConfPathDoesNotLift. It built a local
// config, called editPgConfig, and asserted the local config was still paused - on a config the
// callee never receives. It could not fail, which is worse than no test at all: it read as a
// regression guard for the one exclusion that most needs one. Do not restore it, and do not invent
// a seam to thread a *config into editPgConfig purely to observe this - the task forbids exactly
// that. The runtime check is step 7 of the stand run ('E' -> the editor opens, [PAUSED] survives);
// the static check is diff review, i.e. that top/menu.go and top/pgconfig.go contain no
// liftPause/liftPauseRefresh call.

// Test_showExtraCloseLifts covers the panel-CLOSING path of showExtra, which is the one place in
// the feature where nothing else writes the cmdline - neither closeExtraView, nor layout, nor
// printStat - so a silent lift would strand [PAUSED] over live data.
//
// The handler runs in a goroutine with NO reader on viewCh: it lifts, then parks on the unbuffered
// send inside closeExtraView and therefore never reaches g.DeleteView on the nil Gui. The goroutine
// stays parked on purpose - draining viewCh would let it run on and dereference that nil Gui, and
// closing viewCh would panic on the send. A non-logtail panel type is used so the Close() branch is
// not entered.
//
// The parked goroutine therefore outlives the test, one per run. That is an INTENTIONAL leak, not
// an oversight: if a goroutine-leak detector is ever wired into this package, this test needs an
// exemption rather than a "fix".
func Test_showExtraCloseLifts(t *testing.T) {
	app := newPausedApp("activity")
	app.config.view.ShowExtra = stat.CollectDiskstats

	go func() { _ = showExtra(app, stat.CollectDiskstats)(nil, nil) }()

	assert.Eventually(t, func() bool { return !app.config.paused.Load() },
		2*time.Second, 5*time.Millisecond,
		"closing the extra panel must lift the pause before it asks the collector for a frame")
}

// Test_showExtraCloseLogtailErrorKeepsPause is the same closing path when the panel does not
// actually close: with a zero config.logtail, Logfile.Close() calls Close() on a nil *os.File,
// which returns os.ErrInvalid rather than panicking, so showExtra leaves through its error return
// above the lift. Nothing closed, nothing to lift.
func Test_showExtraCloseLogtailErrorKeepsPause(t *testing.T) {
	app := newPausedApp("activity")
	app.config.view.ShowExtra = stat.CollectLogtail

	assert.Error(t, showExtra(app, stat.CollectLogtail)(nil, nil))
	assert.True(t, app.config.paused.Load(), "a panel that did not close must not lift the pause")

	select {
	case v := <-app.config.viewCh:
		t.Fatalf("a failed close asked the collector for a fresh frame: %s", v.Name)
	case <-time.After(50 * time.Millisecond):
	}
}
