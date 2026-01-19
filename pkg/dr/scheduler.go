// Package dr provides disaster recovery drill scheduling and execution.
package dr

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Errors returned by the DR scheduler.
var (
	ErrDrillInProgress   = errors.New("drill already in progress")
	ErrDrillNotFound     = errors.New("drill not found")
	ErrSchedulerStopped  = errors.New("scheduler stopped")
	ErrInvalidSchedule   = errors.New("invalid schedule")
)

// DrillType represents the type of disaster recovery drill.
type DrillType string

const (
	// DrillTypeFailover tests failover to a secondary site/region.
	DrillTypeFailover DrillType = "failover"
	// DrillTypeBackupRestore tests backup and restore procedures.
	DrillTypeBackupRestore DrillType = "backup_restore"
	// DrillTypeDataValidation tests data integrity after recovery.
	DrillTypeDataValidation DrillType = "data_validation"
	// DrillTypeNetworkPartition tests behavior during network splits.
	DrillTypeNetworkPartition DrillType = "network_partition"
	// DrillTypeServiceDegradation tests graceful degradation.
	DrillTypeServiceDegradation DrillType = "service_degradation"
	// DrillTypeFullRecovery tests complete recovery from scratch.
	DrillTypeFullRecovery DrillType = "full_recovery"
)

// DrillStatus represents the status of a drill.
type DrillStatus string

const (
	StatusScheduled  DrillStatus = "scheduled"
	StatusRunning    DrillStatus = "running"
	StatusCompleted  DrillStatus = "completed"
	StatusFailed     DrillStatus = "failed"
	StatusCancelled  DrillStatus = "cancelled"
	StatusSkipped    DrillStatus = "skipped"
)

// Schedule defines when drills should run.
type Schedule struct {
	// Type is the drill type.
	Type DrillType
	// Cron is the cron expression for scheduling.
	Cron string
	// Frequency is the frequency if cron is not used.
	Frequency time.Duration
	// Window is the time window for execution.
	Window *TimeWindow
	// Priority is the drill priority (higher = more important).
	Priority int
	// MaxDuration is the maximum drill duration.
	MaxDuration time.Duration
	// Enabled controls whether the schedule is active.
	Enabled bool
	// RequireApproval requires manual approval before execution.
	RequireApproval bool
	// NotifyBefore is how long before to notify stakeholders.
	NotifyBefore time.Duration
}

// TimeWindow defines when drills can run.
type TimeWindow struct {
	// DaysOfWeek (0=Sunday, 6=Saturday).
	DaysOfWeek []int
	// StartHour is the start of the window (0-23).
	StartHour int
	// EndHour is the end of the window (0-23).
	EndHour int
	// Timezone is the timezone for the window.
	Timezone string
}

// DrillConfig holds configuration for a drill run.
type DrillConfig struct {
	// ID is the unique drill identifier.
	ID string
	// Type is the drill type.
	Type DrillType
	// ScheduleID links to the schedule that triggered this drill.
	ScheduleID string
	// Targets are the systems/services to test.
	Targets []string
	// Parameters are drill-specific parameters.
	Parameters map[string]interface{}
	// Timeout is the maximum drill duration.
	Timeout time.Duration
	// DryRun performs the drill without making changes.
	DryRun bool
	// Callbacks for various drill stages.
	OnStart    func(drill *Drill)
	OnComplete func(drill *Drill)
	OnFail     func(drill *Drill, err error)
}

// Drill represents a DR drill instance.
type Drill struct {
	ID         string
	Type       DrillType
	ScheduleID string
	Status     DrillStatus
	StartTime  time.Time
	EndTime    time.Time
	Duration   time.Duration
	Targets    []string
	Results    *DrillResults
	Error      error
	mu         sync.RWMutex
}

// DrillResults holds the results of a drill.
type DrillResults struct {
	// Success indicates overall success.
	Success bool
	// Steps are the individual drill steps.
	Steps []*DrillStep
	// Metrics are collected metrics.
	Metrics map[string]float64
	// Issues are problems found during the drill.
	Issues []*Issue
	// RecoveryTime is the measured recovery time.
	RecoveryTime time.Duration
	// DataLoss indicates any data loss detected.
	DataLoss bool
	// DataLossAmount is the amount of data lost.
	DataLossAmount int64
}

// DrillStep represents a step in the drill.
type DrillStep struct {
	Name      string
	Status    DrillStatus
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
	Error     string
	Details   map[string]interface{}
}

// Issue represents a problem found during the drill.
type Issue struct {
	Severity    string
	Component   string
	Description string
	Timestamp   time.Time
}

// DrillEvent represents a drill-related event.
type DrillEvent struct {
	Type      string
	DrillID   string
	DrillType DrillType
	Status    DrillStatus
	Timestamp time.Time
	Message   string
	Error     error
}

// Listener receives drill events.
type Listener func(*DrillEvent)

// Executor executes a specific drill type.
type Executor interface {
	// Execute runs the drill.
	Execute(ctx context.Context, config *DrillConfig) (*DrillResults, error)
	// Validate checks if the drill can be executed.
	Validate(config *DrillConfig) error
	// Cleanup cleans up after a drill.
	Cleanup(ctx context.Context, drill *Drill) error
}

// SchedulerConfig holds scheduler configuration.
type SchedulerConfig struct {
	// CheckInterval is how often to check for due drills.
	CheckInterval time.Duration
	// MaxConcurrent is the maximum concurrent drills.
	MaxConcurrent int
	// RetryAttempts is the number of retry attempts for failed drills.
	RetryAttempts int
	// RetryDelay is the delay between retries.
	RetryDelay time.Duration
	// DefaultTimeout is the default drill timeout.
	DefaultTimeout time.Duration
}

// DefaultSchedulerConfig returns a default scheduler configuration.
func DefaultSchedulerConfig() *SchedulerConfig {
	return &SchedulerConfig{
		CheckInterval:  time.Minute,
		MaxConcurrent:  1,
		RetryAttempts:  2,
		RetryDelay:     time.Minute * 5,
		DefaultTimeout: time.Hour,
	}
}

// Scheduler manages DR drill scheduling and execution.
type Scheduler struct {
	config     *SchedulerConfig
	schedules  map[string]*Schedule
	drills     map[string]*Drill
	executors  map[DrillType]Executor
	listeners  []Listener
	running    map[string]bool
	lastRun    map[string]time.Time
	stopCh     chan struct{}
	wg         sync.WaitGroup
	mu         sync.RWMutex
}

// NewScheduler creates a new DR scheduler.
func NewScheduler(config *SchedulerConfig) *Scheduler {
	if config == nil {
		config = DefaultSchedulerConfig()
	}

	return &Scheduler{
		config:    config,
		schedules: make(map[string]*Schedule),
		drills:    make(map[string]*Drill),
		executors: make(map[DrillType]Executor),
		running:   make(map[string]bool),
		lastRun:   make(map[string]time.Time),
		stopCh:    make(chan struct{}),
	}
}

// RegisterExecutor registers an executor for a drill type.
func (s *Scheduler) RegisterExecutor(drillType DrillType, executor Executor) {
	s.mu.Lock()
	s.executors[drillType] = executor
	s.mu.Unlock()
}

// AddSchedule adds a drill schedule.
func (s *Scheduler) AddSchedule(id string, schedule *Schedule) error {
	if schedule.Frequency <= 0 && schedule.Cron == "" {
		return ErrInvalidSchedule
	}

	s.mu.Lock()
	s.schedules[id] = schedule
	s.mu.Unlock()

	s.emitEvent(&DrillEvent{
		Type:      "schedule_added",
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("schedule %s added for %s drills", id, schedule.Type),
	})

	return nil
}

// RemoveSchedule removes a drill schedule.
func (s *Scheduler) RemoveSchedule(id string) {
	s.mu.Lock()
	delete(s.schedules, id)
	s.mu.Unlock()
}

// EnableSchedule enables or disables a schedule.
func (s *Scheduler) EnableSchedule(id string, enabled bool) {
	s.mu.Lock()
	if schedule, exists := s.schedules[id]; exists {
		schedule.Enabled = enabled
	}
	s.mu.Unlock()
}

// Start starts the scheduler.
func (s *Scheduler) Start() {
	s.wg.Add(1)
	go s.runLoop()
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *Scheduler) runLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkSchedules()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Scheduler) checkSchedules() {
	s.mu.RLock()
	schedules := make(map[string]*Schedule, len(s.schedules))
	for id, schedule := range s.schedules {
		schedules[id] = schedule
	}
	s.mu.RUnlock()

	now := time.Now()

	for id, schedule := range schedules {
		if !schedule.Enabled {
			continue
		}

		if !s.isInWindow(schedule.Window, now) {
			continue
		}

		if s.isDue(id, schedule, now) {
			s.triggerDrill(id, schedule)
		}
	}
}

func (s *Scheduler) isInWindow(window *TimeWindow, t time.Time) bool {
	if window == nil {
		return true
	}

	// Check timezone
	loc := time.UTC
	if window.Timezone != "" {
		if l, err := time.LoadLocation(window.Timezone); err == nil {
			loc = l
		}
	}
	t = t.In(loc)

	// Check day of week
	if len(window.DaysOfWeek) > 0 {
		dayOk := false
		weekday := int(t.Weekday())
		for _, d := range window.DaysOfWeek {
			if d == weekday {
				dayOk = true
				break
			}
		}
		if !dayOk {
			return false
		}
	}

	// Check hour
	hour := t.Hour()
	if window.StartHour <= window.EndHour {
		return hour >= window.StartHour && hour < window.EndHour
	}
	// Handle overnight windows (e.g., 22:00 - 06:00)
	return hour >= window.StartHour || hour < window.EndHour
}

func (s *Scheduler) isDue(id string, schedule *Schedule, now time.Time) bool {
	s.mu.RLock()
	lastRun, hasRun := s.lastRun[id]
	s.mu.RUnlock()

	if !hasRun {
		return true
	}

	return now.Sub(lastRun) >= schedule.Frequency
}

func (s *Scheduler) triggerDrill(scheduleID string, schedule *Schedule) {
	s.mu.Lock()
	// Check concurrent limit
	runningCount := 0
	for _, running := range s.running {
		if running {
			runningCount++
		}
	}
	if runningCount >= s.config.MaxConcurrent {
		s.mu.Unlock()
		return
	}

	// Mark as running
	s.running[scheduleID] = true
	s.lastRun[scheduleID] = time.Now()
	s.mu.Unlock()

	// Execute in background
	go func() {
		defer func() {
			s.mu.Lock()
			s.running[scheduleID] = false
			s.mu.Unlock()
		}()

		config := &DrillConfig{
			ID:         fmt.Sprintf("%s-%d", scheduleID, time.Now().UnixNano()),
			Type:       schedule.Type,
			ScheduleID: scheduleID,
			Timeout:    schedule.MaxDuration,
		}

		if config.Timeout <= 0 {
			config.Timeout = s.config.DefaultTimeout
		}

		s.ExecuteDrill(context.Background(), config)
	}()
}

// ExecuteDrill executes a drill immediately.
func (s *Scheduler) ExecuteDrill(ctx context.Context, config *DrillConfig) (*Drill, error) {
	s.mu.RLock()
	executor, exists := s.executors[config.Type]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no executor for drill type %s", config.Type)
	}

	// Validate
	if err := executor.Validate(config); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Create drill
	drill := &Drill{
		ID:         config.ID,
		Type:       config.Type,
		ScheduleID: config.ScheduleID,
		Status:     StatusRunning,
		StartTime:  time.Now(),
		Targets:    config.Targets,
	}

	s.mu.Lock()
	s.drills[drill.ID] = drill
	s.mu.Unlock()

	s.emitEvent(&DrillEvent{
		Type:      "drill_started",
		DrillID:   drill.ID,
		DrillType: drill.Type,
		Status:    StatusRunning,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("drill %s started", drill.ID),
	})

	if config.OnStart != nil {
		config.OnStart(drill)
	}

	// Apply timeout
	drillCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	// Execute
	results, err := executor.Execute(drillCtx, config)

	drill.mu.Lock()
	drill.EndTime = time.Now()
	drill.Duration = drill.EndTime.Sub(drill.StartTime)
	drill.Results = results

	if err != nil {
		drill.Status = StatusFailed
		drill.Error = err
		drill.mu.Unlock()

		s.emitEvent(&DrillEvent{
			Type:      "drill_failed",
			DrillID:   drill.ID,
			DrillType: drill.Type,
			Status:    StatusFailed,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("drill %s failed: %v", drill.ID, err),
			Error:     err,
		})

		if config.OnFail != nil {
			config.OnFail(drill, err)
		}

		// Cleanup
		executor.Cleanup(ctx, drill)

		return drill, err
	}

	drill.Status = StatusCompleted
	drill.mu.Unlock()

	s.emitEvent(&DrillEvent{
		Type:      "drill_completed",
		DrillID:   drill.ID,
		DrillType: drill.Type,
		Status:    StatusCompleted,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("drill %s completed successfully", drill.ID),
	})

	if config.OnComplete != nil {
		config.OnComplete(drill)
	}

	// Cleanup
	executor.Cleanup(ctx, drill)

	return drill, nil
}

// CancelDrill cancels a running drill.
func (s *Scheduler) CancelDrill(id string) error {
	s.mu.Lock()
	drill, exists := s.drills[id]
	if !exists {
		s.mu.Unlock()
		return ErrDrillNotFound
	}
	s.mu.Unlock()

	drill.mu.Lock()
	if drill.Status != StatusRunning {
		drill.mu.Unlock()
		return nil
	}
	drill.Status = StatusCancelled
	drill.EndTime = time.Now()
	drill.Duration = drill.EndTime.Sub(drill.StartTime)
	drill.mu.Unlock()

	s.emitEvent(&DrillEvent{
		Type:      "drill_cancelled",
		DrillID:   drill.ID,
		DrillType: drill.Type,
		Status:    StatusCancelled,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("drill %s cancelled", drill.ID),
	})

	return nil
}

// GetDrill returns a drill by ID.
func (s *Scheduler) GetDrill(id string) *Drill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.drills[id]
}

// ListDrills returns all drills.
func (s *Scheduler) ListDrills() []*Drill {
	s.mu.RLock()
	defer s.mu.RUnlock()

	drills := make([]*Drill, 0, len(s.drills))
	for _, d := range s.drills {
		drills = append(drills, d)
	}
	return drills
}

// ListSchedules returns all schedules.
func (s *Scheduler) ListSchedules() map[string]*Schedule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*Schedule, len(s.schedules))
	for id, schedule := range s.schedules {
		result[id] = schedule
	}
	return result
}

// AddListener adds an event listener.
func (s *Scheduler) AddListener(listener Listener) {
	s.mu.Lock()
	s.listeners = append(s.listeners, listener)
	s.mu.Unlock()
}

func (s *Scheduler) emitEvent(event *DrillEvent) {
	s.mu.RLock()
	listeners := s.listeners
	s.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// Stats returns scheduler statistics.
func (s *Scheduler) Stats() *SchedulerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &SchedulerStats{
		TotalSchedules: len(s.schedules),
		TotalDrills:    len(s.drills),
		ByStatus:       make(map[DrillStatus]int),
		ByType:         make(map[DrillType]int),
	}

	for _, drill := range s.drills {
		drill.mu.RLock()
		stats.ByStatus[drill.Status]++
		stats.ByType[drill.Type]++
		drill.mu.RUnlock()
	}

	for _, running := range s.running {
		if running {
			stats.RunningDrills++
		}
	}

	return stats
}

// SchedulerStats holds scheduler statistics.
type SchedulerStats struct {
	TotalSchedules int
	TotalDrills    int
	RunningDrills  int
	ByStatus       map[DrillStatus]int
	ByType         map[DrillType]int
}

// MockExecutor is a mock executor for testing.
type MockExecutor struct {
	ValidateFunc func(config *DrillConfig) error
	ExecuteFunc  func(ctx context.Context, config *DrillConfig) (*DrillResults, error)
	CleanupFunc  func(ctx context.Context, drill *Drill) error
}

// Validate validates the drill config.
func (e *MockExecutor) Validate(config *DrillConfig) error {
	if e.ValidateFunc != nil {
		return e.ValidateFunc(config)
	}
	return nil
}

// Execute executes the drill.
func (e *MockExecutor) Execute(ctx context.Context, config *DrillConfig) (*DrillResults, error) {
	if e.ExecuteFunc != nil {
		return e.ExecuteFunc(ctx, config)
	}
	return &DrillResults{
		Success: true,
		Steps: []*DrillStep{
			{
				Name:      "mock_step",
				Status:    StatusCompleted,
				StartTime: time.Now(),
				EndTime:   time.Now(),
			},
		},
		Metrics: map[string]float64{
			"recovery_time_seconds": 5.0,
		},
	}, nil
}

// Cleanup cleans up after the drill.
func (e *MockExecutor) Cleanup(ctx context.Context, drill *Drill) error {
	if e.CleanupFunc != nil {
		return e.CleanupFunc(ctx, drill)
	}
	return nil
}
