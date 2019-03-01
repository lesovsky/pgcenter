// Stuff related to memory/swap usage stats

package stat

import (
	"bufio"
	"bytes"
	"database/sql"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
)

const (
	procMeminfoFile    = "/proc/meminfo"
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
func (m *Meminfo) Read(conn *sql.DB, isLocal bool, pgcAvail bool) {
	if isLocal {
		m.ReadLocal()
	} else if pgcAvail{
		m.ReadRemote(conn)
	}
}

// ReadLocal reads stats from local 'procfs' filesystem
func (m *Meminfo) ReadLocal() {
	content, err := ioutil.ReadFile(procMeminfoFile)
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
func (m *Meminfo) ReadRemote(conn *sql.DB) {
	rows, err := conn.Query(pgProcMeminfoQuery)
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
