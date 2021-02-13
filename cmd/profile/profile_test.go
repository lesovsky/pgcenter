package profile

import (
	"github.com/lesovsky/pgcenter/profile"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func Test_validate(t *testing.T) {
	testcases := []struct {
		valid bool
		cfg   profile.Config
	}{
		{valid: true, cfg: profile.Config{Frequency: 50 * time.Millisecond}},
		{valid: false, cfg: profile.Config{Frequency: time.Millisecond - 1}},
		{valid: false, cfg: profile.Config{Frequency: time.Second + 1}},
	}

	for _, tc := range testcases {
		err := validate(tc.cfg)
		if tc.valid {
			assert.NoError(t, err)
		} else {
			assert.Error(t, err)
		}
	}
}
