package report

import (
	"archive/tar"
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/stretchr/testify/assert"
)

// replslotsCols is the canonical 15-column layout of the replslots report,
// matching internal/query/replication_slots.go (SelectStatReplicationSlotsQuery
// returns Ncols=15, DiffIntvl [6,13]). Columns 6..13 are the eight cumulative
// logical-decoding counters that get diffed; the rest pass through verbatim.
var replslotsCols = []string{
	"slot_name", "slot_type", "active", "wal_status", "retained,KiB", "safe,KiB",
	"spill_txns", "spill_count", "spill,KiB",
	"stream_txns", "stream_count", "stream,KiB",
	"total_txns", "total,KiB",
	"stats_age",
}

// ansiRE matches the SGR color escapes printStatHeader/printStatSample wrap
// around cells, so golden output can be normalized for value-level assertions.
var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

// stripANSI removes terminal color escapes from report output.
func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

// Test_app_doReport_ReplSlots exercises the full doReport pipeline for the
// version-independent replslots report against a synthetic in-memory tar. The
// selector SelectStatReplicationSlotsQuery ignores the PostgreSQL version, so a
// single recorded version (PG14) suffices. The tar carries two cumulative ticks
// (meta + replslots per tick); the first tick is discarded by processData as
// the prev snapshot and the second becomes curr, producing the data rows.
//
// The two ticks are exactly one second apart, so the rate divisor itv == 1 and
// each diffed column (6..13) equals tick2 - tick1 with no scaling. The report
// proves three properties from Decision 3:
//  1. Cumulative counters in the diffed block (cols 6..13) subtract correctly.
//  2. Rows are paired across snapshots by slot_name (UniqueKey defaults to 0).
//  3. The retained,KiB DESC order (OrderKey 4) holds — the logical slot
//     (retained 2048) prints before the physical slot (retained 1024).
//
// The physical slot is the [007] coalesce-contract behavioral check: its eight
// diffed counters are coalesced "0" in both ticks (a physical slot is absent
// from pg_stat_replication_slots, so the LEFT JOIN yields NULL → recorder
// stores "0"). Those cells must diff to a clean "0" without aborting the sample
// (an empty string there would crash diffPair → ParseInt("")). The output is
// compared against a golden so the contract is pinned without a live PostgreSQL.
func Test_app_doReport_ReplSlots(t *testing.T) {
	const ncols = 15

	// Meta result mirrors SelectCommonProperties (7-column shape; readMeta only
	// consumes column index 1 for version_num). replslots is version-independent
	// so PG14 is an arbitrary-but-fixed choice.
	metaRes := stat.PGresult{
		Valid: true, Ncols: 7, Nrows: 1,
		Cols: []string{"version", "version_num", "track_commit_timestamp", "max_connections", "autovacuum_max_workers", "recovery", "start_time_unix"},
		Values: [][]sql.NullString{
			{
				{String: "14.9", Valid: true}, {String: "140009", Valid: true},
				{String: "off", Valid: true}, {String: "100", Valid: true}, {String: "3", Valid: true},
				{String: "false", Valid: true}, {String: "1622828486655396e-6", Valid: true},
			},
		},
	}
	metaBytes, err := json.Marshal(metaRes)
	assert.NoError(t, err)

	mkRow := func(vals []string) []sql.NullString {
		row := make([]sql.NullString, ncols)
		for i, v := range vals {
			row[i] = sql.NullString{String: v, Valid: true}
		}
		return row
	}

	// Two slots, both present (with identical slot_name) in both ticks so they
	// pair by UniqueKey 0. Listed physical-first to prove the report re-sorts to
	// retained,KiB DESC (logical 2048 must end up above physical 1024).
	//
	// physical_b: all eight diffed counters (cols 6..13) are "0" in both ticks
	// — the coalesce-contract zero-cell case; deltas must come out "0".
	// logical_a: counters grow so each diffed column has a non-zero delta:
	//   spill_txns  10->15 = 5    spill_count  20->28 = 8
	//   spill,KiB   30->45 = 15   stream_txns  40->52 = 12
	//   stream_count 50->70 = 20  stream,KiB   60->90 = 30
	//   total_txns  70->100 = 30  total,KiB    80->130 = 50
	physPrev := []string{"physical_b", "physical", "true", "reserved", "1024", "0", "0", "0", "0", "0", "0", "0", "0", "0", "01:00:00"}
	physCurr := []string{"physical_b", "physical", "true", "reserved", "1024", "0", "0", "0", "0", "0", "0", "0", "0", "0", "02:00:00"}
	logPrev := []string{"logical_a", "logical", "true", "reserved", "2048", "0", "10", "20", "30", "40", "50", "60", "70", "80", "01:00:00"}
	logCurr := []string{"logical_a", "logical", "true", "reserved", "2048", "0", "15", "28", "45", "52", "70", "90", "100", "130", "02:00:00"}

	// Tick 1 (prev): discarded by processData's first-snapshot rule
	// (!prevStat.Valid -> continue). Carries the same slot_names as curr but in
	// the OPPOSITE row order (logical-first here, physical-first in curr) so the
	// test actually exercises slot_name pairing: diff() matches curr rows to prev
	// rows by slot_name (UniqueKey 0), not by position. If pairing were positional
	// (curr[i] vs prev[i]), logical_a(curr) would diff against physical_b(prev)
	// and yield garbage/negative deltas — the golden would change and this fails.
	statPrev := stat.PGresult{
		Valid: true, Ncols: ncols, Nrows: 2, Cols: replslotsCols,
		Values: [][]sql.NullString{mkRow(logPrev), mkRow(physPrev)},
	}
	prevBytes, err := json.Marshal(statPrev)
	assert.NoError(t, err)

	// Tick 2 (curr): cumulative values for the logical slot are larger; the
	// physical slot stays at "0". Produces the reported data rows.
	statCurr := stat.PGresult{
		Valid: true, Ncols: ncols, Nrows: 2, Cols: replslotsCols,
		Values: [][]sql.NullString{mkRow(physCurr), mkRow(logCurr)},
	}
	currBytes, err := json.Marshal(statCurr)
	assert.NoError(t, err)

	sysinfoBytes := []byte(`{"ticks":100,"cpu_count":4}`)

	// Compose tar (two ticks; per-tick layout matches tarRecorder.write(): meta
	// + replslots + sysinfo). Filenames use the recorder's 20060102T150405.000
	// format and the two ticks are one second apart so itv == 1.
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	writeEntry := func(name string, payload []byte) {
		hdr := &tar.Header{Name: name, Size: int64(len(payload)), Mode: 0644}
		assert.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write(payload)
		assert.NoError(t, err)
	}
	writeEntry("meta.20260519T100000.000.json", metaBytes)
	writeEntry("replslots.20260519T100000.000.json", prevBytes)
	writeEntry("sysinfo.20260519T100000.000.json", sysinfoBytes)
	writeEntry("meta.20260519T100001.000.json", metaBytes)
	writeEntry("replslots.20260519T100001.000.json", currBytes)
	writeEntry("sysinfo.20260519T100001.000.json", sysinfoBytes)
	assert.NoError(t, tw.Close())

	// Note: OrderColName is deliberately unset so the view's OrderKey=4
	// (retained,KiB DESC) default sort governs row order.
	config := Config{
		ReportType: "replslots",
		TruncLimit: 32,
		TsStart:    time.Date(2026, 5, 19, 0, 0, 0, 0, time.Now().Location()),
		TsEnd:      time.Date(2026, 5, 19, 23, 59, 59, 0, time.Now().Location()),
	}

	app := newApp(config)
	var buf bytes.Buffer
	app.writer = &buf

	tr := tar.NewReader(&tarBuf)
	assert.NoError(t, app.doReport(tr))

	out := buf.String()
	assert.NotEmpty(t, out)
	// Timestamp header line emitted by printStatSample matches "YYYY/MM/DD".
	assert.Regexp(t, regexp.MustCompile(`\d{4}/\d{2}/\d{2}`), out)
	// Both slots must appear (rows paired by slot_name across snapshots).
	assert.Contains(t, out, "logical_a")
	assert.Contains(t, out, "physical_b")
	// retained,KiB DESC: the logical slot (2048) must print before the physical
	// slot (1024) regardless of input order (input was physical-first).
	assert.Less(t, strings.Index(out, "logical_a"), strings.Index(out, "physical_b"))
	// Pin the diff math independently of the golden so the deltas are still
	// guarded if the golden is ever regenerated against buggy code. Strip ANSI
	// color codes and collapse whitespace, then assert the exact computed rows:
	// logical_a cols 6..13 deltas (5 8 15 12 20 30 30 50) and the physical_b
	// coalesced-zero deltas (all 0). retained,KiB (2048/1024) and safe,KiB (0)
	// pass through verbatim.
	normalized := strings.Join(strings.Fields(stripANSI(out)), " ")
	assert.Contains(t, normalized, "logical_a logical true reserved 2048 0 5 8 15 12 20 30 30 50")
	assert.Contains(t, normalized, "physical_b physical true reserved 1024 0 0 0 0 0 0 0 0 0")

	if *update {
		assert.NoError(t, os.WriteFile("testdata/report_record_replslots.golden", buf.Bytes(), 0644))
		return
	}

	want, err := os.ReadFile("testdata/report_record_replslots.golden")
	assert.NoError(t, err)
	assert.Equal(t, string(want), out)
}

// Test_app_doReport_ReplSlots_empty verifies that a tar carrying two replslots
// ticks with zero rows (an empty replication-slots set — a normal state, see
// Decision 5) prints only the timestamp/column header: no data rows and no
// INFO/WARNING line (those are procpidstat-specific). The header-only output is
// pinned to a golden for consistency with the other replay tests.
func Test_app_doReport_ReplSlots_empty(t *testing.T) {
	const ncols = 15

	metaRes := stat.PGresult{
		Valid: true, Ncols: 7, Nrows: 1,
		Cols: []string{"version", "version_num", "track_commit_timestamp", "max_connections", "autovacuum_max_workers", "recovery", "start_time_unix"},
		Values: [][]sql.NullString{
			{
				{String: "14.9", Valid: true}, {String: "140009", Valid: true},
				{String: "off", Valid: true}, {String: "100", Valid: true}, {String: "3", Valid: true},
				{String: "false", Valid: true}, {String: "1622828486655396e-6", Valid: true},
			},
		},
	}
	metaBytes, err := json.Marshal(metaRes)
	assert.NoError(t, err)

	// Both ticks carry zero rows: valid result, no slots recorded.
	emptyRes := stat.PGresult{
		Valid: true, Ncols: ncols, Nrows: 0, Cols: replslotsCols,
		Values: [][]sql.NullString{},
	}
	emptyBytes, err := json.Marshal(emptyRes)
	assert.NoError(t, err)

	sysinfoBytes := []byte(`{"ticks":100,"cpu_count":4}`)

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	writeEntry := func(name string, payload []byte) {
		hdr := &tar.Header{Name: name, Size: int64(len(payload)), Mode: 0644}
		assert.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write(payload)
		assert.NoError(t, err)
	}
	writeEntry("meta.20260519T100000.000.json", metaBytes)
	writeEntry("replslots.20260519T100000.000.json", emptyBytes)
	writeEntry("sysinfo.20260519T100000.000.json", sysinfoBytes)
	writeEntry("meta.20260519T100001.000.json", metaBytes)
	writeEntry("replslots.20260519T100001.000.json", emptyBytes)
	writeEntry("sysinfo.20260519T100001.000.json", sysinfoBytes)
	assert.NoError(t, tw.Close())

	config := Config{
		ReportType: "replslots",
		TruncLimit: 32,
		TsStart:    time.Date(2026, 5, 19, 0, 0, 0, 0, time.Now().Location()),
		TsEnd:      time.Date(2026, 5, 19, 23, 59, 59, 0, time.Now().Location()),
	}

	app := newApp(config)
	var buf bytes.Buffer
	app.writer = &buf

	tr := tar.NewReader(&tarBuf)
	assert.NoError(t, app.doReport(tr))

	out := buf.String()
	assert.NotEmpty(t, out)
	// The column header is present. With zero rows printStatSample never runs,
	// so the per-sample timestamp line ("YYYY/MM/DD ...") is absent — only the
	// column header from printStatHeader remains. That is the expected
	// header-only output for an empty replication-slots set.
	assert.Contains(t, out, "slot_name")
	assert.NotRegexp(t, regexp.MustCompile(`\d{4}/\d{2}/\d{2}`), out)
	// Strongest no-rows signal: printStatSample emits the timestamp/", rate: "
	// line only when at least one row prints, so its absence proves zero data
	// rows (independent of which tokens appear in the column header).
	assert.NotContains(t, out, ", rate: ")
	// Secondary guard: no slot names leak into the output.
	assert.NotContains(t, out, "logical")
	assert.NotContains(t, out, "physical")
	// This test drives app.doReport directly (not RunMain), so printReportHeader's
	// banner INFO lines are out of scope; these guards specifically prove the
	// procpidstat-only diagnostics (no-data INFO at report.go:331-335 and the
	// IO/iodelay WARNING) do not leak into a replslots report.
	assert.NotContains(t, out, "INFO")
	assert.NotContains(t, out, "WARNING")

	if *update {
		assert.NoError(t, os.WriteFile("testdata/report_record_replslots_empty.golden", buf.Bytes(), 0644))
		return
	}

	want, err := os.ReadFile("testdata/report_record_replslots_empty.golden")
	assert.NoError(t, err)
	assert.Equal(t, string(want), out)
}
