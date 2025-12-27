package version

import "fmt"

var (
	// Version is the semantic version of Keystone Core (set by build flags)
	Version = "dev"
	// GitCommit is the git commit hash (set by build flags)
	GitCommit = "unknown"
	// BuildDate is the build date (set by build flags)
	BuildDate = "unknown"
)

// Info represents version information
type Info struct {
	Version   string
	GitCommit string
	BuildDate string
}

// Get returns the version information
func Get() Info {
	return Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
	}
}

// String returns a formatted version string
func (i Info) String() string {
	return fmt.Sprintf("Keystone Core %s (commit: %s, built: %s)", i.Version, i.GitCommit, i.BuildDate)
}
