// SPDX-License-Identifier: Apache-2.0

// Package controlplane orchestrates the kscore-server runtime: agent
// connection state, command dispatch, and batch-job execution.
//
// The package owns the in-memory cache of registered agents, drives the
// heartbeat-monitor loop that marks unresponsive agents stale, and
// publishes commands to NATS once Epic 05 lands. It depends on
// internal/state for persistence and exposes interfaces consumed by
// the gRPC + REST listeners in pkg/api/server.
//
// See PROJECT-DETAILS.md §4.4 for the deterministic startup/shutdown
// sequence and the agent-status state machine documented in
// internal/state/types.go.
package controlplane
