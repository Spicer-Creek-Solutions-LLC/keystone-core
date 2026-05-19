package verification

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPVerifier(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("pong"))
		case "/teapot":
			w.WriteHeader(http.StatusTeapot)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close) // parent has parallel subtests; defer would close early

	v := HTTPVerifier{Client: srv.Client()}
	cases := []struct {
		name        string
		cfg         map[string]any
		wantSuccess bool
		wantErrCfg  bool
	}{
		{"2xx default ok", map[string]any{"url": srv.URL + "/ok"}, true, false},
		{"explicit expect_status", map[string]any{"url": srv.URL + "/ok", "expect_status": 200}, true, false},
		{"body contains", map[string]any{"url": srv.URL + "/ok", "expect_body_contains": "pon"}, true, false},
		{"body missing", map[string]any{"url": srv.URL + "/ok", "expect_body_contains": "zzz"}, false, false},
		{"status mismatch", map[string]any{"url": srv.URL + "/teapot", "expect_status": 200}, false, false},
		{"non-2xx default fails", map[string]any{"url": srv.URL + "/missing"}, false, false},
		{"missing url", map[string]any{}, false, true},
		{"bad expect_status type", map[string]any{"url": srv.URL, "expect_status": "200"}, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := v.Verify(context.Background(), Step{Type: "http", Config: c.cfg})
			if r.Success != c.wantSuccess {
				t.Fatalf("Success = %v, want %v (msg=%q err=%v)", r.Success, c.wantSuccess, r.Message, r.Error)
			}
			if c.wantErrCfg && !errors.Is(r.Error, ErrConfig) {
				t.Errorf("Error = %v, want ErrConfig", r.Error)
			}
			if r.Success && r.Duration <= 0 {
				t.Errorf("Duration = %v, want > 0", r.Duration)
			}
		})
	}
}

func TestHTTPVerifier_RequestError(t *testing.T) {
	t.Parallel()
	v := HTTPVerifier{Client: &http.Client{}}
	// Unroutable port on localhost → transport error, not a config error.
	r := v.Verify(context.Background(), Step{Config: map[string]any{"url": "http://127.0.0.1:1/x"}})
	if r.Success {
		t.Fatal("Success = true, want false on transport error")
	}
	if r.Error == nil {
		t.Error("Error = nil, want transport error")
	}
}

func TestHTTPVerifier_NilClientDefault(t *testing.T) {
	t.Parallel()
	v := HTTPVerifier{}
	if v.client() == nil {
		t.Fatal("client() = nil with no injected client")
	}
	if v.client().Timeout != defaultHTTPTimeout {
		t.Errorf("default timeout = %v, want %v", v.client().Timeout, defaultHTTPTimeout)
	}
}

func TestHTTPVerifier_Type(t *testing.T) {
	t.Parallel()
	if (HTTPVerifier{}).Type() != "http" {
		t.Error("Type() != http")
	}
}
