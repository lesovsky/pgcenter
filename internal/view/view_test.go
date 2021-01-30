package view

import (
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNew(t *testing.T) {
	v := New()
	assert.Equal(t, 15, len(v)) // 15 is the total number of views have to be returned
}

func TestViews_Configure(t *testing.T) {
	testcases := []struct {
		version     int
		recovery    string
		trackCommit string
		app         string
	}{
		// v13 matrix
		{version: 130000, recovery: "f", trackCommit: "on", app: "top"},
		{version: 130000, recovery: "f", trackCommit: "on", app: "record"},
		{version: 130000, recovery: "f", trackCommit: "off", app: "top"},
		{version: 130000, recovery: "f", trackCommit: "off", app: "record"},
		{version: 130000, recovery: "t", trackCommit: "on", app: "top"},
		{version: 130000, recovery: "t", trackCommit: "on", app: "record"},
		{version: 130000, recovery: "t", trackCommit: "off", app: "top"},
		{version: 130000, recovery: "t", trackCommit: "off", app: "record"},
		// v12 matrix
		{version: 120000, recovery: "f", trackCommit: "on", app: "top"},
		{version: 120000, recovery: "f", trackCommit: "on", app: "record"},
		{version: 120000, recovery: "f", trackCommit: "off", app: "top"},
		{version: 120000, recovery: "f", trackCommit: "off", app: "record"},
		{version: 120000, recovery: "t", trackCommit: "on", app: "top"},
		{version: 120000, recovery: "t", trackCommit: "on", app: "record"},
		{version: 120000, recovery: "t", trackCommit: "off", app: "top"},
		{version: 120000, recovery: "t", trackCommit: "off", app: "record"},
		// v11 matrix
		{version: 110000, recovery: "f", trackCommit: "on", app: "top"},
		{version: 110000, recovery: "f", trackCommit: "on", app: "record"},
		{version: 110000, recovery: "f", trackCommit: "off", app: "top"},
		{version: 110000, recovery: "f", trackCommit: "off", app: "record"},
		{version: 110000, recovery: "t", trackCommit: "on", app: "top"},
		{version: 110000, recovery: "t", trackCommit: "on", app: "record"},
		{version: 110000, recovery: "t", trackCommit: "off", app: "top"},
		{version: 110000, recovery: "t", trackCommit: "off", app: "record"},
		// v9.6 matrix
		{version: 90600, recovery: "f", trackCommit: "on", app: "top"},
		{version: 90600, recovery: "f", trackCommit: "on", app: "record"},
		{version: 90600, recovery: "f", trackCommit: "off", app: "top"},
		{version: 90600, recovery: "f", trackCommit: "off", app: "record"},
		{version: 90600, recovery: "t", trackCommit: "on", app: "top"},
		{version: 90600, recovery: "t", trackCommit: "on", app: "record"},
		{version: 90600, recovery: "t", trackCommit: "off", app: "top"},
		{version: 90600, recovery: "t", trackCommit: "off", app: "record"},
		// v9.5 matrix
		{version: 90500, recovery: "f", trackCommit: "on", app: "top"},
		{version: 90500, recovery: "f", trackCommit: "on", app: "record"},
		{version: 90500, recovery: "f", trackCommit: "off", app: "top"},
		{version: 90500, recovery: "f", trackCommit: "off", app: "record"},
		{version: 90500, recovery: "t", trackCommit: "on", app: "top"},
		{version: 90500, recovery: "t", trackCommit: "on", app: "record"},
		{version: 90500, recovery: "t", trackCommit: "off", app: "top"},
		{version: 90500, recovery: "t", trackCommit: "off", app: "record"},
		// v9.4 matrix
		{version: 90400, recovery: "f", trackCommit: "on", app: "top"},
		{version: 90400, recovery: "f", trackCommit: "on", app: "record"},
		{version: 90400, recovery: "f", trackCommit: "off", app: "top"},
		{version: 90400, recovery: "f", trackCommit: "off", app: "record"},
		{version: 90400, recovery: "t", trackCommit: "on", app: "top"},
		{version: 90400, recovery: "t", trackCommit: "on", app: "record"},
		{version: 90400, recovery: "t", trackCommit: "off", app: "top"},
		{version: 90400, recovery: "t", trackCommit: "off", app: "record"},
	}

	for _, tc := range testcases {
		views := New()
		err := views.Configure(tc.version, tc.recovery, tc.trackCommit, tc.app)
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
