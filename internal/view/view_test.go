package view

import (
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNew(t *testing.T) {
	v := New()
	assert.Equal(t, 27, len(v)) // 27 is the total number of views have to be returned
}

// TestNew_StatementsJITView guards the statements_jit view wiring: it must be registered,
// gated to PG15+, excluded from recording (NotRecordable), keyed by the synthetic md5 queryid,
// and sorted by the first *_total column (gen_total).
func TestNew_StatementsJITView(t *testing.T) {
	v := New()
	jit, ok := v["statements_jit"]
	assert.True(t, ok)
	assert.True(t, jit.NotRecordable)
	assert.Equal(t, query.PostgresV15, jit.MinRequiredVersion)
	// Pin the PG15-default map values that Configure() overrides per version.
	assert.Equal(t, 13, jit.Ncols)
	assert.Equal(t, [2]int{6, 10}, jit.DiffIntvl)
	assert.Equal(t, 11, jit.UniqueKey)
	assert.Equal(t, 2, jit.OrderKey)
	assert.True(t, jit.OrderDesc)
	assert.NotEqual(t, "", jit.Msg)
}

// TestNew_StatIOView guards the stat_io count view wiring: it must be registered,
// gated to PG16+, excluded from recording (NotRecordable), keyed by synthetic io_key,
// and sorted by the first diffed counter column.
func TestNew_StatIOView(t *testing.T) {
	v := New()
	statio, ok := v["stat_io"]
	assert.True(t, ok)
	assert.True(t, statio.NotRecordable)
	assert.Equal(t, query.PostgresV16, statio.MinRequiredVersion)
	// Pin the PG16-default map values that Configure() overrides per version.
	assert.Equal(t, 16, statio.Ncols)
	assert.Equal(t, [2]int{4, 14}, statio.DiffIntvl)
	assert.Equal(t, 4, statio.OrderKey)
	assert.True(t, statio.OrderDesc)
	assert.Equal(t, 0, statio.UniqueKey)
	assert.NotEqual(t, "", statio.Msg)
}

// TestNew_StatIOTimeView guards the stat_io_time view wiring: it must be registered,
// gated to PG16+, excluded from recording (NotRecordable), keyed by synthetic io_key,
// and its Msg must carry the track_io_timing hint (Decision 9).
func TestNew_StatIOTimeView(t *testing.T) {
	v := New()
	statioTime, ok := v["stat_io_time"]
	assert.True(t, ok)
	assert.True(t, statioTime.NotRecordable)
	assert.Equal(t, query.PostgresV16, statioTime.MinRequiredVersion)
	// Pin the PG16-default map values that Configure() overrides per version.
	assert.Equal(t, 10, statioTime.Ncols)
	assert.Equal(t, [2]int{4, 8}, statioTime.DiffIntvl)
	assert.Equal(t, 4, statioTime.OrderKey)
	assert.True(t, statioTime.OrderDesc)
	assert.Equal(t, 0, statioTime.UniqueKey)
	assert.Contains(t, statioTime.Msg, "track_io_timing")
}

// TestNew_ReplslotsView guards the replslots view wiring: it must be registered,
// gated to PG14+, excluded from recording (NotRecordable), and sorted by retained,KiB.
func TestNew_ReplslotsView(t *testing.T) {
	v := New()
	replslots, ok := v["replslots"]
	assert.True(t, ok)
	assert.True(t, replslots.NotRecordable)
	assert.Equal(t, query.PostgresV14, replslots.MinRequiredVersion)
	// Pin the PG14-default map values that Configure() overrides per version.
	assert.Equal(t, 15, replslots.Ncols)
	assert.Equal(t, [2]int{6, 13}, replslots.DiffIntvl)
	assert.Equal(t, 4, replslots.OrderKey)
	assert.True(t, replslots.OrderDesc)
	assert.Equal(t, "Show replication slots statistics", replslots.Msg)
}

// TestNew_BgwriterView guards the bgwriter view wiring: it must be registered,
// gated to PG14+, and excluded from recording (NotRecordable).
func TestNew_BgwriterView(t *testing.T) {
	v := New()
	bgwriter, ok := v["bgwriter"]
	assert.True(t, ok)
	assert.True(t, bgwriter.NotRecordable)
	assert.Equal(t, query.PostgresV14, bgwriter.MinRequiredVersion)
	// Pin the PG14-default map values that Configure() overrides per version.
	assert.Equal(t, 12, bgwriter.Ncols)
	assert.Equal(t, [2]int{3, 10}, bgwriter.DiffIntvl)
	assert.Equal(t, "Show bgwriter / checkpointer statistics", bgwriter.Msg)
}

func TestViews_Configure(t *testing.T) {
	testcases := []struct {
		version     int
		recovery    string
		trackCommit string
		querylen    int
	}{
		// v14 matrix
		{version: 140000, recovery: "f", trackCommit: "on", querylen: 256},
		{version: 140000, recovery: "f", trackCommit: "on", querylen: 0},
		{version: 140000, recovery: "f", trackCommit: "off", querylen: 256},
		{version: 140000, recovery: "f", trackCommit: "off", querylen: 0},
		{version: 140000, recovery: "t", trackCommit: "on", querylen: 256},
		{version: 140000, recovery: "t", trackCommit: "on", querylen: 0},
		{version: 140000, recovery: "t", trackCommit: "off", querylen: 256},
		{version: 140000, recovery: "t", trackCommit: "off", querylen: 0},
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
			assert.Equal(t, query.PgStatDatabaseGeneralPG11, views["databases_general"].QueryTmpl)
			assert.Equal(t, 18, views["databases_general"].Ncols)
			assert.Equal(t, [2]int{2, 16}, views["databases_general"].DiffIntvl)
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
		{version: 160000, total: 27},
		{version: 140000, total: 24},
		{version: 130000, total: 19},
		{version: 120000, total: 16},
		{version: 110000, total: 14},
		{version: 100000, total: 14},
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
