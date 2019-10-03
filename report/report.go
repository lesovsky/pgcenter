// Code related to 'pgcenter record' command

package report

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"github.com/lesovsky/pgcenter/lib/stat"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
)

// ReportOptions contains settings of the requested report
type ReportOptions struct {
	InputFile     string
	TsStart       time.Time
	TsEnd         time.Time
	OrderColName  string
	OrderDesc     bool
	FilterColName string
	Regexp        *regexp.Regexp
	TruncLimit    int
	RowLimit      int
	ReportType    string
	Context       stat.ContextUnit
	Interval      time.Duration
}

const (
	repeatHeaderAfter = 20
	ascFlag           = "+"
)

// RunMain is the main entry point for 'pgcenter report' sub-command
func RunMain(args []string, opts ReportOptions) {
	f, err := os.Open(opts.InputFile)
	if err != nil {
		log.Fatalf("ERROR: failed to open file: %s\n", err)
	}
	defer f.Close()

	fmt.Printf("INFO: reading from %s\n", opts.InputFile)
	fmt.Printf("INFO: report %s\n", opts.ReportType)
	fmt.Printf("INFO: start from: %s, end at: %s, delta interval: %s\n", opts.TsStart, opts.TsEnd, opts.Interval.String())

	// initialize tar reader
	tr := tar.NewReader(f)

	// report loop
	if err := doReport(tr, opts); err != nil {
		log.Fatalln(err)
	}
}

// Read statistics file and create a report based on report settings
func doReport(r *tar.Reader, opts ReportOptions) error {
	var prevStat, diffStat stat.PGresult
	var prevTs time.Time
	var linesPrinted int8

	// read files headers continuously, read stats files requested by user and skip others.
	for {
		hdr, err := r.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("failed to advance position within tar file: %s", err)
		}

		// check stats filename, skip files if their names doesn't contain name of requested statistics
		if !strings.Contains(hdr.Name, opts.ReportType) {
			continue
		}

		// calculate timestamp when stats were recorded
		layout := "20060102T150405"
		s := strings.Split(hdr.Name, ".")
		currTs, err := time.Parse(layout, s[1])
		if err != nil {
			return fmt.Errorf("failed to parse timestamp from filename %s: %s", hdr.Name, err)
		}

		// skip snapshots if they're outside of the requested time interval
		if !(currTs.After(opts.TsStart) && currTs.Before(opts.TsEnd)) {
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
		if opts.Interval > interval {
			fmt.Println("WARNING: specified interval too long, adjusting it to an interval equal between current and previous statistics snapshots")
			opts.Interval = interval
		}

		// calculate delta between current and previous stats snapshots
		if opts.Context.DiffIntvl != stat.NoDiff {
			if err := diffStat.Diff(&prevStat, &currStat, uint(interval/opts.Interval), opts.Context.DiffIntvl, opts.Context.UniqueKey); err != nil {
				return fmt.Errorf("failed diff on %s: %s", hdr.Name, err)
			}
		} else {
			diffStat = currStat
		}

		// when diff done and previous snapshot is not needed, replace it with current snapshot
		prevStat = currStat
		prevTs = currTs

		// formatting  the report
		formatReport(&diffStat, &opts)

		// print header after every Nth lines
		linesPrinted = printStatHeader(linesPrinted, diffStat.Cols, opts)

		// print the stats - calculated delta between previous and current stats snapshots
		linesPrinted += printStatReport(&diffStat, opts, currTs)
	} //end for

	return nil
}

// formatReport does report formatting - sort and aligning
func formatReport(d *stat.PGresult, opts *ReportOptions) {
	if opts.OrderColName != "" {
		doSort(d, opts)
	}

	// align values for printing, use dynamic aligning
	if !opts.Context.Aligned {
		err := d.SetAlign(opts.Context.ColsWidth, opts.TruncLimit, true)
		if err == nil {
			opts.Context.Aligned = true
		}
	}
}

// printStatHeader periodically prints names of stats columns
func printStatHeader(printedNum int8, cols []string, opts ReportOptions) int8 {
	if printedNum >= repeatHeaderAfter {
		fmt.Printf("         ")
		for i, name := range cols {
			fmt.Printf("\033[%d;%dm%-*s\033[0m", 37, 1, opts.Context.ColsWidth[i]+2, name)
		}
		fmt.Printf("\n")
		return 0
	}
	return printedNum
}

// printReport prints given stats
func printStatReport(d *stat.PGresult, opts ReportOptions, ts time.Time) (printedNum int8) {
	// print stats values
	var printFirst = true // every first line in the snapshot should begin with timestamp when stats were taken
	var linesPrinted int  // count lines printed per snapshot (for limiting purposes)

	// loop through the rows, check for filtered values and print if values are satisfied
	for colnum, rownum := 0, 0; rownum < d.Nrows; rownum, colnum = rownum+1, 0 {
		var doPrint = true // assume the filtering is disabled by default and row should be printed

		// if filtering (grep) is enabled, a target column should be found and check values
		// if value doesn't match, skip it and proceed to next row
		if opts.FilterColName != "" {
			// if filter enabled, use pessimistic approach and considering the value will not match
			doPrint = false
			for idx, colname := range d.Cols {
				if colname == opts.FilterColName {
					if opts.Regexp.MatchString(d.Result[rownum][idx].String) {
						doPrint = true // value matched, so print the whole row
						break
					}
				}
			}
		}

		// print the row
		if doPrint {
			if printFirst {
				fmt.Printf("%s ", ts.Format("15:04:05"))
				printFirst = false
			} else {
				fmt.Printf("         ")
			}

			for i := range d.Cols {
				// truncate values that longer than column width
				valuelen := len(d.Result[rownum][colnum].String)
				if valuelen > opts.Context.ColsWidth[i] {
					width := opts.Context.ColsWidth[i]
					// truncate value up to column width and replace last character with '~' symbol
					d.Result[rownum][colnum].String = d.Result[rownum][colnum].String[:width-1] + "~"
				}

				// last col with no truncation of not specified otherwise
				if i != len(d.Cols)-1 {
					fmt.Printf("%-*s", opts.Context.ColsWidth[i]+2, d.Result[rownum][colnum].String)
				} else {
					fmt.Printf("%s", d.Result[rownum][colnum].String)
				}

				colnum++
			}

			fmt.Printf("\n")
			printedNum++

			// check number of printed lines, if limit is reached skip remaining rows and proceed to a next stats file
			if linesPrinted++; opts.RowLimit > 0 && linesPrinted >= opts.RowLimit {
				break
			}
		} // end if
	} // end for

	return printedNum
}

// Perform sort of statistics based on column requested by user
func doSort(stat *stat.PGresult, opts *ReportOptions) {
	var sortKey int

	// set ascending order if required
	if opts.OrderColName[0] == ascFlag[0] {
		opts.OrderDesc = false // set to Asc
		opts.OrderColName = strings.TrimLeft(opts.OrderColName, ascFlag)
	}

	for k, v := range stat.Cols {
		if v == opts.OrderColName {
			sortKey = k
			break
		}
	}

	// use descending order by default
	stat.Sort(sortKey, opts.OrderDesc)
}
