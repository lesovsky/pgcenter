// Error rate facility used with gocui.MainLoop to check errors rate in case if main loop will fail too often.
// In case of seldom error or in case when gocui.MainLoop stopped manually (when running 3-rd party programs) this
// seldom errors can be ignored and gocui.MainLoop can be restarted. But if something really goes wrong inside gocui,
// error rate facility should catch this situation and stop program execution.

package top

import (
	"fmt"
	"time"
)

// ErrorRate describes details about how often errors occurs
type ErrorRate struct {
	timeCurr    time.Time     // time of the latest error
	timePrev    time.Time     // time when previous error occurred
	timeElapsed time.Duration // interval between two last errors
	errCnt      int           // errors counter
}

// Check the number of errors within specified interval
func (e *ErrorRate) Check(errInterval time.Duration, errMaxcount int) error {
	e.timeCurr = time.Now()
	e.timeElapsed = e.timeCurr.Sub(e.timePrev)
	if e.timeElapsed > errInterval { // interval between errors too long, reset counter
		e.timePrev = e.timeCurr
		e.errCnt = 0
	} else { // otherwise increment counter
		e.errCnt++
		if e.errCnt > errMaxcount { // if errors limit is reached, exit with error
			return fmt.Errorf("too many errors")
		}
	}
	return nil
}
