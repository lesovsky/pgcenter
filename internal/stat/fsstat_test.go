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

// The task-08 TDD anchors name these tests Test_matchDataDirFs_* (unexported matcher). The matcher
// is exported as MatchDataDirFs because the verbose composer lives in package top and calls it
// across the package boundary, so the tests follow the exported name. Coverage is identical.

// Test_MatchDataDirFs_longestPrefix verifies the longest-mount-prefix-wins rule: when both "/"
// and "/var/lib/pgsql" are valid prefixes of the data_directory, the more specific mount wins.
// local=false to skip EvalSymlinks (the test path does not exist on disk).
func Test_MatchDataDirFs_longestPrefix(t *testing.T) {
	fss := Fsstats{
		{Mount: Mount{Device: "/dev/root", Mountpoint: "/", Fstype: "ext4"}},
		{Mount: Mount{Device: "/dev/data", Mountpoint: "/var/lib/pgsql", Fstype: "xfs"}},
	}

	got, ok := MatchDataDirFs("/var/lib/pgsql/data", fss, false)
	assert.True(t, ok)
	assert.Equal(t, "/var/lib/pgsql", got.Mount.Mountpoint)
	assert.Equal(t, "/dev/data", got.Mount.Device)
}

// Test_MatchDataDirFs_componentBoundary verifies the prefix match is by path component:
// mount "/var" must NOT match data_directory "/variable/data" (string-prefix would, component
// boundary must not).
func Test_MatchDataDirFs_componentBoundary(t *testing.T) {
	fss := Fsstats{
		{Mount: Mount{Device: "/dev/root", Mountpoint: "/", Fstype: "ext4"}},
		{Mount: Mount{Device: "/dev/var", Mountpoint: "/var", Fstype: "ext4"}},
	}

	got, ok := MatchDataDirFs("/variable/data", fss, false)
	assert.True(t, ok)
	// "/var" is not a component-boundary prefix of "/variable/data"; only "/" matches.
	assert.Equal(t, "/", got.Mount.Mountpoint)
}

// Test_MatchDataDirFs_noMatch verifies that when no mountpoint is a prefix of data_directory
// (no root mount present), ok is false so the composer renders n/a.
func Test_MatchDataDirFs_noMatch(t *testing.T) {
	fss := Fsstats{
		{Mount: Mount{Device: "/dev/data", Mountpoint: "/srv/pgdata", Fstype: "ext4"}},
	}

	_, ok := MatchDataDirFs("/var/lib/pgsql/data", fss, false)
	assert.False(t, ok)
}

// Test_MatchDataDirFs_emptyInputs verifies graceful handling of an empty Fsstats slice and an
// empty data_directory (no panic, ok=false).
func Test_MatchDataDirFs_emptyInputs(t *testing.T) {
	_, ok := MatchDataDirFs("/var/lib/pgsql/data", Fsstats{}, false)
	assert.False(t, ok)

	_, ok = MatchDataDirFs("", Fsstats{{Mount: Mount{Mountpoint: "/"}}}, false)
	assert.False(t, ok)
}

// Test_MatchDataDirFs_evalSymlinksFailure verifies that a broken/inaccessible symlink under
// local=true degrades to ok=false without panicking and without surfacing the raw path.
func Test_MatchDataDirFs_evalSymlinksFailure(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "broken")
	// Point the symlink at a non-existent target so EvalSymlinks fails.
	assert.NoError(t, os.Symlink(filepath.Join(dir, "does-not-exist"), link))

	fss := Fsstats{{Mount: Mount{Mountpoint: "/"}}}

	assert.NotPanics(t, func() {
		_, ok := MatchDataDirFs(link, fss, true)
		assert.False(t, ok)
	})
}

// Test_MatchDataDirFs_localResolvesSymlink verifies that with local=true the data_directory
// symlink is resolved before matching: a symlink whose real path lives under a more specific
// mount selects that mount.
func Test_MatchDataDirFs_localResolvesSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	assert.NoError(t, os.Mkdir(real, 0o750))
	link := filepath.Join(dir, "link")
	assert.NoError(t, os.Symlink(real, link))

	// The resolved path is dir/real; a mount at dir is its longest prefix.
	fss := Fsstats{
		{Mount: Mount{Mountpoint: "/"}},
		{Mount: Mount{Mountpoint: dir, Device: "/dev/tmp"}},
	}

	got, ok := MatchDataDirFs(link, fss, true)
	assert.True(t, ok)
	assert.Equal(t, dir, got.Mount.Mountpoint)
}
