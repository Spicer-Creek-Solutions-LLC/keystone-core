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

// aptProvider implements Provider against the apt + dpkg toolchain.
// aptGet and dpkgQuery are absolute paths resolved at detect time
// so the provider doesn't re-LookPath on every call.
//
// runner is the injection point that lets apt_test.go pin arg
// formation without invoking apt-get for real. defaultProvider
// wires it to execRun; tests inject a capturing shim.
type aptProvider struct {
	aptGet     string
	dpkgQuery  string
	runner     commandRunner
	dpkgLookup dpkgLookupFn
}

// dpkgLookupFn is the injection point for the Lookup path. The
// production impl shells out to dpkg-query; tests inject a fake
// that returns canned stdout/stderr so parseDpkgStatus exercises
// against fixed strings without touching the real database.
type dpkgLookupFn func(ctx context.Context, dpkgQuery, name string) (stdout string, exitCode int, err error)

func newAptProvider(aptGet, dpkgQuery string) *aptProvider {
	return &aptProvider{
		aptGet:     aptGet,
		dpkgQuery:  dpkgQuery,
		runner:     execRun,
		dpkgLookup: realDpkgLookup,
	}
}

func (p *aptProvider) Lookup(name string) (*PkgInfo, error) {
	stdout, exit, err := p.dpkgLookup(context.Background(), p.dpkgQuery, name)
	if err != nil && exit != 1 {
		// Exit 1 is the canonical "no packages found" — not a
		// real error. Anything else (binary missing, IO error)
		// surfaces.
		return nil, fmt.Errorf("dpkg-query %s: %w", name, err)
	}
	return parseDpkgStatus(name, stdout)
}

func (p *aptProvider) Install(ctx context.Context, name, version string) error {
	pkgSpec := name
	if version != "" {
		pkgSpec = name + "=" + version
	}
	args := []string{"install", "-y", "--no-install-recommends", pkgSpec}
	env := []string{"DEBIAN_FRONTEND=noninteractive"}
	return p.runner(ctx, p.aptGet, args, env)
}

func (p *aptProvider) Remove(ctx context.Context, name string) error {
	args := []string{"remove", "-y", name}
	env := []string{"DEBIAN_FRONTEND=noninteractive"}
	return p.runner(ctx, p.aptGet, args, env)
}

// parseDpkgStatus reads the output of:
//
//	dpkg-query -W -f='${db:Status-Abbrev} ${Version}\n' <name>
//
// db:Status-Abbrev is the 3-char field that summarises desired +
// status + error flags. The first character is desired action; the
// second is current status. We care about the second:
//
//	" i " (or "ii") — installed; the Version field is populated
//	" n " — not installed; Version is empty
//	" c " — config-files remaining (treated as not installed)
//
// Exit 1 from dpkg-query (passed as empty stdout) → not installed.
// Malformed lines return an error so a future dpkg format change is
// loud rather than silently treating packages as missing.
func parseDpkgStatus(name, stdout string) (*PkgInfo, error) {
	out := strings.TrimSpace(stdout)
	if out == "" {
		return &PkgInfo{Name: name, Installed: false}, nil
	}
	// Expected: "<3-char-abbrev> <version>" or just "<3-char-abbrev>"
	// when no version is recorded.
	parts := strings.SplitN(out, " ", 2)
	abbrev := strings.TrimSpace(parts[0])
	if len(abbrev) < 2 {
		return nil, fmt.Errorf("dpkg-query: unexpected status %q", out)
	}
	currentStatus := abbrev[1] // second char of the status abbrev
	switch currentStatus {
	case 'i':
		version := ""
		if len(parts) == 2 {
			version = strings.TrimSpace(parts[1])
		}
		return &PkgInfo{Name: name, Installed: true, Version: version}, nil
	case 'n', 'c', 'H', 'U', 'F', 'W', 't':
		// dpkg-query second-char codes (see dpkg(1)):
		//   n — not installed
		//   c — config-files only
		//   H — half-installed
		//   U — unpacked
		//   F — half-configured
		//   W — triggers-awaited
		//   t — triggers-pending
		// None of these are "currently usable installed", so they
		// drift from state=installed.
		return &PkgInfo{Name: name, Installed: false}, nil
	default:
		return nil, fmt.Errorf("dpkg-query: unknown status %q for %s", abbrev, name)
	}
}

// realDpkgLookup invokes dpkg-query and returns stdout + exit code
// (without surfacing exit-1-as-not-installed as a Go error — that's
// the caller's job).
func realDpkgLookup(ctx context.Context, dpkgQuery, name string) (string, int, error) {
	cmd := exec.CommandContext(ctx, dpkgQuery, "-W", "-f=${db:Status-Abbrev} ${Version}\n", name) //nolint:gosec // dpkgQuery is from exec.LookPath at detect time; name is validated by pkgNameRE before Lookup is called
	out, err := cmd.Output()
	if err == nil {
		return string(out), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// dpkg-query writes its "no packages found" message to
		// stderr (captured in exitErr.Stderr) and returns exit 1.
		// Stdout in that case is empty.
		return "", exitErr.ExitCode(), err
	}
	return "", -1, err
}

// execRun is the production commandRunner. Captures stderr into the
// wrapped error so the operator sees apt-get's actual complaint.
func execRun(ctx context.Context, bin string, args []string, env []string) error {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin is a fixed module-internal constant
	cmd.Env = append(cmd.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("%s %s: exit %d: %s", bin, strings.Join(args, " "), exitErr.ExitCode(), strings.TrimSpace(string(out)))
	}
	return fmt.Errorf("%s %s: %w", bin, strings.Join(args, " "), err)
}
