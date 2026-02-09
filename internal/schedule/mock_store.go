package schedule

import (
	"context"
	"sync"
	"time"
)

// MockStore is an in-memory implementation of Store for testing.
type MockStore struct {
	schedules          map[string]*Schedule
	executions         map[string]*Execution
	maintenanceWindows map[string]*MaintenanceWindow
	locks              map[string]*LockInfo
	mu                 sync.RWMutex
	closed             bool
}

// NewMockStore creates a new mock store.
func NewMockStore() *MockStore {
	return &MockStore{
		schedules:          make(map[string]*Schedule),
		executions:         make(map[string]*Execution),
		maintenanceWindows: make(map[string]*MaintenanceWindow),
		locks:              make(map[string]*LockInfo),
	}
}

func (s *MockStore) checkState() error {
	if s.closed {
		return ErrStoreClosed
	}
	return nil
}

// Schedule operations

// CreateSchedule creates a new schedule.
func (s *MockStore) CreateSchedule(ctx context.Context, schedule *Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkState(); err != nil {
		return err
	}

	if _, exists := s.schedules[schedule.ID]; exists {
		return ErrScheduleExists
	}

	// Deep copy
	copied := *schedule
	s.schedules[schedule.ID] = &copied
	return nil
}

// GetSchedule retrieves a schedule by ID.
func (s *MockStore) GetSchedule(ctx context.Context, id string) (*Schedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := s.checkState(); err != nil {
		return nil, err
	}

	schedule, exists := s.schedules[id]
	if !exists {
		return nil, ErrScheduleNotFound
	}

	// Return a copy
	copied := *schedule
	return &copied, nil
}

// UpdateSchedule updates an existing schedule.
func (s *MockStore) UpdateSchedule(ctx context.Context, schedule *Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkState(); err != nil {
		return err
	}

	if _, exists := s.schedules[schedule.ID]; !exists {
		return ErrScheduleNotFound
	}

	copied := *schedule
	s.schedules[schedule.ID] = &copied
	return nil
}

// DeleteSchedule deletes a schedule.
func (s *MockStore) DeleteSchedule(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkState(); err != nil {
		return err
	}

	if _, exists := s.schedules[id]; !exists {
		return ErrScheduleNotFound
	}

	delete(s.schedules, id)
	return nil
}

// ListSchedules lists schedules matching the given filter.
func (s *MockStore) ListSchedules(ctx context.Context, filter *Filter) ([]*Schedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := s.checkState(); err != nil {
		return nil, err
	}

	result := make([]*Schedule, 0, len(s.schedules))
	for _, schedule := range s.schedules {
		if filter != nil && !s.matchesScheduleFilter(schedule, filter) {
			continue
		}
		copied := *schedule
		result = append(result, &copied)
	}

	return result, nil
}

func (s *MockStore) matchesScheduleFilter(schedule *Schedule, filter *Filter) bool {
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

	return true
}

// WatchSchedules watches for schedule changes.
func (s *MockStore) WatchSchedules(ctx context.Context, handler WatchHandler) error {
	// Mock implementation - just return nil
	return nil
}

// Execution operations

// CreateExecution creates a new execution record.
func (s *MockStore) CreateExecution(ctx context.Context, execution *Execution) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkState(); err != nil {
		return err
	}

	copied := *execution
	s.executions[execution.ID] = &copied
	return nil
}

// GetExecution retrieves an execution by ID.
func (s *MockStore) GetExecution(ctx context.Context, id string) (*Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := s.checkState(); err != nil {
		return nil, err
	}

	execution, exists := s.executions[id]
	if !exists {
		return nil, ErrExecutionNotFound
	}

	copied := *execution
	return &copied, nil
}

// UpdateExecution updates an execution record.
func (s *MockStore) UpdateExecution(ctx context.Context, execution *Execution) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkState(); err != nil {
		return err
	}

	if _, exists := s.executions[execution.ID]; !exists {
		return ErrExecutionNotFound
	}

	copied := *execution
	s.executions[execution.ID] = &copied
	return nil
}

// ListExecutions lists executions matching the given filter.
func (s *MockStore) ListExecutions(ctx context.Context, filter *ExecutionFilter) ([]*Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := s.checkState(); err != nil {
		return nil, err
	}

	result := make([]*Execution, 0, len(s.executions))
	for _, execution := range s.executions {
		if filter != nil {
			if filter.ScheduleID != "" && execution.ScheduleID != filter.ScheduleID {
				continue
			}
			if len(filter.Status) > 0 {
				matched := false
				for _, status := range filter.Status {
					if execution.Status == status {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
		}
		copied := *execution
		result = append(result, &copied)
	}

	return result, nil
}

// DeleteOldExecutions deletes executions older than the retention period.
func (s *MockStore) DeleteOldExecutions(ctx context.Context, scheduleID string, keepCount int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkState(); err != nil {
		return 0, err
	}

	// Count executions for this schedule
	var executions []*Execution
	for _, e := range s.executions {
		if e.ScheduleID == scheduleID {
			executions = append(executions, e)
		}
	}

	if len(executions) <= keepCount {
		return 0, nil
	}

	// Sort by time (oldest first) and delete extras
	deleted := 0
	toDelete := len(executions) - keepCount
	for _, e := range executions {
		if deleted >= toDelete {
			break
		}
		delete(s.executions, e.ID)
		deleted++
	}

	return deleted, nil
}

// Maintenance window operations

// CreateMaintenanceWindow creates a new maintenance window.
func (s *MockStore) CreateMaintenanceWindow(ctx context.Context, window *MaintenanceWindow) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkState(); err != nil {
		return err
	}

	if _, exists := s.maintenanceWindows[window.ID]; exists {
		return ErrMaintenanceWindowExists
	}

	copied := *window
	s.maintenanceWindows[window.ID] = &copied
	return nil
}

// GetMaintenanceWindow retrieves a maintenance window by ID.
func (s *MockStore) GetMaintenanceWindow(ctx context.Context, id string) (*MaintenanceWindow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := s.checkState(); err != nil {
		return nil, err
	}

	window, exists := s.maintenanceWindows[id]
	if !exists {
		return nil, ErrMaintenanceWindowNotFound
	}

	copied := *window
	return &copied, nil
}

// UpdateMaintenanceWindow updates a maintenance window.
func (s *MockStore) UpdateMaintenanceWindow(ctx context.Context, window *MaintenanceWindow) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkState(); err != nil {
		return err
	}

	if _, exists := s.maintenanceWindows[window.ID]; !exists {
		return ErrMaintenanceWindowNotFound
	}

	copied := *window
	s.maintenanceWindows[window.ID] = &copied
	return nil
}

// DeleteMaintenanceWindow deletes a maintenance window.
func (s *MockStore) DeleteMaintenanceWindow(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkState(); err != nil {
		return err
	}

	if _, exists := s.maintenanceWindows[id]; !exists {
		return ErrMaintenanceWindowNotFound
	}

	delete(s.maintenanceWindows, id)
	return nil
}

// ListMaintenanceWindows lists maintenance windows matching the given filter.
func (s *MockStore) ListMaintenanceWindows(ctx context.Context, filter *MaintenanceWindowFilter) ([]*MaintenanceWindow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := s.checkState(); err != nil {
		return nil, err
	}

	result := make([]*MaintenanceWindow, 0, len(s.maintenanceWindows))
	for _, window := range s.maintenanceWindows {
		if filter != nil && !s.matchesWindowFilter(window, filter) {
			continue
		}
		copied := *window
		result = append(result, &copied)
	}

	return result, nil
}

func (s *MockStore) matchesWindowFilter(window *MaintenanceWindow, filter *MaintenanceWindowFilter) bool {
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

	if filter.IncludesTime != nil {
		if !window.ContainsTime(*filter.IncludesTime) {
			return false
		}
	}

	return true
}

// WatchMaintenanceWindows watches for maintenance window changes.
func (s *MockStore) WatchMaintenanceWindows(ctx context.Context, handler MaintenanceWindowWatchHandler) error {
	return nil
}

// Lock operations

// AcquireLock acquires a distributed lock.
func (s *MockStore) AcquireLock(ctx context.Context, lockID, holderID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkState(); err != nil {
		return false, err
	}

	if _, exists := s.locks[lockID]; exists {
		return false, nil
	}

	s.locks[lockID] = &LockInfo{
		LockID:     lockID,
		HolderID:   holderID,
		AcquiredAt: time.Now().UTC().Format(time.RFC3339),
	}
	return true, nil
}

// ReleaseLock releases a distributed lock.
func (s *MockStore) ReleaseLock(ctx context.Context, lockID, holderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkState(); err != nil {
		return err
	}

	lock, exists := s.locks[lockID]
	if !exists {
		return nil
	}

	if lock.HolderID != holderID {
		return ErrLockAcquisitionFailed
	}

	delete(s.locks, lockID)
	return nil
}

// IsLocked checks if a lock is held.
func (s *MockStore) IsLocked(ctx context.Context, lockID string) (locked bool, holder string, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := s.checkState(); err != nil {
		return false, "", err
	}

	lock, exists := s.locks[lockID]
	if !exists {
		return false, "", nil
	}

	return true, lock.HolderID, nil
}

// Close closes the resource and releases any associated resources.
func (s *MockStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	return nil
}

// Helper methods for testing

// Reset clears all data in the mock store.
func (s *MockStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.schedules = make(map[string]*Schedule)
	s.executions = make(map[string]*Execution)
	s.maintenanceWindows = make(map[string]*MaintenanceWindow)
	s.locks = make(map[string]*LockInfo)
	s.closed = false
}

// GetScheduleCount returns the number of schedules.
func (s *MockStore) GetScheduleCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.schedules)
}

// GetExecutionCount returns the number of executions.
func (s *MockStore) GetExecutionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.executions)
}

// GetMaintenanceWindowCount returns the number of maintenance windows.
func (s *MockStore) GetMaintenanceWindowCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.maintenanceWindows)
}
