// Package capabilities provides capability lock management for module security
package capabilities

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CapabilityLock represents a locked set of capabilities for a module
type CapabilityLock struct {
	// ModuleName is the name of the locked module
	ModuleName string `json:"module_name" yaml:"module_name"`

	// Version is the module version when locked
	Version string `json:"version" yaml:"version"`

	// Capabilities is the set of locked capabilities
	Capabilities []string `json:"capabilities" yaml:"capabilities"`

	// CapabilityConfigs stores the locked configuration for each capability
	CapabilityConfigs map[string]*CapabilityPolicyConfig `json:"capability_configs,omitempty" yaml:"capability_configs,omitempty"`

	// LockedAt is when the lock was created
	LockedAt time.Time `json:"locked_at" yaml:"locked_at"`

	// LockedBy is who created the lock (user, system, etc.)
	LockedBy string `json:"locked_by" yaml:"locked_by"`

	// Reason explains why the lock was created
	Reason string `json:"reason,omitempty" yaml:"reason,omitempty"`

	// Hash is the content hash of the module when locked (for integrity)
	Hash string `json:"hash,omitempty" yaml:"hash,omitempty"`
}

// HasCapability checks if a capability is in the locked set
func (cl *CapabilityLock) HasCapability(capName string) bool {
	for _, cap := range cl.Capabilities {
		if cap == capName {
			return true
		}
	}
	return false
}

// GetCapabilityConfig returns the locked config for a capability
func (cl *CapabilityLock) GetCapabilityConfig(capName string) *CapabilityPolicyConfig {
	if cl.CapabilityConfigs == nil {
		return nil
	}
	return cl.CapabilityConfigs[capName]
}

// AddCapability adds a capability to the lock (for lock creation)
func (cl *CapabilityLock) AddCapability(capName string, config *CapabilityPolicyConfig) {
	// Check if already present
	for _, cap := range cl.Capabilities {
		if cap == capName {
			// Update config
			if cl.CapabilityConfigs == nil {
				cl.CapabilityConfigs = make(map[string]*CapabilityPolicyConfig)
			}
			if config != nil {
				cl.CapabilityConfigs[capName] = config
			}
			return
		}
	}
	cl.Capabilities = append(cl.Capabilities, capName)
	if config != nil {
		if cl.CapabilityConfigs == nil {
			cl.CapabilityConfigs = make(map[string]*CapabilityPolicyConfig)
		}
		cl.CapabilityConfigs[capName] = config
	}
}

// LockStore provides storage for capability locks
type LockStore interface {
	// GetLock retrieves a lock for a module
	GetLock(moduleName string) (*CapabilityLock, error)

	// SetLock stores a lock for a module
	SetLock(lock *CapabilityLock) error

	// DeleteLock removes a lock for a module
	DeleteLock(moduleName string) error

	// ListLocks returns all locks
	ListLocks() ([]*CapabilityLock, error)

	// HasLock checks if a module has a lock
	HasLock(moduleName string) bool
}

// InMemoryLockStore implements LockStore in memory (for testing)
type InMemoryLockStore struct {
	locks map[string]*CapabilityLock
	mu    sync.RWMutex
}

// NewInMemoryLockStore creates a new in-memory lock store
func NewInMemoryLockStore() *InMemoryLockStore {
	return &InMemoryLockStore{
		locks: make(map[string]*CapabilityLock),
	}
}

// GetLock retrieves a lock for a module
func (s *InMemoryLockStore) GetLock(moduleName string) (*CapabilityLock, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	lock, ok := s.locks[moduleName]
	if !ok {
		return nil, nil // No lock found (not an error)
	}
	return lock, nil
}

// SetLock stores a lock for a module
func (s *InMemoryLockStore) SetLock(lock *CapabilityLock) error {
	if lock == nil {
		return fmt.Errorf("lock cannot be nil")
	}
	if lock.ModuleName == "" {
		return fmt.Errorf("lock module name cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.locks[lock.ModuleName] = lock
	return nil
}

// DeleteLock removes a lock for a module
func (s *InMemoryLockStore) DeleteLock(moduleName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.locks, moduleName)
	return nil
}

// ListLocks returns all locks
func (s *InMemoryLockStore) ListLocks() ([]*CapabilityLock, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	locks := make([]*CapabilityLock, 0, len(s.locks))
	for _, lock := range s.locks {
		locks = append(locks, lock)
	}
	return locks, nil
}

// HasLock checks if a module has a lock
func (s *InMemoryLockStore) HasLock(moduleName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.locks[moduleName]
	return ok
}

// FileLockStore implements LockStore using a JSON file
type FileLockStore struct {
	path string
	mu   sync.RWMutex
}

// NewFileLockStore creates a new file-based lock store
func NewFileLockStore(path string) *FileLockStore {
	return &FileLockStore{
		path: path,
	}
}

// loadLocks loads all locks from the file
func (s *FileLockStore) loadLocks() (map[string]*CapabilityLock, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]*CapabilityLock), nil
		}
		return nil, fmt.Errorf("failed to read lock file: %w", err)
	}

	var locks map[string]*CapabilityLock
	if err := json.Unmarshal(data, &locks); err != nil {
		return nil, fmt.Errorf("failed to parse lock file: %w", err)
	}

	if locks == nil {
		locks = make(map[string]*CapabilityLock)
	}

	return locks, nil
}

// saveLocks saves all locks to the file
func (s *FileLockStore) saveLocks(locks map[string]*CapabilityLock) error {
	data, err := json.MarshalIndent(locks, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal locks: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create lock directory: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0644); err != nil {
		return fmt.Errorf("failed to write lock file: %w", err)
	}

	return nil
}

// GetLock retrieves a lock for a module
func (s *FileLockStore) GetLock(moduleName string) (*CapabilityLock, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	locks, err := s.loadLocks()
	if err != nil {
		return nil, err
	}

	lock, ok := locks[moduleName]
	if !ok {
		return nil, nil
	}
	return lock, nil
}

// SetLock stores a lock for a module
func (s *FileLockStore) SetLock(lock *CapabilityLock) error {
	if lock == nil {
		return fmt.Errorf("lock cannot be nil")
	}
	if lock.ModuleName == "" {
		return fmt.Errorf("lock module name cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	locks, err := s.loadLocks()
	if err != nil {
		return err
	}

	locks[lock.ModuleName] = lock

	return s.saveLocks(locks)
}

// DeleteLock removes a lock for a module
func (s *FileLockStore) DeleteLock(moduleName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	locks, err := s.loadLocks()
	if err != nil {
		return err
	}

	delete(locks, moduleName)

	return s.saveLocks(locks)
}

// ListLocks returns all locks
func (s *FileLockStore) ListLocks() ([]*CapabilityLock, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	locks, err := s.loadLocks()
	if err != nil {
		return nil, err
	}

	result := make([]*CapabilityLock, 0, len(locks))
	for _, lock := range locks {
		result = append(result, lock)
	}
	return result, nil
}

// HasLock checks if a module has a lock
func (s *FileLockStore) HasLock(moduleName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	locks, err := s.loadLocks()
	if err != nil {
		return false
	}

	_, ok := locks[moduleName]
	return ok
}

// LockManager provides high-level lock management
type LockManager struct {
	store     LockStore
	evaluator *PolicyEvaluator
}

// NewLockManager creates a new lock manager
func NewLockManager(store LockStore, evaluator *PolicyEvaluator) *LockManager {
	return &LockManager{
		store:     store,
		evaluator: evaluator,
	}
}

// LockModule creates a lock for a module based on its current capabilities
func (lm *LockManager) LockModule(moduleName, version string, capabilities []string, configs map[string]*CapabilityPolicyConfig, lockedBy, reason string) (*CapabilityLock, error) {
	// Check if already locked
	existing, err := lm.store.GetLock(moduleName)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing lock: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("module %q is already locked (locked at %v by %s)", moduleName, existing.LockedAt, existing.LockedBy)
	}

	lock := &CapabilityLock{
		ModuleName:        moduleName,
		Version:           version,
		Capabilities:      capabilities,
		CapabilityConfigs: configs,
		LockedAt:          time.Now(),
		LockedBy:          lockedBy,
		Reason:            reason,
	}

	if err := lm.store.SetLock(lock); err != nil {
		return nil, fmt.Errorf("failed to store lock: %w", err)
	}

	return lock, nil
}

// UnlockModule removes a lock for a module
func (lm *LockManager) UnlockModule(moduleName, unlockedBy, reason string) error {
	lock, err := lm.store.GetLock(moduleName)
	if err != nil {
		return fmt.Errorf("failed to get lock: %w", err)
	}
	if lock == nil {
		return fmt.Errorf("module %q is not locked", moduleName)
	}

	if err := lm.store.DeleteLock(moduleName); err != nil {
		return fmt.Errorf("failed to delete lock: %w", err)
	}

	return nil
}

// CheckUpdate checks if a module update is allowed based on locks
func (lm *LockManager) CheckUpdate(moduleName string, newCapabilities []string, newConfigs map[string]*CapabilityPolicyConfig) (*UpdateCheckResult, error) {
	lock, err := lm.store.GetLock(moduleName)
	if err != nil {
		return nil, fmt.Errorf("failed to get lock: %w", err)
	}

	if lock == nil {
		// No lock, update allowed
		return &UpdateCheckResult{
			Allowed: true,
			Reason:  "module is not locked",
		}, nil
	}

	result := &UpdateCheckResult{
		Allowed:     true,
		AddedCaps:   []string{},
		RemovedCaps: []string{},
		BlockedCaps: []string{},
	}

	// Build set of locked capabilities
	lockedSet := make(map[string]bool)
	for _, cap := range lock.Capabilities {
		lockedSet[cap] = true
	}

	// Check for new capabilities not in lock
	for _, cap := range newCapabilities {
		if !lockedSet[cap] {
			result.AddedCaps = append(result.AddedCaps, cap)
			result.BlockedCaps = append(result.BlockedCaps, cap)
			result.Allowed = false
		}
	}

	// Check for removed capabilities
	newSet := make(map[string]bool)
	for _, cap := range newCapabilities {
		newSet[cap] = true
	}
	for _, cap := range lock.Capabilities {
		if !newSet[cap] {
			result.RemovedCaps = append(result.RemovedCaps, cap)
		}
	}

	if !result.Allowed {
		result.Reason = fmt.Sprintf("module is locked: new capabilities [%v] not allowed", result.BlockedCaps)
	} else if len(result.RemovedCaps) > 0 {
		result.Reason = fmt.Sprintf("module is locked: capabilities [%v] were removed (allowed)", result.RemovedCaps)
	} else {
		result.Reason = "module is locked: no capability changes"
	}

	return result, nil
}

// GetLock retrieves the lock for a module
func (lm *LockManager) GetLock(moduleName string) (*CapabilityLock, error) {
	return lm.store.GetLock(moduleName)
}

// ListLocks returns all module locks
func (lm *LockManager) ListLocks() ([]*CapabilityLock, error) {
	return lm.store.ListLocks()
}

// IsLocked checks if a module is locked
func (lm *LockManager) IsLocked(moduleName string) bool {
	return lm.store.HasLock(moduleName)
}

// CreateLockFromManifest creates a lock from module manifest capabilities
func CreateLockFromManifest(moduleName, version string, capabilities []string, configs map[string]*CapabilityPolicyConfig, lockedBy string) *CapabilityLock {
	return &CapabilityLock{
		ModuleName:        moduleName,
		Version:           version,
		Capabilities:      capabilities,
		CapabilityConfigs: configs,
		LockedAt:          time.Now(),
		LockedBy:          lockedBy,
		Reason:            "locked from manifest",
	}
}
