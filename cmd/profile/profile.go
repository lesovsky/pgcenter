// Entry point for 'pgcenter profile' command

package profile

import (
	"github.com/lesovsky/pgcenter/lib/utils"
	"github.com/lesovsky/pgcenter/profile"
	"github.com/spf13/cobra"
	"time"
)

var (
	conn utils.Conninfo
	opts profile.TraceOptions
	frequency int
)

var CommandDefinition = &cobra.Command{
	Use:     "profile",
	Short:   "wait events profiler",
	Long:    `'pgcenter profile' profiles wait events of running queries`,
	Version: "dummy", // use constants from 'cmd' package
	PreRun:  preFlightSetup,
	Run: func(command *cobra.Command, args []string) {
		profile.RunMain(args, conn, opts)
	},
}

func init() {
	CommandDefinition.Flags().StringVarP(&conn.Host, "host", "h", "", "database server host or socket directory")
	CommandDefinition.Flags().IntVarP(&conn.Port, "port", "p", 5432, "database server port")
	CommandDefinition.Flags().StringVarP(&conn.User, "username", "U", "", "database user name")
	CommandDefinition.Flags().StringVarP(&conn.Dbname, "dbname", "d", "", "database name to connect to")
	CommandDefinition.Flags().IntVarP(&opts.Pid, "pid", "P", -1, "PID of Postgres backend to profile to")
	CommandDefinition.Flags().IntVarP(&frequency, "freq", "F", 100, "profile with this frequency")
	CommandDefinition.Flags().IntVarP(&opts.Strsize, "strsize", "s", 128, "limit length of print query strings to STRSIZE chars (default 128)")
}

func preFlightSetup(_ *cobra.Command, _ []string) {
	// setup profiling interval
	switch {
	case frequency < 1:
		opts.Interval = 1 * time.Millisecond
	case frequency > 1000:
		opts.Interval = 1 * time.Second
	default:
		opts.Interval = time.Millisecond * (time.Second / (time.Duration(frequency) * time.Millisecond))
	}
}