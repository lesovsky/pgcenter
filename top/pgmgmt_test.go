package top

import (
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_doReload(t *testing.T) {
	testcases := []struct {
		buf string
	}{
		{buf: dialogPrompts[dialogPgReload] + "y"},
		{buf: dialogPrompts[dialogPgReload] + "n"},
		{buf: dialogPrompts[dialogPgReload] + "invalid"},
	}

	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	for _, tc := range testcases {
		assert.NoError(t, doReload(nil, tc.buf, conn))
	}

	// Even with closed conn, error is suppressed.
	conn.Close()
	assert.NoError(t, doReload(nil, testcases[0].buf, conn))
}

func Test_resetStat(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	fn := resetStat(conn)
	assert.NoError(t, fn(nil, nil))

	conn.Close()
	assert.NoError(t, fn(nil, nil))
}
