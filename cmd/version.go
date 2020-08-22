// Stuff related to program versions, releases, etc.

package cmd

import (
	"fmt"
)

const (
	// programName is the name of this program
	programName = "pgcenter"

	// programIssuesUrl is the public URL for posting issues, bug reports and asking questions
	programIssuesUrl = "https://github.com/lesovsky/pgcenter/issues"
)

var (
	// Git variables imported at build stage
	gitTag, gitCommit, gitBranch string
)

// PrintVersion prints the name and version of this program
func printVersion() string {
	return fmt.Sprintf("%s %s %s-%s\n", programName, gitTag, gitCommit, gitBranch)
}
