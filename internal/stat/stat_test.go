package stat

import (
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/view"
	"github.com/stretchr/testify/assert"
	"regexp"
	"testing"
	"time"
)

func TestNewCollector(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	c, err := NewCollector(conn)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	conn.Close()
	c, err = NewCollector(conn)
	assert.Error(t, err)
	assert.Nil(t, c)
}

func TestCollector_Update(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	props, err := GetPostgresProperties(conn)
	assert.NoError(t, err)

	views := view.Views{
		"activity": {
			Name:      "activity",
			QueryTmpl: query.PgStatActivityQueryDefault,
			DiffIntvl: [2]int{0, 0},
			Ncols:     14,
			OrderKey:  0,
			OrderDesc: true,
			ColsWidth: map[int]int{},
			Msg:       "Show activity statistics",
			Filters:   map[int]*regexp.Regexp{},
			Refresh:   1 * time.Second,
		},
	}
	opts := query.Options{}

	views.Configure(props.VersionNum, props.GucTrackCommitTimestamp)
	opts.Adjust(props.VersionNum, props.Recovery, "top")
	for k, v := range views {
		q, err := query.PrepareQuery(views["activity"].QueryTmpl, opts)
		assert.NoError(t, err)
		v.Query = q
		views[k] = v
	}

	c, err := NewCollector(conn)
	assert.NoError(t, err)
	assert.NotNil(t, c)
	c.config.collectExtra = CollectDiskstats

	stat, err := c.Update(conn, views["activity"])
	assert.NoError(t, err)
	assert.NotNil(t, stat)

	assert.NotEqual(t, float64(0), stat.System.LoadAvg.One)
	assert.NotEqual(t, float64(0), stat.System.Meminfo.MemUsed)
	assert.NotEqual(t, float64(0), stat.System.CpuStat.User)
	assert.NotEqual(t, 0, len(stat.System.Diskstats))
	assert.NotEqual(t, float64(0), stat.Pgstat.Activity.ConnTotal)
	assert.True(t, stat.Pgstat.Result.Valid)
	assert.NotEqual(t, 0, len(stat.Pgstat.Result.Values))
	assert.NotEqual(t, 0, len(stat.Pgstat.Result.Cols))
}

func TestCollector_collectDiskstats(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	c, err := NewCollector(conn)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	diskstats, err := c.collectDiskstats(conn)
	assert.NoError(t, err)
	assert.NotNil(t, diskstats)
	assert.Greater(t, len(diskstats), 0)
}

func TestCollector_collectNetdevs(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	c, err := NewCollector(conn)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	netdevs, err := c.collectNetdevs(conn)
	assert.NoError(t, err)
	assert.NotNil(t, netdevs)
	assert.Greater(t, len(netdevs), 0)
}

func Test_readUptimeLocal(t *testing.T) {
	ticks, err := getSysticksLocal()
	assert.NoError(t, err)
	assert.NotEqual(t, float64(0), ticks)

	got, err := readUptimeLocal("testdata/proc/uptime.golden", ticks)
	assert.NoError(t, err)
	assert.Equal(t, float64(170191868), got)

	_, err = readUptimeLocal("testdata/proc/stat.golden", ticks)
	assert.Error(t, err)
}

func Test_getSysticksLocal(t *testing.T) {
	ticks, err := getSysticksLocal()
	assert.NoError(t, err)
	assert.NotEqual(t, float64(0), ticks)
}

func Test_sValue(t *testing.T) {
	testcases := []struct {
		prev  float64
		curr  float64
		itv   float64
		ticks float64
		want  float64
	}{
		{prev: 1000, curr: 2000, itv: 100, ticks: 100, want: 1000}, // delta 1000 per second within 1 second
		{prev: 1000, curr: 5000, itv: 100, ticks: 100, want: 4000}, // delta 4000 per second within 1 second
		{prev: 1000, curr: 5000, itv: 400, ticks: 100, want: 1000}, // delta 1000 per second within 4 second
		{prev: 2000, curr: 1000, want: 0},                          // nothing, current less than previous
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, sValue(tc.prev, tc.curr, tc.itv, tc.ticks))
	}
}
