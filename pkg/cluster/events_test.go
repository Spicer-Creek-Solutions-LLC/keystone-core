package cluster

import (
	"context"
	"testing"
	"time"
)

func TestDefaultEventPartitions(t *testing.T) {
	if DefaultEventPartitions != 16 {
		t.Errorf("DefaultEventPartitions = %v, want 16", DefaultEventPartitions)
	}
}

func TestEventPartitionPrefix(t *testing.T) {
	if eventPartitionPrefix != "/event_partitions/" {
		t.Errorf("eventPartitionPrefix = %v, want '/event_partitions/'", eventPartitionPrefix)
	}
}

func TestEventOffsetPrefix(t *testing.T) {
	if eventOffsetPrefix != "/event_offsets/" {
		t.Errorf("eventOffsetPrefix = %v, want '/event_offsets/'", eventOffsetPrefix)
	}
}

func TestDefaultEventProcessorConfig(t *testing.T) {
	config := DefaultEventProcessorConfig()

	if config == nil {
		t.Fatal("DefaultEventProcessorConfig() returned nil")
	}

	if config.NumPartitions != DefaultEventPartitions {
		t.Errorf("NumPartitions = %v, want %v", config.NumPartitions, DefaultEventPartitions)
	}

	if config.ProcessInterval != 100*time.Millisecond {
		t.Errorf("ProcessInterval = %v, want 100ms", config.ProcessInterval)
	}

	if config.BatchSize != 100 {
		t.Errorf("BatchSize = %v, want 100", config.BatchSize)
	}

	if config.MaxRetries != 3 {
		t.Errorf("MaxRetries = %v, want 3", config.MaxRetries)
	}

	if config.RetryDelay != time.Second {
		t.Errorf("RetryDelay = %v, want 1s", config.RetryDelay)
	}
}

func TestEventPartitionInfo_Fields(t *testing.T) {
	now := time.Now()
	info := &EventPartitionInfo{
		PartitionID:    5,
		OwnerMemberID:  "member-1",
		AssignedAt:     now,
		LastProcessed:  1000,
		ProcessedCount: 5000,
		ErrorCount:     10,
	}

	if info.PartitionID != 5 {
		t.Errorf("PartitionID = %v, want 5", info.PartitionID)
	}
	if info.OwnerMemberID != "member-1" {
		t.Errorf("OwnerMemberID = %v, want 'member-1'", info.OwnerMemberID)
	}
	if info.LastProcessed != 1000 {
		t.Errorf("LastProcessed = %v, want 1000", info.LastProcessed)
	}
	if info.ProcessedCount != 5000 {
		t.Errorf("ProcessedCount = %v, want 5000", info.ProcessedCount)
	}
	if info.ErrorCount != 10 {
		t.Errorf("ErrorCount = %v, want 10", info.ErrorCount)
	}
}

func TestEventProcessorConfig_Fields(t *testing.T) {
	config := &EventProcessorConfig{
		NumPartitions:   32,
		ProcessInterval: 50 * time.Millisecond,
		BatchSize:       50,
		MaxRetries:      5,
		RetryDelay:      2 * time.Second,
	}

	if config.NumPartitions != 32 {
		t.Errorf("NumPartitions = %v, want 32", config.NumPartitions)
	}
	if config.ProcessInterval != 50*time.Millisecond {
		t.Errorf("ProcessInterval = %v, want 50ms", config.ProcessInterval)
	}
	if config.BatchSize != 50 {
		t.Errorf("BatchSize = %v, want 50", config.BatchSize)
	}
	if config.MaxRetries != 5 {
		t.Errorf("MaxRetries = %v, want 5", config.MaxRetries)
	}
	if config.RetryDelay != 2*time.Second {
		t.Errorf("RetryDelay = %v, want 2s", config.RetryDelay)
	}
}

func TestNewEventProcessorDistributor_Validation(t *testing.T) {
	config := DefaultEventProcessorConfig()
	etcd := &EtcdClient{}

	clusterConfig := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	mm := &MembershipManager{
		config:   clusterConfig,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	leaderConfig := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	le, _ := NewLeaderElector(leaderConfig, etcd, "member-1")

	tests := []struct {
		name       string
		config     *EventProcessorConfig
		etcd       *EtcdClient
		membership *MembershipManager
		leader     *LeaderElector
		wantErr    bool
	}{
		{
			name:       "nil etcd",
			config:     config,
			etcd:       nil,
			membership: mm,
			leader:     le,
			wantErr:    true,
		},
		{
			name:       "nil membership",
			config:     config,
			etcd:       etcd,
			membership: nil,
			leader:     le,
			wantErr:    true,
		},
		{
			name:       "nil leader",
			config:     config,
			etcd:       etcd,
			membership: mm,
			leader:     nil,
			wantErr:    true,
		},
		{
			name:       "nil config uses default",
			config:     nil,
			etcd:       etcd,
			membership: mm,
			leader:     le,
			wantErr:    false,
		},
		{
			name:       "valid config",
			config:     config,
			etcd:       etcd,
			membership: mm,
			leader:     le,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			epd, err := NewEventProcessorDistributor(tt.config, tt.etcd, tt.membership, tt.leader)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewEventProcessorDistributor() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && epd == nil {
				t.Error("NewEventProcessorDistributor() returned nil without error")
			}
		})
	}
}

func TestEventProcessorDistributor_RegisterHandler(t *testing.T) {
	config := DefaultEventProcessorConfig()
	etcd := &EtcdClient{}
	clusterConfig := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	mm := &MembershipManager{
		config:   clusterConfig,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	leaderConfig := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	le, _ := NewLeaderElector(leaderConfig, etcd, "member-1")

	epd, _ := NewEventProcessorDistributor(config, etcd, mm, le)

	called := false
	handler := func(ctx context.Context, eventType string, eventData []byte) error {
		called = true
		return nil
	}

	epd.RegisterHandler("test.event", handler)

	epd.mu.RLock()
	_, exists := epd.handlers["test.event"]
	epd.mu.RUnlock()

	if !exists {
		t.Error("Handler was not registered")
	}

	epd.UnregisterHandler("test.event")

	epd.mu.RLock()
	_, exists = epd.handlers["test.event"]
	epd.mu.RUnlock()

	if exists {
		t.Error("Handler was not unregistered")
	}

	// Suppress unused variable warning
	_ = called
}

func TestEventProcessorDistributor_GetPartition(t *testing.T) {
	config := DefaultEventProcessorConfig()
	etcd := &EtcdClient{}
	clusterConfig := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	mm := &MembershipManager{
		config:   clusterConfig,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	leaderConfig := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	le, _ := NewLeaderElector(leaderConfig, etcd, "member-1")

	epd, _ := NewEventProcessorDistributor(config, etcd, mm, le)

	// Get partition for an event
	partition := epd.GetPartition("test.event", "event-123")

	// Partition should be within range
	if partition < 0 || partition >= config.NumPartitions {
		t.Errorf("GetPartition() = %v, want 0-%v", partition, config.NumPartitions-1)
	}

	// Same event should always get same partition
	partition2 := epd.GetPartition("test.event", "event-123")
	if partition != partition2 {
		t.Errorf("GetPartition() not consistent: %v != %v", partition, partition2)
	}

	// Different events may get different partitions
	partitions := make(map[int]bool)
	for i := 0; i < 100; i++ {
		p := epd.GetPartition("event.type", string(rune(i)))
		partitions[p] = true
	}

	// Should use multiple partitions (with 100 events and 16 partitions)
	if len(partitions) < 5 {
		t.Errorf("Poor partition distribution: only %v partitions used", len(partitions))
	}
}

func TestEventProcessorDistributor_SetPartitioner(t *testing.T) {
	config := DefaultEventProcessorConfig()
	etcd := &EtcdClient{}
	clusterConfig := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	mm := &MembershipManager{
		config:   clusterConfig,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	leaderConfig := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	le, _ := NewLeaderElector(leaderConfig, etcd, "member-1")

	epd, _ := NewEventProcessorDistributor(config, etcd, mm, le)

	// Custom partitioner that always returns 0
	epd.SetPartitioner(func(eventType, eventID string) int {
		return 0
	})

	partition := epd.GetPartition("any.event", "any-id")
	if partition != 0 {
		t.Errorf("GetPartition() with custom partitioner = %v, want 0", partition)
	}
}

func TestEventProcessorDistributor_LocalPartitions(t *testing.T) {
	config := DefaultEventProcessorConfig()
	etcd := &EtcdClient{}
	clusterConfig := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	mm := &MembershipManager{
		config:   clusterConfig,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	leaderConfig := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	le, _ := NewLeaderElector(leaderConfig, etcd, "member-1")

	epd, _ := NewEventProcessorDistributor(config, etcd, mm, le)

	// Initially no local partitions
	localPartitions := epd.GetLocalPartitions()
	if len(localPartitions) != 0 {
		t.Errorf("GetLocalPartitions() initially = %v, want empty", len(localPartitions))
	}

	// Manually add local partitions
	epd.mu.Lock()
	epd.localPartitions[0] = true
	epd.localPartitions[5] = true
	epd.mu.Unlock()

	localPartitions = epd.GetLocalPartitions()
	if len(localPartitions) != 2 {
		t.Errorf("GetLocalPartitions() = %v, want 2", len(localPartitions))
	}

	if !epd.IsLocalPartition(0) {
		t.Error("IsLocalPartition(0) should be true")
	}
	if !epd.IsLocalPartition(5) {
		t.Error("IsLocalPartition(5) should be true")
	}
	if epd.IsLocalPartition(1) {
		t.Error("IsLocalPartition(1) should be false")
	}
}

func TestEventProcessorDistributor_GetPartitionInfo(t *testing.T) {
	config := DefaultEventProcessorConfig()
	etcd := &EtcdClient{}
	clusterConfig := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	mm := &MembershipManager{
		config:   clusterConfig,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	leaderConfig := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	le, _ := NewLeaderElector(leaderConfig, etcd, "member-1")

	epd, _ := NewEventProcessorDistributor(config, etcd, mm, le)

	// Non-existent partition
	_, err := epd.GetPartitionInfo(0)
	if err == nil {
		t.Error("GetPartitionInfo() should fail for non-existent partition")
	}

	// Add partition info
	now := time.Now()
	epd.mu.Lock()
	epd.partitions[0] = &EventPartitionInfo{
		PartitionID:    0,
		OwnerMemberID:  "member-1",
		AssignedAt:     now,
		ProcessedCount: 100,
	}
	epd.mu.Unlock()

	info, err := epd.GetPartitionInfo(0)
	if err != nil {
		t.Errorf("GetPartitionInfo() error = %v", err)
	}

	if info.PartitionID != 0 {
		t.Errorf("PartitionID = %v, want 0", info.PartitionID)
	}
	if info.OwnerMemberID != "member-1" {
		t.Errorf("OwnerMemberID = %v, want 'member-1'", info.OwnerMemberID)
	}
	if info.ProcessedCount != 100 {
		t.Errorf("ProcessedCount = %v, want 100", info.ProcessedCount)
	}
}

func TestEventProcessorDistributor_GetAllPartitionInfo(t *testing.T) {
	config := DefaultEventProcessorConfig()
	etcd := &EtcdClient{}
	clusterConfig := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	mm := &MembershipManager{
		config:   clusterConfig,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	leaderConfig := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	le, _ := NewLeaderElector(leaderConfig, etcd, "member-1")

	epd, _ := NewEventProcessorDistributor(config, etcd, mm, le)

	// Initially empty
	infos := epd.GetAllPartitionInfo()
	if len(infos) != 0 {
		t.Errorf("GetAllPartitionInfo() initially = %v, want 0", len(infos))
	}

	// Add partition infos
	epd.mu.Lock()
	epd.partitions[0] = &EventPartitionInfo{PartitionID: 0, OwnerMemberID: "member-1"}
	epd.partitions[1] = &EventPartitionInfo{PartitionID: 1, OwnerMemberID: "member-2"}
	epd.mu.Unlock()

	infos = epd.GetAllPartitionInfo()
	if len(infos) != 2 {
		t.Errorf("GetAllPartitionInfo() = %v, want 2", len(infos))
	}
}

func TestEventProcessorDistributor_GetPartitionDistribution(t *testing.T) {
	config := DefaultEventProcessorConfig()
	etcd := &EtcdClient{}
	clusterConfig := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	mm := &MembershipManager{
		config:   clusterConfig,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	leaderConfig := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	le, _ := NewLeaderElector(leaderConfig, etcd, "member-1")

	epd, _ := NewEventProcessorDistributor(config, etcd, mm, le)

	// Add partitions assigned to different members
	epd.mu.Lock()
	epd.partitions[0] = &EventPartitionInfo{PartitionID: 0, OwnerMemberID: "member-1"}
	epd.partitions[1] = &EventPartitionInfo{PartitionID: 1, OwnerMemberID: "member-1"}
	epd.partitions[2] = &EventPartitionInfo{PartitionID: 2, OwnerMemberID: "member-2"}
	epd.mu.Unlock()

	distribution := epd.GetPartitionDistribution()

	if distribution["member-1"] != 2 {
		t.Errorf("member-1 partition count = %v, want 2", distribution["member-1"])
	}
	if distribution["member-2"] != 1 {
		t.Errorf("member-2 partition count = %v, want 1", distribution["member-2"])
	}
}

func TestEventProcessorDistributor_GetProcessingStats(t *testing.T) {
	config := DefaultEventProcessorConfig()
	etcd := &EtcdClient{}
	clusterConfig := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	mm := &MembershipManager{
		config:   clusterConfig,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	leaderConfig := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	le, _ := NewLeaderElector(leaderConfig, etcd, "member-1")

	epd, _ := NewEventProcessorDistributor(config, etcd, mm, le)

	// Add partitions with stats
	epd.mu.Lock()
	epd.partitions[0] = &EventPartitionInfo{ProcessedCount: 100, ErrorCount: 5}
	epd.partitions[1] = &EventPartitionInfo{ProcessedCount: 200, ErrorCount: 10}
	epd.mu.Unlock()

	processed, errors := epd.GetProcessingStats()

	if processed != 300 {
		t.Errorf("Total processed = %v, want 300", processed)
	}
	if errors != 15 {
		t.Errorf("Total errors = %v, want 15", errors)
	}
}

func TestEventProcessorDistributor_ShouldProcessEvent(t *testing.T) {
	config := DefaultEventProcessorConfig()
	etcd := &EtcdClient{}
	clusterConfig := &Config{ClusterName: "test", HeartbeatInterval: time.Second}
	mm := &MembershipManager{
		config:   clusterConfig,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	leaderConfig := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	le, _ := NewLeaderElector(leaderConfig, etcd, "member-1")

	epd, _ := NewEventProcessorDistributor(config, etcd, mm, le)

	// Set local partitions
	epd.mu.Lock()
	epd.localPartitions[0] = true
	epd.localPartitions[5] = true
	epd.mu.Unlock()

	// Custom partitioner for predictable testing
	epd.SetPartitioner(func(eventType, eventID string) int {
		if eventID == "local-event" {
			return 0
		}
		return 10
	})

	if !epd.ShouldProcessEvent("test", "local-event") {
		t.Error("ShouldProcessEvent() should return true for local partition")
	}

	if epd.ShouldProcessEvent("test", "remote-event") {
		t.Error("ShouldProcessEvent() should return false for non-local partition")
	}
}
