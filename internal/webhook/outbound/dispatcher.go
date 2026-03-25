package outbound

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// ErrCircuitOpen is returned when a delivery is rejected because the
// endpoint's circuit breaker is in the open state.
var ErrCircuitOpen = fmt.Errorf("circuit breaker open: endpoint unavailable")

// circuitState tracks per-endpoint failure/success counts.
type circuitState struct {
	failures    int
	successes   int
	state       string // "closed", "open", "half-open"
	openUntil   time.Time
	mu          sync.Mutex
}

// DispatcherConfig configures circuit breaker thresholds for the Dispatcher.
type DispatcherConfig struct {
	// FailureThreshold is the number of consecutive failures before opening
	// the circuit. Default: 5.
	FailureThreshold int
	// SuccessThreshold is the number of consecutive successes in half-open
	// state before closing the circuit. Default: 2.
	SuccessThreshold int
	// OpenDuration is how long the circuit stays open before transitioning
	// to half-open. Default: 30s.
	OpenDuration time.Duration
}

// Dispatcher delivers webhook payloads to subscription URLs.
type Dispatcher struct {
	client   *http.Client
	circuits sync.Map // url-host → *circuitState
	cbConfig DispatcherConfig
}

// NewDispatcher creates a dispatcher with the given HTTP timeout.
func NewDispatcher(timeout time.Duration) *Dispatcher {
	return NewDispatcherWithConfig(timeout, DispatcherConfig{})
}

// NewDispatcherWithConfig creates a dispatcher with explicit circuit breaker config.
func NewDispatcherWithConfig(timeout time.Duration, cfg DispatcherConfig) *Dispatcher {
	if timeout <= 0 {
		timeout = 10 * time.Second
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
	return &Dispatcher{
		client:   &http.Client{Timeout: timeout},
		cbConfig: cfg,
	}
}

// endpointKey extracts a circuit breaker key from a subscription URL.
func endpointKey(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Host
}

// getCircuit returns (or creates) the circuit state for an endpoint.
func (d *Dispatcher) getCircuit(key string) *circuitState {
	if v, ok := d.circuits.Load(key); ok {
		return v.(*circuitState)
	}
	cs := &circuitState{state: "closed"}
	actual, _ := d.circuits.LoadOrStore(key, cs)
	return actual.(*circuitState)
}

// Deliver sends payload to the subscription's URL with appropriate headers.
// Returns the HTTP status code and any error.
func (d *Dispatcher) Deliver(ctx context.Context, sub *Subscription, payload []byte, deliveryID string) (int, error) {
	key := endpointKey(sub.URL)
	cs := d.getCircuit(key)

	// Check circuit state
	cs.mu.Lock()
	switch cs.state {
	case "open":
		if time.Now().Before(cs.openUntil) {
			cs.mu.Unlock()
			return 0, ErrCircuitOpen
		}
		// Transition to half-open
		cs.state = "half-open"
		cs.successes = 0
	}
	cs.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.URL, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(WebhookIDHeader, deliveryID)
	req.Header.Set(WebhookTimestampHeader, FormatTimestamp(time.Now().Unix()))

	if sub.Secret != "" {
		sig := Sign([]byte(sub.Secret), payload)
		req.Header.Set(SignatureHeader, sig)
	}

	for k, v := range sub.Headers {
		req.Header.Set(k, v)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		d.recordFailure(cs)
		return 0, fmt.Errorf("deliver webhook: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		d.recordSuccess(cs)
		return resp.StatusCode, nil
	}

	if resp.StatusCode >= 500 {
		d.recordFailure(cs)
	}
	return resp.StatusCode, fmt.Errorf("endpoint returned HTTP %d", resp.StatusCode)
}

func (d *Dispatcher) recordFailure(cs *circuitState) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.failures++
	cs.successes = 0
	if cs.failures >= d.cbConfig.FailureThreshold {
		cs.state = "open"
		cs.openUntil = time.Now().Add(d.cbConfig.OpenDuration)
	}
}

func (d *Dispatcher) recordSuccess(cs *circuitState) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.successes++
	cs.failures = 0
	if cs.state == "half-open" && cs.successes >= d.cbConfig.SuccessThreshold {
		cs.state = "closed"
	}
}
