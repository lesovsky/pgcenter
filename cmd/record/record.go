// Entry point for 'pgcenter record' command.

package record

import (
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/record"
	"github.com/spf13/cobra"
	"time"
)

var (
	recordConfig record.Config
	connOptions  postgres.ConnectionOptions
	oneshot      bool

	// CommandDefinition defines 'record' sub-command.
	CommandDefinition = &cobra.Command{
		Use:   "record",
		Short: "record stats to file",
		Long:  `'pgcenter record' connects to PostgreSQL and collects stats into local file.`,
		RunE: func(command *cobra.Command, args []string) error {
			// Convert 'oneshot' to set of options.
			if oneshot {
				recordConfig.AppendFile = true
				recordConfig.Count = 1
				recordConfig.Interval = time.Millisecond // interval must not be zero - ticker will panic.
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

			return record.RunMain(pgConfig, recordConfig)
		},
	}
)

func init() {
	defaultRecordFile := "pgcenter.stat.tar"

	CommandDefinition.Flags().StringVarP(&connOptions.Host, "host", "h", "", "database server host or socket directory")
	CommandDefinition.Flags().IntVarP(&connOptions.Port, "port", "p", 5432, "database server port")
	CommandDefinition.Flags().StringVarP(&connOptions.User, "username", "U", "", "database user name")
	CommandDefinition.Flags().StringVarP(&connOptions.Dbname, "dbname", "d", "", "database name to connect to")
	CommandDefinition.Flags().DurationVarP(&recordConfig.Interval, "interval", "i", time.Second, "statistics recording interval (default: 1 second)")
	CommandDefinition.Flags().IntVarP(&recordConfig.Count, "count", "c", -1, "number of statistics samples to record")
	CommandDefinition.Flags().StringVarP(&recordConfig.OutputFile, "file", "f", defaultRecordFile, "file where statistics are saved")
	CommandDefinition.Flags().BoolVarP(&recordConfig.AppendFile, "append", "a", false, "append statistics to file (default: true)")
	CommandDefinition.Flags().IntVarP(&recordConfig.StringLimit, "strlimit", "t", 0, "maximum query length to record (default: 0, no limit)")
	CommandDefinition.Flags().BoolVarP(&oneshot, "oneshot", "1", false, "append single statistics snapshot to file and exit")
}
