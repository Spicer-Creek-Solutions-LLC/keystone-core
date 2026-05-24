// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryableStatus(t *testing.T) {
	for _, code := range []int{429, 502, 503, 504} {
		if !retryableStatus(code) {
			t.Errorf("status %d should be retryable", code)
		}
	}
	for _, code := range []int{200, 201, 400, 401, 403, 404, 409, 422, 500, 501} {
		if retryableStatus(code) {
			t.Errorf("status %d should not be retryable", code)
		}
	}
}

func TestRetryWaitHonoursRetryAfterSeconds(t *testing.T) {
	resp := &http.Response{Header: http.Header{"Retry-After": {"2"}}}
	if got := retryWait(resp, 0); got != 2*time.Second {
		t.Errorf("retryWait with Retry-After: 2 = %v, want 2s", got)
	}
	resp = &http.Response{Header: http.Header{"Retry-After": {"99999"}}}
	if got := retryWait(resp, 0); got != maxRetryWait {
		t.Errorf("retryWait should cap at %v, got %v", maxRetryWait, got)
	}
}

func TestRetryWaitBackoffWhenNoHeader(t *testing.T) {
	// No Retry-After: exponential backoff with jitter, never exceeding the cap,
	// and growing with the attempt number.
	w0 := retryWait(nil, 0)
	w3 := retryWait(nil, 3)
	if w0 <= 0 || w0 > maxRetryWait {
		t.Errorf("backoff(0) = %v out of range", w0)
	}
	if w3 < 4*time.Second {
		t.Errorf("backoff(3) = %v, expected >= 4s base", w3)
	}
}

// newTestClient points a client at a test server with retry sleeps effectively
// disabled (the server sends Retry-After: 0).
func newTestClient(t *testing.T, h http.HandlerFunc) *client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return newClient(srv.URL, "owner/repo", "test-token", 0)
}

func TestDoRetriesOn429ThenSucceeds(t *testing.T) {
	var calls int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) <= 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.do(http.MethodGet, "/issues", nil, &out); err != nil {
		t.Fatalf("do: %v", err)
	}
	if !out.OK {
		t.Error("response not decoded")
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("expected 3 calls (2 rate-limited + 1 ok), got %d", got)
	}
}

func TestDoGivesUpAfterMaxAttempts(t *testing.T) {
	var calls int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	if err := c.do(http.MethodPost, "/issues", map[string]string{"title": "x"}, nil); err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	if got := atomic.LoadInt32(&calls); got != maxAttempts {
		t.Errorf("expected %d attempts, got %d", maxAttempts, got)
	}
}

func TestDoNonRetryableStatusFailsImmediately(t *testing.T) {
	var calls int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, `{"message":"nope"}`, http.StatusUnprocessableEntity)
	})
	if err := c.do(http.MethodPost, "/issues", map[string]string{"title": "x"}, nil); err == nil {
		t.Fatal("expected an error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("422 should not be retried; got %d calls", got)
	}
}

func TestDoThrottleAppliesToMutationsOnly(t *testing.T) {
	var lastMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := newClient(srv.URL, "owner/repo", "tok", 40*time.Millisecond)

	start := time.Now()
	_ = c.do(http.MethodGet, "/x", nil, nil)
	if d := time.Since(start); d >= 40*time.Millisecond {
		t.Errorf("GET should not be throttled, took %v", d)
	}
	start = time.Now()
	_ = c.do(http.MethodPost, "/x", map[string]string{"a": "b"}, nil)
	if d := time.Since(start); d < 40*time.Millisecond {
		t.Errorf("POST should be throttled by ~40ms, took %v", d)
	}
	if lastMethod != http.MethodPost {
		t.Errorf("unexpected last method %q", lastMethod)
	}
}
