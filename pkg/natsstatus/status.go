// SPDX-License-Identifier: Apache-2.0

package natsstatus

import "time"

// EndpointStatus is the observed reachability state for one NATS
// endpoint. ConnectionManager updates this from disconnect/reconnect
// callbacks; the breaker (CircuitStatus) is independent metadata.
type EndpointStatus string

const (
	EndpointStatusUnknown      EndpointStatus = "unknown"
	EndpointStatusConnecting   EndpointStatus = "connecting"
	EndpointStatusConnected    EndpointStatus = "connected"
	EndpointStatusDisconnected EndpointStatus = "disconnected"
	EndpointStatusFailed       EndpointStatus = "failed"
)

// CircuitStatus is the per-endpoint breaker state (Epic 05 task 7
// state machine: closed → open → half-open → closed).
type CircuitStatus string

const (
	CircuitClosed   CircuitStatus = "closed"
	CircuitOpen     CircuitStatus = "open"
	CircuitHalfOpen CircuitStatus = "half-open"
)

// EndpointSnapshot is the wire-format observability view of one
// NATS endpoint. Rendered into /api/status's nats_endpoints array.
//
// LastConnected / LastDisconnect are pointers because time.Time's
// zero value isn't omitted by `omitempty` (it's a struct, not nil)
// — pointer-with-omitempty is the standard Go idiom for "optional
// timestamp."
//
// Latency fields are surfaced as integer milliseconds (rather than
// time.Duration's stringly form) so dashboards can plot them
// without parsing.
type EndpointSnapshot struct {
	URL            string         `json:"url"`
	Status         EndpointStatus `json:"status"`
	Circuit        CircuitStatus  `json:"circuit"`
	LastConnected  *time.Time     `json:"last_connected,omitempty"`
	LastDisconnect *time.Time     `json:"last_disconnect,omitempty"`
	LastError      string         `json:"last_error,omitempty"`
	FailureCount   int64          `json:"failure_count"`
	SuccessCount   int64          `json:"success_count"`
	LatencyP50Ms   int64          `json:"latency_p50_ms"`
	LatencyP99Ms   int64          `json:"latency_p99_ms"`
}
