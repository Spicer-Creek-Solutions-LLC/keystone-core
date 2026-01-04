package targeting

import (
	"testing"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

func TestNewMatcher(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid expression",
			expression: "os:linux",
			wantErr:    false,
		},
		{
			name:       "valid complex expression",
			expression: "os:linux and role:web",
			wantErr:    false,
		},
		{
			name:        "empty expression",
			expression:  "",
			wantErr:     true,
			errContains: "empty target expression",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewMatcher(tt.expression)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewMatcher() expected error, got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("NewMatcher() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("NewMatcher() unexpected error: %v", err)
				return
			}

			if matcher == nil {
				t.Errorf("NewMatcher() returned nil matcher")
			}
		})
	}
}

func TestMatcher_Match(t *testing.T) {
	agents := []*AgentInfo{
		{
			ID:     "agent-1",
			Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
			Metadata: &pb.AgentMetadata{
				Hostname:        "web-server-01",
				Os:              "linux",
				Arch:            "amd64",
				PlatformVersion: "5.15.0",
				AgentVersion:    "1.0.0",
				IpAddresses:     []string{"192.168.1.10", "10.0.0.10"},
				Labels: map[string]string{
					"role":       "web",
					"datacenter": "us-west",
					"env":        "prod",
				},
			},
		},
		{
			ID:     "agent-2",
			Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
			Metadata: &pb.AgentMetadata{
				Hostname:        "db-server-01",
				Os:              "linux",
				Arch:            "amd64",
				PlatformVersion: "5.15.0",
				AgentVersion:    "1.0.0",
				IpAddresses:     []string{"192.168.1.20"},
				Labels: map[string]string{
					"role":       "db",
					"datacenter": "us-west",
					"env":        "prod",
				},
			},
		},
		{
			ID:     "agent-3",
			Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
			Metadata: &pb.AgentMetadata{
				Hostname:        "api-server-01",
				Os:              "darwin",
				Arch:            "arm64",
				PlatformVersion: "14.0",
				AgentVersion:    "1.0.0",
				IpAddresses:     []string{"192.168.1.30"},
				Labels: map[string]string{
					"role":       "api",
					"datacenter": "us-east",
					"env":        "dev",
				},
			},
		},
		{
			ID:     "agent-4",
			Status: pb.AgentStatus_AGENT_STATUS_OFFLINE,
			Metadata: &pb.AgentMetadata{
				Hostname:        "web-server-02",
				Os:              "linux",
				Arch:            "amd64",
				PlatformVersion: "5.15.0",
				AgentVersion:    "1.0.0",
				IpAddresses:     []string{"192.168.1.40"},
				Labels: map[string]string{
					"role":       "web",
					"datacenter": "us-west",
					"env":        "prod",
				},
			},
		},
	}

	tests := []struct {
		name       string
		expression string
		wantIDs    []string
		wantErr    bool
	}{
		{
			name:       "match all linux",
			expression: "os:linux",
			wantIDs:    []string{"agent-1", "agent-2", "agent-4"},
		},
		{
			name:       "match role web",
			expression: "role:web",
			wantIDs:    []string{"agent-1", "agent-4"},
		},
		{
			name:       "match os and role",
			expression: "os:linux and role:web",
			wantIDs:    []string{"agent-1", "agent-4"},
		},
		{
			name:       "match with or",
			expression: "role:web or role:api",
			wantIDs:    []string{"agent-1", "agent-3", "agent-4"},
		},
		{
			name:       "match with not",
			expression: "os:linux and not role:db",
			wantIDs:    []string{"agent-1", "agent-4"},
		},
		{
			name:       "match hostname glob",
			expression: "hostname:web-*",
			wantIDs:    []string{"agent-1", "agent-4"},
		},
		{
			name:       "match datacenter",
			expression: "datacenter:us-west",
			wantIDs:    []string{"agent-1", "agent-2", "agent-4"},
		},
		{
			name:       "match datacenter and env",
			expression: "datacenter:us-west and env:prod",
			wantIDs:    []string{"agent-1", "agent-2", "agent-4"},
		},
		{
			name:       "match status online",
			expression: "status:agent_status_online",
			wantIDs:    []string{"agent-1", "agent-2", "agent-3"},
		},
		{
			name:       "complex expression",
			expression: "(os:linux and role:web) or (os:darwin and role:api)",
			wantIDs:    []string{"agent-1", "agent-3", "agent-4"},
		},
		{
			name:       "no matches",
			expression: "os:windows",
			wantIDs:    []string{},
		},
		{
			name:       "match specific id",
			expression: "id:agent-2",
			wantIDs:    []string{"agent-2"},
		},
		{
			name:       "match id glob",
			expression: "id:agent-*",
			wantIDs:    []string{"agent-1", "agent-2", "agent-3", "agent-4"},
		},
		{
			name:       "match architecture",
			expression: "arch:arm64",
			wantIDs:    []string{"agent-3"},
		},
		{
			name:       "match online linux web",
			expression: "status:agent_status_online and os:linux and role:web",
			wantIDs:    []string{"agent-1"},
		},
		{
			name:       "match role with labels prefix",
			expression: "labels.role:web",
			wantIDs:    []string{"agent-1", "agent-4"},
		},
		{
			name:       "match datacenter with labels prefix",
			expression: "labels.datacenter:us-west and labels.env:prod",
			wantIDs:    []string{"agent-1", "agent-2", "agent-4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewMatcher(tt.expression)
			if err != nil {
				t.Fatalf("NewMatcher() error = %v", err)
			}

			matched, err := matcher.Match(agents)
			if (err != nil) != tt.wantErr {
				t.Errorf("Match() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			gotIDs := make([]string, len(matched))
			for i, agent := range matched {
				gotIDs[i] = agent.ID
			}

			if len(gotIDs) != len(tt.wantIDs) {
				t.Errorf("Match() returned %d agents, want %d\nGot IDs: %v\nWant IDs: %v",
					len(gotIDs), len(tt.wantIDs), gotIDs, tt.wantIDs)
				return
			}

			// Check that all expected IDs are present (order doesn't matter)
			gotMap := make(map[string]bool)
			for _, id := range gotIDs {
				gotMap[id] = true
			}

			for _, wantID := range tt.wantIDs {
				if !gotMap[wantID] {
					t.Errorf("Match() missing expected agent ID %q\nGot: %v\nWant: %v",
						wantID, gotIDs, tt.wantIDs)
				}
			}
		})
	}
}

func TestMatcher_MatchIDs(t *testing.T) {
	agents := []*AgentInfo{
		{
			ID:     "agent-1",
			Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
			Metadata: &pb.AgentMetadata{
				Os: "linux",
				Labels: map[string]string{
					"role": "web",
				},
			},
		},
		{
			ID:     "agent-2",
			Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
			Metadata: &pb.AgentMetadata{
				Os: "linux",
				Labels: map[string]string{
					"role": "db",
				},
			},
		},
	}

	matcher, err := NewMatcher("role:web")
	if err != nil {
		t.Fatalf("NewMatcher() error = %v", err)
	}

	ids, err := matcher.MatchIDs(agents)
	if err != nil {
		t.Fatalf("MatchIDs() error = %v", err)
	}

	if len(ids) != 1 {
		t.Errorf("MatchIDs() returned %d IDs, want 1", len(ids))
	}

	if len(ids) > 0 && ids[0] != "agent-1" {
		t.Errorf("MatchIDs() = %v, want [agent-1]", ids)
	}
}

func TestAgentToMetadata(t *testing.T) {
	agent := &AgentInfo{
		ID:     "test-agent",
		Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
		Metadata: &pb.AgentMetadata{
			Hostname:        "test-host",
			Os:              "linux",
			Arch:            "amd64",
			PlatformVersion: "5.15.0",
			AgentVersion:    "1.0.0",
			IpAddresses:     []string{"192.168.1.10", "10.0.0.10"},
			Labels: map[string]string{
				"role": "test",
				"env":  "dev",
			},
		},
	}

	metadata := agentToMetadata(agent)

	// Check all expected fields
	// Labels are available both directly and with "labels." prefix
	expected := map[string]string{
		"id":               "test-agent",
		"status":           "agent_status_online",
		"hostname":         "test-host",
		"os":               "linux",
		"arch":             "amd64",
		"platform_version": "5.15.0",
		"agent_version":    "1.0.0",
		"ip":               "192.168.1.10,10.0.0.10",
		"role":             "test",        // Direct label access
		"env":              "dev",         // Direct label access
		"labels.role":      "test",        // Explicit namespace
		"labels.env":       "dev",         // Explicit namespace
	}

	for key, want := range expected {
		got, exists := metadata[key]
		if !exists {
			t.Errorf("agentToMetadata() missing key %q", key)
			continue
		}
		if got != want {
			t.Errorf("agentToMetadata()[%q] = %q, want %q", key, got, want)
		}
	}

	// Check that no extra fields are present
	if len(metadata) != len(expected) {
		t.Errorf("agentToMetadata() returned %d fields, want %d", len(metadata), len(expected))
	}
}

func TestAgentToMetadata_NilMetadata(t *testing.T) {
	agent := &AgentInfo{
		ID:       "test-agent",
		Status:   pb.AgentStatus_AGENT_STATUS_ONLINE,
		Metadata: nil,
	}

	metadata := agentToMetadata(agent)

	// Should still have ID and status
	if metadata["id"] != "test-agent" {
		t.Errorf("agentToMetadata()[id] = %q, want %q", metadata["id"], "test-agent")
	}

	if metadata["status"] != "agent_status_online" {
		t.Errorf("agentToMetadata()[status] = %q, want %q", metadata["status"], "agent_status_online")
	}

	// Should only have those two fields
	if len(metadata) != 2 {
		t.Errorf("agentToMetadata() returned %d fields, want 2", len(metadata))
	}
}
