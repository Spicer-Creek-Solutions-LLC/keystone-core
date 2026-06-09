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

// ErrVersionPinUnsupported is returned by the pacman backend when a
// declaration pins a specific version. Arch is a rolling release: the
// repositories only ever carry the current version, and pacman has no
// `name=version` install spec, so an exact-version request cannot be
// honoured. Erroring is the honest answer — installing the latest
// instead would silently violate the declared version.
var ErrVersionPinUnsupported = errors.New("pkg: pacman cannot install a specific version (Arch is a rolling release; only the repo's current version is installable)")

// pacmanProvider implements Provider against Arch's pacman. Like apk,
// pacman is a single binary: -S / -R for mutation and -Q for the
// read-only query.
//
// runner is the injection point that lets pacman_test.go pin arg
// formation without invoking pacman for real; newPacmanProvider wires it
// to execRun. query is the matching seam for the lookup path.
type pacmanProvider struct {
	pacman string
	runner commandRunner
	query  pacmanQueryFn
}

// pacmanQueryFn is the injection point for the Lookup path. The
// production impl shells out to `pacman -Q <name>`; tests inject a fake
// returning canned stdout + exit so parsePacmanQuery exercises against
// fixed strings without touching a real Arch install.
type pacmanQueryFn func(ctx context.Context, pacman, name string) (stdout string, exitCode int, err error)

func newPacmanProvider(pacman string) *pacmanProvider {
	return &pacmanProvider{
		pacman: pacman,
		runner: execRun,
		query:  realPacmanQuery,
	}
}

func (p *pacmanProvider) Lookup(name string) (*PkgInfo, error) {
	stdout, exit, err := p.query(context.Background(), p.pacman, name)
	if err != nil && exit != 1 {
		// Exit 1 is pacman's "package was not found" — not a real error.
		// Anything else (binary missing, IO error) surfaces.
		return nil, fmt.Errorf("pacman -Q %s: %w", name, err)
	}
	return parsePacmanQuery(name, stdout, exit), nil
}

func (p *pacmanProvider) Install(ctx context.Context, name, version string) error {
	if version != "" {
		return fmt.Errorf("%w (requested %s=%s)", ErrVersionPinUnsupported, name, version)
	}
	// --noconfirm suppresses the install prompt; -S syncs from the repos.
	args := []string{"-S", "--noconfirm", name}
	return p.runner(ctx, p.pacman, args, nil)
}

func (p *pacmanProvider) Remove(ctx context.Context, name string) error {
	args := []string{"-R", "--noconfirm", name}
	return p.runner(ctx, p.pacman, args, nil)
}

// parsePacmanQuery reads the output of `pacman -Q <name>`, an installed
// line of which is:
//
//	nginx 1.20.1-1
//
// (the package name, a space, then "<pkgver>-<pkgrel>"). exit==1 (with
// the "was not found" message on stderr, so stdout is empty) means the
// package is not installed. The version is the second whitespace field.
func parsePacmanQuery(name, stdout string, exit int) *PkgInfo {
	out := strings.TrimSpace(stdout)
	if exit != 0 || out == "" {
		return &PkgInfo{Name: name, Installed: false}
	}
	fields := strings.Fields(out)
	if len(fields) < 2 {
		// Installed but unparseable version line; report installed without
		// a version rather than dropping the install state.
		return &PkgInfo{Name: name, Installed: true}
	}
	return &PkgInfo{Name: name, Installed: true, Version: fields[1]}
}

// realPacmanQuery invokes `pacman -Q <name>` and returns stdout + exit
// code without surfacing exit-1-as-not-installed as a Go error (the
// caller interprets the code, mirroring realRpmLookup).
func realPacmanQuery(ctx context.Context, pacman, name string) (string, int, error) {
	cmd := exec.CommandContext(ctx, pacman, "-Q", name) //nolint:gosec // pacman is from exec.LookPath at detect time; name is validated by pkgNameRE before Lookup is called
	out, err := cmd.Output()
	if err == nil {
		return string(out), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return string(out), exitErr.ExitCode(), err
	}
	return "", -1, err
}
