// Stuff related to program versions, releases, etc.

package cmd

import (
	"fmt"
)

const (
	ProgramName      = "pgcenter"
	ProgramVersion   = "0.5"
	ProgramRelease   = "0"
	ProgramIssuesUrl = "https://github.com/lesovsky/pgcenter/issues"
)

func PrintVersion() string {
	return fmt.Sprintf("%s %s.%s\n", ProgramName, ProgramVersion, ProgramRelease)
}
