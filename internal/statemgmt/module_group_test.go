package statemgmt

import (
	"context"
	"runtime"
	"testing"
)

func TestGroupModule_NewGroupModule(t *testing.T) {
	module := NewGroupModule()
	if module == nil {
		t.Fatal("NewGroupModule returned nil")
	}
	if module.Name() != "group" {
		t.Errorf("Expected name 'group', got '%s'", module.Name())
	}
	validStates := module.ValidStates()
	if len(validStates) != 2 {
		t.Errorf("Expected 2 valid states, got %d", len(validStates))
	}
}

func TestGroupModule_CheckGroupExists(t *testing.T) {
	module := NewGroupModule()
	ctx := context.Background()

	// Check for wheel/admin group (should exist on all Unix-like systems)
	var groupName string
	if runtime.GOOS == "darwin" {
		groupName = "wheel"
	} else {
		groupName = "root"
	}

	decl := &StateDeclaration{
		ID:         groupName,
		Module:     "group",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	result, err := module.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if !result.Present {
		t.Errorf("Expected %s group to be present", groupName)
	}
	if result.CurrentState != "present" {
		t.Errorf("Expected current state 'present', got '%s'", result.CurrentState)
	}
}

func TestGroupModule_CheckGroupNotExists(t *testing.T) {
	module := NewGroupModule()
	ctx := context.Background()

	// Check for non-existent group
	decl := &StateDeclaration{
		ID:         "thisgroupdefinitelydoesnotexist12345",
		Module:     "group",
		State:      "absent",
		Parameters: map[string]interface{}{},
	}

	result, err := module.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("Expected non-existent group to not be present")
	}
	if result.CurrentState != "absent" {
		t.Errorf("Expected current state 'absent', got '%s'", result.CurrentState)
	}
	if !result.Matches {
		t.Error("Expected state to match (absent group should match absent state)")
	}
}

func TestGroupModule_GetGroupMembers(t *testing.T) {
	module := NewGroupModule()

	// Get members for wheel/root group (exists on all Unix-like systems)
	var groupName string
	if runtime.GOOS == "darwin" {
		groupName = "wheel"
	} else {
		groupName = "root"
	}

	members, err := module.getGroupMembers(groupName)
	if err != nil {
		t.Fatalf("getGroupMembers failed: %v", err)
	}

	// Members might be empty but shouldn't error
	if members == nil {
		t.Error("Expected non-nil members slice")
	}
}

// Darwin-specific tests

func TestGroupModule_FindNextAvailableGID_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping macOS-specific test")
	}

	module := NewGroupModule()
	ctx := context.Background()

	gid, err := module.findNextAvailableGID(ctx)
	if err != nil {
		t.Fatalf("findNextAvailableGID failed: %v", err)
	}

	// GID should be >= 501 (regular groups on macOS start at 501)
	if gid < 501 {
		t.Errorf("Expected GID >= 501, got %d", gid)
	}
}

func TestGroupModule_GetGroupMembersDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping macOS-specific test")
	}

	module := NewGroupModule()

	// Get members for wheel group (system group with members)
	members, err := module.getGroupMembersDarwin("wheel")
	if err != nil {
		t.Fatalf("getGroupMembersDarwin failed: %v", err)
	}

	// wheel group typically has root as a member on macOS
	if members == nil {
		t.Error("Expected non-nil members slice")
	}
}

func TestGroupModule_Check_Darwin_GroupAttributes(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping macOS-specific test")
	}

	module := NewGroupModule()
	ctx := context.Background()

	// Check wheel group with specific attributes
	decl := &StateDeclaration{
		ID:     "wheel",
		Module: "group",
		State:  "present",
		Parameters: map[string]interface{}{
			"gid": 0, // wheel is always GID 0 on macOS
		},
	}

	result, err := module.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if !result.Present {
		t.Error("Expected wheel group to be present")
	}

	// GID 0 should match
	if !result.Matches {
		t.Errorf("Expected state to match (wheel should have GID 0), diff: %v", result.Diff)
	}
}

func TestGroupModule_DsclHelpers_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping macOS-specific test")
	}

	// Just verify the helpers exist and can be called
	// We don't actually create/delete groups in tests as that requires root

	module := NewGroupModule()

	// Test that the module has the required methods
	if module == nil {
		t.Fatal("NewGroupModule returned nil")
	}

	// The methods exist if this compiles
	_ = module.dsclCreate
	_ = module.dsclCreateProperty
	_ = module.dsclDelete
	_ = module.findNextAvailableGID
	_ = module.addUserToGroupDarwin
	_ = module.removeUserFromGroupDarwin
	_ = module.updateGroupMembersDarwin
	_ = module.getGroupMembersDarwin
}
