// Stuff related to memory/swap usage stats

package stat

import (
	"bufio"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Meminfo describes memory/swap stats based on /proc/meminfo.
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

// readMeminfo returns memory/swap stats based on type of passed DB connection.
func readMeminfo(db *postgres.DB, schemaExists bool) (Meminfo, error) {
	if db.Local {
		return readMeminfoLocal("/proc/meminfo")
	} else if schemaExists {
		return readMeminfoRemote(db)
	}

	return Meminfo{}, nil
}

// readMeminfoLocal returns memory/swap stats read from local proc file.
func readMeminfoLocal(statfile string) (Meminfo, error) {
	var stat Meminfo

	f, err := os.Open(filepath.Clean(statfile))
	if err != nil {
		return stat, err
	}
	defer func() {
		_ = f.Close()
	}()

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

// readMeminfoRemote returns memory/swap stats from SQL stats schema.
func readMeminfoRemote(db *postgres.DB) (Meminfo, error) {
	var stat Meminfo

	query := `SELECT metric, metric_value
		FROM pgcenter.sys_proc_meminfo
		WHERE metric IN ('MemTotal:','MemFree:','SwapTotal:','SwapFree:', 'Cached:','Dirty:','Writeback:','Buffers:','Slab:')
		ORDER BY 1`

	rows, err := db.Query(query)
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
