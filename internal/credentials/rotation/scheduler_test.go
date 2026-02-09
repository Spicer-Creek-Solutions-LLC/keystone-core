package rotation

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
)

func TestNewScheduler(t *testing.T) {
	store := credentials.NewInMemoryCredentialStore()
	audit := credentials.NewInMemoryAuditLogger(nil)
	engine := NewEngine(store, audit)
	policies := NewPolicyEngine()

	sched := NewScheduler(engine, policies, store, audit)
	if sched == nil {
		t.Fatal("expected non-nil scheduler")
	}
	if sched.IsRunning() {
		t.Error("should not be running initially")
	}
}

func TestScheduler_StartStop(t *testing.T) {
	store := credentials.NewInMemoryCredentialStore()
	engine := NewEngine(store, nil)
	policies := NewPolicyEngine()
	sched := NewScheduler(engine, policies, store, nil)
	sched.SetCheckInterval(50 * time.Millisecond)

	ctx := context.Background()
	err := sched.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sched.IsRunning() {
		t.Error("should be running after Start")
	}

	// Starting again should fail
	err = sched.Start(ctx)
	if err == nil {
		t.Error("expected error for double start")
	}

	sched.Stop()
	// Give time for goroutine to exit
	time.Sleep(100 * time.Millisecond)

	if sched.IsRunning() {
		t.Error("should not be running after Stop")
	}

	// Stopping again is a no-op
	sched.Stop()
}

func TestScheduler_GetStatus(t *testing.T) {
	store := credentials.NewInMemoryCredentialStore()
	engine := NewEngine(store, nil)
	policies := NewPolicyEngine()

	_ = policies.AddPolicy(&Policy{
		ID:              "p1",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword},
		MaxAge:          time.Hour,
		Enabled:         true,
	})

	sched := NewScheduler(engine, policies, store, nil)

	status := sched.GetStatus()
	if status.Running {
		t.Error("should not be running")
	}
	if status.PolicyCount != 1 {
		t.Errorf("expected 1 policy, got %d", status.PolicyCount)
	}
	if status.ActiveJobs != 0 {
		t.Errorf("expected 0 active jobs, got %d", status.ActiveJobs)
	}
}

func TestScheduler_TriggerNow(t *testing.T) {
	store := credentials.NewInMemoryCredentialStore()
	audit := credentials.NewInMemoryAuditLogger(nil)
	engine := NewEngine(store, audit)

	provider := newMockProvider(credentials.CredentialTypeSSHPassword)
	engine.RegisterProvider(provider)

	policies := NewPolicyEngine()
	sched := NewScheduler(engine, policies, store, audit)

	ctx := context.Background()

	// Store a credential
	_ = store.Store(ctx, &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "cred-1",
			CredentialType: credentials.CredentialTypeSSHPassword,
		},
		Username: "admin",
		Password: "old-pass",
	})

	result, err := sched.TriggerNow(ctx, "cred-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
}

func TestScheduler_TriggerNowNotFound(t *testing.T) {
	store := credentials.NewInMemoryCredentialStore()
	engine := NewEngine(store, nil)
	policies := NewPolicyEngine()
	sched := NewScheduler(engine, policies, store, nil)

	_, err := sched.TriggerNow(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for missing credential")
	}
}

func TestScheduler_AddPolicy(t *testing.T) {
	store := credentials.NewInMemoryCredentialStore()
	engine := NewEngine(store, nil)
	policies := NewPolicyEngine()
	sched := NewScheduler(engine, policies, store, nil)

	err := sched.AddPolicy(&Policy{
		ID:              "p1",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword},
		MaxAge:          90 * 24 * time.Hour,
		Schedule:        "0 2 * * *",
		Enabled:         true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScheduler_AddPolicyInvalidCron(t *testing.T) {
	store := credentials.NewInMemoryCredentialStore()
	engine := NewEngine(store, nil)
	policies := NewPolicyEngine()
	sched := NewScheduler(engine, policies, store, nil)

	err := sched.AddPolicy(&Policy{
		ID:              "p1",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword},
		MaxAge:          time.Hour,
		Schedule:        "invalid cron",
		Enabled:         true,
	})
	if err == nil {
		t.Error("expected error for invalid cron")
	}
}

func TestScheduler_AutomaticRotation(t *testing.T) {
	store := credentials.NewInMemoryCredentialStore()
	audit := credentials.NewInMemoryAuditLogger(nil)
	engine := NewEngine(store, audit)

	provider := newMockProvider(credentials.CredentialTypeSSHPassword)
	engine.RegisterProvider(provider)

	policies := NewPolicyEngine()
	_ = policies.AddPolicy(&Policy{
		ID:              "p1",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword},
		MaxAge:          1 * time.Millisecond, // Very short for testing
		Enabled:         true,
		RollbackOnFail:  true,
	})

	sched := NewScheduler(engine, policies, store, audit)
	sched.SetCheckInterval(50 * time.Millisecond)

	ctx := context.Background()

	// Store an old credential
	_ = store.Store(ctx, &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "old-cred",
			CredentialType: credentials.CredentialTypeSSHPassword,
			CreatedAt:      time.Now().Add(-24 * time.Hour),
		},
		Username: "admin",
		Password: "pass",
	})

	err := sched.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for the scheduler to pick up and process the rotation
	time.Sleep(200 * time.Millisecond)
	sched.Stop()

	status := sched.GetStatus()
	if status.CompletedJobs == 0 {
		t.Error("expected at least 1 completed job from automatic rotation")
	}
}

func TestScheduler_ContextCancellation(t *testing.T) {
	store := credentials.NewInMemoryCredentialStore()
	engine := NewEngine(store, nil)
	policies := NewPolicyEngine()
	sched := NewScheduler(engine, policies, store, nil)
	sched.SetCheckInterval(50 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	err := sched.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cancel context
	cancel()
	time.Sleep(100 * time.Millisecond)

	if sched.IsRunning() {
		t.Error("scheduler should stop when context is cancelled")
	}
}

func TestScheduler_SetCheckInterval(t *testing.T) {
	store := credentials.NewInMemoryCredentialStore()
	engine := NewEngine(store, nil)
	policies := NewPolicyEngine()
	sched := NewScheduler(engine, policies, store, nil)

	sched.SetCheckInterval(5 * time.Minute)

	sched.mu.RLock()
	defer sched.mu.RUnlock()
	if sched.checkInterval != 5*time.Minute {
		t.Errorf("expected 5m interval, got %v", sched.checkInterval)
	}
}

func TestScheduler_NilAuditLogger(t *testing.T) {
	store := credentials.NewInMemoryCredentialStore()
	engine := NewEngine(store, nil)
	policies := NewPolicyEngine()
	sched := NewScheduler(engine, policies, store, nil)

	// Should not panic with nil audit logger
	sched.logAudit(context.Background(), "test", true, "msg")
}
