package top

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func Test_errorRate_check(t *testing.T) {
	testcases := []struct {
		erate errorRate
		want  int
		ok    bool
	}{
		{erate: errorRate{timePrev: time.Now().Add(time.Duration(-10) * time.Second), errCnt: 0}, ok: true, want: 1},
		{erate: errorRate{timePrev: time.Now().Add(time.Duration(-70) * time.Second), errCnt: 5}, ok: true, want: 0},
		{erate: errorRate{timePrev: time.Now().Add(time.Duration(-10) * time.Second), errCnt: 9}, ok: true, want: 10},
		{erate: errorRate{timePrev: time.Now().Add(time.Duration(-10) * time.Second), errCnt: 10}, ok: false, want: 0},
	}

	for _, tc := range testcases {
		if tc.ok {
			assert.NoError(t, tc.erate.check(time.Minute, 10))
			assert.Equal(t, tc.erate.errCnt, tc.want)
		} else {
			assert.Error(t, tc.erate.check(time.Minute, 10))
		}
	}
}
