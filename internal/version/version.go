package version

const (
	// programName is the name of this program.
	programName = "pgcenter"
)

var (
	// Git variables imported at build stage.
	gitTag, gitCommit, gitBranch string
)

// Version returns the name and version information of this program.
func Version() (string, string, string, string) {
	return programName, gitTag, gitCommit, gitBranch
}
