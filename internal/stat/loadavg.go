// Stuff related to 'load average' stats

package stat

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"io/ioutil"
	"strconv"
	"strings"
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
	content, err := ioutil.ReadFile("/proc/loadavg")
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
	db.QueryRow("SELECT min1, min5, min15 FROM pgcenter.sys_proc_loadavg").Scan(&la.One, &la.Five, &la.Fifteen)
}

/* new */

func readLoadAverage(db *postgres.DB, schemaExists bool) (LoadAvg, error) {
	if db.Local {
		return readLoadAverageLocal("/proc/loadavg")
	} else if schemaExists {
		return readLoadAverageRemote(db)
	}

	return LoadAvg{}, nil
}

func readLoadAverageLocal(statfile string) (LoadAvg, error) {
	var stat LoadAvg

	data, err := ioutil.ReadFile(statfile)
	if err != nil {
		return stat, err
	}

	fields := strings.Fields(string(data))

	if len(fields) < 3 {
		return stat, fmt.Errorf("%s invalid content", statfile)
	}

	values := make([]float64, 3)
	for i, value := range fields[0:3] {
		values[i], err = strconv.ParseFloat(value, 64)
		if err != nil {
			return stat, err
		}
	}

	stat.One, stat.Five, stat.Fifteen = values[0], values[1], values[2]

	return stat, nil
}

//
func readLoadAverageRemote(db *postgres.DB) (LoadAvg, error) {
	var stat LoadAvg
	err := db.QueryRow("SELECT min1, min5, min15 FROM pgcenter.sys_proc_loadavg").Scan(&stat.One, &stat.Five, &stat.Fifteen)
	if err != nil {
		return stat, err
	}
	return stat, nil
}
