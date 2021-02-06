package top

import (
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_resetStat(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	fn := resetStat(conn, true)
	assert.NoError(t, fn(nil, nil))

	fn = resetStat(conn, false)
	assert.NoError(t, fn(nil, nil))

	conn.Close()
	assert.NoError(t, fn(nil, nil))
}
