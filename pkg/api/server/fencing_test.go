// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeFencer is a controllable Fencer. mode mirrors the cluster fence
// modes: when fenced, "read_only" blocks writes only, "strict" blocks
// everything. It counts live (un-released) guards so tests can assert
// the interceptor/middleware always releases.
type fakeFencer struct {
	mu     sync.Mutex
	fenced bool
	strict bool // strict ⇒ reads are blocked too when fenced
	live   int  // outstanding (acquired but not released) guards
}

func (f *fakeFencer) Guard(write bool) (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fenced && (write || f.strict) {
		return nil, errors.New("fenced")
	}
	f.live++
	var once sync.Once
	return func() {
		once.Do(func() {
			f.mu.Lock()
			f.live--
			f.mu.Unlock()
		})
	}, nil
}

func (f *fakeFencer) liveCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.live
}

func TestIsWriteMethod(t *testing.T) {
	writes := []string{
		"/keystone.core.v1.StateService/ApplyState",
		"/keystone.core.v1.ControlPlaneService/ExecuteCommand",
		"/keystone.core.v1.ControlPlaneService/BatchExecuteCommand",
		"/keystone.core.v1.SecretsService/WriteSecret",
		"/keystone.core.v1.ClusterService/TransferLeader",
	}
	reads := []string{
		"/keystone.core.v1.StateService/GetStateStatus",
		"/keystone.core.v1.ControlPlaneService/ListAgents",
		"/keystone.core.v1.ClusterService/GetClusterStatus",
		"/keystone.core.v1.EventService/SubscribeEvents",
		// CoordinationService must never be classified as a write — it
		// is the recovery channel (and runs on its own listener).
		"/keystone.core.v1.CoordinationService/PropagateState",
		// Unknown method ⇒ read (storage-quorum backstop; reads continue).
		"/keystone.core.v1.SomeNewService/SomeMethod",
	}
	for _, m := range writes {
		if !isWriteMethod(m) {
			t.Errorf("isWriteMethod(%q) = false, want true", m)
		}
	}
	for _, m := range reads {
		if isWriteMethod(m) {
			t.Errorf("isWriteMethod(%q) = true, want false", m)
		}
	}
}

func TestFencingUnaryInterceptor(t *testing.T) {
	okHandler := func(context.Context, any) (any, error) { return "ok", nil }

	tests := []struct {
		name     string
		fenced   bool
		strict   bool
		method   string
		wantErr  bool
		wantCode codes.Code
	}{
		{"not fenced: write passes", false, false, "/keystone.core.v1.StateService/ApplyState", false, codes.OK},
		{"read_only fenced: write blocked", true, false, "/keystone.core.v1.StateService/ApplyState", true, codes.Unavailable},
		{"read_only fenced: read passes", true, false, "/keystone.core.v1.StateService/GetStateStatus", false, codes.OK},
		{"strict fenced: read blocked", true, true, "/keystone.core.v1.StateService/GetStateStatus", true, codes.Unavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeFencer{fenced: tt.fenced, strict: tt.strict}
			interceptor := fencingUnaryInterceptor(f)
			_, err := interceptor(context.Background(), nil,
				&grpc.UnaryServerInfo{FullMethod: tt.method}, okHandler)
			if tt.wantErr {
				if status.Code(err) != tt.wantCode {
					t.Fatalf("code = %v, want %v (err=%v)", status.Code(err), tt.wantCode, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Every path must leave no guard outstanding.
			if n := f.liveCount(); n != 0 {
				t.Fatalf("live guards = %d, want 0 (release not called)", n)
			}
		})
	}
}

func TestFencingHTTPMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name     string
		fenced   bool
		strict   bool
		method   string
		wantCode int
	}{
		{"not fenced: POST passes", false, false, http.MethodPost, http.StatusOK},
		{"read_only fenced: POST blocked", true, false, http.MethodPost, http.StatusServiceUnavailable},
		{"read_only fenced: DELETE blocked", true, false, http.MethodDelete, http.StatusServiceUnavailable},
		{"read_only fenced: GET passes", true, false, http.MethodGet, http.StatusOK},
		{"strict fenced: GET blocked", true, true, http.MethodGet, http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeFencer{fenced: tt.fenced, strict: tt.strict}
			h := fencingHTTPMiddleware(f)(next)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tt.method, "/api/v1/anything", nil))
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if n := f.liveCount(); n != 0 {
				t.Fatalf("live guards = %d, want 0 (release not called)", n)
			}
		})
	}
}

func TestIsHTTPWrite(t *testing.T) {
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		if isHTTPWrite(m) {
			t.Errorf("isHTTPWrite(%q) = true, want false", m)
		}
	}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if !isHTTPWrite(m) {
			t.Errorf("isHTTPWrite(%q) = false, want true", m)
		}
	}
}
