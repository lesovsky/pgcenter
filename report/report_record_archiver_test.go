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

// archiverCols is the canonical 9-column layout of the archiver report, matching
// internal/query/archiver.go (SelectStatArchiverQuery returns Ncols=9,
// DiffIntvl [0,0], version-independent).
var archiverCols = []string{
	"source", "ready", "archived", "last_archived", "archived_age",
	"failed", "last_failed", "failed_age", "stats_age",
}

// Test_app_doReport_Archiver exercises the full doReport pipeline for the
// archiver report against a synthetic in-memory tar: a recording of two
// cumulative ticks plus a meta record whose version_num drives report-time
// view.Configure. The first tick is discarded by processData's first-snapshot
// rule (!prevStat.Valid -> continue), so two ticks produce exactly one data row.
//
// What this replay pins, and what it deliberately does not:
//
//   - It pins the rendering and diff pipeline for this screen — above all the
//     PASS-THROUGH property. SelectStatArchiverQuery returns DiffIntvl{0,0},
//     which short-circuits calculateDelta before diff() is ever entered
//     (internal/stat/postgres.go:589-597), so every column is copied from the
//     current tick verbatim. The assertions therefore pin ABSOLUTE current
//     values, not deltas: a future change that gives this screen a diffed range
//     turns the golden and the value sentinels red.
//   - It pins that the four NULL-able columns render as blank cells rather than
//     a "0" or a placeholder token (the never_archived subcase).
//   - It does NOT pin the SQL. Report replays recorded stat.PGresult JSON, and
//     the column names and their order come from the fixture this test writes,
//     never from the query text. Reordering the aliases inside
//     internal/query/archiver.go would not redden anything here; that layout is
//     pinned by the query and view unit tests (internal/query, internal/view).
//   - For the same reason it does not pin the view REGISTRY either: processData
//     re-configures through a one-entry map it builds itself
//     (report/report.go:282-284) and Views.Configure switches on the map key, so
//     an unregistered "archiver" would still receive Ncols and DiffIntvl from
//     the selector. What this test depends on from internal/view is the
//     `case "archiver":` inside Configure, not the New() entry.
//
// The screen is version-independent (SelectStatArchiverQuery ignores its
// parameter), so a single golden with no version suffix suffices — the
// report_record_stat_io_time.golden convention. The two ticks are exactly one
// second apart so the rate divisor itv == 1; with nothing diffed that spacing
// only fixes the printed "rate:" value in the golden.
func Test_app_doReport_Archiver(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		// A cluster that has archived and has failures. Values are chosen so a
		// pass-through row and a diffed row cannot be confused: were the screen
		// diffed, archived would render 3 instead of 100003 and ready 4 instead
		// of 14. A NotContains delta sentinel is deliberately NOT used: the
		// would-be delta "3" is a substring of "100003", so it would be vacuous.
		prev := archiverRow(
			"Archiver", "10", "100000", "000000010000000000000021", "00:00:41",
			"5", "000000010000000000000019", "00:12:02", "01:00:00",
		)
		curr := archiverRow(
			"Archiver", "14", "100003", "000000010000000000000024", "00:00:07",
			"8", "000000010000000000000019", "00:14:02", "02:00:00",
		)

		out := runArchiverReplay(t, archiverCols, [][]sql.NullString{prev, curr}, true)

		assert.NotEmpty(t, out)
		// Timestamp header line emitted by printStatSample matches "YYYY/MM/DD".
		assert.Regexp(t, regexp.MustCompile(`\d{4}/\d{2}/\d{2}`), out)
		// Row sentinel: localizes a failure to "row missing" rather than
		// "golden differs".
		assert.Contains(t, out, "Archiver")
		// Pass-through sentinels: the CURRENT absolute values, not deltas.
		assert.Contains(t, out, "100003")
		assert.Contains(t, out, "000000010000000000000024")

		const wantFile = "testdata/report_record_archiver.golden"
		if *update {
			assert.NoError(t, os.WriteFile(wantFile, []byte(out), 0644))
			return
		}
		want, err := os.ReadFile(wantFile)
		assert.NoError(t, err)
		assert.Equal(t, string(want), out)
	})

	t.Run("never_archived", func(t *testing.T) {
		// A cluster with archive_mode=on that has never archived anything: the
		// four NULL-able columns arrive as SQL NULLs in both ticks. They must be
		// sql.NullString{Valid: false} — not {String: "", Valid: true} — because
		// that is what the recorder writes for a SQL NULL, and the difference is
		// exactly what this subcase claims to be about.
		//
		// This is its own recording rather than a second row of the populated
		// one on purpose: alignment is computed once from the first printed
		// sample (formatStatSample returns early when view.Aligned), so a
		// NULL-first recording would size the columns from blank cells and then
		// truncate the 24-character WAL names of any later row.
		null := sql.NullString{Valid: false}
		mk := func(statsAge string) []sql.NullString {
			return []sql.NullString{
				{String: "Archiver", Valid: true},
				{String: "0", Valid: true},
				{String: "0", Valid: true},
				null,
				null,
				{String: "0", Valid: true},
				null,
				null,
				{String: statsAge, Valid: true},
			}
		}

		out := runArchiverReplay(t, archiverCols, [][]sql.NullString{mk("01:00:00"), mk("02:00:00")}, true)

		assert.NotEmpty(t, out)

		stripped := statIOStripANSI(out)

		// The header still names all nine columns — the blank cells do not
		// collapse the layout.
		for _, name := range archiverCols {
			assert.Contains(t, stripped, name)
		}

		// The load-bearing assertion, and the machine reading of the user-spec's
		// "колонки пустые (не 0 и не прочерк)": the single data line splits into
		// exactly five whitespace-separated fields — Archiver, 0, 0, 0,
		// 02:00:00. If the four NULL cells rendered any token at all the count
		// would be nine.
		//
		// No NotContains "n/a" assertion here: naLiteral (top/stat.go) is a
		// TUI-only sentinel from the top package and can never appear in report
		// output, so such an assertion would be vacuous by construction.
		line := archiverDataLine(t, stripped)
		assert.Equal(t, 5, len(strings.Fields(line)), "data line %q", line)
	})

	t.Run("no_archiver_entries", func(t *testing.T) {
		// A recording that carries meta.* and sysinfo.* but no archiver.* entry
		// at all — e.g. a recording made before the screen existed. Nothing
		// matches the report type, so nothing is ever aligned and
		// printStatHeader returns early on !v.Aligned: the buffer stays
		// LITERALLY empty. The three INFO: lines live in printReportHeader,
		// which the CLI path calls outside doReport, so they are not written
		// here either.
		//
		// The claim is about the buffer, not the error return: doReport returns
		// nil on every path (processData's error is printed with fmt.Println and
		// swallowed), so the return value carries no information — see the
		// comment on runArchiverReplay's assert.NoError.
		out := runArchiverReplay(t, archiverCols, [][]sql.NullString{
			archiverRow("Archiver", "10", "100000", "000000010000000000000021", "00:00:41",
				"5", "000000010000000000000019", "00:12:02", "01:00:00"),
			archiverRow("Archiver", "14", "100003", "000000010000000000000024", "00:00:07",
				"8", "000000010000000000000019", "00:14:02", "02:00:00"),
		}, false)

		assert.Empty(t, out)
	})
}

// archiverRow converts a tick's values into a row of non-NULL sql.NullString.
func archiverRow(vals ...string) []sql.NullString {
	row := make([]sql.NullString, len(vals))
	for i, v := range vals {
		row[i] = sql.NullString{String: v, Valid: true}
	}
	return row
}

// archiverDataLine returns the data row of an ANSI-stripped report output: the
// line following the "<ts>, rate: <interval>" line printStatSample emits on its
// own line before the first row of a snapshot.
func archiverDataLine(t *testing.T, stripped string) string {
	t.Helper()

	lines := strings.Split(stripped, "\n")
	for i, l := range lines {
		if strings.Contains(l, ", rate: ") && i+1 < len(lines) {
			return lines[i+1]
		}
	}
	t.Fatalf("no timestamp line found in output:\n%s", stripped)
	return ""
}

// runArchiverReplay builds a synthetic two-tick tar from the given rows, runs
// the full doReport pipeline over it and returns the produced output.
//
// writeStat toggles whether the archiver.* entries are written at all: with
// false the tar carries only meta.* and sysinfo.*, which is the no_archiver_entries
// fixture. That toggle is what makes the emptiness assertion meaningful — an
// empty buffer is also what a BROKEN fixture produces (TsStart/TsEnd not
// bracketing the filename dates, a three-part entry name failing isFilenameOK, a
// mistyped ReportType), so the same tar with the entries added back must produce
// output.
func runArchiverReplay(t *testing.T, cols []string, rows [][]sql.NullString, writeStat bool) string {
	t.Helper()

	ncols := len(cols)

	// Meta result mirrors SelectCommonProperties (7-column shape; readMeta only
	// consumes column index 1 for version_num, which drives the version-aware
	// view.Configure at report time). 17 is used to make it plain the archiver
	// screen does not branch on the version.
	metaRes := stat.PGresult{
		Valid: true, Ncols: 7, Nrows: 1,
		Cols: []string{"version", "version_num", "track_commit_timestamp", "max_connections", "autovacuum_max_workers", "recovery", "start_time_unix"},
		Values: [][]sql.NullString{
			{
				{String: "17.1", Valid: true}, {String: "170000", Valid: true},
				{String: "off", Valid: true}, {String: "100", Valid: true}, {String: "3", Valid: true},
				{String: "false", Valid: true}, {String: "1622828486655396e-6", Valid: true},
			},
		},
	}
	metaBytes, err := json.Marshal(metaRes)
	assert.NoError(t, err)

	mkResult := func(row []sql.NullString) []byte {
		res := stat.PGresult{
			Valid: true, Ncols: ncols, Nrows: 1, Cols: cols,
			Values: [][]sql.NullString{row},
		}
		b, e := json.Marshal(res)
		assert.NoError(t, e)
		return b
	}

	sysinfoBytes := []byte(`{"ticks":100,"cpu_count":4}`)

	// Compose tar (two ticks; per-tick layout matches tarRecorder.write(): meta
	// + archiver + sysinfo). Filenames use the recorder's 20060102T150405.000
	// format — isFilenameOK requires exactly four dot-separated parts, which is
	// what the .000 millisecond field provides — and the two ticks are one
	// second apart so itv == 1.
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	writeEntry := func(name string, payload []byte) {
		hdr := &tar.Header{Name: name, Size: int64(len(payload)), Mode: 0644}
		assert.NoError(t, tw.WriteHeader(hdr))
		_, e := tw.Write(payload)
		assert.NoError(t, e)
	}
	timestamps := []string{"20260519T100000.000", "20260519T100001.000"}
	for i, ts := range timestamps {
		writeEntry("meta."+ts+".json", metaBytes)
		if writeStat {
			writeEntry("archiver."+ts+".json", mkResult(rows[i]))
		}
		writeEntry("sysinfo."+ts+".json", sysinfoBytes)
	}
	assert.NoError(t, tw.Close())

	// TsStart/TsEnd must bracket the filename dates or isFilenameTimestampOK
	// silently skips every entry and any subcase degenerates into the
	// empty-archive one while looking like a real recording.
	config := Config{
		ReportType: "archiver",
		TruncLimit: 32,
		TsStart:    time.Date(2026, 5, 19, 0, 0, 0, 0, time.Now().Location()),
		TsEnd:      time.Date(2026, 5, 19, 23, 59, 59, 0, time.Now().Location()),
	}

	app := newApp(config)
	var buf bytes.Buffer
	app.writer = &buf

	tr := tar.NewReader(&tarBuf)
	// Hygiene only, NOT evidence: doReport returns nil on every path
	// (report/report.go:109-151), so this assertion can never turn red. Every
	// claim this test makes is made about the buffer.
	assert.NoError(t, app.doReport(tr))

	return buf.String()
}
