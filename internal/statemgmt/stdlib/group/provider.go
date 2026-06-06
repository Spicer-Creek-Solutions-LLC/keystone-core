// SPDX-License-Identifier: Apache-2.0

package group

import (
	"context"
	"errors"
	"fmt"
	"os/user"
	"strconv"
)

// ErrUnsupportedOS is returned from mutating Provider methods on
// platforms where v1.0 doesn't ship a real implementation. Lookup
// works cross-platform (it uses os/user.LookupGroup which goes
// through NSS); only Add / Mod / Del are gated.
var ErrUnsupportedOS = errors.New("group: unsupported OS for mutating operations in v1.0 (Linux only)")

// ErrNoBackend is returned on a Linux host where no supported
// group-management toolchain was detected — neither shadow-utils
// (groupadd/groupmod/groupdel) nor BusyBox (addgroup/delgroup).
var ErrNoBackend = errors.New("group: no supported group toolchain detected on this host (neither shadow-utils groupadd nor BusyBox addgroup found)")

// ErrModUnsupported is returned by the BusyBox backend's Mod path.
// BusyBox ships no groupmod equivalent, so changing an existing
// group's GID is not possible without shadow-utils. The operator
// should install the shadow package or recreate the group.
var ErrModUnsupported = errors.New("group: changing an existing group's GID is unavailable on BusyBox (no groupmod); install the shadow package or recreate the group")

// commandRunner is the injection point that lets the BusyBox
// backend's tests pin arg formation without invoking addgroup. The
// production wiring is runManaged.
type commandRunner func(ctx context.Context, bin string, args []string) error

// IsNoBackend / IsModUnsupported expose the new sentinel matchers so
// the gRPC server + CLI can render friendlier messages on the
// operator-facing surface. (IsUnsupportedOS lives in group.go.)
func IsNoBackend(err error) bool      { return errors.Is(err, ErrNoBackend) }
func IsModUnsupported(err error) bool { return errors.Is(err, ErrModUnsupported) }

// GroupInfo is the on-system shape we care about. Linux groups also
// carry a list of members; v1.0 ignores members at the group level
// (the user module's groups: param manages supplementary memberships).
type GroupInfo struct {
	Name string
	GID  int
}

// Provider is the OS-level surface the group module depends on.
// Production code uses the platform-specific real impl returned by
// defaultProvider(); tests inject a fake.
type Provider interface {
	// Lookup returns nil, nil when no group exists with that name.
	Lookup(name string) (*GroupInfo, error)

	// Add creates the group. gid==nil lets the system choose.
	// system==true requests a system-group GID range
	// (groupadd --system on Linux).
	Add(ctx context.Context, name string, gid *int, system bool) error

	// Mod changes an existing group's GID.
	Mod(ctx context.Context, name string, gid int) error

	// Del removes the group.
	Del(ctx context.Context, name string) error
}

// osLookup implements the cross-platform Lookup half of Provider
// using os/user.LookupGroup. Real platform providers embed this so
// they don't reimplement the read path.
type osLookup struct{}

func (osLookup) Lookup(name string) (*GroupInfo, error) {
	g, err := user.LookupGroup(name)
	if err != nil {
		var unknown user.UnknownGroupError
		if errors.As(err, &unknown) {
			return nil, nil
		}
		return nil, fmt.Errorf("lookup group %q: %w", name, err)
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return nil, fmt.Errorf("parse gid for %q: %w", name, err)
	}
	return &GroupInfo{Name: g.Name, GID: gid}, nil
}
