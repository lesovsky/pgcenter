//  Entry point for 'pgcenter config' command

package config

import (
	config "github.com/lesovsky/pgcenter/config"
	utils "github.com/lesovsky/pgcenter/lib/utils"
	"github.com/spf13/cobra"
)

var (
	conn utils.Conninfo
	cfg  config.Config
)

var CommandDefinition = &cobra.Command{
	Use:     "config",
	Short:   "configures Postgres to work with pgcenter",
	Long:    `'pgcenter config' configures Postgres to work with pgcenter`,
	Version: "dummy", // use constants from 'cmd' package
	Run: func(command *cobra.Command, args []string) {
		config.RunMain(args, conn, cfg)
	},
}

func init() {
	CommandDefinition.Flags().StringVarP(&conn.Host, "host", "h", "", "database server host or socket directory")
	CommandDefinition.Flags().IntVarP(&conn.Port, "port", "p", 5432, "database server port")
	CommandDefinition.Flags().StringVarP(&conn.User, "username", "U", "", "database user name")
	CommandDefinition.Flags().StringVarP(&conn.Dbname, "dbname", "d", "", "database name to connect to")
	CommandDefinition.Flags().BoolVarP(&cfg.Install, "install", "i", false, "install stats schema into the database")
	CommandDefinition.Flags().BoolVarP(&cfg.Uninstall, "uninstall", "u", false, "uninstall stats schema from the database")
}
