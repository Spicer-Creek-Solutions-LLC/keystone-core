// Package blueprint provides tests for snapshot functionality.
package blueprint

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestDefaultSnapshotConfig(t *testing.T) {
	config := DefaultSnapshotConfig()

	if config.StorePath != "/var/lib/keystone-core/snapshots" {
		t.Errorf("Expected StorePath /var/lib/keystone-core/snapshots, got %s", config.StorePath)
	}
	if config.MaxSnapshotsPerBlueprint != 10 {
		t.Errorf("Expected MaxSnapshotsPerBlueprint 10, got %d", config.MaxSnapshotsPerBlueprint)
	}
	if config.MaxTotalSnapshots != 100 {
		t.Errorf("Expected MaxTotalSnapshots 100, got %d", config.MaxTotalSnapshots)
	}
	if config.CompressSnapshots {
		t.Error("Expected CompressSnapshots to be false by default")
	}
}

func TestNewSnapshotManager(t *testing.T) {
	tmpDir := t.TempDir()

	config := &SnapshotConfig{
		StorePath:                tmpDir,
		MaxSnapshotsPerBlueprint: 5,
		MaxTotalSnapshots:        50,
	}

	manager, err := NewSnapshotManager(config)
	if err != nil {
		t.Fatalf("NewSnapshotManager failed: %v", err)
	}

	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}
}

func TestNewSnapshotManager_NilConfig(t *testing.T) {
	// This will fail because /var/lib/keystone-core/snapshots likely doesn't exist
	// and we can't create it without permissions
	_, err := NewSnapshotManager(nil)
	// Just verify it doesn't panic - error is expected if dir doesn't exist
	_ = err
}

func TestNewSnapshotManager_CreateDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	snapshotDir := filepath.Join(tmpDir, "nested", "snapshots")

	config := &SnapshotConfig{
		StorePath:                snapshotDir,
		MaxSnapshotsPerBlueprint: 5,
		MaxTotalSnapshots:        50,
	}

	manager, err := NewSnapshotManager(config)
	if err != nil {
		t.Fatalf("NewSnapshotManager failed: %v", err)
	}

	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}

	// Verify directory was created
	info, err := os.Stat(snapshotDir)
	if err != nil {
		t.Fatalf("Directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Expected a directory")
	}
}

func TestSnapshotManager_CreateSnapshot(t *testing.T) {
	tmpDir := t.TempDir()

	config := &SnapshotConfig{
		StorePath:                tmpDir,
		MaxSnapshotsPerBlueprint: 5,
		MaxTotalSnapshots:        50,
	}

	manager, err := NewSnapshotManager(config)
	if err != nil {
		t.Fatalf("NewSnapshotManager failed: %v", err)
	}

	capture := NewStateCapture()
	capture.AddFile(FileCaptureEntry{
		Path:   "/etc/nginx/nginx.conf",
		Exists: true,
		Mode:   0644,
	})
	capture.AddPackage(PackageCaptureEntry{
		Name:      "nginx",
		Installed: true,
		Version:   "1.18.0",
	})

	snapshot, err := manager.CreateSnapshot("agent-1", "web-stack", "1.0.0", "production", capture)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	if snapshot.ID == "" {
		t.Error("Expected non-empty snapshot ID")
	}
	if snapshot.AgentID != "agent-1" {
		t.Errorf("Expected AgentID 'agent-1', got '%s'", snapshot.AgentID)
	}
	if snapshot.BlueprintName != "web-stack" {
		t.Errorf("Expected BlueprintName 'web-stack', got '%s'", snapshot.BlueprintName)
	}
	if snapshot.BlueprintVersion != "1.0.0" {
		t.Errorf("Expected BlueprintVersion '1.0.0', got '%s'", snapshot.BlueprintVersion)
	}
	if snapshot.Namespace != "production" {
		t.Errorf("Expected Namespace 'production', got '%s'", snapshot.Namespace)
	}
	if snapshot.Checksum == "" {
		t.Error("Expected non-empty checksum")
	}
	if snapshot.Size <= 0 {
		t.Error("Expected positive size")
	}
	if len(snapshot.StateCapture.Files) != 1 {
		t.Errorf("Expected 1 file capture, got %d", len(snapshot.StateCapture.Files))
	}
	if len(snapshot.StateCapture.Packages) != 1 {
		t.Errorf("Expected 1 package capture, got %d", len(snapshot.StateCapture.Packages))
	}
}

func TestSnapshotManager_GetSnapshot(t *testing.T) {
	tmpDir := t.TempDir()

	config := &SnapshotConfig{
		StorePath:                tmpDir,
		MaxSnapshotsPerBlueprint: 5,
		MaxTotalSnapshots:        50,
	}

	manager, err := NewSnapshotManager(config)
	if err != nil {
		t.Fatalf("NewSnapshotManager failed: %v", err)
	}

	capture := NewStateCapture()
	created, err := manager.CreateSnapshot("agent-1", "test-bp", "1.0.0", "default", capture)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	retrieved, err := manager.GetSnapshot(created.ID)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}

	if retrieved.ID != created.ID {
		t.Errorf("Expected ID '%s', got '%s'", created.ID, retrieved.ID)
	}
	if retrieved.BlueprintName != created.BlueprintName {
		t.Errorf("Expected BlueprintName '%s', got '%s'", created.BlueprintName, retrieved.BlueprintName)
	}
}

func TestSnapshotManager_GetSnapshot_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	config := &SnapshotConfig{
		StorePath:                tmpDir,
		MaxSnapshotsPerBlueprint: 5,
		MaxTotalSnapshots:        50,
	}

	manager, err := NewSnapshotManager(config)
	if err != nil {
		t.Fatalf("NewSnapshotManager failed: %v", err)
	}

	_, err = manager.GetSnapshot("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent snapshot")
	}
}

func TestSnapshotManager_ListSnapshots(t *testing.T) {
	tmpDir := t.TempDir()

	config := &SnapshotConfig{
		StorePath:                tmpDir,
		MaxSnapshotsPerBlueprint: 10,
		MaxTotalSnapshots:        100,
	}

	manager, err := NewSnapshotManager(config)
	if err != nil {
		t.Fatalf("NewSnapshotManager failed: %v", err)
	}

	// Create multiple snapshots
	capture := NewStateCapture()
	for i := 0; i < 3; i++ {
		start := time.Now()
		if err := helpers.WaitForTimeout(2*time.Second, 1*time.Millisecond, func() (bool, error) {
			return time.Since(start) >= 10*time.Millisecond, nil
		}); err != nil {
			t.Fatalf("timestamp wait did not elapse: %v", err)
		}
		_, err := manager.CreateSnapshot("agent-1", "test-bp", "1.0.0", "default", capture)
		if err != nil {
			t.Fatalf("CreateSnapshot failed: %v", err)
		}
	}

	snapshots, err := manager.ListSnapshots("", "", "")
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}

	if len(snapshots) != 3 {
		t.Errorf("Expected 3 snapshots, got %d", len(snapshots))
	}

	// Verify sorted by time (newest first)
	for i := 1; i < len(snapshots); i++ {
		if snapshots[i-1].CreatedAt.Before(snapshots[i].CreatedAt) {
			t.Error("Snapshots should be sorted newest first")
		}
	}
}

func TestSnapshotManager_ListSnapshots_Filtered(t *testing.T) {
	tmpDir := t.TempDir()

	config := &SnapshotConfig{
		StorePath:                tmpDir,
		MaxSnapshotsPerBlueprint: 10,
		MaxTotalSnapshots:        100,
	}

	manager, err := NewSnapshotManager(config)
	if err != nil {
		t.Fatalf("NewSnapshotManager failed: %v", err)
	}

	capture := NewStateCapture()
	_, _ = manager.CreateSnapshot("agent-1", "bp-a", "1.0.0", "ns-1", capture)
	_, _ = manager.CreateSnapshot("agent-2", "bp-a", "1.0.0", "ns-1", capture)
	_, _ = manager.CreateSnapshot("agent-1", "bp-b", "1.0.0", "ns-2", capture)

	// Filter by agent
	snapshots, _ := manager.ListSnapshots("agent-1", "", "")
	if len(snapshots) != 2 {
		t.Errorf("Expected 2 snapshots for agent-1, got %d", len(snapshots))
	}

	// Filter by blueprint
	snapshots, _ = manager.ListSnapshots("", "bp-a", "")
	if len(snapshots) != 2 {
		t.Errorf("Expected 2 snapshots for bp-a, got %d", len(snapshots))
	}

	// Filter by namespace
	snapshots, _ = manager.ListSnapshots("", "", "ns-2")
	if len(snapshots) != 1 {
		t.Errorf("Expected 1 snapshot for ns-2, got %d", len(snapshots))
	}

	// Combined filter
	snapshots, _ = manager.ListSnapshots("agent-1", "bp-a", "ns-1")
	if len(snapshots) != 1 {
		t.Errorf("Expected 1 snapshot for combined filter, got %d", len(snapshots))
	}
}

func TestSnapshotManager_GetLatestSnapshot(t *testing.T) {
	tmpDir := t.TempDir()

	config := &SnapshotConfig{
		StorePath:                tmpDir,
		MaxSnapshotsPerBlueprint: 10,
		MaxTotalSnapshots:        100,
	}

	manager, err := NewSnapshotManager(config)
	if err != nil {
		t.Fatalf("NewSnapshotManager failed: %v", err)
	}

	capture := NewStateCapture()
	_, _ = manager.CreateSnapshot("agent-1", "test-bp", "1.0.0", "default", capture)
	start := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 1*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 10*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("timestamp wait did not elapse: %v", err)
	}
	latest, _ := manager.CreateSnapshot("agent-1", "test-bp", "1.1.0", "default", capture)

	retrieved, err := manager.GetLatestSnapshot("agent-1", "test-bp", "default")
	if err != nil {
		t.Fatalf("GetLatestSnapshot failed: %v", err)
	}

	if retrieved.ID != latest.ID {
		t.Error("Expected to retrieve the latest snapshot")
	}
}

func TestSnapshotManager_GetLatestSnapshot_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	config := &SnapshotConfig{
		StorePath:                tmpDir,
		MaxSnapshotsPerBlueprint: 10,
		MaxTotalSnapshots:        100,
	}

	manager, err := NewSnapshotManager(config)
	if err != nil {
		t.Fatalf("NewSnapshotManager failed: %v", err)
	}

	_, err = manager.GetLatestSnapshot("agent-1", "test-bp", "default")
	if err == nil {
		t.Error("Expected error for non-existent snapshot")
	}
}

func TestSnapshotManager_DeleteSnapshot(t *testing.T) {
	tmpDir := t.TempDir()

	config := &SnapshotConfig{
		StorePath:                tmpDir,
		MaxSnapshotsPerBlueprint: 10,
		MaxTotalSnapshots:        100,
	}

	manager, err := NewSnapshotManager(config)
	if err != nil {
		t.Fatalf("NewSnapshotManager failed: %v", err)
	}

	capture := NewStateCapture()
	snapshot, _ := manager.CreateSnapshot("agent-1", "test-bp", "1.0.0", "default", capture)

	err = manager.DeleteSnapshot(snapshot.ID)
	if err != nil {
		t.Fatalf("DeleteSnapshot failed: %v", err)
	}

	// Verify deleted
	_, err = manager.GetSnapshot(snapshot.ID)
	if err == nil {
		t.Error("Expected error for deleted snapshot")
	}
}

func TestSnapshotManager_DeleteSnapshot_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	config := &SnapshotConfig{
		StorePath:                tmpDir,
		MaxSnapshotsPerBlueprint: 10,
		MaxTotalSnapshots:        100,
	}

	manager, err := NewSnapshotManager(config)
	if err != nil {
		t.Fatalf("NewSnapshotManager failed: %v", err)
	}

	// Should not error for non-existent snapshot
	err = manager.DeleteSnapshot("non-existent")
	if err != nil {
		t.Errorf("DeleteSnapshot should not error for non-existent: %v", err)
	}
}

func TestSnapshotManager_CleanupOldSnapshots(t *testing.T) {
	tmpDir := t.TempDir()

	config := &SnapshotConfig{
		StorePath:                tmpDir,
		MaxSnapshotsPerBlueprint: 10,
		MaxTotalSnapshots:        100,
	}

	manager, err := NewSnapshotManager(config)
	if err != nil {
		t.Fatalf("NewSnapshotManager failed: %v", err)
	}

	capture := NewStateCapture()
	_, _ = manager.CreateSnapshot("agent-1", "test-bp", "1.0.0", "default", capture)

	// Cleanup with 0 duration should delete all
	deleted, err := manager.CleanupOldSnapshots(0)
	if err != nil {
		t.Fatalf("CleanupOldSnapshots failed: %v", err)
	}

	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}

	// Verify cleaned up
	snapshots, _ := manager.ListSnapshots("", "", "")
	if len(snapshots) != 0 {
		t.Errorf("Expected 0 snapshots after cleanup, got %d", len(snapshots))
	}
}

func TestNewStateCapture(t *testing.T) {
	capture := NewStateCapture()

	if capture == nil {
		t.Fatal("Expected non-nil StateCapture")
	}
	if capture.Custom == nil {
		t.Error("Expected initialized Custom map")
	}
}

func TestStateCapture_AddFile(t *testing.T) {
	capture := NewStateCapture()

	capture.AddFile(FileCaptureEntry{
		Path:   "/etc/test",
		Exists: true,
	})

	if len(capture.Files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(capture.Files))
	}
}

func TestStateCapture_AddPackage(t *testing.T) {
	capture := NewStateCapture()

	capture.AddPackage(PackageCaptureEntry{
		Name:      "test-pkg",
		Installed: true,
	})

	if len(capture.Packages) != 1 {
		t.Errorf("Expected 1 package, got %d", len(capture.Packages))
	}
}

func TestStateCapture_AddService(t *testing.T) {
	capture := NewStateCapture()

	capture.AddService(ServiceCaptureEntry{
		Name:    "test-svc",
		Running: true,
	})

	if len(capture.Services) != 1 {
		t.Errorf("Expected 1 service, got %d", len(capture.Services))
	}
}

func TestStateCapture_AddUser(t *testing.T) {
	capture := NewStateCapture()

	capture.AddUser(UserCaptureEntry{
		Name:   "testuser",
		Exists: true,
	})

	if len(capture.Users) != 1 {
		t.Errorf("Expected 1 user, got %d", len(capture.Users))
	}
}

func TestStateCapture_AddGroup(t *testing.T) {
	capture := NewStateCapture()

	capture.AddGroup(GroupCaptureEntry{
		Name:   "testgroup",
		Exists: true,
	})

	if len(capture.Groups) != 1 {
		t.Errorf("Expected 1 group, got %d", len(capture.Groups))
	}
}

func TestStateCapture_SetCustom(t *testing.T) {
	capture := NewStateCapture()

	capture.SetCustom("key1", "value1")
	capture.SetCustom("key2", 123)

	if capture.Custom["key1"] != "value1" {
		t.Error("Expected custom key1 = value1")
	}
	if capture.Custom["key2"] != 123 {
		t.Error("Expected custom key2 = 123")
	}
}

func TestGenerateSnapshotID(t *testing.T) {
	id1 := generateSnapshotID("agent-1", "blueprint", "namespace")
	id2 := generateSnapshotID("agent-1", "blueprint", "namespace")

	// IDs should be different (based on timestamp)
	if id1 == id2 {
		t.Error("Expected different IDs for different calls")
	}

	// IDs should be 16 characters (truncated SHA256)
	if len(id1) != 16 {
		t.Errorf("Expected ID length 16, got %d", len(id1))
	}
}

func TestSnapshotManager_EnforceLimits(t *testing.T) {
	tmpDir := t.TempDir()

	config := &SnapshotConfig{
		StorePath:                tmpDir,
		MaxSnapshotsPerBlueprint: 2, // Very low limit
		MaxTotalSnapshots:        5,
	}

	manager, err := NewSnapshotManager(config)
	if err != nil {
		t.Fatalf("NewSnapshotManager failed: %v", err)
	}

	capture := NewStateCapture()

	// Create more snapshots than the limit
	for i := 0; i < 4; i++ {
		start := time.Now()
		if err := helpers.WaitForTimeout(2*time.Second, 1*time.Millisecond, func() (bool, error) {
			return time.Since(start) >= 10*time.Millisecond, nil
		}); err != nil {
			t.Fatalf("timestamp wait did not elapse: %v", err)
		}
		_, err := manager.CreateSnapshot("agent-1", "test-bp", "1.0.0", "default", capture)
		if err != nil {
			t.Fatalf("CreateSnapshot failed: %v", err)
		}
	}

	// Should only have MaxSnapshotsPerBlueprint (2) remaining for this blueprint
	snapshots, _ := manager.ListSnapshots("", "test-bp", "")
	if len(snapshots) > 2 {
		t.Errorf("Expected at most 2 snapshots after limit enforcement, got %d", len(snapshots))
	}
}
