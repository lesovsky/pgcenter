package query

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
)

func Test_SelectStatIOQuery(t *testing.T) {
	testcases := []struct {
		version       int
		wantNcols     int
		wantDiffIntvl [2]int
	}{
		{version: 140000, wantNcols: 16, wantDiffIntvl: [2]int{4, 14}},
		{version: 150000, wantNcols: 16, wantDiffIntvl: [2]int{4, 14}},
		// PG16/17: KiB derived from op_bytes.
		{version: 160000, wantNcols: 16, wantDiffIntvl: [2]int{4, 14}},
		{version: 170000, wantNcols: 16, wantDiffIntvl: [2]int{4, 14}},
		// PG18: native read_bytes/write_bytes/extend_bytes.
		{version: 180000, wantNcols: 16, wantDiffIntvl: [2]int{4, 14}},
	}

	for _, tc := range testcases {
		t.Run(fmt.Sprintf("version/%d", tc.version), func(t *testing.T) {
			_, gotNcols, gotDiffIntvl := SelectStatIOQuery(tc.version)
			assert.Equal(t, tc.wantNcols, gotNcols)
			assert.Equal(t, tc.wantDiffIntvl, gotDiffIntvl)
		})
	}
}

func Test_SelectStatIOTimeQuery(t *testing.T) {
	testcases := []struct {
		version       int
		wantNcols     int
		wantDiffIntvl [2]int
	}{
		{version: 140000, wantNcols: 10, wantDiffIntvl: [2]int{4, 8}},
		{version: 150000, wantNcols: 10, wantDiffIntvl: [2]int{4, 8}},
		{version: 160000, wantNcols: 10, wantDiffIntvl: [2]int{4, 8}},
		{version: 170000, wantNcols: 10, wantDiffIntvl: [2]int{4, 8}},
		{version: 180000, wantNcols: 10, wantDiffIntvl: [2]int{4, 8}},
	}

	for _, tc := range testcases {
		t.Run(fmt.Sprintf("version/%d", tc.version), func(t *testing.T) {
			_, gotNcols, gotDiffIntvl := SelectStatIOTimeQuery(tc.version)
			assert.Equal(t, tc.wantNcols, gotNcols)
			assert.Equal(t, tc.wantDiffIntvl, gotDiffIntvl)
		})
	}
}

// Test_SelectStatIOQuery_NullSafety verifies that every diffed column produced by the count selector
// is wrapped in coalesce(...,0). pg_stat_io returns NULL in many cells (fsyncs for temp relation,
// reads for background writer); a NULL reaches the diff machinery as an empty string and aborts the
// whole sample at strconv.ParseInt("") (internal/stat/postgres.go diffPair -> parsePairInt), blanking
// the screen. The diff machinery (PGresult/diff) is unexported in package stat, so the protection is
// verified here at its real source: the SQL string itself must coalesce every column inside DiffIntvl.
func Test_SelectStatIOQuery_NullSafety(t *testing.T) {
	// Raw view columns that land inside the count DiffIntvl [4,14] and can be NULL on some rows.
	diffedCols := []string{"reads", "writes", "extends", "hits", "evictions", "writebacks", "reuses", "fsyncs"}

	for _, version := range []int{160000, 180000} {
		t.Run(fmt.Sprintf("version/%d", version), func(t *testing.T) {
			q, _, _ := SelectStatIOQuery(version)
			for _, col := range diffedCols {
				assert.Contains(t, q, fmt.Sprintf("coalesce(%s,0)", col),
					"diffed column %q must be wrapped in coalesce(...,0) to survive NULL rows", col)
			}

			// The KiB columns (idx 5,7,9) also sit inside DiffIntvl and are derived from byte sources
			// that can themselves be NULL: op_bytes on PG16/17, native *_bytes on PG18. Assert those
			// sources are coalesced too, so all 11 diffed columns are NULL-guarded (task Details edge case).
			if version >= PostgresV18 {
				for _, col := range []string{"read_bytes", "write_bytes", "extend_bytes"} {
					assert.Contains(t, q, fmt.Sprintf("coalesce(%s,0)", col),
						"PG18 KiB source %q must be coalesced", col)
				}
			} else {
				assert.Contains(t, q, "coalesce(op_bytes,0)",
					"PG16/17 KiB multiplier op_bytes must be coalesced")
			}
		})
	}
}

// Test_SelectStatIOTimeQuery_NullSafety verifies coalesce(...,0) on the time selector's diffed columns.
func Test_SelectStatIOTimeQuery_NullSafety(t *testing.T) {
	diffedCols := []string{"read_time", "write_time", "writeback_time", "extend_time", "fsync_time"}

	for _, version := range []int{160000, 180000} {
		t.Run(fmt.Sprintf("version/%d", version), func(t *testing.T) {
			q, _, _ := SelectStatIOTimeQuery(version)
			for _, col := range diffedCols {
				assert.Contains(t, q, fmt.Sprintf("coalesce(%s,0)", col),
					"diffed column %q must be wrapped in coalesce(...,0) to survive NULL rows", col)
			}
		})
	}
}

// Test_StatIOQueries tests count-screen query execution against all supported Postgres versions.
// pg_stat_io exists only on PG16+, so PG14/15 are skipped. PG16 run gates the op_bytes path; PG18 run
// gates the native *_bytes columns AND the presence of object='wal' rows (a shape the local PG17-only
// environment cannot verify).
func Test_StatIOQueries(t *testing.T) {
	versions := []int{160000, 170000, 180000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_io/%d", version), func(t *testing.T) {
			tmpl, wantNcols, _ := SelectStatIOQuery(version)

			opts := NewOptions(version, "f", "off", 256, "public")
			q, err := Format(tmpl, opts)
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}
			defer conn.Close()

			rows, err := conn.Query(q)
			assert.NoError(t, err)

			// Assert the live column count matches the declared Ncols: PG16 verifies the op_bytes-derived
			// KiB path, PG18 verifies the native read_bytes/write_bytes/extend_bytes path.
			assert.Len(t, rows.FieldDescriptions(), wantNcols)
			rows.Close()
			assert.NoError(t, rows.Err())

			// PG18 also gates the new object='wal' rows that no earlier version emits.
			if version >= PostgresV18 {
				var walRows int
				err = conn.QueryRow("SELECT count(*) FROM pg_stat_io WHERE object = 'wal'").Scan(&walRows)
				assert.NoError(t, err)
				assert.Greater(t, walRows, 0, "PG18 pg_stat_io must contain object='wal' rows")
			}
		})
	}
}

// Test_StatIOTimeQueries tests time-screen query execution against all supported Postgres versions.
func Test_StatIOTimeQueries(t *testing.T) {
	versions := []int{160000, 170000, 180000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_io_time/%d", version), func(t *testing.T) {
			tmpl, wantNcols, _ := SelectStatIOTimeQuery(version)

			opts := NewOptions(version, "f", "off", 256, "public")
			q, err := Format(tmpl, opts)
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}
			defer conn.Close()

			rows, err := conn.Query(q)
			assert.NoError(t, err)

			assert.Len(t, rows.FieldDescriptions(), wantNcols)
			rows.Close()
			assert.NoError(t, rows.Err())
		})
	}
}

// Test_SelectStatIOQuery_NoTemplateArtifacts guards against accidental Go-template delimiters in the
// flat SQL: Configure() always runs the query through query.Format, so a stray {{ would break parsing.
func Test_SelectStatIOQuery_NoTemplateArtifacts(t *testing.T) {
	for _, version := range []int{160000, 180000} {
		qCount, _, _ := SelectStatIOQuery(version)
		qTime, _, _ := SelectStatIOTimeQuery(version)
		assert.False(t, strings.Contains(qCount, "{{"), "count query must not contain template delimiters")
		assert.False(t, strings.Contains(qTime, "{{"), "time query must not contain template delimiters")
	}
}
