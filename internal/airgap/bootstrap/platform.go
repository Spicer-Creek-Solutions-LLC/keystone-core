// Package bootstrap provides tools for creating, verifying, and installing
// self-contained bootstrap packages for air-gapped Keystone Core deployments.
package bootstrap

import (
	"fmt"
	"strings"
)

// Platform represents a target OS/architecture combination.
type Platform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

// String returns the platform in "os/arch" format.
func (p Platform) String() string {
	return p.OS + "/" + p.Arch
}

// Validate checks that the platform is a supported combination.
func (p Platform) Validate() error {
	if p.OS == "" {
		return fmt.Errorf("platform OS is required")
	}
	if p.Arch == "" {
		return fmt.Errorf("platform architecture is required")
	}
	for _, sp := range SupportedPlatforms() {
		if sp.OS == p.OS && sp.Arch == p.Arch {
			return nil
		}
	}
	return fmt.Errorf("unsupported platform: %s", p)
}

// ParsePlatform parses a "os/arch" string into a Platform.
func ParsePlatform(s string) (Platform, error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Platform{}, fmt.Errorf("invalid platform format %q: expected os/arch", s)
	}
	p := Platform{OS: parts[0], Arch: parts[1]}
	if err := p.Validate(); err != nil {
		return Platform{}, err
	}
	return p, nil
}

// SupportedPlatforms returns the list of platforms supported for bootstrap packages.
func SupportedPlatforms() []Platform {
	return []Platform{
		{OS: "linux", Arch: "amd64"},
		{OS: "linux", Arch: "arm64"},
		{OS: "windows", Arch: "amd64"},
	}
}
