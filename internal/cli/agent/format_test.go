// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func TestValidateOutput(t *testing.T) {
	t.Parallel()
	for _, in := range []string{FormatTable, FormatJSON} {
		if err := validateOutput(in); err != nil {
			t.Errorf("validateOutput(%q) = %v, want nil", in, err)
		}
	}
	if err := validateOutput("yaml"); err == nil {
		t.Error("validateOutput(yaml) = nil, want error")
	}
}

func TestStatusName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   v1.AgentStatus
		want string
	}{
		{v1.AgentStatus_AGENT_STATUS_PENDING, "pending"},
		{v1.AgentStatus_AGENT_STATUS_CONNECTED, "connected"},
		{v1.AgentStatus_AGENT_STATUS_STALE, "stale"},
		{v1.AgentStatus_AGENT_STATUS_DISABLED, "disabled"},
		{v1.AgentStatus_AGENT_STATUS_UNSPECIFIED, "unspecified"},
	}
	for _, tc := range cases {
		if got := statusName(tc.in); got != tc.want {
			t.Errorf("statusName(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatLabels(t *testing.T) {
	t.Parallel()
	if got := formatLabels(nil); got != "-" {
		t.Errorf("nil labels = %q, want -", got)
	}
	if got := formatLabels(map[string]string{}); got != "-" {
		t.Errorf("empty labels = %q, want -", got)
	}
	// Multiple keys must render in deterministic alphabetical order.
	got := formatLabels(map[string]string{"zone": "us", "env": "prod", "app": "k"})
	if got != "app=k,env=prod,zone=us" {
		t.Errorf("got %q, want app=k,env=prod,zone=us", got)
	}
}

func TestRenderAgentList_EmptyTable(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := renderAgentList(&buf, &v1.ListAgentsResponse{}, FormatTable); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	// Header still emitted on empty list.
	for _, want := range []string{"ID", "STATUS", "HOSTNAME", "LAST-HEARTBEAT"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing header %q in:\n%s", want, out)
		}
	}
}

func TestRenderAgentList_TablePopulated(t *testing.T) {
	t.Parallel()
	hb := timestamppb.New(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	resp := &v1.ListAgentsResponse{Agents: []*v1.Agent{
		{Id: "a-1", Hostname: "h1", Os: "linux", Status: v1.AgentStatus_AGENT_STATUS_CONNECTED,
			Labels: map[string]string{"env": "dev"}, LastHeartbeatAt: hb},
		{Id: "a-2", Hostname: "h2", Os: "darwin", Status: v1.AgentStatus_AGENT_STATUS_STALE},
	}}
	var buf bytes.Buffer
	if err := renderAgentList(&buf, resp, FormatTable); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"a-1", "h1", "linux", "connected", "env=dev", "2026-01-02T03:04:05Z",
		"a-2", "h2", "darwin", "stale", "-"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderAgentList_JSON(t *testing.T) {
	t.Parallel()
	resp := &v1.ListAgentsResponse{
		Agents:     []*v1.Agent{{Id: "agent-1", Hostname: "n"}},
		TotalCount: 1,
	}
	var buf bytes.Buffer
	if err := renderAgentList(&buf, resp, FormatJSON); err != nil {
		t.Fatalf("render: %v", err)
	}
	// Must be valid JSON.
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if _, ok := got["agents"]; !ok {
		t.Errorf("missing agents key in:\n%s", buf.String())
	}
}

func TestRenderAgentList_InvalidFormatErrors(t *testing.T) {
	t.Parallel()
	err := renderAgentList(&bytes.Buffer{}, &v1.ListAgentsResponse{}, "yaml")
	if err == nil {
		t.Error("expected error for unsupported format")
	}
}
