package report

import (
	"archive/tar"
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/align"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "update golden files")

func Test_app_doReport(t *testing.T) {
	testcases := []struct {
		start    string
		end      string
		config   Config
		wantFile string
	}{
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "activity", TruncLimit: 32},
			wantFile: "testdata/report_activity.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "replication", TruncLimit: 32},
			wantFile: "testdata/report_replication.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "databases_general", TruncLimit: 32},
			wantFile: "testdata/report_databases_general.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "databases_sessions", TruncLimit: 32},
			wantFile: "testdata/report_databases_sessions.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "tables", TruncLimit: 32},
			wantFile: "testdata/report_tables.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "indexes", TruncLimit: 32},
			wantFile: "testdata/report_indexes.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "sizes", TruncLimit: 32},
			wantFile: "testdata/report_sizes.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "functions", TruncLimit: 32},
			wantFile: "testdata/report_functions.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "wal", TruncLimit: 32},
			wantFile: "testdata/report_wal.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "statements_timings", TruncLimit: 32},
			wantFile: "testdata/report_statements_timings.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "statements_general", TruncLimit: 32},
			wantFile: "testdata/report_statements_general.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "statements_io", TruncLimit: 32},
			wantFile: "testdata/report_statements_io.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "statements_local", TruncLimit: 32},
			wantFile: "testdata/report_statements_local.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "statements_temp", TruncLimit: 32},
			wantFile: "testdata/report_statements_temp.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "statements_wal", TruncLimit: 32},
			wantFile: "testdata/report_statements_wal.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "progress_vacuum", TruncLimit: 32},
			wantFile: "testdata/report_progress_vacuum.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "progress_cluster", TruncLimit: 32},
			wantFile: "testdata/report_progress_cluster.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "progress_index", TruncLimit: 32},
			wantFile: "testdata/report_progress_index.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "progress_analyze", TruncLimit: 32},
			wantFile: "testdata/report_progress_analyze.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "progress_basebackup", TruncLimit: 32},
			wantFile: "testdata/report_progress_basebackup.golden",
		},
		{
			start: "2021-06-14 11:56:00", end: "2021-06-14 11:57:00",
			config:   Config{ReportType: "progress_copy", TruncLimit: 32},
			wantFile: "testdata/report_progress_copy.golden",
		},
		{ // start, end times within report interval
			start: "2021-06-14 11:56:41", end: "2021-06-14 11:57:42",
			config:   Config{ReportType: "activity", TruncLimit: 32},
			wantFile: "testdata/report_activity_start_end.golden",
		},
		{ // start, end times within report interval, set order by pid (desc)
			start: "2021-06-14 11:56:41", end: "2021-06-14 11:57:42",
			config:   Config{ReportType: "activity", OrderColName: "pid", OrderDesc: true, TruncLimit: 32},
			wantFile: "testdata/report_activity_order_pid_desc.golden",
		},
		{ // start, end times within report interval, set order by pid (asc)
			start: "2021-06-14 11:56:41", end: "2021-06-14 11:57:42",
			config:   Config{ReportType: "activity", OrderColName: "pid", OrderDesc: false, TruncLimit: 32},
			wantFile: "testdata/report_activity_order_pid_asc.golden",
		},
		{ // start, end times within report interval, grep by query:UPDATE
			start: "2021-06-14 11:56:41", end: "2021-06-14 11:57:42",
			config:   Config{ReportType: "activity", FilterColName: "query", FilterRE: regexp.MustCompile("SELECT"), TruncLimit: 32},
			wantFile: "testdata/report_activity_grep.golden",
		},
		{ // start, end times within report interval, limit by number of rows
			start: "2021-06-14 11:56:41", end: "2021-06-14 11:57:42",
			config:   Config{ReportType: "statements_timings", RowLimit: 10, TruncLimit: 32},
			wantFile: "testdata/report_statements_timings_limit.golden",
		},
		{ // start, end times within report interval, limit by number of rows, string limit
			start: "2021-06-14 11:56:41", end: "2021-06-14 11:57:42",
			config:   Config{ReportType: "statements_timings", RowLimit: 10, TruncLimit: 64},
			wantFile: "testdata/report_statements_timings_limit_truncate.golden",
		},
	}

	for _, tc := range testcases {
		ts, err := time.ParseInLocation("2006-01-02 15:04:05", tc.start, time.Now().Location())
		assert.NoError(t, err)
		te, err := time.ParseInLocation("2006-01-02 15:04:05", tc.end, time.Now().Location())
		assert.NoError(t, err)

		tc.config.TsStart = ts
		tc.config.TsEnd = te

		app := newApp(tc.config)
		var buf bytes.Buffer
		app.writer = &buf

		f, err := os.Open("testdata/pgcenter.stat.golden.tar")
		assert.NoError(t, err)
		tr := tar.NewReader(f)

		err = app.doReport(tr)
		assert.NoError(t, err)

		if *update {
			assert.NoError(t, os.WriteFile(tc.wantFile, buf.Bytes(), 0644))
			continue
		}

		want, err := os.ReadFile(tc.wantFile)
		assert.NoError(t, err)

		assert.Equal(t, string(want), buf.String())
	}
}

func Test_readTar(t *testing.T) {
	config := Config{
		ReportType: "databases_general",
		TsStart:    time.Date(2021, 06, 14, 00, 00, 00, 0, time.UTC),
		TsEnd:      time.Date(2021, 06, 14, 23, 59, 59, 0, time.UTC),
		TruncLimit: 32}
	f, err := os.Open("testdata/pgcenter.stat.golden.tar")
	assert.NoError(t, err)
	tr := tar.NewReader(f)

	dataCh := make(chan data)
	doneCh := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		var count int
		for {
			select {
			case <-dataCh:
				count++
			case <-doneCh:
				assert.Equal(t, 10, count)
				wg.Done()
				break
			}
		}
	}()

	err = readTar(tr, config, dataCh, doneCh)
	assert.NoError(t, err)

	wg.Wait()

	assert.NoError(t, f.Close())
}

func Test_processData(t *testing.T) {
	prev := stat.PGresult{
		Valid: true, Ncols: 19, Nrows: 1,
		Cols: []string{
			"datname", "backends", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes",
			"conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age",
		},
		Values: [][]sql.NullString{
			{
				{String: "example_db", Valid: true}, {String: "15", Valid: true}, {String: "1000", Valid: true}, {String: "10", Valid: true},
				{String: "4000", Valid: true}, {String: "20000", Valid: true}, {String: "2000", Valid: true}, {String: "6000", Valid: true},
				{String: "8000", Valid: true}, {String: "12000", Valid: true}, {String: "3000", Valid: true}, {String: "50", Valid: true},
				{String: "60", Valid: true}, {String: "0", Valid: true}, {String: "100", Valid: true}, {String: "50000", Valid: true},
				{String: "500", Valid: true}, {String: "5", Valid: true}, {String: "11 days 10:10:10", Valid: true},
			},
		},
	}
	curr := stat.PGresult{
		Valid: true, Ncols: 19, Nrows: 1,
		Cols: []string{
			"datname", "backends", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes",
			"conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age",
		},
		Values: [][]sql.NullString{
			{
				{String: "example_db", Valid: true}, {String: "11", Valid: true}, {String: "1500", Valid: true}, {String: "15", Valid: true},
				{String: "6000", Valid: true}, {String: "30000", Valid: true}, {String: "3000", Valid: true}, {String: "9000", Valid: true},
				{String: "12000", Valid: true}, {String: "18000", Valid: true}, {String: "4500", Valid: true}, {String: "75", Valid: true},
				{String: "90", Valid: true}, {String: "1", Valid: true}, {String: "150", Valid: true}, {String: "75000", Valid: true},
				{String: "750", Valid: true}, {String: "8", Valid: true}, {String: "11 days 10:10:11", Valid: true},
			},
		},
	}

	config := Config{ReportType: "databases_general", TruncLimit: 32, OrderColName: "datname"}
	app := newApp(config)
	var buf bytes.Buffer
	app.writer = &buf

	views := view.New()

	dataCh := make(chan data)
	doneCh := make(chan struct{})

	go func() {
		dataCh <- data{
			ts:   time.Date(2021, 01, 01, 00, 00, 00, 0, time.UTC),
			res:  prev,
			meta: metadata{version: 140000},
		}

		dataCh <- data{
			ts:   time.Date(2021, 01, 01, 00, 00, 01, 0, time.UTC),
			res:  curr,
			meta: metadata{version: 140000},
		}

		doneCh <- struct{}{}
	}()

	err := processData(app, views["activity"], config, dataCh, doneCh)
	assert.NoError(t, err)

	want, err := os.ReadFile("testdata/report_sample.golden")
	assert.NoError(t, err)

	assert.Equal(t, string(want), buf.String())
}

func Test_readMeta(t *testing.T) {
	testcases := []struct {
		valid bool
		res   stat.PGresult
		want  metadata
	}{
		{
			valid: true,
			res: stat.PGresult{
				Values: [][]sql.NullString{
					{
						{String: "14beta1 (Ubuntu 14~beta1-1.pgdg20.04+1)", Valid: true}, {String: "140000", Valid: true},
						{String: "off", Valid: true}, {String: "100", Valid: true}, {String: "3", Valid: true},
						{String: "false", Valid: true}, {String: "1622828486655396e-6", Valid: true},
					},
				},
				Cols:  []string{"version", "version_num", "track_commit_timestamp", "max_connections", "autovacuum_max_workers", "recovery", "start_time_unix"},
				Ncols: 7, Nrows: 1, Valid: true,
			},
			want: metadata{version: 140000},
		},
		{
			valid: false,
			res: stat.PGresult{
				Values: [][]sql.NullString{
					{
						{String: "14beta1 (Ubuntu 14~beta1-1.pgdg20.04+1)", Valid: true}, {String: "invalid", Valid: true},
						{String: "off", Valid: true}, {String: "100", Valid: true}, {String: "3", Valid: true},
						{String: "false", Valid: true}, {String: "1622828486655396e-6", Valid: true},
					},
				},
				Cols:  []string{"version", "version_num", "track_commit_timestamp", "max_connections", "autovacuum_max_workers", "recovery", "start_time_unix"},
				Ncols: 7, Nrows: 1, Valid: true,
			},
			want: metadata{version: 140000},
		},
		// Reproduces issue #122: shared_preload_libraries was added to SelectCommonProperties
		// in cbfa0a4, making it 8 columns. Tar files recorded after that commit have Ncols=8
		// and were incorrectly rejected by the strict "!= 7" check.
		{
			valid: true,
			res: stat.PGresult{
				Values: [][]sql.NullString{
					{
						{String: "14.9", Valid: true}, {String: "140009", Valid: true},
						{String: "off", Valid: true}, {String: "100", Valid: true}, {String: "3", Valid: true},
						{String: "pg_stat_statements", Valid: true},
						{String: "false", Valid: true}, {String: "1622828486655396e-6", Valid: true},
					},
				},
				Cols:  []string{"version", "version_num", "track_commit_timestamp", "max_connections", "autovacuum_max_workers", "shared_preload_libraries", "recovery", "start_time_unix"},
				Ncols: 8, Nrows: 1, Valid: true,
			},
			want: metadata{version: 140009},
		},
		{valid: false, res: stat.PGresult{Ncols: 1, Nrows: 1, Valid: true}},
		{valid: false, res: stat.PGresult{Ncols: 7, Nrows: 0, Valid: true}},
	}

	for _, tc := range testcases {
		got, err := readMeta(tc.res)
		if tc.valid {
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		} else {
			assert.Error(t, err)
		}
	}
}

func Test_isFilenameOK(t *testing.T) {
	testcases := []struct {
		valid  bool
		name   string
		report string
	}{
		{valid: true, name: "databases_general.20210116T140630.123.json", report: "databases_general"},
		{valid: true, name: "databases_general.20210116T140630.000.json", report: "databases_general"},
		{valid: true, name: "meta.20210116T140630.123.json", report: "databases_general"},
		{valid: false, name: "databases_general.20210116T140630.123.json", report: "replication"},
		{valid: false, name: "databases_general.20210116T140630.json", report: "databases_general"},
	}

	for _, tc := range testcases {
		if tc.valid {
			assert.NoError(t, isFilenameOK(tc.name, tc.report))
		} else {
			assert.Error(t, isFilenameOK(tc.name, tc.report))
		}
	}
}

// Test_isFilenameOK_sysinfo verifies that the "sysinfo" filename prefix is
// accepted alongside "meta" and the requested report type. Without this,
// sysinfo.* tar entries would be silently skipped by readTar.
func Test_isFilenameOK_sysinfo(t *testing.T) {
	assert.NoError(t, isFilenameOK("sysinfo.20260519T100000.000.json", "procpidstat"))
}

// Test_readMeta_with_sysinfo builds an in-memory tar that mirrors the
// recorder's per-tick layout (meta + procpidstat + sysinfo, sysinfo written
// last) for two consecutive ticks, invokes readTar in a goroutine, and
// drains dataCh until doneCh fires. The test asserts:
//   - readTar emits one data item per tick (gated by metaOK && statOK).
//   - The second tick's data item carries meta.ticks=100 and
//     meta.cpuCount=4 — sysinfo from tick 1 is merged into the metadata
//     struct (which persists across loop iterations) and carried forward
//     into the next tick's send. This mirrors real recorder ordering where
//     sysinfo is appended after stats; the first tick is skipped by
//     processData (first-snapshot rule) so the one-tick lag is harmless.
func Test_readMeta_with_sysinfo(t *testing.T) {
	// Build meta PGresult (7-column SelectCommonProperties shape).
	metaRes := stat.PGresult{
		Valid: true, Ncols: 7, Nrows: 1,
		Cols: []string{"version", "version_num", "track_commit_timestamp", "max_connections", "autovacuum_max_workers", "recovery", "start_time_unix"},
		Values: [][]sql.NullString{
			{
				{String: "14.9", Valid: true}, {String: "140009", Valid: true},
				{String: "off", Valid: true}, {String: "100", Valid: true}, {String: "3", Valid: true},
				{String: "false", Valid: true}, {String: "1622828486655396e-6", Valid: true},
			},
		},
	}
	metaBytes, err := json.Marshal(metaRes)
	assert.NoError(t, err)

	// Build procpidstat data entry (minimal 19-column result with one row).
	statRes := stat.PGresult{
		Valid: true, Ncols: 19, Nrows: 1,
		Cols: []string{
			"pid", "datname", "usename", "state", "wait_etype", "wait_event",
			"cpu_time_user", "cpu_time_system", "cpu_time_total",
			"read_total,KiB", "write_total,KiB", "iodelay_total,s",
			"min_flt", "maj_flt", "vsize", "rss", "%cpu", "%all", "query",
		},
		Values: [][]sql.NullString{
			{
				{String: "1234", Valid: true}, {String: "postgres", Valid: true}, {String: "postgres", Valid: true},
				{String: "active", Valid: true}, {String: "", Valid: true}, {String: "", Valid: true},
				{String: "00:00:01", Valid: true}, {String: "00:00:00", Valid: true}, {String: "00:00:01", Valid: true},
				{String: "0", Valid: true}, {String: "0", Valid: true}, {String: "00:00:00", Valid: true},
				{String: "0", Valid: true}, {String: "0", Valid: true}, {String: "0", Valid: true}, {String: "0", Valid: true},
				{String: "0.0", Valid: true}, {String: "0.0", Valid: true}, {String: "SELECT 1", Valid: true},
			},
		},
	}
	statBytes, err := json.Marshal(statRes)
	assert.NoError(t, err)

	// Build sysinfo blob.
	sysinfoBytes := []byte(`{"ticks":100,"cpu_count":4}`)

	// Compose tar with two ticks. Per-tick order matches the recorder's
	// write() function: meta + view stats first (sysinfo recorded last).
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	writeEntry := func(name string, payload []byte) {
		hdr := &tar.Header{Name: name, Size: int64(len(payload)), Mode: 0644}
		assert.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write(payload)
		assert.NoError(t, err)
	}
	// Tick 1
	writeEntry("meta.20260519T100000.000.json", metaBytes)
	writeEntry("procpidstat.20260519T100000.000.json", statBytes)
	writeEntry("sysinfo.20260519T100000.000.json", sysinfoBytes)
	// Tick 2
	writeEntry("meta.20260519T100001.000.json", metaBytes)
	writeEntry("procpidstat.20260519T100001.000.json", statBytes)
	writeEntry("sysinfo.20260519T100001.000.json", sysinfoBytes)
	assert.NoError(t, tw.Close())

	tr := tar.NewReader(&tarBuf)
	config := Config{
		ReportType: "procpidstat",
		TsStart:    time.Date(2026, 5, 19, 0, 0, 0, 0, time.Now().Location()),
		TsEnd:      time.Date(2026, 5, 19, 23, 59, 59, 0, time.Now().Location()),
	}

	dataCh := make(chan data)
	doneCh := make(chan struct{})

	var items []data
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case d := <-dataCh:
				items = append(items, d)
			case <-doneCh:
				return
			}
		}
	}()

	err = readTar(tr, config, dataCh, doneCh)
	assert.NoError(t, err)
	wg.Wait()

	if assert.Len(t, items, 2) {
		// First tick lags: sysinfo arrives after stat in the recorder's
		// per-tick layout, so the data send fires before sysinfo is merged.
		// processData skips the first snapshot anyway (!prevStat.Valid →
		// continue), so the zero values here are harmless.
		assert.Equal(t, float64(0), items[0].meta.ticks)
		assert.Equal(t, 0, items[0].meta.cpuCount)
		// Second tick must carry the sysinfo values merged in by tick 1.
		assert.Equal(t, float64(100), items[1].meta.ticks)
		assert.Equal(t, 4, items[1].meta.cpuCount)
	}
}

// Test_readTar_sizeCap verifies that a crafted oversized tar header Size is
// rejected on each of the three readTar branches (meta.*, sysinfo.*, stat)
// before any allocation, no data is sent to the channel, and a legitimate
// under-limit entry still replays. The crafted entries set hdr.Size far above
// stat.MaxResultFileSize independently of the (tiny) real payload length, so
// writeEntry (which hardcodes Size=len(payload)) cannot be used here — the
// header is built directly via tw.WriteHeader.
func Test_readTar_sizeCap(t *testing.T) {
	overLimit := stat.MaxResultFileSize + 1

	// Legitimate (under-limit) meta + stat entries, used to prove a normal
	// entry still replays after the guard is in place.
	metaRes := stat.PGresult{
		Valid: true, Ncols: 7, Nrows: 1,
		Cols: []string{"version", "version_num", "track_commit_timestamp", "max_connections", "autovacuum_max_workers", "recovery", "start_time_unix"},
		Values: [][]sql.NullString{
			{
				{String: "14.9", Valid: true}, {String: "140009", Valid: true},
				{String: "off", Valid: true}, {String: "100", Valid: true}, {String: "3", Valid: true},
				{String: "false", Valid: true}, {String: "1622828486655396e-6", Valid: true},
			},
		},
	}
	metaBytes, err := json.Marshal(metaRes)
	assert.NoError(t, err)

	statRes := stat.PGresult{
		Valid: true, Ncols: 19, Nrows: 1,
		Cols: []string{
			"pid", "datname", "usename", "state", "wait_etype", "wait_event",
			"cpu_time_user", "cpu_time_system", "cpu_time_total",
			"read_total,KiB", "write_total,KiB", "iodelay_total,s",
			"min_flt", "maj_flt", "vsize", "rss", "%cpu", "%all", "query",
		},
		Values: [][]sql.NullString{
			{
				{String: "1234", Valid: true}, {String: "postgres", Valid: true}, {String: "postgres", Valid: true},
				{String: "active", Valid: true}, {String: "", Valid: true}, {String: "", Valid: true},
				{String: "00:00:01", Valid: true}, {String: "00:00:00", Valid: true}, {String: "00:00:01", Valid: true},
				{String: "0", Valid: true}, {String: "0", Valid: true}, {String: "00:00:00", Valid: true},
				{String: "0", Valid: true}, {String: "0", Valid: true}, {String: "0", Valid: true}, {String: "0", Valid: true},
				{String: "0.0", Valid: true}, {String: "0.0", Valid: true}, {String: "SELECT 1", Valid: true},
			},
		},
	}
	statBytes, err := json.Marshal(statRes)
	assert.NoError(t, err)

	config := Config{
		ReportType: "procpidstat",
		TsStart:    time.Date(2026, 5, 19, 0, 0, 0, 0, time.Now().Location()),
		TsEnd:      time.Date(2026, 5, 19, 23, 59, 59, 0, time.Now().Location()),
	}

	// oversizedTar returns tar bytes containing a single entry whose header
	// Size is crafted above the limit. Only the header is emitted (no body and
	// no Close): the tar.Writer enforces Size == bytes-written, so we cannot
	// pair a huge declared Size with a tiny real payload. readTar rejects on
	// the header Size before reading any body, so the header alone is enough.
	oversizedTar := func(name string) []byte {
		var tarBuf bytes.Buffer
		tw := tar.NewWriter(&tarBuf)
		hdr := &tar.Header{Name: name, Size: overLimit, Mode: 0644}
		assert.NoError(t, tw.WriteHeader(hdr))
		return tarBuf.Bytes()
	}

	// runReadTar drives readTar over the given tar bytes and returns the
	// error plus how many data items were sent to the channel.
	runReadTar := func(tarBytes []byte) (error, int) {
		tr := tar.NewReader(bytes.NewReader(tarBytes))
		dataCh := make(chan data)
		doneCh := make(chan struct{})
		var count int
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-dataCh:
					count++
				case <-doneCh:
					return
				}
			}
		}()
		rerr := readTar(tr, config, dataCh, doneCh)
		wg.Wait()
		return rerr, count
	}

	// meta.* branch — hits the NewPGresultFile guard (over-limit error).
	t.Run("meta branch over limit", func(t *testing.T) {
		err, count := runReadTar(oversizedTar("meta.20260519T100000.000.json"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds limit")
		assert.Equal(t, 0, count)
	})

	// stat (default) branch — hits the NewPGresultFile guard (over-limit error).
	t.Run("stat branch over limit", func(t *testing.T) {
		err, count := runReadTar(oversizedTar("procpidstat.20260519T100000.000.json"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds limit")
		assert.Equal(t, 0, count)
	})

	// sysinfo.* branch — hits the inline check in report.go (defense-in-depth).
	t.Run("sysinfo branch over limit", func(t *testing.T) {
		err, count := runReadTar(oversizedTar("sysinfo.20260519T100000.000.json"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds limit")
		assert.Equal(t, 0, count)
	})

	// Legitimate under-limit entries still replay: a full tick (meta + stat)
	// produces exactly one data item with no error.
	t.Run("under limit replays", func(t *testing.T) {
		var tarBuf bytes.Buffer
		tw := tar.NewWriter(&tarBuf)
		writeEntry := func(name string, payload []byte) {
			hdr := &tar.Header{Name: name, Size: int64(len(payload)), Mode: 0644}
			assert.NoError(t, tw.WriteHeader(hdr))
			_, werr := tw.Write(payload)
			assert.NoError(t, werr)
		}
		writeEntry("meta.20260519T100000.000.json", metaBytes)
		writeEntry("procpidstat.20260519T100000.000.json", statBytes)
		assert.NoError(t, tw.Close())

		err, count := runReadTar(tarBuf.Bytes())
		assert.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}

// Test_emitProcPidStatAvailabilityWarnings exercises the one-shot WARNING
// detection that scans the first procpidstat result for empty IO / iodelay
// column sentinels. Covers acceptance criteria for empty IO columns (9–10)
// and empty iodelay column (11), and the no-row no-warning path.
func Test_emitProcPidStatAvailabilityWarnings(t *testing.T) {
	mkRow := func(read, write, iodelay string) []sql.NullString {
		row := make([]sql.NullString, 19)
		for i := range row {
			row[i] = sql.NullString{String: "x", Valid: true}
		}
		row[stat.ColReadTotalKiB] = sql.NullString{String: read, Valid: true}
		row[stat.ColWriteTotalKiB] = sql.NullString{String: write, Valid: true}
		row[stat.ColIODelayTotalS] = sql.NullString{String: iodelay, Valid: true}
		return row
	}

	testcases := []struct {
		name string
		res  stat.PGresult
		want string
	}{
		{
			name: "empty IO read column emits IO warning",
			res: stat.PGresult{
				Valid: true, Ncols: 19, Nrows: 1,
				Values: [][]sql.NullString{mkRow("", "100", "00:00:01")},
			},
			want: "WARNING: IO stats unavailable in recorded data\n",
		},
		{
			name: "empty IO write column emits IO warning",
			res: stat.PGresult{
				Valid: true, Ncols: 19, Nrows: 1,
				Values: [][]sql.NullString{mkRow("100", "", "00:00:01")},
			},
			want: "WARNING: IO stats unavailable in recorded data\n",
		},
		{
			name: "empty iodelay column emits iodelay warning",
			res: stat.PGresult{
				Valid: true, Ncols: 19, Nrows: 1,
				Values: [][]sql.NullString{mkRow("100", "200", "")},
			},
			want: "WARNING: iodelay stats unavailable in recorded data\n",
		},
		{
			name: "all unavailable emits both warnings",
			res: stat.PGresult{
				Valid: true, Ncols: 19, Nrows: 1,
				Values: [][]sql.NullString{mkRow("", "", "")},
			},
			want: "WARNING: IO stats unavailable in recorded data\nWARNING: iodelay stats unavailable in recorded data\n",
		},
		{
			name: "populated columns emit nothing",
			res: stat.PGresult{
				Valid: true, Ncols: 19, Nrows: 1,
				Values: [][]sql.NullString{mkRow("100", "200", "00:00:01")},
			},
			want: "",
		},
		{
			name: "zero rows emit nothing",
			res:  stat.PGresult{Valid: true, Ncols: 19, Nrows: 0, Values: [][]sql.NullString{}},
			want: "",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			assert.NoError(t, emitProcPidStatAvailabilityWarnings(&buf, tc.res))
			assert.Equal(t, tc.want, buf.String())
		})
	}
}

// Test_app_doReport_procpidstat exercises the full doReport pipeline for
// the procpidstat report type against a synthetic in-memory tar. The tar
// contains two ticks; each tick carries meta + procpidstat + sysinfo (the
// same layout tarRecorder.write() produces). The first tick is consumed by
// processData as the prev snapshot (no output); the second tick becomes
// curr and produces a timestamp header line + one data row. The test runs
// without any PostgreSQL connection or live procfs.
func Test_app_doReport_procpidstat(t *testing.T) {
	// Meta result mirrors SelectCommonProperties (7-column shape; readMeta
	// only consumes column index 1 for version_num).
	metaRes := stat.PGresult{
		Valid: true, Ncols: 7, Nrows: 1,
		Cols: []string{"version", "version_num", "track_commit_timestamp", "max_connections", "autovacuum_max_workers", "recovery", "start_time_unix"},
		Values: [][]sql.NullString{
			{
				{String: "14.9", Valid: true}, {String: "140009", Valid: true},
				{String: "off", Valid: true}, {String: "100", Valid: true}, {String: "3", Valid: true},
				{String: "false", Valid: true}, {String: "1622828486655396e-6", Valid: true},
			},
		},
	}
	metaBytes, err := json.Marshal(metaRes)
	assert.NoError(t, err)

	// procpidstat column order must match internal/stat.procPidResultCols
	// (canonical 19-column header). UniqueKey is pid (col 0), so both
	// snapshots must share the same pid to be paired by diff().
	cols := []string{
		"pid", "datname", "usename", "state", "wait_etype", "wait_event",
		"all_total,s", "us_total,s", "sy_total,s",
		"read_total,KiB", "write_total,KiB",
		"iodelay_total,s",
		"%all", "%us", "%sy",
		"read,KiB/s", "write,KiB/s",
		"%iodelay",
		"query",
	}

	mkRow := func(allTotal, usTotal, syTotal, readKiB, writeKiB, iodelay, pcAll, pcUs, pcSy, readRate, writeRate, pcIODelay string) []sql.NullString {
		return []sql.NullString{
			{String: "1234", Valid: true}, {String: "postgres", Valid: true}, {String: "postgres", Valid: true},
			{String: "active", Valid: true}, {String: "", Valid: true}, {String: "", Valid: true},
			{String: allTotal, Valid: true}, {String: usTotal, Valid: true}, {String: syTotal, Valid: true},
			{String: readKiB, Valid: true}, {String: writeKiB, Valid: true},
			{String: iodelay, Valid: true},
			{String: pcAll, Valid: true}, {String: pcUs, Valid: true}, {String: pcSy, Valid: true},
			{String: readRate, Valid: true}, {String: writeRate, Valid: true},
			{String: pcIODelay, Valid: true},
			{String: "SELECT 1", Valid: true},
		}
	}

	// Snapshot 1: first tick — discarded as prev by processData's
	// first-snapshot rule (!prevStat.Valid -> continue). procpidstat's view
	// config in view.New() sets DiffIntvl=[0,0], so countDiff returns curr
	// verbatim (no per-column delta) and these prev values never appear in
	// output regardless of their content.
	statPrev := stat.PGresult{
		Valid: true, Ncols: 19, Nrows: 1, Cols: cols,
		Values: [][]sql.NullString{
			mkRow("00:00:00", "00:00:00", "00:00:00", "0", "0", "00:00:00", "0", "0", "0", "0", "0", "0"),
		},
	}
	prevBytes, err := json.Marshal(statPrev)
	assert.NoError(t, err)

	// Snapshot 2: second tick — non-zero values produce the data row.
	statCurr := stat.PGresult{
		Valid: true, Ncols: 19, Nrows: 1, Cols: cols,
		Values: [][]sql.NullString{
			mkRow("00:00:05", "00:00:03", "00:00:02", "1024", "2048", "00:00:01", "5.0", "3.0", "2.0", "100.00", "200.00", "1.0"),
		},
	}
	currBytes, err := json.Marshal(statCurr)
	assert.NoError(t, err)

	sysinfoBytes := []byte(`{"ticks":100,"cpu_count":4}`)

	// Compose tar (two ticks; per-tick layout matches tarRecorder.write()).
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	writeEntry := func(name string, payload []byte) {
		hdr := &tar.Header{Name: name, Size: int64(len(payload)), Mode: 0644}
		assert.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write(payload)
		assert.NoError(t, err)
	}
	writeEntry("meta.20260519T100000.000.json", metaBytes)
	writeEntry("procpidstat.20260519T100000.000.json", prevBytes)
	writeEntry("sysinfo.20260519T100000.000.json", sysinfoBytes)
	writeEntry("meta.20260519T100001.000.json", metaBytes)
	writeEntry("procpidstat.20260519T100001.000.json", currBytes)
	writeEntry("sysinfo.20260519T100001.000.json", sysinfoBytes)
	assert.NoError(t, tw.Close())

	config := Config{
		ReportType: "procpidstat",
		TruncLimit: 32,
		TsStart:    time.Date(2026, 5, 19, 0, 0, 0, 0, time.Now().Location()),
		TsEnd:      time.Date(2026, 5, 19, 23, 59, 59, 0, time.Now().Location()),
	}

	app := newApp(config)
	var buf bytes.Buffer
	app.writer = &buf

	tr := tar.NewReader(&tarBuf)
	assert.NoError(t, app.doReport(tr))

	out := buf.String()
	assert.NotEmpty(t, out)
	// Timestamp header line emitted by printStatSample matches "YYYY/MM/DD".
	assert.Regexp(t, regexp.MustCompile(`\d{4}/\d{2}/\d{2}`), out)
	// Data-row presence pinned independently of the query column: the pid
	// from snapshot 2 must appear (catches regressions that suppress the
	// row but keep the timestamp header).
	assert.Contains(t, out, "1234")
	// "SELECT 1" comes from the data row (last column of snapshot 2).
	assert.Contains(t, out, "SELECT 1")
}

// Test_processData_no_procpidstat_data verifies that when the data channel
// closes with zero items and the report type is "procpidstat", the function
// prints the INFO message and returns nil (no error).
func Test_processData_no_procpidstat_data(t *testing.T) {
	config := Config{ReportType: "procpidstat", TruncLimit: 32}
	app := newApp(config)
	var buf bytes.Buffer
	app.writer = &buf

	views := view.New()

	dataCh := make(chan data)
	doneCh := make(chan struct{})

	go func() {
		doneCh <- struct{}{}
	}()

	err := processData(app, views["procpidstat"], config, dataCh, doneCh)
	assert.NoError(t, err)
	assert.Equal(t, "INFO: no procpidstat data in this archive\n", buf.String())
}

func Test_isFilenameTimestampOK(t *testing.T) {
	testcases := []struct {
		valid bool
		name  string
		start string
		end   string
		want  string
	}{
		{valid: true, name: "databases_general.20210116T140630.123.json", start: "14:00:00.000", end: "15:00:00.000", want: "20210116 14:06:30.123"},
		{valid: false, name: "invalid.json", start: "14:00:00.000", end: "15:00:00.000", want: "20210116 14:06:30.000"},
		{valid: false, name: "invalid.invalid-ts.json", start: "14:00:00.000", end: "15:00:00.000", want: "20210116 14:06:30.000"},
		{valid: false, name: "databases_general.20210116T140630.json", start: "14:30:00.000", end: "15:00:00.000", want: "20210116 14:06:30.000"},
		{valid: false, name: "databases_general.20210116T140630.json", start: "13:30:00.000", end: "14:00:00.000", want: "20210116 14:06:30.000"},
	}

	loc := time.Now().Location()

	for _, tc := range testcases {
		start, err := time.ParseInLocation("20060102 15:04:05.000", fmt.Sprintf("20210116 %s", tc.start), loc)
		assert.NoError(t, err)

		end, err := time.ParseInLocation("20060102 15:04:05.000", fmt.Sprintf("20210116 %s", tc.end), loc)
		assert.NoError(t, err)

		got, err := isFilenameTimestampOK(tc.name, start, end)
		if tc.valid {
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got.Format("20060102 15:04:05.000"))
		} else {
			assert.Error(t, err)
		}
	}
}

func Test_countDiff(t *testing.T) {
	prev := stat.PGresult{
		Valid: true, Ncols: 19, Nrows: 1,
		Cols: []string{
			"datname", "backends", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes",
			"conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age",
		},
		Values: [][]sql.NullString{
			{
				{String: "example_db", Valid: true}, {String: "15", Valid: true}, {String: "1000", Valid: true}, {String: "10", Valid: true},
				{String: "4000", Valid: true}, {String: "20000", Valid: true}, {String: "2000", Valid: true}, {String: "6000", Valid: true},
				{String: "8000", Valid: true}, {String: "12000", Valid: true}, {String: "3000", Valid: true}, {String: "50", Valid: true},
				{String: "60", Valid: true}, {String: "0", Valid: true}, {String: "100", Valid: true}, {String: "50000", Valid: true},
				{String: "500", Valid: true}, {String: "5", Valid: true}, {String: "11 days 10:10:10", Valid: true},
			},
		},
	}
	curr := stat.PGresult{
		Valid: true, Ncols: 19, Nrows: 1,
		Cols: []string{
			"datname", "backends", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes",
			"conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age",
		},
		Values: [][]sql.NullString{
			{
				{String: "example_db", Valid: true}, {String: "11", Valid: true}, {String: "1500", Valid: true}, {String: "15", Valid: true},
				{String: "6000", Valid: true}, {String: "30000", Valid: true}, {String: "3000", Valid: true}, {String: "9000", Valid: true},
				{String: "12000", Valid: true}, {String: "18000", Valid: true}, {String: "4500", Valid: true}, {String: "75", Valid: true},
				{String: "90", Valid: true}, {String: "1", Valid: true}, {String: "150", Valid: true}, {String: "75000", Valid: true},
				{String: "750", Valid: true}, {String: "8", Valid: true}, {String: "11 days 10:10:11", Valid: true},
			},
		},
	}

	want1second := stat.PGresult{
		Valid: true, Ncols: 19, Nrows: 1,
		Cols: []string{
			"datname", "backends", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes",
			"conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age",
		},
		Values: [][]sql.NullString{
			{
				{String: "example_db", Valid: true}, {String: "11", Valid: true}, {String: "500", Valid: true}, {String: "5", Valid: true},
				{String: "2000", Valid: true}, {String: "10000", Valid: true}, {String: "1000", Valid: true}, {String: "3000", Valid: true},
				{String: "4000", Valid: true}, {String: "6000", Valid: true}, {String: "1500", Valid: true}, {String: "25", Valid: true},
				{String: "30", Valid: true}, {String: "1", Valid: true}, {String: "50", Valid: true}, {String: "25000", Valid: true},
				{String: "250", Valid: true}, {String: "3", Valid: true}, {String: "11 days 10:10:11", Valid: true},
			},
		},
	}

	want5second := stat.PGresult{
		Valid: true, Ncols: 19, Nrows: 1,
		Cols: []string{
			"datname", "backends", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes",
			"conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age",
		},
		Values: [][]sql.NullString{
			{
				{String: "example_db", Valid: true}, {String: "11", Valid: true}, {String: "100", Valid: true}, {String: "1", Valid: true},
				{String: "400", Valid: true}, {String: "2000", Valid: true}, {String: "200", Valid: true}, {String: "600", Valid: true},
				{String: "800", Valid: true}, {String: "1200", Valid: true}, {String: "300", Valid: true}, {String: "5", Valid: true},
				{String: "6", Valid: true}, {String: "0", Valid: true}, {String: "10", Valid: true}, {String: "5000", Valid: true},
				{String: "50", Valid: true}, {String: "0", Valid: true}, {String: "11 days 10:10:11", Valid: true},
			},
		},
	}

	views := view.New()
	v := views["databases_general"]

	got, err := countDiff(curr, prev, 1, v)
	assert.NoError(t, err)
	assert.Equal(t, want1second, got)

	got, err = countDiff(curr, prev, 5, v)
	assert.NoError(t, err)
	assert.Equal(t, want5second, got)
}

func Test_getColumnIndex(t *testing.T) {
	testcases := []struct {
		colname string
		wantIdx int
		wantOk  bool
	}{
		{colname: "testcol2", wantIdx: 1, wantOk: true},
		{colname: "unknown", wantIdx: -1, wantOk: false},
		{colname: "", wantIdx: -1, wantOk: false},
	}

	for _, tc := range testcases {
		got, ok := getColumnIndex([]string{"testcol1", "testcol2", "testcol3"}, tc.colname)
		assert.Equal(t, tc.wantIdx, got)
		assert.Equal(t, tc.wantOk, ok)
	}
}

func Test_formatStatSample(t *testing.T) {
	res := &stat.PGresult{
		Valid: true,
		Ncols: 18,
		Nrows: 2,
		Cols: []string{
			"datname", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes",
			"conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age",
		},
		Values: [][]sql.NullString{
			{
				{String: "example_db", Valid: true}, {String: "5423", Valid: true}, {String: "24", Valid: true}, {String: "8452", Valid: true},
				{String: "8452145", Valid: true}, {String: "45214", Valid: true}, {String: "58452", Valid: true}, {String: "4521", Valid: true},
				{String: "45221", Valid: true}, {String: "45854", Valid: true}, {String: "248", Valid: true}, {String: "785", Valid: true},
				{String: "2", Valid: true}, {String: "4774", Valid: true}, {String: "698785411", Valid: true}, {String: "4582.02", Valid: true},
				{String: "42.12", Valid: true}, {String: "10 days 10:10:10", Valid: true},
			},
			{
				{String: "example_db2", Valid: true}, {String: "84521", Valid: true}, {String: "866", Valid: true}, {String: "59654", Valid: true},
				{String: "485421", Valid: true}, {String: "86421", Valid: true}, {String: "89642", Valid: true}, {String: "9869", Valid: true},
				{String: "45212", Valid: true}, {String: "96969", Valid: true}, {String: "124", Valid: true}, {String: "858", Valid: true},
				{String: "0", Valid: true}, {String: "8457", Valid: true}, {String: "6581546", Valid: true}, {String: "2445.77", Valid: true},
				{String: "458.01", Valid: true}, {String: "10 days 10:10:10", Valid: true},
			},
		},
	}

	views := view.New()
	v := views["databases_general"]

	formatStatSample(res, &v, Config{})

	assert.True(t, v.Aligned)
	assert.NotNil(t, v.ColsWidth)
	assert.NotNil(t, v.Cols)
}

func Test_printReportHeader(t *testing.T) {
	tsStart, err := time.Parse("2006-01-02 15:04:05 MST", "2021-01-18 05:00:00 +05")
	assert.NoError(t, err)
	tsEnd, err := time.Parse("2006-01-02 15:04:05 MST", "2021-01-18 06:00:00 +05")
	assert.NoError(t, err)

	c := Config{
		InputFile:  "test_example.stat.tar",
		ReportType: "test_example",
		TsStart:    tsStart,
		TsEnd:      tsEnd,
	}

	want := `INFO: reading from test_example.stat.tar
INFO: report test_example
INFO: start from: 2021-01-18 05:00:00 +05, to: 2021-01-18 06:00:00 +05
`

	var buf bytes.Buffer
	assert.NoError(t, printReportHeader(&buf, c))
	assert.Equal(t, want, buf.String())
}

func Test_printStatHeader(t *testing.T) {
	res := &stat.PGresult{
		Valid: true, Ncols: 18, Nrows: 0,
		Cols: []string{
			"datname", "commits", "rollbacks", "reads", "hits", "returned", "fetched", "inserts", "updates", "deletes",
			"conflicts", "deadlocks", "csum_fails", "temp_files", "temp_bytes", "read_t", "write_t", "stats_age",
		},
		Values: [][]sql.NullString{},
	}

	views := view.New()
	v := views["databases_general"]

	widthes, cols := align.SetAlign(*res, 32, true)
	v.ColsWidth = widthes
	v.Cols = cols
	v.Aligned = true

	var buf bytes.Buffer

	n, err := printStatHeader(&buf, 20, v)
	assert.Equal(t, 0, n)
	assert.Equal(t,
		"\x1b[37;1mdatname  \x1b[0m\x1b[37;1mcommits  \x1b[0m\x1b[37;1mrollbacks  \x1b[0m\x1b[37;1mreads  \x1b[0m\x1b[37;1mhits  \x1b[0m\x1b[37;1mreturned  \x1b[0m\x1b[37;1mfetched  \x1b[0m\x1b[37;1minserts  \x1b[0m\x1b[37;1mupdates  \x1b[0m\x1b[37;1mdeletes  \x1b[0m\x1b[37;1mconflicts  \x1b[0m\x1b[37;1mdeadlocks  \x1b[0m\x1b[37;1mcsum_fails  \x1b[0m\x1b[37;1mtemp_files  \x1b[0m\x1b[37;1mtemp_bytes  \x1b[0m\x1b[37;1mread_t  \x1b[0m\x1b[37;1mwrite_t  \x1b[0m\x1b[37;1mstats_age  \x1b[0m\n",
		buf.String(),
	)
	assert.NoError(t, err)

	n, err = printStatHeader(&buf, 10, v)
	assert.Equal(t, 10, n)
	assert.NoError(t, err)
}

func Test_printStatSample(t *testing.T) {
	res := &stat.PGresult{
		Valid: true,
		Ncols: 18,
		Nrows: 2,
		Cols: []string{
			"datname", "commits", "rollbacks", "reads",
			"hits", "returned", "fetched", "inserts",
			"updates", "deletes", "conflicts", "deadlocks",
			"csum_fails", "temp_files", "temp_bytes", "read_t",
			"write_t", "stats_age",
		},
		Values: [][]sql.NullString{
			{
				{String: "example_db", Valid: true}, {String: "5423", Valid: true}, {String: "24", Valid: true}, {String: "8452", Valid: true},
				{String: "8452145", Valid: true}, {String: "45214", Valid: true}, {String: "58452", Valid: true}, {String: "4521", Valid: true},
				{String: "45221", Valid: true}, {String: "45854", Valid: true}, {String: "248", Valid: true}, {String: "785", Valid: true},
				{String: "2", Valid: true}, {String: "4774", Valid: true}, {String: "698785411", Valid: true}, {String: "4582.02", Valid: true},
				{String: "42.12", Valid: true}, {String: "10 days 10:10:10", Valid: true},
			},
			{
				{String: "example_db2", Valid: true}, {String: "84521", Valid: true}, {String: "866", Valid: true}, {String: "59654", Valid: true},
				{String: "485421", Valid: true}, {String: "86421", Valid: true}, {String: "89642", Valid: true}, {String: "9869", Valid: true},
				{String: "45212", Valid: true}, {String: "96969", Valid: true}, {String: "124", Valid: true}, {String: "858", Valid: true},
				{String: "0", Valid: true}, {String: "8457", Valid: true}, {String: "6581546", Valid: true}, {String: "2445.77", Valid: true},
				{String: "458.01", Valid: true}, {String: "10 days 10:10:10", Valid: true},
			},
		},
	}

	views := view.New()
	v := views["databases_general"]

	widthes, cols := align.SetAlign(*res, 32, true)
	v.ColsWidth = widthes
	v.Cols = cols
	v.Aligned = true

	f, err := os.CreateTemp("/tmp", "pgcenter-test-report-")
	assert.NoError(t, err)
	fname := f.Name()

	// print report
	n, err := printStatSample(f, res, v, Config{}, time.Time{}, time.Second)
	assert.NoError(t, err)
	assert.Equal(t, 2, n)

	// rewind to beginning
	_, err = f.Seek(0, io.SeekStart)
	assert.NoError(t, err)

	// read file
	got, err := io.ReadAll(f)
	assert.NoError(t, err)

	// read wanted
	want, err := os.ReadFile("testdata/report_entry_sample.golden")
	assert.NoError(t, err)

	// compare created and wanted
	assert.Equal(t, want, got)

	// cleanup
	assert.NoError(t, f.Close())
	assert.NoError(t, os.Remove(fname))
}

func Test_describeReport(t *testing.T) {
	testcases := []struct {
		report string
		want   string
	}{
		{report: "databases_general", want: pgStatDatabaseGeneralDescription},
		{report: "databases_sessions", want: pgStatDatabaseSessionsDescription},
		{report: "activity", want: pgStatActivityDescription},
		{report: "replication", want: pgStatReplicationDescription},
		{report: "tables", want: pgStatTablesDescription},
		{report: "indexes", want: pgStatIndexesDescription},
		{report: "functions", want: pgStatFunctionsDescription},
		{report: "wal", want: pgStatWALDescription},
		{report: "sizes", want: pgStatSizesDescription},
		{report: "progress_vacuum", want: pgStatProgressVacuumDescription},
		{report: "progress_cluster", want: pgStatProgressClusterDescription},
		{report: "progress_index", want: pgStatProgressCreateIndexDescription},
		{report: "progress_analyze", want: pgStatProgressAnalyzeDescription},
		{report: "progress_basebackup", want: pgStatProgressBasebackupDescription},
		{report: "progress_copy", want: pgStatProgressCopyDescription},
		{report: "statements_timings", want: pgStatStatementsTimingsDescription},
		{report: "statements_general", want: pgStatStatementsGeneralDescription},
		{report: "statements_io", want: pgStatStatementsIODescription},
		{report: "statements_local", want: pgStatStatementsLocalDescription},
		{report: "statements_temp", want: pgStatStatementsTempDescription},
		{report: "bgwriter", want: pgStatBgwriterDescription},
		{report: "replslots", want: pgStatReplicationSlotsDescription},
		{report: "stat_io", want: pgStatIODescription},
		{report: "stat_io_time", want: pgStatIOTimeDescription},
		{report: "statements_jit", want: pgStatStatementsJITDescription},
		{report: "procpidstat", want: procPidStatDescription},
		{report: "invalid", want: "unknown description requested"},
	}

	for _, tc := range testcases {
		var buf bytes.Buffer

		err := describeReport(&buf, tc.report)
		assert.NoError(t, err)
		assert.Equal(t, tc.want, buf.String())
	}

}

func Test_describeProgressColumnOrder(t *testing.T) {
	// The describe test above compares descriptions by identity, so it cannot notice a row that
	// landed in the wrong slot. Row order has to match the order the queries emit, otherwise
	// `report -d` documents a layout that does not exist.
	testcases := []struct {
		name    string
		text    string
		markers []string
	}{
		{
			name:    "vacuum",
			text:    pgStatProgressVacuumDescription,
			markers: []string{"\n- relation", "\n- started_by", "\n- mode", "\n- state"},
		},
		{
			name:    "analyze",
			text:    pgStatProgressAnalyzeDescription,
			markers: []string{"\n- relation", "\n- started_by", "\n- state"},
		},
		{
			name:    "basebackup",
			text:    pgStatProgressBasebackupDescription,
			markers: []string{"\n- duration", "\n- backup_type", "\n- state"},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			prev := -1
			for _, m := range tc.markers {
				pos := strings.Index(tc.text, m)
				// Presence first: strings.Index returns -1 for a missing marker, and -1 is less than
				// anything, so an ordering-only assertion would pass on a row that is not there at all.
				require.NotEqual(t, -1, pos, "description must contain a row for %q", strings.TrimPrefix(m, "\n- "))
				assert.Greater(t, pos, prev, "row %q is out of order", strings.TrimPrefix(m, "\n- "))
				prev = pos
			}
		})
	}
}

func Test_describeActivityColumnOrder(t *testing.T) {
	// Same reason as Test_describeProgressColumnOrder: Test_describeReport compares descriptions
	// by identity and cannot notice a row that landed in the wrong slot. The list below is the
	// column order of query.PgStatActivityPG13 (internal/query/activity.go) - the layout that
	// `report -d -A` claims to document.
	columns := []string{
		"pid", "leader", "cl_addr", "cl_port", "datname", "usename", "appname", "backend_type",
		"wait_etype", "wait_event", "state", "backend_xid", "horizon_xacts", "xact_age",
		"query_age", "change_age", "query",
	}

	prev := -1
	for _, c := range columns {
		// The marker is anchored on both sides: "\n- " keeps it off words inside the prose
		// (leader_pid, Process ID), and the trailing tab keeps a short name off a longer row -
		// without it "\n- query" matches the "- query_age" row and the offsets compared below
		// are not the ones being checked.
		pos := strings.Index(pgStatActivityDescription, "\n- "+c+"\t")
		// Presence first: strings.Index returns -1 for a missing marker, and -1 is less than
		// anything, so an ordering-only assertion would pass on a row that is not there at all.
		require.NotEqual(t, -1, pos, "description must contain a row for %q", c)
		assert.Greater(t, pos, prev, "row %q is out of order", c)
		prev = pos
	}
}

func Test_describeActivityCaveats(t *testing.T) {
	// Each of the three new columns can be misread into a wrong pg_terminate_backend decision,
	// and there is nothing to catch in code for any of them - the caveats in this block are the
	// only place the traps are written down. So every caveat is asserted on its own: deleting any
	// single one must redden a subtest that names it.
	assert.Contains(t, pgStatActivityDescription, "available since PG13",
		"block must say the three new columns require PG13+")

	testcases := []struct {
		name    string
		markers []string
	}{
		{
			// Derived leader: an empty one would otherwise read as "not a parallel backend".
			name:    "leader is derived, not the raw leader_pid",
			markers: []string{"leader_pid"},
		},
		{
			// Backends are not the only holders of the horizon, and the others are not in the view.
			name:    "horizon covers backend sources only",
			markers: []string{"replication slots", "prepared transactions", "standby feedback"},
		},
		{
			// Same column name on the replication screen, different formula, incomparable numbers.
			name:    "horizon_xacts is computed differently than on the replication screen",
			markers: []string{"age(backend_xmin)", "pg_last_committed_xact()", "replication report"},
		},
		{
			// The operationally heaviest one: blank may mean "not allowed to see", not "holds nothing".
			name:    "an empty cell may mean missing privileges",
			markers: []string{"unprivileged"},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			for _, m := range tc.markers {
				assert.Contains(t, pgStatActivityDescription, m,
					"caveat %q is missing or reworded: no %q in the block", tc.name, m)
			}
		})
	}
}

// Test_printStatSample_zeroWidthGuard pins the zero-width guard of the report
// truncation path to the semantics of its twin, top/printDataCell
// (top/stat.go:997-1016): when the column width reads as zero or negative the
// function returns an error and the cell is NOT printed — not truncated, not
// silently blank.
//
// view.ColsWidth is a map[int]int, so a key that was never computed yields 0
// without any signal, and `value[:width-1]` becomes `value[:-1]` — a slice
// bounds panic. A row wider than the layout the widths were computed from is
// exactly what a mid-archive version change produces, which is why report/
// must not stay the only unguarded truncation point.
func Test_printStatSample_zeroWidthGuard(t *testing.T) {
	newRes := func() *stat.PGresult {
		return &stat.PGresult{
			Valid: true, Ncols: 3, Nrows: 1,
			Cols: []string{"col_a", "col_b", "col_zero"},
			Values: [][]sql.NullString{
				{
					{String: "aaa", Valid: true},
					{String: "bbb", Valid: true},
					{String: "zero_width_cell", Valid: true},
				},
			},
		}
	}

	t.Run("missing width returns error and does not print the cell", func(t *testing.T) {
		v := view.New()["activity"]
		v.Cols = []string{"col_a", "col_b", "col_zero"}
		// No key for index 2: the map returns 0 for it.
		v.ColsWidth = map[int]int{0: 8, 1: 8}

		var buf bytes.Buffer
		n, err := printStatSample(&buf, newRes(), v, Config{}, time.Time{}, time.Second)

		require.Error(t, err)
		assert.Equal(t, 0, n)
		// Neither the whole value nor a truncated form of it may reach the writer.
		assert.NotContains(t, buf.String(), "zero_width_cell")
		assert.NotContains(t, buf.String(), "~")
	})

	t.Run("normal widths behave as before", func(t *testing.T) {
		v := view.New()["activity"]
		v.Cols = []string{"col_a", "col_b", "col_zero"}
		v.ColsWidth = map[int]int{0: 8, 1: 8, 2: 16}

		var buf bytes.Buffer
		n, err := printStatSample(&buf, newRes(), v, Config{}, time.Time{}, time.Second)

		assert.NoError(t, err)
		assert.Equal(t, 1, n)
		out := stripANSI(buf.String())
		assert.Contains(t, out, "aaa")
		assert.Contains(t, out, "bbb")
		assert.Contains(t, out, "zero_width_cell")
		assert.NotContains(t, out, "~")
	})

	t.Run("positive width still truncates", func(t *testing.T) {
		v := view.New()["activity"]
		v.Cols = []string{"col_a", "col_b", "col_zero"}
		v.ColsWidth = map[int]int{0: 8, 1: 8, 2: 4}

		var buf bytes.Buffer
		n, err := printStatSample(&buf, newRes(), v, Config{}, time.Time{}, time.Second)

		assert.NoError(t, err)
		assert.Equal(t, 1, n)
		assert.Contains(t, stripANSI(buf.String()), "zer~")
	})

	// The guard belongs INSIDE `if valuelen > width`, exactly as in
	// top/printDataCell: an empty value at a zero width has nothing to truncate
	// and must not raise an error. Hoisting the guard out of the branch would be
	// invisible to the subtests above, where every value is non-empty.
	t.Run("empty value at zero width is not an error", func(t *testing.T) {
		res := &stat.PGresult{
			Valid: true, Ncols: 3, Nrows: 1,
			Cols: []string{"col_a", "col_empty", "col_c"},
			Values: [][]sql.NullString{
				{
					{String: "aaa", Valid: true},
					{String: "", Valid: true},
					{String: "ccc", Valid: true},
				},
			},
		}

		v := view.New()["activity"]
		v.Cols = []string{"col_a", "col_empty", "col_c"}
		// No key for index 1, but its value is empty, so nothing is truncated.
		v.ColsWidth = map[int]int{0: 8, 2: 8}

		var buf bytes.Buffer
		n, err := printStatSample(&buf, res, v, Config{}, time.Time{}, time.Second)

		assert.NoError(t, err)
		assert.Equal(t, 1, n)
		out := stripANSI(buf.String())
		assert.Contains(t, out, "aaa")
		assert.Contains(t, out, "ccc")
	})
}

// activityColsPG12 is the 14-column activity layout recorded by PostgreSQL
// 10-12: 'state' sits at index 9.
var activityColsPG12 = []string{
	"pid", "cl_addr", "cl_port", "datname", "usename", "appname", "backend_type",
	"wait_etype", "wait_event", "state", "xact_age", "query_age", "change_age", "query",
}

// activityColsPG13 is the 17-column activity layout recorded by PostgreSQL 13+:
// three columns are inserted mid-layout, so 'state' moves to index 10 and index
// 9 — the index 'state' resolved to on the older layout — becomes 'wait_event'.
var activityColsPG13 = []string{
	"pid", "leader", "cl_addr", "cl_port", "datname", "usename", "appname", "backend_type",
	"wait_etype", "wait_event", "state", "backend_xid", "horizon_xacts",
	"xact_age", "query_age", "change_age", "query",
}

// activityTick is one recorded sample: the PostgreSQL version stored in the
// meta.* entry plus the activity result stored next to it.
type activityTick struct {
	version int
	cols    []string
	rows    [][]string
}

// activityTarBase is the timestamp of the first tick written by buildActivityTar.
var activityTarBase = time.Date(2026, 5, 19, 10, 0, 0, 0, time.Now().Location())

// buildActivityTar composes an in-memory recording in the tarRecorder.write()
// layout — meta.* + activity.* + sysinfo.* per tick, filenames in the recorder's
// 20060102T150405.000 format, ticks one second apart so itv == 1. Each tick
// carries its own recorded version, which is what lets a single archive span a
// major-version boundary the way `pgcenter record -a` across an upgrade does.
func buildActivityTar(t *testing.T, ticks []activityTick) *bytes.Buffer {
	t.Helper()

	metaCols := []string{"version", "version_num", "track_commit_timestamp", "max_connections", "autovacuum_max_workers", "recovery", "start_time_unix"}
	sysinfoBytes := []byte(`{"ticks":100,"cpu_count":4}`)

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)

	writeEntry := func(name string, payload []byte) {
		hdr := &tar.Header{Name: name, Size: int64(len(payload)), Mode: 0644}
		require.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write(payload)
		require.NoError(t, err)
	}

	for i, tick := range ticks {
		ts := activityTarBase.Add(time.Duration(i) * time.Second).Format("20060102T150405.000")

		// readMeta consumes only column index 1 (version_num); the rest mirrors
		// SelectCommonProperties so the entry has a realistic shape.
		metaBytes, err := json.Marshal(stat.PGresult{
			Valid: true, Ncols: len(metaCols), Nrows: 1, Cols: metaCols,
			Values: [][]sql.NullString{{
				{String: fmt.Sprintf("%d.0", tick.version/10000), Valid: true},
				{String: fmt.Sprintf("%d", tick.version), Valid: true},
				{String: "off", Valid: true}, {String: "100", Valid: true}, {String: "3", Valid: true},
				{String: "false", Valid: true}, {String: "1622828486655396e-6", Valid: true},
			}},
		})
		require.NoError(t, err)

		values := make([][]sql.NullString, len(tick.rows))
		for r, row := range tick.rows {
			require.Equal(t, len(tick.cols), len(row), "row %d of tick %d does not match its layout", r, i)
			values[r] = make([]sql.NullString, len(row))
			for c, v := range row {
				values[r][c] = sql.NullString{String: v, Valid: true}
			}
		}
		statBytes, err := json.Marshal(stat.PGresult{
			Valid: true, Ncols: len(tick.cols), Nrows: len(tick.rows),
			Cols: tick.cols, Values: values,
		})
		require.NoError(t, err)

		writeEntry("meta."+ts+".json", metaBytes)
		writeEntry("activity."+ts+".json", statBytes)
		writeEntry("sysinfo."+ts+".json", sysinfoBytes)
	}
	require.NoError(t, tw.Close())

	return &tarBuf
}

// runProcessDataOnTar feeds the archive through readTar in a goroutine and calls
// processData directly in the test goroutine. app.doReport is deliberately
// bypassed: it runs processData in a goroutine of its own, where a panic takes
// down the whole test binary instead of reddening a single test.
func runProcessDataOnTar(t *testing.T, config Config, tarBuf *bytes.Buffer) (string, error) {
	t.Helper()

	app := newApp(config)
	var buf bytes.Buffer
	app.writer = &buf

	dataCh := make(chan data)
	doneCh := make(chan struct{})
	readErrCh := make(chan error, 1)

	go func() {
		readErrCh <- readTar(tar.NewReader(tarBuf), config, dataCh, doneCh)
	}()

	err := processData(app, app.view, config, dataCh, doneCh)

	// On its error path processData returns without draining or closing the
	// channels, so readTar stays blocked on one of them. Drain until the reader
	// signals it is done, otherwise a failing assertion turns into a hang.
	// Terminating on doneCh is safe because readTar sends it from a defer, i.e.
	// only after its last send on dataCh has been consumed.
	if err != nil {
		go func() {
			for {
				select {
				case <-dataCh:
				case <-doneCh:
					return
				}
			}
		}()
	}

	select {
	case readErr := <-readErrCh:
		require.NoError(t, readErr)
	case <-time.After(10 * time.Second):
		require.FailNow(t, "readTar did not finish")
	}

	return buf.String(), err
}

// activityReplayConfig is the report config shared by the version-change tests.
func activityReplayConfig() Config {
	return Config{
		ReportType: "activity",
		TruncLimit: 32,
		TsStart:    activityTarBase.Add(-time.Hour),
		TsEnd:      activityTarBase.Add(time.Hour),
	}
}

// Test_processData_versionChange_recomputesLayout replays an archive that spans
// a 12 -> 13 boundary and asserts that the layout state the older samples left
// behind does not survive it: the header of the NEW layout must appear (which
// requires resetting the header-repeat counter, since only two lines were
// printed before the boundary — far short of repeatHeaderAfter), and the column
// widths must be recomputed from the new sample rather than inherited.
//
// Two ticks per version are required: the first tick of each version is consumed
// by the version-change branch as the prev snapshot, so a shorter archive would
// leave the test asserting against an empty report.
func Test_processData_versionChange_recomputesLayout(t *testing.T) {
	// 'usename' occupies index 4 of the old layout, where its longest value is
	// 3 characters, so the old width there is the column-name minimum of 8. In
	// the new layout index 4 is 'datname', whose value is 23 characters long:
	// inherited widths would truncate it to "very_lo~".
	oldRows := [][]string{
		{"1001", "10.0.0.1", "5432", "olddb", "bob", "psql", "client backend", "Client", "ClientRead", "active", "00:00:01", "00:00:01", "00:00:01", "select 1"},
		{"1002", "10.0.0.2", "5432", "olddb", "ann", "psql", "client backend", "Client", "ClientRead", "idle", "00:00:02", "00:00:02", "00:00:02", "select 2"},
	}
	newRows := [][]string{
		{"3003", "", "10.0.0.3", "5432", "very_long_database_name", "bob", "psql", "client backend", "Client", "ClientRead", "active", "1234", "42", "00:00:03", "00:00:03", "00:00:03", "select 3"},
		{"4004", "", "10.0.0.4", "5432", "very_long_database_name", "ann", "psql", "client backend", "Client", "ClientRead", "idle", "", "0", "00:00:04", "00:00:04", "00:00:04", "select 4"},
	}

	tarBuf := buildActivityTar(t, []activityTick{
		{version: 120000, cols: activityColsPG12, rows: oldRows},
		{version: 120000, cols: activityColsPG12, rows: oldRows},
		{version: 130000, cols: activityColsPG13, rows: newRows},
		{version: 130000, cols: activityColsPG13, rows: newRows},
	})

	out, err := runProcessDataOnTar(t, activityReplayConfig(), tarBuf)
	require.NoError(t, err)

	normalized := stripANSI(out)

	// The pre-boundary half of the report is present at all.
	assert.Contains(t, normalized, "olddb")

	// Header of the new layout: the three columns inserted at PG 13.
	assert.Contains(t, normalized, "leader")
	assert.Contains(t, normalized, "backend_xid")
	assert.Contains(t, normalized, "horizon_xacts")

	// It is printed immediately, i.e. before the first data row of the new
	// version — not after another repeatHeaderAfter lines have accumulated.
	hdrPos := strings.Index(normalized, "horizon_xacts")
	rowPos := strings.Index(normalized, "very_long_database_name")
	require.NotEqual(t, -1, hdrPos, "header of the new layout is missing")
	require.NotEqual(t, -1, rowPos, "data rows of the new version are missing")
	assert.Less(t, hdrPos, rowPos, "new header must precede the first row of the new version")

	// ...and it is a SECOND header rather than the only one: it follows the rows
	// printed under the old layout, so the pre-boundary half kept its own header.
	oldRowPos := strings.Index(normalized, "olddb")
	require.NotEqual(t, -1, oldRowPos, "data rows of the old version are missing")
	assert.Less(t, oldRowPos, hdrPos,
		"new header must be printed after the pre-boundary rows, not instead of the old header")

	// Widths recomputed from the new sample: the value that moved into a
	// narrower column of the old layout is not truncated by the old width.
	assert.NotContains(t, normalized, "very_lo~")
}

// Test_processData_versionChange_reresolvesOrderColumn asserts that -o is
// re-resolved against the new column list after the boundary. 'state' is index 9
// on PG 12 and index 10 on PG 13, where index 9 is 'wait_event' — so a surviving
// index silently sorts the report by a column the operator never asked for.
//
// The values discriminate all THREE candidate orders, which is what makes the
// test load-bearing for the latch specifically. Re-resolving -o against the new
// list sorts by 'state' descending and puts pid 3003 ("idle") first; a surviving
// index 9 sorts by 'wait_event' descending and puts 4004 ("ClientRead") first;
// and merely falling back to the screen default without re-resolving sorts by
// pid descending, which puts 4004 first too. Without that third case separated,
// an implementation that never re-resolves would pass.
func Test_processData_versionChange_reresolvesOrderColumn(t *testing.T) {
	oldRows := [][]string{
		{"1001", "10.0.0.1", "5432", "olddb", "bob", "psql", "client backend", "Client", "ClientRead", "active", "00:00:01", "00:00:01", "00:00:01", "select 1"},
		{"1002", "10.0.0.2", "5432", "olddb", "ann", "psql", "client backend", "Client", "ClientRead", "idle", "00:00:02", "00:00:02", "00:00:02", "select 2"},
	}
	newRows := [][]string{
		{"3003", "", "10.0.0.3", "5432", "newdb", "bob", "psql", "client backend", "Client", "AAAsync", "idle", "1234", "42", "00:00:03", "00:00:03", "00:00:03", "select 3"},
		{"4004", "", "10.0.0.4", "5432", "newdb", "ann", "psql", "client backend", "Client", "ClientRead", "active", "", "0", "00:00:04", "00:00:04", "00:00:04", "select 4"},
	}

	tarBuf := buildActivityTar(t, []activityTick{
		{version: 120000, cols: activityColsPG12, rows: oldRows},
		{version: 120000, cols: activityColsPG12, rows: oldRows},
		{version: 130000, cols: activityColsPG13, rows: newRows},
		{version: 130000, cols: activityColsPG13, rows: newRows},
	})

	config := activityReplayConfig()
	config.OrderColName = "state"
	config.OrderDesc = true

	out, err := runProcessDataOnTar(t, config, tarBuf)
	require.NoError(t, err)

	normalized := stripANSI(out)
	posIdle := strings.Index(normalized, "3003")
	posActive := strings.Index(normalized, "4004")
	require.NotEqual(t, -1, posIdle, "row 3003 is missing from the report")
	require.NotEqual(t, -1, posActive, "row 4004 is missing from the report")
	assert.Less(t, posIdle, posActive,
		"after the boundary rows must be ordered by state re-resolved against the new layout — "+
			"not by the stale index (wait_event) and not by the screen default (pid)")
}

// Test_processData_versionChange_orderColumnMissing covers the case the latch
// alone cannot fix: the requested -o column does not exist in the new layout, so
// getColumnIndex fails, the latch simply stays down and the index resolved
// against the OLD layout would survive. The archive runs 13 -> 12 (an archive
// stitched across a downgrade; the branch compares versions for inequality, not
// order) with -o horizon_xacts, which is index 12 on PG 13 and absent on PG 12,
// where index 12 is 'change_age'.
//
// The values distinguish all three states: sorted by the screen default (pid,
// descending — the seed restored from view.New()) 4004 comes first, while both
// a surviving index 12 and a restored index 0 with a surviving ascending
// direction put 3003 first.
func Test_processData_versionChange_orderColumnMissing(t *testing.T) {
	// Named by position relative to the boundary, not by PostgreSQL version: this
	// archive runs 13 -> 12, so the rows recorded FIRST are the 17-column ones.
	beforeBoundaryRows := [][]string{
		{"1001", "", "10.0.0.1", "5432", "olddb", "bob", "psql", "client backend", "Client", "ClientRead", "active", "1234", "5", "00:00:01", "00:00:01", "00:00:01", "select 1"},
		{"1002", "", "10.0.0.2", "5432", "olddb", "ann", "psql", "client backend", "Client", "ClientRead", "idle", "", "9", "00:00:02", "00:00:02", "00:00:02", "select 2"},
	}
	afterBoundaryRows := [][]string{
		{"3003", "10.0.0.3", "5432", "newdb", "bob", "psql", "client backend", "Client", "ClientRead", "active", "00:00:03", "00:00:03", "00:00:01", "select 3"},
		{"4004", "10.0.0.4", "5432", "newdb", "ann", "psql", "client backend", "Client", "ClientRead", "idle", "00:00:04", "00:00:04", "00:00:09", "select 4"},
	}

	tarBuf := buildActivityTar(t, []activityTick{
		{version: 130000, cols: activityColsPG13, rows: beforeBoundaryRows},
		{version: 130000, cols: activityColsPG13, rows: beforeBoundaryRows},
		{version: 120000, cols: activityColsPG12, rows: afterBoundaryRows},
		{version: 120000, cols: activityColsPG12, rows: afterBoundaryRows},
	})

	config := activityReplayConfig()
	config.OrderColName = "horizon_xacts"
	config.OrderDesc = false

	out, err := runProcessDataOnTar(t, config, tarBuf)
	require.NoError(t, err)

	normalized := stripANSI(out)
	posHigh := strings.Index(normalized, "4004")
	posLow := strings.Index(normalized, "3003")
	require.NotEqual(t, -1, posHigh, "row 4004 is missing from the report")
	require.NotEqual(t, -1, posLow, "row 3003 is missing from the report")
	assert.Less(t, posHigh, posLow, "after the boundary the screen default sort key must be restored")
}

// Test_app_doReport_errorPathDoesNotHang drives the real app.doReport, which the other
// tests in this file deliberately bypass — and that bypass is exactly why the hang went
// unnoticed: runProcessDataOnTar drains the channels itself, so it never reproduces what
// production does.
//
// The archive widens between two samples of the SAME version, so the alignment computed
// for the narrow layout leaves the extra columns without a width. The zero-width guard in
// printStatSample turns what used to be a `[:-1]` panic into a returned error — and an
// error out of processData leaves readTar blocked on two unbuffered channels with no
// receiver, so before the drain was added the command hung forever instead of exiting.
// A silent hang is worse than a crash for a CLI in a pipe.
func Test_app_doReport_errorPathDoesNotHang(t *testing.T) {
	narrow := []string{"pid", "datname"}
	wide := []string{"pid", "datname", "usename", "state"}

	tarBuf := buildActivityTar(t, []activityTick{
		{version: 170000, cols: narrow, rows: [][]string{{"100", "postgres"}}},
		{version: 170000, cols: narrow, rows: [][]string{{"100", "postgres"}}},
		{version: 170000, cols: wide, rows: [][]string{{"100", "a_database_name_wider_than_the_column", "postgres", "active"}}},
		{version: 170000, cols: wide, rows: [][]string{{"100", "a_database_name_wider_than_the_column", "postgres", "active"}}},
	})

	app := &app{
		config: Config{ReportType: "activity", TsStart: time.Unix(0, 0), TsEnd: time.Now().Add(time.Hour), TruncLimit: 32},
		view:   view.New()["activity"],
		writer: io.Discard,
	}

	done := make(chan error, 1)
	go func() { done <- app.doReport(tar.NewReader(bytes.NewReader(tarBuf.Bytes()))) }()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("doReport did not return: readTar is still blocked, the command would hang forever")
	}
}
