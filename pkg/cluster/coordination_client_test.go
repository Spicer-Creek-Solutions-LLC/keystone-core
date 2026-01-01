package cluster

import (
	"context"
	"testing"
	"time"
)

func TestNewCoordinationClient(t *testing.T) {
	tests := []struct {
		name      string
		config    *CoordinationClientConfig
		wantError bool
	}{
		{
			name:      "nil config",
			config:    nil,
			wantError: true,
		},
		{
			name:      "empty local ID",
			config:    &CoordinationClientConfig{},
			wantError: true,
		},
		{
			name:      "valid config",
			config:    DefaultCoordinationClientConfig("server-1"),
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewCoordinationClient(tt.config)
			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if client == nil {
				t.Error("expected client, got nil")
			}
		})
	}
}

func TestDefaultCoordinationClientConfig(t *testing.T) {
	config := DefaultCoordinationClientConfig("server-1")

	if config.LocalID != "server-1" {
		t.Errorf("expected local_id=server-1, got %s", config.LocalID)
	}

	if config.DialTimeout <= 0 {
		t.Error("dial_timeout should be positive")
	}

	if config.RequestTimeout <= 0 {
		t.Error("request_timeout should be positive")
	}

	if config.KeepaliveInterval <= 0 {
		t.Error("keepalive_interval should be positive")
	}

	if config.MaxRetries <= 0 {
		t.Error("max_retries should be positive")
	}
}

func TestCoordinationClient_AddRemovePeer(t *testing.T) {
	config := DefaultCoordinationClientConfig("server-1")
	client, err := NewCoordinationClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// Test adding self (should be ignored)
	err = client.AddPeer("server-1", "localhost:9090")
	if err != nil {
		t.Errorf("adding self should not error: %v", err)
	}
	if client.GetPeerCount() != 0 {
		t.Error("self should not be added as peer")
	}

	// Test adding peer with empty member ID
	err = client.AddPeer("", "localhost:9091")
	if err == nil {
		t.Error("expected error for empty member_id")
	}

	// Test adding peer with empty address
	err = client.AddPeer("server-2", "")
	if err == nil {
		t.Error("expected error for empty address")
	}

	// Test removing non-existent peer (should not error)
	client.RemovePeer("server-999")
}

func TestCoordinationClient_ListPeers(t *testing.T) {
	config := DefaultCoordinationClientConfig("server-1")
	client, err := NewCoordinationClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// Initially empty
	peers := client.ListPeers()
	if len(peers) != 0 {
		t.Errorf("expected 0 peers, got %d", len(peers))
	}
}

func TestCoordinationClient_IsPeerHealthy(t *testing.T) {
	config := DefaultCoordinationClientConfig("server-1")
	client, err := NewCoordinationClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// Non-existent peer should return false
	if client.IsPeerHealthy("server-2") {
		t.Error("non-existent peer should not be healthy")
	}
}

func TestCoordinationClient_GetPeerLastSeen(t *testing.T) {
	config := DefaultCoordinationClientConfig("server-1")
	client, err := NewCoordinationClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// Non-existent peer should return error
	_, err = client.GetPeerLastSeen("server-2")
	if err == nil {
		t.Error("expected error for non-existent peer")
	}
}

func TestCoordinationClient_GetHealthyPeerCount(t *testing.T) {
	config := DefaultCoordinationClientConfig("server-1")
	client, err := NewCoordinationClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// Initially 0
	if count := client.GetHealthyPeerCount(); count != 0 {
		t.Errorf("expected 0 healthy peers, got %d", count)
	}
}

func TestCoordinationClient_Close(t *testing.T) {
	config := DefaultCoordinationClientConfig("server-1")
	client, err := NewCoordinationClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Close should succeed even with no peers
	err = client.Close()
	if err != nil {
		t.Errorf("close failed: %v", err)
	}

	// Verify peers map is cleared
	if client.GetPeerCount() != 0 {
		t.Error("peers should be cleared after close")
	}
}

func TestCoordinationClient_WithTimeout(t *testing.T) {
	config := DefaultCoordinationClientConfig("server-1")
	config.RequestTimeout = 5 * time.Second
	client, err := NewCoordinationClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	ctxWithTimeout, cancel := client.withTimeout(ctx)
	defer cancel()

	// Should have a deadline
	deadline, ok := ctxWithTimeout.Deadline()
	if !ok {
		t.Error("expected context to have deadline")
	}

	expectedDeadline := time.Now().Add(5 * time.Second)
	if deadline.After(expectedDeadline.Add(time.Second)) {
		t.Error("deadline is too far in the future")
	}
}

func TestCoordinationClient_NewRequestID(t *testing.T) {
	config := DefaultCoordinationClientConfig("server-1")
	client, err := NewCoordinationClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	id1 := client.newRequestID()
	id2 := client.newRequestID()

	if id1 == "" {
		t.Error("request ID should not be empty")
	}

	if id1 == id2 {
		t.Error("request IDs should be unique")
	}
}

func TestCoordinationClient_MarkPeerHealth(t *testing.T) {
	config := DefaultCoordinationClientConfig("server-1")
	client, err := NewCoordinationClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// Manually add a peer to test marking
	client.mu.Lock()
	client.peers["server-2"] = &peerConnection{
		memberID: "server-2",
		address:  "localhost:9091",
		healthy:  true,
		lastSeen: time.Now().Add(-time.Hour),
	}
	client.mu.Unlock()

	// Mark as unhealthy
	client.markPeerUnhealthy("server-2")
	if client.IsPeerHealthy("server-2") {
		t.Error("peer should be marked unhealthy")
	}

	// Mark as healthy
	client.markPeerHealthy("server-2")
	if !client.IsPeerHealthy("server-2") {
		t.Error("peer should be marked healthy")
	}

	// Check that lastSeen is updated
	lastSeen, err := client.GetPeerLastSeen("server-2")
	if err != nil {
		t.Errorf("failed to get last seen: %v", err)
	}
	if time.Since(lastSeen) > time.Second {
		t.Error("lastSeen should be updated to recent time")
	}
}

func TestCoordinationClient_GetPeerErrors(t *testing.T) {
	config := DefaultCoordinationClientConfig("server-1")
	client, err := NewCoordinationClient(config)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Test ClusterHealth with non-existent peer
	_, err = client.ClusterHealth(ctx, "server-999", false, false)
	if err == nil {
		t.Error("expected error for non-existent peer")
	}

	// Test GetLeader with non-existent peer
	_, err = client.GetLeader(ctx, "server-999")
	if err == nil {
		t.Error("expected error for non-existent peer")
	}

	// Test NATSStatus with non-existent peer
	_, err = client.NATSStatus(ctx, "server-999")
	if err == nil {
		t.Error("expected error for non-existent peer")
	}

	// Test Heartbeat with non-existent peer
	_, err = client.Heartbeat(ctx, "server-999")
	if err == nil {
		t.Error("expected error for non-existent peer")
	}
}
