// Command 'pgcenter config' is used for managing pgCenter's stats schema. This schema should be installed
// into target database and used for gathering '/proc' stats from hosts where the database runs.
// Main aim of this schema is providing stats in case when pgCenter and Postgres are running on separate hosts.

package config

import (
	"database/sql"
	"fmt"
	"github.com/lesovsky/pgcenter/lib/utils"
	"github.com/lib/pq"
)

type Config struct {
	Install   bool
	Uninstall bool
}

const (
	doInstall = iota
	doUninstall
)

var (
	conninfo utils.Conninfo
	db       *sql.DB
	err      error
)

// Main function for 'pgcenter config' command.
func RunMain(args []string, c utils.Conninfo, cfg Config) {
	conninfo = c // copy conninfo from external struct into local one
	utils.HandleExtraArgs(args, &conninfo)

	db, err = utils.CreateConn(&conninfo)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err.Error())
		return
	}

	defer db.Close()

	switch {
	case cfg.Install == true:
		// install stats schema
		if err := manageSchema(doInstall); err != nil {
			fmt.Println(err)
		}
	case cfg.Uninstall == true:
		// uninstall stats schema
		if err := manageSchema(doUninstall); err != nil {
			fmt.Println(err)
		}
	}

	return
}

// Used for installing or removing stats schema into/from Postgres database
func manageSchema(action int) error {
	var sqlSet []string
	var msg string

	// Select an appropriate set of SQL commands depending on specified action
	switch action {
	case doInstall:
		sqlSet = createSchemaSqlSet
		msg = "Statistics schema installed."
	case doUninstall:
		sqlSet = dropSchemaSqlSet
		msg = "Statistics schema removed."
	}

	// Start a transaction and execute SQL commands. Rollback it if something goes wrong.
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("ERROR: failed to begin transaction: %s", err)
	}

	for _, query := range sqlSet {
		_, err := tx.Exec(query)
		if err, ok := err.(*pq.Error); ok {
			tx.Rollback()
			return fmt.Errorf("%s: %s\nDETAIL: %s\nHINT: %s\nSTATEMENT: %s", err.Severity, err.Message, err.Detail, err.Hint, query)
		}
	}

	// Commit the transaction if everything is OK.
	tx.Commit()
	fmt.Println(msg)

	return nil
}
