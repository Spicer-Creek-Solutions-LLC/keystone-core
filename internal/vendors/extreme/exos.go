// Package extreme provides Extreme Networks EXOS device adapters for proxy agents.
package extreme

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/protocols/ssh"
	"github.com/shawnbutts/keystone-core/internal/proxy"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

// EXOSAdapter implements VendorAdapter for Extreme Networks EXOS devices.
type EXOSAdapter struct {
	vendors.BaseVendorAdapter
	sshAdapter *ssh.Adapter
	shell      *ssh.NetworkDeviceShell
}

// EXOSConfig contains Extreme EXOS specific configuration.
type EXOSConfig struct {
	*vendors.VendorConfig
}

// DefaultEXOSConfig returns a default EXOS configuration.
func DefaultEXOSConfig() *EXOSConfig {
	cfg := vendors.DefaultVendorConfig()
	cfg.EnablePrompt = "#"
	cfg.ConfigPrompt = "#"
	cfg.DisablePaging = true
	return &EXOSConfig{VendorConfig: cfg}
}

// NewEXOSAdapter creates a new Extreme EXOS adapter.
func NewEXOSAdapter(config *EXOSConfig) *EXOSAdapter {
	if config == nil {
		config = DefaultEXOSConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.Timeout = config.Timeout

	return &EXOSAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *EXOSAdapter) Vendor() vendors.VendorType {
	return vendors.VendorExtremeEXOS
}

// Type implements ProtocolAdapter.Type.
func (a *EXOSAdapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *EXOSAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:  "extreme_exos",
		Prompts: []string{"#", ">", "."},
	}
	shell, err := a.sshAdapter.NewNetworkDeviceShell(ctx, shellConfig)
	if err != nil {
		_ = a.sshAdapter.Disconnect(ctx)
		return fmt.Errorf("failed to create shell: %w", err)
	}
	a.shell = shell

	if a.Config.DisablePaging {
		_, _ = a.runCommand(ctx, "disable clipaging")
	}

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *EXOSAdapter) Disconnect(ctx context.Context) error {
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
func (a *EXOSAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
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
func (a *EXOSAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
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
	output, err := a.runCommand(ctx, "show switch")
	result.Latency = time.Since(start)

	if err != nil {
		result.Healthy = false
		result.Status = fmt.Sprintf("health check failed: %v", err)
		return result, nil
	}

	result.Healthy = true
	result.Status = "connected"
	result.Details["switch"] = strings.TrimSpace(output)
	return result, nil
}

// IsConnected implements ProtocolAdapter.IsConnected.
func (a *EXOSAdapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *EXOSAdapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *EXOSAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	if section == "" {
		return a.runCommand(ctx, "show configuration")
	}
	return a.runCommand(ctx, fmt.Sprintf("show configuration %s", section))
}

// SetConfig implements VendorAdapter.SetConfig.
// EXOS applies commands directly at the privileged level.
func (a *EXOSAdapter) SetConfig(ctx context.Context, commands []string) error {
	for _, cmd := range commands {
		if _, err := a.runCommand(ctx, cmd); err != nil {
			return fmt.Errorf("command failed '%s': %w", cmd, err)
		}
	}
	return nil
}

// GetFacts implements VendorAdapter.GetFacts.
func (a *EXOSAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "Extreme Networks",
		OSType: "EXOS",
		Raw:    make(map[string]string),
	}

	version, err := a.runCommand(ctx, "show version")
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}
	facts.Raw["show version"] = version
	a.parseVersion(version, facts)

	switchInfo, err := a.runCommand(ctx, "show switch")
	if err == nil {
		facts.Raw["show switch"] = switchInfo
		a.parseSwitchInfo(switchInfo, facts)
	}

	ports, err := a.runCommand(ctx, "show ports")
	if err == nil {
		facts.Raw["show ports"] = ports
		facts.Interfaces = a.parsePorts(ports)
	}

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *EXOSAdapter) SaveConfig(ctx context.Context) error {
	_, err := a.runCommand(ctx, "save configuration primary")
	return err
}

func (a *EXOSAdapter) runCommand(ctx context.Context, command string) (string, error) {
	if a.shell == nil {
		return "", fmt.Errorf("shell not initialized")
	}

	result, err := a.shell.Execute(ctx, command)
	if err != nil {
		return "", err
	}

	output := result.Output
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "invalid input") ||
		strings.Contains(lowerOutput, "error:") ||
		strings.Contains(lowerOutput, "command not found") {
		return output, fmt.Errorf("command error: %s", output)
	}
	return output, nil
}

func (a *EXOSAdapter) parseVersion(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "ExtremeXOS version") {
			if match := regexp.MustCompile(`(\d+\.\d+\.\d+(?:\.\d+)?)`).FindStringSubmatch(line); len(match) > 1 {
				facts.OSVersion = match[1]
			}
		}

		if strings.Contains(line, "System MAC:") || strings.Contains(line, "Switch") && strings.Contains(line, "Serial") {
			if match := regexp.MustCompile(`([A-Z0-9]{10,})`).FindStringSubmatch(line); len(match) > 1 {
				if facts.SerialNumber == "" {
					facts.SerialNumber = match[1]
				}
			}
		}
	}
}

func (a *EXOSAdapter) parseSwitchInfo(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "SysName:") || strings.HasPrefix(line, "sysName:") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Hostname = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "System Type:") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Model = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "System UpTime:") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Uptime = parseEXOSUptime(strings.TrimSpace(parts[1]))
			}
		}
	}
}

func (a *EXOSAdapter) parsePorts(output string) []vendors.InterfaceFact {
	var interfaces []vendors.InterfaceFact

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Port") || strings.HasPrefix(line, "====") ||
			strings.HasPrefix(line, "---") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		// Port format: port_number state speed ...
		portName := fields[0]
		if !strings.Contains(portName, ":") && !strings.ContainsAny(portName, "0123456789") {
			continue
		}

		iface := vendors.InterfaceFact{
			Name:        portName,
			AdminStatus: "up",
			OperStatus:  "down",
		}

		for _, f := range fields[1:] {
			switch strings.ToLower(f) {
			case "e", "enabled":
				iface.AdminStatus = "up"
			case "d", "disabled":
				iface.AdminStatus = "down"
			case "active", "ready":
				iface.OperStatus = "up"
			}
		}

		interfaces = append(interfaces, iface)
	}

	return interfaces
}

func parseEXOSUptime(s string) time.Duration {
	var days, hours, minutes, seconds int

	if match := regexp.MustCompile(`(\d+)\s*day`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &days)
	}
	if match := regexp.MustCompile(`(\d+)\s*hr`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &hours)
	}
	if match := regexp.MustCompile(`(\d+)\s*min`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &minutes)
	}
	if match := regexp.MustCompile(`(\d+)\s*sec`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &seconds)
	}

	return time.Duration(days)*24*time.Hour +
		time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second
}

// NewEXOSAdapterFactory creates an adapter factory for Extreme EXOS.
func NewEXOSAdapterFactory(config *EXOSConfig) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultEXOSConfig()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewEXOSAdapter(config), nil
	}
}

func init() {
	vendors.Register(vendors.VendorExtremeEXOS, NewEXOSAdapterFactory(nil))
}
