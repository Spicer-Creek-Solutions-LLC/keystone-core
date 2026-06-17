// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func (f *fakeClient) QuarantineAgent(ctx context.Context, in *v1.QuarantineAgentRequest, _ ...grpc.CallOption) (*v1.QuarantineAgentResponse, error) {
	return f.QuarantineFn(ctx, in)
}

func (f *fakeClient) UnquarantineAgent(ctx context.Context, in *v1.UnquarantineAgentRequest, _ ...grpc.CallOption) (*v1.UnquarantineAgentResponse, error) {
	return f.UnquarantineFn(ctx, in)
}

func TestQuarantineCmd_PassesIDAndReason(t *testing.T) {
	t.Parallel()
	var gotID, gotReason string
	fc := &fakeClient{
		QuarantineFn: func(_ context.Context, in *v1.QuarantineAgentRequest) (*v1.QuarantineAgentResponse, error) {
			gotID, gotReason = in.GetAgentId(), in.GetReason()
			return &v1.QuarantineAgentResponse{
				AgentId: in.GetAgentId(),
				Status:  v1.AgentStatus_AGENT_STATUS_DISABLED,
			}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"quarantine", "agent-9", "--reason", "compromised"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotID != "agent-9" || gotReason != "compromised" {
		t.Errorf("got id=%q reason=%q, want agent-9/compromised", gotID, gotReason)
	}
	out := buf.String()
	for _, want := range []string{"agent-9", "quarantined", "disabled"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestUnquarantineCmd_Success(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{
		UnquarantineFn: func(_ context.Context, in *v1.UnquarantineAgentRequest) (*v1.UnquarantineAgentResponse, error) {
			return &v1.UnquarantineAgentResponse{
				AgentId: in.GetAgentId(),
				Status:  v1.AgentStatus_AGENT_STATUS_CONNECTED,
			}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"unquarantine", "agent-9"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"agent-9", "unquarantined", "connected"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestQuarantineCmd_RequiresAgentID(t *testing.T) {
	t.Parallel()
	cmd := NewCommand(Deps{Dial: fakeDial(&fakeClient{})})
	cmd.SetArgs([]string{"quarantine"})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for missing agent-id arg")
	}
}

func TestQuarantineCmd_ServerErrorPropagates(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{
		QuarantineFn: func(_ context.Context, _ *v1.QuarantineAgentRequest) (*v1.QuarantineAgentResponse, error) {
			return nil, status.Error(codes.NotFound, "agent \"ghost\" not found")
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"quarantine", "ghost"})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v, want it to mention not found", err)
	}
}
