// SPDX-License-Identifier: Apache-2.0

//go:build linux

package group

import (
	"context"
	"strconv"
)

// busyboxProvider drives BusyBox's group applets (addgroup /
// delgroup), the toolchain Alpine ships in place of shadow-utils.
// There is no groupmod equivalent, so Mod is unavailable
// (ErrModUnsupported). The read path (Lookup) is the shared,
// cross-platform osLookup.
type busyboxProvider struct {
	osLookup
	addgroup string
	delgroup string

	// run is the exec seam; production wiring is runManaged. Tests
	// inject a recorder to assert arg formation without invoking
	// BusyBox.
	run commandRunner
}

func newBusyboxProvider(addgroup string) *busyboxProvider {
	return &busyboxProvider{
		addgroup: addgroup,
		delgroup: "delgroup",
		run:      runManaged,
	}
}

// Add creates the group via `addgroup [-g GID] [-S] NAME`.
func (p *busyboxProvider) Add(ctx context.Context, name string, gid *int, system bool) error {
	args := []string{}
	if gid != nil {
		args = append(args, "-g", strconv.Itoa(*gid))
	}
	if system {
		args = append(args, "-S")
	}
	args = append(args, name)
	return p.run(ctx, p.addgroup, args)
}

// Mod is unavailable on BusyBox: there is no groupmod, so an existing
// group's GID can't be changed in place.
func (p *busyboxProvider) Mod(_ context.Context, _ string, _ int) error {
	return ErrModUnsupported
}

// Del removes the group via `delgroup NAME`.
func (p *busyboxProvider) Del(ctx context.Context, name string) error {
	return p.run(ctx, p.delgroup, []string{name})
}
