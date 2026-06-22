package record

import (
	"archive/tar"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/view"
	"github.com/stretchr/testify/assert"
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
		version    int
		pgssSchema string
		wantN      int
		wantV      int
	}{
		// wantN counts filtered views (version-incompatible + statements_* without pgss);
		// wantV counts remaining views after filtering. After feature 008 no production
		// view sets NotRecordable=true, so every view now reaches the version gate (and,
		// for statements_*, the pgss gate); whether a view is dropped is decided purely by
		// version compatibility and pgss availability.
		// The bgwriter and replslots views (both MinRequiredVersion=PostgresV14) are now
		// recordable, so on PG14 they pass both gates and join wantV: relative to the
		// pre-008 NotRecordable baseline this moves 2 views from filtered to remaining on
		// the PG14 rows only (wantN -2, wantV +2).
		// stat_io + stat_io_time (PostgresV16) and statements_jit (PostgresV15) are also
		// recordable now, but on PG14 they are still dropped — by the version gate (and, for
		// statements_jit, by the pgss gate when pgssSchema=="") instead of NotRecordable —
		// so they stay counted in wantN; only the reason for dropping them changed.
		// On PG13 and below all five views are version-incompatible and dropped by the
		// version gate regardless of NotRecordable, so those rows are unchanged from the
		// pre-008 baseline.
		{version: 140000, pgssSchema: "", wantN: 9, wantV: 18},
		{version: 140000, pgssSchema: "public", wantN: 3, wantV: 24},
		{version: 130000, pgssSchema: "public", wantN: 8, wantV: 19},
		{version: 120000, pgssSchema: "public", wantN: 11, wantV: 16},
		{version: 110000, pgssSchema: "public", wantN: 13, wantV: 14},
		{version: 100000, pgssSchema: "public", wantN: 13, wantV: 14},
	}

	for _, tc := range testcases {
		n, v := filterViews(tc.version, tc.pgssSchema, view.New())
		assert.Equal(t, tc.wantN, n)
		assert.Equal(t, tc.wantV, len(v))
	}
}

func TestFilterViews_NotRecordable(t *testing.T) {
	// After task 03 procpidstat is registered with NotRecordable=false (the
	// local/remote gate moved to app.setup()), so filterViews must keep it.
	// The NotRecordable=true drop branch is covered by
	// TestFilterViews_dropsExplicitNotRecordable below.
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

// TestFilterViews_dropsExplicitNotRecordable pins the NotRecordable=true
// drop branch in filterViews() (record.go:210). After task 03 no production
// view sets NotRecordable=true, so the mechanism would otherwise be
// untested; this test guards against accidental removal of the check.
func TestFilterViews_dropsExplicitNotRecordable(t *testing.T) {
	views := view.Views{
		"explicit_not_recordable": {
			Name:          "explicit_not_recordable",
			NotRecordable: true,
		},
	}

	n, v := filterViews(0, "", views)
	assert.Equal(t, 1, n)
	assert.Equal(t, 0, len(v))
	assert.NotContains(t, v, "explicit_not_recordable")
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
