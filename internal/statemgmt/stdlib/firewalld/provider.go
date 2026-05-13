package firewalld

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms (firewalld is
// Linux-only).
var ErrUnsupportedOS = errors.New("firewalld: unsupported OS for v0.1 (Linux only)")

// ErrNoFirewallCmd is returned when the `firewall-cmd` binary is not
// on PATH.
var ErrNoFirewallCmd = errors.New("firewalld: the firewall-cmd binary was not found on PATH")

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsNoFirewallCmd(err error) bool { return errors.Is(err, ErrNoFirewallCmd) }

// Item is one firewalld zone item this module manages — a service,
// a port, or a rich rule. Kind is the suffix used in the
// `firewall-cmd --<verb>-<kind>=<value>` flags.
type Item struct {
	Kind  string // service | port | rich-rule
	Value string
}

// Provider abstracts the firewall-cmd operations. Production shells
// out to `firewall-cmd`; tests inject a fake. Every operation acts
// on firewalld's *permanent* configuration; Reload makes a permanent
// change active in the running firewalld.
type Provider interface {
	// Has reports whether the item is currently enabled on the zone
	// (`firewall-cmd --permanent --zone=Z --query-<kind>=<v>` exits
	// 0). Any non-zero exit is taken as "not enabled" — a structural
	// problem (a missing zone) surfaces from Add/Remove instead.
	Has(ctx context.Context, zone string, it Item) (bool, error)
	// Add enables the item on the zone.
	Add(ctx context.Context, zone string, it Item) error
	// Remove disables the item on the zone.
	Remove(ctx context.Context, zone string, it Item) error
	// Reload runs `firewall-cmd --reload` (push permanent → runtime).
	Reload(ctx context.Context) error
}

// commandRunner runs `firewall-cmd`. It returns combined stdout +
// stderr and, on a non-zero exit, an error wrapping the exit code
// and trimmed output.
type commandRunner func(ctx context.Context, bin string, args []string) (string, error)
