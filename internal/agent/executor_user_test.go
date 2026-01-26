package agent

import (
	"context"
	"os/user"
	"runtime"
	"testing"
	"time"
)

func TestLookupUserForSwitch_Empty(t *testing.T) {
	result, err := LookupUserForSwitch("")
	if err != nil {
		t.Fatalf("LookupUserForSwitch with empty string should not error: %v", err)
	}
	if result != nil {
		t.Error("LookupUserForSwitch with empty string should return nil")
	}
}

func TestLookupUserForSwitch_CurrentUser(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Skipf("Could not get current user: %v", err)
	}

	result, err := LookupUserForSwitch(currentUser.Username)
	if err != nil {
		t.Fatalf("LookupUserForSwitch failed for current user: %v", err)
	}

	if result == nil {
		t.Fatal("LookupUserForSwitch returned nil for current user")
	}

	if result.Username != currentUser.Username {
		t.Errorf("Expected username %q, got %q", currentUser.Username, result.Username)
	}

	if result.HomeDir == "" {
		t.Error("Expected HomeDir to be set")
	}
}

func TestLookupUserForSwitch_NonExistent(t *testing.T) {
	_, err := LookupUserForSwitch("nonexistent_user_12345")
	if err == nil {
		t.Error("LookupUserForSwitch should error for non-existent user")
	}
}

func TestCanSwitchUser_Empty(t *testing.T) {
	err := CanSwitchUser("")
	if err != nil {
		t.Errorf("CanSwitchUser with empty string should not error: %v", err)
	}
}

func TestCanSwitchUser_NonExistent(t *testing.T) {
	err := CanSwitchUser("nonexistent_user_12345")
	if err == nil {
		t.Error("CanSwitchUser should error for non-existent user")
	}
}

func TestCanSwitchUser_RequiresRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has different user switching requirements")
	}

	currentUser, err := user.Current()
	if err != nil {
		t.Skipf("Could not get current user: %v", err)
	}

	// If we're not root, we can't switch to other users
	if currentUser.Uid != "0" {
		// Try to switch to root - should fail
		err := CanSwitchUser("root")
		if err == nil {
			t.Error("Non-root user should not be able to switch to root")
		}
	}
}

func TestGetCurrentUser(t *testing.T) {
	username := GetCurrentUser()
	if username == "" {
		t.Error("GetCurrentUser should return a non-empty username")
	}

	// Should match os/user.Current()
	currentUser, err := user.Current()
	if err == nil && username != currentUser.Username {
		t.Errorf("GetCurrentUser returned %q, expected %q", username, currentUser.Username)
	}
}

func TestSetUserCredential_Nil(t *testing.T) {
	result := SetUserCredential(nil, nil)
	if result != nil {
		t.Error("SetUserCredential(nil, nil) should return nil")
	}
}

func TestExecuteWithUser_NonRootFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has different user switching requirements")
	}

	currentUser, err := user.Current()
	if err != nil {
		t.Skipf("Could not get current user: %v", err)
	}

	// Skip if running as root
	if currentUser.Uid == "0" {
		t.Skip("Test requires non-root user")
	}

	executor := NewExecutor()
	ctx := context.Background()

	req := &ExecuteCommandRequest{
		CommandID: "test-user-switch",
		Command:   "whoami",
		User:      "root",
		Timeout:   5 * time.Second,
	}

	_, err = executor.Execute(ctx, req, nil)
	if err == nil {
		t.Error("Execute with user switch should fail for non-root")
	}
}

func TestExecuteWithUser_SameUser(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Skipf("Could not get current user: %v", err)
	}

	executor := NewExecutor()
	ctx := context.Background()

	// Running as ourselves shouldn't require special privileges
	// but on Unix it still goes through the credential check which requires root
	if runtime.GOOS != "windows" && currentUser.Uid != "0" {
		// Non-root can't use setuid at all, even to the same user
		req := &ExecuteCommandRequest{
			CommandID: "test-same-user",
			Command:   "whoami",
			User:      currentUser.Username,
			Timeout:   5 * time.Second,
		}

		_, err = executor.Execute(ctx, req, nil)
		if err == nil {
			t.Error("Non-root should fail even when switching to same user")
		}
		return
	}

	// If we're root or on Windows without password, the behavior differs
	t.Skip("Root user or Windows - test not applicable")
}

func TestExecuteWithUser_EmptyUser(t *testing.T) {
	executor := NewExecutor()
	ctx := context.Background()

	// Empty user should not attempt user switching
	req := &ExecuteCommandRequest{
		CommandID: "test-no-user-switch",
		Command:   "echo",
		Args:      []string{"hello"},
		Timeout:   5 * time.Second,
	}

	result, err := executor.Execute(ctx, req, nil)
	if err != nil {
		t.Fatalf("Execute without user should succeed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}
}

func TestLookupUserForSwitch_ByUID(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Skipf("Could not get current user: %v", err)
	}

	// Try looking up by UID
	result, err := LookupUserForSwitch(currentUser.Uid)
	if err != nil {
		t.Fatalf("LookupUserForSwitch by UID failed: %v", err)
	}

	if result == nil {
		t.Fatal("LookupUserForSwitch by UID returned nil")
	}

	if result.Username != currentUser.Username {
		t.Errorf("Expected username %q, got %q", currentUser.Username, result.Username)
	}
}

func TestUserSwitchResult_Groups(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows doesn't have Unix-style groups")
	}

	currentUser, err := user.Current()
	if err != nil {
		t.Skipf("Could not get current user: %v", err)
	}

	result, err := LookupUserForSwitch(currentUser.Username)
	if err != nil {
		t.Fatalf("LookupUserForSwitch failed: %v", err)
	}

	// User should have at least their primary group
	if result.GID == 0 && currentUser.Gid != "0" {
		t.Error("Expected non-zero GID")
	}

	// Check that SysProcAttr was created for Unix
	if result.SysProcAttr == nil {
		t.Error("Expected SysProcAttr to be set on Unix")
	}

	if result.SysProcAttr != nil && result.SysProcAttr.Credential == nil {
		t.Error("Expected Credential to be set in SysProcAttr")
	}
}
