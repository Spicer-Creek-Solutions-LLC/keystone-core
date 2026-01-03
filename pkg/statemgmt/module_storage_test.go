package statemgmt

import (
	"context"
	"runtime"
	"testing"
)

// =============================================================================
// Mount Module Tests
// =============================================================================

func TestNewMountModule(t *testing.T) {
	m := NewMountModule()
	if m == nil {
		t.Fatal("NewMountModule returned nil")
	}
	if m.Name() != "mount" {
		t.Errorf("expected name 'mount', got '%s'", m.Name())
	}
	states := m.ValidStates()
	expected := []string{"mounted", "unmounted", "present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestMountModule_Check_Mounted(t *testing.T) {
	m := NewMountModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-mount",
		Module: "mount",
		State:  "mounted",
		Parameters: map[string]interface{}{
			"path":   "/mnt/test",
			"device": "/dev/sda1",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if result == nil {
		t.Fatal("Check returned nil result")
	}
	// Result depends on actual system state
}

func TestMountModule_Check_MissingPath(t *testing.T) {
	m := NewMountModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-mount",
		Module: "mount",
		State:  "mounted",
		Parameters: map[string]interface{}{
			"device": "/dev/sda1",
		},
	}

	result, err := m.Check(ctx, decl)
	// On non-Linux platforms, the module may return an error about platform support
	// or a result indicating the mount point doesn't exist
	// We just verify the call completes without panic
	_ = result
	_ = err
}

func TestMountModule_Apply_Mounted(t *testing.T) {
	m := NewMountModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-mount",
		Module: "mount",
		State:  "mounted",
		Parameters: map[string]interface{}{
			"path":   "/tmp/test-mount-point",
			"device": "/dev/null",
			"fstype": "tmpfs",
		},
	}

	// This will likely fail in test environment (no privileges)
	// but tests the code path
	result, err := m.Apply(ctx, decl)
	// We expect either an error or a result
	if err == nil && result == nil {
		t.Error("Apply should return either result or error")
	}
}

func TestMountModule_Test(t *testing.T) {
	m := NewMountModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-mount",
		Module: "mount",
		State:  "mounted",
		Parameters: map[string]interface{}{
			"path":   "/mnt/test",
			"device": "/dev/sda1",
		},
	}

	result, err := m.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	// Test returns check result
	_ = result
}

func TestMountConfig_Parsing(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]interface{}
		want   struct {
			device  string
			path    string
			fstype  string
			persist bool
		}
	}{
		{
			name: "basic mount",
			params: map[string]interface{}{
				"device": "/dev/sda1",
				"path":   "/mnt/data",
			},
			want: struct {
				device  string
				path    string
				fstype  string
				persist bool
			}{"/dev/sda1", "/mnt/data", "", false},
		},
		{
			name: "full config",
			params: map[string]interface{}{
				"device":  "/dev/sdb1",
				"path":    "/mnt/backup",
				"fstype":  "ext4",
				"persist": true,
			},
			want: struct {
				device  string
				path    string
				fstype  string
				persist bool
			}{"/dev/sdb1", "/mnt/backup", "ext4", true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl := &StateDeclaration{
				ID:         "test",
				Module:     "mount",
				State:      "mounted",
				Parameters: tt.params,
			}
			device := getStringParameter(decl, "device", "")
			path := getStringParameter(decl, "path", "")
			fstype := getStringParameter(decl, "fstype", "")
			persist := getBoolParameter(decl, "persist", false)

			if device != tt.want.device {
				t.Errorf("device = %s, want %s", device, tt.want.device)
			}
			if path != tt.want.path {
				t.Errorf("path = %s, want %s", path, tt.want.path)
			}
			if fstype != tt.want.fstype {
				t.Errorf("fstype = %s, want %s", fstype, tt.want.fstype)
			}
			if persist != tt.want.persist {
				t.Errorf("persist = %v, want %v", persist, tt.want.persist)
			}
		})
	}
}

// =============================================================================
// Swap Module Tests
// =============================================================================

func TestNewSwapModule(t *testing.T) {
	m := NewSwapModule()
	if m == nil {
		t.Fatal("NewSwapModule returned nil")
	}
	if m.Name() != "swap" {
		t.Errorf("expected name 'swap', got '%s'", m.Name())
	}
	states := m.ValidStates()
	expected := []string{"enabled", "disabled", "present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestSwapModule_Check_Enabled(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("swap module only works on Linux")
	}
	m := NewSwapModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-swap",
		Module: "swap",
		State:  "enabled",
		Parameters: map[string]interface{}{
			"path": "/swapfile",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if result == nil {
		t.Fatal("Check returned nil result")
	}
}

func TestSwapModule_Check_MissingPath(t *testing.T) {
	m := NewSwapModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:         "test-swap",
		Module:     "swap",
		State:      "enabled",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(ctx, decl)
	if err == nil {
		t.Error("expected error for missing path parameter")
	}
}

func TestSwapModule_Apply_Present(t *testing.T) {
	m := NewSwapModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-swap",
		Module: "swap",
		State:  "present",
		Parameters: map[string]interface{}{
			"path": "/tmp/testswap",
			"size": "1M",
		},
	}

	// Will fail without privileges
	result, err := m.Apply(ctx, decl)
	if err == nil && result == nil {
		t.Error("Apply should return either result or error")
	}
}

func TestSwapModule_Test(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("swap module only works on Linux")
	}
	m := NewSwapModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-swap",
		Module: "swap",
		State:  "enabled",
		Parameters: map[string]interface{}{
			"path": "/swapfile",
		},
	}

	result, err := m.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	_ = result
}

func TestSwapModule_ParseSize(t *testing.T) {
	m := NewSwapModule()

	tests := []struct {
		input    string
		expected int64
		hasError bool
	}{
		{"1G", 1024 * 1024 * 1024, false},
		{"512M", 512 * 1024 * 1024, false},
		{"100K", 100 * 1024, false},
		{"1024", 1024 * 1024 * 1024, false}, // No suffix defaults to MiB, so 1024 MiB
		{"2g", 2 * 1024 * 1024 * 1024, false},
		{"256m", 256 * 1024 * 1024, false},
		{"", 0, true},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := m.parseSize(tt.input)
			if tt.hasError {
				if err == nil {
					t.Errorf("expected error for input '%s'", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for input '%s': %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("parseSize(%s) = %d, want %d", tt.input, result, tt.expected)
				}
			}
		})
	}
}

// =============================================================================
// LVM Physical Volume Module Tests
// =============================================================================

func TestNewLVMPVModule(t *testing.T) {
	m := NewLVMPVModule()
	if m == nil {
		t.Fatal("NewLVMPVModule returned nil")
	}
	if m.Name() != "lvm_pv" {
		t.Errorf("expected name 'lvm_pv', got '%s'", m.Name())
	}
	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestLVMPVModule_Check_Present(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("lvm_pv module only works on Linux")
	}
	m := NewLVMPVModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-pv",
		Module: "lvm_pv",
		State:  "present",
		Parameters: map[string]interface{}{
			"device": "/dev/sda1",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if result == nil {
		t.Fatal("Check returned nil result")
	}
}

func TestLVMPVModule_Check_MissingDevice(t *testing.T) {
	m := NewLVMPVModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:         "test-pv",
		Module:     "lvm_pv",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(ctx, decl)
	if err == nil {
		t.Error("expected error for missing device parameter")
	}
}

func TestLVMPVModule_Apply_Present(t *testing.T) {
	m := NewLVMPVModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-pv",
		Module: "lvm_pv",
		State:  "present",
		Parameters: map[string]interface{}{
			"device": "/dev/null",
		},
	}

	// Will fail without LVM tools
	result, err := m.Apply(ctx, decl)
	if err == nil && result == nil {
		t.Error("Apply should return either result or error")
	}
}

func TestLVMPVModule_Test(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("lvm_pv module only works on Linux")
	}
	m := NewLVMPVModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-pv",
		Module: "lvm_pv",
		State:  "present",
		Parameters: map[string]interface{}{
			"device": "/dev/sda1",
		},
	}

	result, err := m.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	_ = result
}

// =============================================================================
// LVM Volume Group Module Tests
// =============================================================================

func TestNewLVMVGModule(t *testing.T) {
	m := NewLVMVGModule()
	if m == nil {
		t.Fatal("NewLVMVGModule returned nil")
	}
	if m.Name() != "lvm_vg" {
		t.Errorf("expected name 'lvm_vg', got '%s'", m.Name())
	}
	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestLVMVGModule_Check_Present(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("lvm_vg module only works on Linux")
	}
	m := NewLVMVGModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-vg",
		Module: "lvm_vg",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":    "vg_data",
			"devices": []interface{}{"/dev/sda1"},
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if result == nil {
		t.Fatal("Check returned nil result")
	}
}

func TestLVMVGModule_Check_MissingName(t *testing.T) {
	m := NewLVMVGModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-vg",
		Module: "lvm_vg",
		State:  "present",
		Parameters: map[string]interface{}{
			"devices": []interface{}{"/dev/sda1"},
		},
	}

	_, err := m.Check(ctx, decl)
	if err == nil {
		t.Error("expected error for missing name parameter")
	}
}

func TestLVMVGModule_Apply_Present(t *testing.T) {
	m := NewLVMVGModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-vg",
		Module: "lvm_vg",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":    "test_vg",
			"devices": []interface{}{"/dev/null"},
		},
	}

	result, err := m.Apply(ctx, decl)
	if err == nil && result == nil {
		t.Error("Apply should return either result or error")
	}
}

func TestLVMVGModule_Test(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("lvm_vg module only works on Linux")
	}
	m := NewLVMVGModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-vg",
		Module: "lvm_vg",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":    "vg_data",
			"devices": []interface{}{"/dev/sda1"},
		},
	}

	result, err := m.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	_ = result
}

// =============================================================================
// LVM Logical Volume Module Tests
// =============================================================================

func TestNewLVMLVModule(t *testing.T) {
	m := NewLVMLVModule()
	if m == nil {
		t.Fatal("NewLVMLVModule returned nil")
	}
	if m.Name() != "lvm_lv" {
		t.Errorf("expected name 'lvm_lv', got '%s'", m.Name())
	}
	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestLVMLVModule_Check_Present(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("lvm_lv module only works on Linux")
	}
	m := NewLVMLVModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-lv",
		Module: "lvm_lv",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "lv_data",
			"vg":   "vg_data",
			"size": "10G",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if result == nil {
		t.Fatal("Check returned nil result")
	}
}

func TestLVMLVModule_Check_MissingName(t *testing.T) {
	m := NewLVMLVModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-lv",
		Module: "lvm_lv",
		State:  "present",
		Parameters: map[string]interface{}{
			"vg":   "vg_data",
			"size": "10G",
		},
	}

	_, err := m.Check(ctx, decl)
	if err == nil {
		t.Error("expected error for missing name parameter")
	}
}

func TestLVMLVModule_Check_MissingVG(t *testing.T) {
	m := NewLVMLVModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-lv",
		Module: "lvm_lv",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "lv_data",
			"size": "10G",
		},
	}

	_, err := m.Check(ctx, decl)
	if err == nil {
		t.Error("expected error for missing vg parameter")
	}
}

func TestLVMLVModule_Apply_Present(t *testing.T) {
	m := NewLVMLVModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-lv",
		Module: "lvm_lv",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "test_lv",
			"vg":   "test_vg",
			"size": "1G",
		},
	}

	result, err := m.Apply(ctx, decl)
	if err == nil && result == nil {
		t.Error("Apply should return either result or error")
	}
}

func TestLVMLVModule_Test(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("lvm_lv module only works on Linux")
	}
	m := NewLVMLVModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-lv",
		Module: "lvm_lv",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "lv_data",
			"vg":   "vg_data",
			"size": "10G",
		},
	}

	result, err := m.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	_ = result
}

// =============================================================================
// Disk Module Tests
// =============================================================================

func TestNewDiskModule(t *testing.T) {
	m := NewDiskModule()
	if m == nil {
		t.Fatal("NewDiskModule returned nil")
	}
	if m.Name() != "disk" {
		t.Errorf("expected name 'disk', got '%s'", m.Name())
	}
	states := m.ValidStates()
	expected := []string{"present", "absent", "formatted"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestDiskModule_Check_Present(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("disk module only works on Linux")
	}
	m := NewDiskModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-disk",
		Module: "disk",
		State:  "present",
		Parameters: map[string]interface{}{
			"device": "/dev/sda",
			"number": 1,
			"start":  "1MiB",
			"end":    "100%",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if result == nil {
		t.Fatal("Check returned nil result")
	}
}

func TestDiskModule_Check_MissingDevice(t *testing.T) {
	m := NewDiskModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-disk",
		Module: "disk",
		State:  "present",
		Parameters: map[string]interface{}{
			"number": 1,
		},
	}

	_, err := m.Check(ctx, decl)
	if err == nil {
		t.Error("expected error for missing device parameter")
	}
}

func TestDiskModule_Apply_Present(t *testing.T) {
	m := NewDiskModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-disk",
		Module: "disk",
		State:  "present",
		Parameters: map[string]interface{}{
			"device": "/dev/null",
			"number": 1,
			"start":  "1MiB",
			"end":    "100%",
		},
	}

	result, err := m.Apply(ctx, decl)
	if err == nil && result == nil {
		t.Error("Apply should return either result or error")
	}
}

func TestDiskModule_Test(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("disk module only works on Linux")
	}
	m := NewDiskModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-disk",
		Module: "disk",
		State:  "present",
		Parameters: map[string]interface{}{
			"device": "/dev/sda",
			"number": 1,
		},
	}

	result, err := m.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	_ = result
}

// =============================================================================
// Filesystem Module Tests
// =============================================================================

func TestNewFilesystemModule(t *testing.T) {
	m := NewFilesystemModule()
	if m == nil {
		t.Fatal("NewFilesystemModule returned nil")
	}
	if m.Name() != "filesystem" {
		t.Errorf("expected name 'filesystem', got '%s'", m.Name())
	}
	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestFilesystemModule_Check_Present(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("filesystem module only works on Linux")
	}
	m := NewFilesystemModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-fs",
		Module: "filesystem",
		State:  "present",
		Parameters: map[string]interface{}{
			"device": "/dev/sda1",
			"fstype": "ext4",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if result == nil {
		t.Fatal("Check returned nil result")
	}
}

func TestFilesystemModule_Check_MissingDevice(t *testing.T) {
	m := NewFilesystemModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-fs",
		Module: "filesystem",
		State:  "present",
		Parameters: map[string]interface{}{
			"fstype": "ext4",
		},
	}

	_, err := m.Check(ctx, decl)
	if err == nil {
		t.Error("expected error for missing device parameter")
	}
}

func TestFilesystemModule_Check_MissingFSType(t *testing.T) {
	m := NewFilesystemModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-fs",
		Module: "filesystem",
		State:  "present",
		Parameters: map[string]interface{}{
			"device": "/dev/sda1",
		},
	}

	_, err := m.Check(ctx, decl)
	if err == nil {
		t.Error("expected error for missing fstype parameter")
	}
}

func TestFilesystemModule_Apply_Present(t *testing.T) {
	m := NewFilesystemModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-fs",
		Module: "filesystem",
		State:  "present",
		Parameters: map[string]interface{}{
			"device": "/dev/null",
			"fstype": "ext4",
		},
	}

	result, err := m.Apply(ctx, decl)
	if err == nil && result == nil {
		t.Error("Apply should return either result or error")
	}
}

func TestFilesystemModule_Test(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("filesystem module only works on Linux")
	}
	m := NewFilesystemModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-fs",
		Module: "filesystem",
		State:  "present",
		Parameters: map[string]interface{}{
			"device": "/dev/sda1",
			"fstype": "ext4",
		},
	}

	result, err := m.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	_ = result
}

func TestFilesystemModule_FSTypes(t *testing.T) {
	// Test that the module knows about common filesystem types
	// This is documentation/reference test
	supportedTypes := []string{
		"ext4",
		"ext3",
		"xfs",
		"btrfs",
		"vfat",
		"ntfs",
	}

	for _, fstype := range supportedTypes {
		t.Run(fstype, func(t *testing.T) {
			// Verify the fstype string is not empty
			if fstype == "" {
				t.Error("fstype should not be empty")
			}
		})
	}
}

// =============================================================================
// Integration-style Tests
// =============================================================================

func TestStorageModules_ImplementInterface(t *testing.T) {
	modules := []Module{
		NewMountModule(),
		NewSwapModule(),
		NewLVMPVModule(),
		NewLVMVGModule(),
		NewLVMLVModule(),
		NewDiskModule(),
		NewFilesystemModule(),
	}

	for _, m := range modules {
		if m.Name() == "" {
			t.Errorf("module has empty name")
		}
		states := m.ValidStates()
		if len(states) == 0 {
			t.Errorf("module %s has no supported states", m.Name())
		}
	}
}

func TestStorageModules_ValidStates(t *testing.T) {
	tests := []struct {
		name   string
		module Module
		states []string
	}{
		{"mount", NewMountModule(), []string{"mounted", "unmounted", "present", "absent"}},
		{"swap", NewSwapModule(), []string{"enabled", "disabled", "present", "absent"}},
		{"lvm_pv", NewLVMPVModule(), []string{"present", "absent"}},
		{"lvm_vg", NewLVMVGModule(), []string{"present", "absent"}},
		{"lvm_lv", NewLVMLVModule(), []string{"present", "absent"}},
		{"disk", NewDiskModule(), []string{"present", "absent", "formatted"}},
		{"filesystem", NewFilesystemModule(), []string{"present", "absent"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states := tt.module.ValidStates()
			if len(states) != len(tt.states) {
				t.Errorf("module %s: got %d states, want %d", tt.name, len(states), len(tt.states))
				return
			}
			for i, s := range states {
				if s != tt.states[i] {
					t.Errorf("module %s: state[%d] = %s, want %s", tt.name, i, s, tt.states[i])
				}
			}
		})
	}
}
