// SPDX-License-Identifier: Apache-2.0

package targeting

import (
	"reflect"
	"testing"

	"go.keystone-core.io/keystone-core/internal/state"
)

func TestFlatten_Populated(t *testing.T) {
	t.Parallel()

	rec := state.AgentRecord{
		ID:           "agent-01",
		Hostname:     "web-prod-01",
		OS:           "linux",
		Architecture: "amd64",
		IPAddresses:  []string{"10.0.1.5", "fe80::1"},
		Labels:       map[string]string{"role": "web", "env": "prod"},
		Status:       state.AgentStatusConnected,
	}
	got := Flatten(rec)

	want := map[string]any{
		"id":       "agent-01",
		"hostname": "web-prod-01",
		"os":       "linux",
		"arch":     "amd64",
		"status":   "online",
		"ip":       []string{"10.0.1.5", "fe80::1"},
		"labels":   map[string]string{"role": "web", "env": "prod"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Flatten()\n  got:  %#v\n  want: %#v", got, want)
	}
}

func TestFlatten_Empty(t *testing.T) {
	t.Parallel()

	got := Flatten(state.AgentRecord{})

	if got["ip"] == nil {
		t.Error("ip should be []string{}, got nil")
	}
	if got["labels"] == nil {
		t.Error("labels should be map[string]string{}, got nil")
	}
	if got["status"] != "" {
		t.Errorf("empty status passthrough: got %q, want empty string", got["status"])
	}
	if _, ok := got["ip"].([]string); !ok {
		t.Errorf("ip type = %T, want []string", got["ip"])
	}
	if _, ok := got["labels"].(map[string]string); !ok {
		t.Errorf("labels type = %T, want map[string]string", got["labels"])
	}
}

func TestFlatten_StatusMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   state.AgentStatus
		want string
	}{
		{state.AgentStatusConnected, "online"},
		{state.AgentStatusStale, "stale"},
		{state.AgentStatusPending, "pending"},
		{state.AgentStatusDisabled, "disabled"},
		{state.AgentStatus("unknown"), "unknown"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.in), func(t *testing.T) {
			t.Parallel()
			got := Flatten(state.AgentRecord{Status: tc.in})
			if got["status"] != tc.want {
				t.Errorf("status %q -> %q, want %q", tc.in, got["status"], tc.want)
			}
		})
	}
}
