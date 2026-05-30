// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func TestListCmd_TableOutput(t *testing.T) {
	t.Parallel()
	hb := timestamppb.New(time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC))
	fc := &fakeClient{
		ListAgentsFn: func(_ context.Context, _ *v1.ListAgentsRequest) (*v1.ListAgentsResponse, error) {
			return &v1.ListAgentsResponse{Agents: []*v1.Agent{{
				Id:              "agent-1",
				Hostname:        "node-a",
				Os:              "linux",
				Status:          v1.AgentStatus_AGENT_STATUS_CONNECTED,
				Labels:          map[string]string{"env": "prod", "zone": "us"},
				LastHeartbeatAt: hb,
			}}}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"list"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"ID", "STATUS", "HOSTNAME", "OS", "LAST-HEARTBEAT", "LABELS",
		"agent-1", "node-a", "linux", "connected", "env=prod,zone=us", "2026-05-30T12:00:00Z"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestListCmd_JSONOutput(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{
		ListAgentsFn: func(_ context.Context, _ *v1.ListAgentsRequest) (*v1.ListAgentsResponse, error) {
			return &v1.ListAgentsResponse{
				Agents:     []*v1.Agent{{Id: "agent-x", Hostname: "n"}},
				TotalCount: 1,
			}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"list", "--output", "json"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := buf.String()
	for _, want := range []string{`"agents"`, `"agent-x"`, `"totalCount"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestListCmd_StatusFlagMapsToRequest(t *testing.T) {
	t.Parallel()
	cases := []struct {
		flag string
		want v1.AgentStatus
	}{
		{"pending", v1.AgentStatus_AGENT_STATUS_PENDING},
		{"connected", v1.AgentStatus_AGENT_STATUS_CONNECTED},
		{"stale", v1.AgentStatus_AGENT_STATUS_STALE},
		{"disabled", v1.AgentStatus_AGENT_STATUS_DISABLED},
		{"CONNECTED", v1.AgentStatus_AGENT_STATUS_CONNECTED}, // case-insensitive
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.flag, func(t *testing.T) {
			t.Parallel()
			var got *v1.ListAgentsRequest
			fc := &fakeClient{
				ListAgentsFn: func(_ context.Context, in *v1.ListAgentsRequest) (*v1.ListAgentsResponse, error) {
					got = in
					return &v1.ListAgentsResponse{}, nil
				},
			}
			cmd := NewCommand(Deps{Dial: fakeDial(fc)})
			cmd.SetArgs([]string{"list", "--status", tc.flag})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if got == nil {
				t.Fatal("ListAgents not called")
			}
			if got.GetStatus() != tc.want {
				t.Errorf("Status = %v, want %v", got.GetStatus(), tc.want)
			}
		})
	}
}

func TestListCmd_LabelFlagMapsToRequest(t *testing.T) {
	t.Parallel()
	var got *v1.ListAgentsRequest
	fc := &fakeClient{
		ListAgentsFn: func(_ context.Context, in *v1.ListAgentsRequest) (*v1.ListAgentsResponse, error) {
			got = in
			return &v1.ListAgentsResponse{}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"list", "--label", "env=prod"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.GetLabelKey() != "env" || got.GetLabelValue() != "prod" {
		t.Errorf("LabelKey/Value = %q/%q, want env/prod", got.GetLabelKey(), got.GetLabelValue())
	}
}

func TestListCmd_LimitFlagMapsToPageSize(t *testing.T) {
	t.Parallel()
	var got *v1.ListAgentsRequest
	fc := &fakeClient{
		ListAgentsFn: func(_ context.Context, in *v1.ListAgentsRequest) (*v1.ListAgentsResponse, error) {
			got = in
			return &v1.ListAgentsResponse{}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"list", "--limit", "7"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.GetPageSize() != 7 {
		t.Errorf("PageSize = %d, want 7", got.GetPageSize())
	}
}

func TestListCmd_InvalidStatusReturnsError(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{
		ListAgentsFn: func(_ context.Context, _ *v1.ListAgentsRequest) (*v1.ListAgentsResponse, error) {
			t.Fatal("ListAgents must not be called when --status validation fails")
			return nil, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"list", "--status", "bogus"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown --status") {
		t.Fatalf("err = %v, want unknown --status", err)
	}
}

func TestListCmd_InvalidLabelReturnsError(t *testing.T) {
	t.Parallel()
	cases := []string{"no-equals", "=missing-key", "missing-value="}
	for _, in := range cases {
		in := in
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			fc := &fakeClient{
				ListAgentsFn: func(_ context.Context, _ *v1.ListAgentsRequest) (*v1.ListAgentsResponse, error) {
					t.Fatal("ListAgents must not be called when --label validation fails")
					return nil, nil
				},
			}
			cmd := NewCommand(Deps{Dial: fakeDial(fc)})
			cmd.SetArgs([]string{"list", "--label", in})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "must be key=value") {
				t.Fatalf("err = %v, want key=value validation error", err)
			}
		})
	}
}

func TestListCmd_InvalidOutputReturnsError(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"list", "--output", "yaml"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid --output") {
		t.Fatalf("err = %v, want invalid --output", err)
	}
}

func TestListCmd_RPCErrorWrapped(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{
		ListAgentsFn: func(_ context.Context, _ *v1.ListAgentsRequest) (*v1.ListAgentsResponse, error) {
			return nil, errors.New("boom")
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"list"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "agent list:") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want wrapped boom", err)
	}
}

func TestListCmd_AuthHeaderAttachedFromFlag(t *testing.T) {
	t.Parallel()
	var sawCtx context.Context
	fc := &fakeClient{
		ListAgentsFn: func(ctx context.Context, _ *v1.ListAgentsRequest) (*v1.ListAgentsResponse, error) {
			sawCtx = ctx
			return &v1.ListAgentsResponse{}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"list", "--api-key", "secret-token"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if sawCtx == nil {
		t.Fatal("ListAgents context not captured")
	}
	md, ok := metadata.FromOutgoingContext(sawCtx)
	if !ok {
		t.Fatal("no outgoing metadata on captured context")
	}
	got := md.Get("authorization")
	if len(got) == 0 || !strings.Contains(got[0], "Bearer secret-token") {
		t.Errorf("authorization header = %v, want Bearer secret-token", got)
	}
}
