package view

import (
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNew(t *testing.T) {
	v := New()
	assert.Equal(t, 18, len(v)) // 18 is the total number of views have to be returned
}

func TestViews_Configure(t *testing.T) {
	testcases := []struct {
		version     int
		recovery    string
		trackCommit string
		querylen    int
	}{
		// v13 matrix
		{version: 130000, recovery: "f", trackCommit: "on", querylen: 256},
		{version: 130000, recovery: "f", trackCommit: "on", querylen: 0},
		{version: 130000, recovery: "f", trackCommit: "off", querylen: 256},
		{version: 130000, recovery: "f", trackCommit: "off", querylen: 0},
		{version: 130000, recovery: "t", trackCommit: "on", querylen: 256},
		{version: 130000, recovery: "t", trackCommit: "on", querylen: 0},
		{version: 130000, recovery: "t", trackCommit: "off", querylen: 256},
		{version: 130000, recovery: "t", trackCommit: "off", querylen: 0},
		// v12 matrix
		{version: 120000, recovery: "f", trackCommit: "on", querylen: 256},
		{version: 120000, recovery: "f", trackCommit: "on", querylen: 0},
		{version: 120000, recovery: "f", trackCommit: "off", querylen: 256},
		{version: 120000, recovery: "f", trackCommit: "off", querylen: 0},
		{version: 120000, recovery: "t", trackCommit: "on", querylen: 256},
		{version: 120000, recovery: "t", trackCommit: "on", querylen: 0},
		{version: 120000, recovery: "t", trackCommit: "off", querylen: 256},
		{version: 120000, recovery: "t", trackCommit: "off", querylen: 0},
		// v11 matrix
		{version: 110000, recovery: "f", trackCommit: "on", querylen: 256},
		{version: 110000, recovery: "f", trackCommit: "on", querylen: 0},
		{version: 110000, recovery: "f", trackCommit: "off", querylen: 256},
		{version: 110000, recovery: "f", trackCommit: "off", querylen: 0},
		{version: 110000, recovery: "t", trackCommit: "on", querylen: 256},
		{version: 110000, recovery: "t", trackCommit: "on", querylen: 0},
		{version: 110000, recovery: "t", trackCommit: "off", querylen: 256},
		{version: 110000, recovery: "t", trackCommit: "off", querylen: 0},
		// v9.6 matrix
		{version: 90600, recovery: "f", trackCommit: "on", querylen: 256},
		{version: 90600, recovery: "f", trackCommit: "on", querylen: 0},
		{version: 90600, recovery: "f", trackCommit: "off", querylen: 256},
		{version: 90600, recovery: "f", trackCommit: "off", querylen: 0},
		{version: 90600, recovery: "t", trackCommit: "on", querylen: 256},
		{version: 90600, recovery: "t", trackCommit: "on", querylen: 0},
		{version: 90600, recovery: "t", trackCommit: "off", querylen: 256},
		{version: 90600, recovery: "t", trackCommit: "off", querylen: 0},
		// v9.5 matrix
		{version: 90500, recovery: "f", trackCommit: "on", querylen: 256},
		{version: 90500, recovery: "f", trackCommit: "on", querylen: 0},
		{version: 90500, recovery: "f", trackCommit: "off", querylen: 256},
		{version: 90500, recovery: "f", trackCommit: "off", querylen: 0},
		{version: 90500, recovery: "t", trackCommit: "on", querylen: 256},
		{version: 90500, recovery: "t", trackCommit: "on", querylen: 0},
		{version: 90500, recovery: "t", trackCommit: "off", querylen: 256},
		{version: 90500, recovery: "t", trackCommit: "off", querylen: 0},
		// v9.4 matrix
		{version: 90400, recovery: "f", trackCommit: "on", querylen: 256},
		{version: 90400, recovery: "f", trackCommit: "on", querylen: 0},
		{version: 90400, recovery: "f", trackCommit: "off", querylen: 256},
		{version: 90400, recovery: "f", trackCommit: "off", querylen: 0},
		{version: 90400, recovery: "t", trackCommit: "on", querylen: 256},
		{version: 90400, recovery: "t", trackCommit: "on", querylen: 0},
		{version: 90400, recovery: "t", trackCommit: "off", querylen: 256},
		{version: 90400, recovery: "t", trackCommit: "off", querylen: 0},
	}

	for _, tc := range testcases {
		views := New()
		opts := query.NewOptions(tc.version, tc.recovery, tc.trackCommit, tc.querylen, "public")
		err := views.Configure(opts)
		assert.NoError(t, err)

		switch tc.version {
		case 130000:
			if tc.trackCommit == "on" {
				assert.Equal(t, query.PgStatReplicationExtended, views["replication"].QueryTmpl)
				assert.Equal(t, 17, views["replication"].Ncols)
			} else {
				assert.Equal(t, query.PgStatReplicationDefault, views["replication"].QueryTmpl)
			}
		case 120000:
			if tc.trackCommit == "on" {
				assert.Equal(t, query.PgStatReplicationExtended, views["replication"].QueryTmpl)
				assert.Equal(t, 17, views["replication"].Ncols)
			} else {
				assert.Equal(t, query.PgStatReplicationDefault, views["replication"].QueryTmpl)
			}
			assert.Equal(t, query.PgStatStatementsTimingPG12, views["statements_timings"].QueryTmpl)
		case 110000:
			if tc.trackCommit == "on" {
				assert.Equal(t, query.PgStatReplicationExtended, views["replication"].QueryTmpl)
				assert.Equal(t, 17, views["replication"].Ncols)
			} else {
				assert.Equal(t, query.PgStatReplicationDefault, views["replication"].QueryTmpl)
			}
			assert.Equal(t, query.PgStatDatabasePG11, views["databases"].QueryTmpl)
			assert.Equal(t, 17, views["databases"].Ncols)
			assert.Equal(t, [2]int{1, 15}, views["databases"].DiffIntvl)
		case 90600:
			if tc.trackCommit == "on" {
				assert.Equal(t, query.PgStatReplication96Extended, views["replication"].QueryTmpl)
				assert.Equal(t, 14, views["replication"].Ncols)
			} else {
				assert.Equal(t, query.PgStatReplication96, views["replication"].QueryTmpl)
				assert.Equal(t, 12, views["replication"].Ncols)
			}
			assert.Equal(t, query.PgStatActivity96, views["activity"].QueryTmpl)
			assert.Equal(t, 13, views["activity"].Ncols)
		case 90500:
			assert.Equal(t, query.PgStatActivity95, views["activity"].QueryTmpl)
			assert.Equal(t, 12, views["activity"].Ncols)
		}

		for _, v := range views {
			assert.NotEqual(t, "", v.Query)
		}
	}
}

func TestView_VersionOK(t *testing.T) {
	testcases := []struct {
		version int
		total   int
	}{
		{version: 130000, total: 18},
		{version: 120000, total: 15},
		{version: 110000, total: 13},
		{version: 100000, total: 13},
	}

	for _, tc := range testcases {
		views := New()

		var total int
		for _, v := range views {
			if v.VersionOK(tc.version) {
				total++
			}
		}
		assert.Equal(t, tc.total, total)
	}
}
