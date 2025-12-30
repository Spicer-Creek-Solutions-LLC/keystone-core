package state

import (
	"context"
	"fmt"
	"time"
)

// MigrationStats tracks migration progress and results
type MigrationStats struct {
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration

	// Counts
	AgentsMigrated           int
	CommandsMigrated         int
	BatchJobsMigrated        int
	BatchAgentResultsMigrated int

	// Errors
	Errors []MigrationError
}

// MigrationError represents an error during migration
type MigrationError struct {
	Table   string
	ID      string
	Message string
}

// MigrationOptions configures the migration process
type MigrationOptions struct {
	// DryRun if true, validates migration without writing to target
	DryRun bool

	// BatchSize controls how many records to migrate at once
	BatchSize int

	// ProgressCallback is called periodically with progress updates
	ProgressCallback func(table string, current, total int)

	// ContinueOnError if true, continues migration even if some records fail
	ContinueOnError bool

	// SkipExisting if true, skips records that already exist in target
	SkipExisting bool
}

// DefaultMigrationOptions returns sensible defaults for migration
func DefaultMigrationOptions() *MigrationOptions {
	return &MigrationOptions{
		DryRun:          false,
		BatchSize:       100,
		ContinueOnError: false,
		SkipExisting:    true,
	}
}

// Migrator handles data migration between storage backends
type Migrator struct {
	source Store
	target Store
	opts   *MigrationOptions
}

// NewMigrator creates a new migrator
func NewMigrator(source, target Store, opts *MigrationOptions) *Migrator {
	if opts == nil {
		opts = DefaultMigrationOptions()
	}
	return &Migrator{
		source: source,
		target: target,
		opts:   opts,
	}
}

// Migrate performs the full migration from source to target
func (m *Migrator) Migrate(ctx context.Context) (*MigrationStats, error) {
	stats := &MigrationStats{
		StartTime: time.Now(),
	}

	// Verify connections
	if err := m.source.Ping(ctx); err != nil {
		return nil, fmt.Errorf("source database ping failed: %w", err)
	}
	if err := m.target.Ping(ctx); err != nil {
		return nil, fmt.Errorf("target database ping failed: %w", err)
	}

	// Migration order matters due to foreign keys:
	// 1. Agents (no dependencies)
	// 2. Commands (depends on agents)
	// 3. Batch jobs (no dependencies)
	// 4. Batch agent results (depends on agents and batch jobs)

	// Migrate agents
	if err := m.migrateAgents(ctx, stats); err != nil {
		if !m.opts.ContinueOnError {
			return stats, fmt.Errorf("agent migration failed: %w", err)
		}
	}

	// Migrate commands
	if err := m.migrateCommands(ctx, stats); err != nil {
		if !m.opts.ContinueOnError {
			return stats, fmt.Errorf("command migration failed: %w", err)
		}
	}

	// Migrate batch jobs
	if err := m.migrateBatchJobs(ctx, stats); err != nil {
		if !m.opts.ContinueOnError {
			return stats, fmt.Errorf("batch job migration failed: %w", err)
		}
	}

	// Migrate batch agent results
	if err := m.migrateBatchAgentResults(ctx, stats); err != nil {
		if !m.opts.ContinueOnError {
			return stats, fmt.Errorf("batch agent results migration failed: %w", err)
		}
	}

	stats.EndTime = time.Now()
	stats.Duration = stats.EndTime.Sub(stats.StartTime)

	return stats, nil
}

// migrateAgents migrates all agent records
func (m *Migrator) migrateAgents(ctx context.Context, stats *MigrationStats) error {
	// Get all agents from source
	agents, err := m.source.ListAgents(ctx, &AgentFilter{
		Limit: 0, // No limit - get all
	})
	if err != nil {
		return fmt.Errorf("failed to list agents: %w", err)
	}

	total := len(agents)
	if m.opts.ProgressCallback != nil {
		m.opts.ProgressCallback("agents", 0, total)
	}

	for i, agent := range agents {
		if m.opts.DryRun {
			stats.AgentsMigrated++
			continue
		}

		// Check if agent already exists in target
		if m.opts.SkipExisting {
			existing, err := m.target.GetAgent(ctx, agent.ID)
			if err == nil && existing != nil {
				// Agent already exists, skip
				continue
			}
		}

		if err := m.target.SaveAgent(ctx, agent); err != nil {
			migErr := MigrationError{
				Table:   "agents",
				ID:      agent.ID,
				Message: err.Error(),
			}
			stats.Errors = append(stats.Errors, migErr)

			if !m.opts.ContinueOnError {
				return fmt.Errorf("failed to migrate agent %s: %w", agent.ID, err)
			}
			continue
		}

		stats.AgentsMigrated++

		if m.opts.ProgressCallback != nil && (i+1)%m.opts.BatchSize == 0 {
			m.opts.ProgressCallback("agents", i+1, total)
		}
	}

	if m.opts.ProgressCallback != nil {
		m.opts.ProgressCallback("agents", total, total)
	}

	return nil
}

// migrateCommands migrates all command records
func (m *Migrator) migrateCommands(ctx context.Context, stats *MigrationStats) error {
	// Get all commands from source
	commands, err := m.source.ListCommands(ctx, &CommandFilter{
		Limit: 0, // No limit - get all
	})
	if err != nil {
		return fmt.Errorf("failed to list commands: %w", err)
	}

	total := len(commands)
	if m.opts.ProgressCallback != nil {
		m.opts.ProgressCallback("commands", 0, total)
	}

	for i, cmd := range commands {
		if m.opts.DryRun {
			stats.CommandsMigrated++
			continue
		}

		// Check if command already exists in target
		if m.opts.SkipExisting {
			existing, err := m.target.GetCommand(ctx, cmd.ID)
			if err == nil && existing != nil {
				// Command already exists, skip
				continue
			}
		}

		if err := m.target.SaveCommand(ctx, cmd); err != nil {
			migErr := MigrationError{
				Table:   "commands",
				ID:      cmd.ID,
				Message: err.Error(),
			}
			stats.Errors = append(stats.Errors, migErr)

			if !m.opts.ContinueOnError {
				return fmt.Errorf("failed to migrate command %s: %w", cmd.ID, err)
			}
			continue
		}

		stats.CommandsMigrated++

		if m.opts.ProgressCallback != nil && (i+1)%m.opts.BatchSize == 0 {
			m.opts.ProgressCallback("commands", i+1, total)
		}
	}

	if m.opts.ProgressCallback != nil {
		m.opts.ProgressCallback("commands", total, total)
	}

	return nil
}

// migrateBatchJobs migrates all batch job records (without agent results)
func (m *Migrator) migrateBatchJobs(ctx context.Context, stats *MigrationStats) error {
	// Get all batch jobs from source
	jobs, err := m.source.ListBatchJobs(ctx, &BatchJobFilter{
		Limit: 0, // No limit - get all
	})
	if err != nil {
		return fmt.Errorf("failed to list batch jobs: %w", err)
	}

	total := len(jobs)
	if m.opts.ProgressCallback != nil {
		m.opts.ProgressCallback("batch_jobs", 0, total)
	}

	for i, job := range jobs {
		if m.opts.DryRun {
			stats.BatchJobsMigrated++
			continue
		}

		// Check if batch job already exists in target
		if m.opts.SkipExisting {
			existing, err := m.target.GetBatchJob(ctx, job.ID)
			if err == nil && existing != nil {
				// Batch job already exists, skip
				continue
			}
		}

		// Save batch job without agent results (they're migrated separately)
		jobCopy := *job
		jobCopy.AgentResults = nil

		if err := m.target.SaveBatchJob(ctx, &jobCopy); err != nil {
			migErr := MigrationError{
				Table:   "batch_jobs",
				ID:      job.ID,
				Message: err.Error(),
			}
			stats.Errors = append(stats.Errors, migErr)

			if !m.opts.ContinueOnError {
				return fmt.Errorf("failed to migrate batch job %s: %w", job.ID, err)
			}
			continue
		}

		stats.BatchJobsMigrated++

		if m.opts.ProgressCallback != nil && (i+1)%m.opts.BatchSize == 0 {
			m.opts.ProgressCallback("batch_jobs", i+1, total)
		}
	}

	if m.opts.ProgressCallback != nil {
		m.opts.ProgressCallback("batch_jobs", total, total)
	}

	return nil
}

// migrateBatchAgentResults migrates all batch agent result records
func (m *Migrator) migrateBatchAgentResults(ctx context.Context, stats *MigrationStats) error {
	// Get all batch jobs to access their agent results
	jobs, err := m.source.ListBatchJobs(ctx, &BatchJobFilter{
		Limit: 0, // No limit - get all
	})
	if err != nil {
		return fmt.Errorf("failed to list batch jobs for results: %w", err)
	}

	// Count total results for progress
	var allResults []*BatchAgentResultRecord
	for _, job := range jobs {
		// Fetch full job with results
		fullJob, err := m.source.GetBatchJob(ctx, job.ID)
		if err != nil {
			continue
		}
		for _, result := range fullJob.AgentResults {
			allResults = append(allResults, result)
		}
	}

	total := len(allResults)
	if m.opts.ProgressCallback != nil {
		m.opts.ProgressCallback("batch_agent_results", 0, total)
	}

	for i, result := range allResults {
		if m.opts.DryRun {
			stats.BatchAgentResultsMigrated++
			continue
		}

		if err := m.target.SaveBatchAgentResult(ctx, result.BatchJobID, result); err != nil {
			migErr := MigrationError{
				Table:   "batch_agent_results",
				ID:      fmt.Sprintf("%s/%s", result.BatchJobID, result.AgentID),
				Message: err.Error(),
			}
			stats.Errors = append(stats.Errors, migErr)

			if !m.opts.ContinueOnError {
				return fmt.Errorf("failed to migrate batch agent result %s/%s: %w",
					result.BatchJobID, result.AgentID, err)
			}
			continue
		}

		stats.BatchAgentResultsMigrated++

		if m.opts.ProgressCallback != nil && (i+1)%m.opts.BatchSize == 0 {
			m.opts.ProgressCallback("batch_agent_results", i+1, total)
		}
	}

	if m.opts.ProgressCallback != nil {
		m.opts.ProgressCallback("batch_agent_results", total, total)
	}

	return nil
}

// ValidateMigration compares source and target to verify migration completeness
func (m *Migrator) ValidateMigration(ctx context.Context) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid: true,
	}

	// Compare agent counts
	sourceAgents, err := m.source.ListAgents(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list source agents: %w", err)
	}
	targetAgents, err := m.target.ListAgents(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list target agents: %w", err)
	}

	result.SourceAgentCount = len(sourceAgents)
	result.TargetAgentCount = len(targetAgents)
	if result.SourceAgentCount != result.TargetAgentCount {
		result.Valid = false
		result.Discrepancies = append(result.Discrepancies,
			fmt.Sprintf("agent count mismatch: source=%d, target=%d",
				result.SourceAgentCount, result.TargetAgentCount))
	}

	// Compare command counts
	sourceCommands, err := m.source.ListCommands(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list source commands: %w", err)
	}
	targetCommands, err := m.target.ListCommands(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list target commands: %w", err)
	}

	result.SourceCommandCount = len(sourceCommands)
	result.TargetCommandCount = len(targetCommands)
	if result.SourceCommandCount != result.TargetCommandCount {
		result.Valid = false
		result.Discrepancies = append(result.Discrepancies,
			fmt.Sprintf("command count mismatch: source=%d, target=%d",
				result.SourceCommandCount, result.TargetCommandCount))
	}

	// Compare batch job counts
	sourceBatchJobs, err := m.source.ListBatchJobs(ctx, &BatchJobFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list source batch jobs: %w", err)
	}
	targetBatchJobs, err := m.target.ListBatchJobs(ctx, &BatchJobFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list target batch jobs: %w", err)
	}

	result.SourceBatchJobCount = len(sourceBatchJobs)
	result.TargetBatchJobCount = len(targetBatchJobs)
	if result.SourceBatchJobCount != result.TargetBatchJobCount {
		result.Valid = false
		result.Discrepancies = append(result.Discrepancies,
			fmt.Sprintf("batch job count mismatch: source=%d, target=%d",
				result.SourceBatchJobCount, result.TargetBatchJobCount))
	}

	// Count batch agent results
	var sourceResultCount, targetResultCount int
	for _, job := range sourceBatchJobs {
		fullJob, err := m.source.GetBatchJob(ctx, job.ID)
		if err == nil {
			sourceResultCount += len(fullJob.AgentResults)
		}
	}
	for _, job := range targetBatchJobs {
		fullJob, err := m.target.GetBatchJob(ctx, job.ID)
		if err == nil {
			targetResultCount += len(fullJob.AgentResults)
		}
	}

	result.SourceBatchResultCount = sourceResultCount
	result.TargetBatchResultCount = targetResultCount
	if result.SourceBatchResultCount != result.TargetBatchResultCount {
		result.Valid = false
		result.Discrepancies = append(result.Discrepancies,
			fmt.Sprintf("batch result count mismatch: source=%d, target=%d",
				result.SourceBatchResultCount, result.TargetBatchResultCount))
	}

	return result, nil
}

// ValidationResult holds the results of migration validation
type ValidationResult struct {
	Valid bool

	SourceAgentCount       int
	TargetAgentCount       int
	SourceCommandCount     int
	TargetCommandCount     int
	SourceBatchJobCount    int
	TargetBatchJobCount    int
	SourceBatchResultCount int
	TargetBatchResultCount int

	Discrepancies []string
}

// MigrateFromSQLiteToPostgreSQL is a convenience function for common migration
func MigrateFromSQLiteToPostgreSQL(ctx context.Context, sqlitePath, postgresDSN string, opts *MigrationOptions) (*MigrationStats, error) {
	// Create source (SQLite) store
	sourceConfig := &Config{
		Backend:    "sqlite",
		SQLitePath: sqlitePath,
		SQLiteWAL:  true,
	}
	source, err := NewSQLiteStore(sourceConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}
	defer source.Close()

	// Create target (PostgreSQL) store
	targetConfig := &Config{
		Backend:       "postgresql",
		PostgreSQLDSN: postgresDSN,
	}
	target, err := NewPostgreSQLStore(targetConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	defer target.Close()

	// Create migrator and run
	migrator := NewMigrator(source, target, opts)
	return migrator.Migrate(ctx)
}

// ValidateSQLiteToPostgreSQLMigration validates migration between SQLite and PostgreSQL
func ValidateSQLiteToPostgreSQLMigration(ctx context.Context, sqlitePath, postgresDSN string) (*ValidationResult, error) {
	// Create source (SQLite) store
	sourceConfig := &Config{
		Backend:    "sqlite",
		SQLitePath: sqlitePath,
		SQLiteWAL:  true,
	}
	source, err := NewSQLiteStore(sourceConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}
	defer source.Close()

	// Create target (PostgreSQL) store
	targetConfig := &Config{
		Backend:       "postgresql",
		PostgreSQLDSN: postgresDSN,
	}
	target, err := NewPostgreSQLStore(targetConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	defer target.Close()

	// Create migrator and validate
	migrator := NewMigrator(source, target, nil)
	return migrator.ValidateMigration(ctx)
}
