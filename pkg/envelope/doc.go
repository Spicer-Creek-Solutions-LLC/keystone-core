// SPDX-License-Identifier: Apache-2.0

// Package envelope is the wire-format wrapper around every Keystone
// Core NATS message.
//
// PROJECT-DETAILS §4.2 mandates an Envelope around every published
// payload so dedup (Task 6), tracing (correlation IDs), priority
// routing (future), and TTL enforcement (future) can run without a
// per-domain wire-format renegotiation.
//
// Wire format is JSON for v1.0. The inner payload is json.RawMessage
// so it stays human-readable in tcpdumps and logs; binary payloads
// will get a separate path if/when they're needed.
//
// This package is intentionally framework-agnostic: it has no NATS
// dependency. Producers (internal/nats.Manager) and consumers
// (kscore-agent in Epic 06, external SDKs) both depend on it.
package envelope
