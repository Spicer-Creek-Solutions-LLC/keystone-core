// Smoke tests for the pkg/api/v1 generated proto code.
//
// These exist to close the loop on `make proto` end-to-end (epic 03
// task 3): the buf codegen pipeline produces files, those files
// compile, and the resulting types are functionally usable as proto
// messages. Tests of specific message semantics belong with the
// consumers (auth, control plane, etc.) — this file is a smoke layer
// for the contract that "the generated code works at all."

package v1_test

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// Compile-time assertions: every generated message type satisfies
// proto.Message. A representative sample across the 9 generated files
// catches the case where a future codegen change drops the interface.
var (
	_ proto.Message = (*v1.Agent)(nil)
	_ proto.Message = (*v1.Command)(nil)
	_ proto.Message = (*v1.BatchJob)(nil)
	_ proto.Message = (*v1.BatchAgentResult)(nil)
	_ proto.Message = (*v1.Principal)(nil)
	_ proto.Message = (*v1.RegisterRequest)(nil)
	_ proto.Message = (*v1.HeartbeatResponse)(nil)
	_ proto.Message = (*v1.ServerStatusResponse)(nil)
	_ proto.Message = (*v1.ListAgentsRequest)(nil)
	_ proto.Message = (*v1.ApplyStateResponse)(nil)
	_ proto.Message = (*v1.SubscribeEventsResponse)(nil)
	_ proto.Message = (*v1.EvaluationResult)(nil)
	_ proto.Message = (*v1.SecretMetadata)(nil)
	_ proto.Message = (*v1.ClusterMember)(nil)
	_ proto.Message = (*v1.ClusterHealthResponse)(nil)
)

func TestAgent_RoundTrip(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	want := &v1.Agent{
		Id:              "agent-1",
		Hostname:        "host-1",
		Os:              "linux",
		Architecture:    "amd64",
		IpAddresses:     []string{"10.0.0.1", "fe80::1"},
		PlatformVersion: "Ubuntu 24.04",
		AgentVersion:    "0.0.1",
		Labels:          map[string]string{"role": "web", "env": "prod"},
		Status:          v1.AgentStatus_AGENT_STATUS_CONNECTED,
		RegisteredAt:    timestamppb.New(now),
		LastHeartbeatAt: timestamppb.New(now.Add(5 * time.Minute)),
		Metrics:         map[string]string{"load": "0.42"},
	}

	wire, err := proto.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(wire) == 0 {
		t.Fatal("marshaled output is empty")
	}

	got := &v1.Agent{}
	if err := proto.Unmarshal(wire, got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(want, got) {
		t.Errorf("round-trip mismatch:\n  want: %+v\n  got:  %+v", want, got)
	}

	// Spot-check individual fields to fail with clearer messages than
	// proto.Equal's bool result if any specific field regresses.
	if got.GetId() != want.Id {
		t.Errorf("Id: %q, want %q", got.GetId(), want.Id)
	}
	if got.GetStatus() != v1.AgentStatus_AGENT_STATUS_CONNECTED {
		t.Errorf("Status: %v", got.GetStatus())
	}
	if !got.GetRegisteredAt().AsTime().Equal(now) {
		t.Errorf("RegisteredAt: %v, want %v", got.GetRegisteredAt().AsTime(), now)
	}
	if got.GetLabels()["role"] != "web" {
		t.Errorf("Labels[role]: %q", got.GetLabels()["role"])
	}
}

func TestCommand_RoundTrip(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	want := &v1.Command{
		Id:             "cmd-1",
		AgentId:        "agent-1",
		Command:        "uptime",
		Args:           []string{"-p"},
		Env:            map[string]string{"FOO": "bar"},
		WorkingDir:     "/tmp",
		User:           "root",
		TimeoutSeconds: 30,
		Status:         v1.CommandStatus_COMMAND_STATUS_COMPLETED,
		ExitCode:       0,
		Stdout:         "up 5 days",
		Stderr:         "",
		StartedAt:      timestamppb.New(now),
		CompletedAt:    timestamppb.New(now.Add(time.Second)),
	}

	wire, err := proto.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	got := &v1.Command{}
	if err := proto.Unmarshal(wire, got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !proto.Equal(want, got) {
		t.Errorf("round-trip mismatch:\n  want: %+v\n  got:  %+v", want, got)
	}
}

// TestEnumValuesPresent guards against silent removal of expected enum
// constants in a future codegen regression.
func TestEnumValuesPresent(t *testing.T) {
	tests := []struct {
		name string
		got  int32
		want int32
	}{
		{"AgentStatus_PENDING", int32(v1.AgentStatus_AGENT_STATUS_PENDING), 1},
		{"AgentStatus_CONNECTED", int32(v1.AgentStatus_AGENT_STATUS_CONNECTED), 2},
		{"CommandStatus_RUNNING", int32(v1.CommandStatus_COMMAND_STATUS_RUNNING), 2},
		{"BatchJobStatus_PARTIAL", int32(v1.BatchJobStatus_BATCH_JOB_STATUS_PARTIAL), 5},
		{"AuthMethod_API_KEY", int32(v1.AuthMethod_AUTH_METHOD_API_KEY), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
			}
		})
	}
}
