package nats

import (
	"strings"
	"testing"
)

func TestNewSubjectBuilder_RejectsBadCluster(t *testing.T) {
	tests := []struct {
		name    string
		cluster string
		wantErr string
	}{
		{"empty", "", "must not be empty"},
		{"dot", "prod.east", "forbidden character"},
		{"star", "prod*", "forbidden character"},
		{"angle", "prod>", "forbidden character"},
		{"space", "prod east", "forbidden character"},
		{"tab", "prod\teast", "whitespace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSubjectBuilder(tt.cluster)
			if err == nil {
				t.Fatalf("expected error for cluster=%q", tt.cluster)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestNewSubjectBuilder_AcceptsValid(t *testing.T) {
	for _, cluster := range []string{"default", "prod-east", "test_1", "alpha123"} {
		b, err := NewSubjectBuilder(cluster)
		if err != nil {
			t.Errorf("NewSubjectBuilder(%q) = %v", cluster, err)
			continue
		}
		if b.Cluster() != cluster {
			t.Errorf("Cluster() = %q, want %q", b.Cluster(), cluster)
		}
		if b.Prefix() != "kscore."+cluster {
			t.Errorf("Prefix() = %q, want kscore.%s", b.Prefix(), cluster)
		}
	}
}

func TestSubjectBuilder_Patterns(t *testing.T) {
	b, err := NewSubjectBuilder("prod")
	if err != nil {
		t.Fatalf("NewSubjectBuilder: %v", err)
	}
	tests := []struct {
		got, want string
	}{
		{b.AgentRegister(), "kscore.prod.agent.register"},
		{b.AgentHeartbeat(), "kscore.prod.agent.heartbeat"},
		{b.AgentCommand("agent-7"), "kscore.prod.agent.agent-7.command"},
		{b.AgentResponse("agent-7"), "kscore.prod.agent.agent-7.response"},
		{b.AgentState("agent-7"), "kscore.prod.agent.agent-7.state"},
		{b.AgentEvents("agent-7"), "kscore.prod.agent.agent-7.events"},
		{b.ServerAnnounce(), "kscore.prod.server.announce"},
		{b.ServerControl(), "kscore.prod.server.control"},
		{b.BootstrapRegister("agent-7"), "kscore.prod.bootstrap.agent-7.register"},
		{b.BootstrapResponse("agent-7"), "kscore.prod.bootstrap.agent-7.response"},
		{b.Discovery(), "kscore.prod.discovery"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}

func TestSubjectBuilder_Validate(t *testing.T) {
	b, err := NewSubjectBuilder("prod")
	if err != nil {
		t.Fatalf("NewSubjectBuilder: %v", err)
	}

	tests := []struct {
		name    string
		subject string
		wantErr string
	}{
		{"valid root", "kscore.prod", ""},
		{"valid prefixed", "kscore.prod.agent.register", ""},
		{"valid with id", "kscore.prod.agent.x-1.command", ""},
		{"empty rejected", "", "empty"},
		{"missing prefix", "kscore.other.agent.register", "must start with"},
		{"different root", "ksdev.prod.x", "must start with"},
		{"prefix-only-no-dot looks valid", "kscore.prod", ""},
		{"adjacent prefix is not a child", "kscore.production", "must start with"},
		{"contains star", "kscore.prod.agent.*", "wildcard"},
		{"contains gt", "kscore.prod.agent.>", "wildcard"},
		{"contains space", "kscore.prod.agent foo", "non-printable"},
		{"contains tab", "kscore.prod.agent\tfoo", "non-printable"},
		{"contains newline", "kscore.prod.agent\nfoo", "non-printable"},
		{"contains null byte", "kscore.prod.agent\x00foo", "non-printable"},
		{"contains DEL", "kscore.prod.agent\x7ffoo", "non-printable"},
		{"contains high bit", "kscore.prod.agent\x80foo", "non-printable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := b.Validate(tt.subject)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate(%q) = %v, want nil", tt.subject, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate(%q) = nil, want error containing %q", tt.subject, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestSubjectBuilder_AllPatternsValidate(t *testing.T) {
	// Every typed constructor must produce a subject that Validate
	// accepts. Catches drift if a future contributor renames the
	// prefix in one place but not another.
	b, err := NewSubjectBuilder("default")
	if err != nil {
		t.Fatalf("NewSubjectBuilder: %v", err)
	}
	produced := []string{
		b.AgentRegister(),
		b.AgentHeartbeat(),
		b.AgentCommand("a1"),
		b.AgentResponse("a1"),
		b.AgentState("a1"),
		b.AgentEvents("a1"),
		b.ServerAnnounce(),
		b.ServerControl(),
		b.BootstrapRegister("a1"),
		b.BootstrapResponse("a1"),
		b.Discovery(),
	}
	for _, s := range produced {
		if err := b.Validate(s); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", s, err)
		}
	}
}
