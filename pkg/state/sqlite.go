package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	pb "github.com/titananvil/titan-anvil/pkg/api/v1"
)

// SQLiteStore implements Store using SQLite
type SQLiteStore struct {
	db   *sql.DB
	path string
}

// NewSQLiteStore creates a new SQLite store
func NewSQLiteStore(config *Config) (*SQLiteStore, error) {
	path := config.SQLitePath
	if path == "" {
		path = "./data/titan-anvil.db"
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Build connection string with options
	connStr := path
	params := []string{}

	if config.SQLiteWAL {
		params = append(params, "_journal_mode=WAL")
	}

	if config.SQLiteBusyTimeout > 0 {
		params = append(params, fmt.Sprintf("_timeout=%d", config.SQLiteBusyTimeout))
	}

	if len(params) > 0 {
		connStr = fmt.Sprintf("%s?%s", path, strings.Join(params, "&"))
	}

	// Open database
	db, err := sql.Open("sqlite3", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings for SQLite
	db.SetMaxOpenConns(1) // SQLite only supports one writer
	db.SetMaxIdleConns(1)

	store := &SQLiteStore{
		db:   db,
		path: path,
	}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates the database schema
func (s *SQLiteStore) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS agents (
		id TEXT PRIMARY KEY,
		hostname TEXT NOT NULL,
		os TEXT NOT NULL,
		architecture TEXT NOT NULL,
		ip_addresses TEXT,
		platform_version TEXT,
		agent_version TEXT,
		labels TEXT,
		status INTEGER NOT NULL,
		last_heartbeat TIMESTAMP NOT NULL,
		registered_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		cpu_percent REAL,
		memory_percent REAL,
		disk_percent REAL,
		load_average TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
	CREATE INDEX IF NOT EXISTS idx_agents_last_heartbeat ON agents(last_heartbeat);

	CREATE TABLE IF NOT EXISTS commands (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		command TEXT NOT NULL,
		args TEXT,
		env TEXT,
		working_dir TEXT,
		user TEXT,
		timeout INTEGER,
		status INTEGER NOT NULL,
		exit_code INTEGER,
		stdout TEXT,
		stderr TEXT,
		error TEXT,
		created_at TIMESTAMP NOT NULL,
		started_at TIMESTAMP,
		completed_at TIMESTAMP,
		duration_ms INTEGER,
		FOREIGN KEY (agent_id) REFERENCES agents(id)
	);

	CREATE INDEX IF NOT EXISTS idx_commands_agent_id ON commands(agent_id);
	CREATE INDEX IF NOT EXISTS idx_commands_status ON commands(status);
	CREATE INDEX IF NOT EXISTS idx_commands_created_at ON commands(created_at);

	CREATE TABLE IF NOT EXISTS batch_jobs (
		id TEXT PRIMARY KEY,
		target TEXT NOT NULL,
		command TEXT NOT NULL,
		args TEXT,
		env TEXT,
		working_dir TEXT,
		user TEXT,
		timeout INTEGER,
		concurrency INTEGER,
		status INTEGER NOT NULL,
		created_at TIMESTAMP NOT NULL,
		started_at TIMESTAMP,
		completed_at TIMESTAMP,
		duration_ms INTEGER,
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
		batch_job_id TEXT NOT NULL,
		agent_id TEXT NOT NULL,
		success INTEGER NOT NULL,
		exit_code INTEGER,
		error TEXT,
		duration_ms INTEGER,
		created_at TIMESTAMP NOT NULL,
		PRIMARY KEY (batch_job_id, agent_id),
		FOREIGN KEY (batch_job_id) REFERENCES batch_jobs(id),
		FOREIGN KEY (agent_id) REFERENCES agents(id)
	);

	CREATE INDEX IF NOT EXISTS idx_batch_agent_results_batch_job_id ON batch_agent_results(batch_job_id);
	CREATE INDEX IF NOT EXISTS idx_batch_agent_results_agent_id ON batch_agent_results(agent_id);
	`

	_, err := s.db.Exec(schema)
	return err
}

// SaveAgent saves an agent record
func (s *SQLiteStore) SaveAgent(ctx context.Context, agent *AgentRecord) error {
	ipAddresses, _ := json.Marshal(agent.IPAddresses)
	labels, _ := json.Marshal(agent.Labels)
	loadAvg, _ := json.Marshal(agent.LoadAverage)

	query := `
		INSERT INTO agents (
			id, hostname, os, architecture, ip_addresses, platform_version,
			agent_version, labels, status, last_heartbeat, registered_at,
			updated_at, cpu_percent, memory_percent, disk_percent, load_average
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			hostname = excluded.hostname,
			os = excluded.os,
			architecture = excluded.architecture,
			ip_addresses = excluded.ip_addresses,
			platform_version = excluded.platform_version,
			agent_version = excluded.agent_version,
			labels = excluded.labels,
			status = excluded.status,
			last_heartbeat = excluded.last_heartbeat,
			updated_at = excluded.updated_at,
			cpu_percent = excluded.cpu_percent,
			memory_percent = excluded.memory_percent,
			disk_percent = excluded.disk_percent,
			load_average = excluded.load_average
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
func (s *SQLiteStore) GetAgent(ctx context.Context, agentID string) (*AgentRecord, error) {
	query := `
		SELECT id, hostname, os, architecture, ip_addresses, platform_version,
			agent_version, labels, status, last_heartbeat, registered_at, updated_at,
			cpu_percent, memory_percent, disk_percent, load_average
		FROM agents
		WHERE id = ?
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
func (s *SQLiteStore) ListAgents(ctx context.Context, filter *AgentFilter) ([]*AgentRecord, error) {
	query := `
		SELECT id, hostname, os, architecture, ip_addresses, platform_version,
			agent_version, labels, status, last_heartbeat, registered_at, updated_at,
			cpu_percent, memory_percent, disk_percent, load_average
		FROM agents
		WHERE 1=1
	`
	args := []interface{}{}

	if filter != nil {
		if filter.Status != nil {
			query += " AND status = ?"
			args = append(args, *filter.Status)
		}

		// Add sorting
		sortBy := "registered_at"
		if filter.SortBy != "" {
			sortBy = filter.SortBy
		}
		sortOrder := "DESC"
		if filter.SortOrder != "" {
			sortOrder = strings.ToUpper(filter.SortOrder)
		}
		query += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)

		// Add pagination
		if filter.Limit > 0 {
			query += " LIMIT ?"
			args = append(args, filter.Limit)
		}
		if filter.Offset > 0 {
			query += " OFFSET ?"
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
func (s *SQLiteStore) UpdateAgentStatus(ctx context.Context, agentID string, status pb.AgentStatus, lastHeartbeat time.Time) error {
	query := `UPDATE agents SET status = ?, last_heartbeat = ?, updated_at = ? WHERE id = ?`
	_, err := s.db.ExecContext(ctx, query, status, lastHeartbeat, time.Now(), agentID)
	return err
}

// UpdateAgentMetrics updates an agent's metrics
func (s *SQLiteStore) UpdateAgentMetrics(ctx context.Context, agentID string, metrics *pb.SystemMetrics) error {
	loadAvg, _ := json.Marshal(metrics.LoadAverage)
	query := `
		UPDATE agents
		SET cpu_percent = ?, memory_percent = ?, disk_percent = ?, load_average = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := s.db.ExecContext(ctx, query,
		metrics.CpuPercent, metrics.MemoryPercent, metrics.DiskPercent, loadAvg, time.Now(), agentID,
	)
	return err
}

// DeleteAgent deletes an agent
func (s *SQLiteStore) DeleteAgent(ctx context.Context, agentID string) error {
	query := `DELETE FROM agents WHERE id = ?`
	_, err := s.db.ExecContext(ctx, query, agentID)
	return err
}

// SaveCommand saves a command record
func (s *SQLiteStore) SaveCommand(ctx context.Context, cmd *CommandRecord) error {
	args, _ := json.Marshal(cmd.Args)
	env, _ := json.Marshal(cmd.Env)

	query := `
		INSERT INTO commands (
			id, agent_id, command, args, env, working_dir, user, timeout,
			status, exit_code, stdout, stderr, error, created_at, started_at,
			completed_at, duration_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		cmd.ID, cmd.AgentID, cmd.Command, args, env, cmd.WorkingDir, cmd.User,
		cmd.Timeout, cmd.Status, cmd.ExitCode, cmd.Stdout, cmd.Stderr, cmd.Error,
		cmd.CreatedAt, cmd.StartedAt, cmd.CompletedAt, cmd.DurationMs,
	)

	return err
}

// GetCommand retrieves a command by ID
func (s *SQLiteStore) GetCommand(ctx context.Context, commandID string) (*CommandRecord, error) {
	query := `
		SELECT id, agent_id, command, args, env, working_dir, user, timeout,
			status, exit_code, stdout, stderr, error, created_at, started_at,
			completed_at, duration_ms
		FROM commands
		WHERE id = ?
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
func (s *SQLiteStore) ListCommands(ctx context.Context, filter *CommandFilter) ([]*CommandRecord, error) {
	query := `
		SELECT id, agent_id, command, args, env, working_dir, user, timeout,
			status, exit_code, stdout, stderr, error, created_at, started_at,
			completed_at, duration_ms
		FROM commands
		WHERE 1=1
	`
	args := []interface{}{}

	if filter != nil {
		if filter.AgentID != "" {
			query += " AND agent_id = ?"
			args = append(args, filter.AgentID)
		}
		if filter.Status != nil {
			query += " AND status = ?"
			args = append(args, *filter.Status)
		}
		if filter.StartTime != nil {
			query += " AND created_at >= ?"
			args = append(args, *filter.StartTime)
		}
		if filter.EndTime != nil {
			query += " AND created_at <= ?"
			args = append(args, *filter.EndTime)
		}

		// Add sorting
		sortBy := "created_at"
		if filter.SortBy != "" {
			sortBy = filter.SortBy
		}
		sortOrder := "DESC"
		if filter.SortOrder != "" {
			sortOrder = strings.ToUpper(filter.SortOrder)
		}
		query += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)

		// Add pagination
		if filter.Limit > 0 {
			query += " LIMIT ?"
			args = append(args, filter.Limit)
		}
		if filter.Offset > 0 {
			query += " OFFSET ?"
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
func (s *SQLiteStore) UpdateCommandStatus(ctx context.Context, commandID string, status pb.CommandStatus) error {
	query := `UPDATE commands SET status = ? WHERE id = ?`
	_, err := s.db.ExecContext(ctx, query, status, commandID)
	return err
}

// UpdateCommandResult updates command execution result
func (s *SQLiteStore) UpdateCommandResult(ctx context.Context, commandID string, result *CommandResult) error {
	query := `
		UPDATE commands
		SET status = ?, exit_code = ?, stdout = ?, stderr = ?, error = ?,
			started_at = ?, completed_at = ?, duration_ms = ?
		WHERE id = ?
	`
	_, err := s.db.ExecContext(ctx, query,
		result.Status, result.ExitCode, result.Stdout, result.Stderr, result.Error,
		result.StartedAt, result.CompletedAt, result.DurationMs, commandID,
	)
	return err
}

// SaveBatchJob saves a batch job record
func (s *SQLiteStore) SaveBatchJob(ctx context.Context, job *BatchJobRecord) error {
	args, _ := json.Marshal(job.Args)
	env, _ := json.Marshal(job.Env)

	query := `
		INSERT INTO batch_jobs (
			id, target, command, args, env, working_dir, user, timeout,
			concurrency, status, created_at, started_at, completed_at,
			duration_ms, total_agents, completed_agents, successful_agents,
			failed_agents, success_rate
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			started_at = excluded.started_at,
			completed_at = excluded.completed_at,
			duration_ms = excluded.duration_ms,
			total_agents = excluded.total_agents,
			completed_agents = excluded.completed_agents,
			successful_agents = excluded.successful_agents,
			failed_agents = excluded.failed_agents,
			success_rate = excluded.success_rate
	`

	_, err := s.db.ExecContext(ctx, query,
		job.ID, job.Target, job.Command, string(args), string(env),
		job.WorkingDir, job.User, job.Timeout, job.Concurrency,
		job.Status, job.CreatedAt, job.StartedAt, job.CompletedAt,
		job.DurationMs, job.TotalAgents, job.CompletedAgents,
		job.SuccessfulAgents, job.FailedAgents, job.SuccessRate,
	)

	return err
}

// GetBatchJob retrieves a batch job by ID
func (s *SQLiteStore) GetBatchJob(ctx context.Context, batchJobID string) (*BatchJobRecord, error) {
	query := `
		SELECT id, target, command, args, env, working_dir, user, timeout,
			   concurrency, status, created_at, started_at, completed_at,
			   duration_ms, total_agents, completed_agents, successful_agents,
			   failed_agents, success_rate
		FROM batch_jobs
		WHERE id = ?
	`

	var job BatchJobRecord
	var args, env sql.NullString
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

	if args.Valid {
		json.Unmarshal([]byte(args.String), &job.Args)
	}
	if env.Valid {
		json.Unmarshal([]byte(env.String), &job.Env)
	}
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
		WHERE batch_job_id = ?
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
func (s *SQLiteStore) ListBatchJobs(ctx context.Context, filter *BatchJobFilter) ([]*BatchJobRecord, error) {
	query := `
		SELECT id, target, command, args, env, working_dir, user, timeout,
			   concurrency, status, created_at, started_at, completed_at,
			   duration_ms, total_agents, completed_agents, successful_agents,
			   failed_agents, success_rate
		FROM batch_jobs
		WHERE 1=1
	`

	args := []interface{}{}

	if filter.Status != nil {
		query += " AND status = ?"
		args = append(args, *filter.Status)
	}

	if filter.Target != "" {
		query += " AND target LIKE ?"
		args = append(args, "%"+filter.Target+"%")
	}

	if filter.StartTime != nil {
		query += " AND created_at >= ?"
		args = append(args, *filter.StartTime)
	}

	if filter.EndTime != nil {
		query += " AND created_at <= ?"
		args = append(args, *filter.EndTime)
	}

	// Sorting
	sortBy := "created_at"
	if filter.SortBy != "" {
		sortBy = filter.SortBy
	}
	sortOrder := "DESC"
	if filter.SortOrder == "asc" {
		sortOrder = "ASC"
	}
	query += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)

	// Pagination
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*BatchJobRecord

	for rows.Next() {
		var job BatchJobRecord
		var argsStr, envStr sql.NullString
		var startedAt, completedAt sql.NullTime

		err := rows.Scan(
			&job.ID, &job.Target, &job.Command, &argsStr, &envStr,
			&job.WorkingDir, &job.User, &job.Timeout, &job.Concurrency,
			&job.Status, &job.CreatedAt, &startedAt, &completedAt,
			&job.DurationMs, &job.TotalAgents, &job.CompletedAgents,
			&job.SuccessfulAgents, &job.FailedAgents, &job.SuccessRate,
		)
		if err != nil {
			return nil, err
		}

		if argsStr.Valid {
			json.Unmarshal([]byte(argsStr.String), &job.Args)
		}
		if envStr.Valid {
			json.Unmarshal([]byte(envStr.String), &job.Env)
		}
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
func (s *SQLiteStore) UpdateBatchJobStatus(ctx context.Context, batchJobID string, status pb.BatchJobStatus) error {
	query := `UPDATE batch_jobs SET status = ? WHERE id = ?`
	_, err := s.db.ExecContext(ctx, query, status, batchJobID)
	return err
}

// UpdateBatchJobProgress updates the progress of a batch job
func (s *SQLiteStore) UpdateBatchJobProgress(ctx context.Context, batchJobID string, progress *BatchJobProgress) error {
	query := `
		UPDATE batch_jobs
		SET total_agents = ?,
		    completed_agents = ?,
		    successful_agents = ?,
		    failed_agents = ?,
		    success_rate = ?,
		    started_at = ?,
		    completed_at = ?,
		    duration_ms = ?
		WHERE id = ?
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
func (s *SQLiteStore) SaveBatchAgentResult(ctx context.Context, batchJobID string, result *BatchAgentResultRecord) error {
	query := `
		INSERT INTO batch_agent_results (
			batch_job_id, agent_id, success, exit_code, error, duration_ms, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(batch_job_id, agent_id) DO UPDATE SET
			success = excluded.success,
			exit_code = excluded.exit_code,
			error = excluded.error,
			duration_ms = excluded.duration_ms
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
func (s *SQLiteStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close closes the database connection
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
