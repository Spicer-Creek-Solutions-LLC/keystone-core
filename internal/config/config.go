// Package config fixes where a Generation 2 binary reads its configuration and
// how it behaves when that configuration is absent.
//
// It parses nothing yet. P11a lands the convention so that C-stage tasks inherit
// one rather than each inventing a path; the shape of a configuration file is
// theirs.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Role is the binary asking. Each has its own file: the server's configuration
// names broker credentials the agent must never read, and one shared file would
// make that separation a matter of discipline rather than of filesystem mode.
type Role string

const (
	RoleOperator Role = "keystone"
	RoleServer   Role = "keystone-server"
	RoleAgent    Role = "keystone-agent"
)

// ErrNotConfigured reports that no configuration file was found.
//
// It is an error and never a default. ADR-0009 § 1 requires the operator admin
// group to be configured rather than defaulted, and RFC 0003 § "The default
// user" requires the same of the execution user, both for the same reason: a
// product that invents a value when none is stated makes a security decision on
// the deployment's behalf and does not say so.
var ErrNotConfigured = errors.New("no configuration file found; keystone does not default one")

// ErrRelativePath reports a configuration path that is not absolute.
//
// A relative path resolves against the process's working directory, so the same
// service would read different credentials depending on where it was started —
// and an attacker who can influence a unit's working directory would choose
// which file it reads. That is the property the package comment claims, and
// review of #331 found it claimed rather than enforced: an override was returned
// verbatim.
//
// A relative path is an error and never silently ignored. Falling through to the
// next candidate would read a file the operator did not name, which is worse
// than refusing.
var ErrRelativePath = errors.New("configuration paths must be absolute")

// absolute rejects a relative path, naming the variable that supplied it so the
// operator can fix the right one.
func absolute(varName, v string) error {
	if !filepath.IsAbs(v) {
		return fmt.Errorf("%w: %s=%q", ErrRelativePath, varName, v)
	}
	return nil
}

// EnvOverride is the variable that displaces the search path entirely. One
// variable per role, so setting the agent's cannot redirect the server's.
func EnvOverride(r Role) string {
	switch r {
	case RoleOperator:
		return "KEYSTONE_CONFIG"
	case RoleServer:
		return "KEYSTONE_SERVER_CONFIG"
	case RoleAgent:
		return "KEYSTONE_AGENT_CONFIG"
	}
	return ""
}

// SearchPath returns the ordered locations a role reads, first match winning.
//
// The system path is last, not first: a deployment's file beats a package
// default, and an operator's own file beats both. Nothing here reads $PWD —
// a configuration picked up from the working directory means the same command
// behaves differently depending on where it was run from.
func SearchPath(r Role, env func(string) string) ([]string, error) {
	if env == nil {
		env = os.Getenv
	}
	if v := env(EnvOverride(r)); v != "" {
		if err := absolute(EnvOverride(r), v); err != nil {
			return nil, err
		}
		return []string{v}, nil
	}
	var paths []string
	if r == RoleOperator {
		// Only the operator CLI has a per-user configuration. A service reading
		// from a home directory reads from whatever account it happens to run
		// as, which is not a deployment decision.
		//
		// Both variables are checked for absoluteness: a relative HOME would
		// produce a relative candidate just as an override would.
		if h := env("XDG_CONFIG_HOME"); h != "" {
			if err := absolute("XDG_CONFIG_HOME", h); err != nil {
				return nil, err
			}
			paths = append(paths, filepath.Join(h, "keystone", "config.toml"))
		} else if h := env("HOME"); h != "" {
			if err := absolute("HOME", h); err != nil {
				return nil, err
			}
			paths = append(paths, filepath.Join(h, ".config", "keystone", "config.toml"))
		}
	}
	return append(paths, filepath.Join("/etc", "keystone", string(r)+".toml")), nil
}

// Locate returns the first path in the search order that exists.
func Locate(r Role, env func(string) string) (string, error) {
	candidates, err := SearchPath(r, env)
	if err != nil {
		return "", err
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", ErrNotConfigured
}
