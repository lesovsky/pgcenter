// 'pgcenter profile' - wait events profiler for Postgres queries

package profile

import (
	"fmt"
	"github.com/jackc/pgx/v4"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"io"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"
)

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

// profileStat describes stat snapshot retrieved from Postgres' pg_stat_activity view.
type profileStat struct {
	queryDurationSec float64 // number of seconds query is running at the moment of snapshotting.
	changeStateTime  string  // value of pg_stat_activity.change_state tells about when query has been finished (or new one started)
	state            string  // backend state
	waitEntry        string  // wait_event_type/wait_event
	queryText        string  // query executed by backend
}

// Config defines program's configuration options.
type Config struct {
	Pid       int // PID of profiled backend
	Frequency time.Duration
	Strsize   int // Limit length for query string
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
	durations map[string]float64
	ratios    map[string]float64
}

// newStatsStore creates new stats store.
func newStatsStore() stats {
	return stats{
		durations: make(map[string]float64),
		ratios:    make(map[string]float64),
	}
}

// profileLoop profiles and prints profiling results.
func profileLoop(w io.Writer, conn *postgres.DB, cfg Config, doQuit chan os.Signal) error {
	var prev profileStat
	s := newStatsStore()

	_, err := fmt.Fprintf(w, "LOG: Profiling process %d with %s sampling\n", cfg.Pid, cfg.Frequency)
	if err != nil {
		return err
	}

	t := time.NewTicker(cfg.Frequency)

	for {
		curr, profileErr := getProfileSnapshot(conn, cfg.Pid)
		if profileErr != nil && profileErr == pgx.ErrNoRows {
			// print collected stats before exit
			err := printStat(w, s)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintf(w, "LOG: Stop profiling, process with pid %d doesn't exist (%s)\n", cfg.Pid, profileErr.Error())
			if err != nil {
				return err
			}

			return nil
		} else if profileErr != nil {
			return profileErr
		}

		switch {
		case prev.state != "active" && curr.state == "active":
			// !active -> active - a query has been started - begin to count stats.
			err := printHeader(w, curr, cfg.Strsize)
			if err != nil {
				return err
			}
			s = countWaitings(s, curr, profileStat{})
			prev = curr
		case prev.state == "active" && curr.state == "active" && prev.changeStateTime == curr.changeStateTime:
			// active -> active - query continues executing - continue to count stats.
			s = countWaitings(s, curr, prev)
			prev = curr
		case prev.state == "active" && curr.state == "active" && prev.changeStateTime != curr.changeStateTime:
			// active -> active (new) - a new query has been started - print stat for previous query, count new stats.
			err := printStat(w, s)
			if err != nil {
				return err
			}
			s = resetCounters(s)
			s = countWaitings(s, curr, profileStat{})
			prev = profileStat{}
		case prev.state == "active" && curr.state != "active":
			// active -> idle - query has been finished, but no new query started - print stat, waiting for new query.
			err := printStat(w, s)
			if err != nil {
				return err
			}
			s = resetCounters(s)
			prev = profileStat{}
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

// getProfileSnapshot get necessary activity snapshot from Postgres.
func getProfileSnapshot(conn *postgres.DB, pid int) (profileStat, error) {
	query := "SELECT " +
		"extract(epoch from clock_timestamp() - query_start) AS query_duration, " +
		"date_trunc('milliseconds', state_change) AS state_change_time, " +
		"state AS state, " +
		"coalesce(wait_event_type ||'.'|| wait_event, '') AS wait_entry, query " +
		"FROM pg_stat_activity WHERE pid = $1 /* pgcenter profile */"

	var s profileStat

	row := conn.QueryRow(query, pid)
	err := row.Scan(&s.queryDurationSec,
		&s.changeStateTime,
		&s.state,
		&s.waitEntry,
		&s.queryText,
	)

	return s, err
}

// countWaitings counts wait events durations and its percent rations accordingly to total query time.
func countWaitings(s stats, curr profileStat, prev profileStat) stats {
	// calculate durations
	if curr.waitEntry == "" {
		s.durations["Running"] = s.durations["Running"] + (curr.queryDurationSec - prev.queryDurationSec)
	} else {
		s.durations[curr.waitEntry] = s.durations[curr.waitEntry] + (curr.queryDurationSec - prev.queryDurationSec)
	}

	// calculate ratios
	for k, v := range s.durations {
		s.ratios[k] = (100 * v) / curr.queryDurationSec
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

// printHeader prints profile header.
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

// printStat prints collected wait events durations and percent ratios.
func printStat(w io.Writer, s stats) error {
	if len(s.durations) == 0 {
		return nil
	} // nothing to do

	var totalPct, totalTime float64
	p := make(waitEvents, len(s.durations))
	i := 0

	for k, v := range s.durations {
		p[i] = waitEvent{k, v}
		i++
	}

	// Sort wait events by percent ratios.
	sort.Sort(p)

	// Print stats and calculating totals.
	for _, e := range p {
		_, err := fmt.Fprintf(w, "%*.2f %*.6f %s\n", 6, s.ratios[e.waitEventName], 12, e.waitEventValue, e.waitEventName)
		if err != nil {
			return err
		}
		totalPct += s.ratios[e.waitEventName]
		totalTime += e.waitEventValue
	}

	// Print totals.
	_, err := fmt.Fprintf(w, "------ ------------ -----------------------------\n")
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "%*.2f %*.6f\n", 6, totalPct, 12, totalTime)
	if err != nil {
		return err
	}

	return nil
}

// truncateQuery truncates string if it's longer than limit and returns truncated copy of string.
func truncateQuery(q string, limit int) string {
	if len(q) > limit {
		return q[:limit]
	}
	return q
}
