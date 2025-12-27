package cluster

import (
	"context"
	"testing"
	"time"
)

func TestHealthStatus_Constants(t *testing.T) {
	tests := []struct {
		status HealthStatus
		want   string
	}{
		{HealthStatusHealthy, "healthy"},
		{HealthStatusDegraded, "degraded"},
		{HealthStatusUnhealthy, "unhealthy"},
		{HealthStatusUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if string(tt.status) != tt.want {
				t.Errorf("HealthStatus = %v, want %v", string(tt.status), tt.want)
			}
		})
	}
}

func TestHealthCheckType_Constants(t *testing.T) {
	tests := []struct {
		checkType HealthCheckType
		want      string
	}{
		{HealthCheckHeartbeat, "heartbeat"},
		{HealthCheckEtcd, "etcd"},
		{HealthCheckDatabase, "database"},
		{HealthCheckNATS, "nats"},
		{HealthCheckApplication, "application"},
	}

	for _, tt := range tests {
		t.Run(string(tt.checkType), func(t *testing.T) {
			if string(tt.checkType) != tt.want {
				t.Errorf("HealthCheckType = %v, want %v", string(tt.checkType), tt.want)
			}
		})
	}
}

func TestHealthEventType_Constants(t *testing.T) {
	tests := []struct {
		eventType HealthEventType
		want      string
	}{
		{HealthEventMemberHealthy, "member_healthy"},
		{HealthEventMemberDegraded, "member_degraded"},
		{HealthEventMemberUnhealthy, "member_unhealthy"},
		{HealthEventMemberFailed, "member_failed"},
		{HealthEventMemberRecovered, "member_recovered"},
		{HealthEventPartitionStart, "partition_start"},
		{HealthEventPartitionEnd, "partition_end"},
		{HealthEventQuorumLost, "quorum_lost"},
		{HealthEventQuorumRestored, "quorum_restored"},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			if string(tt.eventType) != tt.want {
				t.Errorf("HealthEventType = %v, want %v", string(tt.eventType), tt.want)
			}
		})
	}
}

func TestNewHealthMonitor_Validation(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second, HeartbeatTimeout: 30 * time.Second}
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
		membership *MembershipManager
		etcd       *EtcdClient
		localID    string
		wantErr    bool
	}{
		{
			name:       "nil config",
			config:     nil,
			membership: mm,
			etcd:       etcd,
			localID:    "member-1",
			wantErr:    true,
		},
		{
			name:       "nil membership",
			config:     config,
			membership: nil,
			etcd:       etcd,
			localID:    "member-1",
			wantErr:    true,
		},
		{
			name:       "nil etcd",
			config:     config,
			membership: mm,
			etcd:       nil,
			localID:    "member-1",
			wantErr:    true,
		},
		{
			name:       "empty local ID",
			config:     config,
			membership: mm,
			etcd:       etcd,
			localID:    "",
			wantErr:    true,
		},
		{
			name:       "valid config",
			config:     config,
			membership: mm,
			etcd:       etcd,
			localID:    "member-1",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hm, err := NewHealthMonitor(tt.config, tt.membership, tt.etcd, tt.localID)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewHealthMonitor() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && hm == nil {
				t.Error("NewHealthMonitor() returned nil without error")
			}
		})
	}
}

func TestHealthMonitor_RecordHeartbeat(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second, HeartbeatTimeout: 30 * time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	hm, _ := NewHealthMonitor(config, mm, etcd, "member-1")

	// Add a member to track
	hm.members["member-2"] = &MemberHealth{
		MemberID:     "member-2",
		Status:       HealthStatusUnhealthy,
		CheckResults: make(map[HealthCheckType]*HealthCheckResult),
	}

	// Record heartbeat
	hm.RecordHeartbeat("member-2")

	health, err := hm.GetMemberHealth("member-2")
	if err != nil {
		t.Fatalf("GetMemberHealth() error = %v", err)
	}

	if health.Status != HealthStatusHealthy {
		t.Errorf("Status = %v, want %v", health.Status, HealthStatusHealthy)
	}

	if health.ConsecutiveFails != 0 {
		t.Errorf("ConsecutiveFails = %v, want 0", health.ConsecutiveFails)
	}
}

func TestHealthMonitor_GetMemberHealth_NotFound(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	hm, _ := NewHealthMonitor(config, mm, etcd, "member-1")

	_, err := hm.GetMemberHealth("nonexistent")
	if err == nil {
		t.Error("GetMemberHealth() should return error for nonexistent member")
	}
}

func TestHealthMonitor_GetAllMemberHealth(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	hm, _ := NewHealthMonitor(config, mm, etcd, "member-1")

	// Add members
	hm.members["member-1"] = &MemberHealth{
		MemberID:     "member-1",
		Status:       HealthStatusHealthy,
		CheckResults: make(map[HealthCheckType]*HealthCheckResult),
	}
	hm.members["member-2"] = &MemberHealth{
		MemberID:     "member-2",
		Status:       HealthStatusDegraded,
		CheckResults: make(map[HealthCheckType]*HealthCheckResult),
	}

	health := hm.GetAllMemberHealth()

	if len(health) != 2 {
		t.Errorf("GetAllMemberHealth() returned %d members, want 2", len(health))
	}
}

func TestHealthMonitor_GetHealthyMemberCount(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	hm, _ := NewHealthMonitor(config, mm, etcd, "member-1")

	// Add members with different statuses
	hm.members["member-1"] = &MemberHealth{
		MemberID:     "member-1",
		Status:       HealthStatusHealthy,
		CheckResults: make(map[HealthCheckType]*HealthCheckResult),
	}
	hm.members["member-2"] = &MemberHealth{
		MemberID:     "member-2",
		Status:       HealthStatusDegraded, // Degraded counts as healthy
		CheckResults: make(map[HealthCheckType]*HealthCheckResult),
	}
	hm.members["member-3"] = &MemberHealth{
		MemberID:     "member-3",
		Status:       HealthStatusUnhealthy,
		CheckResults: make(map[HealthCheckType]*HealthCheckResult),
	}

	count := hm.GetHealthyMemberCount()
	if count != 2 {
		t.Errorf("GetHealthyMemberCount() = %d, want 2", count)
	}
}

func TestHealthMonitor_GetFailedMembers(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	hm, _ := NewHealthMonitor(config, mm, etcd, "member-1")

	hm.members["member-1"] = &MemberHealth{
		MemberID:     "member-1",
		Status:       HealthStatusHealthy,
		CheckResults: make(map[HealthCheckType]*HealthCheckResult),
	}
	hm.members["member-2"] = &MemberHealth{
		MemberID:     "member-2",
		Status:       HealthStatusUnhealthy,
		CheckResults: make(map[HealthCheckType]*HealthCheckResult),
	}

	failed := hm.GetFailedMembers()
	if len(failed) != 1 {
		t.Errorf("GetFailedMembers() returned %d, want 1", len(failed))
	}
	if len(failed) > 0 && failed[0] != "member-2" {
		t.Errorf("GetFailedMembers() = %v, want [member-2]", failed)
	}
}

func TestHealthMonitor_GetDegradedMembers(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	hm, _ := NewHealthMonitor(config, mm, etcd, "member-1")

	hm.members["member-1"] = &MemberHealth{
		MemberID:     "member-1",
		Status:       HealthStatusHealthy,
		CheckResults: make(map[HealthCheckType]*HealthCheckResult),
	}
	hm.members["member-2"] = &MemberHealth{
		MemberID:     "member-2",
		Status:       HealthStatusDegraded,
		CheckResults: make(map[HealthCheckType]*HealthCheckResult),
	}

	degraded := hm.GetDegradedMembers()
	if len(degraded) != 1 {
		t.Errorf("GetDegradedMembers() returned %d, want 1", len(degraded))
	}
}

func TestHealthMonitor_AddObserver(t *testing.T) {
	config := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	etcd := &EtcdClient{}
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	hm, _ := NewHealthMonitor(config, mm, etcd, "member-1")

	called := false
	observer := func(event HealthEvent) {
		called = true
	}

	hm.AddObserver(observer)

	if len(hm.observers) != 1 {
		t.Errorf("AddObserver() observers count = %d, want 1", len(hm.observers))
	}

	// Suppress unused variable warning
	_ = called
}

func TestDefaultHealthMonitorConfig(t *testing.T) {
	config := DefaultHealthMonitorConfig()

	if config.CheckInterval != defaultHealthCheckInterval {
		t.Errorf("CheckInterval = %v, want %v", config.CheckInterval, defaultHealthCheckInterval)
	}
	if config.FailureThreshold != defaultFailureThreshold {
		t.Errorf("FailureThreshold = %d, want %d", config.FailureThreshold, defaultFailureThreshold)
	}
	if config.SlowThreshold != defaultSlowThreshold {
		t.Errorf("SlowThreshold = %v, want %v", config.SlowThreshold, defaultSlowThreshold)
	}
}

func TestHealthCheckResult_Fields(t *testing.T) {
	result := &HealthCheckResult{
		Type:      HealthCheckEtcd,
		Status:    HealthStatusHealthy,
		Latency:   100 * time.Millisecond,
		Message:   "test message",
		Timestamp: time.Now(),
		Error:     nil,
	}

	if result.Type != HealthCheckEtcd {
		t.Errorf("Type = %v, want %v", result.Type, HealthCheckEtcd)
	}
	if result.Status != HealthStatusHealthy {
		t.Errorf("Status = %v, want %v", result.Status, HealthStatusHealthy)
	}
	if result.Latency != 100*time.Millisecond {
		t.Errorf("Latency = %v, want 100ms", result.Latency)
	}
}

func TestMemberHealth_Fields(t *testing.T) {
	now := time.Now()
	health := &MemberHealth{
		MemberID:         "member-1",
		Status:           HealthStatusHealthy,
		LastHeartbeat:    now,
		LastHealthCheck:  now,
		ConsecutiveFails: 0,
		LatencyP50:       50 * time.Millisecond,
		LatencyP99:       200 * time.Millisecond,
		CheckResults:     make(map[HealthCheckType]*HealthCheckResult),
	}

	if health.MemberID != "member-1" {
		t.Errorf("MemberID = %v, want member-1", health.MemberID)
	}
	if health.Status != HealthStatusHealthy {
		t.Errorf("Status = %v, want %v", health.Status, HealthStatusHealthy)
	}
}

func TestEtcdHealthChecker(t *testing.T) {
	etcd := &EtcdClient{}
	checker := NewEtcdHealthChecker(etcd)

	if checker.Name() != HealthCheckEtcd {
		t.Errorf("Name() = %v, want %v", checker.Name(), HealthCheckEtcd)
	}

	if checker.Interval() != 5*time.Second {
		t.Errorf("Interval() = %v, want 5s", checker.Interval())
	}

	// Test check when not connected
	ctx := context.Background()
	result := checker.Check(ctx)

	if result.Status != HealthStatusUnhealthy {
		t.Errorf("Check() status = %v, want %v (not connected)", result.Status, HealthStatusUnhealthy)
	}
}

func TestHealthEvent_Fields(t *testing.T) {
	event := HealthEvent{
		Type:         HealthEventMemberFailed,
		MemberID:     "member-1",
		OldStatus:    HealthStatusHealthy,
		NewStatus:    HealthStatusUnhealthy,
		Reason:       "heartbeat timeout",
		Timestamp:    time.Now(),
		CheckResults: make(map[HealthCheckType]*HealthCheckResult),
	}

	if event.Type != HealthEventMemberFailed {
		t.Errorf("Type = %v, want %v", event.Type, HealthEventMemberFailed)
	}
	if event.MemberID != "member-1" {
		t.Errorf("MemberID = %v, want member-1", event.MemberID)
	}
	if event.OldStatus != HealthStatusHealthy {
		t.Errorf("OldStatus = %v, want %v", event.OldStatus, HealthStatusHealthy)
	}
	if event.NewStatus != HealthStatusUnhealthy {
		t.Errorf("NewStatus = %v, want %v", event.NewStatus, HealthStatusUnhealthy)
	}
}
