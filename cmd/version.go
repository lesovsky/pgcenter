// Stuff related to program versions, releases, etc.

package cmd

import (
	"fmt"
)

const (
	// ProgramName is the name of this program
	ProgramName = "pgcenter"
	// ProgramIssuesUrl is the public URL for posting issues, bug reports and asking questions
	ProgramIssuesUrl = "https://github.com/lesovsky/pgcenter/issues"
)

var (
	GitTag, GitCommit, GitBranch string
)

// PrintVersion prints the name and version of this program
func PrintVersion() string {
	return fmt.Sprintf("%s %s %s-%s\n", ProgramName, GitTag, GitCommit, GitBranch)
}
