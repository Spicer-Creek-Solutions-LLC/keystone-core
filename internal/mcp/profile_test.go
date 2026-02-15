package mcp

import "testing"

func TestParseProfile(t *testing.T) {
	tests := []struct {
		input   string
		want    Profile
		wantErr bool
	}{
		{"read_only", ProfileReadOnly, false},
		{"ops_safe", ProfileOpsSafe, false},
		{"ops_admin", ProfileOpsAdmin, false},
		{"invalid", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		got, err := ParseProfile(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseProfile(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseProfile(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestProfile_Allows(t *testing.T) {
	tests := []struct {
		profile Profile
		tool    string
		want    bool
	}{
		// read_only allows reads
		{ProfileReadOnly, "agent_list", true},
		{ProfileReadOnly, "agent_show", true},
		{ProfileReadOnly, "event_query", true},
		{ProfileReadOnly, "cluster_status", true},
		// read_only denies writes
		{ProfileReadOnly, "exec_run", false},
		{ProfileReadOnly, "state_apply", false},
		{ProfileReadOnly, "runbook_execute", false},

		// ops_safe allows exec but not state_apply
		{ProfileOpsSafe, "exec_run", true},
		{ProfileOpsSafe, "agent_list", true},
		{ProfileOpsSafe, "runbook_execute", true},
		{ProfileOpsSafe, "state_apply", false},

		// ops_admin allows everything
		{ProfileOpsAdmin, "exec_run", true},
		{ProfileOpsAdmin, "state_apply", true},
		{ProfileOpsAdmin, "agent_list", true},

		// unknown tool
		{ProfileReadOnly, "unknown_tool", false},
		{ProfileOpsAdmin, "unknown_tool", false},
	}
	for _, tt := range tests {
		got := tt.profile.Allows(tt.tool)
		if got != tt.want {
			t.Errorf("%s.Allows(%q) = %v, want %v", tt.profile, tt.tool, got, tt.want)
		}
	}
}

func TestProfile_Allows_InvalidProfile(t *testing.T) {
	p := Profile("bogus")
	if p.Allows("agent_list") {
		t.Error("invalid profile should not allow any tool")
	}
}
