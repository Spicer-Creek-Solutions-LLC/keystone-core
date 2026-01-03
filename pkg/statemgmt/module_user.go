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
		// macOS: use dscl (Directory Service command line)
		return m.createUserDarwin(ctx, decl, result)
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

// createUserDarwin creates a user on macOS using dscl
func (m *UserModule) createUserDarwin(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	username := decl.ID
	userPath := "/Users/" + username

	// Get or generate UID
	uid := getIntParameter(decl, "uid", -1)
	if uid < 0 {
		// Find next available UID (start at 501 for regular users)
		var err error
		uid, err = m.findNextAvailableUID(ctx)
		if err != nil {
			return fmt.Errorf("failed to find available UID: %w", err)
		}
	}

	// Get or use default GID (20 = staff group on macOS)
	gid := getIntParameter(decl, "gid", 20)

	// Get or use default home directory
	home := getStringParameter(decl, "home", "/Users/"+username)

	// Get or use default shell
	shell := getStringParameter(decl, "shell", "/bin/zsh")

	// Get real name (defaults to username)
	realName := getStringParameter(decl, "fullname", username)

	// Create user record
	if err := m.dsclCreate(ctx, userPath); err != nil {
		return fmt.Errorf("failed to create user record: %w", err)
	}

	// Set user properties
	properties := map[string]string{
		"UserShell":        shell,
		"RealName":         realName,
		"UniqueID":         strconv.Itoa(uid),
		"PrimaryGroupID":   strconv.Itoa(gid),
		"NFSHomeDirectory": home,
	}

	for prop, value := range properties {
		if err := m.dsclCreateProperty(ctx, userPath, prop, value); err != nil {
			// Try to clean up on failure
			_ = m.dsclDelete(ctx, userPath)
			return fmt.Errorf("failed to set %s: %w", prop, err)
		}
	}

	// Create home directory if requested
	if getBoolParameter(decl, "createhome", true) {
		// Use createhomedir if available, otherwise mkdir
		cmd := exec.CommandContext(ctx, "createhomedir", "-c", "-u", username)
		if err := cmd.Run(); err != nil {
			// Fallback: create directory manually
			if err := exec.CommandContext(ctx, "mkdir", "-p", home).Run(); err != nil {
				return fmt.Errorf("failed to create home directory: %w", err)
			}
			// Set ownership
			if err := exec.CommandContext(ctx, "chown", fmt.Sprintf("%d:%d", uid, gid), home).Run(); err != nil {
				return fmt.Errorf("failed to set home directory ownership: %w", err)
			}
		}
	}

	// Add to additional groups
	if groups := getStringSliceParameter(decl, "groups"); groups != nil && len(groups) > 0 {
		for _, group := range groups {
			if err := m.addUserToGroupDarwin(ctx, username, group); err != nil {
				// Log but don't fail - group might not exist
				result.Comment = fmt.Sprintf("User %s created (warning: failed to add to group %s: %v)", username, group, err)
			}
		}
	}

	if result.Comment == "" {
		result.Comment = fmt.Sprintf("User %s created", username)
	}
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
		// macOS: use dscl
		return m.modifyUserDarwin(ctx, decl, result)
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

// modifyUserDarwin modifies a user on macOS using dscl
func (m *UserModule) modifyUserDarwin(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	username := decl.ID
	userPath := "/Users/" + username
	var modified bool

	// Update UID if specified
	if uid := getIntParameter(decl, "uid", -1); uid >= 0 {
		if err := m.dsclCreateProperty(ctx, userPath, "UniqueID", strconv.Itoa(uid)); err != nil {
			return fmt.Errorf("failed to set UID: %w", err)
		}
		modified = true
	}

	// Update GID if specified
	if gid := getIntParameter(decl, "gid", -1); gid >= 0 {
		if err := m.dsclCreateProperty(ctx, userPath, "PrimaryGroupID", strconv.Itoa(gid)); err != nil {
			return fmt.Errorf("failed to set GID: %w", err)
		}
		modified = true
	}

	// Update home directory if specified
	if home := getStringParameter(decl, "home", ""); home != "" {
		if err := m.dsclCreateProperty(ctx, userPath, "NFSHomeDirectory", home); err != nil {
			return fmt.Errorf("failed to set home directory: %w", err)
		}
		modified = true
	}

	// Update shell if specified
	if shell := getStringParameter(decl, "shell", ""); shell != "" {
		if err := m.dsclCreateProperty(ctx, userPath, "UserShell", shell); err != nil {
			return fmt.Errorf("failed to set shell: %w", err)
		}
		modified = true
	}

	// Update full name if specified
	if fullname := getStringParameter(decl, "fullname", ""); fullname != "" {
		if err := m.dsclCreateProperty(ctx, userPath, "RealName", fullname); err != nil {
			return fmt.Errorf("failed to set full name: %w", err)
		}
		modified = true
	}

	// Update groups if specified
	if groups := getStringSliceParameter(decl, "groups"); groups != nil {
		// Get current groups and update as needed
		for _, group := range groups {
			if err := m.addUserToGroupDarwin(ctx, username, group); err != nil {
				// Log warning but continue
				result.Comment = fmt.Sprintf("User %s modified (warning: failed to add to group %s)", username, group)
			}
		}
		modified = true
	}

	if !modified {
		result.Comment = "No changes needed"
	} else if result.Comment == "" {
		result.Comment = fmt.Sprintf("User %s modified", username)
	}
	return nil
}

// deleteUser deletes a user
func (m *UserModule) deleteUser(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	username := decl.ID

	if runtime.GOOS == "linux" || runtime.GOOS == "freebsd" {
		cmd := exec.CommandContext(ctx, "userdel", username)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to delete user: %w (output: %s)", err, string(output))
		}
	} else if runtime.GOOS == "darwin" {
		// macOS: use dscl to delete user
		userPath := "/Users/" + username
		if err := m.dsclDelete(ctx, userPath); err != nil {
			return fmt.Errorf("failed to delete user: %w", err)
		}
	} else {
		return fmt.Errorf("user deletion not supported on %s", runtime.GOOS)
	}

	result.Comment = fmt.Sprintf("User %s deleted", username)
	return nil
}

// getUserShell gets the user's shell
func (m *UserModule) getUserShell(username string) (string, error) {
	if runtime.GOOS == "darwin" {
		// macOS: use dscl to get UserShell
		userPath := "/Users/" + username
		cmd := exec.Command("dscl", ".", "-read", userPath, "UserShell")
		output, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("failed to read UserShell: %w", err)
		}
		// Output format: "UserShell: /bin/zsh"
		outputStr := strings.TrimSpace(string(output))
		parts := strings.SplitN(outputStr, ":", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[1]), nil
		}
		return "", fmt.Errorf("failed to parse dscl output")
	}

	if runtime.GOOS != "linux" && runtime.GOOS != "freebsd" {
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
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

	outputStr := strings.TrimSpace(string(output))

	// Linux format: "username : group1 group2 group3"
	// macOS format: "group1 group2 group3"
	if parts := strings.Split(outputStr, ":"); len(parts) >= 2 {
		// Linux format with colon separator
		groups := strings.Fields(parts[1])
		return groups, nil
	}

	// macOS format - just space-separated group names
	groups := strings.Fields(outputStr)
	if len(groups) == 0 {
		return nil, fmt.Errorf("no groups found for user %s", username)
	}
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

// dsclCreate creates a new directory service record
func (m *UserModule) dsclCreate(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "dscl", ".", "-create", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("dscl create failed: %w (output: %s)", err, string(output))
	}
	return nil
}

// dsclCreateProperty sets a property on a directory service record
func (m *UserModule) dsclCreateProperty(ctx context.Context, path, property, value string) error {
	cmd := exec.CommandContext(ctx, "dscl", ".", "-create", path, property, value)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("dscl create property failed: %w (output: %s)", err, string(output))
	}
	return nil
}

// dsclDelete deletes a directory service record
func (m *UserModule) dsclDelete(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "dscl", ".", "-delete", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("dscl delete failed: %w (output: %s)", err, string(output))
	}
	return nil
}

// dsclRead reads a property from a directory service record
func (m *UserModule) dsclRead(ctx context.Context, path, property string) (string, error) {
	cmd := exec.CommandContext(ctx, "dscl", ".", "-read", path, property)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("dscl read failed: %w", err)
	}
	// Output format: "Property: value"
	outputStr := strings.TrimSpace(string(output))
	parts := strings.SplitN(outputStr, ":", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1]), nil
	}
	return "", fmt.Errorf("failed to parse dscl output: %s", outputStr)
}

// findNextAvailableUID finds the next available UID on macOS
func (m *UserModule) findNextAvailableUID(ctx context.Context) (int, error) {
	// List all users and find the highest UID >= 501
	cmd := exec.CommandContext(ctx, "dscl", ".", "-list", "/Users", "UniqueID")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to list users: %w", err)
	}

	maxUID := 500 // Start checking from 501
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			uid, err := strconv.Atoi(fields[len(fields)-1])
			if err == nil && uid >= 501 && uid > maxUID {
				maxUID = uid
			}
		}
	}

	return maxUID + 1, nil
}

// addUserToGroupDarwin adds a user to a group on macOS
func (m *UserModule) addUserToGroupDarwin(ctx context.Context, username, groupname string) error {
	// Check if group exists
	groupPath := "/Groups/" + groupname
	cmd := exec.CommandContext(ctx, "dscl", ".", "-read", groupPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("group %s does not exist", groupname)
	}

	// Add user to group's GroupMembership
	cmd = exec.CommandContext(ctx, "dscl", ".", "-append", groupPath, "GroupMembership", username)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add user to group: %w (output: %s)", err, string(output))
	}
	return nil
}

func init() {
	RegisterModule(NewUserModule())
}
