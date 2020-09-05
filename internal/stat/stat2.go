package stat

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"os/exec"
	"strconv"
	"strings"
)

//
type Stat2 struct {
	System
	//Postgres
}

//
type System struct {
	LoadAvg LoadAvg
	//Cpuusage
	Meminfo
}

//
type Collector struct {
	ticks        float64
	schemaExists bool
	prev         Stat2
	curr         Stat2
}

func NewCollector(db *postgres.DB) (*Collector, error) {
	cmdOutput, err := exec.Command("getconf", "CLK_TCK").Output()
	if err != nil {
		return nil, fmt.Errorf("determine clock frequency failed: %s", err)
	}

	systicks, err := strconv.ParseFloat(strings.TrimSpace(string(cmdOutput)), 64)
	if err != nil {
		return nil, fmt.Errorf("parse clock frequency value failed: %s", err)
	}

	// In case of remote DB, check pgcenter schema exists. In case of error, just consider the schema is not exist.
	var exists bool
	if !db.Local {
		if err := db.QueryRow(PgCheckPgcenterSchemaQuery).Scan(&exists); err != nil {
			exists = false
		}
	}

	return &Collector{
		ticks:        systicks,
		schemaExists: exists,
	}, nil
}

func (c *Collector) Update(db *postgres.DB) (Stat2, error) {
	loadavg, err := readLoadAverage(db, c.schemaExists)
	if err != nil {
		return Stat2{}, err
	}

	return Stat2{
		System{
			LoadAvg: loadavg,
		},
	}, nil
}
