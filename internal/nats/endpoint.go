package nats

import (
	"sort"
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/pkg/natsstatus"
)

// Public type aliases (Task 11). EndpointSnapshot, EndpointStatus,
// and CircuitStatus moved to pkg/natsstatus so /api/status and
// future SDK consumers can render them without crossing
// internal-package boundaries. Aliases keep the internal callers
// (ConnectionManager, breaker) writing the short name.
type (
	EndpointStatus   = natsstatus.EndpointStatus
	CircuitStatus    = natsstatus.CircuitStatus
	EndpointSnapshot = natsstatus.EndpointSnapshot
)

// Re-export the enum constants so internal/nats consumers don't
// have to import natsstatus too.
const (
	EndpointStatusUnknown      = natsstatus.EndpointStatusUnknown
	EndpointStatusConnecting   = natsstatus.EndpointStatusConnecting
	EndpointStatusConnected    = natsstatus.EndpointStatusConnected
	EndpointStatusDisconnected = natsstatus.EndpointStatusDisconnected
	EndpointStatusFailed       = natsstatus.EndpointStatusFailed

	CircuitClosed   = natsstatus.CircuitClosed
	CircuitOpen     = natsstatus.CircuitOpen
	CircuitHalfOpen = natsstatus.CircuitHalfOpen
)

// Endpoint describes one NATS server reachable by the
// ConnectionManager. URL is canonical; Scheme is parsed from the URL
// for StrategySelector dispatch. Priority/Weight govern selection
// order — higher Priority is preferred; Weight tunes the post-v1.0
// load-distribution path (today only the highest-priority bucket is
// used). Tags are operator-supplied labels (region, role) that
// surface in Snapshot for observability and are reserved for v2.x+
// supercluster routing.
type Endpoint struct {
	URL      string
	Scheme   string
	Auth     string
	Priority int
	Weight   int
	Tags     []string
}

// latencyWindow is the ring-buffer capacity used for per-endpoint
// percentile tracking. 64 samples is large enough for stable P50/P99
// at the v1.0 ping cadence (5s) while keeping the per-state memory
// footprint bounded (~512 B per endpoint).
const latencyWindow = 64

// EndpointState is the live observability snapshot for one Endpoint.
// All fields are guarded by mu so Snapshot() can return a consistent
// view without exposing the lock to callers. Construction goes
// through newEndpointState — zero values are not considered valid.
type EndpointState struct {
	URL string

	mu             sync.RWMutex
	status         EndpointStatus
	circuit        CircuitStatus
	lastConnected  time.Time
	lastDisconnect time.Time
	lastError      string
	failureCount   int64
	successCount   int64
	rtts           []time.Duration
	rttCursor      int
	rttCount       int
}


func newEndpointState(url string) *EndpointState {
	return &EndpointState{
		URL:     url,
		status:  EndpointStatusUnknown,
		circuit: CircuitClosed,
		rtts:    make([]time.Duration, latencyWindow),
	}
}

// SetStatus records a transition. Connected timestamps last-connected
// time; disconnected/failed timestamps last-disconnect time.
func (s *EndpointState) SetStatus(status EndpointStatus, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
	switch status {
	case EndpointStatusConnected:
		s.lastConnected = now
	case EndpointStatusDisconnected, EndpointStatusFailed:
		s.lastDisconnect = now
	}
}

// SetCircuit records the breaker state for this endpoint.
func (s *EndpointState) SetCircuit(c CircuitStatus) {
	s.mu.Lock()
	s.circuit = c
	s.mu.Unlock()
}

// RecordSuccess increments the success counter. Used by Publish on
// no-error returns and by the periodic RTT probe.
func (s *EndpointState) RecordSuccess() {
	s.mu.Lock()
	s.successCount++
	s.mu.Unlock()
}

// RecordFailure increments the failure counter and stashes the error
// string for diagnostics. The breaker (Task 7) reads failureCount via
// Snapshot to advance state.
func (s *EndpointState) RecordFailure(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failureCount++
	if err != nil {
		s.lastError = err.Error()
	}
}

// RecordRTT pushes a sample into the ring buffer. Called by the
// ConnectionManager ping loop.
func (s *EndpointState) RecordRTT(d time.Duration) {
	if d < 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rtts[s.rttCursor] = d
	s.rttCursor = (s.rttCursor + 1) % len(s.rtts)
	if s.rttCount < len(s.rtts) {
		s.rttCount++
	}
}

// Snapshot returns a stable point-in-time view. Percentile computation
// happens here (not on every RecordRTT) to keep the hot path cheap.
//
// LastConnected / LastDisconnect are pointer-or-nil so the JSON
// renderer can omit unset timestamps cleanly.
func (s *EndpointState) Snapshot() EndpointSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p50, p99 := percentilesLocked(s.rtts, s.rttCount)
	snap := EndpointSnapshot{
		URL:          s.URL,
		Status:       s.status,
		Circuit:      s.circuit,
		LastError:    s.lastError,
		FailureCount: s.failureCount,
		SuccessCount: s.successCount,
		LatencyP50Ms: p50.Milliseconds(),
		LatencyP99Ms: p99.Milliseconds(),
	}
	if !s.lastConnected.IsZero() {
		t := s.lastConnected
		snap.LastConnected = &t
	}
	if !s.lastDisconnect.IsZero() {
		t := s.lastDisconnect
		snap.LastDisconnect = &t
	}
	return snap
}

// percentilesLocked returns approximate P50/P99 of the populated
// portion of the ring buffer. Not exact under concurrent writes —
// callers hold the read lock for snapshot consistency.
func percentilesLocked(samples []time.Duration, count int) (p50, p99 time.Duration) {
	if count == 0 {
		return 0, 0
	}
	live := make([]time.Duration, count)
	copy(live, samples[:count])
	sort.Slice(live, func(i, j int) bool { return live[i] < live[j] })
	idx50 := count * 50 / 100
	idx99 := count * 99 / 100
	if idx50 >= count {
		idx50 = count - 1
	}
	if idx99 >= count {
		idx99 = count - 1
	}
	return live[idx50], live[idx99]
}

// sortEndpointsByPriority returns a new slice sorted highest-priority
// first; ties broken by URL for determinism. The selector iterates
// this order when picking a target.
func sortEndpointsByPriority(eps []Endpoint) []Endpoint {
	out := append([]Endpoint(nil), eps...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].URL < out[j].URL
	})
	return out
}
