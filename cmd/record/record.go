// Entry point for 'pgcenter record' command

package record

import (
	"github.com/lesovsky/pgcenter/lib/utils"
	"github.com/lesovsky/pgcenter/record"
	"github.com/spf13/cobra"
	"time"
)

const (
	defaultRecordFile = "pgcenter.stat.tar"
)

var (
	conn    utils.Conninfo
	opts    record.RecordOptions
	oneshot bool

	CommandDefinition = &cobra.Command{
		Use:     "record",
		Short:   "record stats to file",
		Long:    `'pgcenter record' connects to PostgreSQL and collects stats into local file.`,
		Version: "dummy", // use constants from 'cmd' package
		PreRun:  preFlightSetup,
		Run: func(command *cobra.Command, args []string) {
			record.RunMain(args, conn, opts)
		},
	}
)

func init() {
	CommandDefinition.Flags().StringVarP(&conn.Host, "host", "h", "", "database server host or socket directory")
	CommandDefinition.Flags().IntVarP(&conn.Port, "port", "p", 5432, "database server port")
	CommandDefinition.Flags().StringVarP(&conn.User, "username", "U", "", "database user name")
	CommandDefinition.Flags().StringVarP(&conn.Dbname, "dbname", "d", "", "database name to connect to")
	CommandDefinition.Flags().DurationVarP(&opts.Interval, "interval", "i", 1*time.Second, "polling interval (default: 1 second)")
	CommandDefinition.Flags().Int32VarP(&opts.Count, "count", "c", -1, "number of stats samples to collect")
	CommandDefinition.Flags().StringVarP(&opts.OutputFile, "file", "f", defaultRecordFile, "file where stats are saved")
	CommandDefinition.Flags().BoolVarP(&opts.AppendFile, "append", "a", false, "append statistics to a file, instead of creating a new one")
	CommandDefinition.Flags().BoolVarP(&oneshot, "oneshot", "1", false, "append single statistics snapshot to file and exit")
}

// analyze startup parameters and prepare options for record program
func preFlightSetup(_ *cobra.Command, _ []string) {
	// dereference aliases
	dereferenceOneshot()
}

// oneshot is a shortcut for "--append --count 1 --interval 0"
func dereferenceOneshot() {
	if oneshot {
		opts.AppendFile = true
		opts.Count = 1
		opts.Interval = 0
	}
}
