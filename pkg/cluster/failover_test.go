package cluster

import (
	"testing"
	"time"
)

func TestFailoverState_Constants(t *testing.T) {
	tests := []struct {
		state FailoverState
		want  string
	}{
		{FailoverStateIdle, "idle"},
		{FailoverStateDetecting, "detecting"},
		{FailoverStateInitiated, "initiated"},
		{FailoverStateInProgress, "in_progress"},
		{FailoverStateCompleted, "completed"},
		{FailoverStateFailed, "failed"},
		{FailoverStateRolledBack, "rolled_back"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if string(tt.state) != tt.want {
				t.Errorf("FailoverState = %v, want %v", string(tt.state), tt.want)
			}
		})
	}
}

func TestFailoverReason_Constants(t *testing.T) {
	tests := []struct {
		reason FailoverReason
		want   string
	}{
		{FailoverReasonHeartbeatLoss, "heartbeat_loss"},
		{FailoverReasonHealthCheck, "health_check_failed"},
		{FailoverReasonManualTrigger, "manual_trigger"},
		{FailoverReasonGracefulDrain, "graceful_drain"},
		{FailoverReasonNetworkPartition, "network_partition"},
	}

	for _, tt := range tests {
		t.Run(string(tt.reason), func(t *testing.T) {
			if string(tt.reason) != tt.want {
				t.Errorf("FailoverReason = %v, want %v", string(tt.reason), tt.want)
			}
		})
	}
}

func TestFailoverEventType_Constants(t *testing.T) {
	tests := []struct {
		eventType FailoverEventType
		want      string
	}{
		{FailoverEventStarted, "failover_started"},
		{FailoverEventProgress, "failover_progress"},
		{FailoverEventCompleted, "failover_completed"},
		{FailoverEventFailed, "failover_failed"},
		{FailoverEventAgentMoved, "agent_moved"},
		{FailoverEventJobMoved, "job_moved"},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			if string(tt.eventType) != tt.want {
				t.Errorf("FailoverEventType = %v, want %v", string(tt.eventType), tt.want)
			}
		})
	}
}

func TestNewFailoverManager_Validation(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	tests := []struct {
		name       string
		config     *Config
		etcd       *EtcdClient
		membership *MembershipManager
		localID    string
		wantErr    bool
	}{
		{
			name:       "nil config",
			config:     nil,
			etcd:       etcd,
			membership: mm,
			localID:    "member-1",
			wantErr:    true,
		},
		{
			name:       "nil etcd",
			config:     config,
			etcd:       nil,
			membership: mm,
			localID:    "member-1",
			wantErr:    true,
		},
		{
			name:       "nil membership",
			config:     config,
			etcd:       etcd,
			membership: nil,
			localID:    "member-1",
			wantErr:    true,
		},
		{
			name:       "empty local ID",
			config:     config,
			etcd:       etcd,
			membership: mm,
			localID:    "",
			wantErr:    true,
		},
		{
			name:       "valid config",
			config:     config,
			etcd:       etcd,
			membership: mm,
			localID:    "member-1",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, err := NewFailoverManager(tt.config, tt.etcd, tt.membership, nil, nil, nil, tt.localID)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewFailoverManager() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && fm == nil {
				t.Error("NewFailoverManager() returned nil without error")
			}
		})
	}
}

func TestFailoverManager_GetActiveFailovers(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	fm, _ := NewFailoverManager(config, etcd, mm, nil, nil, nil, "member-1")

	// Initially empty
	active := fm.GetActiveFailovers()
	if len(active) != 0 {
		t.Errorf("GetActiveFailovers() = %d, want 0", len(active))
	}
}

func TestFailoverManager_GetFailoverHistory(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	fm, _ := NewFailoverManager(config, etcd, mm, nil, nil, nil, "member-1")

	// Add some history
	fm.failoverHistory = append(fm.failoverHistory, &FailoverOperation{
		ID:             "fo-1",
		FailedMemberID: "member-2",
		State:          FailoverStateCompleted,
	})

	history := fm.GetFailoverHistory(10)
	if len(history) != 1 {
		t.Errorf("GetFailoverHistory() = %d, want 1", len(history))
	}
}

func TestFailoverManager_GetFailoverStats(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	fm, _ := NewFailoverManager(config, etcd, mm, nil, nil, nil, "member-1")

	stats := fm.GetFailoverStats()

	if _, ok := stats["total_failovers"]; !ok {
		t.Error("GetFailoverStats() missing total_failovers")
	}
	if _, ok := stats["active_failovers"]; !ok {
		t.Error("GetFailoverStats() missing active_failovers")
	}
}

func TestFailoverManager_AddObserver(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	fm, _ := NewFailoverManager(config, etcd, mm, nil, nil, nil, "member-1")

	called := false
	observer := func(event FailoverEvent) {
		called = true
	}

	fm.AddObserver(observer)

	if len(fm.observers) != 1 {
		t.Errorf("AddObserver() observers count = %d, want 1", len(fm.observers))
	}

	// Suppress unused variable warning
	_ = called
}

func TestFailoverOperation_Fields(t *testing.T) {
	now := time.Now()
	endTime := now.Add(10 * time.Second)

	op := &FailoverOperation{
		ID:               "fo-test-1",
		FailedMemberID:   "member-2",
		Reason:           FailoverReasonHeartbeatLoss,
		State:            FailoverStateCompleted,
		StartTime:        now,
		EndTime:          &endTime,
		AgentsReassigned: 10,
		JobsReassigned:   5,
		EventsReplayed:   100,
		Steps:            make([]*FailoverStep, 0),
	}

	if op.ID != "fo-test-1" {
		t.Errorf("ID = %v, want fo-test-1", op.ID)
	}
	if op.FailedMemberID != "member-2" {
		t.Errorf("FailedMemberID = %v, want member-2", op.FailedMemberID)
	}
	if op.Reason != FailoverReasonHeartbeatLoss {
		t.Errorf("Reason = %v, want %v", op.Reason, FailoverReasonHeartbeatLoss)
	}
	if op.State != FailoverStateCompleted {
		t.Errorf("State = %v, want %v", op.State, FailoverStateCompleted)
	}
	if op.AgentsReassigned != 10 {
		t.Errorf("AgentsReassigned = %d, want 10", op.AgentsReassigned)
	}
}

func TestFailoverStep_Fields(t *testing.T) {
	now := time.Now()
	endTime := now.Add(time.Second)

	step := &FailoverStep{
		Name:      "reassign_agents",
		Status:    FailoverStateCompleted,
		StartTime: now,
		EndTime:   &endTime,
		Details: map[string]interface{}{
			"count": 10,
		},
	}

	if step.Name != "reassign_agents" {
		t.Errorf("Name = %v, want reassign_agents", step.Name)
	}
	if step.Status != FailoverStateCompleted {
		t.Errorf("Status = %v, want %v", step.Status, FailoverStateCompleted)
	}
	if step.Details["count"] != 10 {
		t.Errorf("Details[count] = %v, want 10", step.Details["count"])
	}
}

func TestFailoverEvent_Fields(t *testing.T) {
	event := FailoverEvent{
		Type:        FailoverEventStarted,
		OperationID: "fo-1",
		MemberID:    "member-2",
		State:       FailoverStateInitiated,
		Reason:      FailoverReasonHeartbeatLoss,
		Timestamp:   time.Now(),
		Details: map[string]interface{}{
			"test": "value",
		},
	}

	if event.Type != FailoverEventStarted {
		t.Errorf("Type = %v, want %v", event.Type, FailoverEventStarted)
	}
	if event.OperationID != "fo-1" {
		t.Errorf("OperationID = %v, want fo-1", event.OperationID)
	}
	if event.MemberID != "member-2" {
		t.Errorf("MemberID = %v, want member-2", event.MemberID)
	}
}

func TestFailoverConstants(t *testing.T) {
	if defaultFailoverTimeout != 30*time.Second {
		t.Errorf("defaultFailoverTimeout = %v, want 30s", defaultFailoverTimeout)
	}
	if defaultAgentReassignBatch != 100 {
		t.Errorf("defaultAgentReassignBatch = %d, want 100", defaultAgentReassignBatch)
	}
	if defaultJobReassignBatch != 50 {
		t.Errorf("defaultJobReassignBatch = %d, want 50", defaultJobReassignBatch)
	}
	if defaultFailoverCooldown != 10*time.Second {
		t.Errorf("defaultFailoverCooldown = %v, want 10s", defaultFailoverCooldown)
	}
	if defaultMaxConcurrentFails != 2 {
		t.Errorf("defaultMaxConcurrentFails = %d, want 2", defaultMaxConcurrentFails)
	}
}
