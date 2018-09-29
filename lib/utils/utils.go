// Package 'utils' provides functions that used in pgcenter's sub-commands.

package utils

import (
	"fmt"
)

const (
	DefaultPager  = "less"
	DefaultEditor = "vi"
	DefaultPsql   = "psql"
)

// Container for connection settings to Postgres
type Conninfo struct {
	Host      string
	Port      int
	User      string
	Dbname    string
	ConnLocal bool // is Postgres running on localhost?
}

// Read and parse extra arguments
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
