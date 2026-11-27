package view

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNew(t *testing.T) {
	v := New()
	assert.Equal(t, 29, len(v)) // 29 is the total number of views have to be returned
}

func TestNew_AutovacuumScoresView(t *testing.T) {
	views := New()
	autovacuum, ok := views["autovacuum_scores"]
	assert.True(t, ok)
	assert.Equal(t, "autovacuum_scores", autovacuum.Name)
	assert.Equal(t, "Show autovacuum scores", autovacuum.Msg)
	assert.Equal(t, query.PgStatAutovacuumScoresDefault, autovacuum.QueryTmpl)
	assert.Equal(t, 11, autovacuum.Ncols)
	assert.Equal(t, [2]int{0, 0}, autovacuum.DiffIntvl)
	assert.Equal(t, 1, autovacuum.OrderKey)
	assert.True(t, autovacuum.OrderDesc)
	assert.Equal(t, 0, autovacuum.UniqueKey)
	assert.True(t, autovacuum.NotRecordable)
	assert.Equal(t, query.PostgresV19, autovacuum.MinRequiredVersion)
	assert.NotNil(t, autovacuum.ColsWidth)
	assert.NotNil(t, autovacuum.Filters)
	assert.False(t, autovacuum.VersionOK(query.PostgresV18))
	assert.True(t, autovacuum.VersionOK(query.PostgresV19))
}

func TestNew_TablesIndexesAndNegativeScreens(t *testing.T) {
	views := New()
	assert.Equal(t, query.PgStatTablesDefault, views["tables"].QueryTmpl)
	assert.Equal(t, 19, views["tables"].Ncols)
	assert.Equal(t, [2]int{1, 18}, views["tables"].DiffIntvl)
	assert.Equal(t, query.PgStatIndexesDefault, views["indexes"].QueryTmpl)
	assert.Equal(t, 6, views["indexes"].Ncols)
	assert.Equal(t, [2]int{1, 5}, views["indexes"].DiffIntvl)
	assertNegativeScreensUnchanged(t, views)
}

func TestViews_ConfigureTablesIndexesAutovacuum(t *testing.T) {
	for _, version := range []int{query.PostgresV18, query.PostgresV19} {
		t.Run(fmt.Sprintf("PG%d", version/10000), func(t *testing.T) {
			views := New()
			opts := query.NewOptions(version, "f", "off", 0, "public")
			assert.NoError(t, views.Configure(opts))

			autovacuum := views["autovacuum_scores"]
			assert.Equal(t, query.PgStatAutovacuumScoresDefault, autovacuum.QueryTmpl)
			assert.Equal(t, 11, autovacuum.Ncols)
			assert.Equal(t, [2]int{0, 0}, autovacuum.DiffIntvl)
			assert.Equal(t, 1, autovacuum.OrderKey)
			assert.True(t, autovacuum.OrderDesc)
			assert.Equal(t, query.PostgresV19, autovacuum.MinRequiredVersion)
			assert.Equal(t, "Show autovacuum scores", autovacuum.Msg)
			assert.NotNil(t, autovacuum.ColsWidth)
			assert.NotNil(t, autovacuum.Filters)

			tables := views["tables"]
			indexes := views["indexes"]
			if version >= query.PostgresV19 {
				assert.Equal(t, query.PgStatTablesPG19, tables.QueryTmpl)
				assert.Equal(t, 20, tables.Ncols)
				assert.Equal(t, query.PgStatIndexesPG19, indexes.QueryTmpl)
				assert.Equal(t, 7, indexes.Ncols)
			} else {
				assert.Equal(t, query.PgStatTablesDefault, tables.QueryTmpl)
				assert.Equal(t, 19, tables.Ncols)
				assert.Equal(t, query.PgStatIndexesDefault, indexes.QueryTmpl)
				assert.Equal(t, 6, indexes.Ncols)
			}
			assert.Equal(t, [2]int{1, 18}, tables.DiffIntvl)
			assert.Equal(t, [2]int{1, 5}, indexes.DiffIntvl)
			assert.Less(t, tables.DiffIntvl[1], 19)
			assert.Less(t, indexes.DiffIntvl[1], 6)
			assertNegativeScreensUnchanged(t, views)
		})
	}
}

func assertNegativeScreensUnchanged(t *testing.T, views Views) {
	t.Helper()
	functions := views["functions"]
	assert.Equal(t, query.PgStatFunctionsDefault, functions.QueryTmpl)
	assert.Equal(t, 8, functions.Ncols)
	assert.Equal(t, [2]int{3, 3}, functions.DiffIntvl)
	sizes := views["sizes"]
	assert.Equal(t, query.PgTablesSizesDefault, sizes.QueryTmpl)
	assert.Equal(t, 12, sizes.Ncols)
	assert.Equal(t, [2]int{7, 11}, sizes.DiffIntvl)
}

// TestNew_StatementsJITView guards the statements_jit view wiring: it must be registered,
// gated to PG15+, recordable, keyed by the synthetic md5 queryid,
// and sorted by the first *_total column (gen_total).
func TestNew_StatementsJITView(t *testing.T) {
	v := New()
	jit, ok := v["statements_jit"]
	assert.True(t, ok)
	assert.False(t, jit.NotRecordable)
	assert.Equal(t, query.PostgresV15, jit.MinRequiredVersion)
	// Pin the PG15-default map values that Configure() overrides per version.
	assert.Equal(t, 13, jit.Ncols)
	assert.Equal(t, [2]int{6, 10}, jit.DiffIntvl)
	assert.Equal(t, 11, jit.UniqueKey)
	assert.Equal(t, 2, jit.OrderKey)
	assert.True(t, jit.OrderDesc)
	// Msg is load-bearing (Decision 4): it doubles as the empty-screen hint for jit=off.
	assert.Contains(t, jit.Msg, "jit=off")
}

// TestNew_StatIOView guards the stat_io count view wiring: it must be registered,
// gated to PG16+, recordable, keyed by synthetic io_key,
// and sorted by the first diffed counter column.
func TestNew_StatIOView(t *testing.T) {
	v := New()
	statio, ok := v["stat_io"]
	assert.True(t, ok)
	assert.False(t, statio.NotRecordable)
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
// gated to PG16+, recordable, keyed by synthetic io_key,
// and its Msg must carry the track_io_timing hint (Decision 9).
func TestNew_StatIOTimeView(t *testing.T) {
	v := New()
	statioTime, ok := v["stat_io_time"]
	assert.True(t, ok)
	assert.False(t, statioTime.NotRecordable)
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
// gated to PG14+, recordable, and sorted by retained,KiB.
func TestNew_ReplslotsView(t *testing.T) {
	v := New()
	replslots, ok := v["replslots"]
	assert.True(t, ok)
	assert.False(t, replslots.NotRecordable)
	assert.Equal(t, query.PostgresV14, replslots.MinRequiredVersion)
	// Pin the PG14-default map values that Configure() overrides per version.
	assert.Equal(t, 15, replslots.Ncols)
	assert.Equal(t, [2]int{6, 13}, replslots.DiffIntvl)
	assert.Equal(t, 4, replslots.OrderKey)
	assert.True(t, replslots.OrderDesc)
	assert.Equal(t, "Show replication slots statistics", replslots.Msg)
}

// TestNew_BgwriterView guards the bgwriter view wiring: it must be registered,
// gated to PG14+, and recordable.
func TestNew_BgwriterView(t *testing.T) {
	v := New()
	bgwriter, ok := v["bgwriter"]
	assert.True(t, ok)
	assert.False(t, bgwriter.NotRecordable)
	assert.Equal(t, query.PostgresV14, bgwriter.MinRequiredVersion)
	// Pin the PG14-default map values that Configure() overrides per version.
	assert.Equal(t, 12, bgwriter.Ncols)
	assert.Equal(t, [2]int{3, 10}, bgwriter.DiffIntvl)
	assert.Equal(t, "Show bgwriter / checkpointer statistics", bgwriter.Msg)
}

// TestNew_ArchiverView guards the archiver view wiring: it must be registered,
// gated to PG14+, recordable, undiffed, and keyed by the constant 'Archiver' literal
// at column 0.
func TestNew_ArchiverView(t *testing.T) {
	v := New()
	archiver, ok := v["archiver"]
	assert.True(t, ok)
	// Name must equal the map key: top/config_view.go writes the working view back with
	// views[view.Name], so a divergence silently parks column widths and filters in a
	// phantom entry. It is also the report type string and the tar entry prefix.
	assert.Equal(t, "archiver", archiver.Name)
	assert.False(t, archiver.NotRecordable)
	assert.Equal(t, query.PostgresV14, archiver.MinRequiredVersion)
	// The static seed, before Configure() reassigns it from the selector.
	assert.Equal(t, query.PgStatArchiverDefault, archiver.QueryTmpl)
	assert.Equal(t, 9, archiver.Ncols)
	// DiffIntvl{0,0} is deliberate: nothing on this screen is diffed, which is what
	// keeps the NULL-able columns away from diff()/strconv.ParseInt.
	assert.Equal(t, [2]int{0, 0}, archiver.DiffIntvl)
	assert.Equal(t, 0, archiver.OrderKey)
	assert.True(t, archiver.OrderDesc)
	assert.Equal(t, 0, archiver.UniqueKey)
	// Both maps must be non-nil: top/config_view.go:100 and :124 write into ColsWidth and :166
	// writes into Filters in place, so a nil map is a runtime panic inside a gocui key handler
	// that no count test would catch.
	assert.NotNil(t, archiver.ColsWidth)
	assert.NotNil(t, archiver.Filters)
	// Msg is load-bearing and pinned verbatim (Decision 5): it is the cmdline text on every
	// switch to the screen, and the only place the archive_mode requirement is stated.
	assert.Equal(t, "Show archiver statistics (requires archive_mode=on)", archiver.Msg)
}

func TestViews_Configure(t *testing.T) {
	testcases := []struct {
		version     int
		recovery    string
		trackCommit string
		querylen    int
	}{
		// v19 matrix
		{version: 190000, recovery: "f", trackCommit: "on", querylen: 256},
		{version: 190000, recovery: "f", trackCommit: "off", querylen: 0},
		{version: 190000, recovery: "t", trackCommit: "on", querylen: 256},
		{version: 190000, recovery: "t", trackCommit: "off", querylen: 0},
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
		case 190000:
			// The three progress screens gain columns on PG 19; the diffed pairs shift with them.
			assert.Equal(t, query.PgStatProgressVacuumPG19, views["progress_vacuum"].QueryTmpl)
			assert.Equal(t, 15, views["progress_vacuum"].Ncols)
			assert.Equal(t, [2]int{12, 13}, views["progress_vacuum"].DiffIntvl)
			assert.Equal(t, query.PgStatProgressAnalyzePG19, views["progress_analyze"].QueryTmpl)
			assert.Equal(t, 13, views["progress_analyze"].Ncols)
			assert.Equal(t, query.PgStatProgressBasebackupPG19, views["progress_basebackup"].QueryTmpl)
			assert.Equal(t, 12, views["progress_basebackup"].Ncols)
			assert.Equal(t, [2]int{10, 10}, views["progress_basebackup"].DiffIntvl)
			// PG 19 added wal_fpi_bytes; this pins that Configure carries the new wal layout
			// into the registered view, not just that the selector returns it.
			assert.Equal(t, query.PgStatWALPG19, views["wal"].QueryTmpl)
			assert.Equal(t, 8, views["wal"].Ncols)
			assert.Equal(t, [2]int{2, 6}, views["wal"].DiffIntvl)
			// pg_stat_archiver is schema-stable, so the archiver layout is the same on every version.
			// Regression guard only: New() already sets these values, so deleting case "archiver":
			// from Configure() cannot redden this. What it does catch is drift between
			// SelectStatArchiverQuery and the static entry. The wal assertions above are the
			// real wiring gate.
			assert.Equal(t, query.PgStatArchiverDefault, views["archiver"].QueryTmpl)
			assert.Equal(t, 9, views["archiver"].Ncols)
			assert.Equal(t, [2]int{0, 0}, views["archiver"].DiffIntvl)
		case 140000:
			// Everything below PG 19 keeps today's progress layouts, byte for byte.
			assert.Equal(t, query.PgStatProgressVacuumDefault, views["progress_vacuum"].QueryTmpl)
			assert.Equal(t, 13, views["progress_vacuum"].Ncols)
			assert.Equal(t, [2]int{10, 11}, views["progress_vacuum"].DiffIntvl)
			assert.Equal(t, query.PgStatProgressAnalyzeDefault, views["progress_analyze"].QueryTmpl)
			assert.Equal(t, 12, views["progress_analyze"].Ncols)
			assert.Equal(t, query.PgStatProgressBasebackupDefault, views["progress_basebackup"].QueryTmpl)
			assert.Equal(t, 11, views["progress_basebackup"].Ncols)
			assert.Equal(t, [2]int{9, 9}, views["progress_basebackup"].DiffIntvl)
			// The PG 14-17 wal layout must not move when the PG 19 branch is added.
			assert.Equal(t, query.PgStatWALPG14, views["wal"].QueryTmpl)
			assert.Equal(t, 11, views["wal"].Ncols)
			assert.Equal(t, [2]int{2, 9}, views["wal"].DiffIntvl)
			// pg_stat_archiver is schema-stable, so the archiver layout is the same on every version.
			// Regression guard only — see the note in the 190000 arm above.
			assert.Equal(t, query.PgStatArchiverDefault, views["archiver"].QueryTmpl)
			assert.Equal(t, 9, views["archiver"].Ncols)
			assert.Equal(t, [2]int{0, 0}, views["archiver"].DiffIntvl)
		case 130000:
			if tc.trackCommit == "on" {
				assert.Equal(t, query.PgStatReplicationExtended, views["replication"].QueryTmpl)
				assert.Equal(t, 17, views["replication"].Ncols)
			} else {
				assert.Equal(t, query.PgStatReplicationDefault, views["replication"].QueryTmpl)
			}
			// leader/backend_xid/horizon_xacts appear at PG 13; the selector's own table test pins the
			// boundary, this pins that Configure actually carries it into the view.
			assert.Equal(t, query.PgStatActivityPG13, views["activity"].QueryTmpl)
			assert.Equal(t, 17, views["activity"].Ncols)
		case 120000:
			if tc.trackCommit == "on" {
				assert.Equal(t, query.PgStatReplicationExtended, views["replication"].QueryTmpl)
				assert.Equal(t, 17, views["replication"].Ncols)
			} else {
				assert.Equal(t, query.PgStatReplicationDefault, views["replication"].QueryTmpl)
			}
			assert.Equal(t, query.PgStatStatementsTimingPG12, views["statements_timings"].QueryTmpl)
			assert.Equal(t, query.PgStatActivityDefault, views["activity"].QueryTmpl)
			assert.Equal(t, 14, views["activity"].Ncols)
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
		{version: 190000, total: 29},
		{version: 160000, total: 28},
		{version: 140000, total: 25},
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
		assert.Equal(t, tc.total, total, "version=%d", tc.version)
	}
}
