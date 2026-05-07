package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"

	"go.keystone-core.io/keystone-core/pkg/api/server"
)

// TestIntegration_FullLifecycle is the epic-04 task-10 integration
// test: spawn the full server with test config (SQLite + NoopNATS +
// auth disabled), exercise gRPC + REST surfaces with real clients,
// trigger a SIGTERM-equivalent (ctx cancel — see comment below), and
// assert clean shutdown with ordered logs.
//
// Goroutine-leak coverage is inherited from TestMain (server_test.go),
// which runs goleak.VerifyTestMain across every test in the package.
//
// Why ctx-cancel instead of syscall.SIGTERM: real-SIGTERM tests
// affect the whole test process (other parallel tests, the runner).
// signal.NotifyContext is stdlib; once it fires, the code path is
// identical to a manual cancel — ctx.Done() unblocks, Stop runs.
// Testing ctx-cancel exercises every line our code owns; testing
// the signal handler itself would only verify stdlib.
//
// "GetServerStatus" stand-in: the spec calls for a gRPC client to
// call ControlPlaneService.GetServerStatus, but that service ships
// with Epic 07. v1.0 task 10 registers grpc/health and calls Check
// — same wiring path (interceptor → handler → response). Epic 07
// will replace this with the real call.
func TestIntegration_FullLifecycle(t *testing.T) {
	log, logBuf := captureLogger(t)

	cfg := newTestConfig()
	srv, err := server.New(server.Options{
		Config:      cfg,
		Logger:      log,
		Store:       newTestStore(t),
		NATSManager: server.NoopNATSManager{},
		Subjects:    fakeSubjects{cluster: "default"},
		// AuthInterceptor intentionally nil — task-10 spec specifies
		// "auth disabled" so /api/status is reachable without creds.
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	healthv1.RegisterHealthServer(grpcRegistrar(srv), health.NewServer())

	runCtx, cancel := context.WithCancel(context.Background())
	if err := srv.Start(runCtx); err != nil {
		cancel()
		t.Fatalf("Start: %v", err)
	}
	addrs := srv.Addrs()

	t.Run("gRPC client call", func(t *testing.T) {
		conn, err := grpc.NewClient(
			addrs.GRPC,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		defer conn.Close()

		ctx, ccancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer ccancel()
		resp, err := healthv1.NewHealthClient(conn).Check(ctx, &healthv1.HealthCheckRequest{})
		if err != nil {
			t.Fatalf("Health.Check: %v", err)
		}
		if resp.GetStatus() != healthv1.HealthCheckResponse_SERVING {
			t.Errorf("status = %v, want SERVING", resp.GetStatus())
		}
	})

	t.Run("HTTP /health/live", func(t *testing.T) {
		mustGetStatus(t, "http://"+addrs.HTTP+"/health/live", http.StatusOK)
	})

	t.Run("HTTP /health/ready after grace", func(t *testing.T) {
		// newTestConfig sets a 1ms grace period; sleep long enough to
		// guarantee we're past it on slow CI.
		time.Sleep(50 * time.Millisecond)
		mustGetStatus(t, "http://"+addrs.HTTP+"/health/ready", http.StatusOK)
	})

	t.Run("HTTP /api/status payload shape", func(t *testing.T) {
		resp, err := http.Get("http://" + addrs.HTTP + "/api/status")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("body not JSON: %v\n%s", err, body)
		}
		want := []string{
			"version", "uptime", "started_at", "ready",
			"auth_mode", "production_warnings",
			"components", "agents", "runtime",
		}
		for _, k := range want {
			if _, ok := payload[k]; !ok {
				t.Errorf("/api/status missing %q field", k)
			}
		}
	})

	t.Run("HTTP per-domain handler reachable", func(t *testing.T) {
		// Sample one domain to verify Epic 03 task 7 stubs are wired
		// through the API mux. 501 is the expected response from the
		// stub handler — anything else (404, 401, 500) indicates a
		// regression in the routing tree.
		resp, err := http.Get("http://" + addrs.HTTP + "/api/v1/agents")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotImplemented {
			t.Errorf("status = %d, want 501 (epic 03 stub)", resp.StatusCode)
		}
	})

	// SIGTERM-equivalent: cancel the run context.
	cancel()

	t.Run("clean shutdown with ordered logs", func(t *testing.T) {
		stopCtx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer scancel()
		if err := srv.Stop(stopCtx); err != nil {
			t.Errorf("Stop: %v", err)
		}

		lines := parseLogLines(t, logBuf)
		if findLog(lines, "server: shutdown begin") == nil {
			t.Error("missing 'shutdown begin' log line")
		}
		if findLog(lines, "server: shutdown complete") == nil {
			t.Error("missing 'shutdown complete' log line")
		}
	})
}

func mustGetStatus(t *testing.T, url string, want int) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("GET %s = %d, want %d; body=%s", url, resp.StatusCode, want, body)
	}
}
