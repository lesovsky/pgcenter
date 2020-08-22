// Stuff related to 'load average' stats

package stat

import (
	"bufio"
	"bytes"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"io/ioutil"
	"strconv"
	"strings"
)

const (
	procLoadAvgFile    = "/proc/loadavg"
	pgProcLoadAvgQuery = "SELECT min1, min5, min15 FROM pgcenter.sys_proc_loadavg"
)

// LoadAvg is the container for 'load average' stats
type LoadAvg struct {
	One     float64
	Five    float64
	Fifteen float64
}

// Read stats into container
func (la *LoadAvg) Read(db *postgres.DB, pgcAvail bool) {
	if db.Local {
		la.ReadLocal()
	} else if pgcAvail {
		la.ReadRemote(db)
	}
}

// ReadLocal reads stat from local 'procfs' filesystem
func (la *LoadAvg) ReadLocal() {
	content, err := ioutil.ReadFile(procLoadAvgFile)
	if err != nil {
		return
	}

	reader := bufio.NewReader(bytes.NewBuffer(content))
	line, _, err := reader.ReadLine()
	if err != nil {
		return
	}

	fields := strings.Fields(string(line))

	/* ignore errors, if something goes wrong - just print zeroes */
	la.One, _ = strconv.ParseFloat(fields[0], 64)
	la.Five, _ = strconv.ParseFloat(fields[1], 64)
	la.Fifteen, _ = strconv.ParseFloat(fields[2], 64)
}

// ReadRemote reads stats from remote Postgres instance
func (la *LoadAvg) ReadRemote(db *postgres.DB) {
	db.QueryRow(pgProcLoadAvgQuery).Scan(&la.One, &la.Five, &la.Fifteen)
}
