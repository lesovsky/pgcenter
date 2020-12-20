package top

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_selectMenuStyle(t *testing.T) {
	testcases := []struct {
		menu menuType
		want int
	}{
		{menu: menuNone, want: 0},
		{menu: menuPgss, want: 5},
		{menu: menuProgress, want: 3},
		{menu: menuConf, want: 4},
	}

	for _, tc := range testcases {
		got := selectMenuStyle(tc.menu)
		assert.Equal(t, tc.want, len(got.items))
	}
}
