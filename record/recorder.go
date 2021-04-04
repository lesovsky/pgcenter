package record

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
	"io"
	"os"
	"path/filepath"
	"time"
)

// recorder defines a way of how to record and store collected stats.
type recorder interface {
	open() error
	collect(dbConfig postgres.Config, views view.Views) (map[string]stat.PGresult, error)
	write(map[string]stat.PGresult) error
	close() error
}

// tarConfig defines configuration needed for creating tar recorder.
type tarConfig struct {
	filename string
	append   bool
}

// tarRecorder implement recorder interface.
// This implementation collects Postgres stats and stores it in .json files packed into .tar archive.
type tarRecorder struct {
	config    tarConfig
	file      *os.File
	fileFlags int
	writer    *tar.Writer
}

// newTarRecorder creates new recorder.
func newTarRecorder(c tarConfig) recorder {
	var flags int
	if c.append {
		flags = os.O_CREATE | os.O_RDWR
	} else {
		flags = os.O_CREATE | os.O_RDWR | os.O_TRUNC
	}

	return &tarRecorder{
		config:    c,
		fileFlags: flags,
	}
}

// open method opens tar archive.
func (c *tarRecorder) open() error {
	f, err := os.OpenFile(filepath.Clean(c.config.filename), c.fileFlags, 0600)
	if err != nil {
		return err
	}

	// Determine seek offset.
	// If truncate is not requested check the file size. For empty files set
	// offset to 0 - start writing from beginning. For non-empty files set
	// offset to -1024 - start writing from last kB, to avoid overwrite tar metadata.
	if (c.fileFlags & os.O_TRUNC) == 0 {
		var offset int64

		st, err := f.Stat()
		if err != nil {
			return err
		}

		if st.Size() > 0 {
			offset = -1024
		}

		_, err = f.Seek(offset, io.SeekEnd)
		if err != nil {
			return err
		}
	} else {
		// If truncate was requested, disable O_TRUNC ans use just O_RDWR to
		// avoid further archive truncation.
		c.fileFlags = os.O_RDWR
	}

	c.file = f
	c.writer = tar.NewWriter(c.file)

	return nil
}

// collect connects to Postgres, collects and returns stats data.
func (c *tarRecorder) collect(dbConfig postgres.Config, views view.Views) (map[string]stat.PGresult, error) {
	db, err := postgres.Connect(dbConfig)
	if err != nil {
		return nil, err
	}

	stats := map[string]stat.PGresult{}

	for k, v := range views {
		res, err := stat.NewPGresultQuery(db, v.Query)
		if err != nil {
			return nil, err
			//fmt.Printf("WARN: %s; skip collecting %s\n", err.Error(), v.Name)
			//continue
		}

		stats[k] = res
	}

	return stats, nil
}

// write accepts stats data and writes it into tar archive.
func (c *tarRecorder) write(stats map[string]stat.PGresult) error {
	for name, v := range stats {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}

		now := time.Now()
		filename := fmt.Sprintf("%s.%s.json", name, now.Format("20060102T150405"))
		hdr := &tar.Header{Name: filename, Mode: 0644, Size: int64(len(data)), ModTime: now}
		err = c.writer.WriteHeader(hdr)
		if err != nil {
			return err
		}

		_, err = c.writer.Write(data)
		if err != nil {
			return err
		}
	}
	return nil
}

// close closes recorder's file and tar writer descriptors.
func (c *tarRecorder) close() error {
	if c.writer != nil {
		err := c.writer.Close()
		if err != nil {
			fmt.Printf("closing tar file failed: %s, continue", err)
		}
	}

	return c.file.Close()
}
