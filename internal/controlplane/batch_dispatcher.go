package controlplane

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shawnbutts/keystone-core/internal/state"
	"github.com/shawnbutts/keystone-core/internal/targeting"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// BatchDispatcherConfig holds configuration for the batch dispatcher.
type BatchDispatcherConfig struct {
	// DefaultBatchSize is the default concurrency for batch operations.
	DefaultBatchSize int
	// BatchDelay is the delay between starting execution on each batch group.
	BatchDelay time.Duration
}

// DefaultBatchDispatcherConfig returns the default configuration.
func DefaultBatchDispatcherConfig() *BatchDispatcherConfig {
	return &BatchDispatcherConfig{
		DefaultBatchSize: 10,
	}
}

// BatchDispatcher handles batch command execution across multiple agents
type BatchDispatcher struct {
	connMgr  *ConnectionManager
	cmdDsp   *CommandDispatcher
	store    state.BatchJobStore
	executor *targeting.BatchExecutor
	config   *BatchDispatcherConfig

	// Track batch jobs
	mu        sync.RWMutex
	batchJobs map[string]*BatchJob
}

// BatchJob represents a batch execution job
type BatchJob struct {
	ID          string
	Target      string
	Command     string
	Args        []string
	Env         map[string]string
	Status      pb.BatchJobStatus
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time

	// Progress tracking
	Total      int32
	Completed  int32
	Successful int32
	Failed     int32

	// Results
	AgentResults []*pb.BatchAgentResult
}

// connectionManagerAdapter adapts ConnectionManager to targeting.ConnectionManager
type connectionManagerAdapter struct {
	connMgr *ConnectionManager
}

func (a *connectionManagerAdapter) ListAgents() []*targeting.AgentInfo {
	agents := a.connMgr.ListAgents()
	result := make([]*targeting.AgentInfo, len(agents))
	for i, agent := range agents {
		result[i] = &targeting.AgentInfo{
			ID:       agent.ID,
			Status:   agent.Status,
			Metadata: agent.Metadata,
		}
	}
	return result
}

func (a *connectionManagerAdapter) GetAgent(id string) (*targeting.AgentInfo, error) {
	agent, err := a.connMgr.GetAgent(id)
	if err != nil {
		return nil, err
	}
	return &targeting.AgentInfo{
		ID:       agent.ID,
		Status:   agent.Status,
		Metadata: agent.Metadata,
	}, nil
}

// NewBatchDispatcher creates a new batch dispatcher with default configuration.
func NewBatchDispatcher(connMgr *ConnectionManager, cmdDsp *CommandDispatcher, store state.BatchJobStore) *BatchDispatcher {
	return NewBatchDispatcherWithConfig(connMgr, cmdDsp, store, nil)
}

// NewBatchDispatcherWithConfig creates a new batch dispatcher with custom configuration.
func NewBatchDispatcherWithConfig(connMgr *ConnectionManager, cmdDsp *CommandDispatcher, store state.BatchJobStore, cfg *BatchDispatcherConfig) *BatchDispatcher {
	if cfg == nil {
		cfg = DefaultBatchDispatcherConfig()
	}

	// Create adapter for connection manager
	adapter := &connectionManagerAdapter{connMgr: connMgr}

	// Create batch executor using the command dispatcher
	executor := targeting.NewBatchExecutor(cmdDsp, adapter)

	// Apply default batch size from config
	if cfg.DefaultBatchSize > 0 {
		executor.SetConcurrency(cfg.DefaultBatchSize)
	}

	return &BatchDispatcher{
		connMgr:   connMgr,
		cmdDsp:    cmdDsp,
		store:     store,
		executor:  executor,
		config:    cfg,
		batchJobs: make(map[string]*BatchJob),
	}
}

// ExecuteBatch executes a command across multiple agents using a target expression
func (bd *BatchDispatcher) ExecuteBatch(ctx context.Context, req *pb.BatchExecuteCommandRequest) (<-chan *pb.BatchExecuteCommandResponse, error) {
	// Generate batch job ID if not provided
	if req.BatchJobId == "" {
		req.BatchJobId = uuid.New().String()
	}

	now := time.Now()

	// Create batch job record
	job := &BatchJob{
		ID:        req.BatchJobId,
		Target:    req.Target,
		Command:   req.Command,
		Args:      req.Args,
		Env:       req.Env,
		Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING,
		CreatedAt: now,
	}

	// Persist to database
	dbJob := &state.BatchJobRecord{
		ID:          job.ID,
		Target:      job.Target,
		Command:     job.Command,
		Args:        job.Args,
		Env:         job.Env,
		WorkingDir:  req.WorkingDir,
		User:        req.User,
		Timeout:     req.Timeout,
		Concurrency: req.Concurrency,
		Status:      job.Status,
		CreatedAt:   job.CreatedAt,
	}

	if err := bd.store.SaveBatchJob(ctx, dbJob); err != nil {
		return nil, fmt.Errorf("failed to save batch job: %w", err)
	}

	// Store job in memory
	bd.mu.Lock()
	bd.batchJobs[job.ID] = job
	bd.mu.Unlock()

	// Create response channel
	responseChan := make(chan *pb.BatchExecuteCommandResponse, 100)

	// Execute batch in background
	go bd.executeBatchJob(ctx, req, job, responseChan)

	return responseChan, nil
}

// executeBatchJob executes the batch job
func (bd *BatchDispatcher) executeBatchJob(ctx context.Context, req *pb.BatchExecuteCommandRequest, job *BatchJob, responseChan chan<- *pb.BatchExecuteCommandResponse) {
	defer close(responseChan)

	// Send batch start event
	startTime := time.Now()
	job.StartedAt = &startTime
	job.Status = pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING

	// Update status in database
	_ = bd.store.UpdateBatchJobStatus(ctx, job.ID, job.Status) //nolint:errcheck // best-effort persistence

	responseChan <- &pb.BatchExecuteCommandResponse{
		BatchJobId: job.ID,
		Type:       pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_START,
		Timestamp:  timestamppb.New(startTime),
	}

	// Set concurrency if specified
	if req.Concurrency > 0 {
		bd.executor.SetConcurrency(int(req.Concurrency))
	}

	// Execute using the batch executor
	opts := &targeting.ExecuteOptions{
		Target:     req.Target,
		Command:    req.Command,
		Args:       req.Args,
		Env:        req.Env,
		WorkingDir: req.WorkingDir,
		User:       req.User,
		Timeout:    req.Timeout,
	}

	batch, err := bd.executor.ExecuteWithOptions(ctx, opts)
	if err != nil {
		// Batch failed to start
		now := time.Now()
		job.CompletedAt = &now
		job.Status = pb.BatchJobStatus_BATCH_JOB_STATUS_FAILED
		responseChan <- &pb.BatchExecuteCommandResponse{
			BatchJobId: job.ID,
			Type:       pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_FAILED,
			Error:      err.Error(),
			Timestamp:  timestamppb.New(now),
		}
		return
	}

	// Update total count
	total, _, _ := batch.Progress()
	//nolint:gosec // G115: batch job agent counts are bounded by cluster size, fits in int32
	job.Total = int32(total)

	// Persist initial progress
	//nolint:gosec // G115: batch job agent counts are bounded by cluster size, fits in int32
	_ = bd.store.UpdateBatchJobProgress(ctx, job.ID, &state.BatchJobProgress{ //nolint:errcheck // best-effort persistence
		TotalAgents: int32(total),
		StartedAt:   job.StartedAt,
	})

	// Send periodic progress updates
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				if batch.IsComplete() {
					close(done)
					return
				}

				// Send progress update
				total, completed, failed := batch.Progress()
				successful := completed - failed
				successRate := batch.SuccessRate()

				job.Completed = int32(completed)   //nolint:gosec // G115: bounded by cluster size
				job.Successful = int32(successful) //nolint:gosec // G115: bounded by cluster size
				job.Failed = int32(failed)         //nolint:gosec // G115: bounded by cluster size

				// Persist progress to database
				//nolint:gosec // G115: batch job agent counts are bounded by cluster size, fits in int32
				_ = bd.store.UpdateBatchJobProgress(ctx, job.ID, &state.BatchJobProgress{ //nolint:errcheck // best-effort persistence
					TotalAgents:      int32(total),
					CompletedAgents:  int32(completed),
					SuccessfulAgents: int32(successful),
					FailedAgents:     int32(failed),
					SuccessRate:      float32(successRate),
					StartedAt:        job.StartedAt,
				})

				now := time.Now()
				//nolint:gosec // G115: batch job agent counts are bounded by cluster size, fits in int32
				responseChan <- &pb.BatchExecuteCommandResponse{
					BatchJobId: job.ID,
					Type:       pb.BatchResponseType_BATCH_RESPONSE_TYPE_PROGRESS,
					Progress: &pb.BatchProgress{
						Total:       int32(total),
						Completed:   int32(completed),
						Successful:  int32(successful),
						Failed:      int32(failed),
						SuccessRate: float32(successRate),
					},
					Timestamp: timestamppb.New(now),
				}
			case <-ctx.Done():
				close(done)
				return
			}
		}
	}()

	// Wait for completion
	<-done

	// Get final results
	results := batch.Results()
	agentResults := make([]*pb.BatchAgentResult, len(results))
	for i, result := range results {
		agentResults[i] = &pb.BatchAgentResult{
			AgentId:    result.AgentID,
			Success:    result.Success,
			ExitCode:   result.ExitCode,
			Error:      errorString(result.Error),
			DurationMs: result.Duration.Milliseconds(),
		}

		// Persist each agent result to database
		_ = bd.store.SaveBatchAgentResult(ctx, job.ID, &state.BatchAgentResultRecord{ //nolint:errcheck // best-effort persistence
			BatchJobID: job.ID,
			AgentID:    result.AgentID,
			Success:    result.Success,
			ExitCode:   result.ExitCode,
			Error:      errorString(result.Error),
			DurationMs: result.Duration.Milliseconds(),
			CreatedAt:  time.Now(),
		})
	}

	job.AgentResults = agentResults

	// Calculate final stats
	total, completed, failed := batch.Progress()
	successful := completed - failed
	successRate := batch.SuccessRate()

	job.Completed = int32(completed)   //nolint:gosec // G115: bounded by cluster size
	job.Successful = int32(successful) //nolint:gosec // G115: bounded by cluster size
	job.Failed = int32(failed)         //nolint:gosec // G115: bounded by cluster size

	// Send completion
	completedTime := time.Now()
	job.CompletedAt = &completedTime
	job.Status = pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED

	duration := completedTime.Sub(job.CreatedAt)

	// Persist final state to database
	//nolint:gosec // G115: batch job agent counts are bounded by cluster size, fits in int32
	_ = bd.store.UpdateBatchJobProgress(ctx, job.ID, &state.BatchJobProgress{ //nolint:errcheck // best-effort persistence
		TotalAgents:      int32(total),
		CompletedAgents:  int32(completed),
		SuccessfulAgents: int32(successful),
		FailedAgents:     int32(failed),
		SuccessRate:      float32(successRate),
		StartedAt:        job.StartedAt,
		CompletedAt:      &completedTime,
		DurationMs:       duration.Milliseconds(),
	})
	_ = bd.store.UpdateBatchJobStatus(ctx, job.ID, job.Status) //nolint:errcheck // best-effort persistence

	//nolint:gosec // G115: batch job agent counts are bounded by cluster size, fits in int32
	responseChan <- &pb.BatchExecuteCommandResponse{
		BatchJobId: job.ID,
		Type:       pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_COMPLETE,
		Summary: &pb.BatchSummary{
			Total:        int32(total),
			Successful:   int32(successful),
			Failed:       int32(failed),
			SuccessRate:  float32(successRate),
			DurationMs:   duration.Milliseconds(),
			AgentResults: agentResults,
		},
		Timestamp: timestamppb.New(completedTime),
	}
}

// GetBatchJobStatus retrieves the status of a batch job
func (bd *BatchDispatcher) GetBatchJobStatus(batchJobID string) (*pb.BatchJobInfo, error) {
	bd.mu.RLock()
	defer bd.mu.RUnlock()

	job, ok := bd.batchJobs[batchJobID]
	if !ok {
		return nil, fmt.Errorf("batch job not found: %s", batchJobID)
	}

	info := &pb.BatchJobInfo{
		BatchJobId: job.ID,
		Target:     job.Target,
		Command:    job.Command,
		Args:       job.Args,
		Status:     job.Status,
		Progress: &pb.BatchProgress{
			Total:       job.Total,
			Completed:   job.Completed,
			Successful:  job.Successful,
			Failed:      job.Failed,
			SuccessRate: calculateSuccessRate(job.Total, job.Successful),
		},
		CreatedAt: timestamppb.New(job.CreatedAt),
	}

	if job.StartedAt != nil {
		info.StartedAt = timestamppb.New(*job.StartedAt)
	}

	if job.CompletedAt != nil {
		info.CompletedAt = timestamppb.New(*job.CompletedAt)
		info.DurationMs = job.CompletedAt.Sub(job.CreatedAt).Milliseconds()

		// Add summary if completed
		info.Summary = &pb.BatchSummary{
			Total:        job.Total,
			Successful:   job.Successful,
			Failed:       job.Failed,
			SuccessRate:  calculateSuccessRate(job.Total, job.Successful),
			DurationMs:   info.DurationMs,
			AgentResults: job.AgentResults,
		}
	}

	return info, nil
}

// ListBatchJobs lists batch jobs with optional filtering
func (bd *BatchDispatcher) ListBatchJobs(status pb.BatchJobStatus, limit int) []*pb.BatchJobInfo {
	bd.mu.RLock()
	defer bd.mu.RUnlock()

	var jobs []*pb.BatchJobInfo
	for _, job := range bd.batchJobs {
		// Filter by status if specified
		if status != pb.BatchJobStatus_BATCH_JOB_STATUS_UNSPECIFIED && job.Status != status {
			continue
		}

		info, _ := bd.GetBatchJobStatus(job.ID)
		if info != nil {
			jobs = append(jobs, info)
		}

		// Apply limit
		if limit > 0 && len(jobs) >= limit {
			break
		}
	}

	return jobs
}

// Helper functions

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func calculateSuccessRate(total, successful int32) float32 {
	if total == 0 {
		return 0.0
	}
	return float32(successful) / float32(total) * 100.0
}
