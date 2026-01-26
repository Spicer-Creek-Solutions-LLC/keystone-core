package statemgmt

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
)

func TestExecutor_ExecuteState(t *testing.T) {
	executor := NewExecutor()
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-executor-file.txt")
	os.Remove(tmpFile)
	defer os.Remove(tmpFile)

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     tmpFile,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "test content",
					},
				},
			},
		},
	}

	run, err := executor.ExecuteState(ctx, stateFile)
	if err != nil {
		t.Fatalf("ExecuteState failed: %v", err)
	}

	if run.Summary.Total != 1 {
		t.Errorf("Expected 1 state, got %d", run.Summary.Total)
	}

	if run.Summary.Succeeded != 1 {
		t.Errorf("Expected 1 succeeded, got %d", run.Summary.Succeeded)
	}

	if run.Summary.Failed != 0 {
		t.Errorf("Expected 0 failed, got %d", run.Summary.Failed)
	}

	if !run.Summary.Success {
		t.Error("Expected overall success")
	}

	// Verify file was created
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}

	if string(content) != "test content" {
		t.Errorf("Expected 'test content', got '%s'", string(content))
	}
}

func TestExecutor_DryRun(t *testing.T) {
	executor := NewExecutor()
	executor.DryRun = true
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-executor-dryrun.txt")
	os.Remove(tmpFile)
	defer os.Remove(tmpFile)

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     tmpFile,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "test content",
					},
				},
			},
		},
	}

	run, err := executor.ExecuteState(ctx, stateFile)
	if err != nil {
		t.Fatalf("ExecuteState failed: %v", err)
	}

	if run.Summary.Total != 1 {
		t.Errorf("Expected 1 state, got %d", run.Summary.Total)
	}

	if run.Summary.Changed != 1 {
		t.Errorf("Expected 1 changed (dry run), got %d", run.Summary.Changed)
	}

	// Verify file was NOT created (dry run)
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error("Expected file to not be created in dry run")
	}
}

func TestExecutor_Idempotency(t *testing.T) {
	executor := NewExecutor()
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-executor-idempotent.txt")
	// Create file with desired state
	if err := os.WriteFile(tmpFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(tmpFile)

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     tmpFile,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "test content",
					},
				},
			},
		},
	}

	run, err := executor.ExecuteState(ctx, stateFile)
	if err != nil {
		t.Fatalf("ExecuteState failed: %v", err)
	}

	if run.Summary.Changed != 0 {
		t.Errorf("Expected 0 changed (idempotent), got %d", run.Summary.Changed)
	}

	if run.Summary.Unchanged != 1 {
		t.Errorf("Expected 1 unchanged, got %d", run.Summary.Unchanged)
	}
}

func TestExecutor_MultipleStates(t *testing.T) {
	executor := NewExecutor()
	ctx := context.Background()

	tmpFile1 := filepath.Join(os.TempDir(), "test-executor-multi1.txt")
	tmpFile2 := filepath.Join(os.TempDir(), "test-executor-multi2.txt")
	os.Remove(tmpFile1)
	os.Remove(tmpFile2)
	defer os.Remove(tmpFile1)
	defer os.Remove(tmpFile2)

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     tmpFile1,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "file 1",
					},
				},
				{
					ID:     tmpFile2,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "file 2",
					},
				},
			},
		},
	}

	run, err := executor.ExecuteState(ctx, stateFile)
	if err != nil {
		t.Fatalf("ExecuteState failed: %v", err)
	}

	if run.Summary.Total != 2 {
		t.Errorf("Expected 2 states, got %d", run.Summary.Total)
	}

	if run.Summary.Succeeded != 2 {
		t.Errorf("Expected 2 succeeded, got %d", run.Summary.Succeeded)
	}

	// Verify both files were created
	content1, err := os.ReadFile(tmpFile1)
	if err != nil {
		t.Fatalf("Failed to read file 1: %v", err)
	}
	if string(content1) != "file 1" {
		t.Errorf("Expected 'file 1', got '%s'", string(content1))
	}

	content2, err := os.ReadFile(tmpFile2)
	if err != nil {
		t.Fatalf("Failed to read file 2: %v", err)
	}
	if string(content2) != "file 2" {
		t.Errorf("Expected 'file 2', got '%s'", string(content2))
	}
}

func TestExecutor_FailHard(t *testing.T) {
	executor := NewExecutor()
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-executor-failhard.txt")
	defer os.Remove(tmpFile)

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     tmpFile,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "test",
					},
					FailHard: true,
				},
			},
		},
	}

	// First run should succeed
	run, err := executor.ExecuteState(ctx, stateFile)
	if err != nil {
		t.Fatalf("ExecuteState failed: %v", err)
	}

	if !run.Summary.Success {
		t.Error("Expected success on first run")
	}
}

func TestExecutor_Retry(t *testing.T) {
	executor := NewExecutor()
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-executor-retry.txt")
	os.Remove(tmpFile)
	defer os.Remove(tmpFile)

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     tmpFile,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "test",
					},
					Retry: &RetryConfig{
						Attempts:          3,
						Delay:             100 * time.Millisecond,
						BackoffMultiplier: 1.5,
						MaxDelay:          1 * time.Second,
					},
				},
			},
		},
	}

	run, err := executor.ExecuteState(ctx, stateFile)
	if err != nil {
		t.Fatalf("ExecuteState failed: %v", err)
	}

	if !run.Summary.Success {
		t.Error("Expected success")
	}
}

func TestApplyState(t *testing.T) {
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-applystate.txt")
	os.Remove(tmpFile)
	defer os.Remove(tmpFile)

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     tmpFile,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "test",
					},
				},
			},
		},
	}

	run, err := ApplyState(ctx, stateFile, false)
	if err != nil {
		t.Fatalf("ApplyState failed: %v", err)
	}

	if !run.Summary.Success {
		t.Error("Expected success")
	}

	// Verify file was created
	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Error("Expected file to be created")
	}
}

func TestCheckState(t *testing.T) {
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-checkstate.txt")
	os.Remove(tmpFile)
	defer os.Remove(tmpFile)

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     tmpFile,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "test",
					},
				},
			},
		},
	}

	run, err := CheckState(ctx, stateFile)
	if err != nil {
		t.Fatalf("CheckState failed: %v", err)
	}

	if !run.DryRun {
		t.Error("Expected dry run mode")
	}

	// Verify file was NOT created
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error("Expected file to not be created in check mode")
	}
}

func TestExecutor_CalculateSummary(t *testing.T) {
	executor := NewExecutor()

	run := &StateRun{
		StartTime: time.Now(),
		Results: []*StateResult{
			{Success: true, Changed: true},
			{Success: true, Changed: false},
			{Success: false, Changed: false},
		},
	}
	run.EndTime = time.Now()

	summary := executor.calculateSummary(run)

	if summary.Total != 3 {
		t.Errorf("Expected 3 total, got %d", summary.Total)
	}

	if summary.Succeeded != 2 {
		t.Errorf("Expected 2 succeeded, got %d", summary.Succeeded)
	}

	if summary.Failed != 1 {
		t.Errorf("Expected 1 failed, got %d", summary.Failed)
	}

	if summary.Changed != 1 {
		t.Errorf("Expected 1 changed, got %d", summary.Changed)
	}

	if summary.Unchanged != 1 {
		t.Errorf("Expected 1 unchanged, got %d", summary.Unchanged)
	}

	if summary.Success {
		t.Error("Expected overall failure (has 1 failed)")
	}
}

func TestExecutor_EvaluateCondition(t *testing.T) {
	executor := NewExecutor()
	ctx := context.Background()

	tests := []struct {
		name      string
		condition string
		expected  bool
	}{
		{
			name:      "true command",
			condition: "true",
			expected:  true,
		},
		{
			name:      "false command",
			condition: "false",
			expected:  false,
		},
		{
			name:      "echo succeeds",
			condition: "echo hello",
			expected:  true,
		},
		{
			name:      "nonexistent command fails",
			condition: "nonexistent-command-12345",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executor.evaluateCondition(ctx, tt.condition)
			if result != tt.expected {
				t.Errorf("evaluateCondition(%q) = %v, want %v", tt.condition, result, tt.expected)
			}
		})
	}
}

func TestExecutor_CalculateSummaryAllSuccess(t *testing.T) {
	executor := NewExecutor()

	run := &StateRun{
		StartTime: time.Now(),
		Results: []*StateResult{
			{Success: true, Changed: true},
			{Success: true, Changed: true},
		},
	}
	run.EndTime = time.Now()

	summary := executor.calculateSummary(run)

	if summary.Total != 2 {
		t.Errorf("Expected 2 total, got %d", summary.Total)
	}

	if summary.Succeeded != 2 {
		t.Errorf("Expected 2 succeeded, got %d", summary.Succeeded)
	}

	if summary.Failed != 0 {
		t.Errorf("Expected 0 failed, got %d", summary.Failed)
	}

	if !summary.Success {
		t.Error("Expected overall success")
	}
}

func TestExecutor_CalculateSummaryEmpty(t *testing.T) {
	executor := NewExecutor()

	run := &StateRun{
		StartTime: time.Now(),
		Results:   []*StateResult{},
	}
	run.EndTime = time.Now()

	summary := executor.calculateSummary(run)

	if summary.Total != 0 {
		t.Errorf("Expected 0 total, got %d", summary.Total)
	}

	if !summary.Success {
		t.Error("Expected success for empty results")
	}
}

type mockEventPublisher struct {
	events []*events.Event
}

func (m *mockEventPublisher) Publish(event *events.Event) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventPublisher) PublishAsync(event *events.Event) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventPublisher) Close() error {
	return nil
}

type fakeModule struct {
	name         string
	checkResult  *ModuleCheckResult
	checkErr     error
	applyResults []*StateResult
	applyErrs    []error
	applyCalls   int
}

func (m *fakeModule) Name() string {
	return m.name
}

func (m *fakeModule) ValidStates() []string {
	return []string{"present"}
}

func (m *fakeModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if m.checkErr != nil {
		return nil, m.checkErr
	}
	if m.checkResult != nil {
		return m.checkResult, nil
	}
	return &ModuleCheckResult{Matches: true}, nil
}

func (m *fakeModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	callIndex := m.applyCalls
	m.applyCalls++

	if callIndex < len(m.applyErrs) && m.applyErrs[callIndex] != nil {
		return nil, m.applyErrs[callIndex]
	}
	if callIndex < len(m.applyResults) && m.applyResults[callIndex] != nil {
		return m.applyResults[callIndex], nil
	}
	return &StateResult{
		StateID: decl.ID,
		Module:  decl.Module,
		Success: true,
	}, nil
}

func (m *fakeModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	return true, nil
}

func TestExecutor_EmitEvent_CorrelationID(t *testing.T) {
	mock := &mockEventPublisher{}
	executor := &Executor{
		EventPublisher: mock,
		EventSource:    "",
	}

	executor.emitEvent(events.EventTypeStateApplyStart, events.SeverityInfo, map[string]interface{}{
		"run_id": "run-123",
	})

	if len(mock.events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(mock.events))
	}

	event := mock.events[0]
	if event.CorrelationID != "run-123" {
		t.Errorf("Expected correlation_id to be run-123, got %q", event.CorrelationID)
	}
	if event.Source != "/state-manager" {
		t.Errorf("Expected default source, got %q", event.Source)
	}
}

func TestExecutor_DryRun_CheckError(t *testing.T) {
	registry := NewModuleRegistry()
	module := &fakeModule{
		name:     "fake",
		checkErr: os.ErrNotExist,
	}
	if err := registry.Register(module); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	executor := &Executor{
		Registry: registry,
		DryRun:   true,
	}

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"fake": {
				{
					ID:     "fake-id",
					Module: "fake",
					State:  "present",
				},
			},
		},
	}

	run, err := executor.ExecuteState(context.Background(), stateFile)
	if err != nil {
		t.Fatalf("ExecuteState returned error: %v", err)
	}

	if run.Summary.Failed != 1 {
		t.Errorf("Expected 1 failed state, got %d", run.Summary.Failed)
	}
}

func TestExecutor_Retry_SucceedsAfterFailure(t *testing.T) {
	registry := NewModuleRegistry()
	module := &fakeModule{
		name: "fake",
		applyErrs: []error{
			os.ErrInvalid,
			nil,
		},
		applyResults: []*StateResult{
			nil,
			{
				StateID: "fake-id",
				Module:  "fake",
				Success: true,
				Changed: true,
			},
		},
	}
	if err := registry.Register(module); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	executor := &Executor{
		Registry: registry,
	}

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"fake": {
				{
					ID:     "fake-id",
					Module: "fake",
					State:  "present",
					Retry: &RetryConfig{
						Attempts:          2,
						Delay:             time.Millisecond,
						BackoffMultiplier: 1,
					},
				},
			},
		},
	}

	run, err := executor.ExecuteState(context.Background(), stateFile)
	if err != nil {
		t.Fatalf("ExecuteState returned error: %v", err)
	}

	if module.applyCalls != 2 {
		t.Errorf("Expected 2 apply calls, got %d", module.applyCalls)
	}
	if run.Summary.Failed != 0 {
		t.Errorf("Expected 0 failed states, got %d", run.Summary.Failed)
	}
}

func TestExecutor_FailHard_ModuleMissing(t *testing.T) {
	executor := &Executor{
		Registry: NewModuleRegistry(),
	}

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"missing": {
				{
					ID:       "missing-id",
					Module:   "missing",
					State:    "present",
					FailHard: true,
				},
			},
		},
	}

	run, err := executor.ExecuteState(context.Background(), stateFile)
	if err == nil {
		t.Fatal("Expected ExecuteState to fail for missing module with fail_hard")
	}

	if run.Summary.Failed != 1 {
		t.Errorf("Expected 1 failed state, got %d", run.Summary.Failed)
	}
}

func TestExecutor_SkipUnless(t *testing.T) {
	registry := NewModuleRegistry()
	module := &fakeModule{name: "fake"}
	if err := registry.Register(module); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	executor := &Executor{
		Registry: registry,
	}

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"fake": {
				{
					ID:     "skip-unless",
					Module: "fake",
					State:  "present",
					Unless: "true",
				},
			},
		},
	}

	run, err := executor.ExecuteState(context.Background(), stateFile)
	if err != nil {
		t.Fatalf("ExecuteState returned error: %v", err)
	}

	if module.applyCalls != 0 {
		t.Errorf("Expected no apply calls, got %d", module.applyCalls)
	}
	if run.Summary.Unchanged != 1 {
		t.Errorf("Expected 1 unchanged state, got %d", run.Summary.Unchanged)
	}
}

func TestExecutor_SkipOnlyIf(t *testing.T) {
	registry := NewModuleRegistry()
	module := &fakeModule{name: "fake"}
	if err := registry.Register(module); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	executor := &Executor{
		Registry: registry,
	}

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"fake": {
				{
					ID:     "skip-onlyif",
					Module: "fake",
					State:  "present",
					OnlyIf: "false",
				},
			},
		},
	}

	run, err := executor.ExecuteState(context.Background(), stateFile)
	if err != nil {
		t.Fatalf("ExecuteState returned error: %v", err)
	}

	if module.applyCalls != 0 {
		t.Errorf("Expected no apply calls, got %d", module.applyCalls)
	}
	if run.Summary.Unchanged != 1 {
		t.Errorf("Expected 1 unchanged state, got %d", run.Summary.Unchanged)
	}
}
