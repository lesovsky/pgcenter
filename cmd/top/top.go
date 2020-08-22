// Entry point for 'pgcenter top' command

package top

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/top"
	"github.com/spf13/cobra"
)

type options struct {
	host   string
	port   int
	user   string
	dbname string
}

var (
	opts options
)

// CommandDefinition is the definition of 'top' CLI sub-command
var CommandDefinition = &cobra.Command{
	Use:     "top",
	Short:   "top-like stats viewer",
	Long:    `'pgcenter top' is the top-like stats viewer.`,
	Version: "dummy", // use constants from 'cmd' package
	RunE: func(command *cobra.Command, args []string) error {
		if len(args) > 0 {
			opts.handleExtraArgs(args)
		}

		pgConfig, err := postgres.NewConfig(opts.host, opts.port, opts.user, opts.dbname)
		if err != nil {
			return err
		}

		return top.RunMain(pgConfig)
	},
}

// Parse user passed parameters values and arguments.
func init() {
	CommandDefinition.Flags().StringVarP(&opts.host, "host", "h", "", "database server host or socket directory")
	CommandDefinition.Flags().IntVarP(&opts.port, "port", "p", 5432, "database server port")
	CommandDefinition.Flags().StringVarP(&opts.user, "username", "U", "", "database user name")
	CommandDefinition.Flags().StringVarP(&opts.dbname, "dbname", "d", "", "database name to connect to")
}

// handleExtraArgs parses extra arguments and uses them as a part of connection options.
func (opts *options) handleExtraArgs(args []string) {
	for i := 0; i < len(args); i++ {
		if opts.dbname == "" {
			opts.dbname = args[i]
		} else {
			fmt.Printf("warning: extra command-line argument %s ignored\n", args[i])
		}

		if i++; i >= len(args) {
			break
		}

		if opts.user == "" {
			opts.user = args[i]
		} else {
			fmt.Printf("warning: extra command-line argument %s ignored\n", args[i])
		}
	}
}
