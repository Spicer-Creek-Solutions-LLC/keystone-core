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
