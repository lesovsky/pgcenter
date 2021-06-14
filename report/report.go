package report

import (
	"archive/tar"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/align"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config contains application settings.
type Config struct {
	Describe      bool
	ReportType    string
	InputFile     string
	TsStart       time.Time
	TsEnd         time.Time
	OrderColName  string
	OrderDesc     bool
	FilterColName string
	FilterRE      *regexp.Regexp
	RowLimit      int
	TruncLimit    int
	Rate          time.Duration
}

const (
	// repeatHeaderAfter defines number of lines after which header should be printed again.
	repeatHeaderAfter = 20
)

// RunMain is the main entry point for 'pgcenter report' sub-command.
func RunMain(c Config) error {
	app := newApp(c)

	// Print report description if requested.
	if c.Describe {
		return describeReport(app.writer, c.ReportType)
	}

	// Open file with statistics.
	f, err := os.Open(c.InputFile)
	if err != nil {
		return err
	}

	defer func() {
		err := f.Close()
		if err != nil {
			fmt.Printf("close file descriptor failed: %s, ignore", err)
		}
	}()

	// Print report header.
	err = printReportHeader(app.writer, app.config)
	if err != nil {
		return err
	}

	// Initialize tar reader.
	tr := tar.NewReader(f)

	// Start printing report.
	return app.doReport(tr)
}

// app defines application container with runtime dependencies.
type app struct {
	config Config
	view   view.View
	writer io.Writer
}

// newApp creates new 'pgcenter record' app.
func newApp(config Config) *app {
	views := view.New()
	v := views[config.ReportType]

	return &app{
		config: config,
		view:   v,
		writer: os.Stdout,
	}
}

// metadata defines metadata of stats snapshot
type metadata struct {
	version int // version reflects Postgres version
}

// data defines unit of stats portion transmitted through channel from stats reader to stats processor.
type data struct {
	ts   time.Time
	res  stat.PGresult
	meta metadata
}

// Read statistics file and create a report based on report settings
func (app *app) doReport(r *tar.Reader) error {
	c := app.config
	v := app.view

	dataCh := make(chan data)
	doneCh := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		err := readTar(r, c, dataCh, doneCh)
		if err != nil {
			fmt.Println(err)
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		err := processData(app, v, c, dataCh, doneCh)
		if err != nil {
			fmt.Println(err)
		}
		wg.Done()
	}()

	wg.Wait()
	return nil
}

// readTar reads stats and metadata from tar stream and send it to data channel.
func readTar(r *tar.Reader, config Config, dataCh chan data, doneCh chan struct{}) error {
	var metaOK, statOK bool
	var meta metadata
	var res stat.PGresult

	defer func() { doneCh <- struct{}{} }()

	for {
		hdr, err := r.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("advance read position failed: %s", err)
		}

		// Check filename - it has valid format and corresponds to requested report type.
		err = isFilenameOK(hdr.Name, config.ReportType)
		if err != nil {
			continue
		}

		// Check timestamp in filename, is it correct and is in requested report interval.
		ts, err := isFilenameTimestampOK(hdr.Name, config.TsStart, config.TsEnd)
		if err != nil {
			continue
		}

		// Read metadata from file.
		if strings.HasPrefix(hdr.Name, "meta.") {
			res, err := stat.NewPGresultFile(r, hdr.Size)
			if err != nil {
				return err
			}

			m, err := readMeta(res)
			if err != nil {
				return err
			}

			metaOK, meta = true, m
		} else {
			// Read stats from file.
			res, err = stat.NewPGresultFile(r, hdr.Size)
			if err != nil {
				return err
			}
			statOK = true
		}

		if !metaOK || !statOK {
			continue
		}

		// Send stats and meta, and reset flags.
		dataCh <- data{ts: ts, res: res, meta: meta}

		metaOK, statOK = false, false

	} //end for

	return nil
}

// processData receives stats from data channel and print it.
func processData(app *app, v view.View, config Config, dataCh chan data, doneCh chan struct{}) error {
	var prevMeta metadata
	var prevStat stat.PGresult
	var prevTs time.Time
	linesPrinted := repeatHeaderAfter // initial value means print header at the beginning of all output
	orderConfigured := false          // flag tells about order is not configured.

	// waiting for stats, or message about reader is done
	for {
		select {
		case d := <-dataCh:
			// If previous stats snapshot is not defined, copy current to previous.
			// Usually this occurs when reading first stat sample at startup.

			// Also checking version of stats in metadata, if it's different also discard previous.
			if !prevStat.Valid || prevMeta.version != d.meta.version {
				prevMeta = d.meta
				prevStat = d.res
				prevTs = d.ts

				views := view.Views{
					config.ReportType: v,
				}
				err := views.Configure(query.Options{
					Version: d.meta.version,
				})
				if err != nil {
					return err
				}

				v = views[config.ReportType]

				continue
			}

			// Calculate time interval.
			interval := d.ts.Sub(prevTs)
			if config.Rate > interval {
				_, err := fmt.Fprintf(
					app.writer,
					"WARNING: specified rate longer than stats snapshots interval, adjusting it to %s\n",
					interval.String(),
				)
				if err != nil {
					return err
				}
				config.Rate = interval
			}

			// When first data read, list of columns is known and it is possible to set up order.
			if config.OrderColName != "" && !orderConfigured {
				if idx, ok := getColumnIndex(d.res.Cols, config.OrderColName); ok {
					v.OrderKey = idx
					v.OrderDesc = config.OrderDesc
					orderConfigured = true
				}
			}

			// Calculate delta between current and previous stats snapshots.
			diffStat, err := countDiff(d.res, prevStat, int(interval/config.Rate), v)
			if err != nil {
				return err
			}

			// Format the stat
			formatStatSample(&diffStat, &v, config)

			// print header after every Nth lines
			linesPrinted, err = printStatHeader(app.writer, linesPrinted, v)
			if err != nil {
				return err
			}

			// print the stats - calculated delta between previous and current stats snapshots
			n, err := printStatSample(app.writer, &diffStat, v, config, d.ts)
			if err != nil {
				return err
			}
			linesPrinted += n

			// Swap previous with current
			prevStat = d.res
			prevTs = d.ts
		case <-doneCh:
			close(dataCh)
			return nil
		}
	}
}

// readMeta creates metadata object from stat.PGresult.
func readMeta(res stat.PGresult) (metadata, error) {
	if res.Nrows != 1 || res.Ncols != 7 {
		return metadata{}, fmt.Errorf("invalid result")
	}

	version, err := strconv.ParseInt(res.Values[0][1].String, 10, 64)
	if err != nil {
		return metadata{}, err
	}

	return metadata{version: int(version)}, nil
}

// isFilenameOK checks filename format.
func isFilenameOK(name string, report string) error {
	s := strings.Split(name, ".")

	// File name should be in the format: 'report_type.timestamp.json'.
	if len(s) != 3 {
		return fmt.Errorf("bad file name format %s, skip", name)
	}

	// Check the filename corresponds to user-requested report or metadata.
	if s[0] != report && s[0] != "meta" {
		return fmt.Errorf("skip sample")
	}

	return nil
}

// isFilenameTimestampOK validates that timestamp in filename is valid and is in interval.
func isFilenameTimestampOK(name string, start, end time.Time) (time.Time, error) {
	s := strings.Split(name, ".")

	// File name should be in the format: 'report_type.timestamp.json'
	if len(s) != 3 {
		return time.Time{}, fmt.Errorf("bad file name format %s, skip", name)
	}

	// Calculate timestamp when stats were recorded, parse timestamp considering it is in local timezone.
	ts, err := time.ParseInLocation("20060102T150405", s[1], time.Now().Location())
	if err != nil {
		return time.Time{}, err
	}

	// skip snapshots if they're outside of the requested time interval
	if ts.Before(start) || ts.After(end) {
		return time.Time{}, fmt.Errorf("out of the requested interval")
	}

	return ts, nil
}

// countDiff compares two stat samples and produce differential sample.
func countDiff(curr, prev stat.PGresult, interval int, v view.View) (stat.PGresult, error) {
	var diff stat.PGresult

	diff, err := stat.Compare(curr, prev, interval, v.DiffIntvl, v.OrderKey, v.OrderDesc, v.UniqueKey)
	if err != nil {
		return stat.PGresult{}, err
	}

	return diff, nil
}

// getColumnIndex return index of specified column in set of columns.
func getColumnIndex(cols []string, colname string) (int, bool) {
	if colname == "" {
		return -1, false
	}

	for i, val := range cols {
		if val == colname {
			return i, true
		}
	}
	return -1, false
}

// formatStatSample does formatting of stat sample.
func formatStatSample(d *stat.PGresult, view *view.View, c Config) {
	if view.Aligned {
		return
	}

	// align values for printing, use dynamic aligning
	widthes, cols := align.SetAlign(*d, c.TruncLimit, true)
	view.ColsWidth = widthes
	view.Cols = cols
	view.Aligned = true
}

// printReportHeader prints report header.
func printReportHeader(w io.Writer, c Config) error {
	tmpl := "INFO: reading from %s\n" +
		"INFO: report %s\n" +
		"INFO: start from: %s, to: %s, with rate: %s\n"
	msg := fmt.Sprintf(tmpl,
		c.InputFile,
		c.ReportType,
		c.TsStart.Format("2006-01-02 15:04:05 MST"),
		c.TsEnd.Format("2006-01-02 15:04:05 MST"),
		c.Rate.String(),
	)

	_, err := fmt.Fprint(w, msg)
	if err != nil {
		return err
	}
	return nil
}

// printStatHeader periodically prints names of stats columns
func printStatHeader(w io.Writer, printedNum int, v view.View) (int, error) {
	if printedNum < repeatHeaderAfter || !v.Aligned {
		return printedNum, nil
	}

	_, err := fmt.Fprintf(w, "         ")
	if err != nil {
		return 0, err
	}

	for i, name := range v.Cols {
		_, err := fmt.Fprintf(w, "\033[%d;%dm%-*s\033[0m", 37, 1, v.ColsWidth[i]+2, name)
		if err != nil {
			return 0, err
		}
	}

	_, err = fmt.Fprintf(w, "\n")
	if err != nil {
		return 0, err
	}
	return 0, nil
}

// printStatSample prints given stats
func printStatSample(w io.Writer, res *stat.PGresult, view view.View, c Config, ts time.Time) (int, error) {
	// print stats values
	var printFirst = true // every first line in the snapshot should begin with timestamp when stats were taken
	var linesPrinted int  // count lines printed per snapshot (for limiting purposes)
	var printedNum int

	// loop through the rows, check for filtered values and print if values are satisfied
	for colnum, rownum := 0, 0; rownum < res.Nrows; rownum, colnum = rownum+1, 0 {
		var doPrint = true // assume the filtering is disabled by default and row should be printed

		// if filtering (grep) is enabled, a target column should be found and check values
		// if value doesn't match, skip it and proceed to next row
		if c.FilterColName != "" {
			// if filter enabled, use pessimistic approach and considering the value will not match
			doPrint = false
			for idx, colname := range res.Cols {
				if colname == c.FilterColName {
					if c.FilterRE.MatchString(res.Values[rownum][idx].String) {
						doPrint = true // value matched, so print the whole row
						break
					}
				}
			}
		}

		// print the row
		if doPrint {
			if printFirst {
				_, err := fmt.Fprintf(w, "%s ", ts.Format("15:04:05"))
				if err != nil {
					return 0, err
				}
				printFirst = false
			} else {
				_, err := fmt.Fprintf(w, "         ")
				if err != nil {
					return 0, err
				}
			}

			for i := range res.Cols {
				// truncate values that longer than column width
				valuelen := len(res.Values[rownum][colnum].String)
				if valuelen > view.ColsWidth[i] {
					width := view.ColsWidth[i]
					// truncate value up to column width and replace last character with '~' symbol
					res.Values[rownum][colnum].String = res.Values[rownum][colnum].String[:width-1] + "~"
				}

				// last col with no truncation of not specified otherwise
				if i != len(res.Cols)-1 {
					_, err := fmt.Fprintf(w, "%-*s", view.ColsWidth[i]+2, res.Values[rownum][colnum].String)
					if err != nil {
						return 0, err
					}
				} else {
					_, err := fmt.Fprintf(w, "%s", res.Values[rownum][colnum].String)
					if err != nil {
						return 0, err
					}
				}

				colnum++
			}

			_, err := fmt.Fprintf(w, "\n")
			if err != nil {
				return 0, err
			}
			printedNum++

			// check number of printed lines, if limit is reached skip remaining rows and proceed to a next stats file
			if linesPrinted++; c.RowLimit > 0 && linesPrinted >= c.RowLimit {
				break
			}
		} // end if
	} // end for

	return printedNum, nil
}

// doDescribe shows detailed description of the requested stats
func describeReport(w io.Writer, report string) error {
	m := map[string]string{
		"databases_general":   pgStatDatabaseGeneralDescription,
		"databases_sessions":  pgStatDatabaseSessionsDescription,
		"activity":            pgStatActivityDescription,
		"replication":         pgStatReplicationDescription,
		"tables":              pgStatTablesDescription,
		"indexes":             pgStatIndexesDescription,
		"functions":           pgStatFunctionsDescription,
		"wal":                 pgStatWALDescription,
		"sizes":               pgStatSizesDescription,
		"progress_vacuum":     pgStatProgressVacuumDescription,
		"progress_cluster":    pgStatProgressClusterDescription,
		"progress_index":      pgStatProgressCreateIndexDescription,
		"progress_analyze":    pgStatProgressAnalyzeDescription,
		"progress_basebackup": pgStatProgressBasebackupDescription,
		"progress_copy":       pgStatProgressCopyDescription,
		"statements_timings":  pgStatStatementsTimingsDescription,
		"statements_general":  pgStatStatementsGeneralDescription,
		"statements_io":       pgStatStatementsIODescription,
		"statements_local":    pgStatStatementsLocalDescription,
		"statements_temp":     pgStatStatementsTempDescription,
		"statements_wal":      pgStatStatementsWalDescription,
	}

	if description, ok := m[report]; ok {
		_, err := fmt.Fprint(w, description)
		if err != nil {
			return err
		}
	} else {
		_, err := fmt.Fprint(w, "unknown description requested")
		if err != nil {
			return err
		}
	}

	return nil
}
