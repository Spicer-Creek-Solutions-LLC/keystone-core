// Package ubiquiti provides Ubiquiti EdgeOS device adapters for proxy agents.
package ubiquiti

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

// EdgeOSAdapter implements VendorAdapter for Ubiquiti EdgeOS devices.
type EdgeOSAdapter struct {
	vendors.BaseVendorAdapter
	sshAdapter   *ssh.Adapter
	shell        *ssh.NetworkDeviceShell
	inConfigMode bool
}

// EdgeOSConfig contains Ubiquiti EdgeOS specific configuration.
type EdgeOSConfig struct {
	*vendors.VendorConfig
}

// DefaultEdgeOSConfig returns a default EdgeOS configuration.
func DefaultEdgeOSConfig() *EdgeOSConfig {
	cfg := vendors.DefaultVendorConfig()
	cfg.EnablePrompt = "$"
	cfg.ConfigPrompt = "#"
	cfg.DisablePaging = true
	return &EdgeOSConfig{VendorConfig: cfg}
}

// NewEdgeOSAdapter creates a new Ubiquiti EdgeOS adapter.
func NewEdgeOSAdapter(config *EdgeOSConfig) *EdgeOSAdapter {
	if config == nil {
		config = DefaultEdgeOSConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.Timeout = config.Timeout

	return &EdgeOSAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *EdgeOSAdapter) Vendor() vendors.VendorType {
	return vendors.VendorUbiquitiEdgeOS
}

// Type implements ProtocolAdapter.Type.
func (a *EdgeOSAdapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *EdgeOSAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:  "ubiquiti_edgeos",
		Prompts: []string{"$", "#", ">"},
	}
	shell, err := a.sshAdapter.NewNetworkDeviceShell(ctx, shellConfig)
	if err != nil {
		_ = a.sshAdapter.Disconnect(ctx)
		return fmt.Errorf("failed to create shell: %w", err)
	}
	a.shell = shell

	if a.Config.DisablePaging {
		_, _ = a.runCommand(ctx, "terminal length 0")
	}

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *EdgeOSAdapter) Disconnect(ctx context.Context) error {
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
func (a *EdgeOSAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
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
func (a *EdgeOSAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
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
func (a *EdgeOSAdapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *EdgeOSAdapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *EdgeOSAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	if section == "" {
		return a.runCommand(ctx, "show configuration commands")
	}
	return a.runCommand(ctx, fmt.Sprintf("show configuration commands | match %s", section))
}

// SetConfig implements VendorAdapter.SetConfig.
// EdgeOS uses configure/set/commit pattern similar to VyOS.
func (a *EdgeOSAdapter) SetConfig(ctx context.Context, commands []string) error {
	if _, err := a.runCommand(ctx, "configure"); err != nil {
		return fmt.Errorf("failed to enter configure mode: %w", err)
	}
	a.inConfigMode = true

	for _, cmd := range commands {
		if _, err := a.runCommand(ctx, cmd); err != nil {
			_, _ = a.runCommand(ctx, "discard")
			_, _ = a.runCommand(ctx, "exit")
			a.inConfigMode = false
			return fmt.Errorf("command failed '%s': %w", cmd, err)
		}
	}

	if _, err := a.runCommand(ctx, "commit"); err != nil {
		_, _ = a.runCommand(ctx, "discard")
		_, _ = a.runCommand(ctx, "exit")
		a.inConfigMode = false
		return fmt.Errorf("commit failed: %w", err)
	}

	_, _ = a.runCommand(ctx, "exit")
	a.inConfigMode = false
	return nil
}

// GetFacts implements VendorAdapter.GetFacts.
func (a *EdgeOSAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "Ubiquiti",
		OSType: "EdgeOS",
		Raw:    make(map[string]string),
	}

	version, err := a.runCommand(ctx, "show version")
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}
	facts.Raw["show version"] = version
	a.parseVersion(version, facts)

	hardware, err := a.runCommand(ctx, "show hardware")
	if err == nil {
		facts.Raw["show hardware"] = hardware
		a.parseHardware(hardware, facts)
	}

	hostname, err := a.runCommand(ctx, "show host name")
	if err == nil {
		facts.Hostname = strings.TrimSpace(hostname)
	}

	intfs, err := a.runCommand(ctx, "show interfaces")
	if err == nil {
		facts.Raw["show interfaces"] = intfs
		facts.Interfaces = a.parseInterfaces(intfs)
	}

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *EdgeOSAdapter) SaveConfig(ctx context.Context) error {
	_, err := a.runCommand(ctx, "save")
	return err
}

func (a *EdgeOSAdapter) runCommand(ctx context.Context, command string) (string, error) {
	if a.shell == nil {
		return "", fmt.Errorf("shell not initialized")
	}

	result, err := a.shell.Execute(ctx, command)
	if err != nil {
		return "", err
	}

	output := result.Output
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "invalid command") ||
		strings.Contains(lowerOutput, "command failed") ||
		strings.Contains(lowerOutput, "configuration path") && strings.Contains(lowerOutput, "is not valid") {
		return output, fmt.Errorf("command error: %s", output)
	}
	return output, nil
}

func (a *EdgeOSAdapter) parseVersion(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Version:") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.OSVersion = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "Uptime:") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Uptime = parseEdgeOSUptime(strings.TrimSpace(parts[1]))
			}
		}
	}
}

func (a *EdgeOSAdapter) parseHardware(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Model:") || strings.HasPrefix(line, "Hardware model:") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Model = strings.TrimSpace(parts[1])
			}
		}

		if strings.HasPrefix(line, "Serial #:") || strings.HasPrefix(line, "Serial number:") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.SerialNumber = strings.TrimSpace(parts[1])
			}
		}
	}
}

func (a *EdgeOSAdapter) parseInterfaces(output string) []vendors.InterfaceFact {
	var interfaces []vendors.InterfaceFact

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "Interface") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		iface := vendors.InterfaceFact{
			Name: fields[0],
		}

		for _, f := range fields[1:] {
			lf := strings.ToLower(f)
			switch {
			case lf == "u/u":
				iface.AdminStatus = "up"
				iface.OperStatus = "up"
			case lf == "u/d" || lf == "up/down":
				iface.AdminStatus = "up"
				iface.OperStatus = "down"
			case lf == "d/d" || lf == "a/d":
				iface.AdminStatus = "down"
				iface.OperStatus = "down"
			}
		}

		if iface.AdminStatus != "" {
			interfaces = append(interfaces, iface)
		}
	}

	return interfaces
}

func parseEdgeOSUptime(s string) time.Duration {
	var days, hours, minutes int

	if match := regexp.MustCompile(`(\d+)\s*day`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &days)
	}
	if match := regexp.MustCompile(`(\d+):(\d+)`).FindStringSubmatch(s); len(match) > 2 {
		fmt.Sscanf(match[1], "%d", &hours)
		fmt.Sscanf(match[2], "%d", &minutes)
	}

	return time.Duration(days)*24*time.Hour +
		time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute
}

// NewEdgeOSAdapterFactory creates an adapter factory for Ubiquiti EdgeOS.
func NewEdgeOSAdapterFactory(config *EdgeOSConfig) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultEdgeOSConfig()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewEdgeOSAdapter(config), nil
	}
}

func init() {
	vendors.Register(vendors.VendorUbiquitiEdgeOS, NewEdgeOSAdapterFactory(nil))
}
