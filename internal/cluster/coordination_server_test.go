package cluster

import (
	"context"
	"testing"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

func TestNewCoordinationServer(t *testing.T) {
	tests := []struct {
		name      string
		config    *CoordinationServerConfig
		wantError bool
	}{
		{
			name:      "nil config",
			config:    nil,
			wantError: true,
		},
		{
			name:      "empty server ID",
			config:    &CoordinationServerConfig{},
			wantError: true,
		},
		{
			name: "valid config",
			config: &CoordinationServerConfig{
				ServerID: "server-1",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// NewCoordinationServer requires a non-nil membership manager for valid configs
			_, err := NewCoordinationServer(tt.config, nil, nil, nil, nil)
			if tt.wantError && err == nil {
				t.Error("expected error, got nil")
			}
			// Valid config with nil membership should also error
			if !tt.wantError && err == nil {
				t.Error("expected error for nil membership, got nil")
			}
		})
	}
}

func TestCoordinationServer_Heartbeat(t *testing.T) {
	// Create a minimal server for heartbeat testing
	server := &CoordinationServer{
		serverID:      "server-1",
		recoveryState: pb.RecoveryState_RECOVERY_STATE_IDLE,
		startTime:     time.Now(),
	}

	ctx := context.Background()
	req := &pb.ServerHeartbeatRequest{
		SenderId: "server-2",
		Sequence: 1,
	}

	resp, err := server.Heartbeat(ctx, req)
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	if resp.ResponderId != "server-1" {
		t.Errorf("expected responder_id=server-1, got %s", resp.ResponderId)
	}

	if resp.Sequence != 1 {
		t.Errorf("expected sequence=1, got %d", resp.Sequence)
	}

	if resp.Timestamp == nil {
		t.Error("expected timestamp to be set")
	}

	// Check metrics
	count, lastAt := server.GetHeartbeatMetrics()
	if count != 1 {
		t.Errorf("expected heartbeat count=1, got %d", count)
	}
	if lastAt.IsZero() {
		t.Error("expected lastHeartbeatAt to be set")
	}
}

func TestCoordinationServer_RecoveryCoordinate(t *testing.T) {
	server := &CoordinationServer{
		serverID:      "server-1",
		recoveryState: pb.RecoveryState_RECOVERY_STATE_IDLE,
		startTime:     time.Now(),
	}

	ctx := context.Background()

	tests := []struct {
		name         string
		action       pb.RecoveryAction
		initialState pb.RecoveryState
		wantAccepted bool
		wantState    pb.RecoveryState
	}{
		{
			name:         "pause action",
			action:       pb.RecoveryAction_RECOVERY_ACTION_PAUSE,
			initialState: pb.RecoveryState_RECOVERY_STATE_IDLE,
			wantAccepted: true,
			wantState:    pb.RecoveryState_RECOVERY_STATE_IN_PROGRESS,
		},
		{
			name:         "resume action",
			action:       pb.RecoveryAction_RECOVERY_ACTION_RESUME,
			initialState: pb.RecoveryState_RECOVERY_STATE_IN_PROGRESS,
			wantAccepted: true,
			wantState:    pb.RecoveryState_RECOVERY_STATE_IDLE,
		},
		{
			name:         "already in progress",
			action:       pb.RecoveryAction_RECOVERY_ACTION_RECONNECT,
			initialState: pb.RecoveryState_RECOVERY_STATE_IN_PROGRESS,
			wantAccepted: false,
			wantState:    pb.RecoveryState_RECOVERY_STATE_IN_PROGRESS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server.SetRecoveryState(tt.initialState)

			resp, err := server.RecoveryCoordinate(ctx, &pb.RecoveryCoordinateRequest{
				RequestId:   "test-request-1",
				InitiatorId: "server-2",
				Action:      tt.action,
			})
			if err != nil {
				t.Fatalf("RecoveryCoordinate failed: %v", err)
			}

			if resp.Accepted != tt.wantAccepted {
				t.Errorf("expected accepted=%v, got %v", tt.wantAccepted, resp.Accepted)
			}

			if resp.State != tt.wantState {
				t.Errorf("expected state=%v, got %v", tt.wantState, resp.State)
			}
		})
	}
}

func TestCoordinationServer_PropagateState(t *testing.T) {
	server := &CoordinationServer{
		serverID:      "server-1",
		recoveryState: pb.RecoveryState_RECOVERY_STATE_IDLE,
		startTime:     time.Now(),
	}

	ctx := context.Background()

	tests := []struct {
		name       string
		updateType pb.StateUpdateType
		wantError  bool
	}{
		{
			name:       "agent register",
			updateType: pb.StateUpdateType_STATE_UPDATE_TYPE_AGENT_REGISTER,
			wantError:  false,
		},
		{
			name:       "agent heartbeat",
			updateType: pb.StateUpdateType_STATE_UPDATE_TYPE_AGENT_HEARTBEAT,
			wantError:  false,
		},
		{
			name:       "agent disconnect",
			updateType: pb.StateUpdateType_STATE_UPDATE_TYPE_AGENT_DISCONNECT,
			wantError:  false,
		},
		{
			name:       "command result",
			updateType: pb.StateUpdateType_STATE_UPDATE_TYPE_COMMAND_RESULT,
			wantError:  false,
		},
		{
			name:       "membership change",
			updateType: pb.StateUpdateType_STATE_UPDATE_TYPE_MEMBERSHIP_CHANGE,
			wantError:  false,
		},
		{
			name:       "unspecified type",
			updateType: pb.StateUpdateType_STATE_UPDATE_TYPE_UNSPECIFIED,
			wantError:  false, // Returns error in response, not RPC error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := server.PropagateState(ctx, &pb.PropagateStateRequest{
				RequestId:  "test-request-1",
				SenderId:   "server-2",
				UpdateType: tt.updateType,
				StateData:  []byte(`{"test": "data"}`),
				Version:    1,
			})
			if tt.wantError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if err == nil {
				if resp.ServerId != "server-1" {
					t.Errorf("expected server_id=server-1, got %s", resp.ServerId)
				}
				if resp.RequestId != "test-request-1" {
					t.Errorf("expected request_id=test-request-1, got %s", resp.RequestId)
				}
			}
		})
	}
}

func TestConvertClusterStatus(t *testing.T) {
	tests := []struct {
		input  Status
		output pb.ClusterHealthStatus
	}{
		{StatusHealthy, pb.ClusterHealthStatus_CLUSTER_HEALTH_STATUS_HEALTHY},
		{StatusDegraded, pb.ClusterHealthStatus_CLUSTER_HEALTH_STATUS_DEGRADED},
		{StatusUnhealthy, pb.ClusterHealthStatus_CLUSTER_HEALTH_STATUS_UNHEALTHY},
		{"unknown", pb.ClusterHealthStatus_CLUSTER_HEALTH_STATUS_UNKNOWN},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := convertClusterStatus(tt.input)
			if result != tt.output {
				t.Errorf("expected %v, got %v", tt.output, result)
			}
		})
	}
}

func TestConvertMemberStatus(t *testing.T) {
	tests := []struct {
		input  MemberStatus
		output pb.MemberHealthStatus
	}{
		{MemberStatusHealthy, pb.MemberHealthStatus_MEMBER_HEALTH_STATUS_HEALTHY},
		{MemberStatusDegraded, pb.MemberHealthStatus_MEMBER_HEALTH_STATUS_DEGRADED},
		{MemberStatusUnhealthy, pb.MemberHealthStatus_MEMBER_HEALTH_STATUS_UNHEALTHY},
		{"unknown", pb.MemberHealthStatus_MEMBER_HEALTH_STATUS_UNKNOWN},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := convertMemberStatus(tt.input)
			if result != tt.output {
				t.Errorf("expected %v, got %v", tt.output, result)
			}
		})
	}
}
