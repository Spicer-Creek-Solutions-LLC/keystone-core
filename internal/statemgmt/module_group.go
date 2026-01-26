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
		// macOS: use dscl (Directory Service command line)
		return m.createGroupDarwin(ctx, decl, result)
	} else if runtime.GOOS == "windows" {
		// Windows: use net localgroup
		return m.createGroupWindows(ctx, decl, result)
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

// createGroupDarwin creates a group on macOS using dscl
func (m *GroupModule) createGroupDarwin(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	groupName := decl.ID
	groupPath := "/Groups/" + groupName

	// Get or generate GID
	gid := getIntParameter(decl, "gid", -1)
	if gid < 0 {
		// Find next available GID (start at 501 for regular groups)
		var err error
		gid, err = m.findNextAvailableGID(ctx)
		if err != nil {
			return fmt.Errorf("failed to find available GID: %w", err)
		}
	}

	// Get real name (defaults to group name)
	realName := getStringParameter(decl, "description", groupName)

	// Create group record
	if err := m.dsclCreate(ctx, groupPath); err != nil {
		return fmt.Errorf("failed to create group record: %w", err)
	}

	// Set group properties
	properties := map[string]string{
		"PrimaryGroupID": strconv.Itoa(gid),
		"RealName":       realName,
		"Password":       "*", // No password (standard for groups)
	}

	for prop, value := range properties {
		if err := m.dsclCreateProperty(ctx, groupPath, prop, value); err != nil {
			// Try to clean up on failure
			_ = m.dsclDelete(ctx, groupPath)
			return fmt.Errorf("failed to set %s: %w", prop, err)
		}
	}

	// Add members if specified
	if members := getStringSliceParameter(decl, "members"); members != nil && len(members) > 0 {
		for _, member := range members {
			if err := m.addUserToGroupDarwin(ctx, groupName, member); err != nil {
				// Log but don't fail - user might not exist
				result.Comment = fmt.Sprintf("Group %s created (warning: failed to add member %s: %v)", groupName, member, err)
			}
		}
	}

	if result.Comment == "" {
		result.Comment = fmt.Sprintf("Group %s created", groupName)
	}
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
		// macOS: use dscl
		return m.modifyGroupDarwin(ctx, decl, result)
	} else if runtime.GOOS == "windows" {
		// Windows: use net localgroup
		return m.modifyGroupWindows(ctx, decl, result)
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

// modifyGroupDarwin modifies a group on macOS using dscl
func (m *GroupModule) modifyGroupDarwin(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	groupName := decl.ID
	groupPath := "/Groups/" + groupName
	var modified bool

	// Update GID if specified
	if gid := getIntParameter(decl, "gid", -1); gid >= 0 {
		if err := m.dsclCreateProperty(ctx, groupPath, "PrimaryGroupID", strconv.Itoa(gid)); err != nil {
			return fmt.Errorf("failed to set GID: %w", err)
		}
		modified = true
	}

	// Update description if specified
	if desc := getStringParameter(decl, "description", ""); desc != "" {
		if err := m.dsclCreateProperty(ctx, groupPath, "RealName", desc); err != nil {
			return fmt.Errorf("failed to set description: %w", err)
		}
		modified = true
	}

	// Update members if specified
	if members := getStringSliceParameter(decl, "members"); members != nil {
		if err := m.updateGroupMembersDarwin(ctx, groupName, members); err != nil {
			return err
		}
		modified = true
	}

	if !modified {
		result.Comment = "No changes needed"
	} else {
		result.Comment = fmt.Sprintf("Group %s modified", groupName)
	}
	return nil
}

// deleteGroup deletes a group
func (m *GroupModule) deleteGroup(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	groupName := decl.ID

	if runtime.GOOS == "linux" || runtime.GOOS == "freebsd" {
		cmd := exec.CommandContext(ctx, "groupdel", groupName)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to delete group: %w (output: %s)", err, string(output))
		}
	} else if runtime.GOOS == "darwin" {
		// macOS: use dscl to delete group
		groupPath := "/Groups/" + groupName
		if err := m.dsclDelete(ctx, groupPath); err != nil {
			return fmt.Errorf("failed to delete group: %w", err)
		}
	} else if runtime.GOOS == "windows" {
		// Windows: use net localgroup /delete
		cmd := exec.CommandContext(ctx, "net", "localgroup", groupName, "/delete")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to delete group: %w (output: %s)", err, string(output))
		}
	} else {
		return fmt.Errorf("group deletion not supported on %s", runtime.GOOS)
	}

	result.Comment = fmt.Sprintf("Group %s deleted", groupName)
	return nil
}

// getGroupMembers gets the members of a group
func (m *GroupModule) getGroupMembers(groupName string) ([]string, error) {
	if runtime.GOOS == "darwin" {
		return m.getGroupMembersDarwin(groupName)
	}

	if runtime.GOOS == "windows" {
		return m.getGroupMembersWindows(groupName)
	}

	if runtime.GOOS != "linux" && runtime.GOOS != "freebsd" {
		return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
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

// getGroupMembersDarwin gets the members of a group on macOS using dscl
func (m *GroupModule) getGroupMembersDarwin(groupName string) ([]string, error) {
	groupPath := "/Groups/" + groupName

	// Read GroupMembership property
	cmd := exec.Command("dscl", ".", "-read", groupPath, "GroupMembership")
	output, err := cmd.Output()
	if err != nil {
		// GroupMembership might not exist if group has no members
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Exit code 185 means the key doesn't exist (no members)
			if exitErr.ExitCode() != 0 {
				return []string{}, nil
			}
		}
		return nil, fmt.Errorf("failed to read group members: %w", err)
	}

	// Parse output: "GroupMembership: user1 user2 user3"
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		return []string{}, nil
	}

	// Split on first colon and take the rest
	if idx := strings.Index(outputStr, ":"); idx >= 0 {
		membersStr := strings.TrimSpace(outputStr[idx+1:])
		if membersStr == "" {
			return []string{}, nil
		}
		members := strings.Fields(membersStr)
		return members, nil
	}

	return []string{}, nil
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

// dsclCreate creates a new record in Directory Services
func (m *GroupModule) dsclCreate(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "dscl", ".", "-create", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("dscl create failed: %w (output: %s)", err, string(output))
	}
	return nil
}

// dsclCreateProperty sets a property on a Directory Services record
func (m *GroupModule) dsclCreateProperty(ctx context.Context, path, property, value string) error {
	cmd := exec.CommandContext(ctx, "dscl", ".", "-create", path, property, value)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("dscl create property failed: %w (output: %s)", err, string(output))
	}
	return nil
}

// dsclDelete deletes a record from Directory Services
func (m *GroupModule) dsclDelete(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "dscl", ".", "-delete", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("dscl delete failed: %w (output: %s)", err, string(output))
	}
	return nil
}

// findNextAvailableGID finds the next available GID on macOS
func (m *GroupModule) findNextAvailableGID(ctx context.Context) (int, error) {
	// List all groups and find the highest GID >= 501
	cmd := exec.CommandContext(ctx, "dscl", ".", "-list", "/Groups", "PrimaryGroupID")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to list groups: %w", err)
	}

	maxGID := 500 // Start at 500, so first available is 501
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			if gid, err := strconv.Atoi(fields[len(fields)-1]); err == nil {
				if gid >= 501 && gid > maxGID {
					maxGID = gid
				}
			}
		}
	}

	return maxGID + 1, nil
}

// addUserToGroupDarwin adds a user to a group on macOS
func (m *GroupModule) addUserToGroupDarwin(ctx context.Context, groupName, username string) error {
	groupPath := "/Groups/" + groupName
	cmd := exec.CommandContext(ctx, "dscl", ".", "-append", groupPath, "GroupMembership", username)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add user to group: %w (output: %s)", err, string(output))
	}
	return nil
}

// removeUserFromGroupDarwin removes a user from a group on macOS
func (m *GroupModule) removeUserFromGroupDarwin(ctx context.Context, groupName, username string) error {
	groupPath := "/Groups/" + groupName
	cmd := exec.CommandContext(ctx, "dscl", ".", "-delete", groupPath, "GroupMembership", username)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove user from group: %w (output: %s)", err, string(output))
	}
	return nil
}

// updateGroupMembersDarwin updates the members of a group on macOS
func (m *GroupModule) updateGroupMembersDarwin(ctx context.Context, groupName string, desiredMembers []string) error {
	currentMembers, err := m.getGroupMembersDarwin(groupName)
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
			if err := m.addUserToGroupDarwin(ctx, groupName, member); err != nil {
				return err
			}
		}
	}

	// Remove extra members
	for _, member := range currentMembers {
		if !desiredMap[member] {
			if err := m.removeUserFromGroupDarwin(ctx, groupName, member); err != nil {
				return err
			}
		}
	}

	return nil
}

// createGroupWindows creates a group on Windows using net localgroup
func (m *GroupModule) createGroupWindows(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	groupName := decl.ID

	// Build net localgroup command
	args := []string{"localgroup", groupName, "/add"}

	// Comment/description
	if comment := getStringParameter(decl, "description", ""); comment != "" {
		args = append(args, fmt.Sprintf("/comment:\"%s\"", comment))
	}

	cmd := exec.CommandContext(ctx, "net", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create group: %w (output: %s)", err, string(output))
	}

	// Add members if specified
	if members := getStringSliceParameter(decl, "members"); members != nil && len(members) > 0 {
		for _, member := range members {
			if err := m.addUserToGroupWindows(ctx, groupName, member); err != nil {
				result.Comment = fmt.Sprintf("Group %s created (warning: failed to add member %s: %v)", groupName, member, err)
			}
		}
	}

	if result.Comment == "" {
		result.Comment = fmt.Sprintf("Group %s created", groupName)
	}
	return nil
}

// modifyGroupWindows modifies a group on Windows
func (m *GroupModule) modifyGroupWindows(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	groupName := decl.ID
	var modified bool

	// Note: Windows net localgroup doesn't support modifying group properties directly
	// GID is not applicable on Windows (SIDs are used instead)
	// Comment can only be set at creation time with net localgroup

	// Update members if specified
	if members := getStringSliceParameter(decl, "members"); members != nil {
		if err := m.updateGroupMembersWindows(ctx, groupName, members); err != nil {
			return err
		}
		modified = true
	}

	if !modified {
		result.Comment = "No changes needed"
	} else {
		result.Comment = fmt.Sprintf("Group %s modified", groupName)
	}
	return nil
}

// getGroupMembersWindows gets the members of a group on Windows
func (m *GroupModule) getGroupMembersWindows(groupName string) ([]string, error) {
	// Use net localgroup to list members
	cmd := exec.Command("net", "localgroup", groupName)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get group members: %w", err)
	}

	// Parse output - members are listed after "Members" line and before "The command completed"
	var members []string
	lines := strings.Split(string(output), "\n")
	inMemberSection := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and dashes
		if line == "" || strings.HasPrefix(line, "---") {
			continue
		}

		// Check for "Members" header
		if strings.HasPrefix(line, "Members") {
			inMemberSection = true
			continue
		}

		// Check for end of member section
		if strings.HasPrefix(line, "The command completed") {
			break
		}

		// If we're in the member section and the line is not empty, it's a member
		if inMemberSection && line != "" {
			// Remove domain prefix if present (e.g., "DOMAIN\username" -> "username")
			if idx := strings.LastIndex(line, "\\"); idx >= 0 {
				line = line[idx+1:]
			}
			members = append(members, line)
		}
	}

	return members, nil
}

// addUserToGroupWindows adds a user to a group on Windows
func (m *GroupModule) addUserToGroupWindows(ctx context.Context, groupName, username string) error {
	cmd := exec.CommandContext(ctx, "net", "localgroup", groupName, username, "/add")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if user is already a member (error 1378)
		if strings.Contains(string(output), "1378") || strings.Contains(string(output), "already a member") {
			return nil
		}
		return fmt.Errorf("failed to add user to group: %w (output: %s)", err, string(output))
	}
	return nil
}

// removeUserFromGroupWindows removes a user from a group on Windows
func (m *GroupModule) removeUserFromGroupWindows(ctx context.Context, groupName, username string) error {
	cmd := exec.CommandContext(ctx, "net", "localgroup", groupName, username, "/delete")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove user from group: %w (output: %s)", err, string(output))
	}
	return nil
}

// updateGroupMembersWindows updates the members of a group on Windows
func (m *GroupModule) updateGroupMembersWindows(ctx context.Context, groupName string, desiredMembers []string) error {
	currentMembers, err := m.getGroupMembersWindows(groupName)
	if err != nil {
		return err
	}

	// Build maps for comparison (case-insensitive on Windows)
	currentMap := make(map[string]bool)
	for _, member := range currentMembers {
		currentMap[strings.ToLower(member)] = true
	}

	desiredMap := make(map[string]bool)
	for _, member := range desiredMembers {
		desiredMap[strings.ToLower(member)] = true
	}

	// Add missing members
	for _, member := range desiredMembers {
		if !currentMap[strings.ToLower(member)] {
			if err := m.addUserToGroupWindows(ctx, groupName, member); err != nil {
				return err
			}
		}
	}

	// Remove extra members
	for _, member := range currentMembers {
		if !desiredMap[strings.ToLower(member)] {
			if err := m.removeUserFromGroupWindows(ctx, groupName, member); err != nil {
				return err
			}
		}
	}

	return nil
}

func init() {
	RegisterModule(NewGroupModule())
}
