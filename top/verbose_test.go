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

	// Persistence acceptance criterion: verbose must survive a real view switch
	// (unlike scrollOffset, which viewSwitchHandler resets). Toggle it on, then
	// switch views and assert the active view still carries the flag.
	config.view = config.views["activity"]
	config.verbose = false
	wg.Add(1)
	go func() { <-config.viewCh; wg.Done() }() // drain the toggle push
	assert.NoError(t, toggleVerbose(app)(nil, nil))
	wg.Wait()

	wg.Add(1)
	go func() { <-config.viewCh; wg.Done() }() // drain the switch push
	viewSwitchHandler(config, "databases_general")
	wg.Wait()

	assert.True(t, config.view.Verbose, "verbose must persist across view switch")

	close(config.viewCh)
}
