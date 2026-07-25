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

// Test_app_doReport_ReplSlots_EmptyRetained is the one place in the corpus where
// the empty-last sort rule (Decision 4) meets real recorded data under a sparse
// DEFAULT sort key: replslots sorts by retained,KiB (OrderKey 4) DESC, and that
// column is genuinely NULL for a slot that reserves no WAL.
//
// The case is built as variant A of the tech-spec's Coverage note — a genuine "0"
// standing next to a blank — because the two obvious constructions do NOT diverge
// from the old behaviour: a lone blank under DESC lands last either way, and
// putting the blank first only changes the outcome if the remaining values order
// differently lexicographically than numerically (2048 vs 1024, the pair in
// Test_app_doReport_ReplSlots above, does not).
//
// Both preconditions variant A needs are set explicitly in the curr tick, whose
// row order is what the sort sees:
//  1. the FIRST row (slot_full) carries a non-empty numeric retained,KiB, so the
//     old code picks the numeric comparator rather than falling into string mode;
//  2. the blank row (slot_blank) sits ABOVE the genuine "0" row (slot_zero), so
//     under the old code — where "" parses to 0 and compares equal to "0" —
//     SliceStable keeps the blank first.
//
// Old behaviour: slot_full, slot_blank, slot_zero (blank indistinguishable from a
// genuine zero). New behaviour: slot_full, slot_zero, slot_blank — the slot with
// no value goes last, which is also what the screen's own SQL already asks for
// with ORDER BY "retained,KiB" DESC NULLS LAST.
//
// Only retained,KiB (col 4, passthrough) is blank: an empty cell in the diffed
// block (cols 6..13) would abort the sample in diffPair -> ParseInt(""). No golden
// is written; the assertions below are value-level and normalized.
func Test_app_doReport_ReplSlots_EmptyRetained(t *testing.T) {
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

	mkRow := func(vals []string) []sql.NullString {
		row := make([]sql.NullString, ncols)
		for i, v := range vals {
			row[i] = sql.NullString{String: v, Valid: true}
		}
		return row
	}

	// slot_blank's retained,KiB is "" — a NULL pg_replication_slots row for a slot
	// that reserves nothing. slot_zero's is a genuine "0": it reserves WAL, just
	// none right now. These two states must not collide.
	fullPrev := []string{"slot_full", "logical", "true", "reserved", "1024", "0", "10", "20", "30", "40", "50", "60", "70", "80", "01:00:00"}
	fullCurr := []string{"slot_full", "logical", "true", "reserved", "1024", "0", "15", "28", "45", "52", "70", "90", "100", "130", "02:00:00"}
	blankPrev := []string{"slot_blank", "logical", "true", "reserved", "", "0", "1", "1", "1", "1", "1", "1", "1", "1", "01:00:00"}
	blankCurr := []string{"slot_blank", "logical", "true", "reserved", "", "0", "2", "2", "2", "2", "2", "2", "2", "2", "02:00:00"}
	zeroPrev := []string{"slot_zero", "physical", "true", "reserved", "0", "0", "0", "0", "0", "0", "0", "0", "0", "0", "01:00:00"}
	zeroCurr := []string{"slot_zero", "physical", "true", "reserved", "0", "0", "0", "0", "0", "0", "0", "0", "0", "0", "02:00:00"}

	statPrev := stat.PGresult{
		Valid: true, Ncols: ncols, Nrows: 3, Cols: replslotsCols,
		Values: [][]sql.NullString{mkRow(fullPrev), mkRow(blankPrev), mkRow(zeroPrev)},
	}
	prevBytes, err := json.Marshal(statPrev)
	assert.NoError(t, err)

	// curr row order IS the sort input order (diff() walks curr rows), so the two
	// preconditions above are set here: non-empty numeric first, blank above zero.
	statCurr := stat.PGresult{
		Valid: true, Ncols: ncols, Nrows: 3, Cols: replslotsCols,
		Values: [][]sql.NullString{mkRow(fullCurr), mkRow(blankCurr), mkRow(zeroCurr)},
	}
	currBytes, err := json.Marshal(statCurr)
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
	writeEntry("replslots.20260519T100000.000.json", prevBytes)
	writeEntry("sysinfo.20260519T100000.000.json", sysinfoBytes)
	writeEntry("meta.20260519T100001.000.json", metaBytes)
	writeEntry("replslots.20260519T100001.000.json", currBytes)
	writeEntry("sysinfo.20260519T100001.000.json", sysinfoBytes)
	assert.NoError(t, tw.Close())

	// OrderColName deliberately unset: the view's default OrderKey=4 / DESC governs.
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

	out := stripANSI(buf.String())
	assert.NotEmpty(t, out)
	assert.Contains(t, out, "slot_full")
	assert.Contains(t, out, "slot_zero")
	assert.Contains(t, out, "slot_blank")

	// The rule under test: 1024 first, then the genuine "0", then the slot with no
	// value at all. Before the fix slot_blank and slot_zero compared equal and
	// stability kept slot_blank second.
	assert.Less(t, strings.Index(out, "slot_full"), strings.Index(out, "slot_zero"),
		"retained,KiB DESC: 1024 must print above the genuine 0")
	assert.Less(t, strings.Index(out, "slot_zero"), strings.Index(out, "slot_blank"),
		"a blank retained,KiB must print BELOW a genuine 0, not collide with it")

	// The blank cell renders blank rather than 0: its row carries one field fewer
	// than an otherwise identically shaped row.
	fieldsOf := func(name string) []string {
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, name) {
				return strings.Fields(line)
			}
		}
		return nil
	}
	assert.Equal(t, len(fieldsOf("slot_zero"))-1, len(fieldsOf("slot_blank")),
		"blank retained,KiB must render empty, not as 0")

	// Diff math is unaffected by the sort change: slot_full's cols 6..13 deltas.
	normalized := strings.Join(strings.Fields(out), " ")
	assert.Contains(t, normalized, "slot_full logical true reserved 1024 0 5 8 15 12 20 30 30 50")
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
