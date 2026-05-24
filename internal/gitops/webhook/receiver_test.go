// SPDX-License-Identifier: Apache-2.0

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

// testClient avoids the shared http.DefaultClient connection pool:
// under the whole-suite (`go test ./...`) parallel load, a pooled
// connection could stall a request long enough to hit the receiver's
// 5s ReadHeaderTimeout and flake. Keep-alives off + explicit timeout
// isolates each call.
var testClient = &http.Client{
	Timeout:   10 * time.Second,
	Transport: &http.Transport{DisableKeepAlives: true},
}

// post POSTs body with the given headers and returns the status code,
// closing the response body before returning (callers assert on status).
func post(t *testing.T, url string, headers map[string]string, body string) int {
	t.Helper()
	httpReq, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
	resp, err := testClient.Do(httpReq)
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
	got := post(t, "http://"+r.Addr()+"/webhooks",
		map[string]string{HeaderArgoCD: "true"},
		`{"app":{"metadata":{"name":"web"},"status":{"sync":{"status":"Synced"}}}}`)
	if got != http.StatusAccepted {
		t.Errorf("status = %d, want 202", got)
	}
}

func TestReceiver_HeaderDetection(t *testing.T) {
	t.Parallel()
	r := startReceiver(t, NewDefaultRegistry(), 0)
	cases := []struct {
		name    string
		headers map[string]string
		body    string
		want    int
	}{
		{
			name:    "no provider header",
			headers: nil,
			body:    `{}`,
			want:    http.StatusBadRequest,
		},
		{
			name:    "ambiguous: two provider headers",
			headers: map[string]string{HeaderArgoCD: "true", HeaderGitHub: "push"},
			body:    `{}`,
			want:    http.StatusBadRequest,
		},
		{
			name:    "detected but parse fails",
			headers: map[string]string{HeaderArgoCD: "true"},
			body:    `{"app":{}}`,
			want:    http.StatusUnprocessableEntity,
		},
		{
			name:    "detected but malformed json",
			headers: map[string]string{HeaderFlux: "Kustomization"},
			body:    `{`,
			want:    http.StatusUnprocessableEntity,
		},
		{
			name:    "github detected via header",
			headers: map[string]string{HeaderGitHub: "push"},
			body:    `{"repository":{"full_name":"acme/web"},"after":"abc"}`,
			want:    http.StatusAccepted,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := post(t, "http://"+r.Addr()+"/webhooks", tc.headers, tc.body)
			if got != tc.want {
				t.Errorf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestReceiver_Authentication(t *testing.T) {
	t.Parallel()
	const secret = "topsecret"
	body := `{"app":{"metadata":{"name":"web"},"status":{"sync":{"status":"Synced"}}}}`
	auths, err := BuildAuthenticators(map[Provider]AuthSpec{
		ProviderArgoCD: {Method: AuthHMAC, Secret: secret},
	})
	if err != nil {
		t.Fatal(err)
	}
	r := New(ReceiverConfig{
		Addr: "127.0.0.1:0", Path: "/webhooks", Authenticators: auths,
	}, NewDefaultRegistry(), nil)
	if startErr := r.Start(context.Background()); startErr != nil {
		t.Fatalf("Start: %v", startErr)
	}
	t.Cleanup(func() { _ = r.Stop(context.Background()) })
	url := "http://" + r.Addr() + "/webhooks"

	t.Run("valid signature accepted", func(t *testing.T) {
		t.Parallel()
		got := post(t, url, map[string]string{
			HeaderArgoCD:  "true",
			"X-Signature": "sha256=" + signHMAC(secret, body),
		}, body)
		if got != http.StatusAccepted {
			t.Errorf("status = %d, want 202", got)
		}
	})
	t.Run("bad signature rejected", func(t *testing.T) {
		t.Parallel()
		got := post(t, url, map[string]string{
			HeaderArgoCD:  "true",
			"X-Signature": "sha256=deadbeef",
		}, body)
		if got != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", got)
		}
	})
	t.Run("missing signature rejected", func(t *testing.T) {
		t.Parallel()
		got := post(t, url, map[string]string{HeaderArgoCD: "true"}, body)
		if got != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", got)
		}
	})
	t.Run("unconfigured provider defaults to None (accepted)", func(t *testing.T) {
		t.Parallel()
		// github has no authenticator → NoneAuthenticator → 202.
		got := post(t, url, map[string]string{HeaderGitHub: "push"},
			`{"repository":{"full_name":"acme/web"},"after":"abc"}`)
		if got != http.StatusAccepted {
			t.Errorf("status = %d, want 202", got)
		}
	})
}

func TestReceiver_EmitsEvent(t *testing.T) {
	t.Parallel()
	fe := &fakeEmitter{}
	r := New(ReceiverConfig{
		Addr: "127.0.0.1:0", Path: "/webhooks", Emitter: fe, EventSource: "node-x",
	}, NewDefaultRegistry(), nil)
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = r.Stop(context.Background()) })

	got := post(t, "http://"+r.Addr()+"/webhooks",
		map[string]string{HeaderArgoCD: "true"},
		`{"app":{"metadata":{"name":"web","namespace":"prod"},"status":{"sync":{"status":"Synced","revision":"abc"}}}}`)
	if got != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", got)
	}
	// Emission is synchronous within the request, so it is observable
	// once the response returns.
	evs := fe.events()
	if len(evs) != 1 {
		t.Fatalf("emitted %d events, want 1", len(evs))
	}
	e := evs[0]
	if e.Type != "gitops.argocd.sync_succeeded" {
		t.Errorf("Type = %q, want gitops.argocd.sync_succeeded (acceptance 102)", e.Type)
	}
	if e.Source != "node-x" || e.Tags["application"] != "web" || e.Tags["revision"] != "abc" {
		t.Errorf("unexpected event fields: source=%q tags=%v", e.Source, e.Tags)
	}
}

func TestReceiver_EmitFailureStill202(t *testing.T) {
	t.Parallel()
	fe := &fakeEmitter{err: errTestPublish}
	r := New(ReceiverConfig{Addr: "127.0.0.1:0", Path: "/webhooks", Emitter: fe},
		NewDefaultRegistry(), nil)
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = r.Stop(context.Background()) })
	got := post(t, "http://"+r.Addr()+"/webhooks",
		map[string]string{HeaderGitHub: "push"},
		`{"repository":{"full_name":"acme/web"},"after":"abc"}`)
	if got != http.StatusAccepted {
		t.Errorf("status = %d, want 202 (publish error is best-effort)", got)
	}
}

func TestReceiver_NilEmitterStill202(t *testing.T) {
	t.Parallel()
	r := startReceiver(t, NewDefaultRegistry(), 0) // no Emitter
	got := post(t, "http://"+r.Addr()+"/webhooks",
		map[string]string{HeaderGitHub: "push"},
		`{"repository":{"full_name":"acme/web"},"after":"abc"}`)
	if got != http.StatusAccepted {
		t.Errorf("status = %d, want 202", got)
	}
}

func TestReceiver_UnregisteredProviderHeader(t *testing.T) {
	t.Parallel()
	// Registry holds only the ArgoCD handler; Detect scans only
	// registered handlers' headers, so a GitHub header resolves to no
	// provider → 400 (ErrNoProvider).
	reg := NewRegistry()
	if err := reg.Register(ArgoCDHandler{}); err != nil {
		t.Fatal(err)
	}
	r := startReceiver(t, reg, 0)
	got := post(t, "http://"+r.Addr()+"/webhooks",
		map[string]string{HeaderGitHub: "push"}, `{}`)
	if got != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", got)
	}
}

func TestReceiver_BodyTooLarge(t *testing.T) {
	t.Parallel()
	r := startReceiver(t, NewDefaultRegistry(), 16)
	got := post(t, "http://"+r.Addr()+"/webhooks",
		map[string]string{HeaderArgoCD: "true"},
		`{"app":{"metadata":{"name":"this-body-is-well-over-sixteen-bytes"}}}`)
	if got != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", got)
	}
}

func TestReceiver_GetIsNotRouted(t *testing.T) {
	t.Parallel()
	r := startReceiver(t, NewDefaultRegistry(), 0)
	resp, err := testClient.Get("http://" + r.Addr() + "/webhooks")
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
