// 'pgcenter profile' - wait events profiler for Postgres queries

package profile

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/stat"
	"io"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"syscall"
	"time"
)

const (
	inclusiveQuery = "SELECT pid, " +
		"extract(epoch from clock_timestamp() - query_start) AS query_duration, " +
		"date_trunc('milliseconds', state_change) AS state_change_time, " +
		"state AS state, " +
		"coalesce(wait_event_type ||'.'|| wait_event, '') AS wait_entry, query " +
		"FROM pg_stat_activity WHERE pid = %d OR leader_pid = %d /* pgcenter profile */"

	exclusiveQuery = "SELECT pid, " +
		"extract(epoch from clock_timestamp() - query_start) AS query_duration, " +
		"date_trunc('milliseconds', state_change) AS state_change_time, " +
		"state AS state, " +
		"coalesce(wait_event_type ||'.'|| wait_event, '') AS wait_entry, query " +
		"FROM pg_stat_activity WHERE pid = %d /* pgcenter profile */"
)

// Config defines program's configuration options.
type Config struct {
	Pid       int           // Process ID of profiled backend
	Frequency time.Duration // Interval used for collecting activity statistics
	Strsize   int           // Limit length for query string
	NoWorkers bool          // Don't collect statistics about children parallel workers
}

// RunMain is the main entry point for 'pgcenter profile' command
func RunMain(dbConfig postgres.Config, config Config) error {
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

// stats defines local statistics storage for profiled query.
type stats struct {
	real        float64
	accumulated float64
	durations   map[string]float64
	ratios      map[string]float64
}

// newStatsStore creates new stats store.
func newStatsStore() stats {
	return stats{
		durations: make(map[string]float64),
		ratios:    make(map[string]float64),
	}
}

// resetStatsStore deletes all entries from the stats maps counters
func resetStatsStore(s stats) stats {
	for k := range s.durations {
		delete(s.durations, k)
	}
	for k := range s.ratios {
		delete(s.ratios, k)
	}
	return s
}

// profileStat describes snapshot of activity statistics about single profiled process.
type profileStat struct {
	queryDurationSec float64 // number of seconds query is running at the moment of snapshot.
	changeStateTime  string  // value of pg_stat_activity.change_state tells about when query has been finished (or new one started)
	state            string  // backend state
	waitEntry        string  // wait_event_type/wait_event
	queryText        string  // query executed by backend
}

// profileLoop profiles target Process ID in a loop and prints profiling results.
func profileLoop(w io.Writer, conn *postgres.DB, cfg Config, doQuit chan os.Signal) error {
	prev := make(map[int]profileStat)
	s := newStatsStore()

	_, err := fmt.Fprintf(w, "LOG: Profiling process %d with %s sampling\n", cfg.Pid, cfg.Frequency)
	if err != nil {
		return err
	}

	t := time.NewTicker(cfg.Frequency)

	pid := cfg.Pid

	// Get Postgres properties, depending on Postgres version select proper version of query.
	props, err := stat.GetPostgresProperties(conn)
	if err != nil {
		return err
	}

	query := selectQuery(pid, cfg.NoWorkers, props.VersionNum)

	for {
		// Get activity snapshot
		res, err := stat.NewPGresultQuery(conn, query)
		if err != nil {
			return err
		}

		// No rows returned means profiled process quits. No reason to continue, print stats and return.
		if res.Nrows == 0 {
			_, err = fmt.Fprintf(w, "LOG: Stop profiling, no process with pid %d\n", pid)
			if err != nil {
				return err
			}

			return nil
		}

		// Extract per-process stats from activity snapshot.
		curr := parseActivitySnapshot(res)

		// Compare previous and current activity snapshots and analyze target process state transition.
		switch {
		case prev[pid].state != "active" && curr[pid].state == "active":
			// !active -> active - a query has been started - begin to count stats.
			err := printHeader(w, curr[pid], cfg.Strsize)
			if err != nil {
				return err
			}
			s = countWaitEvents(s, pid, curr, map[int]profileStat{})
			prev = curr
		case prev[pid].state == "active" && curr[pid].state == "active" && prev[pid].changeStateTime == curr[pid].changeStateTime:
			// active -> active - query continues executing - continue to count stats.
			s = countWaitEvents(s, pid, curr, prev)
			prev = curr
		case prev[pid].state == "active" && curr[pid].state == "active" && prev[pid].changeStateTime != curr[pid].changeStateTime:
			// active -> active (new) - a new query has been started - print stat for previous query, count new stats.
			err := printStat(w, s)
			if err != nil {
				return err
			}
			s = resetStatsStore(s)
			s = countWaitEvents(s, pid, curr, map[int]profileStat{})
			prev = map[int]profileStat{}
		case prev[pid].state == "active" && curr[pid].state != "active":
			// active -> idle - query has been finished, but no new query started - print stat, waiting for new query.
			err := printStat(w, s)
			if err != nil {
				return err
			}
			s = resetStatsStore(s)
			prev = map[int]profileStat{}
		}

		// Wait ticker ticks.
		select {
		case <-t.C:
			continue
		case <-doQuit:
			t.Stop()
			err := printStat(w, s)
			if err != nil {
				return err
			}
			return fmt.Errorf("got interrupt")
		}
	}
}

// parseActivitySnapshot parses PGresult and returns per-process profile statistics.
func parseActivitySnapshot(res stat.PGresult) map[int]profileStat {
	stat := make(map[int]profileStat)

	for _, row := range res.Values {
		var pid int
		var s profileStat

		for i, colname := range res.Cols {
			// Skip empty (NULL) values.
			if !row[i].Valid {
				continue
			}

			switch colname {
			case "pid":
				v, err := strconv.Atoi(row[i].String)
				if err != nil {
					continue
				}
				pid = v
			case "query_duration":
				v, err := strconv.ParseFloat(row[i].String, 64)
				if err != nil {
					continue
				}
				s.queryDurationSec = v
			case "state_change_time":
				s.changeStateTime = row[i].String
			case "state":
				s.state = row[i].String
			case "wait_entry":
				s.waitEntry = row[i].String
			case "query":
				s.queryText = row[i].String
			}
		}

		stat[pid] = s
	}

	return stat
}

// countWaitEvents counts wait events durations and its percent rations accordingly to total query time.
func countWaitEvents(s stats, targetPid int, curr map[int]profileStat, prev map[int]profileStat) stats {
	// Walk through current and previous activity snapshots and calculate durations
	for k, vCurr := range curr {
		if vPrev, ok := prev[k]; ok {
			// found in prev
			delta := vCurr.queryDurationSec - vPrev.queryDurationSec
			s.accumulated += delta

			if vCurr.waitEntry == "" {
				s.durations["Running"] += delta
			} else {
				s.durations[vCurr.waitEntry] += delta
			}
		} else {
			// new, not found in prev
			s.accumulated += vCurr.queryDurationSec
			if vCurr.waitEntry == "" {
				s.durations["Running"] += vCurr.queryDurationSec
			} else {
				s.durations[vCurr.waitEntry] += vCurr.queryDurationSec
			}
		}
	}

	// Update target PID execution duration
	s.real = curr[targetPid].queryDurationSec

	// Calculate ratios of wait_events accordingly to total time (including background workers)
	for k, v := range s.durations {
		s.ratios[k] = (100 * v) / s.accumulated
	}

	return s
}

// printHeader prints report header.
func printHeader(w io.Writer, curr profileStat, strsize int) error {
	q := truncateQuery(curr.queryText, strsize)

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

// waitEvent defines particular wait event and how many times it is occurred.
type waitEvent struct {
	waitEventName  string
	waitEventValue float64
}

// waitEvents defines slice of waitEvent.
type waitEvents []waitEvent

// Implement sort methods for waitEvents.
func (p waitEvents) Len() int           { return len(p) }
func (p waitEvents) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p waitEvents) Less(i, j int) bool { return p[i].waitEventValue > p[j].waitEventValue }

// printStat prints report body with collected wait events durations and percent ratios.
func printStat(w io.Writer, s stats) error {
	if len(s.durations) == 0 {
		return nil
	} // nothing to do

	// Organize collected wait_events into slice for further sorting.
	var totalPct float64
	p := make(waitEvents, len(s.durations))
	i := 0

	for k, v := range s.durations {
		p[i] = waitEvent{k, v}
		i++
	}

	// Sort wait_events by percent ratios.
	sort.Sort(p)

	// Print stats and calculate totals.
	for _, e := range p {
		_, err := fmt.Fprintf(w, "%*.2f %*.6f %s\n", 6, s.ratios[e.waitEventName], 12, e.waitEventValue, e.waitEventName)
		if err != nil {
			return err
		}
		totalPct += s.ratios[e.waitEventName]
	}

	// Print totals.
	_, err := fmt.Fprintf(w, "------ ------------ -----------------------------\n")
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "%*.2f %*.6f\n       %*.6f including workers\n", 6, totalPct, 12, s.real, 12, s.accumulated)
	if err != nil {
		return err
	}

	return nil
}

// truncateQuery truncates string if it's longer than limit and returns truncated copy of string.
func truncateQuery(s string, limit int) string {
	if len(s) > limit {
		return s[:limit]
	}
	return s
}

// selectQuery defines query used for profiling. Possible two kind of queries:
// - exclusive - selects target PID only, with no statistics about background (parallel) workers.
// - inclusive - selects statistics about target PID and background (parallel) workers (using 'leader_pid' available since Postgres 13)
func selectQuery(pid int, exclusive bool, version int) string {
	var query string

	if exclusive || version < 130000 {
		query = fmt.Sprintf(exclusiveQuery, pid)
	} else {
		query = fmt.Sprintf(inclusiveQuery, pid, pid)
	}

	return query
}
