package cluster

import (
	"testing"
	"time"
)

func TestShutdownPhase_Constants(t *testing.T) {
	tests := []struct {
		phase ShutdownPhase
		want  string
	}{
		{ShutdownPhaseRunning, "running"},
		{ShutdownPhaseInitiated, "initiated"},
		{ShutdownPhaseDraining, "draining"},
		{ShutdownPhaseTransferring, "transferring"},
		{ShutdownPhaseDeregistering, "deregistering"},
		{ShutdownPhaseCompleted, "completed"},
		{ShutdownPhaseFailed, "failed"},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			if string(tt.phase) != tt.want {
				t.Errorf("ShutdownPhase = %v, want %v", string(tt.phase), tt.want)
			}
		})
	}
}

func TestShutdownReason_Constants(t *testing.T) {
	tests := []struct {
		reason ShutdownReason
		want   string
	}{
		{ShutdownReasonRequested, "requested"},
		{ShutdownReasonMaintenance, "maintenance"},
		{ShutdownReasonUpgrade, "upgrade"},
		{ShutdownReasonScaleDown, "scale_down"},
		{ShutdownReasonHealthy, "unhealthy"},
		{ShutdownReasonSignal, "signal"},
	}

	for _, tt := range tests {
		t.Run(string(tt.reason), func(t *testing.T) {
			if string(tt.reason) != tt.want {
				t.Errorf("ShutdownReason = %v, want %v", string(tt.reason), tt.want)
			}
		})
	}
}

func TestShutdownEventType_Constants(t *testing.T) {
	tests := []struct {
		eventType ShutdownEventType
		want      string
	}{
		{ShutdownEventStarted, "shutdown_started"},
		{ShutdownEventDrainStarted, "drain_started"},
		{ShutdownEventAgentDrained, "agent_drained"},
		{ShutdownEventJobsCompleted, "jobs_completed"},
		{ShutdownEventLeaderTransfer, "leader_transferred"},
		{ShutdownEventDeregistered, "deregistered"},
		{ShutdownEventCompleted, "shutdown_completed"},
		{ShutdownEventFailed, "shutdown_failed"},
		{ShutdownEventTimeout, "shutdown_timeout"},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			if string(tt.eventType) != tt.want {
				t.Errorf("ShutdownEventType = %v, want %v", string(tt.eventType), tt.want)
			}
		})
	}
}

func TestDefaultShutdownConfig(t *testing.T) {
	config := DefaultShutdownConfig()

	if config.DrainTimeout != defaultDrainTimeout {
		t.Errorf("DrainTimeout = %v, want %v", config.DrainTimeout, defaultDrainTimeout)
	}
	if config.ShutdownTimeout != defaultShutdownTimeout {
		t.Errorf("ShutdownTimeout = %v, want %v", config.ShutdownTimeout, defaultShutdownTimeout)
	}
	if !config.ForceAfterTimeout {
		t.Error("ForceAfterTimeout should be true by default")
	}
	if !config.TransferLeadership {
		t.Error("TransferLeadership should be true by default")
	}
	if !config.WaitForJobs {
		t.Error("WaitForJobs should be true by default")
	}
}

func TestNewGracefulShutdown_Validation(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	mm := &MembershipManager{
		config:   config,
		etcd:     &EtcdClient{},
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	tests := []struct {
		name       string
		config     *ShutdownConfig
		membership *MembershipManager
		localID    string
		wantErr    bool
	}{
		{
			name:       "nil config uses defaults",
			config:     nil,
			membership: mm,
			localID:    "member-1",
			wantErr:    false,
		},
		{
			name:       "nil membership",
			config:     DefaultShutdownConfig(),
			membership: nil,
			localID:    "member-1",
			wantErr:    true,
		},
		{
			name:       "empty local ID",
			config:     DefaultShutdownConfig(),
			membership: mm,
			localID:    "",
			wantErr:    true,
		},
		{
			name:       "valid config",
			config:     DefaultShutdownConfig(),
			membership: mm,
			localID:    "member-1",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs, err := NewGracefulShutdown(tt.config, config, tt.membership, nil, nil, nil, tt.localID)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewGracefulShutdown() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && gs == nil {
				t.Error("NewGracefulShutdown() returned nil without error")
			}
		})
	}
}

func TestGracefulShutdown_GetStatus(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	mm := &MembershipManager{
		config:   config,
		etcd:     &EtcdClient{},
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	gs, _ := NewGracefulShutdown(nil, config, mm, nil, nil, nil, "member-1")

	status := gs.GetStatus()
	if status.Phase != ShutdownPhaseRunning {
		t.Errorf("GetStatus().Phase = %v, want %v", status.Phase, ShutdownPhaseRunning)
	}
}

func TestGracefulShutdown_IsInProgress(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	mm := &MembershipManager{
		config:   config,
		etcd:     &EtcdClient{},
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	gs, _ := NewGracefulShutdown(nil, config, mm, nil, nil, nil, "member-1")

	if gs.IsInProgress() {
		t.Error("IsInProgress() should be false initially")
	}
}

func TestGracefulShutdown_AddObserver(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	mm := &MembershipManager{
		config:   config,
		etcd:     &EtcdClient{},
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	gs, _ := NewGracefulShutdown(nil, config, mm, nil, nil, nil, "member-1")

	called := false
	observer := func(event ShutdownEvent) {
		called = true
	}

	gs.AddObserver(observer)

	if len(gs.observers) != 1 {
		t.Errorf("AddObserver() observers count = %d, want 1", len(gs.observers))
	}

	// Suppress unused variable warning
	_ = called
}

func TestShutdownStatus_Fields(t *testing.T) {
	status := &ShutdownStatus{
		Phase:                 ShutdownPhaseDraining,
		Reason:                ShutdownReasonMaintenance,
		StartTime:             time.Now(),
		CurrentStep:           "draining agents",
		AgentsDrained:         50,
		AgentsRemaining:       50,
		JobsCompleted:         10,
		JobsRemaining:         5,
		LeadershipTransferred: false,
	}

	if status.Phase != ShutdownPhaseDraining {
		t.Errorf("Phase = %v, want %v", status.Phase, ShutdownPhaseDraining)
	}
	if status.Reason != ShutdownReasonMaintenance {
		t.Errorf("Reason = %v, want %v", status.Reason, ShutdownReasonMaintenance)
	}
	if status.AgentsDrained != 50 {
		t.Errorf("AgentsDrained = %d, want 50", status.AgentsDrained)
	}
	if status.AgentsRemaining != 50 {
		t.Errorf("AgentsRemaining = %d, want 50", status.AgentsRemaining)
	}
}

func TestShutdownEvent_Fields(t *testing.T) {
	event := ShutdownEvent{
		Type:      ShutdownEventStarted,
		Phase:     ShutdownPhaseInitiated,
		MemberID:  "member-1",
		Reason:    ShutdownReasonUpgrade,
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"version": "1.2.0",
		},
	}

	if event.Type != ShutdownEventStarted {
		t.Errorf("Type = %v, want %v", event.Type, ShutdownEventStarted)
	}
	if event.Phase != ShutdownPhaseInitiated {
		t.Errorf("Phase = %v, want %v", event.Phase, ShutdownPhaseInitiated)
	}
	if event.MemberID != "member-1" {
		t.Errorf("MemberID = %v, want member-1", event.MemberID)
	}
	if event.Reason != ShutdownReasonUpgrade {
		t.Errorf("Reason = %v, want %v", event.Reason, ShutdownReasonUpgrade)
	}
}

func TestShutdownConfig_Fields(t *testing.T) {
	config := &ShutdownConfig{
		DrainTimeout:       60 * time.Second,
		ShutdownTimeout:    120 * time.Second,
		ForceAfterTimeout:  false,
		TransferLeadership: false,
		WaitForJobs:        false,
	}

	if config.DrainTimeout != 60*time.Second {
		t.Errorf("DrainTimeout = %v, want 60s", config.DrainTimeout)
	}
	if config.ShutdownTimeout != 120*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 120s", config.ShutdownTimeout)
	}
	if config.ForceAfterTimeout {
		t.Error("ForceAfterTimeout should be false")
	}
	if config.TransferLeadership {
		t.Error("TransferLeadership should be false")
	}
	if config.WaitForJobs {
		t.Error("WaitForJobs should be false")
	}
}

func TestShutdownConstants(t *testing.T) {
	if defaultDrainTimeout != 30*time.Second {
		t.Errorf("defaultDrainTimeout = %v, want 30s", defaultDrainTimeout)
	}
	if defaultShutdownTimeout != 60*time.Second {
		t.Errorf("defaultShutdownTimeout = %v, want 60s", defaultShutdownTimeout)
	}
	if defaultJobDrainInterval != 100*time.Millisecond {
		t.Errorf("defaultJobDrainInterval = %v, want 100ms", defaultJobDrainInterval)
	}
}
