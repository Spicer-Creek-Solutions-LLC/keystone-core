// SPDX-License-Identifier: Apache-2.0

// Package natsstatus carries the public observability types for the
// Keystone Core NATS transport: per-endpoint state, circuit-breaker
// state, and the EndpointSnapshot rendered into /api/status.
//
// These types live outside internal/nats so HTTP handlers, future
// SDK consumers, and operator dashboards can render snapshots
// without crossing an internal-package boundary. internal/nats's
// equivalent types are aliases of the public versions.
//
// JSON tags pin the wire shape — operator dashboards depending on
// these field names should remain stable across v1.x.
package natsstatus
