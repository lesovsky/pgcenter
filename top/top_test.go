package top

import (
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_newApp(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	config := newConfig()
	assert.NotNil(t, newApp(conn, config))
	defer conn.Close()
}

func Test_app_setup(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	app := newApp(conn, newConfig())
	assert.NotNil(t, app)

	// before setup, all query texts are empty
	for _, v := range app.config.views {
		assert.Equal(t, "", v.Query)
	}

	assert.NoError(t, app.setup())

	// after setup, all query texts are created from templates
	for _, v := range app.config.views {
		assert.NotEqual(t, "", v.Query)
	}

	// test with closed Postgres connection.
	conn.Close()
	assert.Error(t, app.setup())
}

// This test hangs when executing on Github Actions due to hangs here:
//   github.com/nsf/termbox-go@v0.0.0-20180819125858-b66b20ab708e/api.go:122
//func Test_app_quit(t *testing.T) {
//	conn, err := postgres.NewTestConnect()
//	assert.NoError(t, err)
//
//	app := newApp(conn, newConfig())
//	assert.NotNil(t, app)
//	assert.NoError(t, app.setup())
//
//	ui, err := gocui.NewGui(gocui.OutputNormal)
//	assert.NoError(t, err)
//
//	app.ui = ui
//	fn := app.quit()
//
//	assert.Equal(t, gocui.ErrQuit, fn(app.ui, nil))
//}
