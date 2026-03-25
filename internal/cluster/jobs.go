package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

const (
	// jobKeyPrefix is the etcd key prefix for job data.
	jobKeyPrefix = "/jobs/"

	// jobQueuePrefix is the etcd key prefix for job queues.
	jobQueuePrefix = "/job_queue/"
)

// JobStatus represents the status of a distributed job.
type JobStatus string

const (
	// JobStatusPending indicates the job is waiting to be assigned.
	JobStatusPending JobStatus = "pending"

	// JobStatusAssigned indicates the job has been assigned to a member.
	JobStatusAssigned JobStatus = "assigned"

	// JobStatusRunning indicates the job is currently executing.
	JobStatusRunning JobStatus = "running"

	// JobStatusCompleted indicates the job completed successfully.
	JobStatusCompleted JobStatus = "completed"

	// JobStatusFailed indicates the job failed.
	JobStatusFailed JobStatus = "failed"

	// JobStatusTimeout indicates the job timed out.
	JobStatusTimeout JobStatus = "timeout"

	// JobStatusCancelled indicates the job was cancelled.
	JobStatusCancelled JobStatus = "cancelled"
)

// DistributedJob represents a job to be distributed across the cluster.
type DistributedJob struct {
	// ID is the unique job identifier.
	ID string `json:"id"`

	// Type identifies the type of job.
	Type string `json:"type"`

	// TargetAgentID is the agent this job should run on (optional).
	// If set, the job is routed to the member owning this agent.
	TargetAgentID string `json:"target_agent_id,omitempty"`

	// TargetMemberID is the member this job should run on (optional).
	// If set, overrides agent-based routing.
	TargetMemberID string `json:"target_member_id,omitempty"`

	// Payload contains the job data.
	Payload json.RawMessage `json:"payload"`

	// Status is the current job status.
	Status JobStatus `json:"status"`

	// AssignedMemberID is the member this job is assigned to.
	AssignedMemberID string `json:"assigned_member_id,omitempty"`

	// Priority is the job priority (higher = more important).
	Priority int `json:"priority"`

	// Timeout is the maximum execution time.
	Timeout time.Duration `json:"timeout"`

	// RetryCount is the number of retry attempts.
	RetryCount int `json:"retry_count"`

	// MaxRetries is the maximum number of retries allowed.
	MaxRetries int `json:"max_retries"`

	// CreatedAt is when the job was created.
	CreatedAt time.Time `json:"created_at"`

	// StartedAt is when the job started executing.
	StartedAt *time.Time `json:"started_at,omitempty"`

	// CompletedAt is when the job completed.
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Result contains the job result on completion.
	Result json.RawMessage `json:"result,omitempty"`

	// Error contains error information on failure.
	Error string `json:"error,omitempty"`

	// IdempotencyKey prevents duplicate job execution.
	IdempotencyKey string `json:"idempotency_key,omitempty"`

	// Metadata contains additional job metadata.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Clone creates a deep copy of the job.
func (j *DistributedJob) Clone() *DistributedJob {
	if j == nil {
		return nil
	}
	clone := *j
	if j.Payload != nil {
		clone.Payload = make(json.RawMessage, len(j.Payload))
		copy(clone.Payload, j.Payload)
	}
	if j.Result != nil {
		clone.Result = make(json.RawMessage, len(j.Result))
		copy(clone.Result, j.Result)
	}
	if j.Metadata != nil {
		clone.Metadata = make(map[string]string, len(j.Metadata))
		for k, v := range j.Metadata {
			clone.Metadata[k] = v
		}
	}
	return &clone
}

// JobHandler handles job execution.
type JobHandler func(ctx context.Context, job *DistributedJob) (json.RawMessage, error)

// JobDistributor distributes jobs across cluster members.
type JobDistributor struct {
	config       *Config
	etcd         *EtcdClient
	membership   *MembershipManager
	shardManager *ShardManager
	handlers     map[string]JobHandler
	activeJobs   map[string]*DistributedJob
	jobResults   map[string]chan *DistributedJob
	idempotency  map[string]string // idempotencyKey -> jobID
	mu           sync.RWMutex
	stopChan     chan struct{}
	doneChan     chan struct{}
	started      bool
}

// NewJobDistributor creates a new job distributor.
func NewJobDistributor(config *Config, etcd *EtcdClient, membership *MembershipManager, shardManager *ShardManager) (*JobDistributor, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if etcd == nil {
		return nil, fmt.Errorf("etcd client is required")
	}
	if membership == nil {
		return nil, fmt.Errorf("membership manager is required")
	}
	if shardManager == nil {
		return nil, fmt.Errorf("shard manager is required")
	}

	return &JobDistributor{
		config:       config,
		etcd:         etcd,
		membership:   membership,
		shardManager: shardManager,
		handlers:     make(map[string]JobHandler),
		activeJobs:   make(map[string]*DistributedJob),
		jobResults:   make(map[string]chan *DistributedJob),
		idempotency:  make(map[string]string),
		stopChan:     make(chan struct{}),
		doneChan:     make(chan struct{}),
	}, nil
}

// RegisterHandler registers a handler for a job type.
func (d *JobDistributor) RegisterHandler(jobType string, handler JobHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[jobType] = handler
}

// UnregisterHandler unregisters a handler.
func (d *JobDistributor) UnregisterHandler(jobType string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.handlers, jobType)
}

// Start starts the job distributor.
func (d *JobDistributor) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.started {
		d.mu.Unlock()
		return fmt.Errorf("job distributor already started")
	}
	d.started = true
	d.mu.Unlock()

	// Watch for jobs assigned to this member
	go d.watchJobs(ctx)

	return nil
}

// Stop stops the job distributor.
func (d *JobDistributor) Stop(ctx context.Context) error {
	d.mu.Lock()
	if !d.started {
		d.mu.Unlock()
		return nil
	}
	d.started = false
	d.mu.Unlock()

	close(d.stopChan)

	wait.ForSignal(d.doneChan, 5*time.Second)

	return nil
}

// Submit submits a job for execution.
func (d *JobDistributor) Submit(ctx context.Context, job *DistributedJob) error {
	if job == nil {
		return fmt.Errorf("job is required")
	}
	if job.ID == "" {
		return fmt.Errorf("job ID is required")
	}
	if job.Type == "" {
		return fmt.Errorf("job type is required")
	}

	// Check idempotency
	if job.IdempotencyKey != "" {
		d.mu.RLock()
		existingJobID, exists := d.idempotency[job.IdempotencyKey]
		d.mu.RUnlock()

		if exists {
			return fmt.Errorf("duplicate job with idempotency key, existing job ID: %s", existingJobID)
		}
	}

	// Determine the target member
	targetMemberID := d.determineTargetMember(job)
	if targetMemberID == "" {
		return fmt.Errorf("no available member for job")
	}

	job.AssignedMemberID = targetMemberID
	job.Status = JobStatusAssigned
	job.CreatedAt = time.Now().UTC()

	// Store the job
	if err := d.storeJob(ctx, job); err != nil {
		return fmt.Errorf("failed to store job: %w", err)
	}

	// Track idempotency
	if job.IdempotencyKey != "" {
		d.mu.Lock()
		d.idempotency[job.IdempotencyKey] = job.ID
		d.mu.Unlock()
	}

	return nil
}

// SubmitAndWait submits a job and waits for completion.
func (d *JobDistributor) SubmitAndWait(ctx context.Context, job *DistributedJob) (*DistributedJob, error) {
	// Create result channel
	resultChan := make(chan *DistributedJob, 1)
	d.mu.Lock()
	d.jobResults[job.ID] = resultChan
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		delete(d.jobResults, job.ID)
		d.mu.Unlock()
	}()

	// Submit the job
	if err := d.Submit(ctx, job); err != nil {
		return nil, err
	}

	// Wait for result
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultChan:
		return result, nil
	}
}

// GetJob retrieves a job by ID.
func (d *JobDistributor) GetJob(ctx context.Context, jobID string) (*DistributedJob, error) {
	data, err := d.etcd.Get(ctx, jobKeyPrefix+jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	var job DistributedJob
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	return &job, nil
}

// CancelJob cancels a job.
func (d *JobDistributor) CancelJob(ctx context.Context, jobID string) error {
	job, err := d.GetJob(ctx, jobID)
	if err != nil {
		return err
	}

	if job.Status == JobStatusCompleted || job.Status == JobStatusFailed || job.Status == JobStatusCancelled {
		return fmt.Errorf("job already in terminal state: %s", job.Status)
	}

	job.Status = JobStatusCancelled
	now := time.Now().UTC()
	job.CompletedAt = &now

	return d.storeJob(ctx, job)
}

// ListJobsForMember lists all jobs assigned to a member.
func (d *JobDistributor) ListJobsForMember(ctx context.Context, memberID string) ([]*DistributedJob, error) {
	data, err := d.etcd.List(ctx, jobKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}

	jobs := make([]*DistributedJob, 0)
	for _, value := range data {
		var job DistributedJob
		if err := json.Unmarshal(value, &job); err != nil {
			continue
		}
		if job.AssignedMemberID == memberID {
			jobs = append(jobs, &job)
		}
	}

	return jobs, nil
}

// GetPendingJobsCount returns the count of pending jobs.
func (d *JobDistributor) GetPendingJobsCount(ctx context.Context) (int, error) {
	data, err := d.etcd.List(ctx, jobKeyPrefix)
	if err != nil {
		return 0, fmt.Errorf("failed to list jobs: %w", err)
	}

	count := 0
	for _, value := range data {
		var job DistributedJob
		if err := json.Unmarshal(value, &job); err != nil {
			continue
		}
		if job.Status == JobStatusPending || job.Status == JobStatusAssigned {
			count++
		}
	}

	return count, nil
}

// determineTargetMember determines which member should handle a job.
func (d *JobDistributor) determineTargetMember(job *DistributedJob) string {
	// If explicit member specified, use it
	if job.TargetMemberID != "" {
		member, err := d.membership.GetMember(job.TargetMemberID)
		if err == nil && member.Status.IsHealthy() {
			return job.TargetMemberID
		}
		// Fall through to find another member
	}

	// If target agent specified, route to the member owning that agent
	if job.TargetAgentID != "" {
		memberID, exists := d.shardManager.GetAssignment(job.TargetAgentID)
		if exists {
			member, err := d.membership.GetMember(memberID)
			if err == nil && member.Status.IsHealthy() {
				return memberID
			}
		}
	}

	// Fall back to local member or least-loaded member
	localMember := d.membership.LocalMember()
	if localMember != nil && localMember.Status.IsHealthy() {
		return localMember.ID
	}

	// Find any healthy member
	members := d.membership.GetHealthyMembers()
	if len(members) > 0 {
		return members[0].ID
	}

	return ""
}

// storeJob stores a job in etcd.
func (d *JobDistributor) storeJob(ctx context.Context, job *DistributedJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	return d.etcd.Put(ctx, jobKeyPrefix+job.ID, data, 0)
}

// watchJobs watches for jobs assigned to this member.
func (d *JobDistributor) watchJobs(ctx context.Context) {
	defer func() {
		select {
		case d.doneChan <- struct{}{}:
		default:
		}
	}()

	localMember := d.membership.LocalMember()
	if localMember == nil {
		return
	}

	// Process any existing jobs first
	d.processExistingJobs(ctx, localMember.ID)

	// Watch for new jobs
	err := d.etcd.Watch(ctx, jobKeyPrefix, func(key string, value []byte, deleted bool) {
		if deleted {
			return
		}

		var job DistributedJob
		if err := json.Unmarshal(value, &job); err != nil {
			return
		}

		// Check if this job is assigned to us
		if job.AssignedMemberID != localMember.ID {
			return
		}

		// Process job if it's assigned
		if job.Status == JobStatusAssigned {
			go d.executeJob(ctx, &job)
		}
	})
	_ = err // error logged via callback
}

// processExistingJobs processes any jobs already assigned to this member.
// Jobs found in Running state are assumed to be from a previous crash and
// are reset to Assigned so they get re-executed.
func (d *JobDistributor) processExistingJobs(ctx context.Context, memberID string) {
	jobs, err := d.ListJobsForMember(ctx, memberID)
	if err != nil {
		return
	}

	for _, job := range jobs {
		switch job.Status {
		case JobStatusRunning:
			// Stale from a previous crash — reset to Assigned for re-execution
			job.Status = JobStatusAssigned
			job.StartedAt = nil
			_ = d.storeJob(ctx, job) //nolint:errcheck // best-effort persistence
			go d.executeJob(ctx, job)
		case JobStatusAssigned:
			go d.executeJob(ctx, job)
		}
	}
}

// executeJob executes a job.
func (d *JobDistributor) executeJob(ctx context.Context, job *DistributedJob) {
	d.mu.RLock()
	handler, exists := d.handlers[job.Type]
	d.mu.RUnlock()

	if !exists {
		d.completeJob(ctx, job, nil, fmt.Errorf("no handler for job type: %s", job.Type))
		return
	}

	// Track active job
	d.mu.Lock()
	d.activeJobs[job.ID] = job
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		delete(d.activeJobs, job.ID)
		d.mu.Unlock()
	}()

	// Update status to running
	job.Status = JobStatusRunning
	now := time.Now().UTC()
	job.StartedAt = &now
	if err := d.storeJob(ctx, job); err != nil {
		return
	}

	// Create timeout context if needed
	execCtx := ctx
	var cancel context.CancelFunc
	if job.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, job.Timeout)
		defer cancel()
	}

	// Execute the job
	result, err := handler(execCtx, job)

	// Check for timeout
	switch {
	case execCtx.Err() == context.DeadlineExceeded:
		job.Status = JobStatusTimeout
		job.Error = "job timed out"
	case err != nil:
		// Handle retry
		if job.RetryCount < job.MaxRetries {
			job.RetryCount++
			job.Status = JobStatusAssigned
			_ = d.storeJob(ctx, job) //nolint:errcheck // best-effort persistence
			// Re-queue after delay
			time.AfterFunc(time.Second*time.Duration(job.RetryCount), func() {
				d.executeJob(ctx, job)
			})
			return
		}
		d.completeJob(ctx, job, nil, err)
	default:
		d.completeJob(ctx, job, result, nil)
	}
}

// completeJob marks a job as completed.
func (d *JobDistributor) completeJob(ctx context.Context, job *DistributedJob, result json.RawMessage, err error) {
	now := time.Now().UTC()
	job.CompletedAt = &now

	if err != nil {
		job.Status = JobStatusFailed
		job.Error = err.Error()
	} else {
		job.Status = JobStatusCompleted
		job.Result = result
	}

	// Store final state
	_ = d.storeJob(ctx, job) //nolint:errcheck // best-effort persistence

	// Notify waiters
	d.mu.RLock()
	resultChan, exists := d.jobResults[job.ID]
	d.mu.RUnlock()

	if exists {
		select {
		case resultChan <- job:
		default:
		}
	}
}

// GetActiveJobs returns all active jobs on this member.
func (d *JobDistributor) GetActiveJobs() []*DistributedJob {
	d.mu.RLock()
	defer d.mu.RUnlock()

	jobs := make([]*DistributedJob, 0, len(d.activeJobs))
	for _, job := range d.activeJobs {
		jobs = append(jobs, job.Clone())
	}
	return jobs
}

// GetActiveJobCount returns the count of active jobs.
func (d *JobDistributor) GetActiveJobCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.activeJobs)
}

// ReassignJobsFromMember reassigns all jobs from a member to other members.
// This is called when a member fails.
func (d *JobDistributor) ReassignJobsFromMember(ctx context.Context, failedMemberID string) (int, error) {
	jobs, err := d.ListJobsForMember(ctx, failedMemberID)
	if err != nil {
		return 0, err
	}

	reassigned := 0
	for _, job := range jobs {
		if job.Status != JobStatusAssigned && job.Status != JobStatusRunning {
			continue
		}
		// Find a new member
		newMemberID := d.findHealthyMember(job)
		if newMemberID == "" {
			continue
		}

		job.AssignedMemberID = newMemberID
		job.Status = JobStatusAssigned
		job.RetryCount++

		if err := d.storeJob(ctx, job); err != nil {
			continue
		}
		reassigned++
	}

	return reassigned, nil
}

// findHealthyMember finds a healthy member for a job.
func (d *JobDistributor) findHealthyMember(job *DistributedJob) string {
	// If job has a target agent, try to find member owning it
	if job.TargetAgentID != "" {
		memberID, exists := d.shardManager.GetAssignment(job.TargetAgentID)
		if exists {
			member, err := d.membership.GetMember(memberID)
			if err == nil && member.Status.IsHealthy() {
				return memberID
			}
		}
	}

	// Find any healthy member
	members := d.membership.GetHealthyMembers()
	if len(members) > 0 {
		return members[0].ID
	}

	return ""
}

// CleanupCompletedJobs removes completed jobs older than the specified duration.
func (d *JobDistributor) CleanupCompletedJobs(ctx context.Context, maxAge time.Duration) (int, error) {
	data, err := d.etcd.List(ctx, jobKeyPrefix)
	if err != nil {
		return 0, fmt.Errorf("failed to list jobs: %w", err)
	}

	deleted := 0
	cutoff := time.Now().Add(-maxAge)

	for key, value := range data {
		var job DistributedJob
		if err := json.Unmarshal(value, &job); err != nil {
			continue
		}

		// Only cleanup terminal jobs
		if job.Status != JobStatusCompleted && job.Status != JobStatusFailed && job.Status != JobStatusCancelled && job.Status != JobStatusTimeout {
			continue
		}

		// Check age
		if job.CompletedAt != nil && job.CompletedAt.Before(cutoff) {
			if err := d.etcd.Delete(ctx, key); err != nil {
				continue
			}
			deleted++

			// Clean up idempotency tracking
			if job.IdempotencyKey != "" {
				d.mu.Lock()
				delete(d.idempotency, job.IdempotencyKey)
				d.mu.Unlock()
			}
		}
	}

	return deleted, nil
}
