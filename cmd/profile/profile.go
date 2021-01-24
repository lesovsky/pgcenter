// Entry point for 'pgcenter profile' command

package profile

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/profile"
	"github.com/spf13/cobra"
	"time"
)

var (
	profileConfig profile.Config
	connOptions   postgres.ConnectionOptions

	// CommandDefinition is the definition of 'profile' CLI sub-command
	CommandDefinition = &cobra.Command{
		Use:   "profile",
		Short: "wait events profiler",
		Long:  `'pgcenter profile' profiles wait events of running queries.`,
		RunE: func(command *cobra.Command, args []string) error {
			// Parse extra arguments.
			if len(args) > 0 {
				connOptions.ParseExtraArgs(args)
			}

			// Create connection config.
			pgConfig, err := postgres.NewConfig(connOptions.Host, connOptions.Port, connOptions.User, connOptions.Dbname)
			if err != nil {
				return err
			}

			err = validate(profileConfig)
			if err != nil {
				return err
			}

			return profile.RunMain(pgConfig, profileConfig)
		},
	}
)

func init() {
	CommandDefinition.Flags().StringVarP(&connOptions.Host, "host", "h", "", "database server host or socket directory")
	CommandDefinition.Flags().IntVarP(&connOptions.Port, "port", "p", 5432, "database server port")
	CommandDefinition.Flags().StringVarP(&connOptions.User, "username", "U", "", "database user name")
	CommandDefinition.Flags().StringVarP(&connOptions.Dbname, "dbname", "d", "", "database name to connect to")
	CommandDefinition.Flags().IntVarP(&profileConfig.Pid, "pid", "P", 0, "PID of Postgres backend to profile to")
	CommandDefinition.Flags().DurationVarP(&profileConfig.Frequency, "freq", "F", 100*time.Millisecond, "profile with this frequency (default: 100ms)")
	CommandDefinition.Flags().IntVarP(&profileConfig.Strsize, "strsize", "s", 128, "limit length of print query strings to STRSIZE chars (default 128)")

	_ = CommandDefinition.MarkFlagRequired("pid")
}

func validate(config profile.Config) error {
	if config.Frequency < time.Millisecond || config.Frequency > time.Second {
		return fmt.Errorf("invalid profile frequency, must be between 1 millisecond and 1 second")
	}
	return nil
}
