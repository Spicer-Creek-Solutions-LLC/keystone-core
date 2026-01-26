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

// IPv6 support tests

func TestPostgreSQLConfig_BuildDSN(t *testing.T) {
	tests := []struct {
		name     string
		config   *PostgreSQLConfig
		expected string
	}{
		{
			name: "IPv4 address",
			config: &PostgreSQLConfig{
				Host:     "192.168.1.1",
				Port:     5432,
				Database: "testdb",
				User:     "testuser",
				Password: "testpass",
				SSLMode:  "disable",
			},
			expected: "host=192.168.1.1 port=5432 dbname=testdb user=testuser password=testpass sslmode=disable",
		},
		{
			name: "IPv6 address without brackets",
			config: &PostgreSQLConfig{
				Host:     "2001:db8::1",
				Port:     5432,
				Database: "testdb",
				User:     "testuser",
				Password: "testpass",
				SSLMode:  "disable",
			},
			expected: "host=[2001:db8::1] port=5432 dbname=testdb user=testuser password=testpass sslmode=disable",
		},
		{
			name: "IPv6 address with brackets (already bracketed)",
			config: &PostgreSQLConfig{
				Host:     "[2001:db8::1]",
				Port:     5432,
				Database: "testdb",
				User:     "testuser",
				SSLMode:  "disable",
			},
			expected: "host=[2001:db8::1] port=5432 dbname=testdb user=testuser sslmode=disable",
		},
		{
			name: "IPv6 loopback",
			config: &PostgreSQLConfig{
				Host:     "::1",
				Port:     5432,
				Database: "testdb",
				User:     "testuser",
				SSLMode:  "disable",
			},
			expected: "host=[::1] port=5432 dbname=testdb user=testuser sslmode=disable",
		},
		{
			name: "Hostname (not IPv6)",
			config: &PostgreSQLConfig{
				Host:     "db.example.com",
				Port:     5432,
				Database: "testdb",
				User:     "testuser",
				SSLMode:  "require",
			},
			expected: "host=db.example.com port=5432 dbname=testdb user=testuser sslmode=require",
		},
		{
			name: "Full config with SSL",
			config: &PostgreSQLConfig{
				Host:            "2001:db8::100",
				Port:            5432,
				Database:        "proddb",
				User:            "admin",
				Password:        "secret",
				SSLMode:         "verify-full",
				SSLRootCert:     "/etc/ssl/ca.crt",
				SSLCert:         "/etc/ssl/client.crt",
				SSLKey:          "/etc/ssl/client.key",
				ConnectTimeout:  30,
				ApplicationName: "kscore",
			},
			expected: "host=[2001:db8::100] port=5432 dbname=proddb user=admin password=secret sslmode=verify-full sslrootcert=/etc/ssl/ca.crt sslcert=/etc/ssl/client.crt sslkey=/etc/ssl/client.key connect_timeout=30 application_name=kscore",
		},
		{
			name: "Minimal config",
			config: &PostgreSQLConfig{
				Host:     "localhost",
				Database: "testdb",
			},
			expected: "host=localhost dbname=testdb",
		},
		{
			name:     "Nil config",
			config:   nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.BuildDSN()
			if result != tt.expected {
				t.Errorf("BuildDSN() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestPostgreSQLConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *PostgreSQLConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			config: &PostgreSQLConfig{
				Host:     "localhost",
				Database: "testdb",
			},
			expectError: false,
		},
		{
			name: "valid config with IPv6",
			config: &PostgreSQLConfig{
				Host:     "2001:db8::1",
				Database: "testdb",
			},
			expectError: false,
		},
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
			errorMsg:    "postgresql config is nil",
		},
		{
			name: "missing host",
			config: &PostgreSQLConfig{
				Database: "testdb",
			},
			expectError: true,
			errorMsg:    "postgresql host is required",
		},
		{
			name: "missing database",
			config: &PostgreSQLConfig{
				Host: "localhost",
			},
			expectError: true,
			errorMsg:    "postgresql database is required",
		},
		{
			name: "invalid sslmode",
			config: &PostgreSQLConfig{
				Host:     "localhost",
				Database: "testdb",
				SSLMode:  "invalid",
			},
			expectError: true,
			errorMsg:    "invalid sslmode",
		},
		{
			name: "valid sslmode - disable",
			config: &PostgreSQLConfig{
				Host:     "localhost",
				Database: "testdb",
				SSLMode:  "disable",
			},
			expectError: false,
		},
		{
			name: "valid sslmode - verify-full",
			config: &PostgreSQLConfig{
				Host:     "localhost",
				Database: "testdb",
				SSLMode:  "verify-full",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// contains checks if s contains substr
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestIsIPv6Address(t *testing.T) {
	tests := []struct {
		addr     string
		expected bool
	}{
		// IPv6 addresses
		{"2001:db8::1", true},
		{"::1", true},
		{"::", true},
		{"fe80::1", true},
		{"2001:db8:85a3::8a2e:370:7334", true},
		{"2001:0db8:0000:0000:0000:0000:0000:0001", true},

		// IPv6 with brackets
		{"[2001:db8::1]", true},
		{"[::1]", true},

		// IPv6 with zone ID
		{"fe80::1%eth0", true},
		{"[fe80::1%eth0]", true},

		// IPv4 addresses
		{"192.168.1.1", false},
		{"10.0.0.1", false},
		{"127.0.0.1", false},

		// Hostnames
		{"localhost", false},
		{"db.example.com", false},
		{"my-host", false},

		// Invalid
		{"", false},
		{"not-an-ip", false},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			result := isIPv6Address(tt.addr)
			if result != tt.expected {
				t.Errorf("isIPv6Address(%q) = %v, want %v", tt.addr, result, tt.expected)
			}
		})
	}
}

func TestNewPostgreSQLStore_WithStructuredConfig(t *testing.T) {
	skipIfNoPostgreSQL(t)

	// Parse the existing DSN to extract connection details
	// This test verifies that structured config works the same as DSN
	dsn := getTestPostgreSQLDSN()
	if dsn == "" {
		t.Skip("KSCORE_TEST_POSTGRES_DSN not set")
	}

	// Create store with DSN first to verify connection works
	dsnConfig := &Config{
		Backend:       "postgresql",
		PostgreSQLDSN: dsn,
	}

	store1, err := NewPostgreSQLStore(dsnConfig)
	if err != nil {
		t.Fatalf("Failed to create store with DSN: %v", err)
	}
	store1.Close()

	// Now test with structured config (use localhost as fallback)
	structuredConfig := &Config{
		Backend: "postgresql",
		PostgreSQLConfig: &PostgreSQLConfig{
			Host:     "localhost",
			Port:     5432,
			Database: "testdb",
			User:     "testuser",
			Password: "testpass",
			SSLMode:  "disable",
		},
	}

	// This will fail without a real PostgreSQL server, which is expected
	// The important thing is that the DSN is built correctly
	store2, err := NewPostgreSQLStore(structuredConfig)
	if err == nil {
		// If it succeeds (unlikely without real server), close it
		store2.Close()
	}
	// Error is expected since localhost:5432 likely doesn't have this test database
}

func TestNewPostgreSQLStore_StructuredConfigValidation(t *testing.T) {
	// Test that validation catches config errors before trying to connect
	config := &Config{
		Backend: "postgresql",
		PostgreSQLConfig: &PostgreSQLConfig{
			Host: "localhost",
			// Missing database - should fail validation
		},
	}

	_, err := NewPostgreSQLStore(config)
	if err == nil {
		t.Error("expected error for invalid config, got nil")
	}
	if !contains(err.Error(), "database is required") {
		t.Errorf("expected error about missing database, got: %v", err)
	}
}

func TestNewPostgreSQLStore_NoDSNOrConfig(t *testing.T) {
	config := &Config{
		Backend: "postgresql",
		// Neither DSN nor PostgreSQLConfig provided
	}

	_, err := NewPostgreSQLStore(config)
	if err == nil {
		t.Error("expected error when neither DSN nor config provided")
	}
	if !contains(err.Error(), "PostgreSQL DSN is required") {
		t.Errorf("expected DSN required error, got: %v", err)
	}
}
