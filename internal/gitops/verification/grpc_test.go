// SPDX-License-Identifier: Apache-2.0

package verification

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func TestGRPCVerifier_FakeChecker(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		check       HealthChecker
		cfg         map[string]any
		wantSuccess bool
		wantErrCfg  bool
	}{
		{
			name:        "serving",
			check:       func(context.Context, string, string, bool) (string, error) { return "SERVING", nil },
			cfg:         map[string]any{"target": "h:1"},
			wantSuccess: true,
		},
		{
			name:  "not serving",
			check: func(context.Context, string, string, bool) (string, error) { return "NOT_SERVING", nil },
			cfg:   map[string]any{"target": "h:1"},
		},
		{
			name:  "check error",
			check: func(context.Context, string, string, bool) (string, error) { return "", errors.New("dial fail") },
			cfg:   map[string]any{"target": "h:1"},
		},
		{
			name:       "missing target",
			check:      func(context.Context, string, string, bool) (string, error) { return "SERVING", nil },
			cfg:        map[string]any{},
			wantErrCfg: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := GRPCVerifier{Check: c.check}.Verify(context.Background(), Step{Config: c.cfg})
			if r.Success != c.wantSuccess {
				t.Fatalf("Success = %v, want %v (msg=%q err=%v)", r.Success, c.wantSuccess, r.Message, r.Error)
			}
			if c.wantErrCfg && !errors.Is(r.Error, ErrConfig) {
				t.Errorf("Error = %v, want ErrConfig", r.Error)
			}
		})
	}
}

func TestGRPCVerifier_Type(t *testing.T) {
	t.Parallel()
	if (GRPCVerifier{}).Type() != "grpc" {
		t.Error("Type() != grpc")
	}
}

// startHealthServer runs a real grpc.health.v1 server on a localhost
// port so the default (non-injected) health-check path is exercised
// end-to-end.
func startHealthServer(t *testing.T, status grpc_health_v1.HealthCheckResponse_ServingStatus) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	hs := health.NewServer()
	hs.SetServingStatus("", status)
	grpc_health_v1.RegisterHealthServer(srv, hs)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func TestGRPCVerifier_DefaultHealthCheck_Integration(t *testing.T) {
	t.Parallel()

	t.Run("serving", func(t *testing.T) {
		t.Parallel()
		addr := startHealthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		r := GRPCVerifier{}.Verify(ctx, Step{Config: map[string]any{"target": addr}})
		if !r.Success {
			t.Fatalf("Success = false: %q %v", r.Message, r.Error)
		}
		if r.Data["serving_status"] != "SERVING" {
			t.Errorf("serving_status = %v", r.Data["serving_status"])
		}
	})

	t.Run("not serving", func(t *testing.T) {
		t.Parallel()
		addr := startHealthServer(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		r := GRPCVerifier{}.Verify(ctx, Step{Config: map[string]any{"target": addr}})
		if r.Success {
			t.Fatal("Success = true, want false for NOT_SERVING")
		}
	})

	t.Run("dial failure on closed port", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		r := GRPCVerifier{}.Verify(ctx, Step{Config: map[string]any{"target": "127.0.0.1:1"}})
		if r.Success || r.Error == nil {
			t.Errorf("want failed result with error, got %+v", r)
		}
	})
}
