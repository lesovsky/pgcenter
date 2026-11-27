package record

import (
	"archive/tar"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"testing"
	"time"
)

func Test_app_setup(t *testing.T) {
	dbconfig, err := postgres.NewTestConfig()
	assert.NoError(t, err)

	app := newApp(Config{OutputFile: "/tmp/pgcenter-record-testing.stat.tar"}, dbconfig)

	assert.NoError(t, app.setup())

	assert.NotNil(t, app.views)          // views must not be nil
	assert.Greater(t, len(app.views), 0) // views must contains view objects
	for _, v := range app.views {
		assert.NotEqual(t, "", v.Query) // view's queries must not be empty (must be created using templates)
	}
	assert.NotNil(t, app.recorder)
}

func Test_app_setupPG19AutovacuumScoresExclusion(t *testing.T) {
	conn, err := postgres.NewTestConnectVersion(query.PostgresV19)
	if err != nil {
		t.Skipf("skip: PostgreSQL 19 is not available: %v", err)
	}
	conn.Close()

	dbconfig, err := postgres.NewConfig("127.0.0.1", 21919, "postgres", "pgcenter_fixtures")
	require.NoError(t, err)
	app := newApp(Config{OutputFile: filepath.Join(t.TempDir(), "record.tar")}, dbconfig)
	output := captureStdout(t, func() {
		require.NoError(t, app.setup())
	})

	assert.NotContains(t, app.views, "autovacuum_scores")
	t.Logf("sanitized PG 19 record setup output:\n%s", output)
	assert.Contains(t, output, "INFO: autovacuum_scores is not recordable (live TUI only), skip recording it")
	assert.NotContains(t, output, "not supported by the current version")
	assert.NotNil(t, app.recorder)
}

func Test_app_record(t *testing.T) {
	filename := "/tmp/pgcenter-record-testing.stat.tar"
	// Count only recordable views (NotRecordable=false) plus two non-stat entries
	// written per tick by tarRecorder.write(): meta.* and sysinfo.* (sysinfo is
	// emitted unconditionally so the report pipeline always has CLK_TCK/cpu_count).
	totalViews := countRecordable(view.New()) + 2 // stats + meta + sysinfo
	count, itv := 2, time.Second                  // recording settings

	testcases := []struct {
		name      string
		config    Config
		filesWant int
	}{
		{
			// a new archive should be created with
			name:      "append to new file",
			config:    Config{Count: count, Interval: itv, OutputFile: filename, AppendFile: false},
			filesWant: totalViews * count,
		},
		{
			// append to existing file, previously written files should be kept.
			name:      "append to existing file",
			config:    Config{Count: count, Interval: itv, OutputFile: filename, AppendFile: true},
			filesWant: (totalViews * count) * 2, // doubles because files are from previous test.
		},
		{
			// truncate existing file and write new stats
			name:      "truncate existing file",
			config:    Config{Count: count, Interval: itv, OutputFile: filename, AppendFile: false},
			filesWant: totalViews * count,
		},
	}

	// initial stuff
	doQuit := make(chan os.Signal, 1)
	signal.Notify(doQuit, os.Interrupt)

	dbconfig, err := postgres.NewTestConfig()
	assert.NoError(t, err)

	// Skip when test PostgreSQL is unavailable — app.record would otherwise
	// panic on a nil connection instead of failing cleanly.
	if conn, err := postgres.NewTestConnect(); err != nil {
		t.Skipf("skip: test postgres not available: %v", err)
	} else {
		conn.Close()
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			app := newApp(tc.config, dbconfig)
			assert.NoError(t, app.setup())

			assert.NoError(t, app.record(doQuit))

			// Read written stats.
			f, err := os.Open(filepath.Clean(filename))
			assert.NoError(t, err)
			tr := tar.NewReader(f)

			var filesCount int
			for {
				hdr, err := tr.Next()
				if err == io.EOF {
					break
				}
				filesCount++
				assert.NoError(t, err)
				assert.Greater(t, hdr.Size, int64(0))
			}
			assert.Greater(t, filesCount, 0)
			assert.Equal(t, tc.filesWant, filesCount)
		})
	}
	assert.NoError(t, os.Remove(filename))
}

func Test_filterViews(t *testing.T) {
	testcases := []struct {
		version      int
		pgssSchema   string
		wantN        int
		wantV        int
		wantArchiver bool
	}{
		// wantN counts only version/extension exclusions reported by the aggregate INFO.
		// autovacuum_scores is always excluded by name as live-TUI-only and is deliberately
		// absent from that count, so it cannot be misreported as a server-version exclusion.
		// wantArchiver and the explicit autovacuum_scores assertion below pin membership by
		// name; equal counts alone could hide one view replacing another in the kept set.
		{version: 190000, pgssSchema: "public", wantN: 0, wantV: 28, wantArchiver: true},
		{version: 140000, pgssSchema: "", wantN: 9, wantV: 19, wantArchiver: true},
		{version: 140000, pgssSchema: "public", wantN: 3, wantV: 25, wantArchiver: true},
		{version: 130000, pgssSchema: "public", wantN: 9, wantV: 19, wantArchiver: false},
		{version: 120000, pgssSchema: "public", wantN: 12, wantV: 16, wantArchiver: false},
		{version: 110000, pgssSchema: "public", wantN: 14, wantV: 14, wantArchiver: false},
		{version: 100000, pgssSchema: "public", wantN: 14, wantV: 14, wantArchiver: false},
	}

	for _, tc := range testcases {
		n, v := filterViews(tc.version, tc.pgssSchema, view.New())
		assert.Equal(t, tc.wantN, n, "version=%d pgss=%q", tc.version, tc.pgssSchema)
		assert.Equal(t, tc.wantV, len(v), "version=%d pgss=%q", tc.version, tc.pgssSchema)
		// Membership by lookup rather than assert.Contains: a failing Contains on a view map
		// dumps all 28 View structs, which buries the one bit of information needed here.
		_, gotArchiver := v["archiver"]
		assert.Equal(t, tc.wantArchiver, gotArchiver, "archiver kept? version=%d pgss=%q", tc.version, tc.pgssSchema)
		_, gotAutovacuum := v["autovacuum_scores"]
		assert.False(t, gotAutovacuum, "autovacuum_scores must always be excluded by name; version=%d pgss=%q", tc.version, tc.pgssSchema)
	}
}

func TestFilterViews_NotRecordable(t *testing.T) {
	// procpidstat uses the runtime local/remote gate in app.setup(), not the
	// static NotRecordable filter, so filterViews must keep it.
	views := view.Views{
		"procpidstat": {
			Name:          "procpidstat",
			NotRecordable: false,
		},
	}

	n, v := filterViews(0, "", views)
	assert.Equal(t, 0, n)
	assert.Equal(t, 1, len(v))
	assert.Contains(t, v, "procpidstat")

	// Sanity check: view.New() registers procpidstat with NotRecordable=false and Ncols=19.
	all := view.New()
	pp, ok := all["procpidstat"]
	assert.True(t, ok)
	assert.False(t, pp.NotRecordable)
	assert.Equal(t, 19, pp.Ncols)
}

// TestFilterViews_dropsExplicitNotRecordable pins the generic drop branch
// independently of the production autovacuum_scores registry entry.
func TestFilterViews_dropsExplicitNotRecordable(t *testing.T) {
	views := view.Views{
		"explicit_not_recordable": {
			Name:          "explicit_not_recordable",
			NotRecordable: true,
		},
	}

	n, v := filterViews(0, "", views)
	assert.Equal(t, 0, n)
	assert.Equal(t, 0, len(v))
	assert.NotContains(t, v, "explicit_not_recordable")
}

func TestFilterViews_NotRecordableInfoIsNamedAndSorted(t *testing.T) {
	views := view.Views{
		"zeta":  {Name: "zeta", NotRecordable: true},
		"alpha": {Name: "alpha", NotRecordable: true},
	}

	var (
		n        int
		filtered view.Views
	)
	output := captureStdout(t, func() {
		n, filtered = filterViews(query.PostgresV19, "public", views)
	})

	assert.Equal(t, 0, n, "deliberate live-only exclusions must not increment the unsupported-version count")
	assert.Empty(t, filtered)
	assert.Equal(t, "INFO: alpha is not recordable (live TUI only), skip recording it\n"+
		"INFO: zeta is not recordable (live TUI only), skip recording it\n", output)
}

func TestFilterViews_Recordable(t *testing.T) {
	// A view with NotRecordable=false (zero value) and no version requirement
	// must pass through filterViews unchanged.
	views := view.Views{
		"sample": {
			Name: "sample",
		},
	}

	n, v := filterViews(0, "", views)
	assert.Equal(t, 0, n)
	assert.Equal(t, 1, len(v))
	assert.Contains(t, v, "sample")
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	fn()
	require.NoError(t, w.Close())
	os.Stdout = old
	output, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	return string(output)
}

// countRecordable returns the number of views that filterViews will keep,
// i.e. excluding any with NotRecordable=true.
func countRecordable(views view.Views) int {
	var n int
	for _, v := range views {
		if !v.NotRecordable {
			n++
		}
	}
	return n
}
