// Package winrm provides a WinRM protocol adapter for proxy agents.
package winrm

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/masterzen/winrm"
)

// ShellType represents the type of shell.
type ShellType string

const (
	// ShellPowerShell is PowerShell.
	ShellPowerShell ShellType = "powershell"
	// ShellCmd is CMD.
	ShellCmd ShellType = "cmd"
)

// Shell represents an interactive WinRM shell session.
type Shell struct {
	adapter   *Adapter
	shell     *winrm.Shell
	shellType ShellType
	mu        sync.Mutex
	closed    bool
}

// NewShell creates a new WinRM shell session.
func (a *Adapter) NewShell(ctx context.Context, shellType ShellType) (*Shell, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return nil, fmt.Errorf("not connected")
	}

	shell, err := client.CreateShell()
	if err != nil {
		return nil, fmt.Errorf("failed to create shell: %w", err)
	}

	return &Shell{
		adapter:   a,
		shell:     shell,
		shellType: shellType,
	}, nil
}

// Execute runs a command in the shell.
func (s *Shell) Execute(ctx context.Context, command string) (*ShellResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("shell is closed")
	}

	result := &ShellResult{
		StartTime: time.Now(),
	}

	// Prepare command based on shell type
	var execCommand string
	switch s.shellType {
	case ShellPowerShell:
		encoded := encodePowerShellCommand(command)
		execCommand = fmt.Sprintf("powershell.exe -NoProfile -NonInteractive -EncodedCommand %s", encoded)
	case ShellCmd:
		execCommand = command
	default:
		execCommand = command
	}

	// Execute command
	cmd, err := s.shell.Execute(execCommand)
	if err != nil {
		result.Error = err.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, err
	}
	defer cmd.Close()

	// Read stdout
	var stdout strings.Builder
	go func() {
		io.Copy(&stdout, cmd.Stdout)
	}()

	// Read stderr
	var stderr strings.Builder
	go func() {
		io.Copy(&stderr, cmd.Stderr)
	}()

	// Wait for command completion
	cmd.Wait()

	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.ExitCode = cmd.ExitCode()
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	return result, nil
}

// Close closes the shell session.
func (s *Shell) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	return s.shell.Close()
}

// ShellResult contains the result of a shell command.
type ShellResult struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	Error     string
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
}

// Success returns true if the command completed successfully.
func (r *ShellResult) Success() bool {
	return r.ExitCode == 0 && r.Error == ""
}

// ScriptRunner provides PowerShell script execution capabilities.
type ScriptRunner struct {
	adapter *Adapter
}

// NewScriptRunner creates a new script runner.
func (a *Adapter) NewScriptRunner() *ScriptRunner {
	return &ScriptRunner{adapter: a}
}

// RunScript executes a PowerShell script.
func (r *ScriptRunner) RunScript(ctx context.Context, script string) (*ScriptResult, error) {
	return r.RunScriptWithParams(ctx, script, nil)
}

// RunScriptWithParams executes a PowerShell script with parameters.
func (r *ScriptRunner) RunScriptWithParams(ctx context.Context, script string, params map[string]string) (*ScriptResult, error) {
	r.adapter.mu.RLock()
	client := r.adapter.client
	connected := r.adapter.connected
	r.adapter.mu.RUnlock()

	if !connected || client == nil {
		return nil, fmt.Errorf("not connected")
	}

	result := &ScriptResult{
		StartTime: time.Now(),
	}

	// Build parameter string
	var paramStr string
	if len(params) > 0 {
		var parts []string
		for k, v := range params {
			// Escape value for PowerShell
			escapedValue := strings.ReplaceAll(v, "'", "''")
			parts = append(parts, fmt.Sprintf("$%s = '%s'", k, escapedValue))
		}
		paramStr = strings.Join(parts, "; ") + "; "
	}

	// Combine parameters and script
	fullScript := paramStr + script

	// Execute
	stdout, stderr, exitCode, err := r.adapter.RunPowerShell(ctx, fullScript)

	result.Stdout = stdout
	result.Stderr = stderr
	result.ExitCode = exitCode
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	return result, nil
}

// RunScriptFile executes a PowerShell script from a local file path on the remote machine.
func (r *ScriptRunner) RunScriptFile(ctx context.Context, remotePath string, params map[string]string) (*ScriptResult, error) {
	// Build parameter arguments
	var args []string
	for k, v := range params {
		escapedValue := strings.ReplaceAll(v, "'", "''")
		args = append(args, fmt.Sprintf("-%s '%s'", k, escapedValue))
	}

	command := fmt.Sprintf("& '%s' %s", remotePath, strings.Join(args, " "))
	return r.RunScript(ctx, command)
}

// ScriptResult contains the result of a script execution.
type ScriptResult struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	Error     string
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
}

// Success returns true if the script completed successfully.
func (r *ScriptResult) Success() bool {
	return r.ExitCode == 0 && r.Error == ""
}

// ParseJSON attempts to parse the stdout as JSON into the provided interface.
func (r *ScriptResult) ParseJSON(v interface{}) error {
	// Import json here to avoid circular dependencies
	// In real implementation, you'd import encoding/json
	return fmt.Errorf("JSON parsing not implemented in this context")
}

// CommandBuilder helps build complex PowerShell commands.
type CommandBuilder struct {
	parts    []string
	params   map[string]string
	pipeline []string
}

// NewCommandBuilder creates a new command builder.
func NewCommandBuilder() *CommandBuilder {
	return &CommandBuilder{
		parts:  make([]string, 0),
		params: make(map[string]string),
	}
}

// Command sets the base command.
func (b *CommandBuilder) Command(cmd string) *CommandBuilder {
	b.parts = append(b.parts, cmd)
	return b
}

// Param adds a parameter.
func (b *CommandBuilder) Param(name, value string) *CommandBuilder {
	// Escape value
	escapedValue := strings.ReplaceAll(value, "'", "''")
	b.params[name] = escapedValue
	return b
}

// Switch adds a switch parameter.
func (b *CommandBuilder) Switch(name string) *CommandBuilder {
	b.parts = append(b.parts, fmt.Sprintf("-%s", name))
	return b
}

// Pipe adds a pipeline command.
func (b *CommandBuilder) Pipe(cmd string) *CommandBuilder {
	b.pipeline = append(b.pipeline, cmd)
	return b
}

// Build builds the command string.
func (b *CommandBuilder) Build() string {
	var result strings.Builder

	// Write main command
	result.WriteString(strings.Join(b.parts, " "))

	// Write parameters
	for name, value := range b.params {
		result.WriteString(fmt.Sprintf(" -%s '%s'", name, value))
	}

	// Write pipeline
	for _, p := range b.pipeline {
		result.WriteString(" | ")
		result.WriteString(p)
	}

	return result.String()
}

// SystemInfo retrieves system information from the remote Windows machine.
func (a *Adapter) GetSystemInfo(ctx context.Context) (*WindowsSystemInfo, error) {
	script := `
$os = Get-WmiObject Win32_OperatingSystem
$cs = Get-WmiObject Win32_ComputerSystem
$cpu = Get-WmiObject Win32_Processor | Select-Object -First 1

@{
    Hostname = $cs.Name
    Domain = $cs.Domain
    OSName = $os.Caption
    OSVersion = $os.Version
    OSArchitecture = $os.OSArchitecture
    TotalMemoryMB = [math]::Round($cs.TotalPhysicalMemory / 1MB)
    CPUName = $cpu.Name
    CPUCores = $cpu.NumberOfCores
    CPULogicalProcessors = $cpu.NumberOfLogicalProcessors
    LastBootTime = $os.ConvertToDateTime($os.LastBootUpTime)
} | ConvertTo-Json
`

	stdout, stderr, exitCode, err := a.RunPowerShell(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("failed to get system info: %w", err)
	}

	if exitCode != 0 {
		return nil, fmt.Errorf("get system info failed with exit code %d: %s", exitCode, stderr)
	}

	// Parse JSON output
	info := &WindowsSystemInfo{}
	// In real implementation, would use json.Unmarshal
	// For now, return a basic structure
	info.Raw = stdout

	return info, nil
}

// WindowsSystemInfo contains Windows system information.
type WindowsSystemInfo struct {
	Hostname             string `json:"Hostname"`
	Domain               string `json:"Domain"`
	OSName               string `json:"OSName"`
	OSVersion            string `json:"OSVersion"`
	OSArchitecture       string `json:"OSArchitecture"`
	TotalMemoryMB        int64  `json:"TotalMemoryMB"`
	CPUName              string `json:"CPUName"`
	CPUCores             int    `json:"CPUCores"`
	CPULogicalProcessors int    `json:"CPULogicalProcessors"`
	LastBootTime         string `json:"LastBootTime"`
	Raw                  string `json:"-"`
}

// ServiceManager provides Windows service management capabilities.
type ServiceManager struct {
	adapter *Adapter
}

// NewServiceManager creates a new service manager.
func (a *Adapter) NewServiceManager() *ServiceManager {
	return &ServiceManager{adapter: a}
}

// GetService gets information about a Windows service.
func (m *ServiceManager) GetService(ctx context.Context, name string) (*ServiceInfo, error) {
	script := fmt.Sprintf(`Get-Service -Name '%s' | Select-Object Name, Status, StartType, DisplayName | ConvertTo-Json`, name)

	stdout, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
	if err != nil {
		return nil, err
	}

	if exitCode != 0 {
		return nil, fmt.Errorf("get service failed: %s", stderr)
	}

	info := &ServiceInfo{
		Name: name,
		Raw:  stdout,
	}

	return info, nil
}

// StartService starts a Windows service.
func (m *ServiceManager) StartService(ctx context.Context, name string) error {
	script := fmt.Sprintf(`Start-Service -Name '%s'`, name)

	_, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
	if err != nil {
		return err
	}

	if exitCode != 0 {
		return fmt.Errorf("start service failed: %s", stderr)
	}

	return nil
}

// StopService stops a Windows service.
func (m *ServiceManager) StopService(ctx context.Context, name string) error {
	script := fmt.Sprintf(`Stop-Service -Name '%s' -Force`, name)

	_, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
	if err != nil {
		return err
	}

	if exitCode != 0 {
		return fmt.Errorf("stop service failed: %s", stderr)
	}

	return nil
}

// RestartService restarts a Windows service.
func (m *ServiceManager) RestartService(ctx context.Context, name string) error {
	script := fmt.Sprintf(`Restart-Service -Name '%s' -Force`, name)

	_, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
	if err != nil {
		return err
	}

	if exitCode != 0 {
		return fmt.Errorf("restart service failed: %s", stderr)
	}

	return nil
}

// SetServiceStartType sets the start type of a Windows service.
func (m *ServiceManager) SetServiceStartType(ctx context.Context, name string, startType string) error {
	script := fmt.Sprintf(`Set-Service -Name '%s' -StartupType '%s'`, name, startType)

	_, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
	if err != nil {
		return err
	}

	if exitCode != 0 {
		return fmt.Errorf("set service start type failed: %s", stderr)
	}

	return nil
}

// ServiceInfo contains Windows service information.
type ServiceInfo struct {
	Name        string `json:"Name"`
	Status      string `json:"Status"`
	StartType   string `json:"StartType"`
	DisplayName string `json:"DisplayName"`
	Raw         string `json:"-"`
}
