// Stuff related to memory/swap usage stats

package stat

import (
	"bufio"
	"bytes"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

const (
	pgProcMeminfoQuery = `SELECT metric, metric_value
		FROM pgcenter.sys_proc_meminfo
		WHERE metric IN ('MemTotal:','MemFree:','SwapTotal:','SwapFree:', 'Cached:','Dirty:','Writeback:','Buffers:','Slab:')
		ORDER BY 1`
)

// Meminfo is the container for memory/swap usage stats
type Meminfo struct {
	MemTotal     uint64
	MemFree      uint64
	MemUsed      uint64
	SwapTotal    uint64
	SwapFree     uint64
	SwapUsed     uint64
	MemCached    uint64
	MemBuffers   uint64
	MemDirty     uint64
	MemWriteback uint64
	MemSlab      uint64
}

// Read stats into container
func (m *Meminfo) Read(db *postgres.DB, pgcAvail bool) {
	if db.Local {
		m.ReadLocal()
	} else if pgcAvail {
		m.ReadRemote(db)
	}
}

// ReadLocal reads stats from local 'procfs' filesystem
func (m *Meminfo) ReadLocal() {
	content, err := ioutil.ReadFile("/proc/meminfo")
	if err != nil {
		return
	}

	reader := bufio.NewReader(bytes.NewBuffer(content))
	for {
		line, _, err := reader.ReadLine()
		if err == io.EOF {
			break
		}

		fields := strings.Fields(string(line))
		if len(fields) > 0 {
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return
			}
			value /= 1024 /* kB -> MB conversion */

			switch fields[0] {
			case "MemTotal:":
				m.MemTotal = value
			case "MemFree:":
				m.MemFree = value
			case "SwapTotal:":
				m.SwapTotal = value
			case "SwapFree:":
				m.SwapFree = value
			case "Cached:":
				m.MemCached = value
			case "Dirty:":
				m.MemDirty = value
			case "Writeback:":
				m.MemWriteback = value
			case "Buffers:":
				m.MemBuffers = value
			case "Slab:":
				m.MemSlab = value
			}
		}
	}
	m.MemUsed = m.MemTotal - m.MemFree - m.MemCached - m.MemBuffers - m.MemSlab
	m.SwapUsed = m.SwapTotal - m.SwapFree
}

// ReadRemote reads stats from remote Postgres instance
func (m *Meminfo) ReadRemote(db *postgres.DB) {
	rows, err := db.Query(pgProcMeminfoQuery)
	if err != nil {
		return
	} /* ignore errors, zero stat is ok for us */
	defer rows.Close()

	var name string
	var value uint64
	for rows.Next() {
		if err := rows.Scan(&name, &value); err != nil {
			return
		}
		value /= 1024 /* kB -> MB conversion */

		switch name {
		case "MemTotal:":
			m.MemTotal = value
		case "MemFree:":
			m.MemFree = value
		case "SwapTotal:":
			m.SwapTotal = value
		case "SwapFree:":
			m.SwapFree = value
		case "Cached:":
			m.MemCached = value
		case "Dirty:":
			m.MemDirty = value
		case "Writeback:":
			m.MemWriteback = value
		case "Buffers:":
			m.MemBuffers = value
		case "Slab:":
			m.MemSlab = value
		}
	}

	m.MemUsed = m.MemTotal - m.MemFree - m.MemCached - m.MemBuffers - m.MemSlab
	m.SwapUsed = m.SwapTotal - m.SwapFree
}

/* new */
func readMeminfo(db *postgres.DB, schemaExists bool) (Meminfo, error) {
	if db.Local {
		return readMeminfoLocal("/proc/meminfo")
	} else if schemaExists {
		return readMeminfoRemote(db)
	}

	return Meminfo{}, nil
}

func readMeminfoLocal(statfile string) (Meminfo, error) {
	var stat Meminfo

	f, err := os.Open(statfile)
	if err != nil {
		return stat, err
	}

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		fields := strings.Fields(line)
		if len(fields) < 3 {
			// TODO: log error to stderr
			continue
		}

		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			// TODO: log error to stderr
			continue
		}

		switch fields[0] {
		case "MemTotal:":
			stat.MemTotal = value / 1024
		case "MemFree:":
			stat.MemFree = value / 1024
		case "SwapTotal:":
			stat.SwapTotal = value / 1024
		case "SwapFree:":
			stat.SwapFree = value / 1024
		case "Cached:":
			stat.MemCached = value / 1024
		case "Dirty:":
			stat.MemDirty = value / 1024
		case "Writeback:":
			stat.MemWriteback = value / 1024
		case "Buffers:":
			stat.MemBuffers = value / 1024
		case "Slab:":
			stat.MemSlab = value / 1024
		}
	}
	stat.MemUsed = stat.MemTotal - stat.MemFree - stat.MemCached - stat.MemBuffers - stat.MemSlab
	stat.SwapUsed = stat.SwapTotal - stat.SwapFree

	return stat, scanner.Err()
}

func readMeminfoRemote(db *postgres.DB) (Meminfo, error) {
	var stat Meminfo

	rows, err := db.Query(pgProcMeminfoQuery)
	if err != nil {
		return stat, err
	}
	defer rows.Close()

	var name string
	var value uint64
	for rows.Next() {
		if err := rows.Scan(&name, &value); err != nil {
			// TODO: log error to stderr
			continue
		}

		switch name {
		case "MemTotal:":
			stat.MemTotal = value / 1024
		case "MemFree:":
			stat.MemFree = value / 1024
		case "SwapTotal:":
			stat.SwapTotal = value / 1024
		case "SwapFree:":
			stat.SwapFree = value / 1024
		case "Cached:":
			stat.MemCached = value / 1024
		case "Dirty:":
			stat.MemDirty = value / 1024
		case "Writeback:":
			stat.MemWriteback = value / 1024
		case "Buffers:":
			stat.MemBuffers = value / 1024
		case "Slab:":
			stat.MemSlab = value / 1024
		}
	}

	stat.MemUsed = stat.MemTotal - stat.MemFree - stat.MemCached - stat.MemBuffers - stat.MemSlab
	stat.SwapUsed = stat.SwapTotal - stat.SwapFree

	return stat, rows.Err()
}
