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
)
