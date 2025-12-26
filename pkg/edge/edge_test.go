package edge

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewFileCache(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache")

	cache, err := NewFileCache(cachePath)
	if err != nil {
		t.Fatalf("NewFileCache failed: %v", err)
	}

	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	// Verify directory was created
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Error("cache directory was not created")
	}
}

func TestCache_SetAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewFileCache(tmpDir)
	if err != nil {
		t.Fatalf("NewFileCache failed: %v", err)
	}

	// Create a test entry
	entry := &CacheEntry{
		ID:        "test-1",
		Type:      "state",
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Size:      9,
	}

	// Set the entry
	if err := cache.Set(entry); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get the entry
	retrieved, err := cache.Get("test-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.ID != entry.ID {
		t.Errorf("expected ID %s, got %s", entry.ID, retrieved.ID)
	}

	if string(retrieved.Data) != string(entry.Data) {
		t.Errorf("expected data %s, got %s", entry.Data, retrieved.Data)
	}
}

func TestCache_Expiration(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewFileCache(tmpDir)
	if err != nil {
		t.Fatalf("NewFileCache failed: %v", err)
	}

	// Create an expired entry
	entry := &CacheEntry{
		ID:        "expired-1",
		Type:      "state",
		Data:      []byte("expired data"),
		CreatedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour),
		Size:      12,
	}

	// Set the entry
	if err := cache.Set(entry); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Try to get the expired entry
	_, err = cache.Get("expired-1")
	if err == nil {
		t.Error("expected error for expired entry")
	}
}

func TestCache_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewFileCache(tmpDir)
	if err != nil {
		t.Fatalf("NewFileCache failed: %v", err)
	}

	// Create and set an entry
	entry := &CacheEntry{
		ID:        "delete-1",
		Type:      "command",
		Data:      []byte("to be deleted"),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Size:      13,
	}

	if err := cache.Set(entry); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Delete the entry
	if err := cache.Delete("delete-1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Try to get the deleted entry
	_, err = cache.Get("delete-1")
	if err == nil {
		t.Error("expected error for deleted entry")
	}
}

func TestCache_List(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewFileCache(tmpDir)
	if err != nil {
		t.Fatalf("NewFileCache failed: %v", err)
	}

	// Create multiple entries
	entries := []*CacheEntry{
		{
			ID:        "list-1",
			Type:      "state",
			Data:      []byte("data 1"),
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(1 * time.Hour),
			Size:      6,
		},
		{
			ID:        "list-2",
			Type:      "command",
			Data:      []byte("data 2"),
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(1 * time.Hour),
			Size:      6,
		},
	}

	for _, entry := range entries {
		if err := cache.Set(entry); err != nil {
			t.Fatalf("Set failed: %v", err)
		}
	}

	// List all entries
	listed, err := cache.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(listed) != 2 {
		t.Errorf("expected 2 entries, got %d", len(listed))
	}
}

func TestCache_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewFileCache(tmpDir)
	if err != nil {
		t.Fatalf("NewFileCache failed: %v", err)
	}

	// Add some entries
	entry := &CacheEntry{
		ID:        "clear-1",
		Type:      "state",
		Data:      []byte("data"),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Size:      4,
	}

	if err := cache.Set(entry); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Clear the cache
	if err := cache.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	// Verify cache is empty
	listed, err := cache.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(listed) != 0 {
		t.Errorf("expected 0 entries after clear, got %d", len(listed))
	}
}

func TestCache_Prune(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewFileCache(tmpDir)
	if err != nil {
		t.Fatalf("NewFileCache failed: %v", err)
	}

	// Add expired and valid entries
	entries := []*CacheEntry{
		{
			ID:        "prune-expired",
			Type:      "state",
			Data:      []byte("expired"),
			CreatedAt: time.Now().Add(-2 * time.Hour),
			ExpiresAt: time.Now().Add(-1 * time.Hour),
			Size:      7,
		},
		{
			ID:        "prune-valid",
			Type:      "state",
			Data:      []byte("valid"),
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(1 * time.Hour),
			Size:      5,
		},
	}

	for _, entry := range entries {
		if err := cache.Set(entry); err != nil {
			t.Fatalf("Set failed: %v", err)
		}
	}

	// Prune expired entries
	if err := cache.Prune(); err != nil {
		t.Fatalf("Prune failed: %v", err)
	}

	// Verify only valid entry remains
	listed, err := cache.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(listed) != 1 {
		t.Errorf("expected 1 entry after prune, got %d", len(listed))
	}

	if listed[0].ID != "prune-valid" {
		t.Errorf("wrong entry remained after prune: %s", listed[0].ID)
	}
}

func TestCache_GetSize(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewFileCache(tmpDir)
	if err != nil {
		t.Fatalf("NewFileCache failed: %v", err)
	}

	// Add an entry
	entry := &CacheEntry{
		ID:        "size-1",
		Type:      "state",
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Size:      9,
	}

	if err := cache.Set(entry); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get size
	size, err := cache.GetSize()
	if err != nil {
		t.Fatalf("GetSize failed: %v", err)
	}

	if size == 0 {
		t.Error("expected non-zero cache size")
	}

	t.Logf("Cache size: %d bytes", size)
}

func TestCache_GetStats(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewFileCache(tmpDir)
	if err != nil {
		t.Fatalf("NewFileCache failed: %v", err)
	}

	// Add some entries
	entry := &CacheEntry{
		ID:        "stats-1",
		Type:      "state",
		Data:      []byte("data"),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Size:      4,
	}

	if err := cache.Set(entry); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get stats
	stats, err := cache.GetStats()
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalEntries != 1 {
		t.Errorf("expected 1 entry in stats, got %d", stats.TotalEntries)
	}

	t.Logf("Cache stats: %+v", stats)
}

func TestNewManager(t *testing.T) {
	tmpDir := t.TempDir()
	config := DefaultEdgeConfig()
	config.LocalCachePath = filepath.Join(tmpDir, "cache")

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestManager_Mode(t *testing.T) {
	tmpDir := t.TempDir()
	config := DefaultEdgeConfig()
	config.LocalCachePath = filepath.Join(tmpDir, "cache")

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Check initial mode
	mode := mgr.GetMode()
	if mode != ModeOnline {
		t.Errorf("expected initial mode to be online, got %s", mode)
	}

	// Set offline mode
	if err := mgr.SetMode(ModeOffline); err != nil {
		t.Fatalf("SetMode failed: %v", err)
	}

	// Verify mode changed
	mode = mgr.GetMode()
	if mode != ModeOffline {
		t.Errorf("expected mode to be offline, got %s", mode)
	}
}

func TestManager_Connection(t *testing.T) {
	tmpDir := t.TempDir()
	config := DefaultEdgeConfig()
	config.LocalCachePath = filepath.Join(tmpDir, "cache")

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Check initial connection status
	if !mgr.IsConnected() {
		t.Error("expected initial connection to be true")
	}

	// Disconnect
	mgr.SetConnected(false)
	if mgr.IsConnected() {
		t.Error("expected connection to be false after SetConnected(false)")
	}

	// Verify mode changed to offline
	mode := mgr.GetMode()
	if mode != ModeOffline {
		t.Errorf("expected mode to be offline after disconnect, got %s", mode)
	}

	// Reconnect
	mgr.SetConnected(true)
	if !mgr.IsConnected() {
		t.Error("expected connection to be true after SetConnected(true)")
	}

	// Verify mode changed back to online
	mode = mgr.GetMode()
	if mode != ModeOnline {
		t.Errorf("expected mode to be online after reconnect, got %s", mode)
	}
}

func TestManager_GetStatus(t *testing.T) {
	tmpDir := t.TempDir()
	config := DefaultEdgeConfig()
	config.LocalCachePath = filepath.Join(tmpDir, "cache")

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	status, err := mgr.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if status == nil {
		t.Fatal("expected non-nil status")
	}

	t.Logf("Edge Status:")
	t.Logf("  Mode: %s", status.Mode)
	t.Logf("  Connected: %v", status.Connected)
	t.Logf("  Memory Usage: %d MB", status.MemoryUsageMB)
	t.Logf("  Uptime: %d seconds", status.UptimeSeconds)
}

func TestManager_CheckResourceConstraints(t *testing.T) {
	tmpDir := t.TempDir()
	config := DefaultEdgeConfig()
	config.LocalCachePath = filepath.Join(tmpDir, "cache")
	config.EnableLightweightMode = true
	config.MaxMemoryMB = 1 // Very low limit to trigger constraint

	mgr, err := NewManager(config)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	constrained, err := mgr.CheckResourceConstraints()
	if err != nil {
		t.Fatalf("CheckResourceConstraints failed: %v", err)
	}

	t.Logf("Resource constrained: %v", constrained)
}

func TestOperationMode_String(t *testing.T) {
	modes := []struct {
		mode OperationMode
		str  string
	}{
		{ModeOnline, "online"},
		{ModeOffline, "offline"},
		{ModeLightweight, "lightweight"},
	}

	for _, m := range modes {
		if m.mode.String() != m.str {
			t.Errorf("expected %s, got %s", m.str, m.mode.String())
		}
	}
}

func TestDefaultEdgeConfig(t *testing.T) {
	config := DefaultEdgeConfig()

	if config == nil {
		t.Fatal("expected non-nil config")
	}

	if !config.EnableOfflineMode {
		t.Error("expected offline mode to be enabled by default")
	}

	if config.HeartbeatInterval == 0 {
		t.Error("expected non-zero heartbeat interval")
	}

	t.Logf("Default Config:")
	t.Logf("  Offline Mode: %v", config.EnableOfflineMode)
	t.Logf("  Lightweight Mode: %v", config.EnableLightweightMode)
	t.Logf("  Max Cache Size: %d MB", config.MaxCacheSize/(1024*1024))
	t.Logf("  Heartbeat Interval: %s", config.HeartbeatInterval)
}
