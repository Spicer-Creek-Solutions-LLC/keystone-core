package webhook

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func startReceiver(t *testing.T, reg *Registry, maxBody int64) *Receiver {
	t.Helper()
	r := New(ReceiverConfig{Addr: "127.0.0.1:0", Path: "/webhooks", MaxBodyBytes: maxBody}, reg, nil)
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := r.Stop(context.Background()); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})
	return r
}

// post POSTs body and returns the status code, closing the response
// body before returning (callers only assert on status).
func post(t *testing.T, url, body string) int {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func TestReceiver_AcceptsValidWebhook(t *testing.T) {
	t.Parallel()
	r := startReceiver(t, NewDefaultRegistry(), 0)
	url := "http://" + r.Addr() + "/webhooks?source=argocd"
	got := post(t, url, `{"app":{"metadata":{"name":"web"},"status":{"sync":{"status":"Synced"}}}}`)
	if got != http.StatusAccepted {
		t.Errorf("status = %d, want 202", got)
	}
}

func TestReceiver_ProviderResolution(t *testing.T) {
	t.Parallel()
	r := startReceiver(t, NewDefaultRegistry(), 0)
	cases := []struct {
		name   string
		query  string
		body   string
		want   int
	}{
		{"missing source", "", `{}`, http.StatusBadRequest},
		{"unknown source", "?source=bogus", `{}`, http.StatusBadRequest},
		{"parse failure", "?source=argocd", `{"app":{}}`, http.StatusUnprocessableEntity},
		{"malformed json", "?source=flux", `{`, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := post(t, "http://"+r.Addr()+"/webhooks"+tc.query, tc.body)
			if got != tc.want {
				t.Errorf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestReceiver_ValidProviderNoHandler(t *testing.T) {
	t.Parallel()
	// Registry without the github handler: resolveProvider accepts the
	// valid name but Lookup misses → 400.
	reg := NewRegistry()
	if err := reg.Register(ArgoCDHandler{}); err != nil {
		t.Fatal(err)
	}
	r := startReceiver(t, reg, 0)
	got := post(t, "http://"+r.Addr()+"/webhooks?source=github", `{}`)
	if got != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", got)
	}
}

func TestReceiver_BodyTooLarge(t *testing.T) {
	t.Parallel()
	r := startReceiver(t, NewDefaultRegistry(), 16)
	got := post(t, "http://"+r.Addr()+"/webhooks?source=argocd",
		`{"app":{"metadata":{"name":"this-body-is-well-over-sixteen-bytes"}}}`)
	if got != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", got)
	}
}

func TestReceiver_GetIsNotRouted(t *testing.T) {
	t.Parallel()
	r := startReceiver(t, NewDefaultRegistry(), 0)
	resp, err := http.Get("http://" + r.Addr() + "/webhooks?source=argocd")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, want 405 (POST-only route)", resp.StatusCode)
	}
}

func TestReceiver_StartStopIdempotent(t *testing.T) {
	t.Parallel()
	r := New(ReceiverConfig{Addr: "127.0.0.1:0", Path: "/webhooks"}, NewDefaultRegistry(), nil)
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start#1: %v", err)
	}
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start#2 (idempotent): %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := r.Stop(ctx); err != nil {
		t.Fatalf("Stop#1: %v", err)
	}
	if err := r.Stop(ctx); err != nil {
		t.Fatalf("Stop#2 (idempotent): %v", err)
	}
}

func TestReceiver_StopWithoutStart(t *testing.T) {
	t.Parallel()
	r := New(ReceiverConfig{Addr: "127.0.0.1:0"}, NewDefaultRegistry(), nil)
	if err := r.Stop(context.Background()); err != nil {
		t.Errorf("Stop without Start: %v", err)
	}
}

func TestReceiver_StartPortInUse(t *testing.T) {
	t.Parallel()
	r1 := startReceiver(t, NewDefaultRegistry(), 0)
	r2 := New(ReceiverConfig{Addr: r1.Addr(), Path: "/webhooks"}, NewDefaultRegistry(), nil)
	if err := r2.Start(context.Background()); err == nil {
		t.Error("Start on in-use port = nil, want bind error")
		_ = r2.Stop(context.Background())
	}
}

func TestReceiver_AddrBeforeStart(t *testing.T) {
	t.Parallel()
	r := New(ReceiverConfig{Addr: ":8081"}, NewDefaultRegistry(), nil)
	if got := r.Addr(); got != ":8081" {
		t.Errorf("Addr() before Start = %q, want \":8081\"", got)
	}
}
