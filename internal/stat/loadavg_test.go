package stat

import (
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_readLoadAverage(t *testing.T) {
	conn, err := postgres.TestConnect()
	assert.NoError(t, err)

	// test "local" reading
	conn.Local = true
	got, err := readLoadAverage(conn, false)
	assert.NoError(t, err)
	assert.Greater(t, got.One, float64(0))

	// test "remote" reading
	conn.Local = false
	got, err = readLoadAverage(conn, true)
	assert.NoError(t, err)
	assert.Greater(t, got.One, float64(0))

	// test "remote", but when schema is not available
	got, err = readLoadAverage(conn, false)
	assert.NoError(t, err)
	assert.Equal(t, got.One, float64(0))
}

func Test_readLoadAverageLocal(t *testing.T) {
	testcases := []struct {
		statfile string
		valid    bool
		want     LoadAvg
	}{
		{statfile: "testdata/proc/loadavg.golden", valid: true, want: LoadAvg{One: 2.43, Five: 2.30, Fifteen: 1.74}},
		{statfile: "testdata/proc/loadavg.invalid", valid: false},
	}

	for _, tc := range testcases {
		got, err := readLoadAverageLocal(tc.statfile)
		if tc.valid {
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		} else {
			assert.Error(t, err)
		}
	}
}

func Test_readLoadAverageRemote(t *testing.T) {
	conn, err := postgres.TestConnect()
	assert.NoError(t, err)

	got, err := readLoadAverageRemote(conn)
	assert.NoError(t, err)
	assert.Greater(t, got.One, float64(0))
	assert.Greater(t, got.Five, float64(0))
	assert.Greater(t, got.Fifteen, float64(0))

	conn.Close()
	_, err = readLoadAverageRemote(conn)
	assert.Error(t, err)
}
