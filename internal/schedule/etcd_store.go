package schedule

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/cluster"
)

const (
	// Key prefixes for schedule storage.
	scheduleKeyPrefix           = "/schedules/"
	executionKeyPrefix          = "/schedule_executions/"
	maintenanceWindowKeyPrefix  = "/maintenance_windows/"
	scheduleLockKeyPrefix       = "/schedule_locks/"
)

// EtcdStore implements the Store interface using etcd.
type EtcdStore struct {
	etcd     *cluster.EtcdClient
	memberID string
	mu       sync.RWMutex
	closed   bool
}

// NewEtcdStore creates a new etcd-backed schedule store.
func NewEtcdStore(etcd *cluster.EtcdClient, memberID string) (*EtcdStore, error) {
	if etcd == nil {
		return nil, fmt.Errorf("etcd client is required")
	}
	if memberID == "" {
		return nil, fmt.Errorf("member ID is required")
	}

	return &EtcdStore{
		etcd:     etcd,
		memberID: memberID,
	}, nil
}

// checkState checks if the store is in a valid state.
func (s *EtcdStore) checkState() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return ErrStoreClosed
	}
	if !s.etcd.IsConnected() {
		return ErrStoreNotConnected
	}
	return nil
}

// Schedule operations

// CreateSchedule creates a new schedule.
func (s *EtcdStore) CreateSchedule(ctx context.Context, schedule *Schedule) error {
	if err := s.checkState(); err != nil {
		return err
	}
	if schedule == nil {
		return fmt.Errorf("schedule is required")
	}
	if schedule.ID == "" {
		return fmt.Errorf("schedule ID is required")
	}

	key := scheduleKeyPrefix + schedule.ID

	// Check if schedule already exists
	existing, err := s.etcd.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check existing schedule: %w", err)
	}
	if existing != nil {
		return ErrScheduleExists
	}

	data, err := json.Marshal(schedule)
	if err != nil {
		return fmt.Errorf("failed to marshal schedule: %w", err)
	}

	if err := s.etcd.Put(ctx, key, data, 0); err != nil {
		return fmt.Errorf("failed to store schedule: %w", err)
	}

	return nil
}

// GetSchedule retrieves a schedule by ID.
func (s *EtcdStore) GetSchedule(ctx context.Context, id string) (*Schedule, error) {
	if err := s.checkState(); err != nil {
		return nil, err
	}

	data, err := s.etcd.Get(ctx, scheduleKeyPrefix+id)
	if err != nil {
		return nil, fmt.Errorf("failed to get schedule: %w", err)
	}
	if data == nil {
		return nil, ErrScheduleNotFound
	}

	var schedule Schedule
	if err := json.Unmarshal(data, &schedule); err != nil {
		return nil, fmt.Errorf("failed to unmarshal schedule: %w", err)
	}

	return &schedule, nil
}

// UpdateSchedule updates an existing schedule.
func (s *EtcdStore) UpdateSchedule(ctx context.Context, schedule *Schedule) error {
	if err := s.checkState(); err != nil {
		return err
	}
	if schedule == nil {
		return fmt.Errorf("schedule is required")
	}
	if schedule.ID == "" {
		return fmt.Errorf("schedule ID is required")
	}

	key := scheduleKeyPrefix + schedule.ID

	// Check if schedule exists
	existing, err := s.etcd.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check existing schedule: %w", err)
	}
	if existing == nil {
		return ErrScheduleNotFound
	}

	data, err := json.Marshal(schedule)
	if err != nil {
		return fmt.Errorf("failed to marshal schedule: %w", err)
	}

	if err := s.etcd.Put(ctx, key, data, 0); err != nil {
		return fmt.Errorf("failed to update schedule: %w", err)
	}

	return nil
}

// DeleteSchedule deletes a schedule by ID.
func (s *EtcdStore) DeleteSchedule(ctx context.Context, id string) error {
	if err := s.checkState(); err != nil {
		return err
	}

	key := scheduleKeyPrefix + id

	// Check if schedule exists
	existing, err := s.etcd.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check existing schedule: %w", err)
	}
	if existing == nil {
		return ErrScheduleNotFound
	}

	if err := s.etcd.Delete(ctx, key); err != nil {
		return fmt.Errorf("failed to delete schedule: %w", err)
	}

	return nil
}

// ListSchedules lists schedules matching the filter.
func (s *EtcdStore) ListSchedules(ctx context.Context, filter *ScheduleFilter) ([]*Schedule, error) {
	if err := s.checkState(); err != nil {
		return nil, err
	}

	data, err := s.etcd.List(ctx, scheduleKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list schedules: %w", err)
	}

	schedules := make([]*Schedule, 0, len(data))
	for _, value := range data {
		var schedule Schedule
		if err := json.Unmarshal(value, &schedule); err != nil {
			continue // Skip invalid entries
		}

		// Apply filter
		if filter != nil && !s.matchesScheduleFilter(&schedule, filter) {
			continue
		}

		schedules = append(schedules, &schedule)
	}

	// Sort by priority (descending), then by name
	sort.Slice(schedules, func(i, j int) bool {
		if schedules[i].Priority != schedules[j].Priority {
			return schedules[i].Priority > schedules[j].Priority
		}
		return schedules[i].Name < schedules[j].Name
	})

	// Apply pagination
	if filter != nil {
		if filter.Offset > 0 && filter.Offset < len(schedules) {
			schedules = schedules[filter.Offset:]
		} else if filter.Offset >= len(schedules) {
			return []*Schedule{}, nil
		}

		if filter.Limit > 0 && filter.Limit < len(schedules) {
			schedules = schedules[:filter.Limit]
		}
	}

	return schedules, nil
}

// matchesScheduleFilter checks if a schedule matches the filter criteria.
func (s *EtcdStore) matchesScheduleFilter(schedule *Schedule, filter *ScheduleFilter) bool {
	// Filter by status
	if len(filter.Status) > 0 {
		matched := false
		for _, status := range filter.Status {
			if schedule.Status == status {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Filter by type
	if len(filter.Type) > 0 {
		matched := false
		for _, t := range filter.Type {
			if schedule.Type == t {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Filter by name substring
	if filter.NameContains != "" {
		if !strings.Contains(strings.ToLower(schedule.Name), strings.ToLower(filter.NameContains)) {
			return false
		}
	}

	// Filter by labels
	if len(filter.Labels) > 0 {
		for k, v := range filter.Labels {
			if schedule.Labels == nil || schedule.Labels[k] != v {
				return false
			}
		}
	}

	// Filter by maintenance window
	if filter.MaintenanceWindowID != "" {
		if schedule.MaintenanceWindowID != filter.MaintenanceWindowID {
			return false
		}
	}

	return true
}

// WatchSchedules watches for schedule changes.
func (s *EtcdStore) WatchSchedules(ctx context.Context, handler ScheduleWatchHandler) error {
	if err := s.checkState(); err != nil {
		return err
	}

	return s.etcd.Watch(ctx, scheduleKeyPrefix, func(key string, value []byte, deleted bool) {
		event := &ScheduleWatchEvent{
			ScheduleID: extractID(key, scheduleKeyPrefix),
		}

		if deleted {
			event.Type = WatchEventDeleted
		} else {
			var schedule Schedule
			if err := json.Unmarshal(value, &schedule); err != nil {
				return // Skip invalid data
			}
			event.Schedule = &schedule

			// Determine if this is a create or update
			// For simplicity, we'll use updated for all non-delete events
			// A more sophisticated implementation would track creation revision
			event.Type = WatchEventUpdated
		}

		handler(event)
	})
}

// Execution operations

// CreateExecution creates a new execution record.
func (s *EtcdStore) CreateExecution(ctx context.Context, execution *ScheduleExecution) error {
	if err := s.checkState(); err != nil {
		return err
	}
	if execution == nil {
		return fmt.Errorf("execution is required")
	}
	if execution.ID == "" {
		return fmt.Errorf("execution ID is required")
	}

	// Use a composite key: scheduleID/executionID for easier querying
	key := executionKeyPrefix + execution.ScheduleID + "/" + execution.ID

	data, err := json.Marshal(execution)
	if err != nil {
		return fmt.Errorf("failed to marshal execution: %w", err)
	}

	if err := s.etcd.Put(ctx, key, data, 0); err != nil {
		return fmt.Errorf("failed to store execution: %w", err)
	}

	return nil
}

// GetExecution retrieves an execution by ID.
func (s *EtcdStore) GetExecution(ctx context.Context, id string) (*ScheduleExecution, error) {
	if err := s.checkState(); err != nil {
		return nil, err
	}

	// Since we use composite keys, we need to search all executions
	data, err := s.etcd.List(ctx, executionKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list executions: %w", err)
	}

	for _, value := range data {
		var execution ScheduleExecution
		if err := json.Unmarshal(value, &execution); err != nil {
			continue
		}
		if execution.ID == id {
			return &execution, nil
		}
	}

	return nil, ErrExecutionNotFound
}

// UpdateExecution updates an existing execution.
func (s *EtcdStore) UpdateExecution(ctx context.Context, execution *ScheduleExecution) error {
	if err := s.checkState(); err != nil {
		return err
	}
	if execution == nil {
		return fmt.Errorf("execution is required")
	}
	if execution.ID == "" {
		return fmt.Errorf("execution ID is required")
	}

	key := executionKeyPrefix + execution.ScheduleID + "/" + execution.ID

	// Check if execution exists
	existing, err := s.etcd.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check existing execution: %w", err)
	}
	if existing == nil {
		return ErrExecutionNotFound
	}

	data, err := json.Marshal(execution)
	if err != nil {
		return fmt.Errorf("failed to marshal execution: %w", err)
	}

	if err := s.etcd.Put(ctx, key, data, 0); err != nil {
		return fmt.Errorf("failed to update execution: %w", err)
	}

	return nil
}

// ListExecutions lists executions matching the filter.
func (s *EtcdStore) ListExecutions(ctx context.Context, filter *ExecutionFilter) ([]*ScheduleExecution, error) {
	if err := s.checkState(); err != nil {
		return nil, err
	}

	// If schedule ID is specified, use it to narrow the search
	prefix := executionKeyPrefix
	if filter != nil && filter.ScheduleID != "" {
		prefix = executionKeyPrefix + filter.ScheduleID + "/"
	}

	data, err := s.etcd.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list executions: %w", err)
	}

	executions := make([]*ScheduleExecution, 0, len(data))
	for _, value := range data {
		var execution ScheduleExecution
		if err := json.Unmarshal(value, &execution); err != nil {
			continue // Skip invalid entries
		}

		// Apply filter
		if filter != nil && !s.matchesExecutionFilter(&execution, filter) {
			continue
		}

		executions = append(executions, &execution)
	}

	// Sort by scheduled time (descending - most recent first)
	sort.Slice(executions, func(i, j int) bool {
		return executions[i].ScheduledTime.After(executions[j].ScheduledTime)
	})

	// Apply pagination
	if filter != nil {
		if filter.Offset > 0 && filter.Offset < len(executions) {
			executions = executions[filter.Offset:]
		} else if filter.Offset >= len(executions) {
			return []*ScheduleExecution{}, nil
		}

		if filter.Limit > 0 && filter.Limit < len(executions) {
			executions = executions[:filter.Limit]
		}
	}

	return executions, nil
}

// matchesExecutionFilter checks if an execution matches the filter criteria.
func (s *EtcdStore) matchesExecutionFilter(execution *ScheduleExecution, filter *ExecutionFilter) bool {
	// Filter by schedule ID (already handled in prefix if set)
	if filter.ScheduleID != "" && execution.ScheduleID != filter.ScheduleID {
		return false
	}

	// Filter by status
	if len(filter.Status) > 0 {
		matched := false
		for _, status := range filter.Status {
			if execution.Status == status {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Filter by trigger type
	if len(filter.TriggerType) > 0 {
		matched := false
		for _, t := range filter.TriggerType {
			if execution.TriggerType == t {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Filter by time range
	if filter.StartAfter != nil {
		if execution.ScheduledTime.Before(*filter.StartAfter) {
			return false
		}
	}

	if filter.StartBefore != nil {
		if execution.ScheduledTime.After(*filter.StartBefore) {
			return false
		}
	}

	return true
}

// DeleteOldExecutions deletes executions older than the specified count per schedule.
func (s *EtcdStore) DeleteOldExecutions(ctx context.Context, scheduleID string, keepCount int) (int, error) {
	if err := s.checkState(); err != nil {
		return 0, err
	}

	prefix := executionKeyPrefix + scheduleID + "/"
	data, err := s.etcd.List(ctx, prefix)
	if err != nil {
		return 0, fmt.Errorf("failed to list executions: %w", err)
	}

	// Parse all executions
	type keyedExecution struct {
		key       string
		execution *ScheduleExecution
	}
	executions := make([]keyedExecution, 0, len(data))
	for key, value := range data {
		var execution ScheduleExecution
		if err := json.Unmarshal(value, &execution); err != nil {
			continue
		}
		executions = append(executions, keyedExecution{key: key, execution: &execution})
	}

	// Sort by scheduled time (most recent first)
	sort.Slice(executions, func(i, j int) bool {
		return executions[i].execution.ScheduledTime.After(executions[j].execution.ScheduledTime)
	})

	// Delete executions beyond keepCount
	deleted := 0
	if len(executions) > keepCount {
		for _, ke := range executions[keepCount:] {
			if err := s.etcd.Delete(ctx, ke.key); err != nil {
				continue
			}
			deleted++
		}
	}

	return deleted, nil
}

// Maintenance window operations

// CreateMaintenanceWindow creates a new maintenance window.
func (s *EtcdStore) CreateMaintenanceWindow(ctx context.Context, window *MaintenanceWindow) error {
	if err := s.checkState(); err != nil {
		return err
	}
	if window == nil {
		return fmt.Errorf("maintenance window is required")
	}
	if window.ID == "" {
		return fmt.Errorf("maintenance window ID is required")
	}

	key := maintenanceWindowKeyPrefix + window.ID

	// Check if window already exists
	existing, err := s.etcd.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check existing maintenance window: %w", err)
	}
	if existing != nil {
		return ErrMaintenanceWindowExists
	}

	data, err := json.Marshal(window)
	if err != nil {
		return fmt.Errorf("failed to marshal maintenance window: %w", err)
	}

	if err := s.etcd.Put(ctx, key, data, 0); err != nil {
		return fmt.Errorf("failed to store maintenance window: %w", err)
	}

	return nil
}

// GetMaintenanceWindow retrieves a maintenance window by ID.
func (s *EtcdStore) GetMaintenanceWindow(ctx context.Context, id string) (*MaintenanceWindow, error) {
	if err := s.checkState(); err != nil {
		return nil, err
	}

	data, err := s.etcd.Get(ctx, maintenanceWindowKeyPrefix+id)
	if err != nil {
		return nil, fmt.Errorf("failed to get maintenance window: %w", err)
	}
	if data == nil {
		return nil, ErrMaintenanceWindowNotFound
	}

	var window MaintenanceWindow
	if err := json.Unmarshal(data, &window); err != nil {
		return nil, fmt.Errorf("failed to unmarshal maintenance window: %w", err)
	}

	return &window, nil
}

// UpdateMaintenanceWindow updates an existing maintenance window.
func (s *EtcdStore) UpdateMaintenanceWindow(ctx context.Context, window *MaintenanceWindow) error {
	if err := s.checkState(); err != nil {
		return err
	}
	if window == nil {
		return fmt.Errorf("maintenance window is required")
	}
	if window.ID == "" {
		return fmt.Errorf("maintenance window ID is required")
	}

	key := maintenanceWindowKeyPrefix + window.ID

	// Check if window exists
	existing, err := s.etcd.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check existing maintenance window: %w", err)
	}
	if existing == nil {
		return ErrMaintenanceWindowNotFound
	}

	data, err := json.Marshal(window)
	if err != nil {
		return fmt.Errorf("failed to marshal maintenance window: %w", err)
	}

	if err := s.etcd.Put(ctx, key, data, 0); err != nil {
		return fmt.Errorf("failed to update maintenance window: %w", err)
	}

	return nil
}

// DeleteMaintenanceWindow deletes a maintenance window by ID.
func (s *EtcdStore) DeleteMaintenanceWindow(ctx context.Context, id string) error {
	if err := s.checkState(); err != nil {
		return err
	}

	key := maintenanceWindowKeyPrefix + id

	// Check if window exists
	existing, err := s.etcd.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check existing maintenance window: %w", err)
	}
	if existing == nil {
		return ErrMaintenanceWindowNotFound
	}

	if err := s.etcd.Delete(ctx, key); err != nil {
		return fmt.Errorf("failed to delete maintenance window: %w", err)
	}

	return nil
}

// ListMaintenanceWindows lists maintenance windows matching the filter.
func (s *EtcdStore) ListMaintenanceWindows(ctx context.Context, filter *MaintenanceWindowFilter) ([]*MaintenanceWindow, error) {
	if err := s.checkState(); err != nil {
		return nil, err
	}

	data, err := s.etcd.List(ctx, maintenanceWindowKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list maintenance windows: %w", err)
	}

	windows := make([]*MaintenanceWindow, 0, len(data))
	for _, value := range data {
		var window MaintenanceWindow
		if err := json.Unmarshal(value, &window); err != nil {
			continue // Skip invalid entries
		}

		// Apply filter
		if filter != nil && !s.matchesMaintenanceWindowFilter(&window, filter) {
			continue
		}

		windows = append(windows, &window)
	}

	// Sort by start time (ascending - earliest first)
	sort.Slice(windows, func(i, j int) bool {
		return windows[i].StartTime.Before(windows[j].StartTime)
	})

	// Apply pagination
	if filter != nil {
		if filter.Offset > 0 && filter.Offset < len(windows) {
			windows = windows[filter.Offset:]
		} else if filter.Offset >= len(windows) {
			return []*MaintenanceWindow{}, nil
		}

		if filter.Limit > 0 && filter.Limit < len(windows) {
			windows = windows[:filter.Limit]
		}
	}

	return windows, nil
}

// matchesMaintenanceWindowFilter checks if a window matches the filter criteria.
func (s *EtcdStore) matchesMaintenanceWindowFilter(window *MaintenanceWindow, filter *MaintenanceWindowFilter) bool {
	// Filter by status
	if len(filter.Status) > 0 {
		matched := false
		for _, status := range filter.Status {
			if window.Status == status {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Filter by type
	if len(filter.Type) > 0 {
		matched := false
		for _, t := range filter.Type {
			if window.Type == t {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Filter by time range
	if filter.StartAfter != nil {
		if window.StartTime.Before(*filter.StartAfter) {
			return false
		}
	}

	if filter.EndBefore != nil {
		if window.EndTime.After(*filter.EndBefore) {
			return false
		}
	}

	// Filter by time inclusion
	if filter.IncludesTime != nil {
		if !window.ContainsTime(*filter.IncludesTime) {
			return false
		}
	}

	// Filter by agent ID
	if filter.AgentID != "" && window.Scope != nil {
		matched := false
		// Check if agent is in scope
		if window.Scope.All {
			matched = true
		} else {
			for _, agentID := range window.Scope.AgentIDs {
				if agentID == filter.AgentID {
					matched = true
					break
				}
			}
		}
		if !matched {
			return false
		}
	}

	// Filter by name substring
	if filter.NameContains != "" {
		if !strings.Contains(strings.ToLower(window.Name), strings.ToLower(filter.NameContains)) {
			return false
		}
	}

	// Filter by labels
	if len(filter.Labels) > 0 {
		for k, v := range filter.Labels {
			if window.Labels == nil || window.Labels[k] != v {
				return false
			}
		}
	}

	return true
}

// WatchMaintenanceWindows watches for maintenance window changes.
func (s *EtcdStore) WatchMaintenanceWindows(ctx context.Context, handler MaintenanceWindowWatchHandler) error {
	if err := s.checkState(); err != nil {
		return err
	}

	return s.etcd.Watch(ctx, maintenanceWindowKeyPrefix, func(key string, value []byte, deleted bool) {
		event := &MaintenanceWindowWatchEvent{
			WindowID: extractID(key, maintenanceWindowKeyPrefix),
		}

		if deleted {
			event.Type = WatchEventDeleted
		} else {
			var window MaintenanceWindow
			if err := json.Unmarshal(value, &window); err != nil {
				return // Skip invalid data
			}
			event.Window = &window
			event.Type = WatchEventUpdated
		}

		handler(event)
	})
}

// Lock operations

// AcquireLock attempts to acquire a distributed lock.
func (s *EtcdStore) AcquireLock(ctx context.Context, lockID string, holderID string) (bool, error) {
	if err := s.checkState(); err != nil {
		return false, err
	}

	key := scheduleLockKeyPrefix + lockID

	// Create lock info
	lockInfo := LockInfo{
		LockID:     lockID,
		HolderID:   holderID,
		AcquiredAt: time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(lockInfo)
	if err != nil {
		return false, fmt.Errorf("failed to marshal lock info: %w", err)
	}

	// Use compare-and-swap to atomically acquire lock
	acquired, err := s.etcd.CompareAndSwap(ctx, key, nil, data)
	if err != nil {
		return false, fmt.Errorf("failed to acquire lock: %w", err)
	}

	return acquired, nil
}

// ReleaseLock releases a distributed lock.
func (s *EtcdStore) ReleaseLock(ctx context.Context, lockID string, holderID string) error {
	if err := s.checkState(); err != nil {
		return err
	}

	key := scheduleLockKeyPrefix + lockID

	// Get current lock to verify holder
	data, err := s.etcd.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to get lock: %w", err)
	}
	if data == nil {
		return nil // Lock doesn't exist, nothing to release
	}

	var lockInfo LockInfo
	if err := json.Unmarshal(data, &lockInfo); err != nil {
		return fmt.Errorf("failed to unmarshal lock info: %w", err)
	}

	// Verify holder
	if lockInfo.HolderID != holderID {
		return fmt.Errorf("lock held by different holder: %s", lockInfo.HolderID)
	}

	// Delete the lock
	if err := s.etcd.Delete(ctx, key); err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	return nil
}

// IsLocked checks if a lock is currently held.
func (s *EtcdStore) IsLocked(ctx context.Context, lockID string) (bool, string, error) {
	if err := s.checkState(); err != nil {
		return false, "", err
	}

	key := scheduleLockKeyPrefix + lockID

	data, err := s.etcd.Get(ctx, key)
	if err != nil {
		return false, "", fmt.Errorf("failed to get lock: %w", err)
	}
	if data == nil {
		return false, "", nil
	}

	var lockInfo LockInfo
	if err := json.Unmarshal(data, &lockInfo); err != nil {
		return false, "", fmt.Errorf("failed to unmarshal lock info: %w", err)
	}

	return true, lockInfo.HolderID, nil
}

// Close closes the store.
func (s *EtcdStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	return nil
}

// extractID extracts the ID from an etcd key.
func extractID(key, prefix string) string {
	// Remove the prefix and return the remaining path
	if strings.HasPrefix(key, prefix) {
		return path.Base(strings.TrimPrefix(key, prefix))
	}
	return path.Base(key)
}
