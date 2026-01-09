// Package mirror implements mirror groups and geographic routing for file distribution.
package mirror

import "errors"

var (
	// ErrNoWritableMirrors indicates no writable mirrors are available.
	ErrNoWritableMirrors = errors.New("no writable mirrors available")

	// ErrInsufficientQuorum indicates not enough mirrors for quorum.
	ErrInsufficientQuorum = errors.New("insufficient mirrors for quorum")

	// ErrMirrorNotFound indicates the specified mirror was not found.
	ErrMirrorNotFound = errors.New("mirror not found")

	// ErrMirrorGroupNotFound indicates the specified mirror group was not found.
	ErrMirrorGroupNotFound = errors.New("mirror group not found")

	// ErrMirrorUnhealthy indicates the mirror is unhealthy.
	ErrMirrorUnhealthy = errors.New("mirror is unhealthy")

	// ErrAllMirrorsUnhealthy indicates all mirrors are unhealthy.
	ErrAllMirrorsUnhealthy = errors.New("all mirrors are unhealthy")

	// ErrWriteQuorumFailed indicates write quorum was not achieved.
	ErrWriteQuorumFailed = errors.New("write quorum failed")

	// ErrCircuitOpen indicates the circuit breaker is open.
	ErrCircuitOpen = errors.New("circuit breaker is open")

	// ErrProbeTimeout indicates the latency probe timed out.
	ErrProbeTimeout = errors.New("latency probe timeout")
)
