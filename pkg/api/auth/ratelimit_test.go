package auth

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/testing/helpers"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func TestAuthRateLimiter_IsAllowed(t *testing.T) {
	config := RateLimitConfig{
		MaxFailures:     3,
		LockoutDuration: 5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
		Enabled:         true,
	}
	rl := NewAuthRateLimiter(config)
	defer rl.Stop()

	clientID := "192.168.1.1"

	// Should be allowed initially
	if !rl.IsAllowed(clientID) {
		t.Error("expected client to be allowed initially")
	}

	// Record failures
	rl.RecordFailure(clientID)
	rl.RecordFailure(clientID)
	if !rl.IsAllowed(clientID) {
		t.Error("expected client to be allowed after 2 failures")
	}

	// Third failure should trigger lockout
	rl.RecordFailure(clientID)
	if rl.IsAllowed(clientID) {
		t.Error("expected client to be locked out after 3 failures")
	}
}

func TestAuthRateLimiter_RecordSuccess(t *testing.T) {
	config := RateLimitConfig{
		MaxFailures:     3,
		LockoutDuration: 5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
		Enabled:         true,
	}
	rl := NewAuthRateLimiter(config)
	defer rl.Stop()

	clientID := "192.168.1.1"

	// Record some failures
	rl.RecordFailure(clientID)
	rl.RecordFailure(clientID)

	if rl.GetFailureCount(clientID) != 2 {
		t.Errorf("expected 2 failures, got %d", rl.GetFailureCount(clientID))
	}

	// Successful auth should clear failures
	rl.RecordSuccess(clientID)

	if rl.GetFailureCount(clientID) != 0 {
		t.Errorf("expected 0 failures after success, got %d", rl.GetFailureCount(clientID))
	}
}

func TestAuthRateLimiter_LockoutRemaining(t *testing.T) {
	config := RateLimitConfig{
		MaxFailures:     2,
		LockoutDuration: 1 * time.Second,
		CleanupInterval: 1 * time.Minute,
		Enabled:         true,
	}
	rl := NewAuthRateLimiter(config)
	defer rl.Stop()

	clientID := "192.168.1.1"

	// Not locked out initially
	if rl.GetLockoutRemaining(clientID) != 0 {
		t.Error("expected no lockout initially")
	}

	// Trigger lockout
	rl.RecordFailure(clientID)
	rl.RecordFailure(clientID)

	// Should have remaining lockout time
	remaining := rl.GetLockoutRemaining(clientID)
	if remaining == 0 {
		t.Error("expected lockout remaining time after failures")
	}

	if err := helpers.WaitForTimeout(3*time.Second, 10*time.Millisecond, func() (bool, error) {
		return rl.IsAllowed(clientID), nil
	}); err != nil {
		t.Fatalf("expected client to be allowed after lockout expires: %v", err)
	}

	// Should be allowed again
	if !rl.IsAllowed(clientID) {
		t.Error("expected client to be allowed after lockout expires")
	}
}

func TestAuthRateLimiter_Disabled(t *testing.T) {
	config := RateLimitConfig{
		MaxFailures:     1,
		LockoutDuration: 5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
		Enabled:         false, // Disabled
	}
	rl := NewAuthRateLimiter(config)
	defer rl.Stop()

	clientID := "192.168.1.1"

	// Should always be allowed when disabled
	rl.RecordFailure(clientID)
	rl.RecordFailure(clientID)
	rl.RecordFailure(clientID)

	if !rl.IsAllowed(clientID) {
		t.Error("expected client to always be allowed when rate limiter is disabled")
	}
}

func TestAuthRateLimiter_Stats(t *testing.T) {
	config := RateLimitConfig{
		MaxFailures:     3,
		LockoutDuration: 5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
		Enabled:         true,
	}
	rl := NewAuthRateLimiter(config)
	defer rl.Stop()

	// Record failures for multiple clients
	rl.RecordFailure("client1")
	rl.RecordFailure("client1")
	rl.RecordFailure("client2")
	rl.RecordFailure("client2")
	rl.RecordFailure("client2") // Triggers lockout

	stats := rl.Stats()
	if stats.TrackedClients != 2 {
		t.Errorf("expected 2 tracked clients, got %d", stats.TrackedClients)
	}
	if stats.TotalFailures != 5 {
		t.Errorf("expected 5 total failures, got %d", stats.TotalFailures)
	}
	if stats.LockedClients != 1 {
		t.Errorf("expected 1 locked client, got %d", stats.LockedClients)
	}
}

func TestAuthRateLimiter_Reset(t *testing.T) {
	config := RateLimitConfig{
		MaxFailures:     3,
		LockoutDuration: 5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
		Enabled:         true,
	}
	rl := NewAuthRateLimiter(config)
	defer rl.Stop()

	// Record some failures
	rl.RecordFailure("client1")
	rl.RecordFailure("client2")

	// Reset
	rl.Reset()

	stats := rl.Stats()
	if stats.TrackedClients != 0 {
		t.Errorf("expected 0 tracked clients after reset, got %d", stats.TrackedClients)
	}
}

func TestClientIDFromContext_Peer(t *testing.T) {
	// Create context with peer info
	addr := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 12345}
	p := &peer.Peer{Addr: addr}
	ctx := peer.NewContext(context.Background(), p)

	clientID := ClientIDFromContext(ctx)
	if clientID != "10.0.0.1" {
		t.Errorf("expected client ID '10.0.0.1', got '%s'", clientID)
	}
}

func TestClientIDFromContext_Forwarded(t *testing.T) {
	// Create context with X-Forwarded-For header
	md := metadata.New(map[string]string{
		"x-forwarded-for": "203.0.113.50",
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	clientID := ClientIDFromContext(ctx)
	if clientID != "203.0.113.50" {
		t.Errorf("expected client ID '203.0.113.50', got '%s'", clientID)
	}
}

func TestClientIDFromContext_RealIP(t *testing.T) {
	// Create context with X-Real-IP header
	md := metadata.New(map[string]string{
		"x-real-ip": "198.51.100.25",
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	clientID := ClientIDFromContext(ctx)
	if clientID != "198.51.100.25" {
		t.Errorf("expected client ID '198.51.100.25', got '%s'", clientID)
	}
}

func TestClientIDFromContext_Unknown(t *testing.T) {
	ctx := context.Background()
	clientID := ClientIDFromContext(ctx)
	if clientID != "unknown" {
		t.Errorf("expected client ID 'unknown', got '%s'", clientID)
	}
}

func TestAuthRateLimiter_CheckRateLimit(t *testing.T) {
	config := RateLimitConfig{
		MaxFailures:     2,
		LockoutDuration: 5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
		Enabled:         true,
	}
	rl := NewAuthRateLimiter(config)
	defer rl.Stop()

	// Create context with peer
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.100"), Port: 12345}
	p := &peer.Peer{Addr: addr}
	ctx := peer.NewContext(context.Background(), p)

	// Should pass initially
	if err := rl.CheckRateLimit(ctx); err != nil {
		t.Errorf("expected no error initially, got %v", err)
	}

	// Trigger lockout
	rl.RecordFailure("192.168.1.100")
	rl.RecordFailure("192.168.1.100")

	// Should return rate limit error
	err := rl.CheckRateLimit(ctx)
	if err == nil {
		t.Error("expected rate limit error after lockout")
	}

	// Check error code
	if s, ok := status.FromError(err); ok {
		if s.Code() != codes.ResourceExhausted {
			t.Errorf("expected ResourceExhausted code, got %v", s.Code())
		}
	} else {
		t.Error("expected gRPC status error")
	}
}

func TestDefaultRateLimitConfig(t *testing.T) {
	config := DefaultRateLimitConfig()

	if config.MaxFailures != 5 {
		t.Errorf("expected MaxFailures 5, got %d", config.MaxFailures)
	}
	if config.LockoutDuration != 15*time.Minute {
		t.Errorf("expected LockoutDuration 15m, got %v", config.LockoutDuration)
	}
	if config.CleanupInterval != 5*time.Minute {
		t.Errorf("expected CleanupInterval 5m, got %v", config.CleanupInterval)
	}
	if !config.Enabled {
		t.Error("expected Enabled true by default")
	}
}

func TestRateLimitConfig_String(t *testing.T) {
	config := RateLimitConfig{
		MaxFailures:     5,
		LockoutDuration: 15 * time.Minute,
		Enabled:         true,
	}

	s := config.String()
	if s == "" {
		t.Error("expected non-empty string representation")
	}
}
