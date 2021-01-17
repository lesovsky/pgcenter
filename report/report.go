package report

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/align"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// Options contains settings of the requested report
type Config struct {
	InputFile     string
	TsStart       time.Time
	TsEnd         time.Time
	OrderColName  string
	OrderDesc     bool
	FilterColName string
	FilterRE      *regexp.Regexp
	TruncLimit    int
	RowLimit      int
	ReportType    string
	Interval      time.Duration
}

const (
	repeatHeaderAfter = 20
)

// RunMain is the main entry point for 'pgcenter report' sub-command
func RunMain(c Config) error {
	app := newApp(c)

	f, err := os.Open(c.InputFile)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Printf("INFO: reading from %s\n", c.InputFile)
	fmt.Printf("INFO: report %s\n", c.ReportType)
	fmt.Printf(
		"INFO: start from: %s, to: %s, with interval: %s\n",
		c.TsStart.Format("2006-01-02 15:04:05 MST"),
		c.TsEnd.Format("2006-01-02 15:04:05 MST"),
		c.Interval.String(),
	)

	// initialize tar reader
	tr := tar.NewReader(f)

	// do report
	return app.doReport(tr)
}

// app defines 'pgcenter record' runtime dependencies.
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

// Read statistics file and create a report based on report settings
func (app *app) doReport(r *tar.Reader) error {
	var prevStat stat.PGresult
	var prevTs time.Time
	var linesPrinted = repeatHeaderAfter // initial value means print header at the beginning of all output
	var orderConfigured = false          // flag tells about order is not configured.

	c := app.config
	v := app.view

	// read files headers continuously, read stats files requested by user and skip others.
	for {
		hdr, err := r.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("advance read position failed: %s", err)
		}

		// Check filename - it has valid format and corresponds to requested report type.
		err = isFilenameOK(hdr.Name, c.ReportType)
		if err != nil {
			continue
		}

		// Check timestamp in filename, is it correct and is in requested report interval.
		ts, err := isFilenameTimestampOK(hdr.Name, c.TsStart, c.TsEnd)
		if err != nil {
			continue
		}

		// Read stats from file.
		currStat, err := readFileStat(r, hdr.Size)
		if err != nil {
			return err
		}

		// if previous stats snapshot is not defined, copy current to previous.
		// Usually this occurs when reading first stat sample at startup.
		if prevStat.Valid != true {
			prevStat = currStat
			prevTs = ts
			continue
		}

		// Calculate time interval.
		interval := ts.Sub(prevTs)
		if c.Interval > interval {
			fmt.Println("WARNING: specified interval too long, adjusting it to an interval equal between current and previous statistics snapshots")
			c.Interval = interval
		}

		// When first data read, list of columns is known and it is possible to set up order.
		if c.OrderColName != "" && !orderConfigured {
			if idx, ok := getColumnIndex(currStat.Cols, c.OrderColName); ok {
				v.OrderKey = idx
				v.OrderDesc = c.OrderDesc
				orderConfigured = true
			}
		}

		// Calculate delta between current and previous stats snapshots.
		diffStat, err := countDiff(currStat, prevStat, int(interval/c.Interval), v)
		if err != nil {
			return err
		}

		// Format the stat
		formatStatSample(&diffStat, &v, c)

		// print header after every Nth lines
		linesPrinted, err = printStatHeader(app.writer, linesPrinted, v)
		if err != nil {
			return err
		}

		// print the stats - calculated delta between previous and current stats snapshots
		n, err := printStatReport(app.writer, &diffStat, v, c, ts)
		if err != nil {
			return err
		}
		linesPrinted += n

		// Swap previous with current
		prevStat = currStat
		prevTs = ts
	} //end for

	return nil
}

//
func isFilenameOK(name string, report string) error {
	s := strings.Split(name, ".")

	// File name should be in the format: 'report_type.timestamp.json'
	if len(s) != 3 {
		return fmt.Errorf("bad file name format %s, skip", name)
	}

	// Is filename correspond to user-requested report?
	if s[0] != report {
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
	zone, _ := time.Now().Zone()
	ts, err := time.Parse("20060102T150405-07", s[1]+zone)
	if err != nil {
		return time.Time{}, err
	}

	// skip snapshots if they're outside of the requested time interval
	if ts.Before(start) || ts.After(end) {
		return time.Time{}, fmt.Errorf("out of the requested interval")
	}

	return ts, nil
}

// readFileStat reads content of tar file, unmarshal data and return stat object.
func readFileStat(r *tar.Reader, bufsz int64) (stat.PGresult, error) {
	data := make([]byte, bufsz)

	if _, err := io.ReadFull(r, data); err != nil {
		return stat.PGresult{}, err
	}

	// initialize an empty struct and unmarshal data from the buffer
	res := stat.PGresult{}
	err := json.Unmarshal(data, &res)
	if err != nil {
		return stat.PGresult{}, err
	}

	return res, nil
}

// countDiff compares two stat samples and produce differential sample.
func countDiff(curr, prev stat.PGresult, interval int, v view.View) (stat.PGresult, error) {
	var diff stat.PGresult

	if v.DiffIntvl != [2]int{0, 0} {
		res, err := stat.Compare(curr, prev, interval, v.DiffIntvl, v.OrderKey, v.OrderDesc, v.UniqueKey)
		if err != nil {
			return stat.PGresult{}, err
		}
		diff = res
	} else {
		diff = curr
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

// printStatReport prints given stats
func printStatReport(w io.Writer, res *stat.PGresult, view view.View, c Config, ts time.Time) (int, error) {
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
