// 'pgcenter profile' - wait events profiler for Postgres queries

package profile

//import (
//	"database/sql"
//	"fmt"
//	"github.com/lesovsky/pgcenter/lib/utils"
//	"os"
//	"os/signal"
//	"sort"
//	"syscall"
//	"time"
//)
//
//// auxiliary struct for sorting map
//type waitEvent struct {
//	waitEventName  string
//	waitEventValue float64
//}
//
//// A slice of wait_events that implements sort.Interface to sort by values
//type waitEventsList []waitEvent
//
//// TraceStat describes data retrieved from Postgres' pg_stat_activity view
//type TraceStat struct {
//	queryDurationSec sql.NullFloat64
//	stateChangeTime  sql.NullString
//	state            sql.NullString
//	waitEntry        sql.NullString
//	queryText        sql.NullString
//}
//
//// TraceOptions defines program's configuration options
//type TraceOptions struct {
//	Pid      int           // PID of profiled backend
//	Interval time.Duration // Profiling interval
//	Strsize  int           // Limit length for query string
//}
//
//var (
//	query = `SELECT
//				extract(epoch from clock_timestamp() - query_start) AS query_duration,
//				date_trunc('milliseconds', state_change) AS state_change_time,
//				state AS state,
//				wait_event_type ||'.'|| wait_event AS wait_entry,
//				query
//			FROM pg_stat_activity WHERE pid = $1 /* pgcenter profile */`
//
//	waitEventDurations = make(map[string]float64) // wait events and its durations
//	waitEventPercents  = make(map[string]float64) // wait events and its percent ratios
//
//	signalChan = make(chan os.Signal, 1)
//	exitChan   = make(chan int)
//)
//
//// RunMain is the main entry point for 'pgcenter profile' command
//func RunMain(args []string, conninfo utils.Conninfo, opts TraceOptions) {
//	signal.Notify(signalChan, syscall.SIGINT)
//
//	// Handle extra arguments passed
//	utils.HandleExtraArgs(args, &conninfo)
//
//	// Connect to Postgres
//	conn, err := utils.CreateConn(&conninfo)
//	if err != nil {
//		fmt.Printf("ERROR: %s\n", err.Error())
//		return
//	}
//	defer conn.Close()
//
//	go profileLoop(conn, opts)
//	go checkSignal()
//
//	<-exitChan
//}
//
//// Main profiling loop
//func profileLoop(conn *sql.DB, opts TraceOptions) {
//	prev, curr := TraceStat{}, TraceStat{}
//	startup := true
//
//	fmt.Printf("LOG: Profiling process %d with %s sampling\n", opts.Pid, opts.Interval)
//
//	for {
//		row := conn.QueryRow(query, opts.Pid)
//		err := row.Scan(&curr.queryDurationSec,
//			&curr.stateChangeTime,
//			&curr.state,
//			&curr.waitEntry,
//			&curr.queryText)
//
//		if err != nil && err == sql.ErrNoRows {
//			// print collected stats before exit
//			printStat()
//			fmt.Printf("LOG: Process with pid %d doesn't exist (%s)\n", opts.Pid, err)
//			fmt.Printf("LOG: Stop profiling\n")
//			exitChan <- 1
//			break
//		} else if err != nil {
//			fmt.Printf("ERROR: failed to scan row: %s", err)
//			exitChan <- 1
//			break
//		}
//
//		// Start collecting stats immediately if query is executing, otherwise waiting when query starts
//		if startup {
//			if curr.state.String == "active" {
//				printHeader(curr, opts.Strsize)
//				countWaitings(curr, prev)
//				startup = false
//				prev = curr
//				continue
//			} else { /* waiting until backend becomes active */
//				prev = curr
//				startup = false
//				time.Sleep(2 * time.Millisecond)
//				continue
//			}
//		}
//
//		// Backend's state is cheanged, it means query is started of finished
//		if curr.stateChangeTime != prev.stateChangeTime {
//			// transition to active state -- query started -- reset stats and print header with query text
//			if curr.state.String == "active" {
//				resetCounters()
//				printHeader(curr, opts.Strsize)
//			}
//			// transition from active state -- query finished -- print collected stats and reset it
//			if prev.state.String == "active" {
//				printStat()
//				resetCounters()
//			}
//		} else {
//			// otherwise just count stats and sleep
//			countWaitings(curr, prev)
//			time.Sleep(opts.Interval)
//		}
//
//		// copy current stats snapshot to previous
//		prev = curr
//	}
//}
//
//// Count wait events durations and percent rations
//func countWaitings(curr TraceStat, prev TraceStat) {
//	/* calculate durations for collected wait events */
//	if curr.waitEntry.String == "" {
//		waitEventDurations["Running"] = waitEventDurations["Running"] + (curr.queryDurationSec.Float64 - prev.queryDurationSec.Float64)
//	} else {
//		waitEventDurations[curr.waitEntry.String] = waitEventDurations[curr.waitEntry.String] + (curr.queryDurationSec.Float64 - prev.queryDurationSec.Float64)
//	}
//
//	/* calculate percents */
//	for k, v := range waitEventDurations {
//		waitEventPercents[k] = (100 * v) / curr.queryDurationSec.Float64
//	}
//}
//
//// Reset stats counters -- delete all entries from the maps
//func resetCounters() {
//	for k := range waitEventDurations {
//		delete(waitEventDurations, k)
//	}
//	for k := range waitEventPercents {
//		delete(waitEventPercents, k)
//	}
//}
//
//// Print stats header
//func printHeader(curr TraceStat, strsize int) {
//	var q string
//	if len(curr.queryText.String) > strsize {
//		q = curr.queryText.String[:strsize]
//	} else {
//		q = curr.queryText.String
//	}
//	fmt.Printf("------ ------------ -----------------------------\n")
//	fmt.Printf("%% time      seconds wait_event                     query: %s\n", q)
//	fmt.Printf("------ ------------ -----------------------------\n")
//}
//
//// Print collected stats: wait events durations and percent ratios
//func printStat() {
//	if len(waitEventDurations) == 0 {
//		return
//	} // nothing to do
//
//	var totalPct, totalTime float64
//	p := make(waitEventsList, len(waitEventDurations))
//	i := 0
//
//	for k, v := range waitEventDurations {
//		p[i] = waitEvent{k, v}
//		i++
//	}
//
//	// Sort wait events by percent ratios
//	sort.Sort(sort.Reverse(p))
//
//	// Print stats and calculating totals
//	for _, e := range p {
//		fmt.Printf("%-*.2f %*.6f %s\n", 6, waitEventPercents[e.waitEventName], 12, e.waitEventValue, e.waitEventName)
//		totalPct += waitEventPercents[e.waitEventName]
//		totalTime += e.waitEventValue
//	}
//
//	// Print totals
//	fmt.Printf("------ ------------ -----------------------------\n")
//	fmt.Printf("%-*.2f %*.6f\n", 6, totalPct, 12, totalTime)
//}
//
//// Check for SIGINT, dump stats if catched and exit
//func checkSignal() {
//	for {
//		sig := <-signalChan
//		switch sig {
//		case syscall.SIGINT:
//			exitChan <- 1
//			return
//		default:
//			fmt.Println("LOG: unknown signal, ignore")
//		}
//	}
//}
//
//// Custom methods for sorting wait events in the maps
//func (p waitEventsList) Len() int           { return len(p) }
//func (p waitEventsList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
//func (p waitEventsList) Less(i, j int) bool { return p[i].waitEventValue < p[j].waitEventValue }
