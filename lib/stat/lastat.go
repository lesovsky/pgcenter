// Stuff related to 'load average' stats

package stat

import (
	"bufio"
	"bytes"
	"database/sql"
	"io/ioutil"
	"strconv"
	"strings"
)

const (
	PROC_LOADAVG       = "/proc/loadavg"
	pgProcLoadAvgQuery = "SELECT min1, min5, min15 FROM pgcenter.sys_proc_loadavg"
)

// Container for 'load average' stats
type LoadAvg struct {
	One     float64
	Five    float64
	Fifteen float64
}

// Read stats into container
func (la *LoadAvg) Read(conn *sql.DB, isLocal bool) {
	if isLocal {
		la.ReadLocal()
	} else {
		la.ReadRemote(conn)
	}
}

// Read stat from local procfs cource
func (la *LoadAvg) ReadLocal() {
	content, err := ioutil.ReadFile(PROC_LOADAVG)
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

// Read stats from remote SQL schema
func (la *LoadAvg) ReadRemote(conn *sql.DB) {
	conn.QueryRow(pgProcLoadAvgQuery).Scan(&la.One, &la.Five, &la.Fifteen)
}
