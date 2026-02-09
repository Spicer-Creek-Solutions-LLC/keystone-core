// Package paloalto provides Palo Alto Networks device adapters for proxy agents.
package paloalto

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/protocols/ssh"
	"github.com/shawnbutts/keystone-core/internal/proxy"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

// PANOSAdapter implements VendorAdapter for Palo Alto PAN-OS devices.
type PANOSAdapter struct {
	vendors.BaseVendorAdapter
	sshAdapter *ssh.Adapter
	shell      *ssh.NetworkDeviceShell
	inConfig   bool
}

// PANOSConfig contains PAN-OS specific configuration.
type PANOSConfig struct {
	*vendors.VendorConfig
}

// DefaultPANOSConfig returns a default PAN-OS configuration.
func DefaultPANOSConfig() *PANOSConfig {
	cfg := vendors.DefaultVendorConfig()
	cfg.EnablePrompt = ">"
	cfg.ConfigPrompt = "#"
	cfg.DisablePaging = true
	return &PANOSConfig{VendorConfig: cfg}
}

// NewPANOSAdapter creates a new PAN-OS adapter.
func NewPANOSAdapter(config *PANOSConfig) *PANOSAdapter {
	if config == nil {
		config = DefaultPANOSConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.Timeout = config.Timeout

	return &PANOSAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *PANOSAdapter) Vendor() vendors.VendorType {
	return vendors.VendorPANOS
}

// Type implements ProtocolAdapter.Type.
func (a *PANOSAdapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *PANOSAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:  "paloalto_panos",
		Prompts: []string{">", "#"},
	}
	shell, err := a.sshAdapter.NewNetworkDeviceShell(ctx, shellConfig)
	if err != nil {
		_ = a.sshAdapter.Disconnect(ctx)
		return fmt.Errorf("failed to create shell: %w", err)
	}
	a.shell = shell

	if a.Config.DisablePaging {
		_, _ = a.runCommand(ctx, "set cli pager off")
	}

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *PANOSAdapter) Disconnect(ctx context.Context) error {
	a.Connected = false
	a.inConfig = false

	if a.shell != nil {
		a.shell.Close()
		a.shell = nil
	}

	if a.sshAdapter != nil {
		return a.sshAdapter.Disconnect(ctx)
	}
	return nil
}

// Execute implements ProtocolAdapter.Execute.
func (a *PANOSAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
	start := time.Now()
	result := &protocols.ExecuteResult{StartTime: start}

	output, err := a.runCommand(ctx, req.Command)
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(start)

	if err != nil {
		result.Error = err.Error()
		result.ExitCode = 1
		return result, err
	}

	result.Stdout = []byte(output)
	result.ExitCode = 0
	return result, nil
}

// HealthCheck implements ProtocolAdapter.HealthCheck.
func (a *PANOSAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
	result := &protocols.HealthCheckResult{
		LastCheck: time.Now(),
		Details:   make(map[string]interface{}),
	}

	if !a.Connected || a.shell == nil {
		result.Healthy = false
		result.Status = "not connected"
		return result, nil
	}

	start := time.Now()
	output, err := a.runCommand(ctx, "show system info")
	result.Latency = time.Since(start)

	if err != nil {
		result.Healthy = false
		result.Status = fmt.Sprintf("health check failed: %v", err)
		return result, nil
	}

	result.Healthy = true
	result.Status = "connected"
	result.Details["system_info"] = strings.TrimSpace(output)
	return result, nil
}

// IsConnected implements ProtocolAdapter.IsConnected.
func (a *PANOSAdapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *PANOSAdapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *PANOSAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	if !a.inConfig {
		if err := a.enterConfig(ctx); err != nil {
			return "", err
		}
	}

	var cmd string
	switch section {
	case "":
		cmd = "show"
	case "running":
		// Exit config mode to show running config
		if a.inConfig {
			_ = a.exitConfig(ctx)
		}
		return a.runCommand(ctx, "show running security-policy")
	case "candidate":
		cmd = "show"
	default:
		cmd = fmt.Sprintf("show %s", section)
	}
	return a.runCommand(ctx, cmd)
}

// SetConfig implements VendorAdapter.SetConfig.
// PAN-OS uses 'set' commands in configure mode. Commands must be committed separately.
func (a *PANOSAdapter) SetConfig(ctx context.Context, commands []string) error {
	if !a.inConfig {
		if err := a.enterConfig(ctx); err != nil {
			return err
		}
	}

	for _, cmd := range commands {
		setCmd := cmd
		if !strings.HasPrefix(strings.ToLower(cmd), "set ") &&
			!strings.HasPrefix(strings.ToLower(cmd), "delete ") &&
			!strings.HasPrefix(strings.ToLower(cmd), "edit ") {
			setCmd = "set " + cmd
		}
		if _, err := a.runCommand(ctx, setCmd); err != nil {
			return fmt.Errorf("command failed '%s': %w", setCmd, err)
		}
	}
	return nil
}

// GetFacts implements VendorAdapter.GetFacts.
func (a *PANOSAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "Palo Alto Networks",
		OSType: "PAN-OS",
		Raw:    make(map[string]string),
	}

	// Exit config mode if in it
	if a.inConfig {
		_ = a.exitConfig(ctx)
	}

	sysInfo, err := a.runCommand(ctx, "show system info")
	if err != nil {
		return nil, fmt.Errorf("failed to get system info: %w", err)
	}
	facts.Raw["show system info"] = sysInfo
	a.parseSystemInfo(sysInfo, facts)

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
// PAN-OS uses transactional commits. This commits the candidate configuration.
func (a *PANOSAdapter) SaveConfig(ctx context.Context) error {
	if a.inConfig {
		if err := a.exitConfig(ctx); err != nil {
			return err
		}
	}
	return a.Commit(ctx)
}

// Commit commits the candidate configuration.
func (a *PANOSAdapter) Commit(ctx context.Context) error {
	output, err := a.runCommand(ctx, "commit")
	if err != nil {
		return fmt.Errorf("commit failed: %w", err)
	}
	if strings.Contains(strings.ToLower(output), "failed") {
		return fmt.Errorf("commit failed: %s", output)
	}
	return nil
}

// CommitPartial commits only changes for a specific admin or vsys.
func (a *PANOSAdapter) CommitPartial(ctx context.Context, admin string) error {
	cmd := fmt.Sprintf("commit partial admin %s", admin)
	output, err := a.runCommand(ctx, cmd)
	if err != nil {
		return fmt.Errorf("partial commit failed: %w", err)
	}
	if strings.Contains(strings.ToLower(output), "failed") {
		return fmt.Errorf("partial commit failed: %s", output)
	}
	return nil
}

func (a *PANOSAdapter) enterConfig(ctx context.Context) error {
	if a.inConfig {
		return nil
	}
	if _, err := a.runCommand(ctx, "configure"); err != nil {
		return fmt.Errorf("failed to enter config mode: %w", err)
	}
	a.inConfig = true
	return nil
}

func (a *PANOSAdapter) exitConfig(ctx context.Context) error {
	if !a.inConfig {
		return nil
	}
	if _, err := a.runCommand(ctx, "exit"); err != nil {
		return err
	}
	a.inConfig = false
	return nil
}

func (a *PANOSAdapter) runCommand(ctx context.Context, command string) (string, error) {
	if a.shell == nil {
		return "", fmt.Errorf("shell not initialized")
	}

	result, err := a.shell.Execute(ctx, command)
	if err != nil {
		return "", err
	}

	output := result.Output
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "unknown command") ||
		strings.Contains(lowerOutput, "invalid syntax") ||
		strings.Contains(lowerOutput, "server error") {
		return output, fmt.Errorf("command error: %s", output)
	}
	return output, nil
}

func (a *PANOSAdapter) parseSystemInfo(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "hostname:") {
			facts.Hostname = strings.TrimSpace(strings.TrimPrefix(line, "hostname:"))
		}

		if strings.HasPrefix(line, "model:") {
			facts.Model = strings.TrimSpace(strings.TrimPrefix(line, "model:"))
		}

		if strings.HasPrefix(line, "serial:") {
			facts.SerialNumber = strings.TrimSpace(strings.TrimPrefix(line, "serial:"))
		}

		if strings.HasPrefix(line, "sw-version:") {
			facts.OSVersion = strings.TrimSpace(strings.TrimPrefix(line, "sw-version:"))
		}

		if strings.HasPrefix(line, "uptime:") {
			uptime := parsePANOSUptime(strings.TrimPrefix(line, "uptime:"))
			if uptime > 0 {
				facts.Uptime = uptime
			}
		}
	}
}

// parsePANOSUptime parses PAN-OS uptime format: "30 days, 5:12:45"
func parsePANOSUptime(line string) time.Duration {
	var total time.Duration

	dayMatch := regexp.MustCompile(`(\d+)\s*day`).FindStringSubmatch(line)
	timeMatch := regexp.MustCompile(`(\d+):(\d+):(\d+)`).FindStringSubmatch(line)

	if len(dayMatch) > 1 {
		days, _ := strconv.Atoi(dayMatch[1])
		total += time.Duration(days) * 24 * time.Hour
	}
	if len(timeMatch) > 3 {
		hours, _ := strconv.Atoi(timeMatch[1])
		minutes, _ := strconv.Atoi(timeMatch[2])
		seconds, _ := strconv.Atoi(timeMatch[3])
		total += time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second
	}

	return total
}

// NewPANOSAdapterFactory creates an adapter factory for PAN-OS.
func NewPANOSAdapterFactory(config *PANOSConfig) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultPANOSConfig()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewPANOSAdapter(config), nil
	}
}

func init() {
	vendors.Register(vendors.VendorPANOS, NewPANOSAdapterFactory(nil))
}
