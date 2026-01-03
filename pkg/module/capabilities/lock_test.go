package capabilities

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCapabilityLock_HasCapability(t *testing.T) {
	lock := &CapabilityLock{
		ModuleName:   "test-module",
		Capabilities: []string{"fs.read", "log", "http.get"},
	}

	tests := []struct {
		capName string
		want    bool
	}{
		{"fs.read", true},
		{"log", true},
		{"http.get", true},
		{"exec", false},
		{"fs.write", false},
	}

	for _, tt := range tests {
		t.Run(tt.capName, func(t *testing.T) {
			if got := lock.HasCapability(tt.capName); got != tt.want {
				t.Errorf("HasCapability(%q) = %v, want %v", tt.capName, got, tt.want)
			}
		})
	}
}

func TestCapabilityLock_AddCapability(t *testing.T) {
	lock := &CapabilityLock{
		ModuleName:   "test-module",
		Capabilities: []string{},
	}

	// Add capability without config
	lock.AddCapability("fs.read", nil)
	if !lock.HasCapability("fs.read") {
		t.Error("fs.read should be added")
	}

	// Add capability with config
	config := &CapabilityPolicyConfig{
		AllowedPaths: []string{"/tmp/**"},
	}
	lock.AddCapability("fs.write", config)
	if !lock.HasCapability("fs.write") {
		t.Error("fs.write should be added")
	}
	if lock.GetCapabilityConfig("fs.write") == nil {
		t.Error("fs.write config should be stored")
	}

	// Adding same capability again updates config
	newConfig := &CapabilityPolicyConfig{
		AllowedPaths: []string{"/var/**"},
	}
	lock.AddCapability("fs.write", newConfig)
	if len(lock.Capabilities) != 2 {
		t.Errorf("Capabilities count = %d, want 2 (no duplicates)", len(lock.Capabilities))
	}
	if lock.GetCapabilityConfig("fs.write").AllowedPaths[0] != "/var/**" {
		t.Error("Config should be updated")
	}
}

func TestInMemoryLockStore(t *testing.T) {
	store := NewInMemoryLockStore()

	// Test GetLock on empty store
	lock, err := store.GetLock("nonexistent")
	if err != nil {
		t.Fatalf("GetLock error: %v", err)
	}
	if lock != nil {
		t.Error("Lock should be nil for nonexistent module")
	}

	// Test SetLock
	testLock := &CapabilityLock{
		ModuleName:   "test-module",
		Version:      "1.0.0",
		Capabilities: []string{"fs.read"},
		LockedAt:     time.Now(),
		LockedBy:     "admin",
	}
	if err := store.SetLock(testLock); err != nil {
		t.Fatalf("SetLock error: %v", err)
	}

	// Test GetLock after set
	lock, err = store.GetLock("test-module")
	if err != nil {
		t.Fatalf("GetLock error: %v", err)
	}
	if lock == nil {
		t.Fatal("Lock should not be nil")
	}
	if lock.Version != "1.0.0" {
		t.Errorf("Version = %s, want 1.0.0", lock.Version)
	}

	// Test HasLock
	if !store.HasLock("test-module") {
		t.Error("HasLock should return true")
	}
	if store.HasLock("nonexistent") {
		t.Error("HasLock should return false for nonexistent")
	}

	// Test ListLocks
	locks, err := store.ListLocks()
	if err != nil {
		t.Fatalf("ListLocks error: %v", err)
	}
	if len(locks) != 1 {
		t.Errorf("ListLocks count = %d, want 1", len(locks))
	}

	// Test DeleteLock
	if err := store.DeleteLock("test-module"); err != nil {
		t.Fatalf("DeleteLock error: %v", err)
	}
	if store.HasLock("test-module") {
		t.Error("Lock should be deleted")
	}
}

func TestInMemoryLockStore_Validation(t *testing.T) {
	store := NewInMemoryLockStore()

	// Test nil lock
	err := store.SetLock(nil)
	if err == nil {
		t.Error("SetLock(nil) should return error")
	}

	// Test empty module name
	err = store.SetLock(&CapabilityLock{})
	if err == nil {
		t.Error("SetLock with empty module name should return error")
	}
}

func TestFileLockStore(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "locks.json")

	store := NewFileLockStore(lockPath)

	// Test GetLock on non-existent file
	lock, err := store.GetLock("test-module")
	if err != nil {
		t.Fatalf("GetLock error: %v", err)
	}
	if lock != nil {
		t.Error("Lock should be nil for non-existent file")
	}

	// Test SetLock creates file
	testLock := &CapabilityLock{
		ModuleName:   "test-module",
		Version:      "1.0.0",
		Capabilities: []string{"fs.read", "log"},
		LockedAt:     time.Now(),
		LockedBy:     "admin",
		Reason:       "initial lock",
	}
	if err := store.SetLock(testLock); err != nil {
		t.Fatalf("SetLock error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("Lock file should be created")
	}

	// Test GetLock after set
	lock, err = store.GetLock("test-module")
	if err != nil {
		t.Fatalf("GetLock error: %v", err)
	}
	if lock == nil {
		t.Fatal("Lock should not be nil")
	}
	if lock.Version != "1.0.0" {
		t.Errorf("Version = %s, want 1.0.0", lock.Version)
	}
	if len(lock.Capabilities) != 2 {
		t.Errorf("Capabilities count = %d, want 2", len(lock.Capabilities))
	}

	// Test HasLock
	if !store.HasLock("test-module") {
		t.Error("HasLock should return true")
	}

	// Add another lock
	anotherLock := &CapabilityLock{
		ModuleName:   "another-module",
		Capabilities: []string{"exec"},
	}
	if err := store.SetLock(anotherLock); err != nil {
		t.Fatalf("SetLock error: %v", err)
	}

	// Test ListLocks
	locks, err := store.ListLocks()
	if err != nil {
		t.Fatalf("ListLocks error: %v", err)
	}
	if len(locks) != 2 {
		t.Errorf("ListLocks count = %d, want 2", len(locks))
	}

	// Test DeleteLock
	if err := store.DeleteLock("test-module"); err != nil {
		t.Fatalf("DeleteLock error: %v", err)
	}
	locks, _ = store.ListLocks()
	if len(locks) != 1 {
		t.Errorf("ListLocks count after delete = %d, want 1", len(locks))
	}
}

func TestFileLockStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "locks.json")

	// Create and save with first store instance
	store1 := NewFileLockStore(lockPath)
	testLock := &CapabilityLock{
		ModuleName:   "persistent-module",
		Version:      "2.0.0",
		Capabilities: []string{"kv", "log"},
		LockedBy:     "system",
	}
	if err := store1.SetLock(testLock); err != nil {
		t.Fatalf("SetLock error: %v", err)
	}

	// Create new store instance and verify persistence
	store2 := NewFileLockStore(lockPath)
	lock, err := store2.GetLock("persistent-module")
	if err != nil {
		t.Fatalf("GetLock error: %v", err)
	}
	if lock == nil {
		t.Fatal("Lock should persist")
	}
	if lock.Version != "2.0.0" {
		t.Errorf("Version = %s, want 2.0.0", lock.Version)
	}
	if lock.LockedBy != "system" {
		t.Errorf("LockedBy = %s, want system", lock.LockedBy)
	}
}

func TestLockManager(t *testing.T) {
	lockStore := NewInMemoryLockStore()
	policy := &CapabilityPolicy{SchemaVersion: 1}
	eval := NewPolicyEvaluator(policy, lockStore)
	manager := NewLockManager(lockStore, eval)

	// Test LockModule
	lock, err := manager.LockModule(
		"test-module",
		"1.0.0",
		[]string{"fs.read", "log"},
		nil,
		"admin",
		"security requirement",
	)
	if err != nil {
		t.Fatalf("LockModule error: %v", err)
	}
	if lock.ModuleName != "test-module" {
		t.Errorf("ModuleName = %s, want test-module", lock.ModuleName)
	}

	// Test duplicate lock fails
	_, err = manager.LockModule("test-module", "1.0.1", []string{"fs.read"}, nil, "admin", "")
	if err == nil {
		t.Error("Duplicate lock should fail")
	}

	// Test IsLocked
	if !manager.IsLocked("test-module") {
		t.Error("Module should be locked")
	}
	if manager.IsLocked("unlocked-module") {
		t.Error("Unlocked module should not show as locked")
	}

	// Test GetLock
	lock, err = manager.GetLock("test-module")
	if err != nil {
		t.Fatalf("GetLock error: %v", err)
	}
	if lock == nil {
		t.Error("Lock should exist")
	}

	// Test ListLocks
	locks, err := manager.ListLocks()
	if err != nil {
		t.Fatalf("ListLocks error: %v", err)
	}
	if len(locks) != 1 {
		t.Errorf("ListLocks count = %d, want 1", len(locks))
	}

	// Test UnlockModule
	if err := manager.UnlockModule("test-module", "admin", "no longer needed"); err != nil {
		t.Fatalf("UnlockModule error: %v", err)
	}
	if manager.IsLocked("test-module") {
		t.Error("Module should be unlocked")
	}

	// Test unlock non-existent fails
	err = manager.UnlockModule("nonexistent", "admin", "")
	if err == nil {
		t.Error("Unlocking nonexistent module should fail")
	}
}

func TestLockManager_CheckUpdate(t *testing.T) {
	lockStore := NewInMemoryLockStore()
	manager := NewLockManager(lockStore, nil)

	// Lock with specific capabilities
	_, err := manager.LockModule(
		"locked-module",
		"1.0.0",
		[]string{"fs.read", "log"},
		nil,
		"admin",
		"",
	)
	if err != nil {
		t.Fatalf("LockModule error: %v", err)
	}

	tests := []struct {
		name           string
		newCaps        []string
		wantAllowed    bool
		wantBlockedLen int
	}{
		{
			name:        "same capabilities allowed",
			newCaps:     []string{"fs.read", "log"},
			wantAllowed: true,
		},
		{
			name:        "subset allowed",
			newCaps:     []string{"fs.read"},
			wantAllowed: true,
		},
		{
			name:           "adding capability blocked",
			newCaps:        []string{"fs.read", "log", "exec"},
			wantAllowed:    false,
			wantBlockedLen: 1,
		},
		{
			name:           "adding multiple capabilities blocked",
			newCaps:        []string{"fs.read", "exec", "secrets.write"},
			wantAllowed:    false,
			wantBlockedLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := manager.CheckUpdate("locked-module", tt.newCaps, nil)
			if err != nil {
				t.Fatalf("CheckUpdate error: %v", err)
			}

			if result.Allowed != tt.wantAllowed {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.wantAllowed)
			}
			if len(result.BlockedCaps) != tt.wantBlockedLen {
				t.Errorf("BlockedCaps count = %d, want %d", len(result.BlockedCaps), tt.wantBlockedLen)
			}
		})
	}

	// Test unlocked module
	result, err := manager.CheckUpdate("unlocked-module", []string{"exec"}, nil)
	if err != nil {
		t.Fatalf("CheckUpdate error: %v", err)
	}
	if !result.Allowed {
		t.Error("Unlocked module update should be allowed")
	}
}

func TestCreateLockFromManifest(t *testing.T) {
	caps := []string{"fs.read", "log", "kv"}
	configs := map[string]*CapabilityPolicyConfig{
		"fs.read": {AllowedPaths: []string{"/app/**"}},
	}

	lock := CreateLockFromManifest("my-module", "1.2.3", caps, configs, "deployer")

	if lock.ModuleName != "my-module" {
		t.Errorf("ModuleName = %s, want my-module", lock.ModuleName)
	}
	if lock.Version != "1.2.3" {
		t.Errorf("Version = %s, want 1.2.3", lock.Version)
	}
	if len(lock.Capabilities) != 3 {
		t.Errorf("Capabilities count = %d, want 3", len(lock.Capabilities))
	}
	if lock.LockedBy != "deployer" {
		t.Errorf("LockedBy = %s, want deployer", lock.LockedBy)
	}
	if lock.Reason != "locked from manifest" {
		t.Errorf("Reason = %s, want 'locked from manifest'", lock.Reason)
	}
	if lock.LockedAt.IsZero() {
		t.Error("LockedAt should be set")
	}
	if lock.GetCapabilityConfig("fs.read") == nil {
		t.Error("Config for fs.read should be stored")
	}
}
