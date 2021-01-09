// Entry point for 'pgcenter record' command

package record

import (
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/record"
	"github.com/spf13/cobra"
	"time"
)

var (
	recordOptions record.Config
	connOptions   postgres.ConnectionOptions
	oneshot       bool

	// CommandDefinition defines 'record' sub-command.
	CommandDefinition = &cobra.Command{
		Use:   "record",
		Short: "record stats to file.",
		Long:  `'pgcenter record' connects to PostgreSQL and collects stats into local file.`,
		RunE: func(command *cobra.Command, args []string) error {
			// Convert 'oneshot' to set of options.
			if oneshot {
				recordOptions.AppendFile = true
				recordOptions.Count = 1
				recordOptions.Interval = 0
			}

			// Parse extra arguments.
			if len(args) > 0 {
				connOptions.ParseExtraArgs(args)
			}

			// Create connection config.
			pgConfig, err := postgres.NewConfig(connOptions.Host, connOptions.Port, connOptions.User, connOptions.Dbname)
			if err != nil {
				return err
			}

			return record.RunMain(pgConfig, recordOptions)
		},
	}
)

func init() {
	defaultRecordFile := "pgcenter.stat.tar"

	CommandDefinition.Flags().StringVarP(&connOptions.Host, "host", "h", "", "database server host or socket directory")
	CommandDefinition.Flags().IntVarP(&connOptions.Port, "port", "p", 5432, "database server port")
	CommandDefinition.Flags().StringVarP(&connOptions.User, "username", "U", "", "database user name")
	CommandDefinition.Flags().StringVarP(&connOptions.Dbname, "dbname", "d", "", "database name to connect to")
	CommandDefinition.Flags().DurationVarP(&recordOptions.Interval, "interval", "i", 1*time.Second, "polling interval (default: 1 second)")
	CommandDefinition.Flags().Int32VarP(&recordOptions.Count, "count", "c", -1, "number of stats samples to collect")
	CommandDefinition.Flags().StringVarP(&recordOptions.OutputFile, "file", "f", defaultRecordFile, "file where stats are saved")
	CommandDefinition.Flags().BoolVarP(&recordOptions.AppendFile, "append", "a", false, "append statistics to a file, instead of creating a new one")
	CommandDefinition.Flags().IntVarP(&recordOptions.TruncLimit, "truncate", "t", 0, "maximum query length to record (default: 0, no limit)")
	CommandDefinition.Flags().BoolVarP(&oneshot, "oneshot", "1", false, "append single statistics snapshot to file and exit")
}
