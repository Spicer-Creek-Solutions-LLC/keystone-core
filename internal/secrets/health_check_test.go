package secrets

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPHealthChecker(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		checker := NewHTTPHealthChecker()
		if checker.Type() != "http" {
			t.Errorf("expected type http, got %s", checker.Type())
		}
	})

	t.Run("HealthyEndpoint", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		checker := NewHTTPHealthChecker()
		target := &RotationTarget{ID: "test-1", Name: "Test Target"}
		config := &HealthCheckConfig{
			Type:     "http",
			Endpoint: server.URL,
		}

		healthy, err := checker.Check(context.Background(), target, config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !healthy {
			t.Error("expected healthy result")
		}
	})

	t.Run("UnhealthyEndpoint", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		checker := NewHTTPHealthChecker()
		target := &RotationTarget{ID: "test-1", Name: "Test Target"}
		config := &HealthCheckConfig{
			Type:     "http",
			Endpoint: server.URL,
		}

		healthy, err := checker.Check(context.Background(), target, config)
		if err == nil {
			t.Error("expected error for unhealthy endpoint")
		}
		if healthy {
			t.Error("expected unhealthy result")
		}
	})

	t.Run("CustomExpectedStatus", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}))
		defer server.Close()

		checker := NewHTTPHealthChecker()
		target := &RotationTarget{ID: "test-1", Name: "Test Target"}
		config := &HealthCheckConfig{
			Type:           "http",
			Endpoint:       server.URL,
			ExpectedStatus: http.StatusAccepted,
		}

		healthy, err := checker.Check(context.Background(), target, config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !healthy {
			t.Error("expected healthy result with custom status")
		}
	})

	t.Run("PlaceholderReplacement", func(t *testing.T) {
		var receivedPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		checker := NewHTTPHealthChecker()
		target := &RotationTarget{
			ID:      "target-123",
			Name:    "MyTarget",
			AgentID: "agent-456",
		}
		config := &HealthCheckConfig{
			Type:     "http",
			Endpoint: server.URL + "/health/{target_id}/{agent_id}",
		}

		_, _ = checker.Check(context.Background(), target, config)

		if receivedPath != "/health/target-123/agent-456" {
			t.Errorf("expected path /health/target-123/agent-456, got %s", receivedPath)
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		checker := NewHTTPHealthChecker()
		target := &RotationTarget{ID: "test-1", Name: "Test Target"}
		config := &HealthCheckConfig{
			Type:     "http",
			Endpoint: server.URL,
			Timeout:  100 * time.Millisecond,
		}

		healthy, err := checker.Check(context.Background(), target, config)
		if err == nil {
			t.Error("expected timeout error")
		}
		if healthy {
			t.Error("expected unhealthy result on timeout")
		}
	})

	t.Run("NoEndpoint", func(t *testing.T) {
		checker := NewHTTPHealthChecker()
		target := &RotationTarget{ID: "test-1", Name: "Test Target"}
		config := &HealthCheckConfig{
			Type: "http",
		}

		_, err := checker.Check(context.Background(), target, config)
		if err == nil {
			t.Error("expected error for missing endpoint")
		}
	})

	t.Run("UseTargetEndpoint", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		checker := NewHTTPHealthChecker()
		target := &RotationTarget{
			ID:       "test-1",
			Name:     "Test Target",
			Endpoint: server.URL,
		}
		config := &HealthCheckConfig{
			Type: "http",
		}

		healthy, err := checker.Check(context.Background(), target, config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !healthy {
			t.Error("expected healthy result using target endpoint")
		}
	})
}

func TestTCPHealthChecker(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		checker := NewTCPHealthChecker()
		if checker.Type() != "tcp" {
			t.Errorf("expected type tcp, got %s", checker.Type())
		}
	})

	t.Run("HealthyEndpoint", func(t *testing.T) {
		// Start a TCP listener
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to start listener: %v", err)
		}
		defer listener.Close()

		// Accept connections in background
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				conn.Close()
			}
		}()

		checker := NewTCPHealthChecker()
		target := &RotationTarget{ID: "test-1", Name: "Test Target"}
		config := &HealthCheckConfig{
			Type:     "tcp",
			Endpoint: listener.Addr().String(),
		}

		healthy, err := checker.Check(context.Background(), target, config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !healthy {
			t.Error("expected healthy result")
		}
	})

	t.Run("UnhealthyEndpoint", func(t *testing.T) {
		checker := NewTCPHealthChecker()
		target := &RotationTarget{ID: "test-1", Name: "Test Target"}
		config := &HealthCheckConfig{
			Type:     "tcp",
			Endpoint: "127.0.0.1:59999", // Unlikely to be listening
			Timeout:  100 * time.Millisecond,
		}

		healthy, err := checker.Check(context.Background(), target, config)
		if err == nil {
			t.Error("expected error for unreachable endpoint")
		}
		if healthy {
			t.Error("expected unhealthy result")
		}
	})

	t.Run("NoEndpoint", func(t *testing.T) {
		checker := NewTCPHealthChecker()
		target := &RotationTarget{ID: "test-1", Name: "Test Target"}
		config := &HealthCheckConfig{
			Type: "tcp",
		}

		_, err := checker.Check(context.Background(), target, config)
		if err == nil {
			t.Error("expected error for missing endpoint")
		}
	})
}

func TestExecHealthChecker(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		checker := NewExecHealthChecker()
		if checker.Type() != "exec" {
			t.Errorf("expected type exec, got %s", checker.Type())
		}
	})

	t.Run("SuccessfulCommand", func(t *testing.T) {
		checker := NewExecHealthChecker()
		target := &RotationTarget{ID: "test-1", Name: "Test Target"}
		config := &HealthCheckConfig{
			Type:    "exec",
			Command: "true",
		}

		healthy, err := checker.Check(context.Background(), target, config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !healthy {
			t.Error("expected healthy result")
		}
	})

	t.Run("FailedCommand", func(t *testing.T) {
		checker := NewExecHealthChecker()
		target := &RotationTarget{ID: "test-1", Name: "Test Target"}
		config := &HealthCheckConfig{
			Type:    "exec",
			Command: "false",
		}

		healthy, err := checker.Check(context.Background(), target, config)
		if err == nil {
			t.Error("expected error for failed command")
		}
		if healthy {
			t.Error("expected unhealthy result")
		}
	})

	t.Run("PlaceholderReplacement", func(t *testing.T) {
		checker := NewExecHealthChecker()
		target := &RotationTarget{
			ID:      "target-123",
			Name:    "MyTarget",
			AgentID: "agent-456",
		}
		config := &HealthCheckConfig{
			Type:    "exec",
			Command: "echo {target_id} {agent_id}",
		}

		healthy, err := checker.Check(context.Background(), target, config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !healthy {
			t.Error("expected healthy result")
		}
	})

	t.Run("NoCommand", func(t *testing.T) {
		checker := NewExecHealthChecker()
		target := &RotationTarget{ID: "test-1", Name: "Test Target"}
		config := &HealthCheckConfig{
			Type: "exec",
		}

		_, err := checker.Check(context.Background(), target, config)
		if err == nil {
			t.Error("expected error for missing command")
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		checker := NewExecHealthChecker()
		target := &RotationTarget{ID: "test-1", Name: "Test Target"}
		config := &HealthCheckConfig{
			Type:    "exec",
			Command: "sleep 10",
			Timeout: 100 * time.Millisecond,
		}

		healthy, err := checker.Check(context.Background(), target, config)
		if err == nil {
			t.Error("expected timeout error")
		}
		if healthy {
			t.Error("expected unhealthy result on timeout")
		}
	})
}

func TestHealthCheckRegistry(t *testing.T) {
	t.Run("DefaultCheckers", func(t *testing.T) {
		registry := NewHealthCheckRegistry()

		if _, ok := registry.Get("http"); !ok {
			t.Error("expected http checker to be registered")
		}
		if _, ok := registry.Get("tcp"); !ok {
			t.Error("expected tcp checker to be registered")
		}
		if _, ok := registry.Get("exec"); !ok {
			t.Error("expected exec checker to be registered")
		}
	})

	t.Run("UnknownChecker", func(t *testing.T) {
		registry := NewHealthCheckRegistry()

		_, ok := registry.Get("unknown")
		if ok {
			t.Error("expected unknown checker to not be found")
		}
	})

	t.Run("CheckTarget_WithRetries", func(t *testing.T) {
		attemptCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attemptCount++
			if attemptCount < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		registry := NewHealthCheckRegistry()
		target := &RotationTarget{ID: "test-1", Name: "Test Target"}
		config := &HealthCheckConfig{
			Type:     "http",
			Endpoint: server.URL,
			Retries:  3,
			Interval: 10 * time.Millisecond,
		}

		result, err := registry.CheckTarget(context.Background(), target, config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Healthy {
			t.Error("expected healthy result after retries")
		}
		if result.Attempt != 3 {
			t.Errorf("expected 3 attempts, got %d", result.Attempt)
		}
	})

	t.Run("CheckTarget_AllRetriesFail", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		registry := NewHealthCheckRegistry()
		target := &RotationTarget{ID: "test-1", Name: "Test Target"}
		config := &HealthCheckConfig{
			Type:     "http",
			Endpoint: server.URL,
			Retries:  2,
			Interval: 10 * time.Millisecond,
		}

		result, err := registry.CheckTarget(context.Background(), target, config)
		if err == nil {
			t.Error("expected error after all retries fail")
		}
		if result.Healthy {
			t.Error("expected unhealthy result")
		}
		if result.Attempt != 3 { // 1 initial + 2 retries
			t.Errorf("expected 3 attempts, got %d", result.Attempt)
		}
	})

	t.Run("CheckTarget_NilConfig", func(t *testing.T) {
		registry := NewHealthCheckRegistry()
		target := &RotationTarget{ID: "test-1", Name: "Test Target"}

		_, err := registry.CheckTarget(context.Background(), target, nil)
		if err == nil {
			t.Error("expected error for nil config")
		}
	})

	t.Run("CheckTarget_UnknownType", func(t *testing.T) {
		registry := NewHealthCheckRegistry()
		target := &RotationTarget{ID: "test-1", Name: "Test Target"}
		config := &HealthCheckConfig{
			Type: "unknown",
		}

		_, err := registry.CheckTarget(context.Background(), target, config)
		if err == nil {
			t.Error("expected error for unknown check type")
		}
	})

	t.Run("CheckTargets_Concurrent", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		registry := NewHealthCheckRegistry()
		targets := []*RotationTarget{
			{ID: "target-1", Name: "Target 1"},
			{ID: "target-2", Name: "Target 2"},
			{ID: "target-3", Name: "Target 3"},
		}
		config := &HealthCheckConfig{
			Type:     "http",
			Endpoint: server.URL,
		}

		results, err := registry.CheckTargets(context.Background(), targets, config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("expected 3 results, got %d", len(results))
		}
		for i, result := range results {
			if !result.Healthy {
				t.Errorf("target %d not healthy", i)
			}
		}
	})

	t.Run("CheckTarget_ContextCancelled", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(5 * time.Second)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		registry := NewHealthCheckRegistry()
		target := &RotationTarget{ID: "test-1", Name: "Test Target"}
		config := &HealthCheckConfig{
			Type:     "http",
			Endpoint: server.URL,
			Retries:  3,
			Interval: time.Second,
			Timeout:  10 * time.Second, // Long timeout so we hit cancellation first
		}

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		result, err := registry.CheckTarget(ctx, target, config)
		// Either error or unhealthy result is acceptable when context is cancelled
		if err == nil && (result != nil && result.Healthy) {
			t.Error("expected error or unhealthy result when context cancelled")
		}
	})
}
