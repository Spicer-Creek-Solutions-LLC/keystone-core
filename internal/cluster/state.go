package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

const (
	// Key prefixes for cluster state
	stateKeyPrefix     = "/state/"
	configKeyPrefix    = "/config/"
	leaderKeyPrefix    = "/leader/"
	shardKeyPrefix     = "/shards/"
	lockKeyPrefix      = "/locks/"
	coordinationPrefix = "/coordination/"
)

// StateStore provides cluster state storage using etcd.
type StateStore struct {
	etcd     *EtcdClient
	mu       sync.RWMutex
	watchers map[string][]func(key string, value []byte, deleted bool)
}

// NewStateStore creates a new cluster state store.
func NewStateStore(etcd *EtcdClient) (*StateStore, error) {
	if etcd == nil {
		return nil, fmt.Errorf("etcd client is required")
	}

	return &StateStore{
		etcd:     etcd,
		watchers: make(map[string][]func(key string, value []byte, deleted bool)),
	}, nil
}

// Get retrieves a value by key.
func (s *StateStore) Get(ctx context.Context, key string) ([]byte, error) {
	return s.etcd.Get(ctx, stateKeyPrefix+key)
}

// Set stores a value with optional TTL.
func (s *StateStore) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return s.etcd.Put(ctx, stateKeyPrefix+key, value, ttl)
}

// Delete removes a key.
func (s *StateStore) Delete(ctx context.Context, key string) error {
	return s.etcd.Delete(ctx, stateKeyPrefix+key)
}

// Watch watches for changes to a key prefix.
func (s *StateStore) Watch(ctx context.Context, prefix string, handler func(key string, value []byte, deleted bool)) error {
	// Register the watcher
	s.mu.Lock()
	s.watchers[prefix] = append(s.watchers[prefix], handler)
	s.mu.Unlock()

	return s.etcd.Watch(ctx, stateKeyPrefix+prefix, handler)
}

// List returns all keys with a given prefix.
func (s *StateStore) List(ctx context.Context, prefix string) (map[string][]byte, error) {
	return s.etcd.List(ctx, stateKeyPrefix+prefix)
}

// CompareAndSwap atomically updates a key if it matches the expected value.
func (s *StateStore) CompareAndSwap(ctx context.Context, key string, expected, value []byte) (bool, error) {
	return s.etcd.CompareAndSwap(ctx, stateKeyPrefix+key, expected, value)
}

// ConfigStore provides cluster configuration storage.
type ConfigStore struct {
	etcd *EtcdClient
}

// NewConfigStore creates a new cluster config store.
func NewConfigStore(etcd *EtcdClient) (*ConfigStore, error) {
	if etcd == nil {
		return nil, fmt.Errorf("etcd client is required")
	}

	return &ConfigStore{etcd: etcd}, nil
}

// GetConfig retrieves a configuration value.
func (s *ConfigStore) GetConfig(ctx context.Context, key string) ([]byte, error) {
	return s.etcd.Get(ctx, configKeyPrefix+key)
}

// SetConfig stores a configuration value.
func (s *ConfigStore) SetConfig(ctx context.Context, key string, value []byte) error {
	return s.etcd.Put(ctx, configKeyPrefix+key, value, 0)
}

// DeleteConfig removes a configuration value.
func (s *ConfigStore) DeleteConfig(ctx context.Context, key string) error {
	return s.etcd.Delete(ctx, configKeyPrefix+key)
}

// ListConfigs returns all configuration keys with a given prefix.
func (s *ConfigStore) ListConfigs(ctx context.Context, prefix string) (map[string][]byte, error) {
	return s.etcd.List(ctx, configKeyPrefix+prefix)
}

// WatchConfig watches for configuration changes.
func (s *ConfigStore) WatchConfig(ctx context.Context, prefix string, handler func(key string, value []byte, deleted bool)) error {
	return s.etcd.Watch(ctx, configKeyPrefix+prefix, handler)
}

// ShardStore manages shard assignments.
type ShardStore struct {
	etcd *EtcdClient
}

// NewShardStore creates a new shard store.
func NewShardStore(etcd *EtcdClient) (*ShardStore, error) {
	if etcd == nil {
		return nil, fmt.Errorf("etcd client is required")
	}

	return &ShardStore{etcd: etcd}, nil
}

// GetAssignment retrieves a shard assignment.
func (s *ShardStore) GetAssignment(ctx context.Context, agentID string) (*ShardAssignment, error) {
	data, err := s.etcd.Get(ctx, shardKeyPrefix+agentID)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}

	var assignment ShardAssignment
	if err := json.Unmarshal(data, &assignment); err != nil {
		return nil, fmt.Errorf("failed to unmarshal assignment: %w", err)
	}

	return &assignment, nil
}

// SetAssignment stores a shard assignment.
func (s *ShardStore) SetAssignment(ctx context.Context, assignment *ShardAssignment) error {
	data, err := json.Marshal(assignment)
	if err != nil {
		return fmt.Errorf("failed to marshal assignment: %w", err)
	}

	return s.etcd.Put(ctx, shardKeyPrefix+assignment.AgentID, data, 0)
}

// DeleteAssignment removes a shard assignment.
func (s *ShardStore) DeleteAssignment(ctx context.Context, agentID string) error {
	return s.etcd.Delete(ctx, shardKeyPrefix+agentID)
}

// ListAssignments returns all shard assignments.
func (s *ShardStore) ListAssignments(ctx context.Context) ([]*ShardAssignment, error) {
	data, err := s.etcd.List(ctx, shardKeyPrefix)
	if err != nil {
		return nil, err
	}

	assignments := make([]*ShardAssignment, 0, len(data))
	for _, value := range data {
		var assignment ShardAssignment
		if err := json.Unmarshal(value, &assignment); err != nil {
			continue // Skip invalid entries
		}
		assignments = append(assignments, &assignment)
	}

	return assignments, nil
}

// ListAssignmentsForMember returns all shard assignments for a member.
func (s *ShardStore) ListAssignmentsForMember(ctx context.Context, memberID string) ([]*ShardAssignment, error) {
	assignments, err := s.ListAssignments(ctx)
	if err != nil {
		return nil, err
	}

	filtered := make([]*ShardAssignment, 0)
	for _, a := range assignments {
		if a.MemberID == memberID {
			filtered = append(filtered, a)
		}
	}

	return filtered, nil
}

// WatchAssignments watches for shard assignment changes.
func (s *ShardStore) WatchAssignments(ctx context.Context, handler func(assignment *ShardAssignment, deleted bool)) error {
	return s.etcd.Watch(ctx, shardKeyPrefix, func(key string, value []byte, deleted bool) {
		if deleted {
			// Extract agent ID from key
			if len(key) > len(shardKeyPrefix) {
				agentID := key[len(shardKeyPrefix):]
				handler(&ShardAssignment{AgentID: agentID}, true)
			}
			return
		}

		var assignment ShardAssignment
		if err := json.Unmarshal(value, &assignment); err != nil {
			return
		}
		handler(&assignment, false)
	})
}

// CompareAndSwapAssignment atomically updates an assignment if version matches.
func (s *ShardStore) CompareAndSwapAssignment(ctx context.Context, assignment *ShardAssignment) (bool, error) {
	// Get current assignment
	current, err := s.GetAssignment(ctx, assignment.AgentID)
	if err != nil {
		return false, err
	}

	var expected []byte
	if current != nil {
		expected, err = json.Marshal(current)
		if err != nil {
			return false, fmt.Errorf("failed to marshal current assignment: %w", err)
		}
	}

	// Increment version
	assignment.Version++
	newValue, err := json.Marshal(assignment)
	if err != nil {
		return false, fmt.Errorf("failed to marshal new assignment: %w", err)
	}

	return s.etcd.CompareAndSwap(ctx, shardKeyPrefix+assignment.AgentID, expected, newValue)
}

// DistributedLock provides distributed locking using etcd.
type DistributedLock struct {
	etcd   *EtcdClient
	key    string
	mu     sync.Mutex
	locked bool
}

// NewDistributedLock creates a new distributed lock.
func NewDistributedLock(etcd *EtcdClient, name string) *DistributedLock {
	return &DistributedLock{
		etcd: etcd,
		key:  lockKeyPrefix + name,
	}
}

// Lock acquires the lock.
func (l *DistributedLock) Lock(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.locked {
		return fmt.Errorf("already holding lock")
	}

	// Try to acquire lock using compare-and-swap
	for {
		success, err := l.etcd.CompareAndSwap(ctx, l.key, nil, []byte("locked"))
		if err != nil {
			return fmt.Errorf("failed to acquire lock: %w", err)
		}

		if success {
			l.locked = true
			return nil
		}

		// Wait and retry
		if err := wait.ForContext(ctx, 100*time.Millisecond); err != nil {
			return err
		}
	}
}

// TryLock attempts to acquire the lock without blocking.
func (l *DistributedLock) TryLock(ctx context.Context) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.locked {
		return true, nil
	}

	success, err := l.etcd.CompareAndSwap(ctx, l.key, nil, []byte("locked"))
	if err != nil {
		return false, fmt.Errorf("failed to try lock: %w", err)
	}

	if success {
		l.locked = true
	}
	return success, nil
}

// Unlock releases the lock.
func (l *DistributedLock) Unlock(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.locked {
		return nil
	}

	if err := l.etcd.Delete(ctx, l.key); err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	l.locked = false
	return nil
}

// IsLocked returns true if the lock is currently held.
func (l *DistributedLock) IsLocked() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.locked
}

// CoordinationStore provides coordination primitives.
type CoordinationStore struct {
	etcd *EtcdClient
}

// NewCoordinationStore creates a new coordination store.
func NewCoordinationStore(etcd *EtcdClient) (*CoordinationStore, error) {
	if etcd == nil {
		return nil, fmt.Errorf("etcd client is required")
	}

	return &CoordinationStore{etcd: etcd}, nil
}

// Barrier waits for all members to reach a barrier point.
func (s *CoordinationStore) Barrier(ctx context.Context, name, memberID string, memberCount int) error {
	barrierKey := coordinationPrefix + "barriers/" + name + "/" + memberID

	// Register at the barrier
	if err := s.etcd.Put(ctx, barrierKey, []byte(memberID), 0); err != nil {
		return fmt.Errorf("failed to register at barrier: %w", err)
	}

	// Wait for all members
	for {
		data, err := s.etcd.List(ctx, coordinationPrefix+"barriers/"+name+"/")
		if err != nil {
			return fmt.Errorf("failed to check barrier: %w", err)
		}

		if len(data) >= memberCount {
			return nil
		}

		if err := wait.ForContext(ctx, 100*time.Millisecond); err != nil {
			return err
		}
	}
}

// Elect elects a coordinator for a given task.
func (s *CoordinationStore) Elect(ctx context.Context, name, memberID string) (bool, error) {
	electionKey := coordinationPrefix + "elections/" + name

	success, err := s.etcd.CompareAndSwap(ctx, electionKey, nil, []byte(memberID))
	if err != nil {
		return false, fmt.Errorf("failed to elect: %w", err)
	}

	return success, nil
}

// GetElected returns the elected coordinator for a task.
func (s *CoordinationStore) GetElected(ctx context.Context, name string) (string, error) {
	electionKey := coordinationPrefix + "elections/" + name

	data, err := s.etcd.Get(ctx, electionKey)
	if err != nil {
		return "", fmt.Errorf("failed to get elected: %w", err)
	}

	if data == nil {
		return "", nil
	}

	return string(data), nil
}

// Resign gives up the coordinator role.
func (s *CoordinationStore) Resign(ctx context.Context, name, memberID string) error {
	electionKey := coordinationPrefix + "elections/" + name

	// Only delete if we are the current leader
	success, err := s.etcd.CompareAndSwap(ctx, electionKey, []byte(memberID), nil)
	if err != nil {
		return fmt.Errorf("failed to resign: %w", err)
	}

	if !success {
		return fmt.Errorf("not the current coordinator")
	}

	return s.etcd.Delete(ctx, electionKey)
}

// Counter provides a distributed counter.
type Counter struct {
	etcd *EtcdClient
	key  string
}

// NewCounter creates a new distributed counter.
func NewCounter(etcd *EtcdClient, name string) *Counter {
	return &Counter{
		etcd: etcd,
		key:  coordinationPrefix + "counters/" + name,
	}
}

// Get returns the current counter value.
func (c *Counter) Get(ctx context.Context) (int64, error) {
	data, err := c.etcd.Get(ctx, c.key)
	if err != nil {
		return 0, err
	}

	if data == nil {
		return 0, nil
	}

	var value int64
	if err := json.Unmarshal(data, &value); err != nil {
		return 0, fmt.Errorf("failed to unmarshal counter: %w", err)
	}

	return value, nil
}

// Increment increments the counter and returns the new value.
func (c *Counter) Increment(ctx context.Context) (int64, error) {
	return c.Add(ctx, 1)
}

// Decrement decrements the counter and returns the new value.
func (c *Counter) Decrement(ctx context.Context) (int64, error) {
	return c.Add(ctx, -1)
}

// Add adds a value to the counter and returns the new value.
func (c *Counter) Add(ctx context.Context, delta int64) (int64, error) {
	for {
		current, err := c.Get(ctx)
		if err != nil {
			return 0, err
		}

		newValue := current + delta
		newData, err := json.Marshal(newValue)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal counter: %w", err)
		}

		var currentData []byte
		if current != 0 {
			currentData, err = json.Marshal(current)
			if err != nil {
				return 0, fmt.Errorf("failed to marshal current: %w", err)
			}
		}

		success, err := c.etcd.CompareAndSwap(ctx, c.key, currentData, newData)
		if err != nil {
			return 0, err
		}

		if success {
			return newValue, nil
		}

		// Retry on conflict
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
			continue
		}
	}
}

// Set sets the counter to a specific value.
func (c *Counter) Set(ctx context.Context, value int64) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal counter: %w", err)
	}

	return c.etcd.Put(ctx, c.key, data, 0)
}

// Reset resets the counter to zero.
func (c *Counter) Reset(ctx context.Context) error {
	return c.Set(ctx, 0)
}
