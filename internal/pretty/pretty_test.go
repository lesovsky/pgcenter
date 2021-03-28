package pretty

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSize(t *testing.T) {
	testcases := []struct {
		v    float64
		want string
	}{
		{v: 0, want: "0"},
		{v: 512, want: "512B"},
		{v: 9425, want: "9.2K"},
		{v: 425681, want: "415.7K"},
		{v: 512548751, want: "488.8M"},
		{v: 512254851486, want: "477.1G"},
		{v: 512254851486475, want: "465.9T"},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, Size(tc.v))
	}
}
