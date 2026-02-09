package statemgmt

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ============================================================================
// Pip Module - Python Package Management
// ============================================================================

// PipModule manages Python packages via pip
type PipModule struct {
	*BaseModule
}

// NewPipModule creates a new pip module
func NewPipModule() *PipModule {
	return &PipModule{
		BaseModule: NewBaseModule("pip", []string{"installed", "removed", "latest"}),
	}
}

// Check checks if a Python package is installed
func (m *PipModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	name := getStringParameter(decl, "name", decl.ID)
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	pipCmd := m.getPipCommand(decl)
	installed, version, err := m.isPackageInstalled(ctx, pipCmd, name)
	if err != nil {
		return nil, err
	}

	result.Present = installed
	if installed {
		result.CurrentState = "installed"
		result.Metadata["version"] = version
	} else {
		result.CurrentState = "removed"
	}

	switch decl.State {
	case "installed", "latest":
		result.Matches = installed
		if !installed {
			result.Diff["state"] = map[string]string{"current": "removed", "desired": decl.State}
		}
	case "removed":
		result.Matches = !installed
		if installed {
			result.Diff["state"] = map[string]string{"current": "installed", "desired": "removed"}
		}
	}

	// Check version constraint
	if desiredVersion := getStringParameter(decl, "version", ""); desiredVersion != "" && installed {
		if version != desiredVersion {
			result.Matches = false
			result.Diff["version"] = map[string]string{"current": version, "desired": desiredVersion}
		}
	}

	return result, nil //nolint:nilerr // error captured in result.Error
}

// Apply applies the pip package state
func (m *PipModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	name := getStringParameter(decl, "name", decl.ID)
	if name == "" {
		result.Error = fmt.Errorf("name parameter is required")
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Error = err
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	if checkResult.Matches {
		result.Success = true
		result.Comment = "Already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	pipCmd := m.getPipCommand(decl)
	var applyErr error

	switch decl.State {
	case "installed":
		applyErr = m.installPackage(ctx, pipCmd, decl, name, result)
	case "latest":
		applyErr = m.upgradePackage(ctx, pipCmd, decl, name, result)
	case "removed":
		applyErr = m.removePackage(ctx, pipCmd, name, result)
	}

	if applyErr != nil {
		result.Error = applyErr
	} else {
		result.Success = true
		result.Changed = true
		result.Changes = checkResult.Diff
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil //nolint:nilerr // error captured in result.Error
}

// Test tests if the package is in the desired state
func (m *PipModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil //nolint:nilerr // intentional
}

func (m *PipModule) getPipCommand(decl *StateDeclaration) string {
	if cmd := getStringParameter(decl, "executable", ""); cmd != "" {
		return cmd
	}
	// Check if pip3 is explicitly requested
	if getBoolParameter(decl, "pip3", false) {
		return "pip3"
	}
	// Try pip3 first, fall back to pip
	if _, err := exec.LookPath("pip3"); err == nil {
		return "pip3"
	}
	return "pip"
}

func (m *PipModule) isPackageInstalled(ctx context.Context, pipCmd, name string) (installed bool, version string, err error) {
	cmd := exec.CommandContext(ctx, pipCmd, "show", name) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.Output()
	if err != nil {
		return false, "", nil //nolint:nilerr // pip show returns error when package not installed
	}

	// Parse version from output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Version:") {
			version := strings.TrimSpace(strings.TrimPrefix(line, "Version:"))
			return true, version, nil //nolint:nilerr // returning installation status, no error
		}
	}

	return true, "", nil //nolint:nilerr // package found but version not parsed
}

func (m *PipModule) installPackage(ctx context.Context, pipCmd string, decl *StateDeclaration, name string, result *StateResult) error {
	args := []string{"install"}

	// Add extra index URL if specified
	if extraIndex := getStringParameter(decl, "extra_index_url", ""); extraIndex != "" {
		args = append(args, "--extra-index-url", extraIndex)
	}

	// Add version if specified
	if version := getStringParameter(decl, "version", ""); version != "" {
		name = fmt.Sprintf("%s==%s", name, version)
	}

	// Add user flag if specified
	if getBoolParameter(decl, "user", false) {
		args = append(args, "--user")
	}

	// Add virtualenv if specified
	if venv := getStringParameter(decl, "virtualenv", ""); venv != "" {
		pipCmd = fmt.Sprintf("%s/bin/pip", venv)
	}

	args = append(args, name)

	cmd := exec.CommandContext(ctx, pipCmd, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pip install failed: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Package %s installed", name)
	return nil
}

func (m *PipModule) upgradePackage(ctx context.Context, pipCmd string, decl *StateDeclaration, name string, result *StateResult) error {
	args := []string{"install", "--upgrade"}

	if extraIndex := getStringParameter(decl, "extra_index_url", ""); extraIndex != "" {
		args = append(args, "--extra-index-url", extraIndex)
	}

	if getBoolParameter(decl, "user", false) {
		args = append(args, "--user")
	}

	if venv := getStringParameter(decl, "virtualenv", ""); venv != "" {
		pipCmd = fmt.Sprintf("%s/bin/pip", venv)
	}

	args = append(args, name)

	cmd := exec.CommandContext(ctx, pipCmd, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pip upgrade failed: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Package %s upgraded", name)
	return nil
}

func (m *PipModule) removePackage(ctx context.Context, pipCmd, name string, result *StateResult) error {
	cmd := exec.CommandContext(ctx, pipCmd, "uninstall", "-y", name) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pip uninstall failed: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Package %s removed", name)
	return nil
}

// ============================================================================
// NPM Module - Node.js Package Management
// ============================================================================

// NpmModule manages Node.js packages via npm
type NpmModule struct {
	*BaseModule
}

// NewNpmModule creates a new npm module
func NewNpmModule() *NpmModule {
	return &NpmModule{
		BaseModule: NewBaseModule("npm", []string{"installed", "removed", "latest"}),
	}
}

// Check checks if an npm package is installed
func (m *NpmModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	name := getStringParameter(decl, "name", decl.ID)
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	global := getBoolParameter(decl, "global", false)
	path := getStringParameter(decl, "path", "")

	installed, version, err := m.isPackageInstalled(ctx, name, global, path)
	if err != nil {
		return nil, err
	}

	result.Present = installed
	if installed {
		result.CurrentState = "installed"
		result.Metadata["version"] = version
	} else {
		result.CurrentState = "removed"
	}

	switch decl.State {
	case "installed", "latest":
		result.Matches = installed
		if !installed {
			result.Diff["state"] = map[string]string{"current": "removed", "desired": decl.State}
		}
	case "removed":
		result.Matches = !installed
		if installed {
			result.Diff["state"] = map[string]string{"current": "installed", "desired": "removed"}
		}
	}

	if desiredVersion := getStringParameter(decl, "version", ""); desiredVersion != "" && installed {
		if version != desiredVersion {
			result.Matches = false
			result.Diff["version"] = map[string]string{"current": version, "desired": desiredVersion}
		}
	}

	return result, nil //nolint:nilerr // error captured in result.Error
}

// Apply applies the npm package state
func (m *NpmModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	name := getStringParameter(decl, "name", decl.ID)
	if name == "" {
		result.Error = fmt.Errorf("name parameter is required")
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Error = err
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	if checkResult.Matches {
		result.Success = true
		result.Comment = "Already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	var applyErr error
	switch decl.State {
	case "installed":
		applyErr = m.installPackage(ctx, decl, name, result)
	case "latest":
		applyErr = m.updatePackage(ctx, decl, name, result)
	case "removed":
		applyErr = m.removePackage(ctx, decl, name, result)
	}

	if applyErr != nil {
		result.Error = applyErr
	} else {
		result.Success = true
		result.Changed = true
		result.Changes = checkResult.Diff
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil //nolint:nilerr // error captured in result.Error
}

// Test tests if the package is in the desired state
func (m *NpmModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil //nolint:nilerr // intentional
}

func (m *NpmModule) isPackageInstalled(ctx context.Context, name string, global bool, path string) (installed bool, version string, err error) {
	args := []string{"list", "--json"}
	if global {
		args = append(args, "-g")
	}
	args = append(args, name)

	cmd := exec.CommandContext(ctx, "npm", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	if path != "" {
		cmd.Dir = path
	}

	output, err := cmd.Output()
	if err != nil {
		return false, "", nil //nolint:nilerr // npm list returns error when package not installed
	}

	// Simple check - if the package name appears in the output, it's installed
	if strings.Contains(string(output), fmt.Sprintf("%q", name)) {
		// Try to extract version
		// npm list --json output includes version info
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, `"version"`) {
				// Extract version from JSON
				parts := strings.Split(line, `"`)
				for i, p := range parts {
					if p == "version" && i+2 < len(parts) {
						return true, parts[i+2], nil //nolint:nilerr // returning installation status, no error
					}
				}
			}
		}
		return true, "", nil //nolint:nilerr // package found but version not parsed
	}

	return false, "", nil //nolint:nilerr // package not found is a valid state
}

func (m *NpmModule) installPackage(ctx context.Context, decl *StateDeclaration, name string, result *StateResult) error {
	args := []string{"install"}

	if getBoolParameter(decl, "global", false) {
		args = append(args, "-g")
	}

	// Add version if specified
	if version := getStringParameter(decl, "version", ""); version != "" {
		name = fmt.Sprintf("%s@%s", name, version)
	}

	// Production only
	if getBoolParameter(decl, "production", false) {
		args = append(args, "--production")
	}

	// Save to dependencies
	if getBoolParameter(decl, "save", false) {
		args = append(args, "--save")
	}

	// Save to dev dependencies
	if getBoolParameter(decl, "save_dev", false) {
		args = append(args, "--save-dev")
	}

	args = append(args, name)

	cmd := exec.CommandContext(ctx, "npm", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	if path := getStringParameter(decl, "path", ""); path != "" {
		cmd.Dir = path
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("npm install failed: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Package %s installed", name)
	return nil
}

func (m *NpmModule) updatePackage(ctx context.Context, decl *StateDeclaration, name string, result *StateResult) error {
	args := []string{"update"}

	if getBoolParameter(decl, "global", false) {
		args = append(args, "-g")
	}

	args = append(args, name)

	cmd := exec.CommandContext(ctx, "npm", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	if path := getStringParameter(decl, "path", ""); path != "" {
		cmd.Dir = path
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("npm update failed: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Package %s updated", name)
	return nil
}

func (m *NpmModule) removePackage(ctx context.Context, decl *StateDeclaration, name string, result *StateResult) error {
	args := []string{"uninstall"}

	if getBoolParameter(decl, "global", false) {
		args = append(args, "-g")
	}

	args = append(args, name)

	cmd := exec.CommandContext(ctx, "npm", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	if path := getStringParameter(decl, "path", ""); path != "" {
		cmd.Dir = path
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("npm uninstall failed: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Package %s removed", name)
	return nil
}

// ============================================================================
// Gem Module - Ruby Package Management
// ============================================================================

// GemModule manages Ruby gems
type GemModule struct {
	*BaseModule
}

// NewGemModule creates a new gem module
func NewGemModule() *GemModule {
	return &GemModule{
		BaseModule: NewBaseModule("gem", []string{"installed", "removed", "latest"}),
	}
}

// Check checks if a Ruby gem is installed
func (m *GemModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	name := getStringParameter(decl, "name", decl.ID)
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	gemCmd := getStringParameter(decl, "executable", "gem")
	installed, version, err := m.isGemInstalled(ctx, gemCmd, name)
	if err != nil {
		return nil, err
	}

	result.Present = installed
	if installed {
		result.CurrentState = "installed"
		result.Metadata["version"] = version
	} else {
		result.CurrentState = "removed"
	}

	switch decl.State {
	case "installed", "latest":
		result.Matches = installed
		if !installed {
			result.Diff["state"] = map[string]string{"current": "removed", "desired": decl.State}
		}
	case "removed":
		result.Matches = !installed
		if installed {
			result.Diff["state"] = map[string]string{"current": "installed", "desired": "removed"}
		}
	}

	if desiredVersion := getStringParameter(decl, "version", ""); desiredVersion != "" && installed {
		if version != desiredVersion {
			result.Matches = false
			result.Diff["version"] = map[string]string{"current": version, "desired": desiredVersion}
		}
	}

	return result, nil //nolint:nilerr // error captured in result.Error
}

// Apply applies the gem state
func (m *GemModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	name := getStringParameter(decl, "name", decl.ID)
	if name == "" {
		result.Error = fmt.Errorf("name parameter is required")
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Error = err
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	if checkResult.Matches {
		result.Success = true
		result.Comment = "Already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	gemCmd := getStringParameter(decl, "executable", "gem")
	var applyErr error

	switch decl.State {
	case "installed":
		applyErr = m.installGem(ctx, gemCmd, decl, name, result)
	case "latest":
		applyErr = m.updateGem(ctx, gemCmd, name, result)
	case "removed":
		applyErr = m.removeGem(ctx, gemCmd, name, result)
	}

	if applyErr != nil {
		result.Error = applyErr
	} else {
		result.Success = true
		result.Changed = true
		result.Changes = checkResult.Diff
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil //nolint:nilerr // error captured in result.Error
}

// Test tests if the gem is in the desired state
func (m *GemModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil //nolint:nilerr // err already checked above
}

func (m *GemModule) isGemInstalled(ctx context.Context, gemCmd, name string) (installed bool, version string, err error) {
	cmd := exec.CommandContext(ctx, gemCmd, "list", "-i", name) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.Output()
	if err != nil {
		return false, "", nil //nolint:nilerr // gem list returns error when gem not installed
	}

	if strings.TrimSpace(string(output)) == "true" {
		// Get version
		versionCmd := exec.CommandContext(ctx, gemCmd, "list", name) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
		versionOutput, err := versionCmd.Output()
		if err == nil {
			// Parse "name (version)" format
			lines := strings.Split(string(versionOutput), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, name+" ") {
					// Extract version from parentheses
					start := strings.Index(line, "(")
					end := strings.Index(line, ")")
					if start != -1 && end != -1 && end > start {
						version := line[start+1 : end]
						// Take first version if multiple
						if idx := strings.Index(version, ","); idx != -1 {
							version = version[:idx]
						}
						return true, strings.TrimSpace(version), nil //nolint:nilerr // returning installation status, no error
					}
				}
			}
		}
		return true, "", nil //nolint:nilerr // gem found but version not parsed
	}

	return false, "", nil //nolint:nilerr // gem not found is a valid state
}

func (m *GemModule) installGem(ctx context.Context, gemCmd string, decl *StateDeclaration, name string, result *StateResult) error {
	args := []string{"install"}

	// Add version if specified
	if version := getStringParameter(decl, "version", ""); version != "" {
		args = append(args, "-v", version)
	}

	// User install
	if getBoolParameter(decl, "user", false) {
		args = append(args, "--user-install")
	}

	// No documentation
	if getBoolParameter(decl, "no_doc", true) {
		args = append(args, "--no-document")
	}

	// Custom source
	if source := getStringParameter(decl, "source", ""); source != "" {
		args = append(args, "--source", source)
	}

	args = append(args, name)

	cmd := exec.CommandContext(ctx, gemCmd, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gem install failed: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Gem %s installed", name)
	return nil
}

func (m *GemModule) updateGem(ctx context.Context, gemCmd, name string, result *StateResult) error {
	cmd := exec.CommandContext(ctx, gemCmd, "update", name) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gem update failed: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Gem %s updated", name)
	return nil
}

func (m *GemModule) removeGem(ctx context.Context, gemCmd, name string, result *StateResult) error {
	cmd := exec.CommandContext(ctx, gemCmd, "uninstall", "-x", name) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gem uninstall failed: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Gem %s removed", name)
	return nil
}

// ============================================================================
// UFW Module - Ubuntu Firewall
// ============================================================================

// UfwModule manages Ubuntu's Uncomplicated Firewall
type UfwModule struct {
	*BaseModule
}

// NewUfwModule creates a new ufw module
func NewUfwModule() *UfwModule {
	return &UfwModule{
		BaseModule: NewBaseModule("ufw", []string{"enabled", "disabled", "allow", "deny", "reject", "absent"}),
	}
}

// Check checks the UFW state
func (m *UfwModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("ufw module is only supported on Linux")
	}

	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	// Check if UFW is installed
	if _, err := exec.LookPath("ufw"); err != nil {
		return nil, fmt.Errorf("ufw is not installed")
	}

	// For enable/disable states, check UFW status
	if decl.State == "enabled" || decl.State == "disabled" {
		enabled, err := m.isUfwEnabled(ctx)
		if err != nil {
			return nil, err
		}

		result.Present = true
		if enabled {
			result.CurrentState = "enabled"
		} else {
			result.CurrentState = "disabled"
		}

		result.Matches = result.CurrentState == decl.State
		if !result.Matches {
			result.Diff["state"] = map[string]string{"current": result.CurrentState, "desired": decl.State}
		}
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	// For rule states (allow, deny, reject, absent), check if rule exists
	rule := m.buildRuleSpec(decl)
	exists, err := m.ruleExists(ctx, rule)
	if err != nil {
		return nil, err
	}

	result.Present = exists
	if exists {
		result.CurrentState = "present"
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "allow", "deny", "reject":
		result.Matches = exists
		if !exists {
			result.Diff["rule"] = map[string]string{"current": "absent", "desired": decl.State}
		}
	case "absent":
		result.Matches = !exists
		if exists {
			result.Diff["rule"] = map[string]string{"current": "present", "desired": "absent"}
		}
	}

	return result, nil //nolint:nilerr // error captured in result.Error
}

// Apply applies the UFW state
func (m *UfwModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	if runtime.GOOS != "linux" {
		result.Error = fmt.Errorf("ufw module is only supported on Linux")
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Error = err
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	if checkResult.Matches {
		result.Success = true
		result.Comment = "Already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	var applyErr error
	switch decl.State {
	case "enabled":
		applyErr = m.enableUfw(ctx, result)
	case "disabled":
		applyErr = m.disableUfw(ctx, result)
	case "allow", "deny", "reject":
		applyErr = m.addRule(ctx, decl, result)
	case "absent":
		applyErr = m.deleteRule(ctx, decl, result)
	}

	if applyErr != nil {
		result.Error = applyErr
	} else {
		result.Success = true
		result.Changed = true
		result.Changes = checkResult.Diff
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil //nolint:nilerr // error captured in result.Error
}

// Test tests if UFW is in the desired state
func (m *UfwModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil //nolint:nilerr // intentional
}

func (m *UfwModule) isUfwEnabled(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "ufw", "status") // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.Contains(string(output), "Status: active"), nil //nolint:nilerr // returning status check result, no error
}

func (m *UfwModule) buildRuleSpec(decl *StateDeclaration) string {
	var parts []string

	// Port
	if port := getStringParameter(decl, "port", ""); port != "" {
		parts = append(parts, port)
	}

	// Protocol
	if proto := getStringParameter(decl, "proto", ""); proto != "" {
		parts = append(parts, "/"+proto)
	}

	// From
	if from := getStringParameter(decl, "from", ""); from != "" {
		parts = append(parts, "from", from)
	}

	// To
	if to := getStringParameter(decl, "to", ""); to != "" {
		parts = append(parts, "to", to)
	}

	return strings.Join(parts, " ")
}

func (m *UfwModule) ruleExists(ctx context.Context, rule string) (bool, error) {
	cmd := exec.CommandContext(ctx, "ufw", "status", "numbered") // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	// Simple check - this is a heuristic
	return strings.Contains(string(output), rule), nil //nolint:nilerr // returning existence check result, no error
}

func (m *UfwModule) enableUfw(ctx context.Context, result *StateResult) error {
	cmd := exec.CommandContext(ctx, "ufw", "--force", "enable") // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable ufw: %w (output: %s)", err, string(output))
	}
	result.Comment = "UFW enabled"
	return nil
}

func (m *UfwModule) disableUfw(ctx context.Context, result *StateResult) error {
	cmd := exec.CommandContext(ctx, "ufw", "--force", "disable") // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to disable ufw: %w (output: %s)", err, string(output))
	}
	result.Comment = "UFW disabled"
	return nil
}

func (m *UfwModule) addRule(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	args := []string{decl.State} // allow, deny, or reject

	// Direction (in/out)
	if direction := getStringParameter(decl, "direction", ""); direction != "" {
		args = append(args, direction)
	}

	// From
	if from := getStringParameter(decl, "from", ""); from != "" {
		args = append(args, "from", from)
	}

	// To
	if to := getStringParameter(decl, "to", ""); to != "" {
		args = append(args, "to", to)
	}

	// Port
	if port := getStringParameter(decl, "port", ""); port != "" {
		args = append(args, "port", port)
	}

	// Protocol
	if proto := getStringParameter(decl, "proto", ""); proto != "" {
		args = append(args, "proto", proto)
	}

	// Comment
	if comment := getStringParameter(decl, "comment", ""); comment != "" {
		args = append(args, "comment", comment)
	}

	cmd := exec.CommandContext(ctx, "ufw", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add ufw rule: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("UFW rule added: %s", strings.Join(args, " "))
	return nil
}

func (m *UfwModule) deleteRule(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	args := []string{"delete"}

	// We need to reconstruct the rule to delete
	if port := getStringParameter(decl, "port", ""); port != "" {
		args = append(args, "allow")
		if from := getStringParameter(decl, "from", ""); from != "" {
			args = append(args, "from", from)
		}
		args = append(args, "port", port)
		if proto := getStringParameter(decl, "proto", ""); proto != "" {
			args = append(args, "proto", proto)
		}
	}

	cmd := exec.CommandContext(ctx, "ufw", args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete ufw rule: %w (output: %s)", err, string(output))
	}

	result.Comment = "UFW rule deleted"
	return nil
}

// ============================================================================
// Alternatives Module - update-alternatives
// ============================================================================

// AlternativesModule manages Linux alternatives system
type AlternativesModule struct {
	*BaseModule
}

// NewAlternativesModule creates a new alternatives module
func NewAlternativesModule() *AlternativesModule {
	return &AlternativesModule{
		BaseModule: NewBaseModule("alternatives", []string{"set", "auto"}),
	}
}

// Check checks the alternatives state
func (m *AlternativesModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("alternatives module is only supported on Linux")
	}

	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	// Get current alternative
	current, isAuto, err := m.getCurrentAlternative(ctx, name)
	if err != nil {
		return nil, err
	}

	result.Present = current != ""
	result.CurrentState = current
	result.Metadata["auto"] = isAuto
	result.Metadata["current"] = current

	switch decl.State {
	case "set":
		path := getStringParameter(decl, "path", "")
		if path == "" {
			return nil, fmt.Errorf("path parameter is required for set state")
		}
		result.Matches = current == path
		if !result.Matches {
			result.Diff["path"] = map[string]string{"current": current, "desired": path}
		}
	case "auto":
		result.Matches = isAuto
		if !result.Matches {
			result.Diff["mode"] = map[string]string{"current": "manual", "desired": "auto"}
		}
	}

	return result, nil //nolint:nilerr // error captured in result.Error
}

// Apply applies the alternatives state
func (m *AlternativesModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	if runtime.GOOS != "linux" {
		result.Error = fmt.Errorf("alternatives module is only supported on Linux")
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		result.Error = fmt.Errorf("name parameter is required")
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Error = err
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	if checkResult.Matches {
		result.Success = true
		result.Comment = "Already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil //nolint:nilerr // error captured in result.Error
	}

	var applyErr error
	switch decl.State {
	case "set":
		path := getStringParameter(decl, "path", "")
		applyErr = m.setAlternative(ctx, name, path, result)
	case "auto":
		applyErr = m.setAuto(ctx, name, result)
	}

	if applyErr != nil {
		result.Error = applyErr
	} else {
		result.Success = true
		result.Changed = true
		result.Changes = checkResult.Diff
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil //nolint:nilerr // error captured in result.Error
}

// Test tests if alternatives is in the desired state
func (m *AlternativesModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil //nolint:nilerr // err already checked above
}

func (m *AlternativesModule) getCurrentAlternative(ctx context.Context, name string) (current string, isAuto bool, err error) {
	cmd := exec.CommandContext(ctx, "update-alternatives", "--display", name) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.Output()
	if err != nil {
		return "", false, nil //nolint:nilerr // alternative not configured returns error, which is a valid state
	}

	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		if strings.Contains(line, "link currently points to") {
			parts := strings.Split(line, " ")
			if len(parts) > 0 {
				current = parts[len(parts)-1]
			}
		}
		if strings.Contains(line, "auto mode") {
			isAuto = true
		}
	}

	return current, isAuto, nil //nolint:nilerr // returning parsed alternative info, no error
}

func (m *AlternativesModule) setAlternative(ctx context.Context, name, path string, result *StateResult) error {
	cmd := exec.CommandContext(ctx, "update-alternatives", "--set", name, path) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set alternative: %w (output: %s)", err, string(output))
	}
	result.Comment = fmt.Sprintf("Alternative %s set to %s", name, path)
	return nil
}

func (m *AlternativesModule) setAuto(ctx context.Context, name string, result *StateResult) error {
	cmd := exec.CommandContext(ctx, "update-alternatives", "--auto", name) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set auto mode: %w (output: %s)", err, string(output))
	}
	result.Comment = fmt.Sprintf("Alternative %s set to auto mode", name)
	return nil
}

func init() {
	_ = RegisterModule(NewPipModule())
	_ = RegisterModule(NewNpmModule())
	_ = RegisterModule(NewGemModule())
	_ = RegisterModule(NewUfwModule())
	_ = RegisterModule(NewAlternativesModule())
}
