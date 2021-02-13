package config

import (
	"github.com/lesovsky/pgcenter/config"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_options_validate(t *testing.T) {
	testcases := []struct {
		in    options
		valid bool
	}{
		{in: options{install: true, uninstall: false}, valid: true},
		{in: options{install: false, uninstall: true}, valid: true},
		{in: options{install: false, uninstall: false}, valid: false},
		{in: options{install: true, uninstall: true}, valid: false},
	}

	for _, tc := range testcases {
		got := tc.in.validate()
		if tc.valid {
			assert.NoError(t, got)
		} else {
			assert.Error(t, got)
		}
	}
}

func Test_options_mode(t *testing.T) {
	testcases := []struct {
		in   options
		want int
	}{
		{in: options{install: true}, want: config.Install},
		{in: options{uninstall: true}, want: config.Uninstall},
		{in: options{}, want: -1},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, tc.in.mode())
	}
}
