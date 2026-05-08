package nats

import (
	"errors"
	"testing"
	"time"
)

func TestEndpointState_StatusTransitions(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	s := newEndpointState("nats://a:4222")

	s.SetStatus(EndpointStatusConnecting, now)
	snap := s.Snapshot()
	if snap.Status != EndpointStatusConnecting {
		t.Errorf("Status = %q", snap.Status)
	}
	if snap.LastConnected != nil {
		t.Error("LastConnected should be nil before first connect")
	}

	connectAt := now.Add(time.Second)
	s.SetStatus(EndpointStatusConnected, connectAt)
	if snap = s.Snapshot(); snap.Status != EndpointStatusConnected {
		t.Errorf("Status = %q", snap.Status)
	}
	if snap.LastConnected == nil || !snap.LastConnected.Equal(connectAt) {
		t.Errorf("LastConnected = %v, want %v", snap.LastConnected, connectAt)
	}

	disconnectAt := connectAt.Add(time.Second)
	s.SetStatus(EndpointStatusDisconnected, disconnectAt)
	if snap = s.Snapshot(); snap.LastDisconnect == nil || !snap.LastDisconnect.Equal(disconnectAt) {
		t.Errorf("LastDisconnect = %v, want %v", snap.LastDisconnect, disconnectAt)
	}
	// LastConnected must NOT regress when a later disconnect lands.
	if snap.LastConnected == nil || !snap.LastConnected.Equal(connectAt) {
		t.Errorf("LastConnected after disconnect = %v, want %v", snap.LastConnected, connectAt)
	}
}

func TestEndpointState_FailuresAndSuccess(t *testing.T) {
	s := newEndpointState("nats://a:4222")
	s.RecordSuccess()
	s.RecordSuccess()
	s.RecordFailure(errors.New("conn refused"))
	snap := s.Snapshot()
	if snap.SuccessCount != 2 {
		t.Errorf("SuccessCount = %d, want 2", snap.SuccessCount)
	}
	if snap.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1", snap.FailureCount)
	}
	if snap.LastError != "conn refused" {
		t.Errorf("LastError = %q", snap.LastError)
	}

	// Nil error: failure still counts but LastError stays unchanged.
	s.RecordFailure(nil)
	snap = s.Snapshot()
	if snap.FailureCount != 2 {
		t.Errorf("FailureCount = %d, want 2", snap.FailureCount)
	}
	if snap.LastError != "conn refused" {
		t.Errorf("LastError changed under nil-error failure: %q", snap.LastError)
	}
}

func TestEndpointState_CircuitTransition(t *testing.T) {
	s := newEndpointState("nats://a:4222")
	if got := s.Snapshot().Circuit; got != CircuitClosed {
		t.Errorf("default Circuit = %q, want closed", got)
	}
	s.SetCircuit(CircuitOpen)
	if got := s.Snapshot().Circuit; got != CircuitOpen {
		t.Errorf("after SetCircuit(open) = %q", got)
	}
}

func TestEndpointState_RTTPercentiles(t *testing.T) {
	s := newEndpointState("nats://a:4222")
	for i := 1; i <= 100; i++ {
		s.RecordRTT(time.Duration(i) * time.Millisecond)
	}
	// Ring buffer is 64 wide, so the surviving samples are i=37..100.
	snap := s.Snapshot()
	if snap.LatencyP50Ms == 0 {
		t.Error("P50 = 0 with 100 samples loaded")
	}
	if snap.LatencyP99Ms < snap.LatencyP50Ms {
		t.Errorf("P99 (%dms) < P50 (%dms)", snap.LatencyP99Ms, snap.LatencyP50Ms)
	}
}

func TestEndpointState_RTTNegativeIgnored(t *testing.T) {
	s := newEndpointState("nats://a:4222")
	s.RecordRTT(-1)
	if s.Snapshot().LatencyP50Ms != 0 {
		t.Error("negative RTT was accepted")
	}
}

func TestEndpointState_RTTEmpty(t *testing.T) {
	s := newEndpointState("nats://a:4222")
	snap := s.Snapshot()
	if snap.LatencyP50Ms != 0 || snap.LatencyP99Ms != 0 {
		t.Errorf("zero-sample percentiles = (%dms, %dms), want (0, 0)", snap.LatencyP50Ms, snap.LatencyP99Ms)
	}
}

func TestSortEndpointsByPriority(t *testing.T) {
	in := []Endpoint{
		{URL: "nats://b", Priority: 5},
		{URL: "nats://a", Priority: 5},
		{URL: "nats://c", Priority: 10},
		{URL: "nats://d", Priority: 1},
	}
	out := sortEndpointsByPriority(in)
	want := []string{"nats://c", "nats://a", "nats://b", "nats://d"}
	for i, e := range out {
		if e.URL != want[i] {
			t.Errorf("[%d] = %q, want %q", i, e.URL, want[i])
		}
	}
	// Source slice must remain unchanged.
	if in[0].URL != "nats://b" {
		t.Error("input slice was mutated")
	}
}
