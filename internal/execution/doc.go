// SPDX-License-Identifier: Apache-2.0

// Package execution provides execution primitives shared across the
// agent runtime and the control plane.
//
// The Executor interface (executor.go) describes a single synchronous
// command attempt — `internal/agent.Executor` satisfies it via a tiny
// adapter, so this package owns no os/exec wrapping itself.
//
// ManagedExecution (managed.go) wraps any Executor with a lifecycle
// state machine, observable Callbacks, and a retry policy. States
// follow PROJECT-DETAILS §4.7:
//
//	PENDING → RUNNING → COMPLETED              (exit 0)
//	                  → FAILED                 (non-zero exit, retries exhausted)
//	                  → TIMEOUT                (per-call or context deadline)
//	                  → CANCELLED              (parent context cancelled)
//	                  → RETRYING → RUNNING ... (retry policy active, attempt < max)
//
// Wiring into the agent's command handler is intentionally deferred —
// this task delivers the lifecycle primitive only. Tasks 5-9 build on
// top (Pipeline, Shell, CommandPolicy, BatchDispatcher).
package execution
