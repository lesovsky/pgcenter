// Error rate facility used with gocui.MainLoop to check errors rate in case if main loop will fail too often.
// In case of seldom error or in case when gocui.MainLoop stopped manually (when running 3-rd party programs) this
// seldom errors can be ignored and gocui.MainLoop can be restarted. But if something really goes wrong inside gocui,
// error rate facility should catch this situation and stop program execution.

package top

import (
	"fmt"
	"time"
)

// Describe
type ErrorRate struct {
	t_curr    time.Time     // time of the latest error
	t_prev    time.Time     // time when previous error occured
	t_elapsed time.Duration // interval between two last errors
	err_cnt   int           // errors counter
}

// Check the number of errors within specified interval
func (e *ErrorRate) Check(errInterval time.Duration, errMaxcount int) error {
	e.t_curr = time.Now()
	e.t_elapsed = e.t_curr.Sub(e.t_prev)
	if e.t_elapsed > errInterval { // interval between errors too long, reset counter
		e.t_prev = e.t_curr
		e.err_cnt = 0
	} else { // otherwise increment counter
		e.err_cnt++
		if e.err_cnt > errMaxcount { // if errors limit is reached, exit with error
			return fmt.Errorf("too many errors")
		}
	}
	return nil
}
