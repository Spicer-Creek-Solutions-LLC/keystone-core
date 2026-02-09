// Package fortinet provides Fortinet device adapters for proxy agents.
package fortinet

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

// FortiOSAdapter implements VendorAdapter for FortiOS devices (FortiGate).
type FortiOSAdapter struct {
	vendors.BaseVendorAdapter
	sshAdapter *ssh.Adapter
	shell      *ssh.NetworkDeviceShell
	vdom       string
}

// FortiOSConfig contains FortiOS specific configuration.
type FortiOSConfig struct {
	*vendors.VendorConfig
	// VDOM is the virtual domain to operate in (empty for global).
	VDOM string `json:"vdom,omitempty"`
}

// DefaultFortiOSConfig returns a default FortiOS configuration.
func DefaultFortiOSConfig() *FortiOSConfig {
	cfg := vendors.DefaultVendorConfig()
	cfg.EnablePrompt = "#"
	cfg.ConfigPrompt = "#"
	cfg.DisablePaging = true
	return &FortiOSConfig{VendorConfig: cfg}
}

// NewFortiOSAdapter creates a new FortiOS adapter.
func NewFortiOSAdapter(config *FortiOSConfig) *FortiOSAdapter {
	if config == nil {
		config = DefaultFortiOSConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.Timeout = config.Timeout

	return &FortiOSAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
		vdom:       config.VDOM,
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *FortiOSAdapter) Vendor() vendors.VendorType {
	return vendors.VendorFortiOS
}

// Type implements ProtocolAdapter.Type.
func (a *FortiOSAdapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *FortiOSAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:  "fortinet_fortios",
		Prompts: []string{"#", "$", "(global)", "(vdom)"},
	}
	shell, err := a.sshAdapter.NewNetworkDeviceShell(ctx, shellConfig)
	if err != nil {
		_ = a.sshAdapter.Disconnect(ctx)
		return fmt.Errorf("failed to create shell: %w", err)
	}
	a.shell = shell

	// Disable paging: set console output to standard mode
	if a.Config.DisablePaging {
		_, _ = a.runCommand(ctx, "config system console")
		_, _ = a.runCommand(ctx, "set output standard")
		_, _ = a.runCommand(ctx, "end")
	}

	// Enter VDOM if configured
	if a.vdom != "" {
		if _, err := a.runCommand(ctx, fmt.Sprintf("config vdom\nedit %s", a.vdom)); err != nil {
			return fmt.Errorf("failed to enter VDOM %s: %w", a.vdom, err)
		}
	}

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *FortiOSAdapter) Disconnect(ctx context.Context) error {
	a.Connected = false

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
func (a *FortiOSAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
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
func (a *FortiOSAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
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
	output, err := a.runCommand(ctx, "get system status")
	result.Latency = time.Since(start)

	if err != nil {
		result.Healthy = false
		result.Status = fmt.Sprintf("health check failed: %v", err)
		return result, nil
	}

	result.Healthy = true
	result.Status = "connected"
	result.Details["status"] = strings.TrimSpace(output)
	return result, nil
}

// IsConnected implements ProtocolAdapter.IsConnected.
func (a *FortiOSAdapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *FortiOSAdapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *FortiOSAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	var cmd string
	switch section {
	case "":
		cmd = "show full-configuration"
	case "interface", "interfaces":
		cmd = "show system interface"
	case "firewall":
		cmd = "show firewall policy"
	case "routing":
		cmd = "show router static"
	default:
		cmd = fmt.Sprintf("show %s", section)
	}
	return a.runCommand(ctx, cmd)
}

// SetConfig implements VendorAdapter.SetConfig.
// FortiOS uses config/edit/set/next/end pattern. Commands should be provided
// as individual lines of a configuration block.
func (a *FortiOSAdapter) SetConfig(ctx context.Context, commands []string) error {
	for _, cmd := range commands {
		if _, err := a.runCommand(ctx, cmd); err != nil {
			// Attempt to abort any open config section
			_, _ = a.runCommand(ctx, "abort")
			return fmt.Errorf("command failed '%s': %w", cmd, err)
		}
	}
	return nil
}

// GetFacts implements VendorAdapter.GetFacts.
func (a *FortiOSAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "Fortinet",
		OSType: "FortiOS",
		Raw:    make(map[string]string),
	}

	status, err := a.runCommand(ctx, "get system status")
	if err != nil {
		return nil, fmt.Errorf("failed to get system status: %w", err)
	}
	facts.Raw["get system status"] = status
	a.parseStatus(status, facts)

	perf, err := a.runCommand(ctx, "get system performance status")
	if err == nil {
		facts.Raw["get system performance status"] = perf
		a.parsePerformance(perf, facts)
	}

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
// FortiOS auto-saves on 'end' - this is a no-op but included for interface compliance.
func (a *FortiOSAdapter) SaveConfig(_ context.Context) error {
	return nil
}

// ConfigSection applies a FortiOS configuration section.
// Example: ConfigSection(ctx, "system interface", "port1", map[string]string{"ip": "10.0.0.1/24", "allowaccess": "ping https ssh"})
func (a *FortiOSAdapter) ConfigSection(ctx context.Context, section, name string, settings map[string]string) error {
	if _, err := a.runCommand(ctx, fmt.Sprintf("config %s", section)); err != nil {
		return fmt.Errorf("failed to enter config section: %w", err)
	}

	if name != "" {
		if _, err := a.runCommand(ctx, fmt.Sprintf("edit %s", name)); err != nil {
			_, _ = a.runCommand(ctx, "abort")
			return fmt.Errorf("failed to edit %s: %w", name, err)
		}
	}

	for key, value := range settings {
		if _, err := a.runCommand(ctx, fmt.Sprintf("set %s %s", key, value)); err != nil {
			_, _ = a.runCommand(ctx, "abort")
			return fmt.Errorf("failed to set %s: %w", key, err)
		}
	}

	if name != "" {
		if _, err := a.runCommand(ctx, "next"); err != nil {
			return err
		}
	}

	if _, err := a.runCommand(ctx, "end"); err != nil {
		return err
	}

	return nil
}

func (a *FortiOSAdapter) runCommand(ctx context.Context, command string) (string, error) {
	if a.shell == nil {
		return "", fmt.Errorf("shell not initialized")
	}

	result, err := a.shell.Execute(ctx, command)
	if err != nil {
		return "", err
	}

	output := result.Output
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "command fail") ||
		strings.Contains(lowerOutput, "unknown action") ||
		strings.Contains(lowerOutput, "entry not found") ||
		strings.Contains(lowerOutput, "object already exists") {
		return output, fmt.Errorf("command error: %s", output)
	}
	return output, nil
}

func (a *FortiOSAdapter) parseStatus(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Version:") || strings.HasPrefix(line, "Version :") {
			if match := regexp.MustCompile(`v(\d+\.\d+\.\d+)`).FindStringSubmatch(line); len(match) > 1 {
				facts.OSVersion = match[1]
			}
		}

		if strings.HasPrefix(line, "Hostname:") || strings.HasPrefix(line, "Hostname :") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Hostname = strings.TrimSpace(parts[1])
			}
		}

		if strings.HasPrefix(line, "Serial-Number:") || strings.HasPrefix(line, "Serial-Number :") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.SerialNumber = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "Platform") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Model = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "System time") || strings.Contains(line, "Uptime") {
			uptime := parseFortiUptime(line)
			if uptime > 0 {
				facts.Uptime = uptime
			}
		}
	}
}

func (a *FortiOSAdapter) parsePerformance(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "CPU") && strings.Contains(line, "%") {
			if match := regexp.MustCompile(`(\d+)%`).FindStringSubmatch(line); len(match) > 1 {
				cpu, _ := strconv.ParseFloat(match[1], 64)
				facts.CPUUsage = cpu
			}
		}

		if strings.Contains(line, "Memory") && strings.Contains(line, "used") {
			if match := regexp.MustCompile(`(\d+)\s+(\d+)`).FindStringSubmatch(line); len(match) > 2 {
				total, _ := strconv.ParseInt(match[1], 10, 64)
				used, _ := strconv.ParseInt(match[2], 10, 64)
				facts.MemoryTotal = total * 1024
				facts.MemoryFree = (total - used) * 1024
			}
		}
	}
}

// parseFortiUptime parses FortiOS uptime format.
func parseFortiUptime(line string) time.Duration {
	var total time.Duration

	dayMatch := regexp.MustCompile(`(\d+)\s*day`).FindStringSubmatch(line)
	hourMatch := regexp.MustCompile(`(\d+)\s*hour`).FindStringSubmatch(line)
	minuteMatch := regexp.MustCompile(`(\d+)\s*min`).FindStringSubmatch(line)

	if len(dayMatch) > 1 {
		days, _ := strconv.Atoi(dayMatch[1])
		total += time.Duration(days) * 24 * time.Hour
	}
	if len(hourMatch) > 1 {
		hours, _ := strconv.Atoi(hourMatch[1])
		total += time.Duration(hours) * time.Hour
	}
	if len(minuteMatch) > 1 {
		minutes, _ := strconv.Atoi(minuteMatch[1])
		total += time.Duration(minutes) * time.Minute
	}

	return total
}

// NewFortiOSAdapterFactory creates an adapter factory for FortiOS.
func NewFortiOSAdapterFactory(config *FortiOSConfig) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultFortiOSConfig()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewFortiOSAdapter(config), nil
	}
}

func init() {
	vendors.Register(vendors.VendorFortiOS, NewFortiOSAdapterFactory(nil))
}
