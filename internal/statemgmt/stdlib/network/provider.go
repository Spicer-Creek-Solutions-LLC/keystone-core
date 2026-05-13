package network

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms.
var ErrUnsupportedOS = errors.New("network: unsupported OS for v1.0 (Linux only)")

// ErrNoIP is returned when the `ip` (iproute2) binary is not on
// PATH.
var ErrNoIP = errors.New("network: the ip (iproute2) binary was not found on PATH")

// ErrInterfaceNotFound is returned by GetInterface when the
// declared interface doesn't exist on the host. v1.0 does not
// create physical interfaces (and the dedicated `bond` / `bridge` /
// `vlan` modules create their respective virtual ones).
var ErrInterfaceNotFound = errors.New("network: interface not found")

func IsUnsupportedOS(err error) bool     { return errors.Is(err, ErrUnsupportedOS) }
func IsNoIP(err error) bool              { return errors.Is(err, ErrNoIP) }
func IsInterfaceNotFound(err error) bool { return errors.Is(err, ErrInterfaceNotFound) }

// InterfaceState is the snapshot of one interface's runtime
// configuration that Check / Apply compares against the
// declaration.
type InterfaceState struct {
	Name      string   // ifname
	Up        bool     // IFF_UP / admin state (NOT the lower-link operstate)
	MTU       int      // link MTU
	Addresses []string // canonical CIDR strings (IPv4 + IPv6 mixed)
}

// Provider abstracts the iproute2 operations the network module
// performs. Production shells out to `ip`; tests inject a fake.
type Provider interface {
	// GetInterface returns the current snapshot for `name`.
	// ErrInterfaceNotFound when it doesn't exist.
	GetInterface(ctx context.Context, name string) (*InterfaceState, error)
	// AddAddress runs `ip addr add <cidr> dev <name>`.
	AddAddress(ctx context.Context, name, cidr string) error
	// DelAddress runs `ip addr del <cidr> dev <name>`.
	DelAddress(ctx context.Context, name, cidr string) error
	// SetMTU runs `ip link set dev <name> mtu <mtu>`.
	SetMTU(ctx context.Context, name string, mtu int) error
	// SetLinkUp runs `ip link set dev <name> {up|down}`.
	SetLinkUp(ctx context.Context, name string, up bool) error
}

// commandRunner runs `ip`. It returns combined stdout+stderr and,
// on a non-zero exit, an error wrapping the exit code and trimmed
// output.
type commandRunner func(ctx context.Context, bin string, args []string) (string, error)
