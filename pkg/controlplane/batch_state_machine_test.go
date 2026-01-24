package controlplane

import (
	"errors"
	"testing"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

func TestManagedBatchJob_InitialState(t *testing.T) {
	job := &BatchJob{
		ID:        "test-job-1",
		Target:    "role:webserver",
		Command:   "uptime",
		Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING,
		CreatedAt: time.Now(),
	}

	mbj := NewManagedBatchJob(job, nil)

	if mbj.Status() != pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING {
		t.Errorf("expected pending status, got %v", mbj.Status())
	}
	if !mbj.IsPending() {
		t.Error("expected IsPending() to be true")
	}
	if mbj.IsRunning() {
		t.Error("expected IsRunning() to be false")
	}
	if mbj.IsTerminal() {
		t.Error("expected IsTerminal() to be false")
	}
}

func TestManagedBatchJob_StartWorkflow(t *testing.T) {
	job := &BatchJob{
		ID:        "test-job-1",
		Target:    "role:webserver",
		Command:   "uptime",
		Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING,
		CreatedAt: time.Now(),
	}

	mbj := NewManagedBatchJob(job, nil)

	// Start job
	if !mbj.CanStart() {
		t.Error("expected CanStart() to be true")
	}
	if err := mbj.Start(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mbj.Status() != pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING {
		t.Errorf("expected running status, got %v", mbj.Status())
	}
	if !mbj.IsRunning() {
		t.Error("expected IsRunning() to be true")
	}
	if job.StartedAt == nil {
		t.Error("expected StartedAt to be set")
	}
}

func TestManagedBatchJob_CompleteWorkflow(t *testing.T) {
	job := &BatchJob{
		ID:        "test-job-1",
		Target:    "role:webserver",
		Command:   "uptime",
		Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING,
		CreatedAt: time.Now(),
	}

	mbj := NewManagedBatchJob(job, nil)

	// Start then complete
	mbj.Start()
	if err := mbj.Complete(95.5); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mbj.Status() != pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED {
		t.Errorf("expected completed status, got %v", mbj.Status())
	}
	if !mbj.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if !mbj.IsSuccessful() {
		t.Error("expected IsSuccessful() to be true")
	}
	if job.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestManagedBatchJob_FailWorkflow(t *testing.T) {
	job := &BatchJob{
		ID:        "test-job-1",
		Target:    "role:webserver",
		Command:   "uptime",
		Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING,
		CreatedAt: time.Now(),
	}

	mbj := NewManagedBatchJob(job, nil)

	// Start then fail
	mbj.Start()
	testErr := errors.New("execution failed")
	if err := mbj.Fail(testErr); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mbj.Status() != pb.BatchJobStatus_BATCH_JOB_STATUS_FAILED {
		t.Errorf("expected failed status, got %v", mbj.Status())
	}
	if !mbj.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if mbj.IsSuccessful() {
		t.Error("expected IsSuccessful() to be false")
	}
}

func TestManagedBatchJob_CancelFromPending(t *testing.T) {
	job := &BatchJob{
		ID:        "test-job-1",
		Target:    "role:webserver",
		Command:   "uptime",
		Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING,
		CreatedAt: time.Now(),
	}

	mbj := NewManagedBatchJob(job, nil)

	// Cancel from pending
	if !mbj.CanCancel() {
		t.Error("expected CanCancel() to be true")
	}
	if err := mbj.Cancel(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mbj.Status() != pb.BatchJobStatus_BATCH_JOB_STATUS_CANCELLED {
		t.Errorf("expected cancelled status, got %v", mbj.Status())
	}
	if !mbj.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
}

func TestManagedBatchJob_CancelFromRunning(t *testing.T) {
	job := &BatchJob{
		ID:        "test-job-1",
		Target:    "role:webserver",
		Command:   "uptime",
		Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING,
		CreatedAt: time.Now(),
	}

	mbj := NewManagedBatchJob(job, nil)

	// Start then cancel
	mbj.Start()
	if !mbj.CanCancel() {
		t.Error("expected CanCancel() to be true")
	}
	if err := mbj.Cancel(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mbj.Status() != pb.BatchJobStatus_BATCH_JOB_STATUS_CANCELLED {
		t.Errorf("expected cancelled status, got %v", mbj.Status())
	}
}

func TestManagedBatchJob_InvalidTransitions(t *testing.T) {
	job := &BatchJob{
		ID:        "test-job-1",
		Target:    "role:webserver",
		Command:   "uptime",
		Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING,
		CreatedAt: time.Now(),
	}

	mbj := NewManagedBatchJob(job, nil)

	// Cannot complete from pending
	err := mbj.Complete(100.0)
	if err == nil {
		t.Error("expected error for invalid transition")
	}
	if !errors.Is(err, statemachine.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}

	// Status should not have changed
	if mbj.Status() != pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING {
		t.Errorf("status should not have changed, got %v", mbj.Status())
	}
}

func TestManagedBatchJob_CannotStartFromTerminal(t *testing.T) {
	job := &BatchJob{
		ID:        "test-job-1",
		Target:    "role:webserver",
		Command:   "uptime",
		Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING,
		CreatedAt: time.Now(),
	}

	mbj := NewManagedBatchJob(job, nil)

	// Complete the job
	mbj.Start()
	mbj.Complete(100.0)

	// Cannot start from completed
	if mbj.CanStart() {
		t.Error("expected CanStart() to be false from terminal state")
	}
	err := mbj.Start()
	if err == nil {
		t.Error("expected error when starting from terminal state")
	}
}

func TestManagedBatchJob_Callbacks(t *testing.T) {
	var startedCalls, completedCalls, failedCalls, cancelledCalls int
	var lastStartedID, lastCompletedID string
	var lastSuccessRate float32
	var lastFailedError error

	callbacks := &BatchJobCallbacks{
		OnStarted: func(jobID string) {
			startedCalls++
			lastStartedID = jobID
		},
		OnCompleted: func(jobID string, successRate float32) {
			completedCalls++
			lastCompletedID = jobID
			lastSuccessRate = successRate
		},
		OnFailed: func(jobID string, err error) {
			failedCalls++
			lastFailedError = err
		},
		OnCancelled: func(jobID string) {
			cancelledCalls++
		},
	}

	job := &BatchJob{
		ID:        "test-job-1",
		Target:    "role:webserver",
		Command:   "uptime",
		Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING,
		CreatedAt: time.Now(),
	}

	mbj := NewManagedBatchJob(job, callbacks)

	// Start triggers callback
	mbj.Start()
	if startedCalls != 1 || lastStartedID != "test-job-1" {
		t.Errorf("expected OnStarted called once, got %d", startedCalls)
	}

	// Complete triggers callback
	mbj.Complete(95.5)
	if completedCalls != 1 || lastCompletedID != "test-job-1" {
		t.Errorf("expected OnCompleted called once, got %d", completedCalls)
	}
	if lastSuccessRate != 95.5 {
		t.Errorf("expected success rate 95.5, got %f", lastSuccessRate)
	}

	// Test failure callback
	job2 := &BatchJob{
		ID:        "test-job-2",
		Target:    "role:webserver",
		Command:   "uptime",
		Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING,
		CreatedAt: time.Now(),
	}
	mbj2 := NewManagedBatchJob(job2, callbacks)
	mbj2.Start()
	testErr := errors.New("test error")
	mbj2.Fail(testErr)
	if failedCalls != 1 || lastFailedError != testErr {
		t.Errorf("expected OnFailed called once with error, got %d calls", failedCalls)
	}

	// Test cancelled callback
	job3 := &BatchJob{
		ID:        "test-job-3",
		Target:    "role:webserver",
		Command:   "uptime",
		Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING,
		CreatedAt: time.Now(),
	}
	mbj3 := NewManagedBatchJob(job3, callbacks)
	mbj3.Cancel()
	if cancelledCalls != 1 {
		t.Errorf("expected OnCancelled called once, got %d", cancelledCalls)
	}
}

func TestManagedBatchJob_History(t *testing.T) {
	job := &BatchJob{
		ID:        "test-job-1",
		Target:    "role:webserver",
		Command:   "uptime",
		Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING,
		CreatedAt: time.Now(),
	}

	mbj := NewManagedBatchJob(job, nil)

	mbj.Start()
	mbj.Complete(100.0)

	history := mbj.History()
	if history == nil {
		t.Fatal("history should not be nil")
	}

	records := history.All()
	if len(records) != 2 {
		t.Errorf("expected 2 history records, got %d", len(records))
	}
}

func TestManagedBatchJob_AvailableEvents(t *testing.T) {
	job := &BatchJob{
		ID:        "test-job-1",
		Target:    "role:webserver",
		Command:   "uptime",
		Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING,
		CreatedAt: time.Now(),
	}

	mbj := NewManagedBatchJob(job, nil)

	// From pending, can start or cancel
	events := mbj.AvailableEvents()
	if len(events) != 2 {
		t.Errorf("expected 2 available events from pending, got %d", len(events))
	}

	mbj.Start()

	// From running, can complete, fail, or cancel
	events = mbj.AvailableEvents()
	if len(events) != 3 {
		t.Errorf("expected 3 available events from running, got %d", len(events))
	}
}

func TestManagedBatchJob_Duration(t *testing.T) {
	job := &BatchJob{
		ID:        "test-job-1",
		Target:    "role:webserver",
		Command:   "uptime",
		Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING,
		CreatedAt: time.Now(),
	}

	mbj := NewManagedBatchJob(job, nil)

	// No duration before start
	if mbj.Duration() != 0 {
		t.Error("expected 0 duration before start")
	}

	mbj.Start()

	// Duration should be non-zero while running
	time.Sleep(1 * time.Millisecond)
	runningDuration := mbj.Duration()
	if runningDuration == 0 {
		t.Error("expected non-zero duration while running")
	}

	mbj.Complete(100.0)

	// Duration should be fixed after completion
	finalDuration := mbj.Duration()
	if finalDuration < runningDuration {
		t.Error("expected final duration >= running duration")
	}
}

func TestManagedBatchJob_NilCallbacks(t *testing.T) {
	job := &BatchJob{
		ID:        "test-job-1",
		Target:    "role:webserver",
		Command:   "uptime",
		Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING,
		CreatedAt: time.Now(),
	}

	// Empty callbacks struct
	callbacks := &BatchJobCallbacks{}

	mbj := NewManagedBatchJob(job, callbacks)

	// These should not panic
	mbj.Start()
	mbj.Complete(100.0)
}

func TestBatchJobStatusToString(t *testing.T) {
	tests := []struct {
		status  pb.BatchJobStatus
		display string
	}{
		{pb.BatchJobStatus_BATCH_JOB_STATUS_UNSPECIFIED, "Unspecified"},
		{pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING, "Pending"},
		{pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING, "Running"},
		{pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED, "Completed"},
		{pb.BatchJobStatus_BATCH_JOB_STATUS_FAILED, "Failed"},
		{pb.BatchJobStatus_BATCH_JOB_STATUS_CANCELLED, "Cancelled"},
		{pb.BatchJobStatus(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.display, func(t *testing.T) {
			if got := BatchJobStatusToString(tt.status); got != tt.display {
				t.Errorf("BatchJobStatusToString(%v) = %v, want %v", tt.status, got, tt.display)
			}
		})
	}
}

func TestManagedBatchJob_JobStatusSync(t *testing.T) {
	job := &BatchJob{
		ID:        "test-job-1",
		Target:    "role:webserver",
		Command:   "uptime",
		Status:    pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING,
		CreatedAt: time.Now(),
	}

	mbj := NewManagedBatchJob(job, nil)

	// Verify job.Status is synced with state machine
	if job.Status != pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING {
		t.Errorf("expected job.Status to be pending, got %v", job.Status)
	}

	mbj.Start()
	if job.Status != pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING {
		t.Errorf("expected job.Status to be running, got %v", job.Status)
	}

	mbj.Complete(100.0)
	if job.Status != pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED {
		t.Errorf("expected job.Status to be completed, got %v", job.Status)
	}
}
