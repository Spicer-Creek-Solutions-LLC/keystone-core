package route

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms.
var ErrUnsupportedOS = errors.New("route: unsupported OS for v1.0 (Linux only)")

// ErrNoIP is returned when the `ip` (iproute2) binary is not on
// PATH.
var ErrNoIP = errors.New("route: the ip (iproute2) binary was not found on PATH")

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsNoIP(err error) bool          { return errors.Is(err, ErrNoIP) }

// RouteQuery identifies one route for lookup / deletion: the
// destination (in the requested table) plus an optional metric.
// `ip` allows multiple routes to the same destination at different
// metrics; declaring a metric narrows the match.
type RouteQuery struct {
	Destination string
	Metric      int
	HasMetric   bool
	Table       string
}

// RouteSpec is the full set of attributes for `ip route replace`.
type RouteSpec struct {
	Destination string
	Gateway     string
	Interface   string
	Metric      int
	HasMetric   bool
	Table       string
}

// RouteEntry is the snapshot of a single route returned by
// GetRoute. Gateway / Interface may be "" when the kernel reports a
// route with only one of those (a direct-attached `dev`-only route,
// or a `via`-only nexthop with no explicit dev).
type RouteEntry struct {
	Destination string
	Gateway     string
	Interface   string
	Metric      int
	HasMetric   bool
	Table       string
}

// Provider abstracts the iproute2 routing-table operations.
// Production shells out to `ip`; tests inject a fake.
type Provider interface {
	// GetRoute returns the route matching the query, or (nil, nil)
	// when no such route exists. A real I/O failure surfaces as an
	// error.
	GetRoute(ctx context.Context, q RouteQuery) (*RouteEntry, error)
	// ReplaceRoute runs `ip route replace <spec>`, which adds the
	// route if absent or overwrites an existing entry at the same
	// (destination, metric, table) — the idempotency-friendly verb.
	ReplaceRoute(ctx context.Context, s RouteSpec) error
	// DelRoute runs `ip route del <query>`.
	DelRoute(ctx context.Context, q RouteQuery) error
}

// commandRunner runs `ip`. It returns combined stdout+stderr and,
// on a non-zero exit, an error wrapping the exit code and trimmed
// output.
type commandRunner func(ctx context.Context, bin string, args []string) (string, error)
