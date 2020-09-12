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
	Pgstat
}

//
type System struct {
	LoadAvg
	CpuStat
	Meminfo
}

//
type Collector struct {
	config       Config
	ticks        float64
	schemaExists bool
	prevCpuStat  CpuStat
	currCpuStat  CpuStat
	prevPgStat   Pgstat
	currPgStat   Pgstat
}

type Config struct {
	PgInfo // postgres variables and constants which are not changed in runtime (but might change between Postgres restarts)
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

	// read Postgres properties
	config, err := readPostgresConfig(db)
	if err != nil {
		return nil, fmt.Errorf("read postgres properties failed: %s", err)
	}

	return &Collector{
		config:       Config{PgInfo: config},
		ticks:        systicks,
		schemaExists: exists,
	}, nil
}

func (c *Collector) Update(db *postgres.DB) (Stat2, error) {
	loadavg, err := readLoadAverage(db, c.schemaExists)
	if err != nil {
		return Stat2{}, err
	}

	meminfo, err := readMeminfo(db, c.schemaExists)
	if err != nil {
		return Stat2{}, err
	}

	cpustat, err := readCpuStat(db, c.schemaExists)
	if err != nil {
		return Stat2{}, err
	}

	c.prevCpuStat = c.currCpuStat
	c.currCpuStat = cpustat

	cpuusage := countCpuUsage(c.prevCpuStat, c.currCpuStat, c.ticks)

	pgstat, err := collectPostgresStat(db, c.prevPgStat)
	if err != nil {
		return Stat2{}, err
	}

	c.prevPgStat = c.currPgStat
	c.currPgStat = pgstat

	pgstat.PgInfo = c.config.PgInfo

	return Stat2{
		System: System{
			LoadAvg: loadavg,
			Meminfo: meminfo,
			CpuStat: cpuusage,
		},
		Pgstat: pgstat,
	}, nil
}
