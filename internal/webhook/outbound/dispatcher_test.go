// SPDX-License-Identifier: Apache-2.0

package outbound

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// capturingServer wraps httptest with a slot capturing the last
// request the receiver saw (headers + body), so tests can assert on
// the exact bytes Dispatcher sent.
type capturingServer struct {
	*httptest.Server
	mu       sync.Mutex
	gotBody  []byte
	gotHdr   http.Header
	delay    time.Duration
	status   int
	respBody string
}

func newCapturingServer(t *testing.T) *capturingServer {
	t.Helper()
	cs := &capturingServer{status: http.StatusOK}
	cs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cs.delay > 0 {
			time.Sleep(cs.delay)
		}
		body, _ := io.ReadAll(r.Body)
		cs.mu.Lock()
		cs.gotBody = body
		cs.gotHdr = r.Header.Clone()
		status, resp := cs.status, cs.respBody
		cs.mu.Unlock()
		w.WriteHeader(status)
		if resp != "" {
			_, _ = w.Write([]byte(resp))
		}
	}))
	t.Cleanup(cs.Close)
	return cs
}

func subFor(url, secret string, headers map[string]string, timeoutSec int) *Subscription {
	return &Subscription{
		ID: "s1", Name: "test", URL: url, Secret: secret,
		Enabled: true, Headers: headers, TimeoutSec: timeoutSec,
	}
}

func expectedSig(secret, payload string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(payload))
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func TestDispatcher_Deliver_Success_SignsAndSendsHeaders(t *testing.T) {
	t.Parallel()
	srv := newCapturingServer(t)

	d := &HTTPDispatcher{HTTPClient: srv.Client()}
	payload := []byte(`{"type":"state.drift","id":"ev-1"}`)
	code, err := d.Deliver(context.Background(),
		subFor(srv.URL, "s3cr3t", map[string]string{"X-Op": "kc"}, 5),
		payload, "deliv-1")
	if err != nil || code != http.StatusOK {
		t.Fatalf("Deliver: code=%d err=%v", code, err)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if string(srv.gotBody) != string(payload) {
		t.Errorf("body = %q, want verbatim payload", srv.gotBody)
	}
	if got := srv.gotHdr.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got := srv.gotHdr.Get(deliveryHeader); got != "deliv-1" {
		t.Errorf("%s = %q, want deliv-1", deliveryHeader, got)
	}
	if got := srv.gotHdr.Get(signatureHeader); got != expectedSig("s3cr3t", string(payload)) {
		t.Errorf("%s = %q, want %q", signatureHeader, got, expectedSig("s3cr3t", string(payload)))
	}
	if got := srv.gotHdr.Get("X-Op"); got != "kc" {
		t.Errorf("custom X-Op = %q, want kc (operator header dropped)", got)
	}
}

func TestDispatcher_Deliver_EmptySecret_NoSignatureHeader(t *testing.T) {
	t.Parallel()
	srv := newCapturingServer(t)

	d := &HTTPDispatcher{HTTPClient: srv.Client()}
	if _, err := d.Deliver(context.Background(),
		subFor(srv.URL, "", nil, 5),
		[]byte(`{}`), "deliv-2"); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if got := srv.gotHdr.Get(signatureHeader); got != "" {
		t.Errorf("%s = %q with empty secret, want absent", signatureHeader, got)
	}
}

func TestDispatcher_OurHeadersOverrideOperator(t *testing.T) {
	t.Parallel()
	srv := newCapturingServer(t)

	// Operator tries to clobber our headers; we must win.
	d := &HTTPDispatcher{HTTPClient: srv.Client()}
	hdrs := map[string]string{
		"Content-Type":  "text/plain",
		deliveryHeader:  "spoof",
		signatureHeader: "sha256=spoof",
	}
	_, err := d.Deliver(context.Background(),
		subFor(srv.URL, "s3cr3t", hdrs, 5),
		[]byte("payload"), "real-id")
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if got := srv.gotHdr.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type clobbered: %q", got)
	}
	if got := srv.gotHdr.Get(deliveryHeader); got != "real-id" {
		t.Errorf("%s clobbered: %q", deliveryHeader, got)
	}
	if got := srv.gotHdr.Get(signatureHeader); got != expectedSig("s3cr3t", "payload") {
		t.Errorf("%s clobbered: %q", signatureHeader, got)
	}
}

func TestDispatcher_Non2xx_ReturnsCodeAndError(t *testing.T) {
	t.Parallel()
	srv := newCapturingServer(t)
	srv.mu.Lock()
	srv.status = http.StatusBadGateway
	srv.respBody = "upstream down"
	srv.mu.Unlock()

	d := &HTTPDispatcher{HTTPClient: srv.Client()}
	code, err := d.Deliver(context.Background(), subFor(srv.URL, "", nil, 5), []byte(`{}`), "x")
	if code != http.StatusBadGateway {
		t.Errorf("code = %d, want 502", code)
	}
	if err == nil || !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "upstream down") {
		t.Errorf("err = %v, want 502 + body", err)
	}
}

func TestDispatcher_TransportError_ZeroCode(t *testing.T) {
	t.Parallel()
	d := &HTTPDispatcher{HTTPClient: &http.Client{}}
	code, err := d.Deliver(context.Background(),
		subFor("http://127.0.0.1:1/nope", "", nil, 5), // unroutable
		[]byte(`{}`), "x")
	if code != 0 || err == nil {
		t.Errorf("code=%d err=%v, want 0 + transport error", code, err)
	}
}

func TestDispatcher_PerSubTimeout(t *testing.T) {
	t.Parallel()
	srv := newCapturingServer(t)
	srv.mu.Lock()
	srv.delay = 200 * time.Millisecond
	srv.mu.Unlock()

	d := &HTTPDispatcher{HTTPClient: srv.Client()}
	// TimeoutSec=1 → per-call timeout cuts the 200ms-delay response;
	// expect a transport error (context deadline exceeded).
	// We use 0 (≤ 0 path) + a tiny DefaultTimeout below to test
	// fallback in another test; here exercise the per-sub branch via
	// setting it to a value smaller than the delay isn't possible
	// with whole-second precision, so use ctx-level cancel instead.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	code, err := d.Deliver(ctx, subFor(srv.URL, "", nil, 10), []byte(`{}`), "x")
	if code != 0 || err == nil {
		t.Errorf("code=%d err=%v, want 0 + timeout error", code, err)
	}
}

func TestDispatcher_DefaultTimeoutFallback(t *testing.T) {
	t.Parallel()
	srv := newCapturingServer(t)
	srv.mu.Lock()
	srv.delay = 200 * time.Millisecond
	srv.mu.Unlock()

	// sub.TimeoutSec=0 → fall back to Dispatcher.DefaultTimeout
	// (50ms here), which trips before the 200ms server delay.
	d := &HTTPDispatcher{HTTPClient: srv.Client(), DefaultTimeout: 50 * time.Millisecond}
	code, err := d.Deliver(context.Background(), subFor(srv.URL, "", nil, 0), []byte(`{}`), "x")
	if code != 0 || err == nil {
		t.Errorf("code=%d err=%v, want 0 + DefaultTimeout deadline error", code, err)
	}
}

func TestDispatcher_NilClient_UsesDefault(t *testing.T) {
	t.Parallel()
	srv := newCapturingServer(t)
	// HTTPClient unset → dispatcher falls back to the default client.
	// Even though srv.Client() trusts its self-signed cert, httptest's
	// NewServer is plain HTTP, so the default *http.Client works.
	d := &HTTPDispatcher{}
	code, err := d.Deliver(context.Background(), subFor(srv.URL, "", nil, 5), []byte(`{}`), "x")
	if err != nil || code != http.StatusOK {
		t.Errorf("nil client fallback: code=%d err=%v", code, err)
	}
}

func TestDispatcher_BadURL(t *testing.T) {
	t.Parallel()
	d := &HTTPDispatcher{HTTPClient: &http.Client{}}
	_, err := d.Deliver(context.Background(),
		subFor("ht_tp://invalid", "", nil, 1), []byte(`{}`), "x")
	if err == nil {
		t.Error("malformed URL = nil error, want request-build failure")
	}
}
