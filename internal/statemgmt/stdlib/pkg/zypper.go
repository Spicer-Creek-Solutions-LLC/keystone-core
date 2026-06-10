// SPDX-License-Identifier: Apache-2.0

//go:build linux

package pkg

import (
	"context"
	"fmt"
)

// zypperProvider implements Provider against the zypper + rpm toolchain
// (openSUSE / SLES). openSUSE is an rpm distro, so the read-only query
// path is identical to dnf's — it reuses rpmLookupFn / realRpmLookup /
// parseRpmQuery (`rpm -q --qf '%{VERSION}-%{RELEASE}'`). Only the
// mutating verbs differ: zypper instead of dnf, and zypper's exact-
// edition pin syntax `name=version` instead of dnf's `name-version`.
//
// runner is the injection point that lets zypper_test.go pin arg
// formation without invoking zypper for real; newZypperProvider wires it
// to execRun. rpmLookup is the matching seam for the query path.
type zypperProvider struct {
	zypper    string
	rpm       string
	runner    commandRunner
	rpmLookup rpmLookupFn
}

func newZypperProvider(zypper, rpm string) *zypperProvider {
	return &zypperProvider{
		zypper:    zypper,
		rpm:       rpm,
		runner:    execRun,
		rpmLookup: realRpmLookup,
	}
}

func (p *zypperProvider) Lookup(name string) (*PkgInfo, error) {
	stdout, exit, err := p.rpmLookup(context.Background(), p.rpm, name)
	if err != nil && exit != 1 {
		// Exit 1 is rpm's canonical "package is not installed"; anything
		// else (binary missing, IO error) surfaces.
		return nil, fmt.Errorf("rpm -q %s: %w", name, err)
	}
	return parseRpmQuery(name, stdout)
}

func (p *zypperProvider) Install(ctx context.Context, name, version string) error {
	// zypper's exact-version spec is "name=version" (e.g. nginx=1.20.1-1),
	// using its edition-match `=` operator. Unversioned installs the
	// latest available.
	pkgSpec := name
	if version != "" {
		pkgSpec = name + "=" + version
	}
	// --non-interactive auto-confirms the install (and accepts licenses),
	// the canonical scripted form.
	args := []string{"--non-interactive", "install", pkgSpec}
	return p.runner(ctx, p.zypper, args, nil)
}

func (p *zypperProvider) Remove(ctx context.Context, name string) error {
	args := []string{"--non-interactive", "remove", name}
	return p.runner(ctx, p.zypper, args, nil)
}
