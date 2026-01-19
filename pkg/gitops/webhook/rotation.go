package webhook

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SecretVersion represents a versioned secret
type SecretVersion struct {
	// ID is the unique identifier for this secret version
	ID string `json:"id"`

	// Version is the sequential version number
	Version int `json:"version"`

	// Secret is the actual secret value
	Secret string `json:"secret"`

	// CreatedAt is when this version was created
	CreatedAt time.Time `json:"created_at"`

	// ExpiresAt is when this version expires (optional, zero = no expiry)
	ExpiresAt time.Time `json:"expires_at,omitempty"`

	// RotatedAt is when this was rotated out (zero = still active)
	RotatedAt time.Time `json:"rotated_at,omitempty"`

	// Status indicates the version's current status
	Status SecretStatus `json:"status"`

	// Metadata contains additional version information
	Metadata map[string]string `json:"metadata,omitempty"`
}

// SecretStatus represents the status of a secret version
type SecretStatus string

const (
	// SecretStatusActive means the secret is currently in use
	SecretStatusActive SecretStatus = "active"

	// SecretStatusPending means the secret is newly created but not yet active
	SecretStatusPending SecretStatus = "pending"

	// SecretStatusGracePeriod means the secret has been rotated but is still valid
	SecretStatusGracePeriod SecretStatus = "grace_period"

	// SecretStatusExpired means the secret is no longer valid
	SecretStatusExpired SecretStatus = "expired"

	// SecretStatusRevoked means the secret was manually revoked
	SecretStatusRevoked SecretStatus = "revoked"
)

// RotationConfig configures secret rotation behavior
type RotationConfig struct {
	// Enabled enables automatic rotation
	Enabled bool `json:"enabled"`

	// RotationInterval is how often to rotate secrets
	RotationInterval time.Duration `json:"rotation_interval"`

	// GracePeriod is how long old secrets remain valid after rotation
	GracePeriod time.Duration `json:"grace_period"`

	// SecretLength is the length of generated secrets in bytes
	SecretLength int `json:"secret_length"`

	// MaxVersions is the maximum number of versions to keep
	MaxVersions int `json:"max_versions"`

	// NotifyOnRotation enables notifications when secrets rotate
	NotifyOnRotation bool `json:"notify_on_rotation"`

	// NotifyBeforeExpiry is how long before expiry to notify
	NotifyBeforeExpiry time.Duration `json:"notify_before_expiry"`
}

// DefaultRotationConfig returns sensible defaults
func DefaultRotationConfig() *RotationConfig {
	return &RotationConfig{
		Enabled:            true,
		RotationInterval:   30 * 24 * time.Hour, // 30 days
		GracePeriod:        24 * time.Hour,      // 24 hours
		SecretLength:       32,                  // 256 bits
		MaxVersions:        5,
		NotifyOnRotation:   true,
		NotifyBeforeExpiry: 7 * 24 * time.Hour, // 7 days before
	}
}

// RotationEvent represents a rotation-related event
type RotationEvent struct {
	// Type is the event type
	Type RotationEventType `json:"type"`

	// WebhookID identifies the webhook
	WebhookID string `json:"webhook_id"`

	// SecretVersionID is the affected secret version
	SecretVersionID string `json:"secret_version_id"`

	// Version is the version number
	Version int `json:"version"`

	// Timestamp is when the event occurred
	Timestamp time.Time `json:"timestamp"`

	// Details contains additional event details
	Details map[string]interface{} `json:"details,omitempty"`
}

// RotationEventType represents types of rotation events
type RotationEventType string

const (
	RotationEventCreated        RotationEventType = "created"
	RotationEventRotated        RotationEventType = "rotated"
	RotationEventExpired        RotationEventType = "expired"
	RotationEventRevoked        RotationEventType = "revoked"
	RotationEventGracePeriodEnd RotationEventType = "grace_period_end"
	RotationEventExpiryWarning  RotationEventType = "expiry_warning"
)

// RotationCallback is called when rotation events occur
type RotationCallback func(event *RotationEvent)

// SecretStore interface for storing secrets
type SecretStore interface {
	// GetVersions returns all versions for a webhook
	GetVersions(ctx context.Context, webhookID string) ([]*SecretVersion, error)

	// GetActiveSecret returns the currently active secret
	GetActiveSecret(ctx context.Context, webhookID string) (*SecretVersion, error)

	// GetValidSecrets returns all secrets that can be used for validation
	GetValidSecrets(ctx context.Context, webhookID string) ([]*SecretVersion, error)

	// SaveVersion saves a new secret version
	SaveVersion(ctx context.Context, webhookID string, version *SecretVersion) error

	// UpdateVersionStatus updates a version's status
	UpdateVersionStatus(ctx context.Context, webhookID string, versionID string, status SecretStatus) error

	// DeleteVersion deletes a secret version
	DeleteVersion(ctx context.Context, webhookID string, versionID string) error

	// ListWebhooks returns all webhook IDs
	ListWebhooks(ctx context.Context) ([]string, error)
}

// SecretRotator manages automatic secret rotation
type SecretRotator struct {
	config    *RotationConfig
	store     SecretStore
	callbacks []RotationCallback
	mu        sync.RWMutex

	// Scheduling
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewSecretRotator creates a new secret rotator
func NewSecretRotator(config *RotationConfig, store SecretStore) *SecretRotator {
	if config == nil {
		config = DefaultRotationConfig()
	}

	return &SecretRotator{
		config:    config,
		store:     store,
		callbacks: make([]RotationCallback, 0),
		stopCh:    make(chan struct{}),
	}
}

// OnRotation registers a callback for rotation events
func (r *SecretRotator) OnRotation(callback RotationCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callbacks = append(r.callbacks, callback)
}

// emitEvent emits a rotation event to all callbacks
func (r *SecretRotator) emitEvent(event *RotationEvent) {
	r.mu.RLock()
	callbacks := make([]RotationCallback, len(r.callbacks))
	copy(callbacks, r.callbacks)
	r.mu.RUnlock()

	for _, cb := range callbacks {
		cb(event)
	}
}

// Start starts the automatic rotation scheduler
func (r *SecretRotator) Start(ctx context.Context) error {
	if !r.config.Enabled {
		return nil
	}

	r.wg.Add(1)
	go r.rotationLoop(ctx)

	return nil
}

// Stop stops the rotation scheduler
func (r *SecretRotator) Stop() {
	r.stopOnce.Do(func() {
		close(r.stopCh)
	})
	r.wg.Wait()
}

// rotationLoop runs the periodic rotation check
func (r *SecretRotator) rotationLoop(ctx context.Context) {
	defer r.wg.Done()

	// Check immediately on start
	r.checkRotations(ctx)

	// Check every hour
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.checkRotations(ctx)
		}
	}
}

// checkRotations checks all webhooks for needed rotations
func (r *SecretRotator) checkRotations(ctx context.Context) {
	webhooks, err := r.store.ListWebhooks(ctx)
	if err != nil {
		return
	}

	for _, webhookID := range webhooks {
		r.checkWebhookRotation(ctx, webhookID)
	}
}

// checkWebhookRotation checks a single webhook for rotation
func (r *SecretRotator) checkWebhookRotation(ctx context.Context, webhookID string) {
	active, err := r.store.GetActiveSecret(ctx, webhookID)
	if err != nil {
		return
	}

	if active == nil {
		// No active secret, create initial one
		r.CreateInitialSecret(ctx, webhookID)
		return
	}

	now := time.Now()

	// Check if we should notify about expiry
	if r.config.NotifyOnRotation && r.config.NotifyBeforeExpiry > 0 {
		nextRotation := active.CreatedAt.Add(r.config.RotationInterval)
		warningTime := nextRotation.Add(-r.config.NotifyBeforeExpiry)
		if now.After(warningTime) && now.Before(nextRotation) {
			r.emitEvent(&RotationEvent{
				Type:            RotationEventExpiryWarning,
				WebhookID:       webhookID,
				SecretVersionID: active.ID,
				Version:         active.Version,
				Timestamp:       now,
				Details: map[string]interface{}{
					"expires_at":    nextRotation,
					"days_remaining": int(nextRotation.Sub(now).Hours() / 24),
				},
			})
		}
	}

	// Check if rotation is needed
	if now.After(active.CreatedAt.Add(r.config.RotationInterval)) {
		r.RotateSecret(ctx, webhookID)
	}

	// Check for grace period expirations
	r.checkGracePeriodExpirations(ctx, webhookID)
}

// checkGracePeriodExpirations expires old secrets past grace period
func (r *SecretRotator) checkGracePeriodExpirations(ctx context.Context, webhookID string) {
	versions, err := r.store.GetVersions(ctx, webhookID)
	if err != nil {
		return
	}

	now := time.Now()

	for _, v := range versions {
		if v.Status == SecretStatusGracePeriod {
			gracePeriodEnd := v.RotatedAt.Add(r.config.GracePeriod)
			if now.After(gracePeriodEnd) {
				r.store.UpdateVersionStatus(ctx, webhookID, v.ID, SecretStatusExpired)
				r.emitEvent(&RotationEvent{
					Type:            RotationEventGracePeriodEnd,
					WebhookID:       webhookID,
					SecretVersionID: v.ID,
					Version:         v.Version,
					Timestamp:       now,
				})
			}
		}
	}
}

// CreateInitialSecret creates the first secret for a webhook
func (r *SecretRotator) CreateInitialSecret(ctx context.Context, webhookID string) (*SecretVersion, error) {
	secret, err := r.generateSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}

	now := time.Now()
	version := &SecretVersion{
		ID:        generateVersionID(),
		Version:   1,
		Secret:    secret,
		CreatedAt: now,
		Status:    SecretStatusActive,
		Metadata:  make(map[string]string),
	}

	if err := r.store.SaveVersion(ctx, webhookID, version); err != nil {
		return nil, fmt.Errorf("failed to save secret version: %w", err)
	}

	r.emitEvent(&RotationEvent{
		Type:            RotationEventCreated,
		WebhookID:       webhookID,
		SecretVersionID: version.ID,
		Version:         version.Version,
		Timestamp:       now,
	})

	return version, nil
}

// RotateSecret rotates to a new secret version
func (r *SecretRotator) RotateSecret(ctx context.Context, webhookID string) (*SecretVersion, error) {
	// Get current active version
	current, err := r.store.GetActiveSecret(ctx, webhookID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active secret: %w", err)
	}

	// Generate new secret
	secret, err := r.generateSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}

	now := time.Now()
	newVersion := 1
	if current != nil {
		newVersion = current.Version + 1
	}

	// Create new version
	version := &SecretVersion{
		ID:        generateVersionID(),
		Version:   newVersion,
		Secret:    secret,
		CreatedAt: now,
		Status:    SecretStatusActive,
		Metadata:  make(map[string]string),
	}

	// Mark old version as in grace period
	if current != nil {
		current.Status = SecretStatusGracePeriod
		current.RotatedAt = now
		if err := r.store.UpdateVersionStatus(ctx, webhookID, current.ID, SecretStatusGracePeriod); err != nil {
			return nil, fmt.Errorf("failed to update old secret status: %w", err)
		}
	}

	// Save new version
	if err := r.store.SaveVersion(ctx, webhookID, version); err != nil {
		return nil, fmt.Errorf("failed to save new secret version: %w", err)
	}

	// Clean up old versions
	r.cleanupOldVersions(ctx, webhookID)

	// Emit event
	r.emitEvent(&RotationEvent{
		Type:            RotationEventRotated,
		WebhookID:       webhookID,
		SecretVersionID: version.ID,
		Version:         version.Version,
		Timestamp:       now,
		Details: map[string]interface{}{
			"previous_version": current.Version,
			"grace_period_end": now.Add(r.config.GracePeriod),
		},
	})

	return version, nil
}

// RevokeSecret revokes a specific secret version
func (r *SecretRotator) RevokeSecret(ctx context.Context, webhookID string, versionID string) error {
	if err := r.store.UpdateVersionStatus(ctx, webhookID, versionID, SecretStatusRevoked); err != nil {
		return fmt.Errorf("failed to revoke secret: %w", err)
	}

	versions, _ := r.store.GetVersions(ctx, webhookID)
	var version int
	for _, v := range versions {
		if v.ID == versionID {
			version = v.Version
			break
		}
	}

	r.emitEvent(&RotationEvent{
		Type:            RotationEventRevoked,
		WebhookID:       webhookID,
		SecretVersionID: versionID,
		Version:         version,
		Timestamp:       time.Now(),
	})

	return nil
}

// cleanupOldVersions removes expired versions beyond MaxVersions
func (r *SecretRotator) cleanupOldVersions(ctx context.Context, webhookID string) {
	if r.config.MaxVersions <= 0 {
		return
	}

	versions, err := r.store.GetVersions(ctx, webhookID)
	if err != nil {
		return
	}

	// Count non-expired versions
	validCount := 0
	for _, v := range versions {
		if v.Status != SecretStatusExpired && v.Status != SecretStatusRevoked {
			validCount++
		}
	}

	// Delete expired versions if we have too many
	if len(versions) > r.config.MaxVersions {
		for _, v := range versions {
			if v.Status == SecretStatusExpired || v.Status == SecretStatusRevoked {
				r.store.DeleteVersion(ctx, webhookID, v.ID)
			}
		}
	}
}

// generateSecret generates a cryptographically secure random secret
func (r *SecretRotator) generateSecret() (string, error) {
	length := r.config.SecretLength
	if length <= 0 {
		length = 32
	}

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(bytes), nil
}

// generateVersionID generates a unique version ID
func generateVersionID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return fmt.Sprintf("v-%d-%s", time.Now().UnixNano(), base64.URLEncoding.EncodeToString(bytes)[:8])
}

// RotatingAuthenticator validates against multiple secret versions
type RotatingAuthenticator struct {
	rotator   *SecretRotator
	webhookID string
	header    string
}

// NewRotatingAuthenticator creates an authenticator that supports rotation
func NewRotatingAuthenticator(rotator *SecretRotator, webhookID string) *RotatingAuthenticator {
	return &RotatingAuthenticator{
		rotator:   rotator,
		webhookID: webhookID,
		header:    "X-Hub-Signature-256",
	}
}

// WithHeader sets the signature header
func (a *RotatingAuthenticator) WithHeader(header string) *RotatingAuthenticator {
	a.header = header
	return a
}

// Authenticate validates the request against all valid secrets
func (a *RotatingAuthenticator) Authenticate(r *http.Request, body []byte) error {
	ctx := r.Context()

	validSecrets, err := a.rotator.store.GetValidSecrets(ctx, a.webhookID)
	if err != nil {
		return fmt.Errorf("failed to get valid secrets: %w", err)
	}

	if len(validSecrets) == 0 {
		return fmt.Errorf("no valid secrets configured for webhook")
	}

	// Try each valid secret
	for _, sv := range validSecrets {
		auth := &HMACAuthenticator{
			Secret: sv.Secret,
			Header: a.header,
		}
		if err := auth.Authenticate(r, body); err == nil {
			return nil // Found a valid secret
		}
	}

	return fmt.Errorf("invalid signature - no matching secret version")
}

// InMemorySecretStore is an in-memory implementation of SecretStore
type InMemorySecretStore struct {
	mu       sync.RWMutex
	webhooks map[string][]*SecretVersion
}

// NewInMemorySecretStore creates a new in-memory secret store
func NewInMemorySecretStore() *InMemorySecretStore {
	return &InMemorySecretStore{
		webhooks: make(map[string][]*SecretVersion),
	}
}

// GetVersions returns all versions for a webhook
func (s *InMemorySecretStore) GetVersions(ctx context.Context, webhookID string) ([]*SecretVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	versions := s.webhooks[webhookID]
	result := make([]*SecretVersion, len(versions))
	copy(result, versions)
	return result, nil
}

// GetActiveSecret returns the currently active secret
func (s *InMemorySecretStore) GetActiveSecret(ctx context.Context, webhookID string) (*SecretVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	versions := s.webhooks[webhookID]
	for _, v := range versions {
		if v.Status == SecretStatusActive {
			return v, nil
		}
	}
	return nil, nil
}

// GetValidSecrets returns all secrets that can be used for validation
func (s *InMemorySecretStore) GetValidSecrets(ctx context.Context, webhookID string) ([]*SecretVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	versions := s.webhooks[webhookID]
	valid := make([]*SecretVersion, 0)
	for _, v := range versions {
		if v.Status == SecretStatusActive || v.Status == SecretStatusGracePeriod {
			valid = append(valid, v)
		}
	}
	return valid, nil
}

// SaveVersion saves a new secret version
func (s *InMemorySecretStore) SaveVersion(ctx context.Context, webhookID string, version *SecretVersion) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.webhooks[webhookID] = append(s.webhooks[webhookID], version)
	return nil
}

// UpdateVersionStatus updates a version's status
func (s *InMemorySecretStore) UpdateVersionStatus(ctx context.Context, webhookID string, versionID string, status SecretStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	versions := s.webhooks[webhookID]
	for _, v := range versions {
		if v.ID == versionID {
			v.Status = status
			if status == SecretStatusGracePeriod {
				v.RotatedAt = time.Now()
			}
			return nil
		}
	}
	return fmt.Errorf("version not found: %s", versionID)
}

// DeleteVersion deletes a secret version
func (s *InMemorySecretStore) DeleteVersion(ctx context.Context, webhookID string, versionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	versions := s.webhooks[webhookID]
	for i, v := range versions {
		if v.ID == versionID {
			s.webhooks[webhookID] = append(versions[:i], versions[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("version not found: %s", versionID)
}

// ListWebhooks returns all webhook IDs
func (s *InMemorySecretStore) ListWebhooks(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.webhooks))
	for id := range s.webhooks {
		ids = append(ids, id)
	}
	return ids, nil
}

// RotationStatus returns the current rotation status for a webhook
type RotationStatus struct {
	WebhookID       string         `json:"webhook_id"`
	ActiveVersion   *SecretVersion `json:"active_version"`
	ValidVersions   int            `json:"valid_versions"`
	TotalVersions   int            `json:"total_versions"`
	NextRotation    time.Time      `json:"next_rotation"`
	LastRotation    time.Time      `json:"last_rotation"`
	GracePeriodEnds time.Time      `json:"grace_period_ends,omitempty"`
}

// GetRotationStatus returns the current rotation status
func (r *SecretRotator) GetRotationStatus(ctx context.Context, webhookID string) (*RotationStatus, error) {
	active, err := r.store.GetActiveSecret(ctx, webhookID)
	if err != nil {
		return nil, err
	}

	versions, _ := r.store.GetVersions(ctx, webhookID)
	validSecrets, _ := r.store.GetValidSecrets(ctx, webhookID)

	status := &RotationStatus{
		WebhookID:     webhookID,
		ActiveVersion: active,
		ValidVersions: len(validSecrets),
		TotalVersions: len(versions),
	}

	if active != nil {
		status.NextRotation = active.CreatedAt.Add(r.config.RotationInterval)
		status.LastRotation = active.CreatedAt
	}

	// Check for grace period
	for _, v := range versions {
		if v.Status == SecretStatusGracePeriod {
			status.GracePeriodEnds = v.RotatedAt.Add(r.config.GracePeriod)
			break
		}
	}

	return status, nil
}

// ForceRotation forces an immediate rotation regardless of schedule
func (r *SecretRotator) ForceRotation(ctx context.Context, webhookID string) (*SecretVersion, error) {
	return r.RotateSecret(ctx, webhookID)
}

// GetConfig returns the current rotation configuration
func (r *SecretRotator) GetConfig() *RotationConfig {
	return r.config
}

// UpdateConfig updates the rotation configuration
func (r *SecretRotator) UpdateConfig(config *RotationConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config = config
}
