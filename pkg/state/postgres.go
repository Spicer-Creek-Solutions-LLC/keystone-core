package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// PostgreSQLStore implements Store using PostgreSQL
type PostgreSQLStore struct {
	db  *sql.DB
	dsn string
}

// NewPostgreSQLStore creates a new PostgreSQL store
func NewPostgreSQLStore(config *Config) (*PostgreSQLStore, error) {
	dsn := config.PostgreSQLDSN
	if dsn == "" {
		return nil, fmt.Errorf("PostgreSQL DSN is required")
	}

	// Open database
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	if config.PostgreSQLMaxOpen > 0 {
		db.SetMaxOpenConns(config.PostgreSQLMaxOpen)
	} else {
		db.SetMaxOpenConns(25) // Default
	}

	if config.PostgreSQLMaxIdle > 0 {
		db.SetMaxIdleConns(config.PostgreSQLMaxIdle)
	} else {
		db.SetMaxIdleConns(5) // Default
	}

	if config.PostgreSQLConnMaxLife > 0 {
		db.SetConnMaxLifetime(config.PostgreSQLConnMaxLife)
	} else {
		db.SetConnMaxLifetime(5 * time.Minute) // Default
	}

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	store := &PostgreSQLStore{
		db:  db,
		dsn: dsn,
	}

	// Initialize schema
	if err := store.initSchema(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates the database schema
func (s *PostgreSQLStore) initSchema(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS agents (
		id TEXT PRIMARY KEY,
		hostname TEXT NOT NULL,
		os TEXT NOT NULL,
		architecture TEXT NOT NULL,
		ip_addresses JSONB,
		platform_version TEXT,
		agent_version TEXT,
		labels JSONB,
		status INTEGER NOT NULL,
		last_heartbeat TIMESTAMP WITH TIME ZONE NOT NULL,
		registered_at TIMESTAMP WITH TIME ZONE NOT NULL,
		updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
		cpu_percent REAL,
		memory_percent REAL,
		disk_percent REAL,
		load_average JSONB
	);

	CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
	CREATE INDEX IF NOT EXISTS idx_agents_last_heartbeat ON agents(last_heartbeat);
	CREATE INDEX IF NOT EXISTS idx_agents_labels ON agents USING GIN(labels);

	CREATE TABLE IF NOT EXISTS commands (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
		command TEXT NOT NULL,
		args JSONB,
		env JSONB,
		working_dir TEXT,
		"user" TEXT,
		timeout INTEGER,
		status INTEGER NOT NULL,
		exit_code INTEGER,
		stdout TEXT,
		stderr TEXT,
		error TEXT,
		created_at TIMESTAMP WITH TIME ZONE NOT NULL,
		started_at TIMESTAMP WITH TIME ZONE,
		completed_at TIMESTAMP WITH TIME ZONE,
		duration_ms BIGINT
	);

	CREATE INDEX IF NOT EXISTS idx_commands_agent_id ON commands(agent_id);
	CREATE INDEX IF NOT EXISTS idx_commands_status ON commands(status);
	CREATE INDEX IF NOT EXISTS idx_commands_created_at ON commands(created_at);

	CREATE TABLE IF NOT EXISTS batch_jobs (
		id TEXT PRIMARY KEY,
		target TEXT NOT NULL,
		command TEXT NOT NULL,
		args JSONB,
		env JSONB,
		working_dir TEXT,
		"user" TEXT,
		timeout INTEGER,
		concurrency INTEGER,
		status INTEGER NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE NOT NULL,
		started_at TIMESTAMP WITH TIME ZONE,
		completed_at TIMESTAMP WITH TIME ZONE,
		duration_ms BIGINT,
		total_agents INTEGER DEFAULT 0,
		completed_agents INTEGER DEFAULT 0,
		successful_agents INTEGER DEFAULT 0,
		failed_agents INTEGER DEFAULT 0,
		success_rate REAL DEFAULT 0.0
	);

	CREATE INDEX IF NOT EXISTS idx_batch_jobs_status ON batch_jobs(status);
	CREATE INDEX IF NOT EXISTS idx_batch_jobs_created_at ON batch_jobs(created_at);
	CREATE INDEX IF NOT EXISTS idx_batch_jobs_target ON batch_jobs(target);

	CREATE TABLE IF NOT EXISTS batch_agent_results (
		batch_job_id TEXT NOT NULL REFERENCES batch_jobs(id) ON DELETE CASCADE,
		agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
		success BOOLEAN NOT NULL,
		exit_code INTEGER,
		error TEXT,
		duration_ms BIGINT,
		created_at TIMESTAMP WITH TIME ZONE NOT NULL,
		PRIMARY KEY (batch_job_id, agent_id)
	);

	CREATE INDEX IF NOT EXISTS idx_batch_agent_results_batch_job_id ON batch_agent_results(batch_job_id);
	CREATE INDEX IF NOT EXISTS idx_batch_agent_results_agent_id ON batch_agent_results(agent_id);
	`

	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// SaveAgent saves an agent record
func (s *PostgreSQLStore) SaveAgent(ctx context.Context, agent *AgentRecord) error {
	ipAddresses, _ := json.Marshal(agent.IPAddresses)
	labels, _ := json.Marshal(agent.Labels)
	loadAvg, _ := json.Marshal(agent.LoadAverage)

	query := `
		INSERT INTO agents (
			id, hostname, os, architecture, ip_addresses, platform_version,
			agent_version, labels, status, last_heartbeat, registered_at,
			updated_at, cpu_percent, memory_percent, disk_percent, load_average
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT(id) DO UPDATE SET
			hostname = EXCLUDED.hostname,
			os = EXCLUDED.os,
			architecture = EXCLUDED.architecture,
			ip_addresses = EXCLUDED.ip_addresses,
			platform_version = EXCLUDED.platform_version,
			agent_version = EXCLUDED.agent_version,
			labels = EXCLUDED.labels,
			status = EXCLUDED.status,
			last_heartbeat = EXCLUDED.last_heartbeat,
			updated_at = EXCLUDED.updated_at,
			cpu_percent = EXCLUDED.cpu_percent,
			memory_percent = EXCLUDED.memory_percent,
			disk_percent = EXCLUDED.disk_percent,
			load_average = EXCLUDED.load_average
	`

	_, err := s.db.ExecContext(ctx, query,
		agent.ID, agent.Hostname, agent.OS, agent.Architecture, ipAddresses,
		agent.PlatformVersion, agent.AgentVersion, labels, agent.Status,
		agent.LastHeartbeat, agent.RegisteredAt, agent.UpdatedAt,
		agent.CPUPercent, agent.MemoryPercent, agent.DiskPercent, loadAvg,
	)

	return err
}

// GetAgent retrieves an agent by ID
func (s *PostgreSQLStore) GetAgent(ctx context.Context, agentID string) (*AgentRecord, error) {
	query := `
		SELECT id, hostname, os, architecture, ip_addresses, platform_version,
			agent_version, labels, status, last_heartbeat, registered_at, updated_at,
			cpu_percent, memory_percent, disk_percent, load_average
		FROM agents
		WHERE id = $1
	`

	var agent AgentRecord
	var ipAddresses, labels, loadAvg []byte

	err := s.db.QueryRowContext(ctx, query, agentID).Scan(
		&agent.ID, &agent.Hostname, &agent.OS, &agent.Architecture, &ipAddresses,
		&agent.PlatformVersion, &agent.AgentVersion, &labels, &agent.Status,
		&agent.LastHeartbeat, &agent.RegisteredAt, &agent.UpdatedAt,
		&agent.CPUPercent, &agent.MemoryPercent, &agent.DiskPercent, &loadAvg,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}
	if err != nil {
		return nil, err
	}

	// Unmarshal JSON fields
	json.Unmarshal(ipAddresses, &agent.IPAddresses)
	json.Unmarshal(labels, &agent.Labels)
	json.Unmarshal(loadAvg, &agent.LoadAverage)

	return &agent, nil
}

// ListAgents lists agents with filtering
func (s *PostgreSQLStore) ListAgents(ctx context.Context, filter *AgentFilter) ([]*AgentRecord, error) {
	query := `
		SELECT id, hostname, os, architecture, ip_addresses, platform_version,
			agent_version, labels, status, last_heartbeat, registered_at, updated_at,
			cpu_percent, memory_percent, disk_percent, load_average
		FROM agents
		WHERE 1=1
	`
	args := []interface{}{}
	paramCount := 0

	if filter != nil {
		if filter.Status != nil {
			paramCount++
			query += fmt.Sprintf(" AND status = $%d", paramCount)
			args = append(args, *filter.Status)
		}

		// Add sorting
		sortBy := "registered_at"
		if filter.SortBy != "" {
			// Validate sort column to prevent SQL injection
			validColumns := map[string]bool{
				"id": true, "hostname": true, "os": true, "status": true,
				"last_heartbeat": true, "registered_at": true, "updated_at": true,
			}
			if validColumns[filter.SortBy] {
				sortBy = filter.SortBy
			}
		}
		sortOrder := "DESC"
		if filter.SortOrder != "" && strings.ToUpper(filter.SortOrder) == "ASC" {
			sortOrder = "ASC"
		}
		query += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)

		// Add pagination
		if filter.Limit > 0 {
			paramCount++
			query += fmt.Sprintf(" LIMIT $%d", paramCount)
			args = append(args, filter.Limit)
		}
		if filter.Offset > 0 {
			paramCount++
			query += fmt.Sprintf(" OFFSET $%d", paramCount)
			args = append(args, filter.Offset)
		}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []*AgentRecord
	for rows.Next() {
		var agent AgentRecord
		var ipAddresses, labels, loadAvg []byte

		err := rows.Scan(
			&agent.ID, &agent.Hostname, &agent.OS, &agent.Architecture, &ipAddresses,
			&agent.PlatformVersion, &agent.AgentVersion, &labels, &agent.Status,
			&agent.LastHeartbeat, &agent.RegisteredAt, &agent.UpdatedAt,
			&agent.CPUPercent, &agent.MemoryPercent, &agent.DiskPercent, &loadAvg,
		)
		if err != nil {
			return nil, err
		}

		json.Unmarshal(ipAddresses, &agent.IPAddresses)
		json.Unmarshal(labels, &agent.Labels)
		json.Unmarshal(loadAvg, &agent.LoadAverage)

		agents = append(agents, &agent)
	}

	return agents, rows.Err()
}

// UpdateAgentStatus updates an agent's status
func (s *PostgreSQLStore) UpdateAgentStatus(ctx context.Context, agentID string, status pb.AgentStatus, lastHeartbeat time.Time) error {
	query := `UPDATE agents SET status = $1, last_heartbeat = $2, updated_at = $3 WHERE id = $4`
	_, err := s.db.ExecContext(ctx, query, status, lastHeartbeat, time.Now(), agentID)
	return err
}

// UpdateAgentMetrics updates an agent's metrics
func (s *PostgreSQLStore) UpdateAgentMetrics(ctx context.Context, agentID string, metrics *pb.SystemMetrics) error {
	loadAvg, _ := json.Marshal(metrics.LoadAverage)
	query := `
		UPDATE agents
		SET cpu_percent = $1, memory_percent = $2, disk_percent = $3, load_average = $4, updated_at = $5
		WHERE id = $6
	`
	_, err := s.db.ExecContext(ctx, query,
		metrics.CpuPercent, metrics.MemoryPercent, metrics.DiskPercent, loadAvg, time.Now(), agentID,
	)
	return err
}

// DeleteAgent deletes an agent
func (s *PostgreSQLStore) DeleteAgent(ctx context.Context, agentID string) error {
	query := `DELETE FROM agents WHERE id = $1`
	_, err := s.db.ExecContext(ctx, query, agentID)
	return err
}

// SaveCommand saves a command record
func (s *PostgreSQLStore) SaveCommand(ctx context.Context, cmd *CommandRecord) error {
	args, _ := json.Marshal(cmd.Args)
	env, _ := json.Marshal(cmd.Env)

	query := `
		INSERT INTO commands (
			id, agent_id, command, args, env, working_dir, "user", timeout,
			status, exit_code, stdout, stderr, error, created_at, started_at,
			completed_at, duration_ms
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`

	_, err := s.db.ExecContext(ctx, query,
		cmd.ID, cmd.AgentID, cmd.Command, args, env, cmd.WorkingDir, cmd.User,
		cmd.Timeout, cmd.Status, cmd.ExitCode, cmd.Stdout, cmd.Stderr, cmd.Error,
		cmd.CreatedAt, cmd.StartedAt, cmd.CompletedAt, cmd.DurationMs,
	)

	return err
}

// GetCommand retrieves a command by ID
func (s *PostgreSQLStore) GetCommand(ctx context.Context, commandID string) (*CommandRecord, error) {
	query := `
		SELECT id, agent_id, command, args, env, working_dir, "user", timeout,
			status, exit_code, stdout, stderr, error, created_at, started_at,
			completed_at, duration_ms
		FROM commands
		WHERE id = $1
	`

	var cmd CommandRecord
	var args, env []byte

	err := s.db.QueryRowContext(ctx, query, commandID).Scan(
		&cmd.ID, &cmd.AgentID, &cmd.Command, &args, &env, &cmd.WorkingDir,
		&cmd.User, &cmd.Timeout, &cmd.Status, &cmd.ExitCode, &cmd.Stdout,
		&cmd.Stderr, &cmd.Error, &cmd.CreatedAt, &cmd.StartedAt,
		&cmd.CompletedAt, &cmd.DurationMs,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("command not found: %s", commandID)
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal(args, &cmd.Args)
	json.Unmarshal(env, &cmd.Env)

	return &cmd, nil
}

// ListCommands lists commands with filtering
func (s *PostgreSQLStore) ListCommands(ctx context.Context, filter *CommandFilter) ([]*CommandRecord, error) {
	query := `
		SELECT id, agent_id, command, args, env, working_dir, "user", timeout,
			status, exit_code, stdout, stderr, error, created_at, started_at,
			completed_at, duration_ms
		FROM commands
		WHERE 1=1
	`
	args := []interface{}{}
	paramCount := 0

	if filter != nil {
		if filter.AgentID != "" {
			paramCount++
			query += fmt.Sprintf(" AND agent_id = $%d", paramCount)
			args = append(args, filter.AgentID)
		}
		if filter.Status != nil {
			paramCount++
			query += fmt.Sprintf(" AND status = $%d", paramCount)
			args = append(args, *filter.Status)
		}
		if filter.StartTime != nil {
			paramCount++
			query += fmt.Sprintf(" AND created_at >= $%d", paramCount)
			args = append(args, *filter.StartTime)
		}
		if filter.EndTime != nil {
			paramCount++
			query += fmt.Sprintf(" AND created_at <= $%d", paramCount)
			args = append(args, *filter.EndTime)
		}

		// Add sorting
		sortBy := "created_at"
		if filter.SortBy != "" {
			validColumns := map[string]bool{
				"id": true, "agent_id": true, "command": true, "status": true,
				"created_at": true, "started_at": true, "completed_at": true,
			}
			if validColumns[filter.SortBy] {
				sortBy = filter.SortBy
			}
		}
		sortOrder := "DESC"
		if filter.SortOrder != "" && strings.ToUpper(filter.SortOrder) == "ASC" {
			sortOrder = "ASC"
		}
		query += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)

		// Add pagination
		if filter.Limit > 0 {
			paramCount++
			query += fmt.Sprintf(" LIMIT $%d", paramCount)
			args = append(args, filter.Limit)
		}
		if filter.Offset > 0 {
			paramCount++
			query += fmt.Sprintf(" OFFSET $%d", paramCount)
			args = append(args, filter.Offset)
		}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var commands []*CommandRecord
	for rows.Next() {
		var cmd CommandRecord
		var cmdArgs, env []byte

		err := rows.Scan(
			&cmd.ID, &cmd.AgentID, &cmd.Command, &cmdArgs, &env, &cmd.WorkingDir,
			&cmd.User, &cmd.Timeout, &cmd.Status, &cmd.ExitCode, &cmd.Stdout,
			&cmd.Stderr, &cmd.Error, &cmd.CreatedAt, &cmd.StartedAt,
			&cmd.CompletedAt, &cmd.DurationMs,
		)
		if err != nil {
			return nil, err
		}

		json.Unmarshal(cmdArgs, &cmd.Args)
		json.Unmarshal(env, &cmd.Env)

		commands = append(commands, &cmd)
	}

	return commands, rows.Err()
}

// UpdateCommandStatus updates a command's status
func (s *PostgreSQLStore) UpdateCommandStatus(ctx context.Context, commandID string, status pb.CommandStatus) error {
	query := `UPDATE commands SET status = $1 WHERE id = $2`
	_, err := s.db.ExecContext(ctx, query, status, commandID)
	return err
}

// UpdateCommandResult updates command execution result
func (s *PostgreSQLStore) UpdateCommandResult(ctx context.Context, commandID string, result *CommandResult) error {
	query := `
		UPDATE commands
		SET status = $1, exit_code = $2, stdout = $3, stderr = $4, error = $5,
			started_at = $6, completed_at = $7, duration_ms = $8
		WHERE id = $9
	`
	_, err := s.db.ExecContext(ctx, query,
		result.Status, result.ExitCode, result.Stdout, result.Stderr, result.Error,
		result.StartedAt, result.CompletedAt, result.DurationMs, commandID,
	)
	return err
}

// SaveBatchJob saves a batch job record
func (s *PostgreSQLStore) SaveBatchJob(ctx context.Context, job *BatchJobRecord) error {
	args, _ := json.Marshal(job.Args)
	env, _ := json.Marshal(job.Env)

	query := `
		INSERT INTO batch_jobs (
			id, target, command, args, env, working_dir, "user", timeout,
			concurrency, status, created_at, started_at, completed_at,
			duration_ms, total_agents, completed_agents, successful_agents,
			failed_agents, success_rate
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		ON CONFLICT(id) DO UPDATE SET
			status = EXCLUDED.status,
			started_at = EXCLUDED.started_at,
			completed_at = EXCLUDED.completed_at,
			duration_ms = EXCLUDED.duration_ms,
			total_agents = EXCLUDED.total_agents,
			completed_agents = EXCLUDED.completed_agents,
			successful_agents = EXCLUDED.successful_agents,
			failed_agents = EXCLUDED.failed_agents,
			success_rate = EXCLUDED.success_rate
	`

	_, err := s.db.ExecContext(ctx, query,
		job.ID, job.Target, job.Command, args, env,
		job.WorkingDir, job.User, job.Timeout, job.Concurrency,
		job.Status, job.CreatedAt, job.StartedAt, job.CompletedAt,
		job.DurationMs, job.TotalAgents, job.CompletedAgents,
		job.SuccessfulAgents, job.FailedAgents, job.SuccessRate,
	)

	return err
}

// GetBatchJob retrieves a batch job by ID
func (s *PostgreSQLStore) GetBatchJob(ctx context.Context, batchJobID string) (*BatchJobRecord, error) {
	query := `
		SELECT id, target, command, args, env, working_dir, "user", timeout,
			   concurrency, status, created_at, started_at, completed_at,
			   duration_ms, total_agents, completed_agents, successful_agents,
			   failed_agents, success_rate
		FROM batch_jobs
		WHERE id = $1
	`

	var job BatchJobRecord
	var args, env []byte
	var startedAt, completedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, query, batchJobID).Scan(
		&job.ID, &job.Target, &job.Command, &args, &env,
		&job.WorkingDir, &job.User, &job.Timeout, &job.Concurrency,
		&job.Status, &job.CreatedAt, &startedAt, &completedAt,
		&job.DurationMs, &job.TotalAgents, &job.CompletedAgents,
		&job.SuccessfulAgents, &job.FailedAgents, &job.SuccessRate,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("batch job not found: %s", batchJobID)
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal(args, &job.Args)
	json.Unmarshal(env, &job.Env)

	if startedAt.Valid {
		t := startedAt.Time
		job.StartedAt = &t
	}
	if completedAt.Valid {
		t := completedAt.Time
		job.CompletedAt = &t
	}

	// Load agent results
	resultsQuery := `
		SELECT agent_id, success, exit_code, error, duration_ms, created_at
		FROM batch_agent_results
		WHERE batch_job_id = $1
		ORDER BY created_at
	`

	rows, err := s.db.QueryContext(ctx, resultsQuery, batchJobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var result BatchAgentResultRecord
		var errorStr sql.NullString

		err := rows.Scan(
			&result.AgentID, &result.Success, &result.ExitCode,
			&errorStr, &result.DurationMs, &result.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		result.BatchJobID = batchJobID
		if errorStr.Valid {
			result.Error = errorStr.String
		}

		job.AgentResults = append(job.AgentResults, &result)
	}

	return &job, rows.Err()
}

// ListBatchJobs lists batch jobs with optional filtering
func (s *PostgreSQLStore) ListBatchJobs(ctx context.Context, filter *BatchJobFilter) ([]*BatchJobRecord, error) {
	query := `
		SELECT id, target, command, args, env, working_dir, "user", timeout,
			   concurrency, status, created_at, started_at, completed_at,
			   duration_ms, total_agents, completed_agents, successful_agents,
			   failed_agents, success_rate
		FROM batch_jobs
		WHERE 1=1
	`

	args := []interface{}{}
	paramCount := 0

	if filter != nil {
		if filter.Status != nil {
			paramCount++
			query += fmt.Sprintf(" AND status = $%d", paramCount)
			args = append(args, *filter.Status)
		}

		if filter.Target != "" {
			paramCount++
			query += fmt.Sprintf(" AND target ILIKE $%d", paramCount)
			args = append(args, "%"+filter.Target+"%")
		}

		if filter.StartTime != nil {
			paramCount++
			query += fmt.Sprintf(" AND created_at >= $%d", paramCount)
			args = append(args, *filter.StartTime)
		}

		if filter.EndTime != nil {
			paramCount++
			query += fmt.Sprintf(" AND created_at <= $%d", paramCount)
			args = append(args, *filter.EndTime)
		}

		// Sorting
		sortBy := "created_at"
		if filter.SortBy != "" {
			validColumns := map[string]bool{
				"id": true, "target": true, "command": true, "status": true,
				"created_at": true, "started_at": true, "completed_at": true,
			}
			if validColumns[filter.SortBy] {
				sortBy = filter.SortBy
			}
		}
		sortOrder := "DESC"
		if filter.SortOrder == "asc" {
			sortOrder = "ASC"
		}
		query += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)

		// Pagination
		if filter.Limit > 0 {
			paramCount++
			query += fmt.Sprintf(" LIMIT $%d", paramCount)
			args = append(args, filter.Limit)
		}
		if filter.Offset > 0 {
			paramCount++
			query += fmt.Sprintf(" OFFSET $%d", paramCount)
			args = append(args, filter.Offset)
		}
	} else {
		// Default sorting when no filter provided
		query += " ORDER BY created_at DESC"
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*BatchJobRecord

	for rows.Next() {
		var job BatchJobRecord
		var argsBytes, envBytes []byte
		var startedAt, completedAt sql.NullTime

		err := rows.Scan(
			&job.ID, &job.Target, &job.Command, &argsBytes, &envBytes,
			&job.WorkingDir, &job.User, &job.Timeout, &job.Concurrency,
			&job.Status, &job.CreatedAt, &startedAt, &completedAt,
			&job.DurationMs, &job.TotalAgents, &job.CompletedAgents,
			&job.SuccessfulAgents, &job.FailedAgents, &job.SuccessRate,
		)
		if err != nil {
			return nil, err
		}

		json.Unmarshal(argsBytes, &job.Args)
		json.Unmarshal(envBytes, &job.Env)

		if startedAt.Valid {
			t := startedAt.Time
			job.StartedAt = &t
		}
		if completedAt.Valid {
			t := completedAt.Time
			job.CompletedAt = &t
		}

		jobs = append(jobs, &job)
	}

	return jobs, rows.Err()
}

// UpdateBatchJobStatus updates the status of a batch job
func (s *PostgreSQLStore) UpdateBatchJobStatus(ctx context.Context, batchJobID string, status pb.BatchJobStatus) error {
	query := `UPDATE batch_jobs SET status = $1 WHERE id = $2`
	_, err := s.db.ExecContext(ctx, query, status, batchJobID)
	return err
}

// UpdateBatchJobProgress updates the progress of a batch job
func (s *PostgreSQLStore) UpdateBatchJobProgress(ctx context.Context, batchJobID string, progress *BatchJobProgress) error {
	query := `
		UPDATE batch_jobs
		SET total_agents = $1,
		    completed_agents = $2,
		    successful_agents = $3,
		    failed_agents = $4,
		    success_rate = $5,
		    started_at = $6,
		    completed_at = $7,
		    duration_ms = $8
		WHERE id = $9
	`

	_, err := s.db.ExecContext(ctx, query,
		progress.TotalAgents,
		progress.CompletedAgents,
		progress.SuccessfulAgents,
		progress.FailedAgents,
		progress.SuccessRate,
		progress.StartedAt,
		progress.CompletedAt,
		progress.DurationMs,
		batchJobID,
	)

	return err
}

// SaveBatchAgentResult saves a batch agent result
func (s *PostgreSQLStore) SaveBatchAgentResult(ctx context.Context, batchJobID string, result *BatchAgentResultRecord) error {
	query := `
		INSERT INTO batch_agent_results (
			batch_job_id, agent_id, success, exit_code, error, duration_ms, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT(batch_job_id, agent_id) DO UPDATE SET
			success = EXCLUDED.success,
			exit_code = EXCLUDED.exit_code,
			error = EXCLUDED.error,
			duration_ms = EXCLUDED.duration_ms
	`

	_, err := s.db.ExecContext(ctx, query,
		batchJobID,
		result.AgentID,
		result.Success,
		result.ExitCode,
		result.Error,
		result.DurationMs,
		result.CreatedAt,
	)

	return err
}

// Ping checks database connectivity
func (s *PostgreSQLStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close closes the database connection
func (s *PostgreSQLStore) Close() error {
	return s.db.Close()
}
