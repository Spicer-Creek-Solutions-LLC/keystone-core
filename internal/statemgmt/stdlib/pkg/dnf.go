// SPDX-License-Identifier: Apache-2.0

//go:build linux

package pkg

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// dnfProvider implements Provider against the dnf + rpm toolchain
// (RHEL 8+ / Rocky / Fedora). dnf and rpm are absolute paths resolved
// at detect time so the provider doesn't re-LookPath on every call.
//
// The split mirrors aptProvider: dnf owns the mutating operations
// (install / remove) and rpm owns the read-only query, exactly as
// apt-get + dpkg-query split on Debian.
//
// runner is the injection point that lets dnf_test.go pin arg
// formation without invoking dnf for real. newDnfProvider wires it to
// execRun; tests inject a capturing shim.
type dnfProvider struct {
	dnf       string
	rpm       string
	runner    commandRunner
	rpmLookup rpmLookupFn
}

// rpmLookupFn is the injection point for the Lookup path. The
// production impl shells out to rpm; tests inject a fake that returns
// canned stdout/exit so parseRpmQuery exercises against fixed strings
// without touching the real rpm database.
type rpmLookupFn func(ctx context.Context, rpm, name string) (stdout string, exitCode int, err error)

func newDnfProvider(dnf, rpm string) *dnfProvider {
	return &dnfProvider{
		dnf:       dnf,
		rpm:       rpm,
		runner:    execRun,
		rpmLookup: realRpmLookup,
	}
}

func (p *dnfProvider) Lookup(name string) (*PkgInfo, error) {
	stdout, exit, err := p.rpmLookup(context.Background(), p.rpm, name)
	if err != nil && exit != 1 {
		// Exit 1 is rpm's canonical "package is not installed" — not a
		// real error. Anything else (binary missing, IO error)
		// surfaces.
		return nil, fmt.Errorf("rpm -q %s: %w", name, err)
	}
	return parseRpmQuery(name, stdout)
}

func (p *dnfProvider) Install(ctx context.Context, name, version string) error {
	// dnf's versioned spec is "name-version" (e.g. nginx-1.20.1),
	// unlike apt's "name=version". Unversioned installs the latest
	// available.
	pkgSpec := name
	if version != "" {
		pkgSpec = name + "-" + version
	}
	args := []string{"install", "-y", pkgSpec}
	return p.runner(ctx, p.dnf, args, nil)
}

func (p *dnfProvider) Remove(ctx context.Context, name string) error {
	args := []string{"remove", "-y", name}
	return p.runner(ctx, p.dnf, args, nil)
}

// parseRpmQuery reads the output of:
//
//	rpm -q --qf '%{VERSION}-%{RELEASE}\n' <name>
//
// When the package is installed, rpm prints "<version>-<release>" and
// exits 0. When it is not, rpm prints "package <name> is not installed"
// (to stdout) and exits 1 — the --qf format only applies to matched
// packages, so the not-installed message comes through verbatim.
//
// Empty stdout (the exit-1 case surfaced as "") and the not-installed
// message both map to Installed=false. A line that is neither is taken
// as the version. rpm does not have dpkg's half-installed intermediate
// states (a package is in the rpmdb or it is not), so there is no
// equivalent of parseDpkgStatus's status-char switch.
func parseRpmQuery(name, stdout string) (*PkgInfo, error) {
	out := strings.TrimSpace(stdout)
	if out == "" || strings.Contains(out, "is not installed") {
		return &PkgInfo{Name: name, Installed: false}, nil
	}
	// First line only; defensive against any trailing scriptlet noise.
	version := out
	if i := strings.IndexByte(version, '\n'); i >= 0 {
		version = strings.TrimSpace(version[:i])
	}
	if version == "" {
		return nil, fmt.Errorf("rpm -q: empty version for installed %s", name)
	}
	return &PkgInfo{Name: name, Installed: true, Version: version}, nil
}

// realRpmLookup invokes rpm -q and returns stdout + exit code (without
// surfacing exit-1-as-not-installed as a Go error — that's the
// caller's job, mirroring realDpkgLookup).
func realRpmLookup(ctx context.Context, rpm, name string) (string, int, error) {
	cmd := exec.CommandContext(ctx, rpm, "-q", "--qf", "%{VERSION}-%{RELEASE}\n", name) //nolint:gosec // rpm is from exec.LookPath at detect time; name is validated by pkgNameRE before Lookup is called
	out, err := cmd.Output()
	if err == nil {
		return string(out), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// rpm prints "package <name> is not installed" to stdout (not
		// stderr) and returns exit 1; cmd.Output() still captures that
		// stdout, so return it for parseRpmQuery to recognise.
		return string(out), exitErr.ExitCode(), err
	}
	return "", -1, err
}
