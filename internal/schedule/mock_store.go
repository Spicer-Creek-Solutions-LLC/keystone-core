package schedule

import (
	"context"
	"sync"
	"time"
)

// MockStore is an in-memory implementation of Store for testing.
type MockStore struct {
	schedules          map[string]*Schedule
	executions         map[string]*ScheduleExecution
	maintenanceWindows map[string]*MaintenanceWindow
	locks              map[string]*LockInfo
	mu                 sync.RWMutex
	closed             bool
}

// NewMockStore creates a new mock store.
func NewMockStore() *MockStore {
	return &MockStore{
		schedules:          make(map[string]*Schedule),
		executions:         make(map[string]*ScheduleExecution),
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
	copy := *schedule
	s.schedules[schedule.ID] = &copy
	return nil
}

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
	copy := *schedule
	return &copy, nil
}

func (s *MockStore) UpdateSchedule(ctx context.Context, schedule *Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkState(); err != nil {
		return err
	}

	if _, exists := s.schedules[schedule.ID]; !exists {
		return ErrScheduleNotFound
	}

	copy := *schedule
	s.schedules[schedule.ID] = &copy
	return nil
}

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

func (s *MockStore) ListSchedules(ctx context.Context, filter *ScheduleFilter) ([]*Schedule, error) {
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
		copy := *schedule
		result = append(result, &copy)
	}

	return result, nil
}

func (s *MockStore) matchesScheduleFilter(schedule *Schedule, filter *ScheduleFilter) bool {
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

func (s *MockStore) WatchSchedules(ctx context.Context, handler ScheduleWatchHandler) error {
	// Mock implementation - just return nil
	return nil
}

// Execution operations

func (s *MockStore) CreateExecution(ctx context.Context, execution *ScheduleExecution) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkState(); err != nil {
		return err
	}

	copy := *execution
	s.executions[execution.ID] = &copy
	return nil
}

func (s *MockStore) GetExecution(ctx context.Context, id string) (*ScheduleExecution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := s.checkState(); err != nil {
		return nil, err
	}

	execution, exists := s.executions[id]
	if !exists {
		return nil, ErrExecutionNotFound
	}

	copy := *execution
	return &copy, nil
}

func (s *MockStore) UpdateExecution(ctx context.Context, execution *ScheduleExecution) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkState(); err != nil {
		return err
	}

	if _, exists := s.executions[execution.ID]; !exists {
		return ErrExecutionNotFound
	}

	copy := *execution
	s.executions[execution.ID] = &copy
	return nil
}

func (s *MockStore) ListExecutions(ctx context.Context, filter *ExecutionFilter) ([]*ScheduleExecution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := s.checkState(); err != nil {
		return nil, err
	}

	result := make([]*ScheduleExecution, 0, len(s.executions))
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
		copy := *execution
		result = append(result, &copy)
	}

	return result, nil
}

func (s *MockStore) DeleteOldExecutions(ctx context.Context, scheduleID string, keepCount int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkState(); err != nil {
		return 0, err
	}

	// Count executions for this schedule
	var executions []*ScheduleExecution
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

func (s *MockStore) CreateMaintenanceWindow(ctx context.Context, window *MaintenanceWindow) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkState(); err != nil {
		return err
	}

	if _, exists := s.maintenanceWindows[window.ID]; exists {
		return ErrMaintenanceWindowExists
	}

	copy := *window
	s.maintenanceWindows[window.ID] = &copy
	return nil
}

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

	copy := *window
	return &copy, nil
}

func (s *MockStore) UpdateMaintenanceWindow(ctx context.Context, window *MaintenanceWindow) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkState(); err != nil {
		return err
	}

	if _, exists := s.maintenanceWindows[window.ID]; !exists {
		return ErrMaintenanceWindowNotFound
	}

	copy := *window
	s.maintenanceWindows[window.ID] = &copy
	return nil
}

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
		copy := *window
		result = append(result, &copy)
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

func (s *MockStore) WatchMaintenanceWindows(ctx context.Context, handler MaintenanceWindowWatchHandler) error {
	return nil
}

// Lock operations

func (s *MockStore) AcquireLock(ctx context.Context, lockID string, holderID string) (bool, error) {
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

func (s *MockStore) ReleaseLock(ctx context.Context, lockID string, holderID string) error {
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

func (s *MockStore) IsLocked(ctx context.Context, lockID string) (bool, string, error) {
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
	s.executions = make(map[string]*ScheduleExecution)
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
