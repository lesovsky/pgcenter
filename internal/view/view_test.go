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
		trackCommit string
	}{
		{version: 130000, trackCommit: "on"},
		{version: 130000, trackCommit: "off"},
		{version: 120000, trackCommit: "on"},
		{version: 120000, trackCommit: "off"},
		{version: 110000, trackCommit: "on"},
		{version: 110000, trackCommit: "off"},
		{version: 90600, trackCommit: "on"},
		{version: 90600, trackCommit: "off"},
		{version: 90500, trackCommit: "on"},
		{version: 90500, trackCommit: "off"},
		{version: 90400, trackCommit: "on"},
		{version: 90400, trackCommit: "off"},
	}

	for _, tc := range testcases {
		views := New()
		views.Configure(tc.version, tc.trackCommit)

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
			assert.Equal(t, query.PgStatStatementsTiming12, views["statements_timings"].QueryTmpl)
		case 110000:
			if tc.trackCommit == "on" {
				assert.Equal(t, query.PgStatReplicationExtended, views["replication"].QueryTmpl)
				assert.Equal(t, 17, views["replication"].Ncols)
			} else {
				assert.Equal(t, query.PgStatReplicationDefault, views["replication"].QueryTmpl)
			}
			assert.Equal(t, query.PgStatDatabase11, views["databases"].QueryTmpl)
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
		case 90400:
			assert.Equal(t, query.PgStatReplication96, views["replication"].QueryTmpl)
			assert.Equal(t, 12, views["activity"].Ncols)
		}
	}
}
