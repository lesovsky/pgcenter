// Entry point for 'pgcenter top' command

package top

import (
	//"github.com/lesovsky/pgcenter/cmd"                /* code related with 'root command' handling */
	"github.com/lesovsky/pgcenter/lib/utils"
	"github.com/lesovsky/pgcenter/top" /* code related to 'pgcenter top' functionality */
	"github.com/spf13/cobra"           /* cli */
)

var (
	conn utils.Conninfo
)

// CommandDefinition is the definition of 'top' CLI sub-command
var CommandDefinition = &cobra.Command{
	Use:     "top",
	Short:   "top-like stats viewer",
	Long:    `'pgcenter top' is the top-like stats viewer.`,
	Version: "dummy", // use constants from 'cmd' package
	Run: func(command *cobra.Command, args []string) {
		top.RunMain(args, conn)
	},
}

func init() {
	CommandDefinition.Flags().StringVarP(&conn.Host, "host", "h", "", "database server host or socket directory")
	CommandDefinition.Flags().IntVarP(&conn.Port, "port", "p", 5432, "database server port")
	CommandDefinition.Flags().StringVarP(&conn.User, "username", "U", "", "database user name")
	CommandDefinition.Flags().StringVarP(&conn.Dbname, "dbname", "d", "", "database name to connect to")
	CommandDefinition.Flags().StringVarP(&conn.Logpath, "logpath", "l", "", "database server logpath (logfile optional)")
}
