// SPDX-License-Identifier: Apache-2.0

package health

import (
	"context"
	"errors"
	"time"
)

// NATSPinger is the tiny seam the NATSChecker probes. Satisfied by
// *internal/nats.Manager and any test double exposing the same
// signature.
type NATSPinger interface {
	Health(ctx context.Context) error
}

// DBPinger is the seam the DBChecker probes. Satisfied by
// state.HealthStore (which is embedded in the composite state.Store).
type DBPinger interface {
	Ping(ctx context.Context) error
}

// JetStreamPinger is the seam the JetStreamChecker probes. A bare
// JetStream() (context, error) — typical of internal/nats.Manager —
// adapts via JetStreamPingerFunc.
type JetStreamPinger interface {
	Check(ctx context.Context) error
}

// JetStreamPingerFunc adapts a func to JetStreamPinger. Useful when the
// caller wants to wrap nats.Manager.JetStream() (which returns
// (jsCtx, error)) into the "ctx → error" shape.
type JetStreamPingerFunc func(ctx context.Context) error

// Check implements JetStreamPinger.
func (f JetStreamPingerFunc) Check(ctx context.Context) error { return f(ctx) }

// PingChecker is the shared implementation behind every v1.0 concrete
// checker. Name + Interval are constants; Check delegates to a closure.
// Subsystems that want a richer probe satisfy Checker directly rather
// than wedging extra state in here.
type PingChecker struct {
	name     string
	interval time.Duration
	fn       func(ctx context.Context) error
}

// NewPingChecker constructs a PingChecker. fn==nil treats every Check
// as healthy — callers wanting a permanently-failing checker should
// pass a function that returns a sentinel error instead.
func NewPingChecker(name string, fn func(ctx context.Context) error, interval time.Duration) *PingChecker {
	return &PingChecker{name: name, interval: interval, fn: fn}
}

// Name implements Checker.
func (c *PingChecker) Name() string { return c.name }

// Interval implements Checker.
func (c *PingChecker) Interval() time.Duration { return c.interval }

// Check implements Checker.
func (c *PingChecker) Check(ctx context.Context) error {
	if c == nil || c.fn == nil {
		return nil
	}
	return c.fn(ctx)
}

// Compile-time interface compliance.
var _ Checker = (*PingChecker)(nil)

// NewNATSChecker probes a NATSPinger. Standard name "nats".
func NewNATSChecker(p NATSPinger, interval time.Duration) *PingChecker {
	if p == nil {
		return NewPingChecker("nats", errNoBackend("nats"), interval)
	}
	return NewPingChecker("nats", p.Health, interval)
}

// NewDBChecker probes a DBPinger. Standard name "db".
func NewDBChecker(p DBPinger, interval time.Duration) *PingChecker {
	if p == nil {
		return NewPingChecker("db", errNoBackend("db"), interval)
	}
	return NewPingChecker("db", p.Ping, interval)
}

// NewJetStreamChecker probes a JetStreamPinger. Standard name
// "jetstream".
func NewJetStreamChecker(p JetStreamPinger, interval time.Duration) *PingChecker {
	if p == nil {
		return NewPingChecker("jetstream", errNoBackend("jetstream"), interval)
	}
	return NewPingChecker("jetstream", p.Check, interval)
}

// NewCustomChecker wraps an operator-supplied function. Name is
// caller-chosen — by convention lowercase, no spaces, matching the
// public JSON component-map key.
func NewCustomChecker(name string, fn func(ctx context.Context) error, interval time.Duration) *PingChecker {
	return NewPingChecker(name, fn, interval)
}

// errNoBackend returns a check-fn that reports "no backend configured"
// — used when a constructor receives nil. Keeping the checker registered
// (rather than skipping it) is intentional: operators see a clear
// "component=db, status=unhealthy, err=no backend" instead of silently
// missing the component from /health/ready.
func errNoBackend(component string) func(ctx context.Context) error {
	err := errors.New(component + ": no backend configured")
	return func(context.Context) error { return err }
}
