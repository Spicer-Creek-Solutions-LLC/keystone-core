package cluster

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestSingletonTaskType_Constants(t *testing.T) {
	tests := []struct {
		taskType SingletonTaskType
		want     string
	}{
		{SingletonTaskReactor, "reactor_coordinator"},
		{SingletonTaskScheduler, "scheduled_jobs"},
		{SingletonTaskCleanup, "cleanup"},
		{SingletonTaskMetrics, "metric_aggregation"},
		{SingletonTaskReports, "report_generation"},
		{SingletonTaskRebalance, "agent_rebalance"},
	}

	for _, tt := range tests {
		t.Run(string(tt.taskType), func(t *testing.T) {
			if string(tt.taskType) != tt.want {
				t.Errorf("SingletonTaskType = %v, want %v", string(tt.taskType), tt.want)
			}
		})
	}
}

func TestSingletonTask_Validation(t *testing.T) {
	// Create mock dependencies
	config := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	etcd := &EtcdClient{}
	le, _ := NewLeaderElector(config, etcd, "member-1")
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	manager, err := NewSingletonTaskManager(le, mm)
	if err != nil {
		t.Fatalf("Failed to create singleton task manager: %v", err)
	}

	tests := []struct {
		name    string
		task    *SingletonTask
		wantErr bool
	}{
		{
			name:    "nil task",
			task:    nil,
			wantErr: true,
		},
		{
			name: "empty type",
			task: &SingletonTask{
				Type: "",
				Run:  func(ctx context.Context) error { return nil },
			},
			wantErr: true,
		},
		{
			name: "nil run function",
			task: &SingletonTask{
				Type: SingletonTaskCleanup,
				Run:  nil,
			},
			wantErr: true,
		},
		{
			name: "valid task",
			task: &SingletonTask{
				Type: SingletonTaskCleanup,
				Name: "Test Cleanup",
				Run:  func(ctx context.Context) error { return nil },
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.RegisterTask(tt.task)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterTask() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSingletonTaskManager_RegisterUnregister(t *testing.T) {
	config := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	etcd := &EtcdClient{}
	le, _ := NewLeaderElector(config, etcd, "member-1")
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	manager, _ := NewSingletonTaskManager(le, mm)

	task := &SingletonTask{
		Type: SingletonTaskCleanup,
		Name: "Test Cleanup",
		Run:  func(ctx context.Context) error { return nil },
	}

	// Register
	if err := manager.RegisterTask(task); err != nil {
		t.Errorf("RegisterTask() error = %v", err)
	}

	// Register duplicate should fail
	if err := manager.RegisterTask(task); err == nil {
		t.Error("RegisterTask() should fail for duplicate")
	}

	// Unregister
	if err := manager.UnregisterTask(SingletonTaskCleanup); err != nil {
		t.Errorf("UnregisterTask() error = %v", err)
	}

	// Register again should work
	if err := manager.RegisterTask(task); err != nil {
		t.Errorf("RegisterTask() after unregister error = %v", err)
	}
}

func TestSingletonTaskManager_ListTasks(t *testing.T) {
	config := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	etcd := &EtcdClient{}
	le, _ := NewLeaderElector(config, etcd, "member-1")
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	manager, _ := NewSingletonTaskManager(le, mm)

	// Initially empty
	tasks := manager.ListTasks()
	if len(tasks) != 0 {
		t.Errorf("ListTasks() should return empty list initially, got %d", len(tasks))
	}

	// Register tasks
	task1 := &SingletonTask{Type: SingletonTaskCleanup, Run: func(ctx context.Context) error { return nil }}
	task2 := &SingletonTask{Type: SingletonTaskMetrics, Run: func(ctx context.Context) error { return nil }}

	manager.RegisterTask(task1)
	manager.RegisterTask(task2)

	tasks = manager.ListTasks()
	if len(tasks) != 2 {
		t.Errorf("ListTasks() = %d, want 2", len(tasks))
	}
}

func TestSingletonTaskManager_GetTaskStatus(t *testing.T) {
	config := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	etcd := &EtcdClient{}
	le, _ := NewLeaderElector(config, etcd, "member-1")
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	manager, _ := NewSingletonTaskManager(le, mm)

	// Non-existent task
	_, err := manager.GetTaskStatus(SingletonTaskCleanup)
	if err == nil {
		t.Error("GetTaskStatus() should fail for non-existent task")
	}

	// Register task
	task := &SingletonTask{
		Type: SingletonTaskCleanup,
		Name: "Test Cleanup",
		Run:  func(ctx context.Context) error { return nil },
	}
	manager.RegisterTask(task)

	// Get status
	status, err := manager.GetTaskStatus(SingletonTaskCleanup)
	if err != nil {
		t.Errorf("GetTaskStatus() error = %v", err)
	}

	if status.Type != SingletonTaskCleanup {
		t.Errorf("Status.Type = %v, want %v", status.Type, SingletonTaskCleanup)
	}

	if status.Name != "Test Cleanup" {
		t.Errorf("Status.Name = %v, want 'Test Cleanup'", status.Name)
	}

	if status.Running {
		t.Error("Status.Running should be false initially")
	}
}

func TestSingletonTaskManager_GetAllTaskStatuses(t *testing.T) {
	config := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	etcd := &EtcdClient{}
	le, _ := NewLeaderElector(config, etcd, "member-1")
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	manager, _ := NewSingletonTaskManager(le, mm)

	// Register tasks
	task1 := &SingletonTask{Type: SingletonTaskCleanup, Name: "Cleanup", Run: func(ctx context.Context) error { return nil }}
	task2 := &SingletonTask{Type: SingletonTaskMetrics, Name: "Metrics", Run: func(ctx context.Context) error { return nil }}

	manager.RegisterTask(task1)
	manager.RegisterTask(task2)

	statuses := manager.GetAllTaskStatuses()
	if len(statuses) != 2 {
		t.Errorf("GetAllTaskStatuses() = %d, want 2", len(statuses))
	}
}

func TestSingletonTaskManager_IsTaskRunning(t *testing.T) {
	config := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	etcd := &EtcdClient{}
	le, _ := NewLeaderElector(config, etcd, "member-1")
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	manager, _ := NewSingletonTaskManager(le, mm)

	task := &SingletonTask{
		Type: SingletonTaskCleanup,
		Run:  func(ctx context.Context) error { return nil },
	}
	manager.RegisterTask(task)

	// Initially not running
	if manager.IsTaskRunning(SingletonTaskCleanup) {
		t.Error("IsTaskRunning() should be false initially")
	}

	// Non-existent task
	if manager.IsTaskRunning(SingletonTaskMetrics) {
		t.Error("IsTaskRunning() should be false for non-existent task")
	}
}

func TestSingletonTaskStatus_Fields(t *testing.T) {
	now := time.Now()
	status := &SingletonTaskStatus{
		Type:       SingletonTaskCleanup,
		Name:       "Test Cleanup",
		Running:    true,
		MemberID:   "member-1",
		StartedAt:  now,
		LastRunAt:  now,
		RunCount:   5,
		ErrorCount: 1,
		LastError:  "test error",
	}

	if status.Type != SingletonTaskCleanup {
		t.Errorf("Type = %v, want %v", status.Type, SingletonTaskCleanup)
	}
	if status.Name != "Test Cleanup" {
		t.Errorf("Name = %v, want 'Test Cleanup'", status.Name)
	}
	if !status.Running {
		t.Error("Running should be true")
	}
	if status.MemberID != "member-1" {
		t.Errorf("MemberID = %v, want 'member-1'", status.MemberID)
	}
	if status.RunCount != 5 {
		t.Errorf("RunCount = %v, want 5", status.RunCount)
	}
	if status.ErrorCount != 1 {
		t.Errorf("ErrorCount = %v, want 1", status.ErrorCount)
	}
}

func TestSingletonTask_PeriodicExecution(t *testing.T) {
	config := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	etcd := &EtcdClient{}
	le, _ := NewLeaderElector(config, etcd, "member-1")
	mm := &MembershipManager{
		config:   config,
		etcd:     etcd,
		members:  make(map[string]*Member),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	manager, _ := NewSingletonTaskManager(le, mm)

	var runCount int32

	task := &SingletonTask{
		Type:     SingletonTaskCleanup,
		Name:     "Periodic Task",
		Interval: 50 * time.Millisecond,
		Run: func(ctx context.Context) error {
			atomic.AddInt32(&runCount, 1)
			return nil
		},
	}

	if err := manager.RegisterTask(task); err != nil {
		t.Fatalf("RegisterTask() error = %v", err)
	}

	// Task should have interval set
	manager.mu.RLock()
	registeredTask := manager.tasks[SingletonTaskCleanup]
	manager.mu.RUnlock()

	if registeredTask.Interval != 50*time.Millisecond {
		t.Errorf("Task.Interval = %v, want 50ms", registeredTask.Interval)
	}
}
