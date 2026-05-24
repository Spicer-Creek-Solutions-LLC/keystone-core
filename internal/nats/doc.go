// SPDX-License-Identifier: Apache-2.0

// Package nats provides the v1.0 NATS transport for Keystone Core.
//
// Task 1 (this file set): a Manager that satisfies pkg/api/server.NATSManager —
// embedded mode (in-process nats-server/v2 reached via InProcessConn) and
// external mode (a single nats.Connect against a comma-joined URL list).
//
// Later Epic 05 tasks layer on top:
//   - Tasks 2–3: ConnectionManager with multi-endpoint failover, Endpoint /
//     EndpointState health tracking.
//   - Task 4: SubjectBuilder — every published subject prefixed
//     kscore.{cluster}.… per PROJECT-DETAILS §4.2.
//   - Task 5: Envelope wrapper.
//   - Task 6: SHA-256 dedup window.
//   - Task 7: Per-endpoint circuit breaker.
//   - Task 8: JetStream stream definitions.
//   - Task 9–10: Bootstrap registration handler + agent client.
//
// Manager is intentionally small: it owns the lifecycle and a single
// *nats.Conn used by Health and Publish. Higher-level abstractions wrap
// it without re-implementing connect/embedded logic.
package nats
