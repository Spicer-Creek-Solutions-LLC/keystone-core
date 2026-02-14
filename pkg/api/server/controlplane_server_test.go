package server

import (
	"context"
	"testing"
)

func TestParsePageToken(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected int
	}{
		{"empty token", "", 0},
		{"invalid base64", "not-valid-base64!!!", 0},
		{"valid token offset 0", encodePageToken(0), 0},
		{"valid token offset 10", encodePageToken(10), 10},
		{"valid token offset 100", encodePageToken(100), 100},
		{"negative offset encoded manually", "LTE=", 0}, // base64("-1")
		{"non-numeric", "YWJj", 0},                      // base64("abc")
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePageToken(tt.token)
			if result != tt.expected {
				t.Errorf("parsePageToken(%q) = %d, want %d", tt.token, result, tt.expected)
			}
		})
	}
}

func TestEncodePageToken(t *testing.T) {
	tests := []struct {
		name     string
		offset   int
		expected string
	}{
		{"zero offset", 0, ""},
		{"negative offset", -1, ""},
		{"positive offset 10", 10, "MTA="},   // base64("10")
		{"positive offset 100", 100, "MTAw"}, // base64("100")
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodePageToken(tt.offset)
			if result != tt.expected {
				t.Errorf("encodePageToken(%d) = %q, want %q", tt.offset, result, tt.expected)
			}
		})
	}
}

func TestPageTokenRoundTrip(t *testing.T) {
	offsets := []int{1, 10, 50, 100, 500, 1000}

	for _, offset := range offsets {
		token := encodePageToken(offset)
		decoded := parsePageToken(token)
		if decoded != offset {
			t.Errorf("round trip failed: offset=%d, token=%q, decoded=%d", offset, token, decoded)
		}
	}
}

func TestGetServerStatus(t *testing.T) {
	srv := NewControlPlaneServer(nil, nil, nil, nil)
	resp, err := srv.GetServerStatus(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetServerStatus failed: %v", err)
	}

	status := resp.Status
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if status.Version == "" {
		t.Error("expected non-empty version")
	}
	if status.UptimeSeconds < 0 {
		t.Errorf("uptime should be >= 0, got %d", status.UptimeSeconds)
	}
	if status.StartedAt == nil {
		t.Error("expected non-nil started_at")
	}
	if status.GoroutineCount <= 0 {
		t.Errorf("expected positive goroutine count, got %d", status.GoroutineCount)
	}
	if status.MemoryUsageMb < 0 {
		t.Errorf("expected non-negative memory, got %d", status.MemoryUsageMb)
	}
	if status.ConnectedAgents != 0 {
		t.Errorf("expected 0 connected agents with nil connMgr, got %d", status.ConnectedAgents)
	}
}
