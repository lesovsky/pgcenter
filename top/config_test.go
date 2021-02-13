package top

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_newConfig(t *testing.T) {
	c := newConfig()
	assert.Greater(t, len(c.views), 0)
}
