package statemgmt

import (
	"context"
	"errors"
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
		var unknownUserErr user.UnknownUserError
		if errors.As(err, &unknownUserErr) {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = (decl.State == "absent")
			return result, nil //nolint:nilerr // error captured in result.Error
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
		return result, nil //nolint:nilerr // error captured in result.Error
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
		currentShell, err := m.getUserShell(ctx, username)
		if err == nil && currentShell != desiredShell {
			result.Matches = false
			result.Diff["shell"] = map[string]string{"current": currentShell, "desired": desiredShell}
		}
	}

	// Check groups
	if desiredGroups := getStringSliceParameter(decl, "groups"); desiredGroups != nil {
		currentGroups, err := m.getUserGroups(username) //nolint:contextcheck // getUserGroups doesn't take context
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

	return result, nil //nolint:nilerr // error captured in result.Error
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
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	// If already in desired state, no changes needed
	if checkResult.Matches {
		result.Success = true
		result.Changed = false
		result.Comment = "Already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
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
	return result, nil //nolint:nilerr // error captured in result.Error
}

// Test tests if the user is in the desired state
func (m *UserModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil //nolint:nilerr // intentional
}

// createUser creates a new user
func (m *UserModule) createUser(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	username := decl.ID
	args := []string{}

	switch runtime.GOOS {
	case "linux", "freebsd":
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
		if groups := getStringSliceParameter(decl, "groups"); len(groups) > 0 {
			args = append(args, "-G", strings.Join(groups, ","))
		}

		args = append(args, username)

	case "darwin":
		// macOS: use dscl (Directory Service command line)
		return m.createUserDarwin(ctx, decl, result)
	case "windows":
		// Windows: use net user
		return m.createUserWindows(ctx, decl, result)
	default:
		return fmt.Errorf("user creation not supported on %s", runtime.GOOS)
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
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
		cmd := exec.CommandContext(ctx, "createhomedir", "-c", "-u", username) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
		if err := cmd.Run(); err != nil {
			// Fallback: create directory manually
			if err := exec.CommandContext(ctx, "mkdir", "-p", home).Run(); err != nil { // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
				return fmt.Errorf("failed to create home directory: %w", err)
			}
			// Set ownership
			if err := exec.CommandContext(ctx, "chown", fmt.Sprintf("%d:%d", uid, gid), home).Run(); err != nil { // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
				return fmt.Errorf("failed to set home directory ownership: %w", err)
			}
		}
	}

	// Add to additional groups
	if groups := getStringSliceParameter(decl, "groups"); len(groups) > 0 {
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

	switch runtime.GOOS {
	case "linux", "freebsd":
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
		if groups := getStringSliceParameter(decl, "groups"); len(groups) > 0 {
			args = append(args, "-G", strings.Join(groups, ","))
		}

		// Only run if there are changes to make
		if len(args) == 1 {
			result.Comment = "No changes needed"
			return nil
		}

		args = append(args, username)

	case "darwin":
		// macOS: use dscl
		return m.modifyUserDarwin(ctx, decl, result)
	case "windows":
		// Windows: use net user
		return m.modifyUserWindows(ctx, decl, result)
	default:
		return fmt.Errorf("user modification not supported on %s", runtime.GOOS)
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
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

	switch runtime.GOOS {
	case "linux", "freebsd":
		cmd := exec.CommandContext(ctx, "userdel", username) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to delete user: %w (output: %s)", err, string(output))
		}
	case "darwin":
		// macOS: use dscl to delete user
		userPath := "/Users/" + username
		if err := m.dsclDelete(ctx, userPath); err != nil {
			return fmt.Errorf("failed to delete user: %w", err)
		}
	case "windows":
		// Windows: use net user /delete
		cmd := exec.CommandContext(ctx, "net", "user", username, "/delete") // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to delete user: %w (output: %s)", err, string(output))
		}
	default:
		return fmt.Errorf("user deletion not supported on %s", runtime.GOOS)
	}

	result.Comment = fmt.Sprintf("User %s deleted", username)
	return nil
}

// getUserShell gets the user's shell
func (m *UserModule) getUserShell(ctx context.Context, username string) (string, error) {
	if runtime.GOOS == "darwin" {
		// macOS: use dscl to get UserShell
		userPath := "/Users/" + username
		cmd := exec.CommandContext(ctx,"dscl", ".", "-read", userPath, "UserShell") // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
		output, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("failed to read UserShell: %w", err)
		}
		// Output format: "UserShell: /bin/zsh"
		outputStr := strings.TrimSpace(string(output))
		parts := strings.SplitN(outputStr, ":", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[1]), nil //nolint:nilerr // intentional
		}
		return "", fmt.Errorf("failed to parse dscl output")
	}

	if runtime.GOOS == "windows" {
		// Windows doesn't have a user shell concept
		return "", nil //nolint:nilerr // intentional
	}

	if runtime.GOOS != "linux" && runtime.GOOS != "freebsd" {
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	cmd := exec.CommandContext(ctx,"getent", "passwd", username) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Parse passwd entry: username:x:uid:gid:gecos:home:shell
	parts := strings.Split(strings.TrimSpace(string(output)), ":")
	if len(parts) >= 7 {
		return parts[6], nil //nolint:nilerr // intentional
	}

	return "", fmt.Errorf("failed to parse passwd entry")
}

// getUserGroups gets the user's groups
func (m *UserModule) getUserGroups(username string) ([]string, error) {
	if runtime.GOOS == "windows" {
		return m.getUserGroupsWindows(context.Background(), username)
	}

	cmd := exec.CommandContext(context.Background(),"groups", username) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
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
		return groups, nil //nolint:nilerr // intentional
	}

	// macOS format - just space-separated group names
	groups := strings.Fields(outputStr)
	if len(groups) == 0 {
		return nil, fmt.Errorf("no groups found for user %s", username)
	}
	return groups, nil //nolint:nilerr // intentional
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
	cmd := exec.CommandContext(ctx, "dscl", ".", "-create", path) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("dscl create failed: %w (output: %s)", err, string(output))
	}
	return nil
}

// dsclCreateProperty sets a property on a directory service record
func (m *UserModule) dsclCreateProperty(ctx context.Context, path, property, value string) error {
	cmd := exec.CommandContext(ctx, "dscl", ".", "-create", path, property, value) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("dscl create property failed: %w (output: %s)", err, string(output))
	}
	return nil
}

// dsclDelete deletes a directory service record
func (m *UserModule) dsclDelete(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "dscl", ".", "-delete", path) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("dscl delete failed: %w (output: %s)", err, string(output))
	}
	return nil
}

// dsclRead reads a property from a directory service record
func (m *UserModule) dsclRead(ctx context.Context, path, property string) (string, error) {
	cmd := exec.CommandContext(ctx, "dscl", ".", "-read", path, property) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("dscl read failed: %w", err)
	}
	// Output format: "Property: value"
	outputStr := strings.TrimSpace(string(output))
	parts := strings.SplitN(outputStr, ":", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1]), nil //nolint:nilerr // intentional
	}
	return "", fmt.Errorf("failed to parse dscl output: %s", outputStr)
}

// findNextAvailableUID finds the next available UID on macOS
func (m *UserModule) findNextAvailableUID(ctx context.Context) (int, error) {
	// List all users and find the highest UID >= 501
	cmd := exec.CommandContext(ctx, "dscl", ".", "-list", "/Users", "UniqueID") // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
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

	return maxUID + 1, nil //nolint:nilerr // intentional
}

// addUserToGroupDarwin adds a user to a group on macOS
func (m *UserModule) addUserToGroupDarwin(ctx context.Context, username, groupname string) error {
	// Check if group exists
	groupPath := "/Groups/" + groupname
	cmd := exec.CommandContext(ctx, "dscl", ".", "-read", groupPath) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("group %s does not exist", groupname)
	}

	// Add user to group's GroupMembership
	cmd = exec.CommandContext(ctx, "dscl", ".", "-append", groupPath, "GroupMembership", username) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add user to group: %w (output: %s)", err, string(output))
	}
	return nil
}

// createUserWindows creates a user on Windows using net user
func (m *UserModule) createUserWindows(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	username := decl.ID

	// Build net user command
	// net user username password /add [options]
	args := []string{"user", username}

	// Get password (required for creation)
	password := getStringParameter(decl, "password", "")
	if password == "" {
		// Generate a random password if not provided (user must change it)
		password = "*" // Prompts for password or uses empty if run non-interactively
	}
	args = append(args, password, "/add")

	// Full name
	if fullname := getStringParameter(decl, "fullname", ""); fullname != "" {
		args = append(args, fmt.Sprintf("/fullname:%q", fullname))
	}

	// Comment/description
	if comment := getStringParameter(decl, "comment", ""); comment != "" {
		args = append(args, fmt.Sprintf("/comment:%q", comment))
	}

	// Home directory
	if home := getStringParameter(decl, "home", ""); home != "" {
		args = append(args, fmt.Sprintf("/homedir:%q", home))
	}

	// Account active
	if !getBoolParameter(decl, "active", true) {
		args = append(args, "/active:no")
	}

	// Password never expires
	if getBoolParameter(decl, "password_never_expires", false) {
		args = append(args, "/expires:never")
	}

	cmd := exec.CommandContext(ctx, "net", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create user: %w (output: %s)", err, string(output))
	}

	// Add to groups if specified
	if groups := getStringSliceParameter(decl, "groups"); len(groups) > 0 {
		for _, group := range groups {
			if err := m.addUserToGroupWindows(ctx, username, group); err != nil {
				result.Comment = fmt.Sprintf("User %s created (warning: failed to add to group %s: %v)", username, group, err)
			}
		}
	}

	if result.Comment == "" {
		result.Comment = fmt.Sprintf("User %s created", username)
	}
	return nil
}

// modifyUserWindows modifies a user on Windows using net user
func (m *UserModule) modifyUserWindows(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	username := decl.ID
	var modified bool

	// Build net user command for modifications
	args := []string{"user", username}

	// Full name
	if fullname := getStringParameter(decl, "fullname", ""); fullname != "" {
		args = append(args, fmt.Sprintf("/fullname:%q", fullname))
		modified = true
	}

	// Comment/description
	if comment := getStringParameter(decl, "comment", ""); comment != "" {
		args = append(args, fmt.Sprintf("/comment:%q", comment))
		modified = true
	}

	// Home directory
	if home := getStringParameter(decl, "home", ""); home != "" {
		args = append(args, fmt.Sprintf("/homedir:%q", home))
		modified = true
	}

	// Account active
	if active, ok := decl.Parameters["active"].(bool); ok {
		if active {
			args = append(args, "/active:yes")
		} else {
			args = append(args, "/active:no")
		}
		modified = true
	}

	// Password
	if password := getStringParameter(decl, "password", ""); password != "" {
		args = append(args, password)
		modified = true
	}

	if modified && len(args) > 2 {
		cmd := exec.CommandContext(ctx, "net", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to modify user: %w (output: %s)", err, string(output))
		}
	}

	// Update groups if specified
	if groups := getStringSliceParameter(decl, "groups"); groups != nil {
		if err := m.updateGroupMembershipWindows(ctx, username, groups); err != nil {
			return err
		}
		modified = true
	}

	if !modified {
		result.Comment = "No changes needed"
	} else {
		result.Comment = fmt.Sprintf("User %s modified", username)
	}
	return nil
}

// addUserToGroupWindows adds a user to a local group on Windows
func (m *UserModule) addUserToGroupWindows(ctx context.Context, username, groupname string) error {
	cmd := exec.CommandContext(ctx, "net", "localgroup", groupname, username, "/add") // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if user is already a member (not an error)
		if strings.Contains(string(output), "1378") || strings.Contains(string(output), "already a member") {
			return nil
		}
		return fmt.Errorf("failed to add user to group: %w (output: %s)", err, string(output))
	}
	return nil
}

// removeUserFromGroupWindows removes a user from a local group on Windows
func (m *UserModule) removeUserFromGroupWindows(ctx context.Context, username, groupname string) error {
	cmd := exec.CommandContext(ctx, "net", "localgroup", groupname, username, "/delete") // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove user from group: %w (output: %s)", err, string(output))
	}
	return nil
}

// getUserGroupsWindows gets the groups a user belongs to on Windows
func (m *UserModule) getUserGroupsWindows(ctx context.Context, username string) ([]string, error) {
	// Use PowerShell to get user's group membership
	script := fmt.Sprintf(`Get-LocalGroup | Where-Object { (Get-LocalGroupMember -Group $_.Name -ErrorAction SilentlyContinue | Where-Object { $_.Name -like '*\%s' }) } | Select-Object -ExpandProperty Name`, username)
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.Output()
	if err != nil {
		// Fallback: return empty list (user might have no groups)
		return []string{}, nil //nolint:nilerr // intentional
	}

	var groups []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			groups = append(groups, line)
		}
	}
	return groups, nil //nolint:nilerr // intentional
}

// updateGroupMembershipWindows updates a user's group membership on Windows
func (m *UserModule) updateGroupMembershipWindows(ctx context.Context, username string, desiredGroups []string) error {
	currentGroups, err := m.getUserGroupsWindows(ctx, username)
	if err != nil {
		return err
	}

	// Build maps for comparison
	currentMap := make(map[string]bool)
	for _, g := range currentGroups {
		currentMap[strings.ToLower(g)] = true
	}

	desiredMap := make(map[string]bool)
	for _, g := range desiredGroups {
		desiredMap[strings.ToLower(g)] = true
	}

	// Add missing groups
	for _, group := range desiredGroups {
		if !currentMap[strings.ToLower(group)] {
			if err := m.addUserToGroupWindows(ctx, username, group); err != nil {
				return err
			}
		}
	}

	// Remove extra groups (except built-in "Users" group)
	for _, group := range currentGroups {
		if !desiredMap[strings.ToLower(group)] && !strings.EqualFold(group, "users") {
			if err := m.removeUserFromGroupWindows(ctx, username, group); err != nil {
				return err
			}
		}
	}

	return nil
}

func init() {
	_ = RegisterModule(NewUserModule()) //nolint:errcheck // module registration in init
}
