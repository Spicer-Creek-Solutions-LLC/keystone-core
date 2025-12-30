package state

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// getTestPostgreSQLDSN returns the PostgreSQL DSN for testing.
// Set KSCORE_TEST_POSTGRES_DSN environment variable to run PostgreSQL tests.
// Example: KSCORE_TEST_POSTGRES_DSN="postgres://user:pass@localhost:5432/testdb?sslmode=disable"
func getTestPostgreSQLDSN() string {
	return os.Getenv("KSCORE_TEST_POSTGRES_DSN")
}

// skipIfNoPostgreSQL skips the test if PostgreSQL is not available
func skipIfNoPostgreSQL(t *testing.T) {
	if getTestPostgreSQLDSN() == "" {
		t.Skip("Skipping PostgreSQL test: KSCORE_TEST_POSTGRES_DSN not set")
	}
}

// createTestPostgreSQLStore creates a store for testing, cleaning up tables first
func createTestPostgreSQLStore(t *testing.T) *PostgreSQLStore {
	dsn := getTestPostgreSQLDSN()
	config := &Config{
		Backend:           "postgresql",
		PostgreSQLDSN:     dsn,
		PostgreSQLMaxOpen: 5,
		PostgreSQLMaxIdle: 2,
	}

	store, err := NewPostgreSQLStore(config)
	if err != nil {
		t.Fatalf("Failed to create PostgreSQL store: %v", err)
	}

	// Clean up tables for fresh test
	ctx := context.Background()
	_, err = store.db.ExecContext(ctx, `
		TRUNCATE TABLE batch_agent_results CASCADE;
		TRUNCATE TABLE batch_jobs CASCADE;
		TRUNCATE TABLE commands CASCADE;
		TRUNCATE TABLE agents CASCADE;
	`)
	if err != nil {
		store.Close()
		t.Fatalf("Failed to clean up tables: %v", err)
	}

	return store
}

func TestPostgreSQLStore_AgentOperations(t *testing.T) {
	skipIfNoPostgreSQL(t)

	store := createTestPostgreSQLStore(t)
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
		LastHeartbeat:   time.Now().Truncate(time.Microsecond),
		RegisteredAt:    time.Now().Truncate(time.Microsecond),
		UpdatedAt:       time.Now().Truncate(time.Microsecond),
		CPUPercent:      25.5,
		MemoryPercent:   60.0,
		DiskPercent:     45.0,
		LoadAverage:     []float32{1.0, 1.5, 2.0},
	}

	err := store.SaveAgent(ctx, agent)
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
	newTime := time.Now().Truncate(time.Microsecond)
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

func TestPostgreSQLStore_CommandOperations(t *testing.T) {
	skipIfNoPostgreSQL(t)

	store := createTestPostgreSQLStore(t)
	defer store.Close()

	ctx := context.Background()

	// Create an agent first (for foreign key)
	agent := &AgentRecord{
		ID:            "agent-1",
		Hostname:      "test-host",
		OS:            "linux",
		Architecture:  "amd64",
		Status:        pb.AgentStatus_AGENT_STATUS_ONLINE,
		LastHeartbeat: time.Now().Truncate(time.Microsecond),
		RegisteredAt:  time.Now().Truncate(time.Microsecond),
		UpdatedAt:     time.Now().Truncate(time.Microsecond),
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
		CreatedAt:  time.Now().Truncate(time.Microsecond),
	}

	err := store.SaveCommand(ctx, cmd)
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
	startTime := time.Now().Truncate(time.Microsecond)
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

func TestPostgreSQLStore_BatchJobOperations(t *testing.T) {
	skipIfNoPostgreSQL(t)

	store := createTestPostgreSQLStore(t)
	defer store.Close()

	ctx := context.Background()

	// Create an agent first
	agent := &AgentRecord{
		ID:            "agent-1",
		Hostname:      "test-host",
		OS:            "linux",
		Architecture:  "amd64",
		Status:        pb.AgentStatus_AGENT_STATUS_ONLINE,
		LastHeartbeat: time.Now().Truncate(time.Microsecond),
		RegisteredAt:  time.Now().Truncate(time.Microsecond),
		UpdatedAt:     time.Now().Truncate(time.Microsecond),
	}
	store.SaveAgent(ctx, agent)

	// Test SaveBatchJob
	job := &BatchJobRecord{
		ID:          "batch-1",
		Target:      "os:linux",
		Command:     "uptime",
		Args:        []string{},
		Env:         map[string]string{},
		WorkingDir:  "/tmp",
		User:        "root",
		Timeout:     60,
		Concurrency: 10,
		Status:      pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING,
		CreatedAt:   time.Now().Truncate(time.Microsecond),
	}

	err := store.SaveBatchJob(ctx, job)
	if err != nil {
		t.Fatalf("Failed to save batch job: %v", err)
	}

	// Test GetBatchJob
	retrieved, err := store.GetBatchJob(ctx, "batch-1")
	if err != nil {
		t.Fatalf("Failed to get batch job: %v", err)
	}

	if retrieved.ID != job.ID {
		t.Errorf("Expected ID %s, got %s", job.ID, retrieved.ID)
	}
	if retrieved.Target != job.Target {
		t.Errorf("Expected target %s, got %s", job.Target, retrieved.Target)
	}

	// Test UpdateBatchJobStatus
	err = store.UpdateBatchJobStatus(ctx, "batch-1", pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING)
	if err != nil {
		t.Fatalf("Failed to update batch job status: %v", err)
	}

	// Test UpdateBatchJobProgress
	startTime := time.Now().Truncate(time.Microsecond)
	progress := &BatchJobProgress{
		TotalAgents:      5,
		CompletedAgents:  3,
		SuccessfulAgents: 2,
		FailedAgents:     1,
		SuccessRate:      66.67,
		StartedAt:        &startTime,
	}

	err = store.UpdateBatchJobProgress(ctx, "batch-1", progress)
	if err != nil {
		t.Fatalf("Failed to update batch job progress: %v", err)
	}

	// Test SaveBatchAgentResult
	agentResult := &BatchAgentResultRecord{
		BatchJobID: "batch-1",
		AgentID:    "agent-1",
		Success:    true,
		ExitCode:   0,
		DurationMs: 1500,
		CreatedAt:  time.Now().Truncate(time.Microsecond),
	}

	err = store.SaveBatchAgentResult(ctx, "batch-1", agentResult)
	if err != nil {
		t.Fatalf("Failed to save batch agent result: %v", err)
	}

	// Verify agent result is included
	retrieved, err = store.GetBatchJob(ctx, "batch-1")
	if err != nil {
		t.Fatalf("Failed to get batch job with results: %v", err)
	}

	if len(retrieved.AgentResults) != 1 {
		t.Errorf("Expected 1 agent result, got %d", len(retrieved.AgentResults))
	}

	// Test ListBatchJobs
	jobs, err := store.ListBatchJobs(ctx, &BatchJobFilter{})
	if err != nil {
		t.Fatalf("Failed to list batch jobs: %v", err)
	}

	if len(jobs) != 1 {
		t.Errorf("Expected 1 batch job, got %d", len(jobs))
	}
}

func TestPostgreSQLStore_ListAgents_WithFilters(t *testing.T) {
	skipIfNoPostgreSQL(t)

	store := createTestPostgreSQLStore(t)
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
			LastHeartbeat: time.Now().Truncate(time.Microsecond),
			RegisteredAt:  time.Now().Truncate(time.Microsecond),
			UpdatedAt:     time.Now().Truncate(time.Microsecond),
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

func TestPostgreSQLStore_Ping(t *testing.T) {
	skipIfNoPostgreSQL(t)

	store := createTestPostgreSQLStore(t)
	defer store.Close()

	ctx := context.Background()
	err := store.Ping(ctx)
	if err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

func TestNewStore_PostgreSQL(t *testing.T) {
	skipIfNoPostgreSQL(t)

	dsn := getTestPostgreSQLDSN()
	config := &Config{
		Backend:       "postgresql",
		PostgreSQLDSN: dsn,
	}

	store, err := NewStore(config)
	if err != nil {
		t.Fatalf("NewStore failed for PostgreSQL: %v", err)
	}
	defer store.Close()

	if store == nil {
		t.Error("Expected non-nil store")
	}
}

func TestNewStore_PostgreSQL_NoDSN(t *testing.T) {
	config := &Config{
		Backend:       "postgresql",
		PostgreSQLDSN: "", // No DSN provided
	}

	_, err := NewStore(config)
	if err == nil {
		t.Error("Expected error for PostgreSQL without DSN")
	}
}

func TestPostgreSQLStore_ListCommands_WithFilters(t *testing.T) {
	skipIfNoPostgreSQL(t)

	store := createTestPostgreSQLStore(t)
	defer store.Close()

	ctx := context.Background()

	// Create agents
	for i := 1; i <= 2; i++ {
		agent := &AgentRecord{
			ID:            fmt.Sprintf("agent-%d", i),
			Hostname:      fmt.Sprintf("host-%d", i),
			OS:            "linux",
			Architecture:  "amd64",
			Status:        pb.AgentStatus_AGENT_STATUS_ONLINE,
			LastHeartbeat: time.Now().Truncate(time.Microsecond),
			RegisteredAt:  time.Now().Truncate(time.Microsecond),
			UpdatedAt:     time.Now().Truncate(time.Microsecond),
		}
		store.SaveAgent(ctx, agent)
	}

	// Create commands
	for i := 1; i <= 4; i++ {
		cmd := &CommandRecord{
			ID:        fmt.Sprintf("cmd-%d", i),
			AgentID:   fmt.Sprintf("agent-%d", (i%2)+1),
			Command:   "test",
			Status:    pb.CommandStatus_COMMAND_STATUS_COMPLETED,
			CreatedAt: time.Now().Truncate(time.Microsecond),
		}
		store.SaveCommand(ctx, cmd)
	}

	// Test filter by agent
	filter := &CommandFilter{
		AgentID: "agent-1",
	}

	commands, err := store.ListCommands(ctx, filter)
	if err != nil {
		t.Fatalf("Failed to list commands: %v", err)
	}

	if len(commands) != 2 {
		t.Errorf("Expected 2 commands for agent-1, got %d", len(commands))
	}
}

func TestPostgreSQLStore_ListBatchJobs_WithFilters(t *testing.T) {
	skipIfNoPostgreSQL(t)

	store := createTestPostgreSQLStore(t)
	defer store.Close()

	ctx := context.Background()

	// Create batch jobs
	for i := 1; i <= 4; i++ {
		job := &BatchJobRecord{
			ID:        fmt.Sprintf("batch-%d", i),
			Target:    fmt.Sprintf("env:test%d", i%2),
			Command:   "test",
			Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED,
			CreatedAt: time.Now().Truncate(time.Microsecond),
		}
		store.SaveBatchJob(ctx, job)
	}

	// Test filter by target
	filter := &BatchJobFilter{
		Target: "test0",
	}

	jobs, err := store.ListBatchJobs(ctx, filter)
	if err != nil {
		t.Fatalf("Failed to list batch jobs: %v", err)
	}

	if len(jobs) != 2 {
		t.Errorf("Expected 2 batch jobs with target test0, got %d", len(jobs))
	}

	// Test pagination
	filter = &BatchJobFilter{
		Limit:  2,
		Offset: 1,
	}

	jobs, err = store.ListBatchJobs(ctx, filter)
	if err != nil {
		t.Fatalf("Failed to list batch jobs with pagination: %v", err)
	}

	if len(jobs) != 2 {
		t.Errorf("Expected 2 batch jobs with limit, got %d", len(jobs))
	}
}
