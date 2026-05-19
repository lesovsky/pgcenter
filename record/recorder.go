package record

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// recorder defines a way of how to record and store collected stats.
type recorder interface {
	open() error
	collect(dbConfig postgres.Config, views view.Views) (map[string]stat.PGresult, error)
	write(map[string]stat.PGresult) error
	close() error
}

// tarConfig defines configuration needed for creating tar recorder.
// isLocal/ticks/cpuCount/ioAvailable/delayAcctAvailable are populated by
// app.setup() when the recording target is a local Postgres instance and the
// procpidstat view is recordable; on remote targets they remain zero-valued
// and the procpidstat enrichment branch in collect() is skipped.
type tarConfig struct {
	filename           string
	append             bool
	isLocal            bool
	ticks              float64
	cpuCount           int
	ioAvailable        bool
	delayAcctAvailable bool
}

// tarRecorder implement recorder interface.
// This implementation collects Postgres stats and stores it in .json files packed into .tar archive.
// The prev/curr maps and lastCollect timestamp persist across the
// open→collect→write→close ticks driven by app.record(), mirroring the
// map-rotation protocol used by stat.Collector.Update for the live TUI.
type tarRecorder struct {
	config    tarConfig
	file      *os.File
	fileFlags int
	writer    *tar.Writer
	// procpidstat stateful fields — zero-value safe; populated only when
	// config.isLocal is true and the procpidstat view participates in collect().
	prevProcPidStats map[int]stat.ProcPidStat
	currProcPidStats map[int]stat.ProcPidStat
	prevProcPidIO    map[int]stat.ProcPidIO
	currProcPidIO    map[int]stat.ProcPidIO
	lastCollect      time.Time
}

// newTarRecorder creates new recorder.
func newTarRecorder(c tarConfig) recorder {
	var flags int
	if c.append {
		flags = os.O_CREATE | os.O_RDWR
	} else {
		flags = os.O_CREATE | os.O_RDWR | os.O_TRUNC
	}

	return &tarRecorder{
		config:    c,
		fileFlags: flags,
	}
}

// open method opens tar archive.
func (c *tarRecorder) open() error {
	f, err := os.OpenFile(filepath.Clean(c.config.filename), c.fileFlags, 0600)
	if err != nil {
		return err
	}

	// Determine seek offset.
	// If truncate is not requested check the file size. For empty files set
	// offset to 0 - start writing from beginning. For non-empty files set
	// offset to -1024 - start writing from last kB, to avoid overwrite tar metadata.
	if (c.fileFlags & os.O_TRUNC) == 0 {
		var offset int64

		st, err := f.Stat()
		if err != nil {
			return err
		}

		if st.Size() > 0 {
			offset = -1024
		}

		_, err = f.Seek(offset, io.SeekEnd)
		if err != nil {
			return err
		}
	} else {
		// If truncate was requested, disable O_TRUNC ans use just O_RDWR to
		// avoid further archive truncation.
		c.fileFlags = os.O_RDWR
	}

	c.file = f
	c.writer = tar.NewWriter(c.file)

	return nil
}

// collect connects to Postgres, collects and returns stats data.
func (c *tarRecorder) collect(dbConfig postgres.Config, views view.Views) (map[string]stat.PGresult, error) {
	db, err := postgres.Connect(dbConfig)
	if err != nil {
		return nil, err
	}

	defer db.Close()

	stats := map[string]stat.PGresult{}

	// Collect metadata about running Postgres.
	meta, err := stat.NewPGresultQuery(db, query.SelectCommonProperties)
	if err != nil {
		return nil, err
	}

	stats["meta"] = meta

	// Collect the all necessary stats.
	for k, v := range views {
		res, err := stat.NewPGresultQuery(db, v.Query)
		if err != nil {
			return nil, err
		}

		stats[k] = res
	}

	// procpidstat enrichment — replace the 7-column SQL result with the
	// 19-column display PGresult assembled from per-PID procfs snapshots.
	// Gated on local mode: on a remote target /proc/[pid]/* belongs to a
	// different host and would yield meaningless data.
	if pp, ok := stats["procpidstat"]; ok && c.config.isLocal && pp.Valid {
		c.enrichProcPidStat(stats, pp)
	}

	return stats, nil
}

// enrichProcPidStat performs the per-tick procfs join for the procpidstat
// view: rotates prev/curr maps based on PIDs in the current SQL result, reads
// fresh /proc data per PID, and replaces stats["procpidstat"] with the
// 19-column enriched PGresult produced by stat.BuildProcPidResult.
//
// Mirrors the map-rotation protocol in stat.Collector.Update so recorder and
// live TUI produce equivalent display strings.
func (c *tarRecorder) enrichProcPidStat(stats map[string]stat.PGresult, activity stat.PGresult) {
	// Rotate prev maps: keep only PIDs that exist both in the current SQL
	// result and in the prior tick's curr map. PIDs that vanished are dropped.
	newPrevStats := make(map[int]stat.ProcPidStat)
	newPrevIO := make(map[int]stat.ProcPidIO)
	for _, row := range activity.Values {
		if len(row) == 0 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(row[0].String))
		if err != nil || pid <= 0 {
			continue
		}
		if v, ok := c.currProcPidStats[pid]; ok {
			newPrevStats[pid] = v
		}
		if v, ok := c.currProcPidIO[pid]; ok {
			newPrevIO[pid] = v
		}
	}
	c.prevProcPidStats = newPrevStats
	c.prevProcPidIO = newPrevIO
	c.currProcPidStats = make(map[int]stat.ProcPidStat)
	c.currProcPidIO = make(map[int]stat.ProcPidIO)

	// Read fresh procfs data per backend PID. Per-PID errors (process exited
	// mid-tick, EACCES) are skipped silently — the row will still appear in
	// the SQL columns; BuildProcPidResult renders missing procfs cells as the
	// "0"/"" sentinels documented in the build pipeline.
	for _, row := range activity.Values {
		if len(row) == 0 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(row[0].String))
		if err != nil || pid <= 0 {
			continue
		}
		if st, err := stat.ReadProcPidStat(pid); err == nil {
			c.currProcPidStats[pid] = st
		}
		if c.config.ioAvailable {
			if io, err := stat.ReadProcPidIO(pid); err == nil {
				c.currProcPidIO[pid] = io
			}
		}
	}

	// First tick (lastCollect zero): pass itv=0 so BuildProcPidResult emits
	// "0" for all rate columns — the report pipeline skips the first snapshot
	// anyway because prevStat is not yet Valid.
	var itv float64
	if !c.lastCollect.IsZero() {
		itv = time.Since(c.lastCollect).Seconds()
	}
	c.lastCollect = time.Now()

	stats["procpidstat"] = stat.BuildProcPidResult(
		activity,
		c.prevProcPidStats, c.currProcPidStats,
		c.prevProcPidIO, c.currProcPidIO,
		c.config.ioAvailable,
		c.config.delayAcctAvailable,
		c.config.ticks,
		itv,
		c.config.cpuCount,
	)
}

// write accepts stats data and writes it into tar archive.
//
// A single now is captured at the top of the call and reused for every entry
// written in this tick — the stats entries and the sysinfo entry — so all
// entries from the same write() share an identical timestamp string. The
// report-side pipeline relies on matching timestamps to pair sysinfo with the
// per-tick procpidstat snapshot.
func (c *tarRecorder) write(stats map[string]stat.PGresult) error {
	now := time.Now()

	for name, v := range stats {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}

		hdr := &tar.Header{Name: newFilenameString(now, name), Mode: 0644, Size: int64(len(data)), ModTime: now}
		err = c.writer.WriteHeader(hdr)
		if err != nil {
			return err
		}

		_, err = c.writer.Write(data)
		if err != nil {
			return err
		}
	}

	// Append the sysinfo entry. Recorded every tick so the report pipeline
	// has the runtime constants needed to interpret procpidstat columns even
	// if the recording session is split across tar appends with different
	// hosts (CPU count, CLK_TCK).
	sysinfoData, err := json.Marshal(stat.SysInfo{Ticks: c.config.ticks, CPUCount: c.config.cpuCount})
	if err != nil {
		return err
	}
	hdr := &tar.Header{Name: newFilenameString(now, "sysinfo"), Mode: 0644, Size: int64(len(sysinfoData)), ModTime: now}
	if err := c.writer.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := c.writer.Write(sysinfoData); err != nil {
		return err
	}

	return nil
}

// close closes recorder's file and tar writer descriptors.
func (c *tarRecorder) close() error {
	if c.writer != nil {
		err := c.writer.Close()
		if err != nil {
			fmt.Printf("closing tar file failed: %s, continue", err)
		}
	}

	return c.file.Close()
}

// newFilenameString returns a filename string with formatted timestamp and report name.
func newFilenameString(ts time.Time, name string) string {
	return fmt.Sprintf("%s.%s.json", name, ts.Format("20060102T150405.000"))
}
