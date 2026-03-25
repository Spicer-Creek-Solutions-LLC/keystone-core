package gitops

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCircuitBreakerTransport_OpensAfterFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	transport := NewCircuitBreakerTransport(http.DefaultTransport, CircuitBreakerConfig{
		FailureThreshold: 3,
		OpenDuration:     1 * time.Second,
	})
	client := &http.Client{Transport: transport}

	// 3 failures trip the circuit
	for i := 0; i < 3; i++ {
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("attempt %d: unexpected transport error: %v", i, err)
		}
		resp.Body.Close()
	}

	// 4th call should be rejected
	_, err := client.Get(srv.URL)
	if err == nil {
		t.Fatal("expected circuit open error")
	}
}

func TestCircuitBreakerTransport_ReclosesAfterSuccess(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	transport := NewCircuitBreakerTransport(http.DefaultTransport, CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 1,
		OpenDuration:     50 * time.Millisecond,
	})
	client := &http.Client{Transport: transport}

	// Trip the circuit
	for i := 0; i < 3; i++ {
		resp, _ := client.Get(srv.URL)
		if resp != nil {
			resp.Body.Close()
		}
	}

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// Probe should succeed and reclose
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("half-open probe failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Next call should work (circuit closed)
	resp, err = client.Get(srv.URL)
	if err != nil {
		t.Fatalf("closed circuit call failed: %v", err)
	}
	resp.Body.Close()
}

func TestCircuitBreakerTransport_4xxDoesNotTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	transport := NewCircuitBreakerTransport(http.DefaultTransport, CircuitBreakerConfig{
		FailureThreshold: 2,
		OpenDuration:     1 * time.Second,
	})
	client := &http.Client{Transport: transport}

	for i := 0; i < 5; i++ {
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("attempt %d: 4xx should not cause transport error: %v", i, err)
		}
		resp.Body.Close()
	}
}
