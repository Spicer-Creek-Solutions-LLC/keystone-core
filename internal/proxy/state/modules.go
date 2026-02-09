// Package state provides proxy state modules for different protocols.
package state

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/shawnbutts/keystone-core/internal/proxy"
)

// ProxyModule is the interface for proxy state modules.
type ProxyModule interface {
	// Name returns the module name
	Name() string

	// Execute runs the module on a device
	Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error)

	// Check performs a dry-run check
	Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error)
}

// ModuleContext provides context for module execution.
type ModuleContext struct {
	Device     *proxy.ProxiedDevice
	Executor   proxy.ProxiedExecutor
	Parameters map[string]interface{}
	DryRun     bool
}

// ExecuteCommand executes a command on the device via the executor.
func (mctx *ModuleContext) ExecuteCommand(ctx context.Context, command string) (*proxy.ProxiedExecuteResult, error) {
	return mctx.Executor.Execute(ctx, &proxy.ProxiedExecuteRequest{
		DeviceID: mctx.Device.ID,
		Command:  command,
	})
}

// ModuleResult is the result of a module execution.
type ModuleResult struct {
	Changed bool
	Comment string
	Details map[string]interface{}
}

// ProxyModuleRegistry manages proxy state modules.
type ProxyModuleRegistry struct {
	modules map[string]ProxyModule
	mu      sync.RWMutex
}

// NewProxyModuleRegistry creates a new module registry.
func NewProxyModuleRegistry() *ProxyModuleRegistry {
	return &ProxyModuleRegistry{
		modules: make(map[string]ProxyModule),
	}
}

// Register registers a module.
func (r *ProxyModuleRegistry) Register(name string, module ProxyModule) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modules[name] = module
}

// Get retrieves a module by name.
func (r *ProxyModuleRegistry) Get(name string) (ProxyModule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, ok := r.modules[name]
	if !ok {
		return nil, fmt.Errorf("module '%s' not found", name)
	}
	return module, nil
}

// List returns all registered module names.
func (r *ProxyModuleRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.modules))
	for name := range r.modules {
		names = append(names, name)
	}
	return names
}

// BaseProxyModule provides common functionality for proxy modules.
type BaseProxyModule struct {
	name string
}

// Name returns the module name.
func (m *BaseProxyModule) Name() string {
	return m.name
}

// GetString gets a string parameter.
func (m *BaseProxyModule) GetString(params map[string]interface{}, key string) (string, bool) {
	if v, ok := params[key]; ok {
		if s, ok := v.(string); ok {
			return s, true
		}
	}
	return "", false
}

// GetInt gets an integer parameter.
func (m *BaseProxyModule) GetInt(params map[string]interface{}, key string) (int, bool) {
	if v, ok := params[key]; ok {
		switch n := v.(type) {
		case int:
			return n, true
		case int64:
			return int(n), true
		case float64:
			return int(n), true
		}
	}
	return 0, false
}

// GetBool gets a boolean parameter.
func (m *BaseProxyModule) GetBool(params map[string]interface{}, key string) (value, ok bool) {
	if v, ok := params[key]; ok {
		if b, ok := v.(bool); ok {
			return b, true
		}
	}
	return false, false
}

// GetStringSlice gets a string slice parameter.
func (m *BaseProxyModule) GetStringSlice(params map[string]interface{}, key string) ([]string, bool) {
	if v, ok := params[key]; ok {
		switch s := v.(type) {
		case []string:
			return s, true
		case []interface{}:
			result := make([]string, 0, len(s))
			for _, item := range s {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
			return result, true
		}
	}
	return nil, false
}

// =============================================================================
// SSH Modules
// =============================================================================

// SSHFileModule manages files via SSH.
type SSHFileModule struct {
	BaseProxyModule
}

// NewSSHFileModule creates a new SSH file module.
func NewSSHFileModule() *SSHFileModule {
	return &SSHFileModule{BaseProxyModule{name: "ssh_file"}}
}

// Execute runs the file module.
func (m *SSHFileModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	path, _ := m.GetString(mctx.Parameters, "path")
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	state, _ := m.GetString(mctx.Parameters, "state")
	if state == "" {
		state = "present"
	}

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	switch state {
	case "present", "file":
		return m.ensureFile(ctx, mctx, path, result)
	case "absent":
		return m.removeFile(ctx, mctx, path, result)
	case "directory":
		return m.ensureDirectory(ctx, mctx, path, result)
	default:
		return nil, fmt.Errorf("unknown state: %s", state)
	}
}

// Check performs a dry-run check.
func (m *SSHFileModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

func (m *SSHFileModule) ensureFile(ctx context.Context, mctx ModuleContext, path string, result *ModuleResult) (*ModuleResult, error) {
	// Check if file exists
	checkResult, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("test -f %s && echo exists", path))
	if err != nil {
		return nil, err
	}

	exists := strings.TrimSpace(string(checkResult.Stdout)) == "exists"

	content, hasContent := m.GetString(mctx.Parameters, "content")
	source, hasSource := m.GetString(mctx.Parameters, "source")

	if !exists {
		result.Changed = true
		result.Comment = fmt.Sprintf("File %s will be created", path)

		if mctx.DryRun {
			return result, nil
		}

		switch {
		case hasContent:
			// Create file with content
			cmd := fmt.Sprintf("cat > %s << 'EOFKSCORE'\n%s\nEOFKSCORE", path, content)
			if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
				return nil, fmt.Errorf("failed to create file: %w", err)
			}
		case hasSource:
			// Copy from source
			cmd := fmt.Sprintf("cp %s %s", source, path)
			if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
				return nil, fmt.Errorf("failed to copy file: %w", err)
			}
		default:
			// Create empty file
			cmd := fmt.Sprintf("touch %s", path)
			if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
				return nil, fmt.Errorf("failed to create file: %w", err)
			}
		}

		result.Comment = fmt.Sprintf("File %s created", path)
	} else {
		result.Comment = fmt.Sprintf("File %s already exists", path)

		// Check content if provided
		if hasContent {
			catResult, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("cat %s", path))
			if err == nil && string(catResult.Stdout) != content {
				result.Changed = true
				result.Comment = fmt.Sprintf("File %s will be updated", path)

				if !mctx.DryRun {
					cmd := fmt.Sprintf("cat > %s << 'EOFKSCORE'\n%s\nEOFKSCORE", path, content)
					if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
						return nil, fmt.Errorf("failed to update file: %w", err)
					}
					result.Comment = fmt.Sprintf("File %s updated", path)
				}
			}
		}
	}

	// Handle mode
	if mode, ok := m.GetString(mctx.Parameters, "mode"); ok {
		if !mctx.DryRun {
			cmd := fmt.Sprintf("chmod %s %s", mode, path)
			if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
				return nil, fmt.Errorf("failed to set mode: %w", err)
			}
		}
	}

	// Handle owner
	if owner, ok := m.GetString(mctx.Parameters, "owner"); ok {
		if !mctx.DryRun {
			cmd := fmt.Sprintf("chown %s %s", owner, path)
			if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
				return nil, fmt.Errorf("failed to set owner: %w", err)
			}
		}
	}

	return result, nil
}

func (m *SSHFileModule) removeFile(ctx context.Context, mctx ModuleContext, path string, result *ModuleResult) (*ModuleResult, error) {
	// Check if file exists
	checkResult, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("test -e %s && echo exists", path))
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(string(checkResult.Stdout)) == "exists" {
		result.Changed = true
		result.Comment = fmt.Sprintf("File %s will be removed", path)

		if !mctx.DryRun {
			cmd := fmt.Sprintf("rm -rf %s", path)
			if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
				return nil, fmt.Errorf("failed to remove: %w", err)
			}
			result.Comment = fmt.Sprintf("File %s removed", path)
		}
	} else {
		result.Comment = fmt.Sprintf("File %s already absent", path)
	}

	return result, nil
}

func (m *SSHFileModule) ensureDirectory(ctx context.Context, mctx ModuleContext, path string, result *ModuleResult) (*ModuleResult, error) {
	// Check if directory exists
	checkResult, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("test -d %s && echo exists", path))
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(string(checkResult.Stdout)) != "exists" {
		result.Changed = true
		result.Comment = fmt.Sprintf("Directory %s will be created", path)

		if !mctx.DryRun {
			cmd := fmt.Sprintf("mkdir -p %s", path)
			if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
				return nil, fmt.Errorf("failed to create directory: %w", err)
			}
			result.Comment = fmt.Sprintf("Directory %s created", path)
		}
	} else {
		result.Comment = fmt.Sprintf("Directory %s already exists", path)
	}

	return result, nil
}

// SSHCmdModule executes commands via SSH.
type SSHCmdModule struct {
	BaseProxyModule
}

// NewSSHCmdModule creates a new SSH command module.
func NewSSHCmdModule() *SSHCmdModule {
	return &SSHCmdModule{BaseProxyModule{name: "ssh_cmd"}}
}

// Execute runs the command module.
func (m *SSHCmdModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	cmd, _ := m.GetString(mctx.Parameters, "cmd")
	if cmd == "" {
		return nil, fmt.Errorf("cmd is required")
	}

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	// Check creates condition
	if creates, ok := m.GetString(mctx.Parameters, "creates"); ok {
		checkResult, _ := mctx.ExecuteCommand(ctx, fmt.Sprintf("test -e %s && echo exists", creates))
		if strings.TrimSpace(string(checkResult.Stdout)) == "exists" {
			result.Comment = fmt.Sprintf("Skipped - %s already exists", creates)
			return result, nil
		}
	}

	// Check unless condition
	if unless, ok := m.GetString(mctx.Parameters, "unless"); ok {
		checkResult, _ := mctx.ExecuteCommand(ctx, unless)
		if checkResult.ExitCode == 0 {
			result.Comment = "Skipped - unless condition met"
			return result, nil
		}
	}

	result.Changed = true

	if mctx.DryRun {
		result.Comment = fmt.Sprintf("Would run: %s", cmd)
		return result, nil
	}

	// Execute the command
	execResult, err := mctx.ExecuteCommand(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("command failed: %w", err)
	}

	result.Details["stdout"] = execResult.Stdout
	result.Details["stderr"] = execResult.Stderr
	result.Details["exit_code"] = execResult.ExitCode

	if execResult.ExitCode != 0 {
		return nil, fmt.Errorf("command exited with code %d: %s", execResult.ExitCode, execResult.Stderr)
	}

	result.Comment = "Command executed successfully"
	return result, nil
}

// Check performs a dry-run check.
func (m *SSHCmdModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// SSHServiceModule manages services via SSH.
type SSHServiceModule struct {
	BaseProxyModule
}

// NewSSHServiceModule creates a new SSH service module.
func NewSSHServiceModule() *SSHServiceModule {
	return &SSHServiceModule{BaseProxyModule{name: "ssh_service"}}
}

// Execute runs the service module.
func (m *SSHServiceModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	name, _ := m.GetString(mctx.Parameters, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	state, _ := m.GetString(mctx.Parameters, "state")
	enabled, hasEnabled := m.GetBool(mctx.Parameters, "enabled")

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	// Check current state
	statusResult, _ := mctx.ExecuteCommand(ctx, fmt.Sprintf("systemctl is-active %s 2>/dev/null || service %s status 2>/dev/null", name, name))
	isRunning := strings.Contains(string(statusResult.Stdout), "active") || strings.Contains(string(statusResult.Stdout), "running")

	// Handle state
	switch state {
	case "running", "started":
		if !isRunning {
			result.Changed = true
			if mctx.DryRun {
				result.Comment = fmt.Sprintf("Service %s would be started", name)
			} else {
				_, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("systemctl start %s 2>/dev/null || service %s start", name, name))
				if err != nil {
					return nil, fmt.Errorf("failed to start service: %w", err)
				}
				result.Comment = fmt.Sprintf("Service %s started", name)
			}
		} else {
			result.Comment = fmt.Sprintf("Service %s is already running", name)
		}

	case "stopped":
		if isRunning {
			result.Changed = true
			if mctx.DryRun {
				result.Comment = fmt.Sprintf("Service %s would be stopped", name)
			} else {
				_, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("systemctl stop %s 2>/dev/null || service %s stop", name, name))
				if err != nil {
					return nil, fmt.Errorf("failed to stop service: %w", err)
				}
				result.Comment = fmt.Sprintf("Service %s stopped", name)
			}
		} else {
			result.Comment = fmt.Sprintf("Service %s is already stopped", name)
		}

	case "restarted":
		result.Changed = true
		if mctx.DryRun {
			result.Comment = fmt.Sprintf("Service %s would be restarted", name)
		} else {
			_, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("systemctl restart %s 2>/dev/null || service %s restart", name, name))
			if err != nil {
				return nil, fmt.Errorf("failed to restart service: %w", err)
			}
			result.Comment = fmt.Sprintf("Service %s restarted", name)
		}

	case "reloaded":
		result.Changed = true
		if mctx.DryRun {
			result.Comment = fmt.Sprintf("Service %s would be reloaded", name)
		} else {
			_, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("systemctl reload %s 2>/dev/null || service %s reload", name, name))
			if err != nil {
				return nil, fmt.Errorf("failed to reload service: %w", err)
			}
			result.Comment = fmt.Sprintf("Service %s reloaded", name)
		}
	}

	// Handle enabled
	if hasEnabled {
		enabledResult, _ := mctx.ExecuteCommand(ctx, fmt.Sprintf("systemctl is-enabled %s 2>/dev/null", name))
		isEnabled := strings.TrimSpace(string(enabledResult.Stdout)) == "enabled"

		if enabled && !isEnabled {
			result.Changed = true
			if !mctx.DryRun {
				_, _ = mctx.ExecuteCommand(ctx, fmt.Sprintf("systemctl enable %s 2>/dev/null", name)) //nolint:errcheck // best-effort enable
			}
		} else if !enabled && isEnabled {
			result.Changed = true
			if !mctx.DryRun {
				_, _ = mctx.ExecuteCommand(ctx, fmt.Sprintf("systemctl disable %s 2>/dev/null", name)) //nolint:errcheck // best-effort disable
			}
		}
	}

	return result, nil
}

// Check performs a dry-run check.
func (m *SSHServiceModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// SSHPackageModule manages packages via SSH.
type SSHPackageModule struct {
	BaseProxyModule
}

// NewSSHPackageModule creates a new SSH package module.
func NewSSHPackageModule() *SSHPackageModule {
	return &SSHPackageModule{BaseProxyModule{name: "ssh_package"}}
}

// Execute runs the package module.
func (m *SSHPackageModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	name, _ := m.GetString(mctx.Parameters, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	state, _ := m.GetString(mctx.Parameters, "state")
	if state == "" {
		state = "installed"
	}

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	// Detect package manager
	pmResult, _ := mctx.ExecuteCommand(ctx, "which apt-get dnf yum pacman apk 2>/dev/null | head -1")
	pm := strings.TrimSpace(string(pmResult.Stdout))

	var installCmd, removeCmd, checkCmd string
	switch {
	case strings.Contains(pm, "apt"):
		installCmd = fmt.Sprintf("DEBIAN_FRONTEND=noninteractive apt-get install -y %s", name)
		removeCmd = fmt.Sprintf("apt-get remove -y %s", name)
		checkCmd = fmt.Sprintf("dpkg -l %s 2>/dev/null | grep -q '^ii'", name)
	case strings.Contains(pm, "dnf"):
		installCmd = fmt.Sprintf("dnf install -y %s", name)
		removeCmd = fmt.Sprintf("dnf remove -y %s", name)
		checkCmd = fmt.Sprintf("rpm -q %s", name)
	case strings.Contains(pm, "yum"):
		installCmd = fmt.Sprintf("yum install -y %s", name)
		removeCmd = fmt.Sprintf("yum remove -y %s", name)
		checkCmd = fmt.Sprintf("rpm -q %s", name)
	case strings.Contains(pm, "pacman"):
		installCmd = fmt.Sprintf("pacman -S --noconfirm %s", name)
		removeCmd = fmt.Sprintf("pacman -R --noconfirm %s", name)
		checkCmd = fmt.Sprintf("pacman -Q %s", name)
	case strings.Contains(pm, "apk"):
		installCmd = fmt.Sprintf("apk add %s", name)
		removeCmd = fmt.Sprintf("apk del %s", name)
		checkCmd = fmt.Sprintf("apk info -e %s", name)
	default:
		return nil, fmt.Errorf("unknown package manager")
	}

	// Check if package is installed
	checkResult, _ := mctx.ExecuteCommand(ctx, checkCmd)
	isInstalled := checkResult.ExitCode == 0

	switch state {
	case "installed", "present", "latest":
		if !isInstalled {
			result.Changed = true
			if mctx.DryRun {
				result.Comment = fmt.Sprintf("Package %s would be installed", name)
			} else {
				_, err := mctx.ExecuteCommand(ctx, installCmd)
				if err != nil {
					return nil, fmt.Errorf("failed to install package: %w", err)
				}
				result.Comment = fmt.Sprintf("Package %s installed", name)
			}
		} else {
			result.Comment = fmt.Sprintf("Package %s is already installed", name)
		}

	case "removed", "absent":
		if isInstalled {
			result.Changed = true
			if mctx.DryRun {
				result.Comment = fmt.Sprintf("Package %s would be removed", name)
			} else {
				_, err := mctx.ExecuteCommand(ctx, removeCmd)
				if err != nil {
					return nil, fmt.Errorf("failed to remove package: %w", err)
				}
				result.Comment = fmt.Sprintf("Package %s removed", name)
			}
		} else {
			result.Comment = fmt.Sprintf("Package %s is already absent", name)
		}
	}

	return result, nil
}

// Check performs a dry-run check.
func (m *SSHPackageModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// SSHUserModule manages users via SSH.
type SSHUserModule struct {
	BaseProxyModule
}

// NewSSHUserModule creates a new SSH user module.
func NewSSHUserModule() *SSHUserModule {
	return &SSHUserModule{BaseProxyModule{name: "ssh_user"}}
}

// Execute runs the user module.
func (m *SSHUserModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	name, _ := m.GetString(mctx.Parameters, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	state, _ := m.GetString(mctx.Parameters, "state")
	if state == "" {
		state = "present"
	}

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	// Check if user exists
	checkResult, _ := mctx.ExecuteCommand(ctx, fmt.Sprintf("id %s 2>/dev/null", name))
	exists := checkResult.ExitCode == 0

	switch state {
	case "present":
		if !exists {
			result.Changed = true
			if mctx.DryRun {
				result.Comment = fmt.Sprintf("User %s would be created", name)
			} else {
				cmd := fmt.Sprintf("useradd %s", name)
				if shell, ok := m.GetString(mctx.Parameters, "shell"); ok {
					cmd += fmt.Sprintf(" -s %s", shell)
				}
				if home, ok := m.GetString(mctx.Parameters, "home"); ok {
					cmd += fmt.Sprintf(" -d %s", home)
				}
				if groups, ok := m.GetStringSlice(mctx.Parameters, "groups"); ok {
					cmd += fmt.Sprintf(" -G %s", strings.Join(groups, ","))
				}
				if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
					return nil, fmt.Errorf("failed to create user: %w", err)
				}
				result.Comment = fmt.Sprintf("User %s created", name)
			}
		} else {
			result.Comment = fmt.Sprintf("User %s already exists", name)
		}

	case "absent":
		if exists {
			result.Changed = true
			if mctx.DryRun {
				result.Comment = fmt.Sprintf("User %s would be removed", name)
			} else {
				if _, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("userdel -r %s", name)); err != nil {
					return nil, fmt.Errorf("failed to remove user: %w", err)
				}
				result.Comment = fmt.Sprintf("User %s removed", name)
			}
		} else {
			result.Comment = fmt.Sprintf("User %s already absent", name)
		}
	}

	return result, nil
}

// Check performs a dry-run check.
func (m *SSHUserModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// SSHGroupModule manages groups via SSH.
type SSHGroupModule struct {
	BaseProxyModule
}

// NewSSHGroupModule creates a new SSH group module.
func NewSSHGroupModule() *SSHGroupModule {
	return &SSHGroupModule{BaseProxyModule{name: "ssh_group"}}
}

// Execute runs the group module.
func (m *SSHGroupModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	name, _ := m.GetString(mctx.Parameters, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	state, _ := m.GetString(mctx.Parameters, "state")
	if state == "" {
		state = "present"
	}

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	// Check if group exists
	checkResult, _ := mctx.ExecuteCommand(ctx, fmt.Sprintf("getent group %s", name))
	exists := checkResult.ExitCode == 0

	switch state {
	case "present":
		if !exists {
			result.Changed = true
			if mctx.DryRun {
				result.Comment = fmt.Sprintf("Group %s would be created", name)
			} else {
				cmd := fmt.Sprintf("groupadd %s", name)
				if gid, ok := m.GetInt(mctx.Parameters, "gid"); ok {
					cmd += fmt.Sprintf(" -g %d", gid)
				}
				if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
					return nil, fmt.Errorf("failed to create group: %w", err)
				}
				result.Comment = fmt.Sprintf("Group %s created", name)
			}
		} else {
			result.Comment = fmt.Sprintf("Group %s already exists", name)
		}

	case "absent":
		if exists {
			result.Changed = true
			if mctx.DryRun {
				result.Comment = fmt.Sprintf("Group %s would be removed", name)
			} else {
				if _, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("groupdel %s", name)); err != nil {
					return nil, fmt.Errorf("failed to remove group: %w", err)
				}
				result.Comment = fmt.Sprintf("Group %s removed", name)
			}
		} else {
			result.Comment = fmt.Sprintf("Group %s already absent", name)
		}
	}

	return result, nil
}

// Check performs a dry-run check.
func (m *SSHGroupModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// =============================================================================
// SNMP Modules
// =============================================================================

// SNMPValueModule manages SNMP values.
type SNMPValueModule struct {
	BaseProxyModule
}

// NewSNMPValueModule creates a new SNMP value module.
func NewSNMPValueModule() *SNMPValueModule {
	return &SNMPValueModule{BaseProxyModule{name: "snmp_value"}}
}

// Execute runs the SNMP value module.
func (m *SNMPValueModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	oid, _ := m.GetString(mctx.Parameters, "oid")
	if oid == "" {
		return nil, fmt.Errorf("oid is required")
	}

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	// Get current value
	getResult, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("GET %s", oid))
	if err != nil {
		return nil, fmt.Errorf("failed to get SNMP value: %w", err)
	}

	result.Details["current_value"] = strings.TrimSpace(string(getResult.Stdout))

	// Set new value if specified
	if value, ok := m.GetString(mctx.Parameters, "value"); ok {
		valueType, _ := m.GetString(mctx.Parameters, "type")
		if valueType == "" {
			valueType = "s" // string
		}

		currentValue := strings.TrimSpace(string(getResult.Stdout))
		if currentValue != value {
			result.Changed = true
			if mctx.DryRun {
				result.Comment = fmt.Sprintf("OID %s would be set to %s", oid, value)
			} else {
				setResult, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("SET %s %s %s", oid, valueType, value))
				if err != nil || setResult.ExitCode != 0 {
					return nil, fmt.Errorf("failed to set SNMP value: %w", err)
				}
				result.Comment = fmt.Sprintf("OID %s set to %s", oid, value)
			}
		} else {
			result.Comment = fmt.Sprintf("OID %s already has value %s", oid, value)
		}
	}

	return result, nil
}

// Check performs a dry-run check.
func (m *SNMPValueModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// SNMPTableModule retrieves SNMP tables.
type SNMPTableModule struct {
	BaseProxyModule
}

// NewSNMPTableModule creates a new SNMP table module.
func NewSNMPTableModule() *SNMPTableModule {
	return &SNMPTableModule{BaseProxyModule{name: "snmp_table"}}
}

// Execute runs the SNMP table module.
func (m *SNMPTableModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	oid, _ := m.GetString(mctx.Parameters, "oid")
	if oid == "" {
		return nil, fmt.Errorf("oid is required")
	}

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	// Walk the table
	walkResult, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("WALK %s", oid))
	if err != nil {
		return nil, fmt.Errorf("failed to walk SNMP table: %w", err)
	}

	result.Details["table"] = walkResult.Stdout
	result.Comment = fmt.Sprintf("Retrieved table at %s", oid)

	return result, nil
}

// Check performs a dry-run check.
func (m *SNMPTableModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	return m.Execute(ctx, mctx) // Table retrieval is read-only
}

// =============================================================================
// HTTP/REST Modules
// =============================================================================

// HTTPConfigModule manages configuration via REST API.
type HTTPConfigModule struct {
	BaseProxyModule
}

// NewHTTPConfigModule creates a new HTTP config module.
func NewHTTPConfigModule() *HTTPConfigModule {
	return &HTTPConfigModule{BaseProxyModule{name: "http_config"}}
}

// Execute runs the HTTP config module.
func (m *HTTPConfigModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	path, _ := m.GetString(mctx.Parameters, "path")
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	// Get current configuration
	getResult, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("GET %s", path))
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	var currentConfig map[string]interface{}
	if err := json.Unmarshal(getResult.Stdout, &currentConfig); err != nil {
		currentConfig = nil
	}

	result.Details["current"] = currentConfig

	// Apply new configuration if specified
	if config, ok := mctx.Parameters["config"].(map[string]interface{}); ok {
		configJSON, _ := json.Marshal(config)

		// Check if config differs
		currentJSON, _ := json.Marshal(currentConfig)
		if !bytes.Equal(currentJSON, configJSON) {
			result.Changed = true
			if mctx.DryRun {
				result.Comment = fmt.Sprintf("Config at %s would be updated", path)
			} else {
				method, _ := m.GetString(mctx.Parameters, "method")
				if method == "" {
					method = "PUT"
				}
				setResult, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("%s %s %s", method, path, string(configJSON)))
				if err != nil || setResult.ExitCode != 0 {
					return nil, fmt.Errorf("failed to set config: %w", err)
				}
				result.Comment = fmt.Sprintf("Config at %s updated", path)
			}
		} else {
			result.Comment = fmt.Sprintf("Config at %s is already correct", path)
		}
	}

	return result, nil
}

// Check performs a dry-run check.
func (m *HTTPConfigModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// HTTPResourceModule manages REST resources.
type HTTPResourceModule struct {
	BaseProxyModule
}

// NewHTTPResourceModule creates a new HTTP resource module.
func NewHTTPResourceModule() *HTTPResourceModule {
	return &HTTPResourceModule{BaseProxyModule{name: "http_resource"}}
}

// Execute runs the HTTP resource module.
func (m *HTTPResourceModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	path, _ := m.GetString(mctx.Parameters, "path")
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	state, _ := m.GetString(mctx.Parameters, "state")
	if state == "" {
		state = "present"
	}

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	// Check if resource exists
	getResult, _ := mctx.ExecuteCommand(ctx, fmt.Sprintf("GET %s", path))
	exists := getResult.ExitCode == 0

	switch state {
	case "present":
		if !exists {
			result.Changed = true
			if mctx.DryRun {
				result.Comment = fmt.Sprintf("Resource %s would be created", path)
			} else {
				data, _ := mctx.Parameters["data"].(map[string]interface{})
				dataJSON, _ := json.Marshal(data)
				_, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("POST %s %s", path, string(dataJSON)))
				if err != nil {
					return nil, fmt.Errorf("failed to create resource: %w", err)
				}
				result.Comment = fmt.Sprintf("Resource %s created", path)
			}
		} else {
			result.Comment = fmt.Sprintf("Resource %s already exists", path)
		}

	case "absent":
		if exists {
			result.Changed = true
			if mctx.DryRun {
				result.Comment = fmt.Sprintf("Resource %s would be deleted", path)
			} else {
				_, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("DELETE %s", path))
				if err != nil {
					return nil, fmt.Errorf("failed to delete resource: %w", err)
				}
				result.Comment = fmt.Sprintf("Resource %s deleted", path)
			}
		} else {
			result.Comment = fmt.Sprintf("Resource %s already absent", path)
		}
	}

	return result, nil
}

// Check performs a dry-run check.
func (m *HTTPResourceModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}
