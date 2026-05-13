package firewall

import (
	"context"
	"errors"
)

// Backend names. Also the keys into the Module.backends map and the
// values the `backend:` declaration param accepts.
const (
	BackendIptables  = "iptables"
	BackendNftables  = "nftables"
	BackendFirewalld = "firewalld"
)

// ErrUnsupportedOS is returned by the default detector on non-Linux
// platforms (the v1.0 stdlib firewall backends are all Linux-only).
var ErrUnsupportedOS = errors.New("firewall: unsupported OS for v1.0 (Linux only)")

// ErrNoFirewall is returned when no firewall backend is present —
// none of `firewall-cmd`, `iptables`, or `nft` are on PATH.
var ErrNoFirewall = errors.New("firewall: no firewall backend found on PATH (firewall-cmd, iptables, or nft)")

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsNoFirewall(err error) bool    { return errors.Is(err, ErrNoFirewall) }

// BackendDetector picks the firewall backend in use on the host. The
// abstraction calls Detect once per Check/Apply when no `backend:`
// override is set.
type BackendDetector interface {
	Detect(ctx context.Context) (string, error)
}

// commandRunner is the seam the Linux detector uses to run
// `firewall-cmd --state`. Tests replace it with a stub.
type commandRunner func(ctx context.Context, bin string, args []string) (string, error)
