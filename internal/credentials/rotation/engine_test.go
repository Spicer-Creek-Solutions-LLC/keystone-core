package rotation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
)

// mockProvider implements Provider for testing.
type mockProvider struct {
	mu             sync.Mutex
	supportedTypes []credentials.CredentialType
	validateErr    error
	generateErr    error
	applyErr       error
	verifyErr      error
	rollbackErr    error
	cleanupErr     error
	generated      credentials.Credential
	calls          []string
}

func newMockProvider(types ...credentials.CredentialType) *mockProvider {
	return &mockProvider{
		supportedTypes: types,
		calls:          make([]string, 0),
	}
}

func (m *mockProvider) SupportsType(credType credentials.CredentialType) bool {
	for _, t := range m.supportedTypes {
		if t == credType {
			return true
		}
	}
	return false
}

func (m *mockProvider) ValidateOld(_ context.Context, _ DeviceInfo, _ credentials.Credential) error {
	m.mu.Lock()
	m.calls = append(m.calls, "validate_old")
	m.mu.Unlock()
	return m.validateErr
}

func (m *mockProvider) Generate(_ context.Context, _ DeviceInfo, _ credentials.Credential) (credentials.Credential, error) {
	m.mu.Lock()
	m.calls = append(m.calls, "generate")
	m.mu.Unlock()
	if m.generateErr != nil {
		return nil, m.generateErr
	}
	if m.generated != nil {
		return m.generated, nil
	}
	return &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "new-cred",
			CredentialType: credentials.CredentialTypeSSHPassword,
		},
		Username: "admin",
		Password: "new-password-123",
	}, nil
}

func (m *mockProvider) Apply(_ context.Context, _ DeviceInfo, _, _ credentials.Credential) error {
	m.mu.Lock()
	m.calls = append(m.calls, "apply")
	m.mu.Unlock()
	return m.applyErr
}

func (m *mockProvider) Verify(_ context.Context, _ DeviceInfo, _ credentials.Credential) error {
	m.mu.Lock()
	m.calls = append(m.calls, "verify")
	m.mu.Unlock()
	return m.verifyErr
}

func (m *mockProvider) Rollback(_ context.Context, _ DeviceInfo, _, _ credentials.Credential) error {
	m.mu.Lock()
	m.calls = append(m.calls, "rollback")
	m.mu.Unlock()
	return m.rollbackErr
}

func (m *mockProvider) Cleanup(_ context.Context, _ DeviceInfo, _ credentials.Credential) error {
	m.mu.Lock()
	m.calls = append(m.calls, "cleanup")
	m.mu.Unlock()
	return m.cleanupErr
}

func (m *mockProvider) getCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.calls))
	copy(result, m.calls)
	return result
}

var _ Provider = (*mockProvider)(nil)

func makeTestJob() *Job {
	return &Job{
		ID:             "test-rotation-1",
		DeviceID:       "device-1",
		CredentialID:   "cred-1",
		CredentialType: credentials.CredentialTypeSSHPassword,
		ProxyAgentID:   "proxy-1",
		Device: DeviceInfo{
			ID:   "device-1",
			Host: "10.0.0.1",
			Port: 22,
		},
		RollbackOnFail: true,
		OldCredential: &credentials.SSHPasswordCredential{
			BaseCredential: credentials.BaseCredential{
				CredentialID:   "cred-1",
				CredentialType: credentials.CredentialTypeSSHPassword,
			},
			Username: "admin",
			Password: "old-password",
		},
	}
}

func TestNewEngine(t *testing.T) {
	store := credentials.NewInMemoryCredentialStore()
	audit := credentials.NewInMemoryAuditLogger(nil)
	engine := NewEngine(store, audit)

	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
	if engine.store != store {
		t.Error("store not set")
	}
	if engine.audit != audit {
		t.Error("audit logger not set")
	}
}

func TestEngine_RegisterProvider(t *testing.T) {
	engine := NewEngine(credentials.NewInMemoryCredentialStore(), nil)
	provider := newMockProvider(credentials.CredentialTypeSSHPassword)

	engine.RegisterProvider(provider)

	engine.mu.RLock()
	defer engine.mu.RUnlock()
	if len(engine.providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(engine.providers))
	}
}

func TestEngine_RotateSuccess(t *testing.T) {
	store := credentials.NewInMemoryCredentialStore()
	audit := credentials.NewInMemoryAuditLogger(nil)
	engine := NewEngine(store, audit)

	provider := newMockProvider(credentials.CredentialTypeSSHPassword)
	engine.RegisterProvider(provider)

	job := makeTestJob()
	result, err := engine.Rotate(context.Background(), job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.RolledBack {
		t.Error("should not have rolled back")
	}
	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}
	if job.State != CredRotationCompleted {
		t.Errorf("expected state %s, got %s", CredRotationCompleted, job.State)
	}

	// Verify all steps were called
	calls := provider.getCalls()
	expected := []string{"validate_old", "generate", "apply", "verify", "cleanup"}
	if len(calls) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(calls), calls)
	}
	for i, c := range expected {
		if calls[i] != c {
			t.Errorf("call %d: expected %s, got %s", i, c, calls[i])
		}
	}

	// Verify new credential was stored
	ctx := context.Background()
	cred, err := store.Get(ctx, "new-cred")
	if err != nil {
		t.Fatalf("new credential not stored: %v", err)
	}
	if cred.Type() != credentials.CredentialTypeSSHPassword {
		t.Errorf("stored credential type: expected %s, got %s", credentials.CredentialTypeSSHPassword, cred.Type())
	}

	// Verify audit events
	events := audit.GetEvents()
	if len(events) == 0 {
		t.Fatal("expected audit events")
	}
	// Should have: start, validated, generated, applied, verified, stored, completed
	if len(events) < 7 {
		t.Errorf("expected at least 7 audit events, got %d", len(events))
	}
}

func TestEngine_NoProviderError(t *testing.T) {
	engine := NewEngine(credentials.NewInMemoryCredentialStore(), nil)

	job := makeTestJob()
	_, err := engine.Rotate(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
	if err.Error() != "no provider supports credential type ssh_password" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEngine_DuplicateJobError(t *testing.T) {
	store := credentials.NewInMemoryCredentialStore()
	engine := NewEngine(store, nil)

	provider := newMockProvider(credentials.CredentialTypeSSHPassword)
	// Make generate block so the job stays active
	blockCh := make(chan struct{})
	provider.generateErr = nil
	origGenerate := provider.Generate
	_ = origGenerate

	engine.RegisterProvider(provider)

	// Manually insert a job to simulate active
	engine.mu.Lock()
	engine.jobs["test-rotation-1"] = &managedJob{
		job:     makeTestJob(),
		machine: buildRotationMachine(),
	}
	engine.mu.Unlock()

	job := makeTestJob()
	_, err := engine.Rotate(context.Background(), job)
	close(blockCh)
	if err == nil {
		t.Fatal("expected duplicate job error")
	}
}

func TestEngine_ValidateFailure(t *testing.T) {
	store := credentials.NewInMemoryCredentialStore()
	audit := credentials.NewInMemoryAuditLogger(nil)
	engine := NewEngine(store, audit)

	provider := newMockProvider(credentials.CredentialTypeSSHPassword)
	provider.validateErr = errors.New("connection refused")
	engine.RegisterProvider(provider)

	job := makeTestJob()
	result, err := engine.Rotate(context.Background(), job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Success {
		t.Error("expected failure")
	}
	if job.State != CredRotationFailed {
		t.Errorf("expected state %s, got %s", CredRotationFailed, job.State)
	}

	// Should not have called generate, apply, etc.
	calls := provider.getCalls()
	if len(calls) != 1 {
		t.Errorf("expected 1 call (validate_old), got %d: %v", len(calls), calls)
	}
}

func TestEngine_GenerateFailure(t *testing.T) {
	engine := NewEngine(credentials.NewInMemoryCredentialStore(), nil)

	provider := newMockProvider(credentials.CredentialTypeSSHPassword)
	provider.generateErr = errors.New("key generation failed")
	engine.RegisterProvider(provider)

	job := makeTestJob()
	result, _ := engine.Rotate(context.Background(), job)

	if result.Success {
		t.Error("expected failure")
	}
	calls := provider.getCalls()
	if len(calls) != 2 { // validate_old, generate
		t.Errorf("expected 2 calls, got %d: %v", len(calls), calls)
	}
}

func TestEngine_ApplyFailureWithRollback(t *testing.T) {
	store := credentials.NewInMemoryCredentialStore()
	audit := credentials.NewInMemoryAuditLogger(nil)
	engine := NewEngine(store, audit)

	provider := newMockProvider(credentials.CredentialTypeSSHPassword)
	provider.applyErr = errors.New("device rejected config")
	engine.RegisterProvider(provider)

	job := makeTestJob()
	job.RollbackOnFail = true
	result, _ := engine.Rotate(context.Background(), job)

	if result.Success {
		t.Error("expected failure")
	}
	if !result.RolledBack {
		t.Error("expected rollback")
	}
	if job.State != CredRotationRolledBack {
		t.Errorf("expected state %s, got %s", CredRotationRolledBack, job.State)
	}

	calls := provider.getCalls()
	// validate_old, generate, apply, rollback
	if len(calls) != 4 {
		t.Errorf("expected 4 calls, got %d: %v", len(calls), calls)
	}
	if calls[3] != "rollback" {
		t.Errorf("expected rollback call, got %s", calls[3])
	}
}

func TestEngine_ApplyFailureNoRollback(t *testing.T) {
	engine := NewEngine(credentials.NewInMemoryCredentialStore(), nil)

	provider := newMockProvider(credentials.CredentialTypeSSHPassword)
	provider.applyErr = errors.New("device rejected config")
	engine.RegisterProvider(provider)

	job := makeTestJob()
	job.RollbackOnFail = false
	result, _ := engine.Rotate(context.Background(), job)

	if result.Success {
		t.Error("expected failure")
	}
	if result.RolledBack {
		t.Error("should not have rolled back")
	}
	if job.State != CredRotationFailed {
		t.Errorf("expected state %s, got %s", CredRotationFailed, job.State)
	}
}

func TestEngine_VerifyFailureWithRollback(t *testing.T) {
	engine := NewEngine(credentials.NewInMemoryCredentialStore(), nil)

	provider := newMockProvider(credentials.CredentialTypeSSHPassword)
	provider.verifyErr = errors.New("new credential rejected")
	engine.RegisterProvider(provider)

	job := makeTestJob()
	job.RollbackOnFail = true
	result, _ := engine.Rotate(context.Background(), job)

	if result.Success {
		t.Error("expected failure")
	}
	if !result.RolledBack {
		t.Error("expected rollback")
	}

	calls := provider.getCalls()
	// validate_old, generate, apply, verify, rollback
	if len(calls) != 5 {
		t.Errorf("expected 5 calls, got %d: %v", len(calls), calls)
	}
}

func TestEngine_RollbackAlsoFails(t *testing.T) {
	audit := credentials.NewInMemoryAuditLogger(nil)
	engine := NewEngine(credentials.NewInMemoryCredentialStore(), audit)

	provider := newMockProvider(credentials.CredentialTypeSSHPassword)
	provider.applyErr = errors.New("apply failed")
	provider.rollbackErr = errors.New("rollback failed too")
	engine.RegisterProvider(provider)

	job := makeTestJob()
	job.RollbackOnFail = true
	result, _ := engine.Rotate(context.Background(), job)

	if result.Success {
		t.Error("expected failure")
	}
	if result.RolledBack {
		t.Error("rollback should have failed")
	}
	if job.State != CredRotationFailed {
		t.Errorf("expected state %s, got %s", CredRotationFailed, job.State)
	}

	// Verify audit logged both failures
	events := audit.GetEventsFiltered(&credentials.AuditFilter{FailureOnly: true})
	if len(events) < 2 {
		t.Errorf("expected at least 2 failure audit events, got %d", len(events))
	}
}

func TestEngine_CleanupFailure(t *testing.T) {
	store := credentials.NewInMemoryCredentialStore()
	engine := NewEngine(store, nil)

	provider := newMockProvider(credentials.CredentialTypeSSHPassword)
	provider.cleanupErr = errors.New("cleanup failed")
	engine.RegisterProvider(provider)

	job := makeTestJob()
	result, _ := engine.Rotate(context.Background(), job)

	// Cleanup failure results in failed state (credential was stored but old not cleaned)
	if result.Success {
		t.Error("expected failure from cleanup")
	}
}

func TestEngine_GetJob(t *testing.T) {
	engine := NewEngine(credentials.NewInMemoryCredentialStore(), nil)

	// No jobs
	_, ok := engine.GetJob("nonexistent")
	if ok {
		t.Error("expected job not found")
	}

	// Manually insert
	job := makeTestJob()
	engine.mu.Lock()
	engine.jobs[job.ID] = &managedJob{job: job, machine: buildRotationMachine()}
	engine.mu.Unlock()

	got, ok := engine.GetJob(job.ID)
	if !ok {
		t.Fatal("expected job found")
	}
	if got.ID != job.ID {
		t.Errorf("expected job ID %s, got %s", job.ID, got.ID)
	}
}

func TestEngine_ListJobs(t *testing.T) {
	engine := NewEngine(credentials.NewInMemoryCredentialStore(), nil)

	jobs := engine.ListJobs()
	if len(jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(jobs))
	}

	// Insert two jobs
	engine.mu.Lock()
	engine.jobs["j1"] = &managedJob{job: &Job{ID: "j1"}, machine: buildRotationMachine()}
	engine.jobs["j2"] = &managedJob{job: &Job{ID: "j2"}, machine: buildRotationMachine()}
	engine.mu.Unlock()

	jobs = engine.ListJobs()
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}
}

func TestEngine_NilAuditLogger(t *testing.T) {
	engine := NewEngine(credentials.NewInMemoryCredentialStore(), nil)

	provider := newMockProvider(credentials.CredentialTypeSSHPassword)
	engine.RegisterProvider(provider)

	job := makeTestJob()
	result, err := engine.Rotate(context.Background(), job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
}

func TestEngine_ContextCancellation(t *testing.T) {
	engine := NewEngine(credentials.NewInMemoryCredentialStore(), nil)

	provider := newMockProvider(credentials.CredentialTypeSSHPassword)
	provider.validateErr = context.Canceled
	engine.RegisterProvider(provider)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	job := makeTestJob()
	result, _ := engine.Rotate(ctx, job)
	if result.Success {
		t.Error("expected failure from cancelled context")
	}
}

func TestBuildRotationMachine(t *testing.T) {
	machine := buildRotationMachine()

	if machine.State() != CredRotationPending {
		t.Errorf("expected initial state %s, got %s", CredRotationPending, machine.State())
	}

	// Test happy path
	events := []CredRotationEvent{
		CredRotationEventStart,
		CredRotationEventValidated,
		CredRotationEventGenerated,
		CredRotationEventApplied,
		CredRotationEventVerified,
		CredRotationEventStored,
		CredRotationEventCleanedUp,
	}
	expectedStates := []CredRotationState{
		CredRotationValidating,
		CredRotationGenerating,
		CredRotationApplying,
		CredRotationVerifying,
		CredRotationStoring,
		CredRotationCleanup,
		CredRotationCompleted,
	}

	for i, event := range events {
		if err := machine.Fire(event); err != nil {
			t.Fatalf("failed to fire event %s: %v", event, err)
		}
		if machine.State() != expectedStates[i] {
			t.Errorf("after event %s: expected state %s, got %s", event, expectedStates[i], machine.State())
		}
	}
}

func TestBuildRotationMachine_FailureFromEachState(t *testing.T) {
	activeStates := []struct {
		state  CredRotationState
		events []CredRotationEvent
	}{
		{CredRotationValidating, []CredRotationEvent{CredRotationEventStart}},
		{CredRotationGenerating, []CredRotationEvent{CredRotationEventStart, CredRotationEventValidated}},
		{CredRotationApplying, []CredRotationEvent{CredRotationEventStart, CredRotationEventValidated, CredRotationEventGenerated}},
		{CredRotationVerifying, []CredRotationEvent{CredRotationEventStart, CredRotationEventValidated, CredRotationEventGenerated, CredRotationEventApplied}},
		{CredRotationStoring, []CredRotationEvent{CredRotationEventStart, CredRotationEventValidated, CredRotationEventGenerated, CredRotationEventApplied, CredRotationEventVerified}},
		{CredRotationCleanup, []CredRotationEvent{CredRotationEventStart, CredRotationEventValidated, CredRotationEventGenerated, CredRotationEventApplied, CredRotationEventVerified, CredRotationEventStored}},
	}

	for _, tc := range activeStates {
		t.Run(string(tc.state), func(t *testing.T) {
			machine := buildRotationMachine()
			for _, e := range tc.events {
				if err := machine.Fire(e); err != nil {
					t.Fatalf("setup failed at event %s: %v", e, err)
				}
			}
			if machine.State() != tc.state {
				t.Fatalf("expected state %s, got %s", tc.state, machine.State())
			}

			if err := machine.Fire(CredRotationEventFailed); err != nil {
				t.Fatalf("failed to fire failure event from %s: %v", tc.state, err)
			}
			if machine.State() != CredRotationFailed {
				t.Errorf("expected Failed state, got %s", machine.State())
			}
		})
	}
}

func TestBuildRotationMachine_RollbackPath(t *testing.T) {
	machine := buildRotationMachine()

	// Get to failed state
	_ = machine.Fire(CredRotationEventStart)
	_ = machine.Fire(CredRotationEventFailed)

	if machine.State() != CredRotationFailed {
		t.Fatalf("expected Failed, got %s", machine.State())
	}

	// Rollback
	if err := machine.Fire(CredRotationEventRollback); err != nil {
		t.Fatalf("failed to fire rollback: %v", err)
	}
	if machine.State() != CredRotationRollingBack {
		t.Errorf("expected RollingBack, got %s", machine.State())
	}

	// Complete rollback
	if err := machine.Fire(CredRotationEventRolledBack); err != nil {
		t.Fatalf("failed to fire rolled_back: %v", err)
	}
	if machine.State() != CredRotationRolledBack {
		t.Errorf("expected RolledBack, got %s", machine.State())
	}
}

func TestBuildRotationMachine_InvalidTransition(t *testing.T) {
	machine := buildRotationMachine()

	// Can't validate from pending without starting
	err := machine.Fire(CredRotationEventValidated)
	if err == nil {
		t.Error("expected error for invalid transition")
	}
}

func TestCredRotationState_IsTerminal(t *testing.T) {
	tests := []struct {
		state    CredRotationState
		terminal bool
	}{
		{CredRotationPending, false},
		{CredRotationValidating, false},
		{CredRotationGenerating, false},
		{CredRotationApplying, false},
		{CredRotationVerifying, false},
		{CredRotationStoring, false},
		{CredRotationCleanup, false},
		{CredRotationCompleted, true},
		{CredRotationFailed, true},
		{CredRotationRollingBack, false},
		{CredRotationRolledBack, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := tt.state.IsTerminal(); got != tt.terminal {
				t.Errorf("IsTerminal() = %v, want %v", got, tt.terminal)
			}
		})
	}
}

func TestEngine_StoreFailureWithRollback(t *testing.T) {
	// Use a store that fails on Store
	failStore := &failingStore{
		CredentialStore: credentials.NewInMemoryCredentialStore(),
		storeErr:        errors.New("disk full"),
	}
	engine := NewEngine(failStore, nil)

	provider := newMockProvider(credentials.CredentialTypeSSHPassword)
	engine.RegisterProvider(provider)

	job := makeTestJob()
	job.RollbackOnFail = true
	result, _ := engine.Rotate(context.Background(), job)

	if result.Success {
		t.Error("expected failure")
	}
	if !result.RolledBack {
		t.Error("expected rollback on store failure")
	}
}

type failingStore struct {
	credentials.CredentialStore
	storeErr error
}

func (s *failingStore) Store(_ context.Context, _ credentials.Credential) error {
	return s.storeErr
}

func TestEngine_MultipleProviders(t *testing.T) {
	engine := NewEngine(credentials.NewInMemoryCredentialStore(), nil)

	sshProvider := newMockProvider(credentials.CredentialTypeSSHPassword, credentials.CredentialTypeSSHKey)
	snmpProvider := newMockProvider(credentials.CredentialTypeSNMPv2c, credentials.CredentialTypeSNMPv3)
	engine.RegisterProvider(sshProvider)
	engine.RegisterProvider(snmpProvider)

	// SSH job should use sshProvider
	job := makeTestJob()
	result, err := engine.Rotate(context.Background(), job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("SSH rotation failed: %s", result.Error)
	}

	sshCalls := sshProvider.getCalls()
	if len(sshCalls) == 0 {
		t.Error("SSH provider should have been called")
	}

	snmpCalls := snmpProvider.getCalls()
	if len(snmpCalls) != 0 {
		t.Error("SNMP provider should not have been called")
	}
}

func TestResult_Fields(t *testing.T) {
	now := time.Now()
	result := &Result{
		JobID:          "job-1",
		CredentialID:   "cred-1",
		CredentialType: credentials.CredentialTypeSSHPassword,
		DeviceID:       "device-1",
		Success:        true,
		Duration:       5 * time.Second,
		StartedAt:      now,
		CompletedAt:    now.Add(5 * time.Second),
	}

	if result.JobID != "job-1" {
		t.Error("wrong JobID")
	}
	if result.Duration != 5*time.Second {
		t.Errorf("expected 5s duration, got %v", result.Duration)
	}
}

func TestDeviceInfo_Fields(t *testing.T) {
	device := DeviceInfo{
		ID:           "dev-1",
		Host:         "10.0.0.1",
		Port:         22,
		VendorType:   "cisco_ios",
		ProxyAgentID: "proxy-1",
		Metadata:     map[string]string{"site": "dc1"},
	}

	if device.ID != "dev-1" {
		t.Error("wrong ID")
	}
	if device.Host != "10.0.0.1" {
		t.Error("wrong Host")
	}
	if device.Port != 22 {
		t.Error("wrong Port")
	}
	if device.Metadata["site"] != "dc1" {
		t.Error("wrong metadata")
	}
}
