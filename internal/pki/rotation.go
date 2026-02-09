// Package pki provides PKI management including CA rotation scheduling.
package pki

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// CAState represents the state of a CA.
type CAState string

const (
	// CAStateActive indicates the CA is the current active CA.
	CAStateActive CAState = "active"
	// CAStatePending indicates the CA is pending activation.
	CAStatePending CAState = "pending"
	// CAStateRotating indicates the CA is being rotated out.
	CAStateRotating CAState = "rotating"
	// CAStateRevoked indicates the CA has been revoked.
	CAStateRevoked CAState = "revoked"
	// CAStateExpired indicates the CA has expired.
	CAStateExpired CAState = "expired"
)

// RotationStrategy represents the CA rotation strategy.
type RotationStrategy string

const (
	// StrategyOverlap uses overlapping validity periods.
	StrategyOverlap RotationStrategy = "overlap"
	// StrategyBlueGreen uses blue-green rotation.
	StrategyBlueGreen RotationStrategy = "blue-green"
	// StrategyRolling uses rolling rotation across clusters.
	StrategyRolling RotationStrategy = "rolling"
)

// CAInfo contains information about a Certificate Authority.
type CAInfo struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	State        CAState           `json:"state"`
	SerialNumber string            `json:"serialNumber"`
	Subject      string            `json:"subject"`
	Issuer       string            `json:"issuer,omitempty"`
	NotBefore    time.Time         `json:"notBefore"`
	NotAfter     time.Time         `json:"notAfter"`
	KeyType      string            `json:"keyType"`
	KeySize      int               `json:"keySize"`
	SignatureAlg string            `json:"signatureAlg"`
	Fingerprint  string            `json:"fingerprint"`
	IsRoot       bool              `json:"isRoot"`
	ParentID     string            `json:"parentId,omitempty"`
	CreatedAt    time.Time         `json:"createdAt"`
	ActivatedAt  *time.Time        `json:"activatedAt,omitempty"`
	RotatedAt    *time.Time        `json:"rotatedAt,omitempty"`
	RevokedAt    *time.Time        `json:"revokedAt,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// RemainingValidity returns the remaining validity period.
func (c *CAInfo) RemainingValidity() time.Duration {
	if c.NotAfter.Before(time.Now()) {
		return 0
	}
	return time.Until(c.NotAfter)
}

// ValidityPercentage returns the percentage of validity remaining.
func (c *CAInfo) ValidityPercentage() float64 {
	total := c.NotAfter.Sub(c.NotBefore)
	remaining := c.RemainingValidity()
	if total <= 0 {
		return 0
	}
	return float64(remaining) / float64(total) * 100
}

// IsExpiringSoon returns true if the CA expires within the given duration.
func (c *CAInfo) IsExpiringSoon(threshold time.Duration) bool {
	return c.RemainingValidity() <= threshold
}

// RotationPolicy defines when and how to rotate CAs.
type RotationPolicy struct {
	// Name of the policy
	Name string `json:"name"`

	// Strategy for rotation
	Strategy RotationStrategy `json:"strategy"`

	// RotateBeforeExpiry is when to start rotation before expiry
	RotateBeforeExpiry time.Duration `json:"rotateBeforeExpiry"`

	// MinValidityPeriod is the minimum validity for new CAs
	MinValidityPeriod time.Duration `json:"minValidityPeriod"`

	// MaxValidityPeriod is the maximum validity for new CAs
	MaxValidityPeriod time.Duration `json:"maxValidityPeriod"`

	// OverlapPeriod for overlap strategy
	OverlapPeriod time.Duration `json:"overlapPeriod"`

	// GracePeriod after rotation for cleanup
	GracePeriod time.Duration `json:"gracePeriod"`

	// RequireApproval if manual approval is needed
	RequireApproval bool `json:"requireApproval"`

	// NotifyBeforeDays to send notifications
	NotifyBeforeDays []int `json:"notifyBeforeDays"`

	// AutoRotate enables automatic rotation
	AutoRotate bool `json:"autoRotate"`

	// KeyConfig for new CA generation
	KeyConfig *KeyConfig `json:"keyConfig,omitempty"`
}

// KeyConfig defines key generation parameters.
type KeyConfig struct {
	Type      string `json:"type"`      // RSA, ECDSA, Ed25519
	Size      int    `json:"size"`      // Key size for RSA
	Curve     string `json:"curve"`     // Curve for ECDSA
	Algorithm string `json:"algorithm"` // Signature algorithm
}

// DefaultRotationPolicy returns a default rotation policy.
func DefaultRotationPolicy() *RotationPolicy {
	return &RotationPolicy{
		Name:               "default",
		Strategy:           StrategyOverlap,
		RotateBeforeExpiry: 30 * 24 * time.Hour,  // 30 days
		MinValidityPeriod:  90 * 24 * time.Hour,  // 90 days
		MaxValidityPeriod:  365 * 24 * time.Hour, // 1 year
		OverlapPeriod:      7 * 24 * time.Hour,   // 7 days
		GracePeriod:        24 * time.Hour,       // 1 day
		AutoRotate:         true,
		NotifyBeforeDays:   []int{30, 14, 7, 1},
		KeyConfig: &KeyConfig{
			Type:      "RSA",
			Size:      4096,
			Algorithm: "SHA256WithRSA",
		},
	}
}

// RotationSchedule represents a scheduled rotation.
type RotationSchedule struct {
	ID           string         `json:"id"`
	CAID         string         `json:"caId"`
	PolicyName   string         `json:"policyName"`
	ScheduledAt  time.Time      `json:"scheduledAt"`
	Status       ScheduleStatus `json:"status"`
	ApprovedBy   string         `json:"approvedBy,omitempty"`
	ApprovedAt   *time.Time     `json:"approvedAt,omitempty"`
	StartedAt    *time.Time     `json:"startedAt,omitempty"`
	CompletedAt  *time.Time     `json:"completedAt,omitempty"`
	FailedAt     *time.Time     `json:"failedAt,omitempty"`
	Error        string         `json:"error,omitempty"`
	NewCAID      string         `json:"newCaId,omitempty"`
	RollbackCAID string         `json:"rollbackCaId,omitempty"`
}

// ScheduleStatus represents the status of a scheduled rotation.
type ScheduleStatus string

const (
	// StatusPending indicates the rotation is pending.
	StatusPending ScheduleStatus = "pending"
	// StatusApproved indicates the rotation is approved.
	StatusApproved ScheduleStatus = "approved"
	// StatusInProgress indicates the rotation is in progress.
	StatusInProgress ScheduleStatus = "in_progress"
	// StatusCompleted indicates the rotation is completed.
	StatusCompleted ScheduleStatus = "completed"
	// StatusFailed indicates the rotation failed.
	StatusFailed ScheduleStatus = "failed"
	// StatusCancelled indicates the rotation was cancelled.
	StatusCancelled ScheduleStatus = "cancelled"
	// StatusRolledBack indicates the rotation was rolled back.
	StatusRolledBack ScheduleStatus = "rolled_back"
)

// RotationEvent represents an event during rotation.
type RotationEvent struct {
	Type       string                 `json:"type"`
	CAID       string                 `json:"caId,omitempty"`
	ScheduleID string                 `json:"scheduleId,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	Message    string                 `json:"message"`
	Error      string                 `json:"error,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
}

// CAStore is the interface for storing CA information.
type CAStore interface {
	Get(ctx context.Context, id string) (*CAInfo, error)
	List(ctx context.Context) ([]*CAInfo, error)
	ListByState(ctx context.Context, state CAState) ([]*CAInfo, error)
	Save(ctx context.Context, ca *CAInfo) error
	UpdateState(ctx context.Context, id string, state CAState) error
	Delete(ctx context.Context, id string) error
}

// ScheduleStore is the interface for storing rotation schedules.
type ScheduleStore interface {
	Get(ctx context.Context, id string) (*RotationSchedule, error)
	List(ctx context.Context) ([]*RotationSchedule, error)
	ListByCA(ctx context.Context, caID string) ([]*RotationSchedule, error)
	ListByStatus(ctx context.Context, status ScheduleStatus) ([]*RotationSchedule, error)
	Save(ctx context.Context, schedule *RotationSchedule) error
	Delete(ctx context.Context, id string) error
}

// CAGenerator generates new CAs.
type CAGenerator interface {
	Generate(ctx context.Context, config *CAGenerationConfig) (*CAInfo, error)
}

// CAGenerationConfig configures CA generation.
type CAGenerationConfig struct {
	Name         string
	Subject      string
	ValidityDays int
	KeyConfig    *KeyConfig
	ParentID     string
	IsRoot       bool
	Metadata     map[string]string
}

// RotationManager manages CA rotation.
type RotationManager struct {
	caStore       CAStore
	scheduleStore ScheduleStore
	generator     CAGenerator
	policies      map[string]*RotationPolicy
	listeners     []RotationListener
	mu            sync.RWMutex
	stopCh        chan struct{}
	running       bool
}

// RotationListener is called when rotation events occur.
type RotationListener func(event *RotationEvent)

// NewRotationManager creates a new rotation manager.
func NewRotationManager(caStore CAStore, scheduleStore ScheduleStore, generator CAGenerator) *RotationManager {
	rm := &RotationManager{
		caStore:       caStore,
		scheduleStore: scheduleStore,
		generator:     generator,
		policies:      make(map[string]*RotationPolicy),
		stopCh:        make(chan struct{}),
	}

	// Register default policy
	rm.policies["default"] = DefaultRotationPolicy()

	return rm
}

// RegisterPolicy registers a rotation policy.
func (rm *RotationManager) RegisterPolicy(policy *RotationPolicy) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.policies[policy.Name] = policy
}

// GetPolicy retrieves a policy by name.
func (rm *RotationManager) GetPolicy(name string) (*RotationPolicy, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	p, ok := rm.policies[name]
	return p, ok
}

// AddListener adds a rotation event listener.
func (rm *RotationManager) AddListener(listener RotationListener) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.listeners = append(rm.listeners, listener)
}

// emit sends an event to all listeners.
func (rm *RotationManager) emit(event *RotationEvent) {
	rm.mu.RLock()
	listeners := make([]RotationListener, len(rm.listeners))
	copy(listeners, rm.listeners)
	rm.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// Start starts the rotation manager.
func (rm *RotationManager) Start(ctx context.Context) error {
	rm.mu.Lock()
	if rm.running {
		rm.mu.Unlock()
		return fmt.Errorf("rotation manager already running")
	}
	rm.running = true
	rm.stopCh = make(chan struct{})
	rm.mu.Unlock()

	rm.emit(&RotationEvent{
		Type:      "manager_started",
		Timestamp: time.Now(),
		Message:   "Rotation manager started",
	})

	// Start monitoring loop
	go rm.monitorLoop(ctx)

	return nil
}

// Stop stops the rotation manager.
func (rm *RotationManager) Stop() {
	rm.mu.Lock()
	if !rm.running {
		rm.mu.Unlock()
		return
	}
	close(rm.stopCh)
	rm.running = false
	rm.mu.Unlock()

	rm.emit(&RotationEvent{
		Type:      "manager_stopped",
		Timestamp: time.Now(),
		Message:   "Rotation manager stopped",
	})
}

// monitorLoop monitors CAs and executes scheduled rotations.
func (rm *RotationManager) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rm.stopCh:
			return
		case <-ticker.C:
			rm.checkExpiringCAs(ctx)
			rm.processPendingSchedules(ctx)
		}
	}
}

// checkExpiringCAs checks for expiring CAs and schedules rotations.
func (rm *RotationManager) checkExpiringCAs(ctx context.Context) {
	cas, err := rm.caStore.ListByState(ctx, CAStateActive)
	if err != nil {
		rm.emit(&RotationEvent{
			Type:      "check_error",
			Timestamp: time.Now(),
			Message:   "Failed to list active CAs",
			Error:     err.Error(),
		})
		return
	}

	for _, ca := range cas {
		for policyName, policy := range rm.policies {
			if !policy.AutoRotate {
				continue
			}

			// Check if CA is expiring soon
			if ca.IsExpiringSoon(policy.RotateBeforeExpiry) {
				// Check if rotation is already scheduled
				scheduled, _ := rm.hasScheduledRotation(ctx, ca.ID)
				if !scheduled {
					rm.scheduleRotation(ctx, ca, policyName)
				}
			}

			// Send notifications
			rm.checkNotifications(ca, policy)
		}
	}
}

func (rm *RotationManager) hasScheduledRotation(ctx context.Context, caID string) (bool, error) {
	schedules, err := rm.scheduleStore.ListByCA(ctx, caID)
	if err != nil {
		return false, err
	}

	for _, s := range schedules {
		if s.Status == StatusPending || s.Status == StatusApproved || s.Status == StatusInProgress {
			return true, nil
		}
	}
	return false, nil
}

func (rm *RotationManager) scheduleRotation(ctx context.Context, ca *CAInfo, policyName string) {
	schedule := &RotationSchedule{
		ID:          generateID(),
		CAID:        ca.ID,
		PolicyName:  policyName,
		ScheduledAt: time.Now(),
		Status:      StatusPending,
	}

	if err := rm.scheduleStore.Save(ctx, schedule); err != nil {
		rm.emit(&RotationEvent{
			Type:      "schedule_error",
			CAID:      ca.ID,
			Timestamp: time.Now(),
			Message:   "Failed to schedule rotation",
			Error:     err.Error(),
		})
		return
	}

	rm.emit(&RotationEvent{
		Type:       "rotation_scheduled",
		CAID:       ca.ID,
		ScheduleID: schedule.ID,
		Timestamp:  time.Now(),
		Message:    fmt.Sprintf("Rotation scheduled for CA %s", ca.Name),
	})
}

func (rm *RotationManager) checkNotifications(ca *CAInfo, policy *RotationPolicy) {
	daysRemaining := int(ca.RemainingValidity().Hours() / 24)

	for _, day := range policy.NotifyBeforeDays {
		if daysRemaining == day {
			rm.emit(&RotationEvent{
				Type:      "expiry_notification",
				CAID:      ca.ID,
				Timestamp: time.Now(),
				Message:   fmt.Sprintf("CA %s expires in %d days", ca.Name, daysRemaining),
				Details: map[string]interface{}{
					"daysRemaining": daysRemaining,
					"expiresAt":     ca.NotAfter,
				},
			})
			break
		}
	}
}

// processPendingSchedules processes approved schedules.
func (rm *RotationManager) processPendingSchedules(ctx context.Context) {
	schedules, err := rm.scheduleStore.ListByStatus(ctx, StatusApproved)
	if err != nil {
		return
	}

	for _, schedule := range schedules {
		if schedule.ScheduledAt.Before(time.Now()) {
			_ = rm.executeRotation(ctx, schedule) //nolint:errcheck // best-effort scheduled rotation
		}
	}
}

// executeRotation executes a rotation.
func (rm *RotationManager) executeRotation(ctx context.Context, schedule *RotationSchedule) error {
	policy, ok := rm.GetPolicy(schedule.PolicyName)
	if !ok {
		return fmt.Errorf("policy not found: %s", schedule.PolicyName)
	}

	// Update status
	now := time.Now()
	schedule.Status = StatusInProgress
	schedule.StartedAt = &now
	if err := rm.scheduleStore.Save(ctx, schedule); err != nil {
		return err
	}

	rm.emit(&RotationEvent{
		Type:       "rotation_started",
		CAID:       schedule.CAID,
		ScheduleID: schedule.ID,
		Timestamp:  now,
		Message:    "Rotation started",
	})

	// Get current CA
	currentCA, err := rm.caStore.Get(ctx, schedule.CAID)
	if err != nil {
		return rm.failRotation(ctx, schedule, err)
	}

	// Generate new CA
	validityDays := int(policy.MaxValidityPeriod.Hours() / 24)
	newCA, err := rm.generator.Generate(ctx, &CAGenerationConfig{
		Name:         currentCA.Name,
		Subject:      currentCA.Subject,
		ValidityDays: validityDays,
		KeyConfig:    policy.KeyConfig,
		ParentID:     currentCA.ParentID,
		IsRoot:       currentCA.IsRoot,
		Metadata:     currentCA.Metadata,
	})
	if err != nil {
		return rm.failRotation(ctx, schedule, err)
	}

	// Set new CA state based on strategy
	switch policy.Strategy {
	case StrategyOverlap:
		newCA.State = CAStateActive
		currentCA.State = CAStateRotating
	case StrategyBlueGreen:
		newCA.State = CAStatePending
	case StrategyRolling:
		newCA.State = CAStatePending
	}

	// Save new CA
	if err := rm.caStore.Save(ctx, newCA); err != nil {
		return rm.failRotation(ctx, schedule, err)
	}

	// Update current CA state
	if err := rm.caStore.UpdateState(ctx, currentCA.ID, currentCA.State); err != nil {
		return rm.failRotation(ctx, schedule, err)
	}

	// Mark rotation as complete
	completedAt := time.Now()
	schedule.Status = StatusCompleted
	schedule.CompletedAt = &completedAt
	schedule.NewCAID = newCA.ID
	schedule.RollbackCAID = currentCA.ID
	if err := rm.scheduleStore.Save(ctx, schedule); err != nil {
		return err
	}

	rm.emit(&RotationEvent{
		Type:       "rotation_completed",
		CAID:       schedule.CAID,
		ScheduleID: schedule.ID,
		Timestamp:  completedAt,
		Message:    fmt.Sprintf("Rotation completed. New CA: %s", newCA.ID),
		Details: map[string]interface{}{
			"newCaId":     newCA.ID,
			"oldCaId":     currentCA.ID,
			"newNotAfter": newCA.NotAfter,
		},
	})

	return nil
}

func (rm *RotationManager) failRotation(ctx context.Context, schedule *RotationSchedule, err error) error {
	now := time.Now()
	schedule.Status = StatusFailed
	schedule.FailedAt = &now
	schedule.Error = err.Error()
	_ = rm.scheduleStore.Save(ctx, schedule) //nolint:errcheck // best-effort persistence

	rm.emit(&RotationEvent{
		Type:       "rotation_failed",
		CAID:       schedule.CAID,
		ScheduleID: schedule.ID,
		Timestamp:  now,
		Message:    "Rotation failed",
		Error:      err.Error(),
	})

	return err
}

// ApproveRotation approves a pending rotation.
func (rm *RotationManager) ApproveRotation(ctx context.Context, scheduleID, approver string) error {
	schedule, err := rm.scheduleStore.Get(ctx, scheduleID)
	if err != nil {
		return err
	}

	if schedule.Status != StatusPending {
		return fmt.Errorf("rotation is not pending: %s", schedule.Status)
	}

	now := time.Now()
	schedule.Status = StatusApproved
	schedule.ApprovedBy = approver
	schedule.ApprovedAt = &now

	if err := rm.scheduleStore.Save(ctx, schedule); err != nil {
		return err
	}

	rm.emit(&RotationEvent{
		Type:       "rotation_approved",
		CAID:       schedule.CAID,
		ScheduleID: schedule.ID,
		Timestamp:  now,
		Message:    fmt.Sprintf("Rotation approved by %s", approver),
	})

	return nil
}

// CancelRotation cancels a pending or approved rotation.
func (rm *RotationManager) CancelRotation(ctx context.Context, scheduleID string) error {
	schedule, err := rm.scheduleStore.Get(ctx, scheduleID)
	if err != nil {
		return err
	}

	if schedule.Status != StatusPending && schedule.Status != StatusApproved {
		return fmt.Errorf("cannot cancel rotation in status: %s", schedule.Status)
	}

	schedule.Status = StatusCancelled

	if err := rm.scheduleStore.Save(ctx, schedule); err != nil {
		return err
	}

	rm.emit(&RotationEvent{
		Type:       "rotation_cancelled",
		CAID:       schedule.CAID,
		ScheduleID: schedule.ID,
		Timestamp:  time.Now(),
		Message:    "Rotation cancelled",
	})

	return nil
}

// RollbackRotation rolls back a completed rotation.
func (rm *RotationManager) RollbackRotation(ctx context.Context, scheduleID string) error {
	schedule, err := rm.scheduleStore.Get(ctx, scheduleID)
	if err != nil {
		return err
	}

	if schedule.Status != StatusCompleted {
		return fmt.Errorf("cannot rollback rotation in status: %s", schedule.Status)
	}

	if schedule.RollbackCAID == "" {
		return fmt.Errorf("no rollback CA ID recorded")
	}

	// Reactivate old CA
	if err := rm.caStore.UpdateState(ctx, schedule.RollbackCAID, CAStateActive); err != nil {
		return err
	}

	// Deactivate new CA
	if schedule.NewCAID != "" {
		if err := rm.caStore.UpdateState(ctx, schedule.NewCAID, CAStateRevoked); err != nil {
			return err
		}
	}

	schedule.Status = StatusRolledBack

	if err := rm.scheduleStore.Save(ctx, schedule); err != nil {
		return err
	}

	rm.emit(&RotationEvent{
		Type:       "rotation_rolled_back",
		CAID:       schedule.CAID,
		ScheduleID: schedule.ID,
		Timestamp:  time.Now(),
		Message:    "Rotation rolled back",
	})

	return nil
}

// GetCA retrieves a CA by ID.
func (rm *RotationManager) GetCA(ctx context.Context, id string) (*CAInfo, error) {
	return rm.caStore.Get(ctx, id)
}

// ListCAs lists all CAs.
func (rm *RotationManager) ListCAs(ctx context.Context) ([]*CAInfo, error) {
	return rm.caStore.List(ctx)
}

// GetActiveCA retrieves the active CA.
func (rm *RotationManager) GetActiveCA(ctx context.Context) (*CAInfo, error) {
	cas, err := rm.caStore.ListByState(ctx, CAStateActive)
	if err != nil {
		return nil, err
	}
	if len(cas) == 0 {
		return nil, fmt.Errorf("no active CA found")
	}
	return cas[0], nil
}

// ListSchedules lists all rotation schedules.
func (rm *RotationManager) ListSchedules(ctx context.Context) ([]*RotationSchedule, error) {
	return rm.scheduleStore.List(ctx)
}

// GetSchedule retrieves a schedule by ID.
func (rm *RotationManager) GetSchedule(ctx context.Context, id string) (*RotationSchedule, error) {
	return rm.scheduleStore.Get(ctx, id)
}

// ScheduleRotationNow schedules an immediate rotation for a CA.
func (rm *RotationManager) ScheduleRotationNow(ctx context.Context, caID, policyName string) (*RotationSchedule, error) {
	ca, err := rm.caStore.Get(ctx, caID)
	if err != nil {
		return nil, err
	}

	if _, ok := rm.GetPolicy(policyName); !ok {
		return nil, fmt.Errorf("policy not found: %s", policyName)
	}

	schedule := &RotationSchedule{
		ID:          generateID(),
		CAID:        ca.ID,
		PolicyName:  policyName,
		ScheduledAt: time.Now(),
		Status:      StatusPending,
	}

	if err := rm.scheduleStore.Save(ctx, schedule); err != nil {
		return nil, err
	}

	rm.emit(&RotationEvent{
		Type:       "rotation_scheduled",
		CAID:       ca.ID,
		ScheduleID: schedule.ID,
		Timestamp:  time.Now(),
		Message:    fmt.Sprintf("Immediate rotation scheduled for CA %s", ca.Name),
	})

	return schedule, nil
}

// InMemoryCAStore is an in-memory implementation of CAStore.
type InMemoryCAStore struct {
	cas map[string]*CAInfo
	mu  sync.RWMutex
}

// NewInMemoryCAStore creates a new in-memory CA store.
func NewInMemoryCAStore() *InMemoryCAStore {
	return &InMemoryCAStore{
		cas: make(map[string]*CAInfo),
	}
}

// Get retrieves a CA by ID.
func (s *InMemoryCAStore) Get(_ context.Context, id string) (*CAInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ca, ok := s.cas[id]
	if !ok {
		return nil, fmt.Errorf("CA not found: %s", id)
	}

	// Return a copy
	data, _ := json.Marshal(ca)
	var copied CAInfo
	_ = json.Unmarshal(data, &copied)
	return &copied, nil
}

// List lists all CAs.
func (s *InMemoryCAStore) List(_ context.Context) ([]*CAInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*CAInfo, 0, len(s.cas))
	for _, ca := range s.cas {
		data, _ := json.Marshal(ca)
		var copied CAInfo
		_ = json.Unmarshal(data, &copied)
		result = append(result, &copied)
	}
	return result, nil
}

// ListByState lists CAs by state.
func (s *InMemoryCAStore) ListByState(_ context.Context, state CAState) ([]*CAInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*CAInfo
	for _, ca := range s.cas {
		if ca.State == state {
			data, _ := json.Marshal(ca)
			var copied CAInfo
			_ = json.Unmarshal(data, &copied)
			result = append(result, &copied)
		}
	}
	return result, nil
}

// Save saves a CA.
func (s *InMemoryCAStore) Save(_ context.Context, ca *CAInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Store a copy
	data, _ := json.Marshal(ca)
	var copied CAInfo
	_ = json.Unmarshal(data, &copied)
	s.cas[ca.ID] = &copied
	return nil
}

// UpdateState updates a CA's state.
func (s *InMemoryCAStore) UpdateState(_ context.Context, id string, state CAState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ca, ok := s.cas[id]
	if !ok {
		return fmt.Errorf("CA not found: %s", id)
	}
	ca.State = state
	return nil
}

// Delete deletes a CA.
func (s *InMemoryCAStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cas, id)
	return nil
}

// InMemoryScheduleStore is an in-memory implementation of ScheduleStore.
type InMemoryScheduleStore struct {
	schedules map[string]*RotationSchedule
	mu        sync.RWMutex
}

// NewInMemoryScheduleStore creates a new in-memory schedule store.
func NewInMemoryScheduleStore() *InMemoryScheduleStore {
	return &InMemoryScheduleStore{
		schedules: make(map[string]*RotationSchedule),
	}
}

// Get retrieves a schedule by ID.
func (s *InMemoryScheduleStore) Get(_ context.Context, id string) (*RotationSchedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	schedule, ok := s.schedules[id]
	if !ok {
		return nil, fmt.Errorf("schedule not found: %s", id)
	}

	data, _ := json.Marshal(schedule)
	var copied RotationSchedule
	_ = json.Unmarshal(data, &copied)
	return &copied, nil
}

// List lists all schedules.
func (s *InMemoryScheduleStore) List(_ context.Context) ([]*RotationSchedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*RotationSchedule, 0, len(s.schedules))
	for _, schedule := range s.schedules {
		data, _ := json.Marshal(schedule)
		var copied RotationSchedule
		_ = json.Unmarshal(data, &copied)
		result = append(result, &copied)
	}
	return result, nil
}

// ListByCA lists schedules for a CA.
func (s *InMemoryScheduleStore) ListByCA(_ context.Context, caID string) ([]*RotationSchedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*RotationSchedule
	for _, schedule := range s.schedules {
		if schedule.CAID == caID {
			data, _ := json.Marshal(schedule)
			var copied RotationSchedule
			_ = json.Unmarshal(data, &copied)
			result = append(result, &copied)
		}
	}
	return result, nil
}

// ListByStatus lists schedules by status.
func (s *InMemoryScheduleStore) ListByStatus(_ context.Context, status ScheduleStatus) ([]*RotationSchedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*RotationSchedule
	for _, schedule := range s.schedules {
		if schedule.Status == status {
			data, _ := json.Marshal(schedule)
			var copied RotationSchedule
			_ = json.Unmarshal(data, &copied)
			result = append(result, &copied)
		}
	}
	return result, nil
}

// Save saves a schedule.
func (s *InMemoryScheduleStore) Save(_ context.Context, schedule *RotationSchedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, _ := json.Marshal(schedule)
	var copied RotationSchedule
	_ = json.Unmarshal(data, &copied)
	s.schedules[schedule.ID] = &copied
	return nil
}

// Delete deletes a schedule.
func (s *InMemoryScheduleStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.schedules, id)
	return nil
}

// MockCAGenerator is a mock CA generator for testing.
type MockCAGenerator struct{}

// Generate generates a mock CA.
func (g *MockCAGenerator) Generate(_ context.Context, config *CAGenerationConfig) (*CAInfo, error) {
	now := time.Now()
	validity := time.Duration(config.ValidityDays) * 24 * time.Hour

	return &CAInfo{
		ID:           generateID(),
		Name:         config.Name,
		State:        CAStatePending,
		SerialNumber: generateSerial(),
		Subject:      config.Subject,
		NotBefore:    now,
		NotAfter:     now.Add(validity),
		KeyType:      config.KeyConfig.Type,
		KeySize:      config.KeyConfig.Size,
		SignatureAlg: config.KeyConfig.Algorithm,
		Fingerprint:  generateFingerprint(),
		IsRoot:       config.IsRoot,
		ParentID:     config.ParentID,
		CreatedAt:    now,
		Metadata:     config.Metadata,
	}, nil
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateSerial() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateFingerprint() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ValidateCertificate validates a certificate against a CA.
func ValidateCertificate(cert, ca *x509.Certificate) error {
	// Check if certificate is signed by CA
	if err := cert.CheckSignatureFrom(ca); err != nil {
		return fmt.Errorf("certificate not signed by CA: %w", err)
	}

	// Check validity period
	now := time.Now()
	if now.Before(cert.NotBefore) {
		return fmt.Errorf("certificate not yet valid")
	}
	if now.After(cert.NotAfter) {
		return fmt.Errorf("certificate has expired")
	}

	return nil
}
