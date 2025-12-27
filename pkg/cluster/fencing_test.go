package cluster

import (
	"context"
	"testing"
	"time"
)

func TestFencingMode_Constants(t *testing.T) {
	tests := []struct {
		mode FencingMode
		want string
	}{
		{FencingModeStrict, "strict"},
		{FencingModeReadOnly, "read_only"},
		{FencingModeGraceful, "graceful"},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			if string(tt.mode) != tt.want {
				t.Errorf("FencingMode = %v, want %v", string(tt.mode), tt.want)
			}
		})
	}
}

func TestFenceStatus_Constants(t *testing.T) {
	tests := []struct {
		status FenceStatus
		want   string
	}{
		{FenceStatusActive, "active"},
		{FenceStatusWarning, "warning"},
		{FenceStatusFenced, "fenced"},
		{FenceStatusRecovering, "recovering"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if string(tt.status) != tt.want {
				t.Errorf("FenceStatus = %v, want %v", string(tt.status), tt.want)
			}
		})
	}
}

func TestFenceReason_Constants(t *testing.T) {
	tests := []struct {
		reason FenceReason
		want   string
	}{
		{FenceReasonNone, "none"},
		{FenceReasonLeaseLost, "lease_lost"},
		{FenceReasonQuorumLost, "quorum_lost"},
		{FenceReasonPartitioned, "partitioned"},
		{FenceReasonEpochStale, "epoch_stale"},
		{FenceReasonManual, "manual"},
		{FenceReasonHealthCheck, "health_check_failed"},
	}

	for _, tt := range tests {
		t.Run(string(tt.reason), func(t *testing.T) {
			if string(tt.reason) != tt.want {
				t.Errorf("FenceReason = %v, want %v", string(tt.reason), tt.want)
			}
		})
	}
}

func TestFenceEventType_Constants(t *testing.T) {
	tests := []struct {
		eventType FenceEventType
		want      string
	}{
		{FenceEventFenced, "fenced"},
		{FenceEventUnfenced, "unfenced"},
		{FenceEventEpochBump, "epoch_bump"},
		{FenceEventWarning, "warning"},
		{FenceEventRecovery, "recovery"},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			if string(tt.eventType) != tt.want {
				t.Errorf("FenceEventType = %v, want %v", string(tt.eventType), tt.want)
			}
		})
	}
}

func TestDefaultFenceConfig(t *testing.T) {
	config := DefaultFenceConfig()

	if config.Mode != FencingModeStrict {
		t.Errorf("Mode = %v, want %v", config.Mode, FencingModeStrict)
	}
	if config.CheckInterval != defaultFenceCheckInterval {
		t.Errorf("CheckInterval = %v, want %v", config.CheckInterval, defaultFenceCheckInterval)
	}
	if config.FenceTimeout != defaultFenceTimeout {
		t.Errorf("FenceTimeout = %v, want %v", config.FenceTimeout, defaultFenceTimeout)
	}
	if !config.QuorumRequired {
		t.Error("QuorumRequired should be true by default")
	}
	if !config.LeaseRequired {
		t.Error("LeaseRequired should be true by default")
	}
	if !config.EpochValidation {
		t.Error("EpochValidation should be true by default")
	}
}

func TestNewFenceGuard_Validation(t *testing.T) {
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
		config     *FenceConfig
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
			config:     DefaultFenceConfig(),
			etcd:       nil,
			membership: mm,
			localID:    "member-1",
			wantErr:    true,
		},
		{
			name:       "nil membership",
			config:     DefaultFenceConfig(),
			etcd:       etcd,
			membership: nil,
			localID:    "member-1",
			wantErr:    true,
		},
		{
			name:       "empty local ID",
			config:     DefaultFenceConfig(),
			etcd:       etcd,
			membership: mm,
			localID:    "",
			wantErr:    true,
		},
		{
			name:       "valid config",
			config:     DefaultFenceConfig(),
			etcd:       etcd,
			membership: mm,
			localID:    "member-1",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fg, err := NewFenceGuard(tt.config, tt.etcd, tt.membership, nil, tt.localID)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewFenceGuard() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && fg == nil {
				t.Error("NewFenceGuard() returned nil without error")
			}
		})
	}
}

func TestFenceGuard_GetStatus(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:    config,
		etcd:      etcd,
		members:   make(map[string]*Member),
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
	}

	fg, _ := NewFenceGuard(nil, etcd, mm, nil, "member-1")

	status := fg.GetStatus()
	if status != FenceStatusActive {
		t.Errorf("GetStatus() = %v, want %v", status, FenceStatusActive)
	}
}

func TestFenceGuard_GetReason(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:    config,
		etcd:      etcd,
		members:   make(map[string]*Member),
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
	}

	fg, _ := NewFenceGuard(nil, etcd, mm, nil, "member-1")

	reason := fg.GetReason()
	if reason != FenceReasonNone {
		t.Errorf("GetReason() = %v, want %v", reason, FenceReasonNone)
	}
}

func TestFenceGuard_IsFenced(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:    config,
		etcd:      etcd,
		members:   make(map[string]*Member),
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
	}

	fg, _ := NewFenceGuard(nil, etcd, mm, nil, "member-1")

	if fg.IsFenced() {
		t.Error("IsFenced() should be false initially")
	}
}

func TestFenceGuard_CanWrite(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:    config,
		etcd:      etcd,
		members:   make(map[string]*Member),
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
	}

	fg, _ := NewFenceGuard(nil, etcd, mm, nil, "member-1")

	if !fg.CanWrite() {
		t.Error("CanWrite() should be true initially")
	}
}

func TestFenceGuard_CanRead(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:    config,
		etcd:      etcd,
		members:   make(map[string]*Member),
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
	}

	fg, _ := NewFenceGuard(nil, etcd, mm, nil, "member-1")

	if !fg.CanRead() {
		t.Error("CanRead() should be true initially")
	}
}

func TestFenceGuard_Fence(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:    config,
		etcd:      etcd,
		members:   make(map[string]*Member),
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
	}

	fg, _ := NewFenceGuard(nil, etcd, mm, nil, "member-1")

	fg.Fence(FenceReasonManual)

	if fg.GetStatus() != FenceStatusFenced {
		t.Errorf("GetStatus() = %v, want %v after Fence()", fg.GetStatus(), FenceStatusFenced)
	}
	if fg.GetReason() != FenceReasonManual {
		t.Errorf("GetReason() = %v, want %v after Fence()", fg.GetReason(), FenceReasonManual)
	}
	if !fg.IsFenced() {
		t.Error("IsFenced() should be true after Fence()")
	}
	if fg.CanWrite() {
		t.Error("CanWrite() should be false after Fence()")
	}
}

func TestFenceGuard_AcquireOperation(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:    config,
		etcd:      etcd,
		members:   make(map[string]*Member),
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
	}

	fg, _ := NewFenceGuard(nil, etcd, mm, nil, "member-1")

	// Should succeed when not fenced
	release, err := fg.AcquireOperation(context.Background(), true)
	if err != nil {
		t.Errorf("AcquireOperation() error = %v, want nil", err)
	}
	if release == nil {
		t.Error("AcquireOperation() should return release function")
	}
	release()

	// Should fail when fenced
	fg.Fence(FenceReasonManual)
	_, err = fg.AcquireOperation(context.Background(), true)
	if err == nil {
		t.Error("AcquireOperation() should fail when fenced")
	}
}

func TestFenceGuard_ValidateEpoch(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:    config,
		etcd:      etcd,
		members:   make(map[string]*Member),
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
	}

	fg, _ := NewFenceGuard(nil, etcd, mm, nil, "member-1")

	// Current epoch should be valid
	if err := fg.ValidateEpoch(0); err != nil {
		t.Errorf("ValidateEpoch(0) error = %v", err)
	}

	// Future epochs should be valid
	if err := fg.ValidateEpoch(100); err != nil {
		t.Errorf("ValidateEpoch(100) error = %v", err)
	}
}

func TestFenceGuard_SetLeaseValid(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:    config,
		etcd:      etcd,
		members:   make(map[string]*Member),
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
	}

	fenceConfig := DefaultFenceConfig()
	fenceConfig.LeaseRequired = true

	fg, _ := NewFenceGuard(fenceConfig, etcd, mm, nil, "member-1")

	// Initially valid
	if fg.IsFenced() {
		t.Error("Should not be fenced initially")
	}

	// Set lease invalid - should fence
	fg.SetLeaseValid(false)

	if !fg.IsFenced() {
		t.Error("Should be fenced after lease invalidation")
	}
	if fg.GetReason() != FenceReasonLeaseLost {
		t.Errorf("GetReason() = %v, want %v", fg.GetReason(), FenceReasonLeaseLost)
	}
}

func TestFenceGuard_AddObserver(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:    config,
		etcd:      etcd,
		members:   make(map[string]*Member),
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
	}

	fg, _ := NewFenceGuard(nil, etcd, mm, nil, "member-1")

	called := false
	observer := func(event FenceEvent) {
		called = true
	}

	fg.AddObserver(observer)

	if len(fg.observers) != 1 {
		t.Errorf("AddObserver() observers count = %d, want 1", len(fg.observers))
	}

	// Suppress unused variable warning
	_ = called
}

func TestFenceGuard_GetStats(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:    config,
		etcd:      etcd,
		members:   make(map[string]*Member),
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
	}

	fg, _ := NewFenceGuard(nil, etcd, mm, nil, "member-1")

	stats := fg.GetStats()

	if _, ok := stats["status"]; !ok {
		t.Error("GetStats() missing status")
	}
	if _, ok := stats["reason"]; !ok {
		t.Error("GetStats() missing reason")
	}
	if _, ok := stats["epoch"]; !ok {
		t.Error("GetStats() missing epoch")
	}
	if _, ok := stats["lease_valid"]; !ok {
		t.Error("GetStats() missing lease_valid")
	}
}

func TestFenceGuard_NewFenceToken(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:    config,
		etcd:      etcd,
		members:   make(map[string]*Member),
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
	}

	fg, _ := NewFenceGuard(nil, etcd, mm, nil, "member-1")

	token := fg.NewFenceToken()

	if token.MemberID != "member-1" {
		t.Errorf("Token.MemberID = %v, want member-1", token.MemberID)
	}
	if token.Epoch != fg.GetEpoch() {
		t.Errorf("Token.Epoch = %d, want %d", token.Epoch, fg.GetEpoch())
	}
	if token.Timestamp.IsZero() {
		t.Error("Token.Timestamp should not be zero")
	}
}

func TestFenceGuard_ValidateFenceToken(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:    config,
		etcd:      etcd,
		members:   make(map[string]*Member),
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
	}

	fg, _ := NewFenceGuard(nil, etcd, mm, nil, "member-1")

	// Valid token
	token := fg.NewFenceToken()
	if err := fg.ValidateFenceToken(token); err != nil {
		t.Errorf("ValidateFenceToken() error = %v for valid token", err)
	}

	// Nil token
	if err := fg.ValidateFenceToken(nil); err == nil {
		t.Error("ValidateFenceToken() should fail for nil token")
	}

	// Wrong member
	wrongMember := &FenceToken{
		MemberID:  "wrong-member",
		Epoch:     fg.GetEpoch(),
		Timestamp: time.Now(),
	}
	if err := fg.ValidateFenceToken(wrongMember); err == nil {
		t.Error("ValidateFenceToken() should fail for wrong member")
	}
}

func TestFenceEvent_Fields(t *testing.T) {
	event := FenceEvent{
		Type:      FenceEventFenced,
		Status:    FenceStatusFenced,
		Reason:    FenceReasonQuorumLost,
		MemberID:  "member-1",
		Epoch:     5,
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"healthy_members": 1,
		},
	}

	if event.Type != FenceEventFenced {
		t.Errorf("Type = %v, want %v", event.Type, FenceEventFenced)
	}
	if event.Status != FenceStatusFenced {
		t.Errorf("Status = %v, want %v", event.Status, FenceStatusFenced)
	}
	if event.Reason != FenceReasonQuorumLost {
		t.Errorf("Reason = %v, want %v", event.Reason, FenceReasonQuorumLost)
	}
	if event.Epoch != 5 {
		t.Errorf("Epoch = %d, want 5", event.Epoch)
	}
}

func TestFenceConfig_Fields(t *testing.T) {
	config := &FenceConfig{
		Mode:            FencingModeGraceful,
		CheckInterval:   2 * time.Second,
		FenceTimeout:    10 * time.Second,
		QuorumRequired:  false,
		LeaseRequired:   false,
		EpochValidation: false,
		GracePeriod:     10 * time.Second,
	}

	if config.Mode != FencingModeGraceful {
		t.Errorf("Mode = %v, want %v", config.Mode, FencingModeGraceful)
	}
	if config.CheckInterval != 2*time.Second {
		t.Errorf("CheckInterval = %v, want 2s", config.CheckInterval)
	}
	if config.FenceTimeout != 10*time.Second {
		t.Errorf("FenceTimeout = %v, want 10s", config.FenceTimeout)
	}
	if config.QuorumRequired {
		t.Error("QuorumRequired should be false")
	}
}

func TestFenceConstants(t *testing.T) {
	if defaultFenceCheckInterval != time.Second {
		t.Errorf("defaultFenceCheckInterval = %v, want 1s", defaultFenceCheckInterval)
	}
	if defaultFenceTimeout != 5*time.Second {
		t.Errorf("defaultFenceTimeout = %v, want 5s", defaultFenceTimeout)
	}
}

func TestErrFenced(t *testing.T) {
	if ErrFenced == nil {
		t.Error("ErrFenced should not be nil")
	}
	if ErrFenced.Error() == "" {
		t.Error("ErrFenced.Error() should not be empty")
	}
}

func TestErrStaleEpoch(t *testing.T) {
	if ErrStaleEpoch == nil {
		t.Error("ErrStaleEpoch should not be nil")
	}
	if ErrStaleEpoch.Error() == "" {
		t.Error("ErrStaleEpoch.Error() should not be empty")
	}
}
