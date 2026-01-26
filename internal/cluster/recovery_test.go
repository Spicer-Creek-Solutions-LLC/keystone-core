package cluster

import (
	"testing"
	"time"
)

func TestRecoveryPhase_Constants(t *testing.T) {
	tests := []struct {
		phase RecoveryPhase
		want  string
	}{
		{RecoveryPhaseIdle, "idle"},
		{RecoveryPhaseStarting, "starting"},
		{RecoveryPhaseConnecting, "connecting"},
		{RecoveryPhaseSyncing, "syncing"},
		{RecoveryPhaseVerifying, "verifying"},
		{RecoveryPhaseRejoining, "rejoining"},
		{RecoveryPhaseReclaiming, "reclaiming"},
		{RecoveryPhaseCompleted, "completed"},
		{RecoveryPhaseFailed, "failed"},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			if string(tt.phase) != tt.want {
				t.Errorf("RecoveryPhase = %v, want %v", string(tt.phase), tt.want)
			}
		})
	}
}

func TestRecoveryReason_Constants(t *testing.T) {
	tests := []struct {
		reason RecoveryReason
		want   string
	}{
		{RecoveryReasonRestart, "restart"},
		{RecoveryReasonCrash, "crash"},
		{RecoveryReasonNetworkRestore, "network_restore"},
		{RecoveryReasonManual, "manual"},
	}

	for _, tt := range tests {
		t.Run(string(tt.reason), func(t *testing.T) {
			if string(tt.reason) != tt.want {
				t.Errorf("RecoveryReason = %v, want %v", string(tt.reason), tt.want)
			}
		})
	}
}

func TestRecoveryEventType_Constants(t *testing.T) {
	tests := []struct {
		eventType RecoveryEventType
		want      string
	}{
		{RecoveryEventStarted, "recovery_started"},
		{RecoveryEventConnected, "etcd_connected"},
		{RecoveryEventSyncStarted, "sync_started"},
		{RecoveryEventSyncCompleted, "sync_completed"},
		{RecoveryEventRejoined, "cluster_rejoined"},
		{RecoveryEventCompleted, "recovery_completed"},
		{RecoveryEventFailed, "recovery_failed"},
		{RecoveryEventProgress, "recovery_progress"},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			if string(tt.eventType) != tt.want {
				t.Errorf("RecoveryEventType = %v, want %v", string(tt.eventType), tt.want)
			}
		})
	}
}

func TestDefaultRecoveryConfig(t *testing.T) {
	config := DefaultRecoveryConfig()

	if config.RecoveryTimeout != defaultRecoveryTimeout {
		t.Errorf("RecoveryTimeout = %v, want %v", config.RecoveryTimeout, defaultRecoveryTimeout)
	}
	if config.SyncTimeout != defaultSyncTimeout {
		t.Errorf("SyncTimeout = %v, want %v", config.SyncTimeout, defaultSyncTimeout)
	}
	if config.VerificationTimeout != defaultVerificationTimeout {
		t.Errorf("VerificationTimeout = %v, want %v", config.VerificationTimeout, defaultVerificationTimeout)
	}
	if config.MaxRetries != defaultRecoveryRetries {
		t.Errorf("MaxRetries = %d, want %d", config.MaxRetries, defaultRecoveryRetries)
	}
	if config.RetryBackoff != defaultRecoveryBackoff {
		t.Errorf("RetryBackoff = %v, want %v", config.RetryBackoff, defaultRecoveryBackoff)
	}
	if !config.AutoRecover {
		t.Error("AutoRecover should be true by default")
	}
	if !config.ReclaimAgents {
		t.Error("ReclaimAgents should be true by default")
	}
	if !config.RecoverJobs {
		t.Error("RecoverJobs should be true by default")
	}
}

func TestNewRecoveryManager_Validation(t *testing.T) {
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
		config     *RecoveryConfig
		etcd       *EtcdClient
		membership *MembershipManager
		localID    string
		wantErr    bool
	}{
		{
			name:       "nil config uses defaults",
			config:     nil,
			etcd:       etcd,
			membership: mm,
			localID:    "member-1",
			wantErr:    false,
		},
		{
			name:       "nil etcd",
			config:     DefaultRecoveryConfig(),
			etcd:       nil,
			membership: mm,
			localID:    "member-1",
			wantErr:    true,
		},
		{
			name:       "nil membership",
			config:     DefaultRecoveryConfig(),
			etcd:       etcd,
			membership: nil,
			localID:    "member-1",
			wantErr:    true,
		},
		{
			name:       "empty local ID",
			config:     DefaultRecoveryConfig(),
			etcd:       etcd,
			membership: mm,
			localID:    "",
			wantErr:    true,
		},
		{
			name:       "valid config",
			config:     DefaultRecoveryConfig(),
			etcd:       etcd,
			membership: mm,
			localID:    "member-1",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm, err := NewRecoveryManager(tt.config, config, tt.etcd, tt.membership, nil, nil, tt.localID)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRecoveryManager() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && rm == nil {
				t.Error("NewRecoveryManager() returned nil without error")
			}
		})
	}
}

func TestRecoveryManager_GetStatus(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	rm, _ := NewRecoveryManager(nil, config, etcd, mm, nil, nil, "member-1")

	status := rm.GetStatus()
	if status.Phase != RecoveryPhaseIdle {
		t.Errorf("GetStatus().Phase = %v, want %v", status.Phase, RecoveryPhaseIdle)
	}
}

func TestRecoveryManager_IsRecovering(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	rm, _ := NewRecoveryManager(nil, config, etcd, mm, nil, nil, "member-1")

	if rm.IsRecovering() {
		t.Error("IsRecovering() should be false initially")
	}
}

func TestRecoveryManager_AddObserver(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	rm, _ := NewRecoveryManager(nil, config, etcd, mm, nil, nil, "member-1")

	called := false
	observer := func(event RecoveryEvent) {
		called = true
	}

	rm.AddObserver(observer)

	if len(rm.observers) != 1 {
		t.Errorf("AddObserver() observers count = %d, want 1", len(rm.observers))
	}

	// Suppress unused variable warning
	_ = called
}

func TestRecoveryStatus_Fields(t *testing.T) {
	status := &RecoveryStatus{
		Phase:           RecoveryPhaseSyncing,
		Reason:          RecoveryReasonRestart,
		StartTime:       time.Now(),
		CurrentStep:     "syncing membership",
		Progress:        0.5,
		MembersSynced:   3,
		AgentsReclaimed: 10,
		JobsRecovered:   5,
		Errors:          []string{"warning: partial sync"},
	}

	if status.Phase != RecoveryPhaseSyncing {
		t.Errorf("Phase = %v, want %v", status.Phase, RecoveryPhaseSyncing)
	}
	if status.Reason != RecoveryReasonRestart {
		t.Errorf("Reason = %v, want %v", status.Reason, RecoveryReasonRestart)
	}
	if status.Progress != 0.5 {
		t.Errorf("Progress = %v, want 0.5", status.Progress)
	}
	if status.MembersSynced != 3 {
		t.Errorf("MembersSynced = %d, want 3", status.MembersSynced)
	}
	if status.AgentsReclaimed != 10 {
		t.Errorf("AgentsReclaimed = %d, want 10", status.AgentsReclaimed)
	}
}

func TestRecoveryEvent_Fields(t *testing.T) {
	event := RecoveryEvent{
		Type:      RecoveryEventStarted,
		Phase:     RecoveryPhaseStarting,
		MemberID:  "member-1",
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"reason": RecoveryReasonRestart,
		},
	}

	if event.Type != RecoveryEventStarted {
		t.Errorf("Type = %v, want %v", event.Type, RecoveryEventStarted)
	}
	if event.Phase != RecoveryPhaseStarting {
		t.Errorf("Phase = %v, want %v", event.Phase, RecoveryPhaseStarting)
	}
	if event.MemberID != "member-1" {
		t.Errorf("MemberID = %v, want member-1", event.MemberID)
	}
}

func TestRecoveryState_Fields(t *testing.T) {
	state := &RecoveryState{
		MemberID:        "member-1",
		ClusterName:     "test-cluster",
		LastKnownLeader: "member-2",
		LastHeartbeat:   time.Now(),
		AssignedAgents:  []string{"agent-1", "agent-2"},
		PendingJobs:     []string{"job-1"},
		EventOffset:     12345,
		LeadershipEpoch: 5,
	}

	if state.MemberID != "member-1" {
		t.Errorf("MemberID = %v, want member-1", state.MemberID)
	}
	if state.ClusterName != "test-cluster" {
		t.Errorf("ClusterName = %v, want test-cluster", state.ClusterName)
	}
	if state.LastKnownLeader != "member-2" {
		t.Errorf("LastKnownLeader = %v, want member-2", state.LastKnownLeader)
	}
	if len(state.AssignedAgents) != 2 {
		t.Errorf("AssignedAgents count = %d, want 2", len(state.AssignedAgents))
	}
	if len(state.PendingJobs) != 1 {
		t.Errorf("PendingJobs count = %d, want 1", len(state.PendingJobs))
	}
	if state.EventOffset != 12345 {
		t.Errorf("EventOffset = %d, want 12345", state.EventOffset)
	}
	if state.LeadershipEpoch != 5 {
		t.Errorf("LeadershipEpoch = %d, want 5", state.LeadershipEpoch)
	}
}

func TestRecoveryConfig_Fields(t *testing.T) {
	config := &RecoveryConfig{
		RecoveryTimeout:     120 * time.Second,
		SyncTimeout:         60 * time.Second,
		VerificationTimeout: 20 * time.Second,
		MaxRetries:          5,
		RetryBackoff:        5 * time.Second,
		AutoRecover:         false,
		ReclaimAgents:       false,
		RecoverJobs:         false,
	}

	if config.RecoveryTimeout != 120*time.Second {
		t.Errorf("RecoveryTimeout = %v, want 120s", config.RecoveryTimeout)
	}
	if config.SyncTimeout != 60*time.Second {
		t.Errorf("SyncTimeout = %v, want 60s", config.SyncTimeout)
	}
	if config.VerificationTimeout != 20*time.Second {
		t.Errorf("VerificationTimeout = %v, want 20s", config.VerificationTimeout)
	}
	if config.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", config.MaxRetries)
	}
	if config.AutoRecover {
		t.Error("AutoRecover should be false")
	}
}

func TestRecoveryConstants(t *testing.T) {
	if defaultRecoveryTimeout != 60*time.Second {
		t.Errorf("defaultRecoveryTimeout = %v, want 60s", defaultRecoveryTimeout)
	}
	if defaultSyncTimeout != 30*time.Second {
		t.Errorf("defaultSyncTimeout = %v, want 30s", defaultSyncTimeout)
	}
	if defaultVerificationTimeout != 10*time.Second {
		t.Errorf("defaultVerificationTimeout = %v, want 10s", defaultVerificationTimeout)
	}
	if defaultRecoveryRetries != 3 {
		t.Errorf("defaultRecoveryRetries = %d, want 3", defaultRecoveryRetries)
	}
	if defaultRecoveryBackoff != 2*time.Second {
		t.Errorf("defaultRecoveryBackoff = %v, want 2s", defaultRecoveryBackoff)
	}
}
