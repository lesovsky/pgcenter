package stat

import (
	"bufio"
	"context"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Mount describes a single mounted filesystem.
type Mount struct {
	Device     string
	Mountpoint string
	Fstype     string
	Options    string
}

// parseProcMounts reads mounts and returns slice of mounted filesystems properties.
func parseProcMounts(r io.Reader) ([]Mount, error) {
	var (
		scanner = bufio.NewScanner(r)
		mounts  []Mount
	)

	// Parse line by line, split line to param and value, parse the value to float and save to store.
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())

		if len(parts) != 6 {
			return nil, fmt.Errorf("invalid mounts input: '%s', skip", scanner.Text())
		}

		fstype := parts[2]

		// skip pseudo filesystems.
		re := regexp.MustCompile(`^(ext3|ext4|xfs|btrfs)$`)
		if !re.MatchString(fstype) {
			continue
		}

		s := Mount{
			Device:     parts[0],
			Mountpoint: parts[1],
			Fstype:     fstype,
			Options:    parts[3],
		}

		mounts = append(mounts, s)
	}

	return mounts, scanner.Err()
}

// Fsstat describes various stats related to filesystem usage.
type Fsstat struct {
	Mount      Mount
	Size       float64
	Free       float64
	Avail      float64
	Used       float64
	Reserved   float64
	Pused      float64
	Files      float64
	Filesfree  float64
	Filesused  float64
	Filespused float64
	err        error // error occurred during polling stats
}

// Fsstats combines all mounted filesystems stats.
type Fsstats []Fsstat

// readFsstats returns mounted filesystems stats depending on type of passed DB connection.
func readFsstats(db *postgres.DB, config Config) (Fsstats, error) {
	if db.Local {
		return readFilesystemStatsLocal("/proc/mounts")
	} else if config.SchemaPgcenterAvail {
		return readFilesystemStatsRemote(db)
	}

	return Fsstats{}, nil
}

// readFilesystemStatsLocal opens local stats file, execute stats parser and returns stats.
func readFilesystemStatsLocal(filename string) (Fsstats, error) {
	file, err := os.Open(filepath.Clean(filename))
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	return parseFilesystemStats(file)
}

// parseFilesystemStats parses stats file and return stats.
func parseFilesystemStats(r io.Reader) (Fsstats, error) {
	mounts, err := parseProcMounts(r)
	if err != nil {
		return nil, err
	}

	wg := sync.WaitGroup{}
	statCh := make(chan Fsstat)
	var stats Fsstats

	wg.Add(len(mounts))
	for _, m := range mounts {
		mount := m

		// In pessimistic cases, filesystem might stuck and requesting stats might stuck too. To avoid such situations wrap
		// stats requests into context with timeout. 200 milliseconds timeout should be sufficient for metal gear.
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)

		// Requesting stats.
		go readMountpointStat(mount.Mountpoint, statCh, &wg)

		// Awaiting the stats response from the channel, or context cancellation by timeout.
		select {
		case response := <-statCh:
			if response.err != nil {
				// TODO: handle error
				cancel()
				continue
			}

			stat := Fsstat{
				Mount:      mount,
				Size:       response.Size,
				Free:       response.Free,
				Avail:      response.Avail,
				Used:       response.Used,
				Reserved:   response.Reserved,
				Pused:      response.Pused,
				Files:      response.Files,
				Filesfree:  response.Filesfree,
				Filesused:  response.Filesused,
				Filespused: response.Filespused,
			}
			stats = append(stats, stat)
		case <-ctx.Done():
			cancel()
			continue
		}

		cancel()
	}

	wg.Wait()
	close(statCh)
	return stats, nil
}

// readMountpointStat requests stats from kernel and sends stats to channel.
func readMountpointStat(mountpoint string, ch chan Fsstat, wg *sync.WaitGroup) {
	defer wg.Done()

	var stat syscall.Statfs_t
	if err := syscall.Statfs(mountpoint, &stat); err != nil {
		ch <- Fsstat{err: err}
		return
	}

	// Syscall successful - send stat to the channel.
	ch <- Fsstat{
		Size:       float64(stat.Blocks) * float64(stat.Bsize),
		Free:       float64(stat.Bfree) * float64(stat.Bsize),
		Avail:      float64(stat.Bavail) * float64(stat.Bsize),
		Used:       float64(stat.Blocks-stat.Bfree) * float64(stat.Bsize),
		Reserved:   float64(stat.Bfree-stat.Bavail) * float64(stat.Bsize),
		Pused:      100 * float64(stat.Blocks-stat.Bfree) / float64(stat.Blocks),
		Files:      float64(stat.Files),
		Filesfree:  float64(stat.Ffree),
		Filesused:  float64(stat.Files - stat.Ffree),
		Filespused: 100 * float64(stat.Files-stat.Ffree) / float64(stat.Files),
	}
}

// readFilesystemStatsRemote returns mounted filesystems stats from SQL stats schema.
func readFilesystemStatsRemote(db *postgres.DB) (Fsstats, error) {
	q := "SELECT device, fstype, (pgcenter.get_filesystem_stats(mountpoint)).* FROM pgcenter.sys_proc_mounts"
	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stat Fsstats
	for rows.Next() {
		var fs Fsstat

		err := rows.Scan(&fs.Mount.Device, &fs.Mount.Fstype, &fs.Mount.Mountpoint,
			&fs.Size, &fs.Free, &fs.Avail, &fs.Used, &fs.Reserved, &fs.Pused,
			&fs.Files, &fs.Filesfree, &fs.Filesused, &fs.Filespused)
		if err != nil {
			return nil, err
		}

		// skip pseudo filesystems.
		re := regexp.MustCompile(`^(ext3|ext4|xfs|btrfs)$`)
		if !re.MatchString(fs.Mount.Fstype) {
			continue
		}

		stat = append(stat, fs)
	}

	return stat, nil
}
