// SPDX-License-Identifier: Apache-2.0

// Package version exposes build-time version metadata populated via -ldflags -X.
package version

// Build metadata. Overridden at link time via -ldflags -X.
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

// Info captures a snapshot of the build metadata.
type Info struct {
	Version   string
	GitCommit string
	BuildDate string
}

// Get returns the current build metadata.
func Get() Info {
	return Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
	}
}
