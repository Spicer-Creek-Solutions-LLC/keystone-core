package bond

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms.
var ErrUnsupportedOS = errors.New("bond: unsupported OS for v0.1 (Linux only)")

// ErrNoIP is returned when the `ip` (iproute2) binary is not on PATH.
var ErrNoIP = errors.New("bond: the ip (iproute2) binary was not found on PATH")

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsNoIP(err error) bool          { return errors.Is(err, ErrNoIP) }

// LinkInfo is the minimal subset of `ip -d -j link show <name>`
// the bond module needs. Kind is the kernel kind name (`bond`,
// `bridge`, `vlan`, `veth`, …) or "" for plain devices.
type LinkInfo struct {
	Name string
	Kind string
}

// BondSpec carries the create-time attributes.
type BondSpec struct {
	Name      string
	Mode      string // canonical name (`balance-rr`, `active-backup`, …)
	Miimon    int
	HasMiimon bool
}

// Provider abstracts the iproute2 operations the bond module
// performs.
type Provider interface {
	// GetLink returns metadata for the named interface or (nil, nil)
	// when the interface doesn't exist. Real I/O failures surface as
	// errors.
	GetLink(ctx context.Context, name string) (*LinkInfo, error)
	// CreateBond runs `ip link add <name> type bond mode <m> [miimon
	// N]`.
	CreateBond(ctx context.Context, s BondSpec) error
	// DeleteLink runs `ip link del <name>`.
	DeleteLink(ctx context.Context, name string) error
	// SetMaster runs `ip link set <child> master <master>`.
	SetMaster(ctx context.Context, child, master string) error
}

// commandRunner runs `ip`. It returns combined stdout+stderr and,
// on a non-zero exit, an error wrapping the exit code and trimmed
// output.
type commandRunner func(ctx context.Context, bin string, args []string) (string, error)
