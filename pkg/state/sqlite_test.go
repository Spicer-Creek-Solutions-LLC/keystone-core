package state

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	pb "github.com/titananvil/titan-anvil/pkg/api/v1"
)

func TestSQLiteStore_AgentOperations(t *testing.T) {
	// Create temporary database
	tmpFile := "/tmp/test-titan-anvil-" + time.Now().Format("20060102150405") + ".db"
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
	tmpFile := "/tmp/test-titan-anvil-cmd-" + time.Now().Format("20060102150405") + ".db"
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
	tmpFile := "/tmp/test-titan-anvil-filter-" + time.Now().Format("20060102150405") + ".db"
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
	tmpFile := "/tmp/test-titan-anvil-ping-" + time.Now().Format("20060102150405") + ".db"
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
