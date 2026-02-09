package rotation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// Scheduler manages scheduled credential rotation based on policies.
type Scheduler struct {
	mu            sync.RWMutex
	engine        *Engine
	policies      *PolicyEngine
	audit         credentials.AuditLogger
	store         credentials.CredentialStore
	checkInterval time.Duration
	running       bool
	cancel        context.CancelFunc
	completedJobs int
	failedJobs    int
	lastCheckTime time.Time
	nextCheckTime time.Time
}

// NewScheduler creates a new rotation scheduler.
func NewScheduler(
	engine *Engine,
	policies *PolicyEngine,
	store credentials.CredentialStore,
	audit credentials.AuditLogger,
) *Scheduler {
	return &Scheduler{
		engine:        engine,
		policies:      policies,
		store:         store,
		audit:         audit,
		checkInterval: time.Minute,
	}
}

// SetCheckInterval sets the interval for checking due rotations.
func (s *Scheduler) SetCheckInterval(interval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkInterval = interval
}

// Start starts the scheduler.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("scheduler already running")
	}

	var schedCtx context.Context
	schedCtx, s.cancel = context.WithCancel(ctx)
	s.running = true
	s.mu.Unlock()

	go s.run(schedCtx)
	return nil
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.cancel()
	s.running = false
}

// IsRunning returns whether the scheduler is running.
func (s *Scheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetStatus returns the current scheduler status.
func (s *Scheduler) GetStatus() *SchedulerStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &SchedulerStatus{
		Running:       s.running,
		PolicyCount:   len(s.policies.ListPolicies()),
		ActiveJobs:    len(s.engine.ListJobs()),
		CompletedJobs: s.completedJobs,
		FailedJobs:    s.failedJobs,
		LastCheckTime: s.lastCheckTime,
		NextCheckTime: s.nextCheckTime,
	}
}

// TriggerNow triggers immediate rotation for a specific credential.
func (s *Scheduler) TriggerNow(ctx context.Context, credentialID string) (*Result, error) {
	cred, err := s.store.Get(ctx, credentialID)
	if err != nil {
		return nil, fmt.Errorf("failed to get credential: %w", err)
	}

	job := &Job{
		ID:             fmt.Sprintf("manual-%s-%d", credentialID, time.Now().UnixNano()),
		CredentialID:   credentialID,
		CredentialType: cred.Type(),
		RollbackOnFail: true,
		OldCredential:  cred,
	}

	return s.engine.Rotate(ctx, job)
}

// AddPolicy adds a rotation policy to the scheduler.
func (s *Scheduler) AddPolicy(policy *Policy) error {
	// Validate cron schedule if provided
	if policy.Schedule != "" {
		if _, err := secrets.ParseCron(policy.Schedule); err != nil {
			return fmt.Errorf("invalid cron schedule: %w", err)
		}
	}
	return s.policies.AddPolicy(policy)
}

func (s *Scheduler) run(ctx context.Context) {
	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			return
		case <-ticker.C:
			s.checkDueRotations(ctx)
		}
	}
}

func (s *Scheduler) checkDueRotations(ctx context.Context) {
	now := time.Now()

	s.mu.Lock()
	s.lastCheckTime = now
	s.nextCheckTime = now.Add(s.checkInterval)
	s.mu.Unlock()

	jobs, err := s.policies.FindDueCredentials(ctx, s.store, now)
	if err != nil {
		s.logAudit(ctx, "scheduler_check_failed", false, err.Error())
		return
	}

	for _, job := range jobs {
		select {
		case <-ctx.Done():
			return
		default:
		}

		s.logAudit(ctx, "scheduler_rotation_start", true, fmt.Sprintf("rotating credential %s", job.CredentialID))

		result, err := s.engine.Rotate(ctx, job)

		s.mu.Lock()
		if err != nil || (result != nil && !result.Success) {
			s.failedJobs++
			errMsg := ""
			if err != nil {
				errMsg = err.Error()
			} else if result != nil {
				errMsg = result.Error
			}
			s.mu.Unlock()
			s.logAudit(ctx, "scheduler_rotation_failed", false, errMsg)
		} else {
			s.completedJobs++
			s.mu.Unlock()
			s.logAudit(ctx, "scheduler_rotation_completed", true, fmt.Sprintf("credential %s rotated", job.CredentialID))
		}
	}
}

func (s *Scheduler) logAudit(ctx context.Context, action string, success bool, message string) {
	if s.audit == nil {
		return
	}
	_ = s.audit.LogCredentialAccess(ctx, &credentials.CredentialAccessEvent{
		Action:    action,
		Timestamp: time.Now(),
		Success:   success,
		Error: func() string {
			if !success {
				return message
			}
			return ""
		}(),
		Extra: map[string]interface{}{
			"message": message,
		},
	})
}
