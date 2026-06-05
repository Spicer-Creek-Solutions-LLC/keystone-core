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

// apkProvider implements Provider against Alpine's apk toolchain.
//
// Unlike apt (apt-get + dpkg-query) and dnf (dnf + rpm), apk is a
// single binary that does everything — add / del for mutation and
// `apk list --installed` for the read-only query — so there is one
// resolved path rather than a mutate/query pair.
//
// runner is the injection point that lets apk_test.go pin arg
// formation without invoking apk for real; newApkProvider wires it to
// execRun. listLookup is the matching seam for the query path.
type apkProvider struct {
	apk        string
	runner     commandRunner
	listLookup apkListFn
}

// apkListFn is the injection point for the Lookup path. The production
// impl shells out to `apk list --installed <name>`; tests inject a
// fake returning canned stdout so parseApkList exercises against fixed
// strings without touching a real Alpine install.
type apkListFn func(ctx context.Context, apk, name string) (stdout string, err error)

func newApkProvider(apk string) *apkProvider {
	return &apkProvider{
		apk:        apk,
		runner:     execRun,
		listLookup: realApkList,
	}
}

func (p *apkProvider) Lookup(name string) (*PkgInfo, error) {
	stdout, err := p.listLookup(context.Background(), p.apk, name)
	if err != nil {
		return nil, fmt.Errorf("apk list --installed %s: %w", name, err)
	}
	return parseApkList(name, stdout), nil
}

func (p *apkProvider) Install(ctx context.Context, name, version string) error {
	// apk pins with "name=version" (same shape as apt), e.g.
	// nginx=1.20.1-r0. Unversioned installs the latest available.
	pkgSpec := name
	if version != "" {
		pkgSpec = name + "=" + version
	}
	args := []string{"add", pkgSpec}
	return p.runner(ctx, p.apk, args, nil)
}

func (p *apkProvider) Remove(ctx context.Context, name string) error {
	args := []string{"del", name}
	return p.runner(ctx, p.apk, args, nil)
}

// parseApkList reads the output of `apk list --installed <name>`,
// whose installed lines look like:
//
//	nginx-1.20.1-r0 x86_64 {nginx} (BSD-2-Clause) [installed]
//
// The first whitespace field is the "<name>-<version>" token. Because
// the queried name is known, the version is the field with the
// "<name>-" prefix stripped — robust to hyphens inside the package
// name (apk versions are themselves "<pkgver>-r<pkgrel>", so naive
// last-dash splitting would be wrong).
//
// apk list returns exit 0 with empty output when nothing matches, so
// absence is the empty/no-matching-line case rather than an exit code.
// Lines lacking the [installed] marker, or whose token is not exactly
// this package, are skipped — so it never fails to parse (an
// unrecognised line just means "not this package").
//
// The exact-package test: the token is "<name>-<version>", and an apk
// pkgver always begins with a digit. So a line counts only if its
// token has the "<name>-" prefix AND the remainder starts with a
// digit. That disambiguates "nginx-1.20.1-r0" (package nginx) from
// "nginx-module-foo-1.0-r0" (a different package the name glob also
// matched), which strips to "module-foo-..." — not digit-led.
func parseApkList(name, stdout string) *PkgInfo {
	prefix := name + "-"
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "[installed]") {
			continue
		}
		token := line
		if i := strings.IndexByte(token, ' '); i >= 0 {
			token = token[:i]
		}
		if !strings.HasPrefix(token, prefix) {
			continue // different package matched by the name glob
		}
		version := token[len(prefix):]
		if version == "" || version[0] < '0' || version[0] > '9' {
			continue // not this package's "<name>-<digit…>" token
		}
		return &PkgInfo{Name: name, Installed: true, Version: version}
	}
	return &PkgInfo{Name: name, Installed: false}
}

// realApkList invokes `apk list --installed <name>`. apk returns exit 0
// even when nothing matches (empty stdout), so a non-nil error here is
// a genuine failure (binary missing, IO error) and is surfaced —
// unlike the apt/dnf paths, there is no exit-1-as-absent convention to
// swallow.
func realApkList(ctx context.Context, apk, name string) (string, error) {
	cmd := exec.CommandContext(ctx, apk, "list", "--installed", name) //nolint:gosec // apk is from exec.LookPath at detect time; name is validated by pkgNameRE before Lookup is called
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("exit %d: %s", exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}
