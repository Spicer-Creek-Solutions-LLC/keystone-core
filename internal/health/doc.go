// SPDX-License-Identifier: Apache-2.0

// Package health is the v1.0 component-health subsystem behind the
// kscore-server /health/* endpoints.
//
// Registry hosts a set of Checkers; Snapshot runs them in parallel,
// bounded by a per-check timeout, and returns one Result per checker.
// The HTTP handlers in pkg/api/server translate the Registry's snapshot
// into the long-standing public JSON wire format.
//
// Concrete checkers: NATS connection (via internal/nats.Manager.Health),
// DB ping (via state.HealthStore.Ping), JetStream (via the manager's
// JetStream accessor), and arbitrary operator-supplied custom checks.
// All four are thin wrappers around PingChecker; subsystems wanting a
// richer probe satisfy the same interface (Name + Interval + Check).
//
// Status values:
//
//   - StatusHealthy   — Check returned nil.
//   - StatusUnhealthy — Check returned non-nil (or timed out).
//   - StatusUnknown   — Never observed (reserved for background pollers
//     that haven't fired their first probe yet).
//   - StatusDegraded  — Reserved. v1.0 never produces this; future work
//     (consecutive-failure tracking) will.
//
// The Interval() seam on the Checker interface is a hint for future
// background poll-and-cache work; v1.0 always runs checks on demand at
// snapshot time.
package health
