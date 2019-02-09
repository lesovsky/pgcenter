// Package 'utils' provides functions that used in pgcenter's sub-commands.

package utils

import (
	"fmt"
)

const (
	// DefaultPager is the pager program used by default if nothing other specified
	DefaultPager = "less"
	// DefaultEditor is the editor program used by default if nothing other specified
	DefaultEditor = "vi"
	// DefaultPsql is the default name of psql client
	DefaultPsql = "psql"
)

// Conninfo stores connection settings to Postgres
type Conninfo struct {
	Host      string
	Port      int
	User      string
	Dbname    string
	ConnLocal bool // is Postgres running on localhost?
}

// HandleExtraArgs reads and parses extra arguments passed to program
func HandleExtraArgs(args []string, conn *Conninfo) {
	if len(args) > 0 {
		for i := 0; i < len(args); i++ {
			if conn.Dbname == "" {
				conn.Dbname = args[i]
			} else {
				fmt.Printf("warning: extra command-line argument %s ignored\n", args[i])
			}

			if i++; i >= len(args) {
				break
			}

			if conn.User == "" {
				conn.User = args[i]
			} else {
				fmt.Printf("warning: extra command-line argument %s ignored\n", args[i])
			}
		}
	}
}
