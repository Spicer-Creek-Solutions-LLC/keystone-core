package k8s

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets/injection"
)

// SecretSync synchronizes secrets from external backends to Kubernetes secrets.
type SecretSync struct {
	config *SyncConfig
	source injection.SecretSource
	client KubernetesClient

	mu      sync.RWMutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}

	// Track synced secrets
	syncedSecrets map[string]string // namespace/name -> source version

	// Stats
	stats SyncStats
}

// KubernetesClient is an interface for Kubernetes operations.
type KubernetesClient interface {
	// CreateSecret creates a Kubernetes secret.
	CreateSecret(ctx context.Context, namespace, name string, data map[string][]byte, secretType string, labels, annotations map[string]string) error

	// UpdateSecret updates a Kubernetes secret.
	UpdateSecret(ctx context.Context, namespace, name string, data map[string][]byte, labels, annotations map[string]string) error

	// DeleteSecret deletes a Kubernetes secret.
	DeleteSecret(ctx context.Context, namespace, name string) error

	// GetSecret gets a Kubernetes secret.
	GetSecret(ctx context.Context, namespace, name string) (*Secret, error)

	// ListSecrets lists Kubernetes secrets with a label selector.
	ListSecrets(ctx context.Context, namespace string, labelSelector map[string]string) ([]*Secret, error)
}

// Secret represents a Kubernetes secret.
type Secret struct {
	Name        string
	Namespace   string
	Data        map[string][]byte
	Type        string
	Labels      map[string]string
	Annotations map[string]string
	Version     string
}

// SyncStats contains sync statistics.
type SyncStats struct {
	StartTime      time.Time
	LastSyncTime   time.Time
	SyncCount      int64
	SyncErrors     int64
	SecretsCreated int64
	SecretsUpdated int64
	SecretsDeleted int64
}

// NewSecretSync creates a new secret sync controller.
func NewSecretSync(config *SyncConfig, source injection.SecretSource, client KubernetesClient) (*SecretSync, error) {
	if config == nil {
		config = DefaultSyncConfig()
	}
	if source == nil {
		return nil, fmt.Errorf("source is required")
	}
	if client == nil {
		return nil, fmt.Errorf("kubernetes client is required")
	}

	return &SecretSync{
		config:        config,
		source:        source,
		client:        client,
		syncedSecrets: make(map[string]string),
		stats: SyncStats{
			StartTime: time.Now(),
		},
	}, nil
}

// Run starts the sync controller and blocks until stopped.
func (s *SecretSync) Run(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("sync already running")
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	s.mu.Unlock()

	defer func() {
		close(s.doneCh)
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	// Initial sync
	if err := s.sync(ctx); err != nil {
		slog.Warn("initial sync failed", "error", err)
	}

	// Start sync loop
	ticker := time.NewTicker(s.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.stopCh:
			return nil
		case <-ticker.C:
			if err := s.sync(ctx); err != nil {
				slog.Error("sync error", "error", err)
				s.mu.Lock()
				s.stats.SyncErrors++
				s.mu.Unlock()
			}
		}
	}
}

// Stop stops the sync controller.
func (s *SecretSync) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	close(s.stopCh)
	<-s.doneCh
	return nil
}

// Stats returns the sync statistics.
func (s *SecretSync) Stats() SyncStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// SyncOnce performs a single sync operation.
func (s *SecretSync) SyncOnce(ctx context.Context) error {
	return s.sync(ctx)
}

func (s *SecretSync) sync(ctx context.Context) error {
	s.mu.Lock()
	s.stats.SyncCount++
	s.stats.LastSyncTime = time.Now()
	s.mu.Unlock()

	// Track secrets we've synced this round
	syncedThisRound := make(map[string]bool)

	// Sync each configured secret
	for _, spec := range s.config.Secrets {
		namespace := spec.DestNamespace
		if namespace == "" {
			namespace = s.config.Namespace
		}
		if namespace == "" {
			namespace = "default"
		}

		key := fmt.Sprintf("%s/%s", namespace, spec.DestName)
		syncedThisRound[key] = true

		if err := s.syncSecret(ctx, spec, namespace); err != nil {
			slog.Error("failed to sync secret", "key", key, "error", err)
		}
	}

	// Delete orphaned secrets if configured
	if s.config.DeleteOrphans {
		if err := s.deleteOrphans(ctx, syncedThisRound); err != nil {
			slog.Error("failed to delete orphans", "error", err)
		}
	}

	return nil
}

func (s *SecretSync) syncSecret(ctx context.Context, spec SyncSecretSpec, namespace string) error {
	// Fetch secret from source
	secret, err := s.source.GetSecret(ctx, spec.SourcePath)
	if err != nil {
		return fmt.Errorf("failed to get secret from source: %w", err)
	}
	if secret == nil {
		return fmt.Errorf("secret not found: %s", spec.SourcePath)
	}

	// Convert secret data to bytes
	data := make(map[string][]byte)
	for k, v := range secret.Data {
		destKey := k
		if spec.KeyMapping != nil {
			if mapped, ok := spec.KeyMapping[k]; ok {
				destKey = mapped
			}
		}

		switch val := v.(type) {
		case []byte:
			data[destKey] = val
		case string:
			data[destKey] = []byte(val)
		default:
			// Try JSON encoding for complex types
			jsonData, err := json.Marshal(val)
			if err != nil {
				return fmt.Errorf("failed to convert key %s: %w", k, err)
			}
			data[destKey] = jsonData
		}
	}

	// Build labels
	labels := make(map[string]string)
	labels["app.kubernetes.io/managed-by"] = "keystone-secrets"
	labels["secrets.keystone.io/synced"] = "true"
	for k, v := range spec.Labels {
		labels[k] = v
	}

	// Build annotations
	annotations := make(map[string]string)
	annotations["secrets.keystone.io/source"] = spec.SourcePath
	annotations["secrets.keystone.io/sync-time"] = time.Now().Format(time.RFC3339)
	annotations["secrets.keystone.io/version"] = fmt.Sprintf("%d", secret.Version)
	for k, v := range spec.Annotations {
		annotations[k] = v
	}

	// Determine secret type
	secretType := spec.Type
	if secretType == "" {
		secretType = "Opaque"
	}

	// Check if secret exists
	key := fmt.Sprintf("%s/%s", namespace, spec.DestName)
	existing, err := s.client.GetSecret(ctx, namespace, spec.DestName)

	if err != nil || existing == nil {
		// Create new secret
		if err := s.client.CreateSecret(ctx, namespace, spec.DestName, data, secretType, labels, annotations); err != nil {
			return fmt.Errorf("failed to create secret: %w", err)
		}
		s.mu.Lock()
		s.stats.SecretsCreated++
		s.syncedSecrets[key] = fmt.Sprintf("%d", secret.Version)
		s.mu.Unlock()
		slog.Info("created secret", "namespace", namespace, "name", spec.DestName)
	} else {
		// Check if update is needed
		existingVersion := existing.Annotations["secrets.keystone.io/version"]
		currentVersion := fmt.Sprintf("%d", secret.Version)

		if existingVersion != currentVersion || s.dataChanged(existing.Data, data) {
			if err := s.client.UpdateSecret(ctx, namespace, spec.DestName, data, labels, annotations); err != nil {
				return fmt.Errorf("failed to update secret: %w", err)
			}
			s.mu.Lock()
			s.stats.SecretsUpdated++
			s.syncedSecrets[key] = currentVersion
			s.mu.Unlock()
			slog.Info("updated secret", "namespace", namespace, "name", spec.DestName)
		}
	}

	return nil
}

func (s *SecretSync) dataChanged(existing, updated map[string][]byte) bool {
	if len(existing) != len(updated) {
		return true
	}
	for k, v := range updated {
		if ev, ok := existing[k]; !ok || !bytes.Equal(ev, v) {
			return true
		}
	}
	return false
}

func (s *SecretSync) deleteOrphans(ctx context.Context, synced map[string]bool) error {
	// List secrets managed by keystone
	labelSelector := map[string]string{
		"secrets.keystone.io/synced": "true",
	}

	namespace := s.config.Namespace
	if namespace == "" {
		// Would need to list across all namespaces
		// For now, skip orphan deletion without specific namespace
		return nil
	}

	secrets, err := s.client.ListSecrets(ctx, namespace, labelSelector)
	if err != nil {
		return fmt.Errorf("failed to list secrets: %w", err)
	}

	for _, secret := range secrets {
		key := fmt.Sprintf("%s/%s", secret.Namespace, secret.Name)
		if !synced[key] {
			if err := s.client.DeleteSecret(ctx, secret.Namespace, secret.Name); err != nil {
				slog.Error("failed to delete orphan", "key", key, "error", err)
			} else {
				s.mu.Lock()
				s.stats.SecretsDeleted++
				delete(s.syncedSecrets, key)
				s.mu.Unlock()
				slog.Info("deleted orphan secret", "key", key)
			}
		}
	}

	return nil
}

// =============================================================================
// Mock Kubernetes Client (for testing)
// =============================================================================

// MockKubernetesClient is a mock implementation for testing.
type MockKubernetesClient struct {
	mu      sync.RWMutex
	secrets map[string]*Secret
}

// NewMockKubernetesClient creates a new mock client.
func NewMockKubernetesClient() *MockKubernetesClient {
	return &MockKubernetesClient{
		secrets: make(map[string]*Secret),
	}
}

func (m *MockKubernetesClient) key(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}

// CreateSecret creates a Kubernetes secret.
func (m *MockKubernetesClient) CreateSecret(ctx context.Context, namespace, name string, data map[string][]byte, secretType string, labels, annotations map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.key(namespace, name)
	if _, exists := m.secrets[key]; exists {
		return fmt.Errorf("secret already exists: %s", key)
	}

	m.secrets[key] = &Secret{
		Name:        name,
		Namespace:   namespace,
		Data:        data,
		Type:        secretType,
		Labels:      labels,
		Annotations: annotations,
	}
	return nil
}

// UpdateSecret updates a Kubernetes secret.
func (m *MockKubernetesClient) UpdateSecret(ctx context.Context, namespace, name string, data map[string][]byte, labels, annotations map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.key(namespace, name)
	secret, exists := m.secrets[key]
	if !exists {
		return fmt.Errorf("secret not found: %s", key)
	}

	secret.Data = data
	secret.Labels = labels
	secret.Annotations = annotations
	return nil
}

// DeleteSecret deletes a Kubernetes secret.
func (m *MockKubernetesClient) DeleteSecret(ctx context.Context, namespace, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.key(namespace, name)
	if _, exists := m.secrets[key]; !exists {
		return fmt.Errorf("secret not found: %s", key)
	}

	delete(m.secrets, key)
	return nil
}

// GetSecret retrieves a Kubernetes secret.
func (m *MockKubernetesClient) GetSecret(ctx context.Context, namespace, name string) (*Secret, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := m.key(namespace, name)
	if secret, exists := m.secrets[key]; exists {
		return secret, nil
	}
	return nil, nil
}

// ListSecrets lists Kubernetes secrets.
func (m *MockKubernetesClient) ListSecrets(ctx context.Context, namespace string, labelSelector map[string]string) ([]*Secret, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Secret
	for _, secret := range m.secrets {
		if namespace != "" && secret.Namespace != namespace {
			continue
		}

		// Check label selector
		match := true
		for k, v := range labelSelector {
			if secret.Labels[k] != v {
				match = false
				break
			}
		}

		if match {
			result = append(result, secret)
		}
	}

	return result, nil
}

// EncodeSecretData encodes secret data for Kubernetes.
func EncodeSecretData(data map[string]interface{}) (map[string][]byte, error) {
	result := make(map[string][]byte)
	for k, v := range data {
		switch val := v.(type) {
		case []byte:
			result[k] = val
		case string:
			result[k] = []byte(val)
		default:
			jsonData, err := json.Marshal(val)
			if err != nil {
				return nil, fmt.Errorf("failed to encode %s: %w", k, err)
			}
			result[k] = jsonData
		}
	}
	return result, nil
}

// DecodeSecretData decodes Kubernetes secret data.
func DecodeSecretData(data map[string][]byte) map[string]string {
	result := make(map[string]string)
	for k, v := range data {
		result[k] = string(v)
	}
	return result
}

// Base64EncodeData base64 encodes secret data (for YAML output).
func Base64EncodeData(data map[string][]byte) map[string]string {
	result := make(map[string]string)
	for k, v := range data {
		result[k] = base64.StdEncoding.EncodeToString(v)
	}
	return result
}
