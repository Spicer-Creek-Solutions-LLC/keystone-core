package state

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

func TestSQLiteStore_AgentOperations(t *testing.T) {
	// Create temporary database
	tmpFile := "/tmp/test-keystone-core-" + time.Now().Format("20060102150405") + ".db"
	defer os.Remove(tmpFile)

	config := &Config{
		Backend:    "sqlite",
		SQLitePath: tmpFile,
		SQLiteWAL:  true,
	}

	store, err := NewSQLiteStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Test SaveAgent
	agent := &AgentRecord{
		ID:              "agent-1",
		Hostname:        "test-host",
		OS:              "linux",
		Architecture:    "amd64",
		IPAddresses:     []string{"192.168.1.1", "10.0.0.1"},
		PlatformVersion: "5.10.0",
		AgentVersion:    "1.0.0",
		Labels:          map[string]string{"env": "test"},
		Status:          pb.AgentStatus_AGENT_STATUS_ONLINE,
		LastHeartbeat:   time.Now(),
		RegisteredAt:    time.Now(),
		UpdatedAt:       time.Now(),
		CPUPercent:      25.5,
		MemoryPercent:   60.0,
		DiskPercent:     45.0,
		LoadAverage:     []float32{1.0, 1.5, 2.0},
	}

	err = store.SaveAgent(ctx, agent)
	if err != nil {
		t.Fatalf("Failed to save agent: %v", err)
	}

	// Test GetAgent
	retrieved, err := store.GetAgent(ctx, "agent-1")
	if err != nil {
		t.Fatalf("Failed to get agent: %v", err)
	}

	if retrieved.ID != agent.ID {
		t.Errorf("Expected ID %s, got %s", agent.ID, retrieved.ID)
	}
	if retrieved.Hostname != agent.Hostname {
		t.Errorf("Expected hostname %s, got %s", agent.Hostname, retrieved.Hostname)
	}
	if retrieved.Status != agent.Status {
		t.Errorf("Expected status %v, got %v", agent.Status, retrieved.Status)
	}

	// Test ListAgents
	agents, err := store.ListAgents(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list agents: %v", err)
	}

	if len(agents) != 1 {
		t.Errorf("Expected 1 agent, got %d", len(agents))
	}

	// Test UpdateAgentStatus
	newTime := time.Now()
	err = store.UpdateAgentStatus(ctx, "agent-1", pb.AgentStatus_AGENT_STATUS_OFFLINE, newTime)
	if err != nil {
		t.Fatalf("Failed to update agent status: %v", err)
	}

	updated, err := store.GetAgent(ctx, "agent-1")
	if err != nil {
		t.Fatalf("Failed to get updated agent: %v", err)
	}

	if updated.Status != pb.AgentStatus_AGENT_STATUS_OFFLINE {
		t.Errorf("Expected status OFFLINE, got %v", updated.Status)
	}

	// Test UpdateAgentMetrics
	metrics := &pb.SystemMetrics{
		CpuPercent:    50.0,
		MemoryPercent: 75.0,
		DiskPercent:   80.0,
		LoadAverage:   []float32{2.0, 2.5, 3.0},
	}

	err = store.UpdateAgentMetrics(ctx, "agent-1", metrics)
	if err != nil {
		t.Fatalf("Failed to update metrics: %v", err)
	}

	// Test DeleteAgent
	err = store.DeleteAgent(ctx, "agent-1")
	if err != nil {
		t.Fatalf("Failed to delete agent: %v", err)
	}

	_, err = store.GetAgent(ctx, "agent-1")
	if err == nil {
		t.Error("Expected error getting deleted agent")
	}
}

func TestSQLiteStore_CommandOperations(t *testing.T) {
	tmpFile := "/tmp/test-keystone-core-cmd-" + time.Now().Format("20060102150405") + ".db"
	defer os.Remove(tmpFile)

	config := &Config{
		Backend:    "sqlite",
		SQLitePath: tmpFile,
		SQLiteWAL:  true,
	}

	store, err := NewSQLiteStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create an agent first (for foreign key)
	agent := &AgentRecord{
		ID:            "agent-1",
		Hostname:      "test-host",
		OS:            "linux",
		Architecture:  "amd64",
		Status:        pb.AgentStatus_AGENT_STATUS_ONLINE,
		LastHeartbeat: time.Now(),
		RegisteredAt:  time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.SaveAgent(ctx, agent)

	// Test SaveCommand
	cmd := &CommandRecord{
		ID:         "cmd-1",
		AgentID:    "agent-1",
		Command:    "echo",
		Args:       []string{"hello"},
		Env:        map[string]string{"PATH": "/usr/bin"},
		WorkingDir: "/tmp",
		User:       "test",
		Timeout:    300,
		Status:     pb.CommandStatus_COMMAND_STATUS_PENDING,
		CreatedAt:  time.Now(),
	}

	err = store.SaveCommand(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to save command: %v", err)
	}

	// Test GetCommand
	retrieved, err := store.GetCommand(ctx, "cmd-1")
	if err != nil {
		t.Fatalf("Failed to get command: %v", err)
	}

	if retrieved.ID != cmd.ID {
		t.Errorf("Expected ID %s, got %s", cmd.ID, retrieved.ID)
	}
	if retrieved.Command != cmd.Command {
		t.Errorf("Expected command %s, got %s", cmd.Command, retrieved.Command)
	}

	// Test ListCommands
	commands, err := store.ListCommands(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list commands: %v", err)
	}

	if len(commands) != 1 {
		t.Errorf("Expected 1 command, got %d", len(commands))
	}

	// Test UpdateCommandStatus
	err = store.UpdateCommandStatus(ctx, "cmd-1", pb.CommandStatus_COMMAND_STATUS_RUNNING)
	if err != nil {
		t.Fatalf("Failed to update command status: %v", err)
	}

	// Test UpdateCommandResult
	startTime := time.Now()
	endTime := startTime.Add(5 * time.Second)
	result := &CommandResult{
		Status:      pb.CommandStatus_COMMAND_STATUS_COMPLETED,
		ExitCode:    0,
		Stdout:      "hello\n",
		Stderr:      "",
		StartedAt:   startTime,
		CompletedAt: endTime,
		DurationMs:  5000,
	}

	err = store.UpdateCommandResult(ctx, "cmd-1", result)
	if err != nil {
		t.Fatalf("Failed to update command result: %v", err)
	}

	updated, err := store.GetCommand(ctx, "cmd-1")
	if err != nil {
		t.Fatalf("Failed to get updated command: %v", err)
	}

	if updated.Status != pb.CommandStatus_COMMAND_STATUS_COMPLETED {
		t.Errorf("Expected status COMPLETED, got %v", updated.Status)
	}
	if updated.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", updated.ExitCode)
	}
}

func TestSQLiteStore_ListAgents_WithFilters(t *testing.T) {
	tmpFile := "/tmp/test-keystone-core-filter-" + time.Now().Format("20060102150405") + ".db"
	defer os.Remove(tmpFile)

	config := &Config{
		Backend:    "sqlite",
		SQLitePath: tmpFile,
	}

	store, err := NewSQLiteStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create multiple agents
	for i := 1; i <= 5; i++ {
		agent := &AgentRecord{
			ID:            fmt.Sprintf("agent-%d", i),
			Hostname:      fmt.Sprintf("host-%d", i),
			OS:            "linux",
			Architecture:  "amd64",
			Status:        pb.AgentStatus_AGENT_STATUS_ONLINE,
			LastHeartbeat: time.Now(),
			RegisteredAt:  time.Now(),
			UpdatedAt:     time.Now(),
		}
		if i%2 == 0 {
			agent.Status = pb.AgentStatus_AGENT_STATUS_OFFLINE
		}
		store.SaveAgent(ctx, agent)
	}

	// Test filter by status
	statusFilter := pb.AgentStatus_AGENT_STATUS_ONLINE
	filter := &AgentFilter{
		Status: &statusFilter,
	}

	agents, err := store.ListAgents(ctx, filter)
	if err != nil {
		t.Fatalf("Failed to list agents with filter: %v", err)
	}

	if len(agents) != 3 {
		t.Errorf("Expected 3 online agents, got %d", len(agents))
	}

	// Test pagination
	filter = &AgentFilter{
		Limit:  2,
		Offset: 0,
	}

	agents, err = store.ListAgents(ctx, filter)
	if err != nil {
		t.Fatalf("Failed to list agents with pagination: %v", err)
	}

	if len(agents) != 2 {
		t.Errorf("Expected 2 agents with limit, got %d", len(agents))
	}
}

func TestSQLiteStore_Ping(t *testing.T) {
	tmpFile := "/tmp/test-keystone-core-ping-" + time.Now().Format("20060102150405") + ".db"
	defer os.Remove(tmpFile)

	config := &Config{
		Backend:    "sqlite",
		SQLitePath: tmpFile,
	}

	store, err := NewSQLiteStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	err = store.Ping(ctx)
	if err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

func TestNewStore_SQLite(t *testing.T) {
	tmpFile := "/tmp/test-keystone-core-newstore-" + time.Now().Format("20060102150405") + ".db"
	defer os.Remove(tmpFile)

	config := &Config{
		Backend:    "sqlite",
		SQLitePath: tmpFile,
	}

	store, err := NewStore(config)
	if err != nil {
		t.Fatalf("NewStore failed for SQLite: %v", err)
	}
	defer store.Close()

	if store == nil {
		t.Error("Expected non-nil store")
	}
}

func TestNewStore_PostgreSQL_RequiresDSN(t *testing.T) {
	config := &Config{
		Backend:       "postgresql",
		PostgreSQLDSN: "", // Empty DSN should fail
	}

	_, err := NewStore(config)
	if err == nil {
		t.Error("Expected error for PostgreSQL without DSN")
	}
}

func TestNewStore_InvalidBackend(t *testing.T) {
	config := &Config{
		Backend: "invalid-backend",
	}

	_, err := NewStore(config)
	if err == nil {
		t.Error("Expected error for invalid backend")
	}
	expectedError := "unsupported backend: invalid-backend"
	if err.Error() != expectedError {
		t.Errorf("Expected '%s' error, got: %v", expectedError, err)
	}
}

func TestSQLiteStore_ListCommands_WithFilters(t *testing.T) {
	tmpFile := "/tmp/test-keystone-core-cmdfilter-" + time.Now().Format("20060102150405") + ".db"
	defer os.Remove(tmpFile)

	config := &Config{
		Backend:    "sqlite",
		SQLitePath: tmpFile,
	}

	store, err := NewSQLiteStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create agents
	agent1 := &AgentRecord{
		ID:            "agent-1",
		Hostname:      "host-1",
		OS:            "linux",
		Architecture:  "amd64",
		Status:        pb.AgentStatus_AGENT_STATUS_ONLINE,
		LastHeartbeat: time.Now(),
		RegisteredAt:  time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.SaveAgent(ctx, agent1)

	agent2 := &AgentRecord{
		ID:            "agent-2",
		Hostname:      "host-2",
		OS:            "linux",
		Architecture:  "amd64",
		Status:        pb.AgentStatus_AGENT_STATUS_ONLINE,
		LastHeartbeat: time.Now(),
		RegisteredAt:  time.Now(),
		UpdatedAt:     time.Now(),
	}
	store.SaveAgent(ctx, agent2)

	// Create commands for different agents with different statuses and times
	now := time.Now()
	commands := []*CommandRecord{
		{
			ID:        "cmd-1",
			AgentID:   "agent-1",
			Command:   "echo",
			Args:      []string{"test1"},
			Status:    pb.CommandStatus_COMMAND_STATUS_PENDING,
			CreatedAt: now.Add(-10 * time.Minute),
		},
		{
			ID:        "cmd-2",
			AgentID:   "agent-1",
			Command:   "echo",
			Args:      []string{"test2"},
			Status:    pb.CommandStatus_COMMAND_STATUS_RUNNING,
			CreatedAt: now.Add(-5 * time.Minute),
		},
		{
			ID:        "cmd-3",
			AgentID:   "agent-2",
			Command:   "echo",
			Args:      []string{"test3"},
			Status:    pb.CommandStatus_COMMAND_STATUS_COMPLETED,
			CreatedAt: now.Add(-2 * time.Minute),
		},
		{
			ID:        "cmd-4",
			AgentID:   "agent-2",
			Command:   "echo",
			Args:      []string{"test4"},
			Status:    pb.CommandStatus_COMMAND_STATUS_FAILED,
			CreatedAt: now,
		},
	}

	for _, cmd := range commands {
		err := store.SaveCommand(ctx, cmd)
		if err != nil {
			t.Fatalf("Failed to save command %s: %v", cmd.ID, err)
		}
	}

	// Test 1: Filter by AgentID
	t.Run("FilterByAgentID", func(t *testing.T) {
		filter := &CommandFilter{
			AgentID: "agent-1",
		}

		result, err := store.ListCommands(ctx, filter)
		if err != nil {
			t.Fatalf("Failed to list commands with agent filter: %v", err)
		}

		if len(result) != 2 {
			t.Errorf("Expected 2 commands for agent-1, got %d", len(result))
		}
	})

	// Test 2: Filter by Status
	t.Run("FilterByStatus", func(t *testing.T) {
		status := pb.CommandStatus_COMMAND_STATUS_COMPLETED
		filter := &CommandFilter{
			Status: &status,
		}

		result, err := store.ListCommands(ctx, filter)
		if err != nil {
			t.Fatalf("Failed to list commands with status filter: %v", err)
		}

		if len(result) != 1 {
			t.Errorf("Expected 1 completed command, got %d", len(result))
		}

		if result[0].Status != pb.CommandStatus_COMMAND_STATUS_COMPLETED {
			t.Errorf("Expected status COMPLETED, got %v", result[0].Status)
		}
	})

	// Test 3: Filter by StartTime
	t.Run("FilterByStartTime", func(t *testing.T) {
		startTime := now.Add(-6 * time.Minute)
		filter := &CommandFilter{
			StartTime: &startTime,
		}

		result, err := store.ListCommands(ctx, filter)
		if err != nil {
			t.Fatalf("Failed to list commands with start time filter: %v", err)
		}

		// Should get cmd-2, cmd-3, cmd-4 (created after -6 minutes)
		if len(result) != 3 {
			t.Errorf("Expected 3 commands after start time, got %d", len(result))
		}
	})

	// Test 4: Filter by EndTime
	t.Run("FilterByEndTime", func(t *testing.T) {
		endTime := now.Add(-3 * time.Minute)
		filter := &CommandFilter{
			EndTime: &endTime,
		}

		result, err := store.ListCommands(ctx, filter)
		if err != nil {
			t.Fatalf("Failed to list commands with end time filter: %v", err)
		}

		// Should get cmd-1, cmd-2 (created before -3 minutes)
		if len(result) != 2 {
			t.Errorf("Expected 2 commands before end time, got %d", len(result))
		}
	})

	// Test 5: Sort by different fields
	t.Run("SortByCreatedAt", func(t *testing.T) {
		filter := &CommandFilter{
			SortBy:    "created_at",
			SortOrder: "ASC",
		}

		result, err := store.ListCommands(ctx, filter)
		if err != nil {
			t.Fatalf("Failed to list commands with sort: %v", err)
		}

		if len(result) < 2 {
			t.Fatal("Need at least 2 commands for sort test")
		}

		// First should be oldest (cmd-1)
		if result[0].ID != "cmd-1" {
			t.Errorf("Expected first command to be cmd-1, got %s", result[0].ID)
		}
	})

	// Test 6: Pagination with Limit
	t.Run("PaginationLimit", func(t *testing.T) {
		filter := &CommandFilter{
			Limit: 2,
		}

		result, err := store.ListCommands(ctx, filter)
		if err != nil {
			t.Fatalf("Failed to list commands with limit: %v", err)
		}

		if len(result) != 2 {
			t.Errorf("Expected 2 commands with limit, got %d", len(result))
		}
	})

	// Test 7: Pagination with Limit and Offset
	t.Run("PaginationLimitAndOffset", func(t *testing.T) {
		filter := &CommandFilter{
			Limit:  2,
			Offset: 1,
		}

		result, err := store.ListCommands(ctx, filter)
		if err != nil {
			t.Fatalf("Failed to list commands with limit and offset: %v", err)
		}

		if len(result) != 2 {
			t.Errorf("Expected 2 commands with limit and offset, got %d", len(result))
		}
	})

	// Test 8: Combined filters
	t.Run("CombinedFilters", func(t *testing.T) {
		status := pb.CommandStatus_COMMAND_STATUS_RUNNING
		filter := &CommandFilter{
			AgentID: "agent-1",
			Status:  &status,
		}

		result, err := store.ListCommands(ctx, filter)
		if err != nil {
			t.Fatalf("Failed to list commands with combined filters: %v", err)
		}

		if len(result) != 1 {
			t.Errorf("Expected 1 running command for agent-1, got %d", len(result))
		}

		if result[0].ID != "cmd-2" {
			t.Errorf("Expected cmd-2, got %s", result[0].ID)
		}
	})
}

func TestSQLiteStore_GetAgent_NotFound(t *testing.T) {
	tmpFile := "/tmp/test-keystone-core-notfound-" + time.Now().Format("20060102150405") + ".db"
	defer os.Remove(tmpFile)

	config := &Config{
		Backend:    "sqlite",
		SQLitePath: tmpFile,
	}

	store, err := NewSQLiteStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	_, err = store.GetAgent(ctx, "nonexistent-agent")
	if err == nil {
		t.Error("Expected error when getting nonexistent agent")
	}
}

func TestSQLiteStore_GetCommand_NotFound(t *testing.T) {
	tmpFile := "/tmp/test-keystone-core-cmdnotfound-" + time.Now().Format("20060102150405") + ".db"
	defer os.Remove(tmpFile)

	config := &Config{
		Backend:    "sqlite",
		SQLitePath: tmpFile,
	}

	store, err := NewSQLiteStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	_, err = store.GetCommand(ctx, "nonexistent-command")
	if err == nil {
		t.Error("Expected error when getting nonexistent command")
	}
}

func TestSQLiteStore_CompleteBatchJob(t *testing.T) {
	tmpFile := "/tmp/test-keystone-core-complete-batch-" + time.Now().Format("20060102150405") + ".db"
	defer os.Remove(tmpFile)

	config := &Config{
		Backend:    "sqlite",
		SQLitePath: tmpFile,
		SQLiteWAL:  true,
	}

	store, err := NewSQLiteStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now()

	// Create a batch job first
	job := &BatchJobRecord{
		ID:        "batch-txn-test",
		Target:    "*",
		Command:   "echo test",
		Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING,
		CreatedAt: now,
	}
	if err := store.SaveBatchJob(ctx, job); err != nil {
		t.Fatalf("SaveBatchJob failed: %v", err)
	}

	// Complete it atomically
	completedAt := now.Add(5 * time.Second)
	progress := &BatchJobProgress{
		TotalAgents:      10,
		CompletedAgents:  10,
		SuccessfulAgents: 8,
		FailedAgents:     2,
		SuccessRate:      80.0,
		StartedAt:        &now,
		CompletedAt:      &completedAt,
		DurationMs:       5000,
	}

	err = store.CompleteBatchJob(ctx, "batch-txn-test", pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED, progress)
	if err != nil {
		t.Fatalf("CompleteBatchJob failed: %v", err)
	}

	// Verify both status and progress were written
	result, err := store.GetBatchJob(ctx, "batch-txn-test")
	if err != nil {
		t.Fatalf("GetBatchJob failed: %v", err)
	}

	if result.Status != pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED {
		t.Errorf("expected COMPLETED status, got %v", result.Status)
	}
	if result.TotalAgents != 10 {
		t.Errorf("expected TotalAgents=10, got %d", result.TotalAgents)
	}
	if result.SuccessfulAgents != 8 {
		t.Errorf("expected SuccessfulAgents=8, got %d", result.SuccessfulAgents)
	}
}
