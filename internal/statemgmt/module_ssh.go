package statemgmt

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// AuthorizedKeysModule manages SSH authorized_keys entries
type AuthorizedKeysModule struct {
	*BaseModule
}

// NewAuthorizedKeysModule creates a new authorized_keys module
func NewAuthorizedKeysModule() *AuthorizedKeysModule {
	return &AuthorizedKeysModule{
		BaseModule: NewBaseModule("authorized_keys", []string{"present", "absent"}),
	}
}

// Check checks if the SSH key is in authorized_keys
func (m *AuthorizedKeysModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	user := getStringParameter(decl, "user", "")
	key := getStringParameter(decl, "key", "")
	keyType := getStringParameter(decl, "key_type", "ssh-rsa")
	comment := getStringParameter(decl, "comment", "")

	if user == "" {
		return nil, fmt.Errorf("user parameter is required")
	}
	if key == "" {
		return nil, fmt.Errorf("key parameter is required")
	}

	authKeysPath, err := m.getAuthorizedKeysPath(ctx, user)
	if err != nil {
		return nil, err
	}

	present, currentComment, err := m.keyExists(authKeysPath, keyType, key)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	result := &ModuleCheckResult{
		Present:      present,
		CurrentState: "absent",
		Matches:      false,
		Diff:         make(map[string]interface{}),
		Metadata: map[string]interface{}{
			"user":          user,
			"path":          authKeysPath,
			"key_type":      keyType,
			"comment":       comment,
			"found_comment": currentComment,
		},
	}

	if present {
		result.CurrentState = "present"
	}

	switch decl.State {
	case "present":
		result.Matches = present
		if !present {
			result.Diff["key"] = map[string]interface{}{
				"old": nil,
				"new": fmt.Sprintf("%s %s...", keyType, key[:min(20, len(key))]),
			}
		}
	case "absent":
		result.Matches = !present
		if present {
			result.Diff["key"] = map[string]interface{}{
				"old": fmt.Sprintf("%s %s...", keyType, key[:min(20, len(key))]),
				"new": nil,
			}
		}
	}

	return result, nil
}

// Apply adds or removes the SSH key
func (m *AuthorizedKeysModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Changed: false,
			Comment: fmt.Sprintf("Check failed: %v", err),
		}, nil
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("SSH key already %s", decl.State),
		}, nil
	}

	user := getStringParameter(decl, "user", "")
	key := getStringParameter(decl, "key", "")
	keyType := getStringParameter(decl, "key_type", "ssh-rsa")
	keyComment := getStringParameter(decl, "comment", "")
	options := getStringParameter(decl, "options", "")

	authKeysPath := checkResult.Metadata["path"].(string)

	switch decl.State {
	case "present":
		if err := m.addKey(ctx, authKeysPath, user, keyType, key, keyComment, options); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to add key: %v", err),
			}, nil
		}
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: "SSH key added to authorized_keys",
		}, nil

	case "absent":
		if err := m.removeKey(authKeysPath, keyType, key); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to remove key: %v", err),
			}, nil
		}
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: "SSH key removed from authorized_keys",
		}, nil
	}

	return nil, fmt.Errorf("unknown state: %s", decl.State)
}

// Test verifies the key state
func (m *AuthorizedKeysModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

func (m *AuthorizedKeysModule) getAuthorizedKeysPath(ctx context.Context, user string) (string, error) {
	// Get user's home directory
	var homeDir string

	if runtime.GOOS == "windows" {
		// Windows doesn't have traditional authorized_keys
		return "", fmt.Errorf("authorized_keys not supported on Windows")
	}

	if user == "root" {
		homeDir = "/root"
	} else {
		// Try to get from passwd
		cmd := exec.CommandContext(ctx, "getent", "passwd", user)
		output, err := cmd.Output()
		if err != nil {
			// Fallback to /home/user
			homeDir = filepath.Join("/home", user)
		} else {
			fields := strings.Split(strings.TrimSpace(string(output)), ":")
			if len(fields) >= 6 {
				homeDir = fields[5]
			} else {
				homeDir = filepath.Join("/home", user)
			}
		}
	}

	return filepath.Join(homeDir, ".ssh", "authorized_keys"), nil
}

func (m *AuthorizedKeysModule) keyExists(path, keyType, key string) (exists bool, comment string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return false, "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 2 {
			// Check for options prefix
			idx := 0
			if !strings.HasPrefix(fields[0], "ssh-") && !strings.HasPrefix(fields[0], "ecdsa-") && !strings.HasPrefix(fields[0], "sk-") {
				idx = 1
			}
			if idx < len(fields)-1 && fields[idx] == keyType && fields[idx+1] == key {
				comment := ""
				if len(fields) > idx+2 {
					comment = strings.Join(fields[idx+2:], " ")
				}
				return true, comment, nil
			}
		}
	}

	return false, "", scanner.Err()
}

func (m *AuthorizedKeysModule) addKey(ctx context.Context, path, user, keyType, key, comment, options string) error {
	// Ensure .ssh directory exists
	sshDir := filepath.Dir(path)
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return fmt.Errorf("failed to create .ssh directory: %w", err)
	}

	// Set ownership if running as root
	if os.Getuid() == 0 {
		cmd := exec.CommandContext(ctx, "chown", user, sshDir)
		cmd.Run()
	}

	// Build key line
	var keyLine string
	if options != "" {
		keyLine = fmt.Sprintf("%s %s %s", options, keyType, key)
	} else {
		keyLine = fmt.Sprintf("%s %s", keyType, key)
	}
	if comment != "" {
		keyLine += " " + comment
	}
	keyLine += "\n"

	// Append to file
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open authorized_keys: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(keyLine); err != nil {
		return fmt.Errorf("failed to write key: %w", err)
	}

	// Set ownership if running as root
	if os.Getuid() == 0 {
		cmd := exec.CommandContext(ctx, "chown", user, path)
		cmd.Run()
	}

	return nil
}

func (m *AuthorizedKeysModule) removeKey(path, keyType, key string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Nothing to remove
		}
		return err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Keep empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			lines = append(lines, line)
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) >= 2 {
			idx := 0
			if !strings.HasPrefix(fields[0], "ssh-") && !strings.HasPrefix(fields[0], "ecdsa-") && !strings.HasPrefix(fields[0], "sk-") {
				idx = 1
			}
			if idx < len(fields)-1 && fields[idx] == keyType && fields[idx+1] == key {
				continue // Skip this key
			}
		}
		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Write back
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

// KnownHostsModule manages SSH known_hosts entries
type KnownHostsModule struct {
	*BaseModule
}

// NewKnownHostsModule creates a new known_hosts module
func NewKnownHostsModule() *KnownHostsModule {
	return &KnownHostsModule{
		BaseModule: NewBaseModule("known_hosts", []string{"present", "absent"}),
	}
}

// Check checks if the host key is in known_hosts
func (m *KnownHostsModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	host := getStringParameter(decl, "host", "")
	key := getStringParameter(decl, "key", "")
	keyType := getStringParameter(decl, "key_type", "")
	user := getStringParameter(decl, "user", "")
	hashKnownHosts := getBoolParameter(decl, "hash_known_hosts", false)
	path := getStringParameter(decl, "path", "")

	if host == "" {
		return nil, fmt.Errorf("host parameter is required")
	}

	if path == "" {
		if user == "" {
			path = "/etc/ssh/ssh_known_hosts"
		} else {
			khPath, err := m.getKnownHostsPath(ctx, user)
			if err != nil {
				return nil, err
			}
			path = khPath
		}
	}

	present, foundKey, foundType, err := m.hostExists(path, host)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	result := &ModuleCheckResult{
		Present:      present,
		CurrentState: "absent",
		Matches:      false,
		Diff:         make(map[string]interface{}),
		Metadata: map[string]interface{}{
			"host":             host,
			"path":             path,
			"hash_known_hosts": hashKnownHosts,
			"found_key":        foundKey,
			"found_type":       foundType,
		},
	}

	if present {
		result.CurrentState = "present"
	}

	switch decl.State {
	case "present":
		if key == "" {
			// Just check if any key exists for host
			result.Matches = present
		} else {
			// Check if specific key matches
			result.Matches = present && foundKey == key && (keyType == "" || foundType == keyType)
		}
		if !result.Matches {
			result.Diff["host_key"] = map[string]interface{}{
				"old": foundKey,
				"new": key,
			}
		}
	case "absent":
		result.Matches = !present
		if present {
			result.Diff["host_key"] = map[string]interface{}{
				"old": foundKey,
				"new": nil,
			}
		}
	}

	return result, nil
}

// Apply adds or removes the host key
func (m *KnownHostsModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Changed: false,
			Comment: fmt.Sprintf("Check failed: %v", err),
		}, nil
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("Host key already %s", decl.State),
		}, nil
	}

	host := getStringParameter(decl, "host", "")
	key := getStringParameter(decl, "key", "")
	keyType := getStringParameter(decl, "key_type", "ssh-rsa")
	hashKnownHosts := getBoolParameter(decl, "hash_known_hosts", false)
	path := checkResult.Metadata["path"].(string)

	switch decl.State {
	case "present":
		// If no key provided, scan for it
		if key == "" {
			scannedKey, scannedType, err := m.scanHostKey(ctx, host, keyType)
			if err != nil {
				return &StateResult{
					StateID: decl.ID,
					Module:  m.Name(),
					Success: false,
					Changed: false,
					Comment: fmt.Sprintf("Failed to scan host key: %v", err),
				}, nil
			}
			key = scannedKey
			keyType = scannedType
		}

		if err := m.addHostKey(ctx, path, host, keyType, key, hashKnownHosts); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to add host key: %v", err),
			}, nil
		}
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Host key for %s added", host),
		}, nil

	case "absent":
		if err := m.removeHostKey(path, host); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to remove host key: %v", err),
			}, nil
		}
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Host key for %s removed", host),
		}, nil
	}

	return nil, fmt.Errorf("unknown state: %s", decl.State)
}

// Test verifies the host key state
func (m *KnownHostsModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

func (m *KnownHostsModule) getKnownHostsPath(ctx context.Context, user string) (string, error) {
	if runtime.GOOS == "windows" {
		return "", fmt.Errorf("known_hosts not supported on Windows")
	}

	var homeDir string
	if user == "root" {
		homeDir = "/root"
	} else {
		cmd := exec.CommandContext(ctx, "getent", "passwd", user)
		output, err := cmd.Output()
		if err != nil {
			homeDir = filepath.Join("/home", user)
		} else {
			fields := strings.Split(strings.TrimSpace(string(output)), ":")
			if len(fields) >= 6 {
				homeDir = fields[5]
			} else {
				homeDir = filepath.Join("/home", user)
			}
		}
	}

	return filepath.Join(homeDir, ".ssh", "known_hosts"), nil
}

func (m *KnownHostsModule) hostExists(path, host string) (exists bool, keyData, keyType string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return false, "", "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 3 {
			hosts := strings.Split(fields[0], ",")
			for _, h := range hosts {
				if h == host || h == "["+host+"]" || strings.HasPrefix(h, host+":") {
					return true, fields[2], fields[1], nil
				}
			}
		}
	}

	return false, "", "", scanner.Err()
}

func (m *KnownHostsModule) scanHostKey(ctx context.Context, host, keyType string) (keyData, actualKeyType string, err error) {
	args := []string{"-t", keyType, host}
	cmd := exec.CommandContext(ctx, "ssh-keyscan", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("ssh-keyscan failed: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			return fields[2], fields[1], nil
		}
	}

	return "", "", fmt.Errorf("no key found for host %s", host)
}

func (m *KnownHostsModule) addHostKey(ctx context.Context, path, host, keyType, key string, hashHost bool) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	// Remove existing entry first
	_ = m.removeHostKey(path, host) //nolint:errcheck // best-effort cleanup before adding new key

	hostEntry := host
	if hashHost {
		// Use ssh-keygen to hash
		cmd := exec.CommandContext(ctx, "ssh-keygen", "-H", "-f", path)
		cmd.Run()
	}

	line := fmt.Sprintf("%s %s %s\n", hostEntry, keyType, key)

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // G302: SSH known_hosts files are typically world-readable
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(line)
	return err
}

func (m *KnownHostsModule) removeHostKey(path, host string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			lines = append(lines, line)
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) >= 1 {
			hosts := strings.Split(fields[0], ",")
			skip := false
			for _, h := range hosts {
				if h == host || h == "["+host+"]" || strings.HasPrefix(h, host+":") {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
		}
		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	//nolint:gosec // G306: known_hosts file needs to be readable by SSH clients
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// SSHDConfigModule manages sshd_config settings
type SSHDConfigModule struct {
	*BaseModule
}

// NewSSHDConfigModule creates a new sshd_config module
func NewSSHDConfigModule() *SSHDConfigModule {
	return &SSHDConfigModule{
		BaseModule: NewBaseModule("sshd_config", []string{"present", "absent"}),
	}
}

// Check checks if the sshd_config setting exists
func (m *SSHDConfigModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("sshd_config not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	value := getStringParameter(decl, "value", "")
	path := getStringParameter(decl, "path", "/etc/ssh/sshd_config")

	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	currentValue, exists, err := m.getConfigValue(path, name)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	result := &ModuleCheckResult{
		Present:      exists,
		CurrentState: "absent",
		Matches:      false,
		Diff:         make(map[string]interface{}),
		Metadata: map[string]interface{}{
			"name":          name,
			"path":          path,
			"current_value": currentValue,
		},
	}

	if exists {
		result.CurrentState = "present"
	}

	switch decl.State {
	case "present":
		if value == "" {
			result.Matches = exists
		} else {
			result.Matches = exists && currentValue == value
		}
		if !result.Matches {
			result.Diff[name] = map[string]interface{}{
				"old": currentValue,
				"new": value,
			}
		}
	case "absent":
		result.Matches = !exists
		if exists {
			result.Diff[name] = map[string]interface{}{
				"old": currentValue,
				"new": nil,
			}
		}
	}

	return result, nil
}

// Apply sets or removes the sshd_config setting
func (m *SSHDConfigModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Changed: false,
			Comment: fmt.Sprintf("Check failed: %v", err),
		}, nil
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: "sshd_config setting already correct",
		}, nil
	}

	name := getStringParameter(decl, "name", "")
	value := getStringParameter(decl, "value", "")
	path := getStringParameter(decl, "path", "/etc/ssh/sshd_config")
	backup := getBoolParameter(decl, "backup", true)

	switch decl.State {
	case "present":
		if err := m.setConfigValue(path, name, value, backup); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to set config: %v", err),
			}, nil
		}
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Set %s = %s", name, value),
		}, nil

	case "absent":
		if err := m.removeConfigValue(path, name, backup); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to remove config: %v", err),
			}, nil
		}
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Removed %s from config", name),
		}, nil
	}

	return nil, fmt.Errorf("unknown state: %s", decl.State)
}

// Test verifies the config setting
func (m *SSHDConfigModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

func (m *SSHDConfigModule) getConfigValue(path, name string) (value string, exists bool, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer file.Close()

	// Case-insensitive matching for sshd_config
	pattern := regexp.MustCompile(`(?i)^\s*` + regexp.QuoteMeta(name) + `\s+(.+)$`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		if matches := pattern.FindStringSubmatch(line); matches != nil {
			return strings.TrimSpace(matches[1]), true, nil
		}

		// Also check for Match blocks - simplified handling
		fields := strings.Fields(trimmed)
		if len(fields) >= 2 && strings.EqualFold(fields[0], name) {
			return strings.Join(fields[1:], " "), true, nil
		}
	}

	return "", false, scanner.Err()
}

func (m *SSHDConfigModule) setConfigValue(path, name, value string, backup bool) error {
	// Read current file
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Create backup if requested
	if backup && len(content) > 0 {
		backupPath := path + ".bak"
		if err := os.WriteFile(backupPath, content, 0o600); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	lines := strings.Split(string(content), "\n")
	found := false
	var newLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if this is the line we're looking for
		if !strings.HasPrefix(trimmed, "#") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 1 && strings.EqualFold(fields[0], name) {
				// Replace this line
				newLines = append(newLines, fmt.Sprintf("%s %s", name, value))
				found = true
				continue
			}
		}
		newLines = append(newLines, line)
	}

	// If not found, append at the end
	if !found {
		newLines = append(newLines, fmt.Sprintf("%s %s", name, value))
	}

	return os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0o600)
}

func (m *SSHDConfigModule) removeConfigValue(path, name string, backup bool) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if backup {
		backupPath := path + ".bak"
		if err := os.WriteFile(backupPath, content, 0o600); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !strings.HasPrefix(trimmed, "#") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 1 && strings.EqualFold(fields[0], name) {
				// Skip this line (comment it out instead of removing)
				newLines = append(newLines, "#"+line)
				continue
			}
		}
		newLines = append(newLines, line)
	}

	return os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0o600)
}

