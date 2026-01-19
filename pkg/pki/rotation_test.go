package pki

import (
	"context"
	"testing"
	"time"
)

func TestCAState(t *testing.T) {
	states := []CAState{
		CAStateActive,
		CAStatePending,
		CAStateRotating,
		CAStateRevoked,
		CAStateExpired,
	}

	for _, state := range states {
		if state == "" {
			t.Error("State should not be empty")
		}
	}
}

func TestRotationStrategy(t *testing.T) {
	strategies := []RotationStrategy{
		StrategyOverlap,
		StrategyBlueGreen,
		StrategyRolling,
	}

	for _, s := range strategies {
		if s == "" {
			t.Error("Strategy should not be empty")
		}
	}
}

func TestCAInfo_RemainingValidity(t *testing.T) {
	tests := []struct {
		name       string
		notAfter   time.Time
		wantExpired bool
	}{
		{
			name:        "future expiry",
			notAfter:    time.Now().Add(24 * time.Hour),
			wantExpired: false,
		},
		{
			name:        "past expiry",
			notAfter:    time.Now().Add(-24 * time.Hour),
			wantExpired: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ca := &CAInfo{
				NotBefore: time.Now().Add(-365 * 24 * time.Hour),
				NotAfter:  tt.notAfter,
			}

			remaining := ca.RemainingValidity()
			if tt.wantExpired && remaining != 0 {
				t.Errorf("Expected 0 remaining, got %v", remaining)
			}
			if !tt.wantExpired && remaining <= 0 {
				t.Errorf("Expected positive remaining, got %v", remaining)
			}
		})
	}
}

func TestCAInfo_ValidityPercentage(t *testing.T) {
	now := time.Now()
	ca := &CAInfo{
		NotBefore: now.Add(-180 * 24 * time.Hour), // Started 180 days ago
		NotAfter:  now.Add(185 * 24 * time.Hour),  // Expires in 185 days
	}

	pct := ca.ValidityPercentage()
	if pct < 40 || pct > 60 {
		t.Errorf("ValidityPercentage = %f, expected around 50%%", pct)
	}
}

func TestCAInfo_IsExpiringSoon(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		notAfter  time.Time
		threshold time.Duration
		want      bool
	}{
		{
			name:      "expiring within threshold",
			notAfter:  now.Add(7 * 24 * time.Hour),
			threshold: 30 * 24 * time.Hour,
			want:      true,
		},
		{
			name:      "not expiring within threshold",
			notAfter:  now.Add(60 * 24 * time.Hour),
			threshold: 30 * 24 * time.Hour,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ca := &CAInfo{NotAfter: tt.notAfter}
			if got := ca.IsExpiringSoon(tt.threshold); got != tt.want {
				t.Errorf("IsExpiringSoon = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultRotationPolicy(t *testing.T) {
	policy := DefaultRotationPolicy()

	if policy.Name != "default" {
		t.Errorf("Name = %s, want default", policy.Name)
	}
	if policy.Strategy != StrategyOverlap {
		t.Errorf("Strategy = %s, want overlap", policy.Strategy)
	}
	if policy.RotateBeforeExpiry != 30*24*time.Hour {
		t.Errorf("RotateBeforeExpiry = %v, want 30 days", policy.RotateBeforeExpiry)
	}
	if !policy.AutoRotate {
		t.Error("AutoRotate should be true")
	}
	if len(policy.NotifyBeforeDays) != 4 {
		t.Errorf("NotifyBeforeDays = %d, want 4", len(policy.NotifyBeforeDays))
	}
	if policy.KeyConfig == nil {
		t.Error("KeyConfig should not be nil")
	}
	if policy.KeyConfig.Type != "RSA" {
		t.Errorf("KeyConfig.Type = %s, want RSA", policy.KeyConfig.Type)
	}
}

func TestNewRotationManager(t *testing.T) {
	caStore := NewInMemoryCAStore()
	scheduleStore := NewInMemoryScheduleStore()
	generator := &MockCAGenerator{}

	rm := NewRotationManager(caStore, scheduleStore, generator)

	if rm == nil {
		t.Fatal("Expected non-nil rotation manager")
	}

	// Check default policy is registered
	policy, ok := rm.GetPolicy("default")
	if !ok {
		t.Error("Expected default policy to be registered")
	}
	if policy.Name != "default" {
		t.Errorf("Policy name = %s, want default", policy.Name)
	}
}

func TestRotationManager_RegisterPolicy(t *testing.T) {
	rm := NewRotationManager(NewInMemoryCAStore(), NewInMemoryScheduleStore(), &MockCAGenerator{})

	customPolicy := &RotationPolicy{
		Name:               "custom",
		Strategy:           StrategyBlueGreen,
		RotateBeforeExpiry: 60 * 24 * time.Hour,
	}

	rm.RegisterPolicy(customPolicy)

	policy, ok := rm.GetPolicy("custom")
	if !ok {
		t.Error("Expected custom policy to be found")
	}
	if policy.Strategy != StrategyBlueGreen {
		t.Errorf("Strategy = %s, want blue-green", policy.Strategy)
	}
}

func TestRotationManager_ApproveRotation(t *testing.T) {
	caStore := NewInMemoryCAStore()
	scheduleStore := NewInMemoryScheduleStore()
	rm := NewRotationManager(caStore, scheduleStore, &MockCAGenerator{})

	ctx := context.Background()

	// Create a pending schedule
	schedule := &RotationSchedule{
		ID:          "test-schedule",
		CAID:        "ca-1",
		PolicyName:  "default",
		ScheduledAt: time.Now(),
		Status:      StatusPending,
	}
	scheduleStore.Save(ctx, schedule)

	// Approve it
	err := rm.ApproveRotation(ctx, "test-schedule", "admin@example.com")
	if err != nil {
		t.Fatalf("ApproveRotation failed: %v", err)
	}

	// Verify status
	updated, _ := scheduleStore.Get(ctx, "test-schedule")
	if updated.Status != StatusApproved {
		t.Errorf("Status = %s, want approved", updated.Status)
	}
	if updated.ApprovedBy != "admin@example.com" {
		t.Errorf("ApprovedBy = %s, want admin@example.com", updated.ApprovedBy)
	}
	if updated.ApprovedAt == nil {
		t.Error("ApprovedAt should not be nil")
	}
}

func TestRotationManager_CancelRotation(t *testing.T) {
	caStore := NewInMemoryCAStore()
	scheduleStore := NewInMemoryScheduleStore()
	rm := NewRotationManager(caStore, scheduleStore, &MockCAGenerator{})

	ctx := context.Background()

	// Create a pending schedule
	schedule := &RotationSchedule{
		ID:          "test-schedule",
		CAID:        "ca-1",
		PolicyName:  "default",
		ScheduledAt: time.Now(),
		Status:      StatusPending,
	}
	scheduleStore.Save(ctx, schedule)

	// Cancel it
	err := rm.CancelRotation(ctx, "test-schedule")
	if err != nil {
		t.Fatalf("CancelRotation failed: %v", err)
	}

	// Verify status
	updated, _ := scheduleStore.Get(ctx, "test-schedule")
	if updated.Status != StatusCancelled {
		t.Errorf("Status = %s, want cancelled", updated.Status)
	}
}

func TestRotationManager_CancelRotation_InvalidStatus(t *testing.T) {
	scheduleStore := NewInMemoryScheduleStore()
	rm := NewRotationManager(NewInMemoryCAStore(), scheduleStore, &MockCAGenerator{})

	ctx := context.Background()

	// Create a completed schedule
	schedule := &RotationSchedule{
		ID:     "test-schedule",
		Status: StatusCompleted,
	}
	scheduleStore.Save(ctx, schedule)

	// Try to cancel it
	err := rm.CancelRotation(ctx, "test-schedule")
	if err == nil {
		t.Error("Expected error when cancelling completed rotation")
	}
}

func TestRotationManager_RollbackRotation(t *testing.T) {
	caStore := NewInMemoryCAStore()
	scheduleStore := NewInMemoryScheduleStore()
	rm := NewRotationManager(caStore, scheduleStore, &MockCAGenerator{})

	ctx := context.Background()

	// Create CAs
	oldCA := &CAInfo{ID: "old-ca", State: CAStateRotating}
	newCA := &CAInfo{ID: "new-ca", State: CAStateActive}
	caStore.Save(ctx, oldCA)
	caStore.Save(ctx, newCA)

	// Create completed schedule
	schedule := &RotationSchedule{
		ID:           "test-schedule",
		CAID:         "old-ca",
		Status:       StatusCompleted,
		NewCAID:      "new-ca",
		RollbackCAID: "old-ca",
	}
	scheduleStore.Save(ctx, schedule)

	// Rollback
	err := rm.RollbackRotation(ctx, "test-schedule")
	if err != nil {
		t.Fatalf("RollbackRotation failed: %v", err)
	}

	// Verify states
	updated, _ := scheduleStore.Get(ctx, "test-schedule")
	if updated.Status != StatusRolledBack {
		t.Errorf("Status = %s, want rolled_back", updated.Status)
	}

	oldCAUpdated, _ := caStore.Get(ctx, "old-ca")
	if oldCAUpdated.State != CAStateActive {
		t.Errorf("Old CA state = %s, want active", oldCAUpdated.State)
	}

	newCAUpdated, _ := caStore.Get(ctx, "new-ca")
	if newCAUpdated.State != CAStateRevoked {
		t.Errorf("New CA state = %s, want revoked", newCAUpdated.State)
	}
}

func TestRotationManager_ScheduleRotationNow(t *testing.T) {
	caStore := NewInMemoryCAStore()
	scheduleStore := NewInMemoryScheduleStore()
	rm := NewRotationManager(caStore, scheduleStore, &MockCAGenerator{})

	ctx := context.Background()

	// Create a CA
	ca := &CAInfo{
		ID:    "ca-1",
		Name:  "Test CA",
		State: CAStateActive,
	}
	caStore.Save(ctx, ca)

	// Schedule rotation
	schedule, err := rm.ScheduleRotationNow(ctx, "ca-1", "default")
	if err != nil {
		t.Fatalf("ScheduleRotationNow failed: %v", err)
	}

	if schedule.CAID != "ca-1" {
		t.Errorf("CAID = %s, want ca-1", schedule.CAID)
	}
	if schedule.Status != StatusPending {
		t.Errorf("Status = %s, want pending", schedule.Status)
	}
	if schedule.PolicyName != "default" {
		t.Errorf("PolicyName = %s, want default", schedule.PolicyName)
	}
}

func TestRotationManager_StartStop(t *testing.T) {
	rm := NewRotationManager(NewInMemoryCAStore(), NewInMemoryScheduleStore(), &MockCAGenerator{})

	var started, stopped bool
	rm.AddListener(func(e *RotationEvent) {
		if e.Type == "manager_started" {
			started = true
		}
		if e.Type == "manager_stopped" {
			stopped = true
		}
	})

	ctx := context.Background()

	if err := rm.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Can't start twice
	if err := rm.Start(ctx); err == nil {
		t.Error("Expected error when starting twice")
	}

	rm.Stop()

	if !started {
		t.Error("Expected manager_started event")
	}
	if !stopped {
		t.Error("Expected manager_stopped event")
	}
}

func TestRotationManager_Listener(t *testing.T) {
	rm := NewRotationManager(NewInMemoryCAStore(), NewInMemoryScheduleStore(), &MockCAGenerator{})

	var events []*RotationEvent
	rm.AddListener(func(e *RotationEvent) {
		events = append(events, e)
	})

	rm.emit(&RotationEvent{Type: "test_event"})

	if len(events) != 1 {
		t.Errorf("Events = %d, want 1", len(events))
	}
	if events[0].Type != "test_event" {
		t.Errorf("Event type = %s, want test_event", events[0].Type)
	}
}

func TestInMemoryCAStore(t *testing.T) {
	store := NewInMemoryCAStore()
	ctx := context.Background()

	ca := &CAInfo{
		ID:       "ca-1",
		Name:     "Test CA",
		State:    CAStateActive,
		NotAfter: time.Now().Add(365 * 24 * time.Hour),
	}

	// Test Save
	if err := store.Save(ctx, ca); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Test Get
	retrieved, err := store.Get(ctx, "ca-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.Name != "Test CA" {
		t.Errorf("Name = %s, want Test CA", retrieved.Name)
	}

	// Test List
	cas, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(cas) != 1 {
		t.Errorf("List count = %d, want 1", len(cas))
	}

	// Test ListByState
	activeCAs, err := store.ListByState(ctx, CAStateActive)
	if err != nil {
		t.Fatalf("ListByState failed: %v", err)
	}
	if len(activeCAs) != 1 {
		t.Errorf("Active CAs = %d, want 1", len(activeCAs))
	}

	// Test UpdateState
	if err := store.UpdateState(ctx, "ca-1", CAStateRotating); err != nil {
		t.Fatalf("UpdateState failed: %v", err)
	}

	updated, _ := store.Get(ctx, "ca-1")
	if updated.State != CAStateRotating {
		t.Errorf("State = %s, want rotating", updated.State)
	}

	// Test Delete
	if err := store.Delete(ctx, "ca-1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.Get(ctx, "ca-1")
	if err == nil {
		t.Error("Expected error after delete")
	}
}

func TestInMemoryScheduleStore(t *testing.T) {
	store := NewInMemoryScheduleStore()
	ctx := context.Background()

	schedule := &RotationSchedule{
		ID:          "sched-1",
		CAID:        "ca-1",
		PolicyName:  "default",
		ScheduledAt: time.Now(),
		Status:      StatusPending,
	}

	// Test Save
	if err := store.Save(ctx, schedule); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Test Get
	retrieved, err := store.Get(ctx, "sched-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.CAID != "ca-1" {
		t.Errorf("CAID = %s, want ca-1", retrieved.CAID)
	}

	// Test List
	schedules, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(schedules) != 1 {
		t.Errorf("List count = %d, want 1", len(schedules))
	}

	// Test ListByCA
	caSchedules, err := store.ListByCA(ctx, "ca-1")
	if err != nil {
		t.Fatalf("ListByCA failed: %v", err)
	}
	if len(caSchedules) != 1 {
		t.Errorf("CA schedules = %d, want 1", len(caSchedules))
	}

	// Test ListByStatus
	pendingSchedules, err := store.ListByStatus(ctx, StatusPending)
	if err != nil {
		t.Fatalf("ListByStatus failed: %v", err)
	}
	if len(pendingSchedules) != 1 {
		t.Errorf("Pending schedules = %d, want 1", len(pendingSchedules))
	}

	// Test Delete
	if err := store.Delete(ctx, "sched-1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.Get(ctx, "sched-1")
	if err == nil {
		t.Error("Expected error after delete")
	}
}

func TestMockCAGenerator(t *testing.T) {
	generator := &MockCAGenerator{}
	ctx := context.Background()

	config := &CAGenerationConfig{
		Name:         "Test CA",
		Subject:      "CN=Test CA,O=Test",
		ValidityDays: 365,
		KeyConfig: &KeyConfig{
			Type:      "RSA",
			Size:      4096,
			Algorithm: "SHA256WithRSA",
		},
		IsRoot: true,
	}

	ca, err := generator.Generate(ctx, config)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if ca.Name != "Test CA" {
		t.Errorf("Name = %s, want Test CA", ca.Name)
	}
	if ca.Subject != "CN=Test CA,O=Test" {
		t.Errorf("Subject = %s, want CN=Test CA,O=Test", ca.Subject)
	}
	if ca.State != CAStatePending {
		t.Errorf("State = %s, want pending", ca.State)
	}
	if !ca.IsRoot {
		t.Error("IsRoot should be true")
	}
	if ca.KeyType != "RSA" {
		t.Errorf("KeyType = %s, want RSA", ca.KeyType)
	}
	if ca.KeySize != 4096 {
		t.Errorf("KeySize = %d, want 4096", ca.KeySize)
	}

	// Verify validity period
	validity := ca.NotAfter.Sub(ca.NotBefore)
	expectedValidity := 365 * 24 * time.Hour
	if validity < expectedValidity-time.Hour || validity > expectedValidity+time.Hour {
		t.Errorf("Validity = %v, want ~%v", validity, expectedValidity)
	}
}

func TestScheduleStatus(t *testing.T) {
	statuses := []ScheduleStatus{
		StatusPending,
		StatusApproved,
		StatusInProgress,
		StatusCompleted,
		StatusFailed,
		StatusCancelled,
		StatusRolledBack,
	}

	for _, status := range statuses {
		if status == "" {
			t.Error("Status should not be empty")
		}
	}
}

func TestRotationPolicy(t *testing.T) {
	policy := &RotationPolicy{
		Name:               "strict",
		Strategy:           StrategyBlueGreen,
		RotateBeforeExpiry: 60 * 24 * time.Hour,
		MinValidityPeriod:  180 * 24 * time.Hour,
		MaxValidityPeriod:  730 * 24 * time.Hour, // 2 years
		OverlapPeriod:      14 * 24 * time.Hour,
		GracePeriod:        48 * time.Hour,
		RequireApproval:    true,
		AutoRotate:         false,
		NotifyBeforeDays:   []int{60, 30, 14, 7, 3, 1},
	}

	if policy.RequireApproval != true {
		t.Error("RequireApproval should be true")
	}
	if policy.AutoRotate != false {
		t.Error("AutoRotate should be false")
	}
	if len(policy.NotifyBeforeDays) != 6 {
		t.Errorf("NotifyBeforeDays = %d, want 6", len(policy.NotifyBeforeDays))
	}
}

func TestKeyConfig(t *testing.T) {
	tests := []struct {
		name   string
		config KeyConfig
	}{
		{
			name: "RSA 4096",
			config: KeyConfig{
				Type:      "RSA",
				Size:      4096,
				Algorithm: "SHA256WithRSA",
			},
		},
		{
			name: "ECDSA P-384",
			config: KeyConfig{
				Type:      "ECDSA",
				Curve:     "P-384",
				Algorithm: "ECDSAWithSHA384",
			},
		},
		{
			name: "Ed25519",
			config: KeyConfig{
				Type:      "Ed25519",
				Algorithm: "Ed25519",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config.Type == "" {
				t.Error("Type should not be empty")
			}
			if tt.config.Algorithm == "" {
				t.Error("Algorithm should not be empty")
			}
		})
	}
}

func TestRotationEvent(t *testing.T) {
	event := &RotationEvent{
		Type:       "rotation_scheduled",
		CAID:       "ca-123",
		ScheduleID: "sched-456",
		Timestamp:  time.Now(),
		Message:    "Rotation scheduled",
		Details: map[string]interface{}{
			"reason": "expiring_soon",
		},
	}

	if event.Type != "rotation_scheduled" {
		t.Errorf("Type = %s, want rotation_scheduled", event.Type)
	}
	if event.Details["reason"] != "expiring_soon" {
		t.Error("Details should contain reason")
	}
}

func TestRotationSchedule(t *testing.T) {
	now := time.Now()
	schedule := &RotationSchedule{
		ID:          "sched-1",
		CAID:        "ca-1",
		PolicyName:  "default",
		ScheduledAt: now,
		Status:      StatusCompleted,
		ApprovedBy:  "admin",
		ApprovedAt:  &now,
		StartedAt:   &now,
		CompletedAt: &now,
		NewCAID:     "ca-2",
		RollbackCAID: "ca-1",
	}

	if schedule.NewCAID != "ca-2" {
		t.Errorf("NewCAID = %s, want ca-2", schedule.NewCAID)
	}
	if schedule.RollbackCAID != "ca-1" {
		t.Errorf("RollbackCAID = %s, want ca-1", schedule.RollbackCAID)
	}
}

func TestCAInfo(t *testing.T) {
	now := time.Now()
	activatedAt := now.Add(-180 * 24 * time.Hour)

	ca := &CAInfo{
		ID:           "ca-root",
		Name:         "Root CA",
		State:        CAStateActive,
		SerialNumber: "0123456789ABCDEF",
		Subject:      "CN=Root CA,O=Example",
		NotBefore:    activatedAt,
		NotAfter:     now.Add(5 * 365 * 24 * time.Hour),
		KeyType:      "RSA",
		KeySize:      4096,
		SignatureAlg: "SHA256WithRSA",
		Fingerprint:  "SHA256:abc123",
		IsRoot:       true,
		CreatedAt:    activatedAt,
		ActivatedAt:  &activatedAt,
		Metadata: map[string]string{
			"environment": "production",
		},
	}

	if ca.IsRoot != true {
		t.Error("IsRoot should be true")
	}
	if ca.Metadata["environment"] != "production" {
		t.Error("Metadata should contain environment")
	}
}

func TestGenerateID(t *testing.T) {
	id1 := generateID()
	id2 := generateID()

	if id1 == "" {
		t.Error("ID should not be empty")
	}
	if id1 == id2 {
		t.Error("IDs should be unique")
	}
	if len(id1) != 16 {
		t.Errorf("ID length = %d, want 16", len(id1))
	}
}

func TestGenerateSerial(t *testing.T) {
	serial1 := generateSerial()
	serial2 := generateSerial()

	if serial1 == "" {
		t.Error("Serial should not be empty")
	}
	if serial1 == serial2 {
		t.Error("Serials should be unique")
	}
}

func TestGenerateFingerprint(t *testing.T) {
	fp1 := generateFingerprint()
	fp2 := generateFingerprint()

	if fp1 == "" {
		t.Error("Fingerprint should not be empty")
	}
	if fp1 == fp2 {
		t.Error("Fingerprints should be unique")
	}
}
