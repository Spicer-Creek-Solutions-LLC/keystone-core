package natsstatus

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEndpointSnapshot_JSONRoundTrip(t *testing.T) {
	connected := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	disconnected := time.Date(2026, 5, 8, 11, 59, 0, 0, time.UTC)
	original := EndpointSnapshot{
		URL:            "nats://node-a:4222",
		Status:         EndpointStatusConnected,
		Circuit:        CircuitHalfOpen,
		LastConnected:  &connected,
		LastDisconnect: &disconnected,
		LastError:      "EOF",
		FailureCount:   3,
		SuccessCount:   42,
		LatencyP50Ms:   2,
		LatencyP99Ms:   17,
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got EndpointSnapshot
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.URL != original.URL || got.Status != original.Status || got.Circuit != original.Circuit {
		t.Errorf("string fields drifted: got %+v", got)
	}
	if got.FailureCount != 3 || got.SuccessCount != 42 {
		t.Errorf("counters drifted: %+v", got)
	}
	if got.LatencyP50Ms != 2 || got.LatencyP99Ms != 17 {
		t.Errorf("latency drifted: %+v", got)
	}
}

func TestEndpointSnapshot_OmitsZeroOptional(t *testing.T) {
	zero := EndpointSnapshot{
		URL:     "nats://x:4222",
		Status:  EndpointStatusConnected,
		Circuit: CircuitClosed,
	}
	b, err := json.Marshal(zero)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(b)
	for _, absent := range []string{`"last_connected"`, `"last_disconnect"`, `"last_error"`} {
		if strings.Contains(got, absent) {
			t.Errorf("%s leaked into wire: %s", absent, got)
		}
	}
	// Counters and latencies always render — operators want explicit
	// zeros in the dashboard.
	for _, present := range []string{`"failure_count":0`, `"success_count":0`, `"latency_p50_ms":0`, `"latency_p99_ms":0`} {
		if !strings.Contains(got, present) {
			t.Errorf("expected %s in payload: %s", present, got)
		}
	}
}

func TestEnumValues(t *testing.T) {
	for _, s := range []EndpointStatus{
		EndpointStatusUnknown, EndpointStatusConnecting, EndpointStatusConnected,
		EndpointStatusDisconnected, EndpointStatusFailed,
	} {
		if s == "" {
			t.Error("EndpointStatus should have a non-empty value")
		}
	}
	for _, c := range []CircuitStatus{CircuitClosed, CircuitOpen, CircuitHalfOpen} {
		if c == "" {
			t.Error("CircuitStatus should have a non-empty value")
		}
	}
}
