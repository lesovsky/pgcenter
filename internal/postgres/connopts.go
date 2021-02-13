package postgres

import "fmt"

// ConnectionOptions defines connection options (used by all pgcenter subcommands).
type ConnectionOptions struct {
	Host   string
	Port   int
	User   string
	Dbname string
}

// ParseExtraArgs parses extra arguments passed in CLI and fills ConnectionOptions properties.
func (c *ConnectionOptions) ParseExtraArgs(args []string) {
	for i := 0; i < len(args); i++ {
		if c.Dbname == "" {
			c.Dbname = args[i]
		} else {
			fmt.Printf("warning: extra command-line argument %s ignored\n", args[i])
		}

		if i++; i >= len(args) {
			break
		}

		if c.User == "" {
			c.User = args[i]
		} else {
			fmt.Printf("warning: extra command-line argument %s ignored\n", args[i])
		}
	}
}
