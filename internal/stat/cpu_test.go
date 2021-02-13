package stat

import (
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_readCpuStat(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)
	defer conn.Close()

	// test "local" reading
	conn.Local = true
	got, err := readCpuStat(conn, false)
	assert.NoError(t, err)
	assert.Greater(t, got.Total, float64(0))

	// test "remote" reading
	conn.Local = false
	got, err = readCpuStat(conn, true)
	assert.NoError(t, err)
	assert.Greater(t, got.Total, float64(0))

	// test "remote", but when schema is not available
	got, err = readCpuStat(conn, false)
	assert.NoError(t, err)
	assert.Equal(t, got.Total, float64(0))
}

func Test_readCpuStatLocal(t *testing.T) {
	testcases := []struct {
		statfile string
		valid    bool
		want     CpuStat
	}{
		{
			statfile: "testdata/proc/stat.golden",
			valid:    true,
			want: CpuStat{
				Entry:   "cpu",
				User:    3097668,
				Nice:    1593,
				Sys:     1419618,
				Idle:    132242258,
				Iowait:  42535,
				Irq:     0,
				Softirq: 384686,
				Steal:   0,
				Guest:   0,
				GstNice: 0,
				Total:   137188358,
			},
		},
		{statfile: "testdata/proc/stat.unknown", valid: false},
	}

	for _, tc := range testcases {
		got, err := readCpuStatLocal(tc.statfile)
		if tc.valid {
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		} else {
			assert.Error(t, err)
		}
	}
}

func Test_readCpuStatRemote(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	got, err := readCpuStatRemote(conn)
	assert.NoError(t, err)
	assert.Greater(t, got.Total, float64(0))
	assert.Greater(t, got.User, float64(0))
	assert.Greater(t, got.Sys, float64(0))

	conn.Close()
	_, err = readCpuStatRemote(conn)
	assert.Error(t, err)
}

func Test_countCpuUsage(t *testing.T) {
	prev, err := readCpuStatLocal("testdata/proc/stat.golden")
	assert.NoError(t, err)

	curr, err := readCpuStatLocal("testdata/proc/stat2.golden")
	assert.NoError(t, err)

	got := countCpuUsage(prev, curr, 100)

	want := CpuStat{
		Entry:   "",
		User:    16.666666666666664,
		Nice:    16.666666666666664,
		Sys:     16.666666666666664,
		Idle:    16.666666666666664,
		Iowait:  16.666666666666664,
		Irq:     0,
		Softirq: 16.666666666666664,
		Steal:   0,
		Guest:   0,
		GstNice: 0,
		Total:   0,
	}

	assert.Equal(t, want, got)
}
