// 'pgcenter profile' - wait events profiler for Postgres queries

package profile

import (
	"database/sql"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"io"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"
)

// auxiliary struct for sorting map
type waitEvent struct {
	waitEventName  string
	waitEventValue float64
}

// A slice of wait_events that implements sort.Interface to sort by values
type waitEventsList []waitEvent

// TraceStat describes data retrieved from Postgres' pg_stat_activity view
type TraceStat struct {
	queryDurationSec sql.NullFloat64
	stateChangeTime  sql.NullString
	state            sql.NullString
	waitEntry        sql.NullString
	queryText        sql.NullString
}

// Config defines program's configuration options
type Config struct {
	Pid       int // PID of profiled backend
	Frequency time.Duration
	Strsize   int // Limit length for query string
}

const (
	query = "SELECT " +
		"extract(epoch from clock_timestamp() - query_start) AS query_duration, " +
		"date_trunc('milliseconds', state_change) AS state_change_time, " +
		"state AS state, wait_event_type ||'.'|| wait_event AS wait_entry, query " +
		"FROM pg_stat_activity WHERE pid = $1 /* pgcenter profile */`"
)

// RunMain is the main entry point for 'pgcenter profile' command
func RunMain(dbConfig *postgres.Config, config Config) error {
	// Connect to Postgres
	conn, err := postgres.Connect(dbConfig)
	if err != nil {
		return err
	}
	defer conn.Close()

	// In case of SIGINT stop program gracefully
	doQuit := make(chan os.Signal, 1)
	signal.Notify(doQuit, syscall.SIGINT, syscall.SIGTERM)

	return profileLoop(os.Stdout, conn, config, doQuit)
}

type stats struct {
	durations map[string]float64
	ratios    map[string]float64
}

func newStats() stats {
	return stats{
		durations: make(map[string]float64),
		ratios:    make(map[string]float64),
	}
}

// Main profiling loop
func profileLoop(w io.Writer, conn *postgres.DB, opts Config, doQuit chan os.Signal) error {
	prev, curr := TraceStat{}, TraceStat{}
	startup := true
	s := newStats()

	fmt.Printf("LOG: Profiling process %d with %s sampling\n", opts.Pid, opts.Frequency)

	t := time.NewTicker(opts.Frequency)

	for {
		row := conn.QueryRow(query, opts.Pid)
		err := row.Scan(&curr.queryDurationSec,
			&curr.stateChangeTime,
			&curr.state,
			&curr.waitEntry,
			&curr.queryText,
		)

		if err != nil && err == sql.ErrNoRows {
			// print collected stats before exit
			err := printStat(w, s)
			if err != nil {
				return err
			}
			fmt.Printf("LOG: Process with pid %d doesn't exist (%s)\n", opts.Pid, err)
			fmt.Printf("LOG: Stop profiling\n")
			return nil
		} else if err != nil {
			return err
		}

		// Start collecting stats immediately if query is executing, otherwise waiting when query starts
		if startup {
			if curr.state.String == "active" {
				err := printHeader(w, curr, opts.Strsize)
				if err != nil {
					return err
				}
				s = countWaitings(s, curr, prev)
				startup = false
				prev = curr
				continue
			} else { /* waiting until backend becomes active */
				prev = curr
				startup = false
				time.Sleep(2 * time.Millisecond)
				continue
			}
		}

		// Backend's state is changed, it means query is started of finished
		if curr.stateChangeTime != prev.stateChangeTime {
			// transition to active state -- query started -- reset stats and print header with query text
			if curr.state.String == "active" {
				s = resetCounters(s)
				err := printHeader(w, curr, opts.Strsize)
				if err != nil {
					return err
				}
			}
			// transition from active state -- query finished -- print collected stats and reset it
			if prev.state.String == "active" {
				err := printStat(w, s)
				if err != nil {
					return err
				}
				s = resetCounters(s)
			}
		} else {
			// otherwise just count stats and sleep
			s = countWaitings(s, curr, prev)
			time.Sleep(opts.Frequency)
		}

		// copy current stats snapshot to previous
		prev = curr

		select {
		case <-t.C:
			continue
		case <-doQuit:
			t.Stop()
			return fmt.Errorf("got interrupt")
		}
	}
}

// Count wait events durations and percent rations
func countWaitings(s stats, curr TraceStat, prev TraceStat) stats {
	/* calculate durations for collected wait events */
	if curr.waitEntry.String == "" {
		s.durations["Running"] = s.durations["Running"] + (curr.queryDurationSec.Float64 - prev.queryDurationSec.Float64)
	} else {
		s.durations[curr.waitEntry.String] = s.durations[curr.waitEntry.String] + (curr.queryDurationSec.Float64 - prev.queryDurationSec.Float64)
	}

	/* calculate percents */
	for k, v := range s.durations {
		s.ratios[k] = (100 * v) / curr.queryDurationSec.Float64
	}

	return s
}

// Reset stats counters -- delete all entries from the maps
func resetCounters(s stats) stats {
	for k := range s.durations {
		delete(s.durations, k)
	}
	for k := range s.ratios {
		delete(s.ratios, k)
	}
	return s
}

// Print stats header
func printHeader(w io.Writer, curr TraceStat, strsize int) error {
	var q string
	if len(curr.queryText.String) > strsize {
		q = curr.queryText.String[:strsize]
	} else {
		q = curr.queryText.String
	}

	tmpl := `------ ------------ -----------------------------
%% time      seconds wait_event                     query: %s
------ ------------ -----------------------------
`

	_, err := fmt.Fprintf(w, tmpl, q)
	if err != nil {
		return err
	}

	return nil
}

// Print collected stats: wait events durations and percent ratios
func printStat(w io.Writer, s stats) error {
	if len(s.durations) == 0 {
		return nil
	} // nothing to do

	var totalPct, totalTime float64
	p := make(waitEventsList, len(s.durations))
	i := 0

	for k, v := range s.durations {
		p[i] = waitEvent{k, v}
		i++
	}

	// Sort wait events by percent ratios
	sort.Sort(p)

	// Print stats and calculating totals
	for _, e := range p {
		_, err := fmt.Fprintf(w, "%-*.2f %*.6f %s\n", 6, s.ratios[e.waitEventName], 12, e.waitEventValue, e.waitEventName)
		if err != nil {
			return err
		}
		totalPct += s.ratios[e.waitEventName]
		totalTime += e.waitEventValue
	}

	// Print totals
	_, err := fmt.Fprintf(w, "------ ------------ -----------------------------\n")
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "%-*.2f %*.6f\n", 6, totalPct, 12, totalTime)
	if err != nil {
		return err
	}

	return nil
}

// Custom methods for sorting wait events in the maps
func (p waitEventsList) Len() int           { return len(p) }
func (p waitEventsList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p waitEventsList) Less(i, j int) bool { return p[i].waitEventValue > p[j].waitEventValue }
