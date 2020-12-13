package math

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMin(t *testing.T) {
	assert.Equal(t, 10, Min(15, 10))
	assert.Equal(t, 10, Min(10, 15))
	assert.Equal(t, 15, Min(15, 15))
}

func TestMax(t *testing.T) {
	assert.Equal(t, 15, Max(15, 10))
	assert.Equal(t, 15, Max(10, 15))
	assert.Equal(t, 15, Max(15, 15))
}
