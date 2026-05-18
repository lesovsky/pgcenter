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
	// view.New() includes views with NotRecordable=true (e.g. procpidstat) which
	// filterViews strips before recording. Count only recordable views + metadata file.
	totalViews := countRecordable(view.New()) + 1 // stats + metadata
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
		// wantN counts filtered views (version-incompatible + statements_* + NotRecordable);
		// wantV counts remaining views after filtering. procpidstat (NotRecordable) is always
		// filtered out, so wantN includes +1 for it and wantV stays at 21 max recordable.
		{version: 140000, pgssSchema: "", wantN: 7, wantV: 15},
		{version: 140000, pgssSchema: "public", wantN: 1, wantV: 21},
		{version: 130000, pgssSchema: "public", wantN: 4, wantV: 18},
		{version: 120000, pgssSchema: "public", wantN: 7, wantV: 15},
		{version: 110000, pgssSchema: "public", wantN: 9, wantV: 13},
		{version: 100000, pgssSchema: "public", wantN: 9, wantV: 13},
	}

	for _, tc := range testcases {
		n, v := filterViews(tc.version, tc.pgssSchema, view.New())
		assert.Equal(t, tc.wantN, n)
		assert.Equal(t, tc.wantV, len(v))
	}
}

func TestFilterViews_NotRecordable(t *testing.T) {
	// A view explicitly marked NotRecordable must be removed by filterViews
	// regardless of version compatibility or pg_stat_statements availability.
	views := view.Views{
		"procpidstat": {
			Name:          "procpidstat",
			NotRecordable: true,
		},
	}

	n, v := filterViews(0, "", views)
	assert.Equal(t, 1, n)
	assert.Equal(t, 0, len(v))
	assert.NotContains(t, v, "procpidstat")

	// Sanity check: view.New() registers procpidstat with NotRecordable=true and Ncols=17.
	all := view.New()
	pp, ok := all["procpidstat"]
	assert.True(t, ok)
	assert.True(t, pp.NotRecordable)
	assert.Equal(t, 17, pp.Ncols)
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
