package stat

import (
	"context"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func Test_parseProcMounts(t *testing.T) {
	file, err := os.Open(filepath.Clean("testdata/proc/mounts.golden"))
	assert.NoError(t, err)
	defer func() { _ = file.Close() }()

	stats, err := parseProcMounts(file)
	assert.NoError(t, err)

	want := []Mount{
		{Device: "/dev/mapper/ssd-root", Mountpoint: "/", Fstype: "ext4", Options: "rw,relatime,discard,errors=remount-ro"},
		{Device: "/dev/sda1", Mountpoint: "/boot", Fstype: "ext3", Options: "rw,relatime"},
		{Device: "/dev/mapper/ssd-data", Mountpoint: "/data", Fstype: "ext4", Options: "rw,relatime,discard"},
		{Device: "/dev/sdc1", Mountpoint: "/archive", Fstype: "xfs", Options: "rw,relatime"},
	}

	assert.Equal(t, want, stats)

	// test with wrong format file
	file, err = os.Open(filepath.Clean("testdata/proc/netdev.v1.golden"))
	assert.NoError(t, err)
	defer func() { _ = file.Close() }()

	stats, err = parseProcMounts(file)
	assert.Error(t, err)
	assert.Nil(t, stats)
}

func Test_readFilesystemStatsLocal(t *testing.T) {
	got, err := readFilesystemStatsLocal("/proc/mounts")
	assert.NoError(t, err)
	assert.NotNil(t, got)
	assert.Greater(t, len(got), 0)
}

func Test_parseFilesystemStats(t *testing.T) {
	file, err := os.Open(filepath.Clean("testdata/proc/mounts.golden"))
	assert.NoError(t, err)

	stats, err := parseFilesystemStats(file)
	assert.NoError(t, err)
	assert.Greater(t, len(stats), 1)
	assert.Greater(t, stats[0].Size, float64(0))
	assert.Greater(t, stats[0].Free, float64(0))
	assert.Greater(t, stats[0].Avail, float64(0))
	assert.Greater(t, stats[0].Used, float64(0))
	assert.Greater(t, stats[0].Reserved, float64(0))
	assert.Greater(t, stats[0].Pused, float64(0))
	assert.Greater(t, stats[0].Files, float64(0))
	assert.Greater(t, stats[0].Filesfree, float64(0))
	assert.Greater(t, stats[0].Filesused, float64(0))
	assert.Greater(t, stats[0].Filespused, float64(0))

	_ = file.Close()

	// test with wrong format file
	file, err = os.Open(filepath.Clean("testdata/proc/netdev.v1.golden"))
	assert.NoError(t, err)

	stats, err = parseFilesystemStats(file)
	assert.Error(t, err)
	assert.Nil(t, stats)
	_ = file.Close()
}

func Test_readMountpointStat(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	wg := sync.WaitGroup{}
	ch := make(chan Fsstat)

	wg.Add(1)
	go readMountpointStat("/", ch, &wg)

	select {
	case response := <-ch:
		assert.Greater(t, response.Size, float64(0))
		assert.Greater(t, response.Free, float64(0))
		assert.Greater(t, response.Avail, float64(0))
		assert.Greater(t, response.Used, float64(0))
		assert.Greater(t, response.Reserved, float64(0))
		assert.Greater(t, response.Pused, float64(0))
		assert.Greater(t, response.Files, float64(0))
		assert.Greater(t, response.Filesfree, float64(0))
		assert.Greater(t, response.Filesused, float64(0))
		assert.Greater(t, response.Filespused, float64(0))
	case <-ctx.Done():
		assert.Fail(t, "context cancelled: ", ctx.Err())
	}

	wg.Wait()
	close(ch)
}

func Test_readFilesystemStatsRemote(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	got, err := readFilesystemStatsRemote(conn)
	assert.NoError(t, err)
	assert.Greater(t, len(got), 0)

	// Check device value is not empty
	for i := range got {
		assert.NotEqual(t, got[i].Mount.Device, "")
	}

	conn.Close()
	_, err = readFilesystemStatsRemote(conn)
	assert.Error(t, err)
}
