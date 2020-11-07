// Code related to 'pgcenter record' command

package record

//import (
//	"archive/tar"
//	"database/sql"
//	"encoding/json"
//	"fmt"
//	"github.com/lesovsky/pgcenter/lib/stat"
//	"github.com/lesovsky/pgcenter/lib/utils"
//	"io"
//	"log"
//	"os"
//	"os/signal"
//	"time"
//)
//
//// RecordOptions is the container for recorder settings
//type RecordOptions struct {
//	Interval      time.Duration    // Statistics pollint interval
//	Count         int32            // Number of statistics snapshot to record
//	OutputFile    string           // File where statistics will be saved
//	AppendFile    bool             // Append to a file, or create/truncate at the beginning
//	TruncLimit    int              // Limit of the length, to which query should be truncated
//	contextList   stat.ContextList // List of statistics available for recording
//	sharedOptions stat.Options     // Queries' settings that depend on Postgres version
//}
//
//var (
//	conn   *sql.DB
//	stats  stat.Stat
//	doQuit = make(chan os.Signal, 1)
//)
//
//// RunMain is the program's main entry point
//func RunMain(args []string, conninfo utils.Conninfo, opts RecordOptions) {
//	var err error
//	fmt.Printf("INFO: recording to %s\n", opts.OutputFile)
//
//	// Handle extra arguments passed
//	utils.HandleExtraArgs(args, &conninfo)
//
//	// Setup connection to Postgres
//	conn, err = utils.CreateConn(&conninfo)
//	if err != nil {
//		fmt.Printf("ERROR: %s\n", err.Error())
//		return
//	}
//	defer conn.Close()
//
//	// Open file for statistics
//	var f *os.File
//	if opts.AppendFile {
//		if f, err = os.OpenFile(opts.OutputFile, os.O_CREATE|os.O_RDWR, 0644); err != nil {
//			log.Fatalf("ERROR: failed to open file: %s\n", err)
//		} else {
//			_, _ = f.Seek(-2<<9, io.SeekEnd) // ignore errors, if seek failed it's highly likely an empty file
//		}
//	} else {
//		if f, err = os.OpenFile(opts.OutputFile, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644); err != nil {
//			log.Fatalf("ERROR: failed to create file: %s\n", err)
//		}
//	}
//	defer f.Close()
//
//	// Initialize tar writer
//	tw := tar.NewWriter(f)
//	defer tw.Close()
//
//	// In case of SIGINT stop program gracefully
//	signal.Notify(doQuit, os.Interrupt)
//
//	// Get necessary information about Postgres: version, recovery status, settings, etc.
//	stats.ReadPgInfo(conn, conninfo.ConnLocal)
//
//	// Setup recordOptions - adjust queries for used Postgres version
//	opts.Setup(stats.PgInfo)
//
//	// Recording loop
//	recordLoop(tw, opts)
//}
//
//// Record stats with an interval
//func recordLoop(w *tar.Writer, opts RecordOptions) {
//	// record the number of snapshots requested by user (or record continuously until SIGINT will be received)
//	for i := opts.Count; i != 0; i-- {
//		if err := doWork(w, opts); err != nil {
//			fmt.Printf("ERROR: %s\n", err)
//		}
//
//		select {
//		case <-time.After(opts.Interval):
//			break
//		case <-doQuit:
//			fmt.Println("quit")
//			i = 1 // 'i' decrements to zero after iteration and loop will be finished
//		}
//	}
//}
//
//// Read stats from Postgres and write it to a file
//func doWork(w *tar.Writer, opts RecordOptions) error {
//	now := time.Now()
//
//	// loop over available contexts
//	for ctxUnit := range opts.contextList {
//		// prepare query and select stats from Postgres
//		query, _ := stat.PrepareQuery(opts.contextList[ctxUnit].Query, opts.sharedOptions)
//
//		if err := stats.GetPgstatSample(conn, query); err != nil {
//			return err
//		}
//
//		// write stats to a file
//		name := fmt.Sprintf("%s.%s.json", ctxUnit, now.Format("20060102T150405"))
//		if err := writeToTar(w, stats.CurrPGresult, name); err != nil {
//			return err
//		}
//	}
//	return nil
//}
//
//// Write statistics to a tar-file
//func writeToTar(w *tar.Writer, object interface{}, name string) error {
//	data, err := json.Marshal(object)
//	if err != nil {
//		return fmt.Errorf("failed to marshal: %s", err.Error())
//	}
//
//	hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(len(data)), ModTime: time.Now()}
//	if err := w.WriteHeader(hdr); err != nil {
//		return fmt.Errorf("failed to write header: %s", err)
//	}
//
//	if _, err := w.Write(data); err != nil {
//		return fmt.Errorf("failed to write body: %s", err)
//	}
//
//	return nil
//}
