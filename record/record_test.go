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
	totalViews := len(view.New())
	count, itv := 2, time.Second // recording settings

	testcases := []struct {
		name      string
		config    Config
		filesWant int
	}{
		{
			// a new archive should be created with
			name:      "append to new file",
			config:    Config{Count: count, Interval: itv, OutputFile: filename, TruncateFile: true},
			filesWant: totalViews * count,
		},
		{
			// append to existing file, previously written files should be kept.
			name:      "append to existing file",
			config:    Config{Count: count, Interval: itv, OutputFile: filename, TruncateFile: false},
			filesWant: (totalViews * count) * 2, // doubles because files are from previous test.
		},
		{
			// truncate existing file and write new stats
			name:      "truncate existing file",
			config:    Config{Count: count, Interval: itv, OutputFile: filename, TruncateFile: true},
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
