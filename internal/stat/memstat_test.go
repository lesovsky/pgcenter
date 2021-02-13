package stat

import (
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_readMeminfo(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)
	defer conn.Close()

	// test "local" reading
	conn.Local = true
	got, err := readMeminfo(conn, false)
	assert.NoError(t, err)
	assert.Greater(t, got.MemTotal, uint64(0))

	// test "remote" reading
	conn.Local = false
	got, err = readMeminfo(conn, true)
	assert.NoError(t, err)
	assert.Greater(t, got.MemTotal, uint64(0))

	// test "remote", but when schema is not available
	got, err = readMeminfo(conn, false)
	assert.NoError(t, err)
	assert.Equal(t, got.MemTotal, uint64(0))
}

func Test_readMeminfoLocal(t *testing.T) {
	testcases := []struct {
		statfile string
		valid    bool
		want     Meminfo
	}{
		{statfile: "testdata/proc/meminfo.golden", valid: true, want: Meminfo{
			MemTotal: 32069, MemFree: 21064, MemUsed: 5481,
			SwapTotal: 16383, SwapFree: 16383, SwapUsed: 0,
			MemCached: 4259, MemBuffers: 589, MemDirty: 35, MemWriteback: 0, MemSlab: 676,
		}},
		{statfile: "testdata/proc/meminfo.unknown", valid: false},
	}

	for _, tc := range testcases {
		got, err := readMeminfoLocal(tc.statfile)
		if tc.valid {
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		} else {
			assert.Error(t, err)
		}
	}
}

func Test_readMeminfoRemote(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	got, err := readMeminfoRemote(conn)
	assert.NoError(t, err)
	assert.Greater(t, got.MemTotal, uint64(0))
	assert.Greater(t, got.MemCached, uint64(0))
	assert.Greater(t, got.MemUsed, uint64(0))

	conn.Close()
	_, err = readMeminfoRemote(conn)
	assert.Error(t, err)
}
