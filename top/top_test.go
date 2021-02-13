package top

import (
	"github.com/jroimartin/gocui"
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

func Test_app_quit(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	app := newApp(conn, newConfig())
	assert.NotNil(t, app)

	assert.NoError(t, app.setup())

	fn := app.quit()

	ui, err := gocui.NewGui(gocui.OutputNormal)
	assert.NoError(t, err)
	app.ui = ui

	assert.Equal(t, gocui.ErrQuit, fn(app.ui, nil))
}
