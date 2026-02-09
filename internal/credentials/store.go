// Package credentials provides secure credential management for proxy agents.
package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// CredentialStore provides secure credential storage and retrieval.
type CredentialStore interface {
	// Get retrieves a credential by ID.
	Get(ctx context.Context, id string) (Credential, error)

	// Store stores a credential.
	Store(ctx context.Context, cred Credential) error

	// Delete deletes a credential by ID.
	Delete(ctx context.Context, id string) error

	// List lists all credential IDs.
	List(ctx context.Context) ([]string, error)

	// Exists checks if a credential exists.
	Exists(ctx context.Context, id string) (bool, error)
}

// InMemoryCredentialStore stores credentials in memory.
type InMemoryCredentialStore struct {
	mu          sync.RWMutex
	credentials map[string]Credential
}

// NewInMemoryCredentialStore creates a new in-memory credential store.
func NewInMemoryCredentialStore() *InMemoryCredentialStore {
	return &InMemoryCredentialStore{
		credentials: make(map[string]Credential),
	}
}

// Get retrieves a credential by ID.
func (s *InMemoryCredentialStore) Get(ctx context.Context, id string) (Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cred, exists := s.credentials[id]
	if !exists {
		return nil, ErrCredentialNotFound
	}

	if cred.IsExpired() {
		return nil, ErrCredentialExpired
	}

	return cred, nil
}

// Store stores a credential.
func (s *InMemoryCredentialStore) Store(ctx context.Context, cred Credential) error {
	if err := cred.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCredential, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.credentials[cred.ID()] = cred
	return nil
}

// Delete deletes a credential by ID.
func (s *InMemoryCredentialStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.credentials[id]; !exists {
		return ErrCredentialNotFound
	}

	delete(s.credentials, id)
	return nil
}

// List lists all credential IDs.
func (s *InMemoryCredentialStore) List(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.credentials))
	for id := range s.credentials {
		ids = append(ids, id)
	}
	return ids, nil
}

// Exists checks if a credential exists.
func (s *InMemoryCredentialStore) Exists(ctx context.Context, id string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.credentials[id]
	return exists, nil
}

// Ensure InMemoryCredentialStore implements CredentialStore.
var _ CredentialStore = (*InMemoryCredentialStore)(nil)

// FileCredentialStore stores credentials in encrypted files.
type FileCredentialStore struct {
	mu        sync.RWMutex
	basePath  string
	encryptor Encryptor
}

// FileStoreConfig configures the file credential store.
type FileStoreConfig struct {
	// BasePath is the directory for storing credentials.
	BasePath string
	// EncryptionKey is the key for encrypting credentials.
	EncryptionKey []byte
}

// NewFileCredentialStore creates a new file-based credential store.
func NewFileCredentialStore(config *FileStoreConfig) (*FileCredentialStore, error) {
	if config.BasePath == "" {
		return nil, fmt.Errorf("base path is required")
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(config.BasePath, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create credential directory: %w", err)
	}

	var encryptor Encryptor
	if len(config.EncryptionKey) > 0 {
		var err error
		encryptor, err = NewAESEncryptor(config.EncryptionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create encryptor: %w", err)
		}
	}

	return &FileCredentialStore{
		basePath:  config.BasePath,
		encryptor: encryptor,
	}, nil
}

// credentialPath returns the file path for a credential.
func (s *FileCredentialStore) credentialPath(id string) string {
	return filepath.Join(s.basePath, id+".cred")
}

// Get retrieves a credential by ID.
func (s *FileCredentialStore) Get(ctx context.Context, id string) (Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.credentialPath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrCredentialNotFound
		}
		return nil, fmt.Errorf("failed to read credential file: %w", err)
	}

	// Decrypt if encryptor is configured
	if s.encryptor != nil {
		data, err = s.encryptor.Decrypt(data)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrDecryptionFailed, err)
		}
	}

	// Parse envelope
	var envelope credentialEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse credential envelope: %w", err)
	}

	// Parse credential
	cred, err := ParseCredential(envelope.Type, envelope.Data)
	if err != nil {
		return nil, err
	}

	if cred.IsExpired() {
		return nil, ErrCredentialExpired
	}

	return cred, nil
}

// Store stores a credential.
func (s *FileCredentialStore) Store(ctx context.Context, cred Credential) error {
	if err := cred.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCredential, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Serialize credential
	credData, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("failed to serialize credential: %w", err)
	}

	// Create envelope
	envelope := credentialEnvelope{
		Type: cred.Type(),
		Data: credData,
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("failed to serialize envelope: %w", err)
	}

	// Encrypt if encryptor is configured
	if s.encryptor != nil {
		data, err = s.encryptor.Encrypt(data)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrEncryptionFailed, err)
		}
	}

	// Write to file
	path := s.credentialPath(cred.ID())
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write credential file: %w", err)
	}

	return nil
}

// Delete deletes a credential by ID.
func (s *FileCredentialStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.credentialPath(id)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrCredentialNotFound
		}
		return fmt.Errorf("failed to delete credential file: %w", err)
	}

	return nil
}

// List lists all credential IDs.
func (s *FileCredentialStore) List(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read credential directory: %w", err)
	}

	var ids []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) == ".cred" {
			ids = append(ids, name[:len(name)-5])
		}
	}

	return ids, nil
}

// Exists checks if a credential exists.
func (s *FileCredentialStore) Exists(ctx context.Context, id string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.credentialPath(id)
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Ensure FileCredentialStore implements CredentialStore.
var _ CredentialStore = (*FileCredentialStore)(nil)

// credentialEnvelope wraps a credential with its type for serialization.
type credentialEnvelope struct {
	Type CredentialType  `json:"type"`
	Data json.RawMessage `json:"data"`
}

// CompositeCredentialStore tries multiple stores in order.
type CompositeCredentialStore struct {
	stores []CredentialStore
}

// NewCompositeCredentialStore creates a new composite store.
func NewCompositeCredentialStore(stores ...CredentialStore) *CompositeCredentialStore {
	return &CompositeCredentialStore{stores: stores}
}

// Get retrieves a credential by ID, trying each store in order.
func (s *CompositeCredentialStore) Get(ctx context.Context, id string) (Credential, error) {
	for _, store := range s.stores {
		cred, err := store.Get(ctx, id)
		if err == nil {
			return cred, nil
		}
		if !errors.Is(err, ErrCredentialNotFound) {
			return nil, err
		}
	}
	return nil, ErrCredentialNotFound
}

// Store stores a credential in the first store.
func (s *CompositeCredentialStore) Store(ctx context.Context, cred Credential) error {
	if len(s.stores) == 0 {
		return fmt.Errorf("no stores configured")
	}
	return s.stores[0].Store(ctx, cred)
}

// Delete deletes a credential from all stores.
func (s *CompositeCredentialStore) Delete(ctx context.Context, id string) error {
	var lastErr error
	for _, store := range s.stores {
		if err := store.Delete(ctx, id); err != nil && !errors.Is(err, ErrCredentialNotFound) {
			lastErr = err
		}
	}
	return lastErr
}

// List lists all credential IDs from all stores.
func (s *CompositeCredentialStore) List(ctx context.Context) ([]string, error) {
	seen := make(map[string]bool)
	for _, store := range s.stores {
		ids, err := store.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			seen[id] = true
		}
	}

	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	return ids, nil
}

// Exists checks if a credential exists in any store.
func (s *CompositeCredentialStore) Exists(ctx context.Context, id string) (bool, error) {
	for _, store := range s.stores {
		exists, err := store.Exists(ctx, id)
		if err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
	}
	return false, nil
}

// Ensure CompositeCredentialStore implements CredentialStore.
var _ CredentialStore = (*CompositeCredentialStore)(nil)
