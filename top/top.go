// 'pgcenter top' - top-like stats viewer.

package top

import (
	"database/sql"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/lib/utils"
)

var (
	conninfo utils.Conninfo
	conn     *sql.DB
)

// RunMain is the main entry point for 'pgcenter top' command
func RunMain(args []string, c utils.Conninfo, db *postgres.DB) error {
	var err error

	// Assign conninfo values from external struct into global one (it have to be available everywhere)
	conninfo = c

	// Handle extra arguments passed
	utils.HandleExtraArgs(args, &conninfo)

	// Connect to Postgres
	conn, err = utils.CreateConn(&conninfo)
	if err != nil {
		//fmt.Printf("ERROR: %s\n", err.Error())
		return err
	}
	defer conn.Close()

	// Get necessary information about Postgres, such as version, recovery status, settings, etc.
	stats.ReadPgInfo(conn, conninfo.ConnLocal)

	// Setup context - which kind of stats should be displayed
	ctx.Setup(stats.PgInfo)

	// Run UI
	//if err := uiLoop(); err != nil {
	//	fmt.Println(err)
	//}
	return uiLoop()
}
