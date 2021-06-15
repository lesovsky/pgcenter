package record

import (
	"archive/tar"
	"database/sql"
	"encoding/json"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
	"github.com/stretchr/testify/assert"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func Test_tarRecorder_open_close(t *testing.T) {
	tc := newTarRecorder(tarConfig{filename: "/tmp/pgcenter-record-testing.stat.tar", append: false})
	assert.NoError(t, tc.open())
	assert.NoError(t, tc.close())

	tc = newTarRecorder(tarConfig{filename: "/tmp/pgcenter-record-testing.stat.tar", append: true})
	assert.NoError(t, tc.open())
	assert.NoError(t, tc.close())
}

func Test_tarRecorder(t *testing.T) {
	tc := newTarRecorder(tarConfig{filename: "/tmp/pgcenter-record-testing.stat.tar"})
	assert.NoError(t, tc.open())

	// create and configure views
	db, err := postgres.NewTestConnect()
	assert.NoError(t, err)
	props, err := stat.GetPostgresProperties(db)
	assert.NoError(t, err)
	views := view.New()
	opts := query.NewOptions(props.VersionNum, props.Recovery, props.GucTrackCommitTimestamp, 0, "public")
	assert.NoError(t, views.Configure(opts))
	db.Close()

	// create postgres config
	dbConfig, err := postgres.NewTestConfig()
	assert.NoError(t, err)
	stats, err := tc.collect(dbConfig, views)
	assert.NoError(t, err)
	assert.NotNil(t, stats)

	// check all stats have filled columns
	for _, s := range stats {
		assert.Greater(t, len(s.Cols), 0)
	}

	assert.NoError(t, tc.close())
}

func Test_tarRecorder_write(t *testing.T) {
	stats := map[string]stat.PGresult{
		"pgcenter_record_testing": {
			Valid: true, Ncols: 2, Nrows: 4, Cols: []string{"col1", "col2"},
			Values: [][]sql.NullString{
				{{String: "alfa", Valid: true}, {String: "12.06157", Valid: true}},
				{{String: "bravo", Valid: true}, {String: "819.188", Valid: true}},
				{{String: "charli", Valid: true}, {String: "18.126", Valid: true}},
				{{String: "delta", Valid: true}, {String: "137.176", Valid: true}},
			},
		},
	}

	filename := "/tmp/pgcenter-record-testing.stat.tar"

	// Write testdata.
	tc := newTarRecorder(tarConfig{filename: filename, append: false})
	assert.NoError(t, tc.open())
	assert.NoError(t, tc.write(stats))
	assert.NoError(t, tc.close())

	// Read written testdata and compare with origin testdata.
	f, err := os.Open(filepath.Clean(filename)) // open file
	assert.NoError(t, err)
	assert.NotNil(t, f)

	tr := tar.NewReader(f) // create tar reader
	hdr, err := tr.Next()
	assert.NoError(t, err)
	data := make([]byte, hdr.Size) // make data buffer
	_, err = io.ReadFull(tr, data) // read data from tar to buffer
	assert.NoError(t, err)
	got := stat.PGresult{}
	assert.NoError(t, json.Unmarshal(data, &got))                                    // unmarshal to JSON
	assert.Equal(t, stats, map[string]stat.PGresult{"pgcenter_record_testing": got}) // compare unmarshalled with origin

	// Cleanup.
	assert.NoError(t, os.Remove(filename))
}

func Test_newFilenameString(t *testing.T) {
	testcases := []struct {
		ts   time.Time
		want string
	}{
		{ts: time.Date(2021, 06, 15, 12, 30, 15, 123456789, time.UTC), want: "example.20210615T123015.123.json"},
		{ts: time.Date(2021, 06, 15, 12, 30, 15, 23456789, time.UTC), want: "example.20210615T123015.023.json"},
		{ts: time.Date(2021, 06, 15, 12, 30, 15, 3456789, time.UTC), want: "example.20210615T123015.003.json"},
		{ts: time.Date(2021, 06, 15, 12, 30, 15, 456789, time.UTC), want: "example.20210615T123015.000.json"},
		{ts: time.Date(2021, 06, 15, 12, 30, 15, 789, time.UTC), want: "example.20210615T123015.000.json"},
		{ts: time.Date(2021, 06, 15, 12, 30, 15, 0, time.UTC), want: "example.20210615T123015.000.json"},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, newFilenameString(tc.ts, "example"))
	}
}
