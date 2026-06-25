package stat

import (
	"archive/tar"
	"bytes"
	"database/sql"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/stretchr/testify/assert"
	"io"
	"math"
	"os"
	"testing"
)

// newTestPGresult return PGresult with test content for test purposes.
func newTestPGresult() PGresult {
	return PGresult{
		Valid: true,
		Ncols: 4,
		Nrows: 8,
		Cols:  []string{"col1", "col2", "col3", "col4"},
		Values: [][]sql.NullString{
			{
				{String: "248", Valid: true}, {String: "brodsky", Valid: true}, {String: "row6:value3", Valid: true}, {String: "row6:value4", Valid: true},
			},
			{
				{String: "3", Valid: true}, {String: "direct", Valid: true}, {String: "row3:value3", Valid: true}, {String: "row3:value4", Valid: true},
			},
			{
				{String: "15", Valid: true}, {String: "evioni", Valid: true}, {String: "row5:value3", Valid: true}, {String: "row2:value4", Valid: true},
			},
			{
				{String: "48752", Valid: true}, {String: "aalfia", Valid: true}, {String: "row8:value3", Valid: true}, {String: "row8:value4", Valid: true},
			},
			{
				{String: "2", Valid: true}, {String: "cilla", Valid: true}, {String: "row2:value3", Valid: true}, {String: "row2:value4", Valid: true},
			},
			{
				{String: "4", Valid: true}, {String: "arktika", Valid: true}, {String: "row3:value3", Valid: true}, {String: "row4:value4", Valid: true},
			},
			{
				{String: "3987", Valid: true}, {String: "fasivy", Valid: true}, {String: "row7:value3", Valid: true}, {String: "row7:value4", Valid: true},
			},
			{
				{String: "1", Valid: true}, {String: "bronze", Valid: true}, {String: "row1:value3", Valid: true}, {String: "row1:value4", Valid: true},
			},
		},
	}
}

func Test_collectPostgresStat(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	got, err := collectPostgresStat(conn, query.PgStatDatabaseGeneralDefault)
	assert.NoError(t, err)
	assert.Greater(t, got.Nrows, 0)

	// testing with already closed conn
	conn.Close()
	_, err = collectPostgresStat(conn, "SELECT qq")
	assert.Error(t, err)
}

func Test_collectActivityStat(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	version := 1000000 // suppose to use PG 100.0
	got, err := collectActivityStat(conn, version, "public", 1, 0)
	assert.NoError(t, err)
	assert.Equal(t, "ok", got.State)
	assert.NotEqual(t, "", got.Uptime)
	assert.NotEqual(t, "", got.Recovery)
	assert.NotEqual(t, "", got.Recovery)
	assert.Greater(t, got.ConnTotal+got.ConnIdle+got.ConnIdleXact+got.ConnActive+got.ConnWaiting+got.ConnOthers+got.ConnPrepared, 0)
	assert.NotEqual(t, 0, got.StmtAvgTime)
	assert.NotEqual(t, 0, got.Calls)
	assert.NotEqual(t, 0, got.CallsRate)

	// testing with already closed conn
	conn.Close()
	_, err = collectActivityStat(conn, 0, "public", 1, 0)
	assert.Error(t, err)
}

func Test_collectOverviewStat(t *testing.T) {
	versions := []int{140000, 150000, 160000, 170000, 180000}

	for _, version := range versions {
		conn, err := postgres.NewTestConnectVersion(version)
		if err != nil {
			t.Skipf("postgres %d not available in test environment", version)
		}

		props, err := GetPostgresProperties(conn)
		assert.NoError(t, err)

		// First tick: no prev snapshot. Must not panic; rates stay zero/n/a.
		first, _ := collectOverviewStat(conn, props, 1, PgstatOverview{}, false)
		assert.True(t, first.Valid)
		assert.False(t, first.HasPrev)
		assert.False(t, first.CacheHitRatioValid, "cache hit ratio is n/a on the first tick")
		assert.GreaterOrEqual(t, first.DatabasesCount, int64(1))
		assert.True(t, first.TotalSizeValid)
		assert.GreaterOrEqual(t, first.TotalSize, int64(0))
		assert.GreaterOrEqual(t, first.WalSize, int64(0))

		// Second tick with first as prev: rates are computed, no panic on deltas.
		second, _ := collectOverviewStat(conn, props, 1, first, false)
		assert.True(t, second.Valid)
		assert.True(t, second.HasPrev)
		// tps = (Δcommits + Δrollbacks)/itv; >= 0 (counters never go backwards on a live cluster).
		assert.GreaterOrEqual(t, second.TPSRate, int64(0))
		// others is an interval delta (value, not /s).
		assert.GreaterOrEqual(t, second.OthersInterval, int64(0))
		// bgwr/ckpt absolute counters populated; write/sync deltas computed.
		assert.GreaterOrEqual(t, second.CkptTimed, int64(0))
		assert.GreaterOrEqual(t, second.CkptReq, int64(0))

		// Exact rate formula against a synthetic prev with known deltas. Using prev counters BELOW the
		// live ones guarantees positive, deterministic deltas regardless of background activity. This
		// proves tps = (Δcommit+Δrollback)/itv and others = Δ(...) over the interval (not /s).
		base, _ := collectOverviewStat(conn, props, 1, PgstatOverview{}, false)
		synthPrev := PgstatOverview{
			Valid:             true,
			TotalSizeValid:    true,
			WorkloadCommits:   base.WorkloadCommits - 100,
			WorkloadRollbacks: base.WorkloadRollbacks - 40,
			WorkloadInserts:   base.WorkloadInserts - 20,
			WorkloadOthers:    base.WorkloadOthers - 7,
		}
		// itv = 2 so the /itv division is actually observable (not an identity).
		withItv2, _ := collectOverviewStat(conn, props, 2, synthPrev, false)
		expectedTPS := ((withItv2.WorkloadCommits + withItv2.WorkloadRollbacks) - (synthPrev.WorkloadCommits + synthPrev.WorkloadRollbacks)) / 2
		assert.Equal(t, expectedTPS, withItv2.TPSRate, "tps must be (Δcommit+Δrollback)/itv")
		// others is the raw interval delta, NOT divided by itv.
		assert.Equal(t, withItv2.WorkloadOthers-synthPrev.WorkloadOthers, withItv2.OthersInterval, "others must be the interval delta, not /s")

		conn.Close()
	}
}

func Test_cacheHitRatio(t *testing.T) {
	// Pure-function table test for the per-interval ratio, including the division-by-zero edge.
	testcases := []struct {
		name      string
		dHit      int64
		dRead     int64
		wantRatio float64
		wantValid bool
	}{
		{name: "all hits", dHit: 100, dRead: 0, wantRatio: 100, wantValid: true},
		{name: "half", dHit: 50, dRead: 50, wantRatio: 50, wantValid: true},
		{name: "no io -> n/a", dHit: 0, dRead: 0, wantRatio: 0, wantValid: false},
		{name: "negative denom -> n/a", dHit: -5, dRead: 0, wantRatio: 0, wantValid: false},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ratio, valid := cacheHitRatio(tc.dHit, tc.dRead)
			assert.Equal(t, tc.wantValid, valid)
			assert.Equal(t, tc.wantRatio, ratio)
			assert.False(t, math.IsNaN(ratio), "ratio must never be NaN")
		})
	}
}

func Test_collectOverviewStat_CacheHitRatio(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)
	defer conn.Close()

	props, err := GetPostgresProperties(conn)
	assert.NoError(t, err)

	base, _ := collectOverviewStat(conn, props, 1, PgstatOverview{}, false)

	// Synthetic prev with counters strictly BELOW the live ones -> Δhit/Δread are deterministically
	// positive, so the per-interval ratio path runs and the result is a valid percentage in [0,100].
	prev := PgstatOverview{
		Valid:          true,
		TotalSizeValid: true,
		BlksHit:        base.BlksHit - 800,
		BlksRead:       base.BlksRead - 200,
	}
	got, _ := collectOverviewStat(conn, props, 1, prev, false)
	assert.True(t, got.HasPrev)
	assert.True(t, got.CacheHitRatioValid, "with positive Δ(hit+read) the ratio must be valid")
	assert.GreaterOrEqual(t, got.CacheHitRatio, float64(0))
	assert.LessOrEqual(t, got.CacheHitRatio, float64(100))
	assert.False(t, math.IsNaN(got.CacheHitRatio))

	// Deterministic division-by-zero: prev counters equal to current -> Δ(hit+read) == 0 -> n/a.
	// (cacheHitRatio is unit-tested directly in Test_cacheHitRatio; here we assert the collect wiring.)
	prevEqual := PgstatOverview{Valid: true, TotalSizeValid: true, BlksHit: got.BlksHit, BlksRead: got.BlksRead}
	noio, _ := collectOverviewStat(conn, props, 1, prevEqual, false)
	if noio.BlksHit == prevEqual.BlksHit && noio.BlksRead == prevEqual.BlksRead {
		assert.False(t, noio.CacheHitRatioValid)
		assert.False(t, math.IsNaN(noio.CacheHitRatio))
	}
}

func Test_collectOverviewStat_Degradation(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)
	defer conn.Close()

	props, err := GetPostgresProperties(conn)
	assert.NoError(t, err)

	// Test clusters are primaries with no replication and (typically) no slots: lag/slots/retained
	// degrade to n/a while the rest of the struct is populated (one failed row does not blank the sample).
	got, _ := collectOverviewStat(conn, props, 1, PgstatOverview{}, false)
	assert.True(t, got.Valid)
	// No standbys -> lag is n/a.
	assert.False(t, got.LagBytesValid, "no replication -> lag is n/a")
	// No slots -> retained is n/a, but SlotsCount is still a real 0.
	assert.False(t, got.RetainedValid, "no slots -> retained WAL is n/a")
	assert.Equal(t, int64(0), got.SlotsCount)

	// Archiving backlog: the fixtures role has pg_monitor, so the OWN-QueryRow aggregate must run.
	// On archive_mode=off it is a real 0 with ArchivingBacklogValid=true; either way it must be a
	// non-negative value distinguishable from n/a, and its outcome must NOT have blanked other rows.
	if got.ArchivingBacklogValid {
		assert.GreaterOrEqual(t, got.ArchivingBacklog, int64(0))
	}

	// The degraded replication fields (and whatever backlog state) must NOT gate the cheap rows:
	// count, cache-counter source and the separately-queried size aggregate all stay populated.
	assert.GreaterOrEqual(t, got.DatabasesCount, int64(1))
	assert.True(t, got.TotalSizeValid, "size runs as its own QueryRow and is unaffected by degraded rows")
	assert.GreaterOrEqual(t, got.TotalSize, int64(0))
}

func TestGetPostgresProperties(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	conn.Local = false // set conn as non-local

	got, err := GetPostgresProperties(conn)
	assert.NoError(t, err)
	assert.NotEqual(t, "", got.Version)
	assert.NotEqual(t, 0, got.VersionNum)
	assert.NotEqual(t, "", got.GucTrackCommitTimestamp)
	assert.NotEqual(t, 0, got.GucMaxConnections)
	assert.NotEqual(t, 0, got.GucAVMaxWorkers)
	assert.NotEqual(t, "", got.GucSharedPreLibraries)
	assert.NotEqual(t, "", got.Recovery)
	assert.NotEqual(t, "", got.StartTime)
	assert.NotEqual(t, 0, got.SysTicks)
	assert.NotEqual(t, 0, got.GucMaxWorkerProcesses)
	assert.NotEqual(t, 0, got.GucMaxParallelWorkers)
	assert.NotEqual(t, int64(0), got.GucWalSegmentSize)
	assert.NotEqual(t, "", got.DataDirectory)

	// testing with already closed conn
	conn.Close()
	_, err = GetPostgresProperties(conn)
	assert.Error(t, err)
}

func TestNewPGresultQuery(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	want := PGresult{
		Valid: true, Ncols: 4, Nrows: 3, Cols: []string{"id", "name", "v1", "v2"},
		Values: [][]sql.NullString{
			{{String: "1", Valid: true}, {String: "one", Valid: true}, {String: "10", Valid: true}, {String: "11.1", Valid: true}},
			{{String: "2", Valid: true}, {String: "two", Valid: true}, {String: "20", Valid: true}, {String: "22.2", Valid: true}},
			// next row contains NULL values, all Valid fields are 'false'
			{{String: "3", Valid: true}, {String: "", Valid: false}, {String: "", Valid: false}, {String: "", Valid: false}},
		},
	}
	got, err := NewPGresultQuery(conn, "SELECT * FROM (VALUES (1,'one',10,11.1), (2,'two',20,22.2), (3,NULL,NULL,NULL)) AS t (id,name,v1,v2)")
	assert.NoError(t, err)
	assert.Equal(t, want, got)

	// testing empty query
	_, err = NewPGresultQuery(conn, "")
	assert.Error(t, err)

	// testing with already closed conn
	conn.Close()
	_, err = NewPGresultQuery(conn, "SELECT 1")
	assert.Error(t, err)
}

func Test_NewPGresultFile(t *testing.T) {
	testcases := []struct {
		valid    bool
		filename string
	}{
		{valid: true, filename: "testdata/pgcenter.stat.golden.tar"},
		{valid: false, filename: "testdata/pgcenter.stat.invalid.tar"},
	}

	for _, tc := range testcases {
		t.Run(tc.filename, func(t *testing.T) {
			f, err := os.Open(tc.filename)
			assert.NoError(t, err)

			r := tar.NewReader(f)

			for {
				hdr, err := r.Next()
				if err == io.EOF {
					break
				} else if err != nil {
					assert.Fail(t, "unexpected error", err)
				}

				got, err := NewPGresultFile(r, hdr.Size)
				if tc.valid {
					assert.NoError(t, err)
					assert.NotNil(t, got.Values)
					assert.NotNil(t, got.Cols)
				} else {
					assert.Error(t, err)
					assert.Equal(t, PGresult{}, got)
				}
			}
		})
	}
}

func TestPGresult_validate(t *testing.T) {
	testcases := []struct {
		valid bool
		res   PGresult
	}{
		{valid: true, res: PGresult{
			Valid: true, Ncols: 4, Nrows: 2, Cols: []string{"col1", "col2", "col3", "col4"},
			Values: [][]sql.NullString{
				{{String: "1", Valid: true}, {String: "one", Valid: true}, {String: "10", Valid: true}, {String: "111e-1", Valid: true}},
				{{String: "3", Valid: true}, {String: "", Valid: false}, {String: "", Valid: false}, {String: "", Valid: false}},
			},
		}},
		{valid: false, res: PGresult{
			Valid: true, Ncols: 4, Nrows: 1, Cols: []string{"col1", "col2", "col3", "col4"},
			Values: [][]sql.NullString{
				{{String: "1", Valid: true}, {String: "one", Valid: true}, {String: "10", Valid: true}},
			},
		}},
		{valid: false, res: PGresult{
			Valid: true, Ncols: 4, Nrows: 2, Cols: []string{"col1", "col2", "col3", "col4"},
			Values: [][]sql.NullString{
				{{String: "1", Valid: true}, {String: "one", Valid: true}, {String: "10", Valid: true}, {String: "111e-1", Valid: true}},
			},
		}},
	}

	for _, tc := range testcases {
		err := tc.res.validate()
		if tc.valid {
			assert.NoError(t, err)
		} else {
			assert.Error(t, err)
		}
	}
}

func Test_calculateDelta(t *testing.T) {
	prev := PGresult{
		Valid: true, Ncols: 4, Nrows: 4, Cols: []string{"unique", "col2", "col3", "col4"},
		Values: [][]sql.NullString{
			{{String: "1", Valid: true}, {String: "300", Valid: true}, {String: "100", Valid: true}, {String: "500", Valid: true}},
			{{String: "2", Valid: true}, {String: "400", Valid: true}, {String: "200", Valid: true}, {String: "600", Valid: true}},
			{{String: "3", Valid: true}, {String: "100.0", Valid: true}, {String: "300", Valid: true}, {String: "700", Valid: true}},
			{{String: "4", Valid: true}, {String: "200", Valid: true}, {String: "400.0", Valid: true}, {String: "800", Valid: true}},
			// next row is not present in 'curr' and should be skipped.
			{{String: "5", Valid: true}, {String: "200", Valid: true}, {String: "400.0", Valid: true}, {String: "800", Valid: true}},
		},
	}
	curr := PGresult{
		Valid: true, Ncols: 4, Nrows: 5, Cols: []string{"unique", "col2", "col3", "col4"},
		Values: [][]sql.NullString{
			{{String: "1", Valid: true}, {String: "330.5", Valid: true}, {String: "150", Valid: true}, {String: "500", Valid: true}},
			{{String: "2", Valid: true}, {String: "440", Valid: true}, {String: "280.6", Valid: true}, {String: "620", Valid: true}},
			{{String: "3", Valid: true}, {String: "110", Valid: true}, {String: "300", Valid: true}, {String: "710", Valid: true}},
			{{String: "4", Valid: true}, {String: "220", Valid: true}, {String: "490", Valid: true}, {String: "800", Valid: true}},
			// next row is not present in 'prev' and should be added as-is to 'diff' result.
			{{String: "6", Valid: true}, {String: "560", Valid: true}, {String: "510", Valid: true}, {String: "920", Valid: true}},
		},
	}
	currInvalid := PGresult{
		Valid: true, Ncols: 4, Nrows: 1, Cols: []string{"unique", "col2", "col3", "col4"},
		Values: [][]sql.NullString{
			{{String: "1", Valid: true}, {String: "invalid", Valid: true}, {String: "150", Valid: true}, {String: "500", Valid: true}},
		},
	}
	wantAsc := PGresult{
		Valid: true, Ncols: 4, Nrows: 5, Cols: []string{"unique", "col2", "col3", "col4"},
		Values: [][]sql.NullString{
			{{String: "3", Valid: true}, {String: "10.00", Valid: true}, {String: "0", Valid: true}, {String: "10", Valid: true}},
			{{String: "4", Valid: true}, {String: "20", Valid: true}, {String: "90.00", Valid: true}, {String: "0", Valid: true}},
			{{String: "1", Valid: true}, {String: "30.50", Valid: true}, {String: "50", Valid: true}, {String: "0", Valid: true}},
			{{String: "2", Valid: true}, {String: "40", Valid: true}, {String: "80.60", Valid: true}, {String: "20", Valid: true}},
			{{String: "6", Valid: true}, {String: "560", Valid: true}, {String: "510", Valid: true}, {String: "920", Valid: true}},
		},
	}
	wantDesc := PGresult{
		Valid: true, Ncols: 4, Nrows: 5, Cols: []string{"unique", "col2", "col3", "col4"},
		Values: [][]sql.NullString{
			{{String: "6", Valid: true}, {String: "560", Valid: true}, {String: "510", Valid: true}, {String: "920", Valid: true}},
			{{String: "2", Valid: true}, {String: "40", Valid: true}, {String: "80.60", Valid: true}, {String: "20", Valid: true}},
			{{String: "1", Valid: true}, {String: "30.50", Valid: true}, {String: "50", Valid: true}, {String: "0", Valid: true}},
			{{String: "4", Valid: true}, {String: "20", Valid: true}, {String: "90.00", Valid: true}, {String: "0", Valid: true}},
			{{String: "3", Valid: true}, {String: "10.00", Valid: true}, {String: "0", Valid: true}, {String: "10", Valid: true}},
		},
	}

	// calculate delta with ASC sort
	got, err := calculateDelta(curr, prev, 1, [2]int{1, 3}, 1, false, 0)
	assert.NoError(t, err)
	assert.Equal(t, wantAsc, got)

	// calculate delta with DESC sort
	got, err = calculateDelta(curr, prev, 1, [2]int{1, 3}, 1, true, 0)
	assert.NoError(t, err)
	assert.Equal(t, wantDesc, got)

	// calculate delta with zero diff-interval, just return current value
	got, err = calculateDelta(curr, prev, 1, [2]int{0, 0}, 1, true, 0)
	assert.NoError(t, err)
	assert.Equal(t, curr, got)

	// calculate with invalid input data
	_, err = calculateDelta(currInvalid, prev, 1, [2]int{1, 3}, 1, true, 0)
	assert.Error(t, err)
}

func Test_diff(t *testing.T) {
	prev := PGresult{
		Valid: true, Ncols: 4, Nrows: 4, Cols: []string{"unique", "col2", "col3", "col4"},
		Values: [][]sql.NullString{
			{{String: "1", Valid: true}, {String: "300", Valid: true}, {String: "100", Valid: true}, {String: "500", Valid: true}},
			{{String: "2", Valid: true}, {String: "400", Valid: true}, {String: "200", Valid: true}, {String: "600", Valid: true}},
			{{String: "3", Valid: true}, {String: "100.0", Valid: true}, {String: "300", Valid: true}, {String: "700", Valid: true}},
			{{String: "4", Valid: true}, {String: "200", Valid: true}, {String: "400.0", Valid: true}, {String: "800", Valid: true}},
			// next row is not present in 'curr' and should be skipped.
			{{String: "5", Valid: true}, {String: "200", Valid: true}, {String: "400.0", Valid: true}, {String: "800", Valid: true}},
		},
	}
	curr := PGresult{
		Valid: true, Ncols: 4, Nrows: 5, Cols: []string{"unique", "col2", "col3", "col4"},
		Values: [][]sql.NullString{
			{{String: "1", Valid: true}, {String: "330.5", Valid: true}, {String: "150", Valid: true}, {String: "500", Valid: true}},
			{{String: "2", Valid: true}, {String: "440", Valid: true}, {String: "280.6", Valid: true}, {String: "620", Valid: true}},
			{{String: "3", Valid: true}, {String: "110", Valid: true}, {String: "300", Valid: true}, {String: "710", Valid: true}},
			{{String: "4", Valid: true}, {String: "220", Valid: true}, {String: "490", Valid: true}, {String: "800", Valid: true}},
			// next row is not present in 'prev' and should be added as-is to 'diff' result.
			{{String: "6", Valid: true}, {String: "560", Valid: true}, {String: "510", Valid: true}, {String: "920", Valid: true}},
		},
	}
	want := PGresult{
		Valid: true, Ncols: 4, Nrows: 5, Cols: []string{"unique", "col2", "col3", "col4"},
		Values: [][]sql.NullString{
			{{String: "1", Valid: true}, {String: "30.50", Valid: true}, {String: "50", Valid: true}, {String: "0", Valid: true}},
			{{String: "2", Valid: true}, {String: "40", Valid: true}, {String: "80.60", Valid: true}, {String: "20", Valid: true}},
			{{String: "3", Valid: true}, {String: "10.00", Valid: true}, {String: "0", Valid: true}, {String: "10", Valid: true}},
			{{String: "4", Valid: true}, {String: "20", Valid: true}, {String: "90.00", Valid: true}, {String: "0", Valid: true}},
			{{String: "6", Valid: true}, {String: "560", Valid: true}, {String: "510", Valid: true}, {String: "920", Valid: true}},
		},
	}

	got, err := diff(curr, prev, 1, [2]int{1, 3}, 0)
	assert.NoError(t, err)
	assert.Equal(t, want, got)

	prevValid := PGresult{
		Valid: true, Ncols: 2, Nrows: 1, Cols: []string{"unique", "col2"},
		Values: [][]sql.NullString{
			{{String: "1", Valid: true}, {String: "300", Valid: true}},
		},
	}
	currInvalid := PGresult{
		Valid: true, Ncols: 2, Nrows: 1, Cols: []string{"unique", "col2"},
		Values: [][]sql.NullString{
			{{String: "1", Valid: true}, {String: "invalid", Valid: true}},
		},
	}

	_, err = diff(currInvalid, prevValid, 1, [2]int{1, 3}, 0)
	assert.Error(t, err)
}

// Test_DiffZeroFilledCells locks the behavioral half of tech-debt [007]: pg_stat_io and
// pg_replication_slots NULL cells are coalesced to "0" in SQL before recording, so the recorder
// always stores "0" (never "") for those counters. This test feeds those coalesced-"0" cells
// through diff() (and the public Compare wrapper) and proves they produce clean "0" deltas
// instead of aborting the whole sample — the failure mode an empty "" cell would trigger via
// ParseInt(""). It also proves rows are paired by a synthetic io_key-style UniqueKey (col 0),
// not by position, and that a mixed row (a coalesced-"0" cell alongside a normal cumulative
// counter) diffs each cell correctly.
//
// Test name carries capital "Diff" so the verify mask `-run Diff` selects it. NOTE: Go's
// `-run` is case-sensitive, so use `-run '[Dd]iff'` to also re-run the sibling Test_diff* as
// regression coverage.
func Test_DiffZeroFilledCells(t *testing.T) {
	// io_key-style layout: col 0 is the stable string UniqueKey, col 1 is a text label copied
	// as-is (like pg_stat_io.object), cols 2-3 are diffed cumulative counters. The recorder
	// stored "0" for NULL-after-coalesce cells.
	// prev rows are listed in OPPOSITE order from curr to prove pairing is by UniqueKey, not by
	// row position.
	prev := PGresult{
		Valid: true, Ncols: 4, Nrows: 3, Cols: []string{"io_key", "object", "reads", "hits"},
		Values: [][]sql.NullString{
			// b2: physical-slot-style row — every diffed counter is a coalesced "0".
			{{String: "b2", Valid: true}, {String: "wal", Valid: true}, {String: "0", Valid: true}, {String: "0", Valid: true}},
			// a1: mixed row — one coalesced "0" cell, one normal cumulative counter.
			{{String: "a1", Valid: true}, {String: "client backend", Valid: true}, {String: "0", Valid: true}, {String: "100", Valid: true}},
			// c3: row absent from curr, must be skipped.
			{{String: "c3", Valid: true}, {String: "bgwriter", Valid: true}, {String: "0", Valid: true}, {String: "0", Valid: true}},
		},
	}
	curr := PGresult{
		Valid: true, Ncols: 4, Nrows: 3, Cols: []string{"io_key", "object", "reads", "hits"},
		Values: [][]sql.NullString{
			{{String: "a1", Valid: true}, {String: "client backend", Valid: true}, {String: "0", Valid: true}, {String: "150", Valid: true}},
			{{String: "b2", Valid: true}, {String: "wal", Valid: true}, {String: "0", Valid: true}, {String: "0", Valid: true}},
			// d4: row absent from prev, must be copied as-is (coalesced "0" cells passed through).
			{{String: "d4", Valid: true}, {String: "autovacuum", Valid: true}, {String: "0", Valid: true}, {String: "0", Valid: true}},
		},
	}
	// interval [2,3] covers the cumulative counters; UniqueKey (0) and the text label (1) are
	// outside it and copied as-is.
	want := PGresult{
		Valid: true, Ncols: 4, Nrows: 3, Cols: []string{"io_key", "object", "reads", "hits"},
		Values: [][]sql.NullString{
			// a1 mixed: coalesced "0" reads → "0"; counter hits 150-100 → "50".
			{{String: "a1", Valid: true}, {String: "client backend", Valid: true}, {String: "0", Valid: true}, {String: "50", Valid: true}},
			// b2 all-zero: both coalesced "0" counters → clean "0" deltas, no abort.
			{{String: "b2", Valid: true}, {String: "wal", Valid: true}, {String: "0", Valid: true}, {String: "0", Valid: true}},
			// d4 not in prev: copied as-is.
			{{String: "d4", Valid: true}, {String: "autovacuum", Valid: true}, {String: "0", Valid: true}, {String: "0", Valid: true}},
		},
	}

	got, err := diff(curr, prev, 1, [2]int{2, 3}, 0)
	assert.NoError(t, err, "coalesced-\"0\" cells must diff cleanly, not abort the sample")
	assert.Equal(t, want, got)

	// Compare is the public wrapper report uses (countDiff → Compare → diff). Assert the same
	// zero-cell contract holds through it, and that its sort() step actually runs: sort DESC on
	// the io_key column (skey=0, desc=true) must reorder the diff rows to d4,b2,a1.
	gotCompare, err := Compare(curr, prev, 1, [2]int{2, 3}, 0, true, 0)
	assert.NoError(t, err)
	assert.Equal(t, []string{"d4", "b2", "a1"},
		[]string{gotCompare.Values[0][0].String, gotCompare.Values[1][0].String, gotCompare.Values[2][0].String},
		"Compare must apply DESC sort on io_key after diffing")
	// Same coalesced-"0" / mixed-counter deltas survive the wrapper (a1: reads 0, hits 50).
	assert.Equal(t, "0", gotCompare.Values[2][2].String)
	assert.Equal(t, "50", gotCompare.Values[2][3].String)

	// Demonstrate WHY coalesce is required: an empty "" in-interval cell (a raw NULL serialized
	// without coalesce) reaches ParseInt("") and aborts the entire sample with an error.
	prevEmpty := PGresult{
		Valid: true, Ncols: 4, Nrows: 1, Cols: []string{"io_key", "object", "reads", "hits"},
		Values: [][]sql.NullString{
			{{String: "a1", Valid: true}, {String: "client backend", Valid: true}, {String: "", Valid: true}, {String: "100", Valid: true}},
		},
	}
	currEmpty := PGresult{
		Valid: true, Ncols: 4, Nrows: 1, Cols: []string{"io_key", "object", "reads", "hits"},
		Values: [][]sql.NullString{
			{{String: "a1", Valid: true}, {String: "client backend", Valid: true}, {String: "", Valid: true}, {String: "150", Valid: true}},
		},
	}
	_, err = diff(currEmpty, prevEmpty, 1, [2]int{2, 3}, 0)
	// Bind the error to the empty cell specifically (parsePairInt wraps ParseInt as
	// "convert '%s' to int failed") so this documents the exact NULL-after-serialization
	// failure mode the [007] coalesce prevents, not a generic parse error.
	assert.ErrorContains(t, err, "convert '' to int failed", "empty in-interval cell (uncoalesced NULL) must error — the bug [007] coalesce prevents")
}

// Test_diff_pg18_wal_stats_age reproduces issue #132: when showing WAL statistics on PG 18,
// the stats_age column ('19 days 02:52:00') was incorrectly included in the diff interval
// and caused a parse error. The correct DiffIntvl for PG 18 WAL view is [2, 5].
func Test_diff_pg18_wal_stats_age(t *testing.T) {
	// PG 18 pg_stat_wal columns: source, waldir_size, wal_KiB, records, fpi, buffers_full, stats_age
	prev := PGresult{
		Valid: true, Ncols: 7, Nrows: 1,
		Cols: []string{"source", "waldir_size", "wal,KiB", "records", "fpi", "buffers_full", "stats_age"},
		Values: [][]sql.NullString{
			{
				{String: "WAL", Valid: true},
				{String: "64 MB", Valid: true},
				{String: "12000.00", Valid: true},
				{String: "1000", Valid: true},
				{String: "50", Valid: true},
				{String: "10", Valid: true},
				{String: "19 days 02:52:00", Valid: true},
			},
		},
	}
	curr := PGresult{
		Valid: true, Ncols: 7, Nrows: 1,
		Cols: []string{"source", "waldir_size", "wal,KiB", "records", "fpi", "buffers_full", "stats_age"},
		Values: [][]sql.NullString{
			{
				{String: "WAL", Valid: true},
				{String: "64 MB", Valid: true},
				{String: "12100.00", Valid: true},
				{String: "1010", Valid: true},
				{String: "55", Valid: true},
				{String: "11", Valid: true},
				{String: "19 days 03:01:38", Valid: true},
			},
		},
	}

	// Old (buggy) interval [2, 9]: stats_age at col 6 is inside the diff range and causes parse error.
	_, err := diff(curr, prev, 1, [2]int{2, 9}, 0)
	assert.Error(t, err, "stats_age should not be diffable with interval [2,9] on PG18 WAL data")

	// Correct interval [2, 5]: stats_age at col 6 is outside the diff range.
	got, err := diff(curr, prev, 1, [2]int{2, 5}, 0)
	assert.NoError(t, err)
	assert.Equal(t, "19 days 03:01:38", got.Values[0][6].String, "stats_age should be copied as-is")
	assert.Equal(t, "100.00", got.Values[0][2].String, "wal,KiB delta should be computed")
}

func Test_sort(t *testing.T) {
	res := newTestPGresult()
	testcases := []struct {
		name string
		key  int
		desc bool
		want [][]sql.NullString
	}{
		{
			name: "numeric asc", key: 0, desc: false,
			want: [][]sql.NullString{
				{{String: "1", Valid: true}, {String: "bronze", Valid: true}, {String: "row1:value3", Valid: true}, {String: "row1:value4", Valid: true}},
				{{String: "2", Valid: true}, {String: "cilla", Valid: true}, {String: "row2:value3", Valid: true}, {String: "row2:value4", Valid: true}},
				{{String: "3", Valid: true}, {String: "direct", Valid: true}, {String: "row3:value3", Valid: true}, {String: "row3:value4", Valid: true}},
				{{String: "4", Valid: true}, {String: "arktika", Valid: true}, {String: "row3:value3", Valid: true}, {String: "row4:value4", Valid: true}},
				{{String: "15", Valid: true}, {String: "evioni", Valid: true}, {String: "row5:value3", Valid: true}, {String: "row2:value4", Valid: true}},
				{{String: "248", Valid: true}, {String: "brodsky", Valid: true}, {String: "row6:value3", Valid: true}, {String: "row6:value4", Valid: true}},
				{{String: "3987", Valid: true}, {String: "fasivy", Valid: true}, {String: "row7:value3", Valid: true}, {String: "row7:value4", Valid: true}},
				{{String: "48752", Valid: true}, {String: "aalfia", Valid: true}, {String: "row8:value3", Valid: true}, {String: "row8:value4", Valid: true}},
			},
		},
		{
			name: "numeric desc", key: 0, desc: true,
			want: [][]sql.NullString{
				{{String: "48752", Valid: true}, {String: "aalfia", Valid: true}, {String: "row8:value3", Valid: true}, {String: "row8:value4", Valid: true}},
				{{String: "3987", Valid: true}, {String: "fasivy", Valid: true}, {String: "row7:value3", Valid: true}, {String: "row7:value4", Valid: true}},
				{{String: "248", Valid: true}, {String: "brodsky", Valid: true}, {String: "row6:value3", Valid: true}, {String: "row6:value4", Valid: true}},
				{{String: "15", Valid: true}, {String: "evioni", Valid: true}, {String: "row5:value3", Valid: true}, {String: "row2:value4", Valid: true}},
				{{String: "4", Valid: true}, {String: "arktika", Valid: true}, {String: "row3:value3", Valid: true}, {String: "row4:value4", Valid: true}},
				{{String: "3", Valid: true}, {String: "direct", Valid: true}, {String: "row3:value3", Valid: true}, {String: "row3:value4", Valid: true}},
				{{String: "2", Valid: true}, {String: "cilla", Valid: true}, {String: "row2:value3", Valid: true}, {String: "row2:value4", Valid: true}},
				{{String: "1", Valid: true}, {String: "bronze", Valid: true}, {String: "row1:value3", Valid: true}, {String: "row1:value4", Valid: true}},
			},
		},
		{
			name: "string asc", key: 1, desc: false,
			want: [][]sql.NullString{
				{{String: "48752", Valid: true}, {String: "aalfia", Valid: true}, {String: "row8:value3", Valid: true}, {String: "row8:value4", Valid: true}},
				{{String: "4", Valid: true}, {String: "arktika", Valid: true}, {String: "row3:value3", Valid: true}, {String: "row4:value4", Valid: true}},
				{{String: "248", Valid: true}, {String: "brodsky", Valid: true}, {String: "row6:value3", Valid: true}, {String: "row6:value4", Valid: true}},
				{{String: "1", Valid: true}, {String: "bronze", Valid: true}, {String: "row1:value3", Valid: true}, {String: "row1:value4", Valid: true}},
				{{String: "2", Valid: true}, {String: "cilla", Valid: true}, {String: "row2:value3", Valid: true}, {String: "row2:value4", Valid: true}},
				{{String: "3", Valid: true}, {String: "direct", Valid: true}, {String: "row3:value3", Valid: true}, {String: "row3:value4", Valid: true}},
				{{String: "15", Valid: true}, {String: "evioni", Valid: true}, {String: "row5:value3", Valid: true}, {String: "row2:value4", Valid: true}},
				{{String: "3987", Valid: true}, {String: "fasivy", Valid: true}, {String: "row7:value3", Valid: true}, {String: "row7:value4", Valid: true}},
			},
		},
		{
			name: "string desc", key: 1, desc: true,
			want: [][]sql.NullString{
				{{String: "3987", Valid: true}, {String: "fasivy", Valid: true}, {String: "row7:value3", Valid: true}, {String: "row7:value4", Valid: true}},
				{{String: "15", Valid: true}, {String: "evioni", Valid: true}, {String: "row5:value3", Valid: true}, {String: "row2:value4", Valid: true}},
				{{String: "3", Valid: true}, {String: "direct", Valid: true}, {String: "row3:value3", Valid: true}, {String: "row3:value4", Valid: true}},
				{{String: "2", Valid: true}, {String: "cilla", Valid: true}, {String: "row2:value3", Valid: true}, {String: "row2:value4", Valid: true}},
				{{String: "1", Valid: true}, {String: "bronze", Valid: true}, {String: "row1:value3", Valid: true}, {String: "row1:value4", Valid: true}},
				{{String: "248", Valid: true}, {String: "brodsky", Valid: true}, {String: "row6:value3", Valid: true}, {String: "row6:value4", Valid: true}},
				{{String: "4", Valid: true}, {String: "arktika", Valid: true}, {String: "row3:value3", Valid: true}, {String: "row4:value4", Valid: true}},
				{{String: "48752", Valid: true}, {String: "aalfia", Valid: true}, {String: "row8:value3", Valid: true}, {String: "row8:value4", Valid: true}},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			res.sort(tc.key, tc.desc)
			assert.Equal(t, tc.want, res.Values)
		})
	}

	// test sorting of empty PGresult.
	emptyRes := PGresult{Valid: true, Ncols: 1, Nrows: 0, Cols: []string{"col1"}, Values: [][]sql.NullString{}}
	emptyRes.sort(0, false)
	assert.Equal(t, emptyRes.Values, [][]sql.NullString{})
}

// Test_sort_duration reproduces issue #50: time columns like "791:04:45" were sorted
// as strings, placing them between "79:..." values instead of at the top.
// String comparison: "791..." < "79:..." because '1' (49) < ':' (58).
func Test_sort_duration(t *testing.T) {
	ns := func(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

	// Reproduces the exact scenario from the issue report (desc sort by t_all_t).
	// Without duration-aware sort "791:04:45" would land between "79:18:40" and "77:20:25".
	res := PGresult{
		Valid: true, Ncols: 2, Nrows: 6,
		Cols: []string{"t_all_t", "query"},
		Values: [][]sql.NullString{
			{ns("96:58:35"), ns("q1")},
			{ns("80:39:06"), ns("q2")},
			{ns("79:18:40"), ns("q3")},
			{ns("791:04:45"), ns("q4")}, // 791 hours — must sort above all others
			{ns("74:42:21"), ns("q5")},
			{ns("00:05:23"), ns("q6")},
		},
	}

	res.sort(0, true) // descending by t_all_t
	want := []string{"791:04:45", "96:58:35", "80:39:06", "79:18:40", "74:42:21", "00:05:23"}
	got := make([]string, len(res.Values))
	for i, row := range res.Values {
		got[i] = row[0].String
	}
	assert.Equal(t, want, got)

	res.sort(0, false) // ascending
	wantAsc := []string{"00:05:23", "74:42:21", "79:18:40", "80:39:06", "96:58:35", "791:04:45"}
	gotAsc := make([]string, len(res.Values))
	for i, row := range res.Values {
		gotAsc[i] = row[0].String
	}
	assert.Equal(t, wantAsc, gotAsc)
}

func Test_parseDuration(t *testing.T) {
	testcases := []struct {
		input string
		want  int64
		valid bool
	}{
		{"00:00:00", 0, true},
		{"00:05:23", 323, true},
		{"01:00:00", 3600, true},
		{"96:58:35", 349115, true},
		{"791:04:45", 2847885, true}, // the value from issue #50
		{"1 day 00:00:00", 86400, true},
		{"11 days 10:10:10", 987010, true},
		{"2 days 03:30:45", 185445, true},
		{"invalid", 0, false},
		{"10:20", 0, false}, // missing seconds — not a valid HH:MM:SS
		{"abc:de:fg", 0, false},
	}

	for _, tc := range testcases {
		got, err := parseDuration(tc.input)
		if tc.valid {
			assert.NoError(t, err, "input: %q", tc.input)
			assert.Equal(t, tc.want, got, "input: %q", tc.input)
		} else {
			assert.Error(t, err, "input: %q", tc.input)
		}
	}
}

func Test_diffPair(t *testing.T) {
	testcases := []struct {
		valid bool
		curr  string
		prev  string
		want  string
	}{
		{valid: true, curr: "100", prev: "10", want: "90"},
		{valid: false, curr: "100", prev: ""},
		{valid: true, curr: "100", prev: "55.55", want: "44.45"},
		{valid: true, curr: "44.45", prev: "0", want: "44.45"},
		{valid: true, curr: "1.23456e+05", prev: "100000", want: "23456.00"},
		{valid: true, curr: "100000", prev: "1.23456e+05", want: "-23456.00"},
		{valid: false, curr: "invalid", prev: "1.23456e+05"},
	}

	for _, tc := range testcases {
		got, err := diffPair(tc.curr, tc.prev, 1)
		if tc.valid {
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		} else {
			assert.Error(t, err)
		}
	}
}

func Test_parsePairFloat(t *testing.T) {
	testcases := []struct {
		valid bool
		curr  string
		prev  string
		c     float64
		p     float64
	}{
		{valid: true, curr: "123.456", prev: "654.321", c: 123.456, p: 654.321},
		{valid: true, curr: "1.23456e+05", prev: "6.54321e-01", c: 123456, p: 0.654321},
		{valid: false, curr: "123.456", prev: "invalid"},
		{valid: false, curr: "invalid", prev: "123.456"},
		{valid: false, curr: "123.456", prev: ""},
		{valid: false, curr: "", prev: "123.456"},
	}

	for _, tc := range testcases {
		c, p, err := parsePairFloat(tc.curr, tc.prev)
		if tc.valid {
			assert.NoError(t, err)
			assert.Equal(t, tc.c, c)
			assert.Equal(t, tc.p, p)
		} else {
			assert.Error(t, err)
		}
	}
}

func Test_parsePairInt(t *testing.T) {
	testcases := []struct {
		valid bool
		curr  string
		prev  string
		c     int64
		p     int64
	}{
		{valid: true, curr: "123456", prev: "654321", c: 123456, p: 654321},
		{valid: false, curr: "123456", prev: "invalid"},
		{valid: false, curr: "invalid", prev: "123456"},
		{valid: false, curr: "123456", prev: ""},
		{valid: false, curr: "", prev: "123456"},
	}

	for _, tc := range testcases {
		c, p, err := parsePairInt(tc.curr, tc.prev)
		if tc.valid {
			assert.NoError(t, err)
			assert.Equal(t, tc.c, c)
			assert.Equal(t, tc.p, p)
		} else {
			assert.Error(t, err)
		}
	}
}

func TestPGresult_Fprint(t *testing.T) {
	res := newTestPGresult()

	var buf bytes.Buffer
	err := res.Fprint(&buf)
	assert.NoError(t, err)
	assert.Greater(t, len(buf.String()), 0)
	for i := 1; i <= res.Ncols; i++ {
		assert.Contains(t, buf.String(), fmt.Sprintf("row%d:value4", i))
	}
}

func Test_extensionSchema(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	// test with proper connection
	assert.Equal(t, "pg_catalog", extensionSchema(conn, "plpgsql"))
	assert.Equal(t, "", extensionSchema(conn, "unknown"))

	// test with already closed connection
	conn.Close()
	assert.Equal(t, "", extensionSchema(conn, "plpgsql"))
}

func Test_isSchemaExists(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	// test with proper connection
	assert.True(t, isSchemaExists(conn, "public"))
	assert.False(t, isSchemaExists(conn, "unknown"))

	// test with already closed connection
	conn.Close()
	assert.False(t, isSchemaExists(conn, "public"))
}
