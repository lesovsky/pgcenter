// Stuff related to program versions, releases, etc.

package cmd

import (
	"fmt"
)

const (
	// ProgramName is the name of this program
	ProgramName = "pgcenter"
	// ProgramVersion is the version of this program
	ProgramVersion = "0.6"
	// ProgramRelease is release number of this program
	ProgramRelease = "2"
	// ProgramIssuesUrl is the public URL for posting issues, bug reports and asking questions
	ProgramIssuesUrl = "https://github.com/lesovsky/pgcenter/issues"
)

// PrintVersion prints the name and version of this program
func PrintVersion() string {
	return fmt.Sprintf("%s %s.%s\n", ProgramName, ProgramVersion, ProgramRelease)
}
