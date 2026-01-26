package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// Helper to create a test SQLite store with sample data
func createSourceSQLiteStore(t *testing.T) (*SQLiteStore, string) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "source.db")

	config := &Config{
		Backend:    "sqlite",
		SQLitePath: dbPath,
		SQLiteWAL:  true,
	}

	store, err := NewSQLiteStore(config)
	if err != nil {
		t.Fatalf("Failed to create source SQLite store: %v", err)
	}

	return store, dbPath
}

// Helper to create a target SQLite store (for testing without PostgreSQL)
func createTargetSQLiteStore(t *testing.T) (*SQLiteStore, string) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "target.db")

	config := &Config{
		Backend:    "sqlite",
		SQLitePath: dbPath,
		SQLiteWAL:  true,
	}

	store, err := NewSQLiteStore(config)
	if err != nil {
		t.Fatalf("Failed to create target SQLite store: %v", err)
	}

	return store, dbPath
}

func TestDefaultMigrationOptions(t *testing.T) {
	opts := DefaultMigrationOptions()

	if opts.DryRun != false {
		t.Error("Expected DryRun to be false by default")
	}
	if opts.BatchSize != 100 {
		t.Errorf("Expected BatchSize to be 100, got %d", opts.BatchSize)
	}
	if opts.ContinueOnError != false {
		t.Error("Expected ContinueOnError to be false by default")
	}
	if opts.SkipExisting != true {
		t.Error("Expected SkipExisting to be true by default")
	}
}

func TestNewMigrator(t *testing.T) {
	source, _ := createSourceSQLiteStore(t)
	defer source.Close()

	target, _ := createTargetSQLiteStore(t)
	defer target.Close()

	// Test with nil options (should use defaults)
	m := NewMigrator(source, target, nil)
	if m.opts.BatchSize != 100 {
		t.Error("Expected default options when nil passed")
	}

	// Test with custom options
	customOpts := &MigrationOptions{
		BatchSize: 50,
		DryRun:    true,
	}
	m = NewMigrator(source, target, customOpts)
	if m.opts.BatchSize != 50 {
		t.Errorf("Expected BatchSize 50, got %d", m.opts.BatchSize)
	}
	if m.opts.DryRun != true {
		t.Error("Expected DryRun to be true")
	}
}

func TestMigrator_MigrateAgents(t *testing.T) {
	ctx := context.Background()

	source, _ := createSourceSQLiteStore(t)
	defer source.Close()

	target, _ := createTargetSQLiteStore(t)
	defer target.Close()

	now := time.Now()

	// Add test agents to source
	agents := []*AgentRecord{
		{
			ID:            "agent-1",
			Hostname:      "host1.example.com",
			OS:            "linux",
			Architecture:  "amd64",
			IPAddresses:   []string{"10.0.0.1", "192.168.1.1"},
			Labels:        map[string]string{"env": "prod", "role": "web"},
			Status:        pb.AgentStatus_AGENT_STATUS_ONLINE,
			LastHeartbeat: now,
			RegisteredAt:  now.Add(-24 * time.Hour),
			AgentVersion:  "1.0.0",
		},
		{
			ID:            "agent-2",
			Hostname:      "host2.example.com",
			OS:            "linux",
			Architecture:  "arm64",
			IPAddresses:   []string{"10.0.0.2"},
			Labels:        map[string]string{"env": "staging"},
			Status:        pb.AgentStatus_AGENT_STATUS_OFFLINE,
			LastHeartbeat: now.Add(-1 * time.Hour),
			RegisteredAt:  now.Add(-48 * time.Hour),
			AgentVersion:  "1.0.1",
		},
	}

	for _, agent := range agents {
		if err := source.SaveAgent(ctx, agent); err != nil {
			t.Fatalf("Failed to save agent: %v", err)
		}
	}

	// Migrate
	m := NewMigrator(source, target, nil)
	stats, err := m.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify stats
	if stats.AgentsMigrated != 2 {
		t.Errorf("Expected 2 agents migrated, got %d", stats.AgentsMigrated)
	}
	if len(stats.Errors) != 0 {
		t.Errorf("Expected no errors, got %d", len(stats.Errors))
	}

	// Verify data in target
	targetAgents, err := target.ListAgents(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list target agents: %v", err)
	}
	if len(targetAgents) != 2 {
		t.Errorf("Expected 2 agents in target, got %d", len(targetAgents))
	}

	// Verify specific agent data
	agent1, err := target.GetAgent(ctx, "agent-1")
	if err != nil {
		t.Fatalf("Failed to get agent-1: %v", err)
	}
	if agent1.Hostname != "host1.example.com" {
		t.Errorf("Expected hostname 'host1.example.com', got '%s'", agent1.Hostname)
	}
	if len(agent1.IPAddresses) != 2 {
		t.Errorf("Expected 2 IP addresses, got %d", len(agent1.IPAddresses))
	}
	if agent1.Labels["env"] != "prod" {
		t.Errorf("Expected label env=prod, got %s", agent1.Labels["env"])
	}
}

func TestMigrator_MigrateCommands(t *testing.T) {
	ctx := context.Background()

	source, _ := createSourceSQLiteStore(t)
	defer source.Close()

	target, _ := createTargetSQLiteStore(t)
	defer target.Close()

	now := time.Now()
	startedAt := now.Add(-1 * time.Hour)
	completedAt := now.Add(-1*time.Hour + 150*time.Millisecond)

	// First create agent (foreign key dependency)
	agent := &AgentRecord{
		ID:       "agent-1",
		Hostname: "host1.example.com",
		Status:   pb.AgentStatus_AGENT_STATUS_ONLINE,
	}
	if err := source.SaveAgent(ctx, agent); err != nil {
		t.Fatalf("Failed to save agent: %v", err)
	}

	// Add test commands
	commands := []*CommandRecord{
		{
			ID:          "cmd-1",
			AgentID:     "agent-1",
			Command:     "echo hello",
			Args:        []string{"-n"},
			Env:         map[string]string{"FOO": "bar"},
			WorkingDir:  "/tmp",
			User:        "root",
			Status:      pb.CommandStatus_COMMAND_STATUS_COMPLETED,
			ExitCode:    0,
			Stdout:      "hello",
			DurationMs:  150,
			CreatedAt:   now.Add(-1 * time.Hour),
			StartedAt:   &startedAt,
			CompletedAt: &completedAt,
		},
		{
			ID:         "cmd-2",
			AgentID:    "agent-1",
			Command:    "ls -la",
			Status:     pb.CommandStatus_COMMAND_STATUS_FAILED,
			ExitCode:   1,
			Error:      "permission denied",
			DurationMs: 50,
			CreatedAt:  now,
		},
	}

	for _, cmd := range commands {
		if err := source.SaveCommand(ctx, cmd); err != nil {
			t.Fatalf("Failed to save command: %v", err)
		}
	}

	// Migrate
	m := NewMigrator(source, target, nil)
	stats, err := m.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify stats
	if stats.CommandsMigrated != 2 {
		t.Errorf("Expected 2 commands migrated, got %d", stats.CommandsMigrated)
	}

	// Verify data in target
	targetCmds, err := target.ListCommands(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list target commands: %v", err)
	}
	if len(targetCmds) != 2 {
		t.Errorf("Expected 2 commands in target, got %d", len(targetCmds))
	}

	// Verify specific command data
	cmd1, err := target.GetCommand(ctx, "cmd-1")
	if err != nil {
		t.Fatalf("Failed to get cmd-1: %v", err)
	}
	if cmd1.Command != "echo hello" {
		t.Errorf("Expected command 'echo hello', got '%s'", cmd1.Command)
	}
	if len(cmd1.Args) != 1 || cmd1.Args[0] != "-n" {
		t.Errorf("Expected args ['-n'], got %v", cmd1.Args)
	}
	if cmd1.Env["FOO"] != "bar" {
		t.Errorf("Expected env FOO=bar, got %s", cmd1.Env["FOO"])
	}
}

func TestMigrator_MigrateBatchJobs(t *testing.T) {
	ctx := context.Background()

	source, _ := createSourceSQLiteStore(t)
	defer source.Close()

	target, _ := createTargetSQLiteStore(t)
	defer target.Close()

	now := time.Now()
	startedAt := now.Add(-2 * time.Hour)
	completedAt := now.Add(-2*time.Hour + 30*time.Second)

	// Add test batch jobs
	jobs := []*BatchJobRecord{
		{
			ID:               "job-1",
			Target:           "role=web",
			Command:          "systemctl restart nginx",
			Status:           pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED,
			TotalAgents:      5,
			CompletedAgents:  5,
			SuccessfulAgents: 5,
			FailedAgents:     0,
			CreatedAt:        now.Add(-2 * time.Hour),
			StartedAt:        &startedAt,
			CompletedAt:      &completedAt,
		},
		{
			ID:               "job-2",
			Target:           "env=prod",
			Command:          "cp /tmp/config /etc/app/config",
			Status:           pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING,
			TotalAgents:      10,
			CompletedAgents:  3,
			SuccessfulAgents: 2,
			FailedAgents:     1,
			CreatedAt:        now,
			StartedAt:        &now,
		},
	}

	for _, job := range jobs {
		if err := source.SaveBatchJob(ctx, job); err != nil {
			t.Fatalf("Failed to save batch job: %v", err)
		}
	}

	// Migrate
	m := NewMigrator(source, target, nil)
	stats, err := m.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify stats
	if stats.BatchJobsMigrated != 2 {
		t.Errorf("Expected 2 batch jobs migrated, got %d", stats.BatchJobsMigrated)
	}

	// Verify data in target
	targetJobs, err := target.ListBatchJobs(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list target batch jobs: %v", err)
	}
	if len(targetJobs) != 2 {
		t.Errorf("Expected 2 batch jobs in target, got %d", len(targetJobs))
	}
}

func TestMigrator_MigrateBatchAgentResults(t *testing.T) {
	ctx := context.Background()

	source, _ := createSourceSQLiteStore(t)
	defer source.Close()

	target, _ := createTargetSQLiteStore(t)
	defer target.Close()

	now := time.Now()

	// Create agent and batch job first (foreign key dependencies)
	agent := &AgentRecord{
		ID:       "agent-1",
		Hostname: "host1.example.com",
		Status:   pb.AgentStatus_AGENT_STATUS_ONLINE,
	}
	if err := source.SaveAgent(ctx, agent); err != nil {
		t.Fatalf("Failed to save agent: %v", err)
	}

	job := &BatchJobRecord{
		ID:               "job-1",
		Target:           "*",
		Command:          "echo test",
		Status:           pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED,
		TotalAgents:      1,
		CompletedAgents:  1,
		SuccessfulAgents: 1,
		FailedAgents:     0,
		CreatedAt:        now,
	}
	if err := source.SaveBatchJob(ctx, job); err != nil {
		t.Fatalf("Failed to save batch job: %v", err)
	}

	// Save agent result separately (SaveBatchJob doesn't save results)
	agentResult := &BatchAgentResultRecord{
		BatchJobID: "job-1",
		AgentID:    "agent-1",
		Success:    true,
		ExitCode:   0,
		DurationMs: 100,
		CreatedAt:  now,
	}
	if err := source.SaveBatchAgentResult(ctx, "job-1", agentResult); err != nil {
		t.Fatalf("Failed to save batch agent result: %v", err)
	}

	// Migrate
	m := NewMigrator(source, target, nil)
	stats, err := m.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify stats
	if stats.BatchAgentResultsMigrated != 1 {
		t.Errorf("Expected 1 batch agent result migrated, got %d", stats.BatchAgentResultsMigrated)
	}

	// Verify data in target
	targetJob, err := target.GetBatchJob(ctx, "job-1")
	if err != nil {
		t.Fatalf("Failed to get target batch job: %v", err)
	}
	if len(targetJob.AgentResults) != 1 {
		t.Errorf("Expected 1 agent result, got %d", len(targetJob.AgentResults))
	}
	if targetJob.AgentResults[0].Success != true {
		t.Errorf("Expected success=true, got %v", targetJob.AgentResults[0].Success)
	}
}

func TestMigrator_DryRun(t *testing.T) {
	ctx := context.Background()

	source, _ := createSourceSQLiteStore(t)
	defer source.Close()

	target, _ := createTargetSQLiteStore(t)
	defer target.Close()

	// Add test agent
	agent := &AgentRecord{
		ID:       "agent-1",
		Hostname: "host1.example.com",
		Status:   pb.AgentStatus_AGENT_STATUS_ONLINE,
	}
	if err := source.SaveAgent(ctx, agent); err != nil {
		t.Fatalf("Failed to save agent: %v", err)
	}

	// Migrate with dry run
	opts := &MigrationOptions{
		DryRun: true,
	}
	m := NewMigrator(source, target, opts)
	stats, err := m.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Stats should show migration count
	if stats.AgentsMigrated != 1 {
		t.Errorf("Expected 1 agent in dry run stats, got %d", stats.AgentsMigrated)
	}

	// But target should be empty
	targetAgents, err := target.ListAgents(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list target agents: %v", err)
	}
	if len(targetAgents) != 0 {
		t.Errorf("Expected 0 agents in target after dry run, got %d", len(targetAgents))
	}
}

func TestMigrator_SkipExisting(t *testing.T) {
	ctx := context.Background()

	source, _ := createSourceSQLiteStore(t)
	defer source.Close()

	target, _ := createTargetSQLiteStore(t)
	defer target.Close()

	// Add same agent to both source and target
	agent := &AgentRecord{
		ID:       "agent-1",
		Hostname: "source-hostname",
		Status:   pb.AgentStatus_AGENT_STATUS_ONLINE,
	}
	if err := source.SaveAgent(ctx, agent); err != nil {
		t.Fatalf("Failed to save agent to source: %v", err)
	}

	targetAgent := &AgentRecord{
		ID:       "agent-1",
		Hostname: "target-hostname",
		Status:   pb.AgentStatus_AGENT_STATUS_OFFLINE,
	}
	if err := target.SaveAgent(ctx, targetAgent); err != nil {
		t.Fatalf("Failed to save agent to target: %v", err)
	}

	// Migrate with SkipExisting=true (default)
	m := NewMigrator(source, target, nil)
	stats, err := m.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Agent should not be counted as migrated since it existed
	if stats.AgentsMigrated != 0 {
		t.Errorf("Expected 0 agents migrated (skipped existing), got %d", stats.AgentsMigrated)
	}

	// Target should still have original data
	resultAgent, err := target.GetAgent(ctx, "agent-1")
	if err != nil {
		t.Fatalf("Failed to get agent from target: %v", err)
	}
	if resultAgent.Hostname != "target-hostname" {
		t.Errorf("Expected hostname 'target-hostname', got '%s'", resultAgent.Hostname)
	}
}

func TestMigrator_ProgressCallback(t *testing.T) {
	ctx := context.Background()

	source, _ := createSourceSQLiteStore(t)
	defer source.Close()

	target, _ := createTargetSQLiteStore(t)
	defer target.Close()

	// Add test agents
	for i := 0; i < 5; i++ {
		agent := &AgentRecord{
			ID:       "agent-" + string(rune('0'+i)),
			Hostname: "host" + string(rune('0'+i)) + ".example.com",
			Status:   pb.AgentStatus_AGENT_STATUS_ONLINE,
		}
		if err := source.SaveAgent(ctx, agent); err != nil {
			t.Fatalf("Failed to save agent: %v", err)
		}
	}

	// Track progress callbacks
	var progressCalls []struct {
		table   string
		current int
		total   int
	}

	opts := &MigrationOptions{
		BatchSize: 2,
		ProgressCallback: func(table string, current, total int) {
			progressCalls = append(progressCalls, struct {
				table   string
				current int
				total   int
			}{table, current, total})
		},
	}

	m := NewMigrator(source, target, opts)
	_, err := m.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Should have received progress callbacks
	if len(progressCalls) == 0 {
		t.Error("Expected progress callbacks")
	}

	// Should have start (0) and end (total) calls for agents
	foundAgentStart := false
	foundAgentEnd := false
	for _, call := range progressCalls {
		if call.table == "agents" && call.current == 0 {
			foundAgentStart = true
		}
		if call.table == "agents" && call.current == call.total && call.total > 0 {
			foundAgentEnd = true
		}
	}
	if !foundAgentStart {
		t.Error("Expected progress callback for agents start")
	}
	if !foundAgentEnd {
		t.Error("Expected progress callback for agents end")
	}
}

func TestMigrator_ValidateMigration(t *testing.T) {
	ctx := context.Background()

	source, _ := createSourceSQLiteStore(t)
	defer source.Close()

	target, _ := createTargetSQLiteStore(t)
	defer target.Close()

	now := time.Now()

	// Add test data to source
	agent := &AgentRecord{
		ID:       "agent-1",
		Hostname: "host1.example.com",
		Status:   pb.AgentStatus_AGENT_STATUS_ONLINE,
	}
	if err := source.SaveAgent(ctx, agent); err != nil {
		t.Fatalf("Failed to save agent: %v", err)
	}

	cmd := &CommandRecord{
		ID:        "cmd-1",
		AgentID:   "agent-1",
		Command:   "echo test",
		Status:    pb.CommandStatus_COMMAND_STATUS_COMPLETED,
		CreatedAt: now,
	}
	if err := source.SaveCommand(ctx, cmd); err != nil {
		t.Fatalf("Failed to save command: %v", err)
	}

	// Validate before migration (should show discrepancies)
	m := NewMigrator(source, target, nil)
	result, err := m.ValidateMigration(ctx)
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	if result.Valid {
		t.Error("Expected validation to fail before migration")
	}
	if result.SourceAgentCount != 1 {
		t.Errorf("Expected source agent count 1, got %d", result.SourceAgentCount)
	}
	if result.TargetAgentCount != 0 {
		t.Errorf("Expected target agent count 0, got %d", result.TargetAgentCount)
	}

	// Run migration
	_, err = m.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Validate after migration (should pass)
	result, err = m.ValidateMigration(ctx)
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	if !result.Valid {
		t.Errorf("Expected validation to pass after migration, discrepancies: %v", result.Discrepancies)
	}
	if result.SourceAgentCount != result.TargetAgentCount {
		t.Errorf("Agent counts don't match: source=%d, target=%d",
			result.SourceAgentCount, result.TargetAgentCount)
	}
	if result.SourceCommandCount != result.TargetCommandCount {
		t.Errorf("Command counts don't match: source=%d, target=%d",
			result.SourceCommandCount, result.TargetCommandCount)
	}
}

func TestMigrator_Duration(t *testing.T) {
	ctx := context.Background()

	source, _ := createSourceSQLiteStore(t)
	defer source.Close()

	target, _ := createTargetSQLiteStore(t)
	defer target.Close()

	m := NewMigrator(source, target, nil)
	stats, err := m.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify timing fields are set
	if stats.StartTime.IsZero() {
		t.Error("StartTime should not be zero")
	}
	if stats.EndTime.IsZero() {
		t.Error("EndTime should not be zero")
	}
	if stats.Duration <= 0 {
		t.Error("Duration should be positive")
	}
	if stats.EndTime.Before(stats.StartTime) {
		t.Error("EndTime should be after StartTime")
	}
}

// Integration test for PostgreSQL migration (requires KSCORE_TEST_POSTGRES_DSN)
func TestMigrator_SQLiteToPostgreSQL(t *testing.T) {
	postgresDSN := os.Getenv("KSCORE_TEST_POSTGRES_DSN")
	if postgresDSN == "" {
		t.Skip("Skipping PostgreSQL migration test: KSCORE_TEST_POSTGRES_DSN not set")
	}

	ctx := context.Background()
	now := time.Now()

	// Create SQLite source with data
	source, _ := createSourceSQLiteStore(t)
	defer source.Close()

	agent := &AgentRecord{
		ID:            "agent-1",
		Hostname:      "host1.example.com",
		OS:            "linux",
		Architecture:  "amd64",
		IPAddresses:   []string{"10.0.0.1"},
		Labels:        map[string]string{"env": "prod"},
		Status:        pb.AgentStatus_AGENT_STATUS_ONLINE,
		LastHeartbeat: now,
		RegisteredAt:  now.Add(-24 * time.Hour),
		AgentVersion:  "1.0.0",
	}
	if err := source.SaveAgent(ctx, agent); err != nil {
		t.Fatalf("Failed to save agent: %v", err)
	}

	// Create PostgreSQL target
	targetConfig := &Config{
		Backend:       "postgresql",
		PostgreSQLDSN: postgresDSN,
	}
	target, err := NewPostgreSQLStore(targetConfig)
	if err != nil {
		t.Fatalf("Failed to create PostgreSQL store: %v", err)
	}
	defer target.Close()

	// Clean up target first
	if _, err := target.db.ExecContext(ctx, "TRUNCATE TABLE batch_agent_results, commands, batch_jobs, agents CASCADE"); err != nil {
		t.Logf("Warning: failed to truncate tables: %v", err)
	}

	// Migrate
	m := NewMigrator(source, target, nil)
	stats, err := m.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	if stats.AgentsMigrated != 1 {
		t.Errorf("Expected 1 agent migrated, got %d", stats.AgentsMigrated)
	}

	// Verify in PostgreSQL
	resultAgent, err := target.GetAgent(ctx, "agent-1")
	if err != nil {
		t.Fatalf("Failed to get agent from PostgreSQL: %v", err)
	}
	if resultAgent.Hostname != "host1.example.com" {
		t.Errorf("Expected hostname 'host1.example.com', got '%s'", resultAgent.Hostname)
	}
	if resultAgent.Labels["env"] != "prod" {
		t.Errorf("Expected label env=prod, got %s", resultAgent.Labels["env"])
	}

	// Validate migration
	result, err := m.ValidateMigration(ctx)
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}
	if !result.Valid {
		t.Errorf("Expected valid migration, got discrepancies: %v", result.Discrepancies)
	}
}
