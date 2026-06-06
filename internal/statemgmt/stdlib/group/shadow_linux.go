// SPDX-License-Identifier: Apache-2.0

//go:build linux

package group

import (
	"context"
	"strconv"
)

// shadowProvider shells out to the shadow-utils toolchain
// (groupadd / groupmod / groupdel). These binaries live in /usr/sbin
// on every mainstream distro; we don't pin a path so PATH resolution
// finds whichever wrapper the OS ships. Alpine provides them only
// when the `shadow` package is installed — otherwise the BusyBox
// backend handles the host (see busyboxProvider + defaultProvider's
// detection).
type shadowProvider struct{ osLookup }

func (p shadowProvider) Add(ctx context.Context, name string, gid *int, system bool) error {
	args := []string{}
	if gid != nil {
		args = append(args, "--gid", strconv.Itoa(*gid))
	}
	if system {
		args = append(args, "--system")
	}
	args = append(args, name)
	return runManaged(ctx, "groupadd", args)
}

func (p shadowProvider) Mod(ctx context.Context, name string, gid int) error {
	return runManaged(ctx, "groupmod", []string{"--gid", strconv.Itoa(gid), name})
}

func (p shadowProvider) Del(ctx context.Context, name string) error {
	return runManaged(ctx, "groupdel", []string{name})
}
