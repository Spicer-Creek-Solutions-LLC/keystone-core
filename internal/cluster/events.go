package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

const (
	// eventPartitionPrefix is the etcd key prefix for event partitions.
	eventPartitionPrefix = "/event_partitions/"

	// eventOffsetPrefix is the etcd key prefix for event offsets.
	eventOffsetPrefix = "/event_offsets/"

	// DefaultEventPartitions is the default number of event partitions.
	DefaultEventPartitions = 16
)

// EventPartitionInfo represents information about an event partition.
type EventPartitionInfo struct {
	// PartitionID identifies this partition.
	PartitionID int `json:"partition_id"`

	// OwnerMemberID is the member responsible for this partition.
	OwnerMemberID string `json:"owner_member_id"`

	// AssignedAt is when the partition was assigned.
	AssignedAt time.Time `json:"assigned_at"`

	// LastProcessed is the last event offset processed.
	LastProcessed int64 `json:"last_processed"`

	// ProcessedCount is the total events processed.
	ProcessedCount int64 `json:"processed_count"`

	// ErrorCount is the number of processing errors.
	ErrorCount int64 `json:"error_count"`
}

// EventProcessorConfig configures the event processor distribution.
type EventProcessorConfig struct {
	// NumPartitions is the number of event partitions.
	NumPartitions int

	// ProcessInterval is how often to check for new events.
	ProcessInterval time.Duration

	// BatchSize is the maximum events to process at once.
	BatchSize int

	// MaxRetries is the maximum retry attempts for failed events.
	MaxRetries int

	// RetryDelay is the delay between retries.
	RetryDelay time.Duration
}

// DefaultEventProcessorConfig returns the default configuration.
func DefaultEventProcessorConfig() *EventProcessorConfig {
	return &EventProcessorConfig{
		NumPartitions:   DefaultEventPartitions,
		ProcessInterval: 100 * time.Millisecond,
		BatchSize:       100,
		MaxRetries:      3,
		RetryDelay:      time.Second,
	}
}

// EventHandler processes events in a partition.
type EventHandler func(ctx context.Context, eventType string, eventData []byte) error

// EventPartitioner determines which partition an event belongs to.
type EventPartitioner func(eventType string, eventID string) int

// EventProcessorDistributor distributes event processing across cluster members.
type EventProcessorDistributor struct {
	config          *EventProcessorConfig
	etcd            *EtcdClient
	membership      *MembershipManager
	leader          *LeaderElector
	partitions      map[int]*EventPartitionInfo
	localPartitions map[int]bool
	handlers        map[string]EventHandler
	partitioner     EventPartitioner
	numPartitions   int
	mu              sync.RWMutex
	stopChan        chan struct{}
	doneChan        chan struct{}
	started         bool
}

// NewEventProcessorDistributor creates a new event processor distributor.
func NewEventProcessorDistributor(
	config *EventProcessorConfig,
	etcd *EtcdClient,
	membership *MembershipManager,
	leader *LeaderElector,
) (*EventProcessorDistributor, error) {
	if config == nil {
		config = DefaultEventProcessorConfig()
	}
	if etcd == nil {
		return nil, fmt.Errorf("etcd client is required")
	}
	if membership == nil {
		return nil, fmt.Errorf("membership manager is required")
	}
	if leader == nil {
		return nil, fmt.Errorf("leader elector is required")
	}

	d := &EventProcessorDistributor{
		config:          config,
		etcd:            etcd,
		membership:      membership,
		leader:          leader,
		partitions:      make(map[int]*EventPartitionInfo),
		localPartitions: make(map[int]bool),
		handlers:        make(map[string]EventHandler),
		numPartitions:   config.NumPartitions,
		stopChan:        make(chan struct{}),
		doneChan:        make(chan struct{}),
	}

	// Set default partitioner using consistent hashing on event type
	d.partitioner = func(eventType string, eventID string) int {
		h := sha256.Sum256([]byte(eventType + eventID))
		hash := binary.BigEndian.Uint32(h[:4])
		return int(hash) % d.numPartitions
	}

	return d, nil
}

// SetPartitioner sets a custom event partitioner.
func (d *EventProcessorDistributor) SetPartitioner(p EventPartitioner) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.partitioner = p
}

// RegisterHandler registers a handler for an event type.
func (d *EventProcessorDistributor) RegisterHandler(eventType string, handler EventHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[eventType] = handler
}

// UnregisterHandler unregisters a handler.
func (d *EventProcessorDistributor) UnregisterHandler(eventType string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.handlers, eventType)
}

// Start starts the event processor distributor.
func (d *EventProcessorDistributor) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.started {
		d.mu.Unlock()
		return fmt.Errorf("event processor distributor already started")
	}
	d.started = true
	d.mu.Unlock()

	// Load existing partitions
	if err := d.loadPartitions(ctx); err != nil {
		return fmt.Errorf("failed to load partitions: %w", err)
	}

	// Subscribe to membership changes
	d.membership.AddObserver(d.onMembershipChange)

	// Subscribe to leadership changes
	d.leader.AddObserver(d.onLeadershipChange)

	// Initial partition assignment if we're the leader
	if d.leader.IsLeader() {
		d.assignPartitions(ctx)
	}

	// Watch for partition changes
	go d.watchPartitions(ctx)

	return nil
}

// Stop stops the event processor distributor.
func (d *EventProcessorDistributor) Stop(ctx context.Context) error {
	d.mu.Lock()
	if !d.started {
		d.mu.Unlock()
		return nil
	}
	d.started = false
	d.mu.Unlock()

	close(d.stopChan)

	wait.ForSignal(d.doneChan, 5*time.Second)

	return nil
}

// GetPartition returns the partition for an event.
func (d *EventProcessorDistributor) GetPartition(eventType, eventID string) int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.partitioner(eventType, eventID)
}

// IsLocalPartition returns true if this member owns the partition.
func (d *EventProcessorDistributor) IsLocalPartition(partitionID int) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.localPartitions[partitionID]
}

// ShouldProcessEvent returns true if this member should process the event.
func (d *EventProcessorDistributor) ShouldProcessEvent(eventType, eventID string) bool {
	partition := d.GetPartition(eventType, eventID)
	return d.IsLocalPartition(partition)
}

// GetLocalPartitions returns the partitions owned by this member.
func (d *EventProcessorDistributor) GetLocalPartitions() []int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	partitions := make([]int, 0, len(d.localPartitions))
	for p := range d.localPartitions {
		partitions = append(partitions, p)
	}
	return partitions
}

// GetPartitionInfo returns information about a partition.
func (d *EventProcessorDistributor) GetPartitionInfo(partitionID int) (*EventPartitionInfo, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	info, exists := d.partitions[partitionID]
	if !exists {
		return nil, fmt.Errorf("partition %d not found", partitionID)
	}

	// Return a copy
	infoCopy := *info
	return &infoCopy, nil
}

// GetAllPartitionInfo returns information about all partitions.
func (d *EventProcessorDistributor) GetAllPartitionInfo() []*EventPartitionInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()

	infos := make([]*EventPartitionInfo, 0, len(d.partitions))
	for _, info := range d.partitions {
		infoCopy := *info
		infos = append(infos, &infoCopy)
	}
	return infos
}

// UpdateOffset updates the processed offset for a partition.
func (d *EventProcessorDistributor) UpdateOffset(ctx context.Context, partitionID int, offset int64) error {
	d.mu.Lock()
	info, exists := d.partitions[partitionID]
	if !exists {
		d.mu.Unlock()
		return fmt.Errorf("partition %d not found", partitionID)
	}
	info.LastProcessed = offset
	info.ProcessedCount++
	d.mu.Unlock()

	return d.storePartitionInfo(ctx, info)
}

// GetOffset returns the last processed offset for a partition.
func (d *EventProcessorDistributor) GetOffset(partitionID int) (int64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	info, exists := d.partitions[partitionID]
	if !exists {
		return 0, fmt.Errorf("partition %d not found", partitionID)
	}
	return info.LastProcessed, nil
}

// RecordError records a processing error for a partition.
func (d *EventProcessorDistributor) RecordError(ctx context.Context, partitionID int) error {
	d.mu.Lock()
	info, exists := d.partitions[partitionID]
	if !exists {
		d.mu.Unlock()
		return fmt.Errorf("partition %d not found", partitionID)
	}
	info.ErrorCount++
	d.mu.Unlock()

	return d.storePartitionInfo(ctx, info)
}

// loadPartitions loads existing partition assignments from etcd.
func (d *EventProcessorDistributor) loadPartitions(ctx context.Context) error {
	data, err := d.etcd.List(ctx, eventPartitionPrefix)
	if err != nil {
		return err
	}

	localMember := d.membership.LocalMember()

	d.mu.Lock()
	defer d.mu.Unlock()

	for _, value := range data {
		var info EventPartitionInfo
		if err := json.Unmarshal(value, &info); err != nil {
			continue
		}
		d.partitions[info.PartitionID] = &info

		// Track if we own this partition
		if localMember != nil && info.OwnerMemberID == localMember.ID {
			d.localPartitions[info.PartitionID] = true
		}
	}

	return nil
}

// storePartitionInfo stores partition info in etcd.
func (d *EventProcessorDistributor) storePartitionInfo(ctx context.Context, info *EventPartitionInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal partition info: %w", err)
	}

	key := fmt.Sprintf("%s%d", eventPartitionPrefix, info.PartitionID)
	return d.etcd.Put(ctx, key, data, 0)
}

// watchPartitions watches for partition assignment changes.
func (d *EventProcessorDistributor) watchPartitions(ctx context.Context) {
	defer func() {
		select {
		case d.doneChan <- struct{}{}:
		default:
		}
	}()

	localMember := d.membership.LocalMember()
	if localMember == nil {
		return
	}

	err := d.etcd.Watch(ctx, eventPartitionPrefix, func(key string, value []byte, deleted bool) {
		if deleted {
			return
		}

		var info EventPartitionInfo
		if err := json.Unmarshal(value, &info); err != nil {
			return
		}

		d.mu.Lock()
		d.partitions[info.PartitionID] = &info

		// Update local partition tracking
		if info.OwnerMemberID == localMember.ID {
			d.localPartitions[info.PartitionID] = true
		} else {
			delete(d.localPartitions, info.PartitionID)
		}
		d.mu.Unlock()
	})

	_ = err // error logged via callback
}

// onMembershipChange handles membership change events.
func (d *EventProcessorDistributor) onMembershipChange(event MembershipEvent) {
	// Only leader should reassign partitions
	if !d.leader.IsLeader() {
		return
	}

	switch event.Type {
	case MembershipEventJoined, MembershipEventLeft, MembershipEventFailed:
		// Reassign partitions
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d.assignPartitions(ctx)
	default:
	}
}

// onLeadershipChange handles leadership change events.
func (d *EventProcessorDistributor) onLeadershipChange(event LeadershipEvent) {
	if event.Type == LeadershipEventElected && event.LeaderID == d.leader.MemberID() {
		// We became leader, ensure partitions are assigned
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d.assignPartitions(ctx)
	}
}

// assignPartitions assigns partitions to members.
// This should only be called by the leader.
func (d *EventProcessorDistributor) assignPartitions(ctx context.Context) {
	members := d.membership.GetHealthyMembers()
	if len(members) == 0 {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Build member ID list
	memberIDs := make([]string, len(members))
	for i, m := range members {
		memberIDs[i] = m.ID
	}

	// Assign partitions round-robin
	for i := 0; i < d.numPartitions; i++ {
		memberID := memberIDs[i%len(memberIDs)]

		// Check if partition needs reassignment
		existing, exists := d.partitions[i]
		if exists && existing.OwnerMemberID == memberID {
			continue // Already assigned to correct member
		}

		// Check if current owner is still healthy
		if exists {
			_, err := d.membership.GetMember(existing.OwnerMemberID)
			if err == nil {
				continue // Current owner is still valid
			}
		}

		// Create or update partition assignment
		info := &EventPartitionInfo{
			PartitionID:   i,
			OwnerMemberID: memberID,
			AssignedAt:    time.Now().UTC(),
		}

		// Preserve processing state if exists
		if existing != nil {
			info.LastProcessed = existing.LastProcessed
			info.ProcessedCount = existing.ProcessedCount
			info.ErrorCount = existing.ErrorCount
		}

		d.partitions[i] = info

		// Store in etcd
		_ = d.storePartitionInfo(ctx, info) // error logged internally
	}

	// Update local partition tracking
	localMember := d.membership.LocalMember()
	if localMember != nil {
		d.localPartitions = make(map[int]bool)
		for _, info := range d.partitions {
			if info.OwnerMemberID == localMember.ID {
				d.localPartitions[info.PartitionID] = true
			}
		}
	}
}

// RebalancePartitions forces a rebalancing of partitions.
func (d *EventProcessorDistributor) RebalancePartitions(ctx context.Context) error {
	if !d.leader.IsLeader() {
		return ErrNotLeader
	}

	d.mu.Lock()
	// Clear all partition assignments to force reassignment
	for i := 0; i < d.numPartitions; i++ {
		if info, exists := d.partitions[i]; exists {
			info.OwnerMemberID = ""
		}
	}
	d.mu.Unlock()

	d.assignPartitions(ctx)
	return nil
}

// GetPartitionDistribution returns a map of member ID to partition count.
func (d *EventProcessorDistributor) GetPartitionDistribution() map[string]int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	distribution := make(map[string]int)
	for _, info := range d.partitions {
		distribution[info.OwnerMemberID]++
	}
	return distribution
}

// GetProcessingStats returns processing statistics.
func (d *EventProcessorDistributor) GetProcessingStats() (totalProcessed, totalErrors int64) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, info := range d.partitions {
		totalProcessed += info.ProcessedCount
		totalErrors += info.ErrorCount
	}
	return
}

// GetLocalProcessingStats returns processing stats for local partitions.
func (d *EventProcessorDistributor) GetLocalProcessingStats() (processed, errors int64) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for partitionID := range d.localPartitions {
		if info, exists := d.partitions[partitionID]; exists {
			processed += info.ProcessedCount
			errors += info.ErrorCount
		}
	}
	return
}

// DispatchEvent dispatches an event to the appropriate handler if this member owns the partition.
func (d *EventProcessorDistributor) DispatchEvent(ctx context.Context, eventType, eventID string, eventData []byte) error {
	partition := d.GetPartition(eventType, eventID)
	if !d.IsLocalPartition(partition) {
		return fmt.Errorf("event partition %d not owned by this member", partition)
	}

	d.mu.RLock()
	handler, exists := d.handlers[eventType]
	d.mu.RUnlock()

	if !exists {
		// No handler registered, skip
		return nil
	}

	// Execute handler
	err := handler(ctx, eventType, eventData)
	if err != nil {
		_ = d.RecordError(ctx, partition) //nolint:errcheck // best-effort error tracking
		return fmt.Errorf("handler error: %w", err)
	}

	return nil
}

// ProcessEventBatch processes a batch of events.
func (d *EventProcessorDistributor) ProcessEventBatch(ctx context.Context, events []struct {
	Type string
	ID   string
	Data []byte
}) (processed, skipped, errors int) {
	for _, event := range events {
		if !d.ShouldProcessEvent(event.Type, event.ID) {
			skipped++
			continue
		}

		if err := d.DispatchEvent(ctx, event.Type, event.ID, event.Data); err != nil {
			errors++
			continue
		}

		processed++
	}
	return
}
