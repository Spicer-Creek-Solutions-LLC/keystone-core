package tools

import (
	"testing"

	mcpserver "github.com/shawnbutts/keystone-core/internal/mcp"
)

func TestRegisterAll(t *testing.T) {
	client := testClient(nil, nil, nil, "http://localhost")
	registrars := RegisterAll(client, 100)

	// 6 registrars: agents, exec, cluster, state, events, runbooks
	if len(registrars) != 6 {
		t.Errorf("expected 6 registrars, got %d", len(registrars))
	}

	// Verify none are nil
	for i, r := range registrars {
		if r == nil {
			t.Errorf("registrar[%d] is nil", i)
		}
	}
}

func TestRegisterAll_ProfileFiltering(t *testing.T) {
	tests := []struct {
		profile     mcpserver.Profile
		wantTools   []string
		dontWant    []string
	}{
		{
			profile: mcpserver.ProfileReadOnly,
			wantTools: []string{
				"agent_list", "agent_show", "agent_health",
				"exec_status", "exec_history",
				"state_check", "state_drift", "state_history",
				"event_query", "event_stats",
				"runbook_list", "runbook_status",
				"cluster_status",
			},
			dontWant: []string{
				"exec_run",
				"state_apply",
				"runbook_execute",
			},
		},
		{
			profile: mcpserver.ProfileOpsAdmin,
			wantTools: []string{
				"agent_list", "exec_run", "state_apply",
				"runbook_execute", "cluster_status",
			},
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.profile), func(t *testing.T) {
			for _, name := range tt.wantTools {
				if !tt.profile.Allows(name) {
					t.Errorf("profile %s should allow %s", tt.profile, name)
				}
			}
			for _, name := range tt.dontWant {
				if tt.profile.Allows(name) {
					t.Errorf("profile %s should not allow %s", tt.profile, name)
				}
			}
		})
	}
}
