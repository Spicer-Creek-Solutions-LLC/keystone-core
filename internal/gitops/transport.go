package gitops

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ErrCircuitOpen is returned when the circuit breaker rejects a request.
var ErrCircuitOpen = fmt.Errorf("circuit breaker open: provider unavailable")

// CircuitBreakerTransport wraps an http.RoundTripper with a simple
// three-state circuit breaker (closed → open → half-open → closed).
// Only 5xx responses and transport errors trip the breaker.
type CircuitBreakerTransport struct {
	next             http.RoundTripper
	failureThreshold int
	successThreshold int
	openDuration     time.Duration

	mu        sync.Mutex
	failures  int
	successes int
	state     string // "closed", "open", "half-open"
	openUntil time.Time
}

// CircuitBreakerConfig configures the transport circuit breaker.
type CircuitBreakerConfig struct {
	FailureThreshold int
	SuccessThreshold int
	OpenDuration     time.Duration
}

// NewCircuitBreakerTransport wraps the given transport with a circuit breaker.
func NewCircuitBreakerTransport(next http.RoundTripper, cfg CircuitBreakerConfig) *CircuitBreakerTransport {
	if next == nil {
		next = http.DefaultTransport
	}
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.SuccessThreshold <= 0 {
		cfg.SuccessThreshold = 2
	}
	if cfg.OpenDuration <= 0 {
		cfg.OpenDuration = 30 * time.Second
	}
	return &CircuitBreakerTransport{
		next:             next,
		failureThreshold: cfg.FailureThreshold,
		successThreshold: cfg.SuccessThreshold,
		openDuration:     cfg.OpenDuration,
		state:            "closed",
	}
}

// RoundTrip implements http.RoundTripper.
func (t *CircuitBreakerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	switch t.state {
	case "open":
		if time.Now().Before(t.openUntil) {
			t.mu.Unlock()
			return nil, ErrCircuitOpen
		}
		t.state = "half-open"
		t.successes = 0
	}
	t.mu.Unlock()

	resp, err := t.next.RoundTrip(req)
	if err != nil {
		t.recordFailure()
		return nil, err
	}

	if resp.StatusCode >= 500 {
		t.recordFailure()
	} else {
		t.recordSuccess()
	}

	return resp, nil
}

func (t *CircuitBreakerTransport) recordFailure() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failures++
	t.successes = 0
	if t.failures >= t.failureThreshold {
		t.state = "open"
		t.openUntil = time.Now().Add(t.openDuration)
	}
}

func (t *CircuitBreakerTransport) recordSuccess() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.successes++
	t.failures = 0
	if t.state == "half-open" && t.successes >= t.successThreshold {
		t.state = "closed"
	}
}
