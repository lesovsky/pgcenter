package top

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
	"time"
)

func Test_killSingle(t *testing.T) {
	victim, err := postgres.NewTestConnect()
	assert.NoError(t, err)
	defer victim.Close()

	var pid string
	err = victim.QueryRow("select pg_backend_pid()::text").Scan(&pid)

	testcases := []struct {
		pid   string
		mode  string
		input string
		valid bool
	}{
		{pid: pid, mode: "cancel", input: dialogPrompts[dialogCancelQuery], valid: true},
		{pid: pid, mode: "terminate", input: dialogPrompts[dialogTerminateBackend], valid: true},
		{pid: pid, mode: "terminate", input: dialogPrompts[dialogTerminateBackend], valid: true}, // attempt to terminate the previously terminated pid should not fail
		{pid: "invalid", mode: "terminate", input: dialogPrompts[dialogTerminateBackend], valid: false},
		{pid: pid, mode: "invalid", valid: false},
	}

	db, err := postgres.NewTestConnect()
	assert.NoError(t, err)
	defer db.Close()

	for _, tc := range testcases {
		if tc.valid {
			assert.NoError(t, killSingle(db, tc.mode, tc.input+tc.pid))
		} else {
			assert.Error(t, killSingle(db, tc.mode, tc.input+tc.pid))
		}
	}
}

func Test_killGroup(t *testing.T) {
	testcases := []struct {
		mode string
		mask int
	}{
		{mode: "cancel", mask: groupIdle},
		{mode: "cancel", mask: groupActive},
		{mode: "cancel", mask: groupIdleXact},
		{mode: "terminate", mask: groupIdle},
		{mode: "terminate", mask: groupActive},
		{mode: "terminate", mask: groupIdleXact},
	}

	db, err := postgres.NewTestConnect()
	assert.NoError(t, err)
	defer db.Close()

	app := &app{
		config: newConfig(),
		db:     db,
	}

	// set default values
	app.config.view = app.config.views["activity"]
	app.config.queryOptions.QueryAgeThresh = "00:00:00"

	var wg sync.WaitGroup

	for i, tc := range testcases {
		t.Run(fmt.Sprintln(i), func(t *testing.T) {

			app.config.procMask |= tc.mask // assign mask

			ch := make(chan struct{})

			wg.Add(1)
			go func() {
				victim, err := postgres.NewTestConnect()
				assert.NoError(t, err)

				switch app.config.procMask {
				case groupIdle:
					_, _ = victim.Exec("select 1")
				case groupActive:
					_, _ = victim.Exec("select pg_sleep(10)")
				case groupIdleXact:
					_, _ = victim.Exec("begin")
					time.Sleep(2 * time.Second)
				case groupWaiting, groupOthers:
					// don't know how to emulate
				}

				<-ch
				if tc.mode == "cancel" {
					victim.Close()
				}
				close(ch)
				wg.Done()
			}()

			time.Sleep(1 * time.Second) // make sure victim connection is established and started
			msg, err := killGroup(app, tc.mode)
			assert.NoError(t, err)
			assert.NotEqual(t, "", msg)
			fmt.Println(msg)
			ch <- struct{}{}
			app.config.procMask = 0 // reset mask
		})
		wg.Wait()
	}

	// run test with invalid input
	t.Run("invalid input", func(t *testing.T) {
		app.config.view = app.config.views["tables"]
		msg, err := killGroup(app, "cancel")
		assert.Equal(t, "Terminate or cancel backend allowed in pg_stat_activity.", msg)
		assert.NoError(t, err)

		app.config.view = app.config.views["activity"]
		app.config.procMask = 0
		msg, err = killGroup(app, "cancel")
		assert.Equal(t, "Do nothing. The mask is empty.", msg)
		assert.NoError(t, err)

		app.config.procMask = groupIdle
		msg, err = killGroup(app, "invalid")
		assert.Equal(t, "Do nothing. Unknown mode (not cancel, nor terminate).", msg)
		assert.NoError(t, err)
	})
}

func Test_setProcMask(t *testing.T) {
	testcases := []struct {
		buf  string
		want int
	}{
		{buf: dialogPrompts[dialogSetMask] + "", want: 0},
		{buf: dialogPrompts[dialogSetMask] + "i", want: groupIdle},
		{buf: dialogPrompts[dialogSetMask] + "ix", want: groupIdle + groupIdleXact},
		{buf: dialogPrompts[dialogSetMask] + "aw", want: groupWaiting + groupActive},
		{buf: dialogPrompts[dialogSetMask] + "iax", want: groupIdle + groupIdleXact + groupActive},
		{buf: dialogPrompts[dialogSetMask] + "aox", want: groupOthers + groupActive + groupIdleXact},
		{buf: dialogPrompts[dialogSetMask] + "wixa", want: groupIdle + groupIdleXact + groupActive + groupWaiting},
		{buf: dialogPrompts[dialogSetMask] + "woix", want: groupIdleXact + groupOthers + groupWaiting + groupIdle},
		{buf: dialogPrompts[dialogSetMask] + "iowax", want: groupIdle + groupIdleXact + groupActive + groupWaiting + groupOthers},
	}

	config := newConfig()

	for _, tc := range testcases {
		setProcMask(nil, tc.buf, config)
		assert.Equal(t, tc.want, config.procMask)
	}
}

func Test_showProcMask(t *testing.T) {
	testcases := []int{
		0,
		groupIdle,
		func() int { var m int; m |= groupIdle; m |= groupIdleXact; return m }(),
		func() int { var m int; m |= groupIdle; m |= groupIdleXact; m |= groupActive; return m }(),
		func() int {
			var m int
			m |= groupIdle
			m |= groupIdleXact
			m |= groupActive
			m |= groupWaiting
			return m
		}(),
		func() int {
			var m int
			m |= groupIdle
			m |= groupIdleXact
			m |= groupActive
			m |= groupWaiting
			m |= groupOthers
			return m
		}(),
	}

	for _, tc := range testcases {
		fn := showProcMask(tc)
		assert.NoError(t, fn(nil, nil))
	}
}
