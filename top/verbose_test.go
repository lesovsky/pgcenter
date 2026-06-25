package top

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_toggleVerbose(t *testing.T) {
	testcases := []struct {
		verboseInitial bool
		expected       bool
	}{
		{verboseInitial: false, expected: true}, // off -> on
		{verboseInitial: true, expected: false}, // on -> off
	}

	config := newConfig()
	config.view = config.views["activity"]
	app := &app{config: config}

	wg := sync.WaitGroup{}

	for i, tc := range testcases {
		t.Run(fmt.Sprintln(i), func(t *testing.T) {
			config.verbose = tc.verboseInitial

			wg.Add(1)
			go func() {
				v := <-config.viewCh
				assert.Equal(t, tc.expected, v.Verbose)
				wg.Done()
			}()

			fn := toggleVerbose(app)
			assert.NoError(t, fn(nil, nil))
			wg.Wait()

			// config.verbose flips to the expected value.
			assert.Equal(t, tc.expected, config.verbose)
			// the active view carries the new flag.
			assert.Equal(t, tc.expected, config.view.Verbose)
			// every view in the map is updated, so the flag survives a screen switch.
			for _, v := range config.views {
				assert.Equal(t, tc.expected, v.Verbose)
			}
		})
	}

	close(config.viewCh)
}
