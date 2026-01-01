package nats

import (
	"testing"
)

func TestNewSubjectBuilder(t *testing.T) {
	tests := []struct {
		name            string
		cluster         string
		expectedCluster string
	}{
		{
			name:            "custom cluster",
			cluster:         "production",
			expectedCluster: "production",
		},
		{
			name:            "empty cluster uses default",
			cluster:         "",
			expectedCluster: DefaultCluster,
		},
		{
			name:            "multi-region cluster",
			cluster:         "us-east-1",
			expectedCluster: "us-east-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewSubjectBuilder(tt.cluster)
			if b.Cluster() != tt.expectedCluster {
				t.Errorf("Cluster() = %q, want %q", b.Cluster(), tt.expectedCluster)
			}
		})
	}
}

func TestAgentSubjects(t *testing.T) {
	b := NewSubjectBuilder("test-cluster")

	tests := []struct {
		name     string
		subject  string
		expected string
	}{
		{
			name:     "agent register",
			subject:  b.AgentRegister(),
			expected: "kscore.test-cluster.agent.register",
		},
		{
			name:     "agent heartbeat",
			subject:  b.AgentHeartbeat(),
			expected: "kscore.test-cluster.agent.heartbeat",
		},
		{
			name:     "agent command",
			subject:  b.AgentCommand("agent-123"),
			expected: "kscore.test-cluster.agent.agent-123.command",
		},
		{
			name:     "agent response",
			subject:  b.AgentResponse("agent-123"),
			expected: "kscore.test-cluster.agent.agent-123.response",
		},
		{
			name:     "agent state",
			subject:  b.AgentState("agent-123"),
			expected: "kscore.test-cluster.agent.agent-123.state",
		},
		{
			name:     "agent events",
			subject:  b.AgentEvents("agent-123"),
			expected: "kscore.test-cluster.agent.agent-123.events",
		},
		{
			name:     "agent wildcard",
			subject:  b.AgentWildcard(),
			expected: "kscore.test-cluster.agent.>",
		},
		{
			name:     "agent ID wildcard",
			subject:  b.AgentIDWildcard("agent-123"),
			expected: "kscore.test-cluster.agent.agent-123.*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.subject != tt.expected {
				t.Errorf("got %q, want %q", tt.subject, tt.expected)
			}
		})
	}
}

func TestServerSubjects(t *testing.T) {
	b := NewSubjectBuilder("prod")

	tests := []struct {
		name     string
		subject  string
		expected string
	}{
		{
			name:     "server announce",
			subject:  b.ServerAnnounce(),
			expected: "kscore.prod.server.announce",
		},
		{
			name:     "server control",
			subject:  b.ServerControl("server-1"),
			expected: "kscore.prod.server.server-1.control",
		},
		{
			name:     "server wildcard",
			subject:  b.ServerWildcard(),
			expected: "kscore.prod.server.>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.subject != tt.expected {
				t.Errorf("got %q, want %q", tt.subject, tt.expected)
			}
		})
	}
}

func TestBootstrapSubjects(t *testing.T) {
	b := NewSubjectBuilder("default")

	tests := []struct {
		name     string
		subject  string
		expected string
	}{
		{
			name:     "bootstrap register",
			subject:  b.BootstrapRegister("bootstrap-abc"),
			expected: "kscore.default.bootstrap.bootstrap-abc.register",
		},
		{
			name:     "bootstrap response",
			subject:  b.BootstrapResponse("bootstrap-abc"),
			expected: "kscore.default.bootstrap.bootstrap-abc.response",
		},
		{
			name:     "bootstrap wildcard",
			subject:  b.BootstrapWildcard(),
			expected: "kscore.default.bootstrap.>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.subject != tt.expected {
				t.Errorf("got %q, want %q", tt.subject, tt.expected)
			}
		})
	}
}

func TestDiscoverySubject(t *testing.T) {
	b := NewSubjectBuilder("edge")
	expected := "kscore.edge.discovery"
	if got := b.Discovery(); got != expected {
		t.Errorf("Discovery() = %q, want %q", got, expected)
	}
}

func TestCommandResponseSubject(t *testing.T) {
	b := NewSubjectBuilder("default")
	expected := "kscore.default.command.cmd-456.response"
	if got := b.CommandResponse("cmd-456"); got != expected {
		t.Errorf("CommandResponse() = %q, want %q", got, expected)
	}
}

func TestParseSubject(t *testing.T) {
	tests := []struct {
		name     string
		subject  string
		expected ParsedSubject
	}{
		{
			name:    "agent register",
			subject: "kscore.prod.agent.register",
			expected: ParsedSubject{
				Cluster:   "prod",
				Category:  "agent",
				Operation: "register",
				IsValid:   true,
			},
		},
		{
			name:    "agent command with ID",
			subject: "kscore.prod.agent.agent-123.command",
			expected: ParsedSubject{
				Cluster:   "prod",
				Category:  "agent",
				EntityID:  "agent-123",
				Operation: "command",
				IsValid:   true,
			},
		},
		{
			name:    "server announce",
			subject: "kscore.default.server.announce",
			expected: ParsedSubject{
				Cluster:   "default",
				Category:  "server",
				Operation: "announce",
				IsValid:   true,
			},
		},
		{
			name:    "discovery",
			subject: "kscore.us-east.discovery",
			expected: ParsedSubject{
				Cluster:  "us-east",
				Category: "discovery",
				IsValid:  true,
			},
		},
		{
			name:    "invalid - no prefix",
			subject: "other.something",
			expected: ParsedSubject{
				IsValid: false,
			},
		},
		{
			name:    "invalid - too short",
			subject: "kscore.cluster",
			expected: ParsedSubject{
				IsValid: false,
			},
		},
		{
			name:    "bootstrap register",
			subject: "kscore.prod.bootstrap.boot-123.register",
			expected: ParsedSubject{
				Cluster:   "prod",
				Category:  "bootstrap",
				EntityID:  "boot-123",
				Operation: "register",
				IsValid:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSubject(tt.subject)
			if got.IsValid != tt.expected.IsValid {
				t.Errorf("IsValid = %v, want %v", got.IsValid, tt.expected.IsValid)
			}
			if got.Cluster != tt.expected.Cluster {
				t.Errorf("Cluster = %q, want %q", got.Cluster, tt.expected.Cluster)
			}
			if got.Category != tt.expected.Category {
				t.Errorf("Category = %q, want %q", got.Category, tt.expected.Category)
			}
			if got.EntityID != tt.expected.EntityID {
				t.Errorf("EntityID = %q, want %q", got.EntityID, tt.expected.EntityID)
			}
			if got.Operation != tt.expected.Operation {
				t.Errorf("Operation = %q, want %q", got.Operation, tt.expected.Operation)
			}
		})
	}
}

func TestValidateSubject(t *testing.T) {
	tests := []struct {
		name      string
		subject   string
		expectErr bool
	}{
		{
			name:      "valid agent register",
			subject:   "kscore.prod.agent.register",
			expectErr: false,
		},
		{
			name:      "valid agent command",
			subject:   "kscore.prod.agent.agent-123.command",
			expectErr: false,
		},
		{
			name:      "valid server announce",
			subject:   "kscore.default.server.announce",
			expectErr: false,
		},
		{
			name:      "valid discovery",
			subject:   "kscore.us-east.discovery",
			expectErr: false,
		},
		{
			name:      "valid wildcard subscription",
			subject:   "kscore.prod.agent.>",
			expectErr: false,
		},
		{
			name:      "empty subject",
			subject:   "",
			expectErr: true,
		},
		{
			name:      "missing prefix",
			subject:   "other.prod.agent.register",
			expectErr: true,
		},
		{
			name:      "too short",
			subject:   "kscore.prod",
			expectErr: true,
		},
		{
			name:      "wildcard cluster",
			subject:   "kscore.*.agent.register",
			expectErr: true,
		},
		{
			name:      "unknown category",
			subject:   "kscore.prod.unknown.something",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSubject(tt.subject)
			if tt.expectErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateSubjectForPublish(t *testing.T) {
	tests := []struct {
		name      string
		subject   string
		expectErr bool
	}{
		{
			name:      "valid publish subject",
			subject:   "kscore.prod.agent.register",
			expectErr: false,
		},
		{
			name:      "wildcard not allowed",
			subject:   "kscore.prod.agent.>",
			expectErr: true,
		},
		{
			name:      "single wildcard not allowed",
			subject:   "kscore.prod.agent.*.command",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSubjectForPublish(tt.subject)
			if tt.expectErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBootstrapPermissions(t *testing.T) {
	perms := BootstrapPermissions("prod", "bootstrap-123")

	// Should only be able to publish to registration
	if len(perms.Publish) != 1 {
		t.Errorf("expected 1 publish permission, got %d", len(perms.Publish))
	}
	if perms.Publish[0] != "kscore.prod.agent.register" {
		t.Errorf("unexpected publish permission: %s", perms.Publish[0])
	}

	// Should only be able to subscribe to own response
	if len(perms.Subscribe) != 1 {
		t.Errorf("expected 1 subscribe permission, got %d", len(perms.Subscribe))
	}
	if perms.Subscribe[0] != "kscore.prod.bootstrap.bootstrap-123.response" {
		t.Errorf("unexpected subscribe permission: %s", perms.Subscribe[0])
	}
}

func TestAgentPermissions(t *testing.T) {
	perms := AgentPermissions("prod", "agent-456")

	// Should have 3 publish permissions
	if len(perms.Publish) != 3 {
		t.Errorf("expected 3 publish permissions, got %d", len(perms.Publish))
	}

	expectedPublish := map[string]bool{
		"kscore.prod.agent.heartbeat":         true,
		"kscore.prod.agent.agent-456.response": true,
		"kscore.prod.agent.agent-456.events":   true,
	}
	for _, p := range perms.Publish {
		if !expectedPublish[p] {
			t.Errorf("unexpected publish permission: %s", p)
		}
	}

	// Should have 2 subscribe permissions
	if len(perms.Subscribe) != 2 {
		t.Errorf("expected 2 subscribe permissions, got %d", len(perms.Subscribe))
	}

	expectedSubscribe := map[string]bool{
		"kscore.prod.agent.agent-456.command": true,
		"kscore.prod.agent.agent-456.state":   true,
	}
	for _, s := range perms.Subscribe {
		if !expectedSubscribe[s] {
			t.Errorf("unexpected subscribe permission: %s", s)
		}
	}
}

func TestServerPermissions(t *testing.T) {
	perms := ServerPermissions("prod", "server-1")

	// Server should have broad permissions
	if len(perms.Publish) < 2 {
		t.Errorf("expected at least 2 publish permissions, got %d", len(perms.Publish))
	}

	if len(perms.Subscribe) < 4 {
		t.Errorf("expected at least 4 subscribe permissions, got %d", len(perms.Subscribe))
	}
}

func TestSubjectBuilderWithDifferentClusters(t *testing.T) {
	clusters := []string{"default", "us-east-1", "eu-west-1", "production", "staging"}

	for _, cluster := range clusters {
		t.Run(cluster, func(t *testing.T) {
			b := NewSubjectBuilder(cluster)

			// All subjects should contain the cluster name
			subjects := []string{
				b.AgentRegister(),
				b.AgentHeartbeat(),
				b.AgentCommand("agent-1"),
				b.ServerAnnounce(),
				b.Discovery(),
			}

			for _, s := range subjects {
				if !contains(s, "."+cluster+".") {
					t.Errorf("subject %q does not contain cluster %q", s, cluster)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
