// Code related to 'pgcenter record' command

package report

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/align"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
	"io"
	"log"
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
	ascFlag           = "+"
)

// RunMain is the main entry point for 'pgcenter report' sub-command
func RunMain(c Config) error {
	app := newApp(c)

	f, err := os.Open(c.InputFile)
	if err != nil {
		log.Fatalf("ERROR: failed to open file: %s\n", err)
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
	var prevStat, diffStat stat.PGresult
	var prevTs time.Time
	var linesPrinted = repeatHeaderAfter // initial value means print header at the beginning of all output

	c := app.config
	v := app.view

	// read files headers continuously, read stats files requested by user and skip others.
	for {
		hdr, err := r.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("failed to advance position within tar file: %s", err)
		}

		// check stats filename, skip files if their names doesn't contain name of requested statistics
		if !strings.Contains(hdr.Name, c.ReportType) {
			continue
		}

		s := strings.Split(hdr.Name, ".")
		if len(s) != 3 {
			fmt.Printf("bad file name format %s, skip", hdr.Name)
			continue
		}

		// Calculate timestamp when stats were recorded, parse timestamp considering it is in local timezone.
		zone, _ := time.Now().Zone()
		currTs, err := time.Parse("20060102T150405-07", s[1]+zone)
		if err != nil {
			return fmt.Errorf("failed to parse timestamp from filename %s: %s", hdr.Name, err)
		}

		// skip snapshots if they're outside of the requested time interval
		if currTs.Before(c.TsStart) || currTs.After(c.TsEnd) {
			continue
		}

		// read stats to a buffer
		data := make([]byte, hdr.Size)
		if _, err := io.ReadFull(r, data); err != nil {
			return fmt.Errorf("failed to read stat from %s: %s", hdr.Name, err)
		}

		// initialize an empty struct and unmarshal data from the buffer
		currStat := stat.PGresult{}
		if err = json.Unmarshal(data, &currStat); err != nil {
			return fmt.Errorf("break on %s: failed to unmarshal data from buffer: %s", hdr.Name, err)
		}

		// if previous stats snapshot is not defined, copy current to previous (when reading first snapshot at startup, for example)
		if prevStat.Valid != true {
			prevStat = currStat
			prevTs = currTs
			continue
		}

		// calculate time interval
		interval := currTs.Sub(prevTs)
		if c.Interval > interval {
			fmt.Println("WARNING: specified interval too long, adjusting it to an interval equal between current and previous statistics snapshots")
			c.Interval = interval
		}

		// calculate delta between current and previous stats snapshots
		if v.DiffIntvl != [2]int{0, 0} {
			res, err := stat.Compare(currStat, prevStat, int(interval/c.Interval), v.DiffIntvl, v.OrderKey, v.OrderDesc, v.UniqueKey)
			if err != nil {
				return fmt.Errorf("failed diff on %s: %s", hdr.Name, err)
			}
			diffStat = res
		} else {
			diffStat = currStat
		}

		// when diff done and previous snapshot is not needed, replace it with current snapshot
		prevStat = currStat
		prevTs = currTs

		// formatting  the report
		formatReport(&diffStat, &v, c)

		// print header after every Nth lines
		linesPrinted, err = printStatHeader(app.writer, linesPrinted, v)
		if err != nil {
			return err
		}

		// print the stats - calculated delta between previous and current stats snapshots
		//linesPrinted += printStatReport(&diffStat, v, c, currTs)
		n, err := printStatReport(app.writer, &diffStat, v, c, currTs)
		if err != nil {
			return err
		}
		linesPrinted += n
	} //end for

	return nil
}

// formatReport does report formatting - sort and aligning
func formatReport(d *stat.PGresult, view *view.View, c Config) {
	if c.OrderColName != "" {
		doSort(d, c)
	}

	// align values for printing, use dynamic aligning
	if !view.Aligned {
		widthes, cols := align.SetAlign(*d, c.TruncLimit, true)
		view.ColsWidth = widthes
		view.Cols = cols
		view.Aligned = true
	}
}

// printStatHeader periodically prints names of stats columns
func printStatHeader(w io.Writer, printedNum int, v view.View) (int, error) {
	if printedNum < repeatHeaderAfter || !v.Aligned {
		return printedNum, nil
	}

	fmt.Printf("         ")
	for i, name := range v.Cols {
		_, err := fmt.Fprintf(w, "\033[%d;%dm%-*s\033[0m", 37, 1, v.ColsWidth[i]+2, name)
		if err != nil {
			return 0, err
		}
	}

	_, err := fmt.Fprintf(w, "\n")
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

// Perform sort of statistics based on column requested by user
// TODO: refactor sort to configure in cmd package instead of in-place sorting.
func doSort(stat *stat.PGresult, c Config) {
	//var sortKey int
	//
	//// set ascending order if required
	//if opts.OrderColName[0] == ascFlag[0] {
	//	opts.OrderDesc = false // set to Asc
	//	opts.OrderColName = strings.TrimLeft(opts.OrderColName, ascFlag)
	//}
	//
	//for k, v := range stat.Cols {
	//	if v == opts.OrderColName {
	//		sortKey = k
	//		break
	//	}
	//}

	// --- sort already performed in stat.Compare() method.

	// use descending order by default
	//stat.Sort(sortKey, opts.OrderDesc)
}
