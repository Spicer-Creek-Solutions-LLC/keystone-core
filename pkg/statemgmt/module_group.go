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

// GroupModule implements group management
type GroupModule struct {
	*BaseModule
}

// NewGroupModule creates a new group module
func NewGroupModule() *GroupModule {
	return &GroupModule{
		BaseModule: NewBaseModule("group", []string{"present", "absent"}),
	}
}

// Check checks if a group exists and matches desired state
func (m *GroupModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	groupName := decl.ID

	// Check if group exists
	grp, err := user.LookupGroup(groupName)
	if err != nil {
		if _, ok := err.(user.UnknownGroupError); ok {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = (decl.State == "absent")
			return result, nil
		}
		return nil, fmt.Errorf("failed to lookup group: %w", err)
	}

	result.Present = true
	result.CurrentState = "present"
	result.Metadata["gid"] = grp.Gid
	result.Metadata["name"] = grp.Name

	// For "absent" state, group should not exist
	if decl.State == "absent" {
		result.Matches = false
		result.Diff["state"] = map[string]string{"current": "present", "desired": "absent"}
		return result, nil
	}

	// For "present" state, check attributes
	result.Matches = true

	// Check GID
	if desiredGID := getIntParameter(decl, "gid", -1); desiredGID >= 0 {
		currentGID, _ := strconv.Atoi(grp.Gid)
		if currentGID != desiredGID {
			result.Matches = false
			result.Diff["gid"] = map[string]int{"current": currentGID, "desired": desiredGID}
		}
	}

	// Check members
	if desiredMembers := getStringSliceParameter(decl, "members"); desiredMembers != nil {
		currentMembers, err := m.getGroupMembers(groupName)
		if err == nil {
			if !stringSlicesEqual(currentMembers, desiredMembers) {
				result.Matches = false
				result.Diff["members"] = map[string]interface{}{
					"current": currentMembers,
					"desired": desiredMembers,
				}
			}
		}
	}

	return result, nil
}

// Apply applies the group state
func (m *GroupModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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
			applyErr = m.createGroup(ctx, decl, result)
		} else {
			applyErr = m.modifyGroup(ctx, decl, result)
		}
	case "absent":
		applyErr = m.deleteGroup(ctx, decl, result)
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

// Test tests if the group is in the desired state
func (m *GroupModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// createGroup creates a new group
func (m *GroupModule) createGroup(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	groupName := decl.ID
	args := []string{}

	if runtime.GOOS == "linux" || runtime.GOOS == "freebsd" {
		// Linux/FreeBSD: groupadd
		args = append(args, "groupadd")

		// GID
		if gid := getIntParameter(decl, "gid", -1); gid >= 0 {
			args = append(args, "-g", strconv.Itoa(gid))
		}

		// System group
		if getBoolParameter(decl, "system", false) {
			args = append(args, "-r")
		}

		args = append(args, groupName)

	} else if runtime.GOOS == "darwin" {
		return fmt.Errorf("group creation on macOS requires dscl commands (not yet implemented)")
	} else {
		return fmt.Errorf("group creation not supported on %s", runtime.GOOS)
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create group: %w (output: %s)", err, string(output))
	}

	// Add members if specified
	if members := getStringSliceParameter(decl, "members"); members != nil && len(members) > 0 {
		for _, member := range members {
			if err := m.addUserToGroup(ctx, groupName, member); err != nil {
				return fmt.Errorf("failed to add member %s: %w", member, err)
			}
		}
	}

	result.Comment = fmt.Sprintf("Group %s created", groupName)
	return nil
}

// modifyGroup modifies an existing group
func (m *GroupModule) modifyGroup(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	groupName := decl.ID
	args := []string{}

	if runtime.GOOS == "linux" || runtime.GOOS == "freebsd" {
		// Linux/FreeBSD: groupmod
		args = append(args, "groupmod")

		// GID
		if gid := getIntParameter(decl, "gid", -1); gid >= 0 {
			args = append(args, "-g", strconv.Itoa(gid))
		}

		// Only run if there are changes to make
		if len(args) == 1 {
			// No GID change, but might need to update members
			if members := getStringSliceParameter(decl, "members"); members != nil {
				return m.updateGroupMembers(ctx, groupName, members)
			}
			result.Comment = "No changes needed"
			return nil
		}

		args = append(args, groupName)

	} else if runtime.GOOS == "darwin" {
		return fmt.Errorf("group modification on macOS requires dscl commands (not yet implemented)")
	} else {
		return fmt.Errorf("group modification not supported on %s", runtime.GOOS)
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to modify group: %w (output: %s)", err, string(output))
	}

	// Update members if specified
	if members := getStringSliceParameter(decl, "members"); members != nil {
		if err := m.updateGroupMembers(ctx, groupName, members); err != nil {
			return err
		}
	}

	result.Comment = fmt.Sprintf("Group %s modified", groupName)
	return nil
}

// deleteGroup deletes a group
func (m *GroupModule) deleteGroup(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	groupName := decl.ID
	var cmd *exec.Cmd

	if runtime.GOOS == "linux" || runtime.GOOS == "freebsd" {
		cmd = exec.CommandContext(ctx, "groupdel", groupName)
	} else if runtime.GOOS == "darwin" {
		return fmt.Errorf("group deletion on macOS requires dscl commands (not yet implemented)")
	} else {
		return fmt.Errorf("group deletion not supported on %s", runtime.GOOS)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete group: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Group %s deleted", groupName)
	return nil
}

// getGroupMembers gets the members of a group
func (m *GroupModule) getGroupMembers(groupName string) ([]string, error) {
	if runtime.GOOS != "linux" && runtime.GOOS != "freebsd" {
		return nil, fmt.Errorf("unsupported OS")
	}

	cmd := exec.Command("getent", "group", groupName)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Parse group entry: groupname:x:gid:member1,member2,member3
	parts := strings.Split(strings.TrimSpace(string(output)), ":")
	if len(parts) >= 4 {
		if parts[3] == "" {
			return []string{}, nil
		}
		members := strings.Split(parts[3], ",")
		return members, nil
	}

	return nil, fmt.Errorf("failed to parse group entry")
}

// updateGroupMembers updates the members of a group
func (m *GroupModule) updateGroupMembers(ctx context.Context, groupName string, desiredMembers []string) error {
	currentMembers, err := m.getGroupMembers(groupName)
	if err != nil {
		return err
	}

	// Determine which members to add and remove
	currentMap := make(map[string]bool)
	for _, member := range currentMembers {
		currentMap[member] = true
	}

	desiredMap := make(map[string]bool)
	for _, member := range desiredMembers {
		desiredMap[member] = true
	}

	// Add missing members
	for _, member := range desiredMembers {
		if !currentMap[member] {
			if err := m.addUserToGroup(ctx, groupName, member); err != nil {
				return err
			}
		}
	}

	// Remove extra members
	for _, member := range currentMembers {
		if !desiredMap[member] {
			if err := m.removeUserFromGroup(ctx, groupName, member); err != nil {
				return err
			}
		}
	}

	return nil
}

// addUserToGroup adds a user to a group
func (m *GroupModule) addUserToGroup(ctx context.Context, groupName, username string) error {
	cmd := exec.CommandContext(ctx, "usermod", "-aG", groupName, username)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add user to group: %w (output: %s)", err, string(output))
	}
	return nil
}

// removeUserFromGroup removes a user from a group
func (m *GroupModule) removeUserFromGroup(ctx context.Context, groupName, username string) error {
	cmd := exec.CommandContext(ctx, "gpasswd", "-d", username, groupName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove user from group: %w (output: %s)", err, string(output))
	}
	return nil
}

func init() {
	RegisterModule(NewGroupModule())
}
