package top

import (
	"testing"
	"time"

	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_selectMenuStyle(t *testing.T) {
	testcases := []struct {
		menu menuType
		want int
	}{
		{menu: menuNone, want: 0},
		{menu: menuDatabases, want: 2},
		{menu: menuPgss, want: 7},
		{menu: menuProgress, want: 6},
		{menu: menuConf, want: 4},
		{menu: menuStatIO, want: 2},
		{menu: menuWAL, want: 2},
		{menu: menuTables, want: 2},
	}

	for _, tc := range testcases {
		got := selectMenuStyle(tc.menu, 0)
		assert.Equal(t, tc.want, len(got.items))
	}
}

func Test_menuTablesVersionLabels(t *testing.T) {
	testcases := []struct {
		name    string
		version int
		want    []string
	}{
		{
			name:    "below PG 19",
			version: query.PostgresV18,
			want:    []string{" pg_stat_user_tables", " pg_stat_autovacuum_scores — requires PostgreSQL 19"},
		},
		{
			name:    "PG 19",
			version: query.PostgresV19,
			want:    []string{" pg_stat_user_tables", " pg_stat_autovacuum_scores"},
		},
		{
			name:    "above PG 19",
			version: 200000,
			want:    []string{" pg_stat_user_tables", " pg_stat_autovacuum_scores"},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got := selectMenuStyle(menuTables, tc.version)
			assert.Equal(t, menuTables, got.menuType)
			assert.Equal(t, tc.want, got.items)
		})
	}
}

// Test_menuSelectWAL drives the real menuSelect closure over the menuWAL branch: the cursor
// position is resolved to a view name, the view is sent on viewCh and the menu is reset.
//
// A zero-value &gocui.Gui{} is enough for that: SetView builds a real *gocui.View from the passed
// coordinates without touching a terminal (ErrUnknownView is its "created" signal, which menu.go
// keys off too), while DeleteView/SetCurrentView only scan a slice. The "sysstat" view exists
// because menuClose focuses it at the end of every menuSelect path; without it menuSelect would
// return ErrUnknownView. The "menu" view is taller than the production geometry (menu.go sizes it
// 0,5..72,6+len(items)) so SetCursor can reach a row beyond the two items and exercise the
// default arm — out-of-view rows are rejected with "invalid point".
//
// Note the leak: printCmdline calls g.Update, which spawns a goroutine that parks forever on the
// zero Gui's nil userEvents channel — one per sub-test. Same intentional class as
// Test_showExtraCloseLifts (top/pause_test.go): a goroutine-leak detector would need an exemption
// here rather than a "fix".
//
// What this does NOT cover: menuOpen itself. The menu state is built by hand from
// selectMenuStyle(menuWAL) — the same state menuOpen would leave — so the 'W' → menu-window step
// (title, geometry, menuDraw) is covered by Test_selectMenuStyle plus the stand run.
func Test_menuSelectWAL(t *testing.T) {
	testcases := []struct {
		name string
		cy   int
		want string
	}{
		{name: "first item", cy: 0, want: "wal"},
		{name: "second item", cy: 1, want: "archiver"},
		{name: "out of range", cy: 5, want: "wal"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			g := &gocui.Gui{}

			_, err := g.SetView("sysstat", 0, 0, 20, 20)
			require.Equal(t, gocui.ErrUnknownView, err)

			mv, err := g.SetView("menu", 0, 5, 72, 20)
			require.Equal(t, gocui.ErrUnknownView, err)
			require.NoError(t, mv.SetCursor(0, tc.cy))

			app := &app{config: newConfig(), ui: g}
			app.config.view = app.config.views["activity"]
			app.config.menu = selectMenuStyle(menuWAL, 0)

			received := make(chan view.View, 1)
			go func() { received <- <-app.config.viewCh }()

			assert.NoError(t, menuSelect(app)(g, mv))

			select {
			case v := <-received:
				assert.Equal(t, tc.want, v.Name)
			case <-time.After(time.Second):
				t.Fatal("menu selection did not send the new view on viewCh")
			}

			// The reset at the end of menuSelect is what keeps the next menu press sane.
			assert.Equal(t, menuNone, app.config.menu.menuType)
		})
	}
}

func Test_menuSelectTables(t *testing.T) {
	testcases := []struct {
		name     string
		version  int
		cy       int
		want     string
		wantSend bool
	}{
		{name: "PG 19 first item", version: query.PostgresV19, cy: 0, want: "tables", wantSend: true},
		{name: "PG 19 second item", version: query.PostgresV19, cy: 1, want: "autovacuum_scores", wantSend: true},
		{name: "PG 19 out of range", version: query.PostgresV19, cy: 5, want: "tables", wantSend: true},
		{name: "PG 18 disabled second item", version: query.PostgresV18, cy: 1, want: "activity"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			g := &gocui.Gui{}

			_, err := g.SetView("sysstat", 0, 0, 20, 20)
			require.Equal(t, gocui.ErrUnknownView, err)

			mv, err := g.SetView("menu", 0, 5, 72, 20)
			require.Equal(t, gocui.ErrUnknownView, err)
			require.NoError(t, mv.SetCursor(0, tc.cy))

			app := &app{config: newConfig(), ui: g}
			app.config.viewCh = make(chan view.View, 1)
			app.config.view = app.config.views["activity"]
			app.config.menu = selectMenuStyle(menuTables, tc.version)
			app.postgresProps.VersionNum = tc.version
			original := app.config.view

			assert.NoError(t, menuSelect(app)(g, mv))

			select {
			case got := <-app.config.viewCh:
				assert.True(t, tc.wantSend, "disabled menu item sent a view change")
				assert.Equal(t, tc.want, got.Name)
			default:
				assert.False(t, tc.wantSend, "enabled menu item did not send a view change")
			}

			assert.Equal(t, tc.want, app.config.view.Name)
			if !tc.wantSend {
				assert.Equal(t, original, app.config.view)
			}
			assert.Equal(t, menuNone, app.config.menu.menuType)
			_, err = g.View("menu")
			assert.Equal(t, gocui.ErrUnknownView, err, "selected menu must be closed")
		})
	}
}

// The refusal NOTICE and exactly-once rendering are not observable through printCmdline's g.Update
// seam in this unit test. They remain branch-review and manual SC-5/SC-7 checks.
