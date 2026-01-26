package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SingletonTaskType identifies a type of singleton task.
type SingletonTaskType string

const (
	// SingletonTaskReactor is the reactor coordinator task.
	SingletonTaskReactor SingletonTaskType = "reactor_coordinator"

	// SingletonTaskScheduler is the scheduled job coordinator task.
	SingletonTaskScheduler SingletonTaskType = "scheduled_jobs"

	// SingletonTaskCleanup is the cleanup task (log rotation, etc.).
	SingletonTaskCleanup SingletonTaskType = "cleanup"

	// SingletonTaskMetrics is the metric aggregation task.
	SingletonTaskMetrics SingletonTaskType = "metric_aggregation"

	// SingletonTaskReports is the report generation task.
	SingletonTaskReports SingletonTaskType = "report_generation"

	// SingletonTaskRebalance is the agent rebalancing task.
	SingletonTaskRebalance SingletonTaskType = "agent_rebalance"
)

// SingletonTask defines a task that should only run on one cluster member.
type SingletonTask struct {
	// Type identifies this task.
	Type SingletonTaskType

	// Name is the human-readable name.
	Name string

	// Run is the function that executes the task.
	// It should run continuously until the context is cancelled.
	Run func(ctx context.Context) error

	// OnStart is called when the task starts (optional).
	OnStart func() error

	// OnStop is called when the task stops (optional).
	OnStop func() error

	// Interval is how often to run if the task is periodic (0 = continuous).
	Interval time.Duration

	// LeaderOnly indicates if this task should only run on the leader.
	LeaderOnly bool
}

// SingletonTaskStatus represents the status of a singleton task.
type SingletonTaskStatus struct {
	Type       SingletonTaskType `json:"type"`
	Name       string            `json:"name"`
	Running    bool              `json:"running"`
	MemberID   string            `json:"member_id"`
	StartedAt  time.Time         `json:"started_at,omitempty"`
	LastRunAt  time.Time         `json:"last_run_at,omitempty"`
	RunCount   int64             `json:"run_count"`
	ErrorCount int64             `json:"error_count"`
	LastError  string            `json:"last_error,omitempty"`
}

// SingletonTaskManager manages singleton tasks across the cluster.
type SingletonTaskManager struct {
	leader         *LeaderElector
	membership     *MembershipManager
	tasks          map[SingletonTaskType]*SingletonTask
	taskStatuses   map[SingletonTaskType]*SingletonTaskStatus
	taskCancels    map[SingletonTaskType]context.CancelFunc
	mu             sync.RWMutex
	stopChan       chan struct{}
	started        bool
}

// NewSingletonTaskManager creates a new singleton task manager.
func NewSingletonTaskManager(leader *LeaderElector, membership *MembershipManager) (*SingletonTaskManager, error) {
	if leader == nil {
		return nil, fmt.Errorf("leader elector is required")
	}
	if membership == nil {
		return nil, fmt.Errorf("membership manager is required")
	}

	return &SingletonTaskManager{
		leader:       leader,
		membership:   membership,
		tasks:        make(map[SingletonTaskType]*SingletonTask),
		taskStatuses: make(map[SingletonTaskType]*SingletonTaskStatus),
		taskCancels:  make(map[SingletonTaskType]context.CancelFunc),
		stopChan:     make(chan struct{}),
	}, nil
}

// RegisterTask registers a singleton task.
func (m *SingletonTaskManager) RegisterTask(task *SingletonTask) error {
	if task == nil {
		return fmt.Errorf("task is required")
	}
	if task.Type == "" {
		return fmt.Errorf("task type is required")
	}
	if task.Run == nil {
		return fmt.Errorf("task run function is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tasks[task.Type]; exists {
		return fmt.Errorf("task %s already registered", task.Type)
	}

	m.tasks[task.Type] = task
	m.taskStatuses[task.Type] = &SingletonTaskStatus{
		Type:     task.Type,
		Name:     task.Name,
		Running:  false,
		MemberID: "",
	}

	return nil
}

// UnregisterTask unregisters a singleton task.
func (m *SingletonTaskManager) UnregisterTask(taskType SingletonTaskType) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop the task if running
	if cancel, exists := m.taskCancels[taskType]; exists {
		cancel()
		delete(m.taskCancels, taskType)
	}

	delete(m.tasks, taskType)
	delete(m.taskStatuses, taskType)

	return nil
}

// Start starts the singleton task manager.
func (m *SingletonTaskManager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return fmt.Errorf("singleton task manager already started")
	}
	m.started = true
	m.mu.Unlock()

	// Subscribe to leadership changes
	m.leader.AddObserver(m.onLeadershipChange)

	// Start initial task management
	go m.manageTasks(ctx)

	return nil
}

// Stop stops the singleton task manager.
func (m *SingletonTaskManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return nil
	}
	m.started = false
	m.mu.Unlock()

	close(m.stopChan)

	// Stop all running tasks
	m.stopAllTasks()

	return nil
}

// onLeadershipChange handles leadership change events.
func (m *SingletonTaskManager) onLeadershipChange(event LeadershipEvent) {
	m.mu.RLock()
	started := m.started
	m.mu.RUnlock()

	if !started {
		return
	}

	switch event.Type {
	case LeadershipEventElected:
		if event.LeaderID == m.leader.MemberID() {
			// We became leader, start leader-only tasks
			m.startLeaderTasks()
		} else {
			// Someone else became leader, stop leader-only tasks
			m.stopLeaderTasks()
		}
	case LeadershipEventResigned, LeadershipEventLost:
		if event.PreviousLeaderID == m.leader.MemberID() {
			// We lost leadership, stop leader-only tasks
			m.stopLeaderTasks()
		}
	}
}

// manageTasks continuously manages tasks based on leadership status.
func (m *SingletonTaskManager) manageTasks(ctx context.Context) {
	// Initial check
	m.reconcileTasks()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.reconcileTasks()
		}
	}
}

// reconcileTasks ensures tasks are running based on current state.
func (m *SingletonTaskManager) reconcileTasks() {
	isLeader := m.leader.IsLeader()

	m.mu.Lock()
	defer m.mu.Unlock()

	for taskType, task := range m.tasks {
		status := m.taskStatuses[taskType]
		_, isRunning := m.taskCancels[taskType]

		shouldRun := !task.LeaderOnly || isLeader

		if shouldRun && !isRunning {
			// Start the task
			m.startTaskLocked(taskType, task)
		} else if !shouldRun && isRunning {
			// Stop the task
			m.stopTaskLocked(taskType)
		}

		// Update status
		if status != nil {
			status.Running = isRunning
			if isRunning {
				status.MemberID = m.leader.MemberID()
			}
		}
	}
}

// startLeaderTasks starts all leader-only tasks.
func (m *SingletonTaskManager) startLeaderTasks() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for taskType, task := range m.tasks {
		if task.LeaderOnly {
			if _, running := m.taskCancels[taskType]; !running {
				m.startTaskLocked(taskType, task)
			}
		}
	}
}

// stopLeaderTasks stops all leader-only tasks.
func (m *SingletonTaskManager) stopLeaderTasks() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for taskType, task := range m.tasks {
		if task.LeaderOnly {
			if _, running := m.taskCancels[taskType]; running {
				m.stopTaskLocked(taskType)
			}
		}
	}
}

// stopAllTasks stops all running tasks.
func (m *SingletonTaskManager) stopAllTasks() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for taskType := range m.taskCancels {
		m.stopTaskLocked(taskType)
	}
}

// startTaskLocked starts a task. Must be called with m.mu held.
func (m *SingletonTaskManager) startTaskLocked(taskType SingletonTaskType, task *SingletonTask) {
	ctx, cancel := context.WithCancel(context.Background())
	m.taskCancels[taskType] = cancel

	status := m.taskStatuses[taskType]
	if status != nil {
		status.Running = true
		status.StartedAt = time.Now().UTC()
		status.MemberID = m.leader.MemberID()
	}

	// Call OnStart if defined
	if task.OnStart != nil {
		if err := task.OnStart(); err != nil {
			if status != nil {
				status.LastError = err.Error()
				status.ErrorCount++
			}
		}
	}

	// Run the task
	go m.runTask(ctx, taskType, task)
}

// stopTaskLocked stops a task. Must be called with m.mu held.
func (m *SingletonTaskManager) stopTaskLocked(taskType SingletonTaskType) {
	if cancel, exists := m.taskCancels[taskType]; exists {
		cancel()
		delete(m.taskCancels, taskType)
	}

	if status := m.taskStatuses[taskType]; status != nil {
		status.Running = false
	}

	// Call OnStop if defined
	if task, exists := m.tasks[taskType]; exists && task.OnStop != nil {
		task.OnStop()
	}
}

// runTask runs a task until cancelled.
func (m *SingletonTaskManager) runTask(ctx context.Context, taskType SingletonTaskType, task *SingletonTask) {
	defer func() {
		m.mu.Lock()
		if status := m.taskStatuses[taskType]; status != nil {
			status.Running = false
		}
		delete(m.taskCancels, taskType)
		m.mu.Unlock()
	}()

	if task.Interval > 0 {
		// Periodic task
		ticker := time.NewTicker(task.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.executeTask(ctx, taskType, task)
			}
		}
	} else {
		// Continuous task
		m.executeTask(ctx, taskType, task)
	}
}

// executeTask executes a task and updates status.
func (m *SingletonTaskManager) executeTask(ctx context.Context, taskType SingletonTaskType, task *SingletonTask) {
	m.mu.Lock()
	status := m.taskStatuses[taskType]
	if status != nil {
		status.LastRunAt = time.Now().UTC()
		status.RunCount++
	}
	m.mu.Unlock()

	if err := task.Run(ctx); err != nil && ctx.Err() == nil {
		m.mu.Lock()
		if status := m.taskStatuses[taskType]; status != nil {
			status.LastError = err.Error()
			status.ErrorCount++
		}
		m.mu.Unlock()
	}
}

// GetTaskStatus returns the status of a task.
func (m *SingletonTaskManager) GetTaskStatus(taskType SingletonTaskType) (*SingletonTaskStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status, exists := m.taskStatuses[taskType]
	if !exists {
		return nil, fmt.Errorf("task %s not found", taskType)
	}

	// Return a copy
	statusCopy := *status
	return &statusCopy, nil
}

// GetAllTaskStatuses returns the status of all tasks.
func (m *SingletonTaskManager) GetAllTaskStatuses() []*SingletonTaskStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]*SingletonTaskStatus, 0, len(m.taskStatuses))
	for _, status := range m.taskStatuses {
		statusCopy := *status
		statuses = append(statuses, &statusCopy)
	}

	return statuses
}

// IsTaskRunning returns true if a task is currently running on this member.
func (m *SingletonTaskManager) IsTaskRunning(taskType SingletonTaskType) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, running := m.taskCancels[taskType]
	return running
}

// ForceStartTask forces a task to start, even if conditions aren't met.
// This is useful for manual intervention.
func (m *SingletonTaskManager) ForceStartTask(taskType SingletonTaskType) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, exists := m.tasks[taskType]
	if !exists {
		return fmt.Errorf("task %s not found", taskType)
	}

	if _, running := m.taskCancels[taskType]; running {
		return fmt.Errorf("task %s is already running", taskType)
	}

	m.startTaskLocked(taskType, task)
	return nil
}

// ForceStopTask forces a task to stop.
func (m *SingletonTaskManager) ForceStopTask(taskType SingletonTaskType) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, running := m.taskCancels[taskType]; !running {
		return fmt.Errorf("task %s is not running", taskType)
	}

	m.stopTaskLocked(taskType)
	return nil
}

// ListTasks returns all registered tasks.
func (m *SingletonTaskManager) ListTasks() []SingletonTaskType {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tasks := make([]SingletonTaskType, 0, len(m.tasks))
	for taskType := range m.tasks {
		tasks = append(tasks, taskType)
	}
	return tasks
}
