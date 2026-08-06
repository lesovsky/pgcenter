package top

import (
	"testing"
	"time"

	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newKeybindingsApp builds an app whose keybindings have been registered on a zero-value Gui.
// That is enough: gocui.SetKeybinding only appends to a slice and DeleteKeybinding scans it,
// neither touches a terminal.
func newKeybindingsApp(t *testing.T) *app {
	t.Helper()

	app := &app{config: newConfig(), ui: &gocui.Gui{}}
	require.NoError(t, keybindings(app))
	return app
}

// boundHandler returns the handler the keys table carries for the given view and key, failing the
// test when the row is absent. Uniqueness is Test_keybindingsWAL's business; this helper is about
// WHICH handler a key carries, which no assertion on the registered gocui bindings can reach.
//
// The returned closure is freshly constructed by keybindingsList rather than the very one gocui
// registered - equivalent as long as every row is a stateless constructor over app/app.config,
// which is what the table contains today.
func boundHandler(t *testing.T, app *app, viewname string, k any) func(*gocui.Gui, *gocui.View) error {
	t.Helper()

	for _, b := range keybindingsList(app) {
		if b.viewname == viewname && b.key == k {
			return b.handler
		}
	}

	t.Fatalf("no binding for key %v on view %q", k, viewname)
	return nil
}

// Test_keybindingsWAL pins the 'W' binding of the WAL menu. 'W' being free was a reading of
// keybindings.go before this test existed; DeleteKeybinding turns that reading into a regression
// guard - it removes the first match and reports "keybinding not found" otherwise, so a single
// successful delete followed by a failing one means exactly one binding claims the key.
//
// Probing is destructive, so every assertion group gets a freshly registered app.
func Test_keybindingsWAL(t *testing.T) {
	t.Run("registered on sysstat exactly once", func(t *testing.T) {
		app := newKeybindingsApp(t)

		assert.NoError(t, app.ui.DeleteKeybinding("sysstat", 'W', gocui.ModNone))

		err := app.ui.DeleteKeybinding("sysstat", 'W', gocui.ModNone)
		require.Error(t, err)
		assert.Equal(t, "keybinding not found", err.Error())
	})

	t.Run("not claimed by any other view", func(t *testing.T) {
		for _, viewname := range []string{"", "menu", "dialog", "help"} {
			app := newKeybindingsApp(t)

			err := app.ui.DeleteKeybinding(viewname, 'W', gocui.ModNone)
			require.Error(t, err, "view %q must not bind 'W'", viewname)
			assert.Equal(t, "keybinding not found", err.Error())
		}
	})

	// The other edge of the lower-case half: Test_keybindingsWALCycles runs the handler the TABLE
	// carries, which stays green if the registration loop ever skips the row. This probe watches
	// what gocui actually received.
	//
	// Exactly once, like the 'W' probe above, and for a sharper reason: gocui's execKeybindings
	// runs EVERY matching handler rather than stopping at the first, so a duplicated 'w' row would
	// cycle twice per press (wal -> archiver -> wal) and read as a dead key.
	t.Run("lower-case 'w' still registered", func(t *testing.T) {
		app := newKeybindingsApp(t)

		assert.NoError(t, app.ui.DeleteKeybinding("sysstat", 'w', gocui.ModNone))

		err := app.ui.DeleteKeybinding("sysstat", 'w', gocui.ModNone)
		require.Error(t, err)
		assert.Equal(t, "keybinding not found", err.Error())
	})
}

// Test_keybindingsWALOpensMenu pins WHICH menu 'W' opens - the half that a uniqueness probe cannot
// see. Without it, binding 'W' to menuOpen(menuStatIO, ...) leaves the whole suite green while the
// user-visible behaviour is wrong.
//
// The handler is run rather than compared: closures are not comparable, and the effect is what
// matters. It also drives menuOpen for real, so the title and the item strings - the only text the
// 'W' path shows before a selection is made - are pinned here rather than by review.
//
// menuOpen ignores its *gocui.View argument, and on a zero-value &gocui.Gui{} its SetView,
// SetCurrentView and menuDraw calls all work without a terminal (see Test_menuSelectWAL).
func Test_keybindingsWALOpensMenu(t *testing.T) {
	app := &app{config: newConfig(), ui: &gocui.Gui{}}

	require.NoError(t, boundHandler(t, app, "sysstat", 'W')(app.ui, nil))

	assert.Equal(t, menuWAL, app.config.menu.menuType)
	assert.Equal(t, " Choose WAL / archiver mode (Enter to choose, Esc to exit): ", app.config.menu.title)
	assert.Equal(t, []string{" pg_stat_wal", " pg_stat_archiver"}, app.config.menu.items)

	mv, err := app.ui.View("menu")
	require.NoError(t, err)
	assert.Equal(t, app.config.menu.title, mv.Title)

	// Not the same assertion as the items slice above: that one reads the style menuOpen stored in
	// the config, this one reads what menuDraw actually wrote into the window. Without it, a
	// menuDraw that draws nothing is invisible to the whole package.
	for _, item := range app.config.menu.items {
		assert.Contains(t, mv.Buffer(), item)
	}
}

// Test_keybindingsWALCycles is the lower-case half. The 'w' row is byte-identical to what it was
// before this feature - only its MEANING changed, because switchViewTo gained the "wal" case - so
// nothing but a diff review stood between the primary entry point of the feature and silent
// deletion. Running the bound handler from the wal screen and expecting the archiver screen pins
// both facts at once: 'w' is still bound, and it is bound to the cycle.
//
// Same intentional leak as Test_menuSelectWAL: printCmdline calls g.Update, whose goroutine parks
// forever on the zero Gui's nil userEvents channel.
func Test_keybindingsWALCycles(t *testing.T) {
	app := &app{config: newConfig(), ui: &gocui.Gui{}}
	app.config.view = app.config.views["wal"]

	received := make(chan view.View, 1)
	go func() { received <- <-app.config.viewCh }()

	require.NoError(t, boundHandler(t, app, "sysstat", 'w')(app.ui, nil))

	select {
	case v := <-received:
		assert.Equal(t, "archiver", v.Name)
	case <-time.After(time.Second):
		t.Fatal("'w' did not send the next view on viewCh")
	}
}
