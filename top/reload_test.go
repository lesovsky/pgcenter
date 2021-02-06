package top

import (
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_doReload(t *testing.T) {
	prompt := dialogPrompts[dialogPgReload]
	testcases := []struct {
		answer string
		want   string
	}{
		{answer: prompt + "y", want: "Reload: successful"},
		{answer: prompt + "n", want: "Reload: do nothing, canceled"},
		{answer: prompt + "q", want: "Reload: do nothing, invalid input"},
	}

	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	for _, tc := range testcases {
		assert.Equal(t, tc.want, doReload(tc.answer, conn))
	}

	// Test with closed conn
	conn.Close()
	assert.Equal(t, "Reload: failed, conn closed", doReload(testcases[0].answer, conn))
}
