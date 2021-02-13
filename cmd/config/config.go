// Entry point for 'pgcenter config' command.

package config

import (
	"fmt"
	"github.com/lesovsky/pgcenter/config"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/spf13/cobra"
)

var (
	localOptions options
	connOptions  postgres.ConnectionOptions

	// CommandDefinition defines 'config' sub-command.
	CommandDefinition = &cobra.Command{
		Use:   "config",
		Short: "installs or uninstalls pgcenter stats schema to Postgres",
		Long:  `'pgcenter config' installs or uninstalls pgcenter stats schema to Postgres.`,
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

			// Validate local options.
			err = localOptions.validate()
			if err != nil {
				return err
			}

			// Select runtime mode.
			mode := localOptions.mode()

			return config.RunMain(pgConfig, mode)
		},
	}
)

func init() {
	CommandDefinition.Flags().StringVarP(&connOptions.Host, "host", "h", "", "database server host or socket directory")
	CommandDefinition.Flags().IntVarP(&connOptions.Port, "port", "p", 5432, "database server port")
	CommandDefinition.Flags().StringVarP(&connOptions.User, "username", "U", "", "database user name")
	CommandDefinition.Flags().StringVarP(&connOptions.Dbname, "dbname", "d", "", "database name to connect to")
	CommandDefinition.Flags().BoolVarP(&localOptions.install, "install", "i", false, "install stats schema into the database")
	CommandDefinition.Flags().BoolVarP(&localOptions.uninstall, "uninstall", "u", false, "uninstall stats schema from the database")
}

// options defines set of options used only in 'pgcenter config' scope
type options struct {
	install   bool
	uninstall bool
}

// validate performs sanity checks of passed options
func (opts *options) validate() error {
	if !opts.install && !opts.uninstall {
		return fmt.Errorf("using '--install' or '--uninstall' options are mandatory")
	}

	if opts.install == opts.uninstall {
		return fmt.Errorf("can't use '--install' and '--uninstall' options together")
	}

	return nil
}

// mode return runtime mode
func (opts *options) mode() int {
	if opts.install {
		return config.Install
	}
	if opts.uninstall {
		return config.Uninstall
	}
	return -1
}
