package statemgmt

import (
	"context"
	"fmt"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// UserModule implements user management
type UserModule struct {
	*BaseModule
}

// NewUserModule creates a new user module
func NewUserModule() *UserModule {
	return &UserModule{
		BaseModule: NewBaseModule("user", []string{"present", "absent"}),
	}
}

// Check checks if a user exists and matches desired state
func (m *UserModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	username := decl.ID

	// Check if user exists
	usr, err := user.Lookup(username)
	if err != nil {
		if _, ok := err.(user.UnknownUserError); ok {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = (decl.State == "absent")
			return result, nil
		}
		return nil, fmt.Errorf("failed to lookup user: %w", err)
	}

	result.Present = true
	result.CurrentState = "present"
	result.Metadata["uid"] = usr.Uid
	result.Metadata["gid"] = usr.Gid
	result.Metadata["home"] = usr.HomeDir
	result.Metadata["name"] = usr.Name

	// For "absent" state, user should not exist
	if decl.State == "absent" {
		result.Matches = false
		result.Diff["state"] = map[string]string{"current": "present", "desired": "absent"}
		return result, nil
	}

	// For "present" state, check attributes
	result.Matches = true

	// Check UID
	if desiredUID := getIntParameter(decl, "uid", -1); desiredUID >= 0 {
		currentUID, _ := strconv.Atoi(usr.Uid)
		if currentUID != desiredUID {
			result.Matches = false
			result.Diff["uid"] = map[string]int{"current": currentUID, "desired": desiredUID}
		}
	}

	// Check GID
	if desiredGID := getIntParameter(decl, "gid", -1); desiredGID >= 0 {
		currentGID, _ := strconv.Atoi(usr.Gid)
		if currentGID != desiredGID {
			result.Matches = false
			result.Diff["gid"] = map[string]int{"current": currentGID, "desired": desiredGID}
		}
	}

	// Check home directory
	if desiredHome := getStringParameter(decl, "home", ""); desiredHome != "" {
		if usr.HomeDir != desiredHome {
			result.Matches = false
			result.Diff["home"] = map[string]string{"current": usr.HomeDir, "desired": desiredHome}
		}
	}

	// Check shell (requires additional lookup on Unix)
	if desiredShell := getStringParameter(decl, "shell", ""); desiredShell != "" {
		currentShell, err := m.getUserShell(username)
		if err == nil && currentShell != desiredShell {
			result.Matches = false
			result.Diff["shell"] = map[string]string{"current": currentShell, "desired": desiredShell}
		}
	}

	// Check groups
	if desiredGroups := getStringSliceParameter(decl, "groups"); desiredGroups != nil {
		currentGroups, err := m.getUserGroups(username)
		if err == nil {
			if !stringSlicesEqual(currentGroups, desiredGroups) {
				result.Matches = false
				result.Diff["groups"] = map[string]interface{}{
					"current": currentGroups,
					"desired": desiredGroups,
				}
			}
		}
	}

	return result, nil
}

// Apply applies the user state
func (m *UserModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	// Check current state
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to check current state: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// If already in desired state, no changes needed
	if checkResult.Matches {
		result.Success = true
		result.Changed = false
		result.Comment = "Already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Apply changes
	var applyErr error
	switch decl.State {
	case "present":
		if !checkResult.Present {
			applyErr = m.createUser(ctx, decl, result)
		} else {
			applyErr = m.modifyUser(ctx, decl, result)
		}
	case "absent":
		applyErr = m.deleteUser(ctx, decl, result)
	default:
		applyErr = fmt.Errorf("unsupported state: %s", decl.State)
	}

	if applyErr != nil {
		result.Error = applyErr
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to apply state: %v", applyErr)
	} else {
		result.Success = true
		result.Changed = true
		result.Changes = checkResult.Diff
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// Test tests if the user is in the desired state
func (m *UserModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// createUser creates a new user
func (m *UserModule) createUser(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	username := decl.ID
	args := []string{}

	if runtime.GOOS == "linux" || runtime.GOOS == "freebsd" {
		// Linux/FreeBSD: useradd
		args = append(args, "useradd")

		// UID
		if uid := getIntParameter(decl, "uid", -1); uid >= 0 {
			args = append(args, "-u", strconv.Itoa(uid))
		}

		// GID
		if gid := getIntParameter(decl, "gid", -1); gid >= 0 {
			args = append(args, "-g", strconv.Itoa(gid))
		}

		// Home directory
		if home := getStringParameter(decl, "home", ""); home != "" {
			args = append(args, "-d", home)
		}

		// Shell
		if shell := getStringParameter(decl, "shell", ""); shell != "" {
			args = append(args, "-s", shell)
		}

		// Create home
		if getBoolParameter(decl, "createhome", true) {
			args = append(args, "-m")
		} else {
			args = append(args, "-M")
		}

		// System user
		if getBoolParameter(decl, "system", false) {
			args = append(args, "-r")
		}

		// Groups
		if groups := getStringSliceParameter(decl, "groups"); groups != nil && len(groups) > 0 {
			args = append(args, "-G", strings.Join(groups, ","))
		}

		args = append(args, username)

	} else if runtime.GOOS == "darwin" {
		// macOS: dscl
		return fmt.Errorf("user creation on macOS requires dscl commands (not yet implemented)")
	} else {
		return fmt.Errorf("user creation not supported on %s", runtime.GOOS)
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create user: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("User %s created", username)
	return nil
}

// modifyUser modifies an existing user
func (m *UserModule) modifyUser(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	username := decl.ID
	args := []string{}

	if runtime.GOOS == "linux" || runtime.GOOS == "freebsd" {
		// Linux/FreeBSD: usermod
		args = append(args, "usermod")

		// UID
		if uid := getIntParameter(decl, "uid", -1); uid >= 0 {
			args = append(args, "-u", strconv.Itoa(uid))
		}

		// GID
		if gid := getIntParameter(decl, "gid", -1); gid >= 0 {
			args = append(args, "-g", strconv.Itoa(gid))
		}

		// Home directory
		if home := getStringParameter(decl, "home", ""); home != "" {
			args = append(args, "-d", home)
		}

		// Shell
		if shell := getStringParameter(decl, "shell", ""); shell != "" {
			args = append(args, "-s", shell)
		}

		// Groups
		if groups := getStringSliceParameter(decl, "groups"); groups != nil && len(groups) > 0 {
			args = append(args, "-G", strings.Join(groups, ","))
		}

		// Only run if there are changes to make
		if len(args) == 1 {
			result.Comment = "No changes needed"
			return nil
		}

		args = append(args, username)

	} else if runtime.GOOS == "darwin" {
		return fmt.Errorf("user modification on macOS requires dscl commands (not yet implemented)")
	} else {
		return fmt.Errorf("user modification not supported on %s", runtime.GOOS)
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to modify user: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("User %s modified", username)
	return nil
}

// deleteUser deletes a user
func (m *UserModule) deleteUser(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	username := decl.ID
	var cmd *exec.Cmd

	if runtime.GOOS == "linux" || runtime.GOOS == "freebsd" {
		cmd = exec.CommandContext(ctx, "userdel", username)
	} else if runtime.GOOS == "darwin" {
		return fmt.Errorf("user deletion on macOS requires dscl commands (not yet implemented)")
	} else {
		return fmt.Errorf("user deletion not supported on %s", runtime.GOOS)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete user: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("User %s deleted", username)
	return nil
}

// getUserShell gets the user's shell
func (m *UserModule) getUserShell(username string) (string, error) {
	if runtime.GOOS != "linux" && runtime.GOOS != "freebsd" {
		return "", fmt.Errorf("unsupported OS")
	}

	cmd := exec.Command("getent", "passwd", username)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Parse passwd entry: username:x:uid:gid:gecos:home:shell
	parts := strings.Split(strings.TrimSpace(string(output)), ":")
	if len(parts) >= 7 {
		return parts[6], nil
	}

	return "", fmt.Errorf("failed to parse passwd entry")
}

// getUserGroups gets the user's groups
func (m *UserModule) getUserGroups(username string) ([]string, error) {
	cmd := exec.Command("groups", username)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Parse output: "username : group1 group2 group3"
	outputStr := strings.TrimSpace(string(output))
	parts := strings.Split(outputStr, ":")
	if len(parts) < 2 {
		return nil, fmt.Errorf("unexpected groups output format")
	}

	groups := strings.Fields(parts[1])
	return groups, nil
}

// stringSlicesEqual checks if two string slices are equal (ignoring order)
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	aMap := make(map[string]bool)
	for _, s := range a {
		aMap[s] = true
	}

	for _, s := range b {
		if !aMap[s] {
			return false
		}
	}

	return true
}

func init() {
	RegisterModule(NewUserModule())
}
