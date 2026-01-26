package execution

import (
	"errors"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

func TestManagedExecution_InitialState(t *testing.T) {
	req := &ExecuteRequest{
		CommandID: "test-cmd-1",
		Command:   "echo hello",
	}

	me := NewManagedExecution(req, 3, nil)

	if me.State() != ExecutionStatePending {
		t.Errorf("expected pending state, got %v", me.State())
	}
	if !me.IsPending() {
		t.Error("expected IsPending() to be true")
	}
	if me.IsRunning() {
		t.Error("expected IsRunning() to be false")
	}
	if me.IsTerminal() {
		t.Error("expected IsTerminal() to be false")
	}
	if me.Attempt() != 0 {
		t.Errorf("expected attempt to be 0, got %d", me.Attempt())
	}
	if me.MaxAttempts() != 3 {
		t.Errorf("expected maxAttempts to be 3, got %d", me.MaxAttempts())
	}
}

func TestManagedExecution_StartWorkflow(t *testing.T) {
	req := &ExecuteRequest{
		CommandID: "test-cmd-1",
		Command:   "echo hello",
	}

	me := NewManagedExecution(req, 3, nil)

	// Start
	if err := me.Start(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if me.State() != ExecutionStateRunning {
		t.Errorf("expected running state, got %v", me.State())
	}
	if !me.IsRunning() {
		t.Error("expected IsRunning() to be true")
	}
	if me.Attempt() != 1 {
		t.Errorf("expected attempt to be 1, got %d", me.Attempt())
	}
	if me.Duration() == 0 {
		t.Error("expected non-zero duration after start")
	}
}

func TestManagedExecution_CompleteWorkflow(t *testing.T) {
	req := &ExecuteRequest{
		CommandID: "test-cmd-1",
		Command:   "echo hello",
	}

	me := NewManagedExecution(req, 3, nil)

	me.Start()

	// Complete
	stdout := []byte("hello\n")
	stderr := []byte("")
	if err := me.Complete(stdout, stderr, 0); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if me.State() != ExecutionStateCompleted {
		t.Errorf("expected completed state, got %v", me.State())
	}
	if !me.IsCompleted() {
		t.Error("expected IsCompleted() to be true")
	}
	if !me.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if !me.IsSuccessful() {
		t.Error("expected IsSuccessful() to be true")
	}
	if string(me.Result.Stdout) != "hello\n" {
		t.Errorf("expected stdout to be 'hello\\n', got %s", me.Result.Stdout)
	}
}

func TestManagedExecution_FailWorkflow(t *testing.T) {
	req := &ExecuteRequest{
		CommandID: "test-cmd-1",
		Command:   "bad command",
	}

	me := NewManagedExecution(req, 3, nil)

	me.Start()

	// Fail
	testErr := errors.New("command not found")
	if err := me.Fail(testErr); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if me.State() != ExecutionStateFailed {
		t.Errorf("expected failed state, got %v", me.State())
	}
	if !me.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if me.IsSuccessful() {
		t.Error("expected IsSuccessful() to be false")
	}
	if me.Result.Error.Error() != "command not found" {
		t.Errorf("expected Error message, got %v", me.Result.Error)
	}
}

func TestManagedExecution_TimeoutWorkflow(t *testing.T) {
	req := &ExecuteRequest{
		CommandID: "test-cmd-1",
		Command:   "sleep 100",
	}

	me := NewManagedExecution(req, 3, nil)

	me.Start()

	// Timeout
	if err := me.Timeout(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if me.State() != ExecutionStateTimeout {
		t.Errorf("expected timeout state, got %v", me.State())
	}
	if !me.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
}

func TestManagedExecution_CancelWorkflow(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*ManagedExecution)
	}{
		{"cancel from pending", func(me *ManagedExecution) {}},
		{"cancel from running", func(me *ManagedExecution) { me.Start() }},
		{"cancel from retrying", func(me *ManagedExecution) { me.Start(); me.RequestRetry() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ExecuteRequest{
				CommandID: "test-cmd-1",
				Command:   "echo hello",
			}

			me := NewManagedExecution(req, 3, nil)
			tt.setup(me)

			if err := me.Cancel(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if me.State() != ExecutionStateCancelled {
				t.Errorf("expected cancelled state, got %v", me.State())
			}
			if !me.IsTerminal() {
				t.Error("expected IsTerminal() to be true")
			}
		})
	}
}

func TestManagedExecution_RetryWorkflow(t *testing.T) {
	req := &ExecuteRequest{
		CommandID: "test-cmd-1",
		Command:   "failing command",
	}

	me := NewManagedExecution(req, 3, nil)

	// Start first attempt
	me.Start()
	if me.Attempt() != 1 {
		t.Errorf("expected attempt 1, got %d", me.Attempt())
	}

	// Request retry
	if !me.CanRetry() {
		t.Error("expected CanRetry() to be true")
	}
	if err := me.RequestRetry(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if me.State() != ExecutionStateRetrying {
		t.Errorf("expected retrying state, got %v", me.State())
	}
	if !me.IsRetrying() {
		t.Error("expected IsRetrying() to be true")
	}

	// Retry
	if err := me.Retry(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if me.State() != ExecutionStateRunning {
		t.Errorf("expected running state, got %v", me.State())
	}
	if me.Attempt() != 2 {
		t.Errorf("expected attempt 2, got %d", me.Attempt())
	}

	// Another retry
	me.RequestRetry()
	me.Retry()
	if me.Attempt() != 3 {
		t.Errorf("expected attempt 3, got %d", me.Attempt())
	}

	// Should not be able to retry anymore
	if me.CanRetry() {
		t.Error("expected CanRetry() to be false at max attempts")
	}
}

func TestManagedExecution_MaxRetriesExceeded(t *testing.T) {
	req := &ExecuteRequest{
		CommandID: "test-cmd-1",
		Command:   "failing command",
	}

	me := NewManagedExecution(req, 2, nil)

	// Start first attempt and request retry
	me.Start()
	me.RequestRetry()
	me.Retry()

	// Second attempt - this is last
	me.RequestRetry()

	// Max retries exceeded
	if err := me.FailMaxRetries(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if me.State() != ExecutionStateFailed {
		t.Errorf("expected failed state, got %v", me.State())
	}
	if !me.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
}

func TestManagedExecution_InvalidTransitions(t *testing.T) {
	req := &ExecuteRequest{
		CommandID: "test-cmd-1",
		Command:   "echo hello",
	}

	me := NewManagedExecution(req, 3, nil)

	// Cannot complete from pending
	err := me.Complete(nil, nil, 0)
	if err == nil {
		t.Error("expected error for invalid transition")
	}
	if !errors.Is(err, statemachine.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}

	// State should not have changed
	if me.State() != ExecutionStatePending {
		t.Errorf("state should not have changed, got %v", me.State())
	}
}

func TestManagedExecution_Callbacks(t *testing.T) {
	var startedCalls, completedCalls, failedCalls, cancelledCalls, timeoutCalls int
	var retryingCalls, retryCalls int
	var lastRetryAttempt int

	callbacks := &ExecutionCallbacks{
		OnStarted: func(commandID string) {
			startedCalls++
		},
		OnCompleted: func(commandID string, result *CommandResult) {
			completedCalls++
		},
		OnFailed: func(commandID string, err error) {
			failedCalls++
		},
		OnTimeout: func(commandID string) {
			timeoutCalls++
		},
		OnCancelled: func(commandID string) {
			cancelledCalls++
		},
		OnRetrying: func(commandID string, attempt int, maxAttempts int) {
			retryingCalls++
		},
		OnRetry: func(commandID string, attempt int) {
			retryCalls++
			lastRetryAttempt = attempt
		},
	}

	// Test start callback
	req := &ExecuteRequest{CommandID: "test-cmd-1", Command: "echo hello"}
	me := NewManagedExecution(req, 3, callbacks)
	me.Start()
	if startedCalls != 1 {
		t.Errorf("expected OnStarted called once, got %d", startedCalls)
	}

	// Test complete callback
	me.Complete(nil, nil, 0)
	if completedCalls != 1 {
		t.Errorf("expected OnCompleted called once, got %d", completedCalls)
	}

	// Test retry callbacks
	req2 := &ExecuteRequest{CommandID: "test-cmd-2", Command: "failing"}
	me2 := NewManagedExecution(req2, 3, callbacks)
	me2.Start()
	me2.RequestRetry()
	if retryingCalls != 1 {
		t.Errorf("expected OnRetrying called once, got %d", retryingCalls)
	}

	me2.Retry()
	if retryCalls != 1 || lastRetryAttempt != 2 {
		t.Errorf("expected OnRetry called with attempt 2, got %d calls, attempt %d", retryCalls, lastRetryAttempt)
	}
}

func TestManagedExecution_History(t *testing.T) {
	req := &ExecuteRequest{
		CommandID: "test-cmd-1",
		Command:   "echo hello",
	}

	me := NewManagedExecution(req, 3, nil)

	me.Start()
	me.RequestRetry()
	me.Retry()
	me.Complete(nil, nil, 0)

	history := me.History()
	if history == nil {
		t.Fatal("history should not be nil")
	}

	records := history.All()
	if len(records) != 4 {
		t.Errorf("expected 4 history records, got %d", len(records))
	}
}

func TestManagedExecution_AvailableEvents(t *testing.T) {
	req := &ExecuteRequest{
		CommandID: "test-cmd-1",
		Command:   "echo hello",
	}

	me := NewManagedExecution(req, 3, nil)

	// From pending, can start or cancel
	events := me.AvailableEvents()
	if len(events) != 2 {
		t.Errorf("expected 2 available events from pending, got %d", len(events))
	}

	me.Start()

	// From running, can complete, fail, timeout, cancel, or request retry
	events = me.AvailableEvents()
	if len(events) != 5 {
		t.Errorf("expected 5 available events from running, got %d", len(events))
	}

	me.RequestRetry()

	// From retrying, can retry, cancel, or fail with max retries
	events = me.AvailableEvents()
	if len(events) != 3 {
		t.Errorf("expected 3 available events from retrying, got %d", len(events))
	}
}

func TestManagedExecution_Duration(t *testing.T) {
	req := &ExecuteRequest{
		CommandID: "test-cmd-1",
		Command:   "echo hello",
	}

	me := NewManagedExecution(req, 3, nil)

	// No duration before start
	if me.Duration() != 0 {
		t.Error("expected 0 duration before start")
	}

	me.Start()
	if err := helpers.WaitForTimeout(100*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return me.Duration() > 0, nil
	}); err != nil {
		t.Fatalf("expected duration to start: %v", err)
	}

	// Duration should be non-zero while running
	runningDuration := me.Duration()
	if runningDuration == 0 {
		t.Error("expected non-zero duration while running")
	}

	me.Complete(nil, nil, 0)

	// Duration should be fixed after completion
	finalDuration := me.Duration()
	if finalDuration < runningDuration {
		t.Error("expected final duration >= running duration")
	}
}

func TestManagedExecution_NilCallbacks(t *testing.T) {
	req := &ExecuteRequest{
		CommandID: "test-cmd-1",
		Command:   "echo hello",
	}

	// Empty callbacks struct
	callbacks := &ExecutionCallbacks{}

	me := NewManagedExecution(req, 3, callbacks)

	// These should not panic
	me.Start()
	me.RequestRetry()
	me.Retry()
	me.Complete(nil, nil, 0)
}

func TestExecutionStateToString(t *testing.T) {
	tests := []struct {
		state   ExecutionState
		display string
	}{
		{ExecutionStatePending, "Pending"},
		{ExecutionStateRunning, "Running"},
		{ExecutionStateRetrying, "Retrying"},
		{ExecutionStateCompleted, "Completed"},
		{ExecutionStateFailed, "Failed"},
		{ExecutionStateCancelled, "Cancelled"},
		{ExecutionStateTimeout, "Timeout"},
		{ExecutionState("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := ExecutionStateToString(tt.state); got != tt.display {
				t.Errorf("ExecutionStateToString(%v) = %v, want %v", tt.state, got, tt.display)
			}
		})
	}
}

func TestManagedExecution_FullRetryThenSuccess(t *testing.T) {
	req := &ExecuteRequest{
		CommandID: "test-cmd-1",
		Command:   "flaky command",
	}

	me := NewManagedExecution(req, 3, nil)

	// First attempt fails
	me.Start()
	if me.Attempt() != 1 {
		t.Error("expected attempt 1")
	}

	// Request and do retry
	me.RequestRetry()
	me.Retry()
	if me.Attempt() != 2 {
		t.Error("expected attempt 2")
	}

	// Second attempt succeeds
	me.Complete([]byte("success"), nil, 0)
	if !me.IsSuccessful() {
		t.Error("expected successful")
	}
	if me.Attempt() != 2 {
		t.Errorf("expected 2 attempts, got %d", me.Attempt())
	}
}
