// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"bytes"
	"context"
	"strings"
	"testing"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func TestVerifyCmd_SingleOK(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{
		VerifyFn: func(_ context.Context, in *v1.VerifyAgentRequest) (*v1.VerifyAgentResponse, error) {
			return &v1.VerifyAgentResponse{
				AgentId: in.GetAgentId(), HasCert: true, ChainValid: true,
				SpiffeMatch: true, Ok: true, SpiffeId: "spiffe://kscore.local/agent/" + in.GetAgentId(),
			}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"verify", "agent-7"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"AGENT-ID", "agent-7", "true"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %q in:\n%s", want, buf.String())
		}
	}
}

func TestVerifyCmd_FailingAgentExitsNonZero(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{
		VerifyFn: func(_ context.Context, in *v1.VerifyAgentRequest) (*v1.VerifyAgentResponse, error) {
			return &v1.VerifyAgentResponse{
				AgentId: in.GetAgentId(), HasCert: true, ChainValid: true,
				Expired: true, Ok: false,
			}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"verify", "agent-7"})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "failed certificate verification") {
		t.Fatalf("err = %v, want a verification-failure error", err)
	}
}

func TestVerifyCmd_NoCertDoesNotFail(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{
		VerifyFn: func(_ context.Context, in *v1.VerifyAgentRequest) (*v1.VerifyAgentResponse, error) {
			return &v1.VerifyAgentResponse{AgentId: in.GetAgentId(), HasCert: false, Ok: false}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"verify", "agent-nocert"})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("has_cert=false should not be a failure, got: %v", err)
	}
}

func TestVerifyCmd_All(t *testing.T) {
	t.Parallel()
	var verified []string
	fc := &fakeClient{
		ListAgentsFn: func(_ context.Context, _ *v1.ListAgentsRequest) (*v1.ListAgentsResponse, error) {
			return &v1.ListAgentsResponse{Agents: []*v1.Agent{{Id: "a1"}, {Id: "a2"}}}, nil
		},
		VerifyFn: func(_ context.Context, in *v1.VerifyAgentRequest) (*v1.VerifyAgentResponse, error) {
			verified = append(verified, in.GetAgentId())
			return &v1.VerifyAgentResponse{AgentId: in.GetAgentId(), HasCert: true, Ok: true, ChainValid: true, SpiffeMatch: true}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"verify", "--all"})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(verified) != 2 || verified[0] != "a1" || verified[1] != "a2" {
		t.Errorf("verified = %v, want [a1 a2]", verified)
	}
}

func TestVerifyCmd_ArgValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"neither", []string{"verify"}},
		{"both", []string{"verify", "a1", "--all"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := NewCommand(Deps{Dial: fakeDial(&fakeClient{})})
			cmd.SetArgs(tc.args)
			cmd.SetOut(new(bytes.Buffer))
			cmd.SetErr(new(bytes.Buffer))
			if err := cmd.Execute(); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}
