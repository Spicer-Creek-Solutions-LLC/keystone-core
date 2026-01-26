package statemgmt

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/platform"
)

// ServiceModule implements service management
type ServiceModule struct {
	*BaseModule
}

// NewServiceModule creates a new service module
func NewServiceModule() *ServiceModule {
	return &ServiceModule{
		BaseModule: NewBaseModule("service", []string{"running", "stopped", "enabled", "disabled", "dead"}),
	}
}

// Check checks the current state of a service
func (m *ServiceModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	serviceName := decl.ID
	sm, err := m.detectServiceManager()
	if err != nil {
		return nil, err
	}

	// Check if service is running
	running, err := m.isServiceRunning(ctx, sm, serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to check service status: %w", err)
	}

	// Check if service is enabled
	enabled, err := m.isServiceEnabled(ctx, sm, serviceName)
	if err != nil {
		// Not all service managers support enable/disable
		enabled = false
	}

	result.Present = true
	result.Metadata["running"] = running
	result.Metadata["enabled"] = enabled

	if running {
		result.CurrentState = "running"
	} else {
		result.CurrentState = "stopped"
	}

	// Determine if state matches
	switch decl.State {
	case "running":
		result.Matches = running
		if !running {
			result.Diff["state"] = map[string]string{"current": "stopped", "desired": "running"}
		}
		// Check if should be enabled
		if enableParam := getBoolParameter(decl, "enable", false); enableParam && !enabled {
			result.Matches = false
			result.Diff["enabled"] = map[string]bool{"current": enabled, "desired": true}
		}

	case "stopped", "dead":
		result.Matches = !running
		if running {
			result.Diff["state"] = map[string]string{"current": "running", "desired": "stopped"}
		}

	case "enabled":
		result.Matches = enabled
		if !enabled {
			result.Diff["enabled"] = map[string]bool{"current": enabled, "desired": true}
		}

	case "disabled":
		result.Matches = !enabled
		if enabled {
			result.Diff["enabled"] = map[string]bool{"current": enabled, "desired": false}
		}
	}

	return result, nil
}

// Apply applies the service state
func (m *ServiceModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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

	sm, err := m.detectServiceManager()
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to detect service manager: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Apply changes
	var applyErr error
	switch decl.State {
	case "running":
		applyErr = m.startService(ctx, sm, decl, result)
	case "stopped", "dead":
		applyErr = m.stopService(ctx, sm, decl, result)
	case "enabled":
		applyErr = m.enableService(ctx, sm, decl, result)
	case "disabled":
		applyErr = m.disableService(ctx, sm, decl, result)
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

// Test tests if the service is in the desired state
func (m *ServiceModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// ServiceManager represents different service managers
type ServiceManager string

const (
	SMUnknown        ServiceManager = "unknown"
	SMSystemd        ServiceManager = "systemd"
	SMInitD          ServiceManager = "init.d"
	SMLaunchd        ServiceManager = "launchd"
	SMOpenRC         ServiceManager = "openrc"
	SMUpstart        ServiceManager = "upstart"
	SMWindowsService ServiceManager = "windows_service"
)

// detectServiceManager detects the available service manager using platform detection
func (m *ServiceModule) detectServiceManager() (ServiceManager, error) {
	// Use platform detection for accurate init system detection
	initSys, err := platform.DetectInitSystem()
	if err == nil && initSys != platform.InitUnknown {
		return convertPlatformInitSystem(initSys), nil
	}

	// Fallback to manual detection
	// Check for systemd
	if _, err := exec.LookPath("systemctl"); err == nil {
		return SMSystemd, nil
	}

	// Check for launchd (macOS)
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("launchctl"); err == nil {
			return SMLaunchd, nil
		}
	}

	// Check for Windows Service Manager
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("sc.exe"); err == nil {
			return SMWindowsService, nil
		}
	}

	// Check for OpenRC
	if _, err := exec.LookPath("rc-service"); err == nil {
		return SMOpenRC, nil
	}

	// Check for upstart
	if _, err := exec.LookPath("initctl"); err == nil {
		return SMUpstart, nil
	}

	// Fallback to init.d
	if _, err := exec.LookPath("service"); err == nil {
		return SMInitD, nil
	}

	return SMUnknown, fmt.Errorf("no supported service manager found on %s", runtime.GOOS)
}

// convertPlatformInitSystem converts platform.InitSystem to ServiceManager
func convertPlatformInitSystem(initSys platform.InitSystem) ServiceManager {
	switch initSys {
	case platform.InitSystemd:
		return SMSystemd
	case platform.InitUpstart:
		return SMUpstart
	case platform.InitSysV:
		return SMInitD
	case platform.InitOpenRC:
		return SMOpenRC
	case platform.InitLaunchd:
		return SMLaunchd
	case platform.InitWindowsService:
		return SMWindowsService
	default:
		return SMUnknown
	}
}

// isServiceRunning checks if a service is running
func (m *ServiceModule) isServiceRunning(ctx context.Context, sm ServiceManager, serviceName string) (bool, error) {
	var cmd *exec.Cmd

	switch sm {
	case SMSystemd:
		cmd = exec.CommandContext(ctx, "systemctl", "is-active", serviceName)
	case SMLaunchd:
		cmd = exec.CommandContext(ctx, "launchctl", "list", serviceName)
	case SMOpenRC:
		cmd = exec.CommandContext(ctx, "rc-service", serviceName, "status")
	case SMInitD:
		cmd = exec.CommandContext(ctx, "service", serviceName, "status")
	case SMUpstart:
		cmd = exec.CommandContext(ctx, "status", serviceName)
	case SMWindowsService:
		cmd = exec.CommandContext(ctx, "sc.exe", "query", serviceName)
	default:
		return false, fmt.Errorf("unsupported service manager: %s", sm)
	}

	output, err := cmd.Output()
	if err != nil {
		// Service is not running
		return false, nil
	}

	outputStr := strings.TrimSpace(string(output))

	// Parse output based on service manager
	switch sm {
	case SMSystemd:
		return outputStr == "active", nil
	case SMLaunchd:
		// If launchctl list succeeds, service is loaded
		return true, nil
	case SMOpenRC, SMInitD:
		// Check for "running" or "active" in output
		return strings.Contains(strings.ToLower(outputStr), "running") ||
			strings.Contains(strings.ToLower(outputStr), "active"), nil
	case SMUpstart:
		return strings.Contains(outputStr, "start/running"), nil
	case SMWindowsService:
		// Windows: check for "RUNNING" in output
		return strings.Contains(outputStr, "RUNNING"), nil
	}

	return false, nil
}

// isServiceEnabled checks if a service is enabled
func (m *ServiceModule) isServiceEnabled(ctx context.Context, sm ServiceManager, serviceName string) (bool, error) {
	var cmd *exec.Cmd

	switch sm {
	case SMSystemd:
		cmd = exec.CommandContext(ctx, "systemctl", "is-enabled", serviceName)
	case SMOpenRC:
		cmd = exec.CommandContext(ctx, "rc-update", "show")
	case SMWindowsService:
		cmd = exec.CommandContext(ctx, "sc.exe", "qc", serviceName)
	default:
		// Not all service managers support enable/disable
		return false, fmt.Errorf("enable/disable not supported for %s", sm)
	}

	output, err := cmd.Output()
	if err != nil {
		return false, nil
	}

	outputStr := strings.TrimSpace(string(output))

	switch sm {
	case SMSystemd:
		return outputStr == "enabled", nil
	case SMOpenRC:
		// Check if service is in the output
		return strings.Contains(outputStr, serviceName), nil
	case SMWindowsService:
		// Windows: check for AUTO_START
		return strings.Contains(outputStr, "AUTO_START"), nil
	}

	return false, nil
}

// startService starts a service
func (m *ServiceModule) startService(ctx context.Context, sm ServiceManager, decl *StateDeclaration, result *StateResult) error {
	serviceName := decl.ID
	var cmd *exec.Cmd

	switch sm {
	case SMSystemd:
		cmd = exec.CommandContext(ctx, "systemctl", "start", serviceName)
	case SMLaunchd:
		cmd = exec.CommandContext(ctx, "launchctl", "start", serviceName)
	case SMOpenRC:
		cmd = exec.CommandContext(ctx, "rc-service", serviceName, "start")
	case SMInitD:
		cmd = exec.CommandContext(ctx, "service", serviceName, "start")
	case SMUpstart:
		cmd = exec.CommandContext(ctx, "start", serviceName)
	case SMWindowsService:
		cmd = exec.CommandContext(ctx, "sc.exe", "start", serviceName)
	default:
		return fmt.Errorf("unsupported service manager: %s", sm)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start service: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Service %s started", serviceName)

	// Also enable if requested
	if getBoolParameter(decl, "enable", false) {
		if err := m.enableService(ctx, sm, decl, result); err != nil {
			return err
		}
		result.Comment = fmt.Sprintf("Service %s started and enabled", serviceName)
	}

	return nil
}

// stopService stops a service
func (m *ServiceModule) stopService(ctx context.Context, sm ServiceManager, decl *StateDeclaration, result *StateResult) error {
	serviceName := decl.ID
	var cmd *exec.Cmd

	switch sm {
	case SMSystemd:
		cmd = exec.CommandContext(ctx, "systemctl", "stop", serviceName)
	case SMLaunchd:
		cmd = exec.CommandContext(ctx, "launchctl", "stop", serviceName)
	case SMOpenRC:
		cmd = exec.CommandContext(ctx, "rc-service", serviceName, "stop")
	case SMInitD:
		cmd = exec.CommandContext(ctx, "service", serviceName, "stop")
	case SMUpstart:
		cmd = exec.CommandContext(ctx, "stop", serviceName)
	case SMWindowsService:
		cmd = exec.CommandContext(ctx, "sc.exe", "stop", serviceName)
	default:
		return fmt.Errorf("unsupported service manager: %s", sm)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop service: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Service %s stopped", serviceName)
	return nil
}

// enableService enables a service
func (m *ServiceModule) enableService(ctx context.Context, sm ServiceManager, decl *StateDeclaration, result *StateResult) error {
	serviceName := decl.ID
	var cmd *exec.Cmd

	switch sm {
	case SMSystemd:
		cmd = exec.CommandContext(ctx, "systemctl", "enable", serviceName)
	case SMOpenRC:
		cmd = exec.CommandContext(ctx, "rc-update", "add", serviceName)
	case SMWindowsService:
		cmd = exec.CommandContext(ctx, "sc.exe", "config", serviceName, "start=", "auto")
	default:
		return fmt.Errorf("enable not supported for %s", sm)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable service: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Service %s enabled", serviceName)
	return nil
}

// disableService disables a service
func (m *ServiceModule) disableService(ctx context.Context, sm ServiceManager, decl *StateDeclaration, result *StateResult) error {
	serviceName := decl.ID
	var cmd *exec.Cmd

	switch sm {
	case SMSystemd:
		cmd = exec.CommandContext(ctx, "systemctl", "disable", serviceName)
	case SMOpenRC:
		cmd = exec.CommandContext(ctx, "rc-update", "del", serviceName)
	case SMWindowsService:
		cmd = exec.CommandContext(ctx, "sc.exe", "config", serviceName, "start=", "disabled")
	default:
		return fmt.Errorf("disable not supported for %s", sm)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to disable service: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Service %s disabled", serviceName)
	return nil
}

func init() {
	RegisterModule(NewServiceModule())
}
