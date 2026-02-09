// Package mellanox provides Mellanox/NVIDIA Onyx device adapters for proxy agents.
package mellanox

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

// OnyxAdapter implements VendorAdapter for Mellanox/NVIDIA Onyx devices.
type OnyxAdapter struct {
	vendors.BaseVendorAdapter
	sshAdapter   *ssh.Adapter
	shell        *ssh.NetworkDeviceShell
	inConfigMode bool
}

// OnyxConfig contains Mellanox Onyx specific configuration.
type OnyxConfig struct {
	*vendors.VendorConfig
}

// DefaultOnyxConfig returns a default Onyx configuration.
func DefaultOnyxConfig() *OnyxConfig {
	cfg := vendors.DefaultVendorConfig()
	cfg.EnablePrompt = "#"
	cfg.ConfigPrompt = "(config"
	cfg.DisablePaging = true
	return &OnyxConfig{VendorConfig: cfg}
}

// NewOnyxAdapter creates a new Mellanox/NVIDIA Onyx adapter.
func NewOnyxAdapter(config *OnyxConfig) *OnyxAdapter {
	if config == nil {
		config = DefaultOnyxConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.Timeout = config.Timeout

	return &OnyxAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *OnyxAdapter) Vendor() vendors.VendorType {
	return vendors.VendorMellanoxOnyx
}

// Type implements ProtocolAdapter.Type.
func (a *OnyxAdapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *OnyxAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:  "mellanox_onyx",
		Prompts: []string{">", "#", "(config"},
	}
	shell, err := a.sshAdapter.NewNetworkDeviceShell(ctx, shellConfig)
	if err != nil {
		_ = a.sshAdapter.Disconnect(ctx)
		return fmt.Errorf("failed to create shell: %w", err)
	}
	a.shell = shell

	if _, err := a.runCommand(ctx, "enable"); err != nil {
		_ = a.sshAdapter.Disconnect(ctx)
		return fmt.Errorf("failed to enter enable mode: %w", err)
	}

	if a.Config.DisablePaging {
		_, _ = a.runCommand(ctx, "no cli session paging enable")
	}

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *OnyxAdapter) Disconnect(ctx context.Context) error {
	a.Connected = false
	a.inConfigMode = false

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
func (a *OnyxAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
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
func (a *OnyxAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
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
	output, err := a.runCommand(ctx, "show version")
	result.Latency = time.Since(start)

	if err != nil {
		result.Healthy = false
		result.Status = fmt.Sprintf("health check failed: %v", err)
		return result, nil
	}

	result.Healthy = true
	result.Status = "connected"
	result.Details["version"] = strings.TrimSpace(output)
	return result, nil
}

// IsConnected implements ProtocolAdapter.IsConnected.
func (a *OnyxAdapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *OnyxAdapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *OnyxAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	if section == "" {
		return a.runCommand(ctx, "show running-config")
	}
	return a.runCommand(ctx, fmt.Sprintf("show running-config %s", section))
}

// SetConfig implements VendorAdapter.SetConfig.
func (a *OnyxAdapter) SetConfig(ctx context.Context, commands []string) error {
	if !a.inConfigMode {
		if _, err := a.runCommand(ctx, "configure terminal"); err != nil {
			return fmt.Errorf("failed to enter config mode: %w", err)
		}
		a.inConfigMode = true
	}

	for _, cmd := range commands {
		if _, err := a.runCommand(ctx, cmd); err != nil {
			_, _ = a.runCommand(ctx, "end")
			a.inConfigMode = false
			return fmt.Errorf("command failed '%s': %w", cmd, err)
		}
	}

	_, _ = a.runCommand(ctx, "end")
	a.inConfigMode = false
	return nil
}

// GetFacts implements VendorAdapter.GetFacts.
func (a *OnyxAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "Mellanox",
		OSType: "Onyx",
		Raw:    make(map[string]string),
	}

	version, err := a.runCommand(ctx, "show version")
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}
	facts.Raw["show version"] = version
	a.parseVersion(version, facts)

	system, err := a.runCommand(ctx, "show system")
	if err == nil {
		facts.Raw["show system"] = system
		a.parseSystem(system, facts)
	}

	intfs, err := a.runCommand(ctx, "show interfaces status")
	if err == nil {
		facts.Raw["show interfaces status"] = intfs
		facts.Interfaces = a.parseInterfaces(intfs)
	}

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *OnyxAdapter) SaveConfig(ctx context.Context) error {
	_, err := a.runCommand(ctx, "configuration write")
	return err
}

func (a *OnyxAdapter) runCommand(ctx context.Context, command string) (string, error) {
	if a.shell == nil {
		return "", fmt.Errorf("shell not initialized")
	}

	result, err := a.shell.Execute(ctx, command)
	if err != nil {
		return "", err
	}

	output := result.Output
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "% invalid") ||
		strings.Contains(lowerOutput, "% incomplete") ||
		strings.Contains(lowerOutput, "% unrecognized") {
		return output, fmt.Errorf("command error: %s", output)
	}
	return output, nil
}

func (a *OnyxAdapter) parseVersion(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "Product name") || strings.Contains(line, "Product Name") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Model = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "MLNX-OS") || strings.Contains(line, "Onyx") ||
			strings.Contains(line, "Product release") {
			if match := regexp.MustCompile(`(\d+\.\d+\.\d+[\w.-]*)`).FindStringSubmatch(line); len(match) > 1 {
				if facts.OSVersion == "" {
					facts.OSVersion = match[1]
				}
			}
		}

		if strings.Contains(line, "Uptime") || strings.Contains(line, "uptime") {
			if match := regexp.MustCompile(`(?:Uptime|uptime)\s*:?\s*(.+)`).FindStringSubmatch(line); len(match) > 1 {
				facts.Uptime = parseOnyxUptime(match[1])
			}
		}
	}
}

func (a *OnyxAdapter) parseSystem(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Hostname") || strings.HasPrefix(line, "hostname") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Hostname = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "Serial") && !strings.Contains(line, "Fan") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				if facts.SerialNumber == "" {
					facts.SerialNumber = strings.TrimSpace(parts[1])
				}
			}
		}
	}
}

func (a *OnyxAdapter) parseInterfaces(output string) []vendors.InterfaceFact {
	var interfaces []vendors.InterfaceFact

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "Port") ||
			strings.HasPrefix(line, "Eth") && !strings.ContainsAny(line, "0123456789") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		name := fields[0]
		if !strings.Contains(name, "Eth") && !strings.Contains(name, "eth") &&
			!strings.Contains(name, "mgmt") && !strings.Contains(name, "Po") {
			continue
		}

		iface := vendors.InterfaceFact{Name: name}

		for _, f := range fields[1:] {
			lower := strings.ToLower(f)
			switch lower {
			case "up":
				if iface.AdminStatus == "" {
					iface.AdminStatus = "up"
				} else if iface.OperStatus == "" {
					iface.OperStatus = "up"
				}
			case "down":
				if iface.AdminStatus == "" {
					iface.AdminStatus = "down"
				} else if iface.OperStatus == "" {
					iface.OperStatus = "down"
				}
			}
		}

		if iface.AdminStatus != "" {
			interfaces = append(interfaces, iface)
		}
	}

	return interfaces
}

func parseOnyxUptime(s string) time.Duration {
	var days, hours, minutes, seconds int

	if match := regexp.MustCompile(`(\d+)\s*day`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &days)
	}
	if match := regexp.MustCompile(`(\d+)\s*hour`).FindStringSubmatch(s); len(match) > 1 {
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

// NewOnyxAdapterFactory creates an adapter factory for Mellanox/NVIDIA Onyx.
func NewOnyxAdapterFactory(config *OnyxConfig) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultOnyxConfig()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewOnyxAdapter(config), nil
	}
}

func init() {
	vendors.Register(vendors.VendorMellanoxOnyx, NewOnyxAdapterFactory(nil))
}
