// Package statemgmt provides state management modules for system configuration.
package statemgmt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// =============================================================================
// Logrotate Module
// =============================================================================

// LogrotateModule manages logrotate configuration files.
type LogrotateModule struct {
	*BaseModule
}

// NewLogrotateModule creates a new logrotate module.
func NewLogrotateModule() *LogrotateModule {
	return &LogrotateModule{
		BaseModule: NewBaseModule("logrotate", []string{"present", "absent"}),
	}
}

// Check verifies the current state of a logrotate configuration.
func (m *LogrotateModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("logrotate: name parameter is required")
	}

	configPath := filepath.Join("/etc", "logrotate.d", name)
	state := decl.State
	if state == "" {
		state = "present"
	}

	result.Metadata["name"] = name
	result.Metadata["config_path"] = configPath

	info, err := os.Stat(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = (state == "absent")
			if state == "present" {
				result.Diff["state"] = map[string]string{"current": "absent", "desired": "present"}
			}
			return result, nil
		}
		return nil, fmt.Errorf("failed to stat logrotate config: %w", err)
	}

	result.Present = true
	result.Metadata["mode"] = info.Mode().Perm().String()

	if state == "absent" {
		result.CurrentState = "present"
		result.Matches = false
		result.Diff["state"] = map[string]string{"current": "present", "desired": "absent"}
		return result, nil
	}

	// Check if content matches
	path := getStringParameter(decl, "path", "")
	if path != "" {
		content, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read logrotate config: %w", err)
		}

		expectedContent := m.buildLogrotateConfig(decl)
		if string(content) != expectedContent {
			result.CurrentState = "different"
			result.Matches = false
			result.Diff["content"] = map[string]string{"current": "different", "desired": "updated"}
		} else {
			result.CurrentState = "present"
			result.Matches = true
		}
	} else {
		result.CurrentState = "present"
		result.Matches = true
	}

	return result, nil
}

// Apply applies the logrotate configuration.
func (m *LogrotateModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StartTime: startTime,
		Success:   false,
		Changed:   false,
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		err := fmt.Errorf("logrotate: name parameter is required")
		result.Comment = err.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, err
	}

	configPath := filepath.Join("/etc", "logrotate.d", name)
	state := decl.State
	if state == "" {
		state = "present"
	}

	if state == "absent" {
		if _, err := os.Stat(configPath); err == nil {
			if err := os.Remove(configPath); err != nil {
				result.Comment = fmt.Sprintf("failed to remove logrotate config: %v", err)
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(startTime)
				return result, err
			}
			result.Changed = true
		}
		result.Success = true
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Build and write configuration
	content := m.buildLogrotateConfig(decl)

	// Check if file exists and content matches
	existingContent, err := os.ReadFile(configPath)
	if err == nil && string(existingContent) == content {
		result.Success = true
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	//nolint:gosec // G306: logrotate config files need to be readable by logrotate
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		result.Comment = fmt.Sprintf("failed to write logrotate config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, err
	}

	result.Success = true
	result.Changed = true
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// Test tests if the logrotate configuration would be applied successfully.
func (m *LogrotateModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return false, fmt.Errorf("logrotate: name parameter is required")
	}

	path := getStringParameter(decl, "path", "")
	if path == "" && decl.State == "present" {
		return false, fmt.Errorf("logrotate: path parameter is required for present state")
	}

	return true, nil
}

func (m *LogrotateModule) buildLogrotateConfig(decl *StateDeclaration) string {
	path := getStringParameter(decl, "path", "")
	frequency := getStringParameter(decl, "frequency", "weekly")
	rotate := getIntParameter(decl, "rotate", 4)
	compress := getBoolParameter(decl, "compress", true)
	delaycompress := getBoolParameter(decl, "delaycompress", true)
	missingok := getBoolParameter(decl, "missingok", true)
	notifempty := getBoolParameter(decl, "notifempty", true)
	create := getStringParameter(decl, "create", "")
	postrotate := getStringParameter(decl, "postrotate", "")
	prerotate := getStringParameter(decl, "prerotate", "")
	sharedscripts := getBoolParameter(decl, "sharedscripts", false)
	copytruncate := getBoolParameter(decl, "copytruncate", false)
	size := getStringParameter(decl, "size", "")
	maxsize := getStringParameter(decl, "maxsize", "")
	minsize := getStringParameter(decl, "minsize", "")

	var lines []string
	lines = append(lines, path+" {", fmt.Sprintf("    %s", frequency), fmt.Sprintf("    rotate %d", rotate))

	if compress {
		lines = append(lines, "    compress")
	}
	if delaycompress {
		lines = append(lines, "    delaycompress")
	}
	if missingok {
		lines = append(lines, "    missingok")
	}
	if notifempty {
		lines = append(lines, "    notifempty")
	}
	if copytruncate {
		lines = append(lines, "    copytruncate")
	}
	if sharedscripts {
		lines = append(lines, "    sharedscripts")
	}
	if create != "" {
		lines = append(lines, fmt.Sprintf("    create %s", create))
	}
	if size != "" {
		lines = append(lines, fmt.Sprintf("    size %s", size))
	}
	if maxsize != "" {
		lines = append(lines, fmt.Sprintf("    maxsize %s", maxsize))
	}
	if minsize != "" {
		lines = append(lines, fmt.Sprintf("    minsize %s", minsize))
	}
	if prerotate != "" {
		lines = append(lines, "    prerotate", "        "+prerotate, "    endscript")
	}
	if postrotate != "" {
		lines = append(lines, "    postrotate", "        "+postrotate, "    endscript")
	}
	lines = append(lines, "}")

	return strings.Join(lines, "\n") + "\n"
}

// =============================================================================
// Sudoers Module
// =============================================================================

// SudoersModule manages sudoers configuration.
type SudoersModule struct {
	*BaseModule
}

// NewSudoersModule creates a new sudoers module.
func NewSudoersModule() *SudoersModule {
	return &SudoersModule{
		BaseModule: NewBaseModule("sudoers", []string{"present", "absent"}),
	}
}

// Check verifies the current state of a sudoers entry.
func (m *SudoersModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("sudoers: name parameter is required")
	}

	configPath := filepath.Join("/etc", "sudoers.d", name)
	state := decl.State
	if state == "" {
		state = "present"
	}

	result.Metadata["name"] = name
	result.Metadata["config_path"] = configPath

	info, err := os.Stat(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = (state == "absent")
			if state == "present" {
				result.Diff["state"] = map[string]string{"current": "absent", "desired": "present"}
			}
			return result, nil
		}
		return nil, fmt.Errorf("failed to stat sudoers config: %w", err)
	}

	result.Present = true
	result.Metadata["mode"] = info.Mode().Perm().String()

	if state == "absent" {
		result.CurrentState = "present"
		result.Matches = false
		result.Diff["state"] = map[string]string{"current": "present", "desired": "absent"}
		return result, nil
	}

	// Check if content matches
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read sudoers config: %w", err)
	}

	expectedContent := m.buildSudoersContent(decl)
	if string(content) != expectedContent {
		result.CurrentState = "different"
		result.Matches = false
		result.Diff["content"] = map[string]string{"current": "different", "desired": "updated"}
	} else {
		result.CurrentState = "present"
		result.Matches = true
	}

	return result, nil
}

// Apply applies the sudoers configuration.
func (m *SudoersModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StartTime: startTime,
		Success:   false,
		Changed:   false,
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		err := fmt.Errorf("sudoers: name parameter is required")
		result.Comment = err.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, err
	}

	// Validate name (security: prevent path traversal)
	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		err := fmt.Errorf("sudoers: name cannot contain path separators")
		result.Comment = err.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, err
	}

	configPath := filepath.Join("/etc", "sudoers.d", name)
	state := decl.State
	if state == "" {
		state = "present"
	}

	if state == "absent" {
		if _, err := os.Stat(configPath); err == nil {
			if err := os.Remove(configPath); err != nil {
				result.Comment = fmt.Sprintf("failed to remove sudoers config: %v", err)
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(startTime)
				return result, err
			}
			result.Changed = true
		}
		result.Success = true
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Build content
	content := m.buildSudoersContent(decl)

	// Check if file exists and content matches
	existingContent, err := os.ReadFile(configPath)
	if err == nil && string(existingContent) == content {
		result.Success = true
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Write to temp file first, then validate with visudo
	tempFile, err := os.CreateTemp("", "sudoers-*")
	if err != nil {
		result.Comment = fmt.Sprintf("failed to create temp file: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, err
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.WriteString(content); err != nil {
		tempFile.Close()
		result.Comment = fmt.Sprintf("failed to write temp file: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, err
	}
	tempFile.Close()

	// Validate with visudo -cf
	validate := getBoolParameter(decl, "validate", true)
	if validate {
		//nolint:gosec // G204: visudo execution is intentional for sudoers validation
		cmd := exec.CommandContext(ctx, "visudo", "-cf", tempPath)
		if output, err := cmd.CombinedOutput(); err != nil {
			result.Comment = fmt.Sprintf("sudoers syntax validation failed: %v - %s", err, string(output))
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result, fmt.Errorf("sudoers syntax validation failed: %w", err)
		}
	}

	// Move to final location with secure permissions (0o440)
	//nolint:gosec // G306: sudoers files use 0o440 permissions (root read-only + group read)
	if err := os.WriteFile(configPath, []byte(content), 0o440); err != nil {
		result.Comment = fmt.Sprintf("failed to write sudoers config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, err
	}

	result.Success = true
	result.Changed = true
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// Test tests if the sudoers configuration would be applied successfully.
func (m *SudoersModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return false, fmt.Errorf("sudoers: name parameter is required")
	}

	// Need either user or group
	user := getStringParameter(decl, "user", "")
	group := getStringParameter(decl, "group", "")
	content := getStringParameter(decl, "content", "")

	if user == "" && group == "" && content == "" {
		return false, fmt.Errorf("sudoers: user, group, or content parameter is required")
	}

	return true, nil
}

func (m *SudoersModule) buildSudoersContent(decl *StateDeclaration) string {
	// If raw content is provided, use that
	content := getStringParameter(decl, "content", "")
	if content != "" {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return content
	}

	// Build from parameters
	user := getStringParameter(decl, "user", "")
	group := getStringParameter(decl, "group", "")
	host := getStringParameter(decl, "host", "ALL")
	runas := getStringParameter(decl, "runas", "ALL")
	commands := getStringParameter(decl, "commands", "ALL")
	nopasswd := getBoolParameter(decl, "nopasswd", false)

	var spec string
	if group != "" {
		spec = "%" + group
	} else {
		spec = user
	}

	var tags string
	if nopasswd {
		tags = "NOPASSWD: "
	}

	return fmt.Sprintf("%s %s=(%s) %s%s\n", spec, host, runas, tags, commands)
}

// =============================================================================
// Limits Module (ulimits)
// =============================================================================

// LimitsModule manages /etc/security/limits.d configuration.
type LimitsModule struct {
	*BaseModule
}

// NewLimitsModule creates a new limits module.
func NewLimitsModule() *LimitsModule {
	return &LimitsModule{
		BaseModule: NewBaseModule("limits", []string{"present", "absent"}),
	}
}

// Check verifies the current state of a limits configuration.
func (m *LimitsModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("limits: name parameter is required")
	}

	configPath := filepath.Join("/etc", "security", "limits.d", name+".conf")
	state := decl.State
	if state == "" {
		state = "present"
	}

	result.Metadata["name"] = name
	result.Metadata["config_path"] = configPath

	_, err := os.Stat(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = (state == "absent")
			if state == "present" {
				result.Diff["state"] = map[string]string{"current": "absent", "desired": "present"}
			}
			return result, nil
		}
		return nil, fmt.Errorf("failed to stat limits config: %w", err)
	}

	result.Present = true

	if state == "absent" {
		result.CurrentState = "present"
		result.Matches = false
		result.Diff["state"] = map[string]string{"current": "present", "desired": "absent"}
		return result, nil
	}

	// Check if content matches
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read limits config: %w", err)
	}

	expectedContent := m.buildLimitsContent(decl)
	if string(content) != expectedContent {
		result.CurrentState = "different"
		result.Matches = false
		result.Diff["content"] = map[string]string{"current": "different", "desired": "updated"}
	} else {
		result.CurrentState = "present"
		result.Matches = true
	}

	return result, nil
}

// Apply applies the limits configuration.
func (m *LimitsModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StartTime: startTime,
		Success:   false,
		Changed:   false,
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		err := fmt.Errorf("limits: name parameter is required")
		result.Comment = err.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, err
	}

	configPath := filepath.Join("/etc", "security", "limits.d", name+".conf")
	state := decl.State
	if state == "" {
		state = "present"
	}

	if state == "absent" {
		if _, err := os.Stat(configPath); err == nil {
			if err := os.Remove(configPath); err != nil {
				result.Comment = fmt.Sprintf("failed to remove limits config: %v", err)
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(startTime)
				return result, err
			}
			result.Changed = true
		}
		result.Success = true
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Build content
	content := m.buildLimitsContent(decl)

	// Check if file exists and content matches
	existingContent, err := os.ReadFile(configPath)
	if err == nil && string(existingContent) == content {
		result.Success = true
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Ensure directory exists
	//nolint:gosec // G301: limits.d directory needs system access
	if err := os.MkdirAll("/etc/security/limits.d", 0o755); err != nil {
		result.Comment = fmt.Sprintf("failed to create limits.d directory: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, err
	}

	//nolint:gosec // G306: limits.d config files need to be readable by PAM
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		result.Comment = fmt.Sprintf("failed to write limits config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, err
	}

	result.Success = true
	result.Changed = true
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// Test tests if the limits configuration would be applied successfully.
func (m *LimitsModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return false, fmt.Errorf("limits: name parameter is required")
	}
	return true, nil
}

func (m *LimitsModule) buildLimitsContent(decl *StateDeclaration) string {
	// If raw content is provided, use that
	content := getStringParameter(decl, "content", "")
	if content != "" {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return content
	}

	// Build from parameters
	domain := getStringParameter(decl, "domain", "*")
	limitType := getStringParameter(decl, "limit_type", "soft")
	item := getStringParameter(decl, "item", "")
	value := getStringParameter(decl, "value", "")
	name := getStringParameter(decl, "name", "")

	var lines []string
	lines = append(lines, fmt.Sprintf("# Managed by Keystone Core - %s", name))

	if item != "" && value != "" {
		lines = append(lines, fmt.Sprintf("%s %s %s %s", domain, limitType, item, value))
	}

	// Support for multiple limits via "limits" parameter
	if limits, ok := decl.Parameters["limits"]; ok {
		if limitsList, ok := limits.([]interface{}); ok {
			for _, l := range limitsList {
				lm, ok := l.(map[string]interface{})
				if !ok {
					continue
				}
				d := domain
				if v, ok := lm["domain"].(string); ok {
					d = v
				}
				t := limitType
				if v, ok := lm["type"].(string); ok {
					t = v
				}
				i := ""
				if v, ok := lm["item"].(string); ok {
					i = v
				}
				val := ""
				if v, ok := lm["value"].(string); ok {
					val = v
				}
				if i != "" && val != "" {
					lines = append(lines, fmt.Sprintf("%s %s %s %s", d, t, i, val))
				}
			}
		}
	}

	return strings.Join(lines, "\n") + "\n"
}

// =============================================================================
// Modprobe Module
// =============================================================================

// ModprobeModule manages kernel module loading via modprobe.
type ModprobeModule struct {
	*BaseModule
}

// NewModprobeModule creates a new modprobe module.
func NewModprobeModule() *ModprobeModule {
	return &ModprobeModule{
		BaseModule: NewBaseModule("modprobe", []string{"present", "absent", "blacklist"}),
	}
}

// Check verifies the current state of a kernel module.
func (m *ModprobeModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("modprobe: name parameter is required")
	}

	state := decl.State
	if state == "" {
		state = "present"
	}

	result.Metadata["module"] = name

	// Check if module is loaded
	loaded, err := m.isModuleLoaded(name)
	if err != nil {
		return nil, err
	}
	result.Metadata["loaded"] = loaded

	// Check if module is blacklisted
	blacklisted := m.isModuleBlacklisted(name)
	result.Metadata["blacklisted"] = blacklisted

	switch state {
	case "present":
		result.Present = loaded
		if loaded {
			result.CurrentState = "present"
			result.Matches = true
		} else {
			result.CurrentState = "absent"
			result.Matches = false
			result.Diff["state"] = map[string]string{"current": "absent", "desired": "present"}
		}

	case "absent":
		result.Present = loaded
		if !loaded {
			result.CurrentState = "absent"
			result.Matches = true
		} else {
			result.CurrentState = "present"
			result.Matches = false
			result.Diff["state"] = map[string]string{"current": "present", "desired": "absent"}
		}

	case "blacklist":
		result.Present = blacklisted
		if blacklisted && !loaded {
			result.CurrentState = "blacklist"
			result.Matches = true
		} else {
			result.CurrentState = "not-blacklisted"
			result.Matches = false
			result.Diff["state"] = map[string]string{"current": "not-blacklisted", "desired": "blacklist"}
		}
	}

	return result, nil
}

// Apply applies the kernel module state.
func (m *ModprobeModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StartTime: startTime,
		Success:   false,
		Changed:   false,
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		err := fmt.Errorf("modprobe: name parameter is required")
		result.Comment = err.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, err
	}

	state := decl.State
	if state == "" {
		state = "present"
	}

	params := getStringParameter(decl, "params", "")

	switch state {
	case "present":
		loaded, _ := m.isModuleLoaded(name)
		if !loaded {
			args := []string{name}
			if params != "" {
				args = append(args, strings.Fields(params)...)
			}
			//nolint:gosec // G204: modprobe execution is intentional for kernel module management
			cmd := exec.CommandContext(ctx, "modprobe", args...)
			if output, err := cmd.CombinedOutput(); err != nil {
				result.Comment = fmt.Sprintf("failed to load module %s: %v - %s", name, err, string(output))
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(startTime)
				return result, fmt.Errorf("failed to load module %s: %w", name, err)
			}
			result.Changed = true
		}

		// Handle persistent loading
		persist := getBoolParameter(decl, "persist", false)
		if persist {
			changed, err := m.ensureModulesPersist(name)
			if err != nil {
				result.Comment = fmt.Sprintf("failed to persist module: %v", err)
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(startTime)
				return result, err
			}
			if changed {
				result.Changed = true
			}
		}

	case "absent":
		loaded, _ := m.isModuleLoaded(name)
		if loaded {
			//nolint:gosec // G204: modprobe execution is intentional for kernel module management
			cmd := exec.CommandContext(ctx, "modprobe", "-r", name)
			if output, err := cmd.CombinedOutput(); err != nil {
				result.Comment = fmt.Sprintf("failed to unload module %s: %v - %s", name, err, string(output))
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(startTime)
				return result, fmt.Errorf("failed to unload module %s: %w", name, err)
			}
			result.Changed = true
		}

	case "blacklist":
		// First, unload if loaded
		loaded, _ := m.isModuleLoaded(name)
		if loaded {
			//nolint:gosec // G204: modprobe execution is intentional for kernel module management
			cmd := exec.CommandContext(ctx, "modprobe", "-r", name)
			if _, err := cmd.CombinedOutput(); err != nil {
				// Module might be in use, just warn
				result.Comment = fmt.Sprintf("warning: could not unload module %s (may be in use)", name)
			} else {
				result.Changed = true
			}
		}

		// Add to blacklist
		blacklisted := m.isModuleBlacklisted(name)
		if !blacklisted {
			blacklistPath := filepath.Join("/etc", "modprobe.d", name+"-blacklist.conf")
			content := fmt.Sprintf("# Managed by Keystone Core\nblacklist %s\ninstall %s /bin/true\n", name, name)
			//nolint:gosec // G306: modprobe.d config files need to be readable by the kernel
			if err := os.WriteFile(blacklistPath, []byte(content), 0o644); err != nil {
				result.Comment = fmt.Sprintf("failed to blacklist module: %v", err)
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(startTime)
				return result, err
			}
			result.Changed = true
		}
	}

	result.Success = true
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// Test tests if the modprobe configuration would be applied successfully.
func (m *ModprobeModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return false, fmt.Errorf("modprobe: name parameter is required")
	}
	return true, nil
}

func (m *ModprobeModule) isModuleLoaded(name string) (bool, error) {
	content, err := os.ReadFile("/proc/modules")
	if err != nil {
		return false, fmt.Errorf("failed to read /proc/modules: %w", err)
	}

	// Format: module_name size used_count depends state address
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == name {
			return true, nil
		}
		// Also check with underscores replaced with hyphens and vice versa
		modName := strings.ReplaceAll(name, "-", "_")
		if len(fields) > 0 && fields[0] == modName {
			return true, nil
		}
	}
	return false, nil
}

func (m *ModprobeModule) isModuleBlacklisted(name string) bool {
	blacklistPath := filepath.Join("/etc", "modprobe.d", name+"-blacklist.conf")
	if _, err := os.Stat(blacklistPath); err == nil {
		return true
	}

	// Also check global blacklist files
	files, _ := filepath.Glob("/etc/modprobe.d/*.conf")
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "blacklist ") {
				modName := strings.TrimPrefix(line, "blacklist ")
				modName = strings.TrimSpace(modName)
				if modName == name || modName == strings.ReplaceAll(name, "-", "_") {
					return true
				}
			}
		}
	}
	return false
}

func (m *ModprobeModule) ensureModulesPersist(name string) (bool, error) {
	// Add to /etc/modules-load.d/
	persistPath := filepath.Join("/etc", "modules-load.d", name+".conf")

	if _, err := os.Stat(persistPath); err == nil {
		// Already exists
		return false, nil
	}

	//nolint:gosec // G301: modules-load.d directory needs system access
	if err := os.MkdirAll("/etc/modules-load.d", 0o755); err != nil {
		return false, err
	}

	content := fmt.Sprintf("# Managed by Keystone Core\n%s\n", name)
	//nolint:gosec // G306: modules-load.d config files need to be readable by systemd
	if err := os.WriteFile(persistPath, []byte(content), 0o644); err != nil {
		return false, err
	}

	return true, nil
}

// =============================================================================
// Syslog Module
// =============================================================================

// SyslogModule manages rsyslog configuration.
type SyslogModule struct {
	*BaseModule
}

// NewSyslogModule creates a new syslog module.
func NewSyslogModule() *SyslogModule {
	return &SyslogModule{
		BaseModule: NewBaseModule("syslog", []string{"present", "absent"}),
	}
}

// Check verifies the current state of a syslog configuration.
func (m *SyslogModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("syslog: name parameter is required")
	}

	configPath := filepath.Join("/etc", "rsyslog.d", name+".conf")
	state := decl.State
	if state == "" {
		state = "present"
	}

	result.Metadata["name"] = name
	result.Metadata["config_path"] = configPath

	_, err := os.Stat(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = (state == "absent")
			if state == "present" {
				result.Diff["state"] = map[string]string{"current": "absent", "desired": "present"}
			}
			return result, nil
		}
		return nil, fmt.Errorf("failed to stat syslog config: %w", err)
	}

	result.Present = true

	if state == "absent" {
		result.CurrentState = "present"
		result.Matches = false
		result.Diff["state"] = map[string]string{"current": "present", "desired": "absent"}
		return result, nil
	}

	// Check if content matches
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read syslog config: %w", err)
	}

	expectedContent := m.buildSyslogContent(decl)
	if string(content) != expectedContent {
		result.CurrentState = "different"
		result.Matches = false
		result.Diff["content"] = map[string]string{"current": "different", "desired": "updated"}
	} else {
		result.CurrentState = "present"
		result.Matches = true
	}

	return result, nil
}

// Apply applies the syslog configuration.
func (m *SyslogModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StartTime: startTime,
		Success:   false,
		Changed:   false,
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		err := fmt.Errorf("syslog: name parameter is required")
		result.Comment = err.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, err
	}

	configPath := filepath.Join("/etc", "rsyslog.d", name+".conf")
	state := decl.State
	if state == "" {
		state = "present"
	}

	if state == "absent" {
		if _, err := os.Stat(configPath); err == nil {
			if err := os.Remove(configPath); err != nil {
				result.Comment = fmt.Sprintf("failed to remove syslog config: %v", err)
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(startTime)
				return result, err
			}
			result.Changed = true

			// Restart rsyslog
			cmd := exec.CommandContext(ctx, "systemctl", "restart", "rsyslog")
			if _, err := cmd.CombinedOutput(); err != nil {
				result.Comment = "warning: failed to restart rsyslog"
			}
		}
		result.Success = true
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Build content
	content := m.buildSyslogContent(decl)

	// Check if file exists and content matches
	existingContent, err := os.ReadFile(configPath)
	if err == nil && string(existingContent) == content {
		result.Success = true
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Ensure directory exists
	//nolint:gosec // G301: rsyslog.d directory needs system access
	if err := os.MkdirAll("/etc/rsyslog.d", 0o755); err != nil {
		result.Comment = fmt.Sprintf("failed to create rsyslog.d directory: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, err
	}

	//nolint:gosec // G306: rsyslog config files need to be readable by rsyslog
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		result.Comment = fmt.Sprintf("failed to write syslog config: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, err
	}

	result.Changed = true

	// Restart rsyslog if requested
	restart := getBoolParameter(decl, "restart", true)
	if restart {
		cmd := exec.CommandContext(ctx, "systemctl", "restart", "rsyslog")
		if _, err := cmd.CombinedOutput(); err != nil {
			result.Comment = "warning: failed to restart rsyslog"
		}
	}

	result.Success = true
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// Test tests if the syslog configuration would be applied successfully.
func (m *SyslogModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return false, fmt.Errorf("syslog: name parameter is required")
	}
	return true, nil
}

func (m *SyslogModule) buildSyslogContent(decl *StateDeclaration) string {
	// If raw content is provided, use that
	content := getStringParameter(decl, "content", "")
	if content != "" {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return content
	}

	// Build from parameters
	name := getStringParameter(decl, "name", "")
	facility := getStringParameter(decl, "facility", "*")
	priority := getStringParameter(decl, "priority", "*")
	destination := getStringParameter(decl, "destination", "")

	var lines []string
	lines = append(lines, fmt.Sprintf("# Managed by Keystone Core - %s", name))

	if destination != "" {
		selector := fmt.Sprintf("%s.%s", facility, priority)
		lines = append(lines, fmt.Sprintf("%s\t%s", selector, destination))
	}

	// Support for multiple rules
	if rules, ok := decl.Parameters["rules"]; ok {
		if rulesList, ok := rules.([]interface{}); ok {
			for _, r := range rulesList {
				rm, ok := r.(map[string]interface{})
				if !ok {
					continue
				}
				f := facility
				if v, ok := rm["facility"].(string); ok {
					f = v
				}
				p := priority
				if v, ok := rm["priority"].(string); ok {
					p = v
				}
				d := ""
				if v, ok := rm["destination"].(string); ok {
					d = v
				}
				if d != "" {
					selector := fmt.Sprintf("%s.%s", f, p)
					lines = append(lines, fmt.Sprintf("%s\t%s", selector, d))
				}
			}
		}
	}

	return strings.Join(lines, "\n") + "\n"
}

// =============================================================================
// Lineinfile Module
// =============================================================================

// LineinfileModule manages individual lines in files.
type LineinfileModule struct {
	*BaseModule
}

// NewLineinfileModule creates a new lineinfile module.
func NewLineinfileModule() *LineinfileModule {
	return &LineinfileModule{
		BaseModule: NewBaseModule("lineinfile", []string{"present", "absent"}),
	}
}

// Check verifies the current state of a line in a file.
func (m *LineinfileModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	path := getStringParameter(decl, "path", "")
	if path == "" {
		return nil, fmt.Errorf("lineinfile: path parameter is required")
	}

	state := decl.State
	if state == "" {
		state = "present"
	}

	line := getStringParameter(decl, "line", "")
	regexpStr := getStringParameter(decl, "regexp", "")

	result.Metadata["path"] = path
	result.Metadata["line"] = line

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			result.Present = false
			result.CurrentState = "absent"
			if state == "present" {
				create := getBoolParameter(decl, "create", false)
				result.Matches = !create
				if create {
					result.Diff["file"] = map[string]string{"current": "absent", "desired": "created"}
				}
			} else {
				result.Matches = true
			}
			return result, nil
		}
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	result.Present = true
	lines := strings.Split(string(content), "\n")

	// Find matching line
	found := false
	matchIdx := -1

	if regexpStr != "" {
		re, err := regexp.Compile(regexpStr)
		if err != nil {
			return nil, fmt.Errorf("invalid regexp: %w", err)
		}
		for i, l := range lines {
			if re.MatchString(l) {
				found = true
				matchIdx = i
				break
			}
		}
	} else if line != "" {
		for i, l := range lines {
			if l == line {
				found = true
				matchIdx = i
				break
			}
		}
	}

	result.Metadata["line_found"] = found
	result.Metadata["match_index"] = matchIdx

	switch state {
	case "present":
		switch {
		case !found:
			result.CurrentState = "absent"
			result.Matches = false
			result.Diff["line"] = map[string]string{"current": "absent", "desired": "present"}
		case line != "" && matchIdx >= 0 && lines[matchIdx] != line:
			// Line found by regexp but doesn't match desired line
			result.CurrentState = "different"
			result.Matches = false
			result.Diff["line"] = map[string]string{"current": lines[matchIdx], "desired": line}
		default:
			result.CurrentState = "present"
			result.Matches = true
		}
	case "absent":
		if found {
			result.CurrentState = "present"
			result.Matches = false
			result.Diff["line"] = map[string]string{"current": "present", "desired": "absent"}
		} else {
			result.CurrentState = "absent"
			result.Matches = true
		}
	}

	return result, nil
}

// Apply applies the lineinfile state.
func (m *LineinfileModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StartTime: startTime,
		Success:   false,
		Changed:   false,
	}

	path := getStringParameter(decl, "path", "")
	if path == "" {
		err := fmt.Errorf("lineinfile: path parameter is required")
		result.Comment = err.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, err
	}

	state := decl.State
	if state == "" {
		state = "present"
	}

	line := getStringParameter(decl, "line", "")
	regexpStr := getStringParameter(decl, "regexp", "")
	insertafter := getStringParameter(decl, "insertafter", "")
	insertbefore := getStringParameter(decl, "insertbefore", "")
	create := getBoolParameter(decl, "create", false)
	backup := getBoolParameter(decl, "backup", false)

	// Read existing content
	content, err := os.ReadFile(path)
	var lines []string
	fileExists := true

	if err != nil {
		if os.IsNotExist(err) {
			fileExists = false
			switch {
			case state == "present" && create:
				lines = []string{}
			case state == "present":
				err := fmt.Errorf("file does not exist and create=false: %s", path)
				result.Comment = err.Error()
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(startTime)
				return result, err
			default:
				// absent state, file doesn't exist - nothing to do
				result.Success = true
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(startTime)
				return result, nil
			}
		} else {
			result.Comment = fmt.Sprintf("failed to read file: %v", err)
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result, err
		}
	} else {
		lines = strings.Split(string(content), "\n")
		// Remove trailing empty line if file ends with newline
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
	}

	// Find matching line
	var re *regexp.Regexp
	if regexpStr != "" {
		re, err = regexp.Compile(regexpStr)
		if err != nil {
			result.Comment = fmt.Sprintf("invalid regexp: %v", err)
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result, fmt.Errorf("invalid regexp: %w", err)
		}
	}

	matchIdx := -1
	for i, l := range lines {
		if re != nil && re.MatchString(l) {
			matchIdx = i
			break
		} else if re == nil && line != "" && l == line {
			matchIdx = i
			break
		}
	}

	switch state {
	case "present":
		if line == "" {
			err := fmt.Errorf("lineinfile: line parameter is required for present state")
			result.Comment = err.Error()
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result, err
		}

		if matchIdx >= 0 {
			// Line found - replace if different
			if lines[matchIdx] != line {
				lines[matchIdx] = line
				result.Changed = true
			}
		} else {
			// Line not found - insert
			insertIdx := len(lines) // default: append

			if insertafter != "" {
				if insertafter == "EOF" {
					insertIdx = len(lines)
				} else {
					afterRe, _ := regexp.Compile(insertafter)
					for i, l := range lines {
						if afterRe != nil && afterRe.MatchString(l) {
							insertIdx = i + 1
							break
						}
					}
				}
			} else if insertbefore != "" {
				if insertbefore == "BOF" {
					insertIdx = 0
				} else {
					beforeRe, _ := regexp.Compile(insertbefore)
					for i, l := range lines {
						if beforeRe != nil && beforeRe.MatchString(l) {
							insertIdx = i
							break
						}
					}
				}
			}

			// Insert line
			newLines := make([]string, 0, len(lines)+1)
			newLines = append(newLines, lines[:insertIdx]...)
			newLines = append(newLines, line)
			newLines = append(newLines, lines[insertIdx:]...)
			lines = newLines
			result.Changed = true
		}

	case "absent":
		if matchIdx >= 0 {
			// Remove line
			lines = append(lines[:matchIdx], lines[matchIdx+1:]...)
			result.Changed = true
		}
	}

	if result.Changed {
		// Backup if requested
		if backup && fileExists {
			backupPath := path + ".bak"
			//nolint:gosec // G306: backup files use same permissions as original
			if err := os.WriteFile(backupPath, content, 0o644); err != nil {
				result.Comment = fmt.Sprintf("warning: failed to create backup: %v", err)
			}
		}

		// Write file
		newContent := strings.Join(lines, "\n") + "\n"
		modeStr := getStringParameter(decl, "mode", "")
		fileMode := os.FileMode(0o644)
		if modeStr != "" {
			if modeInt, err := strconv.ParseUint(modeStr, 8, 32); err == nil {
				fileMode = os.FileMode(modeInt)
			}
		} else if fileExists {
			// Preserve existing mode
			if info, err := os.Stat(path); err == nil {
				fileMode = info.Mode().Perm()
			}
		}

		// Ensure parent directory exists if creating
		if !fileExists {
			//nolint:gosec // G301: config file parent directory needs system access
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				result.Comment = fmt.Sprintf("failed to create directory: %v", err)
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(startTime)
				return result, err
			}
		}

		if err := os.WriteFile(path, []byte(newContent), fileMode); err != nil {
			result.Comment = fmt.Sprintf("failed to write file: %v", err)
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result, err
		}
	}

	result.Success = true
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// Test tests if the lineinfile configuration would be applied successfully.
func (m *LineinfileModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	path := getStringParameter(decl, "path", "")
	if path == "" {
		return false, fmt.Errorf("lineinfile: path parameter is required")
	}

	state := decl.State
	if state == "" {
		state = "present"
	}

	if state == "present" {
		line := getStringParameter(decl, "line", "")
		if line == "" {
			return false, fmt.Errorf("lineinfile: line parameter is required for present state")
		}
	}

	return true, nil
}

// =============================================================================
// INI File Module
// =============================================================================

// IniFileModule manages INI file configuration.
type IniFileModule struct {
	*BaseModule
}

// NewIniFileModule creates a new ini_file module.
func NewIniFileModule() *IniFileModule {
	return &IniFileModule{
		BaseModule: NewBaseModule("ini_file", []string{"present", "absent"}),
	}
}

// Check verifies the current state of an INI file entry.
func (m *IniFileModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	path := getStringParameter(decl, "path", "")
	if path == "" {
		return nil, fmt.Errorf("ini_file: path parameter is required")
	}

	section := getStringParameter(decl, "section", "")
	option := getStringParameter(decl, "option", "")
	value := getStringParameter(decl, "value", "")
	state := decl.State
	if state == "" {
		state = "present"
	}

	result.Metadata["path"] = path
	result.Metadata["section"] = section
	result.Metadata["option"] = option

	ini, err := m.parseINI(path)
	if err != nil {
		if os.IsNotExist(err) {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = (state == "absent")
			if state == "present" {
				result.Diff["file"] = map[string]string{"current": "absent", "desired": "created"}
			}
			return result, nil
		}
		return nil, err
	}

	result.Present = true

	// Find current value
	if sectionMap, ok := ini[section]; ok {
		if currentValue, ok := sectionMap[option]; ok {
			result.Metadata["current_value"] = currentValue
			switch {
			case state == "present" && currentValue != value:
				result.CurrentState = "different"
				result.Matches = false
				result.Diff["value"] = map[string]string{"current": currentValue, "desired": value}
			case state == "absent":
				result.CurrentState = "present"
				result.Matches = false
				result.Diff["option"] = map[string]string{"current": "present", "desired": "absent"}
			default:
				result.CurrentState = "present"
				result.Matches = true
			}
		} else {
			result.Metadata["current_value"] = nil
			if state == "present" {
				result.CurrentState = "absent"
				result.Matches = false
				result.Diff["option"] = map[string]string{"current": "absent", "desired": "present"}
			} else {
				result.CurrentState = "absent"
				result.Matches = true
			}
		}
	} else {
		result.Metadata["section_exists"] = false
		if state == "present" {
			result.CurrentState = "absent"
			result.Matches = false
			result.Diff["section"] = map[string]string{"current": "absent", "desired": "present"}
		} else {
			result.CurrentState = "absent"
			result.Matches = true
		}
	}

	return result, nil
}

// Apply applies the INI file state.
func (m *IniFileModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StartTime: startTime,
		Success:   false,
		Changed:   false,
	}

	path := getStringParameter(decl, "path", "")
	if path == "" {
		err := fmt.Errorf("ini_file: path parameter is required")
		result.Comment = err.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, err
	}

	section := getStringParameter(decl, "section", "")
	option := getStringParameter(decl, "option", "")
	value := getStringParameter(decl, "value", "")
	state := decl.State
	if state == "" {
		state = "present"
	}
	create := getBoolParameter(decl, "create", true)
	backup := getBoolParameter(decl, "backup", false)

	// Read existing content
	content, err := os.ReadFile(path)
	fileExists := true

	if err != nil {
		if os.IsNotExist(err) {
			fileExists = false
			switch {
			case state == "present" && create:
				content = []byte{}
			case state == "present":
				err := fmt.Errorf("file does not exist and create=false: %s", path)
				result.Comment = err.Error()
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(startTime)
				return result, err
			default:
				result.Success = true
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(startTime)
				return result, nil
			}
		} else {
			result.Comment = fmt.Sprintf("failed to read file: %v", err)
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result, err
		}
	}

	_ = fileExists // Prevent unused variable warning
	lines := strings.Split(string(content), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	// Process INI file
	currentSection := ""
	sectionFound := false
	optionFound := false
	modified := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Section header
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			currentSection = strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]")
			if currentSection == section {
				sectionFound = true
			}
			continue
		}

		// Option line
		if currentSection == section && strings.Contains(trimmed, "=") {
			parts := strings.SplitN(trimmed, "=", 2)
			key := strings.TrimSpace(parts[0])

			if key == option {
				optionFound = true
				switch state {
				case "present":
					newLine := fmt.Sprintf("%s = %s", option, value)
					if lines[i] != newLine {
						lines[i] = newLine
						modified = true
					}
				case "absent":
					lines = append(lines[:i], lines[i+1:]...)
					modified = true
				}
				break
			}
		}
	}

	if state == "present" && !optionFound {
		newLine := fmt.Sprintf("%s = %s", option, value)

		if !sectionFound {
			// Add section and option
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			lines = append(lines, fmt.Sprintf("[%s]", section), newLine)
			modified = true
		} else {
			// Find end of section and add option
			inSection := false
			insertIdx := len(lines)

			for i, line := range lines {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
					sec := strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]")
					if sec == section {
						inSection = true
					} else if inSection {
						insertIdx = i
						break
					}
				}
			}

			newLines := make([]string, 0, len(lines)+1)
			newLines = append(newLines, lines[:insertIdx]...)
			newLines = append(newLines, newLine)
			newLines = append(newLines, lines[insertIdx:]...)
			lines = newLines
			modified = true
		}
	}

	if modified {
		result.Changed = true

		// Backup if requested
		if backup && fileExists {
			backupPath := path + ".bak"
			//nolint:gosec // G306: backup files use same permissions as original
			if err := os.WriteFile(backupPath, content, 0o644); err != nil {
				result.Comment = fmt.Sprintf("warning: failed to create backup: %v", err)
			}
		}

		// Write file
		newContent := strings.Join(lines, "\n") + "\n"
		modeStr := getStringParameter(decl, "mode", "")
		fileMode := os.FileMode(0o644)
		if modeStr != "" {
			if modeInt, err := strconv.ParseUint(modeStr, 8, 32); err == nil {
				fileMode = os.FileMode(modeInt)
			}
		} else if fileExists {
			if info, err := os.Stat(path); err == nil {
				fileMode = info.Mode().Perm()
			}
		}

		// Ensure parent directory exists
		if !fileExists {
			//nolint:gosec // G301: config file parent directory needs system access
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				result.Comment = fmt.Sprintf("failed to create directory: %v", err)
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(startTime)
				return result, err
			}
		}

		if err := os.WriteFile(path, []byte(newContent), fileMode); err != nil {
			result.Comment = fmt.Sprintf("failed to write file: %v", err)
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result, err
		}
	}

	result.Success = true
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// Test tests if the ini_file configuration would be applied successfully.
func (m *IniFileModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	path := getStringParameter(decl, "path", "")
	if path == "" {
		return false, fmt.Errorf("ini_file: path parameter is required")
	}

	section := getStringParameter(decl, "section", "")
	if section == "" {
		return false, fmt.Errorf("ini_file: section parameter is required")
	}

	option := getStringParameter(decl, "option", "")
	if option == "" {
		return false, fmt.Errorf("ini_file: option parameter is required")
	}

	return true, nil
}

func (m *IniFileModule) parseINI(path string) (map[string]map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	result := make(map[string]map[string]string)
	currentSection := ""

	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}

		// Section header
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			currentSection = strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]")
			if _, ok := result[currentSection]; !ok {
				result[currentSection] = make(map[string]string)
			}
			continue
		}

		// Key=value
		if strings.Contains(trimmed, "=") {
			parts := strings.SplitN(trimmed, "=", 2)
			key := strings.TrimSpace(parts[0])
			value := ""
			if len(parts) > 1 {
				value = strings.TrimSpace(parts[1])
			}
			if currentSection != "" {
				result[currentSection][key] = value
			}
		}
	}

	return result, nil
}

// =============================================================================
// Archive Module
// =============================================================================

// ArchiveModule manages archive extraction and creation.
type ArchiveModule struct {
	*BaseModule
}

// NewArchiveModule creates a new archive module.
func NewArchiveModule() *ArchiveModule {
	return &ArchiveModule{
		BaseModule: NewBaseModule("archive", []string{"extracted", "present", "absent"}),
	}
}

// Check verifies the current state of an archive.
func (m *ArchiveModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	src := getStringParameter(decl, "src", "")
	dest := getStringParameter(decl, "dest", "")
	state := decl.State
	if state == "" {
		state = "extracted"
	}

	result.Metadata["src"] = src
	result.Metadata["dest"] = dest

	switch state {
	case "extracted":
		if dest == "" {
			return nil, fmt.Errorf("archive: dest parameter is required for extracted state")
		}

		// Check if destination exists
		info, err := os.Stat(dest)
		if err != nil {
			if os.IsNotExist(err) {
				result.Present = false
				result.CurrentState = "absent"
				result.Matches = false
				result.Diff["state"] = map[string]string{"current": "absent", "desired": "extracted"}
				return result, nil
			}
			return nil, fmt.Errorf("failed to stat destination: %w", err)
		}

		if !info.IsDir() {
			result.Present = false
			result.CurrentState = "not-directory"
			result.Matches = false
			result.Diff["type"] = map[string]string{"current": "file", "desired": "directory"}
		} else {
			// Check if directory is empty
			entries, _ := os.ReadDir(dest)
			if len(entries) == 0 {
				result.Present = false
				result.CurrentState = "empty"
				result.Matches = false
				result.Diff["content"] = map[string]string{"current": "empty", "desired": "extracted"}
			} else {
				result.Present = true
				result.CurrentState = "extracted"
				result.Matches = true
			}
		}

	case "present":
		if dest == "" {
			return nil, fmt.Errorf("archive: dest parameter is required for present state")
		}

		// Check if archive exists
		if _, err := os.Stat(dest); err != nil {
			if os.IsNotExist(err) {
				result.Present = false
				result.CurrentState = "absent"
				result.Matches = false
				result.Diff["state"] = map[string]string{"current": "absent", "desired": "present"}
			}
		} else {
			result.Present = true
			result.CurrentState = "present"
			result.Matches = true
		}

	case "absent":
		if dest == "" {
			return nil, fmt.Errorf("archive: dest parameter is required for absent state")
		}

		if _, err := os.Stat(dest); err == nil {
			result.Present = true
			result.CurrentState = "present"
			result.Matches = false
			result.Diff["state"] = map[string]string{"current": "present", "desired": "absent"}
		} else {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = true
		}
	}

	return result, nil
}

// Apply applies the archive state.
func (m *ArchiveModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StartTime: startTime,
		Success:   false,
		Changed:   false,
	}

	src := getStringParameter(decl, "src", "")
	dest := getStringParameter(decl, "dest", "")
	state := decl.State
	if state == "" {
		state = "extracted"
	}
	format := getStringParameter(decl, "format", "")

	switch state {
	case "extracted":
		if src == "" {
			err := fmt.Errorf("archive: src parameter is required for extracted state")
			result.Comment = err.Error()
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result, err
		}
		if dest == "" {
			err := fmt.Errorf("archive: dest parameter is required for extracted state")
			result.Comment = err.Error()
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result, err
		}

		// Detect format if not specified
		if format == "" {
			format = m.detectFormat(src)
		}

		// Create destination directory
		//nolint:gosec // G301: extraction destination directory needs to be accessible
		if err := os.MkdirAll(dest, 0o755); err != nil {
			result.Comment = fmt.Sprintf("failed to create destination directory: %v", err)
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result, err
		}

		// Extract archive
		var cmd *exec.Cmd

		//nolint:gosec // G204: tar/unzip execution is intentional for archive extraction
		switch format {
		case "tar.gz", "tgz":
			cmd = exec.CommandContext(ctx, "tar", "-xzf", src, "-C", dest)
		case "tar.bz2", "tbz2":
			cmd = exec.CommandContext(ctx, "tar", "-xjf", src, "-C", dest)
		case "tar.xz", "txz":
			cmd = exec.CommandContext(ctx, "tar", "-xJf", src, "-C", dest)
		case "tar":
			cmd = exec.CommandContext(ctx, "tar", "-xf", src, "-C", dest)
		case "zip":
			cmd = exec.CommandContext(ctx, "unzip", "-o", src, "-d", dest)
		default:
			err := fmt.Errorf("unsupported archive format: %s", format)
			result.Comment = err.Error()
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result, err
		}

		if output, err := cmd.CombinedOutput(); err != nil {
			result.Comment = fmt.Sprintf("failed to extract archive: %v - %s", err, string(output))
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result, fmt.Errorf("failed to extract archive: %w", err)
		}

		result.Changed = true

	case "present":
		if src == "" {
			err := fmt.Errorf("archive: src parameter is required for present state")
			result.Comment = err.Error()
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result, err
		}
		if dest == "" {
			err := fmt.Errorf("archive: dest parameter is required for present state")
			result.Comment = err.Error()
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result, err
		}

		// Detect format if not specified
		if format == "" {
			format = m.detectFormat(dest)
		}

		// Create archive
		var cmd *exec.Cmd

		//nolint:gosec // G204: tar/zip execution is intentional for archive creation
		switch format {
		case "tar.gz", "tgz":
			cmd = exec.CommandContext(ctx, "tar", "-czf", dest, "-C", filepath.Dir(src), filepath.Base(src))
		case "tar.bz2", "tbz2":
			cmd = exec.CommandContext(ctx, "tar", "-cjf", dest, "-C", filepath.Dir(src), filepath.Base(src))
		case "tar.xz", "txz":
			cmd = exec.CommandContext(ctx, "tar", "-cJf", dest, "-C", filepath.Dir(src), filepath.Base(src))
		case "tar":
			cmd = exec.CommandContext(ctx, "tar", "-cf", dest, "-C", filepath.Dir(src), filepath.Base(src))
		case "zip":
			cmd = exec.CommandContext(ctx, "zip", "-r", dest, src)
		default:
			err := fmt.Errorf("unsupported archive format: %s", format)
			result.Comment = err.Error()
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result, err
		}

		if output, err := cmd.CombinedOutput(); err != nil {
			result.Comment = fmt.Sprintf("failed to create archive: %v - %s", err, string(output))
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result, fmt.Errorf("failed to create archive: %w", err)
		}

		result.Changed = true

	case "absent":
		if dest == "" {
			err := fmt.Errorf("archive: dest parameter is required for absent state")
			result.Comment = err.Error()
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result, err
		}

		if _, err := os.Stat(dest); err == nil {
			if err := os.RemoveAll(dest); err != nil {
				result.Comment = fmt.Sprintf("failed to remove archive: %v", err)
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(startTime)
				return result, err
			}
			result.Changed = true
		}
	}

	result.Success = true
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// Test tests if the archive configuration would be applied successfully.
func (m *ArchiveModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	state := decl.State
	if state == "" {
		state = "extracted"
	}

	switch state {
	case "extracted":
		src := getStringParameter(decl, "src", "")
		dest := getStringParameter(decl, "dest", "")
		if src == "" || dest == "" {
			return false, fmt.Errorf("archive: src and dest parameters are required for extracted state")
		}
	case "present":
		src := getStringParameter(decl, "src", "")
		dest := getStringParameter(decl, "dest", "")
		if src == "" || dest == "" {
			return false, fmt.Errorf("archive: src and dest parameters are required for present state")
		}
	case "absent":
		dest := getStringParameter(decl, "dest", "")
		if dest == "" {
			return false, fmt.Errorf("archive: dest parameter is required for absent state")
		}
	}

	return true, nil
}

func (m *ArchiveModule) detectFormat(filename string) string {
	lower := strings.ToLower(filename)

	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		return "tar.gz"
	}
	if strings.HasSuffix(lower, ".tar.bz2") || strings.HasSuffix(lower, ".tbz2") {
		return "tar.bz2"
	}
	if strings.HasSuffix(lower, ".tar.xz") || strings.HasSuffix(lower, ".txz") {
		return "tar.xz"
	}
	if strings.HasSuffix(lower, ".tar") {
		return "tar"
	}
	if strings.HasSuffix(lower, ".zip") {
		return "zip"
	}

	return "tar.gz" // Default
}

// =============================================================================
// Module Registration
// =============================================================================

func init() {
	_ = RegisterModule(NewLogrotateModule())
	_ = RegisterModule(NewSudoersModule())
	_ = RegisterModule(NewLimitsModule())
	_ = RegisterModule(NewModprobeModule())
	_ = RegisterModule(NewSyslogModule())
	_ = RegisterModule(NewLineinfileModule())
	_ = RegisterModule(NewIniFileModule())
	_ = RegisterModule(NewArchiveModule())
}
