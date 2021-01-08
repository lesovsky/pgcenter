// Entry point for 'pgcenter top' command

package top

import (
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/top"
	"github.com/spf13/cobra"
)

var (
	opts postgres.ConnectionOptions

	// CommandDefinition defines 'top' sub-command.
	CommandDefinition = &cobra.Command{
		Use:   "top",
		Short: "top-like stats viewer.",
		Long:  `'pgcenter top' is the top-like stats viewer.`,
		RunE: func(command *cobra.Command, args []string) error {
			// Parse extra arguments.
			if len(args) > 0 {
				opts.ParseExtraArgs(args)
			}

			// Create connection config.
			pgConfig, err := postgres.NewConfig(opts.Host, opts.Port, opts.User, opts.Dbname)
			if err != nil {
				return err
			}

			return top.RunMain(pgConfig)
		},
	}
)

// Parse user passed parameters values and arguments.
func init() {
	CommandDefinition.Flags().StringVarP(&opts.Host, "host", "h", "", "database server host or socket directory")
	CommandDefinition.Flags().IntVarP(&opts.Port, "port", "p", 5432, "database server port")
	CommandDefinition.Flags().StringVarP(&opts.User, "username", "U", "", "database user name")
	CommandDefinition.Flags().StringVarP(&opts.Dbname, "dbname", "d", "", "database name to connect to")
}
