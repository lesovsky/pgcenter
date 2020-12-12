// Stuff related to 'load average' stats

package stat

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"io/ioutil"
	"strconv"
	"strings"
)

// LoadAvg describes 'load average' stats based on /proc/loadavg.
type LoadAvg struct {
	One     float64
	Five    float64
	Fifteen float64
}

// readLoadAverage returns load average stats based on type of passed DB connection.
func readLoadAverage(db *postgres.DB, schemaExists bool) (LoadAvg, error) {
	if db.Local {
		return readLoadAverageLocal("/proc/loadavg")
	} else if schemaExists {
		return readLoadAverageRemote(db)
	}

	return LoadAvg{}, nil
}

// readLoadAverageLocal returns load average stats read from local proc file.
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

// readLoadAverageRemote returns load average stats from SQL stats schema.
func readLoadAverageRemote(db *postgres.DB) (LoadAvg, error) {
	var stat LoadAvg
	err := db.QueryRow("SELECT min1, min5, min15 FROM pgcenter.sys_proc_loadavg").Scan(&stat.One, &stat.Five, &stat.Fifteen)
	if err != nil {
		return stat, err
	}
	return stat, nil
}
