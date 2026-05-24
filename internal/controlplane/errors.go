// SPDX-License-Identifier: Apache-2.0

package controlplane

import "errors"

var (
	// ErrNotRegistered is returned when an operation references an
	// agent ID that is not present in the cache or store.
	ErrNotRegistered = errors.New("controlplane: agent not registered")

	// ErrAgentDisabled is returned by Heartbeat when the target agent
	// has been administratively disabled. The caller (typically the
	// gRPC AgentService) is expected to refuse the heartbeat upstream.
	ErrAgentDisabled = errors.New("controlplane: agent disabled")

	// ErrClosed is returned by ConnectionManager methods invoked after
	// Stop has begun. Stop is one-shot.
	ErrClosed = errors.New("controlplane: connection manager closed")

	// ErrNotStarted is returned when a method that depends on the
	// monitor loop is called before Start.
	ErrNotStarted = errors.New("controlplane: connection manager not started")

	// ErrAgentUnreachable is returned by CommandDispatcher.Dispatch
	// when the target agent is unknown, disabled, or otherwise
	// ineligible to receive commands. Stale agents are NOT rejected —
	// the timeout backstop catches the no-response case so recovery
	// scenarios still flow.
	ErrAgentUnreachable = errors.New("controlplane: agent unreachable")

	// ErrCommandNotFound is returned by RecordResult and Cancel when
	// the command ID is unknown to the dispatcher.
	ErrCommandNotFound = errors.New("controlplane: command not found")

	// ErrCommandFinalized is returned when RecordResult is invoked on
	// a command that has already reached a terminal status. Late
	// duplicate results from the agent are common (retries during a
	// network blip); callers may choose to ignore this error.
	ErrCommandFinalized = errors.New("controlplane: command already finalized")

	// ErrInvalidDispatch is returned when DispatchRequest is missing
	// required fields (AgentID, Command).
	ErrInvalidDispatch = errors.New("controlplane: invalid dispatch request")

	// ErrBatchNotFound is returned when a batch ID is unknown.
	ErrBatchNotFound = errors.New("controlplane: batch not found")

	// ErrBatchInvalidState is returned when a state transition is
	// attempted from a status that disallows it (e.g., RecordAgentResult
	// on a pending batch, MarkRunning on an already-running batch).
	ErrBatchInvalidState = errors.New("controlplane: batch in invalid state for operation")

	// ErrBatchFinalized is returned when an operation is attempted on a
	// batch that has already reached a terminal status.
	ErrBatchFinalized = errors.New("controlplane: batch already finalized")

	// ErrInvalidBatchRequest is returned when BatchRequest is missing
	// required fields (Command, TotalAgents > 0).
	ErrInvalidBatchRequest = errors.New("controlplane: invalid batch request")
)
